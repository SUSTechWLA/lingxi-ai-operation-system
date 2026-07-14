#!/usr/bin/env python3
"""Blender-side contract tests for reusable talking-head studio scenes."""

from __future__ import annotations

import sys
import unittest
from pathlib import Path

try:
    import bpy
except ModuleNotFoundError as exc:
    raise unittest.SkipTest("requires Blender's bpy runtime") from exc

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import blender_renderer
import editorial_studio_builder
import warm_studio_contract as contract


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
    assert scene["ip_subject_light_profile"] == "warm_subject_first_v1"
    assert scene["ip_background_stops_below_face"] == 1.25

    for mode in contract.PRESENTATION_MODES:
        marker_specs = contract.MODE_MARKER_SPECS[mode]
        marker_names = {
            "spawn": f"IP_{mode.title()}_Spawn",
            "focus": f"IP_{mode.title()}_Focus_Head",
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

    for name in ("IP_Seat_Target", "IP_Foot_Target.L", "IP_Foot_Target.R"):
        assert bpy.data.objects.get(name) is not None, name

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

    key = bpy.data.objects["Studio_Key"]
    assert key.data.use_temperature is True
    assert key.data.temperature == profile["keyTemperatureK"]

    rims = [
        obj
        for obj in scene.objects
        if obj.type == "LIGHT"
        and obj.get("ip_light_role") == "rim"
        and obj.data.use_temperature
        and obj.data.temperature == profile["rimTemperatureK"]
    ]
    assert len(rims) == 1


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


if __name__ == "__main__":
    tests = [
        test_warm_studio_saved_scene_has_dual_mode_contract,
        test_camera_plan_binds_authored_cameras,
        test_scene_marker_controls_character_height,
        test_lighting_preset_uses_authored_base_energy,
        test_render_settings_are_compatible_with_blender_51_agx_and_eevee,
        test_editorial_studio_uses_aroll_camera_framing_and_restrained_background_emission,
    ]
    for test in tests:
        test()
        print(f"PASS {test.__name__}")
