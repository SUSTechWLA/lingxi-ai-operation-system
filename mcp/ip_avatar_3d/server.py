#!/usr/bin/env python3
"""MCP server for local 3D IP talking-avatar layer rendering.

The main Tangying system should treat this as an external MCP provider. This
server owns GLB loading, narration timing, motion planning, Blender rendering,
and FFmpeg composition, then returns a playable local video path.
"""

from __future__ import annotations

import hashlib
import json
import math
import os
import re
import shutil
import struct
import subprocess
import sys
import uuid
from pathlib import Path
from typing import Any

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

from rig_semantics import has_presenter_controls, resolve_bone_roles
from master_asset import MASTER_COLLECTION, MASTER_VERSION

try:
    from mcp.server.fastmcp import FastMCP
except ImportError:  # pragma: no cover - tests can import pure helper logic.
    FastMCP = None  # type: ignore[assignment]


class _FallbackMCP:
    def __init__(self, _name: str) -> None:
        pass

    def tool(self, *args: Any, **kwargs: Any) -> Any:
        def decorator(func: Any) -> Any:
            return func

        return decorator

    def run(self) -> None:
        raise RuntimeError(
            "Missing Python MCP SDK. Install with: python3 -m pip install -r mcp/ip_avatar_3d/requirements.txt"
        )


mcp = FastMCP("IP Avatar 3D MCP") if FastMCP else _FallbackMCP("IP Avatar 3D MCP")

DEFAULT_WIDTH = 2560
DEFAULT_HEIGHT = 1440
DEFAULT_FPS = 30
DEFAULT_DURATION_SEC = 6.0
DEFAULT_CAMERA_PRESET = "medium"
DEFAULT_LIGHTING_PRESET = "editorial_soft"
DEFAULT_RENDER_ENGINE = "BLENDER_EEVEE_NEXT"
DEFAULT_QUALITY_PRESET = "production_2k"
CAMERA_PRESETS = {"auto", "wide", "medium", "close"}
LIGHTING_PRESETS = {"editorial_soft", "editorial_crisp", "night_analysis", "scene_default"}
RENDER_ENGINES = {"BLENDER_EEVEE_NEXT", "CYCLES"}
QUALITY_PRESETS = {
    "preview": {"eeveeSamples": 32, "cyclesSamples": 32, "videoCrf": 22, "videoPreset": "medium"},
    "production_2k": {"eeveeSamples": 64, "cyclesSamples": 96, "videoCrf": 16, "videoPreset": "slow"},
    "master": {"eeveeSamples": 256, "cyclesSamples": 192, "videoCrf": 12, "videoPreset": "slow"},
}


def _repo_root() -> Path:
    return Path(__file__).resolve().parents[2]


def _now_id() -> str:
    return uuid.uuid4().hex[:12]


