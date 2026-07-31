import bpy
import json
import math
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
V2_CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_092_reference_eye_candidate_v2.blend")
V3_CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_093_reference_eye_candidate_v3.blend")
REPORT = os.path.join(ROOT, "reports", "reference_eye_candidate_v3.json")
RENDER_DIR = os.path.join(ROOT, "renders", "reference_refine", "eye_candidate_v3")

EYE_CZ = 2.198
IRIS_CZ = 2.190
EYE_RX = 0.0615
CENTERS = {"L": 0.125, "R": -0.125}
COLS = 48
ROWS = 6


def point_at(obj, target):
    obj.rotation_euler = (Vector(target) - obj.location).to_track_quat("-Z", "Y").to_euler()


def set_dimensions(obj, dimensions):
    bpy.context.view_layer.update()
    current = obj.dimensions.copy()
    for axis in range(3):
        if abs(current[axis]) > 1.0e-9:
            obj.scale[axis] *= dimensions[axis] / current[axis]
    bpy.context.view_layer.update()


def set_transform(obj, x, y, z, dimensions):
    matrix = obj.matrix_world.copy()
    matrix.translation = Vector((x, y, z))
    obj.matrix_world = matrix
    set_dimensions(obj, dimensions)


def set_input(node, names, value):
    for name in names:
        if name in node.inputs:
            node.inputs[name].default_value = value
            return


def rebuild_iris(material):
    material.use_nodes = True
    nodes = material.node_tree.nodes
    links = material.node_tree.links
    nodes.clear()
    output = nodes.new("ShaderNodeOutputMaterial")
    shader = nodes.new("ShaderNodeBsdfPrincipled")
    set_input(shader, ("Roughness",), 0.3)
    set_input(shader, ("IOR",), 1.45)
    set_input(shader, ("Specular IOR Level", "Specular"), 0.32)
    set_input(shader, ("Coat Weight", "Coat"), 0.12)
    set_input(shader, ("Coat Roughness",), 0.18)

    texcoord = nodes.new("ShaderNodeTexCoord")
    separate = nodes.new("ShaderNodeSeparateXYZ")
    links.new(texcoord.outputs["Generated"], separate.inputs["Vector"])
    offsets = []
    squares = []
    for socket_name in ("X", "Z"):
        offset = nodes.new("ShaderNodeMath")
        offset.operation = "SUBTRACT"
        offset.inputs[1].default_value = 0.5
        square = nodes.new("ShaderNodeMath")
        square.operation = "MULTIPLY"
        links.new(separate.outputs[socket_name], offset.inputs[0])
        links.new(offset.outputs[0], square.inputs[0])
        links.new(offset.outputs[0], square.inputs[1])
        offsets.append(offset)
        squares.append(square)
    add = nodes.new("ShaderNodeMath")
    add.operation = "ADD"
    links.new(squares[0].outputs[0], add.inputs[0])
    links.new(squares[1].outputs[0], add.inputs[1])
    radius = nodes.new("ShaderNodeMath")
    radius.operation = "SQRT"
    links.new(add.outputs[0], radius.inputs[0])

    base_ramp = nodes.new("ShaderNodeValToRGB")
    base_ramp.color_ramp.elements[0].position = 0.0
    base_ramp.color_ramp.elements[0].color = (0.055, 0.007, 0.0008, 1.0)
    base_ramp.color_ramp.elements[1].position = 0.64
    base_ramp.color_ramp.elements[1].color = (0.006, 0.0005, 0.0001, 1.0)
    gold = base_ramp.color_ramp.elements.new(0.27)
    gold.color = (0.22, 0.050, 0.0035, 1.0)
    amber = base_ramp.color_ramp.elements.new(0.44)
    amber.color = (0.085, 0.012, 0.0008, 1.0)
    links.new(radius.outputs[0], base_ramp.inputs["Fac"])

    angle = nodes.new("ShaderNodeMath")
    angle.operation = "ARCTAN2"
    links.new(offsets[1].outputs[0], angle.inputs[0])
    links.new(offsets[0].outputs[0], angle.inputs[1])
    frequency = nodes.new("ShaderNodeMath")
    frequency.operation = "MULTIPLY"
    frequency.inputs[1].default_value = 23.0
    links.new(angle.outputs[0], frequency.inputs[0])
    sine = nodes.new("ShaderNodeMath")
    sine.operation = "SINE"
    links.new(frequency.outputs[0], sine.inputs[0])
    absolute = nodes.new("ShaderNodeMath")
    absolute.operation = "ABSOLUTE"
    links.new(sine.outputs[0], absolute.inputs[0])
    sharpen = nodes.new("ShaderNodeMath")
    sharpen.operation = "POWER"
    sharpen.inputs[1].default_value = 5.0
    links.new(absolute.outputs[0], sharpen.inputs[0])
    strength = nodes.new("ShaderNodeMath")
    strength.operation = "MULTIPLY"
    strength.inputs[1].default_value = 0.16
    links.new(sharpen.outputs[0], strength.inputs[0])

    spokes = nodes.new("ShaderNodeMixRGB")
    spokes.blend_type = "SCREEN"
    spokes.inputs[2].default_value = (0.24, 0.055, 0.004, 1.0)
    links.new(strength.outputs[0], spokes.inputs[0])
    links.new(base_ramp.outputs["Color"], spokes.inputs[1])
    links.new(spokes.outputs["Color"], shader.inputs["Base Color"])

    noise = nodes.new("ShaderNodeTexNoise")
    noise.inputs["Scale"].default_value = 28.0
    noise.inputs["Detail"].default_value = 2.5
    noise.inputs["Roughness"].default_value = 0.65
    links.new(texcoord.outputs["Generated"], noise.inputs["Vector"])
    bump = nodes.new("ShaderNodeBump")
    bump.inputs["Strength"].default_value = 0.035
    bump.inputs["Distance"].default_value = 0.015
    links.new(noise.outputs["Fac"], bump.inputs["Height"])
    links.new(bump.outputs["Normal"], shader.inputs["Normal"])
    links.new(shader.outputs["BSDF"], output.inputs["Surface"])


