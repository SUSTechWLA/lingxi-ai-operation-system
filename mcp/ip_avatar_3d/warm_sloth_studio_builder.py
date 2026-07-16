#!/usr/bin/env python3
"""Build the editable architectural shell for the warm sloth studio."""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import sys
from dataclasses import dataclass
from pathlib import Path

import bpy
from mathutils import Matrix, Vector

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import warm_studio_contract as contract


WALL_THICKNESS = 0.16
WINDOW_CENTER_Y = 0.0
WINDOW_SILL_HEIGHT = 0.35
DOOR_CENTER_Y = -1.45
AUTHORED_EXPOSURE = contract.SUBJECT_LIGHT_PROFILE["authoredExposure"]
CYCLES_FINAL_EXPOSURE = contract.SUBJECT_LIGHT_PROFILE["cyclesFinalExposure"]


@dataclass(frozen=True)
class StudioContext:
    """Shared Blender state passed through each staged studio builder."""

    scene: bpy.types.Scene
    master: bpy.types.Object
    collections: dict[str, bpy.types.Collection]
    materials: dict[str, bpy.types.Material]


def ensure_collection(scene: bpy.types.Scene, name: str) -> bpy.types.Collection:
    """Return a named collection linked directly below the scene root."""

    result = bpy.data.collections.get(name)
    if result is None:
        result = bpy.data.collections.new(name)
    if all(child != result for child in scene.collection.children):
        scene.collection.children.link(result)
    return result


def move_to_collection(obj: bpy.types.Object, target: bpy.types.Collection) -> None:
    """Move an object exclusively into the requested semantic collection."""

    for current in list(obj.users_collection):
        current.objects.unlink(obj)
    target.objects.link(obj)


def parent_to_master(obj: bpy.types.Object, master: bpy.types.Object) -> None:
    """Parent an object while preserving its authored world transform."""

    bpy.context.view_layer.update()
    world_transform = obj.matrix_world.copy()
    obj.parent = master
    obj.matrix_world = world_transform
    bpy.context.view_layer.update()


def add_bevel(
    obj: bpy.types.Object,
    width: float,
    *,
    segments: int = 2,
) -> bpy.types.Modifier | None:
    """Add a restrained non-destructive bevel when a positive width is given."""

    if width <= 0:
        return None
    modifier = obj.modifiers.new("Edge_Soften", "BEVEL")
    modifier.width = width
    modifier.segments = segments
    return modifier


def _target_collection(
    ctx: StudioContext,
    target: str | bpy.types.Collection,
) -> bpy.types.Collection:
    if isinstance(target, str):
        return ctx.collections[target]
    return target


def add_box(
    ctx: StudioContext,
    name: str,
    location: tuple[float, float, float],
    dimensions: tuple[float, float, float],
    material: bpy.types.Material | None,
    target: str | bpy.types.Collection = "STUDIO_ARCHITECTURE",
    *,
    bevel: float = 0.0,
) -> bpy.types.Object:
    """Create an exactly sized box in the correct collection and hierarchy."""

    bpy.ops.mesh.primitive_cube_add(location=location)
    obj = bpy.context.object
    obj.name = name
    obj.dimensions = dimensions
    bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)
    if material is not None:
        obj.data.materials.append(material)
    add_bevel(obj, bevel)
    move_to_collection(obj, _target_collection(ctx, target))
    parent_to_master(obj, ctx.master)
    return obj


def add_cylinder(
    ctx: StudioContext,
    name: str,
    location: tuple[float, float, float],
    radius: float,
    depth: float,
    material: bpy.types.Material | None,
    target: str | bpy.types.Collection = "STUDIO_ARCHITECTURE",
    *,
    rotation: tuple[float, float, float] = (0.0, 0.0, 0.0),
    vertices: int = 32,
    bevel: float = 0.0,
) -> bpy.types.Object:
    """Create a cylinder with real mesh geometry and semantic ownership."""

    bpy.ops.mesh.primitive_cylinder_add(
        vertices=vertices,
        radius=radius,
        depth=depth,
        location=location,
        rotation=rotation,
    )
    obj = bpy.context.object
    obj.name = name
    if material is not None:
        obj.data.materials.append(material)
    add_bevel(obj, bevel)
    move_to_collection(obj, _target_collection(ctx, target))
    parent_to_master(obj, ctx.master)
    return obj


def add_curve_profile(
    ctx: StudioContext,
    name: str,
    points: list[tuple[float, float, float]],
    material: bpy.types.Material | None,
    target: str | bpy.types.Collection = "STUDIO_ARCHITECTURE",
    *,
    bevel_depth: float = 0.01,
    cyclic: bool = False,
) -> bpy.types.Object:
    """Create an editable poly-curve profile with a circular rendered section."""

    curve = bpy.data.curves.new(name, "CURVE")
    curve.dimensions = "3D"
    curve.resolution_u = 2
    curve.bevel_depth = bevel_depth
    curve.bevel_resolution = 2
    spline = curve.splines.new("POLY")
    spline.points.add(len(points) - 1)
    for point, coordinate in zip(spline.points, points):
        point.co = (*coordinate, 1.0)
    spline.use_cyclic_u = cyclic
    if material is not None:
        curve.materials.append(material)
    obj = bpy.data.objects.new(name, curve)
    _target_collection(ctx, target).objects.link(obj)
    parent_to_master(obj, ctx.master)
    return obj


def add_empty(
    ctx: StudioContext,
    name: str,
    location: tuple[float, float, float],
    target: str | bpy.types.Collection,
) -> bpy.types.Object:
    """Create a semantic assembly root that remains below SET_MASTER."""

    obj = bpy.data.objects.new(name, None)
    _target_collection(ctx, target).objects.link(obj)
    obj.location = location
    obj.empty_display_type = "CUBE"
    obj.empty_display_size = 0.12
    parent_to_master(obj, ctx.master)
    return obj


def add_uv_sphere(
    ctx: StudioContext,
    name: str,
    location: tuple[float, float, float],
    dimensions: tuple[float, float, float],
    material: bpy.types.Material | None,
    target: str | bpy.types.Collection = "STUDIO_PROPS",
    *,
    rotation: tuple[float, float, float] = (0.0, 0.0, 0.0),
    segments: int = 48,
    ring_count: int = 24,
    bevel: float = 0.0,
) -> bpy.types.Object:
    """Create an ellipsoid with applied positive scale and optional bevel."""

    bpy.ops.mesh.primitive_uv_sphere_add(
        segments=segments,
        ring_count=ring_count,
        location=location,
        rotation=rotation,
    )
    obj = bpy.context.object
    obj.name = name
    obj.dimensions = dimensions
    bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)
    for polygon in obj.data.polygons:
        polygon.use_smooth = True
    obj["uv_segments"] = segments
    obj["uv_rings"] = ring_count
    if material is not None:
        obj.data.materials.append(material)
    add_bevel(obj, bevel)
    move_to_collection(obj, _target_collection(ctx, target))
    parent_to_master(obj, ctx.master)
    return obj


def add_cone(
    ctx: StudioContext,
    name: str,
    location: tuple[float, float, float],
    radius1: float,
    radius2: float,
    depth: float,
    material: bpy.types.Material | None,
    target: str | bpy.types.Collection = "STUDIO_PROPS",
    *,
    rotation: tuple[float, float, float] = (0.0, 0.0, 0.0),
    vertices: int = 32,
    bevel: float = 0.0,
) -> bpy.types.Object:
    """Create a tapered solid useful for pottery, baskets, and lamp shades."""

    bpy.ops.mesh.primitive_cone_add(
        vertices=vertices,
        radius1=radius1,
        radius2=radius2,
        depth=depth,
        location=location,
        rotation=rotation,
    )
    obj = bpy.context.object
    obj.name = name
    if material is not None:
        obj.data.materials.append(material)
    add_bevel(obj, bevel)
    move_to_collection(obj, _target_collection(ctx, target))
    parent_to_master(obj, ctx.master)
    return obj


def add_open_frustum(
    ctx: StudioContext,
    name: str,
    location: tuple[float, float, float],
    radius_bottom: float,
    radius_top: float,
    depth: float,
    material: bpy.types.Material | None,
    target: str | bpy.types.Collection = "STUDIO_PROPS",
    *,
    vertices: int = 32,
    thickness: float = 0.006,
) -> bpy.types.Object:
    """Create a smooth capless frustum with a thin solidified fabric wall."""

    points: list[tuple[float, float, float]] = []
    for index in range(vertices):
        angle = math.tau * index / vertices
        points.append(
            (
                radius_bottom * math.cos(angle),
                radius_bottom * math.sin(angle),
                -depth / 2.0,
            )
        )
    for index in range(vertices):
        angle = math.tau * index / vertices
        points.append(
            (
                radius_top * math.cos(angle),
                radius_top * math.sin(angle),
                depth / 2.0,
            )
        )
    faces = [
        (
            index,
            (index + 1) % vertices,
            vertices + (index + 1) % vertices,
            vertices + index,
        )
        for index in range(vertices)
    ]
    mesh = bpy.data.meshes.new(name)
    mesh.from_pydata(points, [], faces)
    mesh.validate()
    mesh.update()
    for polygon in mesh.polygons:
        polygon.use_smooth = True
    if material is not None:
        mesh.materials.append(material)
    obj = bpy.data.objects.new(name, mesh)
    _target_collection(ctx, target).objects.link(obj)
    obj.location = location
    obj["open_bottom"] = True
    obj["open_top"] = True
    obj["shade_profile"] = "smooth_capless_frustum"
    solidify = obj.modifiers.new("Fabric_Wall_Thickness", "SOLIDIFY")
    solidify.thickness = thickness
    solidify.offset = 0.0
    solidify.use_rim = True
    parent_to_master(obj, ctx.master)
    return obj


def add_hollow_vessel(
    ctx: StudioContext,
    name: str,
    location: tuple[float, float, float],
    outer_radius: float,
    height: float,
    wall_thickness: float,
    bottom_thickness: float,
    material: bpy.types.Material | None,
    target: str | bpy.types.Collection = "STUDIO_PROPS",
    *,
    segments: int = 40,
) -> bpy.types.Object:
    """Create a smooth hollow vessel with inner wall, hard rim, and closed base."""

    inner_radius = outer_radius - wall_thickness
    bottom_z = -height / 2.0
    top_z = height / 2.0
    inner_bottom_z = bottom_z + bottom_thickness
    vertices: list[tuple[float, float, float]] = []
    for radius, z in (
        (outer_radius, bottom_z),
        (outer_radius, top_z),
        (inner_radius, inner_bottom_z),
        (inner_radius, top_z),
    ):
        for index in range(segments):
            angle = math.tau * index / segments
            vertices.append((radius * math.cos(angle), radius * math.sin(angle), z))
    outer_bottom = 0
    outer_top = segments
    inner_bottom = segments * 2
    inner_top = segments * 3
    outer_bottom_center = len(vertices)
    vertices.append((0.0, 0.0, bottom_z))
    inner_bottom_center = len(vertices)
    vertices.append((0.0, 0.0, inner_bottom_z))

    faces: list[tuple[int, ...]] = []
    for index in range(segments):
        next_index = (index + 1) % segments
        faces.append(
            (
                outer_bottom + index,
                outer_bottom + next_index,
                outer_top + next_index,
                outer_top + index,
            )
        )
    for index in range(segments):
        next_index = (index + 1) % segments
        faces.append(
            (
                inner_bottom + next_index,
                inner_bottom + index,
                inner_top + index,
                inner_top + next_index,
            )
        )
    for index in range(segments):
        next_index = (index + 1) % segments
        faces.append(
            (
                outer_top + index,
                outer_top + next_index,
                inner_top + next_index,
                inner_top + index,
            )
        )
    for index in range(segments):
        next_index = (index + 1) % segments
        faces.append(
            (outer_bottom_center, outer_bottom + next_index, outer_bottom + index)
        )
    for index in range(segments):
        next_index = (index + 1) % segments
        faces.append(
            (inner_bottom_center, inner_bottom + index, inner_bottom + next_index)
        )

    mesh = bpy.data.meshes.new(name)
    mesh.from_pydata(vertices, [], faces)
    mesh.validate()
    mesh.update()
    for polygon in mesh.polygons[: segments * 2]:
        polygon.use_smooth = True
    if material is not None:
        mesh.materials.append(material)
    obj = bpy.data.objects.new(name, mesh)
    _target_collection(ctx, target).objects.link(obj)
    obj.location = location
    obj["open_top"] = True
    obj["cavity_radius"] = inner_radius
    obj["wall_thickness"] = wall_thickness
    obj["bottom_thickness"] = bottom_thickness
    obj["outer_side_face_count"] = segments
    obj["inner_side_face_count"] = segments
    obj["rim_face_count"] = segments
    obj["bottom_face_count"] = segments * 2
    parent_to_master(obj, ctx.master)
    return obj


def add_torus(
    ctx: StudioContext,
    name: str,
    location: tuple[float, float, float],
    major_radius: float,
    minor_radius: float,
    material: bpy.types.Material | None,
    target: str | bpy.types.Collection = "STUDIO_PROPS",
    *,
    rotation: tuple[float, float, float] = (0.0, 0.0, 0.0),
) -> bpy.types.Object:
    """Create a compact metal or ceramic torus with semantic ownership."""

    bpy.ops.mesh.primitive_torus_add(
        major_radius=major_radius,
        minor_radius=minor_radius,
        major_segments=36,
        minor_segments=8,
        location=location,
        rotation=rotation,
    )
    obj = bpy.context.object
    obj.name = name
    if material is not None:
        obj.data.materials.append(material)
    move_to_collection(obj, _target_collection(ctx, target))
    parent_to_master(obj, ctx.master)
    return obj


def linked_mesh_copy(
    ctx: StudioContext,
    source: bpy.types.Object,
    name: str,
    location: tuple[float, float, float],
    target: str | bpy.types.Collection,
    *,
    rotation: tuple[float, float, float] | None = None,
) -> bpy.types.Object:
    """Place a linked mesh instance while retaining editable object transforms."""

    obj = source.copy()
    obj.data = source.data
    obj.name = name
    obj.location = location
    if rotation is not None:
        obj.rotation_euler = rotation
    _target_collection(ctx, target).objects.link(obj)
    parent_to_master(obj, ctx.master)
    return obj


def parent_assembly(
    root: bpy.types.Object,
    parts: list[bpy.types.Object],
) -> bpy.types.Object:
    """Make assembly parts descend from an already mastered semantic root."""

    for part in parts:
        if part != root:
            parent_to_master(part, root)
    return root


def tag_wood_grain(obj: bpy.types.Object, axis: str) -> bpy.types.Object:
    """Assign an axis-oriented procedural wood variant and record its direction."""

    obj["grain_axis"] = axis
    if obj.type != "MESH":
        return obj
    for index, material in enumerate(obj.data.materials):
        family = material.get("wood_family")
        if not family:
            continue
        canonical = bpy.data.materials.get(str(family))
        if canonical is None:
            continue
        if axis == canonical.get("grain_axis"):
            target = canonical
        elif axis == "END":
            target = bpy.data.materials.get(f"{family}_EndGrain")
        else:
            target = bpy.data.materials.get(f"{family}_Grain{axis}")
        if target is not None:
            obj.data.materials[index] = target
    return obj


def look_at(obj: bpy.types.Object, target: tuple[float, float, float]) -> None:
    """Aim an object's local negative-Z axis at a world-space target."""

    direction = Vector(target) - obj.location
    obj.rotation_euler = direction.to_track_quat("-Z", "Y").to_euler()


def _set_principled_input(
    shader: bpy.types.ShaderNodeBsdfPrincipled,
    name: str,
    value: float | tuple[float, float, float, float],
) -> None:
    socket = shader.inputs.get(name)
    if socket is not None:
        socket.default_value = value


def _principled_material(
    name: str,
    base_color: tuple[float, float, float, float],
    *,
    roughness: float,
    metallic: float = 0.0,
    alpha: float = 1.0,
    transmission: float = 0.0,
) -> bpy.types.Material:
    material = bpy.data.materials.get(name) or bpy.data.materials.new(name)
    material.use_nodes = True
    nodes = material.node_tree.nodes
    nodes.clear()
    output = nodes.new("ShaderNodeOutputMaterial")
    shader = nodes.new("ShaderNodeBsdfPrincipled")
    _set_principled_input(shader, "Base Color", base_color)
    _set_principled_input(shader, "Roughness", roughness)
    _set_principled_input(shader, "Metallic", metallic)
    _set_principled_input(shader, "Alpha", alpha)
    _set_principled_input(shader, "Transmission Weight", transmission)
    _set_principled_input(shader, "Transmission", transmission)
    material.node_tree.links.new(shader.outputs["BSDF"], output.inputs["Surface"])
    if alpha < 1.0:
        try:
            material.surface_render_method = "DITHERED"
        except (AttributeError, TypeError, ValueError):
            try:
                material.blend_method = "BLEND"
            except (AttributeError, TypeError, ValueError):
                pass
        material.diffuse_color = (*base_color[:3], alpha)
        material.use_transparency_overlap = False
    return material


def _material_nodes(
    name: str,
    base_color: tuple[float, float, float, float],
    *,
    roughness: float,
    metallic: float = 0.0,
) -> tuple[
    bpy.types.Material,
    bpy.types.ShaderNodeBsdfPrincipled,
    bpy.types.Node,
]:
    """Create a clean Principled graph and return its material, shader, and output."""

    material = bpy.data.materials.get(name) or bpy.data.materials.new(name)
    material.use_nodes = True
    nodes = material.node_tree.nodes
    nodes.clear()
    output = nodes.new("ShaderNodeOutputMaterial")
    shader = nodes.new("ShaderNodeBsdfPrincipled")
    _set_principled_input(shader, "Base Color", base_color)
    _set_principled_input(shader, "Roughness", roughness)
    _set_principled_input(shader, "Metallic", metallic)
    material.node_tree.links.new(shader.outputs["BSDF"], output.inputs["Surface"])
    return material, shader, output


