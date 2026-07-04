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
        self.assertEqual(reports[0]["mode"], "cinematic")
        self.assertEqual(reports[0]["artifactRefs"], [])
        self.assertEqual(reports[0]["timestamps"]["sampledTimesSec"], [2])

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

    def test_repair_plan_maps_reference_and_fallback_to_generation_actions(self) -> None:
        server = load_server()

        reference_lints = server.build_shot_spec_lints(
            [
                {
                    "id": "SHOT_REF",
                    "durationSec": 6,
                    "visual": "角色在明亮导演台前打开参考图墙",
                    "narrationText": "先锁定角色和场景，再生成镜头。",
                    "whyThisShot": "这个镜头解释参考资产对 AIGC 一致性的价值。",
                    "plannedAssetRoute": "aigc_video",
                    "actionBeats": ["0-2秒打开参考图墙", "2-4秒角色和道具亮起", "4-6秒镜头稳定收束"],
                }
            ]
        )
        self.assertEqual(reference_lints[0]["recommendedAction"], "REGEN_AIGC_WITH_REFERENCE")

        reports = server.build_shot_reports(
            "vp_test",
            "cinematic_story",
            "candidate_01",
            [
                {
                    "shotId": "SHOT_REF",
                    "frameCount": 1,
                    "sampledTimesSec": [4],
                    "passed": True,
                    "needsRegeneration": False,
                    "score": 96,
                    "blockingIssueCount": 0,
                    "warningIssueCount": 0,
                    "metricSummary": {},
                    "representativeIssues": [],
                    "artifactRefs": [{"artifactId": "fallback-1", "sourceType": "fallback_storyboard", "isFallback": True}],
                    "sourceType": "fallback_storyboard",
                    "isFallback": True,
                    "fallbackReason": "no_ready_aigc_video",
                }
            ],
            reference_lints,
        )

        self.assertEqual(reports[0]["decision"], "REGEN_AIGC_WITH_REFERENCE")
        self.assertEqual(reports[0]["repairPlan"]["action"], "REGEN_AIGC_WITH_REFERENCE")
        self.assertTrue(reports[0]["repairPlan"]["toolOverrides"]["requiresReferenceAssets"])
        self.assertEqual(reports[0]["artifactRefs"][0]["sourceType"], "fallback_storyboard")
        self.assertTrue(reports[0]["timestamps"]["generatedAt"])
        aggregate = server._repair_plan_from_summaries(
            [{"shotId": "SHOT_REF", "decision": reports[0]["decision"], "repairAction": reports[0]["repairPlan"]["action"]}]
        )
        self.assertEqual(aggregate["nextAction"], "regenerate_shots")
        self.assertEqual(aggregate["regenerateShotIds"], ["SHOT_REF"])

    def test_prompt_risk_and_severe_render_failures_are_machine_actionable(self) -> None:
        server = load_server()

        lints = server.build_shot_spec_lints(
            [
                {
                    "id": "SHOT_PROMPT",
                    "durationSec": 5,
                    "visual": "AIGC_VIDEO b-roll uses ffmpeg simple_cut artifact",
                    "narrationText": "这段提示词不该泄漏内部术语。",
                    "whyThisShot": "检查投放提示词安全性。",
                    "referenceAssetIds": ["ref-1"],
                    "plannedAssetRoute": "aigc_video",
                    "actionBeats": ["0-2秒主体出现", "2-4秒道具亮起"],
                }
            ]
        )
        self.assertEqual(lints[0]["recommendedAction"], "REVISE_SHOT_SPEC")
        self.assertEqual(lints[0]["repairActionHint"], "PROMPT_PATCH_REGEN")

        prompt_reports = server.build_shot_reports(
            "vp_test",
            "hybrid",
            "candidate_02",
            [{"shotId": "SHOT_PROMPT", "score": 90, "blockingIssueCount": 0, "warningIssueCount": 0, "metricSummary": {}, "representativeIssues": []}],
            lints,
        )
        self.assertEqual(prompt_reports[0]["decision"], "REVISE_SHOT_SPEC")
        self.assertEqual(prompt_reports[0]["repairPlan"]["action"], "PROMPT_PATCH_REGEN")

        severe_reports = server.build_shot_reports(
            "vp_test",
            "talking_head",
            "candidate_03",
            [
                {
                    "shotId": "SHOT_FAIL",
                    "score": 12,
                    "blockingIssueCount": 3,
                    "warningIssueCount": 1,
                    "metricSummary": {},
                    "representativeIssues": [{"code": "render_output_missing", "severity": "blocking"}],
                    "renderError": "ffmpeg exited with status 1",
                }
            ],
            [],
        )
        self.assertEqual(severe_reports[0]["decision"], "HUMAN_REVIEW")
        self.assertEqual(severe_reports[0]["repairPlan"]["action"], "HUMAN_REVIEW")
        self.assertEqual(severe_reports[0]["mode"], "talking_head")


if __name__ == "__main__":
    unittest.main()
