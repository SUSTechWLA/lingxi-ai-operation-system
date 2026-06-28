package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/eventbus"
	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/outbox"
)

// ==================== Mock Implementations ====================

type mockNodeRepo struct {
	nodes    map[string]*model.Node
	findByID func(ctx context.Context, id string) (*model.Node, error)
}

func newMockNodeRepo() *mockNodeRepo {
	return &mockNodeRepo{nodes: make(map[string]*model.Node)}
}

func (m *mockNodeRepo) FindByID(ctx context.Context, id string) (*model.Node, error) {
	if m.findByID != nil {
		return m.findByID(ctx, id)
	}
	n, ok := m.nodes[id]
	if !ok {
		return nil, nil
	}
	return n, nil
}

func (m *mockNodeRepo) FindByTaskID(ctx context.Context, taskID string) ([]*model.Node, error) {
	var result []*model.Node
	for _, n := range m.nodes {
		if n.TaskID == taskID {
			result = append(result, n)
		}
	}
	return result, nil
}

func (m *mockNodeRepo) FindByStatus(ctx context.Context, status model.NodeStatus) ([]*model.Node, error) {
	var result []*model.Node
	for _, n := range m.nodes {
		if n.Status == status {
			result = append(result, n)
		}
	}
	return result, nil
}

func (m *mockNodeRepo) FindChildNodes(ctx context.Context, parentID string) ([]*model.Node, error) {
	// This requires dependency info; simplified for testing
	var result []*model.Node
	return result, nil
}

func (m *mockNodeRepo) Save(ctx context.Context, node *model.Node) error {
	m.nodes[node.ID] = node
	return nil
}

