#!/usr/bin/env python3
"""Blender integration tests for the unrigged anthropomorphic presenter path."""

from __future__ import annotations

import sys
import unittest
from pathlib import Path

try:
    import bpy
except ModuleNotFoundError as exc:
    raise unittest.SkipTest("requires Blender's bpy runtime") from exc

SCRIPT_DIR = Path(__file__).resolve().parent
REPO_ROOT = SCRIPT_DIR.parents[1]
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import blender_renderer


MODEL_PATH = REPO_ROOT / "ip形象/main_ip/turnaround/3d模型.glb"
RIGGED_FBX_PATH = REPO_ROOT / "ip形象/main_ip/turnaround/带骨骼3d模型.fbx"
EXPORTED_GLB_PATH = REPO_ROOT / "ip形象/main_ip/models/main-ip-rigged.glb"


def reset_scene() -> None:
    bpy.ops.wm.read_factory_settings(use_empty=True)


def load_rigged_character():
    reset_scene()
    character_objects, armatures, imported_assets, _ = blender_renderer.import_model(str(MODEL_PATH))
    assert armatures == []
    dimensions = blender_renderer.prepare_character(character_objects, target_height=2.55)
    armature, rig_stats, bone_map = blender_renderer.create_simple_rig(character_objects, dimensions)
    return character_objects, imported_assets, dimensions, armature, rig_stats, bone_map


def load_enhanced_fbx_character():
    reset_scene()
    character_objects, armatures, imported_assets, removed = blender_renderer.import_model(str(RIGGED_FBX_PATH))
    assert len(armatures) == 1
    dimensions = blender_renderer.prepare_character(
        character_objects,
        target_height=2.55,
        preserve_hierarchy=True,
        asset_objects=imported_assets,
    )
    assert sum(
        obj.type == "EMPTY" and obj.name.startswith("IP_Character_Container")
        for obj in bpy.context.scene.objects
    ) == 1
    armature, rig_stats, bone_map = blender_renderer.choose_character_rig(
        {"rigMode": "auto", "preserveExistingRig": True, "enhanceExistingRig": True},
        armatures,
        character_objects,
        dimensions,
    )
    return character_objects, dimensions, armature, rig_stats, bone_map, removed


def test_rigged_fbx_import_preserves_source_materials_and_removes_scene_helpers() -> None:
    character_objects, _, armature, rig_stats, _, _ = load_enhanced_fbx_character()

    assert len(character_objects) == 2
    assert armature.name == "Armature"
    assert rig_stats["inputRigPreserved"] is True
    assert rig_stats["sourceBoneCount"] == 28
    assert not {"Cube", "Camera", "Light"}.intersection({obj.name for obj in bpy.context.scene.objects})
    assert any(
        node.type == "TEX_IMAGE" and node.image and node.image.size[0] == 4096
        for obj in character_objects
        for material in obj.data.materials
        if material and material.use_nodes
        for node in material.node_tree.nodes
    )


def test_rigged_fbx_gains_two_segment_three_digit_hands_with_valid_weights() -> None:
    character_objects, _, armature, rig_stats, bone_map, _ = load_enhanced_fbx_character()
    bones = {bone.name for bone in armature.data.bones}
    expected = {
        f"Finger_{digit:02d}_{segment}.{side}"
        for side in ("L", "R")
        for digit in (1, 2, 3)
        for segment in ("Proximal", "Distal")
    }

    assert expected.issubset(bones)
    assert rig_stats["fingerRigEnhanced"] is True
    assert rig_stats["fingerBoneCount"] == 12
    assert rig_stats["handDetailAddedVertices"] > 0
    assert rig_stats["handDetailVertexCountAfter"] > rig_stats["handDetailVertexCountBefore"]
    assert rig_stats["fingerWeightingMode"] == "soft_digit_blend"
    assert rig_stats["fingerBlendVertexCount"] > 0
    assert rig_stats["preserveVolumeSkinning"] is True
    assert bone_map["finger_1_l"] == "Finger_01_Proximal.L"
    assert bone_map["finger_1_tip_l"] == "Finger_01_Distal.L"
    assert bone_map["finger_3_tip_r"] == "Finger_03_Distal.R"
    for name in expected:
        assert rig_stats["weightedVertexCounts"].get(name, 0) > 0, name
    assert rig_stats["unweightedVertexCount"] == 0
    assert rig_stats["maxVertexInfluences"] <= 4
    assert sum(len(obj.data.vertices) for obj in character_objects) == rig_stats["vertexCount"]


