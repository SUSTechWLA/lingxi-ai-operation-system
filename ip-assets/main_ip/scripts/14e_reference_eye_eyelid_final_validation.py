import bpy
import json
import os


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
FINAL = os.path.join(ROOT, "character_sloth_final.blend")
REPORT = os.path.join(ROOT, "reports", "reference_eye_eyelid_final_validation.json")
RENDER_DIR = os.path.join(ROOT, "renders", "final")


def action_fcurves(action):
    curves = []
    for layer in getattr(action, "layers", []):
        for strip in getattr(layer, "strips", []):
            for channelbag in getattr(strip, "channelbags", []):
                curves.extend(list(getattr(channelbag, "fcurves", [])))
    return curves


def main():
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    head = bpy.data.objects.get("GEO_HeadBody")
    rig = bpy.data.objects.get("RIG_Sloth")
    formal = bpy.data.collections.get("COL_CHR_SLOTH_FINAL")
    lid_material = bpy.data.materials.get("MAT_Eyelid_Skin_Brown")
    socket_material = bpy.data.materials.get("MAT_EyeSocket")
    if not head or not rig or not formal or not lid_material or not socket_material:
        raise RuntimeError("Formal reference eye system incomplete")

    lids = [bpy.data.objects[f"EYE_Lid{part}_{side}"] for side in ("L", "R") for part in ("Upper", "Lower")]
    ramp = next((node for node in lid_material.node_tree.nodes if node.type == "VALTORGB"), None)
    bump = next((node for node in lid_material.node_tree.nodes if node.type == "BUMP"), None)
    lid_shader = next((node for node in lid_material.node_tree.nodes if node.type == "BSDF_PRINCIPLED"), None)
    socket_shader = next((node for node in socket_material.node_tree.nodes if node.type == "BSDF_PRINCIPLED"), None)
    active_action = rig.animation_data.action if rig.animation_data else None
    eye_curves = [
        curve for curve in action_fcurves(active_action)
        if 'pose.bones["Eye.L"]' in curve.data_path or 'pose.bones["Eye.R"]' in curve.data_path
    ]
    seam_max = {}
    for side in ("L", "R"):
        upper = bpy.data.objects[f"EYE_LidUpper_{side}"]
        lower = bpy.data.objects[f"EYE_LidLower_{side}"]
        seam_max[side] = max(
            (
                upper.matrix_world @ upper.data.shape_keys.key_blocks["Blink"].data[col * 6 + 5].co
                - lower.matrix_world @ lower.data.shape_keys.key_blocks["Blink"].data[col * 6 + 5].co
            ).length
            for col in range(48)
        )
    driver_count = sum(
        len(obj.data.shape_keys.animation_data.drivers)
        for obj in lids if obj.data.shape_keys and obj.data.shape_keys.animation_data
    )
    eye_metrics = {
        side: {
            "iris": [round(value, 4) for value in bpy.data.objects[f"EYE_Iris_{side}"].dimensions],
            "limbal": [round(value, 4) for value in bpy.data.objects[f"EYE_IrisLimbal_{side}"].dimensions],
            "pupil": [round(value, 4) for value in bpy.data.objects[f"EYE_Pupil_{side}"].dimensions],
            "sclera": [round(value, 4) for value in bpy.data.objects[f"EYE_Sclera_{side}"].dimensions],
        }
        for side in ("L", "R")
    }
    render_names = ("front", "front_34", "side", "face_closeup", "blink")
    render_pairs = {
        name: {extension: os.path.join(RENDER_DIR, f"{name}.{extension}") for extension in ("png", "exr")}
        for name in render_names
    }
    formal_names = {obj.name for obj in formal.all_objects}
    checks = {
        "final_file_open": os.path.abspath(bpy.data.filepath) == os.path.abspath(FINAL),
        "head_vertex_count": len(head.data.vertices) == 5478,
        "shape_key_count": len(head.data.shape_keys.key_blocks) == 31,
        "uvmap": [uv.name for uv in head.data.uv_layers] == ["UVMap"],
        "modifier_order": [modifier.type for modifier in head.modifiers] == ["ARMATURE", "CORRECTIVE_SMOOTH", "SUBSURF"],
        "single_armature": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]) == 1,
        "rig_bones": len(rig.data.bones) == 52,
        "actions": len(bpy.data.actions) == 60,
        "active_talk_loop": active_action is not None and active_action.name == "Talk_Loop",
        "eye_action_curves_restored": len(eye_curves) == 18 and all(not curve.mute for curve in eye_curves),
        "shared_eyelid_material": all(obj.material_slots and obj.material_slots[0].material is lid_material for obj in lids),
        "eyelid_warm_brown_ramp": ramp is not None and 0.12 < ramp.color_ramp.elements[0].color[0] < 0.16 and 0.18 < ramp.color_ramp.elements[1].color[0] < 0.22,
        "eyelid_soft_surface": bump is not None and bump.inputs["Strength"].default_value <= 0.01 and lid_shader is not None and lid_shader.inputs["Roughness"].default_value >= 0.65,
        "blink_seam": all(value < 1.0e-5 for value in seam_max.values()),
        "lid_drivers": driver_count == 8,
        "reference_eye_metrics": all(metrics["iris"] == [0.07, 0.0018, 0.07] and metrics["limbal"] == [0.073, 0.0001, 0.073] and metrics["pupil"] == [0.04, 0.002, 0.04] and metrics["sclera"] == [0.123, 0.09, 0.148] for metrics in eye_metrics.values()),
        "secondary_catchlights": all(bpy.data.objects.get(f"EYE_CatchlightSecondary_{side}") is not None and f"EYE_CatchlightSecondary_{side}" in formal_names for side in ("L", "R")),
        "socket_warm_brown": socket_shader is not None and tuple(round(value, 3) for value in socket_shader.inputs["Base Color"].default_value[:3]) == (0.115, 0.038, 0.013),
        "render_pairs": all(os.path.exists(path) and os.path.getsize(path) > 100000 for record in render_pairs.values() for path in record.values()),
        "no_temporary_objects": not any(obj.name.startswith(("TMP_", "CXR_")) for obj in bpy.data.objects),
    }
    failed = [name for name, value in checks.items() if not value]
    report = {
        "schema": "sloth_reference_eye_eyelid_final_validation_v1",
        "status": "PASS" if not failed else "FAIL",
        "final": FINAL,
        "summary": {"passed": len(checks) - len(failed), "total": len(checks), "failed": failed},
        "checks": checks,
        "eye_metrics": eye_metrics,
        "blink_seam_max": seam_max,
        "eye_action_curve_count": len(eye_curves),
        "render_pairs": render_pairs,
    }
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    print(json.dumps({"status": report["status"], "report": REPORT, "summary": report["summary"], "eye_metrics": eye_metrics, "blink_seam_max": seam_max, "eye_action_curve_count": len(eye_curves)}, ensure_ascii=False))
    if failed:
        raise RuntimeError("Reference eye/eyelid final validation failed: " + ", ".join(failed))


if __name__ == "__main__":
    main()
