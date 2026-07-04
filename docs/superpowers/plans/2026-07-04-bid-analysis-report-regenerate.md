# 招标文件解析报告生成/重新生成实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将“招标文件原文解析”和“大模型生成解析报告”拆成两个明确步骤；只要已有 `00_招标文件原文解析.md`，用户就可以点击按钮生成或覆盖重生成 `00_招标文件解析报告.md`。

**Architecture:** `parse_bid_files` 只负责本地文件解析并生成原文解析产物；云端新增显式 API 读取原文解析文件，调用大模型生成最终解析报告；前端提供“生成解析报告/重新生成解析报告”按钮。报告重生成时直接覆盖旧的 `00_招标文件解析报告.md`，不创建新版本文件。

**Tech Stack:** Go cloud-backend, Gin HTTP handler, ModelGateway, Python biaoshu-tools, React + TypeScript frontend.

---

## 背景与问题

当前招标文件解析流程中，`parse_bid_files` 已经能成功生成：

```text
00_招标文件原文解析.md
```

但依赖动态 Agent 自动插入或执行 `bid_analysis_report` 时，实际运行链路不稳定，导致最终解析报告：

```text
00_招标文件解析报告.md
```

没有可靠生成。

新的方案将流程拆成两个用户可理解、可重试的步骤：

1. 本地解析招标文件，生成原文解析。
2. 云端读取原文解析，调用大模型生成解析报告。

第二步提供独立按钮，可在用户不满意解析报告时重新生成，并直接覆盖旧报告。

---

## 目标行为

### 第一步：解析招标文件

调用现有 `parse_bid_files` 工具。

输出：

```text
00_招标文件原文解析.md
```

并返回：

```json
{
  "raw_text_path": ".../00_招标文件原文解析.md",
  "report_path": ".../00_招标文件解析报告.md",
  "source_file": ".../招标文件.pdf"
}
```

### 第二步：生成解析报告

前端调用新增云端 API：

```http
POST /api/biaoshu/bid-analysis-report/generate
```

云端读取 `rawTextPath` 指向的 `00_招标文件原文解析.md`，调用大模型，生成并覆盖：

```text
00_招标文件解析报告.md
```

### 重生成规则

如果 `00_招标文件解析报告.md` 已存在：

```text
直接覆盖旧文件
```

如果不存在：

```text
创建新文件
```

---

## 文件改动概览

### Cloud Backend

- Modify: `cloud-backend/internal/agents/biaoshu/handler`
  - 新增解析报告生成 handler。
  - 读取原文解析文件。
  - 调用 ModelGateway。
  - 覆盖写入解析报告文件。

- Modify: `cloud-backend/cmd/tangying-ai-os/main.go`
  - 注册新增 API 路由。

- Modify: `cloud-backend/internal/core/apispec/cloud_spec.go`
  - 将新增 API 写入 OpenAPI spec。

- Modify: `cloud-backend/internal/core/apispec/cloud_schemas.go`
  - 增加请求/响应 schema。

- Optional Modify: `cloud-backend/internal/core/worker/tool/builtin/bid_analysis_report.go`
  - 若复用已有工具逻辑，则提取共享函数，避免 handler 和 tool 各写一套 prompt/文件写入逻辑。

### Biaoshu Tools

- Modify: `biaoshu-tools/app.py`
  - 确保 `parse_bid_files` 只生成 `00_招标文件原文解析.md`。
  - 确保返回 `raw_text_path`、`report_path`、`source_file`。
  - 不再在 Python 服务中调用大模型。

- Modify: `biaoshu-tools/register-tools.ps1`
  - 确认 `parse_bid_files` output schema 包含 `raw_text_path`。

### Frontend

- Modify: `frontend/src/pages/BiaoshuWorkbench.tsx`
  - 增加“生成解析报告/重新生成解析报告”按钮。
  - 绑定按钮 loading/error 状态。

- Modify: `frontend/src/pages/biaoshuArtifactLogic.ts`
  - 将 `BID_RAW_TEXT` 纳入产物阶段。
  - 保留 `BID_ANALYSIS` 作为解析报告阶段。

