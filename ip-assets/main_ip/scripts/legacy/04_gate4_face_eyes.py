import bpy
import json
import math
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_020_face_eyes.blend")
REPORT = os.path.join(ROOT, "reports", "gate4_face_eyes_report.json")
RENDER_DIR = os.path.join(ROOT, "renders", "lookdev", "gate4")
MAIN_OBJECT = "part_00000001.001"
COLLECTION = "COL_CHR_SLOTH_FINAL"


def ensure_dir(path):
    os.makedirs(path, exist_ok=True)


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


def smooth_mesh(obj):
    if obj.type == "MESH":
        for polygon in obj.data.polygons:
            polygon.use_smooth = True


def principled_node(mat):
    mat.use_nodes = True
    node = next((n for n in mat.node_tree.nodes if n.type == "BSDF_PRINCIPLED"), None)
    if node is None:
        mat.node_tree.nodes.clear()
        node = mat.node_tree.nodes.new("ShaderNodeBsdfPrincipled")
        output = mat.node_tree.nodes.new("ShaderNodeOutputMaterial")
        mat.node_tree.links.new(node.outputs["BSDF"], output.inputs["Surface"])
    return node


def set_input(node, name, value):
    socket = node.inputs.get(name)
    if socket is not None:
        socket.default_value = value


def simple_material(name, color, roughness=0.45, specular=0.45, metallic=0.0):
    mat = bpy.data.materials.get(name) or bpy.data.materials.new(name)
    node = principled_node(mat)
    set_input(node, "Base Color", color)
    set_input(node, "Roughness", roughness)
    set_input(node, "Metallic", metallic)
    set_input(node, "Specular IOR Level", specular)
    set_input(node, "Specular", specular)
    return mat


def iris_material():
    name = "MAT_Eye_Iris_Amber"
    mat = bpy.data.materials.get(name) or bpy.data.materials.new(name)
    mat.use_nodes = True
    nodes = mat.node_tree.nodes
    links = mat.node_tree.links
    nodes.clear()
    out = nodes.new("ShaderNodeOutputMaterial")
    bsdf = nodes.new("ShaderNodeBsdfPrincipled")
    tex = nodes.new("ShaderNodeTexNoise")
    ramp = nodes.new("ShaderNodeValToRGB")
    coord = nodes.new("ShaderNodeTexCoord")
    tex.inputs["Scale"].default_value = 18.0
    tex.inputs["Detail"].default_value = 5.0
    tex.inputs["Roughness"].default_value = 0.68
    ramp.color_ramp.elements[0].position = 0.20
    ramp.color_ramp.elements[0].color = (0.012, 0.003, 0.001, 1.0)
    ramp.color_ramp.elements[1].position = 0.78
    ramp.color_ramp.elements[1].color = (0.14, 0.030, 0.002, 1.0)
    warm = ramp.color_ramp.elements.new(0.52)
    warm.color = (0.050, 0.010, 0.0012, 1.0)
    set_input(bsdf, "Roughness", 0.31)
    set_input(bsdf, "Specular IOR Level", 0.48)
    set_input(bsdf, "Specular", 0.48)
    links.new(coord.outputs["Generated"], tex.inputs["Vector"])
    links.new(tex.outputs["Fac"], ramp.inputs["Fac"])
    links.new(ramp.outputs["Color"], bsdf.inputs["Base Color"])
    links.new(bsdf.outputs["BSDF"], out.inputs["Surface"])
    return mat


def cornea_material():
    mat = bpy.data.materials.get("MAT_Eye_Cornea") or bpy.data.materials.new("MAT_Eye_Cornea")
    node = principled_node(mat)
    set_input(node, "Base Color", (0.82, 0.92, 1.0, 1.0))
    set_input(node, "Roughness", 0.06)
    set_input(node, "IOR", 1.38)
    set_input(node, "Transmission Weight", 0.82)
    set_input(node, "Transmission", 0.82)
    set_input(node, "Alpha", 0.28)
    mat.diffuse_color = (0.82, 0.92, 1.0, 0.28)
    if hasattr(mat, "blend_method"):
        mat.blend_method = "BLEND"
    if hasattr(mat, "use_screen_refraction"):
        mat.use_screen_refraction = True
    return mat


