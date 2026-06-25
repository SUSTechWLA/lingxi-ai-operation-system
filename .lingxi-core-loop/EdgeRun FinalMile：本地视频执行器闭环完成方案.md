# EdgeRun FinalMile：本地视频执行器闭环完成方案

## 1. 升级主题

本次升级命名为：

**EdgeRun FinalMile**

主题表达：

```text
补齐本地执行最后一公里：
从 LocalJob 可创建、可领取
升级为
本地工具可执行、可恢复、可验收、可生成 final.mp4
```

当前系统已经具备：

```text
1. cloud-backend 可以根据 executionPlane=local 创建 LocalJob。
2. local-backend 已有 localrunner loop。
3. local-backend 已有 localtool registry。
4. cloud/local 命令白名单基本对齐。
5. hyperframes_project_generator 已 local 化。
6. HYPERFRAMES_PROJECT_GENERATE 已有最小本地 executor。
```

但还缺：

```text
1. HYPERFRAMES_RENDER executor
2. FFMPEG_PROBE executor
3. ARTIFACT_PACKAGE executor
4. PathGuard
5. pending_report 断网补报
6. PlanGuard 本地能力校验
7. LLMPlanner / ToolRetriever 本地能力感知
8. VideoCompositionSpec 正式项目生成
9. 端到端 E2E 验收
```

---

## 2. 总体目标链路

最终目标链路：

```text
用户一句话生成视频
  ↓
cloud：LLMPlanner 生成 AgentPlan
  ↓
cloud：PlanGuard 校验工具和本地能力
  ↓
cloud：PlanCompiler 编译 DAG
  ↓
cloud：NodeExecutor 遇到 local 工具，创建 LocalJob
  ↓
local：localrunner claim LocalJob
  ↓
local：localtool executor 执行命令
  ↓
local：生成本地 artifact
  ↓
local：complete / fail / progress 上报云端
  ↓
cloud：更新 ai_node，推进 DAG
  ↓
frontend：展示本地 final.mp4
```

第一阶段必须跑通：

```text
HYPERFRAMES_PROJECT_GENERATE
  ↓
HYPERFRAMES_RENDER
  ↓
ARTIFACT_PACKAGE
```

第二阶段再扩展：

```text
FFMPEG_PROBE
AUDIO_EXTRACT
FFMPEG_CLIP_EXTRACT
ASR_TRANSCRIBE
VideoCompositionSpec
```

---

## 3. P0：实现 HYPERFRAMES_RENDER 本地执行器

### 3.1 目标

让本地执行器可以真正调用本机 HyperFrames Render Service，把本地 `hyperframes/` 项目渲染成 `final.mp4`。

### 3.2 新增文件

```text
local-backend/internal/localtool/hyperframes_render.go
local-backend/internal/localtool/hyperframes_render_test.go
```

### 3.3 输入 payload

```json
{
  "projectId": "project_001",
  "projectDir": "local://projects/project_001/hyperframes",
  "entry": "index.html",
  "outputPath": "local://projects/project_001/renders/final.mp4",
  "fps": 30,
  "quality": "standard",
  "timeoutSec": 1800
}
```

### 3.4 执行流程

```text
1. 解析 projectDir。
2. 通过 PathGuard 校验 projectDir 是否在 workspace 内。
3. 检查 index.html 是否存在。
4. 解析 outputPath。
5. 创建 outputPath 父目录。
6. 检查 HyperFrames Render Service /health。
7. POST /render。
8. 等待渲染完成。
9. 校验 final.mp4 存在。
10. 校验 final.mp4 size > 0。
11. 返回 VIDEO artifact metadata。
```

### 3.5 输出结果

```json
{
  "success": true,
  "summary": "HyperFrames 渲染完成",
  "artifacts": [
    {
      "artifactId": "artifact_final_video",
      "kind": "VIDEO",
      "name": "final.mp4",
      "storageRef": "local://projects/project_001/renders/final.mp4",
      "mimeType": "video/mp4",
      "sizeBytes": 128000000,
      "localOnly": true
    }
  ],
  "metrics": {
    "renderTimeSec": 143,
    "fps": 30
  }
}
```

### 3.6 注册 executor

