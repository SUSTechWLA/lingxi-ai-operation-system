#!/usr/bin/env python3
"""Validate the production contract of the empty warm-sloth Blender studio."""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import sys
from pathlib import Path
from typing import Any, Iterable

try:
    import bpy
except ModuleNotFoundError as exc:  # pragma: no cover - exercised outside Blender
    raise RuntimeError("validate_warm_studio.py must run inside Blender") from exc

from mathutils import Vector

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import warm_sloth_studio_builder as builder
import warm_studio_contract as contract


SCHEMA_VERSION = "tangying-warm-studio-qa/v1"
MARKER_SPECS = builder.MARKER_SPECS
MARKER_VERSION = "tangying-warm-sloth-studio/v1"
MODE_CAMERA_NAMES = tuple(
    name
    for mode in contract.PRESENTATION_MODES
    for name, _location, _lens in contract.MODE_CAMERA_SPECS[mode].values()
)


def blend_file_bytes(filepath: str | Path | None) -> int | None:
    """Return the current saved blend size, or null for unsaved/missing files."""

    if not filepath:
        return None
    path = Path(filepath).expanduser()
    try:
        return path.stat().st_size if path.is_file() else None
    except OSError:
        return None


def _authored_light_specs() -> dict[str, dict[str, Any]]:
    specs = {
        str(item["name"]): {
            **dict(item),
            "fixture": None,
            "shadow_mode": "full" if item["shadows"] else "restrained",
            "softness": None,
        }
        for item in builder.LIGHT_SPECS
    }
    specs.update(
        {
            "Practical_Wall": {
                "name": "Practical_Wall",
                "type": "POINT",
                "location": (-1.70, 2.360, 2.8555),
                "energy": contract.SUBJECT_LIGHT_PROFILE["practicalWallEnergy"],
                "color": (1.0, 1.0, 1.0),
                "temperature": 2700,
                "role": "practical",
                "target": None,
                "size": None,
                "softness": 0.16,
                "shadows": False,
                "shadow_mode": "restrained",
                "fixture": "Wall_Sconce_Diffuser",
            },
            "Practical_Shelf": {
                "name": "Practical_Shelf",
                "type": "POINT",
                "location": (1.86, 2.48, 1.5622),
                "energy": contract.SUBJECT_LIGHT_PROFILE["practicalShelfEnergy"],
                "color": (1.0, 1.0, 1.0),
                "temperature": 2700,
                "role": "practical",
                "target": None,
                "size": None,
                "softness": 0.16,
                "shadows": False,
                "shadow_mode": "restrained",
                "fixture": "Shelf_TableLamp_Diffuser",
            },
        }
    )
    for name, location in builder.DOWNLIGHT_SPECS:
        specs[name] = {
            "name": name,
            "type": "AREA",
            "location": location,
            "energy": contract.SUBJECT_LIGHT_PROFILE["downlightEnergy"],
            "color": (1.0, 1.0, 1.0),
            "temperature": 3000,
            "role": "downlight",
            "target": (location[0], location[1], 0.72),
            "size": (0.34, 0.34),
            "softness": None,
            "shadows": True,
            "shadow_mode": "full",
            "fixture": f"{name}_Diffuser",
        }
    return specs


LIGHT_SPECS = _authored_light_specs()
LIGHTING_EVIDENCE_SCHEMA = "tangying-warm-studio-lighting-evidence/v1"


def _file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _image_rgb_mae(image_a: Path, image_b: Path) -> float:
    import OpenImageIO as oiio
    import numpy as np

    arrays = []
    for path in (image_a, image_b):
        image = oiio.ImageInput.open(str(path))
        if image is None:
            raise ValueError(f"could not open camera comparison image: {path}")
        try:
            pixels = np.asarray(image.read_image("float"), dtype=np.float32)
        finally:
            image.close()
        if pixels.ndim != 3 or pixels.shape[2] < 3:
            raise ValueError(f"camera comparison image is not RGB: {path}")
        arrays.append(pixels[..., :3])
    if arrays[0].shape != arrays[1].shape:
        raise ValueError(
            f"camera comparison image dimensions differ: {arrays[0].shape} != {arrays[1].shape}"
        )
    return float(np.mean(np.abs(arrays[0] - arrays[1])))


