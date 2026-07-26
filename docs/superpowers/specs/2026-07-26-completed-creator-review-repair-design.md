# Completed Creator Review Repair Design

Date: 2026-07-26
Status: Approved for implementation

## Goal

Repair the existing completed 30-second demo task shown under "Completed and archived". Opening "View finished video" must present the task's real creator-facing artifacts as readable, reviewable content: script and prompts, Shot structure, reference images, voice, clips, and a playable final video. Historical technical envelopes must remain available to developer diagnostics but must never leak into the creator review UI.

## Confirmed failure modes

1. The content-library excerpt treats any string as prose. Historical artifact envelopes stored as JSON strings therefore appear verbatim in cards even though the detail viewer can project part of the same payload into a readable script.
2. Exact text selection is exposed only through a separate canonical-source mode. Users cannot select directly in the formatted review document, and nested historical envelopes do not consistently expose the canonical editable source.
3. The Shot queue defaults to `needs_attention`. A completed task whose Shots are already approved can therefore show zero Shots even while the content library lists many Shot-related artifacts.
4. Completion currently describes workflow state, not media availability. A completed task can point at a local-only artifact whose local media service is unavailable, whose file is missing, or whose container/codec cannot be decoded by the embedded browser.

## Chosen architecture

Use a compatibility projection at the creator boundary, backed by existing immutable artifact identities. The frontend consumes one normalized creator-review document for cards, detail views, text selection, and Shot grouping. The cloud API continues to preserve the original artifact content. Developer diagnostics may inspect the original envelope separately.

### Creator review projection

Add a pure projector that recursively unwraps known historical envelopes such as `artifacts`, `content`, and `package`, then emits only creator-facing fields:

- display label and concise excerpt;
- readable document sections;
- canonical editable text and exact source offsets;
- Shot identity and creator-facing layer summary;
- media role and media references.

Projection is deterministic, bounded by depth and payload size, and never renders unknown object keys as raw JSON in creator mode. Unknown technical content produces a neutral "content is being prepared" state rather than a serialization dump.

### Text selection and scoped revision

The formatted document becomes the selection surface. Text nodes carry source-range metadata so a browser selection can be converted into exact canonical offsets even across nested elements. The revision request includes artifact id, base version, source hash, start/end offsets, selected text, and the instruction. The backend validates the version, hash, and selected slice before applying a replacement; content outside the range must be byte-identical.

Prompt caching rules:

- the task-level system prompt and tool definitions remain byte-for-byte stable;
- the canonical artifact context is referenced by stable artifact/version/hash identity;
- the selected slice and instruction are appended as the user delta;
- a new tool registration affects only a new task, never the active task prefix.

### Completed Shot presentation

Opening a completed task uses `all` or `confirmed` as the initial Shot filter instead of `needs_attention`. If the stored Shot table is absent in a historical task, the compatibility projector groups artifacts by `relatedShotId` and builds a read-only historical Shot view. Each Shot shows only the three creator concepts: IP A-roll, text layer, and supporting/AIGC layer, plus voice and output status when present.

### Media recovery and playback

The media path is validated in order:

1. resolve cloud or local artifact reference;
2. verify the referenced file exists;
3. serve correct MIME type, byte ranges, content length, and cache headers;
4. probe browser-decodable container/codec metadata;
5. use the built-in player only after metadata loads.

Workflow completion and playable delivery are represented separately. A missing source is shown as "finished record, media missing" with a recovery action, never as a confirmed playable result. For the selected real completed task, regeneration starts from the earliest missing or invalid downstream artifact and preserves confirmed upstream script/Shot decisions unless the user explicitly changes them.

## User flow

1. The user opens the existing completed 30-second task and clicks "View finished video".
2. The workspace opens on the final preview if playable; otherwise it opens the first recoverable broken stage with a concise explanation.
3. The content library shows readable labels and excerpts, never JSON.
4. The Shot page shows the task's actual completed Shots and the selected Shot's three-layer composition.
5. The user can select text directly, open a small optimization popover, preview the replacement, and regenerate only that range.
6. Images open in a large review dialog, audio and clips play inline, and the final video plays in the built-in player.

## Testing and acceptance

- Historical JSON-string envelopes produce creator-readable excerpts and no visible JSON syntax.
- Formatted multi-node selections resolve to exact canonical offsets; unselected bytes remain unchanged after revision.
- Revision conflicts are rejected on stale version, mismatched source hash, or mismatched selected slice.
- Stable task prompts and tool definitions remain identical across multiple selection revisions.
- A completed task with approved Shots displays them without manual filter changes.
- A historical task without Shot rows receives a read-only artifact-derived Shot view.
- Media endpoints support `HEAD`/`GET`, byte ranges, correct MIME types, and browser metadata loading.
- The selected real 30-second task opens from "Completed and archived", exposes all available key artifacts, and plays a regenerated final video in the packaged client.
- Raw artifacts remain accessible only in the separate developer diagnostics interface.

## Non-goals

- Do not fabricate a synthetic completed task.
- Do not expose arbitrary artifact fields to creator users.
- Do not regenerate confirmed upstream content merely to repair a downstream media file.
- Do not change active-task system/tool prompt prefixes during scoped revisions.
