import bpy
import json
import os
import struct
from mathutils import Vector

ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
FINAL_BLEND = os.path.join(ROOT, "character_sloth_final.blend")
ORIGINAL = os.path.join(ROOT, "models", "checkpoints", "main-ip-aroll-master_20260718_114057_eye_redesign_final.blend")
REPORT = os.path.join(ROOT, "reports", "final_validation_report.json")
RENDER_DIR = os.path.join(ROOT, "renders", "final")
EXR_DIR = os.path.join(RENDER_DIR, "exr")
EXPECTED_KEYS = [
    "Basis", "Mouth_Rest", "Mouth_A", "Mouth_E", "Mouth_O", "Mouth_U", "Mouth_MBP",
    "Mouth_Smile", "Mouth_Frown", "Mouth_Surprise", "Eye_Squint.L", "Eye_Squint.R",
    "Eye_Wide.L", "Eye_Wide.R", "Eye_Look_Left", "Eye_Look_Right", "Brow_Raise.L",
    "Brow_Raise.R", "Brow_Furrow.L", "Brow_Furrow.R", "Cheek_Smile.L", "Cheek_Smile.R",
    "Cheek_Puff.L", "Cheek_Puff.R", "Nose_Flare.L", "Nose_Flare.R", "Blink.L", "Blink.R",
    "CheekRaise.L", "CheekRaise.R", "JawOpen",
]
FORMAL_ROLES = [
    "RIG_Sloth", "GEO_HeadBody", "GEO_Nose", "GEO_MouthInterior", "GEO_TeethUpper", "GEO_TeethLower", "GEO_Tongue",
    "EYE_Sclera_L", "EYE_Sclera_R", "EYE_Iris_L", "EYE_Iris_R", "EYE_Cornea_L", "EYE_Cornea_R", "EYE_TearLine_L", "EYE_TearLine_R",
    "CLO_Cardigan", "CLO_Hoodie", "CLO_Trousers", "CLO_Buttons", "CLO_Drawstrings",
    "FUR_Face", "FUR_Muzzle", "FUR_Body", "FUR_HandsFeet", "FUR_Ears", "FUR_Brows", "FUR_HeadTuft", "FUR_Outline",
]
RENDER_STEMS = ["front", "front_34", "side", "back", "face_closeup", "production_scene"]


def top_level_mp4_boxes(path):
    size = os.path.getsize(path)
    boxes = []
    with open(path, "rb") as handle:
        cursor = 0
        while cursor + 8 <= size:
            handle.seek(cursor)
            header = handle.read(8)
            length, kind = struct.unpack(">I4s", header)
            if length == 1:
                length = struct.unpack(">Q", handle.read(8))[0]
            if length < 8 or cursor + length > size:
                return boxes, False
            boxes.append({"type": kind.decode("ascii", "replace"), "offset": cursor, "size": length})
            cursor += length
    return boxes, cursor == size


def driver_records():
    records = []
    for owner in list(bpy.data.objects) + list(bpy.data.shape_keys):
        animation = getattr(owner, "animation_data", None)
        if animation and animation.drivers:
            for fcurve in animation.drivers:
                records.append({"owner": owner.name, "path": fcurve.data_path, "valid": bool(fcurve.driver.is_valid)})
    return records


main_obj = bpy.data.objects.get("GEO_HeadBody")
rig = bpy.data.objects.get("RIG_Sloth")
char_collection = bpy.data.collections.get("COL_CHR_SLOTH_FINAL")
lookdev = bpy.data.scenes.get("SCENE_LOOKDEV")
production = bpy.data.scenes.get("Scene")
if not all((main_obj, rig, char_collection, lookdev, production)):
    raise RuntimeError("Final validation prerequisites missing")

keys = main_obj.data.shape_keys.key_blocks
key_names = [key.name for key in keys]
neutral_values = {key.name: key.value for key in keys}
drivers = driver_records()
used_materials = sorted({mat.name for obj in bpy.data.objects for mat in (list(obj.data.materials) if obj.type in {"MESH", "CURVE", "CURVES"} and hasattr(obj.data, "materials") else []) if mat})

render_files = {}
for stem in RENDER_STEMS:
    png = os.path.join(RENDER_DIR, stem + ".png")
    exr = os.path.join(EXR_DIR, stem + ".exr")
    render_files[stem] = {
        "png": png,
        "png_size": os.path.getsize(png) if os.path.exists(png) else 0,
        "png_header": open(png, "rb").read(8).hex() if os.path.exists(png) else None,
        "exr": exr,
        "exr_size": os.path.getsize(exr) if os.path.exists(exr) else 0,
        "exr_header": open(exr, "rb").read(4).hex() if os.path.exists(exr) else None,
    }

mp4 = os.path.join(RENDER_DIR, "turntable.mp4")
mp4_boxes, mp4_complete = top_level_mp4_boxes(mp4) if os.path.exists(mp4) else ([], False)

