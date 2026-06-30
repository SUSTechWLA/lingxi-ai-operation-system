# 躺营 Video Agent 封闭内测使用说明

> 当前版本：v4.0 closed beta。本文面向内测组织者、交付同学和参与测试的创作者。

## 1. 内测定位

封闭内测只面向 1-3 位熟悉创作者，用于验证“视频想法到可手动发布材料”的完整链路。它不是公开发布版本，也不是自动发布平台。

当前支持的主流程：

```text
输入视频想法
→ 启动 Dynamic Agent Run
→ 查看脚本 / 分镜 / Prompt / 发布包等 Artifact
→ 审核、驳回、编辑或返工
→ 遇到素材依赖点时复制 Prompt 到外部网站生成素材
→ 上传外部生成结果回填依赖点
→ 检查 Trace 和 Artifacts
→ 导出小红书 / Bilibili 手动发布材料
```

核心原则：系统不要求用户在项目启动前主动上传素材，也不强制依赖图片/视频生成 API。流程会在需要具体素材的 shot 上暂停，并把依赖的参考图、Prompt、Negative Prompt、规格和上传入口告知用户。

## 2. 当前支持范围

- 口播、知识分享、图文动效类短视频策划与生产资产。
- AIGC shot 前期生产：故事、脚本、shot list、角色/场景/道具提示、关键帧 Prompt、视频 Prompt。
- Dynamic Agent Runtime：PlanGuard、PlanCompiler、Transient DAG、Quality Gate、Artifact Review。
- Director Studio：阶段视图、审核视图、Trace、Artifact、导出和素材依赖点。
- 素材依赖点：`external_generation_request` 可展示 Prompt、参考图、目标规格，并接受用户外部生成后的文件回填。
- 本地 agent 保存用户回填文件，云端登记 `storageRef`、hash、size、shot 关联和 trace。
- 小红书 / Bilibili 手动发布材料导出。
- Local Runner 和 HyperFrames 路径，前提是本地服务可用。
- 项目级 assistant、Artifact 返工、review regenerate、workflow checkpoint recovery。
- 桌面端基础模型 Provider 配置仅保存在 local agent。

## 3. 当前不支持范围

- 公开用户注册和大规模开放测试。
- `/api/chat/*` 全局聊天。
- `/api/bid/*` 标书/投标流程。
- 自动发布到平台。
- 项目开始前强制上传完整素材库。
- Electron renderer 任意命令执行。
- 自动运营复盘。
- 视频问答、长视频理解、全自动资产库管理。
- 多人审批流。
- CosyVoice、DiffSinger、ImageBind、fish-speech、seed-vc、VideoRAG 等重型 VideoAgent 依赖。

## 4. 启动方式

启动云端：

```bash
cd cloud-backend
cp .env.example .env
# 真实内测需填写 OPENAI_API_KEY 和 AUTH_TOKEN_SECRET
docker compose up -d
go build -o build/tangying-ai-os ./cmd/tangying-ai-os
./build/tangying-ai-os
```

启动本地 agent：

```bash
bash scripts/start-local-backend.sh
```

启动前端：

```bash
cd frontend
npm install
npm run dev
```

云端 compose 部署：

```bash
cd cloud-backend/deploy
cp .env.cloud.example .env.cloud
docker compose --env-file .env.cloud -f docker-compose.cloud.yml up -d --build
```

compose 栈包含 PostgreSQL、Redis、Redpanda、MinIO、backend、nginx 和 HyperFrames Render Service。

## 5. 测试人员首次使用流程

1. 登录。
2. 打开 Director Studio。
3. 检查 preflight 状态。
4. 输入视频想法、目标平台和时长。
5. 点击开始。
6. 等待阶段产物生成。
7. 对待审核 Artifact 执行 approve、reject、edit submit 或 regenerate。
8. 如果 Artifacts 中出现“素材依赖点”，打开详情，复制 Prompt、Negative Prompt、参考图和规格。
9. 到任意外部图片/视频生成网站生成素材。
10. 回到同一个素材依赖点，上传生成后的图片或视频。
11. 打开 Trace 检查节点状态和错误。
12. 打开 Export 复制发布文案或下载 Markdown/JSON 发布包。

