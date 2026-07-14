#!/usr/bin/env python3
"""Render deterministic multi-camera and clay QA stills from a warm studio blend."""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import sys
import time
from collections.abc import Callable, Sequence
from contextlib import contextmanager
from pathlib import Path
from statistics import median

try:
    import bpy
except ModuleNotFoundError:  # Keep constants and argument helpers importable in CPython.
    bpy = None  # type: ignore[assignment]

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))


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
EEVEE_SAMPLES = 128
CYCLES_SAMPLES = 64
CYCLES_FINAL_EXPOSURE = -0.6
LIGHTING_EVIDENCE_SCHEMA = "tangying-warm-studio-lighting-evidence/v1"
LIGHTING_EVIDENCE_FILENAME = "lighting-evidence.json"
SUBJECT_MATTE_SOURCE = "Rendered character ID matte from actual scene geometry"
FACE_MASK_SOURCE = (
    "Rendered character ID matte intersected with projected semantic head geometry"
)
BACKGROUND_MASK_SOURCE = (
    "Rendered non-character geometry excluding practical-highlight IDs and clipped display highlights"
)
MIN_CAMERA_PIXEL_MAE = 1e-3


def subject_lighting_render_plan() -> list[dict[str, str | bool]]:
    """Return the required real-character and empty-control evidence matrix."""

    plan: list[dict[str, str | bool]] = []
    for mode in ("standing", "seated"):
        for camera_role in ("medium", "three_quarter", "wide"):
            plan.append(
                {
                    "mode": mode,
                    "engine": "eevee",
                    "cameraRole": camera_role,
                    "emptyRoom": False,
                    "filename": f"eevee-{mode}-{camera_role.replace('_', '-')}.png",
                }
            )
        plan.append(
            {
                "mode": mode,
                "engine": "eevee",
                "cameraRole": "medium",
                "emptyRoom": True,
                "filename": f"eevee-{mode}-medium-empty.png",
            }
        )
    for mode in ("standing", "seated"):
        for empty_room in (False, True):
            suffix = "-empty" if empty_room else ""
            plan.append(
                {
                    "mode": mode,
                    "engine": "cycles",
                    "cameraRole": "medium",
                    "emptyRoom": empty_room,
                    "filename": f"cycles-{mode}-medium{suffix}.png",
                }
            )
    return plan


def subject_mask_from_rendered_id_matte(
    matte_rgba: Sequence[float],
    *,
    threshold: float = 0.5,
) -> list[bool]:
    """Return the actual visible subject silhouette from a rendered geometry matte."""

    if len(matte_rgba) % 4:
        raise ValueError("rendered ID matte must be an RGBA buffer")
    return [float(matte_rgba[index]) > float(threshold) for index in range(0, len(matte_rgba), 4)]


def practical_highlight_mask_from_rendered_id_matte(
    matte_rgba: Sequence[float],
    *,
    threshold: float = 0.5,
) -> list[bool]:
    """Return visible practical fixtures encoded in the matte green channel."""

    if len(matte_rgba) % 4:
        raise ValueError("rendered ID matte must be an RGBA buffer")
    return [
        float(matte_rgba[index + 1]) > float(threshold)
        for index in range(0, len(matte_rgba), 4)
    ]


def _point_in_triangle(
    point: tuple[float, float],
    triangle: Sequence[tuple[float, float]],
) -> bool:
    (px, py) = point
    (ax, ay), (bx, by), (cx, cy) = triangle
    denominator = (by - cy) * (ax - cx) + (cx - bx) * (ay - cy)
    if abs(denominator) <= 1e-12:
        return False
    alpha = ((by - cy) * (px - cx) + (cx - bx) * (py - cy)) / denominator
    beta = ((cy - ay) * (px - cx) + (ax - cx) * (py - cy)) / denominator
    gamma = 1.0 - alpha - beta
    epsilon = 1e-9
    return alpha >= -epsilon and beta >= -epsilon and gamma >= -epsilon


def face_mask_from_projected_head_triangles(
    subject_mask: Sequence[bool],
    *,
    width: int,
    height: int,
    projected_head_triangles: Sequence[Sequence[tuple[float, float]]],
) -> list[bool]:
    """Rasterize live head-weighted geometry and intersect it with the ID matte."""

    if width <= 0 or height <= 0 or len(subject_mask) != width * height:
        raise ValueError("subject mask dimensions do not match width and height")
    triangles: list[tuple[tuple[float, float], ...]] = []
    for triangle in projected_head_triangles:
        if len(triangle) != 3:
            raise ValueError("projected head geometry must contain triangles")
        normalized = tuple((float(x), float(y)) for x, y in triangle)
        if all(math.isfinite(value) for point in normalized for value in point):
            triangles.append(normalized)
    if not triangles:
        raise ValueError("projected head geometry is empty")

    result = [False] * (width * height)
    for triangle in triangles:
        min_x = max(0, math.floor(min(point[0] for point in triangle) * width))
        max_x = min(width - 1, math.ceil(max(point[0] for point in triangle) * width))
        min_y = max(0, math.floor(min(point[1] for point in triangle) * height))
        max_y = min(height - 1, math.ceil(max(point[1] for point in triangle) * height))
        for pixel_y in range(min_y, max_y + 1):
            for pixel_x in range(min_x, max_x + 1):
                index = pixel_y * width + pixel_x
                if not subject_mask[index]:
                    continue
                point = ((pixel_x + 0.5) / width, (pixel_y + 0.5) / height)
                if _point_in_triangle(point, triangle):
                    result[index] = True
    if not any(result):
        raise ValueError("projected head geometry does not overlap the rendered subject mask")
    return result


