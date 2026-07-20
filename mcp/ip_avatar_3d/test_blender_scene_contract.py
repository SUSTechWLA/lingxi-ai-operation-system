#!/usr/bin/env python3
"""Blender-side contract tests for reusable talking-head studio scenes."""

from __future__ import annotations

import json
import math
import sys
import tempfile
import unittest
from pathlib import Path

try:
    import bpy
except ModuleNotFoundError as exc:
    raise unittest.SkipTest("requires Blender's bpy runtime") from exc

SCRIPT_DIR = Path(__file__).resolve().parent
REPO_ROOT = SCRIPT_DIR.parents[1]
DEFAULT_PROFILE_PATH = REPO_ROOT / "ip形象/main_ip/character-profile.json"
DEFAULT_PROFILE = json.loads(DEFAULT_PROFILE_PATH.read_text(encoding="utf-8"))
DEFAULT_STUDIO_PATH = DEFAULT_PROFILE_PATH.parent / DEFAULT_PROFILE["render"]["sceneBlendPath"]
DEFAULT_MASTER_PATH = DEFAULT_PROFILE_PATH.parent / DEFAULT_PROFILE["model"]["masterBlendPath"]
DEFAULT_MANIFEST_PATH = DEFAULT_PROFILE_PATH.parent / DEFAULT_PROFILE["defaultAssetManifest"]
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import blender_renderer
import editorial_studio_builder
import render_warm_studio_qa
import validate_warm_studio as warm_studio_validator
import validate_warm_studio_character as warm_character_validator
import warm_studio_contract as contract
from bpy_extras.object_utils import world_to_camera_view
from mathutils import Vector


def reset_scene() -> None:
    bpy.ops.object.select_all(action="SELECT")
    bpy.ops.object.delete(use_global=False)
    bpy.context.scene.timeline_markers.clear()


def add_camera(name: str) -> bpy.types.Object:
    data = bpy.data.cameras.new(name)
    obj = bpy.data.objects.new(name, data)
    bpy.context.collection.objects.link(obj)
    return obj


def assert_location_matches(
    actual: tuple[float, float, float],
    expected: tuple[float, float, float],
) -> None:
    assert all(abs(left - right) <= 1e-6 for left, right in zip(actual, expected))


def test_warm_studio_saved_scene_has_dual_mode_contract() -> None:
    scene = bpy.context.scene
    assert scene.render.resolution_x == 1920
    assert scene.render.resolution_y == 1080
    assert scene["ip_presentation_modes"] == '["standing", "seated"]'
    assert scene["ip_subject_light_profile"] == contract.SUBJECT_LIGHT_PROFILE["name"]
    assert (
        scene["ip_background_stops_below_face"]
        == contract.SUBJECT_LIGHT_PROFILE["backgroundStopsBelowFace"]
    )

    validation = warm_studio_validator.validate_scene()
    assert validation["errors"] == [], validation["errors"]

    for mode in contract.PRESENTATION_MODES:
        marker_specs = contract.MODE_MARKER_SPECS[mode]
        marker_names = {
            "spawn": f"IP_{mode.title()}_Spawn",
            "focus": f"IP_{mode.title()}_Focus_Head",
            "knee_l": f"IP_{mode.title()}_Knee_Target.L",
            "knee_r": f"IP_{mode.title()}_Knee_Target.R",
            "foot_l": f"IP_{mode.title()}_Foot_Target.L",
            "foot_r": f"IP_{mode.title()}_Foot_Target.R",
        }
        for role, name in marker_names.items():
            marker = bpy.data.objects.get(name)
            assert marker is not None, name
            assert_location_matches(tuple(marker.location), marker_specs[role])

        for role, (name, location, lens) in contract.MODE_CAMERA_SPECS[mode].items():
            camera = bpy.data.objects.get(name)
            assert camera is not None, name
            assert camera.type == "CAMERA"
            assert_location_matches(tuple(camera.location), location)
            assert camera.data.lens == lens
            expected_focus_name = (
                "IP_Transition_Focus"
                if role == "transition"
                else f"IP_{mode.title()}_Focus_Head"
            )
            assert camera["ip_focus_marker"] == expected_focus_name
            assert camera.data.dof.use_dof is True
            assert camera.data.dof.focus_object is bpy.data.objects[expected_focus_name]
            assert camera.data.dof.aperture_fstop == 5.0
            assert camera.data.dof.aperture_blades == 9

    for name in ("IP_Seat_Target", "IP_Foot_Target.L", "IP_Foot_Target.R"):
        assert bpy.data.objects.get(name) is not None, name

    for name in (
        "IP_Seated_Knee_Target.L",
        "IP_Seated_Knee_Target.R",
        "IP_Transition_Focus",
        "Camera_Standing_Transition",
        "Camera_Seated_Transition",
    ):
        assert bpy.data.objects.get(name) is not None, name
    assert bpy.data.objects["Chair_Main"]["hero_visibility_strategy"] == "partial_profile"
    desk_top = bpy.data.objects["Desk_Top"]
    chair_back = bpy.data.objects["Chair_Back"]
    chair_seat = bpy.data.objects["Chair_Seat"]
    assert_location_matches(
        tuple(bpy.data.objects["Chair_Main"].location),
        contract.HERO_CHAIR_LOCATION,
    )
    assert_location_matches(tuple(chair_seat.dimensions), contract.HERO_CHAIR_SEAT_SIZE)
    assert_location_matches(tuple(chair_back.dimensions), contract.HERO_CHAIR_BACK_SIZE)
    desk_top_z = max((desk_top.matrix_world @ Vector(corner)).z for corner in desk_top.bound_box)
    chair_back_z = max(
        (chair_back.matrix_world @ Vector(corner)).z for corner in chair_back.bound_box
    )
    assert chair_back_z > desk_top_z

    for role, name in (
        ("foot_l", "IP_Foot_Target.L"),
        ("foot_r", "IP_Foot_Target.R"),
    ):
        assert_location_matches(
            tuple(bpy.data.objects[name].location),
            contract.MODE_MARKER_SPECS["standing"][role],
        )

    assert_location_matches(
        tuple(bpy.data.objects["IP_Character_Spawn"].location),
        contract.MODE_MARKER_SPECS["standing"]["spawn"],
    )
    assert_location_matches(
        tuple(bpy.data.objects["IP_Focus_Head"].location),
        contract.MODE_MARKER_SPECS["standing"]["focus"],
    )

    profile = contract.SUBJECT_LIGHT_PROFILE
    world_background = next(
        node for node in scene.world.node_tree.nodes if node.type == "BACKGROUND"
    )
    assert abs(world_background.inputs["Strength"].default_value - profile["worldStrength"]) <= 1e-6

    subject_specs = {
        "IP_Subject_Key": ("key", 1100.0, 6500.0),
        "IP_Subject_Fill": ("fill", 110.0, 5200.0),
        "IP_Subject_FrontFill": ("front_fill", 180.0, 5600.0),
        "IP_Subject_Rim": ("rim", 180.0, 3200.0),
    }
    for name, (role, energy, temperature) in subject_specs.items():
        light = bpy.data.objects.get(name)
        assert light is not None, name
        assert light.type == "LIGHT"
        assert light.get("ip_light_role") == role
        assert light.data.energy == energy
        assert light.get("ip_base_energy") == energy
        assert light.data.use_temperature is True
        assert light.data.temperature == temperature
        assert light.get("ip_color_temperature") == int(temperature)

    assert scene.view_settings.view_transform == "AgX"
    assert "Medium High Contrast" in scene.view_settings.look
    key = bpy.data.objects["IP_Subject_Key"]
    assert key.data.type == "SPOT"
    assert abs(math.degrees(key.data.spot_size) - 55.0) <= 1e-5
    assert abs(key.data.spot_blend - 0.65) <= 1e-6
    assert abs(key.data.shadow_soft_size - 0.55) <= 1e-6
    assert key.get("ip_spot_size_degrees") == 55.0
    assert key.get("ip_spot_blend") == 0.65
    assert abs(scene.view_settings.exposure - (-2.45)) <= 1e-6
    assert abs(scene["ip_authored_exposure"] - (-2.45)) <= 1e-6
    assert scene["ip_cycles_final_exposure"] == -3.7
    rim = bpy.data.objects["IP_Subject_Rim"]
    assert rim.data.type == "SPOT"
    assert abs(math.degrees(rim.data.spot_size) - 28.0) <= 1e-5
    assert abs(rim.data.spot_blend - 0.65) <= 1e-6
    assert abs(rim.data.shadow_soft_size - 0.5) <= 1e-6
    front_fill = bpy.data.objects["IP_Subject_FrontFill"]
    assert front_fill.data.type == "AREA"
    assert front_fill.data.use_shadow is False
    assert tuple(front_fill.get("ip_target")) == (0.0, 0.365, 1.2)
    assert scene["ip_cycles_key_energy_multiplier"] == 3.5
    assert scene.view_settings.use_white_balance is True
    assert scene.view_settings.white_balance_temperature == 4500.0
    assert scene.view_settings.white_balance_tint == 10.0