- Optional Create: `frontend/src/utils/biaoshuApi.ts`
  - 封装 `generateBidAnalysisReport` API 调用。

---

## API 设计

### Request

```http
POST /api/biaoshu/bid-analysis-report/generate
Content-Type: application/json
```

```json
{
  "rawTextPath": "E:/.../00_招标文件原文解析.md",
  "reportPath": "E:/.../00_招标文件解析报告.md",
  "sourceFile": "E:/.../招标文件.pdf",
  "projectId": "optional",
  "runId": "optional"
}
```

### Response

```json
{
  "success": true,
  "data": {
    "rawTextPath": "E:/.../00_招标文件原文解析.md",
    "reportPath": "E:/.../00_招标文件解析报告.md",
    "artifact": {
      "unitId": "bid-analysis",
      "kind": "BID_ANALYSIS",
      "name": "00_招标文件解析报告.md",
      "mimeType": "text/markdown",
      "storageRef": "E:/.../00_招标文件解析报告.md"
    }
  }
}
```

### Error Responses

原文解析文件不存在：

```json
{
  "success": false,
  "error": "rawTextPath does not exist"
}
```

原文解析文件为空：

```json
{
  "success": false,
  "error": "raw text file is empty"
}
```

模型调用失败：

```json
{
  "success": false,
  "error": "LLM call failed: ..."
}
```

报告写入失败：

```json
{
  "success": false,
  "error": "failed to write report: ..."
}
```

---

## Task 1: 后端新增生成解析报告 API

**Files:**

- Modify/Create: `cloud-backend/internal/agents/biaoshu/handler/bid_analysis_report_handler.go`
- Modify: `cloud-backend/cmd/tangying-ai-os/main.go`
- Test: `cloud-backend/internal/agents/biaoshu/handler/bid_analysis_report_handler_test.go`

### Steps

- [ ] 写失败测试：`rawTextPath` 存在时，接口读取原文并覆盖写入 `reportPath`。

Expected behavior:

```go
req := GenerateBidAnalysisReportRequest{
    RawTextPath: rawTextPath,
    ReportPath: reportPath,
    SourceFile: "E:/bid/test.docx",
}

// handler returns success
// reportPath exists
// reportPath contains generated model content
// reportPath contains source file path
// reportPath contains original raw text appendix
```

- [ ] 写失败测试：`rawTextPath` 不存在时返回 400。

Expected response:

```json
{
  "success": false,
  "error": "rawTextPath does not exist"
}
```

- [ ] 写失败测试：重复调用会覆盖旧报告。

Test setup:

```text
第一次模型返回：报告版本 A
第二次模型返回：报告版本 B
最终 reportPath 内容只包含版本 B
```

- [ ] 实现 request/response struct。

Suggested structs:

```go
type GenerateBidAnalysisReportRequest struct {
    RawTextPath string `json:"rawTextPath"`
    ReportPath  string `json:"reportPath"`
    SourceFile  string `json:"sourceFile,omitempty"`
    ProjectID   string `json:"projectId,omitempty"`
    RunID       string `json:"runId,omitempty"`
}

type GenerateBidAnalysisReportResponse struct {
    Success bool                            `json:"success"`
    Data    *GenerateBidAnalysisReportData  `json:"data,omitempty"`
    Error   string                          `json:"error,omitempty"`
}

type GenerateBidAnalysisReportData struct {
    RawTextPath string                 `json:"rawTextPath"`
    ReportPath  string                 `json:"reportPath"`
    Artifact    map[string]interface{} `json:"artifact"`
}
```

- [ ] 实现 handler。

Handler logic:

```text
1. Bind JSON request.
2. Validate rawTextPath.
3. Validate reportPath.
4. Read rawTextPath.
5. Reject empty content.
6. Call ModelGateway CapTextToText.
7. Build Markdown report.
8. os.MkdirAll(filepath.Dir(reportPath)).
9. os.WriteFile(reportPath) with overwrite behavior.
10. Return BID_ANALYSIS artifact metadata.
```

- [ ] 在 `main.go` 注册路由。

