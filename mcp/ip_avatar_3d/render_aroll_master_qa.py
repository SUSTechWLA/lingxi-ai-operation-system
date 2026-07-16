#!/usr/bin/env python3
"""Render deterministic visual QA samples from a prepared A-roll master."""

from __future__ import annotations

import json
import shutil
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable, Mapping

try:
    import bpy
    from mathutils import Vector
except ModuleNotFoundError:  # pragma: no cover - manifest helpers are importable outside Blender.
    bpy = None  # type: ignore[assignment]
    Vector = None  # type: ignore[assignment,misc]

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import aroll_actions

try:
    import blender_renderer
except ModuleNotFoundError:  # pragma: no cover - Blender-only implementation dependency.
    blender_renderer = None  # type: ignore[assignment]


FACE_CAPABILITY = "squint_only"
ACTION_CAMERAS = ("Camera_Medium", "Camera_Wide")
AROLL_ACTIONS = tuple(aroll_actions.AROLL_ACTIONS)
HAND_CLOSE_CAMERA_RIGHT = "QA_Hand_Close.R"
HAND_CLOSE_CAMERA_LEFT = "QA_Hand_Close.L"
FACE_CLOSE_CAMERA = "QA_Face_Close"
REPORT_NAME = "qa-report.json"
MIN_OPEN_FIST_PIXEL_DIFFERENCE = 0.012
MIN_FINGER_ROLL_PIXEL_DIFFERENCE = 0.0015
MIN_SILHOUETTE_COVERAGE = 0.001
MAX_SILHOUETTE_COVERAGE = 0.96


@dataclass(frozen=True)
class QASample:
    label: str
    kind: str
    action: str
    camera: str
    path: str
    frame: int = 30
    side: str = ""
    digit: int = 0
    shape_keys: tuple[tuple[str, float], ...] = ()


HAND_SAMPLES = (
    QASample("open", "hand", "Gesture_OpenHand", HAND_CLOSE_CAMERA_RIGHT, "hand/open.png"),
    QASample("fist", "hand", "Gesture_Fist", HAND_CLOSE_CAMERA_RIGHT, "hand/fist.png"),
    QASample("pinch", "hand", "Gesture_Pinch", HAND_CLOSE_CAMERA_RIGHT, "hand/pinch.png"),
    QASample("count_1", "hand", "Gesture_Count_One", HAND_CLOSE_CAMERA_RIGHT, "hand/count_1.png"),
    QASample("count_2", "hand", "Gesture_Count_Two", HAND_CLOSE_CAMERA_RIGHT, "hand/count_2.png"),
    QASample("count_3", "hand", "Gesture_Count_Three", HAND_CLOSE_CAMERA_RIGHT, "hand/count_3.png"),
)

DIGIT_SAMPLES = tuple(
    QASample(
        f"finger_roll_{side}_{digit}",
        "digit",
        f"QA_FingerRoll_{side.upper()}_{digit}",
        HAND_CLOSE_CAMERA_LEFT if side == "l" else HAND_CLOSE_CAMERA_RIGHT,
        f"hand/finger_roll_{side}_{digit}.png",
        side=side,
        digit=digit,
    )
    for side in ("r", "l")
    for digit in (1, 2, 3)
)

