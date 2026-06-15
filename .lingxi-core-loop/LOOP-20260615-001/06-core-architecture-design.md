# 06-Core-Architecture-Design: Core 架构设计

## 1. CONTROL 节点设计

### 设计原则
1. 复用现有节点生命周期 (`CREATED → READY → RUNNING → SUCCESS/FAILED`)
2. 最小侵入: 仅在 `StateService.TransitionNode` 中增加 CONTROL 类型判断
3. 审核操作复用现有 `POST /api/node/:nodeId/success` 和 `/failure`
4. Worker 层不感知 CONTROL 节点（Kafka 不调度）

### 接口设计

#### StateService.TransitionNode 增强

```go
func (s *StateService) TransitionNode(ctx context.Context, nodeID string, newStatus NodeStatus) error {
    // ... existing transition logic ...
    
    // CONTROL node: auto-pause task on READY
    if node.Type == NodeTypeControl && newStatus == NodeStatusReady {
        pauseReason := fmt.Sprintf("Control node '%s' requires review", node.Name)
        s.TransitionTask(ctx, node.TaskID, TaskPaused)
        // Save pause reason to task
        s.taskRepo.UpdatePauseReason(ctx, node.TaskID, pauseReason)
        // Emit review-required event via outbox
        s.eventSaver.SaveEvent(ctx, "task", node.TaskID, "ai.node.review_required", event)
    }
    
    return nil
}
```

#### 审核流程

```
1. CONTROL node → READY
2. StateService → Task PAUSED + "ai.node.review_required" event
3. 前端显示审核界面
4. 用户操作:
   - 通过 → POST /api/node/:nodeId/success
   - 驳回 → POST /api/node/:nodeId/failure {errorMessage: "驳回理由"}
5. StateMachine 正常处理:
   - Success → 检查 Task 状态 → auto-resume
   - Failure → 触发重试 → Task resume
```

### 状态流转图

```
                    ┌──────────┐
                    │ CREATED  │
                    └────┬─────┘
                         │ TryMakeReady
                    ┌────▼─────┐
                    │  READY   │──── Task PAUSED (仅 CONTROL)
                    └────┬─────┘
                         │ 外部 API
                    ┌────▼─────┐
               ┌────│ RUNNING  │────┐
               │    └──────────┘    │
          POST /success        POST /failure
               │                    │
          ┌────▼─────┐        ┌────▼─────┐
          │ SUCCESS  │        │  FAILED  │──→ RETRYING ──→ CREATED
          └────┬─────┘        └──────────┘
               │
          Task RESUME (auto)
```

### 条件依赖兼容

CONTROL 节点可设置 `condition`:
```json
{
  "id": "hr_review_ch3",
  "type": "CONTROL",
  "name": "审核-技术方案",
  "condition": "gen_chapter_3.status == success"
}
```
- 如果 gen_chapter_3 生成失败或跳过，CONTROL 节点直接 SKIPPED
- SKIPPED 的 CONTROL 节点不触发暂停

## 2. 进度事件设计

### 新增 Topic

```go
// internal/eventbus/eventbus.go
const TopicProgress = "ai.node.progress"
```

### 新增 Event 类型

```go
// internal/model/model.go
type ProgressEvent struct {
    ProjectID   string  `json:"project_id"`
    Stage       string  `json:"stage"`       // PARSING, PLANNING, GENERATING, REVIEWING, EXPORTING, COMPLETED
    Progress    float64 `json:"progress"`    // 0.0 ~ 1.0
    CurrentStep string  `json:"current_step"`
    TotalSteps  int     `json:"total_steps"`
    NodeID      string  `json:"node_id,omitempty"`
    Timestamp   string  `json:"timestamp"`
}
```

### 发布方式

业务模块 (bid service) 通过 outbox 发布:
```go
event := model.ProgressEvent{
    ProjectID:  projectID,
    Stage:      "GENERATING",
    Progress:   0.45,
    CurrentStep: "生成第3章/共8章",
    TotalSteps: 8,
    NodeID:     nodeID,
    Timestamp:  time.Now().UTC().Format(time.RFC3339),
}
eventSaver.SaveEvent(ctx, "bid_project", projectID, eventbus.TopicProgress, event)
```

### 消费

前端可选消费 Kafka topic `ai.node.progress` 获取实时进度（如前端不支持 Kafka，可轮询 `GET /api/bid/projects/:id/progress`）。

## 3. bid 模块设计

### 模块结构

```
internal/bid/
  handler/
    handler.go       # Gin HTTP Handler
  service/
    service.go       # BidService: CRUD + 流程编排
    workflow.go      # DAG 构建 + 提交逻辑
  repository/
    repository.go    # BidProject + BidChapter + BidTemplate 数据访问
```

