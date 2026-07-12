# 标书章节字数扩充完整修改计划

> 状态：待实施  
> 日期：2026-07-10  
> 范围：后端 + 前端 + 本地产物注册 + OpenAPI + 质量检查  
> 前置阶段：已完成分章节初稿生成，产物类型为 `BID_CHAPTERS`  
> 目标阶段：在进入去 AI 痕迹、章节合并、Word 转换之前，完成章节字数达标扩充

---

## 1. 修改目标

当前系统已经能生成分章节正文，但章节初稿往往存在字数不足、评分点展开不充分、部分小节偏短的问题。下一步需要新增一个“章节字数扩充闭环”，按照 biaoshu-writer 的要求，将流程从“章节初稿生成”推进到“章节字数检查、扩写、复查、用户逐章确认”。

本次修改不追求一次性生成最终 Word 成稿，而是补齐以下能力：

1. 自动检查每个章节当前字数与目标字数差距。
2. 根据评分分值生成每章目标字数和合格范围。
3. 对不达标章节生成明确的扩写任务。
4. 按章节或小节扩写正文，不整章推倒重写。
5. 扩写后重新检查字数和禁用内容。
6. 将扩写稿作为可查看、可修改、可确认的阶段产物。

---

## 2. 流程调整

### 2.1 当前流程

```text
⑧ 分章撰写
→ ⑩ 输出各章节初稿
→ ⑪ 章节字数检查
→ ⑫ 去 AI 痕迹
→ ⑬ 汇总整合
→ ⑭ Word 转换
```

### 2.2 修改后流程

```text
⑧ 分章撰写
→ ⑧.5 章节字数预检查
→ ⑧.6 生成章节扩写任务书
→ ⑧.7 扩写不足章节
→ ⑧.8 扩写质量检查
→ ⑩ 用户逐章确认
→ ⑪ 正式章节字数检查
→ ⑫ 去 AI 痕迹
→ ⑬ 汇总整合
→ ⑭ Word 转换
```

### 2.3 阶段闸门

必须满足以下条件才能进入扩写：

1. 至少存在一个有效的 `BID_CHAPTERS` 章节初稿。
2. 已存在 `02_评分标准拆解表.md`。
3. 已存在 `04_章节写作任务书.md`。
4. 用户已确认当前章节初稿可以进入字数扩充。
5. 若项目启用了知识库，扩写 prompt 必须读取 `05_知识库检索报告.md`。

---

## 3. 字数计算规则

### 3.1 标准公式

沿用 biaoshu-writer 字数规则：

```text
目标字数 = 评分分值 × (总页数 ÷ 总分) × 780
合格下限 = 目标字数 × 0.75
合格上限 = 目标字数 × 1.25
```

### 3.2 缺失信息处理

如果评分标准中无法提取总页数：

```text
建议按 300 页测算，待用户确认
```

注意：不得把 300 页写成招标文件事实，只能标记为测算假设。

如果无法提取章节分值：

1. 优先从 `04_章节写作任务书.md` 的“对应评分项”“预估字数”字段提取。
2. 其次从 `02_评分标准拆解表.md` 的章节目录和评分项映射中提取。
3. 若仍失败，按章节数均分总目标字数，并在报告中写入 warning。

---

## 4. 新增产物

| 产物 | 类型 | 路径 | 说明 |
|---|---|---|---|
| 字数预检查报告 | `BID_WORD_COUNT_REPORT` | `90_章节字数检查报告.md` | 记录每章当前字数、目标字数、合格范围、缺口 |
| 章节扩写任务书 | `BID_CHAPTER_EXPANSION_TASK_BOOK` | `06_章节扩写任务书.md` | 指明每章需要扩写的小节、评分点、目标补充字数 |
| 章节扩写稿 | `BID_CHAPTERS` | `章节/01_第一章_扩写稿.md` | 保留章节产物类型，通过 metadata 区分扩写稿 |
| 扩写质量检查报告 | `BID_EXPANSION_QA_REPORT` | `91_章节扩写质量检查报告.md` | 检查禁用词、章节小结、字数达标、标题结构 |

### 4.1 章节扩写稿 metadata

扩写后的章节仍使用 `BID_CHAPTERS`，但必须带上扩写元数据：

```json
{
  "chapterNumber": 1,
  "draftStage": "expanded",
  "sourceDraft": "章节/01_第一章_初稿.md",
  "wordCountBefore": 8200,
  "wordCountAfter": 13600,
  "targetWordCount": 15000,
  "qualifiedMin": 11250,
  "qualifiedMax": 18750,
  "expandedAt": "2026-07-10T00:00:00Z"
}
```

