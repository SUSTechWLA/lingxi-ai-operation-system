# Biaoshu Scoring Breakdown Sample Format Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make generated `02_评分标准拆解表.md` follow the provided sample structure: scoring item classification, word-count budgeting, writing strategy tables, chapter pre-planning, and user confirmation.

**Architecture:** Keep the existing Biaoshu scoring-breakdown stage and endpoint. Change only the cloud backend prompt and add lightweight content validation so the generated artifact becomes a structured planning table rather than a long item-by-item scoring memo. Frontend, local backend, database, and OpenAPI do not need changes because `BID_SCORING_BREAKDOWN` already exists and outline generation already reads `scoringReportPath`.

**Tech Stack:** Go cloud backend, ModelGateway prompt construction, Go unit tests, existing React workbench trigger.

---

## Current Context

The current implementation already supports generating `02_评分标准拆解表.md`.

Relevant files:

- `cloud-backend/internal/agents/biaoshu/handler/scoring_breakdown_service.go`
  - Defines `scoringBreakdownSystemPrompt`.
  - Builds the user prompt in `buildScoringBreakdownPrompt`.
  - Calls ModelGateway and writes the Markdown artifact.
- `cloud-backend/internal/agents/biaoshu/handler/scoring_breakdown_service_test.go`
  - Tests basic prompt fields and artifact metadata.
- `cloud-backend/internal/agents/biaoshu/handler/outline_generation_service.go`
  - Already reads `scoringReportPath` and injects the scoring breakdown into outline generation.
- `frontend/src/pages/BiaoshuWorkbench.tsx`
  - Already exposes the scoring-breakdown generation action.

Reference sample:

- `E:\yhbs\标书\02_评分标准拆解表.md`

The reference sample should guide the output format only. The production generator must still use the current project analysis report as the factual source and must not hard-code sample project details such as grass flower maintenance, 19 roads, or 300 pages unless the current analysis report supports them.

## File Structure

- Modify: `cloud-backend/internal/agents/biaoshu/handler/scoring_breakdown_service.go`
  - Replace the current generic four-section prompt with the five-section sample-style structure.
  - Add a required-section validator for generated Markdown.
  - Call the validator before writing the artifact file.
- Modify: `cloud-backend/internal/agents/biaoshu/handler/scoring_breakdown_service_test.go`
  - Update prompt tests to assert sample-style section requirements.
  - Add validator tests for valid output and missing-section output.
- No change: `frontend/src/pages/BiaoshuWorkbench.tsx`
  - Existing button and artifact handling remain valid.
- No change: `cloud-backend/internal/agents/biaoshu/handler/outline_generation_service.go`
  - Existing outline prompt already consumes `02_评分标准拆解表.md`.
- No change: API spec
  - Endpoint request and response shape stay the same.

---

### Task 1: Tighten Prompt Tests Around The Sample Format

**Files:**

- Modify: `cloud-backend/internal/agents/biaoshu/handler/scoring_breakdown_service_test.go`

- [ ] **Step 1: Replace the required prompt assertions**

In `TestBuildScoringBreakdownPromptIncludesRequiredSections`, replace the `required` list with:

```go
required := []string{
	"评分项分类",
	"硬性条件项",
	"正文写作重点项",
	"字数目标计算",
	"目标字数 = 评分分值",
	"各评分项写作策略",
	"技术标章节目录（预规划）",
	"待用户确认",
	"综合实力 7 分",
}
```

- [ ] **Step 2: Add validator tests**

Append these tests to `cloud-backend/internal/agents/biaoshu/handler/scoring_breakdown_service_test.go`:

```go
func TestValidateScoringBreakdownContentAcceptsSampleStyleSections(t *testing.T) {
	content := `# 评分标准拆解表

## 一、评分项分类

### 硬性条件项（9分）-- 证明文件到位即可，正文不展开

