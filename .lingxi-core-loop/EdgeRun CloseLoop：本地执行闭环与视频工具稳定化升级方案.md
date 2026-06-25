# EdgeRun CloseLoop：本地执行闭环与视频工具稳定化升级方案

## 1. 升级主题

本次升级命名为：

**EdgeRun CloseLoop**

主题表达：

```text id="e9r4t5"
从“云端已能创建 LocalJob，本地已有 Runner 骨架”
升级为
“云端部署后，本地可稳定领取、执行、回传、恢复、诊断的视频工具闭环”
```

当前系统已经完成：

```text id="tvnppv"
1. cloud-backend 有 LocalRunner / LocalJob / NodeExecutor local dispatch。
2. local-backend 有 localrunner loop。
3. local-backend 有 localtool registry。
4. cloud-backend 已经接入认证中间件。
5. hyperframes_renderer 已开始标记为 local 工具。
```

但仍存在关键断点：

```text id="kzjc6u"
1. cloud 命令白名单和 local 命令白名单不一致。
2. local-backend 只注册了 HYPERFRAMES_PROJECT_GENERATE。
3. HYPERFRAMES_RENDER 还没有本地 executor。
4. FFMPEG_PROBE 等媒体工具还没有本地 executor。
5. hyperframes_project_generator 仍未彻底 local 化。
6. PlanGuard 还没有检查本地 Runner 能力。
7. 本地任务断网后的 pending_report 机制还不完整。
```

所以本次升级目标不是继续加更多视频功能，而是先完成：

```text id="v13rya"
LocalJob 可创建
LocalJob 可领取
LocalJob 可执行
LocalJob 可进度上报
LocalJob 可完成
LocalJob 可失败重试
LocalJob 可断网恢复
```

---

## 2. 升级目标

最终要达到：

```text id="flxrmt"
cloud-backend 部署在云服务器
local-backend 运行在用户电脑
用户一句话生成视频
cloud 负责 Agent 编排和任务状态
local 负责 HyperFrames / FFmpeg / 本地文件 / final.mp4
```

目标链路：

```text id="x3h5s4"
用户输入
  ↓
cloud：LLM Planner / DAG
  ↓
cloud：遇到 local 工具，创建 LocalJob
  ↓
local：Runner claim LocalJob
  ↓
local：执行本地工具
  ↓
local：保存本地 artifact
  ↓
local：回传 metadata / progress / result
  ↓
cloud：推进 DAG 下游节点
  ↓
frontend：展示本地视频和任务状态
```

---

## 3. 当前最短可跑通目标

不要一口气追求完整视频生产系统。下一步建议先跑通最小闭环：

```text id="kzx2id"
HYPERFRAMES_PROJECT_GENERATE
  ↓
HYPERFRAMES_RENDER
  ↓
ARTIFACT_PACKAGE
```

最小可用链路：

```text id="fzc76s"
cloud 创建 HYPERFRAMES_PROJECT_GENERATE LocalJob
  ↓
local 生成 hyperframes 项目
  ↓
cloud 创建 HYPERFRAMES_RENDER LocalJob
  ↓
local 渲染 final.mp4
  ↓
cloud 收到 final.mp4 metadata
  ↓
DAG 成功结束
```

这是 EdgeRun CloseLoop 的第一验收目标。

---

## 4. P0 修复一：统一 cloud/local 命令白名单

### 4.1 当前问题

local-backend 白名单已经比较完整：

```text id="gkjwrs"
HYPERFRAMES_PROJECT_GENERATE
HYPERFRAMES_RENDER
HYPERFRAMES_LINT
HYPERFRAMES_SNAPSHOT
FFMPEG_PROBE
FFMPEG_CLIP_EXTRACT
FFMPEG_ASSEMBLE
AUDIO_EXTRACT
AUDIO_NORMALIZE
ASR_TRANSCRIBE
ARTIFACT_PACKAGE
LOCAL_FILE_IMPORT
LOCAL_MEDIA_INDEX
```

但 cloud-backend 侧 LocalRunner 的 `ValidCommands` 仍是旧列表，导致：

```text id="a7r58g"
HYPERFRAMES_RENDER
HYPERFRAMES_PROJECT_GENERATE
ARTIFACT_PACKAGE
```

可能在 cloud 创建 LocalJob 前就被拒绝。

### 4.2 解决方案

在 cloud-backend 中统一定义 LocalCommand 常量：

