# Biaoshu Scoring Breakdown Artifact Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `02_评分标准拆解表.md` as a first-class Biaoshu stage artifact and force outline generation to reference `00 + 01 + 02`.

**Architecture:** Add a narrow backend generation endpoint that reads the existing bid analysis report and writes a scoring breakdown Markdown artifact. Wire the frontend to expose this stage between project context and outline, then pass the generated scoring breakdown path into the existing outline endpoint.

**Tech Stack:** Go cloud backend, Gin, ModelGateway, React, TypeScript, Axios, local artifact files.

---

## Current Context

Current Biaoshu manual flow has:

- `00_招标文件解析报告.md`
- `01_项目背景信息确认表.md`
- `03_技术标四级大纲.md`

The `biaoshu-writer` skill requires a missing stage:

- `02_评分标准拆解表.md`

The existing outline backend already accepts `scoringReportPath`, but the frontend does not generate or pass that artifact. This plan fills that gap without introducing the later agent-based context selector.

## File Structure

- Create: `cloud-backend/internal/agents/biaoshu/handler/scoring_breakdown_service.go`
  - Reads `analysisReportPath`.
  - Calls ModelGateway to generate `02_评分标准拆解表.md`.
  - Writes the Markdown file.
  - Returns artifact metadata.
- Create: `cloud-backend/internal/agents/biaoshu/handler/scoring_breakdown_handler.go`
  - Registers `POST /api/biaoshu/scoring-breakdown/generate`.
- Create: `cloud-backend/internal/agents/biaoshu/handler/scoring_breakdown_service_test.go`
  - Tests path validation, prompt construction, and artifact metadata.
- Modify: `cloud-backend/cmd/tangying-ai-os/main.go`
  - Registers the new handler beside existing Biaoshu handlers.
- Modify: `cloud-backend/internal/core/apispec/cloud_spec.go`
  - Adds OpenAPI entry for the new endpoint.
- Modify: `frontend/src/services/api.ts`
  - Adds request/response types and `generateScoringBreakdown`.
  - Passes `scoringReportPath` when generating outline.
- Modify: `frontend/src/pages/biaoshuArtifactLogic.ts`
  - Adds artifact kind `BID_SCORING_BREAKDOWN`.
  - Adds path helper for `02_评分标准拆解表.md`.
  - Adds manual artifact creation/merge support.
- Modify: `frontend/src/pages/BiaoshuWorkbench.tsx`
  - Adds scoring breakdown stage action between context and outline.
  - Requires scoring breakdown before outline generation when available.
- Modify: `frontend/scripts/biaoshu-artifact-logic-check.mjs`
  - Adds assertions for scoring breakdown artifact ordering and path derivation.

---

### Task 1: Add Frontend Artifact Model Support For 02

**Files:**
- Modify: `frontend/src/pages/biaoshuArtifactLogic.ts`
- Modify: `frontend/scripts/biaoshu-artifact-logic-check.mjs`

- [ ] **Step 1: Add scoring breakdown stage metadata**

In `frontend/src/pages/biaoshuArtifactLogic.ts`, find the stage list containing `BID_PROJECT_CONTEXT` and `BID_OUTLINE`. Insert the scoring stage between them:

```ts
{ key: 'scoring', label: '评分标准拆解表', tool: 'scoring_breakdown_generator', kind: 'BID_SCORING_BREAKDOWN', owner: '评分拆解' },
```

Expected ordering:

```ts
[
  ...,
  { key: 'context', label: '项目背景信息确认表', tool: 'project_context_report', kind: 'BID_PROJECT_CONTEXT', owner: '信息确认' },
  { key: 'scoring', label: '评分标准拆解表', tool: 'scoring_breakdown_generator', kind: 'BID_SCORING_BREAKDOWN', owner: '评分拆解' },
  { key: 'outline', label: '技术标大纲', tool: 'outline_generator', kind: 'BID_OUTLINE', owner: '大纲规划' },
]
```

- [ ] **Step 2: Add display name**

In `displayNameForBiaoshuArtifact`, add:

```ts
BID_SCORING_BREAKDOWN: '评分拆解',
```

- [ ] **Step 3: Add path helper**

Near `deriveBiaoshuProjectContextPath` and `deriveBiaoshuOutlinePath`, add:

```ts
export function deriveBiaoshuScoringBreakdownPath(analysisReportPath: string): string {
  return joinBiaoshuSiblingPath(analysisReportPath, '02_评分标准拆解表.md')
}
```

