#!/usr/bin/env python3
"""Contract tests for the dual-mode warm studio demo runner."""

from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

import render_warm_studio_demo as demo


REPO_ROOT = Path(__file__).resolve().parents[2]
PROFILE_PATH = REPO_ROOT / "ip形象/main_ip/character-profile.json"


class WarmStudioDemoTests(unittest.TestCase):
    def test_orchestrates_exactly_two_locked_production_renders(self) -> None:
        calls: list[dict[str, object]] = []

        def fake_renderer(**kwargs: object) -> dict[str, object]:
            calls.append(kwargs)
            mode = str(kwargs["presentationMode"])
            mode_dir = Path(str(kwargs["outputDir"]))
            mode_dir.mkdir(parents=True, exist_ok=True)
            video = mode_dir / "ip_layer.mp4"
            video.write_bytes(f"video-{mode}".encode())
            return {
                "success": True,
                "status": "ready",
                "presentationMode": mode,
                "videoPath": str(video),
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
            result = demo.render_warm_studio_demos(
                PROFILE_PATH,
                Path(temp_dir),
                renderer=fake_renderer,
                media_probe=fake_probe,
                artifact_builder=fake_artifact_builder,
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

    def test_failed_qa_never_publishes_final_outputs(self) -> None:
        calls = 0

        def fake_renderer(**kwargs: object) -> dict[str, object]:
            nonlocal calls
            calls += 1
            mode_dir = Path(str(kwargs["outputDir"]))
            mode_dir.mkdir(parents=True, exist_ok=True)
            video = mode_dir / "ip_layer.mp4"
            video.write_bytes(b"staged-video")
            return {
                "success": True,
                "status": "ready",
                "presentationMode": kwargs["presentationMode"],
                "videoPath": str(video),
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
            with self.assertRaises(demo.DemoQAError):
                demo.render_warm_studio_demos(
                    PROFILE_PATH,
                    output_dir,
                    renderer=fake_renderer,
                    media_probe=failing_probe,
                )

            self.assertEqual(calls, 1)
            for filename in demo.FINAL_FILENAMES.values():
                self.assertFalse((output_dir / filename).exists(), filename)


if __name__ == "__main__":
    unittest.main()
