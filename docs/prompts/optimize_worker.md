# 🚀 灵犀AI OS Worker层升级 Skill 文档（Claude专属）
## 多语言工具注册与自动编排能力升级

> **核心约束**：本升级完全兼容现有灵犀AI OS架构，**不修改任何Orchestrator、AI-Context、NL-Translator的核心代码**，仅扩展Worker层能力。所有设计严格遵循"Everything is Node"、事件驱动、统一协议原则，生成的代码可直接集成到现有系统中。
>
> **本次升级核心目标**：实现**多语言工具标准化接入**、**带元数据的工具注册**、**自动生成可编排Node**，为后续工具向量复用、自进化奠定基础。

---

## 一、本次升级必须实现的3个核心功能
1. **多语言工具统一交互协议**：支持Java/Python/C++/Shell脚本等任意语言编写工具，定义标准输入输出格式
2. **全元数据工具注册机制**：工具启动时自动注册名称、描述、输入输出Schema、示例、标签等完整元数据，为向量库复用做准备
3. **自动生成可编排Node**：工具注册后，自动生成标准Node类型，告知Orchestrator和NL-Translator，可直接用于DAG编排

---

## 二、核心设计原则（必须严格遵守）
1. **工具即微服务**：每个工具是独立进程/服务，通过标准HTTP接口与Worker交互，Worker不依赖任何工具的语言运行时
2. **元数据驱动**：所有工具能力通过元数据描述，Worker和Orchestrator不需要提前知道工具的任何业务逻辑
3. **完全向后兼容**：所有事件格式、数据库表结构、Node格式与现有系统100%兼容
4. **向量友好**：所有注册元数据必须包含足够的语义信息，可直接生成向量存入向量库，实现工具的语义检索和复用
5. **无侵入式**：工具开发者只需要实现3个标准HTTP接口，不需要修改任何系统核心代码

---

## 三、多语言工具统一交互协议（核心）
所有工具无论用什么语言编写，必须暴露以下3个标准HTTP接口，使用JSON格式传输数据。

### 3.1 工具标准接口定义
| 接口 | 方法 | 功能 |
|------|------|------|
| `/tool/info` | GET | 获取工具完整元数据（注册用） |
| `/tool/execute` | POST | 执行工具，接收输入参数，返回执行结果 |
| `/tool/health` | GET | 健康检查，返回工具运行状态 |

### 3.2 接口响应格式（统一）
```json
{
  "code": 200, // 200=成功，其他=失败
  "message": "success",
  "data": {} // 业务数据
}
```

### 3.3 各接口详细规范
#### 1. `/tool/info` 元数据接口（核心）
**响应示例**：
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "toolName": "weather_query", // 工具唯一标识（全局唯一）
    "toolVersion": "1.0.0", // 语义化版本
    "description": "查询指定城市的实时天气信息，支持温度、湿度、风力等数据", // 详细功能描述（用于LLM理解和向量生成）
    "author": "lingxi-ai",
    "tags": ["天气", "查询", "工具", "生活服务"], // 标签（用于分类和检索）
    "inputSchema": { // 输入参数JSON Schema
      "type": "object",
      "properties": {
        "city": {
          "type": "string",
          "description": "要查询的城市名称，如'北京'、'上海'"
        },
        "type": {
          "type": "string",
          "description": "查询类型，可选值：realtime(实时)/forecast(预报)",
          "default": "realtime"
        }
      },
      "required": ["city"]
    },
    "outputSchema": { // 输出结果JSON Schema
      "type": "object",
      "properties": {
        "city": {
          "type": "string",
          "description": "城市名称"
        },
        "temperature": {
          "type": "number",
          "description": "实时温度，单位摄氏度"
        },
        "humidity": {
          "type": "number",
          "description": "相对湿度，百分比"
        }
      }
    },
    "examples": [ // 输入输出示例（用于LLM学习和向量生成）
      {
        "input": {"city": "北京"},
        "output": {"city": "北京", "temperature": 25, "humidity": 60}
      }
    ],
    "timeout": 5000, // 执行超时时间（毫秒）
    "maxRetry": 3 // 最大重试次数
  }
}
```

#### 2. `/tool/execute` 执行接口
**请求示例**：
```json
{
  "nodeId": "node_001",
  "taskId": "task_001",
  "traceId": "trace_abc123",
  "input": {
    "city": "北京",
    "type": "realtime"
  }
}
```

**响应示例（成功）**：
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "city": "北京",
    "temperature": 25,
    "humidity": 60
  }
}
```

**响应示例（失败）**：
```json
{
  "code": 500,
  "message": "天气查询失败",
  "data": {
    "errorCode": "CITY_NOT_FOUND",
    "errorDetail": "城市'火星'不存在"
  }
}
```

