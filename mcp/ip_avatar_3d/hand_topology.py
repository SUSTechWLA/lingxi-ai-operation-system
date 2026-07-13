"""Deterministic, staged topology refinement for the main-IP digit support bands."""
from __future__ import annotations

import math
from dataclasses import dataclass
from typing import Any

import bmesh
import bpy
from mathutils import Vector


JOINT_PROGRESS = (0.46, 0.68)
SUPPORT_OFFSET = 0.045
SUPPORT_BAND_TOLERANCE_RATIO = 0.02
MINIMUM_RING_VERTICES = 4
MINIMUM_RING_AREA_RATIO = 0.000025
MINIMUM_RING_SPAN_RATIO = 0.015
PATCH_LAYER_NAME = "ip_annular_digit_patch"


@dataclass(frozen=True)
class HandVertexRecord:
    object_name: str
    vertex_index: int
    world: Vector
    projection: float
    hand_weight: float


@dataclass(frozen=True)
class DigitTopologyInput:
    side: str
    index: int
    records: list[HandVertexRecord]
    axis: Vector
    base: Vector
    tip: Vector
    hand_group: str

    @property
    def key(self) -> tuple[str, int]:
        return self.side, self.index

    @property
    def length(self) -> float:
        return max((self.tip - self.base).length, 1e-8)


@dataclass(frozen=True)
class HandTopologyResult:
    stats: dict[str, Any]
    ring_centroids: dict[tuple[str, int, int], Vector]
    vertex_owners: dict[str, dict[int, set[tuple[str, int]]]]
    support_vertices: dict[str, dict[tuple[str, int], set[int]]]
    ring_vertices: dict[str, dict[tuple[str, int, int, float], set[int]]]


def _support_specs(region: DigitTopologyInput):
    for joint, progress in enumerate(JOINT_PROGRESS, start=1):
        for offset in (-SUPPORT_OFFSET, SUPPORT_OFFSET):
            yield joint, offset, region.base.lerp(region.tip, progress + offset)


def _boundary_components(bm: bmesh.types.BMesh, obj: bpy.types.Object) -> list[dict[str, Any]]:
    boundary_edges = [edge for edge in bm.edges if len(edge.link_faces) == 1]
    adjacency: dict[bmesh.types.BMVert, set[bmesh.types.BMVert]] = {}
    for edge in boundary_edges:
        first, second = edge.verts
        adjacency.setdefault(first, set()).add(second)
        adjacency.setdefault(second, set()).add(first)

    def coordinate(vertex: bmesh.types.BMVert) -> tuple[float, float, float]:
        world = obj.matrix_world @ vertex.co
        return tuple(float(value) for value in world)

    components: list[dict[str, Any]] = []
    remaining = set(adjacency)
    while remaining:
        vertices: set[bmesh.types.BMVert] = set()
        stack = [remaining.pop()]
        while stack:
            vertex = stack.pop()
            if vertex in vertices:
                continue
            vertices.add(vertex)
            stack.extend(adjacency[vertex] - vertices)
        remaining.difference_update(vertices)
        edges = [edge for edge in boundary_edges if edge.verts[0] in vertices and edge.verts[1] in vertices]
        coordinates = {vertex: coordinate(vertex) for vertex in vertices}
        unordered_edges = sorted(
            [sorted((coordinates[edge.verts[0]], coordinates[edge.verts[1]])) for edge in edges]
        )
        ordered_vertices = sorted(coordinates.values())
        if vertices and all(len(adjacency[vertex]) == 2 for vertex in vertices):
            start = min(vertices, key=coordinates.__getitem__)
            previous = None
            current = start
            ordered_vertices = [coordinates[start]]
            while True:
                next_vertex = next(
                    vertex for vertex in sorted(adjacency[current], key=coordinates.__getitem__) if vertex != previous
                )
                if next_vertex == start:
                    ordered_vertices.append(coordinates[start])
                    break
                ordered_vertices.append(coordinates[next_vertex])
                previous, current = current, next_vertex
        components.append({
            "edgeCount": len(edges),
            "length": sum(
                (Vector(coordinates[edge.verts[0]]) - Vector(coordinates[edge.verts[1]])).length
                for edge in edges
            ),
            "orderedVertices": [list(point) for point in ordered_vertices],
            "vertices": [list(point) for point in sorted(coordinates.values())],
            "edges": [[list(point) for point in edge] for edge in unordered_edges],
        })
    return sorted(components, key=lambda component: (component["vertices"], component["edgeCount"]))


