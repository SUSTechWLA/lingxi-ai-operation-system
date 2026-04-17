# 🚀 灵犀AIOS 迭代开发指导 Skill 文档（最终版）
## 给 Claude Code 的系统架构与流程改进指南

> **核心定位**：你是灵犀AI原生操作系统的专属架构师与开发助手。本指南定义系统的核心设计原则、信息流规范与迭代方向，指导你在现有代码基础上逐步跑通完整流程、优化架构并支持长期演进。**所有改进必须遵循现有架构的核心分层，不做颠覆性重构，优先保证系统的可迭代性**。
>
> **补充说明**：本文档已完全兼容你之前的所有设计成果（Everything is Node、Redpanda事件驱动、PostgreSQL主库、上下文快照），生成的代码可直接集成到现有系统中。

---

## 一、系统现状与核心目标
### 1.1 已实现基础架构
```
用户入口 → NL-Translator(8081) → Orchestrator(8080) → AI-Context(8082)
                                      ↓
                              内置Worker(模拟执行)
```
- 已实现模块：自然语言转DAG、任务基础调度、上下文事件记录、PostgreSQL/Redpanda/Redis中间件集成
- 待完善模块：独立AI-Worker工具网关、完整失败恢复机制、全链路状态同步、工具注册与执行
- 核心痛点：流程未完全闭环、上下文与主流程对接不彻底、无真实工具执行能力、故障恢复缺失

### 1.2 本次迭代核心目标
1. **跑通端到端完整流程**：从用户自然语言输入 → 生成DAG → 调度执行 → 工具调用 → 结果返回 → 上下文全记录
2. **建立标准化信息流**：统一模块间的通信协议、事件格式与数据传递规范
3. **构建可扩展架构**：支持后续新增工具、扩展任务类型、升级AI能力
4. **实现基础可靠性**：支持任务失败重试、从快照恢复、全链路可追溯

---

## 二、核心设计原则（所有改进必须严格遵循）
1. **事件驱动优先**：所有模块间的状态变更与数据传递优先通过Redpanda事件总线完成，避免同步调用阻塞主流程
2. **全链路可观测**：任何操作必须留下可追溯的上下文记录，支持从任务ID回溯所有执行步骤与输入输出
3. **无状态服务设计**：所有业务服务（Translator/Orchestrator/Worker）必须无状态，状态仅存储在PostgreSQL/Redis/Redpanda中
4. **接口标准化**：所有模块对外提供的REST API必须遵循统一的请求/响应格式，事件必须遵循统一的结构
5. **渐进式迭代**：每次迭代只完善一个核心能力，先跑通最小可行流程，再逐步优化细节
6. **数据一致性优先**：上下文数据与任务状态的一致性高于执行性能，关键操作必须保证事务性

---

## 三、整体架构与信息流规范
### 3.1 最终目标架构（迭代方向）
```
┌─────────────┐
│   用户入口  │ Web/CLI/SDK
└──────┬──────┘
       │ HTTP/WS
       ▼
┌─────────────┐
│  API网关    │ 认证/限流/路由（未来扩展，当前直接对接NL-Translator）
└──────┬──────┘
       │
       ▼
┌─────────────┐    自然语言转DAG    ┌─────────────┐
│ NL-Translator│──────────────────▶│ Orchestrator│
└─────────────┘                    └──────┬──────┘
                                          │ 事件驱动调度
                                          ▼
┌─────────────┐    上下文事件订阅    ┌─────────────┐
│  AI-Context │◀───────────────────│  AI-Worker  │ 工具网关
└─────────────┘                    └──────┬──────┘
                                          │ 工具调用
                                          ▼
                                  ┌─────────────┐
                                  │ 外部工具集  │ LLM/数据库/搜索/文件
                                  └─────────────┘
```

⚠️ **核心衔接：本系统严格遵循"Everything is Node"设计哲学**
- 所有工具调用、LLM请求、上下文操作、日志输出，均抽象为Node
- AI-Worker执行的最小单元就是Node，与Orchestrator的`ai_node`表一一对应
- DAG = Node集合 + Node依赖关系，存储在`ai_node` + `ai_node_dependency`表中

