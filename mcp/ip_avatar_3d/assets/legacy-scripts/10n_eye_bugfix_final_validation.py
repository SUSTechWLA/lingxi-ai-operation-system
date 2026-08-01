import bpy
import json
import math
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
FINAL_BLEND = os.path.join(ROOT, "character_sloth_final.blend")
REPORT = os.path.join(ROOT, "reports", "eye_bugfix_final_validation.json")
FINAL_RENDER_DIR = os.path.join(ROOT, "renders", "final")
V9_RENDER_DIR = os.path.join(ROOT, "renders", "eye_bugfix", "candidate_v9")
os.makedirs(os.path.dirname(REPORT), exist_ok=True)

head = bpy.data.objects.get("GEO_HeadBody")
if not head or not head.data.shape_keys:
    raise RuntimeError("Formal head expression system missing")


def world_bounds(obj):
    points = [obj.matrix_world @ Vector(corner) for corner in obj.bound_box]
    return {
        "min": [min(p[i] for p in points) for i in range(3)],
        "max": [max(p[i] for p in points) for i in range(3)],
    }


lid_objects = [
    bpy.data.objects.get(f"EYE_Lid{part}_{side}")
    for side in ("L", "R") for part in ("Upper", "Lower")
]
all_lids_present = all(lid_objects)
lid_driver_records = []
for obj in lid_objects:
    if obj and obj.data.shape_keys and obj.data.shape_keys.animation_data:
        for fc in obj.data.shape_keys.animation_data.drivers:
            lid_driver_records.append({
                "owner": obj.name,
                "path": fc.data_path,
                "valid": bool(fc.driver.is_valid),
            })

seam_distances = {}
for side in ("L", "R"):
    upper = bpy.data.objects.get(f"EYE_LidUpper_{side}")
    lower = bpy.data.objects.get(f"EYE_LidLower_{side}")
    distances = []
    if upper and lower:
        upper_blink = upper.data.shape_keys.key_blocks.get("Blink")
        lower_blink = lower.data.shape_keys.key_blocks.get("Blink")
        for col in range(48):
            index = col * 6 + 5
            upper_world = upper.matrix_world @ upper_blink.data[index].co
            lower_world = lower.matrix_world @ lower_blink.data[index].co
            distances.append((upper_world - lower_world).length)
    seam_distances[side] = max(distances) if distances else None

eye_records = {}
for side in ("L", "R"):
    for role in ("Sclera", "Cornea", "Iris", "Pupil", "Catchlight"):
        obj = bpy.data.objects.get(f"EYE_{role}_{side}")
        if obj:
            eye_records[obj.name] = {
                "dimensions": [round(v, 6) for v in obj.dimensions],
                "bounds": {
                    "min": [round(v, 6) for v in world_bounds(obj)["min"]],
                    "max": [round(v, 6) for v in world_bounds(obj)["max"]],
                },
                "parent": obj.parent.name if obj.parent else None,
                "parent_bone": obj.parent_bone,
                "materials": [slot.material.name if slot.material else None for slot in obj.material_slots],
            }

layer_order_checks = {}
size_checks = {}
for side in ("L", "R"):
    sclera = bpy.data.objects[f"EYE_Sclera_{side}"]
    cornea = bpy.data.objects[f"EYE_Cornea_{side}"]
    iris = bpy.data.objects[f"EYE_Iris_{side}"]
    pupil = bpy.data.objects[f"EYE_Pupil_{side}"]
    upper = bpy.data.objects[f"EYE_LidUpper_{side}"]
    lower = bpy.data.objects[f"EYE_LidLower_{side}"]
    cornea_front = world_bounds(cornea)["min"][1]
    pupil_front = world_bounds(pupil)["min"][1]
    lid_front = min(world_bounds(upper)["min"][1], world_bounds(lower)["min"][1])
    layer_order_checks[side] = {
        "cornea_in_front_of_pupil": cornea_front < pupil_front,
        "lid_in_front_of_cornea": lid_front < cornea_front,
        "values_y": {
            "lid_front": round(lid_front, 6),
            "cornea_front": round(cornea_front, 6),
            "pupil_front": round(pupil_front, 6),
        },
    }
    size_checks[side] = {
        "iris_within_sclera": iris.dimensions.x < sclera.dimensions.x and iris.dimensions.z < sclera.dimensions.z,
        "pupil_within_iris": pupil.dimensions.x < iris.dimensions.x and pupil.dimensions.z < iris.dimensions.z,
        "cornea_close_to_sclera": cornea.dimensions.x <= sclera.dimensions.x * 1.04 and cornea.dimensions.z <= sclera.dimensions.z * 1.04,
    }

