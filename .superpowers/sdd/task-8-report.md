# Task 8 Report: Fail-Closed Production Voice Policy

## Outcome

Task 8 adds a pure voice-policy boundary and applies it before audio
synthesis. Production renders now accept only `heygen` or `elevenlabs`, require
a pinned nonempty voice ID, and require `fallbackPolicy="error"`. Apple and
Kokoro remain preview providers and never report production readiness.

The main-IP profile is intentionally blocked for production until Task 9. It
declares HeyGen as the provider but leaves `voiceId` empty, retains the three
audition candidates, and keeps Apple Eddy as the explicit preview voice.

## Policy Contract

`mcp/ip_avatar_3d/voice_policy.py` defines the provider sets, immutable
`ResolvedVoice` value, and pure `resolve_voice` function. Explicit production
requests fail with `ProductionVoiceUnavailable` when the provider, voice ID,
or fallback policy is not publishable. Preview requests always resolve with
`production_ready=False`; `auto` remains available only for compatibility with
legacy preview callers.

## Server Integration

`render_talking_video` now accepts `renderMode` and `fallbackPolicy`. An omitted
render mode is treated as preview unless a character profile explicitly
declares a mode. A profile preview request selects the nested `voice.preview`
provider and voice instead of silently using or changing the production
selection.

The server resolves policy before `ensure_audio`. A blocked real render raises
before synthesis, while dry-run does not synthesize and reports a `voicePolicy`
object with `ready` or `blocked` status. The same object is written to render
input and report artifacts. After production synthesis, the server also checks
that the returned provider and voice ID still match the pinned request.

The review follow-up separates request intent from backend provenance.
`voicePolicy` and render input use `requestedProvider` and
`requestedVoiceId`. Shared audio-engine metadata keeps actual `provider` and
`voiceId` values derived only from backend `tts_provider` and `voice_id`;
request values are never substituted when backend metadata is absent.
Production succeeds only when both provenance fields are nonempty and exactly
match the resolved request, and the returned audio path is a real file.
Missing provenance, mismatch, missing audio, and explicit provider failures all
raise `ProductionVoiceUnavailable`.

Uploaded audio is unverified for this task. Production rejects `audioPath`
before probing or synthesis. Preview accepts it with `provider="uploaded"`, an
unknown actual voice ID, `humanVoiceProvider=false`, and
`productionReady=false`. Generic macOS preview fallback likewise leaves actual
voice provenance unknown instead of copying the requested voice name.

Apple keeps its existing synthesis and audio-processing path, but its metadata
now sets `humanVoiceProvider=false` and `productionReady=false` and no longer
claims a natural voice provider.

## Profile Contract

`ip形象/main_ip/character-profile.json` now declares:

- `renderMode: "production"`;
- `provider: "heygen"`;
- an empty production `voiceId`;
- `fallbackPolicy: "error"`;
- Apple Eddy under `preview`;
- the three Task 9 production voice candidates.

The existing language, speed, speaking rate, and persona remain unchanged.

## TDD Evidence

The pure policy RED run failed with `ModuleNotFoundError: No module named
'voice_policy'`. After the pure implementation, all 8 policy tests passed.

The server RED run executed 49 tests and produced one failure plus six errors:
the missing render parameters and exception export, absent dry-run policy
state, unchanged profile contract, and Apple still reporting production ready.
Two focused RED/GREEN cycles then proved that the legacy `voiceName` alias
cannot satisfy production's pinned `voiceId` requirement and that a synthesis
backend cannot replace the pinned production provider or voice. After
integration, the server suite runs 51 tests with the two existing Blender-only
tests skipped.

The review RED run expanded the server suite to 59 tests and failed on all
reported provenance gaps: request-to-actual substitution, missing backend
provenance acceptance, generic backend errors, production upload acceptance,
missing audio-path acceptance, and conflated metadata fields. A final focused
RED/GREEN cycle covered the generic macOS preview fallback's requested voice
substitution. The completed suite runs 60 tests with the two existing
Blender-only tests skipped.

## Verification

- `python3 mcp/ip_avatar_3d/test_voice_policy.py`: 8 tests passed.
- `python3 mcp/ip_avatar_3d/test_server.py`: 60 tests run, 2 skipped.
- `python3 -m py_compile` passed for both policy/server modules and tests.
- `python3 -m json.tool ip形象/main_ip/character-profile.json` passed.
- `git diff --cached --check` passed for the owned follow-up scope.

## Residual

Production rendering from the main-IP profile remains blocked by design until
Task 9 auditions the candidates and pins the selected HeyGen `voiceId`. Preview
rendering can be requested explicitly and continues to use Apple Eddy without
being represented as production-ready audio.
