import bpy
import json
import os
from mathutils import Vector

ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_030_fur.blend")
REPORT = os.path.join(ROOT, "reports", "gate5_groom_visual_cleanup.json")
RENDER_DIR = os.path.join(ROOT, "renders", "lookdev", "gate5")


def look_at(obj, point):
    obj.rotation_euler = (Vector(point) - obj.location).to_track_quat("-Z", "Y").to_euler()


disabled = []
for name in ("FUR_Ears", "FUR_Outline"):
    obj = bpy.data.objects.get(name)
    if obj:
        obj.hide_render = True
        obj["groom_status"] = "GUIDE_ONLY_PENDING_SURFACE_REFIT"
        disabled.append(name)

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
    ("rim", (4.90, 6.80, 2.65), (0.0, 0.0, 1.35), 80),
):
    cam.location = loc
    cam.data.lens = lens
    look_at(cam, target)
    scene.camera = cam
    path = os.path.join(RENDER_DIR, name + ".png")
    scene.render.filepath = path
    bpy.ops.render.render(write_still=True)
    outputs[name] = path
close_cam.location = (0.0, -4.63, 2.30)
close_cam.data.lens = 100
look_at(close_cam, (0.0, -0.02, 2.22))
scene.camera = close_cam
path = os.path.join(RENDER_DIR, "face_closeup.png")
scene.render.filepath = path
bpy.ops.render.render(write_still=True)
outputs["face_closeup"] = path

main_obj = bpy.data.objects["part_00000001.001"]
checks = {
    "main_vertex_count_preserved": len(main_obj.data.vertices) == 5478,
    "floating_guides_not_rendered": all(bpy.data.objects[name].hide_render for name in disabled),
    "core_grooms_visible": all(not bpy.data.objects[name].hide_render for name in ("FUR_Face", "FUR_Muzzle", "FUR_Body", "FUR_HeadTuft")),
}
if not all(checks.values()):
    raise RuntimeError("Gate 5 cleanup invariant failed: " + json.dumps(checks))
bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT)
with open(REPORT, "w", encoding="utf-8") as handle:
    json.dump({"gate": "5b", "status": "PASS", "disabled_guide_only_objects": disabled, "checks": checks, "renders": outputs}, handle, ensure_ascii=False, indent=2)
print(json.dumps({"status": "PASS", "checkpoint": CHECKPOINT, "report": REPORT, "checks": checks, "renders": outputs}, ensure_ascii=False))