checks = {
    "final_blend_is_open": os.path.abspath(bpy.data.filepath) == os.path.abspath(FINAL_BLEND),
    "original_source_preserved": os.path.exists(ORIGINAL) and os.path.abspath(ORIGINAL) != os.path.abspath(FINAL_BLEND),
    "only_two_final_scenes": {scene.name for scene in bpy.data.scenes} == {"Scene", "SCENE_LOOKDEV"},
    "shared_character_collection": all(any(child == char_collection for child in scene.collection.children) for scene in (lookdev, production)),
    "collection_marked_asset": char_collection.asset_data is not None,
    "single_armature": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]) == 1 and rig.type == "ARMATURE",
    "armature_bone_count": len(rig.data.bones) == 52,
    "actions_preserved": len(bpy.data.actions) == 60,
    "active_action_restored": rig.animation_data is not None and rig.animation_data.action is not None and rig.animation_data.action.name == "Talk_Loop",
    "main_vertex_count": len(main_obj.data.vertices) == 5478,
    "uv_preserved": [layer.name for layer in main_obj.data.uv_layers] == ["UVMap"],
    "modifier_order": [modifier.type for modifier in main_obj.modifiers] == ["ARMATURE", "CORRECTIVE_SMOOTH", "SUBSURF"],
    "shape_key_order": key_names == EXPECTED_KEYS,
    "neutral_shape_key_values": all(abs(value - (1.0 if name == "Mouth_Rest" else 0.0)) < 1.0e-6 for name, value in neutral_values.items() if name != "Basis"),
    "formal_drivers": len(drivers) == 8 and all(record["valid"] for record in drivers),
    "formal_roles_present": all(bpy.data.objects.get(name) is not None for name in FORMAL_ROLES),
    "no_cxr_or_tmp_objects": not any(obj.name.startswith(("CXR_", "TMP_")) for obj in bpy.data.objects),
    "single_groom_collection": bpy.data.collections.get("GROOM_SLOTH") is not None and bool(bpy.data.collections["GROOM_SLOTH"].get("formal_groom_system")),
    "single_expression_system": main_obj.get("formal_expression_system") == "ShapeKeys_on_GEO_HeadBody",
    "formal_material_system": main_obj.get("formal_material_system") is not None and not any(name.startswith("CXR_") for name in used_materials),
    "production_lights": len([obj for obj in bpy.data.collections.get("COL_PRODUCTION_LIGHTS", []).objects if obj.type == "LIGHT"]) == 3 if bpy.data.collections.get("COL_PRODUCTION_LIGHTS") else False,
    "cycles_agx_scenes": all(scene.render.engine == "CYCLES" and scene.view_settings.view_transform == "AgX" for scene in (lookdev, production)),
    "png_exr_pairs_valid": all(item["png_size"] > 100000 and item["png_header"] == "89504e470d0a1a0a" and item["exr_size"] > 100000 and item["exr_header"] == "762f3101" for item in render_files.values()),
    "turntable_mp4_valid": os.path.exists(mp4) and os.path.getsize(mp4) > 500000 and mp4_complete and {box["type"] for box in mp4_boxes} >= {"ftyp", "mdat", "moov"},
    "no_temporary_frame_directory": not os.path.exists(os.path.join(RENDER_DIR, "_turntable_frames")),
}

passed = [name for name, value in checks.items() if value]
failed = [name for name, value in checks.items() if not value]
status = "PASS" if not failed else "FAIL"
report = {
    "schema": "sloth_final_validation_v1",
    "status": status,
    "final_blend": FINAL_BLEND,
    "original_source": ORIGINAL,
    "summary": {"passed": len(passed), "total": len(checks), "failed": failed},
    "checks": checks,
    "shape_keys": key_names,
    "drivers": drivers,
    "actions": len(bpy.data.actions),
    "materials_used": used_materials,
    "formal_roles": FORMAL_ROLES,
    "renders": render_files,
    "turntable": {"path": mp4, "size": os.path.getsize(mp4) if os.path.exists(mp4) else 0, "top_level_boxes": mp4_boxes, "complete_box_parse": mp4_complete, "codec": "Motion JPEG in ISO-BMFF MP4"},
    "known_structural_notes": [
        "Nose and clothing roles are formal non-rendering handles because those regions remain integrated in GEO_HeadBody to preserve vertex order and binding.",
        "FUR_Ears and FUR_Outline are formal integrated-role handles referencing FUR_Body; rejected floating guide geometry was deleted.",
        "Legacy 36-driver CXR presentation rig was removed with its old eye system; the final eye system has 8 valid Blink/Wide lid drivers.",
    ],
}
with open(REPORT, "w", encoding="utf-8") as handle:
    json.dump(report, handle, ensure_ascii=False, indent=2)
print(json.dumps({"status": status, "report": REPORT, "summary": report["summary"], "failed": failed, "final_blend": FINAL_BLEND, "turntable_size": report["turntable"]["size"]}, ensure_ascii=False))
if failed:
    raise RuntimeError("Final validation failed: " + ", ".join(failed))
