import bpy
import json
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
REPORT = os.path.join(ROOT, "reports", "neutral_pose_probe.json")
RENDER = os.path.join(ROOT, "renders", "reference_refine", "neutral_pose_probe.png")


def point_at(obj, target):
    obj.rotation_euler = (Vector(target) - obj.location).to_track_quat("-Z", "Y").to_euler()


def eye_centers():
    return {
        side: [round(value, 6) for value in bpy.data.objects[f"EYE_Pupil_{side}"].matrix_world.translation]
        for side in ("L", "R")
    }


def main():
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    os.makedirs(os.path.dirname(RENDER), exist_ok=True)
    scene = bpy.data.scenes.get("SCENE_LOOKDEV") or bpy.context.scene
    rig = bpy.data.objects.get("RIG_Sloth")
    head = bpy.data.objects.get("GEO_HeadBody")
    if not rig or not head or not head.data.shape_keys:
        raise RuntimeError("Formal rig/head missing")
    animation_data = rig.animation_data
    old_action = animation_data.action if animation_data else None
    old_frame = scene.frame_current
    old_pose = {bone.name: bone.matrix_basis.copy() for bone in rig.pose.bones}
    old_values = {key.name: key.value for key in head.data.shape_keys.key_blocks}
    old_camera = scene.camera
    old_engine = scene.render.engine
    old_resolution = (scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage)
    old_path = scene.render.filepath
    old_format = scene.render.image_settings.file_format
    before_centers = eye_centers()

    if animation_data:
        animation_data.action = None
    for bone in rig.pose.bones:
        bone.matrix_basis.identity()
    for key in head.data.shape_keys.key_blocks:
        if key.name != "Basis":
            key.value = 0.0
    head.data.shape_keys.key_blocks["Mouth_Rest"].value = 1.0
    bpy.context.view_layer.update()
    neutral_centers = eye_centers()

    camera_data = bpy.data.cameras.new("TMP_NeutralPoseProbeCamera")
    camera = bpy.data.objects.new("TMP_NeutralPoseProbeCamera", camera_data)
    scene.collection.objects.link(camera)
    camera.data.sensor_width = 36.0
    camera.data.lens = 100.0
    camera.location = (0.0, -3.35, 2.20)
    point_at(camera, (0.0, -0.02, 2.20))
    scene.camera = camera
    scene.render.engine = "BLENDER_EEVEE"
    scene.render.resolution_x = 720
    scene.render.resolution_y = 720
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "PNG"
    scene.render.filepath = RENDER
    bpy.ops.render.render(write_still=True)

    bpy.data.objects.remove(camera, do_unlink=True)
    bpy.data.cameras.remove(camera_data)
    for bone in rig.pose.bones:
        bone.matrix_basis = old_pose[bone.name]
    if animation_data:
        animation_data.action = old_action
    scene.frame_set(old_frame)
    for key in head.data.shape_keys.key_blocks:
        key.value = old_values[key.name]
    scene.camera = old_camera
    scene.render.engine = old_engine
    scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage = old_resolution
    scene.render.filepath = old_path
    scene.render.image_settings.file_format = old_format
    bpy.context.view_layer.update()
    after_centers = eye_centers()

    checks = {
        "neutral_eye_mirror_x": abs(neutral_centers["L"][0] + neutral_centers["R"][0]) < 1.0e-5,
        "neutral_eye_equal_y": abs(neutral_centers["L"][1] - neutral_centers["R"][1]) < 1.0e-5,
        "neutral_eye_equal_z": abs(neutral_centers["L"][2] - neutral_centers["R"][2]) < 1.0e-5,
        "pose_restored": before_centers == after_centers,
        "action_restored": (rig.animation_data.action.name if rig.animation_data and rig.animation_data.action else None) == (old_action.name if old_action else None),
        "no_temp_objects": not any(obj.name.startswith("TMP_") for obj in bpy.data.objects),
        "render_exists": os.path.exists(RENDER) and os.path.getsize(RENDER) > 100000,
    }
    failed = [name for name, value in checks.items() if not value]
    report = {
        "schema": "sloth_neutral_pose_probe_v1",
        "status": "PASS" if not failed else "FAIL",
        "old_action": old_action.name if old_action else None,
        "old_frame": old_frame,
        "eye_centers_before": before_centers,
        "eye_centers_neutral": neutral_centers,
        "eye_centers_after": after_centers,
        "checks": checks,
        "failed": failed,
        "render": RENDER,
    }
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    print(json.dumps(report, ensure_ascii=False))
    if failed:
        raise RuntimeError("Neutral pose probe failed: " + ", ".join(failed))


if __name__ == "__main__":
    main()
