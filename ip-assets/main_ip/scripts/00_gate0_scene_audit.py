"""Gate 0 — non-destructive audit for the current Sloth Blender asset.

Allowed writes:
  * reports/scene_audit.json
  * reports/object_map.json
  * reports/rig_report.json
  * reports/shapekey_report.json
  * checkpoints/sloth_000_original.blend (copy=True; never overwritten)

This script does not add, remove, rename, transform, relink, or edit Blender
datablocks. It intentionally avoids mesh validation/apply operations because
those can mutate source data.
"""

from __future__ import annotations

import bpy
import hashlib
import json
import math
import os
import platform
import struct
import sys
from collections import Counter, defaultdict
from datetime import datetime, timezone
from pathlib import Path


PROJECT_ROOT = Path("/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip")
REPORTS_DIR = PROJECT_ROOT / "reports"
CHECKPOINTS_DIR = PROJECT_ROOT / "checkpoints"
CHECKPOINT_PATH = CHECKPOINTS_DIR / "sloth_000_original.blend"
REPORT_PATHS = {
    "scene_audit": REPORTS_DIR / "scene_audit.json",
    "object_map": REPORTS_DIR / "object_map.json",
    "rig_report": REPORTS_DIR / "rig_report.json",
    "shapekey_report": REPORTS_DIR / "shapekey_report.json",
}
EXPECTED_FINAL_OBJECTS = [
    "RIG_Sloth", "GEO_HeadBody", "GEO_Nose", "GEO_MouthInterior",
    "GEO_TeethUpper", "GEO_TeethLower", "GEO_Tongue",
    "EYE_Sclera_L", "EYE_Sclera_R", "EYE_Iris_L", "EYE_Iris_R",
    "EYE_Cornea_L", "EYE_Cornea_R", "EYE_TearLine_L", "EYE_TearLine_R",
    "CLO_Cardigan", "CLO_Hoodie", "CLO_Trousers", "CLO_Buttons",
    "CLO_Drawstrings", "FUR_Face", "FUR_Muzzle", "FUR_Body",
    "FUR_HandsFeet", "FUR_Ears", "FUR_Brows", "FUR_HeadTuft",
    "FUR_Outline",
]


def utc_now():
    return datetime.now(timezone.utc).isoformat()


def clean(value):
    if value is None or isinstance(value, (str, int, float, bool)):
        if isinstance(value, float) and not math.isfinite(value):
            return str(value)
        return value
    if isinstance(value, bytes):
        return value.decode("utf-8", errors="replace")
    if isinstance(value, dict):
        return {str(k): clean(v) for k, v in value.items()}
    if isinstance(value, (list, tuple, set)):
        return [clean(v) for v in value]
    try:
        return [clean(v) for v in value]
    except (TypeError, AttributeError):
        return str(value)


def matrix_rows(matrix):
    return [[round(float(v), 9) for v in row] for row in matrix]


def vector_values(vector):
    return [round(float(v), 9) for v in vector]


def write_json(path, payload):
    path.write_text(
        json.dumps(clean(payload), indent=2, ensure_ascii=False, sort_keys=False) + "\n",
        encoding="utf-8",
    )


def hash_float_triplets(values):
    digest = hashlib.sha256()
    for value in values:
        digest.update(struct.pack("<fff", float(value[0]), float(value[1]), float(value[2])))
    return digest.hexdigest()


def mesh_hashes(mesh):
    position_digest = hashlib.sha256()
    topology_digest = hashlib.sha256()
    for vertex in mesh.vertices:
        position_digest.update(struct.pack("<fff", *map(float, vertex.co)))
    topology_digest.update(struct.pack("<III", len(mesh.vertices), len(mesh.edges), len(mesh.polygons)))
    for edge in mesh.edges:
        topology_digest.update(struct.pack("<II", *map(int, edge.vertices)))
    for polygon in mesh.polygons:
        topology_digest.update(struct.pack("<I", len(polygon.vertices)))
        for index in polygon.vertices:
            topology_digest.update(struct.pack("<I", int(index)))
    uv_hashes = {}
    for layer in mesh.uv_layers:
        digest = hashlib.sha256()
        for loop_uv in layer.data:
            digest.update(struct.pack("<ff", float(loop_uv.uv[0]), float(loop_uv.uv[1])))
        uv_hashes[layer.name] = digest.hexdigest()
    return {
        "vertex_order_position_sha256": position_digest.hexdigest(),
        "topology_and_vertex_order_sha256": topology_digest.hexdigest(),
        "uv_layer_sha256": uv_hashes,
    }


