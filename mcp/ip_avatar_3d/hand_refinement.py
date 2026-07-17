#!/usr/bin/env python3
"""Character-specific three-segment hand rigging for the main IP."""
from __future__ import annotations

import hashlib
import json
import math
import struct
from dataclasses import dataclass
from typing import Any, Callable

import bpy
from mathutils import Vector

from hand_topology import (
    JOINT_PROGRESS,
    SUPPORT_BAND_TOLERANCE_RATIO,
    SUPPORT_OFFSET,
    HandTopologyResult,
    HandVertexRecord,
)


HAND_CONTRACT_KEY = "ip_avatar_hand_contract"
HAND_CONTRACT_VERSION_KEY = "ip_avatar_hand_contract_version"
HAND_SUPPORT_RING_COUNT_KEY = "ip_avatar_hand_support_ring_count"
HAND_TOPOLOGY_MODE_KEY = "ip_avatar_hand_topology_mode"
HAND_AESTHETIC_VERSION_KEY = "ip_avatar_hand_aesthetic_version"
HAND_AESTHETIC_REPORT_KEY = "ip_avatar_hand_aesthetic_report"
HAND_AESTHETIC_SIGNATURE_KEY = "ip_avatar_hand_aesthetic_integrity_sha256"
HAND_CONTRACT_VERSION = 3
HAND_CONTRACT_NAME = "three_segment_source_surface"
SOURCE_SURFACE_TOPOLOGY_MODE = "source_surface_weighted"
HAND_AESTHETIC_VERSION = "three_digit_refined_v2"
HAND_AESTHETIC_REPORT_SCHEMA = "three_digit_hand_aesthetic_report_v3"
LEGACY_HAND_CONTRACT_NAMES = {
    1: "three_segment_annular_strips",
    2: HAND_CONTRACT_NAME,
}


@dataclass
class DigitRegion:
    side: str
    index: int
    records: list[HandVertexRecord]
    axis: Vector
    base: Vector
    joint_1: Vector
    joint_2: Vector
    tip: Vector
    minimum: float
    maximum: float
    feature_center: Vector
    joint_progresses: tuple[float, float] = JOINT_PROGRESS


def _clamp(value: float, minimum: float = 0.0, maximum: float = 1.0) -> float:
    return max(minimum, min(maximum, value))


def _smoothstep(edge_0: float, edge_1: float, value: float) -> float:
    t = _clamp((value - edge_0) / max(edge_1 - edge_0, 1e-8))
    return t * t * (3.0 - 2.0 * t)


def _cluster_hand_vertices(records: list[HandVertexRecord]) -> list[list[HandVertexRecord]]:
    ys = [float(record.world.y) for record in records]
    zs = [float(record.world.z) for record in records]
    centers = [
        Vector((min(ys), sum(zs) / len(zs))),
        Vector((max(ys), max(zs))),
        Vector((sum(ys) / len(ys), min(zs))),
    ]
    for _ in range(24):
        clusters: list[list[HandVertexRecord]] = [[], [], []]
        for record in records:
            feature = Vector((float(record.world.y), float(record.world.z)))
            clusters[min(range(3), key=lambda index: (feature - centers[index]).length_squared)].append(record)
        for index, cluster in enumerate(clusters):
            if cluster:
                centers[index] = sum(
                    (Vector((float(item.world.y), float(item.world.z))) for item in cluster), Vector((0.0, 0.0))
                ) / len(cluster)
    if any(len(cluster) < 12 for cluster in clusters):
        raise RuntimeError("existing hand mesh does not contain three stable digit regions")
    lower_digit = min(range(3), key=lambda index: centers[index].y)
    remaining = sorted((index for index in range(3) if index != lower_digit), key=lambda index: centers[index].x)
    return [clusters[remaining[0]], clusters[remaining[1]], clusters[lower_digit]]


def analyze_three_digit_hands(
    armature: bpy.types.Object,
    objects: list[bpy.types.Object],
    dimensions: dict[str, Any],
    bone_map: dict[str, str],
) -> dict[str, list[DigitRegion]]:
    """Resolve the source hand skin into the main IP's three visible digits."""
    regions_by_side: dict[str, list[DigitRegion]] = {}
    for side, hand_role in (("L", "hand_l"), ("R", "hand_r")):
        hand_name = bone_map[hand_role]
        hand_bone = armature.data.bones[hand_name]
        hand_head = armature.matrix_world @ hand_bone.head_local
        hand_tail = armature.matrix_world @ hand_bone.tail_local
        hand_direction = (hand_tail - hand_head).normalized()
        hand_length = max((hand_tail - hand_head).length, float(dimensions["width"]) * 0.06)
        records: list[HandVertexRecord] = []
        for obj in objects:
            if obj.type != "MESH":
                continue
            hand_group = obj.vertex_groups.get(hand_name)
            if not hand_group:
                continue
            segment_group_indices = {
                group.index
                for digit in (1, 2, 3)
                for role in (
                    f"finger_{digit}_{side.lower()}",
                    f"finger_{digit}_mid_{side.lower()}",
                    f"finger_{digit}_tip_{side.lower()}",
                )
                if (name := bone_map.get(role))
                if (group := obj.vertex_groups.get(name))
            }
            for vertex in obj.data.vertices:
                assignment = next(
                    (
                        item
                        for item in vertex.groups
                        if item.weight > 0.05
                        and (item.group == hand_group.index or item.group in segment_group_indices)
                    ),
                    None,
                )
                if assignment:
                    world = obj.matrix_world @ vertex.co
                    projection = (world - hand_head).dot(hand_direction)
                    if projection >= hand_length * 0.46:
                        records.append(HandVertexRecord(obj.name, vertex.index, world.copy(), projection, float(assignment.weight)))
        if len(records) < 60:
            raise RuntimeError(f"not enough source hand vertices to create finger rig: {hand_name}")
        regions: list[DigitRegion] = []
        for digit_index, cluster in enumerate(_cluster_hand_vertices(records), start=1):
            projections = [record.projection for record in cluster]
            minimum, maximum = min(projections), max(projections)
            span = max(maximum - minimum, hand_length * 0.25)
            proximal = [record.world for record in cluster if record.projection <= minimum + span * 0.28]
            distal = [record.world for record in cluster if record.projection >= maximum - span * 0.16]
            base = sum(proximal, Vector((0.0, 0.0, 0.0))) / max(1, len(proximal))
            tip = sum(distal, Vector((0.0, 0.0, 0.0))) / max(1, len(distal))
            axis = (tip - base).normalized()
            regions.append(DigitRegion(
                side=side,
                index=digit_index,
                records=cluster,
                axis=axis,
                base=base,
                joint_1=base.lerp(tip, JOINT_PROGRESS[0]),
                joint_2=base.lerp(tip, JOINT_PROGRESS[1]),
                tip=tip,
                minimum=minimum,
                maximum=maximum,
                feature_center=Vector((
                    sum(float(record.world.y) for record in cluster) / len(cluster),
                    sum(float(record.world.z) for record in cluster) / len(cluster),
                )),
            ))
        regions_by_side[side] = regions
    return regions_by_side


def tapered_radial_scale(progress: float) -> float:
    """Return the approved bounded taper over only the outer 72% of a digit."""
    amount = _clamp((progress - 0.28) / 0.72)
    eased = amount * amount * (3.0 - 2.0 * amount)
    return 1.0 - 0.24 * eased


def _centerline_point(points: tuple[Vector, Vector, Vector, Vector], progress: float) -> Vector:
    joint_1, joint_2 = JOINT_PROGRESS
    progress = _clamp(progress)
    if progress <= joint_1:
        return points[0].lerp(points[1], progress / max(joint_1, 1e-8))
    if progress <= joint_2:
        return points[1].lerp(points[2], (progress - joint_1) / max(joint_2 - joint_1, 1e-8))
    return points[2].lerp(points[3], (progress - joint_2) / max(1.0 - joint_2, 1e-8))


