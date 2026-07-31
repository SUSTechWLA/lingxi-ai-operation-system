import bpy
import json
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_050_materials.blend")
REPORT = os.path.join(ROOT, "reports", "gate7_materials_report.json")
RENDER_DIR = os.path.join(ROOT, "renders", "lookdev", "gate7")


def ensure_dir(path):
    os.makedirs(path, exist_ok=True)


def principled(mat):
    if not mat or not mat.use_nodes or not mat.node_tree:
        return None
    return next((node for node in mat.node_tree.nodes if node.type == "BSDF_PRINCIPLED"), None)


def set_input(node, name, value):
    if node and node.inputs.get(name) is not None:
        node.inputs[name].default_value = value


def ensure_noise_bump(mat, source_node, strength, distance, label):
    nodes = mat.node_tree.nodes
    links = mat.node_tree.links
    bump = nodes.get(label) or nodes.new("ShaderNodeBump")
    bump.name = label
    bump.label = label
    set_input(bump, "Strength", strength)
    set_input(bump, "Distance", distance)
    for link in list(links):
        if link.to_node == bump and link.to_socket == bump.inputs.get("Height"):
            links.remove(link)
    links.new(source_node.outputs.get("Fac") or source_node.outputs[0], bump.inputs["Height"])
    bsdf = principled(mat)
    for link in list(links):
        if link.to_node == bsdf and link.to_socket == bsdf.inputs.get("Normal"):
            links.remove(link)
    links.new(bump.outputs["Normal"], bsdf.inputs["Normal"])
    return bump


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
        ("front_34", (5.94, -5.94, 2.45), (0.0, 0.0, 1.30), 65),
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
main_obj = bpy.data.objects.get("part_00000001.001")
if main_obj is None:
    raise RuntimeError("Main character mesh missing")
before_vertex_count = len(main_obj.data.vertices)
before_shape_keys = [key.name for key in main_obj.data.shape_keys.key_blocks]

rename_map = {
    "CXR_Character_Master_Material": "MAT_Character_Master",
    "CXR_EyeSocket": "MAT_EyeSocket",
    "IP_OralCavity_Material": "MAT_MouthInterior",
    "IP_Teeth_Material": "MAT_Teeth",
    "IP_Tongue_Material": "MAT_Tongue",
}
renamed = {}
for old_name, new_name in rename_map.items():
    mat = bpy.data.materials.get(old_name)
    if mat:
        mat.name = new_name
        mat["material_system"] = "SLOTH_FINAL"
        renamed[old_name] = new_name

master = bpy.data.materials.get("MAT_Character_Master")
if master is None or not master.use_nodes:
    raise RuntimeError("Master PBR material missing")
nodes = master.node_tree.nodes
noise = nodes.get("CXR_Micro_Noise")
bump = nodes.get("CXR_Micro_Bump")
rough_mix = nodes.get("CXR_Micro_RoughnessMix")
tone_mix = nodes.get("CXR_Micro_ToneMix")
rough_range = nodes.get("Roughness Range")
if noise:
    set_input(noise, "Scale", 205.0)
    set_input(noise, "Detail", 4.0)
    set_input(noise, "Roughness", 0.70)
    set_input(noise, "Distortion", 0.10)
if bump:
    set_input(bump, "Strength", 0.105)
    set_input(bump, "Distance", 0.00090)
if rough_mix:
    set_input(rough_mix, "Fac", 0.21)
    set_input(rough_mix, "Factor", 0.21)
if tone_mix:
    set_input(tone_mix, "Fac", 0.055)
    set_input(tone_mix, "Factor", 0.055)
if rough_range:
    set_input(rough_range, "To Min", 0.46)
    set_input(rough_range, "To Max", 0.82)
master_bsdf = principled(master)
set_input(master_bsdf, "Specular IOR Level", 0.26)
set_input(master_bsdf, "Specular", 0.26)
set_input(master_bsdf, "Coat Weight", 0.0)
master["covers_roles"] = "fur skin nose cardigan hoodie trousers buttons drawstrings shoes"

eye_socket = bpy.data.materials.get("MAT_EyeSocket")
set_input(principled(eye_socket), "Base Color", (0.045, 0.010, 0.0025, 1.0))
set_input(principled(eye_socket), "Roughness", 0.62)

sclera = bpy.data.materials.get("MAT_Eye_Sclera")
set_input(principled(sclera), "Base Color", (0.58, 0.53, 0.45, 1.0))
set_input(principled(sclera), "Roughness", 0.27)
set_input(principled(sclera), "Specular IOR Level", 0.46)
set_input(principled(sclera), "Specular", 0.46)

iris = bpy.data.materials.get("MAT_Eye_Iris_Amber")
if iris and iris.use_nodes:
    iris_noise = next((node for node in iris.node_tree.nodes if node.type == "TEX_NOISE"), None)
    if iris_noise:
        set_input(iris_noise, "Scale", 22.0)
        set_input(iris_noise, "Detail", 6.0)
        ensure_noise_bump(iris, iris_noise, 0.12, 0.0020, "Iris micro relief")

