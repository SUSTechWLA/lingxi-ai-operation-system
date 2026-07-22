#!/usr/bin/env python3
"""Repository-level MCP registration and tool-schema contract smoke test.

The test launches each bundled server over real stdio using the official MCP
Python SDK. It intentionally stops after initialize and tools/list so it never
invokes Blender, FFmpeg, Dreamina, a network service, or a generation model.
"""

from __future__ import annotations

import asyncio
import os
import re
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any

try:
    from jsonschema.exceptions import SchemaError
    from jsonschema.validators import validator_for
    from mcp import ClientSession, StdioServerParameters
    from mcp.client.stdio import stdio_client
except ImportError as error:  # pragma: no cover - exercised by CI setup failures.
    raise SystemExit(
        "The official Python MCP SDK is required. Install mcp>=1.27,<2 before "
        "running scripts/test_mcp_contracts.py."
    ) from error


REPO_ROOT = Path(__file__).resolve().parents[1]
SDK_REQUIREMENT = "mcp>=1.27,<2"
SERVER_TIMEOUT_SECONDS = 30


@dataclass(frozen=True)
class ServerContract:
    name: str
    script: Path
    requirement: Path
    expected_tools: frozenset[str]


SERVER_CONTRACTS = (
    ServerContract(
        name="jimeng",
        script=REPO_ROOT / "mcp/jimeng/server.py",
        requirement=REPO_ROOT / "mcp/jimeng/requirements.txt",
        expected_tools=frozenset(
            {
                "check_status",
                "inspect_command",
                "login_headless",
                "check_login",
                "generate_image",
                "generate_video",
                "query_result",
                "list_task",
            }
        ),
    ),
    ServerContract(
        name="video_qa",
        script=REPO_ROOT / "mcp/video_qa/server.py",
        requirement=REPO_ROOT / "mcp/video_qa/requirements.txt",
        expected_tools=frozenset({"analyze_video"}),
    ),
    ServerContract(
        name="ip_avatar_3d",
        script=REPO_ROOT / "mcp/ip_avatar_3d/server.py",
        requirement=REPO_ROOT / "mcp/ip_avatar_3d/requirements.txt",
        expected_tools=frozenset(
            {
                "check_status",
                "check_gpt_sovits_voice",
                "generate_voice_auditions",
                "validate_character_asset",
                "prepare_character_master",
                "record_character_master_visual_inspection",
                "create_character_master_publication_report",
                "publish_character_master",
                "list_aroll_actions",
                "plan_motion",
                "render_talking_video",
            }
        ),
    ),
)


def _assert_tool_schema(server_name: str, tool: Any) -> None:
    name = getattr(tool, "name", "")
    description = getattr(tool, "description", "")
    schema = getattr(tool, "inputSchema", None)

    if not isinstance(name, str) or not re.fullmatch(r"[A-Za-z0-9_.-]+", name):
        raise AssertionError(f"{server_name}: invalid MCP tool name {name!r}")
    if not isinstance(description, str) or not description.strip():
        raise AssertionError(f"{server_name}/{name}: description must be non-empty")
    if not isinstance(schema, dict) or schema.get("type") != "object":
        raise AssertionError(f"{server_name}/{name}: inputSchema must be a type=object JSON Schema")
    try:
        validator_for(schema).check_schema(schema)
    except SchemaError as error:
        raise AssertionError(f"{server_name}/{name}: inputSchema is not valid JSON Schema: {error}") from error

    properties = schema.get("properties", {})
    required = schema.get("required", [])
    if not isinstance(properties, dict):
        raise AssertionError(f"{server_name}/{name}: inputSchema.properties must be an object")
    if not isinstance(required, list) or not all(isinstance(item, str) for item in required):
        raise AssertionError(f"{server_name}/{name}: inputSchema.required must be a string array")
    missing = set(required) - set(properties)
    if missing:
        raise AssertionError(f"{server_name}/{name}: required fields lack schemas: {sorted(missing)}")
    for property_name, property_schema in properties.items():
        if not isinstance(property_name, str) or not isinstance(property_schema, dict):
            raise AssertionError(f"{server_name}/{name}: invalid property schema for {property_name!r}")