def _bone_centerline_points(
    armature: bpy.types.Object,
    bone_map: dict[str, str],
    region: DigitRegion,
) -> tuple[Vector, Vector, Vector, Vector]:
    side = region.side.lower()
    bones = tuple(
        armature.data.bones[bone_map[role]]
        for role in (
            f"finger_{region.index}_{side}",
            f"finger_{region.index}_mid_{side}",
            f"finger_{region.index}_tip_{side}",
        )
    )
    return (
        armature.matrix_world @ bones[0].head_local,
        armature.matrix_world @ bones[0].tail_local,
        armature.matrix_world @ bones[1].tail_local,
        armature.matrix_world @ bones[2].tail_local,
    )


def _sample_width(samples: list[tuple[float, float]], start: float, end: float) -> float:
    radii = sorted(radius for progress, radius in samples if start <= progress <= end)
    if not radii:
        return 0.0
    percentile_index = min(len(radii) - 1, int((len(radii) - 1) * 0.90))
    return radii[percentile_index] * 2.0


def _mesh_adjacency(obj: bpy.types.Object, indices: set[int]) -> dict[int, set[int]]:
    adjacency = {index: set() for index in indices}
    for edge in obj.data.edges:
        first, second = edge.vertices
        if first in indices and second in indices:
            adjacency[first].add(second)
            adjacency[second].add(first)
    return adjacency


def _bounding_volume(points: list[Vector]) -> float:
    if not points:
        return 0.0
    extents = [max(point[axis] for point in points) - min(point[axis] for point in points) for axis in range(3)]
    return max(extents[0] * extents[1] * extents[2], 0.0)


def _measure_current_digit_geometry(
    armature: bpy.types.Object,
    objects: list[bpy.types.Object],
    bone_map: dict[str, str],
    regions_by_side: dict[str, list[DigitRegion]],
) -> dict[str, list[dict[str, Any]]]:
    object_by_name = {obj.name: obj for obj in objects if obj.type == "MESH"}
    measured: dict[str, list[dict[str, Any]]] = {}
    for side, regions in regions_by_side.items():
        digits: list[dict[str, Any]] = []
        for region in regions:
            centerline = _bone_centerline_points(armature, bone_map, region)
            chain_axis = (centerline[-1] - centerline[0]).normalized()
            chain_length = max((centerline[-1] - centerline[0]).length, 1e-8)
            samples: list[tuple[float, float]] = []
            for record in region.records:
                obj = object_by_name[record.object_name]
                world = obj.matrix_world @ obj.data.vertices[record.vertex_index].co
                progress = _clamp((world - centerline[0]).dot(chain_axis) / chain_length)
                radius = (world - _centerline_point(centerline, progress)).length
                samples.append((progress, radius))
            root_width = _sample_width(samples, 0.16, 0.30)
            transition_width = _sample_width(samples, 0.30, 0.40)
            digits.append({
                "digit": region.index,
                "rootWidth": round(root_width, 8),
                "tipWidth": round(_sample_width(samples, 0.84, 1.0), 8),
                "rootWidthTransitionRatioProxy": round(
                    min(root_width, transition_width) / max(root_width, transition_width, 1e-8),
                    8,
                ),
            })
        measured[side.lower()] = digits
    return measured


def refine_three_digit_surface(
    armature: bpy.types.Object,
    objects: list[bpy.types.Object],
    dimensions: dict[str, Any],
    bone_map: dict[str, str],
    regions_by_side: dict[str, list[DigitRegion]],
) -> dict[str, Any]:
    """Conservatively taper and smooth the integrated source hand surface in place."""
    object_by_name = {obj.name: obj for obj in objects if obj.type == "MESH"}
    width = float(dimensions["width"])
    sides: dict[str, dict[str, Any]] = {}
    maximum_smoothing_displacement = 0.0
    maximum_surface_displacement = 0.0

    for side, regions in regions_by_side.items():
        hand_name = bone_map[f"hand_{side.lower()}"]
        hand_bone = armature.data.bones[hand_name]
        hand_head = armature.matrix_world @ hand_bone.head_local
        hand_tail = armature.matrix_world @ hand_bone.tail_local
        hand_axis = (hand_tail - hand_head).normalized()
        hand_length = max((hand_tail - hand_head).length, width * 0.06)
        wrist_points_before: dict[tuple[str, int], Vector] = {}
        palm_points_before: dict[tuple[str, int], Vector] = {}
        for obj in object_by_name.values():
            hand_group = obj.vertex_groups.get(hand_name)
            if not hand_group:
                continue
            for vertex in obj.data.vertices:
                assignment = next(
                    (item for item in vertex.groups if item.group == hand_group.index and item.weight > 0.05),
                    None,
                )
                if not assignment:
                    continue
                world = obj.matrix_world @ vertex.co
                projection = (world - hand_head).dot(hand_axis)
                if projection <= hand_length * 0.18:
                    wrist_points_before[obj.name, vertex.index] = world.copy()
                if projection <= hand_length * 0.46:
                    palm_points_before[obj.name, vertex.index] = world.copy()

        digit_reports: list[dict[str, Any]] = []
        for region in regions:
            centerline = _bone_centerline_points(armature, bone_map, region)
            chain_axis = (centerline[-1] - centerline[0]).normalized()
            chain_length = max((centerline[-1] - centerline[0]).length, 1e-8)
            records_by_object: dict[str, list[HandVertexRecord]] = {}
            for record in region.records:
                records_by_object.setdefault(record.object_name, []).append(record)

            before_samples: list[tuple[float, float]] = []
            after_samples: list[tuple[float, float]] = []
            digit_smoothing_max = 0.0
            digit_surface_max = 0.0
            for object_name, records in records_by_object.items():
                obj = object_by_name[object_name]
                indices = {record.vertex_index for record in records}
                adjacency = _mesh_adjacency(obj, indices)
                original = {index: (obj.matrix_world @ obj.data.vertices[index].co).copy() for index in indices}
                tapered: dict[int, Vector] = {}
                progresses: dict[int, float] = {}
                for index, world in original.items():
                    progress = _clamp((world - centerline[0]).dot(chain_axis) / chain_length)
                    progresses[index] = progress
                    center = _centerline_point(centerline, progress)
                    radial = world - center
                    before_samples.append((progress, radial.length))
                    tapered[index] = center + radial * tapered_radial_scale(progress)

                shaped: dict[int, Vector] = {}
                smoothing_limit = width * 0.004
                for index, point in tapered.items():
                    progress = progresses[index]
                    neighbors = adjacency[index]
                    smoothing = Vector((0.0, 0.0, 0.0))
                    if progress > 0.28 and neighbors:
                        average = sum((tapered[neighbor] for neighbor in neighbors), Vector((0.0, 0.0, 0.0))) / len(neighbors)
                        smoothing = average - point
                        smoothing -= chain_axis * smoothing.dot(chain_axis)
                        smoothing *= 0.16 * _smoothstep(0.28, 1.0, progress)
                        if smoothing.length > smoothing_limit:
                            smoothing = smoothing.normalized() * smoothing_limit
                    shaped[index] = point + smoothing
                    digit_smoothing_max = max(digit_smoothing_max, smoothing.length)

                inverse = obj.matrix_world.inverted()
                for index, world in shaped.items():
                    displacement = world - original[index]
                    digit_surface_max = max(digit_surface_max, displacement.length)
                    local_before = obj.data.vertices[index].co.copy()
                    local_after = inverse @ world
                    local_displacement = local_after - local_before
                    if obj.data.shape_keys:
                        for key in obj.data.shape_keys.key_blocks:
                            key.data[index].co += local_displacement
                    obj.data.vertices[index].co = local_after
                    center = _centerline_point(centerline, progresses[index])
                    after_samples.append((progresses[index], (world - center).length))
                obj.data.update()

            root_width_before = _sample_width(before_samples, 0.16, 0.30)
            root_width = _sample_width(after_samples, 0.16, 0.30)
            tip_width_before = _sample_width(before_samples, 0.84, 1.0)
            tip_width = _sample_width(after_samples, 0.84, 1.0)
            digit_reports.append({
                "digit": region.index,
                "rootWidthBefore": round(root_width_before, 8),
                "rootWidth": round(root_width, 8),
                "tipWidthBefore": round(tip_width_before, 8),
                "tipWidth": round(tip_width, 8),
                "wristBoundaryMaxDisplacement": 0.0,
                "maximumSurfaceDisplacement": round(digit_surface_max, 8),
                "maximumSmoothingDisplacement": round(digit_smoothing_max, 8),
            })
            maximum_smoothing_displacement = max(maximum_smoothing_displacement, digit_smoothing_max)
            maximum_surface_displacement = max(maximum_surface_displacement, digit_surface_max)

        wrist_displacements = [
            ((object_by_name[name].matrix_world @ object_by_name[name].data.vertices[index].co) - before).length
            for (name, index), before in wrist_points_before.items()
        ]
        palm_points_after = [
            object_by_name[name].matrix_world @ object_by_name[name].data.vertices[index].co
            for name, index in palm_points_before
        ]
        wrist_max = max(wrist_displacements, default=0.0)
        for digit_report in digit_reports:
            digit_report["wristBoundaryMaxDisplacement"] = round(wrist_max, 8)
        before_volume = _bounding_volume(list(palm_points_before.values()))
        after_volume = _bounding_volume(palm_points_after)
        sides[side.lower()] = {
            "digitCount": len(digit_reports),
            "digits": digit_reports,
            "wristBoundaryVertexCount": len(wrist_points_before),
            "wristBoundaryMaxDisplacement": round(wrist_max, 8),
            "palmVertexCount": len(palm_points_before),
            "palmAabbVolumeRatioProxy": round(after_volume / max(before_volume, 1e-8), 8) if before_volume else 1.0,
        }

    current_geometry = _measure_current_digit_geometry(
        armature,
        objects,
        bone_map,
        regions_by_side,
    )
    for side, digit_reports in sides.items():
        by_digit = {item["digit"]: item for item in current_geometry[side]}
        for digit_report in digit_reports["digits"]:
            digit_report.update(by_digit[digit_report["digit"]])

    return {
        "handAestheticReportSchema": HAND_AESTHETIC_REPORT_SCHEMA,
        "handAestheticVersion": HAND_AESTHETIC_VERSION,
        "sides": sides,
        "sourceSurfaceShaped": True,
        "sourceSurfaceObjectCount": len(object_by_name),
        "replacementHandObjectCount": 0,
        "maximumSurfaceDisplacement": round(maximum_surface_displacement, 8),
        "maximumSmoothingDisplacement": round(maximum_smoothing_displacement, 8),
    }


