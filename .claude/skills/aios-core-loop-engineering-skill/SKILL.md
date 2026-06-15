---
name: aios-core-loop-engineering
version: 5.0.0
description: >
  Use when working on AIOS Core backend engineering or turning customer requirements into an AIOS
  solution package. Covers customer requirement analysis, solution decomposition, frontend requirement
  definition, external tool contract definition, Core gap decisions, Go Core updates, tests, evidence,
  release, operations, and iterative Core improvement. Does not implement customer frontend code or
  external tool internals.
triggers:
  - 更新 AI OS Core
  - AIOS Core 新功能开发
  - AIOS Core Bug 修复
  - AIOS Core 架构重构
  - AIOS 性能、可靠性、日志或编排能力优化
  - 判断客户需求是否需要修改 AIOS Core
  - 将客户需求转成 AIOS solution
  - 定义客户前端和外部工具要求
  - 自动化标书需求的 AIOS Core 支撑分析
  - AIOS Core Loop Engineering
---

# Lingxi AI OS Core Loop Engineering

## 1. 定位

本 Skill 是 AIOS Core 后端服务的软件工程和 solution 转化闭环。当前默认 Core 只包括：

```text
自然语言理解/入口层
编排层
工具层
日志管理层
依赖的中间件与基础设施适配
```

实际执行时必须先读取真实仓库，不得假设目录、模块或能力已经存在。当前仓库的默认边界是：

```text
AIOS Core 后端：Go 模块、Core API、编排、工具治理、会话、日志和中间件适配
客户前端：独立 client，通过 HTTP API / SDK / 事件视图对接 Core
外部工具：独立服务或进程，通过 Tool Manifest、Schema 和黑盒验收接入 Core
```

本 Skill 支持三类任务：

### A. 将客户需求转成完整 AIOS Solution

```text
客户原始需求
→ 业务目标和角色流程
→ Solution 蓝图
→ Frontend Requirements
→ Workflow / DAG / Session / Artifact 规格
→ External Tool Capability Requirements
→ Core API / Tool Manifest / Event / Trace 集成约束
→ Core Gap 决策
→ NO_CORE_CHANGE 或 CORE_CHANGE
```

Solution 是给当前 agent 和后续开发团队使用的完整交付方案，不是只判断是否改 Core。

### B. 直接更新 AIOS Core

```text
Core 新需求 / Bug / 重构 / 性能或可靠性问题
→ 发现当前实现
→ 明确验收条件
→ 架构与影响分析
→ Go Core 开发
→ 自动测试与修复
→ 人工检查点
→ 发布、运维和复盘
```

### C. 从已有客户 solution 判断是否需要更新 Core

```text
已有 Solution / Workflow / 外部 Tool 约束
→ 判断缺口属于配置、Workflow、外部 Tool 还是 Core
→ NO_CORE_CHANGE 或 CORE_CHANGE
→ 只有 CORE_CHANGE 才进入 Core 代码闭环
```

本 Skill 的目标：

1. 保持 AIOS 作为通用 Agent 平台；
2. 将客户需求稳定转成可开发、可验收、可集成的 solution；
3. 清楚定义客户前端、Workflow、外部工具和 Core 的边界；
4. 只吸收真正的平台通用能力；
5. 每次 Core 更新都有需求、设计、测试、证据、审核、发布和回滚；
6. 不因单一客户场景污染 Core；
7. 可以长期用于后续 AIOS 更新。

---

## 2. 强制边界

### 2.1 本 Skill 直接管理

- AIOS Core Go 代码；
- 入口层、编排层、工具层、日志层和中间件适配；
- 从客户需求到 AIOS solution 的结构化方案；
- 前端团队所需的页面、交互、状态、API 和验收要求；
- 外部工具团队所需的能力契约、输入输出、错误和黑盒验收要求；
- Core API、事件、状态机、配置和数据库迁移；
- Core 单元、组件、集成、回归、性能和可靠性测试；
- Core 架构文档、ADR、发布、回滚和运维；
- Core 与外部能力之间的接口契约和黑盒验证；
- 人工检查点材料、结论和流程状态。

