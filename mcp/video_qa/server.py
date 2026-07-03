#!/usr/bin/env python3
"""Standard MCP stdio server for Tangying video shot QA.

The server keeps stdout reserved for MCP JSON-RPC. Diagnostics should go to
stderr only. The core QA functions are importable without the MCP SDK so local
unit tests can run in minimal Python environments.
"""

from __future__ import annotations

import json
import math
import os
import pathlib
import subprocess
import sys
from typing import Any, Callable

try:
    from mcp.server.fastmcp import FastMCP
except ImportError:
    FastMCP = None  # type: ignore[assignment]


class _FallbackMCP:
    def tool(self) -> Callable[[Callable[..., Any]], Callable[..., Any]]:
        def decorator(func: Callable[..., Any]) -> Callable[..., Any]:
            return func

        return decorator

    def run(self) -> None:
        print(
            "Missing Python MCP SDK. Install with: python3 -m pip install -r mcp/video_qa/requirements.txt",
            file=sys.stderr,
        )
        raise SystemExit(2)


mcp = FastMCP("Video QA MCP") if FastMCP else _FallbackMCP()


def _run(args: list[str], timeout: int = 600) -> str:
    completed = subprocess.run(
        args,
        check=False,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=timeout,
    )
    if completed.returncode != 0:
        detail = completed.stderr.strip() or completed.stdout.strip() or f"exit {completed.returncode}"
        raise RuntimeError(detail)
    return completed.stdout


def _run_bytes(args: list[str], timeout: int = 600) -> bytes:
    completed = subprocess.run(
        args,
        check=False,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=timeout,
    )
    if completed.returncode != 0:
        detail = completed.stderr.decode("utf-8", "ignore").strip() or f"exit {completed.returncode}"
        raise RuntimeError(detail)
    return completed.stdout


def _number(value: Any, fallback: float = 0) -> float:
    if isinstance(value, bool):
        return fallback
    if isinstance(value, (int, float)):
        return float(value)
    if isinstance(value, str):
        try:
            return float(value.strip())
        except ValueError:
            return fallback
    return fallback


def _string(value: Any) -> str:
    return value.strip() if isinstance(value, str) else ""


def _round(value: float) -> float:
    return round(value + 0.0, 4)


def _unique(values: list[str]) -> list[str]:
    seen: set[str] = set()
    out: list[str] = []
    for value in values:
        value = value.strip()
        if value and value not in seen:
            seen.add(value)
            out.append(value)
    return out


def _first_present(*values: Any) -> Any:
    for value in values:
        if isinstance(value, str) and value.strip():
            return value
        if isinstance(value, list) and value:
            return value
        if value not in (None, ""):
            return value
    return None


def _text_char_count(value: Any) -> int:
    if isinstance(value, str):
        return len(value.strip())
    if isinstance(value, list):
        return sum(_text_char_count(item) for item in value)
    if isinstance(value, dict):
        return _text_char_count(_first_present(value.get("text"), value.get("content"), value.get("value")))
    return 0


def _has_bool_key(value: Any, target: str) -> bool:
    if isinstance(value, dict):
        for key, item in value.items():
            if key.lower() == target.lower() and bool(item):
                return True
            if _has_bool_key(item, target):
                return True
    if isinstance(value, list):
        return any(_has_bool_key(item, target) for item in value)
    return False


def _risk_max(current: str, candidate: str) -> str:
    rank = {"low": 0, "medium": 1, "high": 2}
    return candidate if rank.get(candidate, 0) > rank.get(current, 0) else current or candidate