def test_transition_stage_preserves_foreground_desk_for_authored_composition() -> None:
    desk_objects = [
        obj for obj in bpy.context.scene.objects if obj.name.startswith("Desk_")
    ]
    original = {obj.name: obj.hide_render for obj in desk_objects}
    try:
        for obj in desk_objects:
            obj.hide_render = False
        report = blender_renderer.configure_transition_stage_visibility(
            {
                "motionEvents": [
                    {
                        "motion": "avatar_action",
                        "action": "Aroll_Transition_StandToSit",
                        "startState": "standing",
                        "endState": "seated",
                    }
                ]
            }
        )
        assert report["fullBodyStage"] is True, report
        assert report["hiddenDeskObjects"] == []
        assert report["visibleDeskObjects"] == sorted(obj.name for obj in desk_objects)
        assert desk_objects and all(not obj.hide_render for obj in desk_objects)
    finally:
        for name, hidden in original.items():
            bpy.data.objects[name].hide_render = hidden


def test_transition_collision_calibration_allows_authored_chair_contact() -> None:
    all_obstacles = blender_renderer._mode_collision_obstacles()
    desk_obstacles = blender_renderer._mode_collision_obstacles(
        include_chair=False
    )
    assert any(obj.name.startswith("Chair_") for obj in all_obstacles)
    assert desk_obstacles
    assert all(not obj.name.startswith("Chair_") for obj in desk_obstacles)


def test_production_validator_rejects_mode_camera_broken_dof_focus_object() -> None:
    camera = bpy.data.objects["Camera_Standing_Wide"]
    original_focus = camera.data.dof.focus_object
    camera.data.dof.focus_object = bpy.data.objects["IP_Transition_Focus"]
    try:
        validation = warm_studio_validator.validate_scene()
        assert any(
            "Camera_Standing_Wide DOF focus object must be "
            "'IP_Standing_Focus_Head'" in error
            for error in validation["errors"]
        ), validation["errors"]
    finally:
        camera.data.dof.focus_object = original_focus


def test_luminance_masks_follow_rendered_subject_and_projected_head() -> None:
    width = 6
    height = 4
    matte_values = [
        0, 0, 0, 0, 0, 0,
        0, 7, 7, 7, 7, 0,
        0, 7, 7, 7, 7, 0,
        0, 0, 0, 0, 0, 0,
    ]
    matte_rgba = [
        channel
        for value in matte_values
        for channel in (float(bool(value)),) * 3 + (1.0,)
    ]
    subject = render_warm_studio_qa.subject_mask_from_rendered_id_matte(
        matte_rgba,
    )
    left_face = render_warm_studio_qa.face_mask_from_projected_head_triangles(
        subject,
        width=width,
        height=height,
        projected_head_triangles=(
            ((0.18, 0.28), (0.49, 0.28), (0.18, 0.72)),
            ((0.49, 0.28), (0.49, 0.72), (0.18, 0.72)),
        ),
    )
    right_face = render_warm_studio_qa.face_mask_from_projected_head_triangles(
        subject,
        width=width,
        height=height,
        projected_head_triangles=(
            ((0.51, 0.28), (0.82, 0.28), (0.82, 0.72)),
            ((0.51, 0.28), (0.82, 0.72), (0.51, 0.72)),
        ),
    )

    assert sum(subject) == 8
    assert left_face != right_face
    assert all(not selected or subject[index] for index, selected in enumerate(left_face))
    assert all(not selected or subject[index] for index, selected in enumerate(right_face))
    assert any(left_face)
    assert any(right_face)


def test_background_mask_excludes_character_practicals_and_clipped_highlights() -> None:
    width = 4
    height = 3
    pixel_count = width * height
    subject_mask = [index in {5, 6} for index in range(pixel_count)]
    practical_mask = [index == 2 for index in range(pixel_count)]
    display_rgba = []
    for index in range(pixel_count):
        value = 1.0 if index == 9 else 0.5
        display_rgba.extend((value, value, value, 1.0))

    background_mask = render_warm_studio_qa.background_mask_from_geometry_masks(
        subject_mask=subject_mask,
        practical_highlight_mask=practical_mask,
        display_rgba=display_rgba,
        width=width,
        height=height,
    )

    assert sum(background_mask) == pixel_count - 4
    assert background_mask[2] is False
    assert background_mask[5] is False
    assert background_mask[6] is False
    assert background_mask[9] is False


