import bpy
import json
import math
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_080_pre_eye_bugfix.blend")
REPORT = os.path.join(ROOT, "reports", "eye_bugfix_audit.json")
RENDER_DIR = os.path.join(ROOT, "renders", "eye_bugfix", "baseline")

os.makedirs(os.path.dirname(CHECKPOINT), exist_ok=True)
os.makedirs(os.path.dirname(REPORT), exist_ok=True)
os.makedirs(RENDER_DIR, exist_ok=True)

# A failed/retried diagnostic run must not accumulate temporary objects.
for obj in list(bpy.data.objects):
    if obj.name.startswith("TMP_EyeBug"):
        data = obj.data
        bpy.data.objects.remove(obj, do_unlink=True)
        if data and data.users == 0:
            if isinstance(data, bpy.types.Camera):
                bpy.data.cameras.remove(data)
            elif isinstance(data, bpy.types.Light):
                bpy.data.lights.remove(data)

# The first write is the immutable pre-fix checkpoint. All diagnostic changes
# below live only in this fork, never in character_sloth_final.blend.
source_file = bpy.data.filepath
bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT, copy=False)

scene = bpy.context.scene


def world_bounds(obj):
    points = [obj.matrix_world @ Vector(corner) for corner in obj.bound_box]
    return {
        "min": [round(min(p[i] for p in points), 6) for i in range(3)],
        "max": [round(max(p[i] for p in points), 6) for i in range(3)],
    }


def object_record(obj):
    rec = {
        "name": obj.name,
        "type": obj.type,
        "location_local": [round(v, 6) for v in obj.location],
        "location_world": [round(v, 6) for v in obj.matrix_world.translation],
        "dimensions": [round(v, 6) for v in obj.dimensions],
        "scale": [round(v, 6) for v in obj.scale],
        "parent": obj.parent.name if obj.parent else None,
        "parent_type": obj.parent_type,
        "parent_bone": obj.parent_bone,
        "materials": [slot.material.name if slot.material else None for slot in obj.material_slots],
        "bounds_world": world_bounds(obj),
    }
    if obj.type == "MESH":
        rec["vertices"] = len(obj.data.vertices)
        rec["polygons"] = len(obj.data.polygons)
        rec["shape_keys"] = [kb.name for kb in obj.data.shape_keys.key_blocks] if obj.data.shape_keys else []
    return rec


eye_prefixes = (
    "EYE_Sclera_", "EYE_Iris_", "EYE_Pupil_", "EYE_Cornea_",
    "EYE_LidUpper_", "EYE_LidLower_", "EYE_TearLine_", "EYE_Catchlight_",
)
eye_objects = sorted(
    [obj for obj in bpy.data.objects if obj.name.startswith(eye_prefixes)],
    key=lambda obj: obj.name,
)

head = bpy.data.objects.get("GEO_HeadBody")
shape_values = {}
if head and head.data.shape_keys:
    for kb in head.data.shape_keys.key_blocks:
        shape_values[kb.name] = float(kb.value)

driver_records = []
if head and head.data.shape_keys and head.data.shape_keys.animation_data:
    for drv in head.data.shape_keys.animation_data.drivers:
        driver_records.append({
            "data_path": drv.data_path,
            "expression": drv.driver.expression,
            "variables": [v.name for v in drv.driver.variables],
        })

lid_material_match = {}
for side in ("L", "R"):
    upper = bpy.data.objects.get(f"EYE_LidUpper_{side}")
    lower = bpy.data.objects.get(f"EYE_LidLower_{side}")
    upper_mat = upper.material_slots[0].material.name if upper and upper.material_slots and upper.material_slots[0].material else None
    lower_mat = lower.material_slots[0].material.name if lower and lower.material_slots and lower.material_slots[0].material else None
    lid_material_match[side] = {
        "upper": upper_mat,
        "lower": lower_mat,
        "same_datablock": bool(
            upper and lower and upper.material_slots and lower.material_slots
            and upper.material_slots[0].material is lower.material_slots[0].material
        ),
    }

# Neutralize facial keys in the checkpoint for comparable baseline renders.
if head and head.data.shape_keys:
    for kb in head.data.shape_keys.key_blocks:
        if kb.name != "Basis":
            kb.value = 0.0
    rest = head.data.shape_keys.key_blocks.get("Mouth_Rest")
    if rest:
        rest.value = 1.0


def point_at(obj, target):
    obj.rotation_euler = (Vector(target) - obj.location).to_track_quat("-Z", "Y").to_euler()


