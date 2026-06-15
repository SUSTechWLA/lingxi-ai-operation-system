# 04d-API-and-Event-Contracts: 标书生成 — 接口与事件契约

## 1. Core API (新增 /api/bid/*)

所有响应使用 `{code: int, message: string, data: any}` 信封。

### 1.1 POST /api/bid/projects
创建标书项目。
- Request: `{name: string, template_id?: string, industry?: string}`
- Response: `{code: 200, data: {project: {id, name, status, created_at}}}`

### 1.2 GET /api/bid/projects
标书项目列表。
- Query: `?offset=0&limit=20&status=&userId=`
- Response: `{code: 200, data: {items: [...], total: int}}`

### 1.3 GET /api/bid/projects/:id
项目详情。
- Response: `{code: 200, data: {project: {id, name, status, progress, task_id, tender_analysis, structure, chapters: [...], config}}}`

### 1.4 PUT /api/bid/projects/:id
更新项目结构/配置。
- Request: `{structure?: [...], config?: {}}`
- Response: `{code: 200, data: {project: {...}}}`

### 1.5 DELETE /api/bid/projects/:id
删除项目。
- Response: `{code: 200, message: "deleted"}`

### 1.6 POST /api/bid/projects/:id/upload-tender
上传招标文件 (multipart)。
- Request: multipart form `{file: <binary>, options?: {}}`
- Response: `{code: 200, data: {file_path, file_name, file_size}}`

### 1.7 POST /api/bid/projects/:id/start
启动标书生成。内部: 创建 Task → 构建 DAG → 提交 DAG。
- Response: `{code: 200, data: {task_id, message: "started"}}`

### 1.8 POST /api/bid/projects/:id/pause
暂停生成。
- Request: `{reason?: string}`
- Response: `{code: 200, data: {message: "paused"}}`

### 1.9 POST /api/bid/projects/:id/resume
恢复生成。
- Response: `{code: 200, data: {message: "resumed"}}`

### 1.10 POST /api/bid/projects/:id/chapters/:chId/approve
审核通过章节。
- Response: `{code: 200, data: {chapter: {...}}}`

### 1.11 POST /api/bid/projects/:id/chapters/:chId/reject
驳回章节。
- Request: `{comment: string}`
- Response: `{code: 200, data: {chapter: {...}}}`

### 1.12 POST /api/bid/projects/:id/chapters/:chId/regenerate
重新生成章节。
- Response: `{code: 200, data: {node_id, message: "regenerating"}}`

### 1.13 POST /api/bid/projects/:id/export
触发导出。
- Request: `{format?: "docx"|"pdf", cover_info?: {}}`
- Response: `{code: 200, data: {node_id, message: "exporting"}}`

### 1.14 GET /api/bid/projects/:id/export/status
查询导出状态。
- Response: `{code: 200, data: {status, download_url?, error?}}`

### 1.15 GET /api/bid/projects/:id/progress
查询进度。
- Response: `{code: 200, data: {stage, progress, current_step, total_steps}}`

### 1.16 GET /api/bid/templates
标书模板列表。
- Response: `{code: 200, data: {templates: [...]}}`

### 1.17 GET /api/bid/projects/:id/trace
项目 Trace (透传现有 Trace API)。
- Response: `{code: 200, data: {task, contexts: [...]}}`

## 2. Event 契约

### 2.1 ai.bid.stage.change (新增)
Core → Frontend/Kafka
```json
{
  "project_id": "xxx",
  "stage": "PARSING|PLANNING|GENERATING|REVIEWING|EXPORTING|COMPLETED",
  "progress": 0.35,
  "timestamp": "2026-06-15T20:30:00Z"
}
```

### 2.2 ai.bid.chapter.approved (新增)
Core → Frontend/Kafka
```json
{
  "project_id": "xxx",
  "chapter_id": "ch-1",
  "node_id": "xxx-ch-1",
  "approved_by": "user_id",
  "timestamp": "2026-06-15T20:30:00Z"
}
```

### 2.3 ai.bid.chapter.rejected (新增)
Core → Frontend/Kafka
```json
{
  "project_id": "xxx",
  "chapter_id": "ch-1",
  "node_id": "xxx-ch-1",
  "comment": "需要补充项目经验案例",
  "rejected_by": "user_id",
  "timestamp": "2026-06-15T20:30:00Z"
}
```

### 2.4 复用现有 Events
- `ai.node.ready` — 触发 doc_parser, chapter_generator, compliance_checker, doc_exporter
- `ai.node.result` — 接收各工具执行结果
- `ai.task.completed` / `ai.task.failed` — Task 生命周期
- `ai.node.executed` / `ai.node.failed` — Node 审计

## 3. 向后兼容

- 不修改现有 `/api/task/*`, `/api/node/*` 接口
- 不修改现有 Kafka topic
- bid_projects 表新增，不影响现有表
- 新增的 Context type 是追加，不破坏现有查询
