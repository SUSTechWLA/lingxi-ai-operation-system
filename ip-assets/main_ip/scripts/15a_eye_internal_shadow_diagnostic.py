import bpy
import json
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
FINAL = os.path.join(ROOT, "character_sloth_final.blend")
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_101_pre_eye_internal_shadow_fix.blend")
REPORT = os.path.join(ROOT, "reports", "eye_internal_shadow_diagnostic.json")
RENDER_DIR = os.path.join(ROOT, "renders", "eye_internal_shadow", "diagnostic")


def point_at(obj, target):
    obj.rotation_euler = (Vector(target) - obj.location).to_track_quat("-Z", "Y").to_euler()


def action_fcurves(action):
    curves = []
    if action is None:
        return curves
    legacy = getattr(action, "fcurves", None)
    if legacy is not None:
        return list(legacy)
    for layer in getattr(action, "layers", []):
        for strip in getattr(layer, "strips", []):
            for channelbag in getattr(strip, "channelbags", []):
                curves.extend(list(getattr(channelbag, "fcurves", [])))
    return curves


def input_value(socket):
    try:
        value = socket.default_value
        if hasattr(value, "__len__") and not isinstance(value, str):
            return [round(float(item), 6) for item in value]
        return round(float(value), 6)
    except Exception:
        return None


def material_record(material):
    record = {
        "name": material.name,
        "diffuse_color": [round(value, 6) for value in material.diffuse_color],
        "use_nodes": material.use_nodes,
        "surface_render_method": getattr(material, "surface_render_method", None),
        "show_transparent_back": getattr(material, "show_transparent_back", None),
        "nodes": [],
    }
    if material.use_nodes:
        for node in material.node_tree.nodes:
            node_record = {"name": node.name, "type": node.type, "inputs": {}}
            for socket in node.inputs:
                if not socket.is_linked:
                    value = input_value(socket)
                    if value is not None:
                        node_record["inputs"][socket.name] = value
            record["nodes"].append(node_record)
    return record


def object_record(obj):
    return {
        "name": obj.name,
        "world_location": [round(value, 6) for value in obj.matrix_world.translation],
        "dimensions": [round(value, 6) for value in obj.dimensions],
        "materials": [slot.material.name if slot.material else None for slot in obj.material_slots],
        "hide_render": obj.hide_render,
        "visible_camera": getattr(obj, "visible_camera", None),
        "visible_shadow": getattr(obj, "visible_shadow", None),
        "parent": obj.parent.name if obj.parent else None,
        "parent_bone": obj.parent_bone,
    }


def render(scene, path):
    scene.render.filepath = path
    bpy.ops.render.render(write_still=True)
    if not os.path.exists(path) or os.path.getsize(path) < 50000:
        raise RuntimeError("Diagnostic render missing or too small: " + path)