- [ ] **Step 4: Add manual artifact factory**

After `createManualProjectContextArtifact`, add:

```ts
export function createManualScoringBreakdownArtifact(
  artifact: Record<string, unknown> | undefined,
  reportPath: string,
  sourceFile: string,
): BiaoshuArtifactRecord {
  const metadata = objectRecord(artifact?.metadata)
  return {
    id: String(artifact?.id || artifact?.artifactId || 'manual-scoring-breakdown'),
    name: String(artifact?.name || '评分标准拆解表'),
    kind: 'BID_SCORING_BREAKDOWN',
    version: 'v1',
    status: 'valid',
    owner: '评分拆解',
    updatedAt: new Date().toISOString(),
    storageRef: String(artifact?.storageRef || artifact?.storage_ref || reportPath),
    summary: String(artifact?.summary || '手动生成的评分标准拆解表'),
    sourceTool: 'scoring_breakdown_generator',
    metadata: { ...metadata, sourceFile, manualGenerated: true },
  }
}
```

- [ ] **Step 5: Update dependency ordering**

In the function that maps artifact dependencies, replace:

```ts
if (kind === 'BID_OUTLINE') return 'BID_PROJECT_CONTEXT'
```

with:

```ts
if (kind === 'BID_SCORING_BREAKDOWN') return 'BID_PROJECT_CONTEXT'
if (kind === 'BID_OUTLINE') return 'BID_SCORING_BREAKDOWN'
```

- [ ] **Step 6: Update frontend logic check**

In `frontend/scripts/biaoshu-artifact-logic-check.mjs`, import:

```js
createManualScoringBreakdownArtifact,
deriveBiaoshuScoringBreakdownPath,
```

Add assertions near the existing path helper checks:

```js
assert.equal(
  deriveBiaoshuScoringBreakdownPath('E:/bid/out/00_analysis_report.md'),
  'E:/bid/out/02_评分标准拆解表.md',
)
```

Add a manual artifact assertion near the context and outline artifact checks:

```js
const regeneratedScoring = createManualScoringBreakdownArtifact(
  {
    id: 'manual-scoring-1',
    name: 'Scoring breakdown',
    storageRef: 'E:/bid/out/02_评分标准拆解表.md',
    summary: 'Recovered scoring breakdown',
    metadata: { recovered: true },
  },
  'E:/bid/out/02_评分标准拆解表.md',
  'E:/bid/source.pdf',
)
assert.equal(regeneratedScoring.kind, 'BID_SCORING_BREAKDOWN')
assert.equal(regeneratedScoring.owner, '评分拆解')
```

- [ ] **Step 7: Run frontend logic check**

Run:

```bash
cd frontend
node scripts/biaoshu-artifact-logic-check.mjs
```

Expected:

```text
biaoshuArtifactLogic tests passed
```

- [ ] **Step 8: Commit artifact model support**

```bash
git add frontend/src/pages/biaoshuArtifactLogic.ts frontend/scripts/biaoshu-artifact-logic-check.mjs
git commit -m "feat: add biaoshu scoring breakdown artifact model"
```

---

### Task 2: Add Backend Scoring Breakdown Service

**Files:**
- Create: `cloud-backend/internal/agents/biaoshu/handler/scoring_breakdown_service.go`
- Create: `cloud-backend/internal/agents/biaoshu/handler/scoring_breakdown_service_test.go`

- [ ] **Step 1: Write service tests**

Create `cloud-backend/internal/agents/biaoshu/handler/scoring_breakdown_service_test.go`:

```go
package handler

import (
	"strings"
	"testing"
)

func TestBuildScoringBreakdownPromptIncludesRequiredSections(t *testing.T) {
	analysis := "# 招标文件解析报告\n\n### **5. 评分办法与关键得分点**\n\n技术商务分 30 分。\n- 综合实力 7 分\n- 业绩 2 分\n- 种植养护方案 8 分"

	prompt := buildScoringBreakdownPrompt(analysis)

	required := []string{
		"评分项",
		"分值",
		"响应章节建议",
		"必须证明材料",
		"技术标大纲映射",
		"综合实力 7 分",
	}
	for _, item := range required {
		if !strings.Contains(prompt, item) {
			t.Fatalf("expected prompt to contain %q, got %q", item, prompt)
		}
	}
}

func TestScoringBreakdownArtifactMetadata(t *testing.T) {
	artifact := scoringBreakdownArtifact("E:/out/02_评分标准拆解表.md", "source.pdf")

	if artifact["kind"] != "BID_SCORING_BREAKDOWN" {
		t.Fatalf("unexpected kind: %#v", artifact["kind"])
	}
	if artifact["unitId"] != "bid-scoring-breakdown" {
		t.Fatalf("unexpected unitId: %#v", artifact["unitId"])
	}
	if artifact["storageRef"] != "E:/out/02_评分标准拆解表.md" {
		t.Fatalf("unexpected storageRef: %#v", artifact["storageRef"])
	}
}
```

