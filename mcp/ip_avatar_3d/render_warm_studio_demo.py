#!/usr/bin/env python3
"""Render and fail-closed publish one continuous warm-studio A-roll demo."""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
import shutil
import struct
import subprocess
import tempfile
import zlib
from pathlib import Path
from typing import Any, Callable

import server
import aroll_performance_qa


DEMO_JOB = {
    "kind": "standSitActionPack",
    "presentationMode": "standing",
    "cameraPreset": "transition",
    "script": (
        "大家好，我是小唐。先说结论：AI创作真正重要的是可控。我们坐下来拆开看，"
        "选题、脚本、画面和审核都要能修改和复用。最后总结，稳定流程才能带来稳定内容。"
    ),
    "actionSequence": [
        "Aroll_Welcome_OpenArms",
        "Aroll_KeyPoint_OneFinger",
        "Aroll_Transition_StandToSit",
        "Aroll_Seated_Explain",
        "Aroll_Question_PalmUp",
        "Aroll_Seated_LeanIn",
        "Aroll_Transition_SitToStand",
        "Aroll_Conclusion_HandsTogether",
    ],
    "minimumDurationSec": 15.0,
    "maximumDurationSec": 30.0,
}
REQUIRED_LIGHTING_MODES = ("standing", "seated")
ACTION_CATALOG_VERSION = "tangying-ip-aroll-action-catalog/v1"