def _procedural_wood_material(
    name: str,
    light_color: tuple[float, float, float, float],
    dark_color: tuple[float, float, float, float],
    *,
    roughness: float,
    grain_scale: float,
    grain_axis: str = "X",
    wood_family: str | None = None,
    end_grain: bool = False,
) -> bpy.types.Material:
    """Build directional procedural timber from Generated coordinates."""

    material, shader, _output = _material_nodes(
        name, light_color, roughness=roughness
    )
    nodes = material.node_tree.nodes
    links = material.node_tree.links
    coordinates = nodes.new("ShaderNodeTexCoord")
    coordinates.name = "Wood_Generated_Coordinates"
    mapping = nodes.new("ShaderNodeMapping")
    mapping.name = "Wood_Grain_Mapping"
    if end_grain:
        mapping_scale = (grain_scale * 0.72,) * 3
    else:
        grain_index = {"X": 0, "Y": 1, "Z": 2}[grain_axis]
        mapping_scale_values = [grain_scale, grain_scale, grain_scale]
        mapping_scale_values[grain_index] = 0.55
        mapping_scale = tuple(mapping_scale_values)
    mapping.inputs["Scale"].default_value = mapping_scale
    mapping.inputs["Rotation"].default_value = (0.0, 0.0, 0.0)
    object_info = nodes.new("ShaderNodeObjectInfo")
    object_info.name = "Wood_PerObject_Info"
    phase_scale = nodes.new("ShaderNodeVectorMath")
    phase_scale.name = "Wood_Object_Phase_Scale"
    phase_scale.operation = "SCALE"
    phase_scale.inputs[0].default_value = (0.37, 0.61, 0.83)
    phase_offset = nodes.new("ShaderNodeVectorMath")
    phase_offset.name = "Wood_Object_Phase_Offset"
    phase_offset.operation = "ADD"

    warp = nodes.new("ShaderNodeTexNoise")
    warp.name = "Wood_Coarse_Warp"
    warp.noise_dimensions = "4D"
    warp.inputs["Scale"].default_value = 0.52 if end_grain else 0.62
    warp.inputs["Detail"].default_value = 2.2
    warp.inputs["Roughness"].default_value = 0.50
    warp_center = nodes.new("ShaderNodeVectorMath")
    warp_center.name = "Wood_Warp_Center"
    warp_center.operation = "SUBTRACT"
    warp_center.inputs[1].default_value = (0.5, 0.5, 0.5)
    warp_strength = nodes.new("ShaderNodeVectorMath")
    warp_strength.name = "Wood_Warp_Strength"
    warp_strength.operation = "SCALE"
    warp_strength.inputs["Scale"].default_value = 0.13 if end_grain else 0.18
    warped_coordinates = nodes.new("ShaderNodeVectorMath")
    warped_coordinates.name = "Wood_Warped_Coordinates"
    warped_coordinates.operation = "ADD"

    fine_grain = nodes.new("ShaderNodeTexNoise")
    fine_grain.name = "Wood_Fine_Grain"
    fine_grain.noise_dimensions = "4D"
    fine_grain.inputs["Scale"].default_value = 2.35 if end_grain else 3.15
    fine_grain.inputs["Detail"].default_value = 3.4
    fine_grain.inputs["Roughness"].default_value = 0.56

    variation = nodes.new("ShaderNodeTexNoise")
    variation.name = "Wood_Low_Frequency_Variation"
    variation.noise_dimensions = "4D"
    variation.inputs["Scale"].default_value = 0.48 if end_grain else 0.72
    variation.inputs["Detail"].default_value = 2.0
    variation.inputs["Roughness"].default_value = 0.48

    for noise_node, phase_multiplier, suffix in (
        (warp, 3.17, "Warp"),
        (fine_grain, 11.73, "Fine"),
        (variation, 1.91, "Variation"),
    ):
        phase = nodes.new("ShaderNodeMath")
        phase.name = f"Wood_Object_Phase_{suffix}"
        phase.operation = "MULTIPLY"
        phase.inputs[1].default_value = phase_multiplier
        links.new(object_info.outputs["Random"], phase.inputs[0])
        links.new(phase.outputs["Value"], noise_node.inputs["W"])

    mix = nodes.new("ShaderNodeMixRGB")
    mix.name = "Wood_Grain_Mix"
    mix.blend_type = "MULTIPLY"
    mix.inputs[0].default_value = 0.20
    ramp = nodes.new("ShaderNodeValToRGB")
    ramp.name = "Wood_Tone_Ramp"
    ramp.color_ramp.elements[0].position = 0.24
    ramp.color_ramp.elements[0].color = tuple(
        dark_color[index] * 0.32 + light_color[index] * 0.68
        for index in range(4)
    )
    ramp.color_ramp.elements[1].position = 0.82
    ramp.color_ramp.elements[1].color = tuple(
        dark_color[index] * 0.02 + light_color[index] * 0.98
        for index in range(4)
    )
    bump = nodes.new("ShaderNodeBump")
    bump.name = "Wood_Micrograin_Bump"
    bump.inputs["Strength"].default_value = 0.040
    bump.inputs["Distance"].default_value = 0.007
    links.new(coordinates.outputs["Generated"], mapping.inputs["Vector"])
    links.new(object_info.outputs["Random"], phase_scale.inputs["Scale"])
    links.new(mapping.outputs["Vector"], phase_offset.inputs[0])
    links.new(phase_scale.outputs["Vector"], phase_offset.inputs[1])
    links.new(phase_offset.outputs["Vector"], warp.inputs["Vector"])
    links.new(warp.outputs["Color"], warp_center.inputs[0])
    links.new(warp_center.outputs["Vector"], warp_strength.inputs[0])
    links.new(phase_offset.outputs["Vector"], warped_coordinates.inputs[0])
    links.new(warp_strength.outputs["Vector"], warped_coordinates.inputs[1])
    links.new(warped_coordinates.outputs["Vector"], fine_grain.inputs["Vector"])
    links.new(warped_coordinates.outputs["Vector"], variation.inputs["Vector"])
    links.new(fine_grain.outputs["Fac"], mix.inputs[1])
    links.new(variation.outputs["Fac"], mix.inputs[2])
    links.new(mix.outputs["Color"], ramp.inputs["Fac"])
    links.new(ramp.outputs["Color"], shader.inputs["Base Color"])
    links.new(fine_grain.outputs["Fac"], bump.inputs["Height"])
    links.new(bump.outputs["Normal"], shader.inputs["Normal"])
    material["texture_space"] = "Generated"
    material["directional_grain"] = True
    material["grain_axis"] = grain_axis
    material["wood_family"] = wood_family or name
    material["end_grain"] = end_grain
    material["grain_model"] = "nonperiodic_multiscale_4d_noise"
    return material


def _procedural_ceramic_material() -> bpy.types.Material:
    """Build sand ceramic with linked micro-roughness and subtle tactile bump."""

    material, shader, _output = _material_nodes(
        "Ceramic_Sand", (0.62, 0.43, 0.25, 1.0), roughness=0.34
    )
    nodes = material.node_tree.nodes
    links = material.node_tree.links
    coordinates = nodes.new("ShaderNodeTexCoord")
    mapping = nodes.new("ShaderNodeMapping")
    mapping.name = "Ceramic_Micro_Mapping"
    noise = nodes.new("ShaderNodeTexNoise")
    noise.name = "Ceramic_Micro_Roughness"
    noise.inputs["Scale"].default_value = 82.0
    noise.inputs["Detail"].default_value = 2.4
    noise.inputs["Roughness"].default_value = 0.58
    roughness_ramp = nodes.new("ShaderNodeValToRGB")
    roughness_ramp.name = "Ceramic_Roughness_Range"
    roughness_ramp.color_ramp.elements[0].color = (0.28, 0.28, 0.28, 1.0)
    roughness_ramp.color_ramp.elements[1].color = (0.43, 0.43, 0.43, 1.0)
    bump = nodes.new("ShaderNodeBump")
    bump.name = "Ceramic_Micro_Bump"
    bump.inputs["Strength"].default_value = 0.11
    bump.inputs["Distance"].default_value = 0.018
    links.new(coordinates.outputs["Generated"], mapping.inputs["Vector"])
    links.new(mapping.outputs["Vector"], noise.inputs["Vector"])
    links.new(noise.outputs["Fac"], roughness_ramp.inputs["Fac"])
    links.new(roughness_ramp.outputs["Color"], shader.inputs["Roughness"])
    links.new(noise.outputs["Fac"], bump.inputs["Height"])
    links.new(bump.outputs["Normal"], shader.inputs["Normal"])
    return material


def _procedural_plaster_material() -> bpy.types.Material:
    """Create warm plaster with a restrained low-frequency tactile bump."""

    material, shader, _output = _material_nodes(
        "Wall_WarmPlaster", (0.73, 0.59, 0.44, 1.0), roughness=0.72
    )
    nodes = material.node_tree.nodes
    links = material.node_tree.links
    coordinates = nodes.new("ShaderNodeTexCoord")
    mapping = nodes.new("ShaderNodeMapping")
    mapping.name = "Plaster_Mapping"
    noise = nodes.new("ShaderNodeTexNoise")
    noise.name = "Plaster_LowFrequency_Noise"
    noise.inputs["Scale"].default_value = 3.2
    noise.inputs["Detail"].default_value = 2.2
    noise.inputs["Roughness"].default_value = 0.48
    bump = nodes.new("ShaderNodeBump")
    bump.name = "Plaster_Subtle_Bump"
    bump.inputs["Strength"].default_value = 0.12
    bump.inputs["Distance"].default_value = 0.045
    links.new(coordinates.outputs["Generated"], mapping.inputs["Vector"])
    links.new(mapping.outputs["Vector"], noise.inputs["Vector"])
    links.new(noise.outputs["Fac"], bump.inputs["Height"])
    links.new(bump.outputs["Normal"], shader.inputs["Normal"])
    return material


def _procedural_fabric_material(
    name: str,
    base_color: tuple[float, float, float, float],
    *,
    roughness: float,
    alpha: float = 1.0,
    transmission: float = 0.0,
) -> bpy.types.Material:
    """Build a texture-free fabric with fine crossed weave relief."""

    material = _principled_material(
        name,
        base_color,
        roughness=roughness,
        alpha=alpha,
        transmission=transmission,
    )
    nodes = material.node_tree.nodes
    links = material.node_tree.links
    shader = next(node for node in nodes if node.type == "BSDF_PRINCIPLED")
    coordinates = nodes.new("ShaderNodeTexCoord")
    mapping = nodes.new("ShaderNodeMapping")
    mapping.name = f"{name}_Mapping"
    noise = nodes.new("ShaderNodeTexNoise")
    noise.name = f"{name}_Fiber_Noise"
    noise.inputs["Scale"].default_value = 125.0
    noise.inputs["Detail"].default_value = 2.0
    bump = nodes.new("ShaderNodeBump")
    bump.name = f"{name}_Fiber_Bump"
    bump.inputs["Strength"].default_value = 0.10
    bump.inputs["Distance"].default_value = 0.012
    links.new(coordinates.outputs["Generated"], mapping.inputs["Vector"])
    links.new(mapping.outputs["Vector"], noise.inputs["Vector"])
    links.new(noise.outputs["Fac"], bump.inputs["Height"])
    links.new(bump.outputs["Normal"], shader.inputs["Normal"])
    material.use_backface_culling = False
    return material


def _procedural_jute_material() -> bpy.types.Material:
    """Create coarse warm jute with procedural strand variation."""

    material, shader, _output = _material_nodes(
        "Rug_Jute", (0.47, 0.27, 0.11, 1.0), roughness=0.79
    )
    nodes = material.node_tree.nodes
    links = material.node_tree.links
    coordinates = nodes.new("ShaderNodeTexCoord")
    mapping = nodes.new("ShaderNodeMapping")
    mapping.name = "Jute_Mapping"
    wave = nodes.new("ShaderNodeTexWave")
    wave.name = "Jute_Parallel_Strands"
    wave.wave_type = "BANDS"
    wave.bands_direction = "X"
    wave.inputs["Scale"].default_value = 65.0
    wave.inputs["Distortion"].default_value = 3.0
    noise = nodes.new("ShaderNodeTexNoise")
    noise.name = "Jute_Color_Variation"
    noise.inputs["Scale"].default_value = 8.0
    noise.inputs["Detail"].default_value = 3.0
    ramp = nodes.new("ShaderNodeValToRGB")
    ramp.color_ramp.elements[0].color = (0.23, 0.105, 0.035, 1.0)
    ramp.color_ramp.elements[1].color = (0.64, 0.40, 0.18, 1.0)
    bump = nodes.new("ShaderNodeBump")
    bump.inputs["Strength"].default_value = 0.24
    bump.inputs["Distance"].default_value = 0.026
    links.new(coordinates.outputs["Generated"], mapping.inputs["Vector"])
    links.new(mapping.outputs["Vector"], wave.inputs["Vector"])
    links.new(mapping.outputs["Vector"], noise.inputs["Vector"])
    links.new(noise.outputs["Fac"], ramp.inputs["Fac"])
    links.new(ramp.outputs["Color"], shader.inputs["Base Color"])
    links.new(wave.outputs["Fac"], bump.inputs["Height"])
    links.new(bump.outputs["Normal"], shader.inputs["Normal"])
    return material


def _procedural_floor_leaf_material(
    name: str,
    dark_color: tuple[float, float, float, float],
    light_color: tuple[float, float, float, float],
    *,
    roughness: float,
) -> bpy.types.Material:
    """Create subtle paired foliage variation without per-object randomness."""

    material, shader, _output = _material_nodes(name, dark_color, roughness=roughness)
    nodes = material.node_tree.nodes
    links = material.node_tree.links
    coordinates = nodes.new("ShaderNodeTexCoord")
    coordinates.name = f"{name}_Generated_Coordinates"
    noise = nodes.new("ShaderNodeTexNoise")
    noise.name = f"{name}_Soft_Mottling"
    noise.inputs["Scale"].default_value = 3.4
    noise.inputs["Detail"].default_value = 2.2
    noise.inputs["Roughness"].default_value = 0.54
    color_ramp = nodes.new("ShaderNodeValToRGB")
    color_ramp.name = f"{name}_Color_Range"
    color_ramp.color_ramp.elements[0].position = 0.23
    color_ramp.color_ramp.elements[0].color = dark_color
    color_ramp.color_ramp.elements[1].position = 0.81
    color_ramp.color_ramp.elements[1].color = light_color
    roughness_ramp = nodes.new("ShaderNodeValToRGB")
    roughness_ramp.name = f"{name}_Roughness_Range"
    lower_roughness = max(0.0, roughness - 0.035)
    upper_roughness = min(1.0, roughness + 0.045)
    roughness_ramp.color_ramp.elements[0].color = (
        lower_roughness,
        lower_roughness,
        lower_roughness,
        1.0,
    )
    roughness_ramp.color_ramp.elements[1].color = (
        upper_roughness,
        upper_roughness,
        upper_roughness,
        1.0,
    )
    links.new(coordinates.outputs["Generated"], noise.inputs["Vector"])
    links.new(noise.outputs["Fac"], color_ramp.inputs["Fac"])
    links.new(color_ramp.outputs["Color"], shader.inputs["Base Color"])
    links.new(noise.outputs["Fac"], roughness_ramp.inputs["Fac"])
    links.new(roughness_ramp.outputs["Color"], shader.inputs["Roughness"])
    material["foliage_pairing"] = "object_linked_mirrored_variant"
    return material


