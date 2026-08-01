import bpy
import json
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_087_eye_bugfix_final_candidate_v7.blend")
REPORT = os.path.join(ROOT, "reports", "eye_bugfix_final_candidate_v7.json")
RENDER_DIR = os.path.join(ROOT, "renders", "eye_bugfix", "candidate_v7")
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
    if obj.name.startswith(("TMP_EyeBug", "TMP_EyeLayer")):
        data = obj.data
        bpy.data.objects.remove(obj, do_unlink=True)
        if data and data.users == 0:
            if isinstance(data, bpy.types.Camera):
                bpy.data.cameras.remove(data)
            elif isinstance(data, bpy.types.Light):
                bpy.data.lights.remove(data)

bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT, copy=False)

cornea_mat = bpy.data.materials.get("MAT_Eye_Cornea")
if not cornea_mat:
    raise RuntimeError("Cornea material missing")
cornea_mat.use_nodes = True
try:
    cornea_mat.blend_method = "BLEND"
except Exception:
    pass
try:
    cornea_mat.surface_render_method = "BLENDED"
except Exception:
    pass
try:
    cornea_mat.show_transparent_back = False
except Exception:
    pass
cornea_mat.diffuse_color = (0.9, 0.95, 1.0, 0.08)

nt = cornea_mat.node_tree
nt.nodes.clear()
out = nt.nodes.new("ShaderNodeOutputMaterial")
transparent = nt.nodes.new("ShaderNodeBsdfTransparent")
transparent.inputs["Color"].default_value = (1.0, 1.0, 1.0, 1.0)
reflective = nt.nodes.new("ShaderNodeBsdfPrincipled")
reflective.inputs["Base Color"].default_value = (0.002, 0.002, 0.002, 1.0)
reflective.inputs["Roughness"].default_value = 0.025
weight = reflective.inputs.get("Weight")
if weight:
    weight.default_value = 1.0
spec = reflective.inputs.get("Specular IOR Level") or reflective.inputs.get("Specular")
if spec:
    spec.default_value = 0.85
coat = reflective.inputs.get("Coat Weight") or reflective.inputs.get("Clearcoat")
if coat:
    coat.default_value = 0.35
coat_rough = reflective.inputs.get("Coat Roughness") or reflective.inputs.get("Clearcoat Roughness")
if coat_rough:
    coat_rough.default_value = 0.015
mix = nt.nodes.new("ShaderNodeMixShader")
mix.inputs[0].default_value = 0.08
nt.links.new(transparent.outputs["BSDF"], mix.inputs[1])
nt.links.new(reflective.outputs["BSDF"], mix.inputs[2])
nt.links.new(mix.outputs["Shader"], out.inputs["Surface"])

for side in ("L", "R"):
    cornea = bpy.data.objects.get(f"EYE_Cornea_{side}")
    if not cornea:
        raise RuntimeError(f"Missing cornea {side}")
    cornea.hide_render = False
    cornea.data.materials.clear()
    cornea.data.materials.append(cornea_mat)

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
    "shape_keys_before": shape_key_names_before,
    "shape_keys_after": [kb.name for kb in head.data.shape_keys.key_blocks],
    "lid_driver_count": lid_driver_count,
    "same_lid_material": same_material,
    "cornea_shader": "92_PERCENT_TRANSPARENT_8_PERCENT_REFLECTIVE",
    "cornea_backface_visible": getattr(cornea_mat, "show_transparent_back", None),
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
    raise RuntimeError("Candidate v7 invariants failed")
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
