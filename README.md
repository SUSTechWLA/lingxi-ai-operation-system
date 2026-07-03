<p align="center">
  <img src="frontend/src/assets/aios-icon.png" width="112" alt="Tangying AI Operation System" />
</p>

<h1 align="center">躺营 AI 自媒体运营系统</h1>

<p align="center">
  从一个想法开始，把选题、脚本、分镜、素材生成、审核、渲染和交付串成一条可追踪的视频创作流水线。
</p>

<p align="center">
  <a href="https://github.com/SUSTechWLA/tangying-ai-operation-system/wiki">项目 Wiki</a>
  ·
  <a href="https://github.com/SUSTechWLA/tangying-ai-operation-system/wiki/English">English Wiki</a>
  ·
  <a href="#快速开始">快速开始</a>
  ·
  <a href="#适合谁">适合谁</a>
</p>

<p align="center">
  <img alt="Release" src="https://img.shields.io/badge/Release-v0.1.7-111827?style=for-the-badge" />
  <img alt="Video Workflow" src="https://img.shields.io/badge/Video%20Workflow-Cloud%20Orchestration%20%2B%20Local%20Runner-5B6CFF?style=for-the-badge" />
  <img alt="Desktop Client" src="https://img.shields.io/badge/Desktop-React%20%2B%20Electron-16A085?style=for-the-badge" />
  <img alt="Backend" src="https://img.shields.io/badge/Backend-Go-2F80ED?style=for-the-badge" />
</p>

---

## Release 更新

### v0.1.7 - 2026-07-04

- 优化 Dreamina/JiMeng 视频投放提示词：从“工程说明模板”改为 Vibe Creator 画面叙述，按 0-2 秒、2-4 秒、4-6 秒等时间段写清具体画面、对象变化和表达思想。
- 即梦视频 prompt 不再混入 `AIGC_VIDEO`、`b-roll`、`ffmpeg`、`SHOT_VIDEO_CLIP`、`素材意图`、`镜头运动` 等内部标签或拼接说明。
- 系统会把导演字段、口播意图和趣味节拍转写成可见画面，例如创作桌、便利贴、传送带、即梦素材工位、抽帧 QA 放大镜、视频胶囊等具体元素。
- 新增回归测试，防止后续再次把内部生产说明泄漏到外部视频模型 prompt。

### v0.1.6 - 2026-07-04

- MCP AIGC 生成结果新增 `sourceSummary` 和 `assetProvenance`：每个请求都会记录 provider、tool、kind、status、storageRef、localPath、失败原因和是否需要 fallback。
- 自动插入的 `mcp_generation_runner` 默认要求至少 1 个 ready 的 AIGC 视频素材；如果 Dreamina/JiMeng 视频全部失败，系统会明确标记 `externalVideoRequirementSatisfied=false`，不能把 HyperFrames/storyboard fallback 误当成即梦成片。
- 即梦素材位置更清晰：成功下载的 MCP 素材会保存在本地 `TangyingAIOS/cache/mcp/<projectId>/<requestId>/`，并导入 `TangyingAIOS/artifacts/<projectId>/<requestId>/content`。
- 文档补充了“ready / failed / deferred / fallback”的区别，避免外部平台额度消耗后看不到哪些素材真正来自即梦。

### v0.1.5 - 2026-07-04

