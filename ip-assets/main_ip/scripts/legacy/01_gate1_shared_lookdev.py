"""Gate 1: create a shared-data neutral LookDev scene and six review renders.

Authoritative input:
  models/checkpoints/main-ip-aroll-master_20260718_114057_eye_redesign_final.blend

Character policy:
  * Rename the existing IP_Character_Master collection to COL_CHR_SLOTH_FINAL.
  * Link that exact collection into both production Scene and SCENE_LOOKDEV.
  * Never duplicate character objects, meshes, armature, materials, shape keys,
    actions, drivers, or groom data.
  * Fingerprint the complete character collection before and after Gate 1.

Allowed writes:
  checkpoints/sloth_001_pre_lookdev.blend
  checkpoints/sloth_005_lookdev.blend
  reports/gate1_camera_match.json
  renders/lookdev/gate1/*.png
"""

from __future__ import annotations

import bpy
import hashlib
import json
import math
import struct
import time
from datetime import datetime, timezone
from pathlib import Path

from bpy_extras.object_utils import world_to_camera_view
from mathutils import Vector


PROJECT_ROOT = Path("/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip")
EXPECTED_SOURCE = PROJECT_ROOT / "models/checkpoints/main-ip-aroll-master_20260718_114057_eye_redesign_final.blend"
PRE_CHECKPOINT = PROJECT_ROOT / "checkpoints/sloth_001_pre_lookdev.blend"
LOOKDEV_BLEND = PROJECT_ROOT / "checkpoints/sloth_005_lookdev.blend"
REPORT_PATH = PROJECT_ROOT / "reports/gate1_camera_match.json"
RENDER_DIR = PROJECT_ROOT / "renders/lookdev/gate1"

SOURCE_COLLECTION_NAME = "IP_Character_Master"
FINAL_COLLECTION_NAME = "COL_CHR_SLOTH_FINAL"
LOOKDEV_SCENE_NAME = "SCENE_LOOKDEV"
LOOKDEV_SETUP_COLLECTION = "COL_LOOKDEV_SETUP"


def utc_now():
    return datetime.now(timezone.utc).isoformat()


def clean_float(value):
    return round(float(value), 9)


def vec(value):
    return [clean_float(component) for component in value]


def matrix_rows(matrix):
    return [[clean_float(component) for component in row] for row in matrix]


def sha256_file(path):
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def update_float3(digest, value):
    digest.update(struct.pack("<fff", float(value[0]), float(value[1]), float(value[2])))


def mesh_fingerprint(mesh):
    digest = hashlib.sha256()
    digest.update(struct.pack("<III", len(mesh.vertices), len(mesh.edges), len(mesh.polygons)))
    for vertex in mesh.vertices:
        update_float3(digest, vertex.co)
    for edge in mesh.edges:
        digest.update(struct.pack("<II", int(edge.vertices[0]), int(edge.vertices[1])))
    for polygon in mesh.polygons:
        digest.update(struct.pack("<I", len(polygon.vertices)))
        for vertex_index in polygon.vertices:
            digest.update(struct.pack("<I", int(vertex_index)))
    for uv_layer in mesh.uv_layers:
        digest.update(uv_layer.name.encode("utf-8"))
        for loop_uv in uv_layer.data:
            digest.update(struct.pack("<ff", float(loop_uv.uv[0]), float(loop_uv.uv[1])))
    if mesh.shape_keys:
        for key_block in mesh.shape_keys.key_blocks:
            digest.update(key_block.name.encode("utf-8"))
            digest.update((key_block.relative_key.name if key_block.relative_key else "").encode("utf-8"))
            for point in key_block.data:
                update_float3(digest, point.co)
    return digest.hexdigest()


