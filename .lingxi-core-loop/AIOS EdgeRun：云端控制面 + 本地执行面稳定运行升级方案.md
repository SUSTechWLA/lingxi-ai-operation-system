# AIOS EdgeRun：云端控制面 + 本地执行面稳定运行升级方案

## 1. 升级主题

本次升级命名为：

**AIOS EdgeRun**

主题表达：

```text
Cloud Control Plane + Local Execution Plane
云端负责编排，本地负责执行
```

核心目标：

```text
1. cloud-backend 真实部署到云端后，不再依赖本机 CLI、本机文件、本机 127.0.0.1。
2. local-backend 成为稳定的本地工具执行器。
3. FFmpeg、HyperFrames、Hypergen、Whisper、本地模型、本地素材处理都在用户电脑执行。
4. 云端只下发语义化工具任务，不下发任意 shell。
5. 本地执行结果通过安全协议回传云端。
6. 大文件默认保存在本地，小文件和元数据同步云端。
```

一句话：

> **AIOS EdgeRun 要把你的系统从“云端直接执行工具”升级为“云端编排 + 本地稳定执行”的混合架构。**

---

## 2. 当前核心问题

当前系统虽然已经有：

```text
cloud-backend
local-backend
ToolRegistry
AgentRuntime
HyperFrames Render Service
localrunner 雏形
```

但从实际架构看，很多工具仍然偏向：

```text
cloud-backend 编排
  ↓
cloud-backend Worker 直接执行工具
  ↓
cloud-backend 本机写文件 / 调 CLI / 调本机服务
```

如果后端真实部署到云端，会出现严重问题：

```text
1. cloud-backend 的 127.0.0.1 指向云服务器，不是用户电脑。
2. 云端无法直接访问用户电脑的本地文件。
3. 云端无法使用用户电脑安装的 FFmpeg / Node / HyperFrames / Whisper。
4. 用户上传的大视频如果全部上传云端，成本高、慢、隐私风险高。
5. 云端执行 CLI 会让部署环境复杂化。
6. 用户本地素材处理、视频渲染、诊断包生成无法稳定落地。
```

所以必须升级为：

```text
cloud-backend = 控制面
local-backend = 执行面
```

---

## 3. 目标架构

```text
┌────────────────────────────────────────────┐
│                cloud-backend                │
│                                            │
│  Agent Runtime                             │
│  Planner / Guard / Compiler                │
│  Orchestrator / DAG                        │
│  ToolRegistry                              │
│  Local Job Queue                           │
│  Artifact Metadata                         │
│  Review / Approval                         │
│  User / Device / Auth                      │
└────────────────────────────────────────────┘
                     ▲
                     │ HTTPS / WebSocket / Polling
                     ▼
┌────────────────────────────────────────────┐
│                local-backend                │
│                                            │
│  Local Runner                              │
│  Tool Capability Probe                     │
│  FFmpeg Executor                           │
│  HyperFrames Executor                      │
│  Hypergen Executor                         │
│  Local File Manager                        │
│  Local Artifact Store                      │
│  Local Logs / Diagnostics                  │
│  Optional Local Model Provider             │
└────────────────────────────────────────────┘
                     ▲
                     │ localhost
                     ▼
┌────────────────────────────────────────────┐
│             frontend / Electron             │
│                                            │
│  用户交互                                  │
│  本地服务状态                              │
│  Artifact 预览                             │
│  Render Strategy 审核                      │
│  本地文件授权                              │
└────────────────────────────────────────────┘
```

---

## 4. 云端和本地职责边界

### 4.1 cloud-backend 职责

云端应该负责：

```text
1. 用户注册登录
2. 设备绑定
3. Agent 编排
4. LLM 调用
5. ToolRegistry
6. 工具能力发现
7. Pipeline / DAG 状态
8. 人工审核节点
9. LocalJob 创建
10. 任务进度聚合
11. Artifact 元数据
12. 小文件同步
13. 发布平台 API
14. 云端日志和审计
```

