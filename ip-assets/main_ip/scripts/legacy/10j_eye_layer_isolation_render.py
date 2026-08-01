import bpy
import json
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
RENDER_DIR = os.path.join(ROOT, "renders", "eye_bugfix", "layer_isolation")
REPORT = os.path.join(ROOT, "reports", "eye_layer_isolation.json")
os.makedirs(RENDER_DIR, exist_ok=True)
os.makedirs(os.path.dirname(REPORT), exist_ok=True)

scene = bpy.context.scene
head = bpy.data.objects.get("GEO_HeadBody")
for kb in head.data.shape_keys.key_blocks:
    if kb.name != "Basis":
        kb.value = 0.0
rest = head.data.shape_keys.key_blocks.get("Mouth_Rest")
if rest:
    rest.value = 1.0


def point_at(obj, target):
    obj.rotation_euler = (Vector(target) - obj.location).to_track_quat("-Z", "Y").to_euler()


old_camera = scene.camera
old_engine = scene.render.engine
old_res = (scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage)
old_path = scene.render.filepath
old_format = scene.render.image_settings.file_format
old_film = scene.render.film_transparent
old_world_color = tuple(scene.world.color) if scene.world else None

cam_data = bpy.data.cameras.new("TMP_EyeLayerIsolationCamera")
cam = bpy.data.objects.new("TMP_EyeLayerIsolationCamera", cam_data)
scene.collection.objects.link(cam)
cam.data.sensor_width = 36.0
cam.data.lens = 100.0
cam.location = (0.0, -3.35, 2.20)
point_at(cam, (0.0, 0.0, 2.18))
scene.camera = cam
temps = [cam]
for name, location, energy, size in (
    ("TMP_EyeLayer_Key", (-2.2, -3.0, 4.0), 700.0, 2.0),
    ("TMP_EyeLayer_Fill", (2.4, -2.4, 3.0), 430.0, 2.5),
    ("TMP_EyeLayer_Rim", (1.5, 1.8, 3.5), 560.0, 1.7),
):
    data = bpy.data.lights.new(name, "AREA")
    data.energy = energy
    data.shape = "DISK"
    data.size = size
    obj = bpy.data.objects.new(name, data)
    scene.collection.objects.link(obj)
    obj.location = location
    point_at(obj, (0.0, 0.0, 2.18))
    temps.append(obj)

scene.render.engine = "BLENDER_EEVEE"
scene.render.resolution_x = 720
scene.render.resolution_y = 720
scene.render.resolution_percentage = 100
scene.render.image_settings.file_format = "PNG"
scene.render.film_transparent = False
if scene.world:
    scene.world.color = (0.055, 0.055, 0.055)

tracked = [
    bpy.data.objects.get(f"EYE_{role}_{side}")
    for role in ("Cornea", "Pupil", "Catchlight") for side in ("L", "R")
]
visibility = {obj.name: obj.hide_render for obj in tracked if obj}


def render(name):
    scene.render.filepath = os.path.join(RENDER_DIR, name)
    bpy.ops.render.render(write_still=True)


render("01_full.png")
for side in ("L", "R"):
    bpy.data.objects[f"EYE_Cornea_{side}"].hide_render = True
render("02_cornea_off.png")
for side in ("L", "R"):
    bpy.data.objects[f"EYE_Cornea_{side}"].hide_render = False
    bpy.data.objects[f"EYE_Pupil_{side}"].hide_render = True
render("03_pupil_off.png")

for obj in tracked:
    if obj:
        obj.hide_render = visibility[obj.name]

scene.camera = old_camera
scene.render.engine = old_engine
scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage = old_res
scene.render.filepath = old_path
scene.render.image_settings.file_format = old_format
scene.render.film_transparent = old_film
if scene.world and old_world_color:
    scene.world.color = old_world_color
for obj in temps:
    data = obj.data
    bpy.data.objects.remove(obj, do_unlink=True)
    if data and data.users == 0:
        if isinstance(data, bpy.types.Camera):
            bpy.data.cameras.remove(data)
        elif isinstance(data, bpy.types.Light):
            bpy.data.lights.remove(data)

report = {
    "status": "PASS",
    "renders": [os.path.join(RENDER_DIR, name) for name in (
        "01_full.png", "02_cornea_off.png", "03_pupil_off.png"
    )],
    "visibility_restored": all(obj.hide_render == visibility[obj.name] for obj in tracked if obj),
}
with open(REPORT, "w", encoding="utf-8") as f:
    json.dump(report, f, ensure_ascii=False, indent=2)
print(json.dumps(report, ensure_ascii=False))
