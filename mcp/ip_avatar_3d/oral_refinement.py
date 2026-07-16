"""Continuous, rig-ready oral geometry for the staged main-IP face."""

from __future__ import annotations

import math
from dataclasses import dataclass
from typing import Any, Callable

import bpy


ORAL_REFINEMENT_VERSION = 1


@dataclass(frozen=True)
class OralFrame:
    """Mouth-relative dimensions retained from the source face contract."""

    center_x: float
    center_z: float
    surface_y: float
    radius_x: float
    height: float
    depth: float

    @classmethod
    def from_source(cls, source_face: bpy.types.Object, dimensions: dict[str, Any]) -> "OralFrame":
        return cls(
            center_x=float(source_face["mouth_center_x"]),
            center_z=float(source_face["mouth_center_z"]),
            surface_y=float(source_face["mouth_surface_y"]),
            radius_x=float(source_face["mouth_radius_x"]),
            height=float(dimensions["height"]),
            depth=float(source_face.get("head_region_depth", dimensions["depth"])),
        )

    @property
    def origin(self) -> tuple[float, float, float]:
        return (
            self.center_x,
            self.surface_y + self.depth * 0.032,
            self.center_z + self.height * 0.002,
        )


def _mesh_object(
    name: str,
    vertices: list[tuple[float, float, float]],
    faces: list[tuple[int, ...]],
) -> bpy.types.Object:
    mesh = bpy.data.meshes.new(f"{name}_Mesh")
    mesh.from_pydata(vertices, [], faces)
    mesh.update()
    for polygon in mesh.polygons:
        polygon.use_smooth = True
    obj = bpy.data.objects.new(name, mesh)
    bpy.context.collection.objects.link(obj)
    obj["ip_oral_refinement_version"] = ORAL_REFINEMENT_VERSION
    return obj


def _append_material(
    obj: bpy.types.Object,
    material_factory: Callable[..., bpy.types.Material],
    name: str,
    color: tuple[float, float, float, float],
    roughness: float,
) -> None:
    material = bpy.data.materials.get(name) or material_factory(name, color, False)
    bsdf = next((node for node in material.node_tree.nodes if node.type == "BSDF_PRINCIPLED"), None)
    if bsdf and "Roughness" in bsdf.inputs:
        bsdf.inputs["Roughness"].default_value = roughness
    obj.data.materials.append(material)


def _tube_from_rings(
    name: str,
    body_rings: list[list[tuple[float, float, float]]],
) -> bpy.types.Object:
    cap_ring_count = 2
    sides = len(body_rings[0])

    def center(ring: list[tuple[float, float, float]]) -> tuple[float, float, float]:
        return tuple(sum(point[axis] for point in ring) / sides for axis in range(3))

    def rounded_cap(
        endpoint: list[tuple[float, float, float]],
        neighbor: list[tuple[float, float, float]],
    ) -> tuple[list[list[tuple[float, float, float]]], tuple[float, float, float]]:
        endpoint_center = center(endpoint)
        neighbor_center = center(neighbor)
        direction = tuple(endpoint_center[axis] - neighbor_center[axis] for axis in range(3))
        length = math.sqrt(sum(value * value for value in direction))
        if length <= 1e-8:
            direction = (1.0, 0.0, 0.0)
            length = 1.0
        unit_direction = tuple(value / length for value in direction)
        cap_length = max(length * 0.85, 1e-6)
        cap_rings = []
        for index in range(1, cap_ring_count + 1):
            progress = index / (cap_ring_count + 1)
            scale = math.cos(progress * math.pi * 0.5)
            shift = math.sin(progress * math.pi * 0.5) * cap_length
            cap_center = tuple(
                endpoint_center[axis] + unit_direction[axis] * shift for axis in range(3)
            )
            cap_rings.append(
                [
                    tuple(
                        cap_center[axis] + (point[axis] - endpoint_center[axis]) * scale
                        for axis in range(3)
                    )
                    for point in endpoint
                ]
            )
        pole = tuple(
            endpoint_center[axis] + unit_direction[axis] * cap_length for axis in range(3)
        )
        return cap_rings, pole

    start_caps, start_pole = rounded_cap(body_rings[0], body_rings[1])
    end_caps, end_pole = rounded_cap(body_rings[-1], body_rings[-2])
    rings = [*reversed(start_caps), *body_rings, *end_caps]
    vertices = [point for ring in rings for point in ring]
    faces: list[tuple[int, ...]] = []
    for ring_index in range(len(rings) - 1):
        offset = ring_index * sides
        next_offset = offset + sides
        for side in range(sides):
            following = (side + 1) % sides
            faces.append((offset + side, offset + following, next_offset + following, next_offset + side))

    for ring_index, pole in ((0, start_pole), (len(rings) - 1, end_pole)):
        center_index = len(vertices)
        vertices.append(pole)
        offset = ring_index * sides
        for side in range(sides):
            following = (side + 1) % sides
            if ring_index == 0:
                faces.append((center_index, offset + following, offset + side))
            else:
                faces.append((center_index, offset + side, offset + following))
    obj = _mesh_object(name, vertices, faces)
    obj["ip_oral_rounded_cap_ring_count"] = cap_ring_count
    obj["ip_oral_body_ring_count"] = len(body_rings)
    obj["ip_oral_cross_section_count"] = sides
    return obj