### 4.2 多章节产物要求

`BID_CHAPTERS` 是多实例产物，不能按 `kind` 覆盖。注册和合并时必须使用：

```text
kind + chapterNumber + draftStage
```

作为逻辑唯一键。

---

## 5. 后端修改方案

### 5.1 新增文件

| 文件 | 职责 |
|---|---|
| `cloud-backend/internal/agents/biaoshu/handler/chapter_word_count_service.go` | 字数统计、目标字数计算、报告生成 |
| `cloud-backend/internal/agents/biaoshu/handler/chapter_word_count_handler.go` | 注册字数检查 API |
| `cloud-backend/internal/agents/biaoshu/handler/chapter_expansion_service.go` | 构建扩写任务、调用 LLM、写扩写稿 |
| `cloud-backend/internal/agents/biaoshu/handler/chapter_expansion_handler.go` | 注册扩写 API |
| `cloud-backend/internal/agents/biaoshu/handler/chapter_expansion_service_test.go` | 扩写逻辑单元测试 |

### 5.2 API 设计

#### 5.2.1 字数检查 API

```text
POST /api/biaoshu/chapters/word-count/check
```

请求：

```go
type CheckChapterWordCountRequest struct {
    ChapterPaths       []string `json:"chapterPaths"`
    TaskBookPath       string   `json:"taskBookPath"`
    ScoringReportPath  string   `json:"scoringReportPath"`
    OutputReportPath   string   `json:"outputReportPath"`
    Mode               string   `json:"mode,omitempty"` // strict | draft
}
```

响应：

```go
type ChapterWordCountItem struct {
    ChapterNumber   int      `json:"chapterNumber"`
    ChapterTitle    string   `json:"chapterTitle"`
    FilePath        string   `json:"filePath"`
    CurrentWords    int      `json:"currentWords"`
    TargetWords     int      `json:"targetWords"`
    QualifiedMin    int      `json:"qualifiedMin"`
    QualifiedMax    int      `json:"qualifiedMax"`
    GapWords        int      `json:"gapWords"`
    Status          string   `json:"status"` // qualified | too_short | too_long | unknown
    Score           float64  `json:"score"`
    Warnings        []string `json:"warnings,omitempty"`
}

type CheckChapterWordCountResponse struct {
    ReportPath   string                 `json:"reportPath"`
    Items        []ChapterWordCountItem `json:"items"`
    Artifact     map[string]interface{} `json:"artifact"`
    Warnings     []string               `json:"warnings"`
}
```

#### 5.2.2 章节扩写 API

```text
POST /api/biaoshu/chapters/expand
```

请求：

```go
type ExpandChaptersRequest struct {
    ChapterPaths          []string `json:"chapterPaths"`
    WordCountReportPath   string   `json:"wordCountReportPath"`
    ExpansionTaskBookPath string   `json:"expansionTaskBookPath"`
    TaskBookPath          string   `json:"taskBookPath"`
    OutlinePath           string   `json:"outlinePath"`
    ScoringReportPath     string   `json:"scoringReportPath"`
    AnalysisReportPath    string   `json:"analysisReportPath"`
    ContextReportPath     string   `json:"contextReportPath,omitempty"`
    RetrievalReportPath   string   `json:"retrievalReportPath,omitempty"`
    OutputDir             string   `json:"outputDir"`
    Mode                  string   `json:"mode,omitempty"` // strict | draft
}
```

响应：

```go
type ExpandChapterResult struct {
    ChapterNumber    int                    `json:"chapterNumber"`
    ChapterTitle     string                 `json:"chapterTitle"`
    SourcePath       string                 `json:"sourcePath"`
    ExpandedPath     string                 `json:"expandedPath"`
    WordCountBefore  int                    `json:"wordCountBefore"`
    WordCountAfter   int                    `json:"wordCountAfter"`
    TargetWordCount  int                    `json:"targetWordCount"`
    Artifact         map[string]interface{} `json:"artifact,omitempty"`
    Error            string                 `json:"error,omitempty"`
}

type ExpandChaptersResponse struct {
    Results               []ExpandChapterResult `json:"results"`
    ExpansionTaskBookPath string                `json:"expansionTaskBookPath"`
    QaReportPath          string                `json:"qaReportPath"`
    Success               int                   `json:"success"`
    Failed                int                   `json:"failed"`
    Warnings              []string              `json:"warnings"`
}
```

### 5.3 后端实现要点

#### 5.3.1 字数统计

优先使用 Go 实现轻量统计：

