# HyperFrames 无 CLI 接入 AIOS 的完整优化方案

目标：**让 AIOS Agent 不再执行 `npx hyperframes render`，而是通过一个本地/云端 HyperFrames Render Service 直接调用 HyperFrames 的 Node API 完成 lint、snapshot、render、artifact 保存和进度回传。**

HyperFrames 官方不是只有 CLI。`@hyperframes/producer` 明确用于在 Node.js 后端或 CI 中**程序化渲染 MP4 / WebM**，它提供 `createRenderJob` 和 `executeRenderJob` 这类 API；CLI 只是“命令行最快路径”，不是唯一接入方式。([hyperframes.mintlify.app][1])
HyperFrames 仓库结构也说明了 `producer` 是完整渲染管线，`engine` 负责 Puppeteer + FFmpeg，`core` 负责类型、parser、linter、runtime 等。([GitHub][2])

---

# 1. 推荐最终架构

```text id="fwhtvr"
AIOS Agent Runtime
  ↓
LLM Planner
  ↓
AgentPlan
  ↓
PlanGuard
  ↓
PlanCompiler
  ↓
Orchestrator / Worker
  ↓
HyperFrames Tools
  ├── hyperframes_project_generator
  ├── hyperframes_linter
  ├── hyperframes_snapshot
  └── hyperframes_renderer
        ↓ HTTP / SSE
HyperFrames Render Service
  ↓
@hyperframes/core
@hyperframes/producer
  ↓
Chromium + FFmpeg
  ↓
MP4 / PNG snapshots / render report
  ↓
ArtifactStore
```

关键变化：

```text id="2whc70"
旧方案：
Go Worker → exec.Command("npx", "hyperframes", "render")

新方案：
Go Worker → HTTP/SSE → HyperFrames Render Service → @hyperframes/producer
```

这样可以避免：

```text id="4q94c0"
1. 对 npx / 全局 CLI 的运行时依赖
2. stdout/stderr 解析不稳定
3. 进度不可控
4. 取消任务困难
5. 并发渲染难管理
6. AgentRun trace 难打通
```

---

# 2. 第一版技术路线

## 2.1 不直接把 HyperFrames 写进 Go

HyperFrames 是 Node / TypeScript 生态。你的 AIOS 后端是 Go。最稳方案是：

```text id="lnsz67"
Go 负责 Agent 调度
Node 负责 HyperFrames 渲染
二者通过 HTTP / SSE 通信
```

不要做：

```text id="3q33gm"
Go import HyperFrames
```

也不要继续做：

```text id="ag3r6a"
Go shell 调 npx hyperframes render
```

---

## 2.2 新增独立服务

新增目录：

```text id="gwnd7n"
hyperframes-render-service/
├── package.json
├── tsconfig.json
├── src/
│   ├── server.ts
│   ├── render.ts
│   ├── lint.ts
│   ├── snapshot.ts
│   ├── storage.ts
│   ├── security.ts
│   ├── types.ts
│   └── queue.ts
├── Dockerfile
└── README.md
```

运行依赖：

```text id="vy60bu"
Node.js 22+
Chromium / Chrome
FFmpeg
@hyperframes/core
@hyperframes/producer
```

HyperFrames README 明确 Quick Start 里手动使用需要 Node.js 22+ 和 FFmpeg；服务化后仍然需要这些底层依赖，因为渲染管线本身依赖浏览器和 FFmpeg。([GitHub][3])

---

# 3. HyperFrames Render Service API

## 3.1 健康检查

```http id="s7jalx"
GET /health
```

返回：

```json id="qswn2s"
{
  "ok": true,
  "service": "hyperframes-render-service",
  "version": "1.0.0",
  "dependencies": {
    "node": "22.x",
    "ffmpeg": true,
    "chromium": true,
    "producer": true
  }
}
```

作用：

```text id="2u77vk"
1. AIOS 启动时探测服务是否可用
2. 前端诊断页显示 HyperFrames 状态
3. render 前做 readiness check
```

---

## 3.2 项目检查

```http id="b1k9yb"
POST /lint
```

请求：

```json id="a8r8x5"
{
  "projectDir": "/data/aios/projects/agent_run_001/hyperframes",
  "entry": "index.html"
}
```

返回：

```json id="yrd21i"
{
  "ok": true,
  "errors": [],
  "warnings": [],
  "entry": "index.html",
  "durationMs": 235
}
```

