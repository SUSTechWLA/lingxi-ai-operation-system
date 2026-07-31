import bpy
import json
import os
import time


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
DEMO_BLEND = os.path.join(ROOT, "demos", "sloth_production_demo.blend")
REPORT = os.path.join(ROOT, "reports", "demo_render_benchmark.json")
FRAME_DIR = os.path.join(ROOT, "renders", "demo", "frames")
FIRST_FRAME = 1
LAST_FRAME = 24


def main():
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    os.makedirs(FRAME_DIR, exist_ok=True)
    if os.path.abspath(bpy.data.filepath) != os.path.abspath(DEMO_BLEND):
        bpy.ops.wm.open_mainfile(filepath=DEMO_BLEND)
    scene = bpy.data.scenes.get("SCENE_DEMO_PRODUCTION")
    if not scene:
        raise RuntimeError("Demo production scene is missing")
    bpy.context.window.scene = scene

    for filename in os.listdir(FRAME_DIR):
        if filename.lower().endswith((".jpg", ".jpeg")):
            os.remove(os.path.join(FRAME_DIR, filename))

    old_path = scene.render.filepath
    old_format = scene.render.image_settings.file_format
    old_mode = scene.render.image_settings.color_mode
    old_quality = scene.render.image_settings.quality
    scene.render.resolution_x = 960
    scene.render.resolution_y = 540
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "JPEG"
    scene.render.image_settings.color_mode = "RGB"
    scene.render.image_settings.quality = 90

    frame_times = []
    sequence_start = time.perf_counter()
    for frame in range(FIRST_FRAME, LAST_FRAME + 1):
        scene.frame_set(frame)
        path = os.path.join(FRAME_DIR, f"frame_{frame:04d}.jpg")
        scene.render.filepath = path
        start = time.perf_counter()
        bpy.ops.render.render(write_still=True)
        elapsed = time.perf_counter() - start
        frame_times.append(elapsed)
        if not os.path.exists(path) or os.path.getsize(path) < 30000:
            raise RuntimeError("Benchmark frame missing or too small: " + path)
    total = time.perf_counter() - sequence_start

    scene.render.filepath = old_path
    scene.render.image_settings.file_format = old_format
    scene.render.image_settings.color_mode = old_mode
    scene.render.image_settings.quality = old_quality
    scene.frame_set(1)

    head = bpy.data.objects.get("GEO_HeadBody")
    rig = bpy.data.objects.get("RIG_Sloth")
    checks = {
        "twenty_four_frames": len([name for name in os.listdir(FRAME_DIR) if name.lower().endswith(".jpg")]) == 24,
        "all_frames_valid": all(os.path.getsize(os.path.join(FRAME_DIR, f"frame_{frame:04d}.jpg")) > 30000 for frame in range(FIRST_FRAME, LAST_FRAME + 1)),
        "head_invariants": head is not None and len(head.data.vertices) == 5478 and len(head.data.shape_keys.key_blocks) == 31,
        "single_armature": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]) == 1,
        "demo_action_active": rig is not None and rig.animation_data and rig.animation_data.action and rig.animation_data.action.name == "CXR_DEMO_TalkLoop_Slow",
    }
    failed = [name for name, value in checks.items() if not value]
    if failed:
        raise RuntimeError("Demo render benchmark failed: " + ", ".join(failed))
    average = sum(frame_times) / len(frame_times)
    report = {
        "schema": "sloth_demo_render_benchmark_v1",
        "status": "PASS",
        "demo_blend": DEMO_BLEND,
        "scene": scene.name,
        "profile": "AUTO_BALANCED",
        "engine": scene.render.engine,
        "resolution": [960, 540],
        "fps": 24,
        "frames": [FIRST_FRAME, LAST_FRAME],
        "frame_count": len(frame_times),
        "first_frame_seconds": round(frame_times[0], 4),
        "average_frame_seconds": round(average, 4),
        "min_frame_seconds": round(min(frame_times), 4),
        "max_frame_seconds": round(max(frame_times), 4),
        "sequence_seconds": round(total, 4),
        "estimated_full_seconds": round(average * 120, 2),
        "frame_dir": FRAME_DIR,
        "checks": checks,
    }
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    print(json.dumps({
        "status": "PASS",
        "report": REPORT,
        "first_frame_seconds": report["first_frame_seconds"],
        "average_frame_seconds": report["average_frame_seconds"],
        "sequence_seconds": report["sequence_seconds"],
        "estimated_full_seconds": report["estimated_full_seconds"],
        "checks": checks,
    }, ensure_ascii=False))


if __name__ == "__main__":
    main()
