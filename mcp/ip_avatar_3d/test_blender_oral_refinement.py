#!/usr/bin/env python3
"""Blender contract tests for the continuous oral geometry refinement."""

from __future__ import annotations

import sys
from pathlib import Path

try:
    import bpy
except ModuleNotFoundError as exc:
    raise RuntimeError("requires Blender's bpy runtime") from exc

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import blender_renderer
import oral_refinement


def mesh_component_count(obj) -> int:
    adjacency = {vertex.index: set() for vertex in obj.data.vertices}
    for edge in obj.data.edges:
        adjacency[edge.vertices[0]].add(edge.vertices[1])
        adjacency[edge.vertices[1]].add(edge.vertices[0])
    remaining = set(adjacency)
    components = 0
    while remaining:
        components += 1
        stack = [remaining.pop()]
        while stack:
            current = stack.pop()
            neighbors = adjacency[current] & remaining
            remaining.difference_update(neighbors)
            stack.extend(neighbors)
    return components


def build_oral_scene():
    bpy.ops.wm.read_factory_settings(use_empty=True)
    scene = bpy.context.scene

    armature_data = bpy.data.armatures.new("OralFixtureRigData")
    armature = bpy.data.objects.new("OralFixtureRig", armature_data)
    scene.collection.objects.link(armature)
    bpy.context.view_layer.objects.active = armature
    armature.select_set(True)
    bpy.ops.object.mode_set(mode="EDIT")
    try:
        head = armature_data.edit_bones.new("Head")
        head.head = (0.0, 0.0, 1.45)
        head.tail = (0.0, 0.0, 2.05)
        jaw = armature_data.edit_bones.new("Jaw")
        jaw.head = (0.0, 0.0, 1.62)
        jaw.tail = (0.0, -0.08, 1.45)
        jaw.parent = head
        parent = jaw
        for index in range(1, 4):
            tongue = armature_data.edit_bones.new(f"Tongue_{index:02d}")
            tongue.head = (0.0, -0.02 * (index - 1), 1.62)
            tongue.tail = (0.0, -0.02 * index, 1.62)
            tongue.parent = parent
            tongue.use_connect = index > 1
            parent = tongue
    finally:
        bpy.ops.object.mode_set(mode="OBJECT")

    mesh = bpy.data.meshes.new("OralFixtureFaceMesh")
    mesh.from_pydata(
        [(-0.3, 0.0, 1.5), (0.3, 0.0, 1.5), (0.0, 0.0, 1.9)],
        [],
        [(0, 1, 2)],
    )
    source_face = bpy.data.objects.new("OralFixtureFace", mesh)
    scene.collection.objects.link(source_face)
    source_face["mouth_center_x"] = 0.0
    source_face["mouth_center_z"] = 1.65
    source_face["mouth_surface_y"] = 0.0
    source_face["mouth_radius_x"] = 0.22
    source_face["head_region_depth"] = 0.34

    bone_map = {
        "head": "Head",
        "jaw": "Jaw",
        "tongue_1": "Tongue_01",
        "tongue_2": "Tongue_02",
        "tongue_3": "Tongue_03",
    }
    dimensions = {"height": 2.5, "width": 1.1, "depth": 0.62}
    return source_face, armature, bone_map, dimensions


def build_oral_fixture():
    source_face, armature, bone_map, dimensions = build_oral_scene()
    result = oral_refinement.create_refined_oral_interior(
        source_face,
        armature,
        bone_map,
        dimensions,
        material_factory=blender_renderer.material,
        bind_object=blender_renderer._bind_internal_face_object,
    )
    return result, armature


def test_dental_arches_are_connected_and_follow_expected_bones():
    result, armature = build_oral_fixture()
    assert set(result) == {
        "oral_cavity", "upper_teeth", "lower_teeth", "upper_gum", "lower_gum", "tongue",
    }
    assert mesh_component_count(result["upper_teeth"]) == 1
    assert mesh_component_count(result["lower_teeth"]) == 1
    assert set(group.name for group in result["upper_teeth"].vertex_groups) == {"Head"}
    assert set(group.name for group in result["lower_teeth"].vertex_groups) == {"Jaw"}
    assert result["upper_teeth"].modifiers["IP_Face_Armature"].object == armature
    assert result["lower_teeth"].modifiers["IP_Face_Armature"].object == armature


