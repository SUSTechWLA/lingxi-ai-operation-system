# Skill：躺营AIOS 自媒体运营功能全量升级（基于现有架构扩展）

## 一、Skill 概述

本 Skill 用于指导在躺营AI OS 现有架构上，通过**复用并扩展现有的 Publish 模块、Orchestrator、Worker、Tool 接口和事件系统**，从当前的「单次发布」能力升级为覆盖「素材管理 → 内容生成 → 对话创作 → 运营配套」的自媒体全链路能力。所有新增功能遵循现有分层规范、API 规范和扩展点机制。

## 二、前置校验条件

执行前确认以下条件满足（不满足则终止并告知用户）：

1. `PublishPage.tsx` 及其子组件（`UploadCard`、`TitleInput`、`AIHelperPanel` 等）可正常运行。
2. `PublishHandler` 提供 `/api/publish`、`/api/ai/generate`、`/api/ai/polish` 等端点，且鉴权中间件可用。
3. `Tool` 接口（`Name`, `Description`, `Type`, `Execute`, `ValidateParameters`）已定义且有 `llm_api`、`bash`、`polisher` 等实现。
4. `ToolRegistry` 在 `main.go` 中注册工具，Orchestrator 可通过节点 `type` 字段找到对应 Tool。
5. Outbox 模式已实现（`outbox` 表 + Relay 协程），`ai.node.ready` 主题正常工作。
6. PostgreSQL、MinIO、Redis、Qdrant、Redpanda 均可通过 Docker Compose 启动。
7. Context 审计表 `ai_context` 有 `source_module`、`metadata` 等字段，可记录执行指标。

## 三、核心执行原则

1. **严格遵循现有规范**：Go 代码沿用 handler/service/repository/model 分层；前端沿用 React + TypeScript + TailwindCSS + Zustand。
2. **所有新增能力封装为 Tool**：通过 Orchestrator 调度执行，走 Outbox 事件和 Context 审计，不新增独立的后台任务。
3. **复用现有 API 风格**：新端点返回格式 `{ "code": 0, "message": "success", "data": ... }`，错误复用现有错误码体系。
4. **最小化表结构变更**：仅用 `ALTER TABLE ADD COLUMN IF NOT EXISTS` 扩展，不改动现有列。
5. **前端增量开发**：在现有 `PublishPage` 基础上新增面板和页面，复用 `Zustand` store 和 Axios 封装，保持 TailwindCSS 风格。

## 四、分阶段执行步骤

### 阶段 1：架构扫描与契约定义

**执行动作**：
1. 读取 `internal/worker/tools/` 下现有 Tool 实现，确认接口签名和注册方式。
2. 读取 `internal/handler/publish/` 的路由注册方式。
3. 读取 `frontend/src/` 的目录结构、Zustand store 写法、Axios 拦截器。
4. 定义新增 Tool 清单及其参数/输出结构（如下表），写入《工具契约文档》。
5. 定义新增 RESTful 端点清单（如 `/api/ai/generate-from-media`），明确请求体和响应体。
6. 生成前端 Mock 数据（JSON 文件），放在 `frontend/src/mocks/` 下。

**新增 Tool 规划**：

| Tool 名称 | 类型 | 用途 | 关键参数 | 关键输出 |
|-----------|------|------|----------|----------|
| `media_analyzer` | CUSTOM | 素材标签/向量生成 | `file_ids`[]string | `tags`[]string, `embedding` |
| `content_generator` | CUSTOM | 多素材合成完整内容包 | `media_ids`, `platform`, `style` | `title`, `description`, `script`, `tags` |
| `content_checker` | CUSTOM | 合规性检测 | `content` | `is_compliant`, `violations` |
| `platform_adapter` | CUSTOM | 跨平台内容适配 | `source_content`, `target_platform` | `adapted_content` |

**新增 API 规划**：

| 端点 | 方法 | 用途 |
|------|------|------|
| `/api/ai/generate-from-media` | POST | 基于上传媒体生成内容（已有，需扩展） |
| `/api/media/list` | GET | 素材列表（分页+筛选） |
| `/api/media/:id` | GET | 素材详情 |
| `/api/media/:id/tags` | PUT | 更新素材标签 |
| `/api/media/upload-batch` | POST | 批量上传 |
| `/api/content/list` | GET | 内容历史列表 |
| `/api/content/:id/versions` | GET | 内容版本历史 |
| `/api/trace/:taskId` | GET | 任务追踪（已有，确保兼容） |