云端不应该直接做：

```text
1. 调用户电脑上的 FFmpeg
2. 读用户本地视频文件
3. 写用户本地项目目录
4. 调用户电脑上的 HyperFrames Render Service
5. 处理本地大视频文件
6. 执行用户电脑 CLI
```

---

### 4.2 local-backend 职责

本地应该负责：

```text
1. 本地文件读写
2. 本地项目工作区管理
3. 本地 artifact 保存
4. FFmpeg / ffprobe
5. HyperFrames / Hypergen 渲染
6. 本地素材抽帧
7. 本地音频提取
8. 本地字幕转录
9. 本地模型 Provider
10. 大视频处理
11. 本地执行日志
12. 本地诊断包
13. 本地服务健康检查
```

---

## 5. 通信方向设计

### 5.1 不建议云端主动 HTTP 调本地

不要设计成：

```text
cloud-backend → http://用户电脑:18080
```

原因：

```text
1. 用户电脑通常在 NAT 后面。
2. 公司网络、家庭网络、防火墙会阻断入站请求。
3. 本地 IP 不稳定。
4. 安全风险高。
```

---

### 5.2 推荐本地主动连接云端

推荐：

```text
local-backend 主动连接 cloud-backend
  ↓
注册设备
  ↓
心跳
  ↓
拉取任务
  ↓
执行任务
  ↓
回传结果
```

连接方式分阶段：

```text
Phase 1：HTTP Polling
Phase 2：WebSocket
Phase 3：WebSocket + 离线队列 + 断点恢复
```

---

## 6. 核心协议：Local Runner

### 6.1 Runner 注册

本地启动后向云端注册：

```http
POST /api/local-runners/register
```

请求：

```json
{
  "deviceId": "device_macbook_001",
  "userId": "user_001",
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

### 6.2 心跳

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
  "memoryUsageMb": 2048,
  "lastError": null
}
```

---

### 6.3 拉取任务

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

### 6.4 上报进度

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

### 6.5 完成任务

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
        "localPath": "/Users/wang/Library/Application Support/TangyingAIOS/projects/project_001/renders/final.mp4",
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

### 6.6 失败任务

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
    "details": "http://127.0.0.1:8787/health connection refused"
  },
  "diagnostics": {
    "stdout": "...",
    "stderr": "...",
    "logsRef": "local://projects/project_001/logs/local_job_001.log"
  },
  "retryable": true
}
```

---

## 7. ToolManifest 升级

### 7.1 增加执行面字段

当前工具 manifest 需要增加：

```yaml
executionPlane: cloud | local | remote_http | hybrid
requiresUserDevice: true | false
artifactLocation: cloud | local | both
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

### 7.2 示例：HyperFrames Renderer

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

riskLevel: medium
costLevel: medium
sideEffect: true
idempotent: false

approvalPolicy:
  required: true
  mode: before_execute
  reason: 视频渲染耗时较长并会产生大文件，需要用户确认。
```

---

### 7.3 示例：Script Generator

```yaml
name: video_script_generator
type: cloud_prompt_tool
executionPlane: cloud
requiresUserDevice: false
artifactLocation: cloud

capabilities:
  - script_generation
  - video_planning

riskLevel: low
costLevel: low
sideEffect: false
idempotent: true
```

---

### 7.4 示例：Seedance

```yaml
name: seedance_clip_generator
type: remote_http_tool
executionPlane: remote_http
requiresUserDevice: false
artifactLocation: both

provider: seedance

providerCapabilities:
  maxDurationSec: 15
  supportsImageReference: true
  supportsVideoReference: true

capabilities:
  - video_generation
  - broll_generation
```

---

## 8. NodeExecutor 改造

### 8.1 当前问题

现在 NodeExecutor 主要逻辑是：

```text
收到 node ready
  ↓
从 ToolRegistry 找工具
  ↓
直接执行工具
```

改造后必须变成：

```text
收到 node ready
  ↓
