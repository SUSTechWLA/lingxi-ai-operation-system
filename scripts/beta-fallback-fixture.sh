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

shot_candidates = [
    {
        "shotId": shot["shotId"],
        "candidateId": f"{shot['shotId']}-fallback-candidate",
        "attemptIndex": 0,
        "status": "ACCEPTED_FOR_ASSEMBLY",
        "durationSec": shot["durationSec"],
        "sourceType": "fallback_preview",
        "isFallback": True,
        "artifactRefs": [{"artifactId": "final-preview", "sourceType": "fallback_preview", "isFallback": True}],
        "qaReport": next((report for report in qa.get("shotReports", []) if report.get("shotId") == shot["shotId"]), {}),
    }
    for shot in shot_list
]
accepted_shots = [
    {
        "shotId": item["shotId"],
        "candidateId": item["candidateId"],
        "durationSec": item["durationSec"],
        "sourceType": item["sourceType"],
        "isFallback": item["isFallback"],
        "artifactId": "final-preview",
    }
    for item in shot_candidates
]
assembly_plan = {
    "status": "fallback_preview_ready",
    "resolution": "640x360",
    "fps": 24,
    "pixelFormat": "yuv420p",
    "codec": "h264",
    "steps": [
        "ALL_SHOTS_ACCEPTED_GATE",
        "NORMALIZE_ACCEPTED_SHOTS",
        "FFMPEG_CONCAT",
        "GLOBAL_VOICEOVER_ALIGN",
        "GLOBAL_BGM_MIX_AND_DUCKING",
        "GLOBAL_SUBTITLE_RENDER",
        "FINAL_VIDEO_QA",
        "EXPORT_PUBLISH",
    ],
    "acceptedShots": accepted_shots,
}
subtitle_timeline = {
    "scope": "global",
    "cues": [
        {"shotId": "SHOT_01", "startSec": 0, "endSec": 3, "text": "Closed beta fallback preview starts."},
        {"shotId": "SHOT_02", "startSec": 3, "endSec": 6, "text": "Closed beta fallback preview ends."},
    ],
}
audio_mix_plan = {
    "scope": "global",
    "voiceoverAlign": True,
    "bgmDucking": True,
    "targetLufs": -16,
    "note": "Fallback fixture has no baked per-shot BGM; final audio is handled at assembly scope.",
}
final_qa_report = {
    "status": "passed" if qa.get("passed") else "failed",
    "passed": bool(qa.get("passed")),
    "reportRef": f"local://projects/{project_id}/reports/video_frame_qa/video_frame_qa.json",
    "summary": qa.get("summary", ""),
}
provenance_summary = {
    "rawShotCandidateCount": len(shot_candidates),
    "repairedShotCandidateCount": 0,
    "acceptedShotCount": len(accepted_shots),
    "normalizedShotClipCount": len(accepted_shots),
    "concatVideoCount": 1,
    "finalAudioMixCount": 1,
    "finalSubtitleTrackCount": 1,
    "finalVideoCount": 1 if qa.get("passed") else 0,
    "realAigcVideoCount": 0,
    "fallbackCount": len(shot_candidates) + 1,
}
pipeline_reports = {
    "shot_list.json": {"schemaVersion": 1, "shots": shot_list},
    "shot_split_report.json": {
        "schemaVersion": 1,
        "policy": {
            "minShotDurationSec": 3,
            "maxShotDurationSec": 15,
            "preferredShotDurationSec": "6-8",
            "splitByScriptSemantics": True,
            "splitByVisualChange": True,
        },
        "reason": "fixture uses two semantic fallback shots with global assembly.",
    },
    "shot_duration_validation.json": {"schemaVersion": 1, "valid": True, "durations": [{"shotId": shot["shotId"], "durationSec": shot["durationSec"]} for shot in shot_list]},
    "shot_candidates.json": {"schemaVersion": 1, "shotCandidates": shot_candidates},
    "repair_plans.json": {"schemaVersion": 1, "repairPlans": [report.get("repairPlan", {}) for report in qa.get("shotReports", [])]},
    "accepted_shots.json": {"schemaVersion": 1, "acceptedShots": accepted_shots},
    "assembly_plan.json": {"schemaVersion": 1, "assemblyPlan": assembly_plan},
    "subtitle_timeline.json": {"schemaVersion": 1, "subtitleTimeline": subtitle_timeline},
    "audio_mix_plan.json": {"schemaVersion": 1, "audioMixPlan": audio_mix_plan},
    "final_qa_report.json": {"schemaVersion": 1, "finalQaReport": final_qa_report},
    "provenance_summary.json": {"schemaVersion": 1, "provenanceSummary": provenance_summary},
}
for filename, payload in pipeline_reports.items():
    (report_dir / filename).write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

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
    "shotCandidates": shot_candidates,
    "acceptedShots": accepted_shots,
    "assemblyPlan": assembly_plan,
    "subtitleTimeline": subtitle_timeline,
    "audioMixPlan": audio_mix_plan,
    "finalQaReport": final_qa_report,
    "provenanceSummary": provenance_summary,
}
(out_dir / "artifact_manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps({"ok": True, "outDir": str(out_dir), "manifest": str(out_dir / "artifact_manifest.json")}, ensure_ascii=False))
PY

echo "Fallback fixture written to: $OUT_DIR"
