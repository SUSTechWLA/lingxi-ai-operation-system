import bpy
import json
import os


FINAL = "/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip/character_sloth_final.blend"


def main():
    bpy.ops.wm.open_mainfile(filepath=FINAL)
    records = {}
    for side in ("L", "R"):
        pupil = bpy.data.objects[f"EYE_Pupil_{side}"]
        records[side] = {
            "dimensions": [round(value, 8) for value in pupil.dimensions],
            "scale": [round(value, 8) for value in pupil.scale],
            "world_location": [round(value, 8) for value in pupil.matrix_world.translation],
        }
    print(json.dumps({"status": "RECOVERED", "file": bpy.data.filepath, "pupils": records}, ensure_ascii=False))


if __name__ == "__main__":
    main()
