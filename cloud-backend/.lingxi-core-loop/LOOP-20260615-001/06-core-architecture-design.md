# 06-Core-Architecture-Design: 长任务支持架构

## 1. Node 模型扩展

```go
type Node struct {
    // ... 已有字段保持不变 ...
    
    // 长任务支持
    LongRunning bool        `json:"longRunning"`           // 是否为长任务节点
    Progress    float64     `json:"progress"`              // 0.0 ~ 1.0
    CurrentStep string      `json:"currentStep,omitempty"` // 当前步骤描述
    HeartbeatAt *time.Time  `json:"heartbeatAt,omitempty"` // 最后心跳时间
}
```

## 2. 新增节点状态

```go
const (
    // ... 已有状态 ...
    NodeHeartbeatTimeout NodeStatus = "HEARTBEAT_TIMEOUT" // 心跳超时（可触发重试）
)
```

状态转换扩展：
```
RUNNING ──(心跳超时 5min)──→ HEARTBEAT_TIMEOUT ──→ RETRYING → CREATED
RUNNING ──(checkpoint)──→ 保留 progress/checkpoint
```

## 3. ProgressReporter 工具接口

```go
// ProgressUpdate 进度更新
type ProgressUpdate struct {
    Progress   float64                `json:"progress"`   // 0.0 ~ 1.0
    Step       string                 `json:"step"`       // 当前步骤描述
    Checkpoint map[string]interface{} `json:"checkpoint,omitempty"` // 断点数据
}

// ProgressReporter 可选接口，支持进度的 Tool 实现
type ProgressReporter interface {
    SetProgressCallback(cb func(ctx context.Context, update ProgressUpdate))
}
```

## 4. 心跳与进度流程

```
┌──────────┐    progress     ┌─────────┐   ai.node.progress   ┌──────────────┐
│  Worker   │ ─────────────→ │  Kafka  │ ──────────────────→  │ Orchestrator │
│ NodeExec  │   + heartbeat  │         │                      │  Consumer    │
└──────────┘                 └─────────┘                      └──────┬───────┘
                                                                    │
                              ┌─────────────────────────────────────┤
                              │                                     │
                              ▼                                     ▼
                     ┌──────────────┐                      ┌──────────────┐
                     │  PostgreSQL  │                      │    Redis     │
                     │  (真相源)     │                      │  (快速缓存)  │
                     └──────────────┘                      └──────────────┘
```

### 4.1 NodeExecutor 扩展

对 long_running=true 的节点，NodeExecutor 启动一个 heartbeat goroutine：

```go
func (ne *NodeExecutor) ExecuteNode(ctx context.Context, event eventbus.Event) {
    // ... 现有逻辑 ...
    
    // 长任务：启动心跳 goroutine
    if isLongRunning {
        ctx, cancel := context.WithCancel(ctx)
        defer cancel()
        go ne.heartbeatLoop(ctx, taskID, nodeID, 30*time.Second)
    }
    
    // ... 执行工具 ...
}
```

### 4.2 进度回调链

```
Tool (Execute) 
  → cb(ProgressUpdate{Progress: 0.5, Step: "processing..."})
    → NodeExecutor.reportProgress()
      → Kafka publish "ai.node.progress"
        → Orchestrator Consumer
          → DB UPDATE (progress, current_step, heartbeat_at)
          → Redis SET (task:{taskId}:node:{nodeId}:progress)
```

## 5. Scheduler 心跳超时检测

在现有 `recoverStaleCreatedNodes` 基础上增加 `recoverStaleRunningNodes`：

```go
func (s *Scheduler) detectHeartbeatTimeout(ctx context.Context) {
    // 查找 long_running=true, status=RUNNING, heartbeat_at < now - 5min
    nodes := s.nodeRepo.FindStaleRunningNodes(ctx, 5*time.Minute)
    for _, node := range nodes {
        // 转换为 HEARTBEAT_TIMEOUT
        s.stateService.TransitionNode(ctx, node.ID, model.NodeHeartbeatTimeout, nil, 
            "heartbeat timeout: no heartbeat for 5 minutes")
        // 触发重试
        s.stateMachine.OnHeartbeatTimeout(ctx, node.ID)
    }
}
```

## 6. Checkpoint 恢复

### 6.1 Checkpoint 保存
Tool 在 `ProgressUpdate` 中设置 `Checkpoint` 字段。NodeExecutor 将 checkpoint 保存到 `ai_context` 表的 `snapshot_data`：

```go
// 在 reportProgress 中
if update.Checkpoint != nil {
    contextRepo.Save(ctx, &model.Context{
        ContextType:  "NODE_CHECKPOINT",
        TaskID:       taskID,
        NodeID:       nodeID,
        SnapshotData: update.Checkpoint,
    })
}
```

### 6.2 Checkpoint 恢复
节点重试时（RETRYING → CREATED → READY → RUNNING），NodeExecutor 查询最新 checkpoint 并注入到 ToolContext：

```go
type ToolContext struct {
    TaskID     string
    NodeID     string
    RetryCount int
    Checkpoint map[string]interface{} // 新增
}

// NodeExecutor 执行前获取 checkpoint
if node.RetryCount > 0 {
    snapshot := contextRepo.FindLatestSnapshotByNodeID(ctx, node.ID)
    if snapshot != nil {
        toolCtx.Checkpoint = snapshot.SnapshotData
    }
}
```

## 7. API 端点

### GET /api/task/:taskId/progress

返回任务级聚合进度：

```json
{
    "code": 200,
    "data": {
        "taskId": "xxx",
        "status": "RUNNING",
        "progress": 0.65,
        "totalNodes": 5,
        "completedNodes": 3,
        "nodes": [
            {
                "nodeId": "node-1",
                "name": "generate_chapter_3",
                "status": "RUNNING",
                "progress": 0.75,
                "currentStep": "正在生成第3章第2节...",
                "heartbeatAt": "2026-06-15T10:30:00Z"
            }
        ]
    }
}
```

### 已有端点扩展

`GET /api/task/:taskId` 返回的 node 对象增加 `progress`、`currentStep`、`heartbeatAt` 字段。

## 8. 数据库迁移

```sql
ALTER TABLE ai_node ADD COLUMN IF NOT EXISTS long_running BOOLEAN DEFAULT FALSE;
ALTER TABLE ai_node ADD COLUMN IF NOT EXISTS progress DOUBLE PRECISION DEFAULT 0.0;
ALTER TABLE ai_node ADD COLUMN IF NOT EXISTS current_step VARCHAR(500) DEFAULT '';
ALTER TABLE ai_node ADD COLUMN IF NOT EXISTS heartbeat_at TIMESTAMP;

CREATE INDEX IF NOT EXISTS idx_node_heartbeat ON ai_node(heartbeat_at) WHERE long_running = TRUE;
```

## 9. DAG 节点声明长任务

在 DAG NodeRequest 中增加可选字段：

```go
type NodeRequest struct {
    // ... 已有字段 ...
    LongRunning bool   `json:"longRunning,omitempty"` // 是否长任务
    HeartbeatTimeoutSec *int `json:"heartbeatTimeoutSec,omitempty"` // 自定义心跳超时
}
```

## 10. 配置扩展

```go
type WorkerConfig struct {
    // ... 已有字段 ...
    HeartbeatIntervalSec int `json:"heartbeatIntervalSec"` // 默认 30
    HeartbeatTimeoutSec  int `json:"heartbeatTimeoutSec"`  // 默认 300 (5min)
}
```