def add_uv_sphere(name, location, scale, material, collection, rig, bone):
    bpy.ops.mesh.primitive_uv_sphere_add(segments=64, ring_count=32, location=location)
    obj = bpy.context.object
    obj.name = name
    obj.scale = scale
    bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)
    smooth_mesh(obj)
    obj.data.materials.append(material)
    move_to_collection(obj, collection)
    attach_to_bone(obj, rig, bone)
    return obj


def add_disc(name, location, radius, depth, material, collection, rig, bone):
    bpy.ops.mesh.primitive_cylinder_add(vertices=64, radius=radius, depth=depth, location=location, rotation=(math.pi / 2.0, 0.0, 0.0))
    obj = bpy.context.object
    obj.name = name
    bpy.ops.object.transform_apply(location=False, rotation=True, scale=True)
    smooth_mesh(obj)
    bevel = obj.modifiers.new("Soft edge", "BEVEL")
    bevel.width = min(radius * 0.08, 0.003)
    bevel.segments = 3
    obj.data.materials.append(material)
    move_to_collection(obj, collection)
    attach_to_bone(obj, rig, bone)
    return obj


def add_lid(name, center, side, upper, material, collection, rig, controller_keys):
    cx, cy, cz = center
    rx = 0.078
    rz = 0.068 if upper else 0.055
    thickness = 0.010 if upper else 0.005
    samples = 40
    vertices = []
    for i in range(samples):
        u = -1.0 + 2.0 * i / (samples - 1)
        arch = math.sqrt(max(0.0, 1.0 - u * u))
        edge = (rz * arch) if upper else (-rz * arch)
        inner = edge - thickness * max(0.25, arch) if upper else edge + thickness * max(0.25, arch)
        ywrap = -0.004 + 0.014 * (u * u)
        vertices.append((rx * u, ywrap, edge))
        vertices.append((rx * u, ywrap + 0.002, inner))
    faces = []
    for i in range(samples - 1):
        a = 2 * i
        faces.append((a, a + 1, a + 3, a + 2))
    mesh = bpy.data.meshes.new(name + "_Mesh")
    mesh.from_pydata(vertices, [], faces)
    mesh.update()
    obj = bpy.data.objects.new(name, mesh)
    collection.objects.link(obj)
    obj.location = (cx, cy, cz)
    smooth_mesh(obj)
    obj.data.materials.append(material)
    solid = obj.modifiers.new("Lid thickness", "SOLIDIFY")
    solid.thickness = 0.0015
    solid.offset = 0.0
    bevel = obj.modifiers.new("Lid softness", "BEVEL")
    bevel.width = 0.0008
    bevel.segments = 2

    basis = obj.shape_key_add(name="Basis")
    blink = obj.shape_key_add(name="Blink")
    wide = obj.shape_key_add(name="Wide")
    for i in range(samples):
        u = -1.0 + 2.0 * i / (samples - 1)
        small_arch = math.sqrt(max(0.0, 1.0 - u * u))
        idx = 2 * i
        if upper:
            # Keep the outer/top edge on the orbital rim and pull only the
            # inner edge down to the closure line, so the lid covers the
            # upper half of the globe instead of collapsing into a thin bar.
            blink.data[idx + 1].co.z = -0.008 * small_arch
            wide.data[idx].co.z += 0.026 * small_arch
            wide.data[idx + 1].co.z += 0.019 * small_arch
        else:
            # Mirror the upper-lid behavior for a complete lower-half cover.
            blink.data[idx + 1].co.z = -0.008 * small_arch
            wide.data[idx].co.z -= 0.014 * small_arch
            wide.data[idx + 1].co.z -= 0.010 * small_arch

    for key_name, controller_name in (("Blink", controller_keys[0]), ("Wide", controller_keys[1])):
        fcurve = obj.data.shape_keys.key_blocks[key_name].driver_add("value")
        driver = fcurve.driver
        driver.type = "SUM"
        variable = driver.variables.new()
        variable.name = "ctrl"
        variable.type = "SINGLE_PROP"
        target = variable.targets[0]
        target.id_type = "KEY"
        target.id = bpy.data.objects[MAIN_OBJECT].data.shape_keys
        target.data_path = 'key_blocks["%s"].value' % controller_name

    attach_to_bone(obj, rig, "Head")
    return obj


