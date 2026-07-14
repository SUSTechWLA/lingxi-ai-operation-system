#!/usr/bin/env python3
"""Render deterministic multi-camera and clay QA stills from a warm studio blend."""

from __future__ import annotations

import argparse
import sys
from collections.abc import Callable, Sequence
from pathlib import Path

try:
    import bpy
except ModuleNotFoundError:  # Keep constants and argument helpers importable in CPython.
    bpy = None  # type: ignore[assignment]


CAMERA_OUTPUTS = {
    "Camera_Wide": "01-wide.png",
    "Camera_Medium": "02-medium.png",
    "Camera_Close": "03-close.png",
    "Camera_ThreeQuarter_Left": "04-three-quarter-left.png",
    "Camera_ThreeQuarter_Right": "05-three-quarter-right.png",
    "Camera_Desk_Detail": "06-desk-detail.png",
    "Camera_Shelf_Detail": "07-shelf-detail.png",
}
CLAY_OUTPUT = "08-wide-clay.png"
EEVEE_SAMPLES = 32
CYCLES_SAMPLES = 64
CYCLES_FINAL_EXPOSURE = -0.8


def _require_bpy() -> None:
    if bpy is None:
        raise RuntimeError("render_warm_studio_qa.py must run inside Blender")


def engine_identifier(engine: str) -> str:
    """Translate the stable CLI engine name to Blender's versioned identifier."""

    normalized = engine.strip().lower()
    if normalized == "cycles":
        return "CYCLES"
    if normalized != "eevee":
        raise ValueError("engine must be either 'eevee' or 'cycles'")
    if bpy is not None and bpy.app.version >= (5, 0, 0):
        return "BLENDER_EEVEE"
    return "BLENDER_EEVEE_NEXT"


def _temporary_clay_material():
    material = bpy.data.materials.new("QA_WarmStudio_Clay_Temporary")
    if material.node_tree is None:
        bpy.data.materials.remove(material)
        raise RuntimeError("Blender did not initialize nodes for the temporary clay material")
    shader = next(
        node for node in material.node_tree.nodes if node.type == "BSDF_PRINCIPLED"
    )
    shader.inputs["Base Color"].default_value = (0.58, 0.43, 0.30, 1.0)
    shader.inputs["Roughness"].default_value = 0.72
    return material


def _remove_known_outputs(destination: Path) -> None:
    """Remove only deterministic QA filenames so filtered reruns cannot look complete."""

    for filename in (*CAMERA_OUTPUTS.values(), CLAY_OUTPUT):
        path = destination / filename
        if path.is_file() or path.is_symlink():
            path.unlink()


def _set_engine_and_samples(scene, engine: str) -> None:
    identifier = engine_identifier(engine)
    try:
        scene.render.engine = identifier
    except TypeError:
        if engine.strip().lower() != "eevee":
            raise
        scene.render.engine = "BLENDER_EEVEE"
    if engine.strip().lower() == "eevee":
        eevee = getattr(scene, "eevee", None)
        if eevee is not None:
            if hasattr(eevee, "taa_render_samples"):
                eevee.taa_render_samples = EEVEE_SAMPLES
            if hasattr(eevee, "taa_samples"):
                eevee.taa_samples = EEVEE_SAMPLES
        scene["ip_qa_eevee_samples"] = EEVEE_SAMPLES
    else:
        if not hasattr(scene, "cycles"):
            raise RuntimeError("Cycles settings are unavailable in this Blender build")
        scene.cycles.samples = CYCLES_SAMPLES
        if hasattr(scene.cycles, "use_denoising"):
            scene.cycles.use_denoising = True
        scene.view_settings.exposure = float(
            scene.get("ip_cycles_final_exposure", CYCLES_FINAL_EXPOSURE)
        )
        scene["ip_qa_cycles_samples"] = CYCLES_SAMPLES


def _validated_camera_names(camera_names: Sequence[str] | None) -> list[str]:
    requested = list(CAMERA_OUTPUTS) if camera_names is None else list(camera_names)
    if not requested:
        raise ValueError("At least one QA camera must be selected")
    unknown = sorted(set(requested) - set(CAMERA_OUTPUTS))
    if unknown:
        raise ValueError("Unknown camera name(s): " + ", ".join(unknown))
    missing = [name for name in requested if bpy.data.objects.get(name) is None]
    if missing:
        raise ValueError("Required camera object(s) missing: " + ", ".join(missing))
    wrong_type = [name for name in requested if bpy.data.objects[name].type != "CAMERA"]
    if wrong_type:
        raise ValueError("QA camera object(s) have wrong type: " + ", ".join(wrong_type))
    wide = bpy.data.objects.get("Camera_Wide")
    if wide is None or wide.type != "CAMERA":
        raise ValueError("Camera_Wide is required for the clay QA render")
    requested_set = set(requested)
    return [name for name in CAMERA_OUTPUTS if name in requested_set]


