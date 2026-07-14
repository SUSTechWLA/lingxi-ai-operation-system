#!/usr/bin/env python3
"""Geometry-based placement QA for the canonical character in the warm studio."""

from __future__ import annotations

import argparse
import json
import math
import sys
from contextlib import contextmanager
from pathlib import Path
from typing import Any, Iterable

import bpy
from bpy_extras.object_utils import world_to_camera_view
from mathutils import Vector
from mathutils.bvhtree import BVHTree

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import blender_renderer
from master_asset import append_master_collection


DEFAULT_SAMPLE_FRAMES = (1, 15, 29)
SOLE_BAND_HEIGHT_M = 0.005
DEFORMATION_SPIKE_RATIO = 3.0
DEFORMATION_SPIKE_DELTA_M = 0.02


def assert_render_armature_modifiers(
    character_objects: Iterable[bpy.types.Object],
    armature: bpy.types.Object,
) -> list[str]:
    """Require each discovered Armature modifier to participate in rendering."""
    checked: list[str] = []
    for obj in character_objects:
        if obj.type != "MESH":
            continue
        for modifier in obj.modifiers:
            if modifier.type != "ARMATURE":
                continue
            if modifier.object != armature:
                raise RuntimeError(
                    f"{obj.name}/{modifier.name} targets "
                    f"{getattr(modifier.object, 'name', None)!r}, expected {armature.name!r}"
                )
            if not modifier.show_render:
                raise RuntimeError(
                    f"shoe sole sampling requires {obj.name}/{modifier.name} "
                    "Armature modifier show_render=true"
                )
            checked.append(f"{obj.name}/{modifier.name}")
    if not checked:
        raise RuntimeError("shoe sole sampling found no Armature modifiers")
    return checked


def _evaluated_world_bvh(
    obj: bpy.types.Object,
    depsgraph: bpy.types.Depsgraph,
) -> BVHTree | None:
    geometry = _evaluated_world_bvh_geometry(obj, depsgraph)
    return geometry[0] if geometry else None


def _evaluated_world_bvh_geometry(
    obj: bpy.types.Object,
    depsgraph: bpy.types.Depsgraph,
) -> tuple[BVHTree, list[Vector], list[list[int]]] | None:
    if obj.type != "MESH" or obj.hide_render:
        return None
    evaluated = obj.evaluated_get(depsgraph)
    mesh = evaluated.to_mesh(preserve_all_data_layers=True, depsgraph=depsgraph)
    try:
        vertices = [evaluated.matrix_world @ vertex.co for vertex in mesh.vertices]
        mesh.calc_loop_triangles()
        polygons = [list(triangle.vertices) for triangle in mesh.loop_triangles]
        if not vertices or not polygons:
            return None
        tree = BVHTree.FromPolygons(vertices, polygons, all_triangles=True, epsilon=1e-6)
        return tree, vertices, polygons
    finally:
        evaluated.to_mesh_clear()


@contextmanager
def _render_modifier_state(objects: Iterable[bpy.types.Object]):
    states: list[tuple[bpy.types.Modifier, bool]] = []
    for obj in objects:
        if obj.type != "MESH":
            continue
        for modifier in obj.modifiers:
            states.append((modifier, bool(modifier.show_viewport)))
            modifier.show_viewport = bool(modifier.show_render)
    bpy.context.view_layer.update()
    try:
        yield
    finally:
        for modifier, show_viewport in states:
            modifier.show_viewport = show_viewport
        bpy.context.view_layer.update()


def count_evaluated_mesh_intersections(
    left_objects: Iterable[bpy.types.Object],
    right_objects: Iterable[bpy.types.Object],
    depsgraph: bpy.types.Depsgraph,
    camera: bpy.types.Object | None = None,
) -> dict[str, Any]:
    """Count overlapping evaluated polygon pairs between render-visible meshes."""
    left = [(obj, _evaluated_world_bvh_geometry(obj, depsgraph)) for obj in left_objects]
    right = [(obj, _evaluated_world_bvh(obj, depsgraph)) for obj in right_objects]
    object_pairs: list[list[str]] = []
    triangle_pair_count = 0
    for left_obj, left_geometry in left:
        if left_geometry is None:
            continue
        left_tree, vertices, triangles = left_geometry
        visible_triangles: set[int] | None = None
        if camera is not None:
            visible_triangles = set()
            for index, triangle in enumerate(triangles):
                projected = [
                    world_to_camera_view(bpy.context.scene, camera, vertices[vertex_index])
                    for vertex_index in triangle
                ]
                if any(
                    point.z > 0 and 0 <= point.x <= 1 and 0 <= point.y <= 1
                    for point in projected
                ):
                    visible_triangles.add(index)
        for right_obj, right_tree in right:
            if right_tree is None:
                continue
            overlaps = left_tree.overlap(right_tree)
            if visible_triangles is not None:
                overlaps = [pair for pair in overlaps if pair[0] in visible_triangles]
            if not overlaps:
                continue
            object_pairs.append([left_obj.name, right_obj.name])
            triangle_pair_count += len(overlaps)
    return {
        "trianglePairCount": triangle_pair_count,
        "objectPairs": object_pairs,
    }


