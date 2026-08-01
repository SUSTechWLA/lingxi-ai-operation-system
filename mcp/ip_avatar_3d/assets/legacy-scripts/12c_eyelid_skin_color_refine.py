import bpy
import json
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
CLEAN_CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_095_reference_face_form.blend")
RESULT_CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_097_eyelid_skin_color.blend")
REPORT = os.path.join(ROOT, "reports", "eyelid_skin_color_refine.json")
RENDER_DIR = os.path.join(ROOT, "renders", "reference_refine", "eyelid_skin_color")


def point_at(obj, target):
    obj.rotation_euler = (Vector(target) - obj.location).to_track_quat("-Z", "Y").to_euler()


def set_input(node, names, value):
    for name in names:
        if name in node.inputs:
            node.inputs[name].default_value = value
            return


def rebuild_eyelid_material(material):
    material.use_nodes = True
    nodes = material.node_tree.nodes
    links = material.node_tree.links
    nodes.clear()
    output = nodes.new("ShaderNodeOutputMaterial")
    shader = nodes.new("ShaderNodeBsdfPrincipled")
    shader.location = (420, 20)
    set_input(shader, ("Roughness",), 0.67)
    set_input(shader, ("IOR",), 1.42)
    set_input(shader, ("Specular IOR Level", "Specular"), 0.20)
    set_input(shader, ("Sheen Weight", "Sheen"), 0.05)

    noise = nodes.new("ShaderNodeTexNoise")
    noise.location = (-420, 40)
    noise.inputs["Scale"].default_value = 34.0
    noise.inputs["Detail"].default_value = 2.2
    noise.inputs["Roughness"].default_value = 0.68
    noise.inputs["Distortion"].default_value = 0.08

    ramp = nodes.new("ShaderNodeValToRGB")
    ramp.location = (-140, 80)
    ramp.color_ramp.elements[0].position = 0.22
    ramp.color_ramp.elements[0].color = (0.105, 0.034, 0.012, 1.0)
    ramp.color_ramp.elements[1].position = 0.78
    ramp.color_ramp.elements[1].color = (0.245, 0.102, 0.038, 1.0)
    links.new(noise.outputs["Fac"], ramp.inputs["Fac"])
    links.new(ramp.outputs["Color"], shader.inputs["Base Color"])

    bump = nodes.new("ShaderNodeBump")
    bump.location = (120, -140)
    bump.inputs["Strength"].default_value = 0.035
    bump.inputs["Distance"].default_value = 0.008
    links.new(noise.outputs["Fac"], bump.inputs["Height"])
    links.new(bump.outputs["Normal"], shader.inputs["Normal"])
    links.new(shader.outputs["BSDF"], output.inputs["Surface"])
    material.diffuse_color = (0.17, 0.065, 0.023, 1.0)
    material["reference_color_intent"] = "warm_light_brown_matching_surrounding_face_fur"