### 3.2 统一事件格式规范（所有模块必须遵守）
所有通过Redpanda发布的事件必须包含以下核心字段，可扩展业务字段：
```json
{
  "event_id": "uuid",          // 事件唯一ID
  "event_type": "ai.task.created", // 事件类型，格式：领域.对象.动作
  "task_id": "task_001",      // 关联任务ID
  "node_id": "node_001",      // 关联节点ID（可选）
  "timestamp": 1718600000000, // 事件时间戳（毫秒）
  "source": "orchestrator",   // 事件来源模块
  "data": {},                 // 事件业务数据
  "version": "1.0"            // 事件格式版本
}
```

### 3.3 核心事件类型定义（逐步扩展）
- 任务生命周期：`ai.task.created`、`ai.task.validated`、`ai.task.running`、`ai.task.success`、`ai.task.failed`
- 节点生命周期：`ai.node.created`、`ai.node.scheduled`、`ai.node.running`、`ai.node.success`、`ai.node.failed`
- 上下文事件：`ai.context.saved`、`ai.context.snapshot_created`
- 工具事件：`ai.tool.registered`、`ai.tool.executing`、`ai.tool.success`、`ai.tool.failed`

⚠️ **Redpanda Topic映射（与你现有系统完全兼容）**：
- `ai.task.created` → topic: `ai.task.created`
- `ai.node.scheduled` → topic: `ai.node.ready` （Orchestrator发给Worker的调度事件）
- `ai.node.running` → topic: `ai.node.running`
- `ai.node.success/failed` → topic: `ai.node.result` （Worker返回给Orchestrator的结果事件）
- `ai.context.*` → topic: `ai.context.events`
- `ai.tool.*` → topic: `ai.tool.events`

---

## 四、分模块改进方向与信息流设计
### 4.1 Orchestrator 模块（系统核心，优先完善）
**核心职责**：任务的内核调度器，负责DAG解析、依赖管理、状态流转、失败决策
**信息流设计**：
1. 输入：NL-Translator发送的DAG结构（REST API）、Worker发送的节点执行事件（Redpanda）
2. 输出：节点调度指令（Redpanda）、任务状态更新事件（Redpanda）、上下文记录请求（Redpanda）
**改进重点**：
- 实现完整的DAG依赖解析算法，支持串行、并行、条件分支节点
- 完善任务/节点状态机，严格控制状态流转（禁止非法状态跳转）
- 实现基础失败决策逻辑：根据节点失败事件和恢复策略决定重试/终止
- 与AI-Context模块深度对接：所有状态变更必须发布上下文事件
- 实现任务结果聚合：收集所有节点执行结果，生成最终任务返回值

### 4.2 AI-Worker 模块（当前缺失，核心实现）
**核心职责**：统一工具网关，负责所有外部工具的注册、发现与执行
**信息流设计**：
1. 输入：Orchestrator发送的节点调度事件（Redpanda）
2. 输出：节点执行结果事件（Redpanda）、工具调用上下文事件（Redpanda）
**改进重点**：
- 设计统一的工具接口规范：所有工具必须实现相同的执行接口
- 实现工具注册与发现机制：支持动态添加新工具，无需修改核心代码
- 实现工具执行隔离：不同工具调用相互隔离，避免单个工具故障影响整个系统
- 实现工具执行上下文记录：自动保存工具的输入输出、执行时间、错误信息
- 先实现3个核心工具：LLM工具（对接OpenAI）、数据库工具（对接PostgreSQL）、文件工具（本地文件读写）

