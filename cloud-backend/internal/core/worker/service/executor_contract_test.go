package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/config"
	"github.com/tangying-ai/aios-core/internal/core/eventbus"
	"github.com/tangying-ai/aios-core/internal/core/localrunner"
	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/observability"
	"github.com/tangying-ai/aios-core/internal/core/worker/executor"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

type workerEventSink struct {
	mu     sync.Mutex
	events []observability.Event
}

func (s *workerEventSink) Write(_ context.Context, event observability.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (*workerEventSink) Close(context.Context) error { return nil }

func (s *workerEventSink) snapshot() []observability.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]observability.Event(nil), s.events...)
}

func TestExecuteNodeEmitsPairedToolLifecycleWithoutArguments(t *testing.T) {
	const privateArgument = "PRIVATE_TOOL_ARGUMENT_SENTINEL"
	registry := tool.NewToolRegistry()
	registry.Register(&contractExecutableTool{
		name:   "observed_exec",
		result: tool.SuccessResult(map[string]interface{}{"result": "ok"}),
	})
	publisher := &recordingEventPublisher{}
	sink := &workerEventSink{}
	emitter := observability.NewEmitter(
		observability.Source{Service: "cloud-backend", Component: "worker", Environment: "test"},
		observability.Runtime{},
		sink,
		16,
	)
	nodeExecutor := NewNodeExecutor(registry, publisher, config.WorkerConfig{}, nil, nil, nil).
		WithObservability(emitter)
	ctx := observability.WithCorrelation(context.Background(), observability.Correlation{
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:  "00f067aa0ba902b7",
	})
	nodeExecutor.ExecuteNode(ctx, eventbus.Event{
		TaskID: "task-private", NodeID: "node-private", Type: string(model.NodeTypeTool),
		Payload: map[string]interface{}{
			"tool": "observed_exec", "parameters": map[string]interface{}{"query": privateArgument},
		},
	})
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	events := sink.snapshot()
	var started, completed int
	for _, event := range events {
		if err := event.Validate(); err != nil {
			t.Fatalf("invalid event %s: %v", event.EventType, err)
		}
		switch event.EventType {
		case observability.EventTypeToolCallStarted:
			started++
		case observability.EventTypeToolCallCompleted:
			completed++
		}
	}
	if started != 1 || completed != 1 {
		t.Fatalf("tool event pairing started=%d completed=%d events=%+v", started, completed, events)
	}
	wire, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), privateArgument) {
		t.Fatalf("private tool arguments leaked into events: %s", wire)
	}
}

func TestProgressCheckpointAndHeartbeatPublishExecutionCorrelation(t *testing.T) {
	publisher := &recordingEventPublisher{}
	nodeExecutor := NewNodeExecutor(tool.NewToolRegistry(), publisher, config.WorkerConfig{}, nil, nil, nil)
	correlation := observability.Correlation{
		TraceID:      "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:       "00f067aa0ba902b7",
		ParentSpanID: "b7ad6b7169203331",
	}
	ctx := observability.WithCorrelation(context.Background(), correlation)

	nodeExecutor.publishProgress(ctx, "task", "node", 0.5, "half")
	nodeExecutor.publishCheckpoint(ctx, "task", "node", 0.5, "half", map[string]interface{}{"frame": 10})
	nodeExecutor.publishHeartbeat(ctx, "task", "node", "idem")

	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	if len(publisher.events) != 3 {
		t.Fatalf("published events = %d, want 3", len(publisher.events))
	}
	for i, event := range publisher.events {
		if event.TraceID != correlation.TraceID || event.SpanID != correlation.SpanID || event.ParentSpanID != correlation.ParentSpanID {
			t.Errorf("event %d correlation = %#v", i, event)
		}
	}
}

func TestExecuteNodeBlocksUnresolvedExactReferenceBeforeToolExecution(t *testing.T) {
	registry := tool.NewToolRegistry()
	executable := &contractExecutableTool{name: "contract_exec", result: tool.SuccessResult(map[string]interface{}{"result": "ok"})}
	registry.Register(executable)
	publisher := &recordingEventPublisher{}
	nodeExecutor := NewNodeExecutor(registry, publisher, config.WorkerConfig{}, executor.NewDirectExecutor(), nil, newFakeNodeRepo())
	sink, emitter := observeWorkerExecutor(nodeExecutor)

	nodeExecutor.ExecuteNode(context.Background(), eventbus.Event{
		TaskID: "task-1", NodeID: "consume", Type: string(model.NodeTypeTool),
		Payload: map[string]interface{}{
			"tool":              "contract_exec",
			"parameters":        map[string]interface{}{"query": "{{missing.output.value}}"},
			"contractArguments": map[string]interface{}{"query": "{{missing.output.value}}"},
		},
	})

	if executable.calls != 0 {
		t.Fatalf("tool executed despite unresolved reference: %d calls", executable.calls)
	}
	failure := publisher.lastStatus(model.NodeFailed)
	if !strings.Contains(failure.ErrorMessage, "INPUT_REFERENCE_UNRESOLVED") {
		t.Fatalf("failure = %#v, want INPUT_REFERENCE_UNRESOLVED", failure)
	}
	assertWorkerLifecycle(t, emitter, sink, observability.EventTypeToolCallFailed, "TOOL.EXECUTION.FAILED", 1)
}

