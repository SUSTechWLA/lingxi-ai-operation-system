import bpy
import json
import os


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
DEMO_BLEND = os.path.join(ROOT, "demos", "sloth_production_demo.blend")
REPORT = os.path.join(ROOT, "reports", "demo_camera_refine.json")
PREVIEW_DIR = os.path.join(ROOT, "renders", "demo", "preview_refined")


def remove_action(name):
    action = bpy.data.actions.get(name)
    if action:
        bpy.data.actions.remove(action)


def main():
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    os.makedirs(PREVIEW_DIR, exist_ok=True)
    if os.path.abspath(bpy.data.filepath) != os.path.abspath(DEMO_BLEND):
        bpy.ops.wm.open_mainfile(filepath=DEMO_BLEND)

    scene = bpy.data.scenes.get("SCENE_DEMO_PRODUCTION")
    camera = bpy.data.objects.get("CXR_DEMO_Camera")
    target = bpy.data.objects.get("CXR_DEMO_Target")
    if not scene or not camera or not target:
        raise RuntimeError("Demo scene camera system is missing")

    camera.animation_data_clear()
    camera.data.animation_data_clear()
    target.animation_data_clear()
    for name in ("CXR_DEMO_CameraAction", "CXR_DEMO_CameraLensAction", "CXR_DEMO_TargetAction"):
        remove_action(name)

    camera_keys = {
        1: ((0.0, -8.70, 2.06), 58.0),
        42: ((-0.14, -7.55, 2.10), 59.0),
        82: ((0.14, -6.65, 2.16), 62.0),
        120: ((0.0, -5.90, 2.22), 64.0),
    }
    for frame, (location, lens) in camera_keys.items():
        camera.location = location
        camera.data.lens = lens
        camera.keyframe_insert(data_path="location", frame=frame)
        camera.data.keyframe_insert(data_path="lens", frame=frame)
    target_keys = {
        1: (0.0, -0.01, 1.34),
        58: (0.0, -0.01, 1.44),
        120: (0.0, -0.02, 1.59),
    }
    for frame, location in target_keys.items():
        target.location = location
        target.keyframe_insert(data_path="location", frame=frame)
    camera_action = camera.animation_data.action if camera.animation_data else None
    lens_action = camera.data.animation_data.action if camera.data.animation_data else None
    if camera_action:
        camera_action.name = "CXR_DEMO_CameraAction"
    if lens_action and lens_action is not camera_action:
        lens_action.name = "CXR_DEMO_CameraLensAction"
    if target.animation_data and target.animation_data.action:
        target.animation_data.action.name = "CXR_DEMO_TargetAction"

    scene.camera = camera
    scene.render.resolution_x = 960
    scene.render.resolution_y = 540
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"
    scene.render.image_settings.color_depth = "8"
    previews = {}
    for frame in (1, 24, 36, 60, 84, 96, 120):
        scene.frame_set(frame)
        path = os.path.join(PREVIEW_DIR, f"demo_refined_{frame:03d}.png")
        scene.render.filepath = path
        bpy.ops.render.render(write_still=True)
        previews[str(frame)] = path
    scene.frame_set(1)

    head = bpy.data.objects.get("GEO_HeadBody")
    rig = bpy.data.objects.get("RIG_Sloth")
    checks = {
        "formal_head_invariants": head is not None and len(head.data.vertices) == 5478 and len(head.data.shape_keys.key_blocks) == 31,
        "single_armature": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]) == 1,
        "demo_action_preserved": rig is not None and rig.animation_data and rig.animation_data.action and rig.animation_data.action.name == "CXR_DEMO_TalkLoop_Slow",
        "camera_action_named": camera_action is not None and camera_action.name == "CXR_DEMO_CameraAction",
        "lens_action_bound": lens_action is not None and (lens_action is camera_action or lens_action.name == "CXR_DEMO_CameraLensAction"),
        "target_action_named": target.animation_data and target.animation_data.action and target.animation_data.action.name == "CXR_DEMO_TargetAction",
        "previews_written": all(os.path.exists(path) and os.path.getsize(path) > 50000 for path in previews.values()),
    }
    failed = [name for name, value in checks.items() if not value]
    if failed:
        raise RuntimeError("Demo camera refinement failed: " + ", ".join(failed))
    bpy.ops.wm.save_as_mainfile(filepath=DEMO_BLEND, copy=False)
    report = {
        "schema": "sloth_demo_camera_refine_v1",
        "status": "PASS",
        "demo_blend": DEMO_BLEND,
        "camera_keys": {str(frame): {"location": list(record[0]), "lens": record[1]} for frame, record in camera_keys.items()},
        "target_keys": {str(frame): list(location) for frame, location in target_keys.items()},
        "previews": previews,
        "checks": checks,
    }
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    print(json.dumps({"status": "PASS", "demo_blend": DEMO_BLEND, "report": REPORT, "previews": previews, "checks": checks}, ensure_ascii=False))


if __name__ == "__main__":
    main()