FINAL_FILENAMES = {
    "seated": "Sloth_WarmStudio_Seated_Demo_1080p.mp4",
    "standSit": "Sloth_WarmStudio_StandSit_Demo_1080p.mp4",
    "reel": "Sloth_WarmStudio_DualMode_Reel_1080p.mp4",
    "actionPack": "Sloth_WarmStudio_ActionPack_1080p.mp4",
    "transitionContactSheet": "Sloth_WarmStudio_Transition_ContactSheet.png",
    "visemeComparison": "Sloth_WarmStudio_Viseme_Comparison.png",
    "report": "Sloth_WarmStudio_ActionPack_Report.json",
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
    duration = float(probe.get("durationSec") or 0)
    if not (
        float(DEMO_JOB["minimumDurationSec"])
        <= duration
        <= float(DEMO_JOB["maximumDurationSec"])
    ):
        errors.append("staged video must be between 15 and 30 seconds")

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


def _transition_bounds(
    performance: dict[str, Any], canonical_duration: float
) -> tuple[float, float]:
    events = performance.get("motionEvents") or []
    stand_to_sit = next(
        (
            event
            for event in events
            if isinstance(event, dict)
            and event.get("action") == "Aroll_Transition_StandToSit"
        ),
        None,
    )
    sit_to_stand = next(
        (
            event
            for event in events
            if isinstance(event, dict)
            and event.get("action") == "Aroll_Transition_SitToStand"
        ),
        None,
    )
    if not isinstance(stand_to_sit, dict) or not isinstance(sit_to_stand, dict):
        raise DemoQAError("canonical performance is missing both physical transitions")
    start = max(0.0, float(stand_to_sit["timeSec"]) - 0.4)
    end = min(
        canonical_duration,
        float(sit_to_stand["timeSec"]) + float(sit_to_stand["duration"]) + 0.4,
    )
    if end <= start:
        raise DemoQAError("canonical transition review window is empty")
    return start, end


def _transition_contact_sheet(
    video: Path,
    output: Path,
    start: float,
    end: float,
) -> None:
    sampling_fps = 18.0 / max(end - start, 0.1)
    _run(
        [
            _ffmpeg(),
            "-y",
            "-ss",
            f"{start:.6f}",
            "-t",
            f"{end - start:.6f}",
            "-i",
            str(video),
            "-vf",
            f"fps={sampling_fps:.8f},scale=320:180:flags=lanczos,"
            "tile=6x3:nb_frames=18:padding=0:margin=0",
            "-frames:v",
            "1",
            str(output),
        ]
    )


def _viseme_selections(
    performance: dict[str, Any], fps: int
) -> list[dict[str, Any]]:
    visemes = performance.get("visemes") or {}
    evidence = visemes.get("evidence") or {}
    timeline = evidence.get("evaluatedJawTimeline") or {}
    samples = [item for item in timeline.get("samples") or [] if isinstance(item, dict)]
    if not samples:
        raise DemoQAError("evaluated viseme timeline is unavailable")
    aliases = {
        "Mouth_Rest": {"closed", "rest"},
        "Mouth_MBP": {"mbp"},
        "Mouth_A": {"a"},
        "Mouth_E": {"e"},
        "Mouth_O": {"o"},
        "Mouth_U": {"u"},
        "Mouth_Surprise": {"surprise"},
    }
    selections: list[dict[str, Any]] = []
    for label, accepted in aliases.items():
        candidates = [
            item for item in samples if str(item.get("viseme") or "").lower() in accepted
        ]
        fallback = not candidates
        candidates = candidates or samples
        selector = min if label in {"Mouth_Rest", "Mouth_MBP"} else max
        selected = selector(candidates, key=lambda item: float(item.get("jawRadians") or 0.0))
        frame = int(selected.get("frame") or 1)
        selections.append(
            {
                "label": label,
                "frame": frame,
                "timeSec": round(max(0, frame - 1) / float(fps), 6),
                "viseme": str(selected.get("viseme") or ""),
                "jawRadians": float(selected.get("jawRadians") or 0.0),
                "fallbackToJawExtreme": fallback,
            }
        )
    return selections


def _viseme_comparison(
    video: Path,
    output: Path,
    selections: list[dict[str, Any]],
) -> None:
    glyphs = {
        "A": ("01110", "10001", "10001", "11111", "10001", "10001", "10001"),
        "B": ("11110", "10001", "10001", "11110", "10001", "10001", "11110"),
        "E": ("11111", "10000", "10000", "11110", "10000", "10000", "11111"),
        "I": ("11111", "00100", "00100", "00100", "00100", "00100", "11111"),
        "M": ("10001", "11011", "10101", "10101", "10001", "10001", "10001"),
        "O": ("01110", "10001", "10001", "10001", "10001", "10001", "01110"),
        "P": ("11110", "10001", "10001", "11110", "10000", "10000", "10000"),
        "R": ("11110", "10001", "10001", "11110", "10100", "10010", "10001"),
        "S": ("01111", "10000", "10000", "01110", "00001", "00001", "11110"),
        "T": ("11111", "00100", "00100", "00100", "00100", "00100", "00100"),
        "U": ("10001", "10001", "10001", "10001", "10001", "10001", "01110"),
    }

    def write_label_overlay(path: Path) -> None:
        width, height = 1920, 540
        pixels = bytearray(width * height * 4)
        short_labels = ("REST", "MBP", "A", "E", "O", "U", "SURPRISE")
        for index, label in enumerate(short_labels):
            cell_x = (index % 4) * 480
            cell_y = (index // 4) * 270
            for y in range(cell_y, cell_y + 42):
                row = (y * width + cell_x) * 4
                for x in range(480):
                    offset = row + x * 4
                    pixels[offset : offset + 4] = bytes((0, 0, 0, 178))
            cursor_x = cell_x + 12
            for character in label:
                glyph = glyphs[character]
                for glyph_y, bits in enumerate(glyph):
                    for glyph_x, enabled in enumerate(bits):
                        if enabled != "1":
                            continue
                        for dy in range(4):
                            for dx in range(4):
                                x = cursor_x + glyph_x * 4 + dx
                                y = cell_y + 7 + glyph_y * 4 + dy
                                offset = (y * width + x) * 4
                                pixels[offset : offset + 4] = bytes((255, 255, 255, 255))
                cursor_x += 24

        def chunk(kind: bytes, payload: bytes) -> bytes:
            return (
                struct.pack(">I", len(payload))
                + kind
                + payload
                + struct.pack(">I", zlib.crc32(kind + payload) & 0xFFFFFFFF)
            )

        scanlines = b"".join(
            b"\x00" + bytes(pixels[y * width * 4 : (y + 1) * width * 4])
            for y in range(height)
        )
        path.write_bytes(
            b"\x89PNG\r\n\x1a\n"
            + chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 6, 0, 0, 0))
            + chunk(b"IDAT", zlib.compress(scanlines, 9))
            + chunk(b"IEND", b"")
        )

    label_overlay = output.with_name(f".{output.stem}-labels.png")
    write_label_overlay(label_overlay)
    split_outputs = "".join(f"[source{index}]" for index in range(len(selections)))
    filters = [f"[0:v]split={len(selections)}{split_outputs}"]
    for index, selection in enumerate(selections):
        start = float(selection["timeSec"])
        filters.append(
            f"[source{index}]trim=start={start:.6f}:end={start + 0.05:.6f},"
            "setpts=PTS-STARTPTS,scale=480:270:flags=lanczos"
            f"[viseme{index}]"
        )
    stack_inputs = "".join(f"[viseme{index}]" for index in range(len(selections)))
    layout = "0_0|480_0|960_0|1440_0|0_270|480_270|960_270"
    filters.append(
        f"{stack_inputs}xstack=inputs={len(selections)}:layout={layout}:fill=black[grid]"
    )
    filters.append("[grid][1:v]overlay=0:0[out]")
    try:
        _run(
            [
                _ffmpeg(),
                "-y",
                "-i",
                str(video),
                "-i",
                str(label_overlay),
                "-filter_complex",
                ";".join(filters),
                "-map",
                "[out]",
                "-frames:v",
                "1",
                str(output),
            ],
        )
    finally:
        label_overlay.unlink(missing_ok=True)