| 序号 | 评分项 | 分值 | 证明材料 | 正文处理 |
|------|--------|------|----------|----------|
| 1 | 项目负责人职称 | 3 | 身份证+职称证+社保证明 | 仅在人员配置表中体现 |

### 正文写作重点项（21分）-- 需详实展开，争取评委会主观打分

| 序号 | 评分项 | 分值 | 分值占比 | 写作优先级 |
|------|--------|------|----------|------------|
| 2 | 施工组织方案 | 8 | 38.1% | ★★★ 最高 |

## 二、字数目标计算

公式：**目标字数 = 评分分值 × (总页数 ÷ 总分) × 780**

## 三、各评分项写作策略

### 3.1 施工组织方案（8分）

| 分解 | 分值 | 对应招标要求 | 写作策略 |
|------|------|-------------|----------|
| 施工部署 | 4分 | 招标文件要求 | 分阶段展开 |

## 四、技术标章节目录（预规划）

第一章  项目概况与总体方案

## 五、待用户确认

1. 以上评分项拆解和写作策略是否准确？
`

	if err := validateScoringBreakdownContent(content); err != nil {
		t.Fatalf("expected valid scoring breakdown content, got %v", err)
	}
}

func TestValidateScoringBreakdownContentRejectsMissingSampleSections(t *testing.T) {
	content := `# 评分标准拆解表

## 1. 评分总览

| 评分项 | 分值 |
|--------|------|
| 施工组织方案 | 8 |
`

	if err := validateScoringBreakdownContent(content); err == nil {
		t.Fatal("expected missing-section validation error")
	}
}
```

- [ ] **Step 3: Run the focused tests and confirm they fail**

Run:

```powershell
cd cloud-backend
go test ./internal/agents/biaoshu/handler -run "ScoringBreakdown|ValidateScoringBreakdown" -count=1
```

Expected result:

```text
FAIL
```

Expected reason:

```text
undefined: validateScoringBreakdownContent
```

The prompt assertion may also fail until Task 2 updates the prompt.

---

### Task 2: Rewrite The Scoring Breakdown Prompt To Match The Sample

**Files:**

- Modify: `cloud-backend/internal/agents/biaoshu/handler/scoring_breakdown_service.go`

- [ ] **Step 1: Replace `scoringBreakdownSystemPrompt`**

Replace the current `scoringBreakdownSystemPrompt` constant with:

```go
const scoringBreakdownSystemPrompt = `你是专业的投标技术标评分标准拆解专家。请根据《招标文件解析报告》中的评分办法、技术要求、投标文件组成要求，生成《评分标准拆解表》。

必须输出 Markdown，结构固定如下：

# 评分标准拆解表

## 一、评分项分类

先把评分项分成两类：

### 硬性条件项（X分）-- 证明文件到位即可，正文不展开

用于整理人员证书、设备发票、车辆证件、业绩合同、资质证书、社保证明等客观证明材料类得分项。表格列固定为：

| 序号 | 评分项 | 分值 | 证明材料 | 正文处理 |
|------|--------|------|----------|----------|

正文处理必须说明该项是否仅在证明材料、人员配置表、设备投入清单、业绩表或附件中体现；不得把客观证明材料项扩写成大篇幅正文。

### 正文写作重点项（X分）-- 需详实展开，争取评委会主观打分

用于整理施工组织、技术方案、服务方案、质量管理、安全生产、应急预案、进度保障、设计方案、养护方案等需要正文展开的主观评分项。表格列固定为：

| 序号 | 评分项 | 分值 | 分值占比 | 写作优先级 |
|------|--------|------|----------|------------|

分值占比 = 该评分项分值 ÷ 正文写作重点项总分。写作优先级按分值和评审影响分为“★★★ 最高”“★★☆ 中高”“★☆☆ 中低”。

## 二、字数目标计算

必须给出公式：

**目标字数 = 评分分值 × (总页数 ÷ 总分) × 780**