def finalize_mode_report(report: dict[str, Any]) -> dict[str, Any]:
    """Apply the blocking Task 5 thresholds and attach deterministic reasons."""
    reasons: list[str] = []
    for side in ("left", "right"):
        clearance = float(report["floorClearance"][side]["minimum"])
        if clearance < -0.02:
            reasons.append(f"{side} foot clearance {clearance:.6f} m is below -0.020000 m")
    for key in (
        "deskIntersectionCount",
        "chairIntersectionCount",
        "handIntersectionCount",
    ):
        count = int(report[key])
        if count:
            reasons.append(f"{key}={count}")
    spike_count = int(report["deformationSpikeCount"])
    if spike_count:
        reasons.append(f"deformationSpikeCount={spike_count}")
    visibility = report["cameraVisibility"]
    for role in ("head", "leftHand", "rightHand"):
        if int(visibility[role]["insideCount"]) <= 0:
            reasons.append(f"{role} has no sampled geometry inside the medium frame")
    report["failureReasons"] = reasons
    report["success"] = not reasons
    return report


def _descendant_meshes(root: bpy.types.Object) -> list[bpy.types.Object]:
    return [
        obj
        for obj in (root, *root.children_recursive)
        if obj.type == "MESH" and not obj.hide_render
    ]


def _studio_collision_objects() -> tuple[list[bpy.types.Object], list[bpy.types.Object]]:
    desk_root = next(
        (obj for obj in bpy.data.objects if obj.get("assembly_role") == "main_desk"),
        None,
    )
    chair_root = bpy.data.objects.get("Chair_Main")
    if desk_root is None or chair_root is None:
        raise RuntimeError("warm studio is missing authored desk/chair collision assemblies")
    return _descendant_meshes(desk_root), _descendant_meshes(chair_root)


def _floor_height() -> tuple[str, float]:
    floor = bpy.data.objects.get("Studio_Floor")
    if floor is None or floor.type != "MESH" or floor.hide_render:
        raise RuntimeError("warm studio is missing render-visible Studio_Floor geometry")
    depsgraph = bpy.context.evaluated_depsgraph_get()
    evaluated = floor.evaluated_get(depsgraph)
    mesh = evaluated.to_mesh(preserve_all_data_layers=True, depsgraph=depsgraph)
    try:
        heights = [float((evaluated.matrix_world @ vertex.co).z) for vertex in mesh.vertices]
    finally:
        evaluated.to_mesh_clear()
    if not heights:
        raise RuntimeError("Studio_Floor has no evaluated vertices")
    return floor.name, max(heights)


def _vertex_weights(obj: bpy.types.Object, vertex: bpy.types.MeshVertex) -> dict[int, float]:
    return {assignment.group: float(assignment.weight) for assignment in vertex.groups}


def _armature_only_evaluated_mesh(
    obj: bpy.types.Object,
    depsgraph: bpy.types.Depsgraph,
) -> tuple[bpy.types.Object, bpy.types.Mesh, list[tuple[bpy.types.Modifier, bool]]]:
    states: list[tuple[bpy.types.Modifier, bool]] = []
    for modifier in obj.modifiers:
        if modifier.type == "ARMATURE":
            continue
        states.append((modifier, bool(modifier.show_viewport)))
        modifier.show_viewport = False
    bpy.context.view_layer.update()
    evaluated = obj.evaluated_get(depsgraph)
    mesh = evaluated.to_mesh(preserve_all_data_layers=True, depsgraph=depsgraph)
    return evaluated, mesh, states