def test_rigged_fbx_face_retopologizes_original_mesh_without_visible_overlays() -> None:
    character_objects, dimensions, armature, rig_stats, bone_map, _ = load_enhanced_fbx_character()
    original_mesh_names = {obj.name for obj in bpy.data.objects if obj.type == "MESH"}
    original_vertex_count = sum(len(obj.data.vertices) for obj in character_objects)
    face = blender_renderer.setup_face(
        {
            "characterId": "main_ip_sloth",
            "faceScreenMode": "source",
            "mouthMode": "source_mesh_visemes",
            "mouthHeightRatio": 0.805,
            "mouthScale": 1.0,
            "mouthStyle": "source_mesh",
            "facialDetailMode": "rich",
            "facialTopologyMode": "source_retopology",
        },
        dimensions,
        armature,
        character_objects,
        bone_map,
    )

    face_mesh = face["mouth"]
    assert face_mesh in character_objects
    assert len(face_mesh.data.vertices) > original_vertex_count - sum(
        len(obj.data.vertices) for obj in character_objects if obj != face_mesh
    )
    assert face_mesh["source_face_topology"] == "integrated_source_retopology"
    assert face_mesh["integrated_mouth_seam"] is True
    assert face_mesh["mouth_seam_edge_count"] >= 8
    assert face_mesh["mouth_upper_boundary_count"] >= 8
    assert face_mesh["mouth_lower_boundary_count"] >= 8
    assert face_mesh["source_texture_face_preserved"] is True

    forbidden_overlays = {
        "IP_UpperLip", "IP_LowerLip",
        "IP_UpperLid.L", "IP_LowerLid.L", "IP_UpperLid.R", "IP_LowerLid.R",
    }
    assert forbidden_overlays.isdisjoint({obj.name for obj in bpy.data.objects})
    assert not {
        "upper_lip", "lower_lip", "upper_lid_l", "lower_lid_l", "upper_lid_r", "lower_lid_r",
    }.intersection(face)

    internal_roles = {"oral_cavity", "upper_teeth", "lower_teeth", "tongue"}
    assert internal_roles.issubset(face), sorted(internal_roles.difference(face))
    new_meshes = {
        obj.name
        for obj in bpy.data.objects
        if obj.type == "MESH" and obj.name not in original_mesh_names
    }
    assert new_meshes == {face[role].name for role in internal_roles}
    for role in internal_roles:
        obj = face[role]
        assert obj.get("ip_face_topology_role") == role
        assert obj.modifiers.get("IP_Face_Armature") is not None

    expected_bones = {"Jaw", "Eye.L", "Eye.R", "Tongue_01", "Tongue_02", "Tongue_03"}
    assert expected_bones.issubset({bone.name for bone in armature.data.bones})
    assert bone_map["jaw"] == "Jaw"
    assert bone_map["eye_l"] == "Eye.L"
    assert bone_map["eye_r"] == "Eye.R"
    assert bone_map["tongue_1"] == "Tongue_01"
    assert rig_stats["facialRigEnhanced"] is True
    for bone_name in ("Jaw", "Eye.L", "Eye.R"):
        assert rig_stats["weightedVertexCounts"].get(bone_name, 0) > 0, bone_name

    names = {key.name for key in face_mesh.data.shape_keys.key_blocks}
    expected = {
        "Mouth_Rest", "Mouth_A", "Mouth_E", "Mouth_O", "Mouth_U", "Mouth_MBP",
        "Mouth_Smile", "Mouth_Frown", "Mouth_Surprise",
        "Eye_Blink.L", "Eye_Blink.R", "Eye_Wide.L", "Eye_Wide.R",
        "Eye_Look_Left", "Eye_Look_Right", "Brow_Raise.L", "Brow_Raise.R",
        "Brow_Furrow.L", "Brow_Furrow.R", "Cheek_Smile.L", "Cheek_Smile.R",
        "Cheek_Puff.L", "Cheek_Puff.R", "Nose_Flare.L", "Nose_Flare.R",
    }
    assert expected.issubset(names), sorted(expected.difference(names))
    assert face_mesh["source_mouth_replacement"] is False
    assert face_mesh["facial_detail_mode"] == "rich_source_mesh"
    assert face_mesh["mouth_vertex_count"] >= 32
    assert face_mesh.data.color_attributes.get("IP_Mouth_Mask") is None
    assert all(
        node.name not in {"IP_Mouth_Open_Mix", "IP_Mouth_Mask_Attribute", "IP_Mouth_Open_Value"}
        for material in face_mesh.data.materials
        if material and material.use_nodes
        for node in material.node_tree.nodes
    )
    basis = face_mesh.data.shape_keys.key_blocks["Basis"]
    blink = face_mesh.data.shape_keys.key_blocks["Eye_Blink.L"]
    assert sum(
        (basis.data[index].co - blink.data[index].co).length > 1e-5
        for index in range(len(basis.data))
    ) >= 12


