#!/usr/bin/env python3
"""Blender-independent contract tests for the warm studio scene."""

from __future__ import annotations

import json
import unittest
from pathlib import Path

import warm_studio_contract as contract


class WarmStudioContractTests(unittest.TestCase):
    def test_main_profile_uses_warm_studio_without_regression(self) -> None:
        profile = json.loads(
            (
                contract.repo_root()
                / "ip形象/main_ip/character-profile.json"
            ).read_text()
        )
        render = profile["render"]
        voice = profile["voice"]

        self.assertEqual(
            profile["model"]["masterBlendPath"],
            "models/main-ip-aroll-master-20260720.blend",
        )
        self.assertEqual(
            render["sceneBlendPath"],
            "scenes/warm-sloth-studio-20260720.blend",
        )
        self.assertEqual(
            profile["defaultAssetManifest"],
            "manifests/default-aroll-assets.json",
        )
        self.assertEqual(render["presentationMode"], "standing")
        self.assertEqual(render["cameraPreset"], "front_talking")
        self.assertEqual(render["qualityPreset"], "production_1080p")
        self.assertEqual(render["resolution"], {"width": 1920, "height": 1080})
        self.assertEqual(render["fps"], 30)
        self.assertEqual(voice["provider"], "gpt_sovits_local")
        self.assertEqual(voice["voiceId"], "main_ip_warm_knowledge_host_v1")
        self.assertEqual(voice["fallbackPolicy"], "error")

    def test_room_and_opening_dimensions_are_fixed(self) -> None:
        self.assertEqual(contract.ROOM_SIZE, (6.2, 5.8, 3.4))
        self.assertEqual(contract.DESK_SIZE, (2.7, 0.95, 0.92))
        self.assertEqual(contract.WINDOW_OPENING, (2.6, 2.75))
        self.assertEqual(contract.DOOR_OPENING, (1.0, 2.4))
        self.assertEqual(contract.TARGET_CHARACTER_HEIGHT, 2.55)

    def test_production_render_resolution_is_1080p(self) -> None:
        self.assertEqual(contract.DEFAULT_RENDER_RESOLUTION, (1920, 1080))

    def test_dual_mode_contract_is_explicit(self) -> None:
        self.assertEqual(contract.PRESENTATION_MODES, ("standing", "seated"))
        self.assertEqual(set(contract.MODE_MARKER_SPECS), {"standing", "seated"})
        self.assertEqual(set(contract.MODE_CAMERA_SPECS), {"standing", "seated"})
        for mode in contract.PRESENTATION_MODES:
            self.assertEqual(
                set(contract.MODE_MARKER_SPECS[mode]),
                {"spawn", "focus", "seat", "knee_l", "knee_r", "foot_l", "foot_r"},
            )
            self.assertEqual(
                set(contract.MODE_CAMERA_SPECS[mode]),
                {"wide", "medium", "close", "three_quarter", "transition"},
            )

    def test_front_talking_cameras_are_centered_for_both_modes(self) -> None:
        for mode in contract.PRESENTATION_MODES:
            for role in ("wide", "medium", "close"):
                _name, location, _lens = contract.MODE_CAMERA_SPECS[mode][role]
                self.assertEqual(location[0], 0.0, f"{mode}.{role} must stay frontal")

    def test_seated_contract_has_knees_and_transition_camera(self) -> None:
        seated = contract.MODE_MARKER_SPECS["seated"]
        self.assertEqual(
            set(seated),
            {"spawn", "focus", "seat", "knee_l", "knee_r", "foot_l", "foot_r"},
        )
        self.assertIn("transition", contract.MODE_CAMERA_SPECS["standing"])
        self.assertIn("transition", contract.MODE_CAMERA_SPECS["seated"])
        self.assertEqual(
            contract.MODE_CAMERA_SPECS["standing"]["transition"][0],
            "Camera_Standing_Transition",
        )

    def test_stool_visibility_contract_is_not_full_occlusion(self) -> None:
        self.assertEqual(contract.SEAT_VISIBILITY_PROFILE["strategy"], "partial_profile")
        self.assertGreaterEqual(
            contract.SEAT_VISIBILITY_PROFILE["minimumVisibleFraction"], 0.08
        )
        self.assertLessEqual(
            contract.SEAT_VISIBILITY_PROFILE["maximumVisibleFraction"], 0.28
        )

    def test_hero_chair_has_readable_compact_profile(self) -> None:
        self.assertEqual(contract.HERO_CHAIR_LOCATION, (0.0, 1.35, 0.0))
        self.assertEqual(contract.HERO_CHAIR_SEAT_SIZE, (1.06, 0.58, 0.12))
        self.assertEqual(contract.HERO_CHAIR_BACK_SIZE, (1.03, 0.10, 0.54))
        self.assertEqual(contract.HERO_CHAIR_ARM_X, 0.56)
        self.assertEqual(contract.HERO_CHAIR_FOOT_X, 0.415)

    def test_transition_camera_is_wide_enough_for_full_body_contact(self) -> None:
        self.assertEqual(contract.TRANSITION_FOCUS_LOCATION, (0.0, 0.38, 1.48))
        for mode in contract.PRESENTATION_MODES:
            self.assertEqual(contract.MODE_CAMERA_SPECS[mode]["transition"][2], 20.0)

    def test_subject_first_light_profile_is_bounded(self) -> None:
        profile = contract.SUBJECT_LIGHT_PROFILE
        self.assertEqual(profile["name"], "bright_subject_first_v5")
        self.assertEqual(profile["worldStrength"], 0.1)
        self.assertEqual(profile["keyTemperatureK"], 6500)
        self.assertEqual(profile["keyEnergy"], 1100.0)
        self.assertEqual(profile["keyTarget"], (0.0, 0.365, 1.9))
        self.assertEqual(profile["keySpotSizeDegrees"], 55.0)
        self.assertEqual(profile["keySpotBlend"], 0.65)
        self.assertEqual(profile["keySoftRadius"], 0.55)
        self.assertEqual(profile["fillEnergy"], 110.0)
        self.assertEqual(profile["frontFillEnergy"], 180.0)
        self.assertEqual(profile["frontFillTemperatureK"], 5600)
        self.assertEqual(profile["frontFillTarget"], (0.0, 0.365, 1.2))
        self.assertEqual(profile["rimEnergy"], 180.0)
        self.assertEqual(profile["rimSpotSizeDegrees"], 28.0)
        self.assertEqual(profile["rimSpotBlend"], 0.65)
        self.assertEqual(profile["rimSoftRadius"], 0.5)
        self.assertEqual(profile["windowEnergy"], 50.0)
        self.assertEqual(profile["practicalWallEnergy"], 28.0)
        self.assertEqual(profile["practicalShelfEnergy"], 30.0)
        self.assertEqual(profile["downlightEnergy"], 5.0)
        self.assertEqual(profile["authoredExposure"], -2.45)
        self.assertEqual(profile["cyclesFinalExposure"], -3.7)
        self.assertEqual(profile["cyclesKeyEnergyMultiplier"], 3.5)
        self.assertEqual(profile["whiteBalanceTemperatureK"], 4500.0)
        self.assertEqual(profile["whiteBalanceTint"], 10.0)
        self.assertEqual(profile["brightNeutralRedBlueRatio"], (0.95, 1.22))
        self.assertEqual(profile["brightNeutralRedGreenRatio"], (0.95, 1.14))
        self.assertEqual(profile["rimTemperatureK"], 3200)
        self.assertEqual(profile["backgroundStopsBelowFace"], 1.75)
        self.assertEqual(profile["backgroundStopsRange"], (1.2, 2.2))
        self.assertGreater(
            profile["keyEnergy"],
            4.0
            * max(
                profile["fillEnergy"],
                profile["frontFillEnergy"],
                profile["rimEnergy"],
            ),
        )

    def test_required_collections_are_complete_and_immutable(self) -> None:
        self.assertIsInstance(contract.REQUIRED_COLLECTIONS, tuple)
        self.assertEqual(
            contract.REQUIRED_COLLECTIONS,
            (
                "STUDIO_ARCHITECTURE",
                "STUDIO_FURNITURE",
                "STUDIO_PROPS",
                "STUDIO_VEGETATION",
                "STUDIO_BRAND",
                "STUDIO_LIGHTS",
                "STUDIO_CAMERAS",
                "STUDIO_MARKERS",
                "QA_ONLY",
            ),
        )

    def test_required_objects_are_complete_and_immutable(self) -> None:
        self.assertIsInstance(contract.REQUIRED_OBJECTS, tuple)
        self.assertEqual(
            contract.REQUIRED_OBJECTS,
            (
                "SET_MASTER",
                "IP_Character_Spawn",
                "IP_Focus_Head",
                "IP_Focus_Desk",
                "IP_Focus_Shelf",
                "Camera_Wide",
                "Camera_Medium",
                "Camera_Close",
            ),
        )

    def test_camera_specs_match_the_approved_plan(self) -> None:
        self.assertEqual(
            contract.CAMERA_SPECS,
            {
                "Camera_Wide": {
                    "location": (0.0, -2.74, 1.67),
                    "target": (0.0, 0.45, 0.85),
                    "lens": 18.0,
                    "focus": "IP_Focus_Head",
                },
                "Camera_Medium": {
                    "location": (0.0, -2.15, 1.78),
                    "target": (0.0, 0.30, 1.85),
                    "lens": 50.0,
                    "focus": "IP_Focus_Head",
                },
                "Camera_Close": {
                    "location": (0.08, -1.82, 1.92),
                    "target": (0.0, 0.30, 1.93),
                    "lens": 70.0,
                    "focus": "IP_Focus_Head",
                },
                "Camera_ThreeQuarter_Left": {
                    "location": (-2.25, -1.65, 1.82),
                    "target": (0.0, 0.30, 1.72),
                    "lens": 50.0,
                    "focus": "IP_Focus_Head",
                },
                "Camera_ThreeQuarter_Right": {
                    "location": (2.25, -1.65, 1.82),
                    "target": (0.0, 0.30, 1.72),
                    "lens": 50.0,
                    "focus": "IP_Focus_Head",
                },
                "Camera_Desk_Detail": {
                    "location": (1.95, -2.07, 1.68),
                    "target": (0.55, -0.25, 0.98),
                    "lens": 85.0,
                    "focus": "IP_Focus_Desk",
                },
                "Camera_Shelf_Detail": {
                    "location": (1.35, -0.35, 1.95),
                    "target": (1.45, 2.50, 1.95),
                    "lens": 85.0,
                    "focus": "IP_Focus_Shelf",
                },
            },
        )

    def test_camera_specs_are_deeply_read_only(self) -> None:
        with self.assertRaises(TypeError):
            contract.CAMERA_SPECS["Camera_Wide"]["lens"] = 35.0

        with self.assertRaises(TypeError):
            contract.CAMERA_SPECS["Camera_Extra"] = {}

    def test_floor_plants_are_strict_mirrors(self) -> None:
        transforms = contract.floor_plant_transforms()
        self.assertIsInstance(transforms, tuple)
        self.assertEqual(len(transforms), 2)

        left_spec, right_spec = transforms
        self.assertEqual(left_spec, (-2.15, 0.65, 0.0, 0.18))
        self.assertEqual(right_spec, (2.15, 0.65, 0.0, -0.18))
        self.assertEqual(left_spec[0], -right_spec[0])
        self.assertEqual(left_spec[1:3], right_spec[1:3])
        self.assertEqual(left_spec[3], -right_spec[3])

    def test_floor_plants_share_the_approved_link_key(self) -> None:
        self.assertEqual(contract.PLANT_LINK_KEY, "WarmStudio_FloorPlant_LinkedData")

    def test_repo_root_resolves_from_the_contract_module(self) -> None:
        root = contract.repo_root()
        self.assertEqual(root, Path(contract.__file__).resolve().parents[2])
        self.assertTrue((root / "mcp/ip_avatar_3d/warm_studio_contract.py").is_file())
        self.assertTrue(
            (
                root
                / "docs/superpowers/specs/2026-07-20-default-sloth-aroll-assets-design.md"
            ).is_file()
        )


if __name__ == "__main__":
    unittest.main()
