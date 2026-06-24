# AIOS EdgeRun：云端控制面 + 本地执行面稳定化升级方案

## 1. 升级主题

本次升级命名为：

**AIOS EdgeRun**

主题表达：

```text
Cloud Control Plane + Local Execution Plane
云端负责编排，本地负责执行
```

本次升级不是继续堆视频工具，而是修正系统执行架构：

```text
当前问题：
cloud-backend 既编排又执行部分本地工具

升级目标：
cloud-backend 只做控制面
local-backend 成为本地工具执行面
```

最终目标：

```text
服务器后端部署在云端
用户电脑运行 local-backend
本地 FFmpeg / HyperFrames / Hypergen / 文件处理 / 本地模型稳定执行
云端只负责 Agent、DAG、审核、任务状态和结果管理
```

---

## 2. 当前系统判断

当前系统已经完成了一部分关键基础：

```text
已完成：
1. ToolManifest 已经支持 executionPlane / localRequirements / artifactLocation。
2. cloud-backend 已经有 localrunner / local_jobs / local_runners 表。
3. NodeExecutor 已能识别 executionPlane=local 并创建 LocalJob。
4. ai_node 状态已经扩展 WAITING_LOCAL / LOCAL_RUNNING / LOCAL_COMPLETED 等状态。
5. hyperframes_renderer 已开始标记为 local 工具。

仍缺失：
1. local-backend 还不是完整 Local Runner。
2. local-backend 还没有稳定 claim job / execute / complete 的循环。
3. local-backend 还没有本地工具执行器。
4. hyperframes_project_generator 仍可能在云端执行。
5. localrunner API 需要认证。
6. PlanGuard / ToolRetriever 还没有结合本地执行器能力过滤工具。
```

所以当前系统处于：

```text
云端控制面骨架已成型
本地执行面尚未闭环
```

---

## 3. 最终目标架构

```text
┌──────────────────────────────────────────────┐
│                cloud-backend                  │
│                                              │
│  User / Auth / Device                         │
│  Agent Runtime                               │
│  LLM Planner / PlanGuard / PlanCompiler       │
│  Orchestrator / DAG                           │
│  ToolRegistry                                 │
│  LocalRunner Manager                          │
│  LocalJob Queue                               │
│  Artifact Metadata                            │
│  Review / Approval                            │
│  Cloud Logs                                   │
└──────────────────────────────────────────────┘
                      ▲
                      │ HTTPS Polling / WebSocket
                      ▼
┌──────────────────────────────────────────────┐
│                local-backend                  │
│                                              │
│  Local Runner Loop                            │
│  Capability Probe                             │
│  Local Tool Registry                          │
│  Local Tool Executor                          │
│  FFmpeg / HyperFrames / Hypergen              │
│  Local File Manager                           │
│  Local Artifact Store                         │
│  Local Logs / Diagnostics                     │
│  Pending Report Queue                         │
└──────────────────────────────────────────────┘
                      ▲
                      │ localhost
                      ▼
┌──────────────────────────────────────────────┐
│              frontend / Electron              │
│                                              │
│  用户输入                                     │
│  本地执行器状态                               │
│  工具能力预检                                 │
│  Artifact 预览                                │
│  审核 / 确认                                  │
│  本地文件授权                                 │
└──────────────────────────────────────────────┘
```

核心原则：

```text
1. 云端不直接执行用户本地 CLI。
2. 云端不直接访问用户本地文件。
3. 云端不主动访问用户电脑端口。
4. 本地主动连接云端领取任务。
5. 云端只下发语义化工具任务。
6. 本地只执行白名单工具命令。
7. 大文件默认 local-only。
8. 小文件和 metadata 同步云端。
```

---

## 4. 云端与本地职责边界

### 4.1 cloud-backend 负责

```text
1. 用户登录与鉴权
2. 设备绑定
3. Agent 编排
4. LLM 调用
5. 工具选择
6. PlanGuard 校验
7. DAG 编译
8. 节点状态管理
9. LocalJob 创建
10. 本地执行器状态管理
11. 审核节点
12. Artifact 元数据
13. 小文件存储
14. 发布平台 API
15. 云端日志和审计
```

### 4.2 local-backend 负责

