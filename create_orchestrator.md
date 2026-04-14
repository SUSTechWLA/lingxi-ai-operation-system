# 🚀 灵犀AI OS Orchestrator（Redpanda生产级）最终版SDD
> **⚠️ 给AI代码生成工具的强约束指令**：本SDD是唯一开发标准，必须100%严格遵守，不得修改任何技术选型、数据结构、流程和约束。生成完成后自动执行所有测试步骤，验证全流程跑通。
>
> **目标**：生成一个可直接启动、自动测试通过的分布式AI编排内核，基于Redpanda事件驱动，无状态、可水平扩展。

---

## 📁 一、项目结构（必须严格遵守）
```
ai-orchestrator/
├── pom.xml
├── docker-compose.yml
├── src/main/java/com/lingxi/ai/orchestrator/
│   ├── OrchestratorApplication.java
│   ├── controller/
│   │   └── TaskController.java
│   ├── service/
│   │   ├── OrchestratorService.java
│   │   ├── PlannerService.java
│   │   └── StateMachineService.java
│   ├── model/
│   │   ├── Task.java
│   │   ├── Node.java
│   │   ├── DAG.java
│   │   ├── TaskStatus.java
│   │   ├── NodeStatus.java
│   │   ├── NodeTaskEvent.java
│   │   └── NodeResultEvent.java
│   ├── event/
│   │   ├── EventProducer.java
│   │   └── EventConsumer.java
│   ├── repository/
│   │   └── RedisTaskRepository.java
│   └── config/
│       ├── KafkaConfig.java
│       └── RedisConfig.java
└── src/main/resources/
    └── application.yml
```

---

## 🛠️ 二、技术栈（强约束，不得修改）
| 技术 | 版本要求 | 用途 |
|------|----------|------|
| Java | 17+ | 开发语言 |
| Spring Boot | 3.2.x | 应用框架 |
| Spring Kafka | 3.1.x | Redpanda客户端 |
| Redpanda | v23.3.11 | 事件总线（Kafka兼容） |
| Redis | 7.x | 状态存储+分布式锁 |
| Maven | 3.9.x | 构建工具 |
| Lombok | 最新 | 简化代码 |
| Jackson | 最新 | JSON序列化 |

---

## 🐳 三、依赖启动配置
### 3.1 docker-compose.yml（必须包含）
```yaml
version: "3.9"

services:
  postgres:
    image: postgres:16-alpine
    container_name: lingxi-postgres
    environment:
      POSTGRES_USER: wanglian
      POSTGRES_PASSWORD: 123
      POSTGRES_DB: lingxi_db
    ports:
      - "5432:5432"

  redis:
    image: redis:7-alpine
    container_name: lingxi-redis
    ports:
      - "6379:6379"

  # ✅ Kafka 替代（更轻量 & Mac友好）
  redpanda:
    image: redpandadata/redpanda:latest
    container_name: lingxi-redpanda
    command:
      - redpanda start
      - --overprovisioned
      - --smp 1
      - --memory 512M
      - --reserve-memory 0M
    ports:
      - "9092:9092"

  minio:
    image: minio/minio:latest
    container_name: lingxi-minio
    environment:
      MINIO_ROOT_USER: wanglian
      MINIO_ROOT_PASSWORD: 12345678
    command: server /data --console-address ":9001"
    ports:
      - "9000:9000"
      - "9001:9001"

  # ⭐ 向量数据库（替代 Milvus）
  qdrant:
    image: qdrant/qdrant:latest
    container_name: lingxi-qdrant
    ports:
      - "6333:6333"
      - "6334:6334"
    volumes:
      - ./qdrant_storage:/qdrant/storage   # 👈 加这个
```