func (m *mockNodeRepo) UpdateStatus(ctx context.Context, id string, status model.NodeStatus, output map[string]interface{}, errMsg string) error {
	n, ok := m.nodes[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	n.Status = status
	if output != nil {
		n.Output = output
	}
	if errMsg != "" {
		n.ErrorMessage = errMsg
	}
	return nil
}

func (m *mockNodeRepo) FindStaleRunningNodes(ctx context.Context, timeoutSec int) ([]*model.Node, error) {
	var result []*model.Node
	now := time.Now()
	for _, n := range m.nodes {
		if n.LongRunning && n.Status == model.NodeRunning {
			if n.HeartbeatAt == nil || now.Sub(*n.HeartbeatAt) > time.Duration(timeoutSec)*time.Second {
				result = append(result, n)
			}
		}
	}
	return result, nil
}

func (m *mockNodeRepo) UpdateHeartbeat(ctx context.Context, id string, progress float64, currentStep string) error {
	n, ok := m.nodes[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	now := time.Now()
	n.HeartbeatAt = &now
	if progress >= 0 {
		n.Progress = progress
	}
	if currentStep != "" {
		n.CurrentStep = currentStep
	}
	return nil
}

type mockTaskRepo struct {
	tasks map[string]*model.Task
}

func newMockTaskRepo() *mockTaskRepo {
	return &mockTaskRepo{tasks: make(map[string]*model.Task)}
}

func (m *mockTaskRepo) Save(ctx context.Context, task *model.Task) error {
	m.tasks[task.ID] = task
	return nil
}

func (m *mockTaskRepo) FindByID(ctx context.Context, id string) (*model.Task, error) {
	t, ok := m.tasks[id]
	if !ok {
		return nil, nil
	}
	return t, nil
}

func (m *mockTaskRepo) FindRecent(ctx context.Context) (*model.Task, error) {
	for _, t := range m.tasks {
		return t, nil // return first found (most recent in mock)
	}
	return nil, nil
}

func (m *mockTaskRepo) UpdateStatus(ctx context.Context, id string, status model.TaskStatus) error {
	t, ok := m.tasks[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	t.Status = status
	return nil
}

type mockDepRepo struct {
	deps map[string][]*model.NodeDependency // childID -> deps
}

func newMockDepRepo() *mockDepRepo {
	return &mockDepRepo{deps: make(map[string][]*model.NodeDependency)}
}

func (m *mockDepRepo) Save(ctx context.Context, dep *model.NodeDependency) error {
	m.deps[dep.ChildNodeID] = append(m.deps[dep.ChildNodeID], dep)
	return nil
}

func (m *mockDepRepo) FindByChildID(ctx context.Context, childID string) ([]*model.NodeDependency, error) {
	return m.deps[childID], nil
}

func (m *mockDepRepo) FindByTaskID(ctx context.Context, taskID string) ([]*model.NodeDependency, error) {
	var all []*model.NodeDependency
	for _, deps := range m.deps {
		all = append(all, deps...)
	}
	return all, nil
}

type mockContextRepo struct {
	saved []*model.Context
}

func newMockContextRepo() *mockContextRepo {
	return &mockContextRepo{}
}

func (m *mockContextRepo) Save(ctx context.Context, c *model.Context) error {
	m.saved = append(m.saved, c)
	return nil
}

func (m *mockContextRepo) FindByTaskID(ctx context.Context, taskID string) ([]*model.Context, error) {
	var result []*model.Context
	for _, c := range m.saved {
		if c.TaskID == taskID {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *mockContextRepo) FindLatestSnapshotByNodeID(ctx context.Context, nodeID string) (*model.Context, error) {
	return nil, nil
}

type mockEventSaver struct {
	events []savedEvent
}

type savedEvent struct {
	aggregateType string
	aggregateID   string
	eventType     string
	event         eventbus.Event
}

func newMockEventSaver() *mockEventSaver {
	return &mockEventSaver{}
}

func (m *mockEventSaver) SaveEvent(ctx context.Context, aggregateType, aggregateID, eventType string, event eventbus.Event) error {
	m.events = append(m.events, savedEvent{
		aggregateType: aggregateType,
		aggregateID:   aggregateID,
		eventType:     eventType,
		event:         event,
	})
	return nil
}

// Verify mockEventSaver satisfies the interface
var _ outbox.EventSaver = (*mockEventSaver)(nil)

type mockPublisher struct {
	published []publishedEvent
}

type publishedEvent struct {
	topic string
	key   string
	event eventbus.Event
}

func newMockPublisher() *mockPublisher {
	return &mockPublisher{}
}

func (m *mockPublisher) Publish(topic, key string, event eventbus.Event) error {
	m.published = append(m.published, publishedEvent{topic: topic, key: key, event: event})
	return nil
}

var _ eventbus.EventPublisher = (*mockPublisher)(nil)

// ==================== StateService Tests ====================

func TestStateService_TransitionNode_Success(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	nodeRepo.nodes["node-1"] = &model.Node{
		ID:     "node-1",
		TaskID: "task-1",
		Status: model.NodeCreated,
	}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)

	node, err := ss.TransitionNode(context.Background(), "node-1", model.NodeReady, nil, "")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if node.Status != model.NodeReady {
		t.Errorf("Expected READY, got %s", node.Status)
	}
}

func TestStateService_TransitionNode_NotFound(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)

	_, err := ss.TransitionNode(context.Background(), "nonexistent", model.NodeReady, nil, "")
	if err == nil {
		t.Error("Expected error for nonexistent node")
	}
}

func TestStateService_TransitionNode_WithOutputAndError(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	nodeRepo.nodes["node-1"] = &model.Node{
		ID:     "node-1",
		TaskID: "task-1",
		Status: model.NodeRunning,
	}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)

	output := map[string]interface{}{"result": "done"}
	node, err := ss.TransitionNode(context.Background(), "node-1", model.NodeFailed, output, "timeout")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if node.Output["result"] != "done" {
		t.Error("Expected output to be set")
	}
	if node.ErrorMessage != "timeout" {
		t.Errorf("Expected error message 'timeout', got '%s'", node.ErrorMessage)
	}
}

func TestStateService_TransitionTask_Success(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	taskRepo.tasks["task-1"] = &model.Task{
		ID:     "task-1",
		Status: model.TaskRunning,
	}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)

	err := ss.TransitionTask(context.Background(), "task-1", model.TaskSuccess)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if taskRepo.tasks["task-1"].Status != model.TaskSuccess {
		t.Errorf("Expected SUCCESS, got %s", taskRepo.tasks["task-1"].Status)
	}

	// Should have saved a task completed event
	found := false
	for _, e := range eventSaver.events {
		if e.eventType == eventbus.TopicTaskCompleted {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected task completed event to be saved")
	}
}

func TestStateService_TransitionTask_Failed(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	taskRepo.tasks["task-1"] = &model.Task{
		ID:     "task-1",
		Status: model.TaskRunning,
	}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)

	err := ss.TransitionTask(context.Background(), "task-1", model.TaskFailed)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	found := false
	for _, e := range eventSaver.events {
		if e.eventType == eventbus.TopicTaskFailed {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected task failed event to be saved")
	}
}

func TestStateService_TransitionTask_NotFound(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)

	err := ss.TransitionTask(context.Background(), "nonexistent", model.TaskSuccess)
	if err == nil {
		t.Error("Expected error for nonexistent task")
	}
}

func TestStateService_CheckDependenciesMet_NoDeps(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)

	met, err := ss.CheckDependenciesMet(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !met {
		t.Error("Node with no dependencies should be met")
	}
}

func TestStateService_CheckDependenciesMet_AllParentsSucceeded(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	nodeRepo.nodes["parent-1"] = &model.Node{ID: "parent-1", Status: model.NodeSuccess}
	nodeRepo.nodes["parent-2"] = &model.Node{ID: "parent-2", Status: model.NodeSuccess}
	depRepo.deps["child-1"] = []*model.NodeDependency{
		{ParentNodeID: "parent-1", ChildNodeID: "child-1"},
		{ParentNodeID: "parent-2", ChildNodeID: "child-1"},
	}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)

	met, err := ss.CheckDependenciesMet(context.Background(), "child-1")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !met {
		t.Error("Dependencies should be met when all parents succeeded")
	}
}

func TestStateService_CheckDependenciesMet_ParentSkipped(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	nodeRepo.nodes["parent-1"] = &model.Node{ID: "parent-1", Status: model.NodeSkipped}
	depRepo.deps["child-1"] = []*model.NodeDependency{
		{ParentNodeID: "parent-1", ChildNodeID: "child-1"},
	}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)

	met, _ := ss.CheckDependenciesMet(context.Background(), "child-1")
	if !met {
		t.Error("SKIPPED parent should satisfy dependency")
	}
}

func TestStateService_CheckDependenciesMet_ParentPending(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	nodeRepo.nodes["parent-1"] = &model.Node{ID: "parent-1", Status: model.NodeRunning}
	depRepo.deps["child-1"] = []*model.NodeDependency{
		{ParentNodeID: "parent-1", ChildNodeID: "child-1"},
	}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)

	met, _ := ss.CheckDependenciesMet(context.Background(), "child-1")
	if met {
		t.Error("RUNNING parent should not satisfy dependency")
	}
}

