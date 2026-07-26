# Completed Creator Review Repair Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Make the existing completed 30-second demo reopen as a readable, directly selectable, Shot-organized, media-playable review and regenerate only its first invalid downstream result.

**Architecture:** Add one creator-review projection shared by cards and detail views while keeping raw payloads in Developer Diagnostics. Use canonical UTF-16 offsets for scoped revisions, derive completed-task Shots from durable rows or historical artifact relationships, and resolve canonical local media references before playback.

**Tech Stack:** React 18, TypeScript 5.5, Vite 7, Electron 43, Node 22, Go 1.25, net/http, existing cloud/local artifact services.

## Global Constraints

- Repair the real completed 30-second task; do not fabricate a completed task.
- Never expose raw JSON, paths, hashes, providers, tool calls, or internal IDs in creator UI.
- Every displayed text artifact supports direct selection and scoped revision.
- Scoped revision preserves every unselected source byte.
- Keep task system prompts and tool definitions byte-for-byte stable; selection and instruction are the appended user delta.
- Do not regenerate confirmed upstream content solely to repair downstream media.
- Raw technical payloads remain available only in Developer Diagnostics.

---

### Task 1: Normalize historical artifacts into one creator review projection

**Files:**
- Create: frontend/src/features/creator-studio/creatorReviewProjection.ts
- Modify: frontend/src/features/creator-studio/components/CreatorContentLibrary.tsx
- Modify: frontend/src/features/creator-studio/components/JsonArtifactViewer.tsx
- Modify: frontend/src/features/creator-studio/components/ArtifactProofingCanvas.tsx
- Test: frontend/scripts/creator-studio-logic-check.mjs

**Interfaces:**
- Consumes: buildArtifactReviewModel(value: unknown): ArtifactReviewModel | null.
- Produces: projectCreatorReviewContent(content: unknown): CreatorReviewProjection and creatorReviewExcerpt(content: unknown): string.

- [ ] **Step 1: Write the failing projection test**

~~~js
const historical = JSON.stringify({ artifacts: [{
  kind: 'VIDEO_SCRIPT',
  content: JSON.stringify({ title: '30秒认识躺营', detailedScript: '真实口播正文。' }),
}] })
assert.equal(projection.creatorReviewExcerpt(historical), '真实口播正文。')
assert.equal(projection.creatorReviewExcerpt(historical).includes('{'), false)
assert.equal(projection.projectCreatorReviewContent(historical).canonicalText, '真实口播正文。')
assert.equal(projection.creatorReviewExcerpt('{"toolCall":{"name":"debug"}}'), '内容已生成，选择后可查看。')
~~~

- [ ] **Step 2: Run test and verify RED**

Run: npm run test:creator

Expected: FAIL because the projection module does not exist.

- [ ] **Step 3: Implement the bounded allow-list projector**

~~~ts
export interface CreatorReviewProjection {
  canonicalText?: string
  excerpt: string
  document: ArtifactReviewModel | null
}

export function projectCreatorReviewContent(content: unknown): CreatorReviewProjection {
  for (const candidate of creatorPayloadCandidates(content, 0)) {
    const document = buildArtifactReviewModel(candidate)
    const canonicalText = document?.script || readableScalar(candidate)
    if (canonicalText) return { canonicalText, excerpt: compactCreatorExcerpt(canonicalText), document }
  }
  return { excerpt: '内容已生成，选择后可查看。', document: null }
}
~~~

Decode strings only when valid JSON; recurse at most six levels; follow only artifacts, content, package, payload, and data; inspect at most 64 array entries. Accept only detailedScript, script, transcript, narration, prompt, description, summary, title, text, and value.

- [ ] **Step 4: Route cards and detail viewers through the projection**

Replace readableExcerpt(preview.content) with creatorReviewExcerpt(preview.content). Render the readable document or canonical text. Unknown objects show the neutral generated-content message and are never serialized in creator mode.

