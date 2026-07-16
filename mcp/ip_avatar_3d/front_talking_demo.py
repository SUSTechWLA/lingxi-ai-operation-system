#!/usr/bin/env python3
"""Deterministic request contract for the formal front-facing AIOS demo."""

from __future__ import annotations

import hashlib
import json
import os
import shutil
import subprocess
import tempfile
from fractions import Fraction
from pathlib import Path
from typing import Any


SIGNED_OFF_STAGING_RELATIVE_PATH = Path(
    "outputs/final/refined-aroll-evidence/production-oral-fixed/ip_layer.mp4"
)
FINAL_VIDEO_RELATIVE_PATH = Path(
    "outputs/final/MainIP_Sloth_Refined_Aroll_Demo_1080p.mp4"
)
SIGNED_OFF_VIDEO_SHA256 = (
    "0ce20701c6df6498ca471d64f6d3dd5d41c385f8c1387e821798a8e82cadd8c1"
)


DEMO_SPEC: dict[str, Any] = {
    "kind": "frontTalkingKnowledgeDemo",
    "topic": "为什么 AI 视频不要每三秒换画面",
    "script": (
        "很多 AI 视频看起来很热闹，却让人记不住观点。问题不是素材不够，"
        "而是没有分清 A-roll 和 B-roll。A-roll 负责建立信任，所以主角要稳定面对镜头；"
        "B-roll 只在数据、案例或界面证据出现时短暂覆盖。画面少一点，信息反而更清楚。"
    ),
    "durationSec": 20.0,
    "resolution": {"width": 1920, "height": 1080},
    "fps": 30,
    "profileId": "talking_head",
    "projectMode": "voice_visual",
    "generationMode": "provider_api",
    "presentationMode": "standing",
    "cameraPreset": "front_talking",
    "actionSequence": [
        "Aroll_Greeting_Wave",
        "Aroll_OpenPalm_Explain",
        "Aroll_KeyPoint_OneFinger",
        "Aroll_Explain_Left",
        "Aroll_Agree_Nod",
        "Aroll_Conclusion_HandsTogether",
    ],
    "brollWindows": [
        {
            "startSec": 11.2,
            "endSec": 14.4,
            "purpose": "A-roll 与 B-roll 分工证据图",
        }
    ],
}


def validate_demo_spec(spec: dict[str, Any]) -> list[str]:
    """Return contract violations without mutating the request."""

    errors: list[str] = []
    duration = float(spec.get("durationSec") or 0.0)
    if not 15.0 <= duration <= 30.0:
        errors.append("durationSec must be between 15 and 30 seconds")
    if spec.get("resolution") != {"width": 1920, "height": 1080}:
        errors.append("resolution must be 1920x1080")
    if int(spec.get("fps") or 0) != 30:
        errors.append("fps must be 30")
    if spec.get("profileId") != "talking_head":
        errors.append("profileId must be talking_head")
    if spec.get("presentationMode") != "standing":
        errors.append("presentationMode must be standing")
    if spec.get("cameraPreset") != "front_talking":
        errors.append("cameraPreset must be front_talking")
    actions = [str(action) for action in spec.get("actionSequence") or []]
    if not actions:
        errors.append("actionSequence must not be empty")
    if any("Transition" in action for action in actions):
        errors.append("front talking demo cannot contain physical transition actions")
    broll_duration = sum(
        max(0.0, float(item.get("endSec") or 0.0) - float(item.get("startSec") or 0.0))
        for item in spec.get("brollWindows") or []
    )
    if duration > 0.0 and broll_duration / duration > 0.20:
        errors.append("B-roll coverage must not exceed 20 percent")
    return errors


def build_agent_run_payload(project_id: str) -> dict[str, Any]:
    """Build the public dynamic-agent request used by Director Studio."""

    errors = validate_demo_spec(DEMO_SPEC)
    if errors:
        raise ValueError("; ".join(errors))
    topic = str(DEMO_SPEC["topic"])
    duration = float(DEMO_SPEC["durationSec"])
    return {
        "message": f"请先判断创作类型，再创作一个{duration:g}秒视频：{topic}",
        "domain": "video_creation",
        "mode": "dynamic_agent",
        "context": {
            "projectId": str(project_id),
            "topic": topic,
            "script": DEMO_SPEC["script"],
            "durationSec": duration,
            "targetDurationSec": duration,
            "videoType": DEMO_SPEC["profileId"],
            "profileId": DEMO_SPEC["profileId"],
            "projectMode": DEMO_SPEC["projectMode"],
            "generationMode": DEMO_SPEC["generationMode"],
            "aigcProvider": "disabled",
            "aspectRatio": "16:9",
            "language": "zh-CN",
            "presentationMode": DEMO_SPEC["presentationMode"],
            "cameraPreset": DEMO_SPEC["cameraPreset"],
            "actionSequence": list(DEMO_SPEC["actionSequence"]),
            "brollWindows": list(DEMO_SPEC["brollWindows"]),
        },
    }


