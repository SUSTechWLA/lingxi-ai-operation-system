import bpy
import json
import math
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_040_clothes_hands.blend")
REPORT = os.path.join(ROOT, "reports", "gate6_clothes_hands_report.json")
RENDER_DIR = os.path.join(ROOT, "renders", "lookdev", "gate6")
CHAR_COLLECTION = "COL_CHR_SLOTH_FINAL"
DETAIL_COLLECTION = "CLOTHING_DETAILS"


def ensure_dir(path):
    os.makedirs(path, exist_ok=True)


def set_input(node, name, value):
    socket = node.inputs.get(name)
    if socket is not None:
        socket.default_value = value


def material(name, color, roughness=0.70, specular=0.24):
    mat = bpy.data.materials.get(name) or bpy.data.materials.new(name)
    mat.use_nodes = True
    bsdf = next((n for n in mat.node_tree.nodes if n.type == "BSDF_PRINCIPLED"), None)
    if bsdf is None:
        mat.node_tree.nodes.clear()
        bsdf = mat.node_tree.nodes.new("ShaderNodeBsdfPrincipled")
        out = mat.node_tree.nodes.new("ShaderNodeOutputMaterial")
        mat.node_tree.links.new(bsdf.outputs["BSDF"], out.inputs["Surface"])
    set_input(bsdf, "Base Color", color)
    set_input(bsdf, "Roughness", roughness)
    set_input(bsdf, "Specular IOR Level", specular)
    set_input(bsdf, "Specular", specular)
    return mat


def attach_to_bone(obj, rig, bone_name):
    bpy.context.view_layer.update()
    world = obj.matrix_world.copy()
    obj.parent = rig
    obj.parent_type = "BONE"
    obj.parent_bone = bone_name
    obj.matrix_world = world


def move_to_collection(obj, collection):
    for owner in list(obj.users_collection):
        owner.objects.unlink(obj)
    collection.objects.link(obj)


def add_detail_curve(name, paths, bevel_depth, mat, collection, rig, bone="Spine2", cyclic_flags=None):
    curve = bpy.data.curves.new(name + "_Curve", "CURVE")
    curve.dimensions = "3D"
    curve.resolution_u = 2
    curve.bevel_depth = bevel_depth
    curve.bevel_resolution = 2
    curve.use_fill_caps = True
    curve.materials.append(mat)
    if cyclic_flags is None:
        cyclic_flags = [False] * len(paths)
    for coords, cyclic in zip(paths, cyclic_flags):
        spline = curve.splines.new("BEZIER")
        spline.bezier_points.add(len(coords) - 1)
        for point, co in zip(spline.bezier_points, coords):
            point.co = co
            point.handle_left_type = "AUTO"
            point.handle_right_type = "AUTO"
        spline.use_cyclic_u = cyclic
    obj = bpy.data.objects.new(name, curve)
    collection.objects.link(obj)
    attach_to_bone(obj, rig, bone)
    return obj


def rectangle_path(cx, y, cz, width, height):
    return [
        (cx - width / 2.0, y, cz - height / 2.0),
        (cx + width / 2.0, y, cz - height / 2.0),
        (cx + width / 2.0, y, cz + height / 2.0),
        (cx - width / 2.0, y, cz + height / 2.0),
    ]


def look_at(obj, point):
    obj.rotation_euler = (Vector(point) - obj.location).to_track_quat("-Z", "Y").to_euler()


def render_views(scene):
    ensure_dir(RENDER_DIR)
    cam = bpy.data.objects.get("CAM_LOOKDEV_65MM")
    close_cam = bpy.data.objects.get("CAM_LOOKDEV_100MM") or cam
    scene.view_layers[0].material_override = None
    scene.render.engine = "BLENDER_EEVEE"
    scene.render.resolution_x = 768
    scene.render.resolution_y = 768
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "PNG"
    outputs = {}
    for name, loc, target, lens in (
        ("front", (0.0, -8.40, 2.45), (0.0, 0.0, 1.30), 65),
        ("side", (8.40, 0.0, 2.45), (0.0, 0.0, 1.30), 65),
        ("back", (0.0, 8.40, 2.45), (0.0, 0.0, 1.30), 65),
    ):
        cam.location = loc
        cam.data.lens = lens
        look_at(cam, target)
        scene.camera = cam
        path = os.path.join(RENDER_DIR, name + ".png")
        scene.render.filepath = path
        bpy.ops.render.render(write_still=True)
        outputs[name] = path
    close_cam.location = (0.43, -2.45, 1.12)
    close_cam.data.lens = 100
    look_at(close_cam, (0.43, -0.02, 1.05))
    scene.camera = close_cam
    path = os.path.join(RENDER_DIR, "hand_closeup.png")
    scene.render.filepath = path
    bpy.ops.render.render(write_still=True)
    outputs["hand_closeup"] = path
    return outputs