- [ ] **Step 5: Run focused verification**

Run: npm run test:creator && npm run build

Expected: PASS; the creator-surface scan finds no raw JSON expression.

- [ ] **Step 6: Commit**

~~~bash
git add frontend/src/features/creator-studio/creatorReviewProjection.ts frontend/src/features/creator-studio/components/CreatorContentLibrary.tsx frontend/src/features/creator-studio/components/JsonArtifactViewer.tsx frontend/src/features/creator-studio/components/ArtifactProofingCanvas.tsx frontend/scripts/creator-studio-logic-check.mjs
git commit -m "fix: project historical artifacts for creator review"
~~~

---

### Task 2: Make readable text directly selectable and revise only the selected range

**Files:**
- Create: frontend/src/features/creator-studio/components/ReviewableTextSurface.tsx
- Modify: frontend/src/features/creator-studio/components/JsonArtifactViewer.tsx
- Modify: frontend/src/features/creator-studio/components/MarkdownArtifactViewer.tsx
- Modify: frontend/src/features/creator-studio/components/TextSelectionAssistant.tsx
- Modify: frontend/src/features/creator-studio/components/ArtifactReviewPanel.tsx
- Modify: frontend/src/features/creator-studio/textSelection.ts
- Modify: cloud-backend/internal/core/artifact/revision_service.go
- Test: frontend/scripts/creator-studio-logic-check.mjs
- Test: cloud-backend/internal/core/artifact/revision_service_test.go
- Test: cloud-backend/internal/agents/video/service/creator_view_test.go

**Interfaces:**
- Consumes: CreatorReviewProjection.canonicalText, buildTextSelection, normalized selection provenance.
- Produces: selectionFromDomRange(root, source, range) and a generator path returning only selected replacement text.

- [ ] **Step 1: Write failing multi-node selection tests**

~~~js
assert.deepEqual(
  textSelection.buildTextSelectionFromLengths('开头正文结尾', [2, 2, 2], 1, 0, 1, 2),
  { kind: 'text', start: 2, end: 4, text: '正文' },
)
~~~

Also assert JSON and Markdown viewers render ReviewableTextSurface without a separate selection-mode button.

- [ ] **Step 2: Run npm run test:creator and verify RED**

Expected: FAIL because the helper and component do not exist.

- [ ] **Step 3: Implement direct selection**

~~~tsx
export default function ReviewableTextSurface({ source, surfaceRef, onSelectionChange }: Props) {
  const capture = () => {
    const selection = window.getSelection()
    const range = selection?.rangeCount === 1 ? selection.getRangeAt(0) : null
    onSelectionChange(range ? selectionFromDomRange(surfaceRef.current, source, range) : null)
  }
  return <div ref={surfaceRef} className="creator-reviewable-text" role="document" onMouseUp={capture} onKeyUp={capture}>{source}</div>
}
~~~

Use TreeWalker with SHOW_TEXT to sum preceding text lengths. Reject selections outside the root, over 4,000 UTF-16 code units, or whose rendered slice differs from source.slice(start, end).

- [ ] **Step 4: Write failing backend scoped-generation tests**

Use source 开头正文结尾, offsets 2..4, selected text 正文, and a generator returning 新文. Assert stored content is exactly 开头新文结尾; the system prompt contains neither source nor selection; the user prompt requests replacement text only.

- [ ] **Step 5: Run backend tests and verify RED**

Run: go test ./internal/core/artifact ./internal/agents/video/service

Expected: FAIL because the current generator asks for and stores a full replacement document.

- [ ] **Step 6: Implement scoped replacement with stable prompt prefix**

~~~go
systemPrompt := buildRevisionSystemPrompt(base.StageName, s.readStageInstruction(base))
userPrompt := buildSelectedRevisionUserPrompt(originalContent, selection, req.Message)
replacement, err := s.generator(ctx, systemPrompt, userPrompt, options)
data, err = spliceUTF16Selection(originalContent, selection, replacement)
~~~

