import bpy
import json
import os
from mathutils import Vector


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
FINAL_BLEND = os.path.join(ROOT, "character_sloth_final.blend")
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_090_pre_reference_refine.blend")
REPORT = os.path.join(ROOT, "reports", "reference_refine_audit.json")


def world_bounds(obj):
    points = [obj.matrix_world @ Vector(corner) for corner in obj.bound_box]
    return {
        "min": [round(min(p[i] for p in points), 6) for i in range(3)],
        "max": [round(max(p[i] for p in points), 6) for i in range(3)],
    }


def object_record(obj):
    record = {
        "name": obj.name,
        "type": obj.type,
        "parent": obj.parent.name if obj.parent else None,
        "parent_type": obj.parent_type if obj.parent else None,
        "parent_bone": obj.parent_bone if obj.parent_type == "BONE" else None,
        "collections": sorted(c.name for c in obj.users_collection),
        "location": [round(v, 6) for v in obj.location],
        "dimensions": [round(v, 6) for v in obj.dimensions],
        "hide_render": bool(obj.hide_render),
    }
    if obj.type == "MESH":
        record.update({
            "vertices": len(obj.data.vertices),
            "edges": len(obj.data.edges),
            "polygons": len(obj.data.polygons),
            "uv_layers": [uv.name for uv in obj.data.uv_layers],
            "materials": [slot.material.name if slot.material else None for slot in obj.material_slots],
            "vertex_groups": [group.name for group in obj.vertex_groups],
            "modifiers": [modifier.type for modifier in obj.modifiers],
            "bounds_world": world_bounds(obj),
        })
        if obj.data.shape_keys:
            record["shape_keys"] = [key.name for key in obj.data.shape_keys.key_blocks]
            animation_data = obj.data.shape_keys.animation_data
            record["shape_key_driver_count"] = len(animation_data.drivers) if animation_data else 0
    return record


def main():
    os.makedirs(os.path.dirname(CHECKPOINT), exist_ok=True)
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    current_file = os.path.abspath(bpy.data.filepath)
    if current_file != os.path.abspath(FINAL_BLEND):
        raise RuntimeError(f"Unexpected open file: {current_file}")

    head = bpy.data.objects.get("GEO_HeadBody")
    rig = bpy.data.objects.get("RIG_Sloth")
    final_collection = bpy.data.collections.get("COL_CHR_SLOTH_FINAL")
    if not head or not rig or not final_collection:
        raise RuntimeError("Formal character core is incomplete")

    before_path = bpy.data.filepath
    bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT, copy=True)
    if bpy.data.filepath != before_path:
        raise RuntimeError("Checkpoint operation changed the working filepath")

    objects = [object_record(obj) for obj in sorted(bpy.data.objects, key=lambda item: item.name)]
    report = {
        "schema": "sloth_reference_refine_audit_v1",
        "status": "PASS",
        "open_file": bpy.data.filepath,
        "checkpoint": CHECKPOINT,
        "checkpoint_exists": os.path.exists(CHECKPOINT) and os.path.getsize(CHECKPOINT) > 0,
        "scene": bpy.context.scene.name,
        "scenes": sorted(scene.name for scene in bpy.data.scenes),
        "formal_collection": final_collection.name,
        "formal_collection_objects": sorted(obj.name for obj in final_collection.all_objects),
        "objects": objects,
        "counts": {
            "objects": len(bpy.data.objects),
            "materials": len(bpy.data.materials),
            "actions": len(bpy.data.actions),
            "scenes": len(bpy.data.scenes),
            "armatures": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]),
        },
        "core_invariants": {
            "head_vertices": len(head.data.vertices),
            "head_uv_layers": [uv.name for uv in head.data.uv_layers],
            "head_shape_keys": [key.name for key in head.data.shape_keys.key_blocks],
            "head_vertex_groups": [group.name for group in head.vertex_groups],
            "head_modifiers": [modifier.type for modifier in head.modifiers],
            "rig_bones": len(rig.data.bones),
            "actions": sorted(action.name for action in bpy.data.actions),
        },
        "role_groups": {
            "eyes": sorted(obj.name for obj in bpy.data.objects if obj.name.startswith("EYE_")),
            "fur": sorted(obj.name for obj in bpy.data.objects if obj.name.startswith("FUR_")),
            "clothing": sorted(obj.name for obj in bpy.data.objects if obj.name.startswith("CLO_")),
            "mouth": sorted(obj.name for obj in bpy.data.objects if obj.name.startswith("GEO_Mouth") or obj.name.startswith("GEO_Teeth") or obj.name == "GEO_Tongue"),
            "cameras": sorted(obj.name for obj in bpy.data.objects if obj.type == "CAMERA"),
            "lights": sorted(obj.name for obj in bpy.data.objects if obj.type == "LIGHT"),
        },
        "render": {
            "engine": bpy.context.scene.render.engine,
            "resolution": [bpy.context.scene.render.resolution_x, bpy.context.scene.render.resolution_y],
            "fps": bpy.context.scene.render.fps,
            "view_transform": bpy.context.scene.view_settings.look,
            "camera": bpy.context.scene.camera.name if bpy.context.scene.camera else None,
        },
    }
    if not report["checkpoint_exists"]:
        report["status"] = "FAIL"
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    print(json.dumps({
        "status": report["status"],
        "open_file": report["open_file"],
        "checkpoint": CHECKPOINT,
        "counts": report["counts"],
        "core_invariants": {
            "head_vertices": report["core_invariants"]["head_vertices"],
            "shape_keys": len(report["core_invariants"]["head_shape_keys"]),
            "rig_bones": report["core_invariants"]["rig_bones"],
            "actions": len(report["core_invariants"]["actions"]),
        },
        "role_groups": report["role_groups"],
        "report": REPORT,
    }, ensure_ascii=False))


if __name__ == "__main__":
    main()
