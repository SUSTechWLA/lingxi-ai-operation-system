# 本地 IP 数字人口播渲染

当前发布路径使用 `ip形象/main_ip/character-profile.json` 作为唯一入口。角色、骨骼、口型、动作、摄像机、灯光、场景和声音都从该配置解析，避免每条视频重复建模或发生形象漂移。

## 当前主资产

```text
ip形象/main_ip/models/main-ip-aroll-master-20260720.blend
ip形象/main_ip/models/main-ip-rigged.glb
ip形象/main_ip/models/main-ip-rig-report.json
ip形象/main_ip/scenes/warm-sloth-studio-20260720.blend
ip形象/main_ip/manifests/default-aroll-assets.json
ip形象/main_ip/voice/reference/main_ip_voice_ref_v1.wav
```

主资产包含：

- 人形躯干与四肢骨骼、手腕和左右各三根独立三段手指；
- 下颌、眼球、舌头、原始面部几何和多口型控制；
- 站姿、坐姿、坐站转换、招手、解释、指向、计数、思考、点头、摇头和强调动作；
- 正面口播相机、暖色工作室和以角色为主的灯光；
- GPT-SoVITS 固定 IP 声音合同与失败关闭策略。

角色 Master 与暖色工作室是两个独立资产。工作室文件不内置角色，渲染器每次只从 Master 导入一套正式角色，因此不会因为切换景别产生重复角色、重复 Armature 或镜头专用模型。

## 系统默认 A-roll

调用方同时省略 `characterProfilePath` 和 `modelPath` 时，`ip_avatar_3d` 自动使用仓库内置的 `ip形象/main_ip/character-profile.json`。当前默认身份固定为：

- 角色：`main_ip_sloth`；
- 角色资产：`main-ip-aroll-master-20260720.blend`；
- 场景资产：`warm-sloth-studio-20260720.blend`；
- 默认构图：`front_talking`；
- 默认形态：`standing`；
- 默认输出：1920x1080、30fps、Eevee、AgX；
- 默认正式声音：`main_ip_warm_knowledge_host_v1`。

`wide`、`medium`、`close`、`three_quarter`、`transition` 和 `auto` 使用同一套角色与当前工作室，不会选择另一份角色模型。当前版本化工作室已验证的默认形态是 `standing`；需要坐姿时必须显式选择具备双形态契约的场景，不能把未校准的坐姿冒充为生产可用。资产路径和 SHA-256 由 `default-aroll-assets.json` 固定；升级时发布新的版本化文件，不覆盖旧资产。

## MCP 调用

启动服务：

```bash
python3 mcp/ip_avatar_3d/server.py
```

默认 A-roll 只需传入口播稿；以下参数展示显式覆盖写法：

```json
{
  "script": "今天我们用一个主题，走完整条 AI 视频创作流水线。",
  "presentationMode": "standing",
  "cameraPreset": "front_talking",
  "durationSec": 15,
  "resolution": {"width": 1920, "height": 1080},
  "fps": 30
}
```

当前默认工作室的 `presentationMode` 使用 `standing` 或自动选择。角色 Master 仍保留坐姿及坐站转换动作；坐姿渲染需要显式传入已完成座椅、脚底和双形态镜头校准的场景。动作时间线由脚本语义生成，不允许手穿身体、脚底漂移或手指缩放。

## 产物与门禁

典型输出包括：

```text
ip_layer.mp4
preview_frame.png
motion_plan.json
subtitle.srt
rigged_avatar.blend
rigged_avatar.glb
rig_report.json
render_report.json
```

发布视频要求：

- 1920x1080、H.264 High、yuv420p、CFR 30fps；
- AAC 48kHz，响度接近 -16 LUFS，真峰值不高于 -1.5 dBTP；
- 完整解码、帧数准确、无相邻精确重复帧；
- 长焦口播景别、手指单位缩放、抬手投影面积和画面边距通过 QA；
- 模型、场景、声音和输出 SHA 可追踪。

旧 `sprite2d` / `svg2d` 本地工具仍可加载用户自备角色包，但发布仓库不再内置旧演示角色资产。
