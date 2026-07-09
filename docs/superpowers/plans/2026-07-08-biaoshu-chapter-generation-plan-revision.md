# Biaoshu Chapter Generation Plan Revision Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 `docs/修改计划-分章撰写.md` 修订为可安全落地的分章撰写实施方案，补齐多章节产物注册、阶段确认、RAG 条件、OpenAPI 同步和长任务风险控制。

**Architecture:** 保留原计划的后端 `cloud-backend/internal/agents/biaoshu/handler` 同步生成入口，但先修正产物模型，使多个 `BID_CHAPTERS` 能并存。前端仍通过 `BiaoshuWorkbench` 触发生成，local-backend 继续保存本地项目 manifest，不引入数据库或 Docker 依赖。

**Tech Stack:** Go + Gin + ModelGateway, React + TypeScript + Axios, local-backend Go manifest store, OpenAPI generator.

## Global Constraints

- 不向 `local-backend` 添加 PostgreSQL、Redis、Kafka、MinIO、Docker 或云端 LLM API Key 依赖。
- 新增模型调用必须走 `modelgateway.CapTextToText`，不新增绕过 Gateway 的 LLM HTTP 调用。
- 新增或变更云端 API 必须同步 `cloud-backend/internal/core/apispec/cloud_spec.go`。
- 生成文档文件 `cloud-backend/docs/API_REFERENCE.md` 和 `frontend/src/utils/api-types.generated.ts` 不手工编辑，只通过生成器更新。
- `BID_CHAPTERS` 是多实例产物，必须支持逐章查看、修改、确认，不能按 kind 覆盖成单条记录。
- `04_章节写作任务书.md` 必须经用户确认后，才允许进入分章撰写。
- 若项目目录存在 `embedding_config.json` 或 `vector_db/`，或用户明确要求知识库，正文生成前必须先生成并使用 `05_知识库检索报告.md`。
- 分章撰写可以先作为同步长请求实现，但必须有超时、部分失败返回和清晰的前端提示。

---

## File Structure

- Modify: `docs/修改计划-分章撰写.md`
  - 将原方案修订为可执行版本，补充前置条件、产物多实例模型、API 文档同步、测试范围和非目标。
- Modify: `frontend/src/pages/biaoshuArtifactLogic.ts`
  - 调整手工产物合并规则，允许 `BID_CHAPTERS` 按章节编号或 artifact id 多实例并存。
- Modify: `frontend/src/pages/BiaoshuWorkbench.tsx`
  - 增加分章撰写按钮、确认门禁、生成状态、结果注册和逐章查看入口。
- Modify: `frontend/src/services/api.ts`
  - 增加 `generateChapters` API 类型和调用函数，使用 600s 超时。
- Modify: `local-backend/internal/localagent/biaoshu_project_store.go`
  - 调整 manifest artifact upsert 规则，避免多个 `BID_CHAPTERS` 互相覆盖。
- Modify: `local-backend/internal/localagent/biaoshu_project_store_test.go`
  - 增加多章节 artifact 注册测试。
- Create: `cloud-backend/internal/agents/biaoshu/handler/chapter_generation_service.go`
  - 实现章节解析、字数分配、并发 LLM 调用、文件写入和 artifact 返回。
- Create: `cloud-backend/internal/agents/biaoshu/handler/chapter_generation_handler.go`
  - 注册 `POST /api/biaoshu/chapters/generate`。
- Create: `cloud-backend/internal/agents/biaoshu/handler/chapter_generation_service_test.go`
  - 覆盖大纲解析、文件名清洗、字数分配 fallback、单章失败不阻塞。
- Modify: `cloud-backend/cmd/tangying-ai-os/main.go`
  - 注册 `ChapterGenerationHandler`。
- Modify: `cloud-backend/internal/core/apispec/cloud_spec.go`
  - 补充 `chapter-task-book/generate` 和 `chapters/generate` 两个 Biaoshu API。

---

### Task 1: 修订计划文档

**Files:**
- Modify: `docs/修改计划-分章撰写.md`

**Interfaces:**
- Consumes: 当前可行性分析结论。
- Produces: 一份新的方案章节，明确哪些内容保留、哪些内容必须修改。

- [ ] **Step 1: 增加“修订结论”章节**

在 `docs/修改计划-分章撰写.md` 的背景之后加入：

