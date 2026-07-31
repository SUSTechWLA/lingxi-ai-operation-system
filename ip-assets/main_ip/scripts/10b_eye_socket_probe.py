import bpy
import json
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
REPORT = os.path.join(ROOT, "reports", "eye_socket_probe.json")
os.makedirs(os.path.dirname(REPORT), exist_ok=True)

head = bpy.data.objects.get("GEO_HeadBody")
if not head or head.type != "MESH":
    raise RuntimeError("GEO_HeadBody mesh not found")

world_points = [head.matrix_world @ v.co for v in head.data.vertices]


def percentile(values, p):
    values = sorted(values)
    if not values:
        return None
    idx = int(round((len(values) - 1) * p))
    return round(values[idx], 6)


socket_samples = {}
for side, cx in (("L", 0.128), ("R", -0.128)):
    region = [
        p for p in world_points
        if abs(p.x - cx) <= 0.095 and 2.105 <= p.z <= 2.290 and p.y < 0.0
    ]
    front_depths = [p.y for p in region]
    socket_samples[side] = {
        "vertex_count": len(region),
        "frontmost_y": round(min(front_depths), 6) if front_depths else None,
        "y_p10": percentile(front_depths, 0.10),
        "y_median": percentile(front_depths, 0.50),
        "y_p90": percentile(front_depths, 0.90),
        "rearmost_y": round(max(front_depths), 6) if front_depths else None,
        "closest_vertices": [
            [round(p.x, 6), round(p.y, 6), round(p.z, 6)]
            for p in sorted(region, key=lambda q: q.y)[:20]
        ],
    }


def serialize_driver(fcurve):
    driver = fcurve.driver
    variables = []
    for var in driver.variables:
        variables.append({
            "name": var.name,
            "type": var.type,
            "targets": [
                {
                    "id": target.id.name if target.id else None,
                    "data_path": target.data_path,
                    "bone_target": target.bone_target,
                    "transform_type": target.transform_type,
                    "transform_space": target.transform_space,
                }
                for target in var.targets
            ],
        })
    return {
        "data_path": fcurve.data_path,
        "array_index": fcurve.array_index,
        "expression": driver.expression,
        "type": driver.type,
        "variables": variables,
    }


lid_drivers = {}
for side in ("L", "R"):
    for part in ("Upper", "Lower"):
        obj = bpy.data.objects.get(f"EYE_Lid{part}_{side}")
        key = f"{part}_{side}"
        lid_drivers[key] = {
            "object": obj.name if obj else None,
            "shape_keys": [kb.name for kb in obj.data.shape_keys.key_blocks] if obj and obj.data.shape_keys else [],
            "vertex_count": len(obj.data.vertices) if obj and obj.type == "MESH" else 0,
            "polygon_count": len(obj.data.polygons) if obj and obj.type == "MESH" else 0,
            "first_polygons": [list(poly.vertices) for poly in list(obj.data.polygons)[:12]] if obj and obj.type == "MESH" else [],
            "drivers": [
                serialize_driver(fc)
                for fc in (
                    obj.data.shape_keys.animation_data.drivers
                    if obj and obj.data.shape_keys and obj.data.shape_keys.animation_data
                    else []
                )
            ],
        }

report = {
    "status": "PASS",
    "blend_file": bpy.data.filepath,
    "head_vertex_count": len(head.data.vertices),
    "socket_samples": socket_samples,
    "lid_drivers": lid_drivers,
}

with open(REPORT, "w", encoding="utf-8") as f:
    json.dump(report, f, ensure_ascii=False, indent=2)

print(json.dumps(report, ensure_ascii=False))
