# Video Agent Beta P0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the repository consistent with the first self-use beta of the Video Agent by removing stale bid/chat API contract surfaces and refreshing the video-first architecture documentation.

**Architecture:** The implementation keeps runtime behavior stable and changes only contracts/docs for P0. OpenAPI remains the source of truth for generated API markdown and frontend generated types. `publish` remains a compatibility publishing layer for beta and is documented as a future `distribution` migration.

**Tech Stack:** Go 1.25 cloud backend, OpenAPI generator in `cloud-backend/internal/core/apispec`, React/Vite frontend generated API types, Markdown docs.

---

### Task 1: Guard the OpenAPI Spec Against Bid/Chat Product Surfaces

**Files:**
- Modify: `cloud-backend/internal/core/apispec/cloud_spec_test.go`

- [ ] **Step 1: Write the failing test**

Add a test that checks the generated cloud spec has no active bid/chat tags or paths:

```go
func TestBuildCloudSpec_DoesNotExposeRemovedBusinessLines(t *testing.T) {
	spec := BuildCloudSpec()

	for _, tag := range spec.Tags {
		if tag.Name == "Bid" || tag.Name == "Chat" {
			t.Fatalf("removed business line tag %q must not be exposed", tag.Name)
		}
	}

	for path := range spec.Paths {
		if strings.HasPrefix(path, "/api/bid") || strings.HasPrefix(path, "/api/chat") {
			t.Fatalf("removed business line path %q must not be exposed", path)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd cloud-backend && go test ./internal/core/apispec -run TestBuildCloudSpec_DoesNotExposeRemovedBusinessLines -count=1`

Expected before implementation: FAIL with `removed business line tag "Chat" must not be exposed` or `removed business line tag "Bid" must not be exposed`.

- [ ] **Step 3: Implement minimal OpenAPI cleanup**

Modify `cloud-backend/internal/core/apispec/cloud_spec.go` by removing:

```go
Tag("Chat", "AI assistant conversational dialog").
Tag("Bid", "Bid/tender document generation")
```

No route handlers are removed in this task because current code already lacks `/api/bid/*` and `/api/chat/*` route registration.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd cloud-backend && go test ./internal/core/apispec -run TestBuildCloudSpec_DoesNotExposeRemovedBusinessLines -count=1`

Expected: PASS.

### Task 2: Regenerate Generated API Artifacts

**Files:**
- Modify: `cloud-backend/docs/API_REFERENCE.md`
- Modify: `frontend/src/utils/api-types.generated.ts`

- [ ] **Step 1: Regenerate docs and types**

Run: `cd cloud-backend && make gen-docs`

Expected: command exits 0 and overwrites `cloud-backend/docs/API_REFERENCE.md` and `frontend/src/utils/api-types.generated.ts`.

- [ ] **Step 2: Verify generated artifacts do not expose removed routes**

Run: `rg "/api/bid|/api/chat|Bid|Chat" cloud-backend/docs/API_REFERENCE.md frontend/src/utils/api-types.generated.ts`

Expected: no matches for removed product API surfaces. If matches refer only to unrelated words inside generated schemas, inspect them before changing anything.

- [ ] **Step 3: Verify docs are in sync**

Run: `cd cloud-backend && make api-docs-check`

Expected: PASS.

### Task 3: Rewrite the Architecture Doc for Video Agent Beta

**Files:**
- Modify: `docs/ARCHITECTURE.md`

- [ ] **Step 1: Replace current architecture doc with beta-oriented structure**

Write a concise architecture document with these sections:

```markdown
# 躺营 Video Agent 架构设计文档

1. 产品定位
2. Beta 自用全流程
3. 系统运行边界
4. 总体架构
5. Core 通用引擎
6. Video Agent 业务层
7. Publish 兼容发布层与 Distribution 路线
8. Operation 运营复盘路线
9. Artifact 创作资产体系
10. Local Runner 本地执行体系
11. Model Gateway 模型网关
12. HyperFrames 渲染链路
13. API 设计
14. 数据模型
15. 前端页面结构
16. 测试与验收标准
17. 后续路线图
```

Use current route names such as `/api/video-projects`, `/api/agent/runs`, `/api/artifacts`, `/api/local-runners`, and `/api/publish`. State clearly that `/api/publish` is beta compatibility and future migration target is `/api/distribution/*`.

- [ ] **Step 2: Check architecture doc for forbidden current product claims**

Run: `rg -n "标书|投标|通用对话|AI 对话助手|/api/bid|/api/chat|internal/agents/bid|internal/agents/chat" docs/ARCHITECTURE.md`

Expected: no matches.

### Task 4: Beta Verification

**Files:**
- No source modifications unless verification exposes a defect.

- [ ] **Step 1: Run cloud OpenAPI tests**

Run: `cd cloud-backend && go test ./internal/core/apispec -count=1`

Expected: PASS.

- [ ] **Step 2: Run cloud backend tests**

Run: `cd cloud-backend && go test ./...`

Expected: PASS.

- [ ] **Step 3: Run local backend tests**

Run: `cd local-backend && go test ./...`

Expected: PASS.

- [ ] **Step 4: Run frontend build**

Run: `cd frontend && npm run build`

Expected: PASS.

- [ ] **Step 5: Run final stale-surface scan**

Run: `rg -n "/api/bid|/api/chat|Tag\\(\"Bid\"|Tag\\(\"Chat\"|internal/agents/bid|internal/agents/chat" cloud-backend/internal/core/apispec cloud-backend/docs/API_REFERENCE.md frontend/src/utils/api-types.generated.ts docs/ARCHITECTURE.md README.md AGENTS.md`

Expected: no matches.

---

## Self-Review

- Spec coverage: P0 OpenAPI cleanup, generated docs/types, architecture rewrite, and beta verification are all represented.
- Scope check: package rename to `distribution`, new `operation` APIs, and route migrations are intentionally excluded.
- Placeholder scan: no placeholder implementation steps remain.

