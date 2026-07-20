#!/usr/bin/env python3
"""Portable integrity contract for the bundled default A-roll assets."""

from __future__ import annotations

import hashlib
import json
import string
from pathlib import Path
from typing import Any, Optional


SCHEMA_VERSION = "tangying-default-aroll-assets/v1"
REQUIRED_ROLES = ("characterMaster", "studioTemplate")


def sha256_file(path: Path) -> str:
    """Return the lowercase SHA-256 digest of a file."""

    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _is_sha256(value: Any) -> bool:
    return (
        isinstance(value, str)
        and len(value) == 64
        and all(character in string.hexdigits for character in value)
    )


def validate_manifest(
    path: Path,
    repo_root: Optional[Path] = None,
) -> dict[str, Any]:
    """Validate schema, portability, existence, separation, and file hashes.

    ``repo_root`` is the directory against which asset paths are resolved. It
    should be the character-profile directory for the bundled manifest.
    """

    manifest_path = Path(path).expanduser().resolve()
    root = Path(repo_root).expanduser().resolve() if repo_root else manifest_path.parent
    errors: list[str] = []
    resolved_assets: dict[str, dict[str, Any]] = {}

    if not manifest_path.is_file():
        return {
            "success": False,
            "manifestPath": str(manifest_path),
            "repoRoot": str(root),
            "errors": [f"manifest does not exist: {manifest_path}"],
            "assets": resolved_assets,
        }

    try:
        payload = json.loads(manifest_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        return {
            "success": False,
            "manifestPath": str(manifest_path),
            "repoRoot": str(root),
            "errors": [f"manifest is not readable JSON: {exc}"],
            "assets": resolved_assets,
        }

    if not isinstance(payload, dict):
        errors.append("manifest root must be an object")
        payload = {}
    if payload.get("schemaVersion") != SCHEMA_VERSION:
        errors.append(f"schemaVersion must be {SCHEMA_VERSION}")

    asset_specs = payload.get("assets")
    if not isinstance(asset_specs, dict):
        errors.append("assets must be an object")
        asset_specs = {}

    resolved_paths: dict[str, Path] = {}
    for role in REQUIRED_ROLES:
        spec = asset_specs.get(role)
        if not isinstance(spec, dict):
            errors.append(f"assets.{role} is required")
            continue

        raw_path = spec.get("path")
        if not isinstance(raw_path, str) or not raw_path.strip():
            errors.append(f"{role}.path must be a non-empty string")
            continue
        relative_path = Path(raw_path)
        if relative_path.is_absolute():
            errors.append(f"{role}.path must be repository-relative")
            continue

        resolved_path = (root / relative_path).resolve()
        try:
            resolved_path.relative_to(root)
        except ValueError:
            errors.append(f"{role}.path resolves outside repository root")
            continue

        resolved_paths[role] = resolved_path
        expected_hash = spec.get("sha256")
        asset_result: dict[str, Any] = {
            "path": str(resolved_path),
            "relativePath": relative_path.as_posix(),
            "sha256": "",
            "exists": resolved_path.is_file(),
        }
        resolved_assets[role] = asset_result

        if resolved_path.suffix.lower() != ".blend":
            errors.append(f"{role}.path must point to a Blender .blend file")
        if not resolved_path.is_file():
            errors.append(f"{role}.path does not exist: {resolved_path}")
            continue
        if not _is_sha256(expected_hash):
            errors.append(f"{role}.sha256 must be 64 hexadecimal characters")
            continue

        actual_hash = sha256_file(resolved_path)
        asset_result["sha256"] = actual_hash
        asset_result["bytes"] = resolved_path.stat().st_size
        if actual_hash != str(expected_hash).lower():
            errors.append(f"{role}.sha256 does not match file")

    character_path = resolved_paths.get("characterMaster")
    studio_path = resolved_paths.get("studioTemplate")
    if character_path is not None and studio_path is not None and character_path == studio_path:
        errors.append(
            "characterMaster and studioTemplate must resolve to different files"
        )

    return {
        "success": not errors,
        "manifestPath": str(manifest_path),
        "repoRoot": str(root),
        "schemaVersion": payload.get("schemaVersion", ""),
        "characterId": payload.get("characterId", ""),
        "releaseId": payload.get("releaseId", ""),
        "errors": errors,
        "assets": resolved_assets,
    }


__all__ = [
    "REQUIRED_ROLES",
    "SCHEMA_VERSION",
    "sha256_file",
    "validate_manifest",
]
