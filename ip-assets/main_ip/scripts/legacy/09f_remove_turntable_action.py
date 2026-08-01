import bpy
import json
import os

ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
FINAL_BLEND = os.path.join(ROOT, "character_sloth_final.blend")
RIG_REPORT = os.path.join(ROOT, "reports", "rig_report.json")
REPORT = os.path.join(ROOT, "reports", "gate9_turntable_action_cleanup.json")

with open(RIG_REPORT, "r", encoding="utf-8") as handle:
    original = json.load(handle)
original_names = {action["name"] for action in original["actions"]}
current_names = {action.name for action in bpy.data.actions}
extras = [action for action in bpy.data.actions if action.name not in original_names]
removed = []
for action in extras:
    if action.users == 0:
        removed.append(action.name)
        bpy.data.actions.remove(action)

checks = {
    "exact_original_action_count": len(bpy.data.actions) == 60,
    "all_original_actions_present": {action.name for action in bpy.data.actions} == original_names,
    "one_turntable_action_removed": len(removed) == 1 and removed[0].startswith("CAM_LOOKDEV_65MM"),
    "active_character_action_preserved": bpy.data.objects["RIG_Sloth"].animation_data.action.name == "Talk_Loop",
}
if not all(checks.values()):
    raise RuntimeError("Turntable Action cleanup failed: " + json.dumps(checks))
bpy.ops.wm.save_as_mainfile(filepath=FINAL_BLEND)
with open(REPORT, "w", encoding="utf-8") as handle:
    json.dump({"gate": "9f", "status": "PASS", "removed": removed, "checks": checks}, handle, ensure_ascii=False, indent=2)
print(json.dumps({"status": "PASS", "blend": FINAL_BLEND, "report": REPORT, "removed": removed, "checks": checks}, ensure_ascii=False))