def _restore_armature_only_mesh(
    evaluated: bpy.types.Object,
    states: list[tuple[bpy.types.Modifier, bool]],
) -> None:
    evaluated.to_mesh_clear()
    for modifier, show_viewport in states:
        modifier.show_viewport = show_viewport
    bpy.context.view_layer.update()


def sample_shoe_soles(
    character_objects: Iterable[bpy.types.Object],
    armature: bpy.types.Object,
    bone_map: dict[str, str],
    floor_z: float,
) -> dict[str, Any]:
    objects = list(character_objects)
    checked_modifiers = assert_render_armature_modifiers(objects, armature)
    depsgraph = bpy.context.evaluated_depsgraph_get()
    sides: dict[str, Any] = {}
    for side, label in (("l", "left"), ("r", "right")):
        group_name = bone_map[f"foot_{side}"]
        sole_points: list[Vector] = []
        sampled_meshes: list[str] = []
        for obj in objects:
            if obj.type != "MESH":
                continue
            group = obj.vertex_groups.get(group_name)
            if group is None:
                continue
            normal_matrix = obj.matrix_world.to_3x3().inverted_safe().transposed()
            eligible: list[int] = []
            rest_z: dict[int, float] = {}
            for vertex in obj.data.vertices:
                weights = _vertex_weights(obj, vertex)
                weight = weights.get(group.index, 0.0)
                if weight < 0.5 or weight < max(weights.values(), default=0.0):
                    continue
                normal = (normal_matrix @ vertex.normal).normalized()
                if normal.z > -0.25:
                    continue
                eligible.append(vertex.index)
                rest_z[vertex.index] = float((obj.matrix_world @ vertex.co).z)
            if not eligible:
                continue
            rest_minimum = min(rest_z.values())
            indices = [
                index
                for index in eligible
                if rest_z[index] <= rest_minimum + SOLE_BAND_HEIGHT_M
            ]
            evaluated, mesh, states = _armature_only_evaluated_mesh(obj, depsgraph)
            try:
                if len(mesh.vertices) != len(obj.data.vertices):
                    raise RuntimeError(
                        f"shoe sole topology changed for {obj.name}: "
                        f"{len(obj.data.vertices)} -> {len(mesh.vertices)}"
                    )
                evaluated_normal_matrix = (
                    evaluated.matrix_world.to_3x3().inverted_safe().transposed()
                )
                points = [
                    evaluated.matrix_world @ mesh.vertices[index].co
                    for index in indices
                    if (
                        evaluated_normal_matrix @ mesh.vertices[index].normal
                    ).normalized().z <= -0.10
                ]
                sole_points.extend(points)
            finally:
                _restore_armature_only_mesh(evaluated, states)
            if points:
                sampled_meshes.append(obj.name)
        if not sole_points:
            raise RuntimeError(f"no evaluated shoe sole geometry found for {label}")
        clearances = [float(point.z) - floor_z for point in sole_points]
        sides[label] = {
            "minimum": min(clearances),
            "maximum": max(clearances),
            "sampledVertexCount": len(sole_points),
            "sampledMeshes": sampled_meshes,
            "footGroup": group_name,
        }
    return {
        **sides,
        "floorZ": floor_z,
        "armatureModifiers": checked_modifiers,
    }


def _semantic_region_points(
    character_objects: Iterable[bpy.types.Object],
    bone_map: dict[str, str],
) -> dict[str, list[Vector]]:
    role_groups = {
        "head": {bone_map["head"]},
        "leftHand": {
            bone_map[role]
            for role in bone_map
            if role == "hand_l" or role.startswith("finger_") and role.endswith("_l")
        },
        "rightHand": {
            bone_map[role]
            for role in bone_map
            if role == "hand_r" or role.startswith("finger_") and role.endswith("_r")
        },
    }
    points = {role: [] for role in role_groups}
    depsgraph = bpy.context.evaluated_depsgraph_get()
    for obj in character_objects:
        if obj.type != "MESH" or obj.hide_render:
            continue
        group_indices = {
            role: {
                group.index
                for name in names
                if (group := obj.vertex_groups.get(name)) is not None
            }
            for role, names in role_groups.items()
        }
        if not any(group_indices.values()):
            continue
        evaluated, mesh, states = _armature_only_evaluated_mesh(obj, depsgraph)
        try:
            if len(mesh.vertices) != len(obj.data.vertices):
                raise RuntimeError(f"semantic geometry topology changed for {obj.name}")
            for vertex in obj.data.vertices:
                weights = _vertex_weights(obj, vertex)
                for role, indices in group_indices.items():
                    if sum(weights.get(index, 0.0) for index in indices) < 0.25:
                        continue
                    points[role].append(evaluated.matrix_world @ mesh.vertices[vertex.index].co)
        finally:
            _restore_armature_only_mesh(evaluated, states)
    for role, samples in points.items():
        if not samples:
            raise RuntimeError(f"no evaluated semantic geometry found for {role}")
    return points