def add_orbital_rim(name, center, material, collection, rig):
    cx, cy, cz = center
    curve = bpy.data.curves.new(name + "_Curve", "CURVE")
    curve.dimensions = "3D"
    curve.resolution_u = 2
    curve.bevel_depth = 0.0028
    curve.bevel_resolution = 4
    spline = curve.splines.new("NURBS")
    samples = 48
    spline.points.add(samples - 1)
    for i, point in enumerate(spline.points):
        angle = 2.0 * math.pi * i / samples
        point.co = (0.079 * math.cos(angle), 0.0, 0.085 * math.sin(angle), 1.0)
    spline.use_cyclic_u = True
    spline.order_u = 3
    obj = bpy.data.objects.new(name, curve)
    collection.objects.link(obj)
    obj.location = (cx, cy, cz)
    obj.data.materials.append(material)
    attach_to_bone(obj, rig, "Head")
    return obj


def add_tearline(name, center, material, collection, rig):
    cx, cy, cz = center
    curve = bpy.data.curves.new(name + "_Curve", "CURVE")
    curve.dimensions = "3D"
    curve.resolution_u = 16
    curve.bevel_depth = 0.0031
    curve.bevel_resolution = 4
    spline = curve.splines.new("BEZIER")
    spline.bezier_points.add(4)
    points = [(-0.070, -0.004, -0.022), (-0.038, -0.006, -0.049), (0.0, -0.007, -0.058), (0.038, -0.006, -0.049), (0.070, -0.004, -0.022)]
    for bp, co in zip(spline.bezier_points, points):
        bp.co = co
        bp.handle_left_type = "AUTO"
        bp.handle_right_type = "AUTO"
    obj = bpy.data.objects.new(name, curve)
    collection.objects.link(obj)
    obj.location = (cx, cy, cz)
    obj.data.materials.append(material)
    attach_to_bone(obj, rig, "Head")
    return obj


def ensure_controller_key(obj, name, source=None, scale=1.0):
    keys = obj.data.shape_keys.key_blocks
    if name in keys:
        return keys[name]
    new_key = obj.shape_key_add(name=name)
    basis = keys[0]
    if source and source in keys:
        src = keys[source]
        for i in range(len(obj.data.vertices)):
            new_key.data[i].co = basis.data[i].co + scale * (src.data[i].co - basis.data[i].co)
    return new_key


def reset_expression_values(keys):
    for key in keys:
        if key.name == "Basis":
            continue
        key.value = 1.0 if key.name == "Mouth_Rest" else 0.0


def look_at(obj, point):
    obj.rotation_euler = (Vector(point) - obj.location).to_track_quat("-Z", "Y").to_euler()


def render_expression_tests(scene, main_obj):
    ensure_dir(RENDER_DIR)
    cam = bpy.data.objects.get("CAM_LOOKDEV_100MM") or bpy.data.objects.get("CAM_LOOKDEV_65MM")
    if cam is None:
        raise RuntimeError("LookDev face camera missing")
    cam.location = (0.0, -4.63, 2.30)
    cam.data.lens = 100
    look_at(cam, (0.0, -0.02, 2.22))
    scene.camera = cam
    scene.view_layers[0].material_override = None
    scene.render.engine = "BLENDER_EEVEE"
    scene.render.resolution_x = 768
    scene.render.resolution_y = 768
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "PNG"
    keys = main_obj.data.shape_keys.key_blocks
    tests = {
        "neutral": {},
        "blink": {"Blink.L": 1.0, "Blink.R": 1.0},
        "smile": {"Mouth_Smile": 1.0, "CheekRaise.L": 0.65, "CheekRaise.R": 0.65},
        "wide_eyes": {"Eye_Wide.L": 1.0, "Eye_Wide.R": 1.0},
        "mouth_open": {"JawOpen": 1.0, "Mouth_A": 0.75},
    }
    outputs = {}
    for name, values in tests.items():
        reset_expression_values(keys)
        for key_name, value in values.items():
            if key_name in keys:
                keys[key_name].value = value
        scene.frame_set(scene.frame_current)
        path = os.path.join(RENDER_DIR, name + ".png")
        scene.render.filepath = path
        bpy.ops.render.render(write_still=True)
        outputs[name] = path
    reset_expression_values(keys)
    scene.frame_set(scene.frame_current)
    return outputs