说明：官方文档中 `@hyperframes/producer` 页面提到 lint / parse composition HTML 应使用 `@hyperframes/core`，所以 lint 能力应在服务中封装成 `/lint`，而不是让 Go 调 CLI。([hyperframes.mintlify.app][1])

---

## 3.3 关键帧快照

```http id="7hx3aq"
POST /snapshot
```

请求：

```json id="s0fyvq"
{
  "projectDir": "/data/aios/projects/agent_run_001/hyperframes",
  "entry": "index.html",
  "times": [0, 3, 6, 9],
  "outputDir": "/data/aios/projects/agent_run_001/snapshots"
}
```

返回：

```json id="jzpl0t"
{
  "ok": true,
  "snapshots": [
    {
      "timeSec": 0,
      "path": "/data/aios/projects/agent_run_001/snapshots/0000.png"
    },
    {
      "timeSec": 3,
      "path": "/data/aios/projects/agent_run_001/snapshots/0003.png"
    }
  ]
}
```

第一版如果 snapshot API 实现成本高，可以先做：

```text id="5yhdkn"
生成 HyperFrames 项目
  ↓
lint
  ↓
render draft 短预览
  ↓
用户审核
  ↓
render final
```

但更推荐有 snapshot，因为它能降低渲染成本。

---

## 3.4 渲染视频

```http id="bbvo3b"
POST /render
```

请求：

```json id="5log3m"
{
  "projectDir": "/data/aios/projects/agent_run_001/hyperframes",
  "entry": "index.html",
  "outputPath": "/data/aios/projects/agent_run_001/output/final.mp4",
  "fps": 30,
  "quality": "standard",
  "format": "mp4",
  "workers": 4,
  "useGpu": false
}
```

返回：

```json id="1eikza"
{
  "ok": true,
  "jobId": "render_job_001",
  "outputPath": "/data/aios/projects/agent_run_001/output/final.mp4",
  "format": "mp4",
  "durationMs": 126000,
  "artifact": {
    "kind": "VIDEO",
    "name": "final.mp4",
    "storageRef": "local://agent_run_001/output/final.mp4"
  }
}
```

内部用：

```ts id="x2p8cd"
import { createRenderJob, executeRenderJob } from "@hyperframes/producer";

const job = createRenderJob({
  fps: 30,
  quality: "standard",
  format: "mp4",
  workers: 4,
  useGpu: false,
});

await executeRenderJob(job, projectDir, outputPath);
```

官方 `@hyperframes/producer` 文档给出的程序化用法就是先 `createRenderJob`，再 `executeRenderJob(job, './my-video', './output.mp4')`。([hyperframes.mintlify.app][1])

---

## 3.5 流式渲染进度

```http id="lcs99w"
POST /render/stream
```

返回 SSE：

```text id="hd8nvh"
event: status
data: {"jobId":"render_job_001","status":"queued","progress":0}

event: status
data: {"jobId":"render_job_001","status":"rendering","progress":42}

event: status
data: {"jobId":"render_job_001","status":"encoding","progress":88}

event: complete
data: {"jobId":"render_job_001","outputPath":"/data/.../final.mp4"}
```

如果 `@hyperframes/producer` 的当前版本不能直接提供细粒度 progress callback，就先实现粗粒度状态：

```text id="xdrb8u"
queued
preparing
rendering
encoding
complete
failed
```

不要强依赖 CLI stdout。

---

# 4. Node 服务核心实现

## 4.1 `package.json`

```json id="j2tw4k"
{
  "name": "hyperframes-render-service",
  "version": "1.0.0",
  "type": "module",
  "scripts": {
    "dev": "tsx src/server.ts",
    "build": "tsc",
    "start": "node dist/server.js"
  },
  "dependencies": {
    "@hyperframes/core": "latest",
    "@hyperframes/producer": "latest",
    "fastify": "^5.0.0",
    "zod": "^3.23.8",
    "nanoid": "^5.0.0"
  },
  "devDependencies": {
    "tsx": "^4.19.0",
    "typescript": "^5.6.0",
    "@types/node": "^22.0.0"
  }
}
```

---

## 4.2 `src/types.ts`

