#!/usr/bin/env python3
"""Build the reusable dark editorial A-roll studio used by the IP avatar MCP."""

from __future__ import annotations

import math
import sys
from pathlib import Path

import bpy
from mathutils import Vector


def clear_all() -> None:
    bpy.ops.object.select_all(action="SELECT")
    bpy.ops.object.delete(use_global=False)
    for datablocks in (bpy.data.materials, bpy.data.cameras, bpy.data.lights, bpy.data.curves):
        for block in list(datablocks):
            if block.users == 0:
                datablocks.remove(block)


def collection(name: str) -> bpy.types.Collection:
    result = bpy.data.collections.get(name)
    if result:
        return result
    result = bpy.data.collections.new(name)
    bpy.context.scene.collection.children.link(result)
    return result


def move_to_collection(obj: bpy.types.Object, target: bpy.types.Collection) -> None:
    for current in list(obj.users_collection):
        current.objects.unlink(obj)
    target.objects.link(obj)


def principled_material(
    name: str,
    base_color: tuple[float, float, float, float],
    *,
    roughness: float,
    metallic: float = 0.0,
    noise_scale: float = 0.0,
    noise_strength: float = 0.0,
) -> bpy.types.Material:
    mat = bpy.data.materials.new(name)
    mat.use_nodes = True
    nodes = mat.node_tree.nodes
    links = mat.node_tree.links
    nodes.clear()
    output = nodes.new("ShaderNodeOutputMaterial")
    bsdf = nodes.new("ShaderNodeBsdfPrincipled")
    bsdf.inputs["Base Color"].default_value = base_color
    bsdf.inputs["Roughness"].default_value = roughness
    bsdf.inputs["Metallic"].default_value = metallic
    links.new(bsdf.outputs["BSDF"], output.inputs["Surface"])
    if noise_scale > 0 and noise_strength > 0:
        noise = nodes.new("ShaderNodeTexNoise")
        noise.inputs["Scale"].default_value = noise_scale
        noise.inputs["Detail"].default_value = 3.5
        noise.inputs["Roughness"].default_value = 0.62
        bump = nodes.new("ShaderNodeBump")
        bump.inputs["Strength"].default_value = noise_strength
        bump.inputs["Distance"].default_value = 0.04
        links.new(noise.outputs["Fac"], bump.inputs["Height"])
        links.new(bump.outputs["Normal"], bsdf.inputs["Normal"])
    return mat


def emissive_material(
    name: str,
    color: tuple[float, float, float, float],
    strength: float,
    *,
    roughness: float = 0.32,
) -> bpy.types.Material:
    mat = principled_material(name, color, roughness=roughness, metallic=0.05)
    bsdf = next(node for node in mat.node_tree.nodes if node.type == "BSDF_PRINCIPLED")
    if "Emission Color" in bsdf.inputs:
        bsdf.inputs["Emission Color"].default_value = color
        bsdf.inputs["Emission Strength"].default_value = strength
    return mat


def add_box(
    name: str,
    location: tuple[float, float, float],
    dimensions: tuple[float, float, float],
    mat: bpy.types.Material,
    target: bpy.types.Collection,
    *,
    bevel: float = 0.03,
) -> bpy.types.Object:
    bpy.ops.mesh.primitive_cube_add(location=location)
    obj = bpy.context.object
    obj.name = name
    obj.dimensions = dimensions
    bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)
    if bevel > 0:
        modifier = obj.modifiers.new("Edge_Soften", "BEVEL")
        modifier.width = bevel
        modifier.segments = 3
    obj.data.materials.append(mat)
    move_to_collection(obj, target)
    return obj


def add_cylinder(
    name: str,
    location: tuple[float, float, float],
    radius: float,
    depth: float,
    mat: bpy.types.Material,
    target: bpy.types.Collection,
    *,
    vertices: int = 48,
) -> bpy.types.Object:
    bpy.ops.mesh.primitive_cylinder_add(vertices=vertices, radius=radius, depth=depth, location=location)
    obj = bpy.context.object
    obj.name = name
    obj.data.materials.append(mat)
    move_to_collection(obj, target)
    modifier = obj.modifiers.new("Edge_Soften", "BEVEL")
    modifier.width = 0.025
    modifier.segments = 3
    return obj


def look_at(obj: bpy.types.Object, target: tuple[float, float, float]) -> None:
    direction = Vector(target) - obj.location
    obj.rotation_euler = direction.to_track_quat("-Z", "Y").to_euler()