The user prompt includes at most 320 UTF-16 units before and after the selection and never repeats system instructions or tool definitions. spliceUTF16Selection revalidates the selected slice and preserves prefix and suffix exactly. Keep full-document revision only when no text selection exists.

- [ ] **Step 7: Run all focused checks**

~~~bash
npm run test:creator
npm run build
go test ./internal/core/artifact ./internal/agents/video/service
~~~

Expected: PASS; repeated scoped revisions use the identical system-prompt string and only vary the appended user delta.

- [ ] **Step 8: Commit**

~~~bash
git add frontend/src/features/creator-studio/components/ReviewableTextSurface.tsx frontend/src/features/creator-studio/components/JsonArtifactViewer.tsx frontend/src/features/creator-studio/components/MarkdownArtifactViewer.tsx frontend/src/features/creator-studio/components/TextSelectionAssistant.tsx frontend/src/features/creator-studio/components/ArtifactReviewPanel.tsx frontend/src/features/creator-studio/textSelection.ts frontend/scripts/creator-studio-logic-check.mjs cloud-backend/internal/core/artifact/revision_service.go cloud-backend/internal/core/artifact/revision_service_test.go cloud-backend/internal/agents/video/service/creator_view_test.go
git commit -m "fix: revise only selected creator text"
~~~

---

### Task 3: Show completed Shots and recover historical Shot grouping

**Files:**
- Create: frontend/src/features/creator-studio/completedShotProjection.ts
- Modify: frontend/src/features/creator-studio/logic.ts
- Modify: frontend/src/features/creator-studio/ProjectWorkspacePage.tsx
- Modify: frontend/src/features/creator-studio/components/ShotReviewQueue.tsx
- Modify: frontend/src/features/creator-studio/components/ShotInspector.tsx
- Test: frontend/scripts/creator-studio-logic-check.mjs
- Test: cloud-backend/internal/agents/video/service/shot_review_test.go

**Interfaces:**
- Consumes: project status, stepArtifacts.shots, relatedShotId, and GET /video-projects/:id/shots.
- Produces: initialShotFilters(projectStatus) and projectHistoricalShots(artifacts).

- [ ] **Step 1: Write failing completed-task Shot tests**

~~~js
assert.deepEqual(logic.initialShotFilters('completed'), { status: 'all' })
assert.deepEqual(logic.initialShotFilters('active'), { status: 'needs_attention' })
assert.deepEqual(completedShots.projectHistoricalShots([
  { artifactId: 'a', relatedShotId: 'shot-01', reviewCategory: 'text', reviewLabel: '视频提示词' },
  { artifactId: 'b', relatedShotId: 'shot-01', reviewCategory: 'video', reviewLabel: '合成视频' },
])[0].artifactIds, ['a', 'b'])
~~~

- [ ] **Step 2: Run npm run test:creator and verify RED**

Expected: FAIL because completed filters and historical grouping do not exist.

- [ ] **Step 3: Implement completed-task filter initialization**

After CreationView loads, initialize completed or archived projects with status all; active projects retain needs_attention. Never overwrite a user-changed filter.

- [ ] **Step 4: Implement historical grouping**

When the durable Shot endpoint returns zero, group creator-visible artifacts whose relatedShotId matches shot[-_ ]?[0-9]{1,4}. Sort numerically. Show IP A-roll, text layer, supporting/AIGC layer, voice, and output labels, mark the view 历史任务回看, and never show internal IDs or storage references.

- [ ] **Step 5: Verify filter semantics**

~~~bash
npm run test:creator
npm run build
go test ./internal/agents/video/service -run 'Test.*Shot.*Page|Test.*Shot.*Summary'
~~~

Expected: PASS; approved Shots appear under all and confirmed.

- [ ] **Step 6: Commit**

