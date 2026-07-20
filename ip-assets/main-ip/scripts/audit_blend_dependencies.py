"""Audit external dependencies of the currently opened Blender file.

This script is intentionally read-only with respect to the .blend file. Run it as:

    blender -b asset.blend --python audit_blend_dependencies.py -- --output report.json
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path
import sys

import bpy


def _dependency(kind: str, datablock: object) -> dict[str, object]:
    raw_path = str(getattr(datablock, "filepath", "") or "")
    absolute_path = str(Path(bpy.path.abspath(raw_path)).resolve()) if raw_path else ""
    packed_file = getattr(datablock, "packed_file", None)
    packed_files = getattr(datablock, "packed_files", None)
    is_packed = bool(packed_file) or bool(packed_files and len(packed_files) > 0)
    entry: dict[str, object] = {
        "kind": kind,
        "name": str(getattr(datablock, "name", "")),
        "source": str(getattr(datablock, "source", "")),
        "rawPath": raw_path,
        "absolutePath": absolute_path,
        "packed": is_packed,
        "exists": bool(absolute_path and Path(absolute_path).exists()),
    }
    if kind == "image":
        material_users = sorted(
            material.name
            for material in bpy.data.materials
            if material.use_nodes
            and any(
                getattr(node, "image", None) == datablock
                for node in material.node_tree.nodes
            )
        )
        entry["materialUsers"] = material_users
        entry["objectUsers"] = sorted(
            obj.name
            for obj in bpy.data.objects
            if any(
                slot.material is not None and slot.material.name in material_users
                for slot in obj.material_slots
            )
        )
    elif kind == "sound":
        sequence_users: list[str] = []
        for scene in bpy.data.scenes:
            editor = scene.sequence_editor
            if editor is None:
                continue
            strips = getattr(editor, "strips_all", None) or getattr(editor, "sequences_all", ())
            for strip in strips:
                if getattr(strip, "sound", None) == datablock:
                    sequence_users.append(f"{scene.name}:{strip.name}")
        entry["sequenceUsers"] = sorted(sequence_users)
    return entry


def _collect() -> list[dict[str, object]]:
    collections = (
        ("image", bpy.data.images),
        ("font", bpy.data.fonts),
        ("sound", bpy.data.sounds),
        ("movie_clip", bpy.data.movieclips),
        ("cache_file", bpy.data.cache_files),
        ("volume", bpy.data.volumes),
        ("library", bpy.data.libraries),
    )
    dependencies: list[dict[str, object]] = []
    for kind, datablocks in collections:
        for datablock in datablocks:
            entry = _dependency(kind, datablock)
            if entry["rawPath"] and not str(entry["rawPath"]).startswith("<"):
                dependencies.append(entry)
    return dependencies


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", required=True)
    args = parser.parse_args(sys.argv[sys.argv.index("--") + 1 :] if "--" in sys.argv else [])

    dependencies = _collect()
    external_unpacked = [item for item in dependencies if not item["packed"]]
    missing = [item for item in external_unpacked if not item["exists"]]
    payload = {
        "schemaVersion": "tangying-blend-dependency-audit/v1",
        "blendFile": str(Path(bpy.data.filepath).resolve()),
        "blenderVersion": bpy.app.version_string,
        "dependencies": dependencies,
        "externalUnpacked": external_unpacked,
        "missingExternal": missing,
        "safeToRemoveUnreferencedSidecars": not missing,
    }
    output = Path(args.output).expanduser().resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"output": str(output), "dependencyCount": len(dependencies), "missingCount": len(missing)}))


if __name__ == "__main__":
    main()
