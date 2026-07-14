#!/usr/bin/env python3
"""Blender integration tests for the unrigged anthropomorphic presenter path."""

from __future__ import annotations

import json
import math
import os
import sys
import tempfile
import traceback
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


def hand_relative_tip(armature, bone_map, side: str, digit: int):
    hand = armature.pose.bones[bone_map[f"hand_{side}"]]
    distal = armature.pose.bones[bone_map[f"finger_{digit}_tip_{side}"]]
    armature_space = hand.matrix.inverted() @ distal.tail
    return armature.matrix_world.to_3x3() @ armature_space


def hand_relative_digit_length(armature, bone_map, side: str, digit: int) -> float:
    world_scale = armature.matrix_world.to_3x3()
    roles = (
        f"finger_{digit}_{side}",
        f"finger_{digit}_mid_{side}",
        f"finger_{digit}_tip_{side}",
    )
    length = 0.0
    for role in roles:
        bone = armature.data.bones[bone_map[role]]
        length += (world_scale @ (bone.tail_local - bone.head_local)).length
    return length


def object_property_indices(obj, name: str) -> list[int]:
    value = obj.get(name)
    assert isinstance(value, str) and value, name
    indices = json.loads(value)
    assert isinstance(indices, list) and indices, name
    return [int(index) for index in indices]


def shape_key_world_coordinates(obj, shape_name: str, indices: list[int]):
    key = obj.data.shape_keys.key_blocks[shape_name]
    return [obj.matrix_world @ key.data[index].co for index in indices]


def vertex_uv_coordinates(obj, layer_name: str, vertex_index: int) -> list[list[float]]:
    layer = obj.data.uv_layers[layer_name]
    coordinates = [
        [float(layer.data[loop_index].uv.x), float(layer.data[loop_index].uv.y)]
        for polygon in obj.data.polygons
        for loop_index in polygon.loop_indices
        if int(obj.data.loops[loop_index].vertex_index) == vertex_index
    ]
    return sorted(coordinates)


def color_attribute_signature(obj) -> list[dict[str, str]]:
    return sorted(
        (
            {
                "name": attribute.name,
                "data_type": attribute.data_type,
                "domain": attribute.domain,
            }
            for attribute in obj.data.color_attributes
        ),
        key=lambda item: (item["name"], item["domain"], item["data_type"]),
    )


def mesh_attribute_signature(obj) -> list[dict[str, str]]:
    return sorted(
        (
            {
                "name": attribute.name,
                "data_type": attribute.data_type,
                "domain": attribute.domain,
            }
            for attribute in obj.data.attributes
            if not attribute.name.startswith(".") and attribute.name != "material_index"
        ),
        key=lambda item: (item["name"], item["domain"], item["data_type"]),
    )