def test_rigged_fbx_action_library_uses_source_axes_distal_fingers_and_rich_face() -> None:
    character_objects, dimensions, armature, _, bone_map, _ = load_enhanced_fbx_character()
    face = blender_renderer.setup_face(
        {
            "characterId": "main_ip_sloth",
            "faceScreenMode": "source",
            "mouthMode": "source_mesh_visemes",
            "mouthHeightRatio": 0.805,
            "mouthScale": 1.0,
            "facialDetailMode": "rich",
            "facialTopologyMode": "source_retopology",
        },
        dimensions,
        armature,
        character_objects,
        bone_map,
    )

    report = blender_renderer.create_action_library(armature, face, bone_map, fps=30)

    assert {
        "Gesture_Count_One", "Gesture_Count_Two", "Gesture_Pinch",
        "Gesture_OpenHand", "Gesture_Fist", "Gesture_WristTwist", "Gesture_FingerWave",
    }.issubset(report["actions"])
    assert {
        "Face_Neutral", "Face_Happy", "Face_Thinking", "Face_Surprised",
        "Face_Confused", "Face_Serious", "Face_Blink",
    }.issubset(report["faceActions"])

    armature.animation_data.action = bpy.data.actions["Idle_Speaking"]
    bpy.context.scene.frame_set(1)
    left_arm = armature.pose.bones[bone_map["upper_arm_l"]].rotation_euler
    right_arm = armature.pose.bones[bone_map["upper_arm_r"]].rotation_euler
    assert left_arm.z > 0.8
    assert right_arm.z < -0.8

    armature.animation_data.action = bpy.data.actions["Gesture_Wave"]
    bpy.context.scene.frame_set(15)
    wrist_out = armature.pose.bones[bone_map["hand_r"]].rotation_euler.copy()
    open_fingers = [
        armature.pose.bones[bone_map[f"finger_{digit}_r"]].rotation_euler.copy()
        for digit in (1, 2, 3)
    ]
    middle_out = armature.pose.bones[bone_map["finger_2_r"]]
    middle_out_direction = (middle_out.tail - middle_out.head).normalized()
    bpy.context.scene.frame_set(30)
    wrist_in = armature.pose.bones[bone_map["hand_r"]].rotation_euler.copy()
    middle_in = armature.pose.bones[bone_map["finger_2_r"]]
    middle_in_direction = (middle_in.tail - middle_in.head).normalized()
    assert wrist_out.y < -0.90, tuple(wrist_out)
    assert wrist_in.y < -0.90, tuple(wrist_in)
    assert abs(wrist_out.y - wrist_in.y) < 0.10
    assert wrist_out.z * wrist_in.z < 0.0
    assert all(abs(finger.z) < 0.04 for finger in open_fingers)
    assert open_fingers[0].x > 0.10
    assert open_fingers[2].x < -0.10
    assert middle_out_direction.z > 0.45, tuple(middle_out_direction)
    assert middle_in_direction.z > 0.45, tuple(middle_in_direction)

    armature.animation_data.action = bpy.data.actions["Gesture_Fist"]
    bpy.context.scene.frame_set(30)
    for digit in (1, 2, 3):
        proximal = armature.pose.bones[bone_map[f"finger_{digit}_r"]].rotation_euler
        distal = armature.pose.bones[bone_map[f"finger_{digit}_tip_r"]].rotation_euler
        assert proximal.z > 0.20, (digit, tuple(proximal))
        assert distal.z > 0.06, (digit, tuple(distal))

    armature.animation_data.action = bpy.data.actions["Gesture_WristTwist"]
    bpy.context.scene.frame_set(30)
    wrist = armature.pose.bones[bone_map["hand_r"]].rotation_euler
    upper_arm = armature.pose.bones[bone_map["upper_arm_r"]].rotation_euler
    forearm = armature.pose.bones[bone_map["forearm_r"]].rotation_euler
    assert abs(wrist.y) > 0.45, tuple(wrist)
    assert upper_arm.z > -1.0, tuple(upper_arm)
    assert forearm.z > 0.45, tuple(forearm)

    for action_name in ("Gesture_Count_One", "Gesture_Count_Two", "Gesture_Pinch"):
        armature.animation_data.action = bpy.data.actions[action_name]
        bpy.context.scene.frame_set(30)
        for role in (
            "finger_1_r", "finger_1_tip_r", "finger_2_r", "finger_2_tip_r",
            "finger_3_r", "finger_3_tip_r",
        ):
            rotation = armature.pose.bones[bone_map[role]].rotation_euler
            assert abs(rotation.z) <= 1.25, (action_name, role, tuple(rotation))

    armature.animation_data.action = bpy.data.actions["Gesture_Count_One"]
    bpy.context.scene.frame_set(30)
    extended = armature.pose.bones[bone_map["finger_1_r"]].rotation_euler
    curled_2 = armature.pose.bones[bone_map["finger_2_r"]].rotation_euler
    curled_3 = armature.pose.bones[bone_map["finger_3_r"]].rotation_euler
    assert abs(extended.z) < 0.16
    assert curled_2.z > 0.20
    assert curled_3.z > 0.22

    keys = face["mouth"].data.shape_keys
    keys.animation_data.action = bpy.data.actions["Face_Surprised"]
    bpy.context.scene.frame_set(30)
    assert keys.key_blocks["Mouth_Surprise"].value > 0.5
    assert keys.key_blocks["Eye_Wide.L"].value > 0.5
    assert keys.key_blocks["Brow_Raise.R"].value > 0.5

    armature.animation_data.action = bpy.data.actions["Look_Left"]
    bpy.context.scene.frame_set(30)
    assert abs(armature.pose.bones[bone_map["eye_l"]].rotation_euler.y) > 0.02
    assert abs(armature.pose.bones[bone_map["eye_r"]].rotation_euler.y) > 0.02


