#!/usr/bin/env python3
"""Render deterministic visual QA samples from a prepared A-roll master."""

from __future__ import annotations

import hashlib
import json
import os
import shutil
import subprocess
import sys
import tempfile
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
import master_asset

try:
    import blender_renderer
except ModuleNotFoundError:  # pragma: no cover - Blender-only implementation dependency.
    blender_renderer = None  # type: ignore[assignment]


FACE_CAPABILITY = "squint_only"
ACTION_CAMERAS = ("Camera_Medium", "Camera_Wide")
AROLL_ACTIONS = tuple(aroll_actions.AROLL_ACTIONS)
HAND_CLOSE_CAMERA_RIGHT = "QA_Hand_Close.R"
HAND_CLOSE_CAMERA_LEFT = "QA_Hand_Close.L"
HAND_RELAXED_CAMERA_RIGHT = "QA_Hand_Relaxed.R"
HAND_WAVE_CAMERA_RIGHT = "QA_Hand_Wave.R"
FACE_CLOSE_CAMERA = "QA_Face_Close"
REPORT_NAME = "qa-report.json"
QA_REPORT_SCHEMA_VERSION = "tangying-aroll-master-qa/v1"
MIN_OPEN_FIST_PIXEL_DIFFERENCE = master_asset.MIN_OPEN_FIST_PIXEL_DIFFERENCE
MIN_FINGER_ROLL_PIXEL_DIFFERENCE = master_asset.MIN_FINGER_ROLL_PIXEL_DIFFERENCE
MIN_SILHOUETTE_COVERAGE = 0.001
MAX_SILHOUETTE_COVERAGE = 0.96
MIN_HAND_PIXEL_MARGIN = 0.04
MASK_COLORS = {
    "oral_cavity": (0.0, 1.0, 1.0, 1.0),
    "upper_teeth": (1.0, 0.0, 0.0, 1.0),
    "lower_teeth": (1.0, 1.0, 0.0, 1.0),
    "upper_gum": (1.0, 0.0, 1.0, 1.0),
    "lower_gum": (1.0, 1.0, 1.0, 1.0),
    "tongue": (0.0, 1.0, 0.0, 1.0),
    "hand": (0.0, 0.0, 1.0, 1.0),
}
CONTACT_SHEET_LABELS = (
    "Rest",
    "MBP",
    "A",
    "E",
    "O",
    "U",
    "Smile",
    "Surprise",
    "relaxed",
    "open",
    "fist",
    "pinch",
    "count 1",
    "count 2",
    "count 3",
    "point",
    "camera-facing wave",
)


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
    QASample(
        "relaxed",
        "hand",
        "Aroll_Idle_Listening",
        HAND_RELAXED_CAMERA_RIGHT,
        "hand/relaxed.png",
    ),
    QASample("open", "hand", "Gesture_OpenHand", HAND_CLOSE_CAMERA_RIGHT, "hand/open.png"),
    QASample("fist", "hand", "Gesture_Fist", HAND_CLOSE_CAMERA_RIGHT, "hand/fist.png"),
    QASample("pinch", "hand", "Gesture_Pinch", HAND_CLOSE_CAMERA_RIGHT, "hand/pinch.png"),
    QASample("count_1", "hand", "Gesture_Count_One", HAND_CLOSE_CAMERA_RIGHT, "hand/count_1.png"),
    QASample("count_2", "hand", "Gesture_Count_Two", HAND_CLOSE_CAMERA_RIGHT, "hand/count_2.png"),
    QASample("count_3", "hand", "Gesture_Count_Three", HAND_CLOSE_CAMERA_RIGHT, "hand/count_3.png"),
    QASample("point", "hand", "Gesture_Point_Right", HAND_CLOSE_CAMERA_RIGHT, "hand/point.png"),
    QASample(
        "camera_facing_wave",
        "hand",
        "Gesture_Wave",
        HAND_WAVE_CAMERA_RIGHT,
        "hand/camera_facing_wave.png",
    ),
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
        "Rest",
        "face",
        "Face_Neutral",
        FACE_CLOSE_CAMERA,
        "face/Rest.png",
        shape_keys=(("Mouth_Rest", 1.0),),
    ),
    QASample(
        "MBP",
        "face",
        "Mouth_MBP",
        FACE_CLOSE_CAMERA,
        "face/MBP.png",
        shape_keys=(("Mouth_MBP", 1.0),),
    ),
    QASample("A", "face", "Mouth_A", FACE_CLOSE_CAMERA, "face/A.png", shape_keys=(("Mouth_A", 1.0),)),
    QASample("E", "face", "Mouth_E", FACE_CLOSE_CAMERA, "face/E.png", shape_keys=(("Mouth_E", 1.0),)),
    QASample("O", "face", "Mouth_O", FACE_CLOSE_CAMERA, "face/O.png", shape_keys=(("Mouth_O", 1.0),)),
    QASample("U", "face", "Mouth_U", FACE_CLOSE_CAMERA, "face/U.png", shape_keys=(("Mouth_U", 1.0),)),
    QASample(
        "Smile",
        "face",
        "Face_Happy",
        FACE_CLOSE_CAMERA,
        "face/Smile.png",
        shape_keys=(("Mouth_Smile", 1.0),),
    ),
    QASample(
        "Surprise",
        "face",
        "Face_Surprise",
        FACE_CLOSE_CAMERA,
        "face/Surprise.png",
        shape_keys=(("Mouth_Surprise", 1.0),),
    ),
    QASample(
        "Squint",
        "face",
        "Face_Squint",
        FACE_CLOSE_CAMERA,
        "face/Squint.png",
        shape_keys=(("Eye_Squint.L", 1.0), ("Eye_Squint.R", 1.0)),
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
COMPARISON_CROP_LABELS = frozenset(
    {"Rest", "A", "Smile", "open", "fist", "camera_facing_wave"}
)
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


def qa_manifest_payload() -> list[dict[str, Any]]:
    """Return the frozen publication sample manifest."""
    return [
        {
            "label": sample.label,
            "kind": sample.kind,
            "action": sample.action,
            "camera": sample.camera,
            "frame": sample.frame,
            "path": sample.path,
            "side": sample.side,
            "digit": sample.digit,
            "shapeKeys": [list(item) for item in sample.shape_keys],
        }
        for sample in QA_SAMPLES
    ]


def qa_manifest_sha256() -> str:
    encoded = json.dumps(
        qa_manifest_payload(), sort_keys=True, separators=(",", ":")
    ).encode("utf-8")
    return hashlib.sha256(encoded).hexdigest()


def find_ffmpeg() -> str | None:
    for variable in ("TANGYING_FFMPEG_BIN", "FFMPEG_BIN"):
        configured = os.environ.get(variable, "").strip()
        if not configured:
            continue
        candidate = Path(configured).expanduser().resolve()
        if candidate.is_file() and os.access(candidate, os.X_OK):
            return str(candidate)
    return shutil.which("ffmpeg")


def _require_blender() -> None:
    if bpy is None or Vector is None or blender_renderer is None:
        raise RuntimeError("A-roll master QA requires Blender's bpy runtime")


def required_relative_paths() -> tuple[str, ...]:
    return tuple(sample.path for sample in QA_SAMPLES)


def _shape_key_owners(scene: Any) -> dict[str, list[Any]]:
    owners: dict[str, list[Any]] = {}
    for obj in sorted(scene.objects, key=lambda item: item.name):
        shape_keys = getattr(getattr(obj, "data", None), "shape_keys", None)
        if not shape_keys:
            continue
        for key in shape_keys.key_blocks:
            owners.setdefault(key.name, []).append(obj)
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


def validate_scene_contract(
    scene: Any | None = None,
    *,
    samples: Iterable[QASample] | None = None,
) -> dict[str, Any]:
    """Validate all reusable assets before rendering any QA output."""
    _require_blender()
    scene = scene or bpy.context.scene
    selected_samples = tuple(samples) if samples is not None else QA_SAMPLES
    required_actions = {
        sample.action for sample in selected_samples if sample.kind in {"hand", "action"}
    }
    required_shape_keys = {
        shape_name
        for sample in selected_samples
        for shape_name, _value in sample.shape_keys
    }
    violations: list[str] = []
    armatures = [obj for obj in scene.objects if obj.type == "ARMATURE"]
    if len(armatures) != 1:
        violations.append(f"expected exactly one Armature, found {len(armatures)}")
        armature = armatures[0] if armatures else None
    else:
        armature = armatures[0]

    missing_actions = sorted(name for name in required_actions if bpy.data.actions.get(name) is None)
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
    missing_shapes = sorted(required_shape_keys - set(shape_owners))
    if missing_shapes:
        violations.append(f"missing required Shape Keys: {', '.join(missing_shapes)}")
    squint_owners = {
        owner
        for name in ("Eye_Squint.L", "Eye_Squint.R")
        if name in shape_owners
        for owner in shape_owners[name]
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


def _render_selected_samples(
    output_dir: Path,
    samples: Iterable[QASample],
    *,
    resolution: int,
) -> tuple[dict[str, Any], dict[str, Any], list[dict[str, Any]]]:
    scene = bpy.context.scene
    selected_samples = tuple(samples)
    contract = validate_scene_contract(scene, samples=selected_samples)
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
    rendered = [
        _render_sample(scene, sample, output_dir, contract, baseline, resolution)
        for sample in selected_samples
    ]
    for sample in rendered:
        metrics = silhouette_metrics(output_dir / sample["path"])
        coverage = float(metrics["coverage"])
        if not MIN_SILHOUETTE_COVERAGE <= coverage <= MAX_SILHOUETTE_COVERAGE:
            raise RuntimeError(
                f"malformed silhouette for {sample['path']}: coverage={coverage:.6f}"
            )
        sample["silhouette"] = metrics
        sample["frameMd5"] = frame_md5(output_dir / sample["path"])
        sample["sha256"] = hashlib.sha256(
            (output_dir / sample["path"]).read_bytes()
        ).hexdigest()
    return contract, lighting, rendered


def _reset_armature_pose(scene: Any, armature: Any) -> None:
    armature.animation_data_create()
    armature.animation_data.action = None
    scene.frame_set(1)
    for pose_bone in armature.pose.bones:
        pose_bone.matrix_basis.identity()
        pose_bone.rotation_mode = "XYZ"


def _activate_shape_keys(shape_owners: Mapping[str, Any], values: Mapping[str, float]) -> None:
    seen: set[int] = set()
    for owners in shape_owners.values():
        for owner in owners:
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
        for owner in shape_owners[name]:
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


def _hand_surface_points(
    scene: Any,
    armature: Any,
    bone_map: Mapping[str, str],
    side: str,
) -> list[Any]:
    depsgraph = bpy.context.evaluated_depsgraph_get()
    points: list[Any] = []
    for obj in _character_meshes(scene, armature):
        if str(obj.get("ip_face_topology_role") or "") in master_asset.REQUIRED_ORAL_ROLES:
            continue
        target_indices = _hand_face_vertex_indices(obj, bone_map, side)
        if not target_indices:
            continue
        evaluated = obj.evaluated_get(depsgraph)
        mesh = evaluated.to_mesh(preserve_all_data_layers=True)
        owns_mesh = mesh is not None
        if mesh is None:
            mesh = evaluated.data
        try:
            if max(target_indices) >= len(mesh.vertices):
                continue
            points.extend(
                evaluated.matrix_world @ mesh.vertices[index].co
                for index in target_indices
            )
        finally:
            if owns_mesh:
                evaluated.to_mesh_clear()
    return points or _pose_points(armature, dict(bone_map), side)


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


def _frame_camera(
    name: str,
    points: Iterable[Any],
    *,
    lens: float = 70.0,
    distance_scale: float = 5.5,
) -> Any:
    points = list(points)
    if not points:
        raise RuntimeError(f"A-roll QA cannot frame {name}: no target points")
    center = sum(points, Vector((0.0, 0.0, 0.0))) / len(points)
    radius = max((point - center).length for point in points)
    distance = max(0.45, radius * distance_scale)
    camera = _get_or_create_camera(name)
    camera.data.lens = lens
    camera.data.clip_start = 0.01
    camera.data.clip_end = max(100.0, distance * 10.0)
    camera.location = center + Vector((0.0, -distance, radius * 0.08))
    camera.rotation_euler = (center - camera.location).to_track_quat("-Z", "Y").to_euler()
    return camera


def _configure_close_cameras(scene: Any, armature: Any, bone_map: dict[str, str]) -> None:
    _set_action_sample(scene, armature, "Gesture_OpenHand", 30)
    _frame_camera(
        HAND_CLOSE_CAMERA_RIGHT,
        _hand_surface_points(scene, armature, bone_map, "r"),
    )
    _set_action_sample(scene, armature, "Aroll_Explain_Left", 30)
    _frame_camera(
        HAND_CLOSE_CAMERA_LEFT,
        _hand_surface_points(scene, armature, bone_map, "l"),
    )
    _set_action_sample(scene, armature, "Aroll_Idle_Listening", 30)
    _frame_camera(
        HAND_RELAXED_CAMERA_RIGHT,
        _hand_surface_points(scene, armature, bone_map, "r"),
        distance_scale=7.5,
    )
    _set_action_sample(scene, armature, "Gesture_Wave", 30)
    _frame_camera(
        HAND_WAVE_CAMERA_RIGHT,
        _hand_surface_points(scene, armature, bone_map, "r"),
    )

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


def _projected_vertex_count(scene: Any, camera: Any, obj: Any) -> int:
    from bpy_extras.object_utils import world_to_camera_view

    if obj.hide_render or obj.hide_viewport:
        return 0
    evaluated = obj.evaluated_get(bpy.context.evaluated_depsgraph_get())
    mesh = evaluated.to_mesh(preserve_all_data_layers=True)
    owns_mesh = mesh is not None
    if mesh is None:
        mesh = evaluated.data
    try:
        step = max(1, len(mesh.vertices) // 256)
        return sum(
            0.0 <= projected.x <= 1.0
            and 0.0 <= projected.y <= 1.0
            and projected.z >= 0.0
            for index in range(0, len(mesh.vertices), step)
            for vertex in [mesh.vertices[index]]
            for projected in [world_to_camera_view(scene, camera, evaluated.matrix_world @ vertex.co)]
        )
    finally:
        if owns_mesh:
            evaluated.to_mesh_clear()


def _mask_material(name: str, color: tuple[float, float, float, float]) -> Any:
    material = bpy.data.materials.new(name)
    material.diffuse_color = color
    material.use_nodes = True
    nodes = material.node_tree.nodes
    nodes.clear()
    output = nodes.new("ShaderNodeOutputMaterial")
    emission = nodes.new("ShaderNodeEmission")
    emission.inputs["Color"].default_value = color
    emission.inputs["Strength"].default_value = 1.0
    material.node_tree.links.new(emission.outputs["Emission"], output.inputs["Surface"])
    return material


def _hand_vertex_indices(
    obj: Any,
    bone_map: Mapping[str, str],
    side: str,
) -> set[int]:
    roles = {f"hand_{side}"} | {
        role
        for digit in (1, 2, 3)
        for role in aroll_actions.chain_roles(side, digit).values()
    }
    bone_names = {bone_map[role] for role in roles if role in bone_map}
    group_indices = {
        group.index for group in obj.vertex_groups if group.name in bone_names
    }
    if not group_indices:
        return set()
    return {
        vertex.index
        for vertex in obj.data.vertices
        if any(
            membership.group in group_indices and float(membership.weight) > 1e-6
            for membership in vertex.groups
        )
    }


def _hand_face_indices(
    obj: Any,
    bone_map: Mapping[str, str],
    side: str,
) -> set[int]:
    weighted_vertices = _hand_vertex_indices(obj, bone_map, side)
    return {
        polygon.index
        for polygon in obj.data.polygons
        if sum(index in weighted_vertices for index in polygon.vertices)
        >= max(1, (len(polygon.vertices) + 1) // 2)
    }


def _hand_face_vertex_indices(
    obj: Any,
    bone_map: Mapping[str, str],
    side: str,
) -> set[int]:
    face_indices = _hand_face_indices(obj, bone_map, side)
    return {
        vertex_index
        for polygon in obj.data.polygons
        if polygon.index in face_indices
        for vertex_index in polygon.vertices
    }


def _read_color_mask(path: Path) -> tuple[int, int, list[float]]:
    image = bpy.data.images.load(str(path), check_existing=False)
    try:
        width, height = (int(value) for value in image.size)
        return width, height, list(image.pixels[:])
    finally:
        bpy.data.images.remove(image)


def _pixel_frame_metrics(
    width: int,
    height: int,
    indices: Iterable[int],
) -> dict[str, Any]:
    active = list(indices)
    if not active:
        return {
            "pixelCount": 0,
            "bounds": None,
            "marginPixels": -1,
            "marginFraction": -1.0,
            "borderTouching": True,
        }
    xs = [index % width for index in active]
    ys = [index // width for index in active]
    bounds = [min(xs), min(ys), max(xs), max(ys)]
    margin_pixels = min(
        bounds[0],
        bounds[1],
        width - 1 - bounds[2],
        height - 1 - bounds[3],
    )
    return {
        "pixelCount": len(active),
        "bounds": bounds,
        "marginPixels": margin_pixels,
        "marginFraction": round(
            float(margin_pixels) / max(1, min(width, height) - 1), 6
        ),
        "borderTouching": margin_pixels == 0,
    }


def _render_pixel_mask(
    scene: Any,
    path: Path,
    character_meshes: Iterable[Any],
    bone_map: Mapping[str, str],
    hand_side: str,
) -> dict[str, Any]:
    path.parent.mkdir(parents=True, exist_ok=True)
    materials = {
        "occluder": _mask_material("QA_Mask_Occluder", (0.0, 0.0, 0.0, 1.0)),
        **{
            role: _mask_material(f"QA_Mask_{role}", color)
            for role, color in MASK_COLORS.items()
        },
    }
    snapshots: list[tuple[Any, Any, Any]] = []
    previous_filepath = scene.render.filepath
    view_settings = scene.view_settings
    previous_view = (
        view_settings.view_transform,
        view_settings.look,
        float(view_settings.exposure),
        float(view_settings.gamma),
    )
    try:
        view_settings.view_transform = "Standard"
        view_settings.look = "None"
        view_settings.exposure = 0.0
        view_settings.gamma = 1.0
        for obj in sorted(character_meshes, key=lambda item: item.name):
            original_mesh = obj.data
            mask_mesh = original_mesh.copy()
            snapshots.append((obj, original_mesh, mask_mesh))
            obj.data = mask_mesh
            mask_mesh.materials.clear()
            role = str(obj.get("ip_face_topology_role") or "")
            if role in master_asset.REQUIRED_ORAL_ROLES:
                mask_mesh.materials.append(materials[role])
                for polygon in mask_mesh.polygons:
                    polygon.material_index = 0
                continue

            mask_mesh.materials.append(materials["occluder"])
            target_faces = (
                _hand_face_indices(obj, bone_map, hand_side)
                if hand_side
                else set()
            )
            if not target_faces:
                for polygon in mask_mesh.polygons:
                    polygon.material_index = 0
                continue
            mask_mesh.materials.append(materials["hand"])
            for polygon in mask_mesh.polygons:
                polygon.material_index = int(polygon.index in target_faces)

        scene.render.filepath = str(path)
        bpy.context.view_layer.update()
        bpy.ops.render.render(write_still=True)
    finally:
        scene.render.filepath = previous_filepath
        (
            view_settings.view_transform,
            view_settings.look,
            view_settings.exposure,
            view_settings.gamma,
        ) = previous_view
        for obj, original_mesh, mask_mesh in reversed(snapshots):
            obj.data = original_mesh
            bpy.data.meshes.remove(mask_mesh)
        for material in materials.values():
            bpy.data.materials.remove(material)
        bpy.context.view_layer.update()

    width, height, pixels = _read_color_mask(path)
    role_by_bits = {
        tuple(channel > 0.5 for channel in color[:3]): role
        for role, color in MASK_COLORS.items()
    }
    indices: dict[str, list[int]] = {role: [] for role in MASK_COLORS}
    for pixel_index, offset in enumerate(range(0, len(pixels), 4)):
        red, green, blue, alpha = pixels[offset : offset + 4]
        if alpha < 0.05:
            continue
        bits = (red >= 0.08, green >= 0.08, blue >= 0.08)
        role = role_by_bits.get(bits)
        if role is not None:
            indices[role].append(pixel_index)
    return {
        "path": str(path),
        "width": width,
        "height": height,
        "rolePixelCounts": {
            role: len(role_indices) for role, role_indices in indices.items()
        },
        "handPixelFrame": _pixel_frame_metrics(width, height, indices["hand"]),
    }


def _oral_render_evidence(
    scene: Any,
    camera: Any,
    rendered_pixel_counts: Mapping[str, int],
    mask_path: str,
) -> dict[str, Any]:
    role_objects = {
        role: [
            obj
            for obj in scene.objects
            if obj.type == "MESH" and str(obj.get("ip_face_topology_role") or "") == role
        ]
        for role in master_asset.REQUIRED_ORAL_ROLES
    }
    object_visibility: dict[str, Any] = {}
    component_counts: dict[str, int] = {}
    projected_counts: dict[str, int] = {}
    for role, objects in role_objects.items():
        count = sum(master_asset._mesh_component_count(obj) for obj in objects)
        projected = sum(_projected_vertex_count(scene, camera, obj) for obj in objects)
        component_counts[role] = count
        projected_counts[role] = projected
        object_visibility[role] = {
            "objects": [obj.name for obj in objects],
            "renderEnabled": bool(objects) and all(not obj.hide_render for obj in objects),
            "projectedVertexSamples": projected,
            "visiblePixelCount": int(rendered_pixel_counts.get(role, 0)),
            "visibleInCamera": int(rendered_pixel_counts.get(role, 0)) > 0,
        }
    upper_teeth_pixels = int(rendered_pixel_counts.get("upper_teeth", 0))
    lower_teeth_pixels = int(rendered_pixel_counts.get("lower_teeth", 0))
    tongue_pixels = int(rendered_pixel_counts.get("tongue", 0))
    return {
        "objectVisibility": object_visibility,
        "oralComponentCounts": component_counts,
        "dentalExposure": {
            "upperProjectedVertexSamples": projected_counts["upper_teeth"],
            "lowerProjectedVertexSamples": projected_counts["lower_teeth"],
            "upperVisiblePixelCount": upper_teeth_pixels,
            "lowerVisiblePixelCount": lower_teeth_pixels,
            "visiblePixelCount": upper_teeth_pixels + lower_teeth_pixels,
            "visible": upper_teeth_pixels + lower_teeth_pixels > 0,
            "occlusionAware": True,
            "maskPath": mask_path,
        },
        "tongueExposure": {
            "projectedVertexSamples": projected_counts["tongue"],
            "visiblePixelCount": tongue_pixels,
            "visible": tongue_pixels > 0,
            "occlusionAware": True,
            "maskPath": mask_path,
        },
    }


def _extrema_frame_intersections(
    armature: Any, bone_map: Mapping[str, str]
) -> list[dict[str, Any]]:
    head = armature.pose.bones[bone_map["head"]]
    head_center = armature.matrix_world @ ((head.head + head.tail) * 0.5)
    head_radius = max((armature.matrix_world @ head.tail - armature.matrix_world @ head.head).length, 0.08)
    findings: list[dict[str, Any]] = []
    for side in ("l", "r"):
        points = _pose_points(armature, bone_map, side)
        minimum_distance = min((point - head_center).length for point in points)
        if minimum_distance < head_radius * 0.52:
            findings.append(
                {
                    "kind": "hand_face_extrema_intersection",
                    "side": side,
                    "minimumDistance": round(float(minimum_distance), 6),
                    "threshold": round(float(head_radius * 0.52), 6),
                }
            )
    return findings


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
    hand_side = ""
    if sample.kind in {"hand", "digit"}:
        hand_side = sample.side or (
            "l" if sample.camera == HAND_CLOSE_CAMERA_LEFT else "r"
        )
    mask_relative_path = Path("masks") / sample.path
    mask = _render_pixel_mask(
        scene,
        output_dir / mask_relative_path,
        contract["characterMeshes"],
        bone_map,
        hand_side,
    )
    evidence = _oral_render_evidence(
        scene,
        camera,
        mask["rolePixelCounts"],
        mask_relative_path.as_posix(),
    )
    result = {
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
        "pixelMaskPath": mask_relative_path.as_posix(),
        "pixelMaskSha256": hashlib.sha256(
            (output_dir / mask_relative_path).read_bytes()
        ).hexdigest(),
        **evidence,
        "extremaFrameIntersections": _extrema_frame_intersections(armature, bone_map),
    }
    if sample.kind in {"hand", "digit"}:
        result["handPixelFrame"] = mask["handPixelFrame"]
    return result


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


def role_mask_difference(first: Path, second: Path, role: str) -> float:
    if role not in MASK_COLORS:
        raise RuntimeError(f"unknown rendered mask role: {role}")
    first_width, first_height, first_pixels = _read_color_mask(first)
    second_width, second_height, second_pixels = _read_color_mask(second)
    if (first_width, first_height) != (second_width, second_height):
        raise RuntimeError(
            "role-mask comparison requires equal dimensions: "
            f"{first.name}={first_width}x{first_height}, "
            f"{second.name}={second_width}x{second_height}"
        )
    target_bits = tuple(channel > 0.5 for channel in MASK_COLORS[role][:3])

    def selected(pixels: list[float]) -> tuple[bool, ...]:
        return tuple(
            alpha >= 0.05
            and (red >= 0.08, green >= 0.08, blue >= 0.08) == target_bits
            for red, green, blue, alpha in (
                pixels[offset : offset + 4] for offset in range(0, len(pixels), 4)
            )
        )

    first_mask = selected(first_pixels)
    second_mask = selected(second_pixels)
    union_count = sum(left or right for left, right in zip(first_mask, second_mask))
    if union_count == 0:
        raise RuntimeError(f"rendered {role} masks are both empty")
    return round(
        sum(left != right for left, right in zip(first_mask, second_mask)) / union_count,
        6,
    )


def frame_md5(path: Path, ffmpeg: str | None = None) -> str:
    executable = ffmpeg or find_ffmpeg()
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


BITMAP_FONT = {
    "A": ("01110", "10001", "10001", "11111", "10001", "10001", "10001"),
    "B": ("11110", "10001", "10001", "11110", "10001", "10001", "11110"),
    "C": ("01111", "10000", "10000", "10000", "10000", "10000", "01111"),
    "D": ("11110", "10001", "10001", "10001", "10001", "10001", "11110"),
    "E": ("11111", "10000", "10000", "11110", "10000", "10000", "11111"),
    "F": ("11111", "10000", "10000", "11110", "10000", "10000", "10000"),
    "G": ("01111", "10000", "10000", "10111", "10001", "10001", "01111"),
    "H": ("10001", "10001", "10001", "11111", "10001", "10001", "10001"),
    "I": ("11111", "00100", "00100", "00100", "00100", "00100", "11111"),
    "J": ("00111", "00010", "00010", "00010", "10010", "10010", "01100"),
    "K": ("10001", "10010", "10100", "11000", "10100", "10010", "10001"),
    "L": ("10000", "10000", "10000", "10000", "10000", "10000", "11111"),
    "M": ("10001", "11011", "10101", "10101", "10001", "10001", "10001"),
    "N": ("10001", "11001", "10101", "10011", "10001", "10001", "10001"),
    "O": ("01110", "10001", "10001", "10001", "10001", "10001", "01110"),
    "P": ("11110", "10001", "10001", "11110", "10000", "10000", "10000"),
    "Q": ("01110", "10001", "10001", "10001", "10101", "10010", "01101"),
    "R": ("11110", "10001", "10001", "11110", "10100", "10010", "10001"),
    "S": ("01111", "10000", "10000", "01110", "00001", "00001", "11110"),
    "T": ("11111", "00100", "00100", "00100", "00100", "00100", "00100"),
    "U": ("10001", "10001", "10001", "10001", "10001", "10001", "01110"),
    "V": ("10001", "10001", "10001", "10001", "10001", "01010", "00100"),
    "W": ("10001", "10001", "10001", "10101", "10101", "10101", "01010"),
    "X": ("10001", "10001", "01010", "00100", "01010", "10001", "10001"),
    "Y": ("10001", "10001", "01010", "00100", "00100", "00100", "00100"),
    "Z": ("11111", "00001", "00010", "00100", "01000", "10000", "11111"),
    "0": ("01110", "10001", "10011", "10101", "11001", "10001", "01110"),
    "1": ("00100", "01100", "00100", "00100", "00100", "00100", "01110"),
    "2": ("01110", "10001", "00001", "00010", "00100", "01000", "11111"),
    "3": ("11110", "00001", "00001", "01110", "00001", "00001", "11110"),
    "4": ("00010", "00110", "01010", "10010", "11111", "00010", "00010"),
    "5": ("11111", "10000", "10000", "11110", "00001", "00001", "11110"),
    "6": ("01110", "10000", "10000", "11110", "10001", "10001", "01110"),
    "7": ("11111", "00001", "00010", "00100", "01000", "01000", "01000"),
    "8": ("01110", "10001", "10001", "01110", "10001", "10001", "01110"),
    "9": ("01110", "10001", "10001", "01111", "00001", "00001", "01110"),
    "-": ("00000", "00000", "00000", "11111", "00000", "00000", "00000"),
    " ": ("00000",) * 7,
}


def _write_bitmap_label(path: Path, label: str, width: int, height: int) -> None:
    _require_blender()
    text = label.upper()
    glyph_width = max(1, len(text) * 6 - 1)
    scale = max(1, min(4, (width - 16) // glyph_width, (height - 8) // 7))
    text_width = glyph_width * scale
    text_height = 7 * scale
    origin_x = max(0, (width - text_width) // 2)
    origin_y = max(0, (height - text_height) // 2)
    background = (0.035, 0.045, 0.055, 1.0)
    foreground = (0.92, 0.95, 0.96, 1.0)
    accent = (0.10, 0.58, 0.48, 1.0)
    pixels = list(background) * (width * height)
    for y in range(min(2, height)):
        for x in range(width):
            offset = (y * width + x) * 4
            pixels[offset : offset + 4] = accent
    for character_index, character in enumerate(text):
        glyph = BITMAP_FONT.get(character, BITMAP_FONT[" "])
        for row, bits in enumerate(glyph):
            for column, enabled in enumerate(bits):
                if enabled != "1":
                    continue
                for pixel_y in range(scale):
                    for pixel_x in range(scale):
                        x = origin_x + (character_index * 6 + column) * scale + pixel_x
                        y = origin_y + (6 - row) * scale + pixel_y
                        if 0 <= x < width and 0 <= y < height:
                            offset = (y * width + x) * 4
                            pixels[offset : offset + 4] = foreground
    image = bpy.data.images.new(
        f"QA_Label_{path.stem}", width=width, height=height, alpha=True
    )
    try:
        image.pixels.foreach_set(pixels)
        image.filepath_raw = str(path)
        image.file_format = "PNG"
        image.save()
    finally:
        bpy.data.images.remove(image)


def create_contact_sheet(
    paths: Iterable[Path | str],
    output_path: Path | str,
    *,
    columns: int = 4,
    cell_size: int = 320,
    labels: Iterable[str] | None = None,
) -> dict[str, Any]:
    """Compose fixed QA crops into one deterministic PNG with ffmpeg."""
    sources = [Path(path).expanduser().resolve() for path in paths]
    missing = [str(path) for path in sources if not path.is_file()]
    if missing:
        raise RuntimeError("contact-sheet inputs are missing: " + ", ".join(missing))
    if not sources:
        raise RuntimeError("contact sheet requires at least one input")
    executable = find_ffmpeg()
    if not executable:
        raise RuntimeError("contact-sheet generation requires ffmpeg on PATH")
    output = Path(output_path).expanduser().resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    columns = max(1, int(columns))
    cell_size = max(64, int(cell_size))
    resolved_labels = list(labels) if labels is not None else []
    if resolved_labels and len(resolved_labels) != len(sources):
        raise RuntimeError(
            f"contact sheet has {len(sources)} inputs but {len(resolved_labels)} labels"
        )
    label_band = max(24, min(56, int(round(cell_size * 0.125)))) if resolved_labels else 0
    content_height = cell_size - label_band
    with tempfile.TemporaryDirectory(prefix="qa-contact-labels-", dir=output.parent) as tmp:
        label_paths: list[Path] = []
        for index, label in enumerate(resolved_labels):
            label_path = Path(tmp) / f"label-{index:02d}.png"
            _write_bitmap_label(label_path, label, cell_size, label_band)
            label_paths.append(label_path)

        filters = []
        layout = []
        for index in range(len(sources)):
            filters.append(
                f"[{index}:v]scale={cell_size}:{content_height}:force_original_aspect_ratio=decrease,"
                f"pad={cell_size}:{content_height}:(ow-iw)/2:(oh-ih)/2:color=0x20242a[image{index}]"
            )
            if resolved_labels:
                label_input = len(sources) + index
                filters.append(
                    f"[image{index}][{label_input}:v]vstack=inputs=2[v{index}]"
                )
            else:
                filters.append(f"[image{index}]null[v{index}]")
            layout.append(
                f"{(index % columns) * cell_size}_{(index // columns) * cell_size}"
            )
        filters.append(
            "".join(f"[v{index}]" for index in range(len(sources)))
            + f"xstack=inputs={len(sources)}:layout={'|'.join(layout)}:fill=0x14171c[out]"
        )
        command = [executable, "-v", "error", "-y"]
        for source in (*sources, *label_paths):
            command.extend(("-i", str(source)))
        command.extend(
            (
                "-filter_complex",
                ";".join(filters),
                "-map",
                "[out]",
                "-frames:v",
                "1",
                str(output),
            )
        )
        completed = subprocess.run(command, check=False, capture_output=True, text=True)
    if completed.returncode != 0 or not output.is_file() or output.stat().st_size <= 0:
        detail = completed.stderr.strip() or "empty contact-sheet output"
        raise RuntimeError(f"contact-sheet generation failed: {detail}")
    return {
        "path": str(output),
        "inputCount": len(sources),
        "columns": columns,
        "cellSize": cell_size,
        "inputs": [str(path) for path in sources],
        "labels": resolved_labels,
        "labelBandPixels": label_band,
    }


def create_qa_contact_sheets(
    refined_dir: Path | str,
    baseline_dir: Path | str,
    qa_output_path: Path | str,
    comparison_output_path: Path | str,
) -> dict[str, Any]:
    """Create the approved oral/hand QA sheet and fixed before/after comparison."""
    refined = Path(refined_dir).expanduser().resolve()
    baseline = Path(baseline_dir).expanduser().resolve()
    face_crops = [
        "face/Rest.png",
        "face/MBP.png",
        "face/A.png",
        "face/E.png",
        "face/O.png",
        "face/U.png",
        "face/Smile.png",
        "face/Surprise.png",
    ]
    hand_crops = [sample.path for sample in HAND_SAMPLES]
    comparison_crops = [
        "face/Rest.png",
        "face/A.png",
        "face/Smile.png",
        "hand/open.png",
        "hand/fist.png",
        "hand/camera_facing_wave.png",
    ]
    comparison_paths = [
        directory / relative_path
        for relative_path in comparison_crops
        for directory in (baseline, refined)
    ]
    comparison_labels = [
        f"{version} {label}"
        for label in ("Rest", "A", "Smile", "open", "fist", "camera-facing wave")
        for version in ("Legacy", "Refined")
    ]
    return {
        "qa": create_contact_sheet(
            [refined / path for path in (*face_crops, *hand_crops)],
            qa_output_path,
            labels=CONTACT_SHEET_LABELS,
        ),
        "comparison": create_contact_sheet(
            comparison_paths,
            comparison_output_path,
            labels=comparison_labels,
        ),
    }


def create_publication_contact_sheets(
    qa_dir: Path | str,
    qa_output_path: Path | str,
    comparison_output_path: Path | str,
) -> dict[str, Any]:
    """Create review sheets that compare meaningful states of one staged master."""
    source = Path(qa_dir).expanduser().resolve()
    qa_paths = [
        "face/Rest.png",
        "face/MBP.png",
        "face/A.png",
        "face/E.png",
        "face/O.png",
        "face/U.png",
        "face/Smile.png",
        "face/Surprise.png",
        *(sample.path for sample in HAND_SAMPLES),
    ]
    comparison_paths = [
        "face/Rest.png",
        "face/A.png",
        "face/MBP.png",
        "face/O.png",
        "hand/open.png",
        "hand/fist.png",
        "hand/finger_roll_r_1.png",
        "hand/finger_roll_r_2.png",
        "hand/finger_roll_r_3.png",
        "hand/camera_facing_wave.png",
        "hand/count_3.png",
        "hand/point.png",
    ]
    return {
        "qa": create_contact_sheet(
            [source / path for path in qa_paths],
            qa_output_path,
            labels=CONTACT_SHEET_LABELS,
        ),
        "comparison": create_contact_sheet(
            [source / path for path in comparison_paths],
            comparison_output_path,
            labels=(
                "Rest",
                "A open",
                "MBP closed",
                "O round",
                "Open hand",
                "Fist",
                "Finger roll 1",
                "Finger roll 2",
                "Finger roll 3",
                "Camera-facing wave",
                "Count three",
                "Point",
            ),
        ),
    }


def _comparison_metrics(output_dir: Path) -> dict[str, Any]:
    open_fist = role_mask_difference(
        output_dir / "masks/hand/open.png",
        output_dir / "masks/hand/fist.png",
        "hand",
    )
    if open_fist < MIN_OPEN_FIST_PIXEL_DIFFERENCE:
        raise RuntimeError(
            f"open/fist rendered hand-mask difference {open_fist:.6f} is below "
            f"{MIN_OPEN_FIST_PIXEL_DIFFERENCE:.6f}"
        )
    finger_roll: dict[str, float] = {}
    for side in ("r", "l"):
        for first, second in ((1, 2), (2, 3)):
            key = f"{side}{first}-{side}{second}"
            difference = role_mask_difference(
                output_dir / f"masks/hand/finger_roll_{side}_{first}.png",
                output_dir / f"masks/hand/finger_roll_{side}_{second}.png",
                "hand",
            )
            finger_roll[key] = difference
            if difference < MIN_FINGER_ROLL_PIXEL_DIFFERENCE:
                raise RuntimeError(
                    f"finger-roll phase {key} rendered hand-mask difference "
                    f"{difference:.6f} is below "
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


def _validate_rendered_pixel_gates(samples: Iterable[Mapping[str, Any]]) -> None:
    samples = list(samples)
    for sample in samples:
        if sample["kind"] not in {"hand", "digit"}:
            continue
        frame = sample.get("handPixelFrame") or {}
        if int(frame.get("pixelCount", 0)) <= 0:
            raise RuntimeError(f"hand pixel mask is empty for {sample['label']}")
        if bool(frame.get("borderTouching")):
            raise RuntimeError(f"hand pixel mask touches the frame border for {sample['label']}")
        if float(frame.get("marginFraction", -1.0)) < MIN_HAND_PIXEL_MARGIN:
            raise RuntimeError(
                f"hand pixel margin {float(frame.get('marginFraction', -1.0)):.6f} "
                f"is below {MIN_HAND_PIXEL_MARGIN:.6f} for {sample['label']}"
            )

    faces = {sample["label"]: sample for sample in samples if sample["kind"] == "face"}
    for label in ("Rest", "MBP"):
        dental = int(faces[label]["dentalExposure"]["visiblePixelCount"])
        tongue = int(faces[label]["tongueExposure"]["visiblePixelCount"])
        if dental or tongue:
            raise RuntimeError(
                f"closed oral sample {label} exposes {dental} dental and {tongue} tongue pixels"
            )
    for label in ("A", "E", "O", "U", "Surprise"):
        dental = int(faces[label]["dentalExposure"]["visiblePixelCount"])
        tongue = int(faces[label]["tongueExposure"]["visiblePixelCount"])
        if dental <= 0 or tongue <= 0:
            raise RuntimeError(
                f"open oral sample {label} requires visible rendered dental and tongue pixels"
            )


def run_fixed_comparison_crops(
    output_dir: Path | str,
    *,
    resolution: int = 640,
) -> dict[str, Any]:
    """Render only legacy-compatible fixed crops for before/after evidence."""
    _require_blender()
    output_dir = Path(output_dir).expanduser().resolve()
    output_dir.mkdir(parents=True, exist_ok=True)
    samples = tuple(sample for sample in QA_SAMPLES if sample.label in COMPARISON_CROP_LABELS)
    _contract, lighting, rendered = _render_selected_samples(
        output_dir, samples, resolution=resolution
    )
    require_files(output_dir, (sample.path for sample in samples))
    report = {
        "status": "ready",
        "mode": "legacy_compatible_comparison_crops",
        "lighting": lighting,
        "contract": {
            "labels": [sample.label for sample in samples],
            "sampleCount": len(samples),
            "requiredFiles": [sample.path for sample in samples],
        },
        "samples": rendered,
    }
    report_path = output_dir / "comparison-crops-report.json"
    report_path.write_text(
        json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    print(f"AROLL_COMPARISON_CROPS_REPORT={report_path}")
    return report


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
    _validate_rendered_pixel_gates(samples)
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
        sample["sha256"] = hashlib.sha256(
            (output_dir / sample["path"]).read_bytes()
        ).hexdigest()

    comparisons = _comparison_metrics(output_dir)
    duplicates = _duplicate_metrics(output_dir)
    qa_sheet = output_dir / "MainIP_Sloth_Oral_Hand_QA.png"
    comparison_sheet = output_dir / "MainIP_Sloth_Oral_Hand_Comparison.png"
    create_publication_contact_sheets(
        output_dir,
        qa_sheet,
        comparison_sheet,
    )
    current_blend = Path(str(bpy.data.filepath or "")).expanduser()
    staged_sha256 = ""
    capability_report: dict[str, Any] = {}
    if current_blend.is_file():
        current_blend = current_blend.resolve()
        staged_sha256 = master_asset._sha256_file(current_blend)
        collection = bpy.data.collections.get(master_asset.MASTER_COLLECTION)
        if collection is not None:
            validated = master_asset.validate_master_collection(
                collection, current_blend
            )
            capabilities = validated.get("capabilities")
            if isinstance(capabilities, Mapping):
                capability_report = dict(capabilities)
    capability_sha256 = (
        master_asset._canonical_json_sha256(capability_report)
        if capability_report
        else ""
    )
    manifest = qa_manifest_payload()
    manifest_sha256 = qa_manifest_sha256()
    report = {
        "schemaVersion": QA_REPORT_SCHEMA_VERSION,
        "status": "ready",
        "stagedSha256": staged_sha256,
        "capabilityReport": capability_report,
        "capabilityReportSha256": capability_sha256,
        "manifest": manifest,
        "manifestSha256": manifest_sha256,
        "faceCapability": {
            "blinkCapability": FACE_CAPABILITY,
            "qaSample": "face/Squint.png",
            "fullBlinkClaimed": False,
        },
        "lighting": lighting,
        "contract": {
            "actions": list(AROLL_ACTIONS),
            "cameras": list(ACTION_CAMERAS),
            "handCloseCameras": [HAND_CLOSE_CAMERA_LEFT, HAND_CLOSE_CAMERA_RIGHT],
            "sampleCount": len(QA_SAMPLES),
            "requiredFiles": list(required_relative_paths()),
            "manifestSha256": manifest_sha256,
        },
        "samples": samples,
        "comparisons": comparisons,
        "framemd5": duplicates,
        "sheets": {
            "qa": {
                "path": str(qa_sheet),
                "sha256": hashlib.sha256(qa_sheet.read_bytes()).hexdigest(),
            },
            "comparison": {
                "path": str(comparison_sheet),
                "sha256": hashlib.sha256(
                    comparison_sheet.read_bytes()
                ).hexdigest(),
            },
        },
    }
    report_path = output_dir / REPORT_NAME
    report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"AROLL_MASTER_QA_REPORT={report_path}")
    return report


def build_staged_refined_master(input_path: Path | str) -> dict[str, Any]:
    """Run the existing builder while binding final refinement to pre-refinement hashes."""
    _require_blender()
    input_path = Path(input_path).expanduser().resolve()
    try:
        data = json.loads(input_path.read_text(encoding="utf-8"))
    except (OSError, ValueError, json.JSONDecodeError) as exc:
        raise RuntimeError(f"cannot read staged refined build input: {input_path}") from exc
    if data.get("prepareMaster") is not True or data.get("useMasterAsset") is not False:
        raise RuntimeError("staged refined build requires prepareMaster=true and useMasterAsset=false")
    source_path = Path(str(data.get("modelPath") or "")).expanduser().resolve()
    if not source_path.is_file():
        raise RuntimeError(f"staged refined build source model is missing: {source_path}")
    staged_path = Path(str(data.get("riggedBlendPath") or "")).expanduser().resolve()
    if (
        staged_path.name != "main-ip-aroll-master-refined.blend"
        or staged_path.parent.name != "staging"
    ):
        raise RuntimeError(
            "staged refined build target must be staging/main-ip-aroll-master-refined.blend"
        )

    captured_hashes: dict[str, str] = {}
    original_prepare = blender_renderer.prepare_character
    original_save = blender_renderer.save_master_collection
    original_argv = list(sys.argv)

    def prepare_and_capture(character_objects: Iterable[Any], *args: Any, **kwargs: Any) -> Any:
        dimensions = original_prepare(character_objects, *args, **kwargs)
        captured_hashes.update(master_asset.source_surface_hashes(character_objects))
        return dimensions

    def save_bound_master(**kwargs: Any) -> dict[str, Any]:
        if not captured_hashes:
            raise RuntimeError("source UV/material hashes were not captured before refinement")
        kwargs["refined_intent"] = True
        kwargs["expected_source_surface_hashes"] = dict(captured_hashes)
        return master_asset.save_master_collection(**kwargs)

    try:
        blender_renderer.prepare_character = prepare_and_capture
        blender_renderer.save_master_collection = save_bound_master
        separator = original_argv.index("--") if "--" in original_argv else len(original_argv)
        sys.argv = [*original_argv[:separator], "--", str(input_path)]
        blender_renderer.main()
    finally:
        sys.argv = original_argv
        blender_renderer.prepare_character = original_prepare
        blender_renderer.save_master_collection = original_save

    if not staged_path.is_file():
        raise RuntimeError(f"staged refined builder did not create {staged_path}")
    validated = master_asset.validate_master_collection(
        bpy.data.collections.get(master_asset.MASTER_COLLECTION), staged_path
    )
    capabilities = validated.get("capabilities")
    if not isinstance(capabilities, Mapping):
        raise RuntimeError("staged refined builder produced no live capability report")
    result = {
        "status": "passed",
        "sourcePath": str(source_path),
        "stagedPath": str(staged_path),
        "sourceSurfaceHashes": dict(captured_hashes),
        "capabilityReport": dict(capabilities),
    }
    print("AROLL_STAGED_BUILD=" + json.dumps(result, sort_keys=True))
    return result


def publish_staged_refined_master(
    staged_path: Path | str,
    final_path: Path | str,
    publication_report_path: Path | str,
) -> dict[str, Any]:
    """Publish one staged refined master from SHA-bound QA evidence."""
    report_path = Path(publication_report_path).expanduser().resolve()
    try:
        report = json.loads(report_path.read_text(encoding="utf-8"))
    except (OSError, ValueError, json.JSONDecodeError) as exc:
        raise RuntimeError(
            f"cannot read refined publication report: {report_path}"
        ) from exc
    if not isinstance(report, Mapping):
        raise RuntimeError("refined publication report must contain an object")
    result = master_asset.publish_refined_master(
        staged_path,
        final_path,
        report,
    )
    print("AROLL_REFINED_PUBLICATION=" + json.dumps(result, sort_keys=True))
    return result


def record_staged_visual_inspection(
    staged_path: Path | str,
    qa_report_path: Path | str,
    output_path: Path | str,
    reviewer: str,
    reviewed_at: str,
    notes: str = "",
) -> dict[str, Any]:
    result = master_asset.record_visual_inspection(
        staged_path,
        qa_report_path,
        reviewer,
        reviewed_at,
        output_path,
        notes,
    )
    print("AROLL_VISUAL_INSPECTION=" + json.dumps(result, sort_keys=True))
    return result


def build_staged_publication_package(
    staged_path: Path | str,
    qa_report_path: Path | str,
    inspection_report_path: Path | str,
    output_dir: Path | str,
) -> dict[str, Any]:
    result = master_asset.create_publication_report(
        staged_path,
        qa_report_path,
        inspection_report_path,
        output_dir,
    )
    print("AROLL_PUBLICATION_PACKAGE=" + json.dumps(result, sort_keys=True))
    return result


def main() -> None:
    args = sys.argv[sys.argv.index("--") + 1 :] if "--" in sys.argv else []
    if not args:
        raise RuntimeError(
            "usage: blender master.blend --python render_aroll_master_qa.py -- output_dir\n"
            "   or: blender --background --python render_aroll_master_qa.py -- "
            "--build-staged input.json\n"
            "   or: blender --background --python render_aroll_master_qa.py -- "
            "--publish-staged staged.blend final.blend publication-report.json\n"
            "   or: blender --background --python render_aroll_master_qa.py -- "
            "--record-inspection staged.blend qa-report.json output.json reviewer reviewed-at notes\n"
            "   or: blender --background --python render_aroll_master_qa.py -- "
            "--build-publication staged.blend qa-report.json inspection.json output-dir"
        )
    if args[0] == "--build-staged":
        if len(args) != 2:
            raise RuntimeError("--build-staged requires exactly one input JSON path")
        build_staged_refined_master(args[1])
        return
    if args[0] == "--publish-staged":
        if len(args) != 4:
            raise RuntimeError(
                "--publish-staged requires staged, final, and publication report paths"
            )
        publish_staged_refined_master(args[1], args[2], args[3])
        return
    if args[0] == "--record-inspection":
        if len(args) != 7:
            raise RuntimeError(
                "--record-inspection requires staged, QA report, output, reviewer, reviewed-at, and notes"
            )
        record_staged_visual_inspection(
            args[1], args[2], args[3], args[4], args[5], args[6]
        )
        return
    if args[0] == "--build-publication":
        if len(args) != 5:
            raise RuntimeError(
                "--build-publication requires staged, QA report, inspection, and output directory"
            )
        build_staged_publication_package(args[1], args[2], args[3], args[4])
        return
    run_qa(args[0])


if __name__ == "__main__":
    main()
