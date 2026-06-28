# 躺营 Video Agent Beta-0.2 小范围上线优化方案

## 1. 目标

将当前系统从“自用 beta”升级为：

> **可供 1–3 个熟人小范围试用的视频创作 Agent Beta-0.2。**

本轮优化只做上线小范围 beta 必需事项，不扩展完整商业化功能。

## 2. Beta-0.2 定位

### 2.1 产品定位

躺营 Video Agent Beta-0.2 是一个面向个人创作者的视频创作导演台，支持用户从一个视频想法出发，分阶段生成创意方案、脚本、分镜、结构、预览、产物和发布素材。

### 2.2 内测承诺

Beta-0.2 必须稳定支持：

```text
输入视频想法
→ 启动 Dynamic Agent Run
→ 生成阶段产物
→ 人工审核 / 驳回 / 返工
→ 查看执行追踪
→ 查看 Artifact
→ 导出发布素材
```

### 2.3 不承诺能力

Beta-0.2 不承诺：

```text
1. 自动生成最终高质量视频。
2. 自动发布到小红书、B站、抖音等平台。
3. 自动采集运营数据。
4. 多用户协同审批。
5. 完整资产库。
6. 完整视频 Provider 接入。
7. 自动剪辑复杂视频。
8. 商业级稳定性。
```

---

## 3. 本轮优化原则

### 3.1 只修上线阻塞问题

本轮只处理：

```text
1. 业务边界一致性。
2. 核心流程可跑通。
3. 前端体验可理解。
4. 错误可诊断。
5. 测试和文档可验证。
6. 小范围内测可交付。
```

### 3.2 不做大重构

本轮不要做：

```text
1. publish → distribution 全量迁移。
2. 新增 operation 模块。
3. 大规模拆分 video 目录。
4. 引入复杂素材库。
5. 接入多个视频生成 API。
6. 引入 VideoAgent 的 Python 重工具链。
7. 重写 Agent Runtime。
8. 重写 Orchestrator。
```

### 3.3 保持当前可用链路优先

当前系统已有：

```text
Dynamic Agent Runtime
PlanGuard
PlanCompiler
DAG Orchestrator
Artifact Review
Local Runner
HyperFrames Render Service
Director Studio
Publish 兼容层
```

本轮重点是“修通、验收、打包”，不是“推倒重来”。

---

## 4. Beta-0.2 必须跑通的两个场景

## 4.1 场景一：口播 / 图文视频

### 用户输入

```text
请帮我做一个 45 秒视频，讲智能体改变的是工作流。
```

### 预期结果

系统必须能完成：

```text
1. 启动 Agent Run。
2. 生成创意方案。
3. 生成脚本。
4. 生成卡片 / Beat / 分镜结构。
5. 生成预览或预览计划。
6. 生成待审核 Artifact。
7. 用户可以审核通过、驳回或要求重新生成。
8. 用户可以查看执行追踪。
9. 用户可以查看产物列表。
10. 用户可以导出发布素材。
```

### 验收标准

```text
1. 页面不白屏。
2. 启动后能看到 Run ID。
3. 追踪页能看到节点状态。
4. 审核页能看到待审核项。
5. 产物页能看到至少 3 个 Artifact。
6. 驳回后能重新生成或给出明确错误。
7. 导出页能展示可复制内容。
```

---

## 4.2 场景二：AIGC 镜头式视频前期生产

### 用户输入

```text
我想做一个 60 秒动画短片，讲猫咪通过 0 和 1 传输“你好”的故事。
```

### 预期结果

系统必须能完成：

```text
1. 生成故事方向。
2. 生成脚本。
3. 生成 Shot List。
4. 生成角色 / 场景 / 道具设定。
5. 生成关键帧 Prompt。
6. 生成视频 Prompt。
7. 允许用户手动生成视频素材。
8. 允许用户记录或导入素材信息。
9. 生成发布标题、简介、标签或封面文案。
```

### 验收标准

```text
1. 不要求自动调用视频生成模型。
2. 不要求自动剪辑。
3. 必须能稳定生成 Prompt 包。
4. 必须能复制 Prompt。
5. 必须能回到项目继续查看历史产物。
```

---

## 5. P0：业务边界与路由一致性

## 5.1 目标

确保 Beta-0.2 只暴露视频创作相关入口，不再暴露无关业务入口。

## 5.2 检查项

执行：

```bash
grep -R "internal/agents/bid\|internal/agents/chat\|/api/bid\|/api/chat\|标书\|投标\|通用对话\|AI 对话助手" . -n
```

