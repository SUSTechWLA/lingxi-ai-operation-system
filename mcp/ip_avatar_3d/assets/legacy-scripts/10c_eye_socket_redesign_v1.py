import bpy
import json
import math
import os
from mathutils import Matrix, Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_081_eye_bugfix_candidate_v1.blend")
REPORT = os.path.join(ROOT, "reports", "eye_bugfix_candidate_v1.json")
RENDER_DIR = os.path.join(ROOT, "renders", "eye_bugfix", "candidate_v1")
os.makedirs(os.path.dirname(CHECKPOINT), exist_ok=True)
os.makedirs(os.path.dirname(REPORT), exist_ok=True)
os.makedirs(RENDER_DIR, exist_ok=True)

scene = bpy.context.scene
head = bpy.data.objects.get("GEO_HeadBody")
rig = bpy.data.objects.get("RIG_Sloth")
formal_collection = bpy.data.collections.get("COL_CHR_SLOTH_FINAL")
if not head or head.type != "MESH" or not rig or rig.type != "ARMATURE":
    raise RuntimeError("Required GEO_HeadBody or RIG_Sloth missing")

head_vertex_count_before = len(head.data.vertices)
head_shape_keys_before = [kb.name for kb in head.data.shape_keys.key_blocks] if head.data.shape_keys else []

# Clean checkpoint-only audit objects before creating the production candidate.
for obj in list(bpy.data.objects):
    if obj.name.startswith("TMP_EyeBug"):
        data = obj.data
        bpy.data.objects.remove(obj, do_unlink=True)
        if data and data.users == 0:
            if isinstance(data, bpy.types.Camera):
                bpy.data.cameras.remove(data)
            elif isinstance(data, bpy.types.Light):
                bpy.data.lights.remove(data)

# Stage output is a new checkpoint; the previous checkpoint and final file remain untouched.
bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT, copy=False)


def set_principled(material, base_color, roughness, specular=0.35):
    material.use_nodes = True
    nt = material.node_tree
    nt.nodes.clear()
    out = nt.nodes.new("ShaderNodeOutputMaterial")
    bsdf = nt.nodes.new("ShaderNodeBsdfPrincipled")
    bsdf.inputs["Base Color"].default_value = base_color
    bsdf.inputs["Roughness"].default_value = roughness
    spec = bsdf.inputs.get("Specular IOR Level") or bsdf.inputs.get("Specular")
    if spec:
        spec.default_value = specular
    nt.links.new(bsdf.outputs["BSDF"], out.inputs["Surface"])
    return nt, bsdf


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


def assign_single_material(obj, material):
    obj.data.materials.clear()
    obj.data.materials.append(material)


def smooth_mesh(obj):
    if obj and obj.type == "MESH":
        for poly in obj.data.polygons:
            poly.use_smooth = True
        obj.data.update()


# Rebuild the shared eyelid skin material. Upper and lower lids deliberately use
# the exact same datablock so a closed eye cannot split into two colors.
lid_mat = bpy.data.materials.get("MAT_Eyelid_WarmBrown") or bpy.data.materials.new("MAT_Eyelid_WarmBrown")
lid_mat.name = "MAT_Eyelid_Skin_Brown"
nt, lid_bsdf = set_principled(lid_mat, (0.235, 0.070, 0.028, 1.0), 0.48, 0.30)
tex = nt.nodes.new("ShaderNodeTexNoise")
tex.inputs["Scale"].default_value = 22.0
tex.inputs["Detail"].default_value = 2.2
tex.inputs["Roughness"].default_value = 0.65
bump = nt.nodes.new("ShaderNodeBump")
bump.inputs["Strength"].default_value = 0.07
bump.inputs["Distance"].default_value = 0.002
nt.links.new(tex.outputs["Fac"], bump.inputs["Height"])
nt.links.new(bump.outputs["Normal"], lid_bsdf.inputs["Normal"])

old_lower_mat = bpy.data.materials.get("MAT_Eyelid_Lower")

# Warmer, less graphic eye materials.
sclera_mat = bpy.data.materials.get("MAT_Eye_Sclera") or bpy.data.materials.new("MAT_Eye_Sclera")
set_principled(sclera_mat, (0.58, 0.50, 0.42, 1.0), 0.34, 0.42)

