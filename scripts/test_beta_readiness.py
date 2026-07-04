import unittest
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))
import beta_readiness


def base_snapshot():
    return {
        "smoke": {"passed": True},
        "localAgent": {"status": "ok"},
        "hyperframes": {"status": "ok"},
        "ffmpeg": {"available": True},
        "diagnostics": {"available": True},
        "qaFixture": {"passed": True, "structuredShotReports": 2, "repairPlanAvailable": True},
        "mcpProviders": [
            {
                "id": "jimeng",
                "status": "ok",
                "capabilities": ["aigc_video", "aigc_image"],
                "tools": ["jimeng.generate_video", "jimeng.generate_image"],
            }
        ],
        "modelProviders": {
            "text_to_text": {"configured": True},
            "text_to_video": {"configured": True},
        },
    }


class BetaReadinessTest(unittest.TestCase):
    def test_go_when_real_aigc_services_and_gates_are_ready(self):
        result = beta_readiness.evaluate(base_snapshot(), require_real_aigc=True)

        self.assertEqual(result["decision"], "GO")
        self.assertEqual(result["capabilities"]["realAigc"], True)
        self.assertEqual(result["capabilities"]["oneSentenceHighQuality"], True)
        self.assertEqual(result["blockingIssues"], [])

    def test_fallback_only_is_conditional_not_high_quality_go(self):
        snapshot = base_snapshot()
        snapshot["mcpProviders"] = []

        result = beta_readiness.evaluate(snapshot)

        self.assertEqual(result["decision"], "CONDITIONAL")
        self.assertEqual(result["capabilities"]["fallbackPreview"], True)
        self.assertEqual(result["capabilities"]["realAigc"], False)
        self.assertEqual(result["capabilities"]["oneSentenceHighQuality"], False)
        self.assertTrue(any("real AIGC" in item for item in result["warnings"]))

    def test_health_ok_boolean_is_accepted(self):
        snapshot = base_snapshot()
        snapshot["hyperframes"] = {"ok": True}

        result = beta_readiness.evaluate(snapshot, require_real_aigc=True)

        self.assertEqual(result["decision"], "GO")
        self.assertEqual(result["capabilities"]["hyperframes"], True)

    def test_real_aigc_required_blocks_without_healthy_provider(self):
        snapshot = base_snapshot()
        snapshot["mcpProviders"] = [{"id": "jimeng", "status": "unhealthy", "capabilities": ["aigc_video"]}]

        result = beta_readiness.evaluate(snapshot, require_real_aigc=True)

        self.assertEqual(result["decision"], "BLOCKED")
        self.assertTrue(any("healthy AIGC video provider" in item for item in result["blockingIssues"]))
        self.assertTrue(any("Configure and preflight" in item for item in result["nextActions"]))

    def test_missing_core_gates_blocks_beta(self):
        snapshot = base_snapshot()
        snapshot["smoke"] = {"passed": False}
        snapshot["diagnostics"] = {"available": False}
        snapshot["qaFixture"] = {"passed": True, "structuredShotReports": 0, "repairPlanAvailable": False}

        result = beta_readiness.evaluate(snapshot)

        self.assertEqual(result["decision"], "BLOCKED")
        self.assertTrue(any("beta smoke" in item for item in result["blockingIssues"]))
        self.assertTrue(any("diagnostics" in item for item in result["blockingIssues"]))
        self.assertTrue(any("structured shot QA" in item for item in result["blockingIssues"]))


if __name__ == "__main__":
    unittest.main()
