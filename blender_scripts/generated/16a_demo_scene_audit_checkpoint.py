import bpy
import json
import os


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
FINAL = os.path.join(ROOT, "character_sloth_final.blend")
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_104_pre_demo_video.blend")
REPORT = os.path.join(ROOT, "reports", "demo_scene_audit.json")


def action_fcurves(action):
    curves = []
    if action is None:
        return curves
    legacy = getattr(action, "fcurves", None)
    if legacy is not None:
        return list(legacy)
    for layer in getattr(action, "layers", []):
        for strip in getattr(layer, "strips", []):
            for channelbag in getattr(strip, "channelbags", []):
                curves.extend(list(getattr(channelbag, "fcurves", [])))
    return curves


def collection_names(collection):
    names = [collection.name]
    for child in collection.children:
        names.extend(collection_names(child))
    return names


def scene_record(scene, formal_names):
    cameras = []
    lights = []
    non_character = []
    for obj in scene.objects:
        if obj.type == "CAMERA":
            cameras.append({
                "name": obj.name,
                "lens": round(obj.data.lens, 4),
                "location": [round(value, 5) for value in obj.location],
                "rotation": [round(value, 5) for value in obj.rotation_euler],
            })
        elif obj.type == "LIGHT":
            lights.append({
                "name": obj.name,
                "type": obj.data.type,
                "energy": round(obj.data.energy, 4),
                "color": [round(value, 5) for value in obj.data.color],
                "location": [round(value, 5) for value in obj.location],
            })
        elif obj.name not in formal_names:
            non_character.append({"name": obj.name, "type": obj.type})
    return {
        "name": scene.name,
        "object_count": len(scene.objects),
        "collection_names": collection_names(scene.collection),
        "formal_character_object_count": len([obj for obj in scene.objects if obj.name in formal_names]),
        "formal_character_shared": formal_names.issubset({obj.name for obj in scene.objects}),
        "active_camera": scene.camera.name if scene.camera else None,
        "cameras": cameras,
        "lights": lights,
        "non_character_objects": non_character,
        "world": scene.world.name if scene.world else None,
        "frame_start": scene.frame_start,
        "frame_end": scene.frame_end,
        "frame_current": scene.frame_current,
        "render": {
            "engine": scene.render.engine,
            "resolution": [scene.render.resolution_x, scene.render.resolution_y, scene.render.resolution_percentage],
            "fps": scene.render.fps,
            "filepath": scene.render.filepath,
            "film_transparent": scene.render.film_transparent,
            "view_transform": scene.view_settings.look,
        },
    }


def cycles_record():
    record = {"compute_device_type": None, "devices": [], "error": None}
    try:
        preferences = bpy.context.preferences.addons["cycles"].preferences
        record["compute_device_type"] = preferences.compute_device_type
        preferences.get_devices()
        record["devices"] = [
            {"name": device.name, "type": device.type, "use": bool(device.use)}
            for device in preferences.devices
        ]
    except Exception as error:
        record["error"] = str(error)
    return record


def main():
    os.makedirs(os.path.dirname(CHECKPOINT), exist_ok=True)
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    if os.path.abspath(bpy.data.filepath) != os.path.abspath(FINAL):
        bpy.ops.wm.open_mainfile(filepath=FINAL)
    bpy.ops.wm.save_as_mainfile(filepath=CHECKPOINT, copy=True)

    formal = bpy.data.collections.get("COL_CHR_SLOTH_FINAL")
    rig = bpy.data.objects.get("RIG_Sloth")
    head = bpy.data.objects.get("GEO_HeadBody")
    if not formal or not rig or not head or not head.data.shape_keys:
        raise RuntimeError("Formal character system is incomplete")
    formal_names = {obj.name for obj in formal.all_objects}
    active_action = rig.animation_data.action if rig.animation_data else None
    action_records = []
    for action in bpy.data.actions:
        curves = action_fcurves(action)
        action_records.append({
            "name": action.name,
            "frame_range": [round(value, 4) for value in action.frame_range],
            "fcurve_count": len(curves),
            "bone_channels": sorted({
                curve.data_path.split('pose.bones["', 1)[1].split('"]', 1)[0]
                for curve in curves if 'pose.bones["' in curve.data_path
            }),
        })

    shape_keys = head.data.shape_keys
    driver_count = len(shape_keys.animation_data.drivers) if shape_keys.animation_data else 0
    report = {
        "schema": "sloth_demo_scene_audit_v1",
        "status": "PASS",
        "blend_file": bpy.data.filepath,
        "checkpoint": CHECKPOINT,
        "blender_version": bpy.app.version_string,
        "active_scene": bpy.context.scene.name,
        "formal_collection": formal.name,
        "formal_object_count": len(formal_names),
        "scenes": [scene_record(scene, formal_names) for scene in bpy.data.scenes],
        "rig": {
            "name": rig.name,
            "bone_count": len(rig.data.bones),
            "active_action": active_action.name if active_action else None,
            "action_count": len(bpy.data.actions),
        },
        "actions": action_records,
        "head": {
            "vertex_count": len(head.data.vertices),
            "shape_key_count": len(shape_keys.key_blocks),
            "shape_key_names": [key.name for key in shape_keys.key_blocks],
            "driver_count": driver_count,
            "modifier_order": [modifier.type for modifier in head.modifiers],
            "uv_layers": [uv.name for uv in head.data.uv_layers],
        },
        "hardware": cycles_record(),
        "checks": {
            "checkpoint_written": os.path.exists(CHECKPOINT) and os.path.getsize(CHECKPOINT) > 100000,
            "two_or_more_scenes": len(bpy.data.scenes) >= 2,
            "formal_character_single": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]) == 1,
            "head_invariants": len(head.data.vertices) == 5478 and len(shape_keys.key_blocks) == 31,
            "talk_loop_active": active_action is not None and active_action.name == "Talk_Loop",
        },
    }
    failed = [name for name, value in report["checks"].items() if not value]
    report["status"] = "PASS" if not failed else "FAIL"
    report["failed"] = failed
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    print(json.dumps({
        "status": report["status"],
        "report": REPORT,
        "checkpoint": CHECKPOINT,
        "active_scene": report["active_scene"],
        "scene_names": [scene["name"] for scene in report["scenes"]],
        "active_action": report["rig"]["active_action"],
        "hardware": report["hardware"],
        "checks": report["checks"],
    }, ensure_ascii=False))
    if failed:
        raise RuntimeError("Demo scene audit failed: " + ", ".join(failed))


if __name__ == "__main__":
    main()