def build_material_library() -> dict[str, bpy.types.Material]:
    """Create the texture-free, Blender 5.1-compatible studio material library."""

    materials = {
        "Wall_WarmPlaster": _procedural_plaster_material(),
        "Desk_WarmOak": _procedural_wood_material(
            "Desk_WarmOak",
            (0.48, 0.245, 0.075, 1.0),
            (0.17, 0.060, 0.018, 1.0),
            roughness=0.42,
            grain_scale=7.5,
        ),
        "Slat_Walnut": _procedural_wood_material(
            "Slat_Walnut",
            (0.235, 0.095, 0.032, 1.0),
            (0.055, 0.018, 0.009, 1.0),
            roughness=0.47,
            grain_scale=9.0,
        ),
        "Floor_LightOak": _procedural_wood_material(
            "Floor_LightOak",
            (0.68, 0.43, 0.20, 1.0),
            (0.28, 0.12, 0.04, 1.0),
            roughness=0.50,
            grain_scale=8.0,
            grain_axis="Y",
        ),
        "Trim_WarmWhite": _principled_material(
            "Trim_WarmWhite", (0.88, 0.80, 0.67, 1.0), roughness=0.58
        ),
        "Glass_Window": _principled_material(
            "Glass_Window",
            (0.68, 0.82, 0.86, 1.0),
            roughness=0.08,
            alpha=0.22,
            transmission=0.92,
        ),
        "Metal_DarkBrown": _principled_material(
            "Metal_DarkBrown", (0.09, 0.045, 0.025, 1.0), roughness=0.31, metallic=0.76
        ),
        "Curtain_Sheer": _procedural_fabric_material(
            "Curtain_Sheer",
            (0.94, 0.89, 0.79, 1.0),
            roughness=0.76,
            alpha=0.34,
            transmission=0.24,
        ),
        "Curtain_Outer": _procedural_fabric_material(
            "Curtain_Outer", (0.74, 0.62, 0.46, 1.0), roughness=0.74
        ),
        "Ceramic_Sand": _procedural_ceramic_material(),
        "Rug_Jute": _procedural_jute_material(),
        "Book_Mustard": _principled_material(
            "Book_Mustard", (0.54, 0.31, 0.055, 1.0), roughness=0.52
        ),
        "Book_Brown": _principled_material(
            "Book_Brown", (0.24, 0.075, 0.030, 1.0), roughness=0.56
        ),
        "Book_Gray": _principled_material(
            "Book_Gray", (0.30, 0.27, 0.23, 1.0), roughness=0.58
        ),
        "Book_Olive": _principled_material(
            "Book_Olive", (0.27, 0.29, 0.09, 1.0), roughness=0.57
        ),
        "Paper_Warm": _principled_material(
            "Paper_Warm", (0.82, 0.70, 0.53, 1.0), roughness=0.68
        ),
        "Leaf_MutedGreen": _principled_material(
            "Leaf_MutedGreen", (0.16, 0.29, 0.075, 1.0), roughness=0.65
        ),
        "Leaf_Olive": _principled_material(
            "Leaf_Olive", (0.29, 0.32, 0.085, 1.0), roughness=0.67
        ),
        "Stem_DarkGreen": _principled_material(
            "Stem_DarkGreen", (0.07, 0.15, 0.035, 1.0), roughness=0.69
        ),
        "FloorLeaf_Sage_A": _procedural_floor_leaf_material(
            "FloorLeaf_Sage_A",
            (0.105, 0.205, 0.040, 1.0),
            (0.190, 0.305, 0.075, 1.0),
            roughness=0.63,
        ),
        "FloorLeaf_Sage_B": _procedural_floor_leaf_material(
            "FloorLeaf_Sage_B",
            (0.135, 0.225, 0.045, 1.0),
            (0.235, 0.330, 0.085, 1.0),
            roughness=0.66,
        ),
        "FloorLeaf_Sage_C": _procedural_floor_leaf_material(
            "FloorLeaf_Sage_C",
            (0.165, 0.235, 0.048, 1.0),
            (0.275, 0.340, 0.090, 1.0),
            roughness=0.68,
        ),
        "Basket_Wicker": _procedural_fabric_material(
            "Basket_Wicker", (0.43, 0.235, 0.085, 1.0), roughness=0.64
        ),
        "Lamp_Shade_Warm": _procedural_fabric_material(
            "Lamp_Shade_Warm", (0.80, 0.60, 0.34, 1.0), roughness=0.67
        ),
        "Lamp_Emissive_Warm": _principled_material(
            "Lamp_Emissive_Warm", (1.0, 0.56, 0.20, 1.0), roughness=0.30
        ),
        "Chair_Fabric": _procedural_fabric_material(
            "Chair_Fabric", (0.43, 0.25, 0.13, 1.0), roughness=0.70
        ),
        "Portrait_Background": _principled_material(
            "Portrait_Background", (0.64, 0.46, 0.29, 1.0), roughness=0.66
        ),
        "Portrait_Dark": _principled_material(
            "Portrait_Dark", (0.15, 0.055, 0.025, 1.0), roughness=0.58
        ),
    }
    wood_families = {
        "Desk_WarmOak": {
            "light": (0.48, 0.245, 0.075, 1.0),
            "dark": (0.17, 0.060, 0.018, 1.0),
            "roughness": 0.42,
            "scale": 7.5,
            "canonical_axis": "X",
        },
        "Slat_Walnut": {
            "light": (0.235, 0.095, 0.032, 1.0),
            "dark": (0.055, 0.018, 0.009, 1.0),
            "roughness": 0.47,
            "scale": 9.0,
            "canonical_axis": "X",
        },
        "Floor_LightOak": {
            "light": (0.68, 0.43, 0.20, 1.0),
            "dark": (0.28, 0.12, 0.04, 1.0),
            "roughness": 0.50,
            "scale": 8.0,
            "canonical_axis": "Y",
        },
    }
    for family, spec in wood_families.items():
        canonical = materials[family]
        canonical["grain_axis"] = spec["canonical_axis"]
        canonical["wood_family"] = family
        for axis in ("X", "Y", "Z"):
            if axis == spec["canonical_axis"]:
                continue
            variant_name = f"{family}_Grain{axis}"
            materials[variant_name] = _procedural_wood_material(
                variant_name,
                spec["light"],
                spec["dark"],
                roughness=spec["roughness"],
                grain_scale=spec["scale"],
                grain_axis=axis,
                wood_family=family,
            )
    materials["Desk_WarmOak_EndGrain"] = _procedural_wood_material(
        "Desk_WarmOak_EndGrain",
        (0.50, 0.27, 0.095, 1.0),
        (0.12, 0.032, 0.010, 1.0),
        roughness=0.46,
        grain_scale=8.8,
        grain_axis="END",
        wood_family="Desk_WarmOak",
        end_grain=True,
    )
    lamp_shader = next(
        node
        for node in materials["Lamp_Emissive_Warm"].node_tree.nodes
        if node.type == "BSDF_PRINCIPLED"
    )
    _set_principled_input(lamp_shader, "Emission Color", (1.0, 0.38, 0.08, 1.0))
    _set_principled_input(lamp_shader, "Emission", (1.0, 0.38, 0.08, 1.0))
    _set_principled_input(lamp_shader, "Emission Strength", 1.08)
    shade_shader = next(
        node
        for node in materials["Lamp_Shade_Warm"].node_tree.nodes
        if node.type == "BSDF_PRINCIPLED"
    )
    _set_principled_input(shade_shader, "Transmission Weight", 0.18)
    _set_principled_input(shade_shader, "Transmission", 0.18)
    _set_principled_input(shade_shader, "Emission Color", (0.72, 0.28, 0.07, 1.0))
    _set_principled_input(shade_shader, "Emission", (0.72, 0.28, 0.07, 1.0))
    _set_principled_input(shade_shader, "Emission Strength", 0.072)
    return materials


def create_scene_context() -> StudioContext:
    """Reset Blender and initialize the complete semantic scene foundation."""

    bpy.ops.wm.read_factory_settings(use_empty=True)
    scene = bpy.context.scene
    scene.name = "Scene_Warm_Sloth_Studio"
    scene.unit_settings.system = "METRIC"
    scene.unit_settings.length_unit = "METERS"
    scene.unit_settings.scale_length = 1.0
    scene["ip_scene_contract"] = "tangying-warm-sloth-studio/v1"
    scene["ip_authored_exposure"] = AUTHORED_EXPOSURE

    collections = {
        name: ensure_collection(scene, name) for name in contract.REQUIRED_COLLECTIONS
    }
    master = bpy.data.objects.new("SET_MASTER", None)
    collections["STUDIO_MARKERS"].objects.link(master)
    master.empty_display_type = "PLAIN_AXES"
    master.empty_display_size = 0.5
    master["room_size"] = json.dumps(list(contract.ROOM_SIZE))
    master["contract"] = "studio_set_master"

    return StudioContext(
        scene=scene,
        master=master,
        collections=collections,
        materials=build_material_library(),
    )


def build_back_wall(ctx: StudioContext) -> bpy.types.Object:
    """Build the full-width rear wall at the positive-Y room boundary."""

    width, depth, height = contract.ROOM_SIZE
    return add_box(
        ctx,
        "Wall_Back",
        (0.0, depth / 2.0, height / 2.0),
        (width, WALL_THICKNESS, height),
        ctx.materials["Wall_WarmPlaster"],
    )


def build_front_wall(ctx: StudioContext) -> list[bpy.types.Object]:
    """Build a complete front wall as two editable full-height segments."""

    width, depth, height = contract.ROOM_SIZE
    half_segment = width / 2.0
    return [
        add_box(
            ctx,
            "Wall_Front_Left",
            (-width / 4.0, -depth / 2.0, height / 2.0),
            (half_segment, WALL_THICKNESS, height),
            ctx.materials["Wall_WarmPlaster"],
        ),
        add_box(
            ctx,
            "Wall_Front_Right",
            (width / 4.0, -depth / 2.0, height / 2.0),
            (half_segment, WALL_THICKNESS, height),
            ctx.materials["Wall_WarmPlaster"],
        ),
    ]


def build_left_window_wall(
    ctx: StudioContext,
    clear_width: float = 2.6,
    clear_height: float = 2.75,
) -> list[bpy.types.Object]:
    """Build the left wall from four segments around a genuine opening."""

    width, depth, height = contract.ROOM_SIZE
    wall_x = -width / 2.0
    opening_front = WINDOW_CENTER_Y - clear_width / 2.0
    opening_rear = WINDOW_CENTER_Y + clear_width / 2.0
    opening_top = WINDOW_SILL_HEIGHT + clear_height
    front_length = opening_front + depth / 2.0
    rear_length = depth / 2.0 - opening_rear
    material = ctx.materials["Wall_WarmPlaster"]
    return [
        add_box(
            ctx,
            "Wall_Left_WindowFront",
            (wall_x, -depth / 2.0 + front_length / 2.0, height / 2.0),
            (WALL_THICKNESS, front_length, height),
            material,
        ),
        add_box(
            ctx,
            "Wall_Left_WindowRear",
            (wall_x, opening_rear + rear_length / 2.0, height / 2.0),
            (WALL_THICKNESS, rear_length, height),
            material,
        ),
        add_box(
            ctx,
            "Wall_Left_WindowLower",
            (wall_x, WINDOW_CENTER_Y, WINDOW_SILL_HEIGHT / 2.0),
            (WALL_THICKNESS, clear_width, WINDOW_SILL_HEIGHT),
            material,
        ),
        add_box(
            ctx,
            "Wall_Left_WindowUpper",
            (wall_x, WINDOW_CENTER_Y, opening_top + (height - opening_top) / 2.0),
            (WALL_THICKNESS, clear_width, height - opening_top),
            material,
        ),
    ]


def build_right_door_wall(
    ctx: StudioContext,
    clear_width: float = 1.0,
    clear_height: float = 2.4,
) -> list[bpy.types.Object]:
    """Build the right wall around a camera-side door opening."""

    width, depth, height = contract.ROOM_SIZE
    wall_x = width / 2.0
    opening_front = DOOR_CENTER_Y - clear_width / 2.0
    opening_rear = DOOR_CENTER_Y + clear_width / 2.0
    front_length = opening_front + depth / 2.0
    rear_length = depth / 2.0 - opening_rear
    material = ctx.materials["Wall_WarmPlaster"]
    return [
        add_box(
            ctx,
            "Wall_Right_DoorFront",
            (wall_x, -depth / 2.0 + front_length / 2.0, height / 2.0),
            (WALL_THICKNESS, front_length, height),
            material,
        ),
        add_box(
            ctx,
            "Wall_Right_DoorRear",
            (wall_x, opening_rear + rear_length / 2.0, height / 2.0),
            (WALL_THICKNESS, rear_length, height),
            material,
        ),
        add_box(
            ctx,
            "Wall_Right_DoorLintel",
            (wall_x, DOOR_CENTER_Y, clear_height + (height - clear_height) / 2.0),
            (WALL_THICKNESS, clear_width, height - clear_height),
            material,
        ),
    ]


def build_baseboards_and_crown(ctx: StudioContext) -> list[bpy.types.Object]:
    """Add restrained perimeter baseboards and crown trim."""

    width, depth, height = contract.ROOM_SIZE
    trim = ctx.materials["Trim_WarmWhite"]
    objects = [
        add_box(ctx, "Baseboard_Back", (0.0, depth / 2.0 - 0.10, 0.06), (width, 0.04, 0.12), trim, bevel=0.008),
        add_box(ctx, "Baseboard_Front", (0.0, -depth / 2.0 + 0.10, 0.06), (width, 0.04, 0.12), trim, bevel=0.008),
        add_box(ctx, "Baseboard_Left", (-width / 2.0 + 0.10, 0.0, 0.06), (0.04, depth - 0.20, 0.12), trim, bevel=0.008),
        add_box(ctx, "Crown_Back", (0.0, depth / 2.0 - 0.10, height - 0.055), (width, 0.055, 0.11), trim, bevel=0.012),
        add_box(ctx, "Crown_Front", (0.0, -depth / 2.0 + 0.10, height - 0.055), (width, 0.055, 0.11), trim, bevel=0.012),
        add_box(ctx, "Crown_Left", (-width / 2.0 + 0.10, 0.0, height - 0.055), (0.055, depth - 0.20, 0.11), trim, bevel=0.012),
        add_box(ctx, "Crown_Right", (width / 2.0 - 0.10, 0.0, height - 0.055), (0.055, depth - 0.20, 0.11), trim, bevel=0.012),
    ]

    door_front = DOOR_CENTER_Y - contract.DOOR_OPENING[0] / 2.0
    door_rear = DOOR_CENTER_Y + contract.DOOR_OPENING[0] / 2.0
    front_length = door_front + depth / 2.0 - 0.10
    rear_length = depth / 2.0 - door_rear - 0.10
    objects.extend(
        [
            add_box(
                ctx,
                "Baseboard_Right_Front",
                (width / 2.0 - 0.10, -depth / 2.0 + front_length / 2.0, 0.06),
                (0.04, front_length, 0.12),
                trim,
                bevel=0.008,
            ),
            add_box(
                ctx,
                "Baseboard_Right_Rear",
                (width / 2.0 - 0.10, door_rear + rear_length / 2.0, 0.06),
                (0.04, rear_length, 0.12),
                trim,
                bevel=0.008,
            ),
        ]
    )
    return objects


def build_floorboards(
    ctx: StudioContext,
    board_width: float = 0.18,
    gap: float = 0.004,
) -> list[bpy.types.Object]:
    """Lay exact-width oak board modules with explicit edit-friendly gaps."""

    width, depth, _height = contract.ROOM_SIZE
    pitch = board_width + gap
    count = max(1, math.floor((width + gap) / pitch))
    covered_width = count * board_width + (count - 1) * gap
    first_x = -covered_width / 2.0 + board_width / 2.0
    boards = []
    for index in range(count):
        board = add_box(
            ctx,
            f"Floorboard_{index + 1:03d}",
            (first_x + index * pitch, 0.0, 0.006),
            (board_width, depth - 0.20, 0.012),
            ctx.materials["Floor_LightOak"],
            bevel=0.0015,
        )
        board["board_width"] = board_width
        board["board_gap"] = gap
        tag_wood_grain(board, "Y")
        boards.append(board)
    return boards


def _add_curtain_mesh(
    ctx: StudioContext,
    name: str,
    center_y: float,
    width: float,
    bottom: float,
    top: float,
    base_x: float,
    amplitude: float,
    folds: int,
    material: bpy.types.Material,
) -> bpy.types.Object:
    segments = max(8, folds * 4)
    vertices: list[tuple[float, float, float]] = []
    faces: list[tuple[int, int, int, int]] = []
    for index in range(segments + 1):
        ratio = index / segments
        y = center_y - width / 2.0 + width * ratio
        x = base_x + amplitude * math.sin(ratio * folds * math.tau)
        vertices.extend(((x, y, bottom), (x, y, top)))
    for index in range(segments):
        lower = index * 2
        faces.append((lower, lower + 2, lower + 3, lower + 1))

    mesh = bpy.data.meshes.new(name)
    mesh.from_pydata(vertices, [], faces)
    mesh.validate()
    mesh.update()
    mesh.materials.append(material)
    obj = bpy.data.objects.new(name, mesh)
    ctx.collections["STUDIO_ARCHITECTURE"].objects.link(obj)
    parent_to_master(obj, ctx.master)
    solidify = obj.modifiers.new("Frozen_Fabric_Thickness", "SOLIDIFY")
    solidify.thickness = 0.006
    solidify.offset = 0.0
    add_bevel(obj, 0.003, segments=2)
    return obj


def build_window_frames_and_curtains(
    ctx: StudioContext,
    clear_width: float = 2.6,
    clear_height: float = 2.75,
) -> list[bpy.types.Object]:
    """Build a framed glazed opening and frozen layered curtain geometry."""

    room_width, _depth, _height = contract.ROOM_SIZE
    wall_x = -room_width / 2.0
    opening_mid_z = WINDOW_SILL_HEIGHT + clear_height / 2.0
    frame_width = 0.075
    trim = ctx.materials["Trim_WarmWhite"]

    frame = bpy.data.objects.new("Window_Frame", None)
    ctx.collections["STUDIO_ARCHITECTURE"].objects.link(frame)
    frame.location = (wall_x, WINDOW_CENTER_Y, opening_mid_z)
    frame.empty_display_type = "CUBE"
    frame.empty_display_size = 0.14
    frame["clear_width"] = clear_width
    frame["clear_height"] = clear_height
    parent_to_master(frame, ctx.master)

    pieces = [
        add_box(
            ctx,
            "Window_Frame_FrontJamb",
            (wall_x, WINDOW_CENTER_Y - clear_width / 2.0 - frame_width / 2.0, opening_mid_z),
            (0.20, frame_width, clear_height + frame_width * 2.0),
            trim,
            bevel=0.008,
        ),
        add_box(
            ctx,
            "Window_Frame_RearJamb",
            (wall_x, WINDOW_CENTER_Y + clear_width / 2.0 + frame_width / 2.0, opening_mid_z),
            (0.20, frame_width, clear_height + frame_width * 2.0),
            trim,
            bevel=0.008,
        ),
        add_box(
            ctx,
            "Window_Frame_Header",
            (wall_x, WINDOW_CENTER_Y, WINDOW_SILL_HEIGHT + clear_height + frame_width / 2.0),
            (0.20, clear_width, frame_width),
            trim,
            bevel=0.008,
        ),
        add_box(
            ctx,
            "Window_Frame_Lower",
            (wall_x, WINDOW_CENTER_Y, WINDOW_SILL_HEIGHT - frame_width / 2.0),
            (0.20, clear_width, frame_width),
            trim,
            bevel=0.008,
        ),
    ]
    for piece in pieces:
        parent_to_master(piece, frame)

    glass = add_box(
        ctx,
        "Glass_Window",
        (wall_x, WINDOW_CENTER_Y, opening_mid_z),
        (0.018, clear_width - 0.05, clear_height - 0.05),
        ctx.materials["Glass_Window"],
        bevel=0.003,
    )
    sill = add_box(
        ctx,
        "Window_Sill",
        (wall_x + 0.07, WINDOW_CENTER_Y, WINDOW_SILL_HEIGHT - 0.035),
        (0.34, clear_width + 0.16, 0.07),
        trim,
        bevel=0.012,
    )
    track = add_curve_profile(
        ctx,
        "Curtain_Rail",
        [
            (wall_x + 0.27, WINDOW_CENTER_Y - clear_width / 2.0 - 0.36, 3.20),
            (wall_x + 0.27, WINDOW_CENTER_Y + clear_width / 2.0 + 0.36, 3.20),
        ],
        ctx.materials["Metal_DarkBrown"],
        bevel_depth=0.018,
    )
    sheer = _add_curtain_mesh(
        ctx,
        "Curtain_Sheer",
        WINDOW_CENTER_Y,
        clear_width + 0.18,
        WINDOW_SILL_HEIGHT + 0.02,
        3.16,
        wall_x + 0.20,
        0.035,
        12,
        ctx.materials["Curtain_Sheer"],
    )
    outer_front = _add_curtain_mesh(
        ctx,
        "Curtain_Outer_Front",
        WINDOW_CENTER_Y - clear_width / 2.0 - 0.10,
        0.64,
        0.12,
        3.16,
        wall_x + 0.28,
        0.065,
        7,
        ctx.materials["Curtain_Outer"],
    )
    outer_rear = _add_curtain_mesh(
        ctx,
        "Curtain_Outer_Rear",
        WINDOW_CENTER_Y + clear_width / 2.0 + 0.10,
        0.64,
        0.12,
        3.16,
        wall_x + 0.28,
        0.065,
        7,
        ctx.materials["Curtain_Outer"],
    )
    blocker = add_box(
        ctx,
        "Window_ExteriorLightBlocker",
        (wall_x - 0.48, WINDOW_CENTER_Y, opening_mid_z),
        (0.05, clear_width + 0.70, clear_height + 0.35),
        ctx.materials["Metal_DarkBrown"],
        bevel=0.01,
    )
    blocker["purpose"] = "reverse_angle_light_blocker"
    return [frame, *pieces, glass, sill, track, sheer, outer_front, outer_rear, blocker]