计算规则：
- 总分优先使用招标文件评分办法中的技术商务或技术标总分。
- 总页数优先使用招标文件或用户已确认信息中的页数要求。
- 如果总页数未明确，使用“建议按300页测算，待用户确认”。
- 只对“正文写作重点项”分配目标字数；硬性条件项不分配正文目标字数。
- 合格范围 = 目标字数 × 0.75 ~ 1.25。

表格列固定为：

| 评分项 | 分值 | 目标字数 | 合格范围 |
|--------|------|----------|----------|

## 三、各评分项写作策略

对每一个“正文写作重点项”分别设置小节，格式为：

### 3.N 评分项名称（X分）-- 重要性判断

优先使用表格拆解。常规表格列为：

| 分解 | 分值 | 对应招标要求 | 写作策略 |
|------|------|-------------|----------|

如果招标文件没有给出子项分值，则列为“招标文件未明确”，但仍要按得分点、服务场景、技术措施或管理流程拆解。

对紧急预案、风险处置、服务保障等场景类评分项，可使用：

| 核心场景 | 写作策略 |
|----------|----------|

写作策略必须具体到可落入后续大纲的内容，例如流程、表格、职责、响应时限、设备人员投入、质量控制点、风险闭环、检查频次、验收标准。不得只写“详细阐述”“加强管理”等空泛表达。

## 四、技术标章节目录（预规划）

按评分项组织章节，确保每个正文写作重点项独立成章或独立成节。必须输出章节清单，并在章节名后标注对应评分项和分值，例如：

第一章  项目概况与总体方案
第二章  XXX方案（X分）
第三章  XXX管理体系（X分）

章节目录必须服务于后续《技术标四级大纲》生成。

## 五、待用户确认

固定输出以下确认问题：

1. 以上评分项拆解和写作策略是否准确？
2. 章节目录结构是否合理？是否需要调整？
3. 字数目标分配是否合适？

最后固定输出：

> 确认后回复"**确认**"进入下一阶段（生成4级标题详细大纲）。

要求：
- 不编造招标文件未出现的分值、评分项、证明材料和硬性时限。
- 若信息缺失，标注“招标文件未明确”或“待用户确认”。
- 不硬套样例中的项目事实；样例只代表结构，不代表当前项目内容。
- 不输出代码块。
- Markdown 一级、二级、三级标题必须严格使用上述中文编号结构。`
```

- [ ] **Step 2: Replace `buildScoringBreakdownPrompt`**

Replace the current function body with:

```go
func buildScoringBreakdownPrompt(analysisText string) string {
	return fmt.Sprintf(`## 招标文件解析报告

%s

请基于上述解析报告生成评分标准拆解表。输出要学习样例的组织方式：先区分“硬性条件项”和“正文写作重点项”，再计算正文写作项的字数目标，随后逐项拆解写作策略，最后给出技术标章节目录预规划和待用户确认问题。

重点抽取并转换以下内容：
- 评分办法与关键得分点。
- 技术要求与服务范围。
- 投标文件组成、格式与递交要求。
- 风险点、扣分点和废标事项。

拆解原则：
- 人员证书、车辆设备、业绩合同、资质证明、社保证明等客观材料项归入“硬性条件项”，只说明证明材料和正文处理方式。
- 施工组织、技术方案、服务方案、质量管理、安全生产、应急预案、进度保障、设计方案、养护方案等主观评分项归入“正文写作重点项”，必须展开写作策略。
- 正文写作重点项必须计算分值占比、写作优先级、目标字数和合格范围。
- 字数测算公式必须使用“目标字数 = 评分分值 × (总页数 ÷ 总分) × 780”。
- 如果总页数未明确，写明“建议按300页测算，待用户确认”，不要把300页描述为招标文件事实。
- 后续技术标章节目录必须与正文写作重点项一一对应，方便生成四级大纲。
`, strings.TrimSpace(analysisText))
}
```

