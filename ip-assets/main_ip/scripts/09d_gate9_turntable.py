import bpy
import json
import os
import struct

ROOT = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip"
FINAL_BLEND = os.path.join(ROOT, "character_sloth_final.blend")
REPORT = os.path.join(ROOT, "reports", "gate9_turntable_report.json")
OUTPUT = os.path.join(ROOT, "renders", "final", "turntable.mp4")
FRAME_DIR = os.path.join(ROOT, "renders", "final", "_turntable_frames")


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
            raise RuntimeError("Invalid JPEG turntable frame: " + path)
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

scene = bpy.data.scenes.get("SCENE_LOOKDEV")
camera = bpy.data.objects.get("CAM_LOOKDEV_65MM")
if scene is None or camera is None:
    raise RuntimeError("LookDev scene or turntable camera missing")
bpy.context.window.scene = scene

saved = {
    "location": camera.location.copy(),
    "rotation": camera.rotation_euler.copy(),
    "lens": camera.data.lens,
    "frame_start": scene.frame_start,
    "frame_end": scene.frame_end,
    "fps": scene.render.fps,
    "resolution": (scene.render.resolution_x, scene.render.resolution_y),
    "samples": scene.cycles.samples,
}
camera.animation_data_clear()
target = bpy.data.objects.new("TMP_TurntableTarget", None)
scene.collection.objects.link(target)
target.location = (0.0, 0.0, 1.30)
constraint = camera.constraints.new("TRACK_TO")
constraint.name = "TMP_TurntableTrack"
constraint.target = target
constraint.track_axis = "TRACK_NEGATIVE_Z"
constraint.up_axis = "UP_Y"

keyframes = {
    1: (0.0, -8.40, 2.45),
    13: (8.40, 0.0, 2.45),
    25: (0.0, 8.40, 2.45),
    37: (-8.40, 0.0, 2.45),
    49: (0.0, -8.40, 2.45),
}
for frame, location in keyframes.items():
    camera.location = location
    camera.keyframe_insert(data_path="location", frame=frame)
if camera.animation_data and camera.animation_data.action and hasattr(camera.animation_data.action, "fcurves"):
    for fcurve in camera.animation_data.action.fcurves:
        for keyframe in fcurve.keyframe_points:
            keyframe.interpolation = "LINEAR"

scene.camera = camera
camera.data.lens = 65
scene.frame_start = 1
scene.frame_end = 48
scene.render.fps = 24
scene.render.fps_base = 1.0
scene.render.engine = "CYCLES"
scene.cycles.device = "CPU"
scene.cycles.samples = 12
scene.cycles.use_denoising = True
scene.render.resolution_x = 512
scene.render.resolution_y = 512
scene.render.resolution_percentage = 100
scene.view_settings.view_transform = "AgX"
for candidate in ("AgX - Medium High Contrast", "Medium High Contrast", "None"):
    try:
        scene.view_settings.look = candidate
        break
    except Exception:
        continue
os.makedirs(FRAME_DIR, exist_ok=True)
for filename in os.listdir(FRAME_DIR):
    path = os.path.join(FRAME_DIR, filename)
    if os.path.isfile(path):
        os.remove(path)
scene.render.image_settings.file_format = "JPEG"
scene.render.image_settings.color_mode = "RGB"
scene.render.image_settings.quality = 90
jpeg_paths = []
for frame in range(1, 49):
    scene.frame_set(frame)
    path = os.path.join(FRAME_DIR, "frame_%04d.jpg" % frame)
    scene.render.filepath = path
    bpy.ops.render.render(write_still=True)
    jpeg_paths.append(path)
mux_mjpeg_mp4(jpeg_paths, OUTPUT, 512, 512, 24)
for path in jpeg_paths:
    os.remove(path)
os.rmdir(FRAME_DIR)

camera.animation_data_clear()
camera.constraints.remove(constraint)
bpy.data.objects.remove(target, do_unlink=True)
camera.location = saved["location"]
camera.rotation_euler = saved["rotation"]
camera.data.lens = saved["lens"]
scene.frame_start = saved["frame_start"]
scene.frame_end = saved["frame_end"]
scene.render.fps = saved["fps"]
scene.render.resolution_x, scene.render.resolution_y = saved["resolution"]
scene.cycles.samples = 32
scene.render.image_settings.file_format = "PNG"
scene.render.filepath = OUTPUT
scene.frame_set(1)

checks = {
    "mp4_exists": os.path.exists(OUTPUT) and os.path.getsize(OUTPUT) > 0,
    "mp4_ftyp_header": open(OUTPUT, "rb").read(12)[4:8] == b"ftyp",
    "temporary_target_removed": bpy.data.objects.get("TMP_TurntableTarget") is None,
    "camera_animation_removed": camera.animation_data is None or camera.animation_data.action is None,
    "character_unmodified": len(bpy.data.objects["GEO_HeadBody"].data.vertices) == 5478,
    "single_armature": len([obj for obj in bpy.data.objects if obj.type == "ARMATURE"]) == 1,
}
if not all(checks.values()):
    raise RuntimeError("Turntable invariant failed: " + json.dumps(checks))
bpy.ops.wm.save_as_mainfile(filepath=FINAL_BLEND)
with open(REPORT, "w", encoding="utf-8") as handle:
    json.dump({"gate": "9d", "status": "PASS", "output": OUTPUT, "frames": 48, "fps": 24, "resolution": [512, 512], "engine": "CYCLES", "samples": 12, "view_transform": "AgX", "codec": "Motion JPEG in ISO-BMFF MP4 (in-process, no subprocess)", "checks": checks}, handle, ensure_ascii=False, indent=2)
print(json.dumps({"status": "PASS", "blend": FINAL_BLEND, "report": REPORT, "output": OUTPUT, "checks": checks}, ensure_ascii=False))
