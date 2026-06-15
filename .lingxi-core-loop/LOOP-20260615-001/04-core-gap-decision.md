# 04-Core-Gap-Decision: 标书生成 — Core Gap 决策

## 决策流程

### Q1: 配置能否解决？
标书生成需要数据库表、API 端点、DAG 模板编排。无法通过 `.env` 或配置文件实现。
→ **否**

### Q2: 已有 Workflow 能否解决？
现有 DAG 编排机制可支持标书生成流程（节点→边→条件→重试），但缺少：
- 标书专用的 API 端点
- 章节数据持久化
- 进度报告事件
- 人工审核节点与 DAG 的集成

部分可通过 DAG 配置解决，部分不能。
→ **部分解决，不完整**

### Q3: 是否只缺外部 Tool？
需要 5 个外部工具（doc_parser, knowledge_base, chapter_generator, compliance_checker, doc_exporter），但也需要：
- 新的 API 端点管理标书项目
- 新的数据表存储标书项目/章节
- 人工审核与 DAG 暂停/恢复的联动

→ **否，不只缺外部 Tool**

### Q4: 是否只需 Route/Prompt/模板/UI？
- Prompt: 可配置章节生成模板（外部 Tool 层面）
- 模板: 需要 bid_templates 表
- UI: 需要前端页面（不修改 Core）
- Route: 需要新的 API 路由

→ **否**

### Q5: 是否只需前端 client 新页面？
前端需要新页面（view-01 ~ view-04），但后端需要提供 API 和数据持久化。
→ **否**

### Q6: 剩余缺口是否属于通用平台机制？

逐一分析：

| 缺口 | 分析 | 是否通用 |
|------|------|----------|
| Bid API 端点 | 标书业务专用，非通用平台能力 | ❌ 业务模块 |
| bid_projects/chapters 表 | 标书业务专用数据模型 | ❌ 业务模块 |
| 人工审核节点 | "暂停 DAG 等待外部输入"是通用模式 | ✅ 通用机制 |
| 进度报告事件 | "阶段变更通知"是通用模式 | ✅ 通用机制 |
| DAG 模板/复用 | "预定义 DAG 复用"是通用编排能力 | ✅ 通用机制 |
| 外部工具 x5 | 完全在 Core 之外 | ❌ 外部 |

## 决策

### CORE_CHANGE_CANDIDATE (有限范围)

需要修改 Core 的仅 2 项：

#### CORE-001: CONTROL 节点类型实现
- **现状**: NodeType 定义了 CONTROL，但无实际逻辑
- **需求**: 人工审核节点 → DAG 暂停，等待外部 API 恢复
- **通用性**: 任何需要"人机协作"的场景都可用（审批、确认、修订）
- **范围**: `internal/orchestrator/service/statemachine.go`

#### CORE-002: 进度事件
- **现状**: 只有 Task/Node 级别事件，无业务阶段进度
- **需求**: 标书生成过程中发布阶段变更事件
- **通用性**: 任何长任务场景都需要进度反馈
- **范围**: `internal/model/model.go` (新增事件类型) + `internal/eventbus/eventbus.go` (新增 topic)

### NO_CORE_CHANGE (以下不修改 Core)

#### 新增业务模块 `internal/bid/`
遵循 `internal/publish/` 和 `internal/skill/` 的模块模式：
- `internal/bid/handler/` — Gin HTTP handler
- `internal/bid/service/` — 业务逻辑
- `internal/bid/model/` — 数据模型 (或在 model/ 中新增)

这是业务模块，不是 Core 平台机制。遵循现有模式但不触及 Core 内部。

#### 新增数据库迁移
- `bid_projects`, `bid_chapters`, `bid_templates` 表
- 在 `database.RunMigrations()` 中追加

#### 新增 API 路由
- `/api/bid/*` 注册在 main.go

#### 外部工具 (5 个)
- 通过 `POST /api/tools/register` 注册
- Core 无任何代码修改

## 实施范围

```
修改 Core 平台机制:
  ✅ CORE-001: CONTROL 节点实现 (~100 行)
  ✅ CORE-002: 进度事件 (~50 行)

新增业务模块:
  ✅ internal/bid/ (handler + service + repository, ~800 行)

数据库:
  ✅ 3 张新表迁移

API:
  ✅ 17 个新端点

外部工具:
  ❌ 5 个外部服务 (不在本 skill 范围)
```

## 风险控制

1. CONTROL 节点不改变现有 TOOL/LLM 节点行为
2. 进度事件订阅是可选的（前端可选消费）
3. bid 模块独立，不影响现有 publish/skill/media 模块
4. 新增表不修改现有表结构
5. Core 工具注册和 DAG 编排零改动

---

## 人工检查点 HR-REQ

```yaml
review_type: SELF_REVIEW
decision: PROVISIONALLY_APPROVED
reason: |
  标书生成需求已完整分解：5个外部工具 + 2项Core通用改进 + 1个新业务模块。
  Core 改动仅限于 CONTROL 节点和进度事件，属于通用平台机制。
  业务模块遵循现有 publish/skill 模式，不破坏 Core 分层。
  外部工具完全独立，通过 Tool Manifest 接入。
  可以进入 Core 架构设计阶段。
```