```go id="i49pov"
const (
    CmdHyperFramesProjectGenerate = "HYPERFRAMES_PROJECT_GENERATE"
    CmdHyperFramesRender          = "HYPERFRAMES_RENDER"
    CmdHyperFramesLint            = "HYPERFRAMES_LINT"
    CmdHyperFramesSnapshot        = "HYPERFRAMES_SNAPSHOT"

    CmdFFmpegProbe       = "FFMPEG_PROBE"
    CmdFFmpegClipExtract = "FFMPEG_CLIP_EXTRACT"
    CmdFFmpegAssemble    = "FFMPEG_ASSEMBLE"

    CmdAudioExtract   = "AUDIO_EXTRACT"
    CmdAudioNormalize = "AUDIO_NORMALIZE"
    CmdASRTranscribe  = "ASR_TRANSCRIBE"

    CmdArtifactPackage = "ARTIFACT_PACKAGE"
    CmdLocalFileImport = "LOCAL_FILE_IMPORT"
    CmdLocalMediaIndex = "LOCAL_MEDIA_INDEX"
)
```

然后统一白名单：

```go id="llq3e8"
var ValidCommands = map[string]bool{
    CmdHyperFramesProjectGenerate: true,
    CmdHyperFramesRender:          true,
    CmdHyperFramesLint:            true,
    CmdHyperFramesSnapshot:        true,

    CmdFFmpegProbe:       true,
    CmdFFmpegClipExtract: true,
    CmdFFmpegAssemble:    true,

    CmdAudioExtract:   true,
    CmdAudioNormalize: true,
    CmdASRTranscribe:  true,

    CmdArtifactPackage: true,
    CmdLocalFileImport: true,
    CmdLocalMediaIndex: true,
}
```

### 4.3 同步 local-backend

local-backend 不要自己再写一套不同字符串。至少保持同名常量，或者通过共享协议文档固定。

新增文件：

```text id="vyarw4"
docs/contracts/local_commands.md
```

内容固定每个 command 的：

```text id="om47kt"
command name
payload schema
output schema
是否需要路径校验
是否需要用户确认
是否允许重试
```

---

## 5. P0 修复二：hyperframes_project_generator 彻底 local 化

### 5.1 当前问题

`hyperframes_project_generator` 如果仍走 cloud builtin，会导致：

```text id="w68ekd"
projectDir 生成在云端
  ↓
renderer 在本地执行
  ↓
本地找不到 projectDir
```

这是云端部署后必然出问题的点。

### 5.2 修改 tool manifest

将：

```yaml id="byssv6"
type: builtin_prompt_tool
endpoint: builtin://video-creation/hyperframes_project_generator
```

改为：

```yaml id="jda58v"
name: hyperframes_project_generator
type: local_tool
executionPlane: local
requiresUserDevice: true
artifactLocation: local
localCommand: HYPERFRAMES_PROJECT_GENERATE

localRequirements:
  os:
    - darwin
    - windows
    - linux
  commands:
    - node
  minDiskMb: 512
  requiresNetwork: false

capabilities:
  - video_composition
  - hyperframes_project
  - html_video_project
  - local_project_generation

approvalPolicy:
  required: false
  mode: none
```

### 5.3 payload 设计

cloud 下发：

```json id="tdqqwp"
{
  "projectId": "project_001",
  "compositionSpec": {
    "specVersion": "aios-video-composition-v1",
    "durationSec": 30,
    "fps": 30,
    "resolution": {
      "width": 1920,
      "height": 1080
    },
    "tracks": []
  },
  "script": {
    "title": "端午节的来历",
    "voiceover": "端午节并不只有一个来源……"
  },
  "outputDir": "local://projects/project_001/hyperframes"
}
```

local 输出：

```json id="wce3e1"
{
  "success": true,
  "projectDir": "local://projects/project_001/hyperframes",
  "entry": "index.html",
  "files": [
    "index.html",
    "assets/data.json",
    "assets/style.css",
    "manifest.json"
  ],
  "artifacts": [
    {
      "kind": "HYPERFRAMES_PROJECT",
      "name": "hyperframes_project",
      "storageRef": "local://projects/project_001/hyperframes",
      "localOnly": true
    }
  ]
}
```

---

## 6. P0 修复三：实现 HYPERFRAMES_RENDER 本地执行器

### 6.1 新增文件

```text id="dm6a6t"
local-backend/internal/localtool/hyperframes_render.go
```