def assert_task6_eye_metadata(
    face_mesh,
    *,
    verify_modified_vertices: bool = True,
    verify_mesh_attributes: bool = True,
) -> None:
    if face_mesh.get("blink_capability") == "squint_only":
        assert face_mesh["true_eyelid_topology"] is False
        assert face_mesh["eyelid_topology_mode"] == "squint_only_source_skin"
        assert face_mesh["eye_region_subdivision_level"] == 0
        assert face_mesh["eye_region_subdivision_added_vertices"] == 0
        assert face_mesh["eye_region_boundary_fixed"] is True
        assert face_mesh["eye_region_uv_preserved"] is True
        assert face_mesh["eye_region_deform_weights_preserved"] is True
        assert 0.10 <= face_mesh["squint_max_closure_fraction"] <= 0.14

        custom_signature = json.loads(str(face_mesh["eye_region_custom_data_signature"]))
        assert custom_signature["uv_layers"] == [layer.name for layer in face_mesh.data.uv_layers]
        assert custom_signature["color_attributes"] == color_attribute_signature(face_mesh)
        if verify_mesh_attributes:
            assert custom_signature["attributes"] == mesh_attribute_signature(face_mesh)
        modified = json.loads(str(face_mesh["eye_region_modified_uv_data"]))
        assert modified["new_lid_vertex_count"] == 0
        assert modified["all_finite"] is True
        assert modified["all_within_source_bounds"] is True
        assert set(modified["sides"]) == {"l", "r"}
        deform = json.loads(str(face_mesh["eye_region_deform_evidence"]))
        assert deform["new_lid_vertex_count"] == 0
        assert deform["squint_vertex_count"] > 0
        assert deform["preserved_original_vertex_count"] == deform["original_vertex_count"]

        keys = face_mesh.data.shape_keys.key_blocks
        assert keys.get("Eye_Squint.L") is not None
        assert keys.get("Eye_Squint.R") is not None
        assert keys.get("Eye_Blink.L") is None
        assert keys.get("Eye_Blink.R") is None
        basis = keys["Basis"]
        side_regions = {}
        for side in ("l", "r"):
            core = set(object_property_indices(face_mesh, f"eyeball_core_indices_{side}"))
            skin = set(object_property_indices(face_mesh, f"squint_skin_indices_{side}"))
            upper = set(object_property_indices(face_mesh, f"squint_upper_indices_{side}"))
            lower = set(object_property_indices(face_mesh, f"squint_lower_indices_{side}"))
            assert core
            assert skin
            assert upper.union(lower) == skin
            assert upper.isdisjoint(lower)
            assert skin.isdisjoint(core)
            assert face_mesh[f"squint_eye_weight_zero_{side}"] is True
            assert face_mesh[f"eyeball_core_excluded_{side}"] is True
            assert face_mesh[f"squint_non_skin_max_displacement_{side}"] <= 1e-9
            assert face_mesh[f"squint_core_max_displacement_{side}"] <= 1e-9
            eye_group = face_mesh.vertex_groups[f"Eye.{side.upper()}"]
            shape = keys[f"Eye_Squint.{side.upper()}"]
            moved = {
                index
                for index in range(len(basis.data))
                if (shape.data[index].co - basis.data[index].co).length > 1e-8
            }
            assert moved
            stored_skin_uvs = {
                int(item["vertex"]): item["uvs"]
                for item in modified["sides"][side]["squint_skin_uv_data"]
            }
            for index in (skin if verify_modified_vertices else moved):
                try:
                    eye_weight = eye_group.weight(index)
                except RuntimeError:
                    eye_weight = 0.0
                assert eye_weight <= 1e-8
            if verify_modified_vertices:
                assert len(moved) <= len(skin)
                assert set(stored_skin_uvs) == skin
                center_x = float(face_mesh[f"squint_center_x_{side}"])
                center_z = float(face_mesh[f"squint_center_z_{side}"])
                radius_x = float(face_mesh[f"squint_radius_x_{side}"])
                radius_z = float(face_mesh[f"squint_radius_z_{side}"])
                adjacency = {vertex.index: set() for vertex in face_mesh.data.vertices}
                for edge in face_mesh.data.edges:
                    left, right = (int(value) for value in edge.vertices)
                    adjacency[left].add(right)
                    adjacency[right].add(left)
                first_skin_ring = {
                    neighbor
                    for index in core
                    for neighbor in adjacency[index]
                    if neighbor not in core
                }
                second_skin_ring = {
                    neighbor
                    for index in first_skin_ring
                    for neighbor in adjacency[index]
                    if neighbor not in core and neighbor not in first_skin_ring
                }
                assert skin.issubset(first_skin_ring.union(second_skin_ring))
                for index in skin:
                    assert stored_skin_uvs[index] == vertex_uv_coordinates(
                        face_mesh,
                        face_mesh.data.uv_layers.active.name,
                        index,
                    )
                    world = face_mesh.matrix_world @ basis.data[index].co
                    normalized_x = (float(world.x) - center_x) / radius_x
                    normalized_z = (float(world.z) - center_z) / radius_z
                    assert math.hypot(normalized_x, normalized_z) <= 1.30 + 1e-6
                    assert normalized_z > 0.0 if index in upper else normalized_z < 0.0
                assert max(
                    (shape.data[index].co - basis.data[index].co).length for index in core
                ) <= 1e-9
                assert max(
                    (shape.data[index].co - basis.data[index].co).length
                    for index in range(len(basis.data))
                    if index not in skin
                ) <= 1e-9
            else:
                expected_skin_loops = [
                    uv
                    for coordinates in stored_skin_uvs.values()
                    for uv in coordinates
                ]
                assert len(moved) <= len(expected_skin_loops)
                for index in moved:
                    actual_uvs = vertex_uv_coordinates(
                        face_mesh,
                        face_mesh.data.uv_layers.active.name,
                        index,
                    )
                    assert actual_uvs
                    assert all(
                        any(
                            all(
                                abs(actual[axis] - expected[axis]) <= 5e-5
                                for axis in (0, 1)
                            )
                            for expected in expected_skin_loops
                        )
                        for actual in actual_uvs
                    )

            stored_core_uvs = {
                int(item["vertex"]): item["uvs"]
                for item in json.loads(str(face_mesh[f"eyeball_core_uv_data_{side}"]))
            }
            if verify_modified_vertices:
                assert set(stored_core_uvs) == core
                for index in core:
                    assert stored_core_uvs[index] == vertex_uv_coordinates(
                        face_mesh,
                        face_mesh.data.uv_layers.active.name,
                        index,
                    )
            else:
                imported_core_uvs = []
                for index in range(len(face_mesh.data.vertices)):
                    try:
                        eye_weight = eye_group.weight(index)
                    except RuntimeError:
                        eye_weight = 0.0
                    if eye_weight > 0.015:
                        imported_core_uvs.extend(
                            vertex_uv_coordinates(
                                face_mesh,
                                face_mesh.data.uv_layers.active.name,
                                index,
                            )
                        )
                expected_core_uvs = [
                    uv
                    for coordinates in stored_core_uvs.values()
                    for uv in coordinates
                ]
                assert imported_core_uvs
                assert expected_core_uvs
                assert all(
                    math.isfinite(value)
                    for uv in (*imported_core_uvs, *expected_core_uvs)
                    for value in uv
                )
            side_regions[side] = skin if verify_modified_vertices else moved

        assert side_regions["l"].isdisjoint(side_regions["r"])
        assert not any(
            material and material.name.startswith(("IP_EyelidSkin.", "IP_EyelidMargin."))
            for material in face_mesh.data.materials
        )
        assert face_mesh["source_pbr_materials_tuned"] is True
        assert json.loads(str(face_mesh["source_pbr_material_names"]))
        role_metadata = json.loads(str(face_mesh["source_pbr_role_metadata"]))
        assert set(role_metadata) == {"base_color", "metallic", "normal", "roughness"}
        return

