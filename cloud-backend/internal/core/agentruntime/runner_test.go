package agentruntime

import (
	"context"
	"testing"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestRunnerStart_CreatesTaskScopesDAGAndStoresRun(t *testing.T) {
	store := newMemoryRunStore()
	orch := &fakeOrchestrator{taskID: "task-1"}
	planner := staticPlanner{plan: &AgentPlan{
		Goal:   "make video",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{ID: "script", Tool: "video_script_generator", Arguments: map[string]interface{}{"topic": "AI workflows"}},
		},
	}}
	catalog := staticToolCatalog{
		"video_script_generator": &tool.ToolManifest{
			Name:     "video_script_generator",
			Endpoint: "builtin://video-creation/video_script_generator",
		},
	}

	runner := NewRunner(orch, store, planner, NewPlanGuard(catalog, nil), NewPlanCompiler(catalog))
	run, err := runner.Start(context.Background(), StartRunRequest{
		UserID:  "user-1",
		Message: "make a video about AI workflows",
		Domain:  "video_creation",
	})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if run.TaskID != "task-1" || run.Status != RunStatusRunning {
		t.Fatalf("unexpected run: %#v", run)
	}
	if orch.submittedTaskID != "task-1" {
		t.Fatalf("DAG submitted to wrong task: %q", orch.submittedTaskID)
	}
	expectedNodeID := scopedNodeID("task-1", "script")
	if len(orch.submitted.Nodes) != 1 || orch.submitted.Nodes[0].ID != expectedNodeID {
		t.Fatalf("node IDs were not scoped by task: %#v", orch.submitted.Nodes)
	}
	if _, ok := store.runs[run.ID]; !ok {
		t.Fatalf("run was not stored")
	}
}

func TestRunnerStart_RecordsPlanJudgeWarningsAfterGuardPasses(t *testing.T) {
	store := newMemoryRunStore()
	orch := &fakeOrchestrator{taskID: "task-1"}
	planner := staticPlanner{plan: &AgentPlan{
		Goal:   "make video",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{ID: "script", Tool: "video_script_generator", Arguments: map[string]interface{}{"topic": "AI workflows"}},
		},
	}}
	catalog := staticToolCatalog{
		"video_script_generator": &tool.ToolManifest{
			Name:     "video_script_generator",
			Endpoint: "builtin://video-creation/video_script_generator",
		},
	}
	judge := recordingPlanJudge{
		passed:   true,
		warnings: []PlanJudgeWarning{{Code: "missing_publish_copy", Message: "publish copy missing", Severity: "warning"}},
	}

	runner := NewRunner(orch, store, planner, NewPlanGuard(catalog, nil), NewPlanCompiler(catalog)).
		WithPlanJudge(&judge)
	run, err := runner.Start(context.Background(), StartRunRequest{
		UserID:  "user-1",
		Message: "make a video about AI workflows",
		Domain:  "video_creation",
	})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if !judge.called {
		t.Fatalf("plan judge was not called")
	}
	warnings, ok := run.Metadata["planJudgeWarnings"].([]PlanJudgeWarning)
	if !ok || len(warnings) != 1 || warnings[0].Code != "missing_publish_copy" {
		t.Fatalf("plan judge warnings not stored on run metadata: %#v", run.Metadata)
	}
	taskWarnings, ok := orch.createdInput["planJudgeWarnings"].([]PlanJudgeWarning)
	if !ok || len(taskWarnings) != 1 {
		t.Fatalf("plan judge warnings not included in task input: %#v", orch.createdInput)
	}
	if run.Status != RunStatusRunning {
		t.Fatalf("warnings must not block execution, run status = %s", run.Status)
	}
}

func TestRunnerStart_BlocksWhenPlanJudgeFails(t *testing.T) {
	store := newMemoryRunStore()
	orch := &fakeOrchestrator{taskID: "task-1"}
	planner := staticPlanner{plan: &AgentPlan{
		Goal:   "make video",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{ID: "script", Tool: "video_script_generator", Arguments: map[string]interface{}{"topic": "AI workflows"}},
		},
	}}
	catalog := staticToolCatalog{
		"video_script_generator": &tool.ToolManifest{
			Name:     "video_script_generator",
			Endpoint: "builtin://video-creation/video_script_generator",
		},
	}
	judge := recordingPlanJudge{
		passed: false,
		warnings: []PlanJudgeWarning{{
			Code:     "missing_required_video_stage",
			Message:  "render stage missing",
			Severity: "error",
		}},
	}

	runner := NewRunner(orch, store, planner, NewPlanGuard(catalog, nil), NewPlanCompiler(catalog)).
		WithPlanJudge(&judge)
	_, err := runner.Start(context.Background(), StartRunRequest{
		UserID:  "user-1",
		Message: "make a video about AI workflows",
		Domain:  "video_creation",
	})
	if err == nil {
		t.Fatal("expected failed plan judge to block run start")
	}
	if orch.createdInput != nil || orch.submitted != nil {
		t.Fatalf("runner should not create or submit DAG after plan judge failure: input=%#v submitted=%#v", orch.createdInput, orch.submitted)
	}
	if len(store.runs) != 0 {
		t.Fatalf("failed plan should not be persisted as a run: %#v", store.runs)
	}
}

