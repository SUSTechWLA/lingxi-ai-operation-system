import bpy
import json
import math
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
V1_CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_091_reference_eye_candidate_v1.blend")
V2_CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_092_reference_eye_candidate_v2.blend")
REPORT = os.path.join(ROOT, "reports", "reference_eye_candidate_v2.json")
RENDER_DIR = os.path.join(ROOT, "renders", "reference_refine", "eye_candidate_v2")

EYE_CZ = 2.198
IRIS_CZ = 2.192
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


def set_world_transform(obj, x, y, z, dimensions):
    matrix = obj.matrix_world.copy()
    matrix.translation = Vector((x, y, z))
    obj.matrix_world = matrix
    set_world_dimensions(obj, dimensions)


def input_value(node, names, value):
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
    input_value(shader, ("Roughness",), 0.31)
    input_value(shader, ("IOR",), 1.45)
    input_value(shader, ("Specular IOR Level", "Specular"), 0.34)
    input_value(shader, ("Coat Weight", "Coat"), 0.1)
    input_value(shader, ("Coat Roughness",), 0.2)

    texcoord = nodes.new("ShaderNodeTexCoord")
    separate = nodes.new("ShaderNodeSeparateXYZ")
    links.new(texcoord.outputs["Generated"], separate.inputs["Vector"])
    squared = []
    for socket_name in ("X", "Z"):
        subtract = nodes.new("ShaderNodeMath")
        subtract.operation = "SUBTRACT"
        subtract.inputs[1].default_value = 0.5
        multiply = nodes.new("ShaderNodeMath")
        multiply.operation = "MULTIPLY"
        links.new(separate.outputs[socket_name], subtract.inputs[0])
        links.new(subtract.outputs[0], multiply.inputs[0])
        links.new(subtract.outputs[0], multiply.inputs[1])
        squared.append(multiply)
    add = nodes.new("ShaderNodeMath")
    add.operation = "ADD"
    links.new(squared[0].outputs[0], add.inputs[0])
    links.new(squared[1].outputs[0], add.inputs[1])
    root = nodes.new("ShaderNodeMath")
    root.operation = "SQRT"
    links.new(add.outputs[0], root.inputs[0])

    ramp = nodes.new("ShaderNodeValToRGB")
    color_ramp = ramp.color_ramp
    color_ramp.elements[0].position = 0.0
    color_ramp.elements[0].color = (0.20, 0.032, 0.0025, 1.0)
    color_ramp.elements[1].position = 0.62
    color_ramp.elements[1].color = (0.010, 0.0010, 0.0002, 1.0)
    middle = color_ramp.elements.new(0.29)
    middle.color = (0.43, 0.085, 0.0045, 1.0)
    outer = color_ramp.elements.new(0.46)
    outer.color = (0.10, 0.012, 0.0007, 1.0)
    links.new(root.outputs[0], ramp.inputs["Fac"])

    noise = nodes.new("ShaderNodeTexNoise")
    noise.inputs["Scale"].default_value = 22.0
    noise.inputs["Detail"].default_value = 3.5
    noise.inputs["Roughness"].default_value = 0.7
    links.new(texcoord.outputs["Generated"], noise.inputs["Vector"])
    mix = nodes.new("ShaderNodeMixRGB")
    mix.blend_type = "MULTIPLY"
    mix.inputs[0].default_value = 0.14
    links.new(ramp.outputs["Color"], mix.inputs[1])
    links.new(noise.outputs["Fac"], mix.inputs[2])
    links.new(mix.outputs["Color"], shader.inputs["Base Color"])
    bump = nodes.new("ShaderNodeBump")
    bump.inputs["Strength"].default_value = 0.045
    bump.inputs["Distance"].default_value = 0.02
    links.new(noise.outputs["Fac"], bump.inputs["Height"])
    links.new(bump.outputs["Normal"], shader.inputs["Normal"])
    links.new(shader.outputs["BSDF"], output.inputs["Surface"])


def rebuild_lid(material):
    material.use_nodes = True
    nodes = material.node_tree.nodes
    links = material.node_tree.links
    nodes.clear()
    output = nodes.new("ShaderNodeOutputMaterial")
    shader = nodes.new("ShaderNodeBsdfPrincipled")
    shader.inputs["Base Color"].default_value = (0.050, 0.014, 0.005, 1.0)
    input_value(shader, ("Roughness",), 0.64)
    input_value(shader, ("IOR",), 1.42)
    input_value(shader, ("Specular IOR Level", "Specular"), 0.22)
    noise = nodes.new("ShaderNodeTexNoise")
    noise.inputs["Scale"].default_value = 55.0
    noise.inputs["Detail"].default_value = 2.0
    bump = nodes.new("ShaderNodeBump")
    bump.inputs["Strength"].default_value = 0.025
    bump.inputs["Distance"].default_value = 0.01
    links.new(noise.outputs["Fac"], bump.inputs["Height"])
    links.new(bump.outputs["Normal"], shader.inputs["Normal"])
    links.new(shader.outputs["BSDF"], output.inputs["Surface"])


def rebuild_limbal(material):
    material.use_nodes = True
    nodes = material.node_tree.nodes
    links = material.node_tree.links
    nodes.clear()
    output = nodes.new("ShaderNodeOutputMaterial")
    shader = nodes.new("ShaderNodeBsdfPrincipled")
    shader.inputs["Base Color"].default_value = (0.006, 0.0007, 0.0001, 1.0)
    input_value(shader, ("Roughness",), 0.3)
    input_value(shader, ("Coat Weight", "Coat"), 0.08)
    links.new(shader.outputs["BSDF"], output.inputs["Surface"])