def build_shot_spec_lints(shot_list: list[dict[str, Any]]) -> list[dict[str, Any]]:
    lints: list[dict[str, Any]] = []
    for idx, shot in enumerate(shot_list):
        shot_id = _string(_first_present(shot.get("shotId"), shot.get("id"))) or f"shot_{idx + 1:02d}"
        duration = _number(_first_present(shot.get("durationSec"), shot.get("duration")), 0)
        screen_text_chars = _text_char_count(_first_present(shot.get("screenText"), shot.get("text"), shot.get("title")))
        must_be_exact = _has_bool_key(shot, "mustBeExact")
        lint: dict[str, Any] = {
            "shotId": shot_id,
            "riskLevel": "low",
            "durationSec": _round(duration),
            "screenTextChars": screen_text_chars,
            "requiredAssets": [],
            "generationWarnings": [],
            "fatalGateTriggered": False,
        }

        if duration and (duration < 3 or duration > 15):
            lint["riskLevel"] = "high"
            lint["fatalGateTriggered"] = True
            lint["recommendedAction"] = "REVISE_SHOT_SPEC"
            lint["recommendedFix"] = "将该 shot 调整到 3-15 秒，或拆分成多个单场景 shot 后再生成。"
            lint["generationWarnings"].append("duration_outside_3_15s")

        if screen_text_chars > 60:
            lint["riskLevel"] = "high"
            lint["fatalGateTriggered"] = True
            lint["renderStrategyHint"] = "html_overlay"
            lint["recommendedAction"] = "REVISE_SHOT_SPEC"
            lint["recommendedFix"] = "screenText 过长，建议缩短画面文字，并改由 HyperFrames/HTML overlay 精确渲染。"
            lint["generationWarnings"].append("screen_text_too_long")
        elif screen_text_chars > 36:
            lint["riskLevel"] = _risk_max(lint["riskLevel"], "medium")
            lint["renderStrategyHint"] = "html_overlay"
            lint.setdefault("recommendedAction", "RERENDER_HTML")
            lint.setdefault("recommendedFix", "画面文字偏长，建议改为 HTML overlay 并控制为 1-2 行。")
            lint["generationWarnings"].append("screen_text_should_use_html_overlay")

        if must_be_exact:
            lint["exactTextMustUseHtml"] = True
            lint["riskLevel"] = _risk_max(lint["riskLevel"], "medium")
            lint["renderStrategyHint"] = "html_overlay"
            if lint.get("recommendedAction") in (None, "", "PASS"):
                lint["recommendedAction"] = "RERENDER_HTML"
            lint.setdefault("recommendedFix", "MustBeExact 文字必须走 HyperFrames/HTML overlay，禁止由 AIGC 视频模型内生生成。")
            lint["generationWarnings"].append("must_be_exact_text_requires_html_overlay")

        if _has_bool_key(shot, "mustMatchPrevious"):
            lint["riskLevel"] = _risk_max(lint["riskLevel"], "medium")
            lint["requiredAssets"].append({"kind": "reference_image", "role": "previous_shot_end_state", "required": True})
            lint["generationWarnings"].append("continuity_reference_required")

        lint.setdefault("recommendedAction", "PASS")
        lint["generationWarnings"] = _unique(lint["generationWarnings"])
        if not lint["requiredAssets"]:
            lint.pop("requiredAssets")
        lints.append(lint)
    return lints


def _issue_codes(summary: dict[str, Any]) -> set[str]:
    return {str(issue.get("code", "")) for issue in summary.get("representativeIssues", []) if isinstance(issue, dict)}


def _decision_for_summary(summary: dict[str, Any], lint: dict[str, Any] | None = None) -> str:
    if lint and lint.get("fatalGateTriggered") and lint.get("recommendedAction") not in (None, "", "PASS"):
        return str(lint["recommendedAction"])
    codes = _issue_codes(summary)
    if {"top_left_text_zone_crowded", "lower_third_text_zone_crowded"} & codes:
        return "RERENDER_HTML"
    if int(summary.get("blockingIssueCount", 0) or 0) > 0:
        return "RECOMPOSITE"
    if lint and lint.get("recommendedAction") not in (None, "", "PASS"):
        return "PASS_WITH_FIX"
    if int(summary.get("warningIssueCount", 0) or 0) > 0:
        return "PASS_WITH_FIX"
    return "PASS"


