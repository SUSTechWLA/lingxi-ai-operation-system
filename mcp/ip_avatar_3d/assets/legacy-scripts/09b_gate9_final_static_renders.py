import bpy
import json
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
FINAL_BLEND = os.path.join(ROOT, "character_sloth_final.blend")
REPORT = os.path.join(ROOT, "reports", "gate9_static_render_report.json")
RENDER_DIR = os.path.join(ROOT, "renders", "final")
EXR_DIR = os.path.join(RENDER_DIR, "exr")


def ensure_dir(path):
    os.makedirs(path, exist_ok=True)


def look_at(obj, point):
    obj.rotation_euler = (Vector(point) - obj.location).to_track_quat("-Z", "Y").to_euler()


def set_agx(scene):
    result = {"view_transform": None, "look": None}
    try:
        scene.view_settings.view_transform = "AgX"
    except Exception:
        scene.view_settings.view_transform = "Filmic"
    result["view_transform"] = scene.view_settings.view_transform
    for candidate in ("AgX - Medium High Contrast", "Medium High Contrast", "None"):
        try:
            scene.view_settings.look = candidate
            break
        except Exception:
            continue
    result["look"] = scene.view_settings.look
    scene.view_settings.exposure = 0.0
    scene.view_settings.gamma = 1.0
    return result


def configure_cycles(scene, resolution_x, resolution_y, samples=32):
    scene.render.engine = "CYCLES"
    scene.cycles.device = "CPU"
    scene.cycles.samples = samples
    scene.cycles.use_denoising = True
    if hasattr(scene.cycles, "use_adaptive_sampling"):
        scene.cycles.use_adaptive_sampling = True
        scene.cycles.adaptive_threshold = 0.03
    scene.render.resolution_x = resolution_x
    scene.render.resolution_y = resolution_y
    scene.render.resolution_percentage = 100
    scene.render.film_transparent = False
    return set_agx(scene)


def save_render_result(scene, stem):
    exr_path = os.path.join(EXR_DIR, stem + ".exr")
    png_path = os.path.join(RENDER_DIR, stem + ".png")
    scene.render.image_settings.file_format = "OPEN_EXR"
    scene.render.image_settings.color_mode = "RGBA"
    scene.render.image_settings.color_depth = "32"
    scene.render.image_settings.exr_codec = "ZIP"
    scene.render.filepath = exr_path
    bpy.ops.render.render(write_still=True)
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"
    scene.render.image_settings.color_depth = "8"
    bpy.data.images["Render Result"].save_render(filepath=png_path, scene=scene)
    return {"png": png_path, "exr": exr_path}


ensure_dir(RENDER_DIR)
ensure_dir(EXR_DIR)
main_obj = bpy.data.objects.get("GEO_HeadBody")
rig = bpy.data.objects.get("RIG_Sloth")
char_collection = bpy.data.collections.get("COL_CHR_SLOTH_FINAL")
if main_obj is None or rig is None or char_collection is None:
    raise RuntimeError("Final character hierarchy missing")

keys = main_obj.data.shape_keys.key_blocks
for key in keys:
    if key.name != "Basis":
        key.value = 1.0 if key.name == "Mouth_Rest" else 0.0
scene_lookdev = bpy.data.scenes.get("SCENE_LOOKDEV")
scene_production = bpy.data.scenes.get("Scene")
if scene_lookdev is None or scene_production is None:
    raise RuntimeError("Final lookdev or production scene missing")

outputs = {}
color_management = {}
bpy.context.window.scene = scene_lookdev
color_management[scene_lookdev.name] = configure_cycles(scene_lookdev, 1024, 1024, 32)
cam = bpy.data.objects.get("CAM_LOOKDEV_65MM")
close_cam = bpy.data.objects.get("CAM_LOOKDEV_100MM") or cam
views = {
    "front": ((0.0, -8.40, 2.45), (0.0, 0.0, 1.30), 65),
    "front_34": ((5.94, -5.94, 2.45), (0.0, 0.0, 1.30), 65),
    "side": ((8.40, 0.0, 2.45), (0.0, 0.0, 1.30), 65),
    "back": ((0.0, 8.40, 2.45), (0.0, 0.0, 1.30), 65),
}
for name, (location, target, lens) in views.items():
    cam.location = location
    cam.data.lens = lens
    look_at(cam, target)
    scene_lookdev.camera = cam
    outputs[name] = save_render_result(scene_lookdev, name)

close_cam.location = (0.0, -4.63, 2.30)
close_cam.data.lens = 100
look_at(close_cam, (0.0, -0.02, 2.22))
scene_lookdev.camera = close_cam
outputs["face_closeup"] = save_render_result(scene_lookdev, "face_closeup")

bpy.context.window.scene = scene_production
color_management[scene_production.name] = configure_cycles(scene_production, 1280, 720, 32)
production_camera = bpy.data.objects.get("Camera_Medium") or scene_production.camera
if production_camera is None:
    raise RuntimeError("Production camera missing")
scene_production.camera = production_camera
outputs["production_scene"] = save_render_result(scene_production, "production_scene")

bpy.context.window.scene = scene_lookdev
scene_lookdev.camera = close_cam
checks = {
    "single_armature": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]) == 1,
    "main_vertex_count_preserved": len(main_obj.data.vertices) == 5478,
    "shared_collection_in_both_scenes": all(any(child == char_collection for child in scene.collection.children) for scene in (scene_lookdev, scene_production)),
    "cycles_used": scene_lookdev.render.engine == "CYCLES" and scene_production.render.engine == "CYCLES",
    "agx_used": all(data["view_transform"] == "AgX" for data in color_management.values()),
    "six_png_and_exr_pairs": len(outputs) == 6 and all(os.path.exists(pair["png"]) and os.path.exists(pair["exr"]) for pair in outputs.values()),
}
if not all(checks.values()):
    raise RuntimeError("Gate 9 static render invariant failed: " + json.dumps(checks))

bpy.ops.wm.save_as_mainfile(filepath=FINAL_BLEND)
report = {
    "gate": "9b",
    "status": "PASS",
    "result_blend": FINAL_BLEND,
    "engine": "CYCLES",
    "samples": 32,
    "denoising": True,
    "color_management": color_management,
    "outputs": outputs,
    "checks": checks,
    "notes": [
        "Each EXR/PNG pair comes from one Cycles render result.",
        "EXR files use 32-bit RGBA ZIP; PNG files use the same AgX display transform.",
        "No bloom, depth-of-field cheat, or high-sample masking was used.",
    ],
}
with open(REPORT, "w", encoding="utf-8") as handle:
    json.dump(report, handle, ensure_ascii=False, indent=2)
print(json.dumps({"status": "PASS", "blend": FINAL_BLEND, "report": REPORT, "checks": checks, "outputs": outputs}, ensure_ascii=False))