def tune_principled_material(material, color, roughness, coat):
    material.use_nodes = True
    shader = next((node for node in material.node_tree.nodes if node.type == "BSDF_PRINCIPLED"), None)
    if not shader:
        material.node_tree.nodes.clear()
        output = material.node_tree.nodes.new("ShaderNodeOutputMaterial")
        shader = material.node_tree.nodes.new("ShaderNodeBsdfPrincipled")
        material.node_tree.links.new(shader.outputs["BSDF"], output.inputs["Surface"])
    shader.inputs["Base Color"].default_value = color
    set_input(shader, ("Roughness",), roughness)
    set_input(shader, ("Coat Weight", "Coat"), coat)


def lid_coordinates(side, part, pose):
    result = []
    for col in range(COLS):
        u = -1.0 + 2.0 * col / (COLS - 1)
        s = math.sqrt(max(0.0, 1.0 - u * u))
        taper = s ** 1.5
        x = CENTERS[side] + EYE_RX * u
        if pose == "Blink":
            inner_z = EYE_CZ - 0.018 * s - 0.003 * s * s
            outer_z = EYE_CZ + 0.038 * s if part == "Upper" else EYE_CZ - 0.052 * s
        elif part == "Upper":
            inner_z = EYE_CZ + (0.025 if pose == "Basis" else 0.040) * s
            outer_z = inner_z + 0.007 * s * taper
        else:
            inner_z = EYE_CZ - (0.038 if pose == "Basis" else 0.048) * s
            outer_z = inner_z - 0.004 * s * taper
        inner_y = -0.2510 + 0.0010 * u * u
        outer_y = inner_y + ((-0.2474 - 0.0003 * s) - inner_y) * taper
        for row in range(ROWS):
            t = row / (ROWS - 1)
            smooth_t = t * t * (3.0 - 2.0 * t)
            result.append((
                x,
                outer_y * (1.0 - smooth_t) + inner_y * smooth_t - 0.0008 * math.sin(math.pi * t) * s,
                outer_z * (1.0 - smooth_t) + inner_z * smooth_t,
            ))
    return result