```markdown
## 1.3 修订结论

本方案总体可行，但 v1.0 不应原样实施。必须先修正以下问题：

1. `BID_CHAPTERS` 是多实例产物，前端和 local-backend 不能继续按 `kind` 覆盖。
2. 分章撰写必须位于 `04_章节写作任务书.md` 用户确认之后。
3. 若项目启用了知识库，必须先生成 `05_知识库检索报告.md`，章节写作 prompt 必须读取对应章节检索结果。
4. 新增 API 必须同步 OpenAPI spec 和生成文档。
5. 单章 `max_tokens=8000` 只适合作为初稿生成；若要求达标长正文，需要后续按小节分块生成。
```

- [ ] **Step 2: 修改原“本次不包含”部分**

将“知识库检索本次暂不实现”改为：

```markdown
- **知识库检索（Stage ⑦）**：默认不强制启用；但当项目目录存在 `embedding_config.json`、`vector_db/`，或用户明确要求“参考知识库/检索历史标书/从向量库召回内容”时，分章撰写前必须先生成并读取 `05_知识库检索报告.md`。
```

- [ ] **Step 3: 文档自查**

Run: `rg -n "暂不实现|BID_CHAPTERS|OpenAPI|用户确认|vector_db" docs/修改计划-分章撰写.md`

Expected: 能看到新的 RAG 条件、用户确认门禁、多实例产物和 OpenAPI 同步要求。

---

### Task 2: 支持 `BID_CHAPTERS` 多实例合并

**Files:**
- Modify: `frontend/src/pages/biaoshuArtifactLogic.ts`
- Test: `frontend/scripts/biaoshu-artifact-logic-check.mjs`

**Interfaces:**
- Consumes: `BiaoshuArtifactRecord`。
- Produces: `mergeManualBiaoshuArtifacts(artifacts, manualArtifacts)` 能保留多个 `kind === 'BID_CHAPTERS'` 的记录。

- [ ] **Step 1: 增加唯一键函数**

在 `mergeOneManualArtifact` 前增加：

```typescript
function biaoshuArtifactMergeKey(artifact: BiaoshuArtifactRecord): string {
  if (artifact.kind === 'BID_CHAPTERS') {
    const chapterNumber = artifact.metadata?.chapterNumber
    if (chapterNumber !== undefined && chapterNumber !== null) {
      return `${artifact.kind}:${String(chapterNumber)}`
    }
    return `${artifact.kind}:${artifact.id || artifact.storageRef}`
  }
  return artifact.kind
}
```

- [ ] **Step 2: 修改替换条件**

将：

```typescript
if (artifact.kind !== manualArtifact.kind) return artifact
```

改为：

```typescript
if (biaoshuArtifactMergeKey(artifact) !== biaoshuArtifactMergeKey(manualArtifact)) return artifact
```

- [ ] **Step 3: 增加脚本测试**

在 `frontend/scripts/biaoshu-artifact-logic-check.mjs` 中新增断言：

```javascript
const multiChapterArtifacts = mergeManualBiaoshuArtifacts([], [
  {
    id: 'chapter-01',
    name: '第一章',
    kind: 'BID_CHAPTERS',
    version: '-',
    status: 'valid',
    owner: 'AI写作',
    updatedAt: '2026-07-08 10:00:00',
    storageRef: 'E:/out/章节/01_第一章_初稿.md',
    summary: '章节初稿',
    sourceTool: 'chapter_generation',
    metadata: { chapterNumber: 1 },
  },
  {
    id: 'chapter-02',
    name: '第二章',
    kind: 'BID_CHAPTERS',
    version: '-',
    status: 'valid',
    owner: 'AI写作',
    updatedAt: '2026-07-08 10:01:00',
    storageRef: 'E:/out/章节/02_第二章_初稿.md',
    summary: '章节初稿',
    sourceTool: 'chapter_generation',
    metadata: { chapterNumber: 2 },
  },
])
assert.equal(multiChapterArtifacts.filter((item) => item.kind === 'BID_CHAPTERS').length, 2)
```

- [ ] **Step 4: 运行测试**

Run: `cd frontend && node scripts/biaoshu-artifact-logic-check.mjs`

Expected: `biaoshuArtifactLogic tests passed`

---

### Task 3: 支持 local-backend 多章节 artifact 注册

**Files:**
- Modify: `local-backend/internal/localagent/biaoshu_project_store.go`
- Modify: `local-backend/internal/localagent/biaoshu_project_store_test.go`

**Interfaces:**
- Consumes: `BiaoshuArtifactRegisterRequest{ID, Kind, StorageRef, Metadata}`。
- Produces: `registerBiaoshuProjectArtifact(projectID, req)` 对 `BID_CHAPTERS` 按 `ID` 或 `metadata.chapterNumber` upsert。