- [ ] **Step 3: Add required-section data**

Add this near the prompt constant:

```go
var scoringBreakdownRequiredSections = []string{
	"## 一、评分项分类",
	"### 硬性条件项",
	"### 正文写作重点项",
	"## 二、字数目标计算",
	"目标字数 = 评分分值",
	"## 三、各评分项写作策略",
	"## 四、技术标章节目录（预规划）",
	"## 五、待用户确认",
}
```

- [ ] **Step 4: Add content validation**

Add this function near `buildScoringBreakdownPrompt`:

```go
func validateScoringBreakdownContent(content string) error {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return fmt.Errorf("LLM returned empty scoring breakdown")
	}
	for _, section := range scoringBreakdownRequiredSections {
		if !strings.Contains(trimmed, section) {
			return fmt.Errorf("scoring breakdown missing required section %q", section)
		}
	}
	if strings.Contains(trimmed, "```") {
		return fmt.Errorf("scoring breakdown must not contain code fences")
	}
	return nil
}
```

- [ ] **Step 5: Call validation before writing the file**

In `GenerateScoringBreakdown`, replace:

```go
content := strings.TrimSpace(result.Content)
if content == "" {
	return nil, fmt.Errorf("LLM returned empty scoring breakdown")
}
```

with:

```go
content := strings.TrimSpace(result.Content)
if err := validateScoringBreakdownContent(content); err != nil {
	return nil, err
}
```

- [ ] **Step 6: Run focused tests**

Run:

```powershell
cd cloud-backend
go test ./internal/agents/biaoshu/handler -run "ScoringBreakdown|ValidateScoringBreakdown" -count=1
```

Expected result:

```text
ok  	github.com/tangying-ai/aios-core/internal/agents/biaoshu/handler
```

---

### Task 3: Verify The Change Does Not Break Existing Handler Behavior

**Files:**

- No additional file edits.

- [ ] **Step 1: Run the full Biaoshu handler test package**

Run:

```powershell
cd cloud-backend
go test ./internal/agents/biaoshu/handler -count=1
```

Expected result:

```text
ok  	github.com/tangying-ai/aios-core/internal/agents/biaoshu/handler
```

- [ ] **Step 2: Run the broader cloud backend tests**

Run:

```powershell
cd cloud-backend
go test ./...
```

Expected result:

```text
PASS
```

If unrelated existing tests fail, record the failing package and error output in the implementation summary, then still keep the focused handler tests as the primary verification for this prompt-only change.

---

### Task 4: Manually Generate A New `02_评分标准拆解表.md`

**Files:**

- The generator writes a project artifact such as `biaoshu-tools/output/<项目名>/02_评分标准拆解表.md`.

- [ ] **Step 1: Start or reuse the cloud backend**

Use the repository's normal cloud backend startup process. If the server is already running on `http://127.0.0.1:8080`, reuse it.

Common local command:

```powershell
cd cloud-backend
go run ./cmd/tangying-ai-os/main.go
```

Expected result:

```text
cloud backend listening on port 8080
```

The exact log line may differ; the acceptance condition is that `POST /api/biaoshu/scoring-breakdown/generate` is reachable.

- [ ] **Step 2: Call the existing scoring-breakdown endpoint**

Use a project that already has `00_招标文件解析报告.md`. Example command:

```powershell
$body = @{
  analysisReportPath = 'E:\lingxi\tangying-ai-operation-system\biaoshu-tools\output\养护3\00_招标文件解析报告.md'
  scoringReportPath = 'E:\lingxi\tangying-ai-operation-system\biaoshu-tools\output\养护3\02_评分标准拆解表.md'
  sourceFile = '养护3'
} | ConvertTo-Json

Invoke-RestMethod -Uri 'http://127.0.0.1:8080/api/biaoshu/scoring-breakdown/generate' `
  -Method Post `
  -ContentType 'application/json; charset=utf-8' `
  -Body $body `
  -TimeoutSec 180
```