建议不要继续在 `main.go` 里手写所有注册逻辑。新增：

```text
local-backend/internal/localtool/bootstrap.go
```

接口：

```go
func RegisterDefaultExecutors(reg *Registry, cfg ExecutorConfig) error
```

注册：

```go
registry.Register(
    localtool.NewHyperFramesRenderExecutor(cfg.WorkspaceRoot, cfg.HyperFramesServiceURL),
    localtool.CommandHyperFramesRender,
)
```

---

## 4. P0：实现 FFMPEG_PROBE 本地执行器

### 4.1 目标

让系统能读取用户本地 MP4 的基础信息，为后续“上传素材剪辑 / 加字幕 / 抽音频”做准备。

### 4.2 新增文件

```text
local-backend/internal/localtool/ffmpeg_probe.go
local-backend/internal/localtool/ffmpeg_probe_test.go
```

### 4.3 输入 payload

```json
{
  "input": "local://projects/project_001/assets/input.mp4"
}
```

### 4.4 执行流程

```text
1. 解析 input。
2. PathGuard 校验可读。
3. 调用 ffprobe。
4. 输出 JSON。
5. 解析 duration、resolution、codec、fps、audio。
6. 返回 media metadata。
```

### 4.5 输出结果

```json
{
  "success": true,
  "media": {
    "durationSec": 61.28,
    "width": 1920,
    "height": 1080,
    "videoCodec": "h264",
    "audioCodec": "aac",
    "fps": 30,
    "bitrate": 8200000,
    "hasAudio": true
  }
}
```

### 4.6 注册 executor

```go
registry.Register(
    localtool.NewFFmpegProbeExecutor(cfg.WorkspaceRoot),
    localtool.CommandFFmpegProbe,
)
```

---

## 5. P0：实现 ARTIFACT_PACKAGE 本地执行器

### 5.1 目标

把一次视频生产的核心产物打包为可下载、可诊断、可复现的项目包。

### 5.2 新增文件

```text
local-backend/internal/localtool/artifact_package.go
local-backend/internal/localtool/artifact_package_test.go
```

### 5.3 输入 payload

```json
{
  "projectId": "project_001",
  "include": [
    "local://projects/project_001/renders/final.mp4",
    "local://projects/project_001/hyperframes/manifest.json",
    "local://projects/project_001/artifacts/render_report.json"
  ],
  "output": "local://projects/project_001/packages/project_package.zip"
}
```

### 5.4 执行流程

```text
1. 校验 include 列表。
2. 每个路径必须经过 PathGuard。
3. 不允许打包 workspace 外的文件。
4. 创建 zip。
5. 返回 PACKAGE artifact metadata。
```

### 5.5 输出结果

```json
{
  "success": true,
  "artifacts": [
    {
      "kind": "PACKAGE",
      "name": "project_package.zip",
      "storageRef": "local://projects/project_001/packages/project_package.zip",
      "mimeType": "application/zip",
      "localOnly": true
    }
  ]
}
```

---

## 6. P0：新增 PathGuard

### 6.1 目标

本地执行器必须安全。云端不能通过 LocalJob 让本地端访问任意路径。

### 6.2 新增文件

```text
local-backend/internal/localtool/path_guard.go
local-backend/internal/localtool/path_guard_test.go
```

### 6.3 支持协议

```text
local://projects/{projectId}/...
local://artifacts/...
local://cache/...
local://logs/...
```

### 6.4 禁止

```text
../
绝对路径直接透传
~/.ssh
~/.aws
系统目录
未授权用户目录
workspace 外路径
```

### 6.5 接口

```go
type PathGuard struct {
    WorkspaceRoot string
    AuthorizedRoots []string
}

func (g *PathGuard) Resolve(uri string) (string, error)
func (g *PathGuard) EnsureReadable(uri string) (string, error)
func (g *PathGuard) EnsureWritable(uri string) (string, error)
```

### 6.6 所有本地 executor 必须使用 PathGuard

```text
HYPERFRAMES_PROJECT_GENERATE
HYPERFRAMES_RENDER
FFMPEG_PROBE
FFMPEG_CLIP_EXTRACT
AUDIO_EXTRACT
ARTIFACT_PACKAGE
```