def build_door_leaf_and_hardware(
    ctx: StudioContext,
    clear_width: float = 1.0,
    clear_height: float = 2.4,
) -> list[bpy.types.Object]:
    """Build an inward-swinging door, frame, hardware, and QA clearance arc."""

    room_width, _depth, _height = contract.ROOM_SIZE
    wall_x = room_width / 2.0
    trim = ctx.materials["Trim_WarmWhite"]
    frame_width = 0.07
    frame_parts = [
        add_box(
            ctx,
            "Door_Frame_FrontJamb",
            (wall_x, DOOR_CENTER_Y - clear_width / 2.0 - frame_width / 2.0, clear_height / 2.0),
            (0.20, frame_width, clear_height),
            trim,
            bevel=0.008,
        ),
        add_box(
            ctx,
            "Door_Frame_RearJamb",
            (wall_x, DOOR_CENTER_Y + clear_width / 2.0 + frame_width / 2.0, clear_height / 2.0),
            (0.20, frame_width, clear_height),
            trim,
            bevel=0.008,
        ),
        add_box(
            ctx,
            "Door_Frame_Header",
            (wall_x, DOOR_CENTER_Y, clear_height + frame_width / 2.0),
            (0.20, clear_width + frame_width * 2.0, frame_width),
            trim,
            bevel=0.008,
        ),
    ]

    hinge = bpy.data.objects.new("Door_Hinge", None)
    ctx.collections["STUDIO_ARCHITECTURE"].objects.link(hinge)
    hinge.location = (wall_x - 0.02, DOOR_CENTER_Y + clear_width / 2.0 - 0.03, 0.0)
    hinge.empty_display_type = "PLAIN_AXES"
    hinge.empty_display_size = 0.16
    parent_to_master(hinge, ctx.master)

    leaf = add_box(
        ctx,
        "Door_Leaf",
        (wall_x - 0.02, DOOR_CENTER_Y, (clear_height - 0.05) / 2.0),
        (0.05, clear_width - 0.06, clear_height - 0.05),
        ctx.materials["Floor_LightOak"],
        bevel=0.012,
    )
    leaf["clear_width"] = clear_width
    leaf["clear_height"] = clear_height
    leaf["hinge_side"] = "rear"
    leaf["swing_angle_degrees"] = 18.0
    tag_wood_grain(leaf, "Z")
    parent_to_master(leaf, hinge)

    handles = []
    for side, handle_x in (("Inner", wall_x - 0.08), ("Outer", wall_x + 0.04)):
        handle = add_cylinder(
            ctx,
            f"Door_Handle_{side}",
            (handle_x, DOOR_CENTER_Y - clear_width * 0.34, 1.04),
            0.025,
            0.12,
            ctx.materials["Metal_DarkBrown"],
            rotation=(0.0, math.pi / 2.0, 0.0),
            bevel=0.004,
        )
        parent_to_master(handle, hinge)
        handles.append(handle)

    for index, z in enumerate((0.38, 1.18, 1.98), start=1):
        plate = add_box(
            ctx,
            f"Door_HingePlate_{index:02d}",
            (wall_x - 0.055, DOOR_CENTER_Y + clear_width / 2.0 - 0.04, z),
            (0.012, 0.065, 0.15),
            ctx.materials["Metal_DarkBrown"],
            bevel=0.003,
        )
        parent_to_master(plate, hinge)
        handles.append(plate)

    swing_angle = math.radians(-18.0)
    radius = clear_width - 0.06
    arc_points = []
    for index in range(13):
        angle = swing_angle * index / 12.0
        arc_points.append(
            (
                hinge.location.x + math.sin(angle) * radius,
                hinge.location.y - math.cos(angle) * radius,
                0.018,
            )
        )
    swing_arc = add_curve_profile(
        ctx,
        "Door_SwingClearance",
        arc_points,
        ctx.materials["Metal_DarkBrown"],
        "QA_ONLY",
        bevel_depth=0.006,
    )
    swing_arc.hide_render = True
    swing_arc["clearance_radius"] = radius
    hinge.rotation_euler.z = swing_angle
    return [*frame_parts, hinge, leaf, *handles, swing_arc]


def build_architecture(ctx: StudioContext) -> dict[str, bpy.types.Object]:
    """Build the complete metric room shell, openings, trim, and dressings."""

    width, depth, height = contract.ROOM_SIZE
    volume = add_box(
        ctx,
        "Studio_InteriorVolume",
        (0.0, 0.0, height / 2.0),
        contract.ROOM_SIZE,
        None,
        "QA_ONLY",
    )
    volume.display_type = "WIRE"
    volume.display.show_shadows = False
    volume.hide_render = True
    volume["purpose"] = "room_dimension_contract"

    tag_wood_grain(
        add_box(
            ctx,
            "Studio_Floor",
            (0.0, 0.0, -0.06),
            (width + 0.36, depth + 0.36, 0.12),
            ctx.materials["Floor_LightOak"],
            bevel=0.008,
        ),
        "Y",
    )
    add_box(
        ctx,
        "Studio_Ceiling",
        (0.0, 0.0, height + 0.06),
        (width + 0.36, depth + 0.36, 0.12),
        ctx.materials["Wall_WarmPlaster"],
        bevel=0.008,
    )
    build_back_wall(ctx)
    build_front_wall(ctx)
    build_left_window_wall(ctx)
    build_right_door_wall(ctx)
    build_baseboards_and_crown(ctx)
    build_floorboards(ctx)
    build_window_frames_and_curtains(ctx)
    build_door_leaf_and_hardware(ctx)
    return {obj.name: obj for obj in bpy.data.objects}


def build_main_desk(
    ctx: StudioContext,
    size: tuple[float, float, float] = contract.DESK_SIZE,
    location: tuple[float, float, float] = (0.0, -0.49, 0.0),
) -> bpy.types.Object:
    """Build the near-camera solid-oak hero desk at its approved dimensions."""

    x, y, floor_z = location
    width, depth, height = size
    top_thickness = 0.10
    end_cap_width = 0.025
    oak = ctx.materials["Desk_WarmOak"]
    end_oak = ctx.materials["Desk_WarmOak_EndGrain"]
    metal = ctx.materials["Metal_DarkBrown"]
    top = tag_wood_grain(
        add_box(
            ctx,
            "Desk_Top",
            (x, y, floor_z + height - top_thickness / 2.0),
            (width - end_cap_width * 2.0, depth, top_thickness),
            oak,
            "STUDIO_FURNITURE",
            bevel=0.025,
        ),
        "X",
    )
    top["assembly_role"] = "main_desk"
    top["authored_size"] = json.dumps(list(size))

    parts: list[bpy.types.Object] = []
    for side, end_x in (
        ("Left", x - width / 2.0 + end_cap_width / 2.0),
        ("Right", x + width / 2.0 - end_cap_width / 2.0),
    ):
        cap = tag_wood_grain(
            add_box(
                ctx,
                f"Desk_Top_Endgrain_{side}",
                (end_x, y, floor_z + height - top_thickness / 2.0),
                (end_cap_width, depth, top_thickness),
                end_oak,
                "STUDIO_FURNITURE",
                bevel=0.006,
            ),
            "END",
        )
        parts.append(cap)

    drawer_case = tag_wood_grain(
        add_box(
            ctx,
            "Desk_Drawer_Case",
            (x, y - 0.20, floor_z + height - top_thickness - 0.12),
            (1.34, 0.39, 0.24),
            oak,
            "STUDIO_FURNITURE",
            bevel=0.014,
        ),
        "X",
    )
    parts.append(drawer_case)
    for index, drawer_x in enumerate((x - 0.325, x + 0.325), start=1):
        front = tag_wood_grain(
            add_box(
                ctx,
                f"Desk_Drawer_Front_{index:02d}",
                (drawer_x, y - 0.405, floor_z + height - top_thickness - 0.12),
                (0.61, 0.035, 0.19),
                oak,
                "STUDIO_FURNITURE",
                bevel=0.012,
            ),
            "X",
        )
        handle = add_box(
            ctx,
            f"Desk_Drawer_Handle_{index:02d}",
            (drawer_x, y - 0.430, floor_z + height - top_thickness - 0.10),
            (0.24, 0.025, 0.025),
            metal,
            "STUDIO_FURNITURE",
            bevel=0.009,
        )
        parts.extend((front, handle))

    for side, leg_x in (("Left", x - width / 2.0 + 0.18), ("Right", x + width / 2.0 - 0.18)):
        foot = add_box(
            ctx,
            f"Desk_Foot_{side}",
            (leg_x, y, floor_z + (height - top_thickness) / 2.0),
            (0.075, depth - 0.23, height - top_thickness),
            metal,
            "STUDIO_FURNITURE",
            bevel=0.012,
        )
        parts.append(foot)
    parts.extend(
        (
            add_box(
                ctx,
                "Desk_Frame_BackRail",
                (x, y + depth / 2.0 - 0.12, floor_z + height - top_thickness - 0.045),
                (width - 0.34, 0.065, 0.09),
                metal,
                "STUDIO_FURNITURE",
                bevel=0.012,
            ),
            add_box(
                ctx,
                "Desk_Frame_FrontRail",
                (x, y - depth / 2.0 + 0.12, floor_z + height - top_thickness - 0.045),
                (width - 0.34, 0.065, 0.09),
                metal,
                "STUDIO_FURNITURE",
                bevel=0.012,
            ),
        )
    )
    parent_assembly(top, parts)
    return top


def build_hero_stool_chair(
    ctx: StudioContext,
    location: tuple[float, float, float] = contract.HERO_CHAIR_LOCATION,
) -> bpy.types.Object:
    """Build a visible seated-mode chair with a restrained hero profile."""

    x, y, floor_z = location
    root = add_empty(ctx, "Chair_Main", location, "STUDIO_FURNITURE")
    root["hero_visibility_strategy"] = contract.SEAT_VISIBILITY_PROFILE["strategy"]
    root["minimum_visible_fraction"] = contract.SEAT_VISIBILITY_PROFILE[
        "minimumVisibleFraction"
    ]
    root["maximum_visible_fraction"] = contract.SEAT_VISIBILITY_PROFILE[
        "maximumVisibleFraction"
    ]
    root.hide_render = False
    fabric = ctx.materials["Chair_Fabric"]
    oak = ctx.materials["Desk_WarmOak"]
    parts = [
        add_box(
            ctx,
            "Chair_Seat",
            (x, y, floor_z + 0.50),
            contract.HERO_CHAIR_SEAT_SIZE,
            fabric,
            "STUDIO_FURNITURE",
            bevel=0.045,
        ),
        add_box(
            ctx,
            "Chair_Back",
            (x, y + 0.265, floor_z + 0.99),
            contract.HERO_CHAIR_BACK_SIZE,
            fabric,
            "STUDIO_FURNITURE",
            bevel=0.035,
        ),
    ]
    for side, arm_x in (
        ("Left", x - contract.HERO_CHAIR_ARM_X),
        ("Right", x + contract.HERO_CHAIR_ARM_X),
    ):
        parts.append(
            tag_wood_grain(
                add_box(
                    ctx,
                    f"Chair_Arm_{side}",
                    (arm_x, y - 0.01, floor_z + 0.615),
                    (0.1125, 0.52, 0.065),
                    oak,
                    "STUDIO_FURNITURE",
                    bevel=0.022,
                ),
                "Y",
            )
        )
        parts.append(
            tag_wood_grain(
                add_box(
                    ctx,
                    f"Chair_ArmSupport_{side}",
                    (arm_x, y - 0.18, floor_z + 0.55),
                    (0.090, 0.060, 0.17),
                    oak,
                    "STUDIO_FURNITURE",
                    bevel=0.014,
                ),
                "Z",
            )
        )
    for index, (foot_x, foot_y) in enumerate(
        (
            (x - contract.HERO_CHAIR_FOOT_X, y - 0.20),
            (x + contract.HERO_CHAIR_FOOT_X, y - 0.20),
            (x - contract.HERO_CHAIR_FOOT_X, y + 0.20),
            (x + contract.HERO_CHAIR_FOOT_X, y + 0.20),
        ),
        start=1,
    ):
        parts.append(
            tag_wood_grain(
                add_box(
                    ctx,
                    f"Chair_Foot_{index:02d}",
                    (foot_x, foot_y, floor_z + 0.22),
                    (0.0975, 0.065, 0.44),
                    oak,
                    "STUDIO_FURNITURE",
                    bevel=0.014,
                ),
                "Z",
            )
        )
    parent_assembly(root, parts)
    return root


def build_left_six_drawer_cabinet(
    ctx: StudioContext,
    location: tuple[float, float, float] = (-1.72, 2.50, 0.0),
) -> bpy.types.Object:
    """Build the grounded three-by-two drawer cabinet in the left back zone."""

    x, y, floor_z = location
    oak = ctx.materials["Desk_WarmOak"]
    metal = ctx.materials["Metal_DarkBrown"]
    body = tag_wood_grain(
        add_box(
            ctx,
            "Cabinet_Left",
            (x, y, floor_z + 0.55),
            (1.34, 0.40, 0.90),
            oak,
            "STUDIO_FURNITURE",
            bevel=0.020,
        ),
        "X",
    )
    body["drawer_layout"] = "3x2"
    parts: list[bpy.types.Object] = []
    top = tag_wood_grain(
        add_box(
            ctx,
            "Cabinet_Left_Top",
            (x, y, floor_z + 1.025),
            (1.40, 0.44, 0.05),
            oak,
            "STUDIO_FURNITURE",
            bevel=0.014,
        ),
        "X",
    )
    parts.append(top)
    drawer_index = 0
    for row_z in (floor_z + 0.34, floor_z + 0.73):
        for column_x in (x - 0.43, x, x + 0.43):
            drawer_index += 1
            front = tag_wood_grain(
                add_box(
                    ctx,
                    f"Cabinet_Drawer_{drawer_index:02d}",
                    (column_x, y - 0.215, row_z),
                    (0.38, 0.040, 0.32),
                    oak,
                    "STUDIO_FURNITURE",
                    bevel=0.013,
                ),
                "X",
            )
            handle = add_box(
                ctx,
                f"Cabinet_Handle_{drawer_index:02d}",
                (column_x, y - 0.246, row_z + 0.065),
                (0.16, 0.025, 0.026),
                metal,
                "STUDIO_FURNITURE",
                bevel=0.008,
            )
            parts.extend((front, handle))
    for index, (foot_x, foot_y) in enumerate(
        (
            (x - 0.54, y - 0.14),
            (x + 0.54, y - 0.14),
            (x - 0.54, y + 0.14),
            (x + 0.54, y + 0.14),
        ),
        start=1,
    ):
        parts.append(
            add_box(
                ctx,
                f"Cabinet_Foot_{index:02d}",
                (foot_x, foot_y, floor_z + 0.05),
                (0.07, 0.07, 0.10),
                metal,
                "STUDIO_FURNITURE",
                bevel=0.012,
            )
        )
    parent_assembly(body, parts)
    return body


def build_right_slat_wall(
    ctx: StudioContext,
    x_range: tuple[float, float] = (0.35, 2.88),
    back_y: float = 2.82,
    slat_pitch: float = 0.095,
) -> bpy.types.Object:
    """Build a backed, real-depth walnut slat feature on the right rear wall."""

    start_x, end_x = x_range
    width = end_x - start_x
    feature_bottom = 0.14
    feature_top = 3.22
    feature_height = feature_top - feature_bottom
    feature_center_z = (feature_bottom + feature_top) / 2.0
    backing = add_box(
        ctx,
        "SlatWall_Back_Right",
        ((start_x + end_x) / 2.0, back_y, feature_center_z),
        (width + 0.08, 0.040, feature_height),
        ctx.materials["Metal_DarkBrown"],
        "STUDIO_FURNITURE",
        bevel=0.008,
    )
    backing["x_range"] = json.dumps(list(x_range))
    backing["slat_pitch"] = slat_pitch
    slat_count = math.floor(width / slat_pitch) + 1
    first = tag_wood_grain(
        add_box(
            ctx,
            "Slat_Back_001",
            (start_x, back_y - 0.052, feature_center_z),
            (0.048, 0.065, feature_height),
            ctx.materials["Slat_Walnut"],
            "STUDIO_FURNITURE",
            bevel=0.010,
        ),
        "Z",
    )
    slats = [first]
    for index in range(1, slat_count):
        slat = linked_mesh_copy(
            ctx,
            first,
            f"Slat_Back_{index + 1:03d}",
            (start_x + index * slat_pitch, back_y - 0.052, feature_center_z),
            "STUDIO_FURNITURE",
        )
        slat["grain_axis"] = "Z"
        slats.append(slat)
    parent_assembly(backing, slats)
    return backing