### 6.2 执行职责

```text id="omq504"
1. 解析 local:// projectDir。
2. 检查 projectDir 存在。
3. 检查 entry 文件存在。
4. 检查 outputPath 父目录存在，不存在则创建。
5. 调用本机 HyperFrames Render Service。
6. 等待渲染完成。
7. 校验 final.mp4 存在且大小 > 0。
8. 返回 artifact metadata。
```

### 6.3 payload

```json id="fot95a"
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

### 6.4 输出

```json id="qrhkq6"
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
      "localOnly": true,
      "sizeBytes": 108923123
    }
  ],
  "metrics": {
    "renderTimeSec": 143,
    "fps": 30
  }
}
```

### 6.5 注册 executor

在 local-agent main 中增加：

```go id="drxu8l"
registry.Register(
    localtool.NewHyperFramesRenderExecutor(server.Paths().DataDir, cfg.HyperFrames.ServiceURL),
    "HYPERFRAMES_RENDER",
)
```

### 6.6 配置项

local-backend 增加环境变量：

```text id="v2roio"
TANGYING_HYPERFRAMES_SERVICE_URL=http://127.0.0.1:8787
TANGYING_LOCAL_WORKSPACE_ROOT=...
TANGYING_RENDER_TIMEOUT_SEC=1800
```

---

## 7. P0 修复四：实现 FFMPEG_PROBE 本地执行器

### 7.1 新增文件

```text id="xant1a"
local-backend/internal/localtool/ffmpeg_probe.go
```

### 7.2 执行职责

```text id="szwwsm"
1. 解析 local:// input。
2. 校验路径授权。
3. 调用 ffprobe。
4. 解析 JSON 输出。
5. 返回视频 metadata。
```

### 7.3 payload

```json id="mq4l62"
{
  "input": "local://projects/project_001/assets/input.mp4"
}
```

### 7.4 输出

```json id="qm272w"
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

### 7.5 注册 executor

```go id="wpktgs"
registry.Register(
    localtool.NewFFmpegProbeExecutor(server.Paths().DataDir),
    "FFMPEG_PROBE",
)
```

---

## 8. P0 修复五：实现 ARTIFACT_PACKAGE 本地执行器

### 8.1 新增文件

```text id="80e0h6"
local-backend/internal/localtool/artifact_package.go
```

### 8.2 用途

将本地项目产物打包：

```text id="mv4xlm"
final.mp4
render_report.json
video_composition_spec.json
hyperframes manifest
logs
```

生成：

```text id="x7hazn"
project_package.zip
```

### 8.3 payload

```json id="yggqwa"
{
  "projectId": "project_001",
  "include": [
    "local://projects/project_001/renders/final.mp4",
    "local://projects/project_001/artifacts/render_report.json",
    "local://projects/project_001/hyperframes/manifest.json"
  ],
  "output": "local://projects/project_001/packages/project_package.zip"
}
```

---

## 9. P0 修复六：LocalRunner 安全校验强化

### 9.1 当前风险

已有 auth middleware，但还需要细化校验：

```text id="ssxmn3"
runner 是否属于当前用户
runner 是否属于当前 device
job 是否属于当前用户
job 是否允许该 runner claim
complete/fail 是否来自 claim 该 job 的 runner
sessionId 是否匹配
```

### 9.2 增加请求头

local-backend 调 cloud 时统一携带：

```text id="pilif5"
Authorization: Bearer <token>
X-Device-ID: <device_id>
X-Runner-ID: <runner_id>
X-Runner-Session-ID: <session_id>
```

### 9.3 cloud 校验函数

新增：

```go id="ahcfi2"
func (s *Service) ValidateRunnerAccess(ctx context.Context, userID, deviceID, runnerID, sessionID string) error
```

校验：

```text id="w6erou"
1. runner 存在。
2. runner.user_id == userID。
3. runner.device_id == deviceID。
4. runner.session_id == sessionID。
5. runner.status != revoked。
```

新增：

```go id="cy7c06"
func (s *Service) ValidateJobAccess(ctx context.Context, userID, runnerID, jobID string) error
```

校验：

```text id="ufo8im"
1. job 存在。
2. job.user_id == userID。
3. 如果 job.runner_id 不为空，则必须等于 runnerID。
4. complete/fail 只能由 claimed runner 上报。
```

---

## 10. P0 修复七：PlanGuard 加 Local Capability 校验

### 10.1 问题