func TestExecuteNodeValidatesPureInputAgainstDelegatedManifestBeforeLocalDispatch(t *testing.T) {
	registry := tool.NewToolRegistry()
	registry.RegisterExternal(&tool.ToolManifest{
		Name: "logical_local", ExecutionPlane: tool.ExecutionPlaneLocal, LocalCommand: "LOCAL_MCP_TOOL_CALL",
		InputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"query": map[string]interface{}{"type": "string"}},
			"required": []interface{}{"query"}, "additionalProperties": false,
		},
	})
	dispatcher := &fakeLocalJobDispatcher{job: &localrunner.LocalJob{ID: "local-contract-job"}}
	publisher := &recordingEventPublisher{}
	nodeExecutor := NewNodeExecutor(registry, publisher, config.WorkerConfig{}, nil, nil, newFakeNodeRepo())
	nodeExecutor.SetLocalJobDispatcher(dispatcher)
	sink, emitter := observeWorkerExecutor(nodeExecutor)

	nodeExecutor.ExecuteNode(context.Background(), eventbus.Event{
		TaskID: "task-1", NodeID: "local", Type: string(model.NodeTypeTool),
		Payload: map[string]interface{}{
			"tool":              "external",
			"parameters":        map[string]interface{}{"tool": "logical_local", "query": float64(7), "intent": "routing-only"},
			"contractArguments": map[string]interface{}{"query": float64(7)},
		},
	})

	if dispatcher.req.NodeID != "" {
		t.Fatalf("invalid input dispatched locally: %#v", dispatcher.req)
	}
	failure := publisher.lastStatus(model.NodeFailed)
	if !strings.Contains(failure.ErrorMessage, "INPUT_SCHEMA_INVALID") {
		t.Fatalf("failure = %#v, want INPUT_SCHEMA_INVALID", failure)
	}
	assertWorkerLifecycle(t, emitter, sink, observability.EventTypeToolCallFailed, "TOOL.ARGUMENT.SCHEMA_INVALID", 1)
}

func TestExecuteNodeValidatesExecutableToolResultBeforePublishingSuccess(t *testing.T) {
	registry := tool.NewToolRegistry()
	executable := &contractExecutableTool{
		name:   "canonical_exec",
		result: tool.SuccessResult(map[string]interface{}{"unexpected": true}),
		manifest: tool.ToolManifest{
			Name: "canonical_exec",
			InputSchema: map[string]interface{}{
				"type": "object", "properties": map[string]interface{}{"query": map[string]interface{}{"type": "string"}},
				"required": []interface{}{"query"}, "additionalProperties": false,
			},
			OutputSchema: map[string]interface{}{
				"type": "object", "properties": map[string]interface{}{"result": map[string]interface{}{"type": "string"}},
				"required": []interface{}{"result"}, "additionalProperties": false,
			},
		},
	}
	registry.Register(executable)
	publisher := &recordingEventPublisher{}
	nodeExecutor := NewNodeExecutor(registry, publisher, config.WorkerConfig{}, nil, nil, newFakeNodeRepo())
	sink, emitter := observeWorkerExecutor(nodeExecutor)

	nodeExecutor.ExecuteNode(context.Background(), eventbus.Event{
		TaskID: "task-1", NodeID: "exec", Type: string(model.NodeTypeTool),
		Payload: map[string]interface{}{
			"tool": "canonical_exec", "parameters": map[string]interface{}{"query": "hello"},
			"contractArguments": map[string]interface{}{"query": "hello"},
		},
	})

	if executable.calls != 1 {
		t.Fatalf("calls = %d, want 1", executable.calls)
	}
	failure := publisher.lastStatus(model.NodeFailed)
	if !strings.Contains(failure.ErrorMessage, "OUTPUT_SCHEMA_INVALID") {
		t.Fatalf("failure = %#v, want OUTPUT_SCHEMA_INVALID", failure)
	}
	if publisher.hasStatus(model.NodeSuccess) {
		t.Fatal("invalid executable output published success")
	}
	assertWorkerLifecycle(t, emitter, sink, observability.EventTypeToolCallFailed, "TOOL.EXECUTION.FAILED", 1)
}