```text
1. 本地文件读写
2. 本地工作区管理
3. 本地 artifact 保存
4. FFmpeg / ffprobe
5. HyperFrames / Hypergen 渲染
6. 视频抽帧
7. 音频提取
8. 字幕转录
9. 本地模型 Provider
10. 大文件处理
11. 本地日志
12. 诊断包
13. 本地工具能力探测
14. 任务断网恢复
```

---

## 5. 工具执行面分类

所有工具必须明确执行面。

### 5.1 cloud 工具

适合云端执行：

```text
video_script_generator
shot_splitter
visual_plan_generator
render_strategy_planner
proposal_generator
publish_copy_generator
fact_checker
knowledge_researcher
```

特点：

```text
1. 主要依赖 LLM / API。
2. 输入输出较小。
3. 适合云端审计。
4. 不依赖用户本地文件。
```

---

### 5.2 local 工具

必须本地执行：

```text
hyperframes_project_generator
hyperframes_renderer
ffmpeg_probe
ffmpeg_clip_extractor
audio_extractor
audio_normalizer
asr_transcriber_local
video_package_exporter
local_media_indexer
artifact_packager
```

特点：

```text
1. 依赖用户本地文件。
2. 依赖本地 CLI。
3. 处理大视频。
4. 涉及隐私素材。
5. 产物通常较大。
```

---

### 5.3 remote_http 工具

第三方 API 执行：

```text
seedance_clip_generator
image_generation_provider
tts_provider
stock_video_search
platform_publish_api
```

特点：

```text
1. 外部服务提供。
2. 云端调用更方便。
3. 需要成本和权限控制。
```

---

## 6. ToolManifest 改造标准

### 6.1 新增字段

所有工具 Manifest 必须包含：

```yaml
executionPlane: cloud | local | remote_http | hybrid
requiresUserDevice: true | false
artifactLocation: cloud | local | both
localCommand: COMMAND_NAME
localRequirements:
  os:
    - darwin
    - windows
    - linux
  commands:
    - ffmpeg
    - node
  minDiskMb: 2048
  requiresNetwork: false
```

---

### 6.2 hyperframes_project_generator

必须改成 local：

```yaml
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

approvalPolicy:
  required: true
  mode: after_artifact
  reason: HyperFrames 项目将作为最终渲染输入，需要审核。
```

原因：

```text
如果 project_generator 在云端执行，生成的 projectDir 在云端；
renderer 在本地执行时，本地找不到这个目录。
```

---

### 6.3 hyperframes_renderer

```yaml
name: hyperframes_renderer
type: local_tool
executionPlane: local
requiresUserDevice: true
artifactLocation: local
localCommand: HYPERFRAMES_RENDER

localRequirements:
  os:
    - darwin
    - windows
    - linux
  commands:
    - node
    - ffmpeg
  minDiskMb: 2048
  requiresNetwork: false

capabilities:
  - video_render
  - hyperframes
  - html_to_video
  - mp4_render

approvalPolicy:
  required: true
  mode: before_execute
  reason: 视频渲染耗时较长并会生成大文件，需要用户确认。
```

---

### 6.4 ffmpeg_probe

```yaml
name: ffmpeg_probe
type: local_tool
executionPlane: local
requiresUserDevice: true
artifactLocation: local
localCommand: FFMPEG_PROBE

localRequirements:
  commands:
    - ffmpeg
  minDiskMb: 128

capabilities:
  - media_probe
  - video_metadata
```

---

### 6.5 script_generator

```yaml
name: video_script_generator
type: cloud_prompt_tool
executionPlane: cloud
requiresUserDevice: false
artifactLocation: cloud

capabilities:
  - script_generation
  - video_planning
```

---

## 7. LocalJob 协议

### 7.1 Runner 注册

local-backend 启动后注册：

```http
POST /api/local-runners/register
```

请求：

```json
{
  "deviceId": "device_macbook_001",
  "runnerVersion": "1.0.0",
  "platform": {
    "os": "darwin",
    "arch": "arm64",
    "hostname": "Wang-MacBook"
  },
  "workspaceRoot": "local://aios/projects",
  "capabilities": [
    {
      "toolName": "ffmpeg_probe",
      "command": "FFMPEG_PROBE",
      "available": true,
      "version": "ffmpeg 7.0"
    },
    {
      "toolName": "hyperframes_renderer",
      "command": "HYPERFRAMES_RENDER",
      "available": true,
      "version": "adapter 1.0.0"
    }
  ]
}
```

