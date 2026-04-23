package service

import (
	"testing"
	"time"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model"
)

func TestDAGValidator_ValidDAG(t *testing.T) {
	validator := NewDAGValidator()

	dag := &model.DAGRequest{
		Nodes: []model.NodeRequest{
			{ID: "1", Type: "LLM", Name: "step1"},
			{ID: "2", Type: "TOOL", Name: "step2"},
		},
		Edges: []model.Edge{
			{From: "1", To: "2"},
		},
	}

	if err := validator.Validate(dag); err != nil {
		t.Errorf("Expected valid DAG, got error: %v", err)
	}
}

func TestDAGValidator_EmptyDAG(t *testing.T) {
	validator := NewDAGValidator()

	dag := &model.DAGRequest{}
	if err := validator.Validate(dag); err == nil {
		t.Error("Expected error for empty DAG")
	}
}

func TestDAGValidator_DuplicateNodes(t *testing.T) {
	validator := NewDAGValidator()

	dag := &model.DAGRequest{
		Nodes: []model.NodeRequest{
			{ID: "1", Type: "LLM", Name: "step1"},
			{ID: "1", Type: "LLM", Name: "step1_dup"},
		},
	}

	if err := validator.Validate(dag); err == nil {
		t.Error("Expected error for duplicate node IDs")
	}
}

func TestDAGValidator_CycleDetection(t *testing.T) {
	validator := NewDAGValidator()

	dag := &model.DAGRequest{
		Nodes: []model.NodeRequest{
			{ID: "1", Type: "LLM", Name: "step1"},
			{ID: "2", Type: "LLM", Name: "step2"},
			{ID: "3", Type: "LLM", Name: "step3"},
		},
		Edges: []model.Edge{
			{From: "1", To: "2"},
			{From: "2", To: "3"},
			{From: "3", To: "1"},
		},
	}

	if err := validator.Validate(dag); err == nil {
		t.Error("Expected error for cycle in DAG")
	}
}

func TestDAGValidator_InvalidEdge(t *testing.T) {
	validator := NewDAGValidator()

	dag := &model.DAGRequest{
		Nodes: []model.NodeRequest{
			{ID: "1", Type: "LLM", Name: "step1"},
		},
		Edges: []model.Edge{
			{From: "1", To: "999"},
		},
	}

	if err := validator.Validate(dag); err == nil {
		t.Error("Expected error for invalid edge reference")
	}
}

func TestRetryPolicy_ShouldRetry(t *testing.T) {
	rp := NewRetryPolicy()

	if !rp.ShouldRetry(0, 3) {
		t.Error("Should retry when retryCount < maxRetry")
	}
	if !rp.ShouldRetry(2, 3) {
		t.Error("Should retry when retryCount < maxRetry")
	}
	if rp.ShouldRetry(3, 3) {
		t.Error("Should not retry when retryCount >= maxRetry")
	}
}

func TestRetryPolicy_GetDelay(t *testing.T) {
	rp := NewRetryPolicy()

	delay0 := rp.GetDelay(0)
	if delay0 != 1*time.Second {
		t.Errorf("Expected 1s initial delay, got %v", delay0)
	}

	delay1 := rp.GetDelay(1)
	if delay1 != 2*time.Second {
		t.Errorf("Expected 2s delay, got %v", delay1)
	}

	delay10 := rp.GetDelay(100)
	if delay10 != 60*time.Second {
		t.Errorf("Expected max 60s delay, got %v", delay10)
	}
}