---

## 7. P0：抽离 LocalTool Bootstrap

### 7.1 当前问题

如果所有 executor 都在 `main.go` 中注册，后续会越来越乱。

### 7.2 新增文件

```text
local-backend/internal/localtool/bootstrap.go
```

### 7.3 建议结构

```go
type ExecutorConfig struct {
    WorkspaceRoot string
    HyperFramesServiceURL string
    RenderTimeoutSec int
    FFmpegPath string
}

func RegisterDefaultExecutors(reg *Registry, cfg ExecutorConfig) error {
    guard := NewPathGuard(cfg.WorkspaceRoot)

    reg.Register(NewHyperFramesProjectExecutor(cfg.WorkspaceRoot, guard), CommandHyperFramesProjectGenerate)
    reg.Register(NewHyperFramesRenderExecutor(cfg.WorkspaceRoot, cfg.HyperFramesServiceURL, guard), CommandHyperFramesRender)
    reg.Register(NewFFmpegProbeExecutor(cfg.WorkspaceRoot, cfg.FFmpegPath, guard), CommandFFmpegProbe)
    reg.Register(NewArtifactPackageExecutor(cfg.WorkspaceRoot, guard), CommandArtifactPackage)

    return nil
}
```

### 7.4 main.go 目标形态

```go
registry := localtool.NewRegistry()

err := localtool.RegisterDefaultExecutors(registry, localtool.ExecutorConfig{
    WorkspaceRoot: server.Paths().DataDir,
    HyperFramesServiceURL: cfg.HyperFrames.ServiceURL,
    RenderTimeoutSec: cfg.HyperFrames.TimeoutSec,
    FFmpegPath: cfg.Tools.FFmpegPath,
})
if err != nil {
    log.Fatal(err)
}
```

---

## 8. P1：PlanGuard 增加 LocalCapabilityValidator

### 8.1 目标

本地执行器离线或缺少依赖时，不应该生成不可执行 DAG。

### 8.2 新增接口

```go
type LocalCapabilityProvider interface {
    HasOnlineRunner(ctx context.Context, userID string) (bool, error)
    SupportsCommand(ctx context.Context, userID string, command string) (bool, error)
    SatisfiesRequirements(ctx context.Context, userID string, req *tool.LocalRequirements) (bool, []string, error)
}
```

### 8.3 PlanGuard 校验规则

```text
if manifest.ExecutionPlane == "local":
    1. 检查用户是否有 online runner
    2. 检查 runner 是否支持 localCommand
    3. 检查 runner 是否满足 localRequirements
    4. 不满足则阻断 plan
```

### 8.4 错误示例

```json
{
  "valid": false,
  "errors": [
    {
      "code": "LOCAL_RUNNER_NOT_AVAILABLE",
      "tool": "hyperframes_renderer",
      "message": "本地执行器未启动，无法执行视频渲染。请启动桌面端 local-backend。"
    }
  ]
}
```

```json
{
  "valid": false,
  "errors": [
    {
      "code": "LOCAL_CAPABILITY_MISSING",
      "tool": "ffmpeg_probe",
      "message": "本地未检测到 ffmpeg，无法分析视频素材。"
    }
  ]
}
```

---

## 9. P1：LocalRunner 安全校验强化

### 9.1 目标

现有认证中间件只是第一层。下一步必须校验 runner、device、job 的归属关系。

### 9.2 请求头

local-backend 请求 cloud 时统一携带：

```text
Authorization: Bearer <token>
X-Device-ID: <device_id>
X-Runner-ID: <runner_id>
X-Runner-Session-ID: <session_id>
```

### 9.3 cloud 校验函数

新增：

```go
func (s *Service) ValidateRunnerAccess(ctx context.Context, userID, deviceID, runnerID, sessionID string) error
```

校验：

```text
1. runner 存在
2. runner.user_id == userID
3. runner.device_id == deviceID
4. runner.session_id == sessionID
5. runner.status != revoked
```

新增：

```go
func (s *Service) ValidateJobAccess(ctx context.Context, userID, runnerID, jobID string) error
```

校验：

