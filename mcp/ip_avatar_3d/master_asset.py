#!/usr/bin/env python3
"""Stable Blender collection helpers for curated IP character masters."""

from __future__ import annotations

from pathlib import Path
from typing import Any, Iterable

try:
    import bpy
except ImportError:  # pragma: no cover - pure validation tests run without Blender.
    bpy = None  # type: ignore[assignment]


MASTER_COLLECTION = "IP_Character_Master"
MASTER_VERSION_PROPERTY = "ip_aroll_master_version"
MASTER_VERSION = 1
MASTER_QA_CAMERAS = {
    "Camera_Medium": {"lens": 58.0, "distance_scale": 2.1, "target_height": 0.64},
    "Camera_Wide": {"lens": 50.0, "distance_scale": 3.2, "target_height": 0.52},
}


def _require_bpy() -> Any:
    if bpy is None:
        raise RuntimeError("Blender bpy is required for master asset operations")
    return bpy


def _unique_objects(objects: Iterable[Any]) -> list[Any]:
    unique: list[Any] = []
    seen: set[int] = set()
    for obj in objects:
        identity = id(obj)
        if identity in seen:
            continue
        seen.add(identity)
        unique.append(obj)
    return unique


def _require_one_armature(objects: Iterable[Any], source: Path | str) -> Any:
    armatures = [obj for obj in _unique_objects(objects) if getattr(obj, "type", "") == "ARMATURE"]
    if not armatures:
        raise RuntimeError(f"{source} must contain exactly one Armature")
    if len(armatures) > 1:
        names = ", ".join(str(getattr(obj, "name", "<unnamed>")) for obj in armatures)
        raise RuntimeError(f"{source} contains duplicate Armatures: {names}")
    return armatures[0]


def validate_master_collection(collection: Any, master_path: Path | str) -> dict[str, Any]:
    """Fail closed unless a loaded master has supported metadata and one rig."""
    if collection is None or getattr(collection, "name", "") != MASTER_COLLECTION:
        raise RuntimeError(f"missing {MASTER_COLLECTION} in {master_path}")
    version = collection.get(MASTER_VERSION_PROPERTY)
    if version is None:
        raise RuntimeError(f"missing {MASTER_VERSION_PROPERTY} in {master_path}")
    if type(version) is not int:
        raise RuntimeError(
            f"invalid {MASTER_VERSION_PROPERTY}={version!r} in {master_path}; expected an integer"
        )
    resolved_version = version
    if resolved_version != MASTER_VERSION:
        raise RuntimeError(
            f"unsupported {MASTER_VERSION_PROPERTY}={resolved_version} in {master_path}; expected {MASTER_VERSION}"
        )
    objects = list(collection.all_objects)
    armature = _require_one_armature(objects, master_path)
    return {
        "collection": MASTER_COLLECTION,
        "version": resolved_version,
        "armature": armature,
        "objects": objects,
    }


def ensure_master_collection(character_objects: Iterable[Any], armature: Any) -> Any:
    """Move one complete character hierarchy into the named master collection."""
    blender = _require_bpy()
    objects = _unique_objects([*character_objects, armature])
    selected_armature = _require_one_armature(objects, "character master")
    if selected_armature is not armature:
        raise RuntimeError("character master armature does not match the supplied Armature")
    if blender.data.collections.get(MASTER_COLLECTION) is not None:
        raise RuntimeError(f"duplicate {MASTER_COLLECTION} collection")

    collection = blender.data.collections.new(MASTER_COLLECTION)
    blender.context.scene.collection.children.link(collection)
    for obj in objects:
        if collection.objects.get(obj.name) is None:
            collection.objects.link(obj)
        for owner in list(getattr(obj, "users_collection", [])):
            if owner != collection:
                owner.objects.unlink(obj)
    collection[MASTER_VERSION_PROPERTY] = MASTER_VERSION
    validate_master_collection(collection, "current Blender file")
    return collection