def _camera_visibility(
    camera: bpy.types.Object,
    region_points: dict[str, list[Vector]],
) -> dict[str, Any]:
    scene = bpy.context.scene
    report: dict[str, Any] = {"camera": camera.name}
    for role, points in region_points.items():
        projected = [world_to_camera_view(scene, camera, point) for point in points]
        inside = [point for point in projected if point.z > 0 and 0 <= point.x <= 1 and 0 <= point.y <= 1]
        report[role] = {
            "sampledPointCount": len(projected),
            "insideCount": len(inside),
            "frameBounds": {
                "min": [min(float(point.x) for point in projected), min(float(point.y) for point in projected)],
                "max": [max(float(point.x) for point in projected), max(float(point.y) for point in projected)],
            },
        }
    return report


def _points_inside_objects(
    points: Iterable[Vector],
    objects: Iterable[bpy.types.Object],
    depsgraph: bpy.types.Depsgraph,
) -> int:
    trees = [
        tree
        for obj in objects
        if (tree := _evaluated_world_bvh(obj, depsgraph)) is not None
    ]
    count = 0
    for point in points:
        for tree in trees:
            nearest = tree.find_nearest(point)
            if nearest[0] is None or nearest[1] is None:
                continue
            location, normal = nearest[0], nearest[1]
            if (point - location).dot(normal) < -1e-5:
                count += 1
                break
    return count


def _render_mesh_edge_snapshot(
    character_objects: Iterable[bpy.types.Object],
    depsgraph: bpy.types.Depsgraph,
) -> dict[str, list[float]]:
    snapshot: dict[str, list[float]] = {}
    for obj in character_objects:
        if obj.type != "MESH" or obj.hide_render:
            continue
        evaluated = obj.evaluated_get(depsgraph)
        mesh = evaluated.to_mesh(preserve_all_data_layers=True, depsgraph=depsgraph)
        try:
            points = [evaluated.matrix_world @ vertex.co for vertex in mesh.vertices]
            snapshot[obj.name] = [
                float((points[edge.vertices[0]] - points[edge.vertices[1]]).length)
                for edge in mesh.edges
            ]
        finally:
            evaluated.to_mesh_clear()
    return snapshot


def _deformation_spikes(
    baseline: dict[str, list[float]],
    current: dict[str, list[float]],
) -> tuple[int, float]:
    count = 0
    maximum_ratio = 1.0
    for name, baseline_lengths in baseline.items():
        current_lengths = current.get(name)
        if current_lengths is None or len(current_lengths) != len(baseline_lengths):
            count += 1
            continue
        for first, second in zip(baseline_lengths, current_lengths):
            if not math.isfinite(second):
                count += 1
                continue
            ratio = second / max(first, 1e-6)
            maximum_ratio = max(maximum_ratio, ratio)
            if ratio > DEFORMATION_SPIKE_RATIO and second - first > DEFORMATION_SPIKE_DELTA_M:
                count += 1
    return count, maximum_ratio


def _validation_motion_plan() -> dict[str, Any]:
    return {
        "durationSec": 1.0,
        "motionEvents": [
            {"timeSec": 0.0, "motion": "nod", "duration": 1.0, "strength": 0.65},
            {"timeSec": 0.0, "motion": "present", "duration": 1.0, "strength": 0.75},
        ],
        "lipSync": [
            {"timeSec": 0.0, "viseme": "a", "open": 0.65},
            {"timeSec": 0.5, "viseme": "e", "open": 0.55},
        ],
    }