def topology_summary(mesh):
    edge_face_count = [0] * len(mesh.edges)
    for polygon in mesh.polygons:
        for edge_index in polygon.edge_keys:
            pass
        for edge_index in polygon.loop_indices:
            edge_face_count[mesh.loops[edge_index].edge_index] += 1
    boundary = sum(1 for count in edge_face_count if count == 1)
    nonmanifold = sum(1 for count in edge_face_count if count != 2)
    loose = sum(1 for count in edge_face_count if count == 0)
    degenerate_faces = sum(1 for polygon in mesh.polygons if polygon.area <= 1.0e-12)
    ngon_histogram = Counter(len(polygon.vertices) for polygon in mesh.polygons)
    return {
        "vertices": len(mesh.vertices),
        "edges": len(mesh.edges),
        "polygons": len(mesh.polygons),
        "loops": len(mesh.loops),
        "boundary_edges": boundary,
        "nonmanifold_or_boundary_edges": nonmanifold,
        "loose_edges": loose,
        "degenerate_faces_area_epsilon": degenerate_faces,
        "polygon_sides": {str(k): v for k, v in sorted(ngon_histogram.items())},
    }


def serialize_constraint(constraint):
    result = {
        "name": constraint.name,
        "type": constraint.type,
        "mute": constraint.mute,
        "influence": float(constraint.influence),
        "target": getattr(getattr(constraint, "target", None), "name", None),
        "subtarget": getattr(constraint, "subtarget", ""),
        "owner_space": getattr(constraint, "owner_space", None),
        "target_space": getattr(constraint, "target_space", None),
    }
    return result


def serialize_driver_fcurve(fcurve, owner_label):
    driver = fcurve.driver
    variables = []
    for variable in driver.variables:
        targets = []
        for target in variable.targets:
            target_id = getattr(target, "id", None)
            targets.append({
                "id": getattr(target_id, "name", None),
                "id_type": getattr(target_id, "id_type", None),
                "data_path": getattr(target, "data_path", ""),
                "bone_target": getattr(target, "bone_target", ""),
                "transform_type": getattr(target, "transform_type", None),
                "transform_space": getattr(target, "transform_space", None),
            })
        variables.append({"name": variable.name, "type": variable.type, "targets": targets})
    return {
        "owner": owner_label,
        "data_path": fcurve.data_path,
        "array_index": fcurve.array_index,
        "mute": fcurve.mute,
        "driver_type": driver.type,
        "expression": driver.expression,
        "is_valid": driver.is_valid,
        "variables": variables,
    }


def animation_summary(id_data, owner_label):
    animation_data = getattr(id_data, "animation_data", None)
    if animation_data is None:
        return {"action": None, "drivers": [], "nla_tracks": []}
    action = getattr(animation_data, "action", None)
    tracks = []
    for track in animation_data.nla_tracks:
        tracks.append({
            "name": track.name,
            "mute": track.mute,
            "is_solo": track.is_solo,
            "strips": [{
                "name": strip.name,
                "action": getattr(getattr(strip, "action", None), "name", None),
                "frame_start": float(strip.frame_start),
                "frame_end": float(strip.frame_end),
                "blend_type": strip.blend_type,
                "influence": float(strip.influence),
            } for strip in track.strips],
        })
    return {
        "action": getattr(action, "name", None),
        "drivers": [serialize_driver_fcurve(fc, owner_label) for fc in animation_data.drivers],
        "nla_tracks": tracks,
    }


def serialize_modifier(modifier):
    result = {
        "name": modifier.name,
        "type": modifier.type,
        "show_viewport": modifier.show_viewport,
        "show_render": modifier.show_render,
    }
    for attr in (
        "object", "vertex_group", "levels", "render_levels", "subdivision_type",
        "use_deform_preserve_volume", "node_group", "thickness", "offset",
        "strength", "texture", "deform_method", "use_axis", "merge_threshold",
    ):
        if hasattr(modifier, attr):
            value = getattr(modifier, attr)
            result[attr] = getattr(value, "name", clean(value))
    return result