func TestRunnerStart_RecordsAgentToolTrace(t *testing.T) {
	store := newMemoryRunStore()
	orch := &fakeOrchestrator{taskID: "task-1"}
	planner := staticPlanner{plan: &AgentPlan{
		Goal:   "make current event video",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		ToolTrace: &ToolTrace{
			CandidateTools: []ToolCandidateTrace{
				{Name: "custom_news_search", Score: 0.91, Reason: "matched fresh_knowledge and current-event keywords"},
			},
		},
		KnowledgePolicy: &KnowledgePolicy{
			FreshnessLevel:    FreshnessHigh,
			RetrievalPolicy:   RetrievalRequired,
			SearchQueries:     []string{"佛得角 世界杯 出线 最新"},
			BlockOnEmptyFacts: true,
			MustUseFacts:      true,
		},
		Steps: []AgentStep{
			{
				ID:             "retrieve_fresh_facts",
				Intent:         "获取最新事实",
				Tool:           "custom_news_search",
				Reason:         "用户提到世界杯出线，需要外部事实确认",
				Arguments:      map[string]interface{}{"query": "佛得角 世界杯 出线 最新"},
				ExpectedOutput: []string{"facts", "sources"},
			},
			{
				ID:        "script",
				Tool:      "video_script_generator",
				DependsOn: []string{"retrieve_fresh_facts"},
				Arguments: map[string]interface{}{"topic": "佛得角世界杯出线"},
			},
		},
	}}
	catalog := staticToolCatalog{
		"custom_news_search": &tool.ToolManifest{
			Name:         "custom_news_search",
			Type:         "http",
			Endpoint:     "https://search.example.test/query",
			Capabilities: []string{"fresh_knowledge", "news_search", "web_search"},
			Parameters:   map[string]tool.ParamDef{"query": {Type: "string", Required: true}},
			Output:       map[string]tool.ParamDef{"facts": {Type: "array"}, "sources": {Type: "array"}},
			RiskLevel:    tool.RiskLow,
			CostLevel:    tool.CostLow,
		},
		"video_script_generator": &tool.ToolManifest{
			Name:       "video_script_generator",
			Type:       "builtin_prompt_tool",
			Endpoint:   "builtin://video-creation/video_script_generator",
			Parameters: map[string]tool.ParamDef{"topic": {Type: "string", Required: true}, "knowledgeContext": {Type: "object"}},
		},
	}

	runner := NewRunner(orch, store, planner, NewPlanGuard(catalog, nil), NewPlanCompiler(catalog))
	run, err := runner.Start(context.Background(), StartRunRequest{
		UserID:  "user-1",
		Message: "请帮我做一个30秒视频，讲佛得角国家以及佛得角世界杯出线是一个奇迹。",
		Domain:  "video_creation",
	})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	trace, ok := run.Metadata["agentToolTrace"].(map[string]interface{})
	if !ok {
		t.Fatalf("run metadata should include agentToolTrace: %#v", run.Metadata)
	}
	planned, _ := trace["plannedTools"].([]string)
	if len(planned) != 2 || planned[0] != "custom_news_search" || planned[1] != "video_script_generator" {
		t.Fatalf("trace should record planned tools: %#v", trace)
	}
	guardDecision, _ := trace["guardDecision"].(map[string]interface{})
	if guardDecision["passed"] != true {
		t.Fatalf("trace should record guard decision: %#v", trace)
	}
	knowledgeSummary, _ := trace["knowledgeContext"].(map[string]interface{})
	if knowledgeSummary["generatedBy"] == nil {
		t.Fatalf("trace should summarize planned knowledge context: %#v", trace)
	}
	if orch.createdInput["agentToolTrace"] == nil {
		t.Fatalf("task input should include agentToolTrace: %#v", orch.createdInput)
	}
}