def _load_mode(
    scene_path: Path,
    master_path: Path,
    mode: str,
) -> tuple[list[bpy.types.Object], bpy.types.Object, dict[str, str], dict[str, Any]]:
    blender_renderer.load_scene_template(str(scene_path))
    mode_objects = blender_renderer.resolve_scene_mode_objects(mode)
    assets = append_master_collection(master_path, bpy.context.scene.collection)
    character_objects = [obj for obj in assets if obj.type == "MESH"]
    armatures = [obj for obj in assets if obj.type == "ARMATURE"]
    dimensions = blender_renderer.prepare_character(
        character_objects,
        target_height=blender_renderer.scene_target_height({"presentationMode": mode}),
        preserve_hierarchy=True,
        asset_objects=assets,
    )
    rig_data = {
        "useMasterAsset": True,
        "rigMode": "auto",
        "preserveExistingRig": True,
        "enhanceExistingRig": False,
        "faceScreenMode": "source",
        "mouthMode": "source_mesh_visemes",
        "facialTopologyMode": "source_only",
        "characterId": "main_ip_sloth",
    }
    armature, _, bone_map = blender_renderer.choose_character_rig(
        rig_data,
        armatures,
        character_objects,
        dimensions,
    )
    face = blender_renderer.setup_face(
        rig_data,
        dimensions,
        armature,
        character_objects,
        bone_map,
    )
    blender_renderer.animate(
        armature,
        face,
        _validation_motion_plan(),
        fps=30,
        bone_map=bone_map,
        presentation_mode=mode,
    )
    scene_data = {
        "presentationMode": mode,
        "sceneBlendPath": str(scene_path),
        "durationSec": 1.0,
        "fps": 30,
        "resolution": {"width": 1920, "height": 1080},
        "cameraPlan": [{"frame": 1, "camera": "Camera_Medium"}],
        "lightingPreset": "editorial_soft",
    }
    scene_stats = blender_renderer.setup_authored_scene(scene_data, dimensions, mode_objects)
    scene_stats["placement"] = blender_renderer.place_character_in_authored_scene(
        character_objects,
        assets,
        armature,
        face,
        dimensions,
        mode_objects,
    )
    return character_objects, armature, bone_map, mode_objects


def validate_mode(
    *,
    scene_path: Path,
    master_path: Path,
    mode: str,
    sample_frames: tuple[int, ...] = DEFAULT_SAMPLE_FRAMES,
) -> dict[str, Any]:
    character_objects, armature, bone_map, mode_objects = _load_mode(
        scene_path,
        master_path,
        mode,
    )
    desk_objects, chair_objects = _studio_collision_objects()
    floor_name, floor_z = _floor_height()
    all_render_meshes = [*character_objects, *desk_objects, *chair_objects]
    frame_reports: list[dict[str, Any]] = []
    baseline_edges: dict[str, list[float]] | None = None
    deformation_spike_count = 0
    maximum_edge_ratio = 1.0
    desk_intersections = 0
    chair_intersections = 0
    hand_intersections = 0
    clearance_samples = {"left": [], "right": []}
    visibility_totals = {
        role: {"sampledPointCount": 0, "insideCount": 0}
        for role in ("head", "leftHand", "rightHand")
    }
    for frame in sample_frames:
        bpy.context.scene.frame_set(int(frame))
        bpy.context.view_layer.update()
        with _render_modifier_state(all_render_meshes):
            depsgraph = bpy.context.evaluated_depsgraph_get()
            soles = sample_shoe_soles(
                character_objects,
                armature,
                bone_map,
                floor_z,
            )
            regions = _semantic_region_points(character_objects, bone_map)
            visibility = _camera_visibility(mode_objects["cameras"]["medium"], regions)
            desk = count_evaluated_mesh_intersections(
                character_objects,
                desk_objects,
                depsgraph,
                mode_objects["cameras"]["medium"],
            )
            chair = count_evaluated_mesh_intersections(
                character_objects,
                chair_objects,
                depsgraph,
                mode_objects["cameras"]["medium"],
            )
            hand_points = [*regions["leftHand"], *regions["rightHand"]]
            hand_count = _points_inside_objects(
                hand_points,
                [*desk_objects, *chair_objects],
                depsgraph,
            )
            edges = _render_mesh_edge_snapshot(character_objects, depsgraph)
        if baseline_edges is None:
            baseline_edges = edges
            spike_count, edge_ratio = 0, 1.0
        else:
            spike_count, edge_ratio = _deformation_spikes(baseline_edges, edges)
        deformation_spike_count += spike_count
        maximum_edge_ratio = max(maximum_edge_ratio, edge_ratio)
        desk_intersections += int(desk["trianglePairCount"])
        chair_intersections += int(chair["trianglePairCount"])
        hand_intersections += hand_count
        for side in ("left", "right"):
            clearance_samples[side].append(float(soles[side]["minimum"]))
        for role in visibility_totals:
            visibility_totals[role]["sampledPointCount"] += int(
                visibility[role]["sampledPointCount"]
            )
            visibility_totals[role]["insideCount"] += int(visibility[role]["insideCount"])
        frame_reports.append(
            {
                "frame": int(frame),
                "floorClearance": {
                    side: float(soles[side]["minimum"])
                    for side in ("left", "right")
                },
                "deskIntersectionCount": int(desk["trianglePairCount"]),
                "deskObjectPairs": desk["objectPairs"],
                "chairIntersectionCount": int(chair["trianglePairCount"]),
                "chairObjectPairs": chair["objectPairs"],
                "handIntersectionCount": hand_count,
                "cameraVisibility": visibility,
                "deformationSpikeCount": spike_count,
                "maximumEdgeRatio": edge_ratio,
            }
        )
    report = {
        "mode": mode,
        "sampleCount": len(sample_frames),
        "sampleFrames": [int(frame) for frame in sample_frames],
        "markers": {
            key: mode_objects[key].name
            for key in ("spawn", "focus", "seat", "foot_l", "foot_r")
        },
        "cameras": {
            role: camera.name for role, camera in mode_objects["cameras"].items()
        },
        "floorObject": floor_name,
        "floorClearance": {
            side: {
                "minimum": min(values),
                "maximum": max(values),
                "samples": values,
            }
            for side, values in clearance_samples.items()
        },
        "deskIntersectionCount": desk_intersections,
        "chairIntersectionCount": chair_intersections,
        "handIntersectionCount": hand_intersections,
        "deformationSpikeCount": deformation_spike_count,
        "maximumEdgeRatio": maximum_edge_ratio,
        "cameraVisibility": {
            "camera": mode_objects["cameras"]["medium"].name,
            **visibility_totals,
        },
        "frames": frame_reports,
    }
    return finalize_mode_report(report)


