#!/usr/bin/env python3
from __future__ import annotations

import importlib.util
import pathlib
import unittest


def load_server():
    path = pathlib.Path(__file__).with_name("server.py")
    spec = importlib.util.spec_from_file_location("video_qa_server", path)
    module = importlib.util.module_from_spec(spec)
    assert spec and spec.loader
    spec.loader.exec_module(module)
    return module


class VideoQAServerTests(unittest.TestCase):
    def test_shot_spec_lint_flags_duration_and_exact_text(self) -> None:
        server = load_server()

        lints = server.build_shot_spec_lints(
            [
                {
                    "id": "SHOT_LONG",
                    "durationSec": 18,
                    "screenText": "这个 shot 文案非常长，应该在生成前拆分或改成更短的画面文字。",
                },
                {
                    "id": "SHOT_EXACT",
                    "durationSec": 6,
                    "visualPlan": {"textLayers": [{"id": "title", "text": "AI 视频生产闭环", "mustBeExact": True}]},
                },
            ]
        )

        self.assertEqual(lints[0]["recommendedAction"], "REVISE_SHOT_SPEC")
        self.assertTrue(lints[0]["fatalGateTriggered"])
        self.assertEqual(lints[1]["renderStrategyHint"], "html_overlay")
        self.assertEqual(lints[1]["recommendedAction"], "RERENDER_HTML")
        self.assertIn("must_be_exact_text_requires_html_overlay", lints[1]["generationWarnings"])

    def test_shot_report_maps_text_gate_to_rerender_html(self) -> None:
        server = load_server()

        summaries = [
            {
                "shotId": "SHOT_TEXT",
                "frameCount": 1,
                "sampledTimesSec": [2],
                "passed": False,
                "needsRegeneration": True,
                "score": 82,
                "blockingIssueCount": 1,
                "warningIssueCount": 0,
                "metricSummary": {
                    "maxTopLeftTextZoneEdgeDensity": 0.13,
                    "maxLowerThirdEdgeDensity": 0.02,
                    "maxFullFrameEdgeDensity": 0.04,
                },
                "representativeIssues": [
                    {
                        "code": "top_left_text_zone_crowded",
                        "severity": "blocking",
                        "zone": "top_left",
                        "message": "左上文字安全区过于拥挤。",
                        "suggestion": "减少左上角叠字。",
                        "value": 0.13,
                        "threshold": 0.11,
                    }
                ],
            }
        ]

        reports = server.build_shot_reports("vp_test", "aigc_shot", "candidate_01", summaries, [])

        self.assertEqual(reports[0]["decision"], "RERENDER_HTML")
        self.assertTrue(reports[0]["fatalGateTriggered"])
        self.assertEqual(reports[0]["repairPlan"]["action"], "RERENDER_HTML")
        self.assertTrue(reports[0]["repairPlan"]["toolOverrides"]["textOverlayNeeded"])
        self.assertIn("no embedded text", reports[0]["repairPlan"]["promptPatch"]["negativeAdditions"])

    def test_cinematic_shot_spec_lint_scores_script_alignment_inputs(self) -> None:
        server = load_server()

        lints = server.build_shot_spec_lints(
            [
                {
                    "id": "SHOT_CINE",
                    "durationSec": 6,
                    "visual": "角色在导演台前把任务卡排成队",
                    "narrationText": "混乱需求终于排队了。",
                    "plannedAssetRoute": "aigc_video",
                }
            ]
        )

        self.assertIn("director_reason_missing", lints[0]["generationWarnings"])
        self.assertIn("reference_assets_missing", lints[0]["generationWarnings"])
        self.assertLess(lints[0]["scriptVisualCompletenessScore"], 100)

        reports = server.build_shot_reports(
            "vp_test",
            "cinematic_story",
            "candidate_01",
            [
                {
                    "shotId": "SHOT_CINE",
                    "frameCount": 1,
                    "sampledTimesSec": [2],
                    "passed": True,
                    "needsRegeneration": False,
                    "score": 100,
                    "blockingIssueCount": 0,
                    "warningIssueCount": 0,
                    "metricSummary": {
                        "maxTopLeftTextZoneEdgeDensity": 0.01,
                        "maxLowerThirdEdgeDensity": 0.02,
                        "maxFullFrameEdgeDensity": 0.04,
                    },
                    "representativeIssues": [],
                }
            ],
            lints,
        )

        self.assertIn("scriptAlignment", reports[0])
        self.assertIn("whyThisShot/dramaticPurpose", reports[0]["scriptAlignment"]["missing"])


if __name__ == "__main__":
    unittest.main()