- 新增影视类 `cinematic_story` 创作链路：故事大纲、详细剧本、角色/场景/道具档案、多视角参考图、3-15 秒 shot、运镜光影和画面意义会被拆成可审核阶段。
- 口播和影视视频都强化“口播稿/剧本先行，脚本再决定素材”的流程，系统会区分 HyperFrames 精确文字层、页面录屏、Dreamina/JiMeng AIGC 图片和 AIGC shot。
- 即梦/Dreamina 图片和视频统一通过标准 MCP 调用：参考图走 `jimeng.generate_image`，shot 视频走 `jimeng.generate_video`，本地 runner 不再新增 provider 专用工具。
- Shot-level QA 升级为质量门：每个 shot 输出剧本匹配度、导演理由、参考资产覆盖数、动作节拍数、视觉复杂度、文字安全区和返修决策。
- 修复影视链路中模型输出漂移导致参考资产缺失、MCP 图片空参数、Dreamina 图片分辨率参数不兼容、render 超时阻塞后续 QA 的问题。
- 已用 release 分支当前代码跑通 16:9 正能量搞笑影视宣传短片 E2E：`vp-b1a3a300`，29 个节点全成功，最终视频 1920x1080 / 18 秒，QA `score=100`，4 个 shot 全部 `PASS`。
- 验证中 Dreamina 图片 MCP 已成功生成 1 个参考图，其余按预算 defer；Dreamina 视频 MCP 因账号余额不足返回 `CreditPreDeductNotEnough`，系统保留失败请求并自动走可 QA 的 fallback 渲染。

### v0.1.4 - 2026-07-03

- 将视频抽帧 QA 拆成标准 Python MCP 服务：`mcp/video_qa/server.py` 暴露 `video_qa.analyze_video`，后续 OCR、ASR、PyIQA、VLM judge 都可以继续按 MCP 扩展。
- 本地 `VIDEO_FRAME_QA` 只保留路径校验、`local://` 解析和 MCP 调用，不再把重媒体 QA 算法写死在 Go runner 中。
- 成片 QA 输出稳定升级为 `SHOT_QA_REPORT` 和 `SHOT_REPAIR_PLAN`，前端审核面板优先展示 shot 级结论、量化指标和返修建议。
- 新增 Python MCP server 自测、Go stdio MCP 集成测试和 `VIDEO_FRAME_QA` MCP E2E 测试，确保 QA 能力能作为独立 MCP 服务被系统调用。

### v0.1.3 - 2026-07-03

- 升级视频 QA 为 shot 级质量检查：每个 shot 都输出抽帧数、平均/最大视觉密度指标、质量分、结论和返修建议。
- 新增 `repairPlan` 聚合结论，可直接区分 `approve`、`manual_review` 和 `regenerate_shots`，为后续自动返修重生成提供稳定接口。
- 云端 `visual_qa` 节点正式声明 `shotSummaries`、`repairPlan`、`needsRegeneration` 输出，审核门和后续节点可稳定读取。
- 更新视频 QA Wiki，说明 shot 级量化指标、阻断/警告策略和返修建议字段。
- 已用 30 秒 16:9 “视频 Agent”宣传片跑通脚本、预览、渲染、shot 级 QA、发布文案完整链路。

### v0.1.2 - 2026-07-03

- 新增成片抽帧 QA 链路：`VIDEO_FRAME_QA` 会在本地渲染后自动抽帧，输出 `video_frame_qa.json` 和 contact sheet，再进入人工审核门。
- 动态视频计划会在 render 后、publish 前自动插入 `visual_qa`，QA 未通过或未确认时阻断后续发布文案和交付。
- 修复本地 artifact 传递：下游本地工具可从上游 `VIDEO` artifact 的 `localPath` 解析出真实 `outputPath`。
- 优化 local runner 失败回报清理，避免旧 token 或旧 runner 产生的 pending report 反复阻塞新任务。
- 优化 QA contact sheet 排布，抽帧少于 16 张时按实际帧数动态成图，减少空黑区域，方便人工复看。
- 已用 30 秒 16:9 项目开源上线宣传视频跑通脚本、审核、预览、渲染、抽帧 QA、发布文案完整链路。

### v0.1.1 - 2026-07-03