def test_rigged_fbx_talking_timeline_uses_source_axes_distal_fingers_and_blink() -> None:
    character_objects, dimensions, armature, _, bone_map, _ = load_enhanced_fbx_character()
    face = blender_renderer.setup_face(
        {
            "characterId": "main_ip_sloth",
            "faceScreenMode": "source",
            "mouthMode": "source_mesh_visemes",
            "mouthHeightRatio": 0.805,
            "mouthScale": 1.0,
            "facialDetailMode": "rich",
            "facialTopologyMode": "source_retopology",
        },
        dimensions,
        armature,
        character_objects,
        bone_map,
    )
    plan = {
        "durationSec": 2.0,
        "motionEvents": [
            {"timeSec": 0.0, "motion": "wave", "duration": 1.2, "strength": 0.9},
            {"timeSec": 0.35, "motion": "blink", "duration": 0.35, "strength": 1.0},
        ],
        "lipSync": [{"timeSec": 0.0, "viseme": "a", "open": 0.8}],
    }

    blender_renderer.animate(armature, face, plan, fps=30, bone_map=bone_map)

    bpy.context.scene.frame_set(15)
    left_arm = armature.pose.bones[bone_map["upper_arm_l"]].rotation_euler
    right_arm = armature.pose.bones[bone_map["upper_arm_r"]].rotation_euler
    wrist_out = armature.pose.bones[bone_map["hand_r"]].rotation_euler.copy()
    open_fingers = [
        armature.pose.bones[bone_map[f"finger_{digit}_r"]].rotation_euler.copy()
        for digit in (1, 2, 3)
    ]
    middle_out = armature.pose.bones[bone_map["finger_2_r"]]
    middle_out_direction = (middle_out.tail - middle_out.head).normalized()
    keys = face["mouth"].data.shape_keys.key_blocks
    assert left_arm.z > 0.8
    assert right_arm.z > -0.8
    assert wrist_out.y < -0.90, tuple(wrist_out)
    assert all(abs(finger.z) < 0.04 for finger in open_fingers)
    assert open_fingers[0].x > 0.10
    assert open_fingers[2].x < -0.10
    assert keys["Eye_Blink.L"].value > 0.5
    assert keys["Eye_Blink.R"].value > 0.5
    assert keys["Mouth_A"].value > 0.5
    assert max(abs(value) for value in armature.pose.bones[bone_map["jaw"]].rotation_euler) > 0.01

    bpy.context.scene.frame_set(23)
    wrist_in = armature.pose.bones[bone_map["hand_r"]].rotation_euler.copy()
    middle_in = armature.pose.bones[bone_map["finger_2_r"]]
    middle_in_direction = (middle_in.tail - middle_in.head).normalized()
    assert wrist_in.y < -0.90, tuple(wrist_in)
    assert abs(wrist_out.y - wrist_in.y) < 0.12
    assert wrist_out.z * wrist_in.z < 0.0
    assert middle_out_direction.z > 0.45, tuple(middle_out_direction)
    assert middle_in_direction.z > 0.45, tuple(middle_in_direction)


def test_publish_render_detail_is_non_destructive_and_deformation_aware() -> None:
    character_objects, dimensions, armature, _, bone_map, _ = load_enhanced_fbx_character()
    face = blender_renderer.setup_face(
        {
            "characterId": "main_ip_sloth",
            "faceScreenMode": "source",
            "mouthMode": "source_mesh_visemes",
            "mouthHeightRatio": 0.805,
            "mouthScale": 1.0,
            "facialDetailMode": "rich",
            "facialTopologyMode": "source_retopology",
        },
        dimensions,
        armature,
        character_objects,
        bone_map,
    )
    source = face["mouth"]
    shape_names_before = [key.name for key in source.data.shape_keys.key_blocks]

    report = blender_renderer.configure_character_render_detail(
        character_objects,
        {"qualityPreset": "production_2k", "renderDetailMode": "publish"},
    )

    corrective = source.modifiers.get("IP_Render_CorrectiveSmooth")
    subdivision = source.modifiers.get("IP_Render_Subdivision")
    assert report["mode"] == "publish"
    assert source.name in report["detailedObjects"]
    assert corrective is not None and corrective.type == "CORRECTIVE_SMOOTH"
    assert subdivision is not None and subdivision.type == "SUBSURF"
    assert subdivision.subdivision_type == "SIMPLE"
    assert subdivision.levels == 1
    assert subdivision.render_levels >= 1
    assert [key.name for key in source.data.shape_keys.key_blocks] == shape_names_before
    assert all(polygon.use_smooth for polygon in source.data.polygons)


