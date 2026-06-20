# 04a-Solution-Blueprint: 自动化标书生成 — Solution 蓝图

## 1. 总体方案

标书生成是**多阶段异步长任务**，使用 AIOS 的 DAG 编排能力串联各阶段。整体架构：投标专员在前端创建标书项目 → Core 创建 Task 并驱动 DAG → 各阶段通过外部 Tool 完成 → 人工审核节点暂停 DAG → 导出为 Word。

```
┌─────────────────────────────────────────────────────────┐
│                      Frontend                            │
│  标书项目管理 │ 招标文件上传 │ 章节审核 │ 进度追踪      │
└──────────────┬──────────────────────────────────────────┘
               │ HTTP API (/api/bid/*)
┌──────────────▼──────────────────────────────────────────┐
│                    AIOS Core                             │
│  ┌─────────┐  ┌──────────┐  ┌────────┐  ┌───────────┐  │
│  │Bid Entry│  │Orchestr. │  │Tool    │  │Trace/     │  │
│  │(新API)  │  │(现有DAG) │  │Bridge  │  │Context    │  │
│  └─────────┘  └──────────┘  └────────┘  └───────────┘  │
└──────────────┬──────────────────────────────────────────┘
               │ Tool Manifest / HTTP / Event
┌──────────────▼──────────────────────────────────────────┐
│                External Tools                            │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌───────────┐  │
│  │DocParser │ │Knowledge │ │Chapter   │ │DocExporter│  │
│  │招标解析  │ │企业知识库│ │Gen 章节  │ │标书导出   │  │
│  └──────────┘ └──────────┘ └──────────┘ └───────────┘  │
└─────────────────────────────────────────────────────────┘
```

## 2. 职责边界

### AIOS Core 负责 (需要修改 Core)
1. **Bid API 端点** (`/api/bid/*`) — 标书项目 CRUD、DAG 触发
2. **标书 DAG 模板** — 预定义的标书生成工作流
3. **人工审核节点** — DAG 暂停/恢复机制增强
4. **进度事件** — 标书阶段的进度百分比事件
5. **标书数据模型** — bid_projects, bid_chapters 表

### 不修改 Core — 通过 Workflow 配置实现
1. DAG 节点编排逻辑（阶段顺序、依赖关系）
2. 章节生成 prompt 模板
3. 合规检查规则配置
4. 标书模板定义

### 外部 Tool 负责 (不修改 Core)
1. 招标文件解析 (PDF/Word OCR/NLP)
2. 企业资料检索与匹配
3. 章节内容 AI 生成 (调用 LLM)
4. Word/PDF 文档导出
5. 合规检查引擎

### 前端负责 (不修改 Core)
1. 标书项目管理页面
2. 招标文件上传界面
3. 章节规划编辑和审核
4. 标书预览和导出按钮
5. 进度仪表盘

## 3. 核心 DAG 流程

```
[Bid Project Created]
        │
        ▼
  [1. Parse Tender Doc]  ← 外部工具: doc_parser
        │
        ▼
  [2. Plan Structure]    ← LLM: 章节规划
        │
        ▼
  [3. HR: Approve Plan]  ← 人工审核节点 (暂停)
        │
   ┌────┴────┐
   ▼         ▼         ▼
[4a.Ch1]  [4b.Ch2]  [4c.ChN]  ← 并发章节生成 (外部工具)
   │         │         │
   └────┬────┘         │
        ▼              │
  [5. HR: Review Ch]   │  ← 逐章人工审核
        │              │
   (驳回则回 4x)       │
        ▼              ▼
  [6. Compliance Check]  ← 外部工具: compliance_checker
        │
        ▼
  [7. HR: Final Review]  ← 终稿审核
        │
        ▼
  [8. Export to Word]    ← 外部工具: doc_exporter
        │
        ▼
    [Completed]
```

## 4. 数据模型设计