```ts id="pbi0il"
export type RenderQuality = "draft" | "standard" | "high";
export type RenderFormat = "mp4" | "webm" | "mov" | "png-sequence";

export interface RenderRequest {
  projectDir: string;
  entry?: string;
  outputPath: string;
  fps?: number;
  quality?: RenderQuality;
  format?: RenderFormat;
  workers?: number;
  useGpu?: boolean;
}

export interface RenderResult {
  ok: boolean;
  jobId: string;
  outputPath?: string;
  durationMs?: number;
  error?: string;
}

export interface LintRequest {
  projectDir: string;
  entry?: string;
}

export interface SnapshotRequest {
  projectDir: string;
  entry?: string;
  times: number[];
  outputDir: string;
}
```

---

## 4.3 `src/security.ts`

```ts id="gzmafl"
import path from "node:path";

export interface SecurityConfig {
  allowedProjectRoots: string[];
  allowedOutputRoots: string[];
}

export function assertPathAllowed(targetPath: string, allowedRoots: string[], label: string) {
  const resolved = path.resolve(targetPath);

  const allowed = allowedRoots.some((root) => {
    const resolvedRoot = path.resolve(root);
    return resolved === resolvedRoot || resolved.startsWith(resolvedRoot + path.sep);
  });

  if (!allowed) {
    throw new Error(`${label} path is not allowed: ${targetPath}`);
  }

  return resolved;
}
```

必须做路径限制。HyperFrames 渲染 HTML，本质上会执行浏览器环境，输入 projectDir 和输出目录不能让用户任意指定系统路径。

---

## 4.4 `src/render.ts`

```ts id="k2jfoe"
import { createRenderJob, executeRenderJob } from "@hyperframes/producer";
import { nanoid } from "nanoid";
import { assertPathAllowed, SecurityConfig } from "./security.js";
import { RenderRequest, RenderResult } from "./types.js";

export async function renderVideo(
  req: RenderRequest,
  security: SecurityConfig,
): Promise<RenderResult> {
  const startedAt = Date.now();
  const jobId = `render_${nanoid(12)}`;

  try {
    const projectDir = assertPathAllowed(req.projectDir, security.allowedProjectRoots, "projectDir");
    const outputPath = assertPathAllowed(req.outputPath, security.allowedOutputRoots, "outputPath");

    const job = createRenderJob({
      fps: req.fps ?? 30,
      quality: req.quality ?? "standard",
      format: req.format ?? "mp4",
      workers: Math.min(Math.max(req.workers ?? 2, 1), 8),
      useGpu: req.useGpu ?? false,
      debug: false
    });

    await executeRenderJob(job, projectDir, outputPath);

    return {
      ok: true,
      jobId,
      outputPath,
      durationMs: Date.now() - startedAt
    };
  } catch (err) {
    return {
      ok: false,
      jobId,
      error: err instanceof Error ? err.message : String(err),
      durationMs: Date.now() - startedAt
    };
  }
}
```

---

## 4.5 `src/server.ts`

```ts id="4km0il"
import Fastify from "fastify";
import { z } from "zod";
import { renderVideo } from "./render.js";

const app = Fastify({ logger: true });

const security = {
  allowedProjectRoots: [
    process.env.HYPERFRAMES_PROJECT_ROOT ?? "/data/aios/projects"
  ],
  allowedOutputRoots: [
    process.env.HYPERFRAMES_OUTPUT_ROOT ?? "/data/aios/projects"
  ]
};

const renderSchema = z.object({
  projectDir: z.string(),
  entry: z.string().optional(),
  outputPath: z.string(),
  fps: z.number().int().min(1).max(120).optional(),
  quality: z.enum(["draft", "standard", "high"]).optional(),
  format: z.enum(["mp4", "webm", "mov", "png-sequence"]).optional(),
  workers: z.number().int().min(1).max(8).optional(),
  useGpu: z.boolean().optional()
});

app.get("/health", async () => {
  return {
    ok: true,
    service: "hyperframes-render-service",
    version: "1.0.0"
  };
});

app.post("/render", async (request, reply) => {
  const parsed = renderSchema.safeParse(request.body);
  if (!parsed.success) {
    return reply.code(400).send({
      ok: false,
      error: parsed.error.flatten()
    });
  }

  const result = await renderVideo(parsed.data, security);
  return reply.code(result.ok ? 200 : 500).send(result);
});

const port = Number(process.env.PORT ?? 8787);
const host = process.env.HOST ?? "127.0.0.1";

app.listen({ port, host });
```

