import bpy
import json
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
FINAL = os.path.join(ROOT, "character_sloth_final.blend")
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_100_pre_eye_socket_balance.blend")
REPORT = os.path.join(ROOT, "reports", "eye_socket_color_balance.json")
RENDER_DIR = os.path.join(ROOT, "renders", "final")


def point_at(obj, target):
    obj.rotation_euler = (Vector(target) - obj.location).to_track_quat("-Z", "Y").to_euler()


def set_input(node, names, value):
    for name in names:
        if name in node.inputs:
            node.inputs[name].default_value = value
            return


def action_fcurves(action):
    curves = []
    for layer in getattr(action, "layers", []):
        for strip in getattr(layer, "strips", []):
            for channelbag in getattr(strip, "channelbags", []):
                curves.extend(list(getattr(channelbag, "fcurves", [])))
    return curves


def render_pair(scene, camera, basename):
    paths = {}
    for file_format, extension, depth in (("PNG", "png", "8"), ("OPEN_EXR", "exr", "32")):
        path = os.path.join(RENDER_DIR, basename + "." + extension)
        scene.render.image_settings.file_format = file_format
        scene.render.image_settings.color_mode = "RGBA"
        scene.render.image_settings.color_depth = depth
        scene.render.filepath = path
        bpy.ops.render.render(write_still=True)
        paths[extension] = path
    return paths


def main():
    os.makedirs(os.path.dirname(CHECKPOINT), exist_ok=True)
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    if os.path.abspath(bpy.data.filepath) != os.path.abspath(FINAL):
        bpy.ops.wm.open_mainfile(filepath=FINAL)
    bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT, copy=True)
    scene = bpy.data.scenes.get("SCENE_LOOKDEV") or bpy.context.scene
    rig = bpy.data.objects.get("RIG_Sloth")
    head = bpy.data.objects.get("GEO_HeadBody")
    material = bpy.data.materials.get("MAT_EyeSocket")
    if not rig or not head or not material or not material.use_nodes:
        raise RuntimeError("Formal eye socket system missing")
    shader = next((node for node in material.node_tree.nodes if node.type == "BSDF_PRINCIPLED"), None)
    if not shader:
        raise RuntimeError("Eye socket Principled shader missing")
    shader.inputs["Base Color"].default_value = (0.115, 0.038, 0.013, 1.0)
    set_input(shader, ("Roughness",), 0.66)
    set_input(shader, ("Specular IOR Level", "Specular"), 0.18)
    material.diffuse_color = (0.115, 0.038, 0.013, 1.0)
    material["reference_color_intent"] = "warm_brown_socket_depth_without_near_black_wedge"

    action = rig.animation_data.action if rig.animation_data else None
    muted = []
    for curve in action_fcurves(action):
        if 'pose.bones["Eye.L"]' in curve.data_path or 'pose.bones["Eye.R"]' in curve.data_path:
            muted.append((curve, curve.mute))
            curve.mute = True
    old_eye_basis = {name: rig.pose.bones[name].matrix_basis.copy() for name in ("Eye.L", "Eye.R")}
    for name in ("Eye.L", "Eye.R"):
        rig.pose.bones[name].matrix_basis.identity()
    old_values = {key.name: key.value for key in head.data.shape_keys.key_blocks}
    for key in head.data.shape_keys.key_blocks:
        if key.name != "Basis":
            key.value = 0.0
    head.data.shape_keys.key_blocks["Mouth_Rest"].value = 1.0
    bpy.context.view_layer.update()

    old_camera = scene.camera
    old_engine = scene.render.engine
    old_resolution = (scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage)
    old_path = scene.render.filepath
    old_format = scene.render.image_settings.file_format
    old_mode = scene.render.image_settings.color_mode
    old_depth = scene.render.image_settings.color_depth
    old_samples = scene.cycles.samples
    camera_data = bpy.data.cameras.new("TMP_EyeSocketBalanceCamera")
    camera = bpy.data.objects.new("TMP_EyeSocketBalanceCamera", camera_data)
    scene.collection.objects.link(camera)
    camera.data.sensor_width = 36.0
    camera.data.lens = 100.0
    camera.location = (0.0, -3.35, 2.20)
    point_at(camera, (0.0, -0.02, 2.20))
    scene.camera = camera
    scene.render.engine = "CYCLES"
    scene.cycles.samples = 24
    scene.cycles.use_denoising = True
    scene.render.resolution_x = 1024
    scene.render.resolution_y = 1024
    scene.render.resolution_percentage = 100
    outputs = {"face_closeup": render_pair(scene, camera, "face_closeup")}
    head.data.shape_keys.key_blocks["Blink.L"].value = 1.0
    head.data.shape_keys.key_blocks["Blink.R"].value = 1.0
    bpy.context.view_layer.update()
    outputs["blink"] = render_pair(scene, camera, "blink")

    for key in head.data.shape_keys.key_blocks:
        key.value = old_values[key.name]
    for name in ("Eye.L", "Eye.R"):
        rig.pose.bones[name].matrix_basis = old_eye_basis[name]
    for curve, old_mute in muted:
        curve.mute = old_mute
    scene.camera = old_camera
    scene.render.engine = old_engine
    scene.cycles.samples = old_samples
    scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage = old_resolution
    scene.render.filepath = old_path
    scene.render.image_settings.file_format = old_format
    scene.render.image_settings.color_mode = old_mode
    scene.render.image_settings.color_depth = old_depth
    bpy.data.objects.remove(camera, do_unlink=True)
    bpy.data.cameras.remove(camera_data)
    bpy.context.view_layer.update()

    checks = {
        "head_vertex_count": len(head.data.vertices) == 5478,
        "shape_key_count": len(head.data.shape_keys.key_blocks) == 31,
        "active_action_restored": (rig.animation_data.action.name if rig.animation_data and rig.animation_data.action else None) == (action.name if action else None),
        "eye_fcurves_restored": all(curve.mute == old_mute for curve, old_mute in muted),
        "socket_is_warm_brown": tuple(round(value, 3) for value in material.diffuse_color[:3]) == (0.115, 0.038, 0.013),
        "render_pairs": all(os.path.exists(path) and os.path.getsize(path) > 100000 for record in outputs.values() for path in record.values()),
        "no_temp_objects": not any(obj.name.startswith("TMP_") for obj in bpy.data.objects),
    }
    failed = [name for name, value in checks.items() if not value]
    if failed:
        raise RuntimeError("Eye socket balance failed: " + ", ".join(failed))
    bpy.ops.wm.save_as_mainfile(filepath=FINAL, copy=False)
    report = {"schema": "sloth_eye_socket_color_balance_v1", "status": "PASS", "checkpoint": CHECKPOINT, "final": FINAL, "checks": checks, "renders": outputs}
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    print(json.dumps({"status": "PASS", "final": FINAL, "report": REPORT, "checks": checks, "renders": outputs}, ensure_ascii=False))


if __name__ == "__main__":
    main()