FACE_SAMPLES = (
    QASample(
        "neutral",
        "face",
        "Face_Neutral",
        FACE_CLOSE_CAMERA,
        "face/neutral.png",
        shape_keys=(("Mouth_Rest", 1.0),),
    ),
    QASample(
        "happy",
        "face",
        "Face_Happy",
        FACE_CLOSE_CAMERA,
        "face/happy.png",
        shape_keys=(("Mouth_Smile", 0.8),),
    ),
    QASample(
        "serious",
        "face",
        "Face_Serious",
        FACE_CLOSE_CAMERA,
        "face/serious.png",
        shape_keys=(("Mouth_Frown", 0.7),),
    ),
    QASample(
        "squint",
        "face",
        "Face_Squint",
        FACE_CLOSE_CAMERA,
        "face/squint.png",
        shape_keys=(("Eye_Squint.L", 1.0), ("Eye_Squint.R", 1.0)),
    ),
    QASample("A", "face", "Mouth_A", FACE_CLOSE_CAMERA, "face/A.png", shape_keys=(("Mouth_A", 1.0),)),
    QASample("E", "face", "Mouth_E", FACE_CLOSE_CAMERA, "face/E.png", shape_keys=(("Mouth_E", 1.0),)),
    QASample("O", "face", "Mouth_O", FACE_CLOSE_CAMERA, "face/O.png", shape_keys=(("Mouth_O", 1.0),)),
    QASample(
        "MBP",
        "face",
        "Mouth_MBP",
        FACE_CLOSE_CAMERA,
        "face/MBP.png",
        shape_keys=(("Mouth_MBP", 1.0),),
    ),
)

ACTION_SAMPLES = tuple(
    QASample(
        f"{action}_{camera}",
        "action",
        action,
        camera,
        f"actions/{action}/{camera}.png",
    )
    for action in AROLL_ACTIONS
    for camera in ACTION_CAMERAS
)

QA_SAMPLES = (*HAND_SAMPLES, *DIGIT_SAMPLES, *FACE_SAMPLES, *ACTION_SAMPLES)
REQUIRED_ACTIONS = frozenset(
    {*AROLL_ACTIONS, *(sample.action for sample in HAND_SAMPLES)}
)
REQUIRED_SHAPE_KEYS = frozenset(
    shape_name
    for sample in FACE_SAMPLES
    for shape_name, _value in sample.shape_keys
)
REQUIRED_BONE_ROLES = frozenset(
    {
        "head",
        "hand_l",
        "hand_r",
        *(
            role
            for side in ("l", "r")
            for digit in (1, 2, 3)
            for role in aroll_actions.chain_roles(side, digit).values()
        ),
    }
)


def _require_blender() -> None:
    if bpy is None or Vector is None or blender_renderer is None:
        raise RuntimeError("A-roll master QA requires Blender's bpy runtime")


def required_relative_paths() -> tuple[str, ...]:
    return tuple(sample.path for sample in QA_SAMPLES)


def _shape_key_owners(scene: Any) -> dict[str, Any]:
    owners: dict[str, Any] = {}
    for obj in sorted(scene.objects, key=lambda item: item.name):
        shape_keys = getattr(getattr(obj, "data", None), "shape_keys", None)
        if not shape_keys:
            continue
        for key in shape_keys.key_blocks:
            owners.setdefault(key.name, obj)
    return owners


def _belongs_to_armature(obj: Any, armature: Any) -> bool:
    current = obj
    while current is not None:
        if current == armature:
            return True
        current = current.parent
    return any(
        modifier.type == "ARMATURE" and modifier.object == armature
        for modifier in getattr(obj, "modifiers", ())
    )


def _character_meshes(scene: Any, armature: Any) -> list[Any]:
    master_collection = bpy.data.collections.get("IP_Character_Master")
    master_objects = set(master_collection.all_objects) if master_collection else set()
    return [
        obj
        for obj in scene.objects
        if obj.type == "MESH"
        and (obj in master_objects or _belongs_to_armature(obj, armature))
    ]


def _decode_bone_map(armature: Any) -> dict[str, str]:
    encoded = armature.get("ip_avatar_bone_map")
    if not encoded:
        raise RuntimeError("A-roll QA contract failed: Armature is missing ip_avatar_bone_map")
    try:
        decoded = json.loads(str(encoded))
    except (TypeError, ValueError) as exc:
        raise RuntimeError("A-roll QA contract failed: ip_avatar_bone_map is not valid JSON") from exc
    if not isinstance(decoded, dict) or not all(
        isinstance(role, str) and isinstance(name, str) for role, name in decoded.items()
    ):
        raise RuntimeError("A-roll QA contract failed: ip_avatar_bone_map must be a string map")
    return decoded


