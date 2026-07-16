# Task 9 Report: IP Production Voice

## Outcome

The user has approved `ip形象/ip音频.wav` as the permanent local IP voice
source. The file remains untracked and unmodified at SHA-256
`0304b63d381c065e90b34ad373c6438f8b71123ffa74781f66fe191043570dd9`.

The deterministic reference clip
`ip形象/main_ip/voice/reference/main_ip_voice_ref_v1.wav` also remains
untracked. It is a 48 kHz mono PCM16 extraction starting at `1.248396` seconds
for `8.542` seconds, with SHA-256
`0119b8a407ed1be5731db699d6166b7e4c86de540435c69bf4fb5229415382d8`.

The local GPT-SoVITS production adapter, mastering gate, policy, pinned profile,
and mocked tests are implemented. Local bundle preflight is **READY** with the
verified transcript, `v2ProPlus` checkpoints, and all expected hashes matching.
No substitute Apple or Kokoro audio and no real final production voice were
generated in this worker. Runtime `productionReady` remains false until an
actual synthesized WAV passes mastering and loudness verification.

## Local GPT-SoVITS Contract

`gpt_sovits_local` is an explicit production provider. Production requires a
nonempty stable bundle `voiceId` and `fallbackPolicy="error"`. The stdlib HTTP
client defaults to `http://127.0.0.1:9880` and rejects non-loopback endpoints
unless remote access is explicitly enabled, preventing private local paths
from being sent off-machine by default.

Before HTTP, the client requires readable nonempty reference, GPT checkpoint,
and SoVITS checkpoint files plus a nonempty prompt transcript explicitly
marked verified. It hashes all three files and rejects any supplied expected
hash mismatch. Under one process lock per endpoint it calls the official
`GET /set_gpt_weights`, `GET /set_sovits_weights`, and `POST /tts` sequence.
The lock covers the complete model-switch and synthesis operation so concurrent
requests cannot interleave model bundles.

Inference uses seed `20260714`, WAV/non-streaming output, disabled
parallel inference, and explicit deterministic settings where supported. The
returned bytes must parse as a nonempty WAV before atomic output publication.
Reference/checkpoint hashes are checked again while the endpoint lock is held
so changed files cannot produce stale provenance.

Production metadata includes actual provider and voice ID, reference and both
checkpoint hashes, normalized endpoint, `GPT-SoVITS/api_v2` model identifier,
configured model version, seed/settings, and generated-file SHA-256.
Bundle preflight reports `bundleReady=true` but never claims
`productionReady`. Raw synthesis readiness is accepted only when the adapter
reports complete matching provenance and a real nonempty output file.

The explicit local A-roll route then creates a separate 48 kHz mono PCM16 WAV
using restrained 55 Hz high-pass and 18 kHz low-pass filters, gentle 1.5:1
compression, and `loudnorm=I=-16:TP=-1.5:LRA=7`. Mastering is staged and only
atomically replaces the final master after WAV format validation and measured
loudness verification. Final `productionReady=true` requires integrated
loudness within `-16 +/-0.5 LUFS`, true peak at or below `-1.5 dBTP`, and a
nonempty mastered file. Metadata preserves the raw generated SHA-256 and adds
the mastered SHA-256 plus measured integrated loudness, true peak, and LRA.

`check_gpt_sovits_voice` deterministically validates a profile bundle and
hashes without synthesis or network I/O. The main-IP profile currently reports
`ready` and `bundleReady=true` through this tool. Environment variables and
home-directory markers in private local paths are expanded before resolution.

## MCP Contract

`generate_voice_auditions` reads exactly three unique, nonempty
`voice.productionVoiceCandidates` entries from the character profile and uses
the explicit `heygen` provider for every synthesis request. All candidates use
the same script, language, speed, speaking rate, duration calculation, and
normalization target:

```text
loudnorm=I=-16:TP=-1.5:LRA=7
```

Each set is published under a deterministic SHA-256 directory derived from
the script, ordered candidate IDs, HeyGen synthesis settings, normalization
settings, and WAV output settings. Names and order inside the set remain
deterministic:

```text
<outputDir>/<setId>/audition_A.wav
<outputDir>/<setId>/audition_B.wav
<outputDir>/<setId>/audition_C.wav
```

The public MCP result exposes only `label` and `path` for each A/B/C candidate
and sets `requiresUserSelection=true`. Resolved paths are constrained to the
caller output root and all generated child names are fixed safe components.
Provider voice IDs are written only to the private `manifest.json`, together
with the actual synthesis provider, actual voice ID, synthesis source, shared
settings, content hashes, byte lengths, and measured loudness.

