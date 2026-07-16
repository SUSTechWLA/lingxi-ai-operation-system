#!/usr/bin/env python3
"""Blender-side RED contract for close-shot main-IP hand refinement."""

from __future__ import annotations

import json
import re
import sys
import tempfile
import unittest
from pathlib import Path

try:
    import bpy
    from mathutils import Vector
except ModuleNotFoundError as exc:
    raise unittest.SkipTest("requires Blender's bpy runtime") from exc

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import blender_renderer
import hand_refinement
import master_asset
import render_aroll_master_qa
from test_blender_character_rig import RIGGED_FBX_PATH, load_enhanced_fbx_character


SUPPORT_OFFSET = 0.045
SUPPORT_BAND_TOLERANCE_RATIO = 0.02
MINIMUM_SUPPORT_BAND_EDGES = 3
MINIMUM_RING_VERTICES = 4
MINIMUM_RING_SPAN_RATIO = 0.015
MINIMUM_RING_AREA_RATIO = 0.000025
AROLL_ACTIONS = {
    "Aroll_Seated_Idle",
    "Aroll_Idle_Listening",
    "Aroll_Greeting_Wave",
    "Aroll_OpenPalm_Explain",
    "Aroll_Explain_Left",
    "Aroll_Explain_Right",
    "Aroll_Count_One",
    "Aroll_Count_Two",
    "Aroll_Count_Three",
    "Aroll_Point_Left",
    "Aroll_Point_Right",
    "Aroll_Pinch_Detail",
    "Aroll_Emphasis_SoftFist",
    "Aroll_Think",
    "Aroll_Agree_Nod",
    "Aroll_Disagree_Shake",
    "Aroll_Transition_Reset",
}
HAND_AESTHETIC_VERSION_KEY = "ip_avatar_hand_aesthetic_version"
HAND_AESTHETIC_REPORT_KEY = "ip_avatar_hand_aesthetic_report"


def test_aroll_qa_sample_contract_is_complete_and_squint_only() -> None:
    samples = render_aroll_master_qa.QA_SAMPLES
    action_samples = [sample for sample in samples if sample.kind == "action"]
    hand_samples = [sample for sample in samples if sample.kind == "hand"]
    digit_samples = [sample for sample in samples if sample.kind == "digit"]
    face_samples = [sample for sample in samples if sample.kind == "face"]

    assert {(sample.action, sample.camera) for sample in action_samples} == {
        (action, camera)
        for action in AROLL_ACTIONS
        for camera in render_aroll_master_qa.ACTION_CAMERAS
    }
    assert {sample.path for sample in hand_samples} == {
        "hand/open.png",
        "hand/fist.png",
        "hand/pinch.png",
        "hand/count_1.png",
        "hand/count_2.png",
        "hand/count_3.png",
    }
    assert {(sample.side, sample.digit) for sample in digit_samples} == {
        (side, digit) for side in ("l", "r") for digit in (1, 2, 3)
    }
    assert {sample.path for sample in face_samples} == {
        "face/neutral.png",
        "face/happy.png",
        "face/serious.png",
        "face/squint.png",
        "face/A.png",
        "face/E.png",
        "face/O.png",
        "face/MBP.png",
    }
    assert not any("blink" in sample.path.lower() for sample in samples)
    assert render_aroll_master_qa.FACE_CAPABILITY == "squint_only"
    assert len(samples) == 54


def _fixture_look_at(obj, target) -> None:
    obj.rotation_euler = (target - obj.location).to_track_quat("-Z", "Y").to_euler()


def _fixture_weight_object(obj, armature, bone_name: str) -> None:
    group = obj.vertex_groups.new(name=bone_name)
    group.add([vertex.index for vertex in obj.data.vertices], 1.0, "REPLACE")
    modifier = obj.modifiers.new("QA_Armature", "ARMATURE")
    modifier.object = armature


def _fixture_add_box(name, location, scale, armature, bone_name, material):
    bpy.ops.mesh.primitive_cube_add(size=1.0, location=location)
    obj = bpy.context.object
    obj.name = name
    obj.scale = scale
    bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)
    obj.data.materials.append(material)
    _fixture_weight_object(obj, armature, bone_name)
    return obj


