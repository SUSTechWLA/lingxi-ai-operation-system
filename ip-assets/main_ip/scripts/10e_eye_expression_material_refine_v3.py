import bpy
import json
import math
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_083_eye_bugfix_candidate_v3.blend")
REPORT = os.path.join(ROOT, "reports", "eye_bugfix_candidate_v3.json")
RENDER_DIR = os.path.join(ROOT, "renders", "eye_bugfix", "candidate_v3")
os.makedirs(os.path.dirname(CHECKPOINT), exist_ok=True)
os.makedirs(os.path.dirname(REPORT), exist_ok=True)
os.makedirs(RENDER_DIR, exist_ok=True)

scene = bpy.context.scene
head = bpy.data.objects.get("GEO_HeadBody")
rig = bpy.data.objects.get("RIG_Sloth")
formal_collection = bpy.data.collections.get("COL_CHR_SLOTH_FINAL")
if not head or not rig:
    raise RuntimeError("Formal head/rig missing")

head_vertex_count_before = len(head.data.vertices)
shape_key_names_before = [kb.name for kb in head.data.shape_keys.key_blocks]

for obj in list(bpy.data.objects):
    if obj.name.startswith("TMP_EyeBug"):
        data = obj.data
        bpy.data.objects.remove(obj, do_unlink=True)
        if data and data.users == 0:
            if isinstance(data, bpy.types.Camera):
                bpy.data.cameras.remove(data)
            elif isinstance(data, bpy.types.Light):
                bpy.data.lights.remove(data)

bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT, copy=False)


def principled_node(material):
    return next((n for n in material.node_tree.nodes if n.type == "BSDF_PRINCIPLED"), None)


def set_bsdf(material, color, roughness, specular):
    bsdf = principled_node(material)
    if not bsdf:
        raise RuntimeError(f"Principled node missing in {material.name}")
    bsdf.inputs["Base Color"].default_value = color
    bsdf.inputs["Roughness"].default_value = roughness
    spec = bsdf.inputs.get("Specular IOR Level") or bsdf.inputs.get("Specular")
    if spec:
        spec.default_value = specular


lid_mat = bpy.data.materials.get("MAT_Eyelid_Skin_Brown")
pupil_mat = bpy.data.materials.get("MAT_Eye_Pupil")
tear_mat = bpy.data.materials.get("MAT_TearLine")
if not lid_mat or not pupil_mat or not tear_mat:
    raise RuntimeError("Required eye materials missing")

set_bsdf(lid_mat, (0.052, 0.010, 0.0028, 1.0), 0.62, 0.18)
set_bsdf(pupil_mat, (0.00025, 0.00008, 0.00003, 1.0), 0.30, 0.04)
set_bsdf(tear_mat, (0.030, 0.008, 0.003, 1.0), 0.34, 0.20)

EYE_CZ = 2.198
EYE_RX = 0.0615
EYE_RZ = 0.074
CENTERS = {"L": 0.125, "R": -0.125}


def lid_world_pairs(side, part, pose):
    cx = CENTERS[side]
    coords = []
    for i in range(40):
        u = -1.0 + 2.0 * i / 39.0
        s = math.sqrt(max(0.0, 1.0 - u * u))
        x = cx + EYE_RX * u
        if part == "Upper":
            outer_z = EYE_CZ + EYE_RZ * s
            if pose == "Basis":
                inner_z = EYE_CZ + 0.040 * s
            elif pose == "Blink":
                inner_z = EYE_CZ - 0.040 * s - 0.0015 * s * s
            else:
                inner_z = EYE_CZ + 0.053 * s
        else:
            outer_z = EYE_CZ - EYE_RZ * s
            if pose == "Basis":
                inner_z = EYE_CZ - 0.052 * s
            elif pose == "Blink":
                inner_z = EYE_CZ - 0.040 * s - 0.0015 * s * s
            else:
                inner_z = EYE_CZ - 0.058 * s

        outer_y = -0.2140 - 0.0015 * s
        inner_y = -0.2285 + 0.0030 * u * u
        coords.extend((Vector((x, outer_y, outer_z)), Vector((x, inner_y, inner_z))))
    return coords