def classify_object(obj):
    name = obj.name.lower()
    roles = []
    if obj.type == "ARMATURE": roles.append("armature")
    if obj.type == "CAMERA": roles.append("production_or_test_camera")
    if obj.type in {"CURVES", "CURVE"} or any(t in name for t in ("fur", "hair", "groom", "brow", "tuft")):
        roles.append("groom_or_fur")
    if any(t in name for t in ("eye", "sclera", "iris", "pupil", "cornea", "tear")): roles.append("eye")
    if any(t in name for t in ("mouth", "oral", "teeth", "tooth", "tongue", "lip")): roles.append("mouth_oral")
    if any(t in name for t in ("cardigan", "hood", "trouser", "pants", "cloth", "shirt", "button", "string")): roles.append("clothing")
    if any(t in name for t in ("hand", "finger", "wrist", "nail", "foot", "toe")): roles.append("hands_or_feet")
    if any(t in name for t in ("head", "face", "muzzle", "cheek", "nose", "part_00000001")): roles.append("head_face_or_main_body")
    if any(t in name for t in ("body", "torso", "part_00000002")): roles.append("body_or_clothing_shell")
    temporary = name.startswith("cxr_") or any(t in name for t in ("temp", "preview", "fit_", "helper", "backup", "old", "deprecated"))
    return {"candidate_roles": roles, "temporary_or_refinement_name": temporary}


def mesh_object_summary(obj):
    mesh = obj.data
    hashes = mesh_hashes(mesh)
    return {
        "mesh_data": mesh.name,
        "topology": topology_summary(mesh),
        "hashes": hashes,
        "uv_layers": [{
            "name": layer.name,
            "active": mesh.uv_layers.active == layer,
            "active_render": layer.active_render,
            "loops": len(layer.data),
            "sha256": hashes["uv_layer_sha256"].get(layer.name),
        } for layer in mesh.uv_layers],
        "color_attributes": [{
            "name": attr.name,
            "domain": attr.domain,
            "data_type": attr.data_type,
        } for attr in getattr(mesh, "color_attributes", [])],
        "material_slots": [slot.material.name if slot.material else None for slot in obj.material_slots],
        "vertex_groups": [{
            "name": group.name,
            "index": group.index,
            "lock_weight": group.lock_weight,
        } for group in obj.vertex_groups],
        "shape_keys": getattr(getattr(mesh, "shape_keys", None), "name", None),
    }


def object_summary(obj):
    result = {
        "name": obj.name,
        "type": obj.type,
        "data": getattr(getattr(obj, "data", None), "name", None),
        "collections": sorted(collection.name for collection in obj.users_collection),
        "parent": getattr(obj.parent, "name", None),
        "parent_type": obj.parent_type,
        "parent_bone": obj.parent_bone,
        "location": vector_values(obj.location),
        "rotation_mode": obj.rotation_mode,
        "rotation_euler": vector_values(obj.rotation_euler),
        "scale": vector_values(obj.scale),
        "dimensions": vector_values(obj.dimensions),
        "matrix_world": matrix_rows(obj.matrix_world),
        "hide_viewport": obj.hide_viewport,
        "hide_render": obj.hide_render,
        "hide_get": obj.hide_get(),
        "visible_get": obj.visible_get(),
        "modifiers": [serialize_modifier(modifier) for modifier in obj.modifiers],
        "constraints": [serialize_constraint(constraint) for constraint in obj.constraints],
        "animation": animation_summary(obj, f"OBJECT:{obj.name}"),
        "classification": classify_object(obj),
    }
    if obj.type == "MESH":
        result["mesh"] = mesh_object_summary(obj)
    elif obj.type == "ARMATURE":
        result["armature"] = {"bones": len(obj.data.bones), "pose_bones": len(obj.pose.bones)}
    elif obj.type in {"CURVES", "CURVE"}:
        data = obj.data
        result["curve_data"] = {
            "splines_or_curves": len(getattr(data, "splines", getattr(data, "curves", []))),
            "points": len(getattr(data, "points", [])),
            "materials": [material.name if material else None for material in data.materials],
        }
    return result