def background_mask_from_geometry_masks(
    *,
    subject_mask: Sequence[bool],
    practical_highlight_mask: Sequence[bool],
    display_rgba: Sequence[float],
    width: int,
    height: int,
) -> list[bool]:
    """Select visible background while excluding geometry IDs and display clipping."""

    pixel_count = width * height
    if width <= 0 or height <= 0:
        raise ValueError("background mask dimensions must be positive")
    if len(subject_mask) != pixel_count or len(practical_highlight_mask) != pixel_count:
        raise ValueError("background geometry masks do not match width and height")
    if len(display_rgba) != pixel_count * 4:
        raise ValueError("display RGBA buffer length does not match dimensions")
    clipping_threshold = 1.0 - (0.5 / 255.0)
    result = [
        not bool(subject_mask[index])
        and not bool(practical_highlight_mask[index])
        and max(float(value) for value in display_rgba[index * 4 : index * 4 + 3])
        < clipping_threshold
        for index in range(pixel_count)
    ]
    if not any(result):
        raise ValueError("background mask is empty")
    return result


def _linear_luminance(rgb: Sequence[float]) -> float:
    return (
        0.2126 * float(rgb[0])
        + 0.7152 * float(rgb[1])
        + 0.0722 * float(rgb[2])
    )


def _rgba_luminances(
    rgba: Sequence[float],
    mask: Sequence[bool],
) -> list[float]:
    if len(rgba) != len(mask) * 4:
        raise ValueError("RGBA buffer length does not match mask")
    return [
        _linear_luminance(rgba[index * 4 : index * 4 + 3])
        for index, selected in enumerate(mask)
        if selected
    ]


def _clipped_components(
    clipped: Sequence[bool],
    *,
    width: int,
    height: int,
) -> list[list[int]]:
    pending = set(index for index, selected in enumerate(clipped) if selected)
    components: list[list[int]] = []
    while pending:
        start = pending.pop()
        component = [start]
        stack = [start]
        while stack:
            index = stack.pop()
            x = index % width
            y = index // width
            neighbors = []
            if x > 0:
                neighbors.append(index - 1)
            if x + 1 < width:
                neighbors.append(index + 1)
            if y > 0:
                neighbors.append(index - width)
            if y + 1 < height:
                neighbors.append(index + width)
            for neighbor in neighbors:
                if neighbor in pending:
                    pending.remove(neighbor)
                    component.append(neighbor)
                    stack.append(neighbor)
        components.append(component)
    return components


def measure_lighting_evidence(
    *,
    linear_rgba: Sequence[float],
    display_rgba: Sequence[float],
    subject_mask: Sequence[bool],
    face_mask: Sequence[bool],
    background_mask: Sequence[bool],
    width: int,
    height: int,
    micro_catchlight_max_pixels: int = 2,
) -> dict[str, float | int]:
    """Measure scene-linear separation and display-referred non-micro clipping."""

    pixel_count = width * height
    if (
        len(subject_mask) != pixel_count
        or len(face_mask) != pixel_count
        or len(background_mask) != pixel_count
    ):
        raise ValueError("lighting masks do not match width and height")
    subject_pixels = sum(bool(value) for value in subject_mask)
    face_pixels = sum(bool(value) for value in face_mask)
    background_pixels = sum(bool(value) for value in background_mask)
    if subject_pixels == 0:
        raise ValueError("subject mask is empty")
    if face_pixels == 0:
        raise ValueError("face mask is empty")
    if background_pixels == 0:
        raise ValueError("background mask is empty")
    if any(face and not subject for face, subject in zip(face_mask, subject_mask)):
        raise ValueError("face mask must be a subset of the rendered subject mask")
    if any(background and subject for background, subject in zip(background_mask, subject_mask)):
        raise ValueError("background mask must exclude the rendered subject mask")
    face_luminances = _rgba_luminances(linear_rgba, face_mask)
    background_luminances = _rgba_luminances(linear_rgba, background_mask)
    face_luminance = float(median(face_luminances))
    background_luminance = float(median(background_luminances))
    stops = math.log2(max(face_luminance, 1e-8) / max(background_luminance, 1e-8))

    if len(display_rgba) != pixel_count * 4:
        raise ValueError("display RGBA buffer length does not match dimensions")
    clipping_threshold = 1.0 - (0.5 / 255.0)
    clipped = [
        bool(subject_mask[index])
        and max(float(value) for value in display_rgba[index * 4 : index * 4 + 3])
        >= clipping_threshold
        for index in range(pixel_count)
    ]
    components = _clipped_components(clipped, width=width, height=height)
    micro_components = [
        component
        for component in components
        if len(component) <= max(0, int(micro_catchlight_max_pixels))
    ]
    micro_pixels = sum(len(component) for component in micro_components)
    clipped_pixels = sum(len(component) for component in components) - micro_pixels
    display_subject = []
    for index, selected in enumerate(subject_mask):
        if not selected:
            continue
        rgb = tuple(float(value) for value in display_rgba[index * 4 : index * 4 + 3])
        display_subject.append((_linear_luminance(rgb), rgb))
    display_subject.sort(key=lambda item: item[0])
    bright_threshold = display_subject[
        int(0.70 * max(0, len(display_subject) - 1))
    ][0]
    bright_pixels = [rgb for luminance, rgb in display_subject if luminance >= bright_threshold]
    bright_median = tuple(
        float(median(pixel[channel] for pixel in bright_pixels))
        for channel in range(3)
    )
    red_blue_ratio = bright_median[0] / max(bright_median[2], 1e-8)
    red_green_ratio = bright_median[0] / max(bright_median[1], 1e-8)
    return {
        "linearFaceLuminance": face_luminance,
        "linearBackgroundLuminance": background_luminance,
        "backgroundStopsBelowFace": stops,
        "highlightClipRatio": clipped_pixels / subject_pixels,
        "subjectPixelCount": subject_pixels,
        "facePixelCount": face_pixels,
        "backgroundPixelCount": background_pixels,
        "microCatchlightComponentCount": len(micro_components),
        "microCatchlightPixelCount": micro_pixels,
        "nonCatchlightClippedPixelCount": clipped_pixels,
        "brightNeutralPixelCount": len(bright_pixels),
        "brightNeutralMedianRgb": list(bright_median),
        "brightNeutralRedBlueRatio": red_blue_ratio,
        "brightNeutralRedGreenRatio": red_green_ratio,
    }


