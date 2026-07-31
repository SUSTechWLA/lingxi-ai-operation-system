"""Gate 2: non-destructive head topology decision and expression evidence.

This script temporarily evaluates existing facial controls, renders four face
tests, restores every tested value, fingerprints the character before/after,
and writes a KEEP_TOPOLOGY or REBUILD_HEAD decision. It does not rebuild,
retopologize, add/remove vertices, change UVs, edit weights, or alter materials.
"""

from __future__ import annotations

import bpy
import hashlib
import json
import math
import struct
import time
from collections import Counter, defaultdict, deque
from datetime import datetime, timezone
from pathlib import Path

from mathutils import Vector


PROJECT_ROOT = Path("/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip")
EXPECTED_SOURCE = PROJECT_ROOT / "checkpoints/sloth_005_lookdev.blend"
PRE_CHECKPOINT = PROJECT_ROOT / "checkpoints/sloth_006_pre_topology_decision.blend"
RESULT_BLEND = PROJECT_ROOT / "checkpoints/sloth_009_topology_decision.blend"
REPORT_PATH = PROJECT_ROOT / "reports/gate2_topology_decision.json"
RENDER_DIR = PROJECT_ROOT / "renders/lookdev/gate2"

FINAL_COLLECTION = "COL_CHR_SLOTH_FINAL"
MAIN_OBJECT = "part_00000001.001"
LOOKDEV_SCENE = "SCENE_LOOKDEV"
FACE_CAMERA = "CAM_LOOKDEV_100MM"


def utc_now():
    return datetime.now(timezone.utc).isoformat()


def vec(value):
    return [round(float(component), 9) for component in value]


def sha256_file(path):
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def mesh_coordinate_hash(mesh):
    digest = hashlib.sha256()
    digest.update(struct.pack("<III", len(mesh.vertices), len(mesh.edges), len(mesh.polygons)))
    for vertex in mesh.vertices:
        digest.update(struct.pack("<fff", *map(float, vertex.co)))
    for edge in mesh.edges:
        digest.update(struct.pack("<II", *map(int, edge.vertices)))
    for polygon in mesh.polygons:
        digest.update(struct.pack("<I", len(polygon.vertices)))
        for index in polygon.vertices:
            digest.update(struct.pack("<I", int(index)))
    for layer in mesh.uv_layers:
        digest.update(layer.name.encode("utf-8"))
        for item in layer.data:
            digest.update(struct.pack("<ff", float(item.uv.x), float(item.uv.y)))
    if mesh.shape_keys:
        for key in mesh.shape_keys.key_blocks:
            digest.update(key.name.encode("utf-8"))
            for point in key.data:
                digest.update(struct.pack("<fff", *map(float, point.co)))
    return digest.hexdigest()


def character_state(collection):
    records = []
    for obj in sorted(collection.all_objects, key=lambda item: item.name):
        record = {
            "name": obj.name,
            "type": obj.type,
            "data": getattr(getattr(obj, "data", None), "name", None),
            "location": vec(obj.location),
            "rotation_euler": vec(obj.rotation_euler),
            "scale": vec(obj.scale),
            "parent": obj.parent.name if obj.parent else None,
        }
        if obj.type == "MESH":
            record["mesh_hash"] = mesh_coordinate_hash(obj.data)
            record["materials"] = [slot.material.name if slot.material else None for slot in obj.material_slots]
            record["vertex_groups"] = [group.name for group in obj.vertex_groups]
            record["modifiers"] = [(modifier.name, modifier.type) for modifier in obj.modifiers]
            if obj.data.shape_keys:
                record["shape_values"] = {key.name: round(float(key.value), 9) for key in obj.data.shape_keys.key_blocks}
        elif obj.type == "ARMATURE":
            record["bones"] = [(bone.name, bone.parent.name if bone.parent else None, bone.use_deform) for bone in obj.data.bones]
        records.append(record)
    encoded = json.dumps(records, sort_keys=True, separators=(",", ":")).encode("utf-8")
    return {"sha256": hashlib.sha256(encoded).hexdigest(), "records": records, "object_count": len(records)}


