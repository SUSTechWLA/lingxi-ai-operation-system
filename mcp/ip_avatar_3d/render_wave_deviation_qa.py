#!/usr/bin/env python3
"""Render wrist-deviation candidates while the palm remains camera-facing."""

from __future__ import annotations

import json
import sys
from pathlib import Path

import bpy
from mathutils import Vector

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

from blender_renderer import apply_lighting_preset, finger_euler
from rig_semantics import resolve_bone_roles


def look_at(obj: bpy.types.Object, target: Vector) -> None:
    obj.rotation_euler = (target - obj.location).to_track_quat("-Z", "Y").to_euler()


def main() -> None:
    args = sys.argv[sys.argv.index("--") + 1 :] if "--" in sys.argv else []
    if not args:
        raise RuntimeError("usage: blender character.blend --python render_wave_deviation_qa.py -- output_dir")
    output_dir = Path(args[0]).expanduser().resolve()
    output_dir.mkdir(parents=True, exist_ok=True)

    scene = bpy.context.scene
    scene.timeline_markers.clear()
    scene.render.resolution_x = 720
    scene.render.resolution_y = 720
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"
    apply_lighting_preset("editorial_soft")

    armature = max(
        (obj for obj in scene.objects if obj.type == "ARMATURE"),
        key=lambda obj: len(obj.data.bones),
    )
    encoded_map = armature.get("ip_avatar_bone_map")
    bone_map = json.loads(encoded_map) if encoded_map else resolve_bone_roles(
        bone.name for bone in armature.data.bones
    )
    action = bpy.data.actions.get("Gesture_Wave")
    if not action:
        raise RuntimeError("character Blend has no Gesture_Wave action")
    armature.animation_data_create()
    armature.animation_data.action = action

    close_camera = bpy.data.objects.get("Camera_Close") or scene.camera
    medium_camera = bpy.data.objects.get("Camera_Medium")
    if not close_camera or not medium_camera:
        raise RuntimeError("character Blend requires close and medium cameras")

    candidates = (
        ("neutral", 15, 0.0, 0.0),
        ("x_neg", 15, -0.12, 0.0),
        ("x_pos", 15, 0.12, 0.0),
        ("z_neg", 15, 0.0, -0.12),
        ("z_pos", 15, 0.0, 0.12),
        ("combo_neg", 15, -0.10, -0.18),
        ("combo_small_neg", 15, -0.035, -0.055),
        ("frame30_neutral", 30, 0.0, 0.0),
    )
    report: list[dict[str, object]] = []
    for label, frame, x_value, z_value in candidates:
        scene.frame_set(frame)
        hand = armature.pose.bones[bone_map["hand_r"]]
        hand.rotation_mode = "XYZ"
        hand.rotation_euler = (x_value, -1.10, z_value)
        for digit, splay in ((1, 0.14), (2, 0.0), (3, -0.14)):
            armature.pose.bones[bone_map[f"finger_{digit}_r"]].rotation_euler = finger_euler(
                "r", 0.015, splay
            )
            armature.pose.bones[bone_map[f"finger_{digit}_tip_r"]].rotation_euler = finger_euler(
                "r", 0.005, splay * 0.20
            )
        bpy.context.view_layer.update()

        scene.camera = medium_camera
        medium_path = output_dir / f"{label}_medium.png"
        scene.render.filepath = str(medium_path)
        bpy.ops.render.render(write_still=True)

        target = armature.matrix_world @ hand.tail
        close_camera.location = target + Vector((0.0, -0.72, 0.04))
        close_camera.data.lens = 72
        close_camera.data.clip_start = 0.01
        look_at(close_camera, target)
        scene.camera = close_camera
        close_path = output_dir / f"{label}_close.png"
        scene.render.filepath = str(close_path)
        bpy.ops.render.render(write_still=True)
        report.append(
            {
                "label": label,
                "frame": frame,
                "wristEuler": [x_value, -1.10, z_value],
                "mediumPath": str(medium_path),
                "closePath": str(close_path),
            }
        )

    report_path = output_dir / "wave-deviation-report.json"
    report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(f"WAVE_DEVIATION_QA_REPORT={report_path}")


if __name__ == "__main__":
    main()
