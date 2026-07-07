# 本地 IP 数字人口播渲染 Wiki

本文说明 `LocalIpTalkingAvatarRenderTool` 的使用边界、资产协议和验证方式。它面向口播 / 知识类视频中的 A-roll 角色层：使用已有 IP 形象、本地口播音频、字幕和背景，在用户电脑上确定性生成卡通数字人口播视频。

## 定位

这个工具不是 AIGC 视频生成器，也不调用 Seedance、Stable Diffusion、图生视频或真人口型模型。它的目标是降低口播视频的拍摄、场地和出镜成本，让波波、阿斯特这类品牌 IP 可以在 HyperGen / HyperFrames 层被稳定控制。

当前推荐模式是 `svg2d`：

- 主视觉使用 `ip形象/` 参考图生成的高保真透明纹理。
- 局部动画由时间轴控制，包括口型、眨眼、呼吸、点头、轻微漂浮、手势反馈和发光。
- `avatar_scene.json` 输出 HyperGen 可读的角色部件、运动通道和绑定建议。
- `voice_profile.json` 输出角色声音画像，区分波波和阿斯特的人设语气。

## 适用场景

| 场景 | 是否适用 | 说明 |
|---|---:|---|
| 口播 / 知识类视频 A-roll | 是 | 用本地 IP 角色承接口播正文，减少真人拍摄。 |
| 品牌角色讲解 | 是 | 可以选择 `bobo` 或 `aster`，绑定不同声音画像和动作风格。 |
| HyperGen 可控文字层 | 是 | IP 角色层可和本地字幕、标题、卡片、图表合成。 |
| AIGC 背景或 B-roll | 配合使用 | AIGC 层仍负责背景、氛围和动态素材，文字与角色层由本地渲染控制。 |
| 真实人脸数字人 | 否 | 不做真人脸、真人嘴型或换脸。 |
| 纯文生视频 | 否 | 该工具只使用已有本地角色资产。 |

## 角色资产

角色目录位于：

```text
assets/characters/{characterId}/
```

当前内置：

```text
assets/characters/bobo/
assets/characters/aster/
```

核心文件：

```text
character.json
renderer/rig.json
renderer/{character}_puppet.svg
renderer/front_cutout.png
```

`front_cutout.png` 来自 `ip形象/波波/front.png` 或 `ip形象/阿斯特/front.png` 的本地透明 cutout，用于保持和参考图一致的材质、比例、服饰和视觉细节。`rig.json` 描述 HyperGen 可控制的部件和运动通道，`puppet.svg` 是稳定部件 ID 的矢量参考。

## 输入

最小输入：

```json
{
  "characterId": "bobo",
  "script": "大家好，我是波波。今天介绍这个开源 AI 视频创作项目。",
  "outputDir": "tmp/ip_talking_avatar_bobo_promo",
  "renderMode": "svg2d",
  "resolution": { "width": 1280, "height": 720 },
  "fps": 24
}
```

生产建议传入真实口播音频：

```json
{
  "characterId": "aster",
  "script": "第一，系统把脚本、分镜、素材和合成串成可回溯流程。",
  "audioPath": "local://projects/.../narration.wav",
  "subtitlePath": "local://projects/.../subtitle.srt",
  "backgroundPath": "local://projects/.../background.png",
  "renderMode": "svg2d",
  "interactionLevel": "expressive"
}
```

如果没有 `audioPath` 但有 `script`，本地会用 macOS `say` 生成预览口播音频。该音频只用于口型和动作预览，`voice_profile.json` 会标记 `previewOnly=true`。正式成片应上传更自然的人设口播音频。

## 输出

输出目录包含：

```text
audio_analysis.json
lip_sync_timeline.json
motion_timeline.json
avatar_timeline.json
avatar_scene.json
voice_profile.json
avatar_layer.webm
final.mp4
render_report.json
```

关键产物：

| 文件 | 用途 |
|---|---|
| `final.mp4` | 合成后的完整 IP 口播视频。 |
| `avatar_layer.webm` | 透明角色层，可用于后续合成。 |
| `avatar_scene.json` | HyperGen 控制 schema，包含角色资产、运动通道、口型和动作绑定建议。 |
| `voice_profile.json` | 声音画像。波波偏年轻活泼，阿斯特偏沉稳严谨。 |
| `render_report.json` | QA 结果，检查音频、视频、时间轴、rig、参考 SVG 和最终视频是否生成。 |

## Live2D 路线

Live2D 可以用于更自然的 IP 动画，但需要真正的 Cubism 资产包，不是几张 PNG 参考图就能直接得到。

接入条件：

1. 分层美术或 PSD 已整理成 Cubism 模型。
2. 存在 `.model3.json`、纹理、物理、表情和 `motion3.json`。
3. 模型参数包含 `ParamMouthOpenY`、眼睛开合、头部角度、身体角度等。
4. 本地 renderer bridge 能读取同一份 `avatar_scene.json`，输出 `avatar_layer.webm` 或 `avatar_layer.mp4`。

当前 `live2d` 在资产协议中预留，默认不会被 Planner 当作可执行渲染模式。没有 Cubism 模型时应使用 `svg2d`。

## 验证命令

```bash
go test -C local-backend ./internal/localtool ./internal/localrunner
go test -C cloud-backend ./internal/core/worker/tool/builtin ./internal/core/localrunner
```

生成波波 demo：

```bash
TANGYING_RUN_IP_AVATAR_PROMO_DEMO=1 \
TANGYING_IP_AVATAR_PROMO_OUTPUT_DIR=tmp/ip_talking_avatar_bobo_promo \
go test -C local-backend -count=1 ./internal/localtool -run TestLocalIpTalkingAvatarProjectPromoDemo -v
```

生成阿斯特 demo：

```bash
TANGYING_RUN_IP_AVATAR_PROMO_DEMO=1 \
TANGYING_IP_AVATAR_PROMO_CHARACTER=aster \
TANGYING_IP_AVATAR_PROMO_RENDER_MODE=svg2d \
TANGYING_IP_AVATAR_PROMO_OUTPUT_DIR=tmp/ip_talking_avatar_aster_demo \
go test -C local-backend -count=1 ./internal/localtool -run TestLocalIpTalkingAvatarProjectPromoDemo -v
```

## 常见问题

- 角色看起来像贴图：确认使用的是 `frontTexture` 和 `renderMode=svg2d`，不要回退到早期几何 fallback。
- 声音不够自然：本地 `say` 只是预览，正式口播需要上传匹配 IP 的高质量音频。
- 字幕没有烧录：本机 FFmpeg 可能缺少 `subtitles/libass` 滤镜；工具会生成无烧录字幕的视频，并保留 SRT 给前端预览。
- 想要 1:1 动作还原：需要 Live2D Cubism 或其他分层骨骼资产。当前 `svg2d` 先保证视觉还原和基础口播动作可控。
