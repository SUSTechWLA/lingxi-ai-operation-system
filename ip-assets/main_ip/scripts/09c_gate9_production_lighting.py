import bpy
import json
import os
from mathutils import Vector

ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
FINAL_BLEND = os.path.join(ROOT, "character_sloth_final.blend")
REPORT = os.path.join(ROOT, "reports", "gate9_production_lighting_report.json")
PNG = os.path.join(ROOT, "renders", "final", "production_scene.png")
EXR = os.path.join(ROOT, "renders", "final", "exr", "production_scene.exr")


def look_at(obj, point):
    obj.rotation_euler = (Vector(point) - obj.location).to_track_quat("-Z", "Y").to_euler()


def add_area(name, location, energy, color, size, collection, target=(0.0, 0.0, 1.35)):
    old = bpy.data.objects.get(name)
    if old:
        bpy.data.objects.remove(old, do_unlink=True)
    data = bpy.data.lights.new(name + "_Data", "AREA")
    data.energy = energy
    data.color = color
    data.shape = "DISK"
    data.size = size
    obj = bpy.data.objects.new(name, data)
    collection.objects.link(obj)
    obj.location = location
    look_at(obj, target)
    return obj


scene = bpy.data.scenes.get("Scene")
if scene is None:
    raise RuntimeError("Production scene missing")
collection = bpy.data.collections.get("COL_PRODUCTION_LIGHTS")
if collection is None:
    collection = bpy.data.collections.new("COL_PRODUCTION_LIGHTS")
if collection.name not in {child.name for child in scene.collection.children}:
    scene.collection.children.link(collection)

lights = [
    add_area("LIGHT_PROD_Key", (3.8, -4.2, 4.8), 1050.0, (1.0, 0.74, 0.50), 3.4, collection),
    add_area("LIGHT_PROD_Fill", (-3.2, -2.6, 3.2), 520.0, (0.68, 0.78, 1.0), 4.5, collection),
    add_area("LIGHT_PROD_Rim", (0.6, 3.4, 4.2), 760.0, (1.0, 0.64, 0.40), 2.8, collection),
]

if scene.world and scene.world.use_nodes:
    background = next((node for node in scene.world.node_tree.nodes if node.type == "BACKGROUND"), None)
    if background:
        background.inputs["Color"].default_value = (0.055, 0.042, 0.032, 1.0)
        background.inputs["Strength"].default_value = 0.28

bpy.context.window.scene = scene
scene.camera = bpy.data.objects.get("Camera_Medium") or scene.camera
scene.render.engine = "CYCLES"
scene.cycles.device = "CPU"
scene.cycles.samples = 32
scene.cycles.use_denoising = True
scene.render.resolution_x = 1280
scene.render.resolution_y = 720
scene.render.resolution_percentage = 100
scene.render.film_transparent = False
scene.view_settings.view_transform = "AgX"
for candidate in ("AgX - Medium High Contrast", "Medium High Contrast", "None"):
    try:
        scene.view_settings.look = candidate
        break
    except Exception:
        continue

scene.render.image_settings.file_format = "OPEN_EXR"
scene.render.image_settings.color_mode = "RGBA"
scene.render.image_settings.color_depth = "32"
scene.render.image_settings.exr_codec = "ZIP"
scene.render.filepath = EXR
bpy.ops.render.render(write_still=True)
scene.render.image_settings.file_format = "PNG"
scene.render.image_settings.color_depth = "8"
bpy.data.images["Render Result"].save_render(filepath=PNG, scene=scene)

checks = {
    "three_production_lights": len([obj for obj in collection.objects if obj.type == "LIGHT"]) == 3,
    "cycles_agx": scene.render.engine == "CYCLES" and scene.view_settings.view_transform == "AgX",
    "outputs_exist": os.path.exists(PNG) and os.path.exists(EXR),
    "single_character_armature": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]) == 1,
}
if not all(checks.values()):
    raise RuntimeError("Production lighting invariant failed: " + json.dumps(checks))
bpy.ops.wm.save_as_mainfile(filepath=FINAL_BLEND)
with open(REPORT, "w", encoding="utf-8") as handle:
    json.dump({"gate": "9c", "status": "PASS", "lighting": [{"name": obj.name, "energy": obj.data.energy, "color": list(obj.data.color)} for obj in lights], "outputs": {"png": PNG, "exr": EXR}, "checks": checks}, handle, ensure_ascii=False, indent=2)
print(json.dumps({"status": "PASS", "blend": FINAL_BLEND, "report": REPORT, "checks": checks, "png": PNG, "exr": EXR}, ensure_ascii=False))
