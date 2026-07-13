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

- `python3 mcp/ip_avatar_3d/test_server.py`: 74 tests run, 2 skipped, pass.
- `python3 mcp/ip_avatar_3d/test_voice_policy.py`: 8 tests run, pass.
- `python3 -m py_compile mcp/ip_avatar_3d/server.py mcp/ip_avatar_3d/test_server.py`: pass.

## Task Status

**BLOCKED** on production provider connectivity. Code and tests are ready, but
Task 9 must not be marked complete until real HeyGen auditions are generated,
presented blind, selected by the user, and verified before pinning a voice ID.
