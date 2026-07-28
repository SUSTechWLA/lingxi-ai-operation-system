# Focused Completed Project Review Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make completed projects easy to inspect by giving requirements, direction, script, images, audio, video, Shot review, preview, and delivery the correct amount of space while showing only the latest current artifacts.

**Architecture:** Replace the permanent left artifact library with an optional compact horizontal `CreatorCurrentArtifactSwitcher`. The workspace is always single-column; the switcher appears only when a non-Shot step has at least two current reviewable artifacts. Existing purpose-built review panels remain responsible for document selection, image conversation, audio/video playback, Shot queue/inspector, version history, regeneration, and delivery recovery.

**Tech Stack:** React 19, TypeScript, CSS, existing creator-studio source/logic contract test harness, Vite/Electron.

## Global Constraints

- Preserve the current uncommitted fixes in `creatorReviewArtifacts.ts`, `logic.ts`, `ProjectWorkspacePage.tsx`, `CreatorProcessTimeline.tsx`, `index.css`, and `creator-studio-logic-check.mjs` that project only the latest current artifacts and remove duplicate progress rows.
- Do not change backend audit retention. Historical attempts remain available only through explicit history/developer diagnostics.
- Do not remove selection-based text revision, image conversation, media range revision, impact confirmation, restoration, Shot regeneration, final assembly repair, or installed video playback.
- The Shot step keeps `ShotReviewQueue` and `ShotInspector`; it is not converted into artifact tabs.
- One current artifact renders no switcher. Two or more current artifacts render exactly one switcher.
- Preserve the warm Tangying visual language and all dark-theme tokens.
- Preserve unrelated worktree changes and stage only task-specific files before each commit.

---

### Task 1: Define the compact current-artifact switcher contract

**Files:**
- Create: `frontend/src/features/creator-studio/components/CreatorCurrentArtifactSwitcher.tsx`
- Modify: `frontend/src/features/creator-studio/creatorReviewArtifacts.ts`
- Modify: `frontend/scripts/creator-studio-logic-check.mjs`

**Component contract:**

```ts
interface CreatorCurrentArtifactSwitcherProps {
  projectId: string
  artifacts: readonly CreatorReviewArtifact[]
  selectedArtifactId?: string
  onSelect: (artifact: CreatorReviewArtifact) => void
}
```

The caller passes only `currentCreatorReviewArtifacts(...)` output. The component returns `null` for zero or one artifact.

- [ ] **Step 1: Add failing projection and source-contract assertions**

In `creator-studio-logic-check.mjs`, assert:

- the switcher file exists and exports `CreatorCurrentArtifactSwitcher`;
- it returns `null` when `artifacts.length <= 1`;
- the container uses `role="tablist"` and each selectable item uses `role="tab"`, `aria-selected`, `tabIndex`, and visible labels;
- it handles `ArrowLeft`, `ArrowRight`, `Home`, and `End` without trapping focus;
- it loads only compact image metadata/previews and never mounts `<video>` or `<audio>` inside the switcher;
- media labels reuse `reviewLabel`, safe `shotLabel`, and duration when available;
- historical/stale artifact navigation is absent.

Add a pure helper in `creatorReviewArtifacts.ts`:

```ts
export function shouldShowCreatorArtifactSwitcher(
  artifacts: readonly CreatorReviewArtifact[],
): boolean {
  return artifacts.length > 1
}
```

Test false for 0/1 and true for 2 current artifacts.

- [ ] **Step 2: Verify red**

Run:

```bash
cd frontend
npm run test:creator
```

Expected: the contract fails because `CreatorCurrentArtifactSwitcher.tsx` and the helper do not exist.

- [ ] **Step 3: Implement the switcher with bounded preview work**

Render a horizontally scrollable tablist. Use roving `tabIndex`; keyboard navigation focuses the next tab and calls `onSelect`. For image tabs only, call `getCreatorArtifactContent`, resolve the local media URL with `resolveCreatorArtifactMediaUrl`, and show a compact lazy-loaded thumbnail. Text tabs show a semantic label. Audio/video tabs show a type icon and formatted duration without mounting a player.

