#!/usr/bin/env python3
"""Blender-side RED contract for close-shot main-IP hand refinement."""

from __future__ import annotations

import sys
import unittest
import re
from pathlib import Path

try:
    import bpy
except ModuleNotFoundError as exc:
    raise unittest.SkipTest("requires Blender's bpy runtime") from exc

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import blender_renderer
import hand_refinement
from test_blender_character_rig import load_enhanced_fbx_character


SUPPORT_OFFSET = 0.045
SUPPORT_BAND_TOLERANCE_RATIO = 0.02
MINIMUM_SUPPORT_BAND_EDGES = 3
MINIMUM_RING_VERTICES = 4
MINIMUM_RING_SPAN_RATIO = 0.015
MINIMUM_RING_AREA_RATIO = 0.000025


def digit_roles(side: str, digit: int) -> tuple[str, str, str]:
    return (
        f"finger_{digit}_{side}",
        f"finger_{digit}_mid_{side}",
        f"finger_{digit}_tip_{side}",
    )


def expected_digit_bone_names(side: str, digit: int) -> tuple[str, str, str]:
    suffix = side.upper()
    return tuple(
        f"Finger_{digit:02d}_{segment}.{suffix}"
        for segment in ("Proximal", "Middle", "Distal")
    )


def is_finger_deform_bone_name(name: str) -> bool:
    normalized = re.sub(r"[^a-z0-9]+", "", name.lower())
    return re.fullmatch(r"finger\d+(?:proximal|middle|distal)[lr]", normalized) is not None


def hand_relative_tip(armature, bone_map, side: str, digit: int):
    hand = armature.pose.bones[bone_map[f"hand_{side}"]]
    distal = armature.pose.bones[bone_map[f"finger_{digit}_tip_{side}"]]
    armature_space = hand.matrix.inverted() @ distal.tail
    return armature.matrix_world.to_3x3() @ armature_space


def digit_chain_length(armature, bone_map, side: str, digit: int) -> float:
    roles = digit_roles(side, digit)
    missing = [role for role in roles if role not in bone_map]
    if missing:
        raise ValueError(f"{side} digit {digit} is missing required chain roles: {', '.join(missing)}")
    world_scale = armature.matrix_world.to_3x3()
    length = 0.0
    for role in roles:
        bone = armature.data.bones[bone_map[role]]
        length += (world_scale @ (bone.tail_local - bone.head_local)).length
    return length


def sample_action_tips(armature, bone_map, action_name: str, frame: int = 30):
    blender_renderer.create_action_library(armature, {}, bone_map, fps=30)
    armature.animation_data.action = bpy.data.actions[action_name]
    bpy.context.scene.frame_set(frame)
    return {
        (side, digit): hand_relative_tip(armature, bone_map, side, digit).copy()
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
        digit: hand_relative_tip(armature, bone_map, side, digit).copy()
        for digit in (1, 2, 3)
    }
    bpy.context.scene.frame_set({1: 15, 2: 30, 3: 45}[selected])
    after = {
        digit: hand_relative_tip(armature, bone_map, side, digit).copy()
        for digit in (1, 2, 3)
    }
    return before, after


def sampled_digit_rotations(armature, bone_map, action_name: str, side: str, digit: int, frame: int = 30):
    blender_renderer.create_action_library(armature, {}, bone_map, fps=30)
    armature.animation_data.action = bpy.data.actions[action_name]
    bpy.context.scene.frame_set(frame)
    return tuple(
        armature.pose.bones[bone_map[role]].rotation_euler.copy()
        for role in digit_roles(side, digit)
    )


def _deform_assignments(obj, vertex, armature, group_names=None):
    group_names = group_names or {group.index: group.name for group in obj.vertex_groups}
    for assignment in vertex.groups:
        group_name = group_names.get(assignment.group)
        bone = armature.data.bones.get(group_name) if group_name else None
        if bone and bone.use_deform:
            yield group_name, float(assignment.weight)


