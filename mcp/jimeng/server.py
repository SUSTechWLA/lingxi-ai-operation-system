#!/usr/bin/env python3
"""Standard MCP stdio server for Dreamina/JiMeng CLI.

This server intentionally logs nothing to stdout. MCP stdio reserves stdout for
JSON-RPC messages; use stderr logging only when diagnostics are needed.
"""

from __future__ import annotations

import json
import os
import subprocess
import sys
from typing import Any

try:
    from mcp.server.fastmcp import FastMCP
except ImportError as fastmcp_error:  # pragma: no cover - compatibility with newer/alternate MCP SDKs
    try:
        from mcp.server import MCPServer as FastMCP  # type: ignore
    except ImportError as mcp_error:
        print(
            "Missing Python MCP SDK. Install with: python3 -m pip install -r mcp/jimeng/requirements.txt",
            file=sys.stderr,
        )
        raise mcp_error from fastmcp_error


mcp = FastMCP("JiMeng MCP")
DREAMINA_COMMAND = os.environ.get("DREAMINA_COMMAND", "dreamina")


def _run_dreamina(args: list[str], timeout: int = 600) -> str:
    completed = subprocess.run(
        [DREAMINA_COMMAND, *args],
        check=False,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=timeout,
    )
    if completed.returncode != 0:
        detail = completed.stderr.strip() or completed.stdout.strip() or f"exit {completed.returncode}"
        raise RuntimeError(detail)
    return completed.stdout.strip()


def _parse_object(stdout: str) -> dict[str, Any]:
    text = stdout.strip()
    if not text:
        raise RuntimeError("dreamina output is empty")
    start = text.find("{")
    end = text.rfind("}")
    if start >= 0 and end >= start:
        return json.loads(text[start : end + 1])
    lowered = text.lower()
    if (
        "已复用当前本地 oauth 登录态" in lowered
        or "已登录" in lowered
        or "already logged in" in lowered
        or "already authenticated" in lowered
    ):
        return {"status": "authenticated", "loggedIn": True, "reused": True, "message": text.splitlines()[0]}
    pairs: dict[str, Any] = {}
    for line in text.splitlines():
        if "=" in line:
            key, value = line.split("=", 1)
            pairs[key.strip()] = value.strip()
    if pairs:
        return pairs
    raise RuntimeError(f"dreamina output is not JSON: {text.splitlines()[0]}")


def _append_option(args: list[str], name: str, value: Any) -> None:
    if value is None or value == "":
        return
    if isinstance(value, bool):
        if value:
            args.append(f"--{name}")
        return
    args.append(f"--{name}={value}")


def _normalize_duration(duration: int) -> int:
    if duration <= 0:
        return 5
    return min(max(duration, 4), 15)


def _normalize_video_resolution(video_resolution: str) -> str:
    value = video_resolution.strip().lower().replace(" ", "").replace("*", "x")
    if value in {"3840x2160", "2160p", "4k", "uhd"}:
        return "4k"
    if value in {"1920x1080", "1080p", "fullhd", "fhd"}:
        return "1080p"
    if value in {"1280x720", "720p", "hd"}:
        return "720p"
    return video_resolution.strip()


def _default_video_model_version(model_version: str, video_resolution: str) -> str:
    if model_version.strip():
        return model_version.strip()
    if video_resolution in {"1080p", "4k"}:
        return "seedance2.0_vip"
    return ""


def _generation_result(stdout: str) -> dict[str, Any]:
    data = _parse_object(stdout)
    status = data.get("gen_status") or data.get("genStatus") or data.get("status")
    submit_id = data.get("submit_id") or data.get("submitId")
    fail_reason = data.get("fail_reason") or data.get("failReason") or data.get("message") or data.get("error")
    if status in {"fail", "failed", "error"}:
        raise RuntimeError(f"dreamina generation failed: {fail_reason or 'unknown error'}")
    if not submit_id:
        raise RuntimeError("dreamina output missing submit_id")
    if status not in {"querying", "success"}:
        raise RuntimeError(f"dreamina output has unsupported gen_status {status!r}")
    return data


@mcp.tool()
def check_status() -> dict[str, Any]:
    """Check whether the local Dreamina CLI can be invoked."""
    stdout = _run_dreamina(["version"], timeout=30)
    return {"available": True, "command": DREAMINA_COMMAND, "version": stdout.strip()}


@mcp.tool()
def inspect_command(command: str = "") -> dict[str, Any]:
    """Return Dreamina CLI help text for a command."""
    args = [command, "-h"] if command else ["-h"]
    return {"command": command, "help": _run_dreamina(args, timeout=30)}


@mcp.tool()
def login_headless() -> dict[str, Any]:
    """Start or reuse Dreamina headless OAuth login."""
    return _parse_object(_run_dreamina(["login", "--headless"], timeout=120))


@mcp.tool()
def check_login(device_code: str, poll: int = 30) -> dict[str, Any]:
    """Check Dreamina OAuth login completion."""
    return _parse_object(_run_dreamina(["login", "checklogin", f"--device_code={device_code}", f"--poll={poll}"], timeout=180))


@mcp.tool()
def generate_image(
    prompt: str,
    mode: str = "text2image",
    images: list[str] | None = None,
    ratio: str = "",
    resolution_type: str = "",
    model_version: str = "",
    generate_num: int = 0,
    poll: int = 0,
) -> dict[str, Any]:
    """Generate images through the user-managed Dreamina CLI."""
    args = [mode]
    if images:
        args.append("--images=" + ",".join(images))
    _append_option(args, "prompt", prompt)
    _append_option(args, "ratio", ratio)
    _append_option(args, "resolution_type", resolution_type)
    _append_option(args, "model_version", model_version)
    _append_option(args, "generate_num", generate_num if generate_num > 0 else None)
    _append_option(args, "poll", poll if poll > 0 else None)
    return _generation_result(_run_dreamina(args))


@mcp.tool()
def generate_video(
    prompt: str,
    mode: str = "text2video",
    image: str = "",
    images: list[str] | None = None,
    video: str = "",
    audio: str = "",
    duration: int = 0,
    ratio: str = "",
    video_resolution: str = "",
    model_version: str = "",
    poll: int = 0,
) -> dict[str, Any]:
    """Generate videos through the user-managed Dreamina CLI."""
    duration = _normalize_duration(duration)
    video_resolution = _normalize_video_resolution(video_resolution)
    model_version = _default_video_model_version(model_version, video_resolution)
    if not ratio.strip():
        ratio = "16:9"
    args = [mode]
    _append_option(args, "image", image)
    if images:
        args.append("--images=" + ",".join(images))
    _append_option(args, "video", video)
    _append_option(args, "audio", audio)
    _append_option(args, "prompt", prompt)
    _append_option(args, "duration", duration)
    _append_option(args, "ratio", ratio)
    _append_option(args, "video_resolution", video_resolution)
    _append_option(args, "model_version", model_version)
    _append_option(args, "poll", poll if poll > 0 else None)
    return _generation_result(_run_dreamina(args))


@mcp.tool()
def query_result(submit_id: str, download_dir: str = "") -> dict[str, Any]:
    """Query or download a Dreamina generation result."""
    args = ["query_result", f"--submit_id={submit_id}"]
    _append_option(args, "download_dir", download_dir)
    return _generation_result(_run_dreamina(args))


@mcp.tool()
def list_task(gen_status: str = "") -> dict[str, Any]:
    """List Dreamina generation history."""
    args = ["list_task"]
    _append_option(args, "gen_status", gen_status)
    return _parse_object(_run_dreamina(args, timeout=120))


if __name__ == "__main__":
    mcp.run()
