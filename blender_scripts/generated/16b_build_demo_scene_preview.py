import bpy
import json
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
FINAL = os.path.join(ROOT, "character_sloth_final.blend")
DEMO_BLEND = os.path.join(ROOT, "demos", "sloth_production_demo.blend")
REPORT = os.path.join(ROOT, "reports", "demo_scene_setup.json")
PREVIEW_DIR = os.path.join(ROOT, "renders", "demo", "preview")
DEMO_SCENE = "SCENE_DEMO_PRODUCTION"
FPS = 24
FRAME_START = 1
FRAME_END = 120


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


def point_at(obj, target):
    obj.rotation_euler = (Vector(target) - obj.location).to_track_quat("-Z", "Y").to_euler()


def remove_existing_demo_data():
    scene = bpy.data.scenes.get(DEMO_SCENE)
    if scene:
        bpy.data.scenes.remove(scene)
    collection = bpy.data.collections.get("CXR_DEMO_SETUP")
    if collection:
        for obj in list(collection.objects):
            data = obj.data
            bpy.data.objects.remove(obj, do_unlink=True)
            if data and data.users == 0:
                if isinstance(data, bpy.types.Mesh):
                    bpy.data.meshes.remove(data)
                elif isinstance(data, bpy.types.Camera):
                    bpy.data.cameras.remove(data)
        bpy.data.collections.remove(collection)
    for name in ("CXR_DEMO_TalkLoop_Slow", "CXR_DEMO_FaceAction", "CXR_DEMO_CameraAction", "CXR_DEMO_TargetAction"):
        action = bpy.data.actions.get(name)
        if action:
            bpy.data.actions.remove(action)
    for name in ("CXR_DEMO_Ground", "CXR_DEMO_Camera", "CXR_DEMO_Target"):
        obj = bpy.data.objects.get(name)
        if obj:
            data = obj.data
            bpy.data.objects.remove(obj, do_unlink=True)
            if data and data.users == 0:
                if isinstance(data, bpy.types.Mesh):
                    bpy.data.meshes.remove(data)
                elif isinstance(data, bpy.types.Camera):
                    bpy.data.cameras.remove(data)
    for name in ("CXR_DEMO_GroundMesh", "CXR_DEMO_CameraData"):
        mesh = bpy.data.meshes.get(name)
        if mesh:
            bpy.data.meshes.remove(mesh)
        camera = bpy.data.cameras.get(name)
        if camera:
            bpy.data.cameras.remove(camera)
    for name in ("CXR_DEMO_GroundMaterial",):
        material = bpy.data.materials.get(name)
        if material:
            bpy.data.materials.remove(material)
    world = bpy.data.worlds.get("CXR_DEMO_World")
    if world:
        bpy.data.worlds.remove(world)


def create_ground(collection):
    mesh = bpy.data.meshes.new("CXR_DEMO_GroundMesh")
    profile = [(-5.5, 0.0), (0.8, 0.0), (1.55, 0.18), (2.10, 0.75), (2.38, 1.65), (2.48, 4.5)]
    vertices = []
    for x in (-7.5, 7.5):
        for y, z in profile:
            vertices.append((x, y, z - 0.015))
    count = len(profile)
    faces = []
    for index in range(count - 1):
        faces.append((index, index + 1, count + index + 1, count + index))
    mesh.from_pydata(vertices, [], faces)
    mesh.update()
    ground = bpy.data.objects.new("CXR_DEMO_Ground", mesh)
    collection.objects.link(ground)
    for polygon in mesh.polygons:
        polygon.use_smooth = True
    bevel = ground.modifiers.new("CXR_DEMO_GroundBevel", "BEVEL")
    bevel.width = 0.18
    bevel.segments = 5

    material = bpy.data.materials.new("CXR_DEMO_GroundMaterial")
    material.use_nodes = True
    shader = next(node for node in material.node_tree.nodes if node.type == "BSDF_PRINCIPLED")
    shader.inputs["Base Color"].default_value = (0.028, 0.012, 0.006, 1.0)
    shader.inputs["Roughness"].default_value = 0.72
    specular = shader.inputs.get("Specular IOR Level") or shader.inputs.get("Specular")
    if specular:
        specular.default_value = 0.22
    mesh.materials.append(material)
    return ground


def create_demo_world(source_world):
    world = source_world.copy() if source_world else bpy.data.worlds.new("CXR_DEMO_World")
    world.name = "CXR_DEMO_World"
    world.use_nodes = True
    background = next((node for node in world.node_tree.nodes if node.type == "BACKGROUND"), None)
    if background:
        background.inputs["Color"].default_value = (0.012, 0.006, 0.003, 1.0)
        background.inputs["Strength"].default_value = 0.22
    return world