- [ ] **Step 2: Run tests and confirm they fail**

Run:

```bash
cd cloud-backend
go test ./internal/agents/biaoshu/handler -run ScoringBreakdown -count=1
```

Expected: FAIL with undefined functions `buildScoringBreakdownPrompt` and `scoringBreakdownArtifact`.

- [ ] **Step 3: Create service implementation**

Create `cloud-backend/internal/agents/biaoshu/handler/scoring_breakdown_service.go`:

```go
package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
)

const scoringBreakdownSystemPrompt = `你是专业的投标技术标评分标准拆解专家。请根据《招标文件解析报告》中的评分办法、技术要求、投标文件组成要求，生成《评分标准拆解表》。

必须输出 Markdown，结构固定如下：

# 评分标准拆解表

## 1. 评分总览
用表格列出评分大类、分值、评审重点、对技术标的影响。

## 2. 技术商务评分项逐项拆解
每个评分项必须包含：
- 评分项
- 分值
- 招标文件依据
- 响应章节建议
- 必须证明材料
- 写作要点
- 风险提示

## 3. 技术标大纲映射
用表格给出“评分项 -> 建议一级章节 -> 建议二级章节 -> 必须覆盖内容”。

## 4. 废标与扣分风险清单
列出技术商务标中容易导致扣分、不得分、废标的事项。

要求：
- 不编造招标文件未出现的分值和证明材料。
- 若信息缺失，标注“招标文件未明确”。
- 技术标章节建议必须服务于后续四级大纲生成。
- 不输出代码块。`

type GenerateScoringBreakdownRequest struct {
	AnalysisReportPath string `json:"analysisReportPath"`
	ScoringReportPath  string `json:"scoringReportPath"`
	SourceFile         string `json:"sourceFile,omitempty"`
}

type GenerateScoringBreakdownResponse struct {
	AnalysisReportPath string                 `json:"analysisReportPath"`
	ScoringReportPath  string                 `json:"scoringReportPath"`
	Content            string                 `json:"content"`
	Artifact           map[string]interface{} `json:"artifact"`
}

func GenerateScoringBreakdown(
	ctx context.Context,
	gw *modelgateway.Gateway,
	req GenerateScoringBreakdownRequest,
) (*GenerateScoringBreakdownResponse, error) {
	analysisReportPath := strings.TrimSpace(req.AnalysisReportPath)
	scoringReportPath := strings.TrimSpace(req.ScoringReportPath)
	sourceFile := strings.TrimSpace(req.SourceFile)

	if analysisReportPath == "" {
		return nil, fmt.Errorf("analysisReportPath is required")
	}
	if scoringReportPath == "" {
		return nil, fmt.Errorf("scoringReportPath is required")
	}
	if gw == nil {
		return nil, fmt.Errorf("model gateway is not available")
	}

	analysisBytes, err := os.ReadFile(analysisReportPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read analysis report: %w", err)
	}

	result, err := gw.Execute(ctx, &modelgateway.ModelRequest{
		Capability: modelgateway.CapTextToText,
		Messages: []modelgateway.Message{
			{Role: "system", Content: scoringBreakdownSystemPrompt},
			{Role: "user", Content: buildScoringBreakdownPrompt(string(analysisBytes))},
		},
		Parameters: map[string]any{
			"temperature": 0.2,
			"max_tokens":  12000.0,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	content := strings.TrimSpace(result.Content)
	if content == "" {
		return nil, fmt.Errorf("LLM returned empty scoring breakdown")
	}

	if err := os.MkdirAll(filepath.Dir(scoringReportPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create scoring report directory: %w", err)
	}
	if err := os.WriteFile(scoringReportPath, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write scoring report: %w", err)
	}

	return &GenerateScoringBreakdownResponse{
		AnalysisReportPath: analysisReportPath,
		ScoringReportPath:  scoringReportPath,
		Content:            content,
		Artifact:           scoringBreakdownArtifact(scoringReportPath, sourceFile),
	}, nil
}

func buildScoringBreakdownPrompt(analysisText string) string {
	return fmt.Sprintf(`## 招标文件解析报告

