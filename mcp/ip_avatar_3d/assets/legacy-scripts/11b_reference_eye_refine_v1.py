import bpy
import json
import math
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
PRE_CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_090_pre_reference_refine.blend")
CANDIDATE = os.path.join(ROOT, "checkpoints", "sloth_091_reference_eye_candidate_v1.blend")
REPORT = os.path.join(ROOT, "reports", "reference_eye_candidate_v1.json")
RENDER_DIR = os.path.join(ROOT, "renders", "reference_refine", "eye_candidate_v1")

EYE_CZ = 2.198
IRIS_CZ = 2.194
EYE_RX = 0.0615
CENTERS = {"L": 0.125, "R": -0.125}
COLS = 48
ROWS = 6


def point_at(obj, target):
    obj.rotation_euler = (Vector(target) - obj.location).to_track_quat("-Z", "Y").to_euler()


def set_world_dimensions(obj, dimensions):
    bpy.context.view_layer.update()
    current = obj.dimensions.copy()
    for axis in range(3):
        if abs(current[axis]) > 1.0e-9:
            obj.scale[axis] *= dimensions[axis] / current[axis]
    bpy.context.view_layer.update()


def set_principled_input(node, names, value):
    for name in names:
        if name in node.inputs:
            node.inputs[name].default_value = value
            return True
    return False


def rebuild_iris_material(material):
    material.use_nodes = True
    nodes = material.node_tree.nodes
    links = material.node_tree.links
    nodes.clear()

    output = nodes.new("ShaderNodeOutputMaterial")
    output.location = (760, 20)
    principled = nodes.new("ShaderNodeBsdfPrincipled")
    principled.location = (500, 20)
    set_principled_input(principled, ("Roughness",), 0.29)
    set_principled_input(principled, ("IOR",), 1.45)
    set_principled_input(principled, ("Specular IOR Level", "Specular"), 0.36)
    set_principled_input(principled, ("Coat Weight", "Coat"), 0.12)
    set_principled_input(principled, ("Coat Roughness",), 0.18)

    texcoord = nodes.new("ShaderNodeTexCoord")
    texcoord.location = (-760, 40)
    separate = nodes.new("ShaderNodeSeparateXYZ")
    separate.location = (-580, 40)
    links.new(texcoord.outputs["Generated"], separate.inputs["Vector"])

    dx = nodes.new("ShaderNodeMath")
    dx.operation = "SUBTRACT"
    dx.inputs[1].default_value = 0.5
    dx.location = (-400, 140)
    dz = nodes.new("ShaderNodeMath")
    dz.operation = "SUBTRACT"
    dz.inputs[1].default_value = 0.5
    dz.location = (-400, -40)
    links.new(separate.outputs["X"], dx.inputs[0])
    links.new(separate.outputs["Z"], dz.inputs[0])

    dx2 = nodes.new("ShaderNodeMath")
    dx2.operation = "MULTIPLY"
    dx2.location = (-230, 140)
    dz2 = nodes.new("ShaderNodeMath")
    dz2.operation = "MULTIPLY"
    dz2.location = (-230, -40)
    links.new(dx.outputs[0], dx2.inputs[0])
    links.new(dx.outputs[0], dx2.inputs[1])
    links.new(dz.outputs[0], dz2.inputs[0])
    links.new(dz.outputs[0], dz2.inputs[1])

    add = nodes.new("ShaderNodeMath")
    add.operation = "ADD"
    add.location = (-60, 80)
    links.new(dx2.outputs[0], add.inputs[0])
    links.new(dz2.outputs[0], add.inputs[1])
    sqrt = nodes.new("ShaderNodeMath")
    sqrt.operation = "SQRT"
    sqrt.location = (100, 110)
    links.new(add.outputs[0], sqrt.inputs[0])

    ramp = nodes.new("ShaderNodeValToRGB")
    ramp.location = (270, 150)
    color_ramp = ramp.color_ramp
    while len(color_ramp.elements) > 2:
        color_ramp.elements.remove(color_ramp.elements[-1])
    color_ramp.elements[0].position = 0.0
    color_ramp.elements[0].color = (0.48, 0.105, 0.012, 1.0)
    color_ramp.elements[1].position = 0.62
    color_ramp.elements[1].color = (0.055, 0.008, 0.0015, 1.0)
    middle = color_ramp.elements.new(0.31)
    middle.color = (0.82, 0.285, 0.025, 1.0)
    outer = color_ramp.elements.new(0.47)
    outer.color = (0.23, 0.038, 0.003, 1.0)
    links.new(sqrt.outputs[0], ramp.inputs["Fac"])

    noise = nodes.new("ShaderNodeTexNoise")
    noise.location = (-50, -230)
    noise.inputs["Scale"].default_value = 18.0
    noise.inputs["Detail"].default_value = 3.0
    noise.inputs["Roughness"].default_value = 0.68
    noise.inputs["Distortion"].default_value = 0.18
    links.new(texcoord.outputs["Generated"], noise.inputs["Vector"])

    mix = nodes.new("ShaderNodeMixRGB")
    mix.blend_type = "MULTIPLY"
    mix.inputs[0].default_value = 0.28
    mix.location = (300, -20)
    links.new(ramp.outputs["Color"], mix.inputs[1])
    links.new(noise.outputs["Fac"], mix.inputs[2])
    links.new(mix.outputs["Color"], principled.inputs["Base Color"])

    bump = nodes.new("ShaderNodeBump")
    bump.location = (300, -210)
    bump.inputs["Strength"].default_value = 0.09
    bump.inputs["Distance"].default_value = 0.035
    links.new(noise.outputs["Fac"], bump.inputs["Height"])
    links.new(bump.outputs["Normal"], principled.inputs["Normal"])
    links.new(principled.outputs["BSDF"], output.inputs["Surface"])