def mouth_open_gap(face_mesh, shape_name: str) -> float:
    upper = shape_key_world_coordinates(
        face_mesh,
        shape_name,
        object_property_indices(face_mesh, "mouth_upper_boundary_indices"),
    )
    lower = shape_key_world_coordinates(
        face_mesh,
        shape_name,
        object_property_indices(face_mesh, "mouth_lower_boundary_indices"),
    )
    return max(float(point.z) for point in upper) - min(float(point.z) for point in lower)


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
    source_material = next(
        material
        for obj in character_objects
        for material in obj.data.materials
        if material
        and material.use_nodes
        and {
            "texture_pbr_20250901.png",
            "texture_pbr_20250901_metallic.png",
            "texture_pbr_20250901_normal.png",
            "texture_pbr_20250901_roughness.png",
        }.issubset(
            {
                node.image.name
                for node in material.node_tree.nodes
                if node.type == "TEX_IMAGE" and node.image
            }
        )
    )
    roles = blender_renderer._resolve_source_pbr_texture_roles(source_material)
    assert {role: node.image.name for role, node in roles.items()} == {
        "base_color": "texture_pbr_20250901.png",
        "metallic": "texture_pbr_20250901_metallic.png",
        "normal": "texture_pbr_20250901_normal.png",
        "roughness": "texture_pbr_20250901_roughness.png",
    }