- [ ] **Step 1: 增加 artifact upsert key**

在 `registerBiaoshuProjectArtifact` 附近增加：

```go
func biaoshuArtifactUpsertKey(artifact BiaoshuProjectArtifact) string {
	if artifact.Kind == ArtifactKindChapters {
		if chapterNumber, ok := artifact.Metadata["chapterNumber"]; ok {
			return string(artifact.Kind) + ":" + fmt.Sprint(chapterNumber)
		}
		if strings.TrimSpace(artifact.ID) != "" {
			return string(artifact.Kind) + ":" + strings.TrimSpace(artifact.ID)
		}
	}
	return string(artifact.Kind)
}
```

- [ ] **Step 2: 修改替换循环**

将原来的：

```go
if existing.Kind == artifact.Kind {
```

改为：

```go
if biaoshuArtifactUpsertKey(existing) == biaoshuArtifactUpsertKey(artifact) {
```

- [ ] **Step 3: 增加测试**

在 `biaoshu_project_store_test.go` 增加：

```go
func TestRegisterBiaoshuProjectArtifactKeepsMultipleChapters(t *testing.T) {
	s := newTestServer(t)
	manifest, err := s.createBiaoshuProject(BiaoshuProjectCreateRequest{
		ProjectName: "multi-chapter",
		OutputDir:   filepath.Join(t.TempDir(), "multi-chapter"),
	})
	if err != nil {
		t.Fatal(err)
	}

	for i := 1; i <= 2; i++ {
		_, err = s.registerBiaoshuProjectArtifact(manifest.ProjectID, BiaoshuArtifactRegisterRequest{
			ID:         fmt.Sprintf("chapter-%02d", i),
			Kind:       string(ArtifactKindChapters),
			Name:       fmt.Sprintf("第%d章", i),
			Status:     "valid",
			StorageRef: fmt.Sprintf("章节/%02d_初稿.md", i),
			MimeType:   "text/markdown",
			Metadata: map[string]interface{}{
				"chapterNumber": i,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.readBiaoshuProjectManifest(manifest.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, artifact := range got.Artifacts {
		if artifact.Kind == ArtifactKindChapters {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected 2 chapter artifacts, got %d: %#v", count, got.Artifacts)
	}
}
```

- [ ] **Step 4: 运行测试**

Run: `cd local-backend && go test ./internal/localagent`

Expected: PASS

---

### Task 4: 后端分章服务

**Files:**
- Create: `cloud-backend/internal/agents/biaoshu/handler/chapter_generation_service.go`
- Create: `cloud-backend/internal/agents/biaoshu/handler/chapter_generation_service_test.go`

**Interfaces:**
- Consumes: `GenerateChaptersRequest`
- Produces: `GenerateChapters(ctx context.Context, gw *modelgateway.Gateway, req GenerateChaptersRequest) (*GenerateChaptersResponse, error)`

- [ ] **Step 1: 定义请求响应类型**

在新文件中定义：

```go
type GenerateChaptersRequest struct {
	TaskBookPath       string `json:"taskBookPath"`
	OutlinePath        string `json:"outlinePath"`
	ScoringReportPath  string `json:"scoringReportPath,omitempty"`
	AnalysisReportPath string `json:"analysisReportPath,omitempty"`
	ContextReportPath  string `json:"contextReportPath,omitempty"`
	RetrievalReportPath string `json:"retrievalReportPath,omitempty"`
	OutputDir          string `json:"outputDir"`
	Mode               string `json:"mode,omitempty"`
}

type ChapterResult struct {
	ChapterNumber int                    `json:"chapterNumber"`
	ChapterTitle  string                 `json:"chapterTitle"`
	FilePath      string                 `json:"filePath"`
	Content       string                 `json:"content,omitempty"`
	Artifact      map[string]interface{} `json:"artifact,omitempty"`
	Error         string                 `json:"error,omitempty"`
	WordCount     int                    `json:"wordCount"`
	ScoreWeight   float64                `json:"scoreWeight"`
}

type GenerateChaptersResponse struct {
	Chapters       []ChapterResult `json:"chapters"`
	Success        int             `json:"success"`
	Failed         int             `json:"failed"`
	Warnings       []string        `json:"warnings"`
	TotalWordCount int             `json:"totalWordCount"`
	TotalScore     int             `json:"totalScore"`
}
```

- [ ] **Step 2: 编写大纲解析测试**