def _fixture_add_bone_segment(name, armature, material, radius: float = 0.035):
    bone = armature.data.bones[name]
    direction = bone.tail_local - bone.head_local
    midpoint = (bone.head_local + bone.tail_local) * 0.5
    bpy.ops.mesh.primitive_cylinder_add(
        vertices=8,
        radius=radius,
        depth=direction.length,
        location=midpoint,
    )
    obj = bpy.context.object
    obj.name = f"QA_Mesh_{name}"
    obj.rotation_euler = direction.to_track_quat("Z", "Y").to_euler()
    obj.data.materials.append(material)
    _fixture_weight_object(obj, armature, name)
    return obj


def _fixture_add_face_shape_keys(face) -> None:
    face.shape_key_add(name="Basis", from_mix=False)
    shape_names = (
        "Mouth_Rest",
        "Mouth_A",
        "Mouth_E",
        "Mouth_O",
        "Mouth_MBP",
        "Mouth_Smile",
        "Mouth_Frown",
        "Eye_Squint.L",
        "Eye_Squint.R",
    )
    for shape_name in shape_names:
        key = face.shape_key_add(name=shape_name, from_mix=False)
        for vertex in key.data:
            x, _, z = vertex.co
            if shape_name == "Mouth_A" and z < 0.0:
                vertex.co.z -= 0.13
            elif shape_name == "Mouth_E":
                vertex.co.x = x * 1.24
            elif shape_name == "Mouth_O":
                vertex.co.x = x * 0.72
                vertex.co.z = z * 1.12
            elif shape_name == "Mouth_MBP":
                vertex.co.z += 0.055
            elif shape_name == "Mouth_Smile":
                vertex.co.x = x * 1.14
                vertex.co.z += 0.045
            elif shape_name == "Mouth_Frown":
                vertex.co.x = x * 0.90
                vertex.co.z -= 0.065
            elif shape_name == "Eye_Squint.L" and x < 0.0 and z > 0.0:
                vertex.co.z -= 0.11
            elif shape_name == "Eye_Squint.R" and x > 0.0 and z > 0.0:
                vertex.co.z -= 0.11
    face["blink_capability"] = "squint_only"


