# Biaoshu Artifact Actions Visible Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the Biaoshu artifact table so the action buttons, especially the entry used to view and modify artifacts, are always visible.

**Architecture:** Keep the current table-based desktop layout, but make column sizing explicit and keep the actions column pinned to the right. Long local Windows paths should be truncated inside a bounded column instead of pushing the action buttons out of the visible area.

**Tech Stack:** React, TypeScript, Tailwind CSS, Vite frontend.

---

## Problem Summary

The screenshot shows the artifact table's action area clipped on the far right. In `frontend/src/pages/BiaoshuWorkbench.tsx`, `BiaoshuArtifactTable` renders a full-width table with unconstrained table layout. The `路径` column contains long local paths such as `E:\lingxi\tangying-ai-operation-system\...`; even with `truncate`, table layout can allocate too much width to earlier columns and push the second action button out of the card.

The visible button text is currently `查看`, but the artifact edit flow is opened from that view dialog. The button should communicate this by using `查看/修改`.

## File Structure

- Modify: `frontend/src/pages/BiaoshuWorkbench.tsx`
  - Update `BiaoshuArtifactTable`.
  - Add explicit column widths via `colgroup`.
  - Wrap the table in an `overflow-x-auto` container.
  - Pin the operation column with `sticky right-0`.
  - Rename the view button label to `查看/修改`.
- Verify: `frontend/scripts/biaoshu-artifact-logic-check.mjs`
  - Existing script should still pass after the UI-only change.
- Verify: frontend build
  - `npm run build`.

---

### Task 1: Add A Stable Table Layout For Artifact Rows

**Files:**
- Modify: `frontend/src/pages/BiaoshuWorkbench.tsx`

- [ ] **Step 1: Locate the current table component**

Open `frontend/src/pages/BiaoshuWorkbench.tsx` and find:

```tsx
function BiaoshuArtifactTable({ artifacts, onView }: { artifacts: BiaoshuArtifactRecord[]; onView: (a: BiaoshuArtifactRecord) => void }) {
  const headers = ['ID', '名称', '类型', '状态', '负责人', '路径', '操作']
```

Expected: This component renders the table shown in the screenshot.

- [ ] **Step 2: Replace the table wrapper and table element**

Replace:

```tsx
return (
  <section className="card overflow-hidden p-0">
    <table className="w-full text-left text-sm">
```

with:

```tsx
return (
  <section className="card overflow-hidden p-0">
    <div className="overflow-x-auto">
      <table className="min-w-[1120px] w-full table-fixed text-left text-sm">
        <colgroup>
          <col className="w-[19%]" />
          <col className="w-[19%]" />
          <col className="w-[8%]" />
          <col className="w-[8%]" />
          <col className="w-[8%]" />
          <col className="w-[27%]" />
          <col className="w-[160px]" />
        </colgroup>
```

Expected: The table keeps a stable minimum width and gets horizontal scroll only when the viewport is too narrow.

- [ ] **Step 3: Close the new scroll wrapper**

At the end of the component, replace:

```tsx
      </table>
    </section>
  )
}
```

with:

```tsx
      </table>
    </div>
  </section>
)
}
```

Expected: JSX remains balanced with `section > div > table`.

- [ ] **Step 4: Run a TypeScript syntax check through build**

Run:

```bash
cd frontend
npm run build
```

Expected: Build succeeds. If it fails with a JSX closing-tag error, recheck the wrapper closing tags from Step 3.

---

### Task 2: Pin The Operation Column And Bound Long Paths

**Files:**
- Modify: `frontend/src/pages/BiaoshuWorkbench.tsx`

- [ ] **Step 1: Make the operation header sticky**

Replace the generic header render:

```tsx
{headers.map((header) => (
  <th className="whitespace-nowrap px-4 py-3" key={header}>{header}</th>
))}
```

with:

```tsx
{headers.map((header) => (
  <th
    className={`whitespace-nowrap px-4 py-3 ${header === '操作' ? 'sticky right-0 z-20 bg-background-mist' : ''}`}
    key={header}
  >
    {header}
  </th>
))}
```

Expected: The `操作` header stays visible on the right side when the table scrolls horizontally.

- [ ] **Step 2: Bound the path cell**

Replace:

```tsx
<td className="max-w-sm truncate px-4 py-3 font-mono text-xs text-ink-muted" title={artifact.storageRef}>{artifact.storageRef || '-'}</td>
```

with:

```tsx
<td className="px-4 py-3 font-mono text-xs text-ink-muted" title={artifact.storageRef}>
  <div className="truncate">{artifact.storageRef || '-'}</div>
</td>
```

Expected: Path text truncates inside the explicit path column instead of influencing overall table width.