def _write_json(path: Path, data: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8")


def _readable_path(raw: str, *, base_dir: str = "") -> Path:
    raw = str(raw or "").strip()
    if not raw:
        raise ValueError("path is required")
    if raw.startswith("local://"):
        mapped = _local_ref_to_path(raw)
        if mapped:
            return mapped
        raise ValueError(f"cannot resolve local ref without TANGYING_DATA_DIR: {raw}")
    path = Path(raw).expanduser()
    if not path.is_absolute():
        root = Path(base_dir).expanduser() if base_dir else Path.cwd()
        path = root / path
    return path.resolve()


def _load_character_profile(raw: str) -> tuple[Path, dict[str, Any]]:
    path = _readable_path(raw)
    if not path.is_file():
        raise FileNotFoundError(f"characterProfilePath does not exist: {path}")
    profile = json.loads(path.read_text(encoding="utf-8"))
    if profile.get("schemaVersion") != "tangying-ip-character/v1":
        raise ValueError("characterProfilePath must use schemaVersion tangying-ip-character/v1")
    return path, profile


def _profile_asset(profile_path: Path, raw: Any) -> str:
    if not raw:
        return ""
    candidate = Path(str(raw)).expanduser()
    if not candidate.is_absolute():
        candidate = profile_path.parent / candidate
    return str(candidate.resolve())


def _read_gltf_document(path: Path) -> dict[str, Any]:
    if path.suffix.lower() == ".gltf":
        return json.loads(path.read_text(encoding="utf-8"))
    with path.open("rb") as handle:
        magic, version, _total_length = struct.unpack("<4sII", handle.read(12))
        if magic != b"glTF" or version != 2:
            raise ValueError("modelPath is not a glTF 2.0 GLB")
        chunk_length, chunk_type = struct.unpack("<I4s", handle.read(8))
        if chunk_type != b"JSON":
            raise ValueError("GLB first chunk is not JSON")
        return json.loads(handle.read(chunk_length).decode("utf-8").rstrip(" \0"))


def _local_ref_to_path(ref: str) -> Path | None:
    data_dir = os.environ.get("TANGYING_DATA_DIR", "").strip()
    if not data_dir:
        return None
    match = re.match(r"^local://projects/([^/]+)/artifacts/([^/]+)/", ref)
    if not match:
        return None
    project_id, artifact_id = match.group(1), match.group(2)
    return Path(data_dir).expanduser().resolve() / "artifacts" / project_id / artifact_id / "content"


def _tool_path(env_names: list[str], fallback_names: list[str]) -> str:
    for name in env_names:
        value = os.environ.get(name, "").strip()
        if value and Path(value).exists():
            return value
    for value in fallback_names:
        if Path(value).exists():
            return value
    for value in fallback_names:
        found = shutil.which(value)
        if found:
            return found
    return ""


def find_blender() -> str:
    return _tool_path(
        ["TANGYING_BLENDER_BIN", "BLENDER_BIN"],
        [
            "/Applications/Blender.app/Contents/MacOS/Blender",
            "blender",
        ],
    )


def find_ffmpeg() -> str:
    return _tool_path(["TANGYING_FFMPEG_BIN", "FFMPEG_BIN"], ["ffmpeg"])


def find_ffprobe() -> str:
    return _tool_path(["TANGYING_FFPROBE_BIN", "FFPROBE_BIN"], ["ffprobe"])


def find_audio_engine() -> str:
    configured = os.environ.get("TANGYING_AUDIO_ENGINE", "").strip()
    candidates = [
        configured,
        str(Path.home() / ".agents/skills/hyperframes-media/scripts/audio.mjs"),
        str(Path.home() / ".claude/skills/hyperframes-media/scripts/audio.mjs"),
    ]
    return next((value for value in candidates if value and Path(value).is_file()), "")


def _run(args: list[str], timeout: int = 600) -> subprocess.CompletedProcess[str]:
    completed = subprocess.run(
        args,
        check=False,
        text=True,
        encoding="utf-8",
        errors="replace",
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=timeout,
    )
    if completed.returncode != 0:
        detail = completed.stderr.strip() or completed.stdout.strip() or f"exit {completed.returncode}"
        raise RuntimeError(detail)
    return completed


def estimate_blender_timeout(duration_sec: float, fps: int, width: int, height: int) -> int:
    """Budget Blender time from frame count and pixel load, with a preview floor."""
    duration = max(0.1, float(duration_sec or DEFAULT_DURATION_SEC))
    pixel_scale = max(1.0, (max(1, int(width)) * max(1, int(height))) / (640 * 360))
    frame_rate_scale = max(1.0, max(1, int(fps)) / 12.0)
    return max(600, int(math.ceil(duration * 15.0 * pixel_scale * frame_rate_scale)))


def estimate_duration(script: str) -> float:
    text = re.sub(r"\s+", "", script or "")
    if not text:
        return DEFAULT_DURATION_SEC
    chinese_or_word_count = len(re.findall(r"[\u4e00-\u9fff]|[A-Za-z0-9]+", text))
    punctuation_count = len(re.findall(r"[。！？；.!?;]", script or ""))
    duration = chinese_or_word_count / 4.2 + punctuation_count * 0.25
    return float(min(max(duration, 4.0), 90.0))


def audio_duration_sec(audio_path: str) -> float:
    if not audio_path:
        return 0.0
    ffprobe = find_ffprobe()
    if not ffprobe:
        return 0.0
    completed = _run(
        [
            ffprobe,
            "-v",
            "error",
            "-show_entries",
            "format=duration",
            "-of",
            "default=noprint_wrappers=1:nokey=1",
            audio_path,
        ],
        timeout=30,
    )
    try:
        return float(completed.stdout.strip())
    except ValueError:
        return 0.0


def _srt_time(seconds: float) -> str:
    seconds = max(0.0, seconds)
    millis = int(round((seconds - math.floor(seconds)) * 1000))
    whole = int(math.floor(seconds))
    h = whole // 3600
    m = (whole % 3600) // 60
    s = whole % 60
    return f"{h:02d}:{m:02d}:{s:02d},{millis:03d}"


def split_script(script: str) -> list[str]:
    items = [item.strip() for item in re.findall(r"[^。！？；.!?;]+[。！？；.!?;]?", script or "") if item.strip()]
    return items or ([script.strip()] if script.strip() else [""])


def build_subtitle_text(script: str, duration_sec: float) -> str:
    segments = split_script(script)
    weights = [max(1, len(re.sub(r"\s+", "", segment))) for segment in segments]
    total = sum(weights) or 1
    cursor = 0.0
    rows: list[str] = []
    for idx, segment in enumerate(segments, start=1):
        span = duration_sec * weights[idx - 1] / total
        if idx == len(segments):
            end = duration_sec
        else:
            end = min(duration_sec, cursor + max(0.8, span))
        rows.append(f"{idx}\n{_srt_time(cursor)} --> {_srt_time(end)}\n{segment}\n")
        cursor = end
    return "\n".join(rows)


def _sentence_boundary_times(script: str, duration_sec: float) -> list[float]:
    text = script or ""
    boundaries = [match.end() for match in re.finditer(r"[。！？；.!?;]", text)]
    text_len = max(1, len(text))
    return [min(duration_sec - 0.1, max(0.1, duration_sec * boundary / text_len)) for boundary in boundaries]


def _keyword_events(script: str, duration_sec: float) -> list[dict[str, Any]]:
    table = [
        {
            "pattern": r"大家好|你好|您好|欢迎|\bhello\b|\bhi\b|\bwelcome\b|\bgreetings\b",
            "motion": "wave",
            "strength": 1.0,
            "action": "Aroll_Greeting_Wave",
            "semantic": "greeting",
            "gestureGroup": "right_hand",
            "duration": 0.9,
        },
        {
            "pattern": r"第一|首先|第一个|\bfirst\b|\bfirstly\b",
            "motion": "point_left",
            "strength": 0.9,
            "action": "Aroll_Count_One",
            "semantic": "enumeration",
            "gestureGroup": "right_hand",
        },
        {
            "pattern": r"第二|其次|第二个|\bsecond\b|\bsecondly\b",
            "motion": "point_right",
            "strength": 0.9,
            "action": "Aroll_Count_Two",
            "semantic": "enumeration",
            "gestureGroup": "right_hand",
        },
        {
            "pattern": r"第三|第三个|\bthird\b|\bthirdly\b",
            "motion": "point_left",
            "strength": 0.85,
            "action": "Aroll_Count_Three",
            "semantic": "enumeration",
            "gestureGroup": "right_hand",
        },
        {
            "pattern": r"不同意|不赞成|反对|不对|摇头|但是|不过|\bdisagree\b|\bdo not agree\b|\bdon't agree\b|\bhowever\b|\bbut\b",
            "motion": "head_shake",
            "strength": 0.9,
            "action": "Aroll_Disagree_Shake",
            "semantic": "disagreement",
            "gestureGroup": "head",
        },
        {
            "pattern": r"(?<!不)同意|赞成|没错|是的|点头|当然|\bagree\b|\byes\b|\bcorrect\b|\bexactly\b",
            "motion": "nod",
            "strength": 0.9,
            "action": "Aroll_Agree_Nod",
            "semantic": "agreement",
            "gestureGroup": "head",
        },
        {
            "pattern": r"解释|说明|介绍|讲解|展开|\bexplain\b|\bexplanation\b|\bdescribe\b|\bintroduce\b|\bwalk through\b",
            "motion": "present",
            "strength": 0.82,
            "action": "Aroll_OpenPalm_Explain",
            "semantic": "explanation",
            "gestureGroup": "right_hand",
            "gestureGroups": ["right_hand", "left_hand"],
            "duration": 0.85,
        },
        {
            "pattern": r"细节|具体|详细|微小|小处|\bdetail\b|\bdetails\b|\bspecific\b|\bprecise\b|\bsmall detail\b",
            "motion": "finger_wave",
            "strength": 0.82,
            "action": "Aroll_Pinch_Detail",
            "semantic": "detail",
            "gestureGroup": "right_hand",
            "duration": 0.78,
        },
        {
            "pattern": r"重点|注意|结论|总之|最后|记住|关键|\bemphasize\b|\bemphasis\b|\bin conclusion\b|\bconclusion\b|\bkey point\b|\bremember\b",
            "motion": "emphasis",
            "strength": 1.0,
            "action": "Aroll_Emphasis_SoftFist",
            "semantic": "emphasis",
            "gestureGroup": "right_hand",
            "duration": 0.72,
        },
        {
            "pattern": r"分析|思考|判断|考虑|\bthink\b|\banalyze\b|\bconsider\b",
            "motion": "think",
            "strength": 0.82,
            "action": "Aroll_Think",
            "semantic": "thinking",
            "gestureGroup": "right_hand",
        },
        {
            "pattern": r"左边|\bleft\b",
            "motion": "point_left",
            "strength": 0.9,
            "action": "Aroll_Point_Left",
            "semantic": "pointing",
            "gestureGroup": "left_hand",
        },
        {
            "pattern": r"右边|\bright\b",
            "motion": "point_right",
            "strength": 0.9,
            "action": "Aroll_Point_Right",
            "semantic": "pointing",
            "gestureGroup": "right_hand",
        },
        {"pattern": r"挥手", "motion": "wave", "strength": 1.0, "gestureGroup": "right_hand"},
        {"pattern": r"张开手|摊开手", "motion": "open_hand", "strength": 0.78, "gestureGroup": "right_hand"},
        {"pattern": r"握拳", "motion": "fist", "strength": 0.72, "gestureGroup": "right_hand"},
        {"pattern": r"转动手腕|手腕", "motion": "wrist_twist", "strength": 0.68, "gestureGroup": "right_hand"},
        {"pattern": r"动动手指|手指", "motion": "finger_wave", "strength": 0.68, "gestureGroup": "right_hand"},
        {"pattern": r"抬手", "motion": "present", "strength": 0.8, "gestureGroup": "body"},
        {"pattern": r"腿", "motion": "leg_step", "strength": 0.75, "gestureGroup": "body"},
        {"pattern": r"大家|全面", "motion": "open_arms", "strength": 0.72, "gestureGroup": "body"},
        {"pattern": r"也许|可能|不确定", "motion": "shrug", "strength": 0.78, "gestureGroup": "body"},
        {"pattern": r"所以", "motion": "nod", "strength": 0.8, "gestureGroup": "head"},
        {"pattern": r"开源|流程", "motion": "present", "strength": 0.75, "gestureGroup": "body"},
        {"pattern": r"视频", "motion": "wave", "strength": 0.7, "gestureGroup": "right_hand"},
        {"pattern": r"创作", "motion": "happy_bounce", "strength": 0.8, "gestureGroup": "body"},
    ]
    events: list[dict[str, Any]] = []
    text_len = max(1, len(script or ""))
    claimed_spans: list[tuple[int, int]] = []
    for rule in table:
        for match in re.finditer(str(rule["pattern"]), script or "", flags=re.IGNORECASE):
            span = match.span()
            if any(span[0] < end and start < span[1] for start, end in claimed_spans):
                continue
            claimed_spans.append(span)
            index = span[0]
            t = min(max(0.0, duration_sec - 0.2), max(0.2, duration_sec * index / text_len))
            event = {
                "timeSec": round(t, 3),
                "motion": str(rule["motion"]),
                "duration": float(rule.get("duration", 0.75)),
                "strength": float(rule["strength"]),
            }
            gesture_group = rule.get("gestureGroup") or _gesture_group_for_motion(str(rule["motion"]))
            if gesture_group:
                event["gestureGroup"] = str(gesture_group)
            if rule.get("action"):
                semantic = str(rule.get("semantic") or "")
                event.update(
                    {
                        "action": str(rule["action"]),
                        "semantic": semantic,
                        "eventName": f"aroll.{semantic}" if semantic else str(rule["action"]),
                    }
                )
                if rule.get("gestureGroups"):
                    event["gestureGroups"] = list(rule["gestureGroups"])
            events.append(event)
    coalesced: list[dict[str, Any]] = []
    latest_by_motion: dict[str, dict[str, Any]] = {}
    for event in sorted(events, key=_motion_event_sort_key):
        previous = latest_by_motion.get(str(event.get("action") or event["motion"]))
        if previous and float(event["timeSec"]) - float(previous["timeSec"]) < 0.85:
            previous["strength"] = min(1.0, max(float(previous["strength"]), float(event["strength"])))
            previous["duration"] = max(float(previous["duration"]), float(event["duration"]))
            continue
        coalesced.append(event)
        latest_by_motion[str(event.get("action") or event["motion"])] = event
    return coalesced


MOTION_GESTURE_GROUPS: dict[str, str] = {
    "weight_shift": "body",
    "happy_bounce": "body",
    "leg_step": "body",
    "open_arms": "body",
    "present": "body",
    "shrug": "body",
    "wave": "right_hand",
    "point": "left_hand",
    "point_left": "left_hand",
    "point_right": "right_hand",
    "open_hand": "right_hand",
    "fist": "right_hand",
    "wrist_twist": "right_hand",
    "finger_wave": "right_hand",
    "think": "right_hand",
    "emphasis": "right_hand",
    "nod": "head",
    "head_shake": "head",
}


def _gesture_group_for_motion(motion: str) -> str | None:
    return MOTION_GESTURE_GROUPS.get(str(motion))


def _motion_event_sort_key(event: dict[str, Any]) -> tuple[float, str, str, str]:
    return (
        float(event.get("timeSec") or 0.0),
        str(event.get("gestureGroup") or _gesture_group_for_motion(str(event.get("motion") or "")) or ""),
        str(event.get("action") or ""),
        str(event.get("motion") or ""),
    )


def _finalize_motion_events(events: list[dict[str, Any]], duration_sec: float) -> list[dict[str, Any]]:
    normalized: list[dict[str, Any]] = []
    for event in events:
        item = dict(event)
        if not item.get("gestureGroup"):
            gesture_group = _gesture_group_for_motion(str(item.get("motion") or ""))
            if gesture_group:
                item["gestureGroup"] = gesture_group
        normalized.append(item)

    latest_end_by_group: dict[str, float] = {}
    resolved: list[dict[str, Any]] = []
    for event in sorted(normalized, key=_motion_event_sort_key):
        item = dict(event)
        if item.get("gestureGroup"):
            groups = [
                str(group)
                for group in item.get("gestureGroups", [item["gestureGroup"]])
                if str(group) in {"right_hand", "left_hand", "head", "body"}
            ] or [str(item["gestureGroup"])]
            duration = max(0.1, float(item.get("duration") or 0.75))
            start = max(0.0, float(item.get("timeSec") or 0.0))
            previous_end = max((latest_end_by_group.get(group, -1.0) for group in groups), default=-1.0)
            if start < previous_end:
                start = previous_end
            if start + duration > duration_sec:
                duration = max(0.1, duration_sec - start)
            item["timeSec"] = round(start, 3)
            item["duration"] = round(duration, 3)
            for group in groups:
                latest_end_by_group[group] = start + duration
        resolved.append(item)
    return sorted(resolved, key=_motion_event_sort_key)


MANDARIN_VISEME_CHARS = {
    "mbp": "不把吧爸八白百本比变别并步部被表面们没每门明名目某旁跑拍片品平",
    "o": "我好说做过国果中重动同手头口后走有都总种从",
    "u": "无五物用如书出路主住注数术需去具据区局",
    "e": "你一第里理力起器给这的也结简见先新今影应因点前天迎析实",
    "a": "啊大家发开来讲想看还改感安判断化",
}


def _viseme_for_char(char: str) -> tuple[str, float]:
    lowered = char.lower()
    if char in "，,。.!！？?；;、 \n\t":
        return "closed", 0.0
    if lowered in "bmp" or char in MANDARIN_VISEME_CHARS["mbp"]:
        return "mbp", 0.05
    if lowered == "o" or char in MANDARIN_VISEME_CHARS["o"]:
        return "o", 0.72
    if lowered == "a" or char in MANDARIN_VISEME_CHARS["a"]:
        return "a", 0.78
    if lowered in "ei" or char in MANDARIN_VISEME_CHARS["e"]:
        return "e", 0.48
    if lowered == "u" or char in MANDARIN_VISEME_CHARS["u"]:
        return "u", 0.42
    return "a", 0.52


def build_motion_plan(script: str, duration_sec: float, fps: int = DEFAULT_FPS, motion_style: str = "expressive") -> dict[str, Any]:
    duration_sec = float(duration_sec or estimate_duration(script))
    fps = max(8, min(int(fps or DEFAULT_FPS), 60))
    strength_scale = 1.0 if motion_style != "subtle" else 0.55
    events: list[dict[str, Any]] = [
        {"timeSec": 0.0, "motion": "idle_breath", "duration": round(duration_sec, 3), "strength": 0.45 * strength_scale}
    ]
    blink_t = 1.15
    blink_intervals = (2.85, 3.55, 3.10, 3.90)
    blink_index = 0
    while blink_t < duration_sec:
        events.append({"timeSec": round(blink_t, 3), "motion": "blink", "duration": 0.16, "strength": 1.0})
        blink_t += blink_intervals[blink_index % len(blink_intervals)]
        blink_index += 1
    gaze_t = 1.85
    gaze_direction = -1
    while gaze_t < duration_sec:
        events.append(
            {
                "timeSec": round(gaze_t, 3),
                "motion": "micro_gaze",
                "duration": 0.72,
                "strength": 0.22 * strength_scale,
                "direction": gaze_direction,
            }
        )
        gaze_direction *= -1
        gaze_t += 2.45 if gaze_direction > 0 else 3.05
    antenna_t = 0.8
    while antenna_t < duration_sec:
        events.append({"timeSec": round(antenna_t, 3), "motion": "antenna_wiggle", "duration": 1.0, "strength": 0.65 * strength_scale})
        antenna_t += 4.0
    boundary_times = _sentence_boundary_times(script, duration_sec)
    for t in boundary_times:
        events.append({"timeSec": round(t, 3), "motion": "nod", "duration": 0.55, "strength": 0.65 * strength_scale})
        events.append(
            {
                "timeSec": round(max(0.1, t - 0.12), 3),
                "motion": "brow_beat",
                "duration": 0.38,
                "strength": 0.38 * strength_scale,
            }
        )
    keyword_events = _keyword_events(script, duration_sec)
    events.extend(keyword_events)
    expressive_motions = {
        "wave", "present", "point", "point_left", "point_right", "open_arms",
        "think", "shrug", "emphasis", "happy_bounce", "leg_step",
        "open_hand", "fist", "wrist_twist", "finger_wave",
    }
    if duration_sec >= 4.0 and not any(event.get("motion") in expressive_motions for event in keyword_events):
        events.extend(
            [
                {
                    "timeSec": round(duration_sec * 0.28, 3),
                    "motion": "present",
                    "duration": 0.95,
                    "strength": 0.48 * strength_scale,
                },
                {
                    "timeSec": round(duration_sec * 0.68, 3),
                    "motion": "emphasis",
                    "duration": 0.72,
                    "strength": 0.52 * strength_scale,
                },
            ]
        )
    if duration_sec >= 4.0:
        events.append(
            {
                "timeSec": 0.18,
                "motion": "weight_shift",
                "duration": 1.15,
                "strength": 0.28 * strength_scale,
                "direction": 1,
            }
        )
    events = _finalize_motion_events(events, duration_sec)

    chars = list(script or "")
    if not chars:
        chars = [" "]
    sample_step = max(1.0 / fps, 0.08)
    lip_sync: list[dict[str, Any]] = []
    t = 0.0
    while t <= duration_sec + 1e-6:
        char = chars[min(len(chars) - 1, int((t / max(duration_sec, 0.001)) * len(chars)))]
        viseme, open_value = _viseme_for_char(char)
        previous = lip_sync[-1]["open"] if lip_sync else open_value
        smoothed = previous * 0.35 + open_value * 0.65
        lip_sync.append({"timeSec": round(t, 3), "viseme": viseme, "open": round(smoothed, 3)})
        t += sample_step

    return {
        "schemaVersion": "ip-avatar-3d-motion-plan/v1",
        "durationSec": round(duration_sec, 3),
        "fps": fps,
        "motionStyle": motion_style,
        "motionEvents": events,
        "lipSync": lip_sync,
        "notes": [
            "This plan is deterministic and local.",
            "Production-quality rigging, face-screen placement, and voice can be replaced inside the MCP provider without changing Tangying core.",
        ],
    }


def build_camera_plan(
    duration_sec: float,
    fps: int = DEFAULT_FPS,
    camera_preset: str = DEFAULT_CAMERA_PRESET,
) -> list[dict[str, Any]]:
    """Build deterministic cuts against the authored studio camera names."""
    preset = str(camera_preset or DEFAULT_CAMERA_PRESET).strip().lower()
    if preset not in CAMERA_PRESETS:
        raise ValueError(f"cameraPreset must be one of: {', '.join(sorted(CAMERA_PRESETS))}")
    fps = max(8, min(int(fps or DEFAULT_FPS), 60))
    duration_sec = max(0.1, float(duration_sec or DEFAULT_DURATION_SEC))
    camera_name = {
        "wide": "Camera_Wide",
        "medium": "Camera_Medium",
        "close": "Camera_Close",
    }
    if preset != "auto":
        return [{"frame": 1, "camera": camera_name.get(preset, "Camera_Medium")}]
    if duration_sec < 4.0:
        return [{"frame": 1, "camera": "Camera_Medium"}]

    final_frame = max(1, int(round(duration_sec * fps)))
    establish_end = min(1.65, max(1.35, duration_sec * 0.22))
    candidates = [
        (1, "Camera_Wide"),
        (min(final_frame, max(2, int(round(establish_end * fps)) + 1)), "Camera_Medium"),
        (min(final_frame, max(2, int(round(duration_sec * 0.58 * fps)) + 1)), "Camera_Close"),
        (min(final_frame, max(2, int(round(duration_sec * 0.78 * fps)) + 1)), "Camera_Medium"),
    ]
    plan_by_frame: dict[int, str] = {}
    for frame, camera in candidates:
        plan_by_frame[frame] = camera
    return [{"frame": frame, "camera": plan_by_frame[frame]} for frame in sorted(plan_by_frame)]


def ensure_audio(
    script: str,
    output_dir: Path,
    duration_sec: float,
    audio_path: str = "",
    voice_name: str = "",
    speaking_rate: int = 190,
    *,
    voice_provider: str = "auto",
    voice_id: str = "",
    voice_language: str = "zh",
    voice_speed: float = 1.0,
) -> tuple[str, str, dict[str, Any]]:
    if audio_path:
        resolved = _readable_path(audio_path)
        if not resolved.exists():
            raise FileNotFoundError(f"audioPath does not exist: {resolved}")
        return str(resolved), "uploaded_audio", {
            "provider": "uploaded",
            "voiceId": "",
            "language": voice_language,
            "speed": voice_speed,
            "humanVoiceProvider": True,
            "productionReady": True,
        }

    output_dir.mkdir(parents=True, exist_ok=True)
    provider = str(voice_provider or "auto").strip().lower()
    if provider not in {"auto", "heygen", "elevenlabs", "kokoro", "apple"}:
        raise ValueError("voiceProvider must be one of: auto, heygen, elevenlabs, kokoro, apple")
    language = str(voice_language or "zh").strip().lower()
    speed = max(0.7, min(float(voice_speed or 1.0), 1.3))
    pinned_voice = str(voice_id or voice_name or "").strip()
    if provider == "apple":
        ffmpeg = find_ffmpeg()
        say = shutil.which("say")
        if not ffmpeg or not say:
            raise RuntimeError("voiceProvider=apple requires macOS say and ffmpeg")
        selected_voice = pinned_voice or "Eddy (中文（中国大陆）)"
        raw_audio = output_dir / "narration_apple.aiff"
        mastered_audio = output_dir / "narration_master.m4a"
        effective_rate = max(145, min(int(round(float(speaking_rate) * speed)), 215))
        _run(
            [
                say,
                "-o",
                str(raw_audio),
                "-r",
                str(effective_rate),
                "-v",
                selected_voice,
                script,
            ],
            timeout=180,
        )
        _run(
            [
                ffmpeg,
                "-y",
                "-i",
                str(raw_audio),
                "-af",
                (
                    "highpass=f=65,lowpass=f=15000,"
                    "equalizer=f=145:t=q:w=0.9:g=1.6,"
                    "equalizer=f=3200:t=q:w=1.2:g=0.8,"
                    "acompressor=threshold=-20dB:ratio=2.0:attack=12:release=160:makeup=1.4,"
                    "loudnorm=I=-16:LRA=8:TP=-1.5"
                ),
                "-ar",
                "48000",
                "-ac",
                "1",
                "-c:a",
                "aac",
                "-b:a",
                "192k",
                str(mastered_audio),
            ],
            timeout=180,
        )
        return str(mastered_audio), "apple_neural_voice", {
            "provider": "apple",
            "voiceId": selected_voice,
            "language": language,
            "speed": speed,
            "speakingRate": effective_rate,
            "humanVoiceProvider": False,
            "naturalVoiceProvider": True,
            "productionReady": True,
            "masteringPreset": "warm_knowledge_host_v1",
        }
    audio_engine = find_audio_engine()
    node = shutil.which("node")
    if audio_engine and node and script.strip():
        request_path = output_dir / "audio_request.json"
        meta_path = output_dir / "audio_meta.json"
        request: dict[str, Any] = {
            "provider": provider,
            "lang": language,
            "speed": speed,
            "lines": [{"id": "narration", "text": script}],
            "bgm": {"mode": "none"},
        }
        if pinned_voice:
            request["voice"] = pinned_voice
        _write_json(request_path, request)
        try:
            _run(
                [
                    node,
                    audio_engine,
                    "--request",
                    str(request_path),
                    "--hyperframes",
                    str(output_dir),
                    "--out",
                    str(meta_path),
                    "--only",
                    "tts",
                ],
                timeout=900,
            )
            meta = json.loads(meta_path.read_text(encoding="utf-8"))
            voices = meta.get("voices") or []
            narration = next((item for item in voices if item.get("id") == "narration"), None)
            if not narration:
                raise RuntimeError("HyperFrames audio engine did not produce narration")
            generated = Path(str(narration.get("path") or ""))
            if not generated.is_absolute():
                generated = output_dir / generated
            generated = generated.resolve()
            if not generated.is_file():
                raise RuntimeError(f"HyperFrames narration file is missing: {generated}")
            actual_provider = str(meta.get("tts_provider") or provider)
            actual_voice = str(meta.get("voice_id") or pinned_voice)
            return str(generated), f"hyperframes_{actual_provider}", {
                "provider": actual_provider,
                "voiceId": actual_voice,
                "language": language,
                "speed": speed,
                "humanVoiceProvider": actual_provider in {"heygen", "elevenlabs"},
                "productionReady": actual_provider in {"heygen", "elevenlabs"},
                "audioMetaPath": str(meta_path),
            }
        except (OSError, RuntimeError, ValueError, json.JSONDecodeError):
            if provider != "auto":
                raise

    ffmpeg = find_ffmpeg()
    if not ffmpeg:
        raise RuntimeError("ffmpeg is required to create fallback narration audio")
    say = shutil.which("say")
    if say and script.strip():
        aiff_path: Path | None = output_dir / "narration_preview.aiff"
        audio_out = output_dir / "narration_preview.m4a"
        say_args = [say, "-o", str(aiff_path), "-r", str(max(120, min(int(speaking_rate), 260)))]
        if voice_name.strip():
            say_args.extend(["-v", voice_name.strip()])
        say_args.append(script)
        try:
            _run(say_args, timeout=180)
        except RuntimeError:
            fallback_args = [say, "-o", str(aiff_path), "-r", str(max(120, min(int(speaking_rate), 260))), script]
            try:
                _run(fallback_args, timeout=180)
            except RuntimeError:
                aiff_path = None
        if aiff_path and aiff_path.exists():
            _run(
                [
                    ffmpeg,
                    "-y",
                    "-i",
                    str(aiff_path),
                    "-af",
                    "loudnorm,acompressor=threshold=-18dB:ratio=2.2:attack=12:release=180",
                    "-c:a",
                    "aac",
                    "-b:a",
                    "128k",
                    str(audio_out),
                ],
                timeout=120,
            )
            return str(audio_out), "local_say_preview", {
                "provider": "macos_say",
                "voiceId": voice_name.strip(),
                "language": language,
                "speed": speed,
                "humanVoiceProvider": False,
                "productionReady": False,
            }

    audio_out = output_dir / "silent_reference.m4a"
    _run(
        [
            ffmpeg,
            "-y",
            "-f",
            "lavfi",
            "-i",
            "anullsrc=channel_layout=stereo:sample_rate=44100",
            "-t",
            f"{duration_sec:.3f}",
            "-c:a",
            "aac",
            "-b:a",
            "96k",
            str(audio_out),
        ],
        timeout=60,
    )
    return str(audio_out), "silent_fallback", {
        "provider": "silent",
        "voiceId": "",
        "language": language,
        "speed": speed,
        "humanVoiceProvider": False,
        "productionReady": False,
    }


def _build_compose_video_args(
    *,
    frames_dir: Path,
    audio_path: str,
    output_path: Path,
    duration_sec: float,
    fps: int,
    background_path: str = "",
    background_brightness: float = 0.88,
    width: int = DEFAULT_WIDTH,
    height: int = DEFAULT_HEIGHT,
    video_crf: int = 16,
    video_preset: str = "slow",
) -> list[str]:
    ffmpeg = find_ffmpeg()
    if not ffmpeg:
        raise RuntimeError("ffmpeg is required to compose IP avatar video")
    pattern = str(frames_dir / "frame_%04d.png")
    args = [ffmpeg, "-y"]
    if background_path:
        args.extend(["-loop", "1", "-framerate", str(fps), "-i", background_path])
    args.extend(["-framerate", str(fps), "-start_number", "1", "-i", pattern])
    if audio_path:
        args.extend(["-i", audio_path])

    frame_input = 1 if background_path else 0
    audio_input = frame_input + 1
    if background_path:
        args.extend(
            [
                "-filter_complex",
                (
                    f"[0:v]scale={width}:{height}:force_original_aspect_ratio=increase,"
                    f"crop={width}:{height},"
                    f"colorchannelmixer=rr={background_brightness:.3f}:gg={background_brightness:.3f}:bb={background_brightness:.3f}[bg];"
                    f"[{frame_input}:v]format=rgba[avatar];"
                    "[bg][avatar]overlay=0:0:format=auto,format=yuv420p[v]"
                ),
                "-map",
                "[v]",
            ]
        )
    else:
        args.extend(["-map", f"{frame_input}:v:0"])
    if audio_path:
        args.extend(
            [
                "-map",
                f"{audio_input}:a:0",
                "-filter:a",
                "loudnorm=I=-16:TP=-1.5:LRA=7",
                "-c:a",
                "aac",
                "-b:a",
                "128k",
                "-ar",
                "48000",
                "-shortest",
            ]
        )
    args.extend(
        [
            "-t",
            f"{duration_sec:.3f}",
            "-c:v",
            "libx264",
            "-crf",
            str(max(0, min(int(video_crf), 51))),
            "-preset",
            str(video_preset),
            "-profile:v",
            "high",
            "-pix_fmt",
            "yuv420p",
            "-movflags",
            "+faststart",
            "-r",
            str(fps),
            "-fps_mode",
            "cfr",
        ]
    )
    args.append(str(output_path))
    return args


def _compose_video(
    frames_dir: Path,
    audio_path: str,
    output_path: Path,
    duration_sec: float,
    fps: int,
    *,
    background_path: str = "",
    background_brightness: float = 0.88,
    width: int = DEFAULT_WIDTH,
    height: int = DEFAULT_HEIGHT,
    video_crf: int = 16,
    video_preset: str = "slow",
) -> None:
    args = _build_compose_video_args(
        frames_dir=frames_dir,
        audio_path=audio_path,
        output_path=output_path,
        duration_sec=duration_sec,
        fps=fps,
        background_path=background_path,
        background_brightness=background_brightness,
        width=width,
        height=height,
        video_crf=video_crf,
        video_preset=video_preset,
    )
    _run(args, timeout=max(120, int(duration_sec * 60)))


def _extract_preview(video_path: Path, preview_path: Path) -> None:
    ffmpeg = find_ffmpeg()
    if not ffmpeg:
        return
    _run(
        [
            ffmpeg,
            "-y",
            "-ss",
            "0.1",
            "-i",
            str(video_path),
            "-frames:v",
            "1",
            str(preview_path),
        ],
        timeout=60,
    )


def _compose_alpha_webm(frames_dir: Path, output_path: Path, duration_sec: float, fps: int) -> None:
    ffmpeg = find_ffmpeg()
    if not ffmpeg:
        return
    pattern = str(frames_dir / "frame_%04d.png")
    _run(
        [
            ffmpeg,
            "-y",
            "-framerate",
            str(fps),
            "-i",
            pattern,
            "-t",
            f"{duration_sec:.3f}",
            "-c:v",
            "libvpx-vp9",
            "-pix_fmt",
            "yuva420p",
            "-auto-alt-ref",
            "0",
            str(output_path),
        ],
        timeout=max(120, int(duration_sec * 80)),
    )


def _probe_video(path: Path) -> dict[str, Any]:
    ffprobe = find_ffprobe()
    if not ffprobe or not path.exists():
        return {"videoExists": path.exists()}
    completed = _run(
        [
            ffprobe,
            "-v",
            "error",
            "-select_streams",
            "v:0",
            "-show_entries",
            "stream=width,height,nb_frames,r_frame_rate,avg_frame_rate:format=duration",
            "-of",
            "json",
            str(path),
        ],
        timeout=30,
    )
    data = json.loads(completed.stdout or "{}")
    stream = (data.get("streams") or [{}])[0]
    duration = float((data.get("format") or {}).get("duration") or 0)
    frame_rate = str(stream.get("avg_frame_rate") or stream.get("r_frame_rate") or "0/1")
    try:
        numerator, denominator = frame_rate.split("/", 1)
        fps = float(numerator) / max(float(denominator), 1.0)
    except (TypeError, ValueError, ZeroDivisionError):
        fps = 0.0
    return {
        "videoExists": path.exists(),
        "durationSec": round(duration, 3),
        "width": int(stream.get("width") or 0),
        "height": int(stream.get("height") or 0),
        "frameCount": int(stream.get("nb_frames") or 0) if str(stream.get("nb_frames") or "").isdigit() else 0,
        "fps": round(fps, 3),
        "constantFrameRate": bool(stream.get("r_frame_rate") == stream.get("avg_frame_rate") and fps > 0),
    }


@mcp.tool()
def check_status() -> dict[str, Any]:
    """Check local dependencies for 3D IP avatar rendering."""
    blender = find_blender()
    ffmpeg = find_ffmpeg()
    ffprobe = find_ffprobe()
    return {
        "available": bool(blender and ffmpeg and ffprobe),
        "provider": "ip_avatar_3d",
        "tools": {
            "blender": blender,
            "ffmpeg": ffmpeg,
            "ffprobe": ffprobe,
        },
        "toolsMissing": [name for name, value in {"blender": blender, "ffmpeg": ffmpeg, "ffprobe": ffprobe}.items() if not value],
    }


@mcp.tool()
def validate_character_asset(modelPath: str = "", characterProfilePath: str = "") -> dict[str, Any]:
    """Validate a GLB/GLTF rig and visemes before a talking-video render."""
    profile_path: Path | None = None
    profile: dict[str, Any] = {}
    if characterProfilePath:
        profile_path, profile = _load_character_profile(characterProfilePath)
        if not modelPath:
            modelPath = _profile_asset(profile_path, (profile.get("model") or {}).get("path"))
    if not modelPath:
        return {
            "status": "model_pending",
            "success": False,
            "readyForTalkingVideo": False,
            "characterProfilePath": str(profile_path) if profile_path else "",
            "reason": "model.path is not configured",
        }

    model_path = _readable_path(modelPath)
    if not model_path.is_file():
        return {
            "status": "model_missing",
            "success": False,
            "readyForTalkingVideo": False,
            "modelPath": str(model_path),
            "reason": "model file does not exist",
        }
    if model_path.suffix.lower() not in {".glb", ".gltf"}:
        raise ValueError("modelPath must point to a GLB or GLTF file")

    document = _read_gltf_document(model_path)
    nodes = document.get("nodes") or []
    skins = document.get("skins") or []
    bone_names = list(
        dict.fromkeys(
            str(nodes[index].get("name") or f"node_{index}")
            for skin in skins
            for index in skin.get("joints") or []
            if isinstance(index, int) and 0 <= index < len(nodes)
        )
    )
    bone_map = resolve_bone_roles(bone_names)
    model_config = profile.get("model") or {}
    required_roles = [str(role) for role in model_config.get("requiredBoneRoles") or []]
    missing_roles = [role for role in required_roles if role not in bone_map]

    shape_keys: list[str] = []
    for mesh in document.get("meshes") or []:
        target_names = (mesh.get("extras") or {}).get("targetNames") or []
        shape_keys.extend(str(name) for name in target_names)
    shape_keys = list(dict.fromkeys(shape_keys))
    facial_config = profile.get("facial") or {}
    required_shape_keys = [str(name) for name in facial_config.get("requiredShapeKeys") or []]
    missing_shape_keys = [name for name in required_shape_keys if name not in shape_keys]

    missing_profile_assets: list[str] = []
    if profile_path:
        referenced_paths = list((profile.get("turnaround") or {}).values())
        render_config = profile.get("render") or {}
        background = render_config.get("backgroundPath")
        if background:
            referenced_paths.append(background)
        scene_blend = render_config.get("sceneBlendPath")
        if scene_blend:
            referenced_paths.append(scene_blend)
        for raw in referenced_paths:
            resolved = Path(_profile_asset(profile_path, raw))
            if not resolved.is_file():
                missing_profile_assets.append(str(resolved))

    presenter_ready = has_presenter_controls(bone_map)
    ready = bool(skins) and presenter_ready and not missing_roles and not missing_shape_keys and not missing_profile_assets
    return {
        "status": "ready" if ready else "partial",
        "success": ready,
        "readyForTalkingVideo": ready,
        "characterId": str(profile.get("characterId") or ""),
        "characterProfilePath": str(profile_path) if profile_path else "",
        "modelPath": str(model_path),
        "hasSkin": bool(skins),
        "skinCount": len(skins),
        "boneCount": len(bone_names),
        "boneMap": bone_map,
        "presenterControlsReady": presenter_ready,
        "missingBoneRoles": missing_roles,
        "shapeKeys": shape_keys,
        "missingShapeKeys": missing_shape_keys,
        "animationCount": len(document.get("animations") or []),
        "missingProfileAssets": missing_profile_assets,
    }


def _master_output_paths(profile_path: Path, model_config: dict[str, Any], output_dir: str) -> dict[str, Path]:
    configured = _profile_asset(profile_path, model_config.get("masterBlendPath"))
    if not configured:
        raise ValueError(f"character profile model.masterBlendPath is required: {profile_path}")
    configured_path = Path(configured)
    if configured_path.suffix.lower() != ".blend":
        raise ValueError("model.masterBlendPath must point to a Blender .blend file")
    root = _readable_path(output_dir) if output_dir else configured_path.parent
    stem = configured_path.stem
    base = stem[: -len("-master")] if stem.endswith("-master") else stem
    return {
        "master": root / configured_path.name,
        "glb": root / f"{base}-rigged.glb",
        "report": root / f"{base}-rig-report.json",
        "qa": root / f"{base}-qa-input.json",
    }


@mcp.tool()
def prepare_character_master(
    sourceModel: str,
    characterProfilePath: str,
    outputDir: str = "",
    qualityTier: str = "aroll_close",
    dryRun: bool = False,
) -> dict[str, Any]:
    """Prepare one stable, versioned character master through Blender's asset-only path."""
    profile_path, profile = _load_character_profile(characterProfilePath)
    model_config = profile.get("model") or {}
    facial_config = profile.get("facial") or {}
    render_config = profile.get("render") or {}
    requested_tier = str(qualityTier or "aroll_close").strip().lower()
    if requested_tier != "aroll_close":
        raise ValueError("qualityTier must be aroll_close")
    configured_tier = str(model_config.get("qualityTier") or requested_tier).strip().lower()
    if configured_tier != requested_tier:
        raise ValueError(
            f"qualityTier={requested_tier} does not match character profile qualityTier={configured_tier}"
        )

    if sourceModel:
        source_path = _readable_path(sourceModel)
    else:
        source_path = Path(
            _profile_asset(profile_path, model_config.get("sourcePath") or model_config.get("path"))
        )
    if not source_path.is_file():
        raise FileNotFoundError(f"sourceModel does not exist: {source_path}")
    if source_path.suffix.lower() not in {".fbx", ".glb", ".gltf"}:
        raise ValueError("sourceModel must point to an FBX, GLB, or GLTF file")

    paths = _master_output_paths(profile_path, model_config, outputDir)
    for path in paths.values():
        path.parent.mkdir(parents=True, exist_ok=True)
    fps = max(8, min(int(render_config.get("fps") or DEFAULT_FPS), 60))
    resolution = render_config.get("resolution") or {}
    width = max(320, min(int(resolution.get("width") or DEFAULT_WIDTH), 3840))
    height = max(180, min(int(resolution.get("height") or DEFAULT_HEIGHT), 2160))
    motion_plan = build_motion_plan("", 1.0, fps, str(render_config.get("motionStyle") or "expressive"))
    qa_input = {
        "schemaVersion": "ip-avatar-3d-master-prepare/v1",
        "characterId": str(profile.get("characterId") or ""),
        "characterProfilePath": str(profile_path),
        "sourceModel": str(source_path),
        "modelPath": str(source_path),
        "qualityTier": requested_tier,
        "masterCollection": MASTER_COLLECTION,
        "masterVersion": MASTER_VERSION,
        "masterBlendPath": str(paths["master"]),
        "useMasterAsset": False,
        "prepareMaster": True,
        "assetOnly": True,
        "riggedBlendPath": str(paths["master"]),
        "riggedGlbPath": str(paths["glb"]),
        "rigReportPath": str(paths["report"]),
        "sceneBlendPath": "",
        "backgroundPath": "",
        "backgroundMode": "asset_only",
        "durationSec": 1.0,
        "fps": fps,
        "resolution": {"width": width, "height": height},
        "transparent": False,
        "renderEngine": str(render_config.get("renderEngine") or DEFAULT_RENDER_ENGINE),
        "qualityPreset": "master",
        "renderDetailMode": "publish",
        "eeveeSamples": QUALITY_PRESETS["master"]["eeveeSamples"],
        "cyclesSamples": QUALITY_PRESETS["master"]["cyclesSamples"],
        "targetCharacterHeight": float(render_config.get("targetCharacterHeight") or 2.55),
        "faceScreenMode": str(render_config.get("faceScreenMode") or "source"),
        "rigMode": str(model_config.get("rigMode") or "auto"),
        "preserveExistingRig": bool(model_config.get("preserveExistingRig", True)),
        "enhanceExistingRig": bool(model_config.get("enhanceExistingRig", True)),
        "elbowRig": True,
        "mouthMode": str(facial_config.get("mouthMode") or "auto"),
        "mouthHeightRatio": float(facial_config.get("mouthHeightRatio") or 0.56),
        "mouthScale": float(facial_config.get("mouthScale") or 1.0),
        "mouthStyle": str(facial_config.get("mouthStyle") or "auto"),
        "facialDetailMode": str(facial_config.get("facialDetailMode") or "rich"),
        "facialTopologyMode": str(facial_config.get("topologyMode") or "source_retopology"),
        "blinkCapability": str(facial_config.get("blinkCapability") or "squint_only"),
        "motionPlan": motion_plan,
    }
    _write_json(paths["qa"], qa_input)

    result = {
        "status": "planned" if dryRun else "ready",
        "success": True,
        "dryRun": bool(dryRun),
        "characterId": str(profile.get("characterId") or ""),
        "characterProfilePath": str(profile_path),
        "sourceModel": str(source_path),
        "qualityTier": requested_tier,
        "masterBlendPath": str(paths["master"]),
        "exportGlbPath": str(paths["glb"]),
        "rigReportPath": str(paths["report"]),
        "qaInputPath": str(paths["qa"]),
    }
    if dryRun:
        return result

    blender = find_blender()
    if not blender:
        raise RuntimeError("Blender is required. Set TANGYING_BLENDER_BIN or install Blender.")
    blender_script = Path(__file__).with_name("blender_renderer.py")
    _run(
        [blender, "--background", "--python", str(blender_script), "--", str(paths["qa"])],
        timeout=max(1200, int(render_config.get("blenderTimeoutSec") or 0)),
    )
    missing = [
        path.name
        for path in (paths["master"], paths["glb"], paths["report"])
        if not path.is_file()
    ]
    if missing:
        raise RuntimeError(f"Blender did not produce character master outputs: {missing}")
    result["masterExists"] = True
    result["exportGlbExists"] = True
    result["rigReportExists"] = True
    return result


@mcp.tool()
def plan_motion(script: str, durationSec: float = 0, fps: int = DEFAULT_FPS, motionStyle: str = "expressive") -> dict[str, Any]:
    """Analyze narration text into deterministic lip-sync and body-motion timelines."""
    duration = float(durationSec or estimate_duration(script))
    return build_motion_plan(script, duration, fps, motionStyle)


@mcp.tool()
def render_talking_video(
    script: str,
    modelPath: str = "",
    characterProfilePath: str = "",
    audioPath: str = "",
    subtitlePath: str = "",
    backgroundPath: str = "",
    sceneBlendPath: str = "",
    outputDir: str = "",
    characterId: str = "",
    shotId: str = "",
    durationSec: float = 0,
    fps: int = DEFAULT_FPS,
    width: int = DEFAULT_WIDTH,
    height: int = DEFAULT_HEIGHT,
    transparent: bool = False,
    faceScreenMode: str = "source",
    rigMode: str = "auto",
    preserveExistingRig: bool = True,
    enhanceExistingRig: bool = True,
    elbowRig: bool = True,
    mouthMode: str = "auto",
    mouthHeightRatio: float = 0.56,
    mouthScale: float = 1.0,
    mouthStyle: str = "auto",
    facialDetailMode: str = "rich",
    facialTopologyMode: str = "auto",
    backgroundBrightness: float = 0.88,
    cameraPreset: str = DEFAULT_CAMERA_PRESET,
    lightingPreset: str = DEFAULT_LIGHTING_PRESET,
    renderEngine: str = DEFAULT_RENDER_ENGINE,
    qualityPreset: str = DEFAULT_QUALITY_PRESET,
    renderDetailMode: str = "auto",
    targetCharacterHeight: float = 2.55,
    motionStyle: str = "expressive",
    voiceName: str = "",
    speakingRate: int = 190,
    voiceProvider: str = "auto",
    voiceId: str = "",
    voiceLanguage: str = "zh",
    voiceSpeed: float = 1.0,
    blenderTimeoutSec: int = 0,
    dryRun: bool = False,
) -> dict[str, Any]:
    """Render a talking IP video layer from narration text and a local GLB/GLTF/FBX model."""
    profile_path: Path | None = None
    master_blend_path = ""
    master_configured = False
    quality_tier = ""
    if characterProfilePath:
        profile_path, profile = _load_character_profile(characterProfilePath)
        model_config = profile.get("model") or {}
        facial_config = profile.get("facial") or {}
        render_config = profile.get("render") or {}
        voice_config = profile.get("voice") or {}
        quality_tier = str(model_config.get("qualityTier") or "")
        if model_config.get("masterBlendPath"):
            master_configured = True
            master_blend_path = _profile_asset(profile_path, model_config.get("masterBlendPath"))
        if not modelPath:
            modelPath = _profile_asset(profile_path, model_config.get("path"))
        if not backgroundPath:
            backgroundPath = _profile_asset(profile_path, render_config.get("backgroundPath"))
        if not sceneBlendPath:
            sceneBlendPath = _profile_asset(profile_path, render_config.get("sceneBlendPath"))
        characterId = characterId or str(profile.get("characterId") or "")
        if rigMode == "auto":
            rigMode = str(model_config.get("rigMode") or rigMode)
        preserveExistingRig = bool(model_config.get("preserveExistingRig", preserveExistingRig))
        enhanceExistingRig = bool(model_config.get("enhanceExistingRig", enhanceExistingRig))
        if mouthMode == "auto":
            mouthMode = str(facial_config.get("mouthMode") or mouthMode)
        if mouthHeightRatio == 0.56:
            mouthHeightRatio = float(facial_config.get("mouthHeightRatio") or mouthHeightRatio)
        if mouthScale == 1.0:
            mouthScale = float(facial_config.get("mouthScale") or mouthScale)
        if mouthStyle == "auto":
            mouthStyle = str(facial_config.get("mouthStyle") or mouthStyle)
        facialDetailMode = str(facial_config.get("facialDetailMode") or facialDetailMode)
        if facialTopologyMode == "auto":
            facialTopologyMode = str(facial_config.get("topologyMode") or facialTopologyMode)
        if faceScreenMode == "source":
            faceScreenMode = str(render_config.get("faceScreenMode") or faceScreenMode)
        if backgroundBrightness == 0.88:
            backgroundBrightness = float(render_config.get("backgroundBrightness", backgroundBrightness))
        if cameraPreset == DEFAULT_CAMERA_PRESET:
            cameraPreset = str(render_config.get("cameraPreset") or cameraPreset)
        if lightingPreset == DEFAULT_LIGHTING_PRESET:
            lightingPreset = str(render_config.get("lightingPreset") or lightingPreset)
        if renderEngine == DEFAULT_RENDER_ENGINE:
            renderEngine = str(render_config.get("renderEngine") or renderEngine)
        if qualityPreset == DEFAULT_QUALITY_PRESET:
            qualityPreset = str(render_config.get("qualityPreset") or qualityPreset)
        if renderDetailMode == "auto":
            renderDetailMode = str(render_config.get("renderDetailMode") or renderDetailMode)
        if targetCharacterHeight == 2.55:
            targetCharacterHeight = float(render_config.get("targetCharacterHeight") or targetCharacterHeight)
        if motionStyle == "expressive":
            motionStyle = str(render_config.get("motionStyle") or motionStyle)
        if fps == DEFAULT_FPS:
            fps = int(render_config.get("fps") or fps)
        resolution = render_config.get("resolution") or {}
        if width == DEFAULT_WIDTH:
            width = int(resolution.get("width") or width)
        if height == DEFAULT_HEIGHT:
            height = int(resolution.get("height") or height)
        if not transparent:
            transparent = bool(render_config.get("transparent", transparent))
        if blenderTimeoutSec <= 0:
            blenderTimeoutSec = int(render_config.get("blenderTimeoutSec") or 0)
        if voiceProvider == "auto":
            voiceProvider = str(voice_config.get("provider") or voiceProvider)
        if not voiceId:
            voiceId = str(voice_config.get("voiceId") or voiceId)
        if voiceLanguage == "zh":
            voiceLanguage = str(voice_config.get("language") or voiceLanguage)
        if voiceSpeed == 1.0:
            voiceSpeed = float(voice_config.get("speed") or voiceSpeed)
        if speakingRate == 190:
            speakingRate = int(voice_config.get("speakingRate") or speakingRate)

    use_master_asset = False
    if master_configured:
        configured_master = Path(master_blend_path)
        if configured_master.suffix.lower() != ".blend":
            raise ValueError("masterBlendPath must point to a Blender .blend file")
        if configured_master.is_file():
            use_master_asset = True
            enhanceExistingRig = False
            facialTopologyMode = "source_only"
        elif not dryRun:
            raise FileNotFoundError(
                f"masterBlendPath does not exist: {configured_master}; run prepare_character_master first"
            )

    if not modelPath and not use_master_asset:
        if profile_path:
            raise ValueError(f"character profile model.path is not configured yet: {profile_path}")
        raise ValueError("modelPath is required when characterProfilePath is not provided")
    model_path = _readable_path(modelPath) if modelPath else None
    if model_path and not model_path.exists() and not use_master_asset:
        raise FileNotFoundError(f"modelPath does not exist: {model_path}")
    if model_path and model_path.suffix.lower() not in {".glb", ".gltf", ".fbx"} and not use_master_asset:
        raise ValueError("modelPath must point to a GLB, GLTF, or rigged FBX file")

    output_dir = _readable_path(outputDir) if outputDir else (_repo_root() / "tmp" / "ip_avatar_3d" / _now_id()).resolve()
    output_dir.mkdir(parents=True, exist_ok=True)
    fps = max(8, min(int(fps or DEFAULT_FPS), 60))
    width = max(320, min(int(width or DEFAULT_WIDTH), 3840))
    height = max(180, min(int(height or DEFAULT_HEIGHT), 2160))
    background_brightness = max(0.35, min(float(backgroundBrightness or 0.88), 1.1))
    mouth_height_ratio = max(0.35, min(float(mouthHeightRatio or 0.56), 0.85))
    mouth_scale = max(0.35, min(float(mouthScale or 1.0), 2.5))
    mouth_style = str(mouthStyle or "auto").strip().lower()
    facial_detail_mode = str(facialDetailMode or "rich").strip().lower()
    if facial_detail_mode not in {"rich", "basic", "off", "none"}:
        raise ValueError("facialDetailMode must be one of: rich, basic, off")
    facial_topology_mode = str(facialTopologyMode or "auto").strip().lower()
    if facial_topology_mode == "auto":
        facial_topology_mode = "source_only"
    if facial_topology_mode not in {
        "source_only", "source_retopology", "integrated_source", "volumetric", "off", "none"
    }:
        raise ValueError(
            "facialTopologyMode must be one of: auto, source_only, source_retopology, integrated_source, volumetric, off"
        )
    target_character_height = max(0.25, min(float(targetCharacterHeight or 2.55), 10.0))

    camera_preset = str(cameraPreset or DEFAULT_CAMERA_PRESET).strip().lower()
    if camera_preset not in CAMERA_PRESETS:
        raise ValueError(f"cameraPreset must be one of: {', '.join(sorted(CAMERA_PRESETS))}")
    lighting_preset = str(lightingPreset or DEFAULT_LIGHTING_PRESET).strip().lower()
    if lighting_preset not in LIGHTING_PRESETS:
        raise ValueError(f"lightingPreset must be one of: {', '.join(sorted(LIGHTING_PRESETS))}")
    render_engine = str(renderEngine or DEFAULT_RENDER_ENGINE).strip().upper()
    if render_engine == "EEVEE":
        render_engine = DEFAULT_RENDER_ENGINE
    if render_engine not in RENDER_ENGINES:
        raise ValueError(f"renderEngine must be one of: {', '.join(sorted(RENDER_ENGINES))}")
    quality_preset = str(qualityPreset or DEFAULT_QUALITY_PRESET).strip().lower()
    if quality_preset not in QUALITY_PRESETS:
        raise ValueError(f"qualityPreset must be one of: {', '.join(sorted(QUALITY_PRESETS))}")
    quality = QUALITY_PRESETS[quality_preset]
    render_detail_mode = str(renderDetailMode or "auto").strip().lower()
    if render_detail_mode not in {"auto", "publish", "preview", "off", "none"}:
        raise ValueError("renderDetailMode must be one of: auto, publish, preview, off")
    voice_provider = str(voiceProvider or "auto").strip().lower()
    voice_language = str(voiceLanguage or "zh").strip().lower()
    voice_speed = max(0.7, min(float(voiceSpeed or 1.0), 1.3))

    resolved_background = str(_readable_path(backgroundPath)) if backgroundPath else ""
    if resolved_background and not Path(resolved_background).is_file():
        raise FileNotFoundError(f"backgroundPath does not exist: {resolved_background}")
    resolved_scene = str(_readable_path(sceneBlendPath)) if sceneBlendPath else ""
    if resolved_scene:
        scene_path = Path(resolved_scene)
        if scene_path.suffix.lower() != ".blend":
            raise ValueError("sceneBlendPath must point to a Blender .blend file")
        if not scene_path.is_file():
            raise FileNotFoundError(f"sceneBlendPath does not exist: {resolved_scene}")

    initial_duration = float(durationSec or 0)
    if audioPath:
        resolved_audio = str(_readable_path(audioPath))
        initial_duration = audio_duration_sec(resolved_audio) or initial_duration
    duration = initial_duration or estimate_duration(script)
    motion_plan = build_motion_plan(script, duration, fps, motionStyle)
    duration = float(motion_plan["durationSec"])
    camera_plan = build_camera_plan(duration, fps, camera_preset)
    background_mode = "blender_scene" if resolved_scene else ("static_plate" if resolved_background else "transparent_or_world")
    effective_transparent = bool(transparent and not resolved_scene)
    blender_timeout = (
        max(300, min(int(blenderTimeoutSec), 21600))
        if int(blenderTimeoutSec or 0) > 0
        else estimate_blender_timeout(duration, fps, width, height)
    )

    subtitle_out = Path(subtitlePath).expanduser().resolve() if subtitlePath else output_dir / "subtitle.srt"
    if not subtitlePath:
        subtitle_out.write_text(build_subtitle_text(script, duration), encoding="utf-8")
    audio_out = ""
    audioSource = "none"
    audio_metadata: dict[str, Any] = {
        "provider": voice_provider,
        "voiceId": str(voiceId or voiceName or ""),
        "language": voice_language,
        "speed": voice_speed,
        "humanVoiceProvider": False,
        "productionReady": False,
    }
    if not dryRun:
        audio_out, audioSource, audio_metadata = ensure_audio(
            script,
            output_dir,
            duration,
            audioPath,
            voiceName,
            speakingRate,
            voice_provider=voice_provider,
            voice_id=voiceId,
            voice_language=voice_language,
            voice_speed=voice_speed,
        )

    plan_path = output_dir / "motion_plan.json"
    render_input_path = output_dir / "render_input.json"
    report_path = output_dir / "render_report.json"
    frames_dir = output_dir / "frames"
    video_path = output_dir / "ip_layer.mp4"
    alpha_path = output_dir / "avatar_layer.webm"
    preview_path = output_dir / "preview_frame.png"
    rigged_blend_path = output_dir / "rigged_avatar.blend"
    rigged_glb_path = output_dir / "rigged_avatar.glb"
    rig_report_path = output_dir / "rig_report.json"

    _write_json(plan_path, motion_plan)
    render_input = {
        "schemaVersion": "ip-avatar-3d-blender-render/v2",
        "script": script,
        "characterId": characterId,
        "characterProfilePath": str(profile_path) if profile_path else "",
        "shotId": shotId,
        "modelPath": str(model_path) if model_path else "",
        "masterBlendPath": master_blend_path,
        "masterConfigured": master_configured,
        "masterExists": bool(master_blend_path and Path(master_blend_path).is_file()),
        "useMasterAsset": use_master_asset,
        "preparationRequired": bool(master_configured and not use_master_asset),
        "qualityTier": quality_tier,
        "sceneBlendPath": resolved_scene,
        "backgroundPath": resolved_background,
        "backgroundMode": background_mode,
        "backgroundBrightness": background_brightness,
        "framesDir": str(frames_dir),
        "previewPath": str(preview_path),
        "durationSec": duration,
        "fps": fps,
        "resolution": {"width": width, "height": height},
        "transparent": effective_transparent,
        "cameraPreset": camera_preset,
        "cameraPlan": camera_plan,
        "lightingPreset": lighting_preset,
        "renderEngine": render_engine,
        "qualityPreset": quality_preset,
        "renderDetailMode": render_detail_mode,
        "eeveeSamples": quality["eeveeSamples"],
        "cyclesSamples": quality["cyclesSamples"],
        "videoCrf": quality["videoCrf"],
        "videoPreset": quality["videoPreset"],
        "targetCharacterHeight": target_character_height,
        "blenderTimeoutSec": blender_timeout,
        "faceScreenMode": faceScreenMode,
        "rigMode": rigMode,
        "preserveExistingRig": bool(preserveExistingRig),
        "enhanceExistingRig": bool(enhanceExistingRig),
        "elbowRig": bool(elbowRig),
        "mouthMode": mouthMode,
        "mouthHeightRatio": mouth_height_ratio,
        "mouthScale": mouth_scale,
        "mouthStyle": mouth_style,
        "facialDetailMode": facial_detail_mode,
        "facialTopologyMode": facial_topology_mode,
        "preserveSourceMaterials": True,
        "riggedBlendPath": str(rigged_blend_path),
        "riggedGlbPath": str(rigged_glb_path),
        "rigReportPath": str(rig_report_path),
        "motionPlanPath": str(plan_path),
        "motionPlan": motion_plan,
        "voice": {
            "provider": voice_provider,
            "voiceId": str(voiceId or voiceName or ""),
            "language": voice_language,
            "speed": voice_speed,
        },
    }
    _write_json(render_input_path, render_input)

    if dryRun:
        report = {
            "success": True,
            "dryRun": True,
            "modelExists": bool(model_path and model_path.exists()),
            "masterExists": bool(master_blend_path and Path(master_blend_path).is_file()),
            "preparationRequired": bool(master_configured and not use_master_asset),
            "motionPlanGenerated": plan_path.exists(),
            "subtitleGenerated": subtitle_out.exists(),
            "blenderRequired": True,
        }
        _write_json(report_path, report)
        return {
            "status": "planned",
            "success": True,
            "dryRun": True,
            "providerName": "ip_avatar_3d",
            "sourceType": "ip_aroll_video",
            "characterId": characterId,
            "shotId": shotId,
            "durationSec": duration,
            "modelPath": str(model_path) if model_path else "",
            "masterBlendPath": master_blend_path,
            "sceneBlendPath": resolved_scene,
            "characterProfilePath": str(profile_path) if profile_path else "",
            "motionPlanPath": str(plan_path),
            "subtitlePath": str(subtitle_out),
            "renderInputPath": str(render_input_path),
            "renderReportPath": str(report_path),
            "riggedBlendPath": str(rigged_blend_path),
            "riggedGlbPath": str(rigged_glb_path),
            "rigReportPath": str(rig_report_path),
        }

    blender = find_blender()
    if not blender:
        raise RuntimeError("Blender is required. Set TANGYING_BLENDER_BIN or install Blender.")
    blender_script = Path(__file__).with_name("blender_renderer.py")
    _run(
        [blender, "--background", "--python", str(blender_script), "--", str(render_input_path)],
        timeout=blender_timeout,
    )

    missing_rig_outputs = [
        path.name
        for path in (rigged_blend_path, rigged_glb_path, rig_report_path)
        if not path.is_file()
    ]
    if missing_rig_outputs:
        raise RuntimeError(f"Blender did not produce rig outputs: {missing_rig_outputs}")

    _compose_video(
        frames_dir,
        audio_out,
        video_path,
        duration,
        fps,
        background_path="" if resolved_scene else resolved_background,
        background_brightness=background_brightness,
        width=width,
        height=height,
        video_crf=int(quality["videoCrf"]),
        video_preset=str(quality["videoPreset"]),
    )
    _extract_preview(video_path, preview_path)
    avatar_layer_path = ""
    if effective_transparent:
        _compose_alpha_webm(frames_dir, alpha_path, duration, fps)
        avatar_layer_path = str(alpha_path) if alpha_path.exists() else ""

    qa = _probe_video(video_path)
    qa.update(
        {
            "modelExists": bool(model_path and model_path.exists()),
            "masterExists": bool(master_blend_path and Path(master_blend_path).is_file()),
            "motionPlanGenerated": plan_path.exists(),
            "subtitleGenerated": subtitle_out.exists(),
            "audioGenerated": bool(audio_out and Path(audio_out).exists()),
            "audioSource": audioSource,
            "finalVideoGenerated": video_path.exists(),
        }
    )
    report = {"success": video_path.exists(), "qa": qa, "renderInputPath": str(render_input_path)}
    _write_json(report_path, report)
    digest = hashlib.sha256(video_path.read_bytes()).hexdigest() if video_path.exists() else ""
    return {
        "status": "ready" if video_path.exists() else "failed",
        "success": video_path.exists(),
        "providerName": "ip_avatar_3d",
        "sourceType": "ip_aroll_video",
        "kind": "video",
        "characterId": characterId,
        "shotId": shotId,
        "durationSec": duration,
        "videoPath": str(video_path),
        "localPath": str(video_path),
        "mediaPath": str(video_path),
        "avatarLayerPath": avatar_layer_path,
        "previewImagePath": str(preview_path) if preview_path.exists() else "",
        "modelPath": str(model_path) if model_path else "",
        "masterBlendPath": master_blend_path,
        "sceneBlendPath": resolved_scene,
        "characterProfilePath": str(profile_path) if profile_path else "",
        "audioPath": audio_out,
        "audioSource": audioSource,
        "voice": audio_metadata,
        "voicePreviewOnly": not bool(audio_metadata.get("productionReady")),
        "subtitlePath": str(subtitle_out),
        "motionPlanPath": str(plan_path),
        "renderInputPath": str(render_input_path),
        "renderReportPath": str(report_path),
        "riggedBlendPath": str(rigged_blend_path),
        "riggedGlbPath": str(rigged_glb_path),
        "rigReportPath": str(rig_report_path),
        "sha256": digest,
        "qa": qa,
        "metadata": {
            "layer": "ip_aroll",
            "renderEngine": "blender",
            "modelFormat": "blend_master" if use_master_asset else (model_path.suffix.lower().lstrip(".") if model_path else ""),
            "characterProfilePath": str(profile_path) if profile_path else "",
            "masterBlendPath": master_blend_path,
            "useMasterAsset": use_master_asset,
            "qualityTier": quality_tier,
            "faceScreenMode": faceScreenMode,
            "rigMode": rigMode,
            "preserveExistingRig": bool(preserveExistingRig),
            "enhanceExistingRig": bool(enhanceExistingRig),
            "elbowRig": bool(elbowRig),
            "mouthMode": mouthMode,
            "mouthHeightRatio": mouth_height_ratio,
            "mouthScale": mouth_scale,
            "mouthStyle": mouth_style,
            "facialDetailMode": facial_detail_mode,
            "facialTopologyMode": facial_topology_mode,
            "preserveSourceMaterials": True,
            "backgroundMode": background_mode,
            "backgroundBrightness": background_brightness,
            "sceneBlendPath": resolved_scene,
            "cameraPreset": camera_preset,
            "cameraPlan": camera_plan,
            "lightingPreset": lighting_preset,
            "renderEngine": render_engine,
            "qualityPreset": quality_preset,
            "renderDetailMode": render_detail_mode,
            "eeveeSamples": quality["eeveeSamples"],
            "videoCrf": quality["videoCrf"],
            "targetCharacterHeight": target_character_height,
            "blenderTimeoutSec": blender_timeout,
            "transparent": effective_transparent,
            "resolution": f"{width}x{height}",
            "fps": fps,
            "voice": audio_metadata,
        },
    }


if __name__ == "__main__":
    mcp.run()