def build_rounded_arch(name: str, frame: OralFrame, *, upper: bool) -> bpy.types.Object:
    """Build one continuous dental arcade instead of isolated tooth primitives."""
    longitudinal_rings = 13
    cross_sections = 8
    teeth_z = frame.height * (0.007 if upper else -0.010)
    arch_sign = 1.0 if upper else -1.0
    rings = []
    for index in range(longitudinal_rings):
        progress = index / (longitudinal_rings - 1)
        horizontal = progress * 2.0 - 1.0
        center_x = horizontal * frame.radius_x * 0.74
        center_z = teeth_z + arch_sign * frame.height * 0.0075 * (1.0 - horizontal * horizontal)
        center_y = frame.depth * 0.010
        end_rounding = 0.72 + 0.28 * (1.0 - abs(horizontal))
        radial_y = frame.depth * 0.018 * end_rounding
        radial_z = frame.height * 0.0062 * end_rounding
        ring = []
        for side in range(cross_sections):
            angle = math.tau * side / cross_sections
            ring.append(
                (
                    center_x,
                    center_y + math.cos(angle) * radial_y,
                    center_z + math.sin(angle) * radial_z,
                )
            )
        rings.append(ring)
    obj = _tube_from_rings(name, rings)
    obj.location = frame.origin
    obj["ip_oral_component_count"] = 1
    obj["ip_dental_arch_rings"] = longitudinal_rings
    return obj


def build_gum_arch(name: str, frame: OralFrame, *, upper: bool) -> bpy.types.Object:
    """Build a continuous soft-tissue arch behind its corresponding dentition."""
    longitudinal_rings = 13
    cross_sections = 8
    gum_z = frame.height * (0.010 if upper else -0.013)
    arch_sign = 1.0 if upper else -1.0
    rings = []
    for index in range(longitudinal_rings):
        progress = index / (longitudinal_rings - 1)
        horizontal = progress * 2.0 - 1.0
        center_x = horizontal * frame.radius_x * 0.77
        center_z = gum_z + arch_sign * frame.height * 0.008 * (1.0 - horizontal * horizontal)
        center_y = -frame.depth * 0.001
        end_rounding = 0.68 + 0.32 * (1.0 - abs(horizontal))
        radial_y = frame.depth * 0.019 * end_rounding
        radial_z = frame.height * 0.0072 * end_rounding
        ring = []
        for side in range(cross_sections):
            angle = math.tau * side / cross_sections
            ring.append(
                (
                    center_x,
                    center_y + math.cos(angle) * radial_y,
                    center_z + math.sin(angle) * radial_z,
                )
            )
        rings.append(ring)
    obj = _tube_from_rings(name, rings)
    obj.location = frame.origin
    obj["ip_oral_component_count"] = 1
    obj["ip_gum_arch_rings"] = longitudinal_rings
    return obj


def build_oral_cup(name: str, frame: OralFrame) -> bpy.types.Object:
    """Build a single concave oral-cavity surface beneath the mouth opening."""
    radial_rings = 5
    segments = 24
    vertices = [(0.0, frame.depth * 0.016, 0.0)]
    for ring_index in range(1, radial_rings + 1):
        ratio = ring_index / radial_rings
        for segment in range(segments):
            angle = math.tau * segment / segments
            vertices.append(
                (
                    math.cos(angle) * frame.radius_x * 0.96 * ratio,
                    frame.depth * (0.016 - 0.050 * ratio * ratio),
                    math.sin(angle) * frame.height * 0.026 * ratio,
                )
            )
    faces: list[tuple[int, ...]] = []
    for segment in range(segments):
        following = (segment + 1) % segments
        faces.append((0, 1 + following, 1 + segment))
    for ring_index in range(1, radial_rings):
        inner = 1 + (ring_index - 1) * segments
        outer = 1 + ring_index * segments
        for segment in range(segments):
            following = (segment + 1) % segments
            faces.append((inner + segment, inner + following, outer + following, outer + segment))
    obj = _mesh_object(name, vertices, faces)
    obj.location = frame.origin
    obj["ip_oral_component_count"] = 1
    obj["ip_oral_cup_radial_rings"] = radial_rings
    return obj