func TestExecuteNodePanicEmitsExactlyOneStableFailure(t *testing.T) {
	registry := tool.NewToolRegistry()
	registry.Register(panicWorkerTool{})
	nodeExecutor := NewNodeExecutor(registry, &recordingEventPublisher{}, config.WorkerConfig{}, nil, nil, newFakeNodeRepo())
	sink, emitter := observeWorkerExecutor(nodeExecutor)
	nodeExecutor.ExecuteNode(context.Background(), eventbus.Event{
		TaskID: "task-panic", NodeID: "node-panic", Type: string(model.NodeTypeTool),
		Payload: map[string]interface{}{"tool": "panic_tool", "parameters": map[string]interface{}{}},
	})
	assertWorkerLifecycle(t, emitter, sink, observability.EventTypeToolCallFailed, "TOOL.EXECUTION.FAILED", 1)
}

func TestExecuteNodeLocalDispatchFailureEmitsExactlyOneStableFailure(t *testing.T) {
	registry := tool.NewToolRegistry()
	registry.RegisterExternal(&tool.ToolManifest{Name: "local_failure", ExecutionPlane: tool.ExecutionPlaneLocal, LocalCommand: "LOCAL_MCP_TOOL_CALL"})
	nodeExecutor := NewNodeExecutor(registry, &recordingEventPublisher{}, config.WorkerConfig{}, nil, nil, newFakeNodeRepo())
	nodeExecutor.SetLocalJobDispatcher(&fakeLocalJobDispatcher{err: errors.New("runner unavailable")})
	sink, emitter := observeWorkerExecutor(nodeExecutor)
	nodeExecutor.ExecuteNode(context.Background(), eventbus.Event{
		TaskID: "task-local", NodeID: "node-local", Type: string(model.NodeTypeTool),
		Payload: map[string]interface{}{"tool": "local_failure", "parameters": map[string]interface{}{}},
	})
	assertWorkerLifecycle(t, emitter, sink, observability.EventTypeToolCallFailed, "TOOL.EXECUTION.FAILED", 1)
}

func observeWorkerExecutor(nodeExecutor *NodeExecutor) (*workerEventSink, *observability.Emitter) {
	sink := &workerEventSink{}
	emitter := observability.NewEmitter(observability.Source{Service: "cloud", Component: "worker", Environment: "test"}, observability.Runtime{}, sink, 32)
	nodeExecutor.WithObservability(emitter)
	return sink, emitter
}

func assertWorkerLifecycle(t *testing.T, emitter *observability.Emitter, sink *workerEventSink, terminal observability.EventType, code string, attempt int64) {
	t.Helper()
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	var started, ended int
	for _, event := range sink.snapshot() {
		if err := event.Validate(); err != nil {
			t.Fatalf("invalid worker event %s: %v", event.EventType, err)
		}
		if event.EventType == observability.EventTypeToolCallStarted {
			started++
		}
		if event.EventType == terminal {
			ended++
			if event.Execution.Attempt != attempt {
				t.Errorf("attempt=%d, want %d", event.Execution.Attempt, attempt)
			}
			if code != "" && (event.Error == nil || event.Error.Code != code) {
				t.Errorf("terminal error=%+v, want code %s", event.Error, code)
			}
		}
	}
	if started != 1 || ended != 1 {
		t.Fatalf("tool lifecycle started=%d ended=%d events=%+v", started, ended, sink.snapshot())
	}
}

type panicWorkerTool struct{}

func (panicWorkerTool) Name() string                                   { return "panic_tool" }
func (panicWorkerTool) Description() string                            { return "panic test" }
func (panicWorkerTool) Type() tool.ToolType                            { return tool.ToolTypeCustom }
func (panicWorkerTool) ValidateParameters(map[string]interface{}) bool { return true }
func (panicWorkerTool) Manifest() tool.ToolManifest                    { return tool.ToolManifest{Name: "panic_tool"} }
func (panicWorkerTool) Execute(context.Context, map[string]interface{}, tool.ToolContext) tool.ToolResult {
	panic("worker tool panic")
}