def test_talking_timeline_uses_clamped_bezier_interpolation() -> None:
    character_objects, dimensions, armature, _, bone_map, _ = load_enhanced_fbx_character()
    face = blender_renderer.setup_face(
        {
            "characterId": "main_ip_sloth",
            "faceScreenMode": "source",
            "mouthMode": "source_mesh_visemes",
            "mouthHeightRatio": 0.805,
            "mouthScale": 1.0,
            "facialDetailMode": "rich",
            "facialTopologyMode": "source_retopology",
        },
        dimensions,
        armature,
        character_objects,
        bone_map,
    )
    plan = {
        "durationSec": 1.0,
        "motionEvents": [{"timeSec": 0.1, "motion": "weight_shift", "duration": 0.8, "strength": 0.3}],
        "lipSync": [{"timeSec": 0.0, "viseme": "a", "open": 0.7}],
    }

    blender_renderer.animate(armature, face, plan, fps=30, bone_map=bone_map)

    actions = [armature.animation_data.action, face["mouth"].data.shape_keys.animation_data.action]
    keyframes = [
        point
        for action in actions
        for curve in blender_renderer.iter_action_fcurves(action)
        for point in curve.keyframe_points
    ]
    assert keyframes
    assert all(point.interpolation == "BEZIER" for point in keyframes)
    assert all(point.handle_left_type == "AUTO_CLAMPED" for point in keyframes)
    assert all(point.handle_right_type == "AUTO_CLAMPED" for point in keyframes)


def test_exported_glb_reimport_keeps_source_humanoid_axis_profile() -> None:
    reset_scene()
    character_objects, armatures, imported_assets, _ = blender_renderer.import_model(str(EXPORTED_GLB_PATH))
    dimensions = blender_renderer.prepare_character(
        character_objects,
        target_height=2.55,
        preserve_hierarchy=True,
        asset_objects=imported_assets,
    )
    weights_before = blender_renderer._collect_weight_stats(character_objects)
    assert sum(
        obj.type == "EMPTY" and obj.name.startswith("IP_Character_Container")
        for obj in bpy.context.scene.objects
    ) == 1
    armature, _, bone_map = blender_renderer.choose_character_rig(
        {"rigMode": "auto", "preserveExistingRig": True, "enhanceExistingRig": True},
        armatures,
        character_objects,
        dimensions,
    )
    weights_after = blender_renderer._collect_weight_stats(character_objects)
    face = blender_renderer.setup_face(
        {
            "faceScreenMode": "source",
            "mouthMode": "existing_visemes",
            "facialDetailMode": "rich",
            "facialTopologyMode": "source_retopology",
            "mouthHeightRatio": 0.805,
            "mouthScale": 1.0,
        },
        dimensions,
        armature,
        character_objects,
        bone_map,
    )

    forbidden_overlays = {
        "IP_UpperLip", "IP_LowerLip",
        "IP_UpperLid.L", "IP_LowerLid.L", "IP_UpperLid.R", "IP_LowerLid.R",
    }
    assert forbidden_overlays.isdisjoint({obj.name for obj in bpy.context.scene.objects})
    assert {"IP_OralCavity", "IP_UpperTeeth", "IP_LowerTeeth", "IP_Tongue"}.issubset(
        {obj.name for obj in bpy.context.scene.objects}
    )
    assert {"jaw", "eye_l", "eye_r", "tongue_1", "tongue_2", "tongue_3"}.issubset(bone_map)
    for bone_name in ("Head", "Jaw", "Eye.L", "Eye.R"):
        assert weights_after["weightedVertexCounts"].get(bone_name, 0) == weights_before["weightedVertexCounts"].get(bone_name, 0)
    assert face["mouth"].get("integrated_mouth_seam") is True
    assert blender_renderer.uses_source_humanoid_axes(armature) is True
    blender_renderer.create_action_library(armature, face, bone_map, fps=30)
    armature.animation_data.action = bpy.data.actions["Idle_Speaking"]
    bpy.context.scene.frame_set(1)
    assert armature.pose.bones[bone_map["upper_arm_l"]].rotation_euler.z > 0.8
    assert armature.pose.bones[bone_map["upper_arm_r"]].rotation_euler.z < -0.8


