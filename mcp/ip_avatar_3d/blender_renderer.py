#!/usr/bin/env python3
"""Blender background renderer for the IP Avatar 3D MCP provider."""

from __future__ import annotations

import json
import heapq
import math
import sys
from array import array
from collections import deque
from pathlib import Path
from typing import Any

import bmesh
import bpy
from bpy_extras.object_utils import world_to_camera_view
from mathutils import Vector
from mathutils.bvhtree import BVHTree

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

from rig_semantics import has_presenter_controls, resolve_bone_roles
from hand_refinement import enhance_three_segment_hands
from master_asset import append_master_collection, save_master_collection
import aroll_actions


CAMERA_NAMES = {
    "wide": "Camera_Wide",
    "medium": "Camera_Medium",
    "close": "Camera_Close",
}
LIGHTING_MULTIPLIERS = {
    "scene_default": {"default": 1.0},
    "editorial_soft": {"default": 1.0},
    "editorial_crisp": {"default": 1.0, "key": 1.15, "fill": 0.90, "rim": 1.10},
    "night_analysis": {"default": 0.82, "key": 0.92, "fill": 0.64, "rim": 1.08, "practical": 1.12},
}
SOLE_BAND_HEIGHT_M = 0.005
TARGET_SOLE_CLEARANCE_M = 0.0015
MIN_MEDIUM_FRAME_POINTS = 128
MEDIUM_FRAME_SAFE_MARGIN = 0.04
TARGET_MEDIUM_VERTICAL_SPAN = 0.84


def read_input() -> dict:
    if "--" not in sys.argv:
        raise RuntimeError("missing render input json after --")
    args = sys.argv[sys.argv.index("--") + 1 :]
    if not args:
        raise RuntimeError("missing render input json")
    with open(args[0], "r", encoding="utf-8") as handle:
        return json.load(handle)


def clear_scene() -> None:
    bpy.ops.object.select_all(action="SELECT")
    bpy.ops.object.delete()


def load_scene_template(path: str) -> None:
    scene_path = Path(path).expanduser().resolve()
    if scene_path.suffix.lower() != ".blend" or not scene_path.is_file():
        raise RuntimeError(f"invalid Blender scene template: {scene_path}")
    bpy.ops.wm.open_mainfile(filepath=str(scene_path), load_ui=False)


def resolve_scene_mode_objects(mode: str) -> dict[str, Any]:
    selected = str(mode or "standing").strip().lower()
    if selected not in {"standing", "seated"}:
        raise ValueError(f"unsupported presentation mode: {mode}")
    prefix = selected.title()
    names = {
        "spawn": f"IP_{prefix}_Spawn",
        "focus": f"IP_{prefix}_Focus_Head",
        "seat": "IP_Seat_Target",
        "foot_l": f"IP_{prefix}_Foot_Target.L",
        "foot_r": f"IP_{prefix}_Foot_Target.R",
    }
    camera_names = {
        "wide": f"Camera_{prefix}_Wide",
        "medium": f"Camera_{prefix}_Medium",
        "three_quarter": f"Camera_{prefix}_ThreeQuarter",
    }
    resolved = {key: bpy.data.objects.get(name) for key, name in names.items()}
    resolved["cameras"] = {
        role: bpy.data.objects.get(name) for role, name in camera_names.items()
    }
    missing = [name for key, name in names.items() if resolved[key] is None]
    missing.extend(
        name for role, name in camera_names.items() if resolved["cameras"][role] is None
    )
    if missing:
        raise RuntimeError(
            f"studio presentation mode {selected} is incomplete: {sorted(missing)}"
        )
    resolved["mode"] = selected
    return resolved


def resolve_authored_scene_mode_objects(data: dict[str, Any]) -> dict[str, Any] | None:
    is_warm_contract = bpy.context.scene.get("ip_presentation_modes") is not None
    has_explicit_mode = "presentationMode" in data
    if not is_warm_contract and not has_explicit_mode:
        return None
    return resolve_scene_mode_objects(str(data.get("presentationMode") or "standing"))


def scene_target_height(data: dict) -> float:
    configured = max(0.25, float(data.get("targetCharacterHeight") or 2.55))
    mode = str(data.get("presentationMode") or "standing").strip().lower()
    mode_spawn = bpy.data.objects.get(f"IP_{mode.title()}_Spawn")
    spawn = mode_spawn or bpy.data.objects.get("IP_Character_Spawn")
    if not spawn:
        return configured
    return max(0.25, float(spawn.get("target_height", configured)))


def apply_lighting_preset(preset: str) -> dict[str, Any]:
    selected = str(preset or "editorial_soft").strip().lower()
    multipliers = LIGHTING_MULTIPLIERS.get(selected, LIGHTING_MULTIPLIERS["editorial_soft"])
    lights = [obj for obj in bpy.context.scene.objects if obj.type == "LIGHT"]
    key_lights = [
        light
        for light in lights
        if str(light.get("ip_light_role", "default")).strip().lower() == "key"
    ]
    fallback_shadow = next((light for light in lights if light.data.type == "AREA"), None)
    shadow_caster = key_lights[0] if key_lights else fallback_shadow
    roles: dict[str, int] = {}
    for light in lights:
        base_energy = float(light.get("ip_base_energy", light.data.energy))
        role = str(light.get("ip_light_role", "default")).strip().lower()
        multiplier = float(multipliers.get(role, multipliers.get("default", 1.0)))
        light["ip_base_energy"] = base_energy
        light.data.energy = base_energy * multiplier
        if hasattr(light.data, "use_shadow"):
            light.data.use_shadow = light == shadow_caster
        roles[role] = roles.get(role, 0) + 1
    return {
        "preset": selected,
        "lightCount": len(lights),
        "roles": roles,
        "shadowCasterCount": 1 if shadow_caster else 0,
        "shadowCasters": [shadow_caster.name] if shadow_caster else [],
    }


def configure_camera_plan(
    data: dict,
    mode_objects: dict[str, Any] | None = None,
) -> dict[str, Any]:
    scene = bpy.context.scene
    scene.timeline_markers.clear()
    requested = data.get("cameraPlan") or [
        {"frame": 1, "camera": CAMERA_NAMES.get(str(data.get("cameraPreset") or "medium"), "Camera_Medium")}
    ]
    if mode_objects is None:
        cameras = {obj.name: obj for obj in scene.objects if obj.type == "CAMERA"}
        role_cameras: dict[str, bpy.types.Object] = {}
    else:
        role_cameras = dict(mode_objects["cameras"])
        cameras = {camera.name: camera for camera in role_cameras.values()}
    fallback = role_cameras.get("medium") or cameras.get("Camera_Medium") or scene.camera
    if fallback is None:
        fallback = next(iter(cameras.values()), None)
    if not fallback:
        raise RuntimeError("studio scene must contain at least one camera")

    aliases = {
        "wide": "wide",
        "medium": "medium",
        "three_quarter": "three_quarter",
        "Camera_Wide": "wide",
        "Camera_Medium": "medium",
        "Camera_ThreeQuarter": "three_quarter",
        "Camera_Close": "three_quarter",
    }
    missing: list[str] = []
    cuts: list[dict[str, Any]] = []
    for item in requested:
        requested_name = str(item.get("camera") or "Camera_Medium")
        camera = role_cameras.get(aliases.get(requested_name, "")) or cameras.get(requested_name)
        if not camera:
            missing.append(requested_name)
            camera = fallback
        frame = max(1, int(item.get("frame") or 1))
        marker = scene.timeline_markers.new(f"IP_Cut_{frame:04d}_{camera.name}", frame=frame)
        marker.camera = camera
        cuts.append({"frame": frame, "camera": camera.name})
    cuts.sort(key=lambda item: item["frame"])
    scene.camera = cameras.get(cuts[0]["camera"], fallback) if cuts else fallback
    return {
        "cuts": cuts,
        "missingCameras": sorted(set(missing)),
        "activeCamera": scene.camera.name,
        "cameraNames": {
            role: camera.name for role, camera in role_cameras.items()
        },
    }


def configure_render_settings(data: dict, *, authored_scene: bool) -> None:
    scene = bpy.context.scene
    scene.frame_start = 1
    scene.frame_end = max(1, int(float(data["durationSec"]) * int(data["fps"])))
    scene.render.fps = int(data["fps"])
    scene.render.resolution_x = int(data["resolution"]["width"])
    scene.render.resolution_y = int(data["resolution"]["height"])
    scene.render.resolution_percentage = 100
    scene.render.film_transparent = False if authored_scene else bool(data.get("transparent") or data.get("backgroundPath"))

    requested_engine = str(data.get("renderEngine") or "BLENDER_EEVEE_NEXT").upper()
    if requested_engine == "CYCLES":
        scene.render.engine = "CYCLES"
        scene.cycles.samples = max(16, int(data.get("cyclesSamples") or 64))
        scene.cycles.use_denoising = True
    else:
        try:
            scene.render.engine = "BLENDER_EEVEE_NEXT"
        except TypeError:
            scene.render.engine = "BLENDER_EEVEE"
        eevee = getattr(scene, "eevee", None)
        requested_samples = max(16, int(data.get("eeveeSamples") or 64))
        if eevee is not None:
            for attribute in ("taa_render_samples", "taa_samples"):
                if hasattr(eevee, attribute):
                    setattr(eevee, attribute, requested_samples)

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
    if authored_scene:
        scene.view_settings.exposure = float(scene.get("ip_authored_exposure", -0.45))
    else:
        scene.view_settings.exposure = -0.35


def material(name: str, color: tuple[float, float, float, float], emission: bool = True) -> bpy.types.Material:
    mat = bpy.data.materials.new(name)
    mat.diffuse_color = color
    mat.use_nodes = True
    bsdf = next((node for node in mat.node_tree.nodes if node.type == "BSDF_PRINCIPLED"), None)
    if bsdf:
        if "Base Color" in bsdf.inputs:
            bsdf.inputs["Base Color"].default_value = color
        if "Alpha" in bsdf.inputs:
            bsdf.inputs["Alpha"].default_value = color[3]
        if emission and "Emission Color" in bsdf.inputs:
            bsdf.inputs["Emission Color"].default_value = color
            bsdf.inputs["Emission Strength"].default_value = 0.7
    try:
        mat.surface_render_method = "DITHERED"
    except Exception:
        try:
            mat.blend_method = "BLEND"
        except Exception:
            pass
    return mat


def imported_objects_before() -> set[str]:
    return {obj.name for obj in bpy.data.objects}


def import_model(
    path: str,
) -> tuple[list[bpy.types.Object], list[bpy.types.Object], list[bpy.types.Object], list[str]]:
    before = imported_objects_before()
    suffix = Path(path).suffix.lower()
    if suffix == ".fbx":
        bpy.ops.import_scene.fbx(
            filepath=path,
            automatic_bone_orientation=False,
            use_prepost_rot=True,
        )
    elif suffix in {".glb", ".gltf"}:
        bpy.ops.import_scene.gltf(filepath=path)
    else:
        raise RuntimeError(f"unsupported character model format: {suffix or '<none>'}")
    imported = [obj for obj in bpy.data.objects if obj.name not in before]
    if not imported:
        raise RuntimeError(f"{suffix.upper().lstrip('.')} import produced no objects")
    mesh_objects = [obj for obj in imported if obj.type == "MESH"]
    armatures = [obj for obj in imported if obj.type == "ARMATURE"]
    if not mesh_objects:
        raise RuntimeError(f"{suffix.upper().lstrip('.')} import produced no mesh objects")

    if armatures:
        def attached_to_input_rig(obj: bpy.types.Object) -> bool:
            if any(modifier.type == "ARMATURE" and modifier.object in armatures for modifier in obj.modifiers):
                return True
            parent = obj.parent
            while parent:
                if parent in armatures:
                    return True
                parent = parent.parent
            return False

        rigged_meshes = [obj for obj in mesh_objects if attached_to_input_rig(obj)]
        largest_vertex_count = max(len(obj.data.vertices) for obj in mesh_objects)
        substantial_unbound = [
            obj
            for obj in mesh_objects
            if len(obj.data.vertices) >= max(64, int(largest_vertex_count * 0.01))
        ]
        character_objects = list(dict.fromkeys(rigged_meshes + substantial_unbound))
    else:
        largest_vertex_count = max(len(obj.data.vertices) for obj in mesh_objects)
        minimum_character_vertices = max(24, int(largest_vertex_count * 0.003))
        character_objects = [
            obj for obj in mesh_objects if len(obj.data.vertices) >= minimum_character_vertices
        ]
    removed_names: list[str] = []
    keep_names = {obj.name for obj in character_objects + armatures}
    if armatures:
        changed = True
        while changed:
            changed = False
            for obj in imported:
                if obj.name in keep_names or obj.type != "EMPTY":
                    continue
                parent_is_kept = bool(obj.parent and obj.parent.name in keep_names)
                child_is_kept = any(child.name in keep_names for child in obj.children)
                if parent_is_kept or child_is_kept:
                    keep_names.add(obj.name)
                    changed = True
    kept_objects: list[bpy.types.Object] = []
    for obj in imported:
        if obj.name in keep_names:
            kept_objects.append(obj)
            continue
        removed_names.append(obj.name)
        bpy.data.objects.remove(obj, do_unlink=True)

    for index, obj in enumerate(character_objects, start=1):
        if not armatures:
            obj.name = "IP_Character_Source" if len(character_objects) == 1 else f"IP_Character_Source.{index:02d}"
        obj["source_materials_preserved"] = True
    return character_objects, armatures, kept_objects, removed_names


def object_bbox(objects: list[bpy.types.Object]) -> tuple[Vector, Vector]:
    points: list[Vector] = []
    for obj in objects:
        if obj.type not in {"MESH", "CURVE", "SURFACE"}:
            continue
        if hasattr(obj, "bound_box") and obj.bound_box:
            for corner in obj.bound_box:
                points.append(obj.matrix_world @ Vector(corner))
    if not points:
        return Vector((-1, -1, 0)), Vector((1, 1, 2))
    min_v = Vector((min(p.x for p in points), min(p.y for p in points), min(p.z for p in points)))
    max_v = Vector((max(p.x for p in points), max(p.y for p in points), max(p.z for p in points)))
    return min_v, max_v


def _apply_object_transform(obj: bpy.types.Object, *, location: bool, rotation: bool, scale: bool) -> None:
    bpy.ops.object.select_all(action="DESELECT")
    obj.hide_viewport = False
    obj.hide_render = False
    obj.select_set(True)
    bpy.context.view_layer.objects.active = obj
    bpy.ops.object.transform_apply(location=location, rotation=rotation, scale=scale)


def prepare_character(
    objects: list[bpy.types.Object],
    target_height: float = 2.55,
    *,
    preserve_hierarchy: bool = False,
    asset_objects: list[bpy.types.Object] | None = None,
) -> dict[str, Any]:
    if preserve_hierarchy:
        assets = asset_objects or list(objects)
        existing_containers = [
            obj
            for obj in assets
            if obj.type == "EMPTY"
            and obj.name.startswith("IP_Character_Container")
            and obj.parent not in assets
        ]
        if existing_containers:
            container = min(
                existing_containers,
                key=lambda obj: (obj.name != "IP_Character_Container", obj.name),
            )
            container.name = "IP_Character_Container"
        else:
            container = bpy.data.objects.new("IP_Character_Container", None)
            bpy.context.collection.objects.link(container)
            roots = [obj for obj in assets if obj.parent not in assets]
            for obj in roots:
                world_matrix = obj.matrix_world.copy()
                obj.parent = container
                obj.matrix_world = world_matrix
        bpy.context.view_layer.update()

        min_v, max_v = object_bbox(objects)
        source_height = max(0.01, max_v.z - min_v.z)
        uniform_scale = target_height / source_height
        container.scale = tuple(float(value) * uniform_scale for value in container.scale)
        bpy.context.view_layer.update()
        min_v, max_v = object_bbox(objects)
        container.location += Vector((-(min_v.x + max_v.x) * 0.5, -(min_v.y + max_v.y) * 0.5, -min_v.z))
        bpy.context.view_layer.update()
        min_v, max_v = object_bbox(objects)
        return {
            "min": min_v,
            "max": max_v,
            "width": max_v.x - min_v.x,
            "depth": max_v.y - min_v.y,
            "height": max_v.z - min_v.z,
            "container": container,
        }

    for obj in objects:
        _apply_object_transform(obj, location=False, rotation=True, scale=True)

    min_v, max_v = object_bbox(objects)
    source_height = max(0.01, max_v.z - min_v.z)
    uniform_scale = target_height / source_height
    for obj in objects:
        obj.scale = (uniform_scale, uniform_scale, uniform_scale)
        _apply_object_transform(obj, location=False, rotation=False, scale=True)

    min_v, max_v = object_bbox(objects)
    offset = Vector((-(min_v.x + max_v.x) * 0.5, -(min_v.y + max_v.y) * 0.5, -min_v.z))
    for obj in objects:
        obj.location += offset
        _apply_object_transform(obj, location=True, rotation=False, scale=False)

    min_v, max_v = object_bbox(objects)
    return {
        "min": min_v,
        "max": max_v,
        "width": max_v.x - min_v.x,
        "depth": max_v.y - min_v.y,
        "height": max_v.z - min_v.z,
    }


def configure_character_render_detail(
    objects: list[bpy.types.Object],
    data: dict[str, Any],
) -> dict[str, Any]:
    """Add non-destructive publish smoothing to substantial source meshes."""
    requested = str(data.get("renderDetailMode") or "auto").strip().lower()
    quality = str(data.get("qualityPreset") or "preview").strip().lower()
    mode = ("publish" if quality in {"production_2k", "master"} else "preview") if requested == "auto" else requested
    if mode in {"off", "none"}:
        return {"mode": "off", "detailedObjects": [], "skippedObjects": []}

    detailed: list[str] = []
    skipped: list[str] = []
    preserve_volume_objects: list[str] = []
    for obj in objects:
        if obj.type != "MESH" or len(obj.data.vertices) < 256 or obj.get("ip_face_topology_role"):
            if obj.type == "MESH":
                skipped.append(obj.name)
            continue
        for polygon in obj.data.polygons:
            polygon.use_smooth = True

        has_deformer = any(modifier.type == "ARMATURE" for modifier in obj.modifiers)
        if has_deformer:
            for modifier in obj.modifiers:
                if modifier.type == "ARMATURE":
                    modifier.use_deform_preserve_volume = True
            preserve_volume_objects.append(obj.name)
            corrective = obj.modifiers.get("IP_Render_CorrectiveSmooth")
            if corrective is None:
                corrective = obj.modifiers.new("IP_Render_CorrectiveSmooth", "CORRECTIVE_SMOOTH")
            corrective.factor = 0.28 if mode == "publish" else 0.18
            corrective.iterations = 3 if mode == "publish" else 2
            if hasattr(corrective, "smooth_type"):
                corrective.smooth_type = "LENGTH_WEIGHTED"
            if hasattr(corrective, "rest_source"):
                corrective.rest_source = "ORCO"

        subdivision = obj.modifiers.get("IP_Render_Subdivision")
        if subdivision is None:
            subdivision = obj.modifiers.new("IP_Render_Subdivision", "SUBSURF")
        # AI-generated topology often has fragile facial UVs. SIMPLE raises
        # deformation sampling density without changing the authored silhouette.
        subdivision.subdivision_type = "SIMPLE"
        subdivision.levels = 1
        subdivision.render_levels = 1 if mode == "publish" else 0
        subdivision.show_render = mode == "publish"
        subdivision.show_viewport = True
        if hasattr(subdivision, "use_limit_surface"):
            subdivision.use_limit_surface = True
        detailed.append(obj.name)

    return {
        "mode": mode,
        "detailedObjects": detailed,
        "skippedObjects": skipped,
        "subdivisionLevel": 1 if detailed else 0,
        "preserveVolumeObjects": preserve_volume_objects,
        "correctiveSmooth": any(
            obj.modifiers.get("IP_Render_CorrectiveSmooth") is not None
            for obj in objects
            if obj.type == "MESH"
        ),
    }


def connected_components(mesh: bpy.types.Mesh) -> list[list[int]]:
    adjacency: list[list[int]] = [[] for _ in mesh.vertices]
    for edge in mesh.edges:
        left, right = edge.vertices
        adjacency[left].append(right)
        adjacency[right].append(left)

    unseen = set(range(len(mesh.vertices)))
    components: list[list[int]] = []
    while unseen:
        first = unseen.pop()
        queue: deque[int] = deque([first])
        component = [first]
        while queue:
            vertex_index = queue.popleft()
            for neighbor in adjacency[vertex_index]:
                if neighbor not in unseen:
                    continue
                unseen.remove(neighbor)
                queue.append(neighbor)
                component.append(neighbor)
        components.append(component)
    return components


def classify_component(obj: bpy.types.Object, vertex_indices: list[int], dimensions: dict[str, Any]) -> str:
    points = [obj.data.vertices[index].co for index in vertex_indices]
    min_x = min(point.x for point in points)
    max_x = max(point.x for point in points)
    min_z = min(point.z for point in points)
    max_z = max(point.z for point in points)
    center_x = (min_x + max_x) * 0.5
    center_z = (min_z + max_z) * 0.5
    width = max(float(dimensions["width"]), 0.01)
    height = max(float(dimensions["height"]), 0.01)

    is_arm = abs(center_x) > width * 0.13 and height * 0.56 < center_z < height * 0.78
    if is_arm:
        return "Arm.L" if center_x > 0 else "Arm.R"
    is_leg = max_z < height * 0.27 and abs(center_x) > width * 0.05
    if is_leg:
        return "Leg.L" if center_x > 0 else "Leg.R"
    if min_z > height * 0.72:
        return "Head"
    if min_z > height * 0.56:
        return "Chest"
    return "Body"


def subdivide_arm_shafts(obj: bpy.types.Object, dimensions: dict[str, Any]) -> dict[str, int]:
    """Add deformation rings only to the long disconnected arm shafts."""
    width = max(float(dimensions["width"]), 0.01)
    shaft_vertices: set[int] = set()
    for component in connected_components(obj.data):
        if not classify_component(obj, component, dimensions).startswith("Arm."):
            continue
        xs = [obj.data.vertices[index].co.x for index in component]
        if max(xs) - min(xs) > width * 0.16:
            shaft_vertices.update(component)
    if not shaft_vertices:
        return {"subdividedEdges": 0, "addedVertices": 0}

    mesh = obj.data
    bm = bmesh.new()
    bm.from_mesh(mesh)
    bm.verts.ensure_lookup_table()
    bm.verts.index_update()
    long_edges = [
        edge
        for edge in bm.edges
        if edge.verts[0].index in shaft_vertices
        and edge.verts[1].index in shaft_vertices
        and edge.calc_length() > width * 0.095
    ]
    before_vertices = len(bm.verts)
    if long_edges:
        bmesh.ops.subdivide_edges(bm, edges=long_edges, cuts=2, use_grid_fill=False)
    bm.to_mesh(mesh)
    added_vertices = len(bm.verts) - before_vertices
    bm.free()
    mesh.update()
    return {"subdividedEdges": len(long_edges), "addedVertices": added_vertices}


def create_simple_rig(
    objects: list[bpy.types.Object], dimensions: dict[str, Any]
) -> tuple[bpy.types.Object, dict[str, Any], dict[str, str]]:
    """Create a compact humanoid presenter rig for a static T-pose character."""
    width = float(dimensions["width"])
    depth = float(dimensions["depth"])
    height = float(dimensions["height"])
    subdivision_stats = {"subdividedEdges": 0, "addedVertices": 0}
    for obj in objects:
        stats = subdivide_arm_shafts(obj, dimensions)
        for key, value in stats.items():
            subdivision_stats[key] += value

    bpy.ops.object.select_all(action="DESELECT")
    bpy.ops.object.armature_add(enter_editmode=True, location=(0, 0, 0))
    armature = bpy.context.object
    armature.name = "IP_Simple_Cartoon_Rig"
    armature.data.name = "IP_Simple_Cartoon_Rig_Data"
    armature.data.display_type = "BBONE"
    armature.show_in_front = True
    edit_bones = armature.data.edit_bones
    edit_bones.remove(edit_bones[0])

    def add_bone(
        name: str,
        head,
        tail,
        parent: str | None = None,
        deform: bool = True,
        connected: bool = False,
    ) -> None:
        bone = edit_bones.new(name)
        bone.head = head
        bone.tail = tail
        bone.roll = 0.0
        bone.use_deform = deform
        if parent:
            bone.parent = edit_bones[parent]
            bone.use_connect = connected

    hips_z = height * 0.36
    spine_z = height * 0.46
    chest_z = height * 0.58
    shoulder_z = height * 0.68
    neck_z = height * 0.73
    head_top_z = height * 0.96
    shoulder_x = width * 0.14
    elbow_x = width * 0.29
    wrist_x = width * 0.43
    hand_x = width * 0.50
    finger_base_x = width * 0.445
    finger_tip_x = width * 0.495
    hip_x = width * 0.075
    knee_z = height * 0.22
    ankle_z = height * 0.07
    mouth_z = height * 0.805

    add_bone("Root", (0, 0, 0), (0, 0, height * 0.08), deform=False)
    add_bone("Body", (0, 0, hips_z), (0, 0, spine_z), "Root")
    add_bone("Spine", (0, 0, spine_z), (0, 0, chest_z), "Body", connected=True)
    add_bone("Chest", (0, 0, chest_z), (0, 0, shoulder_z), "Spine", connected=True)
    add_bone("Neck", (0, 0, shoulder_z), (0, 0, neck_z), "Chest", connected=True)
    add_bone("Head", (0, 0, neck_z), (0, 0, head_top_z), "Neck", connected=True)
    add_bone(
        "Jaw",
        (0, 0, mouth_z + height * 0.018),
        (0, -depth * 0.28, mouth_z - height * 0.026),
        "Head",
    )
    for suffix, side in (("L", 1.0), ("R", -1.0)):
        shoulder = (side * shoulder_x, 0, shoulder_z)
        elbow = (side * elbow_x, 0, shoulder_z)
        wrist = (side * wrist_x, 0, shoulder_z)
        hand_tip = (side * hand_x, 0, shoulder_z)
        add_bone(f"Shoulder.{suffix}", (0, 0, shoulder_z), shoulder, "Chest")
        add_bone(f"UpperArm.{suffix}", shoulder, elbow, f"Shoulder.{suffix}", connected=True)
        add_bone(f"ForeArm.{suffix}", elbow, wrist, f"UpperArm.{suffix}", connected=True)
        add_bone(f"Hand.{suffix}", wrist, hand_tip, f"ForeArm.{suffix}", connected=True)
        for finger_index, z_offset in ((1, height * 0.026), (2, 0.0), (3, -height * 0.026)):
            add_bone(
                f"Finger_{finger_index:02d}.{suffix}",
                (side * finger_base_x, 0, shoulder_z + z_offset),
                (side * finger_tip_x, 0, shoulder_z + z_offset),
                f"Hand.{suffix}",
            )
        add_bone(f"Thigh.{suffix}", (side * hip_x, 0, hips_z), (side * hip_x, 0, knee_z), "Body")
        add_bone(f"Shin.{suffix}", (side * hip_x, 0, knee_z), (side * hip_x, 0, ankle_z), f"Thigh.{suffix}", connected=True)
        add_bone(
            f"Foot.{suffix}",
            (side * hip_x, 0, ankle_z),
            (side * hip_x, -max(depth * 0.46, width * 0.055), ankle_z * 0.72),
            f"Shin.{suffix}",
            connected=True,
        )

        add_bone(f"CTRL_Hand.{suffix}", wrist, hand_tip, "Root", deform=False)
        add_bone(
            f"CTRL_Elbow.{suffix}",
            (side * elbow_x, -depth * 0.85, shoulder_z),
            (side * elbow_x, -depth * 0.85, shoulder_z + height * 0.07),
            "Root",
            deform=False,
        )
        add_bone(
            f"CTRL_Foot.{suffix}",
            (side * hip_x, 0, ankle_z),
            (side * hip_x, -max(depth * 0.46, width * 0.055), ankle_z * 0.72),
            "Root",
            deform=False,
        )
        add_bone(
            f"CTRL_Knee.{suffix}",
            (side * hip_x, -depth * 0.85, knee_z),
            (side * hip_x, -depth * 0.85, knee_z + height * 0.07),
            "Root",
            deform=False,
        )

    add_bone("CTRL_Root", (0, depth * 0.65, 0), (0, depth * 0.65, height * 0.12), deform=False)
    add_bone("CTRL_Body", (0, depth * 0.55, spine_z), (0, depth * 0.55, chest_z), deform=False)
    add_bone("CTRL_Head", (0, depth * 0.48, neck_z), (0, depth * 0.48, head_top_z), deform=False)
    add_bone("CTRL_LookAt", (0, -depth * 2.6, neck_z), (0, -depth * 2.6, neck_z + height * 0.08), deform=False)
    add_bone("CTRL_Mouth", (0, -depth * 1.4, height * 0.755), (0, -depth * 1.4, height * 0.81), deform=False)
    bpy.ops.object.mode_set(mode="POSE")
    for pose_bone in armature.pose.bones:
        pose_bone.rotation_mode = "XYZ"

    def drive_constraint(constraint: bpy.types.Constraint, control_name: str) -> None:
        control = armature.pose.bones[control_name]
        control["ik_fk"] = 0.0
        try:
            control.id_properties_ui("ik_fk").update(min=0.0, max=1.0, description="0 = FK, 1 = IK")
        except Exception:
            pass
        driver = constraint.driver_add("influence").driver
        variable = driver.variables.new()
        variable.name = "ik_fk"
        variable.type = "SINGLE_PROP"
        variable.targets[0].id = armature
        variable.targets[0].data_path = f'pose.bones["{control_name}"]["ik_fk"]'
        driver.expression = "ik_fk"

    for suffix in ("L", "R"):
        hand_ik = armature.pose.bones[f"ForeArm.{suffix}"].constraints.new("IK")
        hand_ik.name = "IP_Hand_IK"
        hand_ik.target = armature
        hand_ik.subtarget = f"CTRL_Hand.{suffix}"
        hand_ik.pole_target = armature
        hand_ik.pole_subtarget = f"CTRL_Elbow.{suffix}"
        hand_ik.chain_count = 2
        hand_ik.influence = 0.0
        drive_constraint(hand_ik, f"CTRL_Hand.{suffix}")

        foot_ik = armature.pose.bones[f"Shin.{suffix}"].constraints.new("IK")
        foot_ik.name = "IP_Foot_IK"
        foot_ik.target = armature
        foot_ik.subtarget = f"CTRL_Foot.{suffix}"
        foot_ik.pole_target = armature
        foot_ik.pole_subtarget = f"CTRL_Knee.{suffix}"
        foot_ik.chain_count = 2
        foot_ik.influence = 0.0
        drive_constraint(foot_ik, f"CTRL_Foot.{suffix}")
    bpy.ops.object.mode_set(mode="OBJECT")

    deform_bones = (
        "Body",
        "Spine",
        "Chest",
        "Neck",
        "Head",
        "Jaw",
        "Shoulder.L",
        "UpperArm.L",
        "ForeArm.L",
        "Hand.L",
        "Finger_01.L",
        "Finger_02.L",
        "Finger_03.L",
        "Shoulder.R",
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
    )
    totals = {name: 0 for name in deform_bones}
    vertex_count = 0
    arm_z_min = height * 0.55
    arm_z_max = height * 0.78
    elbow_blend = width * 0.025
    wrist_blend = width * 0.020
    knee_blend = height * 0.035
    ankle_blend = height * 0.022
    finger_start = wrist_x + width * 0.012
    finger_blend_length = max(hand_x - finger_start, width * 0.02)
    finger_band = height * 0.014
    front_y = float(dimensions["min"].y)

    def add_weight(weights: dict[str, float], name: str, value: float) -> None:
        if value > 1e-6:
            weights[name] = weights.get(name, 0.0) + value

    def spatial_weights(co: Vector) -> dict[str, float]:
        x, y, z = float(co.x), float(co.y), float(co.z)
        abs_x = abs(x)
        suffix = "L" if x >= 0 else "R"
        weights: dict[str, float] = {}

        if arm_z_min <= z <= arm_z_max and abs_x >= shoulder_x * 0.82:
            if abs_x < shoulder_x + width * 0.035:
                t = max(0.0, min(1.0, (abs_x - shoulder_x * 0.82) / max(width * 0.065, 1e-6)))
                add_weight(weights, f"Shoulder.{suffix}", 1.0 - t * 0.65)
                add_weight(weights, f"UpperArm.{suffix}", t * 0.65)
            elif abs_x < elbow_x - elbow_blend:
                add_weight(weights, f"UpperArm.{suffix}", 1.0)
            elif abs_x <= elbow_x + elbow_blend:
                t = (abs_x - (elbow_x - elbow_blend)) / max(elbow_blend * 2.0, 1e-6)
                add_weight(weights, f"UpperArm.{suffix}", 1.0 - t)
                add_weight(weights, f"ForeArm.{suffix}", t)
            elif abs_x < wrist_x - wrist_blend:
                add_weight(weights, f"ForeArm.{suffix}", 1.0)
            elif abs_x <= wrist_x + wrist_blend:
                t = (abs_x - (wrist_x - wrist_blend)) / max(wrist_blend * 2.0, 1e-6)
                add_weight(weights, f"ForeArm.{suffix}", 1.0 - t)
                add_weight(weights, f"Hand.{suffix}", t)
            elif abs_x >= finger_start:
                finger_t = max(0.0, min(1.0, (abs_x - finger_start) / finger_blend_length))
                z_offset = z - shoulder_z
                if z_offset > finger_band:
                    finger_index = 1
                elif z_offset < -finger_band:
                    finger_index = 3
                else:
                    finger_index = 2
                add_weight(weights, f"Hand.{suffix}", 1.0 - finger_t)
                add_weight(weights, f"Finger_{finger_index:02d}.{suffix}", finger_t)
            else:
                add_weight(weights, f"Hand.{suffix}", 1.0)
            return weights

        if z < hips_z + height * 0.08 and abs_x > width * 0.025:
            if z <= ankle_z - ankle_blend:
                add_weight(weights, f"Foot.{suffix}", 1.0)
            elif z <= ankle_z + ankle_blend:
                t = (z - (ankle_z - ankle_blend)) / max(ankle_blend * 2.0, 1e-6)
                add_weight(weights, f"Foot.{suffix}", 1.0 - t)
                add_weight(weights, f"Shin.{suffix}", t)
            elif z < knee_z - knee_blend:
                add_weight(weights, f"Shin.{suffix}", 1.0)
            elif z <= knee_z + knee_blend:
                t = (z - (knee_z - knee_blend)) / max(knee_blend * 2.0, 1e-6)
                add_weight(weights, f"Shin.{suffix}", 1.0 - t)
                add_weight(weights, f"Thigh.{suffix}", t)
            else:
                add_weight(weights, f"Thigh.{suffix}", 1.0)
            return weights

        if z >= neck_z:
            mouth_x_radius = width * 0.085
            mouth_z_radius = height * 0.050
            mouth_x = x / max(mouth_x_radius, 1e-6)
            mouth_vertical = (z - mouth_z) / max(mouth_z_radius, 1e-6)
            front_amount = max(0.0, min(1.0, (front_y + depth * 0.24 - y) / max(depth * 0.24, 1e-6)))
            jaw_mask = max(0.0, 1.0 - mouth_x * mouth_x - mouth_vertical * mouth_vertical) * front_amount
            if jaw_mask > 0.0 and z <= mouth_z + height * 0.018:
                jaw_weight = min(0.78, jaw_mask * 0.78)
                add_weight(weights, "Head", 1.0 - jaw_weight)
                add_weight(weights, "Jaw", jaw_weight)
            else:
                add_weight(weights, "Head", 1.0)
        elif z >= shoulder_z:
            t = (z - shoulder_z) / max(neck_z - shoulder_z, 1e-6)
            add_weight(weights, "Neck", 0.55 + t * 0.45)
            add_weight(weights, "Chest", 0.45 * (1.0 - t))
        elif z >= chest_z:
            t = (z - chest_z) / max(shoulder_z - chest_z, 1e-6)
            add_weight(weights, "Chest", 0.72 + t * 0.28)
            add_weight(weights, "Spine", 0.28 * (1.0 - t))
        elif z >= spine_z:
            t = (z - spine_z) / max(chest_z - spine_z, 1e-6)
            add_weight(weights, "Spine", 0.68 + (1.0 - abs(t - 0.5) * 2.0) * 0.16)
            add_weight(weights, "Chest", max(0.0, t - 0.45) * 0.58)
            add_weight(weights, "Body", max(0.0, 0.55 - t) * 0.58)
        else:
            t = max(0.0, min(1.0, (z - hips_z) / max(spine_z - hips_z, 1e-6)))
            add_weight(weights, "Body", 1.0 - t * 0.45)
            add_weight(weights, "Spine", t * 0.45)
        total = sum(weights.values()) or 1.0
        return {name: value / total for name, value in weights.items()}

    for obj in objects:
        obj.vertex_groups.clear()
        groups = {name: obj.vertex_groups.new(name=name) for name in totals}
        for vertex in obj.data.vertices:
            weights = spatial_weights(vertex.co)
            for bone_name, value in weights.items():
                groups[bone_name].add([vertex.index], value, "REPLACE")
                totals[bone_name] += 1
        modifier = obj.modifiers.new("IP_Simple_Cartoon_Rig_Deform", "ARMATURE")
        modifier.object = armature
        modifier.use_vertex_groups = True
        obj.parent = armature
        obj.matrix_parent_inverse = armature.matrix_world.inverted()
        vertex_count += len(obj.data.vertices)

    influence_counts = [
        len(vertex.groups)
        for obj in objects
        for vertex in obj.data.vertices
    ]
    rig_stats: dict[str, Any] = {
        "weightedVertexCounts": totals,
        "vertexCount": vertex_count,
        "maxVertexInfluences": max(influence_counts, default=0),
        "unweightedVertexCount": sum(1 for count in influence_counts if count == 0),
        "elbowRig": True,
        **subdivision_stats,
    }
    bone_map = {
        "root": "Root",
        "body": "Body",
        "spine": "Spine",
        "chest": "Chest",
        "neck": "Neck",
        "head": "Head",
        "jaw": "Jaw",
        "shoulder_l": "Shoulder.L",
        "upper_arm_l": "UpperArm.L",
        "forearm_l": "ForeArm.L",
        "hand_l": "Hand.L",
        "finger_1_l": "Finger_01.L",
        "finger_2_l": "Finger_02.L",
        "finger_3_l": "Finger_03.L",
        "shoulder_r": "Shoulder.R",
        "upper_arm_r": "UpperArm.R",
        "forearm_r": "ForeArm.R",
        "hand_r": "Hand.R",
        "finger_1_r": "Finger_01.R",
        "finger_2_r": "Finger_02.R",
        "finger_3_r": "Finger_03.R",
        "leg_l": "Thigh.L",
        "shin_l": "Shin.L",
        "foot_l": "Foot.L",
        "leg_r": "Thigh.R",
        "shin_r": "Shin.R",
        "foot_r": "Foot.R",
    }
    rig_stats.update(
        {
            "resolvedRigMode": "humanoid_presenter_generated",
            "inputRigPreserved": False,
            "boneMap": bone_map,
            "presenterControlsReady": True,
            "fingerRig": True,
            "jawRig": True,
            "ikControls": True,
        }
    )
    armature["ip_avatar_bone_map"] = json.dumps(bone_map)
    armature["ip_avatar_generated_humanoid"] = True
    return armature, rig_stats, bone_map