def validate_scene_contract(scene: Any | None = None) -> dict[str, Any]:
    """Validate all reusable assets before rendering any QA output."""
    _require_blender()
    scene = scene or bpy.context.scene
    violations: list[str] = []
    armatures = [obj for obj in scene.objects if obj.type == "ARMATURE"]
    if len(armatures) != 1:
        violations.append(f"expected exactly one Armature, found {len(armatures)}")
        armature = armatures[0] if armatures else None
    else:
        armature = armatures[0]

    missing_actions = sorted(name for name in REQUIRED_ACTIONS if bpy.data.actions.get(name) is None)
    if missing_actions:
        violations.append(f"missing required Actions: {', '.join(missing_actions)}")

    missing_cameras = sorted(
        name
        for name in ACTION_CAMERAS
        if scene.objects.get(name) is None or scene.objects[name].type != "CAMERA"
    )
    if missing_cameras:
        violations.append(f"missing required cameras: {', '.join(missing_cameras)}")

    shape_owners = _shape_key_owners(scene)
    missing_shapes = sorted(REQUIRED_SHAPE_KEYS - set(shape_owners))
    if missing_shapes:
        violations.append(f"missing required Shape Keys: {', '.join(missing_shapes)}")
    squint_owners = {
        shape_owners[name]
        for name in ("Eye_Squint.L", "Eye_Squint.R")
        if name in shape_owners
    }
    if squint_owners and not all(
        str(owner.get("blink_capability") or "") == FACE_CAPABILITY for owner in squint_owners
    ):
        violations.append("Eye_Squint Shape Keys must declare blink_capability=squint_only")

    bone_map: dict[str, str] = {}
    if armature is not None:
        try:
            bone_map = _decode_bone_map(armature)
        except RuntimeError as exc:
            violations.append(str(exc).split(": ", 1)[-1])
        missing_roles = sorted(
            role
            for role in REQUIRED_BONE_ROLES
            if role not in bone_map or armature.pose.bones.get(bone_map[role]) is None
        )
        if missing_roles:
            violations.append(f"missing required bone roles: {', '.join(missing_roles)}")

    character_meshes = _character_meshes(scene, armature) if armature is not None else []
    if armature is not None and not character_meshes:
        violations.append("Armature has no renderable character meshes")

    if violations:
        raise RuntimeError("A-roll QA contract failed:\n- " + "\n- ".join(violations))
    return {
        "armature": armature,
        "boneMap": bone_map,
        "shapeKeyOwners": shape_owners,
        "characterMeshes": character_meshes,
        "actions": list(AROLL_ACTIONS),
        "cameras": list(ACTION_CAMERAS),
    }


def _reset_armature_pose(scene: Any, armature: Any) -> None:
    armature.animation_data_create()
    armature.animation_data.action = None
    scene.frame_set(1)
    for pose_bone in armature.pose.bones:
        pose_bone.matrix_basis.identity()
        pose_bone.rotation_mode = "XYZ"


def _activate_shape_keys(shape_owners: Mapping[str, Any], values: Mapping[str, float]) -> None:
    seen: set[int] = set()
    for owner in shape_owners.values():
        if id(owner) in seen:
            continue
        seen.add(id(owner))
        shape_keys = owner.data.shape_keys
        if shape_keys.animation_data:
            shape_keys.animation_data.action = None
        for key in shape_keys.key_blocks:
            if key.name != "Basis":
                key.value = 0.0
    for name, value in values.items():
        owner = shape_owners[name]
        owner.data.shape_keys.key_blocks[name].value = float(value)