def test_rigged_fbx_gains_three_segment_three_digit_hands_with_valid_weights() -> None:
    character_objects, _, armature, rig_stats, bone_map, _ = load_enhanced_fbx_character()
    deform_bones = {bone.name for bone in armature.data.bones if bone.use_deform}
    expected = {
        f"Finger_{digit:02d}_{segment}.{side}"
        for side in ("L", "R")
        for digit in (1, 2, 3)
        for segment in ("Proximal", "Middle", "Distal")
    }

    assert expected.issubset(deform_bones)
    assert rig_stats["fingerRigEnhanced"] is True
    assert rig_stats["fingerBoneCount"] == 18
    assert rig_stats["fingerTopologyMode"] == "source_surface_weighted"
    assert rig_stats["handDetailAddedVertices"] == 0
    assert rig_stats["handDetailVertexCountAfter"] == rig_stats["handDetailVertexCountBefore"]
    assert rig_stats["handWeightedJointBandCount"] == 12
    assert len(rig_stats["handWeightedJointBandVertexCounts"]) == 12
    assert all(count > 0 for count in rig_stats["handWeightedJointBandVertexCounts"].values())
    assert rig_stats["fingerWeightingMode"] == "isolated_digit_banded"
    assert rig_stats["fingerBlendVertexCount"] > 0
    assert rig_stats["preserveVolumeSkinning"] is True
    assert bone_map["finger_1_l"] == "Finger_01_Proximal.L"
    assert bone_map["finger_1_mid_l"] == "Finger_01_Middle.L"
    assert bone_map["finger_1_tip_l"] == "Finger_01_Distal.L"
    assert bone_map["finger_3_mid_r"] == "Finger_03_Middle.R"
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
    assert face_mesh["true_eyelid_topology"] is False
    assert face_mesh["eyelid_topology_mode"] == "squint_only_source_skin"
    assert face_mesh["blink_capability"] == "squint_only"
    assert face_mesh["eye_region_subdivision_level"] == 0
    assert face_mesh["eye_region_subdivision_added_vertices"] == 0
    assert face_mesh["eye_region_boundary_fixed"] is True
    assert face_mesh["eye_region_uv_preserved"] is True
    assert 0.10 <= face_mesh["squint_max_closure_fraction"] <= 0.14
    for side in ("l", "r"):
        assert len(object_property_indices(face_mesh, f"squint_upper_indices_{side}")) >= 12
        assert len(object_property_indices(face_mesh, f"squint_lower_indices_{side}")) >= 12

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
        "Eye_Squint.L", "Eye_Squint.R", "Eye_Wide.L", "Eye_Wide.R",
        "Eye_Look_Left", "Eye_Look_Right", "Brow_Raise.L", "Brow_Raise.R",
        "Brow_Furrow.L", "Brow_Furrow.R", "Cheek_Smile.L", "Cheek_Smile.R",
        "Cheek_Puff.L", "Cheek_Puff.R", "Nose_Flare.L", "Nose_Flare.R",
    }
    assert expected.issubset(names), sorted(expected.difference(names))
    assert {"Eye_Blink.L", "Eye_Blink.R"}.isdisjoint(names)
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
    squint = face_mesh.data.shape_keys.key_blocks["Eye_Squint.L"]
    assert sum(
        (basis.data[index].co - squint.data[index].co).length > 1e-5
        for index in range(len(basis.data))
    ) >= 8

    for side in ("l", "r"):
        boundary = object_property_indices(face_mesh, f"eye_region_boundary_indices_{side}")
        stored_boundary = json.loads(str(face_mesh[f"eye_region_boundary_basis_coordinates_{side}"]))
        assert {int(item[0]) for item in stored_boundary} == set(boundary)
        for index, coordinate in stored_boundary:
            assert all(
                abs(float(basis.data[int(index)].co[axis]) - float(coordinate[axis])) < 1e-8
                for axis in range(3)
            )
        own_squint = face_mesh.data.shape_keys.key_blocks[f"Eye_Squint.{side.upper()}"]
        other_squint = face_mesh.data.shape_keys.key_blocks[
            f"Eye_Squint.{'R' if side == 'l' else 'L'}"
        ]
        active = object_property_indices(face_mesh, f"squint_skin_indices_{side}")
        eye_group = face_mesh.vertex_groups[f"Eye.{side.upper()}"]
        for index in active:
            try:
                eye_weight = eye_group.weight(index)
            except RuntimeError:
                eye_weight = 0.0
            assert eye_weight <= 1e-8
        assert any((basis.data[index].co - own_squint.data[index].co).length > 1e-5 for index in active)
        assert all((basis.data[index].co - other_squint.data[index].co).length < 1e-8 for index in active)

    uv_evidence = json.loads(str(face_mesh["eye_region_uv_guard_data"]))
    assert uv_evidence["layer"] == face_mesh.data.uv_layers.active.name
    assert len(uv_evidence["vertices"]) >= 32
    for record in uv_evidence["vertices"]:
        actual = vertex_uv_coordinates(face_mesh, uv_evidence["layer"], int(record["vertex"]))
        expected_uvs = sorted(record["uvs"])
        assert len(actual) == len(expected_uvs)
        assert all(
            abs(actual[index][axis] - float(expected_uvs[index][axis])) < 1e-7
            for index in range(len(actual))
            for axis in range(2)
        )

    corner_indices = [
        index
        for index in (
            object_property_indices(face_mesh, "mouth_upper_boundary_indices")
            + object_property_indices(face_mesh, "mouth_lower_boundary_indices")
        )
        if abs(
            float((face_mesh.matrix_world @ basis.data[index].co).x)
            - float(face_mesh["mouth_center_x"])
        ) >= float(face_mesh["mouth_radius_x"]) * 0.70
    ]
    assert corner_indices
    maximum_corner_shift = float(face_mesh["head_region_width"]) * 0.0035
    for shape_name in ("Mouth_Smile", "Mouth_E", "Mouth_MBP"):
        shape = face_mesh.data.shape_keys.key_blocks[shape_name]
        lateral_shift = max(
            abs(
                float((face_mesh.matrix_world @ shape.data[index].co).x)
                - float((face_mesh.matrix_world @ basis.data[index].co).x)
            )
            for index in corner_indices
        )
        assert lateral_shift <= maximum_corner_shift, (shape_name, lateral_shift, maximum_corner_shift)
    assert mouth_open_gap(face_mesh, "Mouth_A") > mouth_open_gap(face_mesh, "Mouth_MBP") + dimensions["height"] * 0.003

    source_material = next(
        material
        for material in face_mesh.data.materials
        if material
        and material.use_nodes
        and material.node_tree.nodes.get("Image Texture - Base Color")
    )
    nodes = source_material.node_tree.nodes
    base_color = nodes.get("Image Texture - Base Color")
    metallic = nodes.get("Image Texture - Metallic")
    normal_image = nodes.get("Image Texture - Normal")
    roughness = nodes.get("Image Texture - Roughness")
    normal_map = nodes.get("Normal Map")
    principled = nodes.get("Principled BSDF")
    assert all((base_color, metallic, normal_image, roughness, normal_map, principled))
    assert base_color.image.name == "texture_pbr_20250901.png"
    assert metallic.image.name == "texture_pbr_20250901_metallic.png"
    assert normal_image.image.name == "texture_pbr_20250901_normal.png"
    assert roughness.image.name == "texture_pbr_20250901_roughness.png"
    assert all(node.image.size[0] == 4096 and node.image.size[1] == 4096 for node in (base_color, metallic, normal_image, roughness))
    assert base_color.image.colorspace_settings.name == "sRGB"
    assert metallic.image.colorspace_settings.name == "Non-Color"
    assert normal_image.image.colorspace_settings.name == "Non-Color"
    assert roughness.image.colorspace_settings.name == "Non-Color"
    assert normal_image.outputs["Color"].is_linked
    assert normal_map.outputs["Normal"].is_linked
    assert metallic.outputs["Color"].is_linked
    assert roughness.outputs["Color"].is_linked
    assert principled.inputs["Metallic"].is_linked
    assert principled.inputs["Roughness"].is_linked
    assert principled.inputs["Normal"].is_linked
    assert 0.20 <= normal_map.inputs["Strength"].default_value <= 0.50
    specular_input = principled.inputs.get("Specular IOR Level") or principled.inputs.get("Specular")
    assert specular_input is not None and 0.20 <= specular_input.default_value <= 0.35
    assert not any(node.type in {"BUMP", "TEX_NOISE"} for node in nodes)
    assert source_material["ip_source_pbr_role_resolution"] == "socket_links"
    assert_task6_eye_metadata(face_mesh)


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
        "Face_Confused", "Face_Serious", "Face_Squint",
    }.issubset(report["faceActions"])
    assert "Face_Blink" not in report["faceActions"]

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


