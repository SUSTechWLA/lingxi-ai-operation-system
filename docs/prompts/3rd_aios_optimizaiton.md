# 🚀 灵犀AI OS Worker 层终极架构设计 Skill 文档
## **多语言/多类型工具统一编排方案**（编排层无感知、Worker 完全解耦、100%兼容现有系统）
> **核心定位**：你是灵犀AI OS 架构师，本次设计**彻底解决多语言/多类型工具接入问题**。
> 核心约束：**编排层(Orchestrator) 永远只认识 Node，完全不感知工具的语言、实现、类型**；所有差异由 Worker 层屏蔽；工具完全自治、可独立部署。
> 适用工具：Python/Java/C++/JS/Shell脚本、LLM API、三方平台API、深度学习模型、自定义算法。

---

## 一、核心设计结论（一句话讲透）
### 架构模型：**三层绝对解耦**
1. **编排层（Orchestrator）**：只处理 `Node` 任务，不关心工具是什么语言、怎么实现
2. **Worker 层（网关+注册中心）**：**唯一中间层**，屏蔽所有工具差异，负责：工具注册、协议转换、执行转发、事件上报
3. **工具层（任意语言/类型）**：标准化黑盒服务，只暴露统一接口，不依赖任何系统组件

### 通信方案：**统一 HTTP/JSON 协议**
✅ 所有语言（Java/C++/Python/JS/Shell）都原生支持
✅ 无侵入、无SDK、无依赖
✅ 调试简单、可观测性强
✅ 完全跨平台、跨进程、跨服务器

---

## 二、整体架构图（必须严格遵守）
```
┌─────────────────────┐
│   编排层 Orchestrator │  ← 只认【Node】，不知工具实现
└──────────┬───────────┘
           │ 事件驱动(Redpanda)：下发Node / 接收执行结果
┌──────────▼───────────┐
│    Worker 网关模块    │  ← 核心：屏蔽多语言/多类型差异
│    (独立部署SpringBoot)
│  1.工具注册中心 2.协议转换 3.执行代理 4.事件上报
└──────────┬───────────┘
           │ 统一HTTP/JSON协议（与语言无关）
┌──────────┴───────────┐
│        工具层         │  ← 任意语言、任意类型、独立运行
│ Python/C++/Java/JS/脚本/LLM/深度学习/三方API
└──────────────────────┘
```

---

## 三、核心设计规范（铁律，不可修改）
1. **编排层无感知**：Orchestrator 只存储/调度 `Node`，不存储任何工具源码、运行时、语言信息
2. **工具黑盒化**：Worker 不托管工具、不加载工具代码、不运行工具进程
3. **协议统一化**：所有工具必须实现 **3个标准HTTP接口**，与语言/类型无关
4. **注册元数据驱动**：工具通过注册暴露能力，Worker 同步给编排层，生成可编排 Node
5. **无中心化依赖**：工具可独立部署、独立重启、独立升级，不影响 Worker/编排层

---

## 四、多语言工具统一标准（核心！所有工具必须遵守）
### 4.1 统一接口（所有语言/类型工具，必须实现这3个接口）
| 接口地址 | 请求方式 | 作用 |
|----------|----------|------|
| `/tool/info`  | GET      | 获取工具元数据（注册用） |
| `/tool/run`   | POST     | 执行工具（接收参数，返回结果） |
| `/tool/health`| GET      | 健康检查 |

### 4.2 统一数据格式（JSON）
**所有工具，无论语言，输入输出完全一致**
```json
// 1. /info 工具元数据（注册到编排层，生成Node）
{
  "toolId": "weather_query_v1",    // 全局唯一标识
  "toolName": "城市天气查询",
  "description": "查询任意城市实时天气", // 向量库用
  "type": "THIRD_API",             // 工具类型：LLM/API/SCRIPT/DL/CORE
  "inputSchema": {},               // 输入规范
  "outputSchema": {},              // 输出规范
  "timeout": 10000
}

// 2. /run 执行请求（Worker → 工具）
{
  "taskId": "t1", "nodeId": "n1", "input": {}
}

// 3. /run 执行结果（工具 → Worker）
{
  "code": 200, "message": "ok", "data": {}, "error": null
}
```

### 4.3 工具分类（支持你所有场景）
| 工具类型 | 说明 | 示例 |
|----------|------|------|
| `LLM` | 大模型API调用 | OpenAI/通义千问 |
| `API` | 三方平台API | 微信/支付宝/高德 |
| `SCRIPT` | 脚本执行 | Shell/Python/JS 脚本 |
| `DL` | 深度学习模型 | Pytorch/TensorFlow C++/Python |
| `CORE` | 自研核心功能 | Java/C++ 服务 |

---

## 五、Worker 模块核心职责（独立部署，屏蔽所有差异）
### 5.1 Worker 核心功能
1. **工具注册服务**
   - 提供 HTTP 注册接口：`/worker/register`
   - 接收工具地址 → 拉取元数据 → 校验 → 上报编排层
2. **工具管理中心**
   - 内存缓存工具地址、类型、元数据
   - 定时健康检查
3. **统一执行代理**
   - 监听 Redpanda：`ai.node.ready`
   - 根据 Node 中的 `toolId` 找到对应工具
   - 转发请求 → 接收结果 → 上报编排层