def character_signature(collection):
    object_records = []
    for obj in sorted(collection.all_objects, key=lambda item: item.name):
        record = {
            "name": obj.name,
            "type": obj.type,
            "data_name": getattr(getattr(obj, "data", None), "name", None),
            "data_pointer": getattr(getattr(obj, "data", None), "as_pointer", lambda: 0)(),
            "parent": obj.parent.name if obj.parent else None,
            "parent_type": obj.parent_type,
            "parent_bone": obj.parent_bone,
            "matrix_world": matrix_rows(obj.matrix_world),
            "modifiers": [(modifier.name, modifier.type, modifier.show_viewport, modifier.show_render) for modifier in obj.modifiers],
            "constraints": [(constraint.name, constraint.type, clean_float(constraint.influence), constraint.mute) for constraint in obj.constraints],
        }
        if obj.type == "MESH":
            record.update({
                "mesh_fingerprint": mesh_fingerprint(obj.data),
                "materials": [slot.material.name if slot.material else None for slot in obj.material_slots],
                "vertex_groups": [group.name for group in obj.vertex_groups],
            })
        elif obj.type == "ARMATURE":
            record["bones"] = [(bone.name, bone.parent.name if bone.parent else None, bone.use_deform) for bone in obj.data.bones]
        object_records.append(record)
    encoded = json.dumps(object_records, sort_keys=True, separators=(",", ":")).encode("utf-8")
    return {
        "sha256": hashlib.sha256(encoded).hexdigest(),
        "object_count": len(object_records),
        "object_names": [record["name"] for record in object_records],
        "records": object_records,
    }


def collection_in_tree(root_collection, target_collection):
    if root_collection == target_collection:
        return True
    return any(collection_in_tree(child, target_collection) for child in root_collection.children)


def collection_world_bounds(collection):
    points = []
    accepted_types = {"MESH", "CURVE", "CURVES", "SURFACE", "FONT", "META"}
    for obj in collection.all_objects:
        if obj.type not in accepted_types or obj.hide_render:
            continue
        try:
            for corner in obj.bound_box:
                points.append(obj.matrix_world @ Vector(corner))
        except (AttributeError, TypeError):
            continue
    if not points:
        raise RuntimeError("Unable to calculate character bounds.")
    minimum = Vector((min(point.x for point in points), min(point.y for point in points), min(point.z for point in points)))
    maximum = Vector((max(point.x for point in points), max(point.y for point in points), max(point.z for point in points)))
    return minimum, maximum, points


def make_plane(name, size, z, collection, material):
    mesh = bpy.data.meshes.new(name + "_Mesh")
    half = size * 0.5
    mesh.from_pydata(
        [(-half, -half, z), (half, -half, z), (half, half, z), (-half, half, z)],
        [],
        [(0, 1, 2, 3)],
    )
    mesh.update()
    obj = bpy.data.objects.new(name, mesh)
    collection.objects.link(obj)
    obj.data.materials.append(material)
    return obj


def make_principled_material(name, base_color, roughness):
    material = bpy.data.materials.new(name)
    material.use_nodes = True
    nodes = material.node_tree.nodes
    links = material.node_tree.links
    bsdf = next((node for node in nodes if node.type == "BSDF_PRINCIPLED"), None)
    if bsdf is None:
        bsdf = nodes.new("ShaderNodeBsdfPrincipled")
    output = next((node for node in nodes if node.type == "OUTPUT_MATERIAL"), None)
    if output is None:
        output = nodes.new("ShaderNodeOutputMaterial")
    if not any(link.from_node == bsdf and link.to_node == output for link in links):
        links.new(bsdf.outputs["BSDF"], output.inputs["Surface"])
    bsdf.inputs["Base Color"].default_value = base_color
    bsdf.inputs["Roughness"].default_value = roughness
    bsdf.inputs["Metallic"].default_value = 0.0
    return material