def _source_surface_topology(
    objects: list[bpy.types.Object],
    regions_by_side: dict[str, list[DigitRegion]],
) -> HandTopologyResult:
    """Describe weighted joint bands without cutting the clean source triangle surface."""
    regions = [region for side_regions in regions_by_side.values() for region in side_regions]
    ring_centroids = {
        (region.side, region.index, joint): region.base.lerp(region.tip, progress)
        for region in regions
        for joint, progress in enumerate(JOINT_PROGRESS, start=1)
    }
    support_vertices: dict[str, dict[tuple[str, int], set[int]]] = {}
    object_stats: dict[str, dict[str, Any]] = {}
    for obj in (candidate for candidate in objects if candidate.type == "MESH"):
        by_region: dict[tuple[str, int], set[int]] = {}
        for region in regions:
            indices = {
                record.vertex_index
                for record in region.records
                if record.object_name == obj.name
            }
            if indices:
                by_region[region.side, region.index] = indices
        if not by_region:
            continue
        support_vertices[obj.name] = by_region
        vertex_count = len(obj.data.vertices)
        object_stats[obj.name] = {
            "vertexCountBefore": vertex_count,
            "vertexCountAfter": vertex_count,
            "sourceSurfacePreserved": True,
            "weightedDigitRegionCount": len(by_region),
        }
    if not object_stats:
        raise RuntimeError("source-surface hand weighting has no eligible mesh objects")
    before_vertices = sum(item["vertexCountBefore"] for item in object_stats.values())
    after_vertices = sum(item["vertexCountAfter"] for item in object_stats.values())
    stats = {
        "fingerTopologyMode": SOURCE_SURFACE_TOPOLOGY_MODE,
        "handDetailObjectCount": len(object_stats),
        "handDetailVertexCountBefore": before_vertices,
        "handDetailVertexCountAfter": after_vertices,
        "handDetailAddedVertices": 0,
        "handJointSupportLoopCount": 0,
        "handWeightedJointTargetCount": len(ring_centroids),
        "handWeightedJointBandCount": 0,
        "handWeightedJointBandVertexCounts": {},
        "handResolvedJointProgress": {
            f"{region.side}{region.index}": list(JOINT_PROGRESS)
            for region in regions
        },
        "handTopologyAudit": object_stats,
    }
    return HandTopologyResult(
        stats=stats,
        ring_centroids=ring_centroids,
        vertex_owners={name: {} for name in object_stats},
        support_vertices=support_vertices,
        ring_vertices={name: {} for name in object_stats},
    )
def _segment_names(region: DigitRegion) -> tuple[str, str, str]:
    return tuple(f"Finger_{region.index:02d}_{segment}.{region.side}" for segment in ("Proximal", "Middle", "Distal"))


def _create_segment_bones(
    armature: bpy.types.Object, regions_by_side: dict[str, list[DigitRegion]], bone_map: dict[str, str]
) -> list[str]:
    bpy.ops.object.select_all(action="DESELECT")
    armature.hide_viewport = False
    armature.select_set(True)
    bpy.context.view_layer.objects.active = armature
    bpy.ops.object.mode_set(mode="EDIT")
    world_to_armature = armature.matrix_world.inverted()
    added_names: list[str] = []
    try:
        for side, regions in regions_by_side.items():
            source_parent = armature.data.edit_bones[bone_map[f"hand_{side.lower()}"]]
            for region in regions:
                parent = source_parent
                for segment_index, name in enumerate(_segment_names(region)):
                    bone = armature.data.edit_bones.new(name)
                    points = (region.base, region.joint_1, region.joint_2, region.tip)
                    bone.head = world_to_armature @ points[segment_index]
                    bone.tail = world_to_armature @ points[segment_index + 1]
                    bone.parent = parent
                    bone.use_connect = segment_index > 0
                    bone.use_deform = True
                    parent = bone
                    added_names.append(name)
    finally:
        if armature.mode != "OBJECT":
            bpy.ops.object.mode_set(mode="OBJECT")
    return added_names


def _limit_and_normalize_weights(
    armature: bpy.types.Object,
    obj: bpy.types.Object,
    vertex_indices: set[int],
    maximum: int,
) -> None:
    deform_group_indices = {
        group.index
        for group in obj.vertex_groups
        if (bone := armature.data.bones.get(group.name)) and bone.use_deform
    }
    for vertex_index in sorted(vertex_indices):
        vertex = obj.data.vertices[vertex_index]
        weighted = sorted(
            (
                (assignment.group, float(assignment.weight))
                for assignment in vertex.groups
                if assignment.group in deform_group_indices and assignment.weight > 1e-8
            ),
            key=lambda item: item[1], reverse=True,
        )
        keep = weighted[:maximum]
        keep_indices = {group for group, _ in keep}
        for assignment in list(vertex.groups):
            if assignment.group in deform_group_indices and assignment.group not in keep_indices:
                obj.vertex_groups[assignment.group].remove([vertex.index])
        total = sum(weight for _, weight in keep)
        if total > 1e-8:
            for group, weight in keep:
                obj.vertex_groups[group].add([vertex.index], weight / total, "REPLACE")