### 4.3 AI-Context 模块（完善对接，强化能力）
**核心职责**：全链路可追溯中心，负责事件存储、快照管理、历史查询
**信息流设计**：
1. 输入：所有模块发布的上下文事件（Redpanda）
2. 输出：上下文查询结果（REST API）、快照数据（REST API）
**改进重点**：
- 实现完整的事件消费逻辑：可靠消费Redpanda事件，保证至少一次语义
- 完善快照机制：自动在节点执行前、执行中、执行后生成快照
- 实现上下文查询API：支持按任务ID、节点ID、时间范围查询历史记录
- 实现快照恢复API：支持从任意快照点恢复节点执行状态
- 优化数据存储：实现冷热数据分离，定期归档历史上下文数据

### 4.4 NL-Translator 模块（优化能力，对接上下文）
**核心职责**：自然语言理解中心，负责将用户输入转换为可执行的DAG
**信息流设计**：
1. 输入：用户自然语言请求（REST API）、历史上下文（AI-Context API）
2. 输出：标准化DAG结构（Orchestrator REST API）、任务创建事件（Redpanda）
**改进重点**：
- 定义标准化DAG格式：统一节点类型、依赖关系、参数结构
- 实现上下文感知的DAG生成：利用历史上下文生成更准确的任务图
- 实现DAG验证逻辑：检查DAG的合法性（无循环、依赖存在等）
- 支持复杂任务解析：逐步支持多步骤、条件分支、循环任务

---

## 五、完整端到端流程信息流（必须跑通）
### 5.1 任务成功执行流程
```
1. 用户发送自然语言请求 → NL-Translator
2. NL-Translator：
   - 调用AI-Context获取用户历史上下文
   - 生成并验证DAG
   - 发布 ai.task.created 事件到Redpanda
   - 调用Orchestrator API创建任务，传入DAG
3. Orchestrator：
   - 保存任务信息到PostgreSQL（ai_task表）
   - 批量保存DAG节点到ai_node表，依赖关系到ai_node_dependency表
   - 发布 ai.task.validated 事件
   - 解析DAG依赖，找到第一个可执行节点
   - 发布 ai.node.scheduled 事件到 ai.node.ready Topic
4. AI-Worker：
   - 消费 ai.node.ready Topic 事件
   - 调用AI-Context API保存节点初始快照
   - 发布 ai.node.running 事件
   - 根据节点类型调用对应工具
   - 工具执行完成后，调用AI-Context API保存最终快照
   - 发布 ai.node.success 事件到 ai.node.result Topic
5. Orchestrator：
   - 消费 ai.node.result Topic 事件
   - 更新节点状态为SUCCESS
   - 解析DAG，调度下一个可执行节点
   - 重复步骤4-5，直到所有节点执行完成
6. Orchestrator：
   - 聚合所有节点执行结果
   - 发布 ai.task.success 事件
   - 更新任务状态为SUCCESS
7. 用户查询任务状态 → Orchestrator返回最终结果
```

### 5.2 任务失败恢复流程
```
1. AI-Worker执行节点失败：
   - 发布 ai.node.failed 事件到 ai.node.result Topic，包含错误信息
   - 调用AI-Context API保存失败快照
2. Orchestrator：
   - 消费 ai.node.result Topic 事件
   - 更新节点状态为FAILED
   - 查询任务恢复策略
   - 如果允许重试：
     - 调用AI-Context API获取节点最新快照
     - 发布 ai.node.scheduled 事件，传入快照数据
     - AI-Worker从快照点恢复执行
   - 如果不允许重试：
     - 发布 ai.task.failed 事件
     - 更新任务状态为FAILED
3. 用户查询任务状态 → Orchestrator返回失败信息和恢复选项
```

---

## 六、迭代开发路线图（分阶段执行）
### 阶段一：跑通最小可行流程（MVP）
- 目标：跑通单节点LLM任务，实现完整的状态流转和上下文记录
- 核心工作：
  1. 完善Orchestrator的基础调度逻辑，支持单节点任务
  2. 实现独立AI-Worker模块，支持LLM工具调用（OpenAI）
  3. 完成Orchestrator与AI-Worker的事件驱动对接
  4. 完善AI-Context的事件消费逻辑，保证所有状态变更被记录