Each set ID uses an exclusive `O_EXCL` lock and a unique staging directory.
All normalized files and the private manifest are built in staging. The
manifest is changed to mode `0600`, and the complete staged set is validated
before one atomic directory rename publishes it. A concurrent call either
fails clearly on the set lock or validates and reuses the already-complete
set. An existing invalid target is treated as a collision and is never
overwritten.

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

Failures remove only the caller-owned staging directory and lock. Previously
published sets and unrelated files under the output root are never unlinked or
overwritten. Existing complete sets are validated against manifest metadata,
candidate provenance, file sizes, SHA-256 hashes, and loudness measurements
before idempotent reuse.

`dryRun=true` never synthesizes audio. It returns the planned label/path list
and deterministic set ID, and reports `blocked` when the local audio engine,
Node.js, or FFmpeg is unavailable.

## TDD Evidence

The RED run executed 70 tests and produced 12 missing-tool errors (including
subtests), all caused by the absent `generate_voice_auditions` function. The
initial GREEN run executed 70 tests successfully with the two existing
Blender-only tests skipped.

The atomic-publication review RED run expanded the suite to 74 tests and
produced six errors for the missing set ID, content-addressed paths,
idempotent reuse, existing-target collision handling, exclusive locking, and
staging cleanup. The follow-up GREEN run executes 74 tests successfully with
the two existing Blender-only tests skipped.

Mocked tests cover successful blind output and the private manifest, identical
settings and deterministic ordering, dry-run blocking, invalid candidate
count, unavailable production tooling, missing provenance, provider and voice
provenance mismatch, missing synthesis output, normalization failure,
loudness-analysis failure, measured loudness outside target, preservation of a
valid set after a failed rerun, concurrent lock collision, validated existing
set reuse, invalid existing target preservation, manifest mode `0600`, and no
partial publication after failure. No test uses the network.

The local-provider RED cycle began with `ModuleNotFoundError` for the missing
client. The policy suite then failed one assertion and errored once because
`gpt_sovits_local` was unsupported. The server suite ran 79 tests with two
failures and four errors for the missing health tool, local route/config handoff,
policy provider, and profile defaults. A final defense-in-depth RED run executed
80 server tests and failed two subtests because incomplete or mismatched local
adapter provenance was still accepted.

The completed local client suite runs 11 tests covering loopback rejection,
official request order/payload, verified prompt requirements, reference and
checkpoint hash mismatch, empty/non-WAV output, timeout/HTTP errors, per-endpoint
serialization, complete provenance, bundle-only preflight semantics,
environment-variable path expansion, pre-HTTP output validation, and bundle
mutation during synthesis. The policy suite runs 10 tests, and the server suite
runs 81 tests with the two existing Blender-only skips. Server tests also cover
the exact mastering format/filter command, raw and mastered hashes, measured
loudness provenance, and fail-closed loudness rejection. All HTTP is mocked.

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

Those failures blocked the former HeyGen audition path. That path and its
atomic publication behavior remain available and tested, but the canonical
production provider is now the user-approved local GPT-SoVITS path.

## Profile State

Only the `voice` section of `ip形象/main_ip/character-profile.json` changed.
Its production provider is `gpt_sovits_local`, `fallbackPolicy` remains
`error`, and `voiceId` is pinned to `main_ip_warm_knowledge_host_v1`. It records
the original source lineage and hash, stable reference clip and hash,
human-verified prompt, Chinese prompt/text languages, `v2ProPlus` GPT and
SoVITS checkpoint paths/hashes, seed `20260714`, and speed `0.94`. Checkpoint
paths use `~` expansion instead of a username-specific absolute path. The old
HeyGen `productionVoiceCandidates` field is absent, so production cannot select
or fall back to a different voice.

## Verification

- `python3 mcp/ip_avatar_3d/test_gpt_sovits_client.py`: 11 tests run, pass.
- `python3 mcp/ip_avatar_3d/test_server.py`: 81 tests run, 2 skipped, pass.
- `python3 mcp/ip_avatar_3d/test_voice_policy.py`: 10 tests run, pass.
- `python3 -m py_compile mcp/ip_avatar_3d/gpt_sovits_client.py mcp/ip_avatar_3d/test_gpt_sovits_client.py mcp/ip_avatar_3d/server.py mcp/ip_avatar_3d/test_server.py`: pass.

## Task Status

**CODE AND CONFIG COMPLETE.** The canonical local bundle is pinned and local
health passes. A real generated A-roll voice was intentionally not produced in
this worker, so no run should report `productionReady=true` until GPT-SoVITS is
running and the returned master passes the format, loudness, and true-peak
gates. The historical HeyGen network preflight remains blocked but is no longer
the canonical production dependency.
