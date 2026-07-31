import bpy
import json
import os


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
FINAL = os.path.join(ROOT, "character_sloth_final.blend")
DEMO_BLEND = os.path.join(ROOT, "demos", "sloth_production_demo.blend")
CHECKPOINT = os.path.join(ROOT, "checkpoints", "sloth_104_pre_demo_video.blend")
OUTPUT = os.path.join(ROOT, "renders", "demo", "sloth_production_demo.mp4")
FRAME_DIR = os.path.join(ROOT, "renders", "demo", "frames")
REPORT = os.path.join(ROOT, "reports", "demo_final_validation.json")
STAGE_VALIDATION = os.path.join(ROOT, "reports", "stage_16_validation.json")
STAGE_SUMMARY = os.path.join(ROOT, "reports", "stage_16_summary.md")


def image_stats(path):
    image = bpy.data.images.load(path, check_existing=False)
    try:
        pixels = image.pixels
        pixel_count = image.size[0] * image.size[1]
        stride_pixels = max(1, pixel_count // 4096)
        luminance = []
        for pixel_index in range(0, pixel_count, stride_pixels):
            index = pixel_index * 4
            red = pixels[index]
            green = pixels[index + 1]
            blue = pixels[index + 2]
            luminance.append(0.2126 * red + 0.7152 * green + 0.0722 * blue)
        mean = sum(luminance) / len(luminance)
        variance = sum((value - mean) ** 2 for value in luminance) / len(luminance)
        return {
            "width": image.size[0],
            "height": image.size[1],
            "mean_luminance": round(mean, 6),
            "luminance_variance": round(variance, 6),
        }
    finally:
        bpy.data.images.remove(image)


def main():
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    if os.path.abspath(bpy.data.filepath) != os.path.abspath(DEMO_BLEND):
        bpy.ops.wm.open_mainfile(filepath=DEMO_BLEND)
    demo_scene = bpy.data.scenes.get("SCENE_DEMO_PRODUCTION")
    formal = bpy.data.collections.get("COL_CHR_SLOTH_FINAL")
    setup = bpy.data.collections.get("CXR_DEMO_SETUP")
    production_lights = bpy.data.collections.get("COL_PRODUCTION_LIGHTS")
    head = bpy.data.objects.get("GEO_HeadBody")
    rig = bpy.data.objects.get("RIG_Sloth")
    if not demo_scene or not formal or not setup or not production_lights or not head or not rig:
        raise RuntimeError("Demo delivery structure is incomplete")

    frame_paths = [os.path.join(FRAME_DIR, f"frame_{frame:04d}.jpg") for frame in range(1, 121)]
    sample_frames = (1, 36, 60, 84, 120)
    sample_stats = {str(frame): image_stats(os.path.join(FRAME_DIR, f"frame_{frame:04d}.jpg")) for frame in sample_frames}
    with open(OUTPUT, "rb") as handle:
        video_bytes = handle.read()
    source_action = bpy.data.actions.get("Talk_Loop")
    demo_action = bpy.data.actions.get("CXR_DEMO_TalkLoop_Slow")
    face_action = bpy.data.actions.get("CXR_DEMO_FaceAction")
    eyelids = [bpy.data.objects[f"EYE_Lid{part}_{side}"] for side in ("L", "R") for part in ("Upper", "Lower")]
    lid_material = bpy.data.materials.get("MAT_Eyelid_Skin_Brown")
    pupil_dimensions = {
        side: [round(value, 6) for value in bpy.data.objects[f"EYE_Pupil_{side}"].dimensions]
        for side in ("L", "R")
    }
    demo_checks = {
        "demo_file_open": os.path.abspath(bpy.data.filepath) == os.path.abspath(DEMO_BLEND),
        "frame_sequence_complete": len(frame_paths) == 120 and all(os.path.exists(path) and os.path.getsize(path) > 30000 for path in frame_paths),
        "video_container": os.path.exists(OUTPUT) and len(video_bytes) > 100000 and video_bytes[4:8] == b"ftyp" and b"mdat" in video_bytes and b"moov" in video_bytes,
        "sample_frames_not_black": all(record["mean_luminance"] > 0.02 and record["luminance_variance"] > 0.001 for record in sample_stats.values()),
        "sample_frames_target_resolution": all(record["width"] == 960 and record["height"] == 540 for record in sample_stats.values()),
        "camera_motion_visible": max(record["mean_luminance"] for record in sample_stats.values()) - min(record["mean_luminance"] for record in sample_stats.values()) > 0.005,
        "scene_timing": demo_scene.frame_start == 1 and demo_scene.frame_end == 120 and demo_scene.render.fps == 24,
        "scene_resolution": demo_scene.render.resolution_x == 960 and demo_scene.render.resolution_y == 540,
        "shared_formal_collection": formal.name in {child.name for child in demo_scene.collection.children},
        "shared_production_lights": production_lights.name in {child.name for child in demo_scene.collection.children},
        "single_formal_character": len([obj for obj in demo_scene.objects if obj.name == "GEO_HeadBody"]) == 1 and len([obj for obj in demo_scene.objects if obj.name == "RIG_Sloth"]) == 1,
        "single_armature": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]) == 1,
        "head_vertex_count": len(head.data.vertices) == 5478,
        "shape_key_count": len(head.data.shape_keys.key_blocks) == 31,
        "uvmap": [uv.name for uv in head.data.uv_layers] == ["UVMap"],
        "source_action_preserved": source_action is not None and [round(value, 4) for value in source_action.frame_range] == [1.0, 29.0],
        "demo_action_separate": demo_action is not None and demo_action is not source_action and rig.animation_data and rig.animation_data.action is demo_action,
        "demo_face_action": face_action is not None and head.data.shape_keys.animation_data and head.data.shape_keys.animation_data.action is face_action,
        "eyelid_material_preserved": lid_material is not None and all(obj.material_slots and obj.material_slots[0].material is lid_material for obj in eyelids),
        "pupil_refinement_preserved": all(values == [0.0325, 0.002, 0.0325] for values in pupil_dimensions.values()),
        "no_tmp_objects": not any(obj.name.startswith("TMP_") for obj in bpy.data.objects),
        "checkpoint_present": os.path.exists(CHECKPOINT) and os.path.getsize(CHECKPOINT) > 100000,
    }

    bpy.ops.wm.save_as_mainfile(filepath=DEMO_BLEND, copy=False)
    bpy.ops.wm.open_mainfile(filepath=FINAL)
    final_head = bpy.data.objects.get("GEO_HeadBody")
    final_rig = bpy.data.objects.get("RIG_Sloth")
    final_action = final_rig.animation_data.action if final_rig and final_rig.animation_data else None
    final_checks = {
        "final_file_opened": os.path.abspath(bpy.data.filepath) == os.path.abspath(FINAL),
        "formal_collection_present": bpy.data.collections.get("COL_CHR_SLOTH_FINAL") is not None,
        "head_invariants": final_head is not None and len(final_head.data.vertices) == 5478 and len(final_head.data.shape_keys.key_blocks) == 31,
        "single_armature": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]) == 1,
        "formal_action_restored": final_action is not None and final_action.name == "Talk_Loop",
        "no_demo_scene": bpy.data.scenes.get("SCENE_DEMO_PRODUCTION") is None,
        "no_demo_objects": not any(obj.name.startswith("CXR_DEMO_") for obj in bpy.data.objects),
        "no_demo_actions": not any(action.name.startswith("CXR_DEMO_") for action in bpy.data.actions),
        "no_demo_collections": not any(collection.name.startswith("CXR_DEMO_") for collection in bpy.data.collections),
    }

    all_checks = {**{"demo_" + name: value for name, value in demo_checks.items()}, **{"final_" + name: value for name, value in final_checks.items()}}
    failed = [name for name, value in all_checks.items() if not value]
    status = "PASS" if not failed else "FAIL"
    report = {
        "schema": "sloth_demo_final_validation_v1",
        "status": status,
        "summary": {"passed": len(all_checks) - len(failed), "total": len(all_checks), "failed": failed},
        "final_blend": FINAL,
        "demo_blend": DEMO_BLEND,
        "video": {
            "path": OUTPUT,
            "size_bytes": len(video_bytes),
            "resolution": [960, 540],
            "fps": 24,
            "frame_count": 120,
            "duration_seconds": 5.0,
            "codec": "Motion JPEG in ISO-BMFF MP4",
            "audio": None,
        },
        "sample_frame_stats": sample_stats,
        "pupil_dimensions": pupil_dimensions,
        "demo_checks": demo_checks,
        "final_asset_isolation_checks": final_checks,
    }
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    with open(STAGE_VALIDATION, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    summary_text = f"""# Stage 16 — Production Scene Demo\n\n- Status: {status}\n- Source asset: `{FINAL}`\n- Demo scene file: `{DEMO_BLEND}`\n- Video: `{OUTPUT}`\n- Format: 960×540, 24 fps, 120 frames, 5.0 seconds, Motion JPEG MP4, no audio\n- Character data: shared `COL_CHR_SLOTH_FINAL`; no duplicated formal head or Armature\n- Animation: original `Talk_Loop` preserved; demo uses `CXR_DEMO_TalkLoop_Slow` and `CXR_DEMO_FaceAction`\n- Camera: `CXR_DEMO_Camera`, 58–64 mm push-in\n- Lighting: shared `COL_PRODUCTION_LIGHTS` with demo-only warm cyclorama\n- Validation: {len(all_checks) - len(failed)}/{len(all_checks)} checks passed\n\n## Visible review priorities\n\n1. Hand and arm gesture amplitude is conservative; a stronger presentation gesture would improve performance readability.\n2. Warm production lighting reduces subtle fur color separation compared with neutral LookDev.\n3. Clothing microtexture and seam depth are still subtle at 540p.\n4. Mouth articulation is intentionally light because no audio was supplied; a lip-sync demo remains a separate test.\n5. Camera push-in now preserves the head tuft and exposes eye, eyelid, muzzle, garment, hand, and silhouette quality without wide-angle distortion.\n"""
    with open(STAGE_SUMMARY, "w", encoding="utf-8") as handle:
        handle.write(summary_text)

    bpy.ops.wm.open_mainfile(filepath=DEMO_BLEND)
    if bpy.context.window and bpy.data.scenes.get("SCENE_DEMO_PRODUCTION"):
        bpy.context.window.scene = bpy.data.scenes["SCENE_DEMO_PRODUCTION"]
    print(json.dumps({
        "status": status,
        "report": REPORT,
        "stage_validation": STAGE_VALIDATION,
        "stage_summary": STAGE_SUMMARY,
        "summary": report["summary"],
        "video": report["video"],
        "sample_frame_stats": sample_stats,
        "final_asset_isolation_checks": final_checks,
        "current_file": bpy.data.filepath,
    }, ensure_ascii=False))
    if failed:
        raise RuntimeError("Demo final validation failed: " + ", ".join(failed))


if __name__ == "__main__":
    main()