返回：

```json
{
  "runnerId": "runner_001",
  "sessionId": "runner_session_001",
  "heartbeatIntervalSec": 15,
  "pollIntervalSec": 3
}
```

---

### 7.2 心跳

```http
POST /api/local-runners/{runnerId}/heartbeat
```

请求：

```json
{
  "sessionId": "runner_session_001",
  "status": "online",
  "runningJobs": 1,
  "diskFreeMb": 120000,
  "cpuLoad": 0.35,
  "memoryUsageMb": 2048
}
```

---

### 7.3 领取任务

```http
GET /api/local-runners/{runnerId}/jobs/claim
```

返回：

```json
{
  "job": {
    "jobId": "local_job_001",
    "projectId": "project_001",
    "nodeId": "node_hyperframes_render",
    "toolName": "hyperframes_renderer",
    "command": "HYPERFRAMES_RENDER",
    "payload": {
      "projectDir": "local://projects/project_001/hyperframes",
      "entry": "index.html",
      "outputPath": "local://projects/project_001/renders/final.mp4",
      "fps": 30,
      "quality": "standard"
    },
    "timeoutSec": 1800,
    "artifactPolicy": {
      "location": "local",
      "syncMetadataToCloud": true,
      "syncFileToCloud": false
    }
  }
}
```

---

### 7.4 上报进度

```http
POST /api/local-jobs/{jobId}/progress
```

请求：

```json
{
  "status": "running",
  "progress": 0.45,
  "step": "rendering",
  "message": "正在渲染第 240 / 520 帧",
  "logs": [
    "HyperFrames render started",
    "FFmpeg encoder ready"
  ]
}
```

---

### 7.5 完成任务

```http
POST /api/local-jobs/{jobId}/complete
```

请求：

```json
{
  "success": true,
  "output": {
    "summary": "视频渲染完成",
    "artifacts": [
      {
        "artifactId": "artifact_final_video",
        "kind": "VIDEO",
        "name": "final.mp4",
        "storageRef": "local://projects/project_001/renders/final.mp4",
        "mimeType": "video/mp4",
        "sizeBytes": 128000000,
        "syncStatus": "local_only"
      }
    ],
    "metrics": {
      "durationSec": 60.2,
      "renderTimeSec": 148,
      "fps": 30
    }
  }
}
```

---

### 7.6 失败任务

```http
POST /api/local-jobs/{jobId}/fail
```

请求：

```json
{
  "success": false,
  "error": {
    "code": "LOCAL_TOOL_EXEC_FAILED",
    "message": "HyperFrames Render Service 不可用",
    "details": "127.0.0.1:8787 connection refused"
  },
  "retryable": true,
  "diagnostics": {
    "logsRef": "local://projects/project_001/logs/local_job_001.log"
  }
}
```

---

## 8. cloud-backend 改造方案

### 8.1 localrunner API 加认证

所有 localrunner API 必须要求：

```text
Authorization: Bearer <user_token>
X-Device-ID: <device_id>
X-Runner-Session-ID: <session_id>
```

云端校验：

```text
1. token.userId 必须存在。
2. deviceId 必须属于当前 userId。
3. runnerId 必须属于当前 userId。
4. job.userId 必须等于 token.userId。
5. job.runnerId 必须等于 runnerId 或可被该 runner claim。
```

禁止匿名：

```text
register
heartbeat
claim
complete
fail
```

---

### 8.2 NodeExecutor 执行面分发

NodeExecutor 执行逻辑改为：

```text
收到 node ready
  ↓
读取 tool manifest
  ↓
判断 executionPlane

executionPlane=cloud:
  走现有云端工具执行

executionPlane=remote_http:
  走外部 HTTP Provider

executionPlane=local:
  创建 LocalJob
  node.status = WAITING_LOCAL
  返回，不阻塞 Worker

executionPlane=hybrid:
  拆成 cloud/local/remote_http 子节点
```

---

### 8.3 LocalJob 完成后推进 DAG

local job complete 时：

```text
1. 更新 local_jobs.status = COMPLETED
2. 写入 local_jobs.output
3. 写入 artifact metadata
4. 更新 ai_node.output
5. ai_node.status = SUCCESS
6. 调用 StateMachine.OnSuccess
7. dependency checker 推进下游节点
```