def _repair_plan(decision: str, summary: dict[str, Any], lint: dict[str, Any] | None = None) -> dict[str, Any]:
    plan: dict[str, Any] = {
        "action": decision,
        "priority": 0 if decision == "PASS" else 2 if decision in {"PASS_WITH_FIX", "HUMAN_REVIEW"} else 1,
        "toolOverrides": {},
        "renderStrategyPatch": {},
        "visualPlanPatch": {},
        "promptPatch": {},
        "recommendedNextStage": "quality_gate",
    }
    if decision == "PASS":
        plan["reason"] = "shot 级 QA 通过，无需工具返工。"
    elif decision in {"RERENDER_HTML", "PASS_WITH_FIX"}:
        plan["action"] = "RERENDER_HTML" if decision == "PASS_WITH_FIX" else decision
        plan["reason"] = "文字安全区或叠字风险需要通过 HyperFrames/FFmpeg 重新渲染或合成。"
        plan["toolOverrides"] = {
            "renderStrategyMode": "hybrid",
            "primaryTool": "hyperframes",
            "secondaryTools": ["ffmpeg"],
            "textOverlayNeeded": True,
            "needsCompositing": True,
        }
        plan["renderStrategyPatch"] = {
            "mode": "hybrid",
            "primaryTool": "hyperframes",
            "secondaryTools": ["ffmpeg"],
            "htmlRequired": True,
            "textOverlayNeeded": True,
            "needsCompositing": True,
        }
        plan["visualPlanPatch"] = {
            "textLayers": [{"position": "safe_area", "background": "semi_transparent_dark", "fontSize": 36}]
        }
        plan["promptPatch"] = {
            "negativeAdditions": ["no embedded text", "no watermark", "no subtitles inside generated video"]
        }
    elif decision == "REVISE_SHOT_SPEC":
        plan["reason"] = "shot 规格本身风险过高，应先修改时长、动作或画面文字后再生成。"
        plan["toolOverrides"] = {"splitShot": True, "minDurationSec": 3, "maxDurationSec": 15}
        plan["renderStrategyPatch"] = {"mode": "hybrid"}
        plan["promptPatch"] = {"negativeAdditions": ["overly complex action", "multiple scene changes in one shot"]}
    elif decision == "RECOMPOSITE":
        plan["reason"] = "画面复杂度或叠层风险需要重新合成。"
        plan["toolOverrides"] = {"primaryTool": "ffmpeg", "secondaryTools": ["hyperframes"], "needsCompositing": True}
        plan["renderStrategyPatch"] = {"mode": "hybrid", "needsCompositing": True}
        plan["promptPatch"] = {"negativeAdditions": ["visual clutter", "busy background", "unreadable UI"]}
    else:
        plan["reason"] = "当前指标不足以自动决定返修方式，需要人工审核。"

    if lint and lint.get("requiredAssets"):
        plan["requiredAssets"] = lint["requiredAssets"]
    return plan


def _scores(summary: dict[str, Any], lint: dict[str, Any] | None = None) -> dict[str, int]:
    codes = _issue_codes(summary)
    text_score = 55 if {"top_left_text_zone_crowded", "lower_third_text_zone_crowded"} & codes else 78 if codes else 96
    metrics = summary.get("metricSummary", {}) if isinstance(summary.get("metricSummary"), dict) else {}
    image_score = 76 if _number(metrics.get("maxFullFrameEdgeDensity"), 0) >= 0.105 else 92
    media_score = 45 if lint and "duration_outside_3_15s" in lint.get("generationWarnings", []) else 96
    return {
        "mediaSpec": media_score,
        "textLayout": text_score,
        "audioSpeech": 100,
        "humanQuality": 100,
        "imageQuality": image_score,
        "temporalStability": 100,
        "promptAlignment": 82,
        "continuity": 82,
    }


def _hard_metrics(summary: dict[str, Any], lint: dict[str, Any] | None = None) -> dict[str, float]:
    out: dict[str, float] = {}
    metrics = summary.get("metricSummary", {}) if isinstance(summary.get("metricSummary"), dict) else {}
    for key, value in metrics.items():
        out[key] = _number(value, 0)
    out["blockingIssueCount"] = _number(summary.get("blockingIssueCount"), 0)
    out["warningIssueCount"] = _number(summary.get("warningIssueCount"), 0)
    if lint:
        out["durationSec"] = _number(lint.get("durationSec"), 0)
        out["screenTextChars"] = _number(lint.get("screenTextChars"), 0)
    return out


def _shot_issues(summary: dict[str, Any]) -> list[dict[str, Any]]:
    timestamp = 0
    sampled = summary.get("sampledTimesSec")
    if isinstance(sampled, list) and sampled:
        timestamp = _number(sampled[0], 0)
    issues: list[dict[str, Any]] = []
    for issue in summary.get("representativeIssues", []):
        if not isinstance(issue, dict):
            continue
        issues.append(
            {
                "type": issue.get("code", ""),
                "severity": issue.get("severity", ""),
                "timestampSec": timestamp,
                "zone": issue.get("zone", ""),
                "evidence": issue.get("message", ""),
                "suggestedFix": issue.get("suggestion", ""),
                "metricValue": _number(issue.get("value"), 0),
                "threshold": _number(issue.get("threshold"), 0),
            }
        )
    return issues