def create_camera(collection):
    camera_data = bpy.data.cameras.new("CXR_DEMO_CameraData")
    camera = bpy.data.objects.new("CXR_DEMO_Camera", camera_data)
    collection.objects.link(camera)
    camera_data.sensor_width = 36.0
    camera_data.lens = 55.0
    target = bpy.data.objects.new("CXR_DEMO_Target", None)
    collection.objects.link(target)
    target.empty_display_type = "SPHERE"
    target.empty_display_size = 0.08
    target.location = (0.0, -0.01, 1.34)
    constraint = camera.constraints.new("TRACK_TO")
    constraint.name = "CXR_DEMO_TrackCharacter"
    constraint.target = target
    constraint.track_axis = "TRACK_NEGATIVE_Z"
    constraint.up_axis = "UP_Y"

    camera_keys = {
        1: ((0.0, -9.55, 2.08), 55.0),
        42: ((-0.16, -8.05, 2.12), 58.0),
        82: ((0.16, -6.55, 2.18), 64.0),
        120: ((0.0, -5.45, 2.26), 70.0),
    }
    for frame, (location, lens) in camera_keys.items():
        camera.location = location
        camera.data.lens = lens
        camera.keyframe_insert(data_path="location", frame=frame)
        camera.data.keyframe_insert(data_path="lens", frame=frame)
    target_keys = {
        1: (0.0, -0.01, 1.34),
        58: (0.0, -0.01, 1.43),
        120: (0.0, -0.02, 1.66),
    }
    for frame, location in target_keys.items():
        target.location = location
        target.keyframe_insert(data_path="location", frame=frame)
    if camera.animation_data and camera.animation_data.action:
        camera.animation_data.action.name = "CXR_DEMO_CameraAction"
    if camera.data.animation_data and camera.data.animation_data.action:
        camera.data.animation_data.action.name = "CXR_DEMO_CameraLensAction"
    if target.animation_data and target.animation_data.action:
        target.animation_data.action.name = "CXR_DEMO_TargetAction"
    return camera, target


def create_demo_rig_action(rig):
    source = rig.animation_data.action if rig.animation_data else None
    if source is None or source.name != "Talk_Loop":
        raise RuntimeError("Talk_Loop is not the active formal action")
    demo_action = source.copy()
    demo_action.name = "CXR_DEMO_TalkLoop_Slow"
    factor = 2.10
    for curve in action_fcurves(demo_action):
        for keyframe in curve.keyframe_points:
            keyframe.co.x = 1.0 + (keyframe.co.x - 1.0) * factor
            keyframe.handle_left.x = 1.0 + (keyframe.handle_left.x - 1.0) * factor
            keyframe.handle_right.x = 1.0 + (keyframe.handle_right.x - 1.0) * factor
        cycle = curve.modifiers.new("CYCLES")
        cycle.mode_before = "REPEAT"
        cycle.mode_after = "REPEAT"
    rig.animation_data.action = demo_action
    return source, demo_action


def create_demo_face_action(head):
    keys = head.data.shape_keys
    if keys.animation_data:
        keys.animation_data_clear()
    for key in keys.key_blocks:
        if key.name != "Basis":
            key.value = 0.0
    keys.key_blocks["Mouth_Rest"].value = 1.0
    smile_keys = {
        1: (0.18, 0.12),
        28: (0.48, 0.34),
        58: (0.24, 0.18),
        88: (0.58, 0.42),
        120: (0.22, 0.16),
    }
    for frame, (smile, cheek) in smile_keys.items():
        keys.key_blocks["Mouth_Smile"].value = smile
        keys.key_blocks["Mouth_Smile"].keyframe_insert(data_path="value", frame=frame)
        for side in ("L", "R"):
            keys.key_blocks[f"CheekRaise.{side}"].value = cheek
            keys.key_blocks[f"CheekRaise.{side}"].keyframe_insert(data_path="value", frame=frame)
    for side in ("L", "R"):
        blink = keys.key_blocks[f"Blink.{side}"]
        for frame, value in ((1, 0.0), (34, 0.0), (36, 1.0), (38, 0.0), (82, 0.0), (84, 1.0), (86, 0.0), (120, 0.0)):
            blink.value = value
            blink.keyframe_insert(data_path="value", frame=frame)
    action = keys.animation_data.action if keys.animation_data else None
    if not action:
        raise RuntimeError("Demo face action was not created")
    action.name = "CXR_DEMO_FaceAction"
    return action


def set_eevee(scene):
    for identifier in ("BLENDER_EEVEE_NEXT", "BLENDER_EEVEE"):
        try:
            scene.render.engine = identifier
            return identifier
        except Exception:
            continue
    raise RuntimeError("Eevee render engine is unavailable")


