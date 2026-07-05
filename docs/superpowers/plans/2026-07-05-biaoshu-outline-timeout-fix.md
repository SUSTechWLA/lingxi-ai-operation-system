# Biaoshu Outline Timeout Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `POST /api/biaoshu/outline/generate` finish reliably for normal bid projects and return actionable diagnostics when the upstream model is slow.

**Architecture:** Keep the existing synchronous API contract, but shrink the prompt before the LLM call, reduce the outline output budget, add request-size and duration diagnostics, and give the frontend outline call a route-specific timeout/error message. This avoids introducing a new async job system while fixing the current root cause: large prompt plus `max_tokens=50000` racing a 120s frontend/provider timeout.

**Tech Stack:** Go cloud backend, Gin handler, ModelGateway OpenAI-compatible provider, React/TypeScript frontend, Axios.

---

## Current Failure Summary

The observed error `timeout of 120000ms exceeded` comes from the frontend Axios client, not a backend business error. The global Axios timeout is `120000` in `frontend/src/services/api.ts`. The outline endpoint is synchronous, and the backend waits for `gw.Execute` to return before sending any response.

The backend outline service currently reads the full bid analysis report and full project context report into one prompt, then asks the model for up to `50000` output tokens. The bid analysis report generator can also append the original tender text as an appendix, so downstream outline generation may resend raw tender content even when the outline only needs the structured analysis.

The running model provider is OpenAI-compatible DeepSeek at `https://api.deepseek.com/chat/completions`, model `deepseek-v4-pro`. The provider HTTP client has a fixed `120s` timeout, so increasing only the frontend timeout will not fully solve the issue.

## File Structure

- Modify: `cloud-backend/internal/agents/biaoshu/handler/outline_generation_service.go`
  - Add compact prompt helpers.
  - Strip generated-report appendices before outline generation.
  - Cap large input sections deterministically.
  - Reduce `max_tokens` for outline generation.
  - Add duration/size logging around the model call.
- Create: `cloud-backend/internal/agents/biaoshu/handler/outline_generation_service_test.go`
  - Unit tests for appendix stripping, text truncation, prompt assembly, and max token selection.
- Modify: `frontend/src/services/api.ts`
  - Add a route-specific timeout for `generateOutline`.
  - Normalize Axios timeout errors into a Chinese user-facing message that identifies model slowness.
- Optional modify after tests pass: `cloud-backend/internal/core/modelgateway/providers/openai/provider.go`
  - Only if reduced prompt/output still hits provider timeout in manual verification, raise provider HTTP timeout to `180s`.

---

### Task 1: Add Failing Tests For Outline Prompt Compaction

**Files:**
- Create: `cloud-backend/internal/agents/biaoshu/handler/outline_generation_service_test.go`

- [ ] **Step 1: Create the test file**

Add this file:

