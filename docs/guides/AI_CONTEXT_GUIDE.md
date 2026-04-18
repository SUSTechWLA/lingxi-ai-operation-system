# 灵犀AI OS AI-Context 模块完整指南

> 本文档适用于零基础开发者，帮助你快速理解和使用 AI-Context 模块。

---

## 目录

1. [什么是 AI-Context？](#什么是-ai-context)
2. [核心功能](#核心功能)
3. [技术栈](#技术栈)
4. [项目结构](#项目结构)
5. [类定义详解](#类定义详解)
6. [API 接口](#api-接口)
7. [快速开始](#快速开始)

---

## 什么是 AI-Context？

**AI-Context（上下文管理器）** 是灵犀AI OS的上下文管理层，负责：

- 记录任务执行过程中的所有操作历史
- 保存节点的快照数据，支持故障恢复
- 提供历史状态追溯能力
- 支持跨模块的上下文查询

简单来说，AI-Context 就像飞机的"黑匣子"，记录一切，支持回溯。

### 定位

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           AIOS 模块分层                                       │
└─────────────────────────────────────────────────────────────────────────────┘

  用户入口层 ──▶ NL-Translator ──▶ Orchestrator ──▶ Worker (规划中)
                                    │
                                    ▼
                              AI-Context ◀────── 记录上下文
                                    │
                              持久化存储 (PostgreSQL)
```

---

## 核心功能

### 1. 任务上下文记录
- 记录任务创建、成功、失败等关键事件
- 记录 DAG 提交和验证事件
- 提供任务级别的历史查询

### 2. 节点上下文记录
- 记录节点调度、就绪、成功、失败、重试等事件
- 记录节点间的依赖关系变化

### 3. 节点快照管理
- 保存节点的完整状态快照
- 支持从快照恢复节点状态
- 用于故障恢复和调试

### 4. 上下文查询
- 按任务ID查询上下文历史
- 按节点ID查询快照
- 支持时间范围查询

---

## 技术栈

| 技术 | 版本 | 用途 |
|------|------|------|
| Java | 17+ | 开发语言 |
| Spring Boot | 3.2.5 | 应用框架 |
| Spring Data JPA | 3.2.x | 数据访问 |
| PostgreSQL | 16 | 持久化存储 (jsonb) |
| Maven | 3.9.x | 构建工具 |
| Lombok | 1.18.32 | 简化代码 |

---

## 项目结构

```
ai-context/
├── pom.xml
└── src/main/java/com/lingxi/ai/context/
    ├── AiContextApplication.java        # 应用启动入口
    ├── entity/                          # 实体层
    │   ├── Context.java                # 上下文实体
    │   └── ContextType.java            # 上下文类型枚举
    ├── repository/                      # 仓储层
    │   └── ContextRepository.java      # 数据访问接口
    ├── service/                        # 服务层
    │   └── ContextService.java         # 业务服务
    ├── controller/                     # 控制器层
    │   └── ContextController.java      # REST API
    └── config/                         # 配置层
        └── DatabaseInitializer.java    # 数据库初始化
```

---

## 类定义详解

### 枚举类

#### ContextType（上下文类型枚举）
**文件位置**: `entity/ContextType.java`

定义了上下文的各种类型：

```java
public enum ContextType {
    TASK_CREATED,       // 任务已创建
    TASK_SUCCESS,       // 任务成功完成
    TASK_FAILED,        // 任务执行失败
    NODE_SCHEDULED,     // 节点已调度
    NODE_READY,         // 节点就绪
    NODE_SUCCESS,       // 节点执行成功
    NODE_FAILED,        // 节点执行失败
    NODE_RETRY,         // 节点重试
    NODE_SNAPSHOT,      // 节点快照
    DAG_SUBMITTED,      // DAG 已提交
    DAG_VALIDATED       // DAG 已验证
}
```

### 实体类

#### Context（上下文实体）
**文件位置**: `entity/Context.java`

代表一条上下文记录：

| 字段 | 类型 | 说明 |
|------|------|------|
| id | Long | 主键，自增 |
| contextType | ContextType | 上下文类型 |
| taskId | String | 所属任务ID |
| nodeId | String | 所属节点ID（可选） |
| metadata | Map<String, Object> | 元数据（jsonb） |
| message | String | 描述信息 |
| snapshotData | Map<String, Object> | 快照数据（jsonb） |
| createdAt | LocalDateTime | 创建时间 |

**主要方法**：

```java
// 创建基础上下文
public static Context create(ContextType type, String taskId, String message)

// 创建带节点ID的上下文
public static Context create(ContextType type, String taskId, String nodeId, String message)

// 创建节点快照
public static Context createSnapshot(String taskId, String nodeId,
    Map<String, Object> snapshotData, String message)
```

### 仓储类

#### ContextRepository
**文件位置**: `repository/ContextRepository.java`

```java
public interface ContextRepository extends JpaRepository<Context, Long> {

    // 按任务ID查询上下文（按时间倒序）
    List<Context> findByTaskIdOrderByCreatedAtDesc(String taskId);

    // 按节点ID和类型查询上下文
    List<Context> findByNodeIdAndContextTypeOrderByCreatedAtDesc(
        String nodeId, ContextType contextType);

    // 按任务ID和类型查询
    List<Context> findByTaskIdAndContextType(String taskId, ContextType contextType);
}
```

### 服务类

#### ContextService
**文件位置**: `service/ContextService.java`

提供完整的上下文管理服务：

**任务上下文方法**：

| 方法 | 说明 |
|------|------|
| `recordTaskCreated(taskId)` | 记录任务创建 |
| `recordTaskSuccess(taskId)` | 记录任务成功 |
| `recordTaskFailed(taskId)` | 记录任务失败 |
| `recordDagSubmitted(taskId)` | 记录DAG提交 |
| `recordDagValidated(taskId)` | 记录DAG验证 |

**节点上下文方法**：

| 方法 | 说明 |
|------|------|
| `recordNodeScheduled(taskId, nodeId, nodeType, nodeName)` | 记录节点调度 |
| `recordNodeReady(taskId, nodeId)` | 记录节点就绪 |
| `recordNodeSuccess(taskId, nodeId)` | 记录节点成功 |
| `recordNodeFailed(taskId, nodeId, errorMessage)` | 记录节点失败 |
| `recordNodeRetry(taskId, nodeId, retryCount, maxRetry)` | 记录节点重试 |

**快照方法**：

| 方法 | 说明 |
|------|------|
| `recordNodeSnapshot(...)` | 记录节点快照（完整状态） |
| `getLatestSnapshotForNode(nodeId)` | 获取节点最新快照 |
| `restoreNodeFromSnapshot(nodeId)` | 从快照恢复节点 |

**查询方法**：

| 方法 | 说明 |
|------|------|
| `getContextForTask(taskId)` | 获取任务的完整上下文历史 |

### 控制器类

#### ContextController
**文件位置**: `controller/ContextController.java`

提供 REST API 接口：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/task/{taskId}/context` | 获取任务上下文 |
| GET | `/api/node/{nodeId}/snapshot/latest` | 获取节点最新快照 |
| POST | `/api/context/task-created` | 记录任务创建 |
| POST | `/api/context/task-success` | 记录任务成功 |
| POST | `/api/context/task-failed` | 记录任务失败 |
| POST | `/api/context/dag-submitted` | 记录DAG提交 |
| POST | `/api/context/node-scheduled` | 记录节点调度 |
| POST | `/api/context/node-ready` | 记录节点就绪 |
| POST | `/api/context/node-success` | 记录节点成功 |
| POST | `/api/context/node-failed` | 记录节点失败 |
| POST | `/api/context/node-retry` | 记录节点重试 |
| POST | `/api/context/node-snapshot` | 记录节点快照 |
| GET | `/api/health` | 健康检查 |

---

## API 接口

### 基础信息

- **基础URL**: `http://localhost:8082`
- **内容类型**: `application/json`

### API 示例

#### 1. 健康检查

```bash
curl http://localhost:8082/api/health
```

响应：
```json
{"status": "UP", "service": "ai-context"}
```

#### 2. 记录任务创建

```bash
curl -X POST http://localhost:8082/api/context/task-created \
  -H "Content-Type: application/json" \
  -d '{"taskId": "task-001"}'
```

#### 3. 查询任务上下文

```bash
curl http://localhost:8082/api/task/task-001/context
```

响应示例：
```json
[
  {
    "id": 1,
    "contextType": "TASK_CREATED",
    "taskId": "task-001",
    "message": "Task created",
    "createdAt": "2024-01-15T10:30:00"
  },
  {
    "id": 2,
    "contextType": "DAG_SUBMITTED",
    "taskId": "task-001",
    "message": "DAG submitted successfully",
    "createdAt": "2024-01-15T10:30:05"
  }
]
```

#### 4. 记录节点快照

```bash
curl -X POST http://localhost:8082/api/context/node-snapshot \
  -H "Content-Type: application/json" \
  -d '{
    "taskId": "task-001",
    "nodeId": "node-001",
    "type": "LLM",
    "name": "write_article",
    "status": "RUNNING",
    "input": {"topic": "AI"},
    "output": {},
    "retryCount": 0,
    "maxRetry": 3,
    "priority": 5,
    "workerGroup": "default",
    "version": 1,
    "errorMessage": null
  }'
```

#### 5. 获取节点最新快照

```bash
curl http://localhost:8082/api/node/node-001/snapshot/latest
```

响应示例：
```json
{
  "nodeId": "node-001",
  "snapshot": {
    "nodeId": "node-001",
    "taskId": "task-001",
    "type": "LLM",
    "name": "write_article",
    "status": "RUNNING",
    "input": {"topic": "AI"},
    "output": {},
    "retryCount": 0,
    "maxRetry": 3,
    "priority": 5,
    "workerGroup": "default",
    "version": 1,
    "snapshotTime": 1705312200000
  }
}
```

---

## 快速开始

### 步骤 1：启动 PostgreSQL

确保 PostgreSQL 已启动：

```bash
# 使用 docker 启动
docker run -d \
  --name lingxi-postgres \
  -e POSTGRES_USER=wanglian \
  -e POSTGRES_PASSWORD=123 \
  -e POSTGRES_DB=lingxi_db \
  -p 5432:5432 \
  postgres:16-alpine
```

### 步骤 2：编译项目

```bash
cd ai-context
mvn clean compile
```

### 步骤 3：启动应用

```bash
mvn spring-boot:run
```

应用会在 `http://localhost:8082` 启动。

### 步骤 4：测试 API

```bash
# 健康检查
curl http://localhost:8082/api/health

# 记录任务创建
curl -X POST http://localhost:8082/api/context/task-created \
  -H "Content-Type: application/json" \
  -d '{"taskId": "test-001"}'

# 查询上下文
curl http://localhost:8082/api/task/test-001/context
```

---

## 与其他模块的交互

### Orchestrator 调用 AI-Context

```
Orchestrator                          AI-Context
    │                                      │
    │  POST /api/context/task-created       │
    │  POST /api/context/node-scheduled     │
    │  POST /api/context/node-success       │
    │  POST /api/context/node-snapshot      │
    │ ────────────────────────────────────▶ │
    │                                      │
    │  GET /api/task/{id}/context           │
    │ ◀──────────────────────────────────── │
```

### 配置项

在 `application.yml` 中配置：

```yaml
server:
  port: 8082

spring:
  datasource:
    url: jdbc:postgresql://localhost:{port}/lingxi_db
    username: {username}
    password: {password}

context:
  service:
    url: http://localhost:8082
```

---

## 数据库表结构

```sql
CREATE TABLE ai_context (
    id BIGSERIAL PRIMARY KEY,
    context_type VARCHAR(50),
    task_id VARCHAR(64),
    node_id VARCHAR(64),
    metadata JSONB,
    message VARCHAR(1000),
    snapshot_data JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_context_task ON ai_context(task_id);
CREATE INDEX idx_context_node ON ai_context(node_id);
```

---

## 下一步

- 了解 [Orchestrator 模块](./ORCHESTRATOR_GUIDE.md) 如何与 Context 交互
- 了解 [NL-Translator 模块](./NL_TRANSLATOR_GUIDE.md) 如何调用 Orchestrator

---

## 常见问题

**Q: AI-Context 和 Orchestrator 的状态存储有什么区别？**
A: Orchestrator 存储运行时状态（当前状态），AI-Context 存储历史记录（操作日志）。

**Q: 快照数据保存在哪里？**
A: 快照数据以 JSONB 格式保存在 PostgreSQL 的 `snapshot_data` 字段中。

**Q: 如何清理历史数据？**
A: 可以通过 `ContextRepository.deleteAll()` 或按时间条件删除旧记录。
