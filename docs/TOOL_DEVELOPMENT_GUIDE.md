# 工具开发对接指南

> 本文档适用于希望通过**非 Go 语言**（Python、Node.js、Shell 等）编写工具并接入灵犀 AI OS 系统的开发者。
> 如果你只想了解如何使用系统内置的 AI 润色、发布等功能，请阅读 [ONBOARDING.md](./ONBOARDING.md)。

---

## 目录

1. [什么是工具](#1-什么是工具)
2. [工具的两种形态](#2-工具的两种形态)
3. [工具执行流程](#3-工具执行流程)
4. [对接方式一：独立脚本（推荐）](#4-对接方式一独立脚本推荐)
5. [对接方式二：Go 内置插件](#5-对接方式二go-内置插件)
6. [JSON 输出契约](#6-json-输出契约)
7. [输入参数传递](#7-输入参数传递)
8. [执行指标和元数据](#8-执行指标和元数据)
9. [错误处理](#9-错误处理)
10. [Python 工具示例](#10-python-工具示例)
11. [Node.js 工具示例](#11-nodejs-工具示例)
12. [Shell 脚本工具示例](#12-shell-脚本工具示例)
13. [注册工具到系统](#13-注册工具到系统)
14. [测试工具](#14-测试工具)
15. [在 DAG 中使用工具](#15-在-dag-中使用工具)
16. [常见问题](#16-常见问题)
17. [附录：完整输出示例](#17-附录完整输出示例)

---

## 1. 什么是工具

**工具（Tool）** 是灵犀 AI OS 中可以独立执行的**功能单元**。系统通过 DAG（有向无环图）编排工具的调用顺序，实现复杂的自动化流程。

简单理解：
- 工具 = 一个"函数"或"命令"，接收输入 → 执行操作 → 返回结果
- DAG = 把这些工具串起来，前一个的输出传给后一个

已实现的内置工具：

| 工具名 | 语言 | 用途 |
|--------|------|------|
| `llm_api` | Go | 调用 OpenAI 兼容 API 进行文本生成 |
| `bash` | Go | 执行 Shell 命令（沙箱保护） |
| `python` | Go | Python3 -c 执行，资源限制 |
| `polisher` | Go | 文本润色（调用 LLM 优化标题/简介） |
| `media_analyzer` | Go | 分析图片/视频，输出标签、建议和摘要 |
| `content_generator` | Go | 基于素材分析和平台风格生成完整内容包 |
| `content_checker` | Go | 检测敏感词、极限词、平台规范违规 |
| `platform_adapter` | Go | 适配内容到抖音、小红书、B站等平台风格 |

你可以在**任何语言**中编写工具并接入系统。

---

## 2. 工具的两种形态

### 形态 A：独立脚本（Subprocess 模式）

你的工具是一个独立的脚本/程序（Python、Node.js、Shell、Ruby 等），系统通过子进程方式调用它。

```
系统 → 运行子进程 → 你的脚本（Python/Node/Shell）
                     ↓
                  stdout 输出 JSON 结果
                     ↓
系统捕获输出 → 记录到节点结果
```

**优点**：无需 Go 知识，任何语言都可以写，独立部署，不重启系统即可更新脚本。

### 形态 B：Go 内置插件（Plugin 模式）

你的工具作为 Go 代码编译到系统二进制文件中。

```
系统 → 直接函数调用 → 你的 Go 工具
                       ↓
                    返回 ToolResult 结构
```

**优点**：性能最好，无进程开销，可访问系统内部 API。
**缺点**：需要 Go 开发环境，修改后需要重新编译。

---

## 3. 工具执行流程

无论是哪种形态，系统都会按以下流程执行工具：

```
1. Orchestrator 调度器发现节点就绪
   ↓
2. Worker 收到 ai.node.ready 事件
   ↓
3. Worker 记录 NODE_SCHEDULED（含 startedAt 时间戳）
   ↓
4. Worker 调用对应工具的 Execute 方法
   ↓
    ┌─ 形态 A（独立脚本）: 创建子进程 → 执行脚本 → 捕获 stdout
    └─ 形态 B（Go 插件）:  直接调用 Execute() 函数
   ↓
5. Worker 计算执行耗时（durationMs）
   ↓
6. Worker 记录 NODE_SUCCESS / NODE_FAILED（含 durationMs、exitCode）
   ↓
7. Worker 发布 ai.node.result 事件到 Kafka
   ↓
8. Context 服务消费事件，将执行指标写入审计记录
```

---

## 4. 对接方式一：独立脚本（推荐）

### 4.1 前置要求

无需 Go 开发环境，只需：
- 你的脚本语言运行环境（Python 3、Node.js 18+、Bash 等）
- 脚本文件放置在系统可以访问的路径

### 4.2 核心原则

你的脚本只需要遵守 **三个规则**：

| # | 规则 | 说明 |
|---|------|------|
| 1 | **输出 JSON 到 stdout** | 执行结果必须是 JSON 格式，写入标准输出 |
| 2 | **退出码 0 表示成功** | 非 0 退出码会被系统判定为失败 |
| 3 | **错误信息写 stderr** | 错误详情写入标准错误输出 |

### 4.3 系统调用方式

系统通过 BashTool 的 `BuildExecutionRequest` 构建执行命令。默认情况下，系统会在 DAG 节点的 `input.command` 字段中指定要执行的命令。

例如，在 DAG 中定义一个工具节点：

```json
{
  "id": "my-tool-node",
  "type": "TOOL",
  "name": "bash",
  "input": {
    "command": "python3 /path/to/your_script.py --param1 value1 --param2 value2"
  }
}
```

系统会执行这个命令并捕获输出。

### 4.4 更规范的方式：包装为 BuildableTool

如果希望系统原生识别你的工具（像 `bash`、`polisher` 一样按名称调用），需要创建一个 Go 的 BuildableTool 包装器。参见 [第 13 节](#13-注册工具到系统)。

---

## 5. 对接方式二：Go 内置插件

如果你有 Go 开发环境，可以将工具直接编译到系统中。

### 5.1 实现 Tool 接口

```go
type Tool interface {
    Name() string                                                    // 工具唯一名称
    Description() string                                             // 工具描述
    Type() ToolType                                                  // LLM / CUSTOM
    Execute(ctx, params, toolCtx) ToolResult                          // 执行逻辑
    ValidateParameters(params map[string]interface{}) bool            // 参数校验
}
```

### 5.2 ToolResult 结构

```go
type ToolResult struct {
    Success   bool                   `json:"success"`              // 是否成功
    Data      map[string]interface{} `json:"data,omitempty"`       // 成功时的返回数据
    Error     string                 `json:"error,omitempty"`      // 失败时错误信息
}
```

### 5.3 辅助函数

```go
// 返回成功结果
return tool.SuccessResult(map[string]interface{}{
    "result": "success data",
})

// 返回失败结果
return tool.FailureResult("error description")
```

### 5.4 完整示例

```go
package builtin

import (
    "context"
    "github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/tool"
)

type MyTool struct{}

func NewMyTool() *MyTool { return &MyTool{} }

func (t *MyTool) Name() string                       { return "my_tool" }
func (t *MyTool) Description() string                 { return "Does something useful" }
func (t *MyTool) Type() tool.ToolType                 { return tool.ToolTypeCustom }
func (t *MyTool) ValidateParameters(params map[string]interface{}) bool {
    _, ok := params["input"].(string)
    return ok
}
func (t *MyTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
    input, _ := params["input"].(string)
    // ... 执行逻辑 ...
    return tool.SuccessResult(map[string]interface{}{
        "output": "processed: " + input,
    })
}
```

---

## 6. JSON 输出契约

这是**最重要**的部分。无论你用什么语言写工具，你的脚本必须输出 JSON 到 stdout。

### 6.1 成功时的输出格式

脚本成功执行时：

```json
{
  "success": true,
  "data": {
    "key1": "value1",
    "key2": 123,
    "key3": {"nested": "object"},
    "key4": ["list", "of", "items"]
  }
}
```

**或简写**（success: true 可以省略，系统默认退出码为 0 即为成功）：

```json
{
  "result": "操作成功",
  "count": 42,
  "items": ["a", "b", "c"]
}
```

### 6.2 失败时的输出格式

脚本执行失败时：

```json
{
  "success": false,
  "error": "描述错误原因",
  "errorCode": "ERROR_001"
}
```

同时脚本的**退出码必须是 1**（或任何非 0 值）。

### 6.3 系统如何处理你的输出

系统会捕获脚本的 stdout 和 stderr，并包装成以下结构写入节点 output：

```json
{
  "exitCode": 0,
  "stdout": "{\"success\": true, \"data\": {...}}",
  "stderr": "",
  "durationMs": 1254
}
```

### 6.4 关键数据字段说明

| 字段 | 类型 | 说明 | 由谁填写 |
|------|------|------|---------|
| `exitCode` | int | 进程退出码（0=成功，非0=失败） | 系统自动 |
| `stdout` | string | 你的脚本输出的 stdout 内容 | 系统自动捕获 |
| `stderr` | string | 你的脚本输出的 stderr 内容 | 系统自动捕获 |
| `durationMs` | int | 执行耗时（毫秒） | 系统自动计算 |
| `startedAt` | string | 执行开始时间（ISO 8601） | 系统自动记录 |
| `startTime` | string | 与 startedAt 含义相同 | 系统自动 |
| `endTime` | string | 执行结束时间 | 系统自动 |
| `error` | string | 错误描述（失败时） | 系统或脚本 |
| `resourceUsage` | object | 资源使用统计（预留，待沙箱集成后可用） | 系统自动 |
| `outputRef` | string | 大文件输出引用路径 | 系统（对于大文件场景） |

---

## 7. 输入参数传递

你的脚本可以通过以下方式接收输入参数：

### 7.1 命令行参数

在 DAG 节点的 `input` 中指定完整命令：

```json
{
  "input": {
    "command": "python3 /opt/tools/weather.py --city 北京 --date 2026-04-26"
  }
}
```

你的脚本侧（Python）：

```python
import argparse
parser = argparse.ArgumentParser()
parser.add_argument('--city')
parser.add_argument('--date')
args = parser.parse_args()
city = args.city
```

### 7.2 环境变量

系统在执行工具时注入环境变量。目前注入的环境变量：

| 环境变量 | 说明 | 示例 |
|---------|------|------|
| `TASK_ID` | 当前任务 ID | `20260426231320-68686850` |
| `NODE_ID` | 当前节点 ID | `polish-1777216400959` |
| `TRACE_ID` | 追踪 ID | `20260426231320-68686850-polish-1777216400959` |

你的脚本可以读取：

```python
import os
task_id = os.environ.get('TASK_ID', '')
node_id = os.environ.get('NODE_ID', '')
```

### 7.3 标准输入（stdin）

系统可以给子进程提供 stdin 数据（需 BuildableTool 支持）。未来版本会支持通过 stdin 传递 JSON 格式的完整参数包。

---

## 8. 执行指标和元数据

系统会自动为每次工具执行记录以下指标，无需你的脚本做任何额外工作：

| 指标 | 记录位置 | 说明 |
|------|---------|------|
| `startedAt` | node.output 和 context.metadata | Worker 开始执行工具的时刻 |
| `durationMs` | node.output 和 context.metadata | 从开始到结束的毫秒数 |
| `exitCode` | node.output 和 context.metadata | 进程退出码 |
| `error` | node.output 和 context.metadata | 错误信息（仅失败时） |

这些指标会自动进入 Context 审计服务，你可以在 `/api/trace/recent` 或 `/api/trace/:taskId` 中查看。

---

## 9. 错误处理

### 9.1 脚本级别的错误

场景：输入参数不合法、配置缺失、外部服务不可用。

```python
import sys, json

def validate_input(params):
    if 'city' not in params:
        print(json.dumps({"success": False, "error": "缺少城市参数"}))
        sys.exit(1)

validate_input(params)
```

### 9.2 系统级别的错误

| 场景 | 系统行为 |
|------|---------|
| 脚本不存在或路径错误 | 系统捕获错误，节点标记为 FAILED |
| 脚本执行超时（默认 120 秒） | 系统终止进程，节点标记为 FAILED |
| 脚本输出不是有效 JSON | 系统仍记录 stdout 字符串，依赖下游解析 |
| 退出码非 0 但 stdout 中有 JSON | 系统优先判断退出码，节点标记为 FAILED |

### 9.3 重试策略

系统支持自动重试失败的节点：

```
指数退避：1s → 2s → 4s → 8s → 16s → 32s → 60s（最大）
默认重试次数：3 次（可在 DAG 节点配置中修改 maxRetry）
```

你的脚本应该是**幂等的**：重复执行同一个输入应该得到同样的结果，不会产生副作用。

---

## 10. Python 工具示例

### 示例：城市天气查询工具

**Step 1** 创建脚本 `/opt/tools/weather_tool.py`：

```python
#!/usr/bin/env python3
"""
天气查询工具
输入：--city 城市名（必需）
输出：JSON 到 stdout
"""

import argparse
import json
import sys
import random

def validate(city):
    if not city or not city.strip():
        return False, "城市名不能为空"
    return True, ""

def query_weather(city):
    """模拟天气查询"""
    conditions = ["晴天", "多云", "小雨", "阴天", "大风"]
    # 在这里替换为你自己的天气 API 调用
    return {
        "city": city,
        "temperature": f"{random.randint(15, 35)}°C",
        "condition": random.choice(conditions),
        "humidity": f"{random.randint(30, 80)}%",
        "wind": f"{random.randint(5, 30)} km/h"
    }

def main():
    parser = argparse.ArgumentParser(description='天气查询工具')
    parser.add_argument('--city', required=True, help='城市名称')
    args = parser.parse_args()

    # 验证输入
    valid, error_msg = validate(args.city)
    if not valid:
        print(json.dumps({"success": False, "error": error_msg}))
        sys.exit(1)

    try:
        # 执行查询
        data = query_weather(args.city)

        # 输出成功结果
        print(json.dumps({
            "success": True,
            "data": data
        }))
        sys.exit(0)
    except Exception as e:
        print(json.dumps({"success": False, "error": str(e)}))
        sys.exit(1)

if __name__ == '__main__':
    main()
```

**Step 2** 使脚本可执行：

```bash
chmod +x /opt/tools/weather_tool.py
```

**Step 3** 测试运行：

```bash
python3 /opt/tools/weather_tool.py --city 北京
# 预期输出：
# {"success": true, "data": {"city": "北京", "temperature": "25°C", ...}}
```

### 示例：内容摘要生成工具

```python
#!/usr/bin/env python3
"""
内容摘要生成工具
输入：
  --text    要摘要的文本（必需）
  --max-length 摘要最大长度（可选，默认 200）
输出：JSON
"""

import argparse
import json
import sys
import os

def summarize(text, max_length):
    """调用 LLM API 生成摘要"""
    # 这里可以整合 openai SDK 或其他 API
    api_key = os.environ.get('OPENAI_API_KEY')
    if not api_key:
        return {"success": False, "error": "OPENAI_API_KEY 环境变量未设置"}

    # ... 实际的 LLM 调用逻辑 ...
    summary = text[:max_length] + "..."

    return {
        "success": True,
        "data": {
            "summary": summary,
            "original_length": len(text),
            "summary_length": len(summary)
        }
    }

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--text', required=True)
    parser.add_argument('--max-length', type=int, default=200)
    args = parser.parse_args()

    result = summarize(args.text, args.max_length)
    print(json.dumps(result, ensure_ascii=False))
    sys.exit(0 if result.get("success") else 1)

if __name__ == '__main__':
    main()
```

---

## 11. Node.js 工具示例

### 示例：网页抓取工具

**Step 1** 创建脚本 `/opt/tools/fetch_page.mjs`：

```javascript
#!/usr/bin/env node
/**
 * 网页抓取工具
 * 输入：--url 目标网址（必需）
 * 输出：JSON 到 stdout
 */

import { argv } from 'process';

function parseArgs() {
  const args = {};
  for (let i = 2; i < argv.length; i += 2) {
    const key = argv[i].replace('--', '');
    args[key] = argv[i + 1];
  }
  return args;
}

async function main() {
  const params = parseArgs();
  const url = params.url;

  // 验证输入
  if (!url) {
    console.log(JSON.stringify({ success: false, error: '缺少 --url 参数' }));
    process.exit(1);
  }

  try {
    // 执行 HTTP 请求
    const response = await fetch(url);
    const text = await response.text();

    // 输出成功结果
    console.log(JSON.stringify({
      success: true,
      data: {
        url: url,
        statusCode: response.status,
        contentLength: text.length,
        contentType: response.headers.get('content-type'),
        snippet: text.substring(0, 500)  // 只保存前 500 字符
      }
    }));
    process.exit(0);
  } catch (error) {
    console.log(JSON.stringify({
      success: false,
      error: `请求失败: ${error.message}`
    }));
    process.exit(1);
  }
}

main();
```

**Step 2** 使脚本可执行：

```bash
chmod +x /opt/tools/fetch_page.mjs
```

**Step 3** 测试运行：

```bash
node /opt/tools/fetch_page.mjs --url https://example.com
# 预期输出：
# {"success": true, "data": {"url": "https://example.com", "statusCode": 200, ...}}
```

---

## 12. Shell 脚本工具示例

### 示例：文件统计工具

```bash
#!/bin/bash
# 文件统计工具
# 输入：--dir 目录路径（必需）
# 输出：JSON 到 stdout

# 解析参数
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir) DIR="$2"; shift 2 ;;
    *) echo "{\"success\": false, \"error\": \"未知参数: $1\"}"; exit 1 ;;
  esac
done

# 验证输入
if [ -z "$DIR" ]; then
  echo '{"success": false, "error": "--dir 参数不能为空"}'
  exit 1
fi

if [ ! -d "$DIR" ]; then
  echo "{\"success\": false, \"error\": \"目录不存在: $DIR\"}"
  exit 1
fi

# 执行统计
FILE_COUNT=$(find "$DIR" -type f | wc -l | tr -d ' ')
DIR_COUNT=$(find "$DIR" -type d | wc -l | tr -d ' ')
TOTAL_SIZE=$(du -sh "$DIR" 2>/dev/null | cut -f1)

# 输出成功结果
echo "{\"success\": true, \"data\": {\"fileCount\": $FILE_COUNT, \"dirCount\": $DIR_COUNT, \"totalSize\": \"$TOTAL_SIZE\"}}"
exit 0
```

---

## 13. 注册工具到系统

### 13.1 方式一：通过 DAG 直接调用（无需 Go 开发）

这是**最简单的方式**，不需要修改 Go 代码。

在 DAG 节点中，将工具类型设为 `TOOL`，名称设为 `bash`，并把你的脚本执行命令放在 `input.command` 中：

```json
{
  "nodes": [
    {
      "id": "weather-query",
      "type": "TOOL",
      "name": "bash",
      "input": {
        "command": "python3 /opt/tools/weather_tool.py --city 北京"
      }
    }
  ]
}
```

系统通过 BashTool 直接调用你的脚本，无需任何代码修改。

### 13.2 方式二：注册为系统原生工具（需 Go 开发）

如果你希望工具像 `bash`、`polisher` 一样按**名称**被调用（例如 `name: "my_tool"`），需要：

**Step 1** 创建 Go 包装器，将你的脚本包装为 `BuildableTool`：

```go
// internal/worker/tool/builtin/my_script_tool.go
package builtin

import (
    "github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/executor"
    "github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/tool"
)

type MyScriptTool struct{}

func NewMyScriptTool() *MyScriptTool { return &MyScriptTool{} }

func (t *MyScriptTool) Name() string        { return "my_tool" }
func (t *MyScriptTool) Description() string  { return "我的自定义工具" }
func (t *MyScriptTool) Type() tool.ToolType  { return tool.ToolTypeCustom }
func (t *MyScriptTool) ValidateParameters(params map[string]interface{}) bool {
    _, ok := params["city"].(string)
    return ok
}

// 将参数转为 shell 命令发送给脚本
func (t *MyScriptTool) BuildExecutionRequest(params map[string]interface{}) (*executor.ExecutionRequest, error) {
    city, _ := params["city"].(string)
    return &executor.ExecutionRequest{
        Command:    "python3",
        Args:       []string{"/opt/tools/weather_tool.py", "--city", city},
        TimeoutSec: 30,
    }, nil
}
```

**Step 2** 在 `cmd/lingxi-ai-os/main.go` 中注册：

```go
toolRegistry.Register(builtin.NewMyScriptTool())
```

**Step 3** 重新编译并启动：

```bash
make build && make run
```

之后就可以在 DAG 中按名称调用：

```json
{
  "id": "weather-query",
  "type": "TOOL",
  "name": "my_tool",
  "input": {"city": "北京"}
}
```

---

## 14. 测试工具

### 14.1 本地直接测试

在终端直接运行你的脚本，验证输出格式：

```bash
# Python
python3 /opt/tools/weather_tool.py --city 北京 | python3 -m json.tool

# Node.js
node /opt/tools/fetch_page.mjs --url https://example.com | python3 -m json.tool
```

检查：
- [ ] 输出是否是有效的 JSON（使用 `python3 -m json.tool` 验证）
- [ ] 成功时 `success` 为 `true`，`data` 中包含你需要的数据
- [ ] 失败时 `success` 为 `false`，`error` 中有错误描述
- [ ] 缺少必要参数时返回错误（非 0 退出码）
- [ ] 异常参数时返回错误

### 14.2 通过系统 API 测试

创建一个 DAG 并提交，观察工具执行情况：

**Step 1** 通过 `/api/node` 创建并提交 DAG：

```bash
curl -X POST http://localhost:8080/api/node \
  -H "Content-Type: application/json" \
  -d '{
    "nodes": [
      {
        "id": "test-1",
        "type": "TOOL",
        "name": "bash",
        "input": {
          "command": "python3 /opt/tools/weather_tool.py --city 北京"
        }
      }
    ],
    "edges": []
  }'
```

**Step 2** 保存返回的 `taskId`，查询执行结果：

```bash
# 查看节点详情（包含 stdout、exitCode、durationMs）
curl http://localhost:8080/api/task/<taskId>
```

**Step 3** 查看完整的链路追踪：

```bash
# 查看任务链路（含执行指标、上下文记录）
curl http://localhost:8080/api/trace/<taskId>
```

### 14.3 前端调试按钮

系统前端页面右下角有一个 **🔍 调试按钮**，点击即可查看最近一次任务的完整链路数据，包括你的工具执行耗时、stdout 输出等。

---

## 15. 在 DAG 中使用工具

### 15.1 最简单的场景：单个工具

```json
{
  "nodes": [
    {
      "id": "my-tool",
      "type": "TOOL",
      "name": "bash",
      "input": {
        "command": "python3 /opt/tools/my_tool.py --input 数据A"
      }
    }
  ],
  "edges": []
}
```

### 15.2 串联场景：A → B → C

```json
{
  "nodes": [
    {
      "id": "step-1",
      "type": "TOOL",
      "name": "bash",
      "input": {
        "command": "python3 /opt/tools/fetch_data.py"
      }
    },
    {
      "id": "step-2",
      "type": "TOOL",
      "name": "bash",
      "input": {
        "command": "python3 /opt/tools/process_data.py"
      }
    },
    {
      "id": "step-3",
      "type": "TOOL",
      "name": "bash",
      "input": {
        "command": "python3 /opt/tools/save_result.py"
      }
    }
  ],
  "edges": [
    {"from": "step-1", "to": "step-2"},
    {"from": "step-2", "to": "step-3"}
  ]
}
```

### 15.3 条件分支场景

```json
{
  "nodes": [
    {
      "id": "check",
      "type": "TOOL",
      "name": "bash",
      "input": {
        "command": "python3 /opt/tools/check_status.py"
      }
    },
    {
      "id": "success-handler",
      "type": "TOOL",
      "name": "bash",
      "condition": "check.status == success",
      "input": {
        "command": "python3 /opt/tools/on_success.py"
      }
    },
    {
      "id": "fail-handler",
      "type": "TOOL",
      "name": "bash",
      "condition": "check.status == failed",
      "input": {
        "command": "python3 /opt/tools/on_fail.py"
      }
    }
  ],
  "edges": [
    {"from": "check", "to": "success-handler"},
    {"from": "check", "to": "fail-handler"}
  ]
}
```

### 15.4 混合使用内置工具和自定义脚本

```json
{
  "nodes": [
    {
      "id": "generate-content",
      "type": "LLM",
      "name": "llm_api",
      "input": {
        "prompt": "写一段关于北京春天的短文"
      }
    },
    {
      "id": "save-to-file",
      "type": "TOOL",
      "name": "bash",
      "input": {
        "command": "python3 /opt/tools/save_content.py"
      }
    }
  ],
  "edges": [
    {"from": "generate-content", "to": "save-to-file"}
  ]
}
```

---

## 16. 常见问题

### Q: 脚本的 stdout 只能输出 JSON 吗？能不能输出日志？

日志写到 **stderr**。系统会捕获 stderr 但不会将它视为执行结果。

```python
import sys
# 日志输出到 stderr（不会影响执行结果）
print("正在查询天气...", file=sys.stderr)
# 结果输出到 stdout（系统解析）
print(json.dumps({"success": true, "data": result}))
```

### Q: 脚本执行超时怎么办？

系统默认超时是 120 秒。如果脚本需要更长时间，需要在 DAG 节点配置中设置（未来支持通过节点参数调整超时时间）。

```python
# 脚本侧也可以自行控制超时
import signal

def handler(signum, frame):
    print(json.dumps({"success": false, "error": "执行超时"}))
    exit(1)

signal.alarm(60)  # 60 秒后触发超时
```

### Q: 我的脚本需要安装第三方依赖怎么办？

两种方案：

1. **虚拟环境方案**：在脚本中使用完整路径指定 Python 虚拟环境：
   ```json
   {"command": "/opt/venv/bin/python3 /opt/tools/my_tool.py --input xxx"}
   ```

2. **容器方案**（推荐）：将你的脚本和依赖打包成 Docker 镜像，通过 shell 命令调用：
   ```json
   {"command": "docker run --rm my-tool-image --input xxx"}
   ```

### Q: 我的工具需要访问数据库怎么办？

系统连接的数据库信息会通过环境变量注入：
- `POSTGRES_HOST`、`POSTGRES_PORT`、`POSTGRES_DB`、`POSTGRES_USER`、`POSTGRES_PASSWORD`

你的脚本可以通过 `os.environ` 读取这些环境变量。

### Q: 脚本输出太大怎么办？

目前 stdout 直接存储在节点数据库中。如果输出非常大（例如超过 1MB），建议将数据写入文件，然后在 stdout 中只输出文件路径：

```python
result = {"outputRef": "/data/outputs/result_20260426.json"}
print(json.dumps({"success": true, "data": result}))
```

### Q: 脚本更新后需要重启系统吗？

如果你使用**方式一（独立脚本）**，不需要重启系统。直接更新脚本文件，下一次 DAG 调用就会使用新版本。

如果你使用**方式二（Go 内置插件）**，需要重新编译并重启系统。

### Q: 如何调试脚本执行问题？

1. 先在终端直接运行脚本，验证输出
2. 查看系统日志：`docker compose logs -f lingxi-ai-os`
3. 查看任务追踪：`curl http://localhost:8080/api/trace/<taskId>`
4. 前端点击右下角 🔍 按钮查看最近任务链路

---

## 17. 附录：完整输出示例

### 17.1 成功执行

节点 output 的完整结构（通过 `/api/trace/:taskId` 查看）：

```json
{
  "task": {
    "taskId": "20260426231320-68686850",
    "status": "SUCCESS",
    "nodes": [
      {
        "id": "weather-query",
        "taskId": "20260426231320-68686850",
        "type": "TOOL",
        "name": "bash",
        "status": "SUCCESS",
        "input": {
          "command": "python3 /opt/tools/weather_tool.py --city 北京"
        },
        "output": {
          "exitCode": 0,
          "stdout": "{\"success\": true, \"data\": {\"city\": \"北京\", \"temperature\": \"25°C\", \"condition\": \"晴天\"}}",
          "stderr": "",
          "durationMs": 1254
        },
        "retryCount": 0,
        "maxRetry": 3
      }
    ]
  },
  "contexts": [
    {
      "contextType": "NODE_READY",
      "sourceModule": "StateMachine",
      "message": "初始节点就绪（无依赖），进入 READY 状态"
    },
    {
      "contextType": "NODE_SCHEDULED",
      "nodeId": "weather-query",
      "sourceModule": "ContextService",
      "metadata": {
        "startedAt": "2026-04-26T23:13:21.048766+08:00"
      },
      "message": "Kafka 事件记录：节点进入运行状态"
    },
    {
      "contextType": "NODE_SUCCESS",
      "nodeId": "weather-query",
      "sourceModule": "ContextService",
      "metadata": {
        "durationMs": 1254,
        "exitCode": 0
      },
      "message": "Kafka 事件记录：节点执行成功"
    }
  ]
}
```

### 17.2 失败执行

```json
{
  "id": "weather-query-fail",
  "type": "TOOL",
  "name": "bash",
  "status": "FAILED",
  "input": {
    "command": "python3 /opt/tools/weather_tool.py"
  },
  "output": {
    "exitCode": 1,
    "stdout": "{\"success\": false, \"error\": \"缺少 --city 参数\"}",
    "stderr": "",
    "durationMs": 32,
    "error": "Tool execution failed with exit code 1"
  },
  "errorMessage": "Tool execution failed with exit code 1",
  "retryCount": 0,
  "maxRetry": 3
}
```

### 17.3 完整上下文链路（正常流程）

一个典型任务的上下文链路（8 条记录，无重复）：

| # | 类型 | 来源模块 | 说明 |
|---|------|---------|------|
| 1 | TASK_CREATED | Orchestrator | 任务创建成功 |
| 2 | DAG_VALIDATED | Orchestrator | DAG 校验通过 |
| 3 | DAG_SUBMITTED | Orchestrator | DAG 已提交 |
| 4 | NODE_READY | StateMachine | 节点就绪 |
| 5 | NODE_SCHEDULED | ContextService | 节点开始执行（含 startedAt） |
| 6 | NODE_SUCCESS | ContextService | 节点执行成功（含 durationMs、exitCode） |
| 7 | NODE_SUCCESS | StateMachine | 节点状态流转完成 |
| 8 | TASK_SUCCESS | StateMachine | 全部节点执行完毕 |

---

> **结语**：灵犀 AI OS 的工具系统设计为语言无关。无论你使用 Python、Node.js、Shell 还是其他语言，只要遵守 JSON stdout 契约，你的工具就能无缝接入系统。
>
> 遇到问题请先查看 [常见问题](#16-常见问题) 章节，或通过前端右下角 🔍 调试按钮查看任务链路排查问题。
