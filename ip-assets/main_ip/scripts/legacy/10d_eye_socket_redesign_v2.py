import bpy
import json
import math
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_082_eye_bugfix_candidate_v2.blend")
REPORT = os.path.join(ROOT, "reports", "eye_bugfix_candidate_v2.json")
RENDER_DIR = os.path.join(ROOT, "renders", "eye_bugfix", "candidate_v2")
os.makedirs(os.path.dirname(CHECKPOINT), exist_ok=True)
os.makedirs(os.path.dirname(REPORT), exist_ok=True)
os.makedirs(RENDER_DIR, exist_ok=True)

scene = bpy.context.scene
head = bpy.data.objects.get("GEO_HeadBody")
rig = bpy.data.objects.get("RIG_Sloth")
formal_collection = bpy.data.collections.get("COL_CHR_SLOTH_FINAL")
if not head or not rig:
    raise RuntimeError("Required formal head or rig is missing")

head_vertex_count_before = len(head.data.vertices)
head_shape_keys_before = [kb.name for kb in head.data.shape_keys.key_blocks]

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


def set_object_world_location(obj, location):
    mat = obj.matrix_world.copy()
    mat.translation = Vector(location)
    obj.matrix_world = mat


def set_world_dimensions(obj, dimensions):
    bpy.context.view_layer.update()
    current = obj.dimensions.copy()
    for axis in range(3):
        if abs(current[axis]) > 1.0e-8:
            obj.scale[axis] *= dimensions[axis] / current[axis]
    bpy.context.view_layer.update()


def remove_object_and_data(name):
    obj = bpy.data.objects.get(name)
    if not obj:
        return
    data = obj.data
    bpy.data.objects.remove(obj, do_unlink=True)
    if data and data.users == 0:
        if isinstance(data, bpy.types.Mesh):
            bpy.data.meshes.remove(data)
        elif isinstance(data, bpy.types.Curve):
            bpy.data.curves.remove(data)


def parent_to_bone_keep_world(obj, bone_name):
    world = obj.matrix_world.copy()
    obj.parent = rig
    obj.parent_type = "BONE"
    obj.parent_bone = bone_name
    obj.matrix_world = world


EYE_CY = -0.176
EYE_CZ = 2.198
EYE_RX = 0.0615
EYE_RY = 0.045
EYE_RZ = 0.074
CENTERS = {"L": 0.125, "R": -0.125}

target_specs = {}
for side, cx in CENTERS.items():
    target_specs.update({
        f"EYE_Sclera_{side}": ((cx, EYE_CY, EYE_CZ), (0.123, 0.090, 0.148)),
        f"EYE_Cornea_{side}": ((cx, EYE_CY - 0.0012, EYE_CZ), (0.126, 0.094, 0.151)),
        f"EYE_Iris_{side}": ((cx, EYE_CY - EYE_RY - 0.0007, EYE_CZ), (0.058, 0.0018, 0.058)),
        f"EYE_Pupil_{side}": ((cx, EYE_CY - EYE_RY - 0.0013, EYE_CZ), (0.030, 0.0020, 0.030)),
        f"EYE_Catchlight_{side}": ((cx + 0.010, EYE_CY - EYE_RY - 0.0020, EYE_CZ + 0.012), (0.0100, 0.0010, 0.0100)),
    })

for name, (location, dimensions) in target_specs.items():
    obj = bpy.data.objects.get(name)
    if not obj:
        raise RuntimeError(f"Missing {name}")
    set_world_dimensions(obj, dimensions)
    set_object_world_location(obj, location)

# Darker, richer amber than candidate v1.
iris_mat = bpy.data.materials.get("MAT_Eye_Iris_Amber")
if iris_mat and iris_mat.use_nodes:
    ramps = [n for n in iris_mat.node_tree.nodes if n.type == "VALTORGB"]
    if ramps:
        ramp = ramps[0].color_ramp
        ramp.elements[0].position = 0.20
        ramp.elements[0].color = (0.012, 0.0018, 0.0005, 1.0)
        ramp.elements[1].position = 0.82
        ramp.elements[1].color = (0.26, 0.055, 0.0025, 1.0)