local job fail 时：

```text
1. 更新 local_jobs.status = FAILED
2. 写入 error
3. ai_node.status = FAILED 或 WAITING_RETRY
4. 根据 retryPolicy 判断是否重试
```

---

### 8.4 PlanGuard 增加 Local Capability 校验

PlanGuard 校验逻辑新增：

```text
如果 tool.executionPlane == local:
  1. 用户是否有在线 runner
  2. runner 是否支持 tool.localCommand
  3. runner 是否满足 localRequirements.commands
  4. runner 是否有足够磁盘
  5. artifactLocation 是否合理
```

错误示例：

```json
{
  "code": "LOCAL_RUNNER_NOT_AVAILABLE",
  "message": "本地执行器未启动，无法执行 hyperframes_renderer。"
}
```

或：

```json
{
  "code": "LOCAL_CAPABILITY_MISSING",
  "message": "本地缺少 ffmpeg，无法执行 FFMPEG_PROBE。"
}
```

---

### 8.5 ToolRetriever 过滤不可用工具

ToolRetriever 在选择工具前应过滤：

```text
1. local 工具但 runner offline → 不选
2. local 工具但 capability 不满足 → 不选
3. remote_http 工具但 provider 未配置 → 不选
4. high cost 工具但预算不足 → 降权或不选
```

否则 Planner 会生成不可执行计划。

---

### 8.6 LLMPlanner 工具摘要增加执行面

传给 LLM 的工具摘要必须包含：

```json
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

这样模型知道这个工具需要用户本地执行器在线。

---

## 9. local-backend 改造方案

### 9.1 新增模块

```text
local-backend/internal/localrunner/
├── client.go
├── loop.go
├── heartbeat.go
├── claim.go
├── report.go
└── capability_probe.go

local-backend/internal/localtool/
├── registry.go
├── executor.go
├── commands.go
├── path_guard.go
├── artifact_reporter.go
└── tools/
    ├── hyperframes_project.go
    ├── hyperframes_render.go
    ├── ffmpeg_probe.go
    ├── ffmpeg_clip.go
    ├── audio_extract.go
    ├── asr.go
    └── package.go
```

---

### 9.2 local-backend 启动流程

```text
local-backend 启动
  ↓
读取 cloud API 地址
  ↓
读取用户 token / deviceId
  ↓
初始化本地 workspace
  ↓
探测本机工具能力
  ├── ffmpeg
  ├── node
  ├── hyperframes-render-service
  ├── hypergen
  ├── whisper
  └── 本地模型 provider
  ↓
向 cloud 注册 runner
  ↓
开启 heartbeat
  ↓
开启 claim loop
```

---

### 9.3 Capability Probe

探测内容：

```text
1. 操作系统
2. CPU 架构
3. FFmpeg 是否可用
4. Node 是否可用
5. HyperFrames Render Service 是否可用
6. Hypergen 是否可用
7. 工作区是否可写
8. 磁盘剩余空间
9. 本地模型配置
```

输出：

```json
{
  "capabilities": [
    {
      "toolName": "ffmpeg_probe",
      "command": "FFMPEG_PROBE",
      "available": true,
      "version": "ffmpeg 7.0"
    },
    {
      "toolName": "hyperframes_renderer",
      "command": "HYPERFRAMES_RENDER",
      "available": true,
      "version": "hyperframes-render-service 1.0.0"
    }
  ],
  "warnings": [
    {
      "code": "TTS_NOT_CONFIGURED",
      "message": "TTS 未配置，无法自动生成口播音频。"
    }
  ]
}
```

---

### 9.4 Claim Loop

```text
while running:
  heartbeat
  job = claimJob()
  if job == nil:
    sleep(pollInterval)
    continue

  validate job
  execute job
  report progress
  complete or fail
