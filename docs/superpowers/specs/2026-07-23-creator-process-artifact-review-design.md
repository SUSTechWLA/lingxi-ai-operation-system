# Creator Process and Artifact Review Design

**Date:** 2026-07-23

**Status:** Approved

## Problem

The creator workspace currently has two competing progress models. The project list treats a terminal project as complete, while the workspace derives its six public steps only from current creator artifacts. A successful dynamic agent run can therefore appear as `6/6` in the project list but show earlier steps as `not_started` and disabled inside the project. The workspace also loads only one current artifact for the selected step, and both JSON agent output and structured artifact content fall back to an unfiltered `<pre>` block.

This makes completed work difficult to inspect, hides intermediate images, videos, Markdown, and versions, and prevents a creator from safely regenerating from a completed step.

## Goals

1. Use one backend-authoritative projection for project progress, step navigation, active work, and historical process state.
2. Keep every step with an existing attempt readable after completion.
3. Make JSON, Markdown, image, video, audio, and fallback file artifacts directly reviewable in the desktop frontend.
4. Show the complete creation process, including attempts, timestamps, current and historical artifacts, review state, and downstream freshness.
5. Regenerate from a completed step by creating a new attempt/version, preserving history, and marking downstream work stale until it is regenerated.
6. Preserve the existing six creator-facing steps and avoid exposing raw internal workflow topology as primary navigation.
7. Meet keyboard, contrast, focus, media-control, and reduced-motion accessibility expectations.

## Non-goals

- Replacing the internal agent planner or workflow engine.
- Showing every developer log line to creators.
- Migrating or deleting historical artifacts.
- Overwriting accepted artifact versions in place.
- Introducing a new external storage service or frontend rendering dependency when native React rendering is sufficient.

## Chosen Approach

Add a creator-facing audit projection to `CreationView`. The projection combines current artifacts, artifact history, durable agent-run/review summaries, and shot state into the existing six public steps. The backend remains the only authority for step status and action availability. The frontend renders this projection as a film-strip timeline and an artifact proofing workspace.

This avoids the two rejected approaches:

- A frontend-only reconstruction from agent trace would duplicate workflow semantics and reproduce status drift.
- Forcing every internal node to create a public creator artifact would expose implementation noise and require a risky data migration.

## Backend Model

### Step summary

Each `CreatorStep` gains durable audit fields:

- `hasHistory`: whether the step has any successful, reviewable, failed, or active attempt.
- `attemptCount`: number of creator-visible attempts.
- `startedAt` and `updatedAt`: stable timestamps for the visible process.
- `isStale`: whether an upstream regeneration invalidated the current result.
- `artifactCount`: number of creator-visible artifacts across current and historical attempts.

A step is readable when `hasHistory` is true, even if its current state is `not_started`. For terminal legacy projects that lack creator artifacts, successful agent nodes mapped to a public step provide the historical fallback. A terminal project is never blindly converted to six successful steps; only mapped durable evidence is projected.

### Process timeline

`CreationView.processTimeline` contains creator-visible process events ordered by durable timestamp:

- public step ID
- attempt number
- event state (`started`, `generated`, `needs_review`, `confirmed`, `failed`, `stale`)
- source type (`artifact`, `review`, `agent_node`, `shot`, `project`)
- source ID
- title and creator-safe summary
- started and completed timestamps
- artifact references

Internal prompts, hidden reasoning, credentials, tool payloads, and unrestricted logs are never included.

### Step artifacts

`CreationView.stepArtifacts` groups lightweight artifact descriptors by public step. A descriptor includes ID, name, kind, MIME type, version, attempt, current/stale flags, created time, media availability, and source lineage. Payloads remain lazy-loaded through the existing artifact content endpoint.

The projection includes current and historical creator artifacts. Agent-node outputs without a persisted artifact may appear as a safe structured timeline summary, but they are not represented as downloadable artifacts.

### Regeneration

Add a dedicated idempotent endpoint:

`POST /api/video-projects/:id/steps/:stepId/regenerations`

Request:

```json
{
  "baseArtifactId": "artifact-id-or-empty-for-legacy-step",
  "baseVersion": 3,
  "instruction": "optional creator instruction",
  "confirmedAffectedStepIds": ["shots", "preview", "delivery"]
}
```

The `Idempotency-Key` header is mandatory. The service resolves the latest durable run/review source for the step, creates or reopens a new attempt through the existing review mutation/runtime path, preserves all prior artifacts, and marks later public steps stale. It rejects stale base versions, mismatched impact confirmation, ambiguous lineage, and concurrent regeneration with `409`.

Downstream artifacts remain readable as historical evidence. They display `stale` until new successful artifacts replace them.

## Frontend Experience

### Process rail

The six-step strip becomes a compact film-strip process rail. Each step shows:

- state and freshness
- take/attempt count
- artifact count
- last update time

Any step with history is clickable. The selected route remains stable across refreshes. Generating status is polled from the backend projection; the frontend does not infer completion.