第一版先把 `/render` 跑通；`/lint`、`/snapshot`、`/render/stream` 可逐步补齐。

---

# 5. Go 后端接入

## 5.1 新增配置

在你的 AIOS 配置中增加：

```yaml id="kbepf7"
hyperframes:
  mode: service # disabled / service
  service_url: http://127.0.0.1:8787
  timeout_sec: 1800
  default_fps: 30
  default_quality: standard
  default_format: mp4
  max_workers: 4
  use_gpu: false
  project_root: /data/aios/projects
  output_root: /data/aios/projects
```

当前阶段建议：

```text id="wv9e4k"
mode 只支持 disabled / service
不要把 cli 作为默认 fallback
```

因为你的目标是“不要 CLI”。

---

## 5.2 新增 Go Client

新增：

```text id="gvc0g2"
cloud-backend/internal/core/hyperframes/
├── client.go
├── types.go
└── config.go
```

### `types.go`

```go id="kntm96"
package hyperframes

type RenderRequest struct {
    ProjectDir string `json:"projectDir"`
    Entry      string `json:"entry,omitempty"`
    OutputPath string `json:"outputPath"`
    FPS        int    `json:"fps,omitempty"`
    Quality    string `json:"quality,omitempty"`
    Format     string `json:"format,omitempty"`
    Workers    int    `json:"workers,omitempty"`
    UseGPU     bool   `json:"useGpu,omitempty"`
}

type RenderResult struct {
    OK         bool   `json:"ok"`
    JobID      string `json:"jobId"`
    OutputPath string `json:"outputPath,omitempty"`
    DurationMs int64  `json:"durationMs,omitempty"`
    Error      string `json:"error,omitempty"`
}

type HealthResult struct {
    OK      bool   `json:"ok"`
    Service string `json:"service"`
    Version string `json:"version"`
}
```

### `client.go`

```go id="7nskai"
package hyperframes

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    "time"
)

type Client struct {
    baseURL    string
    httpClient *http.Client
}

func NewClient(baseURL string, timeout time.Duration) *Client {
    return &Client{
        baseURL: strings.TrimRight(baseURL, "/"),
        httpClient: &http.Client{
            Timeout: timeout,
        },
    }
}

func (c *Client) Health(ctx context.Context) (*HealthResult, error) {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
    if err != nil {
        return nil, err
    }

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var result HealthResult
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }

    if resp.StatusCode >= 300 || !result.OK {
        return nil, fmt.Errorf("hyperframes health check failed: status=%d", resp.StatusCode)
    }

    return &result, nil
}

func (c *Client) Render(ctx context.Context, reqBody RenderRequest) (*RenderResult, error) {
    payload, err := json.Marshal(reqBody)
    if err != nil {
        return nil, err
    }

    req, err := http.NewRequestWithContext(
        ctx,
        http.MethodPost,
        c.baseURL+"/render",
        bytes.NewReader(payload),
    )
    if err != nil {
        return nil, err
    }
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var result RenderResult
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }

    if resp.StatusCode >= 300 || !result.OK {
        if result.Error != "" {
            return &result, fmt.Errorf("hyperframes render failed: %s", result.Error)
        }
        return &result, fmt.Errorf("hyperframes render failed: status=%d", resp.StatusCode)
    }

    return &result, nil
}
```

---

# 6. 修改 AIOS 工具层

## 6.1 新增工具

建议新增 4 个工具：

```text id="f9x2r3"
hyperframes_project_generator
hyperframes_linter
hyperframes_snapshot
hyperframes_renderer
```

第一阶段必须做：

```text id="w2hmnq"
hyperframes_project_generator
hyperframes_renderer
```

第二阶段补：

```text id="i6my7q"
hyperframes_linter
hyperframes_snapshot
```

---

## 6.2 `hyperframes_project_generator`

职责：把 Agent 生成的口播稿、分镜、视频 Prompt 转成 HyperFrames 项目目录。

输入：

```json id="4tfiax"
{
  "topic": "端午节的来历",
  "script": "...",
  "shotList": [],
  "videoPrompts": [],
  "publishCopy": {},
  "style": "16:9 非写实动画，中文知识分享"
}
```

输出：

