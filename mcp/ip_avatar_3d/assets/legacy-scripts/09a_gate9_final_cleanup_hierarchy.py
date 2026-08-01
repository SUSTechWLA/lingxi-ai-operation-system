import bpy
import json
import os

ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_070_final_ready.blend")
REPORT = os.path.join(ROOT, "reports", "gate9_cleanup_hierarchy_report.json")
CHAR_COLLECTION = "COL_CHR_SLOTH_FINAL"


def role_handle(name, collection, integrated_in, role_type):
    existing = bpy.data.objects.get(name)
    if existing and existing.type == "EMPTY":
        obj = existing
    else:
        if existing:
            bpy.data.objects.remove(existing, do_unlink=True)
        obj = bpy.data.objects.new(name, None)
        collection.objects.link(obj)
    obj.empty_display_type = "PLAIN_AXES"
    obj.empty_display_size = 0.045
    obj.hide_render = True
    obj["formal_role"] = role_type
    obj["component_mode"] = "INTEGRATED"
    obj["integrated_in"] = integrated_in
    return obj


char_collection = bpy.data.collections.get(CHAR_COLLECTION)
if char_collection is None:
    raise RuntimeError("Final character collection missing")

removed_review = {"scenes": [], "objects": [], "collections": []}
review_scene = bpy.data.scenes.get("CXR_REVIEW")
if review_scene:
    removed_review["scenes"].append(review_scene.name)
    bpy.data.scenes.remove(review_scene)
review_collection = bpy.data.collections.get("CXR_REVIEW_SETUP")
if review_collection:
    for obj in list(review_collection.objects):
        if len(obj.users_collection) == 1:
            removed_review["objects"].append(obj.name)
            bpy.data.objects.remove(obj, do_unlink=True)
    removed_review["collections"].append(review_collection.name)
    bpy.data.collections.remove(review_collection)

removed_unfitted_grooms = []
for name in ("FUR_Ears", "FUR_Outline"):
    obj = bpy.data.objects.get(name)
    if obj and obj.type != "EMPTY":
        removed_unfitted_grooms.append(name)
        bpy.data.objects.remove(obj, do_unlink=True)

rename_objects = {
    "Armature": "RIG_Sloth",
    "part_00000001.001": "GEO_HeadBody",
    "part_00000002.001": "GEO_SecondaryBoundDetail",
    "IP_OralCavity": "GEO_MouthInterior",
    "IP_UpperTeeth": "GEO_TeethUpper",
    "IP_LowerTeeth": "GEO_TeethLower",
    "IP_Tongue": "GEO_Tongue",
}
renamed = {}
for old_name, new_name in rename_objects.items():
    obj = bpy.data.objects.get(old_name)
    if obj:
        obj.name = new_name
        if obj.data:
            obj.data.name = new_name + ("_Armature" if obj.type == "ARMATURE" else "_Mesh")
        renamed[old_name] = new_name

rig = bpy.data.objects.get("RIG_Sloth")
main_obj = bpy.data.objects.get("GEO_HeadBody")
if rig is None or main_obj is None:
    raise RuntimeError("Formal rig or main mesh rename failed")

secondary = bpy.data.objects.get("GEO_SecondaryBoundDetail")
if secondary and secondary.data.materials:
    secondary.data.materials[0].name = "MAT_SecondaryBoundDetail"

groom_collection = bpy.data.collections.get("GROOM_SLOTH")
clothing_collection = bpy.data.collections.get("CLOTHING_DETAILS")
if groom_collection is None or clothing_collection is None:
    raise RuntimeError("Formal Groom or clothing collection missing")

rib_mat = bpy.data.materials.get("MAT_Ribbing_Ivory")
for name in ("CLO_Ribbing_Cuff_L", "CLO_Ribbing_Cuff_R"):
    obj = bpy.data.objects.get(name)
    if obj and rib_mat:
        obj.data.materials.clear()
        obj.data.materials.append(rib_mat)

handles = []
handles.append(role_handle("GEO_Nose", char_collection, "GEO_HeadBody", "integrated_geometry_region"))
for name in ("CLO_Cardigan", "CLO_Hoodie", "CLO_Trousers", "CLO_Buttons", "CLO_Drawstrings"):
    handles.append(role_handle(name, clothing_collection, "GEO_HeadBody", "integrated_clothing_region"))