```

要求：

```text
1. claim 后本地要持久化 job。
2. 进程崩溃重启后要恢复未上报结果。
3. 网络断开时进入 pending_report。
4. 网络恢复后补报 complete/fail。
```

---

### 9.5 LocalToolExecutor

统一接口：

```go
type LocalToolExecutor interface {
    Execute(ctx context.Context, job LocalJob) (*LocalToolResult, error)
}
```

根据 command 分发：

```text
HYPERFRAMES_PROJECT_GENERATE
HYPERFRAMES_RENDER
FFMPEG_PROBE
FFMPEG_CLIP_EXTRACT
AUDIO_EXTRACT
ARTIFACT_PACKAGE
```

禁止执行任意 shell。

---

## 10. 本地命令白名单

允许：

```text
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

禁止：

```text
bash
sh
cmd.exe
powershell
python arbitrary
node arbitrary
curl arbitrary
rm
mv arbitrary
cp arbitrary
```

原则：

```text
云端只能下发语义命令
本地把语义命令映射为安全实现
```

---

## 11. 本地路径安全

### 11.1 路径协议

统一使用：

```text
local://projects/project_001/renders/final.mp4
local://projects/project_001/hyperframes/index.html
cloud://artifacts/script.json
```

---

### 11.2 允许路径

```text
TangyingAIOS/projects/
TangyingAIOS/artifacts/
TangyingAIOS/cache/
TangyingAIOS/logs/
用户通过文件选择器授权的素材路径
```

---

### 11.3 禁止路径

```text
~/.ssh
~/.aws
~/.config
系统目录
任意父目录穿越
未授权用户目录
```

---

### 11.4 PathGuard

本地执行前必须检查：

```text
1. local:// 是否解析到 workspace 内
2. 是否包含 ../
3. 是否访问授权路径
4. 输出路径是否可写
5. 是否覆盖重要文件
```

---

## 12. Artifact 同步策略

### 12.1 小文件同步云端

同步：

```text
JSON
Markdown
SRT
VTT
缩略图
低清预览
render_report
final_review
diagnostic_summary
```

---

### 12.2 大文件默认 local-only

本地保存：

```text
原始 MP4
中间 MP4
final.mp4
图片序列
音频中间文件
HyperFrames 项目目录
```

云端保存 metadata：

```json
{
  "artifactId": "final_video",
  "kind": "VIDEO",
  "storageRef": "local://projects/project_001/renders/final.mp4",
  "localOnly": true,
  "sizeBytes": 128000000,
  "mimeType": "video/mp4"
}
```

---

### 12.3 用户手动同步云端

前端提供：

```text
打开本地文件
打开所在文件夹
同步到云端
导出项目包
生成诊断包
```

只有用户点击“同步到云端”后才上传大文件。

---

## 13. 视频工具迁移路径

### 13.1 保持云端的工具

```text
pipeline_selector
capability_preflight
proposal_generator
knowledge_researcher
fact_checker
video_script_generator
shot_splitter
visual_feasibility_analyzer
render_strategy_planner
publish_copy_generator
```

---

### 13.2 迁移到本地的工具

```text
hyperframes_project_generator
hyperframes_renderer
hyperframes_linter
hyperframes_snapshot
ffmpeg_probe
ffmpeg_clip_extractor
audio_extractor
audio_normalizer
asr_transcriber_local
video_package_exporter
local_media_indexer
```

---

### 13.3 保持 remote_http 的工具

```text
seedance_clip_generator
image_generator
tts_generator
stock_video_search
platform_publish_api
```

---

## 14. 视频生产链路改造

### 14.1 一句话生成知识视频

```text
用户输入
  ↓
cloud：proposal_generator
  ↓
cloud：video_script_generator
  ↓
cloud：shot_splitter
  ↓
cloud：visual_feasibility_analyzer
  ↓
cloud：render_strategy_planner
  ↓
用户审核 render_strategy
  ↓
remote_http：Seedance 生成 B-roll
  ↓
local：hyperframes_project_generator
  ↓
local：hyperframes_renderer
  ↓
local：video_package_exporter
  ↓
cloud：artifact metadata + final_review
```

---

### 14.2 用户上传 MP4 自动剪辑

```text
用户选择本地 MP4
  ↓
local：LOCAL_FILE_IMPORT
  ↓
local：FFMPEG_PROBE
  ↓
local：AUDIO_EXTRACT
  ↓
local：ASR_TRANSCRIBE
  ↓
cloud：clip_planner / overlay_designer
  ↓
local：FFMPEG_CLIP_EXTRACT
  ↓
local：HYPERFRAMES_PROJECT_GENERATE
  ↓
local：HYPERFRAMES_RENDER
  ↓
local：final.mp4
```

