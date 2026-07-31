import bpy
import json
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
SOURCE = os.path.join(ROOT, "checkpoints", "sloth_103_eye_pupil_color_candidate.blend")
FINAL = os.path.join(ROOT, "character_sloth_final.blend")
REPORT = os.path.join(ROOT, "reports", "eye_internal_shadow_fix.json")
RENDER_DIR = os.path.join(ROOT, "renders", "final")
PUPIL_DIAMETER = 0.0325
PUPIL_COLOR = (0.0060, 0.0014, 0.00035, 1.0)


def point_at(obj, target):
    obj.rotation_euler = (Vector(target) - obj.location).to_track_quat("-Z", "Y").to_euler()


def action_fcurves(action):
    curves = []
    if action is None:
        return curves
    legacy = getattr(action, "fcurves", None)
    if legacy is not None:
        return list(legacy)
    for layer in getattr(action, "layers", []):
        for strip in getattr(layer, "strips", []):
            for channelbag in getattr(strip, "channelbags", []):
                curves.extend(list(getattr(channelbag, "fcurves", [])))
    return curves


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


def main():
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    os.makedirs(RENDER_DIR, exist_ok=True)
    if not os.path.exists(SOURCE):
        raise RuntimeError("Approved eye-shadow candidate is missing")
    if os.path.abspath(bpy.data.filepath) != os.path.abspath(SOURCE):
        bpy.ops.wm.open_mainfile(filepath=SOURCE)

    scene = bpy.data.scenes.get("SCENE_LOOKDEV") or bpy.context.scene
    head = bpy.data.objects.get("GEO_HeadBody")
    rig = bpy.data.objects.get("RIG_Sloth")
    pupil_material = bpy.data.materials.get("MAT_Eye_Pupil")
    if not head or not head.data.shape_keys or not rig or not pupil_material or not pupil_material.use_nodes:
        raise RuntimeError("Formal eye/head/rig system missing")
    pupil_shader = next((node for node in pupil_material.node_tree.nodes if node.type == "BSDF_PRINCIPLED"), None)
    if not pupil_shader:
        raise RuntimeError("Pupil Principled shader missing")

    action = rig.animation_data.action if rig.animation_data else None
    muted_records = []
    for curve in action_fcurves(action):
        if 'pose.bones["Eye.L"]' in curve.data_path or 'pose.bones["Eye.R"]' in curve.data_path:
            muted_records.append((curve, curve.mute))
            curve.mute = True
    old_eye_basis = {name: rig.pose.bones[name].matrix_basis.copy() for name in ("Eye.L", "Eye.R")}
    for name in ("Eye.L", "Eye.R"):
        rig.pose.bones[name].matrix_basis.identity()
    old_shape_values = {key.name: key.value for key in head.data.shape_keys.key_blocks}
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
    old_denoising = scene.cycles.use_denoising

    camera_data = bpy.data.cameras.new("CXR_EyeInternalShadowFinalCamera")
    camera = bpy.data.objects.new("CXR_EyeInternalShadowFinalCamera", camera_data)
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
        key.value = old_shape_values[key.name]
    for name in ("Eye.L", "Eye.R"):
        rig.pose.bones[name].matrix_basis = old_eye_basis[name]
    for curve, old_mute in muted_records:
        curve.mute = old_mute
    scene.camera = old_camera
    scene.render.engine = old_engine
    scene.cycles.samples = old_samples
    scene.cycles.use_denoising = old_denoising
    scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage = old_resolution
    scene.render.filepath = old_path
    scene.render.image_settings.file_format = old_format
    scene.render.image_settings.color_mode = old_mode
    scene.render.image_settings.color_depth = old_depth
    bpy.data.objects.remove(camera, do_unlink=True)
    bpy.data.cameras.remove(camera_data)
    bpy.context.view_layer.update()

    pupil_dimensions = {
        side: [round(value, 6) for value in bpy.data.objects[f"EYE_Pupil_{side}"].dimensions]
        for side in ("L", "R")
    }
    checks = {
        "head_vertex_count_preserved": len(head.data.vertices) == 5478,
        "shape_key_count_preserved": len(head.data.shape_keys.key_blocks) == 31,
        "pupil_dimensions": all(values == [PUPIL_DIAMETER, 0.002, PUPIL_DIAMETER] for values in pupil_dimensions.values()),
        "pupil_color": tuple(round(value, 5) for value in pupil_shader.inputs["Base Color"].default_value) == tuple(round(value, 5) for value in PUPIL_COLOR),
        "iris_dimensions_unchanged": all([round(value, 4) for value in bpy.data.objects[f"EYE_Iris_{side}"].dimensions] == [0.0700, 0.0018, 0.0700] for side in ("L", "R")),
        "neutral_eye_mirror_x": abs(neutral_centers["L"][0] + neutral_centers["R"][0]) < 1.0e-4,
        "neutral_eye_y_close": abs(neutral_centers["L"][1] - neutral_centers["R"][1]) < 1.0e-4,
        "neutral_eye_z_close": abs(neutral_centers["L"][2] - neutral_centers["R"][2]) < 1.0e-4,
        "eye_animation_restored": all(curve.mute == old_mute for curve, old_mute in muted_records),
        "active_action_restored": (rig.animation_data.action.name if rig.animation_data and rig.animation_data.action else None) == (action.name if action else None),
        "render_pairs": all(os.path.exists(path) and os.path.getsize(path) > 100000 for record in outputs.values() for path in record.values()),
        "no_cxr_objects": not any(obj.name.startswith("CXR_") for obj in bpy.data.objects),
    }
    failed = [name for name, value in checks.items() if not value]
    if failed:
        raise RuntimeError("Eye internal shadow promotion failed: " + ", ".join(failed))

    bpy.ops.wm.save_as_mainfile(filepath=FINAL, copy=False)
    report = {
        "schema": "sloth_eye_internal_shadow_fix_v1",
        "status": "PASS",
        "source": SOURCE,
        "final": FINAL,
        "root_cause": "The perceived black shadow was the oversized near-black pupil mesh, not cornea refraction, limbal shading, catchlights, or eye-socket shadow.",
        "changes": {
            "pupil_diameter_before": 0.0400,
            "pupil_diameter_after": PUPIL_DIAMETER,
            "pupil_to_iris_ratio_before": round(0.0400 / 0.0700, 4),
            "pupil_to_iris_ratio_after": round(PUPIL_DIAMETER / 0.0700, 4),
            "pupil_color_before": [0.00025, 0.00005, 0.00001, 1.0],
            "pupil_color_after": list(PUPIL_COLOR),
        },
        "pupil_dimensions": pupil_dimensions,
        "neutral_eye_centers": neutral_centers,
        "checks": checks,
        "renders": outputs,
    }
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    print(json.dumps({"status": "PASS", "final": FINAL, "report": REPORT, "checks": checks, "renders": outputs}, ensure_ascii=False))


if __name__ == "__main__":
    main()