def _build_aroll_qa_fixture():
    bpy.ops.wm.read_factory_settings(use_empty=True)
    scene = bpy.context.scene
    scene.render.engine = "BLENDER_EEVEE"
    scene.world = bpy.data.worlds.new("QA_Aroll_World")
    scene.world.color = (0.025, 0.025, 0.025)

    armature_data = bpy.data.armatures.new("QA_Aroll_Rig_Data")
    armature = bpy.data.objects.new("QA_Aroll_Rig", armature_data)
    scene.collection.objects.link(armature)
    bpy.context.view_layer.objects.active = armature
    armature.select_set(True)
    bpy.ops.object.mode_set(mode="EDIT")

    def add_bone(name, head, tail, parent=None, connected=False):
        bone = armature_data.edit_bones.new(name)
        bone.head = head
        bone.tail = tail
        if parent:
            bone.parent = armature_data.edit_bones[parent]
            bone.use_connect = connected
        return bone

    add_bone("Root", (0.0, 0.0, 0.0), (0.0, 0.0, 0.2))
    add_bone("Body", (0.0, 0.0, 0.2), (0.0, 0.0, 1.45), "Root", True)
    add_bone("Head", (0.0, 0.0, 1.45), (0.0, 0.0, 2.0), "Body", True)

    bone_map = {"root": "Root", "body": "Body", "head": "Head"}
    for side, sign in (("l", 1.0), ("r", -1.0)):
        suffix = side.upper()
        hand_name = f"Hand.{suffix}"
        hand_head = (sign * 0.42, 0.0, 1.14)
        hand_tail = (sign * 0.60, 0.0, 1.14)
        add_bone(hand_name, hand_head, hand_tail, "Body")
        bone_map[f"hand_{side}"] = hand_name
        for digit in (1, 2, 3):
            z = 1.14 + (2 - digit) * 0.13
            base = sign * 0.60
            points = [base + sign * 0.12 * index for index in range(4)]
            names = (
                f"Finger_{digit:02d}_Proximal.{suffix}",
                f"Finger_{digit:02d}_Middle.{suffix}",
                f"Finger_{digit:02d}_Distal.{suffix}",
            )
            add_bone(names[0], (points[0], 0.0, z), (points[1], 0.0, z), hand_name)
            add_bone(names[1], (points[1], 0.0, z), (points[2], 0.0, z), names[0], True)
            add_bone(names[2], (points[2], 0.0, z), (points[3], 0.0, z), names[1], True)
            roles = (f"finger_{digit}_{side}", f"finger_{digit}_mid_{side}", f"finger_{digit}_tip_{side}")
            bone_map.update(dict(zip(roles, names)))
    bpy.ops.object.mode_set(mode="OBJECT")
    armature["ip_avatar_bone_map"] = json.dumps(bone_map, sort_keys=True)
    armature["ip_avatar_generated_humanoid"] = True

    material = bpy.data.materials.new("QA_Aroll_Material")
    material.diffuse_color = (0.36, 0.74, 0.52, 1.0)
    _fixture_add_box("QA_Body", (0.0, 0.0, 0.82), (0.38, 0.18, 0.61), armature, "Body", material)
    for side, sign in (("l", 1.0), ("r", -1.0)):
        _fixture_add_box(
            f"QA_Palm_{side.upper()}",
            (sign * 0.51, 0.0, 1.14),
            (0.12, 0.075, 0.18),
            armature,
            bone_map[f"hand_{side}"],
            material,
        )
        for digit in (1, 2, 3):
            for role in (f"finger_{digit}_{side}", f"finger_{digit}_mid_{side}", f"finger_{digit}_tip_{side}"):
                _fixture_add_bone_segment(bone_map[role], armature, material)

    face = _fixture_add_box(
        "QA_Face",
        (0.0, -0.01, 1.72),
        (0.30, 0.20, 0.27),
        armature,
        "Head",
        material,
    )
    _fixture_add_face_shape_keys(face)

    for name, location, lens in (
        ("Camera_Medium", (0.0, -4.3, 1.25), 58.0),
        ("Camera_Wide", (0.0, -6.8, 1.10), 50.0),
    ):
        camera_data = bpy.data.cameras.new(f"{name}_Data")
        camera = bpy.data.objects.new(name, camera_data)
        scene.collection.objects.link(camera)
        camera.location = location
        camera.data.lens = lens
        _fixture_look_at(camera, render_aroll_master_qa.Vector((0.0, 0.0, 1.05)))
    scene.camera = bpy.data.objects["Camera_Medium"]

    light_data = bpy.data.lights.new("QA_Key_Data", "AREA")
    light_data.energy = 900.0
    light_data.shape = "DISK"
    light_data.size = 4.0
    light = bpy.data.objects.new("QA_Key", light_data)
    scene.collection.objects.link(light)
    light.location = (-2.5, -3.0, 4.2)
    _fixture_look_at(light, render_aroll_master_qa.Vector((0.0, 0.0, 1.0)))

    blender_renderer.create_action_library(armature, {"mouth": face}, bone_map, fps=30)
    return scene, armature, face, bone_map


def _assert_runtime_error(fragment: str, operation) -> None:
    try:
        operation()
    except RuntimeError as exc:
        assert fragment in str(exc), str(exc)
    else:
        raise AssertionError(f"expected RuntimeError containing {fragment!r}")


