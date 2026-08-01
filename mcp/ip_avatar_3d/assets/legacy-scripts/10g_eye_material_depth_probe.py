import bpy
import json
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
REPORT = os.path.join(ROOT, "reports", "eye_material_depth_probe.json")
os.makedirs(os.path.dirname(REPORT), exist_ok=True)


def input_value(sock):
    try:
        value = sock.default_value
        if hasattr(value, "__len__") and not isinstance(value, str):
            return [round(float(v), 6) for v in value]
        return round(float(value), 6)
    except Exception:
        return None


def material_record(material):
    if not material:
        return None
    record = {
        "name": material.name,
        "use_nodes": material.use_nodes,
        "diffuse_color": [round(v, 6) for v in material.diffuse_color],
        "blend_method": getattr(material, "blend_method", None),
        "surface_render_method": getattr(material, "surface_render_method", None),
        "show_transparent_back": getattr(material, "show_transparent_back", None),
        "nodes": [],
    }
    if material.use_nodes:
        for node in material.node_tree.nodes:
            rec = {"name": node.name, "type": node.type, "inputs": {}}
            for sock in node.inputs:
                if not sock.is_linked:
                    value = input_value(sock)
                    if value is not None:
                        rec["inputs"][sock.name] = value
            record["nodes"].append(rec)
    return record


def bounds(obj):
    pts = [obj.matrix_world @ Vector(corner) for corner in obj.bound_box]
    return {
        "min": [round(min(p[i] for p in pts), 6) for i in range(3)],
        "max": [round(max(p[i] for p in pts), 6) for i in range(3)],
    }


objects = {}
for name in (
    "EYE_Sclera_L", "EYE_Cornea_L", "EYE_Iris_L", "EYE_IrisLimbal_L",
    "EYE_Pupil_L", "EYE_Catchlight_L", "EYE_LidUpper_L", "EYE_LidLower_L",
):
    obj = bpy.data.objects.get(name)
    if obj:
        objects[name] = {
            "type": obj.type,
            "world_location": [round(v, 6) for v in obj.matrix_world.translation],
            "dimensions": [round(v, 6) for v in obj.dimensions],
            "bounds": bounds(obj),
            "materials": [slot.material.name if slot.material else None for slot in obj.material_slots],
            "hide_render": obj.hide_render,
            "parent": obj.parent.name if obj.parent else None,
            "parent_bone": obj.parent_bone,
            "vertex_count": len(obj.data.vertices) if obj.type == "MESH" else None,
        }

materials = {
    name: material_record(bpy.data.materials.get(name))
    for name in (
        "MAT_Eye_Cornea", "MAT_Eye_Pupil", "MAT_Eye_Iris_Amber",
        "MAT_Eye_Iris_Limbal", "MAT_Eye_Catchlight", "MAT_Eyelid_Skin_Brown",
    )
}

report = {
    "status": "PASS",
    "blend_file": bpy.data.filepath,
    "objects": objects,
    "materials": materials,
}
with open(REPORT, "w", encoding="utf-8") as f:
    json.dump(report, f, ensure_ascii=False, indent=2)
print(json.dumps(report, ensure_ascii=False))
