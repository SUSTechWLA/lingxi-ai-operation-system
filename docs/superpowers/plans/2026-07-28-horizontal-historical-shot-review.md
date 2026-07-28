# Horizontal Historical Shot Review Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Place the completed-project Shot selector in a horizontal strip above a full-width, readable review dossier with collapsible long historical text.

**Architecture:** Preserve the existing historical artifact projection and selection state. Add historical-mode presentation hooks at the workspace and queue boundaries, then use native `details` disclosures inside the inspector so the exact retained content remains accessible without forcing text walls.

**Tech Stack:** React 18, TypeScript, CSS, Node contract checks, Electron packaging.

## Global Constraints

- Do not change historical artifact data, API contracts, or regeneration behavior.
- Do not change the active-project Shot queue layout.
- Keep the existing warm Creator Studio visual system.
- Keep images, video, audio, keyboard focus, and truthful missing-media states available.
- No breakpoint may restore the historical left sidebar.

---

### Task 1: Lock the historical layout contract

**Files:**
- Modify: `frontend/scripts/creator-studio-logic-check.mjs`
- Test: `frontend/scripts/creator-studio-logic-check.mjs`

**Interfaces:**
- Consumes: source strings already loaded as `workspaceSource`, `queueSource`, `inspectorSource`, and `creatorStylesSource`.
- Produces: regression checks for the historical workspace modifier, horizontal filmstrip, current-shot state, and long-text disclosures.

- [ ] **Step 1: Write failing source-contract checks**

Add assertions alongside the existing historical Shot checks:

```js
assert.match(workspaceSource, /shot-review-workspace\$\{historicalShotMode \? ' is-historical' : ''\}/)
assert.match(queueSource, /historical-shot-filmstrip/)
assert.match(queueSource, /aria-current=\{selectedHistoricalShotId === shot\.id \? 'true' : undefined\}/)
assert.match(inspectorSource, /historical-shot-disclosure/)
assert.match(inspectorSource, /查看完整内容/)
assert.match(creatorStylesSource, /\.shot-review-workspace\.is-historical\s*\{[^}]*grid-template-columns:\s*minmax\(0,\s*1fr\)/s)
assert.match(creatorStylesSource, /\.historical-shot-filmstrip\s*\{[^}]*overflow-x:\s*auto/s)
```

- [ ] **Step 2: Run the focused contract test and verify failure**

Run: `npm run test:creator`

Expected: FAIL because the historical mode modifier, filmstrip, disclosures, and CSS rules are not implemented yet.

- [ ] **Step 3: Commit only after Tasks 2 and 3 make these checks pass**

The test and implementation form one user-visible layout change and will be committed together after Task 3.

---

### Task 2: Restructure historical Shot presentation

**Files:**
- Modify: `frontend/src/features/creator-studio/ProjectWorkspacePage.tsx`
- Modify: `frontend/src/features/creator-studio/components/ShotReviewQueue.tsx`
- Modify: `frontend/src/features/creator-studio/components/ShotInspector.tsx`
- Test: `frontend/scripts/creator-studio-logic-check.mjs`

**Interfaces:**
- Consumes: `historicalShotMode`, `historicalShots`, `selectedHistoricalShotId`, `HistoricalShotReview`, and the existing selection callbacks.
- Produces: `.shot-review-workspace.is-historical`, `.historical-shot-filmstrip`, and reusable `HistoricalTextDisclosure` markup.

- [ ] **Step 1: Mark only the completed-project workspace as historical**

Change the workspace wrapper to:

```tsx
<div className={`shot-review-workspace${historicalShotMode ? ' is-historical' : ''}`}>
```

This lets CSS change historical layout without affecting the live Shot queue.

- [ ] **Step 2: Convert the historical queue to a filmstrip**

In the historical branch of `ShotReviewQueue`, keep the heading and selection callback but add explicit presentation classes:

```tsx
<aside className="shot-review-queue historical-shot-selector" aria-label="历史 Shot 回看">
  <div className="shot-review-queue-heading">...</div>
  <nav className="shot-review-history-list historical-shot-filmstrip" aria-label="选择要回看的 Shot">
    {historicalShots.map(shot => (
      <button
        className={`historical-shot-chip${selectedHistoricalShotId === shot.id ? ' is-selected' : ''}`}
        aria-current={selectedHistoricalShotId === shot.id ? 'true' : undefined}
        onClick={() => onSelectHistorical(shot.id)}
      >...</button>
    ))}
  </nav>
</aside>
```

Each chip shows `Shot N`, `旁白 · 画面 · 媒体`, and `正在查看` only for the selected shot.

- [ ] **Step 3: Add a native disclosure for exact long text**

Add this focused component to `ShotInspector.tsx`:

```tsx
function HistoricalTextDisclosure({ children, label = '查看完整内容' }: { children: string; label?: string }) {
  return (
    <details className="historical-shot-disclosure">
      <summary>{label}</summary>
      <p>{children}</p>
    </details>
  )
}
```