#### 3. `/tool/health` 健康检查接口
**响应示例**：
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "status": "UP", // UP/DOWN
    "version": "1.0.0",
    "timestamp": 1718600000000
  }
}
```

---

## 四、全元数据工具注册机制
### 4.1 工具注册流程
```
1. 工具开发者用任意语言编写工具，实现上述3个标准HTTP接口
2. 工具启动，监听指定端口（如8090）
3. 工具向Worker发送注册请求（或Worker主动发现工具）
4. Worker调用工具的`/tool/info`接口，获取完整元数据
5. Worker验证元数据合法性
6. Worker向Redpanda发送`ai.tool.registered`事件
7. Orchestrator消费事件，将工具元数据存入`ai_tool`表
8. Orchestrator自动生成对应Node类型，更新全局工具注册表
9. NL-Translator自动同步工具列表，可在DAG生成时使用该工具
```

### 4.2 工具注册事件格式（与现有事件格式完全兼容）
```json
{
  "event_id": "uuid",
  "event_type": "ai.tool.registered",
  "task_id": "",
  "node_id": "",
  "timestamp": 1718600000000,
  "source": "ai-worker",
  "data": {
    // 完整的工具元数据（与/tool/info接口返回的data完全一致）
    "toolName": "weather_query",
    "toolVersion": "1.0.0",
    "description": "查询指定城市的实时天气信息",
    "tags": ["天气", "查询"],
    "inputSchema": {},
    "outputSchema": {},
    "examples": [],
    "timeout": 5000,
    "maxRetry": 3,
    // 额外注册信息
    "workerId": "worker_001",
    "workerGroup": "default",
    "toolEndpoint": "http://192.168.1.100:8090" // 工具服务地址
  },
  "version": "1.0"
}
```

### 4.3 数据库表扩展（仅新增字段，不修改原有结构）
```sql
ALTER TABLE ai_tool
ADD COLUMN description TEXT,
ADD COLUMN tags JSONB,
ADD COLUMN input_schema JSONB,
ADD COLUMN output_schema JSONB,
ADD COLUMN examples JSONB,
ADD COLUMN timeout INT DEFAULT 5000,
ADD COLUMN max_retry INT DEFAULT 3,
ADD COLUMN tool_endpoint VARCHAR(255),
ADD COLUMN embedding vector(1536); -- 预留向量字段，用于后续语义检索
```

### 4.4 工具心跳与注销
- **心跳**：Worker每30秒调用一次工具的`/tool/health`接口，更新`ai_tool`表的`last_heartbeat`字段
- **过期机制**：Orchestrator如果90秒未收到某个工具的心跳，自动将其状态标记为`UNAVAILABLE`
- **注销**：工具优雅关闭时，发送`ai.tool.unregistered`事件，Orchestrator将其状态标记为`OFFLINE`

---

## 五、自动生成可编排Node
### 5.1 工具与Node的映射规则
每个工具自动对应一个标准Node类型，映射规则如下：
| 工具元数据 | Node字段 |
|------------|----------|
| toolName | node.name |
| toolVersion | node.input.toolVersion |
| inputSchema | node.input |
| outputSchema | node.output |
| timeout | node.timeout |
| maxRetry | node.retry.max |

### 5.2 自动生成的Node格式（与现有Node格式完全兼容）
```json
{
  "nodeId": "node_001",
  "taskId": "task_001",
  "type": "TOOL",
  "name": "weather_query", // 与工具名称一致
  "status": "READY",
  "input": {
    "city": "北京",
    "type": "realtime",
    "toolVersion": "1.0.0"
  },
  "retry": {
    "count": 0,
    "max": 3 // 与工具maxRetry一致
  },
  "timeout": 5000, // 与工具timeout一致
  "priority": 5,
  "workerGroup": "default",
  "traceId": "trace_abc123"
}
```

### 5.3 编排层自动发现
Orchestrator收到工具注册事件后，自动更新全局工具注册表，并向NL-Translator发送`ai.tool.available`事件。NL-Translator收到事件后，自动将该工具加入工具列表，在生成DAG时可以直接使用该工具生成对应的Node。

---

## 六、Worker层核心执行流程
### 6.1 Worker启动流程
```
1. 启动Spring Boot应用
2. 初始化Redpanda消费者和生产者
3. 初始化工具注册表（本地缓存）
4. 启动工具健康检查定时器（每30秒执行一次）
5. 启动事件消费线程，监听`ai.node.ready` Topic
6. 等待工具注册请求
```

### 6.2 工具注册处理流程
```
1. 接收工具注册请求（POST /worker/register）
2. 调用工具的`/tool/info`接口，获取元数据
3. 验证元数据合法性（必填字段、JSON Schema格式）
4. 调用工具的`/tool/health`接口，验证工具可用性
5. 将工具元数据存入本地缓存
6. 向Redpanda发送`ai.tool.registered`事件
7. 返回注册成功响应
```

### 6.3 Node执行流程（与现有流程完全兼容）
```
1. 消费Redpanda的`ai.node.ready`事件
2. 解析Node信息，获取工具名称和版本
3. 从本地工具注册表中查找对应工具的endpoint
4. 调用工具的`/tool/execute`接口，传入Node的input
5. 捕获所有异常，记录执行时间
6. 构造`ai.node.result`事件，填充output或error信息
7. 发送事件到Redpanda的`ai.node.result` Topic
8. 发送`ai.context.saved`事件，记录完整执行过程
```

---

## 七、第一个多语言工具示例（Python）
```python
# weather_tool.py
from fastapi import FastAPI
from pydantic import BaseModel
from typing import Optional, Dict, Any

