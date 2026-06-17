package workflow

import (
	"encoding/json"
	"fmt"

	"github.com/tangying-ai/aios-core/internal/core/skillruntime"
)

// CompileSkillToDAG converts a SkillManifest into a workflow DAG JSON.
// It translates stage definitions into DAG nodes with the following rules:
//   - approval_required=false → single TOOL node
//   - approval_required=true → {name}_exec (TOOL) + {name} (CONTROL)
//   - optional=true → extra {name}_skip (CONTROL) branch that bypasses exec+approval
func CompileSkillToDAG(skill *skillruntime.SkillManifest) (json.RawMessage, error) {
	if skill == nil || len(skill.Stages) == 0 {
		return nil, fmt.Errorf("skill has no stages")
	}

	var nodes []dagNode
	var edges []dagEdge
	var prevOutputIDs []string // IDs that the next stage depends on

	for i, stage := range skill.Stages {
		isLast := i == len(skill.Stages)-1

		var currentOutputIDs []string

		if stage.Optional {
			// Optional stage: create skip branch + exec branch
			skipID := stage.Name + "_skip"
			execID := stage.Name + "_exec"

			// Skip node (bypasses the stage entirely)
			nodes = append(nodes, dagNode{
				ID:   skipID,
				Type: "CONTROL",
				Name: fmt.Sprintf("跳过-%s", stage.Name),
			})
			currentOutputIDs = append(currentOutputIDs, skipID)

			// Exec node
			execNode := dagNode{
				ID:   execID,
				Type: "TOOL",
				Name: "skill_stage",
				Input: buildStageInput(stage),
			}
			nodes = append(nodes, execNode)

			if stage.ApprovalReq {
				// Exec → Approval → next
				approvalID := stage.Name
				nodes = append(nodes, dagNode{
					ID:   approvalID,
					Type: "CONTROL",
					Name: fmt.Sprintf("审核-%s", stage.Name),
				})
				edges = append(edges, dagEdge{From: execID, To: approvalID})
				currentOutputIDs = append(currentOutputIDs, approvalID)
			} else {
				// Exec → next (no approval needed)
				currentOutputIDs = append(currentOutputIDs, execID)
			}
		} else if stage.ApprovalReq {
			// Non-optional with approval: exec → CONTROL
			execID := stage.Name + "_exec"
			approvalID := stage.Name

			nodes = append(nodes, dagNode{
				ID:   execID,
				Type: "TOOL",
				Name: "skill_stage",
				Input: buildStageInput(stage),
			})
			nodes = append(nodes, dagNode{
				ID:   approvalID,
				Type: "CONTROL",
				Name: fmt.Sprintf("审核-%s", stage.Name),
			})
			edges = append(edges, dagEdge{From: execID, To: approvalID})
			currentOutputIDs = append(currentOutputIDs, approvalID)
		} else {
			// Simple stage: single TOOL node
			nodeID := stage.Name
			nodes = append(nodes, dagNode{
				ID:   nodeID,
				Type: "TOOL",
				Name: "skill_stage",
				Input: buildStageInput(stage),
			})
			currentOutputIDs = append(currentOutputIDs, nodeID)
		}

		// Connect previous outputs to current inputs
		for _, prevID := range prevOutputIDs {
			for _, curID := range currentOutputIDs {
				// Don't connect skip nodes to the exec branch (they're parallel alternatives)
				if !isSkipToExec(prevID, curID) {
					edges = append(edges, dagEdge{From: prevID, To: curID})
				}
			}
		}

		// Last stage: no downstream connections needed
		if isLast {
			_ = len(currentOutputIDs) // final stage
		}

		prevOutputIDs = currentOutputIDs
	}

	dag := dagRequest{Nodes: nodes, Edges: edges}
	data, err := json.Marshal(dag)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal DAG: %w", err)
	}
	return json.RawMessage(data), nil
}

// dagNode is the internal DAG node representation used by the compiler.
type dagNode struct {
	ID    string                 `json:"id"`
	Type  string                 `json:"type"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input,omitempty"`
}

type dagEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type dagRequest struct {
	Nodes []dagNode `json:"nodes"`
	Edges []dagEdge `json:"edges"`
}

// buildStageInput creates the input map for a skill stage node.
func buildStageInput(stage skillruntime.StageDefinition) map[string]interface{} {
	input := map[string]interface{}{
		"stage":           stage.Name,
		"instruction_ref": stage.Instruction,
	}
	if stage.InputSchema != "" {
		input["input_schema"] = stage.InputSchema
	}
	if stage.OutputSchema != "" {
		input["output_schema"] = stage.OutputSchema
	}
	return input
}

// isSkipToExec prevents connecting skip nodes to exec nodes of the same stage.
func isSkipToExec(from, to string) bool {
	// e.g., "visual_design_skip" → "visual_design_exec" should NOT be connected
	return len(from) > 5 && len(to) > 5 &&
		from[len(from)-5:] == "_skip" && to[len(to)-5:] == "_exec"
}