### 2.2 本 Skill 不直接管理

- Python/FastAPI 工具源码；
- 外部 Go、Java、Node.js 工具源码；
- 工具内部算法、模型、库和目录结构；
- 工具开发者的编码过程；
- 标书、论文、合同等业务服务内部实现；
- 客户专属前端实现；
- React、Vue、Electron、移动端等 client 具体代码；
- 客户专属 UI 视觉稿和组件实现；
- 登录、IAM、在线审批中心等当前非核心模块。

### 2.3 对外部工具只定义

```text
需要什么能力
输入和输出是什么
必须达到什么指标
错误如何表达
Core 如何调用
如何进行黑盒验收
```

不得生成外部工具源码、算法方案、模型选型或 FastAPI 工程结构。

### 2.4 对客户前端只定义

```text
用户角色和任务
页面/视图/状态
需要调用的 Core API
请求/响应和错误展示
进度、Trace、Artifact 和人工检查点呈现方式
验收标准
```

不得生成客户专属前端源码、组件实现、视觉系统或 Electron 打包工程。

### 2.5 强制禁止

1. 不得把标书、论文等业务规则写入 Core。
2. 不得因为外部 Tool 未交付而在 Core 中临时实现业务逻辑。
3. 不得为了人工审核强制新增登录、IAM 或审批模块。
4. 不得把流程人工检查点描述为 AIOS 已有的在线审批能力。
5. 未完成 Core Gap 判断前不得修改代码。
6. 不得关闭测试、吞掉错误或伪造成功结果。
7. Agent 的文字结论不得代替命令、退出码和原始证据。
8. 不得把“客户需要页面”解释为必须修改 Core。
9. 不得把“缺少某业务工具”解释为必须修改 Core。

---

## 3. 调用方式

### 3.1 直接更新 Core

```text
/lingxi-aios-core-loop --mode core-update
需求：为编排层增加任务暂停和恢复能力。
```

### 3.2 客户需求的 Core 支撑分析

```text
/lingxi-aios-core-loop --mode solution-assessment
需求：开发自动化标书生成能力。
```

### 3.3 客户需求转 Solution

```text
/lingxi-aios-core-loop --mode solution-design
需求：客户希望用 AIOS 搭建合同审查工作台，支持上传合同、抽取风险、人工确认、导出报告。
```

### 3.4 其他模式

```text
/lingxi-aios-core-loop --mode bugfix --issue BUG-001
/lingxi-aios-core-loop --mode refactor
/lingxi-aios-core-loop --mode performance
/lingxi-aios-core-loop --mode reliability
/lingxi-aios-core-loop --mode observability
/lingxi-aios-core-loop --resume LOOP-20260615-001
/lingxi-aios-core-loop --dry-run
```

| 模式 | 说明 |
|---|---|
| `core-update` | 新增 Core 通用能力 |
| `bugfix` | 修复 Core 缺陷 |
| `refactor` | 保持外部行为的内部重构 |
| `performance` | 优化延迟、吞吐和资源使用 |
| `reliability` | 优化重试、恢复、幂等和容错 |
| `observability` | 优化日志、指标、Trace 和审计 |
| `solution-design` | 将客户需求转成完整 AIOS solution，并判断是否需要修改 Core |
| `solution-assessment` | 从客户需求判断 Core 缺口 |
| `release-review` | 对已有版本做发布检查 |
| `operations-review` | 根据生产问题判断是否需要新 Core Loop |

---

## 4. 逻辑角色

角色是逻辑职责，不要求部署成独立 Agent。

