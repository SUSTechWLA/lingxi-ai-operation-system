import bpy
import json
import math
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_060_rig_validated.blend")
REPORT = os.path.join(ROOT, "reports", "gate8_rig_validation_report.json")
RENDER_DIR = os.path.join(ROOT, "renders", "validation", "gate8")
MAIN_OBJECT = "part_00000001.001"
EXPECTED_KEYS = [
    "Basis", "Mouth_Rest", "Mouth_A", "Mouth_E", "Mouth_O", "Mouth_U", "Mouth_MBP",
    "Mouth_Smile", "Mouth_Frown", "Mouth_Surprise", "Eye_Squint.L", "Eye_Squint.R",
    "Eye_Wide.L", "Eye_Wide.R", "Eye_Look_Left", "Eye_Look_Right", "Brow_Raise.L",
    "Brow_Raise.R", "Brow_Furrow.L", "Brow_Furrow.R", "Cheek_Smile.L", "Cheek_Smile.R",
    "Cheek_Puff.L", "Cheek_Puff.R", "Nose_Flare.L", "Nose_Flare.R", "Blink.L", "Blink.R",
    "CheekRaise.L", "CheekRaise.R", "JawOpen",
]


def ensure_dir(path):
    os.makedirs(path, exist_ok=True)


def look_at(obj, point):
    obj.rotation_euler = (Vector(point) - obj.location).to_track_quat("-Z", "Y").to_euler()


def reset_expressions(keys):
    for key in keys:
        if key.name != "Basis":
            key.value = 1.0 if key.name == "Mouth_Rest" else 0.0


def set_values(keys, values):
    reset_expressions(keys)
    for name, value in values.items():
        if name in keys:
            keys[name].value = value


def render_test(scene, camera, name, closeup=False):
    if closeup:
        camera.location = (0.0, -4.63, 2.30)
        camera.data.lens = 100
        look_at(camera, (0.0, -0.02, 2.22))
    else:
        camera.location = (0.0, -8.40, 2.45)
        camera.data.lens = 65
        look_at(camera, (0.0, 0.0, 1.30))
    scene.camera = camera
    bpy.context.view_layer.update()
    path = os.path.join(RENDER_DIR, name + ".png")
    scene.render.filepath = path
    bpy.ops.render.render(write_still=True)
    return path


def driver_inventory():
    owners = list(bpy.data.objects) + list(bpy.data.shape_keys)
    records = []
    for owner in owners:
        animation = getattr(owner, "animation_data", None)
        if not animation or not animation.drivers:
            continue
        for fcurve in animation.drivers:
            records.append({
                "owner": owner.name,
                "owner_type": owner.__class__.__name__,
                "data_path": fcurve.data_path,
                "array_index": fcurve.array_index,
                "valid": bool(fcurve.driver.is_valid),
                "variables": len(fcurve.driver.variables),
            })
    return records


def world_bounds(obj):
    corners = [obj.matrix_world @ Vector(corner) for corner in obj.bound_box]
    return {
        "min": [min(c[i] for c in corners) for i in range(3)],
        "max": [max(c[i] for c in corners) for i in range(3)],
    }


ensure_dir(os.path.dirname(CHECKPOINT))
ensure_dir(os.path.dirname(REPORT))
ensure_dir(RENDER_DIR)
main_obj = bpy.data.objects.get(MAIN_OBJECT)
rig = next((obj for obj in bpy.data.objects if obj.type == "ARMATURE"), None)
if main_obj is None or rig is None or main_obj.data.shape_keys is None:
    raise RuntimeError("Gate 8 prerequisites missing")

removed_placeholders = []
for obj in list(bpy.data.objects):
    if obj.name == "CXR_FurEmitter":
        removed_placeholders.append(obj.name)
        bpy.data.objects.remove(obj, do_unlink=True)

