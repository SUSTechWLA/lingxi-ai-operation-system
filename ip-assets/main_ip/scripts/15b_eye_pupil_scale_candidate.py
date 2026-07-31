import bpy
import json
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
FINAL = os.path.join(ROOT, "character_sloth_final.blend")
CANDIDATE = os.path.join(ROOT, "checkpoints", "sloth_102_eye_pupil_scale_candidate.blend")
REPORT = os.path.join(ROOT, "reports", "eye_pupil_scale_candidate.json")
RENDER = os.path.join(ROOT, "renders", "eye_internal_shadow", "candidate_pupil_scale.png")
PUPIL_DIAMETER = 0.0325


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


def set_dimensions_preserving_parent_transform(obj, dimensions):
    bpy.context.view_layer.update()
    current = obj.dimensions.copy()
    for axis in range(3):
        if abs(current[axis]) > 1.0e-9:
            obj.scale[axis] *= dimensions[axis] / current[axis]
    bpy.context.view_layer.update()


def main():
    os.makedirs(os.path.dirname(CANDIDATE), exist_ok=True)
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    os.makedirs(os.path.dirname(RENDER), exist_ok=True)
    if os.path.abspath(bpy.data.filepath) != os.path.abspath(FINAL):
        bpy.ops.wm.open_mainfile(filepath=FINAL)

    head = bpy.data.objects.get("GEO_HeadBody")
    rig = bpy.data.objects.get("RIG_Sloth")
    scene = bpy.data.scenes.get("SCENE_LOOKDEV") or bpy.context.scene
    if not head or not head.data.shape_keys or not rig:
        raise RuntimeError("Formal head/rig system missing")

    centers_before = {}
    dimensions_before = {}
    for side in ("L", "R"):
        pupil = bpy.data.objects.get(f"EYE_Pupil_{side}")
        if not pupil:
            raise RuntimeError("Missing formal pupil: " + side)
        centers_before[side] = pupil.matrix_world.translation.copy()
        dimensions_before[side] = tuple(pupil.dimensions)
        set_dimensions_preserving_parent_transform(
            pupil, (PUPIL_DIAMETER, dimensions_before[side][1], PUPIL_DIAMETER)
        )
    bpy.context.view_layer.update()

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

    old_camera = scene.camera
    old_engine = scene.render.engine
    old_resolution = (scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage)
    old_path = scene.render.filepath
    old_format = scene.render.image_settings.file_format
    old_mode = scene.render.image_settings.color_mode
    old_depth = scene.render.image_settings.color_depth
    old_samples = scene.cycles.samples
    old_denoising = scene.cycles.use_denoising

    camera_data = bpy.data.cameras.new("CXR_PupilScaleCandidateCamera")
    camera = bpy.data.objects.new("CXR_PupilScaleCandidateCamera", camera_data)
    scene.collection.objects.link(camera)
    camera.data.sensor_width = 36.0
    camera.data.lens = 100.0
    camera.location = (0.0, -3.35, 2.20)
    point_at(camera, (0.0, -0.02, 2.20))
    scene.camera = camera
    scene.render.engine = "CYCLES"
    scene.cycles.samples = 16
    scene.cycles.use_denoising = True
    scene.render.resolution_x = 768
    scene.render.resolution_y = 768
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"
    scene.render.image_settings.color_depth = "8"
    scene.render.filepath = RENDER
    bpy.ops.render.render(write_still=True)

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

    centers_after = {side: bpy.data.objects[f"EYE_Pupil_{side}"].matrix_world.translation.copy() for side in ("L", "R")}
    dimensions_after = {side: tuple(bpy.data.objects[f"EYE_Pupil_{side}"].dimensions) for side in ("L", "R")}
    checks = {
        "head_vertex_count_preserved": len(head.data.vertices) == 5478,
        "shape_key_count_preserved": len(head.data.shape_keys.key_blocks) == 31,
        "pupil_centers_preserved": all((centers_after[side] - centers_before[side]).length < 1.0e-6 for side in ("L", "R")),
        "pupil_scale_symmetric": all(abs(dimensions_after[side][0] - PUPIL_DIAMETER) < 1.0e-5 and abs(dimensions_after[side][2] - PUPIL_DIAMETER) < 1.0e-5 for side in ("L", "R")),
        "iris_unchanged": all(tuple(round(value, 4) for value in bpy.data.objects[f"EYE_Iris_{side}"].dimensions) == (0.0700, 0.0018, 0.0700) for side in ("L", "R")),
        "render_written": os.path.exists(RENDER) and os.path.getsize(RENDER) > 50000,
        "eye_animation_restored": all(curve.mute == old_mute for curve, old_mute in muted_records),
        "no_cxr_objects": not any(obj.name.startswith("CXR_") for obj in bpy.data.objects),
    }
    failed = [name for name, value in checks.items() if not value]
    if failed:
        raise RuntimeError("Pupil scale candidate failed: " + ", ".join(failed))
    bpy.ops.wm.save_as_mainfile(filepath=CANDIDATE, copy=True)
    report = {
        "schema": "sloth_eye_pupil_scale_candidate_v1",
        "status": "PASS",
        "source": FINAL,
        "candidate": CANDIDATE,
        "render": RENDER,
        "pupil_diameter_before": {side: round(dimensions_before[side][0], 6) for side in ("L", "R")},
        "pupil_diameter_after": PUPIL_DIAMETER,
        "pupil_to_iris_ratio": round(PUPIL_DIAMETER / 0.070, 4),
        "checks": checks,
    }
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    print(json.dumps({"status": "PASS", "candidate": CANDIDATE, "render": RENDER, "report": REPORT, "checks": checks}, ensure_ascii=False))


if __name__ == "__main__":
    main()