| 角色 | 职责 |
|---|---|
| Requirement Analyst | 将输入转换为可验收的 Core 需求 |
| Solution Architect | 将客户需求拆成 Core、Workflow、Frontend 和 External Tool 边界 |
| Frontend Contract Analyst | 定义前端页面、状态、API、错误和验收要求，不写前端代码 |
| Core Explorer | 读取当前仓库、架构、配置、测试和运行证据 |
| Core Gap Analyst | 判断问题是否真正属于 Core |
| Core Architect | 设计接口、状态机、依赖、兼容和回滚 |
| Core Developer | 只修改 AIOS Core Go 代码 |
| Core Verification Agent | 执行测试、分析失败并形成 Evidence |
| Integration Analyst | 验证 Core 与外部能力的黑盒契约 |
| Release & Operations Agent | 发布、灰度、监控、事件和复盘 |
| Manual Review Coordinator | 生成检查材料、接收结论并控制继续或返工 |

本 Skill 不包含 Tool Developer Agent。

---

## 5. 工作区

```text
.lingxi-core-loop/
└── LOOP-<date>-<sequence>/
    ├── loop.yaml
    ├── 00-intake.md
    ├── 01-core-discovery.md
    ├── 02-requirement-spec.md
    ├── 03-acceptance-and-nfr.md
    ├── 04-core-gap-decision.md
    ├── 04a-solution-blueprint.md
    ├── 04b-frontend-requirements.md
    ├── 04c-workflow-spec.md
    ├── 04d-api-and-event-contracts.md
    ├── 05-core-impact-analysis.md
    ├── 06-core-architecture-design.md
    ├── 07-core-execution-plan.md
    ├── 08-core-test-plan.md
    ├── 09-integration-contracts.md
    ├── 10-release-plan.md
    ├── 11-operations-plan.md
    ├── 12-delivery-report.md
    ├── 13-retrospective.md
    ├── external-capability-requirements.md
    ├── adr/
    ├── reviews/
    ├── bugs/
    └── evidence/
```

`external-capability-requirements.md` 只在需要外部 Tool 时生成，且不包含实现方案。

`04a` 到 `04d` 在 `solution-design` 和 `solution-assessment` 中生成；若任务是直接 Core bugfix，可省略。

事实来源优先级：

```text
真实代码和运行证据
> 当前版本 API、事件和配置
> 当前架构文档
> 历史 Loop 与 ADR
> Agent 会话记忆
```

---

## 6. 状态机

```text
CREATED
→ DISCOVERING
→ REQUIREMENT_DRAFTING
→ SOLUTION_DESIGNING
→ REQUIREMENT_REVIEW_PENDING
→ CORE_GAP_ANALYZING
```

Core Gap 结果：

```text
NO_CORE_CHANGE
CORE_CHANGE_REJECTED
CORE_CHANGE_CANDIDATE
NEED_MORE_EVIDENCE
```

需要 Core 更新时：

```text
CORE_CHANGE_CANDIDATE
→ CORE_DESIGNING
→ CORE_REVIEW_PENDING
→ CORE_APPROVED
→ IMPLEMENTING
→ VERIFYING
```

验证结果：

```text
FIX_REQUIRED → IMPLEMENTING
REQUIREMENT_CONFLICT
ARCHITECTURE_CONFLICT
ENVIRONMENT_FAILURE
VERIFIED
```

发布运维：

```text
VERIFIED
→ RELEASE_REVIEW_PENDING
→ READY_FOR_RELEASE
→ RELEASED
→ OPERATING
→ COMPLETED
```

人工检查点采用外部流程，不要求 AIOS 内置审批模块。

---

## 7. 阶段 0：任务接收

### 输入

- 客户需求、Core 新需求、Bug、重构目标或生产问题；
- 当前仓库位置；
- 已知限制；
- 风险等级和目标环境。

### 动作

1. 判断运行模式；
2. 创建 Loop ID；
3. 设置 `core_write_enabled=false`；
4. 识别这是 customer solution、Core update、bugfix 还是运维复盘；
5. 识别初步影响层；
6. 记录假设和未决问题。

### 输出

- `loop.yaml`
- `00-intake.md`

