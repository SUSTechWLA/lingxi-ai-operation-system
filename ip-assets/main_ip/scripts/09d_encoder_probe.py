import bpy
import importlib.util
import json

result = {
    "blender_codec_build": bool(getattr(bpy.app.build_options, "codec", False)),
    "ffmpeg_format_available": "FFMPEG" in {item.identifier for item in bpy.types.ImageFormatSettings.bl_rna.properties["file_format"].enum_items},
    "pyav": importlib.util.find_spec("av") is not None,
    "opencv": importlib.util.find_spec("cv2") is not None,
}
print(json.dumps(result))