```text
1. job 存在
2. job.user_id == userID
3. job.runner_id 为空时，只允许 claim 阶段绑定
4. job.runner_id 不为空时，必须等于 runnerID
5. complete/fail/progress 必须来自 claim 该 job 的 runner
```

### 9.4 保护接口

必须覆盖：

```text
heartbeat
claim
progress
complete
fail
```

---

## 10. P1：pending_report 断网恢复

### 10.1 目标

本地工具执行完成后，如果网络断开或 cloud complete 失败，结果不能丢。

### 10.2 新增文件

```text
local-backend/internal/localrunner/pending_report.go
local-backend/internal/localrunner/pending_report_test.go
```

### 10.3 本地目录

```text
local-data/pending_reports/
├── local_job_001.complete.json
├── local_job_002.fail.json
└── local_job_003.progress.json
```

### 10.4 流程

```text
local job 执行完成
  ↓
先写 pending_report
  ↓
尝试上报 cloud complete/fail
  ↓
成功：删除 pending_report
  ↓
失败：保留 pending_report
  ↓
后续 heartbeat 成功后重试补报
```

### 10.5 cloud 幂等

cloud complete/fail 必须幂等：

```text
1. 同一个 jobId 重复 complete，不重复推进 DAG。
2. 如果 job 已 COMPLETED，重复 complete 返回 200。
3. 如果 output hash 不一致，返回冲突错误。
```

---

## 11. P1：LocalJob Progress 同步 ai_node

### 11.1 目标

前端不能只看到 `WAITING_LOCAL`，要看到本地执行进度。

### 11.2 cloud `ReportProgress` 同步更新

```text
local_jobs.progress
local_jobs.status
local_job_logs
ai_node.progress
ai_node.current_step
ai_node.heartbeat_at
```

### 11.3 前端展示

```text
Node：hyperframes_renderer
状态：LOCAL_RUNNING
进度：45%
当前步骤：rendering frames
日志：正在渲染第 240 / 520 帧
```

---

## 12. P1：LLMPlanner 工具摘要增加 executionPlane

### 12.1 目标

让模型知道工具是在云端、本地还是远程 API 执行。

### 12.2 `compactToolManifests` 增加字段

```json
{
  "name": "hyperframes_renderer",
  "description": "Render HyperFrames project to MP4.",
  "executionPlane": "local",
  "requiresUserDevice": true,
  "artifactLocation": "local",
  "localCommand": "HYPERFRAMES_RENDER",
  "localRequirements": {
    "commands": ["node", "ffmpeg"]
  },
  "capabilities": [
    "video_render",
    "hyperframes",
    "mp4_render"
  ]
}
```

### 12.3 价值

```text
1. 模型不会误把 local 工具当云端工具。
2. 模型能理解 HyperFrames 是本地合成。
3. 模型能理解 Seedance 是 remote_http。
4. 工具选择更稳定。
```

---

## 13. P1：LLMPlanner 接入 HybridToolRetriever

### 13.1 目标

替换纯启发式工具选择，让工具召回结合：

```text
capability
tag
keyword
executionPlane
cost
risk
runner capability
provider availability
```

### 13.2 检索流程

```text
用户需求
  ↓
domain 识别
  ↓
hard filter
  ├── local runner 是否在线
  ├── localCommand 是否可用
  ├── remote provider 是否配置
  └── risk/cost 是否允许
  ↓
capability/tag/keyword 打分
  ↓
TopK 工具交给 LLMPlanner
```

### 13.3 过滤规则

```text
executionPlane=local 且 runner offline → 不召回
localCommand 不支持 → 不召回
remote_http provider 未配置 → 不召回
high cost 且预算低 → 降权或不召回
```

---

## 14. P2：HyperFrames Project Generator 升级为 VideoCompositionSpec 生成器

### 14.1 当前问题

当前项目生成器仍偏 demo，只写标题和脚本文本。下一步要支持正式视频合成协议。

### 14.2 输入协议

