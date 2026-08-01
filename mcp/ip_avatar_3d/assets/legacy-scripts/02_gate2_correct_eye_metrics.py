"""Correct Gate 2 eye-region metrics using visible iris centers.

The hidden CXR_Eye/Cornea placeholders have identity world matrices and are not
valid spatial references. This script only updates the external JSON report; it
does not modify or save any Blender datablock.
"""

from __future__ import annotations

import bpy
import json
from collections import Counter
from pathlib import Path

from mathutils import Vector


PROJECT_ROOT = Path("/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip")
EXPECTED_BLEND = PROJECT_ROOT / "checkpoints/sloth_009_topology_decision.blend"
REPORT_PATH = PROJECT_ROOT / "reports/gate2_topology_decision.json"
MAIN_OBJECT = "part_00000001.001"


def vec(value):
    return [round(float(component), 9) for component in value]


def current_shape_coordinates(obj):
    mesh = obj.data
    blocks = mesh.shape_keys.key_blocks
    coordinates = [point.co.copy() for point in blocks[0].data]
    for key in blocks[1:]:
        if abs(key.value) <= 1.0e-12:
            continue
        for index, point in enumerate(key.data):
            coordinates[index] += (point.co - key.relative_key.data[index].co) * key.value
    return coordinates


def edge_face_counts(mesh):
    counts = [0] * len(mesh.edges)
    for polygon in mesh.polygons:
        for loop_index in polygon.loop_indices:
            counts[mesh.loops[loop_index].edge_index] += 1
    return counts


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
    counts = edge_face_counts(mesh)
    face_sides = Counter(len(polygon.vertices) for polygon in selected_faces)
    triangles = face_sides.get(3, 0)
    quads = face_sides.get(4, 0)
    return {
        "reference_center": vec(center),
        "radius_x": radius_x,
        "radius_z": radius_z,
        "vertices": len(selected),
        "edges": len(selected_edges),
        "faces": len(selected_faces),
        "triangles": triangles,
        "quads": quads,
        "other_faces": len(selected_faces) - triangles - quads,
        "triangle_fraction": round(triangles / len(selected_faces), 6) if selected_faces else None,
        "boundary_edges": sum(1 for edge in selected_edges if counts[edge.index] == 1),
        "valence_histogram": {str(key): value for key, value in sorted(Counter(valence.values()).items())},
        "low_valence_vertices": sum(1 for value in valence.values() if value <= 2),
        "high_valence_vertices": sum(1 for value in valence.values() if value >= 6),
        "selected_vertex_indices": sorted(selected),
    }


def shape_delta(mesh, key_name, indices):
    key = mesh.shape_keys.key_blocks.get(key_name)
    if key is None:
        return {"name": key_name, "exists": False}
    magnitudes = [(key.data[index].co - key.relative_key.data[index].co).length for index in indices]
    moved = sum(1 for value in magnitudes if value > 1.0e-6)
    return {
        "name": key_name,
        "exists": True,
        "relative_key": key.relative_key.name,
        "sampled_vertices": len(magnitudes),
        "moved_vertices": moved,
        "max_delta": round(max(magnitudes, default=0.0), 9),
    }


def nearest_boundary(center, components):
    if not components:
        return None
    best = min(components, key=lambda component: (Vector(component["center"]) - center).length)
    return {"distance": round((Vector(best["center"]) - center).length, 9), "component": best}


if Path(bpy.data.filepath).resolve() != EXPECTED_BLEND.resolve():
    raise RuntimeError(f"Wrong active Gate 2 file: {bpy.data.filepath}")
obj = bpy.data.objects.get(MAIN_OBJECT)
if obj is None or obj.type != "MESH":
    raise RuntimeError("Main mesh unavailable.")
report = json.loads(REPORT_PATH.read_text(encoding="utf-8"))
coordinates = [obj.matrix_world @ coordinate for coordinate in current_shape_coordinates(obj)]

iris_objects = {side: bpy.data.objects.get(f"CXR_Iris.{side}") for side in ("L", "R")}
if not all(iris_objects.values()):
    raise RuntimeError("Visible iris objects missing.")

corrected_regions = {}
corrected_deltas = []
for side, iris in iris_objects.items():
    center = iris.matrix_world.translation.copy()
    stats = region_topology(obj, coordinates, center, 0.24, 0.21, 0.24)
    corrected_regions[side] = {key: value for key, value in stats.items() if key != "selected_vertex_indices"}
    for name in (f"Eye_Squint.{side}", f"Eye_Wide.{side}"):
        corrected_deltas.append(shape_delta(obj.data, name, stats["selected_vertex_indices"]))

eye_layer_names = [name for name in bpy.data.objects.keys() if any(token in name for token in ("CXR_Eye.", "CXR_Cornea.", "CXR_Iris.", "CXR_Pupil.", "CXR_Lid", "CXR_Tearline."))]
layer_inventory = [{
    "name": name,
    "type": bpy.data.objects[name].type,
    "hide_render": bpy.data.objects[name].hide_render,
    "parent_bone": bpy.data.objects[name].parent_bone,
    "world_origin": vec(bpy.data.objects[name].matrix_world.translation),
} for name in sorted(eye_layer_names)]

report["mesh"]["eye_reference_source"] = "Visible CXR_Iris.L/R world origins; hidden CXR_Eye/Cornea placeholders are excluded."
report["mesh"]["eye_regions"] = corrected_regions
report["mesh"]["eye_protrusion"] = {
    "measurement_valid": False,
    "reason": "The current rendered sclera/globe is not represented by a visible independent CXR_Eye or CXR_Cornea surface. Numeric globe-to-orbit protrusion cannot be measured reliably from the hidden identity-transform placeholders; Gate 1/2 renders remain the valid visual evidence.",
}
report["mesh"]["nearest_boundary_to_eye"] = {
    side: nearest_boundary(iris.matrix_world.translation, report["mesh"]["boundary_components"])
    for side, iris in iris_objects.items()
}
report["expression_system"]["shape_deltas"]["eye"] = corrected_deltas
report["expression_system"]["eye_layer_inventory"] = layer_inventory
report["expression_system"]["visible_eye_layer_issues"] = [
    "CXR_Eye.L/R and CXR_Cornea.L/R are hidden from render.",
    "CXR_LidLower.L/R and CXR_Tearline.L/R are hidden from render.",
    "Visible iris/pupil layers and upper-lid curves do not constitute a complete sclera/globe/cornea/tearline/lid system.",
]
report["metrics_correction"] = {
    "applied": True,
    "reason": "Replaced invalid hidden-placeholder eye centers with visible iris centers and explicitly invalidated numeric protrusion measurement.",
    "character_datablocks_modified": False,
}
REPORT_PATH.write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
print("GATE2_EYE_METRICS_CORRECTED")