def validate_lighting_evidence_payload(payload: dict[str, Any]) -> list[str]:
    """Return fail-closed errors for canonical subject-lighting measurements."""

    errors: list[str] = []
    if payload.get("schemaVersion") != LIGHTING_EVIDENCE_SCHEMA:
        errors.append(f"schemaVersion must be {LIGHTING_EVIDENCE_SCHEMA!r}")
    if payload.get("luminanceColorSpace") != "scene_linear_rec709":
        errors.append("luminanceColorSpace must be scene_linear_rec709")
    if payload.get("displayColorSpace") != "AgX Medium High Contrast PNG":
        errors.append("displayColorSpace must be AgX Medium High Contrast PNG")
    if payload.get("subjectMaskSource") != (
        "Rendered character ID matte from actual scene geometry"
    ):
        errors.append("subjectMaskSource must use a rendered actual-geometry ID matte")
    if payload.get("faceMaskSource") != (
        "Rendered character ID matte intersected with projected semantic head geometry"
    ):
        errors.append(
            "faceMaskSource must intersect the geometry ID matte with projected semantic head geometry"
        )
    if payload.get("backgroundMaskSource") != (
        "Rendered non-character geometry excluding practical-highlight IDs and clipped display highlights"
    ):
        errors.append(
            "backgroundMaskSource must exclude character geometry, practical-highlight IDs, and clipped display highlights"
        )

    digest_cache: dict[Path, str] = {}
    comparisons = payload.get("cameraComparisons")
    expected_comparisons = {
        (mode, "medium", camera_role)
        for mode in contract.PRESENTATION_MODES
        for camera_role in ("three_quarter", "wide")
    }
    actual_comparisons: set[tuple[str, str, str]] = set()
    if not isinstance(comparisons, list):
        errors.append("cameraComparisons must be a list")
    else:
        if len(comparisons) != len(expected_comparisons):
            errors.append(
                f"cameraComparisons must contain exactly {len(expected_comparisons)} records"
            )
        for index, comparison in enumerate(comparisons):
            if not isinstance(comparison, dict):
                errors.append(f"camera comparison {index} must be an object")
                continue
            mode = str(comparison.get("mode"))
            label = f"camera comparison {index}/{mode}"
            camera_role_a = str(comparison.get("cameraRoleA", ""))
            camera_role_b = str(comparison.get("cameraRoleB", ""))
            comparison_key = (mode, camera_role_a, camera_role_b)
            if comparison_key in actual_comparisons:
                errors.append(f"{label} duplicates camera-role pair {comparison_key}")
            actual_comparisons.add(comparison_key)
            camera_a = str(comparison.get("cameraA", ""))
            camera_b = str(comparison.get("cameraB", ""))
            active_a = str(comparison.get("activeCameraA", ""))
            active_b = str(comparison.get("activeCameraB", ""))
            matrix_a = comparison.get("matrixWorldA")
            matrix_b = comparison.get("matrixWorldB")
            try:
                pixel_mae = float(comparison["pixelMae"])
            except (KeyError, TypeError, ValueError):
                errors.append(f"{label} pixel MAE is missing")
                continue
            if comparison.get("engine") != "eevee":
                errors.append(f"{label} must compare Eevee QA renders")
            if comparison_key not in expected_comparisons:
                errors.append(f"{label} has an unexpected camera-role pair {comparison_key}")
            elif mode in contract.PRESENTATION_MODES:
                expected_a = contract.MODE_CAMERA_SPECS[mode][camera_role_a][0]
                expected_b = contract.MODE_CAMERA_SPECS[mode][camera_role_b][0]
                if camera_a != expected_a or camera_b != expected_b:
                    errors.append(
                        f"{label} requested cameras must match {expected_a!r} and {expected_b!r}"
                    )
            if not camera_a or not camera_b or camera_a == camera_b:
                errors.append(f"{label} must name two different requested cameras")
            if active_a != camera_a or active_b != camera_b:
                errors.append(f"{label} active cameras must match the requested cameras")
            if (
                not isinstance(matrix_a, list)
                or not isinstance(matrix_b, list)
                or len(matrix_a) != 16
                or len(matrix_b) != 16
                or matrix_a == matrix_b
            ):
                errors.append(f"{label} must contain two different 4x4 camera matrices")
            if not math.isfinite(pixel_mae) or pixel_mae <= 1e-3:
                errors.append(f"{label} pixel MAE must be greater than 0.001")
            image_paths: dict[str, Path] = {}
            for image_key in ("imageA", "imageB"):
                raw_path = comparison.get(image_key)
                if not isinstance(raw_path, str) or not raw_path:
                    errors.append(f"{label} camera comparison {image_key} is missing")
                    continue
                image_path = Path(raw_path).expanduser().resolve()
                image_paths[image_key] = image_path
                if not image_path.is_file():
                    errors.append(
                        f"{label} camera comparison {image_key} is not a regular file: {image_path}"
                    )
                    continue
                digest_key = f"{image_key}Sha256"
                expected_digest = comparison.get(digest_key)
                if not isinstance(expected_digest, str) or len(expected_digest) != 64:
                    errors.append(f"{label} {digest_key} is missing")
                    continue
                actual_digest = digest_cache.get(image_path)
                if actual_digest is None:
                    actual_digest = _file_sha256(image_path)
                    digest_cache[image_path] = actual_digest
                if actual_digest != expected_digest:
                    errors.append(f"{label} {digest_key} does not match the artifact")
            if set(image_paths) == {"imageA", "imageB"} and all(
                path.is_file() for path in image_paths.values()
            ):
                try:
                    actual_mae = _image_rgb_mae(image_paths["imageA"], image_paths["imageB"])
                except ValueError as exc:
                    errors.append(f"{label} {exc}")
                else:
                    if not math.isclose(
                        pixel_mae,
                        actual_mae,
                        rel_tol=1e-6,
                        abs_tol=1e-6,
                    ):
                        errors.append(f"{label} pixel MAE does not match the image artifacts")
        missing_comparisons = sorted(expected_comparisons - actual_comparisons)
        extra_comparisons = sorted(actual_comparisons - expected_comparisons)
        if missing_comparisons:
            errors.append(
                f"missing materially different camera comparisons: {missing_comparisons}"
            )
        if extra_comparisons:
            errors.append(f"unexpected camera comparisons: {extra_comparisons}")

    measurements = payload.get("measurements")
    if not isinstance(measurements, list):
        return [*errors, "measurements must be a list"]
    expected = {
        (mode, engine, "medium")
        for mode in contract.PRESENTATION_MODES
        for engine in ("eevee", "cycles")
    }
    actual: set[tuple[str, str, str]] = set()
    if len(measurements) != len(expected):
        errors.append(f"measurements must contain exactly {len(expected)} records")
    for index, measurement in enumerate(measurements):
        if not isinstance(measurement, dict):
            errors.append(f"measurement {index} must be an object")
            continue
        key = (
            str(measurement.get("mode")),
            str(measurement.get("engine")),
            str(measurement.get("cameraRole")),
        )
        if key in actual:
            errors.append(f"measurement {index} duplicates lighting key {key}")
        actual.add(key)
        label = "/".join(key)
        try:
            face = float(measurement["linearFaceLuminance"])
            background = float(measurement["linearBackgroundLuminance"])
            stops = float(measurement["backgroundStopsBelowFace"])
            clip_ratio = float(measurement["highlightClipRatio"])
            subject_pixels = int(measurement["subjectPixelCount"])
            face_pixels = int(measurement["facePixelCount"])
            background_pixels = int(measurement["backgroundPixelCount"])
            practical_pixels = int(measurement["practicalHighlightPixelCount"])
            bright_pixels = int(measurement["brightNeutralPixelCount"])
            bright_rgb = tuple(float(value) for value in measurement["brightNeutralMedianRgb"])
            red_blue_ratio = float(measurement["brightNeutralRedBlueRatio"])
            red_green_ratio = float(measurement["brightNeutralRedGreenRatio"])
        except (KeyError, TypeError, ValueError):
            errors.append(f"{label} has incomplete numeric lighting evidence")
            continue
        if not all(math.isfinite(value) for value in (face, background, stops, clip_ratio)):
            errors.append(f"{label} lighting evidence must be finite")
        if face <= 0.0 or background <= 0.0:
            errors.append(f"{label} linear luminance samples must be positive")
        elif not math.isclose(
            stops,
            math.log2(face / background),
            rel_tol=1e-6,
            abs_tol=1e-6,
        ):
            errors.append(
                f"{label} backgroundStopsBelowFace is inconsistent with linear luminance"
            )
        background_stops_range = contract.SUBJECT_LIGHT_PROFILE["backgroundStopsRange"]
        if not background_stops_range[0] <= stops <= background_stops_range[1]:
            errors.append(
                f"{label} backgroundStopsBelowFace must be within "
                f"{background_stops_range[0]}..{background_stops_range[1]}"
            )
        if not 0.0 <= clip_ratio < 0.005:
            errors.append(f"{label} highlightClipRatio must be below 0.5 percent")
        if (
            subject_pixels <= 0
            or face_pixels <= 0
            or face_pixels > subject_pixels
            or background_pixels <= 0
            or practical_pixels <= 0
        ):
            errors.append(f"{label} subject/face/background/practical masks must be non-empty")
        if len(bright_rgb) != 3 or bright_pixels <= 0 or not all(
            math.isfinite(value) and value > 0.0 for value in bright_rgb
        ):
            errors.append(f"{label} bright-neutral subject sample must be non-empty and finite")
        else:
            if not math.isclose(
                red_blue_ratio,
                bright_rgb[0] / bright_rgb[2],
                rel_tol=1e-6,
                abs_tol=1e-6,
            ) or not math.isclose(
                red_green_ratio,
                bright_rgb[0] / bright_rgb[1],
                rel_tol=1e-6,
                abs_tol=1e-6,
            ):
                errors.append(f"{label} bright-neutral ratios are inconsistent with median RGB")
        red_blue_bounds = contract.SUBJECT_LIGHT_PROFILE["brightNeutralRedBlueRatio"]
        red_green_bounds = contract.SUBJECT_LIGHT_PROFILE["brightNeutralRedGreenRatio"]
        if not red_blue_bounds[0] <= red_blue_ratio <= red_blue_bounds[1]:
            errors.append(
                f"{label} bright-neutral red/blue ratio must be within "
                f"{red_blue_bounds[0]}..{red_blue_bounds[1]}"
            )
        if not red_green_bounds[0] <= red_green_ratio <= red_green_bounds[1]:
            errors.append(
                f"{label} bright-neutral red/green ratio must be within "
                f"{red_green_bounds[0]}..{red_green_bounds[1]}"
            )
        artifact_hashes = measurement.get("artifactSha256")
        if not isinstance(artifact_hashes, dict):
            errors.append(f"{label} artifactSha256 must be an object")
            artifact_hashes = {}
        for path_key in (
            "beautyPath",
            "linearBeautyPath",
            "emptyRoomPath",
            "linearEmptyRoomPath",
            "subjectMaskPath",
            "faceMaskPath",
            "backgroundMaskPath",
            "practicalHighlightMaskPath",
            "subjectMattePath",
        ):
            if not isinstance(measurement.get(path_key), str) or not measurement[path_key]:
                errors.append(f"{label} {path_key} is missing")
                continue
            artifact_path = Path(measurement[path_key]).expanduser().resolve()
            if not artifact_path.is_file():
                errors.append(f"{label} {path_key} is not a regular file: {artifact_path}")
                continue
            expected_digest = artifact_hashes.get(path_key)
            if not isinstance(expected_digest, str) or len(expected_digest) != 64:
                errors.append(f"{label} {path_key} SHA-256 is missing")
                continue
            actual_digest = digest_cache.get(artifact_path)
            if actual_digest is None:
                actual_digest = _file_sha256(artifact_path)
                digest_cache[artifact_path] = actual_digest
            if actual_digest != expected_digest:
                errors.append(f"{label} {path_key} SHA-256 does not match the artifact")
    missing = sorted(expected - actual)
    extra = sorted(actual - expected)
    if missing:
        errors.append(f"missing medium lighting measurements: {missing}")
    if extra:
        errors.append(f"unexpected lighting measurements: {extra}")
    return errors