def _require_bpy() -> None:
    if bpy is None:
        raise RuntimeError("render_warm_studio_qa.py must run inside Blender")


@contextmanager
def _isolated_timeline_camera_markers(scene):
    """Temporarily remove camera cuts so a QA render owns the active camera."""

    snapshots = [
        (marker.name, int(marker.frame), marker.camera, bool(marker.select))
        for marker in scene.timeline_markers
        if marker.camera is not None
    ]
    for marker in list(scene.timeline_markers):
        if marker.camera is not None:
            scene.timeline_markers.remove(marker)
    try:
        yield
    finally:
        for name, frame, camera, selected in snapshots:
            marker = scene.timeline_markers.new(name, frame=frame)
            marker.camera = camera
            marker.select = selected


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

    def render_path(path: Path, camera) -> None:
        with _isolated_timeline_camera_markers(scene):
            scene.camera = camera
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
            view_layer.material_override = original["material_override"]
            render_path(
                destination / CAMERA_OUTPUTS[camera_name],
                bpy.data.objects[camera_name],
            )

        clay = _temporary_clay_material()
        view_layer.material_override = clay
        render_path(destination / CLAY_OUTPUT, bpy.data.objects["Camera_Wide"])
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


def _sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _camera_metadata(scene, camera, active_camera_at_render: str) -> dict[str, object]:
    return {
        "requestedCamera": camera.name,
        "activeCamera": active_camera_at_render,
        "frame": int(scene.frame_current),
        "lensMm": float(camera.data.lens),
        "matrixWorld": [
            float(component)
            for row in camera.matrix_world
            for component in row
        ],
        "timelineCameraMarkerCountDuringRender": sum(
            marker.camera is not None for marker in scene.timeline_markers
        ),
    }