此阶段禁止修改代码。

---

## 8. 阶段 1：发现真实 AIOS Core

必须检查：

1. 入口层：意图解析、请求模型和调用入口；
2. 编排层：DAG、节点、状态机、重试和恢复；
3. 工具层：注册、发现、调用、超时、错误和版本；
4. 日志层：结构化日志、Trace、指标和审计；
5. 中间件：数据库、消息、缓存和对象存储适配；
6. 配置、API、事件和数据模型；
7. 测试、构建、启动和发布方式；
8. 历史 ADR、Bug 和生产事件。

输出 `01-core-discovery.md`：

- 当前模块和依赖；
- 核心接口和状态机；
- 与本需求相关的代码；
- 已有能力和限制；
- 测试与质量门禁；
- 风险和不一致。

不得假设存在 IAM、登录、审批、用户中心或其他未发现模块。

---

## 9. 阶段 2：需求与验收标准

使用编号：

```text
REQ-xxx 功能需求
AC-xxx 验收标准
NFR-xxx 非功能要求
RISK-xxx 风险
```

Core 需求必须明确：

- 哪一层产生什么行为变化；
- 什么保持不变；
- 外部接口是否变化；
- 失败和恢复行为；
- 持久化、并发和幂等；
- 日志、指标和 Trace；
- 兼容、迁移和回滚。

输出：

- `02-requirement-spec.md`
- `03-acceptance-and-nfr.md`

---

## 9A. 阶段 2A：Solution 设计

当输入来自客户需求时必须生成完整 solution。不要跳过到 Core Gap。

### 必须拆分

- 客户业务目标、用户角色和主流程；
- AIOS Core 需要承担的通用职责；
- Workflow / DAG / Session / Artifact / Trace 规格；
- 客户前端需要呈现的页面、状态、动作和错误；
- 外部 Tool 能力清单；
- Core API、Tool Manifest、事件和日志契约；
- 数据、权限、安全、审计和人工检查点边界；
- 交付里程碑、验收标准和风险。

### 输出

- `04a-solution-blueprint.md`
- `04b-frontend-requirements.md`
- `04c-workflow-spec.md`
- `04d-api-and-event-contracts.md`
- `external-capability-requirements.md`（如需要外部 Tool）

### Solution 判定原则

1. 客户专属差异优先放在前端、Workflow 配置、Prompt、模板或外部 Tool。
2. Core 只提供通用入口、编排、工具治理、Artifact、Trace、状态和中间件能力。
3. 前端要求必须足够具体，让前端开发可以独立实施。
4. 外部 Tool 要求必须足够具体，让工具开发可以独立实施和黑盒验收。
5. 若当前 Core API 不足，只记录为 Core Gap 候选，不在 solution 阶段直接改代码。

---

## 10. 人工检查点 HR-REQ

人工检查点是 Skill 的流程节点，不是 AIOS 的在线审批功能。

生成：

```text
reviews/HR-REQ/review-request.md
reviews/HR-REQ/checklist.md
reviews/HR-REQ/decision.yaml
```

用户可通过会议、线下或外部协作软件讨论，然后提交结论：

```text
APPROVED
APPROVED_WITH_ACTIONS
REVISE_AND_RESUBMIT
REJECTED
DEFERRED
```

只有一个人时允许：

```yaml
review_type: SELF_REVIEW
decision: PROVISIONALLY_APPROVED
```

这允许继续内部开发，但不等于多人正式批准。

---

## 11. 阶段 3：Core Gap 决策

这是最重要的门禁。

### 决策顺序

```text
配置能否解决？
→ 是：NO_CORE_CHANGE

已有 Workflow 能否解决？
→ 是：NO_CORE_CHANGE

是否只缺外部 Tool？
→ 是：NO_CORE_CHANGE，并输出外部能力要求

是否只需 Route、Prompt、模板或 UI？
→ 是：NO_CORE_CHANGE

是否只需前端 client 新页面或状态编排？
→ 是：NO_CORE_CHANGE，并输出 frontend-requirements

剩余缺口是否属于通用平台机制？
→ 否：CORE_CHANGE_REJECTED
→ 是：CORE_CHANGE_CANDIDATE
```