- 跑通影视化 / AIGC shot 视频从页面启动、审核门确认、即梦 MCP 提交、素材下载、本地渲染到最终 artifact 的完整闭环。
- 修复成功任务被旧失败状态覆盖的问题：底层 `ai_task` 已成功时，`agent_run` 可恢复为 `SUCCESS`。
- 修复本地 runner 被已取消或已失败旧任务阻塞的问题，避免 stale local job 影响新项目执行。
- 增强 HyperFrames 本地快速渲染兜底：可按 storyboard 生成 16:9 成片，并严格控制最终时长。
- 修复产物页外部生成请求统计：MCP 自动生成落成 `SHOT_VIDEO_CLIP` 后，不再误提示“待回填素材”。

### v0.1.0 - 2026-07-03

- 统一 CLI 扩展接入方式：后续本地 CLI 能力必须封装为标准 MCP server，再通过本地 Agent provider 注册进入系统。
- 新增 `mcp/` 目录，集中管理所有 MCP 服务；即梦 Dreamina CLI 已迁移到 `mcp/jimeng/server.py`。
- 删除 Go 版即梦 CLI 直连服务，云端统一使用 `mcp_generation_runner`，本地通过标准 MCP provider 调用具体 CLI 能力。
- 完整跑通即梦 MCP 的 provider 注册、`tools/list`、登录态复用和本地 stdio 调用验证。
- 修复视频创作全流程中的计划补全、审核、外部生成请求、本地 artifact 和渲染交付问题。

## 一句话理解

躺营不是一个单点的“文生视频按钮”，而是一个把真实创作过程拆成可审核阶段的 AI 视频制片台。它让云端负责编排，让用户电脑负责本地工具执行和文件生产，让创作者在关键节点确认方向，避免黑盒式生成。

<table>
  <tr>
    <td width="33%">
      <h3>给内容创作者</h3>
      <p>输入主题后，系统按视频类型拆出方案、脚本、分镜、素材需求、预览和成片，减少从零组织流程的成本。</p>
    </td>
    <td width="33%">
      <h3>给团队负责人</h3>
      <p>每个阶段都有产物、状态和审核记录，方便知道项目卡在哪里、谁需要确认、哪些素材还缺。</p>
    </td>
    <td width="33%">
      <h3>给技术团队</h3>
      <p>云端编排、本地 runner、工具 manifest、MCP 扩展和桌面端解耦，方便继续接入新模型和新工具。</p>
    </td>
  </tr>
</table>

## 当前能做什么

| 能力 | 体验结果 |
|---|---|
| 影视化 / AIGC shot 视频 | 从故事大纲、详细剧本、角色/场景/道具档案、多视角参考图到 shot 级生成提示词和 QA |
| 口播 / 知识类视频 | 先生成口播稿，再按口播设计 HyperFrames、录屏、AIGC 图片/视频素材和最终成片 |
| 分阶段审核 | 方案、脚本、分镜、预览、渲染等节点可确认、拒绝、编辑或重新生成 |
| 本地执行器 | 用户电脑负责本地文件、HyperFrames 项目、渲染和工具执行 |
| 即梦 JiMeng MCP 扩展 | 用户显式安装并登录 Dreamina CLI 后，可通过本地 MCP 自动生成 AIGC 素材 |
| Shot 级抽帧 QA | 渲染后按 shot 聚合剧本匹配、参考覆盖、动作节拍、文字安全区和画面复杂度指标，输出返修决策 |
| 手动外部生成兜底 | 没有可用模型或未启用即梦时，系统仍会展示可复制提示词和参考图信息 |

## 创作流程

```mermaid
flowchart LR
  A["一句话视频需求"] --> B["云端 Agent 编排"]
  B --> C["方案 / 脚本 / 分镜"]
  C --> D["人工审核"]
  D --> E["AIGC 素材或手动上传"]
  E --> F["本地预览项目"]
  F --> G["确认后渲染成片"]
  G --> H["抽帧 QA / Contact Sheet"]
  H --> I["发布文案与交付包"]
```

## 产品架构