def test_qa_render_isolates_and_restores_timeline_camera_markers() -> None:
    original_scene = bpy.context.window.scene
    isolated_scene = bpy.data.scenes.new("Task6_Camera_Marker_Isolation")
    bpy.context.window.scene = isolated_scene
    try:
        scene = bpy.context.scene
        medium = bpy.data.objects["Camera_Medium"]
        wide = bpy.data.objects["Camera_Wide"]
        three_quarter = bpy.data.objects["Camera_ThreeQuarter_Left"]
        for camera in (medium, wide, three_quarter):
            isolated_scene.collection.objects.link(camera)
        marker = scene.timeline_markers.new("Authored_Medium", frame=1)
        marker.camera = medium
        scene.camera = medium
        scene.frame_set(29)
        seen: list[tuple[str, int]] = []

        def capture_render(_path: Path) -> None:
            scene.frame_set(scene.frame_current)
            seen.append(
                (
                    scene.camera.name,
                    sum(item.camera is not None for item in scene.timeline_markers),
                )
            )

        render_warm_studio_qa.render_qa_stills(
            Path("/tmp/task6-camera-marker-isolation-test"),
            engine="eevee",
            camera_names=(wide.name, three_quarter.name),
            render_callback=capture_render,
        )

        assert seen == [
            (wide.name, 0),
            (three_quarter.name, 0),
            (wide.name, 0),
        ]
        restored = list(scene.timeline_markers)
        assert [(item.name, item.frame, item.camera.name) for item in restored] == [
            ("Authored_Medium", 1, medium.name)
        ]
        assert scene.camera is medium
    finally:
        bpy.context.window.scene = original_scene
        bpy.data.scenes.remove(isolated_scene)


def test_camera_pixel_mae_accepts_render_image_arrays() -> None:
    import numpy as np

    first = np.zeros((2, 2, 4), dtype=np.float32)
    second = np.zeros((2, 2, 4), dtype=np.float32)
    second[:, :, :3] = 0.25

    assert abs(render_warm_studio_qa._mean_absolute_rgb_difference(first.reshape(-1), second.reshape(-1)) - 0.25) <= 1e-6


def test_lighting_evidence_uses_linear_luminance_and_rejects_large_clipping() -> None:
    width = 5
    height = 5
    pixel_count = width * height
    subject_mask = [True] * pixel_count
    face_mask = [False] * pixel_count
    background_mask = [False] * pixel_count
    for index in (6, 7, 8, 11, 12, 13, 16, 17, 18):
        face_mask[index] = True
    for index in (0, 1, 2, 3, 4):
        subject_mask[index] = False
        background_mask[index] = True

    linear_rgba = []
    display_rgba = []
    for index in range(pixel_count):
        face_value = 0.40 if face_mask[index] else 0.20 if background_mask[index] else 0.18
        linear_rgba.extend((face_value, face_value, face_value, 1.0))
        display_value = 1.0 if index in {0, 1, 5, 6, 24} else 0.75
        display_rgba.extend((display_value, display_value, display_value, 1.0))

    evidence = render_warm_studio_qa.measure_lighting_evidence(
        linear_rgba=linear_rgba,
        display_rgba=display_rgba,
        subject_mask=subject_mask,
        face_mask=face_mask,
        background_mask=background_mask,
        width=width,
        height=height,
        micro_catchlight_max_pixels=1,
    )

    assert abs(evidence["linearFaceLuminance"] - 0.40) <= 1e-6
    assert abs(evidence["linearBackgroundLuminance"] - 0.20) <= 1e-6
    assert abs(evidence["backgroundStopsBelowFace"] - 1.0) <= 1e-6
    assert evidence["microCatchlightPixelCount"] == 1
    assert evidence["nonCatchlightClippedPixelCount"] == 2
    assert abs(evidence["highlightClipRatio"] - (2 / sum(subject_mask))) <= 1e-6
    assert evidence["backgroundPixelCount"] == sum(background_mask)
    assert evidence["brightNeutralPixelCount"] > 0
    assert 0.95 <= evidence["brightNeutralRedBlueRatio"] <= 1.22
    assert 0.95 <= evidence["brightNeutralRedGreenRatio"] <= 1.14


def test_lighting_evidence_fails_closed_for_empty_masks() -> None:
    width = 2
    height = 2
    rgba = [0.25, 0.25, 0.25, 1.0] * (width * height)
    masks = {
        "subject_mask": [True, True, False, False],
        "face_mask": [True, False, False, False],
        "background_mask": [False, False, True, True],
    }
    for empty_name in masks:
        failing = {name: list(value) for name, value in masks.items()}
        failing[empty_name] = [False] * (width * height)
        try:
            render_warm_studio_qa.measure_lighting_evidence(
                linear_rgba=rgba,
                display_rgba=rgba,
                width=width,
                height=height,
                **failing,
            )
        except ValueError as exc:
            assert "empty" in str(exc)
        else:
            raise AssertionError(f"{empty_name} must fail closed when empty")


def test_subject_lighting_render_plan_covers_both_modes_and_engines() -> None:
    plan = render_warm_studio_qa.subject_lighting_render_plan()
    assert [item["mode"] for item in plan if item["engine"] == "eevee"] == [
        "standing",
        "standing",
        "standing",
        "standing",
        "seated",
        "seated",
        "seated",
        "seated",
    ]
    assert {
        (item["mode"], item["cameraRole"])
        for item in plan
        if item["engine"] == "eevee" and not item["emptyRoom"]
    } == {
        ("standing", "medium"),
        ("standing", "three_quarter"),
        ("standing", "wide"),
        ("seated", "medium"),
        ("seated", "three_quarter"),
        ("seated", "wide"),
    }
    assert {
        (item["mode"], item["engine"], item["cameraRole"], item["emptyRoom"])
        for item in plan
        if item["engine"] == "cycles"
    } == {
        ("standing", "cycles", "medium", False),
        ("standing", "cycles", "medium", True),
        ("seated", "cycles", "medium", False),
        ("seated", "cycles", "medium", True),
    }


