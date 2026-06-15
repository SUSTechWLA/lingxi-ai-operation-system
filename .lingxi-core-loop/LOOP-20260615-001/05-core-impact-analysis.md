# 05-Core-Impact-Analysis: 变更影响分析

## 受影响文件

### 数据模型层 (`internal/core/model/`)
| 文件 | 变更 | 类型 |
|------|------|------|
| `model.go` | Node 增加字段，新增 NodeStatus 常量 | 扩展 |

### 仓库层 (`internal/core/model/repository/`)
| 文件 | 变更 | 类型 |
|------|------|------|
| `interfaces.go` | NodeRepo 增加方法 | 扩展 |
| `repository.go` | 实现新方法，SQL 更新 | 扩展 |

### 编排层 (`internal/core/orchestrator/service/`)
| 文件 | 变更 | 类型 |
|------|------|------|
| `statemachine.go` | 处理 HEARTBEAT_TIMEOUT 和 checkpoint 恢复 | 扩展 |
| `state.go` | TransitionNode 处理新状态的时间戳 | 扩展 |
| `scheduler.go` | 增加心跳超时检测扫描 | 扩展 |
| `orchestrator.go` | SubmitDAG 支持 long_running 参数 | 扩展 |

### 编排层 Handler (`internal/core/orchestrator/handler/`)
| 文件 | 变更 | 类型 |
|------|------|------|
| `handler.go` | 新进度查询端点 | 扩展 |

### Worker 层 (`internal/core/worker/`)
| 文件 | 变更 | 类型 |
|------|------|------|
| `tool/tool.go` | 增加 ProgressReporter 接口 | 扩展 |
| `service/executor.go` | 支持长任务心跳上报 | 扩展 |

### 数据库 (`internal/core/database/`)
| 文件 | 变更 | 类型 |
|------|------|------|
| `database.go` | Schema 迁移增加新列 | 扩展 |

### 事件总线 (`internal/core/eventbus/`)
| 文件 | 变更 | 类型 |
|------|------|------|
| `eventbus.go` | Progress 事件消费者（main.go 中注册） | 扩展 |

### 入口 (`cmd/tangying-ai-os/`)
| 文件 | 变更 | 类型 |
|------|------|------|
| `main.go` | 注册新 Consumer，注册新路由 | 扩展 |

## 向后兼容性

| 方面 | 兼容策略 |
|------|---------|
| Node 新字段 | 全部使用零值默认（long_running=false, progress=0） |
| 新状态 | 仅长任务节点可达新状态 |
| API | 只在 node 对象内增加字段，不删除/修改已有字段 |
| 数据库 | ADD COLUMN IF NOT EXISTS，默认值兼容 |
| 现有测试 | 不修改任何现有测试断言 |

## 风险

| 风险 | 缓解 |
|------|------|
| 心跳扫描增加 DB 负载 | 使用索引 `idx_node_status_heartbeat`，间隔 30s |
| Kafka 事件量增加 | Progress 事件限流（每个节点每秒最多 1 个） |
| Redis 缓存不一致 | 使用短 TTL（30s），以 DB 为真相源 |
