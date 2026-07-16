#!/usr/bin/env python3
"""Render representative full-body frames for detailed hand motion events."""

from __future__ import annotations

import json
import sys
from pathlib import Path

import bpy

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import blender_renderer
from rig_semantics import resolve_bone_roles


def main() -> None:
    args = sys.argv[sys.argv.index("--") + 1 :] if "--" in sys.argv else []
    if not args:
        raise RuntimeError("usage: blender character.blend --python render_motion_event_qa.py -- output_dir")
    output_dir = Path(args[0]).expanduser().resolve()
    output_dir.mkdir(parents=True, exist_ok=True)

    scene = bpy.context.scene
    scene.timeline_markers.clear()
    scene.render.resolution_x = 960
    scene.render.resolution_y = 540
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"
    camera = bpy.data.objects.get("Camera_Wide") or scene.camera
    if not camera:
        raise RuntimeError("character Blend has no wide camera")
    scene.camera = camera

    armature = max(
        (obj for obj in scene.objects if obj.type == "ARMATURE"),
        key=lambda obj: len(obj.data.bones),
    )
    encoded_map = armature.get("ip_avatar_bone_map")
    bone_map = json.loads(encoded_map) if encoded_map else resolve_bone_roles(
        bone.name for bone in armature.data.bones
    )
    plan = {
        "durationSec": 6.2,
        "motionEvents": [
            {"timeSec": 0.0, "motion": "idle_breath", "duration": 6.2, "strength": 0.45},
            {"timeSec": 0.18, "motion": "weight_shift", "duration": 1.15, "strength": 0.28, "direction": 1},
            {"timeSec": 0.45, "motion": "wave", "duration": 0.95, "strength": 0.9},
            {"timeSec": 1.55, "motion": "open_hand", "duration": 0.9, "strength": 0.8},
            {"timeSec": 2.55, "motion": "wrist_twist", "duration": 1.1, "strength": 0.82},
            {"timeSec": 3.65, "motion": "finger_wave", "duration": 1.45, "strength": 0.82},
            {"timeSec": 5.05, "motion": "fist", "duration": 0.85, "strength": 0.72},
        ],
        "lipSync": [],
    }
    blender_renderer.animate(armature, {}, plan, fps=30, bone_map=bone_map)

    samples = (
        ("Rest", 1),
        ("Wave_A", 25),
        ("Wave_B", 32),
        ("OpenHand", 60),
        ("WristTwist", 93),
        ("FingerWave", 131),
        ("Fist", 164),
        ("Return", 186),
    )
    report: list[dict[str, object]] = []
    for label, frame in samples:
        scene.frame_set(frame)
        bpy.context.view_layer.update()
        path = output_dir / f"{label}.png"
        scene.render.filepath = str(path)
        bpy.ops.render.render(write_still=True)
        report.append(
            {
                "label": label,
                "frame": frame,
                "upperArmEuler": [round(float(value), 4) for value in armature.pose.bones[bone_map["upper_arm_r"]].rotation_euler],
                "forearmEuler": [round(float(value), 4) for value in armature.pose.bones[bone_map["forearm_r"]].rotation_euler],
                "wristEuler": [round(float(value), 4) for value in armature.pose.bones[bone_map["hand_r"]].rotation_euler],
                "fingerCurl": [
                    round(float(armature.pose.bones[bone_map[f"finger_{digit}_r"]].rotation_euler.z), 4)
                    for digit in (1, 2, 3)
                ],
                "path": str(path),
            }
        )

    report_path = output_dir / "motion-event-qa-report.json"
    report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(f"MOTION_EVENT_QA_REPORT={report_path}")


if __name__ == "__main__":
    main()
