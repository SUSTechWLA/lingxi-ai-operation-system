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
REPO_ROOT = SCRIPT_DIR.parents[1]
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import blender_renderer
import editorial_studio_builder
import validate_warm_studio_character as warm_character_validator
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


def test_mode_resolver_uses_only_mode_specific_markers_and_cameras() -> None:
    expected = {
        "standing": {
            "spawn": "IP_Standing_Spawn",
            "focus": "IP_Standing_Focus_Head",
            "foot_l": "IP_Standing_Foot_Target.L",
            "foot_r": "IP_Standing_Foot_Target.R",
            "cameras": {
                "wide": "Camera_Standing_Wide",
                "medium": "Camera_Standing_Medium",
                "three_quarter": "Camera_Standing_ThreeQuarter",
            },
        },
        "seated": {
            "spawn": "IP_Seated_Spawn",
            "focus": "IP_Seated_Focus_Head",
            "foot_l": "IP_Seated_Foot_Target.L",
            "foot_r": "IP_Seated_Foot_Target.R",
            "cameras": {
                "wide": "Camera_Seated_Wide",
                "medium": "Camera_Seated_Medium",
                "three_quarter": "Camera_Seated_ThreeQuarter",
            },
        },
    }

    for mode, names in expected.items():
        resolved = blender_renderer.resolve_scene_mode_objects(mode)
        assert resolved["mode"] == mode
        assert resolved["spawn"].name == names["spawn"]
        assert resolved["focus"].name == names["focus"]
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
                {"frame": 50, "camera": "Camera_ThreeQuarter"},
            ],
        },
        blender_renderer.resolve_scene_mode_objects("seated"),
    )

    assert [cut["camera"] for cut in report["cuts"]] == [
        "Camera_Seated_Wide",
        "Camera_Seated_Medium",
        "Camera_Seated_ThreeQuarter",
    ]
    assert report["cameraNames"] == {
        "wide": "Camera_Seated_Wide",
        "medium": "Camera_Seated_Medium",
        "three_quarter": "Camera_Seated_ThreeQuarter",
    }
    assert report["missingCameras"] == []
    assert bpy.context.scene.camera.name == "Camera_Seated_Wide"


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
        "three_quarter": "Camera_Seated_ThreeQuarter",
    }
    assert placement["placementRoot"] == "IP_Character_Placement"
    assert_location_matches(
        tuple(bpy.data.objects[placement["placementRoot"]].location),
        tuple(mode_objects["spawn"].location),
    )
    assert placement["worldBounds"]["min"] == [-0.5, -0.03, 0.0]
    assert placement["worldBounds"]["max"] == [0.5, 0.97, 1.0]
    assert scene_stats["mode"] == "seated"
    assert scene_stats["markers"] == placement["markerNames"]
    assert scene_stats["cameras"]["cameraNames"] == placement["cameraNames"]
    for camera in mode_objects["cameras"].values():
        assert camera.data.dof.use_dof is True
        assert camera.data.dof.focus_object == mode_objects["focus"]


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


def test_real_warm_studio_character_validation_passes_both_modes() -> None:
    reports = warm_character_validator.validate_modes(
        scene_path=REPO_ROOT / "ip形象/main_ip/scenes/warm-sloth-studio-v1.blend",
        master_path=REPO_ROOT / "ip形象/main_ip/models/main-ip-aroll-master.blend",
        modes=("standing", "seated"),
        sample_frames=(1, 15, 29),
    )

    assert [report["mode"] for report in reports] == ["standing", "seated"]
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
    tests = [
        test_warm_studio_saved_scene_has_dual_mode_contract,
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
        test_render_settings_are_compatible_with_blender_51_agx_and_eevee,
        test_editorial_studio_uses_aroll_camera_framing_and_restrained_background_emission,
        test_real_warm_studio_character_validation_passes_both_modes,
    ]
    for test in tests:
        test()
        print(f"PASS {test.__name__}")