func TestStateService_CheckTaskCompleted_AllDone(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	nodeRepo.nodes["n1"] = &model.Node{ID: "n1", TaskID: "t1", Status: model.NodeSuccess}
	nodeRepo.nodes["n2"] = &model.Node{ID: "n2", TaskID: "t1", Status: model.NodeSkipped}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)

	completed, _ := ss.CheckTaskCompleted(context.Background(), "t1")
	if !completed {
		t.Error("Task with all SUCCESS/SKIPPED nodes should be completed")
	}
}

func TestStateService_CheckTaskCompleted_PendingNodes(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	nodeRepo.nodes["n1"] = &model.Node{ID: "n1", TaskID: "t1", Status: model.NodeSuccess}
	nodeRepo.nodes["n2"] = &model.Node{ID: "n2", TaskID: "t1", Status: model.NodeRunning}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)

	completed, _ := ss.CheckTaskCompleted(context.Background(), "t1")
	if completed {
		t.Error("Task with RUNNING node should not be completed")
	}
}

func TestStateService_HasRetryableNodes_Yes(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	nodeRepo.nodes["n1"] = &model.Node{
		ID: "n1", TaskID: "t1", Status: model.NodeFailed,
		RetryCount: 1, MaxRetry: 3,
	}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)

	hasRetry, _ := ss.HasRetryableNodes(context.Background(), "t1")
	if !hasRetry {
		t.Error("Should have retryable nodes")
	}
}

func TestStateService_HasRetryableNodes_No(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	nodeRepo.nodes["n1"] = &model.Node{
		ID: "n1", TaskID: "t1", Status: model.NodeFailed,
		RetryCount: 3, MaxRetry: 3,
	}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)

	hasRetry, _ := ss.HasRetryableNodes(context.Background(), "t1")
	if hasRetry {
		t.Error("Should not have retryable nodes when retries exhausted")
	}
}

func TestStateService_InitializeNodeReady(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	node := &model.Node{
		ID: "n1", TaskID: "t1", Status: model.NodeCreated,
		Type: model.NodeTypeLLM, Name: "analyze",
		Input: map[string]interface{}{"prompt": "hello"},
	}
	nodeRepo.nodes["n1"] = node

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)

	err := ss.InitializeNodeReady(context.Background(), node)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if nodeRepo.nodes["n1"].Status != model.NodeReady {
		t.Errorf("Expected READY, got %s", nodeRepo.nodes["n1"].Status)
	}
	if nodeRepo.nodes["n1"].IdempotencyKey != "t1-n1" {
		t.Errorf("Expected idempotencyKey 't1-n1', got '%s'", nodeRepo.nodes["n1"].IdempotencyKey)
	}

	// Should have saved a node ready event
	found := false
	for _, e := range eventSaver.events {
		if e.eventType == eventbus.TopicNodeReady {
			found = true
			if e.event.Payload["name"] != "analyze" {
				t.Error("Expected payload to contain node name")
			}
			break
		}
	}
	if !found {
		t.Error("Expected node ready event to be saved")
	}
}

func TestStateService_InitializeControlNodeReady_PausesWithoutDispatch(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	taskRepo.tasks["t1"] = &model.Task{ID: "t1", Status: model.TaskRunning}
	node := &model.Node{
		ID: "review", TaskID: "t1", Status: model.NodeCreated,
		Type: model.NodeTypeControl, Name: "审核-脚本",
	}
	nodeRepo.nodes["review"] = node

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)

	err := ss.InitializeNodeReady(context.Background(), node)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if nodeRepo.nodes["review"].Status != model.NodeReady {
		t.Errorf("Expected control node READY, got %s", nodeRepo.nodes["review"].Status)
	}
	if taskRepo.tasks["t1"].Status != model.TaskPaused {
		t.Errorf("Expected task PAUSED for review, got %s", taskRepo.tasks["t1"].Status)
	}
	if taskRepo.tasks["t1"].PauseReason == "" {
		t.Error("Expected pause reason for review")
	}
	for _, e := range eventSaver.events {
		if e.eventType == eventbus.TopicNodeReady {
			t.Fatalf("CONTROL node should not be dispatched to worker, got event: %+v", e.event)
		}
	}
}

// ==================== StateMachine Tests ====================

