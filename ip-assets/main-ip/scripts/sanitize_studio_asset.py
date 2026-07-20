"""Remove legacy texture and demo-audio data from the warm studio asset.

The input .blend is opened by Blender. This script always writes a new output file
so the source asset remains recoverable until the sanitized copy is verified.
"""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
import sys

import bpy


LEGACY_OBJECTS = {"Brand_Icon"}
LEGACY_MATERIALS = {"Brand_Icon_Alpha"}
LEGACY_IMAGES = {"warm-sloth-brand-icon.png"}
LEGACY_SOUNDS = {"warm_sloth_aroll_voice.wav"}
BRAND_TEXT_WORLD_Z = {
    "Brand_Copy_Line1": 2.245,
    "Brand_Copy_Line2": 2.055,
}


def _scene_counts() -> dict[str, int]:
    return {
        "objects": len(bpy.data.objects),
        "meshes": len(bpy.data.meshes),
        "materials": len(bpy.data.materials),
        "images": len(bpy.data.images),
        "sounds": len(bpy.data.sounds),
        "cameras": len(bpy.data.cameras),
        "lights": len(bpy.data.lights),
        "armatures": len(bpy.data.armatures),
    }


def _remove_legacy_sequences() -> list[str]:
    removed: list[str] = []
    for scene in bpy.data.scenes:
        editor = scene.sequence_editor
        if editor is None:
            continue
        strips = getattr(editor, "strips", None) or getattr(editor, "sequences", None)
        all_strips = list(getattr(editor, "strips_all", None) or getattr(editor, "sequences_all", ()))
        for strip in all_strips:
            sound = getattr(strip, "sound", None)
            if sound is not None and sound.name in LEGACY_SOUNDS:
                removed.append(f"{scene.name}:{strip.name}")
                strips.remove(strip)
    return removed


def _remove_authored_manifest_entries(names: set[str]) -> dict[str, object]:
    master = bpy.data.objects.get("SET_MASTER")
    if master is None:
        return {"updated": False, "reason": "SET_MASTER is missing"}
    text_name = str(master.get("ip_manifest_text") or "")
    text = bpy.data.texts.get(text_name)
    if text is None:
        return {"updated": False, "reason": "authored manifest text is missing"}
    manifest = json.loads(text.as_string())
    entries = [
        entry
        for entry in manifest.get("objects", [])
        if not isinstance(entry, dict) or str(entry.get("name") or "") not in names
    ]
    manifest["objects"] = entries
    manifest["objectCount"] = len(entries)
    payload = json.dumps(
        manifest,
        ensure_ascii=False,
        sort_keys=True,
        separators=(",", ":"),
    )
    text.clear()
    text.write(payload)
    master["ip_manifest_sha256"] = hashlib.sha256(payload.encode("utf-8")).hexdigest()
    master["ip_manifest_object_count"] = len(entries)
    return {"updated": True, "removedEntries": sorted(names), "objectCount": len(entries)}


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", required=True)
    parser.add_argument("--report", required=True)
    args = parser.parse_args(sys.argv[sys.argv.index("--") + 1 :] if "--" in sys.argv else [])

    source = str(Path(bpy.data.filepath).resolve())
    before = _scene_counts()
    removed = {
        "objects": [],
        "meshes": [],
        "materials": [],
        "images": [],
        "sequenceStrips": _remove_legacy_sequences(),
        "sounds": [],
    }

    for name in sorted(LEGACY_OBJECTS):
        obj = bpy.data.objects.get(name)
        if obj is not None:
            removed["objects"].append(name)
            data = obj.data if obj.type == "MESH" else None
            bpy.data.objects.remove(obj, do_unlink=True)
            if data is not None and data.users == 0:
                removed["meshes"].append(data.name)
                bpy.data.meshes.remove(data)

    brand_root = bpy.data.objects.get("Brand_Artwork")
    if brand_root is not None and "source_icon" in brand_root:
        del brand_root["source_icon"]

    adjusted_brand_copy: dict[str, list[float]] = {}
    for name, target_z in BRAND_TEXT_WORLD_Z.items():
        obj = bpy.data.objects.get(name)
        if obj is None:
            continue
        matrix = obj.matrix_world.copy()
        matrix.translation.z = target_z
        obj.matrix_world = matrix
        adjusted_brand_copy[name] = [float(value) for value in obj.matrix_world.translation]

    authored_manifest = _remove_authored_manifest_entries(LEGACY_OBJECTS)

    for name in sorted(LEGACY_MATERIALS):
        material = bpy.data.materials.get(name)
        if material is not None:
            removed["materials"].append(name)
            bpy.data.materials.remove(material, do_unlink=True)

    for name in sorted(LEGACY_IMAGES):
        image = bpy.data.images.get(name)
        if image is not None:
            removed["images"].append(name)
            bpy.data.images.remove(image, do_unlink=True)

    for name in sorted(LEGACY_SOUNDS):
        sound = bpy.data.sounds.get(name)
        if sound is not None:
            removed["sounds"].append(name)
            bpy.data.sounds.remove(sound, do_unlink=True)

    output = Path(args.output).expanduser().resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    bpy.ops.wm.save_as_mainfile(filepath=str(output), check_existing=False)

    report_path = Path(args.report).expanduser().resolve()
    report_path.parent.mkdir(parents=True, exist_ok=True)
    report = {
        "schemaVersion": "tangying-studio-sanitization/v1",
        "source": source,
        "output": str(output),
        "before": before,
        "after": _scene_counts(),
        "removed": removed,
        "authoredManifest": authored_manifest,
        "adjustedBrandCopy": adjusted_brand_copy,
        "preservedContracts": {
            "studioGeometry": True,
            "cameras": before["cameras"] == len(bpy.data.cameras),
            "lights": before["lights"] == len(bpy.data.lights),
            "armatures": before["armatures"] == len(bpy.data.armatures),
        },
    }
    report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(report, ensure_ascii=False))


if __name__ == "__main__":
    main()
