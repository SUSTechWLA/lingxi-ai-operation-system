import bpy
import json
import math
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
PRE_CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_094_reference_eye_final_candidate.blend")
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_095_reference_face_form.blend")
REPORT = os.path.join(ROOT, "reports", "reference_face_form_refine.json")
RENDER_DIR = os.path.join(ROOT, "renders", "reference_refine", "face_form")


def clamp(value, minimum=0.0, maximum=1.0):
    return max(minimum, min(maximum, value))


def smoothstep(edge0, edge1, value):
    if edge0 == edge1:
        return 0.0
    t = clamp((value - edge0) / (edge1 - edge0))
    return t * t * (3.0 - 2.0 * t)


def band(value, low0, low1, high1, high0):
    return smoothstep(low0, low1, value) * (1.0 - smoothstep(high1, high0, value))


def point_at(obj, target):
    obj.rotation_euler = (Vector(target) - obj.location).to_track_quat("-Z", "Y").to_euler()


def displacement(coordinate):
    x, y, z = coordinate
    front = 1.0 - smoothstep(-0.10, 0.02, y)
    if front <= 0.0 or z < 1.88 or z > 2.42:
        return Vector((0.0, 0.0, 0.0))

    absolute_x = abs(x)
    eye_distance = ((absolute_x - 0.125) / 0.095) ** 2 + ((z - 2.205) / 0.085) ** 2
    eye_protection = clamp(eye_distance / 1.0)

    muzzle_x = 1.0 - smoothstep(0.19, 0.34, absolute_x)
    muzzle_z = band(z, 1.93, 1.99, 2.19, 2.24)
    muzzle_y = front * muzzle_x * muzzle_z * (0.30 + 0.70 * eye_protection)
    muzzle_focus = math.exp(-((absolute_x / 0.22) ** 2 + ((z - 2.075) / 0.11) ** 2))

    cheek_x = band(absolute_x, 0.12, 0.18, 0.34, 0.40)
    cheek_z = band(z, 1.99, 2.05, 2.29, 2.35)
    cheek = front * cheek_x * cheek_z * (0.25 + 0.75 * eye_protection)

    chin_x = 1.0 - smoothstep(0.18, 0.30, absolute_x)
    chin_z = band(z, 1.88, 1.94, 2.05, 2.11)
    chin = front * chin_x * chin_z

    direction_x = 0.0 if absolute_x < 1.0e-8 else (1.0 if x > 0.0 else -1.0)
    delta_x = direction_x * (0.018 * cheek + 0.006 * muzzle_y)
    delta_y = -(0.020 * muzzle_y + 0.008 * muzzle_focus + 0.014 * cheek + 0.010 * chin)
    delta_z = 0.006 * cheek - 0.004 * chin
    return Vector((delta_x, delta_y, delta_z))