读取 ToolManifest.executionPlane
  ↓
cloud        → 云端执行
local        → 创建 LocalJob，等待本地执行
remote_http  → 云端调用外部 API
hybrid       → 根据策略拆分为 cloud/local/remote_http 子任务
```

---

### 8.2 节点状态扩展

新增状态：

```text
PENDING
READY
RUNNING
WAITING_LOCAL
LOCAL_CLAIMED
LOCAL_RUNNING
LOCAL_COMPLETED
LOCAL_FAILED
SUCCESS
FAILED
CANCELLED
```

---

### 8.3 Local Tool 执行逻辑

```text
NodeExecutor.ExecuteNode
  ↓
manifest.executionPlane == local
  ↓
检查用户是否有在线 local runner
  ↓
检查 runner capability 是否满足工具要求
  ↓
创建 local_job
  ↓
node.status = WAITING_LOCAL
  ↓
返回，不阻塞 Worker
  ↓
local-backend claim job
  ↓
local-backend complete job
  ↓
cloud localrunner handler 更新 node result
  ↓
orchestrator 推进下游节点
```

---

## 9. local-backend 改造

### 9.1 新增模块

```text
local-backend/internal/localrunner/
├── client.go
├── loop.go
├── heartbeat.go
├── job_claim.go
├── job_report.go
└── capability_probe.go

local-backend/internal/localtool/
├── registry.go
├── executor.go
├── commands.go
├── sandbox.go
├── path_guard.go
└── tools/
    ├── ffmpeg.go
    ├── hyperframes.go
    ├── hypergen.go
    ├── audio.go
    ├── asr.go
    ├── package.go
    └── filesystem.go
```

---

### 9.2 本地启动流程

```text
local-backend 启动
  ↓
读取本地配置
  ↓
检查 cloud api 地址
  ↓
检查用户 token
  ↓
检查 deviceId
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
开启 job claim loop
```

---

### 9.3 本地 Job Loop

```text
while local-backend running:
  send heartbeat
  claim job
  if no job:
    sleep 3s
  if job:
    validate command
    validate path
    execute local tool
    report progress
    complete or fail
```

---

## 10. 本地命令白名单

云端不能下发任意 shell，只能下发语义命令。

允许命令：

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
rm
curl arbitrary
python arbitrary
node arbitrary
```

如果未来需要视频创作命令，也必须通过沙箱和白名单参数模板，不允许直接拼 shell。

---

## 11. 路径安全设计

### 11.1 本地允许访问路径

```text
TangyingAIOS/projects/
TangyingAIOS/artifacts/
TangyingAIOS/cache/
TangyingAIOS/logs/
用户通过文件选择器显式授权的素材路径
```

---

### 11.2 禁止访问路径

```text
~/.ssh
~/.aws
~/.config
系统目录
任意父级目录 traversal
未授权用户目录
```

---

### 11.3 路径协议

统一使用：

```text
local://projects/project_001/renders/final.mp4
local://assets/imported/input.mp4
cloud://artifacts/script.json
```

本地执行前做解析：

```text
local:// → 转成本地 workspace 绝对路径
cloud:// → 需要先下载或读取云端 artifact
```

---

## 12. Artifact 策略

### 12.1 小文件同步云端

同步：

```text
JSON
Markdown
SRT
VTT
低清缩略图
诊断摘要
render_report
final_review
```

---

### 12.2 大文件默认本地保存

本地保存：

```text
原始 MP4
中间 MP4
最终 final.mp4
图片序列
音频中间文件
HyperFrames 项目目录
```

云端只保存 metadata：

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

### 12.3 用户选择后再上传云端

前端提供：

```text
同步到云端
导出本地文件
打开文件夹
生成诊断包
```

用户点击“同步到云端”后才上传 MinIO。

---

## 13. 视频生产链路改造

### 13.1 一句话生成视频