pupil_mat = bpy.data.materials.get("MAT_Eye_Pupil")
if pupil_mat and pupil_mat.use_nodes:
    bsdf = next((n for n in pupil_mat.node_tree.nodes if n.type == "BSDF_PRINCIPLED"), None)
    if bsdf:
        bsdf.inputs["Base Color"].default_value = (0.0015, 0.0005, 0.0002, 1.0)
        spec = bsdf.inputs.get("Specular IOR Level") or bsdf.inputs.get("Specular")
        if spec:
            spec.default_value = 0.22

lid_mat = bpy.data.materials.get("MAT_Eyelid_Skin_Brown")
if not lid_mat:
    raise RuntimeError("Shared eyelid material missing")
if lid_mat.use_nodes:
    bsdf = next((n for n in lid_mat.node_tree.nodes if n.type == "BSDF_PRINCIPLED"), None)
    if bsdf:
        bsdf.inputs["Base Color"].default_value = (0.18, 0.043, 0.014, 1.0)
        bsdf.inputs["Roughness"].default_value = 0.54


def lid_world_pairs(side, part, pose):
    cx = CENTERS[side]
    coords = []
    samples = 40
    for i in range(samples):
        u = -1.0 + 2.0 * i / (samples - 1)
        s = math.sqrt(max(0.0, 1.0 - u * u))
        x = cx + EYE_RX * u
        if part == "Upper":
            outer_z = EYE_CZ + EYE_RZ * s
            if pose == "Basis":
                inner_z = EYE_CZ + 0.047 * s
            elif pose == "Blink":
                inner_z = EYE_CZ - 0.0040 * s * s
            else:
                inner_z = EYE_CZ + 0.055 * s
        else:
            outer_z = EYE_CZ - EYE_RZ * s
            if pose == "Basis":
                inner_z = EYE_CZ - 0.042 * s
            elif pose == "Blink":
                inner_z = EYE_CZ - 0.0040 * s * s
            else:
                inner_z = EYE_CZ - 0.050 * s

        # Outer edges settle into the face plane, while the aperture edge sits
        # just in front of the cornea. This produces coverage without a bulging globe.
        outer_y = -0.2145 - 0.0020 * s
        inner_y = -0.2280 + 0.0030 * u * u
        coords.extend((Vector((x, outer_y, outer_z)), Vector((x, inner_y, inner_z))))
    return coords


for side in ("L", "R"):
    for part in ("Upper", "Lower"):
        obj = bpy.data.objects.get(f"EYE_Lid{part}_{side}")
        if not obj or len(obj.data.vertices) != 80 or not obj.data.shape_keys:
            raise RuntimeError(f"Unexpected lid topology: EYE_Lid{part}_{side}")
        obj.data.materials.clear()
        obj.data.materials.append(lid_mat)
        inv = obj.matrix_world.inverted()
        for key_name in ("Basis", "Blink", "Wide"):
            kb = obj.data.shape_keys.key_blocks.get(key_name)
            coords = lid_world_pairs(side, part, key_name)
            for index, world_co in enumerate(coords):
                local = inv @ world_co
                kb.data[index].co = local
                if key_name == "Basis":
                    obj.data.vertices[index].co = local
        obj.data.update()

limbal_mat = bpy.data.materials.get("MAT_Eye_Iris_Limbal")
tear_mat = bpy.data.materials.get("MAT_TearLine")