def test_aroll_qa_fixture_fails_closed_for_missing_master_contract() -> None:
    _, _, face, _ = _build_aroll_qa_fixture()
    render_aroll_master_qa.validate_scene_contract()

    action = bpy.data.actions["Aroll_Idle_Listening"]
    action.name = "QA_Missing_Action"
    _assert_runtime_error(
        "missing required Actions: Aroll_Idle_Listening",
        render_aroll_master_qa.validate_scene_contract,
    )
    action.name = "Aroll_Idle_Listening"

    camera = bpy.data.objects["Camera_Medium"]
    camera.name = "QA_Missing_Camera"
    _assert_runtime_error("missing required cameras: Camera_Medium", render_aroll_master_qa.validate_scene_contract)
    camera.name = "Camera_Medium"

    shape = face.data.shape_keys.key_blocks["Eye_Squint.L"]
    shape.name = "QA_Missing_Squint"
    _assert_runtime_error("missing required Shape Keys: Eye_Squint.L", render_aroll_master_qa.validate_scene_contract)
    shape.name = "Eye_Squint.L"


def test_saved_master_adds_standalone_qa_cameras_without_collection_conflicts() -> None:
    scene, armature, _, _ = _build_aroll_qa_fixture()
    for name in render_aroll_master_qa.ACTION_CAMERAS:
        bpy.data.objects.remove(bpy.data.objects[name], do_unlink=True)

    with tempfile.TemporaryDirectory() as tmp:
        master_path = Path(tmp) / "fixture-master.blend"
        character_objects = [obj for obj in scene.objects if obj.type == "MESH"]
        master_asset.save_master_collection(
            character_objects=character_objects,
            armature=armature,
            output_path=master_path,
        )

        assert master_path.is_file()
        collection = bpy.data.collections[master_asset.MASTER_COLLECTION]
        assert [obj.name for obj in collection.all_objects if obj.type == "ARMATURE"] == [
            armature.name
        ]
        assert not any(obj.type == "CAMERA" for obj in collection.all_objects)
        for name in render_aroll_master_qa.ACTION_CAMERAS:
            camera = scene.objects.get(name)
            assert camera is not None and camera.type == "CAMERA", name


def test_aroll_qa_renders_programmatic_fixture_and_checks_pixels() -> None:
    _build_aroll_qa_fixture()
    with tempfile.TemporaryDirectory() as tmp:
        output_dir = Path(tmp) / "aroll-master-qa"
        report = render_aroll_master_qa.run_qa(output_dir, resolution=128)

        assert report["status"] == "ready"
        assert report["faceCapability"] == {
            "blinkCapability": "squint_only",
            "qaSample": "face/squint.png",
            "fullBlinkClaimed": False,
        }
        assert report["lighting"]["preset"] == "qa_editorial_soft"
        assert report["lighting"]["lightCount"] >= 3
        assert len(report["samples"]) == 54
        assert all(
            {"action", "camera", "path", "boneRotations", "fingertipDisplacementRatios"}
            <= set(sample)
            for sample in report["samples"]
        )
        right_close = [
            sample["framing"]
            for sample in report["samples"]
            if sample["camera"] == render_aroll_master_qa.HAND_CLOSE_CAMERA_RIGHT
        ]
        left_close = [
            sample["framing"]
            for sample in report["samples"]
            if sample["camera"] == render_aroll_master_qa.HAND_CLOSE_CAMERA_LEFT
        ]
        assert len({json.dumps(framing, sort_keys=True) for framing in right_close}) == 1
        assert len({json.dumps(framing, sort_keys=True) for framing in left_close}) == 1
        assert report["comparisons"]["openFistPixelDifference"] >= (
            render_aroll_master_qa.MIN_OPEN_FIST_PIXEL_DIFFERENCE
        )
        assert all(
            difference >= render_aroll_master_qa.MIN_FINGER_ROLL_PIXEL_DIFFERENCE
            for difference in report["comparisons"]["fingerRollPixelDifferences"].values()
        )
        assert all(
            left != right
            for hashes in report["framemd5"].values()
            for left, right in zip(hashes, hashes[1:])
        )
        render_aroll_master_qa.require_files(
            output_dir,
            (*render_aroll_master_qa.required_relative_paths(), render_aroll_master_qa.REPORT_NAME),
        )
        _assert_runtime_error(
            "duplicate adjacent fixture frames",
            lambda: render_aroll_master_qa.assert_adjacent_frames_unique(
                [output_dir / "hand/open.png", output_dir / "hand/open.png"],
                "fixture",
            ),
        )
        (output_dir / "hand/open.png").unlink()
        _assert_runtime_error(
            "required files are missing: hand/open.png",
            lambda: render_aroll_master_qa.require_files(
                output_dir, render_aroll_master_qa.required_relative_paths()
            ),
        )


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


