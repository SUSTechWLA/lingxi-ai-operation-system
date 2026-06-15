# 00-Intake: 自动化标书生成功能

## 输入

- **客户需求**: 在 AIOS 平台上实现自动化标书生成能力
- **运行模式**: solution-design
- **仓库位置**: develop_go 分支，Core 代码在 `aios-core/`
- **提交 SHA**: 3271747 (前后端分离完成)

## 已知限制

1. 当前 AIOS Core 是 Go 单体应用，模块化但共享进程
2. 编排层支持 DAG 任务图，节点级重试和状态管理
3. 工具层支持 builtin + external 工具注册
4. 无内置 IAM/审批/用户中心模块
5. 无内置文档生成（Word/PDF）能力
6. 无内置招标文件解析能力

## 风险等级: MEDIUM

标书生成涉及长任务、多步骤编排、外部工具依赖，核心风险在于：
- 需求边界不清，可能过度修改 Core
- 长任务编排和状态恢复能力不足
- 与现有 publish/media/skill 模块的交互

## 初步影响层

- 入口层: 可能需要新的 API 端点
- 编排层: 可能需要长任务支持、人工检查点集成
- 工具层: 需要注册多个外部工具（解析、检索、生成、导出）
- 日志层: 需要增强 Trace 支持长任务审计

## 假设和未决问题

1. [ASSUMPTION] 标书生成是异步长任务，不是同步请求
2. [ASSUMPTION] 招标文件解析需要外部 NLP/OCR 能力
3. [ASSUMPTION] 企业资料检索需要外部知识库/向量数据库
4. [ASSUMPTION] Word 导出是外部工具能力，不进 Core
5. [QUESTION] 是否需要人工审核节点？审核是 Core 能力还是外部流程？
6. [QUESTION] 标书模板管理是 Core 能力还是 Workflow 配置？
7. [QUESTION] 是否需要多轮对话式标书生成（类似现有 skill 模块）？
8. [QUESTION] 是否需要对接现有的 media 模块处理招标附件？

## 初始状态

```yaml
core_write_enabled: false
```
在完成 Core Gap 决策并通过人工检查点之前，不修改任何 Core 代码。
