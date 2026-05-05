package model

import (
	"encoding/json"
	"testing"
)

func TestToJSONB(t *testing.T) {
	data := map[string]interface{}{"key": "value", "num": 42}

	result, err := ToJSONB(data)
	if err != nil {
		t.Fatalf("ToJSONB returned error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if parsed["key"] != "value" {
		t.Errorf("Expected key=value, got %v", parsed["key"])
	}
}

func TestToJSONB_NilValue(t *testing.T) {
	result, err := ToJSONB(nil)
	if err != nil {
		t.Fatalf("ToJSONB(nil) returned error: %v", err)
	}

	if string(result) != "null" {
		t.Errorf("Expected 'null' for nil, got %s", string(result))
	}
}

func TestFromJSONB(t *testing.T) {
	input := `{"key":"value","num":42}`
	var parsed map[string]interface{}

	err := FromJSONB(json.RawMessage(input), &parsed)
	if err != nil {
		t.Fatalf("FromJSONB returned error: %v", err)
	}

	if parsed["key"] != "value" {
		t.Errorf("Expected key=value, got %v", parsed["key"])
	}
}

func TestFromJSONB_InvalidJSON(t *testing.T) {
	var parsed map[string]interface{}

	err := FromJSONB(json.RawMessage("invalid"), &parsed)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestTaskStatus_Constants(t *testing.T) {
	statuses := map[TaskStatus]string{
		TaskCreated: "CREATED",
		TaskRunning: "RUNNING",
		TaskPaused:  "PAUSED",
		TaskSuccess: "SUCCESS",
		TaskFailed:  "FAILED",
	}

	for status, expected := range statuses {
		if string(status) != expected {
			t.Errorf("Expected %s, got %s", expected, string(status))
		}
	}
}

func TestNodeStatus_Constants(t *testing.T) {
	statuses := map[NodeStatus]string{
		NodeCreated:  "CREATED",
		NodeReady:    "READY",
		NodeRunning:  "RUNNING",
		NodeRetrying: "RETRYING",
		NodeSuccess:  "SUCCESS",
		NodeFailed:   "FAILED",
		NodeSkipped:  "SKIPPED",
	}

	for status, expected := range statuses {
		if string(status) != expected {
			t.Errorf("Expected %s, got %s", expected, string(status))
		}
	}
}

func TestNodeType_Constants(t *testing.T) {
	types := map[NodeType]string{
		NodeTypeTool:    "TOOL",
		NodeTypeLLM:     "LLM",
		NodeTypeLog:     "LOG",
		NodeTypeControl: "CONTROL",
	}

	for nt, expected := range types {
		if string(nt) != expected {
			t.Errorf("Expected %s, got %s", expected, string(nt))
		}
	}
}

func TestDAGRequest_JSONRoundTrip(t *testing.T) {
	dag := DAGRequest{
		Nodes: []NodeRequest{
			{ID: "1", Type: "LLM", Name: "step1", Input: map[string]interface{}{"prompt": "hello"}},
			{ID: "2", Type: "TOOL", Name: "step2", Condition: "1.status == success"},
		},
		Edges: []Edge{
			{From: "1", To: "2"},
		},
	}

	data, err := json.Marshal(dag)
	if err != nil {
		t.Fatalf("Failed to marshal DAG: %v", err)
	}

	var parsed DAGRequest
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal DAG: %v", err)
	}

	if len(parsed.Nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(parsed.Nodes))
	}
	if len(parsed.Edges) != 1 {
		t.Errorf("Expected 1 edge, got %d", len(parsed.Edges))
	}
	if parsed.Nodes[0].ID != "1" {
		t.Errorf("Expected node ID '1', got %s", parsed.Nodes[0].ID)
	}
	if parsed.Edges[0].From != "1" || parsed.Edges[0].To != "2" {
		t.Errorf("Expected edge 1->2, got %s->%s", parsed.Edges[0].From, parsed.Edges[0].To)
	}
}

func TestNodeResultEvent_JSONRoundTrip(t *testing.T) {
	event := NodeResultEvent{
		TaskID:       "task-1",
		NodeID:       "node-1",
		Status:       NodeSuccess,
		Data:         map[string]interface{}{"result": "ok"},
		TraceID:      "trace-1",
		IdempotencyKey: "task-1-node-1",
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var parsed NodeResultEvent
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if parsed.TaskID != "task-1" || parsed.NodeID != "node-1" {
		t.Errorf("Round-trip mismatch: %+v", parsed)
	}
	if parsed.Status != NodeSuccess {
		t.Errorf("Expected SUCCESS status, got %s", parsed.Status)
	}
}

func TestNodeRequest_MaxRetryPointer(t *testing.T) {
	maxRetry := 5
	priority := 10

	req := NodeRequest{
		ID:       "1",
		Type:     "LLM",
		Name:     "test",
		MaxRetry: &maxRetry,
		Priority: &priority,
	}

	if req.MaxRetry == nil || *req.MaxRetry != 5 {
		t.Error("MaxRetry pointer not preserved")
	}
	if req.Priority == nil || *req.Priority != 10 {
		t.Error("Priority pointer not preserved")
	}
}

func TestNodeTaskEvent_JSONRoundTrip(t *testing.T) {
	event := NodeTaskEvent{
		TaskID:   "task-1",
		NodeID:   "node-1",
		Type:     "LLM",
		Payload:  map[string]interface{}{"prompt": "analyze"},
		TraceID:  "trace-1",
		IdempotencyKey: "task-1-node-1",
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var parsed NodeTaskEvent
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if parsed.TaskID != "task-1" || parsed.Type != "LLM" {
		t.Errorf("Round-trip mismatch: %+v", parsed)
	}
}
