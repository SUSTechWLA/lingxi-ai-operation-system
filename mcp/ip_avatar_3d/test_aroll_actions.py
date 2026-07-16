import unittest

import aroll_actions


class ArollActionContractTests(unittest.TestCase):
    def test_catalog_actions_have_complete_pose_specs(self) -> None:
        specs = aroll_actions.build_aroll_action_specs(source_rig=True, fps=30)
        for name, metadata in aroll_actions.ACTION_CATALOG.items():
            with self.subTest(action=name):
                self.assertIn(name, specs)
                self.assertGreaterEqual(len(specs[name]), 5)
                frames = [frame for frame, _ in specs[name]]
                self.assertEqual(frames, sorted(frames))
                self.assertEqual(len(frames), len(set(frames)))
                if metadata.end_state == "seated":
                    seated = aroll_actions.presentation_pose("seated", True)
                    for role, transform in seated.items():
                        self.assertEqual(specs[name][-1][1].get(role), transform)

        expected_holds = {
            "Aroll_Welcome_OpenArms": {"__digit_pose_l": "open_hand", "__digit_pose_r": "open_hand"},
            "Aroll_Question_PalmUp": {"__digit_pose_r": "open_hand"},
            "Aroll_Compare_TwoSides": {"__digit_pose_l": "open_hand"},
            "Aroll_KeyPoint_OneFinger": {"__digit_pose_r": "count_one"},
            "Aroll_List_Three": {"__digit_pose_r": "count_three"},
            "Aroll_Caution_Stop": {"__digit_pose_r": "stop"},
            "Aroll_Quote_Frame": {"__digit_pose_l": "count_two", "__digit_pose_r": "count_two"},
            "Aroll_Conclusion_HandsTogether": {"__digit_pose_l": "relaxed_hand", "__digit_pose_r": "relaxed_hand"},
            "Aroll_Seated_Explain": {"__digit_pose_l": "open_hand", "__digit_pose_r": "open_hand"},
            "Aroll_Seated_OpenPalm": {"__digit_pose_r": "open_hand"},
        }
        for name, expected in expected_holds.items():
            with self.subTest(action=name):
                hold = specs[name][2][1]
                self.assertEqual({key: hold.get(key) for key in expected}, expected)

    def test_rich_source_holds_are_distinct_and_seated_amplitude_is_restrained(self) -> None:
        specs = aroll_actions.build_aroll_action_specs(source_rig=True, fps=30)
        comparison = specs["Aroll_Compare_TwoSides"]
        relaxed = aroll_actions.relaxed_upper_body_pose(True)
        self.assertIn("upper_arm_l", comparison[2][1])
        self.assertEqual(comparison[2][1]["upper_arm_r"], relaxed["upper_arm_r"])
        self.assertIn("upper_arm_r", comparison[3][1])
        self.assertEqual(comparison[3][1]["upper_arm_l"], relaxed["upper_arm_l"])

        seated_hold = specs["Aroll_Seated_OpenPalm"][2][1]
        standing_hold = specs["Aroll_Question_PalmUp"][2][1]
        for role in ("upper_arm_r", "forearm_r", "hand_r"):
            self.assertEqual(
                seated_hold[role],
                tuple(value * 0.82 for value in standing_hold[role]),
            )

        seated = aroll_actions.presentation_pose("seated", True)
        lean_spec = specs["Aroll_Seated_LeanIn"]
        for _, pose in lean_spec:
            self.assertLessEqual(abs(pose["body"]["rotation"][0] - seated["body"]["rotation"][0]), 0.10)
            self.assertLessEqual(abs(pose["root"]["location"][1] - seated["root"]["location"][1]), 0.035)

    def test_open_point_count_and_stop_digit_poses_are_semantically_distinct(self) -> None:
        pose_names = ("open_hand", "point", "count_three", "stop")

        def maximum_delta(left: str, right: str) -> float:
            deltas = []
            for digit in (1, 2, 3):
                first = aroll_actions.hand_pose(left)[digit]
                second = aroll_actions.hand_pose(right)[digit]
                deltas.extend(
                    abs(getattr(first, field) - getattr(second, field))
                    for field in ("proximal", "middle", "distal", "splay", "opposition")
                )
            return max(deltas)

        for index, left in enumerate(pose_names):
            for right in pose_names[index + 1 :]:
                with self.subTest(left=left, right=right):
                    self.assertGreaterEqual(maximum_delta(left, right), 0.08)

    def test_catalog_has_unique_stateful_actions(self) -> None:
        payload = aroll_actions.action_catalog_payload()
        names = [item["name"] for item in payload["actions"]]
        self.assertEqual(len(names), len(set(names)))
        self.assertTrue(
            {
                "Aroll_Standing_Idle",
                "Aroll_Seated_Idle",
                "Aroll_Transition_StandToSit",
                "Aroll_Transition_SitToStand",
                "Aroll_Welcome_OpenArms",
                "Aroll_Question_PalmUp",
                "Aroll_Compare_TwoSides",
                "Aroll_KeyPoint_OneFinger",
                "Aroll_List_Three",
                "Aroll_Caution_Stop",
                "Aroll_Quote_Frame",
                "Aroll_Conclusion_HandsTogether",
                "Aroll_Seated_Explain",
                "Aroll_Seated_OpenPalm",
                "Aroll_Seated_LeanIn",
            }.issubset(names)
        )
        for item in payload["actions"]:
            self.assertIn(item["startState"], {"standing", "seated", "either"})
            self.assertIn(item["endState"], {"standing", "seated", "either"})
            self.assertGreater(item["durationSec"], 0.0)
            self.assertTrue(item["channels"])

    def test_sequence_inserts_required_transitions(self) -> None:
        resolved = aroll_actions.resolve_action_sequence(
            ["Aroll_Welcome_OpenArms", "Aroll_Seated_Explain", "Aroll_Conclusion_HandsTogether"],
            initial_state="standing",
        )
        self.assertEqual(
            resolved,
            [
                "Aroll_Welcome_OpenArms",
                "Aroll_Transition_StandToSit",
                "Aroll_Seated_Explain",
                "Aroll_Transition_SitToStand",
                "Aroll_Conclusion_HandsTogether",
            ],
        )

    def test_invalid_action_and_state_fail_closed(self) -> None:
        with self.assertRaisesRegex(ValueError, "unknown A-roll action"):
            aroll_actions.resolve_action_sequence(["Aroll_NotReal"], "standing")
        with self.assertRaisesRegex(ValueError, "initial state"):
            aroll_actions.resolve_action_sequence(["Aroll_Standing_Idle"], "crouching")

    def test_transition_events_are_contiguous_and_end_in_expected_state(self) -> None:
        events = aroll_actions.build_action_events(
            ["Aroll_Transition_StandToSit", "Aroll_Seated_Explain", "Aroll_Transition_SitToStand"],
            initial_state="standing",
            start_time_sec=1.0,
            spacing_sec=0.12,
        )
        self.assertEqual(events[0]["startState"], "standing")
        self.assertEqual(events[0]["endState"], "seated")
        self.assertEqual(events[-1]["endState"], "standing")
        self.assertEqual([event["motion"] for event in events], ["avatar_action"] * 3)
        self.assertTrue(all(events[i]["timeSec"] < events[i + 1]["timeSec"] for i in range(2)))

    def test_catalog_is_source_of_callable_actions_and_specs(self) -> None:
        self.assertEqual(aroll_actions.AROLL_ACTIONS, tuple(aroll_actions.ACTION_CATALOG))
        self.assertEqual(set(aroll_actions.build_aroll_action_specs(False, 30)), set(aroll_actions.ACTION_CATALOG))
        self.assertTrue(all(len(spec) >= 5 for spec in aroll_actions.build_aroll_action_specs(False, 30).values()))

    def test_catalog_and_specs_are_immutable_at_the_boundary(self) -> None:
        with self.assertRaises(TypeError):
            aroll_actions.ACTION_CATALOG["Aroll_NotReal"] = None
        self.assertIsInstance(aroll_actions.ACTION_CATALOG["Aroll_Standing_Idle"], aroll_actions.ActionMetadata)

    def test_blend_action_pose_interpolates_components_and_preserves_digit_pose(self) -> None:
        blended = aroll_actions.blend_action_pose(
            {"root": {"location": (0.0, 2.0, 4.0)}, "__digit_pose_r": "relaxed_hand"},
            {"root": {"location": (2.0, 4.0, 8.0)}, "body": {"rotation": (1.0, 2.0, 3.0)}, "__digit_pose_l": "open_hand"},
            0.5,
        )
        self.assertEqual(blended["root"]["location"], (1.0, 3.0, 6.0))
        self.assertEqual(blended["body"]["rotation"], (0.5, 1.0, 1.5))
        self.assertEqual(blended["__digit_pose_l"], "open_hand")
        self.assertEqual(blended["__digit_pose_r"], "relaxed_hand")

    def test_transition_specs_preserve_canonical_pose_at_state_boundaries(self) -> None:
        specs = aroll_actions.build_aroll_action_specs(True, 30)
        seated = aroll_actions.presentation_pose("seated", True)
        standing = aroll_actions.presentation_pose("standing", True)
        self.assertEqual(specs["Aroll_Transition_StandToSit"][0][1], standing)
        self.assertEqual(specs["Aroll_Transition_StandToSit"][-1][1], seated)
        self.assertEqual(specs["Aroll_Transition_SitToStand"][0][1], seated)
        self.assertEqual(specs["Aroll_Transition_SitToStand"][-1][1], standing)
        self.assertEqual(seated["leg_l"]["rotation"], (0.0, 0.0, 1.35))
        self.assertEqual(seated["leg_r"]["rotation"], (0.05, 0.197, -1.30))

    def test_upper_body_actions_never_override_standing_contact_channels(self) -> None:
        specs = aroll_actions.build_aroll_action_specs(True, 30)
        contact_roles = {
            "root",
            "leg_l",
            "shin_l",
            "foot_l",
            "leg_r",
            "shin_r",
            "foot_r",
        }
        for action_name in (
            "Aroll_Welcome_OpenArms",
            "Aroll_Conclusion_HandsTogether",
        ):
            with self.subTest(action=action_name):
                for _frame, pose in specs[action_name]:
                    self.assertTrue(contact_roles.isdisjoint(pose), pose)

    def test_non_transition_actions_keep_relaxed_arm_channels_at_every_key(self) -> None:
        specs = aroll_actions.build_aroll_action_specs(True, 30)
        relaxed = aroll_actions.relaxed_upper_body_pose(True)
        required_roles = {
            "upper_arm_l",
            "forearm_l",
            "hand_l",
            "upper_arm_r",
            "forearm_r",
            "hand_r",
        }
        for action_name, spec in specs.items():
            if action_name.startswith("Aroll_Transition_"):
                continue
            with self.subTest(action=action_name):
                for _frame, pose in spec:
                    self.assertTrue(required_roles.issubset(pose), pose)
                self.assertEqual(spec[0][1]["upper_arm_l"], relaxed["upper_arm_l"])
                self.assertEqual(spec[-1][1]["upper_arm_r"], relaxed["upper_arm_r"])

    def test_source_conclusion_gathers_both_hands_in_front_of_the_chest(self) -> None:
        hold = aroll_actions.build_aroll_action_specs(True, 30)[
            "Aroll_Conclusion_HandsTogether"
        ][2][1]
        self.assertLessEqual(abs(hold["upper_arm_l"][2]), 0.40)
        self.assertLessEqual(abs(hold["upper_arm_r"][2]), 0.40)
        self.assertLessEqual(hold["forearm_l"][0], -2.0)
        self.assertLessEqual(hold["forearm_r"][0], -2.0)

    def test_source_transition_uses_camera_safe_right_foot_profiles(self) -> None:
        canonical_standing = aroll_actions.presentation_pose("standing", True)
        canonical_seated = aroll_actions.presentation_pose("seated", True)
        standing = aroll_actions.transition_presentation_pose("standing", True)
        seated = aroll_actions.transition_presentation_pose("seated", True)
        specs = aroll_actions.build_transition_specs(True, 30)

        self.assertEqual(
            canonical_standing["foot_r"]["rotation"],
            (0.1542314, -0.0882571, -0.0093923),
        )
        self.assertEqual(
            canonical_seated["foot_r"]["rotation"],
            (-0.1510481, 0.1414181, 0.2348769),
        )
        self.assertSequenceEqual(
            standing["foot_r"]["rotation"],
            (0.1542314, 0.011742900000000001, 0.0306077),
        )
        self.assertSequenceEqual(
            seated["foot_r"]["rotation"],
            (-0.1510481, 0.2414181, 0.2748769),
        )
        self.assertEqual(specs["Aroll_Transition_StandToSit"][0][1], canonical_standing)
        self.assertEqual(specs["Aroll_Transition_StandToSit"][-1][1], canonical_seated)
        self.assertEqual(specs["Aroll_Transition_SitToStand"][0][1], canonical_seated)
        self.assertEqual(specs["Aroll_Transition_SitToStand"][-1][1], canonical_standing)
        interior_foot = specs["Aroll_Transition_StandToSit"][2][1]["foot_r"][
            "rotation"
        ]
        self.assertGreater(interior_foot[1], 0.08)
        self.assertGreater(interior_foot[2], 0.04)


if __name__ == "__main__":
    unittest.main()