def _apply_open_pose(armature: Any, bone_map: dict[str, str]) -> None:
    for side in ("l", "r"):
        blender_renderer.apply_hand_pose(
            armature.pose.bones,
            bone_map,
            side,
            aroll_actions.hand_pose("open_hand"),
        )


def _set_action_sample(scene: Any, armature: Any, action_name: str, frame: int) -> None:
    _reset_armature_pose(scene, armature)
    action = bpy.data.actions.get(action_name)
    if action is None:
        raise RuntimeError(f"A-roll QA contract changed during render: missing Action {action_name}")
    armature.animation_data.action = action
    scene.frame_set(int(frame))
    bpy.context.view_layer.update()


def _set_digit_sample(
    scene: Any,
    armature: Any,
    bone_map: dict[str, str],
    side: str,
    digit: int,
) -> None:
    staging_action = "Gesture_OpenHand" if side == "r" else "Aroll_Explain_Left"
    _set_action_sample(scene, armature, staging_action, 30)
    staged_basis = {
        pose_bone.name: pose_bone.matrix_basis.copy() for pose_bone in armature.pose.bones
    }
    armature.animation_data.action = None
    for pose_bone in armature.pose.bones:
        pose_bone.matrix_basis = staged_basis[pose_bone.name]
    blender_renderer.apply_hand_pose(
        armature.pose.bones,
        bone_map,
        side,
        aroll_actions.finger_roll_pose(digit),
    )
    bpy.context.view_layer.update()


def _pose_points(armature: Any, bone_map: dict[str, str], side: str) -> list[Any]:
    points = []
    roles = [f"hand_{side}"] + [
        role
        for digit in (1, 2, 3)
        for role in aroll_actions.chain_roles(side, digit).values()
    ]
    for role in roles:
        pose_bone = armature.pose.bones[bone_map[role]]
        points.extend((armature.matrix_world @ pose_bone.head, armature.matrix_world @ pose_bone.tail))
    return points


def _get_or_create_camera(name: str) -> Any:
    existing = bpy.data.objects.get(name)
    if existing is not None and existing.type != "CAMERA":
        raise RuntimeError(f"A-roll QA cannot create {name}: name belongs to a non-camera object")
    if existing is not None:
        if bpy.context.scene.objects.get(existing.name) is None:
            bpy.context.scene.collection.objects.link(existing)
        return existing
    data = bpy.data.cameras.new(f"{name}_Data")
    camera = bpy.data.objects.new(name, data)
    bpy.context.scene.collection.objects.link(camera)
    return camera


def _frame_camera(name: str, points: Iterable[Any], *, lens: float = 70.0) -> Any:
    points = list(points)
    if not points:
        raise RuntimeError(f"A-roll QA cannot frame {name}: no target points")
    center = sum(points, Vector((0.0, 0.0, 0.0))) / len(points)
    radius = max((point - center).length for point in points)
    distance = max(0.45, radius * 5.5)
    camera = _get_or_create_camera(name)
    camera.data.lens = lens
    camera.data.clip_start = 0.01
    camera.data.clip_end = max(100.0, distance * 10.0)
    camera.location = center + Vector((0.0, -distance, radius * 0.08))
    camera.rotation_euler = (center - camera.location).to_track_quat("-Z", "Y").to_euler()
    return camera


def _configure_close_cameras(scene: Any, armature: Any, bone_map: dict[str, str]) -> None:
    _set_action_sample(scene, armature, "Gesture_OpenHand", 30)
    _frame_camera(HAND_CLOSE_CAMERA_RIGHT, _pose_points(armature, bone_map, "r"))
    _set_action_sample(scene, armature, "Aroll_Explain_Left", 30)
    _frame_camera(HAND_CLOSE_CAMERA_LEFT, _pose_points(armature, bone_map, "l"))

    _reset_armature_pose(scene, armature)
    bpy.context.view_layer.update()
    head = armature.pose.bones[bone_map["head"]]
    target = armature.matrix_world @ ((head.head + head.tail) * 0.5)
    rig_points = [
        armature.matrix_world @ point
        for bone in armature.data.bones
        for point in (bone.head_local, bone.tail_local)
    ]
    height = max(point.z for point in rig_points) - min(point.z for point in rig_points)
    face_radius = max(0.08, height * 0.13)
    face_points = [
        target + Vector((x * face_radius, 0.0, z * face_radius))
        for x in (-1.0, 1.0)
        for z in (-1.0, 1.0)
    ]
    _frame_camera(FACE_CLOSE_CAMERA, face_points, lens=72.0)