cornea = bpy.data.materials.get("MAT_Eye_Cornea")
cornea_bsdf = principled(cornea)
set_input(cornea_bsdf, "Roughness", 0.035)
set_input(cornea_bsdf, "IOR", 1.38)
set_input(cornea_bsdf, "Transmission Weight", 0.90)
set_input(cornea_bsdf, "Transmission", 0.90)
set_input(cornea_bsdf, "Alpha", 0.22)
if cornea:
    cornea.diffuse_color = (0.72, 0.84, 0.92, 0.22)

pupil = bpy.data.materials.get("MAT_Eye_Pupil")
set_input(principled(pupil), "Base Color", (0.003, 0.0012, 0.0005, 1.0))
set_input(principled(pupil), "Roughness", 0.16)

mouth = bpy.data.materials.get("MAT_MouthInterior")
set_input(principled(mouth), "Base Color", (0.018, 0.0025, 0.004, 1.0))
set_input(principled(mouth), "Roughness", 0.43)
teeth = bpy.data.materials.get("MAT_Teeth")
set_input(principled(teeth), "Base Color", (0.72, 0.60, 0.43, 1.0))
set_input(principled(teeth), "Roughness", 0.34)
tongue = bpy.data.materials.get("MAT_Tongue")
set_input(principled(tongue), "Base Color", (0.25, 0.035, 0.045, 1.0))
set_input(principled(tongue), "Roughness", 0.46)

nail = bpy.data.materials.get("MAT_Nail_Keratin")
set_input(principled(nail), "Base Color", (0.57, 0.45, 0.30, 1.0))
set_input(principled(nail), "Roughness", 0.39)

for mat_name in ("MAT_Fur_RootBrown", "MAT_Fur_TipBrown", "MAT_Fur_Ivory"):
    mat = bpy.data.materials.get(mat_name)
    if mat:
        mat["material_system"] = "SLOTH_FINAL"
        set_input(principled(mat), "Specular IOR Level", 0.20)
        set_input(principled(mat), "Specular", 0.20)

removed_legacy_catchlights = []
for obj in list(bpy.data.objects):
    if obj.name.startswith(("CXR_Catchlight", "CXR_CatchlightSecondary")):
        removed_legacy_catchlights.append(obj.name)
        bpy.data.objects.remove(obj, do_unlink=True)

scene = bpy.data.scenes.get("SCENE_LOOKDEV") or bpy.context.scene
bpy.context.window.scene = scene
renders = render_views(scene)
after_shape_keys = [key.name for key in main_obj.data.shape_keys.key_blocks]
required_materials = [
    "MAT_Character_Master", "MAT_EyeSocket", "MAT_Eye_Sclera", "MAT_Eye_Iris_Amber",
    "MAT_Eye_Pupil", "MAT_Eye_Cornea", "MAT_MouthInterior", "MAT_Teeth", "MAT_Tongue",
    "MAT_Fur_RootBrown", "MAT_Fur_TipBrown", "MAT_Fur_Ivory", "MAT_Nail_Keratin",
]
checks = {
    "main_vertex_count_preserved": len(main_obj.data.vertices) == before_vertex_count == 5478,
    "shape_key_order_preserved": after_shape_keys == before_shape_keys,
    "single_armature": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]) == 1,
    "required_materials_present": all(bpy.data.materials.get(name) is not None for name in required_materials),
    "legacy_catchlights_removed": not any(obj.name.startswith("CXR_Catchlight") for obj in bpy.data.objects),
    "master_uv_images_preserved": all(nodes.get(name) is not None and nodes.get(name).image is not None for name in ("Image Texture - Base Color", "Image Texture - Normal", "Image Texture - Roughness")),
}
if not all(checks.values()):
    raise RuntimeError("Gate 7 invariant failed: " + json.dumps(checks))

bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT)
report = {
    "gate": 7,
    "status": "PASS",
    "source_checkpoint": os.path.join(ROOT, "checkpoints", "sloth_040_clothes_hands.blend"),
    "result_checkpoint": CHECKPOINT,
    "renamed_materials": renamed,
    "required_materials": required_materials,
    "removed_legacy_catchlights": removed_legacy_catchlights,
    "master_microdetail": {"noise_scale": 205.0, "noise_detail": 4.0, "bump_strength": 0.105, "bump_distance": 0.00090, "roughness_mix": 0.21},
    "checks": checks,
    "renders": renders,
    "notes": [
        "Original UVs and all PBR image textures remain connected.",
        "Master material microdetail now carries fabric/fur-scale normal and roughness breakup without changing render samples.",
        "Sclera, teeth and light fur use warm off-whites; pupils and mouth interior use lifted near-blacks.",
        "Only the formal EYE_Catchlight_L/R system remains.",
    ],
}
with open(REPORT, "w", encoding="utf-8") as handle:
    json.dump(report, handle, ensure_ascii=False, indent=2)
print(json.dumps({"status": "PASS", "checkpoint": CHECKPOINT, "report": REPORT, "checks": checks, "renders": renders}, ensure_ascii=False))