def main():
    os.makedirs(os.path.dirname(CHECKPOINT), exist_ok=True)
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    os.makedirs(RENDER_DIR, exist_ok=True)
    if os.path.abspath(bpy.data.filepath) != os.path.abspath(FINAL):
        bpy.ops.wm.open_mainfile(filepath=FINAL)
    bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT, copy=True)

    scene = bpy.data.scenes.get("SCENE_LOOKDEV") or bpy.context.scene
    head = bpy.data.objects.get("GEO_HeadBody")
    rig = bpy.data.objects.get("RIG_Sloth")
    if not head or not head.data.shape_keys or not rig:
        raise RuntimeError("Formal head/rig system missing")

    required_objects = []
    for side in ("L", "R"):
        for role in ("Sclera", "Cornea", "Iris", "IrisLimbal", "Pupil", "Catchlight", "CatchlightSecondary"):
            name = f"EYE_{role}_{side}"
            obj = bpy.data.objects.get(name)
            if not obj:
                raise RuntimeError("Missing formal eye object: " + name)
            required_objects.append(obj)
    required_materials = [
        bpy.data.materials.get(name)
        for name in (
            "MAT_Eye_Sclera", "MAT_Eye_Cornea", "MAT_Eye_Iris_Amber",
            "MAT_Eye_Iris_Limbal", "MAT_Eye_Pupil", "MAT_EyeSocket",
        )
    ]
    if any(material is None for material in required_materials):
        raise RuntimeError("Formal eye material system incomplete")

    action = rig.animation_data.action if rig.animation_data else None
    muted_records = []
    for curve in action_fcurves(action):
        if 'pose.bones["Eye.L"]' in curve.data_path or 'pose.bones["Eye.R"]' in curve.data_path:
            muted_records.append((curve, curve.mute))
            curve.mute = True
    old_eye_basis = {name: rig.pose.bones[name].matrix_basis.copy() for name in ("Eye.L", "Eye.R")}
    for name in ("Eye.L", "Eye.R"):
        rig.pose.bones[name].matrix_basis.identity()
    old_shape_values = {key.name: key.value for key in head.data.shape_keys.key_blocks}
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
    old_denoising = scene.cycles.use_denoising

    camera_data = bpy.data.cameras.new("CXR_EyeShadowDiagnosticCamera")
    camera = bpy.data.objects.new("CXR_EyeShadowDiagnosticCamera", camera_data)
    scene.collection.objects.link(camera)
    camera.data.sensor_width = 36.0
    camera.data.lens = 100.0
    camera.location = (0.0, -3.35, 2.20)
    point_at(camera, (0.0, -0.02, 2.20))
    scene.camera = camera
    scene.render.engine = "CYCLES"
    scene.cycles.samples = 8
    scene.cycles.use_denoising = True
    scene.render.resolution_x = 640
    scene.render.resolution_y = 640
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"
    scene.render.image_settings.color_depth = "8"

    tracked_visibility = {obj.name: obj.hide_render for obj in required_objects}
    paths = {}

    paths["full"] = os.path.join(RENDER_DIR, "01_full.png")
    render(scene, paths["full"])

    for side in ("L", "R"):
        bpy.data.objects[f"EYE_Cornea_{side}"].hide_render = True
    paths["cornea_off"] = os.path.join(RENDER_DIR, "02_cornea_off.png")
    render(scene, paths["cornea_off"])
    for side in ("L", "R"):
        bpy.data.objects[f"EYE_Cornea_{side}"].hide_render = tracked_visibility[f"EYE_Cornea_{side}"]

    for side in ("L", "R"):
        bpy.data.objects[f"EYE_Pupil_{side}"].hide_render = True
    paths["pupil_off"] = os.path.join(RENDER_DIR, "03_pupil_off.png")
    render(scene, paths["pupil_off"])
    for side in ("L", "R"):
        bpy.data.objects[f"EYE_Pupil_{side}"].hide_render = tracked_visibility[f"EYE_Pupil_{side}"]

    for side in ("L", "R"):
        bpy.data.objects[f"EYE_IrisLimbal_{side}"].hide_render = True
    paths["limbal_off"] = os.path.join(RENDER_DIR, "04_limbal_off.png")
    render(scene, paths["limbal_off"])
    for side in ("L", "R"):
        bpy.data.objects[f"EYE_IrisLimbal_{side}"].hide_render = tracked_visibility[f"EYE_IrisLimbal_{side}"]

    socket_material = bpy.data.materials["MAT_EyeSocket"]
    socket_shader = next((node for node in socket_material.node_tree.nodes if node.type == "BSDF_PRINCIPLED"), None)
    if not socket_shader:
        raise RuntimeError("Eye socket Principled shader missing")
    old_socket_color = socket_shader.inputs["Base Color"].default_value[:]
    socket_shader.inputs["Base Color"].default_value = (0.30, 0.12, 0.045, 1.0)
    paths["socket_lifted"] = os.path.join(RENDER_DIR, "05_socket_lifted.png")
    render(scene, paths["socket_lifted"])
    socket_shader.inputs["Base Color"].default_value = old_socket_color

    for side in ("L", "R"):
        bpy.data.objects[f"EYE_Catchlight_{side}"].hide_render = True
        bpy.data.objects[f"EYE_CatchlightSecondary_{side}"].hide_render = True
    paths["catchlights_off"] = os.path.join(RENDER_DIR, "06_catchlights_off.png")
    render(scene, paths["catchlights_off"])

    for obj in required_objects:
        obj.hide_render = tracked_visibility[obj.name]
    for key in head.data.shape_keys.key_blocks:
        key.value = old_shape_values[key.name]
    for name in ("Eye.L", "Eye.R"):
        rig.pose.bones[name].matrix_basis = old_eye_basis[name]
    for curve, old_mute in muted_records:
        curve.mute = old_mute
    scene.camera = old_camera
    scene.render.engine = old_engine
    scene.cycles.samples = old_samples
    scene.cycles.use_denoising = old_denoising
    scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage = old_resolution
    scene.render.filepath = old_path
    scene.render.image_settings.file_format = old_format
    scene.render.image_settings.color_mode = old_mode
    scene.render.image_settings.color_depth = old_depth
    bpy.data.objects.remove(camera, do_unlink=True)
    bpy.data.cameras.remove(camera_data)
    bpy.context.view_layer.update()

    object_records = {obj.name: object_record(obj) for obj in required_objects}
    material_records = {material.name: material_record(material) for material in required_materials}
    checks = {
        "checkpoint_written": os.path.exists(CHECKPOINT) and os.path.getsize(CHECKPOINT) > 100000,
        "head_vertex_count_preserved": len(head.data.vertices) == 5478,
        "shape_key_count_preserved": len(head.data.shape_keys.key_blocks) == 31,
        "render_set_complete": all(os.path.exists(path) and os.path.getsize(path) > 50000 for path in paths.values()),
        "visibility_restored": all(obj.hide_render == tracked_visibility[obj.name] for obj in required_objects),
        "eye_animation_restored": all(curve.mute == old_mute for curve, old_mute in muted_records),
        "no_cxr_objects": not any(obj.name.startswith("CXR_") for obj in bpy.data.objects),
    }
    failed = [name for name, value in checks.items() if not value]
    report = {
        "schema": "sloth_eye_internal_shadow_diagnostic_v1",
        "status": "PASS" if not failed else "FAIL",
        "blend_file": bpy.data.filepath,
        "checkpoint": CHECKPOINT,
        "checks": checks,
        "objects": object_records,
        "materials": material_records,
        "renders": paths,
        "hypotheses": {
            "cornea": "Compare full against cornea_off",
            "pupil": "Compare full against pupil_off",
            "limbal": "Compare full against limbal_off",
            "socket": "Compare full against socket_lifted",
            "catchlights": "Compare full against catchlights_off",
        },
    }
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    print(json.dumps({"status": report["status"], "report": REPORT, "checkpoint": CHECKPOINT, "checks": checks, "renders": paths}, ensure_ascii=False))
    if failed:
        raise RuntimeError("Eye internal shadow diagnostic failed: " + ", ".join(failed))


if __name__ == "__main__":
    main()