ensure_dir(os.path.dirname(CHECKPOINT))
ensure_dir(os.path.dirname(REPORT))
char_collection = bpy.data.collections.get(CHAR_COLLECTION)
rig = next((obj for obj in bpy.data.objects if obj.type == "ARMATURE"), None)
main_obj = bpy.data.objects.get("part_00000001.001")
if char_collection is None or rig is None or main_obj is None:
    raise RuntimeError("Gate 6 prerequisites missing")
before_vertex_count = len(main_obj.data.vertices)
before_shape_keys = [key.name for key in main_obj.data.shape_keys.key_blocks]

detail_collection = bpy.data.collections.get(DETAIL_COLLECTION)
if detail_collection is None:
    detail_collection = bpy.data.collections.new(DETAIL_COLLECTION)
if detail_collection.name not in {child.name for child in char_collection.children}:
    char_collection.children.link(detail_collection)

removed_placeholders = []
placeholder_prefixes = (
    "CXR_ButtonOverlay", "CXR_DrawstringTip", "CXR_Ribbing.Front", "CXR_Ribbing.Back",
    "CXR_Seams.Cardigan", "CXR_Seams.Hood", "CXR_Seams.Trousers",
)
for obj in list(bpy.data.objects):
    if obj.name.startswith(placeholder_prefixes):
        removed_placeholders.append(obj.name)
        bpy.data.objects.remove(obj, do_unlink=True)

formalized_cuffs = {}
for old_name, new_name in (("CXR_Ribbing.Cuff.L", "CLO_Ribbing_Cuff_L"), ("CXR_Ribbing.Cuff.R", "CLO_Ribbing_Cuff_R")):
    obj = bpy.data.objects.get(old_name)
    if obj:
        obj.name = new_name
        obj.data.name = new_name + "_Curve"
        move_to_collection(obj, detail_collection)
        formalized_cuffs[old_name] = new_name

stitch_ivory = material("MAT_Stitch_Ivory", (0.58, 0.53, 0.46, 1.0), 0.76, 0.18)
stitch_shadow = material("MAT_Stitch_Shadow", (0.12, 0.075, 0.045, 1.0), 0.78, 0.16)
rib_ivory = material("MAT_Ribbing_Ivory", (0.48, 0.43, 0.36, 1.0), 0.82, 0.16)
nail_mat = material("MAT_Nail_Keratin", (0.60, 0.48, 0.32, 1.0), 0.42, 0.38)

placket_paths = [
    [(-0.145, -0.319, 1.10), (-0.145, -0.326, 1.38), (-0.135, -0.318, 1.70)],
    [(0.145, -0.319, 1.10), (0.145, -0.326, 1.38), (0.135, -0.318, 1.70)],
]
pocket_paths = [rectangle_path(-0.235, -0.326, 1.245, 0.165, 0.175), rectangle_path(0.235, -0.326, 1.245, 0.165, 0.175)]
hem_paths = [[(-0.335, -0.300, 1.075), (0.0, -0.337, 1.055), (0.335, -0.300, 1.075)]]
hood_paths = [[(-0.285, -0.275, 1.70), (-0.145, -0.345, 1.61), (0.0, -0.365, 1.585), (0.145, -0.345, 1.61), (0.285, -0.275, 1.70)]]
trouser_paths = [[(0.0, -0.303, 1.04), (0.0, -0.322, 0.82), (0.0, -0.307, 0.58)]]

