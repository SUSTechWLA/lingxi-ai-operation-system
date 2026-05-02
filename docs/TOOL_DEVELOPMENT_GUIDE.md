# 工具开发对接指南

> 本文档面向外部开发者，说明如何实现、注册、使用工具类，以及 AI 助手如何通过知识库发现和调用工具。

---

## 目录

1. [工具系统概述](#1-工具系统概述)
2. [工具知识库 API](#2-工具知识库-api)
3. [内置工具一览](#3-内置工具一览)
4. [外部工具开发](#4-外部工具开发)
5. [工具 Manifest 格式](#5-工具-manifest-格式)
6. [注册外部工具](#6-注册外部工具)
7. [执行契约](#7-执行契约)
8. [沙箱规则](#8-沙箱规则)
9. [AI 自创工具](#9-ai-自创工具)
10. [完整示例](#10-完整示例)
11. [安全注意事项](#11-安全注意事项)
12. [故障排查](#12-故障排查)

---

## 1. 工具系统概述

### 1.1 架构

```
┌─────────────────────────────────────────────────────────────┐
│                     AI 助手 / NL-Translator                    │
│  (通过知识库 API 发现工具 → 决策调用 → 输出 tool_call 格式)     │
└──────────────────────┬──────────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────────┐
│                      DAG 调度引擎 (Orchestrator)              │
│  解析 tool_call → 创建 Task + DAG → Worker 执行               │
└──────────────────────┬──────────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────────┐
│                    工具执行引擎 (Worker)                        │
│                                                              │
│  ┌────────────┐  ┌────────────┐  ┌──────────────────────┐   │
│  │ 内置工具    │  │ 外部工具    │  │ bash/python 自创工具  │   │
│  │ (Go实现)    │  │ (HTTP调用)  │  │ (沙箱中执行)          │   │
│  └────────────┘  └────────────┘  └──────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### 1.2 三种工具类型

| 类型 | 说明 | 适用场景 |
|------|------|---------|
| **内置工具 (builtin)** | Go 语言实现，编译进主程序 | 高频使用的标准操作（媒体分析、内容生成、合规检查等） |
| **外部工具 (external)** | 任意语言实现，通过 HTTP 注册和执行 | 专用业务逻辑、第三方 API 封装、遗留系统对接 |
| **自创工具** | AI 通过 bash/python 动态创建的临时脚本 | 没有现有工具覆盖的临时性任务 |

### 1.3 数据流

```
1. 用户请求 → AI 助手接收
2. AI 通过知识库 API 了解可用工具（名称、参数、用法）
3. AI 决定使用某个工具 → 输出 tool_call JSON
4. 系统解析 tool_call → 创建 DAG 任务 → Worker 执行
5. 工具执行结果返回给 AI
6. AI 合成最终回复给用户
```

---

## 2. 工具知识库 API

AI 助手和外部系统通过知识库 API 发现和了解可用工具。

### GET /api/tools — 列出所有工具

列出所有已注册工具（内置 + 外部）的完整 Manifest。

**请求：**
```bash
curl http://localhost:8080/api/tools
```

**响应：**
```json
{
  "code": 200,
  "message": "success",
  "data": [
    {
      "name": "media_analyzer",
      "description": "Analyze media files (images/videos) and generate tags, descriptions, and content suggestions",
      "type": "builtin",
      "parameters": {
        "prompt": { "type": "string", "description": "分析要求", "required": false },
        "file_names": { "type": "array", "description": "文件名列表", "required": false }
      },
      "sandbox": false
    },
    {
      "name": "my_custom_tool",
      "description": "自定义工具示例",
      "type": "external",
      "endpoint": "http://localhost:9001/execute",
      "timeout": 30,
      "parameters": { ... },
      "output": { ... },
      "examples": [ ... ],
      "sandbox": false
    }
  ]
}
```

### GET /api/tools/:name — 获取单个工具详情

**请求：**
```bash
curl http://localhost:8080/api/tools/media_analyzer
```

### POST /api/tools/register — 注册外部工具

向系统注册一个外部 HTTP 工具。参见 [第 6 节](#6-注册外部工具)。

### DELETE /api/tools/:name — 注销外部工具

---

## 3. 内置工具一览

系统预置了以下内置工具，可直接在 DAG 中使用：

| 工具名 | 用途 | 关键参数 | 沙箱 |
|--------|------|---------|------|
| `llm_api` | 调用 LLM API 进行文本生成 | `prompt` / `message` / `content` | 否 |
| `bash` | 沙箱执行 shell 命令 | `command` (shell 命令字符串) | **是** |
| `python` | python3 -c 执行 | `source` (Python 代码) | **是** |
| `polisher` | LLM 润色标题/简介 | `text`, `polishType` (`title` / `description`) | 否 |
| `media_analyzer` | 分析图片/视频素材 | `prompt`, `media_ids`, `file_names` | 否 |
| `content_generator` | 生成完整内容包 | `prompt`, `platform`, `style`, `analysis` | 否 |
| `content_checker` | 合规检查 | `content`, `title`, `platform` | 否 |
| `platform_adapter` | 平台适配 | `source_content`, `target_platform`, `title` | 否 |
| `chat_revise` | 修改现有内容字段 | `message`, `title`, `description`, `keywords` | 否 |
| `chat_generate` | 对话式内容生成 | `messages` (完整消息数组) | 否 |
| `external` | 执行外部注册工具 | `tool` (外部工具名), 其他参数 | 取决于外部工具 |

### 3.1 何时使用 bash/python（沙箱工具）

以下情况应使用 bash/python 工具来创建临时脚本：

- **数据处理**：CSV/JSON 解析、格式转换、批量重命名
- **网络请求**：通过 curl 调用第三方 API、下载文件
- **文本处理**：复杂的正则匹配、模板渲染
- **自定义分析**：没有现成工具且需要特定计算的场景

### 3.2 bash 沙箱限制

bash 工具在沙箱中执行，有以下限制：

| 限制 | 说明 |
|------|------|
| 命令白名单 | `ls`, `cat`, `echo`, `curl`, `python3`, `node`, `grep`, `find`, `wc`, `head`, `tail` 等 |
| 危险模式 | `;`, `\|`, `&&`, `\`\``, `$()`, `>`, `<` 等被禁止 |
| 工作目录 | `/tmp/lingxi-sandbox` |
| 内存限制 | 256MB |
| 超时 | 默认 30s |

> **重要**：如果 bash 的白名单限制太严格，考虑使用 python 工具进行更灵活的操作。python 执行 `python3 -c` 代码，不受命令白名单限制。

---

## 4. 外部工具开发

你可以用**任意编程语言**（Python、Node.js、Go、Rust、Java 等）实现工具，通过 HTTP 注册到系统。

### 4.1 工具生命周期

```
1. 开发: 在你选择的语言中实现工具逻辑
2. 暴露: 提供 HTTP 端点 (POST /execute)
3. 注册: 通过 POST /api/tools/register 提交 Manifest
4. 发现: AI 助手通过知识库 API 自动发现你的工具
5. 执行: 用户触发 → AI 决策 → DAG → Worker → 你的 HTTP 端点
6. 更新: 重新注册覆盖 Manifest
7. 注销: DELETE /api/tools/:name
```

### 4.2 开发要求

外部工具只需满足以下 3 个要求：

1. **HTTP 端点**：暴露一个 `POST` 端点，接收 JSON 请求，返回 JSON 响应
2. **标准请求格式**：接收以下结构的 JSON body
3. **标准响应格式**：返回结构化的 JSON 响应

---

## 5. 工具 Manifest 格式

Manifest 是工具的"身份证"，定义了工具的元数据、参数、输出和示例。

```json
{
  "name": "my_tool",
  "description": "工具的一句话描述，AI 据此判断何时使用",
  "version": "1.0.0",
  "author": "开发者姓名或组织",

  "type": "external",
  "endpoint": "http://localhost:9001/execute",

  "timeout": 30,

  "parameters": {
    "url": {
      "type": "string",
      "description": "要请求的 URL",
      "required": true
    },
    "method": {
      "type": "string",
      "description": "HTTP 方法 (GET/POST)",
      "required": false,
      "default": "GET",
      "enum": ["GET", "POST"]
    },
    "headers": {
      "type": "object",
      "description": "自定义请求头",
      "required": false
    }
  },

  "output": {
    "statusCode": {
      "type": "integer",
      "description": "HTTP 状态码"
    },
    "body": {
      "type": "string",
      "description": "响应体内容"
    }
  },

  "sandbox": false,

  "examples": [
    {
      "input": { "url": "https://api.example.com/data" },
      "output": { "statusCode": 200, "body": "..." }
    }
  ]
}
```

### 字段说明

| 字段 | 必填 | 说明 |
|------|------|------|
| `name` | **是** | 工具全局唯一名，字母数字下划线，AI 通过此名调用 |
| `description` | **是** | 一句话描述工具功能，**AI 据此判断何时使用**，务必清晰 |
| `version` | 否 | 版本号 |
| `author` | 否 | 开发者信息 |
| `type` | **是** | 工具类型：`external`（外部 HTTP 工具）、`builtin`（内置 Go 工具） |
| `endpoint` | **是** | 工具 HTTP 端点完整 URL。系统收到请求后 POST JSON 到此地址 |
| `timeout` | 否 | 超时秒数（默认 30） |
| `parameters` | **是** | 参数定义，键为参数名，值为参数定义对象 |
| `output` | **是** | 输出字段定义，键为字段名，值为字段定义对象 |
| `sandbox` | 否 | 是否需要沙箱隔离（默认 false） |
| `examples` | 否 | 输入输出示例，**帮助 AI 理解如何调用**，建议至少 1 个 |

### 参数/输出字段定义

| 子字段 | 必填 | 说明 |
|--------|------|------|
| `type` | **是** | 数据类型：`string`, `integer`, `number`, `boolean`, `array`, `object` |
| `description` | **是** | 字段描述，AI 据此填写参数值 |
| `required` | 否 | 是否必填（默认 false） |
| `default` | 否 | 默认值 |
| `enum` | 否 | 可选值列表 |

---

## 6. 注册外部工具

### 6.1 注册

```bash
curl -X POST http://localhost:8080/api/tools/register \
  -H "Content-Type: application/json" \
  -d '{
    "name": "image_processor",
    "description": "处理图片文件，返回宽度、高度和格式信息",
    "version": "1.0.0",
    "type": "external",
    "endpoint": "http://localhost:9001/image",
    "timeout": 10,
    "parameters": {
      "image_url": {
        "type": "string",
        "description": "图片URL地址",
        "required": true
      }
    },
    "output": {
      "width": { "type": "string", "description": "图片宽度" },
      "height": { "type": "string", "description": "图片高度" },
      "format": { "type": "string", "description": "图片格式" }
    },
    "examples": [
      {
        "input": { "image_url": "https://example.com/photo.jpg" },
        "output": { "width": "1920", "height": "1080", "format": "jpeg" }
      }
    ]
  }'
```

**成功响应：**
```json
{
  "code": 200,
  "message": "tool registered successfully",
  "data": { "name": "image_processor", "type": "external" }
}
```

### 6.2 查看已注册的工具

```bash
# 列出所有工具
curl http://localhost:8080/api/tools

# 查看具体工具详情
curl http://localhost:8080/api/tools/image_processor
```

### 6.3 注销

```bash
curl -X DELETE http://localhost:8080/api/tools/image_processor
```

---

## 7. 执行契约

当系统调用外部工具时，会 POST JSON 到注册的 `endpoint`，并期待返回 JSON。

### 7.1 请求格式

系统发送的请求体：

```json
{
  "tool": "image_processor",
  "params": {
    "city": "北京"
  },
  "task_id": "20260430120000-abc123",
  "node_id": "tool-image-processor-1714411200000-0"
}
```

| 字段 | 说明 |
|------|------|
| `tool` | 工具名称，用于你的服务识别调用来源 |
| `params` | Manifest 中 parameters 定义的参数，具体值由 AI 根据用户请求填充 |
| `task_id` | 当前 DAG 任务 ID，可用于日志追踪 |
| `node_id` | 当前执行节点 ID |

### 7.2 响应格式

成功的响应（HTTP 200）：

```json
{
  "temperature": "22°C",
  "condition": "晴",
  "humidity": "45%",
  "wind": "3级"
}
```

响应字段应与 Manifest 中 `output` 定义一致。额外字段会被保留但不会被 AI 优先关注。

**错误响应**（HTTP 4xx/5xx）：

```json
{
  "error": "city not found: 未知城市",
  "code": 404
}
```

### 7.3 关键规则

| 规则 | 说明 |
|------|------|
| **同步** | 外部工具必须同步返回结果。如果工具需要长时间处理，建议实现异步模式（先返回 task_id，再轮询） |
| **超时** | 超时会返回错误，超时值由 Manifest 的 `timeout` 字段控制 |
| **重试** | 工具执行失败后，由 Orchestrator 按重试策略重试（默认最多 3 次，指数退避） |
| **幂等** | 工具应尽可能幂等，因为重试可能导致多次执行 |

---

## 8. 沙箱规则

### 8.1 需要沙箱的场景

以下情况需要启用沙箱（在 Manifest 中设置 `"sandbox": true`）：

- **执行用户提供的代码或命令** — 防止恶意代码影响主机
- **访问不可信的外部资源** — 隔离网络请求的文件系统影响
- **处理敏感数据** — 确保临时文件不会泄漏
- **高权限操作** — 防止权限滥用

### 8.2 不需要沙箱的场景

- **纯 LLM 调用** — 数据在内存中处理，无副作用
- **只读 API 调用** — 调用外部 API 获取数据
- **已沙箱的工具组合** — 内部调用已在沙箱中

### 8.3 内置沙箱工具

`bash` 和 `python` 工具自动在沙箱中执行：

| 特性 | 说明 |
|------|------|
| 文件系统隔离 | 仅在 `/tmp/lingxi-sandbox/` 下操作 |
| 资源限制 | 256MB 内存，30s 超时 |
| 命令白名单 | bash 仅允许白名单内的命令 |
| 环境隔离 | 独立环境变量 |
| 自动清理 | 执行完成后自动清理临时文件 |

---

## 9. AI 自创工具

AI 可以动态创建临时工具来完成任务。当没有现成的内置或外部工具时，AI 会自动使用 `bash` 或 `python` 工具作为"自创工具"。

### 9.1 自创工具的典型场景

```python
# 场景 1：数据处理 — AI 用 Python 处理 JSON/CSV 数据
{
  "type": "tool_call",
  "reasoning": "用户想分析这篇文本的关键词，没有现成的工具，我用 Python 实现",
  "reply": "我正在分析文本关键词...",
  "tools": [
    {
      "name": "python",
      "params": {
        "code": "import re, collections; text = \"用户提供的文本\"; words = re.findall(r'\\w+', text); print(collections.Counter(words).most_common(10))"
      }
    }
  ]
}
```

```bash
# 场景 2：调用第三方 API — AI 用 curl + bash
{
  "type": "tool_call",
  "tools": [
    {
      "name": "bash",
      "params": {
        "command": "curl -s https://api.example.com/data | python3 -c \"import sys,json; data=json.load(sys.stdin); print(json.dumps(data, indent=2, ensure_ascii=False))\""
      }
    }
  ]
}
```

### 9.2 自创工具的限制

- 是**临时**的，不会持久化到知识库
- 适用于一次性任务
- 如果某个自创工具被频繁使用，应考虑开发为外部工具

---

## 10. 完整示例

### 10.1 Python 外部工具

**工具代码** (`image_service.py`)：

```python
#!/usr/bin/env python3
"""图片处理外部工具示例"""
import json
import http.server

class ImageHandler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        content_length = int(self.headers['Content-Length'])
        body = self.rfile.read(content_length)
        request = json.loads(body)

        tool = request.get('tool', '')
        params = request.get('params', {})
        task_id = request.get('task_id', '')
        image_url = params.get('image_url', '')

        print(f"[ImageTool] url={image_url}, task={task_id}")

        # 模拟图片处理
        result = {"width": "1920", "height": "1080", "format": "jpeg"}

        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.end_headers()
        self.wfile.write(json.dumps(result).encode())

if __name__ == '__main__':
    server = http.server.HTTPServer(('localhost', 9001), ImageHandler)
    print("Image tool running on http://localhost:9001")
    server.serve_forever()
```

**注册命令：**

```bash
curl -X POST http://localhost:8080/api/tools/register \
  -H "Content-Type: application/json" \
  -d '{
    "name": "image_processor",
    "description": "处理图片文件，返回宽度、高度和格式信息",
    "type": "external",
    "endpoint": "http://localhost:9001/",
    "timeout": 10,
    "parameters": {
      "image_url": {
        "type": "string",
        "description": "图片URL地址",
        "required": true
      }
    },
    "output": {
      "temperature": { "type": "string", "description": "温度" },
      "condition": { "type": "string", "description": "图片格式" },
      "humidity": { "type": "string", "description": "湿度" }
    },
    "examples": [
      {
        "input": { "city": "北京" },
        "output": { "temperature": "22°C", "condition": "晴", "humidity": "45%" }
      }
    ]
  }'
```

### 10.2 Node.js 外部工具

**工具代码** (`translate_service.js`)：

```javascript
#!/usr/bin/env node
const http = require('http');

const server = http.createServer((req, res) => {
  if (req.method !== 'POST') {
    res.writeHead(405);
    return res.end();
  }

  let body = '';
  req.on('data', chunk => body += chunk);
  req.on('end', () => {
    const request = JSON.parse(body);
    const { text, targetLang } = request.params || {};
    const taskId = request.task_id;

    console.log(`[Translate] text=${text}, target=${targetLang}, task=${taskId}`);

    // 模拟翻译
    const result = {
      translatedText: `[${targetLang}] ${text}`,
      sourceLang: 'auto',
      targetLang: targetLang
    };

    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify(result));
  });
});

server.listen(9002, () => console.log('Translate tool on :9002'));
```

### 10.3 Go 内置工具（高级）

如果你希望将工具编译进主程序而不是通过 HTTP，可以实现 Go 的 `Tool` 接口：

```go
package builtin

import (
    "context"
    "fmt"

    "github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/tool"
)

type MyTool struct{}

func NewMyTool() *MyTool { return &MyTool{} }

func (t *MyTool) Name() string        { return "my_tool" }
func (t *MyTool) Description() string  { return "工具功能描述，会出现在 AI 的知识库中" }
func (t *MyTool) Type() tool.ToolType { return tool.ToolTypeCustom }

func (t *MyTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
    // 1. 解析参数
    input, _ := params["input"].(string)
    if input == "" {
        return tool.FailureResult("input is required")
    }

    // 2. 执行业务逻辑
    result := fmt.Sprintf("processed: %s", input)

    // 3. 返回结果（JSON 兼容的 map）
    return tool.SuccessResult(map[string]interface{}{
        "output": result,
    })
}

func (t *MyTool) ValidateParameters(params map[string]interface{}) bool {
    _, ok := params["input"].(string)
    return ok
}
```

然后在 `cmd/lingxi-ai-os/main.go` 注册：

```go
toolRegistry.Register(builtin.NewMyTool())
```

---

## 11. 安全注意事项

### 11.1 工具开发安全

| 注意事项 | 说明 |
|---------|------|
| **输入验证** | 外部工具始终验证 AI 传入的参数，不要信任任何输入 |
| **最小权限** | 你的工具进程应以最低必要权限运行 |
| **超时保护** | 在工具代码中实现请求级超时，防止慢请求阻塞 |
| **错误处理** | 返回有意义的错误信息，帮助 AI 判断是否重试 |
| **幂等设计** | 相同的输入应产生相同的输出（或至少无副作用） |
| **日志脱敏** | 不要在日志中记录敏感数据（API Key、用户隐私等） |

### 11.2 HTTP 端点保护

推荐在外部工具的 HTTP 端点上实施以下保护：

```python
# 可选：验证来源 IP
ALLOWED_IPS = ['127.0.0.1', '::1', '10.0.0.0/8']

# 可选：共享密钥验证
EXPECTED_TOKEN = os.environ.get('TOOL_SECRET', '')
```

### 11.3 沙箱安全

参见 [SANDBOX_INTEGRATION_GUIDE.md](./SANDBOX_INTEGRATION_GUIDE.md) 了解沙箱的安全边界。

---

## 12. 故障排查

### 12.1 工具未注册

```
AI 输出：我找不到合适的工具来完成这个任务
```

**排查：**
```bash
# 检查工具是否已注册
curl http://localhost:8080/api/tools
```

### 12.2 工具执行超时

```
Worker 日志：external tool HTTP call failed: context deadline exceeded
```

**排查：**
- 检查 Manifest 的 timeout 是否足够
- 检查外部工具的服务是否正常运行
- 检查网络连通性

### 12.3 参数错误

```
AI 生成了错误的参数值
```

**排查：**
- Manifest 中的 `parameters` 定义是否清晰？`description` 是否准确描述了参数含义？
- `examples` 是否足够？AI 通过示例学习正确的参数格式
- 检查 `required` 字段是否正确标记了必填参数

### 12.4 外部工具返回错误

```
Worker 日志：external tool returned HTTP 500
```

**排查：**
```bash
# 直接测试外部工具的端点
curl -X POST http://localhost:9001/execute \
  -H "Content-Type: application/json" \
  -d '{"tool": "my_tool", "params": {"key": "value"}, "task_id": "test", "node_id": "test"}'
```

---

> **相关文档：**
> - [API_REFERENCE.md](./API_REFERENCE.md) — 完整 API 参考
> - [ARCHITECTURE.md](./ARCHITECTURE.md) — 系统架构设计
> - [SANDBOX_INTEGRATION_GUIDE.md](./SANDBOX_INTEGRATION_GUIDE.md) — 沙箱集成指南
> - `internal/worker/tool/tool.go` — Tool 接口和注册表源码
> - `internal/worker/tool/builtin/external_tool.go` — 外部工具执行器源码