def test_lighting_evidence_validator_rejects_non_geometric_masks_and_failed_gates() -> None:
    artifact_root = Path(tempfile.mkdtemp(prefix="warm-studio-evidence-contract-"))
    artifact = artifact_root / "artifact.bin"
    artifact.write_bytes(b"verified evidence artifact")
    artifact_sha256 = render_warm_studio_qa._sha256(artifact)
    image_a = artifact_root / "camera-a.png"
    image_b = artifact_root / "camera-b.png"
    for path, color in (
        (image_a, (0.20, 0.40, 0.60, 1.0)),
        (image_b, (0.40, 0.50, 0.70, 1.0)),
    ):
        image = bpy.data.images.new(path.stem, width=2, height=1, alpha=True)
        image.pixels = [*color, *color]
        image.filepath_raw = str(path)
        image.file_format = "PNG"
        image.save()
        bpy.data.images.remove(image)
    camera_pixel_mae = warm_studio_validator._image_rgb_mae(image_a, image_b)
    artifact_paths = {
        key: str(artifact)
        for key in (
            "beautyPath",
            "linearBeautyPath",
            "emptyRoomPath",
            "linearEmptyRoomPath",
            "subjectMaskPath",
            "faceMaskPath",
            "backgroundMaskPath",
            "practicalHighlightMaskPath",
            "subjectMattePath",
        )
    }
    payload = {
        "schemaVersion": render_warm_studio_qa.LIGHTING_EVIDENCE_SCHEMA,
        "luminanceColorSpace": "scene_linear_rec709",
        "displayColorSpace": "AgX Medium High Contrast PNG",
        "subjectMaskSource": "Rendered character ID matte from actual scene geometry",
        "faceMaskSource": "Rendered character ID matte intersected with projected semantic head geometry",
        "backgroundMaskSource": "Rendered non-character geometry excluding practical-highlight IDs and clipped display highlights",
        "cameraComparisons": [
            {
                "mode": mode,
                "engine": "eevee",
                "cameraRoleA": "medium",
                "cameraRoleB": camera_role,
                "cameraA": f"Camera_{mode.title()}_Medium",
                "cameraB": (
                    f"Camera_{mode.title()}_ThreeQuarter"
                    if camera_role == "three_quarter"
                    else f"Camera_{mode.title()}_Wide"
                ),
                "activeCameraA": f"Camera_{mode.title()}_Medium",
                "activeCameraB": (
                    f"Camera_{mode.title()}_ThreeQuarter"
                    if camera_role == "three_quarter"
                    else f"Camera_{mode.title()}_Wide"
                ),
                "matrixWorldA": [1.0] * 16,
                "matrixWorldB": [2.0] * 16,
                "pixelMae": camera_pixel_mae,
                "imageA": str(image_a),
                "imageB": str(image_b),
                "imageASha256": render_warm_studio_qa._sha256(image_a),
                "imageBSha256": render_warm_studio_qa._sha256(image_b),
            }
            for mode in ("standing", "seated")
            for camera_role in ("three_quarter", "wide")
        ],
        "measurements": [
            {
                "mode": mode,
                "engine": engine,
                "cameraRole": "medium",
                "linearFaceLuminance": 0.40,
                "linearBackgroundLuminance": 0.13195079107728943,
                "backgroundStopsBelowFace": 1.6,
                "highlightClipRatio": 0.0049,
                "brightNeutralPixelCount": 3600,
                "brightNeutralMedianRgb": [0.88, 0.84, 0.80],
                "brightNeutralRedBlueRatio": 1.10,
                "brightNeutralRedGreenRatio": 1.047619,
                "subjectPixelCount": 12000,
                "facePixelCount": 3200,
                "backgroundPixelCount": 180000,
                "practicalHighlightPixelCount": 400,
                **artifact_paths,
                "artifactSha256": {
                    key: artifact_sha256 for key in artifact_paths
                },
            }
            for mode in ("standing", "seated")
            for engine in ("eevee", "cycles")
        ],
    }
    assert warm_studio_validator.validate_lighting_evidence_payload(payload) == []

    payload["cameraComparisons"].append(dict(payload["cameraComparisons"][0]))
    errors = warm_studio_validator.validate_lighting_evidence_payload(payload)
    assert any("cameraComparisons must contain exactly 4 records" in error for error in errors)
    assert any("duplicates camera-role pair" in error for error in errors)
    payload["cameraComparisons"].pop()

    payload["measurements"].append(dict(payload["measurements"][0]))
    errors = warm_studio_validator.validate_lighting_evidence_payload(payload)
    assert any("measurements must contain exactly 4 records" in error for error in errors)
    assert any("duplicates lighting key" in error for error in errors)
    payload["measurements"].pop()

    payload["subjectMaskSource"] = "fixed rectangle"
    payload["measurements"][0]["backgroundStopsBelowFace"] = 1.0
    payload["measurements"][1]["highlightClipRatio"] = 0.005
    payload["measurements"][2]["brightNeutralRedBlueRatio"] = 1.30
    payload["measurements"][3]["linearBackgroundLuminance"] = 0.40
    payload["measurements"][3]["subjectMaskPath"] = str(
        artifact_root / "missing-subject-mask.png"
    )
    payload["cameraComparisons"][1]["imageB"] = str(
        artifact_root / "missing-camera.png"
    )
    payload["cameraComparisons"][0]["pixelMae"] = 0.0
    errors = warm_studio_validator.validate_lighting_evidence_payload(payload)
    assert any("geometry ID matte" in error for error in errors)
    assert any("1.2..2.2" in error for error in errors)
    assert any("below 0.5 percent" in error for error in errors)
    assert any("bright-neutral red/blue ratio" in error for error in errors)
    assert any("inconsistent with linear luminance" in error for error in errors)
    assert any("subjectMaskPath is not a regular file" in error for error in errors)
    assert any("pixel MAE" in error for error in errors)
    assert any("camera comparison imageB is not a regular file" in error for error in errors)


def test_mode_resolver_uses_only_mode_specific_markers_and_cameras() -> None:
    expected = {
        "standing": {
            "spawn": "IP_Standing_Spawn",
            "focus": "IP_Standing_Focus_Head",
            "transition_focus": "IP_Transition_Focus",
            "foot_l": "IP_Standing_Foot_Target.L",
            "foot_r": "IP_Standing_Foot_Target.R",
            "cameras": {
                "wide": "Camera_Standing_Wide",
                "medium": "Camera_Standing_Medium",
                "close": "Camera_Standing_Close",
                "three_quarter": "Camera_Standing_ThreeQuarter",
                "transition": "Camera_Standing_Transition",
            },
        },
        "seated": {
            "spawn": "IP_Seated_Spawn",
            "focus": "IP_Seated_Focus_Head",
            "transition_focus": "IP_Transition_Focus",
            "foot_l": "IP_Seated_Foot_Target.L",
            "foot_r": "IP_Seated_Foot_Target.R",
            "cameras": {
                "wide": "Camera_Seated_Wide",
                "medium": "Camera_Seated_Medium",
                "close": "Camera_Seated_Close",
                "three_quarter": "Camera_Seated_ThreeQuarter",
                "transition": "Camera_Seated_Transition",
            },
        },
    }

    for mode, names in expected.items():
        resolved = blender_renderer.resolve_scene_mode_objects(mode)
        assert resolved["mode"] == mode
        assert resolved["spawn"].name == names["spawn"]
        assert resolved["focus"].name == names["focus"]
        assert resolved["transition_focus"].name == names["transition_focus"]
        assert resolved["seat"].name == "IP_Seat_Target"
        assert resolved["foot_l"].name == names["foot_l"]
        assert resolved["foot_r"].name == names["foot_r"]
        assert {
            role: camera.name for role, camera in resolved["cameras"].items()
        } == names["cameras"]


def test_scene_target_height_reads_the_selected_mode_spawn() -> None:
    standing = bpy.data.objects["IP_Standing_Spawn"]
    seated = bpy.data.objects["IP_Seated_Spawn"]
    old_standing = standing["target_height"]
    old_seated = seated["target_height"]
    try:
        standing["target_height"] = 2.45
        seated["target_height"] = 2.15
        assert blender_renderer.scene_target_height(
            {"presentationMode": "standing", "targetCharacterHeight": 2.55}
        ) == 2.45
        assert blender_renderer.scene_target_height(
            {"presentationMode": "seated", "targetCharacterHeight": 2.55}
        ) == 2.15
    finally:
        standing["target_height"] = old_standing
        seated["target_height"] = old_seated