def build_three_floating_shelves(
    ctx: StudioContext,
    center: tuple[float, float, float] = (1.66, 2.62, 1.75),
) -> list[bpy.types.Object]:
    """Build exactly three deep, attached slabs against the slatted feature."""

    x, y, z = center
    shelves = []
    for index, shelf_z in enumerate((z - 0.52, z, z + 0.52), start=1):
        shelf = tag_wood_grain(
            add_box(
                ctx,
                f"Shelf_Right_{index:02d}",
                (x, y, shelf_z),
                (1.86, 0.42, 0.060),
                ctx.materials["Slat_Walnut"],
                "STUDIO_FURNITURE",
                bevel=0.015,
            ),
            "X",
        )
        shelf["attachment_depth"] = 0.20
        shelves.append(shelf)
    return shelves


def build_oval_rug(
    ctx: StudioContext,
    center: tuple[float, float, float] = (0.0, -0.10, 0.012),
    radii: tuple[float, float] = (1.78, 1.20),
) -> bpy.types.Object:
    """Build a solid oval jute rug with sparse silhouette-only curve fibers."""

    rug = add_cylinder(
        ctx,
        "Rug_Main",
        center,
        1.0,
        0.024,
        ctx.materials["Rug_Jute"],
        "STUDIO_FURNITURE",
        vertices=96,
        bevel=0.010,
    )
    rug.dimensions = (radii[0] * 2.0, radii[1] * 2.0, 0.024)
    bpy.context.view_layer.objects.active = rug
    rug.select_set(True)
    bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)
    rug.select_set(False)
    rug["shape"] = "oval"
    rug["radii"] = json.dumps(list(radii))
    rug["fiber_strategy"] = "sparse_silhouette_curves"

    fiber_parts = []
    fiber_count = 24
    for index in range(fiber_count):
        angle = math.tau * index / fiber_count
        tangent_angle = angle + math.pi / 2.0
        edge_x = center[0] + radii[0] * math.cos(angle)
        edge_y = center[1] + radii[1] * math.sin(angle)
        half_length = 0.030 + 0.008 * (index % 3)
        points = [
            (
                edge_x - math.cos(tangent_angle) * half_length,
                edge_y - math.sin(tangent_angle) * half_length,
                center[2] + 0.016,
            ),
            (
                edge_x + math.cos(tangent_angle) * half_length,
                edge_y + math.sin(tangent_angle) * half_length,
                center[2] + 0.016,
            ),
        ]
        fiber_parts.append(
            add_curve_profile(
                ctx,
                f"Rug_EdgeFiber_{index + 1:02d}",
                points,
                ctx.materials["Rug_Jute"],
                "STUDIO_FURNITURE",
                bevel_depth=0.0022,
            )
        )
    parent_assembly(rug, fiber_parts)
    return rug


def build_furniture(ctx: StudioContext) -> dict[str, object]:
    """Build every Task 3 furniture assembly without cameras, lights, or branding."""

    desk = build_main_desk(ctx)
    chair = build_hero_stool_chair(ctx)
    cabinet = build_left_six_drawer_cabinet(ctx)
    slat_wall = build_right_slat_wall(ctx)
    shelves = build_three_floating_shelves(ctx)
    rug = build_oval_rug(ctx)
    return {
        "desk_dimensions": contract.DESK_SIZE,
        "desk": desk,
        "chair": chair,
        "cabinet": cabinet,
        "slat_wall": slat_wall,
        "shelves": shelves,
        "rug": rug,
    }


def _build_small_decorative_plant(
    ctx: StudioContext,
    name: str,
    center: tuple[float, float],
    surface_z: float,
    *,
    size: float = 1.0,
) -> bpy.types.Object:
    """Build a small prop plant; floor-plant vegetation remains a later stage."""

    x, y = center
    pot_height = 0.13 * size
    pot = add_cone(
        ctx,
        name,
        (x, y, surface_z + pot_height / 2.0),
        0.080 * size,
        0.065 * size,
        pot_height,
        ctx.materials["Ceramic_Sand"],
        "STUDIO_PROPS",
        bevel=0.008 * size,
    )
    pot["plant_scope"] = "small_decorative_prop"
    parts: list[bpy.types.Object] = []
    leaf_materials = ("Leaf_MutedGreen", "Leaf_Olive")
    for index, offset in enumerate((-0.035, 0.0, 0.035), start=1):
        stem_top = surface_z + pot_height + (0.13 + 0.025 * index) * size
        stem = add_curve_profile(
            ctx,
            f"{name}_Stem_{index:02d}",
            [
                (x, y, surface_z + pot_height * 0.80),
                (x + offset, y + offset * 0.35, stem_top),
            ],
            ctx.materials["Stem_DarkGreen"],
            "STUDIO_PROPS",
            bevel_depth=0.006 * size,
        )
        leaf = add_uv_sphere(
            ctx,
            f"{name}_Leaf_{index:02d}",
            (x + offset * 1.2, y + offset * 0.5, stem_top),
            (0.075 * size, 0.035 * size, 0.13 * size),
            ctx.materials[leaf_materials[(index - 1) % len(leaf_materials)]],
            "STUDIO_PROPS",
            rotation=(0.0, (-0.28 + index * 0.14), offset * 3.0),
            segments=20,
            ring_count=10,
        )
        parts.extend((stem, leaf))
    parent_assembly(pot, parts)
    return pot


def _build_decorative_globe(
    ctx: StudioContext,
    name: str,
    center: tuple[float, float],
    surface_z: float,
    *,
    radius: float = 0.16,
) -> bpy.types.Object:
    """Build a small globe with visible stand, meridian, and tilted axis."""

    x, y = center
    globe_z = surface_z + radius + 0.075
    globe = add_uv_sphere(
        ctx,
        name,
        (x, y, globe_z),
        (radius * 2.0, radius * 2.0, radius * 2.0),
        ctx.materials["Portrait_Background"],
        "STUDIO_PROPS",
        rotation=(0.0, math.radians(17.0), 0.0),
        segments=48,
        ring_count=24,
    )
    globe["axis_tilt_degrees"] = 17.0
    parts = [
        add_cylinder(
            ctx,
            f"{name}_Base",
            (x, y, surface_z + 0.018),
            radius * 0.62,
            0.036,
            ctx.materials["Metal_DarkBrown"],
            "STUDIO_PROPS",
            vertices=32,
            bevel=0.006,
        ),
        add_curve_profile(
            ctx,
            f"{name}_Axis",
            [
                (x - radius * 0.16, y, surface_z + 0.035),
                (x - radius * 0.12, y, globe_z - radius * 0.84),
                (x + radius * 0.16, y, globe_z + radius * 0.84),
            ],
            ctx.materials["Metal_DarkBrown"],
            "STUDIO_PROPS",
            bevel_depth=0.010,
        ),
        add_torus(
            ctx,
            f"{name}_Meridian",
            (x, y, globe_z),
            radius + 0.012,
            0.008,
            ctx.materials["Metal_DarkBrown"],
            "STUDIO_PROPS",
            rotation=(math.pi / 2.0, 0.0, math.radians(17.0)),
        ),
    ]
    parent_assembly(globe, parts)
    return globe


def _build_table_lamp(
    ctx: StudioContext,
    name: str,
    center: tuple[float, float],
    surface_z: float,
    *,
    height: float,
    task_style: bool = False,
) -> bpy.types.Object:
    """Build a compact lamp with base, shaped stem, diffuser, and warm shade."""

    x, y = center
    base = add_cylinder(
        ctx,
        name,
        (x, y, surface_z + 0.025),
        0.105 if task_style else 0.125,
        0.050,
        ctx.materials["Metal_DarkBrown"],
        "STUDIO_PROPS",
        vertices=32,
        bevel=0.008,
    )
    stem_top = surface_z + height * 0.68
    shade_height = height * 0.32
    shade_center_z = surface_z + height * 0.80 + 0.015
    shade_bottom_z = shade_center_z - shade_height / 2.0
    diffuser_z = shade_bottom_z + 0.012
    lamp_x = x - (0.07 if task_style else 0.0)
    stem_points = [(x, y, surface_z + 0.05)]
    if task_style:
        stem_points.extend(((x, y, surface_z + height * 0.48), (x - 0.07, y, stem_top)))
    else:
        stem_points.append((x, y, stem_top))
    parts: list[bpy.types.Object] = [
        add_curve_profile(
            ctx,
            f"{name}_Stem",
            stem_points,
            ctx.materials["Metal_DarkBrown"],
            "STUDIO_PROPS",
            bevel_depth=0.012,
        ),
        add_uv_sphere(
            ctx,
            f"{name}_Diffuser",
            (lamp_x, y, diffuser_z),
            (0.11, 0.11, 0.11),
            ctx.materials["Lamp_Emissive_Warm"],
            "STUDIO_PROPS",
            segments=48,
            ring_count=24,
        ),
        add_open_frustum(
            ctx,
            f"{name}_Shade",
            (lamp_x, y, shade_center_z),
            0.155 if task_style else 0.185,
            0.095 if task_style else 0.115,
            shade_height,
            ctx.materials["Lamp_Shade_Warm"],
            "STUDIO_PROPS",
            vertices=40,
            thickness=0.006,
        ),
    ]
    parent_assembly(base, parts)
    return base


def build_tabletop_props(ctx: StudioContext) -> list[bpy.types.Object]:
    """Dress the hero desktop with separate readable silhouettes."""

    surface_z = contract.DESK_SIZE[2]
    props: list[bpy.types.Object] = []
    props.append(_build_small_decorative_plant(ctx, "Desk_Plant", (-1.02, -0.40), surface_z, size=0.88))

    mug = add_hollow_vessel(
        ctx,
        "Brand_Mug",
        (0.80, -0.50, surface_z + 0.085),
        0.075,
        0.17,
        0.007,
        0.012,
        ctx.materials["Ceramic_Sand"],
        "STUDIO_PROPS",
        segments=40,
    )
    mug["artwork_status"] = "simple_placeholder_emblem"
    mug_parts = [
        add_curve_profile(
            ctx,
            "Brand_Mug_Handle",
            [
                (0.868, -0.50, surface_z + 0.055),
                (0.925, -0.50, surface_z + 0.075),
                (0.925, -0.50, surface_z + 0.135),
                (0.868, -0.50, surface_z + 0.155),
            ],
            ctx.materials["Ceramic_Sand"],
            "STUDIO_PROPS",
            bevel_depth=0.012,
        ),
        add_uv_sphere(
            ctx,
            "Brand_Mug_Emblem_Simple",
            (0.80, -0.577, surface_z + 0.095),
            (0.038, 0.010, 0.050),
            ctx.materials["Portrait_Dark"],
            "STUDIO_PROPS",
            segments=20,
            ring_count=10,
        ),
    ]
    parent_assembly(mug, mug_parts)
    props.append(mug)

    book_specs = (
        ("Desk_Book_01", (-0.62, -0.42, surface_z + 0.020), (0.42, 0.28, 0.040), "Book_Olive", -0.03),
        ("Desk_Book_02", (-0.60, -0.42, surface_z + 0.059), (0.38, 0.25, 0.038), "Book_Brown", 0.02),
        ("Desk_Book_03", (-0.59, -0.42, surface_z + 0.096), (0.34, 0.23, 0.036), "Book_Mustard", -0.015),
    )
    for name, position, dimensions, material_name, rotation_z in book_specs:
        book = add_box(
            ctx,
            name,
            position,
            dimensions,
            ctx.materials[material_name],
            "STUDIO_PROPS",
            bevel=0.008,
        )
        book.rotation_euler.z = rotation_z
        book["book_orientation"] = "horizontal"
        props.append(book)
    notebook = add_box(
        ctx,
        "Desk_Notebook",
        (0.18, -0.47, surface_z + 0.014),
        (0.34, 0.25, 0.028),
        ctx.materials["Book_Gray"],
        "STUDIO_PROPS",
        bevel=0.010,
    )
    pen = add_cylinder(
        ctx,
        "Desk_Pen",
        (0.21, -0.31, surface_z + 0.008),
        0.008,
        0.29,
        ctx.materials["Metal_DarkBrown"],
        "STUDIO_PROPS",
        rotation=(0.0, math.pi / 2.0, 0.0),
        vertices=16,
        bevel=0.002,
    )
    props.extend((notebook, pen))
    return props


def build_cabinet_props(ctx: StudioContext) -> list[bpy.types.Object]:
    """Dress the left cabinet and place its open woven floor basket beside it."""

    surface_z = 1.05
    props: list[bpy.types.Object] = [
        _build_decorative_globe(ctx, "Cabinet_Globe", (-2.08, 2.45), surface_z, radius=0.17),
        _build_small_decorative_plant(ctx, "Cabinet_Planter", (-1.35, 2.47), surface_z, size=0.72),
        _build_table_lamp(
            ctx,
            "Cabinet_TaskLamp",
            (-1.17, 2.46),
            surface_z,
            height=0.45,
            task_style=True,
        ),
    ]
    for index, (book_x, height, material_name) in enumerate(
        ((-1.77, 0.27, "Book_Brown"), (-1.69, 0.31, "Book_Olive"), (-1.60, 0.24, "Book_Gray")),
        start=1,
    ):
        book = add_box(
                ctx,
                f"Cabinet_Book_{index:02d}",
                (book_x, 2.60, surface_z + height / 2.0),
                (0.065, 0.22, height),
                ctx.materials[material_name],
                "STUDIO_PROPS",
                bevel=0.006,
        )
        book["book_orientation"] = "vertical"
        props.append(book)

    stack_surface = surface_z
    for index, (width, depth, thickness, material_name, rotation_z) in enumerate(
        (
            (0.36, 0.18, 0.038, "Book_Gray", -0.035),
            (0.33, 0.17, 0.034, "Book_Mustard", 0.025),
            (0.30, 0.16, 0.036, "Book_Brown", -0.020),
        ),
        start=1,
    ):
        book = add_box(
            ctx,
            f"Cabinet_StackBook_{index:02d}",
            (-1.73, 2.38, stack_surface + thickness / 2.0),
            (width, depth, thickness),
            ctx.materials[material_name],
            "STUDIO_PROPS",
            bevel=0.006,
        )
        book.rotation_euler.z = rotation_z
        book["book_orientation"] = "horizontal"
        props.append(book)
        stack_surface += thickness

    basket_center = (-2.70, 2.30)
    basket = add_empty(
        ctx,
        "Cabinet_Basket",
        (basket_center[0], basket_center[1], 0.0),
        "STUDIO_PROPS",
    )
    basket["construction"] = "open_woven_reeds_and_hoops"
    basket["placement"] = "floor_left_of_cabinet"
    basket_parts: list[bpy.types.Object] = []
    reed_count = 12
    for index in range(reed_count):
        angle = math.tau * index / reed_count
        bottom_radius = 0.145
        top_radius = 0.190
        basket_parts.append(
            add_curve_profile(
                ctx,
                f"Cabinet_Basket_Reed_{index + 1:02d}",
                [
                    (
                        basket_center[0] + bottom_radius * math.cos(angle),
                        basket_center[1] + bottom_radius * math.sin(angle),
                        0.006,
                    ),
                    (
                        basket_center[0] + top_radius * math.cos(angle),
                        basket_center[1] + top_radius * math.sin(angle),
                        0.30,
                    ),
                ],
                ctx.materials["Basket_Wicker"],
                "STUDIO_PROPS",
                bevel_depth=0.006,
            )
        )
    for index, hoop_z in enumerate((0.007, 0.075, 0.145, 0.215, 0.295), start=1):
        radius = 0.145 + (0.045 * hoop_z / 0.30)
        basket_parts.append(
            add_torus(
                ctx,
                f"Cabinet_Basket_Weave_Hoop_{index:02d}",
                (basket_center[0], basket_center[1], hoop_z),
                radius,
                0.007,
                ctx.materials["Basket_Wicker"],
                "STUDIO_PROPS",
            )
        )
    parent_assembly(basket, basket_parts)
    props.append(basket)
    return props


def _add_shelf_frame(
    ctx: StudioContext,
    name: str,
    center: tuple[float, float, float],
    size: tuple[float, float],
) -> bpy.types.Object:
    """Build a small freestanding frame with a neutral inset."""

    width, height = size
    x, y, bottom_z = center
    frame = add_box(
        ctx,
        name,
        (x, y, bottom_z + height / 2.0),
        (width, 0.035, height),
        ctx.materials["Metal_DarkBrown"],
        "STUDIO_PROPS",
        bevel=0.010,
    )
    inset = add_box(
        ctx,
        f"{name}_Inset",
        (x, y - 0.021, bottom_z + height / 2.0),
        (width - 0.045, 0.012, height - 0.045),
        ctx.materials["Paper_Warm"],
        "STUDIO_PROPS",
        bevel=0.004,
    )
    parent_assembly(frame, [inset])
    return frame