1. 去除 Markdown 标题符号、表格分隔符、HTML 注释。
2. 中文按汉字计数。
3. 英文单词按 token 计数。
4. 数字和单位按一个词组计数。

如果后续需要更精确，可再接入已有 `check_chapter_words.py`，但第一版不应让 cloud-backend 直接依赖 Python 环境。

#### 5.3.2 扩写目标分配

对 `too_short` 章节：

```text
补充字数 = qualifiedMin - currentWords
```

扩写分配优先级：

1. 评分项未充分展开的小节。
2. 当前小节段落少于 3 段的小节。
3. 没有表格的章节补充措施表。
4. 项目背景相关但正文未体现的小节。
5. 风险、质量、安全、进度、验收等可操作措施不足的小节。

#### 5.3.3 并发控制

章节扩写仍可并发，但默认不超过 2：

```go
const chapterExpansionConcurrency = 2
```

原因：扩写 prompt 会包含原章节全文，token 压力高于初稿生成。

#### 5.3.4 单章失败策略

单章扩写失败不得阻断其他章节。响应中对应章节写入 `error` 字段，成功章节正常写入磁盘和注册 artifact。

---

## 6. Prompt 设计

### 6.1 System Prompt

```text
你是专业的投标技术标章节扩写专家。

你的任务不是重写章节，而是在保留原章节标题结构、事实边界和写作风格的基础上，对字数不足或评分点展开不足的章节进行扩写。

必须遵守：
1. 保留原有一级、二级、三级、四级标题结构，不得删除原文有效内容。
2. 不得新增“本章小结”“本章总结”“小结”“总结”等总结性章节。
3. 严禁出现“我方”“我们”，统一使用“本方案”“项目组”“将”。
4. 严禁出现报价、预算金额、投标总价、价格承诺等商务内容。
5. 不得编造招标文件、评分标准、项目背景中不存在的硬性事实。
6. 扩写内容必须围绕评分项、采购需求、项目背景、实施措施展开。
7. 每个被扩写小节至少补充 2-4 个自然段。
8. 必要时增加表格，但不得堆砌空泛表格。
9. 扩写后正文应自然连续，不得出现“以下为扩写内容”等提示语。
10. 输出完整扩写后的章节 Markdown。
```

### 6.2 User Prompt

```text
## 扩写任务

请对【{chapterTitle}】进行字数扩充。

当前字数：{currentWords}
目标字数：{targetWords}
合格下限：{qualifiedMin}
建议补充字数：{gapWords}

## 原章节正文

{chapterContent}

---

## 本章大纲

{outlineSection}

---

## 本章评分项与扩写要求

{chapterExpansionTask}

---

## 章节写作任务书相关内容

{taskBookSection}

---

## 评分标准

{scoringSection}

---

## 项目背景

{contextSection}

---

## 招标文件核心要求

{analysisSection}

---

请输出扩写后的完整章节 Markdown。不得输出解释说明，不得包裹代码块。
```

### 6.3 扩写内容方向

扩写不应简单增加形容词，应优先补充：

1. 流程步骤。
2. 责任分工。
3. 资源配置。
4. 质量控制点。
5. 安全与环保措施。
6. 进度保障措施。
7. 风险识别与应急处置。
8. 检查频次和验收标准。
9. 与项目所在地、气候、交通、水文、周边环境相关的针对性措施。
10. 与评分项逐条对应的表格。

---

## 7. 前端修改方案

### 7.1 修改文件

| 文件 | 修改内容 |
|---|---|
| `frontend/src/services/api.ts` | 新增字数检查和章节扩写 API 调用 |
| `frontend/src/pages/biaoshuArtifactLogic.ts` | 新增扩写产物创建函数、路径推导函数、产物显示名称 |
| `frontend/src/pages/BiaoshuWorkbench.tsx` | 新增“检查字数”“扩写不足章节”按钮、状态和日志 |

### 7.2 API 函数

```typescript
export interface CheckChapterWordCountRequest {
  chapterPaths: string[]
  taskBookPath: string
  scoringReportPath: string
  outputReportPath: string
  mode?: 'strict' | 'draft'
}

export interface ChapterWordCountItem {
  chapterNumber: number
  chapterTitle: string
  filePath: string
  currentWords: number
  targetWords: number
  qualifiedMin: number
  qualifiedMax: number
  gapWords: number
  status: 'qualified' | 'too_short' | 'too_long' | 'unknown'
  score: number
  warnings?: string[]
}

export interface CheckChapterWordCountResponse {
  reportPath: string
  items: ChapterWordCountItem[]
  artifact: Record<string, unknown>
  warnings: string[]
}

export interface ExpandChaptersRequest {
  chapterPaths: string[]
  wordCountReportPath: string
  expansionTaskBookPath: string
  taskBookPath: string
  outlinePath: string
  scoringReportPath: string
  analysisReportPath: string
  contextReportPath?: string
  retrievalReportPath?: string
  outputDir: string
  mode?: 'strict' | 'draft'
}
```