现在 Planner 可能规划 local 工具，但用户本地未启动 runner 或缺依赖，导致 DAG 卡在 `WAITING_LOCAL`。

### 10.2 新增接口

```go id="bp1q0q"
type LocalCapabilityProvider interface {
    HasOnlineRunner(ctx context.Context, userID string) (bool, error)
    SupportsCommand(ctx context.Context, userID string, command string) (bool, error)
    SatisfiesRequirements(ctx context.Context, userID string, req *tool.LocalRequirements) (bool, []string, error)
}
```

### 10.3 PlanGuard 校验逻辑

```text id="tvxnb0"
for each step:
  manifest := toolRegistry.GetManifest(step.Tool)

  if manifest.ExecutionPlane == "local":
      if no online runner:
          reject plan

      if !supports localCommand:
          reject plan

      if !satisfies localRequirements:
          reject plan
```

### 10.4 错误输出

```json id="v20cyp"
{
  "valid": false,
  "errors": [
    {
      "code": "LOCAL_CAPABILITY_MISSING",
      "tool": "hyperframes_renderer",
      "message": "本地执行器未检测到 HYPERFRAMES_RENDER 能力。请启动 local-backend 并确认 HyperFrames Render Service 可用。"
    }
  ]
}
```

---

## 11. P1 优化一：LLMPlanner 工具摘要加入 executionPlane

### 11.1 需要加入字段

传给 LLM 的工具摘要增加：

```json id="l48i21"
{
  "name": "hyperframes_renderer",
  "description": "...",
  "executionPlane": "local",
  "requiresUserDevice": true,
  "artifactLocation": "local",
  "localCommand": "HYPERFRAMES_RENDER",
  "localRequirements": {
    "commands": ["node", "ffmpeg"]
  },
  "capabilities": ["video_render", "hyperframes"]
}
```

### 11.2 目的

让模型知道：

```text id="6ipvk2"
1. 这个工具需要本地执行器。
2. 这个工具会生成 local artifact。
3. 本地不可用时不能选择它。
4. Seedance 是 remote_http，HyperFrames 是 local。
```

---

## 12. P1 优化二：启用 HybridToolRetriever

### 12.1 当前问题

LLMPlanner 仍偏 heuristic tool select。下一步要启用 `HybridToolRetriever`：

```text id="ou6w1n"
硬过滤
  ↓
关键词 / capability / tag 召回
  ↓
executionPlane 可用性过滤
  ↓
risk / cost / approval 排序
  ↓
返回 TopK tools
```

### 12.2 本地能力过滤

Retriever 过滤规则：

```text id="gvfht0"
1. executionPlane=local 且 runner offline → 不召回
2. localCommand 不支持 → 不召回
3. remote_http provider 未配置 → 不召回
4. high cost 且预算低 → 降权
```

---

## 13. P1 优化三：LocalJob progress 同步到 ai_node

### 13.1 当前问题

local job progress 只更新 local_jobs，前端 DAG 可能看不到细进度。

### 13.2 方案

`ReportProgress` 时同步：

```text id="a4958j"
local_jobs.progress
local_jobs.status
local_job_logs
ai_node.progress
ai_node.current_step
ai_node.heartbeat_at
```

### 13.3 前端展示

```text id="a86siw"
Node：hyperframes_renderer
状态：LOCAL_RUNNING
进度：45%
步骤：rendering frames
日志：正在渲染第 240 / 520 帧
```

---

## 14. P1 优化四：pending_report 断网恢复

### 14.1 问题

本地工具执行完成后，如果 cloud complete 请求失败，结果不能丢。

### 14.2 本地持久化目录

```text id="3st0te"
local-data/pending_reports/
├── local_job_001.complete.json
├── local_job_002.fail.json
└── local_job_003.progress.json
```

### 14.3 流程

```text id="qz758c"
local job 执行完成
  ↓
先写 pending_report
  ↓
尝试 complete cloud
  ↓
成功后删除 pending_report
  ↓
失败则保留
  ↓
下次 heartbeat 成功后重试上报
```

### 14.4 幂等

cloud complete 必须支持幂等：

```text id="xo2tyh"
同一个 jobId 重复 complete
  ↓
如果 output hash 相同，返回 success
  ↓
不重复推进 DAG
```

---

## 15. P1 优化五：本地 PathGuard

### 15.1 目的

防止云端下发恶意路径或错误路径。

### 15.2 允许路径

