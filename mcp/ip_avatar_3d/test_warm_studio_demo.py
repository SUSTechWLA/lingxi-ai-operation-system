#!/usr/bin/env python3
"""Contract tests for the dual-mode warm studio demo runner."""

from __future__ import annotations

import json
import os
import tempfile
import unittest
from pathlib import Path

import render_warm_studio_demo as demo


REPO_ROOT = Path(__file__).resolve().parents[2]
PROFILE_PATH = REPO_ROOT / "ip形象/main_ip/character-profile.json"

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
    "maxJawRadians": 0.20484000444412231,
    "mbpJawRadians": 0.0,
}


def write_lighting_evidence(root: Path) -> Path:
    path = root / "lighting-evidence.json"
    path.write_text(
        json.dumps(
            {
                "schemaVersion": "tangying-warm-studio-lighting-evidence/v1",
                "success": True,
                "errors": [],
                "cameraComparisons": [
                    {"mode": mode, "cameraRoleA": "medium", "cameraRoleB": camera_role}
                    for mode in ("standing", "seated")
                    for camera_role in ("three_quarter", "wide")
                ],
                "measurements": [
                    {
                        "mode": mode,
                        "engine": engine,
                        "cameraRole": "medium",
                        "backgroundStopsBelowFace": 1.2,
                        "highlightClipRatio": 0.0,
                        "brightNeutralRedBlueRatio": 1.1,
                        "brightNeutralRedGreenRatio": 1.05,
                    }
                    for mode in ("standing", "seated")
                    for engine in ("eevee", "cycles")
                ],
            }
        )
    )
    return path


def write_rig_report(
    path: Path,
    *,
    transition_status: str = "not_applicable",
    transition_success: bool | None = None,
    viseme_success: bool = True,
) -> None:
    physical = transition_status == "passed"
    motion_events = (
        [
            {
                "timeSec": 0.0,
                "motion": "avatar_action",
                "action": "Aroll_Transition_StandToSit",
                "duration": 1.0,
                "startState": "standing",
                "endState": "seated",
            }
        ]
        if physical
        else [
            {
                "timeSec": 0.0,
                "motion": "avatar_action",
                "action": "Aroll_Transition_Reset",
                "duration": 1.0,
                "startState": "standing",
                "endState": "standing",
            }
        ]
    )
    state_timeline = (
        [
            {"timeSec": 0.0, "state": "standing"},
            {"timeSec": 1.0, "state": "seated"},
        ]
        if physical
        else [
            {"timeSec": 0.0, "state": "standing"},
            {"timeSec": 1.0, "state": "standing"},
        ]
    )
    sampled_frames = [
        {"frame": 1, "timeSec": 0.0, "state": "standing"},
        {"frame": 3, "timeSec": 1.0, "state": "seated" if physical else "standing"},
    ]
    sample_cadence = {
        "fps": 2,
        "frameStart": 1,
        "frameEnd": 4,
        "animationFrameCount": 4,
        "performanceSampleIntervalFrames": 2,
        "performanceSampleCount": 2,
        "contactSampleIntervalFrames": 1,
        "contactSampleCount": 4 if physical else 0,
    }
    jaw_samples = [
        {"frame": 1, "viseme": "mbp", "jawRadians": 0.0},
        {"frame": 2, "viseme": "a", "jawRadians": 0.20484000444412231},
        {"frame": 3, "viseme": "a", "jawRadians": 0.20484000444412231},
        {"frame": 4, "viseme": "closed", "jawRadians": 0.0},
    ]
    path.write_text(
        json.dumps(
            {
                "success": True,
                "scene": {},
                "arollPerformanceQa": {
                    "schemaVersion": "tangying-aroll-performance-qa/v1",
                    "transition": {
                        "status": transition_status,
                        "success": transition_success,
                        "errors": [],
                        "metrics": dict(VALID_TRANSITION_METRICS) if physical else {},
                        "evidence": {
                            **sample_cadence,
                            "physicalTransitionActions": (
                                ["Aroll_Transition_StandToSit"] if physical else []
                            ),
                        },
                    },
                    "visemes": {
                        "status": "passed" if viseme_success else "failed",
                        "success": viseme_success,
                        "errors": [] if viseme_success else ["injected viseme failure"],
                        "metrics": dict(VALID_VISEME_METRICS),
                        "characterDimensions": {"height": 2.55, "width": 1.42},
                        "evidence": {
                            "configuredJawResponse": {"Mouth_A": 0.36},
                            "evaluatedJawTimeline": {
                                "success": True,
                                "errors": [],
                                "maxJawRadians": 0.20484000444412231,
                                "mbpJawRadians": 0.0,
                                "sampleCount": 4,
                                "mbpSampleCount": 1,
                                "sampledFrames": [1, 2, 3, 4],
                                "samples": jaw_samples,
                            },
                        },
                    },
                    "sampledFrames": sampled_frames,
                    "stateTimeline": state_timeline,
                    "sampleCadence": sample_cadence,
                    "motionEvents": motion_events,
                },
            }
        )
    )