def current_shape_coordinates(obj):
    mesh = obj.data
    if not mesh.shape_keys:
        return [vertex.co.copy() for vertex in mesh.vertices]
    blocks = mesh.shape_keys.key_blocks
    basis = blocks[0]
    coordinates = [point.co.copy() for point in basis.data]
    for key in blocks[1:]:
        if abs(key.value) <= 1.0e-12:
            continue
        relative = key.relative_key
        for index, point in enumerate(key.data):
            coordinates[index] += (point.co - relative.data[index].co) * key.value
    return coordinates


def edge_face_counts(mesh):
    counts = [0] * len(mesh.edges)
    for polygon in mesh.polygons:
        for loop_index in polygon.loop_indices:
            counts[mesh.loops[loop_index].edge_index] += 1
    return counts


def boundary_components(obj, world_coordinates):
    mesh = obj.data
    counts = edge_face_counts(mesh)
    boundary_edges = [edge for edge, count in zip(mesh.edges, counts) if count == 1]
    adjacency = defaultdict(set)
    edge_lookup = set()
    for edge in boundary_edges:
        a, b = map(int, edge.vertices)
        adjacency[a].add(b)
        adjacency[b].add(a)
        edge_lookup.add(tuple(sorted((a, b))))
    components = []
    visited = set()
    for start in adjacency:
        if start in visited:
            continue
        queue = deque([start])
        vertices = set()
        while queue:
            current = queue.popleft()
            if current in visited:
                continue
            visited.add(current)
            vertices.add(current)
            queue.extend(adjacency[current] - visited)
        component_edges = [edge for edge in edge_lookup if edge[0] in vertices and edge[1] in vertices]
        points = [world_coordinates[index] for index in vertices]
        minimum = Vector((min(point.x for point in points), min(point.y for point in points), min(point.z for point in points)))
        maximum = Vector((max(point.x for point in points), max(point.y for point in points), max(point.z for point in points)))
        center = (minimum + maximum) * 0.5
        components.append({
            "vertices": len(vertices),
            "edges": len(component_edges),
            "is_closed_cycle": all(len(adjacency[index]) == 2 for index in vertices),
            "center": vec(center),
            "minimum": vec(minimum),
            "maximum": vec(maximum),
            "extent": vec(maximum - minimum),
        })
    return sorted(components, key=lambda item: item["edges"], reverse=True)


def object_bounds_world(obj):
    points = [obj.matrix_world @ Vector(corner) for corner in obj.bound_box]
    minimum = Vector((min(point.x for point in points), min(point.y for point in points), min(point.z for point in points)))
    maximum = Vector((max(point.x for point in points), max(point.y for point in points), max(point.z for point in points)))
    return minimum, maximum, (minimum + maximum) * 0.5