%s

请基于上述解析报告生成评分标准拆解表。重点抽取“评分办法与关键得分点”“技术要求与服务范围”“投标文件组成、格式与递交要求”“风险点和废标事项”。

输出必须服务于后续技术标四级大纲生成，尤其要给出：

| 评分项 | 分值 | 响应章节建议 | 必须证明材料 | 技术标大纲映射 | 风险提示 |
|---|---:|---|---|---|---|
`, strings.TrimSpace(analysisText))
}

func scoringBreakdownArtifact(scoringReportPath, sourceFile string) map[string]interface{} {
	return map[string]interface{}{
		"unitId":     "bid-scoring-breakdown",
		"kind":       "BID_SCORING_BREAKDOWN",
		"name":       filepath.Base(scoringReportPath),
		"mimeType":   "text/markdown",
		"storageRef": scoringReportPath,
		"metadata": map[string]interface{}{
			"status":     "valid",
			"sourceFile": sourceFile,
		},
	}
}
```

- [ ] **Step 4: Run service tests**

Run:

```bash
cd cloud-backend
go test ./internal/agents/biaoshu/handler -run ScoringBreakdown -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit service**

```bash
git add cloud-backend/internal/agents/biaoshu/handler/scoring_breakdown_service.go cloud-backend/internal/agents/biaoshu/handler/scoring_breakdown_service_test.go
git commit -m "feat: generate biaoshu scoring breakdown"
```

---

### Task 3: Add Backend Handler And Route Registration

**Files:**
- Create: `cloud-backend/internal/agents/biaoshu/handler/scoring_breakdown_handler.go`
- Modify: `cloud-backend/cmd/tangying-ai-os/main.go`

- [ ] **Step 1: Create handler**

Create `cloud-backend/internal/agents/biaoshu/handler/scoring_breakdown_handler.go`:

```go
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
	"go.uber.org/zap"
)

type ScoringBreakdownHandler struct {
	gw *modelgateway.Gateway
}

func NewScoringBreakdownHandler(gw *modelgateway.Gateway) *ScoringBreakdownHandler {
	return &ScoringBreakdownHandler{gw: gw}
}

func (h *ScoringBreakdownHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/biaoshu/scoring-breakdown")
	api.POST("/generate", h.Generate)
}

type generateScoringBreakdownRequest struct {
	AnalysisReportPath string `json:"analysisReportPath"`
	ScoringReportPath  string `json:"scoringReportPath"`
	SourceFile         string `json:"sourceFile,omitempty"`
	ProjectID          string `json:"projectId,omitempty"`
	RunID              string `json:"runId,omitempty"`
}

func (h *ScoringBreakdownHandler) Generate(c *gin.Context) {
	var req generateScoringBreakdownRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	result, err := GenerateScoringBreakdown(c.Request.Context(), h.gw, GenerateScoringBreakdownRequest{
		AnalysisReportPath: req.AnalysisReportPath,
		ScoringReportPath:  req.ScoringReportPath,
		SourceFile:         req.SourceFile,
	})
	if err != nil {
		zap.L().Error("scoring breakdown generation failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"analysisReportPath": result.AnalysisReportPath,
			"scoringReportPath":  result.ScoringReportPath,
			"artifact":           result.Artifact,
		},
	})
}
```

- [ ] **Step 2: Register route in main**

In `cloud-backend/cmd/tangying-ai-os/main.go`, find the Biaoshu handler registration block and insert:

```go
scoringHandler := biaoshu_handler.NewScoringBreakdownHandler(gw)
scoringHandler.RegisterRoutes(r)
```

Place it after `contextReportHandler.RegisterRoutes(r)` and before `outlineHandler.RegisterRoutes(r)`.

- [ ] **Step 3: Run handler package tests**

Run:

```bash
cd cloud-backend
go test ./internal/agents/biaoshu/handler -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit handler route**

```bash
git add cloud-backend/internal/agents/biaoshu/handler/scoring_breakdown_handler.go cloud-backend/cmd/tangying-ai-os/main.go
git commit -m "feat: expose biaoshu scoring breakdown endpoint"
```

---

### Task 4: Add Frontend API And Stage Action

**Files:**
- Modify: `frontend/src/services/api.ts`
- Modify: `frontend/src/pages/BiaoshuWorkbench.tsx`

- [ ] **Step 1: Add API types and function**

