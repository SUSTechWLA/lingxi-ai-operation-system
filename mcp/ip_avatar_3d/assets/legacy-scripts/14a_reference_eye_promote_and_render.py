import bpy
import json
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
SOURCE = os.path.join(ROOT, "checkpoints", "sloth_098_eyelid_color_polished.blend")
FINAL = os.path.join(ROOT, "character_sloth_final.blend")
REPORT = os.path.join(ROOT, "reports", "reference_eye_promote_render.json")
RENDER_DIR = os.path.join(ROOT, "renders", "final")


def point_at(obj, target):
    obj.rotation_euler = (Vector(target) - obj.location).to_track_quat("-Z", "Y").to_euler()


def render_pair(scene, camera, basename, location, target, lens):
    camera.location = location
    camera.data.lens = lens
    point_at(camera, target)
    scene.camera = camera
    png_path = os.path.join(RENDER_DIR, basename + ".png")
    exr_path = os.path.join(RENDER_DIR, basename + ".exr")
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"
    scene.render.image_settings.color_depth = "8"
    scene.render.filepath = png_path
    bpy.ops.render.render(write_still=True)
    scene.render.image_settings.file_format = "OPEN_EXR"
    scene.render.image_settings.color_mode = "RGBA"
    scene.render.image_settings.color_depth = "32"
    scene.render.filepath = exr_path
    bpy.ops.render.render(write_still=True)
    return {"png": png_path, "exr": exr_path}


def main():
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    os.makedirs(RENDER_DIR, exist_ok=True)
    if not os.path.exists(SOURCE):
        raise RuntimeError("Polished eyelid checkpoint missing")
    bpy.ops.wm.open_mainfile(filepath=SOURCE)
    head = bpy.data.objects.get("GEO_HeadBody")
    rig = bpy.data.objects.get("RIG_Sloth")
    if not head or len(head.data.vertices) != 5478 or not head.data.shape_keys or not rig:
        raise RuntimeError("Formal character core missing")
    if float(head.get("reference_muzzle_cheek_max_delta", 0.0)) <= 1.0e-8:
        for key in ("reference_muzzle_cheek_applied", "reference_muzzle_cheek_max_delta", "reference_muzzle_cheek_space"):
            if key in head:
                del head[key]

    scene = bpy.data.scenes.get("SCENE_LOOKDEV") or bpy.context.scene
    if bpy.context.window is not None:
        bpy.context.window.scene = scene
    old_camera = scene.camera
    old_engine = scene.render.engine
    old_resolution = (scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage)
    old_path = scene.render.filepath
    old_format = scene.render.image_settings.file_format
    old_mode = scene.render.image_settings.color_mode
    old_depth = scene.render.image_settings.color_depth
    old_samples = scene.cycles.samples
    old_values = {key.name: key.value for key in head.data.shape_keys.key_blocks}

    camera_data = bpy.data.cameras.new("TMP_ReferenceFinalCamera")
    camera = bpy.data.objects.new("TMP_ReferenceFinalCamera", camera_data)
    scene.collection.objects.link(camera)
    camera.data.sensor_width = 36.0
    scene.render.engine = "CYCLES"
    scene.cycles.samples = 24
    scene.cycles.use_denoising = True
    scene.render.resolution_x = 1024
    scene.render.resolution_y = 1024
    scene.render.resolution_percentage = 100
    scene.render.film_transparent = False

    for key in head.data.shape_keys.key_blocks:
        if key.name != "Basis":
            key.value = 0.0
    head.data.shape_keys.key_blocks["Mouth_Rest"].value = 1.0
    bpy.context.view_layer.update()

    outputs = {}
    outputs["front"] = render_pair(scene, camera, "front", (0.0, -10.25, 2.45), (0.0, 0.0, 1.30), 80.0)
    outputs["front_34"] = render_pair(scene, camera, "front_34", (7.25, -7.25, 2.45), (0.0, 0.0, 1.30), 80.0)
    outputs["side"] = render_pair(scene, camera, "side", (10.25, 0.0, 2.45), (0.0, 0.0, 1.30), 80.0)
    outputs["face_closeup"] = render_pair(scene, camera, "face_closeup", (0.0, -3.35, 2.20), (0.0, -0.02, 2.20), 100.0)
    head.data.shape_keys.key_blocks["Blink.L"].value = 1.0
    head.data.shape_keys.key_blocks["Blink.R"].value = 1.0
    bpy.context.view_layer.update()
    outputs["blink"] = render_pair(scene, camera, "blink", (0.0, -3.35, 2.20), (0.0, -0.02, 2.20), 100.0)

    for key in head.data.shape_keys.key_blocks:
        key.value = old_values[key.name]
    scene.camera = old_camera
    scene.render.engine = old_engine
    scene.cycles.samples = old_samples
    scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage = old_resolution
    scene.render.filepath = old_path
    scene.render.image_settings.file_format = old_format
    scene.render.image_settings.color_mode = old_mode
    scene.render.image_settings.color_depth = old_depth
    bpy.data.objects.remove(camera, do_unlink=True)
    bpy.data.cameras.remove(camera_data)
    bpy.context.view_layer.update()

    checks = {
        "head_vertex_count": len(head.data.vertices) == 5478,
        "head_shape_key_count": len(head.data.shape_keys.key_blocks) == 31,
        "single_armature": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]) == 1,
        "rig_bones": len(rig.data.bones) == 52,
        "actions": len(bpy.data.actions) == 60,
        "uvmap": [uv.name for uv in head.data.uv_layers] == ["UVMap"],
        "modifier_order": [modifier.type for modifier in head.modifiers] == ["ARMATURE", "CORRECTIVE_SMOOTH", "SUBSURF"],
        "no_temp_objects": not any(obj.name.startswith("TMP_") for obj in bpy.data.objects),
        "render_pairs": all(os.path.exists(path) and os.path.getsize(path) > 100000 for record in outputs.values() for path in record.values()),
    }
    failed = [name for name, value in checks.items() if not value]
    if failed:
        raise RuntimeError("Promotion validation failed: " + ", ".join(failed))
    bpy.ops.wm.save_as_mainfile(filepath=FINAL, copy=False)
    report = {
        "schema": "sloth_reference_eye_promote_render_v1",
        "status": "PASS",
        "source": SOURCE,
        "final": FINAL,
        "checks": checks,
        "renders": outputs,
        "notes": [
            "Reference eye candidate v4 retained: larger dark amber iris, larger pupil, reduced lower sclera, recessed globe.",
            "All four eyelids share the polished warm light-brown short-fur material.",
            "The failed world-space face-form experiment was not promoted.",
        ],
    }
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    print(json.dumps({"status": "PASS", "final": FINAL, "report": REPORT, "checks": checks, "renders": outputs}, ensure_ascii=False))


if __name__ == "__main__":
    main()