def rebuild_simple_principled(material, base_color, roughness, coat=0.0):
    material.use_nodes = True
    nodes = material.node_tree.nodes
    links = material.node_tree.links
    nodes.clear()
    output = nodes.new("ShaderNodeOutputMaterial")
    shader = nodes.new("ShaderNodeBsdfPrincipled")
    shader.inputs["Base Color"].default_value = base_color
    set_principled_input(shader, ("Roughness",), roughness)
    set_principled_input(shader, ("IOR",), 1.45)
    set_principled_input(shader, ("Specular IOR Level", "Specular"), 0.34)
    set_principled_input(shader, ("Coat Weight", "Coat"), coat)
    set_principled_input(shader, ("Coat Roughness",), 0.2)
    links.new(shader.outputs["BSDF"], output.inputs["Surface"])


def rebuild_lid_material(material):
    material.use_nodes = True
    nodes = material.node_tree.nodes
    links = material.node_tree.links
    nodes.clear()
    output = nodes.new("ShaderNodeOutputMaterial")
    shader = nodes.new("ShaderNodeBsdfPrincipled")
    shader.inputs["Base Color"].default_value = (0.235, 0.075, 0.021, 1.0)
    set_principled_input(shader, ("Roughness",), 0.58)
    set_principled_input(shader, ("IOR",), 1.42)
    set_principled_input(shader, ("Specular IOR Level", "Specular"), 0.27)
    noise = nodes.new("ShaderNodeTexNoise")
    noise.inputs["Scale"].default_value = 46.0
    noise.inputs["Detail"].default_value = 2.0
    noise.inputs["Roughness"].default_value = 0.7
    bump = nodes.new("ShaderNodeBump")
    bump.inputs["Strength"].default_value = 0.055
    bump.inputs["Distance"].default_value = 0.018
    links.new(noise.outputs["Fac"], bump.inputs["Height"])
    links.new(bump.outputs["Normal"], shader.inputs["Normal"])
    links.new(shader.outputs["BSDF"], output.inputs["Surface"])