def test_mode_camera_plan_maps_generic_roles_to_selected_cameras() -> None:
    report = blender_renderer.configure_camera_plan(
        {
            "presentationMode": "seated",
            "cameraPlan": [
                {"frame": 1, "camera": "Camera_Wide"},
                {"frame": 25, "camera": "Camera_Medium"},
                {"frame": 50, "camera": "Camera_Close"},
                {"frame": 75, "camera": "Camera_ThreeQuarter"},
                {"frame": 100, "camera": "Camera_Transition"},
            ],
        },
        blender_renderer.resolve_scene_mode_objects("seated"),
    )

    assert [cut["camera"] for cut in report["cuts"]] == [
        "Camera_Seated_Wide",
        "Camera_Seated_Medium",
        "Camera_Seated_Close",
        "Camera_Seated_ThreeQuarter",
        "Camera_Seated_Transition",
    ]
    assert report["cameraNames"] == {
        "wide": "Camera_Seated_Wide",
        "medium": "Camera_Seated_Medium",
        "close": "Camera_Seated_Close",
        "three_quarter": "Camera_Seated_ThreeQuarter",
        "transition": "Camera_Seated_Transition",
    }
    assert report["missingCameras"] == []
    assert bpy.context.scene.camera.name == "Camera_Seated_Wide"

    preset_report = blender_renderer.configure_camera_plan(
        {"presentationMode": "seated", "cameraPreset": "transition"},
        blender_renderer.resolve_scene_mode_objects("seated"),
    )
    assert preset_report["cuts"] == [
        {"frame": 1, "camera": "Camera_Seated_Transition"}
    ]
    assert preset_report["missingCameras"] == []

    close_report = blender_renderer.configure_camera_plan(
        {"presentationMode": "seated", "cameraPreset": "close"},
        blender_renderer.resolve_scene_mode_objects("seated"),
    )
    assert close_report["cuts"] == [
        {"frame": 1, "camera": "Camera_Seated_Close"}
    ]


def test_authored_scene_placement_persists_selected_mode_contract() -> None:
    mode_objects = blender_renderer.resolve_scene_mode_objects("seated")
    data = {
        "presentationMode": "seated",
        "sceneBlendPath": "warm-sloth-studio-v1.blend",
        "durationSec": 2,
        "fps": 30,
        "resolution": {"width": 1920, "height": 1080},
        "cameraPlan": [{"frame": 1, "camera": "Camera_Medium"}],
    }
    scene_stats = blender_renderer.setup_authored_scene(
        data,
        {"height": 1.0},
        mode_objects,
    )

    bpy.ops.mesh.primitive_cube_add(size=1.0, location=(0.0, 0.0, 0.5))
    character = bpy.context.object
    character.name = "Task5_Test_Character"
    rig = bpy.data.objects.new("Task5_Test_Rig", None)
    bpy.context.scene.collection.objects.link(rig)
    scene_stats["placement"] = blender_renderer.place_character_in_authored_scene(
        [character],
        [],
        rig,
        {},
        {"height": 1.0, "container": None},
        mode_objects,
    )

    placement = scene_stats["placement"]
    assert placement["mode"] == "seated"
    assert placement["markerNames"] == {
        "spawn": "IP_Seated_Spawn",
        "focus": "IP_Seated_Focus_Head",
        "seat": "IP_Seat_Target",
        "foot_l": "IP_Seated_Foot_Target.L",
        "foot_r": "IP_Seated_Foot_Target.R",
    }
    assert placement["cameraNames"] == {
        "wide": "Camera_Seated_Wide",
        "medium": "Camera_Seated_Medium",
        "close": "Camera_Seated_Close",
        "three_quarter": "Camera_Seated_ThreeQuarter",
        "transition": "Camera_Seated_Transition",
    }
    assert placement["placementRoot"] == "IP_Character_Placement"
    assert_location_matches(
        tuple(bpy.data.objects[placement["placementRoot"]].location),
        tuple(mode_objects["spawn"].location),
    )
    assert placement["worldBounds"]["min"] == [-0.5, -0.07, 0.0]
    assert placement["worldBounds"]["max"] == [0.5, 0.93, 1.0]
    assert scene_stats["mode"] == "seated"
    assert scene_stats["markers"] == placement["markerNames"]
    assert scene_stats["cameras"]["cameraNames"] == placement["cameraNames"]
    for role, camera in mode_objects["cameras"].items():
        assert camera.data.dof.use_dof is True
        expected_focus = (
            bpy.data.objects["IP_Transition_Focus"]
            if role == "transition"
            else mode_objects["focus"]
        )
        assert camera.data.dof.focus_object == expected_focus


def test_camera_plan_binds_authored_cameras() -> None:
    reset_scene()
    for name in ("Camera_Wide", "Camera_Medium", "Camera_Close"):
        add_camera(name)

    report = blender_renderer.configure_camera_plan(
        {
            "cameraPreset": "auto",
            "cameraPlan": [
                {"frame": 1, "camera": "Camera_Wide"},
                {"frame": 25, "camera": "Camera_Medium"},
                {"frame": 90, "camera": "Camera_Close"},
            ],
        }
    )

    markers = sorted(bpy.context.scene.timeline_markers, key=lambda marker: marker.frame)
    assert [marker.frame for marker in markers] == [1, 25, 90]
    assert [marker.camera.name for marker in markers] == ["Camera_Wide", "Camera_Medium", "Camera_Close"]
    assert bpy.context.scene.camera.name == "Camera_Wide"
    assert report["missingCameras"] == []


def test_shoe_sampling_rejects_render_disabled_armature_modifier() -> None:
    reset_scene()
    armature_data = bpy.data.armatures.new("Task5_Armature_Data")
    armature = bpy.data.objects.new("Task5_Armature", armature_data)
    bpy.context.scene.collection.objects.link(armature)
    bpy.ops.mesh.primitive_cube_add(size=1.0)
    shoe = bpy.context.object
    shoe.name = "Task5_Shoe"
    modifier = shoe.modifiers.new("Armature", "ARMATURE")
    modifier.object = armature
    modifier.show_viewport = True
    modifier.show_render = False

    try:
        warm_character_validator.assert_render_armature_modifiers([shoe], armature)
    except RuntimeError as exc:
        assert "show_render=true" in str(exc)
    else:
        raise AssertionError("render-disabled Armature modifier was accepted")


def test_collision_counter_uses_evaluated_triangle_geometry() -> None:
    reset_scene()
    bpy.ops.mesh.primitive_cube_add(size=1.0, location=(0.0, 0.0, 0.5))
    character = bpy.context.object
    character.name = "Task5_Collision_Character"
    bpy.ops.mesh.primitive_cube_add(size=1.0, location=(0.75, 0.0, 0.5))
    obstacle = bpy.context.object
    obstacle.name = "Task5_Collision_Obstacle"

    report = warm_character_validator.count_evaluated_mesh_intersections(
        [character],
        [obstacle],
        bpy.context.evaluated_depsgraph_get(),
    )

    assert report["trianglePairCount"] > 0
    assert report["objectPairs"] == [
        ["Task5_Collision_Character", "Task5_Collision_Obstacle"]
    ]