Abort in-flight thumbnail loads on unmount or artifact change. Preview failures fall back to the semantic label and do not block selection.

- [ ] **Step 4: Verify the component contract**

Run:

```bash
cd frontend
npm run test:creator
npm run build
```

Expected: both commands exit zero.

- [ ] **Step 5: Commit only Task 1 files**

```bash
git add frontend/src/features/creator-studio/components/CreatorCurrentArtifactSwitcher.tsx frontend/src/features/creator-studio/creatorReviewArtifacts.ts frontend/scripts/creator-studio-logic-check.mjs
git commit -m "feat: add compact current artifact switcher"
```

### Task 2: Rewire the completed-project workspace to a single full-width canvas

**Files:**
- Modify: `frontend/src/features/creator-studio/ProjectWorkspacePage.tsx`
- Delete: `frontend/src/features/creator-studio/components/CreatorContentLibrary.tsx`
- Modify: `frontend/scripts/creator-studio-logic-check.mjs`

**Render structure:**

```tsx
<div className="creator-content-workspace">
  {showArtifactSwitcher && (
    <CreatorCurrentArtifactSwitcher
      projectId={projectId}
      artifacts={visibleArtifacts}
      selectedArtifactId={selectedArtifact?.artifactId}
      onSelect={selectArtifact}
    />
  )}
  <div id="creator-proofing-canvas" className="creator-proofing-canvas">
    {stepSpecificReviewPanel}
  </div>
</div>
```

- [ ] **Step 1: Change source-contract tests before production code**

Replace assertions that require `CreatorContentLibrary` with assertions that:

- `ProjectWorkspacePage` imports and renders `CreatorCurrentArtifactSwitcher`;
- switcher visibility is `!isShotsStep && !historicalShotMode && shouldShowCreatorArtifactSwitcher(visibleArtifacts)`;
- the same `selectedArtifact` still reaches `ArtifactReviewPanel` and `PreviewDeliveryPanel`;
- `ProjectBriefPanel`, `ShotReviewQueue`, `ShotInspector`, and `PreviewDeliveryPanel` remain present;
- the workspace no longer imports or renders `CreatorContentLibrary`;
- selected artifact state remains scoped by `stepId` and changing a tab does not change workflow step state.

- [ ] **Step 2: Verify red**

Run:

```bash
cd frontend
npm run test:creator
```

Expected: the workspace contract fails because the permanent library is still rendered.

- [ ] **Step 3: Replace the sidebar and preserve specialized panels**

Remove the `CreatorContentLibrary` import and aside. Compute:

```ts
const showArtifactSwitcher = !isShotsStep &&
  !historicalShotMode &&
  shouldShowCreatorArtifactSwitcher(visibleArtifacts)
```

Render the switcher immediately above the proofing canvas. Keep requirements routed to `ProjectBriefPanel`, direction/script to `ArtifactReviewPanel`, Shots to queue-plus-inspector, and preview/delivery to `PreviewDeliveryPanel`.

Delete `CreatorContentLibrary.tsx` only after `rg -n "CreatorContentLibrary" frontend/src` confirms no production imports remain.

- [ ] **Step 4: Verify all creator behaviors still compile**

Run:

```bash
cd frontend
npm run test:creator
npm run build
npm run lint
```

Expected: all commands exit zero.

- [ ] **Step 5: Commit only Task 2 files**

```bash
git add frontend/src/features/creator-studio/ProjectWorkspacePage.tsx frontend/src/features/creator-studio/components/CreatorContentLibrary.tsx frontend/scripts/creator-studio-logic-check.mjs
git commit -m "fix: give completed reviews a full width canvas"
```

### Task 3: Implement readable, responsive, and accessible styling

**Files:**
- Modify: `frontend/src/index.css`
- Modify: `frontend/scripts/creator-studio-logic-check.mjs`

**Required CSS behavior:**

```css
.creator-content-workspace {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 0.75rem;
  min-width: 0;
}

.creator-current-artifact-switcher {
  display: flex;
  max-width: 100%;
  overflow-x: auto;
  overscroll-behavior-inline: contain;
}

.artifact-review-document {
  width: min(100%, 78ch);
  margin-inline: auto;
}
```

