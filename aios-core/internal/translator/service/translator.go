package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/common/jsonx"
	"github.com/tangying-ai/aios-core/internal/config"
	"github.com/tangying-ai/aios-core/internal/model"
	skillSvc "github.com/tangying-ai/aios-core/internal/skill/service"
)

const systemPromptTmpl = `你是一个任务分解专家。请将用户的自然语言任务分解为多个执行节点（Node），并构建一个有向无环图（DAG）。

每个 Node 的格式：
- id: 节点唯一标识（字符串）
- type: 节点类型（固定为 TOOL）
- name: 工具名称（必须从下方可用工具列表中选择，不得编造）
- input: 输入数据（Map格式，根据工具的 input_schema 填写参数）
- deps: 依赖的节点ID列表（空列表表示无依赖）

要求：
1. 节点之间可以有线性依赖、并行关系
2. 返回纯 JSON 格式，不要有其他文字
3. JSON 结构应该是：{"nodes": [node1, node2, ...], "edges": [edge1, edge2, ...]}
4. edges 格式：{"from": "nodeId1", "to": "nodeId2"}

示例输出：
{
  "nodes": [
    {
      "id": "1",
      "type": "TOOL",
      "name": "llm_api",
      "input": {},
      "deps": []
    },
    {
      "id": "2",
      "type": "TOOL",
      "name": "llm_api",
      "input": {},
      "deps": ["1"]
    }
  ],
  "edges": [
    {"from": "1", "to": "2"}
  ]
}

## 可用工具列表（只能使用以下工具，不得编造工具名）
%s`

type NlToDagService struct {
	cfg             config.OpenAIConfig
	orchestratorURL string
	httpClient      *http.Client
	toolManifestSvc *skillSvc.ToolManifestService
}

func NewNlToDagService(cfg config.OpenAIConfig, orchestratorURL string, toolManifestSvc *skillSvc.ToolManifestService) *NlToDagService {
	return &NlToDagService{
		cfg:             cfg,
		orchestratorURL: orchestratorURL,
		httpClient:      &http.Client{Timeout: time.Duration(cfg.Timeout) * time.Second},
		toolManifestSvc: toolManifestSvc,
	}
}

