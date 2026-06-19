# 01 Core Discovery

## Existing Core Capabilities

- `internal/core/orchestrator`: DAG、节点状态、依赖检查、重试、暂停/恢复、长任务 heartbeat/progress。
- `internal/core/worker`: tool registry、内置 tool、external HTTP tool bridge、sandbox/direct executor。
- `internal/core/workflow`: workflow template、run tracking、skill compiler、`cmd/skill2workflow`。
- `internal/core/skillruntime`: 从 `skills/{name}/{version}/skill.yaml` 加载 skill package，并暴露 `/api/skills`。
- `internal/core/artifact`: versioned artifact model/service/repository。
- `internal/agents/video`: video project 和 workflow run API 骨架。

## Existing Skill Inputs

- `.codex/skills/create-opinion-videos/SKILL.md`: 口播稿和 HyperFrames reference 文档优先，必要时生成图片和渲染。
- `.codex/skills/film-shot-reconstruction/SKILL.md`: 临时源片分析与最终原创重构包严格隔离，多个人工确认门。
- `.codex/skills/video-creator/SKILL.md`: Markdown-first 9 阶段视频生产骨干，API manifest 仅用于图片/视频生成。
- `.codex/skills/voice-post-production/skill.md` and `processor.py`: 音频格式统一、响度、EQ、压缩、合并。

## Gaps Found

- Runtime loader 只支持 `skills/{name}/{version}/skill.yaml`，业务 skill 尚未转为 production package。
- Compiler 原先生成 `skill_stage` tool node，但 Core 没有该内置 tool，编译结果不能稳定执行。
- External tool bridge 需要 `node.name=external` 且 `input.parameters.tool=<external_name>`；顶层 `input.tool` 会被路由器误当成 builtin tool 名。
- Optional stage 旧编译逻辑将上游依赖连到 approval 而不是 exec，导致可选执行分支可能提前运行。
- CONTROL 节点 READY 后会被投递给 worker，人工成功后也不会自动唤醒下游，无法支撑视频流水线阶段门。
- 自动注册 skill workflow 使用随机 template ID，重启会产生重复模板，不适合版本锁定。