def test_generated_humanoid_rig_has_presenter_limbs_and_valid_weights() -> None:
    character_objects, _, _, armature, rig_stats, bone_map = load_rigged_character()
    bones = {bone.name for bone in armature.data.bones}
    required = {
        "Root",
        "Body",
        "Spine",
        "Chest",
        "Neck",
        "Head",
        "Jaw",
        "UpperArm.L",
        "ForeArm.L",
        "Hand.L",
        "Finger_01.L",
        "Finger_02.L",
        "Finger_03.L",
        "UpperArm.R",
        "ForeArm.R",
        "Hand.R",
        "Finger_01.R",
        "Finger_02.R",
        "Finger_03.R",
        "Thigh.L",
        "Shin.L",
        "Foot.L",
        "Thigh.R",
        "Shin.R",
        "Foot.R",
        "CTRL_Hand.L",
        "CTRL_Elbow.L",
        "CTRL_Hand.R",
        "CTRL_Elbow.R",
        "CTRL_Foot.L",
        "CTRL_Knee.L",
        "CTRL_Foot.R",
        "CTRL_Knee.R",
    }
    assert required.issubset(bones)
    for name in (
        "UpperArm.L",
        "ForeArm.L",
        "Hand.L",
        "UpperArm.R",
        "ForeArm.R",
        "Hand.R",
        "Finger_01.L",
        "Finger_02.L",
        "Finger_03.L",
        "Finger_01.R",
        "Finger_02.R",
        "Finger_03.R",
        "Jaw",
        "Thigh.L",
        "Shin.L",
        "Foot.L",
        "Thigh.R",
        "Shin.R",
        "Foot.R",
        "Head",
    ):
        assert rig_stats["weightedVertexCounts"].get(name, 0) > 0, name
    assert rig_stats["unweightedVertexCount"] == 0
    assert rig_stats["maxVertexInfluences"] <= 4
    assert bone_map["leg_l"] == "Thigh.L"
    assert bone_map["leg_r"] == "Thigh.R"
    assert bone_map["jaw"] == "Jaw"
    assert bone_map["finger_2_l"] == "Finger_02.L"
    assert sum(len(obj.data.vertices) for obj in character_objects) == rig_stats["vertexCount"]


def test_generated_rig_exposes_optional_hand_and_foot_ik_controls() -> None:
    _, _, _, armature, _, _ = load_rigged_character()
    for suffix in ("L", "R"):
        forearm = armature.pose.bones[f"ForeArm.{suffix}"]
        shin = armature.pose.bones[f"Shin.{suffix}"]
        hand_ik = forearm.constraints.get("IP_Hand_IK")
        foot_ik = shin.constraints.get("IP_Foot_IK")
        assert hand_ik is not None
        assert hand_ik.target == armature
        assert hand_ik.subtarget == f"CTRL_Hand.{suffix}"
        assert hand_ik.pole_subtarget == f"CTRL_Elbow.{suffix}"
        assert hand_ik.chain_count == 2
        assert foot_ik is not None
        assert foot_ik.target == armature
        assert foot_ik.subtarget == f"CTRL_Foot.{suffix}"
        assert foot_ik.pole_subtarget == f"CTRL_Knee.{suffix}"
        assert foot_ik.chain_count == 2

    hand_before = armature.pose.bones["Hand.L"].head.copy()
    hand_control = armature.pose.bones["CTRL_Hand.L"]
    hand_control["ik_fk"] = 1.0
    hand_control.location.z += 0.22
    bpy.context.scene.frame_set(2)
    bpy.context.view_layer.update()
    assert armature.pose.bones["ForeArm.L"].constraints["IP_Hand_IK"].influence > 0.99
    assert (armature.pose.bones["Hand.L"].head - hand_before).length > 0.05


def test_sloth_face_deforms_original_mesh_for_nine_visemes() -> None:
    character_objects, _, dimensions, armature, _, bone_map = load_rigged_character()
    original_mesh_objects = {obj.name for obj in bpy.data.objects if obj.type == "MESH"}
    original_topology = {
        obj.name: (len(obj.data.vertices), len(obj.data.polygons))
        for obj in character_objects
    }
    face = blender_renderer.setup_face(
        {
            "characterId": "main_ip_sloth",
            "faceScreenMode": "source",
            "mouthMode": "source_mesh_visemes",
            "mouthHeightRatio": 0.805,
            "mouthScale": 1.0,
            "mouthStyle": "source_mesh",
        },
        dimensions,
        armature,
        character_objects,
        bone_map,
    )

    mouth = face["mouth"]
    assert mouth in character_objects
    assert {obj.name for obj in bpy.data.objects if obj.type == "MESH"} == original_mesh_objects
    assert not any(obj.name.startswith("IP_Mouth") for obj in bpy.data.objects)
    assert (len(mouth.data.vertices), len(mouth.data.polygons)) == original_topology[mouth.name]
    shape_names = {key.name for key in mouth.data.shape_keys.key_blocks}
    expected = {
        "Mouth_Rest",
        "Mouth_A",
        "Mouth_E",
        "Mouth_O",
        "Mouth_U",
        "Mouth_MBP",
        "Mouth_Smile",
        "Mouth_Frown",
        "Mouth_Surprise",
    }
    assert expected.issubset(shape_names)
    assert "patch" not in face
    assert mouth["mouth_style"] == "source_mesh"
    assert mouth["source_mouth_removed"] is False
    assert mouth["source_mouth_deformed"] is True
    assert mouth["mouth_vertex_count"] >= 40
    assert mouth.data.color_attributes.get("IP_Mouth_Mask") is not None
    mouth_material = next(
        material
        for material in mouth.data.materials
        if material and material.node_tree and material.node_tree.nodes.get("IP_Mouth_Open_Mix")
    )
    assert mouth_material.node_tree.nodes.get("IP_Mouth_Open_Value") is not None
    assert mouth_material.node_tree.animation_data is not None
    assert mouth_material.node_tree.animation_data.drivers
    keys = mouth.data.shape_keys.key_blocks
    basis = keys["Basis"]
    rest = keys["Mouth_Rest"]
    open_a = keys["Mouth_A"]
    assert all((basis.data[index].co - rest.data[index].co).length < 1e-8 for index in range(len(basis.data)))
    assert sum(
        (basis.data[index].co - open_a.data[index].co).length > 1e-5
        for index in range(len(basis.data))
    ) >= 40
    open_a.value = 1.0
    bpy.context.scene.frame_set(2)
    bpy.context.view_layer.update()
    assert mouth_material.node_tree.nodes["IP_Mouth_Open_Value"].outputs[0].default_value > 0.8