def render_reviews(scene, head):
    old_camera = scene.camera
    old_engine = scene.render.engine
    old_resolution = (scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage)
    old_path = scene.render.filepath
    old_format = scene.render.image_settings.file_format
    old_values = {key.name: key.value for key in head.data.shape_keys.key_blocks}
    camera_data = bpy.data.cameras.new("TMP_ReferenceFaceCamera")
    camera = bpy.data.objects.new("TMP_ReferenceFaceCamera", camera_data)
    scene.collection.objects.link(camera)
    camera.data.sensor_width = 36.0
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
        point_at(camera, (0.0, -0.02, 2.18))
        scene.render.filepath = os.path.join(RENDER_DIR, name)
        bpy.ops.render.render(write_still=True)

    bpy.context.view_layer.update()
    render("neutral_front.png", (0.0, -3.35, 2.20))
    render("neutral_front_34.png", (1.65, -3.15, 2.25), 95.0)
    render("strict_side.png", (3.55, -0.02, 2.22))
    head.data.shape_keys.key_blocks["Mouth_Smile"].value = 0.72
    head.data.shape_keys.key_blocks["CheekRaise.L"].value = 0.45
    head.data.shape_keys.key_blocks["CheekRaise.R"].value = 0.45
    bpy.context.view_layer.update()
    render("smile_front_34.png", (1.35, -3.25, 2.22), 95.0)
    head.data.shape_keys.key_blocks["Mouth_Smile"].value = 0.0
    head.data.shape_keys.key_blocks["CheekRaise.L"].value = 0.0
    head.data.shape_keys.key_blocks["CheekRaise.R"].value = 0.0
    head.data.shape_keys.key_blocks["JawOpen"].value = 0.78
    head.data.shape_keys.key_blocks["Mouth_A"].value = 0.62
    bpy.context.view_layer.update()
    render("mouth_open_front.png", (0.0, -3.35, 2.20))

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
    os.makedirs(os.path.dirname(CHECKPOINT), exist_ok=True)
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    os.makedirs(RENDER_DIR, exist_ok=True)
    if not os.path.exists(PRE_CHECKPOINT):
        raise RuntimeError("Eye-final recovery checkpoint missing")
    head = bpy.data.objects.get("GEO_HeadBody")
    if not head or len(head.data.vertices) != 5478 or not head.data.shape_keys:
        raise RuntimeError("Formal head invariant missing")

    shape_names = [key.name for key in head.data.shape_keys.key_blocks]
    action_names = sorted(action.name for action in bpy.data.actions)
    sample_indices = list(range(0, len(head.data.vertices), 37))
    expression_deltas_before = {
        key.name: [tuple(key.data[index].co - head.data.shape_keys.key_blocks["Basis"].data[index].co) for index in sample_indices]
        for key in head.data.shape_keys.key_blocks if key.name != "Basis"
    }

    applied = not bool(head.get("reference_muzzle_cheek_applied", False))
    affected = 0
    maximum_delta = 0.0
    if applied:
        basis = head.data.shape_keys.key_blocks["Basis"]
        deltas = [displacement(vertex.co) for vertex in basis.data]
        for delta in deltas:
            if delta.length > 1.0e-8:
                affected += 1
                maximum_delta = max(maximum_delta, delta.length)
        for key in head.data.shape_keys.key_blocks:
            for index, delta in enumerate(deltas):
                if delta.length > 1.0e-8:
                    key.data[index].co += delta
        head.data.update()
        head["reference_muzzle_cheek_applied"] = True
        head["reference_muzzle_cheek_max_delta"] = maximum_delta

    maximum_expression_drift = 0.0
    basis = head.data.shape_keys.key_blocks["Basis"]
    for key in head.data.shape_keys.key_blocks:
        if key.name == "Basis":
            continue
        for sample_position, index in enumerate(sample_indices):
            before = Vector(expression_deltas_before[key.name][sample_position])
            after = key.data[index].co - basis.data[index].co
            maximum_expression_drift = max(maximum_expression_drift, (after - before).length)

    render_reviews(bpy.context.scene, head)
    checks = {
        "head_vertex_count": len(head.data.vertices) == 5478,
        "shape_key_order": [key.name for key in head.data.shape_keys.key_blocks] == shape_names,
        "actions_unchanged": sorted(action.name for action in bpy.data.actions) == action_names,
        "expression_delta_preserved": maximum_expression_drift < 1.0e-6,
        "maximum_delta_safe": maximum_delta <= 0.040,
        "no_temp_objects": not any(obj.name.startswith("TMP_") for obj in bpy.data.objects),
    }
    failed = [name for name, value in checks.items() if not value]
    report = {
        "schema": "sloth_reference_face_form_v1",
        "status": "PASS" if not failed else "FAIL",
        "checkpoint": CHECKPOINT,
        "applied_this_run": applied,
        "affected_vertices": affected,
        "maximum_delta": maximum_delta,
        "maximum_expression_delta_drift": maximum_expression_drift,
        "checks": checks,
        "failed": failed,
        "renders": sorted(os.path.join(RENDER_DIR, name) for name in os.listdir(RENDER_DIR) if name.endswith(".png")),
    }
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    if failed:
        raise RuntimeError("Reference face form validation failed: " + ", ".join(failed))
    bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT, copy=True)
    print(json.dumps({"status": report["status"], "checkpoint": CHECKPOINT, "report": REPORT, "applied": applied, "affected_vertices": affected, "maximum_delta": maximum_delta, "maximum_expression_delta_drift": maximum_expression_drift, "checks": checks, "renders": report["renders"]}, ensure_ascii=False))


if __name__ == "__main__":
    main()
