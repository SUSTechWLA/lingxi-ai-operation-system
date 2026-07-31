import bpy
import json
import os
import struct
import time


ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
DEMO_BLEND = os.path.join(ROOT, "demos", "sloth_production_demo.blend")
REPORT = os.path.join(ROOT, "reports", "demo_render_report.json")
MANIFEST = os.path.join(ROOT, "manifests", "demo_render_manifest.json")
FRAME_DIR = os.path.join(ROOT, "renders", "demo", "frames")
OUTPUT = os.path.join(ROOT, "renders", "demo", "sloth_production_demo.mp4")
FRAME_START = 1
FRAME_END = 120
FPS = 24
WIDTH = 960
HEIGHT = 540


def box(kind, payload):
    return struct.pack(">I4s", 8 + len(payload), kind.encode("ascii")) + payload


def full_box(kind, version, flags, payload):
    return box(kind, struct.pack(">B", version) + flags.to_bytes(3, "big") + payload)


def identity_matrix():
    return struct.pack(">9I", 0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000)


def mux_mjpeg_mp4(jpeg_paths, output, width, height, fps):
    samples = []
    for path in jpeg_paths:
        with open(path, "rb") as handle:
            data = handle.read()
        if not (data.startswith(b"\xff\xd8") and data.endswith(b"\xff\xd9")):
            raise RuntimeError("Invalid JPEG demo frame: " + path)
        samples.append(data)
    count = len(samples)
    duration = count
    timescale = fps
    ftyp = box("ftyp", b"isom" + struct.pack(">I", 512) + b"isomiso2mp41")
    mdat_payload = b"".join(samples)
    mdat = box("mdat", mdat_payload)
    first_offset = len(ftyp) + 8
    offsets = []
    cursor = first_offset
    for sample in samples:
        offsets.append(cursor)
        cursor += len(sample)

    mvhd_payload = (
        struct.pack(">IIII", 0, 0, timescale, duration)
        + struct.pack(">I", 0x00010000)
        + struct.pack(">H", 0x0100)
        + b"\x00\x00"
        + b"\x00" * 8
        + identity_matrix()
        + b"\x00" * 24
        + struct.pack(">I", 2)
    )
    mvhd = full_box("mvhd", 0, 0, mvhd_payload)
    tkhd_payload = (
        struct.pack(">IIIII", 0, 0, 1, 0, duration)
        + b"\x00" * 8
        + struct.pack(">hhhh", 0, 0, 0, 0)
        + identity_matrix()
        + struct.pack(">II", width << 16, height << 16)
    )
    tkhd = full_box("tkhd", 0, 7, tkhd_payload)
    mdhd = full_box("mdhd", 0, 0, struct.pack(">IIIIHH", 0, 0, timescale, duration, 0x55C4, 0))
    hdlr = full_box("hdlr", 0, 0, struct.pack(">I4s", 0, b"vide") + b"\x00" * 12 + b"VideoHandler\x00")
    vmhd = full_box("vmhd", 0, 1, struct.pack(">HHHH", 0, 0, 0, 0))
    url = full_box("url ", 0, 1, b"")
    dref = full_box("dref", 0, 0, struct.pack(">I", 1) + url)
    dinf = box("dinf", dref)
    compressor = b"Blender Motion JPEG"
    compressor_name = bytes([len(compressor)]) + compressor + b"\x00" * (31 - len(compressor))
    visual_entry = (
        b"\x00" * 6
        + struct.pack(">H", 1)
        + struct.pack(">HH", 0, 0)
        + b"\x00" * 12
        + struct.pack(">HH", width, height)
        + struct.pack(">II", 0x00480000, 0x00480000)
        + struct.pack(">I", 0)
        + struct.pack(">H", 1)
        + compressor_name
        + struct.pack(">HH", 0x0018, 0xFFFF)
    )
    stsd = full_box("stsd", 0, 0, struct.pack(">I", 1) + box("mjpg", visual_entry))
    stts = full_box("stts", 0, 0, struct.pack(">III", 1, count, 1))
    stsc = full_box("stsc", 0, 0, struct.pack(">IIII", 1, 1, 1, 1))
    stsz = full_box("stsz", 0, 0, struct.pack(">II", 0, count) + b"".join(struct.pack(">I", len(sample)) for sample in samples))
    stco = full_box("stco", 0, 0, struct.pack(">I", count) + b"".join(struct.pack(">I", offset) for offset in offsets))
    stss = full_box("stss", 0, 0, struct.pack(">I", count) + b"".join(struct.pack(">I", index + 1) for index in range(count)))
    stbl = box("stbl", stsd + stts + stsc + stsz + stco + stss)
    minf = box("minf", vmhd + dinf + stbl)
    mdia = box("mdia", mdhd + hdlr + minf)
    trak = box("trak", tkhd + mdia)
    moov = box("moov", mvhd + trak)
    with open(output, "wb") as handle:
        handle.write(ftyp)
        handle.write(mdat)
        handle.write(moov)


def valid_frame(path):
    return os.path.exists(path) and os.path.getsize(path) > 30000