iris_mat = bpy.data.materials.get("MAT_Eye_Iris_Amber") or bpy.data.materials.new("MAT_Eye_Iris_Amber")
nt, iris_bsdf = set_principled(iris_mat, (0.32, 0.075, 0.009, 1.0), 0.27, 0.38)
texcoord = nt.nodes.new("ShaderNodeTexCoord")
noise = nt.nodes.new("ShaderNodeTexNoise")
noise.inputs["Scale"].default_value = 14.0
noise.inputs["Detail"].default_value = 4.0
noise.inputs["Roughness"].default_value = 0.72
noise.inputs["Distortion"].default_value = 0.18
ramp = nt.nodes.new("ShaderNodeValToRGB")
ramp.color_ramp.elements[0].position = 0.24
ramp.color_ramp.elements[0].color = (0.035, 0.006, 0.002, 1.0)
ramp.color_ramp.elements[1].position = 0.78
ramp.color_ramp.elements[1].color = (0.60, 0.17, 0.012, 1.0)
ibump = nt.nodes.new("ShaderNodeBump")
ibump.inputs["Strength"].default_value = 0.10
ibump.inputs["Distance"].default_value = 0.004
nt.links.new(texcoord.outputs["Generated"], noise.inputs["Vector"])
nt.links.new(noise.outputs["Fac"], ramp.inputs["Fac"])
nt.links.new(ramp.outputs["Color"], iris_bsdf.inputs["Base Color"])
nt.links.new(noise.outputs["Fac"], ibump.inputs["Height"])
nt.links.new(ibump.outputs["Normal"], iris_bsdf.inputs["Normal"])

pupil_mat = bpy.data.materials.get("MAT_Eye_Pupil") or bpy.data.materials.new("MAT_Eye_Pupil")
set_principled(pupil_mat, (0.006, 0.0025, 0.0015, 1.0), 0.20, 0.45)

limbal_mat = bpy.data.materials.get("MAT_Eye_Iris_Limbal") or bpy.data.materials.new("MAT_Eye_Iris_Limbal")
set_principled(limbal_mat, (0.028, 0.006, 0.0025, 1.0), 0.29, 0.34)

tear_mat = bpy.data.materials.get("MAT_TearLine") or bpy.data.materials.new("MAT_TearLine")
set_principled(tear_mat, (0.20, 0.12, 0.075, 1.0), 0.18, 0.50)

catch_mat = bpy.data.materials.get("MAT_Eye_Catchlight") or bpy.data.materials.new("MAT_Eye_Catchlight")
set_principled(catch_mat, (0.82, 0.82, 0.78, 1.0), 0.12, 0.40)


# New eye envelope derived from the socket probe.
EYE_CY = -0.175
EYE_CZ = 2.198
EYE_RX = 0.055
EYE_RY = 0.044
EYE_RZ = 0.066
CENTERS = {"L": 0.125, "R": -0.125}

before_eye = {}
after_eye = {}
for side, cx in CENTERS.items():
    targets = {
        "Sclera": ((cx, EYE_CY, EYE_CZ), (0.110, 0.088, 0.132)),
        "Cornea": ((cx, EYE_CY - 0.0012, EYE_CZ), (0.113, 0.092, 0.135)),
        "Iris": ((cx, EYE_CY - EYE_RY - 0.0008, EYE_CZ), (0.048, 0.0018, 0.048)),
        "Pupil": ((cx, EYE_CY - EYE_RY - 0.0014, EYE_CZ), (0.023, 0.0020, 0.023)),
        "Catchlight": ((cx + 0.008, EYE_CY - EYE_RY - 0.0020, EYE_CZ + 0.010), (0.0082, 0.0010, 0.0082)),
    }
    for role, (location, dimensions) in targets.items():
        obj = bpy.data.objects.get(f"EYE_{role}_{side}")
        if not obj:
            raise RuntimeError(f"Missing EYE_{role}_{side}")
        before_eye[obj.name] = {
            "world_location": [round(v, 6) for v in obj.matrix_world.translation],
            "dimensions": [round(v, 6) for v in obj.dimensions],
        }
        set_world_dimensions(obj, dimensions)
        set_object_world_location(obj, location)
        smooth_mesh(obj)
        if role == "Sclera":
            assign_single_material(obj, sclera_mat)
        elif role == "Iris":
            assign_single_material(obj, iris_mat)
        elif role == "Pupil":
            assign_single_material(obj, pupil_mat)
        elif role == "Catchlight":
            assign_single_material(obj, catch_mat)
        after_eye[obj.name] = {
            "world_location": [round(v, 6) for v in obj.matrix_world.translation],
            "dimensions": [round(v, 6) for v in obj.dimensions],
        }