func TestStateMachine_OnSuccess(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	nodeRepo.nodes["n1"] = &model.Node{ID: "n1", TaskID: "t1", Status: model.NodeRunning}
	taskRepo.tasks["t1"] = &model.Task{ID: "t1", Status: model.TaskRunning}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)
	sm := NewStateMachine(ss, nodeRepo, taskRepo, eventSaver)

	err := sm.OnSuccess(context.Background(), "n1", map[string]interface{}{"result": "ok"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if nodeRepo.nodes["n1"].Status != model.NodeSuccess {
		t.Errorf("Expected SUCCESS, got %s", nodeRepo.nodes["n1"].Status)
	}
}

func TestStateMachine_OnSuccess_WakesDownstreamAfterManualControlApproval(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	review := &model.Node{ID: "review", TaskID: "t1", Status: model.NodeReady, Type: model.NodeTypeControl, Name: "审核"}
	next := &model.Node{ID: "next", TaskID: "t1", Status: model.NodeCreated, Type: model.NodeTypeLLM, Name: "generate"}
	nodeRepo.nodes["review"] = review
	nodeRepo.nodes["next"] = next
	taskRepo.tasks["t1"] = &model.Task{ID: "t1", Status: model.TaskPaused, PauseReason: "waiting review"}
	depRepo.deps["next"] = []*model.NodeDependency{
		{ParentNodeID: "review", ChildNodeID: "next"},
	}
	nodeRepoWithChildren := &mockNodeRepoWithChildren{
		mockNodeRepo: nodeRepo,
		children: map[string][]*model.Node{
			"review": {next},
		},
	}

	ss := NewStateService(nodeRepoWithChildren, taskRepo, depRepo, ctxRepo, eventSaver)
	dc := NewDependencyChecker(nodeRepoWithChildren, ss, eventSaver)
	ss.SetDependencyChecker(dc)
	sm := NewStateMachine(ss, nodeRepoWithChildren, taskRepo, eventSaver)

	err := sm.OnSuccess(context.Background(), "review", map[string]interface{}{"approved": true})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if nodeRepoWithChildren.nodes["next"].Status != model.NodeReady {
		t.Errorf("Expected downstream node READY after approval, got %s", nodeRepoWithChildren.nodes["next"].Status)
	}
	if taskRepo.tasks["t1"].Status != model.TaskRunning {
		t.Errorf("Expected task RUNNING after approval resumes workflow, got %s", taskRepo.tasks["t1"].Status)
	}
}

func TestStateMachine_OnSuccess_WakesDownstreamWithoutOptionalDependencyChecker(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	review := &model.Node{ID: "composition_review_gate", TaskID: "t1", Status: model.NodeReady, Type: model.NodeTypeReviewGate, Name: "审核-视频结构", Input: map[string]interface{}{"stage": "composition"}}
	next := &model.Node{ID: "preview_exec", TaskID: "t1", Status: model.NodeCreated, Type: model.NodeTypeTool, Name: "external", Input: map[string]interface{}{"tool": "hyperframes_project_generator", "stage": "preview"}}
	nodeRepo.nodes["composition_review_gate"] = review
	nodeRepo.nodes["preview_exec"] = next
	taskRepo.tasks["t1"] = &model.Task{ID: "t1", Status: model.TaskPaused, PauseReason: "waiting composition review"}
	depRepo.deps["preview_exec"] = []*model.NodeDependency{
		{ParentNodeID: "composition_review_gate", ChildNodeID: "preview_exec"},
	}
	nodeRepoWithChildren := &mockNodeRepoWithChildren{
		mockNodeRepo: nodeRepo,
		children: map[string][]*model.Node{
			"composition_review_gate": {next},
		},
	}

	ss := NewStateService(nodeRepoWithChildren, taskRepo, depRepo, ctxRepo, eventSaver)
	sm := NewStateMachine(ss, nodeRepoWithChildren, taskRepo, eventSaver)

	err := sm.OnSuccess(context.Background(), "composition_review_gate", map[string]interface{}{"approved": true})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if nodeRepoWithChildren.nodes["preview_exec"].Status != model.NodeReady {
		t.Fatalf("expected preview_exec READY after composition approval, got %s", nodeRepoWithChildren.nodes["preview_exec"].Status)
	}
	if taskRepo.tasks["t1"].Status != model.TaskRunning {
		t.Fatalf("expected task RUNNING after approval resumes workflow, got %s", taskRepo.tasks["t1"].Status)
	}
	foundReadyEvent := false
	for _, event := range eventSaver.events {
		if event.eventType == eventbus.TopicNodeReady && event.event.NodeID == "preview_exec" {
			foundReadyEvent = true
		}
	}
	if !foundReadyEvent {
		t.Fatalf("expected preview_exec READY event after approval, got %+v", eventSaver.events)
	}
}

func TestStateMachine_OnSuccess_TaskCompletes(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	nodeRepo.nodes["n1"] = &model.Node{ID: "n1", TaskID: "t1", Status: model.NodeRunning}
	taskRepo.tasks["t1"] = &model.Task{ID: "t1", Status: model.TaskRunning}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)
	sm := NewStateMachine(ss, nodeRepo, taskRepo, eventSaver)

	_ = sm.OnSuccess(context.Background(), "n1", nil)

	// Single node completing means task should be completed
	if taskRepo.tasks["t1"].Status != model.TaskSuccess {
		t.Errorf("Expected task SUCCESS, got %s", taskRepo.tasks["t1"].Status)
	}
}

func TestStateMachine_OnFailure_ShouldRetry(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	nodeRepo.nodes["n1"] = &model.Node{
		ID: "n1", TaskID: "t1", Status: model.NodeRunning,
		RetryCount: 0, MaxRetry: 3,
	}
	taskRepo.tasks["t1"] = &model.Task{ID: "t1", Status: model.TaskRunning}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)
	sm := NewStateMachine(ss, nodeRepo, taskRepo, eventSaver)

	err := sm.OnFailure(context.Background(), "n1", "temporary error")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should transition to RETRYING then CREATED
	if nodeRepo.nodes["n1"].RetryCount != 1 {
		t.Errorf("Expected retryCount=1, got %d", nodeRepo.nodes["n1"].RetryCount)
	}
}

func TestStateMachine_OnFailure_MaxRetriesExceeded(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	nodeRepo.nodes["n1"] = &model.Node{
		ID: "n1", TaskID: "t1", Status: model.NodeRunning,
		RetryCount: 3, MaxRetry: 3,
	}
	taskRepo.tasks["t1"] = &model.Task{ID: "t1", Status: model.TaskRunning}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)
	sm := NewStateMachine(ss, nodeRepo, taskRepo, eventSaver)

	err := sm.OnFailure(context.Background(), "n1", "permanent error")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if nodeRepo.nodes["n1"].Status != model.NodeFailed {
		t.Errorf("Expected FAILED, got %s", nodeRepo.nodes["n1"].Status)
	}

	// Task should be paused (then failed since no retryable nodes)
	if taskRepo.tasks["t1"].Status != model.TaskFailed {
		t.Errorf("Expected task FAILED, got %s", taskRepo.tasks["t1"].Status)
	}

	// Should have saved a node failed event
	found := false
	for _, e := range eventSaver.events {
		if e.eventType == eventbus.TopicNodeFailed {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected node failed event")
	}
}