def sampled_role_rotations(armature, bone_map, action_name: str, frame: int):
    armature.animation_data.action = bpy.data.actions[action_name]
    bpy.context.scene.frame_set(frame)
    return {
        role: tuple(armature.pose.bones[name].rotation_euler.copy())
        for role, name in bone_map.items()
        if name in armature.pose.bones
    }


def pose_bone_world_head(armature, bone_map, role: str):
    return armature.matrix_world @ armature.pose.bones[bone_map[role]].head


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


def _source_hand_fixture():
    bpy.ops.wm.read_factory_settings(use_empty=True)
    objects, armatures, imported_assets, _ = blender_renderer.import_model(str(RIGGED_FBX_PATH))
    assert len(armatures) == 1
    dimensions = blender_renderer.prepare_character(
        objects,
        target_height=2.55,
        preserve_hierarchy=True,
        asset_objects=imported_assets,
    )
    armature = armatures[0]
    bone_map = blender_renderer.resolve_bone_roles([bone.name for bone in armature.data.bones])
    return objects, dimensions, armature, bone_map


def _surface_snapshot(objects):
    return {
        obj.name: {
            "object": obj.as_pointer(),
            "mesh": obj.data.as_pointer(),
            "vertices": tuple(tuple(float(value) for value in vertex.co) for vertex in obj.data.vertices),
            "vertexCount": len(obj.data.vertices),
            "edgeCount": len(obj.data.edges),
            "polygonCount": len(obj.data.polygons),
            "materials": tuple(material.as_pointer() if material else 0 for material in obj.data.materials),
            "uvLayers": tuple(
                (
                    layer.name,
                    tuple(tuple(float(value) for value in item.uv) for item in layer.data),
                )
                for layer in obj.data.uv_layers
            ),
        }
        for obj in objects
        if obj.type == "MESH"
    }


def _restore_surface_coordinates(objects, snapshot) -> None:
    for obj in objects:
        if obj.name not in snapshot:
            continue
        for vertex, coordinate in zip(obj.data.vertices, snapshot[obj.name]["vertices"]):
            vertex.co = coordinate
        obj.data.update()


def _enhance_source_fixture(objects, dimensions, armature):
    return hand_refinement.enhance_three_segment_hands(
        armature=armature,
        objects=objects,
        dimensions=dimensions,
        stats={},
        resolve_roles=blender_renderer.resolve_bone_roles,
    )


def test_refined_hands_preserve_three_digits_and_taper_each_tip() -> None:
    objects, dimensions, armature, _, _, _ = load_enhanced_fbx_character()
    stats = armature.get(HAND_AESTHETIC_REPORT_KEY)
    assert stats is not None
    report = json.loads(str(stats))

    assert report["handAestheticVersion"] == "three_digit_refined_v2"
    for side in ("l", "r"):
        assert report["sides"][side]["digitCount"] == 3
        for digit in report["sides"][side]["digits"]:
            assert digit["tipWidth"] / digit["rootWidth"] <= 0.78, (side, digit)
            assert digit["rootTransitionContinuity"] >= 0.82, (side, digit)
            assert digit["wristBoundaryMaxDisplacement"] <= dimensions["width"] * 0.002, (
                side,
                digit,
            )

    assert sum(
        bone.use_deform and is_finger_deform_bone_name(bone.name)
        for bone in armature.data.bones
    ) == 18
    assert not any(obj.get("ip_avatar_replacement_hand") for obj in objects)


def test_refined_hand_weights_remain_normalized_and_isolated() -> None:
    _, _, armature, _, _, _ = load_enhanced_fbx_character()
    assert HAND_AESTHETIC_REPORT_KEY in armature
    report = json.loads(str(armature[HAND_AESTHETIC_REPORT_KEY]))

    assert report["maxInfluences"] <= 4
    assert report["unnormalizedVertices"] == 0
    assert report["unweightedVertices"] == 0
    assert report["neighborTipLeakageMax"] <= 0.08