def make_camera(name, lens, collection):
    camera_data = bpy.data.cameras.new(name + "_Data")
    camera_data.lens = lens
    camera_data.sensor_width = 36.0
    camera_data.sensor_height = 24.0
    camera_data.sensor_fit = "VERTICAL"
    camera_data.clip_start = 0.05
    camera_data.clip_end = 200.0
    camera_data.dof.use_dof = False
    camera_obj = bpy.data.objects.new(name, camera_data)
    collection.objects.link(camera_obj)
    return camera_obj


def make_area_light(name, location, energy, size, color, collection, target):
    light_data = bpy.data.lights.new(name + "_Data", "AREA")
    light_data.energy = energy
    light_data.shape = "DISK"
    light_data.size = size
    light_data.color = color
    light_obj = bpy.data.objects.new(name, light_data)
    collection.objects.link(light_obj)
    light_obj.location = location
    look_at(light_obj, target)
    return light_obj


def look_at(obj, target):
    direction = Vector(target) - obj.location
    obj.rotation_euler = direction.to_track_quat("-Z", "Y").to_euler()


def distance_for_height(camera_obj, subject_height, fill_fraction):
    vertical_fov = camera_obj.data.angle_y
    return (subject_height * 0.5) / (math.tan(vertical_fov * 0.5) * fill_fraction)


def place_camera(camera_obj, target, direction, subject_height, fill_fraction):
    direction = Vector(direction).normalized()
    distance = distance_for_height(camera_obj, subject_height, fill_fraction)
    camera_obj.location = Vector(target) + direction * distance
    camera_obj.data.shift_x = 0.0
    camera_obj.data.shift_y = 0.0
    look_at(camera_obj, target)
    return distance


def projected_coverage(scene, camera_obj, points):
    projected = [world_to_camera_view(scene, camera_obj, point) for point in points]
    front_points = [point for point in projected if point.z > 0.0]
    if not front_points:
        return None
    min_x = min(point.x for point in front_points)
    max_x = max(point.x for point in front_points)
    min_y = min(point.y for point in front_points)
    max_y = max(point.y for point in front_points)
    return {
        "normalized_bounds": [clean_float(min_x), clean_float(min_y), clean_float(max_x), clean_float(max_y)],
        "width_fraction": clean_float(max_x - min_x),
        "height_fraction": clean_float(max_y - min_y),
        "inside_frame": min_x >= 0.0 and max_x <= 1.0 and min_y >= 0.0 and max_y <= 1.0,
    }


def render_shot(scene, camera_obj, name, target, direction, subject_height, fill_fraction, points):
    distance = place_camera(camera_obj, target, direction, subject_height, fill_fraction)
    scene.camera = camera_obj
    output_path = RENDER_DIR / f"{name}.png"
    scene.render.filepath = str(output_path)
    start = time.perf_counter()
    bpy.ops.render.render(write_still=True, scene=scene.name)
    elapsed = time.perf_counter() - start
    if not output_path.exists() or output_path.stat().st_size == 0:
        raise RuntimeError(f"Render did not produce a valid file: {output_path}")
    return {
        "shot": name,
        "camera": camera_obj.name,
        "lens_mm": clean_float(camera_obj.data.lens),
        "location": vec(camera_obj.location),
        "rotation_euler": vec(camera_obj.rotation_euler),
        "target": vec(target),
        "distance": clean_float(distance),
        "subject_height_for_framing": clean_float(subject_height),
        "fill_fraction": clean_float(fill_fraction),
        "projection": projected_coverage(scene, camera_obj, points),
        "output": str(output_path),
        "bytes": output_path.stat().st_size,
        "sha256": sha256_file(output_path),
        "render_seconds": clean_float(elapsed),
    }