def build_shelf_prop_clusters(ctx: StudioContext) -> list[bpy.types.Object]:
    """Build balanced shelf clusters with linked book families and hero props."""

    shelf_tops = (1.26, 1.78, 2.30)
    props: list[bpy.types.Object] = []
    book_materials = ("Book_Mustard", "Book_Brown", "Book_Gray", "Book_Olive", "Book_Brown")
    book_heights = (0.28, 0.34, 0.25, 0.31, 0.37)
    source_books: list[bpy.types.Object] = []
    cursor_x = 0.84
    for index, (height, material_name) in enumerate(zip(book_heights, book_materials), start=1):
        thickness = 0.052 + index * 0.006
        source = add_box(
            ctx,
            f"Book_Shelf_Source_{index:02d}",
            (cursor_x, 2.48, shelf_tops[0] + height / 2.0),
            (thickness, 0.23, height),
            ctx.materials[material_name],
            "STUDIO_PROPS",
            bevel=0.006,
        )
        source["linked_book_family"] = index
        source["book_orientation"] = "vertical"
        source_books.append(source)
        props.append(source)
        cursor_x += thickness + 0.018

    duplicate_positions = (
        (1.72, 2.48, shelf_tops[1]),
        (1.80, 2.48, shelf_tops[1]),
        (2.16, 2.48, shelf_tops[2]),
        (2.24, 2.48, shelf_tops[2]),
        (2.33, 2.48, shelf_tops[2]),
    )
    for index, (source, (book_x, book_y, base_z)) in enumerate(
        zip(source_books, duplicate_positions), start=1
    ):
        duplicate = linked_mesh_copy(
            ctx,
            source,
            f"Book_Shelf_Linked_{index:02d}",
            (book_x, book_y, base_z + book_heights[index - 1] / 2.0),
            "STUDIO_PROPS",
            rotation=(0.0, 0.0, (-0.035 + index * 0.012)),
        )
        duplicate["linked_book_family"] = index
        props.append(duplicate)

    props.extend(
        (
            _add_shelf_frame(ctx, "Shelf_Frame_01", (1.48, 2.49, shelf_tops[0]), (0.27, 0.32)),
            _add_shelf_frame(ctx, "Shelf_Frame_02", (0.98, 2.49, shelf_tops[2]), (0.24, 0.29)),
            add_cone(
                ctx,
                "Shelf_Pottery_01",
                (2.29, 2.49, shelf_tops[0] + 0.105),
                0.115,
                0.080,
                0.21,
                ctx.materials["Ceramic_Sand"],
                "STUDIO_PROPS",
                bevel=0.010,
            ),
            add_uv_sphere(
                ctx,
                "Shelf_Pottery_02",
                (1.34, 2.49, shelf_tops[1] + 0.10),
                (0.18, 0.16, 0.20),
                ctx.materials["Ceramic_Sand"],
                "STUDIO_PROPS",
                segments=28,
                ring_count=14,
            ),
            _build_small_decorative_plant(
                ctx, "Shelf_Plant_01", (2.42, 2.48), shelf_tops[1], size=0.76
            ),
            _build_small_decorative_plant(
                ctx, "Shelf_Plant_02", (1.46, 2.48), shelf_tops[2], size=0.68
            ),
            _build_decorative_globe(
                ctx, "Shelf_Globe", (2.15, 2.48), shelf_tops[1], radius=0.14
            ),
            _build_table_lamp(
                ctx,
                "Shelf_TableLamp",
                (1.86, 2.48),
                shelf_tops[0],
                height=0.43,
            ),
        )
    )
    return props


def build_right_wall_portrait(ctx: StudioContext) -> bpy.types.Object:
    """Build a neutral geometry-only sloth/play portrait on the right wall."""

    frame = add_box(
        ctx,
        "RightWall_Portrait",
        (3.005, 0.78, 1.78),
        (0.060, 0.78, 0.92),
        ctx.materials["Metal_DarkBrown"],
        "STUDIO_PROPS",
        bevel=0.016,
    )
    frame["artwork_status"] = "neutral_procedural_placeholder"
    panel = add_box(
        ctx,
        "RightWall_Portrait_Panel",
        (2.968, 0.78, 1.78),
        (0.020, 0.68, 0.82),
        ctx.materials["Paper_Warm"],
        "STUDIO_PROPS",
        bevel=0.008,
    )
    face = add_uv_sphere(
        ctx,
        "Portrait_Sloth_Face",
        (2.950, 0.78, 1.86),
        (0.025, 0.39, 0.44),
        ctx.materials["Portrait_Background"],
        "STUDIO_PROPS",
        segments=28,
        ring_count=14,
    )
    parts: list[bpy.types.Object] = [panel, face]
    for side, eye_y in (("Left", 0.68), ("Right", 0.88)):
        patch = add_uv_sphere(
            ctx,
            f"Portrait_Sloth_EyePatch_{side}",
            (2.934, eye_y, 1.92),
            (0.018, 0.13, 0.18),
            ctx.materials["Portrait_Dark"],
            "STUDIO_PROPS",
            rotation=(math.radians(12.0), 0.0, 0.0),
            segments=20,
            ring_count=10,
        )
        eye = add_uv_sphere(
            ctx,
            f"Portrait_Sloth_Eye_{side}",
            (2.923, eye_y, 1.925),
            (0.014, 0.045, 0.055),
            ctx.materials["Metal_DarkBrown"],
            "STUDIO_PROPS",
            segments=16,
            ring_count=8,
        )
        parts.extend((patch, eye))
    nose = add_uv_sphere(
        ctx,
        "Portrait_Sloth_Nose",
        (2.922, 0.78, 1.83),
        (0.015, 0.085, 0.060),
        ctx.materials["Portrait_Dark"],
        "STUDIO_PROPS",
        segments=18,
        ring_count=9,
    )
    play = add_cone(
        ctx,
        "Portrait_Play_Motif",
        (2.927, 0.78, 1.55),
        0.075,
        0.075,
        0.018,
        ctx.materials["Book_Mustard"],
        "STUDIO_PROPS",
        rotation=(0.0, math.pi / 2.0, math.pi / 2.0),
        vertices=3,
        bevel=0.006,
    )
    parts.extend((nose, play))
    parent_assembly(frame, parts)
    return frame


def build_set_dressing(ctx: StudioContext) -> dict[str, object]:
    """Build the Task 3 prop layer while reserving final plants and brand art."""

    tabletop = build_tabletop_props(ctx)
    cabinet = build_cabinet_props(ctx)
    shelves = build_shelf_prop_clusters(ctx)
    portrait = build_right_wall_portrait(ctx)
    return {
        "tabletop": tabletop,
        "cabinet": cabinet,
        "shelves": shelves,
        "portrait": portrait,
    }


def _broad_leaf_mesh(ctx: StudioContext) -> bpy.types.Mesh:
    """Create one ridged broad-leaf mesh shared by every main-plant leaf."""

    existing = bpy.data.meshes.get("WarmStudio_BroadLeaf_LinkedMesh")
    if existing is not None:
        return existing

    rows = (
        (0.000, 0.010, 0.000),
        (0.095, 0.070, 0.010),
        (0.220, 0.115, 0.022),
        (0.340, 0.072, 0.035),
        (0.420, 0.008, 0.045),
    )
    vertices: list[tuple[float, float, float]] = []
    for height, half_width, ridge in rows:
        vertices.extend(
            (
                (-half_width, 0.0, height),
                (0.0, ridge, height),
                (half_width, 0.0, height),
            )
        )
    faces: list[tuple[int, int, int, int]] = []
    for row_index in range(len(rows) - 1):
        current = row_index * 3
        following = current + 3
        faces.extend(
            (
                (current, following, following + 1, current + 1),
                (current + 1, following + 1, following + 2, current + 2),
            )
        )

    mesh = bpy.data.meshes.new("WarmStudio_BroadLeaf_LinkedMesh")
    mesh.from_pydata(vertices, [], faces)
    mesh.validate()
    mesh.update()
    mesh.materials.append(ctx.materials["Leaf_MutedGreen"])
    for polygon in mesh.polygons:
        polygon.use_smooth = True
    return mesh


def _tapered_hollow_pot_mesh(ctx: StudioContext) -> bpy.types.Mesh:
    """Create a smooth, open ceramic pot mesh suitable for close-up framing."""

    existing = bpy.data.meshes.get("WarmStudio_FloorPot_LinkedMesh")
    if existing is not None:
        return existing

    segments = 48
    height = 0.42
    wall = 0.024
    outer_bottom = 0.220
    outer_top = 0.265
    inner_bottom = outer_bottom - wall
    inner_top = outer_top - wall
    vertices: list[tuple[float, float, float]] = []
    for radius, z in (
        (outer_bottom, 0.0),
        (outer_top, height),
        (inner_bottom, 0.035),
        (inner_top, height),
    ):
        for index in range(segments):
            angle = math.tau * index / segments
            vertices.append((radius * math.cos(angle), radius * math.sin(angle), z))

    outer_lower = 0
    outer_upper = segments
    inner_lower = segments * 2
    inner_upper = segments * 3
    faces: list[tuple[int, int, int, int]] = []
    for index in range(segments):
        next_index = (index + 1) % segments
        faces.extend(
            (
                (
                    outer_lower + index,
                    outer_lower + next_index,
                    outer_upper + next_index,
                    outer_upper + index,
                ),
                (
                    inner_lower + next_index,
                    inner_lower + index,
                    inner_upper + index,
                    inner_upper + next_index,
                ),
                (
                    outer_upper + index,
                    outer_upper + next_index,
                    inner_upper + next_index,
                    inner_upper + index,
                ),
                (
                    outer_lower + next_index,
                    outer_lower + index,
                    inner_lower + index,
                    inner_lower + next_index,
                ),
            )
        )

    mesh = bpy.data.meshes.new("WarmStudio_FloorPot_LinkedMesh")
    mesh.from_pydata(vertices, [], faces)
    mesh.validate()
    mesh.update()
    mesh.materials.append(ctx.materials["Ceramic_Sand"])
    for polygon in mesh.polygons:
        polygon.use_smooth = True
    return mesh


def _direct_parent_with_local_transform(
    obj: bpy.types.Object,
    root: bpy.types.Object,
    location: tuple[float, float, float],
    *,
    rotation: tuple[float, float, float] = (0.0, 0.0, 0.0),
    scale: tuple[float, float, float] = (1.0, 1.0, 1.0),
) -> bpy.types.Object:
    """Parent a new assembly part using an explicit local-space transform."""

    obj.parent = root
    obj.matrix_parent_inverse = Matrix.Identity(4)
    obj.location = location
    obj.rotation_euler = rotation
    obj.scale = scale
    return obj


def build_floor_plant_source(ctx: StudioContext) -> bpy.types.Object:
    """Build one radial 24-stem plant whose data can be mirrored by linking."""

    vegetation = ctx.collections["STUDIO_VEGETATION"]
    curve = bpy.data.curves.new("WarmStudio_FloorPlant_StemCurves", "CURVE")
    curve.dimensions = "3D"
    curve.resolution_u = 4
    curve.bevel_depth = 0.012
    curve.bevel_resolution = 3
    curve.resolution_v = 2
    curve.materials.append(ctx.materials["Stem_DarkGreen"])

    endpoints: list[tuple[Vector, Vector, float, int]] = []
    tier_specs = (
        (0.78, 0.22, 0.78, 0.0),
        (1.08, 0.30, 0.90, math.pi / 8.0),
        (1.36, 0.38, 1.00, 0.0),
    )
    for tier_index, (height, radius, leaf_scale, azimuth_offset) in enumerate(tier_specs):
        for radial_index in range(8):
            angle = azimuth_offset + math.tau * radial_index / 8.0
            radial = Vector((math.cos(angle), math.sin(angle), 0.0))
            endpoint = Vector((radial.x * radius, radial.y * radius, height))
            spline = curve.splines.new("BEZIER")
            spline.bezier_points.add(2)
            control_points = (
                Vector((radial.x * 0.030, radial.y * 0.030, 0.34)),
                Vector((endpoint.x * 0.42, endpoint.y * 0.42, height * 0.64)),
                endpoint,
            )
            for point, coordinate in zip(spline.bezier_points, control_points):
                point.co = coordinate
                point.handle_left_type = "AUTO"
                point.handle_right_type = "AUTO"
            spline.resolution_u = 4
            outward = Vector((radial.x, radial.y, 0.48 + tier_index * 0.05)).normalized()
            foliage_variant = (radial_index + tier_index) % 3
            endpoints.append((endpoint, outward, leaf_scale, foliage_variant))

    root = bpy.data.objects.new("FloorPlant_Left", curve)
    vegetation.objects.link(root)
    bpy.ops.object.select_all(action="DESELECT")
    bpy.context.view_layer.objects.active = root
    root.select_set(True)
    bpy.ops.object.convert(target="MESH")
    root.select_set(False)
    root.data.name = contract.PLANT_LINK_KEY
    for polygon in root.data.polygons:
        polygon.use_smooth = True
    root["plant_part"] = "curved_stem_canopy"
    root["stem_count"] = len(endpoints)
    root["leaf_count"] = len(endpoints)
    root["stem_construction"] = "24_bezier_splines_converted_to_linked_mesh"
    root["canopy_layout"] = "three_tier_radial_360"
    root["azimuth_span_degrees"] = 360.0
    parent_to_master(root, ctx.master)

    leaf_mesh = _broad_leaf_mesh(ctx)
    foliage_material_names = (
        "FloorLeaf_Sage_A",
        "FloorLeaf_Sage_B",
        "FloorLeaf_Sage_C",
    )
    for index, (endpoint, outward, leaf_scale, foliage_variant) in enumerate(
        endpoints, start=1
    ):
        leaf = bpy.data.objects.new(f"FloorPlant_Left_Leaf_{index:02d}", leaf_mesh)
        vegetation.objects.link(leaf)
        leaf["plant_part"] = "broad_leaf"
        leaf["leaf_index"] = index
        variant_name = foliage_material_names[foliage_variant]
        leaf["foliage_variant"] = variant_name
        leaf.material_slots[0].link = "OBJECT"
        leaf.material_slots[0].material = ctx.materials[variant_name]
        subdivision = leaf.modifiers.new("Leaf_Smooth_Form", "SUBSURF")
        subdivision.levels = 1
        subdivision.render_levels = 2
        solidify = leaf.modifiers.new("Leaf_Thickness", "SOLIDIFY")
        solidify.thickness = 0.006
        solidify.offset = 0.0
        _direct_parent_with_local_transform(
            leaf,
            root,
            tuple(endpoint),
            rotation=Vector((0.0, 0.0, 1.0)).rotation_difference(outward).to_euler(),
            scale=(leaf_scale, leaf_scale, leaf_scale),
        )

    pot = bpy.data.objects.new("FloorPlant_Left_Pot", _tapered_hollow_pot_mesh(ctx))
    vegetation.objects.link(pot)
    pot["plant_part"] = "ceramic_pot"
    pot["open_top"] = True
    add_bevel(pot, 0.008, segments=3)
    _direct_parent_with_local_transform(pot, root, (0.0, 0.0, 0.0))

    soil = add_cylinder(
        ctx,
        "FloorPlant_Left_Soil",
        (0.0, 0.0, 0.0),
        0.238,
        0.022,
        ctx.materials["Portrait_Dark"],
        "STUDIO_VEGETATION",
        vertices=48,
        bevel=0.004,
    )
    soil["plant_part"] = "soil"
    _direct_parent_with_local_transform(soil, root, (0.0, 0.0, 0.392))
    return root


def linked_plant_instance(
    name: str,
    source: bpy.types.Object,
    transform: tuple[float, float, float, float],
    ctx: StudioContext,
) -> bpy.types.Object:
    """Place a linked plant root and linked copies of each detailed child part."""

    x, y, z, rotation_z = transform
    if source.name == name:
        result = source
    else:
        result = source.copy()
        result.data = source.data
        result.name = name
        ctx.collections["STUDIO_VEGETATION"].objects.link(result)
        result.parent = ctx.master
        result.matrix_parent_inverse = Matrix.Identity(4)
        for source_part in source.children:
            part = source_part.copy()
            if source_part.data is not None:
                part.data = source_part.data
            part.name = source_part.name.replace(source.name, name, 1)
            ctx.collections["STUDIO_VEGETATION"].objects.link(part)
            part.parent = result
            part.matrix_parent_inverse = Matrix.Identity(4)
            part.matrix_basis = source_part.matrix_basis.copy()
            if source_part.get("plant_part") == "broad_leaf":
                part.material_slots[0].link = "OBJECT"
                part.material_slots[0].material = source_part.material_slots[0].material
    result.location = (x, y, z)
    result.rotation_euler = (0.0, 0.0, rotation_z)
    result["linked_data_key"] = contract.PLANT_LINK_KEY
    result["stem_count"] = source["stem_count"]
    result["leaf_count"] = source["leaf_count"]
    return result


def build_symmetric_floor_plants(
    ctx: StudioContext,
) -> tuple[bpy.types.Object, bpy.types.Object]:
    """Build the two reference-matched floor plants as mirrored linked assets."""

    source = build_floor_plant_source(ctx)
    left_spec, right_spec = contract.floor_plant_transforms()
    left = linked_plant_instance("FloorPlant_Left", source, left_spec, ctx)
    right = linked_plant_instance("FloorPlant_Right", source, right_spec, ctx)
    left["symmetry_partner"] = right.name
    right["symmetry_partner"] = left.name
    left["linked_data_key"] = right["linked_data_key"] = contract.PLANT_LINK_KEY
    return left, right


def _brand_icon_material(image: bpy.types.Image) -> bpy.types.Material:
    """Create the alpha-aware image material used by the framed icon plane."""

    material = bpy.data.materials.get("Brand_Icon_Alpha") or bpy.data.materials.new(
        "Brand_Icon_Alpha"
    )
    material.use_nodes = True
    nodes = material.node_tree.nodes
    nodes.clear()
    output = nodes.new("ShaderNodeOutputMaterial")
    shader = nodes.new("ShaderNodeBsdfPrincipled")
    texture = nodes.new("ShaderNodeTexImage")
    texture.name = "Brand_Icon_Packed_Texture"
    texture.image = image
    texture.interpolation = "Linear"
    _set_principled_input(shader, "Roughness", 0.58)
    _set_principled_input(shader, "Metallic", 0.0)
    material.node_tree.links.new(texture.outputs["Color"], shader.inputs["Base Color"])
    material.node_tree.links.new(texture.outputs["Alpha"], shader.inputs["Alpha"])
    material.node_tree.links.new(shader.outputs["BSDF"], output.inputs["Surface"])
    try:
        material.surface_render_method = "DITHERED"
    except (AttributeError, TypeError, ValueError):
        try:
            material.blend_method = "BLEND"
        except (AttributeError, TypeError, ValueError):
            pass
    material.use_transparency_overlap = False
    material.use_backface_culling = True
    return material