def test_existing_rich_face_without_task6_metadata_fails_closed() -> None:
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

    source["eyelid_topology_mode"] = "integrated_source_face"
    source["true_eyelid_topology"] = True
    source["blink_capability"] = "full"
    try:
        blender_renderer.validate_task6_face_metadata(source)
    except RuntimeError as exc:
        assert "full-blink metadata is no longer reusable" in str(exc)
    else:
        raise AssertionError("legacy full-blink metadata was accepted")

    source["eyelid_topology_mode"] = "squint_only_source_skin"
    source["true_eyelid_topology"] = False
    source["blink_capability"] = "squint_only"
    del source["squint_skin_indices_l"]

    try:
        blender_renderer.add_rich_source_face_shapes(source, dimensions)
    except RuntimeError as exc:
        assert "rebuild master from source FBX" in str(exc)
        assert "squint_skin_indices_l" in str(exc)
    else:
        raise AssertionError("incomplete Task 6 face metadata was silently reused")


def test_task6_reuse_recomputes_squint_and_pbr_evidence() -> None:
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

    def assert_rebuild_failure(expected_problem: str) -> None:
        try:
            blender_renderer.validate_task6_face_metadata(source)
        except RuntimeError as exc:
            message = str(exc)
            assert "rebuild master from source FBX" in message
            assert expected_problem in message, message
        else:
            raise AssertionError(f"Task 6 reuse accepted invalid {expected_problem}")

    center_x = float(source["squint_center_x_l"])
    del source["squint_center_x_l"]
    assert_rebuild_failure("squint_center_x_l")
    source["squint_center_x_l"] = center_x

    keys = source.data.shape_keys.key_blocks
    core_index = object_property_indices(source, "eyeball_core_indices_l")[0]
    original_coordinate = keys["Eye_Squint.L"].data[core_index].co.copy()
    keys["Eye_Squint.L"].data[core_index].co.x += 0.01
    assert source["squint_core_max_displacement_l"] <= 1e-9
    assert_rebuild_failure("squint_core_max_displacement_l")
    keys["Eye_Squint.L"].data[core_index].co = original_coordinate

    skin_index = object_property_indices(source, "squint_skin_indices_l")[0]
    eye_group = source.vertex_groups["Eye.L"]
    try:
        original_weight = eye_group.weight(skin_index)
    except RuntimeError:
        original_weight = None
    eye_group.add([skin_index], 0.25, "REPLACE")
    assert source["squint_eye_weight_zero_l"] is True
    assert_rebuild_failure("squint_eye_weight_zero_l")
    if original_weight is None:
        eye_group.remove([skin_index])
    else:
        eye_group.add([skin_index], original_weight, "REPLACE")

    source_material = next(
        material
        for material in source.data.materials
        if material
        and material.use_nodes
        and material.node_tree.nodes.get("Image Texture - Base Color")
    )
    source_material.node_tree.nodes.remove(
        source_material.node_tree.nodes["Image Texture - Base Color"]
    )
    assert source["source_pbr_materials_tuned"] is True
    assert_rebuild_failure("source_pbr_graph")


