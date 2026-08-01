import bpy
import json
import math
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_084_eye_bugfix_candidate_v4.blend")
REPORT = os.path.join(ROOT, "reports", "eye_bugfix_candidate_v4.json")
RENDER_DIR = os.path.join(ROOT, "renders", "eye_bugfix", "candidate_v4")
os.makedirs(os.path.dirname(CHECKPOINT), exist_ok=True)
os.makedirs(os.path.dirname(REPORT), exist_ok=True)
os.makedirs(RENDER_DIR, exist_ok=True)

# If a previous attempt stopped after deleting/recreating only part of the lid
# set, reload the clean on-disk stage checkpoint before doing any work.
expected_lids = [
    f"EYE_Lid{part}_{side}"
    for side in ("L", "R") for part in ("Upper", "Lower")
]
partial_retry = (
    os.path.abspath(bpy.data.filepath) == os.path.abspath(CHECKPOINT)
    and any(
        bpy.data.objects.get(name) is None
        or len(bpy.data.objects[name].data.vertices) != 80
        for name in expected_lids
    )
)
if partial_retry:
    bpy.ops.wm.open_mainfile(filepath=CHECKPOINT)

scene = bpy.context.scene
head = bpy.data.objects.get("GEO_HeadBody")
rig = bpy.data.objects.get("RIG_Sloth")
formal_collection = bpy.data.collections.get("COL_CHR_SLOTH_FINAL")
if not head or not rig or not formal_collection:
    raise RuntimeError("Formal character structure missing")

head_vertex_count_before = len(head.data.vertices)
head_shape_names_before = [kb.name for kb in head.data.shape_keys.key_blocks]
old_lid_vertex_counts = {
    obj.name: len(obj.data.vertices)
    for obj in bpy.data.objects
    if obj.name.startswith(("EYE_LidUpper_", "EYE_LidLower_"))
}

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

lid_mat = bpy.data.materials.get("MAT_Eyelid_Skin_Brown")
pupil_mat = bpy.data.materials.get("MAT_Eye_Pupil")
if not lid_mat or not pupil_mat:
    raise RuntimeError("Eye materials missing")

# Force the pupil to remain deep and readable under neutral studio light.
pupil_mat.use_nodes = True
nt = pupil_mat.node_tree
nt.nodes.clear()
out = nt.nodes.new("ShaderNodeOutputMaterial")
diffuse = nt.nodes.new("ShaderNodeBsdfDiffuse")
diffuse.inputs["Color"].default_value = (0.00035, 0.00010, 0.00004, 1.0)
diffuse.inputs["Roughness"].default_value = 0.55
nt.links.new(diffuse.outputs["BSDF"], out.inputs["Surface"])

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

        outer_x = cx + OUTER_RX * u
        inner_x = cx + INNER_RX * u
        outer_y = -0.2110 - 0.0015 * s
        inner_y = -0.2290 + 0.0025 * u * u

        for row in range(ROWS):
            t = row / (ROWS - 1)
            smooth_t = t * t * (3.0 - 2.0 * t)
            x = outer_x * (1.0 - smooth_t) + inner_x * smooth_t
            z = outer_z * (1.0 - smooth_t) + inner_z * smooth_t
            y = outer_y * (1.0 - smooth_t) + inner_y * smooth_t
            y -= 0.0040 * math.sin(math.pi * t) * s
            coords.append((x, y, z))
    return coords


faces = []
for col in range(COLS - 1):
    for row in range(ROWS - 1):
        a = col * ROWS + row
        b = (col + 1) * ROWS + row
        c = (col + 1) * ROWS + row + 1
        d = col * ROWS + row + 1
        faces.append((a, d, c, b))


def remove_mesh_object(name):
    obj = bpy.data.objects.get(name)
    if not obj:
        return
    data = obj.data
    bpy.data.objects.remove(obj, do_unlink=True)
    if data and data.users == 0:
        bpy.data.meshes.remove(data)


def parent_to_head_keep_world(obj):
    world = obj.matrix_world.copy()
    obj.parent = rig
    obj.parent_type = "BONE"
    obj.parent_bone = "Head"
    obj.matrix_world = world


def add_shape_driver(key_block, source_name):
    fcurve = key_block.driver_add("value")
    driver = fcurve.driver
    driver.type = "SUM"
    var = driver.variables.new()
    var.name = "ctrl"
    var.type = "SINGLE_PROP"
    target = var.targets[0]
    target.id_type = "KEY"
    target.id = head.data.shape_keys
    target.data_path = f'key_blocks["{source_name}"].value'


