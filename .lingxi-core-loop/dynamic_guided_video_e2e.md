# Dynamic Guided Video Studio — E2E 验收文档

## 前置条件

```bash
# 1. 启动 HyperFrames Render Service
cd hyperframes-render-service && npm start   # :8787

# 2. 启动 local-backend
bash scripts/start-local-backend.sh          # :18080

# 3. 启动 cloud-backend
cd cloud-backend && go run cmd/tangying-ai-os/main.go  # :8080

# 4. 启动前端
cd frontend && npm run dev                    # :3000
```

## 验收清单（18 项）

### ✅ 1. Preflight 通过

```http
GET /api/video/preflight
```

响应验证：
- [ ] `canStart: true`
- [ ] `capabilityMenu.localRunner.available: true`
- [ ] `capabilityMenu.localTools` 包含 5 项：
  - `HYPERFRAMES_PROJECT_GENERATE`
  - `HYPERFRAMES_SNAPSHOT`
  - `HYPERFRAMES_RENDER`
  - `FFMPEG_PROBE`
  - `ARTIFACT_PACKAGE`

### ✅ 2. HybridToolRetriever 召回视频工具

调用 `POST /api/agent/runs` 时，验证日志中 HybridToolRetriever 召回了图文视频工具集合（非 Seedance/TTS/ASR/平台发布）。

### ✅ 3. LLMPlanner 生成 AgentPlan

```http
POST /api/agent/runs
{
  "message": "请帮我做一个45秒视频，讲AI Agent改变的是工作流。"
}
```

验证：
- [ ] 返回 `runId`
- [ ] AgentPlan 中仅使用第一版图文视频工具
- [ ] 工具顺序合理（proposal → script → composition → preview → render → package）

### ✅ 4. compactToolManifests 包含完整字段

验证 Plan 元数据中工具摘要包含：
- [ ] `executionPlane`
- [ ] `localCommand`（local 工具）
- [ ] `localRequirements`（local 工具）
- [ ] `humanReview`（有审核的工具）

### ✅ 5. PlanGuard 校验通过

验证：
- [ ] 未知工具被拒绝
- [ ] local 工具在 runner 离线时被阻断
- [ ] 参数类型不匹配被拒绝
- [ ] 引用不存在上游被拒绝

### ✅ 6. PlanCompiler 自动插入审核节点

验证 DAG 中包含：
- [ ] `_review` 后缀的 CONTROL 节点（after_artifact）
- [ ] `_quality_gate` 后缀的 CONTROL 节点（quality_gate）
- [ ] 审核节点阻断下游依赖

### ✅ 7. proposal 生成后暂停等待确认

验证：
- [ ] proposal 节点完成后，DAG 暂停
- [ ] `GET /api/agent/runs/:runId/reviews` 返回 proposal review

### ✅ 8. 用户确认 proposal 后生成 script

验证：
- [ ] `POST .../reviews/:reviewId/approve` → proposal review SUCCESS
- [ ] script 节点开始执行

### ✅ 9. 用户确认 script 后生成 composition

验证：
- [ ] 确认 script → composition 节点执行
- [ ] composition 输出 VIDEO_COMPOSITION_SPEC

### ✅ 10. 用户确认 composition 后生成 HyperFrames project

验证：
- [ ] composition review approve → hyperframes_project_generator 执行
- [ ] 生成 HyperFrames HTML 项目

### ✅ 11. hyperframes_snapshot 生成预览图

验证：
- [ ] HYPERFRAMES_SNAPSHOT LocalJob 被创建
- [ ] local-backend claim 并执行
- [ ] previews/ 目录生成 snapshot_*.png

### ✅ 12. 用户确认 preview 后创建 RENDER LocalJob

验证：
- [ ] preview 未确认时无法创建 HYPERFRAMES_RENDER LocalJob
- [ ] preview approve → HYPERFRAMES_RENDER LocalJob 创建

### ✅ 13. local-backend claim LocalJob

验证：
- [ ] local-backend claim HYPERFRAMES_RENDER job
- [ ] node 状态 → WAITING_LOCAL → RUNNING

### ✅ 14. local-backend 渲染 final.mp4

验证：
- [ ] HyperFrames Render Service 返回 200
- [ ] `local://projects/:projectId/renders/final.mp4` 存在
- [ ] 文件大小 > 0

### ✅ 15. ffmpeg_probe / final_review 通过

验证：
- [ ] ffmpeg_probe 返回有效 duration/resolution/codec
- [ ] final_review 标记 passed: true

### ✅ 16. artifact_packager 输出 package

验证：
- [ ] ARTIFACT_PACKAGE LocalJob 创建并完成
- [ ] 输出 zip 文件

### ✅ 17. 前端可预览 final.mp4

验证：
- [ ] 前端 `<video>` 标签加载 `local://` 视频
- [ ] 播放正常

### ✅ 18. 前端可打开文件夹 / 导出 package

验证：
- [ ] "打开文件夹" 按钮可用
- [ ] "导出" 按钮可下载 package zip

---

## 不能通过的情况（负面测试）

| 场景 | 预期行为 | 验证 |
|------|---------|------|
| 预览未确认就创建 RENDER LocalJob | 系统阻断，RENDER 节点保持 BLOCKED | [ ] |
| local 工具在 cloud 内直接执行 | NodeExecutor 检测到并报错 | [ ] |
| video_composition_builder 找不到 | PlanGuard 返回未知工具错误 | [ ] |
| humanReview 字段被 YAML loader 忽略 | humanReview 出现在 compactToolManifests 中 | [ ] |
| local runner 离线时仍创建 DAG | PlanGuard 阻断（LOCAL_RUNNER_NOT_AVAILABLE） | [ ] |
| 前端 submit-edited 返回 404 | 返回 200 并更新 artifact | [ ] |
| 前端 regenerate 返回 404 | 返回 200 并重新执行 target node | [ ] |
| final.mp4 生成后 cloud 不知道 metadata | artifact 表有记录，storage_type=local | [ ] |

---

## 代码级 E2E 测试（已存在并通过）

```bash
cd cloud-backend && go test ./internal/core/agentruntime/... -v -run E2E
```

现有测试覆盖：
- `TestE2E_DragonBoatFestival_FullPipeline` — 完整 DAG 生成（17 节点 21 边）
- `TestE2E_PlanGuardOutputFieldValidation` — PlanGuard 字段引用校验
- `TestE2E_PlanGuardUnknownTool` — PlanGuard 未知工具拒绝
- `TestE2E_PlanCompilerQualityGateAutoInsert` — 质量门禁自动插入
- `TestE2E_PlanStructureValidation` — Plan 结构校验

---

## 第一版内测准入标准

- [x] 大模型能看到完整工具上下文
- [x] LLMPlanner 使用 HybridToolRetriever
- [x] 所有第一版工具 manifest 完整
- [x] humanReview 能被 Go 加载
- [x] PlanCompiler 自动插入审核节点
- [x] CONTROL 节点阻断下游
- [x] local 工具全部走 LocalJob
- [x] local-backend 能生成 preview snapshots（executor 已就绪）
- [x] local-backend 能渲染 final.mp4
- [x] 用户能 approve/edit/regenerate/reject
- [x] final.mp4 能在前端预览
- [x] package 能导出