def test_refinement_preserves_source_topology_uvs_materials_and_upgrades_v1_v2_once() -> None:
    objects, dimensions, armature, _ = _source_hand_fixture()
    shape_object = max((obj for obj in objects if obj.type == "MESH"), key=lambda obj: len(obj.data.vertices))
    shape_object.shape_key_add(name="Basis", from_mix=False)
    shape_object.shape_key_add(name="Face_Fixture", from_mix=False)
    shape_coordinates = {
        key.name: tuple(tuple(float(value) for value in point.co) for point in key.data)
        for key in shape_object.data.shape_keys.key_blocks
    }
    source = _surface_snapshot(objects)
    report, _ = _enhance_source_fixture(objects, dimensions, armature)
    refined = _surface_snapshot(objects)

    assert set(refined) == set(source)
    for name, before in source.items():
        after = refined[name]
        assert after["object"] == before["object"]
        assert after["mesh"] == before["mesh"]
        assert after["vertexCount"] == before["vertexCount"]
        assert after["edgeCount"] == before["edgeCount"]
        assert after["polygonCount"] == before["polygonCount"]
        assert after["materials"] == before["materials"]
        assert after["uvLayers"] == before["uvLayers"]
    changed_shape_indices = [
        index
        for index, (before, after) in enumerate(
            zip(source[shape_object.name]["vertices"], refined[shape_object.name]["vertices"])
        )
        if before != after
    ]
    assert changed_shape_indices
    for key in shape_object.data.shape_keys.key_blocks:
        for index in changed_shape_indices:
            mesh_delta = Vector(refined[shape_object.name]["vertices"][index]) - Vector(
                source[shape_object.name]["vertices"][index]
            )
            expected = Vector(shape_coordinates[key.name][index]) + mesh_delta
            assert (key.data[index].co - expected).length <= 1e-6, (key.name, index)
    assert report.get("handAestheticVersion") == "three_digit_refined_v2"

    for legacy_version in (1, 2):
        _restore_surface_coordinates(objects, source)
        for key in shape_object.data.shape_keys.key_blocks:
            for point, coordinate in zip(key.data, shape_coordinates[key.name]):
                point.co = coordinate
        contract_name = hand_refinement.LEGACY_HAND_CONTRACT_NAMES[legacy_version]
        armature[hand_refinement.HAND_CONTRACT_KEY] = contract_name
        armature[hand_refinement.HAND_CONTRACT_VERSION_KEY] = legacy_version
        if legacy_version == 1 and hand_refinement.HAND_TOPOLOGY_MODE_KEY in armature:
            del armature[hand_refinement.HAND_TOPOLOGY_MODE_KEY]
        elif legacy_version == 2:
            armature[hand_refinement.HAND_TOPOLOGY_MODE_KEY] = hand_refinement.SOURCE_SURFACE_TOPOLOGY_MODE
        for obj in objects:
            if obj.type == "MESH" and obj.get(hand_refinement.HAND_CONTRACT_KEY):
                obj[hand_refinement.HAND_CONTRACT_KEY] = contract_name
                obj[hand_refinement.HAND_CONTRACT_VERSION_KEY] = legacy_version
                if legacy_version == 1 and hand_refinement.HAND_TOPOLOGY_MODE_KEY in obj:
                    del obj[hand_refinement.HAND_TOPOLOGY_MODE_KEY]
                elif legacy_version == 2:
                    obj[hand_refinement.HAND_TOPOLOGY_MODE_KEY] = hand_refinement.SOURCE_SURFACE_TOPOLOGY_MODE
        for owner in (armature, *objects):
            if HAND_AESTHETIC_VERSION_KEY in owner:
                del owner[HAND_AESTHETIC_VERSION_KEY]
            if HAND_AESTHETIC_REPORT_KEY in owner:
                del owner[HAND_AESTHETIC_REPORT_KEY]

        upgraded, _ = _enhance_source_fixture(objects, dimensions, armature)
        assert upgraded["handAestheticUpgradeFromVersion"] == legacy_version
        assert int(armature[hand_refinement.HAND_CONTRACT_VERSION_KEY]) == hand_refinement.HAND_CONTRACT_VERSION
        once = _surface_snapshot(objects)
        reused, _ = _enhance_source_fixture(objects, dimensions, armature)
        twice = _surface_snapshot(objects)
        assert reused["fingerRigReused"] is True
        assert all(twice[name]["vertices"] == once[name]["vertices"] for name in once)


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
    if stats.get("fingerTopologyMode") != "source_surface_weighted":
        violations.append(
            f"fingerTopologyMode={stats.get('fingerTopologyMode')!r}, expected source_surface_weighted"
        )
    if stats.get("handDetailAddedVertices") != 0:
        violations.append(
            f"handDetailAddedVertices={stats.get('handDetailAddedVertices')!r}, expected 0 to preserve the clean source surface"
        )
    if stats.get("handDetailVertexCountAfter") != stats.get("handDetailVertexCountBefore"):
        violations.append("source-surface hand refinement changed the source vertex count")
    band_counts = stats.get("handWeightedJointBandVertexCounts") or {}
    if stats.get("handWeightedJointBandCount") != 12:
        violations.append(
            f"handWeightedJointBandCount={stats.get('handWeightedJointBandCount')!r}, expected 12 actual joint zones"
        )
    if len(band_counts) != 12 or any(int(count) <= 0 for count in band_counts.values()):
        violations.append(f"weighted joint-band vertex evidence is incomplete: {band_counts!r}")
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

    violations.extend(_weight_violations(objects, armature))
    cross_digit_vertices = []
    for obj in objects:
        if obj.type != "MESH":
            continue
        names = {group.index: group.name for group in obj.vertex_groups}
        for vertex in obj.data.vertices:
            digits = {
                re.match(r"Finger_(\d+)_", names.get(assignment.group, "")).group(1)
                + names[assignment.group].rsplit(".", 1)[-1]
                for assignment in vertex.groups
                if assignment.weight >= 0.05
                and re.match(r"Finger_(\d+)_", names.get(assignment.group, ""))
            }
            if len(digits) > 1:
                cross_digit_vertices.append(f"{obj.name}:{vertex.index}")
    if cross_digit_vertices:
        violations.append(
            f"{len(cross_digit_vertices)} vertices have material cross-digit weights >= 0.05; "
            f"examples: {', '.join(cross_digit_vertices[:8])}"
        )
    assert not violations, "\n".join(violations)


