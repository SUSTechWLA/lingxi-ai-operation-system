"""Pure production and preview voice policy resolution."""

from __future__ import annotations

from dataclasses import dataclass


PRODUCTION_PROVIDERS = frozenset({"heygen", "elevenlabs", "gpt_sovits_local"})
PREVIEW_PROVIDERS = frozenset({"apple", "kokoro"})
_SUPPORTED_PROVIDERS = PRODUCTION_PROVIDERS | PREVIEW_PROVIDERS | {"auto"}


class ProductionVoiceUnavailable(RuntimeError):
    """Raised when an explicit production voice request is not publishable."""


@dataclass(frozen=True)
class ResolvedVoice:
    provider: str
    voice_id: str
    language: str
    speed: float
    production_ready: bool
    allow_preview_fallback: bool


def resolve_voice(
    *,
    mode: str,
    provider: str,
    voice_id: str,
    fallback_policy: str,
    language: str = "zh",
    speed: float = 1.0,
) -> ResolvedVoice:
    """Resolve a voice request without performing I/O or synthesis."""
    normalized_mode = str(mode or "").strip().lower()
    if normalized_mode not in {"production", "preview"}:
        raise ValueError("mode must be one of: preview, production")

    normalized_provider = str(provider or "auto").strip().lower()
    normalized_voice_id = str(voice_id or "").strip()
    normalized_fallback = str(fallback_policy or "").strip().lower()
    normalized_language = str(language or "zh").strip().lower()
    normalized_speed = max(0.7, min(float(speed or 1.0), 1.3))

    if normalized_mode == "production":
        if normalized_provider not in PRODUCTION_PROVIDERS:
            raise ProductionVoiceUnavailable(
                "production voice provider must be one of: elevenlabs, gpt_sovits_local, heygen"
            )
        if not normalized_voice_id:
            raise ProductionVoiceUnavailable("production voice ID must be pinned and nonempty")
        if normalized_fallback != "error":
            raise ProductionVoiceUnavailable('production fallbackPolicy must be "error"')
        return ResolvedVoice(
            provider=normalized_provider,
            voice_id=normalized_voice_id,
            language=normalized_language,
            speed=normalized_speed,
            production_ready=True,
            allow_preview_fallback=False,
        )

    if normalized_provider not in _SUPPORTED_PROVIDERS:
        raise ValueError(
            "preview voice provider must be one of: auto, apple, elevenlabs, gpt_sovits_local, heygen, kokoro"
        )
    return ResolvedVoice(
        provider=normalized_provider,
        voice_id=normalized_voice_id,
        language=normalized_language,
        speed=normalized_speed,
        production_ready=False,
        allow_preview_fallback=normalized_fallback != "error",
    )