ensure_dir(os.path.dirname(CHECKPOINT))
ensure_dir(os.path.dirname(REPORT))
collection = bpy.data.collections.get(COLLECTION)
main_obj = bpy.data.objects.get(MAIN_OBJECT)
rig = next((obj for obj in bpy.data.objects if obj.type == "ARMATURE"), None)
if collection is None or main_obj is None or rig is None:
    raise RuntimeError("Gate 4 prerequisites missing")

before_vertex_count = len(main_obj.data.vertices)
before_key_names = [key.name for key in main_obj.data.shape_keys.key_blocks]
old_eye_tokens = ("CXR_Eye", "CXR_Cornea", "CXR_Iris", "CXR_Pupil", "CXR_Lid", "CXR_Tearline")
old_eye_objects = [obj for obj in list(bpy.data.objects) if obj.name.startswith(old_eye_tokens)]
removed_names = [obj.name for obj in old_eye_objects]
for obj in old_eye_objects:
    bpy.data.objects.remove(obj, do_unlink=True)

sclera_mat = simple_material("MAT_Eye_Sclera", (0.58, 0.54, 0.47, 1.0), roughness=0.30, specular=0.46)
iris_mat = iris_material()
pupil_mat = simple_material("MAT_Eye_Pupil", (0.006, 0.003, 0.0015, 1.0), roughness=0.18, specular=0.52)
cornea_mat = cornea_material()
lid_mat = simple_material("MAT_Eyelid_WarmBrown", (0.035, 0.009, 0.003, 1.0), roughness=0.67, specular=0.28)
lower_lid_mat = simple_material("MAT_Eyelid_Lower", (0.070, 0.022, 0.009, 1.0), roughness=0.72, specular=0.24)
tear_mat = simple_material("MAT_TearLine", (0.38, 0.30, 0.23, 1.0), roughness=0.12, specular=0.62)
catchlight_mat = simple_material("MAT_Eye_Catchlight", (0.95, 0.90, 0.78, 1.0), roughness=0.04, specular=0.70)
catchlight_bsdf = principled_node(catchlight_mat)
set_input(catchlight_bsdf, "Emission Color", (0.52, 0.46, 0.34, 1.0))
set_input(catchlight_bsdf, "Emission", (0.52, 0.46, 0.34, 1.0))
set_input(catchlight_bsdf, "Emission Strength", 0.30)

ensure_controller_key(main_obj, "Blink.L", "Eye_Squint.L", 1.30)
ensure_controller_key(main_obj, "Blink.R", "Eye_Squint.R", 1.30)
ensure_controller_key(main_obj, "CheekRaise.L", "Cheek_Smile.L", 1.10)
ensure_controller_key(main_obj, "CheekRaise.R", "Cheek_Smile.R", 1.10)
ensure_controller_key(main_obj, "JawOpen", "Mouth_A", 1.15)