keys = main_obj.data.shape_keys.key_blocks
key_names = [key.name for key in keys]
modifier_order = [modifier.type for modifier in main_obj.modifiers]
uv_names = [layer.name for layer in main_obj.data.uv_layers]

max_influences = 0
zero_weight_vertices = 0
over_four_vertices = []
for vertex in main_obj.data.vertices:
    influence_count = sum(1 for element in vertex.groups if element.weight > 1.0e-5)
    max_influences = max(max_influences, influence_count)
    if influence_count == 0:
        zero_weight_vertices += 1
    if influence_count > 4:
        over_four_vertices.append(vertex.index)

drivers = driver_inventory()
invalid_drivers = [record for record in drivers if not record["valid"]]

eye_containment = {}
for side in ("L", "R"):
    cornea = bpy.data.objects["EYE_Cornea_" + side]
    sclera = bpy.data.objects["EYE_Sclera_" + side]
    iris = bpy.data.objects["EYE_Iris_" + side]
    pupil = bpy.data.objects["EYE_Pupil_" + side]
    catchlight = bpy.data.objects["EYE_Catchlight_" + side]
    cb = world_bounds(cornea)
    sb = world_bounds(sclera)
    ib = world_bounds(iris)
    pb = world_bounds(pupil)
    hb = world_bounds(catchlight)
    eye_containment[side] = {
        "cornea_bounds": cb,
        "sclera_bounds": sb,
        "iris_bounds": ib,
        "pupil_bounds": pb,
        "catchlight_bounds": hb,
        "sclera_inside_cornea_xz": sb["min"][0] >= cb["min"][0] - 0.002 and sb["max"][0] <= cb["max"][0] + 0.002 and sb["min"][2] >= cb["min"][2] - 0.002 and sb["max"][2] <= cb["max"][2] + 0.002,
        "front_layers_behind_cornea_front": all(bounds["min"][1] >= cb["min"][1] - 0.001 for bounds in (ib, pb, hb)),
    }

scene = bpy.data.scenes.get("SCENE_LOOKDEV") or bpy.context.scene
bpy.context.window.scene = scene
scene.render.engine = "BLENDER_EEVEE"
scene.render.resolution_x = 512
scene.render.resolution_y = 512
scene.render.resolution_percentage = 100
scene.render.image_settings.file_format = "PNG"
scene.view_layers[0].material_override = None
camera = bpy.data.objects.get("CAM_LOOKDEV_100MM") or bpy.data.objects.get("CAM_LOOKDEV_65MM")

active_action = rig.animation_data.action if rig.animation_data else None
if rig.animation_data:
    rig.animation_data.action = None
bpy.context.view_layer.update()
pose_snapshot = {bone.name: bone.matrix_basis.copy() for bone in rig.pose.bones}


def restore_pose():
    for name, matrix in pose_snapshot.items():
        rig.pose.bones[name].matrix_basis = matrix
    bpy.context.view_layer.update()


def add_rotation(bone_name, axis, degrees):
    bone = rig.pose.bones.get(bone_name)
    if bone is None:
        raise RuntimeError("Missing pose bone: " + bone_name)
    bone.rotation_mode = "XYZ"
    bone.rotation_euler[axis] += math.radians(degrees)


outputs = {}
expression_tests = {
    "neutral": {},
    "blink": {"Blink.L": 1.0, "Blink.R": 1.0},
    "smile": {"Mouth_Smile": 1.0, "CheekRaise.L": 0.70, "CheekRaise.R": 0.70},
    "wide_eyes": {"Eye_Wide.L": 1.0, "Eye_Wide.R": 1.0},
    "mouth_open": {"JawOpen": 1.0, "Mouth_A": 0.80},
}
restore_pose()
for name, values in expression_tests.items():
    set_values(keys, values)
    outputs[name] = render_test(scene, camera, name, closeup=True)

