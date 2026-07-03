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
  <img alt="Release" src="https://img.shields.io/badge/Release-v0.1.0-111827?style=for-the-badge" />
  <img alt="Video Workflow" src="https://img.shields.io/badge/Video%20Workflow-Cloud%20Orchestration%20%2B%20Local%20Runner-5B6CFF?style=for-the-badge" />
  <img alt="Desktop Client" src="https://img.shields.io/badge/Desktop-React%20%2B%20Electron-16A085?style=for-the-badge" />
  <img alt="Backend" src="https://img.shields.io/badge/Backend-Go-2F80ED?style=for-the-badge" />
</p>

---

## Release 更新

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
| 影视化 / AIGC shot 视频 | 规划角色、场景、连续性、关键帧、外部生成请求和本地预览渲染 |
| 口播 / 知识类视频 | 生成脚本、时间窗、画面段落、提示词、预览项目和最终视频 |
| 分阶段审核 | 方案、脚本、分镜、预览、渲染等节点可确认、拒绝、编辑或重新生成 |
| 本地执行器 | 用户电脑负责本地文件、HyperFrames 项目、渲染和工具执行 |
| 即梦 JiMeng MCP 扩展 | 用户显式安装并登录 Dreamina CLI 后，可通过本地 MCP 自动生成 AIGC 素材 |
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
  G --> H["导出交付包"]
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

## 项目文档

- [中文 Wiki](https://github.com/SUSTechWLA/tangying-ai-operation-system/wiki)
- [English Wiki](https://github.com/SUSTechWLA/tangying-ai-operation-system/wiki/English)
- [MCP Provider 接入](docs/mcp-providers.md)

Wiki 中包含产品介绍、系统边界、核心流程、即梦 MCP 使用方式和后续路线图。