### 新表: bid_projects
```sql
CREATE TABLE bid_projects (
  id VARCHAR(64) PRIMARY KEY,
  user_id VARCHAR(64),
  name VARCHAR(255),
  status VARCHAR(32),  -- DRAFT/PARSING/PLANNING/GENERATING/REVIEWING/EXPORTING/COMPLETED
  task_id VARCHAR(64) REFERENCES ai_task(id),
  tender_file_path VARCHAR(512),  -- MinIO path
  tender_analysis JSONB,  -- 解析结果
  structure JSONB,  -- 章节结构
  config JSONB,  -- 配置参数
  progress REAL DEFAULT 0,  -- 进度 0.0-1.0
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

### 新表: bid_chapters
```sql
CREATE TABLE bid_chapters (
  id VARCHAR(64) PRIMARY KEY,
  project_id VARCHAR(64) REFERENCES bid_projects(id),
  node_id VARCHAR(64) REFERENCES ai_node(id),
  title VARCHAR(255),
  content TEXT,
  status VARCHAR(32),  -- PENDING/GENERATING/REVIEWING/APPROVED/REJECTED
  review_comment TEXT,
  score_items JSONB,  -- 关联的评分项
  sort_order INT,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

### 新表: bid_templates
```sql
CREATE TABLE bid_templates (
  id VARCHAR(64) PRIMARY KEY,
  name VARCHAR(255),
  category VARCHAR(128),
  structure JSONB,  -- 预定义章节结构
  workflow_dag JSONB,  -- 预定义 DAG 模板
  created_at TIMESTAMPTZ DEFAULT NOW()
);
```

### 扩展现有表
- `ai_context`: 新增 context_type 值: `BID_STAGE_CHANGE`, `BID_CHAPTER_APPROVED`, `BID_CHAPTER_REJECTED`

## 5. API 设计

所有新 API 统一使用 `{code, message, data}` 信封，注册在 `/api/bid/*`:

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/api/bid/projects` | 创建标书项目 |
| GET | `/api/bid/projects` | 列表 (分页、状态筛选) |
| GET | `/api/bid/projects/:id` | 获取项目详情 (含章节、进度) |
| PUT | `/api/bid/projects/:id` | 更新项目 (结构调整) |
| DELETE | `/api/bid/projects/:id` | 删除项目 |
| POST | `/api/bid/projects/:id/start` | 启动标书生成 (提交 DAG) |
| POST | `/api/bid/projects/:id/pause` | 暂停生成 |
| POST | `/api/bid/projects/:id/resume` | 恢复生成 |
| POST | `/api/bid/projects/:id/chapters/:chId/approve` | 审核通过章节 |
| POST | `/api/bid/projects/:id/chapters/:chId/reject` | 驳回章节 (附带意见) |
| POST | `/api/bid/projects/:id/chapters/:chId/regenerate` | 重新生成章节 |
| POST | `/api/bid/projects/:id/export` | 触发导出 |
| GET | `/api/bid/projects/:id/export/status` | 查询导出状态 |
| GET | `/api/bid/projects/:id/progress` | 查询当前进度 |
| POST | `/api/bid/projects/:id/upload-tender` | 上传招标文件 |
| GET | `/api/bid/templates` | 获取标书模板列表 |
| GET | `/api/bid/projects/:id/trace` | 获取项目 Trace |

## 6. 事件设计

复用现有 Kafka topic + 新增:

| Topic | Purpose |
|-------|---------|
| `ai.node.ready` (现有) | 触发外部工具执行 |
| `ai.node.result` (现有) | 接收工具执行结果 |
| `ai.bid.stage.change` (新增) | 标书阶段变更 |
| `ai.bid.chapter.approved` (新增) | 章节审核通过 |
| `ai.bid.chapter.rejected` (新增) | 章节审核驳回 |

## 7. 里程碑

| 里程碑 | 内容 | 依赖 |
|--------|------|------|
| M1: Core Gap 确认 | 确定 Core 修改范围 | Phase 3 |
| M2: 外部工具定义 | doc_parser, knowledge_base, chapter_generator, compliance_checker, doc_exporter 能力规格 | Phase 2A |
| M3: Core 数据模型 | bid_projects/bid_chapters 表 + 迁移 | M1 通过 |
| M4: Core API | `/api/bid/*` 端点实现 | M3 |
| M5: 人工审核节点 | DAG 暂停/恢复 + 审核事件 | M4 |
| M6: 外部工具集成 | 工具注册 + DAG 联通 | M2 + M4 |
| M7: 前端开发 | 标书管理页面 | M4 APIs 就绪 |
| M8: 端到端测试 | 完整标书生成流程 | M5 + M6 + M7 |