### 核心接口

```go
// BidService
type BidService interface {
    CreateProject(ctx, input) (*BidProject, error)
    GetProject(ctx, projectID) (*BidProject, error)
    ListProjects(ctx, filters) ([]*BidProject, int, error)
    UpdateProject(ctx, projectID, input) (*BidProject, error)
    DeleteProject(ctx, projectID) error
    UploadTender(ctx, projectID, file) (*TenderFile, error)
    StartGeneration(ctx, projectID) (string, error) // returns task_id
    PauseGeneration(ctx, projectID) error
    ResumeGeneration(ctx, projectID) error
    ApproveChapter(ctx, projectID, chapterID) error
    RejectChapter(ctx, projectID, chapterID, comment) error
    RegenerateChapter(ctx, projectID, chapterID) (string, error)
    ExportDocument(ctx, projectID, format) (string, error)
    GetExportStatus(ctx, projectID) (*ExportStatus, error)
    GetProgress(ctx, projectID) (*Progress, error)
    GetTrace(ctx, projectID) (*Trace, error)
}
```

### 依赖注入

```go
type BidService struct {
    bidRepo       BidRepository
    orchService   *orchestrator.OrchestratorService
    taskControl   *orchestrator.TaskExecutionControl
    eventSaver    outbox.EventSaver
    mediaSvc      *media.MediaService
    toolRegistry  *tool.ToolRegistry
}
```

### Workflow 构建 (workflow.go)

```go
func (s *BidService) buildDAG(project *BidProject) *model.DAGRequest {
    chapters := project.Structure // 从 project.structure JSONB 解析
    
    nodes := []model.NodeRequest{}
    edges := []model.Edge{}
    
    // Root: 招标文件解析
    nodes = append(nodes, model.NodeRequest{
        ID: "parse_tender", Type: "TOOL", Name: "doc_parser",
        Input: map[string]interface{}{"file_path": project.TenderFilePath},
    })
    
    // 章节规划
    nodes = append(nodes, model.NodeRequest{
        ID: "plan_structure", Type: "LLM", Name: "llm_api",
        Input: map[string]interface{}{/* prompt with tender analysis */},
    })
    edges = append(edges, model.Edge{From: "parse_tender", To: "plan_structure"})
    
    // 审核-章节规划
    nodes = append(nodes, model.NodeRequest{
        ID: "hr_approve_plan", Type: "CONTROL", Name: "审核-章节规划",
    })
    edges = append(edges, model.Edge{From: "plan_structure", To: "hr_approve_plan"})
    
    // 并发章节生成
    for i, ch := range chapters {
        chID := fmt.Sprintf("gen_chapter_%d", i)
        nodes = append(nodes, model.NodeRequest{
            ID: chID, Type: "TOOL", Name: "chapter_generator",
            Input: map[string]interface{}{"chapter": ch},
        })
        edges = append(edges, model.Edge{From: "hr_approve_plan", To: chID})
        
        // 逐章审核
        reviewID := fmt.Sprintf("hr_review_%d", i)
        nodes = append(nodes, model.NodeRequest{
            ID: reviewID, Type: "CONTROL", Name: fmt.Sprintf("审核-%s", ch.Title),
        })
        edges = append(edges, model.Edge{From: chID, To: reviewID})
    }
    
    // 合规检查 → 终审 → 导出
    // ...
    
    return &model.DAGRequest{Nodes: nodes, Edges: edges}
}
```

## 4. 数据库迁移

```sql
-- aios-core/internal/database/database.go RunMigrations() 中追加

CREATE TABLE IF NOT EXISTS bid_projects (
    id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(64),
    name VARCHAR(255) NOT NULL,
    status VARCHAR(32) DEFAULT 'DRAFT',
    task_id VARCHAR(64) REFERENCES ai_task(id),
    template_id VARCHAR(64),
    industry VARCHAR(128),
    tender_file_path VARCHAR(512),
    tender_file_name VARCHAR(255),
    tender_analysis JSONB,
    structure JSONB,
    config JSONB,
    progress REAL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS bid_chapters (
    id VARCHAR(64) PRIMARY KEY,
    project_id VARCHAR(64) NOT NULL REFERENCES bid_projects(id) ON DELETE CASCADE,
    node_id VARCHAR(64),
    title VARCHAR(255) NOT NULL,
    content TEXT,
    status VARCHAR(32) DEFAULT 'PENDING',
    review_comment TEXT,
    score_items JSONB,
    sort_order INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS bid_templates (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    category VARCHAR(128),
    industry VARCHAR(128),
    structure JSONB NOT NULL,
    workflow_dag JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```