def create_limbal_ring(side, cx):
    name = f"EYE_IrisLimbal_{side}"
    remove_object_and_data(name)
    verts, faces = [], []
    count = 64
    outer_r, inner_r = 0.0312, 0.0270
    y = EYE_CY - EYE_RY - 0.00095
    for i in range(count):
        angle = 2.0 * math.pi * i / count
        ca, sa = math.cos(angle), math.sin(angle)
        verts.append((cx + outer_r * ca, y, EYE_CZ + outer_r * sa))
        verts.append((cx + inner_r * ca, y - 0.0001, EYE_CZ + inner_r * sa))
    for i in range(count):
        j = (i + 1) % count
        faces.append((2 * i, 2 * i + 1, 2 * j + 1, 2 * j))
    mesh = bpy.data.meshes.new(name + "_Mesh")
    mesh.from_pydata(verts, [], faces)
    mesh.materials.append(limbal_mat)
    mesh.update()
    obj = bpy.data.objects.new(name, mesh)
    (formal_collection or scene.collection).objects.link(obj)
    parent_to_bone_keep_world(obj, f"Eye.{side}")


def create_tearline(side, cx):
    name = f"EYE_TearLine_{side}"
    remove_object_and_data(name)
    curve = bpy.data.curves.new(name + "_Curve", "CURVE")
    curve.dimensions = "3D"
    curve.resolution_u = 2
    curve.bevel_depth = 0.00065
    curve.bevel_resolution = 2
    curve.materials.append(tear_mat)
    spline = curve.splines.new("BEZIER")
    count = 24
    spline.bezier_points.add(count - 1)
    for i, bp in enumerate(spline.bezier_points):
        u = -0.90 + 1.80 * i / (count - 1)
        s = math.sqrt(max(0.0, 1.0 - u * u))
        bp.co = (
            cx + EYE_RX * u,
            -0.2290 + 0.0030 * u * u,
            EYE_CZ - 0.042 * s,
        )
        bp.handle_left_type = "AUTO"
        bp.handle_right_type = "AUTO"
    obj = bpy.data.objects.new(name, curve)
    (formal_collection or scene.collection).objects.link(obj)
    parent_to_bone_keep_world(obj, "Head")


for side, cx in CENTERS.items():
    create_limbal_ring(side, cx)
    create_tearline(side, cx)

if head.data.shape_keys:
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
for name in ("Eye_Wide.L", "Eye_Wide.R"):
    head.data.shape_keys.key_blocks[name].value = 1.0
bpy.context.view_layer.update()
render_view("wide_front.png", (0.0, -3.35, 2.20))

for kb in head.data.shape_keys.key_blocks:
    if kb.name != "Basis":
        kb.value = 0.0
rest = head.data.shape_keys.key_blocks.get("Mouth_Rest")
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
lid_material_match = all(
    bpy.data.objects[f"EYE_LidUpper_{side}"].material_slots[0].material
    is bpy.data.objects[f"EYE_LidLower_{side}"].material_slots[0].material
    for side in ("L", "R")
)

report = {
    "status": "PASS",
    "checkpoint": CHECKPOINT,
    "head_vertex_count_before": head_vertex_count_before,
    "head_vertex_count_after": len(head.data.vertices),
    "head_shape_keys_before": head_shape_keys_before,
    "head_shape_keys_after": [kb.name for kb in head.data.shape_keys.key_blocks],
    "lid_driver_count": lid_driver_count,
    "lid_material_match": lid_material_match,
    "eye_dimensions": {
        name: [round(v, 6) for v in bpy.data.objects[name].dimensions]
        for name in target_specs
    },
    "renders": [os.path.join(RENDER_DIR, name) for name in (
        "neutral_front.png", "neutral_front_34.png", "neutral_side.png",
        "blink_front.png", "wide_front.png"
    )],
}
report["invariants_pass"] = bool(
    head_vertex_count_before == len(head.data.vertices)
    and head_shape_keys_before == [kb.name for kb in head.data.shape_keys.key_blocks]
    and lid_driver_count == 8
    and lid_material_match
)
if not report["invariants_pass"]:
    raise RuntimeError("Candidate v2 invariants failed")

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
