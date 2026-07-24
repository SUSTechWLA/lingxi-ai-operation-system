# User Content Review and Developer Diagnostics Design

## Goal

Turn Creator Studio into a low-learning-cost content review surface. A creator should only judge and improve outputs that change the finished video:

1. text and generation prompts;
2. generated reference images and keyframes;
3. video clips;
4. narration and other audio.

Execution traces, JSON payloads, tool and MCP calls, quality reports, continuity reports, package manifests, and recovery diagnostics remain available to developers in a separate diagnostics interface and never appear in the ordinary creator navigation.

## Approved Product Boundary

The creator and developer surfaces use the same project data but have different projections.

- Creator Studio answers: “What will the audience see or hear, and how do I improve it?”
- Developer Diagnostics answers: “What did the task execute, why did it fail, and what evidence proves the result?”

Technical artifacts are not deleted and are not relabeled as creator content. They are filtered out before the creator artifact list is rendered. The existing feature-gated `#/developer/*` route remains the isolated developer entry and continues to be excluded from ordinary production creator bundles unless explicitly enabled.

## Creator Artifact Projection

Every artifact visible to a creator belongs to one of four categories:

| Category | Example kinds | Primary review action |
| --- | --- | --- |
| Text | `VIDEO_SCRIPT`, `VIDEO_PROMPTS`, `KEYFRAME_PROMPTS`, prompt-bearing external generation requests, publish copy | Read, select text, request a local rewrite, edit directly |
| Image | `SHOT_KEYFRAME`, generated reference images, storyboard and preview images | Preview immediately, enlarge, select a region, discuss and regenerate |
| Video | `SHOT_VIDEO_CLIP`, `COMPOSITED_SHOT_VIDEO`, selected preview and final video | Play, select a time range, request a scoped revision |
| Audio | `SHOT_AUDIO`, narration masters and uploaded narration | Play, select a time range, adjust voice, pacing, pauses, or regenerate |

All other artifacts are absent from Creator Studio. If a step contains no creator-facing artifact, the interface says that key content is still being prepared. It does not expose technical fallbacks or raw file links.

Classification uses kind, MIME type, metadata artifact type, generation kind, and file name. The classifier is deterministic and shared by the artifact list and the main proofing canvas so the two surfaces cannot disagree.

## Creator Information Architecture

The artifact area becomes a content library rather than an audit drawer:

```text
[文字与提示词] [参考图] [视频片段] [语音]

Key content list                 Current review
Shot 01 video prompt       ->    readable text or media
Shot 01 reference image          local selection and feedback
Shot 01 narration                confirm / regenerate / versions
```

The current creation step remains visible, but attempt numbers, byte sizes, storage types, artifact kinds, hashes, and technical status labels do not appear. A compact history entry is available only when previous creator-facing versions exist.

The visual signature is a contact-sheet rhythm: media thumbnails use a consistent cinematic frame, while text uses a quiet manuscript surface. The existing warm Tangying palette remains, with less dark chrome and fewer badges. Color roles:

- canvas cream `#FFF6D6`;
- review paper `#FFFDF6`;
- ink brown `#2D1907`;
- action amber `#F4A000`;
- selection amber `#FFE39A`;
- success green `#2F7A55`.

Display text continues to use the product’s current Chinese sans-serif stack. Script and prompt bodies use the body stack with generous line height; technical monospace typography is absent from Creator Studio.

## Text Selection and Revision

Creator text is selectable in the readable view. Mouse, trackpad, and keyboard selection produce a floating assistant anchored to the selection.

The selection contains:

```json
{
  "kind": "text",
  "start": 24,
  "end": 68,
  "text": "selected source text"
}
```

The assistant offers:

- more concise;
- stronger visual language;
- improve rhythm;
- custom instruction.

The instruction revision API receives the exact source range and selected text. The backend validates non-empty text, integer bounds, maximum selection length, and normalized agreement between the range and the current artifact. A stale or mismatched range returns a conflict and never rewrites a different passage.

The existing impact preview, downstream invalidation, idempotency, version creation, and restore flow remain mandatory. Direct editing remains available for complete text replacement.

## Image Review and Conversation

Image artifacts always render an actual thumbnail. A file path or link is never the primary representation.