def add_light(
    name: str,
    light_type: str,
    location: tuple[float, float, float],
    energy: float,
    color: tuple[float, float, float],
    role: str,
    target_collection: bpy.types.Collection,
    *,
    aim: tuple[float, float, float] | None = None,
    size: float = 1.0,
    size_y: float | None = None,
) -> bpy.types.Object:
    data = bpy.data.lights.new(name, light_type)
    data.energy = energy
    data.color = color
    if light_type == "AREA":
        data.shape = "RECTANGLE"
        data.size = size
        data.size_y = size_y or size
    elif light_type == "POINT":
        data.shadow_soft_size = size
    obj = bpy.data.objects.new(name, data)
    target_collection.objects.link(obj)
    obj.location = location
    obj["ip_base_energy"] = float(energy)
    obj["ip_light_role"] = role
    if aim is not None:
        look_at(obj, aim)
    return obj


def add_camera(
    name: str,
    location: tuple[float, float, float],
    target: tuple[float, float, float],
    lens: float,
    f_stop: float,
    focus: bpy.types.Object,
    target_collection: bpy.types.Collection,
) -> bpy.types.Object:
    data = bpy.data.cameras.new(name)
    data.lens = lens
    data.sensor_width = 36.0
    data.clip_start = 0.05
    data.clip_end = 100.0
    data.dof.use_dof = True
    data.dof.focus_object = focus
    data.dof.aperture_fstop = f_stop
    data.dof.aperture_blades = 9
    obj = bpy.data.objects.new(name, data)
    target_collection.objects.link(obj)
    obj.location = location
    look_at(obj, target)
    return obj


