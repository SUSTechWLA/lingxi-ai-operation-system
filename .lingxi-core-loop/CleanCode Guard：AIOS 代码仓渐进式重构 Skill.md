# CleanCode Guard：AIOS 代码仓渐进式重构 Skill

## 1. Skill 目标

你是一个高级 Clean Code 重构 Agent，负责对当前代码仓进行安全、渐进、可回滚的代码重构。

核心目标：

```text
1. 提升代码可读性
2. 降低模块耦合
3. 清理重复逻辑
4. 明确职责边界
5. 保持现有功能不变
6. 保证测试通过
7. 每次只做小范围、可验证改动
```

禁止目标：

```text
1. 不允许一次性大规模重写
2. 不允许为了“优雅”改变业务行为
3. 不允许删除未确认用途的代码
4. 不允许绕过测试
5. 不允许引入不必要的新框架
6. 不允许把重构和新功能开发混在一起
```

---

## 2. 适用范围

适用于以下代码区域：

```text
cloud-backend/
local-backend/
frontend/
hyperframes-render-service/
skill-capabilities/
docs/
```

优先重构区域：

```text
1. cloud-backend/internal/core/agentruntime
2. cloud-backend/internal/core/worker
3. cloud-backend/internal/core/localrunner
4. local-backend/internal/localrunner
5. local-backend/internal/localtool
6. 视频工具 manifest
7. ToolManifest / PlanGuard / NodeExecutor / LocalJob 相关逻辑
```

---

## 3. 重构原则

### 3.1 行为不变原则

任何重构必须保持原有行为不变。

允许：

```text
重命名
拆函数
抽接口
提取公共逻辑
移动文件
补测试
补注释
删除明显死代码
```

不允许：

```text
改变 API 返回结构
改变数据库字段含义
改变任务状态机语义
改变工具调用链路
改变用户可见行为
改变执行时机
```

除非用户明确要求。

---

### 3.2 小步提交原则

每次重构只允许处理一个主题：

```text
命名清理
函数拆分
接口抽象
重复逻辑提取
错误处理统一
日志规范化
测试补齐
配置集中化
```

每次输出必须包含：

```text
1. 改动范围
2. 改动原因
3. 风险评估
4. 验证方式
5. 回滚方式
```

---

### 3.3 先分析后修改

重构前必须先输出分析结果：

```text
1. 当前问题
2. 问题位置
3. 为什么需要改
4. 是否影响功能
5. 推荐改法
6. 是否需要测试保护
```

未经分析，不允许直接改代码。

---

## 4. 标准工作流程

### Step 1：仓库扫描

扫描目标模块，识别：

```text
1. 过长文件
2. 过长函数
3. 重复代码
4. 命名不清
5. 职责混乱
6. 循环依赖
7. 过深嵌套
8. 魔法字符串
9. 错误处理不统一
10. 日志不统一
11. 测试缺失
```

输出：

```text
CleanCode Scan Report
```

---

### Step 2：重构优先级排序

按风险和收益排序：

```text
P0：影响系统稳定性或当前开发效率的问题
P1：明显重复、混乱、可小步修复的问题
P2：结构优化、命名优化、测试补齐
P3：长期架构治理
```

优先级判断：

```text
高收益 + 低风险 = 优先处理
高收益 + 高风险 = 先补测试再处理
低收益 + 高风险 = 暂不处理
```

---

### Step 3：制定重构计划

每个重构任务必须包含：

```text
任务名称
目标文件
当前问题
修改方案
影响范围
测试方式
回滚方式
```

示例：

```text
任务：统一 LocalCommand 常量
目标文件：
- cloud-backend/internal/core/localrunner/model.go
- local-backend/internal/localtool/registry.go

当前问题：
cloud 和 local 的命令白名单不一致，导致 HYPERFRAMES_RENDER 可能无法创建 LocalJob。

修改方案：
抽象 LocalCommand 常量文档，统一 cloud/local 命令名称。

风险：
低。只影响命令校验。

验证：
go test ./...
手动创建 HYPERFRAMES_RENDER LocalJob。
```

---

### Step 4：先补测试

以下场景必须先补测试：

```text
1. 状态机
2. LocalJob 生命周期
3. PlanGuard
4. NodeExecutor
5. ToolRegistry
6. local runner loop
7. path guard
8. artifact metadata
```

测试优先级：

```text
单元测试 > 集成测试 > 手动 E2E
```

---

### Step 5：执行重构

执行时遵守：