### 3.2 pom.xml核心依赖（必须包含）
```xml
<parent>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-parent</artifactId>
    <version>3.2.5</version>
    <relativePath/>
</parent>

<dependencies>
    <!-- Spring Boot Web -->
    <dependency>
        <groupId>org.springframework.boot</groupId>
        <artifactId>spring-boot-starter-web</artifactId>
    </dependency>

    <!-- Spring Kafka (Redpanda兼容) -->
    <dependency>
        <groupId>org.springframework.kafka</groupId>
        <artifactId>spring-kafka</artifactId>
    </dependency>

    <!-- Spring Data Redis -->
    <dependency>
        <groupId>org.springframework.boot</groupId>
        <artifactId>spring-boot-starter-data-redis</artifactId>
    </dependency>

    <!-- Lombok -->
    <dependency>
        <groupId>org.projectlombok</groupId>
        <artifactId>lombok</artifactId>
        <optional>true</optional>
    </dependency>

    <!-- Jackson -->
    <dependency>
        <groupId>com.fasterxml.jackson.core</groupId>
        <artifactId>jackson-databind</artifactId>
    </dependency>

    <!-- Test -->
    <dependency>
        <groupId>org.springframework.boot</groupId>
        <artifactId>spring-boot-starter-test</artifactId>
        <scope>test</scope>
    </dependency>
    <dependency>
        <groupId>org.springframework.kafka</groupId>
        <artifactId>spring-kafka-test</artifactId>
        <scope>test</scope>
    </dependency>
</dependencies>
```

### 3.3 application.yml（必须包含）
```yaml
server:
  port: 8080

spring:
  application:
    name: ai-orchestrator

  kafka:
    bootstrap-servers: localhost:9092
    consumer:
      group-id: orchestrator-group
      auto-offset-reset: earliest
      key-deserializer: org.apache.kafka.common.serialization.StringDeserializer
      value-deserializer: org.springframework.kafka.support.serializer.JsonDeserializer
      properties:
        spring.json.trusted.packages: "*"
    producer:
      acks: all
      key-serializer: org.apache.kafka.common.serialization.StringSerializer
      value-serializer: org.springframework.kafka.support.serializer.JsonSerializer

  data:
    redis:
      host: localhost
      port: 6379
      timeout: 2000

logging:
  level:
    com.lingxi.ai.orchestrator: INFO
    org.springframework.kafka: INFO
```

---

## 📊 四、核心数据结构（必须100%一致）
### 4.1 状态枚举
```java
// TaskStatus.java
public enum TaskStatus {
    CREATED, RUNNING, SUCCESS, FAILED
}

// NodeStatus.java
public enum NodeStatus {
    PENDING, RUNNING, SUCCESS, FAILED
}
```

### 4.2 事件模型
```java
// NodeTaskEvent.java（发给Worker）
@Data
@NoArgsConstructor
@AllArgsConstructor
public class NodeTaskEvent {
    private String taskId;
    private String nodeId;
    private String type; // LLM / TOOL
    private Map<String, Object> payload;
    private String traceId;
}

// NodeResultEvent.java（Worker返回）
@Data
@NoArgsConstructor
@AllArgsConstructor
public class NodeResultEvent {
    private String taskId;
    private String nodeId;
    private NodeStatus status;
    private Map<String, Object> output;
    private String traceId;
    private String errorMessage; // 失败时填充
}
```

### 4.3 领域模型
```java
// Task.java
@Data
public class Task {
    private String taskId;
    private String prompt;
    private TaskStatus status;
    private DAG dag;
    private String traceId;
    private long createTime;
    private long startTime;
    private long endTime;
}

// Node.java
@Data
public class Node {
    private String nodeId;
    private String type;
    private String task;
    private List<String> deps;
    private NodeStatus status;
    private Map<String, Object> input;
    private Map<String, Object> output;
    private String errorMessage;
}

// DAG.java
@Data
public class DAG {
    private List<Node> nodes;

    // 生成示例DAG（2个节点，依赖关系1→2）
    public static DAG sample() {
        DAG dag = new DAG();
        Node node1 = new Node();
        node1.setNodeId("1");
        node1.setType("LLM");
        node1.setTask("write_article");
        node1.setDeps(Collections.emptyList());
        node1.setStatus(NodeStatus.PENDING);

        Node node2 = new Node();
        node2.setNodeId("2");
        node2.setType("LLM");
        node2.setTask("summarize");
        node2.setDeps(Arrays.asList("1"));
        node2.setStatus(NodeStatus.PENDING);

        dag.setNodes(Arrays.asList(node1, node2));
        return dag;
    }

    // 获取所有可执行节点（依赖全部完成）
    public List<Node> getReadyNodes() {
        return nodes.stream()
                .filter(node -> node.getStatus() == NodeStatus.PENDING)
                .filter(node -> node.getDeps().stream()
                        .allMatch(depId -> nodes.stream()
                                .anyMatch(n -> n.getNodeId().equals(depId) && n.getStatus() == NodeStatus.SUCCESS)))
                .collect(Collectors.toList());
    }

    // 检查DAG是否完成
    public boolean isCompleted() {
        return nodes.stream().allMatch(node -> node.getStatus() == NodeStatus.SUCCESS);
    }

    // 检查DAG是否失败
    public boolean isFailed() {
        return nodes.stream().anyMatch(node -> node.getStatus() == NodeStatus.FAILED);
    }
}
```