def build_derived_artifacts(records: list[dict[str, Any]], paths: dict[str, Path]) -> None:
    if len(records) != 1:
        raise DemoQAError("derived artifacts require exactly one canonical render")
    record = records[0]
    canonical = Path(record["publishedStagePath"])
    canonical_duration = float(record["probe"]["durationSec"])
    performance = record["collisionReport"]["report"]["arollPerformanceQa"]
    transition_start, transition_end = _transition_bounds(
        performance, canonical_duration
    )
    seated_start = max(
        0.0,
        min(transition_start, canonical_duration - 15.0),
    )
    seated_duration = min(15.0, canonical_duration - seated_start)
    if seated_duration < 15.0 - 1e-6:
        raise DemoQAError("canonical render is too short for the seated review clip")

    shutil.copy2(canonical, paths["reel"])
    shutil.copy2(canonical, paths["actionPack"])
    _run(
        [
            _ffmpeg(),
            "-y",
            "-ss",
            f"{seated_start:.6f}",
            "-i",
            str(canonical),
            "-t",
            f"{seated_duration:.6f}",
            "-vf",
            "fps=30,scale=1920:1080:flags=lanczos,setsar=1",
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
            str(paths["seated"]),
        ],
        timeout=1800,
    )
    _transition_contact_sheet(
        canonical,
        paths["transitionContactSheet"],
        transition_start,
        transition_end,
    )
    fps = int((performance.get("sampleCadence") or {}).get("fps") or 30)
    selections = _viseme_selections(performance, fps)
    _viseme_comparison(canonical, paths["visemeComparison"], selections)
    record["derivedEvidence"] = {
        "seatedClip": {
            "startSec": round(seated_start, 6),
            "durationSec": round(seated_duration, 6),
        },
        "transitionWindow": {
            "startSec": round(transition_start, 6),
            "endSec": round(transition_end, 6),
            "sampleCount": 18,
        },
        "visemeSelections": selections,
    }


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
        for mode in REQUIRED_LIGHTING_MODES
        for engine in ("eevee", "cycles")
    }
    expected_comparisons = {
        (mode, "medium", camera_role)
        for mode in REQUIRED_LIGHTING_MODES
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


def _strict_report_number(value: Any) -> float | None:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        return None
    parsed = float(value)
    return parsed if math.isfinite(parsed) else None


def _strict_report_int(value: Any) -> int | None:
    if isinstance(value, bool) or not isinstance(value, int):
        return None
    return value


def _validate_performance_structure(
    mode: str,
    performance: dict[str, Any],
    transition: dict[str, Any],
) -> list[dict[str, Any]]:
    cadence = performance.get("sampleCadence")
    sampled_frames = performance.get("sampledFrames")
    state_timeline = performance.get("stateTimeline")
    motion_events = performance.get("motionEvents")
    evidence = transition.get("evidence")
    if not all(
        isinstance(item, dict)
        for item in (cadence, transition, evidence)
    ) or not all(
        isinstance(item, list)
        for item in (sampled_frames, state_timeline, motion_events)
    ):
        raise DemoQAError(f"{mode} transition geometry QA did not pass")
    assert isinstance(cadence, dict)
    assert isinstance(evidence, dict)
    assert isinstance(sampled_frames, list)
    assert isinstance(state_timeline, list)
    assert isinstance(motion_events, list)

    integer_fields = (
        "fps",
        "frameStart",
        "frameEnd",
        "animationFrameCount",
        "performanceSampleIntervalFrames",
        "performanceSampleCount",
        "contactSampleIntervalFrames",
        "contactSampleCount",
    )
    values = {name: _strict_report_int(cadence.get(name)) for name in integer_fields}
    if any(value is None for value in values.values()):
        raise DemoQAError(f"{mode} transition geometry QA did not pass")
    fps = values["fps"]
    frame_start = values["frameStart"]
    frame_end = values["frameEnd"]
    frame_count = values["animationFrameCount"]
    assert fps is not None and frame_start is not None and frame_end is not None
    assert frame_count is not None
    expected_frames = list(range(frame_start, frame_end + 1, 2))
    if (
        fps <= 0
        or frame_start <= 0
        or frame_end < frame_start
        or frame_count != frame_end - frame_start + 1
        or values["performanceSampleIntervalFrames"] != 2
        or values["performanceSampleCount"] != len(expected_frames)
        or values["performanceSampleCount"] != len(sampled_frames)
        or values["contactSampleIntervalFrames"] != 1
    ):
        raise DemoQAError(f"{mode} transition geometry QA did not pass")
    for name in integer_fields:
        if evidence.get(name) != cadence.get(name):
            raise DemoQAError(f"{mode} transition geometry QA did not pass")

    timeline: list[tuple[float, str]] = []
    for item in state_timeline:
        if not isinstance(item, dict):
            raise DemoQAError(f"{mode} transition geometry QA did not pass")
        time_sec = _strict_report_number(item.get("timeSec"))
        state = item.get("state")
        if time_sec is None or state not in {"standing", "seated"}:
            raise DemoQAError(f"{mode} transition geometry QA did not pass")
        timeline.append((time_sec, str(state)))
    if (
        not timeline
        or timeline[0][0] != 0.0
        or any(after[0] < before[0] for before, after in zip(timeline, timeline[1:]))
    ):
        raise DemoQAError(f"{mode} transition geometry QA did not pass")

    ordered_events: list[tuple[float, float, dict[str, Any]]] = []
    for event in motion_events:
        if not isinstance(event, dict):
            raise DemoQAError(f"{mode} transition geometry QA did not pass")
        if event.get("motion") != "avatar_action":
            continue
        time_sec = _strict_report_number(event.get("timeSec"))
        duration = _strict_report_number(event.get("duration"))
        if time_sec is None or duration is None or duration < 0.0:
            raise DemoQAError(f"{mode} transition geometry QA did not pass")
        ordered_events.append((time_sec, duration, event))
    ordered_events.sort(key=lambda item: item[0])
    expected_timeline = [(0.0, timeline[0][1])]
    state = timeline[0][1]
    for time_sec, duration, event in ordered_events:
        start_state = event.get("startState")
        end_state = event.get("endState")
        if start_state != state or end_state not in {"standing", "seated"}:
            raise DemoQAError(f"{mode} transition geometry QA did not pass")
        state = str(end_state)
        expected_timeline.append((time_sec + duration, state))
    if timeline != expected_timeline:
        raise DemoQAError(f"{mode} transition geometry QA did not pass")

    for expected_frame, sample in zip(expected_frames, sampled_frames):
        if not isinstance(sample, dict):
            raise DemoQAError(f"{mode} transition geometry QA did not pass")
        frame = _strict_report_int(sample.get("frame"))
        time_sec = _strict_report_number(sample.get("timeSec"))
        expected_time = round((expected_frame - frame_start) / float(fps), 6)
        if frame != expected_frame or time_sec != expected_time:
            raise DemoQAError(f"{mode} transition geometry QA did not pass")
        expected_state = max(
            (item for item in timeline if item[0] <= expected_time),
            key=lambda item: item[0],
        )[1]
        if sample.get("state") != expected_state:
            raise DemoQAError(f"{mode} transition geometry QA did not pass")

    physical_events = [dict(event) for event in aroll_performance_qa.physical_transition_events(motion_events)]
    physical_actions = [str(event.get("action") or "") for event in physical_events]
    if evidence.get("physicalTransitionActions") != physical_actions:
        raise DemoQAError(f"{mode} transition geometry QA did not pass")
    expected_contact_count = frame_count if physical_events else 0
    if values["contactSampleCount"] != expected_contact_count:
        raise DemoQAError(f"{mode} transition geometry QA did not pass")
    return physical_events


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

    performance = payload.get("arollPerformanceQa")
    if (
        not isinstance(performance, dict)
        or performance.get("schemaVersion") != "tangying-aroll-performance-qa/v1"
    ):
        raise DemoQAError(f"{mode} transition geometry QA did not pass")
    transition = performance.get("transition")
    if not isinstance(transition, dict):
        raise DemoQAError(f"{mode} transition geometry QA did not pass")
    physical_events = _validate_performance_structure(mode, performance, transition)
    if physical_events:
        recomputed_transition = aroll_performance_qa.validate_transition_metrics(
            transition.get("metrics")
        )
        if (
            recomputed_transition.get("success") is not True
            or transition.get("status") != recomputed_transition.get("status")
            or transition.get("success") is not recomputed_transition.get("success")
            or transition.get("errors") != recomputed_transition.get("errors")
            or transition.get("metrics") != recomputed_transition.get("metrics")
        ):
            raise DemoQAError(f"{mode} transition geometry QA did not pass")
    elif (
        transition.get("status") != "not_applicable"
        or transition.get("success") is not None
        or transition.get("errors") != []
        or transition.get("metrics") != {}
        or len({item.get("state") for item in performance["stateTimeline"]}) != 1
    ):
        raise DemoQAError(f"{mode} transition geometry QA did not pass")
    visemes = performance.get("visemes")
    if not isinstance(visemes, dict):
        raise DemoQAError(f"{mode} viseme QA did not pass")
    dimensions = visemes.get("characterDimensions")
    if not isinstance(dimensions, dict):
        raise DemoQAError(f"{mode} viseme QA did not pass")
    recomputed_visemes = aroll_performance_qa.validate_viseme_metrics(
        visemes.get("metrics"),
        character_height=dimensions.get("height"),
        character_width=dimensions.get("width"),
    )
    viseme_evidence = visemes.get("evidence")
    jaw_timeline = (
        viseme_evidence.get("evaluatedJawTimeline")
        if isinstance(viseme_evidence, dict)
        else None
    )
    jaw_samples = jaw_timeline.get("samples") if isinstance(jaw_timeline, dict) else None
    recomputed_jaw = aroll_performance_qa.summarize_jaw_samples(jaw_samples)
    cadence = performance["sampleCadence"]
    expected_jaw_frames = list(
        range(int(cadence["frameStart"]), int(cadence["frameEnd"]) + 1)
    )
    jaw_summary_fields = (
        "success",
        "errors",
        "maxJawRadians",
        "mbpJawRadians",
        "sampleCount",
        "mbpSampleCount",
        "sampledFrames",
    )
    if (
        recomputed_visemes.get("success") is not True
        or visemes.get("status") != recomputed_visemes.get("status")
        or visemes.get("success") is not recomputed_visemes.get("success")
        or visemes.get("errors") != recomputed_visemes.get("errors")
        or visemes.get("metrics") != recomputed_visemes.get("metrics")
        or dimensions != recomputed_visemes.get("characterDimensions")
        or not isinstance(jaw_timeline, dict)
        or recomputed_jaw.get("success") is not True
        or any(
            jaw_timeline.get(name) != recomputed_jaw.get(name)
            for name in jaw_summary_fields
        )
        or recomputed_jaw.get("sampledFrames") != expected_jaw_frames
        or recomputed_jaw.get("sampleCount") != cadence["animationFrameCount"]
        or recomputed_jaw.get("maxJawRadians")
        != recomputed_visemes["metrics"].get("maxJawRadians")
        or recomputed_jaw.get("mbpJawRadians")
        != recomputed_visemes["metrics"].get("mbpJawRadians")
    ):
        raise DemoQAError(f"{mode} viseme QA did not pass")
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


def _validate_published_video(key: str, probe: dict[str, Any]) -> None:
    errors: list[str] = []
    if not probe.get("videoExists"):
        errors.append("video is missing")
    if (probe.get("width"), probe.get("height")) != (1920, 1080):
        errors.append("video must be 1920x1080")
    if not math.isclose(float(probe.get("fps") or 0.0), 30.0, abs_tol=0.01):
        errors.append("video must be 30 fps")
    if probe.get("constantFrameRate") is not True:
        errors.append("video must be CFR")
    duration = float(probe.get("durationSec") or 0.0)
    if not (
        float(DEMO_JOB["minimumDurationSec"])
        <= duration
        <= float(DEMO_JOB["maximumDurationSec"])
    ):
        errors.append("video must be between 15 and 30 seconds")
    if errors:
        raise DemoQAError(f"{key} QA failed: {'; '.join(errors)}")


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
        mode = str(DEMO_JOB["presentationMode"])
        canonical_dir = staging_root / "canonical"
        result = renderer(
            script=str(DEMO_JOB["script"]),
            characterProfilePath=str(profile_path),
            outputDir=str(canonical_dir),
            shotId=str(DEMO_JOB["kind"]),
            presentationMode=mode,
            actionSequence=list(DEMO_JOB["actionSequence"]),
            cameraPreset=str(DEMO_JOB["cameraPreset"]),
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
        performance = collision_report["report"]["arollPerformanceQa"]
        expected_physical = [
            "Aroll_Transition_StandToSit",
            "Aroll_Transition_SitToStand",
        ]
        if (
            performance["transition"].get("status") != "passed"
            or performance["transition"].get("success") is not True
            or performance["transition"].get("evidence", {}).get(
                "physicalTransitionActions"
            )
            != expected_physical
        ):
            raise DemoQAError("canonical render did not validate both physical transitions")
        shutil.copy2(video_path, staged_paths["standSit"])
        records.append(
            {
                "mode": mode,
                "script": DEMO_JOB["script"],
                "result": result,
                "probe": probe,
                "collisionReport": collision_report,
                "publishedStagePath": str(staged_paths["standSit"]),
            }
        )

        artifact_builder(records, staged_paths)
        video_probes: dict[str, dict[str, Any]] = {}
        for key in ("seated", "standSit", "reel", "actionPack"):
            derived_probe = media_probe(staged_paths[key])
            _validate_published_video(key, derived_probe)
            video_probes[key] = derived_probe

        render_config = profile.get("render") or {}
        model_config = profile.get("model") or {}
        scene_path = _asset_path(profile_path, str(render_config.get("sceneBlendPath") or ""))
        master_path = _asset_path(profile_path, str(model_config.get("masterBlendPath") or ""))
        record = records[0]
        render_report_path = Path(
            str(record["result"].get("renderReportPath") or "")
        ).expanduser()
        report: dict[str, Any] = {
            "schemaVersion": "tangying-sloth-warm-studio-action-pack/v1",
            "success": True,
            "kind": DEMO_JOB["kind"],
            "actionCatalogVersion": ACTION_CATALOG_VERSION,
            "script": DEMO_JOB["script"],
            "actionSequence": list(DEMO_JOB["actionSequence"]),
            "presentationMode": DEMO_JOB["presentationMode"],
            "cameraPreset": DEMO_JOB["cameraPreset"],
            "profile": _asset_record(profile_path),
            "character": _asset_record(master_path),
            "studio": _asset_record(scene_path),
            "voicePolicy": {
                "provider": voice.get("provider"),
                "voiceId": voice.get("voiceId"),
                "fallbackPolicy": voice.get("fallbackPolicy"),
                "gptSovitsLocal": voice.get("gptSovitsLocal"),
            },
            "voice": record["result"].get("voice") or {},
            "renderQa": record["result"].get("qa") or {},
            "renderProvenance": {
                "renderReportSha256": (
                    _sha256(render_report_path) if render_report_path.is_file() else ""
                ),
                "canonicalVideoSha256": _sha256(staged_paths["standSit"]),
                "singleBlenderRender": True,
            },
            "stateTimeline": performance["stateTimeline"],
            "transitionQa": performance["transition"],
            "visemeQa": performance["visemes"],
            "collisionReport": collision_report,
            "derivedEvidence": record.get("derivedEvidence") or {},
            "lightingEvidence": lighting_evidence,
            "collisionReportPointer": (
                f"{FINAL_FILENAMES['report']}#/collisionReport"
            ),
        }
        report["videos"] = {
            key: {
                "path": str(output_dir / FINAL_FILENAMES[key]),
                "sha256": _sha256(staged_paths[key]),
                "qa": video_probes[key],
            }
            for key in ("seated", "standSit", "reel", "actionPack")
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