```text
用户输入
  ↓
cloud：Agent Planner
  ↓
cloud：script_generator
  ↓
cloud：shot_splitter
  ↓
cloud：render_strategy_planner
  ↓
cloud：用户审核 render_strategy
  ↓
remote_http：Seedance 生成 B-roll
  ↓
local：下载/接收素材到 workspace
  ↓
local：hyperframes_project_generator
  ↓
local：hyperframes_renderer
  ↓
local：final.mp4
  ↓
cloud：artifact metadata + final_review
```

---

### 13.2 用户上传 MP4 剪辑

```text
用户本地选择 MP4
  ↓
local：media_importer
  ↓
local：ffmpeg_probe
  ↓
local：audio_extractor
  ↓
local：asr_transcribe
  ↓
cloud：clip_planner
  ↓
local：ffmpeg_clip_extract
  ↓
cloud：overlay_designer
  ↓
local：hyperframes_project_generate
  ↓
local：hyperframes_render
  ↓
local：final.mp4
```

---

### 13.3 Seedance + HyperFrames 混合

```text
cloud：render_strategy_planner
  ↓
remote_http：seedance_clip_generator
  ↓
cloud/local：seedance_clip_qc
  ↓
local：保存 Seedance clip
  ↓
local：HyperFrames 统一合成字幕、卡片、BGM、转场
  ↓
local：final.mp4
```

---

## 14. 前端体验升级

### 14.1 本地执行器状态栏

显示：

```text
本地执行器：在线 / 离线
云端连接：正常 / 断开
FFmpeg：可用 / 不可用
HyperFrames：可用 / 不可用
Node：可用 / 不可用
本地工作区：可写 / 不可写
剩余磁盘：xxx GB
```

---

### 14.2 工具能力预检页

生成视频前显示：

```text
当前可用能力：
- 云端 LLM：可用
- HyperFrames 本地渲染：可用
- FFmpeg 本地处理：可用
- Seedance：已配置
- ASR：未配置
- TTS：未配置

推荐模式：
- 可生成图文口播视频
- 可上传 MP4 自动剪辑加字幕
- 可使用 Seedance 生成短 B-roll
```

---

### 14.3 Local Job 进度页

每个本地任务显示：

```text
任务：HyperFrames 渲染
状态：执行中
进度：43%
当前步骤：渲染帧
日志：...
输出：final.mp4
```

---

## 15. 断网和重连设计

### 15.1 云端断开时

local-backend 应：

```text
1. 继续保存本地项目和日志。
2. 暂停领取新任务。
3. 正在执行的本地任务可以继续执行。
4. 完成后进入 pending_upload 状态。
5. 云端恢复后自动上报结果。
```

---

### 15.2 本地退出时

cloud-backend 应：

```text
1. heartbeat 超时后 runner 标记 offline。
2. 正在执行的 local job 标记 runner_lost。
3. 允许用户重新连接后恢复。
4. 超时太久则任务进入 needs_retry。
```

---

### 15.3 幂等设计

LocalJob 必须有：

```text
jobId
nodeId
projectId
idempotencyKey
attempt
```

如果本地重复上报 complete，云端应幂等处理，不重复推进 DAG。

---

## 16. 数据库表设计

### 16.1 local_runners

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