def test_tongue_is_connected_tapered_and_weighted_to_three_bones():
    result, _ = build_oral_fixture()
    tongue = result["tongue"]
    assert mesh_component_count(tongue) == 1
    assert {group.name for group in tongue.vertex_groups} == {"Tongue_01", "Tongue_02", "Tongue_03"}
    assert tongue["ip_tongue_longitudinal_rings"] >= 9
    assert tongue["ip_tongue_tip_width_ratio"] < 0.72
    assert tongue["ip_tongue_center_groove"] is True


def quad_face_count(obj) -> int:
    return sum(len(polygon.vertices) == 4 for polygon in obj.data.polygons)


def test_dental_arches_and_tongue_use_multi_ring_rounded_end_caps():
    result, _ = build_oral_fixture()
    expected_body_rings = {
        "upper_teeth": (13, 8),
        "lower_teeth": (13, 8),
        "tongue": (11, 12),
    }
    for role, (body_rings, cross_sections) in expected_body_rings.items():
        obj = result[role]
        assert quad_face_count(obj) >= (body_rings - 1 + 4) * cross_sections


def create_stale_oral_object(name: str, role: str):
    mesh = bpy.data.meshes.new(f"{name}_Mesh")
    mesh.from_pydata([(0.0, 0.0, 0.0), (0.01, 0.0, 0.0), (0.0, 0.01, 0.0)], [], [(0, 1, 2)])
    obj = bpy.data.objects.new(name, mesh)
    bpy.context.scene.collection.objects.link(obj)
    obj["ip_face_topology_role"] = role
    obj["ip_stale_oral_fixture"] = True
    return obj


def test_current_refined_oral_roles_are_reused_without_regeneration():
    source_face, armature, bone_map, dimensions = build_oral_scene()
    initial = blender_renderer.create_integrated_oral_interior(source_face, armature, bone_map, dimensions)
    reused = blender_renderer.create_integrated_oral_interior(source_face, armature, bone_map, dimensions)
    assert {role: obj.name for role, obj in reused.items()} == {
        role: obj.name for role, obj in initial.items()
    }


def test_previous_version_oral_roles_are_replaced_after_geometry_contract_bump():
    previous_version = 1
    source_face, armature, bone_map, dimensions = build_oral_scene()
    legacy = blender_renderer.create_integrated_oral_interior(source_face, armature, bone_map, dimensions)
    for obj in legacy.values():
        obj["ip_oral_refinement_version"] = previous_version
        obj["ip_previous_oral_version_fixture"] = True

    regenerated = blender_renderer.create_integrated_oral_interior(source_face, armature, bone_map, dimensions)

    assert oral_refinement.ORAL_REFINEMENT_VERSION > previous_version
    assert not any(obj.get("ip_previous_oral_version_fixture") for obj in bpy.context.scene.objects)
    assert all(
        obj["ip_oral_refinement_version"] == oral_refinement.ORAL_REFINEMENT_VERSION
        for obj in regenerated.values()
    )


def test_stale_duplicate_oral_roles_are_all_removed_before_regeneration():
    source_face, armature, bone_map, dimensions = build_oral_scene()
    create_stale_oral_object("LegacyUpperTeethA", "upper_teeth")
    create_stale_oral_object("LegacyUpperTeethB", "upper_teeth")
    create_stale_oral_object("LegacyTongueA", "tongue")
    create_stale_oral_object("LegacyTongueB", "tongue")

    result = blender_renderer.create_integrated_oral_interior(source_face, armature, bone_map, dimensions)

    assert not any(obj.get("ip_stale_oral_fixture") for obj in bpy.context.scene.objects)
    for role, obj in result.items():
        matching = [
            candidate
            for candidate in bpy.context.scene.objects
            if candidate.get("ip_face_topology_role") == role
        ]
        assert matching == [obj]


if __name__ == "__main__":
    tests = (
        test_dental_arches_are_connected_and_follow_expected_bones,
        test_tongue_is_connected_tapered_and_weighted_to_three_bones,
        test_dental_arches_and_tongue_use_multi_ring_rounded_end_caps,
        test_current_refined_oral_roles_are_reused_without_regeneration,
        test_previous_version_oral_roles_are_replaced_after_geometry_contract_bump,
        test_stale_duplicate_oral_roles_are_all_removed_before_regeneration,
    )
    failures = []
    for test in tests:
        try:
            test()
            print(f"PASS {test.__name__}")
        except AssertionError as exc:
            failures.append((test, exc))
            print(f"FAIL {test.__name__}: {exc}")
    if failures:
        print(f"FAIL {len(failures)} oral-refinement test(s) failed")
        sys.exit(1)