func TestStateMachine_OnFailure_NodeNotFound(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)
	sm := NewStateMachine(ss, nodeRepo, taskRepo, eventSaver)

	err := sm.OnFailure(context.Background(), "nonexistent", "error")
	if err == nil {
		t.Error("Expected error for nonexistent node")
	}
}

// ==================== TaskExecutionControl Tests ====================

func TestTaskExecutionControl_PauseTask(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	taskRepo.tasks["t1"] = &model.Task{ID: "t1", Status: model.TaskRunning}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)
	tc := NewTaskExecutionControl(taskRepo, nodeRepo, ss)

	err := tc.PauseTask(context.Background(), "t1", "manual pause")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if taskRepo.tasks["t1"].Status != model.TaskPaused {
		t.Errorf("Expected PAUSED, got %s", taskRepo.tasks["t1"].Status)
	}
	if taskRepo.tasks["t1"].PauseReason != "manual pause" {
		t.Errorf("Expected pause reason 'manual pause', got '%s'", taskRepo.tasks["t1"].PauseReason)
	}
}

func TestTaskExecutionControl_PauseTask_NotFound(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)
	tc := NewTaskExecutionControl(taskRepo, nodeRepo, ss)

	err := tc.PauseTask(context.Background(), "nonexistent", "reason")
	if err == nil {
		t.Error("Expected error for nonexistent task")
	}
}

func TestTaskExecutionControl_ResumeTask(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	taskRepo.tasks["t1"] = &model.Task{ID: "t1", Status: model.TaskPaused, PauseReason: "manual"}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)
	tc := NewTaskExecutionControl(taskRepo, nodeRepo, ss)

	err := tc.ResumeTask(context.Background(), "t1")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if taskRepo.tasks["t1"].Status != model.TaskRunning {
		t.Errorf("Expected RUNNING, got %s", taskRepo.tasks["t1"].Status)
	}
	if taskRepo.tasks["t1"].PauseReason != "" {
		t.Errorf("Expected empty pause reason, got '%s'", taskRepo.tasks["t1"].PauseReason)
	}
}

func TestTaskExecutionControl_RetryNode(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	nodeRepo.nodes["n1"] = &model.Node{
		ID: "n1", TaskID: "t1", Status: model.NodeFailed,
		RetryCount: 3, MaxRetry: 3, ErrorMessage: "permanent error",
	}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)
	tc := NewTaskExecutionControl(taskRepo, nodeRepo, ss)

	err := tc.RetryNode(context.Background(), "n1")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if nodeRepo.nodes["n1"].Status != model.NodeReady {
		t.Errorf("Expected READY after retry, got %s", nodeRepo.nodes["n1"].Status)
	}
	if nodeRepo.nodes["n1"].RetryCount != 0 {
		t.Errorf("Expected retryCount=0 after manual retry, got %d", nodeRepo.nodes["n1"].RetryCount)
	}
	if nodeRepo.nodes["n1"].ErrorMessage != "" {
		t.Errorf("Expected empty error message after retry, got '%s'", nodeRepo.nodes["n1"].ErrorMessage)
	}
}

func TestTaskExecutionControl_RetryNode_NotFound(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)
	tc := NewTaskExecutionControl(taskRepo, nodeRepo, ss)

	err := tc.RetryNode(context.Background(), "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent node")
	}
}

func TestTaskExecutionControl_GetTaskPauseReason_Paused(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	taskRepo.tasks["t1"] = &model.Task{ID: "t1", Status: model.TaskPaused, PauseReason: "waiting for approval"}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)
	tc := NewTaskExecutionControl(taskRepo, nodeRepo, ss)

	reason := tc.GetTaskPauseReason(context.Background(), "t1")
	if reason != "waiting for approval" {
		t.Errorf("Expected 'waiting for approval', got '%s'", reason)
	}
}

func TestTaskExecutionControl_GetTaskPauseReason_PausedNoReason(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	taskRepo.tasks["t1"] = &model.Task{ID: "t1", Status: model.TaskPaused}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)
	tc := NewTaskExecutionControl(taskRepo, nodeRepo, ss)

	reason := tc.GetTaskPauseReason(context.Background(), "t1")
	if reason != "Task was paused" {
		t.Errorf("Expected 'Task was paused', got '%s'", reason)
	}
}

func TestTaskExecutionControl_GetTaskPauseReason_NotPaused(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	taskRepo.tasks["t1"] = &model.Task{ID: "t1", Status: model.TaskRunning}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)
	tc := NewTaskExecutionControl(taskRepo, nodeRepo, ss)

	reason := tc.GetTaskPauseReason(context.Background(), "t1")
	if reason != "Task is not paused" {
		t.Errorf("Expected 'Task is not paused', got '%s'", reason)
	}
}

func TestTaskExecutionControl_GetTaskPauseReason_NotFound(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)
	tc := NewTaskExecutionControl(taskRepo, nodeRepo, ss)

	reason := tc.GetTaskPauseReason(context.Background(), "nonexistent")
	if reason != "Task not found" {
		t.Errorf("Expected 'Task not found', got '%s'", reason)
	}
}

// ==================== evaluateCondition Tests ====================

func TestEvaluateCondition_StatusEqualsSuccess(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	nodeRepo.nodes["nodeA"] = &model.Node{ID: "nodeA", Status: model.NodeSuccess}

	result := evaluateCondition("nodeA.status == success", "t1", context.Background(), nodeRepo)
	if !result {
		t.Error("Expected condition to be true when nodeA.status == success")
	}
}