def _hand_relative_tip(armature: Any, bone_map: dict[str, str], side: str, digit: int) -> Any:
    hand = armature.pose.bones[bone_map[f"hand_{side}"]]
    distal = armature.pose.bones[bone_map[f"finger_{digit}_tip_{side}"]]
    armature_space = hand.matrix.inverted() @ distal.tail
    return armature.matrix_world.to_3x3() @ armature_space


def _digit_chain_length(armature: Any, bone_map: dict[str, str], side: str, digit: int) -> float:
    world_scale = armature.matrix_world.to_3x3()
    return sum(
        (world_scale @ (bone.tail_local - bone.head_local)).length
        for bone in (
            armature.data.bones[bone_map[role]]
            for role in aroll_actions.chain_roles(side, digit).values()
        )
    )


def _tip_displacement_ratios(
    armature: Any,
    bone_map: dict[str, str],
    baseline: Mapping[tuple[str, int], Any],
) -> dict[str, float]:
    return {
        f"{side}{digit}": round(
            (_hand_relative_tip(armature, bone_map, side, digit) - baseline[side, digit]).length
            / max(_digit_chain_length(armature, bone_map, side, digit), 1e-9),
            6,
        )
        for side in ("l", "r")
        for digit in (1, 2, 3)
    }


def _bone_rotations(armature: Any, bone_map: dict[str, str]) -> dict[str, list[float]]:
    return {
        role: [round(float(value), 6) for value in armature.pose.bones[name].matrix_basis.to_euler("XYZ")]
        for role, name in sorted(bone_map.items())
        if armature.pose.bones.get(name) is not None
    }


def _camera_metrics(camera: Any) -> dict[str, Any]:
    return {
        "lensMm": round(float(camera.data.lens), 4),
        "location": [round(float(value), 6) for value in camera.location],
        "rotationEuler": [round(float(value), 6) for value in camera.rotation_euler],
    }


def _configure_render(scene: Any, resolution: int) -> None:
    resolution = max(64, int(resolution))
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"
    scene.render.image_settings.color_depth = "8"
    scene.render.film_transparent = True
    scene.render.resolution_x = resolution
    scene.render.resolution_y = resolution