Clicking a thumbnail opens a full-screen review dialog:

- the image occupies the main stage and supports zoom and pan;
- pointer drag creates an optional normalized rectangle selection;
- the right panel contains the conversation and quick actions;
- Escape closes the dialog and focus returns to the originating thumbnail.

Quick actions are:

- locally redraw the selected area;
- improve composition;
- adjust style or lighting;
- regenerate the entire image;
- replace the image;
- keep this version.

Each action becomes an ordinary revision instruction with the current artifact ID, version, and optional rectangle selection. The user sees the expected downstream effect before confirming. The dialog does not invent a separate image mutation protocol.

If the preview cannot be read, the primary recovery is “Reload preview.” “Locate original file” is a secondary fallback and never replaces the preview.

## Video and Audio Review

Video continues to use the shared built-in player. A user may set the selection start from the current playhead, set the end from the playhead, clear the range, and submit a scoped revision. Manual numeric millisecond fields are removed from the primary interface.

Audio receives the same transport and time-selection model:

- play and pause;
- current and total time;
- volume and playback speed;
- set start and end from the playhead;
- quick actions for tone, speed, pauses, pronunciation, and full regeneration.

The stored selection remains the existing `time` selection, so video and audio share backend validation and provenance.

## Developer Diagnostics

The existing feature-gated `DeveloperConsolePage` becomes the dedicated diagnostics entry. It does not appear in Creator Studio navigation.

The diagnostics page is task-oriented:

1. task and run summary;
2. node timeline and current state;
3. tool and MCP calls with request, response, duration, retry, and error classification;
4. artifact registry including technical artifacts, raw JSON, hashes, storage references, lineage, and version history;
5. human and quality gates;
6. logs and recovery controls.

Diagnostics default to a selected project/run rather than a global data dump. Sensitive provider credentials and local absolute paths remain redacted. Production builds omit the developer console unless `VITE_ENABLE_DEVELOPER_CONSOLE=1`; development builds keep it available.

## Data Flow

1. Creation View returns the complete artifact descriptors.
2. `classifyCreatorReviewArtifact` maps each descriptor to text, image, video, audio, or hidden.
3. Creator Studio filters hidden artifacts before selection, counting, empty states, and rendering.
4. The selected visible artifact is hydrated through the existing cloud/local content resolver.
5. The proofing canvas renders the category-specific viewer.
6. Text, rectangle, or time selections are normalized and sent through the existing revision-impact and revision APIs.
7. Developer Diagnostics continues to query the complete, unfiltered run and artifact data.

## Error Handling and Accessibility

- A failed thumbnail or media request shows a bounded recovery card with retry.
- A stale artifact selection refreshes the current version and preserves the written instruction.
- Dialogs trap focus, close with Escape, restore trigger focus, and expose labelled controls.
- Text selection actions are keyboard reachable.
- All preview images have useful alternative text.
- Narrow layouts move the conversation below the media without horizontal overflow.
- Reduced-motion mode removes non-essential dialog and selection transitions.

## Testing and Acceptance

1. The creator artifact classifier exposes only text, image, video, and audio artifacts and hides technical artifacts.
2. Hidden artifacts never affect creator counts, empty states, or default selection.
3. Text selection survives frontend serialization, API schema generation, backend normalization, idempotency hashing, and revision provenance.
4. A stale or mismatched text range is rejected before mutation.
5. Selecting text opens one floating assistant and a scoped instruction creates a new version.
6. Image artifacts render thumbnails; clicking one opens a full-screen dialog with the image and conversation panel.
7. Rectangle selection, image quick actions, impact preview, confirmation, and version restoration work together.
8. Video and audio can create valid time selections from playback state without exposing raw millisecond inputs.
9. Creator Studio contains no raw JSON, hashes, storage references, tool names, MCP calls, QA reports, or diagnostic logs.
10. Developer Diagnostics retains complete task, node, tool, MCP, artifact, gate, log, and recovery information when its feature flag is enabled.
11. Desktop and 390 px layouts have no horizontal overflow.
12. Frontend tests, cloud schema tests, service tests, lint, production builds, Electron packaging, and installed-client smoke tests pass.
