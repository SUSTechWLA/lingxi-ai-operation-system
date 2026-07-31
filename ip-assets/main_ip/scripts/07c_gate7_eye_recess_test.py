import bpy
import json
import os
from mathutils import Vector

ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_051_eye_recess_test.blend")
REPORT = os.path.join(ROOT, "reports", "gate7_eye_recess_test.json")
RENDER_DIR = os.path.join(ROOT, "renders", "lookdev", "gate7_eye_recess")
os.makedirs(RENDER_DIR, exist_ok=True)

moved = {}
for prefix in ("EYE_Sclera_", "EYE_Cornea_", "EYE_Iris_", "EYE_Pupil_", "EYE_Catchlight_"):
    for side in ("L", "R"):
        obj = bpy.data.objects.get(prefix + side)
        if obj:
            world = obj.matrix_world.copy()
            before = list(world.translation)
            world.translation.y += 0.010
            obj.matrix_world = world
            moved[obj.name] = {"before": before, "after": list(obj.matrix_world.translation)}


def look_at(obj, point):
    obj.rotation_euler = (Vector(point) - obj.location).to_track_quat("-Z", "Y").to_euler()


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
    ("front_34", (5.94, -5.94, 2.45), (0.0, 0.0, 1.30), 65),
    ("side", (8.40, 0.0, 2.45), (0.0, 0.0, 1.30), 65),
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

checks = {
    "ten_eye_layers_recessed": len(moved) == 10,
    "lids_remained_at_orbital_plane": all(bpy.data.objects.get("EYE_Lid%s_%s" % (lid, side)) is not None for lid in ("Upper", "Lower") for side in ("L", "R")),
    "main_vertex_count_preserved": len(bpy.data.objects["part_00000001.001"].data.vertices) == 5478,
}
if not all(checks.values()):
    raise RuntimeError("Gate 7c invariant failed: " + json.dumps(checks))
bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT)
with open(REPORT, "w", encoding="utf-8") as handle:
    json.dump({"gate": "7c", "status": "TEST_READY", "moved": moved, "checks": checks, "renders": outputs}, handle, ensure_ascii=False, indent=2)
print(json.dumps({"status": "TEST_READY", "checkpoint": CHECKPOINT, "report": REPORT, "checks": checks, "renders": outputs}, ensure_ascii=False))