---

## 🎯 五、Redpanda Topic设计（必须创建）
```text
ai.task.created    # 任务创建事件
ai.node.ready      # 节点就绪事件（发给Worker）
ai.node.result     # 节点执行结果事件（Worker返回）
ai.task.completed  # 任务完成事件
ai.task.failed     # 任务失败事件
```

---

## 🧩 六、模块设计与职责
### 6.1 Controller层
```java
// TaskController.java
@RestController
@RequestMapping("/api/task")
public class TaskController {
    @Autowired
    private OrchestratorService orchestratorService;

    // 创建任务
    @PostMapping
    public ResponseEntity<Task> createTask(@RequestBody Map<String, String> request) {
        String prompt = request.get("prompt");
        Task task = orchestratorService.createTask(prompt);
        return ResponseEntity.ok(task);
    }

    // 查询任务状态
    @GetMapping("/{taskId}")
    public ResponseEntity<Task> getTask(@PathVariable String taskId) {
        Task task = orchestratorService.getTask(taskId);
        return ResponseEntity.ok(task);
    }
}
```

### 6.2 Service层
#### OrchestratorService（核心调度）
```java
@Service
public class OrchestratorService {
    @Autowired
    private PlannerService plannerService;
    @Autowired
    private StateMachineService stateMachineService;
    @Autowired
    private EventProducer eventProducer;
    @Autowired
    private RedisTaskRepository taskRepository;

    // 创建任务
    public Task createTask(String prompt) {
        // 1. 生成唯一ID
        String taskId = UUID.randomUUID().toString();
        String traceId = UUID.randomUUID().toString();

        // 2. 生成DAG
        DAG dag = plannerService.plan(prompt);

        // 3. 创建任务
        Task task = new Task();
        task.setTaskId(taskId);
        task.setPrompt(prompt);
        task.setStatus(TaskStatus.CREATED);
        task.setDag(dag);
        task.setTraceId(traceId);
        task.setCreateTime(System.currentTimeMillis());

        // 4. 保存到Redis
        taskRepository.saveTask(task);

        // 5. 发布任务创建事件
        eventProducer.sendTaskCreatedEvent(task);

        // 6. 启动任务执行
        startTask(task);

        return task;
    }

    // 启动任务
    public void startTask(Task task) {
        // 更新任务状态为RUNNING
        task.setStatus(TaskStatus.RUNNING);
        task.setStartTime(System.currentTimeMillis());
        taskRepository.saveTask(task);

        // 调度所有就绪节点
        scheduleReadyNodes(task);
    }

    // 调度就绪节点
    public void scheduleReadyNodes(Task task) {
        List<Node> readyNodes = task.getDag().getReadyNodes();
        for (Node node : readyNodes) {
            // 更新节点状态为RUNNING
            node.setStatus(NodeStatus.RUNNING);
            taskRepository.saveNode(task.getTaskId(), node);

            // 发布节点就绪事件
            NodeTaskEvent event = new NodeTaskEvent(
                    task.getTaskId(),
                    node.getNodeId(),
                    node.getType(),
                    node.getInput(),
                    task.getTraceId()
            );
            eventProducer.sendNodeReadyEvent(event);
        }
    }

    // 查询任务
    public Task getTask(String taskId) {
        return taskRepository.getTask(taskId);
    }
}
```

#### PlannerService（Mock实现）
```java
@Service
public class PlannerService {
    // Mock规划，返回固定2节点DAG
    public DAG plan(String prompt) {
        return DAG.sample();
    }
}
```