def build_shot_reports(
    project_id: str,
    video_type: str,
    candidate_id: str,
    summaries: list[dict[str, Any]],
    spec_lints: list[dict[str, Any]],
) -> list[dict[str, Any]]:
    lint_by_shot = {lint.get("shotId"): lint for lint in spec_lints}
    reports: list[dict[str, Any]] = []
    seen: set[str] = set()
    for summary in summaries:
        shot_id = str(summary.get("shotId", ""))
        lint = lint_by_shot.get(shot_id)
        decision = _decision_for_summary(summary, lint)
        report = {
            "schemaVersion": 1,
            "projectId": project_id,
            "shotId": shot_id,
            "videoType": video_type,
            "candidateId": candidate_id,
            "overallScore": int(summary.get("score", 0) or 0),
            "decision": decision,
            "fatalGateTriggered": bool(summary.get("blockingIssueCount")) or bool(lint and lint.get("fatalGateTriggered")),
            "scores": _scores(summary, lint),
            "hardMetrics": _hard_metrics(summary, lint),
            "issues": _shot_issues(summary),
            "repairPlan": _repair_plan(decision, summary, lint),
            "sourceSummary": summary,
        }
        if lint:
            report["shotSpecLint"] = lint
        reports.append(report)
        seen.add(shot_id)
    for lint in spec_lints:
        shot_id = str(lint.get("shotId", ""))
        if shot_id in seen:
            continue
        score = 55 if lint.get("fatalGateTriggered") else 78 if lint.get("generationWarnings") else 100
        summary = {
            "shotId": shot_id,
            "passed": not lint.get("fatalGateTriggered"),
            "needsRegeneration": bool(lint.get("fatalGateTriggered")),
            "score": score,
            "blockingIssueCount": 1 if lint.get("fatalGateTriggered") else 0,
            "warningIssueCount": 0 if lint.get("fatalGateTriggered") else len(lint.get("generationWarnings", [])),
            "metricSummary": {},
            "conclusion": lint.get("recommendedFix", ""),
            "recommendations": [lint.get("recommendedFix", "")] if lint.get("recommendedFix") else [],
        }
        decision = _decision_for_summary(summary, lint)
        reports.append(
            {
                "schemaVersion": 1,
                "projectId": project_id,
                "shotId": shot_id,
                "videoType": video_type,
                "candidateId": candidate_id,
                "overallScore": score,
                "decision": decision,
                "fatalGateTriggered": bool(lint.get("fatalGateTriggered")),
                "scores": _scores(summary, lint),
                "hardMetrics": _hard_metrics(summary, lint),
                "issues": [],
                "repairPlan": _repair_plan(decision, summary, lint),
                "shotSpecLint": lint,
                "sourceSummary": summary,
            }
        )
    return reports


def _windows(shot_list: list[dict[str, Any]]) -> list[dict[str, Any]]:
    out: list[dict[str, Any]] = []
    start = 0.0
    for idx, shot in enumerate(shot_list):
        shot_id = _string(_first_present(shot.get("shotId"), shot.get("id"))) or f"shot_{idx + 1:02d}"
        duration = _number(_first_present(shot.get("durationSec"), shot.get("duration")), 4)
        out.append({"shotId": shot_id, "start": start, "end": start + duration})
        start += duration
    return out


def _shot_id_at(windows: list[dict[str, Any]], time_sec: float) -> str:
    for window in windows:
        if time_sec >= window["start"] and time_sec < window["end"]:
            return str(window["shotId"])
    if windows and time_sec >= windows[-1]["end"]:
        return str(windows[-1]["shotId"])
    return ""


def _image_size(path: pathlib.Path) -> tuple[int, int]:
    data = json.loads(
        _run(
            [
                "ffprobe",
                "-v",
                "error",
                "-select_streams",
                "v:0",
                "-show_entries",
                "stream=width,height",
                "-of",
                "json",
                str(path),
            ],
            timeout=30,
        )
    )
    stream = data.get("streams", [{}])[0]
    return int(stream["width"]), int(stream["height"])


def _raw_rgb(path: pathlib.Path) -> tuple[int, int, bytes]:
    width, height = _image_size(path)
    data = _run_bytes(["ffmpeg", "-v", "error", "-i", str(path), "-f", "rawvideo", "-pix_fmt", "rgb24", "-"], timeout=60)
    return width, height, data


def _edge_density(data: bytes, width: int, height: int, rect: tuple[int, int, int, int]) -> float:
    x0, y0, x1, y1 = rect
    x0, y0 = max(0, x0), max(0, y0)
    x1, y1 = min(width, x1), min(height, y1)
    step = 4
    edges = 0
    total = 0

    def luma(x: int, y: int) -> float:
        idx = (y * width + x) * 3
        return 0.299 * data[idx] + 0.587 * data[idx + 1] + 0.114 * data[idx + 2]

    for y in range(y0, max(y0, y1 - step), step):
        for x in range(x0, max(x0, x1 - step), step):
            if abs(luma(x, y) - luma(x + step, y)) + abs(luma(x, y) - luma(x, y + step)) > 55:
                edges += 1
            total += 1
    return edges / total if total else 0