def lid_coordinates(side, part, pose):
    center_x = CENTERS[side]
    result = []
    for col in range(COLS):
        u = -1.0 + 2.0 * col / (COLS - 1)
        s = math.sqrt(max(0.0, 1.0 - u * u))
        taper = s ** 1.5
        x = center_x + EYE_RX * u
        if pose == "Blink":
            inner_z = EYE_CZ - 0.018 * s - 0.003 * s * s
            outer_z = EYE_CZ + 0.038 * s if part == "Upper" else EYE_CZ - 0.052 * s
        elif part == "Upper":
            inner_z = EYE_CZ + (0.025 if pose == "Basis" else 0.040) * s
            outer_z = inner_z + 0.007 * s * taper
        else:
            inner_z = EYE_CZ - (0.043 if pose == "Basis" else 0.050) * s
            outer_z = inner_z - 0.004 * s * taper
        inner_y = -0.2510 + 0.0010 * u * u
        raw_outer_y = -0.2474 - 0.0003 * s
        outer_y = inner_y + (raw_outer_y - inner_y) * taper
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
    old_values = {key.name: key.value for key in head.data.shape_keys.key_blocks}
    camera_data = bpy.data.cameras.new("TMP_ReferenceEyeV2Camera")
    camera = bpy.data.objects.new("TMP_ReferenceEyeV2Camera", camera_data)
    scene.collection.objects.link(camera)
    camera_data.sensor_width = 36.0
    scene.camera = camera
    scene.render.engine = "BLENDER_EEVEE"
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
    head.data.shape_keys.key_blocks["Eye_Wide.L"].value = 0.75
    head.data.shape_keys.key_blocks["Eye_Wide.R"].value = 0.75
    bpy.context.view_layer.update()
    render("wide_front.png", (0.0, -3.35, 2.20))
    for key in head.data.shape_keys.key_blocks:
        key.value = old_values[key.name]
    scene.camera = old_camera
    scene.render.engine = old_engine
    scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage = old_resolution
    scene.render.filepath = old_path
    scene.render.image_settings.file_format = old_format
    bpy.data.objects.remove(camera, do_unlink=True)
    bpy.data.cameras.remove(camera_data)


def main():
    os.makedirs(os.path.dirname(V2_CHECKPOINT), exist_ok=True)
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    os.makedirs(RENDER_DIR, exist_ok=True)
    head = bpy.data.objects.get("GEO_HeadBody")
    if not head or len(head.data.vertices) != 5478 or not head.data.shape_keys:
        raise RuntimeError("Formal head invariant missing")
    shape_names = [key.name for key in head.data.shape_keys.key_blocks]
    action_names = sorted(action.name for action in bpy.data.actions)
    if not os.path.exists(V1_CHECKPOINT):
        raise RuntimeError("V1 recovery checkpoint missing")

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
    rebuild_limbal(bpy.data.materials["MAT_Eye_Iris_Limbal"])
    rebuild_lid(bpy.data.materials["MAT_Eyelid_Skin_Brown"])

    for side, center_x in CENTERS.items():
        set_world_transform(bpy.data.objects[f"EYE_Iris_{side}"], center_x, -0.2217, IRIS_CZ, (0.065, 0.0018, 0.065))
        set_world_transform(bpy.data.objects[f"EYE_IrisLimbal_{side}"], center_x, -0.2220, IRIS_CZ, (0.0685, 0.0001, 0.0685))
        set_world_transform(bpy.data.objects[f"EYE_Pupil_{side}"], center_x, -0.2223, IRIS_CZ, (0.036, 0.0020, 0.036))
        set_world_transform(bpy.data.objects[f"EYE_Catchlight_{side}"], center_x + 0.0075, -0.2230, IRIS_CZ + 0.012, (0.0082, 0.0010, 0.0082))
        set_world_transform(bpy.data.objects[f"EYE_CatchlightSecondary_{side}"], center_x - 0.0085, -0.2230, IRIS_CZ - 0.010, (0.0024, 0.0008, 0.0024))

    render_reviews(bpy.context.scene, head)

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
        "schema": "sloth_reference_eye_candidate_v2",
        "status": "PASS" if not failed else "FAIL",
        "checkpoint": V2_CHECKPOINT,
        "metrics": {
            "iris_diameter": 0.065,
            "limbal_diameter": 0.0685,
            "pupil_diameter": 0.036,
            "iris_center_z": IRIS_CZ,
            "upper_aperture_center_z": EYE_CZ + 0.025,
            "lower_aperture_center_z": EYE_CZ - 0.043,
            "blink_seam_max": seam_max,
        },
        "checks": checks,
        "failed": failed,
        "renders": sorted(os.path.join(RENDER_DIR, name) for name in os.listdir(RENDER_DIR) if name.endswith(".png")),
    }
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    if failed:
        raise RuntimeError("Eye candidate v2 failed: " + ", ".join(failed))
    bpy.ops.wm.save_as_mainfile(filepath=V2_CHECKPOINT, copy=True)
    print(json.dumps({"status": report["status"], "checkpoint": V2_CHECKPOINT, "report": REPORT, "metrics": report["metrics"], "checks": checks, "renders": report["renders"]}, ensure_ascii=False))


if __name__ == "__main__":
    main()