func TestEvaluateCondition_StatusEqualsFailed(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	nodeRepo.nodes["nodeA"] = &model.Node{ID: "nodeA", Status: model.NodeFailed}

	result := evaluateCondition("nodeA.status == failed", "t1", context.Background(), nodeRepo)
	if !result {
		t.Error("Expected condition to be true when nodeA.status == failed")
	}
}

func TestEvaluateCondition_StatusNotEqual(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	nodeRepo.nodes["nodeA"] = &model.Node{ID: "nodeA", Status: model.NodeSuccess}

	result := evaluateCondition("nodeA.status == failed", "t1", context.Background(), nodeRepo)
	if result {
		t.Error("Expected condition to be false when nodeA.status is SUCCESS but checking == failed")
	}
}

func TestEvaluateCondition_CaseInsensitive(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	nodeRepo.nodes["nodeA"] = &model.Node{ID: "nodeA", Status: model.NodeSuccess}

	result := evaluateCondition("nodeA.status == SUCCESS", "t1", context.Background(), nodeRepo)
	if !result {
		t.Error("Expected case-insensitive comparison")
	}
}

func TestEvaluateCondition_UnknownNode(t *testing.T) {
	nodeRepo := newMockNodeRepo()

	result := evaluateCondition("unknown.status == success", "t1", context.Background(), nodeRepo)
	if result {
		t.Error("Unknown node should return false")
	}
}

func TestEvaluateCondition_InvalidFormat(t *testing.T) {
	nodeRepo := newMockNodeRepo()

	// No "==" operator - should treat as true
	result := evaluateCondition("invalid condition", "t1", context.Background(), nodeRepo)
	if !result {
		t.Error("Invalid format without == should treat as true")
	}
}

func TestEvaluateCondition_NoDotInLeftSide(t *testing.T) {
	nodeRepo := newMockNodeRepo()

	result := evaluateCondition("nodeA == success", "t1", context.Background(), nodeRepo)
	if !result {
		t.Error("No dot in left side should treat as true")
	}
}

func TestEvaluateCondition_OutputField(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	nodeRepo.nodes["nodeA"] = &model.Node{
		ID:     "nodeA",
		Status: model.NodeSuccess,
		Output: map[string]interface{}{"score": 0.95},
	}

	result := evaluateCondition("nodeA.score == 0.95", "t1", context.Background(), nodeRepo)
	if !result {
		t.Error("Expected output field comparison to be true")
	}
}

// ==================== OrchestratorService Tests ====================

func TestOrchestratorService_CreateTask(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)
	os := NewOrchestratorService(taskRepo, nodeRepo, depRepo, ctxRepo, ss)

	task, err := os.CreateTask(context.Background(), map[string]interface{}{"query": "test"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if task.ID == "" {
		t.Error("Expected non-empty task ID")
	}
	if task.Status != model.TaskCreated {
		t.Errorf("Expected CREATED, got %s", task.Status)
	}
}

func TestOrchestratorService_SubmitDAG(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)
	os := NewOrchestratorService(taskRepo, nodeRepo, depRepo, ctxRepo, ss)

	// Create task first
	task, _ := os.CreateTask(context.Background(), map[string]interface{}{})

	dagReq := &model.DAGRequest{
		Nodes: []model.NodeRequest{
			{ID: "1", Type: "LLM", Name: "step1"},
			{ID: "2", Type: "TOOL", Name: "step2"},
		},
		Edges: []model.Edge{
			{From: "1", To: "2"},
		},
	}

	err := os.SubmitDAG(context.Background(), task.ID, dagReq)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Nodes should be saved
	if nodeRepo.nodes["1"] == nil {
		t.Error("Expected node 1 to be saved")
	}
	if nodeRepo.nodes["2"] == nil {
		t.Error("Expected node 2 to be saved")
	}

	// Dependencies should be saved
	if len(depRepo.deps["2"]) != 1 {
		t.Error("Expected dependency from 1 to 2")
	}

	// Task should be RUNNING
	if taskRepo.tasks[task.ID].Status != model.TaskRunning {
		t.Errorf("Expected RUNNING, got %s", taskRepo.tasks[task.ID].Status)
	}

	// Root node (no incoming edges) should be initialized to READY
	if nodeRepo.nodes["1"].Status != model.NodeReady {
		t.Errorf("Expected root node READY, got %s", nodeRepo.nodes["1"].Status)
	}
}

func TestOrchestratorService_SubmitDAG_InvalidDAG(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)
	os := NewOrchestratorService(taskRepo, nodeRepo, depRepo, ctxRepo, ss)

	task, _ := os.CreateTask(context.Background(), map[string]interface{}{})

	// Empty DAG
	err := os.SubmitDAG(context.Background(), task.ID, &model.DAGRequest{})
	if err == nil {
		t.Error("Expected error for invalid DAG")
	}
}

func TestOrchestratorService_SubmitDAG_TaskNotFound(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)
	os := NewOrchestratorService(taskRepo, nodeRepo, depRepo, ctxRepo, ss)

	dagReq := &model.DAGRequest{
		Nodes: []model.NodeRequest{{ID: "1", Type: "LLM", Name: "step1"}},
	}

	err := os.SubmitDAG(context.Background(), "nonexistent", dagReq)
	if err == nil {
		t.Error("Expected error for nonexistent task")
	}
}