def _configure_qa_lighting(scene: Any, character_meshes: Iterable[Any]) -> dict[str, Any]:
    points = [
        obj.matrix_world @ Vector(corner)
        for obj in character_meshes
        for corner in obj.bound_box
    ]
    if not points:
        raise RuntimeError("A-roll QA lighting requires character bounds")
    minimum = Vector((min(point.x for point in points), min(point.y for point in points), min(point.z for point in points)))
    maximum = Vector((max(point.x for point in points), max(point.y for point in points), max(point.z for point in points)))
    center = (minimum + maximum) * 0.5
    height = max(0.25, float(maximum.z - minimum.z))
    width = max(0.25, float(maximum.x - minimum.x))

    for light in (obj for obj in scene.objects if obj.type == "LIGHT"):
        light.hide_render = True
    for name in ("QA_Key_Softbox", "QA_Fill_Softbox", "QA_Rim_Softbox"):
        existing = bpy.data.objects.get(name)
        if existing:
            bpy.data.objects.remove(existing, do_unlink=True)

    specifications = (
        (
            "QA_Key_Softbox",
            center + Vector((-width * 0.85, -height * 1.35, height * 0.72)),
            340.0,
            (1.0, 0.86, 0.72),
            height * 0.72,
        ),
        (
            "QA_Fill_Softbox",
            center + Vector((width * 0.95, -height * 1.05, height * 0.38)),
            170.0,
            (0.72, 0.86, 1.0),
            height * 0.86,
        ),
        (
            "QA_Rim_Softbox",
            center + Vector((0.0, height * 0.72, height * 0.78)),
            220.0,
            (1.0, 0.72, 0.48),
            height * 0.62,
        ),
    )
    names: list[str] = []
    for name, location, energy, color, size in specifications:
        data = bpy.data.lights.new(name, type="AREA")
        data.energy = energy
        data.shape = "DISK"
        data.size = max(0.35, size)
        data.color = color
        light = bpy.data.objects.new(name, data)
        scene.collection.objects.link(light)
        light.location = location
        blender_renderer.look_at(light, center)
        names.append(name)
    return {
        "preset": "qa_editorial_soft",
        "lightCount": len(names),
        "lights": names,
    }


def _render_sample(
    scene: Any,
    sample: QASample,
    output_dir: Path,
    contract: Mapping[str, Any],
    baseline: Mapping[tuple[str, int], Any],
    resolution: int,
) -> dict[str, Any]:
    armature = contract["armature"]
    bone_map = contract["boneMap"]
    shape_owners = contract["shapeKeyOwners"]
    _activate_shape_keys(shape_owners, {"Mouth_Rest": 1.0})
    if sample.kind in {"hand", "action"}:
        _set_action_sample(scene, armature, sample.action, sample.frame)
    elif sample.kind == "digit":
        _set_digit_sample(scene, armature, bone_map, sample.side, sample.digit)
    elif sample.kind == "face":
        _reset_armature_pose(scene, armature)
        _activate_shape_keys(shape_owners, dict(sample.shape_keys))
        bpy.context.view_layer.update()
    else:  # pragma: no cover - the frozen manifest cannot reach this branch.
        raise RuntimeError(f"unknown A-roll QA sample kind: {sample.kind}")

    camera = scene.objects.get(sample.camera)
    if camera is None or camera.type != "CAMERA":
        raise RuntimeError(f"A-roll QA camera disappeared during render: {sample.camera}")
    scene.camera = camera
    _configure_render(scene, resolution)
    if sample.kind == "action":
        scene.render.resolution_y = max(36, int(round(scene.render.resolution_x * 9.0 / 16.0)))
    path = output_dir / sample.path
    path.parent.mkdir(parents=True, exist_ok=True)
    scene.render.filepath = str(path)
    bpy.ops.render.render(write_still=True)
    return {
        "label": sample.label,
        "kind": sample.kind,
        "action": sample.action,
        "camera": sample.camera,
        "frame": sample.frame,
        "path": sample.path,
        "boneRotations": _bone_rotations(armature, bone_map),
        "fingertipDisplacementRatios": _tip_displacement_ratios(
            armature, bone_map, baseline
        ),
        "shapeKeys": dict(sample.shape_keys),
        "framing": _camera_metrics(camera),
    }


def _read_alpha_mask(path: Path) -> tuple[int, int, tuple[bool, ...]]:
    _require_blender()
    image = bpy.data.images.load(str(path), check_existing=False)
    try:
        width, height = (int(value) for value in image.size)
        pixels = image.pixels[:]
        mask = tuple(float(pixels[index]) >= 0.5 for index in range(3, len(pixels), 4))
        return width, height, mask
    finally:
        bpy.data.images.remove(image)