def ensure_master_qa_cameras(armature: Any) -> dict[str, Any]:
    """Create deterministic standalone cameras without adding them to the master collection."""
    blender = _require_bpy()
    from mathutils import Vector

    rig_points = [
        armature.matrix_world @ point
        for bone in armature.data.bones
        for point in (bone.head_local, bone.tail_local)
    ]
    if not rig_points:
        raise RuntimeError("character master Armature has no bones for QA camera framing")

    minimum = Vector(tuple(min(point[index] for point in rig_points) for index in range(3)))
    maximum = Vector(tuple(max(point[index] for point in rig_points) for index in range(3)))
    height = maximum.z - minimum.z
    if height <= 1e-6:
        raise RuntimeError("character master Armature has zero height for QA camera framing")

    center_x = (minimum.x + maximum.x) * 0.5
    center_y = (minimum.y + maximum.y) * 0.5
    cameras: dict[str, Any] = {}
    for name, spec in MASTER_QA_CAMERAS.items():
        camera = blender.data.objects.get(name)
        if camera is not None and camera.type != "CAMERA":
            raise RuntimeError(f"cannot create {name}: name belongs to a non-camera object")
        if camera is None:
            camera_data = blender.data.cameras.new(f"{name}_Data")
            camera = blender.data.objects.new(name, camera_data)
        if blender.context.scene.objects.get(camera.name) is None:
            blender.context.scene.collection.objects.link(camera)

        target = Vector(
            (center_x, center_y, minimum.z + height * float(spec["target_height"]))
        )
        distance = max(0.5, height * float(spec["distance_scale"]))
        camera.location = target + Vector((0.0, -distance, height * 0.025))
        camera.rotation_euler = (target - camera.location).to_track_quat("-Z", "Y").to_euler()
        camera.data.lens = float(spec["lens"])
        camera.data.clip_start = 0.01
        camera.data.clip_end = max(100.0, distance * 10.0)
        cameras[name] = camera

    blender.context.scene.camera = cameras["Camera_Medium"]
    return cameras


def save_master_collection(
    *,
    character_objects: Iterable[Any],
    armature: Any,
    output_path: Path,
) -> dict[str, Any]:
    """Save a versioned, self-contained character master Blend."""
    objects = _unique_objects([*character_objects, armature])
    _require_one_armature(objects, "character master")
    output_path = Path(output_path).expanduser().resolve()
    if output_path.suffix.lower() != ".blend":
        raise RuntimeError(f"master output must be a .blend file: {output_path}")
    output_path.parent.mkdir(parents=True, exist_ok=True)
    collection = ensure_master_collection(objects, armature)
    collection[MASTER_VERSION_PROPERTY] = MASTER_VERSION
    ensure_master_qa_cameras(armature)
    _require_bpy().ops.wm.save_as_mainfile(filepath=str(output_path))
    return {
        "collection": collection.name,
        "version": MASTER_VERSION,
        "path": str(output_path),
    }


def append_master_collection(master_path: Path, scene_collection: Any) -> list[Any]:
    """Append and validate a curated master, including its reusable Actions."""
    blender = _require_bpy()
    master_path = Path(master_path).expanduser().resolve()
    if master_path.suffix.lower() != ".blend" or not master_path.is_file():
        raise RuntimeError(f"invalid master Blend: {master_path}")
    if blender.data.collections.get(MASTER_COLLECTION) is not None:
        raise RuntimeError(f"duplicate {MASTER_COLLECTION} collection while appending {master_path}")

    with blender.data.libraries.load(str(master_path), link=False) as (source, target):
        matches = [name for name in source.collections if name == MASTER_COLLECTION]
        if len(matches) != 1:
            if not matches:
                raise RuntimeError(f"missing {MASTER_COLLECTION} in {master_path}")
            raise RuntimeError(f"duplicate {MASTER_COLLECTION} collections in {master_path}")
        target.collections = [MASTER_COLLECTION]
        target.actions = list(source.actions)

    collection = target.collections[0] if target.collections else None
    validated = validate_master_collection(collection, master_path)
    scene_collection.children.link(collection)
    return list(validated["objects"])
