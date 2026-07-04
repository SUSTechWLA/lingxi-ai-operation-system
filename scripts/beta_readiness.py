#!/usr/bin/env python3
"""Closed beta readiness evaluator.

The evaluator is intentionally deterministic and dependency-free so it can run
inside CI, local developer machines, and support workflows without provider keys.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any


def evaluate(snapshot: dict[str, Any], require_real_aigc: bool = False) -> dict[str, Any]:
    blocking: list[str] = []
    warnings: list[str] = []
    actions: list[str] = []

    smoke_passed = bool(_dig(snapshot, "smoke", "passed"))
    local_agent_ok = _status_ok(snapshot.get("localAgent"))
    hyperframes_ok = _status_ok(snapshot.get("hyperframes"))
    ffmpeg_available = bool(_dig(snapshot, "ffmpeg", "available"))
    diagnostics_available = bool(_dig(snapshot, "diagnostics", "available"))
    qa_ok = _qa_gate_ok(snapshot.get("qaFixture"))
    real_aigc = _has_healthy_aigc_video_provider(snapshot.get("mcpProviders"))
    text_model = _model_configured(snapshot.get("modelProviders"), "text_to_text")
    video_model = _model_configured(snapshot.get("modelProviders"), "text_to_video")

    if not smoke_passed:
        blocking.append("beta smoke did not pass")
        actions.append("Run bash scripts/beta-smoke-check.sh and fix all failures.")
    if not ffmpeg_available:
        blocking.append("FFmpeg is not available")
        actions.append("Install FFmpeg and restart local agent and HyperFrames service.")
    if not qa_ok:
        blocking.append("structured shot QA report or repairPlan is unavailable")
        actions.append("Run bash scripts/beta-fallback-fixture.sh and inspect shot_qa_reports.json.")
    if not diagnostics_available:
        blocking.append("diagnostics export is unavailable")
        actions.append("Start local agent and verify POST /api/local/diagnostics returns beta-diagnostics.zip.")
    if require_real_aigc and not real_aigc:
        blocking.append("no healthy AIGC video provider is available")
        actions.append("Configure and preflight JiMeng/Dreamina MCP provider before inviting real AIGC testers.")
    if require_real_aigc and not text_model:
        blocking.append("text_to_text model provider is not configured")
        actions.append("Configure an OpenAI-compatible text model provider in local settings.")
    if require_real_aigc and not video_model:
        blocking.append("text_to_video model provider is not configured")
        actions.append("Configure or verify the video model/provider route used for AIGC planning.")

    if not local_agent_ok:
        warnings.append("local agent health was not confirmed")
        actions.append("Start local agent on http://127.0.0.1:18080 before creator trials.")
    if not hyperframes_ok:
        warnings.append("HyperFrames render service health was not confirmed")
        actions.append("Start HyperFrames render service and verify /health.")
    if not real_aigc:
        warnings.append("real AIGC video provider is not healthy; beta is fallback-preview only")
        actions.append("Use fallback smoke for engineering validation only; do not promise real AIGC quality.")
    if not text_model:
        warnings.append("text_to_text model provider was not confirmed")
    if not video_model:
        warnings.append("text_to_video provider route was not confirmed")

    if blocking:
        decision = "BLOCKED"
    elif real_aigc and text_model and video_model and local_agent_ok and hyperframes_ok:
        decision = "GO"
    else:
        decision = "CONDITIONAL"

    capabilities = {
        "fallbackPreview": smoke_passed and qa_ok,
        "diagnostics": diagnostics_available,
        "structuredShotQa": qa_ok,
        "realAigc": real_aigc,
        "localAgent": local_agent_ok,
        "hyperframes": hyperframes_ok,
        "ffmpeg": ffmpeg_available,
        "oneSentenceHighQuality": decision == "GO",
    }

    summary = _summary(decision, capabilities, require_real_aigc)
    return {
        "schemaVersion": 1,
        "decision": decision,
        "summary": summary,
        "blockingIssues": _unique(blocking),
        "warnings": _unique(warnings),
        "nextActions": _unique(actions),
        "capabilities": capabilities,
    }


def _summary(decision: str, capabilities: dict[str, bool], require_real_aigc: bool) -> str:
    if decision == "GO":
        return "Ready for controlled real AIGC creator trials with one-sentence high-quality video claims."
    if decision == "BLOCKED":
        return "Not ready for closed beta; fix blocking issues before inviting users."
    if capabilities.get("fallbackPreview"):
        if require_real_aigc:
            return "Fallback path works, but real AIGC readiness is incomplete."
        return "Ready only for controlled fallback-preview engineering trials, not for high-quality real AIGC promises."
    return "Only partial local readiness is confirmed."


def _has_healthy_aigc_video_provider(value: Any) -> bool:
    if not isinstance(value, list):
        return False
    for provider in value:
        if not isinstance(provider, dict) or not _status_ok(provider):
            continue
        capabilities = _lower_strings(provider.get("capabilities"))
        tools = _lower_strings(provider.get("tools"))
        source_types = _lower_strings(provider.get("sourceTypes"))
        if "aigc_video" in capabilities or "aigc_video" in source_types:
            return True
        if any("generate_video" in tool or "video" in tool for tool in tools):
            return True
    return False


def _model_configured(value: Any, key: str) -> bool:
    if not isinstance(value, dict):
        return False
    provider = value.get(key)
    if isinstance(provider, bool):
        return provider
    if not isinstance(provider, dict):
        return False
    if "configured" in provider:
        return bool(provider.get("configured"))
    return bool(provider.get("baseUrl") and provider.get("model"))


def _qa_gate_ok(value: Any) -> bool:
    if not isinstance(value, dict) or not bool(value.get("passed")):
        return False
    return int(value.get("structuredShotReports") or 0) > 0 and bool(value.get("repairPlanAvailable"))


def _status_ok(value: Any) -> bool:
    if not isinstance(value, dict):
        return False
    return str(value.get("status", "")).lower() in {"ok", "healthy", "ready", "pass", "passed"}


def _dig(value: dict[str, Any], *keys: str) -> Any:
    current: Any = value
    for key in keys:
        if not isinstance(current, dict):
            return None
        current = current.get(key)
    return current


def _lower_strings(value: Any) -> set[str]:
    if not isinstance(value, list):
        return set()
    return {str(item).lower() for item in value if item is not None}


def _unique(items: list[str]) -> list[str]:
    seen: set[str] = set()
    result: list[str] = []
    for item in items:
        if item in seen:
            continue
        seen.add(item)
        result.append(item)
    return result


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Evaluate Tangying AIOS closed beta readiness.")
    parser.add_argument("--input", help="Readiness snapshot JSON file. Reads stdin when omitted.")
    parser.add_argument("--output", help="Write readiness report JSON to this file.")
    parser.add_argument("--require-real-aigc", action="store_true", help="Block unless real AIGC video provider gates pass.")
    args = parser.parse_args(argv)

    if args.input:
        snapshot = json.loads(Path(args.input).read_text(encoding="utf-8"))
    else:
        snapshot = json.load(sys.stdin)

    report = evaluate(snapshot, require_real_aigc=args.require_real_aigc)
    encoded = json.dumps(report, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        Path(args.output).parent.mkdir(parents=True, exist_ok=True)
        Path(args.output).write_text(encoded, encoding="utf-8")
    print(encoded, end="")
    return 1 if report["decision"] == "BLOCKED" else 0


if __name__ == "__main__":
    raise SystemExit(main())