#### StateMachineService（状态机管理）
```java
@Service
public class StateMachineService {
    @Autowired
    private OrchestratorService orchestratorService;
    @Autowired
    private EventProducer eventProducer;
    @Autowired
    private RedisTaskRepository taskRepository;

    // 处理节点执行结果
    public void handleNodeResult(NodeResultEvent event) {
        String taskId = event.getTaskId();
        String nodeId = event.getNodeId();

        // 1. 获取任务和节点
        Task task = taskRepository.getTask(taskId);
        Node node = task.getDag().getNodes().stream()
                .filter(n -> n.getNodeId().equals(nodeId))
                .findFirst()
                .orElseThrow(() -> new RuntimeException("Node not found: " + nodeId));

        // 2. 更新节点状态
        node.setStatus(event.getStatus());
        node.setOutput(event.getOutput());
        node.setErrorMessage(event.getErrorMessage());
        taskRepository.saveNode(taskId, node);

        // 3. 检查任务状态
        if (task.getDag().isFailed()) {
            // 任务失败
            task.setStatus(TaskStatus.FAILED);
            task.setEndTime(System.currentTimeMillis());
            taskRepository.saveTask(task);
            eventProducer.sendTaskFailedEvent(task);
            return;
        }

        if (task.getDag().isCompleted()) {
            // 任务成功
            task.setStatus(TaskStatus.SUCCESS);
            task.setEndTime(System.currentTimeMillis());
            taskRepository.saveTask(task);
            eventProducer.sendTaskCompletedEvent(task);
            return;
        }

        // 4. 调度下一批就绪节点
        orchestratorService.scheduleReadyNodes(task);
    }
}
```

### 6.3 Event层
#### EventProducer（事件发送）
```java
@Service
public class EventProducer {
    @Autowired
    private KafkaTemplate<String, Object> kafkaTemplate;

    public void sendTaskCreatedEvent(Task task) {
        kafkaTemplate.send("ai.task.created", task.getTaskId(), task);
    }

    public void sendNodeReadyEvent(NodeTaskEvent event) {
        kafkaTemplate.send("ai.node.ready", event.getTaskId() + "-" + event.getNodeId(), event);
    }

    public void sendTaskCompletedEvent(Task task) {
        kafkaTemplate.send("ai.task.completed", task.getTaskId(), task);
    }

    public void sendTaskFailedEvent(Task task) {
        kafkaTemplate.send("ai.task.failed", task.getTaskId(), task);
    }
}
```

#### EventConsumer（事件消费）
```java
@Service
public class EventConsumer {
    @Autowired
    private StateMachineService stateMachineService;
    @Autowired
    private WorkerService workerService; // 内置Mock Worker

    // 消费节点就绪事件（内置Worker执行）
    @KafkaListener(topics = "ai.node.ready", groupId = "worker-group")
    public void handleNodeReady(NodeTaskEvent event) {
        workerService.executeTask(event);
    }

    // 消费节点结果事件
    @KafkaListener(topics = "ai.node.result", groupId = "orchestrator-group")
    public void handleNodeResult(NodeResultEvent event) {
        stateMachineService.handleNodeResult(event);
    }
}
```

### 6.4 WorkerService（内置Mock Worker）
```java
@Service
public class WorkerService {
    @Autowired
    private KafkaTemplate<String, Object> kafkaTemplate;

    // 执行任务（Mock实现，模拟延迟）
    public void executeTask(NodeTaskEvent event) {
        try {
            // 模拟执行延迟
            Thread.sleep(1000);

            // 构造结果
            NodeResultEvent result = new NodeResultEvent();
            result.setTaskId(event.getTaskId());
            result.setNodeId(event.getNodeId());
            result.setStatus(NodeStatus.SUCCESS);
            result.setTraceId(event.getTraceId());

            // Mock输出
            Map<String, Object> output = new HashMap<>();
            if (event.getTask().equals("write_article")) {
                output.put("content", "这是一篇AI生成的文章...");
            } else if (event.getTask().equals("summarize")) {
                output.put("summary", "这是文章的摘要...");
            }
            result.setOutput(output);

            // 发送结果
            kafkaTemplate.send("ai.node.result", event.getTaskId() + "-" + event.getNodeId(), result);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            // 发送失败结果
            NodeResultEvent result = new NodeResultEvent();
            result.setTaskId(event.getTaskId());
            result.setNodeId(event.getNodeId());
            result.setStatus(NodeStatus.FAILED);
            result.setTraceId(event.getTraceId());
            result.setErrorMessage("执行中断");
            kafkaTemplate.send("ai.node.result", event.getTaskId() + "-" + event.getNodeId(), result);
        }
    }
}
```

