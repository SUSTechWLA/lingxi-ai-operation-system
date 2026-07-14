#!/usr/bin/env python3
"""Render and fail-closed publish the standing/seated warm studio demos."""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
import shutil
import subprocess
import tempfile
from pathlib import Path
from typing import Any, Callable

import server


STANDING_SCRIPT = (
    "大家好，我是小唐。今天想和你分享一个判断：AI视频真正重要的，不只是生成速度，"
    "而是选题、脚本、画面和审核都能被理解、修改和复用。这样创作才会越来越稳定。"
)
SEATED_SCRIPT = (
    "换一个更安静的视角，我们继续聊。面对快速变化的信息，先确认事实，再形成观点，"
    "最后用清楚的结构表达出来。慢一点想明白，往往能让内容走得更远。"
)
MODE_SCRIPTS = {"standing": STANDING_SCRIPT, "seated": SEATED_SCRIPT}

FINAL_FILENAMES = {
    "standing": "Sloth_WarmStudio_Standing_Demo_1080p.mp4",
    "seated": "Sloth_WarmStudio_Seated_Demo_1080p.mp4",
    "reel": "Sloth_WarmStudio_DualMode_Reel_1080p.mp4",
    "standingContactSheet": "Sloth_WarmStudio_Standing_ContactSheet.png",
    "seatedContactSheet": "Sloth_WarmStudio_Seated_ContactSheet.png",
    "lightingComparison": "Sloth_WarmStudio_Lighting_Comparison.png",
    "report": "Sloth_WarmStudio_Integration_Report.json",
}


class DemoQAError(RuntimeError):
    """Raised when a staged production render cannot be published."""


def _sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _run(command: list[str], *, timeout: int = 600) -> subprocess.CompletedProcess[str]:
    completed = subprocess.run(
        command,
        text=True,
        capture_output=True,
        timeout=timeout,
        check=False,
    )
    if completed.returncode != 0:
        raise DemoQAError(
            f"command failed ({completed.returncode}): {' '.join(command)}\n"
            f"{completed.stderr.strip()}"
        )
    return completed


def probe_media(path: Path) -> dict[str, Any]:
    ffprobe = server.find_ffprobe()
    if not ffprobe or not path.is_file():
        return {"videoExists": path.is_file()}
    completed = _run(
        [
            ffprobe,
            "-v",
            "error",
            "-show_entries",
            "stream=index,codec_type,codec_name,width,height,r_frame_rate,avg_frame_rate,sample_rate,channels:format=duration",
            "-of",
            "json",
            str(path),
        ],
        timeout=60,
    )
    payload = json.loads(completed.stdout or "{}")
    streams = payload.get("streams") or []
    video = next((stream for stream in streams if stream.get("codec_type") == "video"), {})
    audio = next((stream for stream in streams if stream.get("codec_type") == "audio"), {})
    frame_rate = str(video.get("avg_frame_rate") or video.get("r_frame_rate") or "0/1")
    try:
        numerator, denominator = frame_rate.split("/", 1)
        fps = float(numerator) / float(denominator)
    except (TypeError, ValueError, ZeroDivisionError):
        fps = 0.0
    return {
        "videoExists": path.is_file(),
        "width": int(video.get("width") or 0),
        "height": int(video.get("height") or 0),
        "fps": round(fps, 3),
        "constantFrameRate": bool(
            video.get("r_frame_rate") == video.get("avg_frame_rate") and fps > 0
        ),
        "durationSec": round(float((payload.get("format") or {}).get("duration") or 0), 3),
        "videoCodec": str(video.get("codec_name") or ""),
        "audioCodec": str(audio.get("codec_name") or ""),
        "audioSampleRateHz": int(audio.get("sample_rate") or 0),
        "audioChannels": int(audio.get("channels") or 0),
    }


