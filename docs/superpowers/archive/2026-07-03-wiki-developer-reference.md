# Wiki Developer Reference Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add module-level developer reference pages to the GitHub Wiki and link them from the main Wiki pages.

**Architecture:** Keep `Home.md` and `English.md` focused on product introduction. Add `Developer-Guide.md` as the developer entry point, then create one focused Wiki page per major module with exact code paths and verification commands.

**Tech Stack:** GitHub Wiki Markdown, Mermaid diagrams, Go backend, React/Electron frontend, local Go agent, MCP/Dreamina CLI integration.

## Global Constraints

- Only update GitHub Wiki content and develop_go planning records.
- Do not modify core application code.
- Do not modify or push release.
- Use Chinese as the primary language for developer reference pages, with exact English technical names preserved.
- Every new Wiki page must link back to `Developer-Guide` and `Home`.

---

### Task 1: Developer Entry Page

**Files:**
- Create: `/Users/wanglian/Projects/tangying-ai-operation-system.wiki/Developer-Guide.md`

**Interfaces:**
- Consumes: Existing `Home.md`, `English.md`, `_Sidebar.md`.
- Produces: `Developer-Guide` link target for all module pages.

- [ ] **Step 1: Create the page**

Write `Developer-Guide.md` with:
- purpose and audience
- repository map
- recommended reading order
- local startup commands
- full verification commands
- links to the six module pages

- [ ] **Step 2: Verify links**

Run:

```bash
cd /Users/wanglian/Projects/tangying-ai-operation-system.wiki
rg -n "Frontend-Desktop|Cloud-Backend-Agent-Runtime|Local-Backend-Runner-MCP|Video-Creation-Workflows|JiMeng-MCP-Integration|Data-Artifacts-Review-Gates" Developer-Guide.md
```

Expected: all six module links appear.

### Task 2: Core Module Pages

**Files:**
- Create: `/Users/wanglian/Projects/tangying-ai-operation-system.wiki/Frontend-Desktop.md`
- Create: `/Users/wanglian/Projects/tangying-ai-operation-system.wiki/Cloud-Backend-Agent-Runtime.md`
- Create: `/Users/wanglian/Projects/tangying-ai-operation-system.wiki/Local-Backend-Runner-MCP.md`
- Create: `/Users/wanglian/Projects/tangying-ai-operation-system.wiki/Video-Creation-Workflows.md`
- Create: `/Users/wanglian/Projects/tangying-ai-operation-system.wiki/JiMeng-MCP-Integration.md`
- Create: `/Users/wanglian/Projects/tangying-ai-operation-system.wiki/Data-Artifacts-Review-Gates.md`

**Interfaces:**
- Consumes: Real repository paths under `frontend/`, `cloud-backend/`, `local-backend/`, `skill-capabilities/`.
- Produces: module pages linked from `Developer-Guide`, `Home`, and `_Sidebar`.

- [ ] **Step 1: Create six focused pages**

Each page must include:
- module responsibility
- code paths
- request/data flow
- common change points
- verification commands
- links back to `Developer-Guide` and `Home`

- [ ] **Step 2: Verify key paths exist**

Run:

```bash
cd /Users/wanglian/Projects/tangying-ai-operation-system
test -f frontend/src/pages/DirectorStudioPage.tsx
test -f cloud-backend/cmd/tangying-ai-os/main.go
test -f cloud-backend/internal/core/agentruntime/plan_compiler.go
test -f local-backend/cmd/local-agent/main.go
test -f local-backend/internal/localtool/mcp_tool_call.go
test -f local-backend/internal/jimengmcp/server.go
```

Expected: command exits 0.

### Task 3: Home, English, and Sidebar Links

**Files:**
- Modify: `/Users/wanglian/Projects/tangying-ai-operation-system.wiki/Home.md`
- Modify: `/Users/wanglian/Projects/tangying-ai-operation-system.wiki/English.md`
- Modify: `/Users/wanglian/Projects/tangying-ai-operation-system.wiki/_Sidebar.md`

**Interfaces:**
- Consumes: `Developer-Guide` and six module pages.
- Produces: discoverable Wiki navigation.

- [ ] **Step 1: Add Home developer section**

Add a compact “开发者快速入口” section in `Home.md` linking `Developer-Guide` and the six module pages.

- [ ] **Step 2: Add English pointer**

Add a short English note in `English.md` linking to `Developer-Guide`.

- [ ] **Step 3: Update sidebar**

Add a “Developer Reference” group in `_Sidebar.md` with the developer guide and six module links.

### Task 4: Verification and Push

**Files:**
- Verify: `/Users/wanglian/Projects/tangying-ai-operation-system.wiki/*.md`
- Verify: `/Users/wanglian/Projects/tangying-ai-operation-system/docs/superpowers/plans/2026-07-03-wiki-developer-reference.md`

**Interfaces:**
- Consumes: all previous task outputs.
- Produces: pushed Wiki and pushed develop_go planning record.

- [ ] **Step 1: Check for missing links and stub markers**

Run:

```bash
cd /Users/wanglian/Projects/tangying-ai-operation-system.wiki
rg -n "占位" . || true
rg -n "Developer-Guide|Frontend-Desktop|Cloud-Backend-Agent-Runtime|Local-Backend-Runner-MCP|Video-Creation-Workflows|JiMeng-MCP-Integration|Data-Artifacts-Review-Gates" Home.md English.md _Sidebar.md
```

Expected: no stub-marker output; navigation links appear.

- [ ] **Step 2: Commit and push Wiki**

```bash
cd /Users/wanglian/Projects/tangying-ai-operation-system.wiki
git add .
git commit -m "Add developer reference wiki pages"
git push origin master
```

- [ ] **Step 3: Commit and push develop_go plan**

```bash
cd /Users/wanglian/Projects/tangying-ai-operation-system
git add docs/superpowers/plans/2026-07-03-wiki-developer-reference.md
git commit -m "docs: plan wiki developer reference"
git push develop_go develop_go
```