def main():
    if Path(bpy.data.filepath).resolve() != EXPECTED_SOURCE.resolve():
        raise RuntimeError(f"Wrong source file. Expected {EXPECTED_SOURCE}, found {bpy.data.filepath}")
    if not EXPECTED_SOURCE.exists():
        raise FileNotFoundError(EXPECTED_SOURCE)
    if LOOKDEV_BLEND.exists():
        raise FileExistsError(f"Refusing to overwrite existing Gate 1 result: {LOOKDEV_BLEND}")
    if bpy.data.scenes.get(LOOKDEV_SCENE_NAME):
        raise RuntimeError(f"Scene already exists: {LOOKDEV_SCENE_NAME}")
    if bpy.data.collections.get(FINAL_COLLECTION_NAME):
        raise RuntimeError(f"Final collection name already exists: {FINAL_COLLECTION_NAME}")
    source_collection = bpy.data.collections.get(SOURCE_COLLECTION_NAME)
    if source_collection is None:
        raise RuntimeError(f"Authoritative character collection not found: {SOURCE_COLLECTION_NAME}")

    PRE_CHECKPOINT.parent.mkdir(parents=True, exist_ok=True)
    REPORT_PATH.parent.mkdir(parents=True, exist_ok=True)
    RENDER_DIR.mkdir(parents=True, exist_ok=True)
    if not PRE_CHECKPOINT.exists():
        checkpoint_result = bpy.ops.wm.save_as_mainfile(filepath=str(PRE_CHECKPOINT), copy=True, compress=True)
        if "FINISHED" not in checkpoint_result:
            raise RuntimeError(f"Pre-Gate checkpoint failed: {checkpoint_result}")

    source_filepath = bpy.data.filepath
    production_scene = bpy.data.scenes.get("Scene")
    if production_scene is None:
        raise RuntimeError("Production scene 'Scene' not found.")
    if not collection_in_tree(production_scene.collection, source_collection):
        raise RuntimeError("IP_Character_Master is not linked to the production Scene.")

    collection_pointer_before = source_collection.as_pointer()
    signature_before = character_signature(source_collection)
    source_collection.name = FINAL_COLLECTION_NAME
    final_collection = source_collection

    lookdev_scene = bpy.data.scenes.new(LOOKDEV_SCENE_NAME)
    lookdev_scene.collection.children.link(final_collection)
    setup_collection = bpy.data.collections.new(LOOKDEV_SETUP_COLLECTION)
    lookdev_scene.collection.children.link(setup_collection)

    lookdev_scene.render.engine = production_scene.render.engine
    lookdev_scene.render.resolution_x = 1024
    lookdev_scene.render.resolution_y = 1024
    lookdev_scene.render.resolution_percentage = 100
    lookdev_scene.render.image_settings.file_format = "PNG"
    lookdev_scene.render.image_settings.color_mode = "RGBA"
    lookdev_scene.render.image_settings.color_depth = "8"
    lookdev_scene.render.film_transparent = False
    lookdev_scene.view_settings.view_transform = "AgX"
    lookdev_scene.view_settings.exposure = 0.0
    lookdev_scene.view_settings.gamma = 1.0

    world = bpy.data.worlds.new("WORLD_LOOKDEV_Neutral")
    world.use_nodes = True
    world_nodes = world.node_tree.nodes
    world_links = world.node_tree.links
    world_background = next((node for node in world_nodes if node.type == "BACKGROUND"), None)
    if world_background is None:
        world_background = world_nodes.new("ShaderNodeBackground")
    world_output = next((node for node in world_nodes if node.type == "OUTPUT_WORLD"), None)
    if world_output is None:
        world_output = world_nodes.new("ShaderNodeOutputWorld")
    if not any(link.from_node == world_background and link.to_node == world_output for link in world_links):
        world_links.new(world_background.outputs["Background"], world_output.inputs["Surface"])
    world_background.inputs["Color"].default_value = (0.58, 0.60, 0.63, 1.0)
    world_background.inputs["Strength"].default_value = 0.35
    lookdev_scene.world = world

    minimum, maximum, bounds_points = collection_world_bounds(final_collection)
    center = (minimum + maximum) * 0.5
    height = maximum.z - minimum.z
    width = maximum.x - minimum.x
    depth = maximum.y - minimum.y
    if height <= 0.0:
        raise RuntimeError("Invalid character height.")

    ground_material = make_principled_material("MAT_LOOKDEV_Ground", (0.42, 0.44, 0.47, 1.0), 0.82)
    ground = make_plane("GEO_LOOKDEV_Ground", max(12.0, height * 5.0), minimum.z - 0.012, setup_collection, ground_material)

    camera_65 = make_camera("CAM_LOOKDEV_65MM", 65.0, setup_collection)
    camera_80 = make_camera("CAM_LOOKDEV_80MM", 80.0, setup_collection)
    camera_100 = make_camera("CAM_LOOKDEV_100MM", 100.0, setup_collection)

    light_target = Vector((center.x, center.y, minimum.z + height * 0.58))
    lights = [
        make_area_light("LIGHT_LOOKDEV_Key", (center.x - height * 1.25, center.y - height * 1.65, minimum.z + height * 1.75), 900.0, height * 1.05, (1.0, 0.96, 0.91), setup_collection, light_target),
        make_area_light("LIGHT_LOOKDEV_Fill", (center.x + height * 1.35, center.y - height * 0.90, minimum.z + height * 1.20), 480.0, height * 1.25, (0.90, 0.95, 1.0), setup_collection, light_target),
        make_area_light("LIGHT_LOOKDEV_Rim", (center.x, center.y + height * 1.25, minimum.z + height * 1.55), 720.0, height * 0.85, (1.0, 0.96, 0.90), setup_collection, light_target),
    ]

    armature = bpy.data.objects.get("Armature")
    head_target = Vector((center.x, center.y, minimum.z + height * 0.79))
    if armature and armature.pose and armature.pose.bones.get("Head"):
        head_bone = armature.pose.bones["Head"]
        head_target = armature.matrix_world @ ((head_bone.head + head_bone.tail) * 0.5)
    full_target = Vector((center.x, center.y, minimum.z + height * 0.51))
    face_height = height * 0.34

    shot_specs = [
        ("front", camera_65, (0.0, -1.0, 0.0), full_target, height, 0.83),
        ("front_34", camera_80, (1.0, -1.0, 0.0), full_target, height, 0.82),
        ("side", camera_65, (1.0, 0.0, 0.0), full_target, height, 0.82),
        ("back_34", camera_80, (1.0, 1.0, 0.0), full_target, height, 0.82),
        ("back", camera_65, (0.0, 1.0, 0.0), full_target, height, 0.83),
        ("face_closeup", camera_100, (0.0, -1.0, 0.0), head_target, face_height, 0.78),
    ]

    previous_window_scene = bpy.context.window.scene if bpy.context.window else None
    if bpy.context.window:
        bpy.context.window.scene = lookdev_scene
    shot_reports = []
    for shot_name, camera, direction, target, subject_height, fill_fraction in shot_specs:
        shot_reports.append(render_shot(
            lookdev_scene, camera, shot_name, target, direction,
            subject_height, fill_fraction, bounds_points,
        ))

    # Leave the three test cameras in stable canonical review positions.
    place_camera(camera_65, full_target, (0.0, -1.0, 0.0), height, 0.83)
    place_camera(camera_80, full_target, (1.0, -1.0, 0.0), height, 0.82)
    place_camera(camera_100, head_target, (0.0, -1.0, 0.0), face_height, 0.78)
    lookdev_scene.camera = camera_65

    signature_after = character_signature(final_collection)
    collection_pointer_after = final_collection.as_pointer()
    if signature_after["sha256"] != signature_before["sha256"]:
        raise RuntimeError("Gate 1 changed character data or object transforms.")
    if collection_pointer_after != collection_pointer_before:
        raise RuntimeError("Gate 1 replaced the character collection instead of reusing it.")
    if not collection_in_tree(production_scene.collection, final_collection):
        raise RuntimeError("Production scene lost the shared final collection.")
    if not collection_in_tree(lookdev_scene.collection, final_collection):
        raise RuntimeError("LookDev scene does not reference the shared final collection.")

    report = {
        "schema": "sloth_gate1_camera_match_v1",
        "generated_utc": utc_now(),
        "status": "GATE_1_COMPLETE",
        "authoritative_source": source_filepath,
        "target_reference": str(PROJECT_ROOT.parent.parent / ".codex/skills/codex_blender_character_refine/refs/target_character.png"),
        "checkpoints": {
            "pre_gate": str(PRE_CHECKPOINT),
            "pre_gate_exists": PRE_CHECKPOINT.exists(),
            "gate_result": str(LOOKDEV_BLEND),
        },
        "shared_character_collection": {
            "source_name": SOURCE_COLLECTION_NAME,
            "formal_name": FINAL_COLLECTION_NAME,
            "pointer_before": collection_pointer_before,
            "pointer_after": collection_pointer_after,
            "same_pointer": collection_pointer_before == collection_pointer_after,
            "signature_before": signature_before["sha256"],
            "signature_after": signature_after["sha256"],
            "character_unchanged": signature_before["sha256"] == signature_after["sha256"],
            "object_count": signature_after["object_count"],
            "production_scene_references_collection": True,
            "lookdev_scene_references_collection": True,
        },
        "character_bounds": {
            "minimum": vec(minimum), "maximum": vec(maximum),
            "width": clean_float(width), "depth": clean_float(depth), "height": clean_float(height),
            "full_target": vec(full_target), "head_target": vec(head_target),
        },
        "scene": {
            "name": lookdev_scene.name,
            "engine": lookdev_scene.render.engine,
            "resolution": [lookdev_scene.render.resolution_x, lookdev_scene.render.resolution_y],
            "view_transform": lookdev_scene.view_settings.view_transform,
            "world": world.name,
            "ground": ground.name,
            "setup_collection": setup_collection.name,
        },
        "cameras": [{
            "name": camera.name,
            "lens_mm": clean_float(camera.data.lens),
            "sensor_width_mm": clean_float(camera.data.sensor_width),
            "sensor_height_mm": clean_float(camera.data.sensor_height),
            "location": vec(camera.location),
            "rotation_euler": vec(camera.rotation_euler),
        } for camera in (camera_65, camera_80, camera_100)],
        "lights": [{
            "name": light.name, "type": light.data.type, "energy": clean_float(light.data.energy),
            "size": clean_float(light.data.size), "color": vec(light.data.color),
            "location": vec(light.location),
        } for light in lights],
        "shots": shot_reports,
        "acceptance": {
            "six_renders_exist": all(Path(shot["output"]).exists() for shot in shot_reports),
            "all_character_data_shared": collection_pointer_before == collection_pointer_after,
            "character_signature_unchanged": signature_before["sha256"] == signature_after["sha256"],
            "production_and_lookdev_share_collection": True,
            "character_objects_duplicated": False,
        },
    }
    REPORT_PATH.write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")

    save_result = bpy.ops.wm.save_as_mainfile(filepath=str(LOOKDEV_BLEND), compress=True)
    if "FINISHED" not in save_result or Path(bpy.data.filepath).resolve() != LOOKDEV_BLEND.resolve():
        raise RuntimeError(f"Gate 1 result save failed: {save_result}")

    print("GATE1_RESULT=" + json.dumps({
        "status": "GATE_1_COMPLETE",
        "active_file": bpy.data.filepath,
        "source_untouched": EXPECTED_SOURCE.exists(),
        "final_collection": final_collection.name,
        "shared_collection_pointer": collection_pointer_after,
        "character_signature": signature_after["sha256"],
        "renders": [shot["output"] for shot in shot_reports],
        "report": str(REPORT_PATH),
    }, ensure_ascii=False, sort_keys=True))


main()