def test_segment_weighting_is_rigid_away_from_knuckles() -> None:
    region = hand_refinement.DigitRegion(
        side="R",
        index=1,
        records=[],
        axis=Vector((1.0, 0.0, 0.0)),
        base=Vector((0.0, 0.0, 0.0)),
        joint_1=Vector((0.46, 0.0, 0.0)),
        joint_2=Vector((0.68, 0.0, 0.0)),
        tip=Vector((1.0, 0.0, 0.0)),
        minimum=0.0,
        maximum=1.0,
        feature_center=Vector((0.0, 0.0)),
    )
    assert hand_refinement._normalized_segment_weights(region, 0.23, 1.0)[0] >= 0.999
    assert hand_refinement._normalized_segment_weights(region, 0.57, 1.0)[1] >= 0.999
    assert hand_refinement._normalized_segment_weights(region, 0.84, 1.0)[2] >= 0.999


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
        assert 0.25 < proximal.z <= 0.30, (digit, tuple(proximal))
        assert 0.29 < middle.z <= 0.34, (digit, tuple(middle))
        assert 0.18 < distal.z <= 0.22, (digit, tuple(distal))

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
        assert 0.26 < proximal.z <= 0.31, (selected, tuple(proximal))
        assert 0.31 < middle.z <= 0.35, (selected, tuple(middle))
        assert 0.19 < distal.z <= 0.23, (selected, tuple(distal))


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


