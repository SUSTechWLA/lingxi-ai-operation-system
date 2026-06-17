# AIOS 视频创作升级——测试与验收规范

## 1. 测试目标

保证升级具备：

- 业务正确性
- DAG/事件幂等
- 故障恢复
- 局部重跑隔离
- Provider 合同稳定
- Electron 本地执行安全
- 旧功能兼容
- 不消耗真实 API 的可重复 CI

## 2. 测试金字塔

### L1 单元测试

覆盖：

- Skill manifest/parser/validator。
- Workflow compiler。
- Stage 状态映射。
- Rerun 影响范围。
- Artifact 版本事务。
- Model fingerprint/error mapping/retry。
- Shot BoundaryState 校验。
- Visual Component DSL 校验。
- Local command payload 校验。
- 前端 Store 和关键组件。

### L2 Repository 集成

使用测试 PostgreSQL/Redis/MinIO：

- 项目 CRUD。
- Artifact current/version。
- WorkflowRun/StageRun。
- 并发更新。
- Outbox。
- Runner lease。

### L3 服务集成

启动 Go Service + Fake Provider + Fake Local Runner：

- API。
- Workflow。
- Event。
- Approval。
- Rerun。
- Recovery。

### L4 E2E

浏览器/Electron：

- 新建项目。
- 完成审核。
- 查看 Shot/Beat。
- 局部重跑。
- 任务进度。
- 刷新恢复。
- 本地模拟渲染。

## 3. 测试环境

建议新增：

```text
deploy/docker-compose.test.yml
scripts/test-all.sh
scripts/test-backend.sh
scripts/test-frontend.sh
scripts/test-e2e.sh
test/fixtures/
```

环境变量：

```env
MODEL_PROVIDER_MODE=fake
VIDEO_CREATION_ENABLED=true
LOCAL_RUNNER_ENABLED=true
FAKE_PROVIDER_LATENCY_MS=10
```

## 4. 后端门禁

基础：

```bash
cd aios-core
go test ./...
go test -race ./...
go vet ./...
```

建议增加：

```bash
staticcheck ./...
golangci-lint run
```

如果仓库尚未引入对应工具，不得把工具缺失误报为业务失败；应在 CI 镜像中固定版本。

覆盖率建议：

- 新增 Core 包：行覆盖率 ≥ 80%。
- 新增视频领域 Service：≥ 75%。
- Handler 不强制高覆盖，但核心错误分支必须测试。

## 5. 前端门禁

建议添加：

```bash
cd frontend
npm ci
npm run typecheck
npm run lint
npm run test -- --run
npm run build
```

测试工具建议：

- Vitest
- React Testing Library
- MSW
- Playwright

## 6. Electron 门禁

- preload 暴露 API 快照测试。
- command type 白名单。
- 路径逃逸。
- 超时取消。
- Runner heartbeat。
- Playwright Electron 启动 smoke。

## 7. 核心测试矩阵

| ID | 场景 | 期望 |
|---|---|---|
| T-SKILL-01 | 加载两个合法 Skill | HEALTHY |
| T-SKILL-02 | 缺少 stage 文件 | 对应 Skill UNHEALTHY，Core 正常 |
| T-SKILL-03 | 重复 name/version | 启动报告冲突，不静默覆盖 |
| T-WF-01 | 创建 WorkflowRun | 关联 Task 和 trace |
| T-WF-02 | Approval approve 两次 | 第二次幂等 |
| T-WF-03 | Approval reject | 下游不执行 |
| T-WF-04 | 服务重启 | Run 可恢复 |
| T-WF-05 | 重复 Kafka result | 节点/Artifact 不重复 |
| T-RERUN-01 | Shot_02 重跑 | Shot_01/03 不变 |
| T-RERUN-02 | 新重跑失败 | 旧 current Artifact 仍有效 |
| T-ART-01 | 两版本并发写 | 只有一个 current |
| T-MODEL-01 | Fake success | Artifact + usage |
| T-MODEL-02 | 429 + Retry-After | 限次重试后成功 |
| T-MODEL-03 | 401 | 不重试，映射 AUTH_FAILED |
| T-MODEL-04 | schema invalid | Stage 失败，不登记成功 Artifact |
| T-MODEL-05 | 相同 fingerprint | 按策略复用 |
| T-AIGC-01 | 3 Shot 全流程 | 完成 |
| T-AIGC-02 | 手动导入视频 | 关联正确 Shot |
| T-AIGC-03 | 帧审核失败 | Review issue 结构化 |
| T-VOICE-01 | 3 Beat 全流程 | Bundle 生成 |
| T-VOICE-02 | 未知组件 | Schema 拒绝 |
| T-VOICE-03 | 单 Beat 修改 | 只更新受影响 Artifact |
| T-LOCAL-01 | Runner claim | 租约正确 |
| T-LOCAL-02 | Runner 离线 | Job 可重新分配/等待 |
| T-LOCAL-03 | `../` 路径 | 拒绝 |
| T-LOCAL-04 | 任意 shell payload | 拒绝 |
| T-UI-01 | 页面刷新 | 状态恢复 |
| T-UI-02 | SSE 断开 | 轮询降级 |
| T-REG-01 | 旧 publish API | 通过 |
| T-REG-02 | 旧 skill dialog | 通过 |
| T-REG-03 | 旧 media/tool API | 通过 |