def _mean_absolute_rgb_difference(
    first_rgba: Sequence[float],
    second_rgba: Sequence[float],
) -> float:
    if len(first_rgba) != len(second_rgba) or len(first_rgba) % 4:
        raise ValueError("camera comparison buffers must be same-sized RGBA images")
    if len(first_rgba) == 0:
        raise ValueError("camera comparison buffers are empty")
    difference = sum(
        abs(float(first_rgba[index + channel]) - float(second_rgba[index + channel]))
        for index in range(0, len(first_rgba), 4)
        for channel in range(3)
    )
    return difference / ((len(first_rgba) // 4) * 3)


def _configure_cycles_device(scene) -> dict[str, object]:
    report: dict[str, object] = {"requested": "CPU", "enabledDevices": []}
    try:
        preferences = bpy.context.preferences.addons["cycles"].preferences
        for backend in ("METAL", "OPTIX", "CUDA", "HIP", "ONEAPI"):
            try:
                preferences.compute_device_type = backend
                preferences.get_devices()
            except (TypeError, ValueError, RuntimeError):
                continue
            enabled = []
            for device in preferences.devices:
                use_device = device.type != "CPU"
                device.use = use_device
                if use_device:
                    enabled.append(f"{device.name} ({device.type})")
            if enabled:
                scene.cycles.device = "GPU"
                report = {"requested": backend, "enabledDevices": enabled}
                break
    except (AttributeError, KeyError, RuntimeError):
        pass
    report["sceneDevice"] = scene.cycles.device
    return report


def _configure_linear_exr_output(scene, exr_path: Path) -> None:
    group = bpy.data.node_groups.new(
        f"QA_Linear_{exr_path.stem}",
        "CompositorNodeTree",
    )
    scene.compositing_node_group = group
    render_layers = group.nodes.new("CompositorNodeRLayers")
    output = group.nodes.new("CompositorNodeOutputFile")
    output.file_output_items.new("RGBA", "Image")
    output.directory = str(exr_path.parent)
    output.file_name = exr_path.stem
    output.format.file_format = "OPEN_EXR_MULTILAYER"
    output.format.color_depth = "32"
    output.save_as_render = False
    group.links.new(render_layers.outputs["Image"], output.inputs["Image"])


def _render_display_and_linear(
    *,
    scene,
    camera,
    engine: str,
    png_path: Path,
    exr_path: Path,
    resolution: tuple[int, int],
) -> tuple[float, dict[str, object]]:
    png_path.parent.mkdir(parents=True, exist_ok=True)
    exr_path.parent.mkdir(parents=True, exist_ok=True)
    png_path.unlink(missing_ok=True)
    exr_path.unlink(missing_ok=True)
    _set_engine_and_samples(scene, engine)
    scene.render.resolution_x, scene.render.resolution_y = resolution
    scene.render.resolution_percentage = 100
    scene.render.film_transparent = False
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"
    scene.render.image_settings.color_depth = "8"
    scene.render.filepath = str(png_path)
    _configure_linear_exr_output(scene, exr_path)
    started = time.monotonic()
    active_cameras: list[str] = []

    def capture_active_camera(render_scene, *_args) -> None:
        active_cameras.append(getattr(render_scene.camera, "name", ""))

    with _isolated_timeline_camera_markers(scene):
        scene.camera = camera
        bpy.app.handlers.render_pre.append(capture_active_camera)
        try:
            bpy.ops.render.render(write_still=True)
        finally:
            bpy.app.handlers.render_pre.remove(capture_active_camera)
        metadata = _camera_metadata(
            scene,
            camera,
            active_cameras[-1] if active_cameras else getattr(scene.camera, "name", ""),
        )
    duration = time.monotonic() - started
    if not png_path.is_file() or not exr_path.is_file():
        raise RuntimeError(
            f"render did not produce both display PNG and linear EXR: {png_path}, {exr_path}"
        )
    if metadata["requestedCamera"] != metadata["activeCamera"]:
        raise RuntimeError(
            "QA render active camera mismatch: "
            f"requested {metadata['requestedCamera']!r}, rendered {metadata['activeCamera']!r}"
        )
    if metadata["timelineCameraMarkerCountDuringRender"] != 0:
        raise RuntimeError("QA render did not isolate timeline camera markers")
    return duration, metadata


def _emission_material(name: str, color: tuple[float, float, float]):
    material = bpy.data.materials.new(name)
    material.use_nodes = True
    nodes = material.node_tree.nodes
    nodes.clear()
    output = nodes.new("ShaderNodeOutputMaterial")
    emission = nodes.new("ShaderNodeEmission")
    emission.inputs["Color"].default_value = (*color, 1.0)
    emission.inputs["Strength"].default_value = 1.0
    material.node_tree.links.new(emission.outputs["Emission"], output.inputs["Surface"])
    return material


def _practical_highlight_objects(scene) -> list[object]:
    fixture_names = {
        str(obj.get("ip_fixture"))
        for obj in scene.objects
        if obj.type == "LIGHT" and obj.get("ip_light_role") == "practical"
    }
    result = []
    for obj in scene.objects:
        materials = getattr(getattr(obj, "data", None), "materials", ())
        has_emissive_fixture_material = any(
            material is not None and "emissive" in material.name.lower()
            for material in materials
        )
        if obj.name in fixture_names or has_emissive_fixture_material:
            result.append(obj)
    return result


def _render_subject_id_matte(
    *,
    scene,
    camera,
    character_objects: Sequence[object],
    practical_objects: Sequence[object],
    output_path: Path,
    resolution: tuple[int, int],
) -> tuple[float, dict[str, object]]:
    character_ids = {id(obj) for obj in character_objects}
    practical_ids = {id(obj) for obj in practical_objects}
    subject = _emission_material("QA_Subject_ID_Red", (1.0, 0.0, 0.0))
    practical = _emission_material("QA_Practical_ID_Green", (0.0, 1.0, 0.0))
    background_material = _emission_material("QA_Background_ID_Black", (0.0, 0.0, 0.0))
    material_snapshots: dict[object, tuple[object | None, ...]] = {}
    world_background = None
    world_snapshot = None
    if scene.world is not None and scene.world.use_nodes:
        world_background = next(
            node for node in scene.world.node_tree.nodes if node.type == "BACKGROUND"
        )
        world_snapshot = (
            tuple(world_background.inputs["Color"].default_value),
            float(world_background.inputs["Strength"].default_value),
        )
    original = {
        "compositing_node_group": scene.compositing_node_group,
        "engine": scene.render.engine,
        "camera": scene.camera,
        "resolution_x": scene.render.resolution_x,
        "resolution_y": scene.render.resolution_y,
        "resolution_percentage": scene.render.resolution_percentage,
        "film_transparent": scene.render.film_transparent,
        "file_format": scene.render.image_settings.file_format,
        "color_mode": scene.render.image_settings.color_mode,
        "color_depth": scene.render.image_settings.color_depth,
        "filepath": scene.render.filepath,
        "view_transform": scene.view_settings.view_transform,
        "look": scene.view_settings.look,
        "exposure": scene.view_settings.exposure,
        "material_override": bpy.context.view_layer.material_override,
    }
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.unlink(missing_ok=True)
    active_cameras: list[str] = []

    def capture_active_camera(render_scene, *_args) -> None:
        active_cameras.append(getattr(render_scene.camera, "name", ""))

    started = time.monotonic()
    try:
        for obj in scene.objects:
            if obj.hide_render or obj.type not in {"MESH", "CURVE", "SURFACE", "FONT", "META"}:
                continue
            materials = getattr(obj.data, "materials", None)
            if materials is None:
                continue
            material_snapshots[obj] = tuple(materials)
            materials.clear()
            if id(obj) in character_ids:
                materials.append(subject)
            elif id(obj) in practical_ids:
                materials.append(practical)
            else:
                materials.append(background_material)
        if world_background is not None:
            world_background.inputs["Color"].default_value = (0.0, 0.0, 0.0, 1.0)
            world_background.inputs["Strength"].default_value = 0.0
        scene.compositing_node_group = None
        bpy.context.view_layer.material_override = None
        _set_engine_and_samples(scene, "eevee")
        scene.render.resolution_x, scene.render.resolution_y = resolution
        scene.render.resolution_percentage = 100
        scene.render.film_transparent = False
        scene.render.image_settings.file_format = "PNG"
        scene.render.image_settings.color_mode = "RGBA"
        scene.render.image_settings.color_depth = "8"
        scene.render.filepath = str(output_path)
        try:
            scene.view_settings.view_transform = "Raw"
        except (TypeError, ValueError):
            scene.view_settings.view_transform = "Standard"
        scene.view_settings.look = "None"
        scene.view_settings.exposure = 0.0
        with _isolated_timeline_camera_markers(scene):
            scene.camera = camera
            bpy.app.handlers.render_pre.append(capture_active_camera)
            try:
                bpy.ops.render.render(write_still=True)
            finally:
                bpy.app.handlers.render_pre.remove(capture_active_camera)
            metadata = _camera_metadata(
                scene,
                camera,
                active_cameras[-1] if active_cameras else getattr(scene.camera, "name", ""),
            )
    finally:
        for obj, materials_snapshot in material_snapshots.items():
            materials = obj.data.materials
            materials.clear()
            for material in materials_snapshot:
                materials.append(material)
        if world_background is not None and world_snapshot is not None:
            world_background.inputs["Color"].default_value = world_snapshot[0]
            world_background.inputs["Strength"].default_value = world_snapshot[1]
        scene.compositing_node_group = original["compositing_node_group"]
        scene.render.engine = original["engine"]
        scene.camera = original["camera"]
        scene.render.resolution_x = original["resolution_x"]
        scene.render.resolution_y = original["resolution_y"]
        scene.render.resolution_percentage = original["resolution_percentage"]
        scene.render.film_transparent = original["film_transparent"]
        scene.render.image_settings.file_format = original["file_format"]
        scene.render.image_settings.color_mode = original["color_mode"]
        scene.render.image_settings.color_depth = original["color_depth"]
        scene.render.filepath = original["filepath"]
        scene.view_settings.view_transform = original["view_transform"]
        scene.view_settings.look = original["look"]
        scene.view_settings.exposure = original["exposure"]
        bpy.context.view_layer.material_override = original["material_override"]
        for material in (subject, practical, background_material):
            bpy.data.materials.remove(material, do_unlink=True)
    duration = time.monotonic() - started
    if not output_path.is_file():
        raise RuntimeError(f"subject ID matte was not written: {output_path}")
    if metadata["requestedCamera"] != metadata["activeCamera"]:
        raise RuntimeError("subject ID matte rendered from the wrong active camera")
    return duration, metadata


def _read_rgba(path: Path):
    import OpenImageIO as oiio
    import numpy as np

    image_input = oiio.ImageInput.open(str(path))
    if image_input is None:
        raise RuntimeError(f"OpenImageIO could not open {path}")
    try:
        spec = image_input.spec()
        pixels = image_input.read_image(oiio.FLOAT)
    finally:
        image_input.close()
    if pixels is None:
        raise RuntimeError(f"OpenImageIO could not read {path}")
    pixels = np.asarray(pixels, dtype=np.float32)
    if pixels.shape[:2] != (spec.height, spec.width):
        raise RuntimeError(f"unexpected image shape for {path}: {pixels.shape}")
    if pixels.shape[2] == 3:
        alpha = np.ones((spec.height, spec.width, 1), dtype=np.float32)
        pixels = np.concatenate((pixels, alpha), axis=2)
    if pixels.shape[2] != 4:
        raise RuntimeError(f"expected RGBA image at {path}, found {pixels.shape[2]} channels")
    return np.flipud(pixels), (spec.width, spec.height)


def _write_mask_png(path: Path, mask: Sequence[bool], width: int, height: int) -> None:
    import OpenImageIO as oiio
    import numpy as np

    values = np.asarray(mask, dtype=np.uint8).reshape((height, width, 1)) * 255
    rgba = np.concatenate((values, values, values, np.full_like(values, 255)), axis=2)
    rgba = np.flipud(rgba)
    output = oiio.ImageOutput.create(str(path))
    if output is None:
        raise RuntimeError(f"OpenImageIO could not create {path}")
    path.parent.mkdir(parents=True, exist_ok=True)
    spec = oiio.ImageSpec(width, height, 4, oiio.UINT8)
    try:
        if not output.open(str(path), spec):
            raise RuntimeError(f"OpenImageIO could not open mask output {path}")
        if not output.write_image(rgba):
            raise RuntimeError(f"OpenImageIO could not write mask output {path}")
    finally:
        output.close()


def _projected_head_triangles(scene, camera, character_objects, bone_map):
    from bpy_extras.object_utils import world_to_camera_view

    role_names = {
        str(bone_map[role])
        for role in ("head", "jaw", "eye_l", "eye_r")
        if role in bone_map
    }
    depsgraph = bpy.context.evaluated_depsgraph_get()
    projected: list[tuple[tuple[float, float], ...]] = []
    for obj in character_objects:
        if obj.type != "MESH":
            continue
        evaluated = obj.evaluated_get(depsgraph)
        group_indices = {
            group.index
            for group in evaluated.vertex_groups
            if group.name in role_names
            or any(token in group.name.lower() for token in ("head", "hair", "jaw", "eye"))
        }
        if not group_indices:
            continue
        mesh = evaluated.to_mesh(preserve_all_data_layers=True, depsgraph=depsgraph)
        try:
            mesh.calc_loop_triangles()
            vertex_weights = [
                sum(group.weight for group in vertex.groups if group.group in group_indices)
                for vertex in mesh.vertices
            ]
            for triangle in mesh.loop_triangles:
                weights = [vertex_weights[index] for index in triangle.vertices]
                if sum(weight >= 0.35 for weight in weights) < 2 or sum(weights) / 3.0 < 0.35:
                    continue
                points: list[tuple[float, float]] = []
                for vertex_index in triangle.vertices:
                    world_point = evaluated.matrix_world @ mesh.vertices[vertex_index].co
                    coordinate = world_to_camera_view(scene, camera, world_point)
                    if (
                        coordinate.z <= 0.0
                        or not math.isfinite(coordinate.x)
                        or not math.isfinite(coordinate.y)
                    ):
                        points = []
                        break
                    points.append((float(coordinate.x), float(coordinate.y)))
                if len(points) == 3:
                    projected.append(tuple(points))
        finally:
            evaluated.to_mesh_clear()
    if not projected:
        raise RuntimeError(
            "canonical character head-weighted geometry did not project into the medium camera"
        )
    return projected


def _relative(path: Path, root: Path) -> str:
    try:
        return str(path.relative_to(root))
    except ValueError:
        return str(path)


def render_subject_lighting_evidence(
    *,
    scene_path: Path,
    master_path: Path,
    output_dir: Path,
    resolution: tuple[int, int] = (960, 540),
    frame: int = 29,
) -> dict[str, object]:
    """Render and measure the real canonical character in every Task 6 QA view."""

    _require_bpy()
    import validate_warm_studio as studio_validator
    import validate_warm_studio_character as character_validator

    scene_path = Path(scene_path).expanduser().resolve()
    master_path = Path(master_path).expanduser().resolve()
    destination = Path(output_dir).expanduser().resolve()
    if not scene_path.is_file() or scene_path.suffix.lower() != ".blend":
        raise RuntimeError(f"invalid warm studio Blend: {scene_path}")
    if not master_path.is_file() or master_path.suffix.lower() != ".blend":
        raise RuntimeError(f"invalid canonical master Blend: {master_path}")
    destination.mkdir(parents=True, exist_ok=True)
    linear_dir = destination / "linear"
    mask_dir = destination / "masks"
    render_records: list[dict[str, object]] = []
    measurements: list[dict[str, object]] = []
    camera_comparisons: list[dict[str, object]] = []
    hardware: dict[str, object] = {
        "blenderVersion": ".".join(str(value) for value in bpy.app.version),
        "resolution": list(resolution),
        "frame": int(frame),
    }

    for mode in ("standing", "seated"):
        character_objects, _armature, bone_map, mode_objects = character_validator._load_mode(
            scene_path,
            master_path,
            mode,
            (1, 15, frame),
        )
        scene = bpy.context.scene
        scene.frame_set(frame)
        bpy.context.view_layer.update()
        medium_camera = mode_objects["cameras"]["medium"]
        projected_head_triangles = _projected_head_triangles(
            scene,
            medium_camera,
            character_objects,
            bone_map,
        )
        mode_outputs: dict[tuple[str, bool], tuple[Path, Path]] = {}
        eevee_camera_outputs: dict[str, tuple[Path, dict[str, object]]] = {}

        for engine in ("eevee", "cycles"):
            if engine == "cycles":
                hardware["cycles"] = _configure_cycles_device(scene)
            camera_roles = ("medium", "three_quarter", "wide") if engine == "eevee" else ("medium",)
            for camera_role in camera_roles:
                stem = f"{engine}-{mode}-{camera_role.replace('_', '-')}"
                png_path = destination / f"{stem}.png"
                exr_path = linear_dir / f"{stem}.exr"
                seconds, camera_metadata = _render_display_and_linear(
                    scene=scene,
                    camera=mode_objects["cameras"][camera_role],
                    engine=engine,
                    png_path=png_path,
                    exr_path=exr_path,
                    resolution=resolution,
                )
                render_records.append(
                    {
                        "mode": mode,
                        "engine": engine,
                        "cameraRole": camera_role,
                        "emptyRoom": False,
                        "displayPath": str(png_path),
                        "linearPath": str(exr_path),
                        "renderSeconds": seconds,
                        **camera_metadata,
                    }
                )
                if engine == "eevee":
                    eevee_camera_outputs[camera_role] = (png_path, camera_metadata)
                if camera_role == "medium":
                    mode_outputs[(engine, False)] = (png_path, exr_path)

            original_hide = {obj: bool(obj.hide_render) for obj in character_objects}
            try:
                for obj in character_objects:
                    obj.hide_render = True
                stem = f"{engine}-{mode}-medium-empty"
                png_path = destination / f"{stem}.png"
                exr_path = linear_dir / f"{stem}.exr"
                seconds, camera_metadata = _render_display_and_linear(
                    scene=scene,
                    camera=medium_camera,
                    engine=engine,
                    png_path=png_path,
                    exr_path=exr_path,
                    resolution=resolution,
                )
            finally:
                for obj, hidden in original_hide.items():
                    obj.hide_render = hidden
            render_records.append(
                {
                    "mode": mode,
                    "engine": engine,
                    "cameraRole": "medium",
                    "emptyRoom": True,
                    "displayPath": str(png_path),
                    "linearPath": str(exr_path),
                    "renderSeconds": seconds,
                    **camera_metadata,
                }
            )
            mode_outputs[(engine, True)] = (png_path, exr_path)

        medium_display, medium_metadata = eevee_camera_outputs["medium"]
        medium_rgba, medium_size = _read_rgba(medium_display)
        if medium_size != resolution:
            raise RuntimeError(f"camera comparison image size mismatch: {medium_size}")
        for camera_role in ("three_quarter", "wide"):
            comparison_display, comparison_metadata = eevee_camera_outputs[camera_role]
            comparison_rgba, comparison_size = _read_rgba(comparison_display)
            if comparison_size != resolution:
                raise RuntimeError(
                    f"camera comparison image size mismatch: {comparison_size}"
                )
            camera_comparisons.append(
                {
                    "mode": mode,
                    "engine": "eevee",
                    "cameraRoleA": "medium",
                    "cameraRoleB": camera_role,
                    "cameraA": medium_metadata["requestedCamera"],
                    "cameraB": comparison_metadata["requestedCamera"],
                    "activeCameraA": medium_metadata["activeCamera"],
                    "activeCameraB": comparison_metadata["activeCamera"],
                    "matrixWorldA": medium_metadata["matrixWorld"],
                    "matrixWorldB": comparison_metadata["matrixWorld"],
                    "pixelMae": _mean_absolute_rgb_difference(
                        medium_rgba.reshape(-1),
                        comparison_rgba.reshape(-1),
                    ),
                    "imageA": str(medium_display),
                    "imageB": str(comparison_display),
                    "imageASha256": _sha256(medium_display),
                    "imageBSha256": _sha256(comparison_display),
                }
            )

        matte_path = mask_dir / f"{mode}-subject-id-matte.png"
        practical_objects = _practical_highlight_objects(scene)
        matte_seconds, matte_camera_metadata = _render_subject_id_matte(
            scene=scene,
            camera=medium_camera,
            character_objects=character_objects,
            practical_objects=practical_objects,
            output_path=matte_path,
            resolution=resolution,
        )
        matte_rgba, matte_size = _read_rgba(matte_path)
        if matte_size != resolution:
            raise RuntimeError(f"subject ID matte size mismatch: {matte_size} != {resolution}")
        subject_mask = subject_mask_from_rendered_id_matte(matte_rgba.reshape(-1))
        practical_highlight_mask = practical_highlight_mask_from_rendered_id_matte(
            matte_rgba.reshape(-1)
        )
        face_mask = face_mask_from_projected_head_triangles(
            subject_mask,
            width=resolution[0],
            height=resolution[1],
            projected_head_triangles=projected_head_triangles,
        )
        subject_mask_path = mask_dir / f"{mode}-subject-mask.png"
        face_mask_path = mask_dir / f"{mode}-face-mask.png"
        practical_mask_path = mask_dir / f"{mode}-practical-highlight-mask.png"
        _write_mask_png(subject_mask_path, subject_mask, *resolution)
        _write_mask_png(face_mask_path, face_mask, *resolution)
        _write_mask_png(
            practical_mask_path,
            practical_highlight_mask,
            *resolution,
        )
        projected_head_points = [
            point
            for triangle in projected_head_triangles
            for point in triangle
        ]
        projected_bounds = {
            "min": [
                min(value[0] for value in projected_head_points),
                min(value[1] for value in projected_head_points),
            ],
            "max": [
                max(value[0] for value in projected_head_points),
                max(value[1] for value in projected_head_points),
            ],
            "triangleCount": len(projected_head_triangles),
        }

        for engine in ("eevee", "cycles"):
            beauty_png, beauty_exr = mode_outputs[(engine, False)]
            empty_png, empty_exr = mode_outputs[(engine, True)]
            linear_rgba, linear_size = _read_rgba(beauty_exr)
            display_rgba, display_size = _read_rgba(beauty_png)
            _empty_rgba, empty_size = _read_rgba(empty_exr)
            if {linear_size, display_size, empty_size} != {resolution}:
                raise RuntimeError(
                    f"lighting evidence image size mismatch for {mode}/{engine}: "
                    f"{linear_size}, {display_size}, {empty_size}"
                )
            background_mask = background_mask_from_geometry_masks(
                subject_mask=subject_mask,
                practical_highlight_mask=practical_highlight_mask,
                display_rgba=display_rgba.reshape(-1),
                width=resolution[0],
                height=resolution[1],
            )
            background_mask_path = mask_dir / f"{mode}-{engine}-background-mask.png"
            _write_mask_png(background_mask_path, background_mask, *resolution)
            evidence = measure_lighting_evidence(
                linear_rgba=linear_rgba.reshape(-1),
                display_rgba=display_rgba.reshape(-1),
                subject_mask=subject_mask,
                face_mask=face_mask,
                background_mask=background_mask,
                width=resolution[0],
                height=resolution[1],
            )
            artifact_paths = {
                "beautyPath": str(beauty_png),
                "linearBeautyPath": str(beauty_exr),
                "emptyRoomPath": str(empty_png),
                "linearEmptyRoomPath": str(empty_exr),
                "subjectMaskPath": str(subject_mask_path),
                "faceMaskPath": str(face_mask_path),
                "backgroundMaskPath": str(background_mask_path),
                "practicalHighlightMaskPath": str(practical_mask_path),
                "subjectMattePath": str(matte_path),
            }
            measurements.append(
                {
                    "mode": mode,
                    "engine": engine,
                    "cameraRole": "medium",
                    **evidence,
                    **artifact_paths,
                    "artifactSha256": {
                        key: _sha256(Path(path)) for key, path in artifact_paths.items()
                    },
                    "practicalHighlightPixelCount": sum(practical_highlight_mask),
                    "subjectMatteRenderSeconds": matte_seconds,
                    "subjectMatteCamera": matte_camera_metadata,
                    "projectedHeadBounds": projected_bounds,
                }
            )

    payload: dict[str, object] = {
        "schemaVersion": LIGHTING_EVIDENCE_SCHEMA,
        "scene": str(scene_path),
        "sceneSha256": _sha256(scene_path),
        "canonicalMaster": str(master_path),
        "canonicalMasterSha256": _sha256(master_path),
        "luminanceColorSpace": "scene_linear_rec709",
        "displayColorSpace": "AgX Medium High Contrast PNG",
        "subjectMaskSource": SUBJECT_MATTE_SOURCE,
        "faceMaskSource": FACE_MASK_SOURCE,
        "backgroundMaskSource": BACKGROUND_MASK_SOURCE,
        "hardware": hardware,
        "renders": render_records,
        "cameraComparisons": camera_comparisons,
        "measurements": measurements,
    }
    errors = studio_validator.validate_lighting_evidence_payload(payload)
    payload["errors"] = errors
    payload["success"] = not errors
    report_path = destination / LIGHTING_EVIDENCE_FILENAME
    report_path.write_text(
        json.dumps(payload, ensure_ascii=False, indent=2),
        encoding="utf-8",
    )
    payload["reportPath"] = str(report_path)
    return payload


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
    parser.add_argument(
        "--subject-evidence",
        action="store_true",
        help="Render the full canonical-master Task 6 Eevee/Cycles evidence matrix",
    )
    parser.add_argument("--master", type=Path, help="Canonical master .blend")
    parser.add_argument("--frame", type=int, default=29)
    parser.add_argument("--width", type=int, default=960)
    parser.add_argument("--height", type=int, default=540)
    raw_argv = sys.argv if argv is None else argv
    args = parser.parse_args(_arguments_after_separator(raw_argv))
    _require_bpy()
    blend_path = args.blend_path.expanduser().resolve()
    if not blend_path.is_file():
        parser.error(f"blend file does not exist: {blend_path}")
    bpy.ops.wm.open_mainfile(filepath=str(blend_path))
    if args.subject_evidence:
        if args.master is None:
            parser.error("--subject-evidence requires --master")
        if args.width <= 0 or args.height <= 0:
            parser.error("--width and --height must be positive")
        report = render_subject_lighting_evidence(
            scene_path=blend_path,
            master_path=args.master,
            output_dir=args.output_dir,
            resolution=(args.width, args.height),
            frame=args.frame,
        )
        print("WARM_STUDIO_LIGHTING_EVIDENCE=" + json.dumps(report, ensure_ascii=False))
        return 0 if report["success"] else 2
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