for name, integrated in (
    ("FUR_HandsFeet", "FUR_HandsFeet_L/R"),
    ("FUR_Ears", "FUR_Body"),
    ("FUR_Brows", "FUR_Brows_L/R"),
    ("FUR_Outline", "FUR_Body"),
):
    handles.append(role_handle(name, groom_collection, integrated, "groom_role"))

main_obj["formal_expression_system"] = "ShapeKeys_on_GEO_HeadBody"
main_obj["formal_material_system"] = "MAT_Character_Master + specialized eye/mouth/fur materials"
rig["formal_rig"] = True
groom_collection["formal_groom_system"] = True
clothing_collection["formal_clothing_detail_system"] = True

if hasattr(char_collection, "asset_mark"):
    char_collection.asset_mark()
    if char_collection.asset_data:
        char_collection.asset_data.author = "Tangying IP Production"
        char_collection.asset_data.description = "Production sloth character with one rig, one groom system, one expression system and shared lookdev/production scenes"

removed_unused_cxr_materials = []
for mat in list(bpy.data.materials):
    if mat.name.startswith("CXR_") and mat.users == 0:
        removed_unused_cxr_materials.append(mat.name)
        bpy.data.materials.remove(mat)

character_objects = []
for obj in bpy.data.objects:
    if any(collection == char_collection or collection.name in {"GROOM_SLOTH", "CLOTHING_DETAILS"} for collection in obj.users_collection):
        character_objects.append(obj)

formal_roles = [
    "RIG_Sloth", "GEO_HeadBody", "GEO_Nose", "GEO_MouthInterior", "GEO_TeethUpper", "GEO_TeethLower", "GEO_Tongue",
    "EYE_Sclera_L", "EYE_Sclera_R", "EYE_Iris_L", "EYE_Iris_R", "EYE_Cornea_L", "EYE_Cornea_R", "EYE_TearLine_L", "EYE_TearLine_R",
    "CLO_Cardigan", "CLO_Hoodie", "CLO_Trousers", "CLO_Buttons", "CLO_Drawstrings",
    "FUR_Face", "FUR_Muzzle", "FUR_Body", "FUR_HandsFeet", "FUR_Ears", "FUR_Brows", "FUR_HeadTuft", "FUR_Outline",
]
checks = {
    "single_formal_armature": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]) == 1 and rig.name == "RIG_Sloth",
    "main_vertex_count_preserved": len(main_obj.data.vertices) == 5478,
    "formal_roles_present": all(bpy.data.objects.get(name) is not None for name in formal_roles),
    "no_cxr_character_objects": not any(obj.name.startswith("CXR_") for obj in character_objects),
    "unfitted_groom_geometry_removed": all(bpy.data.objects[name].type == "EMPTY" for name in ("FUR_Ears", "FUR_Outline")),
    "two_final_scenes": {scene.name for scene in bpy.data.scenes} == {"Scene", "SCENE_LOOKDEV"},
    "collection_is_asset": char_collection.asset_data is not None,
}
if not all(checks.values()):
    raise RuntimeError("Gate 9 cleanup invariant failed: " + json.dumps(checks))

bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT)
report = {
    "gate": "9a",
    "status": "PASS",
    "source_checkpoint": os.path.join(ROOT, "checkpoints", "sloth_060_rig_validated.blend"),
    "result_checkpoint": CHECKPOINT,
    "removed_review_setup": removed_review,
    "removed_unfitted_groom_geometry": removed_unfitted_grooms,
    "renamed_objects": renamed,
    "role_handles": [{"name": obj.name, "integrated_in": obj["integrated_in"], "role": obj["formal_role"]} for obj in handles],
    "removed_unused_cxr_materials": removed_unused_cxr_materials,
    "formal_roles": formal_roles,
    "character_object_count": len(character_objects),
    "checks": checks,
}
with open(REPORT, "w", encoding="utf-8") as handle:
    json.dump(report, handle, ensure_ascii=False, indent=2)
print(json.dumps({"status": "PASS", "checkpoint": CHECKPOINT, "report": REPORT, "checks": checks, "formal_roles": formal_roles}, ensure_ascii=False))