```go
func TestExtractChaptersFromOutline(t *testing.T) {
	outline := "## 一、总体项目管理方案\n\n### 1.1 管理目标\n\n## 二、施工方案\n\n### 2.1 工艺流程\n"
	chapters := extractChaptersFromOutline(outline)
	if len(chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(chapters))
	}
	if chapters[0].Number != 1 || chapters[0].Title != "一、总体项目管理方案" {
		t.Fatalf("unexpected first chapter: %#v", chapters[0])
	}
	if !strings.Contains(chapters[0].OutlineSection, "### 1.1 管理目标") {
		t.Fatalf("chapter section missing child heading: %s", chapters[0].OutlineSection)
	}
}
```

- [ ] **Step 3: 实现大纲解析**

实现规则：

```go
var chapterHeadingRe = regexp.MustCompile(`^##\s*([一二三四五六七八九十]+)、(.+?)\s*$`)
```

`extractChaptersFromOutline` 必须只匹配 `## `，不能匹配 `###`。

- [ ] **Step 4: 编写并发生成逻辑**

`GenerateChapters` 使用：

```go
sem := make(chan struct{}, chapterConcurrency)
results := make([]ChapterResult, len(chapters))
var wg sync.WaitGroup
for i, chapter := range chapters {
	i, chapter := i, chapter
	wg.Add(1)
	go func() {
		defer wg.Done()
		sem <- struct{}{}
		defer func() { <-sem }()
		results[i] = generateOneChapter(ctx, gw, inputs, chapter)
	}()
}
wg.Wait()
```

单章失败写入 `ChapterResult.Error`，不得让整体请求直接失败，除非输入文件读取或章节解析失败。

- [ ] **Step 5: 运行后端测试**

Run: `cd cloud-backend && go test ./internal/agents/biaoshu/handler`

Expected: PASS

---

### Task 5: 后端 HTTP 路由与 OpenAPI

**Files:**
- Create: `cloud-backend/internal/agents/biaoshu/handler/chapter_generation_handler.go`
- Modify: `cloud-backend/cmd/tangying-ai-os/main.go`
- Modify: `cloud-backend/internal/core/apispec/cloud_spec.go`

**Interfaces:**
- Consumes: `GenerateChapters`.
- Produces: `POST /api/biaoshu/chapters/generate`.

- [ ] **Step 1: 增加 handler**

```go
type ChapterGenerationHandler struct {
	gw *modelgateway.Gateway
}

func NewChapterGenerationHandler(gw *modelgateway.Gateway) *ChapterGenerationHandler {
	return &ChapterGenerationHandler{gw: gw}
}

func (h *ChapterGenerationHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/biaoshu")
	api.POST("/chapters/generate", h.Generate)
}
```

- [ ] **Step 2: 注册路由**

在 `chapterTaskBookHandler.RegisterRoutes(r)` 后增加：

```go
chapterGenerationHandler := biaoshu_handler.NewChapterGenerationHandler(gw)
chapterGenerationHandler.RegisterRoutes(r)
```

- [ ] **Step 3: 补 OpenAPI 路由**

在 `cloud_spec.go` 的 Biaoshu 区块增加 `POST /api/biaoshu/chapter-task-book/generate` 和 `POST /api/biaoshu/chapters/generate`，请求体字段至少包含：

```text
taskBookPath, outlinePath, scoringReportPath, analysisReportPath, contextReportPath, retrievalReportPath, outputDir, mode
```

- [ ] **Step 4: 运行 API 文档检查**

Run: `cd cloud-backend && make gen-docs`

Expected: generated docs updated.

Run: `cd cloud-backend && make api-docs-check`

Expected: PASS

---

### Task 6: 前端 API 与 UI 接入

**Files:**
- Modify: `frontend/src/services/api.ts`
- Modify: `frontend/src/pages/biaoshuArtifactLogic.ts`
- Modify: `frontend/src/pages/BiaoshuWorkbench.tsx`

**Interfaces:**
- Consumes: `POST /api/biaoshu/chapters/generate`。
- Produces: 用户在任务书产物行点击“分章撰写”后，生成并注册多条章节初稿。

- [ ] **Step 1: 增加 API 函数**

在 `api.ts` 增加：

```typescript
export interface GenerateChaptersRequest {
  taskBookPath: string
  outlinePath: string
  scoringReportPath?: string
  analysisReportPath?: string
  contextReportPath?: string
  retrievalReportPath?: string
  outputDir: string
  mode?: 'strict' | 'draft'
}

export interface ChapterResult {
  chapterNumber: number
  chapterTitle: string
  filePath: string
  content?: string
  artifact?: Record<string, unknown>
  error?: string
  wordCount: number
  scoreWeight: number
}

export interface GenerateChaptersResponse {
  chapters: ChapterResult[]
  success: number
  failed: number
  warnings: string[]
  totalWordCount: number
  totalScore: number
}

export const generateChapters = async (
  payload: GenerateChaptersRequest
): Promise<GenerateChaptersResponse> => {
  const response = await api.post<ApiResponse<GenerateChaptersResponse>>('/biaoshu/chapters/generate', payload, {
    timeout: 600000,
  })
  return response.data.data
}
```

