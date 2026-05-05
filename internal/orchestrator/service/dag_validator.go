package service

import (
	"fmt"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model"
)

type DAGValidator struct{}

func NewDAGValidator() *DAGValidator {
	return &DAGValidator{}
}

func (v *DAGValidator) Validate(dag *model.DAGRequest) error {
	if dag == nil {
		return fmt.Errorf("DAG request is nil")
	}
	if len(dag.Nodes) == 0 {
		return fmt.Errorf("DAG must have at least one node")
	}

	nodeIDs := make(map[string]bool)
	for _, node := range dag.Nodes {
		if node.ID == "" {
			return fmt.Errorf("node ID cannot be empty")
		}
		if nodeIDs[node.ID] {
			return fmt.Errorf("duplicate node ID: %s", node.ID)
		}
		nodeIDs[node.ID] = true
	}

	for _, edge := range dag.Edges {
		if !nodeIDs[edge.From] {
			return fmt.Errorf("edge references unknown node: %s", edge.From)
		}
		if !nodeIDs[edge.To] {
			return fmt.Errorf("edge references unknown node: %s", edge.To)
		}
	}

	if err := v.checkCycle(dag); err != nil {
		return err
	}

	return nil
}

func (v *DAGValidator) checkCycle(dag *model.DAGRequest) error {
	adj := make(map[string][]string)
	for _, edge := range dag.Edges {
		adj[edge.From] = append(adj[edge.From], edge.To)
	}

	visited := make(map[string]int)
	var nodeIDs []string
	for _, n := range dag.Nodes {
		nodeIDs = append(nodeIDs, n.ID)
	}

	var dfs func(nodeID string) error
	dfs = func(nodeID string) error {
		if visited[nodeID] == 1 {
			return fmt.Errorf("cycle detected at node: %s", nodeID)
		}
		if visited[nodeID] == 2 {
			return nil
		}
		visited[nodeID] = 1
		for _, next := range adj[nodeID] {
			if err := dfs(next); err != nil {
				return err
			}
		}
		visited[nodeID] = 2
		return nil
	}

	for _, id := range nodeIDs {
		if visited[id] == 0 {
			if err := dfs(id); err != nil {
				return err
			}
		}
	}

	return nil
}