4. **事件同步**
   - 工具注册/上下线 → 通知编排层更新 Node 可用列表
   - 工具执行日志 → 上报 AI-Context

### 5.2 Worker 不做的事情（绝对不做）
❌ 不加载任何工具代码
❌ 不运行任何工具进程
❌ 不关心工具用什么语言编写
❌ 不存储工具业务逻辑
❌ 不耦合工具实现

---

## 六、Node 与工具映射规则（编排层无感知的关键）
### 6.1 编排层只认这一个 Node 结构
```json
{
  "nodeId": "n_001",
  "taskId": "t_001",
  "toolId": "weather_query_v1", // 唯一关联：工具唯一ID
  "input": { "city": "北京" },
  "status": "READY"
}
```
### 6.2 映射逻辑
1. 工具注册 → 生成 `toolId`
2. Worker 把 `toolId` + 元数据同步给编排层
3. 编排层只使用 `toolId` 调度 Node
4. Worker 根据 `toolId` 找到工具地址并转发执行
> **编排层永远不知道这个 toolId 对应的是Python、C++ 还是脚本**

---

## 七、端到端流程（全链路闭环）
```
1. 【工具启动】
   Python/C++/Java/脚本工具启动，暴露 /info /run /health 接口

2. 【工具注册】
   工具调用 Worker: /worker/register → 传入工具地址
   Worker 拉取元数据 → 发送 ai.tool.registered 事件 → 编排层保存工具

3. 【编排调度】
   Orchestrator 创建 Node → 携带 toolId → 发给 Worker

4. 【Worker 转发执行】
   Worker 根据 toolId 找到工具 → HTTP 调用 /run → 屏蔽语言差异

5. 【结果上报】
   工具返回结果 → Worker 封装成 NodeResult → 发给编排层
```

---

## 八、多语言工具适配模板（直接可用）
### 8.1 Python 工具（LLM/深度学习/脚本）
```python
from fastapi import FastAPI
app = FastAPI()
@app.get("/info")
def info(): return {"toolId":"py_llm","toolName":"Python大模型","type":"LLM"}
@app.post("/run")
def run(data: dict): return {"code":200,"data":"result"}
@app.get("/health")
def health(): return {"status":"UP"}
```

### 8.2 C++ 工具（高性能深度学习）
```cpp
// 使用 C++ HTTP 库（crow/librhtp）
// 暴露 /info /run /health 三个接口，返回JSON
```

### 8.3 Java 工具（自研核心服务）
```java
@RestController
public class ToolController {
    @GetMapping("/info")
    public Map info() { return Map.of("toolId","java_core","type","CORE"); }
}
```

### 8.4 Shell/JS 脚本工具
```bash
# 用 curl + 简单HTTP服务暴露统一接口
```

---

## 九、向量库复用 + 自进化支持（你的核心需求）
### 9.1 向量库复用
- 工具注册时，Worker 把 `description + inputSchema` 生成向量
- 存入 PostgreSQL `vector` 字段
- NL-Translator 语义检索工具 → **减少 LLM Token 消耗**

### 9.2 自进化
- 工具执行统计（成功率/耗时/调用量）存入元数据
- 自动推荐最优工具
- 新工具注册时，自动匹配相似能力，避免重复开发

---

## 十、数据库设计（仅扩展，不破坏现有表）
```sql
-- 工具注册表（编排层存储，Worker同步）
CREATE TABLE ai_tool (
  tool_id VARCHAR(64) PRIMARY KEY,  -- Node 绑定的唯一标识
  tool_name VARCHAR(100) NOT NULL,
  description TEXT,                 -- 向量生成用
  type VARCHAR(32),                 -- LLM/API/SCRIPT/DL
  meta JSONB NOT NULL,              -- 完整元数据
  endpoint VARCHAR(255) NOT NULL,    -- 工具地址
  status VARCHAR(32) DEFAULT 'ONLINE',
  embedding vector(1536),           -- 向量字段
  create_time DATETIME DEFAULT NOW()
);
```

---

## 十一、给 Claude 的强制编码规范
1. **Worker 是独立 SpringBoot 应用**，端口 `8083`，独立部署
2. **不侵入任何现有模块**，兼容 Redpanda/PostgreSQL/Node 格式
3. **工具完全黑盒**，Worker 只通过 HTTP 调用
4. **编排层只认 toolId**，不感知语言/类型
5. **必须包含**：注册接口、事件消费、工具转发、健康检查、元数据管理
6. **必须提供**：Python/Java 多语言工具示例
7. **所有事件格式**与现有系统 100% 兼容

---

## 十二、验收标准
1. ✅ 启动 Worker，无依赖，独立运行
2. ✅ Python/C++/Java/脚本工具均可注册
3. ✅ 编排层下发 Node（仅携带 toolId）
4. ✅ Worker 自动转发执行，返回结果
5. ✅ 编排层完全不感知工具语言
6. ✅ 工具元数据入库，支持向量检索
7. ✅ 全链路可观测，执行日志上报 AI-Context

---

# 🎯 你直接复制给 Claude，指令：
> **按照这份 Skill 文档，生成灵犀AI OS Worker 层完整代码**
> 要求：独立部署、支持多语言工具、编排层无感知、HTTP统一协议、带工具注册+执行代理+事件上报，同时提供 Python/Java 工具示例。