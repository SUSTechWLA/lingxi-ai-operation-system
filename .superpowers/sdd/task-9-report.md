# Task 9 Report: Blind IP Voice Auditions

## Outcome

The code and mocked-test portion of Task 9 is implemented. The real voice
deliverable is **BLOCKED**, not complete, because both required production
provider preflights timed out on 2026-07-13. No substitute Apple or Kokoro
audio was generated, no real audition manifest was published, and no voice was
selected or pinned.

## MCP Contract

`generate_voice_auditions` reads exactly three unique, nonempty
`voice.productionVoiceCandidates` entries from the character profile and uses
the explicit `heygen` provider for every synthesis request. All candidates use
the same script, language, speed, speaking rate, duration calculation, and
normalization target:

```text
loudnorm=I=-16:TP=-1.5:LRA=7
```

Successful output names and order are deterministic:

```text
audition_A.wav
audition_B.wav
audition_C.wav
```

The public MCP result exposes only `label` and `path` for each A/B/C candidate
and sets `requiresUserSelection=true`. Provider voice IDs are written only to
the private `manifest.json`, together with the actual synthesis provider,
actual voice ID, synthesis source, shared settings, and measured loudness. The
manifest is created with mode `0600`.

## Fail-Closed Behavior

Generation fails without publishing partial A/B/C output when any of these
conditions occurs:

- the candidate list is not exactly three unique, nonempty IDs;
- the local production toolchain is unavailable;
- explicit HeyGen synthesis fails;
- returned `tts_provider` or `voice_id` provenance is missing or mismatched;
- synthesized audio is missing or empty;
- FFmpeg normalization fails or produces no file;
- FFmpeg loudness analysis fails or returns invalid measurements;
- integrated loudness is outside `-16 +/- 0.5 LUFS` or true peak exceeds
  `-1.5 dBTP`.

`dryRun=true` never synthesizes audio. It returns the planned label/path list
and reports `blocked` when the local audio engine, Node.js, or FFmpeg is
unavailable.

## TDD Evidence

The RED run executed 70 tests and produced 12 missing-tool errors (including
subtests), all caused by the absent `generate_voice_auditions` function. The
GREEN run executed 70 tests successfully with the two existing Blender-only
tests skipped.

Mocked tests cover successful blind output and the private manifest, identical
settings and deterministic ordering, dry-run blocking, invalid candidate
count, unavailable production tooling, missing provenance, provider and voice
provenance mismatch, missing synthesis output, normalization failure,
loudness-analysis failure, and measured loudness outside target. No test uses
the network.

## Provider Preflight

Both required live commands were run from the repository root on 2026-07-13:

```text
npx hyperframes auth status
```

Result: exit 1, `TypeError: fetch failed`, caused by `ConnectTimeoutError`
with code `UND_ERR_CONNECT_TIMEOUT` after 10000 ms.

```text
node ~/.agents/skills/hyperframes-media/scripts/heygen-tts.mjs --list
```

Result: exit 1, `TypeError: fetch failed`, caused by `ConnectTimeoutError`
with code `UND_ERR_CONNECT_TIMEOUT` after 10000 ms.

Because production authentication and the HeyGen voice API are unreachable,
real A/B/C auditions cannot be generated or presented for blind user
selection.

## Profile State

`ip形象/main_ip/character-profile.json` was not modified. Its production
provider remains `heygen`, `fallbackPolicy` remains `error`, and `voiceId`
remains empty because no real blind user selection occurred.

## Verification

- `python3 mcp/ip_avatar_3d/test_server.py`: 70 tests run, 2 skipped, pass.
- `python3 mcp/ip_avatar_3d/test_voice_policy.py`: 8 tests run, pass.
- `python3 -m py_compile mcp/ip_avatar_3d/server.py mcp/ip_avatar_3d/test_server.py`: pass.

## Task Status

**BLOCKED** on production provider connectivity. Code and tests are ready, but
Task 9 must not be marked complete until real HeyGen auditions are generated,
presented blind, selected by the user, and verified before pinning a voice ID.