def render_reviews(scene, head):
    old_camera = scene.camera
    old_engine = scene.render.engine
    old_resolution = (scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage)
    old_path = scene.render.filepath
    old_format = scene.render.image_settings.file_format
    old_samples = scene.cycles.samples
    old_values = {key.name: key.value for key in head.data.shape_keys.key_blocks}
    camera_data = bpy.data.cameras.new("TMP_ReferenceEyeV3Camera")
    camera = bpy.data.objects.new("TMP_ReferenceEyeV3Camera", camera_data)
    scene.collection.objects.link(camera)
    camera_data.sensor_width = 36.0
    scene.camera = camera
    scene.render.resolution_x = 720
    scene.render.resolution_y = 720
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "PNG"
    for key in head.data.shape_keys.key_blocks:
        if key.name != "Basis":
            key.value = 0.0
    head.data.shape_keys.key_blocks["Mouth_Rest"].value = 1.0

    def render(name, location, lens=100.0):
        camera.location = location
        camera.data.lens = lens
        point_at(camera, (0.0, -0.02, 2.20))
        scene.render.filepath = os.path.join(RENDER_DIR, name)
        bpy.ops.render.render(write_still=True)

    scene.render.engine = "BLENDER_EEVEE"
    bpy.context.view_layer.update()
    render("neutral_front.png", (0.0, -3.35, 2.20))
    render("neutral_front_34.png", (1.65, -3.15, 2.25), 95.0)
    render("strict_side.png", (3.55, -0.02, 2.22))
    head.data.shape_keys.key_blocks["Blink.L"].value = 1.0
    head.data.shape_keys.key_blocks["Blink.R"].value = 1.0
    bpy.context.view_layer.update()
    render("blink_front.png", (0.0, -3.35, 2.20))
    head.data.shape_keys.key_blocks["Blink.L"].value = 0.0
    head.data.shape_keys.key_blocks["Blink.R"].value = 0.0
    scene.render.engine = "CYCLES"
    scene.cycles.samples = 24
    scene.cycles.use_denoising = True
    render("cycles_eye_closeup.png", (0.0, -4.50, 2.28), 105.0)

    for key in head.data.shape_keys.key_blocks:
        key.value = old_values[key.name]
    scene.camera = old_camera
    scene.render.engine = old_engine
    scene.cycles.samples = old_samples
    scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage = old_resolution
    scene.render.filepath = old_path
    scene.render.image_settings.file_format = old_format
    bpy.data.objects.remove(camera, do_unlink=True)
    bpy.data.cameras.remove(camera_data)


