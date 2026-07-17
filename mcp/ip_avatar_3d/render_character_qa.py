#!/usr/bin/env python3
"""Render viseme and expression stills from a completed talking-character Blend."""

from __future__ import annotations

import sys
from pathlib import Path

import bpy
from mathutils import Vector


def look_at(obj: bpy.types.Object, target: tuple[float, float, float]) -> None:
    obj.rotation_euler = (Vector(target) - obj.location).to_track_quat("-Z", "Y").to_euler()


def render_still(scene: bpy.types.Scene, path: Path) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    scene.render.filepath = str(path)
    bpy.context.view_layer.update()
    bpy.ops.render.render(write_still=True)


def main() -> None:
    args = sys.argv[sys.argv.index("--") + 1 :] if "--" in sys.argv else []
    if len(args) < 2:
        raise RuntimeError("usage: blender --background character.blend --python render_character_qa.py -- character.blend output_dir")
    output_dir = Path(args[1]).expanduser().resolve()
    scene = bpy.context.scene
    scene.timeline_markers.clear()
    scene.frame_set(1)
    scene.render.resolution_x = 640
    scene.render.resolution_y = 640
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"

    camera = bpy.data.objects.get("Camera_Close") or scene.camera
    if not camera:
        raise RuntimeError("character Blend has no camera")
    camera.location = (0.0, -3.3, 2.05)
    camera.data.lens = 85
    look_at(camera, (0.0, 0.0, 2.05))
    scene.camera = camera

    mouth = bpy.data.objects.get("IP_Mouth") or next(
        (
            obj
            for obj in scene.objects
            if obj.type == "MESH"
            and obj.data.shape_keys
            and obj.data.shape_keys.key_blocks.get("Mouth_A")
        ),
        None,
    )
    if not mouth or not mouth.data.shape_keys:
        raise RuntimeError("character Blend has no Mouth_* shape keys")
    shape_keys = mouth.data.shape_keys
    shape_keys.animation_data_create()
    shape_keys.animation_data.action = None
    visemes = [
        "Mouth_Rest",
        "Mouth_A",
        "Mouth_E",
        "Mouth_O",
        "Mouth_U",
        "Mouth_MBP",
        "Mouth_Smile",
        "Mouth_Frown",
        "Mouth_Surprise",
    ]
    for viseme in visemes:
        for key in shape_keys.key_blocks:
            if key.name != "Basis":
                key.value = 1.0 if key.name == viseme else 0.0
        render_still(scene, output_dir / "viseme_stills" / f"{viseme}.png")

    armature = max(
        (obj for obj in scene.objects if obj.type == "ARMATURE"),
        key=lambda obj: len(obj.data.bones),
        default=None,
    )
    if not armature:
        raise RuntimeError("character Blend has no humanoid armature")
    armature.animation_data_create()
    camera.location = (0.0, -3.25, 2.08)
    camera.data.lens = 82
    look_at(camera, (0.0, 0.0, 2.08))
    expression_pairs = {
        "Face_Neutral": "Look_Camera",
        "Face_Happy": "Expression_Happy",
        "Face_Thinking": "Expression_Thinking",
        "Face_Surprised": "Expression_Surprised",
        "Face_Confused": "Expression_Confused",
        "Face_Serious": "Expression_Serious",
        "Face_Blink": "Look_Camera",
    }
    for face_action_name, body_action_name in expression_pairs.items():
        face_action = bpy.data.actions.get(face_action_name)
        body_action = bpy.data.actions.get(body_action_name)
        if not face_action or not body_action:
            raise RuntimeError(f"missing expression action pair: {face_action_name}/{body_action_name}")
        shape_keys.animation_data.action = face_action
        armature.animation_data.action = body_action
        scene.frame_set(round((float(face_action.frame_range[0]) + float(face_action.frame_range[1])) * 0.5))
        render_still(scene, output_dir / "expression_stills" / f"{face_action_name}.png")

    shape_keys.animation_data.action = bpy.data.actions.get("Face_Neutral")
    for action_name in ("Look_Left", "Look_Right"):
        action = bpy.data.actions.get(action_name)
        if not action:
            raise RuntimeError(f"missing gaze action: {action_name}")
        armature.animation_data.action = action
        scene.frame_set(round((float(action.frame_range[0]) + float(action.frame_range[1])) * 0.5))
        render_still(scene, output_dir / "expression_stills" / f"{action_name}.png")

    shape_keys.animation_data.action = bpy.data.actions.get("Face_Neutral")
    for key in shape_keys.key_blocks:
        if key.name != "Basis":
            key.value = 1.0 if key.name == "Mouth_Rest" else 0.0
    camera.location = (0.0, -5.2, 1.78)
    camera.data.lens = 58
    look_at(camera, (0.0, 0.0, 1.78))
    gesture_actions = (
        "Idle_Speaking",
        "Gesture_Wave",
        "Gesture_Explain",
        "Gesture_Point_Left",
        "Gesture_Point_Right",
        "Gesture_Think",
        "Gesture_Count_One",
        "Gesture_Count_Two",
        "Gesture_Pinch",
        "Gesture_OpenHand",
        "Gesture_Fist",
        "Gesture_WristTwist",
        "Gesture_FingerWave",
        "Gesture_Step",
    )
    for action_name in gesture_actions:
        action = bpy.data.actions.get(action_name)
        if not action:
            raise RuntimeError(f"missing gesture action: {action_name}")
        armature.animation_data.action = action
        scene.frame_set(round((float(action.frame_range[0]) + float(action.frame_range[1])) * 0.5))
        render_still(scene, output_dir / "gesture_stills" / f"{action_name}.png")

    print(f"VISEME_STILLS={output_dir / 'viseme_stills'}")
    print(f"EXPRESSION_STILLS={output_dir / 'expression_stills'}")
    print(f"GESTURE_STILLS={output_dir / 'gesture_stills'}")


if __name__ == "__main__":
    main()