FLOOR_CONTACT_ROOTS = (
    "Desk_Top",
    "Chair_Main",
    "Cabinet_Left",
    "Rug_Main",
)
REQUIRED_MATERIALS = (
    "Wall_WarmPlaster",
    "Floor_LightOak",
    "Desk_WarmOak",
    "Slat_Walnut",
    "Chair_Fabric",
    "Rug_Jute",
    "Ceramic_Sand",
    "FloorLeaf_Sage_A",
)
PRODUCTION_COLLECTIONS = tuple(
    name for name in contract.REQUIRED_COLLECTIONS if name != "QA_ONLY"
)
BRAND_OBJECTS = (
    "Brand_Artwork",
    "Brand_BackPanel",
    "Brand_Paper",
    "Brand_Copy_Line1",
    "Brand_Copy_Line2",
    "Brand_Glass",
    "Brand_Frame_Top",
    "Brand_Frame_Bottom",
    "Brand_Frame_Left",
    "Brand_Frame_Right",
)
MATERIAL_BINDINGS = {
    "Desk_Top": "Desk_WarmOak",
    "Slat_Back_001": "Slat_Walnut_GrainZ",
    "Wall_Left_WindowFront": "Wall_WarmPlaster",
    "Curtain_Sheer": "Curtain_Sheer",
    "Curtain_Outer_Front": "Curtain_Outer",
    "Rug_Main": "Rug_Jute",
    "Brand_Glass": "Brand_Glass_Rough",
    "FloorPlant_Left_Pot": "Ceramic_Sand",
    "FloorPlant_Left_Leaf_01": "FloorLeaf_Sage_A",
}
CRITICAL_PRODUCTION_OBJECTS = (
    "Studio_Floor",
    "Studio_Ceiling",
    "Wall_Back",
    "Wall_Front_Left",
    "Wall_Front_Right",
    "Wall_Left_WindowFront",
    "Wall_Left_WindowRear",
    "Wall_Left_WindowLower",
    "Wall_Left_WindowUpper",
    "Wall_Right_DoorFront",
    "Wall_Right_DoorRear",
    "Wall_Right_DoorLintel",
    "Desk_Top",
    "Chair_Main",
    "Chair_Seat",
    "Chair_Back",
    "Chair_Foot_01",
    "Chair_Foot_02",
    "Chair_Foot_03",
    "Chair_Foot_04",
    "Cabinet_Left",
    "SlatWall_Back_Right",
    "Shelf_Right_01",
    "Shelf_Right_02",
    "Shelf_Right_03",
    "Rug_Main",
)


def _close_vector(
    actual: Iterable[float],
    expected: Iterable[float],
    tolerance: float = 1e-4,
) -> bool:
    actual_values = tuple(actual)
    expected_values = tuple(expected)
    if len(actual_values) != len(expected_values):
        return False
    return all(
        math.isclose(float(left), float(right), abs_tol=tolerance)
        for left, right in zip(actual_values, expected_values)
    )


def _scene_object(name: str) -> bpy.types.Object | None:
    """Return an object only when it belongs to the active scene."""

    return bpy.context.scene.objects.get(name)


def _reachable_collections() -> set[bpy.types.Collection]:
    result: set[bpy.types.Collection] = set()
    pending = list(bpy.context.scene.collection.children)
    while pending:
        collection = pending.pop()
        if collection in result:
            continue
        result.add(collection)
        pending.extend(collection.children)
    return result


def _find_collection_path(
    root: bpy.types.Collection,
    collection: bpy.types.Collection,
) -> list[bpy.types.Collection] | None:
    """Return the full active-scene collection path, including root and target."""

    if root is collection:
        return [root]
    for child in root.children:
        found = _find_collection_path(child, collection)
        if found is not None:
            return [root, *found]
    return None


def _find_layer_collection_path(
    root: bpy.types.LayerCollection,
    collection: bpy.types.Collection,
) -> list[bpy.types.LayerCollection] | None:
    if root.collection is collection:
        return [root]
    for child in root.children:
        found = _find_layer_collection_path(child, collection)
        if found is not None:
            return [root, *found]
    return None


def _manifest_entry(obj: bpy.types.Object) -> dict[str, object]:
    return {
        "name": obj.name,
        "type": obj.type,
        "parent": obj.parent.name if obj.parent is not None else None,
        "collections": sorted(collection.name for collection in obj.users_collection),
    }


def _allowed_qa_extra(obj: bpy.types.Object) -> bool:
    qa_collection = bpy.data.collections.get("QA_ONLY")
    return (
        qa_collection is not None
        and bool(obj.get("qa_only"))
        and obj.hide_render
        and obj.users_collection
        and all(collection is qa_collection for collection in obj.users_collection)
    )