def _point_segment_distance(point: Vector, start: Vector, end: Vector) -> float:
    segment = end - start
    fraction = max(0.0, min(1.0, (point - start).dot(segment) / max(segment.length_squared, 1e-16)))
    return (point - (start + segment * fraction)).length


def _component_contains(candidate: dict[str, Any], source: dict[str, Any], tolerance: float) -> bool:
    segments = [(Vector(edge[0]), Vector(edge[1])) for edge in source["edges"]]
    samples = [Vector(vertex) for vertex in candidate["vertices"]]
    samples.extend((Vector(edge[0]) + Vector(edge[1])) * 0.5 for edge in candidate["edges"])
    return all(
        min(_point_segment_distance(sample, start, end) for start, end in segments) <= tolerance
        for sample in samples
    )


def _boundary_subdivision_preserved(source: dict[str, Any], staged: dict[str, Any]) -> tuple[bool, str]:
    source_components = source["boundaryComponents"]
    staged_components = staged["boundaryComponents"]
    if len(source_components) != len(staged_components):
        return False, f"component count {len(source_components)} -> {len(staged_components)}"
    source_length = float(source["boundaryLength"])
    staged_length = float(staged["boundaryLength"])
    length_tolerance = max(1e-7, source_length * 1e-5)
    if abs(source_length - staged_length) > length_tolerance:
        return False, f"boundary length {source_length:.9f} -> {staged_length:.9f}"
    containment_tolerance = max(1e-6, source_length * 1e-6)
    unmatched = list(source_components)
    for candidate in staged_components:
        matching = next((component for component in unmatched if _component_contains(candidate, component, containment_tolerance)), None)
        if matching is None:
            return False, f"staged component is not contained in a source component within {containment_tolerance:.9g}"
        unmatched.remove(matching)
    return True, "boundary components preserved by subdivision"


def _mesh_audit(bm: bmesh.types.BMesh, obj: bpy.types.Object) -> dict[str, Any]:
    bm.edges.ensure_lookup_table()
    boundary = sum(len(edge.link_faces) == 1 for edge in bm.edges)
    non_manifold = sum(len(edge.link_faces) > 2 for edge in bm.edges)
    loose = sum(not edge.link_faces for edge in bm.edges)
    uv_valid = True
    for layer in bm.loops.layers.uv.values():
        for face in bm.faces:
            for loop in face.loops:
                uv = loop[layer].uv
                uv_valid = uv_valid and math.isfinite(uv.x) and math.isfinite(uv.y)
    components = _boundary_components(bm, obj)
    return {
        "boundaryEdges": boundary,
        "boundaryComponentCount": len(components),
        "boundaryLength": sum(float(component["length"]) for component in components),
        "boundaryComponents": components,
        "nonManifoldEdges": non_manifold,
        "looseEdges": loose,
        "uvValid": uv_valid,
    }


def _capture_support_vertex_coordinates(
    staged_owners: dict[bmesh.types.BMVert, set[tuple[str, int]]],
    staged_support: dict[tuple[str, int], set[bmesh.types.BMVert]],
    staged_rings: dict[tuple[str, int, int, float], set[bmesh.types.BMVert]],
) -> dict[bmesh.types.BMVert, Vector]:
    source_vertices = set(staged_owners)
    source_vertices.update(vertex for vertices in staged_support.values() for vertex in vertices)
    source_vertices.update(vertex for vertices in staged_rings.values() for vertex in vertices)
    return {vertex: vertex.co.copy() for vertex in source_vertices}