func TestExecuteNodeRequiresJSONBuildableStdoutWhenOutputSchemaDeclared(t *testing.T) {
	registry := tool.NewToolRegistry()
	registry.Register(&contractBuildableTool{name: "canonical_build", stdout: "not-json"})
	publisher := &recordingEventPublisher{}
	nodeExecutor := NewNodeExecutor(registry, publisher, config.WorkerConfig{ToolTimeoutSeconds: 5}, executor.NewDirectExecutor(), nil, newFakeNodeRepo())

	nodeExecutor.ExecuteNode(context.Background(), eventbus.Event{
		TaskID: "task-1", NodeID: "build", Type: string(model.NodeTypeTool),
		Payload: map[string]interface{}{"tool": "canonical_build", "parameters": map[string]interface{}{}, "contractArguments": map[string]interface{}{}},
	})
	failure := publisher.lastStatus(model.NodeFailed)
	if !strings.Contains(failure.ErrorMessage, "OUTPUT_SCHEMA_INVALID") {
		t.Fatalf("failure = %#v, want OUTPUT_SCHEMA_INVALID", failure)
	}
}

func TestExecuteNodeValidatesDelegatedMCPStructuredContent(t *testing.T) {
	registry := tool.NewToolRegistry()
	bridge := &contractExecutableTool{
		name: "external",
		result: tool.SuccessResult(map[string]interface{}{
			"content":           []interface{}{map[string]interface{}{"type": "text", "text": "ok"}},
			"structuredContent": map[string]interface{}{"unexpected": true},
			"isError":           false,
		}),
	}
	registry.Register(bridge)
	registry.RegisterExternal(&tool.ToolManifest{
		Name: "mcp_asset", Boundary: tool.BoundaryMCPProvider,
		InputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"prompt": map[string]interface{}{"type": "string"}},
			"required": []interface{}{"prompt"}, "additionalProperties": false,
		},
		OutputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"assetId": map[string]interface{}{"type": "string"}},
			"required": []interface{}{"assetId"}, "additionalProperties": false,
		},
	})
	publisher := &recordingEventPublisher{}
	nodeExecutor := NewNodeExecutor(registry, publisher, config.WorkerConfig{}, nil, nil, newFakeNodeRepo())

	nodeExecutor.ExecuteNode(context.Background(), eventbus.Event{
		TaskID: "task-1", NodeID: "mcp", Type: string(model.NodeTypeTool),
		Payload: map[string]interface{}{
			"tool": "external", "parameters": map[string]interface{}{"tool": "mcp_asset", "prompt": "hello"},
			"contractArguments": map[string]interface{}{"prompt": "hello"},
		},
	})

	failure := publisher.lastStatus(model.NodeFailed)
	if bridge.calls != 1 || !strings.Contains(failure.ErrorMessage, "OUTPUT_SCHEMA_INVALID") {
		t.Fatalf("delegated MCP structuredContent was not enforced: calls=%d failure=%#v", bridge.calls, failure)
	}
}

func TestExecuteNodeFailsDelegatedMCPIsErrorWithoutOutputSchema(t *testing.T) {
	registry := tool.NewToolRegistry()
	bridge := &contractExecutableTool{
		name: "external",
		result: tool.SuccessResult(map[string]interface{}{
			"content": []interface{}{map[string]interface{}{"type": "text", "text": "remote renderer failed"}},
			"isError": true,
		}),
	}
	registry.Register(bridge)
	registry.RegisterExternal(&tool.ToolManifest{
		Name: "mcp_error", Boundary: tool.BoundaryMCPProvider,
		InputSchema: map[string]interface{}{"type": "object"},
	})
	publisher := &recordingEventPublisher{}
	nodeExecutor := NewNodeExecutor(registry, publisher, config.WorkerConfig{}, nil, nil, newFakeNodeRepo())

	nodeExecutor.ExecuteNode(context.Background(), eventbus.Event{
		TaskID: "task-1", NodeID: "mcp-error", Type: string(model.NodeTypeTool),
		Payload: map[string]interface{}{
			"tool": "external", "parameters": map[string]interface{}{"tool": "mcp_error"},
			"contractArguments": map[string]interface{}{},
		},
	})

	failure := publisher.lastStatus(model.NodeFailed)
	if bridge.calls != 1 || !strings.Contains(failure.ErrorMessage, "MCP_TOOL_ERROR") || !strings.Contains(failure.ErrorMessage, "remote renderer failed") {
		t.Fatalf("delegated MCP error result was not preserved: calls=%d failure=%#v", bridge.calls, failure)
	}
	if publisher.hasStatus(model.NodeSuccess) {
		t.Fatal("MCP isError result published success")
	}
}

