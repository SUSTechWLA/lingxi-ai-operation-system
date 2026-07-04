#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${BETA_FIXTURE_OUT:-$ROOT_DIR/scripts/tmp/beta-fallback-fixture}"
PROJECT_ID="beta-fixture"
VIDEO_PATH="$OUT_DIR/final-preview.mp4"
REPORT_DIR="$OUT_DIR/reports/video_frame_qa"

command -v ffmpeg >/dev/null 2>&1 || {
  echo "ffmpeg is required for beta fallback fixture" >&2
  exit 1
}
command -v python3 >/dev/null 2>&1 || {
  echo "python3 is required for beta fallback fixture" >&2
  exit 1
}

rm -rf "$OUT_DIR"
mkdir -p "$REPORT_DIR"

ffmpeg -y -v error \
  -f lavfi -i "testsrc2=size=640x360:rate=24" \
  -t 6 \
  -pix_fmt yuv420p \
  "$VIDEO_PATH"

python3 - "$ROOT_DIR" "$OUT_DIR" "$PROJECT_ID" "$VIDEO_PATH" "$REPORT_DIR" <<'PY'
import importlib.util
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
out_dir = pathlib.Path(sys.argv[2])
project_id = sys.argv[3]
video_path = pathlib.Path(sys.argv[4])
report_dir = pathlib.Path(sys.argv[5])

server_path = root / "mcp" / "video_qa" / "server.py"
spec = importlib.util.spec_from_file_location("video_qa_server", server_path)
module = importlib.util.module_from_spec(spec)
assert spec and spec.loader
spec.loader.exec_module(module)

shot_list = [
    {
        "id": "SHOT_01",
        "shotId": "SHOT_01",
        "durationSec": 3,
        "visual": "Fallback preview color test pattern for the first beta shot.",
        "narrationText": "Closed beta fallback preview starts.",
        "whyThisShot": "Smoke fixture proves QA can evaluate a local preview without AIGC provider access.",
        "plannedAssetRoute": "fallback_preview",
        "actionBeats": ["0-1秒 test pattern moves", "1-3秒 stable preview continues"],
    },
    {
        "id": "SHOT_02",
        "shotId": "SHOT_02",
        "durationSec": 3,
        "visual": "Fallback preview color test pattern for the second beta shot.",
        "narrationText": "Closed beta fallback preview ends.",
        "whyThisShot": "Smoke fixture records fallback provenance and repair plan.",
        "plannedAssetRoute": "fallback_preview",
        "actionBeats": ["3-4秒 pattern changes", "4-6秒 final preview holds"],
    },
]

qa = module.analyze_video(
    projectId=project_id,
    videoPath=str(video_path),
    outputDir=str(report_dir),
    outputRefPrefix=f"local://projects/{project_id}/reports/video_frame_qa",
    sampleIntervalSec=2,
    shotList=shot_list,
    videoType="hybrid",
    candidateId="fallback-smoke",
)

manifest = {
    "schemaVersion": 1,
    "projectId": project_id,
    "mode": "hybrid",
    "finalArtifact": {
        "id": "final-preview",
        "kind": "VIDEO",
        "path": str(video_path),
        "sourceType": "fallback_preview",
        "providerName": "local-ffmpeg-testsrc2",
        "providerJobId": "beta-fallback-fixture",
        "fallbackReason": "closed_beta_no_external_provider_fixture",
        "isFallback": True,
        "generatedAt": module._now_iso(),
        "inputPromptHash": "",
        "sourceArtifactIds": [],
    },
    "qa": {
        "passed": qa.get("passed"),
        "score": qa.get("score"),
        "shotCount": qa.get("shotCount"),
        "repairPlan": qa.get("repairPlan"),
        "shotReportsPath": str(report_dir / "shot_qa_reports.json"),
        "videoFrameQAPath": str(report_dir / "video_frame_qa.json"),
    },
}
(out_dir / "artifact_manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps({"ok": True, "outDir": str(out_dir), "manifest": str(out_dir / "artifact_manifest.json")}, ensure_ascii=False))
PY

echo "Fallback fixture written to: $OUT_DIR"
