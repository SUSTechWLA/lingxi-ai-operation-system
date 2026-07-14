import unittest

import aroll_actions


class ArollActionContractTests(unittest.TestCase):
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

    def test_transition_specs_end_in_mirrored_seated_pose(self) -> None:
        specs = aroll_actions.build_aroll_action_specs(True, 30)
        seated = aroll_actions.presentation_pose("seated", True)
        self.assertEqual(specs["Aroll_Transition_StandToSit"][-1][1], seated)
        self.assertEqual(specs["Aroll_Transition_SitToStand"][-1][1], {})
        self.assertEqual(seated["leg_l"]["rotation"], (0.02, -0.08, 1.16))
        self.assertEqual(seated["leg_r"]["rotation"], (0.02, 0.08, -1.16))


if __name__ == "__main__":
    unittest.main()
