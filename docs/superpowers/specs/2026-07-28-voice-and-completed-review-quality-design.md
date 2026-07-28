# Voice and Completed Review Quality Design

## Goal

Ship two creator-facing quality improvements as one coherent product update:

1. Every default IP talking-head video uses the approved sloth voice derived from the owner-provided recording, while creators may either clone an authorized reference recording with GPT-SoVITS or use a finished narration recording directly.
2. Completed-project requirements, direction, script, Shot, preview, and delivery surfaces emphasize the latest acceptable result in a full-width review canvas instead of squeezing readable content beside a permanent artifact sidebar.

ChatTTS is not part of this design.

## Confirmed Product Decisions

### Voice modes

IP talking-head projects expose three mutually exclusive modes:

- `default_ip`: synthesize the confirmed narration with the pinned main-IP GPT-SoVITS profile.
- `reference_clone`: synthesize the confirmed narration with GPT-SoVITS using an authorized user recording and its exact reference transcript.
- `recorded_narration`: use an authorized finished narration recording directly as the audio master after deterministic mastering.

`default_ip` is selected automatically for IP talking-head creation. Existing talking-head projects without an explicit mode resolve to `default_ip`. Non-talking-head projects do not receive an implicit IP voice.

### Completed-project review layout

The approved layout is **full-width focused review**:

- requirements, direction, and script render as readable documents in the full review canvas;
- one current artifact produces no artifact navigation;
- two or more current artifacts produce a compact horizontal switcher above the canvas;
- image, video, and audio content use the available canvas width;
- the Shot step keeps its purpose-built Shot queue and inspector;
- preview and delivery keep the purpose-built player and delivery controls;
- historical versions remain secondary, collapsed, and explicitly requested.

## Canonical Voice Assets

The owner-provided source recording remains the provenance source:

- source: `ip形象/ip音频.wav` in the owner workspace;
- duration: `26.679729` seconds;
- encoding: 48 kHz mono PCM16 WAV;
- SHA-256: `0304b63d381c065e90b34ad373c6438f8b71123ffa74781f66fe191043570dd9`.

The deployable reference is `ip-assets/main-ip/voice/reference/main_ip_voice_ref_v1.wav`. It is the verified speech interval derived from the source after leading silence removal. The pinned profile is `ip-assets/main-ip/character-profile.json`, provider `gpt_sovits_local`, voice ID `main_ip_warm_knowledge_host_v1`.

The source recording is never looped or reused as arbitrary narration. It supplies approved voice provenance; the short deployable reference supplies the exact GPT-SoVITS conditioning sample.

## Architecture

### Standard MCP capability

Append `ip_avatar_3d.synthesize_reference_voice` to the existing MCP provider catalog. Existing tool names and schemas remain byte-for-byte unchanged. A running task keeps its snapshotted tool catalog; newly started tasks can discover the appended tool.

The tool supports `default_ip` and `reference_clone` synthesis through one stable input contract. It accepts target text, provider, voice ID, reference audio, exact reference text, verification and usage-rights flags, language, speed, output directory, and optional character profile. It returns a versioned result containing the mastered audio path, duration, provider, voice ID, input and output hashes, production-readiness, rights confirmation, and provenance.

`recorded_narration` does not synthesize speech. It enters the same audio-master service through a separate explicit source mode so arbitrary `audioPath` values cannot bypass project ownership and hash validation.

### Component boundaries

1. **Creator voice selection** owns the three user-facing modes and input completeness rules.
2. **Local artifact intake** owns upload, MIME validation, project-scoped storage references, content hashes, and playback metadata.
3. **Voice request projection** transfers only allow-listed project-scoped identifiers, hashes, transcript, provider, and consent state to cloud project configuration.
4. **Local voice resolver** verifies project ownership, resolves the local artifact, and rejects traversal, remote URLs, cross-project references, or hash mismatch.
5. **MCP synthesis tool** owns GPT-SoVITS conditioning, deterministic synthesis, mastering, and provenance.
6. **Audio-master compiler** fingerprints script version, mode, provider, voice profile, reference or narration hash, transcript hash, synthesis settings, and mastered output.
7. **A-roll compiler** consumes only a production-ready audio master. It cannot silently select Apple preview TTS, ChatTTS, or another voice.
8. **Focused review workspace** owns current-artifact navigation and full-width presentation without changing backend audit retention.