def main():
    os.makedirs(os.path.dirname(V3_CHECKPOINT), exist_ok=True)
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    os.makedirs(RENDER_DIR, exist_ok=True)
    head = bpy.data.objects.get("GEO_HeadBody")
    if not head or len(head.data.vertices) != 5478 or not head.data.shape_keys:
        raise RuntimeError("Formal head invariant missing")
    if not os.path.exists(V2_CHECKPOINT):
        raise RuntimeError("V2 recovery checkpoint missing")
    shape_names = [key.name for key in head.data.shape_keys.key_blocks]
    action_names = sorted(action.name for action in bpy.data.actions)

    for side in ("L", "R"):
        for part in ("Upper", "Lower"):
            obj = bpy.data.objects[f"EYE_Lid{part}_{side}"]
            inverse = obj.matrix_world.inverted()
            for pose in ("Basis", "Blink", "Wide"):
                key = obj.data.shape_keys.key_blocks[pose]
                for index, world_coordinate in enumerate(lid_coordinates(side, part, pose)):
                    local_coordinate = inverse @ Vector(world_coordinate)
                    key.data[index].co = local_coordinate
                    if pose == "Basis":
                        obj.data.vertices[index].co = local_coordinate
            obj.data.update()

    rebuild_iris(bpy.data.materials["MAT_Eye_Iris_Amber"])
    tune_principled_material(bpy.data.materials["MAT_Eye_Iris_Limbal"], (0.004, 0.0004, 0.00008, 1.0), 0.3, 0.08)
    tune_principled_material(bpy.data.materials["MAT_Eye_Sclera"], (0.72, 0.65, 0.55, 1.0), 0.38, 0.04)
    tune_principled_material(bpy.data.materials["MAT_Eye_Pupil"], (0.00025, 0.00005, 0.00001, 1.0), 0.18, 0.16)
    tune_principled_material(bpy.data.materials["MAT_Eyelid_Skin_Brown"], (0.075, 0.023, 0.007, 1.0), 0.62, 0.0)

    for side, center_x in CENTERS.items():
        set_transform(bpy.data.objects[f"EYE_Iris_{side}"], center_x, -0.2217, IRIS_CZ, (0.066, 0.0018, 0.066))
        set_transform(bpy.data.objects[f"EYE_IrisLimbal_{side}"], center_x, -0.2220, IRIS_CZ, (0.069, 0.0001, 0.069))
        set_transform(bpy.data.objects[f"EYE_Pupil_{side}"], center_x, -0.2223, IRIS_CZ, (0.038, 0.0020, 0.038))
        set_transform(bpy.data.objects[f"EYE_Catchlight_{side}"], center_x + 0.0075, -0.2230, IRIS_CZ + 0.012, (0.0080, 0.0010, 0.0080))
        set_transform(bpy.data.objects[f"EYE_CatchlightSecondary_{side}"], center_x - 0.0080, -0.2230, IRIS_CZ - 0.010, (0.0022, 0.0008, 0.0022))

    render_reviews(bpy.context.scene, head)

    seam_max = {}
    for side in ("L", "R"):
        upper = bpy.data.objects[f"EYE_LidUpper_{side}"]
        lower = bpy.data.objects[f"EYE_LidLower_{side}"]
        seam_max[side] = max(
            (
                upper.matrix_world @ upper.data.shape_keys.key_blocks["Blink"].data[col * ROWS + ROWS - 1].co
                - lower.matrix_world @ lower.data.shape_keys.key_blocks["Blink"].data[col * ROWS + ROWS - 1].co
            ).length
            for col in range(COLS)
        )
    lid_driver_count = sum(
        len(obj.data.shape_keys.animation_data.drivers)
        for obj in bpy.data.objects
        if obj.name.startswith(("EYE_LidUpper_", "EYE_LidLower_")) and obj.data.shape_keys and obj.data.shape_keys.animation_data
    )
    checks = {
        "head_vertex_count": len(head.data.vertices) == 5478,
        "shape_keys_unchanged": [key.name for key in head.data.shape_keys.key_blocks] == shape_names,
        "actions_unchanged": sorted(action.name for action in bpy.data.actions) == action_names,
        "lid_drivers": lid_driver_count == 8,
        "blink_seam": all(value < 1.0e-5 for value in seam_max.values()),
        "iris_within_sclera": all(bpy.data.objects[f"EYE_IrisLimbal_{side}"].dimensions.x < bpy.data.objects[f"EYE_Sclera_{side}"].dimensions.x for side in ("L", "R")),
        "no_temp_objects": not any(obj.name.startswith("TMP_") for obj in bpy.data.objects),
    }
    failed = [name for name, value in checks.items() if not value]
    report = {
        "schema": "sloth_reference_eye_candidate_v3",
        "status": "PASS" if not failed else "FAIL",
        "checkpoint": V3_CHECKPOINT,
        "metrics": {"iris_diameter": 0.066, "limbal_diameter": 0.069, "pupil_diameter": 0.038, "iris_center_z": IRIS_CZ, "lower_aperture_center_z": EYE_CZ - 0.038, "blink_seam_max": seam_max},
        "checks": checks,
        "failed": failed,
        "renders": sorted(os.path.join(RENDER_DIR, name) for name in os.listdir(RENDER_DIR) if name.endswith(".png")),
    }
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    if failed:
        raise RuntimeError("Eye candidate v3 failed: " + ", ".join(failed))
    bpy.ops.wm.save_as_mainfile(filepath=V3_CHECKPOINT, copy=True)
    print(json.dumps({"status": report["status"], "checkpoint": V3_CHECKPOINT, "report": REPORT, "metrics": report["metrics"], "checks": checks, "renders": report["renders"]}, ensure_ascii=False))


if __name__ == "__main__":
    main()
