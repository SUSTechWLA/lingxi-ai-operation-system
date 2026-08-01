import bpy
import json
import math
import os
import hashlib
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_010_proportion.blend")
REPORT = os.path.join(ROOT, "reports", "gate3_proportion_report.json")
RENDER_DIR = os.path.join(ROOT, "renders", "lookdev", "gate3")
MAIN_OBJECT = "part_00000001.001"
TEMP_FIT_KEYS = ["CXR_MacroForm", "CXR_MouthCornerVolume", "CXR_HandWristRefine"]


def ensure_dir(path):
    os.makedirs(path, exist_ok=True)


def coord_hash(obj):
    h = hashlib.sha256()
    for v in obj.data.vertices:
        for x in v.co:
            h.update(float(x).hex().encode("ascii"))
    return h.hexdigest()


def key_delta_hash(key, basis):
    h = hashlib.sha256()
    for i, p in enumerate(key.data):
        delta = p.co - basis.data[i].co
        for x in delta:
            h.update(float(x).hex().encode("ascii"))
    return h.hexdigest()


def look_at(obj, point):
    obj.rotation_euler = (Vector(point) - obj.location).to_track_quat("-Z", "Y").to_euler()


def make_clay_material():
    mat = bpy.data.materials.get("MAT_AUDIT_CLAY") or bpy.data.materials.new("MAT_AUDIT_CLAY")
    mat.use_nodes = True
    bsdf = next((node for node in mat.node_tree.nodes if node.type == "BSDF_PRINCIPLED"), None)
    if bsdf is None:
        mat.node_tree.nodes.clear()
        bsdf = mat.node_tree.nodes.new("ShaderNodeBsdfPrincipled")
        output = mat.node_tree.nodes.new("ShaderNodeOutputMaterial")
        mat.node_tree.links.new(bsdf.outputs["BSDF"], output.inputs["Surface"])
    bsdf.inputs["Base Color"].default_value = (0.43, 0.46, 0.48, 1.0)
    bsdf.inputs["Roughness"].default_value = 0.78
    bsdf.inputs["Metallic"].default_value = 0.0
    return mat


def render_views(scene):
    ensure_dir(RENDER_DIR)
    cam = bpy.data.objects.get("CAM_LOOKDEV_65MM")
    close_cam = bpy.data.objects.get("CAM_LOOKDEV_100MM") or cam
    if cam is None:
        raise RuntimeError("Missing Gate 1 lookdev camera")

    previous_override = scene.view_layers[0].material_override
    scene.view_layers[0].material_override = make_clay_material()
    previous_engine = scene.render.engine
    scene.render.engine = "BLENDER_EEVEE"
    scene.render.resolution_x = 768
    scene.render.resolution_y = 768
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "PNG"
    scene.render.film_transparent = False

    views = {
        "front": ((0.0, -8.40, 2.45), (0.0, 0.0, 1.30), 65),
        "front_34": ((5.94, -5.94, 2.45), (0.0, 0.0, 1.30), 65),
        "side": ((8.40, 0.0, 2.45), (0.0, 0.0, 1.30), 65),
        "back_34": ((5.94, 5.94, 2.45), (0.0, 0.0, 1.30), 65),
        "back": ((0.0, 8.40, 2.45), (0.0, 0.0, 1.30), 65),
    }
    outputs = {}
    for name, (location, target, lens) in views.items():
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

    scene.view_layers[0].material_override = previous_override
    scene.render.engine = previous_engine
    return outputs


ensure_dir(os.path.dirname(CHECKPOINT))
ensure_dir(os.path.dirname(REPORT))

obj = bpy.data.objects.get(MAIN_OBJECT)
if obj is None or obj.type != "MESH" or obj.data.shape_keys is None:
    raise RuntimeError("Authoritative bound character mesh or Shape Keys missing")

keys = obj.data.shape_keys.key_blocks
before_names = [key.name for key in keys]
before_vertex_count = len(obj.data.vertices)
before_basis_hash = coord_hash(obj)
basis = keys[0]