### 6.5 Repository层
```java
@Repository
public class RedisTaskRepository {
    @Autowired
    private RedisTemplate<String, Object> redisTemplate;

    private static final String TASK_KEY_PREFIX = "task:";
    private static final String NODE_KEY_PREFIX = "node:";

    // 保存任务
    public void saveTask(Task task) {
        redisTemplate.opsForValue().set(TASK_KEY_PREFIX + task.getTaskId(), task);
    }

    // 获取任务
    public Task getTask(String taskId) {
        return (Task) redisTemplate.opsForValue().get(TASK_KEY_PREFIX + taskId);
    }

    // 保存节点
    public void saveNode(String taskId, Node node) {
        redisTemplate.opsForValue().set(NODE_KEY_PREFIX + taskId + ":" + node.getNodeId(), node);
        // 同时更新任务中的节点
        Task task = getTask(taskId);
        task.getDag().getNodes().stream()
                .filter(n -> n.getNodeId().equals(node.getNodeId()))
                .findFirst()
                .ifPresent(n -> {
                    n.setStatus(node.getStatus());
                    n.setOutput(node.getOutput());
                    n.setErrorMessage(node.getErrorMessage());
                });
        saveTask(task);
    }
}
```

---

## 🔁 七、完整执行链路（必须严格实现）
```text
1. 用户POST /api/task {"prompt":"写文章并生成摘要"}
2. Orchestrator生成taskId和traceId
3. Planner生成2节点DAG（write_article → summarize）
4. 任务保存到Redis，状态为CREATED
5. 发布ai.task.created事件
6. 任务状态更新为RUNNING，调度就绪节点（node1）
7. 发布ai.node.ready事件
8. Worker消费事件，执行node1（模拟1秒延迟）
9. Worker发布ai.node.result事件（SUCCESS）
10. Orchestrator更新node1状态为SUCCESS
11. 检查DAG，发现node2依赖已完成，调度node2
12. 发布ai.node.ready事件
13. Worker消费事件，执行node2（模拟1秒延迟）
14. Worker发布ai.node.result事件（SUCCESS）
15. Orchestrator更新node2状态为SUCCESS
16. 检查DAG已全部完成，更新任务状态为SUCCESS
17. 发布ai.task.completed事件
```

---

## 🧪 八、自动测试要求（AI必须执行）
### 8.1 启动步骤
```bash
# 1. 启动依赖
docker-compose up -d

# 2. 等待依赖健康检查通过
sleep 10

# 3. 启动应用
mvn spring-boot:run
```

### 8.2 测试步骤
```bash
# 1. 创建任务
TASK_ID=$(curl -s -X POST http://localhost:8080/api/task \
  -H "Content-Type: application/json" \
  -d '{"prompt":"写文章并生成摘要"}' | jq -r '.taskId')

echo "创建任务成功，Task ID: $TASK_ID"

# 2. 等待5秒让任务执行
sleep 5

# 3. 查询任务状态
curl -s http://localhost:8080/api/task/$TASK_ID | jq .
```

### 8.3 必须验证的检查点
✅ 任务状态最终为`SUCCESS`  
✅ DAG包含2个节点  
✅ 两个节点状态均为`SUCCESS`  
✅ 节点1输出包含`content`字段  
✅ 节点2输出包含`summary`字段  
✅ Redpanda所有topic都有消息产生  
✅ Redis中存在`task:{taskId}`和`node:{taskId}:1`、`node:{taskId}:2`键

---

## ⚠️ 九、工程约束（必须严格遵守）
### ❌ 绝对禁止
1. Orchestrator直接执行任何LLM或工具调用
2. Worker直接调用Orchestrator的任何接口
3. 任何同步调用Worker的代码
4. 不经过Redpanda的直接通信
5. 硬编码任何业务逻辑到Orchestrator内核
6. 修改任何核心数据结构

### ✅ 必须做到
1. 所有节点执行必须通过Redpanda事件分发
2. 所有状态必须存储在Redis中
3. Orchestrator完全无状态，可水平扩展
4. 所有异常必须捕获并返回合理的错误信息
5. 所有日志必须包含traceId
6. 所有方法必须有清晰的注释

---

## 🎯 十、给AI的最终指令
> 请严格按照本SDD生成完整的项目代码，确保所有文件都在正确的位置，所有依赖都正确配置。生成完成后自动执行以下操作：
> 1. 执行`docker-compose up -d`启动Redpanda和Redis
> 2. 等待依赖健康检查通过
> 3. 执行`mvn spring-boot:run`启动应用
> 4. 等待应用启动完成
> 5. 执行上述测试步骤
> 6. 验证所有检查点都通过
> 7. 如果有任何错误，自动修复并重新测试
> 8. 所有测试通过后，输出"✅ Orchestrator项目生成并测试通过！"