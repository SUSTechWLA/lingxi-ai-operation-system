import bpy
import json
import os

ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_050_materials.blend")
REPORT = os.path.join(ROOT, "reports", "gate7_eye_recess_promotion.json")

checks = {
    "source_is_recess_test": bpy.data.filepath.endswith("sloth_051_eye_recess_test.blend"),
    "main_vertex_count_preserved": len(bpy.data.objects["part_00000001.001"].data.vertices) == 5478,
    "single_armature": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]) == 1,
    "formal_eye_layers_present": all(bpy.data.objects.get(prefix + side) is not None for prefix in ("EYE_Sclera_", "EYE_Cornea_", "EYE_Iris_", "EYE_Pupil_", "EYE_LidUpper_", "EYE_LidLower_") for side in ("L", "R")),
}
if not all(checks.values()):
    raise RuntimeError("Gate 7 promotion invariant failed: " + json.dumps(checks))
bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT)
with open(REPORT, "w", encoding="utf-8") as handle:
    json.dump({"gate": "7d", "status": "PASS", "promoted_from": "sloth_051_eye_recess_test.blend", "result_checkpoint": CHECKPOINT, "checks": checks}, handle, ensure_ascii=False, indent=2)
print(json.dumps({"status": "PASS", "checkpoint": CHECKPOINT, "report": REPORT, "checks": checks}, ensure_ascii=False))