~~~bash
git add frontend/src/features/creator-studio/completedShotProjection.ts frontend/src/features/creator-studio/logic.ts frontend/src/features/creator-studio/ProjectWorkspacePage.tsx frontend/src/features/creator-studio/components/ShotReviewQueue.tsx frontend/src/features/creator-studio/components/ShotInspector.tsx frontend/scripts/creator-studio-logic-check.mjs cloud-backend/internal/agents/video/service/shot_review_test.go
git commit -m "fix: show completed and historical shots"
~~~

---

### Task 4: Resolve canonical local media and distinguish missing delivery

**Files:**
- Modify: local-backend/internal/localagent/server.go
- Modify: local-backend/internal/localagent/server_test.go
- Modify: frontend/src/features/creator-studio/components/ArtifactProofingCanvas.tsx
- Modify: frontend/src/features/creator-studio/components/SimpleVideoPlayer.tsx
- Modify: frontend/src/features/creator-studio/components/PreviewDeliveryPanel.tsx
- Modify: frontend/src/features/creator-studio/logic.ts
- Test: frontend/scripts/creator-studio-logic-check.mjs

**Interfaces:**
- Consumes: local media URLs containing path or storageRef.
- Produces: resolveLocalProjectMedia(projectID, storageRef, absolutePath) and states loading, playable, missing, unsupported, service_unavailable.

- [ ] **Step 1: Write failing media tests**

Store artifact video-1, issue a HEAD request to /api/local/media with projectId vp-1 and storageRef local://projects/vp-1/artifacts/video-1/hash/final.mp4. Assert 200, video/mp4, Accept-Ranges bytes, and correct length. GET with Range bytes=0-3 returns 206. Mismatched project and unknown artifact return 400 and 404.

- [ ] **Step 2: Run go test ./internal/localagent -run TestLocalProjectMedia and verify RED**

Expected: FAIL with 400 because storageRef is ignored and absolute path is required.

- [ ] **Step 3: Implement canonical local reference resolution**

Accept exactly one of path or storageRef. Resolve artifact references to localArtifactPaths(projectID, artifactID).content and render references beneath the project root. Validate safe segments and reject traversal. Prefer stored MIME metadata when the physical content file has no extension. Continue using http.ServeContent for HEAD and byte ranges.

- [ ] **Step 4: Add explicit creator media states**

Map 404 to 成片文件缺失，可从成片步骤重新生成; connection failure to 本地媒体服务未启动; browser decode failure after a successful response to 当前编码不受客户端支持，需要转为 H.264/AAC MP4. Show a confirmed final badge only after loadedmetadata.

- [ ] **Step 5: Verify and commit**

~~~bash
go test ./internal/localagent
npm run test:creator
npm run build
git add local-backend/internal/localagent/server.go local-backend/internal/localagent/server_test.go frontend/src/features/creator-studio/components/ArtifactProofingCanvas.tsx frontend/src/features/creator-studio/components/SimpleVideoPlayer.tsx frontend/src/features/creator-studio/components/PreviewDeliveryPanel.tsx frontend/src/features/creator-studio/logic.ts frontend/scripts/creator-studio-logic-check.mjs
git commit -m "fix: restore local completed video playback"
~~~

---

### Task 5: Reopen and repair the completed task in place

**Files:**
- Modify: frontend/src/features/creator-studio/ProjectsHomePage.tsx
- Modify: frontend/src/features/creator-studio/components/PreviewDeliveryPanel.tsx
- Modify: frontend/src/features/creator-studio/logic.ts
- Test: frontend/scripts/creator-studio-logic-check.mjs

**Interfaces:**
- Consumes: completed card, CreationView steps, media availability, and existing regeneration APIs.
- Produces: completedTaskLandingStep(view, availability) and completedRepairScope(view, availability).

- [ ] **Step 1: Write failing landing and repair tests**

