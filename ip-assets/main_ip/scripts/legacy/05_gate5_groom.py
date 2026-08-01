import bpy
import json
import math
import os
import random
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_030_fur.blend")
REPORT = os.path.join(ROOT, "reports", "gate5_groom_report.json")
RENDER_DIR = os.path.join(ROOT, "renders", "lookdev", "gate5")
CHAR_COLLECTION = "COL_CHR_SLOTH_FINAL"
GROOM_COLLECTION = "GROOM_SLOTH"


def ensure_dir(path):
    os.makedirs(path, exist_ok=True)


def set_input(node, name, value):
    socket = node.inputs.get(name)
    if socket is not None:
        socket.default_value = value


def fur_material(name, root_color, tip_color, roughness):
    mat = bpy.data.materials.get(name) or bpy.data.materials.new(name)
    mat.use_nodes = True
    nodes = mat.node_tree.nodes
    links = mat.node_tree.links
    nodes.clear()
    out = nodes.new("ShaderNodeOutputMaterial")
    bsdf = nodes.new("ShaderNodeBsdfPrincipled")
    noise = nodes.new("ShaderNodeTexNoise")
    ramp = nodes.new("ShaderNodeValToRGB")
    coord = nodes.new("ShaderNodeTexCoord")
    noise.inputs["Scale"].default_value = 8.0
    noise.inputs["Detail"].default_value = 3.0
    noise.inputs["Roughness"].default_value = 0.72
    ramp.color_ramp.elements[0].color = root_color
    ramp.color_ramp.elements[1].color = tip_color
    set_input(bsdf, "Roughness", roughness)
    set_input(bsdf, "Specular IOR Level", 0.22)
    set_input(bsdf, "Specular", 0.22)
    set_input(bsdf, "Coat Weight", 0.0)
    set_input(bsdf, "Sheen Weight", 0.14)
    links.new(coord.outputs["Generated"], noise.inputs["Vector"])
    links.new(noise.outputs["Fac"], ramp.inputs["Fac"])
    links.new(ramp.outputs["Color"], bsdf.inputs["Base Color"])
    links.new(bsdf.outputs["BSDF"], out.inputs["Surface"])
    return mat


def move_to_collection(obj, collection):
    for owner in list(obj.users_collection):
        owner.objects.unlink(obj)
    collection.objects.link(obj)


def attach_to_bone(obj, rig, bone_name):
    bpy.context.view_layer.update()
    world = obj.matrix_world.copy()
    obj.parent = rig
    obj.parent_type = "BONE"
    obj.parent_bone = bone_name
    obj.matrix_world = world


def add_strand_object(name, strand_points, bevel_depth, materials, collection, rig, bone="Head"):
    curve = bpy.data.curves.new(name + "_Curve", "CURVE")
    curve.dimensions = "3D"
    curve.resolution_u = 1
    curve.bevel_depth = bevel_depth
    curve.bevel_resolution = 1
    curve.resolution_u = 2
    curve.use_fill_caps = True
    for mat in materials:
        curve.materials.append(mat)
    for index, points in enumerate(strand_points):
        spline = curve.splines.new("POLY")
        spline.points.add(len(points) - 1)
        for point, co in zip(spline.points, points):
            point.co = (*co, 1.0)
        spline.material_index = index % max(1, len(materials))
    obj = bpy.data.objects.new(name, curve)
    collection.objects.link(obj)
    attach_to_bone(obj, rig, bone)
    return obj


def curve_root_world(obj, spline):
    if spline.type == "BEZIER":
        co = spline.bezier_points[0].co
    else:
        co = spline.points[0].co.xyz
    return obj.matrix_world @ co


def soften_face_material_boundary(obj, ivory, light_brown, brown):
    obj.data.materials.clear()
    obj.data.materials.append(ivory)
    obj.data.materials.append(light_brown)
    obj.data.materials.append(brown)
    counts = {"ivory": 0, "light_brown": 0, "brown": 0}
    for i, spline in enumerate(obj.data.splines):
        root = curve_root_world(obj, spline)
        distance = (root.x / 0.29) ** 2 + ((root.z - 2.22) / 0.34) ** 2
        if distance < 0.50:
            spline.material_index = 0
            counts["ivory"] += 1
        elif distance < 0.95:
            # Interleave colors across the transition band instead of using a
            # binary material border.
            phase = (i * 37) % 10
            if distance < 0.68 and phase < 6:
                spline.material_index = 0
                counts["ivory"] += 1
            elif phase < 8:
                spline.material_index = 1
                counts["light_brown"] += 1
            else:
                spline.material_index = 2
                counts["brown"] += 1
        else:
            spline.material_index = 2 if i % 4 else 1
            counts["brown" if spline.material_index == 2 else "light_brown"] += 1
    return counts


