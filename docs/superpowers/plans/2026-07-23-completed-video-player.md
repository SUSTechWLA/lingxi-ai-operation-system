# Completed Video Player Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make completed creator projects open directly into an accessible built-in video player.

**Architecture:** Add pure routing and artifact-selection policies to creator logic, then render all completed-video artifacts through one focused React player component. Existing media URL resolution and artifact fallback behavior remain unchanged.

**Tech Stack:** React 19, TypeScript, HTMLMediaElement and Fullscreen APIs, CSS, Node contract tests, Vite, Electron.

## Global Constraints

- Do not change backend artifact contracts.
- Preserve explicit artifact selection and all non-video viewers.
- Do not add a third-party player dependency.
- Keep controls keyboard accessible in both light and dark themes.

---

### Task 1: Completed-project routing and video preference

**Files:**
- Modify: `frontend/scripts/creator-studio-logic-check.mjs`
- Modify: `frontend/src/features/creator-studio/logic.ts`
- Modify: `frontend/src/features/creator-studio/VideoLibraryPage.tsx`
- Modify: `frontend/src/features/creator-studio/ProjectWorkspacePage.tsx`

**Interfaces:**
- Produces: `creatorProjectEntryStep(status: string, activeStep?: CreatorStepId): CreatorStepId`
- Extends: `selectCreatorStepArtifact(artifacts, preferredArtifactId, currentArtifactId, preferredKind?)`

- [ ] **Step 1: Write failing logic and source-contract assertions**

Add assertions proving terminal projects resolve to `preview`, active projects preserve `activeStep`, video preference outranks a JSON current artifact, and an explicit preferred artifact still wins.

- [ ] **Step 2: Run the focused suite and verify red**

Run: `npm run test:creator`

Expected: FAIL because `creatorProjectEntryStep` and kind preference are not implemented.

- [ ] **Step 3: Implement minimal pure logic and wire the UI**

Make terminal cards say “观看成片”, compute their route with `creatorProjectEntryStep`, and ask the selection helper for `VIDEO` only on preview and delivery steps.

- [ ] **Step 4: Run the focused suite and verify green**

Run: `npm run test:creator`

Expected: PASS.

### Task 2: Accessible reusable video player

**Files:**
- Create: `frontend/src/features/creator-studio/components/SimpleVideoPlayer.tsx`
- Modify: `frontend/src/features/creator-studio/components/ArtifactProofingCanvas.tsx`
- Modify: `frontend/src/features/creator-studio/components/PreviewDeliveryPanel.tsx`
- Modify: `frontend/src/index.css`
- Modify: `frontend/scripts/creator-studio-logic-check.mjs`

**Interfaces:**
- Produces: `SimpleVideoPlayer({ src, title, downloadName, onError })`

- [ ] **Step 1: Write failing source-contract assertions**

Assert that the shared component exposes play, seek, mute, volume, rate, fullscreen, download, and keyboard controls, and that both consumers import it instead of mounting their own video elements.

- [ ] **Step 2: Run the focused suite and verify red**

Run: `npm run test:creator`

Expected: FAIL because the shared component does not exist.

- [ ] **Step 3: Implement the player and styles**

Use a single `<video>` ref, native media events, semantic buttons/ranges/select, a responsive control bar, and theme variables. Replace both direct video branches with `SimpleVideoPlayer`.

- [ ] **Step 4: Run frontend verification**

Run:

```bash
npm run test:creator
npm run test:settings
npm run build
npm run lint
git diff --check
```

Expected: all commands exit zero.

### Task 3: Package and installed-app smoke test

**Files:**
- Verify only: `frontend/release/Tangying-AI-Video-Creator-0.2.1-mac-arm64.dmg`

**Interfaces:**
- Consumes: completed frontend implementation and the existing desktop packaging script.

- [ ] **Step 1: Build the desktop package**

Run: `bash scripts/build-local-desktop.sh --skip-install`

Expected: app bundle and DMG complete without signing errors.

- [ ] **Step 2: Install recoverably and launch**

Move the current app bundle to a timestamped backup, copy the new app to `/Applications`, and launch it.

- [ ] **Step 3: Exercise the completed-project flow**

Open “我的视频”, activate “观看成片”, verify the preview route, then operate play/pause and confirm seek, volume, rate, fullscreen, and download controls are present.

- [ ] **Step 4: Verify package integrity**

Run:

```bash
hdiutil verify frontend/release/Tangying-AI-Video-Creator-0.2.1-mac-arm64.dmg
codesign --verify --deep --strict --verbose=2 "/Applications/Tangying AI Video Creation Assistant.app"
```

Expected: both commands exit zero.