def lid_coords(side, part, pose):
    cx = CENTERS[side]
    coords = []
    for col in range(COLS):
        u = -1.0 + 2.0 * col / (COLS - 1)
        s = math.sqrt(max(0.0, 1.0 - u * u))
        taper = s ** 1.45
        x = cx + EYE_RX * u

        if pose == "Blink":
            inner_z = EYE_CZ - 0.020 * s - 0.003 * s * s
        elif part == "Upper":
            inner_z = EYE_CZ + (0.027 if pose == "Basis" else 0.042) * s
        else:
            inner_z = EYE_CZ - (0.036 if pose == "Basis" else 0.046) * s

        if part == "Upper":
            outer_z = inner_z + (0.012 if pose != "Blink" else 0.016) * s * taper
        else:
            outer_z = inner_z - (0.009 if pose != "Blink" else 0.012) * s * taper

        inner_y = -0.2510 + 0.0010 * u * u
        raw_outer_y = -0.2472 - 0.0004 * s
        outer_y = inner_y + (raw_outer_y - inner_y) * taper

        for row in range(ROWS):
            t = row / (ROWS - 1)
            smooth_t = t * t * (3.0 - 2.0 * t)
            z = outer_z * (1.0 - smooth_t) + inner_z * smooth_t
            y = outer_y * (1.0 - smooth_t) + inner_y * smooth_t
            y -= 0.0011 * math.sin(math.pi * t) * s
            coords.append((x, y, z))
    return coords


def set_eye_world_transform(obj, x, y, z, dimensions):
    matrix = obj.matrix_world.copy()
    matrix.translation = Vector((x, y, z))
    obj.matrix_world = matrix
    set_world_dimensions(obj, dimensions)


def ensure_secondary_catchlight(side, primary):
    name = f"EYE_CatchlightSecondary_{side}"
    obj = bpy.data.objects.get(name)
    if obj is None:
        obj = primary.copy()
        obj.data = primary.data.copy()
        obj.name = name
        obj.data.name = name + "_Mesh"
        bpy.data.collections["COL_CHR_SLOTH_FINAL"].objects.link(obj)
    obj.parent = primary.parent
    obj.parent_type = primary.parent_type
    obj.parent_bone = primary.parent_bone
    obj.matrix_parent_inverse = primary.matrix_parent_inverse.copy()
    return obj


def render_reviews(scene, head):
    old_camera = scene.camera
    old_engine = scene.render.engine
    old_resolution = (scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage)
    old_path = scene.render.filepath
    old_format = scene.render.image_settings.file_format
    old_film = scene.render.film_transparent
    old_values = {key.name: key.value for key in head.data.shape_keys.key_blocks}

    camera_data = bpy.data.cameras.new("TMP_ReferenceEyeCamera")
    camera = bpy.data.objects.new("TMP_ReferenceEyeCamera", camera_data)
    scene.collection.objects.link(camera)
    camera_data.sensor_width = 36.0
    scene.camera = camera
    scene.render.engine = "BLENDER_EEVEE"
    scene.render.resolution_x = 720
    scene.render.resolution_y = 720
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "PNG"
    scene.render.film_transparent = False

    for key in head.data.shape_keys.key_blocks:
        if key.name != "Basis":
            key.value = 0.0
    if "Mouth_Rest" in head.data.shape_keys.key_blocks:
        head.data.shape_keys.key_blocks["Mouth_Rest"].value = 1.0
    bpy.context.view_layer.update()

    def render(filename, location, lens, target=(0.0, -0.02, 2.20)):
        camera.location = location
        camera.data.lens = lens
        point_at(camera, target)
        scene.render.filepath = os.path.join(RENDER_DIR, filename)
        bpy.ops.render.render(write_still=True)

    render("neutral_front.png", (0.0, -3.35, 2.20), 100.0)
    render("neutral_front_34.png", (1.65, -3.15, 2.25), 95.0)
    render("strict_side.png", (3.55, -0.02, 2.22), 100.0)

    head.data.shape_keys.key_blocks["Blink.L"].value = 1.0
    head.data.shape_keys.key_blocks["Blink.R"].value = 1.0
    bpy.context.view_layer.update()
    render("blink_front.png", (0.0, -3.35, 2.20), 100.0)

    head.data.shape_keys.key_blocks["Blink.L"].value = 0.0
    head.data.shape_keys.key_blocks["Blink.R"].value = 0.0
    head.data.shape_keys.key_blocks["Eye_Wide.L"].value = 0.75
    head.data.shape_keys.key_blocks["Eye_Wide.R"].value = 0.75
    bpy.context.view_layer.update()
    render("wide_front.png", (0.0, -3.35, 2.20), 100.0)

    for key in head.data.shape_keys.key_blocks:
        key.value = old_values[key.name]
    bpy.context.view_layer.update()
    scene.camera = old_camera
    scene.render.engine = old_engine
    scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage = old_resolution
    scene.render.filepath = old_path
    scene.render.image_settings.file_format = old_format
    scene.render.film_transparent = old_film
    bpy.data.objects.remove(camera, do_unlink=True)
    bpy.data.cameras.remove(camera_data)