def look_at(obj, point):
    obj.rotation_euler = (Vector(point) - obj.location).to_track_quat("-Z", "Y").to_euler()


def render_views(scene):
    ensure_dir(RENDER_DIR)
    cam = bpy.data.objects.get("CAM_LOOKDEV_65MM")
    close_cam = bpy.data.objects.get("CAM_LOOKDEV_100MM") or cam
    if cam is None:
        raise RuntimeError("LookDev camera missing")
    scene.view_layers[0].material_override = None
    scene.render.engine = "BLENDER_EEVEE"
    scene.render.resolution_x = 768
    scene.render.resolution_y = 768
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "PNG"
    outputs = {}
    setups = {
        "front": ((0.0, -8.40, 2.45), (0.0, 0.0, 1.30), 65),
        "side": ((8.40, 0.0, 2.45), (0.0, 0.0, 1.30), 65),
        "rim": ((4.90, 6.80, 2.65), (0.0, 0.0, 1.35), 80),
    }
    for name, (location, target, lens) in setups.items():
        cam.location = location
        cam.data.lens = lens
        look_at(cam, target)
        scene.camera = cam
        path = os.path.join(RENDER_DIR, name + ".png")
        scene.render.filepath = path
        bpy.ops.render.render(write_still=True)
        outputs[name] = path
    close_cam.location = (0.0, -4.63, 2.30)
    close_cam.data.lens = 100
    look_at(close_cam, (0.0, -0.02, 2.22))
    scene.camera = close_cam
    path = os.path.join(RENDER_DIR, "face_closeup.png")
    scene.render.filepath = path
    bpy.ops.render.render(write_still=True)
    outputs["face_closeup"] = path
    return outputs


ensure_dir(os.path.dirname(CHECKPOINT))
ensure_dir(os.path.dirname(REPORT))
char_collection = bpy.data.collections.get(CHAR_COLLECTION)
rig = next((obj for obj in bpy.data.objects if obj.type == "ARMATURE"), None)
main_obj = bpy.data.objects.get("part_00000001.001")
if char_collection is None or rig is None or main_obj is None:
    raise RuntimeError("Gate 5 prerequisites missing")
before_vertex_count = len(main_obj.data.vertices)

groom_collection = bpy.data.collections.get(GROOM_COLLECTION)
if groom_collection is None:
    groom_collection = bpy.data.collections.new(GROOM_COLLECTION)
if groom_collection.name not in {child.name for child in char_collection.children}:
    char_collection.children.link(groom_collection)

brown = fur_material("MAT_Fur_RootBrown", (0.075, 0.023, 0.008, 1.0), (0.25, 0.105, 0.038, 1.0), 0.72)
light_brown = fur_material("MAT_Fur_TipBrown", (0.12, 0.052, 0.018, 1.0), (0.39, 0.20, 0.075, 1.0), 0.75)
ivory = fur_material("MAT_Fur_Ivory", (0.42, 0.36, 0.28, 1.0), (0.72, 0.65, 0.53, 1.0), 0.80)

rename_map = {
    "CXR_FurMaskFeather": "FUR_Face",
    "CXR_FurHead": "FUR_Body",
    "CXR_FurTuft": "FUR_HeadTuft",
    "CXR_FurHand.L": "FUR_HandsFeet_L",
    "CXR_FurHand.R": "FUR_HandsFeet_R",
    "CXR_BrowFur.L": "FUR_Brows_L",
    "CXR_BrowFur.R": "FUR_Brows_R",
}
renamed = {}
for old_name, new_name in rename_map.items():
    obj = bpy.data.objects.get(old_name)
    if obj is None:
        raise RuntimeError("Missing groom source: " + old_name)
    obj.name = new_name
    obj.data.name = new_name + "_Curve"
    move_to_collection(obj, groom_collection)
    if obj.type == "CURVE":
        obj.data.bevel_depth = min(max(obj.data.bevel_depth * 0.78, 0.00045), 0.00135)
        obj.data.bevel_resolution = 1
    renamed[old_name] = new_name

face_blend_counts = soften_face_material_boundary(bpy.data.objects["FUR_Face"], ivory, light_brown, brown)
for name in ("FUR_Body", "FUR_HeadTuft", "FUR_HandsFeet_L", "FUR_HandsFeet_R", "FUR_Brows_L", "FUR_Brows_R"):
    obj = bpy.data.objects[name]
    obj.data.materials.clear()
    obj.data.materials.append(brown)
    obj.data.materials.append(light_brown)
    for i, spline in enumerate(obj.data.splines):
        spline.material_index = 0 if i % 5 else 1