func TestScopeDAGToTask_RewritesNodeReferences(t *testing.T) {
	scoped := scopeDAGToTask("task-1", &model.DAGRequest{
		Nodes: []model.NodeRequest{
			{ID: "script", Input: map[string]interface{}{"parameters": map[string]interface{}{"brief": "x"}}},
			{ID: "shot", Input: map[string]interface{}{
				"parameters": map[string]interface{}{
					"script": "{{script.output.content}}",
					"nested": []interface{}{"{{script.output.summary}}"},
				},
			}},
		},
		Edges: []model.Edge{{From: "script", To: "shot"}},
	})

	params := scoped.Nodes[1].Input["parameters"].(map[string]interface{})
	expectedScriptID := scopedNodeID("task-1", "script")
	if params["script"] != "{{"+expectedScriptID+".output.content}}" {
		t.Fatalf("reference was not scoped: %#v", params["script"])
	}
	nested := params["nested"].([]interface{})
	if nested[0] != "{{"+expectedScriptID+".output.summary}}" {
		t.Fatalf("nested reference was not scoped: %#v", nested)
	}
}

func TestScopeDAGToTask_RewritesReviewSourceNodeIDs(t *testing.T) {
	scoped := scopeDAGToTask("task-1", &model.DAGRequest{
		Nodes: []model.NodeRequest{
			{ID: "script_exec", Input: map[string]interface{}{"parameters": map[string]interface{}{"topic": "AI"}}},
			{ID: "script_review", Input: map[string]interface{}{
				"sourceNode":         "script_exec",
				"qualityCheckerNode": "script_quality_exec",
				"checkerStep":        "script_quality",
				"productionStep":     "script",
			}},
			{ID: "script_quality_exec", Input: map[string]interface{}{}},
		},
		Edges: []model.Edge{{From: "script_exec", To: "script_review"}},
	})

	reviewInput := scoped.Nodes[1].Input
	if reviewInput["sourceNode"] != scopedNodeID("task-1", "script_exec") {
		t.Fatalf("sourceNode should be scoped, got %#v", reviewInput["sourceNode"])
	}
	if reviewInput["qualityCheckerNode"] != scopedNodeID("task-1", "script_quality_exec") {
		t.Fatalf("qualityCheckerNode should be scoped, got %#v", reviewInput["qualityCheckerNode"])
	}
	if reviewInput["checkerStep"] != "script_quality" || reviewInput["productionStep"] != "script" {
		t.Fatalf("step metadata should not be rewritten as node IDs: %#v", reviewInput)
	}
}

func TestScopeDAGToTask_KeepsNodeIDsWithinDatabaseLimit(t *testing.T) {
	longStepID := "script_generation_with_reference_content_review_before_and_extra_suffix"
	scoped := scopeDAGToTask("20260623152045-a3f2", &model.DAGRequest{
		Nodes: []model.NodeRequest{{ID: longStepID, Input: map[string]interface{}{}}},
	})
	if got := len(scoped.Nodes[0].ID); got > 64 {
		t.Fatalf("scoped node id length = %d, want <= 64: %q", got, scoped.Nodes[0].ID)
	}
	if scoped.Nodes[0].Input["agentOriginalNodeId"] != longStepID {
		t.Fatalf("original node id not recorded: %#v", scoped.Nodes[0].Input)
	}
}

type staticPlanner struct {
	plan *AgentPlan
}

func (p staticPlanner) GeneratePlan(context.Context, StartRunRequest) (*AgentPlan, error) {
	return p.plan, nil
}

type fakeOrchestrator struct {
	taskID          string
	createdInput    map[string]interface{}
	submittedTaskID string
	submitted       *model.DAGRequest
}

func (o *fakeOrchestrator) CreateTask(_ context.Context, input map[string]interface{}) (*model.Task, error) {
	o.createdInput = input
	return &model.Task{ID: o.taskID, Status: model.TaskCreated, CreatedAt: time.Now()}, nil
}

func (o *fakeOrchestrator) SubmitDAG(_ context.Context, taskID string, dag *model.DAGRequest) error {
	o.submittedTaskID = taskID
	o.submitted = dag
	return nil
}

func (o *fakeOrchestrator) GetTaskWithDetails(context.Context, string) (map[string]interface{}, error) {
	return map[string]interface{}{"status": string(model.TaskRunning)}, nil
}

type memoryRunStore struct {
	runs map[string]*Run
}

func newMemoryRunStore() *memoryRunStore {
	return &memoryRunStore{runs: make(map[string]*Run)}
}

func (s *memoryRunStore) SaveRun(_ context.Context, run *Run) error {
	s.runs[run.ID] = run
	return nil
}

func (s *memoryRunStore) FindRun(_ context.Context, id string) (*Run, error) {
	return s.runs[id], nil
}

type recordingPlanJudge struct {
	called   bool
	passed   bool
	warnings []PlanJudgeWarning
}

func (j *recordingPlanJudge) Evaluate(*AgentPlan) PlanJudgeReport {
	j.called = true
	return PlanJudgeReport{Passed: j.passed, Warnings: j.warnings}
}