```json id="d3m8p7"
{
  "projectDir": "/data/aios/projects/agent_run_001/hyperframes",
  "entry": "index.html",
  "files": [
    "index.html",
    "assets/style.css",
    "assets/data.json"
  ],
  "summary": "已生成 HyperFrames 项目。"
}
```

第一版不要追求复杂视频画面。可以生成“口播知识分享模板”：

```text id="vxu78l"
1. 标题页
2. 知识点卡片
3. 时间线页
4. 关键概念页
5. 总结页
6. 字幕/旁白同步
```

---

## 6.3 `hyperframes_renderer`

职责：调用 HyperFrames Render Service。

输入：

```json id="a8vfl8"
{
  "projectDir": "{{hyperframes_project.output.projectDir}}",
  "entry": "{{hyperframes_project.output.entry}}",
  "format": "mp4",
  "quality": "standard",
  "fps": 30
}
```

输出：

```json id="crpg5e"
{
  "outputPath": "/data/aios/projects/agent_run_001/output/final.mp4",
  "artifact": {
    "kind": "VIDEO",
    "name": "final.mp4",
    "storageRef": "local://agent_run_001/output/final.mp4"
  },
  "summary": "视频渲染完成。"
}
```

---

## 6.4 替换旧 CLI 逻辑

在你当前的 `executeHyperframesRenderer()` 位置，做成：

```go id="dcp3gx"
func executeHyperframesRenderer(ctx context.Context, input map[string]interface{}) (*tool.ToolResult, error) {
    if cfg.HyperFrames.Mode != "service" {
        return nil, fmt.Errorf("hyperframes service mode is disabled")
    }

    client := hyperframes.NewClient(
        cfg.HyperFrames.ServiceURL,
        time.Duration(cfg.HyperFrames.TimeoutSec)*time.Second,
    )

    projectDir := getString(input, "projectDir")
    entry := getStringDefault(input, "entry", "index.html")

    outputPath := buildOutputPath(input)

    result, err := client.Render(ctx, hyperframes.RenderRequest{
        ProjectDir: projectDir,
        Entry:      entry,
        OutputPath: outputPath,
        FPS:        cfg.HyperFrames.DefaultFPS,
        Quality:    cfg.HyperFrames.DefaultQuality,
        Format:     cfg.HyperFrames.DefaultFormat,
        Workers:    cfg.HyperFrames.MaxWorkers,
        UseGPU:     cfg.HyperFrames.UseGPU,
    })
    if err != nil {
        return nil, err
    }

    return &tool.ToolResult{
        Success: true,
        Data: map[string]interface{}{
            "jobId": result.JobID,
            "outputPath": result.OutputPath,
            "artifact": map[string]interface{}{
                "kind": "VIDEO",
                "name": "final.mp4",
                "storageRef": toStorageRef(result.OutputPath),
            },
            "summary": "HyperFrames video rendered successfully.",
        },
    }, nil
}
```

关键要求：

```text id="5g41aw"
不再调用 exec.Command("npx", ...)
不再检测 hyperframes CLI
不再返回“请手动运行 CLI”的 guidance 作为成功结果
```

如果服务不可用，工具应返回失败，并在前端显示：

```text id="7v4kgh"
HyperFrames Render Service 未启动，请检查本地渲染服务。
```

---

# 7. ToolManifest 设计

## 7.1 `hyperframes_project_generator.tool.yaml`

```yaml id="u3ad0i"
name: hyperframes_project_generator
description: 根据口播稿、分镜和视频 Prompt 生成 HyperFrames HTML 视频项目目录。
type: builtin
version: 1.0.0

capabilities:
  - video_creation
  - hyperframes_project
  - html_video_composition

tags:
  - hyperframes
  - html
  - video
  - project_generation

parameters:
  topic:
    type: string
    required: true
  script:
    type: string
    required: true
  shotList:
    type: array
    required: true
  videoPrompts:
    type: array
    required: false
  style:
    type: string
    required: false

output:
  projectDir:
    type: string
  entry:
    type: string
  files:
    type: array
  summary:
    type: string

costLevel: low
riskLevel: low
sideEffect: false
idempotent: true

approvalPolicy:
  required: true
  mode: after_artifact
  blocksDownstream: true
  reason: HyperFrames 项目会作为最终视频渲染输入，建议审核后再渲染。
  reviewArtifactKinds:
    - HTML
    - JSON

artifactPolicy:
  produceArtifact: true
  artifactKinds:
    - HTML
    - JSON
  defaultReviewRequired: true

nextRecommendedTools:
  - hyperframes_linter
  - hyperframes_snapshot
  - hyperframes_renderer
```