Expected result:

```text
success : True
```

- [ ] **Step 3: Inspect the generated Markdown**

Open the generated file and verify it contains:

```markdown
# 评分标准拆解表

## 一、评分项分类
### 硬性条件项
### 正文写作重点项
## 二、字数目标计算
## 三、各评分项写作策略
## 四、技术标章节目录（预规划）
## 五、待用户确认
```

Expected content behavior:

- Hard evidence items are not expanded into long body-writing chapters.
- Body writing items have priority, word-count budget, and writing strategy.
- Missing page count is marked as `建议按300页测算，待用户确认`.
- Chapter pre-planning maps directly to body writing scoring items.
- No sample-only project facts appear unless present in the current analysis report.

---

### Task 5: Confirm Outline Generation Benefits From The New Structure

**Files:**

- No additional file edits unless verification reveals a bug.

- [ ] **Step 1: Generate or regenerate `03_技术标四级大纲.md`**

Use the existing workbench action or the existing outline API after the new scoring breakdown exists.

Expected behavior:

- The outline service reads `00_招标文件解析报告.md`.
- The outline service reads `01_项目背景信息确认表.md`.
- The outline service reads the newly structured `02_评分标准拆解表.md`.

- [ ] **Step 2: Inspect outline coverage**

Verify the generated outline:

- Covers every “正文写作重点项” from `02_评分标准拆解表.md`.
- Does not waste full chapters on pure proof-material items.
- Uses chapter order close to `四、技术标章节目录（预规划）`.
- Allocates more detailed headings to high-priority, high-score items.

- [ ] **Step 3: Run frontend build only if frontend files changed**

This plan does not require frontend edits. If an implementer changes frontend files while executing, run:

```powershell
cd frontend
npm run build
```

Expected result:

```text
built
```

If no frontend files changed, do not run this command for this task.

---

## Acceptance Criteria

- `cloud-backend/internal/agents/biaoshu/handler/scoring_breakdown_service.go` prompt requires the sample-style five-section structure.
- `validateScoringBreakdownContent` rejects old generic output that lacks the sample-style sections.
- `go test ./internal/agents/biaoshu/handler -run "ScoringBreakdown|ValidateScoringBreakdown" -count=1` passes.
- `go test ./internal/agents/biaoshu/handler -count=1` passes.
- Generated `02_评分标准拆解表.md` contains:
  - `## 一、评分项分类`
  - `### 硬性条件项`
  - `### 正文写作重点项`
  - `## 二、字数目标计算`
  - `## 三、各评分项写作策略`
  - `## 四、技术标章节目录（预规划）`
  - `## 五、待用户确认`
- The generated artifact uses current bid-analysis facts and does not hard-code sample-specific project facts.
- The follow-up outline generation uses the new scoring breakdown to structure chapters around real scoring priorities.

## Commit Plan

After implementation and verification:

```powershell
git add cloud-backend/internal/agents/biaoshu/handler/scoring_breakdown_service.go cloud-backend/internal/agents/biaoshu/handler/scoring_breakdown_service_test.go
git commit -m "feat(biaoshu): align scoring breakdown with sample format"
```

Do not stage generated project artifacts such as `biaoshu-tools/output/*/02_评分标准拆解表.md` unless the user explicitly asks to keep sample output in git.

## Self-Review

- Spec coverage: The plan covers the requested sample-based generation behavior, prompt changes, tests, manual generation, and outline verification.
- Completeness scan: The plan contains exact files, commands, code snippets, and expected outcomes.
- Type consistency: Existing request and response structs remain unchanged; artifact kind remains `BID_SCORING_BREAKDOWN`.
- Boundary check: The change stays inside `cloud-backend`; it does not add database, Docker, Redis, Kafka, MinIO, or local-backend dependencies.