def _weight_violations(objects, armature) -> list[str]:
    violations: list[str] = []
    for obj in objects:
        if obj.type != "MESH":
            continue
        group_names = {group.index: group.name for group in obj.vertex_groups}
        non_positive_by_bone: dict[str, int] = {}
        non_positive_vertices: set[int] = set()
        for vertex in obj.data.vertices:
            assignments = list(_deform_assignments(obj, vertex, armature, group_names))
            positive = [(name, weight) for name, weight in assignments if weight > 0.0]
            non_positive = [name for name, weight in assignments if weight <= 0.0]
            if not positive:
                violations.append(f"{obj.name} vertex {vertex.index} has no positive deform-bone assignment")
            if non_positive:
                non_positive_vertices.add(vertex.index)
                for name in non_positive:
                    non_positive_by_bone[name] = non_positive_by_bone.get(name, 0) + 1
            if len(assignments) > 4:
                violations.append(
                    f"{obj.name} vertex {vertex.index} has {len(assignments)} deform influences, "
                    "expected at most 4"
                )
            total = sum(weight for _, weight in assignments)
            if abs(total - 1.0) > 1e-4:
                violations.append(
                    f"{obj.name} vertex {vertex.index} weights sum to {total:.6f}, expected 1.0"
                )
        if non_positive_by_bone:
            counts = ", ".join(
                f"{name}={count}" for name, count in sorted(non_positive_by_bone.items())
            )
            violations.append(
                f"{obj.name} has {sum(non_positive_by_bone.values())} non-positive deform assignments "
                f"across {len(non_positive_vertices)} vertices: {counts}"
            )
    return violations


def _support_band_ring_edges(
    objects,
    armature,
    chain_names,
    plane_point,
    axis,
    tolerance: float,
    chain_length: float,
) -> int:
    largest_ring = 0
    for obj in objects:
        if obj.type != "MESH":
            continue
        group_names = {group.index: group.name for group in obj.vertex_groups}
        near_plane = set()
        world_coordinates = {}
        for vertex in obj.data.vertices:
            digit_weight = sum(
                weight
                for bone_name, weight in _deform_assignments(obj, vertex, armature, group_names)
                if bone_name in chain_names
            )
            world = obj.matrix_world @ vertex.co
            if digit_weight > 1e-5 and abs((world - plane_point).dot(axis)) <= tolerance:
                near_plane.add(vertex.index)
                world_coordinates[vertex.index] = world
        adjacency: dict[int, set[int]] = {}
        for edge in obj.data.edges:
            first, second = edge.vertices
            if first not in near_plane or second not in near_plane:
                continue
            adjacency.setdefault(first, set()).add(second)
            adjacency.setdefault(second, set()).add(first)
        visited: set[int] = set()
        for start in adjacency:
            if start in visited:
                continue
            stack = [start]
            component_vertices: set[int] = set()
            while stack:
                current = stack.pop()
                if current in component_vertices:
                    continue
                component_vertices.add(current)
                visited.add(current)
                stack.extend(adjacency[current] - component_vertices)
            component_edges = sum(len(adjacency[vertex]) for vertex in component_vertices) // 2
            if len(component_vertices) < MINIMUM_RING_VERTICES:
                continue
            if component_edges != len(component_vertices):
                continue
            if any(len(adjacency[vertex]) != 2 for vertex in component_vertices):
                continue

            radial = [
                world_coordinates[vertex]
                - plane_point
                - axis * (world_coordinates[vertex] - plane_point).dot(axis)
                for vertex in component_vertices
            ]
            max_span = max(
                (left - right).length
                for index, left in enumerate(radial)
                for right in radial[index + 1 :]
            )
            max_area = max(
                abs(axis.dot(left.cross(right)))
                for index, left in enumerate(radial)
                for right in radial[index + 1 :]
            )
            if max_span < chain_length * MINIMUM_RING_SPAN_RATIO:
                continue
            if max_area < chain_length * chain_length * MINIMUM_RING_AREA_RATIO:
                continue
            largest_ring = max(largest_ring, component_edges)
    return largest_ring