def _analyze_frame(path: pathlib.Path) -> dict[str, Any]:
    width, height, data = _raw_rgb(path)
    top_left = _edge_density(data, width, height, (0, 0, int(width * 0.42), int(height * 0.28)))
    lower = _edge_density(data, width, height, (0, int(height * 0.58), width, int(height * 0.94)))
    full = _edge_density(data, width, height, (0, 0, width, height))
    metrics = {
        "topLeftTextZoneEdgeDensity": _round(top_left),
        "lowerThirdEdgeDensity": _round(lower),
        "fullFrameEdgeDensity": _round(full),
    }
    issues: list[dict[str, Any]] = []
    if top_left >= 0.11:
        issues.append(
            {
                "code": "top_left_text_zone_crowded",
                "severity": "blocking",
                "zone": "top_left",
                "message": "左上文字安全区过于拥挤，容易出现品牌、镜头标签和源画面文字互相覆盖。",
                "suggestion": "该 shot 应减少左上角叠字，隐藏源截图文字，或把标题移到底部单一信息层。",
                "value": metrics["topLeftTextZoneEdgeDensity"],
                "threshold": 0.11,
            }
        )
    elif top_left >= 0.09:
        issues.append(
            {
                "code": "top_left_text_zone_busy",
                "severity": "warning",
                "zone": "top_left",
                "message": "左上文字安全区信息偏多。",
                "suggestion": "建议只保留品牌或镜头标签之一。",
                "value": metrics["topLeftTextZoneEdgeDensity"],
                "threshold": 0.09,
            }
        )
    if lower >= 0.095:
        issues.append(
            {
                "code": "lower_third_text_zone_crowded",
                "severity": "blocking",
                "zone": "lower_third",
                "message": "底部三分之一区域过于拥挤，标题、字幕或原片字幕可能互相覆盖。",
                "suggestion": "减少底部文案行数，裁掉原片字幕，或提高底部遮罩并保留单一字幕层。",
                "value": metrics["lowerThirdEdgeDensity"],
                "threshold": 0.095,
            }
        )
    elif lower >= 0.075:
        issues.append(
            {
                "code": "lower_third_text_zone_busy",
                "severity": "warning",
                "zone": "lower_third",
                "message": "底部三分之一区域信息密度偏高。",
                "suggestion": "建议压缩字幕行数或缩短标题。",
                "value": metrics["lowerThirdEdgeDensity"],
                "threshold": 0.075,
            }
        )
    if full >= 0.105:
        issues.append(
            {
                "code": "frame_visual_clutter_high",
                "severity": "warning",
                "zone": "full_frame",
                "message": "整帧视觉复杂度偏高，可能影响短视频观看理解。",
                "suggestion": "建议降低背景细节、放大主体、减少伪 UI 或伪文字元素。",
                "value": metrics["fullFrameEdgeDensity"],
                "threshold": 0.105,
            }
        )
    return {"passed": not any(issue["severity"] == "blocking" for issue in issues), "metrics": metrics, "issues": issues}


