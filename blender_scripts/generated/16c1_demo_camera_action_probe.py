import bpy
import json
import os


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
REPORT = os.path.join(ROOT, "reports", "demo_camera_action_probe.json")


def animation_record(id_block):
    animation = getattr(id_block, "animation_data", None)
    action = animation.action if animation else None
    return {
        "has_animation_data": animation is not None,
        "action": action.name if action else None,
        "action_id_root": getattr(action, "id_root", None) if action else None,
        "action_users": action.users if action else None,
    }


def main():
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    camera = bpy.data.objects.get("CXR_DEMO_Camera")
    target = bpy.data.objects.get("CXR_DEMO_Target")
    if not camera or not target:
        raise RuntimeError("Demo camera or target missing")
    report = {
        "schema": "sloth_demo_camera_action_probe_v1",
        "status": "PASS",
        "blend_file": bpy.data.filepath,
        "camera_object": animation_record(camera),
        "camera_data": animation_record(camera.data),
        "target_object": animation_record(target),
        "cxr_actions": [
            {"name": action.name, "id_root": getattr(action, "id_root", None), "users": action.users}
            for action in bpy.data.actions if action.name.startswith("CXR_DEMO_")
        ],
    }
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    print(json.dumps(report, ensure_ascii=False))


if __name__ == "__main__":
    main()