def _resolve_support_vertex_indices(
    obj: bpy.types.Object,
    source_points: dict[bmesh.types.BMVert, Vector],
    staged_owners: dict[bmesh.types.BMVert, set[tuple[str, int]]],
    staged_support: dict[tuple[str, int], set[bmesh.types.BMVert]],
    staged_rings: dict[tuple[str, int, int, float], set[bmesh.types.BMVert]],
) -> tuple[
    dict[int, set[tuple[str, int]]],
    dict[tuple[str, int], set[int]],
    dict[str, int],
    dict[tuple[str, int, int, float], set[int]],
]:
    """Resolve pre-write BMesh support coordinates to Mesh indices without using BMesh indices."""
    mesh_points = [(vertex.index, vertex.co.copy()) for vertex in obj.data.vertices]
    if not mesh_points:
        raise RuntimeError(f"support-vertex mapping has no mesh vertices on {obj.name}")
    low = Vector((min(point[1][axis] for point in mesh_points) for axis in range(3)))
    high = Vector((max(point[1][axis] for point in mesh_points) for axis in range(3)))
    tolerance = max(1e-7, (high - low).length * 1e-6)

    def coordinate_key(point: Vector) -> tuple[int, int, int]:
        return tuple(int(round(float(value) / tolerance)) for value in point)

    buckets: dict[tuple[int, int, int], list[tuple[int, Vector]]] = {}
    for index, point in mesh_points:
        buckets.setdefault(coordinate_key(point), []).append((index, point))
    resolved: dict[bmesh.types.BMVert, int] = {}
    used_targets: dict[int, bmesh.types.BMVert] = {}
    for vertex, point in sorted(source_points.items(), key=lambda item: (coordinate_key(item[1]), tuple(item[1]))):
        candidates = buckets.get(coordinate_key(point), []) or mesh_points
        nearest = sorted(((candidate - point).length, index) for index, candidate in candidates)
        if not nearest or nearest[0][0] > tolerance:
            raise RuntimeError(
                f"support-vertex mapping missing on {obj.name}: key={coordinate_key(point)} "
                f"nearest={nearest[0][0] if nearest else 'none'} tolerance={tolerance:.9g}"
            )
        best_distance, best_index = nearest[0]
        if len(nearest) > 1 and abs(nearest[1][0] - best_distance) <= tolerance * 1e-4:
            raise RuntimeError(
                f"support-vertex mapping ambiguous on {obj.name}: key={coordinate_key(point)} "
                f"candidates={nearest[:2]} tolerance={tolerance:.9g}"
            )
        if best_index in used_targets and used_targets[best_index] != vertex:
            raise RuntimeError(f"support-vertex mapping collision on {obj.name}: mesh vertex {best_index}")
        resolved[vertex] = best_index
        used_targets[best_index] = vertex
    owners = {resolved[vertex]: set(owner_set) for vertex, owner_set in staged_owners.items()}
    support = {key: {resolved[vertex] for vertex in vertices} for key, vertices in staged_support.items()}
    ring_counts = {
        f"{side}{digit}.J{joint}{offset:+.3f}": len({resolved[vertex] for vertex in vertices})
        for (side, digit, joint, offset), vertices in staged_rings.items()
    }
    if any(count == 0 for count in ring_counts.values()):
        raise RuntimeError(f"support-vertex mapping resolved an empty ring on {obj.name}: {ring_counts}")
    rings = {key: {resolved[vertex] for vertex in vertices} for key, vertices in staged_rings.items()}
    return owners, support, ring_counts, rings


def _face_components(faces: set[bmesh.types.BMFace]) -> list[set[bmesh.types.BMFace]]:
    result: list[set[bmesh.types.BMFace]] = []
    remaining = set(faces)
    while remaining:
        component = {remaining.pop()}
        stack = list(component)
        while stack:
            face = stack.pop()
            neighbors = {linked for edge in face.edges for linked in edge.link_faces if linked in remaining}
            remaining.difference_update(neighbors)
            component.update(neighbors)
            stack.extend(neighbors)
        result.append(component)
    return result


def _face_world_center(obj: bpy.types.Object, face: bmesh.types.BMFace) -> Vector:
    return sum(
        (obj.matrix_world @ vertex.co for vertex in face.verts), Vector((0.0, 0.0, 0.0))
    ) / len(face.verts)