def collection_summary(collection):
    return {
        "name": collection.name,
        "objects": sorted(obj.name for obj in collection.objects),
        "children": sorted(child.name for child in collection.children),
        "parent_collections": sorted(parent.name for parent in bpy.data.collections if collection.name in parent.children),
        "hide_viewport": collection.hide_viewport,
        "hide_render": collection.hide_render,
        "instance_offset": vector_values(collection.instance_offset),
    }


def material_summary(material):
    nodes = []
    links = 0
    if material.use_nodes and material.node_tree:
        nodes = [{"name": node.name, "type": node.bl_idname, "label": node.label} for node in material.node_tree.nodes]
        links = len(material.node_tree.links)
    return {
        "name": material.name,
        "users": material.users,
        "use_nodes": material.use_nodes,
        "blend_method": getattr(material, "surface_render_method", getattr(material, "blend_method", None)),
        "diffuse_color": vector_values(material.diffuse_color),
        "node_count": len(nodes),
        "link_count": links,
        "nodes": nodes,
        "animation": animation_summary(material, f"MATERIAL:{material.name}"),
    }


def image_summary(image):
    filepath = bpy.path.abspath(image.filepath) if image.filepath else ""
    return {
        "name": image.name,
        "source": image.source,
        "filepath": filepath,
        "exists": bool(filepath and Path(filepath).exists()),
        "packed": image.packed_file is not None,
        "size": list(image.size),
        "colorspace": image.colorspace_settings.name,
    }


def action_summary(action):
    legacy_fcurves = getattr(action, "fcurves", None)
    legacy_groups = getattr(action, "groups", None)
    layers = []
    layered_fcurve_count = 0
    for layer in getattr(action, "layers", []):
        layer_record = {"name": layer.name, "strips": []}
        for strip in getattr(layer, "strips", []):
            strip_record = {"type": getattr(strip, "type", None), "channelbags": []}
            for channelbag in getattr(strip, "channelbags", []):
                channelbag_fcurves = list(getattr(channelbag, "fcurves", []))
                layered_fcurve_count += len(channelbag_fcurves)
                strip_record["channelbags"].append({
                    "slot_handle": getattr(channelbag, "slot_handle", None),
                    "fcurves": len(channelbag_fcurves),
                })
            layer_record["strips"].append(strip_record)
        layers.append(layer_record)
    slots = []
    for slot in getattr(action, "slots", []):
        slots.append({
            "name": getattr(slot, "name", getattr(slot, "identifier", str(slot))),
            "identifier": getattr(slot, "identifier", None),
            "target_id_type": getattr(slot, "target_id_type", None),
            "handle": getattr(slot, "handle", None),
        })
    return {
        "name": action.name,
        "users": action.users,
        "frame_range": [float(action.frame_range[0]), float(action.frame_range[1])],
        "fcurves": len(legacy_fcurves) if legacy_fcurves is not None else layered_fcurve_count,
        "legacy_groups": [group.name for group in legacy_groups] if legacy_groups is not None else [],
        "slots": slots,
        "layers": layers,
    }


def bound_mesh_report(obj, armature_obj):
    bone_names = {bone.name for bone in armature_obj.data.bones if bone.use_deform}
    group_by_index = {group.index: group.name for group in obj.vertex_groups}
    total_weights = []
    deform_counts = []
    unweighted_deform = 0
    over_four = 0
    for vertex in obj.data.vertices:
        deform = []
        for membership in vertex.groups:
            group_name = group_by_index.get(membership.group)
            if group_name in bone_names and membership.weight > 0.0:
                deform.append(float(membership.weight))
        if not deform:
            unweighted_deform += 1
            total_weights.append(0.0)
        else:
            total_weights.append(sum(deform))
        deform_counts.append(len(deform))
        if len(deform) > 4:
            over_four += 1
    return {
        "object": obj.name,
        "armature": armature_obj.name,
        "vertices": len(obj.data.vertices),
        "vertex_groups": len(obj.vertex_groups),
        "deform_bones": len(bone_names),
        "unweighted_to_deform_bones": unweighted_deform,
        "vertices_over_four_deform_influences": over_four,
        "max_deform_influences": max(deform_counts, default=0),
        "weight_sum_min": min(total_weights, default=0.0),
        "weight_sum_max": max(total_weights, default=0.0),
        "weight_sum_mean": sum(total_weights) / len(total_weights) if total_weights else 0.0,
        "missing_deform_bone_groups": sorted(bone_names - set(group_by_index.values())),
        "extra_nonbone_groups": sorted(set(group_by_index.values()) - {bone.name for bone in armature_obj.data.bones}),
    }


