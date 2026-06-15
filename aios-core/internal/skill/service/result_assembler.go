package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/common/jsonx"
	"github.com/tangying-ai/aios-core/internal/model"
)

// OrchestratorInterface defines the methods we need from the orchestrator.
type OrchestratorInterface interface {
	CreateTask(ctx context.Context, input map[string]interface{}) (*model.Task, error)
	SubmitDAG(ctx context.Context, taskID string, dagReq *model.DAGRequest) error
	GetTaskWithDetails(ctx context.Context, taskID string) (map[string]interface{}, error)
}

// ContextInterface defines the methods we need from the context service.
type ContextInterface interface {
	RecordContext(ctx context.Context, taskID, nodeID string, ctxType model.ContextType, sourceModule, message string, metadata map[string]interface{}) error
}

// ResultAssembler polls task execution and extracts results from node outputs.
type ResultAssembler struct {
	orchestrator OrchestratorInterface
	contextSvc   ContextInterface
}

func NewResultAssembler(orchestrator OrchestratorInterface, contextSvc ContextInterface) *ResultAssembler {
	return &ResultAssembler{
		orchestrator: orchestrator,
		contextSvc:   contextSvc,
	}
}

// CreateTask delegates to the orchestrator to create a new task.
func (a *ResultAssembler) CreateTask(ctx context.Context, input map[string]interface{}) (*model.Task, error) {
	return a.orchestrator.CreateTask(ctx, input)
}

// SubmitDAG delegates to the orchestrator to submit a DAG for a task.
// Node IDs are scoped with the task ID to prevent cross-task collisions.
func (a *ResultAssembler) SubmitDAG(ctx context.Context, taskID string, dagReq *model.DAGRequest) error {
	// Scope node IDs to prevent cross-task collisions
	nodeIDMap := make(map[string]string, len(dagReq.Nodes))
	for i := range dagReq.Nodes {
		originalID := dagReq.Nodes[i].ID
		scopedID := taskID + "-" + originalID
		nodeIDMap[originalID] = scopedID
		dagReq.Nodes[i].ID = scopedID
	}
	for i := range dagReq.Edges {
		if mapped, ok := nodeIDMap[dagReq.Edges[i].From]; ok {
			dagReq.Edges[i].From = mapped
		}
		if mapped, ok := nodeIDMap[dagReq.Edges[i].To]; ok {
			dagReq.Edges[i].To = mapped
		}
	}

	return a.orchestrator.SubmitDAG(ctx, taskID, dagReq)
}

// PollAndExtract polls the orchestrator for task results and extracts fields from node outputs.
func (a *ResultAssembler) PollAndExtract(
	ctx context.Context,
	taskID string,
	nodes []model.NodeRequest,
) (string, *model.ChatFields, error) {
	_, nodeOutputs, err := a.pollTaskResults(ctx, taskID, nodes)
	if err != nil {
		return "", nil, fmt.Errorf("task execution failed: %w", err)
	}

	reply, fields := a.extractFieldsFromOutputs(nodeOutputs)

	// Record context
	if a.contextSvc != nil && (fields.Title != "" || fields.Description != "" || len(fields.Keywords) > 0) {
		a.recordContext(ctx, taskID, fields)
	}



	return reply, fields, nil
}