Use a clamped preview plus disclosure for narration and each layer summary. Do not truncate or transform the disclosure text.

- [ ] **Step 4: Keep compact text readable and safely wrapped**

Render narration and layer summaries with a shared `.historical-shot-text-preview` class. Add a `title` only where it aids browser inspection; the complete text remains in the disclosure.

- [ ] **Step 5: Run the focused contract test**

Run: `npm run test:creator`

Expected: layout source checks still fail until Task 3 adds the required CSS; existing creator behavior checks remain green.

---

### Task 3: Build the full-width responsive review desk

**Files:**
- Modify: `frontend/src/index.css`
- Test: `frontend/scripts/creator-studio-logic-check.mjs`

**Interfaces:**
- Consumes: `.shot-review-workspace.is-historical`, `.historical-shot-selector`, `.historical-shot-filmstrip`, `.historical-shot-chip`, `.historical-shot-text-preview`, and `.historical-shot-disclosure`.
- Produces: a full-width historical layout, horizontal Shot selector, readable content grids, and mobile behavior.

- [ ] **Step 1: Make historical mode a one-column workspace**

Add:

```css
.shot-review-workspace.is-historical {
  grid-template-columns: minmax(0, 1fr);
}
.shot-review-workspace.is-historical .shot-review-detail {
  width: 100%;
}
```

- [ ] **Step 2: Style the horizontal filmstrip**

Add a grid-auto-flow column strip with `overflow-x: auto`, `scroll-snap-type: x proximity`, and a `minmax(9.5rem, 1fr)` auto column. Selected chips use the current primary color as a top rule and soft fill; focus uses `:focus-visible`.

- [ ] **Step 3: Improve dossier reading geometry**

Use a four-column detail grid at wide widths, a two-column grid at medium widths, and one column on narrow screens. Keep paragraph line-height between `1.6` and `1.75`, set `overflow-wrap: anywhere` for retained machine-style identifiers, and remove fixed minimum heights from layer cards.

- [ ] **Step 4: Add clamped previews and disclosure styling**

Apply a four-line clamp to `.historical-shot-text-preview`. Style `.historical-shot-disclosure summary` as a compact text action and keep the expanded paragraph fully wrapped. Hide the preview when its adjacent disclosure is open only if the result remains understandable; otherwise keep both with the disclosure containing the exact source text.

- [ ] **Step 5: Add responsive rules**

At the existing narrow breakpoint, retain the horizontal filmstrip, reduce chip width, collapse detail/layer grids, and keep the dossier full width.

- [ ] **Step 6: Run the focused test and commit**

Run: `npm run test:creator`

Expected: PASS.

Commit:

```bash
git add frontend/scripts/creator-studio-logic-check.mjs frontend/src/features/creator-studio/ProjectWorkspacePage.tsx frontend/src/features/creator-studio/components/ShotReviewQueue.tsx frontend/src/features/creator-studio/components/ShotInspector.tsx frontend/src/index.css
git commit -m "fix(creator): redesign historical shot review layout"
```

---

### Task 4: Verify, package, and inspect the real completed project

**Files:**
- Verify: `frontend/release/Tangying-AI-Video-Creator-0.2.1-mac-arm64.dmg`
- Verify: `/Applications/Tangying AI Video Creation Assistant.app`

**Interfaces:**
- Consumes: the completed layout and existing desktop build script.
- Produces: a tested DMG and installed client showing the real historical project correctly.

- [ ] **Step 1: Run frontend regression checks**

Run:

```bash
cd frontend
npm run test:creator
npm run lint
npm run build
```

Expected: all commands exit `0`.

- [ ] **Step 2: Run the relevant backend artifact test**

Run: `cd cloud-backend && go test ./internal/core/artifact -count=1`

Expected: PASS; the presentation-only change does not alter hydration behavior.

- [ ] **Step 3: Build the desktop package**

Run: `bash scripts/build-local-desktop.sh --skip-install`

Expected: Electron builder produces the arm64 DMG without errors.

- [ ] **Step 4: Install and launch safely**

Quit the running client, move the previous exact `/Applications/Tangying AI Video Creation Assistant.app` into a unique temporary backup directory, copy the packaged app into `/Applications`, and launch it through Computer Use.

- [ ] **Step 5: Verify the real completed project**

Open `我的视频`, select the completed 30-second demo project, enter `分镜与素材`, and verify the five-shot filmstrip, full-width dossier, readable previews/disclosures, media states, and narrow-window response.

- [ ] **Step 6: Verify post-launch package integrity**

Run:

```bash
codesign --verify --deep --strict --verbose=2 "/Applications/Tangying AI Video Creation Assistant.app"
shasum -a 256 "frontend/release/Tangying-AI-Video-Creator-0.2.1-mac-arm64.dmg"
git diff --check
git status --short
```

Expected: the installed app remains valid after launch, a DMG checksum is printed, the diff check is clean, and only intentional files are modified.