def test_action_library_contains_talking_gestures_and_expressions() -> None:
    character_objects, _, dimensions, armature, _, bone_map = load_rigged_character()
    face = blender_renderer.setup_face(
        {
            "characterId": "main_ip_sloth",
            "faceScreenMode": "source",
            "mouthMode": "source_mesh_visemes",
            "mouthHeightRatio": 0.805,
            "mouthScale": 1.0,
            "mouthStyle": "source_mesh",
        },
        dimensions,
        armature,
        character_objects,
        bone_map,
    )

    report = blender_renderer.create_action_library(armature, face, bone_map, fps=30)

    expected = {
        "Idle_Speaking",
        "Talk_Loop",
        "Gesture_Wave",
        "Gesture_Explain",
        "Gesture_OpenArms",
        "Gesture_Point_Left",
        "Gesture_Point_Right",
        "Gesture_Present_Left",
        "Gesture_Present_Right",
        "Gesture_Shrug",
        "Gesture_Think",
        "Gesture_Step",
        "Gesture_Nod",
        "Gesture_ShakeHead",
        "Gesture_Emphasis",
        "Look_Camera",
        "Look_Left",
        "Look_Right",
        "Expression_Happy",
        "Expression_Thinking",
        "Expression_Surprised",
        "Expression_Confused",
        "Expression_Serious",
    }
    assert expected.issubset(set(report["actions"]))

    armature.animation_data.action = bpy.data.actions["Gesture_Wave"]
    bpy.context.scene.frame_set(15)
    forearm = armature.pose.bones[bone_map["forearm_r"]].rotation_euler
    hand = armature.pose.bones[bone_map["hand_r"]].rotation_euler
    fingers = [
        armature.pose.bones[bone_map[f"finger_{digit}_r"]].rotation_euler
        for digit in (1, 2, 3)
    ]
    assert max(abs(value) for value in forearm) > 0.50
    assert hand.y < -0.90
    assert all(abs(finger.z) < 0.04 for finger in fingers)
    assert fingers[0].x > 0.10
    assert fingers[2].x < -0.10


def test_talking_timeline_animates_multiaxis_hands_fingers_jaw_and_source_mouth() -> None:
    character_objects, _, dimensions, armature, _, bone_map = load_rigged_character()
    face = blender_renderer.setup_face(
        {
            "characterId": "main_ip_sloth",
            "faceScreenMode": "source",
            "mouthMode": "source_mesh_visemes",
            "mouthHeightRatio": 0.805,
            "mouthScale": 1.0,
            "mouthStyle": "source_mesh",
        },
        dimensions,
        armature,
        character_objects,
        bone_map,
    )
    plan = {
        "durationSec": 2.0,
        "motionEvents": [
            {"timeSec": 0.0, "motion": "wave", "duration": 1.2, "strength": 1.0},
            {"timeSec": 0.4, "motion": "present", "duration": 1.0, "strength": 0.65},
        ],
        "lipSync": [{"timeSec": 0.0, "viseme": "a", "open": 0.9}],
    }

    blender_renderer.animate(armature, face, plan, fps=30, bone_map=bone_map)

    bpy.context.scene.frame_set(15)
    forearm = armature.pose.bones[bone_map["forearm_r"]].rotation_euler
    hand = armature.pose.bones[bone_map["hand_r"]].rotation_euler
    fingers = [
        armature.pose.bones[bone_map[f"finger_{digit}_r"]].rotation_euler
        for digit in (1, 2, 3)
    ]
    jaw = armature.pose.bones[bone_map["jaw"]].rotation_euler
    assert abs(forearm.y) > 0.05 or abs(forearm.z) > 0.05
    assert abs(hand.y) > 0.05 or abs(hand.z) > 0.05
    assert all(abs(finger.z) < 0.04 for finger in fingers)
    assert fingers[0].x > 0.10
    assert fingers[2].x < -0.10
    assert max(abs(value) for value in jaw) > 0.02
    assert face["mouth"].data.shape_keys.key_blocks["Mouth_A"].value > 0.5