def _validate_mode_result(mode: str, result: dict[str, Any], probe: dict[str, Any]) -> None:
    errors: list[str] = []
    if result.get("success") is not True or result.get("status") != "ready":
        errors.append("renderer did not return success/ready")
    if result.get("presentationMode") != mode:
        errors.append("renderer returned the wrong presentation mode")
    if not probe.get("videoExists"):
        errors.append("staged video is missing")
    if (probe.get("width"), probe.get("height")) != (1920, 1080):
        errors.append("staged video must be 1920x1080")
    if not math.isclose(float(probe.get("fps") or 0), 30.0, abs_tol=0.01):
        errors.append("staged video must be 30 fps")
    if probe.get("constantFrameRate") is False:
        errors.append("staged video must use a constant frame rate")
    if float(probe.get("durationSec") or 0) <= 0:
        errors.append("staged video duration must be positive")

    voice = result.get("voice") or {}
    provider = str(voice.get("tts_provider") or voice.get("provider") or "")
    voice_id = str(voice.get("voice_id") or voice.get("voiceId") or "")
    if provider != "gpt_sovits_local":
        errors.append("production voice provider must be gpt_sovits_local")
    if voice_id != "main_ip_warm_knowledge_host_v1":
        errors.append("production voice ID is not the canonical main-IP voice")
    if voice.get("productionReady") is not True:
        errors.append("production voice provenance is not ready")

    loudness = (result.get("qa") or {}).get("finalAudioLoudness") or {}
    integrated = float(loudness.get("integratedLufs") or 0)
    true_peak = float(loudness.get("truePeakDbtp") or 0)
    if not math.isclose(integrated, -16.0, abs_tol=0.5):
        errors.append("final audio must be -16 +/-0.5 LUFS")
    if true_peak > -1.5:
        errors.append("final audio true peak must not exceed -1.5 dBTP")
    if errors:
        raise DemoQAError(f"{mode} QA failed: {'; '.join(errors)}")


def _ffmpeg() -> str:
    executable = server.find_ffmpeg()
    if not executable:
        raise DemoQAError("ffmpeg is unavailable")
    return executable


def _contact_sheet(video: Path, output: Path, duration: float) -> None:
    sampling_fps = 12.0 / max(duration, 0.1)
    _run(
        [
            _ffmpeg(),
            "-y",
            "-i",
            str(video),
            "-vf",
            f"fps={sampling_fps:.8f},scale=480:270:flags=lanczos,tile=4x3:nb_frames=12",
            "-frames:v",
            "1",
            str(output),
        ]
    )


def build_derived_artifacts(records: list[dict[str, Any]], paths: dict[str, Path]) -> None:
    by_mode = {str(record["mode"]): record for record in records}
    standing = Path(by_mode["standing"]["publishedStagePath"])
    seated = Path(by_mode["seated"]["publishedStagePath"])
    standing_duration = float(by_mode["standing"]["probe"]["durationSec"])
    seated_duration = float(by_mode["seated"]["probe"]["durationSec"])
    _contact_sheet(standing, paths["standingContactSheet"], standing_duration)
    _contact_sheet(seated, paths["seatedContactSheet"], seated_duration)

    _run(
        [
            _ffmpeg(),
            "-y",
            "-i",
            str(standing),
            "-i",
            str(seated),
            "-filter_complex",
            "[0:v]fps=30,scale=1920:1080:flags=lanczos,setsar=1[v0];"
            "[1:v]fps=30,scale=1920:1080:flags=lanczos,setsar=1[v1];"
            "[0:a]aresample=48000,aformat=channel_layouts=mono[a0];"
            "[1:a]aresample=48000,aformat=channel_layouts=mono[a1];"
            "[v0][a0][v1][a1]concat=n=2:v=1:a=1[v][a]",
            "-map",
            "[v]",
            "-map",
            "[a]",
            "-c:v",
            "libx264",
            "-preset",
            "medium",
            "-crf",
            "17",
            "-pix_fmt",
            "yuv420p",
            "-c:a",
            "aac",
            "-ar",
            "48000",
            "-ac",
            "1",
            str(paths["reel"]),
        ],
        timeout=1800,
    )

    _run(
        [
            _ffmpeg(),
            "-y",
            "-ss",
            f"{standing_duration * 0.35:.3f}",
            "-i",
            str(standing),
            "-ss",
            f"{seated_duration * 0.35:.3f}",
            "-i",
            str(seated),
            "-filter_complex",
            "[0:v]scale=960:540:flags=lanczos,setsar=1[left];"
            "[1:v]scale=960:540:flags=lanczos,setsar=1[right];"
            "[left][right]hstack=inputs=2",
            "-frames:v",
            "1",
            str(paths["lightingComparison"]),
        ]
    )