def _validate_authored_manifest(report: dict[str, Any]) -> None:
    master = _scene_object("SET_MASTER")
    if master is None:
        report["errors"].append("Authored manifest unavailable because SET_MASTER is missing")
        return
    manifest_text_name = master.get("ip_manifest_text")
    manifest_text = (
        bpy.data.texts.get(str(manifest_text_name)) if manifest_text_name else None
    )
    if manifest_text is None:
        report["errors"].append("SET_MASTER authored manifest text is missing")
        return
    payload = manifest_text.as_string()
    expected_hash = hashlib.sha256(payload.encode("utf-8")).hexdigest()
    if master.get("ip_manifest_sha256") != expected_hash:
        report["errors"].append(
            "SET_MASTER authored manifest SHA-256 does not match its text payload"
        )
    try:
        manifest = json.loads(payload)
    except json.JSONDecodeError as exc:
        report["errors"].append(f"SET_MASTER authored manifest JSON is invalid: {exc}")
        return
    if manifest.get("schemaVersion") != builder.MANIFEST_VERSION:
        report["errors"].append(
            f"SET_MASTER authored manifest version must be {builder.MANIFEST_VERSION!r}"
        )
    if master.get("ip_manifest_version") != builder.MANIFEST_VERSION:
        report["errors"].append(
            f"SET_MASTER ip_manifest_version must be {builder.MANIFEST_VERSION!r}"
        )
    stored_entries = manifest.get("objects")
    if not isinstance(stored_entries, list):
        report["errors"].append("SET_MASTER authored manifest objects must be a list")
        return
    expected_by_name = {
        str(entry.get("name")): entry
        for entry in stored_entries
        if isinstance(entry, dict) and entry.get("name")
    }
    current_objects = {
        obj.name: obj
        for obj in bpy.context.scene.objects
        if not _allowed_qa_extra(obj)
    }
    for name in sorted(set(expected_by_name) - set(current_objects)):
        report["errors"].append(
            f"Authored manifest object '{name}' is missing from the active scene"
        )
    for name in sorted(set(current_objects) - set(expected_by_name)):
        report["errors"].append(
            f"Unauthorized active-scene object '{name}' is not in the authored manifest"
        )
    for name in sorted(set(expected_by_name) & set(current_objects)):
        current_entry = _manifest_entry(current_objects[name])
        if current_entry != expected_by_name[name]:
            report["errors"].append(
                f"Authored manifest mismatch for '{name}': "
                f"expected {expected_by_name[name]!r}, found {current_entry!r}"
            )
    stored_count = manifest.get("objectCount")
    if stored_count != len(expected_by_name):
        report["errors"].append(
            f"Authored manifest objectCount is {stored_count!r}; "
            f"entry count is {len(expected_by_name)}"
        )
    if master.get("ip_manifest_object_count") != len(expected_by_name):
        report["errors"].append(
            "SET_MASTER ip_manifest_object_count does not match the authored manifest"
        )


def _descendants(root: bpy.types.Object) -> list[bpy.types.Object]:
    result: list[bpy.types.Object] = []
    pending = list(root.children)
    while pending:
        child = pending.pop()
        result.append(child)
        pending.extend(child.children)
    return result


def _world_min_z(objects: Iterable[bpy.types.Object]) -> float | None:
    depsgraph = bpy.context.evaluated_depsgraph_get()
    heights: list[float] = []
    for obj in objects:
        if obj.type not in {"MESH", "CURVE", "FONT", "SURFACE", "META"}:
            continue
        evaluated = obj.evaluated_get(depsgraph)
        mesh = None
        try:
            mesh = evaluated.to_mesh()
            heights.extend(
                (evaluated.matrix_world @ vertex.co).z for vertex in mesh.vertices
            )
        finally:
            if mesh is not None:
                evaluated.to_mesh_clear()
    return min(heights) if heights else None


def _production_objects() -> set[bpy.types.Object]:
    result: set[bpy.types.Object] = set()
    reachable = _reachable_collections()
    scene_objects = set(bpy.context.scene.objects)
    for name in PRODUCTION_COLLECTIONS:
        collection = bpy.data.collections.get(name)
        if collection is not None and collection in reachable:
            result.update(obj for obj in collection.all_objects if obj in scene_objects)
    return result


def _record_required(
    report: dict[str, Any],
    category: str,
    names: Iterable[str],
) -> None:
    destination = report[category]
    for name in names:
        present = _scene_object(name) is not None
        destination[name] = present
        if not present:
            report["errors"].append(f"Required object '{name}' is missing")


def _validate_markers(report: dict[str, Any]) -> None:
    marker_collection = bpy.data.collections.get("STUDIO_MARKERS")
    for name, (expected_location, expected_role) in MARKER_SPECS.items():
        marker = _scene_object(name)
        if marker is None:
            continue
        if marker.type != "EMPTY":
            report["errors"].append(f"Marker '{name}' must be an EMPTY, found {marker.type}")
        if not _close_vector(marker.location, expected_location):
            report["errors"].append(
                f"Marker '{name}' location {tuple(round(v, 4) for v in marker.location)} "
                f"does not match {expected_location}"
            )
        if marker.get("contract") != expected_role:
            report["errors"].append(
                f"Marker '{name}' contract must be '{expected_role}', "
                f"found {marker.get('contract')!r}"
            )
        if not math.isclose(
            float(marker.get("target_height", -1.0)),
            contract.TARGET_CHARACTER_HEIGHT,
            abs_tol=1e-6,
        ):
            report["errors"].append(
                f"Marker '{name}' target_height must be {contract.TARGET_CHARACTER_HEIGHT}, "
                f"found {marker.get('target_height')!r}"
            )
        if marker.get("ip_marker_version") != MARKER_VERSION:
            report["errors"].append(
                f"{name} ip_marker_version must be {MARKER_VERSION!r}, "
                f"found {marker.get('ip_marker_version')!r}"
            )
        if marker_collection is not None and marker_collection not in marker.users_collection:
            report["errors"].append(f"Marker '{name}' is not in STUDIO_MARKERS")