func TestExecuteNodeResolvesReferencesInsideTypedContainers(t *testing.T) {
	registry := tool.NewToolRegistry()
	executable := &contractExecutableTool{
		name: "typed_exec", result: tool.SuccessResult(map[string]interface{}{"ok": true}),
		manifest: tool.ToolManifest{Name: "typed_exec", InputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"items": map[string]interface{}{
				"type": "array", "items": map[string]interface{}{
					"type": "object", "properties": map[string]interface{}{"id": map[string]interface{}{"type": "string"}},
					"required": []interface{}{"id"}, "additionalProperties": false,
				},
			}}, "required": []interface{}{"items"}, "additionalProperties": false,
		}},
	}
	registry.Register(executable)
	publisher := &recordingEventPublisher{}
	nodeRepo := newFakeNodeRepo(&model.Node{ID: "produce", TaskID: "task-1", Status: model.NodeSuccess, Output: map[string]interface{}{"assetId": "asset-1"}})
	nodeExecutor := NewNodeExecutor(registry, publisher, config.WorkerConfig{}, nil, nil, nodeRepo)
	typedArguments := map[string]interface{}{"items": []map[string]interface{}{{"id": "{{produce.output.assetId}}"}}}

	nodeExecutor.ExecuteNode(context.Background(), eventbus.Event{
		TaskID: "task-1", NodeID: "typed", Type: string(model.NodeTypeTool),
		Payload: map[string]interface{}{"tool": "typed_exec", "parameters": typedArguments, "contractArguments": typedArguments},
	})

	if executable.calls != 1 || publisher.hasStatus(model.NodeFailed) {
		t.Fatalf("typed container reference did not resolve: calls=%d failure=%#v", executable.calls, publisher.lastStatus(model.NodeFailed))
	}
}

type contractExecutableTool struct {
	name     string
	manifest tool.ToolManifest
	result   tool.ToolResult
	calls    int
}

func (t *contractExecutableTool) Name() string                                   { return t.name }
func (t *contractExecutableTool) Description() string                            { return "contract test" }
func (t *contractExecutableTool) Type() tool.ToolType                            { return tool.ToolTypeCustom }
func (t *contractExecutableTool) ValidateParameters(map[string]interface{}) bool { return true }
func (t *contractExecutableTool) Manifest() tool.ToolManifest {
	manifest := t.manifest
	if manifest.Name == "" {
		manifest.Name = t.name
	}
	return manifest
}
func (t *contractExecutableTool) Execute(context.Context, map[string]interface{}, tool.ToolContext) tool.ToolResult {
	t.calls++
	return t.result
}

type contractBuildableTool struct {
	name   string
	stdout string
}

func (t *contractBuildableTool) Name() string                                   { return t.name }
func (t *contractBuildableTool) Description() string                            { return "contract buildable test" }
func (t *contractBuildableTool) Type() tool.ToolType                            { return tool.ToolTypeCode }
func (t *contractBuildableTool) ValidateParameters(map[string]interface{}) bool { return true }
func (t *contractBuildableTool) Execute(context.Context, map[string]interface{}, tool.ToolContext) tool.ToolResult {
	return tool.FailureResult("BuildExecutionRequest must be used")
}
func (t *contractBuildableTool) Manifest() tool.ToolManifest {
	return tool.ToolManifest{Name: t.name, OutputSchema: map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{"result": map[string]interface{}{"type": "string"}},
		"required": []interface{}{"result"}, "additionalProperties": false,
	}}
}
func (t *contractBuildableTool) BuildExecutionRequest(map[string]interface{}) (*executor.ExecutionRequest, error) {
	return &executor.ExecutionRequest{Command: "sh", Args: []string{"-c", "printf '%s' \"$CONTRACT_STDOUT\""}, Env: map[string]string{"CONTRACT_STDOUT": t.stdout}, TimeoutSec: 5}, nil
}

type recordingEventPublisher struct {
	mu     sync.Mutex
	events []eventbus.Event
}

func (p *recordingEventPublisher) Publish(_ string, _ string, event eventbus.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event)
	return nil
}

func (p *recordingEventPublisher) lastStatus(status model.NodeStatus) eventbus.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	for index := len(p.events) - 1; index >= 0; index-- {
		if p.events[index].Status == string(status) {
			return p.events[index]
		}
	}
	return eventbus.Event{}
}

func (p *recordingEventPublisher) hasStatus(status model.NodeStatus) bool {
	return p.lastStatus(status).Status != ""
}