### 7.3 UI 入口

在产物表中增加：

1. 当存在 `BID_CHAPTERS` 初稿时，显示“检查字数”。
2. 当存在 `BID_WORD_COUNT_REPORT` 且报告中有 `too_short` 章节时，显示“扩写不足章节”。
3. 扩写过程中显示 `扩写中...`，按钮禁用。
4. 扩写完成后，逐章注册扩写稿。

### 7.4 前端提示

扩写前弹出确认：

```text
系统将只扩写字数不足的章节，并保留原章节结构。
扩写可能耗时数分钟。是否继续？
```

扩写完成提示：

```text
章节扩写完成：成功 {success} 章，失败 {failed} 章。
请逐章查看扩写稿并确认。
```

---

## 8. local-backend 修改方案

### 8.1 多实例产物注册

当前 `BID_CHAPTERS` 如果仍按 `kind` 覆盖，会导致初稿和扩写稿互相覆盖。必须调整 upsert key。

推荐逻辑：

```text
普通产物：kind
章节产物：kind + chapterNumber + draftStage
```

### 8.2 扩写稿读取和保存

扩写稿仍是 Markdown 文件，可沿用现有本地 artifact 读取和 AI 修改能力。无需新增数据库、Docker、Redis、Kafka、MinIO。

---

## 9. OpenAPI 同步

新增 API 必须更新：

```text
cloud-backend/internal/core/apispec/cloud_spec.go
```

新增路径：

```text
POST /api/biaoshu/chapters/word-count/check
POST /api/biaoshu/chapters/expand
```

完成后运行：

```bash
cd cloud-backend
make gen-docs
make api-docs-check
```

不得手工编辑：

```text
cloud-backend/docs/API_REFERENCE.md
frontend/src/utils/api-types.generated.ts
```

---

## 10. 质量检查规则

扩写后必须生成 `91_章节扩写质量检查报告.md`。

### 10.1 自动检查项

| 检查项 | 规则 | 失败处理 |
|---|---|---|
| 字数 | `qualifiedMin <= wordCount <= qualifiedMax` | 标记需继续扩写或人工确认 |
| 禁用称谓 | 不得包含“我方”“我们” | 标记失败 |
| 商务价格 | 不得包含“报价”“预算金额”“投标总价”“价格承诺” | 标记失败 |
| 总结标题 | 不得包含“本章小结”“本章总结” | 标记失败 |
| 标题结构 | 一级标题仍为中文数字章节 | 标记 warning |
| 空泛表述 | 高频出现“加强管理”“确保落实”等无措施表达 | 标记 warning |
| 表格 | 扩写后章节尽量至少 1 个表格 | 标记 warning |

### 10.2 失败处理

1. 字数仍不足：生成二次扩写建议，不自动无限重试。
2. 禁用词失败：要求模型进行局部修正。
3. 商务价格失败：直接阻断进入下一阶段。
4. 标题结构失败：提示用户人工检查。

---

## 11. 测试计划

### 11.1 后端单元测试

```bash
cd cloud-backend
go test ./internal/agents/biaoshu/handler
```

测试用例：

1. 中文 Markdown 字数统计正确。
2. Markdown 表格和注释不干扰字数统计。
3. 从评分拆解表提取总分、章节分值。
4. 无法提取分值时 fallback 到均分。
5. `too_short` 章节生成扩写任务。
6. 单章扩写失败不影响其他章节。
7. 扩写稿 artifact metadata 包含 `draftStage=expanded`。

### 11.2 local-backend 测试

```bash
cd local-backend
go test ./internal/localagent
```

测试用例：

1. 同一章节初稿和扩写稿可以并存。
2. 不同章节扩写稿可以并存。
3. 重新生成同一章节扩写稿会替换同一 `chapterNumber + draftStage`，不影响其他章节。

### 11.3 前端测试

```bash
cd frontend
node scripts/biaoshu-artifact-logic-check.mjs
npm run build
```

测试用例：

1. `BID_WORD_COUNT_REPORT` 显示名称正确。
2. `BID_CHAPTER_EXPANSION_TASK_BOOK` 显示名称正确。
3. 多个 `BID_CHAPTERS` 扩写稿不互相覆盖。
4. 扩写按钮只在字数检查报告存在后显示。