def _add_vertical_image_plane(
    ctx: StudioContext,
    name: str,
    location: tuple[float, float, float],
    size: tuple[float, float],
    material: bpy.types.Material,
) -> bpy.types.Object:
    """Create a front-facing XZ plane with deterministic full-frame UVs."""

    width, height = size
    vertices = (
        (-width / 2.0, 0.0, -height / 2.0),
        (width / 2.0, 0.0, -height / 2.0),
        (width / 2.0, 0.0, height / 2.0),
        (-width / 2.0, 0.0, height / 2.0),
    )
    mesh = bpy.data.meshes.new(name)
    mesh.from_pydata(vertices, [], ((0, 1, 2, 3),))
    mesh.validate()
    mesh.update()
    uv_layer = mesh.uv_layers.new(name="UVMap")
    for loop, uv in zip(uv_layer.data, ((0.0, 0.0), (1.0, 0.0), (1.0, 1.0), (0.0, 1.0))):
        loop.uv = uv
    mesh.materials.append(material)
    obj = bpy.data.objects.new(name, mesh)
    ctx.collections["STUDIO_BRAND"].objects.link(obj)
    obj.location = location
    parent_to_master(obj, ctx.master)
    return obj


def _add_brand_text(
    ctx: StudioContext,
    name: str,
    body: str,
    location: tuple[float, float, float],
) -> bpy.types.Object:
    """Add editable centered brand copy using Blender's built-in font."""

    text = bpy.data.curves.new(name, "FONT")
    text.body = body
    text.align_x = "CENTER"
    text.align_y = "CENTER"
    text.size = 0.105
    text.extrude = 0.002
    text.bevel_depth = 0.0008
    text.bevel_resolution = 2
    text.materials.append(ctx.materials["Brand_Copy_Dark"])
    obj = bpy.data.objects.new(name, text)
    ctx.collections["STUDIO_BRAND"].objects.link(obj)
    obj.location = location
    obj.rotation_euler = (math.pi / 2.0, 0.0, 0.0)
    parent_to_master(obj, ctx.master)
    return obj


def build_brand_art(ctx: StudioContext, icon_path: Path) -> bpy.types.Object:
    """Build packed, editable framed artwork in the back-left cabinet zone."""

    resolved_path = Path(icon_path).expanduser().resolve()
    if not resolved_path.is_file():
        raise FileNotFoundError(f"Warm studio brand icon not found: {resolved_path}")
    image = bpy.data.images.load(str(resolved_path), check_existing=True)
    image.name = resolved_path.name
    image.alpha_mode = "STRAIGHT"
    image.pack()

    icon_material = _brand_icon_material(image)
    glass_material = _principled_material(
        "Brand_Glass_Rough",
        (0.93, 0.90, 0.85, 1.0),
        roughness=0.20,
        alpha=0.05,
        transmission=0.96,
    )
    copy_material = _principled_material(
        "Brand_Copy_Dark",
        (0.055, 0.018, 0.008, 1.0),
        roughness=0.52,
    )
    ctx.materials["Brand_Icon_Alpha"] = icon_material
    ctx.materials["Brand_Glass_Rough"] = glass_material
    ctx.materials["Brand_Copy_Dark"] = copy_material

    center_x = -1.72
    center_z = 2.15
    outer_width = 0.98
    outer_height = 1.34
    frame_width = 0.060
    root = add_empty(
        ctx,
        "Brand_Artwork",
        (center_x, 2.72, center_z),
        "STUDIO_BRAND",
    )
    root["brand_zone"] = "back_left_above_cabinet"
    root["copy_language"] = "English"
    root["source_icon"] = resolved_path.name

    back_panel = add_box(
        ctx,
        "Brand_BackPanel",
        (center_x, 2.765, center_z),
        (outer_width, 0.050, outer_height),
        ctx.materials["Slat_Walnut"],
        "STUDIO_BRAND",
        bevel=0.010,
    )
    paper = add_box(
        ctx,
        "Brand_Paper",
        (center_x, 2.727, center_z),
        (outer_width - frame_width * 1.75, 0.018, outer_height - frame_width * 1.75),
        ctx.materials["Paper_Warm"],
        "STUDIO_BRAND",
        bevel=0.004,
    )
    icon = _add_vertical_image_plane(
        ctx,
        "Brand_Icon",
        (center_x, 2.711, center_z + 0.205),
        (0.50, 0.50),
        icon_material,
    )
    line1 = _add_brand_text(
        ctx,
        "Brand_Copy_Line1",
        "Slow Down.",
        (center_x, 2.706, center_z - 0.225),
    )
    line2 = _add_brand_text(
        ctx,
        "Brand_Copy_Line2",
        "Think Better.",
        (center_x, 2.706, center_z - 0.395),
    )
    glass = add_box(
        ctx,
        "Brand_Glass",
        (center_x, 2.691, center_z),
        (outer_width - frame_width * 1.72, 0.008, outer_height - frame_width * 1.72),
        glass_material,
        "STUDIO_BRAND",
        bevel=0.003,
    )
    frame_parts = [
        add_box(
            ctx,
            "Brand_Frame_Top",
            (center_x, 2.686, center_z + outer_height / 2.0 - frame_width / 2.0),
            (outer_width, 0.080, frame_width),
            ctx.materials["Metal_DarkBrown"],
            "STUDIO_BRAND",
            bevel=0.010,
        ),
        add_box(
            ctx,
            "Brand_Frame_Bottom",
            (center_x, 2.686, center_z - outer_height / 2.0 + frame_width / 2.0),
            (outer_width, 0.080, frame_width),
            ctx.materials["Metal_DarkBrown"],
            "STUDIO_BRAND",
            bevel=0.010,
        ),
        add_box(
            ctx,
            "Brand_Frame_Left",
            (center_x - outer_width / 2.0 + frame_width / 2.0, 2.686, center_z),
            (frame_width, 0.080, outer_height - frame_width * 2.0),
            ctx.materials["Metal_DarkBrown"],
            "STUDIO_BRAND",
            bevel=0.010,
        ),
        add_box(
            ctx,
            "Brand_Frame_Right",
            (center_x + outer_width / 2.0 - frame_width / 2.0, 2.686, center_z),
            (frame_width, 0.080, outer_height - frame_width * 2.0),
            ctx.materials["Metal_DarkBrown"],
            "STUDIO_BRAND",
            bevel=0.010,
        ),
    ]
    parent_assembly(
        root,
        [back_panel, paper, icon, line1, line2, glass, *frame_parts],
    )
    return root


STATIC_MARKER_SPECS = {
    "IP_Focus_Desk": ((0.80, -0.50, 1.005), "focus_desk"),
    "IP_Focus_Shelf": ((1.45, 2.50, 1.95), "focus_shelf"),
    "IP_Transition_Focus": (contract.TRANSITION_FOCUS_LOCATION, "transition_focus"),
}

MARKER_SPECS = {
    "IP_Character_Spawn": (
        contract.MODE_MARKER_SPECS["standing"]["spawn"],
        "character_spawn",
    ),
    "IP_Focus_Head": (
        contract.MODE_MARKER_SPECS["standing"]["focus"],
        "focus_head",
    ),
    "IP_Seat_Target": (
        contract.MODE_MARKER_SPECS["standing"]["seat"],
        "seat",
    ),
    "IP_Foot_Target.L": (
        contract.MODE_MARKER_SPECS["standing"]["foot_l"],
        "foot_l",
    ),
    "IP_Foot_Target.R": (
        contract.MODE_MARKER_SPECS["standing"]["foot_r"],
        "foot_r",
    ),
    **STATIC_MARKER_SPECS,
}
for _mode in contract.PRESENTATION_MODES:
    _mode_markers = contract.MODE_MARKER_SPECS[_mode]
    _mode_title = _mode.title()
    MARKER_SPECS.update(
        {
            f"IP_{_mode_title}_Spawn": (_mode_markers["spawn"], f"{_mode}_spawn"),
            f"IP_{_mode_title}_Focus_Head": (
                _mode_markers["focus"],
                f"{_mode}_focus_head",
            ),
            f"IP_{_mode_title}_Knee_Target.L": (
                _mode_markers["knee_l"],
                f"{_mode}_knee_l",
            ),
            f"IP_{_mode_title}_Knee_Target.R": (
                _mode_markers["knee_r"],
                f"{_mode}_knee_r",
            ),
            f"IP_{_mode_title}_Foot_Target.L": (
                _mode_markers["foot_l"],
                f"{_mode}_foot_l",
            ),
            f"IP_{_mode_title}_Foot_Target.R": (
                _mode_markers["foot_r"],
                f"{_mode}_foot_r",
            ),
        }
    )

CAMERA_F_STOPS = {
    "Camera_Wide": 5.6,
    "Camera_Medium": 5.0,
    "Camera_Close": 4.8,
    "Camera_ThreeQuarter_Left": 4.8,
    "Camera_ThreeQuarter_Right": 4.8,
    "Camera_Desk_Detail": 6.3,
    "Camera_Shelf_Detail": 4.0,
}
MODE_CAMERA_F_STOP = 5.0

CAMERA_RESULT_KEYS = {
    "Camera_Wide": "wide",
    "Camera_Medium": "medium",
    "Camera_Close": "close",
    "Camera_ThreeQuarter_Left": "three_quarter_left",
    "Camera_ThreeQuarter_Right": "three_quarter_right",
    "Camera_Desk_Detail": "desk_detail",
    "Camera_Shelf_Detail": "shelf_detail",
}


def _add_studio_marker(
    ctx: StudioContext,
    name: str,
    location: tuple[float, float, float],
    marker_contract: str,
) -> bpy.types.Object:
    """Create a renderer-facing empty with explicit scale-contract metadata."""

    marker = add_empty(ctx, name, location, "STUDIO_MARKERS")
    marker.empty_display_type = "CIRCLE" if name != "IP_Character_Spawn" else "ARROWS"
    marker.empty_display_size = 0.16 if name != "IP_Character_Spawn" else 0.28
    marker["contract"] = marker_contract
    marker["target_height"] = contract.TARGET_CHARACTER_HEIGHT
    marker["ip_marker_version"] = "tangying-warm-sloth-studio/v1"
    return marker


def _add_studio_camera(
    ctx: StudioContext,
    name: str,
    spec: dict[str, object],
) -> bpy.types.Object:
    """Create one restrained-DOF camera directly from the immutable contract."""

    focus_name = str(spec["focus"])
    focus = bpy.data.objects[focus_name]
    data = bpy.data.cameras.new(f"{name}_Data")
    data.lens = float(spec["lens"])
    data.sensor_width = 36.0
    data.clip_start = 0.03
    data.clip_end = 100.0
    data.dof.use_dof = True
    data.dof.focus_object = focus
    data.dof.aperture_fstop = CAMERA_F_STOPS[name]
    data.dof.aperture_blades = 9
    camera = bpy.data.objects.new(name, data)
    ctx.collections["STUDIO_CAMERAS"].objects.link(camera)
    camera.location = tuple(spec["location"])
    look_at(camera, tuple(spec["target"]))
    camera["ip_target"] = tuple(spec["target"])
    camera["ip_focus_marker"] = focus_name
    camera["ip_camera_role"] = CAMERA_RESULT_KEYS[name]
    camera["ip_framing_contract"] = "contract.CAMERA_SPECS"
    parent_to_master(camera, ctx.master)
    return camera


def _add_mode_camera(
    ctx: StudioContext,
    *,
    name: str,
    location: tuple[float, float, float],
    lens: float,
    focus_name: str,
    role: str,
) -> bpy.types.Object:
    """Create one mode-specific camera from the dual-mode contract."""

    focus = bpy.data.objects[focus_name]
    data = bpy.data.cameras.new(f"{name}_Data")
    data.lens = lens
    data.sensor_width = 36.0
    data.clip_start = 0.03
    data.clip_end = 100.0
    data.dof.use_dof = True
    data.dof.focus_object = focus
    data.dof.aperture_fstop = MODE_CAMERA_F_STOP
    data.dof.aperture_blades = 9
    camera = bpy.data.objects.new(name, data)
    ctx.collections["STUDIO_CAMERAS"].objects.link(camera)
    camera.location = location
    look_at(camera, tuple(focus.location))
    camera["ip_focus_marker"] = focus_name
    camera["ip_camera_role"] = role
    camera["ip_framing_contract"] = "contract.MODE_CAMERA_SPECS"
    parent_to_master(camera, ctx.master)
    return camera


def build_markers_and_cameras(ctx: StudioContext) -> dict[str, bpy.types.Object]:
    """Build legacy and dual-mode markers with their authored cameras."""

    for name, (location, marker_contract) in MARKER_SPECS.items():
        _add_studio_marker(ctx, name, location, marker_contract)

    result: dict[str, bpy.types.Object] = {}
    for name, spec in contract.CAMERA_SPECS.items():
        result[CAMERA_RESULT_KEYS[name]] = _add_studio_camera(ctx, name, spec)
    for mode in contract.PRESENTATION_MODES:
        for role, (name, location, lens) in contract.MODE_CAMERA_SPECS[mode].items():
            focus_name = (
                "IP_Transition_Focus"
                if role == "transition"
                else f"IP_{mode.title()}_Focus_Head"
            )
            _add_mode_camera(
                ctx,
                name=name,
                location=location,
                lens=lens,
                focus_name=focus_name,
                role=role,
            )
    ctx.scene["ip_presentation_modes"] = json.dumps(list(contract.PRESENTATION_MODES))
    ctx.scene["ip_subject_light_profile"] = contract.SUBJECT_LIGHT_PROFILE["name"]
    ctx.scene["ip_background_stops_below_face"] = contract.SUBJECT_LIGHT_PROFILE[
        "backgroundStopsBelowFace"
    ]
    ctx.scene.camera = result["wide"]
    return result


LIGHT_SPECS = (
    {
        "name": "Window_Softbox",
        "type": "AREA",
        "location": (-3.00, -0.10, 2.35),
        "energy": contract.SUBJECT_LIGHT_PROFILE["windowEnergy"],
        "color": (1.0, 1.0, 1.0),
        "temperature": 4800,
        "role": "window",
        "target": (0.0, 0.35, 1.35),
        "size": (2.25, 2.65),
        "shadows": True,
    },
    {
        "name": "IP_Subject_Key",
        "type": "SPOT",
        "location": (-1.10, -0.55, 2.55),
        "energy": contract.SUBJECT_LIGHT_PROFILE["keyEnergy"],
        "color": (1.0, 1.0, 1.0),
        "temperature": contract.SUBJECT_LIGHT_PROFILE["keyTemperatureK"],
        "role": "key",
        "target": contract.SUBJECT_LIGHT_PROFILE["keyTarget"],
        "size": (contract.SUBJECT_LIGHT_PROFILE["keySoftRadius"],) * 2,
        "spot_size_degrees": contract.SUBJECT_LIGHT_PROFILE["keySpotSizeDegrees"],
        "spot_blend": contract.SUBJECT_LIGHT_PROFILE["keySpotBlend"],
        "shadows": True,
    },
    {
        "name": "IP_Subject_Fill",
        "type": "AREA",
        "location": (2.55, -1.60, 2.35),
        "energy": contract.SUBJECT_LIGHT_PROFILE["fillEnergy"],
        "color": (1.0, 1.0, 1.0),
        "temperature": 5200,
        "role": "fill",
        "target": contract.MODE_MARKER_SPECS["standing"]["focus"],
        "size": (2.20, 2.20),
        "shadows": False,
    },
    {
        "name": "IP_Subject_FrontFill",
        "type": "AREA",
        "location": (0.0, -1.60, 1.55),
        "energy": contract.SUBJECT_LIGHT_PROFILE["frontFillEnergy"],
        "color": (1.0, 1.0, 1.0),
        "temperature": contract.SUBJECT_LIGHT_PROFILE["frontFillTemperatureK"],
        "role": "front_fill",
        "target": contract.SUBJECT_LIGHT_PROFILE["frontFillTarget"],
        "size": (2.80, 2.00),
        "shadows": False,
    },
    {
        "name": "IP_Subject_Rim",
        "type": "SPOT",
        "location": (2.20, 1.85, 2.65),
        "energy": contract.SUBJECT_LIGHT_PROFILE["rimEnergy"],
        "color": (1.0, 1.0, 1.0),
        "temperature": contract.SUBJECT_LIGHT_PROFILE["rimTemperatureK"],
        "role": "rim",
        "target": contract.MODE_MARKER_SPECS["standing"]["focus"],
        "size": (contract.SUBJECT_LIGHT_PROFILE["rimSoftRadius"],) * 2,
        "spot_size_degrees": contract.SUBJECT_LIGHT_PROFILE["rimSpotSizeDegrees"],
        "spot_blend": contract.SUBJECT_LIGHT_PROFILE["rimSpotBlend"],
        "shadows": True,
    },
)

DOWNLIGHT_SPECS = (
    ("Downlight_01", (-1.72, -0.35, 3.345)),
    ("Downlight_02", (0.0, 1.05, 3.345)),
    ("Downlight_03", (1.72, -0.35, 3.345)),
)