def build_project_payload(local_path_hint: str) -> dict[str, Any]:
    """Build the Director Studio project shell for the formal demo."""

    return {
        "name": str(DEMO_SPEC["topic"]),
        "description": "正面知识分享 IP 口播 AIOS 端到端验收",
        "mode": DEMO_SPEC["projectMode"],
        "skillName": "video-creator",
        "skillVersion": "v4.0",
        "workflowName": "dynamic-agent-video-creation",
        "workflowVersion": "v4.0",
        "generationMode": DEMO_SPEC["generationMode"],
        "aspectRatio": "16:9",
        "targetDurationSec": int(DEMO_SPEC["durationSec"]),
        "language": "zh-CN",
        "localPathHint": str(local_path_hint),
        "config": {
            "entry": "front_talking_aios_demo",
            "topic": DEMO_SPEC["topic"],
            "profileId": DEMO_SPEC["profileId"],
            "cameraPreset": DEMO_SPEC["cameraPreset"],
            "presentationMode": DEMO_SPEC["presentationMode"],
        },
    }


def _frame_rate(value: Any) -> float:
    try:
        return float(Fraction(str(value)))
    except (ValueError, ZeroDivisionError):
        return 0.0


def _sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def publish_signed_off_video(repo_root: str | Path) -> dict[str, Any]:
    """Atomically publish the exact corrected render approved for Task 4."""

    root = Path(repo_root).expanduser().resolve()
    staging = root / SIGNED_OFF_STAGING_RELATIVE_PATH
    final = root / FINAL_VIDEO_RELATIVE_PATH
    if not staging.is_file():
        raise RuntimeError(f"signed-off staging video is missing: {staging}")

    staging_sha256 = _sha256_file(staging)
    if staging_sha256 != SIGNED_OFF_VIDEO_SHA256:
        raise RuntimeError(
            "signed-off staging video SHA-256 mismatch: "
            f"expected {SIGNED_OFF_VIDEO_SHA256}, got {staging_sha256}"
        )

    final.parent.mkdir(parents=True, exist_ok=True)
    temporary: Path | None = None
    try:
        descriptor, temporary_name = tempfile.mkstemp(
            prefix=f".{final.name}.",
            suffix=".tmp",
            dir=final.parent,
        )
        temporary = Path(temporary_name)
        with os.fdopen(descriptor, "wb") as target, staging.open("rb") as source:
            shutil.copyfileobj(source, target)
            os.fchmod(target.fileno(), staging.stat().st_mode & 0o777)
            target.flush()
            os.fsync(target.fileno())
        os.replace(temporary, final)
        temporary = None
    finally:
        if temporary is not None:
            temporary.unlink(missing_ok=True)

    final_sha256 = _sha256_file(final)
    if final_sha256 != SIGNED_OFF_VIDEO_SHA256:  # pragma: no cover - atomic copy guard
        raise RuntimeError("published final video SHA-256 does not match signed-off staging")
    return {
        "published": True,
        "stagingPath": str(staging),
        "finalPath": str(final),
        "sha256": final_sha256,
        "sizeBytes": final.stat().st_size,
    }


def validate_media_probe(probe: dict[str, Any]) -> dict[str, Any]:
    """Validate an ffprobe payload against the front-talking delivery contract."""

    streams = probe.get("streams") or []
    video = next(
        (stream for stream in streams if stream.get("codec_type") == "video"),
        {},
    )
    audio = next(
        (stream for stream in streams if stream.get("codec_type") == "audio"),
        {},
    )
    duration = float((probe.get("format") or {}).get("duration") or 0.0)
    nominal_fps = _frame_rate(video.get("r_frame_rate"))
    average_fps = _frame_rate(video.get("avg_frame_rate"))
    constant_frame_rate = abs(nominal_fps - average_fps) <= 0.001

    errors: list[str] = []
    if not 15.0 <= duration <= 30.0:
        errors.append("duration must be between 15 and 30 seconds")
    if (video.get("width"), video.get("height")) != (1920, 1080):
        errors.append("video must be 1920x1080")
    if video.get("codec_name") != "h264":
        errors.append("video codec must be H.264")
    if abs(average_fps - 30.0) > 0.001 or not constant_frame_rate:
        errors.append("video must use constant 30 fps")
    if audio.get("codec_name") != "aac":
        errors.append("audio codec must be AAC")
    if int(audio.get("sample_rate") or 0) != 48000:
        errors.append("audio sample rate must be 48 kHz")

    return {
        "passed": not errors,
        "errors": errors,
        "metrics": {
            "durationSec": duration,
            "width": int(video.get("width") or 0),
            "height": int(video.get("height") or 0),
            "videoCodec": str(video.get("codec_name") or ""),
            "audioCodec": str(audio.get("codec_name") or ""),
            "fps": average_fps,
            "constantFrameRate": constant_frame_rate,
            "audioSampleRate": int(audio.get("sample_rate") or 0),
            "audioChannels": int(audio.get("channels") or 0),
        },
    }


def probe_media(path: str | Path, ffprobe_bin: str = "ffprobe") -> dict[str, Any]:
    """Run ffprobe and return a deterministic delivery QA report."""

    media_path = Path(path).expanduser().resolve()
    completed = subprocess.run(
        [
            ffprobe_bin,
            "-v",
            "error",
            "-show_entries",
            (
                "format=duration,size,bit_rate:"
                "stream=index,codec_type,codec_name,width,height,r_frame_rate,"
                "avg_frame_rate,pix_fmt,sample_rate,channels"
            ),
            "-of",
            "json",
            str(media_path),
        ],
        check=True,
        capture_output=True,
        text=True,
    )
    report = validate_media_probe(json.loads(completed.stdout))
    report["path"] = str(media_path)
    return report
