#!/usr/bin/env python3
"""Reapply a motion plan to an existing Blend and render a bounded frame range."""

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
    if len(args) != 3:
        raise RuntimeError(
            "usage: blender existing.blend --python rerender_timeline_range.py -- "
            "render_input.json frame_start frame_end"
        )

    input_path = Path(args[0]).expanduser().resolve()
    frame_start = max(1, int(args[1]))
    frame_end = max(frame_start, int(args[2]))
    data = json.loads(input_path.read_text(encoding="utf-8"))
    motion_plan = data.get("motionPlan") or {}
    fps = max(1, int(data.get("fps") or motion_plan.get("fps") or 30))

    scene = bpy.context.scene
    armature = max(
        (obj for obj in scene.objects if obj.type == "ARMATURE"),
        key=lambda obj: len(obj.data.bones),
    )
    encoded_map = armature.get("ip_avatar_bone_map")
    bone_map = json.loads(encoded_map) if encoded_map else resolve_bone_roles(
        bone.name for bone in armature.data.bones
    )
    meshes = [obj for obj in scene.objects if obj.type == "MESH"]
    mouth = blender_renderer.find_existing_viseme_mouth(meshes)
    face = {"mouth": mouth} if mouth else {}

    blender_renderer.animate(armature, face, motion_plan, fps=fps, bone_map=bone_map)
    blender_renderer.apply_lighting_preset(str(data.get("lightingPreset") or "editorial_soft"))

    duration = max(1.0 / fps, float(data.get("durationSec") or motion_plan.get("durationSec") or 1.0))
    full_frame_end = max(1, int(duration * fps))
    scene.render.fps = fps
    scene.render.fps_base = 1.0
    scene.frame_start = 1
    scene.frame_end = full_frame_end
    scene.render.resolution_x = int((data.get("resolution") or {}).get("width") or scene.render.resolution_x)
    scene.render.resolution_y = int((data.get("resolution") or {}).get("height") or scene.render.resolution_y)
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"
    frames_dir = Path(data["framesDir"]).expanduser().resolve()
    frames_dir.mkdir(parents=True, exist_ok=True)
    scene.render.filepath = str(frames_dir / "frame_")

    blend_path = Path(data.get("riggedBlendPath") or bpy.data.filepath).expanduser().resolve()
    bpy.ops.wm.save_as_mainfile(filepath=str(blend_path))
    scene.frame_start = frame_start
    scene.frame_end = min(frame_end, full_frame_end)
    bpy.ops.render.render(animation=True)
    print(
        "RERENDERED_TIMELINE_RANGE="
        + json.dumps(
            {
                "blendPath": str(blend_path),
                "framesDir": str(frames_dir),
                "frameStart": scene.frame_start,
                "frameEnd": scene.frame_end,
            },
            ensure_ascii=False,
        )
    )


if __name__ == "__main__":
    main()