**交付物**：
- 《工具契约文档》（含参数 JSON Schema 和输出格式）
- 前端 Mock 数据文件
- 经用户确认后进入阶段 2

---

### 阶段 2：素材管理 Tool 与 API 实现

**执行动作**：

**后端**：
1. 在 `internal/worker/tools/` 下新建 `media_analyzer.go`，实现 `Tool` 接口。
2. `Execute` 内调用 `callOpenAI()`（复用 `PublishService` 的通用封装），将图片/视频转文字，输出标签和向量。
3. 在 `main.go` 的 `initTools()` 中注册 `MediaAnalyzerTool{}`。
4. 在 `internal/handler/media/` 新增 handler（`MediaHandler`），提供素材列表、详情、标签更新接口，复用现有 `PostgreSQL` 连接池。
5. 素材元数据存储于新表 `media_assets`（字段：`id`, `user_id`, `original_name`, `mime_type`, `size`, `minio_path`, `tags JSONB`, `embedding_id`, `created_at`）。
6. 素材文件复用现有 MinIO 上传逻辑；向量存储复用现有 Qdrant 连接。

**数据库迁移**：
```sql
CREATE TABLE IF NOT EXISTS media_assets (
    id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL,
    original_name TEXT NOT NULL,
    mime_type VARCHAR(50),
    size BIGINT,
    minio_path TEXT,
    tags JSONB DEFAULT '[]',
    embedding_id VARCHAR(64),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_media_assets_user_id ON media_assets(user_id);
```

**前端**：
1. 在 `PublishPage.tsx` 的 `UploadCard` 组件中，新增「上传历史」按钮，点击弹出素材库侧边面板（`MediaLibraryPanel.tsx`）。
2. 素材库面板从 `/api/media/list` 拉取数据，支持按标签筛选、多选插入到当前发布表单。
3. 所有网络请求使用现有 Axios 实例（`frontend/src/api/client.ts`）。

**校验标准**：
- 通过 Postman 或前端上传图片后，`/api/media/list` 可返回记录且 tags 已填充。
- `MediaAnalyzerTool` 可通过 Orchestrator 提交单节点 DAG 成功执行。

---

### 阶段 3：内容生成引擎 Tool 实现

**执行动作**：

**后端**：
1. 新建 `internal/worker/tools/content_generator.go`，实现 `Tool` 接口：
   - 入参：`media_ids`, `platform`, `style`, `keywords`。
   - 内部：拼装提示词模板，调用 `PublishService.callOpenAI()`，返回结构化 JSON（标题、简介、脚本、标签等）。
2. 新建 `internal/worker/tools/content_checker.go`：
   - 入参：`content`（上一步输出）。
   - 内部：调用大模型审查敏感词/违规内容，返回 `is_compliant` 和 `violations` 列表。
3. 新建 `internal/worker/tools/platform_adapter.go`：
   - 入参：`source_content` + `target_platform`。
   - 内部：调用大模型将内容适配为目标平台格式（字数、语气、标签）。
4. 在 `main.go` 中注册这三个工具。
5. 在 `PublishService` 中新增方法 `GenerateFromMedia()`，逻辑：
   - 创建 DAG：[`media_analyzer` → `content_generator` → `content_checker` → `platform_adapter`]。
   - 根据用户选择的平台，`platform_adapter` 节点使用条件分支（`condition` 字段）只适配对应平台。
   - 提交 Orchestrator，返回 `taskId`。
6. 新增端点 `POST /api/ai/generate-from-media`（扩展现有接口），调用 `GenerateFromMedia()`。

**前端**：
1. 在右侧面板新增「内容生成工作台」（`ContentWorkbench.tsx`），表单包含：选择已上传媒体、目标平台、风格关键词。
2. 点击「生成」调用 `/api/ai/generate-from-media`，显示加载动画（复用现有 AI 加载遮罩组件）。
3. 生成结果在中央编辑区展示，标题/简介自动填入 `TitleInput` 和 `DescriptionInput`，脚本/标签展示在额外区域。