def main():
    os.makedirs(os.path.dirname(REPORT), exist_ok=True)
    os.makedirs(os.path.dirname(MANIFEST), exist_ok=True)
    os.makedirs(FRAME_DIR, exist_ok=True)
    os.makedirs(os.path.dirname(OUTPUT), exist_ok=True)
    if os.path.abspath(bpy.data.filepath) != os.path.abspath(DEMO_BLEND):
        bpy.ops.wm.open_mainfile(filepath=DEMO_BLEND)
    scene = bpy.data.scenes.get("SCENE_DEMO_PRODUCTION")
    if not scene:
        raise RuntimeError("Demo production scene is missing")
    bpy.context.window.scene = scene

    old_path = scene.render.filepath
    old_format = scene.render.image_settings.file_format
    old_mode = scene.render.image_settings.color_mode
    old_quality = scene.render.image_settings.quality
    scene.render.resolution_x = WIDTH
    scene.render.resolution_y = HEIGHT
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "JPEG"
    scene.render.image_settings.color_mode = "RGB"
    scene.render.image_settings.quality = 90

    render_start = time.perf_counter()
    rendered = []
    reused = []
    frame_times = []
    for frame in range(FRAME_START, FRAME_END + 1):
        path = os.path.join(FRAME_DIR, f"frame_{frame:04d}.jpg")
        if valid_frame(path):
            reused.append(frame)
            continue
        scene.frame_set(frame)
        scene.render.filepath = path
        start = time.perf_counter()
        bpy.ops.render.render(write_still=True)
        frame_times.append(time.perf_counter() - start)
        if not valid_frame(path):
            raise RuntimeError("Demo frame missing or too small: " + path)
        rendered.append(frame)
    render_seconds = time.perf_counter() - render_start

    jpeg_paths = [os.path.join(FRAME_DIR, f"frame_{frame:04d}.jpg") for frame in range(FRAME_START, FRAME_END + 1)]
    missing = [path for path in jpeg_paths if not valid_frame(path)]
    if missing:
        raise RuntimeError("Incomplete demo sequence: " + ", ".join(missing[:5]))
    mux_start = time.perf_counter()
    mux_mjpeg_mp4(jpeg_paths, OUTPUT, WIDTH, HEIGHT, FPS)
    mux_seconds = time.perf_counter() - mux_start

    scene.render.filepath = old_path
    scene.render.image_settings.file_format = old_format
    scene.render.image_settings.color_mode = old_mode
    scene.render.image_settings.quality = old_quality
    scene.frame_set(1)

    head = bpy.data.objects.get("GEO_HeadBody")
    rig = bpy.data.objects.get("RIG_Sloth")
    checks = {
        "frame_sequence_complete": len(jpeg_paths) == 120 and all(valid_frame(path) for path in jpeg_paths),
        "mp4_exists": os.path.exists(OUTPUT) and os.path.getsize(OUTPUT) > 100000,
        "mp4_ftyp_header": open(OUTPUT, "rb").read(12)[4:8] == b"ftyp",
        "head_invariants": head is not None and len(head.data.vertices) == 5478 and len(head.data.shape_keys.key_blocks) == 31,
        "single_armature": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]) == 1,
        "demo_action_active": rig is not None and rig.animation_data and rig.animation_data.action and rig.animation_data.action.name == "CXR_DEMO_TalkLoop_Slow",
        "single_formal_head_in_scene": len([obj for obj in scene.objects if obj.name == "GEO_HeadBody"]) == 1,
    }
    failed = [name for name, value in checks.items() if not value]
    if failed:
        raise RuntimeError("Demo video render failed: " + ", ".join(failed))

    bpy.ops.wm.save_as_mainfile(filepath=DEMO_BLEND, copy=False)
    report = {
        "schema": "sloth_demo_render_report_v1",
        "status": "PASS",
        "demo_blend": DEMO_BLEND,
        "scene": scene.name,
        "output": OUTPUT,
        "frame_dir": FRAME_DIR,
        "frame_range": [FRAME_START, FRAME_END],
        "frame_count": len(jpeg_paths),
        "fps": FPS,
        "duration_seconds": FRAME_END / FPS,
        "resolution": [WIDTH, HEIGHT],
        "engine": scene.render.engine,
        "view_transform": scene.view_settings.view_transform,
        "look": scene.view_settings.look,
        "codec": "Motion JPEG in ISO-BMFF MP4 (in-process, no subprocess)",
        "rendered_frames_this_run": rendered,
        "reused_frames": reused,
        "render_seconds_this_run": round(render_seconds, 3),
        "average_new_frame_seconds": round(sum(frame_times) / len(frame_times), 4) if frame_times else 0.0,
        "mux_seconds": round(mux_seconds, 4),
        "output_size_bytes": os.path.getsize(OUTPUT),
        "checks": checks,
    }
    manifest = {
        "schema": "sloth_demo_render_manifest_v1",
        "status": "PASS",
        "character_collection": "COL_CHR_SLOTH_FINAL",
        "rig": "RIG_Sloth",
        "demo_scene": scene.name,
        "camera": scene.camera.name if scene.camera else None,
        "rig_action": rig.animation_data.action.name if rig and rig.animation_data and rig.animation_data.action else None,
        "face_action": head.data.shape_keys.animation_data.action.name if head and head.data.shape_keys.animation_data and head.data.shape_keys.animation_data.action else None,
        "output": OUTPUT,
        "frame_sequence": os.path.join(FRAME_DIR, "frame_####.jpg"),
        "fps": FPS,
        "resolution": [WIDTH, HEIGHT],
        "duration_seconds": FRAME_END / FPS,
        "audio": None,
    }
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
    with open(MANIFEST, "w", encoding="utf-8") as handle:
        json.dump(manifest, handle, ensure_ascii=False, indent=2)
    print(json.dumps({
        "status": "PASS",
        "demo_blend": DEMO_BLEND,
        "output": OUTPUT,
        "report": REPORT,
        "manifest": MANIFEST,
        "rendered_count": len(rendered),
        "reused_count": len(reused),
        "render_seconds_this_run": report["render_seconds_this_run"],
        "output_size_bytes": report["output_size_bytes"],
        "checks": checks,
    }, ensure_ascii=False))


if __name__ == "__main__":
    main()