## 5.3 处理规则

### 必须删除或停用

```text
1. /api/bid/*
2. /api/chat/*
3. 标书入口
4. 通用 Chat 页面
5. 通用 Chat 菜单
6. README / ARCHITECTURE / AGENTS / CLAUDE 中的旧业务描述
```

### 可以保留

```text
1. internal/core/agentruntime
2. internal/core/orchestrator
3. internal/core/workflow
4. internal/core/skillruntime
5. internal/core/modelgateway
6. internal/core/artifact
7. internal/core/localrunner
8. internal/agents/video
9. internal/agents/publish
```

## 5.4 验收标准

```text
1. 前端导航不出现 bid / 标书 / 通用聊天。
2. OpenAPI 不暴露 /api/bid/*。
3. OpenAPI 不暴露 /api/chat/*。
4. 架构文档只描述 video + publish 兼容层。
5. 项目仍能编译和启动。
```

---

## 6. P0：OpenAPI 与实际路由一致

## 6.1 目标

保证前端、文档、后端路由一致，避免小范围内测时出现“文档有但接口无”或“接口有但文档无”。

## 6.2 必须执行

```bash
cd cloud-backend
make gen-docs
make api-docs-check
go test ./...
```

## 6.3 验收标准

API 文档必须包含：

```text
Auth:
- POST /api/auth/register
- POST /api/auth/login
- GET /api/auth/me

Dynamic Agent:
- POST /api/agent/runs
- GET /api/agent/runs/:id
- GET /api/agent/runs/:id/trace
- GET /api/agent/runs/:id/reviews
- POST /api/agent/runs/:id/reviews/:rid/approve
- POST /api/agent/runs/:id/reviews/:rid/reject

Video Projects:
- GET /api/video-projects
- POST /api/video-projects
- GET /api/video-projects/:id
- GET /api/video-projects/:id/session
- PATCH /api/video-projects/:id
- DELETE /api/video-projects/:id

Artifacts:
- GET /api/video-projects/:id/artifacts
- GET /api/artifacts/:id
- GET /api/artifacts/:id/content
- GET /api/artifacts/:id/history
- POST /api/artifacts/:id/revise

Local Runner:
- POST /api/local-runners/register
- POST /api/local-runners/:runnerId/heartbeat
- GET /api/local-runners/:runnerId/jobs/claim
- POST /api/local-jobs/:jobId/progress
- POST /api/local-jobs/:jobId/complete
- POST /api/local-jobs/:jobId/fail

Publish compatibility:
- POST /api/publish
- POST /api/ai/generate
- POST /api/ai/generate-from-media
- POST /api/ai/polish
```

API 文档不得包含：

```text
/api/bid/*
/api/chat/*
```

---

## 7. P0：导演工作台可用性修复

## 7.1 目标

让 Beta 用户进入系统后知道怎么开始、怎么看结果、怎么处理错误。

## 7.2 必须优化的前端点

### 7.2.1 项目启动区

当前首页启动区需要明确告诉用户：

```text
1. 输入视频想法。
2. 选择或填写视频时长。
3. 点击启动。
4. 系统会分阶段生成产物。
5. 每个关键阶段需要用户确认。
```

### 7.2.2 审核页

审核页必须展示：

```text
1. 当前待审核阶段。
2. 当前 Artifact 内容。
3. 通过按钮。
4. 驳回按钮。
5. 修改意见输入框。
6. 重新生成按钮。
```

### 7.2.3 追踪页

追踪页必须展示：

```text
1. 节点名称。
2. 节点状态。
3. 开始时间。
4. 错误信息。
5. 关联 Artifact。
```

### 7.2.4 产物页

产物页必须展示：

```text
1. Artifact 名称。
2. Artifact 类型。
3. Artifact 状态。
4. Artifact 内容预览。
5. 复制按钮。
6. 历史版本入口。
```

### 7.2.5 导出页

导出页第一版只需要支持：

```text
1. 复制脚本。
2. 复制分镜。
3. 复制 Prompt。
4. 复制发布文案。
5. 导出 Markdown。
6. 导出 JSON。
```

暂不要求打包 zip。

## 7.3 验收标准

```text
1. 新用户进入导演台后能在 30 秒内知道如何启动。
2. 运行中能看到当前阶段。
3. 失败时能看到错误原因。
4. 审核时能看到产物内容。
5. 产物可以复制。
6. 发布素材可以导出。
```

---

## 8. P0：错误提示与诊断包

## 8.1 目标