def validate_modes(
    *,
    scene_path: Path,
    master_path: Path,
    modes: tuple[str, ...] = ("standing", "seated"),
    sample_frames: tuple[int, ...] = DEFAULT_SAMPLE_FRAMES,
) -> list[dict[str, Any]]:
    scene_path = Path(scene_path).expanduser().resolve()
    master_path = Path(master_path).expanduser().resolve()
    if not scene_path.is_file() or scene_path.suffix.lower() != ".blend":
        raise RuntimeError(f"invalid warm studio Blend: {scene_path}")
    if not master_path.is_file() or master_path.suffix.lower() != ".blend":
        raise RuntimeError(f"invalid canonical master Blend: {master_path}")
    if not sample_frames:
        raise RuntimeError("at least one sample frame is required")
    return [
        validate_mode(
            scene_path=scene_path,
            master_path=master_path,
            mode=mode,
            sample_frames=sample_frames,
        )
        for mode in modes
    ]


def _parse_args() -> argparse.Namespace:
    args = sys.argv[sys.argv.index("--") + 1 :] if "--" in sys.argv else []
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--scene", type=Path, default=Path(bpy.data.filepath))
    parser.add_argument("--master", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--modes", default="standing,seated")
    parser.add_argument("--frames", default=",".join(str(frame) for frame in DEFAULT_SAMPLE_FRAMES))
    return parser.parse_args(args)


def main() -> None:
    args = _parse_args()
    modes = tuple(value.strip() for value in args.modes.split(",") if value.strip())
    frames = tuple(int(value.strip()) for value in args.frames.split(",") if value.strip())
    reports = validate_modes(
        scene_path=args.scene,
        master_path=args.master,
        modes=modes,
        sample_frames=frames,
    )
    result = {
        "scene": str(Path(args.scene).expanduser().resolve()),
        "master": str(Path(args.master).expanduser().resolve()),
        "success": all(report["success"] for report in reports),
        "reports": reports,
    }
    args.output = Path(args.output).expanduser().resolve()
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8")
    print("WARM_STUDIO_CHARACTER_VALIDATION=" + json.dumps(result, ensure_ascii=False))
    if not result["success"]:
        raise RuntimeError(f"warm studio character validation failed; see {args.output}")


if __name__ == "__main__":
    main()