def _mesh_support_band_evidence(objects, armature, bone_map) -> tuple[int, list[str]]:
    support_bands = 0
    violations: list[str] = []
    for side in ("l", "r"):
        for digit in (1, 2, 3):
            roles = digit_roles(side, digit)
            missing = [role for role in roles if role not in bone_map]
            if missing:
                violations.append(
                    f"{side} digit {digit} has no measurable joint support bands; missing {', '.join(missing)}"
                )
                continue
            proximal, middle, distal = (armature.data.bones[bone_map[role]] for role in roles)
            base = armature.matrix_world @ proximal.head_local
            tip = armature.matrix_world @ distal.tail_local
            chain_length = (tip - base).length
            if chain_length <= 1e-5:
                violations.append(f"{side} digit {digit} has a zero-length chain")
                continue
            axis = (tip - base).normalized()
            chain_names = {bone.name for bone in (proximal, middle, distal)}
            tolerance = chain_length * SUPPORT_BAND_TOLERANCE_RATIO
            joints = (armature.matrix_world @ proximal.tail_local, armature.matrix_world @ middle.tail_local)
            for joint_index, joint in enumerate(joints, start=1):
                for offset in (-SUPPORT_OFFSET, SUPPORT_OFFSET):
                    plane_point = joint + axis * chain_length * offset
                    ring_edges = _support_band_ring_edges(
                        objects,
                        armature,
                        chain_names,
                        plane_point,
                        axis,
                        tolerance,
                        chain_length,
                    )
                    if ring_edges >= MINIMUM_SUPPORT_BAND_EDGES:
                        support_bands += 1
                    else:
                        violations.append(
                            f"{side} digit {digit} joint {joint_index} support offset {offset:+.3f} "
                            f"has no closed weighted mesh ring with at least "
                            f"{MINIMUM_SUPPORT_BAND_EDGES} edges"
                        )
    return support_bands, violations


def test_main_ip_has_three_segments_per_digit_and_clean_weights() -> None:
    objects, _, armature, stats, bone_map, _ = load_enhanced_fbx_character()
    violations: list[str] = []

    expected_bone_names = {
        bone_name
        for side in ("l", "r")
        for digit in (1, 2, 3)
        for bone_name in expected_digit_bone_names(side, digit)
    }
    actual_finger_deform_bones = {
        bone.name
        for bone in armature.data.bones
        if bone.use_deform and is_finger_deform_bone_name(bone.name)
    }
    if actual_finger_deform_bones != expected_bone_names:
        violations.append(
            f"armature finger deform set has {len(actual_finger_deform_bones)} bones, expected exactly 18"
        )
    missing_deform_bones = sorted(expected_bone_names - actual_finger_deform_bones)
    if missing_deform_bones:
        violations.append(f"missing finger deform bones: {', '.join(missing_deform_bones)}")
    unexpected_deform_bones = sorted(actual_finger_deform_bones - expected_bone_names)
    if unexpected_deform_bones:
        violations.append(f"unexpected finger deform bones: {', '.join(unexpected_deform_bones)}")
    if stats.get("fingerBoneCount") != 18:
        violations.append(f"reported fingerBoneCount={stats.get('fingerBoneCount')!r}, expected 18")
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
            expected_names = expected_digit_bone_names(side, digit)
            if tuple(bone.name for bone in (proximal, middle, distal)) != expected_names:
                violations.append(
                    f"{side} digit {digit} resolves to "
                    f"{tuple(bone.name for bone in (proximal, middle, distal))}, expected {expected_names}"
                )
            if not all(bone.use_deform for bone in (proximal, middle, distal)):
                violations.append(f"{side} digit {digit} chain bones must all use deform")
            if middle.parent != proximal or distal.parent != middle:
                violations.append(f"{side} digit {digit} is not a proximal-middle-distal parent chain")
            if not middle.use_connect or not distal.use_connect:
                violations.append(f"{side} digit {digit} middle and distal bones must be connected")
            if (middle.head_local - proximal.tail_local).length > 1e-5:
                violations.append(f"{side} digit {digit} has a gap at the proximal-middle joint")
            if (distal.head_local - middle.tail_local).length > 1e-5:
                violations.append(f"{side} digit {digit} has a gap at the middle-distal joint")

    support_band_count, support_band_violations = _mesh_support_band_evidence(objects, armature, bone_map)
    if support_band_count < 24:
        violations.append(
            f"actual mesh has {support_band_count} digit-joint support bands, expected at least 24"
        )
    violations.extend(support_band_violations)
    violations.extend(_weight_violations(objects, armature))
    assert not violations, "\n".join(violations)