created_details = []
created_details.append(add_detail_curve("CLO_Stitching_Placket", placket_paths, 0.00135, stitch_shadow, detail_collection, rig))
created_details.append(add_detail_curve("CLO_Stitching_Pockets", pocket_paths, 0.00125, stitch_ivory, detail_collection, rig, cyclic_flags=[True, True]))
created_details.append(add_detail_curve("CLO_Stitching_Hem", hem_paths, 0.00145, stitch_shadow, detail_collection, rig))
created_details.append(add_detail_curve("CLO_Stitching_Hood", hood_paths, 0.00125, stitch_ivory, detail_collection, rig))
created_details.append(add_detail_curve("CLO_Stitching_Trousers", trouser_paths, 0.00115, stitch_shadow, detail_collection, rig, bone="Hips"))

rib_paths = []
for layer in range(6):
    z = 1.055 + layer * 0.008
    rib_paths.append([(-0.33, -0.297 - layer * 0.001, z), (0.0, -0.333 - layer * 0.001, z - 0.004), (0.33, -0.297 - layer * 0.001, z)])
created_details.append(add_detail_curve("CLO_Ribbing_Hem", rib_paths, 0.00165, rib_ivory, detail_collection, rig))

formal_nails = []
for index in range(1, 4):
    for side in ("L", "R"):
        old_name = "CXR_Nail.%02d.%s" % (index, side)
        obj = bpy.data.objects.get(old_name)
        if obj is None:
            continue
        new_name = "GEO_Nail_%02d_%s" % (index, side)
        obj.name = new_name
        obj.data.name = new_name + "_Mesh"
        obj.scale.x *= 1.10
        obj.scale.y *= 1.08
        obj.scale.z *= 1.18
        if len(obj.data.materials):
            obj.data.materials[0] = nail_mat
        else:
            obj.data.materials.append(nail_mat)
        bevel = obj.modifiers.get("Nail softness") or obj.modifiers.new("Nail softness", "BEVEL")
        bevel.width = 0.0012
        bevel.segments = 3
        move_to_collection(obj, detail_collection)
        formal_nails.append(new_name)

scene = bpy.data.scenes.get("SCENE_LOOKDEV") or bpy.context.scene
bpy.context.window.scene = scene
renders = render_views(scene)
after_shape_keys = [key.name for key in main_obj.data.shape_keys.key_blocks]
checks = {
    "main_vertex_count_preserved": len(main_obj.data.vertices) == before_vertex_count == 5478,
    "shape_key_order_preserved": after_shape_keys == before_shape_keys,
    "single_armature": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]) == 1,
    "invalid_placeholders_removed": not any(obj.name.startswith(placeholder_prefixes) for obj in bpy.data.objects),
    "six_formal_nails": len(formal_nails) == 6,
    "clothing_details_created": len(created_details) == 6,
}
if not all(checks.values()):
    raise RuntimeError("Gate 6 invariant failed: " + json.dumps(checks))

bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT)
report = {
    "gate": 6,
    "status": "PASS",
    "source_checkpoint": os.path.join(ROOT, "checkpoints", "sloth_030_fur.blend"),
    "result_checkpoint": CHECKPOINT,
    "strategy": "Non-destructive bound-clothing detail overlays; no global Solidify on combined character mesh",
    "removed_invalid_placeholders": removed_placeholders,
    "formalized_cuffs": formalized_cuffs,
    "created_detail_objects": [obj.name for obj in created_details],
    "formal_nails": formal_nails,
    "checks": checks,
    "renders": renders,
    "notes": [
        "Main mesh combines anatomy and garments, so a global thickness modifier would be unsafe.",
        "Perceived garment construction is increased with placket, pocket, hood, hem and trouser seams plus six rib rows.",
        "Nails retain their existing bone parenting and topology; only transform scale, bevel and keratin material changed.",
        "Previously baked hand-wrist refinement remains in Basis and all expressions.",
    ],
}
with open(REPORT, "w", encoding="utf-8") as handle:
    json.dump(report, handle, ensure_ascii=False, indent=2)
print(json.dumps({"status": "PASS", "checkpoint": CHECKPOINT, "report": REPORT, "checks": checks, "renders": renders}, ensure_ascii=False))