// TranslateToDag translates natural language to a DAG.
// Routes through the DAG pipeline (Orchestrator -> Worker -> llm_api) for full traceability.
func (s *NlToDagService) TranslateToDag(ctx context.Context, prompt string) (*model.DAGRequest, error) {
	zap.L().Info("Translating natural language to DAG via pipeline", zap.String("prompt", prompt))

	// Query available tools for the LLM prompt
	var toolsDesc string
	if s.toolManifestSvc != nil {
		var err error
		toolsDesc, err = s.toolManifestSvc.FormatForPrompt(ctx)
		if err != nil {
			zap.L().Warn("Failed to query tool manifests for translator, using fallback", zap.Error(err))
		}
	}
	if toolsDesc == "" {
		toolsDesc = "llm_api: 通用大模型调用，可执行任意文本生成任务\nchat_generate: 通用内容生成\nchat_revise: 修改已有内容"
	}

	nodeID := fmt.Sprintf("nl-translate-%d", time.Now().UnixMilli())

	systemPrompt := fmt.Sprintf(systemPromptTmpl, toolsDesc)

	fullPrompt := fmt.Sprintf(`%s

用户需求：%s`, systemPrompt, prompt)

	dagPayload := map[string]interface{}{
		"nodes": []map[string]interface{}{
			{
				"id":   nodeID,
				"type": "TOOL",
				"name": "llm_api",
				"input": map[string]interface{}{
					"prompt": fullPrompt,
				},
			},
		},
		"edges": []map[string]interface{}{},
	}

	dagBody, _ := json.Marshal(dagPayload)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", s.orchestratorURL+"/api/node", bytes.NewReader(dagBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create DAG submit request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to submit translation DAG: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var submitResult map[string]interface{}
	if err := json.Unmarshal(respBody, &submitResult); err != nil {
		return nil, fmt.Errorf("failed to parse DAG submit response: %w", err)
	}

	taskID, _ := submitResult["taskId"].(string)
	if taskID == "" {
		return nil, fmt.Errorf("orchestrator did not return taskId: %s", string(respBody))
	}

	zap.L().Info("NL translation DAG submitted",
		zap.String("taskId", taskID),
		zap.String("nodeId", nodeID))

	// Step 2: Poll for result (max 60s, every 800ms)
	maxAttempts := 75
	pollInterval := 800 * time.Millisecond

	for i := 0; i < maxAttempts; i++ {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled while polling translation result: %w", ctx.Err())
		default:
		}

		httpReq, err := http.NewRequestWithContext(ctx, "GET", s.orchestratorURL+"/api/task/"+taskID, nil)
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}

		httpResp, err := s.httpClient.Do(httpReq)
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}

		body, _ := io.ReadAll(httpResp.Body)
		httpResp.Body.Close()

		var taskResult map[string]interface{}
		if err := json.Unmarshal(body, &taskResult); err != nil {
			time.Sleep(pollInterval)
			continue
		}

		// Find target node
		var nodeStatus string
		var nodeOutput map[string]interface{}

		if nodes, ok := taskResult["nodes"].([]interface{}); ok {
			for _, n := range nodes {
				node, ok := n.(map[string]interface{})
				if !ok {
					continue
				}
				nid, _ := node["id"].(string)
				if nid != nodeID {
					continue
				}
				nodeStatus, _ = node["status"].(string)
				if output, ok := node["output"].(map[string]interface{}); ok {
					nodeOutput = output
				}
				break
			}
		}

		switch nodeStatus {
		case "SUCCESS":
			if nodeOutput == nil {
				return nil, fmt.Errorf("node %s has no output", nodeID)
			}
			stdout, _ := nodeOutput["stdout"].(string)
			if stdout == "" {
				return nil, fmt.Errorf("node %s stdout is empty", nodeID)
			}

			var toolOutput map[string]interface{}
			if err := json.Unmarshal([]byte(stdout), &toolOutput); err != nil {
				return nil, fmt.Errorf("failed to parse tool stdout: %w", err)
			}

			contentStr, _ := toolOutput["content"].(string)
			if contentStr == "" {
				return nil, fmt.Errorf("tool output content is empty")
			}

			var dag model.DAGRequest
			if err := jsonx.ExtractJSON(contentStr, &dag); err != nil {
				return nil, fmt.Errorf("failed to parse LLM response as DAG: %w", err)
			}

			zap.L().Info("Successfully translated to DAG via pipeline",
				zap.String("taskId", taskID),
				zap.Int("nodeCount", len(dag.Nodes)))

			return &dag, nil

		case "FAILED":
			return nil, fmt.Errorf("translation node %s failed", nodeID)

		default:
			time.Sleep(pollInterval)
		}
	}

	return nil, fmt.Errorf("polling timed out for translation task %s", taskID)
}

// TranslateAndSubmit translates natural language to DAG and submits the resulting DAG to the orchestrator.
// The translation itself goes through the DAG pipeline, and the result is submitted as a new task.
func (s *NlToDagService) TranslateAndSubmit(ctx context.Context, prompt string) (map[string]interface{}, error) {
	dag, err := s.TranslateToDag(ctx, prompt)
	if err != nil {
		return nil, err
	}

	body, _ := json.Marshal(dag)
	req, err := http.NewRequestWithContext(ctx, "POST", s.orchestratorURL+"/api/node", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to submit to orchestrator: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse orchestrator response: %w", err)
	}

	zap.L().Info("Successfully submitted translated task to orchestrator")
	return result, nil
}

func (s *NlToDagService) GetTaskStatus(ctx context.Context, taskID string) (map[string]interface{}, error) {
	url := s.orchestratorURL + "/api/task/" + taskID

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch task status: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}