```go
package handler

import (
	"strings"
	"testing"
)

func TestStripBidAnalysisAppendixRemovesRawTenderText(t *testing.T) {
	input := "# 招标文件解析报告\n\n## 项目基本信息\n\n保留结构化分析。\n\n---\n\n## 附录：招标文件原文\n\n这里是很长的招标文件原文。"

	got := stripBidAnalysisAppendix(input)

	if strings.Contains(got, "这里是很长的招标文件原文") {
		t.Fatalf("expected raw tender appendix to be removed, got %q", got)
	}
	if !strings.Contains(got, "保留结构化分析") {
		t.Fatalf("expected structured analysis to be kept, got %q", got)
	}
}

func TestStripBidAnalysisAppendixKeepsReportWithoutAppendix(t *testing.T) {
	input := "# 招标文件解析报告\n\n## 评分办法\n\n技术部分 80 分。"

	got := stripBidAnalysisAppendix(input)

	if got != input {
		t.Fatalf("expected report without appendix to stay unchanged, got %q", got)
	}
}

func TestLimitTextForPromptKeepsHeadingAndAddsTruncationNotice(t *testing.T) {
	input := "标题\n" + strings.Repeat("内容", 200)

	got := limitTextForPrompt(input, 20, "解析报告")

	if len([]rune(got)) > 120 {
		t.Fatalf("expected compact text, got rune length %d: %q", len([]rune(got)), got)
	}
	if !strings.Contains(got, "标题") {
		t.Fatalf("expected beginning of text to be preserved, got %q", got)
	}
	if !strings.Contains(got, "解析报告已截断") {
		t.Fatalf("expected truncation notice, got %q", got)
	}
}

func TestBuildOutlineUserPromptStripsAndLimitsInputs(t *testing.T) {
	analysis := "# 招标文件解析报告\n\n## 评分办法\n\n技术方案 80 分。\n\n## 附录：招标文件原文\n\n" + strings.Repeat("原文", 100)
	context := "# 项目背景信息确认表\n\n项目位于杭州。"
	scoring := "# 评分标准拆解表\n\n施工组织设计 40 分。"

	prompt, meta := buildOutlineUserPrompt(analysis, context, scoring)

	if strings.Contains(prompt, "原文原文原文") {
		t.Fatalf("expected raw tender appendix to be stripped, got %q", prompt)
	}
	if !strings.Contains(prompt, "技术方案 80 分") {
		t.Fatalf("expected analysis scoring content to be kept, got %q", prompt)
	}
	if !strings.Contains(prompt, "项目位于杭州") {
		t.Fatalf("expected context report to be included, got %q", prompt)
	}
	if !strings.Contains(prompt, "施工组织设计 40 分") {
		t.Fatalf("expected scoring report to be included, got %q", prompt)
	}
	if meta.AnalysisOriginalRunes <= meta.AnalysisPromptRunes {
		t.Fatalf("expected analysis prompt to be smaller after appendix strip, meta=%+v", meta)
	}
}

func TestOutlineMaxTokensIsBounded(t *testing.T) {
	if outlineMaxTokens > 16000 {
		t.Fatalf("outlineMaxTokens should stay bounded for synchronous requests, got %d", outlineMaxTokens)
	}
	if outlineMaxTokens < 8000 {
		t.Fatalf("outlineMaxTokens should be large enough for a four-level outline, got %d", outlineMaxTokens)
	}
}
```

- [ ] **Step 2: Run the new tests and verify they fail**

Run:

```bash
cd cloud-backend
go test ./internal/agents/biaoshu/handler -run 'Outline|Strip|Limit' -count=1
```

Expected: FAIL with undefined symbols such as `stripBidAnalysisAppendix`, `limitTextForPrompt`, `buildOutlineUserPrompt`, or `outlineMaxTokens`.

- [ ] **Step 3: Commit the failing tests**

```bash
git add cloud-backend/internal/agents/biaoshu/handler/outline_generation_service_test.go
git commit -m "test: cover biaoshu outline prompt compaction"
```

---

### Task 2: Implement Prompt Compaction In Outline Service

**Files:**
- Modify: `cloud-backend/internal/agents/biaoshu/handler/outline_generation_service.go`

- [ ] **Step 1: Add constants and metadata type**

In `outline_generation_service.go`, after `outlineSystemPrompt`, add:

```go
const (
	outlineAnalysisPromptRuneLimit = 18000
	outlineContextPromptRuneLimit  = 8000
	outlineScoringPromptRuneLimit  = 6000
	outlineMaxTokens               = 12000
)

type outlinePromptMetadata struct {
	AnalysisOriginalRunes int
	AnalysisPromptRunes   int
	ContextOriginalRunes  int
	ContextPromptRunes    int
	ScoringOriginalRunes  int
	ScoringPromptRunes    int
}
```

- [ ] **Step 2: Add appendix stripping helper**

In the same file, before `GenerateOutline`, add:

```go
func stripBidAnalysisAppendix(text string) string {
	markers := []string{
		"\n## 附录：招标文件原文",
		"\n# 附录：招标文件原文",
		"\n---\n\n## 附录：招标文件原文",
	}
	for _, marker := range markers {
		if idx := strings.Index(text, marker); idx >= 0 {
			return strings.TrimSpace(text[:idx])
		}
	}
	return strings.TrimSpace(text)
}
```

- [ ] **Step 3: Add deterministic text limiter**

Add:

```go
func limitTextForPrompt(text string, maxRunes int, label string) string {
	trimmed := strings.TrimSpace(text)
	if maxRunes <= 0 {
		return trimmed
	}
	runes := []rune(trimmed)
	if len(runes) <= maxRunes {
		return trimmed
	}
	kept := strings.TrimSpace(string(runes[:maxRunes]))
	return fmt.Sprintf("%s\n\n> 注：%s已截断，仅保留前 %d 字用于本次大纲生成。", kept, label, maxRunes)
}
```

- [ ] **Step 4: Add prompt builder**

Add:

```go
func buildOutlineUserPrompt(analysisText, contextText, scoringText string) (string, outlinePromptMetadata) {
	analysisOriginalRunes := len([]rune(analysisText))
	contextOriginalRunes := len([]rune(contextText))
	scoringOriginalRunes := len([]rune(scoringText))

	compactAnalysis := limitTextForPrompt(
		stripBidAnalysisAppendix(analysisText),
		outlineAnalysisPromptRuneLimit,
		"招标文件解析报告",
	)
	compactContext := limitTextForPrompt(
		contextText,
		outlineContextPromptRuneLimit,
		"项目背景信息确认表",
	)
	compactScoring := ""
	if strings.TrimSpace(scoringText) != "" {
		compactScoring = "\n\n## 评分标准拆解表\n\n" + limitTextForPrompt(
			scoringText,
			outlineScoringPromptRuneLimit,
			"评分标准拆解表",
		)
	}

	prompt := fmt.Sprintf(
		"## 招标文件解析报告\n\n%s\n\n## 项目背景信息确认表\n\n%s%s",
		compactAnalysis,
		compactContext,
		compactScoring,
	)

	return prompt, outlinePromptMetadata{
		AnalysisOriginalRunes: analysisOriginalRunes,
		AnalysisPromptRunes:   len([]rune(compactAnalysis)),
		ContextOriginalRunes:  contextOriginalRunes,
		ContextPromptRunes:    len([]rune(compactContext)),
		ScoringOriginalRunes:  scoringOriginalRunes,
		ScoringPromptRunes:    len([]rune(compactScoring)),
	}
}
```

- [ ] **Step 5: Replace inline prompt assembly**

In `GenerateOutline`, replace the current scoring text and `userPrompt := fmt.Sprintf(...)` block with:

```go
	var scoringText string
	scoringReportPath := strings.TrimSpace(req.ScoringReportPath)
	if scoringReportPath != "" {
		if scoringBytes, err := os.ReadFile(scoringReportPath); err == nil {
			scoringText = string(scoringBytes)
		}
	}

	userPrompt, promptMeta := buildOutlineUserPrompt(string(analysisBytes), string(contextBytes), scoringText)
```

- [ ] **Step 6: Reduce max tokens in the model request**

In the `Parameters` map, replace:

```go
"max_tokens":  50000.0,
```

with:

```go
"max_tokens":  float64(outlineMaxTokens),
```

- [ ] **Step 7: Add model-call diagnostics**

Add `time` and `go.uber.org/zap` to imports:

```go
	"time"

	"go.uber.org/zap"
```

Immediately before `gw.Execute`, add:

```go
	start := time.Now()
	zap.L().Info("biaoshu outline generation model call starting",
		zap.Int("analysisOriginalRunes", promptMeta.AnalysisOriginalRunes),
		zap.Int("analysisPromptRunes", promptMeta.AnalysisPromptRunes),
		zap.Int("contextOriginalRunes", promptMeta.ContextOriginalRunes),
		zap.Int("contextPromptRunes", promptMeta.ContextPromptRunes),
		zap.Int("scoringOriginalRunes", promptMeta.ScoringOriginalRunes),
		zap.Int("scoringPromptRunes", promptMeta.ScoringPromptRunes),
		zap.Int("maxTokens", outlineMaxTokens),
	)
```

Immediately after a successful model call, add:

```go
	zap.L().Info("biaoshu outline generation model call completed",
		zap.Int64("durationMs", time.Since(start).Milliseconds()),
		zap.String("model", result.Usage.Model),
		zap.Int("promptTokens", result.Usage.PromptTokens),
		zap.Int("outputTokens", result.Usage.OutputTokens),
	)
```

- [ ] **Step 8: Run focused tests**

Run:

```bash
cd cloud-backend
go test ./internal/agents/biaoshu/handler -run 'Outline|Strip|Limit' -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit implementation**

```bash
git add cloud-backend/internal/agents/biaoshu/handler/outline_generation_service.go cloud-backend/internal/agents/biaoshu/handler/outline_generation_service_test.go
git commit -m "fix: compact biaoshu outline generation prompt"
```

---

### Task 3: Improve Frontend Outline Timeout Handling

**Files:**
- Modify: `frontend/src/services/api.ts`

- [ ] **Step 1: Add route-specific timeout constants**

Near the Axios instance, after `const API_BASE = ...`, add:

```ts
const DEFAULT_API_TIMEOUT_MS = 120000
const BIAOSHU_OUTLINE_TIMEOUT_MS = 180000
```

Change the Axios instance from:

```ts
const api = axios.create({
  baseURL: API_BASE,
  timeout: 120000,
})
```

to:

```ts
const api = axios.create({
  baseURL: API_BASE,
  timeout: DEFAULT_API_TIMEOUT_MS,
})
```

- [ ] **Step 2: Add a timeout message helper**

After `apiErrorMessage`, add:

```ts
const outlineTimeoutMessage = (error: unknown): string | null => {
  if (!axios.isAxiosError(error)) return null
  if (error.code !== 'ECONNABORTED') return null
  return '生成大纲耗时过长，模型服务未在 180 秒内返回。请稍后重试，或先精简招标文件解析报告后再生成。'
}
```

- [ ] **Step 3: Use the route-specific timeout in `generateOutline`**

Replace:

```ts
export const generateOutline = async (
  payload: GenerateOutlineRequest
): Promise<GenerateOutlineResponse> => {
  const response = await api.post<GenerateOutlineResponse>('/biaoshu/outline/generate', payload)
  return response.data
}
```

with:

```ts
export const generateOutline = async (
  payload: GenerateOutlineRequest
): Promise<GenerateOutlineResponse> => {
  try {
    const response = await api.post<GenerateOutlineResponse>('/biaoshu/outline/generate', payload, {
      timeout: BIAOSHU_OUTLINE_TIMEOUT_MS,
    })
    return response.data
  } catch (error) {
    const message = outlineTimeoutMessage(error)
    if (message && error instanceof Error) {
      error.message = message
    }
    throw error
  }
}
```

- [ ] **Step 4: Run frontend static checks**

Run:

```bash
cd frontend
npm run build
```

Expected: build completes successfully.

- [ ] **Step 5: Commit frontend change**

```bash
git add frontend/src/services/api.ts
git commit -m "fix: clarify biaoshu outline timeout handling"
```

---

### Task 4: Manual Reproduction And Timing Check

**Files:**
- No code changes unless this task proves provider timeout still fails.

- [ ] **Step 1: Confirm backend model-provider config**

Run:

```powershell
Invoke-RestMethod -Uri 'http://127.0.0.1:8080/api/config/model-provider' -Method Get -TimeoutSec 5 | ConvertTo-Json -Depth 5
```

Expected: JSON includes `endpoint`, `model`, and `hasKey: true`.

- [ ] **Step 2: Call outline API with the known local sample**

Run:

```powershell
$body = @{
  analysisReportPath = 'E:\lingxi\tangying-ai-operation-system\biaoshu-tools\output\养护\00_招标文件解析报告.md'
  contextReportPath = 'E:\lingxi\tangying-ai-operation-system\biaoshu-tools\output\养护\01_项目背景信息确认表.md'
  outlinePath = 'E:\lingxi\tangying-ai-operation-system\biaoshu-tools\output\养护\03_技术标四级大纲.md'
  sourceFile = '养护'
} | ConvertTo-Json

$elapsed = Measure-Command {
  Invoke-RestMethod -Uri 'http://127.0.0.1:8080/api/biaoshu/outline/generate' `
    -Method Post `
    -ContentType 'application/json' `
    -Body $body `
    -TimeoutSec 180
}