def _collect_weight_stats(armature: bpy.types.Object, objects: list[bpy.types.Object]) -> dict[str, Any]:
    totals: dict[str, int] = {}
    counts: list[int] = []
    for obj in objects:
        if obj.type != "MESH":
            continue
        names = {group.index: group.name for group in obj.vertex_groups}
        for vertex in obj.data.vertices:
            assignments = [
                assignment
                for assignment in vertex.groups
                if assignment.weight > 1e-6
                and (bone := armature.data.bones.get(names.get(assignment.group, "")))
                and bone.use_deform
            ]
            counts.append(len(assignments))
            for assignment in assignments:
                name = names.get(assignment.group)
                if name:
                    totals[name] = totals.get(name, 0) + 1
    return {
        "weightedVertexCounts": totals,
        "vertexCount": sum(len(obj.data.vertices) for obj in objects if obj.type == "MESH"),
        "maxVertexInfluences": max(counts, default=0),
        "unweightedVertexCount": sum(count == 0 for count in counts),
    }


def _collect_aesthetic_weight_stats(
    armature: bpy.types.Object,
    objects: list[bpy.types.Object],
    regions_by_side: dict[str, list[DigitRegion]],
) -> dict[str, Any]:
    unnormalized = 0
    unweighted = 0
    max_influences = 0
    neighbor_tip_leakage = 0.0
    region_lookup = {
        (record.object_name, record.vertex_index): region
        for regions in regions_by_side.values()
        for region in regions
        for record in region.records
    }
    for obj in objects:
        if obj.type != "MESH":
            continue
        group_names = {group.index: group.name for group in obj.vertex_groups}
        for vertex in obj.data.vertices:
            region = region_lookup.get((obj.name, vertex.index))
            if not region:
                continue
            assignments = [
                (group_names.get(item.group, ""), float(item.weight))
                for item in vertex.groups
                if item.weight > 1e-8
                and (bone := armature.data.bones.get(group_names.get(item.group, "")))
                and bone.use_deform
            ]
            max_influences = max(max_influences, len(assignments))
            total = sum(weight for _, weight in assignments)
            if not assignments:
                unweighted += 1
            elif abs(total - 1.0) > 1e-4:
                unnormalized += 1

            progress = _clamp(
                ((obj.matrix_world @ vertex.co) - region.base).dot(region.axis)
                / max((region.tip - region.base).length, 1e-8)
            )
            if progress < 0.82:
                continue
            own_prefix = f"Finger_{region.index:02d}_"
            own_suffix = f".{region.side}"
            leakage = sum(
                weight
                for name, weight in assignments
                if name.startswith("Finger_")
                and name.endswith(own_suffix)
                and not name.startswith(own_prefix)
            )
            neighbor_tip_leakage = max(neighbor_tip_leakage, leakage)
    return {
        "maxInfluences": max_influences,
        "unnormalizedVertices": unnormalized,
        "unweightedVertices": unweighted,
        "neighborTipLeakageMax": round(neighbor_tip_leakage, 8),
    }


def _normalized_segment_weights(region: DigitRegion, progress: float, total: float) -> list[float]:
    joint_1, joint_2 = region.joint_progresses
    if total <= 0.0:
        return [0.0, 0.0, 0.0]
    blend_half_width = min(SUPPORT_OFFSET * 1.1, (joint_2 - joint_1) * 0.24)
    if progress <= joint_1 - blend_half_width:
        return [total, 0.0, 0.0]
    if progress < joint_1 + blend_half_width:
        amount = _smoothstep(
            joint_1 - blend_half_width,
            joint_1 + blend_half_width,
            progress,
        )
        return [total * (1.0 - amount), total * amount, 0.0]
    if progress <= joint_2 - blend_half_width:
        return [0.0, total, 0.0]
    if progress < joint_2 + blend_half_width:
        amount = _smoothstep(
            joint_2 - blend_half_width,
            joint_2 + blend_half_width,
            progress,
        )
        return [0.0, total * (1.0 - amount), total * amount]
    return [0.0, 0.0, total]


def _isolated_digit_totals(
    owners: set[tuple[str, int]],
    by_key: dict[tuple[str, int], DigitRegion],
    world: Vector,
    total: float,
) -> dict[tuple[str, int], float]:
    """Keep support-ring ownership measurable without materially cross-driving digits."""
    if not owners or total <= 0.0:
        return {}
    ranked = sorted(
        owners,
        key=lambda owner: (
            (
                (world - by_key[owner].base)
                - by_key[owner].axis * (world - by_key[owner].base).dot(by_key[owner].axis)
            ).length_squared,
            owner,
        ),
    )
    if len(ranked) == 1:
        return {ranked[0]: total}
    trace_weight = min(0.01, total * 0.02)
    result = {owner: trace_weight for owner in ranked[1:]}
    result[ranked[0]] = total - trace_weight * (len(ranked) - 1)
    return result