<table>
  <tr>
    <td width="25%"><strong>桌面客户端</strong><br/>React + Electron，给真实用户操作项目、审核、追踪和导出。</td>
    <td width="25%"><strong>本地 Agent</strong><br/>管理本机目录、模型配置、本地 artifacts、MCP provider 和 runner。</td>
    <td width="25%"><strong>云端 Backend</strong><br/>负责任务编排、DAG、审核门、工具 manifest、持久化和 API。</td>
    <td width="25%"><strong>工具扩展</strong><br/>HyperFrames、FFmpeg、JiMeng MCP 等工具通过明确命令边界接入。</td>
  </tr>
</table>

## 适合谁

- 想把短视频生产流程标准化的个人创作者和小团队
- 需要“AI 生成 + 人工确认 + 本地交付”的视频工作流
- 希望保留本地文件控制权，不把所有素材、密钥和中间产物交给云端的团队
- 正在验证 AIGC 影视化、口播视频、自动分镜和多 Agent 编排的产品原型

## 快速开始

> 当前版本适合作为内测版、工程演示版和私有部署原型。真实商用前建议先用自己的模型账号、即梦账号和本地 runner 跑完整链路。

```bash
# 本地 Agent
bash scripts/start-local-backend.sh

# 桌面前端
bash scripts/start-frontend.sh

# 云端 Backend
bash scripts/start-cloud-backend.sh
```

打开桌面端后，进入“躺营导演台”，输入视频主题，选择“口播知识视频”或“影视/AIGC shot 视频”入口即可开始。

## MCP 扩展

本地 Agent 支持标准 MCP provider 注册。provider 可以用 Python、Node、Go 或其他语言实现，只要暴露标准 `tools/list` 与 `tools/call` 能力即可；系统只保存 provider 配置，不绑定具体实现语言。

即梦 JiMeng 扩展内置了 Python stdio MCP server，用来封装用户本机 Dreamina CLI。用户端提供显式授权的一键安装向导：

1. 安装或更新 Dreamina CLI。
2. 注册本地 JiMeng MCP provider。
3. 启动 `python3 mcp/jimeng/server.py` 标准 MCP 服务。
4. 获取即梦登录码，在即梦页面完成授权。
5. 开启“自动调用即梦生成素材”。

Dreamina OAuth、积分、任务记录和日志仍保留在用户自己的机器和即梦 CLI 目录中，云端不保存即梦凭据。

更多 provider 配置见 [MCP Provider 接入](docs/mcp-providers.md)。

<details>
<summary><strong>开发者验证命令</strong></summary>

```bash
cd local-backend && go test ./...
cd ../cloud-backend && go test ./...
cd ../frontend && npm run test:director && npm run lint && npm run build
```

API 文档：

```text
Cloud backend: http://localhost:8080/docs
Local agent:   http://localhost:18080/api/local/docs
```

</details>

## 开发与版本管理

- `develop_go` 是 Go/核心系统开发者分支，口头简称 `developgo`；常规功能开发先从该分支拉出 `feature/*`。
- `release` 是发布分支，只接收来自 `develop_go` 或 `hotfix/*` 的合入。
- 每次合入 `release` 都必须在本 README 的「Release 更新」中追加用户可读的更新内容。
- 对外发布必须创建语义化 tag，例如 `v0.1.2`；tag 指向对应 release 提交，不复用旧 tag。
- 临时素材、渲染缓存和本地测试输出不进入发布提交。

完整规范见 [版本管理 Wiki](docs/version-management.md)。

## 项目文档

- [中文 Wiki](https://github.com/SUSTechWLA/tangying-ai-operation-system/wiki)
- [English Wiki](https://github.com/SUSTechWLA/tangying-ai-operation-system/wiki/English)
- [MCP Provider 接入](docs/mcp-providers.md)
- [版本管理 Wiki](docs/version-management.md)
- [视频抽帧 QA Wiki](docs/video-frame-qa.md)
- [影视类视频创作流程](docs/cinematic-video-workflow.md)

Wiki 中包含产品介绍、系统边界、核心流程、即梦 MCP 使用方式和后续路线图。