def _validate_cameras(report: dict[str, Any]) -> None:
    camera_collection = bpy.data.collections.get("STUDIO_CAMERAS")
    for name, spec in contract.CAMERA_SPECS.items():
        camera = _scene_object(name)
        if camera is None:
            continue
        if camera.type != "CAMERA":
            report["errors"].append(f"Camera '{name}' has type {camera.type}, expected CAMERA")
            continue
        if not math.isclose(camera.data.lens, float(spec["lens"]), abs_tol=1e-4):
            report["errors"].append(
                f"Camera '{name}' lens is {camera.data.lens:g}mm; "
                f"expected {float(spec['lens']):g}mm"
            )
        if not _close_vector(camera.location, spec["location"]):
            report["errors"].append(
                f"Camera '{name}' location {tuple(round(v, 4) for v in camera.location)} "
                f"does not match {tuple(spec['location'])}"
            )
        target = tuple(spec["target"])
        forward = camera.rotation_euler.to_matrix() @ Vector((0.0, 0.0, -1.0))
        expected_forward = (Vector(target) - camera.location).normalized()
        if forward.normalized().dot(expected_forward) < 0.999999:
            report["errors"].append(
                f"{name} orientation must point toward contract target {target}"
            )
        if not _close_vector(camera.get("ip_target", ()), target):
            report["errors"].append(
                f"{name} ip_target must be {target}, found {camera.get('ip_target')!r}"
            )
        expected_role = builder.CAMERA_RESULT_KEYS[name]
        if camera.get("ip_camera_role") != expected_role:
            report["errors"].append(
                f"{name} ip_camera_role must be {expected_role!r}, "
                f"found {camera.get('ip_camera_role')!r}"
            )
        if camera.get("ip_framing_contract") != "contract.CAMERA_SPECS":
            report["errors"].append(
                f"{name} ip_framing_contract must be 'contract.CAMERA_SPECS'"
            )
        if not math.isclose(camera.data.sensor_width, 36.0, abs_tol=1e-5):
            report["errors"].append(
                f"{name} sensor_width must be 36mm, found {camera.data.sensor_width:g}"
            )
        if not math.isclose(camera.data.clip_start, 0.03, abs_tol=1e-6):
            report["errors"].append(
                f"{name} clip_start must be 0.03m, found {camera.data.clip_start:g}"
            )
        if not math.isclose(camera.data.clip_end, 100.0, abs_tol=1e-5):
            report["errors"].append(
                f"{name} clip_end must be 100m, found {camera.data.clip_end:g}"
            )
        focus_name = str(spec["focus"])
        focus = _scene_object(focus_name)
        if camera.data.dof.focus_object is not focus or camera.get("ip_focus_marker") != focus_name:
            found = getattr(camera.data.dof.focus_object, "name", None)
            report["errors"].append(
                f"{name} focus marker must be '{focus_name}', found {found!r}"
            )
        if not camera.data.dof.use_dof:
            report["errors"].append(f"{name} must have depth of field enabled")
        expected_fstop = builder.CAMERA_F_STOPS[name]
        if not math.isclose(
            camera.data.dof.aperture_fstop,
            expected_fstop,
            abs_tol=1e-5,
        ):
            report["errors"].append(
                f"{name} aperture_fstop must be {expected_fstop:g}, "
                f"found {camera.data.dof.aperture_fstop:g}"
            )
        if camera.data.dof.aperture_blades != 9:
            report["errors"].append(
                f"{name} aperture_blades must be 9, "
                f"found {camera.data.dof.aperture_blades}"
            )
        if camera_collection is not None and camera_collection not in camera.users_collection:
            report["errors"].append(f"Camera '{name}' is not in STUDIO_CAMERAS")

    for mode in contract.PRESENTATION_MODES:
        for role, (name, location, lens) in contract.MODE_CAMERA_SPECS[mode].items():
            focus_name = (
                "IP_Transition_Focus"
                if role == "transition"
                else f"IP_{mode.title()}_Focus_Head"
            )
            camera = _scene_object(name)
            if camera is None:
                report["errors"].append(f"Required mode camera '{name}' is missing")
                continue
            if camera.type != "CAMERA":
                report["errors"].append(
                    f"Mode camera '{name}' has type {camera.type}, expected CAMERA"
                )
                continue
            if not _close_vector(camera.location, location):
                report["errors"].append(
                    f"{name} location must be {location}, found {tuple(camera.location)}"
                )
            if not math.isclose(camera.data.lens, lens, abs_tol=1e-4):
                report["errors"].append(
                    f"{name} lens must be {lens:g}mm, found {camera.data.lens:g}mm"
                )
            if camera.get("ip_focus_marker") != focus_name:
                report["errors"].append(
                    f"{name} focus marker must be {focus_name!r}"
                )
            focus = _scene_object(focus_name)
            if not camera.data.dof.use_dof:
                report["errors"].append(f"{name} must have depth of field enabled")
            if focus is None or camera.data.dof.focus_object is not focus:
                found = getattr(camera.data.dof.focus_object, "name", None)
                report["errors"].append(
                    f"{name} DOF focus object must be {focus_name!r}, found {found!r}"
                )
            if not math.isclose(
                camera.data.dof.aperture_fstop,
                builder.MODE_CAMERA_F_STOP,
                abs_tol=1e-5,
            ):
                report["errors"].append(
                    f"{name} aperture_fstop must be {builder.MODE_CAMERA_F_STOP:g}, "
                    f"found {camera.data.dof.aperture_fstop:g}"
                )
            if camera.data.dof.aperture_blades != 9:
                report["errors"].append(
                    f"{name} aperture_blades must be 9, "
                    f"found {camera.data.dof.aperture_blades}"
                )
            if camera.get("ip_camera_role") != role:
                report["errors"].append(f"{name} camera role must be {role!r}")
            if camera.get("ip_framing_contract") != "contract.MODE_CAMERA_SPECS":
                report["errors"].append(
                    f"{name} must use contract.MODE_CAMERA_SPECS"
                )
            if camera_collection is not None and camera_collection not in camera.users_collection:
                report["errors"].append(f"Camera '{name}' is not in STUDIO_CAMERAS")

    camera_objects = [obj for obj in bpy.context.scene.objects if obj.type == "CAMERA"]
    expected_camera_count = len(contract.CAMERA_SPECS) + len(MODE_CAMERA_NAMES)
    if len(camera_objects) != expected_camera_count:
        report["errors"].append(
            f"Scene must contain exactly {expected_camera_count} cameras; "
            f"found {len(camera_objects)}"
        )
    wide = _scene_object("Camera_Wide")
    if wide is not None and bpy.context.scene.camera is not wide:
        active = getattr(bpy.context.scene.camera, "name", None)
        report["errors"].append(
            f"Active production camera must be 'Camera_Wide', found {active!r}"
        )


