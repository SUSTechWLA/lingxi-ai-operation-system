#!/usr/bin/env python3
"""Render one deterministic evidence frame from the published default pair."""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
import time
import traceback
from pathlib import Path
from typing import Any

import bpy


FORMAL_COLLECTION = "COL_CHR_SLOTH_FINAL"
MASTER_COLLECTION = "IP_Character_Master"
SCHEMA_VERSION = "tangying-default-aroll-smoke/v1"


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--master", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--report", required=True)
    argv = sys.argv[sys.argv.index("--") + 1 :] if "--" in sys.argv else []
    return parser.parse_args(argv)


def write_report(path: Path, payload: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8")


def main() -> None:
    args = parse_args()
    master_path = Path(args.master).expanduser().resolve()
    studio_path = Path(bpy.data.filepath).expanduser().resolve()
    output_path = Path(args.output).expanduser().resolve()
    report_path = Path(args.report).expanduser().resolve()
    output_path.parent.mkdir(parents=True, exist_ok=True)
    report_path.parent.mkdir(parents=True, exist_ok=True)
    studio_sha256 = sha256_file(studio_path)
    started = time.perf_counter()

    try:
        if bpy.data.collections.get(FORMAL_COLLECTION) is not None:
            raise RuntimeError("studio already contains COL_CHR_SLOTH_FINAL")
        if bpy.data.collections.get(MASTER_COLLECTION) is not None:
            raise RuntimeError("studio already contains IP_Character_Master")
        spawn = bpy.data.objects.get("IP_Standing_Spawn") or bpy.data.objects.get(
            "IP_Character_Spawn"
        )
        if spawn is None:
            raise RuntimeError("studio is missing a standing character spawn marker")

        repo_root = Path(__file__).resolve().parents[3]
        provider_dir = repo_root / "mcp/ip_avatar_3d"
        if str(provider_dir) not in sys.path:
            sys.path.insert(0, str(provider_dir))
        import blender_renderer

        imported_assets = blender_renderer.append_runtime_master_collection(
            master_path,
            bpy.context.scene.collection,
        )
        meshes = [obj for obj in imported_assets if obj.type == "MESH"]
        armatures = [obj for obj in imported_assets if obj.type == "ARMATURE"]
        if not meshes:
            raise RuntimeError("published master contains no mesh objects")
        if len(armatures) != 1:
            raise RuntimeError("published master must append exactly one Armature")

        target_height = float(spawn.get("target_height", 2.55))
        dimensions = blender_renderer.prepare_character(
            meshes,
            target_height=target_height,
            preserve_hierarchy=True,
            asset_objects=imported_assets,
        )
        container = dimensions.get("container")
        if container is None:
            raise RuntimeError("smoke render could not create a character container")
        container.location += spawn.matrix_world.translation
        bpy.context.view_layer.update()

        scene = bpy.context.scene
        camera = (
            bpy.data.objects.get("Camera_Standing_Medium")
            or bpy.data.objects.get("Camera_Medium")
        )
        if camera is None or camera.type != "CAMERA":
            raise RuntimeError("studio is missing a usable medium camera")
        scene.camera = camera
        try:
            scene.render.engine = "BLENDER_EEVEE_NEXT"
        except TypeError:
            scene.render.engine = "BLENDER_EEVEE"
        scene.render.resolution_x = 640
        scene.render.resolution_y = 360
        scene.render.resolution_percentage = 100
        scene.render.image_settings.file_format = "PNG"
        scene.render.film_transparent = False
        scene.render.filepath = str(output_path)
        scene.view_settings.view_transform = "AgX"
        scene.frame_set(1)

        bpy.ops.render.render(write_still=True)
        if not output_path.is_file() or output_path.read_bytes()[:8] != b"\x89PNG\r\n\x1a\n":
            raise RuntimeError("smoke render did not produce a valid PNG")
        rendered_image = bpy.data.images.load(str(output_path), check_existing=False)
        try:
            dimensions_px = [int(value) for value in rendered_image.size]
        finally:
            bpy.data.images.remove(rendered_image)

        formal_count = sum(
            collection.name == FORMAL_COLLECTION for collection in bpy.data.collections
        )
        master_count = sum(
            collection.name == MASTER_COLLECTION for collection in bpy.data.collections
        )
        armature_count = sum(obj.type == "ARMATURE" for obj in bpy.data.objects)
        errors: list[str] = []
        if formal_count != 1:
            errors.append("render scene must contain exactly one COL_CHR_SLOTH_FINAL")
        if master_count != 1:
            errors.append("render scene must contain exactly one IP_Character_Master")
        if armature_count != 1:
            errors.append("render scene must contain exactly one Armature")
        if dimensions_px != [640, 360]:
            errors.append(f"render dimensions must be 640x360, got {dimensions_px}")

        result = {
            "schemaVersion": SCHEMA_VERSION,
            "status": "PASS" if not errors else "FAIL",
            "productionReady": not errors,
            "errors": errors,
            "masterPath": str(master_path),
            "masterSha256": sha256_file(master_path),
            "studioPath": str(studio_path),
            "studioSha256": studio_sha256,
            "studioSourceUnchanged": sha256_file(studio_path) == studio_sha256,
            "outputPath": str(output_path),
            "outputSha256": sha256_file(output_path),
            "outputBytes": output_path.stat().st_size,
            "dimensions": dimensions_px,
            "camera": camera.name,
            "presentationMode": "standing",
            "cameraPreset": "front_talking",
            "engine": scene.render.engine,
            "viewTransform": scene.view_settings.view_transform,
            "look": scene.view_settings.look,
            "formalCollectionCount": formal_count,
            "masterCollectionCount": master_count,
            "armatureCount": armature_count,
            "characterMeshCount": len(meshes),
            "renderSeconds": time.perf_counter() - started,
            "blenderVersion": bpy.app.version_string,
            "pythonVersion": sys.version.split()[0],
        }
        write_report(report_path, result)
        if errors:
            raise RuntimeError("; ".join(errors))
        print(json.dumps({"status": "PASS", "report": str(report_path)}))
    except Exception as exc:
        failure = {
            "schemaVersion": SCHEMA_VERSION,
            "status": "FAIL",
            "productionReady": False,
            "error": str(exc),
            "traceback": traceback.format_exc(),
            "masterPath": str(master_path),
            "studioPath": str(studio_path),
            "outputPath": str(output_path),
        }
        write_report(report_path, failure)
        raise


if __name__ == "__main__":
    main()