def test_think_motion_event_drives_head_hand_and_finger_pose() -> None:
    _, _, _, armature, _, bone_map = load_rigged_character()
    plan = {
        "durationSec": 1.5,
        "motionEvents": [
            {"timeSec": 0.0, "motion": "think", "duration": 1.2, "strength": 1.0},
        ],
        "lipSync": [{"timeSec": 0.0, "viseme": "closed", "open": 0.0}],
    }

    blender_renderer.animate(armature, {}, plan, fps=30, bone_map=bone_map)

    bpy.context.scene.frame_set(15)
    head = armature.pose.bones[bone_map["head"]].rotation_euler
    forearm = armature.pose.bones[bone_map["forearm_r"]].rotation_euler
    hand = armature.pose.bones[bone_map["hand_r"]].rotation_euler
    finger = armature.pose.bones[bone_map["finger_2_r"]].rotation_euler
    assert abs(head.y) > 0.05 or abs(head.z) > 0.05
    assert forearm.x > 0.45
    assert abs(hand.y) > 0.10 or abs(hand.z) > 0.10
    assert abs(finger.z) > 0.15


def test_source_hand_events_raise_wrist_and_drive_individual_digits() -> None:
    _, _, armature, _, bone_map, _ = load_enhanced_fbx_character()
    plan = {
        "durationSec": 4.0,
        "motionEvents": [
            {"timeSec": 0.0, "motion": "open_hand", "duration": 0.8, "strength": 0.8},
            {"timeSec": 1.0, "motion": "wrist_twist", "duration": 0.8, "strength": 0.85},
            {"timeSec": 2.0, "motion": "finger_wave", "duration": 0.9, "strength": 0.85},
            {"timeSec": 3.0, "motion": "fist", "duration": 0.8, "strength": 0.75},
        ],
        "lipSync": [],
    }

    blender_renderer.animate(armature, {}, plan, fps=30, bone_map=bone_map)

    bpy.context.scene.frame_set(43)
    upper_arm = armature.pose.bones[bone_map["upper_arm_r"]].rotation_euler
    forearm = armature.pose.bones[bone_map["forearm_r"]].rotation_euler
    wrist = armature.pose.bones[bone_map["hand_r"]].rotation_euler
    assert upper_arm.z > -1.0
    assert forearm.z > 0.45
    assert abs(wrist.y) > 0.25

    bpy.context.scene.frame_set(67)
    curls = [
        armature.pose.bones[bone_map[f"finger_{digit}_r"]].rotation_euler.z
        for digit in (1, 2, 3)
    ]
    assert max(curls) > 0.10
    assert max(curls) - min(curls) > 0.04

    bpy.context.scene.frame_set(103)
    assert all(
        armature.pose.bones[bone_map[f"finger_{digit}_r"]].rotation_euler.z > 0.20
        for digit in (1, 2, 3)
    )


def test_overlapping_hand_events_share_one_arm_stage_pose() -> None:
    _, _, armature, _, bone_map, _ = load_enhanced_fbx_character()
    plan = {
        "durationSec": 1.0,
        "motionEvents": [
            {"timeSec": 0.0, "motion": "wrist_twist", "duration": 1.0, "strength": 1.0},
            {"timeSec": 0.0, "motion": "finger_wave", "duration": 1.0, "strength": 1.0},
        ],
        "lipSync": [],
    }

    blender_renderer.animate(armature, {}, plan, fps=30, bone_map=bone_map)

    bpy.context.scene.frame_set(15)
    upper_arm = armature.pose.bones[bone_map["upper_arm_r"]].rotation_euler
    forearm = armature.pose.bones[bone_map["forearm_r"]].rotation_euler
    assert -1.0 < upper_arm.z < -0.65, tuple(upper_arm)
    assert 0.45 < forearm.z < 0.90, tuple(forearm)


if __name__ == "__main__":
    tests = [
        test_generated_humanoid_rig_has_presenter_limbs_and_valid_weights,
        test_generated_rig_exposes_optional_hand_and_foot_ik_controls,
        test_sloth_face_deforms_original_mesh_for_nine_visemes,
        test_action_library_contains_talking_gestures_and_expressions,
        test_talking_timeline_animates_multiaxis_hands_fingers_jaw_and_source_mouth,
        test_think_motion_event_drives_head_hand_and_finger_pose,
        test_source_hand_events_raise_wrist_and_drive_individual_digits,
        test_overlapping_hand_events_share_one_arm_stage_pose,
        test_rigged_fbx_import_preserves_source_materials_and_removes_scene_helpers,
        test_rigged_fbx_gains_two_segment_three_digit_hands_with_valid_weights,
        test_rigged_fbx_face_retopologizes_original_mesh_without_visible_overlays,
        test_rigged_fbx_action_library_uses_source_axes_distal_fingers_and_rich_face,
        test_rigged_fbx_talking_timeline_uses_source_axes_distal_fingers_and_blink,
        test_publish_render_detail_is_non_destructive_and_deformation_aware,
        test_talking_timeline_uses_clamped_bezier_interpolation,
        test_exported_glb_reimport_keeps_source_humanoid_axis_profile,
    ]
    for test in tests:
        test()
        print(f"PASS {test.__name__}")
