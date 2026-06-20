# Coding Agent 主执行指令

你正在升级仓库 `SUSTechWLA/tangying-ai-operation-system` 的 `develop_go` 分支。

## 目标

按照本目录文档，将系统升级为能稳定运行以下两类视频生产工作流的自媒体 AIOS：

1. AIGC 镜头式视频。
2. 文字口播 + HyperGenKeyframe 可视化视频。

## 必读顺序

1. `README.md`
2. `01_REQUIREMENTS_AND_SCOPE.md`
3. `02_TECHNICAL_DESIGN.md`
4. `03_IMPLEMENTATION_PLAN.md`
5. `04_TEST_AND_ACCEPTANCE.md`
6. `TASK_STATUS.md` 或 `TASK_STATUS_TEMPLATE.md`

同时阅读仓库：

- 根目录 `AGENTS.md`
- 根目录 `CLAUDE.md`
- `aios-core/docs/AIOS_CORE_BACKEND_BOUNDARY.md`
- `aios-core/docs/ARCHITECTURE.md`
- `aios-core/docs/TOOL_DEVELOPMENT_GUIDE.md`

## 强制执行方式

### 1. 不得一次实现全部

从 P0 开始，一次只完成一个 Task。完成后：

- 运行该 Task 测试。
- 运行全量回归。
- 更新 `TASK_STATUS.md`。
- 输出变更摘要和风险。

### 2. 先检查实际代码

文档是设计目标，不替代源码事实。任何修改前：

- 定位当前包和接口。
- 搜索现有同类实现。
- 优先复用。
- 如果文档路径与源码不一致，保持架构边界并在状态文件记录差异。

### 3. 不重写已有核心

禁止重写：

- Orchestrator DAG。
- Worker/Tool Registry。
- Outbox。
- Rust Sandbox。
- 旧 Publish/Chat/Media API。

只做必要的接口扩展和依赖注入。

### 4. 测试优先

每个 Task：

1. 写或更新测试。
2. 实现最小代码。
3. 运行目标测试。
4. 运行 `go test -race ./...`。
5. 不得通过删除测试、跳过断言或调用真实收费 API 解决失败。

### 5. 数据兼容

- 新表优先。
- 旧表和 API 保留。
- migration 可重复。
- 新功能由 feature flag 控制。
- 默认关闭或不影响旧功能，直到对应 Phase 验收完成。

### 6. 外部模型

- 使用 Model Gateway。
- 自动测试只用 Fake Provider。
- 不把 Key、Token、base64 大结果写入日志。
- 长视频生成不得阻塞 HTTP Handler。

### 7. Electron 安全

- 不接受任意 Shell。
- 只执行 command type 白名单。
- 校验 project root 和路径。
- 使用 context isolation。
- 云端 Token 使用安全存储。

## 当前执行命令

第一次执行时只完成：

```text
P0-T01 仓库基线清点
```

不要开始 P1。生成：

```text
docs/upgrade/video-creation-v1/BASELINE_TEST_REPORT.md
docs/upgrade/video-creation-v1/TASK_STATUS.md
```

报告至少包含：

- 当前 commit。
- 实际目录树。
- 现有测试清单。
- 后端测试结果。
- 前端 build 结果。
- 当前已知失败。
- 文档设计与实际代码差异。
- 下一任务建议。

## 每轮输出格式

```text
完成任务：
修改文件：
新增测试：
执行命令：
测试结果：
兼容性影响：
剩余风险：
下一任务：
```

遇到无法访问的外部服务时，使用 Fake/Mock 继续，不得把任务停在“等待用户提供 API Key”。