def build_articulated_tongue(name: str, frame: OralFrame) -> bpy.types.Object:
    """Build a continuous tapered tongue with a shallow dorsal center groove."""
    longitudinal_rings = 11
    cross_sections = 12
    base_width = frame.radius_x * 0.57
    tip_width = base_width * 0.62
    rings = []
    for index in range(longitudinal_rings):
        progress = index / (longitudinal_rings - 1)
        width = base_width + (tip_width - base_width) * progress
        thickness = frame.height * (0.0072 - 0.0020 * progress)
        center_y = frame.depth * (-0.030 + 0.115 * progress)
        center_z = -frame.height * (0.011 + 0.0025 * progress)
        ring = []
        for side in range(cross_sections):
            angle = math.tau * side / cross_sections
            x = math.cos(angle) * width
            top_surface = max(0.0, math.sin(angle))
            groove = top_surface * math.exp(-((x / max(width, 1e-6)) * 3.0) ** 2) * thickness * 0.34
            ring.append((x, center_y, center_z + math.sin(angle) * thickness - groove))
        rings.append(ring)
    obj = _tube_from_rings(name, rings)
    obj.location = frame.origin
    obj["ip_oral_component_count"] = 1
    obj["ip_tongue_longitudinal_rings"] = longitudinal_rings
    obj["ip_tongue_tip_width_ratio"] = tip_width / base_width
    obj["ip_tongue_center_groove"] = True
    return obj


def rigid_weights(obj: bpy.types.Object, bone_name: str) -> dict[str, dict[int, float]]:
    return {bone_name: {vertex.index: 1.0 for vertex in obj.data.vertices}}


def tongue_weights(
    obj: bpy.types.Object,
    bone_map: dict[str, str],
) -> dict[str, dict[int, float]]:
    roles = ("tongue_1", "tongue_2", "tongue_3")
    coordinates = [float(vertex.co.y) for vertex in obj.data.vertices]
    minimum, maximum = min(coordinates), max(coordinates)
    weights = {bone_map[role]: {} for role in roles}
    for vertex in obj.data.vertices:
        progress = (float(vertex.co.y) - minimum) / max(maximum - minimum, 1e-6)
        raw = [max(0.0, 1.0 - abs(progress - center) / 0.46) for center in (0.12, 0.50, 0.88)]
        total = sum(raw) or 1.0
        for role, value in zip(roles, raw):
            weights[bone_map[role]][vertex.index] = value / total
    return weights


def create_refined_oral_interior(
    source_face: bpy.types.Object,
    armature: bpy.types.Object,
    bone_map: dict[str, str],
    dimensions: dict[str, Any],
    *,
    material_factory: Callable[..., bpy.types.Material],
    bind_object: Callable[..., None],
) -> dict[str, bpy.types.Object]:
    """Create the versioned oral roles without modifying the source facial mesh."""
    frame = OralFrame.from_source(source_face, dimensions)
    upper_teeth = build_rounded_arch("IP_UpperTeeth", frame, upper=True)
    lower_teeth = build_rounded_arch("IP_LowerTeeth", frame, upper=False)
    upper_gum = build_gum_arch("IP_UpperGum", frame, upper=True)
    lower_gum = build_gum_arch("IP_LowerGum", frame, upper=False)
    cavity = build_oral_cup("IP_OralCavity", frame)
    tongue = build_articulated_tongue("IP_Tongue", frame)

    _append_material(upper_teeth, material_factory, "IP_Teeth_Material", (0.92, 0.82, 0.66, 1.0), 0.38)
    _append_material(lower_teeth, material_factory, "IP_Teeth_Material", (0.92, 0.82, 0.66, 1.0), 0.38)
    _append_material(upper_gum, material_factory, "IP_Gum_Material", (0.33, 0.075, 0.085, 1.0), 0.58)
    _append_material(lower_gum, material_factory, "IP_Gum_Material", (0.33, 0.075, 0.085, 1.0), 0.58)
    _append_material(cavity, material_factory, "IP_OralCavity_Material", (0.030, 0.004, 0.006, 1.0), 0.84)
    _append_material(tongue, material_factory, "IP_Tongue_Material", (0.34, 0.055, 0.065, 1.0), 0.66)

    roles = {
        "oral_cavity": cavity,
        "upper_teeth": upper_teeth,
        "lower_teeth": lower_teeth,
        "upper_gum": upper_gum,
        "lower_gum": lower_gum,
        "tongue": tongue,
    }
    for role, obj in roles.items():
        obj["ip_face_topology_role"] = role

    bind_object(upper_teeth, armature, rigid_weights(upper_teeth, bone_map["head"]))
    bind_object(upper_gum, armature, rigid_weights(upper_gum, bone_map["head"]))
    bind_object(lower_teeth, armature, rigid_weights(lower_teeth, bone_map["jaw"]))
    bind_object(lower_gum, armature, rigid_weights(lower_gum, bone_map["jaw"]))
    bind_object(cavity, armature, rigid_weights(cavity, bone_map["head"]))
    bind_object(tongue, armature, tongue_weights(tongue, bone_map))
    return roles