**校验标准**：
- 选择 2 张图片、选择平台“小红书”，点击生成后，`TitleInput` 和 `DescriptionInput` 自动填入 AI 生成的内容。
- 后端可通过 Context 审计追溯到 `media_analyzer` → `content_generator` 的完整链路。

---

### 阶段 4：对话式创作（Translator 增强）

**执行动作**：

**后端**：
1. 在 `internal/translator/` 下新增 `ChatTranslator`，提供 `POST /api/translate/chat`：
   - 入参：`session_id`, `message`, `history`[]。
   - 内部：调用大模型，分析用户意图，识别缺失的创作要素（如“缺少目标平台”），返回引导问题或补全后的 DAG。
2. 会话状态存储在 Redis（Key：`chat:session:{session_id}`），TTL 24 小时。
3. 大模型回复时可附带“建议动作”，如「建议上传素材」「是否需要生成脚本」，前端据此展示引导卡片。

**前端**：
1. 在 `AIHelperPanel.tsx` 中新增「对话创作」Tab，展示聊天界面（`ChatPanel.tsx`）。
2. 用户输入自然语言（如“写一篇关于北京故宫的小红书游记”）。
3. AI 逐步反问答疑后，调用 `/api/ai/generate-from-media` 生成最终内容并填入编辑器。

**校验标准**：
- 在聊天面板输入模糊需求后，AI 能反问“请上传一张故宫图片”，上传后继续生成完整内容。
- 对话历史可在 24 小时内恢复。

---

### 阶段 5：运营配套 Tool 实现

**执行动作**：

**后端**：
1. 在 `platform_adapter.go` 中完善多平台规则库（抖音、小红书、微博、B站），作为 Tool 的默认提示词参数。
2. 新增 `internal/worker/tools/batch_processor.go`：
   - 入参：`action`（"analyze"/"generate"），`media_ids`[]。
   - 内部：循环为每个 media_id 提交一个 DAG（通过 Orchestrator），返回子任务 ID 列表。
3. 新增端点 `POST /api/media/batch-process`，支持批量素材解析。

**前端**：
1. 在 `UploadCard.tsx` 中新增批量拖拽上传功能（复用 `react-dropzone`）。
2. 上传完成后展示批量处理进度（通过轮询 `/api/trace/:taskId` 获取各子任务状态）。

**校验标准**：
- 拖拽 5 张图片到 `UploadCard`，可自动调用批量处理，生成 5 个解析 DAG 并逐个在后台完成。

---

### 阶段 6：与现有能力融合

**执行动作**：
1. 确认所有新表通过 `user_id` 关联现有用户表。
2. 所有新 Tool 的输出中带上 `startedAt`、`durationMs`、`exitCode`，进入 Context 审计。
3. 前端所有新接口的 Axios 请求走现有 `Vite proxy → /api/*`，鉴权 token 从状态管理中获取。
4. 错误码、提示语、Toast 弹窗（2.5s 消失）复用现有 UX 规范。
5. 沙箱执行：`bash` 工具已具备命令白名单+目录限制，`media_analyzer` 若需在沙箱内调 Python，只需在 `Execute` 内构造 bash 调用脚本。

**校验标准**：
- 未登录状态下访问新接口返回 401。
- 任意 Tool 执行失败，错误信息出现在 `/api/trace/:taskId` 的 metadata 中。

---

### 阶段 7：部署与联调

**执行动作**：
1. `docker-compose.yml` 新增 `media_assets` 表的初始化 SQL。
2. 提示词模板通过 `ai_prompt_templates` 表初始化（若无此表则新增，字段：`id`, `template_name`, `content`, `variables JSONB`, `created_at`）。
3. 全流程联调：上传 → 解析 → 生成 → 合规检查 → 平台适配 → 发布。
4. 异常测试：大模型超时 → 重试 → 失败提示；沙箱 crash → 降级（若配置 `SANDBOX_FALLBACK=true`）。

**校验标准**：
- `docker compose up -d` 后，新功能全套可用，且原有功能不受影响（终端无新增错误日志）。

---

### 阶段 8：文档与验收

**执行动作**：
1. 生成《自媒体运营功能使用指南》《新增 Tool 开发规范》。
2. 按验收标准逐项自检，录制全流程演示视频。