$elapsed.TotalSeconds
```

Expected: command returns a successful JSON response and elapsed time is under `180` seconds.

- [ ] **Step 3: Inspect generated outline**

Run:

```powershell
Get-Item -LiteralPath 'E:\lingxi\tangying-ai-operation-system\biaoshu-tools\output\养护\03_技术标四级大纲.md' | Select-Object FullName,Length,LastWriteTime
Get-Content -LiteralPath 'E:\lingxi\tangying-ai-operation-system\biaoshu-tools\output\养护\03_技术标四级大纲.md' -TotalCount 40
```

Expected: file exists, is non-empty, and starts with Markdown outline headings.

- [ ] **Step 4: Check backend logs for compaction diagnostics**

Watch the backend terminal or log output for:

```text
biaoshu outline generation model call starting
biaoshu outline generation model call completed
```

Expected: starting log shows `analysisPromptRunes` lower than `analysisOriginalRunes` when an appendix was present, and completed log shows `durationMs`.

- [ ] **Step 5: If provider still times out at 120 seconds, apply provider timeout fallback**

Only perform this step if the backend returns an error containing `Client.Timeout exceeded while awaiting headers` around 120 seconds.

Modify `cloud-backend/internal/core/modelgateway/providers/openai/provider.go`:

```go
client:         &http.Client{Timeout: 180 * time.Second},
```

Run:

```bash
cd cloud-backend
go test ./internal/core/modelgateway/providers/openai ./internal/agents/biaoshu/handler -count=1
```

Expected: PASS.

Commit:

```bash
git add cloud-backend/internal/core/modelgateway/providers/openai/provider.go
git commit -m "fix: allow longer model calls for outline generation"
```

---

### Task 5: Full Verification

**Files:**
- No code changes.

- [ ] **Step 1: Run biaoshu handler tests**

```bash
cd cloud-backend
go test ./internal/agents/biaoshu/handler -count=1
```

Expected: PASS.

- [ ] **Step 2: Run ModelGateway provider tests if provider timeout changed**

```bash
cd cloud-backend
go test ./internal/core/modelgateway/providers/openai -count=1
```

Expected: PASS.

- [ ] **Step 3: Run frontend build**

```bash
cd frontend
npm run build
```

Expected: PASS.

- [ ] **Step 4: Run local backend tests to protect boundary rules**

```bash
cd local-backend
go test ./...
```

Expected: PASS. No database, Docker, Kafka, Redis, or MinIO dependencies should be added to `local-backend`.

- [ ] **Step 5: Final commit if any verification-only fixes were needed**

```bash
git status --short
git add cloud-backend/internal/agents/biaoshu/handler/outline_generation_service.go cloud-backend/internal/agents/biaoshu/handler/outline_generation_service_test.go frontend/src/services/api.ts
git commit -m "fix: prevent biaoshu outline generation timeout"
```

Expected: commit succeeds if there are staged changes. If Task 4 already committed all changes, `git status --short` should show no files from this plan.

---

## Acceptance Criteria

- `POST /api/biaoshu/outline/generate` no longer sends raw tender appendices back into the outline prompt.
- Outline generation uses `outlineMaxTokens = 12000`, not `50000`.
- Backend logs record input compaction sizes and model-call duration.
- Frontend outline generation has a dedicated `180000ms` timeout and a clear Chinese timeout message.
- Focused backend tests pass.
- Frontend build passes.
- Manual sample generation creates `biaoshu-tools/output/养护/03_技术标四级大纲.md`.

## Rollback Plan

If generated outlines become too shallow after lowering `outlineMaxTokens`, increase `outlineMaxTokens` from `12000` to `16000` and rerun the focused outline tests plus manual sample generation. Do not restore `50000` for the synchronous endpoint, because it recreates the timeout risk.

If prompt truncation removes important scoring content, pass `scoringReportPath` from the frontend in a separate follow-up change so scoring details are included through the dedicated scoring section rather than through raw report appendices.

## Self-Review

- Spec coverage: The plan addresses the observed 120s frontend timeout, the synchronous backend LLM wait, the oversized prompt, the high output token cap, and missing diagnostics.
- Placeholder scan: No task uses unspecified implementation placeholders; code snippets and commands are concrete.
- Type consistency: Helper names used in tests match helper names introduced in implementation: `stripBidAnalysisAppendix`, `limitTextForPrompt`, `buildOutlineUserPrompt`, and `outlineMaxTokens`.
