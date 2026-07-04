#!/usr/bin/env bash
set -uo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${BETA_READINESS_OUT:-$ROOT_DIR/scripts/tmp/beta-readiness}"
LOCAL_AGENT_URL="${LOCAL_AGENT_URL:-http://127.0.0.1:18080}"
HYPERFRAMES_URL="${HYPERFRAMES_URL:-http://127.0.0.1:8787}"
REQUIRE_REAL_AIGC="${BETA_READINESS_REQUIRE_AIGC:-0}"

info() { printf '\033[1;34m[INFO]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[WARN]\033[0m %s\n' "$*"; }

mkdir -p "$OUT_DIR"

fetch_json() {
  local url="$1"
  local dest="$2"
  if ! command -v curl >/dev/null 2>&1; then
    return 1
  fi
  curl -fsS --max-time 3 "$url" -o "$dest" >/dev/null 2>&1
}

post_json() {
  local url="$1"
  local dest="$2"
  local body="$3"
  if ! command -v curl >/dev/null 2>&1; then
    return 1
  fi
  curl -fsS --max-time 8 -X POST "$url" \
    -H 'Content-Type: application/json' \
    -d "$body" \
    -o "$dest" >/dev/null 2>&1
}

info "Running closed beta smoke check"
BETA_FIXTURE_OUT="$OUT_DIR/fallback-fixture" bash "$ROOT_DIR/scripts/beta-smoke-check.sh" >"$OUT_DIR/beta-smoke.log" 2>&1
SMOKE_EXIT=$?
if [[ "$SMOKE_EXIT" -ne 0 ]]; then
  warn "beta smoke failed; see $OUT_DIR/beta-smoke.log"
fi

fetch_json "$LOCAL_AGENT_URL/api/local/health" "$OUT_DIR/local-agent-health.json" || true
fetch_json "$LOCAL_AGENT_URL/api/local/mcp-providers/status" "$OUT_DIR/mcp-provider-status.json" || true
fetch_json "$LOCAL_AGENT_URL/api/local/model-providers" "$OUT_DIR/model-providers.json" || true
fetch_json "$HYPERFRAMES_URL/health" "$OUT_DIR/hyperframes-health.json" || true

if [[ -s "$OUT_DIR/local-agent-health.json" ]]; then
  post_json "$LOCAL_AGENT_URL/api/local/diagnostics" "$OUT_DIR/diagnostics-response.json" '{"reason":"beta-readiness-check"}' || true
fi

python3 - "$OUT_DIR" "$SMOKE_EXIT" <<'PY'
import json
import shutil
import sys
from pathlib import Path

out_dir = Path(sys.argv[1])
smoke_exit = int(sys.argv[2])

def load_json(name):
    path = out_dir / name
    if not path.exists() or path.stat().st_size == 0:
        return None
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError:
        return None

def status_from_health(payload):
    if isinstance(payload, dict) and str(payload.get("status", "")).lower() == "ok":
        return {"status": "ok"}
    return {"status": "unknown"}

def normalize_mcp(payload):
    providers = []
    for provider in (payload or {}).get("providers", []):
        tools = []
        capabilities = []
        for tool in provider.get("tools", []) or []:
            name = str(tool.get("name", ""))
            if name:
                tools.append(name)
            lower = name.lower()
            if "generate_video" in lower and "aigc_video" not in capabilities:
                capabilities.append("aigc_video")
            if "generate_image" in lower and "aigc_image" not in capabilities:
                capabilities.append("aigc_image")
        providers.append({
            "id": provider.get("id", "unknown"),
            "status": "ok" if provider.get("reachable") else "unhealthy",
            "capabilities": capabilities,
            "tools": tools,
        })
    return providers

def normalize_models(payload):
    result = {}
    providers = (payload or {}).get("providers", {})
    for key, provider in providers.items():
        configured = bool(provider.get("hasApiKey") and provider.get("baseUrl") and provider.get("model"))
        result[key] = {"configured": configured}
    return result

def qa_fixture_status():
    manifest = load_json("fallback-fixture/artifact_manifest.json")
    qa = manifest.get("qa", {}) if isinstance(manifest, dict) else {}
    shot_reports_path = qa.get("shotReportsPath")
    reports = None
    if isinstance(shot_reports_path, str) and Path(shot_reports_path).exists():
        try:
            reports = json.loads(Path(shot_reports_path).read_text(encoding="utf-8"))
        except json.JSONDecodeError:
            reports = None
    if reports is None:
        reports = load_json("fallback-fixture/reports/video_frame_qa/shot_qa_reports.json")
    repair = qa.get("repairPlan") if isinstance(qa.get("repairPlan"), dict) else load_json("fallback-fixture/reports/video_frame_qa/shot_repair_plan.json")
    if isinstance(reports, dict):
        report_count = len(reports.get("shotReports", reports.get("reports", [])))
    elif isinstance(reports, list):
        report_count = len(reports)
    else:
        report_count = 0
    return {
        "passed": report_count > 0 and isinstance(repair, dict),
        "structuredShotReports": report_count,
        "repairPlanAvailable": isinstance(repair, dict) and bool(repair),
    }

diagnostics = load_json("diagnostics-response.json")
snapshot = {
    "schemaVersion": 1,
    "smoke": {"passed": smoke_exit == 0},
    "localAgent": status_from_health(load_json("local-agent-health.json")),
    "hyperframes": status_from_health(load_json("hyperframes-health.json")),
    "ffmpeg": {"available": shutil.which("ffmpeg") is not None},
    "diagnostics": {"available": isinstance(diagnostics, dict) and bool(diagnostics.get("path"))},
    "qaFixture": qa_fixture_status(),
    "mcpProviders": normalize_mcp(load_json("mcp-provider-status.json")),
    "modelProviders": normalize_models(load_json("model-providers.json")),
}
(out_dir / "readiness-input.json").write_text(json.dumps(snapshot, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
PY

EVALUATOR_ARGS=(--input "$OUT_DIR/readiness-input.json" --output "$OUT_DIR/readiness-report.json")
if [[ "$REQUIRE_REAL_AIGC" == "1" ]]; then
  EVALUATOR_ARGS+=(--require-real-aigc)
fi

info "Evaluating beta readiness"
python3 "$ROOT_DIR/scripts/beta_readiness.py" "${EVALUATOR_ARGS[@]}"
EVAL_EXIT=$?
printf '\nReadiness input: %s\nReadiness report: %s\nSmoke log: %s\n' \
  "$OUT_DIR/readiness-input.json" \
  "$OUT_DIR/readiness-report.json" \
  "$OUT_DIR/beta-smoke.log"
exit "$EVAL_EXIT"