def armature_report(obj):
    bones = []
    for bone in obj.data.bones:
        bones.append({
            "name": bone.name,
            "parent": getattr(bone.parent, "name", None),
            "use_deform": bone.use_deform,
            "use_connect": bone.use_connect,
            "head_local": vector_values(bone.head_local),
            "tail_local": vector_values(bone.tail_local),
            "roll": float(bone.matrix_local.to_euler().z),
        })
    pose_bones = []
    for pose_bone in obj.pose.bones:
        pose_bones.append({
            "name": pose_bone.name,
            "rotation_mode": pose_bone.rotation_mode,
            "constraints": [serialize_constraint(c) for c in pose_bone.constraints],
            "custom_shape": getattr(getattr(pose_bone, "custom_shape", None), "name", None),
        })
    return {
        "name": obj.name,
        "data": obj.data.name,
        "bones": bones,
        "pose_bones": pose_bones,
        "object_constraints": [serialize_constraint(c) for c in obj.constraints],
        "animation": animation_summary(obj, f"ARMATURE_OBJECT:{obj.name}"),
        "data_animation": animation_summary(obj.data, f"ARMATURE_DATA:{obj.data.name}"),
    }


def shapekey_mesh_report(mesh):
    keys = mesh.shape_keys
    blocks = []
    basis_count = len(mesh.vertices)
    for index, block in enumerate(keys.key_blocks):
        blocks.append({
            "index": index,
            "name": block.name,
            "relative_key": getattr(block.relative_key, "name", None),
            "vertex_count": len(block.data),
            "vertex_count_matches_mesh": len(block.data) == basis_count,
            "value": float(block.value),
            "slider_min": float(block.slider_min),
            "slider_max": float(block.slider_max),
            "mute": block.mute,
            "vertex_group": block.vertex_group,
            "interpolation": block.interpolation,
            "coordinate_sha256": hash_float_triplets(point.co for point in block.data),
        })
    users = sorted(obj.name for obj in bpy.data.objects if obj.type == "MESH" and obj.data == mesh)
    return {
        "mesh_data": mesh.name,
        "object_users": users,
        "mesh_vertex_count": basis_count,
        "shape_key_datablock": keys.name,
        "use_relative": keys.use_relative,
        "eval_time": float(keys.eval_time),
        "key_order": [block.name for block in keys.key_blocks],
        "key_blocks": blocks,
        "animation": animation_summary(keys, f"SHAPE_KEYS:{keys.name}"),
    }


def driver_inventory():
    results = []
    data_groups = [
        ("OBJECT", bpy.data.objects), ("MESH", bpy.data.meshes),
        ("ARMATURE", bpy.data.armatures), ("MATERIAL", bpy.data.materials),
        ("CURVE", bpy.data.curves), ("CAMERA", bpy.data.cameras),
        ("LIGHT", bpy.data.lights), ("WORLD", bpy.data.worlds),
        ("SCENE", bpy.data.scenes), ("KEY", bpy.data.shape_keys),
        ("NODE_GROUP", bpy.data.node_groups),
    ]
    for label, collection in data_groups:
        for datablock in collection:
            summary = animation_summary(datablock, f"{label}:{datablock.name}")
            results.extend(summary["drivers"])
    return results


def cycles_device_summary():
    result = {"available": False, "compute_device_type": None, "devices": [], "error": None}
    try:
        addon = bpy.context.preferences.addons.get("cycles")
        if addon is None:
            return result
        preferences = addon.preferences
        preferences.get_devices()
        result["available"] = True
        result["compute_device_type"] = preferences.compute_device_type
        result["devices"] = [{
            "name": device.name,
            "type": device.type,
            "use": device.use,
            "id": device.id,
        } for device in preferences.devices]
    except Exception as exc:
        result["error"] = f"{type(exc).__name__}: {exc}"
    return result