In `frontend/src/services/api.ts`, add after project context report types:

```ts
export interface GenerateScoringBreakdownRequest {
  analysisReportPath: string
  scoringReportPath: string
  sourceFile?: string
  projectId?: string
  runId?: string
}

export interface GenerateScoringBreakdownResponse {
  success: boolean
  data?: {
    analysisReportPath: string
    scoringReportPath: string
    artifact: Record<string, unknown>
  }
  error?: string
}

export const generateScoringBreakdown = async (
  payload: GenerateScoringBreakdownRequest
): Promise<GenerateScoringBreakdownResponse> => {
  const response = await api.post<GenerateScoringBreakdownResponse>('/biaoshu/scoring-breakdown/generate', payload)
  return response.data
}
```

- [ ] **Step 2: Import new helpers in workbench**

In `frontend/src/pages/BiaoshuWorkbench.tsx`, update imports from `./biaoshuArtifactLogic`:

```ts
createManualScoringBreakdownArtifact,
deriveBiaoshuScoringBreakdownPath,
```

Update imports from `../services/api`:

```ts
generateScoringBreakdown,
```

- [ ] **Step 3: Track scoring artifact**

Near existing artifact lookups:

```ts
const contextArtifact = artifacts.find((a) => a.kind === 'BID_PROJECT_CONTEXT')
const outlineArtifact = artifacts.find((a) => a.kind === 'BID_OUTLINE')
```

Insert:

```ts
const scoringArtifact = artifacts.find((a) => a.kind === 'BID_SCORING_BREAKDOWN')
```

- [ ] **Step 4: Handle generated scoring artifact**

In `handleReportGenerated`, add this branch before `BID_OUTLINE`:

```ts
} else if (kind === 'BID_SCORING_BREAKDOWN') {
  upsertManualArtifact(createManualScoringBreakdownArtifact(artifact, filePath, sourceFile))
```

- [ ] **Step 5: Add generate scoring handler**

Add this function near `handleGenerateContextReport` and `handleGenerateOutline`:

```ts
const handleGenerateScoringBreakdown = async () => {
  if (!analysisArtifact?.storageRef) return
  setGeneratingOutline(true)
  setContextError(null)
  try {
    const sourceFile = typeof analysisArtifact.metadata?.sourceFile === 'string'
      ? analysisArtifact.metadata.sourceFile
      : ''
    const scoringReportPath = deriveScoringBreakdownPath(analysisArtifact.storageRef)
    const result = await generateScoringBreakdown({
      analysisReportPath: analysisArtifact.storageRef,
      scoringReportPath,
      sourceFile,
      projectId: run?.id,
      runId: run?.id,
    })
    if (!result.success) {
      setContextError(result.error || '生成评分标准拆解表失败')
      return
    }
    if (result.data) {
      onReportGenerated(result.data.artifact, result.data.scoringReportPath, sourceFile)
    }
  } catch (e: unknown) {
    setContextError(e instanceof Error ? e.message : '生成评分标准拆解表失败')
  } finally {
    setGeneratingOutline(false)
  }
}
```

Also add alias near existing path helpers:

```ts
const deriveScoringBreakdownPath = deriveBiaoshuScoringBreakdownPath
```

- [ ] **Step 6: Pass scoring path to outline**

In `handleGenerateOutline`, change:

```ts
const result = await generateOutline({
  analysisReportPath: analysisArtifact.storageRef,
  contextReportPath: contextArtifact.storageRef,
  outlinePath,
  sourceFile,
})
```

to:

```ts
const result = await generateOutline({
  analysisReportPath: analysisArtifact.storageRef,
  contextReportPath: contextArtifact.storageRef,
  scoringReportPath: scoringArtifact?.storageRef || deriveScoringBreakdownPath(analysisArtifact.storageRef),
  outlinePath,
  sourceFile,
})
```

- [ ] **Step 7: Add a visible action button for scoring**

In the stage action area where report/context/outline generation buttons are rendered, add a button between context report generation and outline generation:

```tsx
<button
  type="button"
  onClick={handleGenerateScoringBreakdown}
  disabled={!analysisArtifact?.storageRef || generatingOutline}
  className="inline-flex items-center gap-2 rounded-lg bg-amber-500 px-4 py-2 text-sm font-black text-white shadow-sm hover:bg-amber-600 disabled:cursor-not-allowed disabled:opacity-50"
>
  <FiRefreshCw className={generatingOutline ? 'animate-spin' : ''} />
  生成评分拆解表
</button>
```