def _asset_path(profile_path: Path, value: str) -> Path:
    return (profile_path.parent / value).expanduser().resolve()


def _asset_record(path: Path) -> dict[str, Any]:
    return {
        "path": str(path),
        "exists": path.is_file(),
        "sha256": _sha256(path) if path.is_file() else "",
    }


def _lighting_evidence(path: Path | None) -> dict[str, Any]:
    if path is None or not path.is_file():
        raise DemoQAError("verified lighting evidence is required before publication")
    try:
        payload = json.loads(path.read_text())
    except (OSError, json.JSONDecodeError) as exc:
        raise DemoQAError(f"lighting evidence is unreadable: {exc}") from exc
    measurements = payload.get("measurements")
    comparisons = payload.get("cameraComparisons")
    expected_measurements = {
        (mode, engine, "medium")
        for mode in MODE_SCRIPTS
        for engine in ("eevee", "cycles")
    }
    expected_comparisons = {
        (mode, "medium", camera_role)
        for mode in MODE_SCRIPTS
        for camera_role in ("three_quarter", "wide")
    }
    actual_measurements = {
        (
            str(measurement.get("mode")),
            str(measurement.get("engine")),
            str(measurement.get("cameraRole")),
        )
        for measurement in measurements or []
        if isinstance(measurement, dict)
    }
    actual_comparisons = {
        (
            str(comparison.get("mode")),
            str(comparison.get("cameraRoleA")),
            str(comparison.get("cameraRoleB")),
        )
        for comparison in comparisons or []
        if isinstance(comparison, dict)
    }
    if (
        payload.get("schemaVersion") != "tangying-warm-studio-lighting-evidence/v1"
        or payload.get("success") is not True
        or payload.get("errors") != []
        or not isinstance(measurements, list)
        or len(measurements) != 4
        or actual_measurements != expected_measurements
        or not isinstance(comparisons, list)
        or len(comparisons) != 4
        or actual_comparisons != expected_comparisons
    ):
        raise DemoQAError("lighting evidence is incomplete or did not pass its canonical gates")
    for measurement in measurements:
        try:
            stops = float(measurement["backgroundStopsBelowFace"])
            clip_ratio = float(measurement["highlightClipRatio"])
            red_blue = float(measurement["brightNeutralRedBlueRatio"])
            red_green = float(measurement["brightNeutralRedGreenRatio"])
        except (KeyError, TypeError, ValueError) as exc:
            raise DemoQAError("lighting evidence has incomplete numeric gates") from exc
        if not (
            1.0 <= stops <= 1.5
            and 0.0 <= clip_ratio < 0.005
            and 0.95 <= red_blue <= 1.22
            and 0.95 <= red_green <= 1.14
        ):
            raise DemoQAError("lighting evidence contains a failed measurement gate")
    return {
        "path": str(path.resolve()),
        "available": True,
        "sha256": _sha256(path),
        "schemaVersion": payload["schemaVersion"],
        "measurements": measurements,
        "cameraComparisons": comparisons,
    }


def _embedded_collision_report(mode: str, result: dict[str, Any]) -> dict[str, Any]:
    source = Path(str(result.get("rigReportPath") or "")).expanduser().resolve()
    if not source.is_file():
        raise DemoQAError(f"{mode} rig/collision report is missing")
    try:
        payload = json.loads(source.read_text())
    except (OSError, json.JSONDecodeError) as exc:
        raise DemoQAError(f"{mode} rig/collision report is unreadable: {exc}") from exc
    if not isinstance(payload, dict) or payload.get("success") is not True:
        raise DemoQAError(f"{mode} rig/collision report did not pass")
    return {
        "sourceFileName": source.name,
        "sha256": _sha256(source),
        "report": payload,
    }