def main():
    os.makedirs(os.path.dirname(DEMO_BLEND), exist_ok=True)
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    os.makedirs(PREVIEW_DIR, exist_ok=True)
    if os.path.abspath(bpy.data.filepath) != os.path.abspath(FINAL):
        bpy.ops.wm.open_mainfile(filepath=FINAL)
    remove_existing_demo_data()

    source_scene = bpy.data.scenes.get("Scene")
    formal = bpy.data.collections.get("COL_CHR_SLOTH_FINAL")
    production_lights = bpy.data.collections.get("COL_PRODUCTION_LIGHTS")
    rig = bpy.data.objects.get("RIG_Sloth")
    head = bpy.data.objects.get("GEO_HeadBody")
    if not source_scene or not formal or not production_lights or not rig or not head or not head.data.shape_keys:
        raise RuntimeError("Formal production scene components are missing")

    scene = bpy.data.scenes.new(DEMO_SCENE)
    scene.collection.children.link(formal)
    scene.collection.children.link(production_lights)
    setup = bpy.data.collections.new("CXR_DEMO_SETUP")
    scene.collection.children.link(setup)
    ground = create_ground(setup)
    camera, target = create_camera(setup)
    scene.camera = camera
    scene.world = create_demo_world(source_scene.world)
    source_action, demo_action = create_demo_rig_action(rig)
    face_action = create_demo_face_action(head)

    scene.frame_start = FRAME_START
    scene.frame_end = FRAME_END
    scene.render.fps = FPS
    scene.render.fps_base = 1.0
    engine = set_eevee(scene)
    scene.render.resolution_x = 960
    scene.render.resolution_y = 540
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"
    scene.render.image_settings.color_depth = "8"
    scene.render.film_transparent = False
    scene.view_settings.view_transform = "AgX"
    for look in ("AgX - Medium High Contrast", "Medium High Contrast", "None"):
        try:
            scene.view_settings.look = look
            break
        except Exception:
            continue
    bpy.context.window.scene = scene

    previews = {}
    for frame in (1, 60, 120):
        scene.frame_set(frame)
        path = os.path.join(PREVIEW_DIR, f"demo_preview_{frame:03d}.png")
        scene.render.filepath = path
        bpy.ops.render.render(write_still=True)
        previews[str(frame)] = path
    scene.frame_set(1)

    checks = {
        "shared_formal_collection": formal.name in {child.name for child in scene.collection.children},
        "single_armature": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]) == 1,
        "formal_head_invariants": len(head.data.vertices) == 5478 and len(head.data.shape_keys.key_blocks) == 31,
        "source_action_unchanged": source_action.name == "Talk_Loop" and [round(value, 4) for value in source_action.frame_range] == [1.0, 29.0],
        "demo_action_is_copy": demo_action is not source_action and demo_action.name == "CXR_DEMO_TalkLoop_Slow",
        "demo_face_action": face_action.name == "CXR_DEMO_FaceAction",
        "camera_and_target": camera.name == "CXR_DEMO_Camera" and target.name == "CXR_DEMO_Target",
        "production_lights_shared": production_lights.name in {child.name for child in scene.collection.children},
        "preview_frames_written": all(os.path.exists(path) and os.path.getsize(path) > 50000 for path in previews.values()),
        "only_one_formal_character": len([obj for obj in scene.objects if obj.name == "GEO_HeadBody"]) == 1 and len([obj for obj in scene.objects if obj.name == "RIG_Sloth"]) == 1,
    }
    failed = [name for name, value in checks.items() if not value]
    if failed:
        raise RuntimeError("Demo scene setup failed: " + ", ".join(failed))

    bpy.ops.wm.save_as_mainfile(filepath=DEMO_BLEND, copy=False)
    report = {
        "schema": "sloth_demo_scene_setup_v1",
        "status": "PASS",
        "source": FINAL,
        "demo_blend": DEMO_BLEND,
        "scene": scene.name,
        "shared_character_collection": formal.name,
        "shared_lighting_collection": production_lights.name,
        "temporary_setup_collection": setup.name,
        "engine": engine,
        "resolution": [960, 540],
        "fps": FPS,
        "frame_range": [FRAME_START, FRAME_END],
        "duration_seconds": FRAME_END / FPS,
        "source_action": source_action.name,
        "demo_action": demo_action.name,
        "demo_action_frame_range": [round(value, 4) for value in demo_action.frame_range],
        "face_action": face_action.name,
        "previews": previews,
        "checks": checks,
    }
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    print(json.dumps({
        "status": "PASS",
        "demo_blend": DEMO_BLEND,
        "scene": scene.name,
        "engine": engine,
        "previews": previews,
        "report": REPORT,
        "checks": checks,
    }, ensure_ascii=False))


if __name__ == "__main__":
    main()