### 11.4 全量验证

```bash
cd local-backend && go test ./...
cd ../cloud-backend && go test ./...
cd ../cloud-backend && make api-docs-check
cd ../frontend && npm run build
```

---

## 12. 实施任务拆分

### Task 1：补齐章节产物多实例能力

修改：

```text
frontend/src/pages/biaoshuArtifactLogic.ts
local-backend/internal/localagent/biaoshu_project_store.go
```

验收：

1. 多个章节初稿可同时显示。
2. 初稿和扩写稿可同时显示。
3. 重新扩写同一章节只替换该章节扩写稿。

### Task 2：实现字数检查服务

新增：

```text
chapter_word_count_service.go
chapter_word_count_handler.go
```

验收：

1. 能读取多个章节文件。
2. 能生成 `90_章节字数检查报告.md`。
3. 能返回结构化 `items`。

### Task 3：实现扩写任务书生成

在扩写服务中生成：

```text
06_章节扩写任务书.md
```

验收：

1. 每个不足章节都有扩写原因。
2. 每个不足章节都有建议补充字数。
3. 每个不足章节有对应评分项和建议扩写小节。

### Task 4：实现章节扩写服务

新增：

```text
chapter_expansion_service.go
chapter_expansion_handler.go
```

验收：

1. 只扩写 `too_short` 章节。
2. 输出 `章节/XX_第X章_扩写稿.md`。
3. 扩写失败不影响其他章节。
4. 扩写稿不包含代码块围栏。

### Task 5：实现扩写质量检查

输出：

```text
91_章节扩写质量检查报告.md
```

验收：

1. 检查禁用词。
2. 检查商务价格词。
3. 检查章节总结标题。
4. 检查扩写后字数是否进入合格范围。

### Task 6：前端接入

修改：

```text
frontend/src/services/api.ts
frontend/src/pages/biaoshuArtifactLogic.ts
frontend/src/pages/BiaoshuWorkbench.tsx
```

验收：

1. 能点击“检查字数”。
2. 能查看字数检查报告。
3. 能点击“扩写不足章节”。
4. 扩写稿能逐章查看和修改。

### Task 7：OpenAPI 和文档同步

修改：

```text
cloud-backend/internal/core/apispec/cloud_spec.go
```

运行：

```bash
cd cloud-backend
make gen-docs
make api-docs-check
```

验收：

1. Swagger 中出现两个新增接口。
2. API reference 无漂移。

---

## 13. 风险与缓解

### 13.1 风险：扩写变成灌水

缓解：

1. 扩写任务书必须指定评分点和小节。
2. prompt 明确禁止空泛表述。
3. QA 报告检查高频空泛词。

### 13.2 风险：单章 token 超限

缓解：

1. 默认按章节扩写。
2. 大章节可按二级小节拆分扩写。
3. 保留 `Mode=draft`，允许先扩写重点小节。

### 13.3 风险：扩写破坏原结构

缓解：

1. prompt 要求保留标题结构。
2. QA 报告比对扩写前后标题列表。
3. 标题缺失时标记失败。

### 13.4 风险：产物覆盖

缓解：

1. `BID_CHAPTERS` 使用 `chapterNumber + draftStage` 作为逻辑唯一键。
2. local-backend 和 frontend 同步修改。

### 13.5 风险：用户未确认就进入后续阶段

缓解：

1. 扩写稿生成后仍需用户逐章确认。
2. 未确认扩写稿不得进入去 AI 痕迹和合并阶段。

---

## 14. 验收标准

本次修改完成后，应满足：

1. 系统能基于已有章节初稿生成字数检查报告。
2. 系统能识别字数不足章节。
3. 系统能生成章节扩写任务书。
4. 系统能扩写不足章节并输出扩写稿。
5. 扩写稿与初稿都能在产物表中保留。
6. 扩写稿可查看、可 AI 修改、可人工确认。
7. 扩写后自动生成质量检查报告。
8. 新增 API 已同步 OpenAPI。
9. `local-backend` 不新增数据库、Docker、Redis、Kafka、MinIO 或 LLM API Key 依赖。
10. 基础验证命令全部通过。

---

## 15. 推荐落地顺序

推荐按以下顺序实施：

```text
1. 修正 BID_CHAPTERS 多实例注册
2. 实现字数检查报告
3. 实现扩写任务书
4. 实现扩写服务
5. 实现扩写质量检查报告
6. 前端接入按钮和状态
7. OpenAPI 同步
8. 全量验证
```

这样可以先保证产物不会覆盖，再逐步接入扩写能力，避免生成了文件但前端和本地项目无法正确管理。
