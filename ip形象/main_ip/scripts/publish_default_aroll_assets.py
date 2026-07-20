#!/usr/bin/env python3
"""Publish and audit the approved character/studio pair without touching source.

Run inside Blender 4.x or newer. The input Blend is opened by Blender before
this script starts; every export happens in a disposable background process.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import struct
import sys
import traceback
from pathlib import Path
from typing import Any, Iterable

import bpy
from mathutils import Vector


FORMAL_COLLECTION = "COL_CHR_SLOTH_FINAL"
MASTER_COLLECTION = "IP_Character_Master"
MASTER_VERSION_PROPERTY = "ip_aroll_master_version"
MASTER_VERSION = 1
REQUIRED_STUDIO_MARKERS = ("IP_Character_Spawn", "IP_Focus_Head")
STANDING_STUDIO_MARKERS = (
    "IP_Standing_Spawn",
    "IP_Standing_Focus_Head",
    "IP_Transition_Focus",
    "IP_Seat_Target",
    "IP_Standing_Foot_Target.L",
    "IP_Standing_Foot_Target.R",
)
STANDING_STUDIO_CAMERAS = (
    "Camera_Standing_Wide",
    "Camera_Standing_Medium",
    "Camera_Standing_Close",
    "Camera_Standing_ThreeQuarter",
    "Camera_Standing_Transition",
)
RUNTIME_ORAL_ROLES = (
    "oral_cavity",
    "upper_teeth",
    "lower_teeth",
    "upper_gum",
    "lower_gum",
    "tongue",
)
SCHEMA_VERSION = "tangying-default-aroll-blender-audit/v1"


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--kind", choices=("source", "character", "studio"), required=True)
    parser.add_argument("--output")
    parser.add_argument("--report", required=True)
    parser.add_argument("--audit-only", action="store_true")
    argv = sys.argv[sys.argv.index("--") + 1 :] if "--" in sys.argv else []
    return parser.parse_args(argv)


def _collection_tree(collection: bpy.types.Collection) -> set[bpy.types.Collection]:
    result: set[bpy.types.Collection] = {collection}
    stack = list(collection.children)
    while stack:
        child = stack.pop()
        if child in result:
            continue
        result.add(child)
        stack.extend(child.children)
    return result


def _driver_target_objects(owner: Any) -> set[bpy.types.Object]:
    targets: set[bpy.types.Object] = set()
    animation_data = getattr(owner, "animation_data", None)
    for fcurve in getattr(animation_data, "drivers", ()) or ():
        for variable in fcurve.driver.variables:
            for target in variable.targets:
                target_id = getattr(target, "id", None)
                if isinstance(target_id, bpy.types.Object):
                    targets.add(target_id)
    return targets


def _object_dependencies(obj: bpy.types.Object) -> set[bpy.types.Object]:
    dependencies: set[bpy.types.Object] = set()
    if obj.parent is not None and obj.name != "IP_Character_Container":
        dependencies.add(obj.parent)
    for constraint in obj.constraints:
        target = getattr(constraint, "target", None)
        if isinstance(target, bpy.types.Object):
            dependencies.add(target)
    for modifier in obj.modifiers:
        for property_name in ("object", "target", "origin", "mirror_object", "offset_object"):
            target = getattr(modifier, property_name, None)
            if isinstance(target, bpy.types.Object):
                dependencies.add(target)
    dependencies.update(_driver_target_objects(obj))
    dependencies.update(_driver_target_objects(obj.data) if obj.data is not None else ())
    shape_keys = getattr(obj.data, "shape_keys", None) if obj.data is not None else None
    if shape_keys is not None:
        dependencies.update(_driver_target_objects(shape_keys))
    return dependencies


def character_objects() -> set[bpy.types.Object]:
    formal = bpy.data.collections.get(FORMAL_COLLECTION)
    if formal is None:
        raise RuntimeError(f"missing required collection {FORMAL_COLLECTION}")
    keep = set(formal.all_objects)
    existing_master = bpy.data.collections.get(MASTER_COLLECTION)
    if existing_master is not None:
        keep.update(existing_master.all_objects)

    changed = True
    while changed:
        changed = False
        for obj in list(keep):
            for dependency in _object_dependencies(obj):
                if dependency not in keep:
                    keep.add(dependency)
                    changed = True
        for obj in bpy.data.objects:
            if obj in keep:
                continue
            references_character = obj.parent in keep
            if not references_character:
                references_character = bool(_object_dependencies(obj) & keep)
            if references_character and obj.type not in {"CAMERA", "LIGHT"}:
                keep.add(obj)
                changed = True

    keep.update(obj for obj in bpy.data.objects if obj.type == "ARMATURE")
    return {
        obj
        for obj in keep
        if not obj.name.startswith("CXR_") and obj.type not in {"CAMERA", "LIGHT"}
    }


def _set_active_scene(scene: bpy.types.Scene) -> None:
    if bpy.context.window is not None:
        bpy.context.window.scene = scene


def _purge_orphans(*, preserve_actions: bool) -> int:
    if preserve_actions:
        for action in bpy.data.actions:
            action.use_fake_user = True
    purge = getattr(bpy.data, "orphans_purge", None)
    if purge is None:
        return 0
    result = purge(do_local_ids=True, do_linked_ids=True, do_recursive=True)
    return int(result or 0)


def _gum_material() -> bpy.types.Material:
    material = bpy.data.materials.get("MAT_Gums")
    if material is None:
        material = bpy.data.materials.new("MAT_Gums")
        material.use_nodes = True
    principled = next(
        (node for node in material.node_tree.nodes if node.type == "BSDF_PRINCIPLED"),
        None,
    )
    if principled is not None:
        principled.inputs["Base Color"].default_value = (0.24, 0.055, 0.045, 1.0)
        principled.inputs["Roughness"].default_value = 0.58
        if "Specular IOR Level" in principled.inputs:
            principled.inputs["Specular IOR Level"].default_value = 0.28
    return material


def _ensure_runtime_gum_roles(formal: bpy.types.Collection) -> list[str]:
    role_objects = {
        str(obj.get("ip_face_topology_role") or ""): obj
        for obj in formal.all_objects
        if obj.type == "MESH" and obj.get("ip_face_topology_role")
    }
    created: list[str] = []
    material = _gum_material()
    specifications = (
        ("upper_gum", "upper_teeth", "GEO_GumUpper"),
        ("lower_gum", "lower_teeth", "GEO_GumLower"),
    )
    for role, source_role, name in specifications:
        if role in role_objects:
            continue
        source = role_objects.get(source_role)
        if source is None:
            raise RuntimeError(f"cannot derive {role}: missing {source_role}")
        gum = source.copy()
        gum.data = source.data.copy()
        gum.name = name
        gum.data.name = f"{name}_Mesh"
        gum.animation_data_clear()
        if gum.data.shape_keys is not None:
            raise RuntimeError(f"cannot derive {role} from Shape Key geometry")
        for collection in tuple(gum.users_collection):
            collection.objects.unlink(gum)
        formal.objects.link(gum)
        for key in tuple(gum.keys()):
            del gum[key]
        gum["ip_face_topology_role"] = role
        gum["ip_published_runtime_component"] = True
        gum["formal_material_system"] = "COL_CHR_SLOTH_FINAL"
        gum.data.materials.clear()
        gum.data.materials.append(material)

        minimum = Vector(
            tuple(min(vertex.co[axis] for vertex in gum.data.vertices) for axis in range(3))
        )
        maximum = Vector(
            tuple(max(vertex.co[axis] for vertex in gum.data.vertices) for axis in range(3))
        )
        center = (minimum + maximum) * 0.5
        for vertex in gum.data.vertices:
            delta = vertex.co - center
            delta.x *= 1.06
            delta.y *= 1.10
            delta.z *= 1.10
            vertex.co = center + delta
        gum.data.update()
        role_objects[role] = gum
        created.append(gum.name)
    return created


def export_character() -> dict[str, Any]:
    formal = bpy.data.collections.get(FORMAL_COLLECTION)
    if formal is None:
        raise RuntimeError(f"missing required collection {FORMAL_COLLECTION}")
    created_runtime_components = _ensure_runtime_gum_roles(formal)
    keep_objects = character_objects()
    if not any(obj.type == "ARMATURE" for obj in keep_objects):
        raise RuntimeError("formal character dependency closure has no Armature")

    for obj in keep_objects:
        if obj.parent is not None and obj.parent not in keep_objects:
            matrix_world = obj.matrix_world.copy()
            obj.parent = None
            obj.matrix_world = matrix_world
    for action in list(bpy.data.actions):
        if action.name.startswith("CXR_"):
            bpy.data.actions.remove(action)

    master = bpy.data.collections.get(MASTER_COLLECTION)
    if master is None:
        master = bpy.data.collections.new(MASTER_COLLECTION)
    master[MASTER_VERSION_PROPERTY] = MASTER_VERSION
    if master.children.get(formal.name) is None:
        master.children.link(formal)
    for obj in keep_objects:
        if master.objects.get(obj.name) is None:
            master.objects.link(obj)

    scene = bpy.data.scenes.new("SCENE_CHARACTER_MASTER")
    scene.collection.children.link(master)
    _set_active_scene(scene)
    for old_scene in list(bpy.data.scenes):
        if old_scene != scene:
            bpy.data.scenes.remove(old_scene)

    for obj in list(bpy.data.objects):
        if obj not in keep_objects:
            bpy.data.objects.remove(obj, do_unlink=True)

    keep_collections = _collection_tree(formal) | _collection_tree(master)
    for collection in list(bpy.data.collections):
        if collection not in keep_collections:
            bpy.data.collections.remove(collection, do_unlink=True)

    scene.camera = None
    scene.frame_start = 1
    scene.frame_end = 1
    scene.frame_set(1)
    bpy.context.view_layer.update()
    return {
        "orphanDatablocksPurged": _purge_orphans(preserve_actions=True),
        "createdRuntimeComponents": created_runtime_components,
    }


def _studio_scene() -> bpy.types.Scene:
    candidates = [
        scene
        for scene in bpy.data.scenes
        if all(scene.objects.get(name) is not None for name in REQUIRED_STUDIO_MARKERS)
    ]
    if not candidates:
        raise RuntimeError("no scene contains the required studio markers")
    return min(candidates, key=lambda scene: (scene.name != "SCENE_PRODUCTION", scene.name))


def _copy_custom_properties(source: bpy.types.Object, target: bpy.types.Object) -> None:
    for key in source.keys():
        target[key] = source[key]


def _ensure_empty(
    scene: bpy.types.Scene,
    name: str,
    source: bpy.types.Object,
    *,
    location: Vector | None = None,
) -> bpy.types.Object:
    existing = bpy.data.objects.get(name)
    if existing is not None:
        return existing
    marker = bpy.data.objects.new(name, None)
    scene.collection.objects.link(marker)
    marker.matrix_world = source.matrix_world.copy()
    if location is not None:
        marker.matrix_world.translation = location
    marker.empty_display_type = "SPHERE"
    marker.empty_display_size = 0.08
    _copy_custom_properties(source, marker)
    return marker


def _ensure_camera(
    scene: bpy.types.Scene,
    name: str,
    source: bpy.types.Object,
) -> bpy.types.Object:
    existing = bpy.data.objects.get(name)
    if existing is not None:
        return existing
    camera = source.copy()
    camera.data = source.data.copy()
    camera.name = name
    camera.data.name = f"{name}_Data"
    camera.animation_data_clear()
    camera.data.animation_data_clear()
    scene.collection.objects.link(camera)
    return camera


def _ensure_standing_aroll_contract(scene: bpy.types.Scene) -> dict[str, Any]:
    spawn = bpy.data.objects["IP_Character_Spawn"]
    focus = bpy.data.objects["IP_Focus_Head"]
    chair = bpy.data.objects.get("Chair_Seat")
    seat_location = chair.matrix_world.translation.copy() if chair else spawn.matrix_world.translation.copy()
    seat_location.z = float(seat_location.z + 0.08)

    _ensure_empty(scene, "IP_Standing_Spawn", spawn)
    _ensure_empty(scene, "IP_Standing_Focus_Head", focus)
    _ensure_empty(scene, "IP_Transition_Focus", focus)
    _ensure_empty(scene, "IP_Seat_Target", chair or spawn, location=seat_location)
    for suffix, offset in (("L", -0.22), ("R", 0.22)):
        foot_location = spawn.matrix_world.translation + Vector((offset, 0.0, 0.0))
        _ensure_empty(
            scene,
            f"IP_Standing_Foot_Target.{suffix}",
            spawn,
            location=foot_location,
        )

    camera_sources = {
        "Camera_Standing_Wide": "Camera_Wide",
        "Camera_Standing_Medium": "Camera_Medium",
        "Camera_Standing_Close": "Camera_Close",
        "Camera_Standing_ThreeQuarter": "Camera_ThreeQuarter_Left",
        "Camera_Standing_Transition": "Camera_Wide",
    }
    for name, source_name in camera_sources.items():
        source = bpy.data.objects.get(source_name)
        if source is None or source.type != "CAMERA":
            raise RuntimeError(f"missing camera required for standing A-roll contract: {source_name}")
        _ensure_camera(scene, name, source)

    scene["ip_scene_contract"] = "tangying-default-sloth-aroll-studio/v1"
    scene["ip_presentation_modes"] = json.dumps(["standing"])
    scene.render.resolution_x = 1920
    scene.render.resolution_y = 1080
    scene.render.resolution_percentage = 100
    scene.camera = bpy.data.objects["Camera_Standing_Medium"]
    return {
        "sceneContract": scene["ip_scene_contract"],
        "presentationModes": ["standing"],
        "standingMarkers": list(STANDING_STUDIO_MARKERS),
        "standingCameras": list(STANDING_STUDIO_CAMERAS),
    }


def export_studio() -> dict[str, Any]:
    studio_scene = _studio_scene()
    contract = _ensure_standing_aroll_contract(studio_scene)
    remove_objects = character_objects()
    remove_objects.update(obj for obj in bpy.data.objects if obj.type == "ARMATURE")
    remove_objects.update(obj for obj in bpy.data.objects if obj.name.startswith("CXR_"))
    _set_active_scene(studio_scene)
    for scene in list(bpy.data.scenes):
        if scene != studio_scene:
            bpy.data.scenes.remove(scene)
    for obj in list(remove_objects):
        if obj.name in bpy.data.objects:
            bpy.data.objects.remove(obj, do_unlink=True)

    formal = bpy.data.collections.get(FORMAL_COLLECTION)
    master = bpy.data.collections.get(MASTER_COLLECTION)
    collections_to_remove: set[bpy.types.Collection] = set()
    if formal is not None:
        collections_to_remove.update(_collection_tree(formal))
    if master is not None:
        collections_to_remove.update(_collection_tree(master))
    collections_to_remove.update(
        collection for collection in bpy.data.collections if collection.name.startswith("CXR_")
    )
    for collection in sorted(collections_to_remove, key=lambda item: item.name, reverse=True):
        if collection.name in bpy.data.collections:
            bpy.data.collections.remove(collection, do_unlink=True)
    for action in list(bpy.data.actions):
        if action.name.startswith("CXR_"):
            bpy.data.actions.remove(action)

    studio_scene.frame_start = 1
    studio_scene.frame_end = 1
    studio_scene.frame_set(1)
    bpy.context.view_layer.update()
    return {
        "orphanDatablocksPurged": _purge_orphans(preserve_actions=False),
        **contract,
    }


def _vertex_fingerprint(obj: bpy.types.Object) -> str:
    digest = hashlib.sha256()
    digest.update(obj.name.encode("utf-8"))
    for vertex in obj.data.vertices:
        digest.update(struct.pack("<3d", *[float(value) for value in vertex.co]))
    return digest.hexdigest()


def _driver_records() -> list[dict[str, Any]]:
    records: list[dict[str, Any]] = []
    owners: list[tuple[str, Any]] = []
    for obj in bpy.data.objects:
        owners.append((f"OBJECT:{obj.name}", obj))
        if obj.data is not None:
            owners.append((f"DATA:{obj.data.name}", obj.data))
            shape_keys = getattr(obj.data, "shape_keys", None)
            if shape_keys is not None:
                owners.append((f"SHAPE_KEYS:{shape_keys.name}", shape_keys))
    seen: set[tuple[int, str, int]] = set()
    for owner_name, owner in owners:
        animation_data = getattr(owner, "animation_data", None)
        for fcurve in getattr(animation_data, "drivers", ()) or ():
            key = (owner.as_pointer(), fcurve.data_path, int(fcurve.array_index))
            if key in seen:
                continue
            seen.add(key)
            records.append(
                {
                    "owner": owner_name,
                    "dataPath": fcurve.data_path,
                    "arrayIndex": int(fcurve.array_index),
                    "variableCount": len(fcurve.driver.variables),
                }
            )
    return sorted(records, key=lambda item: (item["owner"], item["dataPath"], item["arrayIndex"]))


def audit(kind: str) -> dict[str, Any]:
    current_path = Path(bpy.data.filepath).expanduser().resolve()
    formal = bpy.data.collections.get(FORMAL_COLLECTION)
    master = bpy.data.collections.get(MASTER_COLLECTION)
    formal_objects = sorted(obj.name for obj in formal.all_objects) if formal else []
    master_objects = sorted(obj.name for obj in master.all_objects) if master else []
    closure_objects = sorted(obj.name for obj in character_objects()) if formal else []
    formal_collections = [
        collection.name for collection in bpy.data.collections if collection.name == FORMAL_COLLECTION
    ]
    master_collections = [
        collection.name for collection in bpy.data.collections if collection.name == MASTER_COLLECTION
    ]
    mesh_objects = sorted((obj for obj in bpy.data.objects if obj.type == "MESH"), key=lambda obj: obj.name)
    shape_key_order = {
        obj.name: [block.name for block in obj.data.shape_keys.key_blocks]
        for obj in mesh_objects
        if obj.data.shape_keys is not None
    }
    uv_layers = {obj.name: [layer.name for layer in obj.data.uv_layers] for obj in mesh_objects}
    vertex_fingerprints = {
        obj.name: {
            "vertices": len(obj.data.vertices),
            "edges": len(obj.data.edges),
            "polygons": len(obj.data.polygons),
            "orderedCoordinateSha256": _vertex_fingerprint(obj),
        }
        for obj in mesh_objects
    }
    armatures = sorted(obj.name for obj in bpy.data.objects if obj.type == "ARMATURE")
    cameras = sorted(obj.name for obj in bpy.data.objects if obj.type == "CAMERA")
    lights = sorted(obj.name for obj in bpy.data.objects if obj.type == "LIGHT")
    markers = {name: bpy.data.objects.get(name) is not None for name in REQUIRED_STUDIO_MARKERS}
    standing_markers = {
        name: bpy.data.objects.get(name) is not None for name in STANDING_STUDIO_MARKERS
    }
    standing_cameras = {
        name: bpy.data.objects.get(name) is not None for name in STANDING_STUDIO_CAMERAS
    }
    driver_records = _driver_records()
    action_names = sorted(action.name for action in bpy.data.actions)
    material_names = sorted(material.name for material in bpy.data.materials)
    groom_objects = sorted(
        obj.name
        for obj in bpy.data.objects
        if obj.type in {"CURVE", "CURVES"} or obj.name.startswith("FUR_")
    )
    face_topology_roles = {
        role: sorted(
            obj.name
            for obj in bpy.data.objects
            if obj.type == "MESH" and str(obj.get("ip_face_topology_role") or "") == role
        )
        for role in RUNTIME_ORAL_ROLES
    }
    errors: list[str] = []
    if kind == "character":
        if len(formal_collections) != 1:
            errors.append("character master must contain exactly one COL_CHR_SLOTH_FINAL")
        if len(master_collections) != 1:
            errors.append("character master must contain exactly one IP_Character_Master")
        if len(armatures) != 1:
            errors.append("character master must contain exactly one Armature")
        if cameras:
            errors.append("character master must not contain cameras")
        if lights:
            errors.append("character master must not contain lights")
        if not shape_key_order:
            errors.append("character master must preserve Shape Keys")
        if not any(uv_layers.values()):
            errors.append("character master must preserve UV layers")
        invalid_oral_roles = [
            role for role, objects in face_topology_roles.items() if len(objects) != 1
        ]
        if invalid_oral_roles:
            errors.append(
                "character master must contain each runtime oral role exactly once: "
                + ", ".join(invalid_oral_roles)
            )
    elif kind == "studio":
        if formal_collections or master_collections:
            errors.append("studio template must not contain formal character collections")
        if armatures:
            errors.append("studio template must not contain an Armature")
        if not all(markers.values()):
            errors.append("studio template is missing required spawn/focus markers")
        if not all(standing_markers.values()):
            errors.append("studio template is missing the standing A-roll marker contract")
        if not all(standing_cameras.values()):
            errors.append("studio template is missing the standing A-roll camera contract")
        try:
            presentation_modes = json.loads(
                str(bpy.context.scene.get("ip_presentation_modes") or "[]")
            )
        except json.JSONDecodeError:
            presentation_modes = []
        if presentation_modes != ["standing"]:
            errors.append("studio template must declare standing as its supported presentation mode")
        if len(cameras) < 3:
            errors.append("studio template must contain at least three cameras")
        if len(lights) < 3:
            errors.append("studio template must contain at least three lights")

    scene = bpy.context.scene
    return {
        "schemaVersion": SCHEMA_VERSION,
        "status": "PASS" if not errors else "FAIL",
        "kind": kind,
        "errors": errors,
        "filePath": str(current_path),
        "fileBytes": current_path.stat().st_size if current_path.is_file() else None,
        "fileSha256": sha256_file(current_path) if current_path.is_file() else "",
        "blenderVersion": bpy.app.version_string,
        "pythonVersion": sys.version.split()[0],
        "sceneNames": sorted(scene.name for scene in bpy.data.scenes),
        "activeScene": scene.name,
        "collections": sorted(collection.name for collection in bpy.data.collections),
        "formalCollections": formal_collections,
        "masterCollections": master_collections,
        "formalObjects": formal_objects,
        "masterObjects": master_objects,
        "characterDependencyClosure": closure_objects,
        "objectHierarchy": {
            obj.name: {
                "type": obj.type,
                "parent": obj.parent.name if obj.parent else "",
                "collections": sorted(collection.name for collection in obj.users_collection),
            }
            for obj in bpy.data.objects
        },
        "objects": sorted(obj.name for obj in bpy.data.objects),
        "objectCounts": {
            object_type: sum(obj.type == object_type for obj in bpy.data.objects)
            for object_type in sorted({obj.type for obj in bpy.data.objects})
        },
        "armatures": armatures,
        "cameras": cameras,
        "lights": lights,
        "requiredMarkers": markers,
        "standingMarkers": standing_markers,
        "standingCameras": standing_cameras,
        "sceneContract": str(scene.get("ip_scene_contract") or ""),
        "presentationModes": json.loads(
            str(scene.get("ip_presentation_modes") or "[]")
        ),
        "meshVertexOrder": vertex_fingerprints,
        "shapeKeyOrder": shape_key_order,
        "shapeKeyCount": sum(len(order) for order in shape_key_order.values()),
        "uvLayers": uv_layers,
        "driverCount": len(driver_records),
        "drivers": driver_records,
        "actions": action_names,
        "materials": material_names,
        "groomObjects": groom_objects,
        "faceTopologyRoles": face_topology_roles,
        "render": {
            "engine": scene.render.engine,
            "resolutionX": scene.render.resolution_x,
            "resolutionY": scene.render.resolution_y,
            "fps": scene.render.fps,
            "viewTransform": scene.view_settings.view_transform,
            "look": scene.view_settings.look,
        },
    }


def write_report(path: Path, payload: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8")


def main() -> None:
    args = _parse_args()
    report_path = Path(args.report).expanduser().resolve()
    source_path = Path(bpy.data.filepath).expanduser().resolve()
    source_sha256 = sha256_file(source_path)
    try:
        mutation: dict[str, Any] = {}
        if not args.audit_only:
            if args.kind == "source":
                raise ValueError("--kind source is audit-only")
            if not args.output:
                raise ValueError("--output is required for publication")
            output_path = Path(args.output).expanduser().resolve()
            output_path.parent.mkdir(parents=True, exist_ok=True)
            mutation = export_character() if args.kind == "character" else export_studio()
            bpy.ops.wm.save_as_mainfile(filepath=str(output_path))
            bpy.ops.wm.open_mainfile(filepath=str(output_path), load_ui=False)

        result = audit(args.kind)
        result.update(
            {
                "sourcePath": str(source_path),
                "sourceSha256": source_sha256,
                "outputPath": result["filePath"],
                "outputSha256": result["fileSha256"],
                "auditOnly": bool(args.audit_only),
                "mutationSummary": mutation,
            }
        )
        write_report(report_path, result)
        if result["status"] != "PASS":
            raise RuntimeError("; ".join(result["errors"]))
        print(json.dumps({"status": result["status"], "report": str(report_path)}))
    except Exception as exc:
        failure = {
            "schemaVersion": SCHEMA_VERSION,
            "status": "FAIL",
            "kind": args.kind,
            "sourcePath": str(source_path),
            "sourceSha256": source_sha256,
            "error": str(exc),
            "traceback": traceback.format_exc(),
        }
        write_report(report_path, failure)
        raise


if __name__ == "__main__":
    main()