```text id="md9c3b"
local://projects/
local://artifacts/
local://cache/
local://logs/
用户显式导入的素材路径
```

### 15.3 禁止

```text id="7vfuyo"
../
~/.ssh
~/.aws
系统目录
未授权绝对路径
```

### 15.4 接口

```go id="n9wuaj"
type PathGuard interface {
    ResolveLocalURI(uri string) (string, error)
    EnsureReadable(uri string) error
    EnsureWritable(uri string) error
}
```

所有 local executor 必须通过 PathGuard。

---

## 16. P2 优化：正式 VideoCompositionSpec 项目生成

当前本地 HyperFrames Project Generator 是最小实现。下一步要升级为真正 VideoCompositionSpec 生成器。

### 16.1 输入

```json id="yv6lg5"
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
        "type": "video",
        "clips": []
      },
      {
        "type": "caption",
        "items": []
      },
      {
        "type": "overlay",
        "items": []
      },
      {
        "type": "audio",
        "clips": []
      }
    ]
  }
}
```

### 16.2 输出文件

```text id="45tzxs"
hyperframes/
├── index.html
├── assets/
│   ├── data.json
│   ├── style.css
│   └── media/
├── manifest.json
└── DESIGN.md
```

### 16.3 支持轨道

必须支持：

```text id="nglxg3"
video track
audio track
caption track
overlay track
image track
transition
```

---

## 17. 推荐实施顺序

## Phase 1：先修闭环阻断点

任务：

```text id="och0xv"
1. cloud ValidCommands 补齐。
2. hyperframes_project_generator 改 local。
3. local-backend 注册 HYPERFRAMES_RENDER。
4. local-backend 实现 HyperFramesRenderExecutor。
5. local-backend 实现 FFMPEG_PROBE。
6. local-backend 实现 ARTIFACT_PACKAGE。
```

验收：

```text id="tq06vw"
手动创建 HYPERFRAMES_PROJECT_GENERATE 和 HYPERFRAMES_RENDER LocalJob，local 能完整执行并 complete。
```

---

## Phase 2：安全和稳定性

任务：

```text id="rus7bi"
1. runner ownership 校验。
2. runner session 校验。
3. job ownership 校验。
4. PathGuard。
5. pending_report。
6. complete/fail 幂等。
```

验收：

```text id="gk0ija"
断网后本地执行完成，恢复后能补报；伪造 runner/job 不能操作别人的任务。
```

---

## Phase 3：Planner/Guard 能力感知

任务：

```text id="m06i2y"
1. PlanGuard 接入 LocalCapabilityProvider。
2. ToolRetriever 过滤不可用 local tool。
3. LLMPlanner 工具摘要增加 executionPlane。
4. 前端展示本地能力预检。
```

验收：

```text id="z83ony"
本地执行器离线时，计划阶段阻断或提示，不进入不可执行 DAG。
```

---

## Phase 4：VideoCompositionSpec 正式化

任务：

```text id="laq3lf"
1. 定义 compositionSpec schema。
2. Project Generator 支持 tracks。
3. Renderer 接收 composition manifest。
4. final_review 校验 final.mp4。
```

验收：

```text id="gnj5cs"
能生成包含字幕、卡片、音频、视频素材的 final.mp4。
```

---

## Phase 5：完整 E2E

输入：

```text id="wjva7c"
请生成一个 30 秒端午节知识分享视频。
```

验收：

```text id="czms7k"
1. cloud 生成 script。
2. cloud 生成 shot_list。
3. cloud 生成 render_strategy。
4. local 生成 hyperframes project。
5. local 渲染 final.mp4。
6. cloud 收到 artifact metadata。
7. DAG SUCCESS。
8. 前端能预览 final.mp4。
```

---

## 18. Coding Agent 执行指令