async def _list_all_tools(session: ClientSession) -> list[Any]:
    tools: list[Any] = []
    cursor: str | None = None
    seen_cursors: set[str] = set()
    while True:
        page = await session.list_tools(cursor=cursor)
        tools.extend(page.tools)
        cursor = page.nextCursor
        if not cursor:
            return tools
        if cursor in seen_cursors:
            raise AssertionError(f"tools/list returned repeated pagination cursor {cursor!r}")
        seen_cursors.add(cursor)


async def _smoke_server(contract: ServerContract) -> None:
    if not contract.script.is_file():
        raise AssertionError(f"{contract.name}: missing server script {contract.script}")

    # The registration smoke needs no provider credentials. Only pass the
    # process settings needed to launch Python so local secrets cannot reach a
    # bundled server during this test.
    environment = {
        "PATH": os.environ.get("PATH", ""),
        "PYTHONIOENCODING": "utf-8",
        "PYTHONUNBUFFERED": "1",
    }
    parameters = StdioServerParameters(
        command=sys.executable,
        args=[str(contract.script)],
        cwd=str(REPO_ROOT),
        env=environment,
    )
    async with stdio_client(parameters) as (read_stream, write_stream):
        async with ClientSession(read_stream, write_stream) as session:
            initialized = await session.initialize()
            protocol_version = getattr(initialized, "protocolVersion", "")
            if not isinstance(protocol_version, str) or not protocol_version:
                raise AssertionError(f"{contract.name}: initialize returned no protocolVersion")

            tools = await _list_all_tools(session)
            if not tools:
                raise AssertionError(f"{contract.name}: tools/list returned no tools")
            for tool in tools:
                _assert_tool_schema(contract.name, tool)

            actual_names = {tool.name for tool in tools}
            missing = contract.expected_tools - actual_names
            if missing:
                raise AssertionError(f"{contract.name}: tools/list missing {sorted(missing)}")

            print(
                f"PASS {contract.name}: protocol={protocol_version}, "
                f"tools={len(actual_names)}"
            )


def _assert_repository_wiring() -> None:
    for contract in SERVER_CONTRACTS:
        requirement = contract.requirement.read_text(encoding="utf-8").strip()
        if requirement != SDK_REQUIREMENT:
            raise AssertionError(
                f"{contract.requirement.relative_to(REPO_ROOT)} must contain exactly "
                f"{SDK_REQUIREMENT!r}; got {requirement!r}"
            )

    workflow = (REPO_ROOT / ".github/workflows/ci.yml").read_text(encoding="utf-8")
    test_job_match = re.search(
        r"(?ms)^  test:\s*$\n(?P<body>.*?)(?=^  [A-Za-z0-9_-]+:\s*$|\Z)",
        workflow,
    )
    if not test_job_match:
        raise AssertionError("CI workflow must define a test job")
    test_job = test_job_match.group("body")
    if not re.search(r"uses:\s*actions/setup-python@v\d+", test_job):
        raise AssertionError("CI test job must install Python with actions/setup-python")
    if SDK_REQUIREMENT not in test_job:
        raise AssertionError(f"CI must install the official SDK constraint {SDK_REQUIREMENT}")
    if "python3 scripts/test_mcp_contracts.py" not in test_job:
        raise AssertionError("CI must run scripts/test_mcp_contracts.py")


async def _main() -> None:
    for contract in SERVER_CONTRACTS:
        await asyncio.wait_for(_smoke_server(contract), timeout=SERVER_TIMEOUT_SECONDS)
    _assert_repository_wiring()
    print("PASS repository MCP dependency and CI wiring")


if __name__ == "__main__":
    asyncio.run(_main())