def main():
    os.makedirs(os.path.dirname(CANDIDATE), exist_ok=True)
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    os.makedirs(RENDER_DIR, exist_ok=True)

    head = bpy.data.objects.get("GEO_HeadBody")
    rig = bpy.data.objects.get("RIG_Sloth")
    formal = bpy.data.collections.get("COL_CHR_SLOTH_FINAL")
    if not head or not rig or not formal or not head.data.shape_keys:
        raise RuntimeError("Formal eye/character system is incomplete")
    if len(head.data.vertices) != 5478:
        raise RuntimeError("Unexpected head vertex count")

    shape_names_before = [key.name for key in head.data.shape_keys.key_blocks]
    action_names_before = sorted(action.name for action in bpy.data.actions)
    if not os.path.exists(PRE_CHECKPOINT):
        bpy.ops.wm.save_as_mainfile(filepath=PRE_CHECKPOINT, copy=True)

    for side in ("L", "R"):
        for part in ("Upper", "Lower"):
            obj = bpy.data.objects.get(f"EYE_Lid{part}_{side}")
            if not obj or len(obj.data.vertices) != COLS * ROWS or not obj.data.shape_keys:
                raise RuntimeError(f"Invalid eyelid: {side} {part}")
            inverse = obj.matrix_world.inverted()
            for pose in ("Basis", "Blink", "Wide"):
                key = obj.data.shape_keys.key_blocks.get(pose)
                if not key:
                    raise RuntimeError(f"Missing lid shape key {obj.name}:{pose}")
                for index, world_coordinate in enumerate(lid_coords(side, part, pose)):
                    local_coordinate = inverse @ Vector(world_coordinate)
                    key.data[index].co = local_coordinate
                    if pose == "Basis":
                        obj.data.vertices[index].co = local_coordinate
            obj.data.update()

    iris_material = bpy.data.materials.get("MAT_Eye_Iris_Amber")
    limbal_material = bpy.data.materials.get("MAT_Eye_Iris_Limbal")
    sclera_material = bpy.data.materials.get("MAT_Eye_Sclera")
    pupil_material = bpy.data.materials.get("MAT_Eye_Pupil")
    lid_material = bpy.data.materials.get("MAT_Eyelid_Skin_Brown")
    catchlight_material = bpy.data.materials.get("MAT_Eye_Catchlight")
    required_materials = (iris_material, limbal_material, sclera_material, pupil_material, lid_material, catchlight_material)
    if not all(required_materials):
        raise RuntimeError("Required formal eye material missing")

    rebuild_iris_material(iris_material)
    rebuild_simple_principled(limbal_material, (0.018, 0.0025, 0.0006, 1.0), 0.27, 0.06)
    rebuild_simple_principled(sclera_material, (0.66, 0.58, 0.47, 1.0), 0.4, 0.04)
    rebuild_simple_principled(pupil_material, (0.00035, 0.00008, 0.00002, 1.0), 0.2, 0.18)
    rebuild_lid_material(lid_material)

    for side, center_x in CENTERS.items():
        iris = bpy.data.objects[f"EYE_Iris_{side}"]
        limbal = bpy.data.objects[f"EYE_IrisLimbal_{side}"]
        pupil = bpy.data.objects[f"EYE_Pupil_{side}"]
        primary = bpy.data.objects[f"EYE_Catchlight_{side}"]
        set_eye_world_transform(iris, center_x, -0.2217, IRIS_CZ, (0.068, 0.0018, 0.068))
        set_eye_world_transform(limbal, center_x, -0.2220, IRIS_CZ, (0.0735, 0.0001, 0.0735))
        set_eye_world_transform(pupil, center_x, -0.2223, IRIS_CZ, (0.040, 0.0020, 0.040))
        set_eye_world_transform(primary, center_x + 0.010, -0.2230, IRIS_CZ + 0.014, (0.011, 0.0010, 0.011))
        secondary = ensure_secondary_catchlight(side, primary)
        set_eye_world_transform(secondary, center_x - 0.010, -0.2230, IRIS_CZ - 0.012, (0.0042, 0.0008, 0.0042))
        secondary["production_role"] = "eye_catchlight_secondary"

    render_reviews(bpy.context.scene, head)

    lid_driver_count = sum(
        len(obj.data.shape_keys.animation_data.drivers)
        for obj in bpy.data.objects
        if obj.name.startswith(("EYE_LidUpper_", "EYE_LidLower_"))
        and obj.data.shape_keys and obj.data.shape_keys.animation_data
    )
    seam_max = {}
    for side in ("L", "R"):
        upper = bpy.data.objects[f"EYE_LidUpper_{side}"]
        lower = bpy.data.objects[f"EYE_LidLower_{side}"]
        distances = []
        for col in range(COLS):
            index = col * ROWS + (ROWS - 1)
            upper_world = upper.matrix_world @ upper.data.shape_keys.key_blocks["Blink"].data[index].co
            lower_world = lower.matrix_world @ lower.data.shape_keys.key_blocks["Blink"].data[index].co
            distances.append((upper_world - lower_world).length)
        seam_max[side] = max(distances)

    checks = {
        "head_vertex_count": len(head.data.vertices) == 5478,
        "head_shape_key_order": [key.name for key in head.data.shape_keys.key_blocks] == shape_names_before,
        "actions_unchanged": sorted(action.name for action in bpy.data.actions) == action_names_before,
        "lid_vertex_counts": all(len(bpy.data.objects[f"EYE_Lid{part}_{side}"].data.vertices) == 288 for side in ("L", "R") for part in ("Upper", "Lower")),
        "lid_shape_keys": all([key.name for key in bpy.data.objects[f"EYE_Lid{part}_{side}"].data.shape_keys.key_blocks] == ["Basis", "Blink", "Wide"] for side in ("L", "R") for part in ("Upper", "Lower")),
        "lid_drivers": lid_driver_count == 8,
        "blink_seam": all(distance < 1.0e-5 for distance in seam_max.values()),
        "iris_within_sclera": all(bpy.data.objects[f"EYE_IrisLimbal_{side}"].dimensions.x < bpy.data.objects[f"EYE_Sclera_{side}"].dimensions.x for side in ("L", "R")),
        "secondary_catchlights": all(bpy.data.objects.get(f"EYE_CatchlightSecondary_{side}") is not None for side in ("L", "R")),
        "no_temp_objects": not any(obj.name.startswith("TMP_") for obj in bpy.data.objects),
    }
    failed = [name for name, value in checks.items() if not value]
    report = {
        "schema": "sloth_reference_eye_candidate_v1",
        "status": "PASS" if not failed else "FAIL",
        "pre_checkpoint": PRE_CHECKPOINT,
        "candidate": CANDIDATE,
        "metrics": {
            "sclera_dimensions": [0.123, 0.09, 0.148],
            "iris_diameter": 0.068,
            "limbal_diameter": 0.0735,
            "pupil_diameter": 0.040,
            "iris_center_z": IRIS_CZ,
            "neutral_upper_aperture_center_z": EYE_CZ + 0.027,
            "neutral_lower_aperture_center_z": EYE_CZ - 0.036,
            "blink_seam_max": seam_max,
        },
        "checks": checks,
        "failed": failed,
        "renders": sorted(os.path.join(RENDER_DIR, name) for name in os.listdir(RENDER_DIR) if name.endswith(".png")),
    }
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    if failed:
        raise RuntimeError("Eye candidate validation failed: " + ", ".join(failed))
    bpy.ops.wm.save_as_mainfile(filepath=CANDIDATE, copy=True)
    print(json.dumps({
        "status": report["status"],
        "candidate": CANDIDATE,
        "report": REPORT,
        "metrics": report["metrics"],
        "checks": checks,
        "renders": report["renders"],
    }, ensure_ascii=False))


if __name__ == "__main__":
    main()