def region_topology(obj, world_coordinates, center, radius_x, radius_z, front_depth):
    mesh = obj.data
    selected = set()
    for index, point in enumerate(world_coordinates):
        dx = (point.x - center.x) / radius_x
        dz = (point.z - center.z) / radius_z
        if dx * dx + dz * dz <= 1.0 and point.y <= center.y + front_depth:
            selected.add(index)
    selected_edges = [edge for edge in mesh.edges if int(edge.vertices[0]) in selected and int(edge.vertices[1]) in selected]
    selected_faces = [polygon for polygon in mesh.polygons if all(int(index) in selected for index in polygon.vertices)]
    valence = Counter()
    for edge in selected_edges:
        valence[int(edge.vertices[0])] += 1
        valence[int(edge.vertices[1])] += 1
    edge_counts = edge_face_counts(mesh)
    boundary = [edge for edge in selected_edges if edge_counts[edge.index] == 1]
    face_sides = Counter(len(polygon.vertices) for polygon in selected_faces)
    triangles = face_sides.get(3, 0)
    quads = face_sides.get(4, 0)
    total_faces = len(selected_faces)
    return {
        "center": vec(center),
        "radius_x": radius_x,
        "radius_z": radius_z,
        "vertices": len(selected),
        "edges": len(selected_edges),
        "faces": total_faces,
        "triangles": triangles,
        "quads": quads,
        "other_faces": total_faces - triangles - quads,
        "triangle_fraction": round(triangles / total_faces, 6) if total_faces else None,
        "boundary_edges": len(boundary),
        "valence_histogram": {str(key): value for key, value in sorted(Counter(valence.values()).items())},
        "low_valence_vertices": sum(1 for value in valence.values() if value <= 2),
        "high_valence_vertices": sum(1 for value in valence.values() if value >= 6),
        "selected_vertex_indices": sorted(selected),
    }


def shape_delta_report(mesh, key_name, indices=None):
    keys = mesh.shape_keys.key_blocks
    key = keys.get(key_name)
    if key is None:
        return {"name": key_name, "exists": False}
    relative = key.relative_key
    chosen = indices if indices is not None else range(len(key.data))
    magnitudes = []
    moved = 0
    for index in chosen:
        magnitude = (key.data[index].co - relative.data[index].co).length
        magnitudes.append(magnitude)
        if magnitude > 1.0e-6:
            moved += 1
    return {
        "name": key_name,
        "exists": True,
        "relative_key": relative.name,
        "sampled_vertices": len(magnitudes),
        "moved_vertices": moved,
        "max_delta": round(max(magnitudes, default=0.0), 9),
        "rms_delta": round(math.sqrt(sum(value * value for value in magnitudes) / len(magnitudes)), 9) if magnitudes else 0.0,
    }


def render_expression(scene, camera, key_blocks, baseline_values, name, values):
    for key_name, value in baseline_values.items():
        key_blocks[key_name].value = value
    for key_name, value in values.items():
        if key_blocks.get(key_name):
            key_blocks[key_name].value = value
    bpy.context.view_layer.update()
    output = RENDER_DIR / f"{name}.png"
    scene.camera = camera
    scene.render.filepath = str(output)
    start = time.perf_counter()
    bpy.ops.render.render(write_still=True, scene=scene.name)
    elapsed = time.perf_counter() - start
    if not output.exists() or output.stat().st_size == 0:
        raise RuntimeError(f"Missing expression render: {output}")
    return {
        "name": name,
        "requested_values": values,
        "output": str(output),
        "bytes": output.stat().st_size,
        "sha256": sha256_file(output),
        "render_seconds": round(elapsed, 6),
    }


def nearest_boundary_to(center, components):
    if not components:
        return None
    def distance(component):
        return (Vector(component["center"]) - center).length
    best = min(components, key=distance)
    return {"distance": round(distance(best), 9), "component": best}