```json
{
  "compositionSpec": {
    "specVersion": "aios-video-composition-v1",
    "durationSec": 60,
    "fps": 30,
    "resolution": {
      "width": 1920,
      "height": 1080
    },
    "tracks": [
      {
        "id": "video_track_01",
        "type": "video",
        "clips": []
      },
      {
        "id": "caption_track_01",
        "type": "caption",
        "items": []
      },
      {
        "id": "overlay_track_01",
        "type": "overlay",
        "items": []
      },
      {
        "id": "audio_track_01",
        "type": "audio",
        "clips": []
      }
    ]
  }
}
```

### 14.3 输出目录

```text
hyperframes/
├── index.html
├── assets/
│   ├── data.json
│   ├── style.css
│   └── media/
├── manifest.json
└── DESIGN.md
```

### 14.4 支持能力

```text
1. 字幕 caption track
2. 视频素材 video track
3. 音频 audio track
4. 文字卡片 overlay track
5. 图片 track
6. 简单转场
7. 安全区
8. 16:9 默认画幅
```

---

## 15. P2：Final Review 最终质检

### 15.1 目标

生成 final.mp4 后，必须验证产物可用。

### 15.2 新增工具

```text
final_review_generator
ffprobe_validator
black_frame_checker
artifact_completeness_checker
```

### 15.3 输出

```json
{
  "artifactKind": "final_review",
  "passed": true,
  "checks": {
    "fileExists": true,
    "fileSizeValid": true,
    "durationValid": true,
    "hasVideoTrack": true,
    "hasAudioTrack": true,
    "blackFrameDetected": false,
    "artifactComplete": true
  },
  "finalVideo": {
    "storageRef": "local://projects/project_001/renders/final.mp4",
    "durationSec": 59.8,
    "resolution": "1920x1080"
  }
}
```

---

## 16. 推荐实施阶段

## Phase 1：执行器闭环

目标：

```text
本地可以执行 HYPERFRAMES_PROJECT_GENERATE、HYPERFRAMES_RENDER、FFMPEG_PROBE、ARTIFACT_PACKAGE。
```

任务：

```text
1. 实现 HyperFramesRenderExecutor。
2. 实现 FFmpegProbeExecutor。
3. 实现 ArtifactPackageExecutor。
4. 抽离 RegisterDefaultExecutors。
5. 所有 executor 使用 PathGuard。
6. 补单元测试。
```

验收：

```text
手动创建 LocalJob，本地能 claim、execute、complete。
```

---

## Phase 2：安全和恢复

目标：

```text
本地执行安全，可断网恢复。
```

任务：

```text
1. Runner ownership 校验。
2. Job ownership 校验。
3. Runner session 校验。
4. pending_report。
5. complete/fail 幂等。
6. 本地诊断日志。
```

验收：

```text
断网后本地任务完成，网络恢复后能补报。
```

---

## Phase 3：计划阶段能力感知

目标：

```text
不生成不可执行的 DAG。
```

任务：

```text
1. PlanGuard 增加 LocalCapabilityValidator。
2. ToolRetriever 接入 local runner capability。
3. LLMPlanner 工具摘要加入 executionPlane。
4. local runner offline 时明确阻断。
```

验收：

```text
本地执行器离线时，系统提示启动本地端，而不是卡在 WAITING_LOCAL。
```

---

## Phase 4：VideoCompositionSpec 正式化

目标：

```text
HyperFrames Project Generator 支持正式视频合成。
```

任务：

```text
1. 定义 composition spec schema。
2. 支持 video/audio/caption/overlay tracks。
3. 生成 index.html、style.css、data.json、manifest.json。
4. 支持 final_review。
```

验收：

```text
能生成带字幕、卡片、音频、视频素材的 final.mp4。
```

---

## Phase 5：端到端视频 E2E

输入：

```text
请生成一个 30 秒端午节知识分享视频。
```

验收：

```text
1. cloud 生成 script。
2. cloud 生成 shot_list。
3. cloud 生成 render_strategy。
4. local 生成 hyperframes project。
5. local 渲染 final.mp4。
6. local 打包 package。
7. cloud 收到 artifact metadata。
8. ai_task SUCCESS。
9. 前端能预览 final.mp4。
```

---

## 17. Coding Agent 执行指令