def add_crossing_triangle(name: str) -> bpy.types.Object:
    mesh = bpy.data.meshes.new(f"{name}_Mesh")
    mesh.from_pydata(
        [(-2.0, 0.0, -2.0), (2.0, 0.0, -2.0), (0.0, 0.0, 2.0)],
        [],
        [(0, 1, 2)],
    )
    obj = bpy.data.objects.new(name, mesh)
    bpy.context.scene.collection.objects.link(obj)
    return obj


def test_full_collision_detects_crossing_triangle_with_all_vertices_outside_frame() -> None:
    reset_scene()
    character = add_crossing_triangle("Task5_CrossFrame_Character")
    bpy.ops.mesh.primitive_cube_add(size=0.2, location=(0.0, 0.0, 0.0))
    obstacle = bpy.context.object
    obstacle.name = "Task5_CrossFrame_Obstacle"
    camera = add_camera("Task5_CrossFrame_Camera")
    camera.data.type = "ORTHO"
    camera.data.ortho_scale = 2.0
    camera.location = (0.0, -5.0, 0.0)
    blender_renderer.look_at(camera, (0.0, 0.0, 0.0))

    report = warm_character_validator.count_full_evaluated_mesh_intersections(
        [character],
        [obstacle],
        bpy.context.evaluated_depsgraph_get(),
    )

    assert report["trianglePairCount"] > 0


def test_hand_collision_uses_weighted_faces_not_vertex_inside_checks() -> None:
    reset_scene()
    armature_data = bpy.data.armatures.new("Task5_Hand_Armature_Data")
    armature = bpy.data.objects.new("Task5_Hand_Armature", armature_data)
    bpy.context.scene.collection.objects.link(armature)
    hand = add_crossing_triangle("Task5_Weighted_Hand")
    group = hand.vertex_groups.new(name="LeftHand")
    group.add([0, 1, 2], 1.0, "REPLACE")
    modifier = hand.modifiers.new("Armature", "ARMATURE")
    modifier.object = armature
    modifier.show_viewport = True
    modifier.show_render = True
    bpy.ops.mesh.primitive_cube_add(size=0.2, location=(0.0, 0.0, 0.0))
    obstacle = bpy.context.object
    obstacle.name = "Task5_Hand_Obstacle"

    report = warm_character_validator.count_hand_weighted_face_intersections(
        [hand],
        [obstacle],
        armature,
        {"hand_l": "LeftHand", "hand_r": "RightHand"},
        bpy.context.evaluated_depsgraph_get(),
    )

    assert report["trianglePairCount"] > 0
    assert report["sampledFaceCount"] == 1


def test_warm_authored_scene_never_falls_back_to_shared_markers() -> None:
    reset_scene()
    bpy.context.scene["ip_presentation_modes"] = '["standing", "seated"]'
    for name in (
        "IP_Character_Spawn",
        "IP_Focus_Head",
        "IP_Seat_Target",
        "IP_Foot_Target.L",
        "IP_Foot_Target.R",
    ):
        marker = bpy.data.objects.new(name, None)
        bpy.context.scene.collection.objects.link(marker)
    for name in ("Camera_Wide", "Camera_Medium", "Camera_Close"):
        add_camera(name)

    try:
        blender_renderer.resolve_authored_scene_mode_objects(
            {"presentationMode": "seated"}
        )
    except RuntimeError as exc:
        assert "IP_Seated_Spawn" in str(exc)
        assert "IP_Seated_Foot_Target.L" in str(exc)
    else:
        raise AssertionError("warm scene accepted shared standing aliases")

    del bpy.context.scene["ip_presentation_modes"]
    assert blender_renderer.resolve_authored_scene_mode_objects({}) is None


def test_validation_success_fails_each_required_geometry_gate() -> None:
    passing = {
        "mode": "seated",
        "sampleCount": 3,
        "floorClearance": {
            "left": {"minimum": 0.001},
            "right": {"minimum": 0.002},
        },
        "deskIntersectionCount": 0,
        "chairIntersectionCount": 0,
        "handIntersectionCount": 0,
        "deformationSpikeCount": 0,
        "cameraVisibility": {
            "head": {"insideCount": 12},
            "leftHand": {"insideCount": 8},
            "rightHand": {"insideCount": 9},
        },
        "frames": [
            {
                "frame": frame,
                "cameraVisibility": {
                    role: {
                        "insideCount": 128,
                        "frameBounds": {
                            "min": [0.04, 0.04],
                            "max": [0.96, 0.96],
                        },
                    }
                    for role in ("head", "leftHand", "rightHand")
                },
            }
            for frame in (1, 15, 29)
        ],
    }
    report = warm_character_validator.finalize_mode_report(passing)
    assert report["success"] is True
    assert report["failureReasons"] == []

    failing_changes = (
        ("floorClearance", {"left": {"minimum": -0.000001}, "right": {"minimum": 0.0}}),
        ("deskIntersectionCount", 1),
        ("chairIntersectionCount", 1),
        ("handIntersectionCount", 1),
        ("deformationSpikeCount", 1),
    )
    for key, value in failing_changes:
        candidate = {
            **passing,
            "floorClearance": {
                side: dict(metrics)
                for side, metrics in passing["floorClearance"].items()
            },
            "cameraVisibility": {
                role: dict(metrics)
                for role, metrics in passing["cameraVisibility"].items()
            },
            key: value,
        }
        failed = warm_character_validator.finalize_mode_report(candidate)
        assert failed["success"] is False, key
        assert failed["failureReasons"], key

    per_frame = {
        **passing,
        "frames": [
            *passing["frames"][:1],
            {
                "frame": 15,
                "cameraVisibility": {
                    role: {
                        **metrics,
                        "insideCount": 127 if role == "leftHand" else 128,
                    }
                    for role, metrics in passing["frames"][1]["cameraVisibility"].items()
                },
            },
            *passing["frames"][2:],
        ],
    }
    failed = warm_character_validator.finalize_mode_report(per_frame)
    assert failed["success"] is False
    assert any("frame 15 leftHand" in reason for reason in failed["failureReasons"])

    unsafe_bounds = {
        **passing,
        "frames": [
            *passing["frames"][:1],
            {
                "frame": 15,
                "cameraVisibility": {
                    "head": {
                        "insideCount": 128,
                        "frameBounds": {
                            "min": [0.04, 0.04],
                            "max": [0.96, 0.961],
                        },
                    },
                    "leftHand": {
                        "insideCount": 128,
                        "frameBounds": {
                            "min": [0.04, 0.039],
                            "max": [0.96, 0.96],
                        },
                    },
                    "rightHand": {
                        "insideCount": 128,
                        "frameBounds": {
                            "min": [0.04, 0.04],
                            "max": [0.96, 0.96],
                        },
                    },
                },
            },
            *passing["frames"][2:],
        ],
    }
    failed = warm_character_validator.finalize_mode_report(unsafe_bounds)
    assert failed["success"] is False
    assert any("frame 15 head" in reason and "maxY" in reason for reason in failed["failureReasons"])
    assert any("frame 15 leftHand" in reason and "minY" in reason for reason in failed["failureReasons"])