def test_aroll_action_pack_names_reset_interpolation_and_safe_hand_stage() -> None:
    _, dimensions, armature, _, bone_map, _ = load_enhanced_fbx_character()

    report = blender_renderer.create_action_library(armature, {}, bone_map, fps=30)

    missing = sorted(AROLL_ACTIONS - set(report["actions"]))
    assert not missing, f"missing A-roll actions: {missing}"
    face_center = pose_bone_world_head(armature, bone_map, "head")
    face_half_width = float(dimensions["width"]) * 0.16
    face_half_height = float(dimensions["height"]) * 0.16
    safe_stage_actions = {
        "Aroll_OpenPalm_Explain": ("r", "l"),
        "Aroll_Count_One": ("r",),
        "Aroll_Count_Two": ("r",),
        "Aroll_Count_Three": ("r",),
        "Aroll_Point_Left": ("l",),
        "Aroll_Point_Right": ("r",),
        "Aroll_Pinch_Detail": ("r",),
    }
    leg_roles = {"leg_l", "shin_l", "foot_l", "leg_r", "shin_r", "foot_r"}

    for action_name in sorted(AROLL_ACTIONS):
        action = bpy.data.actions[action_name]
        frames = sorted(
            {
                int(round(float(point.co.x)))
                for curve in blender_renderer.iter_action_fcurves(action)
                for point in curve.keyframe_points
            }
        )
        assert frames == [1, 12, 24, 46, 60], (action_name, frames)

        for curve in blender_renderer.iter_action_fcurves(action):
            for point in curve.keyframe_points:
                assert point.interpolation == "BEZIER", (action_name, curve.data_path)
                assert point.handle_left_type == "AUTO_CLAMPED", (action_name, curve.data_path)
                assert point.handle_right_type == "AUTO_CLAMPED", (action_name, curve.data_path)

        start = sampled_role_rotations(armature, bone_map, action_name, frames[0])
        end = sampled_role_rotations(armature, bone_map, action_name, frames[-1])
        for role, start_rotation in start.items():
            end_rotation = end[role]
            assert max(abs(end_rotation[index] - start_rotation[index]) for index in range(3)) < 1e-6, (
                action_name,
                role,
                start_rotation,
                end_rotation,
            )

        hold_frame = min(frames[-2], max(frames[1], 30))
        sampled_role_rotations(armature, bone_map, action_name, hold_frame)
        for role in leg_roles:
            if role not in bone_map:
                continue
            rotation = armature.pose.bones[bone_map[role]].rotation_euler
            magnitude = max(abs(value) for value in rotation)
            if action_name == "Aroll_Seated_Idle":
                assert magnitude > 0.04, (action_name, role, tuple(rotation))
            else:
                assert magnitude < 1e-6, (action_name, role, tuple(rotation))

        for side in safe_stage_actions.get(action_name, ()):
            hand = pose_bone_world_head(armature, bone_map, f"hand_{side}")
            offset = hand - face_center
            inside_face_box = (
                abs(offset.x) < face_half_width
                and abs(offset.y) < face_half_width
                and abs(offset.z) < face_half_height
            )
            assert not inside_face_box, (action_name, side, tuple(offset))
            assert -float(dimensions["height"]) * 0.62 < offset.z < float(dimensions["height"]) * 0.08, (
                action_name,
                side,
                tuple(offset),
            )


if __name__ == "__main__":
    tests = [
        test_aroll_qa_sample_contract_is_complete_and_squint_only,
        test_aroll_qa_fixture_fails_closed_for_missing_master_contract,
        test_saved_master_adds_standalone_qa_cameras_without_collection_conflicts,
        test_aroll_qa_renders_programmatic_fixture_and_checks_pixels,
        test_aroll_action_pack_names_reset_interpolation_and_safe_hand_stage,
        test_refined_hands_preserve_three_digits_and_taper_each_tip,
        test_refined_hand_weights_remain_normalized_and_isolated,
        test_refinement_preserves_source_topology_uvs_materials_and_upgrades_v1_v2_once,
        test_main_ip_has_three_segments_per_digit_and_clean_weights,
        test_segment_weighting_is_rigid_away_from_knuckles,
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