def _summaries(frames: list[dict[str, Any]]) -> list[dict[str, Any]]:
    order: list[str] = []
    by_shot: dict[str, dict[str, Any]] = {}
    for frame in frames:
        shot_id = str(frame.get("shotId") or "unmapped_shot")
        if shot_id not in by_shot:
            by_shot[shot_id] = {
                "shotId": shot_id,
                "frameCount": 0,
                "sampledTimesSec": [],
                "passed": True,
                "needsRegeneration": False,
                "blockingIssueCount": 0,
                "warningIssueCount": 0,
                "metricSummary": {},
                "recommendations": [],
                "representativeIssues": [],
                "_top": [],
                "_lower": [],
                "_full": [],
                "_issueCodes": set(),
            }
            order.append(shot_id)
        acc = by_shot[shot_id]
        acc["frameCount"] += 1
        acc["sampledTimesSec"].append(frame["timeSec"])
        acc["_top"].append(frame["metrics"]["topLeftTextZoneEdgeDensity"])
        acc["_lower"].append(frame["metrics"]["lowerThirdEdgeDensity"])
        acc["_full"].append(frame["metrics"]["fullFrameEdgeDensity"])
        for issue in frame.get("issues", []):
            if issue["severity"] == "blocking":
                acc["blockingIssueCount"] += 1
                acc["passed"] = False
                acc["needsRegeneration"] = True
            else:
                acc["warningIssueCount"] += 1
            if issue["code"] not in acc["_issueCodes"]:
                acc["representativeIssues"].append(issue)
                acc["_issueCodes"].add(issue["code"])
            suggestion = issue.get("suggestion", "")
            if suggestion and suggestion not in acc["recommendations"]:
                acc["recommendations"].append(suggestion)

    out: list[dict[str, Any]] = []
    for shot_id in order:
        acc = by_shot[shot_id]
        top, lower, full = acc.pop("_top"), acc.pop("_lower"), acc.pop("_full")
        acc.pop("_issueCodes")
        acc["metricSummary"] = {
            "avgTopLeftTextZoneEdgeDensity": _round(sum(top) / len(top)),
            "maxTopLeftTextZoneEdgeDensity": _round(max(top)),
            "avgLowerThirdEdgeDensity": _round(sum(lower) / len(lower)),
            "maxLowerThirdEdgeDensity": _round(max(lower)),
            "avgFullFrameEdgeDensity": _round(sum(full) / len(full)),
            "maxFullFrameEdgeDensity": _round(max(full)),
        }
        acc["score"] = max(0, 100 - acc["blockingIssueCount"] * 18 - acc["warningIssueCount"] * 5)
        if acc["blockingIssueCount"]:
            acc["decision"] = "RERENDER_HTML" if _issue_codes(acc) & {"top_left_text_zone_crowded", "lower_third_text_zone_crowded"} else "RECOMPOSITE"
            acc["repairAction"] = acc["decision"]
            acc["fatalGateTriggered"] = True
            acc["conclusion"] = f"{shot_id} 未通过：发现 {acc['blockingIssueCount']} 个阻断问题，建议返修该 shot。"
        elif acc["warningIssueCount"]:
            acc["decision"] = "PASS_WITH_FIX"
            acc["repairAction"] = "RERENDER_HTML"
            acc["fatalGateTriggered"] = False
            acc["conclusion"] = f"{shot_id} 通过但有 {acc['warningIssueCount']} 个警告，建议人工复看。"
        else:
            acc["decision"] = "PASS"
            acc["repairAction"] = "PASS"
            acc["fatalGateTriggered"] = False
            acc["conclusion"] = f"{shot_id} 通过：抽帧文字安全区和画面复杂度稳定。"
        out.append(acc)
    return out


def _repair_plan_from_summaries(summaries: list[dict[str, Any]]) -> dict[str, Any]:
    regenerate = [s["shotId"] for s in summaries if s.get("needsRegeneration")]
    review = [s["shotId"] for s in summaries if not s.get("needsRegeneration") and s.get("warningIssueCount")]
    decision_by_shot = {s["shotId"]: s.get("decision", "") for s in summaries}
    repair_by_shot = {s["shotId"]: s.get("repairAction", "") for s in summaries}
    action_counts: dict[str, int] = {}
    recommendations: list[str] = []
    for summary in summaries:
        action = str(summary.get("repairAction", ""))
        if action:
            action_counts[action] = action_counts.get(action, 0) + 1
        for recommendation in summary.get("recommendations", []):
            if recommendation and recommendation not in recommendations:
                recommendations.append(recommendation)
    if regenerate:
        next_action = "regenerate_shots"
        conclusion = f"{len(regenerate)} 个 shot 需要返修重生成：" + ", ".join(regenerate) + "。"
    elif review:
        next_action = "manual_review"
        conclusion = f"{len(review)} 个 shot 有警告，建议人工复看：" + ", ".join(review) + "。"
    else:
        next_action = "approve"
        conclusion = "所有 shot 抽帧 QA 通过，可以进入发布文案和交付。"
    return {
        "needsRegeneration": bool(regenerate),
        "nextAction": next_action,
        "conclusion": conclusion,
        "regenerateShotIds": regenerate,
        "manualReviewShotIds": review,
        "globalRecommendations": recommendations,
        "decisionByShot": decision_by_shot,
        "repairActionByShot": repair_by_shot,
        "repairActionCounts": action_counts,
        "regenerateShotCount": len(regenerate),
        "manualReviewShotCount": len(review),
        "totalEvaluatedShotCount": len(summaries),
    }