### Review workspace

The selected step has three areas:

1. A concise step header with state, attempt, freshness, and “Regenerate from this step”.
2. A primary proofing canvas for the selected artifact.
3. An artifact drawer grouped by attempt and media type, with current, historical, and stale badges.

The existing confirmation and revision controls remain available when allowed by the backend. Regeneration first shows the affected downstream steps and then starts a new attempt with an idempotency key.

### Typed artifact renderers

#### JSON

- Summary bar with root type, property/item count, nesting depth, and search matches.
- Default structured view with collapsible object keys.
- Homogeneous arrays render as a readable table when safe; nested values remain expandable.
- Search filters matching keys and scalar values.
- Raw tab provides formatted JSON, copy, and download.
- Large documents render incrementally by collapsed branches instead of expanding the full payload.

#### Markdown

- Semantic headings, paragraphs, lists, blockquotes, tables, links, and fenced code.
- Generated table of contents for documents with multiple headings.
- Preview/source tabs and copy/download controls.
- HTML in Markdown is treated as text; it is not injected into the DOM.

#### Image

- Fit, fill, 100%, zoom, and original-file actions.
- Existing rectangle selection remains available for revision instructions.
- Alt text falls back to the artifact name and step label.

#### Video and audio

- Native accessible controls, duration and media metadata, open/download actions.
- Existing time-range selection remains available for revision instructions.
- Captions are attached when a related caption artifact exists.

#### Fallback file

- File name, MIME type, size when available, version, lineage, open/download.
- Text is only previewed when the backend identifies a safe text MIME type.

### Agent review output

The pending-review panel uses the same typed artifact presentation primitives. Structured review output is summarized and expandable; it no longer dumps a whole JSON object into a raw `<pre>` by default.

### Visual and accessibility direction

The workspace keeps Tangying's warm neutral palette but adopts an editing-suite proofing surface: a dark ink process rail, restrained amber state accents, and a large low-noise review canvas. State is never represented by color alone. All controls support keyboard focus, semantic labels, visible focus rings, sufficient contrast in light and dark themes, reduced motion, and native media keyboard controls.

## State and Refresh Behavior

- Initial route load fetches one `CreationView` and lazy-loads only the selected artifact payload.
- Active project states poll the creation view using the existing bounded backoff behavior.
- Completed projects do not poll indefinitely; a successful regeneration switches them back to active polling.
- Mutations return the updated `CreationView` whenever possible.
- Abort controllers and request tokens prevent stale artifact responses from replacing a newer selection.
- A reload produces the same selected-step readability and progress because all displayed status is durable.

## Error Handling

- `409`: show that content changed and reload the authoritative view without discarding readable history.
- `404`: show an unavailable artifact card while keeping the process timeline visible.
- Media load failure: show metadata and open/download actions.
- Malformed JSON: show a readable parse warning and escaped raw text.
- Regeneration failure: retain the failed attempt in the timeline and keep the previous accepted artifact available.

## Security and Privacy

- Project ownership is checked before every aggregate, artifact, and regeneration read/write.
- Timeline summaries are allow-listed and never expose hidden reasoning, secrets, raw tool arguments, or unrestricted logs.
- Markdown HTML is not executed.
- Media URLs continue through authenticated/local artifact URL resolution.
- Download names are sanitized by the existing artifact delivery boundary.

## Testing and Acceptance Criteria

### Backend

- A terminal legacy project with successful mapped agent nodes exposes readable completed steps even when creator artifacts are missing.
- A step with artifact history is readable and lists current plus historical descriptors.
- A regeneration preserves prior versions and marks every later step stale.
- Repeated idempotency keys return the same attempt; conflicting requests return `409`.
- Cross-project reads and regenerations return `404`.
- Creator timeline output excludes restricted run data.

### Frontend

- Completed steps can be opened directly and after refresh.
- JSON opens in structured mode, supports search/collapse/table/raw/copy/download, and never defaults to an unbounded wall of text.
- Markdown renders semantically and offers source mode without executing embedded HTML.
- Image, video, and audio artifacts render in the workspace; failed media remains downloadable.
- Artifact history and stale/current badges are visible by attempt.
- Regeneration shows impact, starts a new attempt, retains old results, and updates progress without a full app restart.
- Keyboard-only users can traverse steps, artifact drawer, renderer tabs, and mutation controls.

### Release gate

- Go unit/integration tests pass for creator view, handler, artifact history, and regeneration.
- Frontend logic/component tests and production build pass.
- The installed Electron client opens the known completed demo project, shows all durable creation stages, displays its final video, and can navigate back to every completed step.
- A fresh regeneration from a completed step visibly creates a new attempt and marks downstream steps stale.

## Rollout and Compatibility

The response additions are backward-compatible JSON fields. Existing clients can ignore them. The new frontend prefers the audit fields and retains a compatibility fallback for servers without them. No destructive migration is required. Legacy projects gain better visibility from persisted evidence, while newly generated projects produce the full projection immediately.
