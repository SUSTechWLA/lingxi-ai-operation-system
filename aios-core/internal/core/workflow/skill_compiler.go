package workflow

import (
	"encoding/json"
	"fmt"
	"strings"

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

	for _, stage := range skill.Stages {
		var currentEntryIDs []string
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
			currentEntryIDs = append(currentEntryIDs, skipID)
			currentOutputIDs = append(currentOutputIDs, skipID)

			// Exec node
			nodes = append(nodes, buildExecutionNode(execID, skill, stage))
			currentEntryIDs = append(currentEntryIDs, execID)

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

			nodes = append(nodes, buildExecutionNode(execID, skill, stage))
			currentEntryIDs = append(currentEntryIDs, execID)
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
			nodes = append(nodes, buildStageNode(nodeID, skill, stage))
			currentEntryIDs = append(currentEntryIDs, nodeID)
			currentOutputIDs = append(currentOutputIDs, nodeID)
		}

		// Connect previous outputs to current inputs
		for _, prevID := range prevOutputIDs {
			for _, curID := range currentEntryIDs {
				edges = append(edges, dagEdge{From: prevID, To: curID})
			}
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
	ID                  string                 `json:"id"`
	Type                string                 `json:"type"`
	Name                string                 `json:"name"`
	Input               map[string]interface{} `json:"input,omitempty"`
	LongRunning         bool                   `json:"longRunning,omitempty"`
	HeartbeatTimeoutSec *int                   `json:"heartbeatTimeoutSec,omitempty"`
}

type dagEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type dagRequest struct {
	Nodes []dagNode `json:"nodes"`
	Edges []dagEdge `json:"edges"`
}

func buildStageNode(nodeID string, skill *skillruntime.SkillManifest, stage skillruntime.StageDefinition) dagNode {
	kind := strings.ToUpper(stage.Kind)
	if kind == "CONTROL" || kind == "APPROVAL" {
		return dagNode{
			ID:   nodeID,
			Type: "CONTROL",
			Name: fmt.Sprintf("审核-%s", stage.Name),
		}
	}
	return buildExecutionNode(nodeID, skill, stage)
}

func buildExecutionNode(nodeID string, skill *skillruntime.SkillManifest, stage skillruntime.StageDefinition) dagNode {
	node := dagNode{
		ID:    nodeID,
		Type:  "TOOL",
		Name:  "external",
		Input: buildStageInput(skill, stage),
	}
	if stage.LongRunning {
		node.LongRunning = true
	}
	if stage.HeartbeatTimeoutSec > 0 {
		timeout := stage.HeartbeatTimeoutSec
		node.HeartbeatTimeoutSec = &timeout
	}
	return node
}

// buildStageInput creates the input map for a skill stage node.
func buildStageInput(skill *skillruntime.SkillManifest, stage skillruntime.StageDefinition) map[string]interface{} {
	toolName := stage.Tool
	if toolName == "" {
		toolName = "skill_stage_agent"
	}

	parameters := map[string]interface{}{
		"tool":            toolName,
		"skill_name":      skill.Name,
		"skill_version":   skill.Version,
		"stage":           stage.Name,
		"stage_kind":      stage.Kind,
		"instruction_ref": stage.Instruction,
	}
	if stage.InputSchema != "" {
		parameters["input_schema"] = stage.InputSchema
	}
	if stage.OutputSchema != "" {
		parameters["output_schema"] = stage.OutputSchema
	}
	for k, v := range stage.Input {
		parameters[k] = v
	}

	input := map[string]interface{}{
		"stage":      stage.Name,
		"parameters": parameters,
	}
	return input
}

// isSkipToExec prevents connecting skip nodes to exec nodes of the same stage.
func isSkipToExec(from, to string) bool {
	// e.g., "visual_design_skip" → "visual_design_exec" should NOT be connected
	return len(from) > 5 && len(to) > 5 &&
		from[len(from)-5:] == "_skip" && to[len(to)-5:] == "_exec"
}

// TemplateIDForSkill returns the stable workflow template ID for a skill version.
func TemplateIDForSkill(name, version string) string {
	return "wf-" + sanitizeTemplateIDPart(name) + "-" + sanitizeTemplateIDPart(version)
}

func sanitizeTemplateIDPart(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
