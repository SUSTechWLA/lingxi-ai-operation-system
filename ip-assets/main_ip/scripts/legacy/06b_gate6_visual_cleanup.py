import bpy
import json
import os
from mathutils import Vector

ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_040_clothes_hands.blend")
REPORT = os.path.join(ROOT, "reports", "gate6_visual_cleanup.json")
RENDER_DIR = os.path.join(ROOT, "renders", "lookdev", "gate6")


def look_at(obj, point):
    obj.rotation_euler = (Vector(point) - obj.location).to_track_quat("-Z", "Y").to_euler()


removed = []
for obj in list(bpy.data.objects):
    if obj.name.startswith(("CLO_Stitching_", "CLO_Ribbing_Hem")):
        removed.append(obj.name)
        bpy.data.objects.remove(obj, do_unlink=True)

scene = bpy.data.scenes.get("SCENE_LOOKDEV") or bpy.context.scene
bpy.context.window.scene = scene
scene.render.engine = "BLENDER_EEVEE"
scene.render.resolution_x = 768
scene.render.resolution_y = 768
scene.render.resolution_percentage = 100
scene.render.image_settings.file_format = "PNG"
scene.view_layers[0].material_override = None
cam = bpy.data.objects.get("CAM_LOOKDEV_65MM")
close_cam = bpy.data.objects.get("CAM_LOOKDEV_100MM") or cam
outputs = {}
for name, loc, target, lens in (
    ("front", (0.0, -8.40, 2.45), (0.0, 0.0, 1.30), 65),
    ("side", (8.40, 0.0, 2.45), (0.0, 0.0, 1.30), 65),
    ("back", (0.0, 8.40, 2.45), (0.0, 0.0, 1.30), 65),
):
    cam.location = loc
    cam.data.lens = lens
    look_at(cam, target)
    scene.camera = cam
    path = os.path.join(RENDER_DIR, name + ".png")
    scene.render.filepath = path
    bpy.ops.render.render(write_still=True)
    outputs[name] = path
close_cam.location = (0.43, -2.45, 1.12)
close_cam.data.lens = 100
look_at(close_cam, (0.43, -0.02, 1.05))
scene.camera = close_cam
path = os.path.join(RENDER_DIR, "hand_closeup.png")
scene.render.filepath = path
bpy.ops.render.render(write_still=True)
outputs["hand_closeup"] = path

main_obj = bpy.data.objects["part_00000001.001"]
checks = {
    "main_vertex_count_preserved": len(main_obj.data.vertices) == 5478,
    "floating_overlays_removed": not any(obj.name.startswith(("CLO_Stitching_", "CLO_Ribbing_Hem")) for obj in bpy.data.objects),
    "formal_cuffs_preserved": all(bpy.data.objects.get(name) is not None for name in ("CLO_Ribbing_Cuff_L", "CLO_Ribbing_Cuff_R")),
    "formal_nails_preserved": all(bpy.data.objects.get("GEO_Nail_%02d_%s" % (i, side)) is not None for i in range(1, 4) for side in ("L", "R")),
}
if not all(checks.values()):
    raise RuntimeError("Gate 6 cleanup invariant failed: " + json.dumps(checks))
bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT)
with open(REPORT, "w", encoding="utf-8") as handle:
    json.dump({"gate": "6b", "status": "PASS", "removed_unfitted_overlays": removed, "checks": checks, "renders": outputs, "material_detail_handoff": "Gate 7"}, handle, ensure_ascii=False, indent=2)
print(json.dumps({"status": "PASS", "checkpoint": CHECKPOINT, "report": REPORT, "checks": checks, "renders": outputs}, ensure_ascii=False))