def _local_patch(
    bm: bmesh.types.BMesh, obj: bpy.types.Object, region: DigitTopologyInput
) -> set[bmesh.types.BMFace]:
    """Grow a digit tube from weighted seeds, admitting low-weight palm faces only locally."""
    record_indices = {record.vertex_index for record in region.records if record.object_name == obj.name}
    if not record_indices:
        return set()
    hand_group = obj.vertex_groups.get(region.hand_group)
    weights = {
        vertex.index: max((float(item.weight) for item in vertex.groups if hand_group and item.group == hand_group.index), default=0.0)
        for vertex in obj.data.vertices
    }
    core = {face for face in bm.faces if any(vertex.index in record_indices for vertex in face.verts)}
    if not core:
        return set()
    core_points = [obj.matrix_world @ vertex.co for face in core for vertex in face.verts]
    radii = [
        ((point - region.base) - region.axis * (point - region.base).dot(region.axis)).length
        for point in core_points
    ]
    radius = max(sorted(radii)[len(radii) // 2] * 1.45, region.length * 0.055)
    # The proximal bands sit on the digit-palm transition.  The tube therefore
    # admits the nearby palm sheet, while the axial envelope keeps it local to
    # this hand rather than traversing into the forearm or torso.
    envelope_radius = max(radius * 6.0, region.length * 0.8)
    accepted = set(core)
    frontier = list(core)
    while frontier:
        face = frontier.pop()
        for edge in face.edges:
            for neighbor in edge.link_faces:
                if neighbor in accepted:
                    continue
                center = _face_world_center(obj, neighbor)
                relative = center - region.base
                progress = relative.dot(region.axis) / region.length
                radial = (relative - region.axis * relative.dot(region.axis)).length
                low_hand_weight = max(weights.get(vertex.index, 0.0) for vertex in neighbor.verts) <= 0.05
                if -0.35 <= progress <= 1.18 and radial <= envelope_radius and (low_hand_weight or progress >= -0.12):
                    accepted.add(neighbor)
                    frontier.append(neighbor)
    # Keep only the component attached to the record set; an envelope can touch other body surfaces.
    return max(_face_components(accepted), key=lambda component: len(component & core))


def _mark_patch(bm: bmesh.types.BMesh, obj: bpy.types.Object, region: DigitTopologyInput, bit: int) -> bool:
    layer = bm.faces.layers.int.get(PATCH_LAYER_NAME) or bm.faces.layers.int.new(PATCH_LAYER_NAME)
    patch = _local_patch(bm, obj, region)
    for face in patch:
        face[layer] = int(face[layer]) | bit
    return bool(patch)


def _bisect_patch(
    bm: bmesh.types.BMesh, obj: bpy.types.Object, bit: int, point: Vector, axis: Vector
) -> bool:
    layer = bm.faces.layers.int.get(PATCH_LAYER_NAME)
    marked = [] if layer is None else [face for face in bm.faces if int(face[layer]) & bit]
    if not marked:
        return False
    epsilon = 1e-7

    def distance(vertex: bmesh.types.BMVert) -> float:
        return ((obj.matrix_world @ vertex.co) - point).dot(axis)

    def crosses(edge: bmesh.types.BMEdge) -> bool:
        first, second = (distance(vertex) for vertex in edge.verts)
        return min(first, second) <= epsilon and max(first, second) >= -epsilon

    crossing_faces = {
        face
        for face in marked
        if min(distance(vertex) for vertex in face.verts) <= epsilon
        and max(distance(vertex) for vertex in face.verts) >= -epsilon
    }
    crossing_edges = {edge for face in crossing_faces for edge in face.edges if crosses(edge)}
    if not crossing_edges:
        return False
    # A split edge must bring both incident surface faces into the BMesh op.
    # Passing whole BMesh faces keeps material, smooth flags, and loop custom
    # data on the same split path BMesh normally uses for mesh-wide cuts.
    closure_faces = set(crossing_faces)
    closure_faces.update(face for edge in crossing_edges for face in edge.link_faces)
    closure_edges = {edge for face in closure_faces for edge in face.edges}
    closure_vertices = {vertex for edge in closure_edges for vertex in edge.verts}
    result = bmesh.ops.bisect_plane(
        bm,
        geom=[*closure_vertices, *closure_edges, *closure_faces],
        plane_co=obj.matrix_world.inverted() @ point,
        plane_no=(obj.matrix_world.to_3x3().transposed() @ axis).normalized(),
        clear_inner=False,
        clear_outer=False,
    )
    return bool(result.get("geom_cut"))


def _edge_components(edges: set[bmesh.types.BMEdge]):
    adjacency: dict[bmesh.types.BMVert, set[bmesh.types.BMVert]] = {}
    for edge in edges:
        first, second = edge.verts
        adjacency.setdefault(first, set()).add(second)
        adjacency.setdefault(second, set()).add(first)
    remaining = set(adjacency)
    while remaining:
        vertices: set[bmesh.types.BMVert] = set()
        stack = [remaining.pop()]
        while stack:
            vertex = stack.pop()
            if vertex in vertices:
                continue
            vertices.add(vertex)
            stack.extend(adjacency[vertex] - vertices)
        remaining.difference_update(vertices)
        yield vertices, {edge for edge in edges if edge.verts[0] in vertices and edge.verts[1] in vertices}


def _best_ring(
    bm: bmesh.types.BMesh, obj: bpy.types.Object, bit: int, point: Vector, axis: Vector, length: float
) -> tuple[set[bmesh.types.BMVert], Vector] | None:
    layer = bm.faces.layers.int.get(PATCH_LAYER_NAME)
    if layer is None:
        return None
    epsilon = max(length * 1e-5, 1e-7)
    plane_edges = {
        edge for edge in bm.edges
        if any(int(face[layer]) & bit for face in edge.link_faces)
        and all(abs(((obj.matrix_world @ vertex.co) - point).dot(axis)) <= epsilon for vertex in edge.verts)
    }
    candidates = []
    for vertices, edges in _edge_components(plane_edges):
        if len(vertices) < MINIMUM_RING_VERTICES or len(edges) != len(vertices):
            continue
        if any(sum(vertex in edge.verts for edge in edges) != 2 for vertex in vertices):
            continue
        if any(len(edge.link_faces) != 2 for edge in edges):
            continue
        radial = []
        for vertex in vertices:
            relative = obj.matrix_world @ vertex.co - point
            radial.append(relative - axis * relative.dot(axis))
        span = max((left - right).length for index, left in enumerate(radial) for right in radial[index + 1:])
        area = max(abs(axis.dot(left.cross(right))) for index, left in enumerate(radial) for right in radial[index + 1:])
        if span < length * MINIMUM_RING_SPAN_RATIO or area < length * length * MINIMUM_RING_AREA_RATIO:
            continue
        centroid = sum(
            (obj.matrix_world @ vertex.co for vertex in vertices), Vector((0.0, 0.0, 0.0))
        ) / len(vertices)
        coordinates = tuple(sorted(tuple(round(float(value), 8) for value in obj.matrix_world @ vertex.co) for vertex in vertices))
        candidates.append((vertices, centroid, ((centroid - point).length_squared, -len(edges), coordinates)))
    if not candidates:
        return None
    vertices, centroid, _ = min(candidates, key=lambda candidate: candidate[2])
    return vertices, centroid


def apply_annular_hand_topology(
    objects: list[bpy.types.Object], regions: list[DigitTopologyInput]
) -> HandTopologyResult:
    """Apply all cuts on staged BMeshes and write only after every required ring validates."""
    staged: list[tuple[bpy.types.Object, bmesh.types.BMesh, dict[str, Any], list[tuple[DigitTopologyInput, int]]]] = []
    ring_centroids: dict[tuple[str, int, int], Vector] = {}
    staged_owners: dict[str, dict[bmesh.types.BMVert, set[tuple[str, int]]]] = {}
    staged_support: dict[str, dict[tuple[str, int], set[bmesh.types.BMVert]]] = {}
    staged_rings: dict[str, dict[tuple[str, int, int, float], set[bmesh.types.BMVert]]] = {}
    try:
        for obj in (obj for obj in objects if obj.type == "MESH"):
            object_regions = [region for region in regions if any(record.object_name == obj.name for record in region.records)]
            if not object_regions:
                continue
            if obj.data.shape_keys:
                raise RuntimeError(f"cannot refine hand topology after Shape Keys exist on {obj.name}")
            bm = bmesh.new()
            bm.from_mesh(obj.data)
            bm.verts.ensure_lookup_table()
            bm.edges.ensure_lookup_table()
            bm.faces.ensure_lookup_table()
            source_audit = _mesh_audit(bm, obj)
            marked = []
            for owner, region in enumerate(object_regions):
                bit = 1 << owner
                if not _mark_patch(bm, obj, region, bit):
                    raise RuntimeError(f"no local digit tube for side {region.side} digit {region.index} on {obj.name}")
                marked.append((region, bit))
            # Descending planes prevent an earlier split from obscuring a more distal loop.
            for region, bit in marked:
                for _, _, point in sorted(_support_specs(region), key=lambda spec: (spec[2] - region.base).dot(region.axis), reverse=True):
                    if not _bisect_patch(bm, obj, bit, point, region.axis):
                        raise RuntimeError(f"staged cut produced no surface split for side {region.side} digit {region.index}")
            staged.append((obj, bm, source_audit, marked))
        if not staged:
            raise RuntimeError("hand topology has no eligible mesh objects")
        for obj, bm, _, marked in staged:
            owners = staged_owners.setdefault(obj.name, {})
            support = staged_support.setdefault(obj.name, {})
            rings = staged_rings.setdefault(obj.name, {})
            for region, bit in marked:
                for joint, offset, point in _support_specs(region):
                    ring = _best_ring(bm, obj, bit, point, region.axis, region.length)
                    if ring is None:
                        raise RuntimeError(
                            f"staged annular strip missing side {region.side} digit {region.index} "
                            f"joint {joint} offset {offset:+.3f}"
                        )
                    vertices, centroid = ring
                    rings[region.side, region.index, joint, offset] = set(vertices)
                    ring_centroids[region.side, region.index, joint] = ring_centroids.get(
                        (region.side, region.index, joint), Vector((0.0, 0.0, 0.0))
                    ) + centroid * 0.5
                    support.setdefault(region.key, set()).update(vertices)
                    for vertex in vertices:
                        owners.setdefault(vertex, set()).add(region.key)
            audit = _mesh_audit(bm, obj)
            source_audit = next(item[2] for item in staged if item[0] == obj)
            for key in ("nonManifoldEdges", "looseEdges"):
                if int(audit[key]) > int(source_audit[key]):
                    raise RuntimeError(f"staged topology introduced {key} on {obj.name}: {source_audit[key]} -> {audit[key]}")
            boundary_preserved, boundary_reason = _boundary_subdivision_preserved(source_audit, audit)
            if not boundary_preserved:
                raise RuntimeError(
                    f"staged topology changed boundary geometry on {obj.name}: {boundary_reason}; "
                    f"sourceComponents={source_audit['boundaryComponents']}; "
                    f"stagedComponents={audit['boundaryComponents']}"
                )
            if not audit["uvValid"]:
                raise RuntimeError(f"staged topology invalidated UV coordinates on {obj.name}")
        vertex_owners: dict[str, dict[int, set[tuple[str, int]]]] = {}
        support_vertices: dict[str, dict[tuple[str, int], set[int]]] = {}
        ring_vertices: dict[str, dict[tuple[str, int, int, float], set[int]]] = {}
        object_stats: dict[str, dict[str, Any]] = {}
        for obj, bm, source_audit, _ in staged:
            audit = _mesh_audit(bm, obj)
            layer = bm.faces.layers.int.get(PATCH_LAYER_NAME)
            if layer is not None:
                bm.faces.layers.int.remove(layer)
            before = len(obj.data.vertices)
            source_points = _capture_support_vertex_coordinates(
                staged_owners[obj.name], staged_support[obj.name], staged_rings[obj.name]
            )
            bm.to_mesh(obj.data)
            obj.data.update()
            owners, support, ring_counts, rings = _resolve_support_vertex_indices(
                obj, source_points, staged_owners[obj.name], staged_support[obj.name], staged_rings[obj.name]
            )
            vertex_owners[obj.name] = owners
            support_vertices[obj.name] = support
            ring_vertices[obj.name] = rings
            object_stats[obj.name] = {
                "vertexCountBefore": before,
                "vertexCountAfter": len(obj.data.vertices),
                "sourceAudit": source_audit,
                "stagedAudit": audit,
                "supportVertexMapping": ring_counts,
            }
        before_vertices = sum(stats["vertexCountBefore"] for stats in object_stats.values())
        after_vertices = sum(stats["vertexCountAfter"] for stats in object_stats.values())
        stats = {
            "handDetailObjectCount": len(staged),
            "handDetailVertexCountBefore": before_vertices,
            "handDetailVertexCountAfter": after_vertices,
            "handDetailAddedVertices": after_vertices - before_vertices,
            "handJointSupportLoopCount": len(ring_centroids) * 2,
            "handResolvedJointProgress": {f"{region.side}{region.index}": list(JOINT_PROGRESS) for region in regions},
            "handTopologyAudit": object_stats,
        }
        return HandTopologyResult(stats, ring_centroids, vertex_owners, support_vertices, ring_vertices)
    finally:
        for _, bm, _, _ in staged:
            bm.free()