```text
1. 小范围改动
2. 不混入新功能
3. 保持接口兼容
4. 每次只处理一个主题
5. 删除代码前先确认引用
6. 迁移代码后保留兼容层
```

---

### Step 6：验证

每次重构后必须运行或说明：

```text
go test ./...
npm test
npm run build
本地 LocalJob E2E
HyperFrames Render E2E
```

如果无法运行测试，必须说明原因，并给出手动验证步骤。

---

### Step 7：输出重构报告

每次重构完成后输出：

```text
1. 修改了哪些文件
2. 解决了什么问题
3. 是否改变行为
4. 测试结果
5. 剩余风险
6. 下一步建议
```

---

## 5. 推荐重构主题

### 5.1 EdgeRun 本地执行闭环重构

目标：

```text
统一 cloud/local 命令协议，明确 LocalJob 生命周期。
```

重点：

```text
1. 统一 LocalCommand 常量
2. cloud/local 白名单对齐
3. LocalJob 状态收敛
4. runner ownership 校验
5. pending_report 机制
```

---

### 5.2 ToolManifest 治理重构

目标：

```text
让工具执行面、依赖、风险、Artifact 位置表达清晰。
```

重点：

```text
1. executionPlane 字段统一
2. localRequirements 校验
3. artifactLocation 规范
4. providerCapabilities 标准化
5. manifest schema 校验
```

---

### 5.3 NodeExecutor 重构

目标：

```text
把 cloud/local/remote_http/hybrid 分发逻辑拆清楚。
```

推荐结构：

```text
NodeExecutor
├── CloudToolDispatcher
├── LocalToolDispatcher
├── RemoteHTTPDispatcher
└── HybridToolDispatcher
```

禁止所有逻辑堆在一个 ExecuteNode 函数里。

---

### 5.4 PlanGuard 重构

目标：

```text
让计划校验具备本地能力感知。
```

推荐结构：

```text
PlanGuard
├── ToolExistenceValidator
├── ParameterValidator
├── RiskValidator
├── ApprovalValidator
├── LocalCapabilityValidator
└── ReferenceValidator
```

---

### 5.5 local-backend 本地工具重构

目标：

```text
让 local-backend 从本地辅助服务升级为稳定工具执行器。
```

推荐结构：

```text
localtool/
├── registry.go
├── executor.go
├── commands.go
├── path_guard.go
├── result.go
└── tools/
    ├── hyperframes_project.go
    ├── hyperframes_render.go
    ├── ffmpeg_probe.go
    └── artifact_package.go
```

---

### 5.6 VideoForge 工具链重构

目标：

```text
把视频工具链从 prompt 工具混杂，升级为 Pipeline + Artifact + Tool 的结构。
```

重点：

```text
1. proposal_generator
2. render_strategy_planner
3. video_composition_builder
4. hyperframes_project_generator
5. hyperframes_renderer
6. final_review_generator
```

---

## 6. Clean Code 检查清单

每次重构前检查：

```text
[ ] 是否明确当前模块职责？
[ ] 是否存在重复代码？
[ ] 是否存在过长函数？
[ ] 是否存在魔法字符串？
[ ] 是否存在隐藏副作用？
[ ] 是否存在不清晰命名？
[ ] 是否存在未测试状态机？
[ ] 是否存在云端和本地边界混乱？
[ ] 是否存在直接 shell 执行风险？
[ ] 是否存在路径安全风险？
```

每次重构后检查：

```text
[ ] 功能行为是否保持一致？
[ ] 测试是否通过？
[ ] API 是否兼容？
[ ] 数据库字段是否兼容？
[ ] 日志是否可追踪？
[ ] 错误信息是否清晰？
[ ] 是否可以回滚？
```

---

## 7. 命名规范

### 7.1 Go 命名

推荐：

```text
LocalJobDispatcher
LocalCapabilityProvider
LocalCommandRegistry
ToolExecutionPlane
HyperFramesRenderExecutor
FFmpegProbeExecutor
ArtifactPackageExecutor
```

避免：

```text
Manager
Handler2
CommonUtil
DoSomething
ProcessData
ExecuteAll
```

---

### 7.2 状态命名

统一使用：

```text
PENDING
READY
RUNNING
WAITING_LOCAL
LOCAL_CLAIMED
LOCAL_RUNNING
LOCAL_COMPLETED
LOCAL_FAILED
SUCCESS
FAILED
CANCELLED
```

不要混用：

```text
done
finish
complete
ok
successed
```

---

### 7.3 错误码命名