func TestOrchestratorService_GetTaskWithDetails(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)
	os := NewOrchestratorService(taskRepo, nodeRepo, depRepo, ctxRepo, ss)

	task, _ := os.CreateTask(context.Background(), map[string]interface{}{"query": "test"})

	result, err := os.GetTaskWithDetails(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result["taskId"] != task.ID {
		t.Errorf("Expected task ID %s, got %v", task.ID, result["taskId"])
	}
}

func TestOrchestratorService_GetTaskWithDetails_NotFound(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)
	os := NewOrchestratorService(taskRepo, nodeRepo, depRepo, ctxRepo, ss)

	result, err := os.GetTaskWithDetails(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != nil {
		t.Error("Expected nil for nonexistent task")
	}
}

// ==================== DependencyChecker Tests ====================

func TestDependencyChecker_OnNodeExecuted_ChildBecomesReady(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	// Parent succeeded
	nodeRepo.nodes["parent"] = &model.Node{ID: "parent", TaskID: "t1", Status: model.NodeSuccess}
	// Child is CREATED, waiting for parent
	child := &model.Node{
		ID: "child", TaskID: "t1", Status: model.NodeCreated,
		Type: model.NodeTypeLLM, Name: "analyze",
		Input: map[string]interface{}{"prompt": "hello"},
	}
	nodeRepo.nodes["child"] = child

	// Setup dependency: child depends on parent
	depRepo.deps["child"] = []*model.NodeDependency{
		{ParentNodeID: "parent", ChildNodeID: "child"},
	}

	// Mock FindChildNodes to return child for parent
	nodeRepoWithChildren := &mockNodeRepoWithChildren{
		mockNodeRepo: nodeRepo,
		children: map[string][]*model.Node{
			"parent": {child},
		},
	}

	ss := NewStateService(nodeRepoWithChildren, taskRepo, depRepo, ctxRepo, eventSaver)
	dc := NewDependencyChecker(nodeRepoWithChildren, ss, eventSaver)

	dc.OnNodeExecuted(context.Background(), "parent", "t1")

	// Child should now be READY
	if nodeRepoWithChildren.nodes["child"].Status != model.NodeReady {
		t.Errorf("Expected child READY, got %s", nodeRepoWithChildren.nodes["child"].Status)
	}
}

func TestDependencyChecker_OnNodeExecuted_ReviewGatePausesWithoutDispatch(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	taskRepo.tasks["t1"] = &model.Task{ID: "t1", Status: model.TaskRunning}
	nodeRepo.nodes["knowledge_researcher_exec"] = &model.Node{
		ID:     "knowledge_researcher_exec",
		TaskID: "t1",
		Status: model.NodeSuccess,
	}
	review := &model.Node{
		ID:     "knowledge_researcher_review",
		TaskID: "t1",
		Status: model.NodeCreated,
		Type:   model.NodeTypeReviewGate,
		Name:   "审核-knowledge_researcher",
		Input: map[string]interface{}{
			"tool":        "knowledge_researcher",
			"reviewPhase": "after_artifact",
		},
	}
	nodeRepo.nodes["knowledge_researcher_review"] = review
	depRepo.deps["knowledge_researcher_review"] = []*model.NodeDependency{
		{ParentNodeID: "knowledge_researcher_exec", ChildNodeID: "knowledge_researcher_review"},
	}

	nodeRepoWithChildren := &mockNodeRepoWithChildren{
		mockNodeRepo: nodeRepo,
		children: map[string][]*model.Node{
			"knowledge_researcher_exec": {review},
		},
	}
	ss := NewStateService(nodeRepoWithChildren, taskRepo, depRepo, ctxRepo, eventSaver)
	dc := NewDependencyChecker(nodeRepoWithChildren, ss, eventSaver)

	dc.OnNodeExecuted(context.Background(), "knowledge_researcher_exec", "t1")

	if nodeRepoWithChildren.nodes["knowledge_researcher_review"].Status != model.NodeReady {
		t.Errorf("Expected review gate READY, got %s", nodeRepoWithChildren.nodes["knowledge_researcher_review"].Status)
	}
	if taskRepo.tasks["t1"].Status != model.TaskPaused {
		t.Errorf("Expected task PAUSED for review gate, got %s", taskRepo.tasks["t1"].Status)
	}
	for _, e := range eventSaver.events {
		if e.eventType == eventbus.TopicNodeReady && e.event.NodeID == "knowledge_researcher_review" {
			t.Fatalf("REVIEW_GATE should not be dispatched to worker, got event: %+v", e.event)
		}
	}
}

// Extended mock that supports FindChildNodes
type mockNodeRepoWithChildren struct {
	*mockNodeRepo
	children map[string][]*model.Node
}

func (m *mockNodeRepoWithChildren) FindChildNodes(ctx context.Context, parentID string) ([]*model.Node, error) {
	return m.children[parentID], nil
}

func TestDependencyChecker_OnNodeExecuted_ConditionNotMet(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	// Parent succeeded so dependencies are met, but child has condition requiring different status
	nodeRepo.nodes["parent"] = &model.Node{ID: "parent", TaskID: "t1", Status: model.NodeSuccess}
	// Child has condition: parent.status == failed (not met since parent is SUCCESS)
	child := &model.Node{
		ID: "child", TaskID: "t1", Status: model.NodeCreated,
		Condition: "parent.status == failed",
		Type:      model.NodeTypeLLM, Name: "conditional_step",
	}
	nodeRepo.nodes["child"] = child

	depRepo.deps["child"] = []*model.NodeDependency{
		{ParentNodeID: "parent", ChildNodeID: "child"},
	}

	nodeRepoWithChildren := &mockNodeRepoWithChildren{
		mockNodeRepo: nodeRepo,
		children: map[string][]*model.Node{
			"parent": {child},
		},
	}

	ss := NewStateService(nodeRepoWithChildren, taskRepo, depRepo, ctxRepo, eventSaver)
	dc := NewDependencyChecker(nodeRepoWithChildren, ss, eventSaver)

	dc.OnNodeExecuted(context.Background(), "parent", "t1")

	// Child should be SKIPPED because condition not met (parent.status != failed)
	if nodeRepoWithChildren.nodes["child"].Status != model.NodeSkipped {
		t.Errorf("Expected child SKIPPED, got %s", nodeRepoWithChildren.nodes["child"].Status)
	}
}