---

## 7.2 `hyperframes_renderer.tool.yaml`

```yaml id="yyk53s"
name: hyperframes_renderer
description: 调用 HyperFrames Render Service 将 HyperFrames HTML 项目渲染为 MP4 视频文件，不依赖 CLI。
type: http_service
version: 1.0.0

capabilities:
  - video_creation
  - hyperframes_render
  - html_to_video
  - mp4_render

tags:
  - hyperframes
  - render
  - mp4
  - final_video

parameters:
  projectDir:
    type: string
    required: true
  entry:
    type: string
    required: false
    default: index.html
  fps:
    type: number
    required: false
    default: 30
  quality:
    type: string
    required: false
    default: standard
  format:
    type: string
    required: false
    default: mp4

output:
  outputPath:
    type: string
  artifact:
    type: object
  summary:
    type: string

costLevel: high
riskLevel: medium
sideEffect: true
idempotent: false

approvalPolicy:
  required: true
  mode: before_execute
  blocksDownstream: true
  reason: 渲染视频耗时较长且会产生大文件，必须用户确认后执行。
  reviewArtifactKinds:
    - HTML
    - SNAPSHOT
    - VIDEO

artifactPolicy:
  produceArtifact: true
  artifactKinds:
    - VIDEO
  defaultReviewRequired: true

nextRecommendedTools:
  - video_package_exporter
  - package_quality_checker
```

---

# 8. AgentPlan 编排更新

原链路：

```text id="n96gui"
video_prompt_generator
  ↓
video_package_exporter
```

升级为：

```text id="0z99vb"
video_prompt_generator
  ↓
hyperframes_project_generator
  ↓
hyperframes_linter
  ↓
hyperframes_snapshot
  ↓
用户审核
  ↓
hyperframes_renderer
  ↓
video_package_exporter
```

第一阶段可以简化为：

```text id="t2wfew"
video_prompt_generator
  ↓
hyperframes_project_generator
  ↓
用户审核
  ↓
hyperframes_renderer
  ↓
video_package_exporter
```

推荐 LLM Planner 输出：

```json id="ymoo9t"
{
  "id": "hyperframes_project",
  "intent": "生成 HyperFrames HTML 视频项目",
  "tool": "hyperframes_project_generator",
  "dependsOn": ["script_generation", "shot_split", "video_prompt_generation"],
  "arguments": {
    "topic": "端午节的来历",
    "script": "{{script_generation.output.script}}",
    "shotList": "{{shot_split.output.shotList}}",
    "videoPrompts": "{{video_prompt_generation.output.videoPrompts}}",
    "style": "中文知识分享视频，16:9，非写实动画"
  },
  "expectedOutput": ["projectDir", "entry"],
  "produceArtifact": true
}
```

然后：

```json id="lct5fk"
{
  "id": "hyperframes_render",
  "intent": "将 HyperFrames 项目渲染为 MP4 视频",
  "tool": "hyperframes_renderer",
  "dependsOn": ["hyperframes_project"],
  "arguments": {
    "projectDir": "{{hyperframes_project.output.projectDir}}",
    "entry": "{{hyperframes_project.output.entry}}",
    "fps": 30,
    "quality": "standard",
    "format": "mp4"
  },
  "expectedOutput": ["outputPath", "artifact"],
  "produceArtifact": true
}
```

---

# 9. 项目生成器第一版模板

`hyperframes_project_generator` 不要一开始追求复杂的电影生成。先做稳定模板。

## 9.1 生成目录

```text id="h5kayr"
/data/aios/projects/{agentRunId}/hyperframes/
├── index.html
├── assets/
│   ├── data.json
│   └── style.css
└── manifest.json
```

## 9.2 `data.json`

```json id="jlvj8r"
{
  "topic": "端午节的来历",
  "script": "...",
  "shots": [
    {
      "shotId": "SHOT_01",
      "durationSec": 6,
      "scriptText": "...",
      "visual": "...",
      "camera": "...",
      "transitionIn": "...",
      "transitionOut": "..."
    }
  ],
  "style": {
    "aspectRatio": "16:9",
    "language": "zh-CN",
    "visualStyle": "非写实动画，去AI感"
  }
}
```

