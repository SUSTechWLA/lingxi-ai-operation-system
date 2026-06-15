# Lingxi AI OS Core Loop Engineering Skill V5.0.0

这是灵犀 AIOS Core 后端服务的软件开发闭环和客户 solution 转化 Skill。

## 管理范围

- AIOS Core Go 代码；
- 入口层、编排层、工具层、日志层和中间件适配；
- Core 架构、测试、发布和运维；
- 将客户需求转成完整 AIOS solution；
- 客户需求是否需要修改 Core 的判断；
- 前端团队和外部工具团队的接口、能力和验收要求。

## 不管理

- 外部 Python/FastAPI 工具实现；
- 工具算法和模型选型；
- 客户专属前端代码；
- 登录、IAM、在线审批等当前非核心模块；
- 标书、论文等具体业务代码。

## 人工检查

保留人工检查点，但使用外部会议或自审记录：

```text
生成检查包 → 暂停流程 → 讨论/自审 → 提交结论 → 继续或返工
```

不要求 AIOS 当前具备登录和审批系统。

## 调用示例

```text
/lingxi-aios-core-loop --mode core-update
需求：为编排层增加运行暂停和恢复能力。
```

```text
/lingxi-aios-core-loop --mode solution-assessment
需求：分析自动化标书是否需要修改 AIOS Core。
```

```text
/lingxi-aios-core-loop --mode solution-design
需求：把客户的合同审查工作台需求转成 AIOS solution。
```

首次运行必须先完成 discovery 和 core-gap-decision；未得到 `CORE_CHANGE_CANDIDATE` 前不得修改代码。