### 16.2 local_jobs

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
    created_at TIMESTAMP NOT NULL,
    claimed_at TIMESTAMP,
    completed_at TIMESTAMP,
    updated_at TIMESTAMP NOT NULL
);
```

---

### 16.3 local_job_logs

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

## 17. 实施阶段

## Phase 1：Local Runner 接入云端

目标：让 cloud-backend 可以管理本地 runner。

任务：

```text
1. 接入 localrunner service 到 main.go。
2. 新增 localrunner HTTP handler。
3. 支持 register / heartbeat / claim / progress / complete / fail。
4. 新增 local_runners / local_jobs 表。
5. 前端显示本地 runner 在线状态。
```

验收：

```text
local-backend 启动后，cloud-backend 能看到 runner online。
```

---

## Phase 2：ToolManifest 执行面治理

目标：工具明确在哪执行。

任务：

```text
1. ToolManifest 增加 executionPlane。
2. ToolManifest 增加 localRequirements。
3. ToolManifest 增加 artifactLocation。
4. PlanGuard 检查 local tool 是否有在线 runner。
5. ToolRetriever 根据 runner capabilities 过滤工具。
```

验收：

```text
如果本地没有 FFmpeg，Planner 不应选择 ffmpeg 工具，或必须提示安装。
```

---

## Phase 3：NodeExecutor 支持 Local Dispatch

目标：云端 Worker 遇到 local tool 时不直接执行。

任务：

```text
1. NodeExecutor 根据 executionPlane 分发。
2. local tool 创建 LocalJob。
3. node 状态进入 WAITING_LOCAL。
4. local job complete 后推进 DAG。
5. local job fail 后节点失败或进入重试。
```

验收：

```text
hyperframes_renderer 不在 cloud-backend 执行，而是由 local-backend 执行。
```

---

## Phase 4：local-backend 实现工具执行

目标：local-backend 成为真正执行器。

任务：

```text
1. 实现 runner loop。
2. 实现 capability_probe。
3. 实现 command 白名单。
4. 实现 HYPERFRAMES_RENDER。
5. 实现 FFMPEG_PROBE。
6. 实现 FFMPEG_CLIP_EXTRACT。
7. 实现 ARTIFACT_PACKAGE。
8. 实现本地 job logs。
```

验收：

```text
服务器在云端，用户本地仍能渲染 final.mp4。
```

---

## Phase 5：视频工具迁移到本地执行

迁移顺序：

```text
1. ffmpeg_probe
2. audio_extractor
3. hyperframes_project_generator
4. hyperframes_renderer
5. video_package_exporter
6. asr_transcriber_local
7. local_media_indexer
```

保留云端工具：

```text
1. script_generator
2. shot_splitter
3. visual_plan_generator
4. render_strategy_planner
5. publish_copy_generator
6. fact_checker
```

---

## Phase 6：稳定性与恢复

任务：

```text
1. local job 重试机制。
2. runner offline 恢复。
3. pending upload 队列。
4. 本地任务断点恢复。
5. 诊断包导出。
6. 云端任务状态和本地任务状态一致性检查。
```

验收：

```text
渲染过程中断网，网络恢复后能继续上报任务结果。
```

---

## 18. 关键验收用例

### 用例 1：云端部署 + 本地渲染

环境：

```text
cloud-backend 部署在云服务器
local-backend 运行在用户 Mac
HyperFrames Render Service 运行在用户 Mac
```

输入：

```text
请生成一个 30 秒知识分享视频。
```

验收：

```text
1. cloud 生成 AgentPlan。
2. cloud 生成 LocalJob。
3. local claim HYPERFRAMES_RENDER。
4. final.mp4 生成在用户本地。
5. cloud 收到 artifact metadata。
6. 前端可以预览本地 final.mp4。
```

---

### 用例 2：用户上传 MP4

输入：

```text
用户选择本地 input.mp4，要求加字幕和知识卡片。
```

验收：

```text
1. input.mp4 不强制上传云端。
2. local 执行 ffprobe。
3. local 提取音频。
4. local 或 cloud 生成字幕。
5. local HyperFrames 合成字幕和卡片。
6. final.mp4 本地生成。
```

---

### 用例 3：本地执行器离线

操作：

```text
关闭 local-backend。
```

验收：

```text
1. cloud 显示 runner offline。
2. Planner 不选择 local-only 工具，或提示用户启动本地执行器。
3. 已等待 local 的节点保持 WAITING_LOCAL 或转 needs_runner。
```

---

### 用例 4：断网恢复

操作：

```text
local 执行渲染过程中断网，渲染完成后恢复网络。
```

验收：

```text
1. 本地继续完成渲染。
2. 结果进入 pending_report。
3. 网络恢复后自动 complete。
4. cloud DAG 正常推进。
```

---

## 19. 给 Coding Agent 的任务指令

```text
目标：
实施 AIOS EdgeRun 升级，确保 cloud-backend 真实部署到云端后，local-backend 可以稳定执行本地工具。系统必须从“云端直接执行工具”升级为“云端控制面 + 本地执行面”。