## 8. 工作流模拟测试

### 8.1 AIGC fixture

输入：

```json
{
  "idea": "一只猫用叫声传输二进制消息",
  "durationSec": 30,
  "aspectRatio": "16:9",
  "shotCount": 3
}
```

Fake Provider 输出：

- 1 个 Script。
- 2 个 Character/Prop。
- 3 个 Shot。
- 每 Shot 1 个 Storyboard 和 2 个 Keyframe。
- 3 个轻量 MP4 或占位 Artifact。
- SHOT_02 可通过参数注入审核失败。

断言：

- 产物数量。
- 版本。
- 顺序。
- 边界状态。
- 成本记录。
- 重跑隔离。

### 8.2 Voice fixture

输入：

```json
{
  "opinion": "AI替代的不是岗位，而是整套工作流程",
  "durationSec": 45
}
```

输出：

- 3 NarrationBeat。
- 3 VisualBeat。
- 其中 1 个 generated_image。
- 1 个 HyperGen Bundle。
- 1 个 Fake Render MP4。

## 9. 故障注入

Fake Provider 请求参数：

```json
{
  "_test": {
    "failMode": "rate_limit|timeout|server_error|invalid_schema",
    "failCount": 1
  }
}
```

必须测试：

- 429 一次后成功。
- 连续 500 超过重试后失败。
- timeout 被 context cancel。
- 回调重复。
- MinIO 上传失败。
- DB 成功但事件总线不可用，由 Outbox 后续补发。
- Electron 完成回调重复。
- 后端重启扫描卡住 RUNNING。

## 10. 数据库测试

- Schema 可重复执行。
- migration 向前。
- 新功能关闭时旧表不影响启动。
- FK 和索引。
- 分页。
- soft delete。
- 并发 current artifact。

建议索引：

```sql
CREATE INDEX ON video_projects(user_id, updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX ON workflow_runs(project_id, created_at DESC);
CREATE INDEX ON workflow_stage_runs(workflow_run_id, stage_id, status);
CREATE INDEX ON creation_units(project_id, kind, sequence_no);
CREATE INDEX ON artifacts(project_id, unit_id, kind, is_current);
CREATE INDEX ON model_calls(project_id, created_at DESC);
CREATE INDEX ON local_jobs(status, created_at);
```

## 11. 安全测试

- API key 不出现在日志和错误响应。
- 上传伪造 MIME。
- 压缩包 Zip Slip。
- 本地路径 traversal。
- Electron XSS 不能获得 Node API。
- 超大 JSON 和超大上传限制。
- Provider webhook 伪造。
- Presigned URL 过期。
- 非法 Project/Artifact 关联。

## 12. 性能与稳定性

最低压力场景：

- 100 个项目分页。
- 单项目 100 个 Shot。
- 20 个并发 Fake 模型调用。
- 10 个 SSE 客户端。
- 50 个重复事件。
- 1 GB 文件采用流式上传，不加载进内存。

观察：

- goroutine 泄漏。
- 连接池耗尽。
- Redis 锁未释放。
- SSE 重连风暴。
- 临时文件未清理。

## 13. 回归门禁

任何 Phase 完成后都必须运行：

```bash
cd aios-core && go test -race ./...
cd frontend && npm run build
```

在 P5 以后还必须运行：

```bash
./scripts/test-video-workflows.sh
```

在 P7 以后：

```bash
./scripts/test-electron-runner.sh
```

发布前：

```bash
./scripts/test-all.sh
```

## 14. Definition of Done

一个功能只有满足以下条件才算完成：

- 需求 ID 有对应实现。
- 有自动化测试。
- 错误路径有测试。
- API/Schema 文档更新。
- 日志包含 trace。
- 不记录 secret。
- Feature flag 行为验证。
- 旧功能回归。
- Agent 更新 TASK_STATUS。
- 无未解释的 race、panic、测试跳过。