def test_scene_marker_controls_character_height() -> None:
    reset_scene()
    spawn = bpy.data.objects.new("IP_Character_Spawn", None)
    spawn["target_height"] = 2.35
    bpy.context.collection.objects.link(spawn)

    height = blender_renderer.scene_target_height({"targetCharacterHeight": 2.55})

    assert abs(height - 2.35) < 1e-6


def test_lighting_preset_uses_authored_base_energy() -> None:
    reset_scene()
    bpy.ops.object.light_add(type="AREA")
    key = bpy.context.object
    key.name = "Studio_Key"
    key.data.energy = 500
    key["ip_base_energy"] = 500.0
    key["ip_light_role"] = "key"
    bpy.ops.object.light_add(type="AREA")
    fill = bpy.context.object
    fill["ip_light_role"] = "fill"
    bpy.ops.object.light_add(type="POINT")
    practical = bpy.context.object
    practical["ip_light_role"] = "practical"

    report = blender_renderer.apply_lighting_preset("editorial_crisp")

    assert key.data.energy == 575.0
    assert key.data.use_shadow is True
    assert fill.data.use_shadow is False
    assert practical.data.use_shadow is False
    assert report["lightCount"] == 3
    assert report["shadowCasterCount"] == 1
    assert report["preset"] == "editorial_crisp"


def test_production_calibration_frames_include_interval_bounds_and_limb_extrema() -> None:
    scene = bpy.context.scene
    original_range = (scene.frame_start, scene.frame_end)
    bpy.ops.object.armature_add()
    marker = bpy.context.object
    marker.name = "Calibration_Frame_Test"
    limb = marker.pose.bones[0]
    limb.name = "Calibration_Limb"
    try:
        scene.frame_start = 1
        scene.frame_end = 61
        limb.rotation_mode = "XYZ"
        for frame, value in (
            (20, 0.0),
            (21, 0.1),
            (22, 0.2),
            (23, 0.3),
            (24, 0.2),
            (25, 0.1),
            (26, 0.0),
        ):
            limb.rotation_euler.x = value
            limb.keyframe_insert(data_path="rotation_euler", frame=frame, index=0)
        marker.location.x = 0.0
        marker.keyframe_insert(data_path="location", frame=27, index=0)

        frames = blender_renderer.production_calibration_frames(
            scene,
            30,
            actions=(marker.animation_data.action,),
            bone_names=(limb.name,),
        )

        assert {1, 16, 20, 23, 26, 31, 46, 61}.issubset(frames)
        assert 21 not in frames
        assert 22 not in frames
        assert 24 not in frames
        assert 25 not in frames
        assert 27 not in frames
        assert len(frames) < 61
    finally:
        bpy.data.objects.remove(marker, do_unlink=True)
        scene.frame_start, scene.frame_end = original_range


def test_medium_frame_boundary_reduction_preserves_candidate_bounds() -> None:
    reset_scene()
    camera = add_camera("Boundary_Reduction_Camera")
    camera.location = (0.0, -5.0, 0.0)
    blender_renderer.look_at(camera, (0.0, 0.0, 0.0))
    bpy.context.view_layer.update()
    points = [
        Vector((-0.4 + 0.8 * (index / 129.0), 0.0, 0.3 * math.sin(index)))
        for index in range(130)
    ]
    reduced = blender_renderer._medium_frame_boundary_regions(
        camera,
        {"head": points},
    )

    assert reduced["head"]["sampleCount"] == 130
    assert len(reduced["head"]["boundaryPoints"]) <= 5
    for lens in (50.0, 35.0):
        camera.data.lens = lens
        expected = [
            world_to_camera_view(bpy.context.scene, camera, point) for point in points
        ]
        metrics = blender_renderer._medium_frame_metrics(
            camera,
            {1: reduced},
        )[1]["head"]
        assert metrics["insideCount"] == 130, (metrics, reduced["head"])
        assert abs(metrics["frameBounds"]["min"][0] - min(point.x for point in expected)) < 1e-6
        assert abs(metrics["frameBounds"]["min"][1] - min(point.y for point in expected)) < 1e-6
        assert abs(metrics["frameBounds"]["max"][0] - max(point.x for point in expected)) < 1e-6
        assert abs(metrics["frameBounds"]["max"][1] - max(point.y for point in expected)) < 1e-6


def test_medium_camera_calibration_preserves_long_lens_and_dollies_back() -> None:
    reset_scene()
    camera = add_camera("Dolly_Calibration_Camera")
    camera.location = (0.0, -2.2, 1.35)
    camera.data.lens = 50.0
    camera.data["ip_authored_shift_y"] = 0.0
    blender_renderer.look_at(camera, (0.0, 0.0, 1.35))
    authored_location = tuple(camera.location)

    def region(center_x: float, center_z: float, radius: float) -> list[Vector]:
        return [
            Vector(
                (
                    center_x + radius * math.cos(index * math.tau / 130.0),
                    0.0,
                    center_z + radius * math.sin(index * math.tau / 130.0),
                )
            )
            for index in range(130)
        ]

    regions = {
        "head": region(0.0, 1.82, 0.30),
        "leftHand": region(-1.02, 1.34, 0.16),
        "rightHand": region(1.02, 1.34, 0.16),
    }
    original_sampler = blender_renderer.sample_character_semantic_regions
    blender_renderer.sample_character_semantic_regions = lambda *_args, **_kwargs: regions
    try:
        report = blender_renderer.calibrate_mode_medium_camera(
            [],
            {},
            {"cameras": {"medium": camera}},
            sample_frames=(1, 30),
        )
    finally:
        blender_renderer.sample_character_semantic_regions = original_sampler

    assert report["lens"] >= 48.0, report
    assert report["dollyDistance"] > 0.0, report
    assert tuple(report["authoredLocation"]) == authored_location, report
    assert tuple(report["location"]) != authored_location, report
    metrics = {
        item["frame"]: {key: value for key, value in item.items() if key != "frame"}
        for item in report["frames"]
    }
    assert blender_renderer._medium_frame_candidate_is_safe(metrics), report


def test_dollied_camera_opens_the_named_studio_fourth_wall() -> None:
    reset_scene()
    for name in blender_renderer.REMOVABLE_FOURTH_WALL_NAMES:
        bpy.ops.mesh.primitive_cube_add(size=1.0)
        bpy.context.object.name = name

    report = blender_renderer.configure_removable_fourth_wall(1.95)

    assert report["status"] == "open"
    assert set(report["objects"]) == set(blender_renderer.REMOVABLE_FOURTH_WALL_NAMES)
    assert all(bpy.data.objects[name].hide_render for name in report["objects"])
    assert all(
        bpy.data.objects[name].get("ip_removable_fourth_wall") is True
        for name in report["objects"]
    )