def gather_reports(source_filepath, source_size, checkpoint_size, checkpoint_created_this_run):
    all_objects = [object_summary(obj) for obj in bpy.data.objects]
    object_by_type = Counter(obj.type for obj in bpy.data.objects)
    mesh_objects = [obj for obj in bpy.data.objects if obj.type == "MESH"]
    armatures = [obj for obj in bpy.data.objects if obj.type == "ARMATURE"]
    groom_objects = [obj for obj in bpy.data.objects if obj.type in {"CURVES", "CURVE"} or "fur" in obj.name.lower() or "hair" in obj.name.lower()]
    drivers = driver_inventory()

    scenes = []
    for scene in bpy.data.scenes:
        scenes.append({
            "name": scene.name,
            "objects": sorted(obj.name for obj in scene.objects),
            "camera": getattr(scene.camera, "name", None),
            "world": getattr(scene.world, "name", None),
            "frame_start": scene.frame_start,
            "frame_end": scene.frame_end,
            "frame_current": scene.frame_current,
            "render": {
                "engine": scene.render.engine,
                "resolution_x": scene.render.resolution_x,
                "resolution_y": scene.render.resolution_y,
                "resolution_percentage": scene.render.resolution_percentage,
                "fps": scene.render.fps,
                "filepath": scene.render.filepath,
                "film_transparent": scene.render.film_transparent,
            },
            "view_settings": {
                "look": scene.view_settings.look,
                "view_transform": scene.view_settings.view_transform,
                "exposure": scene.view_settings.exposure,
                "gamma": scene.view_settings.gamma,
            },
            "cycles": {
                "device": getattr(scene.cycles, "device", None),
                "samples": getattr(scene.cycles, "samples", None),
                "use_denoising": getattr(scene.cycles, "use_denoising", None),
            } if hasattr(scene, "cycles") else None,
        })

    bound_meshes = []
    for mesh_obj in mesh_objects:
        linked_armatures = []
        for modifier in mesh_obj.modifiers:
            if modifier.type == "ARMATURE" and modifier.object:
                linked_armatures.append(modifier.object)
        if mesh_obj.parent and mesh_obj.parent.type == "ARMATURE":
            linked_armatures.append(mesh_obj.parent)
        seen = set()
        for armature_obj in linked_armatures:
            if armature_obj.name not in seen:
                bound_meshes.append(bound_mesh_report(mesh_obj, armature_obj))
                seen.add(armature_obj.name)

    shape_meshes = [shapekey_mesh_report(mesh) for mesh in bpy.data.meshes if mesh.shape_keys]
    key_count = sum(len(report["key_blocks"]) for report in shape_meshes)
    active_file = bpy.data.filepath
    active_scene = bpy.context.scene.name if bpy.context.scene else None
    active_object = bpy.context.view_layer.objects.active.name if bpy.context.view_layer.objects.active else None
    selected = sorted(obj.name for obj in bpy.context.selected_objects)

    expected_present = [name for name in EXPECTED_FINAL_OBJECTS if name in bpy.data.objects]
    expected_missing = [name for name in EXPECTED_FINAL_OBJECTS if name not in bpy.data.objects]
    exact_final_collection = bpy.data.collections.get("COL_CHR_SLOTH_FINAL")
    temp_candidates = sorted(obj.name for obj in bpy.data.objects if classify_object(obj)["temporary_or_refinement_name"])
    risks = []
    if len(armatures) != 1:
        risks.append({"severity": "HIGH", "code": "ARMATURE_COUNT", "message": f"Expected one formal armature; found {len(armatures)}."})
    if exact_final_collection is None:
        risks.append({"severity": "HIGH", "code": "FINAL_COLLECTION_MISSING", "message": "COL_CHR_SLOTH_FINAL does not yet exist."})
    if temp_candidates:
        risks.append({"severity": "MEDIUM", "code": "TEMP_OR_REFINEMENT_OBJECTS", "message": f"Found {len(temp_candidates)} temporary/refinement-name candidates; Gate 9 cleanup is required."})
    for item in bound_meshes:
        if item["unweighted_to_deform_bones"]:
            risks.append({"severity": "HIGH", "code": "UNWEIGHTED_BOUND_VERTICES", "object": item["object"], "count": item["unweighted_to_deform_bones"]})
        if item["vertices_over_four_deform_influences"]:
            risks.append({"severity": "MEDIUM", "code": "OVER_FOUR_INFLUENCES", "object": item["object"], "count": item["vertices_over_four_deform_influences"]})
    for report in shape_meshes:
        mismatches = [block["name"] for block in report["key_blocks"] if not block["vertex_count_matches_mesh"]]
        if mismatches:
            risks.append({"severity": "BLOCKER", "code": "SHAPEKEY_VERTEX_COUNT_MISMATCH", "mesh": report["mesh_data"], "keys": mismatches})

    scene_audit = {
        "schema": "sloth_gate0_scene_audit_v1",
        "generated_utc": utc_now(),
        "audit_mode": "read_only_except_reports_and_checkpoint_copy",
        "source": {
            "filepath_before_checkpoint": source_filepath,
            "filepath_after_checkpoint": active_file,
            "source_size_bytes": source_size,
            "checkpoint_path": str(CHECKPOINT_PATH),
            "checkpoint_size_bytes": checkpoint_size,
            "checkpoint_saved_with_copy_true": True,
            "checkpoint_created_this_run": checkpoint_created_this_run,
        },
        "blender": {
            "version": bpy.app.version_string,
            "version_tuple": list(bpy.app.version),
            "build_branch": clean(bpy.app.build_branch),
            "build_commit_date": clean(bpy.app.build_commit_date),
            "build_hash": clean(bpy.app.build_hash),
            "binary_path": bpy.app.binary_path,
            "python": sys.version,
        },
        "system": {
            "os": platform.platform(),
            "machine": platform.machine(),
            "processor": platform.processor(),
            "logical_cpu_count": os.cpu_count(),
            "cycles_devices": cycles_device_summary(),
            "hardware_profile_recommendation": "AUTO_CREATOR if reported Metal GPU and memory are adequate; otherwise AUTO_CONSTRAINED pending memory verification",
        },
        "context": {"active_scene": active_scene, "active_object": active_object, "selected_objects": selected},
        "counts": {
            "scenes": len(bpy.data.scenes), "collections": len(bpy.data.collections),
            "objects": len(bpy.data.objects), "objects_by_type": dict(sorted(object_by_type.items())),
            "meshes": len(bpy.data.meshes), "materials": len(bpy.data.materials),
            "images": len(bpy.data.images), "armatures": len(bpy.data.armatures),
            "actions": len(bpy.data.actions), "shape_key_datablocks": len(bpy.data.shape_keys),
            "shape_key_blocks_total": key_count, "drivers_total": len(drivers),
            "groom_candidates": len(groom_objects),
        },
        "scenes": scenes,
        "collections": [collection_summary(collection) for collection in bpy.data.collections],
        "materials": [material_summary(material) for material in bpy.data.materials],
        "images": [image_summary(image) for image in bpy.data.images],
        "libraries": [{"name": library.name, "filepath": bpy.path.abspath(library.filepath)} for library in bpy.data.libraries],
        "actions": [action_summary(action) for action in bpy.data.actions],
        "groom_candidates": [object_summary(obj) for obj in groom_objects],
        "risk_register": risks,
    }

    object_map = {
        "schema": "sloth_gate0_object_map_v1",
        "generated_utc": utc_now(),
        "source_filepath": source_filepath,
        "target_structure": {
            "collection": "COL_CHR_SLOTH_FINAL",
            "collection_exists": exact_final_collection is not None,
            "expected_objects": EXPECTED_FINAL_OBJECTS,
            "expected_present_by_exact_name": expected_present,
            "expected_missing_by_exact_name": expected_missing,
        },
        "classification_summary": {
            "armatures": [obj.name for obj in armatures],
            "shape_key_objects": sorted(obj.name for obj in mesh_objects if obj.data.shape_keys),
            "groom_candidates": sorted(obj.name for obj in groom_objects),
            "camera_candidates": sorted(obj.name for obj in bpy.data.objects if obj.type == "CAMERA"),
            "temporary_or_refinement_name_candidates": temp_candidates,
        },
        "objects": all_objects,
    }

    rig_report = {
        "schema": "sloth_gate0_rig_report_v1",
        "generated_utc": utc_now(),
        "source_filepath": source_filepath,
        "armature_count": len(armatures),
        "armatures": [armature_report(obj) for obj in armatures],
        "bound_meshes": bound_meshes,
        "actions": [action_summary(action) for action in bpy.data.actions],
        "all_drivers": drivers,
        "object_constraints": [{"object": obj.name, "constraints": [serialize_constraint(c) for c in obj.constraints]} for obj in bpy.data.objects if obj.constraints],
        "pose_constraints": [{
            "armature": obj.name,
            "bones": [{"bone": pb.name, "constraints": [serialize_constraint(c) for c in pb.constraints]} for pb in obj.pose.bones if pb.constraints],
        } for obj in armatures],
    }

    shapekey_report = {
        "schema": "sloth_gate0_shapekey_report_v1",
        "generated_utc": utc_now(),
        "source_filepath": source_filepath,
        "shape_key_datablocks": len(shape_meshes),
        "shape_key_blocks_total": key_count,
        "meshes": shape_meshes,
        "risk_notes": [
            "Shape-key order, vertex counts, relative-key relationships, coordinates, and drivers are fingerprinted before any future fit propagation.",
            "Any future Basis update must preserve every existing relative expression delta and must not reorder key blocks or bound vertices without approval.",
        ],
    }
    return scene_audit, object_map, rig_report, shapekey_report