def _validate_lights(report: dict[str, Any]) -> None:
    light_collection = bpy.data.collections.get("STUDIO_LIGHTS")
    for name, spec in LIGHT_SPECS.items():
        light = _scene_object(name)
        if light is None:
            report["errors"].append(f"Required light '{name}' is missing")
            continue
        if light.type != "LIGHT":
            report["errors"].append(f"Light '{name}' has type {light.type}, expected LIGHT")
            continue
        expected_type = str(spec["type"])
        if light.data.type != expected_type:
            report["errors"].append(
                f"{name} type must be {expected_type}, found {light.data.type}"
            )
        if not _close_vector(light.location, spec["location"]):
            report["errors"].append(
                f"{name} location must be {tuple(spec['location'])}, "
                f"found {tuple(round(value, 4) for value in light.location)}"
            )
        expected_role = str(spec["role"])
        if light.get("ip_light_role") != expected_role:
            report["errors"].append(
                f"{name} ip_light_role must be {expected_role!r}, "
                f"found {light.get('ip_light_role')!r}"
            )
        energy = float(spec["energy"])
        if not math.isclose(light.data.energy, energy, abs_tol=1e-4):
            report["errors"].append(
                f"{name} energy must be {energy:g}, found {light.data.energy:g}"
            )
        if not math.isclose(
            float(light.get("ip_base_energy", -1.0)), energy, abs_tol=1e-4
        ):
            report["errors"].append(
                f"{name} ip_base_energy must be {energy:g}, "
                f"found {light.get('ip_base_energy')!r}"
            )
        temperature = int(spec["temperature"])
        if light.get("ip_color_temperature") != temperature:
            report["errors"].append(
                f"{name} ip_color_temperature must be {temperature}K"
            )
        if not light.data.use_temperature:
            report["errors"].append(f"{name} must use native Blender temperature")
        elif not math.isclose(light.data.temperature, temperature, abs_tol=1e-4):
            report["errors"].append(
                f"{name} native temperature must be {temperature}K, "
                f"found {light.data.temperature:g}K"
            )
        expected_color = tuple(spec["color"])
        if not _close_vector(light.data.color, expected_color, 1e-5):
            report["errors"].append(
                f"{name} color must be {expected_color}, "
                f"found {tuple(round(value, 4) for value in light.data.color)}"
            )
        authored_color = light.get("ip_authored_color", ())
        if not _close_vector(authored_color, expected_color, 1e-5):
            report["errors"].append(
                f"{name} ip_authored_color must be {expected_color}, "
                f"found {authored_color!r}"
            )
        elif not _close_vector(authored_color, light.data.color, 1e-5):
            report["errors"].append(
                f"{name} ip_authored_color must agree with the light data color"
            )
        shadows = bool(spec["shadows"])
        if bool(light.data.use_shadow) != shadows:
            report["errors"].append(
                f"{name} use_shadow must be {shadows}"
            )
        if bool(light.get("ip_casts_shadow")) != shadows:
            report["errors"].append(
                f"{name} ip_casts_shadow must be {shadows}"
            )
        if light.get("ip_shadow_mode") != spec["shadow_mode"]:
            report["errors"].append(
                f"{name} ip_shadow_mode must be {spec['shadow_mode']!r}, "
                f"found {light.get('ip_shadow_mode')!r}"
            )
        target = spec["target"]
        if target is not None:
            if not _close_vector(light.get("ip_target", ()), target):
                report["errors"].append(
                    f"{name} ip_target must be {tuple(target)}, "
                    f"found {light.get('ip_target')!r}"
                )
            forward = light.rotation_euler.to_matrix() @ Vector((0.0, 0.0, -1.0))
            expected_forward = (Vector(target) - light.location).normalized()
            if forward.normalized().dot(expected_forward) < 0.999999:
                report["errors"].append(
                    f"{name} orientation must point toward target {tuple(target)}"
                )
        elif light.get("ip_target") is not None:
            report["errors"].append(f"{name} ip_target must be absent for a point practical")

        if expected_type == "AREA" and light.data.type == "AREA":
            if light.data.shape != "RECTANGLE":
                report["errors"].append(
                    f"{name} shape must be RECTANGLE, found {light.data.shape}"
                )
            expected_size = tuple(spec["size"])
            if not math.isclose(light.data.size, expected_size[0], abs_tol=1e-5):
                report["errors"].append(
                    f"{name} size must be {expected_size[0]:g}, found {light.data.size:g}"
                )
            if not math.isclose(light.data.size_y, expected_size[1], abs_tol=1e-5):
                report["errors"].append(
                    f"{name} size_y must be {expected_size[1]:g}, "
                    f"found {light.data.size_y:g}"
                )
        if expected_type == "POINT" and light.data.type == "POINT":
            softness = float(spec["softness"])
            if not math.isclose(light.data.shadow_soft_size, softness, abs_tol=1e-5):
                report["errors"].append(
                    f"{name} shadow_soft_size must be {softness:g}, "
                    f"found {light.data.shadow_soft_size:g}"
                )

        expected_specular = 0.55 if expected_role in {"window", "key"} else 0.30
        if not math.isclose(light.data.diffuse_factor, 1.0, abs_tol=1e-5):
            report["errors"].append(f"{name} diffuse_factor must be 1.0")
        if not math.isclose(
            light.data.specular_factor, expected_specular, abs_tol=1e-5
        ):
            report["errors"].append(
                f"{name} specular_factor must be {expected_specular:g}"
            )
        expected_fixture = spec["fixture"]
        if expected_fixture is None:
            if light.get("ip_fixture") is not None:
                report["errors"].append(f"{name} ip_fixture must be absent")
        elif light.get("ip_fixture") != expected_fixture:
            report["errors"].append(
                f"{name} ip_fixture must be {expected_fixture!r}, "
                f"found {light.get('ip_fixture')!r}"
            )
        if light_collection is not None and light_collection not in light.users_collection:
            report["errors"].append(f"Light '{name}' is not in STUDIO_LIGHTS")

    light_objects = [obj for obj in bpy.context.scene.objects if obj.type == "LIGHT"]
    if len(light_objects) != len(LIGHT_SPECS):
        report["errors"].append(
            f"Scene must contain exactly {len(LIGHT_SPECS)} authored lights; "
            f"found {len(light_objects)}"
        )


def _validate_brand(report: dict[str, Any]) -> None:
    line1 = _scene_object("Brand_Copy_Line1")
    line2 = _scene_object("Brand_Copy_Line2")
    if line1 is not None and getattr(line1.data, "body", None) != "Slow Down.":
        report["errors"].append("Brand_Copy_Line1 must read exactly 'Slow Down.'")
    if line2 is not None and getattr(line2.data, "body", None) != "Think Better.":
        report["errors"].append("Brand_Copy_Line2 must read exactly 'Think Better.'")