Suggested route:

```go
biaoshuReportHandler := biaoshu_handler.NewBidAnalysisReportHandler(gw)
biaoshuReportHandler.RegisterRoutes(r)
```

- [ ] 跑测试。

```bash
cd cloud-backend
go test ./internal/agents/biaoshu/handler -run BidAnalysisReport -count=1
```

Expected:

```text
ok
```

---

## Task 2: 抽出共享报告生成逻辑

**Files:**

- Modify/Create: `cloud-backend/internal/agents/biaoshu/handler/bid_analysis_report_service.go`
- Optional Modify: `cloud-backend/internal/core/worker/tool/builtin/bid_analysis_report.go`
- Test: `cloud-backend/internal/agents/biaoshu/handler/bid_analysis_report_handler_test.go`

### Steps

- [ ] 新增 `GenerateBidAnalysisReport` 服务函数。

Suggested signature:

```go
func GenerateBidAnalysisReport(
    ctx context.Context,
    gw *modelgateway.Gateway,
    rawTextPath string,
    reportPath string,
    sourceFile string,
) (*GeneratedBidAnalysisReport, error)
```

- [ ] 服务函数返回结构。

```go
type GeneratedBidAnalysisReport struct {
    RawTextPath string
    ReportPath  string
    Content     string
    Artifact    map[string]interface{}
}
```

- [ ] 将 prompt 放在服务层。

Prompt requirements:

```text
1. 项目基本信息
2. 投标人资格条件
3. 技术要求与服务范围
4. 商务条款、工期、质量、安全要求
5. 评分办法与关键得分点
6. 投标文件组成、格式与递交要求
7. 风险点、澄清点与后续标书编写建议
```

- [ ] 确保生成的 Markdown 包含：

```markdown
# 招标文件解析报告

**源文件**: ...

---

<模型生成内容>

---

## 附录：招标文件原文

<原文解析内容>
```

- [ ] 跑测试。

```bash
cd cloud-backend
go test ./internal/agents/biaoshu/handler -run BidAnalysisReport -count=1
```

---

## Task 3: 原文解析加入产物追踪

**Files:**

- Modify: `frontend/src/pages/biaoshuArtifactLogic.ts`
- Optional Modify: `cloud-backend/internal/core/artifact/materializer.go`
- Test: existing frontend type check

### Steps

- [ ] 在 `BIAOSHU_STAGES` 中增加原文解析阶段。

Expected stage:

```ts
{
  key: 'raw-parse',
  label: '招标文件原文解析',
  tool: 'parse_bid_files',
  kind: 'BID_RAW_TEXT',
  owner: '文件解析',
}
```

- [ ] 保留解析报告阶段。

Expected stage:

```ts
{
  key: 'parse-report',
  label: '招标文件解析报告',
  tool: 'bid_analysis_report',
  kind: 'BID_ANALYSIS',
  owner: 'AI分析',
}
```

- [ ] 如果 artifact materializer 不支持 `BID_RAW_TEXT`，补充 artifact kind 支持。

Expected behavior:

```text
BID_RAW_TEXT artifact appears in artifact list.
BID_ANALYSIS artifact appears after report generation.
```

- [ ] 跑类型检查。

```bash
cd frontend
npx.cmd tsc -b
```

Expected:

```text
exit code 0
```

---

## Task 4: 前端新增生成/重新生成按钮

**Files:**

- Modify: `frontend/src/pages/BiaoshuWorkbench.tsx`
- Optional Create: `frontend/src/utils/biaoshuApi.ts`

### Steps

- [ ] 封装 API 调用。

Suggested function:

```ts
export async function generateBidAnalysisReport(input: {
  rawTextPath: string
  reportPath: string
  sourceFile?: string
  projectId?: string
  runId?: string
}) {
  const response = await fetch(`${cloudApiBase}/biaoshu/bid-analysis-report/generate`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
  if (!response.ok) {
    throw new Error(await response.text())
  }
  return response.json()
}
```

- [ ] 在页面中找到 `BID_RAW_TEXT` 产物。

Logic:

```ts
const rawTextArtifact = artifacts.find((item) => item.kind === 'BID_RAW_TEXT')
const analysisArtifact = artifacts.find((item) => item.kind === 'BID_ANALYSIS')
```

- [ ] 根据产物状态显示按钮。

Rules:

```text
rawTextArtifact exists && !analysisArtifact:
  显示“生成解析报告”

rawTextArtifact exists && analysisArtifact exists:
  显示“重新生成解析报告”

rawTextArtifact missing:
  不显示按钮或 disabled
```

- [ ] 点击按钮时调用新 API。

Required input:

```ts
{
  rawTextPath: rawTextArtifact.storageRef,
  reportPath: deriveReportPath(rawTextArtifact.storageRef),
  sourceFile: rawTextArtifact.metadata?.sourceFile,
  projectId,
  runId,
}
```

- [ ] 实现 `deriveReportPath`。

Expected conversion:

```text
.../00_招标文件原文解析.md
->
.../00_招标文件解析报告.md
```

- [ ] 成功后刷新任务/产物列表。

Expected behavior:

```text
按钮 loading 结束
解析报告 artifact 可见
重新生成后报告内容被覆盖
```

- [ ] 跑类型检查。

```bash
cd frontend
npx.cmd tsc -b
```

---

## Task 5: OpenAPI 文档更新

**Files:**

- Modify: `cloud-backend/internal/core/apispec/cloud_spec.go`
- Modify: `cloud-backend/internal/core/apispec/cloud_schemas.go`
- Generated: `cloud-backend/docs/API_REFERENCE.md`
- Generated: `frontend/src/utils/api-types.generated.ts`

### Steps

- [ ] 在 `cloud_spec.go` 添加路由描述。

Route:

```text
POST /api/biaoshu/bid-analysis-report/generate
```

- [ ] 在 `cloud_schemas.go` 添加请求/响应 schema。

Schema names:

```text
BiaoshuGenerateBidAnalysisReportRequest
BiaoshuGenerateBidAnalysisReportResponse
```

- [ ] 重新生成 API 文档。

```bash
cd cloud-backend
make gen-docs
```

- [ ] 校验 API 文档同步。

```bash
cd cloud-backend
make api-docs-check
```

---

## Task 6: 端到端手工验证

### Steps

- [ ] 启动云端后端。

```bash
cd cloud-backend
go build -o build/tangying-ai-os cmd/tangying-ai-os/main.go
./build/tangying-ai-os
```

- [ ] 启动前端。

```bash
cd frontend
npm run dev
```

- [ ] 在界面上传或选择招标文件。

Expected:

```text
生成 00_招标文件原文解析.md
产物列表出现“招标文件原文解析”
```

- [ ] 点击“生成解析报告”。

Expected:

```text
生成 00_招标文件解析报告.md
产物列表出现“招标文件解析报告”
```

- [ ] 再次点击“重新生成解析报告”。

Expected:

```text
旧的 00_招标文件解析报告.md 被覆盖
文件路径不变
内容更新时间或内容发生变化
```

---

## Verification Checklist

- [ ] `parse_bid_files` 不调用大模型。
- [ ] `00_招标文件原文解析.md` 可以单独作为产物追踪。
- [ ] 后端 API 可以只依赖 `rawTextPath` 生成报告。
- [ ] 重生成时覆盖旧 `00_招标文件解析报告.md`。
- [ ] 前端按钮只在有原文解析时可用。
- [ ] 模型调用失败时前端能显示错误。
- [ ] 所有测试通过。

## Verification Commands

```bash
cd cloud-backend
go test ./...
```

```bash
cd frontend
npx.cmd tsc -b
```

```bash
python -m unittest biaoshu-tools\test_app.py
python -m py_compile biaoshu-tools\app.py biaoshu-tools\test_app.py
```

---

## Notes

- 这次方案不再依赖动态 Agent 自动插入 `bid_analysis_report`。
- 动态 Agent 自动流程可以保留，但前端主路径应使用显式 API。
- 报告重生成直接覆盖旧文件，不做历史版本管理。
- 原文解析产物是后续所有报告生成的前置条件。
