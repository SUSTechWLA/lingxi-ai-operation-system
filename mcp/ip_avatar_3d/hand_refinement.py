#!/usr/bin/env python3
"""Character-specific three-segment hand rigging for the main IP."""
from __future__ import annotations

import json
import math
from dataclasses import dataclass
from typing import Any, Callable

import bpy
from mathutils import Vector

from hand_topology import (
    JOINT_PROGRESS,
    SUPPORT_BAND_TOLERANCE_RATIO,
    SUPPORT_OFFSET,
    DigitTopologyInput,
    HandTopologyResult,
    HandVertexRecord,
    apply_annular_hand_topology,
)


HAND_CONTRACT_KEY = "ip_avatar_hand_contract"
HAND_CONTRACT_VERSION_KEY = "ip_avatar_hand_contract_version"
HAND_SUPPORT_RING_COUNT_KEY = "ip_avatar_hand_support_ring_count"
HAND_CONTRACT_VERSION = 1
HAND_CONTRACT_NAME = "three_segment_annular_strips"


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
            for vertex in obj.data.vertices:
                assignment = next(
                    (item for item in vertex.groups if item.group == hand_group.index and item.weight > 0.05), None
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


def _topology_inputs(regions_by_side: dict[str, list[DigitRegion]], bone_map: dict[str, str]) -> list[DigitTopologyInput]:
    return [
        DigitTopologyInput(region.side, region.index, region.records, region.axis, region.base, region.tip, bone_map[f"hand_{region.side.lower()}"])
        for regions in regions_by_side.values() for region in regions
    ]


def _project_ring_joints(regions_by_side: dict[str, list[DigitRegion]], topology: HandTopologyResult) -> None:
    for regions in regions_by_side.values():
        for region in regions:
            length = max((region.tip - region.base).length, 1e-8)
            for joint_index, attribute in ((1, "joint_1"), (2, "joint_2")):
                centroid = topology.ring_centroids[region.side, region.index, joint_index]
                progress = _clamp((centroid - region.base).dot(region.axis) / length)
                setattr(region, attribute, region.base.lerp(region.tip, progress))


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


def _limit_and_normalize_weights(obj: bpy.types.Object, maximum: int) -> None:
    for vertex in obj.data.vertices:
        weighted = sorted(
            ((assignment.group, float(assignment.weight)) for assignment in vertex.groups if assignment.weight > 1e-8),
            key=lambda item: item[1], reverse=True,
        )
        keep = weighted[:maximum]
        keep_indices = {group for group, _ in keep}
        for assignment in list(vertex.groups):
            if assignment.group not in keep_indices:
                obj.vertex_groups[assignment.group].remove([vertex.index])
        total = sum(weight for _, weight in keep)
        if total > 1e-8:
            for group, weight in keep:
                obj.vertex_groups[group].add([vertex.index], weight / total, "REPLACE")


def _collect_weight_stats(objects: list[bpy.types.Object]) -> dict[str, Any]:
    totals: dict[str, int] = {}
    counts: list[int] = []
    for obj in objects:
        if obj.type != "MESH":
            continue
        names = {group.index: group.name for group in obj.vertex_groups}
        for vertex in obj.data.vertices:
            assignments = [assignment for assignment in vertex.groups if assignment.weight > 1e-6]
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


def _normalized_segment_weights(region: DigitRegion, progress: float, total: float) -> list[float]:
    joint_1, joint_2 = region.joint_progresses
    centers = (joint_1 * 0.5, (joint_1 + joint_2) * 0.5, (joint_2 + 1.0) * 0.5)
    widths = (joint_1, joint_2 - joint_1, 1.0 - joint_2)
    raw = [max(0.0, 1.0 - abs(progress - center) / max(width, 1e-5)) ** 2 for center, width in zip(centers, widths)]
    raw_total = sum(raw)
    return [0.0, 0.0, 0.0] if raw_total <= 1e-8 or total <= 0.0 else [value / raw_total * total for value in raw]


def _assign_segment_weights(
    armature: bpy.types.Object,
    objects: list[bpy.types.Object],
    regions_by_side: dict[str, list[DigitRegion]],
    bone_map: dict[str, str],
    maximum_influences: int,
    topology: HandTopologyResult,
) -> tuple[int, int]:
    blended_vertices = 0
    isolated_support_memberships = 0
    all_segment_names = [name for regions in regions_by_side.values() for region in regions for name in _segment_names(region)]
    for obj in objects:
        if obj.type == "MESH":
            for name in all_segment_names:
                obj.vertex_groups.get(name) or obj.vertex_groups.new(name=name)
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
            for vertex in obj.data.vertices:
                owners = {owner for owner in owner_sets.get(vertex.index, set()) if owner[0] == side}
                assignment = next((item for item in vertex.groups if item.group == hand_group.index and item.weight > 1e-8), None)
                world = obj.matrix_world @ vertex.co
                if owners:
                    if len(owners) > 3:
                        raise RuntimeError(f"mapped ring vertex {obj.name}:{vertex.index} has too many owners: {sorted(owners)}")
                    # Do not depend on BMesh-interpolated Hand assignments at a support ring.
                    for group in deform_groups:
                        group.remove([vertex.index])
                    per_owner = 0.72 / len(owners)
                    for owner in sorted(owners):
                        region = by_key[owner]
                        progress = _clamp((world - region.base).dot(region.axis) / max((region.tip - region.base).length, 1e-8))
                        weights = _normalized_segment_weights(region, progress, per_owner)
                        chosen = max(range(3), key=lambda index: weights[index])
                        groups[_segment_names(region)[chosen]].add([vertex.index], per_owner, "REPLACE")
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
                membership_power = 1.6 + progress * 2.8
                raw = [math.exp(-0.5 * (distance / sigma) ** 2) ** membership_power for distance in distances]
                memberships = [value / max(sum(raw), 1e-8) for value in raw]
                segment_total = hand_weight * _smoothstep(0.08, 0.38, progress)
                if segment_total > 0.08 and sorted(memberships, reverse=True)[1] > 0.08:
                    blended_vertices += 1
                assigned = 0.0
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
                    if in_support_band and vertex.index not in support_indices:
                        if digit_total > 1e-8:
                            isolated_support_memberships += 1
                        digit_total = 0.0
                    for name, weight in zip(_segment_names(region), _normalized_segment_weights(region, region_progress, digit_total)):
                        if weight > 1e-8:
                            groups[name].add([vertex.index], weight, "REPLACE")
                            assigned += weight
                hand_group.add([vertex.index], max(0.0, hand_weight - assigned), "REPLACE")
    for obj in objects:
        if obj.type != "MESH":
            continue
        _limit_and_normalize_weights(obj, maximum_influences)
        for modifier in obj.modifiers:
            if modifier.type == "ARMATURE" and modifier.object == armature:
                modifier.use_deform_preserve_volume = True
    return blended_vertices, isolated_support_memberships


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
) -> dict[str, Any]:
    """Fail closed unless an already-refined master still satisfies its hand contract."""
    violations: list[str] = []
    if armature.get(HAND_CONTRACT_KEY) != HAND_CONTRACT_NAME or int(armature.get(HAND_CONTRACT_VERSION_KEY, 0)) != HAND_CONTRACT_VERSION:
        violations.append("armature is missing the validated three-segment hand contract marker")
    marked_meshes = [
        obj for obj in objects
        if obj.type == "MESH"
        and obj.get(HAND_CONTRACT_KEY) == HAND_CONTRACT_NAME
        and int(obj.get(HAND_CONTRACT_VERSION_KEY, 0)) == HAND_CONTRACT_VERSION
    ]
    ring_count = sum(int(obj.get(HAND_SUPPORT_RING_COUNT_KEY, 0)) for obj in marked_meshes)
    if not marked_meshes or ring_count < 24:
        violations.append(f"hand topology mesh markers validate only {ring_count}/24 support rings")

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

    weight_stats = _collect_weight_stats(objects)
    missing_weights = [name for name in sorted(expected_names) if weight_stats["weightedVertexCounts"].get(name, 0) <= 0]
    if missing_weights:
        violations.append("expected segment groups have no positive weights: " + ", ".join(missing_weights))
    if weight_stats["maxVertexInfluences"] > 4:
        violations.append(f"max vertex influences is {weight_stats['maxVertexInfluences']}, expected at most 4")
    if weight_stats["unweightedVertexCount"] != 0:
        violations.append(f"unweighted vertex count is {weight_stats['unweightedVertexCount']}")
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
) -> None:
    armature[HAND_CONTRACT_KEY] = HAND_CONTRACT_NAME
    armature[HAND_CONTRACT_VERSION_KEY] = HAND_CONTRACT_VERSION
    armature[HAND_SUPPORT_RING_COUNT_KEY] = topology.stats["handJointSupportLoopCount"]
    by_name = {obj.name: obj for obj in objects if obj.type == "MESH"}
    for object_name, rings in topology.ring_vertices.items():
        obj = by_name[object_name]
        obj[HAND_CONTRACT_KEY] = HAND_CONTRACT_NAME
        obj[HAND_CONTRACT_VERSION_KEY] = HAND_CONTRACT_VERSION
        obj[HAND_SUPPORT_RING_COUNT_KEY] = len(rings)


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
        stats.update(_validate_reusable_three_segment_hand_rig(armature, objects, expected_names))
        stats.update({"boneMap": bone_map, "fingerRig": True, "fingerRigEnhanced": False, "fingerRigReused": True,
                      "fingerBoneCount": 18, "fingerSegmentCount": 3, "preserveVolumeSkinning": True})
        return stats, bone_map
    conflicting = sorted(existing_names.intersection(expected_names))
    if set(conflicting) == legacy_two_segment_names and not existing_names.intersection(middle_names):
        stats.update(_collect_weight_stats(objects))
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
    topology = apply_annular_hand_topology(objects, _topology_inputs(regions_by_side, bone_map))
    _project_ring_joints(regions_by_side, topology)
    added_names = _create_segment_bones(armature, regions_by_side, bone_map)
    blended_vertices, support_isolation_count = _assign_segment_weights(
        armature, objects, regions_by_side, bone_map, maximum_influences, topology
    )
    bone_map = resolve_roles([bone.name for bone in armature.data.bones])
    ring_diagnostics = _ring_diagnostics(armature, objects, regions_by_side, bone_map, topology)
    stats.update(topology.stats)
    stats.update(_collect_weight_stats(objects))
    stats.update({"boneMap": bone_map, "fingerRig": True, "fingerRigEnhanced": True, "fingerBoneCount": len(added_names),
                  "fingerSegmentCount": 3, "addedFingerBones": added_names, "fingerWeightingMode": "soft_digit_blend",
                  "fingerTopologyMode": "deterministic_annular_strips", "fingerBlendVertexCount": blended_vertices,
                  "preserveVolumeSkinning": True, "handRingDiagnostics": ring_diagnostics,
                  "externalSupportIsolationCount": support_isolation_count})
    _mark_validated_three_segment_hand_rig(armature, objects, topology)
    armature["ip_avatar_bone_map"] = json.dumps(bone_map)
    armature["ip_avatar_finger_rig_enhanced"] = True
    return stats, bone_map
