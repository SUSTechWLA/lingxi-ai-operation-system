#!/usr/bin/env python3
"""Render a front-view wrist-roll sweep to find the authored palm-facing angle."""

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
        raise RuntimeError("usage: blender character.blend --python render_wave_palm_qa.py -- output_dir")
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

    angles = (-1.45, -1.10, -0.75, -0.40, 0.0, 0.40, 0.75, 1.10, 1.45)
    report: list[dict[str, object]] = []
    for index, angle in enumerate(angles):
        scene.frame_set(15)
        hand = armature.pose.bones[bone_map["hand_r"]]
        hand.rotation_mode = "XYZ"
        hand.rotation_euler = (0.04, angle, 0.08)
        for digit, splay in ((1, 0.14), (2, 0.0), (3, -0.14)):
            proximal = armature.pose.bones[bone_map[f"finger_{digit}_r"]]
            distal = armature.pose.bones[bone_map[f"finger_{digit}_tip_r"]]
            proximal.rotation_euler = finger_euler("r", 0.015, splay)
            distal.rotation_euler = finger_euler("r", 0.005, splay * 0.20)
        bpy.context.view_layer.update()

        target = armature.matrix_world @ hand.tail
        close_camera.location = target + Vector((0.0, -0.72, 0.04))
        close_camera.data.lens = 72
        close_camera.data.clip_start = 0.01
        look_at(close_camera, target)
        scene.camera = close_camera
        close_path = output_dir / f"angle_{index:02d}_{angle:+.2f}_close.png"
        scene.render.filepath = str(close_path)
        bpy.ops.render.render(write_still=True)

        scene.camera = medium_camera
        full_path = output_dir / f"angle_{index:02d}_{angle:+.2f}_medium.png"
        scene.render.filepath = str(full_path)
        bpy.ops.render.render(write_still=True)
        report.append(
            {
                "angle": angle,
                "closePath": str(close_path),
                "mediumPath": str(full_path),
            }
        )

    report_path = output_dir / "wave-palm-angle-report.json"
    report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(f"WAVE_PALM_QA_REPORT={report_path}")


if __name__ == "__main__":
    main()