def build_scene() -> dict[str, bpy.types.Object]:
    clear_all()
    scene = bpy.context.scene
    scene.name = "Scene_Editorial_Aroll"
    scene["ip_scene_contract"] = "tangying-editorial-studio/v1"
    scene["ip_authored_exposure"] = -0.58

    studio = collection("STUDIO_SET")
    props = collection("STUDIO_PROPS")
    lights = collection("STUDIO_LIGHTS")
    cameras = collection("STUDIO_CAMERAS")
    markers = collection("STUDIO_MARKERS")

    floor_mat = principled_material(
        "PBR_Floor_Graphite",
        (0.036, 0.043, 0.052, 1.0),
        roughness=0.34,
        metallic=0.08,
        noise_scale=7.5,
        noise_strength=0.16,
    )
    wall_mat = principled_material(
        "PBR_Wall_Charcoal",
        (0.025, 0.033, 0.045, 1.0),
        roughness=0.66,
        noise_scale=4.2,
        noise_strength=0.08,
    )
    panel_mat = principled_material(
        "PBR_Panel_BlueBlack",
        (0.012, 0.022, 0.034, 1.0),
        roughness=0.28,
        metallic=0.22,
    )
    trim_mat = principled_material(
        "PBR_Trim_Gunmetal",
        (0.045, 0.052, 0.060, 1.0),
        roughness=0.24,
        metallic=0.68,
    )
    wood_mat = principled_material(
        "PBR_Walnut_Muted",
        (0.105, 0.052, 0.028, 1.0),
        roughness=0.46,
        noise_scale=5.0,
        noise_strength=0.11,
    )
    book_blue = principled_material("Book_Deep_Teal", (0.028, 0.118, 0.142, 1), roughness=0.55)
    book_red = principled_material("Book_Muted_Red", (0.185, 0.045, 0.042, 1), roughness=0.58)
    book_gray = principled_material("Book_Warm_Gray", (0.18, 0.17, 0.15, 1), roughness=0.62)
    screen_mat = emissive_material("Editorial_Display_Glass", (0.008, 0.028, 0.045, 1), 0.16, roughness=0.18)
    cyan_mat = emissive_material("Editorial_Cyan_Accent", (0.025, 0.34, 0.52, 1), 0.78)
    amber_mat = emissive_material("Editorial_Amber_Accent", (0.78, 0.245, 0.035, 1), 0.72)

    add_box("Studio_Floor", (0, 1.0, -0.10), (15.5, 13.0, 0.20), floor_mat, studio, bevel=0.02)
    add_box("Back_Wall", (0, 4.2, 2.55), (13.2, 0.30, 5.3), wall_mat, studio, bevel=0.05)
    add_box("Left_Return_Wall", (-6.45, 0.7, 2.45), (0.26, 7.2, 5.1), wall_mat, studio, bevel=0.04)
    add_box("Right_Return_Wall", (6.45, 0.7, 2.45), (0.26, 7.2, 5.1), wall_mat, studio, bevel=0.04)
    add_box("Ceiling_Header", (0, 3.8, 5.08), (13.0, 1.0, 0.24), trim_mat, studio, bevel=0.06)

    add_box("Display_Recess_Frame", (0, 3.92, 2.72), (7.0, 0.26, 3.22), trim_mat, studio, bevel=0.10)
    add_box("Editorial_Display", (0, 3.71, 2.72), (6.58, 0.08, 2.82), screen_mat, studio, bevel=0.08)
    for x in (-2.15, -1.08, 0.0, 1.08, 2.15):
        add_box(f"Display_Vertical_{x:+.2f}", (x, 3.655, 2.72), (0.018, 0.025, 2.45), cyan_mat, props, bevel=0.004)
    for z in (1.95, 2.45, 2.95, 3.45):
        add_box(f"Display_Horizontal_{z:.2f}", (0, 3.65, z), (5.75, 0.025, 0.014), cyan_mat, props, bevel=0.003)
    for index, (x, z, width, mat) in enumerate(
        [
            (-2.15, 3.35, 0.72, amber_mat),
            (-2.15, 3.08, 1.05, cyan_mat),
            (2.18, 2.03, 0.92, amber_mat),
            (2.18, 2.30, 1.28, cyan_mat),
        ],
        start=1,
    ):
        add_box(f"Editorial_Data_Bar_{index:02d}", (x, 3.61, z), (width, 0.035, 0.065), mat, props, bevel=0.015)

    for side, sign in (("L", -1), ("R", 1)):
        start_x = sign * 4.45
        for index in range(7):
            x = start_x + sign * index * 0.22
            add_box(f"Acoustic_Slat_{side}_{index:02d}", (x, 3.79, 2.56), (0.105, 0.18, 4.25), wood_mat, studio, bevel=0.025)
        accent_x = sign * 3.72
        accent_mat = cyan_mat if sign < 0 else amber_mat
        add_box(f"Vertical_Practical_{side}", (accent_x, 3.54, 2.75), (0.055, 0.06, 3.35), accent_mat, props, bevel=0.025)

    add_box("Low_Console", (0, 3.28, 0.43), (5.4, 0.62, 0.72), trim_mat, props, bevel=0.08)
    add_box("Low_Console_Inset", (0, 2.95, 0.45), (4.65, 0.045, 0.38), panel_mat, props, bevel=0.04)
    for side, x in (("L", -4.88), ("R", 4.88)):
        for shelf_index, z in enumerate((1.15, 2.05, 2.95), start=1):
            add_box(f"Shelf_{side}_{shelf_index}", (x, 3.34, z), (1.55, 0.55, 0.10), wood_mat, props, bevel=0.025)

    for index, (x, z, w, h, mat) in enumerate(
        [
            (-5.18, 1.42, 0.18, 0.44, book_red),
            (-4.94, 1.38, 0.22, 0.36, book_blue),
            (-4.67, 1.45, 0.18, 0.50, book_gray),
            (4.62, 2.31, 0.22, 0.42, book_blue),
            (4.89, 2.27, 0.18, 0.34, book_gray),
            (5.12, 2.34, 0.18, 0.48, book_red),
        ],
        start=1,
    ):
        add_box(f"Book_{index:02d}", (x, 3.10, z), (w, 0.28, h), mat, props, bevel=0.012)
    add_cylinder("Desk_Practical_Left", (-4.88, 3.15, 3.24), 0.13, 0.34, amber_mat, props)
    add_cylinder("Desk_Practical_Right", (4.88, 3.15, 1.36), 0.13, 0.34, cyan_mat, props)

    spawn = bpy.data.objects.new("IP_Character_Spawn", None)
    markers.objects.link(spawn)
    spawn.location = (0, 0, 0)
    spawn.empty_display_type = "CIRCLE"
    spawn.empty_display_size = 0.5
    spawn["target_height"] = 2.55
    spawn["contract"] = "character_spawn"

    focus = bpy.data.objects.new("IP_Focus_Head", None)
    markers.objects.link(focus)
    focus.location = (0, 0, 1.84)
    focus.empty_display_type = "SPHERE"
    focus.empty_display_size = 0.12
    focus["contract"] = "camera_focus"

    camera_wide = add_camera("Camera_Wide", (0, -7.9, 1.65), (0, 0, 1.34), 52, 5.6, focus, cameras)
    camera_medium = add_camera("Camera_Medium", (0, -5.2, 1.78), (0, 0, 1.78), 58, 5.2, focus, cameras)
    camera_close = add_camera("Camera_Close", (0.12, -4.55, 1.92), (0, 0, 1.88), 68, 4.8, focus, cameras)
    scene.camera = camera_medium

    face_target = (0, 0, 1.45)
    add_light("Studio_Key", "AREA", (-3.35, -3.35, 4.35), 620, (1.0, 0.91, 0.80), "key", lights, aim=face_target, size=3.2, size_y=2.2)
    add_light("Studio_Fill", "AREA", (3.6, -2.45, 3.25), 205, (0.70, 0.84, 1.0), "fill", lights, aim=face_target, size=3.8, size_y=2.6)
    add_light("Studio_Rim_Cool", "AREA", (-2.65, 1.55, 3.85), 320, (0.35, 0.68, 1.0), "rim", lights, aim=(0, 0.1, 1.6), size=2.2, size_y=1.1)
    add_light("Studio_Rim_Warm", "AREA", (2.75, 1.65, 3.45), 270, (1.0, 0.48, 0.21), "rim", lights, aim=(0, 0.1, 1.45), size=2.0, size_y=1.0)
    add_light("Studio_Top_Soft", "AREA", (0, -0.15, 5.0), 105, (0.82, 0.90, 1.0), "top", lights, aim=(0, 0, 1.1), size=3.0, size_y=2.0)
    add_light("Practical_Left", "POINT", (-3.72, 3.22, 2.75), 26, (0.18, 0.56, 1.0), "practical", lights, size=0.35)
    add_light("Practical_Right", "POINT", (3.72, 3.22, 2.75), 22, (1.0, 0.30, 0.08), "practical", lights, size=0.35)

    world = bpy.data.worlds.get("Editorial_Studio_World") or bpy.data.worlds.new("Editorial_Studio_World")
    world.use_nodes = True
    background = next(node for node in world.node_tree.nodes if node.type == "BACKGROUND")
    background.inputs["Color"].default_value = (0.012, 0.017, 0.026, 1.0)
    background.inputs["Strength"].default_value = 0.045
    scene.world = world

    try:
        scene.render.engine = "BLENDER_EEVEE_NEXT"
    except TypeError:
        scene.render.engine = "BLENDER_EEVEE"
    scene.render.resolution_x = 1920
    scene.render.resolution_y = 1080
    scene.render.resolution_percentage = 100
    scene.render.fps = 30
    scene.render.film_transparent = False
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"
    scene.render.image_settings.color_depth = "8"
    scene.render.image_settings.color_management = "FOLLOW_SCENE"
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
    scene.view_settings.exposure = float(scene["ip_authored_exposure"])
    scene.view_settings.gamma = 1.0

    return {
        "wide": camera_wide,
        "medium": camera_medium,
        "close": camera_close,
        "spawn": spawn,
        "focus": focus,
    }