def preserve_existing_rig(
    armatures: list[bpy.types.Object],
    objects: list[bpy.types.Object],
) -> tuple[bpy.types.Object, dict[str, Any], dict[str, str]]:
    armature = max(armatures, key=lambda item: len(item.data.bones))
    armature.show_in_front = True
    for pose_bone in armature.pose.bones:
        pose_bone.rotation_mode = "XYZ"
    bone_map = resolve_bone_roles(bone.name for bone in armature.data.bones)
    influence_counts = [len(vertex.groups) for obj in objects for vertex in obj.data.vertices]
    group_totals: dict[str, int] = {}
    for obj in objects:
        index_to_name = {group.index: group.name for group in obj.vertex_groups}
        for vertex in obj.data.vertices:
            for assignment in vertex.groups:
                name = index_to_name.get(assignment.group)
                if name:
                    group_totals[name] = group_totals.get(name, 0) + 1
    stats: dict[str, Any] = {
        "resolvedRigMode": "existing_humanoid_preserved",
        "inputRigPreserved": True,
        "sourceArmature": armature.name,
        "sourceBoneCount": len(armature.data.bones),
        "boneMap": bone_map,
        "presenterControlsReady": has_presenter_controls(bone_map),
        "weightedVertexCounts": group_totals,
        "vertexCount": sum(len(obj.data.vertices) for obj in objects),
        "maxVertexInfluences": max(influence_counts, default=0),
        "unweightedVertexCount": sum(1 for count in influence_counts if count == 0),
        "elbowRig": "forearm_l" in bone_map or "forearm_r" in bone_map,
        "subdividedEdges": 0,
        "addedVertices": 0,
    }
    armature["ip_avatar_bone_map"] = json.dumps(bone_map)
    armature["ip_avatar_preserved_source_rig"] = True
    return armature, stats, bone_map


def _collect_weight_stats(objects: list[bpy.types.Object]) -> dict[str, Any]:
    group_totals: dict[str, int] = {}
    influence_counts: list[int] = []
    unweighted = 0
    for obj in objects:
        index_to_name = {group.index: group.name for group in obj.vertex_groups}
        for vertex in obj.data.vertices:
            assignments = [assignment for assignment in vertex.groups if assignment.weight > 1e-6]
            influence_counts.append(len(assignments))
            if not assignments:
                unweighted += 1
            for assignment in assignments:
                name = index_to_name.get(assignment.group)
                if name:
                    group_totals[name] = group_totals.get(name, 0) + 1
    return {
        "weightedVertexCounts": group_totals,
        "vertexCount": sum(len(obj.data.vertices) for obj in objects),
        "maxVertexInfluences": max(influence_counts, default=0),
        "unweightedVertexCount": unweighted,
    }


def _limit_and_normalize_weights(obj: bpy.types.Object, maximum: int = 4) -> None:
    for vertex in obj.data.vertices:
        weighted = sorted(
            (
                (assignment.group, float(assignment.weight))
                for assignment in vertex.groups
                if assignment.weight > 1e-8
            ),
            key=lambda item: item[1],
            reverse=True,
        )
        keep = weighted[:maximum]
        keep_indices = {group_index for group_index, _ in keep}
        for group_index, _ in weighted[maximum:]:
            obj.vertex_groups[group_index].remove([vertex.index])
        total = sum(weight for _, weight in keep)
        if total <= 1e-8:
            continue
        for group_index, weight in keep:
            obj.vertex_groups[group_index].add([vertex.index], weight / total, "REPLACE")
        for assignment in list(vertex.groups):
            if assignment.group not in keep_indices and assignment.weight <= 1e-8:
                obj.vertex_groups[assignment.group].remove([vertex.index])


def enhance_existing_presenter_rig(
    armature: bpy.types.Object,
    objects: list[bpy.types.Object],
    dimensions: dict[str, Any],
    stats: dict[str, Any],
    bone_map: dict[str, str],
) -> tuple[dict[str, Any], dict[str, str]]:
    """Add articulated three-digit hands without replacing source skeleton or skinning."""
    return enhance_three_segment_hands(
        armature=armature,
        objects=objects,
        dimensions=dimensions,
        stats=stats,
        resolve_roles=resolve_bone_roles,
        maximum_influences=4,
    )


def enhance_existing_facial_rig(
    armature: bpy.types.Object,
    objects: list[bpy.types.Object],
    dimensions: dict[str, Any],
    stats: dict[str, Any],
    bone_map: dict[str, str],
) -> tuple[dict[str, Any], dict[str, str]]:
    """Add compact jaw, eye, and tongue controls while retaining source skinning."""
    if "head" not in bone_map:
        stats["facialRigEnhanced"] = False
        stats["facialEnhancementReason"] = "source rig has no resolved head bone"
        return stats, bone_map

    head_bounds = weighted_region_bounds(objects, "head", minimum_weight=0.25)
    if not head_bounds:
        stats["facialRigEnhanced"] = False
        stats["facialEnhancementReason"] = "source mesh has no stable Head-weighted region"
        return stats, bone_map

    required = {
        "jaw": "Jaw",
        "eye_l": "Eye.L",
        "eye_r": "Eye.R",
        "tongue_1": "Tongue_01",
        "tongue_2": "Tongue_02",
        "tongue_3": "Tongue_03",
    }
    existing_roles = resolve_bone_roles(bone.name for bone in armature.data.bones)
    missing_roles = [role for role in required if role not in existing_roles]
    existing_weight_stats = _collect_weight_stats(objects)
    if not missing_roles and all(
        existing_weight_stats["weightedVertexCounts"].get(existing_roles[role], 0) > 0
        for role in ("jaw", "eye_l", "eye_r")
    ):
        stats.update(existing_weight_stats)
        stats.update(
            {
                "boneMap": existing_roles,
                "facialRigEnhanced": False,
                "facialRigReused": True,
                "facialEnhancementReason": "source rig already contains weighted jaw, eye, and tongue controls",
                "jawRig": True,
                "eyeRig": True,
                "tongueRig": True,
            }
        )
        armature["ip_avatar_bone_map"] = json.dumps(existing_roles)
        return stats, existing_roles
    min_v = head_bounds["min"]
    max_v = head_bounds["max"]
    width = max(float(head_bounds["width"]), 0.01)
    depth = max(float(head_bounds["depth"]), 0.01)
    region_height = max(float(head_bounds["height"]), 0.01)
    center_x = (float(min_v.x) + float(max_v.x)) * 0.5
    center_y = (float(min_v.y) + float(max_v.y)) * 0.5
    mouth_z = float(min_v.z) + region_height * 0.22
    eye_z = float(min_v.z) + region_height * 0.46
    eye_offset = width * 0.185
    mouth_front_y = float(min_v.y) + depth * 0.10

    if missing_roles:
        bpy.ops.object.select_all(action="DESELECT")
        armature.hide_viewport = False
        armature.select_set(True)
        bpy.context.view_layer.objects.active = armature
        bpy.ops.object.mode_set(mode="EDIT")
        world_to_armature = armature.matrix_world.inverted()
        try:
            head_parent = armature.data.edit_bones[bone_map["head"]]
            if "jaw" in missing_roles:
                jaw = armature.data.edit_bones.new(required["jaw"])
                jaw.head = world_to_armature @ Vector((center_x, center_y, mouth_z + region_height * 0.105))
                jaw.tail = world_to_armature @ Vector((center_x, center_y, mouth_z - region_height * 0.105))
                jaw.parent = head_parent
                jaw.use_connect = False
                jaw.use_deform = True
            for role, sign in (("eye_l", 1.0), ("eye_r", -1.0)):
                if role not in missing_roles:
                    continue
                eye = armature.data.edit_bones.new(required[role])
                eye.head = world_to_armature @ Vector((center_x + sign * eye_offset, mouth_front_y + depth * 0.11, eye_z))
                eye.tail = world_to_armature @ Vector((center_x + sign * eye_offset, mouth_front_y - depth * 0.05, eye_z))
                eye.parent = head_parent
                eye.use_connect = False
                eye.use_deform = True

            jaw_parent = armature.data.edit_bones.get(required["jaw"]) or head_parent
            tongue_start = Vector((center_x, mouth_front_y + depth * 0.08, mouth_z - region_height * 0.015))
            tongue_step = Vector((0.0, -depth * 0.035, region_height * 0.004))
            parent = jaw_parent
            for index, role in enumerate(("tongue_1", "tongue_2", "tongue_3")):
                if role in existing_roles:
                    parent = armature.data.edit_bones[existing_roles[role]]
                    continue
                bone = armature.data.edit_bones.new(required[role])
                bone.head = world_to_armature @ (tongue_start + tongue_step * index)
                bone.tail = world_to_armature @ (tongue_start + tongue_step * (index + 1))
                bone.parent = parent
                bone.use_connect = index > 0
                bone.use_deform = True
                parent = bone
        finally:
            bpy.ops.object.mode_set(mode="OBJECT")

    bone_map = resolve_bone_roles(bone.name for bone in armature.data.bones)
    head_name = bone_map["head"]
    for obj in objects:
        if obj.type != "MESH":
            continue
        head_group = obj.vertex_groups.get(head_name)
        if not head_group:
            continue
        deform_groups = {
            role: obj.vertex_groups.get(bone_map[role]) or obj.vertex_groups.new(name=bone_map[role])
            for role in ("jaw", "eye_l", "eye_r")
            if role in bone_map
        }
        for vertex in obj.data.vertices:
            head_assignment = next(
                (assignment for assignment in vertex.groups if assignment.group == head_group.index),
                None,
            )
            if not head_assignment or head_assignment.weight <= 0.02:
                continue
            head_weight = float(head_assignment.weight)
            world = obj.matrix_world @ vertex.co
            assigned: dict[str, float] = {}

            if float(world.z) <= mouth_z + region_height * 0.075:
                vertical = max(
                    0.0,
                    min(1.0, (mouth_z + region_height * 0.075 - float(world.z)) / (region_height * 0.31)),
                )
                horizontal = max(0.0, 1.0 - (abs(float(world.x) - center_x) / (width * 0.62)) ** 4)
                frontness = max(
                    0.0,
                    min(1.0, (float(min_v.y) + depth * 0.70 - float(world.y)) / (depth * 0.52)),
                )
                jaw_weight = head_weight * 0.82 * vertical * vertical * horizontal * frontness
                if jaw_weight > 0.01:
                    assigned["jaw"] = jaw_weight

            for role, sign in (("eye_l", 1.0), ("eye_r", -1.0)):
                eye_x = center_x + sign * eye_offset
                nx = (float(world.x) - eye_x) / (width * 0.105)
                nz = (float(world.z) - eye_z) / (region_height * 0.125)
                radial = max(0.0, 1.0 - nx * nx - nz * nz)
                frontness = max(
                    0.0,
                    min(1.0, (float(min_v.y) + depth * 0.29 - float(world.y)) / (depth * 0.22)),
                )
                eye_weight = head_weight * 0.94 * radial * radial * frontness
                if eye_weight > 0.015:
                    assigned[role] = eye_weight

            total = min(head_weight * 0.96, sum(assigned.values()))
            if total <= 0.0:
                continue
            if sum(assigned.values()) > total:
                scale = total / sum(assigned.values())
                assigned = {role: value * scale for role, value in assigned.items()}
            head_group.add([vertex.index], max(0.0, head_weight - total), "REPLACE")
            for role, value in assigned.items():
                deform_groups[role].add([vertex.index], value, "REPLACE")
        _limit_and_normalize_weights(obj, maximum=4)

    weight_stats = _collect_weight_stats(objects)
    stats.update(weight_stats)
    stats.update(
        {
            "boneMap": bone_map,
            "facialRigEnhanced": True,
            "addedFacialBones": [required[role] for role in missing_roles],
            "jawRig": "jaw" in bone_map,
            "eyeRig": all(role in bone_map for role in ("eye_l", "eye_r")),
            "tongueRig": all(role in bone_map for role in ("tongue_1", "tongue_2", "tongue_3")),
        }
    )
    armature["ip_avatar_bone_map"] = json.dumps(bone_map)
    armature["ip_avatar_facial_rig_enhanced"] = True
    return stats, bone_map


def choose_character_rig(
    data: dict,
    armatures: list[bpy.types.Object],
    objects: list[bpy.types.Object],
    dimensions: dict[str, Any],
) -> tuple[bpy.types.Object, dict[str, Any], dict[str, str]]:
    mode = str(data.get("rigMode") or "auto").lower()
    preserve = bool(data.get("preserveExistingRig", True))
    if armatures and preserve and mode in {"auto", "existing", "preserve", "humanoid"}:
        armature, stats, bone_map = preserve_existing_rig(armatures, objects)
        if bool(data.get("enhanceExistingRig", True)):
            stats, bone_map = enhance_existing_presenter_rig(
                armature,
                objects,
                dimensions,
                stats,
                bone_map,
            )
            stats, bone_map = enhance_existing_facial_rig(
                armature,
                objects,
                dimensions,
                stats,
                bone_map,
            )
        return armature, stats, bone_map
    if mode in {"existing", "preserve", "humanoid"} and not armatures:
        raise RuntimeError("rigMode requires an input Armature, but the GLB contains none")
    if armatures:
        raise RuntimeError("Replacing an existing Armature is disabled; use rigMode=auto to preserve it")
    return create_simple_rig(objects, dimensions)


def look_at(obj: bpy.types.Object, target: tuple[float, float, float]) -> None:
    direction = Vector(target) - obj.location
    obj.rotation_euler = direction.to_track_quat("-Z", "Y").to_euler()