P0：
1. 新增 local_runners / local_jobs / local_job_logs 表。
2. 接入 localrunner service 到 cloud-backend main.go。
3. 新增 localrunner HTTP API：
   - POST /api/local-runners/register
   - POST /api/local-runners/{runnerId}/heartbeat
   - GET /api/local-runners/{runnerId}/jobs/claim
   - POST /api/local-jobs/{jobId}/progress
   - POST /api/local-jobs/{jobId}/complete
   - POST /api/local-jobs/{jobId}/fail
4. ToolManifest 增加 executionPlane、localRequirements、artifactLocation、requiresUserDevice。
5. NodeExecutor 读取 executionPlane。
6. executionPlane=local 时创建 LocalJob，不在 cloud Worker 直接执行。
7. local job complete 后更新 node result 并推进 DAG。

P1：
1. local-backend 新增 localrunner client。
2. local-backend 启动时注册 runner。
3. local-backend 定时 heartbeat。
4. local-backend 实现 claim loop。
5. local-backend 实现 capability probe。
6. capability 包含 ffmpeg、node、hyperframes、hypergen、workspace 可写性。
7. 本地执行结果回传 cloud。

P2：
1. local-backend 实现 command 白名单。
2. 实现 HYPERFRAMES_RENDER。
3. 实现 HYPERFRAMES_PROJECT_GENERATE。
4. 实现 FFMPEG_PROBE。
5. 实现 FFMPEG_CLIP_EXTRACT。
6. 实现 AUDIO_EXTRACT。
7. 实现 ARTIFACT_PACKAGE。
8. 所有本地命令必须经过路径校验。

P3：
1. 迁移 hyperframes_renderer 到 executionPlane=local。
2. 迁移 hyperframes_project_generator 到 executionPlane=local。
3. 迁移 video_package_exporter 到 executionPlane=local。
4. 迁移 ffmpeg/audio 类工具到 executionPlane=local。
5. 保留脚本/分镜/策略类工具在 cloud。

P4：
1. 前端增加本地执行器状态面板。
2. 显示本地工具能力。
3. local tool 不可用时，提示用户启动 local-backend 或安装依赖。
4. 支持 final.mp4 本地预览。
5. 支持用户手动同步 final.mp4 到云端。

P5：
1. 加入断网恢复。
2. 加入 local job 幂等处理。
3. 加入 runner offline 恢复。
4. 加入 pending_report 队列。
5. 加入本地诊断包导出。
```

---

## 20. 最终结论

你必须把系统升级成：

```text
AIOS EdgeRun
= 云端控制面 + 本地执行面
```

最终稳定架构应该是：

```text
cloud-backend：
  负责 Agent、DAG、LLM、审核、任务状态、工具注册、LocalJob 管理

local-backend：
  负责 FFmpeg、HyperFrames、Hypergen、本地文件、本地素材、本地渲染、本地 artifact

frontend：
  负责用户交互、本地状态展示、审核、预览、文件授权
```

关键原则：

```text
1. 云端不直接执行用户本地 CLI。
2. 云端不直接访问用户本地文件。
3. 本地不接收入站公网请求。
4. 本地主动连接云端。
5. 云端只下发语义工具任务。
6. 本地只执行白名单命令。
7. 大文件默认 local-only。
8. 小文件和 metadata 同步云端。
9. 所有本地任务可追踪、可重试、可诊断。
```

一句话总结：

**只有完成 AIOS EdgeRun，服务器后端真实部署到云端后，你的本地视频生成、剪辑、字幕、HyperFrames 渲染和 CLI 工具链才能稳定运行。**