// ==================== CheckTaskFailed Tests ====================

func TestStateService_CheckTaskFailed_NoFailedNodes(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	nodeRepo.nodes["n1"] = &model.Node{ID: "n1", TaskID: "t1", Status: model.NodeSuccess}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)

	failed, _ := ss.CheckTaskFailed(context.Background(), "t1")
	if failed {
		t.Error("Should not be failed when all nodes succeeded")
	}
}

func TestStateService_CheckTaskFailed_WithFailedNode(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	nodeRepo.nodes["n1"] = &model.Node{
		ID: "n1", TaskID: "t1", Status: model.NodeFailed,
		RetryCount: 3, MaxRetry: 3,
	}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)

	failed, _ := ss.CheckTaskFailed(context.Background(), "t1")
	if !failed {
		t.Error("Should be failed when a node has exhausted retries")
	}
}

// ==================== TryMakeReady Tests ====================

func TestStateService_TryMakeReady_DependenciesMet(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	node := &model.Node{
		ID: "n1", TaskID: "t1", Status: model.NodeCreated,
		Type: model.NodeTypeLLM, Name: "step1",
	}
	nodeRepo.nodes["n1"] = node

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)

	ss.TryMakeReady(context.Background(), node)

	if node.Status != model.NodeReady {
		t.Errorf("Expected READY, got %s", node.Status)
	}
}

func TestStateService_TryMakeReady_NotCreated(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	node := &model.Node{
		ID: "n1", TaskID: "t1", Status: model.NodeRunning,
	}
	nodeRepo.nodes["n1"] = node

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)

	ss.TryMakeReady(context.Background(), node)

	if node.Status != model.NodeRunning {
		t.Errorf("Should not change status of non-CREATED node, got %s", node.Status)
	}
}

func TestStateService_TryMakeReady_DependenciesNotMet(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	nodeRepo.nodes["parent"] = &model.Node{ID: "parent", Status: model.NodeRunning}
	depRepo.deps["child"] = []*model.NodeDependency{
		{ParentNodeID: "parent", ChildNodeID: "child"},
	}

	child := &model.Node{
		ID: "child", TaskID: "t1", Status: model.NodeCreated,
	}
	nodeRepo.nodes["child"] = child

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)

	ss.TryMakeReady(context.Background(), child)

	if child.Status != model.NodeCreated {
		t.Errorf("Should not make ready when dependencies not met, got %s", child.Status)
	}
}

// ==================== Context Recording Tests ====================

func TestStateService_RecordContextForTransition(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	nodeRepo.nodes["n1"] = &model.Node{ID: "n1", TaskID: "t1", Status: model.NodeRunning}

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)

	_, _ = ss.TransitionNode(context.Background(), "n1", model.NodeSuccess, nil, "")

	// Should have recorded a context
	if len(ctxRepo.saved) == 0 {
		t.Error("Expected context to be recorded on transition")
	}

	lastCtx := ctxRepo.saved[len(ctxRepo.saved)-1]
	if lastCtx.ContextType != model.ContextNodeSuccess {
		t.Errorf("Expected NODE_SUCCESS context type, got %s", lastCtx.ContextType)
	}
}

// ==================== Scheduler Integration Test ====================

func TestScheduler_RecoverStaleCreatedNodes(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	// Create a stale node (created more than 1 minute ago)
	staleNode := &model.Node{
		ID: "n1", TaskID: "t1", Status: model.NodeCreated,
		Type: model.NodeTypeLLM, Name: "step1",
		CreatedAt: time.Now().Add(-2 * time.Minute),
	}
	nodeRepo.nodes["n1"] = staleNode

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)
	publisher := newMockPublisher()
	scheduler := NewScheduler(nodeRepo, ss, publisher)

	// Directly test recovery logic
	scheduler.recoverStaleCreatedNodes(context.Background())

	if nodeRepo.nodes["n1"].Status != model.NodeReady {
		t.Errorf("Expected stale node to be READY, got %s", nodeRepo.nodes["n1"].Status)
	}
}

func TestScheduler_RecentNodeNotRecovered(t *testing.T) {
	nodeRepo := newMockNodeRepo()
	taskRepo := newMockTaskRepo()
	depRepo := newMockDepRepo()
	ctxRepo := newMockContextRepo()
	eventSaver := newMockEventSaver()

	// Create a recent node (less than 1 minute ago)
	recentNode := &model.Node{
		ID: "n1", TaskID: "t1", Status: model.NodeCreated,
		CreatedAt: time.Now().Add(-30 * time.Second),
	}
	nodeRepo.nodes["n1"] = recentNode

	ss := NewStateService(nodeRepo, taskRepo, depRepo, ctxRepo, eventSaver)
	publisher := newMockPublisher()
	scheduler := NewScheduler(nodeRepo, ss, publisher)

	scheduler.recoverStaleCreatedNodes(context.Background())

	if nodeRepo.nodes["n1"].Status != model.NodeCreated {
		t.Errorf("Recent node should not be recovered, got %s", nodeRepo.nodes["n1"].Status)
	}
}
