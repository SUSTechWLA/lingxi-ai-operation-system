# 灵犀AI OS AI-Worker 模块完整指南

> **状态**: ✅ 已实现 - 本文档描述当前已实现的模块

---

## 目录

1. [什么是 AI-Worker？（新手必看）](#什么是-ai-worker新手必看)
2. [5分钟快速上手](#5分钟快速上手)
3. [核心架构（看图就能懂）](#核心架构看图就能懂)
4. [工具注册完整流程](#工具注册完整流程)
5. [数据流程图解](#数据流程图解)
6. [我想改代码，从哪入手？](#我想改代码从哪入手)
7. [常见问题](#常见问题)

---

## 什么是 AI-Worker？（新手必看）

### 用生活中的例子理解

想象一个**餐厅**：

| 角色 | 餐厅类比 | AI-Worker中的角色 |
|------|---------|------------------|
| 客人 | 来吃饭的人 | 用户 |
| 服务员 | 记菜单、传话 | NL-Translator |
| 厨师长 | 安排做菜顺序 | Orchestrator |
| **帮厨** | **真正做菜的人** | **AI-Worker** |
| 灶台/烤箱 | 做菜的工具 | 各种工具（LLM/数据库/搜索等） |

**AI-Worker 就是那个帮厨！** 它不决定做什么菜（那是厨师长Orchestrator的事），它只负责：
1. 收到指令 → 2. 拿出合适的工具 → 3. 用工具干活 → 4. 把结果交回去

### AI-Worker 到底能做什么？

| 功能 | 说明 |
|------|------|
| 🛠️ **工具注册中心** | 管理各种工具（像工具箱） |
| 📡 **执行代理** | 把任务转发给合适的工具 |
| 📝 **事件上报** | 告诉大家任务完成了 |
| ❤️ **健康检查** | 确保工具都在正常工作 |

---

## 5分钟快速上手

### 第1步：启动Worker

```bash
cd ai-worker
mvn spring-boot:run
```

看到这个就成功了：
```
Started WorkerApplication in 3.456 seconds
```

### 第2步：启动一个测试工具

打开一个**新终端**：

```bash
cd examples
pip install fastapi uvicorn pydantic
python test_tool_final.py
```

测试工具会在端口8092启动。

### 第3步：注册工具

再打开一个**新终端**：

```bash
cd examples
./final_test.sh
```

或者手动注册：

```bash
curl -X POST http://localhost:8083/worker/register \
  -H "Content-Type: application/json" \
  -d '{"endpoint":"http://localhost:8092"}'
```

### 第4步：查看已注册的工具

```bash
curl http://localhost:8083/worker/tools
```

完成！🎉 你已经成功注册了第一个工具！

---

## 核心架构（看图就能懂）

### 整体架构图

```
┌─────────────────────────────────────────────────────────────┐
│                    用户请求来了！                        │
└────────────────────────────┬────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│  NL-Translator (把自然语言翻译成任务)               │
└────────────────────────────┬────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│  Orchestrator (调度器：安排任务顺序)                  │
│  发布事件：ai.node.ready                                 │
└────────────────────────────┬────────────────────────────────┘
                             │ 事件驱动（Redpanda消息队列）
                             ▼
    ┌───────────────────────────────────────────────────┐
    │         AI-Worker (帮厨：真正执行的人)         │
    │  ┌─────────────────────────────────────────────┐  │
    │  │  1. 监听事件：收到 ai.node.ready         │  │
    │  │  2. 查找工具：找到对应的工具地址          │  │
    │  │  3. 转发执行：调用工具的 /run 接口       │  │
    │  │  4. 返回结果：发布 ai.node.result        │  │
    │  └─────────────────────────────────────────────┘  │
    └────────────────────────────┬──────────────────────────┘
                                 │
            ┌────────────────────┼────────────────────┐
            │                    │                    │
            ▼                    ▼                    ▼
    ┌──────────┐         ┌──────────┐         ┌──────────┐
    │ Python   │         │  Java    │         │  C++     │
    │  工具    │         │  工具    │         │  工具    │
    └──────────┘         └──────────┘         └──────────┘
         │                    │                    │
         └────────────────────┴────────────────────┘
                              │
                    统一HTTP接口：
                    /info  - 获取工具信息
                    /run   - 执行工具
                    /health - 健康检查
```

### 项目结构（新手版）

```
ai-worker/
├── 📄 pom.xml                          (Maven配置，不用改)
└── src/main/java/com/lingxi/ai/worker/
    ├── 🚀 WorkerApplication.java         (启动入口，双击运行)
    │
    ├── 📁 controller/                   (对外API接口)
    │   ├── WorkerController.java        (基础API：健康检查等)
    │   └── ExternalToolController.java  (工具注册API)
    │
    ├── 📁 service/                      (核心业务逻辑)
    │   └── NodeExecutor.java            (节点执行器：干活的)
    │
    ├── 📁 event/                        (事件处理)
    │   ├── EventConsumer.java           (接收事件)
    │   └── EventProducer.java           (发送事件)
    │
    ├── 📁 externaltool/                 (外部工具支持 ⭐ 重点看这里)
    │   ├── client/
    │   │   └── ExternalToolClient.java  (调用外部工具的客户端)
    │   ├── registry/
    │   │   └── ExternalToolRegistry.java (工具注册表：工具箱)
    │   ├── service/
    │   │   ├── ToolRegistrationService.java  (注册工具)
    │   │   └── ToolHealthCheckService.java (检查工具健康)
    │   └── model/                     (数据模型)
    │
    ├── 📁 tool/                        (工具接口定义)
    │   ├── Tool.java                   (工具接口)
    │   ├── ToolRegistry.java           (内置工具注册表)
    │   ├── spi/                      (SPI：可插拔扩展)
    │   └── builtin/                  (内置工具)
    │
    ├── 📁 model/                       (数据模型)
    ├── 📁 config/                      (配置文件)
    └── 📁 util/                        (工具类)
```

---

## 工具注册完整流程

### 用生活类比理解工具注册

想象你在**健身房**：

1. **你带了个瑜伽垫**（启动工具）
2. **去前台登记**（调用 `/worker/register`）
3. **前台看你带了什么**（Worker调用工具的 `/info`）
4. **给你发个储物柜钥匙**（注册成功，分配ID）
5. **把你的信息写在黑板上**（发送事件通知大家）

### 完整流程时序图

```
    工具服务                    Worker                    Orchestrator
      │                         │                            │
      │  1. 启动服务             │                            │
      │  监听 8092 端口          │                            │
      │─────────────────────────▶│                            │
      │  2. 注册工具             │                            │
      │  POST /worker/register  │                            │
      │  {"endpoint":"..."}     │                            │
      │─────────────────────────▶│                            │
      │                         │                            │
      │  3. 获取元数据          │                            │
      │  GET /info              │                            │
      │◀────────────────────────│                            │
      │  返回工具信息            │                            │
      │                         │                            │
      │  4. 健康检查            │                            │
      │  GET /health            │                            │
      │◀────────────────────────│                            │
      │  返回 UP                 │                            │
      │                         │                            │
      │  5. 保存到本地缓存       │                            │
      │                         │                            │
      │  6. 发送注册事件         │                            │
      │────────────────────────────────────────────────────▶│
      │  ai.tool.registered      │                            │
      │                         │                            │
      │  7. 返回成功             │                            │
      │◀────────────────────────│                            │
      │                         │                            │
```

### 代码执行流程（想看代码的话）

1. **入口**：`ExternalToolController.registerTool()`
   - 接收HTTP请求：`POST /worker/register`
   - 参数：`{"endpoint": "http://localhost:8092"}`

2. **调用注册服务**：`ToolRegistrationService.registerTool()`
   - 调用 `ExternalToolClient.getToolInfo()` 获取工具信息
   - 调用 `ExternalToolClient.checkHealth()` 检查健康
   - 调用 `ExternalToolRegistry.registerTool()` 保存到本地
   - 发送 `ai.tool.registered` 事件

3. **存入注册表**：`ExternalToolRegistry.registerTool()`
   - 保存到内存缓存 `toolsByName` 和 `toolsByEndpoint`

---

## 数据流程图解

### 完整的数据流转

```
┌─────────────────────────────────────────────────────────────┐
│  场景：用户说"帮我查一下北京的天气"                   │
└─────────────────────────────────────────────────────────────┘

步骤1：用户请求 → NL-Translator
  │
  ▼
{
  "prompt": "帮我查一下北京的天气",
  "userId": "user_001"
}

步骤2：NL-Translator → Orchestrator
  │
  ▼
生成DAG（有向无环图）：
{
  "nodes": [{
    "nodeId": "node_001",
    "toolId": "weather_query_v1",  ← 指定用哪个工具
    "input": {"city": "北京"},
    "status": "READY"
  }]
}

步骤3：Orchestrator → Worker (通过Redpanda)
  │
  ▼
发送事件：ai.node.ready
{
  "eventId": "uuid-xxx",
  "eventType": "ai.node.ready",
  "taskId": "task_001",
  "nodeId": "node_001",
  "payload": {
    "toolId": "weather_query_v1",
    "input": {"city": "北京"}
  }
}

步骤4：Worker 接收事件
  │
  ├─→ 1. EventConsumer 监听到事件
  │
  ├─→ 2. NodeExecutor 开始执行
  │
  ├─→ 3. ExternalToolRegistry 查找工具
  │    找到：weather_query_v1 → http://localhost:8090
  │
  ├─→ 4. ExternalToolClient 调用工具
  │    POST http://localhost:8090/run
  │    {
  │      "taskId": "task_001",
  │      "nodeId": "node_001",
  │      "input": {"city": "北京"}
  │    }
  │
  ▼
步骤5：Python天气工具执行
  │
  ├─→ 1. 接收请求
  │
  ├─→ 2. 查询天气API
  │
  ├─→ 3. 返回结果
  │    {
  │      "code": 200,
  │      "message": "success",
  │      "data": {
  │        "city": "北京",
  │        "temperature": 25,
  │        "weather": "晴"
  │      }
  │    }
  │
  ▼
步骤6：Worker → Orchestrator
  │
  ├─→ 1. 接收工具返回结果
  │
  ├─→ 2. EventProducer 发送结果事件
  │
  ├─→ 3. 发送事件：ai.node.result
  │    {
  │      "eventId": "uuid-yyy",
  │      "eventType": "ai.node.result",
  │      "taskId": "task_001",
  │      "nodeId": "node_001",
  │      "status": "SUCCESS",
  │      "output": {
  │        "city": "北京",
  │        "temperature": 25
  │      }
  │    }
  │
  ▼
步骤7：Orchestrator 更新任务状态
  │
  ▼
步骤8：返回给用户最终结果
```

---

## 我想改代码，从哪入手？

### 场景1：我想添加一个新的内置工具

**步骤**：

1. 在 `tool/builtin/` 下创建新工具类
2. 实现 `Tool` 接口
3. 自动会被 `ToolRegistry` 扫描到

**示例**：

```java
// 1. 创建文件：tool/builtin/MyTool.java
@Component
public class MyTool implements Tool {

    @Override
    public String getName() {
        return "my_tool";  // 工具名称
    }

    @Override
    public String getDescription() {
        return "我的自定义工具";
    }

    @Override
    public ToolType getType() {
        return ToolType.CUSTOM;
    }

    @Override
    public ToolResult execute(Map<String, Object> parameters, ToolContext context) {
        // 在这里写你的逻辑
        Object input = parameters.get("input");
        Map<String, Object> result = Map.of("output", "处理结果: " + input);
        return ToolResult.success(result, Instant.now(), Instant.now());
    }

    @Override
    public boolean validateParameters(Map<String, Object> parameters) {
        return parameters.containsKey("input");
    }
}
```

完成！重启Worker就能用了。

---

### 场景2：我想修改工具注册逻辑

**关键文件**：`externaltool/service/ToolRegistrationService.java`

```java
@Service
public class ToolRegistrationService {

    public Mono<RegisteredTool> registerTool(String endpoint, String workerGroup) {
        // 在这里修改注册逻辑

        // 1. 获取工具信息
        toolClient.getToolInfo(endpoint)

        // 2. 健康检查
        toolClient.checkHealth(endpoint)

        // 3. 保存到注册表
        toolRegistry.registerTool(...)

        // 4. 发送事件
        publishToolRegisteredEvent(...)
    }
}
```

---

### 场景3：我想修改工具执行逻辑

**关键文件**：`service/NodeExecutor.java`

```java
@Service
public class NodeExecutor {

    public void executeNode(NodeTaskEvent event) {
        // 在这里修改执行逻辑

        // 1. 从事件中提取信息
        String toolName = determineToolName(nodeType, payload);

        // 2. 判断是内置工具还是外部工具
        if (externalToolExecutor.isExternalTool(toolName)) {
            // 外部工具：走HTTP
            externalToolExecutor.executeTool(...)
        } else {
            // 内置工具：直接调用
            tool.execute(...)
        }
    }
}
```

---

### 场景4：我想添加新的工具Provider

**关键文件**：`tool/spi/ToolProvider.java`

```java
@Component
public class MyCustomProvider implements ToolProvider {

    @Override
    public String getName() {
        return "my_provider";
    }

    @Override
    public ProviderType getType() {
        return ProviderType.LOCAL_PROCESS;  // 新类型
    }

    @Override
    public int getPriority() {
        return 50;  // 优先级，数字越小越优先
    }

    @Override
    public boolean supports(String toolName) {
        // 判断是否支持这个工具
        return toolName.startsWith("my_");
    }

    @Override
    public Tool getTool(String toolName) {
        // 返回工具实例
    }
}
```

自动会被 `ToolRouter` 发现并使用！

---

## 常见问题

### Q1: Worker和Orchestrator是什么关系？

**A**: 就像餐厅里的**厨师长**和**帮厨**：
- Orchestrator（厨师长）：安排先做什么菜，后做什么菜
- Worker（帮厨）：真正拿起工具做菜

Orchestrator不碰工具，只发号施令；Worker不做决策，只管执行。

---

### Q2: 工具一定要用Python写吗？

**A**: **不用！** 任意语言都可以！

| 语言 | 推荐HTTP框架 |
|------|------------|
| Python | FastAPI / Flask |
| Java | Spring Boot |
| C++ | Crow / libhttpserver |
| JavaScript | Express.js / Koa |
| Go | Gin / Echo |
| Rust | Axum / Warp |

只要实现3个接口就行：`/info`, `/run`, `/health`

---

### Q3: 工具注册后怎么使用？

**A**: 有两种方式：

**方式1：通过Orchestrator（推荐）**
```java
// Orchestrator会自动发现已注册的工具
// 创建Node时指定toolId即可
Node node = Node.builder()
    .toolId("weather_query_v1")
    .input(Map.of("city", "北京"))
    .build();
```

**方式2：直接调用Worker API**
```bash
curl -X POST http://localhost:8083/worker/tools/{toolName}/execute \
  -H "Content-Type: application/json" \
  -d '{"input": {...}}'
```

---

### Q4: 我想看看有哪些已注册的工具？

**A**: 调用这个API：

```bash
curl http://localhost:8083/worker/tools
```

会返回所有已注册工具的列表。

---

### Q5: 工具掉线了怎么办？

**A**: 不用担心！Worker有**健康检查机制**：

1. Worker每30秒自动检查一次工具健康
2. 如果连续90秒没响应，标记为不可用
3. 工具重新上线后，重新注册即可

---

### Q6: 怎么添加数据库配置？

**A**: 修改 `application.yml`：

```yaml
spring:
  datasource:
    url: jdbc:postgresql://localhost:5432/lingxi_ai_os
    username: postgres
    password: 你的密码
```

---

## 下一步

- 想了解Orchestrator？看 [ORCHESTRATOR_GUIDE.md](./ORCHESTRATOR_GUIDE.md)
- 想了解NL-Translator？看 [NL_TRANSLATOR_GUIDE.md](./NL_TRANSLATOR_GUIDE.md)
- 想看完整示例？看 [examples目录](../../examples/)

---

## 相关资源

- [Spring Boot官方文档](https://spring.io/projects/spring-boot)
- [Kafka/Redpanda文档](https://docs.redpanda.com/)
- [FastAPI文档](https://fastapi.tiangolo.com/)

---

祝使用愉快！🎉 有问题随时提！