def main():
    REPORTS_DIR.mkdir(parents=True, exist_ok=True)
    CHECKPOINTS_DIR.mkdir(parents=True, exist_ok=True)
    source_filepath = bpy.data.filepath
    if not source_filepath:
        raise RuntimeError("Current Blender file has never been saved; refusing Gate 0 checkpoint.")
    source_path = Path(source_filepath)
    source_size = source_path.stat().st_size if source_path.exists() else None
    context_before = {
        "filepath": bpy.data.filepath,
        "scene": bpy.context.scene.name if bpy.context.scene else None,
        "active_object": bpy.context.view_layer.objects.active.name if bpy.context.view_layer.objects.active else None,
        "selected": sorted(obj.name for obj in bpy.context.selected_objects),
        "object_count": len(bpy.data.objects),
        "mesh_fingerprints": {obj.name: mesh_hashes(obj.data)["topology_and_vertex_order_sha256"] for obj in bpy.data.objects if obj.type == "MESH"},
    }

    checkpoint_created_this_run = False
    if not CHECKPOINT_PATH.exists():
        result = bpy.ops.wm.save_as_mainfile(filepath=str(CHECKPOINT_PATH), copy=True, compress=True)
        if "FINISHED" not in result or not CHECKPOINT_PATH.exists():
            raise RuntimeError(f"Checkpoint copy failed: {result}")
        checkpoint_created_this_run = True
    checkpoint_size = CHECKPOINT_PATH.stat().st_size

    reports = gather_reports(source_filepath, source_size, checkpoint_size, checkpoint_created_this_run)
    for (name, path), payload in zip(REPORT_PATHS.items(), reports):
        write_json(path, payload)

    context_after = {
        "filepath": bpy.data.filepath,
        "scene": bpy.context.scene.name if bpy.context.scene else None,
        "active_object": bpy.context.view_layer.objects.active.name if bpy.context.view_layer.objects.active else None,
        "selected": sorted(obj.name for obj in bpy.context.selected_objects),
        "object_count": len(bpy.data.objects),
        "mesh_fingerprints": {obj.name: mesh_hashes(obj.data)["topology_and_vertex_order_sha256"] for obj in bpy.data.objects if obj.type == "MESH"},
    }
    if context_after != context_before:
        raise RuntimeError(f"Gate 0 audit changed Blender context or mesh fingerprints: before={context_before} after={context_after}")

    summary = {
        "status": "GATE_0_COMPLETE",
        "source_filepath_preserved": bpy.data.filepath == source_filepath,
        "source_filepath": source_filepath,
        "checkpoint": str(CHECKPOINT_PATH),
        "checkpoint_size_bytes": checkpoint_size,
        "checkpoint_created_this_run": checkpoint_created_this_run,
        "reports": {name: str(path) for name, path in REPORT_PATHS.items()},
        "scene_objects": len(bpy.data.objects),
        "armatures": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]),
        "shape_key_datablocks": len(bpy.data.shape_keys),
        "actions": len(bpy.data.actions),
    }
    print("GATE0_RESULT=" + json.dumps(summary, ensure_ascii=False, sort_keys=True))
    return summary


GATE0_RESULT = main()