def test_validated_three_segment_reuse_requires_contract_marker() -> None:
    objects, dimensions, armature, _, bone_map, _ = load_enhanced_fbx_character()
    before = sum(len(obj.data.vertices) for obj in objects if obj.type == "MESH")
    reused_stats, reused_map = blender_renderer.enhance_existing_presenter_rig(
        armature, objects, dimensions, {}, bone_map
    )
    assert reused_stats["fingerRigReused"] is True
    assert reused_stats["fingerBoneCount"] == 18
    assert reused_stats["fingerSegmentCount"] == 3
    assert reused_map == bone_map
    assert sum(len(obj.data.vertices) for obj in objects if obj.type == "MESH") == before

    target = next(
        (
            (obj, vertex, assignment)
            for obj in objects if obj.type == "MESH"
            for vertex in obj.data.vertices
            for assignment in vertex.groups
            if armature.data.bones.get(obj.vertex_groups[assignment.group].name)
            and armature.data.bones[obj.vertex_groups[assignment.group].name].use_deform
            and assignment.weight > 0.1
        ),
        None,
    )
    assert target is not None
    obj, vertex, assignment = target
    group = obj.vertex_groups[assignment.group]
    original_weight = float(assignment.weight)
    group.add([vertex.index], original_weight * 0.5, "REPLACE")
    try:
        blender_renderer.enhance_existing_presenter_rig(armature, objects, dimensions, {}, bone_map)
    except RuntimeError as exc:
        assert "deform weights sum" in str(exc)
    else:
        raise AssertionError("unnormalized marked three-segment rigs must fail closed")
    group.add([vertex.index], original_weight, "REPLACE")

    del armature[hand_refinement.HAND_CONTRACT_KEY]
    try:
        blender_renderer.enhance_existing_presenter_rig(armature, objects, dimensions, {}, bone_map)
    except RuntimeError as exc:
        assert "unvalidated three-segment hand rig" in str(exc)
    else:
        raise AssertionError("unmarked three-segment rigs must fail closed")


def test_each_digit_moves_independently_and_fist_closes() -> None:
    _, _, armature, _, bone_map, _ = load_enhanced_fbx_character()
    violations: list[str] = []
    open_tips = sample_open_tips(armature, bone_map)
    fist_tips = sample_fist_tips(armature, bone_map)
    for side in ("l", "r"):
        for digit in (1, 2, 3):
            try:
                rest_length = digit_chain_length(armature, bone_map, side, digit)
            except ValueError as exc:
                violations.append(str(exc))
                proximal_name = bone_map.get(f"finger_{digit}_{side}")
                distal_name = bone_map.get(f"finger_{digit}_tip_{side}")
                if not proximal_name or not distal_name:
                    continue
                # This explicit legacy baseline preserves the motion RED evidence until the required chain exists.
                rest_length = (
                    armature.data.bones[proximal_name].length + armature.data.bones[distal_name].length
                )
            displacement = (fist_tips[side, digit] - open_tips[side, digit]).length
            minimum = rest_length * 0.25
            if displacement < minimum:
                violations.append(
                    f"{side} digit {digit} fist tip displacement {displacement:.4f} is below {minimum:.4f}"
                )

    for selected in (1, 2, 3):
        before, after = sample_single_digit_curl(armature, bone_map, "r", selected)
        try:
            selected_length = digit_chain_length(armature, bone_map, "r", selected)
        except ValueError as exc:
            violations.append(str(exc))
            selected_length = (
                armature.data.bones[bone_map[f"finger_{selected}_r"]].length
                + armature.data.bones[bone_map[f"finger_{selected}_tip_r"]].length
            )
        selected_displacement = (after[selected] - before[selected]).length
        selected_minimum = selected_length * 0.18
        if selected_displacement < selected_minimum:
            violations.append(
                f"right digit {selected} curl displacement {selected_displacement:.4f} "
                f"is below {selected_minimum:.4f}"
            )
        for other in {1, 2, 3} - {selected}:
            try:
                other_length = digit_chain_length(armature, bone_map, "r", other)
            except ValueError as exc:
                violations.append(str(exc))
                other_length = (
                    armature.data.bones[bone_map[f"finger_{other}_r"]].length
                    + armature.data.bones[bone_map[f"finger_{other}_tip_r"]].length
                )
            other_displacement = (after[other] - before[other]).length
            other_maximum = other_length * 0.06
            if other_displacement > other_maximum:
                violations.append(
                    f"right digit {selected} also moves digit {other} by {other_displacement:.4f}, "
                    f"above {other_maximum:.4f}"
                )

    assert not violations, "\n".join(violations)