def silhouette_metrics(path: Path) -> dict[str, Any]:
    width, height, mask = _read_alpha_mask(path)
    active = [index for index, value in enumerate(mask) if value]
    if not active:
        return {"coverage": 0.0, "bounds": None, "borderFraction": 0.0}
    xs = [index % width for index in active]
    ys = [index // width for index in active]
    border = sum(
        1
        for x, y in zip(xs, ys)
        if x in {0, width - 1} or y in {0, height - 1}
    )
    return {
        "coverage": round(len(active) / len(mask), 6),
        "bounds": [min(xs), min(ys), max(xs), max(ys)],
        "borderFraction": round(border / len(active), 6),
    }


def alpha_mask_difference(first: Path, second: Path) -> float:
    first_width, first_height, first_mask = _read_alpha_mask(first)
    second_width, second_height, second_mask = _read_alpha_mask(second)
    if (first_width, first_height) != (second_width, second_height):
        raise RuntimeError(
            "alpha-mask comparison requires equal dimensions: "
            f"{first.name}={first_width}x{first_height}, "
            f"{second.name}={second_width}x{second_height}"
        )
    return round(
        sum(left != right for left, right in zip(first_mask, second_mask)) / len(first_mask),
        6,
    )


def frame_md5(path: Path, ffmpeg: str | None = None) -> str:
    executable = ffmpeg or shutil.which("ffmpeg")
    if not executable:
        raise RuntimeError("A-roll QA duplicate-frame checks require ffmpeg on PATH")
    completed = subprocess.run(
        [
            executable,
            "-v",
            "error",
            "-i",
            str(path),
            "-frames:v",
            "1",
            "-pix_fmt",
            "rgba",
            "-f",
            "framemd5",
            "-",
        ],
        check=False,
        capture_output=True,
        text=True,
    )
    if completed.returncode != 0:
        detail = completed.stderr.strip() or "unknown ffmpeg error"
        raise RuntimeError(f"framemd5 failed for {path}: {detail}")
    rows = [line for line in completed.stdout.splitlines() if line and not line.startswith("#")]
    if len(rows) != 1:
        raise RuntimeError(f"framemd5 returned {len(rows)} frame rows for {path}, expected 1")
    return rows[0].rsplit(",", 1)[-1].strip()


def assert_adjacent_frames_unique(paths: Iterable[Path], label: str) -> list[str]:
    paths = list(paths)
    hashes = [frame_md5(path) for path in paths]
    duplicates = [
        f"{paths[index - 1].name} == {paths[index].name}"
        for index in range(1, len(paths))
        if hashes[index - 1] == hashes[index]
    ]
    if duplicates:
        raise RuntimeError(f"duplicate adjacent {label} frames: {', '.join(duplicates)}")
    return hashes


def require_files(output_dir: Path, relative_paths: Iterable[str]) -> None:
    missing = sorted(
        relative_path
        for relative_path in relative_paths
        if not (output_dir / relative_path).is_file()
    )
    if missing:
        raise RuntimeError("A-roll QA required files are missing: " + ", ".join(missing))


def _comparison_metrics(output_dir: Path) -> dict[str, Any]:
    open_fist = alpha_mask_difference(output_dir / "hand/open.png", output_dir / "hand/fist.png")
    if open_fist < MIN_OPEN_FIST_PIXEL_DIFFERENCE:
        raise RuntimeError(
            f"open/fist alpha-mask difference {open_fist:.6f} is below "
            f"{MIN_OPEN_FIST_PIXEL_DIFFERENCE:.6f}"
        )
    finger_roll: dict[str, float] = {}
    for side in ("r", "l"):
        for first, second in ((1, 2), (2, 3)):
            key = f"{side}{first}-{side}{second}"
            difference = alpha_mask_difference(
                output_dir / f"hand/finger_roll_{side}_{first}.png",
                output_dir / f"hand/finger_roll_{side}_{second}.png",
            )
            finger_roll[key] = difference
            if difference < MIN_FINGER_ROLL_PIXEL_DIFFERENCE:
                raise RuntimeError(
                    f"finger-roll phase {key} alpha-mask difference {difference:.6f} is below "
                    f"{MIN_FINGER_ROLL_PIXEL_DIFFERENCE:.6f}"
                )
    return {"openFistPixelDifference": open_fist, "fingerRollPixelDifferences": finger_roll}


def _duplicate_metrics(output_dir: Path) -> dict[str, list[str]]:
    groups = {
        "hand": [sample for sample in HAND_SAMPLES],
        "fingerRollRight": [sample for sample in DIGIT_SAMPLES if sample.side == "r"],
        "fingerRollLeft": [sample for sample in DIGIT_SAMPLES if sample.side == "l"],
        "face": [sample for sample in FACE_SAMPLES],
        "actions": [sample for sample in ACTION_SAMPLES],
    }
    return {
        label: assert_adjacent_frames_unique(
            [output_dir / sample.path for sample in samples], label
        )
        for label, samples in groups.items()
    }


def run_qa(output_dir: Path | str, *, resolution: int = 640) -> dict[str, Any]:
    """Render and validate the deterministic QA contract in the current Blend."""
    _require_blender()
    output_dir = Path(output_dir).expanduser().resolve()
    output_dir.mkdir(parents=True, exist_ok=True)
    scene = bpy.context.scene
    contract = validate_scene_contract(scene)
    armature = contract["armature"]
    bone_map = contract["boneMap"]
    character_meshes = set(contract["characterMeshes"])
    for obj in scene.objects:
        if obj.type == "MESH" and obj not in character_meshes:
            obj.hide_render = True
    lighting = _configure_qa_lighting(scene, character_meshes)
    _configure_close_cameras(scene, armature, bone_map)

    _reset_armature_pose(scene, armature)
    _apply_open_pose(armature, bone_map)
    bpy.context.view_layer.update()
    baseline = {
        (side, digit): _hand_relative_tip(armature, bone_map, side, digit).copy()
        for side in ("l", "r")
        for digit in (1, 2, 3)
    }
    samples = [
        _render_sample(scene, sample, output_dir, contract, baseline, resolution)
        for sample in QA_SAMPLES
    ]
    require_files(output_dir, required_relative_paths())

    for sample in samples:
        metrics = silhouette_metrics(output_dir / sample["path"])
        coverage = float(metrics["coverage"])
        if not MIN_SILHOUETTE_COVERAGE <= coverage <= MAX_SILHOUETTE_COVERAGE:
            raise RuntimeError(
                f"malformed silhouette for {sample['path']}: coverage={coverage:.6f}"
            )
        sample["silhouette"] = metrics
        sample["frameMd5"] = frame_md5(output_dir / sample["path"])

    comparisons = _comparison_metrics(output_dir)
    duplicates = _duplicate_metrics(output_dir)
    report = {
        "status": "ready",
        "faceCapability": {
            "blinkCapability": FACE_CAPABILITY,
            "qaSample": "face/squint.png",
            "fullBlinkClaimed": False,
        },
        "lighting": lighting,
        "contract": {
            "actions": list(AROLL_ACTIONS),
            "cameras": list(ACTION_CAMERAS),
            "handCloseCameras": [HAND_CLOSE_CAMERA_LEFT, HAND_CLOSE_CAMERA_RIGHT],
            "sampleCount": len(QA_SAMPLES),
            "requiredFiles": list(required_relative_paths()),
        },
        "samples": samples,
        "comparisons": comparisons,
        "framemd5": duplicates,
    }
    report_path = output_dir / REPORT_NAME
    report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"AROLL_MASTER_QA_REPORT={report_path}")
    return report


def main() -> None:
    args = sys.argv[sys.argv.index("--") + 1 :] if "--" in sys.argv else []
    if not args:
        raise RuntimeError(
            "usage: blender master.blend --python render_aroll_master_qa.py -- output_dir"
        )
    run_qa(args[0])


if __name__ == "__main__":
    main()