def _validate_plants(report: dict[str, Any]) -> None:
    symmetry: dict[str, Any] = {
        "linkedData": False,
        "mirroredLocations": False,
        "mirroredRotations": False,
        "equalDimensions": False,
    }
    report["plantSymmetry"] = symmetry
    left = _scene_object("FloorPlant_Left")
    right = _scene_object("FloorPlant_Right")
    if left is None or right is None:
        report["errors"].append("FloorPlant_Left and FloorPlant_Right are required")
        return

    left_spec, right_spec = contract.floor_plant_transforms()
    symmetry["linkedData"] = left.data is right.data
    symmetry["mirroredLocations"] = (
        math.isclose(left.location.x, -right.location.x, abs_tol=1e-4)
        and math.isclose(left.location.y, right.location.y, abs_tol=1e-4)
        and math.isclose(left.location.z, right.location.z, abs_tol=1e-4)
    )
    symmetry["mirroredRotations"] = math.isclose(
        left.rotation_euler.z,
        -right.rotation_euler.z,
        abs_tol=1e-4,
    )
    symmetry["equalDimensions"] = _close_vector(left.dimensions, right.dimensions, 1e-3)

    if not symmetry["linkedData"]:
        report["errors"].append(
            "FloorPlant_Left and FloorPlant_Right must share linked mesh data"
        )
    if left.data.name != contract.PLANT_LINK_KEY or right.data.name != contract.PLANT_LINK_KEY:
        report["errors"].append(
            f"Floor plant data must use link key '{contract.PLANT_LINK_KEY}'"
        )
    for plant, spec in ((left, left_spec), (right, right_spec)):
        expected_location = spec[:3]
        if not _close_vector(plant.location, expected_location):
            report["errors"].append(
                f"{plant.name} location {tuple(round(v, 4) for v in plant.location)} "
                f"does not match {expected_location}"
            )
        if not math.isclose(plant.rotation_euler.z, spec[3], abs_tol=1e-4):
            report["errors"].append(
                f"{plant.name} Z rotation {plant.rotation_euler.z:.4f} does not match {spec[3]:.4f}"
            )
        if plant.get("linked_data_key") != contract.PLANT_LINK_KEY:
            report["errors"].append(f"{plant.name} linked_data_key metadata is invalid")
    if not symmetry["mirroredLocations"]:
        report["errors"].append("Floor plant locations are not mirrored across room center")
    if not symmetry["mirroredRotations"]:
        report["errors"].append("Floor plant Z rotations are not mirrored")
    if not symmetry["equalDimensions"]:
        report["errors"].append("Floor plant evaluated dimensions are not equal")

    left_pot = _scene_object("FloorPlant_Left_Pot")
    right_pot = _scene_object("FloorPlant_Right_Pot")
    if left_pot is None or right_pot is None or left_pot.data is not right_pot.data:
        report["errors"].append("Floor plant ceramic pots must share linked mesh data")


def _validate_materials_and_hierarchy(
    report: dict[str, Any], require_packed_brand: bool
) -> None:
    for material_name in REQUIRED_MATERIALS:
        if bpy.data.materials.get(material_name) is None:
            report["errors"].append(f"Required material '{material_name}' is missing")

    production_objects = _production_objects()
    master = _scene_object("SET_MASTER")
    for obj in sorted(production_objects, key=lambda item: item.name):
        if obj.hide_render:
            report["errors"].append(
                f"{obj.name} hide_render must be False for authored production objects"
            )
        if obj is master:
            continue
        ancestor = obj.parent
        visited: set[bpy.types.Object] = set()
        while ancestor is not None and ancestor not in visited and ancestor is not master:
            visited.add(ancestor)
            ancestor = ancestor.parent
        if ancestor is not master:
            report["errors"].append(
                f"{obj.name} ancestry must reach SET_MASTER; "
                f"direct parent is {getattr(obj.parent, 'name', None)!r}"
            )

    missing_materials = sorted(
        obj.name
        for obj in production_objects
        if obj.type in {"MESH", "CURVE", "FONT"}
        and hasattr(obj.data, "materials")
        and len(obj.data.materials) == 0
    )
    if missing_materials:
        report["errors"].append(
            "Renderable production objects have no material: " + ", ".join(missing_materials)
        )

    for object_name, expected_material in MATERIAL_BINDINGS.items():
        obj = _scene_object(object_name)
        if obj is None:
            continue
        actual_material = (
            obj.material_slots[0].material.name
            if obj.material_slots and obj.material_slots[0].material is not None
            else None
        )
        if actual_material != expected_material:
            report["errors"].append(
                f"{object_name} material must be {expected_material!r}, "
                f"found {actual_material!r}"
            )

    negative = sorted(
        obj.name
        for obj in bpy.context.scene.objects
        if any(axis < -1e-7 for axis in obj.scale)
    )
    report["negativeScaleObjects"] = negative
    if negative:
        report["errors"].append(
            "Negative object scale is not production-safe: " + ", ".join(negative)
        )


def _validate_no_character(report: dict[str, Any]) -> None:
    armatures = sorted(
        obj.name for obj in bpy.context.scene.objects if obj.type == "ARMATURE"
    )
    if armatures:
        report["errors"].append(
            "Empty studio must not contain armatures: " + ", ".join(armatures)
        )
    character_meshes = sorted(
        obj.name
        for obj in bpy.context.scene.objects
        if obj.type == "MESH"
        and (
            bool(obj.get("character_mesh"))
            or bool(obj.get("ip_character"))
            or any(token in obj.name.lower() for token in ("character", "avatar"))
            or any(
                "character" in collection.name.lower() or "avatar" in collection.name.lower()
                for collection in obj.users_collection
            )
        )
    )
    if character_meshes:
        report["errors"].append(
            "Empty studio must not contain character meshes: " + ", ".join(character_meshes)
        )


def _validate_floor_contacts(report: dict[str, Any]) -> None:
    floating: list[str] = []
    for name in FLOOR_CONTACT_ROOTS:
        root = _scene_object(name)
        if root is None:
            continue
        minimum = _world_min_z([root, *_descendants(root)])
        if minimum is None:
            continue
        if abs(minimum) > 0.020:
            floating.append(name)
            report["errors"].append(
                f"Furniture '{name}' floor contact is z={minimum:.4f}m; tolerance is ±0.0200m"
            )
    report["floatingFurniture"] = floating