小范围 beta 最怕“失败但不知道为什么”。本轮必须把错误变得可诊断。

## 8.2 后端错误格式

Agent Run / Workflow / Artifact 相关错误统一返回：

```json
{
  "code": "NODE_EXECUTION_FAILED",
  "message": "脚本生成失败",
  "detail": {
    "runId": "xxx",
    "nodeId": "xxx",
    "stage": "script",
    "tool": "video_script_generator",
    "artifactKind": "VIDEO_SCRIPT",
    "rawMessage": "provider timeout"
  }
}
```

## 8.3 前端展示

错误面板展示：

```text
1. 用户可读错误。
2. 错误码。
3. 节点 ID。
4. 阶段。
5. 工具名。
6. 原始错误。
7. 建议操作。
```

建议操作只做三类：

```text
1. 重试当前阶段。
2. 修改输入后重新生成。
3. 检查本地服务 / 模型配置。
```

## 8.4 本地诊断包

Beta-0.2 至少支持：

```text
1. local-backend health。
2. cloud API base。
3. 当前用户信息。
4. 最近一次 runId。
5. 最近错误日志。
6. 本地 Runner 状态。
```

---

## 9. P0：Beta 固定测试用例

## 9.1 目标

不再靠手动感觉判断系统可用，建立最小 beta 用例集。

## 9.2 新增目录

```text
docs/beta-test-cases/
├── case-001-voice-workflow.md
├── case-002-aigc-shot-workflow.md
├── case-003-review-reject-regenerate.md
├── case-004-local-runner-preflight.md
├── case-005-publish-copy-export.md
└── beta-test-report-template.md
```

## 9.3 Case 001：口播视频

```text
输入：
请帮我做一个45秒视频，讲智能体改变的是工作流。

检查：
1. 能启动 run。
2. 能看到 review。
3. 能生成脚本类 Artifact。
4. 能生成结构类 Artifact。
5. 能生成发布素材。
6. 能复制内容。
```

## 9.4 Case 002：镜头式视频

```text
输入：
我想做一个60秒动画短片，讲猫咪通过0和1传输“你好”的故事。

检查：
1. 能生成故事方向。
2. 能生成脚本。
3. 能生成 shot list。
4. 能生成 keyframe prompt。
5. 能生成 video prompt。
6. 能复制 prompt。
```

## 9.5 Case 003：审核返工

```text
操作：
驳回脚本阶段，输入：开头太平，前三秒需要更强冲突。

检查：
1. 驳回成功。
2. 旧 Artifact 保留。
3. 新 Artifact 生成。
4. 下游阶段提示需要重新确认或重新生成。
```

## 9.6 Case 004：本地 Runner

```text
检查：
1. local-backend health 正常。
2. video preflight 能返回。
3. 本地 Runner 状态能展示。
4. 本地服务异常时前端有提示。
```

## 9.7 Case 005：发布素材导出

```text
检查：
1. 能生成标题。
2. 能生成简介。
3. 能生成标签。
4. 能生成封面文案。
5. 能复制。
6. 能导出 Markdown 或 JSON。
```

---

## 10. P0：Beta README 与使用说明

## 10.1 新增文档

```text
docs/BETA_USAGE.md
```

## 10.2 内容结构

```text
# 躺营 Video Agent Beta 使用说明

1. Beta 定位
2. 当前支持能力
3. 当前不支持能力
4. 本地启动方式
5. 云端启动方式
6. 第一次使用流程
7. 推荐测试用例
8. 常见错误处理
9. 如何提交反馈
10. 已知限制
```

## 10.3 必须写清楚的限制

```text
1. 当前不是商业正式版。
2. 当前不自动发布。
3. 当前不保证自动生成最终视频。
4. 当前发布数据复盘为后续能力。
5. 当前适合个人创作者自用和小范围技术内测。
```

---

## 11. P1：视频项目内助手最小实现

## 11.1 目标

用 video-scoped assistant 替代通用 chat，但本轮只做最小可用。

## 11.2 新增接口

```text
POST /api/video-projects/:id/assistant/message
POST /api/video-projects/:id/assistant/revise
POST /api/video-projects/:id/assistant/explain-stage
```

## 11.3 能力范围

### message

用于围绕当前项目提问：

```text
当前项目做到哪一步了？
下一步我应该审核什么？
这个脚本有什么问题？
```

### revise

用于基于当前 Artifact 返工：

```text
把开头改得更有冲突。
把分镜控制在 8 个以内。
让发布文案更适合小红书。
```

### explain-stage

用于解释阶段：

