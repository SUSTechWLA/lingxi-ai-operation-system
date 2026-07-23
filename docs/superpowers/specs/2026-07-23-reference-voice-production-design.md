# Reference Voice Production Design

## Goal

Make every default sloth A-roll use the approved IP voice derived from the owner-provided recording, while allowing a user to upload an authorized recording and synthesize the approved narration script in that voice.

## Existing State and Decision

The main IP profile already pins `gpt_sovits_local`, model hashes, an exact reference transcript, a fixed seed, a 48 kHz mono PCM16 reference, two-pass loudness normalization, and fail-closed production behavior. The creator entry point currently overrides that profile with `ipRenderMode=preview`, which selects the Apple preview voice. This override is the primary defect.

The approved source recording is:

- Duration: `26.679729` seconds
- Format: 48 kHz mono PCM16 WAV
- SHA-256: `0304b63d381c065e90b34ad373c6438f8b71123ffa74781f66fe191043570dd9`

The packaged English-named reference `voice/reference/main_ip_voice_ref_v1.wav` is the speech interval derived from that recording after removing its initial silence. Its metadata matches the source, its duration is `8.542` seconds, and its internal pause aligns with the source interval. It remains the deployable reference because GPT-SoVITS requires a short reference and an exact matching transcript; the 26.68-second source is provenance, not an audio track to loop in a video.

GPT-SoVITS remains the default production engine. ChatTTS is an optional local adapter because it now supports `sample_audio_speaker()` with `spk_smp` and an exact `txt_smp`, but it does not yet have the pinned model bundle, deterministic production provenance, mastering contract, or packaged runtime that the current GPT-SoVITS path has.

## Voice Modes

The project-level `voiceMode` is one of:

- `default_ip`: synthesize the confirmed script with the pinned main IP GPT-SoVITS voice.
- `reference_clone`: synthesize the confirmed script from a user-provided reference recording and its exact transcript.
- `recorded_narration`: use a user-provided finished narration recording without cloning.

Talking-head creation defaults to `default_ip`. Cinematic projects do not receive an IP voice configuration unless explicitly selected.

## Standard MCP Tool

Add `ip_avatar_3d.synthesize_reference_voice` as a standard MCP tool. Existing MCP tools and schemas remain byte-for-byte unchanged; the new tool is appended to the provider catalog.

Input:

```json
{
  "text": "confirmed target narration",
  "provider": "gpt_sovits_local",
  "voiceId": "project voice identifier",
  "referenceAudioPath": "/resolved/local/reference.wav",
  "referenceText": "exact words spoken in the reference",
  "referenceTextVerified": true,
  "usageRightsConfirmed": true,
  "language": "zh",
  "speed": 0.94,
  "outputDir": "/resolved/local/project/output",
  "characterProfilePath": "/optional/profile.json"
}
```

Output:

```json
{
  "schemaVersion": "tangying-reference-voice/v1",
  "status": "ready",
  "audioPath": "/local/project/output/voice_master.wav",
  "durationSec": 30.0,
  "ttsProvider": "gpt_sovits_local",
  "voiceId": "project voice identifier",
  "referenceAudioSha256": "hex",
  "generatedFileSha256": "hex",
  "masteredFileSha256": "hex",
  "productionReady": true,
  "usageRightsConfirmed": true,
  "provenance": {}
}
```

The tool shares the existing GPT-SoVITS client and mastering implementation. It rejects missing target text, missing or mismatched reference transcript confirmation, missing usage-rights confirmation, unsupported formats, unreadable paths, non-loopback production endpoints, incomplete provider provenance, and output outside the requested directory.

`chattts_local` uses the same public contract when installed. If its runtime or pinned model fingerprint is unavailable, it returns a structured blocked result and never silently falls back to another voice.

## Local Storage and Privacy

The browser uploads reference audio to the existing local artifact service. Cloud project state stores only a content-addressed `local://projects/...` storage reference, MIME type, hash, transcript, provider choice, and consent flag. It never stores the absolute local path or audio bytes.

At execution time, the local runner resolves only approved voice-reference storage fields into a path under its project data directory. Path traversal, arbitrary absolute paths from cloud input, remote URLs, and cross-project references are rejected before the MCP call.

Voice recordings and voice embeddings remain local by default. No ASR service or remote voice provider receives the recording in this version. The user supplies the exact reference transcript; automatic transcription can be added later as a separate opt-in local ASR feature.

## Creator Experience

The advanced creator options show a `配音` section only for `IP 口播视频`:

1. `默认 IP 音色` — selected by default; explains that the approved sloth voice is used.
2. `上传录音复刻音色` — accepts one WAV, MP3, M4A, or FLAC reference, the exact text spoken in it, and a required statement confirming authorization.
3. `直接使用已录口播` — accepts one finished narration recording and uses its duration as the audio master.

The main creation prompt remains the creative brief. The target narration is the confirmed script artifact, so a user does not enter the same script twice. File status, errors, and the selected voice mode remain visible before starting.

## Pipeline Data Flow

For `default_ip`, the A-roll compiler emits production rendering with the pinned GPT-SoVITS provider, voice ID, and error fallback policy. It no longer emits the Apple preview override.

For `reference_clone`, project configuration carries the local storage reference, exact reference transcript, provider, and consent. The local execution plane resolves the reference and calls the shared synthesis service before rendering. The render output records both the synthesized audio provenance and the A-roll asset provenance.

For `recorded_narration`, the local runner resolves the finished narration and passes it with verified project ownership and content hash. Production rendering accepts it only with explicit `recorded_narration` provenance; arbitrary `audioPath` remains rejected.

The resulting audio-master revision fingerprints:

- confirmed target script version;
- voice mode and provider;
- voice profile ID/version;
- reference or narration content hash;
- reference transcript hash where applicable;
- synthesis settings and generated/mastered audio hashes.

Any change invalidates downstream lip sync, subtitles, Shot timing, A-roll, and final assembly while preserving earlier versions.

## Compatibility and Cache Stability

- Existing tool definitions are not modified; the new MCP tool is appended.
- A task snapshots one stable tool catalog revision for its lifetime.
- Newly registered MCP tools are visible only to newly started tasks.
- Existing projects without `voiceMode` migrate logically to `default_ip` for talking-head projects.
- Existing non-talking-head creation remains unchanged.

## Testing and Acceptance

1. The canonical IP reference provenance matches the approved source recording metadata and hashes.
2. A default talking-head request compiles to production GPT-SoVITS and never Apple preview TTS.
3. `synthesize_reference_voice` is discoverable through MCP with stable JSON Schema.
4. A verified local reference and transcript produce a mastered 48 kHz mono WAV with complete provenance.
5. Missing rights confirmation, transcript verification, or project-contained local reference fails before synthesis.
6. Reference-clone fields survive frontend creation, cloud planning, local storage resolution, MCP execution, audio-master fingerprinting, and A-roll rendering.
7. Direct recorded narration is accepted only with project ownership, content hash, and explicit mode.
8. The creator UI clearly exposes all three modes and prevents starting an incomplete custom-voice request.
9. Frontend, cloud, local runner, MCP contract, packaging, and installed-client smoke tests pass.
