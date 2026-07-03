# 版本管理 Wiki

本文是仓库内的 Wiki 源文档。GitHub Wiki 更新时应与本文保持一致。

## 分支角色

| 分支 | 定位 | 使用规则 |
|---|---|---|
| `develop_go` | 开发者分支，口头简称 `developgo` | Go/核心系统、云端编排、本地 runner、桌面端联调都先进入这里 |
| `feature/*` | 功能分支 | 从 `develop_go` 拉出，完成后合回 `develop_go` |
| `hotfix/*` | 发布修复分支 | 从 `release` 或当前 tag 拉出，只修阻断问题 |
| `release` | 发布分支 | 只接收已验证的核心代码和发布文档 |

`release` 不直接承载日常开发。临时脚本、渲染缓存、E2E 产物、测试账号 token 不进入 `release` 提交。

## 合入 release 的硬性要求

合入 `release` 前必须完成：

1. 核心链路验证通过，至少包含相关 Go 单测、前端逻辑测试和构建。
2. README 的「Release 更新」追加本次版本的用户可读更新内容。
3. Wiki 源文档同步更新。涉及架构、流程、版本规则、MCP 或 QA 的改动都要写入 `docs/`。
4. 只 stage 本次发布需要的源码和文档，不 stage `tmp/`、`promo/`、本地渲染产物或用户私有配置。
5. 合并到 `release` 后创建语义化 tag。

## Tag 规则

使用语义化版本：

| 类型 | 示例 | 场景 |
|---|---|---|
| patch | `v0.1.2` | bugfix、小功能、文档和兼容增强 |
| minor | `v0.2.0` | 新增稳定用户能力或较大工作流能力 |
| major | `v1.0.0` | 对外 API、数据结构或使用方式出现破坏性变化 |

tag 必须指向 `release` 上的发布提交。不要移动已发布 tag；如果发布内容有误，创建新的 patch 版本。

## 发布记录规范

README 的 release note 要写给用户看，不写内部流水账。每条说明应回答：

- 用户现在能做什么。
- 哪些阻断问题被修复。
- 是否影响使用方式、配置方式或兼容性。
- 是否需要重新启动 cloud backend、local agent 或 frontend。

## 当前发布检查清单

v0.1.2 对应能力：

- 动态视频计划在 render 后自动插入 `visual_qa`。
- 本地 `VIDEO_FRAME_QA` 输出 JSON 报告、抽帧图片和 contact sheet。
- QA 审核门阻断 publish，人工确认后才继续发布文案。
- 本地 artifact 的 `localPath` 可被下游本地工具解析为真实视频路径。
- 30 秒 16:9 宣传视频完整链路已跑通，QA 得分 100，run 状态 `SUCCESS`。