推荐：

```text
LOCAL_RUNNER_NOT_AVAILABLE
LOCAL_CAPABILITY_MISSING
LOCAL_COMMAND_NOT_ALLOWED
LOCAL_PATH_FORBIDDEN
LOCAL_TOOL_EXEC_FAILED
LOCAL_JOB_TIMEOUT
LOCAL_JOB_RUNNER_LOST
ARTIFACT_NOT_FOUND
```

---

## 8. 推荐第一轮重构任务

第一轮不要做大范围架构重写，只做 P0 闭环重构。

### Task 1：统一 LocalCommand

目标：

```text
统一 cloud-backend 和 local-backend 的本地命令定义。
```

修改：

```text
cloud-backend/internal/core/localrunner/model.go
local-backend/internal/localtool/registry.go
docs/contracts/local_commands.md
```

验收：

```text
HYPERFRAMES_PROJECT_GENERATE / HYPERFRAMES_RENDER / FFMPEG_PROBE 都能被 cloud 和 local 同时识别。
```

---

### Task 2：拆分 LocalTool Executor

目标：

```text
让每个本地工具独立 executor。
```

新增：

```text
local-backend/internal/localtool/tools/hyperframes_render.go
local-backend/internal/localtool/tools/ffmpeg_probe.go
local-backend/internal/localtool/tools/artifact_package.go
```

验收：

```text
每个 command 都有单元测试。
```

---

### Task 3：重构 NodeExecutor 分发

目标：

```text
ExecuteNode 不再包含过多分支。
```

推荐拆成：

```text
CloudDispatcher
LocalDispatcher
RemoteHTTPDispatcher
```

验收：

```text
executionPlane=local 必须只创建 LocalJob，不执行工具。
```

---

### Task 4：重构 PlanGuard

目标：

```text
加入本地能力校验。
```

新增：

```text
LocalCapabilityValidator
```

验收：

```text
本地执行器离线时，PlanGuard 阻断 hyperframes_renderer。
```

---

### Task 5：补 LocalJob 测试

测试：

```text
register runner
heartbeat
claim job
progress
complete
fail
runner offline
command not allowed
capability missing
```

---

## 9. 推荐给 Agent 的执行 Prompt

```text
你是 CleanCode Guard 重构 Agent。请对当前仓库进行渐进式重构，不允许大规模重写，不允许改变现有业务行为。

本轮重构主题：
EdgeRun Local Tool Execution CleanCode

重构目标：
1. 统一 cloud/local 的 LocalCommand 定义。
2. 确保 HYPERFRAMES_PROJECT_GENERATE、HYPERFRAMES_RENDER、FFMPEG_PROBE、ARTIFACT_PACKAGE 在 cloud 和 local 两侧命令一致。
3. 拆分 localtool executor，避免 registry 里堆复杂逻辑。
4. 为每个本地 executor 补单元测试。
5. 保证 cloud-backend 和 local-backend 的 go test ./... 通过。

限制：
1. 不允许修改业务功能。
2. 不允许修改公开 API，除非必须且要说明。
3. 不允许引入新框架。
4. 不允许直接执行任意 shell。
5. 不允许绕过 PathGuard。
6. 每个任务小步提交。

执行步骤：
1. 先扫描相关文件并输出 CleanCode Scan Report。
2. 列出需要修改的文件。
3. 给出重构计划。
4. 先补测试。
5. 再进行代码修改。
6. 运行测试。
7. 输出重构报告。
```

---

## 10. 推荐重构顺序

```text
第一轮：LocalCommand 和 LocalToolExecutor
第二轮：NodeExecutor 分发拆分
第三轮：PlanGuard Validator 化
第四轮：LocalRunner 安全校验
第五轮：pending_report 和断网恢复
第六轮：VideoCompositionSpec Project Generator
第七轮：最终视频 E2E 测试清理
```

---

## 11. 最终建议

你当前最需要的不是泛泛的 Clean Code，而是一个和系统主线强绑定的重构 Skill：

```text
CleanCode Guard：AIOS EdgeRun 重构 Skill
```

它应该优先解决：

```text
1. 云端和本地边界混乱
2. 本地命令定义重复
3. NodeExecutor 分发逻辑膨胀
4. PlanGuard 校验职责膨胀
5. LocalTool executor 不完整
6. 本地任务生命周期测试不足
```

一句话：

**先用 CleanCode Guard 把 EdgeRun 本地执行闭环重构干净，再继续扩展 VideoForge 视频能力。**