def main() -> None:
    args = sys.argv[sys.argv.index("--") + 1 :] if "--" in sys.argv else []
    root = Path(__file__).resolve().parents[2]
    blend_path = Path(args[0]).expanduser().resolve() if args else root / "tmp/ip-avatar-3d/editorial-news-studio.blend"
    preview_path = Path(args[1]).expanduser().resolve() if len(args) > 1 else blend_path.with_name("editorial-news-studio-preview.png")
    blend_path.parent.mkdir(parents=True, exist_ok=True)
    preview_path.parent.mkdir(parents=True, exist_ok=True)

    objects = build_scene()
    scene = bpy.context.scene
    scene.camera = objects["wide"]
    scene.render.resolution_x = 960
    scene.render.resolution_y = 540
    scene.render.resolution_percentage = 100
    scene.render.filepath = str(preview_path)
    bpy.ops.render.render(write_still=True)

    scene.camera = objects["medium"]
    scene.render.resolution_x = 1920
    scene.render.resolution_y = 1080
    scene.render.filepath = "//renders/editorial_"
    bpy.ops.wm.save_as_mainfile(filepath=str(blend_path))
    print(f"EDITORIAL_STUDIO_BLEND={blend_path}")
    print(f"EDITORIAL_STUDIO_PREVIEW={preview_path}")


if __name__ == "__main__":
    main()