- [ ] **Step 2: 增加章节 artifact 创建函数**

在 `biaoshuArtifactLogic.ts` 增加：

```typescript
export function deriveBiaoshuChapterOutputDir(outlinePath: string): string {
  return joinBiaoshuSiblingPath(outlinePath, '章节')
}

export function createManualChapterArtifact(
  artifact: Record<string, unknown> | undefined,
  chapterNumber: number,
  chapterTitle: string,
  filePath: string,
): BiaoshuArtifactRecord {
  const metadata = objectRecord(artifact?.metadata)
  metadata.manualGenerated = true
  metadata.chapterNumber = chapterNumber
  return {
    id: String(artifact?.id || artifact?.artifactId || `chapter-${String(chapterNumber).padStart(2, '0')}`),
    name: String(artifact?.name || `第${chapterNumber}章：${chapterTitle}`),
    kind: 'BID_CHAPTERS',
    version: '-',
    status: 'valid',
    owner: 'AI写作',
    updatedAt: new Date().toLocaleString('zh-CN', { hour12: false }),
    storageRef: String(artifact?.storageRef || artifact?.storage_ref || filePath),
    summary: String(artifact?.summary || '并发生成的章节初稿'),
    sourceTool: 'chapter_generation',
    metadata,
  }
}
```

- [ ] **Step 3: 增加 UI 状态和 handler**

`BiaoshuWorkbench.tsx` 增加 `generatingChapters` 状态，并在 handler 中检查：

```typescript
if (!taskBookArtifact?.storageRef || !outlineArtifact?.storageRef) {
  setContextError('请先生成并确认章节写作任务书和技术标大纲')
  return
}
```

如果后续增加确认状态，应将条件收紧为 `taskBookArtifact.metadata?.confirmed === true`。

- [ ] **Step 4: 运行前端构建**

Run: `cd frontend && npm run build`

Expected: build succeeds.

---

### Task 7: 验证与验收

**Files:**
- Modify: `docs/修改计划-分章撰写.md`

**Interfaces:**
- Consumes: Tasks 1-6。
- Produces: 清晰的验收命令和人工验收清单。

- [ ] **Step 1: 补充验证命令**

在计划文档中写入：

```bash
cd cloud-backend && go test ./internal/agents/biaoshu/handler
cd cloud-backend && make api-docs-check
cd local-backend && go test ./internal/localagent
cd frontend && node scripts/biaoshu-artifact-logic-check.mjs
cd frontend && npm run build
```

- [ ] **Step 2: 补充人工验收清单**

```markdown
人工验收：
1. 生成 `04_章节写作任务书.md` 后，用户确认才能点击“分章撰写”。
2. 点击“分章撰写”后，`章节/01_..._初稿.md` 到 `章节/0N_..._初稿.md` 均写入磁盘。
3. 产物表中能同时显示多个 `BID_CHAPTERS`，不是只剩最后一章。
4. 每章均可点击“查看/修改”。
5. 任意单章失败时，其他成功章节仍注册成功，失败章节显示 error。
6. 生成正文不得包含“本章小结”“本章总结”“我方”“我们”“报价”“投标总价”。
```

- [ ] **Step 3: 最终测试**

Run: `cd cloud-backend && go test ./...`

Expected: PASS.

Run: `cd local-backend && go test ./...`

Expected: PASS.

Run: `cd frontend && npm run build`

Expected: PASS.

---

## Self-Review

- Spec coverage: 已覆盖原计划中的后端、前端、产物注册、测试计划，同时补齐多章节覆盖、用户确认、RAG 条件和 OpenAPI 同步。
- Placeholder scan: 本计划不包含未决占位语句或延后实现表述。
- Type consistency: 前端使用 `chapterNumber`，后端 JSON 字段同名；local-backend metadata 使用 `chapterNumber` 作为多实例 upsert key。

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-07-08-biaoshu-chapter-generation-plan-revision.md`. Two execution options:

1. Subagent-Driven (recommended) - dispatch a fresh subagent per task, review between tasks, fast iteration.
2. Inline Execution - execute tasks in this session using executing-plans, batch execution with checkpoints.