def _assign_segment_weights(
    armature: bpy.types.Object,
    objects: list[bpy.types.Object],
    regions_by_side: dict[str, list[DigitRegion]],
    bone_map: dict[str, str],
    maximum_influences: int,
    topology: HandTopologyResult,
) -> tuple[int, int, dict[str, int]]:
    blended_vertices = 0
    isolated_support_memberships = 0
    weighted_joint_band_vertex_counts: dict[str, int] = {}
    all_segment_names = [name for regions in regions_by_side.values() for region in regions for name in _segment_names(region)]
    for obj in objects:
        if obj.type == "MESH":
            for name in all_segment_names:
                obj.vertex_groups.get(name) or obj.vertex_groups.new(name=name)
    source_surface = topology.stats.get("fingerTopologyMode") == SOURCE_SURFACE_TOPOLOGY_MODE
    affected_vertices: dict[str, set[int]] = {
        object_name: set().union(*by_region.values())
        for object_name, by_region in topology.support_vertices.items()
    }
    for side, regions in regions_by_side.items():
        hand_name = bone_map[f"hand_{side.lower()}"]
        centers = [region.feature_center for region in regions]
        separations = [(centers[left] - centers[right]).length for left in range(3) for right in range(left + 1, 3)]
        sigma = max(min(separations, default=0.01) * 0.62, 1e-5)
        segment_names = [name for region in regions for name in _segment_names(region)]
        by_key = {(region.side, region.index): region for region in regions}
        for obj in objects:
            if obj.type != "MESH":
                continue
            owner_sets = topology.vertex_owners.get(obj.name, {})
            hand_group = obj.vertex_groups.get(hand_name)
            if not hand_group:
                if any(any(owner[0] == side for owner in owners) for owners in owner_sets.values()):
                    raise RuntimeError(f"mapped {side} ring support has no Hand group on {obj.name}")
                continue
            groups = {name: obj.vertex_groups[name] for name in segment_names}
            deform_groups = [
                group for group in obj.vertex_groups
                if (bone := armature.data.bones.get(group.name)) and bone.use_deform
            ]
            side_indices = set().union(*(
                topology.support_vertices.get(obj.name, {}).get((side, region.index), set())
                for region in regions
            ))
            for vertex in obj.data.vertices:
                if vertex.index not in side_indices:
                    continue
                owners = {owner for owner in owner_sets.get(vertex.index, set()) if owner[0] == side}
                assignment = next((item for item in vertex.groups if item.group == hand_group.index and item.weight > 1e-8), None)
                world = obj.matrix_world @ vertex.co
                if owners:
                    if len(owners) > 3:
                        raise RuntimeError(f"mapped ring vertex {obj.name}:{vertex.index} has too many owners: {sorted(owners)}")
                    # Do not depend on BMesh-interpolated Hand assignments at a support ring.
                    for group in deform_groups:
                        group.remove([vertex.index])
                    owner_totals = _isolated_digit_totals(owners, by_key, world, 0.72)
                    for owner, owner_total in owner_totals.items():
                        region = by_key[owner]
                        progress = _clamp((world - region.base).dot(region.axis) / max((region.tip - region.base).length, 1e-8))
                        weights = _normalized_segment_weights(region, progress, owner_total)
                        chosen = max(range(3), key=lambda index: weights[index])
                        groups[_segment_names(region)[chosen]].add([vertex.index], owner_total, "REPLACE")
                    hand_group.add([vertex.index], 0.28, "REPLACE")
                    continue
                if not assignment:
                    continue
                hand_weight = float(assignment.weight)
                for group in groups.values():
                    group.remove([vertex.index])
                distances = [(Vector((float(world.y), float(world.z))) - center).length for center in centers]
                nearest_index = min(range(3), key=lambda index: distances[index])
                nearest = regions[nearest_index]
                progress = _clamp((world - nearest.base).dot(nearest.axis) / max((nearest.tip - nearest.base).length, 1e-8))
                memberships = [1.0 if index == nearest_index else 0.0 for index in range(3)]
                segment_total = hand_weight * _smoothstep(0.08, 0.38, progress)
                assigned = 0.0
                segment_blended = False
                for region, membership in zip(regions, memberships):
                    region_progress = _clamp((world - region.base).dot(region.axis) / max((region.tip - region.base).length, 1e-8))
                    digit_total = segment_total * membership
                    support_progresses = tuple(
                        joint + offset for joint in region.joint_progresses for offset in (-SUPPORT_OFFSET, SUPPORT_OFFSET)
                    )
                    in_support_band = any(
                        abs(region_progress - support) <= SUPPORT_BAND_TOLERANCE_RATIO + 1e-4
                        for support in support_progresses
                    )
                    support_indices = topology.support_vertices.get(obj.name, {}).get((region.side, region.index), set())
                    if not source_surface and in_support_band and vertex.index not in support_indices:
                        if digit_total > 1e-8:
                            isolated_support_memberships += 1
                        digit_total = 0.0
                    segment_weights = _normalized_segment_weights(region, region_progress, digit_total)
                    if sum(weight >= 0.05 for weight in segment_weights) > 1:
                        segment_blended = True
                    for name, weight in zip(_segment_names(region), segment_weights):
                        if weight > 1e-8:
                            groups[name].add([vertex.index], weight, "REPLACE")
                            assigned += weight
                if segment_blended:
                    blended_vertices += 1
                    joint_index = min(
                        range(2),
                        key=lambda index: abs(progress - nearest.joint_progresses[index]),
                    ) + 1
                    key = f"{side}{nearest.index}_joint{joint_index}"
                    weighted_joint_band_vertex_counts[key] = (
                        weighted_joint_band_vertex_counts.get(key, 0) + 1
                    )
                hand_group.add([vertex.index], max(0.0, hand_weight - assigned), "REPLACE")
    for obj in objects:
        if obj.type != "MESH":
            continue
        _limit_and_normalize_weights(
            armature,
            obj,
            affected_vertices.get(obj.name, set()),
            maximum_influences,
        )
        for modifier in obj.modifiers:
            if modifier.type == "ARMATURE" and modifier.object == armature:
                modifier.use_deform_preserve_volume = True
    return blended_vertices, isolated_support_memberships, weighted_joint_band_vertex_counts


def _ring_diagnostics(
    armature: bpy.types.Object,
    objects: list[bpy.types.Object],
    regions_by_side: dict[str, list[DigitRegion]],
    bone_map: dict[str, str],
    topology: HandTopologyResult,
) -> dict[str, int]:
    diagnostics: list[dict[str, Any]] = []
    for side, regions in regions_by_side.items():
        for region in regions:
            names = _segment_names(region)
            proximal = armature.data.bones[bone_map[f"finger_{region.index}_{side.lower()}"]]
            middle = armature.data.bones[bone_map[f"finger_{region.index}_mid_{side.lower()}"]]
            distal = armature.data.bones[bone_map[f"finger_{region.index}_tip_{side.lower()}"]]
            bone_base = armature.matrix_world @ proximal.head_local
            bone_tip = armature.matrix_world @ distal.tail_local
            bone_length = max((bone_tip - bone_base).length, 1e-8)
            bone_axis = (bone_tip - bone_base).normalized()
            for joint, joint_bone in ((1, proximal), (2, middle)):
                bone_joint = armature.matrix_world @ joint_bone.tail_local
                centroid = topology.ring_centroids[side, region.index, joint]
                for offset in (-SUPPORT_OFFSET, SUPPORT_OFFSET):
                    topology_plane = region.base.lerp(region.tip, JOINT_PROGRESS[joint - 1] + offset)
                    bone_plane = bone_joint + bone_axis * bone_length * offset
                    for obj in objects:
                        indices = topology.ring_vertices.get(obj.name, {}).get((side, region.index, joint, offset))
                        if not indices:
                            continue
                        group_indices = [obj.vertex_groups[name].index for name in names if obj.vertex_groups.get(name)]
                        points = {index: obj.matrix_world @ obj.data.vertices[index].co for index in indices}
                        adjacency = {index: set() for index in indices}
                        edge_count = 0
                        for edge in obj.data.edges:
                            first, second = edge.vertices
                            if first in indices and second in indices:
                                adjacency[first].add(second)
                                adjacency[second].add(first)
                                edge_count += 1
                        visited: set[int] = set()
                        stack = [next(iter(indices))]
                        while stack:
                            current = stack.pop()
                            if current in visited:
                                continue
                            visited.add(current)
                            stack.extend(adjacency[current] - visited)
                        digit_weights = []
                        for index in indices:
                            vertex = obj.data.vertices[index]
                            digit_weights.append(sum(item.weight for item in vertex.groups if item.group in group_indices))
                        diagnostic = {
                            "object": obj.name,
                            "ring": f"{side}{region.index}.J{joint}{offset:+.3f}",
                            "mappedVertices": len(indices),
                            "cycle": {
                                "edges": edge_count,
                                "connected": len(visited) == len(indices),
                                "degreeTwo": all(len(neighbors) == 2 for neighbors in adjacency.values()),
                            },
                            "positiveDigitVertices": sum(weight > 1e-6 for weight in digit_weights),
                            "digitWeight": {
                                "min": round(min(digit_weights), 8),
                                "max": round(max(digit_weights), 8),
                                "sum": round(sum(digit_weights), 8),
                            },
                            "maxTopologyPlaneDistance": round(max(abs((point - topology_plane).dot(region.axis)) for point in points.values()), 8),
                            "maxBonePlaneDistance": round(max(abs((point - bone_plane).dot(bone_axis)) for point in points.values()), 8),
                            "boneJointProgress": round((bone_joint - bone_base).dot(bone_axis) / bone_length, 8),
                            "topologyCentroidProgress": round((centroid - region.base).dot(region.axis) / max((region.tip - region.base).length, 1e-8), 8),
                        }
                        diagnostics.append(diagnostic)
    return {
        "ringCount": len(diagnostics),
        "closedCycleCount": sum(
            item["cycle"]["connected"] and item["cycle"]["degreeTwo"]
            for item in diagnostics
        ),
        "positiveWeightRingCount": sum(item["positiveDigitVertices"] == item["mappedVertices"] for item in diagnostics),
    }


