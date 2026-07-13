#!/usr/bin/env python3
"""Blender-side RED contract for close-shot main-IP hand refinement."""

from __future__ import annotations

import sys
import unittest
from pathlib import Path

try:
    import bpy
except ModuleNotFoundError as exc:
    raise unittest.SkipTest("requires Blender's bpy runtime") from exc

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import blender_renderer
from test_blender_character_rig import load_enhanced_fbx_character


def digit_roles(side: str, digit: int) -> tuple[str, str, str]:
    return (
        f"finger_{digit}_{side}",
        f"finger_{digit}_mid_{side}",
        f"finger_{digit}_tip_{side}",
    )


def world_tip(armature, bone_map, side: str, digit: int):
    distal = armature.pose.bones[bone_map[f"finger_{digit}_tip_{side}"]]
    return armature.matrix_world @ distal.tail


def digit_chain_length(armature, bone_map, side: str, digit: int) -> float:
    return sum(
        armature.data.bones[bone_map[role]].length
        for role in digit_roles(side, digit)
        if role in bone_map
    )


def sample_action_tips(armature, bone_map, action_name: str, frame: int = 30):
    blender_renderer.create_action_library(armature, {}, bone_map, fps=30)
    armature.animation_data.action = bpy.data.actions[action_name]
    bpy.context.scene.frame_set(frame)
    return {
        (side, digit): world_tip(armature, bone_map, side, digit).copy()
        for side in ("l", "r")
        for digit in (1, 2, 3)
    }


def sample_open_tips(armature, bone_map):
    return sample_action_tips(armature, bone_map, "Gesture_OpenHand")


def sample_fist_tips(armature, bone_map):
    return sample_action_tips(armature, bone_map, "Gesture_Fist")


def sample_single_digit_curl(armature, bone_map, side: str, selected: int):
    blender_renderer.create_action_library(armature, {}, bone_map, fps=30)
    armature.animation_data.action = bpy.data.actions["Gesture_FingerWave"]
    bpy.context.scene.frame_set(1)
    before = {
        digit: world_tip(armature, bone_map, side, digit).copy()
        for digit in (1, 2, 3)
    }
    bpy.context.scene.frame_set({1: 15, 2: 30, 3: 45}[selected])
    after = {
        digit: world_tip(armature, bone_map, side, digit).copy()
        for digit in (1, 2, 3)
    }
    return before, after


def _weight_violations(objects) -> list[str]:
    violations: list[str] = []
    for obj in objects:
        if obj.type != "MESH":
            continue
        for vertex in obj.data.vertices:
            positive = [assignment for assignment in vertex.groups if assignment.weight > 1e-5]
            total = sum(assignment.weight for assignment in positive)
            if not positive:
                violations.append(f"{obj.name} vertex {vertex.index} has no deform weights")
            elif len(positive) > 4:
                violations.append(
                    f"{obj.name} vertex {vertex.index} has {len(positive)} influences, expected at most 4"
                )
            elif abs(total - 1.0) > 1e-4:
                violations.append(
                    f"{obj.name} vertex {vertex.index} weights sum to {total:.6f}, expected 1.0"
                )
    return violations


def test_main_ip_has_three_segments_per_digit_and_clean_weights() -> None:
    objects, _, armature, stats, bone_map, _ = load_enhanced_fbx_character()
    violations: list[str] = []

    if stats.get("fingerBoneCount") != 18:
        violations.append(f"fingerBoneCount={stats.get('fingerBoneCount')!r}, expected 18")
    if stats.get("fingerSegmentCount") != 3:
        violations.append(f"fingerSegmentCount={stats.get('fingerSegmentCount')!r}, expected 3")
    if stats.get("handJointSupportLoopCount", 0) < 24:
        violations.append(
            "handJointSupportLoopCount="
            f"{stats.get('handJointSupportLoopCount')!r}, expected at least 24"
        )
    if stats.get("maxVertexInfluences", float("inf")) > 4:
        violations.append(f"maxVertexInfluences={stats.get('maxVertexInfluences')!r}, expected at most 4")
    if stats.get("unweightedVertexCount") != 0:
        violations.append(f"unweightedVertexCount={stats.get('unweightedVertexCount')!r}, expected 0")

    bones = armature.data.bones
    for side in ("l", "r"):
        for digit in (1, 2, 3):
            roles = digit_roles(side, digit)
            missing = [role for role in roles if role not in bone_map]
            if missing:
                violations.append(f"{side} digit {digit} is missing roles: {', '.join(missing)}")
                continue
            proximal, middle, distal = (bones[bone_map[role]] for role in roles)
            if middle.parent != proximal or distal.parent != middle:
                violations.append(f"{side} digit {digit} is not a proximal-middle-distal parent chain")
            if not middle.use_connect or not distal.use_connect:
                violations.append(f"{side} digit {digit} middle and distal bones must be connected")
            if (middle.head_local - proximal.tail_local).length > 1e-5:
                violations.append(f"{side} digit {digit} has a gap at the proximal-middle joint")
            if (distal.head_local - middle.tail_local).length > 1e-5:
                violations.append(f"{side} digit {digit} has a gap at the middle-distal joint")

    violations.extend(_weight_violations(objects))
    assert not violations, "\n".join(violations)


def test_each_digit_moves_independently_and_fist_closes() -> None:
    _, _, armature, _, bone_map, _ = load_enhanced_fbx_character()
    violations: list[str] = []
    open_tips = sample_open_tips(armature, bone_map)
    fist_tips = sample_fist_tips(armature, bone_map)
    for side in ("l", "r"):
        for digit in (1, 2, 3):
            rest_length = digit_chain_length(armature, bone_map, side, digit)
            displacement = (fist_tips[side, digit] - open_tips[side, digit]).length
            minimum = rest_length * 0.25
            if displacement < minimum:
                violations.append(
                    f"{side} digit {digit} fist tip displacement {displacement:.4f} is below {minimum:.4f}"
                )

    for selected in (1, 2, 3):
        before, after = sample_single_digit_curl(armature, bone_map, "r", selected)
        selected_length = digit_chain_length(armature, bone_map, "r", selected)
        selected_displacement = (after[selected] - before[selected]).length
        selected_minimum = selected_length * 0.18
        if selected_displacement < selected_minimum:
            violations.append(
                f"right digit {selected} curl displacement {selected_displacement:.4f} "
                f"is below {selected_minimum:.4f}"
            )
        for other in {1, 2, 3} - {selected}:
            other_length = digit_chain_length(armature, bone_map, "r", other)
            other_displacement = (after[other] - before[other]).length
            other_maximum = other_length * 0.06
            if other_displacement > other_maximum:
                violations.append(
                    f"right digit {selected} also moves digit {other} by {other_displacement:.4f}, "
                    f"above {other_maximum:.4f}"
                )

    assert not violations, "\n".join(violations)


if __name__ == "__main__":
    tests = [
        test_main_ip_has_three_segments_per_digit_and_clean_weights,
        test_each_digit_moves_independently_and_fist_closes,
    ]
    failures: list[tuple[str, AssertionError]] = []
    for test in tests:
        try:
            test()
            print(f"PASS {test.__name__}")
        except AssertionError as exc:
            failures.append((test.__name__, exc))
            print(f"FAIL {test.__name__}: {exc}")
    if failures:
        print(f"FAIL {len(failures)} focused hand-refinement test(s) failed")
        sys.exit(1)