class WarmStudioDemoTests(unittest.TestCase):
    def test_missing_lighting_evidence_fails_before_render(self) -> None:
        calls = 0

        def unexpected_renderer(**_kwargs: object) -> dict[str, object]:
            nonlocal calls
            calls += 1
            return {}

        with tempfile.TemporaryDirectory() as temp_dir:
            with self.assertRaises(demo.DemoQAError):
                demo.render_warm_studio_demos(
                    PROFILE_PATH,
                    Path(temp_dir),
                    renderer=unexpected_renderer,
                )
            self.assertEqual(calls, 0)

    def test_orchestrates_exactly_two_locked_production_renders(self) -> None:
        calls: list[dict[str, object]] = []

        def fake_renderer(**kwargs: object) -> dict[str, object]:
            calls.append(kwargs)
            mode = str(kwargs["presentationMode"])
            mode_dir = Path(str(kwargs["outputDir"]))
            mode_dir.mkdir(parents=True, exist_ok=True)
            video = mode_dir / "ip_layer.mp4"
            video.write_bytes(f"video-{mode}".encode())
            rig_report = mode_dir / "rig_report.json"
            write_rig_report(rig_report)
            return {
                "success": True,
                "status": "ready",
                "presentationMode": mode,
                "videoPath": str(video),
                "rigReportPath": str(rig_report),
                "voice": {
                    "provider": "gpt_sovits_local",
                    "voiceId": "main_ip_warm_knowledge_host_v1",
                    "productionReady": True,
                },
                "qa": {
                    "finalAudioLoudness": {
                        "integratedLufs": -16.0,
                        "truePeakDbtp": -1.5,
                    }
                },
            }

        def fake_probe(_path: Path) -> dict[str, object]:
            return {
                "videoExists": True,
                "width": 1920,
                "height": 1080,
                "fps": 30.0,
                "durationSec": 8.0,
                "constantFrameRate": True,
            }

        def fake_artifact_builder(
            _records: list[dict[str, object]], paths: dict[str, Path]
        ) -> None:
            for key in ("reel", "standingContactSheet", "seatedContactSheet", "lightingComparison"):
                paths[key].write_bytes(key.encode())

        with tempfile.TemporaryDirectory() as temp_dir:
            lighting_evidence = write_lighting_evidence(Path(temp_dir))
            result = demo.render_warm_studio_demos(
                PROFILE_PATH,
                Path(temp_dir),
                renderer=fake_renderer,
                media_probe=fake_probe,
                artifact_builder=fake_artifact_builder,
                lighting_evidence_path=lighting_evidence,
            )

            self.assertTrue(result["success"])
            self.assertEqual([call["presentationMode"] for call in calls], ["standing", "seated"])
            for call in calls:
                self.assertEqual(call["width"], 1920)
                self.assertEqual(call["height"], 1080)
                self.assertEqual(call["fps"], 30)
                self.assertEqual(call["qualityPreset"], "production_1080p")
                self.assertEqual(call["renderMode"], "production")
                self.assertEqual(call["voiceProvider"], "gpt_sovits_local")
                self.assertEqual(call["voiceId"], "main_ip_warm_knowledge_host_v1")
                self.assertEqual(call["fallbackPolicy"], "error")
            for filename in demo.FINAL_FILENAMES.values():
                self.assertTrue((Path(temp_dir) / filename).is_file(), filename)
            report_text = (Path(temp_dir) / demo.FINAL_FILENAMES["report"]).read_text()
            report = json.loads(report_text)
            self.assertNotIn(".warm-studio-demo-", report_text)
            self.assertTrue(report["lightingEvidence"]["available"])
            self.assertEqual(len(report["lightingEvidence"]["measurements"]), 4)
            self.assertTrue(report["modes"]["standing"]["collisionReport"]["report"]["success"])
            self.assertTrue(report["modes"]["seated"]["collisionReport"]["report"]["success"])

    def test_failed_qa_never_publishes_final_outputs(self) -> None:
        calls = 0

        def fake_renderer(**kwargs: object) -> dict[str, object]:
            nonlocal calls
            calls += 1
            mode_dir = Path(str(kwargs["outputDir"]))
            mode_dir.mkdir(parents=True, exist_ok=True)
            video = mode_dir / "ip_layer.mp4"
            video.write_bytes(b"staged-video")
            rig_report = mode_dir / "rig_report.json"
            write_rig_report(rig_report)
            return {
                "success": True,
                "status": "ready",
                "presentationMode": kwargs["presentationMode"],
                "videoPath": str(video),
                "rigReportPath": str(rig_report),
                "voice": {
                    "provider": "gpt_sovits_local",
                    "voiceId": "main_ip_warm_knowledge_host_v1",
                    "productionReady": True,
                },
                "qa": {
                    "finalAudioLoudness": {
                        "integratedLufs": -16.0,
                        "truePeakDbtp": -1.5,
                    }
                },
            }

        def failing_probe(_path: Path) -> dict[str, object]:
            return {
                "videoExists": True,
                "width": 1920,
                "height": 1080,
                "fps": 24.0,
                "durationSec": 8.0,
                "constantFrameRate": True,
            }

        with tempfile.TemporaryDirectory() as temp_dir:
            output_dir = Path(temp_dir)
            lighting_evidence = write_lighting_evidence(output_dir)
            with self.assertRaises(demo.DemoQAError):
                demo.render_warm_studio_demos(
                    PROFILE_PATH,
                    output_dir,
                    renderer=fake_renderer,
                    media_probe=failing_probe,
                    lighting_evidence_path=lighting_evidence,
                )

            self.assertEqual(calls, 1)
            for filename in demo.FINAL_FILENAMES.values():
                self.assertFalse((output_dir / filename).exists(), filename)

    def test_mid_publication_failure_rolls_back_every_final_output(self) -> None:
        def fake_renderer(**kwargs: object) -> dict[str, object]:
            mode = str(kwargs["presentationMode"])
            mode_dir = Path(str(kwargs["outputDir"]))
            mode_dir.mkdir(parents=True, exist_ok=True)
            video = mode_dir / "ip_layer.mp4"
            video.write_bytes(f"video-{mode}".encode())
            rig_report = mode_dir / "rig_report.json"
            write_rig_report(rig_report)
            return {
                "success": True,
                "status": "ready",
                "presentationMode": mode,
                "videoPath": str(video),
                "rigReportPath": str(rig_report),
                "voice": {
                    "provider": "gpt_sovits_local",
                    "voiceId": "main_ip_warm_knowledge_host_v1",
                    "productionReady": True,
                },
                "qa": {
                    "finalAudioLoudness": {
                        "integratedLufs": -16.0,
                        "truePeakDbtp": -1.5,
                    }
                },
            }

        def fake_probe(_path: Path) -> dict[str, object]:
            return {
                "videoExists": True,
                "width": 1920,
                "height": 1080,
                "fps": 30.0,
                "durationSec": 8.0,
                "constantFrameRate": True,
            }

        def fake_artifact_builder(
            _records: list[dict[str, object]], paths: dict[str, Path]
        ) -> None:
            for key in ("reel", "standingContactSheet", "seatedContactSheet", "lightingComparison"):
                paths[key].write_bytes(key.encode())

        replace_calls = 0

        def failing_publisher(paths: dict[str, Path], output_dir: Path) -> None:
            nonlocal replace_calls

            def fail_once(source: Path, destination: Path) -> None:
                nonlocal replace_calls
                replace_calls += 1
                if replace_calls == 3:
                    raise OSError("injected publication interruption")
                os.replace(source, destination)

            demo.publish_transaction(paths, output_dir, replace_file=fail_once)

        with tempfile.TemporaryDirectory() as temp_dir:
            output_dir = Path(temp_dir)
            lighting_evidence = write_lighting_evidence(output_dir)
            previous_outputs = {
                filename: f"previous-{key}".encode()
                for key, filename in demo.FINAL_FILENAMES.items()
            }
            for filename, payload in previous_outputs.items():
                (output_dir / filename).write_bytes(payload)
            with self.assertRaises(demo.DemoQAError):
                demo.render_warm_studio_demos(
                    PROFILE_PATH,
                    output_dir,
                    renderer=fake_renderer,
                    media_probe=fake_probe,
                    artifact_builder=fake_artifact_builder,
                    lighting_evidence_path=lighting_evidence,
                    publisher=failing_publisher,
                )

            self.assertEqual(replace_calls, 3)
            for filename, payload in previous_outputs.items():
                self.assertEqual((output_dir / filename).read_bytes(), payload, filename)

    def test_embedded_report_rejects_missing_performance_qa(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            report = Path(temp_dir) / "rig_report.json"
            report.write_text(json.dumps({"success": True}))
            with self.assertRaisesRegex(demo.DemoQAError, "transition geometry QA"):
                demo._embedded_collision_report(
                    "standing",
                    {"rigReportPath": str(report)},
                )

    def test_embedded_report_rejects_failed_or_inconsistent_transition_qa(self) -> None:
        cases = (("failed", False), ("passed", False), ("", True))
        for status, success in cases:
            with self.subTest(status=status, success=success):
                with tempfile.TemporaryDirectory() as temp_dir:
                    report = Path(temp_dir) / "rig_report.json"
                    write_rig_report(
                        report,
                        transition_status=status,
                        transition_success=success,
                    )
                    with self.assertRaisesRegex(
                        demo.DemoQAError,
                        "transition geometry QA",
                    ):
                        demo._embedded_collision_report(
                            "standing",
                            {"rigReportPath": str(report)},
                        )

    def test_embedded_report_accepts_only_passed_true_or_not_applicable(self) -> None:
        cases = (("passed", True), ("not_applicable", None))
        for status, success in cases:
            with self.subTest(status=status, success=success):
                with tempfile.TemporaryDirectory() as temp_dir:
                    report = Path(temp_dir) / "rig_report.json"
                    write_rig_report(
                        report,
                        transition_status=status,
                        transition_success=success,
                    )
                    embedded = demo._embedded_collision_report(
                        "standing",
                        {"rigReportPath": str(report)},
                    )
                    self.assertEqual(
                        embedded["report"]["arollPerformanceQa"]["transition"]["status"],
                        status,
                    )

    def test_embedded_report_accepts_reset_mixed_with_a_physical_transition(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            report = Path(temp_dir) / "rig_report.json"
            write_rig_report(
                report,
                transition_status="passed",
                transition_success=True,
            )
            payload = json.loads(report.read_text())
            performance = payload["arollPerformanceQa"]
            performance["motionEvents"].append(
                {
                    "timeSec": 1.1,
                    "motion": "avatar_action",
                    "action": "Aroll_Transition_Reset",
                    "duration": 0.1,
                    "startState": "seated",
                    "endState": "seated",
                }
            )
            performance["stateTimeline"].append(
                {"timeSec": 1.1 + 0.1, "state": "seated"}
            )
            report.write_text(json.dumps(payload))
            embedded = demo._embedded_collision_report(
                "standing",
                {"rigReportPath": str(report)},
            )
            self.assertEqual(
                embedded["report"]["arollPerformanceQa"]["transition"]["status"],
                "passed",
            )

    def test_embedded_report_rejects_failed_or_malformed_viseme_qa(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            report = Path(temp_dir) / "rig_report.json"
            write_rig_report(report, viseme_success=False)
            with self.assertRaisesRegex(demo.DemoQAError, "viseme QA"):
                demo._embedded_collision_report(
                    "seated",
                    {"rigReportPath": str(report)},
                )

            payload = json.loads(report.read_text())
            payload["arollPerformanceQa"]["visemes"] = "malformed"
            report.write_text(json.dumps(payload))
            with self.assertRaisesRegex(demo.DemoQAError, "viseme QA"):
                demo._embedded_collision_report(
                    "seated",
                    {"rigReportPath": str(report)},
                )

    def test_embedded_report_recomputes_transition_metrics(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            report = Path(temp_dir) / "rig_report.json"
            write_rig_report(
                report,
                transition_status="passed",
                transition_success=True,
            )
            payload = json.loads(report.read_text())
            payload["arollPerformanceQa"]["transition"]["metrics"][
                "maxCentralSilhouetteSpike"
            ] = 9.0
            report.write_text(json.dumps(payload))
            with self.assertRaisesRegex(demo.DemoQAError, "transition geometry QA"):
                demo._embedded_collision_report(
                    "standing",
                    {"rigReportPath": str(report)},
                )

    def test_embedded_report_recomputes_visemes_and_requires_passed_status(self) -> None:
        mutations = (
            lambda visemes: visemes.update({"status": "failed", "success": True, "errors": []}),
            lambda visemes: visemes.update({"status": "passed", "success": True, "metrics": {}}),
        )
        for mutate in mutations:
            with self.subTest(mutation=mutate):
                with tempfile.TemporaryDirectory() as temp_dir:
                    report = Path(temp_dir) / "rig_report.json"
                    write_rig_report(report)
                    payload = json.loads(report.read_text())
                    mutate(payload["arollPerformanceQa"]["visemes"])
                    report.write_text(json.dumps(payload))
                    with self.assertRaisesRegex(demo.DemoQAError, "viseme QA"):
                        demo._embedded_collision_report(
                            "standing",
                            {"rigReportPath": str(report)},
                        )

    def test_embedded_report_rejects_configured_gain_without_animated_jaw(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            report = Path(temp_dir) / "rig_report.json"
            write_rig_report(report)
            payload = json.loads(report.read_text())
            timeline = payload["arollPerformanceQa"]["visemes"]["evidence"][
                "evaluatedJawTimeline"
            ]
            for sample in timeline["samples"]:
                sample["jawRadians"] = 0.0
            report.write_text(json.dumps(payload))
            with self.assertRaisesRegex(demo.DemoQAError, "viseme QA"):
                demo._embedded_collision_report(
                    "standing",
                    {"rigReportPath": str(report)},
                )

    def test_embedded_report_rejects_empty_transition_metrics(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            report = Path(temp_dir) / "rig_report.json"
            write_rig_report(
                report,
                transition_status="passed",
                transition_success=True,
            )
            payload = json.loads(report.read_text())
            payload["arollPerformanceQa"]["transition"]["metrics"] = {}
            report.write_text(json.dumps(payload))
            with self.assertRaisesRegex(demo.DemoQAError, "transition geometry QA"):
                demo._embedded_collision_report(
                    "standing",
                    {"rigReportPath": str(report)},
                )

    def test_embedded_report_rejects_malformed_sample_cadence(self) -> None:
        mutations = (
            lambda performance: performance["sampleCadence"].update(
                {"performanceSampleIntervalFrames": 3}
            ),
            lambda performance: performance["sampleCadence"].update(
                {"performanceSampleCount": 99}
            ),
            lambda performance: performance["sampledFrames"][1].update({"frame": 4}),
            lambda performance: performance["transition"]["evidence"].update(
                {"contactSampleCount": 3}
            ),
        )
        for mutate in mutations:
            with self.subTest(mutation=mutate):
                with tempfile.TemporaryDirectory() as temp_dir:
                    report = Path(temp_dir) / "rig_report.json"
                    write_rig_report(
                        report,
                        transition_status="passed",
                        transition_success=True,
                    )
                    payload = json.loads(report.read_text())
                    mutate(payload["arollPerformanceQa"])
                    report.write_text(json.dumps(payload))
                    with self.assertRaisesRegex(demo.DemoQAError, "transition geometry QA"):
                        demo._embedded_collision_report(
                            "standing",
                            {"rigReportPath": str(report)},
                        )

    def test_embedded_report_rejects_inconsistent_success_and_empty_timeline(self) -> None:
        mutations = (
            lambda payload: payload["arollPerformanceQa"]["transition"].update(
                {"status": "passed", "success": True, "errors": ["inconsistent"]}
            ),
            lambda payload: payload["arollPerformanceQa"]["visemes"].update(
                {"success": True, "errors": ["inconsistent"]}
            ),
            lambda payload: payload["arollPerformanceQa"].update(
                {"sampledFrames": []}
            ),
            lambda payload: payload["arollPerformanceQa"].update(
                {"stateTimeline": []}
            ),
            lambda payload: payload["arollPerformanceQa"].update(
                {"schemaVersion": "fabricated/v9"}
            ),
            lambda payload: payload["arollPerformanceQa"]["sampledFrames"][0].update(
                {"state": "seated"}
            ),
            lambda payload: payload["arollPerformanceQa"].pop("sampleCadence"),
        )
        for mutate in mutations:
            with self.subTest(mutation=mutate):
                with tempfile.TemporaryDirectory() as temp_dir:
                    report = Path(temp_dir) / "rig_report.json"
                    write_rig_report(report)
                    payload = json.loads(report.read_text())
                    mutate(payload)
                    report.write_text(json.dumps(payload))
                    with self.assertRaises(demo.DemoQAError):
                        demo._embedded_collision_report(
                            "standing",
                            {"rigReportPath": str(report)},
                        )

if __name__ == "__main__":
    unittest.main()