for side in ("L", "R"):
    for part in ("Upper", "Lower"):
        obj = bpy.data.objects.get(f"EYE_Lid{part}_{side}")
        if not obj or len(obj.data.vertices) != 80 or not obj.data.shape_keys:
            raise RuntimeError(f"Unexpected lid topology on {side} {part}")
        obj.data.materials.clear()
        obj.data.materials.append(lid_mat)
        inv = obj.matrix_world.inverted()
        for key_name in ("Basis", "Blink", "Wide"):
            kb = obj.data.shape_keys.key_blocks[key_name]
            for index, world_co in enumerate(lid_world_pairs(side, part, key_name)):
                local = inv @ world_co
                kb.data[index].co = local
                if key_name == "Basis":
                    obj.data.vertices[index].co = local
        obj.data.update()


def remove_tearline(name):
    obj = bpy.data.objects.get(name)
    if not obj:
        return
    data = obj.data
    bpy.data.objects.remove(obj, do_unlink=True)
    if data and data.users == 0:
        bpy.data.curves.remove(data)


def parent_to_head_keep_world(obj):
    world = obj.matrix_world.copy()
    obj.parent = rig
    obj.parent_type = "BONE"
    obj.parent_bone = "Head"
    obj.matrix_world = world


for side, cx in CENTERS.items():
    name = f"EYE_TearLine_{side}"
    remove_tearline(name)
    curve = bpy.data.curves.new(name + "_Curve", "CURVE")
    curve.dimensions = "3D"
    curve.resolution_u = 2
    curve.bevel_depth = 0.00032
    curve.bevel_resolution = 1
    curve.materials.append(tear_mat)
    spline = curve.splines.new("BEZIER")
    count = 20
    spline.bezier_points.add(count - 1)
    for i, bp in enumerate(spline.bezier_points):
        u = -0.84 + 1.68 * i / (count - 1)
        s = math.sqrt(max(0.0, 1.0 - u * u))
        bp.co = (cx + EYE_RX * u, -0.2290 + 0.0030 * u * u, EYE_CZ - 0.052 * s)
        bp.handle_left_type = "AUTO"
        bp.handle_right_type = "AUTO"
    obj = bpy.data.objects.new(name, curve)
    (formal_collection or scene.collection).objects.link(obj)
    parent_to_head_keep_world(obj)

# Deterministic neutral state.
for kb in head.data.shape_keys.key_blocks:
    if kb.name != "Basis":
        kb.value = 0.0
rest = head.data.shape_keys.key_blocks.get("Mouth_Rest")
if rest:
    rest.value = 1.0
bpy.context.view_layer.update()


def point_at(obj, target):
    obj.rotation_euler = (Vector(target) - obj.location).to_track_quat("-Z", "Y").to_euler()


old_camera = scene.camera
old_engine = scene.render.engine
old_res = (scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage)
old_path = scene.render.filepath
old_format = scene.render.image_settings.file_format
old_film = scene.render.film_transparent
old_world_color = tuple(scene.world.color) if scene.world else None

