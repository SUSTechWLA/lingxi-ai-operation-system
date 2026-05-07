package service

import (
	"testing"

	"github.com/tangying-ai/tangying-ai-operation-system/internal/model"
)

func TestDAGValidator_SingleNodeNoEdges(t *testing.T) {
	validator := NewDAGValidator()

	dag := &model.DAGRequest{
		Nodes: []model.NodeRequest{
			{ID: "1", Type: "LLM", Name: "step1"},
		},
	}

	if err := validator.Validate(dag); err != nil {
		t.Errorf("Single node with no edges should be valid, got error: %v", err)
	}
}

func TestDAGValidator_MultipleIndependentNodes(t *testing.T) {
	validator := NewDAGValidator()

	dag := &model.DAGRequest{
		Nodes: []model.NodeRequest{
			{ID: "1", Type: "LLM", Name: "step1"},
			{ID: "2", Type: "TOOL", Name: "step2"},
			{ID: "3", Type: "LLM", Name: "step3"},
		},
	}

	if err := validator.Validate(dag); err != nil {
		t.Errorf("Multiple independent nodes should be valid, got error: %v", err)
	}
}

func TestDAGValidator_LinearChain(t *testing.T) {
	validator := NewDAGValidator()

	dag := &model.DAGRequest{
		Nodes: []model.NodeRequest{
			{ID: "1", Type: "LLM", Name: "step1"},
			{ID: "2", Type: "TOOL", Name: "step2"},
			{ID: "3", Type: "LLM", Name: "step3"},
		},
		Edges: []model.Edge{
			{From: "1", To: "2"},
			{From: "2", To: "3"},
		},
	}

	if err := validator.Validate(dag); err != nil {
		t.Errorf("Linear chain should be valid, got error: %v", err)
	}
}

func TestDAGValidator_DiamondDAG(t *testing.T) {
	validator := NewDAGValidator()

	dag := &model.DAGRequest{
		Nodes: []model.NodeRequest{
			{ID: "1", Type: "LLM", Name: "start"},
			{ID: "2", Type: "LLM", Name: "branch_a"},
			{ID: "3", Type: "TOOL", Name: "branch_b"},
			{ID: "4", Type: "LLM", Name: "merge"},
		},
		Edges: []model.Edge{
			{From: "1", To: "2"},
			{From: "1", To: "3"},
			{From: "2", To: "4"},
			{From: "3", To: "4"},
		},
	}

	if err := validator.Validate(dag); err != nil {
		t.Errorf("Diamond DAG should be valid, got error: %v", err)
	}
}

func TestDAGValidator_EmptyNodeID(t *testing.T) {
	validator := NewDAGValidator()

	dag := &model.DAGRequest{
		Nodes: []model.NodeRequest{
			{ID: "", Type: "LLM", Name: "step1"},
		},
	}

	if err := validator.Validate(dag); err == nil {
		t.Error("Expected error for empty node ID")
	}
}

func TestDAGValidator_InvalidEdgeFrom(t *testing.T) {
	validator := NewDAGValidator()

	dag := &model.DAGRequest{
		Nodes: []model.NodeRequest{
			{ID: "1", Type: "LLM", Name: "step1"},
			{ID: "2", Type: "LLM", Name: "step2"},
		},
		Edges: []model.Edge{
			{From: "999", To: "2"},
		},
	}

	if err := validator.Validate(dag); err == nil {
		t.Error("Expected error for invalid edge 'from' reference")
	}
}

func TestDAGValidator_InvalidEdgeTo(t *testing.T) {
	validator := NewDAGValidator()

	dag := &model.DAGRequest{
		Nodes: []model.NodeRequest{
			{ID: "1", Type: "LLM", Name: "step1"},
			{ID: "2", Type: "LLM", Name: "step2"},
		},
		Edges: []model.Edge{
			{From: "1", To: "999"},
		},
	}

	if err := validator.Validate(dag); err == nil {
		t.Error("Expected error for invalid edge 'to' reference")
	}
}

func TestDAGValidator_TwoNodeCycle(t *testing.T) {
	validator := NewDAGValidator()

	dag := &model.DAGRequest{
		Nodes: []model.NodeRequest{
			{ID: "1", Type: "LLM", Name: "step1"},
			{ID: "2", Type: "LLM", Name: "step2"},
		},
		Edges: []model.Edge{
			{From: "1", To: "2"},
			{From: "2", To: "1"},
		},
	}

	if err := validator.Validate(dag); err == nil {
		t.Error("Expected error for 2-node cycle")
	}
}

func TestDAGValidator_SelfLoop(t *testing.T) {
	validator := NewDAGValidator()

	dag := &model.DAGRequest{
		Nodes: []model.NodeRequest{
			{ID: "1", Type: "LLM", Name: "step1"},
		},
		Edges: []model.Edge{
			{From: "1", To: "1"},
		},
	}

	if err := validator.Validate(dag); err == nil {
		t.Error("Expected error for self-loop")
	}
}

func TestDAGValidator_ComplexCycle(t *testing.T) {
	validator := NewDAGValidator()

	dag := &model.DAGRequest{
		Nodes: []model.NodeRequest{
			{ID: "1", Type: "LLM", Name: "step1"},
			{ID: "2", Type: "LLM", Name: "step2"},
			{ID: "3", Type: "TOOL", Name: "step3"},
			{ID: "4", Type: "LLM", Name: "step4"},
			{ID: "5", Type: "LLM", Name: "step5"},
		},
		Edges: []model.Edge{
			{From: "1", To: "2"},
			{From: "2", To: "3"},
			{From: "3", To: "4"},
			{From: "4", To: "2"}, // cycle: 2->3->4->2
		},
	}

	if err := validator.Validate(dag); err == nil {
		t.Error("Expected error for complex cycle")
	}
}

func TestDAGValidator_MultipleDuplicateNodes(t *testing.T) {
	validator := NewDAGValidator()

	dag := &model.DAGRequest{
		Nodes: []model.NodeRequest{
			{ID: "1", Type: "LLM", Name: "step1"},
			{ID: "1", Type: "LLM", Name: "step1_dup"},
			{ID: "2", Type: "LLM", Name: "step2"},
			{ID: "2", Type: "LLM", Name: "step2_dup"},
		},
	}

	if err := validator.Validate(dag); err == nil {
		t.Error("Expected error for multiple duplicate node IDs")
	}
}

func TestDAGValidator_NilDAG(t *testing.T) {
	validator := NewDAGValidator()

	if err := validator.Validate(nil); err == nil {
		t.Error("Expected error for nil DAG")
	}
}
