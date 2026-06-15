# Changelog

## 5.0.0

- 将 skill 从单纯 Core Loop 扩展为“客户需求 → AIOS solution → Core Gap → Core Loop”。
- 新增前端需求、Workflow 规格、API/事件契约和 Solution 蓝图输出。
- 明确客户前端和外部工具只定义要求与黑盒验收，不生成实现代码。
- 将默认 loop 模式更新为 `solution-design`，保留 Core update / bugfix / reliability 等模式。

## 4.0.0

- Skill 完全收缩为 AIOS Core 更新与治理流程。
- 删除外部工具源码开发和 Tool Developer Agent。
- 删除对 IAM、登录和在线审批模块的依赖。
- 人工审核改为 `EXTERNAL_MANUAL` 轻量检查点。
- 增加 Core 更新、Bug、重构、性能、可靠性和可观测性模式。
- 增加 `NO_CORE_CHANGE` 快速路径。
- 保留外部能力指标和黑盒集成约束，不管理工具实现。
- 强化 Go Core 测试、Evidence、发布、运维和复盘闭环。
