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
            "eWidth": 0.112,
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

    def test_nontransition_report_is_explicitly_not_applicable(self) -> None:
        report = qa.not_applicable_transition_report()
        self.assertEqual(report["status"], "not_applicable")
        self.assertIsNone(report["success"])
        self.assertEqual(report["metrics"], {})


if __name__ == "__main__":
    unittest.main()