```text
为什么需要先审核预览再渲染？
为什么这个产物被标记 stale？
```

## 11.4 禁止

```text
1. 不恢复 /api/chat/*。
2. 不做全局聊天页。
3. 不让 assistant 绕过 Artifact。
4. 不让 assistant 直接自动发布。
```

---

## 12. P1：发布素材导出补强

## 12.1 目标

即使不自动发布，也要让用户拿到可复制的发布素材。

## 12.2 发布素材结构

```json
{
  "platform": "xiaohongshu",
  "title": "标题",
  "description": "正文",
  "tags": ["标签1", "标签2"],
  "coverText": "封面文案",
  "publishTips": ["发布建议1", "发布建议2"]
}
```

## 12.3 第一版支持平台

```text
1. 小红书
2. B站
```

## 12.4 导出格式

```text
1. Markdown
2. JSON
```

## 12.5 验收

```text
1. 导出页能展示小红书版本。
2. 导出页能展示 B站版本。
3. 每个字段有复制按钮。
4. 能导出 Markdown。
5. 能导出 JSON。
```

---

## 13. P1：本地服务预检优化

## 13.1 目标

用户启动任务前，先知道本地环境是否可用。

## 13.2 Preflight 检查项

```text
1. cloud-backend 是否可访问。
2. local-backend 是否在线。
3. HyperFrames 服务是否在线。
4. 本地项目目录是否可写。
5. 模型 Provider 是否配置。
6. 当前 workflow 能否启动。
```

## 13.3 前端展示

```text
绿色：可启动
黄色：部分能力不可用，但可生成脚本 / Prompt
红色：无法启动，需要修复配置
```

## 13.4 Beta 策略

如果 HyperFrames 不在线，不阻塞脚本 / Prompt 生成。

如果 local-backend 不在线，不阻塞纯文本创作，但提示无法执行本地渲染。

如果模型 Provider 不可用，禁止启动真实生成，但允许 fake mode 测试。

---

## 14. P2：非阻塞优化项

以下内容不阻塞 Beta-0.2，可以排到后续。

```text
1. SSE 替代轮询。
2. 完整 distribution 模块。
3. operation 运营复盘。
4. 自动发布。
5. 视频 Provider 接入。
6. 自动剪辑。
7. 完整资产库。
8. 多用户协作。
9. 素材语义检索。
10. 长视频理解。
```

---

## 15. Beta-0.2 验收清单

## 15.1 工程验收

必须通过：

```bash
cd cloud-backend && go test ./...
cd cloud-backend && make api-docs-check
cd local-backend && go test ./...
cd frontend && npm run build
```

## 15.2 路由验收

必须满足：

```text
/api/bid/* 不存在
/api/chat/* 不存在
/api/agent/runs 可用
/api/video-projects 可用
/api/artifacts 可用
/api/local-runners 可用
/api/publish 可用
/api/ai/* 可用
```

## 15.3 产品验收

必须满足：

```text
1. 能登录。
2. 能进入导演工作台。
3. 能启动一次视频创作 run。
4. 能看到阶段状态。
5. 能看到待审核项。
6. 能审核通过。
7. 能驳回返工。
8. 能查看 Artifact。
9. 能复制产物。
10. 能导出发布素材。
```

## 15.4 稳定性验收

至少完成：

```text
1. 口播视频用例跑通 3 次。
2. 镜头式视频用例跑通 2 次。
3. 审核驳回用例跑通 2 次。
4. 本地服务异常时前端能提示。
5. 模型调用失败时前端能提示。
```

---

## 16. 小范围 Beta 发布方式

## 16.1 内测对象

仅限：

```text
1. 开发者本人。
2. 1–3 个熟人创作者。
3. 能接受手动配置和反馈问题的人。
```

## 16.2 不推荐对象

暂不面向：

```text
1. 完全小白用户。
2. 商业客户。
3. 大规模公开用户。
4. 对自动发布有强需求的人。
```

## 16.3 反馈收集模板

```text
1. 你输入的视频想法是什么？
2. 系统是否成功生成内容？
3. 哪一步卡住？
4. 生成结果是否可用？
5. 哪个页面最难理解？
6. 错误提示是否看得懂？
7. 你最终是否能拿到可发布素材？
8. 你最希望下一版补什么？
```

---

## 17. 本轮推荐提交拆分

### Commit 1：清理 beta 边界

```text
refactor: align beta routes with video-agent scope
```

内容：

```text
1. 停用 bid/chat 路由。
2. 清理前端无关入口。
3. 清理文档旧表述。
```