pose_tests = ["head_turn", "shoulder_raise", "elbow_bend", "wrist_bend", "fingers_spread", "fist"]
for name in pose_tests:
    reset_expressions(keys)
    restore_pose()
    if name == "head_turn":
        add_rotation("Head", 1, 24.0)
    elif name == "shoulder_raise":
        add_rotation("LeftShoulder", 2, -24.0)
    elif name == "elbow_bend":
        add_rotation("LeftForeArm", 2, -68.0)
    elif name == "wrist_bend":
        add_rotation("LeftHand", 0, 28.0)
    elif name == "fingers_spread":
        add_rotation("Finger_01_Proximal.L", 2, -18.0)
        add_rotation("Finger_03_Proximal.L", 2, 18.0)
    elif name == "fist":
        for finger in (1, 2, 3):
            for segment in ("Proximal", "Middle", "Distal"):
                add_rotation("Finger_%02d_%s.L" % (finger, segment), 2, -48.0)
    bpy.context.view_layer.update()
    outputs[name] = render_test(scene, camera, name, closeup=False)

reset_expressions(keys)
restore_pose()
if rig.animation_data:
    rig.animation_data.action = active_action
scene.frame_set(scene.frame_current)
bpy.context.view_layer.update()

checks = {
    "main_vertex_count_preserved": len(main_obj.data.vertices) == 5478,
    "shape_key_order_exact": key_names == EXPECTED_KEYS,
    "modifier_order_preserved": modifier_order == ["ARMATURE", "CORRECTIVE_SMOOTH", "SUBSURF"],
    "uv_preserved": "UVMap" in uv_names,
    "single_armature": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]) == 1,
    "bone_count_preserved": len(rig.data.bones) == 52,
    "actions_preserved": len(bpy.data.actions) == 60,
    "weights_four_or_fewer": max_influences <= 4 and not over_four_vertices,
    "no_unweighted_main_vertices": zero_weight_vertices == 0,
    "formal_drivers_valid": len(drivers) == 8 and not invalid_drivers,
    "eye_layers_inside_cornea": all(item["sclera_inside_cornea_xz"] and item["front_layers_behind_cornea_front"] for item in eye_containment.values()),
    "legacy_fur_emitter_removed": bpy.data.objects.get("CXR_FurEmitter") is None,
    "all_tests_rendered": len(outputs) == 11 and all(os.path.exists(path) for path in outputs.values()),
}
if not all(checks.values()):
    raise RuntimeError("Gate 8 invariant failed: " + json.dumps(checks))

bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT)
report = {
    "gate": 8,
    "status": "PASS",
    "source_checkpoint": os.path.join(ROOT, "checkpoints", "sloth_050_materials.blend"),
    "result_checkpoint": CHECKPOINT,
    "shape_keys": key_names,
    "modifier_order": modifier_order,
    "uv_layers": uv_names,
    "weight_audit": {"max_influences": max_influences, "zero_weight_vertices": zero_weight_vertices, "over_four_vertices": over_four_vertices},
    "rig": {"armature": rig.name, "bones": len(rig.data.bones), "actions": len(bpy.data.actions), "object_constraints": len(rig.constraints), "pose_constraints": sum(len(bone.constraints) for bone in rig.pose.bones)},
    "driver_migration": {"legacy_gate0_driver_count": 36, "legacy_system": "removed CXR eye/catchlight presentation system", "formal_driver_count": len(drivers), "formal_system": "Blink and Wide drivers on upper/lower eyelid shape keys", "invalid": invalid_drivers},
    "eye_containment": eye_containment,
    "removed_placeholders": removed_placeholders,
    "checks": checks,
    "renders": outputs,
    "neutral_state_saved": True,
    "restored_active_action": active_action.name if active_action else None,
}
with open(REPORT, "w", encoding="utf-8") as handle:
    json.dump(report, handle, ensure_ascii=False, indent=2)
print(json.dumps({"status": "PASS", "checkpoint": CHECKPOINT, "report": REPORT, "checks": checks, "renders": outputs}, ensure_ascii=False))