### Core 准入条件

必须同时满足：

1. 属于入口、编排、工具治理、日志或中间件通用职责；
2. 不包含具体业务规则；
3. 可被多个场景复用，或属于平台完整性要求；
4. 无法通过外部 Tool、Workflow 或配置合理实现；
5. 有稳定边界；
6. 可自动测试；
7. 有兼容、迁移和回滚方案。

输出 `04-core-gap-decision.md`。

只有结论为 `CORE_CHANGE_CANDIDATE` 并通过人工检查，才设置：

```yaml
core_write_enabled: true
```

若为 `NO_CORE_CHANGE`，Skill 直接生成评估交付报告，不进入代码开发。

---

## 12. 外部能力需求

若客户需求需要外部 Tool，只输出 `external-capability-requirements.md`。

每项能力只包括：

- capability_id；
- 职责和非职责；
- 输入与输出；
- 质量指标；
- 性能与可靠性；
- 安全和数据要求；
- 错误结构；
- Core 调用约束；
- 黑盒验收场景。

不得生成工具代码、内部设计、模型选型和开发任务细节。

---

## 12A. 前端需求输出

若客户需求需要前端开发，只输出 `04b-frontend-requirements.md`，不得写前端代码。

每个前端需求必须包括：

- view_id；
- 用户角色和目标；
- 页面入口和退出；
- 展示数据和状态；
- 用户动作；
- 调用的 Core API、参数和响应；
- 进度、错误、Trace、Artifact 和人工检查点展示；
- 空状态、加载态、失败态和重试；
- 验收标准。

如果前端需要新增 API 才能完成体验，必须把它列入 `04d-api-and-event-contracts.md` 并进入 Core Gap 判断。

---

## 13. 阶段 4：Core 影响分析

分析：

- 受影响层和包；
- API、事件和状态机变化；
- 数据模型与迁移；
- 并发、事务和幂等；
- 中间件与配置；
- 日志、指标和 Trace；
- 向后兼容；
- 性能、安全和资源；
- 发布和回滚风险。

输出 `05-core-impact-analysis.md`。

---

## 14. 阶段 5：Core 架构设计

设计原则：

1. 保持入口、编排、工具和日志职责清楚；
2. 编排控制逻辑不放入 Tool；
3. Tool 业务逻辑不进入 Core；
4. 入口层不承载复杂业务流程；
5. 日志层不作为业务状态真相；
6. 长任务状态需要持久化；
7. API 和事件需要版本化；
8. 失败恢复路径必须明确；
9. 优先使用扩展点和最小变更；
10. 不因当前单人开发而省略兼容和回滚。

输出：

- `06-core-architecture-design.md`
- `adr/ADR-xxx.md`

---

## 15. 人工检查点 HR-ARCH

检查：

- 是否真的应该修改 Core；
- 是否破坏分层；
- 是否有更小方案；
- 是否向后兼容；
- 是否可测试、迁移和回滚；
- 是否误把业务逻辑放入 Core。

单人开发可以自审后继续，但必须记录独立审核缺失风险。

---

## 16. 阶段 6：执行计划

拆成小型 `TASK-CORE-xxx`，每个任务声明：

- 目标；
- 允许修改范围；
- 禁止修改范围；
- 依赖；
- 代码变化；
- 测试；
- 回滚；
- Definition of Done。

输出 `07-core-execution-plan.md`。

---

## 17. 阶段 7：测试先行设计

输出 `08-core-test-plan.md`。

至少包含：

