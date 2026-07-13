#!/usr/bin/env python3
"""Render close hand poses from a completed talking-character Blend."""

from __future__ import annotations

import json
import sys
from pathlib import Path

import bpy
from mathutils import Vector


def look_at(obj: bpy.types.Object, target: Vector) -> None:
    obj.rotation_euler = (target - obj.location).to_track_quat("-Z", "Y").to_euler()


def main() -> None:
    args = sys.argv[sys.argv.index("--") + 1 :] if "--" in sys.argv else []
    if not args:
        raise RuntimeError("usage: blender character.blend --python render_hand_qa.py -- output_dir")
    output_dir = Path(args[0]).expanduser().resolve()
    output_dir.mkdir(parents=True, exist_ok=True)

    scene = bpy.context.scene
    scene.timeline_markers.clear()
    scene.render.resolution_x = 768
    scene.render.resolution_y = 768
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"
    eevee = getattr(scene, "eevee", None)
    if eevee and hasattr(eevee, "taa_render_samples"):
        eevee.taa_render_samples = 32

    armature = max(
        (obj for obj in scene.objects if obj.type == "ARMATURE"),
        key=lambda obj: len(obj.data.bones),
    )
    camera = bpy.data.objects.get("Camera_Close") or scene.camera
    if not camera:
        raise RuntimeError("character Blend has no camera")
    camera.data.lens = 72
    camera.data.clip_start = 0.01
    scene.camera = camera
    armature.animation_data_create()

    poses = [
        ("Wave_A", "Gesture_Wave", 15),
        ("Wave_B", "Gesture_Wave", 30),
        ("OpenHand", "Gesture_OpenHand", 30),
        ("Fist", "Gesture_Fist", 30),
        ("CountOne", "Gesture_Count_One", 30),
        ("CountTwo", "Gesture_Count_Two", 30),
        ("Pinch", "Gesture_Pinch", 30),
        ("WristTwist_A", "Gesture_WristTwist", 1),
        ("WristTwist_B", "Gesture_WristTwist", 30),
        ("FingerWave_A", "Gesture_FingerWave", 15),
        ("FingerWave_B", "Gesture_FingerWave", 30),
        ("FingerWave_C", "Gesture_FingerWave", 45),
    ]
    report: list[dict[str, object]] = []
    for label, action_name, frame in poses:
        action = bpy.data.actions.get(action_name)
        if not action:
            raise RuntimeError(f"missing hand QA action: {action_name}")
        armature.animation_data.action = action
        scene.frame_set(frame)
        bpy.context.view_layer.update()
        hand = armature.pose.bones.get("RightHand")
        if not hand:
            raise RuntimeError("character rig has no RightHand bone")
        target = armature.matrix_world @ hand.tail
        camera.location = target + Vector((0.0, -0.72, 0.06))
        look_at(camera, target)
        path = output_dir / f"{label}.png"
        scene.render.filepath = str(path)
        bpy.ops.render.render(write_still=True)
        report.append(
            {
                "label": label,
                "action": action_name,
                "frame": frame,
                "wristEuler": [round(float(value), 4) for value in hand.rotation_euler],
                "path": str(path),
            }
        )

    report_path = output_dir / "hand-qa-report.json"
    report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(f"HAND_QA_REPORT={report_path}")


if __name__ == "__main__":
    main()