def _sample_region(image: bpy.types.Image, start_ratio: float, end_ratio: float) -> tuple[float, float, float]:
    width, height = int(image.size[0]), int(image.size[1])
    pixels = image.pixels
    step_x = max(1, width // 30)
    step_y = max(1, height // 18)
    sums = [0.0, 0.0, 0.0]
    count = 0
    for y in range(int(height * 0.18), int(height * 0.80), step_y):
        for x in range(int(width * start_ratio), int(width * end_ratio), step_x):
            offset = (y * width + x) * 4
            sums[0] += float(pixels[offset])
            sums[1] += float(pixels[offset + 1])
            sums[2] += float(pixels[offset + 2])
            count += 1
    if count == 0:
        return (1.0, 1.0, 1.0)
    return tuple(value / count for value in sums)


def _soft_light_color(color: tuple[float, float, float]) -> tuple[float, float, float]:
    maximum = max(max(color), 0.001)
    normalized = tuple(value / maximum for value in color)
    return tuple(min(1.0, 0.58 + value * 0.42) for value in normalized)


def background_light_colors(path: str) -> dict[str, tuple[float, float, float]]:
    defaults = {
        "left": (0.56, 0.78, 1.0),
        "center": (1.0, 0.96, 0.91),
        "right": (1.0, 0.72, 0.46),
    }
    if not path or not Path(path).is_file():
        return defaults
    image = bpy.data.images.load(path, check_existing=False)
    try:
        return {
            "left": _soft_light_color(_sample_region(image, 0.03, 0.30)),
            "center": _soft_light_color(_sample_region(image, 0.38, 0.62)),
            "right": _soft_light_color(_sample_region(image, 0.70, 0.97)),
        }
    finally:
        bpy.data.images.remove(image)


def create_contact_shadow(dimensions: dict[str, Any]) -> None:
    mat = bpy.data.materials.new("IP_Contact_Shadow_Material")
    mat.use_nodes = True
    nodes = mat.node_tree.nodes
    nodes.clear()
    output = nodes.new("ShaderNodeOutputMaterial")
    transparent = nodes.new("ShaderNodeBsdfTransparent")
    diffuse = nodes.new("ShaderNodeBsdfPrincipled")
    diffuse.inputs["Base Color"].default_value = (0.003, 0.004, 0.006, 1.0)
    diffuse.inputs["Roughness"].default_value = 1.0
    mix = nodes.new("ShaderNodeMixShader")
    mix.inputs[0].default_value = 0.11
    mat.node_tree.links.new(transparent.outputs[0], mix.inputs[1])
    mat.node_tree.links.new(diffuse.outputs[0], mix.inputs[2])
    mat.node_tree.links.new(mix.outputs[0], output.inputs[0])
    try:
        mat.surface_render_method = "DITHERED"
    except Exception:
        pass

    bpy.ops.mesh.primitive_uv_sphere_add(segments=64, ring_count=24, location=(0, 0.05, 0.012))
    shadow = bpy.context.object
    shadow.name = "IP_Contact_Shadow"
    shadow.dimensions = (
        float(dimensions["width"]) * 0.42,
        max(float(dimensions["depth"]) * 0.65, 0.22),
        float(dimensions["height"]) * 0.014,
    )
    bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)
    shadow.data.materials.append(mat)


def setup_scene(data: dict, dimensions: dict[str, Any]) -> None:
    scene = bpy.context.scene
    configure_render_settings(data, authored_scene=False)
    if hasattr(scene, "eevee"):
        if hasattr(scene.eevee, "taa_render_samples"):
            scene.eevee.taa_render_samples = max(16, int(data.get("eeveeSamples") or 64))
        if hasattr(scene.eevee, "use_gtao"):
            scene.eevee.use_gtao = True
        if hasattr(scene.eevee, "gtao_distance"):
            scene.eevee.gtao_distance = 0.22
        if hasattr(scene.eevee, "gtao_factor"):
            scene.eevee.gtao_factor = 0.65
    scene.world = bpy.data.worlds.new("ip_avatar_world")
    scene.world.use_nodes = True
    world_background = next((node for node in scene.world.node_tree.nodes if node.type == "BACKGROUND"), None)
    if world_background:
        world_background.inputs["Color"].default_value = (0.11, 0.12, 0.14, 1.0)
        world_background.inputs["Strength"].default_value = 0.18

    height = float(dimensions["height"])
    colors = background_light_colors(str(data.get("backgroundPath") or ""))

    camera_data = bpy.data.cameras.new("Camera")
    camera = bpy.data.objects.new("Camera", camera_data)
    bpy.context.collection.objects.link(camera)
    camera.location = (0, -8.0, height * 0.55)
    look_at(camera, (0, 0, height * 0.49))
    camera_data.lens = 55
    camera_data.clip_start = 0.05
    scene.camera = camera

    def add_area(name: str, location, energy: float, size: float, color, specular: float) -> None:
        bpy.ops.object.light_add(type="AREA", location=location)
        light = bpy.context.object
        light.name = name
        light.data.energy = energy
        light.data.shape = "DISK"
        light.data.size = size
        light.data.color = color
        if hasattr(light.data, "specular_factor"):
            light.data.specular_factor = specular
        look_at(light, (0, 0, height * 0.52))

    add_area("IP_Front_Softbox", (-0.7, -4.4, height * 1.28), 380, 4.5, colors["center"], 0.04)
    add_area("IP_Cool_Key", (-3.0, -2.8, height * 1.18), 190, 3.6, colors["left"], 0.12)
    add_area("IP_Warm_Fill", (3.0, -2.5, height * 0.95), 135, 3.2, colors["right"], 0.12)

    bpy.ops.object.light_add(type="POINT", location=(0.15, -4.0, height * 0.77))
    catchlight = bpy.context.object
    catchlight.name = "IP_Face_Catchlight"
    catchlight.data.energy = 48
    catchlight.data.color = colors["center"]
    catchlight.data.shadow_soft_size = 0.08

    bpy.ops.object.light_add(type="POINT", location=(-1.8, 0.6, height * 0.88))
    cool_rim = bpy.context.object
    cool_rim.name = "IP_Cool_Rim"
    cool_rim.data.energy = 34
    cool_rim.data.color = colors["left"]
    bpy.ops.object.light_add(type="POINT", location=(1.8, 0.6, height * 0.88))
    warm_rim = bpy.context.object
    warm_rim.name = "IP_Warm_Rim"
    warm_rim.data.energy = 30
    warm_rim.data.color = colors["right"]
    create_contact_shadow(dimensions)


def place_character_in_authored_scene(
    character_objects: list[bpy.types.Object],
    imported_assets: list[bpy.types.Object],
    armature: bpy.types.Object,
    face: dict[str, bpy.types.Object],
    dimensions: dict[str, Any],
    mode_objects: dict[str, Any] | None = None,
) -> dict[str, Any]:
    spawn = mode_objects["spawn"] if mode_objects else bpy.data.objects.get("IP_Character_Spawn")
    if not spawn:
        spawn = bpy.data.objects.new("IP_Character_Spawn", None)
        bpy.context.scene.collection.objects.link(spawn)
        spawn.empty_display_type = "CIRCLE"
        spawn["target_height"] = float(dimensions["height"])

    placement = bpy.data.objects.new("IP_Character_Placement", None)
    bpy.context.scene.collection.objects.link(placement)
    placement.empty_display_type = "ARROWS"
    asset_objects = list(
        dict.fromkeys(
            character_objects
            + imported_assets
            + list(face.values())
            + [armature]
            + ([dimensions["container"]] if dimensions.get("container") else [])
        )
    )
    asset_set = set(asset_objects)
    roots = [obj for obj in asset_objects if obj.parent not in asset_set]
    for obj in roots:
        world_matrix = obj.matrix_world.copy()
        obj.parent = placement
        obj.matrix_world = world_matrix
    placement.matrix_world = spawn.matrix_world.copy()
    bpy.context.view_layer.update()

    min_v, max_v = object_bbox(character_objects)
    focus = mode_objects["focus"] if mode_objects else bpy.data.objects.get("IP_Focus_Head")
    if not focus:
        focus = bpy.data.objects.new("IP_Focus_Head", None)
        bpy.context.scene.collection.objects.link(focus)
        focus.empty_display_type = "SPHERE"
        character_height = max(0.01, max_v.z - min_v.z)
        focus.matrix_world.translation = Vector(
            (
                (min_v.x + max_v.x) * 0.5,
                (min_v.y + max_v.y) * 0.5,
                min_v.z + character_height * 0.72,
            )
        )
    focus_cameras = (
        mode_objects["cameras"].values()
        if mode_objects
        else (obj for obj in bpy.context.scene.objects if obj.type == "CAMERA")
    )
    for camera in focus_cameras:
        camera.data.dof.use_dof = True
        camera.data.dof.focus_object = focus
    if mode_objects:
        medium_camera = mode_objects["cameras"]["medium"]
        medium_camera.data["ip_authored_shift_y"] = float(medium_camera.data.shift_y)

    marker_names = {
        key: mode_objects[key].name
        for key in ("spawn", "focus", "seat", "foot_l", "foot_r")
    } if mode_objects else {"spawn": spawn.name, "focus": focus.name}
    camera_names = {
        role: camera.name for role, camera in mode_objects["cameras"].items()
    } if mode_objects else {}
    return {
        "mode": mode_objects["mode"] if mode_objects else "standing",
        "markerNames": marker_names,
        "cameraNames": camera_names,
        "spawnMarker": spawn.name,
        "placementRoot": placement.name,
        "focusMarker": focus.name,
        "worldBounds": {
            "min": [round(value, 5) for value in min_v],
            "max": [round(value, 5) for value in max_v],
        },
    }


def _vertex_group_weights(vertex: bpy.types.MeshVertex) -> dict[int, float]:
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


def _restore_armature_only_evaluated_mesh(
    evaluated: bpy.types.Object,
    states: list[tuple[bpy.types.Modifier, bool]],
) -> None:
    evaluated.to_mesh_clear()
    for modifier, show_viewport in states:
        modifier.show_viewport = show_viewport
    bpy.context.view_layer.update()


def sample_character_shoe_soles(
    character_objects: list[bpy.types.Object],
    armature: bpy.types.Object,
    bone_map: dict[str, str],
    floor_z: float,
) -> dict[str, Any]:
    checked_modifiers: list[str] = []
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
            checked_modifiers.append(f"{obj.name}/{modifier.name}")
    if not checked_modifiers:
        raise RuntimeError("shoe sole sampling found no Armature modifiers")

    depsgraph = bpy.context.evaluated_depsgraph_get()
    sides: dict[str, Any] = {}
    for side, label in (("l", "left"), ("r", "right")):
        group_name = bone_map[f"foot_{side}"]
        sole_points: list[Vector] = []
        sampled_meshes: list[str] = []
        for obj in character_objects:
            if obj.type != "MESH":
                continue
            group = obj.vertex_groups.get(group_name)
            if group is None:
                continue
            normal_matrix = obj.matrix_world.to_3x3().inverted_safe().transposed()
            eligible: list[int] = []
            rest_z: dict[int, float] = {}
            for vertex in obj.data.vertices:
                weights = _vertex_group_weights(vertex)
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
                    raise RuntimeError(f"shoe sole topology changed for {obj.name}")
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
                _restore_armature_only_evaluated_mesh(evaluated, states)
            if points:
                sampled_meshes.append(obj.name)
        if not sole_points:
            raise RuntimeError(f"no evaluated shoe sole geometry found for {label}")
        clearances = [float(point.z) - float(floor_z) for point in sole_points]
        sides[label] = {
            "minimum": min(clearances),
            "maximum": max(clearances),
            "center": [
                sum(float(point[axis]) for point in sole_points) / len(sole_points)
                for axis in range(3)
            ],
            "sampledVertexCount": len(sole_points),
            "sampledMeshes": sampled_meshes,
            "footGroup": group_name,
        }
    return {
        **sides,
        "floorZ": float(floor_z),
        "armatureModifiers": checked_modifiers,
    }


def _render_world_bvh(
    objects: list[bpy.types.Object],
    depsgraph: bpy.types.Depsgraph,
) -> BVHTree | None:
    vertices: list[Vector] = []
    triangles: list[list[int]] = []
    for obj in objects:
        if obj.type != "MESH" or obj.hide_render:
            continue
        evaluated = obj.evaluated_get(depsgraph)
        mesh = evaluated.to_mesh(preserve_all_data_layers=True, depsgraph=depsgraph)
        try:
            offset = len(vertices)
            vertices.extend(evaluated.matrix_world @ vertex.co for vertex in mesh.vertices)
            mesh.calc_loop_triangles()
            triangles.extend(
                [offset + int(index) for index in triangle.vertices]
                for triangle in mesh.loop_triangles
            )
        finally:
            evaluated.to_mesh_clear()
    if not vertices or not triangles:
        return None
    return BVHTree.FromPolygons(vertices, triangles, all_triangles=True, epsilon=1e-6)


def _mode_collision_obstacles() -> list[bpy.types.Object]:
    desk_root = next(
        (obj for obj in bpy.data.objects if obj.get("assembly_role") == "main_desk"),
        None,
    )
    chair_root = bpy.data.objects.get("Chair_Main")
    if desk_root is None or chair_root is None:
        raise RuntimeError("warm studio is missing authored desk/chair collision assemblies")
    return [
        obj
        for root in (desk_root, chair_root)
        for obj in (root, *root.children_recursive)
        if obj.type == "MESH" and not obj.hide_render
    ]


def calibrate_mode_collision_clearance(
    character_objects: list[bpy.types.Object],
    placement: bpy.types.Object,
    sample_frames: list[int] | tuple[int, ...] | None = None,
) -> dict[str, Any]:
    frames = list(sample_frames or range(bpy.context.scene.frame_start, bpy.context.scene.frame_end + 1))
    if not frames:
        raise RuntimeError("collision clearance calibration requires sampled frames")
    obstacles = _mode_collision_obstacles()
    base_y = float(placement.location.y)
    offsets = [0.0]
    for step in range(1, 31):
        offsets.extend((0.05 * step, -0.05 * step))
    selected_offset: float | None = None
    selected_counts: list[dict[str, int]] = []
    for offset in offsets:
        placement.location.y = base_y + offset
        bpy.context.view_layer.update()
        counts: list[dict[str, int]] = []
        for frame in frames:
            bpy.context.scene.frame_set(int(frame))
            bpy.context.view_layer.update()
            depsgraph = bpy.context.evaluated_depsgraph_get()
            character_tree = _render_world_bvh(character_objects, depsgraph)
            obstacle_tree = _render_world_bvh(obstacles, depsgraph)
            count = 0
            if character_tree is not None and obstacle_tree is not None:
                count = len(character_tree.overlap(obstacle_tree))
            counts.append({"frame": int(frame), "trianglePairCount": count})
            if count:
                break
        if len(counts) == len(frames) and not any(item["trianglePairCount"] for item in counts):
            selected_offset = offset
            selected_counts = counts
            break
    if selected_offset is None:
        placement.location.y = base_y
        bpy.context.view_layer.update()
        raise RuntimeError("cannot place character without desk/chair triangle intersections")
    bpy.context.scene.frame_set(frames[0])
    bpy.context.view_layer.update()
    return {
        "worldYOffset": selected_offset,
        "placementY": float(placement.location.y),
        "frames": selected_counts,
    }


def calibrate_mode_foot_contact(
    character_objects: list[bpy.types.Object],
    armature: bpy.types.Object,
    bone_map: dict[str, str],
    placement: bpy.types.Object,
    mode_objects: dict[str, Any],
    sample_frames: list[int] | tuple[int, ...] | None = None,
) -> dict[str, Any]:
    frames = list(sample_frames or range(bpy.context.scene.frame_start, bpy.context.scene.frame_end + 1))
    if not frames:
        raise RuntimeError("foot contact calibration requires sampled frames")
    targets = {
        "left": mode_objects["foot_l"],
        "right": mode_objects["foot_r"],
    }
    floor_z = min(float(target.matrix_world.translation.z) for target in targets.values())
    first_frame = frames[0]
    bpy.context.scene.frame_set(first_frame)
    bpy.context.view_layer.update()
    initial = sample_character_shoe_soles(character_objects, armature, bone_map, floor_z)
    source_center = Vector(
        tuple(
            sum(float(initial[side]["center"][axis]) for side in ("left", "right")) / 2.0
            for axis in range(3)
        )
    )
    target_center = sum(
        (target.matrix_world.translation for target in targets.values()),
        Vector((0.0, 0.0, 0.0)),
    ) / 2.0
    placement.location.x += target_center.x - source_center.x
    bpy.context.view_layer.update()

    base_location = placement.location.copy()
    base_rotation = placement.rotation_euler.copy()
    placement.rotation_mode = "XYZ"
    frame_reports: list[dict[str, Any]] = []
    for frame in frames:
        bpy.context.scene.frame_set(int(frame))
        placement.location = base_location
        placement.rotation_euler = base_rotation
        bpy.context.view_layer.update()
        for _ in range(3):
            soles = sample_character_shoe_soles(character_objects, armature, bone_map, floor_z)
            errors = {
                side: float(targets[side].matrix_world.translation.z)
                + TARGET_SOLE_CLEARANCE_M
                - float(soles[side]["minimum"])
                for side in ("left", "right")
            }
            left_x = float(soles["left"]["center"][0]) - float(placement.matrix_world.translation.x)
            right_x = float(soles["right"]["center"][0]) - float(placement.matrix_world.translation.x)
            span = left_x - right_x
            if abs(span) < 1e-5:
                placement.location.z += max(errors.values())
            else:
                delta_y_rotation = (errors["right"] - errors["left"]) / span
                delta_z = errors["left"] + delta_y_rotation * left_x
                placement.rotation_euler.y += delta_y_rotation
                placement.location.z += delta_z
            bpy.context.view_layer.update()
        corrected = sample_character_shoe_soles(character_objects, armature, bone_map, floor_z)
        placement.keyframe_insert(data_path="location", frame=int(frame))
        placement.keyframe_insert(data_path="rotation_euler", frame=int(frame))
        frame_reports.append(
            {
                "frame": int(frame),
                "left": float(corrected["left"]["minimum"]),
                "right": float(corrected["right"]["minimum"]),
            }
        )
    if placement.animation_data and placement.animation_data.action:
        for fcurve in iter_action_fcurves(placement.animation_data.action):
            for keyframe in fcurve.keyframe_points:
                keyframe.interpolation = "LINEAR"
    bpy.context.scene.frame_set(first_frame)
    bpy.context.view_layer.update()
    return {
        "targetMarkers": {side: target.name for side, target in targets.items()},
        "targetClearance": TARGET_SOLE_CLEARANCE_M,
        "horizontalOffset": [
            float(base_location.x - mode_objects["spawn"].location.x),
            float(base_location.y - mode_objects["spawn"].location.y),
        ],
        "targetCenterDelta": [
            float(target_center.x - source_center.x),
            float(target_center.y - source_center.y),
        ],
        "frames": frame_reports,
    }


def sample_character_semantic_regions(
    character_objects: list[bpy.types.Object],
    bone_map: dict[str, str],
) -> dict[str, list[Vector]]:
    role_groups = {
        "head": {bone_map["head"]},
        "leftHand": {
            name
            for role, name in bone_map.items()
            if role == "hand_l" or role.startswith("finger_") and role.endswith("_l")
        },
        "rightHand": {
            name
            for role, name in bone_map.items()
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
                weights = _vertex_group_weights(vertex)
                for role, indices in group_indices.items():
                    if sum(weights.get(index, 0.0) for index in indices) < 0.25:
                        continue
                    points[role].append(evaluated.matrix_world @ mesh.vertices[vertex.index].co)
        finally:
            _restore_armature_only_evaluated_mesh(evaluated, states)
    for role, samples in points.items():
        if not samples:
            raise RuntimeError(f"no evaluated semantic geometry found for {role}")
    return points


def _medium_frame_metrics(
    camera: bpy.types.Object,
    frame_points: dict[int, dict[str, list[Vector]]],
) -> dict[int, dict[str, dict[str, Any]]]:
    scene = bpy.context.scene
    metrics: dict[int, dict[str, dict[str, Any]]] = {}
    for frame, regions in frame_points.items():
        metrics[frame] = {}
        for role, points in regions.items():
            projected = [world_to_camera_view(scene, camera, point) for point in points]
            inside_count = sum(
                1
                for point in projected
                if point.z > 0 and 0 <= point.x <= 1 and 0 <= point.y <= 1
            )
            metrics[frame][role] = {
                "insideCount": inside_count,
                "frameBounds": {
                    "min": [
                        min(float(point.x) for point in projected),
                        min(float(point.y) for point in projected),
                    ],
                    "max": [
                        max(float(point.x) for point in projected),
                        max(float(point.y) for point in projected),
                    ],
                },
            }
    return metrics


def _medium_frame_candidate_is_safe(
    metrics: dict[int, dict[str, dict[str, Any]]],
) -> bool:
    for regions in metrics.values():
        for region in regions.values():
            if int(region["insideCount"]) < MIN_MEDIUM_FRAME_POINTS:
                return False
            bounds = region["frameBounds"]
            if any(float(value) < MEDIUM_FRAME_SAFE_MARGIN for value in bounds["min"]):
                return False
            if any(float(value) > 1.0 - MEDIUM_FRAME_SAFE_MARGIN for value in bounds["max"]):
                return False
    return True


def _medium_frame_composition_score(
    metrics: dict[int, dict[str, dict[str, Any]]],
    lens: float,
    shift_delta: float,
    authored_lens: float,
) -> tuple[float, float, float, float]:
    bounds = [
        region["frameBounds"]
        for regions in metrics.values()
        for region in regions.values()
    ]
    minimum_x = min(float(item["min"][0]) for item in bounds)
    minimum_y = min(float(item["min"][1]) for item in bounds)
    maximum_x = max(float(item["max"][0]) for item in bounds)
    maximum_y = max(float(item["max"][1]) for item in bounds)
    vertical_span = maximum_y - minimum_y
    center_error = abs((minimum_x + maximum_x) / 2.0 - 0.5) + abs(
        (minimum_y + maximum_y) / 2.0 - 0.5
    )
    return (
        abs(vertical_span - TARGET_MEDIUM_VERTICAL_SPAN),
        center_error,
        abs(authored_lens - lens),
        abs(shift_delta),
    )


def calibrate_mode_medium_camera(
    character_objects: list[bpy.types.Object],
    bone_map: dict[str, str],
    mode_objects: dict[str, Any],
    sample_frames: list[int] | tuple[int, ...] | None = None,
) -> dict[str, Any]:
    frames = list(sample_frames or range(bpy.context.scene.frame_start, bpy.context.scene.frame_end + 1))
    if not frames:
        raise RuntimeError("medium framing calibration requires sampled frames")
    camera = mode_objects["cameras"]["medium"]
    authored_lens = float(camera.data.lens)
    authored_shift_y = float(camera.data.get("ip_authored_shift_y", camera.data.shift_y))
    frame_points: dict[int, dict[str, list[Vector]]] = {}
    for frame in frames:
        bpy.context.scene.frame_set(int(frame))
        bpy.context.view_layer.update()
        frame_points[int(frame)] = sample_character_semantic_regions(
            character_objects,
            bone_map,
        )

    lens_candidates = [
        authored_lens - float(index)
        for index in range(int(max(0.0, authored_lens - 24.0)) + 1)
    ]
    shift_deltas = [0.0]
    for step in range(1, 31):
        shift_deltas.extend((-0.01 * step, 0.01 * step))
    safe_candidates: list[
        tuple[
            tuple[float, float, float, float],
            float,
            float,
            dict[int, dict[str, dict[str, Any]]],
        ]
    ] = []
    for lens in lens_candidates:
        camera.data.lens = max(24.0, lens)
        for delta in shift_deltas:
            camera.data.shift_y = authored_shift_y + delta
            metrics = _medium_frame_metrics(camera, frame_points)
            if _medium_frame_candidate_is_safe(metrics):
                safe_candidates.append(
                    (
                        _medium_frame_composition_score(
                            metrics,
                            float(camera.data.lens),
                            delta,
                            authored_lens,
                        ),
                        float(camera.data.lens),
                        float(camera.data.shift_y),
                        metrics,
                    )
                )
    if not safe_candidates:
        camera.data.lens = authored_lens
        camera.data.shift_y = authored_shift_y
        raise RuntimeError(
            f"{camera.name} cannot keep complete head and both hands inside "
            f"the {MEDIUM_FRAME_SAFE_MARGIN:.0%} safe frame"
        )
    _, selected_lens, selected_shift_y, selected_metrics = min(
        safe_candidates,
        key=lambda candidate: candidate[0],
    )
    camera.data.lens = selected_lens
    camera.data.shift_y = selected_shift_y
    bpy.context.scene.frame_set(frames[0])
    bpy.context.view_layer.update()
    return {
        "camera": camera.name,
        "authoredLens": authored_lens,
        "authoredShiftY": authored_shift_y,
        "lens": float(camera.data.lens),
        "shiftY": float(camera.data.shift_y),
        "minimumPointsPerRegion": MIN_MEDIUM_FRAME_POINTS,
        "safeMargin": MEDIUM_FRAME_SAFE_MARGIN,
        "frames": [
            {"frame": frame, **metrics}
            for frame, metrics in sorted(selected_metrics.items())
        ],
    }


def setup_authored_scene(
    data: dict,
    dimensions: dict[str, Any],
    mode_objects: dict[str, Any] | None = None,
) -> dict[str, Any]:
    configure_render_settings(data, authored_scene=True)
    lighting = apply_lighting_preset(str(data.get("lightingPreset") or "editorial_soft"))
    cameras = configure_camera_plan(data, mode_objects)
    markers = {
        key: mode_objects[key].name
        for key in ("spawn", "focus", "seat", "foot_l", "foot_r")
    } if mode_objects else {
        "spawn": "IP_Character_Spawn",
        "focus": "IP_Focus_Head",
    }
    return {
        "sceneMode": "blender_scene",
        "mode": mode_objects["mode"] if mode_objects else "standing",
        "sourceScene": str(data.get("sceneBlendPath") or ""),
        "targetCharacterHeight": float(dimensions["height"]),
        "markers": markers,
        "lighting": lighting,
        "cameras": cameras,
    }


def add_plane(name: str, loc: tuple[float, float, float], scale: tuple[float, float, float], mat: bpy.types.Material) -> bpy.types.Object:
    bpy.ops.mesh.primitive_plane_add(size=1, location=loc, rotation=(math.radians(90), 0, 0))
    obj = bpy.context.object
    obj.name = name
    obj.scale = scale
    obj.data.materials.append(mat)
    return obj


def bind_mesh_to_bone(obj: bpy.types.Object, armature: bpy.types.Object, bone_name: str) -> None:
    group = obj.vertex_groups.new(name=bone_name)
    group.add(range(len(obj.data.vertices)), 1.0, "REPLACE")
    modifier = obj.modifiers.new("IP_Face_Rig_Deform", "ARMATURE")
    modifier.object = armature
    modifier.use_vertex_groups = True
    obj.parent = armature
    obj.matrix_parent_inverse = armature.matrix_world.inverted()


def add_ellipse(
    name: str,
    location: tuple[float, float, float],
    radius_x: float,
    radius_z: float,
    mat: bpy.types.Material,
    segments: int = 64,
) -> bpy.types.Object:
    vertices = [
        (math.cos(index * math.tau / segments) * radius_x, 0.0, math.sin(index * math.tau / segments) * radius_z)
        for index in range(segments)
    ]
    mesh = bpy.data.meshes.new(f"{name}_Mesh")
    mesh.from_pydata(vertices, [], [tuple(range(segments))])
    mesh.materials.append(mat)
    obj = bpy.data.objects.new(name, mesh)
    bpy.context.collection.objects.link(obj)
    obj.location = location
    return obj


def mouth_shape_coordinates(shape_name: str, width: float, height: float, segments: int = 48) -> list[tuple[float, float, float]]:
    base_rx = width * 0.052
    thickness = height * 0.0042
    specifications = {
        "Mouth_Rest": (base_rx, height * 0.0060, "wave", height * 0.0060),
        "Mouth_A": (base_rx * 0.64, height * 0.0350, "flat", 0.0),
        "Mouth_E": (base_rx * 1.08, height * 0.0140, "flat", 0.0),
        "Mouth_O": (base_rx * 0.58, height * 0.0260, "flat", 0.0),
        "Mouth_U": (base_rx * 0.48, height * 0.0210, "flat", 0.0),
        "Mouth_MBP": (base_rx * 0.92, height * 0.0048, "flat", 0.0),
        "Mouth_Smile": (base_rx * 1.06, height * 0.0065, "smile", height * 0.0170),
        "Mouth_Frown": (base_rx * 0.96, height * 0.0065, "frown", height * 0.0140),
        "Mouth_Surprise": (base_rx * 0.56, height * 0.0410, "flat", 0.0),
    }
    radius_x, radius_z, curve_name, curve_amount = specifications[shape_name]

    def center_curve(x: float, local_radius_x: float) -> float:
        normalized = max(-1.0, min(1.0, x / max(local_radius_x, 1e-5)))
        if curve_name == "wave":
            return curve_amount * math.cos(normalized * math.tau)
        if curve_name == "smile":
            return curve_amount * (normalized * normalized - 0.38)
        if curve_name == "frown":
            return -curve_amount * (normalized * normalized - 0.38)
        return 0.0

    inner_radius_x = max(radius_x - thickness * 0.78, radius_x * 0.68)
    inner_radius_z = max(radius_z - thickness, radius_z * 0.22)
    outer: list[tuple[float, float, float]] = []
    inner: list[tuple[float, float, float]] = []
    for index in range(segments):
        angle = index * math.tau / segments
        outer_x = math.cos(angle) * radius_x
        inner_x = math.cos(angle) * inner_radius_x
        outer.append((outer_x, 0.0, center_curve(outer_x, radius_x) + math.sin(angle) * radius_z))
        inner.append((inner_x, 0.0, center_curve(inner_x, inner_radius_x) + math.sin(angle) * inner_radius_z))
    return outer + inner


def organic_mouth_shape_coordinates(
    shape_name: str,
    width: float,
    height: float,
    segments: int = 48,
) -> list[tuple[float, float, float]]:
    """Filled mouth silhouettes for organic faces rather than screen-face rings."""
    base_rx = width * 0.052
    specifications = {
        "Mouth_Rest": (base_rx * 0.82, height * 0.0032, "smile", height * 0.0050),
        "Mouth_A": (base_rx * 0.60, height * 0.0260, "flat", 0.0),
        "Mouth_E": (base_rx * 1.00, height * 0.0080, "flat", 0.0),
        "Mouth_O": (base_rx * 0.52, height * 0.0210, "flat", 0.0),
        "Mouth_U": (base_rx * 0.42, height * 0.0160, "flat", 0.0),
        "Mouth_MBP": (base_rx * 0.78, height * 0.0024, "flat", 0.0),
        "Mouth_Smile": (base_rx * 1.04, height * 0.0042, "smile", height * 0.0100),
        "Mouth_Frown": (base_rx * 0.94, height * 0.0046, "frown", height * 0.0090),
        "Mouth_Surprise": (base_rx * 0.48, height * 0.0310, "flat", 0.0),
    }
    radius_x, radius_z, curve_name, curve_amount = specifications[shape_name]

    def curve(x: float) -> float:
        normalized = max(-1.0, min(1.0, x / max(radius_x, 1e-6)))
        if curve_name == "smile":
            return curve_amount * (normalized * normalized - 0.42)
        if curve_name == "frown":
            return -curve_amount * (normalized * normalized - 0.42)
        return 0.0

    outer: list[tuple[float, float, float]] = []
    for index in range(segments):
        angle = index * math.tau / segments
        x = math.cos(angle) * radius_x
        outer.append((x, 0.0, curve(x) + math.sin(angle) * radius_z))
    return outer + [(0.0, 0.0, 0.0)]


def weighted_region_bounds(
    objects: list[bpy.types.Object],
    group_suffix: str,
    *,
    minimum_weight: float = 0.25,
) -> dict[str, Any] | None:
    normalized_suffix = "".join(character for character in group_suffix.lower() if character.isalnum())
    points: list[Vector] = []
    indices_by_object: dict[str, set[int]] = {}
    for obj in objects:
        if obj.type != "MESH":
            continue
        matching_groups = {
            group.index
            for group in obj.vertex_groups
            if "".join(character for character in group.name.lower() if character.isalnum()).endswith(normalized_suffix)
        }
        if not matching_groups:
            continue
        indices: set[int] = set()
        for vertex in obj.data.vertices:
            if any(group.group in matching_groups and group.weight >= minimum_weight for group in vertex.groups):
                indices.add(vertex.index)
                points.append(obj.matrix_world @ vertex.co)
        if indices:
            indices_by_object[obj.name] = indices
    if len(points) < 24:
        return None
    min_v = Vector(tuple(min(float(point[axis]) for point in points) for axis in range(3)))
    max_v = Vector(tuple(max(float(point[axis]) for point in points) for axis in range(3)))
    return {
        "min": min_v,
        "max": max_v,
        "width": float(max_v.x - min_v.x),
        "depth": float(max_v.y - min_v.y),
        "height": float(max_v.z - min_v.z),
        "indicesByObject": indices_by_object,
        "vertexCount": len(points),
    }


def _upstream_nodes(input_socket) -> set[bpy.types.Node]:
    pending = [link.from_node for link in input_socket.links]
    found: set[bpy.types.Node] = set()
    while pending:
        node = pending.pop()
        if node in found:
            continue
        found.add(node)
        pending.extend(link.from_node for socket in node.inputs for link in socket.links)
    return found


def _filename_texture_role(node: bpy.types.Node) -> str | None:
    image = getattr(node, "image", None)
    if not image:
        return None
    name = Path(image.filepath or image.name).name.lower()
    matches = [role for role in ("metallic", "normal", "roughness") if role in name]
    if len(matches) > 1:
        return None
    if matches:
        return matches[0]
    base_tokens = ("basecolor", "base_color", "albedo", "diffuse")
    return "base_color" if any(token in name for token in base_tokens) else None


def _resolve_source_pbr_texture_roles(source_material: bpy.types.Material) -> dict[str, bpy.types.Node]:
    """Resolve image roles from shader sockets, using filenames only for missing roles."""
    if not source_material.use_nodes:
        return {}
    nodes = source_material.node_tree.nodes
    principled = next((node for node in nodes if node.type == "BSDF_PRINCIPLED"), None)
    if not principled:
        return {}

    linked_inputs = {
        "base_color": principled.inputs.get("Base Color"),
        "metallic": principled.inputs.get("Metallic"),
        "roughness": principled.inputs.get("Roughness"),
        "normal": principled.inputs.get("Normal"),
    }
    resolved: dict[str, bpy.types.Node] = {}
    used_nodes: set[bpy.types.Node] = set()
    for role, input_socket in linked_inputs.items():
        if input_socket is None:
            continue
        candidates = [node for node in _upstream_nodes(input_socket) if node.type == "TEX_IMAGE" and node.image]
        if len(candidates) > 1:
            names = ", ".join(sorted(node.image.name for node in candidates))
            raise RuntimeError(f"multiple linked images resolve the {role} PBR role: {names}")
        if not candidates:
            continue
        node = candidates[0]
        filename_role = _filename_texture_role(node)
        if filename_role and filename_role != role:
            raise RuntimeError(
                f"PBR socket role {role} conflicts with filename role {filename_role}: {node.image.name}"
            )
        resolved[role] = node
        used_nodes.add(node)

    for role in ("base_color", "metallic", "normal", "roughness"):
        if role in resolved:
            continue
        candidates = [
            node
            for node in nodes
            if node.type == "TEX_IMAGE"
            and node.image
            and node not in used_nodes
            and _filename_texture_role(node) == role
        ]
        if len(candidates) > 1:
            names = ", ".join(sorted(node.image.name for node in candidates))
            raise RuntimeError(f"duplicate filename candidates for the {role} PBR role: {names}")
        if candidates:
            resolved[role] = candidates[0]
            used_nodes.add(candidates[0])
    return resolved


TASK6_FACE_METADATA_PROPERTIES = (
    "true_eyelid_topology",
    "eyelid_topology_mode",
    "blink_capability",
    "eye_region_subdivision_level",
    "eye_region_subdivision_added_vertices",
    "eye_region_boundary_fixed",
    "eye_region_uv_preserved",
    "eye_region_uv_guard_data",
    "eye_region_modified_uv_data",
    "eye_region_custom_data_signature",
    "eye_region_deform_weights_preserved",
    "eye_region_deform_evidence",
    "squint_max_closure_fraction",
    "source_pbr_materials_tuned",
    "source_pbr_material_names",
    "source_pbr_role_metadata",
)


def _task6_link_source(input_socket, node_type: str, output_name: str):
    links = list(input_socket.links) if input_socket else []
    if len(links) != 1:
        return None
    link = links[0]
    if link.from_node.type != node_type or link.from_socket.name != output_name:
        return None
    return link.from_node


def _task6_pbr_material_graph(
    source_material: bpy.types.Material,
) -> dict[str, bpy.types.Node] | None:
    """Resolve only the canonical tuned source graph or Blender's packed glTF graph."""
    if not source_material.use_nodes:
        return None
    nodes = source_material.node_tree.nodes
    if any(node.type in {"BUMP", "TEX_NOISE"} for node in nodes):
        return None
    principled_nodes = [node for node in nodes if node.type == "BSDF_PRINCIPLED"]
    if len(principled_nodes) != 1:
        return None
    principled = principled_nodes[0]
    base_color = _task6_link_source(principled.inputs.get("Base Color"), "TEX_IMAGE", "Color")
    normal_map = _task6_link_source(principled.inputs.get("Normal"), "NORMAL_MAP", "Normal")
    normal_image = (
        _task6_link_source(normal_map.inputs.get("Color"), "TEX_IMAGE", "Color")
        if normal_map
        else None
    )
    if not base_color or not normal_map or not normal_image:
        return None
    if (
        not base_color.image
        or not normal_image.image
        or base_color.image.colorspace_settings.name != "sRGB"
        or normal_image.image.colorspace_settings.name != "Non-Color"
        or normal_map.space != "TANGENT"
        or not 0.20 <= float(normal_map.inputs["Strength"].default_value) <= 0.50
    ):
        return None
    specular = principled.inputs.get("Specular IOR Level") or principled.inputs.get("Specular")
    if specular is None or not 0.20 <= float(specular.default_value) <= 0.35:
        return None

    metallic_link = list(principled.inputs["Metallic"].links)
    roughness_link = list(principled.inputs["Roughness"].links)
    if len(metallic_link) != 1 or len(roughness_link) != 1:
        return None
    metallic_source = metallic_link[0]
    roughness_source = roughness_link[0]
    if metallic_source.from_node.type == "TEX_IMAGE":
        metallic_image = (
            metallic_source.from_node
            if metallic_source.from_socket.name == "Color"
            else None
        )
        roughness_range = (
            roughness_source.from_node
            if roughness_source.from_node.type == "MAP_RANGE"
            and roughness_source.from_socket.name == "Result"
            else None
        )
        roughness_image = (
            _task6_link_source(roughness_range.inputs.get("Value"), "TEX_IMAGE", "Color")
            if roughness_range
            else None
        )
        if (
            not metallic_image
            or not roughness_image
            or not metallic_image.image
            or not roughness_image.image
            or metallic_image.image.colorspace_settings.name != "Non-Color"
            or roughness_image.image.colorspace_settings.name != "Non-Color"
            or abs(float(roughness_range.inputs["To Min"].default_value) - 0.38) > 1e-6
            or abs(float(roughness_range.inputs["To Max"].default_value) - 0.76) > 1e-6
        ):
            return None
    else:
        separate = metallic_source.from_node
        if (
            separate.type != "SEPARATE_COLOR"
            or roughness_source.from_node != separate
            or metallic_source.from_socket.name != "Blue"
            or roughness_source.from_socket.name != "Green"
        ):
            return None
        packed_image = _task6_link_source(separate.inputs.get("Color"), "TEX_IMAGE", "Color")
        if (
            not packed_image
            or not packed_image.image
            or packed_image.image.colorspace_settings.name != "Non-Color"
        ):
            return None
        metallic_image = packed_image
        roughness_image = packed_image

    return {
        "base_color": base_color,
        "metallic": metallic_image,
        "normal": normal_image,
        "roughness": roughness_image,
    }


def _task6_pbr_metadata_is_verifiable(obj: bpy.types.Object) -> bool:
    try:
        material_names = json.loads(str(obj.get("source_pbr_material_names") or "[]"))
        role_metadata = json.loads(str(obj.get("source_pbr_role_metadata") or "{}"))
    except (TypeError, json.JSONDecodeError):
        return False
    if (
        not isinstance(material_names, list)
        or not material_names
        or any(not isinstance(name, str) or not name for name in material_names)
        or not isinstance(role_metadata, dict)
        or set(role_metadata) != {"base_color", "metallic", "normal", "roughness"}
        or any(not isinstance(name, str) or not name for name in role_metadata.values())
    ):
        return False

    materials = {material.name: material for material in obj.data.materials if material}
    for material_name in material_names:
        source_material = materials.get(material_name)
        graph = _task6_pbr_material_graph(source_material) if source_material else None
        if not graph:
            return False
        for role, node in graph.items():
            actual_name = Path(node.image.name).stem.lower()
            claimed_name = Path(role_metadata[role]).stem.lower()
            if claimed_name not in actual_name and actual_name not in claimed_name:
                return False
    return True


def validate_task6_face_metadata(face_mesh: bpy.types.Object) -> None:
    missing = [name for name in TASK6_FACE_METADATA_PROPERTIES if name not in face_mesh]
    if face_mesh.get("eyelid_topology_mode") == "squint_only_source_skin":
        invalid: list[str] = []
        decoded: dict[str, Any] = {}
        side_properties = tuple(
            f"{name}_{side}"
            for side in ("l", "r")
            for name in (
                "eyeball_core_indices",
                "eyeball_core_uv_data",
                "squint_skin_indices",
                "squint_upper_indices",
                "squint_lower_indices",
                "eye_region_boundary_indices",
                "eye_region_boundary_basis_coordinates",
                "squint_eye_weight_zero",
                "eyeball_core_excluded",
                "squint_non_skin_max_displacement",
                "squint_core_max_displacement",
                "squint_center_x",
                "squint_center_z",
                "squint_radius_x",
                "squint_radius_z",
            )
        )
        missing.extend(name for name in side_properties if name not in face_mesh)
        for name in (
            "eye_region_uv_guard_data",
            "eye_region_modified_uv_data",
            "eye_region_custom_data_signature",
            "eye_region_deform_evidence",
            "source_pbr_material_names",
            "source_pbr_role_metadata",
            "eyeball_core_indices_l",
            "eyeball_core_indices_r",
            "eyeball_core_uv_data_l",
            "eyeball_core_uv_data_r",
            "squint_skin_indices_l",
            "squint_skin_indices_r",
        ):
            if name not in face_mesh:
                continue
            try:
                value = json.loads(str(face_mesh[name]))
            except (TypeError, json.JSONDecodeError):
                invalid.append(name)
                continue
            if value in ({}, []):
                invalid.append(name)
            else:
                decoded[name] = value
        if bool(face_mesh.get("true_eyelid_topology")):
            invalid.append("true_eyelid_topology")
        if face_mesh.get("blink_capability") != "squint_only":
            invalid.append("blink_capability")
        if int(face_mesh.get("eye_region_subdivision_level", -1)) != 0:
            invalid.append("eye_region_subdivision_level")
        if int(face_mesh.get("eye_region_subdivision_added_vertices", -1)) != 0:
            invalid.append("eye_region_subdivision_added_vertices")
        for name in (
            "eye_region_boundary_fixed",
            "eye_region_uv_preserved",
            "eye_region_deform_weights_preserved",
            "source_pbr_materials_tuned",
        ):
            if not bool(face_mesh.get(name)):
                invalid.append(name)
        try:
            stored_closure = float(face_mesh.get("squint_max_closure_fraction", 0.0))
        except (TypeError, ValueError):
            stored_closure = 0.0
        if not math.isfinite(stored_closure) or not 0.0 < stored_closure <= 0.12:
            invalid.append("squint_max_closure_fraction")
        modified = decoded.get("eye_region_modified_uv_data", {})
        if modified and (
            int(modified.get("new_lid_vertex_count", -1)) != 0
            or not modified.get("all_finite")
            or not modified.get("all_within_source_bounds")
            or set(modified.get("sides", {})) != {"l", "r"}
        ):
            invalid.append("eye_region_modified_uv_data")
        for side in ("l", "r"):
            side_uv_data = modified.get("sides", {}).get(side, {})
            skin_indices = set(side_uv_data.get("squint_skin_indices", []))
            skin_uv_entries = side_uv_data.get("squint_skin_uv_data", [])
            if (
                not skin_indices
                or not skin_uv_entries
                or {int(item.get("vertex", -1)) for item in skin_uv_entries} != skin_indices
                or any(not item.get("uvs") for item in skin_uv_entries)
            ):
                invalid.append("eye_region_modified_uv_data")
        deform = decoded.get("eye_region_deform_evidence", {})
        if deform and (
            int(deform.get("new_lid_vertex_count", -1)) != 0
            or int(deform.get("squint_vertex_count", 0)) <= 0
            or deform.get("preserved_original_vertex_count") != deform.get("original_vertex_count")
        ):
            invalid.append("eye_region_deform_evidence")
        role_metadata = decoded.get("source_pbr_role_metadata", {})
        if role_metadata and set(role_metadata) != {"base_color", "metallic", "normal", "roughness"}:
            invalid.append("source_pbr_role_metadata")
        if bool(face_mesh.get("source_pbr_materials_tuned")) and not _task6_pbr_metadata_is_verifiable(face_mesh):
            invalid.append("source_pbr_graph")
        for side in ("l", "r"):
            core = set(decoded.get(f"eyeball_core_indices_{side}", []))
            skin = set(decoded.get(f"squint_skin_indices_{side}", []))
            upper = set(json.loads(str(face_mesh.get(f"squint_upper_indices_{side}") or "[]")))
            lower = set(json.loads(str(face_mesh.get(f"squint_lower_indices_{side}") or "[]")))
            if (
                not core
                or not skin
                or skin.intersection(core)
                or upper.union(lower) != skin
                or upper.intersection(lower)
                or not bool(face_mesh.get(f"squint_eye_weight_zero_{side}"))
                or not bool(face_mesh.get(f"eyeball_core_excluded_{side}"))
                or float(face_mesh.get(f"squint_non_skin_max_displacement_{side}", 1.0)) > 1e-9
            ):
                invalid.append(f"squint_skin_indices_{side}")
        keys = face_mesh.data.shape_keys.key_blocks if face_mesh.data.shape_keys else None
        if (
            not keys
            or not keys.get("Eye_Squint.L")
            or not keys.get("Eye_Squint.R")
            or keys.get("Eye_Blink.L")
            or keys.get("Eye_Blink.R")
        ):
            invalid.append("squint_shape_keys")
        else:
            basis = keys["Basis"]
            vertex_count = len(basis.data)
            uv_by_vertex: dict[int, list[tuple[float, float]]] = {
                index: [] for index in range(vertex_count)
            }
            active_uv = face_mesh.data.uv_layers.active
            if active_uv:
                for loop_index, loop in enumerate(face_mesh.data.loops):
                    uv = active_uv.data[loop_index].uv
                    uv_by_vertex[int(loop.vertex_index)].append((float(uv.x), float(uv.y)))
            else:
                invalid.append("squint_skin_uv_data")

            def group_weight(group, index: int) -> float:
                if not group:
                    return 0.0
                try:
                    return float(group.weight(index))
                except RuntimeError:
                    return 0.0

            original_vertex_count = int(deform.get("original_vertex_count", vertex_count))
            runtime: dict[str, dict[str, Any]] = {}
            for side in ("l", "r"):
                suffix = side.upper()
                try:
                    core = {int(value) for value in decoded[f"eyeball_core_indices_{side}"]}
                    skin = {int(value) for value in decoded[f"squint_skin_indices_{side}"]}
                    upper = {
                        int(value)
                        for value in json.loads(str(face_mesh[f"squint_upper_indices_{side}"]))
                    }
                    lower = {
                        int(value)
                        for value in json.loads(str(face_mesh[f"squint_lower_indices_{side}"]))
                    }
                    center_x = float(face_mesh[f"squint_center_x_{side}"])
                    center_z = float(face_mesh[f"squint_center_z_{side}"])
                    radius_x = float(face_mesh[f"squint_radius_x_{side}"])
                    radius_z = float(face_mesh[f"squint_radius_z_{side}"])
                    stored_non_skin = float(face_mesh[f"squint_non_skin_max_displacement_{side}"])
                    stored_core = float(face_mesh[f"squint_core_max_displacement_{side}"])
                except (KeyError, TypeError, ValueError, json.JSONDecodeError):
                    invalid.append(f"squint_skin_indices_{side}")
                    continue
                if (
                    not all(math.isfinite(value) for value in (center_x, center_z, radius_x, radius_z))
                    or radius_x <= 0.0
                    or radius_z <= 0.0
                ):
                    invalid.append(f"squint_radius_x_{side}")
                    continue
                if (
                    not math.isfinite(stored_non_skin)
                    or stored_non_skin > 1e-9
                    or not math.isfinite(stored_core)
                    or stored_core > 1e-9
                ):
                    invalid.append(f"squint_core_max_displacement_{side}")

                stored_skin_entries = modified.get("sides", {}).get(side, {}).get(
                    "squint_skin_uv_data", []
                )
                try:
                    stored_skin_uv_by_vertex = {
                        int(item["vertex"]): sorted(
                            (float(uv[0]), float(uv[1]))
                            for uv in item.get("uvs", [])
                            if len(uv) == 2
                        )
                        for item in stored_skin_entries
                    }
                except (KeyError, TypeError, ValueError):
                    stored_skin_uv_by_vertex = {}
                stored_skin_uvs = [
                    uv
                    for coordinates in stored_skin_uv_by_vertex.values()
                    for uv in coordinates
                ]
                if (
                    not stored_skin_uvs
                    or set(stored_skin_uv_by_vertex) != skin
                    or not all(math.isfinite(value) for uv in stored_skin_uvs for value in uv)
                    or not all(0.0 <= value <= 1.0 for uv in stored_skin_uvs for value in uv)
                ):
                    invalid.append(f"squint_skin_uv_data_{side}")

                if original_vertex_count == vertex_count:
                    allowed_skin = {index for index in skin if 0 <= index < vertex_count}
                    for index in allowed_skin:
                        actual_uvs = sorted(uv_by_vertex.get(index, []))
                        expected_uvs = stored_skin_uv_by_vertex.get(index, [])
                        if (
                            len(actual_uvs) != len(expected_uvs)
                            or not all(
                                abs(actual[axis] - expected[axis]) <= 1e-7
                                for actual, expected in zip(actual_uvs, expected_uvs)
                                for axis in (0, 1)
                            )
                            or not all(
                                math.isfinite(value) and 0.0 <= value <= 1.0
                                for uv in actual_uvs
                                for value in uv
                            )
                        ):
                            invalid.append(f"squint_skin_uv_data_{side}")
                            break
                else:
                    allowed_skin = {
                        index
                        for index, actual_uvs in uv_by_vertex.items()
                        if actual_uvs
                        and all(
                            any(
                                abs(actual[0] - expected[0]) <= 5e-5
                                and abs(actual[1] - expected[1]) <= 5e-5
                                for expected in stored_skin_uvs
                            )
                            for actual in actual_uvs
                        )
                    }
                shape = keys[f"Eye_Squint.{suffix}"]
                displacements = [
                    float((shape.data[index].co - basis.data[index].co).length)
                    for index in range(vertex_count)
                ]
                if not all(math.isfinite(value) for value in displacements):
                    invalid.append(f"squint_shape_data_{side}")
                    continue
                moved = {index for index, value in enumerate(displacements) if value > 1e-9}
                eye_groups = (
                    face_mesh.vertex_groups.get("Eye.L"),
                    face_mesh.vertex_groups.get("Eye.R"),
                )
                own_eye_group = eye_groups[0 if side == "l" else 1]
                actual_core = {
                    index
                    for index in range(vertex_count)
                    if group_weight(own_eye_group, index) > 0.015
                }
                actual_core_max = max((displacements[index] for index in actual_core), default=1.0)
                actual_non_skin_max = max(
                    (value for index, value in enumerate(displacements) if index not in allowed_skin),
                    default=0.0,
                )
                if actual_core_max > 1e-9:
                    invalid.append(f"squint_core_max_displacement_{side}")
                if actual_non_skin_max > 1e-9:
                    invalid.append(f"squint_non_skin_max_displacement_{side}")
                if not moved:
                    invalid.append(f"squint_closure_{side}")
                if any(
                    group_weight(group, index) > 1e-8
                    for index in moved
                    for group in eye_groups
                ):
                    invalid.append(f"squint_eye_weight_zero_{side}")
                if any(
                    not uv_by_vertex.get(index)
                    or not all(
                        math.isfinite(value) and 0.0 <= value <= 1.0
                        for uv in uv_by_vertex[index]
                        for value in uv
                    )
                    for index in moved
                ):
                    invalid.append(f"squint_skin_uv_data_{side}")
                if original_vertex_count != vertex_count and not moved.issubset(allowed_skin):
                    invalid.append(f"squint_skin_uv_data_{side}")

                closure_ratios: list[float] = []
                axis_tolerance = 5e-8 if original_vertex_count == vertex_count else 5e-6
                for index in moved:
                    basis_world = face_mesh.matrix_world @ basis.data[index].co
                    shape_world = face_mesh.matrix_world @ shape.data[index].co
                    displacement_world = shape_world - basis_world
                    if (
                        abs(float(displacement_world.x)) > axis_tolerance
                        or abs(float(displacement_world.y)) > axis_tolerance
                    ):
                        invalid.append(f"squint_displacement_axis_{side}")
                    target = center_z - float(basis_world.z)
                    delta = float(displacement_world.z)
                    if abs(delta) <= 1e-9:
                        continue
                    if abs(target) <= 1e-9:
                        invalid.append(f"squint_closure_{side}")
                        continue
                    ratio = delta / target
                    closure_ratios.append(ratio)
                    if original_vertex_count == vertex_count:
                        normalized_x = (float(basis_world.x) - center_x) / radius_x
                        normalized_z = (float(basis_world.z) - center_z) / radius_z
                        if math.hypot(normalized_x, normalized_z) > 1.30 + 1e-5:
                            invalid.append(f"squint_skin_indices_{side}")
                        if index in upper and normalized_z <= 0.0:
                            invalid.append(f"squint_upper_indices_{side}")
                        if index in lower and normalized_z >= 0.0:
                            invalid.append(f"squint_lower_indices_{side}")
                if (
                    not closure_ratios
                    or min(closure_ratios) <= 0.0
                    or max(closure_ratios) > min(stored_closure, 0.12) + 1e-6
                ):
                    invalid.append(f"squint_closure_{side}")
                runtime[side] = {
                    "allowed_skin": allowed_skin,
                    "moved": moved,
                    "displacements": displacements,
                    "skin": skin,
                }

            if set(runtime) == {"l", "r"}:
                for side, opposite in (("l", "r"), ("r", "l")):
                    if (
                        runtime[side]["skin"].intersection(runtime[opposite]["skin"])
                        or runtime[side]["moved"].intersection(runtime[opposite]["moved"])
                        or max(
                            (
                                runtime[side]["displacements"][index]
                                for index in runtime[opposite]["allowed_skin"]
                            ),
                            default=0.0,
                        ) > 1e-9
                    ):
                        invalid.append(f"squint_opposite_side_isolation_{side}")
        if any(
            material and material.name.startswith(("IP_EyelidSkin.", "IP_EyelidMargin."))
            for material in face_mesh.data.materials
        ):
            invalid.append("generated_eyelid_materials")
        problems = sorted(set(missing + invalid))
        if problems:
            raise RuntimeError(
                "Task 6 squint-only metadata is incomplete "
                f"({', '.join(problems)}); rebuild master from source FBX"
            )
        return
    raise RuntimeError(
        "Task 6 full-blink metadata is no longer reusable; "
        "rebuild master from source FBX for blinkCapability=squint_only"
    )

def tune_source_pbr_materials(obj: bpy.types.Object) -> dict[str, Any]:
    """Keep the source image stack intact while making it predictable for close shots."""
    previously_tuned = bool(obj.get("source_pbr_materials_tuned"))
    previous_material_names = str(obj.get("source_pbr_material_names") or "[]")
    previous_role_metadata = str(obj.get("source_pbr_role_metadata") or "{}")
    tuned_materials: list[str] = []
    tuned_roles: dict[str, str] = {}
    texture_names = {
        "base_color": "Image Texture - Base Color",
        "metallic": "Image Texture - Metallic",
        "normal": "Image Texture - Normal",
        "roughness": "Image Texture - Roughness",
    }
    for source_material in obj.data.materials:
        if not source_material or not source_material.use_nodes:
            continue
        nodes = source_material.node_tree.nodes
        links = source_material.node_tree.links
        images = _resolve_source_pbr_texture_roles(source_material)
        if not set(texture_names).issubset(images) or len(set(images.values())) != len(texture_names):
            continue

        principled = next((node for node in nodes if node.type == "BSDF_PRINCIPLED"), None)
        if not principled:
            continue
        linked_role_resolution = all(
            images[role] in _upstream_nodes(principled.inputs[socket_name])
            for role, socket_name in (
                ("base_color", "Base Color"),
                ("metallic", "Metallic"),
                ("normal", "Normal"),
                ("roughness", "Roughness"),
            )
        )
        normal_candidates = [node for node in _upstream_nodes(principled.inputs["Normal"]) if node.type == "NORMAL_MAP"]
        normal_map = normal_candidates[0] if len(normal_candidates) == 1 else None
        if not normal_map:
            continue
        roughness_range = next(
            (node for node in _upstream_nodes(principled.inputs["Roughness"]) if node.type == "MAP_RANGE"),
            None,
        ) or nodes.new("ShaderNodeMapRange")

        resolved_names = {
            principled: "Principled BSDF",
            normal_map: "Normal Map",
            roughness_range: "Roughness Range",
            **{node: texture_names[role] for role, node in images.items()},
        }
        for index, node in enumerate(resolved_names):
            node.name = f"__IP_PBR_RESOLVED_{index:02d}"
        for node, target_name in resolved_names.items():
            collision = nodes.get(target_name)
            if collision and collision != node:
                collision.name = f"Unresolved {target_name}"
            node.name = target_name

        principled.label = "Source PBR - restrained close shot"
        normal_map.label = "Source tangent normal"
        normal_map.space = "TANGENT"
        normal_map.inputs["Strength"].default_value = 0.34
        for role, node in images.items():
            node.label = texture_names[role]
            node.image.colorspace_settings.name = "sRGB" if role == "base_color" else "Non-Color"

        roughness_range.label = "Soft fur and fabric response"
        if hasattr(roughness_range, "clamp"):
            roughness_range.clamp = True
        roughness_range.inputs["From Min"].default_value = 0.0
        roughness_range.inputs["From Max"].default_value = 1.0
        roughness_range.inputs["To Min"].default_value = 0.38
        roughness_range.inputs["To Max"].default_value = 0.76

        def replace_input_link(input_socket, output_socket) -> None:
            for link in list(input_socket.links):
                links.remove(link)
            links.new(output_socket, input_socket)

        replace_input_link(principled.inputs["Base Color"], images["base_color"].outputs["Color"])
        replace_input_link(principled.inputs["Metallic"], images["metallic"].outputs["Color"])
        replace_input_link(normal_map.inputs["Color"], images["normal"].outputs["Color"])
        replace_input_link(principled.inputs["Normal"], normal_map.outputs["Normal"])
        replace_input_link(roughness_range.inputs["Value"], images["roughness"].outputs["Color"])
        replace_input_link(principled.inputs["Roughness"], roughness_range.outputs["Result"])

        specular = principled.inputs.get("Specular IOR Level") or principled.inputs.get("Specular")
        if specular is not None:
            specular.default_value = 0.28
        source_material["ip_source_pbr_tuned"] = True
        source_material["ip_source_pbr_texture_resolution"] = 4096
        source_material["ip_source_pbr_role_resolution"] = (
            "socket_links" if linked_role_resolution else "socket_links_with_filename_fallback"
        )
        source_material["ip_source_pbr_roles"] = json.dumps(
            {role: node.image.name for role, node in sorted(images.items())},
            sort_keys=True,
        )
        tuned_roles.update({role: node.image.name for role, node in images.items()})
        tuned_materials.append(source_material.name)
    if tuned_materials:
        material_lookup = {material.name: material for material in obj.data.materials if material}
        if any(
            not _task6_pbr_material_graph(material_lookup[name])
            for name in tuned_materials
        ):
            raise RuntimeError(
                "Task 6 source PBR graph is not canonical; rebuild master from source FBX"
            )
        obj["source_pbr_materials_tuned"] = True
        obj["source_pbr_material_names"] = json.dumps(tuned_materials)
        obj["source_pbr_role_metadata"] = json.dumps(tuned_roles, sort_keys=True)
        return {"materials": tuned_materials, "count": len(tuned_materials), "preserved": False}
    if previously_tuned:
        if _task6_pbr_metadata_is_verifiable(obj):
            try:
                preserved_names = json.loads(previous_material_names)
            except json.JSONDecodeError:
                preserved_names = []
            return {"materials": preserved_names, "count": len(preserved_names), "preserved": True}
        obj["source_pbr_materials_tuned"] = False
        obj["source_pbr_material_names"] = "[]"
        obj["source_pbr_role_metadata"] = "{}"
        raise RuntimeError(
            "Task 6 source PBR graph cannot verify stored material claims; "
            "rebuild master from source FBX"
        )
    obj["source_pbr_materials_tuned"] = False
    obj["source_pbr_material_names"] = "[]"
    obj["source_pbr_role_metadata"] = "{}"
    return {"materials": [], "count": 0, "preserved": False}


def refine_integrated_eye_regions(
    obj: bpy.types.Object,
    dimensions: dict[str, Any],
    head_bounds: dict[str, Any],
    bone_map: dict[str, str],
) -> dict[str, int]:
    """Build a restrained source-skin squint without deforming the eye surface."""
    if obj.get("true_eyelid_topology") or obj.get("blink_capability") == "squint_only":
        validate_task6_face_metadata(obj)
        return {
            side: len(json.loads(str(obj.get(f"squint_skin_indices_{side}") or "[]")))
            for side in ("l", "r")
        }
    if obj.data.shape_keys:
        raise RuntimeError("integrated eye refinement must run before creating shape keys")

    mesh = obj.data
    source_material_names_before = [
        source_material.name for source_material in mesh.materials if source_material
    ]
    object_to_world = obj.matrix_world.copy()
    world_to_object = object_to_world.inverted()
    depth = max(float(head_bounds["depth"]), 0.01)
    uv_names_before = [layer.name for layer in mesh.uv_layers]
    active_uv_name = mesh.uv_layers.active.name if mesh.uv_layers.active else ""
    if not active_uv_name:
        raise RuntimeError("squint-only fallback requires the source UV layer")
    color_attributes_before = sorted(
        (
            {
                "name": attribute.name,
                "data_type": attribute.data_type,
                "domain": attribute.domain,
            }
            for attribute in mesh.color_attributes
        ),
        key=lambda item: (item["name"], item["domain"], item["data_type"]),
    )
    custom_attributes_before = sorted(
        (
            {
                "name": attribute.name,
                "data_type": attribute.data_type,
                "domain": attribute.domain,
            }
            for attribute in mesh.attributes
            if not attribute.name.startswith(".")
        ),
        key=lambda item: (item["name"], item["domain"], item["data_type"]),
    )
    vertex_count_before = len(mesh.vertices)
    group_names_before = {group.index: group.name for group in obj.vertex_groups}
    deform_weights_before = {
        vertex.index: sorted(
            (group_names_before[item.group], float(item.weight))
            for item in vertex.groups
            if item.group in group_names_before and item.weight > 1e-8
        )
        for vertex in mesh.vertices
    }
    world_coordinates = [object_to_world @ vertex.co for vertex in mesh.vertices]
    adjacency = {vertex.index: set() for vertex in mesh.vertices}
    for edge in mesh.edges:
        left, right = (int(index) for index in edge.vertices)
        adjacency[left].add(right)
        adjacency[right].add(left)

    def collect_vertex_uvs(indices: set[int]) -> dict[int, list[list[float]]]:
        collected = {index: [] for index in indices}
        uv_layer = mesh.uv_layers[active_uv_name]
        for polygon in mesh.polygons:
            for loop_index in polygon.loop_indices:
                vertex_index = int(mesh.loops[loop_index].vertex_index)
                if vertex_index in collected:
                    uv = uv_layer.data[loop_index].uv
                    collected[vertex_index].append([float(uv.x), float(uv.y)])
        return {index: sorted(values) for index, values in collected.items()}

    def shortest_rim_path(
        allowed: set[int],
        start: int,
        goal: int,
        center_x: float,
        center_z: float,
        radius_x: float,
        radius_z: float,
        *,
        upper: bool,
    ) -> list[int]:
        queue: list[tuple[float, int]] = [(0.0, start)]
        distances = {start: 0.0}
        previous: dict[int, int] = {}
        while queue:
            distance, index = heapq.heappop(queue)
            if index == goal:
                break
            if distance > distances.get(index, float("inf")):
                continue
            for neighbor in adjacency[index]:
                if neighbor not in allowed:
                    continue
                nx = (float(world_coordinates[neighbor].x) - center_x) / radius_x
                nz = (float(world_coordinates[neighbor].z) - center_z) / radius_z
                radial = math.sqrt(nx * nx + nz * nz)
                wrong_side = max(0.0, -nz if upper else nz)
                edge_length = (world_coordinates[neighbor] - world_coordinates[index]).length
                candidate = (
                    distance
                    + edge_length * (1.0 + abs(radial - 1.05) * 2.0)
                    + wrong_side * 0.08
                )
                if candidate >= distances.get(neighbor, float("inf")):
                    continue
                distances[neighbor] = candidate
                previous[neighbor] = index
                heapq.heappush(queue, (candidate, neighbor))
        if goal not in distances:
            return []
        path = [goal]
        while path[-1] != start:
            path.append(previous[path[-1]])
        return list(reversed(path))

    descriptors: dict[str, dict[str, Any]] = {}
    selected_region: set[int] = set()
    original_boundary_coordinates: dict[str, list[list[Any]]] = {}
    for side, role in (("l", "eye_l"), ("r", "eye_r")):
        group_name = bone_map.get(role)
        eye_group = obj.vertex_groups.get(group_name) if group_name else None
        if not eye_group:
            raise RuntimeError(f"squint-only fallback requires the resolved {role} group")
        core = {
            vertex.index
            for vertex in mesh.vertices
            if any(
                assignment.group == eye_group.index and assignment.weight > 0.015
                for assignment in vertex.groups
            )
        }
        if len(core) < 24:
            raise RuntimeError(
                f"squint-only fallback found too few {side.upper()} eye-core vertices"
            )
        center_x = sum(float(world_coordinates[index].x) for index in core) / len(core)
        center_z = sum(float(world_coordinates[index].z) for index in core) / len(core)
        radius_x = (
            max(float(world_coordinates[index].x) for index in core)
            - min(float(world_coordinates[index].x) for index in core)
        ) * 0.60
        radius_z = (
            max(float(world_coordinates[index].z) for index in core)
            - min(float(world_coordinates[index].z) for index in core)
        ) * 0.58
        if radius_x <= 1e-6 or radius_z <= 1e-6:
            raise RuntimeError(f"squint-only fallback found invalid {side.upper()} eye bounds")

        def normalized_radius(index: int) -> float:
            nx = (float(world_coordinates[index].x) - center_x) / radius_x
            nz = (float(world_coordinates[index].z) - center_z) / radius_z
            return math.sqrt(nx * nx + nz * nz)

        def source_eye_weight(index: int) -> float:
            return max(
                (
                    float(assignment.weight)
                    for assignment in mesh.vertices[index].groups
                    if assignment.group == eye_group.index
                ),
                default=0.0,
            )

        first_skin_ring = {
            neighbor
            for index in core
            for neighbor in adjacency[index]
            if neighbor not in core and source_eye_weight(neighbor) <= 1e-8
        }
        second_skin_ring = {
            neighbor
            for index in first_skin_ring
            for neighbor in adjacency[index]
            if neighbor not in core
            and neighbor not in first_skin_ring
            and source_eye_weight(neighbor) <= 1e-8
        }
        skin_annulus = first_skin_ring.union(second_skin_ring)
        if len(skin_annulus) < 24:
            raise RuntimeError(
                f"squint-only fallback found too few topological {side.upper()} skin-ring vertices: "
                f"{len(skin_annulus)}"
            )

        front_limit = float(head_bounds["min"].y) + depth * 0.48
        annulus = {
            vertex.index
            for vertex in mesh.vertices
            if vertex.index not in core
            and 0.72 <= normalized_radius(vertex.index) <= 1.55
            and float(world_coordinates[vertex.index].y) <= front_limit
        }
        if len(annulus) < 48:
            raise RuntimeError(
                f"squint-only fallback could not isolate the {side.upper()} skin rim"
            )
        left = min(
            annulus,
            key=lambda index: (
                (float(world_coordinates[index].x) - (center_x - radius_x * 0.96)) / radius_x
            ) ** 2
            + ((float(world_coordinates[index].z) - center_z) / radius_z) ** 2,
        )
        right = min(
            annulus,
            key=lambda index: (
                (float(world_coordinates[index].x) - (center_x + radius_x * 0.96)) / radius_x
            ) ** 2
            + ((float(world_coordinates[index].z) - center_z) / radius_z) ** 2,
        )
        upper_allowed = {
            index
            for index in annulus
            if float(world_coordinates[index].z) >= center_z - radius_z * 0.18
        }.union({left, right})
        lower_allowed = {
            index
            for index in annulus
            if float(world_coordinates[index].z) <= center_z + radius_z * 0.18
        }.union({left, right})
        upper_path = shortest_rim_path(
            upper_allowed, left, right, center_x, center_z, radius_x, radius_z, upper=True
        )
        lower_path = shortest_rim_path(
            lower_allowed, left, right, center_x, center_z, radius_x, radius_z, upper=False
        )
        if min(len(upper_path), len(lower_path)) < 8:
            raise RuntimeError(
                f"squint-only fallback found incomplete {side.upper()} rim paths: "
                f"upper={len(upper_path)}, lower={len(lower_path)}"
            )
        boundary = set(upper_path).union(lower_path)
        boundary_uvs = [
            uv
            for values in collect_vertex_uvs(boundary).values()
            for uv in values
        ]
        if not boundary_uvs:
            raise RuntimeError(f"integrated {side.upper()} skin rim has no source UVs")
        source_uv_bounds = [
            min(uv[0] for uv in boundary_uvs),
            min(uv[1] for uv in boundary_uvs),
            max(uv[0] for uv in boundary_uvs),
            max(uv[1] for uv in boundary_uvs),
        ]
        selected_region.update(core, annulus)
        original_boundary_coordinates[side] = [
            [index, [float(value) for value in mesh.vertices[index].co]]
            for index in sorted(boundary)
        ]
        descriptors[side] = {
            "center_x": center_x,
            "center_z": center_z,
            "radius_x": radius_x,
            "radius_z": radius_z,
            "core": core,
            "upper_path": upper_path,
            "lower_path": lower_path,
            "eye_group_index": eye_group.index,
            "core_uvs": collect_vertex_uvs(core),
            "source_uv_bounds": source_uv_bounds,
            "annulus": annulus,
            "skin_annulus": skin_annulus,
        }

    squint_regions: dict[str, set[int]] = {}
    all_squint_indices: set[int] = set()
    minimum_weight_sum = float("inf")
    maximum_influences = 0
    for side, descriptor in descriptors.items():
        eye_group_index = int(descriptor["eye_group_index"])

        def resolved_eye_weight(index: int) -> float:
            return max(
                (
                    float(assignment.weight)
                    for assignment in mesh.vertices[index].groups
                    if assignment.group == eye_group_index
                ),
                default=0.0,
            )

        candidates = {
            index
            for index in descriptor["skin_annulus"]
            if resolved_eye_weight(index) <= 1e-8
            and float(world_coordinates[index].y) <= float(head_bounds["min"].y) + depth * 0.48
            and (
                (
                    (float(world_coordinates[index].x) - float(descriptor["center_x"]))
                    / float(descriptor["radius_x"])
                )
                ** 2
                + (
                    (float(world_coordinates[index].z) - float(descriptor["center_z"]))
                    / float(descriptor["radius_z"])
                )
                ** 2
            )
            <= 1.30 ** 2
        }
        if len(candidates) < 24:
            raise RuntimeError(
                f"squint-only fallback found too few {side.upper()} source-skin vertices: "
                f"{len(candidates)}"
            )
        ordered_by_z = sorted(
            candidates,
            key=lambda index: (
                (
                    float(world_coordinates[index].z) - float(descriptor["center_z"])
                )
                / float(descriptor["radius_z"]),
                index,
            ),
        )
        symmetric_count = min(12, len(ordered_by_z) // 2)
        if symmetric_count < 12:
            raise RuntimeError(
                f"squint-only fallback found an incomplete {side.upper()} skin ring: "
                f"candidates={len(ordered_by_z)}"
            )
        lower = set(ordered_by_z[:symmetric_count])
        upper = set(ordered_by_z[-symmetric_count:])
        def normalized_z(index: int) -> float:
            return (
                float(world_coordinates[index].z) - float(descriptor["center_z"])
            ) / float(descriptor["radius_z"])

        if min(normalized_z(index) for index in upper) <= 0.0 or max(
            normalized_z(index) for index in lower
        ) >= 0.0:
            raise RuntimeError(
                f"squint-only fallback found an unbalanced {side.upper()} skin ring"
            )
        candidates = upper.union(lower)
        squint_regions[side] = candidates
        all_squint_indices.update(candidates)
        core_uvs = descriptor["core_uvs"]
        obj[f"eyeball_core_indices_{side}"] = json.dumps(sorted(descriptor["core"]))
        obj[f"eyeball_core_uv_data_{side}"] = json.dumps(
            [
                {"vertex": index, "uvs": core_uvs[index]}
                for index in sorted(descriptor["core"])
            ],
            sort_keys=True,
        )
        obj[f"squint_skin_indices_{side}"] = json.dumps(sorted(candidates))
        obj[f"squint_upper_indices_{side}"] = json.dumps(sorted(upper))
        obj[f"squint_lower_indices_{side}"] = json.dumps(sorted(lower))
        obj[f"squint_center_x_{side}"] = float(descriptor["center_x"])
        obj[f"squint_center_z_{side}"] = float(descriptor["center_z"])
        obj[f"squint_radius_x_{side}"] = float(descriptor["radius_x"])
        obj[f"squint_radius_z_{side}"] = float(descriptor["radius_z"])
        obj[f"squint_eye_weight_zero_{side}"] = True
        obj[f"eyeball_core_excluded_{side}"] = True
        obj[f"eye_region_boundary_indices_{side}"] = json.dumps(sorted(candidates))
        obj[f"eye_region_boundary_basis_coordinates_{side}"] = json.dumps(
            [
                [index, [float(value) for value in mesh.vertices[index].co]]
                for index in sorted(candidates)
            ]
        )
        for index in candidates:
            assignments = [item for item in mesh.vertices[index].groups if item.weight > 1e-8]
            minimum_weight_sum = min(
                minimum_weight_sum,
                sum(float(item.weight) for item in assignments),
            )
            maximum_influences = max(maximum_influences, len(assignments))

    uv_guard_indices = set(
        sorted(set(range(vertex_count_before)).difference(all_squint_indices))[:64]
    )
    uv_guard_data = collect_vertex_uvs(uv_guard_indices)
    if any(not values for values in uv_guard_data.values()):
        raise RuntimeError("squint-only fallback could not establish an untouched UV guard")
    modified_sides = {}
    for side in ("l", "r"):
        skin_uvs = collect_vertex_uvs(squint_regions[side])
        if any(not values for values in skin_uvs.values()):
            raise RuntimeError(
                f"squint-only fallback could not preserve {side.upper()} skin-ring UVs"
            )
        modified_sides[side] = {
            "source_uv_bounds": descriptors[side]["source_uv_bounds"],
            "squint_skin_indices": sorted(squint_regions[side]),
            "squint_skin_uv_data": [
                {"vertex": index, "uvs": skin_uvs[index]}
                for index in sorted(squint_regions[side])
            ],
        }
    obj["true_eyelid_topology"] = False
    obj["eyelid_topology_mode"] = "squint_only_source_skin"
    obj["blink_capability"] = "squint_only"
    obj["eye_region_subdivision_level"] = 0
    obj["eye_region_subdivision_added_vertices"] = 0
    obj["eye_region_boundary_fixed"] = True
    obj["eye_region_uv_preserved"] = True
    obj["eye_region_uv_guard_data"] = json.dumps(
        {
            "layer": active_uv_name,
            "vertices": [
                {"vertex": index, "uvs": uv_guard_data[index]}
                for index in sorted(uv_guard_indices)
            ],
        },
        sort_keys=True,
    )
    obj["eye_region_modified_uv_data"] = json.dumps(
        {
            "layer": active_uv_name,
            "all_finite": True,
            "all_within_source_bounds": True,
            "new_lid_vertex_count": 0,
            "sides": modified_sides,
        },
        sort_keys=True,
    )
    obj["eye_region_custom_data_signature"] = json.dumps(
        {
            "uv_layers": uv_names_before,
            "color_attributes": color_attributes_before,
            "attributes": custom_attributes_before,
        },
        sort_keys=True,
    )
    obj["eye_region_deform_weights_preserved"] = True
    obj["eye_region_deform_evidence"] = json.dumps(
        {
            "original_vertex_count": vertex_count_before,
            "preserved_original_vertex_count": vertex_count_before,
            "new_lid_vertex_count": 0,
            "squint_vertex_count": len(all_squint_indices),
            "minimum_weight_sum": minimum_weight_sum,
            "maximum_influences": maximum_influences,
        },
        sort_keys=True,
    )
    obj["squint_max_closure_fraction"] = 0.12
    return {side: len(indices) for side, indices in squint_regions.items()}

def _source_vertex_luminance(obj: bpy.types.Object) -> dict[int, float]:
    uv_layer = obj.data.uv_layers.active
    if not uv_layer:
        return {}
    source_image = next(
        (
            node.image
            for source_material in obj.data.materials
            if source_material and source_material.use_nodes
            for node in source_material.node_tree.nodes
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


def _shortest_mouth_path(
    adjacency: dict[int, set[int]],
    allowed: set[int],
    world_coordinates: list[Vector],
    luminance: dict[int, float],
    start: int,
    goal: int,
    *,
    center_x: float,
    center_z: float,
    corner_x: float,
    height: float,
) -> list[int]:
    queue: list[tuple[float, int]] = [(0.0, start)]
    distances = {start: 0.0}
    previous: dict[int, int] = {}
    while queue:
        distance, vertex_index = heapq.heappop(queue)
        if vertex_index == goal:
            break
        if distance > distances.get(vertex_index, float("inf")):
            continue
        for neighbor in adjacency[vertex_index]:
            if neighbor not in allowed:
                continue
            world = world_coordinates[neighbor]
            normalized_x = min(1.0, abs(float(world.x) - center_x) / max(corner_x, 1e-6))
            expected_z = center_z + height * 0.0170 * normalized_x ** 1.55
            edge_length = (world - world_coordinates[vertex_index]).length
            texture_cost = luminance.get(neighbor, 1.0) * 0.0045
            curve_cost = abs(float(world.z) - expected_z) * 2.8
            candidate = distance + edge_length + texture_cost + curve_cost
            if candidate < distances.get(neighbor, float("inf")):
                distances[neighbor] = candidate
                previous[neighbor] = vertex_index
                heapq.heappush(queue, (candidate, neighbor))
    if goal not in distances:
        raise RuntimeError("source face retopology could not trace the original mouth groove")
    path = [goal]
    while path[-1] != start:
        path.append(previous[path[-1]])
    path.reverse()
    return path


def _detect_source_mouth_seam(
    obj: bpy.types.Object,
    dimensions: dict[str, Any],
    head_bounds: dict[str, Any],
    *,
    mouth_height_ratio: float,
    mouth_scale: float,
) -> dict[str, Any]:
    width = max(float(head_bounds["width"]), 0.01)
    depth = max(float(head_bounds["depth"]), 0.01)
    height = max(float(dimensions["height"]), 0.01)
    center_x = (float(head_bounds["min"].x) + float(head_bounds["max"].x)) * 0.5
    configured_center_z = float(dimensions["min"].z) + height * mouth_height_ratio
    radius_x = width * 0.170 * max(0.65, min(1.45, float(mouth_scale or 1.0)))
    front_limit = float(head_bounds["min"].y) + depth * 0.27
    world_coordinates = [obj.matrix_world @ vertex.co for vertex in obj.data.vertices]
    luminance = _source_vertex_luminance(obj)
    adjacency = {vertex.index: set() for vertex in obj.data.vertices}
    for edge in obj.data.edges:
        left, right = (int(index) for index in edge.vertices)
        adjacency[left].add(right)
        adjacency[right].add(left)

    allowed = {
        vertex.index
        for vertex in obj.data.vertices
        if abs(float(world_coordinates[vertex.index].x) - center_x) <= radius_x * 1.08
        and configured_center_z - height * 0.012 <= float(world_coordinates[vertex.index].z) <= configured_center_z + height * 0.026
        and float(world_coordinates[vertex.index].y) <= front_limit
    }
    if len(allowed) < 32:
        raise RuntimeError("source face retopology found too few vertices around the original mouth")

    center_candidates = [
        index for index in allowed
        if abs(float(world_coordinates[index].x) - center_x) <= radius_x * 0.24
        and abs(float(world_coordinates[index].z) - configured_center_z) <= height * 0.012
    ]
    if not center_candidates:
        raise RuntimeError("source face retopology could not locate the mouth center")
    center = min(
        center_candidates,
        key=lambda index: luminance.get(index, 1.0) * 0.12
        + abs(float(world_coordinates[index].z) - configured_center_z) * 8.0
        + abs(float(world_coordinates[index].x) - center_x) * 3.0,
    )

    corner_x = radius_x * 0.86
    corner_z = configured_center_z + height * 0.017
    corner_candidates = {
        "left": [
            index for index in allowed
            if float(world_coordinates[index].x) < center_x - radius_x * 0.64
        ],
        "right": [
            index for index in allowed
            if float(world_coordinates[index].x) > center_x + radius_x * 0.64
        ],
    }
    if not all(corner_candidates.values()):
        raise RuntimeError("source face retopology could not locate both mouth corners")

    def corner_score(index: int, sign: float) -> float:
        world = world_coordinates[index]
        return (
            luminance.get(index, 1.0) * 0.060
            + abs(float(world.x) - (center_x + sign * corner_x)) * 1.8
            + abs(float(world.z) - corner_z) * 2.6
        )

    left = min(corner_candidates["left"], key=lambda index: corner_score(index, -1.0))
    right = min(corner_candidates["right"], key=lambda index: corner_score(index, 1.0))
    left_path = _shortest_mouth_path(
        adjacency, allowed, world_coordinates, luminance, left, center,
        center_x=center_x, center_z=configured_center_z, corner_x=corner_x, height=height,
    )
    right_path = _shortest_mouth_path(
        adjacency, allowed, world_coordinates, luminance, center, right,
        center_x=center_x, center_z=configured_center_z, corner_x=corner_x, height=height,
    )
    path = left_path + right_path[1:]
    if len(path) < 9 or len(path) != len(set(path)):
        raise RuntimeError("source face retopology produced an unstable mouth-seam path")
    return {
        "path": path,
        "centerX": center_x,
        "centerZ": configured_center_z,
        "surfaceY": sum(float(world_coordinates[index].y) for index in path) / len(path),
        "radiusX": max(abs(float(world_coordinates[index].x) - center_x) for index in path),
    }


def _mouth_boundary_vertices(
    obj: bpy.types.Object,
    *,
    center_x: float,
    center_z: float,
    radius_x: float,
    height: float,
) -> tuple[set[int], set[int], int]:
    edge_faces: dict[tuple[int, int], list[bpy.types.MeshPolygon]] = {}
    for polygon in obj.data.polygons:
        indices = [int(index) for index in polygon.vertices]
        for offset, left in enumerate(indices):
            right = indices[(offset + 1) % len(indices)]
            edge_faces.setdefault(tuple(sorted((left, right))), []).append(polygon)
    upper: set[int] = set()
    lower: set[int] = set()
    boundary_edge_count = 0
    for edge in obj.data.edges:
        key = tuple(sorted((int(edge.vertices[0]), int(edge.vertices[1]))))
        faces = edge_faces.get(key, [])
        if len(faces) != 1:
            continue
        points = [obj.matrix_world @ obj.data.vertices[index].co for index in key]
        midpoint = (points[0] + points[1]) * 0.5
        if (
            abs(float(midpoint.x) - center_x) > radius_x * 1.08
            or float(midpoint.z) < center_z - height * 0.012
            or float(midpoint.z) > center_z + height * 0.027
        ):
            continue
        boundary_edge_count += 1
        polygon_center = obj.matrix_world @ faces[0].center
        target = upper if float(polygon_center.z) >= float(midpoint.z) else lower
        target.update(key)
    return upper, lower, boundary_edge_count


def _bind_internal_face_object(
    obj: bpy.types.Object,
    armature: bpy.types.Object,
    weights: dict[str, dict[int, float]],
) -> None:
    for bone_name, assignments in weights.items():
        group = obj.vertex_groups.new(name=bone_name)
        for vertex_index, weight in assignments.items():
            if weight > 1e-6:
                group.add([vertex_index], float(weight), "REPLACE")
    modifier = obj.modifiers.new("IP_Face_Armature", "ARMATURE")
    modifier.object = armature
    modifier.use_vertex_groups = True
    obj.parent = armature
    obj.matrix_parent_inverse = armature.matrix_world.inverted()


def _oval_disk_mesh(name: str, radius_x: float, radius_z: float) -> bpy.types.Object:
    segments = 32
    rings = 4
    vertices = [(0.0, radius_z * 0.18, 0.0)]
    for ring in range(1, rings + 1):
        ratio = ring / rings
        for index in range(segments):
            angle = index * math.tau / segments
            vertices.append(
                (
                    math.cos(angle) * radius_x * ratio,
                    radius_z * 0.18 * (1.0 - ratio * ratio),
                    math.sin(angle) * radius_z * ratio,
                )
            )
    faces: list[tuple[int, ...]] = [
        (0, 1 + index, 1 + (index + 1) % segments) for index in range(segments)
    ]
    for ring in range(1, rings):
        inner = 1 + (ring - 1) * segments
        outer = 1 + ring * segments
        for index in range(segments):
            following = (index + 1) % segments
            faces.append((inner + index, outer + index, outer + following, inner + following))
    mesh = bpy.data.meshes.new(f"{name}_Mesh")
    mesh.from_pydata(vertices, [], faces)
    obj = bpy.data.objects.new(name, mesh)
    bpy.context.collection.objects.link(obj)
    return obj


def _ellipsoid_cluster_mesh(
    name: str,
    ellipsoids: list[tuple[tuple[float, float, float], tuple[float, float, float]]],
) -> bpy.types.Object:
    u_segments = 12
    v_segments = 7
    vertices: list[tuple[float, float, float]] = []
    faces: list[tuple[int, ...]] = []
    for center, scale in ellipsoids:
        offset = len(vertices)
        for latitude in range(v_segments + 1):
            phi = -math.pi * 0.5 + math.pi * latitude / v_segments
            for longitude in range(u_segments):
                theta = math.tau * longitude / u_segments
                vertices.append(
                    (
                        center[0] + math.cos(phi) * math.cos(theta) * scale[0],
                        center[1] + math.cos(phi) * math.sin(theta) * scale[1],
                        center[2] + math.sin(phi) * scale[2],
                    )
                )
        for latitude in range(v_segments):
            for longitude in range(u_segments):
                following = (longitude + 1) % u_segments
                a = offset + latitude * u_segments + longitude
                b = offset + latitude * u_segments + following
                c = offset + (latitude + 1) * u_segments + following
                d = offset + (latitude + 1) * u_segments + longitude
                faces.append((a, b, c, d))
    mesh = bpy.data.meshes.new(f"{name}_Mesh")
    mesh.from_pydata(vertices, [], faces)
    for polygon in mesh.polygons:
        polygon.use_smooth = True
    obj = bpy.data.objects.new(name, mesh)
    bpy.context.collection.objects.link(obj)
    return obj


def create_integrated_oral_interior(
    source_face: bpy.types.Object,
    armature: bpy.types.Object,
    bone_map: dict[str, str],
    dimensions: dict[str, Any],
) -> dict[str, bpy.types.Object]:
    role_names = {
        "IP_OralCavity": "oral_cavity",
        "IP_UpperTeeth": "upper_teeth",
        "IP_LowerTeeth": "lower_teeth",
        "IP_Tongue": "tongue",
    }
    existing: dict[str, bpy.types.Object] = {}
    for obj in bpy.context.scene.objects:
        if obj.type != "MESH":
            continue
        role = str(obj.get("ip_face_topology_role") or "")
        if not role:
            base_name = obj.name.split(".")[0]
            role = role_names.get(base_name, "")
        if role:
            obj["ip_face_topology_role"] = role
            existing[role] = obj
    roles = ("oral_cavity", "upper_teeth", "lower_teeth", "tongue")
    if all(role in existing for role in roles):
        return {role: existing[role] for role in roles}

    center_x = float(source_face["mouth_center_x"])
    center_z = float(source_face["mouth_center_z"])
    surface_y = float(source_face["mouth_surface_y"])
    radius_x = float(source_face["mouth_radius_x"])
    height = float(dimensions["height"])
    depth = float(source_face.get("head_region_depth", dimensions["depth"]))
    origin = (center_x, surface_y + depth * 0.032, center_z + height * 0.002)

    cavity = _oval_disk_mesh("IP_OralCavity", radius_x * 0.96, height * 0.026)
    cavity.location = origin
    cavity.data.materials.append(material("IP_OralCavity_Material", (0.030, 0.004, 0.006, 1.0), False))
    cavity["ip_face_topology_role"] = "oral_cavity"
    _bind_internal_face_object(
        cavity,
        armature,
        {bone_map["head"]: {vertex.index: 1.0 for vertex in cavity.data.vertices}},
    )

    tooth_x = [-0.62, -0.38, -0.13, 0.13, 0.38, 0.62]
    tooth_scale = (radius_x * 0.105, depth * 0.011, height * 0.0046)
    upper = _ellipsoid_cluster_mesh(
        "IP_UpperTeeth",
        [
            ((value * radius_x, depth * 0.008, height * (0.0046 + abs(value) * 0.0015)), tooth_scale)
            for value in tooth_x
        ],
    )
    upper.location = origin
    upper.data.materials.append(material("IP_Teeth_Material", (0.92, 0.82, 0.66, 1.0), False))
    upper["ip_face_topology_role"] = "upper_teeth"
    _bind_internal_face_object(
        upper,
        armature,
        {bone_map["head"]: {vertex.index: 1.0 for vertex in upper.data.vertices}},
    )

    lower = _ellipsoid_cluster_mesh(
        "IP_LowerTeeth",
        [
            ((value * radius_x, depth * 0.010, -height * (0.0090 + abs(value) * 0.0010)), tooth_scale)
            for value in tooth_x
        ],
    )
    lower.location = origin
    lower.data.materials.append(bpy.data.materials["IP_Teeth_Material"])
    lower["ip_face_topology_role"] = "lower_teeth"
    _bind_internal_face_object(
        lower,
        armature,
        {bone_map["jaw"]: {vertex.index: 1.0 for vertex in lower.data.vertices}},
    )

    tongue = _ellipsoid_cluster_mesh(
        "IP_Tongue",
        [((0.0, depth * 0.004, -height * 0.0105), (radius_x * 0.52, depth * 0.055, height * 0.0065))],
    )
    tongue.location = origin
    tongue.data.materials.append(material("IP_Tongue_Material", (0.34, 0.055, 0.065, 1.0), False))
    tongue["ip_face_topology_role"] = "tongue"
    local_ys = [float(vertex.co.y) for vertex in tongue.data.vertices]
    minimum_y, maximum_y = min(local_ys), max(local_ys)
    tongue_weights = {bone_map[role]: {} for role in ("tongue_1", "tongue_2", "tongue_3")}
    for vertex in tongue.data.vertices:
        amount = (float(vertex.co.y) - minimum_y) / max(maximum_y - minimum_y, 1e-6)
        centers = (0.12, 0.50, 0.88)
        raw = [max(0.0, 1.0 - abs(amount - center) / 0.46) for center in centers]
        total = sum(raw) or 1.0
        for role, value in zip(("tongue_1", "tongue_2", "tongue_3"), raw):
            tongue_weights[bone_map[role]][vertex.index] = value / total
    _bind_internal_face_object(tongue, armature, tongue_weights)
    return {
        "oral_cavity": cavity,
        "upper_teeth": upper,
        "lower_teeth": lower,
        "tongue": tongue,
    }


def retopologize_source_face(
    objects: list[bpy.types.Object],
    dimensions: dict[str, Any],
    armature: bpy.types.Object,
    bone_map: dict[str, str],
    *,
    mouth_height_ratio: float,
    mouth_scale: float,
) -> dict[str, bpy.types.Object]:
    existing_face = next(
        (obj for obj in objects if obj.type == "MESH" and obj.get("integrated_mouth_seam")),
        None,
    )
    head_bounds = weighted_region_bounds(objects, "head", minimum_weight=0.25)
    if not head_bounds:
        raise RuntimeError("source face retopology requires a Head-weighted mesh region")
    if existing_face:
        validate_task6_face_metadata(existing_face)
        tune_source_pbr_materials(existing_face)
        return {"mouth": existing_face, **create_integrated_oral_interior(existing_face, armature, bone_map, dimensions)}

    exported_face = next(
        (
            obj for obj in objects
            if obj.type == "MESH"
            and obj.data.shape_keys
            and obj.data.shape_keys.key_blocks.get("Mouth_A")
            and obj.data.shape_keys.key_blocks.get("Eye_Blink.L")
        ),
        None,
    )
    exported_internals = {
        obj.name.split(".")[0]
        for obj in bpy.context.scene.objects
        if obj.type == "MESH"
    }
    required_internal_names = {"IP_OralCavity", "IP_UpperTeeth", "IP_LowerTeeth", "IP_Tongue"}
    if exported_face:
        validate_task6_face_metadata(exported_face)
        if not exported_face.get("integrated_mouth_seam"):
            raise RuntimeError(
                "Task 6 integrated mouth metadata is incomplete (integrated_mouth_seam); "
                "rebuild master from source FBX"
            )
        missing_internals = sorted(required_internal_names.difference(exported_internals))
        if missing_internals:
            raise RuntimeError(
                "Task 6 integrated face internals are incomplete "
                f"({', '.join(missing_internals)}); rebuild master from source FBX"
            )
        tune_source_pbr_materials(exported_face)
        return {
            "mouth": exported_face,
            **create_integrated_oral_interior(exported_face, armature, bone_map, dimensions),
        }

    eligible = head_bounds["indicesByObject"]
    candidates = [
        obj for obj in objects
        if obj.type == "MESH" and len(eligible.get(obj.name, set())) >= 24
    ]
    if not candidates:
        raise RuntimeError("source face retopology could not identify the original head mesh")
    source_face = max(candidates, key=lambda obj: len(eligible.get(obj.name, set())))
    tune_source_pbr_materials(source_face)
    if source_face.data.shape_keys:
        raise RuntimeError("source face topology must be integrated before creating shape keys")

    seam = _detect_source_mouth_seam(
        source_face,
        dimensions,
        head_bounds,
        mouth_height_ratio=mouth_height_ratio,
        mouth_scale=mouth_scale,
    )
    edge_lookup = {
        tuple(sorted((int(edge.vertices[0]), int(edge.vertices[1])))): edge
        for edge in source_face.data.edges
    }
    seam_edges = [
        edge_lookup.get(tuple(sorted((left, right))))
        for left, right in zip(seam["path"], seam["path"][1:])
    ]
    if any(edge is None for edge in seam_edges):
        raise RuntimeError("source face retopology mouth path contains a missing edge")

    mesh = source_face.data
    bm = bmesh.new()
    bm.from_mesh(mesh)
    bm.verts.ensure_lookup_table()
    bm.edges.ensure_lookup_table()
    bm_edge_lookup = {
        tuple(sorted((edge.verts[0].index, edge.verts[1].index))): edge
        for edge in bm.edges
    }
    bmesh.ops.split_edges(
        bm,
        edges=[
            bm_edge_lookup[tuple(sorted((left, right)))]
            for left, right in zip(seam["path"], seam["path"][1:])
        ],
    )
    bm.normal_update()
    bm.to_mesh(mesh)
    bm.free()
    mesh.update(calc_edges=True)
    refine_integrated_eye_regions(source_face, dimensions, head_bounds, bone_map)

    upper, lower, boundary_edge_count = _mouth_boundary_vertices(
        source_face,
        center_x=float(seam["centerX"]),
        center_z=float(seam["centerZ"]),
        radius_x=float(seam["radiusX"]),
        height=float(dimensions["height"]),
    )
    if len(upper) < 8 or len(lower) < 8:
        raise RuntimeError(
            f"source face retopology produced incomplete lip boundaries: upper={len(upper)}, lower={len(lower)}"
        )
    source_face["source_face_topology"] = "integrated_source_retopology"
    source_face["integrated_mouth_seam"] = True
    source_face["mouth_seam_edge_count"] = len(seam["path"]) - 1
    source_face["mouth_boundary_edge_count"] = boundary_edge_count
    source_face["mouth_upper_boundary_count"] = len(upper)
    source_face["mouth_lower_boundary_count"] = len(lower)
    source_face["mouth_upper_boundary_indices"] = json.dumps(sorted(upper))
    source_face["mouth_lower_boundary_indices"] = json.dumps(sorted(lower))
    source_face["mouth_center_x"] = float(seam["centerX"])
    source_face["mouth_center_z"] = float(seam["centerZ"])
    source_face["mouth_surface_y"] = float(seam["surfaceY"])
    source_face["mouth_radius_x"] = float(seam["radiusX"])
    source_face["head_region_width"] = float(head_bounds["width"])
    source_face["head_region_depth"] = float(head_bounds["depth"])
    source_face["source_texture_face_preserved"] = True
    source_face["facial_topology_mode"] = "source_retopology"
    return {
        "mouth": source_face,
        **create_integrated_oral_interior(source_face, armature, bone_map, dimensions),
    }


def create_source_mesh_visemes(
    objects: list[bpy.types.Object],
    dimensions: dict[str, Any],
    *,
    mouth_height_ratio: float,
    mouth_scale: float,
) -> dict[str, bpy.types.Object]:
    """Create visemes by deforming the source muzzle instead of overlaying a new mouth."""
    head_bounds = weighted_region_bounds(objects, "head")
    width = max(float(head_bounds["width"] if head_bounds else dimensions["width"]), 0.01)
    depth = max(float(head_bounds["depth"] if head_bounds else dimensions["depth"]), 0.01)
    height = max(float(dimensions["height"]), 0.01)
    region_min = head_bounds["min"] if head_bounds else dimensions["min"]
    region_max = head_bounds["max"] if head_bounds else dimensions["max"]
    front_y = float(region_min.y)
    center_x = (float(region_min.x) + float(region_max.x)) * 0.5
    configured_center_z = float(dimensions["min"].z) + height * mouth_height_ratio
    scale = max(0.65, min(1.45, float(mouth_scale or 1.0)))
    search_radius_x = width * 0.170 * scale
    search_z_min = configured_center_z - height * 0.025 * scale
    search_z_max = configured_center_z + height * 0.020 * scale
    front_limit = front_y + depth * 0.24
    eligible_indices = head_bounds["indicesByObject"] if head_bounds else {}

    world_coordinates = {
        obj.name: [obj.matrix_world @ vertex.co for vertex in obj.data.vertices]
        for obj in objects
        if obj.type == "MESH"
    }

    candidates = [
        (
            sum(
                abs(float(world.x) - center_x) <= search_radius_x
                and search_z_min <= float(world.z) <= search_z_max
                and float(world.y) <= front_limit
                for index, world in enumerate(world_coordinates[obj.name])
                if not eligible_indices or index in eligible_indices.get(obj.name, set())
            ),
            obj,
        )
        for obj in objects
        if obj.type == "MESH"
    ]
    geometric_count, mouth = max(candidates, default=(0, None), key=lambda item: item[0])
    if mouth is None or geometric_count < 16:
        raise RuntimeError("source_mesh_visemes could not identify a deformable source-mouth region")

    if mouth.data.shape_keys:
        existing_names = {key.name for key in mouth.data.shape_keys.key_blocks}
        if {"Mouth_Rest", "Mouth_A", "Mouth_E", "Mouth_O"}.issubset(existing_names):
            if mouth.get("integrated_mouth_seam") or {"Eye_Blink.L", "Eye_Blink.R"}.intersection(
                existing_names
            ):
                validate_task6_face_metadata(mouth)
            mouth["source_mouth_replacement"] = False
            mouth["existing_viseme_mouth_preserved"] = True
            install_viseme_api()
            return {"mouth": mouth}
        raise RuntimeError("source mouth mesh already has unrelated shape keys")

    weights: dict[int, float] = {}
    uv_layer = mouth.data.uv_layers.active
    source_image = None
    for source_material in mouth.data.materials:
        if not source_material or not source_material.use_nodes:
            continue
        for node in source_material.node_tree.nodes:
            image = getattr(node, "image", None)
            if node.type == "TEX_IMAGE" and image and image.colorspace_settings.name == "sRGB":
                source_image = image
                break
        if source_image:
            break

    seeds: set[int] = set()
    if uv_layer and source_image and int(source_image.size[0]) > 0 and int(source_image.size[1]) > 0:
        sample = source_image.copy()
        try:
            sample.scale(512, 512)
            sample_width, sample_height = int(sample.size[0]), int(sample.size[1])
            pixels = array("f", [0.0]) * (sample_width * sample_height * 4)
            sample.pixels.foreach_get(pixels)
            vertex_luminance: dict[int, float] = {}
            for polygon in mouth.data.polygons:
                for loop_index in polygon.loop_indices:
                    vertex_index = mouth.data.loops[loop_index].vertex_index
                    uv = uv_layer.data[loop_index].uv
                    pixel_x = max(0, min(sample_width - 1, int((float(uv.x) % 1.0) * (sample_width - 1))))
                    pixel_y = max(0, min(sample_height - 1, int((float(uv.y) % 1.0) * (sample_height - 1))))
                    offset = (pixel_y * sample_width + pixel_x) * 4
                    luminance = pixels[offset] * 0.2126 + pixels[offset + 1] * 0.7152 + pixels[offset + 2] * 0.0722
                    vertex_luminance[vertex_index] = min(vertex_luminance.get(vertex_index, 1.0), luminance)
            seeds = {
                vertex.index
                for vertex in mouth.data.vertices
                if abs(float(world_coordinates[mouth.name][vertex.index].x) - center_x) <= search_radius_x
                and search_z_min <= float(world_coordinates[mouth.name][vertex.index].z) <= search_z_max
                and float(world_coordinates[mouth.name][vertex.index].y) <= front_limit
                and (not eligible_indices or vertex.index in eligible_indices.get(mouth.name, set()))
                and vertex_luminance.get(vertex.index, 1.0) < 0.46
            }
        finally:
            bpy.data.images.remove(sample)

    if uv_layer and source_image and len(seeds) < 6:
        raise RuntimeError(
            "source_mesh_visemes could not find the original textured mouth; check mouthHeightRatio"
        )

    if len(seeds) >= 6:
        adjacency = {vertex.index: set() for vertex in mouth.data.vertices}
        for edge in mouth.data.edges:
            left, right = edge.vertices
            adjacency[left].add(right)
            adjacency[right].add(left)
        weights.update({vertex_index: 1.0 for vertex_index in seeds})
        frontier = set(seeds)
        for ring_weight in (0.42,):
            neighbors = {neighbor for vertex_index in frontier for neighbor in adjacency[vertex_index]}
            neighbors = {
                vertex_index
                for vertex_index in neighbors
                if vertex_index not in weights
                and abs(float(world_coordinates[mouth.name][vertex_index].x) - center_x) <= search_radius_x
                and search_z_min <= float(world_coordinates[mouth.name][vertex_index].z) <= search_z_max
                and float(world_coordinates[mouth.name][vertex_index].y) <= front_limit
            }
            weights.update({vertex_index: ring_weight for vertex_index in neighbors})
            frontier = neighbors
    else:
        radius_z = max(height * 0.022 * scale, 1e-6)
        for vertex in mouth.data.vertices:
            world = world_coordinates[mouth.name][vertex.index]
            normalized_x = (float(world.x) - center_x) / max(search_radius_x, 1e-6)
            normalized_z = (float(world.z) - configured_center_z) / radius_z
            radial = max(0.0, 1.0 - normalized_x * normalized_x - normalized_z * normalized_z)
            if radial > 0.0 and float(world.y) <= front_limit:
                if eligible_indices and vertex.index not in eligible_indices.get(mouth.name, set()):
                    continue
                weights[vertex.index] = radial

    integrated_topology = bool(mouth.get("integrated_mouth_seam"))
    upper_weights: dict[int, float] = {}
    lower_weights: dict[int, float] = {}
    if integrated_topology:
        try:
            upper_boundary = set(json.loads(str(mouth["mouth_upper_boundary_indices"])))
            lower_boundary = set(json.loads(str(mouth["mouth_lower_boundary_indices"])))
        except (KeyError, TypeError, json.JSONDecodeError) as exc:
            raise RuntimeError("integrated source mouth is missing valid lip-boundary indices") from exc
        adjacency = {vertex.index: set() for vertex in mouth.data.vertices}
        for edge in mouth.data.edges:
            left, right = (int(index) for index in edge.vertices)
            adjacency[left].add(right)
            adjacency[right].add(left)

        def grow_boundary(boundary: set[int]) -> dict[int, float]:
            region = {index: 1.0 for index in boundary}
            frontier = set(boundary)
            for ring_weight in (0.56, 0.22):
                neighbors = {
                    neighbor
                    for vertex_index in frontier
                    for neighbor in adjacency[vertex_index]
                    if neighbor not in region
                }
                neighbors = {
                    index for index in neighbors
                    if abs(float(world_coordinates[mouth.name][index].x) - center_x) <= search_radius_x * 1.10
                    and configured_center_z - height * 0.020 <= float(world_coordinates[mouth.name][index].z) <= configured_center_z + height * 0.035
                    and float(world_coordinates[mouth.name][index].y) <= front_limit
                }
                region.update({index: ring_weight for index in neighbors})
                frontier = neighbors
            return region

        upper_weights = grow_boundary(upper_boundary)
        lower_weights = grow_boundary(lower_boundary)
        weights = {
            index: max(upper_weights.get(index, 0.0), lower_weights.get(index, 0.0))
            for index in set(upper_weights).union(lower_weights)
        }

    if len(weights) < 16:
        raise RuntimeError("source_mesh_visemes found too few original-mouth vertices")
    center_z = (
        float(mouth.get("mouth_center_z", configured_center_z))
        if integrated_topology
        else sum(float(world_coordinates[mouth.name][index].z) * weight for index, weight in weights.items()) / sum(weights.values())
    )
    radius_x = (
        float(mouth.get("mouth_radius_x", search_radius_x))
        if integrated_topology
        else max(abs(float(world_coordinates[mouth.name][index].x) - center_x) for index in weights) or search_radius_x
    )

    basis = mouth.shape_key_add(name="Basis", from_mix=False)
    shape_names = (
        "Mouth_Rest",
        "Mouth_A",
        "Mouth_E",
        "Mouth_O",
        "Mouth_U",
        "Mouth_MBP",
        "Mouth_Smile",
        "Mouth_Frown",
        "Mouth_Surprise",
    )
    keys = {name: mouth.shape_key_add(name=name, from_mix=False) for name in shape_names}

    object_to_world = mouth.matrix_world.copy()
    world_to_object = object_to_world.inverted()

    def deform(
        shape_name: str,
        original: Vector,
        weight: float,
        upper_weight: float,
        lower_weight: float,
    ) -> Vector:
        co = object_to_world @ original
        if shape_name == "Mouth_Rest" or weight <= 0.0:
            return original.copy()
        relative_x = float(co.x) - center_x
        normalized_x = max(-1.0, min(1.0, relative_x / max(radius_x, 1e-6)))
        center_weight = max(0.0, 1.0 - abs(normalized_x)) ** 1.45
        corner_weight = abs(normalized_x) ** 1.35

        if integrated_topology:
            if shape_name == "Mouth_A":
                co.x -= relative_x * 0.025 * weight
                co.z += height * 0.0032 * upper_weight * center_weight
                co.z -= height * 0.0125 * lower_weight * center_weight
                co.y += depth * 0.0040 * max(upper_weight, lower_weight) * center_weight
            elif shape_name == "Mouth_E":
                co.x += math.copysign(
                    width * 0.0028 * weight * (0.28 + corner_weight * 0.72),
                    relative_x or 1.0,
                )
                co.z += height * 0.0010 * upper_weight * center_weight
                co.z -= height * 0.0034 * lower_weight * center_weight
            elif shape_name == "Mouth_O":
                co.x -= relative_x * 0.24 * weight
                co.z += height * 0.0046 * upper_weight * center_weight
                co.z -= height * 0.0098 * lower_weight * center_weight
                co.y += depth * 0.0045 * max(upper_weight, lower_weight) * center_weight
            elif shape_name == "Mouth_U":
                co.x -= relative_x * 0.34 * weight
                co.z += height * 0.0034 * upper_weight * center_weight
                co.z -= height * 0.0072 * lower_weight * center_weight
                co.y += depth * 0.0035 * max(upper_weight, lower_weight) * center_weight
            elif shape_name == "Mouth_MBP":
                co.x -= relative_x * 0.008 * weight
                co.z += (center_z - float(co.z)) * 0.08 * max(upper_weight, lower_weight) * center_weight
            elif shape_name == "Mouth_Smile":
                co.x += math.copysign(width * 0.0018 * weight * corner_weight, relative_x or 1.0)
                co.z += height * 0.0022 * max(upper_weight, lower_weight) * corner_weight
            elif shape_name == "Mouth_Frown":
                co.z -= height * 0.0038 * weight * (corner_weight - 0.08)
            elif shape_name == "Mouth_Surprise":
                co.x -= relative_x * 0.36 * weight
                co.z += height * 0.0060 * upper_weight * center_weight
                co.z -= height * 0.0140 * lower_weight * center_weight
                co.y += depth * 0.0050 * max(upper_weight, lower_weight) * center_weight
            return world_to_object @ co

        if shape_name == "Mouth_A":
            co.x -= relative_x * 0.025 * weight
            co.z -= height * 0.0065 * weight * center_weight
            co.y -= depth * 0.0025 * weight * center_weight
        elif shape_name == "Mouth_E":
            co.x += math.copysign(width * 0.0035 * weight * (0.35 + corner_weight * 0.65), relative_x or 1.0)
            co.z += (center_z - float(co.z)) * 0.06 * weight
        elif shape_name == "Mouth_O":
            co.x -= relative_x * 0.070 * weight
            co.z -= height * 0.0050 * weight * center_weight
            co.y -= depth * 0.0020 * weight * center_weight
        elif shape_name == "Mouth_U":
            co.x -= relative_x * 0.10 * weight
            co.z -= height * 0.0035 * weight * center_weight
            co.y -= depth * 0.0015 * weight * center_weight
        elif shape_name == "Mouth_MBP":
            co.z += (center_z - float(co.z)) * 0.05 * weight
            co.x -= relative_x * 0.010 * weight
        elif shape_name == "Mouth_Smile":
            co.x += math.copysign(width * 0.0025 * weight * corner_weight, relative_x or 1.0)
            co.z += height * 0.0030 * weight * (corner_weight - 0.12)
        elif shape_name == "Mouth_Frown":
            co.z -= height * 0.0032 * weight * (corner_weight - 0.10)
        elif shape_name == "Mouth_Surprise":
            co.x -= relative_x * 0.11 * weight
            co.z -= height * 0.0075 * weight * center_weight
            co.y -= depth * 0.0030 * weight * center_weight
        return world_to_object @ co

    affected = 0
    for index, basis_point in enumerate(basis.data):
        weight = weights.get(index, 0.0)
        if weight > 0.0:
            affected += 1
        for shape_name, key in keys.items():
            key.data[index].co = deform(
                shape_name,
                basis_point.co,
                weight,
                upper_weights.get(index, 0.0),
                lower_weights.get(index, 0.0),
            )

    if not integrated_topology:
        color_attribute = mouth.data.color_attributes.new(
            name="IP_Mouth_Mask",
            type="FLOAT_COLOR",
            domain="CORNER",
        )
        vertical_radius = max(height * 0.020 * scale, 1e-6)
        for polygon in mouth.data.polygons:
            for loop_index in polygon.loop_indices:
                vertex_index = mouth.data.loops[loop_index].vertex_index
                co = world_coordinates[mouth.name][vertex_index]
                vertical_mask = max(0.0, 1.0 - abs(float(co.z) - center_z) / vertical_radius)
                horizontal_mask = max(0.0, 1.0 - abs(float(co.x) - center_x) / max(radius_x, 1e-6))
                mask = (weights.get(vertex_index, 0.0) ** 1.6) * vertical_mask * (0.35 + horizontal_mask * 0.65)
                color_attribute.data[loop_index].color = (mask, mask, mask, 1.0)

        shape_keys = mouth.data.shape_keys
        for source_material in mouth.data.materials:
            if not source_material or not source_material.use_nodes:
                continue
            nodes = source_material.node_tree.nodes
            links = source_material.node_tree.links
            bsdf = next((node for node in nodes if node.type == "BSDF_PRINCIPLED"), None)
            if not bsdf or "Base Color" not in bsdf.inputs:
                continue
            mix = nodes.get("IP_Mouth_Open_Mix") or nodes.new("ShaderNodeMixRGB")
            mix.name = "IP_Mouth_Open_Mix"
            mix.label = "Original texture + source-mouth depth"
            mix.blend_type = "MIX"
            mix.inputs[2].default_value = (0.010, 0.0035, 0.0015, 1.0)

            base_color = bsdf.inputs["Base Color"]
            original_link = base_color.links[0] if base_color.links else None
            if original_link and original_link.from_node != mix:
                links.new(original_link.from_socket, mix.inputs[1])
                links.remove(original_link)
            elif not original_link:
                mix.inputs[1].default_value = base_color.default_value

            vertex_color = nodes.get("IP_Mouth_Mask_Attribute") or nodes.new("ShaderNodeVertexColor")
            vertex_color.name = "IP_Mouth_Mask_Attribute"
            vertex_color.layer_name = "IP_Mouth_Mask"
            open_value = nodes.get("IP_Mouth_Open_Value") or nodes.new("ShaderNodeValue")
            open_value.name = "IP_Mouth_Open_Value"
            multiply = nodes.get("IP_Mouth_Open_Multiply") or nodes.new("ShaderNodeMath")
            multiply.name = "IP_Mouth_Open_Multiply"
            multiply.operation = "MULTIPLY"
            links.new(vertex_color.outputs["Color"], multiply.inputs[0])
            links.new(open_value.outputs[0], multiply.inputs[1])
            links.new(multiply.outputs[0], mix.inputs[0])
            links.new(mix.outputs[0], base_color)

            driver = open_value.outputs[0].driver_add("default_value").driver
            expressions = {
                "a": ("Mouth_A", 0.34),
                "e": ("Mouth_E", 0.12),
                "o": ("Mouth_O", 0.38),
                "u": ("Mouth_U", 0.24),
                "s": ("Mouth_Surprise", 0.45),
            }
            for variable_name, (shape_name, _) in expressions.items():
                variable = driver.variables.new()
                variable.name = variable_name
                variable.type = "SINGLE_PROP"
                variable.targets[0].id_type = "KEY"
                variable.targets[0].id = shape_keys
                variable.targets[0].data_path = f'key_blocks["{shape_name}"].value'
            driver.expression = "min(1.0, " + " + ".join(
                f"{variable_name}*{strength}" for variable_name, (_, strength) in expressions.items()
            ) + ")"

    mouth["mouth_style"] = "source_mesh"
    mouth["source_mouth_removed"] = False
    mouth["source_mouth_replacement"] = False
    mouth["source_mouth_deformed"] = True
    mouth["mouth_vertex_count"] = affected
    mouth["source_mouth_material_driver"] = not integrated_topology
    mouth["mouth_height_ratio"] = float(mouth_height_ratio)
    mouth["mouth_scale"] = scale
    mouth["mouth_center_x"] = center_x
    mouth["mouth_center_z"] = center_z
    mouth["mouth_radius_x"] = radius_x
    mouth["head_region_width"] = width
    mouth["head_region_depth"] = depth
    if integrated_topology:
        mouth["mouth_corner_lateral_limit"] = width * 0.0035
        mouth["mouth_corner_falloff_rings"] = 2
        mouth["mouth_closed_smile_mode"] = "restrained_source_seam"
    install_viseme_api()
    return {"mouth": mouth}


def add_rich_source_face_shapes(
    face_mesh: bpy.types.Object,
    dimensions: dict[str, Any],
) -> dict[str, int]:
    """Add restrained eye, brow, and cheek shapes to the original textured mesh."""
    shape_keys = face_mesh.data.shape_keys
    if not shape_keys or not shape_keys.key_blocks.get("Basis"):
        raise RuntimeError("rich source-face controls require an existing Basis shape key")
    if face_mesh.get("true_eyelid_topology"):
        raise RuntimeError(
            "generated full-blink topology is no longer supported; "
            "rebuild master from source FBX for blinkCapability=squint_only"
        )

    shape_names = (
        "Eye_Squint.L",
        "Eye_Squint.R",
        "Eye_Wide.L",
        "Eye_Wide.R",
        "Eye_Look_Left",
        "Eye_Look_Right",
        "Brow_Raise.L",
        "Brow_Raise.R",
        "Brow_Furrow.L",
        "Brow_Furrow.R",
        "Cheek_Smile.L",
        "Cheek_Smile.R",
        "Cheek_Puff.L",
        "Cheek_Puff.R",
        "Nose_Flare.L",
        "Nose_Flare.R",
    )
    if all(shape_keys.key_blocks.get(name) for name in shape_names):
        validate_task6_face_metadata(face_mesh)
        face_mesh["facial_detail_mode"] = "rich_source_mesh"
        return {
            name: int(face_mesh.get(f"affected_{name}", 0))
            for name in shape_names
        }

    head_bounds = weighted_region_bounds([face_mesh], "head")
    width = max(float(head_bounds["width"] if head_bounds else dimensions["width"]), 0.01)
    depth = max(float(head_bounds["depth"] if head_bounds else dimensions["depth"]), 0.01)
    height = max(float(dimensions["height"]), 0.01)
    min_v = head_bounds["min"] if head_bounds else dimensions["min"]
    max_v = head_bounds["max"] if head_bounds else dimensions["max"]
    center_x = (float(min_v.x) + float(max_v.x)) * 0.5
    front_limit = float(min_v.y) + depth * 0.36
    region_height = max(float(max_v.z - min_v.z), height * 0.1)
    eye_center_z = float(min_v.z) + region_height * 0.46
    eye_offset_x = width * 0.185
    brow_center_z = float(min_v.z) + region_height * 0.66
    cheek_center_z = float(min_v.z) + region_height * 0.22
    object_to_world = face_mesh.matrix_world.copy()
    world_to_object = object_to_world.inverted()
    basis = shape_keys.key_blocks["Basis"]
    squint_regions: dict[str, dict[str, Any]] = {}
    if face_mesh.get("blink_capability") == "squint_only":
        for suffix in ("L", "R"):
            side = suffix.lower()
            try:
                skin = set(json.loads(str(face_mesh[f"squint_skin_indices_{side}"])))
                upper = set(json.loads(str(face_mesh[f"squint_upper_indices_{side}"])))
                lower = set(json.loads(str(face_mesh[f"squint_lower_indices_{side}"])))
                eye_core = set(json.loads(str(face_mesh[f"eyeball_core_indices_{side}"])))
            except (KeyError, TypeError, json.JSONDecodeError) as exc:
                raise RuntimeError(f"squint-only {suffix} metadata is invalid") from exc
            if upper.union(lower) != skin or skin.intersection(eye_core):
                raise RuntimeError(f"squint-only {suffix} skin ring is invalid")
            squint_regions[suffix] = {
                "skin": skin,
                "upper": upper,
                "lower": lower,
                "eye_core": eye_core,
                "center_x": float(face_mesh[f"squint_center_x_{side}"]),
                "center_z": float(face_mesh[f"squint_center_z_{side}"]),
                "radius_x": float(face_mesh[f"squint_radius_x_{side}"]),
                "radius_z": float(face_mesh[f"squint_radius_z_{side}"]),
                "closure_fraction": float(face_mesh["squint_max_closure_fraction"]),
            }
    def region_weight(
        world: Vector,
        center: Vector,
        radius_x: float,
        radius_z: float,
    ) -> float:
        if float(world.y) > front_limit:
            return 0.0
        nx = (float(world.x) - float(center.x)) / max(radius_x, 1e-6)
        nz = (float(world.z) - float(center.z)) / max(radius_z, 1e-6)
        radial = max(0.0, 1.0 - nx * nx - nz * nz)
        frontness = max(0.0, min(1.0, (front_limit - float(world.y)) / max(depth * 0.18, 1e-6)))
        return radial * radial * frontness

    created = {
        name: shape_keys.key_blocks.get(name) or face_mesh.shape_key_add(name=name, from_mix=False)
        for name in shape_names
    }
    affected = {name: 0 for name in shape_names}
    for index, basis_point in enumerate(basis.data):
        world = object_to_world @ basis_point.co
        eye_weights = {
            "L": region_weight(
                world,
                Vector((center_x + eye_offset_x, float(world.y), eye_center_z)),
                width * 0.125,
                region_height * 0.14,
            ),
            "R": region_weight(
                world,
                Vector((center_x - eye_offset_x, float(world.y), eye_center_z)),
                width * 0.125,
                region_height * 0.14,
            ),
        }
        for suffix, sign in (("L", 1.0), ("R", -1.0)):
            squint_name = f"Eye_Squint.{suffix}"
            wide_name = f"Eye_Wide.{suffix}"
            eye_weight = eye_weights[suffix]
            squint = squint_regions.get(suffix)
            if squint:
                if index in squint["skin"]:
                    is_upper = index in squint["upper"]
                    radius_x = max(float(squint["radius_x"]), 1e-6)
                    radius_z = max(float(squint["radius_z"]), 1e-6)
                    normalized_x = max(
                        -1.0,
                        min(
                            1.0,
                            (float(world.x) - float(squint["center_x"])) / radius_x,
                        ),
                    )
                    center_weight = max(0.0, 1.0 - normalized_x * normalized_x) ** 0.65
                    closure = float(squint["closure_fraction"]) * center_weight
                    squinted = world.copy()
                    squinted.z += (float(squint["center_z"]) - float(world.z)) * closure
                    created[squint_name].data[index].co = world_to_object @ squinted
                    widened = world.copy()
                    widened.z -= (
                        float(squint["center_z"]) - float(world.z)
                    ) * 0.045 * center_weight
                    created[wide_name].data[index].co = world_to_object @ widened
                    affected[squint_name] += 1
                    affected[wide_name] += 1
            elif eye_weight > 0.0:
                wide_world = world.copy()
                wide_world.z += (float(world.z) - eye_center_z) * 0.18 * eye_weight
                created[wide_name].data[index].co = world_to_object @ wide_world
                affected[wide_name] += 1

            brow_weight = region_weight(
                world,
                Vector((center_x + sign * eye_offset_x, float(world.y), brow_center_z)),
                width * 0.140,
                region_height * 0.10,
            )
            if brow_weight > 0.0:
                raise_name = f"Brow_Raise.{suffix}"
                furrow_name = f"Brow_Furrow.{suffix}"
                raised = world.copy()
                raised.z += height * 0.012 * brow_weight
                created[raise_name].data[index].co = world_to_object @ raised
                furrowed = world.copy()
                furrowed.z -= height * 0.006 * brow_weight
                furrowed.x -= sign * width * 0.005 * brow_weight
                created[furrow_name].data[index].co = world_to_object @ furrowed
                affected[raise_name] += 1
                affected[furrow_name] += 1

            cheek_weight = region_weight(
                world,
                Vector((center_x + sign * width * 0.095, float(world.y), cheek_center_z)),
                width * 0.170,
                region_height * 0.15,
            )
            if cheek_weight > 0.0:
                cheek_name = f"Cheek_Smile.{suffix}"
                puff_name = f"Cheek_Puff.{suffix}"
                smiled = world.copy()
                smiled.z += height * 0.008 * cheek_weight
                smiled.x += sign * width * 0.003 * cheek_weight
                smiled.y -= depth * 0.005 * cheek_weight
                created[cheek_name].data[index].co = world_to_object @ smiled
                puffed = world.copy()
                puffed.x += sign * width * 0.0045 * cheek_weight
                puffed.y -= depth * 0.010 * cheek_weight
                created[puff_name].data[index].co = world_to_object @ puffed
                affected[cheek_name] += 1
                affected[puff_name] += 1

            nose_weight = region_weight(
                world,
                Vector((center_x + sign * width * 0.040, float(world.y), float(min_v.z) + region_height * 0.315)),
                width * 0.070,
                region_height * 0.075,
            )
            if nose_weight > 0.0:
                nose_name = f"Nose_Flare.{suffix}"
                flared = world.copy()
                flared.x += sign * width * 0.0040 * nose_weight
                flared.y -= depth * 0.0025 * nose_weight
                created[nose_name].data[index].co = world_to_object @ flared
                affected[nose_name] += 1

        combined_eye_weight = max(eye_weights.values())
        if combined_eye_weight > 0.0:
            for name, direction in (("Eye_Look_Left", 1.0), ("Eye_Look_Right", -1.0)):
                looked = world.copy()
                looked.x += direction * width * 0.006 * combined_eye_weight
                created[name].data[index].co = world_to_object @ looked
                affected[name] += 1

    for name, count in affected.items():
        face_mesh[f"affected_{name}"] = count
    for suffix in ("L", "R"):
        squint = squint_regions.get(suffix)
        if squint:
            shape = created[f"Eye_Squint.{suffix}"]
            non_skin_displacement = max(
                (
                    (shape.data[index].co - basis.data[index].co).length
                    for index in range(len(basis.data))
                    if index not in squint["skin"]
                ),
                default=0.0,
            )
            core_displacement = max(
                (
                    (shape.data[index].co - basis.data[index].co).length
                    for index in squint["eye_core"]
                ),
                default=0.0,
            )
            face_mesh[
                f"squint_non_skin_max_displacement_{suffix.lower()}"
            ] = non_skin_displacement
            face_mesh[f"squint_core_max_displacement_{suffix.lower()}"] = core_displacement
    face_mesh["facial_detail_mode"] = "rich_source_mesh"
    face_mesh["source_mouth_replacement"] = False
    face_mesh["eye_center_z"] = eye_center_z
    face_mesh["eye_offset_x"] = eye_offset_x
    face_mesh["eye_radius_x"] = width * 0.115
    face_mesh["eye_radius_z"] = region_height * 0.12
    if squint_regions:
        face_mesh["blink_capability"] = "squint_only"
        face_mesh["squint_deformation_mode"] = "source_skin_ring_18_percent"
    return affected


VOLUMETRIC_FACE_ROLES = (
    "upper_lip",
    "lower_lip",
    "mouth_cavity",
    "upper_lid_l",
    "lower_lid_l",
    "upper_lid_r",
    "lower_lid_r",
)
INTEGRATED_FACE_ROLES = (
    "oral_cavity",
    "upper_teeth",
    "lower_teeth",
    "tongue",
)


def _drive_shape_key(
    target: bpy.types.Object,
    target_name: str,
    source: bpy.types.Object,
    source_name: str,
) -> None:
    target_keys = target.data.shape_keys
    source_keys = source.data.shape_keys
    if not target_keys or not source_keys:
        return
    target_key = target_keys.key_blocks.get(target_name)
    source_key = source_keys.key_blocks.get(source_name)
    if not target_key or not source_key:
        return
    driver = target_key.driver_add("value").driver
    variable = driver.variables.new()
    variable.name = "source_value"
    variable.type = "SINGLE_PROP"
    variable.targets[0].id_type = "KEY"
    variable.targets[0].id = source_keys
    variable.targets[0].data_path = f'key_blocks["{source_name}"].value'
    driver.expression = "source_value"


def _lip_centerline(
    shape_name: str,
    *,
    upper: bool,
    width: float,
    height: float,
    segments: int,
) -> list[tuple[float, float]]:
    specifications = {
        "Mouth_Rest": (0.82, 0.0012, "smile", 0.0048),
        "Mouth_A": (0.62, 0.0270, "neutral", 0.0),
        "Mouth_E": (1.04, 0.0090, "smile", 0.0020),
        "Mouth_O": (0.54, 0.0230, "neutral", 0.0),
        "Mouth_U": (0.44, 0.0170, "neutral", 0.0),
        "Mouth_MBP": (0.84, 0.0003, "neutral", 0.0),
        "Mouth_Smile": (1.08, 0.0030, "smile", 0.0100),
        "Mouth_Frown": (0.96, 0.0035, "frown", 0.0090),
        "Mouth_Surprise": (0.50, 0.0320, "neutral", 0.0),
    }
    width_scale, open_ratio, curve_name, curve_ratio = specifications[shape_name]
    radius_x = width * 0.050 * width_scale
    opening = height * open_ratio
    curve_amount = height * curve_ratio
    points: list[tuple[float, float]] = []
    for index in range(segments):
        normalized = -1.0 + 2.0 * index / max(1, segments - 1)
        x = normalized * radius_x
        corner = abs(normalized) ** 1.65
        if curve_name == "smile":
            curve = curve_amount * (corner - 0.28)
        elif curve_name == "frown":
            curve = -curve_amount * (corner - 0.28)
        else:
            curve = 0.0
        center_open = math.sqrt(max(0.0, 1.0 - normalized * normalized))
        separation = opening * center_open
        z = curve + (separation * 0.34 if upper else -separation * 0.66)
        points.append((x, z))
    return points


def _tube_coordinates(
    centerline: list[tuple[float, float]],
    *,
    radius: float,
    radial_segments: int,
) -> list[tuple[float, float, float]]:
    coordinates: list[tuple[float, float, float]] = []
    for index, (x, z) in enumerate(centerline):
        previous = centerline[max(0, index - 1)]
        following = centerline[min(len(centerline) - 1, index + 1)]
        tangent_x = following[0] - previous[0]
        tangent_z = following[1] - previous[1]
        tangent_length = max(math.hypot(tangent_x, tangent_z), 1e-6)
        normal_x = -tangent_z / tangent_length
        normal_z = tangent_x / tangent_length
        for radial_index in range(radial_segments):
            angle = radial_index * math.tau / radial_segments
            planar = math.cos(angle) * radius
            depth = math.sin(angle) * radius * 0.72
            coordinates.append((x + normal_x * planar, depth, z + normal_z * planar))
    return coordinates


def _create_lip_mesh(
    role: str,
    source_face: bpy.types.Object,
    armature: bpy.types.Object,
    head_bone_name: str,
    location: tuple[float, float, float],
    width: float,
    height: float,
    mat: bpy.types.Material,
) -> bpy.types.Object:
    upper = role == "upper_lip"
    line_segments = 32
    radial_segments = 8
    radius = height * (0.0033 if upper else 0.0037)
    rest_line = _lip_centerline(
        "Mouth_Rest",
        upper=upper,
        width=width,
        height=height,
        segments=line_segments,
    )
    vertices = _tube_coordinates(rest_line, radius=radius, radial_segments=radial_segments)
    faces = [
        (
            line_index * radial_segments + radial_index,
            (line_index + 1) * radial_segments + radial_index,
            (line_index + 1) * radial_segments + (radial_index + 1) % radial_segments,
            line_index * radial_segments + (radial_index + 1) % radial_segments,
        )
        for line_index in range(line_segments - 1)
        for radial_index in range(radial_segments)
    ]
    faces.extend(
        [
            tuple(range(radial_segments - 1, -1, -1)),
            tuple((line_segments - 1) * radial_segments + index for index in range(radial_segments)),
        ]
    )
    object_name = "IP_UpperLip" if upper else "IP_LowerLip"
    mesh = bpy.data.meshes.new(f"{object_name}_Mesh")
    mesh.from_pydata(vertices, [], faces)
    mesh.materials.append(mat)
    for polygon in mesh.polygons:
        polygon.use_smooth = True
    obj = bpy.data.objects.new(object_name, mesh)
    bpy.context.collection.objects.link(obj)
    obj.location = location
    obj.shape_key_add(name="Basis", from_mix=False)
    shape_names = (
        "Mouth_Rest",
        "Mouth_A",
        "Mouth_E",
        "Mouth_O",
        "Mouth_U",
        "Mouth_MBP",
        "Mouth_Smile",
        "Mouth_Frown",
        "Mouth_Surprise",
    )
    for shape_name in shape_names:
        key = obj.shape_key_add(name=shape_name, from_mix=False)
        coordinates = _tube_coordinates(
            _lip_centerline(
                shape_name,
                upper=upper,
                width=width,
                height=height,
                segments=line_segments,
            ),
            radius=radius,
            radial_segments=radial_segments,
        )
        for point, coordinate in zip(key.data, coordinates):
            point.co = coordinate
        _drive_shape_key(obj, shape_name, source_face, shape_name)
    obj["ip_face_topology_role"] = role
    obj["ip_true_lip_topology"] = True
    bind_mesh_to_bone(obj, armature, head_bone_name)
    return obj


def _create_mouth_cavity(
    source_face: bpy.types.Object,
    armature: bpy.types.Object,
    head_bone_name: str,
    location: tuple[float, float, float],
    width: float,
    height: float,
    mat: bpy.types.Material,
) -> bpy.types.Object:
    segments = 48
    vertices = organic_mouth_shape_coordinates("Mouth_Rest", width, height, segments)
    faces = [(index, (index + 1) % segments, segments) for index in range(segments)]
    mesh = bpy.data.meshes.new("IP_MouthCavity_Mesh")
    mesh.from_pydata(vertices, [], faces)
    mesh.materials.append(mat)
    obj = bpy.data.objects.new("IP_MouthCavity", mesh)
    bpy.context.collection.objects.link(obj)
    obj.location = location
    obj.shape_key_add(name="Basis", from_mix=False)
    for shape_name in (
        "Mouth_Rest",
        "Mouth_A",
        "Mouth_E",
        "Mouth_O",
        "Mouth_U",
        "Mouth_MBP",
        "Mouth_Smile",
        "Mouth_Frown",
        "Mouth_Surprise",
    ):
        key = obj.shape_key_add(name=shape_name, from_mix=False)
        for point, coordinate in zip(
            key.data,
            organic_mouth_shape_coordinates(shape_name, width, height, segments),
        ):
            point.co = coordinate
        _drive_shape_key(obj, shape_name, source_face, shape_name)
    solidify = obj.modifiers.new("IP_MouthCavity_Depth", "SOLIDIFY")
    solidify.thickness = height * 0.0025
    obj["ip_face_topology_role"] = "mouth_cavity"
    obj["ip_true_mouth_cavity"] = True
    bind_mesh_to_bone(obj, armature, head_bone_name)
    return obj


def _eyelid_coordinates(
    *,
    upper: bool,
    state: str,
    radius_x: float,
    radius_z: float,
    curvature: float,
    segments: int,
    rows: int,
) -> list[tuple[float, float, float]]:
    sign = 1.0 if upper else -1.0
    coordinates: list[tuple[float, float, float]] = []
    for back in (False, True):
        for index in range(segments):
            normalized = -1.0 + 2.0 * index / max(1, segments - 1)
            arc = math.sqrt(max(0.0, 1.0 - normalized * normalized))
            outer_z = sign * radius_z * arc
            if state == "blink":
                inner_z = (-0.68 if upper else -0.70) * radius_z * arc
            elif state == "wide":
                inner_z = sign * 0.90 * radius_z * arc
            else:
                inner_z = sign * 0.77 * radius_z * arc
            x = normalized * radius_x
            y_curve = curvature * normalized * normalized
            for row in range(rows):
                amount = row / max(1, rows - 1)
                z = outer_z * (1.0 - amount) + inner_z * amount
                y = y_curve - curvature * 0.34 * arc * amount + (curvature * 0.18 if back else 0.0)
                coordinates.append((x, y, z))
    return coordinates


def _create_eyelid_mesh(
    role: str,
    source_face: bpy.types.Object,
    armature: bpy.types.Object,
    head_bone_name: str,
    location: tuple[float, float, float],
    radius_x: float,
    radius_z: float,
    curvature: float,
    mat: bpy.types.Material,
) -> bpy.types.Object:
    upper = role.startswith("upper")
    suffix = "L" if role.endswith("_l") else "R"
    segments = 28
    rows = 3
    layer_size = segments * rows
    vertices = _eyelid_coordinates(
        upper=upper,
        state="rest",
        radius_x=radius_x,
        radius_z=radius_z,
        curvature=curvature,
        segments=segments,
        rows=rows,
    )
    faces: list[tuple[int, ...]] = []
    for back in (False, True):
        offset = layer_size if back else 0
        for index in range(segments - 1):
            for row in range(rows - 1):
                a = offset + index * rows + row
                b = offset + (index + 1) * rows + row
                c = offset + (index + 1) * rows + row + 1
                d = offset + index * rows + row + 1
                faces.append((d, c, b, a) if back else (a, b, c, d))
    for index in range(segments - 1):
        front_outer = index * rows
        next_front_outer = (index + 1) * rows
        back_outer = layer_size + index * rows
        next_back_outer = layer_size + (index + 1) * rows
        front_inner = index * rows + rows - 1
        next_front_inner = (index + 1) * rows + rows - 1
        back_inner = layer_size + index * rows + rows - 1
        next_back_inner = layer_size + (index + 1) * rows + rows - 1
        faces.append((front_outer, back_outer, next_back_outer, next_front_outer))
        faces.append((next_front_inner, next_back_inner, back_inner, front_inner))
    for index in (0, segments - 1):
        front = index * rows
        back = layer_size + index * rows
        faces.append(tuple(front + row for row in range(rows)) + tuple(back + row for row in reversed(range(rows))))

    object_name = f"IP_{'Upper' if upper else 'Lower'}Lid.{suffix}"
    mesh = bpy.data.meshes.new(f"{object_name}_Mesh")
    mesh.from_pydata(vertices, [], faces)
    mesh.materials.append(mat)
    for polygon in mesh.polygons:
        polygon.use_smooth = True
    obj = bpy.data.objects.new(object_name, mesh)
    bpy.context.collection.objects.link(obj)
    obj.location = location
    obj.shape_key_add(name="Basis", from_mix=False)
    blink_name = f"Eye_Blink.{suffix}"
    wide_name = f"Eye_Wide.{suffix}"
    for shape_name, state in ((blink_name, "blink"), (wide_name, "wide")):
        key = obj.shape_key_add(name=shape_name, from_mix=False)
        coordinates = _eyelid_coordinates(
            upper=upper,
            state=state,
            radius_x=radius_x,
            radius_z=radius_z,
            curvature=curvature,
            segments=segments,
            rows=rows,
        )
        for point, coordinate in zip(key.data, coordinates):
            point.co = coordinate
        _drive_shape_key(obj, shape_name, source_face, shape_name)
    obj["ip_face_topology_role"] = role
    obj["ip_true_eyelid_topology"] = True
    bind_mesh_to_bone(obj, armature, head_bone_name)
    return obj


def create_volumetric_face_topology(
    source_face: bpy.types.Object,
    character_objects: list[bpy.types.Object],
    dimensions: dict[str, Any],
    armature: bpy.types.Object,
    head_bone_name: str,
    *,
    mouth_height_ratio: float,
    mouth_scale: float,
) -> dict[str, bpy.types.Object]:
    existing = {
        str(obj.get("ip_face_topology_role")): obj
        for obj in character_objects
        if obj.type == "MESH" and obj.get("ip_face_topology_role")
    }
    if set(VOLUMETRIC_FACE_ROLES).issubset(existing):
        return {role: existing[role] for role in VOLUMETRIC_FACE_ROLES}

    head_bounds = weighted_region_bounds(character_objects, "head")
    width = max(float(head_bounds["width"] if head_bounds else dimensions["width"]), 0.01)
    depth = max(float(head_bounds["depth"] if head_bounds else dimensions["depth"]), 0.01)
    height = max(float(dimensions["height"]), 0.01)
    min_v = head_bounds["min"] if head_bounds else dimensions["min"]
    max_v = head_bounds["max"] if head_bounds else dimensions["max"]
    center_x = (float(min_v.x) + float(max_v.x)) * 0.5
    front_y = float(min_v.y)
    mouth_z = float(source_face.get("mouth_center_z", float(dimensions["min"].z) + height * float(mouth_height_ratio)))
    mouth_center_x = float(source_face.get("mouth_center_x", center_x))
    scale = max(0.70, min(1.35, float(mouth_scale or 1.0)))
    region_height = max(float(max_v.z - min_v.z), height * 0.1)
    eye_center_z = float(source_face.get("eye_center_z", float(min_v.z) + region_height * 0.46))
    eye_offset_x = float(source_face.get("eye_offset_x", width * 0.185))
    eye_radius_x = float(source_face.get("eye_radius_x", width * 0.115))
    eye_radius_z = float(source_face.get("eye_radius_z", region_height * 0.12))

    lip_mat = material("IP_Lip_Fur_Dark", (0.095, 0.038, 0.016, 1.0), False)
    cavity_mat = material("IP_Mouth_Cavity_Deep", (0.007, 0.002, 0.001, 1.0), False)
    lid_mat = material("IP_Eyelid_Fur", (0.155, 0.065, 0.026, 1.0), False)
    for mat, roughness in ((lip_mat, 0.58), (cavity_mat, 0.72), (lid_mat, 0.66)):
        bsdf = next((node for node in mat.node_tree.nodes if node.type == "BSDF_PRINCIPLED"), None)
        if bsdf:
            bsdf.inputs["Roughness"].default_value = roughness
            if "Specular IOR Level" in bsdf.inputs:
                bsdf.inputs["Specular IOR Level"].default_value = 0.24
            if mat in {lip_mat, lid_mat}:
                noise = mat.node_tree.nodes.new("ShaderNodeTexNoise")
                noise.inputs["Scale"].default_value = 78.0
                noise.inputs["Detail"].default_value = 2.0
                bump = mat.node_tree.nodes.new("ShaderNodeBump")
                bump.inputs["Strength"].default_value = 0.075
                bump.inputs["Distance"].default_value = 0.008
                mat.node_tree.links.new(noise.outputs["Fac"], bump.inputs["Height"])
                mat.node_tree.links.new(bump.outputs["Normal"], bsdf.inputs["Normal"])

    lip_location = (mouth_center_x, front_y - depth * 0.018, mouth_z)
    cavity_location = (mouth_center_x, front_y - depth * 0.013, mouth_z)
    mouth_feature_width = width * 2.0
    topology = {
        "upper_lip": _create_lip_mesh(
            "upper_lip", source_face, armature, head_bone_name, lip_location, mouth_feature_width * scale, height * scale, lip_mat
        ),
        "lower_lip": _create_lip_mesh(
            "lower_lip", source_face, armature, head_bone_name, lip_location, mouth_feature_width * scale, height * scale, lip_mat
        ),
        "mouth_cavity": _create_mouth_cavity(
            source_face, armature, head_bone_name, cavity_location, mouth_feature_width * scale, height * scale, cavity_mat
        ),
    }
    for suffix, sign in (("l", 1.0), ("r", -1.0)):
        location = (
            center_x + sign * eye_offset_x,
            front_y - depth * 0.019,
            eye_center_z,
        )
        for upper in (True, False):
            role = f"{'upper' if upper else 'lower'}_lid_{suffix}"
            topology[role] = _create_eyelid_mesh(
                role,
                source_face,
                armature,
                head_bone_name,
                location,
                eye_radius_x,
                eye_radius_z,
                depth * 0.022,
                lid_mat,
            )
    source_face["facial_topology_mode"] = "volumetric"
    source_face["true_lip_topology"] = True
    source_face["true_eyelid_topology"] = True
    return topology


def install_viseme_api() -> None:
    script = '''import bpy

VISEME_MAP = {
    "rest": "Mouth_Rest", "closed": "Mouth_Rest",
    "a": "Mouth_A", "e": "Mouth_E", "i": "Mouth_E",
    "o": "Mouth_O", "u": "Mouth_U", "mbp": "Mouth_MBP",
    "smile": "Mouth_Smile", "frown": "Mouth_Frown",
    "surprise": "Mouth_Surprise",
}


def find_viseme_object():
    for obj in bpy.data.objects:
        shape_keys = getattr(getattr(obj, "data", None), "shape_keys", None)
        if shape_keys and shape_keys.key_blocks.get("Mouth_A") and obj.get("source_mouth_deformed"):
            return obj
    for obj in bpy.data.objects:
        shape_keys = getattr(getattr(obj, "data", None), "shape_keys", None)
        if shape_keys and shape_keys.key_blocks.get("Mouth_A") and not obj.get("ip_face_topology_role"):
            return obj
    raise RuntimeError("No object with Mouth_* shape keys found")


def apply_viseme_timeline(viseme_events, object_name=""):
    mouth = bpy.data.objects.get(object_name) if object_name else find_viseme_object()
    if mouth is None:
        raise RuntimeError(f"Viseme object not found: {object_name}")
    keys = mouth.data.shape_keys.key_blocks
    for event in viseme_events:
        frame = max(1, int(event["frame"]))
        requested = str(event.get("viseme", "rest"))
        shape_name = requested if requested.startswith("Mouth_") else VISEME_MAP.get(requested.lower(), "Mouth_Rest")
        strength = float(event.get("value", 1.0))
        for key in keys:
            if key.name == "Basis":
                continue
            key.value = strength if key.name == shape_name else 0.0
            key.keyframe_insert(data_path="value", frame=frame)
    return len(viseme_events)
'''
    text = bpy.data.texts.get("IP_Viseme_API.py") or bpy.data.texts.new("IP_Viseme_API.py")
    text.clear()
    text.write(script)


def cleanup_bobo_source_mouth_texture(objects: list[bpy.types.Object]) -> dict[str, Any]:
    """Remove Bobo's baked mouth from a copied base-color image with a feathered clone."""
    try:
        import numpy as np
    except ImportError:
        return {"success": False, "reason": "numpy_unavailable"}

    image_nodes: list[bpy.types.Node] = []
    source_image = None
    for obj in objects:
        for mat in obj.data.materials:
            if not mat or not mat.use_nodes:
                continue
            for node in mat.node_tree.nodes:
                image = getattr(node, "image", None)
                if node.type != "TEX_IMAGE" or not image:
                    continue
                lowered = image.name.lower()
                if "texture_pbr_20250901" not in lowered or "metallic" in lowered or "normal" in lowered:
                    continue
                source_image = image
                image_nodes.append(node)
    if not source_image or not image_nodes:
        return {"success": False, "reason": "known_bobo_base_color_not_found"}

    width, height = int(source_image.size[0]), int(source_image.size[1])
    if width < 512 or height < 512:
        return {"success": False, "reason": "source_texture_too_small"}

    cleaned = source_image.copy()
    cleaned.name = f"{source_image.name}_mouth_clean"
    pixels = np.empty(width * height * 4, dtype=np.float32)
    cleaned.pixels.foreach_get(pixels)
    rgba = pixels.reshape((height, width, 4))

    x0, x1 = int(width * 0.315), int(width * 0.382)
    y0, y1 = int(height * 0.492), int(height * 0.523)
    sample_y0 = min(height - 1, y1 + max(4, int(height * 0.006)))
    sample_y1 = min(height, sample_y0 + max(8, int(height * 0.008)))
    source_strip = rgba[sample_y0:sample_y1, x0:x1, :]
    if source_strip.size == 0:
        return {"success": False, "reason": "source_clone_region_empty"}
    replacement = source_strip.mean(axis=0)[None, :, :]

    yy = (np.arange(y0, y1, dtype=np.float32)[:, None] + 0.5 - (y0 + y1) * 0.5) / max((y1 - y0) * 0.5, 1.0)
    xx = (np.arange(x0, x1, dtype=np.float32)[None, :] + 0.5 - (x0 + x1) * 0.5) / max((x1 - x0) * 0.5, 1.0)
    radius = np.sqrt(xx * xx + yy * yy)
    feather = np.clip((1.0 - radius) / 0.20, 0.0, 1.0)[..., None]
    region = rgba[y0:y1, x0:x1, :]
    region[:] = region * (1.0 - feather) + replacement * feather

    cleaned.pixels.foreach_set(rgba.reshape(-1))
    cleaned.update()
    cleaned.pack()
    for node in image_nodes:
        node.image = cleaned
    return {
        "success": True,
        "mode": "feathered_texture_clone",
        "sourceImage": source_image.name,
        "cleanedImage": cleaned.name,
        "normalizedRegion": [0.315, 0.492, 0.382, 0.523],
    }


def create_viseme_mouth(
    dimensions: dict[str, Any],
    armature: bpy.types.Object,
    texture_cleanup: dict[str, Any],
    *,
    head_bone_name: str,
    mouth_height_ratio: float,
    mouth_scale: float,
    mouth_style: str,
) -> dict[str, bpy.types.Object]:
    width = float(dimensions["width"])
    height = float(dimensions["height"])
    face_y = float(dimensions["min"].y)
    mouth_z = height * mouth_height_ratio
    shape_width = width * mouth_scale
    shape_height = height * mouth_scale

    face: dict[str, bpy.types.Object] = {}
    organic_style = mouth_style in {"organic", "organic_dark", "sloth"}
    if not texture_cleanup.get("success") and not organic_style:
        patch_color = (0.88, 0.76, 0.60, 1.0) if organic_style else (0.0035, 0.0050, 0.0080, 1.0)
        patch_name = "IP_Mouth_Cleanup_Muzzle" if organic_style else "IP_Mouth_Cleanup_Black"
        patch_mat = material(patch_name, patch_color, False)
        patch_bsdf = next((node for node in patch_mat.node_tree.nodes if node.type == "BSDF_PRINCIPLED"), None)
        if patch_bsdf:
            patch_bsdf.inputs["Roughness"].default_value = 0.74 if organic_style else 0.34
            if "Metallic" in patch_bsdf.inputs:
                patch_bsdf.inputs["Metallic"].default_value = 0.0
            if "Specular IOR Level" in patch_bsdf.inputs:
                patch_bsdf.inputs["Specular IOR Level"].default_value = 0.16 if organic_style else 0.08
            if organic_style:
                noise = patch_mat.node_tree.nodes.new("ShaderNodeTexNoise")
                noise.inputs["Scale"].default_value = 52.0
                noise.inputs["Detail"].default_value = 2.2
                bump = patch_mat.node_tree.nodes.new("ShaderNodeBump")
                bump.inputs["Strength"].default_value = 0.11
                bump.inputs["Distance"].default_value = 0.018
                patch_mat.node_tree.links.new(noise.outputs["Fac"], bump.inputs["Height"])
                patch_mat.node_tree.links.new(bump.outputs["Normal"], patch_bsdf.inputs["Normal"])
        patch = add_ellipse(
            "IP_Mouth_Cleanup_Patch",
            (0.0, face_y - 0.010, mouth_z),
            width * (0.046 if organic_style else 0.064),
            height * (0.014 if organic_style else 0.022),
            patch_mat,
        )
        patch["purpose"] = "occludes_baked_source_mouth"
        patch["mouth_patch_style"] = "organic_muzzle" if organic_style else "screen_black"
        bind_mesh_to_bone(patch, armature, head_bone_name)
        face["patch"] = patch

    if organic_style:
        mouth_mat = material("IP_Mouth_Organic_Dark", (0.025, 0.010, 0.004, 1.0), False)
    else:
        mouth_mat = material("IP_Mouth_Warm_Gold", (1.0, 0.53, 0.085, 1.0), True)
    mouth_bsdf = next((node for node in mouth_mat.node_tree.nodes if node.type == "BSDF_PRINCIPLED"), None)
    if mouth_bsdf:
        mouth_bsdf.inputs["Roughness"].default_value = 0.42 if organic_style else 0.28
        if not organic_style and "Emission Strength" in mouth_bsdf.inputs:
            mouth_bsdf.inputs["Emission Strength"].default_value = 2.4

    shape_names = (
        "Mouth_Rest",
        "Mouth_A",
        "Mouth_E",
        "Mouth_O",
        "Mouth_U",
        "Mouth_MBP",
        "Mouth_Smile",
        "Mouth_Frown",
        "Mouth_Surprise",
    )
    segments = 48
    coordinate_function = organic_mouth_shape_coordinates if organic_style else mouth_shape_coordinates
    vertices = coordinate_function("Mouth_Rest", shape_width, shape_height, segments)
    if organic_style:
        faces = [(index, (index + 1) % segments, segments) for index in range(segments)]
    else:
        faces = [
            (index, (index + 1) % segments, segments + (index + 1) % segments, segments + index)
            for index in range(segments)
        ]
    mesh = bpy.data.meshes.new("IP_Mouth_Mesh")
    mesh.from_pydata(vertices, [], faces)
    mesh.materials.append(mouth_mat)
    mouth = bpy.data.objects.new("IP_Mouth", mesh)
    bpy.context.collection.objects.link(mouth)
    mouth.location = (0.0, face_y - 0.018, mouth_z)
    mouth.shape_key_add(name="Basis")
    for shape_name in shape_names:
        key = mouth.shape_key_add(name=shape_name)
        key.interpolation = "KEY_LINEAR"
        for point, coordinate in zip(key.data, coordinate_function(shape_name, shape_width, shape_height, segments)):
            point.co = coordinate
    mouth["viseme_shape_keys"] = json.dumps(shape_names)
    mouth["source_mouth_replacement"] = True
    mouth["source_mouth_cleanup"] = json.dumps(texture_cleanup)
    mouth["source_mouth_removed"] = bool(texture_cleanup.get("success"))
    mouth["mouth_style"] = mouth_style
    bind_mesh_to_bone(mouth, armature, head_bone_name)
    install_viseme_api()
    face["mouth"] = mouth
    return face


def find_existing_viseme_mouth(objects: list[bpy.types.Object]) -> bpy.types.Object | None:
    best = None
    best_score = 0
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
    for obj in objects:
        shape_keys = getattr(obj.data, "shape_keys", None)
        if not shape_keys:
            continue
        names = {key.name for key in shape_keys.key_blocks}
        score = len(expected.intersection(names))
        if score > best_score:
            best = obj
            best_score = score
    return best if best_score >= 3 else None


def setup_face(
    data: dict,
    dimensions: dict[str, Any],
    armature: bpy.types.Object,
    character_objects: list[bpy.types.Object],
    bone_map: dict[str, str],
) -> dict[str, bpy.types.Object]:
    mode = str(data.get("faceScreenMode") or "source").lower()
    mouth_mode = str(data.get("mouthMode") or "independent_visemes").lower()
    topology_mode = str(data.get("facialTopologyMode") or "source_only").lower()
    character_id = str(data.get("characterId") or "").lower()
    if bool(data.get("useMasterAsset")):
        existing_mouth = find_existing_viseme_mouth(character_objects)
        if not existing_mouth:
            raise RuntimeError("configured character master is missing preserved Mouth_* Shape Keys")
        face: dict[str, bpy.types.Object] = {"mouth": existing_mouth}
        for obj in character_objects:
            role = str(obj.get("ip_face_topology_role") or "")
            if role:
                face[role] = obj
        existing_mouth["existing_viseme_mouth_preserved"] = True
        return face
    if (
        character_id == "main_ip_sloth"
        and topology_mode in {"volumetric", "true_geometry", "lips_eyelids"}
    ):
        raise RuntimeError(
            f"facialTopologyMode={topology_mode} is not supported for this source asset; "
            "use facialTopologyMode=source_retopology with blinkCapability=squint_only"
        )

    def enrich_source_face(face: dict[str, bpy.types.Object]) -> dict[str, bpy.types.Object]:
        source_face = face.get("mouth")
        if not source_face:
            return face
        rich = str(data.get("facialDetailMode") or "rich").lower() not in {"off", "none", "basic"}
        if rich:
            add_rich_source_face_shapes(source_face, dimensions)
        if topology_mode in {"source_retopology", "integrated_source", "integrated"}:
            face.update(
                retopologize_source_face(
                    character_objects,
                    dimensions,
                    armature,
                    bone_map,
                    mouth_height_ratio=float(data.get("mouthHeightRatio") or 0.56),
                    mouth_scale=float(data.get("mouthScale") or 1.0),
                )
            )
        existing_topology = {
            str(obj.get("ip_face_topology_role"))
            for obj in character_objects
            if obj.type == "MESH" and obj.get("ip_face_topology_role")
        }
        wants_topology = topology_mode in {"volumetric", "true_geometry", "lips_eyelids"}
        has_topology = set(VOLUMETRIC_FACE_ROLES).issubset(existing_topology)
        if wants_topology or has_topology:
            head_bone_name = bone_map.get("head") or bone_map.get("body") or bone_map.get("root")
            if not head_bone_name:
                head_bone_name = next(
                    (bone.name for bone in armature.data.bones if bone.use_deform),
                    armature.data.bones[0].name,
                )
            face.update(
                create_volumetric_face_topology(
                    source_face,
                    character_objects,
                    dimensions,
                    armature,
                    head_bone_name,
                    mouth_height_ratio=float(data.get("mouthHeightRatio") or 0.56),
                    mouth_scale=float(data.get("mouthScale") or 1.0),
                )
            )
        return face

    if mode in {"none", "source", "source_face"}:
        existing_mouth = find_existing_viseme_mouth(character_objects)
        if mouth_mode in {"auto", "existing_visemes", "source_mesh_visemes", "source_mesh"} and existing_mouth:
            existing_mouth["source_mouth_replacement"] = False
            existing_mouth["existing_viseme_mouth_preserved"] = True
            return enrich_source_face({"mouth": existing_mouth})
        if mouth_mode == "existing_visemes" and not existing_mouth:
            raise RuntimeError("mouthMode=existing_visemes requires at least three Mouth_* shape keys")
        if mouth_mode in {"source_mesh_visemes", "source_mesh", "deform_source"}:
            topology: dict[str, bpy.types.Object] = {}
            if topology_mode in {"source_retopology", "integrated_source", "integrated"}:
                topology = retopologize_source_face(
                    character_objects,
                    dimensions,
                    armature,
                    bone_map,
                    mouth_height_ratio=float(data.get("mouthHeightRatio") or 0.56),
                    mouth_scale=float(data.get("mouthScale") or 1.0),
                )
            face = create_source_mesh_visemes(
                character_objects,
                dimensions,
                mouth_height_ratio=float(data.get("mouthHeightRatio") or 0.56),
                mouth_scale=float(data.get("mouthScale") or 1.0),
            )
            face.update({role: obj for role, obj in topology.items() if role != "mouth"})
            return enrich_source_face(face)
        if mouth_mode in {"auto", "independent_visemes", "visemes", "independent"}:
            is_bobo = "bobo" in character_id or "波波" in character_id
            texture_cleanup = (
                cleanup_bobo_source_mouth_texture(character_objects)
                if is_bobo
                else {"success": False, "reason": "character_specific_cleanup_disabled"}
            )
            head_bone_name = bone_map.get("head") or bone_map.get("body") or bone_map.get("root")
            if not head_bone_name:
                head_bone_name = next((bone.name for bone in armature.data.bones if bone.use_deform), armature.data.bones[0].name)
            return create_viseme_mouth(
                dimensions,
                armature,
                texture_cleanup,
                head_bone_name=head_bone_name,
                mouth_height_ratio=float(data.get("mouthHeightRatio") or 0.56),
                mouth_scale=float(data.get("mouthScale") or 1.0),
                mouth_style=str(data.get("mouthStyle") or ("screen_gold" if is_bobo else "organic_dark")).lower(),
            )
        return {}
    black = material("face_screen_black", (0.01, 0.014, 0.018, 1.0), True)
    eye_mat = material("face_eye_cyan", (0.32, 0.9, 1.0, 1.0), True)
    mouth_mat = material("face_mouth_warm", (1.0, 0.42, 0.58, 1.0), True)
    width = float(dimensions["width"])
    height = float(dimensions["height"])
    front_y = float(dimensions["min"].y) - 0.025
    screen = add_plane("dynamic_face_screen", (0, front_y, height * 0.72), (width * 0.58, height * 0.16, 1), black)
    left_eye = add_plane("dynamic_left_eye", (-width * 0.105, front_y - 0.01, height * 0.75), (width * 0.055, height * 0.018, 1), eye_mat)
    right_eye = add_plane("dynamic_right_eye", (width * 0.105, front_y - 0.01, height * 0.75), (width * 0.055, height * 0.018, 1), eye_mat)
    mouth = add_plane("dynamic_mouth", (0, front_y - 0.015, height * 0.68), (width * 0.09, height * 0.014, 1), mouth_mat)
    face = {"screen": screen, "left_eye": left_eye, "right_eye": right_eye, "mouth": mouth}
    face_bone_name = bone_map.get("head") or bone_map.get("body") or bone_map.get("root")
    if not face_bone_name:
        face_bone_name = next((bone.name for bone in armature.data.bones if bone.use_deform), armature.data.bones[0].name)
    for obj in face.values():
        bind_mesh_to_bone(obj, armature, face_bone_name)
    return face


def event_amount(t: float, event: dict) -> float:
    start = float(event.get("timeSec") or 0)
    duration = max(0.001, float(event.get("duration") or 0.5))
    if t < start or t > start + duration:
        return 0.0
    phase = (t - start) / duration
    return math.sin(phase * math.pi) * float(event.get("strength") or 1.0)


def iter_action_fcurves(action: bpy.types.Action | None):
    if not action:
        return
    legacy = getattr(action, "fcurves", None)
    if legacy is not None:
        yield from legacy
        return
    for layer in getattr(action, "layers", ()):
        for strip in getattr(layer, "strips", ()):
            for channelbag in getattr(strip, "channelbags", ()):
                yield from channelbag.fcurves


def set_action_interpolation(action: bpy.types.Action | None) -> None:
    if not action:
        return
    for fcurve in iter_action_fcurves(action):
        for point in fcurve.keyframe_points:
            point.interpolation = "BEZIER"
            point.handle_left_type = "AUTO_CLAMPED"
            point.handle_right_type = "AUTO_CLAMPED"


def finger_euler(side: str, curl: float, splay: float = 0.0, twist: float = 0.0) -> tuple[float, float, float]:
    """Map semantic digit motion onto the generated finger bones' local axes."""
    curl_sign = 1.0 if str(side).lower().startswith("r") else -1.0
    return (float(splay), float(twist), curl_sign * float(curl))


SAFE_FINGER_CURL_MAX = 0.34
DISTAL_FINGER_CURL_SCALE = 0.32
WAVE_PALM_FACING_ROTATION_Y = -1.10
WAVE_OPEN_FINGER_SPLAY = 0.14


def semantic_pose_rotation(role: str, rotation: tuple[float, float, float]) -> tuple[float, float, float]:
    if not role.startswith("finger_"):
        return rotation
    side = "r" if role.endswith("_r") else "l"
    curl, splay, twist = rotation
    return finger_euler(side, curl, splay, twist)


def apply_digit_pose(
    pose_bones,
    bone_map: dict[str, str],
    side: str,
    digit: int,
    digit_pose: aroll_actions.DigitPose,
) -> None:
    """Apply a shared semantic digit pose, preserving exact legacy two-bone support."""
    roles = aroll_actions.chain_roles(side, digit)
    if aroll_actions.has_three_segment_chain(bone_map, side, digit):
        for role, rotation in aroll_actions.hand_pose_eulers(
            side, {digit: digit_pose}, bone_map
        ).items():
            if role in bone_map and bone_map[role] in pose_bones:
                pose_bones[bone_map[role]].rotation_euler = rotation
        return
    proximal_name = bone_map.get(roles["proximal"])
    distal_name = bone_map.get(roles["distal"])
    if proximal_name and proximal_name in pose_bones:
        pose_bones[proximal_name].rotation_euler = semantic_pose_rotation(
            roles["proximal"],
            (digit_pose.proximal, digit_pose.splay, digit_pose.opposition),
        )
    if (
        aroll_actions.has_legacy_two_segment_chain(bone_map, side, digit)
        and distal_name
        and distal_name in pose_bones
    ):
        pose_bones[distal_name].rotation_euler = semantic_pose_rotation(
            roles["distal"],
            (digit_pose.distal, digit_pose.splay * 0.08, 0.0),
        )


def apply_hand_pose(
    pose_bones,
    bone_map: dict[str, str],
    side: str,
    digit_poses,
) -> None:
    for digit in (1, 2, 3):
        apply_digit_pose(pose_bones, bone_map, side, digit, digit_poses[digit])


def lip_at(plan: dict, t: float) -> dict:
    lips = plan.get("lipSync") or []
    if not lips:
        return {"viseme": "closed", "open": 0.0}
    index = min(len(lips) - 1, max(0, int(t / max(float(plan.get("durationSec") or 1), 0.001) * len(lips))))
    return lips[index]


def uses_source_humanoid_axes(armature: bpy.types.Object) -> bool:
    """Detect FBX/Mixamo-style local axes even after custom properties are lost in GLB export."""
    if bool(armature.get("ip_avatar_generated_humanoid")):
        return False
    normalized = {
        "".join(character for character in bone.name.lower() if character.isalnum())
        for bone in armature.data.bones
    }
    source_limb_suffixes = ("leftarm", "leftforearm", "rightarm", "rightforearm")
    return all(any(name.endswith(suffix) for name in normalized) for suffix in source_limb_suffixes)


def source_root_location_from_world(
    armature: bpy.types.Object,
    root_bone: bpy.types.PoseBone,
    world_offset: tuple[float, float, float],
) -> tuple[float, float, float]:
    mapping = armature.matrix_world.to_3x3() @ root_bone.bone.matrix_local.to_3x3()
    local_offset = mapping.inverted_safe() @ Vector(world_offset)
    return tuple(float(value) for value in local_offset)


def _clear_action_channels(action: bpy.types.Action) -> None:
    """Clear keyframe data while preserving one canonical Action datablock."""
    fcurves = getattr(action, "fcurves", None)
    if fcurves is not None:
        for fcurve in list(fcurves):
            fcurves.remove(fcurve)
        return
    for layer in list(action.layers):
        action.layers.remove(layer)
    for slot in list(action.slots):
        action.slots.remove(slot)


def _reuse_runtime_action(animated_id: Any, canonical_name: str) -> dict[str, Any]:
    animated_id.animation_data_create()
    animation_data = animated_id.animation_data
    animation_data.action = None
    canonical = bpy.data.actions.get(canonical_name)
    if canonical is None:
        canonical = bpy.data.actions.new(canonical_name)

    removed_duplicates: list[str] = []
    prefix = f"{canonical_name}."
    for action in list(bpy.data.actions):
        suffix = action.name[len(prefix) :] if action.name.startswith(prefix) else ""
        if action == canonical or len(suffix) != 3 or not suffix.isdigit():
            continue
        removed_duplicates.append(action.name)
        action.user_remap(canonical)
        bpy.data.actions.remove(action)

    _clear_action_channels(canonical)
    canonical.use_fake_user = True
    animation_data.action = canonical
    return {"action": canonical.name, "removedDuplicates": removed_duplicates}


def reconcile_runtime_timeline_actions(
    armature: bpy.types.Object,
    face: dict[str, bpy.types.Object],
) -> dict[str, Any]:
    """Reuse the canonical per-render Actions without touching the master library."""
    resolved = {"body": _reuse_runtime_action(armature, "Talk_Loop")}
    mouth = face.get("mouth")
    shape_keys = mouth.data.shape_keys if mouth and mouth.type == "MESH" else None
    if shape_keys:
        resolved["mouth"] = _reuse_runtime_action(shape_keys, "Mouth_Viseme_Timeline")
    return resolved


def animate(
    armature: bpy.types.Object,
    face: dict[str, bpy.types.Object],
    plan: dict,
    fps: int,
    bone_map: dict[str, str],
    *,
    runtime_actions_prepared: bool = False,
    presentation_mode: str = "standing",
) -> None:
    events = plan.get("motionEvents") or []
    frame_end = max(1, int(float(plan.get("durationSec") or 1) * fps))
    pose = armature.pose.bones
    source_rig = uses_source_humanoid_axes(armature)
    base_pose = aroll_actions.presentation_pose(presentation_mode, source_rig)
    seated = bool(base_pose)
    if seated:
        events = [event for event in events if event.get("motion") != "happy_bounce"]
    if not runtime_actions_prepared:
        reconcile_runtime_timeline_actions(armature, face)
    if armature.animation_data:
        for track in armature.animation_data.nla_tracks:
            track.mute = True
    animated_bones = tuple(dict.fromkeys(name for name in bone_map.values() if name in pose))
    for bone_name in animated_bones:
        pose[bone_name].rotation_mode = "XYZ"
        pose[bone_name].location = (0, 0, 0)
        pose[bone_name].rotation_euler = (0, 0, 0)
        pose[bone_name].scale = (1, 1, 1)

    mouth = face.get("mouth")

    def set_bone(role: str, *, location=None, rotation=None) -> None:
        bone_name = bone_map.get(role)
        if not bone_name or bone_name not in pose:
            return
        if location is not None:
            pose[bone_name].location = location
        if rotation is not None:
            pose[bone_name].rotation_euler = rotation

    for frame in range(1, frame_end + 1, 2):
        t = (frame - 1) / fps
        idle_body_sway = math.sin(t * math.pi * 2 * 0.35) * 0.018
        body_sway = idle_body_sway
        head_nod = math.sin(t * math.pi * 2 * 0.42) * 0.014
        head_turn = 0.0
        head_tilt = -body_sway * 0.45
        eye_gaze = 0.0
        root_lift = 0.0 if seated else math.sin(t * math.pi * 2 * 0.7) * 0.008
        root_side = math.sin(t * math.pi * 2 * 0.23) * 0.004
        shoulder_left = [0.0, 0.0, -0.015]
        shoulder_right = [0.0, 0.0, 0.015]
        arm_drift = math.sin(t * math.pi * 2 * 0.31) * 0.018
        if source_rig:
            upper_left = [arm_drift * 0.35, 0.0, 1.28]
            upper_right = [-arm_drift * 0.35, 0.0, -1.28]
        else:
            upper_left = [-0.72 + arm_drift, 0.04, -0.025]
            upper_right = [-0.72 - arm_drift, -0.04, 0.025]
        fore_drift = math.sin(t * math.pi * 2 * 0.27) * 0.024
        if source_rig:
            fore_left = [-0.10 + fore_drift, 0.0, 0.0]
            fore_right = [-0.10 - fore_drift, 0.0, 0.0]
        else:
            fore_left = [-0.10 + fore_drift, 0.03, -0.012]
            fore_right = [-0.10 - fore_drift, -0.03, 0.012]
        hand_left = [0.0, 0.04, -0.025]
        hand_right = [0.0, -0.04, 0.025]
        digit_poses_left = dict(aroll_actions.hand_pose("relaxed_hand"))
        digit_poses_right = dict(aroll_actions.hand_pose("relaxed_hand"))
        leg_left = 0.0 if seated else math.sin(t * math.pi * 2 * 0.24) * 0.035
        leg_right = -leg_left
        right_hand_stage = 0.0

        def stage_right_hand(amount: float, lift: float = 1.0) -> None:
            """Bring detailed hand motion into a readable chest-level pose."""
            nonlocal right_hand_stage
            right_hand_stage = max(right_hand_stage, amount * lift)

        def apply_right_hand_stage() -> None:
            amount = right_hand_stage
            if source_rig:
                upper_right[0] -= amount * 0.07
                upper_right[1] -= amount * 0.03
                upper_right[2] += amount * 0.54
                fore_right[0] -= amount * 0.50
                fore_right[1] -= amount * 0.06
                fore_right[2] += amount * 0.78
            else:
                upper_right[0] += amount * 0.48
                upper_right[1] -= amount * 0.12
                upper_right[2] += amount * 0.12
                fore_right[0] += amount * 0.92
                fore_right[1] -= amount * 0.18
                fore_right[2] += amount * 0.06

        for event in events:
            amount = event_amount(t, event)
            motion = event.get("motion")
            if motion == "nod":
                head_nod += amount * 0.14
            elif motion == "head_shake":
                head_turn += math.sin(t * 16.0) * amount * 0.15
            elif motion == "emphasis":
                root_lift += amount * 0.025
                body_sway += amount * 0.035
                upper_left[0] += amount * 0.16
                upper_right[0] += amount * 0.16
                upper_left[2] -= amount * 0.045
                upper_right[2] += amount * 0.045
                fore_left[0] += amount * 0.20
                fore_right[0] += amount * 0.20
                fore_left[1] += amount * 0.10
                fore_right[1] -= amount * 0.10
            elif motion == "happy_bounce":
                root_lift += amount * 0.07
                leg_left -= amount * 0.035
                leg_right += amount * 0.035
            elif motion == "wave":
                start = float(event.get("timeSec") or 0.0)
                duration = max(0.001, float(event.get("duration") or 0.9))
                phase = max(0.0, min(1.0, (t - start) / duration))
                wave = math.sin(phase * math.pi * 4.0)
                pose_amount = min(1.0, amount * 1.20)
                if source_rig:
                    shoulder_right[2] += pose_amount * 0.04
                    upper_right[0] -= pose_amount * 0.08
                    upper_right[1] += pose_amount * 0.03
                    upper_right[2] += pose_amount * 1.04
                    fore_right[0] -= pose_amount * 0.16
                    fore_right[1] += pose_amount * 0.02
                    fore_right[2] += pose_amount * 1.15
                else:
                    shoulder_right[0] += pose_amount * 0.06
                    shoulder_right[2] += pose_amount * 0.10
                    upper_right[0] += pose_amount * 0.82
                    upper_right[1] += pose_amount * 0.22
                    upper_right[2] += pose_amount * 0.28
                    fore_right[0] += pose_amount * 1.05
                    fore_right[1] += pose_amount * 0.24 + amount * wave * 0.04
                    fore_right[2] += amount * wave * 0.08
                hand_right[0] += amount * wave * 0.035
                hand_right[1] += pose_amount * (WAVE_PALM_FACING_ROTATION_Y + 0.04)
                hand_right[2] += amount * wave * 0.07
                digit_poses_right = aroll_actions.blend_hand_pose(
                    digit_poses_right, aroll_actions.hand_pose("open_hand"), pose_amount
                )
            elif motion in {"point", "point_left"}:
                if source_rig:
                    upper_left[0] -= amount * 0.04
                    upper_left[1] += amount * 0.02
                    upper_left[2] -= amount * 1.10
                else:
                    upper_left[0] += amount * 0.48
                    upper_left[1] += amount * 0.12
                    upper_left[2] -= amount * 0.16
                fore_left[0] += amount * 0.16
                fore_left[1] += amount * 0.12
                hand_left[1] += amount * 0.08
                digit_poses_left = aroll_actions.blend_hand_pose(
                    digit_poses_left, aroll_actions.hand_pose("point"), amount
                )
            elif motion == "point_right":
                if source_rig:
                    upper_right[0] -= amount * 0.04
                    upper_right[1] -= amount * 0.02
                    upper_right[2] += amount * 1.10
                else:
                    upper_right[0] += amount * 0.48
                    upper_right[1] -= amount * 0.12
                    upper_right[2] += amount * 0.16
                fore_right[0] += amount * 0.16
                fore_right[1] -= amount * 0.12
                hand_right[1] -= amount * 0.08
                digit_poses_right = aroll_actions.blend_hand_pose(
                    digit_poses_right, aroll_actions.hand_pose("point"), amount
                )
            elif motion == "present":
                if source_rig:
                    upper_left[2] -= amount * 0.48
                    upper_right[2] += amount * 0.48
                    fore_left[0] -= amount * 0.72
                    fore_right[0] -= amount * 0.72
                else:
                    upper_left[0] += amount * 0.26
                    upper_right[0] += amount * 0.26
                    upper_left[2] -= amount * 0.10
                    upper_right[2] += amount * 0.10
                    fore_left[0] -= amount * 0.22
                    fore_right[0] -= amount * 0.22
                fore_left[1] += amount * 0.42
                fore_right[1] -= amount * 0.42
                hand_left[1] += amount * 0.54
                hand_right[1] -= amount * 0.54
                hand_left[2] -= amount * 0.16
                hand_right[2] += amount * 0.16
                head_tilt += amount * 0.025
            elif motion == "open_arms":
                shoulder_left[2] -= amount * 0.10
                shoulder_right[2] += amount * 0.10
                if source_rig:
                    upper_left[2] -= amount * 0.92
                    upper_right[2] += amount * 0.92
                    fore_left[0] -= amount * 0.12
                    fore_right[0] -= amount * 0.12
                else:
                    upper_left[0] += amount * 0.36
                    upper_right[0] += amount * 0.36
                    upper_left[1] += amount * 0.10
                    upper_right[1] -= amount * 0.10
                    upper_left[2] -= amount * 0.20
                    upper_right[2] += amount * 0.20
                fore_left[1] += amount * 0.18
                fore_right[1] -= amount * 0.18
                hand_left[1] += amount * 0.42
                hand_right[1] -= amount * 0.42
                digit_poses_left = aroll_actions.blend_hand_pose(
                    digit_poses_left, aroll_actions.hand_pose("open_hand"), amount
                )
                digit_poses_right = aroll_actions.blend_hand_pose(
                    digit_poses_right, aroll_actions.hand_pose("open_hand"), amount
                )
            elif motion == "think":
                if source_rig:
                    upper_right[0] -= amount * 0.12
                    upper_right[1] -= amount * 0.04
                    upper_right[2] += amount * 0.56
                    fore_right[0] -= amount * 0.52
                    fore_right[1] -= amount * 0.08
                    fore_right[2] += amount * 0.98
                else:
                    upper_right[0] += amount * 0.54
                    upper_right[1] -= amount * 0.16
                    upper_right[2] += amount * 0.13
                    fore_right[0] += amount * 1.05
                    fore_right[1] -= amount * 0.22
                    fore_right[2] += amount * 0.08
                hand_right[0] += amount * 0.14
                hand_right[1] -= amount * 0.18
                hand_right[2] += amount * 0.10
                digit_poses_right = aroll_actions.blend_hand_pose(
                    digit_poses_right, aroll_actions.hand_pose("soft_curl"), amount
                )
                head_turn += amount * 0.12
                head_tilt -= amount * 0.09
            elif motion == "shrug":
                shoulder_left[0] -= amount * 0.15
                shoulder_right[0] -= amount * 0.15
                if source_rig:
                    upper_left[2] -= amount * 0.52
                    upper_right[2] += amount * 0.52
                    fore_left[0] -= amount * 0.58
                    fore_right[0] -= amount * 0.58
                else:
                    upper_left[0] += amount * 0.18
                    upper_right[0] += amount * 0.18
                    fore_left[1] += amount * 0.26
                    fore_right[1] -= amount * 0.26
                hand_left[2] -= amount * 0.09
                hand_right[2] += amount * 0.09
                head_tilt += amount * 0.08
            elif motion == "leg_step":
                if seated:
                    continue
                step = math.sin(t * 17.0) * amount * 0.11
                leg_left += step
                leg_right -= step * 0.72
                root_lift += amount * 0.012
            elif motion == "weight_shift":
                if seated:
                    continue
                direction = -1.0 if float(event.get("direction") or 1.0) < 0 else 1.0
                root_side += direction * amount * 0.075
                body_sway += direction * amount * 0.055
                leg_left += direction * amount * 0.090
                leg_right -= direction * amount * 0.055
                root_lift -= amount * 0.012
            elif motion == "fist":
                stage_right_hand(amount, 0.92)
                hand_right[0] += amount * 0.08
                hand_right[1] -= amount * 0.22
                hand_right[2] += amount * 0.08
                digit_poses_right = aroll_actions.blend_hand_pose(
                    digit_poses_right, aroll_actions.hand_pose("fist"), amount
                )
            elif motion == "open_hand":
                stage_right_hand(amount, 0.90)
                hand_right[1] -= amount * 0.28
                hand_right[2] += amount * 0.10
                digit_poses_right = aroll_actions.blend_hand_pose(
                    digit_poses_right, aroll_actions.hand_pose("open_hand"), amount
                )
            elif motion == "wrist_twist":
                stage_right_hand(amount, 0.94)
                twist = math.sin(t * 10.0) * amount * 0.62
                hand_right[1] += twist
            elif motion == "finger_wave":
                stage_right_hand(amount, 0.92)
                hand_right[1] -= amount * 0.20
                start = float(event.get("timeSec") or 0.0)
                duration = max(0.001, float(event.get("duration") or 0.8))
                phase = max(0.0, min(1.0, (t - start) / duration))
                selected = min(3, int(phase * 3.0) + 1)
                digit_poses_right = aroll_actions.blend_hand_pose(
                    digit_poses_right,
                    aroll_actions.finger_roll_pose(selected, unselected_pose="relaxed_hand"),
                    amount,
                )
            elif motion == "antenna_wiggle":
                head_tilt += math.sin(t * 19.0) * amount * 0.018
            elif motion == "micro_gaze":
                direction = -1.0 if float(event.get("direction") or -1.0) < 0 else 1.0
                head_turn += direction * amount * 0.030
                head_tilt -= amount * 0.008
                eye_gaze += direction * amount * 0.10
            elif motion == "brow_beat":
                head_nod += amount * 0.020
        apply_right_hand_stage()
        lip = lip_at(plan, t)
        mouth_open = float(lip.get("open") or 0)
        bpy.context.scene.frame_set(frame)
        root_base = base_pose.get("root", {}).get("location", (0.0, 0.0, 0.0))
        body_base = base_pose.get("body", {}).get("rotation", (0.0, 0.0, 0.0))
        root_world = (root_base[0] + root_side, root_base[1], root_base[2] + root_lift)
        root_name = bone_map.get("root")
        root_location = (
            source_root_location_from_world(armature, pose[root_name], root_world)
            if source_rig and root_name and root_name in pose
            else root_world
        )
        set_bone(
            "root",
            location=root_location,
        )
        set_bone(
            "body",
            rotation=(
                body_base[0],
                body_base[1],
                body_base[2] + body_sway - (idle_body_sway if seated else 0.0),
            ),
        )
        set_bone("spine", rotation=(body_sway * 0.22, 0, body_sway * 0.38))
        set_bone("chest", rotation=(-body_sway * 0.18, 0, body_sway * 0.46))
        set_bone(
            "head",
            rotation=(head_tilt, head_turn, head_nod) if source_rig else (head_nod, head_turn, head_tilt),
        )
        set_bone("jaw", rotation=(mouth_open * 0.14, 0, 0))
        set_bone("eye_l", rotation=(0, eye_gaze, 0))
        set_bone("eye_r", rotation=(0, eye_gaze, 0))
        tongue_wave = math.sin(t * math.pi * 2 * 2.7) * mouth_open
        set_bone("tongue_1", rotation=(mouth_open * 0.015, 0, tongue_wave * 0.025))
        set_bone("tongue_2", rotation=(mouth_open * 0.025, 0, -tongue_wave * 0.045))
        set_bone("tongue_3", rotation=(mouth_open * 0.035, 0, tongue_wave * 0.065))
        set_bone("shoulder_l", rotation=shoulder_left)
        set_bone("upper_arm_l", rotation=upper_left)
        set_bone("forearm_l", rotation=fore_left)
        set_bone("hand_l", rotation=hand_left)
        apply_hand_pose(pose, bone_map, "l", digit_poses_left)
        set_bone("shoulder_r", rotation=shoulder_right)
        set_bone("upper_arm_r", rotation=upper_right)
        set_bone("forearm_r", rotation=fore_right)
        set_bone("hand_r", rotation=hand_right)
        apply_hand_pose(pose, bone_map, "r", digit_poses_right)
        if source_rig:
            lower_motion = {
                "leg_l": (0.0, 0.0, leg_left),
                "shin_l": (0.0, 0.0, -leg_left * 0.34),
                "foot_l": (0.0, 0.0, leg_left * 0.16),
                "leg_r": (0.0, 0.0, -leg_right),
                "shin_r": (0.0, 0.0, leg_right * 0.34),
                "foot_r": (0.0, 0.0, -leg_right * 0.16),
            }
        else:
            lower_motion = {
                "leg_l": (leg_left, 0.0, 0.0),
                "shin_l": (-leg_left * 0.34, 0.0, 0.0),
                "foot_l": (leg_left * 0.16, 0.0, 0.0),
                "leg_r": (leg_right, 0.0, 0.0),
                "shin_r": (-leg_right * 0.34, 0.0, 0.0),
                "foot_r": (leg_right * 0.16, 0.0, 0.0),
            }
        for role, motion_rotation in lower_motion.items():
            base_rotation = base_pose.get(role, {}).get("rotation", (0.0, 0.0, 0.0))
            set_bone(
                role,
                rotation=tuple(base_rotation[index] + motion_rotation[index] for index in range(3)),
            )
        for bone_name in animated_bones:
            bone = pose[bone_name]
            bone.keyframe_insert(data_path="location", frame=frame)
            bone.keyframe_insert(data_path="rotation_euler", frame=frame)
            bone.keyframe_insert(data_path="scale", frame=frame)

        if mouth:
            if mouth.data.shape_keys:
                viseme = str(lip.get("viseme") or "closed").lower()
                viseme_map = {
                    "a": "Mouth_A",
                    "e": "Mouth_E",
                    "i": "Mouth_E",
                    "o": "Mouth_O",
                    "u": "Mouth_U",
                    "mbp": "Mouth_MBP",
                    "closed": "Mouth_Rest",
                    "rest": "Mouth_Rest",
                }
                active_name = viseme_map.get(viseme, "Mouth_Rest")
                key_blocks = mouth.data.shape_keys.key_blocks
                controlled_names = set(viseme_map.values())
                for key in key_blocks:
                    if key.name not in controlled_names:
                        continue
                    key.value = 0.0
                    key.keyframe_insert(data_path="value", frame=frame)
                active_key = key_blocks.get(active_name)
                if active_key:
                    active_key.value = 1.0 if active_name in {"Mouth_Rest", "Mouth_MBP"} else 0.55 + mouth_open * 0.45
                    active_key.keyframe_insert(data_path="value", frame=frame)

                if key_blocks.get("Eye_Squint.L") and key_blocks.get("Eye_Squint.R"):
                    squint_amount = max(
                        (
                            event_amount(t, event)
                            for event in events
                            if event.get("motion") == "blink"
                        ),
                        default=0.0,
                    )
                    happy_amount = max(
                        (
                            event_amount(t, event)
                            for event in events
                            if event.get("motion") == "happy_bounce"
                        ),
                        default=0.0,
                    )
                    think_amount = max(
                        (
                            event_amount(t, event)
                            for event in events
                            if event.get("motion") == "think"
                        ),
                        default=0.0,
                    )
                    brow_amount = max(
                        (
                            event_amount(t, event)
                            for event in events
                            if event.get("motion") == "brow_beat"
                        ),
                        default=0.0,
                    )
                    gaze_amount = sum(
                        event_amount(t, event)
                        * (-1.0 if float(event.get("direction") or -1.0) < 0 else 1.0)
                        for event in events
                        if event.get("motion") == "micro_gaze"
                    )
                    rich_values = {
                        "Eye_Squint.L": squint_amount,
                        "Eye_Squint.R": squint_amount,
                        "Cheek_Smile.L": happy_amount * 0.58,
                        "Cheek_Smile.R": happy_amount * 0.58,
                        "Brow_Raise.L": max(think_amount * 0.44, brow_amount),
                        "Brow_Raise.R": brow_amount * 0.84,
                        "Brow_Furrow.R": think_amount * 0.18,
                        "Eye_Look_Left": max(0.0, gaze_amount) * 0.72,
                        "Eye_Look_Right": max(think_amount * 0.24, max(0.0, -gaze_amount) * 0.72),
                    }
                    for shape_name, value in rich_values.items():
                        shape = key_blocks.get(shape_name)
                        if not shape:
                            continue
                        shape.value = max(0.0, min(1.0, float(value)))
                        shape.keyframe_insert(data_path="value", frame=frame)
            else:
                mouth.scale.x = 0.16 + mouth_open * 0.17
                mouth.scale.y = 0.025 + mouth_open * 0.08
                mouth.keyframe_insert(data_path="scale", frame=frame)

            if "left_eye" in face and "right_eye" in face:
                blink = any(event.get("motion") == "blink" and event_amount(t, event) > 0.55 for event in events)
                for eye_key in ("left_eye", "right_eye"):
                    eye = face[eye_key]
                    eye.scale.y = 0.006 if blink else 0.045
                    eye.keyframe_insert(data_path="scale", frame=frame)

    if armature.animation_data and armature.animation_data.action:
        armature.animation_data.action.name = "Talk_Loop"
        set_action_interpolation(armature.animation_data.action)
    if mouth and mouth.data.shape_keys and mouth.data.shape_keys.animation_data and mouth.data.shape_keys.animation_data.action:
        mouth.data.shape_keys.animation_data.action.name = "Mouth_Viseme_Timeline"
        set_action_interpolation(mouth.data.shape_keys.animation_data.action)


def create_action_library(
    armature: bpy.types.Object,
    face: dict[str, bpy.types.Object],
    bone_map: dict[str, str],
    fps: int,
) -> dict[str, Any]:
    """Create reusable presenter gesture and expression actions in the Blend file."""
    fps = max(8, int(fps or 30))
    armature.animation_data_create()
    previous_action = armature.animation_data.action
    pose_bones = armature.pose.bones
    roles = [role for role, name in bone_map.items() if name in pose_bones]
    source_rig = uses_source_humanoid_axes(armature)

    generated_base_rotations = {
        "upper_arm_l": (-0.72, 0.04, -0.025),
        "upper_arm_r": (-0.72, -0.04, 0.025),
        "forearm_l": (-0.10, 0.03, 0.0),
        "forearm_r": (-0.10, -0.03, 0.0),
        "hand_l": (0.0, 0.04, -0.025),
        "hand_r": (0.0, -0.04, 0.025),
        "finger_1_l": (0.06, 0.0, 0.0),
        "finger_2_l": (0.04, 0.0, 0.0),
        "finger_3_l": (0.08, 0.0, 0.0),
        "finger_1_r": (0.06, 0.0, 0.0),
        "finger_2_r": (0.04, 0.0, 0.0),
        "finger_3_r": (0.08, 0.0, 0.0),
    }
    source_base_rotations = {
        "upper_arm_l": (0.0, 0.0, 1.28),
        "upper_arm_r": (0.0, 0.0, -1.28),
        "forearm_l": (-0.10, 0.0, 0.0),
        "forearm_r": (-0.10, 0.0, 0.0),
        "hand_l": (0.0, 0.03, 0.0),
        "hand_r": (0.0, -0.03, 0.0),
        "finger_1_l": (0.04, 0.0, 0.0),
        "finger_2_l": (0.03, 0.0, 0.0),
        "finger_3_l": (0.06, 0.0, 0.0),
        "finger_1_tip_l": (0.05, 0.0, 0.0),
        "finger_2_tip_l": (0.04, 0.0, 0.0),
        "finger_3_tip_l": (0.08, 0.0, 0.0),
        "finger_1_r": (0.04, 0.0, 0.0),
        "finger_2_r": (0.03, 0.0, 0.0),
        "finger_3_r": (0.06, 0.0, 0.0),
        "finger_1_tip_r": (0.05, 0.0, 0.0),
        "finger_2_tip_r": (0.04, 0.0, 0.0),
        "finger_3_tip_r": (0.08, 0.0, 0.0),
    }
    base_rotations = source_base_rotations if source_rig else generated_base_rotations

    def reset_pose() -> None:
        for role in roles:
            bone = pose_bones[bone_map[role]]
            bone.rotation_mode = "XYZ"
            bone.location = (0.0, 0.0, 0.0)
            bone.rotation_euler = semantic_pose_rotation(
                role,
                base_rotations.get(role, (0.0, 0.0, 0.0)),
            )
            bone.scale = (1.0, 1.0, 1.0)
        for side in ("l", "r"):
            apply_hand_pose(pose_bones, bone_map, side, aroll_actions.hand_pose("relaxed_hand"))

    def named_hand_pose(name: str):
        if name.startswith("finger_roll_"):
            return aroll_actions.finger_roll_pose(int(name.rsplit("_", 1)[1]))
        return aroll_actions.hand_pose(name)

    def build_action(name: str, keyframes: list[tuple[int, dict[str, Any]]]) -> str:
        existing = bpy.data.actions.get(name)
        if existing:
            existing.use_fake_user = True
            return existing.name
        armature.animation_data.action = None
        for frame, rotations in keyframes:
            bpy.context.scene.frame_set(max(1, int(frame)))
            reset_pose()
            for role, rotation in rotations.items():
                if role.startswith("__digit_pose_"):
                    continue
                bone_name = bone_map.get(role)
                if bone_name and bone_name in pose_bones:
                    if isinstance(rotation, dict):
                        if "location" in rotation:
                            pose_bones[bone_name].location = (
                                source_root_location_from_world(
                                    armature, pose_bones[bone_name], rotation["location"]
                                )
                                if source_rig and role == "root"
                                else rotation["location"]
                            )
                        if "rotation" in rotation:
                            pose_bones[bone_name].rotation_euler = semantic_pose_rotation(
                                role, rotation["rotation"]
                            )
                    else:
                        pose_bones[bone_name].rotation_euler = semantic_pose_rotation(role, rotation)
            marked_sides: set[str] = set()
            for role, pose_name in rotations.items():
                if not role.startswith("__digit_pose_"):
                    continue
                side = role.rsplit("_", 1)[1]
                marked_sides.add(side)
                apply_hand_pose(pose_bones, bone_map, side, named_hand_pose(str(pose_name)))
            for side in ("l", "r"):
                for digit in (1, 2, 3):
                    proximal_role = f"finger_{digit}_{side}"
                    distal_role = f"finger_{digit}_tip_{side}"
                    if side in marked_sides or proximal_role not in rotations:
                        continue
                    proximal_rotation = rotations[proximal_role]
                    if aroll_actions.has_three_segment_chain(bone_map, side, digit):
                        apply_digit_pose(
                            pose_bones,
                            bone_map,
                            side,
                            digit,
                            aroll_actions.articulated_pose(*proximal_rotation),
                        )
                    elif (
                        aroll_actions.has_legacy_two_segment_chain(bone_map, side, digit)
                        and distal_role not in rotations
                    ):
                        distal_name = bone_map.get(distal_role)
                        if distal_name and distal_name in pose_bones:
                            pose_bones[distal_name].rotation_euler = semantic_pose_rotation(
                                distal_role,
                                (
                                    proximal_rotation[0] * DISTAL_FINGER_CURL_SCALE,
                                    proximal_rotation[1] * 0.24,
                                    proximal_rotation[2] * 0.65,
                                ),
                            )
            for role in roles:
                bone = pose_bones[bone_map[role]]
                bone.keyframe_insert(data_path="location", frame=frame)
                bone.keyframe_insert(data_path="rotation_euler", frame=frame)
                bone.keyframe_insert(data_path="scale", frame=frame)
        action = armature.animation_data.action
        if not action:
            raise RuntimeError(f"failed to create action: {name}")
        action.name = name
        action.use_fake_user = True
        set_action_interpolation(action)
        return action.name

    mid = max(2, fps)
    end = max(3, fps * 2)
    action_specs: dict[str, list[tuple[int, dict[str, tuple[float, float, float]]]]] = {
        "Idle_Speaking": [
            (1, {"body": (0, 0, -0.018), "head": (-0.01, 0, 0.012)}),
            (mid, {"body": (0, 0, 0.018), "head": (0.018, 0, -0.012)}),
            (end, {"body": (0, 0, -0.018), "head": (-0.01, 0, 0.012)}),
        ],
        "Talk_Loop": [
            (1, {"head": (-0.015, 0, 0), "forearm_l": (-0.16, 0.04, -0.02), "forearm_r": (-0.16, -0.04, 0.02)}),
            (mid, {"head": (0.04, 0, 0), "body": (0, 0, 0.025), "hand_l": (0.02, 0.10, -0.04), "hand_r": (0.02, -0.10, 0.04)}),
            (end, {"head": (-0.015, 0, 0), "forearm_l": (-0.16, 0.04, -0.02), "forearm_r": (-0.16, -0.04, 0.02)}),
        ],
        "Gesture_Wave": [
            (1, {}),
            (mid // 2, {"shoulder_r": (0.06, -0.08, 0.12), "upper_arm_r": (0.05, 0.22, 0.34), "forearm_r": (0.88, 0.04, 0.02), "hand_r": (0.035, WAVE_PALM_FACING_ROTATION_Y, 0.055), "finger_1_r": (0.015, WAVE_OPEN_FINGER_SPLAY, 0.0), "finger_2_r": (0.015, 0.0, 0.0), "finger_3_r": (0.015, -WAVE_OPEN_FINGER_SPLAY, 0.0)}),
            (mid, {"shoulder_r": (0.06, -0.08, 0.12), "upper_arm_r": (0.05, 0.22, 0.34), "forearm_r": (0.88, 0.04, 0.02), "hand_r": (-0.035, WAVE_PALM_FACING_ROTATION_Y, -0.055), "finger_1_r": (0.015, WAVE_OPEN_FINGER_SPLAY, 0.0), "finger_2_r": (0.015, 0.0, 0.0), "finger_3_r": (0.015, -WAVE_OPEN_FINGER_SPLAY, 0.0)}),
            (mid + mid // 2, {"shoulder_r": (0.06, -0.08, 0.12), "upper_arm_r": (0.05, 0.22, 0.34), "forearm_r": (0.88, 0.04, 0.02), "hand_r": (0.035, WAVE_PALM_FACING_ROTATION_Y, 0.055), "finger_1_r": (0.015, WAVE_OPEN_FINGER_SPLAY, 0.0), "finger_2_r": (0.015, 0.0, 0.0), "finger_3_r": (0.015, -WAVE_OPEN_FINGER_SPLAY, 0.0)}),
            (end, {}),
        ],
        "Gesture_Explain": [
            (1, {}),
            (mid, {"upper_arm_l": (-0.40, 0.12, -0.14), "upper_arm_r": (-0.40, -0.12, 0.14), "forearm_l": (-0.52, 0.36, -0.08), "forearm_r": (-0.52, -0.36, 0.08), "hand_l": (0.08, 0.26, -0.12), "hand_r": (0.08, -0.26, 0.12)}),
            (end, {}),
        ],
        "Gesture_OpenArms": [
            (1, {}),
            (mid, {"shoulder_l": (-0.04, 0.0, -0.12), "shoulder_r": (-0.04, 0.0, 0.12), "upper_arm_l": (-0.28, 0.10, -0.24), "upper_arm_r": (-0.28, -0.10, 0.24), "forearm_l": (-0.18, 0.18, -0.06), "forearm_r": (-0.18, -0.18, 0.06), "hand_l": (0.04, 0.20, -0.08), "hand_r": (0.04, -0.20, 0.08)}),
            (end, {}),
        ],
        "Gesture_Point_Left": [
            (1, {}),
            (mid, {"upper_arm_l": (-0.30, 0.12, -0.20), "forearm_l": (-0.10, 0.10, -0.05), "hand_l": (0.02, 0.08, -0.04), "finger_1_l": (0.58, 0.0, 0.0), "finger_2_l": (0.0, 0.0, 0.0), "finger_3_l": (0.68, 0.0, 0.0)}),
            (end, {}),
        ],
        "Gesture_Point_Right": [
            (1, {}),
            (mid, {"upper_arm_r": (-0.30, -0.12, 0.20), "forearm_r": (-0.10, -0.10, 0.05), "hand_r": (0.02, -0.08, 0.04), "finger_1_r": (0.58, 0.0, 0.0), "finger_2_r": (0.0, 0.0, 0.0), "finger_3_r": (0.68, 0.0, 0.0)}),
            (end, {}),
        ],
        "Gesture_Present_Left": [
            (1, {}),
            (mid, {"upper_arm_l": (-0.42, 0.16, -0.10), "forearm_l": (-0.60, 0.52, -0.08), "hand_l": (0.10, 0.32, -0.12), "finger_1_l": (0.16, 0.0, 0.0), "finger_2_l": (0.10, 0.0, 0.0), "finger_3_l": (0.18, 0.0, 0.0)}),
            (end, {}),
        ],
        "Gesture_Present_Right": [
            (1, {}),
            (mid, {"upper_arm_r": (-0.42, -0.16, 0.10), "forearm_r": (-0.60, -0.52, 0.08), "hand_r": (0.10, -0.32, 0.12), "finger_1_r": (0.16, 0.0, 0.0), "finger_2_r": (0.10, 0.0, 0.0), "finger_3_r": (0.18, 0.0, 0.0)}),
            (end, {}),
        ],
        "Gesture_Shrug": [
            (1, {}),
            (mid, {"shoulder_l": (-0.16, 0.0, -0.08), "shoulder_r": (-0.16, 0.0, 0.08), "upper_arm_l": (-0.54, 0.10, -0.08), "upper_arm_r": (-0.54, -0.10, 0.08), "forearm_l": (-0.34, 0.30, 0.0), "forearm_r": (-0.34, -0.30, 0.0), "head": (0.0, 0.0, 0.08)}),
            (end, {}),
        ],
        "Gesture_Think": [
            (1, {}),
            (mid, {"upper_arm_r": (-0.18, -0.18, 0.16), "forearm_r": (0.96, -0.26, 0.10), "hand_r": (0.16, -0.18, 0.12), "finger_1_r": (0.34, 0.0, 0.0), "finger_2_r": (0.24, 0.0, 0.0), "finger_3_r": (0.40, 0.0, 0.0), "head": (0.02, 0.12, -0.10)}),
            (end, {}),
        ],
        "Gesture_Step": [
            (1, {}),
            (mid, {"body": (0.02, 0.0, -0.03), "leg_l": (0.14, 0.04, -0.02), "shin_l": (-0.22, 0.0, 0.0), "foot_l": (0.12, 0.02, 0.0), "leg_r": (-0.06, -0.02, 0.01), "shin_r": (0.08, 0.0, 0.0)}),
            (end, {}),
        ],
        "Gesture_Nod": [(1, {"head": (-0.02, 0, 0)}), (mid, {"head": (0.18, 0, 0)}), (end, {"head": (-0.02, 0, 0)})],
        "Gesture_ShakeHead": [(1, {"head": (0, -0.18, 0)}), (mid, {"head": (0, 0.18, 0)}), (end, {"head": (0, -0.18, 0)})],
        "Gesture_Emphasis": [
            (1, {}),
            (mid, {"body": (0, 0, 0.05), "head": (0.08, 0, 0), "forearm_l": (-0.42, 0, 0), "forearm_r": (-0.42, 0, 0)}),
            (end, {}),
        ],
        "Gesture_OpenHand": [
            (1, {}),
            (mid, {"hand_l": (0.0, 0.30, -0.08), "hand_r": (0.0, -0.30, 0.08), "finger_1_l": (0.02, 0.12, 0.0), "finger_2_l": (0.02, 0.0, 0.0), "finger_3_l": (0.02, -0.12, 0.0), "finger_1_r": (0.02, 0.12, 0.0), "finger_2_r": (0.02, 0.0, 0.0), "finger_3_r": (0.02, -0.12, 0.0)}),
            (end, {}),
        ],
        "Gesture_Fist": [
            (1, {}),
            (mid, {"hand_l": (0.08, 0.28, -0.05), "hand_r": (0.08, -0.28, 0.05), "finger_1_l": (0.24, 0.02, 0.0), "finger_2_l": (0.26, 0.0, 0.0), "finger_3_l": (0.28, -0.02, 0.0), "finger_1_r": (0.24, 0.02, 0.0), "finger_2_r": (0.26, 0.0, 0.0), "finger_3_r": (0.28, -0.02, 0.0)}),
            (end, {}),
        ],
        "Gesture_WristTwist": [
            (1, {"hand_l": (0.0, -0.56, 0.0), "hand_r": (0.0, 0.56, 0.0)}),
            (mid, {"hand_l": (0.0, 0.56, 0.0), "hand_r": (0.0, -0.56, 0.0)}),
            (end, {"hand_l": (0.0, -0.56, 0.0), "hand_r": (0.0, 0.56, 0.0)}),
        ],
        "Gesture_FingerWave": [
            (1, {"finger_1_r": (0.05, 0.10, 0.0), "finger_2_r": (0.05, 0.0, 0.0), "finger_3_r": (0.05, -0.10, 0.0)}),
            (max(2, mid // 2), {"finger_1_r": (0.22, 0.08, 0.0), "finger_2_r": (0.08, 0.0, 0.0), "finger_3_r": (0.05, -0.08, 0.0)}),
            (mid, {"finger_1_r": (0.08, 0.08, 0.0), "finger_2_r": (0.22, 0.0, 0.0), "finger_3_r": (0.08, -0.08, 0.0)}),
            (mid + max(2, mid // 2), {"finger_1_r": (0.05, 0.08, 0.0), "finger_2_r": (0.08, 0.0, 0.0), "finger_3_r": (0.22, -0.08, 0.0)}),
            (end, {"finger_1_r": (0.05, 0.10, 0.0), "finger_2_r": (0.05, 0.0, 0.0), "finger_3_r": (0.05, -0.10, 0.0)}),
        ],
        "Look_Camera": [(1, {"head": (0, 0, 0), "eye_l": (0, 0, 0), "eye_r": (0, 0, 0)}), (end, {"head": (0, 0, 0), "eye_l": (0, 0, 0), "eye_r": (0, 0, 0)})],
        "Look_Left": [(1, {"head": (0, 0.16, 0), "eye_l": (0, 0.12, 0), "eye_r": (0, 0.12, 0)}), (end, {"head": (0, 0.16, 0), "eye_l": (0, 0.12, 0), "eye_r": (0, 0.12, 0)})],
        "Look_Right": [(1, {"head": (0, -0.16, 0), "eye_l": (0, -0.12, 0), "eye_r": (0, -0.12, 0)}), (end, {"head": (0, -0.16, 0), "eye_l": (0, -0.12, 0), "eye_r": (0, -0.12, 0)})],
        "Expression_Happy": [(1, {"head": (-0.025, 0, -0.04)}), (mid, {"head": (0.025, 0, 0.04)}), (end, {"head": (-0.025, 0, -0.04)})],
        "Expression_Thinking": [(1, {"head": (0, 0.16, -0.11)}), (end, {"head": (0, 0.16, -0.11)})],
        "Expression_Surprised": [(1, {"head": (-0.10, 0, 0)}), (end, {"head": (-0.10, 0, 0)})],
        "Expression_Confused": [(1, {"head": (0.03, -0.08, 0.13)}), (end, {"head": (0.03, -0.08, 0.13)})],
        "Expression_Serious": [(1, {"head": (0.08, 0, 0)}), (end, {"head": (0.08, 0, 0)})],
    }
    action_specs.update(
        {
            "Gesture_Count_One": [
                (1, {}),
                (mid, {"upper_arm_r": (-0.24, -0.10, 0.14), "forearm_r": (0.72, -0.22, 0.08), "hand_r": (0.08, -0.16, 0.10), "finger_1_r": (0.02, 0.0, 0.0), "finger_2_r": (0.34, 0.0, 0.0), "finger_3_r": (0.40, 0.0, 0.0)}),
                (end, {}),
            ],
            "Gesture_Count_Two": [
                (1, {}),
                (mid, {"upper_arm_r": (-0.24, -0.10, 0.14), "forearm_r": (0.72, -0.22, 0.08), "hand_r": (0.08, -0.16, 0.10), "finger_1_r": (0.02, 0.0, 0.0), "finger_2_r": (0.02, 0.0, 0.0), "finger_3_r": (0.40, 0.0, 0.0)}),
                (end, {}),
            ],
            "Gesture_Pinch": [
                (1, {}),
                (mid, {"upper_arm_r": (-0.30, -0.12, 0.12), "forearm_r": (0.58, -0.18, 0.06), "hand_r": (0.10, -0.14, 0.08), "finger_1_r": (0.30, 0.0, -0.04), "finger_2_r": (0.10, 0.0, 0.0), "finger_3_r": (0.34, 0.0, 0.04)}),
                (end, {}),
            ],
        }
    )

    if source_rig:
        source_pose_specs: dict[str, list[tuple[int, dict[str, tuple[float, float, float]]]]] = {
            "Talk_Loop": [
                (1, {"head": (0.0, -0.015, -0.012), "forearm_l": (-0.18, 0.0, 0.0), "forearm_r": (-0.18, 0.0, 0.0)}),
                (mid, {"body": (0.0, 0.0, 0.022), "head": (0.0, 0.015, 0.028), "forearm_l": (-0.34, 0.02, -0.02), "forearm_r": (-0.28, -0.02, 0.02), "hand_l": (0.04, 0.12, -0.03), "hand_r": (-0.03, -0.12, 0.03)}),
                (end, {"head": (0.0, -0.015, -0.012), "forearm_l": (-0.18, 0.0, 0.0), "forearm_r": (-0.18, 0.0, 0.0)}),
            ],
            "Gesture_Wave": [
                (1, {}),
                (mid // 2, {"upper_arm_r": (-0.08, 0.03, -0.30), "forearm_r": (-0.25, 0.02, 1.08), "hand_r": (0.035, WAVE_PALM_FACING_ROTATION_Y, 0.055), "finger_1_r": (0.015, WAVE_OPEN_FINGER_SPLAY, 0.0), "finger_2_r": (0.015, 0.0, 0.0), "finger_3_r": (0.015, -WAVE_OPEN_FINGER_SPLAY, 0.0)}),
                (mid, {"upper_arm_r": (-0.08, 0.03, -0.30), "forearm_r": (-0.25, 0.02, 1.08), "hand_r": (-0.035, WAVE_PALM_FACING_ROTATION_Y, -0.055), "finger_1_r": (0.015, WAVE_OPEN_FINGER_SPLAY, 0.0), "finger_2_r": (0.015, 0.0, 0.0), "finger_3_r": (0.015, -WAVE_OPEN_FINGER_SPLAY, 0.0)}),
                (mid + mid // 2, {"upper_arm_r": (-0.08, 0.03, -0.30), "forearm_r": (-0.25, 0.02, 1.08), "hand_r": (0.035, WAVE_PALM_FACING_ROTATION_Y, 0.055), "finger_1_r": (0.015, WAVE_OPEN_FINGER_SPLAY, 0.0), "finger_2_r": (0.015, 0.0, 0.0), "finger_3_r": (0.015, -WAVE_OPEN_FINGER_SPLAY, 0.0)}),
                (end, {}),
            ],
            "Gesture_Explain": [
                (1, {}),
                (mid, {"upper_arm_l": (-0.06, 0.02, 0.82), "upper_arm_r": (-0.06, -0.02, -0.82), "forearm_l": (-0.78, 0.08, -0.08), "forearm_r": (-0.78, -0.08, 0.08), "hand_l": (0.10, 0.24, -0.10), "hand_r": (0.10, -0.24, 0.10), "finger_1_l": (0.16, 0.0, 0.0), "finger_2_l": (0.10, 0.0, 0.0), "finger_3_l": (0.20, 0.0, 0.0), "finger_1_r": (0.16, 0.0, 0.0), "finger_2_r": (0.10, 0.0, 0.0), "finger_3_r": (0.20, 0.0, 0.0)}),
                (end, {}),
            ],
            "Gesture_OpenArms": [
                (1, {}),
                (mid, {"upper_arm_l": (-0.04, 0.02, 0.30), "upper_arm_r": (-0.04, -0.02, -0.30), "forearm_l": (-0.24, 0.08, -0.04), "forearm_r": (-0.24, -0.08, 0.04), "hand_l": (0.04, 0.12, -0.06), "hand_r": (0.04, -0.12, 0.06)}),
                (end, {}),
            ],
            "Gesture_Point_Left": [
                (1, {}),
                (mid, {"upper_arm_l": (-0.04, 0.02, 0.16), "forearm_l": (-0.12, 0.04, -0.02), "hand_l": (0.02, 0.08, -0.04), "finger_1_l": (0.34, 0.0, 0.0), "finger_2_l": (0.02, 0.0, 0.0), "finger_3_l": (0.40, 0.0, 0.0)}),
                (end, {}),
            ],
            "Gesture_Point_Right": [
                (1, {}),
                (mid, {"upper_arm_r": (-0.04, -0.02, -0.16), "forearm_r": (-0.12, -0.04, 0.02), "hand_r": (0.02, -0.08, 0.04), "finger_1_r": (0.34, 0.0, 0.0), "finger_2_r": (0.02, 0.0, 0.0), "finger_3_r": (0.40, 0.0, 0.0)}),
                (end, {}),
            ],
            "Gesture_Present_Left": [
                (1, {}),
                (mid, {"upper_arm_l": (-0.04, 0.02, 0.78), "forearm_l": (-0.92, 0.10, -0.08), "hand_l": (0.12, 0.28, -0.12), "finger_1_l": (0.16, 0.0, 0.0), "finger_2_l": (0.10, 0.0, 0.0), "finger_3_l": (0.18, 0.0, 0.0)}),
                (end, {}),
            ],
            "Gesture_Present_Right": [
                (1, {}),
                (mid, {"upper_arm_r": (-0.04, -0.02, -0.78), "forearm_r": (-0.92, -0.10, 0.08), "hand_r": (0.12, -0.28, 0.12), "finger_1_r": (0.16, 0.0, 0.0), "finger_2_r": (0.10, 0.0, 0.0), "finger_3_r": (0.18, 0.0, 0.0)}),
                (end, {}),
            ],
            "Gesture_Shrug": [
                (1, {}),
                (mid, {"shoulder_l": (-0.10, 0.0, -0.04), "shoulder_r": (-0.10, 0.0, 0.04), "upper_arm_l": (-0.04, 0.02, 0.72), "upper_arm_r": (-0.04, -0.02, -0.72), "forearm_l": (-0.70, 0.12, -0.04), "forearm_r": (-0.70, -0.12, 0.04), "head": (0.10, 0.0, 0.0)}),
                (end, {}),
            ],
            "Gesture_Think": [
                (1, {}),
                (mid, {"upper_arm_r": (-0.12, -0.04, -0.72), "forearm_r": (-0.62, -0.08, 1.02), "hand_r": (0.14, -0.16, 0.10), "finger_1_r": (0.26, 0.0, 0.0), "finger_2_r": (0.18, 0.0, 0.0), "finger_3_r": (0.30, 0.0, 0.0), "head": (-0.08, 0.12, 0.02)}),
                (end, {}),
            ],
            "Gesture_OpenHand": [
                (1, {}),
                (mid, {"upper_arm_r": (-0.07, -0.03, -0.78), "forearm_r": (-0.58, -0.06, 0.78), "hand_r": (0.0, -0.30, 0.10), "finger_1_r": (0.02, 0.12, 0.0), "finger_2_r": (0.02, 0.0, 0.0), "finger_3_r": (0.02, -0.12, 0.0)}),
                (end, {}),
            ],
            "Gesture_Fist": [
                (1, {}),
                (mid, {"upper_arm_r": (-0.07, -0.03, -0.78), "forearm_r": (-0.58, -0.06, 0.78), "hand_r": (0.08, -0.28, 0.08), "finger_1_r": (0.24, 0.02, 0.0), "finger_2_r": (0.26, 0.0, 0.0), "finger_3_r": (0.28, -0.02, 0.0)}),
                (end, {}),
            ],
            "Gesture_WristTwist": [
                (1, {"upper_arm_r": (-0.07, -0.03, -0.78), "forearm_r": (-0.58, -0.06, 0.78), "hand_r": (0.0, 0.56, 0.08)}),
                (mid, {"upper_arm_r": (-0.07, -0.03, -0.78), "forearm_r": (-0.58, -0.06, 0.78), "hand_r": (0.0, -0.56, 0.08)}),
                (end, {"upper_arm_r": (-0.07, -0.03, -0.78), "forearm_r": (-0.58, -0.06, 0.78), "hand_r": (0.0, 0.56, 0.08)}),
            ],
            "Gesture_FingerWave": [
                (1, {"upper_arm_r": (-0.07, -0.03, -0.78), "forearm_r": (-0.58, -0.06, 0.78), "hand_r": (0.0, -0.20, 0.08), "finger_1_r": (0.05, 0.10, 0.0), "finger_2_r": (0.05, 0.0, 0.0), "finger_3_r": (0.05, -0.10, 0.0)}),
                (max(2, mid // 2), {"upper_arm_r": (-0.07, -0.03, -0.78), "forearm_r": (-0.58, -0.06, 0.78), "hand_r": (0.0, -0.20, 0.08), "finger_1_r": (0.22, 0.08, 0.0), "finger_2_r": (0.08, 0.0, 0.0), "finger_3_r": (0.05, -0.08, 0.0)}),
                (mid, {"upper_arm_r": (-0.07, -0.03, -0.78), "forearm_r": (-0.58, -0.06, 0.78), "hand_r": (0.0, -0.20, 0.08), "finger_1_r": (0.08, 0.08, 0.0), "finger_2_r": (0.22, 0.0, 0.0), "finger_3_r": (0.08, -0.08, 0.0)}),
                (mid + max(2, mid // 2), {"upper_arm_r": (-0.07, -0.03, -0.78), "forearm_r": (-0.58, -0.06, 0.78), "hand_r": (0.0, -0.20, 0.08), "finger_1_r": (0.05, 0.08, 0.0), "finger_2_r": (0.08, 0.0, 0.0), "finger_3_r": (0.22, -0.08, 0.0)}),
                (end, {"upper_arm_r": (-0.07, -0.03, -0.78), "forearm_r": (-0.58, -0.06, 0.78), "hand_r": (0.0, -0.20, 0.08), "finger_1_r": (0.05, 0.10, 0.0), "finger_2_r": (0.05, 0.0, 0.0), "finger_3_r": (0.05, -0.10, 0.0)}),
            ],
            "Gesture_Nod": [(1, {"head": (0.0, 0.0, -0.03)}), (mid, {"head": (0.0, 0.0, 0.16)}), (end, {"head": (0.0, 0.0, -0.03)})],
            "Gesture_ShakeHead": [(1, {"head": (0.0, -0.18, 0.0)}), (mid, {"head": (0.0, 0.18, 0.0)}), (end, {"head": (0.0, -0.18, 0.0)})],
            "Look_Left": [(1, {"head": (0.0, 0.14, 0.0), "eye_l": (0.0, 0.12, 0.0), "eye_r": (0.0, 0.12, 0.0)}), (end, {"head": (0.0, 0.14, 0.0), "eye_l": (0.0, 0.12, 0.0), "eye_r": (0.0, 0.12, 0.0)})],
            "Look_Right": [(1, {"head": (0.0, -0.14, 0.0), "eye_l": (0.0, -0.12, 0.0), "eye_r": (0.0, -0.12, 0.0)}), (end, {"head": (0.0, -0.14, 0.0), "eye_l": (0.0, -0.12, 0.0), "eye_r": (0.0, -0.12, 0.0)})],
            "Expression_Happy": [(1, {"head": (-0.035, 0.0, -0.015)}), (mid, {"head": (0.035, 0.0, 0.02)}), (end, {"head": (-0.035, 0.0, -0.015)})],
            "Expression_Thinking": [(1, {"head": (-0.10, 0.14, 0.0)}), (end, {"head": (-0.10, 0.14, 0.0)})],
            "Expression_Surprised": [(1, {"head": (0.0, 0.0, -0.08)}), (end, {"head": (0.0, 0.0, -0.08)})],
            "Expression_Confused": [(1, {"head": (0.11, -0.08, 0.02)}), (end, {"head": (0.11, -0.08, 0.02)})],
            "Expression_Serious": [(1, {"head": (0.0, 0.0, 0.06)}), (end, {"head": (0.0, 0.0, 0.06)})],
        }
        action_specs.update(source_pose_specs)

        action_specs["Gesture_Count_One"] = [
            (1, {}),
            (mid, {"upper_arm_r": (-0.06, -0.02, -0.72), "forearm_r": (-0.72, -0.06, 0.82), "hand_r": (0.10, -0.28, 0.18), "finger_1_r": (0.03, 0.04, 0.0), "finger_2_r": (0.24, 0.0, 0.0), "finger_3_r": (0.28, -0.04, 0.0)}),
            (end, {}),
        ]
        action_specs["Gesture_Count_Two"] = [
            (1, {}),
            (mid, {"upper_arm_r": (-0.06, -0.02, -0.72), "forearm_r": (-0.72, -0.06, 0.82), "hand_r": (0.10, -0.28, 0.18), "finger_1_r": (0.03, 0.08, 0.0), "finger_2_r": (0.03, -0.08, 0.0), "finger_3_r": (0.28, 0.0, 0.0)}),
            (end, {}),
        ]
        action_specs["Gesture_Pinch"] = [
            (1, {}),
            (mid, {"upper_arm_r": (-0.06, -0.02, -0.82), "forearm_r": (-0.64, -0.06, 0.72), "hand_r": (0.12, -0.26, 0.16), "finger_1_r": (0.18, 0.08, -0.04), "finger_2_r": (0.08, 0.0, 0.0), "finger_3_r": (0.22, -0.08, 0.04)}),
            (end, {}),
        ]

    action_specs["Gesture_Count_Three"] = [
        (1, {}),
        (mid, {"upper_arm_r": (-0.06, -0.02, -0.72), "forearm_r": (-0.72, -0.06, 0.82), "hand_r": (0.10, -0.28, 0.18)}),
        (end, {}),
    ]

    def mark_hand_pose(action_name: str, frame: int, side: str, pose_name: str) -> None:
        for keyframe, rotations in action_specs[action_name]:
            if keyframe == frame:
                rotations[f"__digit_pose_{side}"] = pose_name

    for side in ("l", "r"):
        mark_hand_pose("Gesture_OpenHand", mid, side, "open_hand")
        mark_hand_pose("Gesture_Fist", mid, side, "fist")
    mark_hand_pose("Gesture_Pinch", mid, "r", "pinch")
    mark_hand_pose("Gesture_Count_One", mid, "r", "count_one")
    mark_hand_pose("Gesture_Count_Two", mid, "r", "count_two")
    mark_hand_pose("Gesture_Count_Three", mid, "r", "count_three")
    mark_hand_pose("Gesture_Point_Left", mid, "l", "point")
    mark_hand_pose("Gesture_Point_Right", mid, "r", "point")
    mark_hand_pose("Gesture_FingerWave", 1, "r", "open_hand")
    mark_hand_pose("Gesture_FingerWave", end, "r", "open_hand")
    for frame, pose_name in (
        (max(2, mid // 2), "finger_roll_1"),
        (mid, "finger_roll_2"),
        (mid + max(2, mid // 2), "finger_roll_3"),
    ):
        mark_hand_pose("Gesture_FingerWave", frame, "r", pose_name)
    for frame, _ in action_specs["Gesture_Wave"]:
        if frame not in {1, end}:
            mark_hand_pose("Gesture_Wave", frame, "r", "open_hand")

    action_specs.update(aroll_actions.build_aroll_action_specs(source_rig, fps))

    actions = [build_action(name, keyframes) for name, keyframes in action_specs.items()]

    mouth_actions: list[str] = []
    face_actions: list[str] = []
    mouth = face.get("mouth")
    expression_shapes = {
        "Expression_Happy_Mouth": "Mouth_Smile",
        "Expression_Thinking_Mouth": "Mouth_MBP",
        "Expression_Surprised_Mouth": "Mouth_Surprise",
        "Expression_Confused_Mouth": "Mouth_Frown",
        "Expression_Serious_Mouth": "Mouth_MBP",
    }
    if mouth and mouth.type == "MESH" and mouth.data.shape_keys:
        keys = mouth.data.shape_keys
        keys.animation_data_create()
        previous_mouth_action = keys.animation_data.action
        for action_name, active_shape in expression_shapes.items():
            existing = bpy.data.actions.get(action_name)
            if existing:
                existing.use_fake_user = True
                mouth_actions.append(existing.name)
                continue
            keys.animation_data.action = None
            for key in keys.key_blocks:
                if key.name == "Basis":
                    continue
                key.value = 1.0 if key.name == active_shape else 0.0
                key.keyframe_insert(data_path="value", frame=1)
                key.keyframe_insert(data_path="value", frame=end)
            action = keys.animation_data.action
            if action:
                action.name = action_name
                action.use_fake_user = True
                mouth_actions.append(action.name)
        keys.animation_data.action = previous_mouth_action

        face_specs: dict[str, list[tuple[int, dict[str, float]]]] = {
            "Face_Neutral": [(1, {}), (end, {})],
            "Face_Happy": [
                (1, {"Mouth_Smile": 0.72, "Cheek_Smile.L": 0.72, "Cheek_Smile.R": 0.72, "Eye_Squint.L": 0.24, "Eye_Squint.R": 0.24}),
                (end, {"Mouth_Smile": 0.72, "Cheek_Smile.L": 0.72, "Cheek_Smile.R": 0.72, "Eye_Squint.L": 0.24, "Eye_Squint.R": 0.24}),
            ],
            "Face_Thinking": [
                (1, {"Mouth_MBP": 0.28, "Brow_Raise.L": 0.58, "Brow_Furrow.R": 0.18, "Eye_Look_Right": 0.38}),
                (end, {"Mouth_MBP": 0.28, "Brow_Raise.L": 0.58, "Brow_Furrow.R": 0.18, "Eye_Look_Right": 0.38}),
            ],
            "Face_Surprised": [
                (1, {"Mouth_Surprise": 0.82, "Eye_Wide.L": 0.78, "Eye_Wide.R": 0.78, "Brow_Raise.L": 0.74, "Brow_Raise.R": 0.74}),
                (end, {"Mouth_Surprise": 0.82, "Eye_Wide.L": 0.78, "Eye_Wide.R": 0.78, "Brow_Raise.L": 0.74, "Brow_Raise.R": 0.74}),
            ],
            "Face_Confused": [
                (1, {"Mouth_Frown": 0.38, "Brow_Raise.L": 0.52, "Brow_Furrow.R": 0.48, "Eye_Look_Left": 0.24}),
                (end, {"Mouth_Frown": 0.38, "Brow_Raise.L": 0.52, "Brow_Furrow.R": 0.48, "Eye_Look_Left": 0.24}),
            ],
            "Face_Serious": [
                (1, {"Mouth_MBP": 0.34, "Brow_Furrow.L": 0.52, "Brow_Furrow.R": 0.52}),
                (end, {"Mouth_MBP": 0.34, "Brow_Furrow.L": 0.52, "Brow_Furrow.R": 0.52}),
            ],
            "Face_Squint": [
                (1, {}),
                (max(2, fps // 6), {"Eye_Squint.L": 1.0, "Eye_Squint.R": 1.0}),
                (max(3, fps // 3), {}),
            ],
        }
        previous_face_action = keys.animation_data.action
        for action_name, keyframes in face_specs.items():
            existing = bpy.data.actions.get(action_name)
            if existing:
                existing.use_fake_user = True
                face_actions.append(existing.name)
                continue
            keys.animation_data.action = None
            for frame, values in keyframes:
                for key in keys.key_blocks:
                    if key.name == "Basis":
                        continue
                    key.value = float(values.get(key.name, 0.0))
                    key.keyframe_insert(data_path="value", frame=frame)
            action = keys.animation_data.action
            if action:
                action.name = action_name
                action.use_fake_user = True
                face_actions.append(action.name)
        keys.animation_data.action = previous_face_action

    armature.animation_data.action = previous_action
    bpy.context.scene.frame_set(1)
    reset_pose()
    return {"actions": actions, "mouthActions": mouth_actions, "faceActions": face_actions}


def save_rigged_assets(
    data: dict,
    character_objects: list[bpy.types.Object],
    imported_asset_objects: list[bpy.types.Object],
    armature: bpy.types.Object,
    face: dict[str, bpy.types.Object],
    removed_helpers: list[str],
    rig_stats: dict[str, Any],
    scene_stats: dict[str, Any] | None = None,
) -> None:
    blend_path = Path(data["riggedBlendPath"])
    glb_path = Path(data["riggedGlbPath"])
    report_path = Path(data["rigReportPath"])
    for path in (blend_path, glb_path, report_path):
        path.parent.mkdir(parents=True, exist_ok=True)

    bpy.context.scene.frame_set(1)
    bpy.ops.wm.save_as_mainfile(filepath=str(blend_path))
    bpy.ops.object.select_all(action="DESELECT")
    face_objects = list(dict.fromkeys(face.values()))
    asset_objects = list(dict.fromkeys(character_objects + imported_asset_objects + face_objects + [armature]))
    for obj in asset_objects:
        obj.hide_viewport = False
        obj.hide_render = False
        obj.select_set(True)
    bpy.context.view_layer.objects.active = armature
    export_args = {
        "filepath": str(glb_path),
        "export_format": "GLB",
        "use_selection": True,
        "export_animations": True,
        "export_skins": True,
        "export_morph": True,
    }
    try:
        if bpy.ops.export_scene.gltf.get_rna_type().properties.get("export_extras") is not None:
            export_args["export_extras"] = True
    except (AttributeError, RuntimeError):
        pass
    try:
        bpy.ops.export_scene.gltf(**export_args, export_animation_mode="ACTIONS")
    except TypeError:
        try:
            bpy.ops.export_scene.gltf(**export_args, export_animation_mode="ACTIVE_ACTIONS")
        except TypeError:
            bpy.ops.export_scene.gltf(**export_args)

    materials = sorted(
        {
            material_slot.name
            for obj in asset_objects
            if obj.type == "MESH"
            for material_slot in obj.data.materials
            if material_slot
        }
    )
    mouth = face.get("mouth")
    mouth_shape_keys = []
    mouth_cleanup: dict[str, Any] = {}
    if mouth and mouth.type == "MESH" and mouth.data.shape_keys:
        mouth_shape_keys = [key.name for key in mouth.data.shape_keys.key_blocks if key.name != "Basis"]
        try:
            mouth_cleanup = json.loads(str(mouth.get("source_mouth_cleanup") or "{}"))
        except json.JSONDecodeError:
            mouth_cleanup = {}
    report = {
        "success": blend_path.is_file() and glb_path.is_file(),
        "rigMode": str(data.get("rigMode") or "auto"),
        "resolvedRigMode": rig_stats.get("resolvedRigMode", ""),
        "inputRigPreserved": bool(rig_stats.get("inputRigPreserved")),
        "presenterControlsReady": bool(rig_stats.get("presenterControlsReady")),
        "boneMap": rig_stats.get("boneMap", {}),
        "elbowRig": bool(data.get("elbowRig", True)),
        "armature": armature.name,
        "bones": [bone.name for bone in armature.data.bones],
        "characterObjects": [obj.name for obj in character_objects],
        "removedImportHelpers": removed_helpers,
        "sourceMaterialsPreserved": True,
        "materials": materials,
        "rigidVertexCounts": rig_stats.get("weightedVertexCounts", {}),
        "weighting": rig_stats,
        "faceScreenMode": str(data.get("faceScreenMode") or "source"),
        "mouthMode": str(data.get("mouthMode") or "auto"),
        "existingMouthPreserved": bool(mouth and mouth.get("existing_viseme_mouth_preserved")),
        "mouthObject": mouth.name if mouth else "",
        "mouthCleanupPatch": face.get("patch").name if face.get("patch") else "",
        "sourceMouthRemoved": bool(mouth and mouth.get("source_mouth_removed")),
        "sourceMouthOccluded": bool(face.get("patch")) or bool(mouth and mouth.get("source_mouth_removed")),
        "sourceMouthCleanup": mouth_cleanup,
        "mouthShapeKeys": mouth_shape_keys,
        "facialTopologyMode": str(data.get("facialTopologyMode") or "source_only"),
        "facialTopologyObjects": {
            role: face[role].name
            for role in (*VOLUMETRIC_FACE_ROLES, *INTEGRATED_FACE_ROLES)
            if face.get(role)
        },
        "integratedSourceRetopology": bool(mouth and mouth.get("integrated_mouth_seam")),
        "mouthSeamEdgeCount": int(mouth.get("mouth_seam_edge_count", 0)) if mouth else 0,
        "upperLipBoundaryVertexCount": int(mouth.get("mouth_upper_boundary_count", 0)) if mouth else 0,
        "lowerLipBoundaryVertexCount": int(mouth.get("mouth_lower_boundary_count", 0)) if mouth else 0,
        "trueLipTopology": bool(mouth and mouth.get("integrated_mouth_seam"))
        or all(face.get(role) for role in ("upper_lip", "lower_lip", "mouth_cavity")),
        "trueEyelidTopology": bool(mouth and mouth.get("true_eyelid_topology")) or all(
            face.get(role)
            for role in ("upper_lid_l", "lower_lid_l", "upper_lid_r", "lower_lid_r")
        ),
        "sourceEyelidShapeKeys": bool(
            mouth
            and mouth.data.shape_keys
            and mouth.data.shape_keys.key_blocks.get("Eye_Blink.L")
            and mouth.data.shape_keys.key_blocks.get("Eye_Blink.R")
        ),
        "blinkCapability": str(mouth.get("blink_capability") or "none") if mouth else "none",
        "sourceSquintShapeKeys": bool(
            mouth
            and mouth.data.shape_keys
            and mouth.data.shape_keys.key_blocks.get("Eye_Squint.L")
            and mouth.data.shape_keys.key_blocks.get("Eye_Squint.R")
        ),
        "facialBoneControls": {
            role: rig_stats.get("boneMap", {}).get(role, "")
            for role in ("jaw", "eye_l", "eye_r", "tongue_1", "tongue_2", "tongue_3")
        },
        "oralInteriorReady": all(face.get(role) for role in INTEGRATED_FACE_ROLES),
        "embeddedVisemeApi": "IP_Viseme_API.py" if bpy.data.texts.get("IP_Viseme_API.py") else "",
        "actions": sorted(action.name for action in bpy.data.actions),
        "scene": scene_stats or {"sceneMode": str(data.get("backgroundMode") or "transparent_or_world")},
        "legacyFakeFaceObjects": [
            obj.name
            for obj in bpy.data.objects
            if obj.name in {"dynamic_face_screen", "dynamic_left_eye", "dynamic_right_eye", "dynamic_mouth"}
        ],
        "legacyVisibleFaceOverlays": [
            obj.name
            for obj in bpy.data.objects
            if obj.name in {
                "IP_UpperLip", "IP_LowerLip",
                "IP_UpperLid.L", "IP_LowerLid.L", "IP_UpperLid.R", "IP_LowerLid.R",
            }
        ],
    }
    report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")


def main() -> None:
    data = read_input()
    scene_mode = bool(data.get("sceneBlendPath"))
    use_master = bool(data.get("useMasterAsset"))
    presentation_mode = str(data.get("presentationMode") or "standing").strip().lower()
    mode_objects: dict[str, Any] | None = None
    if scene_mode:
        load_scene_template(str(data["sceneBlendPath"]))
        mode_objects = resolve_authored_scene_mode_objects(data)
    else:
        clear_scene()
    target_height = scene_target_height(data)
    if use_master:
        imported_assets = append_master_collection(
            Path(str(data["masterBlendPath"])),
            bpy.context.scene.collection,
        )
        character_objects = [obj for obj in imported_assets if obj.type == "MESH"]
        imported_armatures = [obj for obj in imported_assets if obj.type == "ARMATURE"]
        removed_helpers: list[str] = []
        if not character_objects:
            raise RuntimeError(f"configured character master has no Mesh objects: {data['masterBlendPath']}")
    else:
        character_objects, imported_armatures, imported_assets, removed_helpers = import_model(data["modelPath"])
    preserve_hierarchy = bool(imported_armatures and data.get("preserveExistingRig", True))
    dimensions = prepare_character(
        character_objects,
        target_height=target_height,
        preserve_hierarchy=preserve_hierarchy,
        asset_objects=imported_assets,
    )
    scene_stats: dict[str, Any] = {}
    if scene_mode:
        scene_stats = setup_authored_scene(data, dimensions, mode_objects)
    elif not bool(data.get("assetOnly")):
        setup_scene(data, dimensions)
    armature, rig_stats, bone_map = choose_character_rig(data, imported_armatures, character_objects, dimensions)
    face = setup_face(data, dimensions, armature, character_objects, bone_map)
    if use_master:
        rig_stats["renderDetail"] = {
            "mode": "master_preserved",
            "detailedObjects": [],
            "skippedObjects": [obj.name for obj in character_objects],
        }
    else:
        rig_stats["renderDetail"] = configure_character_render_detail(character_objects, data)
    rig_stats.update(_collect_weight_stats(character_objects))
    rig_stats["boneMap"] = bone_map
    rig_stats["runtimeActions"] = reconcile_runtime_timeline_actions(armature, face)
    animate(
        armature,
        face,
        data["motionPlan"],
        int(data["fps"]),
        bone_map,
        runtime_actions_prepared=True,
        presentation_mode=presentation_mode,
    )
    rig_stats["actionLibrary"] = create_action_library(armature, face, bone_map, int(data["fps"]))
    if bool(data.get("prepareMaster")):
        container = dimensions.get("container")
        master_objects = list(
            dict.fromkeys(
                character_objects
                + imported_assets
                + list(face.values())
                + ([container] if container else [])
            )
        )
        rig_stats["masterAsset"] = save_master_collection(
            character_objects=master_objects,
            armature=armature,
            output_path=Path(data["riggedBlendPath"]),
        )
    if scene_mode:
        scene_stats["placement"] = place_character_in_authored_scene(
            character_objects,
            imported_assets,
            armature,
            face,
            dimensions,
            mode_objects,
        )
        if mode_objects:
            placement = bpy.data.objects[scene_stats["placement"]["placementRoot"]]
            scene_stats["collisionPlacement"] = calibrate_mode_collision_clearance(
                character_objects,
                placement,
            )
            scene_stats["footContact"] = calibrate_mode_foot_contact(
                character_objects,
                armature,
                bone_map,
                placement,
                mode_objects,
            )
            scene_stats["mediumFraming"] = calibrate_mode_medium_camera(
                character_objects,
                bone_map,
                mode_objects,
            )
    container = dimensions.get("container")
    export_assets = imported_assets + ([container] if container else [])
    save_rigged_assets(
        data,
        character_objects,
        export_assets,
        armature,
        face,
        removed_helpers,
        rig_stats,
        scene_stats,
    )

    if bool(data.get("assetOnly")):
        return

    frames_dir = Path(data["framesDir"])
    frames_dir.mkdir(parents=True, exist_ok=True)
    bpy.context.scene.render.image_settings.file_format = "PNG"
    bpy.context.scene.render.image_settings.color_mode = "RGBA"
    bpy.context.scene.render.image_settings.color_depth = "8"
    bpy.context.scene.render.filepath = str(frames_dir / "frame_")
    bpy.context.scene.frame_set(1)
    bpy.ops.render.render(animation=True)


if __name__ == "__main__":
    main()