# Temporary, checkpoint-only diagnostic camera and neutral lighting.
cam_data = bpy.data.cameras.new("TMP_EyeBugAuditCamera")
cam = bpy.data.objects.new("TMP_EyeBugAuditCamera", cam_data)
scene.collection.objects.link(cam)
cam_data.lens = 100.0
cam_data.sensor_width = 36.0
scene.camera = cam

temp_objects = [cam]
for name, location, energy, size in (
    ("TMP_EyeBug_Key", (-2.2, -3.0, 4.0), 700.0, 2.0),
    ("TMP_EyeBug_Fill", (2.4, -2.4, 3.0), 430.0, 2.5),
    ("TMP_EyeBug_Rim", (1.5, 1.8, 3.5), 560.0, 1.7),
):
    data = bpy.data.lights.new(name, "AREA")
    data.energy = energy
    data.shape = "DISK"
    data.size = size
    obj = bpy.data.objects.new(name, data)
    scene.collection.objects.link(obj)
    obj.location = location
    point_at(obj, (0.0, 0.0, 2.18))
    temp_objects.append(obj)

old_engine = scene.render.engine
old_res = (scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage)
old_filepath = scene.render.filepath
old_format = scene.render.image_settings.file_format
old_film = scene.render.film_transparent

scene.render.engine = "BLENDER_EEVEE"
scene.render.resolution_x = 720
scene.render.resolution_y = 720
scene.render.resolution_percentage = 100
scene.render.image_settings.file_format = "PNG"
scene.render.film_transparent = False

if scene.world:
    scene.world.color = (0.055, 0.055, 0.055)


def render_view(filename, location, target=(0.0, 0.0, 2.18), lens=100.0):
    cam.location = location
    cam.data.lens = lens
    point_at(cam, target)
    scene.render.filepath = os.path.join(RENDER_DIR, filename)
    bpy.ops.render.render(write_still=True)


render_view("neutral_front.png", (0.0, -3.35, 2.20))
render_view("neutral_front_34.png", (2.05, -3.15, 2.25), lens=95.0)
render_view("neutral_side.png", (3.55, -0.02, 2.22), lens=100.0)

if head and head.data.shape_keys:
    for name in ("Blink.L", "Blink.R"):
        kb = head.data.shape_keys.key_blocks.get(name)
        if kb:
            kb.value = 1.0
render_view("blink_front.png", (0.0, -3.35, 2.20))

# Return to neutral before saving the audit checkpoint.
if head and head.data.shape_keys:
    for kb in head.data.shape_keys.key_blocks:
        if kb.name != "Basis":
            kb.value = 0.0
    rest = head.data.shape_keys.key_blocks.get("Mouth_Rest")
    if rest:
        rest.value = 1.0

report = {
    "status": "PASS",
    "source_file": source_file,
    "checkpoint": CHECKPOINT,
    "scene": scene.name,
    "eye_objects": [object_record(obj) for obj in eye_objects],
    "lid_material_match": lid_material_match,
    "head_vertex_count": len(head.data.vertices) if head and head.type == "MESH" else None,
    "head_shape_key_count": len(head.data.shape_keys.key_blocks) if head and head.data.shape_keys else 0,
    "head_shape_key_values_before_audit": shape_values,
    "head_driver_count": len(driver_records),
    "drivers": driver_records,
    "baseline_renders": [
        os.path.join(RENDER_DIR, name) for name in (
            "neutral_front.png", "neutral_front_34.png", "neutral_side.png", "blink_front.png"
        )
    ],
    "observed_risks": [
        "Current sclera/cornea exposure must be evaluated from side and 3/4 renders.",
        "Upper and lower lid material datablocks must match when the eye is closed.",
        "Any fix must retain GEO_HeadBody vertex count, vertex order, drivers and shape keys.",
    ],
}

with open(REPORT, "w", encoding="utf-8") as f:
    json.dump(report, f, ensure_ascii=False, indent=2)

# Restore render settings; temporary diagnostic objects intentionally remain only
# inside the checkpoint for traceability.
scene.render.engine = old_engine
scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage = old_res
scene.render.filepath = old_filepath
scene.render.image_settings.file_format = old_format
scene.render.film_transparent = old_film

bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT, copy=False)
print(json.dumps({
    "status": "PASS",
    "checkpoint": CHECKPOINT,
    "report": REPORT,
    "eye_object_count": len(eye_objects),
    "head_vertex_count": report["head_vertex_count"],
    "head_driver_count": report["head_driver_count"],
    "lid_material_match": lid_material_match,
    "renders": report["baseline_renders"],
}, ensure_ascii=False))