def publish_transaction(
    staged_paths: dict[str, Path],
    output_dir: Path,
    *,
    replace_file: Callable[[Path, Path], None] = os.replace,
) -> None:
    """Publish the complete artifact set and roll back any interrupted replacement."""

    backup_dir = next(iter(staged_paths.values())).parent.parent / "publication-backup"
    backup_dir.mkdir()
    destinations = {
        key: output_dir / FINAL_FILENAMES[key]
        for key in FINAL_FILENAMES
    }
    backed_up: list[str] = []
    published: list[str] = []
    try:
        for key, destination in destinations.items():
            if destination.exists():
                os.replace(destination, backup_dir / FINAL_FILENAMES[key])
                backed_up.append(key)
        for key in FINAL_FILENAMES:
            replace_file(staged_paths[key], destinations[key])
            published.append(key)
    except Exception as exc:
        rollback_errors: list[str] = []
        for key in published:
            try:
                destinations[key].unlink(missing_ok=True)
            except OSError as rollback_exc:
                rollback_errors.append(f"remove {key}: {rollback_exc}")
        for key in backed_up:
            try:
                os.replace(backup_dir / FINAL_FILENAMES[key], destinations[key])
            except OSError as rollback_exc:
                rollback_errors.append(f"restore {key}: {rollback_exc}")
        suffix = f"; rollback errors: {rollback_errors}" if rollback_errors else ""
        raise DemoQAError(f"publication transaction failed: {exc}{suffix}") from exc
    finally:
        shutil.rmtree(backup_dir, ignore_errors=True)


