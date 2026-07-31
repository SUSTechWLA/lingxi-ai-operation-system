import bpy
import json
import math
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_085_eye_bugfix_candidate_v5.blend")
REPORT = os.path.join(ROOT, "reports", "eye_bugfix_candidate_v5.json")
RENDER_DIR = os.path.join(ROOT, "renders", "eye_bugfix", "candidate_v5")
os.makedirs(os.path.dirname(CHECKPOINT), exist_ok=True)
os.makedirs(os.path.dirname(REPORT), exist_ok=True)
os.makedirs(RENDER_DIR, exist_ok=True)

scene = bpy.context.scene
head = bpy.data.objects.get("GEO_HeadBody")
if not head or not head.data.shape_keys:
    raise RuntimeError("Formal head shape key system missing")

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


def opaque(material):
    if not material:
        return
    try:
        material.blend_method = "OPAQUE"
    except Exception:
        pass
    material.diffuse_color[3] = 1.0


for name in (
    "MAT_Eye_Pupil", "MAT_Eye_Iris_Amber", "MAT_Eye_Iris_Limbal",
    "MAT_Eye_Catchlight", "MAT_Eyelid_Skin_Brown", "MAT_TearLine",
):
    opaque(bpy.data.materials.get(name))

lid_mat = bpy.data.materials.get("MAT_Eyelid_Skin_Brown")
lid_bsdf = next((n for n in lid_mat.node_tree.nodes if n.type == "BSDF_PRINCIPLED"), None)
if lid_bsdf:
    lid_bsdf.inputs["Base Color"].default_value = (0.025, 0.0045, 0.0012, 1.0)
    lid_bsdf.inputs["Roughness"].default_value = 0.64

iris_mat = bpy.data.materials.get("MAT_Eye_Iris_Amber")
if iris_mat and iris_mat.use_nodes:
    ramp_node = next((n for n in iris_mat.node_tree.nodes if n.type == "VALTORGB"), None)
    if ramp_node:
        ramp_node.color_ramp.elements[0].color = (0.006, 0.0010, 0.0002, 1.0)
        ramp_node.color_ramp.elements[1].color = (0.19, 0.038, 0.0015, 1.0)

cornea_mat = bpy.data.materials.get("MAT_Eye_Cornea")
if not cornea_mat or not cornea_mat.use_nodes:
    raise RuntimeError("Cornea material missing")
try:
    cornea_mat.blend_method = "BLEND"
except Exception:
    pass
cornea_mat.diffuse_color = (0.92, 0.97, 1.0, 0.065)
cornea_bsdf = next((n for n in cornea_mat.node_tree.nodes if n.type == "BSDF_PRINCIPLED"), None)
if not cornea_bsdf:
    raise RuntimeError("Cornea Principled node missing")
cornea_bsdf.inputs["Base Color"].default_value = (0.92, 0.97, 1.0, 1.0)
cornea_bsdf.inputs["Roughness"].default_value = 0.055
cornea_bsdf.inputs["Alpha"].default_value = 0.065
transmission = cornea_bsdf.inputs.get("Transmission Weight") or cornea_bsdf.inputs.get("Transmission")
if transmission:
    transmission.default_value = 0.18
specular = cornea_bsdf.inputs.get("Specular IOR Level") or cornea_bsdf.inputs.get("Specular")
if specular:
    specular.default_value = 0.46
coat = cornea_bsdf.inputs.get("Coat Weight") or cornea_bsdf.inputs.get("Clearcoat")
if coat:
    coat.default_value = 0.20

EYE_CZ = 2.198
OUTER_RX = 0.0660
OUTER_RZ = 0.0780
INNER_RX = 0.0615
CENTERS = {"L": 0.125, "R": -0.125}
COLS = 48
ROWS = 6


def lid_coords(side, part, pose):
    cx = CENTERS[side]
    coords = []
    for col in range(COLS):
        u = -1.0 + 2.0 * col / (COLS - 1)
        s = math.sqrt(max(0.0, 1.0 - u * u))
        tapered_outer_rx = INNER_RX + (OUTER_RX - INNER_RX) * s
        outer_x = cx + tapered_outer_rx * u
        inner_x = cx + INNER_RX * u

        if part == "Upper":
            outer_z = EYE_CZ + OUTER_RZ * s
            if pose == "Basis":
                inner_z = EYE_CZ + 0.037 * s
            elif pose == "Blink":
                inner_z = EYE_CZ - 0.036 * s - 0.003 * s * s
            else:
                inner_z = EYE_CZ + 0.052 * s
        else:
            outer_z = EYE_CZ - OUTER_RZ * s
            if pose == "Basis":
                inner_z = EYE_CZ - 0.058 * s
            elif pose == "Blink":
                inner_z = EYE_CZ - 0.036 * s - 0.003 * s * s
            else:
                inner_z = EYE_CZ - 0.064 * s

        inner_y = -0.2295 + 0.0020 * u * u
        raw_outer_y = -0.2180 - 0.0010 * s
        outer_y = inner_y + (raw_outer_y - inner_y) * s

        for row in range(ROWS):
            t = row / (ROWS - 1)
            smooth_t = t * t * (3.0 - 2.0 * t)
            x = outer_x * (1.0 - smooth_t) + inner_x * smooth_t
            z = outer_z * (1.0 - smooth_t) + inner_z * smooth_t
            y = outer_y * (1.0 - smooth_t) + inner_y * smooth_t
            y -= 0.0032 * math.sin(math.pi * t) * s
            coords.append((x, y, z))
    return coords


for side in ("L", "R"):
    for part in ("Upper", "Lower"):
        obj = bpy.data.objects.get(f"EYE_Lid{part}_{side}")
        if not obj or len(obj.data.vertices) != COLS * ROWS or not obj.data.shape_keys:
            raise RuntimeError(f"Candidate v4 lid missing: {side} {part}")
        inv = obj.matrix_world.inverted()
        for key_name in ("Basis", "Blink", "Wide"):
            kb = obj.data.shape_keys.key_blocks[key_name]
            for index, world_co in enumerate(lid_coords(side, part, key_name)):
                local = inv @ Vector(world_co)
                kb.data[index].co = local
                if key_name == "Basis":
                    obj.data.vertices[index].co = local
        obj.data.update()

for side in ("L", "R"):
    tear = bpy.data.objects.get(f"EYE_TearLine_{side}")
    if tear and tear.type == "CURVE":
        tear.data.bevel_depth = 0.00010

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
render_view("smile_front_34.png", (1.35, -3.35, 2.23))

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
    "shape_key_names_before": shape_key_names_before,
    "shape_key_names_after": [kb.name for kb in head.data.shape_keys.key_blocks],
    "lid_driver_count": lid_driver_count,
    "same_lid_material": same_material,
    "cornea_alpha": cornea_bsdf.inputs["Alpha"].default_value,
    "cornea_transmission": transmission.default_value if transmission else None,
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
    raise RuntimeError("Candidate v5 invariants failed")
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