def task6_source_face_fixture():
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
    return face["mouth"]


def assert_task6_rebuild_failure(source, expected_problem: str) -> None:
    try:
        blender_renderer.validate_task6_face_metadata(source)
    except RuntimeError as exc:
        message = str(exc)
        assert "rebuild master from source FBX" in message
        assert expected_problem in message, message
    else:
        raise AssertionError(f"Task 6 reuse accepted invalid {expected_problem}")


def test_task6_reuse_rejects_lateral_active_skin_displacement() -> None:
    source = task6_source_face_fixture()
    skin_index = object_property_indices(source, "squint_skin_indices_l")[0]
    shape = source.data.shape_keys.key_blocks["Eye_Squint.L"]
    shape.data[skin_index].co.x += 0.25

    assert_task6_rebuild_failure(source, "squint_displacement_axis_l")


def test_task6_reuse_rejects_mutated_active_skin_uvs() -> None:
    source = task6_source_face_fixture()
    skin_index = object_property_indices(source, "squint_skin_indices_l")[0]
    loop_index = next(
        index
        for index, loop in enumerate(source.data.loops)
        if int(loop.vertex_index) == skin_index
    )
    source.data.uv_layers.active.data[loop_index].uv = (99.0, 99.0)

    assert_task6_rebuild_failure(source, "squint_skin_uv_data_l")


def test_task6_reuse_rejects_unrestrained_live_pbr_parameters() -> None:
    source = task6_source_face_fixture()
    source_material = next(
        material
        for material in source.data.materials
        if material and material.use_nodes and material.node_tree.nodes.get("Normal Map")
    )
    source_material.node_tree.nodes["Normal Map"].inputs["Strength"].default_value = 10.0

    assert_task6_rebuild_failure(source, "source_pbr_graph")


def test_source_asset_rejects_generated_full_lid_topology_before_creation() -> None:
    character_objects, dimensions, armature, _, bone_map, _ = load_enhanced_fbx_character()
    object_names_before = set(bpy.data.objects)
    material_names_before = set(bpy.data.materials)

    try:
        blender_renderer.setup_face(
            {
                "characterId": "main_ip_sloth",
                "faceScreenMode": "source",
                "mouthMode": "source_mesh_visemes",
                "mouthHeightRatio": 0.805,
                "mouthScale": 1.0,
                "facialDetailMode": "rich",
                "facialTopologyMode": "volumetric",
            },
            dimensions,
            armature,
            character_objects,
            bone_map,
        )
    except RuntimeError as exc:
        message = str(exc)
        assert "source_retopology" in message
        assert "squint_only" in message
    else:
        raise AssertionError("source asset accepted generated full-lid topology")

    generated_objects = set(bpy.data.objects).difference(object_names_before)
    generated_materials = set(bpy.data.materials).difference(material_names_before)
    assert not any(
        obj.name.startswith(("IP_UpperLid.", "IP_LowerLid."))
        or obj.get("ip_face_topology_role") in {
            "upper_lid_l", "lower_lid_l", "upper_lid_r", "lower_lid_r",
        }
        for obj in generated_objects
    )
    assert not any("Eyelid" in material.name for material in generated_materials)