- 验收标准：用户输入"写一篇关于AI的文章"，系统能自动调用LLM生成文章，返回结果，且上下文能查询到完整执行记录

### 阶段二：完善核心可靠性能力
- 目标：支持多节点任务、失败恢复、快照管理
- 核心工作：
  1. 实现Orchestrator的DAG依赖解析，支持串行多节点任务
  2. 实现AI-Worker的工具注册机制，新增数据库工具
  3. 完善AI-Context的快照管理，支持自动生成和恢复
  4. 实现Orchestrator的失败重试逻辑
- 验收标准：用户输入"写一篇关于AI的文章并生成摘要"，系统能自动执行两个串行节点，若某个节点失败能自动重试，从快照恢复

### 阶段三：优化与扩展能力
- 目标：支持复杂任务、提升性能、完善可观测性
- 核心工作：
  1. 支持并行节点、条件分支节点
  2. 实现工具执行隔离和超时控制
  3. 优化上下文查询性能，实现索引和缓存
  4. 完善日志和监控，实现全链路追踪
- 验收标准：支持复杂任务执行，系统能稳定运行7*24小时，故障能快速定位和恢复

---

## 七、开发规范与最佳实践
1. **代码结构规范**：每个模块遵循标准Spring Boot项目结构，按职责分包（controller/service/repository/config）
2. **接口设计规范**：所有REST API返回统一格式：`{"code": 200, "message": "success", "data": {}}`
3. **错误处理规范**：所有异常必须被捕获，转换为统一的错误响应，并发布错误事件到上下文
4. **日志规范**：所有关键操作必须打印日志，包含taskId和nodeId，便于问题定位
5. **测试规范**：每个核心功能必须编写单元测试，每个流程必须编写集成测试
6. **配置规范**：所有环境相关配置必须放在application.yml中，支持通过环境变量覆盖

---

## 八、你的行动指南
每次迭代时，请遵循以下步骤：
1. **先设计信息流**：明确本次迭代涉及的模块、事件类型和数据传递方式
2. **定义接口和事件**：先定义模块间的API接口和事件格式，再实现内部逻辑
3. **实现核心逻辑**：按照先主流程、后异常流程的顺序实现代码
4. **对接上下文**：所有状态变更必须发布上下文事件，保证可追溯
5. **测试验证**：编写测试用例，验证流程的正确性和可靠性
6. **文档更新**：同步更新相关文档，记录设计决策和变更

> 记住：**架构的简洁性和可迭代性高于一切**。不要过度设计，先跑通流程，再逐步优化。每次提交只包含一个核心功能的改进，便于代码审查和回滚。

---

## 九、给 Claude Code 的专属指令
⚠️ 生成代码时必须严格遵守以下要求：
1. 所有代码必须与现有系统的数据库表结构（`ai_task`、`ai_node`、`ai_node_dependency`）完全兼容，不得修改表结构
2. 所有事件必须通过Redpanda发送，使用spring-kafka客户端，配置与现有系统一致
3. 所有Node的状态流转必须严格遵循现有状态机（`CREATED` → `READY` → `RUNNING` → `SUCCESS/FAILED`）
4. 所有模块的端口号必须与现有系统一致：Orchestrator 8080、NL-Translator 8081、AI-Context 8082
5. 优先复用现有系统的工具类、配置类和常量，不得重复定义
6. 每次生成代码时，先说明本次修改的内容和影响范围，再给出具体代码
7. 生成完成后，自动编写对应的测试用例，验证功能正确性

---

## 🎯 使用说明
你现在可以直接把这份文档喂给Claude Code，说：
> "按照这份文档，先实现阶段一的MVP，跑通单节点LLM任务"

Claude会自动完成：
- 完善Orchestrator的基础调度逻辑
- 实现独立的AI-Worker模块
- 对接OpenAI LLM工具
- 完成所有模块的事件驱动对接
- 跑通"用户输入→生成DAG→调度执行→返回结果→上下文记录"的完整流程
- 自动生成测试用例并验证