Use the surrounding button styles already present in the component if the exact class should match nearby controls.

- [ ] **Step 8: Run frontend checks**

Run:

```bash
cd frontend
node scripts/biaoshu-artifact-logic-check.mjs
npm run build
```

Expected: both commands pass.

- [ ] **Step 9: Commit frontend wiring**

```bash
git add frontend/src/services/api.ts frontend/src/pages/BiaoshuWorkbench.tsx
git commit -m "feat: add scoring breakdown generation to biaoshu workbench"
```

---

### Task 5: Update API Spec And Verify End-To-End

**Files:**
- Modify: `cloud-backend/internal/core/apispec/cloud_spec.go`

- [ ] **Step 1: Add OpenAPI route**

In `cloud-backend/internal/core/apispec/cloud_spec.go`, near other `/api/biaoshu/*` routes, add a route for:

```text
POST /api/biaoshu/scoring-breakdown/generate
```

The request schema must include:

```json
{
  "analysisReportPath": "string",
  "scoringReportPath": "string",
  "sourceFile": "string",
  "projectId": "string",
  "runId": "string"
}
```

The response schema must include:

```json
{
  "success": true,
  "data": {
    "analysisReportPath": "string",
    "scoringReportPath": "string",
    "artifact": {}
  }
}
```

- [ ] **Step 2: Run backend tests**

Run:

```bash
cd cloud-backend
go test ./internal/agents/biaoshu/handler -count=1
go test ./internal/core/apispec -count=1
```

Expected: PASS.

- [ ] **Step 3: Regenerate docs if generator is available**

Run:

```bash
cd cloud-backend
make gen-docs
make api-docs-check
```

Expected: generated API docs are in sync. If `make` is unavailable on Windows, record that and rely on `go test ./internal/core/apispec -count=1`.

- [ ] **Step 4: Manual API test**

With cloud backend running, execute:

```powershell
$body = @{
  analysisReportPath = 'E:\lingxi\tangying-ai-operation-system\biaoshu-tools\output\养护\00_招标文件解析报告.md'
  scoringReportPath = 'E:\lingxi\tangying-ai-operation-system\biaoshu-tools\output\养护\02_评分标准拆解表.md'
  sourceFile = '养护'
} | ConvertTo-Json

Invoke-RestMethod -Uri 'http://127.0.0.1:8080/api/biaoshu/scoring-breakdown/generate' `
  -Method Post `
  -ContentType 'application/json' `
  -Body $body `
  -TimeoutSec 180
```

Expected: response contains `success: true`, and the file `02_评分标准拆解表.md` exists.

- [ ] **Step 5: Manual outline regeneration**

Generate outline from the workbench or API after `02_评分标准拆解表.md` exists.

Expected:

- The new outline references the scoring breakdown.
- The outline covers all technical-business scoring items.
- The output no longer stops after only the first or second chapter.

- [ ] **Step 6: Commit API spec**

```bash
git add cloud-backend/internal/core/apispec/cloud_spec.go cloud-backend/docs/API_REFERENCE.md frontend/src/utils/api-types.generated.ts
git commit -m "docs: add scoring breakdown API spec"
```

Only include generated docs and generated TypeScript types if the generation command succeeded.

---

## Acceptance Criteria

- `02_评分标准拆解表.md` can be generated from `00_招标文件解析报告.md`.
- The generated scoring breakdown is visible in the artifact table as `BID_SCORING_BREAKDOWN`.
- Outline generation sends `scoringReportPath`.
- The outline prompt includes `00 + 01 + 02`.
- `frontend/scripts/biaoshu-artifact-logic-check.mjs` passes.
- `frontend npm run build` passes.
- `go test ./internal/agents/biaoshu/handler -count=1` passes.
- Manual sample generation creates `biaoshu-tools/output/养护/02_评分标准拆解表.md`.

## Deferred Work

The agent-based context selector is intentionally deferred to the project update memo. This implementation keeps context selection deterministic: the outline stage uses the known required inputs `00_招标文件解析报告.md`, `01_项目背景信息确认表.md`, and `02_评分标准拆解表.md`.

## Self-Review

- Spec coverage: The plan adds the missing 02 artifact, wires it into frontend/backend, and makes outline generation reference it.
- Placeholder scan: No unspecified implementation placeholders remain.
- Type consistency: Artifact kind is consistently `BID_SCORING_BREAKDOWN`; path helper is `deriveBiaoshuScoringBreakdownPath`; backend response field is `scoringReportPath`.