def render_reviews(scene, head):
    old_camera = scene.camera
    old_engine = scene.render.engine
    old_resolution = (scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage)
    old_path = scene.render.filepath
    old_format = scene.render.image_settings.file_format
    old_samples = scene.cycles.samples
    old_values = {key.name: key.value for key in head.data.shape_keys.key_blocks}
    camera_data = bpy.data.cameras.new("TMP_EyelidColorCamera")
    camera = bpy.data.objects.new("TMP_EyelidColorCamera", camera_data)
    scene.collection.objects.link(camera)
    camera.data.sensor_width = 36.0
    camera.location = (0.0, -3.35, 2.20)
    camera.data.lens = 100.0
    point_at(camera, (0.0, -0.02, 2.20))
    scene.camera = camera
    scene.render.resolution_x = 720
    scene.render.resolution_y = 720
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "PNG"
    for key in head.data.shape_keys.key_blocks:
        if key.name != "Basis":
            key.value = 0.0
    head.data.shape_keys.key_blocks["Mouth_Rest"].value = 1.0

    scene.render.engine = "BLENDER_EEVEE"
    bpy.context.view_layer.update()
    scene.render.filepath = os.path.join(RENDER_DIR, "neutral_eevee.png")
    bpy.ops.render.render(write_still=True)
    head.data.shape_keys.key_blocks["Blink.L"].value = 1.0
    head.data.shape_keys.key_blocks["Blink.R"].value = 1.0
    bpy.context.view_layer.update()
    scene.render.filepath = os.path.join(RENDER_DIR, "blink_eevee.png")
    bpy.ops.render.render(write_still=True)

    scene.render.engine = "CYCLES"
    scene.cycles.samples = 32
    scene.cycles.use_denoising = True
    scene.render.filepath = os.path.join(RENDER_DIR, "blink_cycles.png")
    bpy.ops.render.render(write_still=True)
    head.data.shape_keys.key_blocks["Blink.L"].value = 0.0
    head.data.shape_keys.key_blocks["Blink.R"].value = 0.0
    bpy.context.view_layer.update()
    scene.render.filepath = os.path.join(RENDER_DIR, "neutral_cycles.png")
    bpy.ops.render.render(write_still=True)

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
    os.makedirs(os.path.dirname(RESULT_CHECKPOINT), exist_ok=True)
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    os.makedirs(RENDER_DIR, exist_ok=True)
    if not os.path.exists(CLEAN_CHECKPOINT):
        raise RuntimeError("Clean eyelid recovery checkpoint missing")
    bpy.ops.wm.open_mainfile(filepath=CLEAN_CHECKPOINT)
    head = bpy.data.objects.get("GEO_HeadBody")
    material = bpy.data.materials.get("MAT_Eyelid_Skin_Brown")
    if not head or len(head.data.vertices) != 5478 or not head.data.shape_keys or not material:
        raise RuntimeError("Formal eyelid system missing after recovery")
    shape_names = [key.name for key in head.data.shape_keys.key_blocks]
    action_names = sorted(action.name for action in bpy.data.actions)
    rebuild_eyelid_material(material)
    render_reviews(bpy.context.scene, head)

    lids = [bpy.data.objects[f"EYE_Lid{part}_{side}"] for side in ("L", "R") for part in ("Upper", "Lower")]
    shared_material = all(obj.material_slots and obj.material_slots[0].material is material for obj in lids)
    seam_max = {}
    for side in ("L", "R"):
        upper = bpy.data.objects[f"EYE_LidUpper_{side}"]
        lower = bpy.data.objects[f"EYE_LidLower_{side}"]
        seam_max[side] = max(
            (
                upper.matrix_world @ upper.data.shape_keys.key_blocks["Blink"].data[col * 6 + 5].co
                - lower.matrix_world @ lower.data.shape_keys.key_blocks["Blink"].data[col * 6 + 5].co
            ).length
            for col in range(48)
        )
    checks = {
        "head_vertex_count": len(head.data.vertices) == 5478,
        "shape_keys_unchanged": [key.name for key in head.data.shape_keys.key_blocks] == shape_names,
        "actions_unchanged": sorted(action.name for action in bpy.data.actions) == action_names,
        "all_lids_share_material": shared_material,
        "blink_seam": all(value < 1.0e-5 for value in seam_max.values()),
        "material_is_warm_brown": tuple(round(value, 3) for value in material.diffuse_color[:3]) == (0.17, 0.065, 0.023),
        "no_temp_objects": not any(obj.name.startswith("TMP_") for obj in bpy.data.objects),
    }
    failed = [name for name, value in checks.items() if not value]
    report = {
        "schema": "sloth_eyelid_skin_color_refine_v1",
        "status": "PASS" if not failed else "FAIL",
        "clean_source": CLEAN_CHECKPOINT,
        "checkpoint": RESULT_CHECKPOINT,
        "color_intent": "warm light brown close to surrounding facial fur, with restrained variation",
        "diffuse_color": list(material.diffuse_color),
        "blink_seam_max": seam_max,
        "checks": checks,
        "failed": failed,
        "renders": sorted(os.path.join(RENDER_DIR, name) for name in os.listdir(RENDER_DIR) if name.endswith(".png")),
    }
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    if failed:
        raise RuntimeError("Eyelid color refinement failed: " + ", ".join(failed))
    bpy.ops.wm.save_as_mainfile(filepath=RESULT_CHECKPOINT, copy=True)
    print(json.dumps({"status": report["status"], "checkpoint": RESULT_CHECKPOINT, "report": REPORT, "diffuse_color": report["diffuse_color"], "blink_seam_max": seam_max, "checks": checks, "renders": report["renders"]}, ensure_ascii=False))


if __name__ == "__main__":
    main()