// pollTaskResults polls the orchestrator for all node results.
func (a *ResultAssembler) pollTaskResults(
	_ context.Context,
	taskID string,
	nodes []model.NodeRequest,
) (map[string]interface{}, map[string]interface{}, error) {
	maxAttempts := 240 // 240 * 500ms = 120s
	pollInterval := 500 * time.Millisecond

	// Use a background-derived context so polling continues even if the
	// HTTP request context is cancelled (e.g. client timeout). This prevents
	// losing results from tasks that complete near the timeout boundary.
	pollCtx, cancel := context.WithTimeout(context.Background(),
		time.Duration(maxAttempts)*pollInterval+30*time.Second)
	defer cancel()

	nodeIDSet := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		nodeIDSet[n.ID] = true
	}

	for i := 0; i < maxAttempts; i++ {
		select {
		case <-pollCtx.Done():
			return nil, nil, fmt.Errorf("task %s polling deadline exceeded", taskID)
		default:
		}

		details, err := a.orchestrator.GetTaskWithDetails(pollCtx, taskID)
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}

		taskStatus, _ := details["status"].(string)
		if taskStatus == "FAILED" {
			var errors []string
			nodeList := extractNodeList(details)
			for _, n := range nodeList {
				nid, _ := n["id"].(string)
				if !nodeIDSet[nid] {
					continue
				}
				status, _ := n["status"].(string)
				if status == "FAILED" {
					errMsg, _ := n["errorMessage"].(string)
					if errMsg == "" {
						errMsg = "unknown error"
					}
					errors = append(errors, nid+": "+errMsg)
				}
			}
			if len(errors) > 0 {
				return nil, nil, fmt.Errorf("task failed: %s", errors[0])
			}
			return nil, nil, fmt.Errorf("task %s failed", taskID)
		}

		nodeList := extractNodeList(details)
		if len(nodeList) == 0 {
			time.Sleep(pollInterval)
			continue
		}

		results := make(map[string]interface{})
		outputs := make(map[string]interface{})
		allComplete := true

		for _, n := range nodeList {
			nid, _ := n["id"].(string)
			if !nodeIDSet[nid] {
				continue
			}

			status, _ := n["status"].(string)
			switch status {
			case "SUCCESS":
				out := extractNodeOutput(n)
				results[nid] = out
				outputs[nid] = out
			case "FAILED":
				errMsg, _ := n["errorMessage"].(string)
				if errMsg == "" {
					errMsg = "unknown error"
				}
				results[nid] = map[string]interface{}{"error": errMsg}
			default:
				allComplete = false
			}
		}

		if allComplete {
			results["_task_id"] = taskID
			return results, outputs, nil
		}

		time.Sleep(pollInterval)
	}

	return nil, nil, fmt.Errorf("task %s timed out after 120s", taskID)
}

// extractFieldsFromOutputs extracts ChatFields from node outputs.
func (a *ResultAssembler) extractFieldsFromOutputs(outputs map[string]interface{}) (string, *model.ChatFields) {
	fields := &model.ChatFields{}
	var replyParts []string

	for _, out := range outputs {
		outMap, ok := out.(map[string]interface{})
		if !ok {
			continue
		}

		// Check for content field (from chat_generate tool)
		if content, ok := outMap["content"].(string); ok && content != "" {
			// Try to parse as JSON first (structured output)
			var parsed map[string]interface{}
			if err := jsonx.ExtractJSON(content, &parsed); err == nil {
				if title, ok := parsed["title"].(string); ok && title != "" {
					fields.Title = title
				}
				if desc, ok := parsed["description"].(string); ok && desc != "" {
					fields.Description = desc
				}
				if body, ok := parsed["body"].(string); ok && body != "" {
					fields.Body = body
				}
				if kw, ok := parsed["keywords"].([]interface{}); ok {
					for _, k := range kw {
						if s, ok := k.(string); ok {
							fields.Keywords = append(fields.Keywords, s)
						}
					}
				}
				// Check nested "fields" object (e.g. {"type": "generate", "fields": {"title": "...", ...}})
				if nestedFields, ok := parsed["fields"].(map[string]interface{}); ok {
					if title, ok := nestedFields["title"].(string); ok && title != "" && fields.Title == "" {
						fields.Title = title
					}
					if desc, ok := nestedFields["description"].(string); ok && desc != "" && fields.Description == "" {
						fields.Description = desc
					}
					if body, ok := nestedFields["body"].(string); ok && body != "" && fields.Body == "" {
						fields.Body = body
					}
					if kw, ok := nestedFields["keywords"].([]interface{}); ok && len(fields.Keywords) == 0 {
						for _, k := range kw {
							if s, ok := k.(string); ok {
								fields.Keywords = append(fields.Keywords, s)
							}
						}
					}
				}
				if reply, ok := parsed["reply"].(string); ok && reply != "" {
					replyParts = append(replyParts, reply)
				}
			} else {
				// Not a JSON object — try array of strings (bare keywords list)
				var arr []interface{}
				if err2 := jsonx.ExtractJSON(content, &arr); err2 == nil && len(arr) > 0 {
					for _, item := range arr {
						if s, ok := item.(string); ok {
							fields.Keywords = append(fields.Keywords, s)
						}
					}
					if len(fields.Keywords) > 0 {
						replyParts = append(replyParts, fmt.Sprintf("已生成 %d 个关键词", len(fields.Keywords)))
					}
				} else {
					// Not JSON at all, use content as plain reply
					replyParts = append(replyParts, content)
				}
			}
		}

		// Also check top-level fields
		if title, ok := outMap["title"].(string); ok && title != "" && fields.Title == "" {
			fields.Title = title
		}
		if desc, ok := outMap["description"].(string); ok && desc != "" && fields.Description == "" {
			fields.Description = desc
		}
		if body, ok := outMap["body"].(string); ok && body != "" && fields.Body == "" {
			fields.Body = body
		}
		// Check top-level keywords (from chat_revise tool)
		if kw, ok := outMap["keywords"].([]interface{}); ok && len(fields.Keywords) == 0 {
			for _, k := range kw {
				if s, ok := k.(string); ok {
					fields.Keywords = append(fields.Keywords, s)
				}
			}
		}
		// Check top-level reply (from chat_revise tool)
		if r, ok := outMap["reply"].(string); ok && r != "" {
			replyParts = append(replyParts, r)
		}

		// Check stdout for parsed JSON
		if stdout, ok := outMap["stdout"].(string); ok && stdout != "" {
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(stdout), &parsed); err == nil {
				if title, ok := parsed["title"].(string); ok && title != "" && fields.Title == "" {
					fields.Title = title
				}
				if desc, ok := parsed["description"].(string); ok && desc != "" && fields.Description == "" {
					fields.Description = desc
				}
				if body, ok := parsed["body"].(string); ok && body != "" && fields.Body == "" {
					fields.Body = body
				}
				if kw, ok := parsed["keywords"].([]interface{}); ok && len(fields.Keywords) == 0 {
					for _, k := range kw {
						if s, ok := k.(string); ok {
							fields.Keywords = append(fields.Keywords, s)
						}
					}
				}
			}
		}
	}

	reply := "内容已生成！"
	if len(replyParts) > 0 {
		reply = strings.Join(replyParts, "\n\n")
	}

	return reply, fields
}