def _validate_render_settings(report: dict[str, Any]) -> None:
    scene = bpy.context.scene
    if scene.render.engine not in {"BLENDER_EEVEE", "BLENDER_EEVEE_NEXT"}:
        report["errors"].append(
            f"Default render engine must be Eevee, found {scene.render.engine!r}"
        )
    if (
        scene.render.resolution_x,
        scene.render.resolution_y,
    ) != contract.DEFAULT_RENDER_RESOLUTION:
        report["errors"].append(
            f"Render resolution must be 1920x1080, found "
            f"{scene.render.resolution_x}x{scene.render.resolution_y}"
        )
    if scene.render.resolution_percentage != 100:
        report["errors"].append("Render resolution percentage must be 100")
    if scene.render.fps != 30 or not math.isclose(scene.render.fps_base, 1.0):
        report["errors"].append(
            f"fps must be 30 with fps_base 1.0, found "
            f"{scene.render.fps}/{scene.render.fps_base:g}"
        )
    if not (
        math.isclose(scene.render.pixel_aspect_x, 1.0, abs_tol=1e-6)
        and math.isclose(scene.render.pixel_aspect_y, 1.0, abs_tol=1e-6)
    ):
        report["errors"].append(
            f"pixel_aspect must be 1:1, found "
            f"{scene.render.pixel_aspect_x:g}:{scene.render.pixel_aspect_y:g}"
        )
    if scene.render.film_transparent:
        report["errors"].append("film_transparent must be False for the opaque studio")
    if scene.view_settings.view_transform != "AgX":
        report["errors"].append(
            f"View transform must be AgX, found {scene.view_settings.view_transform!r}"
        )
    if "Medium High Contrast" not in scene.view_settings.look:
        report["errors"].append(
            f"AgX look must be Medium High Contrast, found {scene.view_settings.look!r}"
        )
    if not scene.view_settings.use_white_balance:
        report["errors"].append("camera white balance must be enabled")
    elif not math.isclose(
        scene.view_settings.white_balance_temperature,
        contract.SUBJECT_LIGHT_PROFILE["whiteBalanceTemperatureK"],
        abs_tol=1e-6,
    ) or not math.isclose(
        scene.view_settings.white_balance_tint,
        contract.SUBJECT_LIGHT_PROFILE["whiteBalanceTint"],
        abs_tol=1e-6,
    ):
        report["errors"].append("camera white balance must match the subject light profile")
    if not math.isclose(
        scene.view_settings.exposure,
        builder.AUTHORED_EXPOSURE,
        abs_tol=1e-6,
    ):
        report["errors"].append(
            f"exposure must be {builder.AUTHORED_EXPOSURE:g}, "
            f"found {scene.view_settings.exposure:g}"
        )
    if scene.render.image_settings.file_format != "PNG":
        report["errors"].append("Render output format must be PNG")
    if scene.render.image_settings.color_mode != "RGBA":
        report["errors"].append("Render output color mode must be RGBA")
    if scene.render.image_settings.color_depth != "8":
        report["errors"].append(
            f"color_depth must be 8-bit, found {scene.render.image_settings.color_depth!r}"
        )

    profile = contract.SUBJECT_LIGHT_PROFILE
    if scene.get("ip_subject_light_profile") != profile["name"]:
        report["errors"].append(
            f"ip_subject_light_profile must be {profile['name']!r}"
        )
    world = scene.world
    background = (
        next((node for node in world.node_tree.nodes if node.type == "BACKGROUND"), None)
        if world is not None and world.use_nodes
        else None
    )
    if background is None:
        report["errors"].append("World Background node is required")
    elif not math.isclose(
        background.inputs["Strength"].default_value,
        profile["worldStrength"],
        abs_tol=1e-6,
    ):
        report["errors"].append(
            f"World strength must be {profile['worldStrength']:g}, "
            f"found {background.inputs['Strength'].default_value:g}"
        )

    metadata_contract = {
        "ip_preview_engine": "BLENDER_EEVEE_NEXT",
        "ip_final_engine": "CYCLES",
        "ip_eevee_render_samples": 128,
        "ip_cycles_final_samples": 128,
        "ip_authored_exposure": builder.AUTHORED_EXPOSURE,
        "ip_cycles_final_exposure": builder.CYCLES_FINAL_EXPOSURE,
    }
    for key, expected in metadata_contract.items():
        actual = scene.get(key)
        if actual != expected:
            report["errors"].append(
                f"{key} must be {expected!r}, found {actual!r}"
            )
    if hasattr(scene, "cycles"):
        if scene.cycles.samples != 128:
            report["errors"].append(
                f"Cycles production samples must be 128, found {scene.cycles.samples}"
            )
        if hasattr(scene.cycles, "use_denoising") and not scene.cycles.use_denoising:
            report["errors"].append("Cycles production denoising must be enabled")
    eevee = getattr(scene, "eevee", None)
    if eevee is not None and hasattr(eevee, "taa_render_samples"):
        if eevee.taa_render_samples != 128:
            report["errors"].append(
                f"Eevee production samples must be 128, found {eevee.taa_render_samples}"
            )


def validate_scene(require_packed_brand: bool = True) -> dict[str, Any]:
    """Return a JSON-serializable structural QA report for the open scene."""

    scene = bpy.context.scene
    scene_objects = list(scene.objects)
    active_meshes = {
        obj.data for obj in scene_objects if obj.type == "MESH" and obj.data is not None
    }
    active_materials = {
        slot.material
        for obj in scene_objects
        for slot in obj.material_slots
        if slot.material is not None
    }
    report: dict[str, Any] = {
        "schemaVersion": SCHEMA_VERSION,
        "blenderVersion": bpy.app.version_string,
        "blendFileBytes": blend_file_bytes(bpy.data.filepath),
        "errors": [],
        "warnings": [],
        "counts": {
            "objects": len(scene_objects),
            "meshes": len(active_meshes),
            "materials": len(active_materials),
            "lights": sum(obj.type == "LIGHT" for obj in scene_objects),
            "cameras": sum(obj.type == "CAMERA" for obj in scene_objects),
        },
        "requiredCollections": {},
        "requiredObjects": {},
        "plantSymmetry": {},
        "packedImages": [],
        "negativeScaleObjects": [],
        "floatingFurniture": [],
    }

    for name in contract.REQUIRED_COLLECTIONS:
        collection = bpy.data.collections.get(name)
        collection_path = (
            _find_collection_path(scene.collection, collection)
            if collection is not None
            else None
        )
        present = collection_path is not None
        report["requiredCollections"][name] = present
        if not present:
            report["errors"].append(
                f"Required collection '{name}' is not reachable from the active scene"
            )
            continue
        if name in PRODUCTION_COLLECTIONS:
            for path_collection in collection_path:
                if path_collection.hide_render:
                    report["errors"].append(
                        f"{path_collection.name} hide_render must be False on the "
                        f"collection path to {name}"
                    )
            layer_path = _find_layer_collection_path(
                bpy.context.view_layer.layer_collection,
                collection,
            )
            if layer_path is None:
                report["errors"].append(
                    f"{name} has no reachable LayerCollection in the active view layer"
                )
            else:
                if any(layer_collection.exclude for layer_collection in layer_path):
                    report["errors"].append(
                        f"{name} LayerCollection exclude must be False"
                    )
                if any(
                    layer_collection.hide_viewport for layer_collection in layer_path
                ):
                    report["errors"].append(
                        f"{name} LayerCollection hide_viewport must be False"
                    )

    required_objects = tuple(
        dict.fromkeys(
            (
                *contract.REQUIRED_OBJECTS,
                *MARKER_SPECS,
                *contract.CAMERA_SPECS,
                *MODE_CAMERA_NAMES,
                "FloorPlant_Left",
                "FloorPlant_Right",
                *CRITICAL_PRODUCTION_OBJECTS,
                *LIGHT_SPECS,
                *BRAND_OBJECTS,
            )
        )
    )
    _record_required(report, "requiredObjects", required_objects)
    _validate_authored_manifest(report)
    _validate_markers(report)
    _validate_cameras(report)
    _validate_lights(report)
    _validate_brand(report)
    _validate_plants(report)
    _validate_materials_and_hierarchy(report, require_packed_brand)
    _validate_no_character(report)
    _validate_floor_contacts(report)
    _validate_render_settings(report)

    report["packedImages"].sort()
    return report


def _arguments_after_separator(argv: list[str]) -> list[str]:
    return argv[argv.index("--") + 1 :] if "--" in argv else argv[1:]


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("output_json", type=Path, help="QA report JSON path")
    parser.add_argument(
        "--allow-unpacked-brand",
        action="store_true",
        help="Deprecated compatibility flag; the studio no longer uses external brand images",
    )
    raw_argv = sys.argv if argv is None else argv
    args = parser.parse_args(_arguments_after_separator(raw_argv))
    report = validate_scene(require_packed_brand=not args.allow_unpacked_brand)
    output = args.output_json.expanduser().resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(
        json.dumps(report, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    print(f"WARM_STUDIO_VALIDATION={output}")
    return 1 if report["errors"] else 0


if __name__ == "__main__":
    raise SystemExit(main())
