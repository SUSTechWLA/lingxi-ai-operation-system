#!/usr/bin/env python3
"""Inspect facial topology in a source character file from Blender headlessly."""

from __future__ import annotations

import argparse
import json
import sys
from array import array
from collections import Counter, deque
from pathlib import Path
from typing import Any

import bpy
from mathutils import Vector


SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import blender_renderer  # noqa: E402


def parse_args() -> argparse.Namespace:
    arguments = sys.argv[sys.argv.index("--") + 1 :] if "--" in sys.argv else []
    parser = argparse.ArgumentParser()
    parser.add_argument("--model", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--target-height", type=float, default=2.55)
    parser.add_argument("--render-dir")
    return parser.parse_args(arguments)


def vector_list(value: Vector) -> list[float]:
    return [round(float(axis), 6) for axis in value]


def component_records(obj: bpy.types.Object, head_group_names: set[str]) -> list[dict[str, Any]]:
    mesh = obj.data
    adjacency = [set() for _ in mesh.vertices]
    for edge in mesh.edges:
        left, right = (int(index) for index in edge.vertices)
        adjacency[left].add(right)
        adjacency[right].add(left)

    component_index = [-1] * len(mesh.vertices)
    components: list[list[int]] = []
    for start in range(len(mesh.vertices)):
        if component_index[start] >= 0:
            continue
        index = len(components)
        queue = deque([start])
        component_index[start] = index
        vertices: list[int] = []
        while queue:
            vertex_index = queue.popleft()
            vertices.append(vertex_index)
            for neighbor in adjacency[vertex_index]:
                if component_index[neighbor] < 0:
                    component_index[neighbor] = index
                    queue.append(neighbor)
        components.append(vertices)

    polygon_counts: Counter[int] = Counter()
    material_counts: dict[int, Counter[int]] = {}
    edge_face_counts: Counter[tuple[int, int]] = Counter()
    for polygon in mesh.polygons:
        indices = [int(index) for index in polygon.vertices]
        component = component_index[indices[0]]
        polygon_counts[component] += 1
        material_counts.setdefault(component, Counter())[int(polygon.material_index)] += 1
        for offset, left in enumerate(indices):
            right = indices[(offset + 1) % len(indices)]
            edge_face_counts[tuple(sorted((left, right)))] += 1

    head_group_indices = {
        group.index for group in obj.vertex_groups if group.name in head_group_names
    }
    records: list[dict[str, Any]] = []
    for index, vertex_indices in enumerate(components):
        world_points = [obj.matrix_world @ mesh.vertices[vertex_index].co for vertex_index in vertex_indices]
        minimum = Vector(tuple(min(float(point[axis]) for point in world_points) for axis in range(3)))
        maximum = Vector(tuple(max(float(point[axis]) for point in world_points) for axis in range(3)))
        weighted_head_vertices = 0
        total_head_weight = 0.0
        for vertex_index in vertex_indices:
            assignments = mesh.vertices[vertex_index].groups
            head_weight = sum(
                float(item.weight) for item in assignments if item.group in head_group_indices
            )
            if head_weight > 0.05:
                weighted_head_vertices += 1
                total_head_weight += head_weight

        component_vertex_set = set(vertex_indices)
        component_edges = [
            edge for edge in mesh.edges
            if int(edge.vertices[0]) in component_vertex_set
            and int(edge.vertices[1]) in component_vertex_set
        ]
        non_manifold_edges = sum(
            edge_face_counts[tuple(sorted((int(edge.vertices[0]), int(edge.vertices[1]))))] != 2
            for edge in component_edges
        )
        materials = {
            (obj.material_slots[material_index].material.name
             if material_index < len(obj.material_slots)
             and obj.material_slots[material_index].material
             else f"slot_{material_index}"): count
            for material_index, count in material_counts.get(index, {}).items()
        }
        records.append(
            {
                "component": index,
                "vertexCount": len(vertex_indices),
                "edgeCount": len(component_edges),
                "faceCount": int(polygon_counts[index]),
                "boundsMin": vector_list(minimum),
                "boundsMax": vector_list(maximum),
                "dimensions": vector_list(maximum - minimum),
                "center": vector_list((minimum + maximum) * 0.5),
                "headWeightedVertexCount": weighted_head_vertices,
                "meanHeadWeight": round(total_head_weight / max(1, weighted_head_vertices), 6),
                "nonManifoldEdgeCount": non_manifold_edges,
                "materials": materials,
            }
        )
    return sorted(records, key=lambda record: record["vertexCount"], reverse=True)


def sampled_vertex_luminance(obj: bpy.types.Object) -> dict[int, float]:
    uv_layer = obj.data.uv_layers.active
    if not uv_layer:
        return {}
    source_image = next(
        (
            node.image
            for material in obj.data.materials
            if material and material.use_nodes
            for node in material.node_tree.nodes
            if node.type == "TEX_IMAGE"
            and getattr(node, "image", None)
            and node.image.colorspace_settings.name == "sRGB"
        ),
        None,
    )
    if not source_image:
        return {}
    sample = source_image.copy()
    try:
        sample.scale(512, 512)
        width, height = int(sample.size[0]), int(sample.size[1])
        pixels = array("f", [0.0]) * (width * height * 4)
        sample.pixels.foreach_get(pixels)
        luminance: dict[int, float] = {}
        for polygon in obj.data.polygons:
            for loop_index in polygon.loop_indices:
                vertex_index = obj.data.loops[loop_index].vertex_index
                uv = uv_layer.data[loop_index].uv
                pixel_x = max(0, min(width - 1, int((float(uv.x) % 1.0) * (width - 1))))
                pixel_y = max(0, min(height - 1, int((float(uv.y) % 1.0) * (height - 1))))
                offset = (pixel_y * width + pixel_x) * 4
                value = pixels[offset] * 0.2126 + pixels[offset + 1] * 0.7152 + pixels[offset + 2] * 0.0722
                luminance[vertex_index] = min(luminance.get(vertex_index, 1.0), float(value))
        return luminance
    finally:
        bpy.data.images.remove(sample)


def facial_region_report(
    character_objects: list[bpy.types.Object], dimensions: dict[str, Any]
) -> dict[str, Any]:
    head_bounds = blender_renderer.weighted_region_bounds(character_objects, "head", minimum_weight=0.25)
    if not head_bounds:
        return {}
    center_x = (float(head_bounds["min"].x) + float(head_bounds["max"].x)) * 0.5
    mouth_center_z = float(dimensions["min"].z) + float(dimensions["height"]) * 0.805
    mouth_radius_x = float(head_bounds["width"]) * 0.18
    mouth_radius_z = float(dimensions["height"]) * 0.030
    front_limit = float(head_bounds["min"].y) + float(head_bounds["depth"]) * 0.25
    payload: dict[str, Any] = {}
    for obj in character_objects:
        if obj.type != "MESH":
            continue
        luminance = sampled_vertex_luminance(obj)
        adjacency = {vertex.index: set() for vertex in obj.data.vertices}
        for edge in obj.data.edges:
            left, right = (int(index) for index in edge.vertices)
            adjacency[left].add(right)
            adjacency[right].add(left)
        records = []
        for vertex in obj.data.vertices:
            world = obj.matrix_world @ vertex.co
            if (
                abs(float(world.x) - center_x) <= mouth_radius_x
                and abs(float(world.z) - mouth_center_z) <= mouth_radius_z
                and float(world.y) <= front_limit
            ):
                records.append(
                    {
                        "vertex": vertex.index,
                        "world": vector_list(world),
                        "luminance": round(luminance.get(vertex.index, 1.0), 6),
                        "valence": len(adjacency[vertex.index]),
                    }
                )
        records.sort(key=lambda record: (record["luminance"], abs(record["world"][2] - mouth_center_z)))
        dark_indices = {record["vertex"] for record in records if record["luminance"] < 0.46}
        dark_edges = [
            [int(edge.vertices[0]), int(edge.vertices[1])]
            for edge in obj.data.edges
            if int(edge.vertices[0]) in dark_indices and int(edge.vertices[1]) in dark_indices
        ]
        if records:
            payload[obj.name] = {
                "mouthCenter": [round(center_x, 6), round(mouth_center_z, 6)],
                "candidateCount": len(records),
                "darkSeedCount": len(dark_indices),
                "darkSeedEdges": dark_edges,
                "darkestCandidates": records[:160],
            }
    return payload


def make_material(name: str, color: tuple[float, float, float, float], *, emission: bool = False) -> bpy.types.Material:
    material = bpy.data.materials.new(name)
    material.use_nodes = True
    nodes = material.node_tree.nodes
    links = material.node_tree.links
    nodes.clear()
    output = nodes.new("ShaderNodeOutputMaterial")
    shader = nodes.new("ShaderNodeEmission" if emission else "ShaderNodeBsdfPrincipled")
    if emission:
        shader.inputs["Color"].default_value = color
        shader.inputs["Strength"].default_value = 0.35
    else:
        shader.inputs["Base Color"].default_value = color
        shader.inputs["Roughness"].default_value = 0.72
    links.new(shader.outputs[0], output.inputs["Surface"])
    return material


def point_at(obj: bpy.types.Object, target: Vector) -> None:
    obj.rotation_euler = (target - obj.location).to_track_quat("-Z", "Y").to_euler()


def render_face_debug(
    character_objects: list[bpy.types.Object],
    dimensions: dict[str, Any],
    output_dir: Path,
) -> dict[str, str]:
    output_dir.mkdir(parents=True, exist_ok=True)
    head_bounds = blender_renderer.weighted_region_bounds(character_objects, "head", minimum_weight=0.25)
    if not head_bounds:
        raise RuntimeError("cannot render face debug without a weighted head region")
    center = (head_bounds["min"] + head_bounds["max"]) * 0.5
    front_y = float(head_bounds["min"].y)

    camera_data = bpy.data.cameras.new("SourceFaceDebugCamera")
    camera_data.type = "ORTHO"
    camera_data.ortho_scale = max(float(head_bounds["height"]) * 1.22, float(head_bounds["width"]) * 1.12)
    camera = bpy.data.objects.new(camera_data.name, camera_data)
    bpy.context.collection.objects.link(camera)
    camera.location = Vector((float(center.x), front_y - max(2.0, float(dimensions["depth"]) * 4.0), float(center.z) - float(head_bounds["height"]) * 0.015))
    point_at(camera, center)
    bpy.context.scene.camera = camera

    world = bpy.data.worlds.new("SourceFaceDebugWorld")
    world.use_nodes = True
    world.node_tree.nodes["Background"].inputs["Color"].default_value = (0.035, 0.035, 0.035, 1.0)
    world.node_tree.nodes["Background"].inputs["Strength"].default_value = 0.16
    bpy.context.scene.world = world

    for name, location, energy, size in (
        ("Key", (-1.4, front_y - 1.8, float(center.z) + 1.0), 760.0, 2.4),
        ("Fill", (1.6, front_y - 1.0, float(center.z) + 0.35), 300.0, 2.8),
        ("Rim", (0.8, float(head_bounds["max"].y) + 1.0, float(center.z) + 0.8), 520.0, 1.8),
    ):
        light_data = bpy.data.lights.new(f"SourceFace{name}", "AREA")
        light_data.energy = energy
        light_data.shape = "DISK"
        light_data.size = size
        light = bpy.data.objects.new(light_data.name, light_data)
        bpy.context.collection.objects.link(light)
        light.location = Vector(location)
        point_at(light, center)

    scene = bpy.context.scene
    scene.render.engine = "BLENDER_EEVEE"
    scene.render.resolution_x = 1024
    scene.render.resolution_y = 1024
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "PNG"
    scene.render.film_transparent = False
    scene.view_settings.look = "AgX - Medium High Contrast"
    scene.view_settings.exposure = -0.65

    material_path = output_dir / "source-face-material.png"
    scene.render.filepath = str(material_path)
    bpy.ops.render.render(write_still=True)

    clay_shades = [
        make_material(f"SourceFaceClay{index}", (value, value * 0.91, value * 0.80, 1.0))
        for index, value in enumerate((0.34, 0.46, 0.58, 0.70))
    ]
    wire = make_material("SourceFaceWire", (0.004, 0.006, 0.008, 1.0), emission=True)
    wire_objects: list[bpy.types.Object] = []
    for obj in character_objects:
        if obj.type != "MESH":
            continue
        obj.data.materials.clear()
        for material in clay_shades:
            obj.data.materials.append(material)
        for polygon in obj.data.polygons:
            polygon.material_index = (polygon.index * 2654435761) % len(clay_shades)
        duplicate = obj.copy()
        duplicate.data = obj.data.copy()
        bpy.context.collection.objects.link(duplicate)
        duplicate.name = f"{obj.name}_Wire"
        duplicate.data.materials.clear()
        duplicate.data.materials.append(wire)
        modifier = duplicate.modifiers.new("SourceTopologyWire", "WIREFRAME")
        modifier.thickness = max(0.0015, float(head_bounds["width"]) * 0.0030)
        modifier.use_replace = True
        modifier.use_boundary = True
        wire_objects.append(duplicate)

    wire_path = output_dir / "source-face-wire.png"
    scene.render.filepath = str(wire_path)
    scene.view_settings.exposure = -0.25
    bpy.ops.render.render(write_still=True)
    return {"material": str(material_path), "wire": str(wire_path)}


def main() -> None:
    args = parse_args()
    bpy.ops.wm.read_factory_settings(use_empty=True)
    character_objects, armatures, imported_assets, removed = blender_renderer.import_model(args.model)
    dimensions = blender_renderer.prepare_character(
        character_objects,
        target_height=args.target_height,
        preserve_hierarchy=True,
        asset_objects=imported_assets,
    )
    head_group_names = {
        group.name
        for obj in character_objects
        for group in obj.vertex_groups
        if group.name.lower().endswith("head")
    }
    report = {
        "source": str(Path(args.model).resolve()),
        "removedHelpers": removed,
        "dimensions": {
            key: vector_list(value) if isinstance(value, Vector) else round(float(value), 6)
            for key, value in dimensions.items()
            if key in {"min", "max", "width", "depth", "height"}
        },
        "armatures": [
            {
                "name": armature.name,
                "boneCount": len(armature.data.bones),
                "bones": [bone.name for bone in armature.data.bones],
            }
            for armature in armatures
        ],
        "headGroups": sorted(head_group_names),
        "meshes": [],
    }
    for obj in character_objects:
        if obj.type != "MESH":
            continue
        report["meshes"].append(
            {
                "name": obj.name,
                "vertexCount": len(obj.data.vertices),
                "edgeCount": len(obj.data.edges),
                "faceCount": len(obj.data.polygons),
                "materialSlots": [
                    slot.material.name if slot.material else None for slot in obj.material_slots
                ],
                "vertexGroups": [group.name for group in obj.vertex_groups],
                "components": component_records(obj, head_group_names),
            }
        )

    report["facialRegions"] = facial_region_report(character_objects, dimensions)

    if args.render_dir:
        report["debugRenders"] = render_face_debug(
            character_objects,
            dimensions,
            Path(args.render_dir).resolve(),
        )

    output = Path(args.output).resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps({"output": str(output), "meshCount": len(report["meshes"])}, ensure_ascii=False))


if __name__ == "__main__":
    main()