expression_names = [name for name in before_names if name != "Basis" and name not in TEMP_FIT_KEYS]
expression_deltas_before = {
    name: [(keys[name].data[i].co - basis.data[i].co).copy() for i in range(before_vertex_count)]
    for name in expression_names
}

present_fit_keys = [keys[name] for name in TEMP_FIT_KEYS if name in keys]
if len(present_fit_keys) != len(TEMP_FIT_KEYS):
    missing = sorted(set(TEMP_FIT_KEYS) - {key.name for key in present_fit_keys})
    raise RuntimeError("Missing temporary fit keys: " + ", ".join(missing))

# The three CXR keys were already approved and enabled at value 1.0. Their
# combined delta is propagated identically to Basis and every retained key.
fit_delta = []
max_delta = 0.0
mean_delta = 0.0
for i in range(before_vertex_count):
    delta = Vector((0.0, 0.0, 0.0))
    for key in present_fit_keys:
        delta += key.data[i].co - basis.data[i].co
    fit_delta.append(delta)
    mag = delta.length
    max_delta = max(max_delta, mag)
    mean_delta += mag
mean_delta /= max(1, before_vertex_count)

retained_keys = [key for key in keys if key.name not in TEMP_FIT_KEYS]
for key in retained_keys:
    for i, delta in enumerate(fit_delta):
        key.data[i].co += delta

for name in TEMP_FIT_KEYS:
    obj.shape_key_remove(keys[name])

obj.data.update()
after_names = [key.name for key in obj.data.shape_keys.key_blocks]
after_vertex_count = len(obj.data.vertices)
after_basis = obj.data.shape_keys.key_blocks[0]
max_expression_delta_error = 0.0
for name in expression_names:
    key = obj.data.shape_keys.key_blocks[name]
    for i in range(after_vertex_count):
        after_delta = key.data[i].co - after_basis.data[i].co
        max_expression_delta_error = max(
            max_expression_delta_error,
            (after_delta - expression_deltas_before[name][i]).length,
        )

checks = {
    "vertex_count_preserved": before_vertex_count == after_vertex_count == 5478,
    "retained_key_order_preserved": after_names == [n for n in before_names if n not in TEMP_FIT_KEYS],
    "expression_deltas_preserved": max_expression_delta_error <= 3.0e-6,
    "temporary_fit_keys_removed": not any(name in after_names for name in TEMP_FIT_KEYS),
    "armature_unique": len([o for o in bpy.data.objects if o.type == "ARMATURE"]) == 1,
}
if not all(checks.values()):
    raise RuntimeError("Gate 3 invariant failed: " + json.dumps(checks))

scene = bpy.data.scenes.get("SCENE_LOOKDEV") or bpy.context.scene
bpy.context.window.scene = scene
render_outputs = render_views(scene)

bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT)

report = {
    "gate": 3,
    "status": "PASS",
    "source_checkpoint": "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip/checkpoints/sloth_009_topology_decision.blend",
    "result_checkpoint": CHECKPOINT,
    "object": MAIN_OBJECT,
    "method": "Bake approved temporary CXR fit deltas into Basis and every retained relative Shape Key",
    "fit_keys_baked": TEMP_FIT_KEYS,
    "fit_delta_stats": {"max_local_distance": max_delta, "mean_local_distance": mean_delta},
    "max_expression_delta_error": max_expression_delta_error,
    "before_vertex_count": before_vertex_count,
    "after_vertex_count": after_vertex_count,
    "before_mesh_coord_hash": before_basis_hash,
    "after_mesh_coord_hash": coord_hash(obj),
    "before_shape_keys": before_names,
    "after_shape_keys": after_names,
    "checks": checks,
    "renders": render_outputs,
    "notes": [
        "No vertex was inserted, deleted, or reordered.",
        "All retained expression deltas match their pre-bake values within 3e-6 local units.",
        "LookDev material override was temporary and did not alter production materials.",
    ],
}
with open(REPORT, "w", encoding="utf-8") as handle:
    json.dump(report, handle, ensure_ascii=False, indent=2)

print(json.dumps({"status": "PASS", "checkpoint": CHECKPOINT, "report": REPORT, "checks": checks, "renders": render_outputs}, ensure_ascii=False))