---

### 14.3 Seedance + HyperFrames 混合视频

```text
cloud：render_strategy_planner
  ↓
remote_http：seedance_clip_generator
  ↓
cloud/local：seedance_clip_qc
  ↓
local：保存 Seedance clip 到 workspace
  ↓
local：HyperFrames 叠加字幕、卡片、音频、转场
  ↓
local：final.mp4
```

---

## 15. 前端升级

### 15.1 本地执行器状态面板

显示：

```text
本地执行器：在线 / 离线
云端连接：正常 / 断开
FFmpeg：可用 / 不可用
Node：可用 / 不可用
HyperFrames：可用 / 不可用
工作区：可写 / 不可写
磁盘剩余：xxx GB
```

---

### 15.2 工具能力预检

在生成视频前显示：

```text
当前能力：
- 云端 LLM：可用
- 本地 HyperFrames：可用
- 本地 FFmpeg：可用
- Seedance：已配置
- ASR：未配置
- TTS：未配置

推荐模式：
- 可生成图文口播视频
- 可上传 MP4 自动剪辑加字幕
- 可用 Seedance 生成短 B-roll
```

---

### 15.3 LocalJob 进度展示

显示：

```text
任务：HyperFrames 渲染
状态：执行中
进度：43%
当前步骤：rendering frames
日志：...
输出：final.mp4
```

---

## 16. 断网恢复和稳定性

### 16.1 云端断开

local-backend：

```text
1. 保持本地任务继续执行。
2. 执行完成后写入 pending_report。
3. 不再 claim 新任务。
4. 网络恢复后补报 complete/fail。
```

---

### 16.2 本地退出

cloud-backend：

```text
1. heartbeat 超时后 runner 标记 offline。
2. 正在执行任务标记 runner_lost。
3. 用户重新启动后允许恢复。
4. 超过超时时间后进入 needs_retry。
```

---

### 16.3 幂等处理

LocalJob 必须包含：

```text
jobId
nodeId
projectId
idempotencyKey
attempt
```

云端对 complete/fail 做幂等：

```text
同一个 jobId 重复 complete，不重复推进 DAG。
同一个 nodeId 多次完成，只接受第一个有效结果。
```

---

## 17. 数据库补充设计

### 17.1 local_runners

```sql
CREATE TABLE local_runners (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    runner_version TEXT,
    os TEXT,
    arch TEXT,
    hostname TEXT,
    status TEXT NOT NULL,
    last_heartbeat_at TIMESTAMP,
    capabilities JSONB,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
```

---

### 17.2 local_jobs

```sql
CREATE TABLE local_jobs (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    project_id TEXT,
    node_id TEXT NOT NULL,
    runner_id TEXT,
    tool_name TEXT NOT NULL,
    command TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL,
    progress NUMERIC DEFAULT 0,
    output JSONB,
    error JSONB,
    attempt INT DEFAULT 0,
    timeout_sec INT,
    idempotency_key TEXT,
    created_at TIMESTAMP NOT NULL,
    claimed_at TIMESTAMP,
    completed_at TIMESTAMP,
    updated_at TIMESTAMP NOT NULL
);
```

---

### 17.3 local_job_logs

```sql
CREATE TABLE local_job_logs (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL,
    level TEXT NOT NULL,
    message TEXT NOT NULL,
    payload JSONB,
    created_at TIMESTAMP NOT NULL
);
```

---

## 18. 实施阶段

## Phase 1：补齐 Local Runner 闭环

目标：

```text
local-backend 能注册、心跳、领取任务、完成任务。
```

任务：

```text
1. local-backend 新增 localrunner client。
2. 实现 register。
3. 实现 heartbeat。
4. 实现 claim loop。
5. 实现 progress / complete / fail。
6. 本地持久化 running job。
7. cloud localrunner API 加认证。
```

验收：

```text
cloud 创建 LocalJob 后，local-backend 能 claim 并上报 complete。
```

---

## Phase 2：实现 LocalToolExecutor

目标：

```text
local-backend 能执行白名单本地工具。
```

任务：

