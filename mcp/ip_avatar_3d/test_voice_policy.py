#!/usr/bin/env python3
from __future__ import annotations

import unittest

from voice_policy import (
    PREVIEW_PROVIDERS,
    PRODUCTION_PROVIDERS,
    ProductionVoiceUnavailable,
    resolve_voice,
)


class VoicePolicyTests(unittest.TestCase):
    def test_provider_sets_are_explicit(self) -> None:
        self.assertEqual(PRODUCTION_PROVIDERS, frozenset({"heygen", "elevenlabs"}))
        self.assertEqual(PREVIEW_PROVIDERS, frozenset({"apple", "kokoro"}))

    def test_production_rejects_preview_voice(self) -> None:
        with self.assertRaises(ProductionVoiceUnavailable):
            resolve_voice(
                mode="production",
                provider="apple",
                voice_id="Eddy (中文（中国大陆）)",
                fallback_policy="error",
            )

    def test_production_requires_pinned_voice_id(self) -> None:
        with self.assertRaises(ProductionVoiceUnavailable):
            resolve_voice(
                mode="production",
                provider="heygen",
                voice_id="  ",
                fallback_policy="error",
            )

    def test_production_requires_error_fallback_policy(self) -> None:
        with self.assertRaises(ProductionVoiceUnavailable):
            resolve_voice(
                mode="production",
                provider="elevenlabs",
                voice_id="voice-123",
                fallback_policy="preview",
            )

    def test_production_pins_provider_and_voice(self) -> None:
        resolved = resolve_voice(
            mode="production",
            provider="heygen",
            voice_id="dMkR1XwIkarpNqWUJLnX",
            fallback_policy="error",
        )

        self.assertEqual(resolved.provider, "heygen")
        self.assertEqual(resolved.voice_id, "dMkR1XwIkarpNqWUJLnX")
        self.assertTrue(resolved.production_ready)
        self.assertFalse(resolved.allow_preview_fallback)

    def test_preview_providers_are_never_production_ready(self) -> None:
        for provider in PREVIEW_PROVIDERS:
            with self.subTest(provider=provider):
                resolved = resolve_voice(
                    mode="preview",
                    provider=provider,
                    voice_id="preview-voice",
                    fallback_policy="preview",
                    language="zh-CN",
                    speed=0.94,
                )

                self.assertFalse(resolved.production_ready)
                self.assertTrue(resolved.allow_preview_fallback)
                self.assertEqual(resolved.language, "zh-cn")
                self.assertEqual(resolved.speed, 0.94)

    def test_preview_accepts_auto_for_legacy_callers(self) -> None:
        resolved = resolve_voice(
            mode="preview",
            provider="auto",
            voice_id="",
            fallback_policy="preview",
        )

        self.assertEqual(resolved.provider, "auto")
        self.assertFalse(resolved.production_ready)
        self.assertTrue(resolved.allow_preview_fallback)

    def test_invalid_render_mode_is_rejected(self) -> None:
        with self.assertRaisesRegex(ValueError, "mode must be one of"):
            resolve_voice(
                mode="draft",
                provider="apple",
                voice_id="Eddy",
                fallback_policy="preview",
            )


if __name__ == "__main__":
    unittest.main()