def render_qa_stills(
    output_dir: Path,
    *,
    engine: str,
    camera_names: Sequence[str] | None = None,
    render_callback: Callable[[Path], None] | None = None,
) -> list[Path]:
    """Render selected camera coverage plus a wide clay diagnostic.

    ``render_callback`` is an injectable write operation used by the contract tests;
    production callers leave it unset so Blender writes the stills.
    """

    _require_bpy()
    engine_identifier(engine)  # Validate before mutating scene state.
    selected = _validated_camera_names(camera_names)
    destination = Path(output_dir).expanduser().resolve()
    destination.mkdir(parents=True, exist_ok=True)
    _remove_known_outputs(destination)
    scene = bpy.context.scene
    view_layer = bpy.context.view_layer

    original = {
        "camera": scene.camera,
        "material_override": view_layer.material_override,
        "engine": scene.render.engine,
        "exposure": scene.view_settings.exposure,
        "resolution_x": scene.render.resolution_x,
        "resolution_y": scene.render.resolution_y,
        "resolution_percentage": scene.render.resolution_percentage,
        "filepath": scene.render.filepath,
        "file_format": scene.render.image_settings.file_format,
        "color_mode": scene.render.image_settings.color_mode,
        "film_transparent": scene.render.film_transparent,
        "cycles_samples": getattr(getattr(scene, "cycles", None), "samples", None),
        "cycles_denoising": getattr(
            getattr(scene, "cycles", None), "use_denoising", None
        ),
        "eevee_render_samples": getattr(
            getattr(scene, "eevee", None), "taa_render_samples", None
        ),
        "eevee_samples": getattr(getattr(scene, "eevee", None), "taa_samples", None),
        "has_qa_eevee_samples": "ip_qa_eevee_samples" in scene,
        "qa_eevee_samples": scene.get("ip_qa_eevee_samples"),
        "has_qa_cycles_samples": "ip_qa_cycles_samples" in scene,
        "qa_cycles_samples": scene.get("ip_qa_cycles_samples"),
    }
    clay = None
    outputs: list[Path] = []

    def render_path(path: Path) -> None:
        scene.render.filepath = str(path)
        if render_callback is None:
            bpy.ops.render.render(write_still=True)
        else:
            render_callback(path)
        outputs.append(path)

    try:
        _set_engine_and_samples(scene, engine)
        scene.render.resolution_x = 960
        scene.render.resolution_y = 540
        scene.render.resolution_percentage = 100
        scene.render.image_settings.file_format = "PNG"
        scene.render.image_settings.color_mode = "RGBA"

        for camera_name in selected:
            scene.camera = bpy.data.objects[camera_name]
            view_layer.material_override = original["material_override"]
            render_path(destination / CAMERA_OUTPUTS[camera_name])

        clay = _temporary_clay_material()
        scene.camera = bpy.data.objects["Camera_Wide"]
        view_layer.material_override = clay
        render_path(destination / CLAY_OUTPUT)
    finally:
        scene.camera = original["camera"]
        view_layer.material_override = original["material_override"]
        scene.render.engine = original["engine"]
        scene.view_settings.exposure = original["exposure"]
        scene.render.resolution_x = original["resolution_x"]
        scene.render.resolution_y = original["resolution_y"]
        scene.render.resolution_percentage = original["resolution_percentage"]
        scene.render.filepath = original["filepath"]
        scene.render.image_settings.file_format = original["file_format"]
        scene.render.image_settings.color_mode = original["color_mode"]
        scene.render.film_transparent = original["film_transparent"]
        cycles = getattr(scene, "cycles", None)
        if cycles is not None:
            if original["cycles_samples"] is not None:
                cycles.samples = original["cycles_samples"]
            if original["cycles_denoising"] is not None and hasattr(cycles, "use_denoising"):
                cycles.use_denoising = original["cycles_denoising"]
        eevee = getattr(scene, "eevee", None)
        if eevee is not None:
            if original["eevee_render_samples"] is not None and hasattr(
                eevee, "taa_render_samples"
            ):
                eevee.taa_render_samples = original["eevee_render_samples"]
            if original["eevee_samples"] is not None and hasattr(eevee, "taa_samples"):
                eevee.taa_samples = original["eevee_samples"]
        for key, presence_key, value_key in (
            (
                "ip_qa_eevee_samples",
                "has_qa_eevee_samples",
                "qa_eevee_samples",
            ),
            (
                "ip_qa_cycles_samples",
                "has_qa_cycles_samples",
                "qa_cycles_samples",
            ),
        ):
            if original[presence_key]:
                scene[key] = original[value_key]
            elif key in scene:
                del scene[key]
        if clay is not None:
            bpy.data.materials.remove(clay, do_unlink=True)
    return outputs


def _arguments_after_separator(argv: list[str]) -> list[str]:
    return argv[argv.index("--") + 1 :] if "--" in argv else argv[1:]


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("blend_path", type=Path)
    parser.add_argument("output_dir", type=Path)
    parser.add_argument("engine", choices=("eevee", "cycles"))
    parser.add_argument(
        "camera_filter",
        nargs="?",
        default=None,
        help="Optional comma-separated subset of the seven contract camera names",
    )
    parser.add_argument(
        "--cameras",
        default=None,
        help="Named form of camera_filter",
    )
    raw_argv = sys.argv if argv is None else argv
    args = parser.parse_args(_arguments_after_separator(raw_argv))
    _require_bpy()
    blend_path = args.blend_path.expanduser().resolve()
    if not blend_path.is_file():
        parser.error(f"blend file does not exist: {blend_path}")
    bpy.ops.wm.open_mainfile(filepath=str(blend_path))
    camera_names = None
    if args.cameras is not None and args.camera_filter is not None:
        parser.error("pass the camera filter either positionally or with --cameras, not both")
    camera_filter = args.cameras if args.cameras is not None else args.camera_filter
    if camera_filter is not None:
        camera_names = [name.strip() for name in camera_filter.split(",") if name.strip()]
    try:
        outputs = render_qa_stills(
            args.output_dir,
            engine=args.engine,
            camera_names=camera_names,
        )
    except ValueError as exc:
        parser.error(str(exc))
    print(f"WARM_STUDIO_QA_OUTPUT_DIR={args.output_dir.expanduser().resolve()}")
    for output in outputs:
        print(f"WARM_STUDIO_QA_IMAGE={output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