```text
1. 新增 localtool registry。
2. 新增 command 白名单。
3. 新增 PathGuard。
4. 实现 FFMPEG_PROBE。
5. 实现 HYPERFRAMES_PROJECT_GENERATE。
6. 实现 HYPERFRAMES_RENDER。
7. 实现 ARTIFACT_PACKAGE。
```

验收：

```text
本地能执行 HYPERFRAMES_RENDER 并生成 final.mp4。
```

---

## Phase 3：迁移视频工具到 local plane

目标：

```text
视频重工具不再云端执行。
```

任务：

```text
1. hyperframes_project_generator → local
2. hyperframes_renderer → local
3. video_package_exporter → local
4. ffmpeg/audio 类工具 → local
5. 修改 ToolManifest。
6. 修改 NodeExecutor 分发。
7. 修改 Artifact metadata。
```

验收：

```text
cloud-backend 部署在云端时，final.mp4 生成在用户本地。
```

---

## Phase 4：PlanGuard / ToolRetriever 能力过滤

目标：

```text
不要生成不可执行计划。
```

任务：

```text
1. PlanGuard 检查在线 runner。
2. PlanGuard 检查 localCommand capability。
3. PlanGuard 检查 localRequirements。
4. ToolRetriever 过滤不可用 local tools。
5. LLMPlanner 工具摘要加入 executionPlane。
```

验收：

```text
本地 FFmpeg 不可用时，系统不会直接规划 ffmpeg 工具。
```

---

## Phase 5：前端体验与诊断

目标：

```text
用户知道本地执行器是否可用，任务卡在哪里。
```

任务：

```text
1. 本地执行器状态面板。
2. 本地工具能力预检。
3. LocalJob 进度展示。
4. final.mp4 本地预览。
5. 打开本地文件夹。
6. 诊断包导出。
```

验收：

```text
用户能看到本地执行器状态、依赖缺失原因和任务进度。
```

---

## Phase 6：稳定性强化

目标：

```text
断网、重启、失败后可恢复。
```

任务：

```text
1. pending_report 队列。
2. local job 幂等。
3. runner offline 恢复。
4. job retry policy。
5. local job timeout。
6. 云端状态一致性巡检。
```

验收：

```text
渲染时断网，恢复后能继续上报结果。
```

---

## 19. 关键验收用例

### 19.1 云端部署 + 本地渲染

环境：

```text
cloud-backend 在云服务器
local-backend 在用户 Mac
HyperFrames Render Service 在用户 Mac
```

输入：

```text
请生成一个 30 秒知识分享视频。
```

验收：

```text
1. cloud 生成 AgentPlan。
2. cloud 创建 LocalJob。
3. local claim HYPERFRAMES_PROJECT_GENERATE。
4. local 生成 HyperFrames 项目。
5. local claim HYPERFRAMES_RENDER。
6. local 生成 final.mp4。
7. cloud 收到 artifact metadata。
8. 前端能预览 local final.mp4。
```

---

### 19.2 本地执行器离线

操作：

```text
关闭 local-backend。
```

验收：

```text
1. cloud 显示 runner offline。
2. PlanGuard 拒绝 local-only 工具。
3. 前端提示用户启动本地执行器。
4. DAG 不应静默卡死。
```

---

### 19.3 用户上传 MP4

输入：

```text
用户选择本地 input.mp4，加字幕和知识卡片。
```

验收：

```text
1. input.mp4 不强制上传云端。
2. local 执行 ffprobe。
3. local 提取音频。
4. local 生成或接收字幕。
5. local HyperFrames 合成字幕和卡片。
6. final.mp4 本地生成。
```

---

### 19.4 断网恢复

操作：

```text
local 执行渲染过程中断网。
```

验收：

```text
1. 本地继续完成渲染。
2. 结果进入 pending_report。
3. 网络恢复后自动 complete。
4. cloud DAG 正常推进。
```

---

## 20. 给 Coding Agent 的完整任务指令