def test_three_segment_digit_poses_key_independent_semantic_curls() -> None:
    _, _, armature, _, bone_map, _ = load_enhanced_fbx_character()

    for digit in (1, 2, 3):
        proximal, middle, distal = sampled_digit_rotations(
            armature, bone_map, "Gesture_Fist", "r", digit
        )
        assert proximal.z > 0.34, (digit, tuple(proximal))
        assert middle.z > 0.42, (digit, tuple(middle))
        assert distal.z > 0.28, (digit, tuple(distal))

    open_digits = [
        sampled_digit_rotations(armature, bone_map, "Gesture_OpenHand", "r", digit)
        for digit in (1, 2, 3)
    ]
    assert all(abs(rotation.z) < 0.04 for chain in open_digits for rotation in chain)
    assert open_digits[0][0].x > 0.10, tuple(open_digits[0][0])
    assert open_digits[2][0].x < -0.10, tuple(open_digits[2][0])

    pinch_lower = sampled_digit_rotations(armature, bone_map, "Gesture_Pinch", "r", 3)
    assert abs(pinch_lower[0].y) > 0.04, tuple(pinch_lower[0])

    for selected, frame in ((1, 15), (2, 30), (3, 45)):
        proximal, middle, distal = sampled_digit_rotations(
            armature, bone_map, "Gesture_FingerWave", "r", selected, frame
        )
        assert proximal.z > 0.34, (selected, tuple(proximal))
        assert middle.z > 0.42, (selected, tuple(middle))
        assert distal.z > 0.28, (selected, tuple(distal))


def test_finger_wave_ends_at_its_shared_open_hand_pose() -> None:
    _, _, armature, _, bone_map, _ = load_enhanced_fbx_character()
    blender_renderer.create_action_library(armature, {}, bone_map, fps=30)
    armature.animation_data.action = bpy.data.actions["Gesture_FingerWave"]

    bpy.context.scene.frame_set(1)
    start_rotations = {
        digit: tuple(
            armature.pose.bones[bone_map[role]].rotation_euler.copy()
            for role in digit_roles("r", digit)
        )
        for digit in (1, 2, 3)
    }
    start_tips = {
        digit: hand_relative_tip(armature, bone_map, "r", digit).copy()
        for digit in (1, 2, 3)
    }

    bpy.context.scene.frame_set(60)
    for digit in (1, 2, 3):
        end_rotations = tuple(
            armature.pose.bones[bone_map[role]].rotation_euler.copy()
            for role in digit_roles("r", digit)
        )
        for start, end in zip(start_rotations[digit], end_rotations):
            assert max(abs(end[index] - start[index]) for index in range(3)) < 1e-6, (
                digit,
                tuple(start),
                tuple(end),
            )
        drift = (hand_relative_tip(armature, bone_map, "r", digit) - start_tips[digit]).length
        assert drift < 1e-6, (digit, drift)


if __name__ == "__main__":
    tests = [
        test_main_ip_has_three_segments_per_digit_and_clean_weights,
        test_validated_three_segment_reuse_requires_contract_marker,
        test_each_digit_moves_independently_and_fist_closes,
        test_three_segment_digit_poses_key_independent_semantic_curls,
        test_finger_wave_ends_at_its_shared_open_hand_pose,
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
