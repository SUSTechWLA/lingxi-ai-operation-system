import bpy
import json
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
FINAL = os.path.join(ROOT, "character_sloth_final.blend")
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_099_pre_neutral_eye_rerender.blend")
REPORT = os.path.join(ROOT, "reports", "reference_final_neutral_eye_render.json")
RENDER_DIR = os.path.join(ROOT, "renders", "final")


def point_at(obj, target):
    obj.rotation_euler = (Vector(target) - obj.location).to_track_quat("-Z", "Y").to_euler()


def render_pair(scene, camera, basename, location, target, lens):
    camera.location = location
    camera.data.lens = lens
    point_at(camera, target)
    scene.camera = camera
    paths = {}
    for file_format, extension, depth in (("PNG", "png", "8"), ("OPEN_EXR", "exr", "32")):
        path = os.path.join(RENDER_DIR, basename + "." + extension)
        scene.render.image_settings.file_format = file_format
        scene.render.image_settings.color_mode = "RGBA"
        scene.render.image_settings.color_depth = depth
        scene.render.filepath = path
        bpy.ops.render.render(write_still=True)
        paths[extension] = path
    return paths


def action_fcurves(action):
    if action is None:
        return []
    legacy = getattr(action, "fcurves", None)
    if legacy is not None:
        return list(legacy)
    curves = []
    for layer in getattr(action, "layers", []):
        for strip in getattr(layer, "strips", []):
            for channelbag in getattr(strip, "channelbags", []):
                curves.extend(list(getattr(channelbag, "fcurves", [])))
    return curves


def main():
    os.makedirs(os.path.dirname(CHECKPOINT), exist_ok=True)
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    os.makedirs(RENDER_DIR, exist_ok=True)
    if os.path.abspath(bpy.data.filepath) != os.path.abspath(FINAL):
        bpy.ops.wm.open_mainfile(filepath=FINAL)
    bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT, copy=True)
    scene = bpy.data.scenes.get("SCENE_LOOKDEV") or bpy.context.scene
    rig = bpy.data.objects.get("RIG_Sloth")
    head = bpy.data.objects.get("GEO_HeadBody")
    if not rig or not head or not head.data.shape_keys:
        raise RuntimeError("Formal character missing")

    action = rig.animation_data.action if rig.animation_data else None
    eye_paths = ('pose.bones["Eye.L"]', 'pose.bones["Eye.R"]')
    muted_records = []
    if action:
        for curve in action_fcurves(action):
            if any(path in curve.data_path for path in eye_paths):
                muted_records.append((curve, curve.mute))
                curve.mute = True
    old_eye_basis = {name: rig.pose.bones[name].matrix_basis.copy() for name in ("Eye.L", "Eye.R")}
    for name in ("Eye.L", "Eye.R"):
        rig.pose.bones[name].matrix_basis.identity()

    old_values = {key.name: key.value for key in head.data.shape_keys.key_blocks}
    for key in head.data.shape_keys.key_blocks:
        if key.name != "Basis":
            key.value = 0.0
    head.data.shape_keys.key_blocks["Mouth_Rest"].value = 1.0
    bpy.context.view_layer.update()

    neutral_centers = {
        side: [round(value, 6) for value in bpy.data.objects[f"EYE_Pupil_{side}"].matrix_world.translation]
        for side in ("L", "R")
    }
    old_camera = scene.camera
    old_engine = scene.render.engine
    old_resolution = (scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage)
    old_path = scene.render.filepath
    old_format = scene.render.image_settings.file_format
    old_mode = scene.render.image_settings.color_mode
    old_depth = scene.render.image_settings.color_depth
    old_samples = scene.cycles.samples
    camera_data = bpy.data.cameras.new("TMP_FinalNeutralEyeCamera")
    camera = bpy.data.objects.new("TMP_FinalNeutralEyeCamera", camera_data)
    scene.collection.objects.link(camera)
    camera.data.sensor_width = 36.0
    scene.render.engine = "CYCLES"
    scene.cycles.samples = 24
    scene.cycles.use_denoising = True
    scene.render.resolution_x = 1024
    scene.render.resolution_y = 1024
    scene.render.resolution_percentage = 100
    scene.render.film_transparent = False

    outputs = {}
    outputs["front"] = render_pair(scene, camera, "front", (0.0, -7.40, 2.38), (0.0, 0.0, 1.30), 80.0)
    outputs["front_34"] = render_pair(scene, camera, "front_34", (5.23, -5.23, 2.38), (0.0, 0.0, 1.30), 80.0)
    outputs["side"] = render_pair(scene, camera, "side", (7.40, 0.0, 2.38), (0.0, 0.0, 1.30), 80.0)
    outputs["face_closeup"] = render_pair(scene, camera, "face_closeup", (0.0, -3.35, 2.20), (0.0, -0.02, 2.20), 100.0)
    head.data.shape_keys.key_blocks["Blink.L"].value = 1.0
    head.data.shape_keys.key_blocks["Blink.R"].value = 1.0
    bpy.context.view_layer.update()
    outputs["blink"] = render_pair(scene, camera, "blink", (0.0, -3.35, 2.20), (0.0, -0.02, 2.20), 100.0)

    for key in head.data.shape_keys.key_blocks:
        key.value = old_values[key.name]
    for name in ("Eye.L", "Eye.R"):
        rig.pose.bones[name].matrix_basis = old_eye_basis[name]
    for curve, old_mute in muted_records:
        curve.mute = old_mute
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
        "shape_key_count": len(head.data.shape_keys.key_blocks) == 31,
        "eye_fcurves_restored": all(curve.mute == old_mute for curve, old_mute in muted_records),
        "active_action_restored": (rig.animation_data.action.name if rig.animation_data and rig.animation_data.action else None) == (action.name if action else None),
        "neutral_eye_mirror_x": abs(neutral_centers["L"][0] + neutral_centers["R"][0]) < 1.0e-4,
        "neutral_eye_y_close": abs(neutral_centers["L"][1] - neutral_centers["R"][1]) < 1.0e-4,
        "neutral_eye_z_close": abs(neutral_centers["L"][2] - neutral_centers["R"][2]) < 1.0e-4,
        "render_pairs": all(os.path.exists(path) and os.path.getsize(path) > 100000 for record in outputs.values() for path in record.values()),
        "no_temp_objects": not any(obj.name.startswith("TMP_") for obj in bpy.data.objects),
    }
    failed = [name for name, value in checks.items() if not value]
    if failed:
        raise RuntimeError("Neutral eye rerender failed: " + ", ".join(failed))
    bpy.ops.wm.save_as_mainfile(filepath=FINAL, copy=False)
    report = {
        "schema": "sloth_reference_final_neutral_eye_render_v1",
        "status": "PASS",
        "checkpoint": CHECKPOINT,
        "final": FINAL,
        "eye_fcurves_temporarily_muted": len(muted_records),
        "neutral_eye_centers": neutral_centers,
        "checks": checks,
        "renders": outputs,
    }
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    print(json.dumps({"status": "PASS", "final": FINAL, "report": REPORT, "eye_fcurves_temporarily_muted": len(muted_records), "neutral_eye_centers": neutral_centers, "checks": checks, "renders": outputs}, ensure_ascii=False))


if __name__ == "__main__":
    main()