```text
目标：
实施 EdgeRun FinalMile 升级。当前系统已经具备 cloud LocalJob 控制面、localrunner loop、localtool registry、HYPERFRAMES_PROJECT_GENERATE 最小 executor。下一步要补齐本地视频执行器，使 cloud-backend 部署到云端后，local-backend 可以稳定生成 final.mp4。

P0：
1. 新增 local-backend/internal/localtool/path_guard.go。
2. 所有 local executor 必须使用 PathGuard。
3. 新增 local-backend/internal/localtool/hyperframes_render.go。
4. 实现 HYPERFRAMES_RENDER executor。
5. 新增 local-backend/internal/localtool/ffmpeg_probe.go。
6. 实现 FFMPEG_PROBE executor。
7. 新增 local-backend/internal/localtool/artifact_package.go。
8. 实现 ARTIFACT_PACKAGE executor。
9. 新增 local-backend/internal/localtool/bootstrap.go。
10. 将 executor 注册从 main.go 抽到 RegisterDefaultExecutors。
11. 补充单元测试：
    - path_guard_test.go
    - hyperframes_render_test.go
    - ffmpeg_probe_test.go
    - artifact_package_test.go

P1：
1. cloud-backend localrunner 增加 ValidateRunnerAccess。
2. cloud-backend localrunner 增加 ValidateJobAccess。
3. local-backend client 请求增加 X-Runner-ID。
4. heartbeat / claim / progress / complete / fail 全部校验 runner 和 job 归属。
5. 实现 complete/fail 幂等。

P2：
1. local-backend 新增 pending_report。
2. execute 完成后先写 pending report。
3. cloud 上报成功后删除 pending report。
4. 网络恢复后自动补报。
5. 增加 pending_report_test.go。

P3：
1. PlanGuard 新增 LocalCapabilityValidator。
2. 接入 LocalCapabilityProvider。
3. executionPlane=local 时检查 online runner。
4. 检查 localCommand 是否支持。
5. 检查 localRequirements。
6. 不满足时阻断计划。

P4：
1. LLMPlanner compactToolManifests 增加 executionPlane、requiresUserDevice、artifactLocation、localCommand、localRequirements、providerCapabilities。
2. LLMPlanner 接入 HybridToolRetriever。
3. HybridToolRetriever 增加 local runner capability hard filter。

P5：
1. 将 HyperFramesProjectExecutor 从 demo HTML 升级为 VideoCompositionSpec generator。
2. 支持 video/audio/caption/overlay tracks。
3. 输出 index.html、assets/data.json、assets/style.css、manifest.json、DESIGN.md。

P6：
1. 增加 E2E 测试文档。
2. 测试 cloud 部署 + local-backend 本地运行。
3. 验证 HYPERFRAMES_PROJECT_GENERATE → HYPERFRAMES_RENDER → final.mp4。
```

---

## 18. 最终验收标准

完成本轮优化后，必须满足：

```text
1. cloud-backend 真实部署在云服务器。
2. local-backend 在用户电脑启动。
3. local-backend 注册 runner。
4. cloud 能看到 runner online。
5. cloud 创建 HYPERFRAMES_PROJECT_GENERATE LocalJob。
6. local claim 并生成 hyperframes 项目。
7. cloud 创建 HYPERFRAMES_RENDER LocalJob。
8. local claim 并生成 final.mp4。
9. local 上报 VIDEO artifact metadata。
10. cloud 推进 DAG 到 SUCCESS。
11. frontend 能显示 final.mp4。
12. local-backend 离线时，PlanGuard 阻断 local 工具。
13. 渲染完成但网络断开时，pending_report 能补报。
```

---

## 19. 最终结论

下一步不要再优先扩展新视频能力。当前最重要的是完成：

```text
EdgeRun FinalMile
```

也就是补齐本地执行最后一公里：

```text
HYPERFRAMES_RENDER
FFMPEG_PROBE
ARTIFACT_PACKAGE
PathGuard
pending_report
PlanGuard LocalCapabilityValidator
```

完成后，你的系统才真正具备：

```text
云端编排
本地执行
本地 final.mp4
云端状态同步
前端预览
断网恢复
```

一句话总结：

**EdgeRun FinalMile 完成后，AIOS 才能从“云端控制面已经成型”进入“真实可用的云端部署 + 本地视频生产系统”。**
