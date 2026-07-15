#!/usr/bin/env python3
"""Pure contract tests for fail-closed A-roll performance QA."""

from __future__ import annotations

import math
import unittest

import aroll_performance_qa as qa


VALID_TRANSITION_METRICS = {
    "maxFootDriftL": 0.012,
    "maxFootDriftR": 0.014,
    "minKneeSeparation": 0.18,
    "minSeatClearance": -0.010,
    "maxSettledSeatClearance": 0.022,
    "maxRootFrameDelta": 0.052,
    "maxCentralSilhouetteSpike": 0.016,
    "seatVisibleFraction": 0.14,
}

VALID_VISEME_METRICS = {
    "mbpGap": 0.002,
    "restGap": 0.004,
    "aGap": 0.028,
    "eWidth": 0.110,
    "oGap": 0.022,
    "oWidth": 0.074,
    "uGap": 0.016,
    "surpriseGap": 0.034,
    "maxJawRadians": 0.232,
    "mbpJawRadians": 0.0,
}


class ArollPerformanceQATests(unittest.TestCase):
    def test_valid_transition_metrics_pass(self) -> None:
        report = qa.validate_transition_metrics(VALID_TRANSITION_METRICS)
        self.assertTrue(report["success"])
        self.assertEqual(report["status"], "passed")
        self.assertEqual(report["errors"], [])

    def test_transition_threshold_boundaries_are_inclusive(self) -> None:
        report = qa.validate_transition_metrics(
            {
                "maxFootDriftL": 0.025,
                "maxFootDriftR": 0.025,
                "minKneeSeparation": 0.075,
                "minSeatClearance": -0.018,
                "maxSettledSeatClearance": 0.035,
                "maxRootFrameDelta": 0.075,
                "maxCentralSilhouetteSpike": 0.045,
                "seatVisibleFraction": 0.08,
            }
        )
        self.assertTrue(report["success"], report)
        upper_visibility = dict(VALID_TRANSITION_METRICS, seatVisibleFraction=0.28)
        self.assertTrue(qa.validate_transition_metrics(upper_visibility)["success"])

    def test_each_transition_threshold_fails_closed(self) -> None:
        failures = {
            "maxFootDriftL": 0.025001,
            "maxFootDriftR": 0.025001,
            "minKneeSeparation": 0.074999,
            "minSeatClearance": -0.018001,
            "maxSettledSeatClearance": 0.035001,
            "maxRootFrameDelta": 0.075001,
            "maxCentralSilhouetteSpike": 0.045001,
            "seatVisibleFraction": 0.079999,
        }
        for key, value in failures.items():
            with self.subTest(key=key):
                report = qa.validate_transition_metrics(
                    dict(VALID_TRANSITION_METRICS, **{key: value})
                )
                self.assertFalse(report["success"], report)
                self.assertEqual(report["status"], "failed")
                self.assertTrue(report["errors"])
        self.assertFalse(
            qa.validate_transition_metrics(
                dict(VALID_TRANSITION_METRICS, seatVisibleFraction=0.280001)
            )["success"]
        )

    def test_transition_thresholds_reject_the_next_float_outside_each_bound(self) -> None:
        limits = qa.TRANSITION_LIMITS
        failures = {
            "maxFootDriftL": math.nextafter(limits["maxFootDrift"], math.inf),
            "maxFootDriftR": math.nextafter(limits["maxFootDrift"], math.inf),
            "minKneeSeparation": math.nextafter(limits["minKneeSeparation"], -math.inf),
            "minSeatClearance": math.nextafter(limits["minSeatClearance"], -math.inf),
            "maxSettledSeatClearance": math.nextafter(
                limits["maxSettledSeatClearance"], math.inf
            ),
            "maxRootFrameDelta": math.nextafter(limits["maxRootFrameDelta"], math.inf),
            "maxCentralSilhouetteSpike": math.nextafter(
                limits["maxCentralSilhouetteSpike"], math.inf
            ),
            "seatVisibleFraction": math.nextafter(
                limits["minSeatVisibleFraction"], -math.inf
            ),
        }
        for key, value in failures.items():
            with self.subTest(key=key, value=value):
                report = qa.validate_transition_metrics(
                    dict(VALID_TRANSITION_METRICS, **{key: value})
                )
                self.assertFalse(report["success"], report)
        report = qa.validate_transition_metrics(
            dict(
                VALID_TRANSITION_METRICS,
                seatVisibleFraction=math.nextafter(
                    limits["maxSeatVisibleFraction"], math.inf
                ),
            )
        )
        self.assertFalse(report["success"], report)

    def test_pixel_line_geometry_and_hidden_stool_fail(self) -> None:
        report = qa.validate_transition_metrics(
            {
                "maxFootDriftL": 0.010,
                "maxFootDriftR": 0.010,
                "minKneeSeparation": 0.015,
                "minSeatClearance": -0.010,
                "maxSettledSeatClearance": 0.020,
                "maxRootFrameDelta": 0.030,
                "maxCentralSilhouetteSpike": 0.081,
                "seatVisibleFraction": 0.0,
            }
        )
        self.assertFalse(report["success"])
        self.assertIn("central silhouette spike", " ".join(report["errors"]))
        self.assertIn("seat visibility", " ".join(report["errors"]))

    def test_missing_and_malformed_transition_metrics_return_errors(self) -> None:
        for metrics in (
            {},
            dict(VALID_TRANSITION_METRICS, maxFootDriftL="not-a-number"),
            dict(VALID_TRANSITION_METRICS, maxRootFrameDelta=math.inf),
            dict(VALID_TRANSITION_METRICS, seatVisibleFraction=True),
        ):
            with self.subTest(metrics=metrics):
                report = qa.validate_transition_metrics(metrics)
                self.assertFalse(report["success"])
                self.assertEqual(report["status"], "failed")
                self.assertTrue(report["errors"])

    def test_transition_metrics_reject_booleans_and_nonfinite_numbers(self) -> None:
        for value in (True, False, math.nan, math.inf, -math.inf):
            with self.subTest(value=value):
                report = qa.validate_transition_metrics(
                    dict(VALID_TRANSITION_METRICS, maxRootFrameDelta=value)
                )
                self.assertFalse(report["success"], report)

    def test_every_transition_metric_is_required(self) -> None:
        for name in qa.TRANSITION_METRIC_NAMES:
            with self.subTest(name=name):
                metrics = dict(VALID_TRANSITION_METRICS)
                del metrics[name]
                report = qa.validate_transition_metrics(metrics)
                self.assertFalse(report["success"], report)
                self.assertIn(f"missing required metric: {name}", report["errors"])

    def test_transition_metrics_reject_numeric_strings(self) -> None:
        for name in qa.TRANSITION_METRIC_NAMES:
            with self.subTest(name=name):
                metrics = dict(VALID_TRANSITION_METRICS)
                metrics[name] = str(metrics[name])
                report = qa.validate_transition_metrics(metrics)
                self.assertFalse(report["success"], report)
                self.assertIn(f"{name} must be a finite number", report["errors"])

    def test_viseme_separation_gate(self) -> None:
        report = qa.validate_viseme_metrics(
            VALID_VISEME_METRICS,
            character_height=2.55,
            character_width=1.42,
        )
        self.assertTrue(report["success"], report)
        self.assertEqual(report["status"], "passed")

    def test_viseme_task6_boundaries_are_exact(self) -> None:
        height = 2.0
        width = 1.0
        metrics = {
            "mbpGap": 0.01,
            "restGap": 0.007,
            "aGap": 0.022,
            "eWidth": math.nextafter(0.112, math.inf),
            "oGap": 0.0196,
            "oWidth": 0.100,
            "uGap": 0.0172,
            "surpriseGap": 0.024,
            "maxJawRadians": 0.20,
            "mbpJawRadians": 0.03,
        }
        self.assertTrue(
            qa.validate_viseme_metrics(
                metrics,
                character_height=height,
                character_width=width,
            )["success"]
        )
        metrics["maxJawRadians"] = 0.25
        self.assertTrue(
            qa.validate_viseme_metrics(
                metrics,
                character_height=height,
                character_width=width,
            )["success"]
        )

    def test_viseme_thresholds_reject_the_next_float_outside_each_bound(self) -> None:
        height = 2.0
        width = 1.0
        boundary = {
            "mbpGap": 0.01,
            "restGap": 0.007,
            "aGap": 0.022,
            "eWidth": math.nextafter(0.112, math.inf),
            "oGap": 0.0196,
            "oWidth": 0.100,
            "uGap": 0.0172,
            "surpriseGap": 0.024,
            "maxJawRadians": 0.20,
            "mbpJawRadians": 0.03,
        }
        failures = {
            "mbpGap": math.nextafter(boundary["mbpGap"], math.inf),
            "aGap": math.nextafter(boundary["aGap"], -math.inf),
            "oGap": math.nextafter(boundary["oGap"], -math.inf),
            "uGap": math.nextafter(boundary["uGap"], -math.inf),
            "surpriseGap": math.nextafter(boundary["surpriseGap"], -math.inf),
            "eWidth": math.nextafter(boundary["eWidth"], boundary["oWidth"]),
            "maxJawRadians": math.nextafter(0.20, -math.inf),
            "mbpJawRadians": math.nextafter(0.03, math.inf),
        }
        for name, value in failures.items():
            with self.subTest(name=name, value=value):
                metrics = dict(boundary, **{name: value})
                report = qa.validate_viseme_metrics(
                    metrics,
                    character_height=height,
                    character_width=width,
                )
                self.assertFalse(report["success"], report)
        report = qa.validate_viseme_metrics(
            dict(boundary, maxJawRadians=math.nextafter(0.25, math.inf)),
            character_height=height,
            character_width=width,
        )
        self.assertFalse(report["success"], report)

    def test_each_viseme_separation_and_jaw_bound_fails(self) -> None:
        failures = {
            "mbpGap": 0.008,
            "aGap": 0.017,
            "oGap": 0.013,
            "uGap": 0.010,
            "surpriseGap": 0.029,
            "eWidth": 0.080,
            "maxJawRadians": 0.199,
        }
        for key, value in failures.items():
            with self.subTest(key=key):
                report = qa.validate_viseme_metrics(
                    dict(VALID_VISEME_METRICS, **{key: value}),
                    character_height=2.55,
                    character_width=1.42,
                )
                self.assertFalse(report["success"], report)
                self.assertTrue(report["errors"])
        self.assertFalse(
            qa.validate_viseme_metrics(
                dict(VALID_VISEME_METRICS, maxJawRadians=0.251),
                character_height=2.55,
                character_width=1.42,
            )["success"]
        )
        self.assertFalse(
            qa.validate_viseme_metrics(
                dict(VALID_VISEME_METRICS, mbpJawRadians=0.031),
                character_height=2.55,
                character_width=1.42,
            )["success"]
        )

    def test_missing_and_malformed_viseme_metrics_return_errors(self) -> None:
        for metrics, height, width in (
            ({}, 2.55, 1.42),
            (dict(VALID_VISEME_METRICS, aGap=None), 2.55, 1.42),
            (VALID_VISEME_METRICS, 0.0, 1.42),
            (VALID_VISEME_METRICS, 2.55, math.nan),
        ):
            with self.subTest(metrics=metrics, height=height, width=width):
                report = qa.validate_viseme_metrics(
                    metrics,
                    character_height=height,
                    character_width=width,
                )
                self.assertFalse(report["success"])
                self.assertTrue(report["errors"])

    def test_viseme_metrics_reject_booleans_and_nonfinite_numbers(self) -> None:
        for value in (True, False, math.nan, math.inf, -math.inf):
            with self.subTest(value=value):
                report = qa.validate_viseme_metrics(
                    dict(VALID_VISEME_METRICS, maxJawRadians=value),
                    character_height=2.55,
                    character_width=1.42,
                )
                self.assertFalse(report["success"], report)

    def test_every_viseme_metric_is_required(self) -> None:
        for name in qa.VISEME_METRIC_NAMES:
            with self.subTest(name=name):
                metrics = dict(VALID_VISEME_METRICS)
                del metrics[name]
                report = qa.validate_viseme_metrics(
                    metrics,
                    character_height=2.55,
                    character_width=1.42,
                )
                self.assertFalse(report["success"], report)
                self.assertIn(f"missing required metric: {name}", report["errors"])

    def test_viseme_metrics_and_dimensions_reject_numeric_strings(self) -> None:
        for name in qa.VISEME_METRIC_NAMES:
            with self.subTest(name=name):
                metrics = dict(VALID_VISEME_METRICS)
                metrics[name] = str(metrics[name])
                report = qa.validate_viseme_metrics(
                    metrics,
                    character_height=2.55,
                    character_width=1.42,
                )
                self.assertFalse(report["success"], report)
                self.assertIn(f"{name} must be a finite number", report["errors"])
        for height, width in (("2.55", 1.42), (2.55, "1.42")):
            with self.subTest(height=height, width=width):
                report = qa.validate_viseme_metrics(
                    VALID_VISEME_METRICS,
                    character_height=height,
                    character_width=width,
                )
                self.assertFalse(report["success"], report)

    def test_zero_jaw_animation_fails_even_when_configured_gain_is_valid(self) -> None:
        jaw = qa.summarize_jaw_samples(
            [
                {"frame": 1, "viseme": "mbp", "jawRadians": 0.0},
                {
                    "frame": 2,
                    "viseme": "a",
                    "jawRadians": 0.0,
                    "configuredJawGain": 0.36,
                },
            ]
        )
        self.assertTrue(jaw["success"], jaw)
        self.assertEqual(jaw["maxJawRadians"], 0.0)
        self.assertEqual(jaw["mbpJawRadians"], 0.0)
        report = qa.validate_viseme_metrics(
            dict(
                VALID_VISEME_METRICS,
                maxJawRadians=jaw["maxJawRadians"],
                mbpJawRadians=jaw["mbpJawRadians"],
            ),
            character_height=2.55,
            character_width=1.42,
        )
        self.assertFalse(report["success"], report)

    def test_central_silhouette_uses_64_column_closest_depth(self) -> None:
        points = []
        for column in range(64):
            depth = 2.0
            if column in (30, 31, 32, 33):
                depth = 1.96
            points.append(((column + 0.5) / 64.0, depth))
            points.append(((column + 0.5) / 64.0, depth + 0.5))
        evidence = qa.detect_central_silhouette_spike(points)
        self.assertTrue(evidence["success"], evidence)
        self.assertEqual(evidence["columnCount"], 64)
        self.assertEqual(evidence["centerColumns"], [30, 31, 32, 33])
        self.assertEqual(evidence["flankColumns"], [29, 34])
        self.assertAlmostEqual(evidence["spikeMeters"], 0.04)

    def test_central_silhouette_missing_columns_fails_closed(self) -> None:
        evidence = qa.detect_central_silhouette_spike([(0.1, 2.0), (0.9, 2.0)])
        self.assertFalse(evidence["success"])
        self.assertTrue(evidence["errors"])

    def test_central_silhouette_can_normalize_an_off_center_subject(self) -> None:
        points = []
        for column in range(64):
            depth = 1.96 if column in (30, 31, 32, 33) else 2.0
            normalized_x = (column + 0.5) / 64.0
            points.append((0.28 + normalized_x * 0.22, depth))

        evidence = qa.detect_central_silhouette_spike(
            points,
            normalize_subject_x=True,
        )

        self.assertTrue(evidence["success"], evidence)
        self.assertTrue(evidence["subjectXNormalized"])
        self.assertAlmostEqual(evidence["subjectMinX"], points[0][0])
        self.assertAlmostEqual(evidence["subjectMaxX"], points[-1][0])
        self.assertAlmostEqual(evidence["spikeMeters"], 0.04)

    def test_central_silhouette_requires_both_flanks_behind_the_center(self) -> None:
        points = []
        for column in range(64):
            depth = 2.0
            if column in (30, 31, 32, 33):
                depth = 1.96
            elif column == 29:
                depth = 1.96
            elif column == 34:
                depth = 2.50
            points.append(((column + 0.5) / 64.0, depth))

        evidence = qa.detect_central_silhouette_spike(points)

        self.assertTrue(evidence["success"], evidence)
        self.assertAlmostEqual(evidence["spikeMeters"], 0.0)

    def test_central_silhouette_requires_a_continuous_center_spike(self) -> None:
        points = []
        for column in range(64):
            depth = 2.0
            if column == 30:
                depth = 1.80
            points.append(((column + 0.5) / 64.0, depth))

        evidence = qa.detect_central_silhouette_spike(points)

        self.assertTrue(evidence["success"], evidence)
        self.assertAlmostEqual(evidence["centerClosestDepth"], 1.80)
        self.assertAlmostEqual(evidence["centerConservativeDepth"], 2.0)
        self.assertAlmostEqual(evidence["spikeMeters"], 0.0)

    def test_seat_visibility_raster_respects_character_occlusion(self) -> None:
        seat = [
            ((0.0, 0.0, 2.0), (1.0, 0.0, 2.0), (1.0, 1.0, 2.0)),
            ((0.0, 0.0, 2.0), (1.0, 1.0, 2.0), (0.0, 1.0, 2.0)),
        ]
        character = [
            ((0.0, 0.0, 1.0), (0.5, 0.0, 1.0), (0.5, 1.0, 1.0)),
            ((0.0, 0.0, 1.0), (0.5, 1.0, 1.0), (0.0, 1.0, 1.0)),
        ]
        evidence = qa.rasterize_seat_visibility(
            character,
            seat,
            width=40,
            height=20,
        )
        self.assertTrue(evidence["success"], evidence)
        self.assertEqual(evidence["foregroundPixelCount"], 800)
        self.assertEqual(evidence["seatVisiblePixelCount"], 400)
        self.assertAlmostEqual(evidence["seatVisibleFraction"], 0.5)

    def test_empty_seat_or_foreground_mask_fails_closed(self) -> None:
        for character, seat in (([], []), ([((0, 0, 1), (1, 0, 1), (0, 1, 1))], [])):
            with self.subTest(character=character, seat=seat):
                evidence = qa.rasterize_seat_visibility(character, seat)
                self.assertFalse(evidence["success"])
                self.assertTrue(evidence["errors"])

    def test_character_wins_equal_depth_mask_ties(self) -> None:
        square = [
            ((0.0, 0.0, 1.0), (1.0, 0.0, 1.0), (1.0, 1.0, 1.0)),
            ((0.0, 0.0, 1.0), (1.0, 1.0, 1.0), (0.0, 1.0, 1.0)),
        ]
        evidence = qa.rasterize_seat_visibility(
            square,
            square,
            width=10,
            height=10,
        )
        self.assertTrue(evidence["success"], evidence)
        self.assertEqual(evidence["seatVisiblePixelCount"], 0)
        self.assertEqual(evidence["foregroundPixelCount"], 100)

    def test_seat_depth_is_perspective_correct_under_screen_barycentrics(self) -> None:
        triangle = ((0.0, 0.0, 2.0), (2.0, 0.0, 8.0 / 3.0), (0.0, 2.0, 8.0 / 3.0))
        character = [
            ((0.0, 0.0, 2.3), (2.0, 0.0, 2.3), (0.0, 2.0, 2.3)),
        ]
        evidence = qa.rasterize_seat_visibility(
            character,
            [triangle],
            width=1,
            height=1,
        )
        self.assertTrue(evidence["success"], evidence)
        self.assertEqual(evidence["seatVisiblePixelCount"], 1)
        self.assertAlmostEqual(evidence["minimumVisibleDepthMeters"], 16.0 / 7.0)

    def test_nonpositive_triangle_depth_fails_closed(self) -> None:
        valid = ((0.0, 0.0, 2.0), (1.0, 0.0, 2.0), (0.0, 1.0, 2.0))
        invalid = ((0.0, 0.0, 0.0), (1.0, 0.0, 2.0), (0.0, 1.0, 2.0))
        evidence = qa.rasterize_seat_visibility([valid], [valid, invalid], width=4, height=4)
        self.assertFalse(evidence["success"], evidence)
        self.assertIn("depth must be greater than zero", " ".join(evidence["errors"]))

    def test_nontransition_report_is_explicitly_not_applicable(self) -> None:
        report = qa.not_applicable_transition_report()
        self.assertEqual(report["status"], "not_applicable")
        self.assertIsNone(report["success"])
        self.assertEqual(report["metrics"], {})

    def test_only_state_changing_physical_actions_require_transition_qa(self) -> None:
        reset = {
            "motion": "avatar_action",
            "action": "Aroll_Transition_Reset",
            "startState": "either",
            "endState": "either",
        }
        stand_to_sit = {
            "motion": "avatar_action",
            "action": "Aroll_Transition_StandToSit",
            "startState": "standing",
            "endState": "seated",
        }
        self.assertEqual(qa.physical_transition_events([reset]), [])
        self.assertEqual(qa.physical_transition_events([reset, stand_to_sit]), [stand_to_sit])


if __name__ == "__main__":
    unittest.main()
