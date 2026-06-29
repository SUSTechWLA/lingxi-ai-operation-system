# AIOS 自媒体视频创作升级文档包

- 基线仓库：https://github.com/SUSTechWLA/tangying-ai-operation-system
- 基线分支：`develop_go`
- 文档日期：2026-06-17
- 目标：将现有 AIOS 增量升级为可稳定执行两类视频 Skill 的云端 Agent 系统，并通过 Electron 完成本地素材、HyperGenKeyframe 与 FFmpeg 执行。
- 适用执行者：Codex、Claude Code、其他可读写代码仓并执行测试的 Coding Agent。

## 文档入口

1. `AIOS_UPGRADE_MASTER_SPEC.md`：合并版主规格，适合直接交给 Coding Agent。
2. `01_REQUIREMENTS_AND_SCOPE.md`：需求升级、范围、用户故事、验收指标。
3. `02_TECHNICAL_DESIGN.md`：模块、数据、接口、事件、工作流、前后端详细设计。
4. `03_IMPLEMENTATION_PLAN.md`：分阶段任务、代码路径、依赖和 Definition of Done。
5. `04_TEST_AND_ACCEPTANCE.md`：自动化测试、故障注入、验收门禁和回归策略。
6. `05_AGENT_EXECUTION_PROMPT.md`：可直接复制给 Coding Agent 的执行指令。
7. `TASK_STATUS_TEMPLATE.md`：Agent 每完成一个任务后必须更新的状态文件模板。
8. `contracts/`：Skill 与 Workflow 的建议 JSON Schema。
9. `workflows/`：两类视频生产工作流的参考 YAML。

## 使用方式

将整个目录复制到仓库，例如：

```bash
mkdir -p docs/upgrade/video-creation-v1
cp -R aios_video_upgrade_spec/* docs/upgrade/video-creation-v1/
```

然后对 Coding Agent 下达：

```text
请先阅读 docs/upgrade/video-creation-v1/05_AGENT_EXECUTION_PROMPT.md，
严格按照文档中的阶段、测试门禁和兼容性要求升级项目。
先只执行 P0 基线检查和 P1 数据模型，不要一次完成全部阶段。
```

## 强制原则

- 先建立基线测试，再修改代码。
- 保留现有 `/api/publish`、`/api/skill/dialog/*`、`/api/task/*`、素材库和旧前端能力。
- 不重写 Orchestrator、Worker、Outbox、Tool Registry 和 Rust 沙箱。
- 新增视频创作能力进入 `internal/core`；视频业务进入 `internal/agents/video`。
- 所有外部模型调用必须通过统一 Model Gateway，默认测试禁止调用真实付费 API。
- 每个阶段必须具备单元测试、集成测试或契约测试，失败不得进入下一阶段。
- 所有数据库变更必须可重复执行、可回滚或向后兼容。