rng = random.Random(51827)
muzzle_strands = []
for side in (-1.0, 1.0):
    for i in range(34):
        x = side * (0.055 + rng.random() * 0.155)
        z = 2.065 + rng.random() * 0.125
        y = -0.302 - 0.012 * (1.0 - min(1.0, abs(x) / 0.21))
        length = 0.008 + rng.random() * 0.012
        root = (x, y, z)
        mid = (x + side * length * 0.28, y - length * 0.52, z + (rng.random() - 0.5) * 0.004)
        tip = (x + side * length * 0.55, y - length, z + (rng.random() - 0.5) * 0.007)
        muzzle_strands.append((root, mid, tip))
fur_muzzle = add_strand_object("FUR_Muzzle", muzzle_strands, 0.00055, [ivory, light_brown], groom_collection, rig)

ear_strands = []
for side in (-1.0, 1.0):
    for i in range(28):
        x = side * (0.345 + rng.random() * 0.070)
        y = -0.105 + (rng.random() - 0.5) * 0.085
        z = 2.115 + rng.random() * 0.145
        length = 0.010 + rng.random() * 0.012
        ear_strands.append(((x, y, z), (x + side * length * 0.45, y - length * 0.25, z), (x + side * length, y - length * 0.45, z + (rng.random() - 0.5) * 0.008)))
fur_ears = add_strand_object("FUR_Ears", ear_strands, 0.00060, [brown, light_brown], groom_collection, rig)

outline_strands = []
for side in (-1.0, 1.0):
    for i in range(30):
        t = i / 29.0
        z = 1.98 + t * 0.54
        x = side * (0.345 + 0.035 * math.sin(math.pi * t))
        y = -0.035 + 0.035 * math.cos(math.pi * t)
        length = 0.010 + 0.010 * math.sin(math.pi * t)
        outline_strands.append(((x, y, z), (x + side * length * 0.55, y, z + 0.002), (x + side * length, y + 0.002, z + 0.004)))
fur_outline = add_strand_object("FUR_Outline", outline_strands, 0.00048, [brown, light_brown], groom_collection, rig)

formal_roles = [
    "FUR_Face", "FUR_Muzzle", "FUR_Body", "FUR_HandsFeet_L", "FUR_HandsFeet_R",
    "FUR_Ears", "FUR_Brows_L", "FUR_Brows_R", "FUR_HeadTuft", "FUR_Outline",
]
scene = bpy.data.scenes.get("SCENE_LOOKDEV") or bpy.context.scene
bpy.context.window.scene = scene
renders = render_views(scene)

checks = {
    "main_vertex_count_preserved": len(main_obj.data.vertices) == before_vertex_count == 5478,
    "single_armature": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]) == 1,
    "formal_groom_roles_present": all(bpy.data.objects.get(name) is not None for name in formal_roles),
    "legacy_groom_names_removed": not any(bpy.data.objects.get(name) for name in rename_map),
    "face_boundary_uses_multiple_tones": sum(1 for value in face_blend_counts.values() if value > 0) >= 2,
    "no_eye_nose_mouth_strands_in_new_muzzle": all(abs(points[0][0]) >= 0.055 and points[0][2] <= 2.19 for points in muzzle_strands),
}
if not all(checks.values()):
    raise RuntimeError("Gate 5 invariant failed: " + json.dumps(checks))

bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT)
report = {
    "gate": 5,
    "status": "PASS",
    "source_checkpoint": os.path.join(ROOT, "checkpoints", "sloth_020_face_eyes.blend"),
    "result_checkpoint": CHECKPOINT,
    "groom_collection": GROOM_COLLECTION,
    "renamed_existing_grooms": renamed,
    "formal_roles": formal_roles,
    "face_transition_spline_counts": face_blend_counts,
    "new_guide_counts": {"muzzle": len(muzzle_strands), "ears": len(ear_strands), "outline": len(outline_strands)},
    "checks": checks,
    "renders": renders,
    "notes": [
        "Existing authored curves were retained and reorganized instead of replaced with density brute force.",
        "White and brown face regions interleave three tones inside a transition band.",
        "Muzzle guides exclude the central nose and mouth corridor.",
        "All new guide objects are head-bone parented and remain part of one GROOM_SLOTH collection.",
    ],
}
with open(REPORT, "w", encoding="utf-8") as handle:
    json.dump(report, handle, ensure_ascii=False, indent=2)
print(json.dumps({"status": "PASS", "checkpoint": CHECKPOINT, "report": REPORT, "checks": checks, "renders": renders}, ensure_ascii=False))