def ellipsoid_front_y(x, z, cx):
    nx = (x - cx) / EYE_RX
    nz = (z - EYE_CZ) / EYE_RZ
    inside = max(0.0, 1.0 - nx * nx - nz * nz)
    return EYE_CY - EYE_RY * math.sqrt(inside)


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
                inner_z = EYE_CZ + 0.040 * s
            elif pose == "Blink":
                inner_z = EYE_CZ - 0.0035 * s * s
            else:  # Wide
                inner_z = EYE_CZ + 0.048 * s
        else:
            outer_z = EYE_CZ - EYE_RZ * s
            if pose == "Basis":
                inner_z = EYE_CZ - 0.036 * s
            elif pose == "Blink":
                inner_z = EYE_CZ - 0.0035 * s * s
            else:  # Wide
                inner_z = EYE_CZ - 0.044 * s

        outer = Vector((x, ellipsoid_front_y(x, outer_z, cx) - 0.0012, outer_z))
        inner = Vector((x, ellipsoid_front_y(x, inner_z, cx) - 0.0032, inner_z))
        coords.extend((outer, inner))
    return coords


lid_driver_count_before = 0
for side in ("L", "R"):
    for part in ("Upper", "Lower"):
        obj = bpy.data.objects.get(f"EYE_Lid{part}_{side}")
        if not obj or obj.type != "MESH" or len(obj.data.vertices) != 80 or not obj.data.shape_keys:
            raise RuntimeError(f"Unexpected topology on EYE_Lid{part}_{side}")
        if obj.data.shape_keys.animation_data:
            lid_driver_count_before += len(obj.data.shape_keys.animation_data.drivers)
        assign_single_material(obj, lid_mat)
        inv = obj.matrix_world.inverted()
        for key_name in ("Basis", "Blink", "Wide"):
            key = obj.data.shape_keys.key_blocks.get(key_name)
            if not key:
                raise RuntimeError(f"Missing {key_name} on {obj.name}")
            coords = lid_world_pairs(side, part, key_name)
            for index, world_co in enumerate(coords):
                local_co = inv @ world_co
                key.data[index].co = local_co
                if key_name == "Basis":
                    obj.data.vertices[index].co = local_co
        for poly in obj.data.polygons:
            poly.use_smooth = True
        obj.data.update()
        solid = obj.modifiers.get("EYE_LidSolidify") or obj.modifiers.new("EYE_LidSolidify", "SOLIDIFY")
        solid.thickness = 0.0012
        solid.offset = 0.0

if old_lower_mat and old_lower_mat.users == 0:
    bpy.data.materials.remove(old_lower_mat)


def link_formal(obj):
    if formal_collection and obj.name not in formal_collection.objects:
        formal_collection.objects.link(obj)
    if not obj.users_collection:
        scene.collection.objects.link(obj)


def parent_to_bone_keep_world(obj, bone_name):
    world = obj.matrix_world.copy()
    obj.parent = rig
    obj.parent_type = "BONE"
    obj.parent_bone = bone_name
    obj.matrix_world = world


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


def create_limbal_ring(side, cx):
    name = f"EYE_IrisLimbal_{side}"
    remove_object_and_data(name)
    verts = []
    faces = []
    count = 64
    outer_r = 0.0272
    inner_r = 0.0235
    y = EYE_CY - EYE_RY - 0.0010
    for i in range(count):
        a = 2.0 * math.pi * i / count
        ca, sa = math.cos(a), math.sin(a)
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
    if formal_collection:
        formal_collection.objects.link(obj)
    else:
        scene.collection.objects.link(obj)
    parent_to_bone_keep_world(obj, f"Eye.{side}")
    return obj


