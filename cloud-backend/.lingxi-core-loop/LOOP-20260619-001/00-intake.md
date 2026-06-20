# 00 Intake

## Mode

`solution-design` with `CORE_CHANGE_CANDIDATE`.

## User Request

将 `.codex/skills/` 中 4 个业务 skill 固化为当前 AIOS Core 后端 agent 可支持的稳定工作流：

- `voice-post-production`: 人声后处理。
- `create-opinion-videos`: 口播观点输出类视频，通过 HyperFrames/keyframes 剪辑。
- `film-shot-reconstruction`: 拉片重构与素材库导入。
- `video-creator`: Markdown-first AIGC 视频生成流水线。

## Initial Boundary

Core 负责 skill runtime、workflow 编译、工具治理、阶段门、长任务追踪、Artifact/Trace 契约和外部工具调用约束。

Core 不实现图像生成、视频生成、HyperFrames 渲染、FFmpeg 拉片、人声音频 DSP 或素材库脚本内部算法；这些作为外部工具黑盒能力接入。

## Assumptions

- 第一版以 `skills/<name>/1.0.0/skill.yaml` 作为生产 runtime package，不直接运行 Codex `SKILL.md`。
- `.codex/skills/*` 继续作为方法论源文档，生产运行读取拆分后的 stage instruction。
- 启动自动注册 skill workflow 只在 `VIDEO_CREATION_ENABLED=true` 时启用。