- 正常、异常和边界；
- 状态转换和幂等；
- 单元、组件和集成；
- 包依赖和跨层调用；
- 旧 API、事件、Workflow 和配置兼容；
- 超时、重试、崩溃和重启恢复；
- 并发、重复消息和中间件不可用；
- 日志、指标、Trace 和敏感信息；
- 性能基线和允许退化阈值。

Go 默认检查：

```bash
gofmt -w .
go vet ./...
go test ./...
go test -race ./...
```

仓库存在 Makefile、Taskfile 或 CI 命令时，优先执行正式命令。

---

## 18. 人工检查点 HR-TEST

检查：

- 每个 AC 是否有测试；
- 是否覆盖失败、恢复和回滚；
- 是否有并发和 Race；
- 是否包含旧行为回归；
- 是否定义性能和可观测性验证。

---

## 19. 阶段 8：Core 开发 Loop

```text
读取冻结规格
→ 修改最小代码范围
→ gofmt
→ 局部测试
→ 相关回归
→ go vet
→ race test
→ 收集 Evidence
→ 提交验证
```

禁止：

- 修改需求以适配代码；
- 删除失败测试；
- 吞掉错误；
- 扩大任务范围；
- 修改外部 Tool 源码；
- 将客户业务分支写入 Core。

---

## 20. 阶段 9：验证、Bug 与修复 Loop

失败分类：

```text
CODE_DEFECT
TEST_DEFECT
REQUIREMENT_CONFLICT
ARCHITECTURE_CONFLICT
ENVIRONMENT_FAILURE
DEPENDENCY_FAILURE
PERFORMANCE_REGRESSION
SECURITY_RISK
UNKNOWN
```

生成 `bugs/BUG-xxx.md`。

修复后执行：

1. 当前失败测试；
2. 所属模块测试；
3. 受影响层回归；
4. 必要的全量测试；
5. Evidence 更新。

默认：

```yaml
max_fix_iterations: 5
max_same_failure: 2
```

达到限制后进入人工分析，禁止无限循环。

---

## 21. Evidence Gate

Agent 自述不是证据。每次验证至少记录：

- Commit SHA 和变更文件；
- 执行命令和退出码；
- 单元、组件、集成和回归结果；
- gofmt、go vet 和 Race；
- 性能和兼容对比；
- 日志、指标和 Trace 样例；
- 已知风险和未完成项。

最小门禁：

```yaml
build: passed
gofmt: passed
go_vet: passed
unit_tests: passed
race_test: passed
acceptance_tests: passed
critical_regressions: 0
high_security_findings: 0
```

具体阈值以仓库现有标准为准。

---

## 22. 外部集成黑盒检查

Core 调用外部能力时，只检查：

- 注册与发现；
- 版本锁定；
- 输入输出 Schema；
- 超时、重试和幂等；
- 错误映射；
- 健康状态；
- Trace ID 和日志；
- 性能与可靠性；
- 外部失败时 Core 的降级行为。

输出 `09-integration-contracts.md` 和 Evidence。

不检查外部工具内部代码。

---

## 23. 人工检查点 HR-QUALITY

检查：

- 需求追踪；
- 代码 Diff；
- 测试 Evidence；
- 失败和修复记录；
- 性能与兼容变化；
- 已知风险。

结论：

```text
APPROVED
APPROVED_WITH_ACTIONS
REVISE_AND_RESUBMIT
REJECTED
```

---

## 24. 阶段 10：发布

输出 `10-release-plan.md`，包括：

- Core 版本和范围；
- 配置和数据迁移；
- 依赖版本；
- 部署顺序；
- 健康检查；
- 灰度范围；
- 监控指标；
- 自动暂停条件；
- 回滚步骤和验证。

本地原型或开发环境必须标记：

```text
release_scope: DEVELOPMENT_ONLY
```

---

## 25. 人工检查点 HR-RELEASE

当前 AIOS 没有登录和审批系统时：

```text
Skill 生成发布检查包
→ 用户线下或会议审核
→ 用户提交 decision.yaml
→ Skill 继续发布步骤
```