不要要求测试人员把个人模型 Provider token 填到云端。桌面端模型设置是本地配置；云端 LLM 执行使用运维管理的服务端凭证。

## 6. 素材依赖点验收标准

一次合格的素材依赖点流程应满足：

- 页面明确显示“素材依赖点”。
- 依赖点里有可复制的 Prompt 和参考信息。
- 用户可以跳到外部网站生成素材，系统流程不会因为未配置图片/视频 API 而完全中断。
- 上传回填先调用 `POST /api/local/artifacts`，文件保存在本机。
- local agent 返回 `storageRef`、`contentHash`、`sizeBytes`。
- 前端再调用 `POST /api/video-projects/:id/external-generation-results`。
- 云端 Artifact 记录来源为 `external_manual_upload`，并关联原始 `generationRequestId` 和 `relatedShotId`。

## 7. 固定内测用例

运行 `docs/beta-test-cases/` 下的用例：

- `case-001-voice-workflow.md`
- `case-002-aigc-shot-workflow.md`
- `case-003-review-reject-regenerate.md`
- `case-004-local-runner-preflight.md`
- `case-005-publish-copy-export.md`

结果记录到 `beta-test-report-template.md`。

## 8. 邀请测试人员前必须验证

```bash
(cd cloud-backend && go test ./...)
(cd cloud-backend && go test -race ./...)
(cd cloud-backend && go run ./evals/video_beta)
(cd cloud-backend && make api-docs-check)
(cd local-backend && go test ./...)
(cd local-backend && go test -race ./...)
(cd hyperframes-render-service && npm run build)
(cd frontend && npm run lint)
(cd frontend && npm run test:director)
(cd frontend && npm run test:security)
(cd frontend && npm run build)
```

浏览器冒烟测试必须覆盖：

- 登录。
- Director Studio 启动流程。
- Preflight 可见。
- Review approve/edit/regenerate 控件。
- Trace、Artifact、Export tabs。
- 素材依赖点 Prompt 展示。
- 素材依赖点上传回填。
- Desktop 设置页没有命令执行 UI。

手动 Browser-plugin 冒烟测试先启动 mock server：

```bash
cd frontend
npm run test:director:browser-server
```

然后打开终端输出的 `appURL`，按上面的冒烟路径执行。

## 9. 常见错误

`RENDER_DEPENDENCY_MISSING`：最终渲染前缺少 preview、composition 或 Local Runner readiness。

`ARTIFACT_MANIFEST_INVALID`：本地工具返回的 artifact metadata 不完整。

`CRITICAL_ARTIFACT_SYNC_FAILED`：关键 artifact 无法写入索引。

`PACKAGE_DEPENDENCY_MISSING`：发布包生成前缺少 final video 或 quality report。

`EXTERNAL_GENERATION_PENDING_UPLOAD`：某个 shot 的素材依赖点仍在等待用户生成并上传图片或视频。

处理建议：

- 重试当前阶段。
- 编辑输入后 regenerate。
- 检查 cloud API base。
- 检查 local-backend health。
- 检查 Local Runner 和 HyperFrames 可用性。
- 遇到 `EXTERNAL_GENERATION_PENDING_UPLOAD` 时，复制依赖点里的 Prompt 到外部网站生成素材，再上传回同一个依赖点。
- 模型配置不完整时，仅在测试场景使用 fake mode。
- review recovery 失败时检查 `/api/video-projects/:id/workflow-runs/:rid/checkpoints`。

## 10. 反馈问题

收集测试反馈时至少记录：

1. 输入的视频想法是什么？
2. 系统生成的内容是否可用？
3. 卡在哪个步骤？
4. 素材依赖点是否看得懂？
5. 外部生成和上传回填是否顺畅？
6. 哪个页面最难理解？
7. 错误信息是否足够明确？
8. 是否拿到了可手动发布的材料？
9. 下一版最应该先改什么？

## 11. 版本限制

封闭内测不是商业上线。生成视频和发布材料仍需要人工检查、人工编辑和人工发布。当前版本的重点是验证主流程、素材依赖点和审核返工闭环是否可用。