def render_warm_studio_demos(
    profile_path: Path,
    output_dir: Path,
    *,
    renderer: Callable[..., dict[str, Any]] | None = None,
    media_probe: Callable[[Path], dict[str, Any]] | None = None,
    artifact_builder: Callable[[list[dict[str, Any]], dict[str, Path]], None] | None = None,
    lighting_evidence_path: Path | None = None,
    publisher: Callable[[dict[str, Path], Path], None] | None = None,
) -> dict[str, Any]:
    profile_path = profile_path.expanduser().resolve()
    output_dir = output_dir.expanduser().resolve()
    if not profile_path.is_file():
        raise FileNotFoundError(f"character profile does not exist: {profile_path}")
    profile = json.loads(profile_path.read_text())
    voice = profile.get("voice") or {}
    if (
        voice.get("provider") != "gpt_sovits_local"
        or voice.get("voiceId") != "main_ip_warm_knowledge_host_v1"
        or voice.get("fallbackPolicy") != "error"
    ):
        raise DemoQAError("character profile does not pin the approved production voice")
    lighting_evidence = _lighting_evidence(lighting_evidence_path)

    renderer = renderer or server.render_talking_video
    media_probe = media_probe or probe_media
    artifact_builder = artifact_builder or build_derived_artifacts
    publisher = publisher or publish_transaction
    output_dir.mkdir(parents=True, exist_ok=True)
    staging_root = Path(tempfile.mkdtemp(prefix=".warm-studio-demo-", dir=output_dir))
    publish_stage = staging_root / "publish"
    publish_stage.mkdir()
    staged_paths = {key: publish_stage / filename for key, filename in FINAL_FILENAMES.items()}
    records: list[dict[str, Any]] = []

    try:
        for mode, script in MODE_SCRIPTS.items():
            mode_dir = staging_root / mode
            result = renderer(
                script=script,
                characterProfilePath=str(profile_path),
                outputDir=str(mode_dir),
                presentationMode=mode,
                width=1920,
                height=1080,
                fps=30,
                qualityPreset="production_1080p",
                renderMode="production",
                voiceProvider="gpt_sovits_local",
                voiceId="main_ip_warm_knowledge_host_v1",
                fallbackPolicy="error",
            )
            video_path = Path(str(result.get("videoPath") or "")).expanduser().resolve()
            probe = media_probe(video_path)
            _validate_mode_result(mode, result, probe)
            collision_report = _embedded_collision_report(mode, result)
            shutil.copy2(video_path, staged_paths[mode])
            records.append(
                {
                    "mode": mode,
                    "script": script,
                    "result": result,
                    "probe": probe,
                    "collisionReport": collision_report,
                    "publishedStagePath": str(staged_paths[mode]),
                }
            )

        artifact_builder(records, staged_paths)
        reel_probe = media_probe(staged_paths["reel"])
        if (reel_probe.get("width"), reel_probe.get("height")) != (1920, 1080):
            raise DemoQAError("combined reel must be 1920x1080")
        if not math.isclose(float(reel_probe.get("fps") or 0), 30.0, abs_tol=0.01):
            raise DemoQAError("combined reel must be 30 fps")

        render_config = profile.get("render") or {}
        model_config = profile.get("model") or {}
        scene_path = _asset_path(profile_path, str(render_config.get("sceneBlendPath") or ""))
        master_path = _asset_path(profile_path, str(model_config.get("masterBlendPath") or ""))
        mode_reports = {
            str(record["mode"]): {
                "script": record["script"],
                "presentationMode": record["mode"],
                "videoSha256": _sha256(staged_paths[str(record["mode"])]),
                "qa": record["probe"],
                "renderQa": record["result"].get("qa") or {},
                "voice": record["result"].get("voice") or {},
                "voicePolicy": record["result"].get("voicePolicy") or {},
                "renderProvenance": {
                    "sceneBlendPath": record["result"].get("sceneBlendPath") or "",
                    "masterBlendPath": record["result"].get("masterBlendPath") or "",
                    "renderReportSha256": (
                        _sha256(Path(str(record["result"].get("renderReportPath"))).resolve())
                        if record["result"].get("renderReportPath")
                        and Path(str(record["result"].get("renderReportPath"))).is_file()
                        else ""
                    ),
                },
                "collisionReport": record["collisionReport"],
            }
            for record in records
        }
        report: dict[str, Any] = {
            "schemaVersion": "tangying-sloth-warm-studio-integration/v1",
            "success": True,
            "profile": _asset_record(profile_path),
            "character": _asset_record(master_path),
            "studio": _asset_record(scene_path),
            "voicePolicy": {
                "provider": voice.get("provider"),
                "voiceId": voice.get("voiceId"),
                "fallbackPolicy": voice.get("fallbackPolicy"),
                "gptSovitsLocal": voice.get("gptSovitsLocal"),
            },
            "modes": mode_reports,
            "combinedReelQa": reel_probe,
            "lightingEvidence": lighting_evidence,
            "collisionReportPointers": [
                f"{FINAL_FILENAMES['report']}#/modes/{mode}/collisionReport"
                for mode in MODE_SCRIPTS
            ],
        }
        report["outputs"] = {
            key: {
                "path": str(output_dir / FINAL_FILENAMES[key]),
                "sha256": _sha256(path),
            }
            for key, path in staged_paths.items()
            if key != "report" and path.is_file()
        }
        staged_paths["report"].write_text(
            json.dumps(report, ensure_ascii=False, indent=2) + "\n"
        )
        missing = [key for key, path in staged_paths.items() if not path.is_file()]
        if missing:
            raise DemoQAError(f"staged publication is incomplete: {missing}")

        publisher(staged_paths, output_dir)
        return report
    finally:
        shutil.rmtree(staging_root, ignore_errors=True)


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--profile", type=Path, required=True)
    parser.add_argument("--output-dir", type=Path, required=True)
    parser.add_argument("--lighting-evidence", type=Path, required=True)
    return parser


def main() -> int:
    args = _parser().parse_args()
    report = render_warm_studio_demos(
        args.profile,
        args.output_dir,
        lighting_evidence_path=args.lighting_evidence,
    )
    print(json.dumps(report, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