cam_data = bpy.data.cameras.new("TMP_EyeBugCandidateCamera")
cam = bpy.data.objects.new("TMP_EyeBugCandidateCamera", cam_data)
scene.collection.objects.link(cam)
cam.data.sensor_width = 36.0
scene.camera = cam
temp_objects = [cam]
for name, location, energy, size in (
    ("TMP_EyeBugCandidate_Key", (-2.2, -3.0, 4.0), 700.0, 2.0),
    ("TMP_EyeBugCandidate_Fill", (2.4, -2.4, 3.0), 430.0, 2.5),
    ("TMP_EyeBugCandidate_Rim", (1.5, 1.8, 3.5), 560.0, 1.7),
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

scene.render.engine = "BLENDER_EEVEE"
scene.render.resolution_x = 720
scene.render.resolution_y = 720
scene.render.resolution_percentage = 100
scene.render.image_settings.file_format = "PNG"
scene.render.film_transparent = False
if scene.world:
    scene.world.color = (0.055, 0.055, 0.055)


def render_view(filename, location, lens=100.0):
    cam.location = location
    cam.data.lens = lens
    point_at(cam, (0.0, 0.0, 2.18))
    scene.render.filepath = os.path.join(RENDER_DIR, filename)
    bpy.ops.render.render(write_still=True)


render_view("neutral_front.png", (0.0, -3.35, 2.20))
render_view("neutral_front_34.png", (2.05, -3.15, 2.25), lens=95.0)
render_view("neutral_side.png", (3.55, -0.02, 2.22))

for name in ("Blink.L", "Blink.R"):
    head.data.shape_keys.key_blocks[name].value = 1.0
bpy.context.view_layer.update()
render_view("blink_front.png", (0.0, -3.35, 2.20))

for name in ("Blink.L", "Blink.R"):
    head.data.shape_keys.key_blocks[name].value = 0.0
head.data.shape_keys.key_blocks["Mouth_Smile"].value = 0.65
head.data.shape_keys.key_blocks["CheekRaise.L"].value = 0.45
head.data.shape_keys.key_blocks["CheekRaise.R"].value = 0.45
bpy.context.view_layer.update()
render_view("smile_front_34.png", (1.35, -3.35, 2.23), lens=100.0)

for kb in head.data.shape_keys.key_blocks:
    if kb.name != "Basis":
        kb.value = 0.0
if rest:
    rest.value = 1.0
bpy.context.view_layer.update()

scene.camera = old_camera
scene.render.engine = old_engine
scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage = old_res
scene.render.filepath = old_path
scene.render.image_settings.file_format = old_format
scene.render.film_transparent = old_film
if scene.world and old_world_color:
    scene.world.color = old_world_color

for obj in temp_objects:
    data = obj.data
    bpy.data.objects.remove(obj, do_unlink=True)
    if data and data.users == 0:
        if isinstance(data, bpy.types.Camera):
            bpy.data.cameras.remove(data)
        elif isinstance(data, bpy.types.Light):
            bpy.data.lights.remove(data)

lid_driver_count = sum(
    len(obj.data.shape_keys.animation_data.drivers)
    for obj in bpy.data.objects
    if obj.name.startswith(("EYE_LidUpper_", "EYE_LidLower_"))
    and obj.data.shape_keys and obj.data.shape_keys.animation_data
)
same_material = all(
    bpy.data.objects[f"EYE_LidUpper_{side}"].material_slots[0].material
    is bpy.data.objects[f"EYE_LidLower_{side}"].material_slots[0].material
    for side in ("L", "R")
)
report = {
    "status": "PASS",
    "checkpoint": CHECKPOINT,
    "head_vertex_count_before": head_vertex_count_before,
    "head_vertex_count_after": len(head.data.vertices),
    "shape_keys_before": shape_key_names_before,
    "shape_keys_after": [kb.name for kb in head.data.shape_keys.key_blocks],
    "lid_driver_count": lid_driver_count,
    "same_lid_material": same_material,
    "renders": [os.path.join(RENDER_DIR, name) for name in (
        "neutral_front.png", "neutral_front_34.png", "neutral_side.png",
        "blink_front.png", "smile_front_34.png"
    )],
}
report["invariants_pass"] = bool(
    head_vertex_count_before == len(head.data.vertices)
    and shape_key_names_before == [kb.name for kb in head.data.shape_keys.key_blocks]
    and lid_driver_count == 8 and same_material
)
if not report["invariants_pass"]:
    raise RuntimeError("Candidate v3 invariants failed")
with open(REPORT, "w", encoding="utf-8") as f:
    json.dump(report, f, ensure_ascii=False, indent=2)

bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT, copy=False)
print(json.dumps({
    "status": "PASS",
    "checkpoint": CHECKPOINT,
    "report": REPORT,
    "invariants_pass": report["invariants_pass"],
    "lid_driver_count": lid_driver_count,
    "head_vertex_count": len(head.data.vertices),
    "renders": report["renders"],
}, ensure_ascii=False))