### Commit 2：修复 OpenAPI

```text
docs: regenerate beta API reference
```

内容：

```text
1. 更新 cloud_spec.go。
2. 生成 API_REFERENCE.md。
3. 更新前端 api types。
```

### Commit 3：导演台 beta 体验优化

```text
feat: improve director studio beta usability
```

内容：

```text
1. 启动区说明。
2. 审核页产物展示。
3. 追踪页错误展示。
4. 产物复制。
5. 导出页发布素材。
```

### Commit 4：错误诊断优化

```text
feat: add structured beta error diagnostics
```

内容：

```text
1. 统一错误结构。
2. 前端错误详情。
3. 本地服务异常提示。
```

### Commit 5：beta 用例和说明文档

```text
docs: add beta usage and test cases
```

内容：

```text
1. docs/BETA_USAGE.md。
2. docs/beta-test-cases/*。
3. beta-test-report-template.md。
```

---

## 18. 给编码 Agent 的执行提示词

```text
请基于当前 tangying-ai-operation-system 项目，把系统升级为可小范围上线的 Beta-0.2。

目标：
不是扩展完整商业功能，而是让 1–3 个熟人能够小范围试用“视频想法 → Agent Run → 阶段产物 → 审核返工 → Artifact 查看 → 发布素材导出”的最小闭环。

必须完成：
1. 清理业务边界：
   - 不再暴露 /api/bid/*
   - 不再暴露 /api/chat/*
   - 前端不出现标书和通用聊天入口
   - 文档只描述 video + publish 兼容层

2. 保持当前核心链路：
   - 保留 Dynamic Agent Runtime
   - 保留 PlanGuard / PlanCompiler
   - 保留 Orchestrator
   - 保留 Artifact Review
   - 保留 Local Runner
   - 保留 HyperFrames Render Service
   - 保留 publish 兼容接口

3. 修复 API 一致性：
   - 更新 cloud-backend/internal/core/apispec/cloud_spec.go
   - 运行 make gen-docs
   - 运行 make api-docs-check
   - 确认 API 文档不暴露 bid/chat

4. 优化导演工作台：
   - 启动区说明更清楚
   - 审核页展示 Artifact 内容
   - 追踪页展示节点状态和错误详情
   - 产物页支持复制
   - 导出页支持发布素材复制和导出 Markdown / JSON

5. 优化错误诊断：
   - Agent Run / Node / Artifact 错误返回结构化字段
   - 前端展示错误码、节点、阶段、工具、原始错误和建议操作
   - 本地服务不可用时给出明确提示

6. 增加 beta 测试用例：
   - docs/beta-test-cases/case-001-voice-workflow.md
   - docs/beta-test-cases/case-002-aigc-shot-workflow.md
   - docs/beta-test-cases/case-003-review-reject-regenerate.md
   - docs/beta-test-cases/case-004-local-runner-preflight.md
   - docs/beta-test-cases/case-005-publish-copy-export.md

7. 增加 beta 使用说明：
   - docs/BETA_USAGE.md
   - 写清楚支持能力、不支持能力、启动方式、测试用例、常见错误、反馈方式

禁止：
1. 不要新增完整 operation 模块。
2. 不要重构 publish 为 distribution。
3. 不要接入多个视频生成 Provider。
4. 不要恢复通用 chat。
5. 不要恢复 bid。
6. 不要重写 Agent Runtime。
7. 不要引入复杂 Python 视频工具链。
8. 不要做自动发布。
9. 不要做完整资产库。
10. 不要做多用户协作。

最终验收：
1. cd cloud-backend && go test ./...
2. cd cloud-backend && make api-docs-check
3. cd local-backend && go test ./...
4. cd frontend && npm run build
5. 口播视频用例跑通。
6. 镜头式视频用例至少生成到 Prompt。
7. 审核驳回返工可用。
8. Artifact 可查看和复制。
9. 发布素材可复制和导出。
10. 系统不暴露 /api/bid/* 和 /api/chat/*。
```

---

## 19. Beta-0.2 发布判定

完成以上任务后，可以发布：

```text
躺营 Video Agent Beta-0.2
```

发布范围：

```text
个人自用 + 1–3 个熟人创作者技术内测。
```

一句话说明：

```text
躺营 Video Agent Beta-0.2 是一个面向个人创作者的视频创作导演台，支持从视频想法出发，分阶段生成创意方案、脚本、分镜、Prompt 和发布素材，并通过审核、追踪和 Artifact 管理创作过程。
```