def _validate_reusable_three_segment_hand_rig(
    armature: bpy.types.Object,
    objects: list[bpy.types.Object],
    expected_names: set[str],
    expected_version: int = HAND_CONTRACT_VERSION,
) -> dict[str, Any]:
    """Fail closed unless an already-refined master still satisfies its hand contract."""
    violations: list[str] = []
    expected_contract_name = LEGACY_HAND_CONTRACT_NAMES.get(expected_version, HAND_CONTRACT_NAME)
    if (
        armature.get(HAND_CONTRACT_KEY) != expected_contract_name
        or int(armature.get(HAND_CONTRACT_VERSION_KEY, 0)) != expected_version
    ):
        violations.append("armature is missing the validated three-segment hand contract marker")
    topology_mode = str(armature.get(HAND_TOPOLOGY_MODE_KEY, ""))
    if expected_version >= 2 and topology_mode != SOURCE_SURFACE_TOPOLOGY_MODE:
        violations.append(
            f"armature hand topology mode is {topology_mode!r}, expected {SOURCE_SURFACE_TOPOLOGY_MODE!r}"
        )
    marked_meshes = [
        obj for obj in objects
        if obj.type == "MESH"
        and obj.get(HAND_CONTRACT_KEY) == expected_contract_name
        and int(obj.get(HAND_CONTRACT_VERSION_KEY, 0)) == expected_version
        and (expected_version == 1 or obj.get(HAND_TOPOLOGY_MODE_KEY) == SOURCE_SURFACE_TOPOLOGY_MODE)
    ]
    if not marked_meshes:
        violations.append("source-surface hand contract has no marked character mesh")
    if expected_version == HAND_CONTRACT_VERSION:
        if armature.get(HAND_AESTHETIC_VERSION_KEY) != HAND_AESTHETIC_VERSION:
            violations.append("armature is missing the validated hand aesthetic marker")
        if not armature.get(HAND_AESTHETIC_REPORT_KEY):
            violations.append("armature is missing the persisted hand aesthetic report")
        if not armature.get(HAND_AESTHETIC_SIGNATURE_KEY):
            violations.append("armature is missing the hand aesthetic integrity signature")
        for obj in marked_meshes:
            if obj.get(HAND_AESTHETIC_VERSION_KEY) != HAND_AESTHETIC_VERSION:
                violations.append(f"{obj.name} is missing the validated hand aesthetic marker")
            if obj.get(HAND_AESTHETIC_SIGNATURE_KEY) != armature.get(HAND_AESTHETIC_SIGNATURE_KEY):
                violations.append(f"{obj.name} has a mismatched hand aesthetic integrity signature")

    bones = armature.data.bones
    for side in ("L", "R"):
        for digit in (1, 2, 3):
            names = tuple(f"Finger_{digit:02d}_{segment}.{side}" for segment in ("Proximal", "Middle", "Distal"))
            if any(name not in bones for name in names):
                violations.append(f"missing validated chain {'/'.join(names)}")
                continue
            proximal, middle, distal = (bones[name] for name in names)
            if not all(bone.use_deform for bone in (proximal, middle, distal)):
                violations.append(f"non-deform validated chain {'/'.join(names)}")
            if middle.parent != proximal or distal.parent != middle or not middle.use_connect or not distal.use_connect:
                violations.append(f"invalid parent/connect chain {'/'.join(names)}")
            if (middle.head_local - proximal.tail_local).length > 1e-5 or (distal.head_local - middle.tail_local).length > 1e-5:
                violations.append(f"gapped validated chain {'/'.join(names)}")

    weight_stats = _collect_weight_stats(armature, objects)
    missing_weights = [name for name in sorted(expected_names) if weight_stats["weightedVertexCounts"].get(name, 0) <= 0]
    if missing_weights:
        violations.append("expected segment groups have no positive weights: " + ", ".join(missing_weights))
    if weight_stats["maxVertexInfluences"] > 4:
        violations.append(f"max vertex influences is {weight_stats['maxVertexInfluences']}, expected at most 4")
    if weight_stats["unweightedVertexCount"] != 0:
        violations.append(f"unweighted vertex count is {weight_stats['unweightedVertexCount']}")
    for obj in objects:
        if obj.type != "MESH":
            continue
        group_names = {group.index: group.name for group in obj.vertex_groups}
        for vertex in obj.data.vertices:
            deform_assignments = [
                (group_names.get(assignment.group, ""), float(assignment.weight))
                for assignment in vertex.groups
                if (bone := armature.data.bones.get(group_names.get(assignment.group, ""))) and bone.use_deform
            ]
            if not deform_assignments:
                violations.append(f"{obj.name} vertex {vertex.index} has no deform-bone assignment")
                continue
            if any(weight <= 0.0 for _, weight in deform_assignments):
                violations.append(f"{obj.name} vertex {vertex.index} has a non-positive deform assignment")
            if len(deform_assignments) > 4:
                violations.append(f"{obj.name} vertex {vertex.index} has {len(deform_assignments)} deform influences")
            total = sum(weight for _, weight in deform_assignments)
            is_refined_hand_vertex = any(name in expected_names for name, _ in deform_assignments)
            if is_refined_hand_vertex and abs(total - 1.0) > 1e-4:
                violations.append(f"{obj.name} vertex {vertex.index} deform weights sum to {total:.6f}")
    modifiers = [
        modifier for obj in objects if obj.type == "MESH"
        for modifier in obj.modifiers
        if modifier.type == "ARMATURE" and modifier.object == armature
    ]
    if not modifiers or any(not modifier.use_deform_preserve_volume for modifier in modifiers):
        violations.append("validated hand rig is missing preserve-volume Armature modifiers")
    if violations:
        raise RuntimeError("unvalidated three-segment hand rig: " + "; ".join(violations))
    return weight_stats


def _mark_validated_three_segment_hand_rig(
    armature: bpy.types.Object,
    objects: list[bpy.types.Object],
    topology: HandTopologyResult,
    aesthetic_report: dict[str, Any],
    regions_by_side: dict[str, list[DigitRegion]],
) -> None:
    signature = _aesthetic_integrity_signature(objects, aesthetic_report, regions_by_side)
    armature[HAND_CONTRACT_KEY] = HAND_CONTRACT_NAME
    armature[HAND_CONTRACT_VERSION_KEY] = HAND_CONTRACT_VERSION
    armature[HAND_SUPPORT_RING_COUNT_KEY] = 0
    armature[HAND_TOPOLOGY_MODE_KEY] = SOURCE_SURFACE_TOPOLOGY_MODE
    armature[HAND_AESTHETIC_VERSION_KEY] = HAND_AESTHETIC_VERSION
    armature[HAND_AESTHETIC_REPORT_KEY] = json.dumps(aesthetic_report, sort_keys=True)
    armature[HAND_AESTHETIC_SIGNATURE_KEY] = signature
    by_name = {obj.name: obj for obj in objects if obj.type == "MESH"}
    for object_name, rings in topology.ring_vertices.items():
        obj = by_name[object_name]
        obj[HAND_CONTRACT_KEY] = HAND_CONTRACT_NAME
        obj[HAND_CONTRACT_VERSION_KEY] = HAND_CONTRACT_VERSION
        obj[HAND_SUPPORT_RING_COUNT_KEY] = 0
        obj[HAND_TOPOLOGY_MODE_KEY] = SOURCE_SURFACE_TOPOLOGY_MODE
        obj[HAND_AESTHETIC_VERSION_KEY] = HAND_AESTHETIC_VERSION
        obj[HAND_AESTHETIC_SIGNATURE_KEY] = signature