cornea_mat = bpy.data.materials.get("MAT_Eye_Cornea")
cornea_node_types = {node.type for node in cornea_mat.node_tree.nodes} if cornea_mat and cornea_mat.use_nodes else set()
same_lid_material = all(
    bpy.data.objects[f"EYE_LidUpper_{side}"].material_slots[0].material
    is bpy.data.objects[f"EYE_LidLower_{side}"].material_slots[0].material
    for side in ("L", "R")
)

evidence_files = {
    "final_face_closeup": os.path.join(FINAL_RENDER_DIR, "face_closeup.png"),
    "final_front_34": os.path.join(FINAL_RENDER_DIR, "front_34.png"),
    "final_side": os.path.join(FINAL_RENDER_DIR, "side.png"),
    "blink_front": os.path.join(V9_RENDER_DIR, "blink_front.png"),
    "cycles_candidate_closeup": os.path.join(V9_RENDER_DIR, "cycles_face_closeup.png"),
}

checks = {
    "final_blend_is_open": os.path.abspath(bpy.data.filepath) == os.path.abspath(FINAL_BLEND),
    "head_vertex_count_preserved": len(head.data.vertices) == 5478,
    "head_shape_key_count_preserved": len(head.data.shape_keys.key_blocks) == 31,
    "all_four_lids_present": all_lids_present,
    "lid_topology_refined": all(obj and len(obj.data.vertices) == 288 for obj in lid_objects),
    "lid_shape_keys_complete": all(obj and [kb.name for kb in obj.data.shape_keys.key_blocks] == ["Basis", "Blink", "Wide"] for obj in lid_objects),
    "eight_valid_lid_drivers": len(lid_driver_records) == 8 and all(record["valid"] for record in lid_driver_records),
    "closed_lid_same_material": same_lid_material,
    "blink_seam_watertight": all(value is not None and value < 1.0e-5 for value in seam_distances.values()),
    "cornea_shader_transparent_reflective": {"BSDF_TRANSPARENT", "MIX_SHADER", "BSDF_PRINCIPLED"}.issubset(cornea_node_types),
    "cornea_backface_disabled": cornea_mat is not None and not bool(getattr(cornea_mat, "show_transparent_back", True)),
    "eye_layer_order_valid": all(all(item[k] for k in ("cornea_in_front_of_pupil", "lid_in_front_of_cornea")) for item in layer_order_checks.values()),
    "eye_layer_sizes_valid": all(all(item.values()) for item in size_checks.values()),
    "eye_bone_parenting_valid": all(record["parent"] == "RIG_Sloth" and record["parent_bone"] in {"Eye.L", "Eye.R"} for name, record in eye_records.items() if any(role in name for role in ("Sclera", "Cornea", "Iris", "Pupil", "Catchlight"))),
    "no_temporary_objects": not any(obj.name.startswith("TMP_") for obj in bpy.data.objects),
    "visual_evidence_present": all(os.path.exists(path) and os.path.getsize(path) > 100000 for path in evidence_files.values()),
}

failed = [name for name, value in checks.items() if not value]
report = {
    "schema": "sloth_eye_bugfix_final_validation_v1",
    "status": "PASS" if not failed else "FAIL",
    "final_blend": FINAL_BLEND,
    "summary": {"passed": len(checks) - len(failed), "total": len(checks), "failed": failed},
    "checks": checks,
    "lid_driver_records": lid_driver_records,
    "blink_seam_max_distance": seam_distances,
    "layer_order": layer_order_checks,
    "size_checks": size_checks,
    "eye_records": eye_records,
    "evidence_files": evidence_files,
}
with open(REPORT, "w", encoding="utf-8") as f:
    json.dump(report, f, ensure_ascii=False, indent=2)
print(json.dumps({
    "status": report["status"],
    "report": REPORT,
    "summary": report["summary"],
    "blink_seam_max_distance": seam_distances,
    "layer_order": layer_order_checks,
    "failed": failed,
}, ensure_ascii=False))
if failed:
    raise RuntimeError("Eye bugfix final validation failed: " + ", ".join(failed))