def create_tearline(side, cx):
    name = f"EYE_TearLine_{side}"
    remove_object_and_data(name)
    curve = bpy.data.curves.new(name + "_Curve", "CURVE")
    curve.dimensions = "3D"
    curve.resolution_u = 2
    curve.bevel_depth = 0.00075
    curve.bevel_resolution = 2
    curve.materials.append(tear_mat)
    spline = curve.splines.new("BEZIER")
    count = 24
    spline.bezier_points.add(count - 1)
    for i, bp in enumerate(spline.bezier_points):
        u = -0.90 + 1.80 * i / (count - 1)
        s = math.sqrt(max(0.0, 1.0 - u * u))
        x = cx + EYE_RX * u
        z = EYE_CZ - 0.036 * s
        y = ellipsoid_front_y(x, z, cx) - 0.0042
        bp.co = (x, y, z)
        bp.handle_left_type = "AUTO"
        bp.handle_right_type = "AUTO"
    obj = bpy.data.objects.new(name, curve)
    if formal_collection:
        formal_collection.objects.link(obj)
    else:
        scene.collection.objects.link(obj)
    parent_to_bone_keep_world(obj, "Head")
    return obj


for side, cx in CENTERS.items():
    create_limbal_ring(side, cx)
    create_tearline(side, cx)

# Set a deterministic neutral face before comparisons.
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
temp_objects = [cam]
cam_data.sensor_width = 36.0
scene.camera = cam

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
    kb = head.data.shape_keys.key_blocks.get(name)
    if kb:
        kb.value = 1.0
bpy.context.view_layer.update()
render_view("blink_front.png", (0.0, -3.35, 2.20))

for name in ("Blink.L", "Blink.R"):
    kb = head.data.shape_keys.key_blocks.get(name)
    if kb:
        kb.value = 0.0
for name in ("Eye_Wide.L", "Eye_Wide.R"):
    kb = head.data.shape_keys.key_blocks.get(name)
    if kb:
        kb.value = 1.0
bpy.context.view_layer.update()
render_view("wide_front.png", (0.0, -3.35, 2.20))

# Restore neutral expression and production render settings.
if head.data.shape_keys:
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

lid_driver_count_after = 0
lid_materials = {}
for side in ("L", "R"):
    upper = bpy.data.objects.get(f"EYE_LidUpper_{side}")
    lower = bpy.data.objects.get(f"EYE_LidLower_{side}")
    for obj in (upper, lower):
        if obj.data.shape_keys.animation_data:
            lid_driver_count_after += len(obj.data.shape_keys.animation_data.drivers)
    lid_materials[side] = {
        "upper": upper.material_slots[0].material.name,
        "lower": lower.material_slots[0].material.name,
        "same_datablock": upper.material_slots[0].material is lower.material_slots[0].material,
    }

report = {
    "status": "PASS",
    "checkpoint": CHECKPOINT,
    "head_vertex_count_before": head_vertex_count_before,
    "head_vertex_count_after": len(head.data.vertices),
    "head_shape_keys_before": head_shape_keys_before,
    "head_shape_keys_after": [kb.name for kb in head.data.shape_keys.key_blocks] if head.data.shape_keys else [],
    "lid_vertex_counts": {
        obj.name: len(obj.data.vertices)
        for obj in bpy.data.objects
        if obj.name.startswith(("EYE_LidUpper_", "EYE_LidLower_"))
    },
    "lid_driver_count_before": lid_driver_count_before,
    "lid_driver_count_after": lid_driver_count_after,
    "lid_materials": lid_materials,
    "eye_before": before_eye,
    "eye_after": after_eye,
    "renders": [
        os.path.join(RENDER_DIR, name) for name in (
            "neutral_front.png", "neutral_front_34.png", "neutral_side.png",
            "blink_front.png", "wide_front.png"
        )
    ],
}
report["invariants_pass"] = bool(
    report["head_vertex_count_before"] == report["head_vertex_count_after"]
    and report["head_shape_keys_before"] == report["head_shape_keys_after"]
    and lid_driver_count_before == lid_driver_count_after == 8
    and all(v["same_datablock"] for v in lid_materials.values())
)
if not report["invariants_pass"]:
    raise RuntimeError("Eye redesign invariants failed")

with open(REPORT, "w", encoding="utf-8") as f:
    json.dump(report, f, ensure_ascii=False, indent=2)

bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT, copy=False)
print(json.dumps({
    "status": report["status"],
    "invariants_pass": report["invariants_pass"],
    "checkpoint": CHECKPOINT,
    "report": REPORT,
    "lid_materials": lid_materials,
    "lid_driver_count": lid_driver_count_after,
    "head_vertex_count": report["head_vertex_count_after"],
    "renders": report["renders"],
}, ensure_ascii=False))