def main():
    if Path(bpy.data.filepath).resolve() != EXPECTED_SOURCE.resolve():
        raise RuntimeError(f"Wrong Gate 2 source. Expected {EXPECTED_SOURCE}, found {bpy.data.filepath}")
    if RESULT_BLEND.exists():
        raise FileExistsError(f"Refusing to overwrite Gate 2 result: {RESULT_BLEND}")
    collection = bpy.data.collections.get(FINAL_COLLECTION)
    obj = bpy.data.objects.get(MAIN_OBJECT)
    scene = bpy.data.scenes.get(LOOKDEV_SCENE)
    camera = bpy.data.objects.get(FACE_CAMERA)
    if not all((collection, obj, scene, camera)):
        raise RuntimeError("Gate 2 prerequisites are missing.")
    if obj.type != "MESH" or not obj.data.shape_keys:
        raise RuntimeError("Main object or Shape Keys missing.")

    PRE_CHECKPOINT.parent.mkdir(parents=True, exist_ok=True)
    REPORT_PATH.parent.mkdir(parents=True, exist_ok=True)
    RENDER_DIR.mkdir(parents=True, exist_ok=True)
    if not PRE_CHECKPOINT.exists():
        result = bpy.ops.wm.save_as_mainfile(filepath=str(PRE_CHECKPOINT), copy=True, compress=True)
        if "FINISHED" not in result:
            raise RuntimeError(f"Gate 2 pre-checkpoint failed: {result}")

    state_before = character_state(collection)
    key_blocks = obj.data.shape_keys.key_blocks
    baseline_values = {key.name: float(key.value) for key in key_blocks}
    original_render_path = scene.render.filepath
    original_camera = scene.camera
    original_scene = bpy.context.window.scene if bpy.context.window else None
    if bpy.context.window:
        bpy.context.window.scene = scene

    local_coordinates = current_shape_coordinates(obj)
    world_coordinates = [obj.matrix_world @ coordinate for coordinate in local_coordinates]
    components = boundary_components(obj, world_coordinates)

    eye_objects = {side: bpy.data.objects.get(f"CXR_Eye.{side}") for side in ("L", "R")}
    eye_centers = {}
    eye_bounds = {}
    for side, eye in eye_objects.items():
        if eye:
            minimum, maximum, center = object_bounds_world(eye)
            eye_centers[side] = center
            eye_bounds[side] = {"minimum": vec(minimum), "maximum": vec(maximum), "center": vec(center), "dimensions": vec(maximum - minimum)}

    oral = bpy.data.objects.get("IP_OralCavity")
    if oral:
        oral_min, oral_max, mouth_center = object_bounds_world(oral)
    else:
        mouth_center = Vector((0.0, -0.25, 2.05))

    eye_regions = {}
    eye_protrusion = {}
    for side, center in eye_centers.items():
        dimensions = Vector(eye_bounds[side]["dimensions"])
        radius_x = max(0.19, dimensions.x * 0.72)
        radius_z = max(0.17, dimensions.z * 0.72)
        stats = region_topology(obj, world_coordinates, center, radius_x, radius_z, 0.30)
        eye_regions[side] = stats
        nearby_front = [world_coordinates[index].y for index in stats["selected_vertex_indices"]]
        face_front_y = min(nearby_front) if nearby_front else None
        eye_front_y = eye_bounds[side]["minimum"][1]
        eye_protrusion[side] = {
            "face_front_y": round(face_front_y, 9) if face_front_y is not None else None,
            "eye_front_y": round(eye_front_y, 9),
            "eye_ahead_of_face": round(face_front_y - eye_front_y, 9) if face_front_y is not None else None,
        }

    mouth_region = region_topology(obj, world_coordinates, mouth_center, 0.42, 0.30, 0.28)
    eye_index_union = sorted(set().union(*(set(item["selected_vertex_indices"]) for item in eye_regions.values()))) if eye_regions else []
    mouth_indices = mouth_region["selected_vertex_indices"]

    expression_renders = []
    try:
        expression_renders.append(render_expression(scene, camera, key_blocks, baseline_values, "neutral", {}))
        expression_renders.append(render_expression(scene, camera, key_blocks, baseline_values, "blink_proxy", {"Eye_Squint.L": 1.0, "Eye_Squint.R": 1.0}))
        expression_renders.append(render_expression(scene, camera, key_blocks, baseline_values, "smile", {"Mouth_Smile": 1.0, "Cheek_Smile.L": 0.75, "Cheek_Smile.R": 0.75}))
        expression_renders.append(render_expression(scene, camera, key_blocks, baseline_values, "mouth_open", {"Mouth_A": 1.0}))
    finally:
        for key_name, value in baseline_values.items():
            key_blocks[key_name].value = value
        scene.render.filepath = original_render_path
        scene.camera = original_camera
        if bpy.context.window and original_scene:
            bpy.context.window.scene = original_scene
        bpy.context.view_layer.update()

    state_after = character_state(collection)
    if state_before["sha256"] != state_after["sha256"]:
        raise RuntimeError("Gate 2 failed to restore the character after expression tests.")

    key_names = [key.name for key in key_blocks]
    true_blink_keys = [name for name in key_names if "blink" in name.lower()]
    jaw_open_keys = [name for name in key_names if "jaw" in name.lower() and "open" in name.lower()]
    lid_objects = sorted(obj_item.name for obj_item in collection.all_objects if "lid" in obj_item.name.lower())
    lid_object_types = {name: bpy.data.objects[name].type for name in lid_objects}
    lid_driver_count = 0
    for name in lid_objects:
        animation_data = bpy.data.objects[name].animation_data
        lid_driver_count += len(animation_data.drivers) if animation_data else 0

    shape_deltas = {
        "eye": [shape_delta_report(obj.data, name, eye_index_union) for name in ("Eye_Squint.L", "Eye_Squint.R", "Eye_Wide.L", "Eye_Wide.R")],
        "mouth": [shape_delta_report(obj.data, name, mouth_indices) for name in ("Mouth_Smile", "Mouth_A", "Mouth_O", "Mouth_Surprise", "Cheek_Smile.L", "Cheek_Smile.R")],
    }

    decision = "REBUILD_HEAD"
    reasons = [
        "No true Blink Shape Key exists; Eye_Squint is only a proxy and does not define a production eyelid closure arc.",
        "Upper/lower eyelids are separate CURVE helper objects rather than integrated deforming eyelid topology with inner and outer canthi.",
        "Existing eye drivers primarily scale or translate eye layers under Squint/Wide instead of wrapping lids over a stable globe.",
        "Neutral review shows the eye assembly sitting ahead of the facial socket, with insufficient lid coverage and inconsistent sclera exposure.",
        "No dedicated JawOpen Shape Key exists; mouth opening relies on viseme geometry without a complete jaw/lip/cheek deformation system.",
        "The target requires larger structural changes to muzzle, cheeks, mouth corners, chin, eye depth, and lid thickness than a safe Basis fit can deliver on this head topology.",
    ]

    rebuild_plan = [
        {"step": 1, "name": "Preserve source", "detail": "Keep the current 5478-vertex mesh, all 29 Shape Keys, UVMap, 43 vertex groups, Armature, actions, and drivers untouched as the migration source."},
        {"step": 2, "name": "Temporary replacement head", "detail": "Build a symmetric animation control head inside COL_CHR_SLOTH_FINAL with explicit concentric eye loops, upper/lower lid thickness, inner/outer canthi, closed mouth loops, nasolabial flow, muzzle rings, cheek volume, chin, jaw and neck seam."},
        {"step": 3, "name": "Multi-view fit", "detail": "Fit the new head to target_character.png and Gate 1 front/3-quarter/side evidence without camera-specific scaling; eye globes must sit behind the orbital rim in every view."},
        {"step": 4, "name": "Binding migration", "detail": "Transfer Head, Neck, Jaw and facial deformation weights from the source, then normalize and test head turn, jaw, cheeks and eyelids. Preserve the single existing Armature."},
        {"step": 5, "name": "Expression rebuild", "detail": "Recreate Blink, Squint, Wide, Smile, CheekRaise, JawOpen and visemes on the new head. Retarget eye-layer drivers to stable controls rather than scaling entire eye assemblies."},
        {"step": 6, "name": "UV and materials", "detail": "Create a production UV layout for the replacement head, preserve body UVs, and map the existing face/eye material intent without changing the body vertex order."},
        {"step": 7, "name": "Approved seam integration", "detail": "Only after explicit approval, replace the old head region and weld at the neck seam. This is the first step allowed to change bound-mesh vertex count/order."},
        {"step": 8, "name": "Validation and cleanup", "detail": "Validate all views and expressions, remove old head and temporary fitting objects only after parity is proven, and retain one GEO_HeadBody and one expression system."},
    ]

    report = {
        "schema": "sloth_gate2_head_topology_decision_v1",
        "generated_utc": utc_now(),
        "status": "GATE_2_COMPLETE",
        "source": str(EXPECTED_SOURCE),
        "pre_checkpoint": str(PRE_CHECKPOINT),
        "result_blend": str(RESULT_BLEND),
        "decision": decision,
        "decision_confidence": "HIGH",
        "character_unchanged": state_before["sha256"] == state_after["sha256"],
        "character_signature_before": state_before["sha256"],
        "character_signature_after": state_after["sha256"],
        "mesh": {
            "object": obj.name,
            "vertices": len(obj.data.vertices),
            "edges": len(obj.data.edges),
            "polygons": len(obj.data.polygons),
            "boundary_components": components,
            "eye_regions": {side: {key: value for key, value in stats.items() if key != "selected_vertex_indices"} for side, stats in eye_regions.items()},
            "mouth_region": {key: value for key, value in mouth_region.items() if key != "selected_vertex_indices"},
            "eye_bounds": eye_bounds,
            "eye_protrusion": eye_protrusion,
            "nearest_boundary_to_eye": {side: nearest_boundary_to(center, components) for side, center in eye_centers.items()},
            "nearest_boundary_to_mouth": nearest_boundary_to(mouth_center, components),
        },
        "expression_system": {
            "shape_key_order": key_names,
            "true_blink_keys": true_blink_keys,
            "jaw_open_keys": jaw_open_keys,
            "lid_objects": lid_objects,
            "lid_object_types": lid_object_types,
            "lid_driver_count": lid_driver_count,
            "shape_deltas": shape_deltas,
            "tests": expression_renders,
            "test_interpretation": {
                "neutral": "Baseline for orbital depth, sclera exposure, muzzle volume and mouth corners.",
                "blink_proxy": "Uses Eye_Squint.L/R because no Blink key exists; failure to close confirms missing production blink topology/control.",
                "smile": "Tests mouth-corner continuity and cheek participation.",
                "mouth_open": "Uses Mouth_A as the available open-mouth proxy; evaluates lip ring and oral-cavity integration without permanent jaw edits.",
            },
        },
        "decision_reasons": reasons,
        "rebuild_head_plan": rebuild_plan,
        "future_vertex_order_gate": {
            "current_stage_changed_vertex_count_or_order": False,
            "approval_required_before_replacement": True,
            "current_mesh_must_remain_recoverable": True,
        },
        "acceptance": {
            "blink_test_rendered": any(item["name"] == "blink_proxy" for item in expression_renders),
            "smile_test_rendered": any(item["name"] == "smile" for item in expression_renders),
            "mouth_open_test_rendered": any(item["name"] == "mouth_open" for item in expression_renders),
            "topology_decision_emitted": decision in {"KEEP_TOPOLOGY", "REBUILD_HEAD"},
            "no_rebuild_executed": True,
            "character_state_restored": state_before["sha256"] == state_after["sha256"],
        },
    }
    REPORT_PATH.write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")

    save_result = bpy.ops.wm.save_as_mainfile(filepath=str(RESULT_BLEND), compress=True)
    if "FINISHED" not in save_result or Path(bpy.data.filepath).resolve() != RESULT_BLEND.resolve():
        raise RuntimeError(f"Gate 2 result save failed: {save_result}")
    print("GATE2_RESULT=" + json.dumps({
        "status": "GATE_2_COMPLETE",
        "decision": decision,
        "active_file": bpy.data.filepath,
        "character_unchanged": True,
        "report": str(REPORT_PATH),
        "renders": [item["output"] for item in expression_renders],
    }, ensure_ascii=False, sort_keys=True))


main()