```text
目标：
实施 AIOS EdgeRun 升级。当前 cloud-backend 已经具备 localrunner 控制面雏形，但 local-backend 还没有完整本地执行闭环。需要把系统升级为“云端控制面 + 本地执行面”，保证 cloud-backend 部署到云端后，本地视频工具仍然稳定运行。

P0：Local Runner 闭环
1. 在 local-backend 新增 internal/localrunner。
2. 实现 cloud client。
3. local-backend 启动后向 cloud 注册 runner。
4. 实现 heartbeat。
5. 实现 claim loop。
6. 实现 progress / complete / fail。
7. 本地持久化 running job 和 pending report。
8. cloud localrunner API 增加认证和 device 校验。

P1：Local Tool Executor
1. 新增 local-backend/internal/localtool。
2. 实现 command 白名单。
3. 实现 PathGuard。
4. 实现 LocalToolExecutor 接口。
5. 实现 HYPERFRAMES_PROJECT_GENERATE。
6. 实现 HYPERFRAMES_RENDER。
7. 实现 FFMPEG_PROBE。
8. 实现 ARTIFACT_PACKAGE。
9. 执行结果回传 artifact metadata。

P2：工具迁移
1. hyperframes_project_generator.tool.yaml 改为 executionPlane=local。
2. hyperframes_renderer 保持 executionPlane=local。
3. video_package_exporter 改为 executionPlane=local。
4. ffmpeg/audio 类工具全部改为 executionPlane=local。
5. 云端脚本/策略/分镜工具保持 executionPlane=cloud。
6. remote provider 工具使用 executionPlane=remote_http。

P3：PlanGuard / ToolRetriever
1. PlanGuard 增加 LocalCapabilityProvider。
2. 检查用户是否有 online runner。
3. 检查 runner 是否支持 localCommand。
4. 检查 runner 是否满足 localRequirements。
5. ToolRetriever 过滤不可用 local tools。
6. LLMPlanner compactToolManifests 增加 executionPlane、requiresUserDevice、artifactLocation、localRequirements。

P4：NodeExecutor 和 DAG 状态
1. NodeExecutor 根据 executionPlane 分发。
2. local 工具创建 LocalJob。
3. node 状态进入 WAITING_LOCAL。
4. complete 后更新 node output 并调用 StateMachine.OnSuccess。
5. fail 后调用 StateMachine.OnFailure 或 retry。
6. progress 同步到 ai_node。

P5：前端体验
1. 增加本地执行器状态面板。
2. 显示 ffmpeg/node/hyperframes 能力。
3. 本地能力缺失时提示安装或启动。
4. 展示 LocalJob 进度。
5. 支持 final.mp4 本地预览。
6. 支持打开文件夹和同步云端。

P6：稳定性
1. 实现 pending_report。
2. 实现断网恢复。
3. 实现 local job 幂等。
4. 实现 runner offline 处理。
5. 实现诊断包导出。
6. 完成云端部署 + 本地渲染 E2E。
```

---

## 21. 最终上线标准

只有满足下面条件，才算 EdgeRun 完成：

```text
1. cloud-backend 部署到云服务器。
2. local-backend 在用户电脑启动。
3. local-backend 能注册 runner。
4. local-backend 能上报本机能力。
5. cloud 能创建 LocalJob。
6. local 能 claim LocalJob。
7. local 能执行 HYPERFRAMES_PROJECT_GENERATE。
8. local 能执行 HYPERFRAMES_RENDER。
9. final.mp4 生成在用户电脑。
10. cloud 收到 artifact metadata。
11. DAG 能继续推进。
12. 本地执行器离线时系统能明确提示，而不是卡死。
13. 断网恢复后任务结果能补报。
```

---

## 22. 最终结论

当前最重要的升级方向不是继续加视频能力，而是补齐执行架构：

```text
cloud-backend = 控制面
local-backend = 执行面
```

如果不完成这个升级，后续 cloud-backend 真正部署到云端后会遇到：

```text
1. 云端访问不到用户本地文件。
2. 云端 127.0.0.1 不是用户电脑。
3. 本地 FFmpeg / HyperFrames / Hypergen 无法被云端直接调用。
4. 大文件处理和视频渲染不稳定。
5. 用户素材隐私和上传成本失控。
```

完成 **AIOS EdgeRun** 后，系统才能稳定进入下一阶段：

```text
VideoForge Studio
  ↓
本地素材处理
  ↓
本地 HyperFrames 合成
  ↓
本地 final.mp4
  ↓
云端 Agent 编排和审核
```

一句话总结：

**AIOS EdgeRun 是你的视频 Agent 从“本地 Demo / 云端半成品”走向“真实云端部署 + 本地稳定生产”的必做升级。**