## Voice Data Flow

### Default IP voice

1. Creation defaults to `default_ip` for IP talking-head projects.
2. The confirmed script becomes the target narration.
3. The local runner loads the pinned main-IP profile and validates its reference file, hashes, model provenance, endpoint policy, and output directory.
4. `ip_avatar_3d.synthesize_reference_voice` invokes the existing GPT-SoVITS client.
5. The generated output is mastered to 48 kHz mono WAV with the pinned loudness and true-peak policy.
6. The audio master and complete provenance become the only production input for timing, lip sync, subtitles, A-roll, and assembly.

### User reference clone

1. The creator uploads one WAV, MP3, M4A, or FLAC reference recording.
2. The creator supplies the exact words spoken in that recording and confirms authorization.
3. Local intake registers the file as a project-scoped artifact and exposes playback and validation state.
4. The confirmed project script remains the target narration; the supplied text is only the reference transcript.
5. The local resolver verifies ownership, storage reference, MIME type, content hash, transcript verification, and consent before the MCP call.
6. GPT-SoVITS produces and masters the target narration with complete reference and model provenance.

### Finished recorded narration

1. The creator uploads one finished narration recording and confirms authorization.
2. Local intake verifies project ownership, format, decodability, duration, and content hash.
3. The audio-master service converts it to the canonical sample format and applies deterministic loudness and true-peak mastering.
4. No TTS engine is called. The mastered recording duration becomes authoritative for downstream timing.

## Privacy and Safety

- Reference recordings and voice embeddings remain local by default.
- Cloud state stores only project-scoped artifact identity, `local://` storage reference, MIME type, content hash, transcript, selected mode/provider, and consent state.
- Absolute paths and audio bytes never enter cloud project state or model prompts.
- Reference cloning requires both `referenceTextVerified=true` and `usageRightsConfirmed=true`.
- Finished narration requires `usageRightsConfirmed=true` and explicit `recorded_narration` provenance.
- Production synthesis accepts only pinned local GPT-SoVITS endpoints and model provenance.
- Missing model runtime, missing consent, transcript mismatch, unreadable media, invalid project ownership, or output-path escape fails before synthesis.

## Failure and Recovery

Production voice generation fails closed. It never falls back to macOS `say`, ChatTTS, an unpinned remote provider, or a different voice. Preview-only audio remains visibly marked and cannot satisfy production delivery.

When regeneration fails:

- the last accepted current audio remains available;
- the failed candidate never replaces it;
- the creator sees a plain-language cause and a scoped retry action;
- logs retain stage, provider, request correlation, model fingerprint, and validation outcome without exposing audio bytes or sensitive local paths.

Changing the confirmed script, voice mode, provider/profile, reference recording, reference transcript, finished narration, or synthesis settings invalidates audio master, timing, lip sync, subtitles, A-roll, preview, and delivery. Earlier accepted versions remain recoverable through explicit history controls.

## Creator Voice Experience

The advanced options for IP talking-head creation show a `配音` section with three radio cards:

1. **默认 IP 音色** — selected by default, names the approved character voice, shows readiness, and provides a short local reference preview.
2. **上传录音复刻音色** — shows upload state, playback, exact-reference-transcript input, authorization confirmation, provider readiness, and validation errors.
3. **直接使用完整口播** — shows upload state, playback, duration, format, and mastering readiness without cloning-specific fields.

The primary creation brief stays focused on the video idea. The confirmed script is the synthesis target, so the creator does not re-enter it in the voice section. A creator may also attach a script document as project reference material through the existing material flow.

The Start action is disabled only when the selected custom mode is incomplete. Errors state exactly what is missing and how to fix it.

## Focused Completed-Project Review

