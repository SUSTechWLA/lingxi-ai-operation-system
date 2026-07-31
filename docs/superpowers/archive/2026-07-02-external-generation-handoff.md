# External Generation Handoff Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the user-side video workstation easier for real users who generate images or video in external model websites and upload the results back.

**Architecture:** Keep the no-API external generation boundary. Add pure formatting helpers in `frontend/src/pages/directorStudioLogic.ts` so copyable handoff packages are testable, then reuse those helpers in `frontend/src/pages/DirectorStudioPage.tsx` for the shot workbench and artifact dependency panel.

**Tech Stack:** React, TypeScript, Vite, existing Node script test `frontend/scripts/director-studio-logic-check.mjs`.

## Global Constraints

- Work first on `develop_go`.
- Release MR must contain core code only; this plan document must not be merged to `release`.
- Do not configure provider API keys for end users.
- Keep external model handoff copyable and low-friction.
- Use TDD: write failing behavior tests before production code.

---

### Task 1: Copyable External Generation Package

**Files:**
- Modify: `frontend/scripts/director-studio-logic-check.mjs`
- Modify: `frontend/src/pages/directorStudioLogic.ts`

**Interfaces:**
- Produces: `buildExternalGenerationCopyPackage(request, options?)`, `externalGenerationReferenceCopyText(reference, index?)`
- Consumes: request objects shaped like existing `ExternalGenerationRequestContent`

- [ ] **Step 1: Write the failing test**

Add assertions that a video request package includes request id, shot id, target settings, Prompt, Negative Prompt, reference roles, reference storage refs, and upload slot language.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npm run test:director`
Expected: FAIL because `buildExternalGenerationCopyPackage` is not exported.

- [ ] **Step 3: Write minimal implementation**

Add exported helper types and formatting functions to `directorStudioLogic.ts`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend && npm run test:director`
Expected: PASS.

### Task 2: Shot Workbench Handoff UI

**Files:**
- Modify: `frontend/src/pages/DirectorStudioPage.tsx`

**Interfaces:**
- Consumes: `buildExternalGenerationCopyPackage`, `externalGenerationReferenceCopyText`

- [ ] **Step 1: Write failing source-level smoke assertions**

Extend `frontend/scripts/director-studio-logic-check.mjs` to require UI strings for `复制生成包`, `复制负面提示`, `复制参考信息`, and `外部生成交付单`.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npm run test:director`
Expected: FAIL because the UI strings are absent.

- [ ] **Step 3: Write minimal implementation**

Update shot request cards and artifact dependency panels to show the full handoff package, negative prompt copy action, reference copy cards, and clearer three-step status text.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend && npm run test:director`
Expected: PASS.

### Task 3: Verification And Release MR Prep

**Files:**
- Modify: no new docs in release branch

**Interfaces:**
- Consumes: develop branch implementation
- Produces: release PR containing only core code

- [ ] **Step 1: Run full relevant verification**

Run: `cd frontend && npm run build`
Run: `cd cloud-backend && go test ./...`
Run: `cd local-backend && go test ./...`

- [ ] **Step 2: Prepare release branch**

Create a release-targeted branch from `release` and apply only core files:
`frontend/src/pages/DirectorStudioPage.tsx`, `frontend/src/pages/directorStudioLogic.ts`, and any required tests if release already contains the test harness.

- [ ] **Step 3: Verify release branch**

Run the same build/test commands on the release branch.

- [ ] **Step 4: Commit, push, and open MR**

Open a PR to `release` and confirm no docs or reference files are included.
