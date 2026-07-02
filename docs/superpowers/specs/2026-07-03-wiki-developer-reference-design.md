# Wiki Developer Reference Design

## Goal

把 GitHub Wiki 从产品介绍扩展为“产品首页 + 开发者参考入口 + 核心模块拆解页”，让新开发者能快速了解项目边界、启动方式、主要模块和常见改动入口。

## Scope

本次只更新 GitHub Wiki 和 develop_go 中的设计/执行记录，不改核心代码，不改 release 分支。

## Recommended Structure

主 Wiki 首页继续面向非技术用户和关注者，保留产品价值、截图、创作流程和项目状态。新增开发者入口页 `Developer-Guide`，并从首页和侧边栏链接过去。

开发者入口页链接 6 个核心模块页：

1. `Frontend-Desktop`：React + Electron 桌面端、导演台页面、本地设置、即梦安装向导和 API service 边界。
2. `Cloud-Backend-Agent-Runtime`：Go 云端主服务、认证、Agent Runtime、PlanCompiler、审核门、local runner dispatcher。
3. `Local-Backend-Runner-MCP`：本地 agent、runner loop、本地工具注册、artifact 文件、MCP provider 配置。
4. `Video-Creation-Workflows`：口播/知识类和影视/AIGC shot 两条创作链路、profile、preflight、workflow/skill 文件。
5. `JiMeng-MCP-Integration`：Dreamina CLI adapter、jimeng-mcp 服务、local MCP client、云端 `LOCAL_MCP_TOOL_CALL` 调用链。
6. `Data-Artifacts-Review-Gates`：VideoProject、Artifact、Review、Trace、external generation result 的数据和状态流。

## Content Rules

- 中文为主，保留英文技术名和准确代码路径。
- 每页必须包含：模块职责、主要代码路径、关键数据/调用流、开发者常见改动入口、验证命令。
- 每页都要有返回 `Developer-Guide` 和 `Home` 的链接。
- 页面内容解释真实代码，不写未来功能承诺。
- 不上传新的非必要截图；复用现有 Wiki 截图和 develop_go 文档链接。

## Link Strategy

- `Home.md` 增加“开发者快速入口”小节，链接 `Developer-Guide` 和 6 个模块页。
- `_Sidebar.md` 增加 “Developer Reference” 分组。
- `English.md` 增加一行指向中文开发者参考页，说明当前开发者参考以中文为主。

## Acceptance Criteria

- Wiki 新增 7 个页面并从首页/侧边栏可达。
- 首页仍保持产品介绍，不被大量技术细节淹没。
- 新开发者能从 `Developer-Guide` 找到启动、测试、模块代码路径和改动入口。
- Wiki 仓库推送到远端 master。
- develop_go 保留本次设计和执行计划，release 不变。