```text id="pbg2um"
目标：
实施 EdgeRun CloseLoop 升级。当前系统已经具备 cloud LocalRunner 控制面和 local-backend runner 骨架，但 E2E 仍被命令白名单、executor 缺失、manifest 未 local、能力校验不足等问题阻断。需要完成本地视频工具执行闭环。

P0：
1. 修改 cloud-backend/internal/core/localrunner/model.go，补齐 ValidCommands：
   - HYPERFRAMES_PROJECT_GENERATE
   - HYPERFRAMES_RENDER
   - HYPERFRAMES_LINT
   - HYPERFRAMES_SNAPSHOT
   - FFMPEG_PROBE
   - FFMPEG_CLIP_EXTRACT
   - FFMPEG_ASSEMBLE
   - AUDIO_EXTRACT
   - AUDIO_NORMALIZE
   - ASR_TRANSCRIBE
   - ARTIFACT_PACKAGE
   - LOCAL_FILE_IMPORT
   - LOCAL_MEDIA_INDEX

2. 修改 hyperframes_project_generator.tool.yaml：
   - type: local_tool
   - executionPlane: local
   - requiresUserDevice: true
   - artifactLocation: local
   - localCommand: HYPERFRAMES_PROJECT_GENERATE

3. 在 local-backend 新增 HyperFramesRenderExecutor：
   - 文件：internal/localtool/hyperframes_render.go
   - command: HYPERFRAMES_RENDER
   - 输入 projectDir / entry / outputPath / fps / quality
   - 调本机 HyperFrames Render Service
   - 输出 final.mp4 artifact metadata

4. 在 local-backend 新增 FFmpegProbeExecutor：
   - 文件：internal/localtool/ffmpeg_probe.go
   - command: FFMPEG_PROBE
   - 调 ffprobe
   - 输出 duration / resolution / codec / fps / hasAudio

5. 在 local-backend 新增 ArtifactPackageExecutor：
   - 文件：internal/localtool/artifact_package.go
   - command: ARTIFACT_PACKAGE
   - 打包项目产物

6. 在 local-backend/cmd/local-agent/main.go 注册：
   - HYPERFRAMES_RENDER
   - FFMPEG_PROBE
   - ARTIFACT_PACKAGE

P1：
1. cloud localrunner 增加 ValidateRunnerAccess。
2. cloud localrunner 增加 ValidateJobAccess。
3. 所有 heartbeat/claim/progress/complete/fail 校验 runner owner、deviceId、sessionId。
4. local client 所有请求增加：
   - X-Device-ID
   - X-Runner-ID
   - X-Runner-Session-ID

P2：
1. local-backend 新增 PathGuard。
2. 所有 local executor 只能访问 local workspace 和用户授权路径。
3. 禁止 ../、系统目录、未授权绝对路径。

P3：
1. local-backend 新增 pending_report。
2. complete/fail 先写本地 pending report。
3. cloud 上报成功后删除。
4. 网络恢复后补报。
5. cloud complete/fail 实现幂等。

P4：
1. PlanGuard 接入 LocalCapabilityProvider。
2. executionPlane=local 时检查 runner online。
3. 检查 runner 支持 localCommand。
4. 检查 localRequirements。
5. 不满足时阻断计划并返回明确错误。

P5：
1. LLMPlanner compactToolManifests 加入：
   - executionPlane
   - requiresUserDevice
   - artifactLocation
   - localCommand
   - localRequirements
   - providerCapabilities
2. ToolRetriever 过滤不可用 local 工具。

P6：
1. Project Generator 从极简 HTML 升级为 VideoCompositionSpec 项目生成器。
2. 支持 video/audio/caption/overlay tracks。
3. 输出 index.html、assets/data.json、assets/style.css、manifest.json、DESIGN.md。

验收：
1. 手动 LocalJob 测试 HYPERFRAMES_PROJECT_GENERATE 成功。
2. 手动 LocalJob 测试 HYPERFRAMES_RENDER 成功。
3. 手动 LocalJob 测试 FFMPEG_PROBE 成功。
4. 云端部署 + 本地 local-backend 能跑通 final.mp4。
5. local-backend 离线时 PlanGuard 阻断而不是 DAG 卡死。
6. 渲染中断网，恢复后 complete 能补报。
```

---

## 19. 最终判断

下一步优化的核心不是“继续扩展 Agent 能力”，而是完成：

```text id="zkq3p0"
EdgeRun CloseLoop
```

也就是让当前系统真正跑通：

```text id="3bfr53"
cloud 创建 LocalJob
local claim
local 执行
local 产出 artifact
local 回传
cloud 推进 DAG
```

优先级排序：

```text id="wolk6x"
第一优先级：命令白名单对齐
第二优先级：HYPERFRAMES_RENDER executor
第三优先级：project_generator local 化
第四优先级：FFMPEG_PROBE executor
第五优先级：PlanGuard 本地能力校验
第六优先级：pending_report 断网恢复
```

一句话总结：

**EdgeRun CloseLoop 完成后，你的系统才真正具备“云端部署 + 本地稳定视频生产”的工程基础。**