def _add_authored_light(
    ctx: StudioContext,
    *,
    name: str,
    light_type: str,
    location: tuple[float, float, float],
    energy: float,
    color: tuple[float, float, float],
    temperature: int,
    role: str,
    shadows: bool,
    target: tuple[float, float, float] | None = None,
    size: tuple[float, float] = (0.25, 0.25),
    spread_degrees: float | None = None,
    spot_size_degrees: float | None = None,
    spot_blend: float | None = None,
    fixture: str | None = None,
) -> bpy.types.Object:
    """Create one tagged production light with reproducible authored energy."""

    data = bpy.data.lights.new(f"{name}_Data", light_type)
    data.energy = energy
    data.color = color
    data.use_temperature = True
    data.temperature = float(temperature)
    data.use_shadow = shadows
    data.diffuse_factor = 1.0
    data.specular_factor = 0.55 if role in {"window", "key"} else 0.30
    if light_type == "AREA":
        data.shape = "RECTANGLE"
        data.size = size[0]
        data.size_y = size[1]
        if spread_degrees is not None:
            data.spread = math.radians(spread_degrees)
    elif light_type == "POINT":
        data.shadow_soft_size = size[0]
    elif light_type == "SPOT":
        data.shadow_soft_size = size[0]
        if spot_size_degrees is not None:
            data.spot_size = math.radians(spot_size_degrees)
        if spot_blend is not None:
            data.spot_blend = spot_blend

    light = bpy.data.objects.new(name, data)
    ctx.collections["STUDIO_LIGHTS"].objects.link(light)
    light.location = location
    if target is not None:
        look_at(light, target)
        light["ip_target"] = target
    light["ip_base_energy"] = float(energy)
    light["ip_light_role"] = role
    light["ip_authored_color"] = color
    light["ip_color_temperature"] = int(temperature)
    if spread_degrees is not None:
        light["ip_spread_degrees"] = float(spread_degrees)
    if spot_size_degrees is not None:
        light["ip_spot_size_degrees"] = float(spot_size_degrees)
    if spot_blend is not None:
        light["ip_spot_blend"] = float(spot_blend)
    light["ip_casts_shadow"] = bool(shadows)
    light["ip_shadow_mode"] = "full" if shadows else "restrained"
    if fixture is not None:
        light["ip_fixture"] = fixture
    parent_to_master(light, ctx.master)
    return light


def _add_downlight_fixture(
    ctx: StudioContext,
    name: str,
    location: tuple[float, float, float],
) -> None:
    """Add a simple recessed trim/diffuser so each authored downlight stays visible."""

    trim = add_cylinder(
        ctx,
        f"{name}_Trim",
        (location[0], location[1], 3.385),
        0.115,
        0.030,
        ctx.materials["Metal_DarkBrown"],
        "STUDIO_LIGHTS",
        vertices=40,
        bevel=0.005,
    )
    diffuser = add_cylinder(
        ctx,
        f"{name}_Diffuser",
        (location[0], location[1], 3.370),
        0.082,
        0.012,
        ctx.materials["Lamp_Emissive_Warm"],
        "STUDIO_LIGHTS",
        vertices=40,
        bevel=0.003,
    )
    trim["ip_fixture_light"] = name
    diffuser["ip_fixture_light"] = name


def _build_wall_sconce(ctx: StudioContext) -> bpy.types.Object:
    """Build the distinct warm practical centered above the brand artwork."""

    root = add_empty(
        ctx,
        "Wall_Sconce",
        (-1.70, 2.78, 2.91),
        "STUDIO_LIGHTS",
    )
    root["fixture_role"] = "brand_wall_sconce"
    root["fixture_light"] = "Practical_Wall"
    backplate = add_cylinder(
        ctx,
        "Wall_Sconce_Backplate",
        (-1.70, 2.810, 2.910),
        0.058,
        0.032,
        ctx.materials["Metal_DarkBrown"],
        "STUDIO_LIGHTS",
        rotation=(math.pi / 2.0, 0.0, 0.0),
        vertices=40,
        bevel=0.006,
    )
    arm = add_curve_profile(
        ctx,
        "Wall_Sconce_Arm",
        [
            (-1.70, 2.785, 2.915),
            (-1.70, 2.610, 2.915),
            (-1.70, 2.515, 2.895),
        ],
        ctx.materials["Metal_DarkBrown"],
        "STUDIO_LIGHTS",
        bevel_depth=0.014,
    )
    shade = add_open_frustum(
        ctx,
        "Wall_Sconce_Shade",
        (-1.70, 2.425, 2.895),
        0.105,
        0.064,
        0.085,
        ctx.materials["Lamp_Shade_Warm"],
        "STUDIO_LIGHTS",
        vertices=40,
        thickness=0.006,
    )
    diffuser = add_uv_sphere(
        ctx,
        "Wall_Sconce_Diffuser",
        (-1.70, 2.360, 2.8555),
        (0.040, 0.040, 0.040),
        ctx.materials["Lamp_Emissive_Warm"],
        "STUDIO_LIGHTS",
        segments=48,
        ring_count=24,
    )
    diffuser["fixture_role"] = "visible_warm_bulb"
    parent_assembly(root, [backplate, arm, shade, diffuser])
    return diffuser


def build_lighting(ctx: StudioContext) -> list[bpy.types.Object]:
    """Build warm daylight, practical, and recessed roles for Eevee/Cycles."""

    lights = [
        _add_authored_light(
            ctx,
            name=str(spec["name"]),
            light_type=str(spec["type"]),
            location=tuple(spec["location"]),
            energy=float(spec["energy"]),
            color=tuple(spec["color"]),
            temperature=int(spec["temperature"]),
            role=str(spec["role"]),
            shadows=bool(spec["shadows"]),
            target=tuple(spec["target"]),
            size=tuple(spec["size"]),
            spread_degrees=(
                float(spec["spread_degrees"])
                if "spread_degrees" in spec
                else None
            ),
            spot_size_degrees=(
                float(spec["spot_size_degrees"])
                if "spot_size_degrees" in spec
                else None
            ),
            spot_blend=(
                float(spec["spot_blend"])
                if "spot_blend" in spec
                else None
            ),
        )
        for spec in LIGHT_SPECS
    ]
    wall_sconce_diffuser = _build_wall_sconce(ctx)
    bpy.context.view_layer.update()
    practical_specs = (
        (
            "Practical_Wall",
            wall_sconce_diffuser.name,
            contract.SUBJECT_LIGHT_PROFILE["practicalWallEnergy"],
        ),
        (
            "Practical_Shelf",
            "Shelf_TableLamp_Diffuser",
            contract.SUBJECT_LIGHT_PROFILE["practicalShelfEnergy"],
        ),
    )
    for name, fixture_name, energy in practical_specs:
        fixture = bpy.data.objects[fixture_name]
        lights.append(
            _add_authored_light(
                ctx,
                name=name,
                light_type="POINT",
                location=tuple(fixture.matrix_world.translation),
                energy=energy,
                color=(1.0, 1.0, 1.0),
                temperature=contract.SUBJECT_LIGHT_PROFILE["practicalTemperatureK"],
                role="practical",
                shadows=False,
                size=(0.16, 0.16),
                fixture=fixture_name,
            )
        )

    for name, location in DOWNLIGHT_SPECS:
        _add_downlight_fixture(ctx, name, location)
        lights.append(
            _add_authored_light(
                ctx,
                name=name,
                light_type="AREA",
                location=location,
                energy=contract.SUBJECT_LIGHT_PROFILE["downlightEnergy"],
                color=(1.0, 1.0, 1.0),
                temperature=3000,
                role="downlight",
                shadows=True,
                target=(location[0], location[1], 0.72),
                size=(0.34, 0.34),
                fixture=f"{name}_Diffuser",
            )
        )
    return lights


def configure_render_settings(ctx: StudioContext) -> None:
    """Set production defaults for fast Eevee previews and Cycles handoff."""

    scene = ctx.scene
    try:
        scene.render.engine = "BLENDER_EEVEE_NEXT"
    except TypeError:
        scene.render.engine = "BLENDER_EEVEE"
    scene.render.resolution_x, scene.render.resolution_y = (
        contract.DEFAULT_RENDER_RESOLUTION
    )
    scene.render.resolution_percentage = 100
    scene.render.pixel_aspect_x = 1.0
    scene.render.pixel_aspect_y = 1.0
    scene.render.fps = 30
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"
    scene.render.image_settings.color_depth = "8"
    scene.render.film_transparent = False

    eevee = getattr(scene, "eevee", None)
    if eevee is not None:
        if hasattr(eevee, "taa_samples"):
            eevee.taa_samples = 16
        if hasattr(eevee, "taa_render_samples"):
            eevee.taa_render_samples = 128
        if hasattr(eevee, "use_raytracing"):
            eevee.use_raytracing = False
        if hasattr(eevee, "use_shadows"):
            eevee.use_shadows = True
        if hasattr(eevee, "shadow_pool_size"):
            eevee.shadow_pool_size = "1024"

    scene.cycles.samples = 128
    scene.cycles.use_denoising = True
    scene["ip_preview_engine"] = "BLENDER_EEVEE_NEXT"
    scene["ip_final_engine"] = "CYCLES"
    scene["ip_eevee_render_samples"] = 128
    scene["ip_cycles_final_samples"] = 128
    scene["ip_cycles_final_exposure"] = CYCLES_FINAL_EXPOSURE
    scene["ip_cycles_key_energy_multiplier"] = contract.SUBJECT_LIGHT_PROFILE[
        "cyclesKeyEnergyMultiplier"
    ]

    try:
        scene.view_settings.view_transform = "AgX"
        for look_name in ("AgX - Medium High Contrast", "Medium High Contrast"):
            try:
                scene.view_settings.look = look_name
                break
            except (TypeError, ValueError):
                continue
    except (TypeError, ValueError):
        scene.view_settings.view_transform = "Filmic"
        scene.view_settings.look = "Medium High Contrast"
    scene.view_settings.exposure = AUTHORED_EXPOSURE
    scene["ip_authored_exposure"] = AUTHORED_EXPOSURE
    scene.view_settings.use_white_balance = True
    scene.view_settings.white_balance_temperature = contract.SUBJECT_LIGHT_PROFILE[
        "whiteBalanceTemperatureK"
    ]
    scene.view_settings.white_balance_tint = contract.SUBJECT_LIGHT_PROFILE[
        "whiteBalanceTint"
    ]
    scene["ip_white_balance_temperature"] = contract.SUBJECT_LIGHT_PROFILE[
        "whiteBalanceTemperatureK"
    ]
    scene["ip_white_balance_tint"] = contract.SUBJECT_LIGHT_PROFILE[
        "whiteBalanceTint"
    ]

    world = bpy.data.worlds.get("World_WarmStudio_Neutral") or bpy.data.worlds.new(
        "World_WarmStudio_Neutral"
    )
    world.use_nodes = True
    background = next(node for node in world.node_tree.nodes if node.type == "BACKGROUND")
    background.inputs["Color"].default_value = (0.075, 0.070, 0.065, 1.0)
    background.inputs["Strength"].default_value = contract.SUBJECT_LIGHT_PROFILE[
        "worldStrength"
    ]
    world["ip_world_role"] = "neutral_low_strength"
    scene.world = world


MANIFEST_VERSION = "tangying-warm-studio-manifest/v1"
MANIFEST_TEXT_NAME = "WarmStudio_AuthoredManifest"


def _manifest_object_entry(obj: bpy.types.Object) -> dict[str, object]:
    """Return stable authored identity for one active-scene object."""

    return {
        "name": obj.name,
        "type": obj.type,
        "parent": obj.parent.name if obj.parent is not None else None,
        "collections": sorted(collection.name for collection in obj.users_collection),
    }


def stamp_authored_manifest(ctx: StudioContext) -> dict[str, object]:
    """Freeze the exact authored object set for later deletion/injection detection."""

    entries = sorted(
        (_manifest_object_entry(obj) for obj in ctx.scene.objects),
        key=lambda item: str(item["name"]),
    )
    manifest: dict[str, object] = {
        "schemaVersion": MANIFEST_VERSION,
        "objectCount": len(entries),
        "objects": entries,
    }
    payload = json.dumps(
        manifest,
        ensure_ascii=False,
        sort_keys=True,
        separators=(",", ":"),
    )
    digest = hashlib.sha256(payload.encode("utf-8")).hexdigest()
    text = bpy.data.texts.get(MANIFEST_TEXT_NAME) or bpy.data.texts.new(
        MANIFEST_TEXT_NAME
    )
    text.clear()
    text.write(payload)
    ctx.master["ip_manifest_version"] = MANIFEST_VERSION
    ctx.master["ip_manifest_text"] = text.name
    ctx.master["ip_manifest_sha256"] = digest
    ctx.master["ip_manifest_object_count"] = len(entries)
    return manifest


def build_scene(
    *,
    include_brand: bool = True,
    include_lighting: bool = True,
    brand_icon_path: Path | None = None,
) -> dict[str, object]:
    """Build the complete empty warm studio through explicit production stages."""

    ctx = create_scene_context()
    build_architecture(ctx)
    furniture = build_furniture(ctx)
    build_set_dressing(ctx)
    build_symmetric_floor_plants(ctx)
    if include_brand:
        icon_path = brand_icon_path or (
            contract.repo_root()
            / "ip形象/main_ip/scenes/assets/warm-sloth-brand-icon.png"
        )
        build_brand_art(ctx, icon_path)
    cameras = build_markers_and_cameras(ctx)
    if include_lighting:
        build_lighting(ctx)
    configure_render_settings(ctx)
    ctx.scene.camera = cameras["wide"]
    stamp_authored_manifest(ctx)
    return {"context": ctx, **furniture, **cameras}


def _default_output_path(relative_path: str) -> Path:
    """Resolve one approved deliverable path from this checkout's repository root."""

    return (contract.repo_root() / relative_path).resolve()


def parse_cli_args(argv: list[str] | None = None) -> argparse.Namespace:
    """Parse optional builder paths, using only arguments after Blender's ``--``."""

    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "blend_path",
        nargs="?",
        type=Path,
        default=_default_output_path(
            "ip形象/main_ip/scenes/warm-sloth-studio-v1.blend"
        ),
        help="Packed warm studio .blend output path",
    )
    parser.add_argument(
        "preview_path",
        nargs="?",
        type=Path,
        default=_default_output_path(
            "ip形象/main_ip/scenes/warm-sloth-studio-v1-preview.png"
        ),
        help="Opaque Camera_Wide PNG preview output path",
    )
    parser.add_argument(
        "brand_icon_path",
        nargs="?",
        type=Path,
        default=_default_output_path(
            "ip形象/main_ip/scenes/assets/warm-sloth-brand-icon.png"
        ),
        help="Approved transparent warm sloth brand icon",
    )
    args = parser.parse_args(argv)
    args.blend_path = args.blend_path.expanduser().resolve()
    args.preview_path = args.preview_path.expanduser().resolve()
    args.brand_icon_path = args.brand_icon_path.expanduser().resolve()
    return args


def _arguments_after_blender_separator(argv: list[str]) -> list[str]:
    """Return script-owned arguments without exposing Blender's own flags."""

    if "--" not in argv:
        return []
    return argv[argv.index("--") + 1 :]


def _render_wide_preview(scene: bpy.types.Scene, output_path: Path) -> None:
    """Render the approved opaque 960x540 hero still and restore scene defaults."""

    wide = bpy.data.objects.get("Camera_Wide")
    if wide is None or wide.type != "CAMERA":
        raise RuntimeError("Camera_Wide is required before rendering the preview")

    original = {
        "filepath": scene.render.filepath,
        "resolution_x": scene.render.resolution_x,
        "resolution_y": scene.render.resolution_y,
        "resolution_percentage": scene.render.resolution_percentage,
        "film_transparent": scene.render.film_transparent,
        "camera": scene.camera,
    }
    output_path.parent.mkdir(parents=True, exist_ok=True)
    try:
        scene.camera = wide
        scene.render.resolution_x = 960
        scene.render.resolution_y = 540
        scene.render.resolution_percentage = 100
        scene.render.film_transparent = False
        scene.render.image_settings.file_format = "PNG"
        scene.render.image_settings.color_mode = "RGBA"
        scene.render.filepath = str(output_path)
        bpy.ops.render.render(write_still=True)
    except Exception:
        output_path.unlink(missing_ok=True)
        raise
    finally:
        scene.render.filepath = original["filepath"]
        scene.render.resolution_x = original["resolution_x"]
        scene.render.resolution_y = original["resolution_y"]
        scene.render.resolution_percentage = original["resolution_percentage"]
        scene.render.film_transparent = original["film_transparent"]
        scene.camera = original["camera"]


def main(argv: list[str] | None = None) -> int:
    """Build, preview, pack, and save the reusable warm sloth studio scene."""

    script_args = (
        _arguments_after_blender_separator(sys.argv) if argv is None else argv
    )
    args = parse_cli_args(script_args)
    named_paths = (
        ("blend_path", args.blend_path),
        ("preview_path", args.preview_path),
        ("brand_icon_path", args.brand_icon_path),
    )
    for index, (left_name, left_path) in enumerate(named_paths):
        for right_name, right_path in named_paths[index + 1 :]:
            if left_path == right_path:
                raise ValueError(
                    "Warm studio CLI paths must be pairwise distinct: "
                    f"{left_name} and {right_name} both resolve to {left_path}"
                )
    if not args.brand_icon_path.is_file():
        raise FileNotFoundError(
            f"Warm studio brand icon does not exist: {args.brand_icon_path}"
        )

    built = build_scene(
        include_brand=True,
        include_lighting=True,
        brand_icon_path=args.brand_icon_path,
    )
    scene = built["context"].scene
    wide = bpy.data.objects["Camera_Wide"]
    bpy.ops.file.pack_all()
    _render_wide_preview(scene, args.preview_path)

    scene.render.resolution_x, scene.render.resolution_y = (
        contract.DEFAULT_RENDER_RESOLUTION
    )
    scene.render.resolution_percentage = 100
    scene.render.film_transparent = False
    scene.camera = wide
    args.blend_path.parent.mkdir(parents=True, exist_ok=True)
    bpy.ops.wm.save_as_mainfile(
        filepath=str(args.blend_path),
        check_existing=False,
    )

    print(f"WARM_STUDIO_BLEND={args.blend_path}")
    print(f"WARM_STUDIO_PREVIEW={args.preview_path}")
    print(f"WARM_STUDIO_BRAND_ICON={args.brand_icon_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