def test_generic_character_retains_legacy_volumetric_topology_mode() -> None:
    character_objects, dimensions, armature, _, bone_map, _ = load_enhanced_fbx_character()
    blender_renderer.setup_face(
        {
            "characterId": "main_ip_sloth",
            "faceScreenMode": "source",
            "mouthMode": "source_mesh_visemes",
            "facialDetailMode": "rich",
            "facialTopologyMode": "source_retopology",
            "mouthHeightRatio": 0.805,
        },
        dimensions,
        armature,
        character_objects,
        bone_map,
    )

    face = blender_renderer.setup_face(
        {
            "characterId": "generic_fixture",
            "faceScreenMode": "source",
            "mouthMode": "existing_visemes",
            "facialDetailMode": "basic",
            "facialTopologyMode": "volumetric",
        },
        dimensions,
        armature,
        character_objects,
        bone_map,
    )

    assert set(blender_renderer.VOLUMETRIC_FACE_ROLES).issubset(face)
    assert face["mouth"]["facial_topology_mode"] == "volumetric"


def test_rigged_fbx_talking_timeline_uses_source_axes_distal_fingers_and_squint() -> None:
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
    assert keys["Eye_Squint.L"].value > 0.5
    assert keys["Eye_Squint.R"].value > 0.5
    assert keys.get("Eye_Blink.L") is None
    assert keys.get("Eye_Blink.R") is None
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
    character_objects, dimensions, armature, rig_stats, bone_map, removed = load_enhanced_fbx_character()
    face = blender_renderer.setup_face(
        {
            "characterId": "main_ip_sloth",
            "faceScreenMode": "source",
            "mouthMode": "source_mesh_visemes",
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
    assert_task6_eye_metadata(face["mouth"])

    with tempfile.TemporaryDirectory(prefix="ip-avatar-task6-") as temporary_directory:
        output = Path(temporary_directory)
        blender_renderer.save_rigged_assets(
            {
                "riggedBlendPath": str(output / "task6.blend"),
                "riggedGlbPath": str(output / "task6.glb"),
                "rigReportPath": str(output / "task6-report.json"),
                "rigMode": "auto",
                "faceScreenMode": "source",
                "mouthMode": "source_mesh_visemes",
                "facialTopologyMode": "source_retopology",
            },
            character_objects,
            [dimensions["container"]],
            armature,
            face,
            removed,
            rig_stats,
        )
        assert (output / "task6.glb").is_file()

        reset_scene()
        character_objects, armatures, imported_assets, _ = blender_renderer.import_model(
            str(output / "task6.glb")
        )
        dimensions = blender_renderer.prepare_character(
            character_objects,
            target_height=2.55,
            preserve_hierarchy=True,
            asset_objects=imported_assets,
        )
        weights_before = blender_renderer._collect_weight_stats(character_objects)
        armature, _, bone_map = blender_renderer.choose_character_rig(
            {"rigMode": "auto", "preserveExistingRig": True, "enhanceExistingRig": False},
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

        source = face["mouth"]
        assert_task6_eye_metadata(
            source,
            verify_modified_vertices=False,
            verify_mesh_attributes=False,
        )
        assert source["eye_region_subdivision_added_vertices"] == 0
        assert json.loads(str(source["eye_region_modified_uv_data"]))["new_lid_vertex_count"] == 0
        assert source["source_pbr_materials_tuned"] is True
        assert json.loads(str(source["source_pbr_role_metadata"]))

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
            assert weights_after["weightedVertexCounts"].get(bone_name, 0) == weights_before[
                "weightedVertexCounts"
            ].get(bone_name, 0)
        assert source.get("integrated_mouth_seam") is True
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


def test_enhanced_timeline_keys_three_segment_fist_and_isolates_finger_roll() -> None:
    _, _, armature, _, bone_map, _ = load_enhanced_fbx_character()
    fist_plan = {
        "durationSec": 1.0,
        "motionEvents": [{"timeSec": 0.0, "motion": "fist", "duration": 1.0, "strength": 1.0}],
        "lipSync": [],
    }
    blender_renderer.animate(armature, {}, fist_plan, fps=30, bone_map=bone_map)
    bpy.context.scene.frame_set(15)
    for digit in (1, 2, 3):
        proximal = armature.pose.bones[bone_map[f"finger_{digit}_r"]].rotation_euler
        middle = armature.pose.bones[bone_map[f"finger_{digit}_mid_r"]].rotation_euler
        distal = armature.pose.bones[bone_map[f"finger_{digit}_tip_r"]].rotation_euler
        assert 0.25 < proximal.z <= 0.30, (digit, tuple(proximal))
        assert 0.29 < middle.z <= 0.34, (digit, tuple(middle))
        assert 0.18 < distal.z <= 0.22, (digit, tuple(distal))

    roll_plan = {
        "durationSec": 1.0,
        "motionEvents": [{"timeSec": 0.0, "motion": "finger_wave", "duration": 1.0, "strength": 1.0}],
        "lipSync": [],
    }
    blender_renderer.animate(armature, {}, roll_plan, fps=30, bone_map=bone_map)
    bpy.context.scene.frame_set(1)
    before = {digit: hand_relative_tip(armature, bone_map, "r", digit).copy() for digit in (1, 2, 3)}
    bpy.context.scene.frame_set(15)
    selected = 2
    after = {digit: hand_relative_tip(armature, bone_map, "r", digit).copy() for digit in (1, 2, 3)}
    proximal = armature.pose.bones[bone_map[f"finger_{selected}_r"]].rotation_euler
    middle = armature.pose.bones[bone_map[f"finger_{selected}_mid_r"]].rotation_euler
    distal = armature.pose.bones[bone_map[f"finger_{selected}_tip_r"]].rotation_euler
    assert 0.26 < proximal.z <= 0.31, tuple(proximal)
    assert 0.31 < middle.z <= 0.35, tuple(middle)
    assert 0.19 < distal.z <= 0.23, tuple(distal)
    for digit in (1, 3):
        displacement = (after[digit] - before[digit]).length
        maximum = hand_relative_digit_length(armature, bone_map, "r", digit) * 0.06
        assert displacement <= maximum, (digit, displacement, maximum)

    point_plan = {
        "durationSec": 1.0,
        "motionEvents": [{"timeSec": 0.0, "motion": "point_right", "duration": 1.0, "strength": 1.0}],
        "lipSync": [],
    }
    blender_renderer.animate(armature, {}, point_plan, fps=30, bone_map=bone_map)
    bpy.context.scene.frame_set(15)
    pointed = [
        armature.pose.bones[bone_map[role]].rotation_euler
        for role in ("finger_1_r", "finger_1_mid_r", "finger_1_tip_r")
    ]
    assert all(abs(rotation.z) < 0.04 for rotation in pointed)
    curled = [
        armature.pose.bones[bone_map[role]].rotation_euler
        for role in ("finger_2_r", "finger_2_mid_r", "finger_2_tip_r")
    ]
    assert 0.25 < curled[0].z <= 0.30, tuple(curled[0])
    assert 0.29 < curled[1].z <= 0.34, tuple(curled[1])
    assert 0.18 < curled[2].z <= 0.22, tuple(curled[2])


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


def deliberate_direct_runner_failure() -> None:
    raise AssertionError("deliberate direct-runner failure")


if __name__ == "__main__":
    tests = [
        test_generated_humanoid_rig_has_presenter_limbs_and_valid_weights,
        test_generated_rig_exposes_optional_hand_and_foot_ik_controls,
        test_sloth_face_deforms_original_mesh_for_nine_visemes,
        test_action_library_contains_talking_gestures_and_expressions,
        test_talking_timeline_animates_multiaxis_hands_fingers_jaw_and_source_mouth,
        test_think_motion_event_drives_head_hand_and_finger_pose,
        test_source_hand_events_raise_wrist_and_drive_individual_digits,
        test_enhanced_timeline_keys_three_segment_fist_and_isolates_finger_roll,
        test_overlapping_hand_events_share_one_arm_stage_pose,
        test_rigged_fbx_import_preserves_source_materials_and_removes_scene_helpers,
        test_rigged_fbx_gains_three_segment_three_digit_hands_with_valid_weights,
        test_rigged_fbx_face_retopologizes_original_mesh_without_visible_overlays,
        test_rigged_fbx_action_library_uses_source_axes_distal_fingers_and_rich_face,
        test_existing_rich_face_without_task6_metadata_fails_closed,
        test_task6_reuse_recomputes_squint_and_pbr_evidence,
        test_task6_reuse_rejects_lateral_active_skin_displacement,
        test_task6_reuse_rejects_mutated_active_skin_uvs,
        test_task6_reuse_rejects_unrestrained_live_pbr_parameters,
        test_source_asset_rejects_generated_full_lid_topology_before_creation,
        test_generic_character_retains_legacy_volumetric_topology_mode,
        test_rigged_fbx_talking_timeline_uses_source_axes_distal_fingers_and_squint,
        test_publish_render_detail_is_non_destructive_and_deformation_aware,
        test_talking_timeline_uses_clamped_bezier_interpolation,
        test_exported_glb_reimport_keeps_source_humanoid_axis_profile,
    ]
    if os.environ.get("IP_AVATAR_FORCE_TEST_FAILURE") == "1":
        tests = [deliberate_direct_runner_failure]
    failures = 0
    for test in tests:
        try:
            test()
        except Exception:
            failures += 1
            print(f"FAIL {test.__name__}")
            traceback.print_exc()
        else:
            print(f"PASS {test.__name__}")
    if failures:
        print(f"FAILED {failures} direct-runner test(s)")
        sys.exit(1)