for side in ("L", "R"):
    for part in ("Upper", "Lower"):
        name = f"EYE_Lid{part}_{side}"
        remove_mesh_object(name)
        mesh = bpy.data.meshes.new(name + "_Mesh")
        mesh.from_pydata(lid_coords(side, part, "Basis"), [], faces)
        mesh.materials.append(lid_mat)
        mesh.update()
        obj = bpy.data.objects.new(name, mesh)
        formal_collection.objects.link(obj)
        parent_to_head_keep_world(obj)
        for poly in mesh.polygons:
            poly.use_smooth = True

        obj.shape_key_add(name="Basis", from_mix=False)
        blink = obj.shape_key_add(name="Blink", from_mix=False)
        wide = obj.shape_key_add(name="Wide", from_mix=False)
        for index, co in enumerate(lid_coords(side, part, "Blink")):
            blink.data[index].co = co
        for index, co in enumerate(lid_coords(side, part, "Wide")):
            wide.data[index].co = co
        add_shape_driver(blink, f"Blink.{side}")
        add_shape_driver(wide, f"Eye_Wide.{side}")

        solid = obj.modifiers.new("EYE_LidSolidify", "SOLIDIFY")
        solid.thickness = 0.0009
        solid.offset = 0.0
        obj["production_role"] = "eyelid"
        obj["eye_side"] = side

# Reduce visible tearline thickness; the formal objects remain active.
for side in ("L", "R"):
    tear = bpy.data.objects.get(f"EYE_TearLine_{side}")
    if tear and tear.type == "CURVE":
        tear.data.bevel_depth = 0.00018
        tear.hide_render = False

# Lift iris/pupil/rim/catchlight very slightly for a more alert, reference-like gaze.
for side, cx in CENTERS.items():
    for role, z, dims in (
        ("Iris", EYE_CZ + 0.0030, (0.058, 0.0018, 0.058)),
        ("Pupil", EYE_CZ + 0.0030, (0.030, 0.0020, 0.030)),
        ("Catchlight", EYE_CZ + 0.0145, (0.0090, 0.0010, 0.0090)),
    ):
        obj = bpy.data.objects.get(f"EYE_{role}_{side}")
        mat = obj.matrix_world.copy()
        mat.translation.z = z
        if role == "Catchlight":
            mat.translation.x = cx + 0.0090
        obj.matrix_world = mat
        bpy.context.view_layer.update()
        current = obj.dimensions.copy()
        for axis in range(3):
            if abs(current[axis]) > 1.0e-8:
                obj.scale[axis] *= dims[axis] / current[axis]
    rim = bpy.data.objects.get(f"EYE_IrisLimbal_{side}")
    if rim:
        mat = rim.matrix_world.copy()
        mat.translation.z = 0.0030
        rim.matrix_world = mat

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

new_lid_vertex_counts = {
    obj.name: len(obj.data.vertices)
    for obj in bpy.data.objects
    if obj.name.startswith(("EYE_LidUpper_", "EYE_LidLower_"))
}
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
    "head_shape_keys_before": head_shape_names_before,
    "head_shape_keys_after": [kb.name for kb in head.data.shape_keys.key_blocks],
    "old_lid_vertex_counts": old_lid_vertex_counts,
    "new_lid_vertex_counts": new_lid_vertex_counts,
    "lid_driver_count": lid_driver_count,
    "same_lid_material": same_material,
    "renders": [os.path.join(RENDER_DIR, name) for name in (
        "neutral_front.png", "neutral_front_34.png", "neutral_side.png",
        "blink_front.png", "smile_front_34.png"
    )],
}
report["invariants_pass"] = bool(
    head_vertex_count_before == len(head.data.vertices)
    and head_shape_names_before == [kb.name for kb in head.data.shape_keys.key_blocks]
    and all(count == COLS * ROWS for count in new_lid_vertex_counts.values())
    and lid_driver_count == 8 and same_material
)
if not report["invariants_pass"]:
    raise RuntimeError("Candidate v4 invariants failed")
with open(REPORT, "w", encoding="utf-8") as f:
    json.dump(report, f, ensure_ascii=False, indent=2)

bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT, copy=False)
print(json.dumps({
    "status": "PASS",
    "checkpoint": CHECKPOINT,
    "report": REPORT,
    "invariants_pass": report["invariants_pass"],
    "lid_vertex_counts": new_lid_vertex_counts,
    "lid_driver_count": lid_driver_count,
    "head_vertex_count": len(head.data.vertices),
    "renders": report["renders"],
}, ensure_ascii=False))