def _aesthetic_integrity_signature(
    objects: list[bpy.types.Object],
    report: dict[str, Any],
    regions_by_side: dict[str, list[DigitRegion]],
) -> str:
    digest = hashlib.sha256()
    digest.update(b"three-digit-hand-aesthetic-v3\0")
    digest.update(json.dumps(
        report,
        sort_keys=True,
        separators=(",", ":"),
        allow_nan=False,
    ).encode("utf-8"))
    by_name = {obj.name: obj for obj in objects if obj.type == "MESH"}
    records = sorted({
        (region.side, region.index, record.object_name, record.vertex_index)
        for regions in regions_by_side.values()
        for region in regions
        for record in region.records
    })
    for side, digit, object_name, vertex_index in records:
        obj = by_name[object_name]
        coordinate = obj.data.vertices[vertex_index].co
        digest.update(f"{side}:{digit}:{object_name}:{vertex_index}\0".encode("utf-8"))
        digest.update(struct.pack("!ddd", *(float(value) for value in coordinate)))
        if obj.data.shape_keys:
            for key in sorted(obj.data.shape_keys.key_blocks, key=lambda item: item.name):
                digest.update(f"shape:{key.name}\0".encode("utf-8"))
                digest.update(struct.pack(
                    "!ddd",
                    *(float(value) for value in key.data[vertex_index].co),
                ))
    return digest.hexdigest()


def _require_finite_number(value: Any, field: str) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)) or not math.isfinite(float(value)):
        raise RuntimeError(
            f"unvalidated three-segment hand rig: incomplete persisted hand aesthetic report ({field})"
        )
    return float(value)


def _stored_aesthetic_report(armature: bpy.types.Object) -> dict[str, Any]:
    try:
        report = json.loads(str(armature[HAND_AESTHETIC_REPORT_KEY]))
    except (KeyError, TypeError, ValueError, json.JSONDecodeError) as exc:
        raise RuntimeError("unvalidated three-segment hand rig: invalid persisted hand aesthetic report") from exc
    if report.get("handAestheticVersion") != HAND_AESTHETIC_VERSION:
        raise RuntimeError("unvalidated three-segment hand rig: stale persisted hand aesthetic report")
    if report.get("handAestheticReportSchema") != HAND_AESTHETIC_REPORT_SCHEMA:
        raise RuntimeError("unvalidated three-segment hand rig: incomplete persisted hand aesthetic report (schema)")
    required_top_level = {
        "handAestheticReportSchema",
        "handAestheticVersion",
        "handAestheticUpgradeFromVersion",
        "sides",
        "sourceSurfaceShaped",
        "sourceSurfaceObjectCount",
        "replacementHandObjectCount",
        "maximumSurfaceDisplacement",
        "maximumSmoothingDisplacement",
        "maxInfluences",
        "unnormalizedVertices",
        "unweightedVertices",
        "neighborTipLeakageMax",
    }
    if not required_top_level.issubset(report):
        raise RuntimeError("unvalidated three-segment hand rig: incomplete persisted hand aesthetic report (top level)")
    sides = report.get("sides") or {}
    if set(sides) != {"l", "r"} or any(side.get("digitCount") != 3 for side in sides.values()):
        raise RuntimeError("unvalidated three-segment hand rig: incomplete persisted hand aesthetic report")
    required_side = {
        "digitCount",
        "digits",
        "wristBoundaryVertexCount",
        "wristBoundaryMaxDisplacement",
        "palmVertexCount",
        "palmAabbVolumeRatioProxy",
    }
    required_digit = {
        "digit",
        "rootWidthBefore",
        "rootWidth",
        "tipWidthBefore",
        "tipWidth",
        "rootWidthTransitionRatioProxy",
        "wristBoundaryMaxDisplacement",
        "maximumSurfaceDisplacement",
        "maximumSmoothingDisplacement",
    }
    for side_name, side in sides.items():
        if not required_side.issubset(side) or len(side.get("digits") or []) != 3:
            raise RuntimeError(
                f"unvalidated three-segment hand rig: incomplete persisted hand aesthetic report ({side_name})"
            )
        if {digit.get("digit") for digit in side["digits"]} != {1, 2, 3}:
            raise RuntimeError(
                f"unvalidated three-segment hand rig: incomplete persisted hand aesthetic report ({side_name} digits)"
            )
        _require_finite_number(side["wristBoundaryMaxDisplacement"], f"{side_name}.wrist")
        _require_finite_number(side["palmAabbVolumeRatioProxy"], f"{side_name}.palmAabbVolumeRatioProxy")
        for digit in side["digits"]:
            if not required_digit.issubset(digit):
                raise RuntimeError(
                    f"unvalidated three-segment hand rig: incomplete persisted hand aesthetic report ({side_name} digit)"
                )
            for field in required_digit - {"digit"}:
                _require_finite_number(digit[field], f"{side_name}.{digit['digit']}.{field}")
    for field in (
        "sourceSurfaceObjectCount",
        "replacementHandObjectCount",
        "maximumSurfaceDisplacement",
        "maximumSmoothingDisplacement",
        "maxInfluences",
        "unnormalizedVertices",
        "unweightedVertices",
        "neighborTipLeakageMax",
    ):
        _require_finite_number(report[field], field)
    return report


def _validate_current_aesthetic_report(
    armature: bpy.types.Object,
    objects: list[bpy.types.Object],
    bone_map: dict[str, str],
    regions_by_side: dict[str, list[DigitRegion]],
    report: dict[str, Any],
) -> None:
    mesh_objects = [obj for obj in objects if obj.type == "MESH"]
    if int(report["sourceSurfaceObjectCount"]) != len(mesh_objects):
        raise RuntimeError("unvalidated three-segment hand rig: persisted hand aesthetic object count is stale")
    if int(report["replacementHandObjectCount"]) != sum(
        bool(obj.get("ip_avatar_replacement_hand")) for obj in mesh_objects
    ):
        raise RuntimeError("unvalidated three-segment hand rig: persisted replacement-hand count is stale")
    region_object_names = {
        record.object_name
        for regions in regions_by_side.values()
        for region in regions
        for record in region.records
    }
    signature = armature.get(HAND_AESTHETIC_SIGNATURE_KEY)
    by_name = {obj.name: obj for obj in mesh_objects}
    for object_name in region_object_names:
        obj = by_name[object_name]
        if (
            obj.get(HAND_CONTRACT_KEY) != HAND_CONTRACT_NAME
            or int(obj.get(HAND_CONTRACT_VERSION_KEY, 0)) != HAND_CONTRACT_VERSION
            or obj.get(HAND_AESTHETIC_VERSION_KEY) != HAND_AESTHETIC_VERSION
            or obj.get(HAND_AESTHETIC_SIGNATURE_KEY) != signature
        ):
            raise RuntimeError(
                f"unvalidated three-segment hand rig: {object_name} has incomplete hand aesthetic markers"
            )
    current_geometry = _measure_current_digit_geometry(armature, objects, bone_map, regions_by_side)
    for side in ("l", "r"):
        persisted_by_digit = {item["digit"]: item for item in report["sides"][side]["digits"]}
        for measured in current_geometry[side]:
            persisted = persisted_by_digit[measured["digit"]]
            for field in ("rootWidth", "tipWidth", "rootWidthTransitionRatioProxy"):
                if abs(float(persisted[field]) - float(measured[field])) > 1e-7:
                    raise RuntimeError(
                        "unvalidated three-segment hand rig: persisted hand aesthetic geometry does not match current mesh"
                    )
    current_weights = _collect_aesthetic_weight_stats(armature, objects, regions_by_side)
    for field, value in current_weights.items():
        if report[field] != value:
            raise RuntimeError(
                "unvalidated three-segment hand rig: persisted hand aesthetic weights do not match current mesh"
            )
    expected_signature = _aesthetic_integrity_signature(objects, report, regions_by_side)
    if armature.get(HAND_AESTHETIC_SIGNATURE_KEY) != expected_signature:
        raise RuntimeError("unvalidated three-segment hand rig: hand aesthetic geometry/report integrity mismatch")


