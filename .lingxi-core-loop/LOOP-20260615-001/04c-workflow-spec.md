# 04c-Workflow-Spec: 标书生成 — Workflow/DAG/Session 规格

## 1. 标书生成 DAG 模板

### 节点定义

| Node ID | Type | Name | Tool | 说明 |
|---------|------|------|------|------|
| parse_tender | TOOL | 招标文件解析 | doc_parser | 解析 PDF/Word，输出评分项和技术要求 |
| plan_structure | LLM | 章节规划 | llm_api | LLM 规划标书目录结构 |
| hr_approve_plan | CONTROL | 审核-章节规划 | — | 人工审核：确认目录结构 |
| gen_chapter_N | TOOL | 生成第N章 | chapter_generator | 并发章节生成 |
| hr_review_N | CONTROL | 审核-第N章 | — | 人工审核：逐章审核 |
| compliance_check | TOOL | 合规检查 | compliance_checker | 评分项覆盖率+格式检查 |
| hr_final | CONTROL | 终稿审核 | — | 人工审核：整体确认 |
| export_doc | TOOL | 导出Word | doc_exporter | 导出 .docx |

### DAG 边 (依赖关系)

```
parse_tender → plan_structure → hr_approve_plan
hr_approve_plan → gen_chapter_1, gen_chapter_2, ..., gen_chapter_N  (并联)
gen_chapter_N → hr_review_N
hr_review_N → compliance_check (hr_review_N 全部完成后)
compliance_check → hr_final → export_doc
```

### 条件分支
- `gen_chapter_N`: condition = `hr_approve_plan.status == success` (仅当审核通过才生成)
- `hr_review_N` 驳回 → 触发 gen_chapter_N 重试 (通过 `RetryNode` API)
- `compliance_check` 未通过 → 可选择性回退到 gen_chapter_N

### 节点输入/输出

**parse_tender**
- Input: `{tender_file_path, options: {extract_tables, extract_images}}`
- Output: `{project_info: {...}, score_items: [...], tech_requirements: [...], qualification_requirements: [...]}`

**plan_structure**
- Input: `{tender_analysis: {{parse_tender.output}}, template_id, style_keywords}`
- Output: `{chapters: [{id, title, sub_chapters, score_items, required_materials}]}`

**gen_chapter_N**
- Input: `{chapter: {{plan_structure.output.chapters[N]}}, tender_requirements: {{parse_tender.output}}, materials: [...], style: {tone, length}}`
- Output: `{content, references, tables: [...]}`

**compliance_check**
- Input: `{chapters: [...], score_items: {{parse_tender.output.score_items}}}`
- Output: `{coverage_report: [{score_item, covered, chapter_id}], format_issues: [...], overall_score}`

**export_doc**
- Input: `{chapters: [...], project_info, template_config, cover_info}`
- Output: `{file_path, file_size, download_url}`

## 2. Session 管理 (对标书项目的封装)

每个标书项目对应:
- 1 个 Orchestrator Task (task_id)
- 1 个 DAG (提交到该 Task)
- N 个 Chapter 记录 (bid_chapters 表)
- 1 组 Context 事件 (Trace)

不创建新的 Session 概念 — 复用现有 Task + Context 体系。

## 3. Artifact 管理

| Artifact | 存储位置 | 访问方式 |
|----------|----------|----------|
| 招标文件原件 | MinIO | presigned URL |
| 解析结果 | bid_projects.tender_analysis (JSONB) | API |
| 章节内容 | bid_chapters.content | API |
| 导出 .docx | MinIO | presigned URL (24h) |
| Trace 记录 | ai_context 表 | GET /api/bid/projects/:id/trace |

## 4. 人工检查点

使用现有 `PauseTask` / `ResumeTask` 机制:
1. CONTROL 节点执行时，Orchestrator 自动暂停 Task
2. 前端显示审核界面
3. 用户点击通过 → API 调用 `ApproveChapter` → Core Resume Task
4. 用户点击驳回 → API 调用 `RejectChapter` → Core 重试上游节点

审核不进入 Core 状态机 — 它是 Orchestrator 外部的前端动作。