def test_render_settings_are_compatible_with_blender_51_agx_and_eevee() -> None:
    reset_scene()

    blender_renderer.configure_render_settings(
        {
            "durationSec": 1,
            "fps": 30,
            "resolution": {"width": 640, "height": 360},
            "renderEngine": "BLENDER_EEVEE_NEXT",
            "transparent": True,
        },
        authored_scene=False,
    )

    scene = bpy.context.scene
    assert scene.render.engine in {"BLENDER_EEVEE", "BLENDER_EEVEE_NEXT"}
    assert scene.view_settings.view_transform == "AgX"
    assert "Medium High Contrast" in scene.view_settings.look
    assert scene.render.film_transparent is True


def test_lighting_qa_applies_and_restores_cycles_key_multiplier() -> None:
    reset_scene()
    data = bpy.data.lights.new("QA_Key_Data", "SPOT")
    key = bpy.data.objects.new("QA_Key", data)
    bpy.context.scene.collection.objects.link(key)
    key["ip_light_role"] = "key"
    key["ip_base_energy"] = 600.0
    bpy.context.scene["ip_cycles_key_energy_multiplier"] = 4.65

    render_warm_studio_qa._set_engine_and_samples(bpy.context.scene, "cycles")
    assert abs(key.data.energy - 2790.0) <= 1e-6

    render_warm_studio_qa._set_engine_and_samples(bpy.context.scene, "eevee")
    assert abs(key.data.energy - 600.0) <= 1e-6


def test_editorial_studio_uses_aroll_camera_framing_and_restrained_background_emission() -> None:
    objects = editorial_studio_builder.build_scene()

    wide = objects["wide"]
    medium = objects["medium"]
    close = objects["close"]
    assert 7.2 <= abs(wide.location.y) <= 8.4
    assert 48 <= wide.data.lens <= 56
    assert 4.5 <= abs(medium.location.y) <= 5.8
    assert 55 <= medium.data.lens <= 62
    assert 3.5 <= abs(close.location.y) <= 4.8
    assert close.data.lens >= 68

    cyan = bpy.data.materials["Editorial_Cyan_Accent"]
    cyan_bsdf = next(node for node in cyan.node_tree.nodes if node.type == "BSDF_PRINCIPLED")
    assert cyan_bsdf.inputs["Emission Strength"].default_value <= 0.9


def test_default_aroll_character_validation_passes_supported_modes() -> None:
    manifest = json.loads(DEFAULT_MANIFEST_PATH.read_text(encoding="utf-8"))
    supported_modes = tuple(manifest["supportedOverrides"]["presentationModes"])
    assert supported_modes == ("standing",)
    reports = warm_character_validator.validate_modes(
        scene_path=DEFAULT_STUDIO_PATH,
        master_path=DEFAULT_MASTER_PATH,
        modes=supported_modes,
        sample_frames=(1, 15, 29),
    )

    assert [report["mode"] for report in reports] == list(supported_modes)
    for report in reports:
        assert report["sampleCount"] == 3
        assert set(report["floorClearance"]) == {"left", "right"}
        assert "deskIntersectionCount" in report
        assert "chairIntersectionCount" in report
        assert "handIntersectionCount" in report
        assert "deformationSpikeCount" in report
        assert set(report["cameraVisibility"]) >= {"head", "leftHand", "rightHand"}
        for side in ("left", "right"):
            assert all(
                0.0 <= clearance <= 0.0035
                for clearance in report["floorClearance"][side]["samples"]
            ), report
        for frame in report["frames"]:
            for role in ("head", "leftHand", "rightHand"):
                visibility = frame["cameraVisibility"][role]
                assert visibility["insideCount"] >= 128, frame
                bounds = visibility["frameBounds"]
                assert all(value >= 0.04 for value in bounds["min"]), frame
                assert all(value <= 0.96 for value in bounds["max"]), frame
        assert report["success"] is True, report


if __name__ == "__main__":
    if bpy.context.scene.get("ip_scene_contract") != "tangying-warm-sloth-studio/v1":
        bpy.ops.wm.open_mainfile(
            filepath=str(
                REPO_ROOT / "ip形象/main_ip/scenes/warm-sloth-studio-v1.blend"
            )
        )
    tests = [
        test_warm_studio_saved_scene_has_dual_mode_contract,
        test_transition_stage_preserves_foreground_desk_for_authored_composition,
        test_transition_collision_calibration_allows_authored_chair_contact,
        test_production_validator_rejects_mode_camera_broken_dof_focus_object,
        test_luminance_masks_follow_rendered_subject_and_projected_head,
        test_background_mask_excludes_character_practicals_and_clipped_highlights,
        test_qa_render_isolates_and_restores_timeline_camera_markers,
        test_camera_pixel_mae_accepts_render_image_arrays,
        test_lighting_evidence_uses_linear_luminance_and_rejects_large_clipping,
        test_lighting_evidence_fails_closed_for_empty_masks,
        test_subject_lighting_render_plan_covers_both_modes_and_engines,
        test_lighting_evidence_validator_rejects_non_geometric_masks_and_failed_gates,
        test_mode_resolver_uses_only_mode_specific_markers_and_cameras,
        test_scene_target_height_reads_the_selected_mode_spawn,
        test_mode_camera_plan_maps_generic_roles_to_selected_cameras,
        test_authored_scene_placement_persists_selected_mode_contract,
        test_camera_plan_binds_authored_cameras,
        test_shoe_sampling_rejects_render_disabled_armature_modifier,
        test_collision_counter_uses_evaluated_triangle_geometry,
        test_full_collision_detects_crossing_triangle_with_all_vertices_outside_frame,
        test_hand_collision_uses_weighted_faces_not_vertex_inside_checks,
        test_warm_authored_scene_never_falls_back_to_shared_markers,
        test_validation_success_fails_each_required_geometry_gate,
        test_scene_marker_controls_character_height,
        test_lighting_preset_uses_authored_base_energy,
        test_production_calibration_frames_include_interval_bounds_and_limb_extrema,
        test_medium_frame_boundary_reduction_preserves_candidate_bounds,
        test_medium_camera_calibration_preserves_long_lens_and_dollies_back,
        test_dollied_camera_opens_the_named_studio_fourth_wall,
        test_render_settings_are_compatible_with_blender_51_agx_and_eevee,
        test_lighting_qa_applies_and_restores_cycles_key_multiplier,
        test_editorial_studio_uses_aroll_camera_framing_and_restrained_background_emission,
        test_default_aroll_character_validation_passes_supported_modes,
    ]
    try:
        for test in tests:
            test()
            print(f"PASS {test.__name__}")
    except Exception:
        import traceback

        traceback.print_exc()
        raise SystemExit(1)