func (a *ResultAssembler) recordContext(ctx context.Context, taskID string, fields *model.ChatFields) {
	var changed []string
	if fields.Title != "" {
		changed = append(changed, fmt.Sprintf("标题=\"%s\"", fields.Title))
	}
	if fields.Description != "" {
		changed = append(changed, fmt.Sprintf("简介=\"%s\"", fields.Description))
	}
	if len(fields.Keywords) > 0 {
		changed = append(changed, fmt.Sprintf("关键词=%v", fields.Keywords))
	}

	message := fmt.Sprintf("AI助手生成了内容: %s", strings.Join(changed, "; "))

	if err := a.contextSvc.RecordContext(ctx, taskID, "", model.ContextType("AI_GENERATE"), "SkillAssistant", message, nil); err != nil {
		zap.L().Warn("Failed to record skill context", zap.Error(err))
	}
}

// --- Helper functions for node extraction ---

func extractNodeList(details map[string]interface{}) []map[string]interface{} {
	nodesRaw, ok := details["nodes"]
	if !ok {
		return nil
	}

	switch v := nodesRaw.(type) {
	case []map[string]interface{}:
		return v
	case []interface{}:
		var result []map[string]interface{}
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				result = append(result, m)
			}
		}
		return result
	case []*model.Node:
		var result []map[string]interface{}
		for _, n := range v {
			if n != nil {
				result = append(result, map[string]interface{}{
					"id":           n.ID,
					"status":       string(n.Status),
					"output":       n.Output,
					"errorMessage": n.ErrorMessage,
				})
			}
		}
		return result
	}
	return nil
}

func extractNodeOutput(node map[string]interface{}) map[string]interface{} {
	output, ok := node["output"].(map[string]interface{})
	if !ok {
		result := make(map[string]interface{})
		if stdout, ok := node["stdout"].(string); ok {
			result["stdout"] = stdout
		}
		return result
	}

	result := make(map[string]interface{})
	for k, v := range output {
		result[k] = v
	}

	stdout, _ := output["stdout"].(string)
	if stdout != "" {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(stdout), &parsed); err == nil {
			for k, v := range parsed {
				result[k] = v
			}
		}
		result["_raw_stdout"] = stdout
	}

	return result
}