new_objects = []
eye_specs = {
    "L": (0.128, -0.205, 2.198, "Eye.L", "Blink.L", "Eye_Wide.L"),
    "R": (-0.128, -0.205, 2.198, "Eye.R", "Blink.R", "Eye_Wide.R"),
}
for side, (cx, cy, cz, eye_bone, blink_ctrl, wide_ctrl) in eye_specs.items():
    center = (cx, cy, cz)
    new_objects.append(add_uv_sphere("EYE_Sclera_" + side, center, (0.070, 0.062, 0.082), sclera_mat, collection, rig, eye_bone))
    new_objects.append(add_disc("EYE_Iris_" + side, (cx, cy - 0.0635, cz), 0.036, 0.0028, iris_mat, collection, rig, eye_bone))
    new_objects.append(add_disc("EYE_Pupil_" + side, (cx, cy - 0.0652, cz), 0.017, 0.0030, pupil_mat, collection, rig, eye_bone))
    new_objects.append(add_uv_sphere("EYE_Cornea_" + side, (cx, cy - 0.001, cz), (0.071, 0.063, 0.083), cornea_mat, collection, rig, eye_bone))
    new_objects.append(add_lid("EYE_LidUpper_" + side, (cx, cy - 0.079, cz), side, True, lid_mat, collection, rig, (blink_ctrl, wide_ctrl)))
    new_objects.append(add_lid("EYE_LidLower_" + side, (cx, cy - 0.079, cz), side, False, lower_lid_mat, collection, rig, (blink_ctrl, wide_ctrl)))
    new_objects.append(add_tearline("EYE_TearLine_" + side, (cx, cy - 0.082, cz), tear_mat, collection, rig))
    new_objects.append(add_disc("EYE_Catchlight_" + side, (cx + 0.011, cy - 0.0672, cz + 0.013), 0.0062, 0.0010, catchlight_mat, collection, rig, eye_bone))

scene = bpy.data.scenes.get("SCENE_LOOKDEV") or bpy.context.scene
bpy.context.window.scene = scene
render_outputs = render_expression_tests(scene, main_obj)

after_key_names = [key.name for key in main_obj.data.shape_keys.key_blocks]
checks = {
    "main_vertex_count_preserved": len(main_obj.data.vertices) == before_vertex_count == 5478,
    "existing_shape_key_order_preserved": after_key_names[:len(before_key_names)] == before_key_names,
    "new_controls_appended": all(name in after_key_names for name in ("Blink.L", "Blink.R", "CheekRaise.L", "CheekRaise.R", "JawOpen")),
    "single_armature": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]) == 1,
    "old_eye_system_removed": not any(obj.name.startswith(old_eye_tokens) for obj in bpy.data.objects),
    "formal_eye_layers_complete": all(bpy.data.objects.get(name) is not None for name in (
        "EYE_Sclera_L", "EYE_Sclera_R", "EYE_Iris_L", "EYE_Iris_R", "EYE_Pupil_L", "EYE_Pupil_R",
        "EYE_Cornea_L", "EYE_Cornea_R", "EYE_TearLine_L", "EYE_TearLine_R",
        "EYE_LidUpper_L", "EYE_LidUpper_R", "EYE_LidLower_L", "EYE_LidLower_R",
    )),
}
if not all(checks.values()):
    raise RuntimeError("Gate 4 invariant failed: " + json.dumps(checks))

bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT)
report = {
    "gate": 4,
    "status": "PASS",
    "source_checkpoint": os.path.join(ROOT, "checkpoints", "sloth_010_proportion.blend"),
    "result_checkpoint": CHECKPOINT,
    "removed_legacy_eye_objects": removed_names,
    "created_eye_objects": [obj.name for obj in new_objects],
    "eye_design": {
        "sclera_centers": {side: list(spec[:3]) for side, spec in eye_specs.items()},
        "sclera_scale": [0.070, 0.062, 0.082],
        "front_surface_y": -0.267,
        "iris_y": -0.2685,
        "lid_front_y": -0.284,
        "principle": "Globes recessed behind orbital plane; eyelid ribbons sit in front and overlap sclera margins",
    },
    "before_shape_keys": before_key_names,
    "after_shape_keys": after_key_names,
    "checks": checks,
    "renders": render_outputs,
    "notes": [
        "No bound-mesh vertex was added, removed, or reordered.",
        "Amber iris uses procedural multi-tone structure rather than a saturated orange ring.",
        "Upper and lower lids have driven Blink and Wide shapes; globes remain independently bone-parented.",
        "Mouth and cheek controls were appended by deriving deltas from existing approved expressions.",
    ],
}
with open(REPORT, "w", encoding="utf-8") as handle:
    json.dump(report, handle, ensure_ascii=False, indent=2)

print(json.dumps({"status": "PASS", "checkpoint": CHECKPOINT, "report": REPORT, "checks": checks, "renders": render_outputs}, ensure_ascii=False))