### Information architecture

The backend continues to retain complete artifact and audit history. The creator surface consumes only the current acceptable projection. Developer diagnostics and explicit version history retain access to historical attempts.

The permanent artifact sidebar is removed from requirements, direction, script, preview, and delivery review. A `CreatorCurrentArtifactSwitcher` appears above the proofing canvas only when at least two current reviewable artifacts exist:

- text artifacts use concise semantic labels;
- images show compact thumbnails;
- video and audio show type, Shot label where safe, and duration when available;
- selected state is keyboard-visible and announced with tab semantics;
- a narrow window uses horizontal scrolling without reducing document width.

### Reading and media layout

- Document content uses a comfortable `68ch` to `78ch` reading measure within a full-width canvas.
- The canvas, not a sidebar, owns available width; outer spacing scales with the window.
- Requirements use the existing structured brief panel at full width.
- Direction and script use readable document projections, selection-based revision, and existing impact confirmation.
- Images and video may exceed the document measure and use the full media canvas.
- Audio uses a full-width compact player with transcript or associated script below it.
- Shot review remains a separate queue-plus-inspector workflow because Shot selection is a real collection, not artifact history.

### Responsive behavior

- Desktop and ordinary Electron windows never reserve a fixed 280–320 px artifact column.
- The switcher scrolls horizontally below its available width.
- The proofing canvas stays single-column at all widths.
- Controls retain visible focus, semantic tabs, useful labels, and reduced-motion compatibility.

The existing warm Tangying palette, typography, and visual language remain. This is an information-hierarchy correction, not a brand redesign.

## Cache and Tool-Catalog Stability

- Existing MCP tool definitions and system-prompt tool blocks are not edited.
- The synthesis tool is appended as a new definition with a stable JSON Schema.
- One task snapshots one ordered tool-catalog revision and reuses it byte-for-byte for the task lifetime.
- Newly registered tools become visible only to newly started tasks.
- Voice configuration values enter task state and artifacts, not the stable system-prompt prefix.

## Testing and Acceptance

### Voice contracts

1. Canonical source and deployable-reference metadata and hashes match the approved profile.
2. Default IP talking-head compilation selects production GPT-SoVITS and never Apple preview TTS or ChatTTS.
3. `synthesize_reference_voice` is discoverable through MCP with a stable, appended schema.
4. A verified project-local reference plus exact transcript generates a mastered 48 kHz mono WAV with complete provenance.
5. A finished narration recording bypasses TTS and becomes the mastered timing authority.
6. Missing consent, transcript verification, model provenance, project ownership, or valid local output fails before synthesis.
7. Custom voice fields survive frontend creation, cloud persistence, local resolution, MCP execution, audio fingerprinting, and A-roll rendering.
8. Failed regeneration preserves the last accepted audio and exposes actionable recovery.

### Review layout

1. Requirements, direction, and script use the full review canvas in common desktop and Electron window widths.
2. One current artifact produces no switcher; multiple current artifacts produce one compact top switcher.
3. Long Chinese text maintains a readable measure and no longer collapses into a narrow left column.
4. Image, video, audio, Markdown, and readable JSON projections remain previewable and revisable.
5. Shot queue, preview player, delivery controls, selection-based revision, impact confirmation, and version restoration continue to work.
6. Historical attempts never reappear as primary cards, counts, or process rows.

### Release verification

- Frontend creator tests, type checking, production build, and lint pass.
- Cloud and local backend unit and contract suites pass.
- MCP Python contract and voice policy suites pass.
- Packaged local-agent and Electron client include the canonical voice asset and tool definition.
- An installed-client smoke test creates or opens a real project, confirms the selected voice mode, verifies local/cloud/renderer health, and inspects the completed-project layout at desktop and narrow widths.

## Out of Scope

- ChatTTS support.
- Remote voice providers or automatic cloud transcription.
- Automatic authorization inference.
- Replacing the pinned IP source recording without a separately reviewed migration.
- Redesigning the global Tangying brand or developer diagnostics.
