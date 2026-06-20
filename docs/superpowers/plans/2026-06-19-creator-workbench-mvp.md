# Creator Workbench MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the default old publish screen with a runnable self-media video creator workbench that uses the existing skills, workflows, and video-project APIs.

**Architecture:** This MVP is frontend-only. It adds a typed compatibility layer over `/api/skills`, `/api/workflows`, and `/api/video-projects`, then renders a creator workbench with a dynamic skill library, brief form, production spine, and publish-material panel.

**Tech Stack:** React 18, TypeScript, TailwindCSS, Zustand only if needed, existing Axios client, existing Vite dev server.

## Global Constraints

- No multi-platform publishing entry in the new default experience.
- No fake image-generation API; image generation is represented as Codex `$imagegen` requests and import slots.
- No fake video-generation API; video stages produce external web video-generation material packages.
- Default first screen is `CreatorWorkbenchPage`.
- Keep old `PublishPage` source available but stop using it as the default nav target.

---

### Task 1: Typed Creator API Compatibility Layer

**Files:**
- Modify: `frontend/src/utils/types.ts`
- Modify: `frontend/src/services/api.ts`

**Interfaces:**
- Produces: `SkillRuntimeItem`, `WorkflowTemplate`, `VideoProject`, `WorkflowRun`, and API functions consumed by the creator page.

- [ ] **Step 1: Add types**

Add skill, workflow, project, and run interfaces to `frontend/src/utils/types.ts`.

- [ ] **Step 2: Add API functions**

Add `fetchSkills`, `fetchWorkflows`, `fetchVideoProjects`, `createVideoProject`, and `createWorkflowRun` to `frontend/src/services/api.ts`.

- [ ] **Step 3: Verify TypeScript**

Run: `npm run build`

Expected first run may fail until Task 2 consumes the new interfaces correctly.

### Task 2: Creator Workbench Page

**Files:**
- Create: `frontend/src/pages/CreatorWorkbenchPage.tsx`

**Interfaces:**
- Consumes: API functions from Task 1.
- Produces: a full-page creator UI that can load skills/workflows and start a compatible video project run.

- [ ] **Step 1: Create the page**

Implement a single focused page with local React state for selected skill, brief, aspect ratio, duration, run state, and loading/error state.

- [ ] **Step 2: Render four work zones**

Render function library, brief composer, production spine, and publish material panel.

- [ ] **Step 3: Start a run**

On submit, create a video project with `aigc_shot` for `aigc-shot-video`; otherwise use `voice_visual`. Then start a workflow run with the selected template id.

- [ ] **Step 4: Verify build**

Run: `npm run build`

Expected: compile errors guide any type fixes.

### Task 3: Navigation Switch

**Files:**
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/components/Sidebar.tsx`
- Modify: `frontend/src/index.css`

**Interfaces:**
- Consumes: `CreatorWorkbenchPage`.
- Produces: default `creator` nav with old publish page no longer default.

- [ ] **Step 1: Make creator default**

Set `activeNav` default to `creator` and render `CreatorWorkbenchPage`.

- [ ] **Step 2: Update sidebar copy**

Make nav items `创作台`, `作品`, `技能`, `系统`; map unimplemented `作品` and `技能` to informative placeholder panels inside the page or current app shell.

- [ ] **Step 3: Tune visual tokens**

Add creator-specific CSS utility classes only where Tailwind classes would be noisy.

- [ ] **Step 4: Final verification**

Run: `npm run build`, then open `http://localhost:3000`.

Expected: the first screen is the self-media creator workbench, not the old publish screen.
