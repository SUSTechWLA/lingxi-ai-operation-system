import bpy
import json
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
SOURCE = os.path.join(ROOT, "checkpoints", "sloth_102_eye_pupil_scale_candidate.blend")
CANDIDATE = os.path.join(ROOT, "checkpoints", "sloth_103_eye_pupil_color_candidate.blend")
REPORT = os.path.join(ROOT, "reports", "eye_pupil_color_candidate.json")
RENDER = os.path.join(ROOT, "renders", "eye_internal_shadow", "candidate_pupil_color.png")
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


def main():
    os.makedirs(os.path.dirname(CANDIDATE), exist_ok=True)
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    os.makedirs(os.path.dirname(RENDER), exist_ok=True)
    if not os.path.exists(SOURCE):
        raise RuntimeError("Pupil scale candidate is missing")
    if os.path.abspath(bpy.data.filepath) != os.path.abspath(SOURCE):
        bpy.ops.wm.open_mainfile(filepath=SOURCE)

    material = bpy.data.materials.get("MAT_Eye_Pupil")
    head = bpy.data.objects.get("GEO_HeadBody")
    rig = bpy.data.objects.get("RIG_Sloth")
    scene = bpy.data.scenes.get("SCENE_LOOKDEV") or bpy.context.scene
    if not material or not material.use_nodes or not head or not head.data.shape_keys or not rig:
        raise RuntimeError("Formal pupil/head/rig system missing")
    shader = next((node for node in material.node_tree.nodes if node.type == "BSDF_PRINCIPLED"), None)
    if not shader:
        raise RuntimeError("Pupil Principled shader missing")
    color_before = tuple(shader.inputs["Base Color"].default_value)
    shader.inputs["Base Color"].default_value = PUPIL_COLOR
    material.diffuse_color = PUPIL_COLOR
    material["reference_color_intent"] = "deep_warm_espresso_pupil_without_black_hole_read"

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

    camera_data = bpy.data.cameras.new("CXR_PupilColorCandidateCamera")
    camera = bpy.data.objects.new("CXR_PupilColorCandidateCamera", camera_data)
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

    pupil_dimensions = {
        side: [round(value, 6) for value in bpy.data.objects[f"EYE_Pupil_{side}"].dimensions]
        for side in ("L", "R")
    }
    checks = {
        "head_vertex_count_preserved": len(head.data.vertices) == 5478,
        "shape_key_count_preserved": len(head.data.shape_keys.key_blocks) == 31,
        "pupil_dimensions_preserved": all(values == [0.0325, 0.002, 0.0325] for values in pupil_dimensions.values()),
        "pupil_material_shared": all(bpy.data.objects[f"EYE_Pupil_{side}"].material_slots[0].material is material for side in ("L", "R")),
        "pupil_color_warm_espresso": tuple(round(value, 5) for value in shader.inputs["Base Color"].default_value) == tuple(round(value, 5) for value in PUPIL_COLOR),
        "render_written": os.path.exists(RENDER) and os.path.getsize(RENDER) > 50000,
        "eye_animation_restored": all(curve.mute == old_mute for curve, old_mute in muted_records),
        "no_cxr_objects": not any(obj.name.startswith("CXR_") for obj in bpy.data.objects),
    }
    failed = [name for name, value in checks.items() if not value]
    if failed:
        raise RuntimeError("Pupil color candidate failed: " + ", ".join(failed))
    bpy.ops.wm.save_as_mainfile(filepath=CANDIDATE, copy=True)
    report = {
        "schema": "sloth_eye_pupil_color_candidate_v1",
        "status": "PASS",
        "source": SOURCE,
        "candidate": CANDIDATE,
        "render": RENDER,
        "pupil_color_before": [round(value, 6) for value in color_before],
        "pupil_color_after": [round(value, 6) for value in PUPIL_COLOR],
        "pupil_dimensions": pupil_dimensions,
        "checks": checks,
    }
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    print(json.dumps({"status": "PASS", "candidate": CANDIDATE, "render": RENDER, "report": REPORT, "checks": checks}, ensure_ascii=False))


if __name__ == "__main__":
    main()