def _shape_and_reweight_hands(
    armature: bpy.types.Object,
    objects: list[bpy.types.Object],
    dimensions: dict[str, Any],
    bone_map: dict[str, str],
    regions_by_side: dict[str, list[DigitRegion]],
    maximum_influences: int,
) -> tuple[HandTopologyResult, dict[str, Any], int, int, dict[str, int]]:
    topology = _source_surface_topology(objects, regions_by_side)
    aesthetic_report = refine_three_digit_surface(
        armature,
        objects,
        dimensions,
        bone_map,
        regions_by_side,
    )
    blended_vertices, support_isolation_count, joint_band_vertex_counts = _assign_segment_weights(
        armature,
        objects,
        regions_by_side,
        bone_map,
        maximum_influences,
        topology,
    )
    aesthetic_report.update(_collect_aesthetic_weight_stats(armature, objects, regions_by_side))
    return (
        topology,
        aesthetic_report,
        blended_vertices,
        support_isolation_count,
        joint_band_vertex_counts,
    )


def enhance_three_segment_hands(
    *, armature: bpy.types.Object, objects: list[bpy.types.Object], dimensions: dict[str, Any], stats: dict[str, Any],
    resolve_roles: Callable[[list[str]], dict[str, str]], maximum_influences: int = 4,
) -> tuple[dict[str, Any], dict[str, str]]:
    """Refine the curated existing-rig hand mesh, bones, and deform weights."""
    bone_map = resolve_roles([bone.name for bone in armature.data.bones])
    if not {"hand_l", "hand_r"}.issubset(bone_map):
        stats["fingerRigEnhanced"] = False
        stats["fingerEnhancementReason"] = "source rig is missing one or both hand roles"
        return stats, bone_map
    expected_names = {
        f"Finger_{digit:02d}_{segment}.{side}"
        for side in ("L", "R")
        for digit in (1, 2, 3)
        for segment in ("Proximal", "Middle", "Distal")
    }
    legacy_two_segment_names = {
        f"Finger_{digit:02d}_{segment}.{side}"
        for side in ("L", "R")
        for digit in (1, 2, 3)
        for segment in ("Proximal", "Distal")
    }
    middle_names = {
        f"Finger_{digit:02d}_Middle.{side}"
        for side in ("L", "R")
        for digit in (1, 2, 3)
    }
    existing_names = {bone.name for bone in armature.data.bones}
    if expected_names.issubset(existing_names):
        contract_version = int(armature.get(HAND_CONTRACT_VERSION_KEY, 0))
        if contract_version == HAND_CONTRACT_VERSION:
            stats.update(
                _validate_reusable_three_segment_hand_rig(
                    armature,
                    objects,
                    expected_names,
                    expected_version=HAND_CONTRACT_VERSION,
                )
            )
            aesthetic_report = _stored_aesthetic_report(armature)
            regions_by_side = analyze_three_digit_hands(armature, objects, dimensions, bone_map)
            _validate_current_aesthetic_report(
                armature,
                objects,
                bone_map,
                regions_by_side,
                aesthetic_report,
            )
            stats.update(aesthetic_report)
            stats.update({
                "boneMap": bone_map,
                "fingerRig": True,
                "fingerRigEnhanced": False,
                "fingerRigReused": True,
                "handAestheticReused": True,
                "fingerBoneCount": 18,
                "fingerSegmentCount": 3,
                "fingerTopologyMode": SOURCE_SURFACE_TOPOLOGY_MODE,
                "preserveVolumeSkinning": True,
            })
            return stats, bone_map
        if contract_version not in LEGACY_HAND_CONTRACT_NAMES:
            raise RuntimeError(
                f"unvalidated three-segment hand rig: unsupported contract version {contract_version}"
            )
        stats.update(
            _validate_reusable_three_segment_hand_rig(
                armature,
                objects,
                expected_names,
                expected_version=contract_version,
            )
        )
        regions_by_side = analyze_three_digit_hands(armature, objects, dimensions, bone_map)
        (
            topology,
            aesthetic_report,
            blended_vertices,
            support_isolation_count,
            joint_band_vertex_counts,
        ) = _shape_and_reweight_hands(
            armature,
            objects,
            dimensions,
            bone_map,
            regions_by_side,
            maximum_influences,
        )
        aesthetic_report["handAestheticUpgradeFromVersion"] = contract_version
        ring_diagnostics = _ring_diagnostics(armature, objects, regions_by_side, bone_map, topology)
        stats.update(topology.stats)
        stats.update(_collect_weight_stats(armature, objects))
        stats.update(aesthetic_report)
        stats.update({
            "boneMap": bone_map,
            "fingerRig": True,
            "fingerRigEnhanced": False,
            "fingerRigReused": True,
            "handAestheticEnhanced": True,
            "handAestheticReused": False,
            "fingerBoneCount": 18,
            "fingerSegmentCount": 3,
            "fingerWeightingMode": "isolated_digit_banded",
            "fingerTopologyMode": SOURCE_SURFACE_TOPOLOGY_MODE,
            "fingerBlendVertexCount": blended_vertices,
            "handWeightedJointBandCount": len(joint_band_vertex_counts),
            "handWeightedJointBandVertexCounts": joint_band_vertex_counts,
            "preserveVolumeSkinning": True,
            "handRingDiagnostics": ring_diagnostics,
            "externalSupportIsolationCount": support_isolation_count,
        })
        _mark_validated_three_segment_hand_rig(
            armature,
            objects,
            topology,
            aesthetic_report,
            regions_by_side,
        )
        armature["ip_avatar_bone_map"] = json.dumps(bone_map)
        return stats, bone_map
    conflicting = sorted(existing_names.intersection(expected_names))
    if set(conflicting) == legacy_two_segment_names and not existing_names.intersection(middle_names):
        stats.update(_collect_weight_stats(armature, objects))
        stats.update({
            "boneMap": bone_map,
            "fingerRig": True,
            "fingerRigEnhanced": False,
            "fingerRigReused": True,
            "fingerBoneCount": 12,
            "fingerSegmentCount": 2,
            "fingerLegacyTwoSegment": True,
            "fingerEnhancementReason": "legacy two-segment finger rig preserved; three-segment migration deferred",
            "preserveVolumeSkinning": True,
        })
        return stats, bone_map
    if conflicting:
        raise RuntimeError("partial three-segment finger rig cannot be refined repeatably: " + ", ".join(conflicting))
    regions_by_side = analyze_three_digit_hands(armature, objects, dimensions, bone_map)
    added_names = _create_segment_bones(armature, regions_by_side, bone_map)
    bone_map = resolve_roles([bone.name for bone in armature.data.bones])
    (
        topology,
        aesthetic_report,
        blended_vertices,
        support_isolation_count,
        joint_band_vertex_counts,
    ) = _shape_and_reweight_hands(
        armature,
        objects,
        dimensions,
        bone_map,
        regions_by_side,
        maximum_influences,
    )
    aesthetic_report["handAestheticUpgradeFromVersion"] = 0
    ring_diagnostics = _ring_diagnostics(armature, objects, regions_by_side, bone_map, topology)
    stats.update(topology.stats)
    stats.update(_collect_weight_stats(armature, objects))
    stats.update(aesthetic_report)
    stats.update({"boneMap": bone_map, "fingerRig": True, "fingerRigEnhanced": True, "fingerBoneCount": len(added_names),
                  "fingerSegmentCount": 3, "addedFingerBones": added_names, "fingerWeightingMode": "isolated_digit_banded",
                  "fingerTopologyMode": SOURCE_SURFACE_TOPOLOGY_MODE, "fingerBlendVertexCount": blended_vertices,
                  "handWeightedJointBandCount": len(joint_band_vertex_counts),
                  "handWeightedJointBandVertexCounts": joint_band_vertex_counts,
                  "preserveVolumeSkinning": True, "handRingDiagnostics": ring_diagnostics,
                  "externalSupportIsolationCount": support_isolation_count})
    _mark_validated_three_segment_hand_rig(
        armature,
        objects,
        topology,
        aesthetic_report,
        regions_by_side,
    )
    armature["ip_avatar_bone_map"] = json.dumps(bone_map)
    armature["ip_avatar_finger_rig_enhanced"] = True
    return stats, bone_map
