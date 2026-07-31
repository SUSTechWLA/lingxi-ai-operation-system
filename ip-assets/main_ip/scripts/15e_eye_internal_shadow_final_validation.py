import bpy
import json
import os


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
FINAL = os.path.join(ROOT, "character_sloth_final.blend")
REPORT = os.path.join(ROOT, "reports", "eye_internal_shadow_final_validation.json")
DIAGNOSTIC = os.path.join(ROOT, "reports", "eye_internal_shadow_diagnostic.json")
FIX_REPORT = os.path.join(ROOT, "reports", "eye_internal_shadow_fix.json")
RENDER_DIR = os.path.join(ROOT, "renders", "final")
PUPIL_COLOR = (0.0060, 0.0014, 0.00035, 1.0)


def action_fcurves(action):
    curves = []
    if action is None:
        return curves
    legacy = getattr(action, "fcurves", None)
    if legacy is not None:
        return list(legacy)
    for layer in getattr(action, "layers", []):
        for strip in getattr(layer, "strips", []):
            for channelbag in getattr(strip, "channelbags", []):
                curves.extend(list(getattr(channelbag, "fcurves", [])))
    return curves


def main():
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    if os.path.abspath(bpy.data.filepath) != os.path.abspath(FINAL):
        bpy.ops.wm.open_mainfile(filepath=FINAL)

    head = bpy.data.objects.get("GEO_HeadBody")
    rig = bpy.data.objects.get("RIG_Sloth")
    formal = bpy.data.collections.get("COL_CHR_SLOTH_FINAL")
    pupil_material = bpy.data.materials.get("MAT_Eye_Pupil")
    lid_material = bpy.data.materials.get("MAT_Eyelid_Skin_Brown")
    if not head or not head.data.shape_keys or not rig or not formal or not pupil_material or not lid_material:
        raise RuntimeError("Formal character eye system incomplete")
    pupil_shader = next((node for node in pupil_material.node_tree.nodes if node.type == "BSDF_PRINCIPLED"), None)
    if not pupil_shader:
        raise RuntimeError("Pupil Principled shader missing")

    lids = [bpy.data.objects[f"EYE_Lid{part}_{side}"] for side in ("L", "R") for part in ("Upper", "Lower")]
    seam_max = {}
    for side in ("L", "R"):
        upper = bpy.data.objects[f"EYE_LidUpper_{side}"]
        lower = bpy.data.objects[f"EYE_LidLower_{side}"]
        seam_max[side] = max(
            (
                upper.matrix_world @ upper.data.shape_keys.key_blocks["Blink"].data[column * 6 + 5].co
                - lower.matrix_world @ lower.data.shape_keys.key_blocks["Blink"].data[column * 6 + 5].co
            ).length
            for column in range(48)
        )

    active_action = rig.animation_data.action if rig.animation_data else None
    eye_curves = [
        curve for curve in action_fcurves(active_action)
        if 'pose.bones["Eye.L"]' in curve.data_path or 'pose.bones["Eye.R"]' in curve.data_path
    ]
    lid_driver_count = sum(
        len(obj.data.shape_keys.animation_data.drivers)
        for obj in lids if obj.data.shape_keys and obj.data.shape_keys.animation_data
    )
    pupil_dimensions = {
        side: [round(value, 6) for value in bpy.data.objects[f"EYE_Pupil_{side}"].dimensions]
        for side in ("L", "R")
    }
    pupil_centers = {
        side: [round(value, 6) for value in bpy.data.objects[f"EYE_Pupil_{side}"].matrix_world.translation]
        for side in ("L", "R")
    }
    layer_depth = {
        side: {
            role: round(bpy.data.objects[f"EYE_{role}_{side}"].matrix_world.translation.y, 6)
            for role in ("Sclera", "Cornea", "Iris", "IrisLimbal", "Pupil")
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
        "diagnostic_report_present": os.path.exists(DIAGNOSTIC) and os.path.getsize(DIAGNOSTIC) > 1000,
        "fix_report_present": os.path.exists(FIX_REPORT) and os.path.getsize(FIX_REPORT) > 1000,
        "pre_fix_checkpoint_present": os.path.exists(os.path.join(ROOT, "checkpoints", "sloth_101_pre_eye_internal_shadow_fix.blend")),
        "head_vertex_count": len(head.data.vertices) == 5478,
        "shape_key_count": len(head.data.shape_keys.key_blocks) == 31,
        "uvmap": [uv.name for uv in head.data.uv_layers] == ["UVMap"],
        "modifier_order": [modifier.type for modifier in head.modifiers] == ["ARMATURE", "CORRECTIVE_SMOOTH", "SUBSURF"],
        "single_armature": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]) == 1,
        "rig_bones": len(rig.data.bones) == 52,
        "actions": len(bpy.data.actions) == 60,
        "active_talk_loop": active_action is not None and active_action.name == "Talk_Loop",
        "eye_action_curves_restored": len(eye_curves) == 18 and all(not curve.mute for curve in eye_curves),
        "pupil_dimensions_reduced": all(values == [0.0325, 0.002, 0.0325] for values in pupil_dimensions.values()),
        "pupil_ratio_reference_balanced": all(abs(values[0] / bpy.data.objects[f"EYE_Iris_{side}"].dimensions.x - 0.464286) < 1.0e-4 for side, values in pupil_dimensions.items()),
        "pupil_color_warm_espresso": tuple(round(value, 5) for value in pupil_shader.inputs["Base Color"].default_value) == tuple(round(value, 5) for value in PUPIL_COLOR),
        "pupil_material_shared": all(bpy.data.objects[f"EYE_Pupil_{side}"].material_slots[0].material is pupil_material for side in ("L", "R")),
        "iris_dimensions_preserved": all([round(value, 4) for value in bpy.data.objects[f"EYE_Iris_{side}"].dimensions] == [0.0700, 0.0018, 0.0700] for side in ("L", "R")),
        "sclera_dimensions_preserved": all([round(value, 4) for value in bpy.data.objects[f"EYE_Sclera_{side}"].dimensions] == [0.1230, 0.0900, 0.1480] for side in ("L", "R")),
        "pupil_centers_mirrored": abs(pupil_centers["L"][0] + pupil_centers["R"][0]) < 1.0e-4 and abs(pupil_centers["L"][1] - pupil_centers["R"][1]) < 1.0e-4 and abs(pupil_centers["L"][2] - pupil_centers["R"][2]) < 1.0e-4,
        "front_layer_order": all(depth["Pupil"] < depth["IrisLimbal"] < depth["Iris"] < depth["Sclera"] for depth in layer_depth.values()),
        "shared_eyelid_material": all(obj.material_slots and obj.material_slots[0].material is lid_material for obj in lids),
        "blink_seam": all(value < 1.0e-5 for value in seam_max.values()),
        "lid_drivers": lid_driver_count == 8,
        "catchlights_formal": all(f"EYE_Catchlight_{side}" in formal_names and f"EYE_CatchlightSecondary_{side}" in formal_names for side in ("L", "R")),
        "render_pairs": all(os.path.exists(path) and os.path.getsize(path) > 100000 for record in render_pairs.values() for path in record.values()),
        "no_temporary_objects": not any(obj.name.startswith(("TMP_", "CXR_")) for obj in bpy.data.objects),
    }
    failed = [name for name, value in checks.items() if not value]
    report = {
        "schema": "sloth_eye_internal_shadow_final_validation_v1",
        "status": "PASS" if not failed else "FAIL",
        "final": FINAL,
        "summary": {"passed": len(checks) - len(failed), "total": len(checks), "failed": failed},
        "checks": checks,
        "pupil_dimensions": pupil_dimensions,
        "pupil_centers": pupil_centers,
        "layer_depth": layer_depth,
        "blink_seam_max": seam_max,
        "eye_action_curve_count": len(eye_curves),
        "render_pairs": render_pairs,
    }
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    print(json.dumps({"status": report["status"], "report": REPORT, "summary": report["summary"], "pupil_dimensions": pupil_dimensions, "blink_seam_max": seam_max, "eye_action_curve_count": len(eye_curves)}, ensure_ascii=False))
    if failed:
        raise RuntimeError("Eye internal shadow final validation failed: " + ", ".join(failed))


if __name__ == "__main__":
    main()