## 9.3 `index.html` 核心要求

HyperFrames 仓库的规范提到 composition 是 HTML 文件，clip 需要 `class="clip"`，GSAP timelines 应该 pause 并注册到 `window.__timelines`，且渲染要避免 `Date.now()` 和未 seeded 的 `Math.random()` 这类非确定性行为。([GitHub][2])

所以模板中必须避免：

```text id="kysjku"
Date.now()
Math.random()
运行时网络请求
不确定动画
```

第一版建议用 CSS/GSAP 固定时间轴，减少动态依赖。

---

# 10. 安全与稳定策略

## 10.1 路径白名单

服务只允许访问：

```text id="b0lgoc"
AIOS_PROJECT_ROOT
AIOS_OUTPUT_ROOT
```

禁止：

```text id="fny9a4"
../../
/etc
用户任意绝对路径
外部网络资源
```

---

## 10.2 渲染前必须 lint

第一版可以先把 lint 作为 warning。第二版必须阻断：

```text id="t63mxm"
hyperframes_project_generator
  ↓
hyperframes_linter
  ↓
lint ok 才 render
```

---

## 10.3 渲染前必须用户确认

`hyperframes_renderer` 是高成本工具：

```yaml id="l57o94"
approvalPolicy:
  required: true
  mode: before_execute
```

必须让用户确认：

```text id="5ozd1a"
1. 项目 HTML 已生成
2. 分镜内容认可
3. 渲染参数认可
4. 预计会消耗本地 CPU/GPU 时间
```

---

# 11. 本地部署方式

## 11.1 开发模式

```bash id="wsknwe"
cd hyperframes-render-service
npm install
npm run dev
```

AIOS 配置：

```yaml id="e5pp5l"
hyperframes:
  mode: service
  service_url: http://127.0.0.1:8787
```

---

## 11.2 Electron 桌面集成

Electron 启动时：

```text id="7971wq"
1. 检查 local-backend
2. 检查 hyperframes-render-service
3. GET /health
4. 如果未启动，则启动 bundled Node service
5. 如果启动失败，显示诊断提示
```

不要让用户自己安装 CLI。

---

## 11.3 Docker 模式

`Dockerfile`：

```dockerfile id="ouoio9"
FROM node:22-bookworm

RUN apt-get update && apt-get install -y \
    ffmpeg \
    chromium \
    fonts-noto-cjk \
    fonts-noto-color-emoji \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY package.json package-lock.json* ./
RUN npm install

COPY tsconfig.json ./
COPY src ./src

RUN npm run build

ENV PORT=8787
ENV HOST=0.0.0.0
ENV HYPERFRAMES_PROJECT_ROOT=/data/aios/projects
ENV HYPERFRAMES_OUTPUT_ROOT=/data/aios/projects

EXPOSE 8787

CMD ["npm", "run", "start"]
```

---

# 12. 端到端验收

输入：

```text id="ee7pij"
请帮我根据端午节的来历创作一个口播知识分享视频。
```

必须验证：

```text id="h9hdsp"
1. AgentPlan 包含 hyperframes_project_generator
2. AgentPlan 包含 hyperframes_renderer
3. hyperframes_project_generator 输出 projectDir / entry
4. hyperframes_renderer 调用 HTTP Service
5. 不出现 npx hyperframes render
6. 不出现 exec.Command("hyperframes")
7. Render Service 输出 final.mp4
8. AIOS 保存 VIDEO artifact
9. final.mp4 出现在最终视频创作包里
10. trace 中能看到 render jobId / outputPath / durationMs
```

---

# 13. 给 Coding Agent 的完整任务指令