app = FastAPI()

# 工具元数据
TOOL_META = {
    "toolName": "weather_query",
    "toolVersion": "1.0.0",
    "description": "查询指定城市的实时天气信息，支持温度、湿度、风力等数据",
    "author": "lingxi-ai",
    "tags": ["天气", "查询", "工具", "生活服务"],
    "inputSchema": {
        "type": "object",
        "properties": {
            "city": {"type": "string", "description": "要查询的城市名称"},
            "type": {"type": "string", "description": "查询类型", "default": "realtime"}
        },
        "required": ["city"]
    },
    "outputSchema": {
        "type": "object",
        "properties": {
            "city": {"type": "string"},
            "temperature": {"type": "number"},
            "humidity": {"type": "number"}
        }
    },
    "examples": [
        {"input": {"city": "北京"}, "output": {"city": "北京", "temperature": 25, "humidity": 60}}
    ],
    "timeout": 5000,
    "maxRetry": 3
}

class ExecuteRequest(BaseModel):
    nodeId: str
    taskId: str
    traceId: str
    input: Dict[str, Any]

@app.get("/tool/info")
async def get_info():
    return {"code": 200, "message": "success", "data": TOOL_META}

@app.post("/tool/execute")
async def execute(request: ExecuteRequest):
    try:
        city = request.input["city"]
        # 模拟天气查询
        result = {
            "city": city,
            "temperature": 25,
            "humidity": 60
        }
        return {"code": 200, "message": "success", "data": result}
    except Exception as e:
        return {"code": 500, "message": str(e), "data": {"errorCode": "EXECUTE_FAILED"}}

@app.get("/tool/health")
async def health():
    return {"code": 200, "message": "success", "data": {"status": "UP", "version": "1.0.0", "timestamp": 1718600000000}}

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8090)
```

---

## 八、向量复用预留设计（为后续迭代做准备）
本次升级预留所有向量复用需要的字段和接口，后续只需添加以下功能即可实现工具的语义检索和复用：
1. 工具注册时，自动将`description`、`tags`、`examples`生成embedding，存入`ai_tool`表的`embedding`字段
2. NL-Translator生成DAG时，先从向量库中检索相似工具，优先使用已有的工具，减少LLM Token消耗
3. 收集工具调用的统计数据（成功率、耗时、用户反馈），自动优化工具的排序和推荐

---

## 九、验收标准（Claude生成代码后必须验证）
1. ✅ 启动Worker服务，端口8083，不修改任何现有系统代码
2. ✅ 启动上述Python天气工具，访问`http://localhost:8090/tool/info`能看到完整元数据
3. ✅ 执行`curl -X POST http://localhost:8083/worker/register -d '{"endpoint":"http://localhost:8090"}'`，工具注册成功
4. ✅ Orchestrator的`ai_tool`表中能看到已注册的天气工具，所有元数据完整
5. ✅ 执行`curl -X POST http://localhost:8080/api/task -d '{"prompt":"查询北京的天气"}'`
6. ✅ NL-Translator自动生成包含`weather_query`工具的单节点DAG
7. ✅ Worker成功调用Python工具，返回天气结果
8. ✅ Orchestrator更新任务状态为SUCCESS，AI-Context能查询到完整执行记录

---

## 十、给Claude的专属指令
⚠️ 生成代码时必须严格遵守以下要求：
1. 所有代码必须与现有灵犀AI OS系统100%兼容，不得修改Orchestrator、AI-Context、NL-Translator的任何核心代码
2. 所有事件格式必须与现有系统完全一致，使用spring-kafka客户端与Redpanda交互
3. 所有数据库操作必须复用现有JPA/MyBatis配置，仅扩展`ai_tool`表的字段
4. 生成的Worker模块是独立的Spring Boot 3应用，端口8083，与现有模块完全解耦
5. 必须包含完整的异常处理、日志记录和健康检查
6. 必须包含上述Python天气工具的示例代码和注册说明
7. 生成完成后，自动编写测试用例，验证所有验收标准
8. 优先复用现有系统的工具类、配置类和常量，不得重复定义

> 记住：**工具是独立的，Worker只是工具的代理和注册中心**。Worker不应该包含任何工具的业务逻辑，所有业务逻辑都在工具服务中实现。