- [ ] **Step 1: Add failing CSS regression assertions**

Assert that:

- `.creator-content-workspace` has one column at base, 760 px, and 390 px breakpoints;
- no creator breakpoint contains `minmax(240px, 280px) minmax(0, 1fr)` or `minmax(280px, 320px) minmax(0, 1fr)`;
- switcher tabs are horizontally scrollable, have visible focus, and keep `flex: 0 0 auto`;
- text documents have a 68–78ch reading measure while media/player surfaces retain full canvas width;
- reduced-motion rules include switcher transitions;
- dark theme uses tokenized foreground/background colors and introduces no hard-coded white surface.

- [ ] **Step 2: Verify red**

Run:

```bash
cd frontend
npm run test:creator
```

Expected: the CSS contract fails because the two-column workspace rules remain and switcher styles are absent.

- [ ] **Step 3: Replace library styles with switcher styles**

Remove the unused `.creator-content-library`, `.creator-content-tabs`, `.creator-content-cards`, and `.creator-content-card*` blocks. Add compact tab, thumbnail, icon, duration, selected, hover, focus, loading, and failure states under `.creator-current-artifact-switcher*`.

Keep `.creator-proofing-canvas` full width. Center `.artifact-review-document`, `.project-brief-copy`, and long script content within a readable measure; do not apply the measure to `ArtifactProofingCanvas` image, audio, or video players.

- [ ] **Step 4: Verify responsive and theme contracts**

Run:

```bash
cd frontend
npm run test:creator
npm run test:settings
npm run build
npm run lint
```

Expected: all commands exit zero.

- [ ] **Step 5: Commit only Task 3 files**

```bash
git add frontend/src/index.css frontend/scripts/creator-studio-logic-check.mjs
git commit -m "style: focus completed project review surfaces"
```

### Task 4: Verify a real completed project and package the client

**Files:**
- Modify if evidence requires: `frontend/src/features/creator-studio/ProjectWorkspacePage.tsx`
- Modify if evidence requires: `frontend/src/index.css`
- Verify: `frontend/release/Tangying-AI-Video-Creator-0.2.1-mac-arm64.dmg`

- [ ] **Step 1: Run the complete creator regression suite**

```bash
cd frontend
npm run test:creator
npm run test:settings
npm run test:developer-build
npm run build
npm run lint
cd ..
git diff --check
```

Expected: all commands exit zero.

- [ ] **Step 2: Build and verify the desktop package**

```bash
bash scripts/build-local-desktop.sh --skip-install
hdiutil verify frontend/release/Tangying-AI-Video-Creator-0.2.1-mac-arm64.dmg
codesign --verify --deep --strict --verbose=2 "frontend/release/mac-arm64/Tangying AI Video Creation Assistant.app"
```

Expected: packaging, DMG verification, and signature verification succeed.

- [ ] **Step 3: Open a real completed project at common window widths**

Use the installed client and a real completed project from “我的视频”. Inspect at approximately 1440 px, 1024 px, 760 px, and 390 px widths:

1. Requirements occupies the full review canvas and shows no artifact switcher when it has one current record.
2. Direction and script show readable Chinese paragraphs; selecting text still opens the scoped revision assistant.
3. A step with multiple current artifacts shows one compact top switcher; changing tabs changes only the proofing canvas.
4. Images render as previews and open the existing conversation/zoom flow.
5. Audio and video play in their full review panels; the switcher never mounts a competing player.
6. Shot review still shows its queue and one inspector/player.
7. Preview/delivery retains final playback, download, recovery, and “从此步骤重新生成”.
8. Historical attempts do not appear in primary counts, cards, or progress rows.
9. Keyboard navigation and dark mode retain visible focus and readable contrast.

- [ ] **Step 4: Fix only evidence-backed regressions, then rerun Steps 1–3**

For each discovered defect, add a failing assertion to `creator-studio-logic-check.mjs` before the fix. Do not broaden the layout redesign beyond the approved full-width review architecture.

- [ ] **Step 5: Record the acceptance evidence**

Record the tested project ID, completed artifact IDs, window widths, selected steps, media playback result, theme, package path, and test command results. Do not include private prompt bodies or local media bytes.
