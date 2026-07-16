#!/usr/bin/env python3
"""Contract tests for the formal front-facing AIOS talking-head demo."""

from __future__ import annotations

import unittest

import front_talking_demo as demo


class FrontTalkingDemoTests(unittest.TestCase):
    def test_demo_spec_is_publishable_front_talking_aroll(self) -> None:
        spec = demo.DEMO_SPEC

        self.assertGreaterEqual(spec["durationSec"], 15.0)
        self.assertLessEqual(spec["durationSec"], 30.0)
        self.assertEqual(spec["resolution"], {"width": 1920, "height": 1080})
        self.assertEqual(spec["fps"], 30)
        self.assertEqual(spec["profileId"], "talking_head")
        self.assertEqual(spec["presentationMode"], "standing")
        self.assertEqual(spec["cameraPreset"], "front_talking")
        self.assertTrue(spec["actionSequence"])
        self.assertFalse(
            any("Transition" in action for action in spec["actionSequence"])
        )

        broll_duration = sum(
            item["endSec"] - item["startSec"] for item in spec["brollWindows"]
        )
        self.assertLessEqual(broll_duration / spec["durationSec"], 0.20)
        self.assertEqual(demo.validate_demo_spec(spec), [])

    def test_agent_run_payload_carries_ip_shooting_contract(self) -> None:
        payload = demo.build_agent_run_payload("project-front-demo")

        self.assertEqual(payload["domain"], "video_creation")
        self.assertEqual(payload["mode"], "dynamic_agent")
        context = payload["context"]
        self.assertEqual(context["projectId"], "project-front-demo")
        self.assertEqual(context["profileId"], "talking_head")
        self.assertEqual(context["cameraPreset"], "front_talking")
        self.assertEqual(context["aigcProvider"], "disabled")
        self.assertEqual(context["presentationMode"], "standing")
        self.assertEqual(context["actionSequence"], demo.DEMO_SPEC["actionSequence"])
        self.assertEqual(context["targetDurationSec"], demo.DEMO_SPEC["durationSec"])

    def test_project_payload_uses_supported_provider_generation_mode(self) -> None:
        payload = demo.build_project_payload("/tmp/front-demo-project")

        self.assertEqual(payload["mode"], "voice_visual")
        self.assertEqual(payload["generationMode"], "provider_api")
        self.assertEqual(payload["targetDurationSec"], 20)
        self.assertEqual(payload["localPathHint"], "/tmp/front-demo-project")
        self.assertEqual(payload["config"]["cameraPreset"], "front_talking")

    def test_media_probe_validates_publishable_delivery_contract(self) -> None:
        report = demo.validate_media_probe(
            {
                "streams": [
                    {
                        "codec_type": "video",
                        "codec_name": "h264",
                        "width": 1920,
                        "height": 1080,
                        "r_frame_rate": "30/1",
                        "avg_frame_rate": "30/1",
                    },
                    {
                        "codec_type": "audio",
                        "codec_name": "aac",
                        "sample_rate": "48000",
                        "channels": 2,
                    },
                ],
                "format": {"duration": "20.032"},
            }
        )

        self.assertTrue(report["passed"])
        self.assertEqual(report["errors"], [])
        self.assertEqual(report["metrics"]["fps"], 30.0)
        self.assertTrue(report["metrics"]["constantFrameRate"])

    def test_media_probe_rejects_wrong_resolution_and_missing_audio(self) -> None:
        report = demo.validate_media_probe(
            {
                "streams": [
                    {
                        "codec_type": "video",
                        "codec_name": "vp9",
                        "width": 1280,
                        "height": 720,
                        "r_frame_rate": "24/1",
                        "avg_frame_rate": "23/1",
                    }
                ],
                "format": {"duration": "8"},
            }
        )

        self.assertFalse(report["passed"])
        self.assertIn("video must be 1920x1080", report["errors"])
        self.assertIn("audio codec must be AAC", report["errors"])
        self.assertIn("video must use constant 30 fps", report["errors"])


if __name__ == "__main__":
    unittest.main()