```text id="segg5q"
目标：
将 HyperFrames 接入 tangying-ai-operation-system，当前阶段不再依赖 CLI，不再调用 npx hyperframes render。通过独立 Node/TypeScript HyperFrames Render Service 调用 @hyperframes/producer 完成渲染，AIOS Go 后端通过 HTTP/SSE 调用该服务。

任务 1：新增 hyperframes-render-service
- 在仓库根目录新增 hyperframes-render-service。
- 使用 Node.js 22 + TypeScript + Fastify。
- 依赖 @hyperframes/core、@hyperframes/producer。
- 实现 GET /health。
- 实现 POST /render。
- 预留 POST /lint、POST /snapshot、POST /render/stream。
- POST /render 内部使用 createRenderJob 和 executeRenderJob。
- 增加 projectDir / outputPath 白名单校验。
- 禁止访问白名单之外路径。
- 输出 final.mp4 到指定 outputPath。

任务 2：新增 Go HyperFrames Client
- 新增 cloud-backend/internal/core/hyperframes/client.go。
- 实现 Health(ctx)。
- 实现 Render(ctx, RenderRequest)。
- 支持 service_url、timeout、fps、quality、format、workers、useGpu 配置。

任务 3：新增配置
- 在 cloud-backend 配置中增加 hyperframes 配置段：
  mode: service / disabled
  service_url
  timeout_sec
  default_fps
  default_quality
  default_format
  max_workers
  use_gpu
  project_root
  output_root

任务 4：改造 video_creation_external_tools.go
- 移除或禁用 exec.Command("npx", "hyperframes", "render") 路径。
- executeHyperframesRenderer 改为调用 HyperFramesClient.Render。
- 服务不可用时返回明确失败。
- 渲染成功后返回 outputPath 和 VIDEO artifact。
- 不再把“请手动运行 CLI”作为成功结果。

任务 5：新增 hyperframes_project_generator
- 根据 topic、script、shotList、videoPrompts 生成 HyperFrames 项目目录。
- 输出 index.html、assets/data.json、manifest.json。
- 生成的 HTML 必须避免 Date.now、Math.random 和运行时网络请求。
- 输出 projectDir、entry、files、summary。
- 该工具需要 artifactPolicy 和 approvalPolicy。

任务 6：注册 ToolManifest
- 新增 hyperframes_project_generator.tool.yaml。
- 新增 hyperframes_renderer.tool.yaml。
- hyperframes_renderer 设置 sideEffect=true、costLevel=high、approvalPolicy.mode=before_execute。
- nextRecommendedTools 中 video_prompt_generator 后推荐 hyperframes_project_generator。
- hyperframes_project_generator 后推荐 hyperframes_renderer。

任务 7：AgentPlan 编排更新
- video_prompt_generator 之后允许 LLMPlanner 选择 hyperframes_project_generator。
- hyperframes_project_generator 之后选择 hyperframes_renderer。
- hyperframes_renderer 之后选择 video_package_exporter。

任务 8：前端 / 本地诊断
- 增加 HyperFrames Render Service 状态显示。
- 显示 service_url、health、ffmpeg/chromium 状态。
- render 前提示用户确认。
- render 完成显示 final.mp4 artifact。

任务 9：E2E 测试
- fake provider 模式下跑 AgentPlan，不实际 render。
- service integration 测试中启动 hyperframes-render-service，传入一个最小 index.html，验证 final.mp4 输出。
- 端午节视频链路验证：
  1. 生成 HyperFrames projectDir。
  2. 调用 Render Service。
  3. 不调用 CLI。
  4. 生成 VIDEO artifact。
```

---

# 14. 当前阶段上线边界

第一版可承诺：

```text id="0lxoem"
AIOS 可通过 HyperFrames Render Service 将 Agent 生成的 HTML 视频项目渲染为 MP4。
```

暂时不要承诺：

```text id="cw58y9"
任意复杂分镜都能稳定生成电影级视频。
```

第一版更适合：

```text id="p0ro8g"
口播知识分享
图文解释视频
字幕卡片视频
数据可视化视频
产品介绍短片
```

不适合立刻做：

```text id="ocr6sz"
复杂角色动画
真实人物动作
长剧情短片
多镜头写实视频
```

---

# 15. 最终结论

当前阶段最优方案是：

```text id="t5g0xz"
HyperFrames Render Service
  +
AIOS Go HyperFrames Client
  +
Agent ToolManifest
  +
Project Generator
  +
Renderer Tool
```

这能实现：

```text id="trkog2"
不用 CLI
不用 npx
不依赖用户手动命令
可由 Agent 自动生成项目
可由系统调用服务渲染
可追踪任务状态
可保存 VIDEO artifact
可接入审核和质量门
```

一句话：

> **把 HyperFrames 从“命令行工具”升级为 AIOS 的“视频渲染执行器服务”，让 Agent 只负责编排和生成项目，真正渲染由 Render Service 稳定执行。**