- [ ] **Step 3: Make the operation cell sticky and visually separated**

Replace:

```tsx
<td className="whitespace-nowrap px-4 py-3">
  <div className="flex items-center gap-1.5">
```

with:

```tsx
<td className="sticky right-0 z-10 w-[160px] whitespace-nowrap bg-white/95 px-4 py-3 shadow-[-10px_0_18px_-18px_rgba(0,0,0,0.35)]">
  <div className="flex items-center justify-end gap-1.5">
```

Expected: Action buttons stay visible on the right edge, and the subtle left shadow shows that this is a pinned column.

- [ ] **Step 4: Rename the view button to expose the modify entry**

Replace:

```tsx
<FiEye /> 查看
```

with:

```tsx
<FiEye /> 查看/修改
```

Expected: Users understand that the modification flow starts from this button.

- [ ] **Step 5: Run frontend build**

Run:

```bash
cd frontend
npm run build
```

Expected: Build succeeds.

---

### Task 3: Add A Compact Small-Screen Fallback

**Files:**
- Modify: `frontend/src/pages/BiaoshuWorkbench.tsx`

- [ ] **Step 1: Let action buttons wrap on narrow cells**

In the sticky operation cell, replace:

```tsx
<div className="flex items-center justify-end gap-1.5">
```

with:

```tsx
<div className="flex flex-wrap items-center justify-end gap-1.5">
```

Expected: If localized text or browser zoom makes the action area tight, buttons wrap within the 160px operation column instead of being clipped.

- [ ] **Step 2: Shorten copy button label only if wrapping is still visually cramped**

If the 160px sticky column is still too tight after Step 1, change the copy button call from:

```tsx
<BiaoshuCopyButton value={biaoshuArtifactToCopyText(artifact)} label="复制" />
```

to:

```tsx
<BiaoshuCopyButton value={biaoshuArtifactToCopyText(artifact)} label="复制" />
```

Expected: No code change is needed if the current two-character label fits. Keep the existing label because it is already short.

- [ ] **Step 3: Verify visual behavior manually**

Start or use the existing frontend dev server, then open the Biaoshu workbench artifacts tab.

Manual checks:

- At desktop width, the `操作` column is fully visible.
- `复制` and `查看/修改` are both visible for valid artifacts.
- Long `E:\lingxi\...` paths are truncated in the path column.
- Horizontal scrolling does not hide the operation column.
- At browser zoom 125%, the operation buttons remain reachable.

Expected: The screenshot issue no longer reproduces.

---

### Task 4: Regression Checks

**Files:**
- Verify: `frontend/scripts/biaoshu-artifact-logic-check.mjs`
- Verify: `frontend/src/pages/BiaoshuWorkbench.tsx`

- [ ] **Step 1: Run existing Biaoshu artifact logic check**

Run:

```bash
cd frontend
node scripts/biaoshu-artifact-logic-check.mjs
```

Expected:

```text
biaoshuArtifactLogic tests passed
```

- [ ] **Step 2: Run frontend build**

Run:

```bash
cd frontend
npm run build
```

Expected: Build succeeds without TypeScript or Vite errors.

- [ ] **Step 3: Inspect diff**

Run:

```bash
git diff -- frontend/src/pages/BiaoshuWorkbench.tsx
```

Expected: Diff is limited to `BiaoshuArtifactTable` layout and button label changes.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/pages/BiaoshuWorkbench.tsx
git commit -m "fix: keep biaoshu artifact actions visible"
```

Expected: Commit succeeds.

---

## Acceptance Criteria

- The operation column is always visible in the artifact table.
- The `查看/修改` button is visible for every valid artifact with a `storageRef`.
- Long local file paths do not push buttons out of the card.
- Horizontal scrolling, when needed, scrolls table content while preserving the pinned action column.
- `npm run build` passes in `frontend`.
- Existing `biaoshu-artifact-logic-check.mjs` still passes.

## Rollback Plan

If sticky table cells render poorly in the Electron shell, revert only the sticky classes and keep the `overflow-x-auto`, `table-fixed`, and `colgroup` changes. That fallback still prevents clipping by allowing horizontal scrolling, though the operation column will no longer remain pinned.

If the 160px action column is too narrow for localized text, increase the last `colgroup` width from `w-[160px]` to `w-[190px]` and keep the path column at `w-[24%]` instead of `w-[27%]`.

## Self-Review

- Spec coverage: The plan fixes the hidden modify/view action, long path pressure, and narrow viewport behavior.
- Placeholder scan: No unspecified placeholders remain.
- Type consistency: The only affected function is `BiaoshuArtifactTable`, and the existing `onView` callback remains unchanged.