def _tile(frame_count: int) -> str:
    frame_count = max(1, min(frame_count, 16))
    best_cols, best_rows, best_waste = 1, frame_count, frame_count
    for cols in range(1, 5):
        rows = math.ceil(frame_count / cols)
        if rows > 4:
            continue
        waste = rows * cols - frame_count
        if waste < best_waste or (waste == best_waste and rows < best_rows) or (
            waste == best_waste and rows == best_rows and cols > best_cols
        ):
            best_cols, best_rows, best_waste = cols, rows, waste
    return f"{best_cols}x{best_rows}"


@mcp.tool()
def analyze_video(
    projectId: str,
    videoPath: str,
    outputDir: str,
    outputRefPrefix: str,
    videoRef: str = "",
    dataDir: str = "",
    sampleIntervalSec: float = 4,
    shotList: list[dict[str, Any]] | None = None,
    videoType: str = "",
    projectMode: str = "",
    profile: str = "",
    candidateId: str = "",
) -> dict[str, Any]:
    """Analyze a rendered video and produce shot-level QA artifacts."""
    project_id = projectId.strip()
    video_path = pathlib.Path(videoPath)
    output_dir = pathlib.Path(outputDir)
    frames_dir = output_dir / "frames"
    frames_dir.mkdir(parents=True, exist_ok=True)
    for old in frames_dir.glob("frame_*.png"):
        old.unlink()
    sample_interval = sampleIntervalSec if sampleIntervalSec > 0 else 4
    frame_pattern = frames_dir / "frame_%03d.png"
    _run(
        [
            "ffmpeg",
            "-y",
            "-v",
            "error",
            "-i",
            str(video_path),
            "-vf",
            f"fps=1/{sample_interval:g}",
            str(frame_pattern),
        ]
    )
    frame_paths = sorted(frames_dir.glob("frame_*.png"))
    if not frame_paths:
        raise RuntimeError("video_qa: no frames extracted")
    contact_sheet = output_dir / "contact_sheet.jpg"
    _run(
        [
            "ffmpeg",
            "-y",
            "-v",
            "error",
            "-i",
            str(video_path),
            "-vf",
            f"fps=1/{sample_interval:g},scale=480:-1,tile={_tile(len(frame_paths))}",
            "-frames:v",
            "1",
            str(contact_sheet),
        ]
    )
    shot_list = shotList or []
    windows = _windows(shot_list)
    frames: list[dict[str, Any]] = []
    blocking_count = 0
    warning_count = 0
    for idx, frame_path in enumerate(frame_paths):
        frame = _analyze_frame(frame_path)
        time_sec = idx * sample_interval
        frame.update(
            {
                "framePath": f"{outputRefPrefix}/frames/{frame_path.name}",
                "frameId": f"frame_{idx + 1:03d}",
                "timeSec": time_sec,
                "shotId": _shot_id_at(windows, time_sec),
            }
        )
        for issue in frame.get("issues", []):
            if issue["severity"] == "blocking":
                blocking_count += 1
            else:
                warning_count += 1
        frames.append(frame)

    spec_lints = build_shot_spec_lints(shot_list)
    shot_summaries = _summaries(frames)
    reports = build_shot_reports(project_id, videoType or projectMode or profile, candidateId, shot_summaries, spec_lints)
    for summary in shot_summaries:
        for report in reports:
            if report["shotId"] == summary["shotId"]:
                summary["decision"] = report["decision"]
                summary["repairAction"] = report["repairPlan"]["action"]
                summary["fatalGateTriggered"] = report["fatalGateTriggered"]
                break
    repair_plan = _repair_plan_from_summaries(shot_summaries)
    passed = blocking_count == 0 and not repair_plan["needsRegeneration"]
    score = max(0, 100 - blocking_count * 18 - warning_count * 5)
    if passed and warning_count == 0:
        summary_text = "抽帧视觉 QA 通过：未发现文字安全区拥挤或明显画面复杂度风险。"
    elif passed:
        summary_text = f"抽帧视觉 QA 通过但有 {warning_count} 个警告，建议人工复看联系表。"
    else:
        summary_text = f"抽帧视觉 QA 未通过：发现 {blocking_count} 个阻断问题、{warning_count} 个警告，需要调整对应 shot 后重新渲染。"

    report = {
        "passed": passed,
        "score": score,
        "frameCount": len(frames),
        "blockingIssueCount": blocking_count,
        "warningIssueCount": warning_count,
        "sampleIntervalSec": sample_interval,
        "summary": summary_text,
        "frames": frames,
        "shotCount": len(shot_summaries),
        "shotSpecLints": spec_lints,
        "shotSummaries": shot_summaries,
        "shotReports": reports,
        "needsRegeneration": repair_plan["needsRegeneration"],
        "repairPlan": repair_plan,
        "policy": {
            "topLeftTextZone": "品牌、镜头标签、源画面文字不能同时挤在左上安全区",
            "lowerThird": "主标题、字幕、原片字幕不能在底部重复叠加",
            "fullFrame": "画面复杂度过高时应减少文字、裁切或加深遮罩",
        },
    }
    report_path = output_dir / "video_frame_qa.json"
    shot_report_path = output_dir / "shot_qa_reports.json"
    repair_path = output_dir / "shot_repair_plan.json"
    report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    shot_report_path.write_text(
        json.dumps({"schemaVersion": 1, "projectId": project_id, "shotReports": reports}, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    repair_path.write_text(
        json.dumps({"schemaVersion": 1, "projectId": project_id, "repairPlan": repair_plan}, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    artifacts = [
        {
            "unitId": "video-frame-qa",
            "kind": "VIDEO_VISUAL_QA_REPORT",
            "name": "video_frame_qa.json",
            "storageType": "local",
            "storageRef": f"{outputRefPrefix}/video_frame_qa.json",
            "mimeType": "application/json",
            "sizeBytes": report_path.stat().st_size,
            "status": "valid",
            "humanApproved": False,
            "dependsOn": ["VIDEO"],
            "producedByTool": "video_frame_qa",
            "producedByRole": "视觉质量审核",
            "metadata": {
                "passed": passed,
                "score": score,
                "blockingIssueCount": blocking_count,
                "warningIssueCount": warning_count,
                "frameCount": len(frames),
                "shotCount": len(shot_summaries),
                "needsRegeneration": repair_plan["needsRegeneration"],
                "repairAction": repair_plan["nextAction"],
            },
        },
        {
            "unitId": "shot-qa-report",
            "kind": "SHOT_QA_REPORT",
            "name": "shot_qa_reports.json",
            "storageType": "local",
            "storageRef": f"{outputRefPrefix}/shot_qa_reports.json",
            "mimeType": "application/json",
            "sizeBytes": shot_report_path.stat().st_size,
            "status": "valid",
            "humanApproved": False,
            "dependsOn": ["VIDEO_VISUAL_QA_REPORT"],
            "producedByTool": "video_frame_qa",
            "producedByRole": "视觉质量审核",
            "metadata": {
                "shotCount": len(reports),
                "needsRegeneration": repair_plan["needsRegeneration"],
                "nextAction": repair_plan["nextAction"],
            },
        },
        {
            "unitId": "shot-repair-plan",
            "kind": "SHOT_REPAIR_PLAN",
            "name": "shot_repair_plan.json",
            "storageType": "local",
            "storageRef": f"{outputRefPrefix}/shot_repair_plan.json",
            "mimeType": "application/json",
            "sizeBytes": repair_path.stat().st_size,
            "status": "valid",
            "humanApproved": False,
            "dependsOn": ["SHOT_QA_REPORT"],
            "producedByTool": "video_frame_qa",
            "producedByRole": "视觉质量审核",
            "metadata": {"needsRegeneration": repair_plan["needsRegeneration"], "nextAction": repair_plan["nextAction"]},
        },
    ]
    if contact_sheet.exists() and contact_sheet.stat().st_size > 0:
        artifacts.append(
            {
                "unitId": "video-frame-qa-contact-sheet",
                "kind": "VIDEO_VISUAL_QA_CONTACT_SHEET",
                "name": "contact_sheet.jpg",
                "storageType": "local",
                "storageRef": f"{outputRefPrefix}/contact_sheet.jpg",
                "mimeType": "image/jpeg",
                "sizeBytes": contact_sheet.stat().st_size,
                "status": "valid",
                "humanApproved": False,
                "dependsOn": ["VIDEO_VISUAL_QA_REPORT"],
                "producedByTool": "video_frame_qa",
                "producedByRole": "视觉质量审核",
            }
        )
    return {
        "success": True,
        "passed": passed,
        "score": score,
        "summary": summary_text,
        "reportRef": f"{outputRefPrefix}/video_frame_qa.json",
        "contactSheetRef": f"{outputRefPrefix}/contact_sheet.jpg",
        "blockingIssueCount": blocking_count,
        "warningIssueCount": warning_count,
        "frames": frames,
        "shotCount": len(shot_summaries),
        "shotSpecLints": spec_lints,
        "shotSummaries": shot_summaries,
        "shotReports": reports,
        "needsRegeneration": repair_plan["needsRegeneration"],
        "repairPlan": repair_plan,
        "artifacts": artifacts,
    }


if __name__ == "__main__":
    mcp.run()