~~~js
assert.equal(logic.completedTaskLandingStep(completedView, { delivery: 'playable' }), 'delivery')
assert.equal(logic.completedTaskLandingStep(completedView, { delivery: 'missing', preview: 'playable' }), 'delivery')
assert.deepEqual(logic.completedRepairScope(completedView, { delivery: 'missing' }), { stepId: 'delivery', preserveUpstream: true })
~~~

- [ ] **Step 2: Run npm run test:creator and verify RED**

Expected: FAIL because the helpers do not exist.

- [ ] **Step 3: Implement recovery UX**

Keep 查看成片 routed to delivery. If delivery is missing or unsupported, show the diagnosis and one 重新生成成片 action using the current delivery base artifact and confirmed affected steps. Do not restart requirements, direction, script, or Shots. Refresh durable task progress after queuing.

- [ ] **Step 4: Verify and commit**

~~~bash
npm run test:creator
npm run build
git add frontend/src/features/creator-studio/ProjectsHomePage.tsx frontend/src/features/creator-studio/components/PreviewDeliveryPanel.tsx frontend/src/features/creator-studio/logic.ts frontend/scripts/creator-studio-logic-check.mjs
git commit -m "fix: repair completed delivery in place"
~~~

---

### Task 6: Regenerate and verify the real demo in the packaged client

**Files:**
- Modify only when verification proves a defect: scripts/one-click-deploy.sh
- Modify only when verification proves a defect: scripts/build-local-desktop.sh
- Create: docs/verification/2026-07-26-completed-demo-review.md

**Interfaces:**
- Consumes: the newest completed task whose title starts with 制作一条30秒、16:9横屏的产品功能口播Demo.
- Produces: packaged macOS client and a secret-free verification report with creator artifact counts, Shot count, delivery version, MIME/container, duration, and outcomes.

- [ ] **Step 1: Run the complete automated suite**

Run frontend tests/build/lint in frontend, cloud Go tests in cloud-backend, and local Go tests in local-backend:

~~~bash
npm run test:creator
npm run test:settings
npm run test:developer-build
npm run build
npm run lint
go test ./...
~~~

Expected: all PASS.

- [ ] **Step 2: Start and verify both services**

~~~bash
bash scripts/one-click-deploy.sh up --skip-install --skip-package
bash scripts/start-local-backend.sh
~~~

Require HTTP 200 from http://127.0.0.1:8080/health and http://127.0.0.1:8787/api/local/health.

- [ ] **Step 3: Open the real completed task**

In 已完成与归档, select the newest completed task whose title has the prefix above. Keep its actual ID only in transient test logs.

- [ ] **Step 4: Regenerate only broken delivery**

Open 查看成片. If media is missing or unsupported, invoke 重新生成成片, preserve upstream content, wait for the durable task, refresh, and verify the current delivery artifact version increased.

- [ ] **Step 5: Perform creator-facing acceptance**

Verify: no raw JSON; direct text selection opens the popover; only the selection changes; at least one real or historical Shot appears with creator layers; available images enlarge; voice and clips play; final video loads metadata, plays, seeks, changes volume/rate, enters full screen, and downloads; Developer Diagnostics retains technical payloads.

- [ ] **Step 6: Build and launch macOS client**

Run bash scripts/build-local-desktop.sh, place the app in the existing frontend/release/mac-arm64 delivery location, and repeat navigation plus playback once.

- [ ] **Step 7: Write verification report and commit**

Write docs/verification/2026-07-26-completed-demo-review.md with commands, outcomes, artifact counts, Shot count, regenerated delivery version, duration, and remaining non-blocking limitations.

~~~bash
git add docs/verification/2026-07-26-completed-demo-review.md
git commit -m "test: verify completed demo review flow"
~~~

Include deployment scripts only when verification required changes.

---

## Self-review record

- Spec coverage: readable projection, direct selection, scoped replacement, stable prompt prefix, completed Shots, local media, real-task regeneration, packaged-client verification, and diagnostics separation each have a task.
- Placeholder scan: no deferred markers or unspecified validation steps.
- Type consistency: public helpers are introduced before consumers.