正式生产发布建议由独立人员确认；单人可完成开发环境发布。

---

## 26. 阶段 11：运维

输出 `11-operations-plan.md`，至少定义：

- 服务健康和请求成功率；
- 编排成功率和节点失败率；
- Tool 调用错误率；
- P95/P99 延迟；
- 重试和恢复次数；
- Goroutine、内存和 CPU；
- 中间件错误；
- 日志错误聚合；
- 版本、配置、告警和 Runbook。

生产问题先归因：

```text
CORE
EXTERNAL_TOOL
WORKFLOW_CONFIG
ROUTE_OR_ENTRY
MIDDLEWARE
DATA
ENVIRONMENT
UNKNOWN
```

只有 `CORE` 问题创建新的 Core Loop。

---

## 27. 阶段 12：交付与复盘

输出：

- `12-delivery-report.md`
- `13-retrospective.md`

交付报告必须回答：

1. 为什么需要或不需要修改 Core；
2. 修改了哪些行为、接口和状态；
3. 执行了哪些测试；
4. 哪些 Evidence 证明通过；
5. 有哪些风险；
6. 如何部署、监控和回滚。

复盘区分：需求、Skill、Core 架构、测试、外部依赖和运维问题。只有可复用经验才更新 Skill。

---

## 28. 人工检查点的轻量实现

默认：

```yaml
execution_mode: EXTERNAL_MANUAL
```

流程：

```text
生成审核包
→ Loop 标记 REVIEW_PENDING
→ 用户组织讨论或自审
→ 用户提交 decision.yaml 或自然语言结论
→ Skill 校验
→ 继续、返工、拒绝或延期
```

不要求：用户注册、登录、IAM、在线审批页面或多人投票模块。

未来 AIOS 增加审批模块后，可切换为 `INTERNAL_APPROVAL`，审核语义不变。

---

## 29. 自动化标书试点边界

标书需求通常需要：

```text
招标文件解析
评分项提取
企业资料检索
章节生成
合规检查
Word 导出
```

这些属于外部能力，不进入 Core。

只有以下通用缺口才考虑 Core：

- 编排层不能表达长任务；
- 不能保存和恢复执行状态；
- 不能锁定 Tool 版本；
- 缺少通用节点重试和错误路由；
- 缺少跨 Tool Trace；
- 缺少通用 Artifact 管理；
- 工具注册、发现和调用机制不足。

“标书需要人工审核”本身不意味着要新增登录、IAM 或审批模块。第一版使用外部人工检查点。

---

## 30. Definition of Done

### 需求

- Required AC 全部通过；
- 无未解决 BLOCKER；
- Core 准入理由仍成立。

### 架构

- 未破坏层次边界；
- 无新增循环依赖；
- 业务逻辑未进入 Core；
- 接口和状态机已文档化。

### 代码

- Go 格式和静态检查通过；
- 变更范围符合计划；
- 无禁用测试和临时绕过。

### 测试

- 单元、组件、集成和回归通过；
- Race 通过；
- 兼容、恢复和失败路径通过；
- 性能在阈值内。

### 证据

- Evidence 与 Commit 绑定；
- 命令和退出码完整；
- 人工检查有记录；
- 风险明确。

### 发布运维

- 发布、迁移和回滚可执行；
- 指标、告警和 Runbook 已定义。

---

## 31. 最终结果

### 不需要修改 Core

```text
NO_CORE_CHANGE
```

返回 Core 支撑说明、外部能力要求、集成约束和风险，不进入代码开发。

### 需要修改 Core

```text
CORE_CHANGE
```

进入：

```text
需求
→ 架构
→ 人工检查
→ Go Core 开发
→ 测试与修复 Loop
→ Evidence Gate
→ 发布
→ 运维
→ 复盘
```

该 Skill 可以持续用于 AI OS 后续的新增功能、Bug、重构、性能、可靠性和可观测性更新，同时不会扩展到外部工具实现和当前不需要的独立业务模块。
