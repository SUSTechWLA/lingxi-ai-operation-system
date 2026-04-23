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

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/config"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model"
)

const systemPrompt = `你是一个任务分解专家。请将用户的自然语言任务分解为多个执行节点（Node），并构建一个有向无环图（DAG）。

每个 Node 的格式：
- id: 节点唯一标识（字符串）
- type: 节点类型（LLM 或 TOOL）
- name: 节点名称（如 write_article, summarize 等）
- input: 输入数据（Map格式，可为空）
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
      "type": "LLM",
      "name": "write_article",
      "input": {},
      "deps": []
    },
    {
      "id": "2",
      "type": "LLM",
      "name": "summarize",
      "input": {},
      "deps": ["1"]
    }
  ],
  "edges": [
    {"from": "1", "to": "2"}
  ]
}`

type NlToDagService struct {
	cfg            config.OpenAIConfig
	orchestratorURL string
	httpClient     *http.Client
}

func NewNlToDagService(cfg config.OpenAIConfig, orchestratorURL string) *NlToDagService {
	return &NlToDagService{
		cfg:             cfg,
		orchestratorURL: orchestratorURL,
		httpClient:      &http.Client{Timeout: time.Duration(cfg.Timeout) * time.Second},
	}
}

func (s *NlToDagService) TranslateToDag(ctx context.Context, prompt string) (*model.DAGRequest, error) {
	zap.L().Info("Translating natural language to DAG", zap.String("prompt", prompt))

	response, err := s.callOpenAI(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to call OpenAI: %w", err)
	}

	var dag model.DAGRequest
	if err := json.Unmarshal([]byte(response), &dag); err != nil {
		return nil, fmt.Errorf("failed to parse DAG: %w", err)
	}

	zap.L().Info("Successfully translated to DAG", zap.Int("nodeCount", len(dag.Nodes)))
	return &dag, nil
}

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

	zap.L().Info("Successfully submitted task to orchestrator")
	return result, nil
}

func (s *NlToDagService) callOpenAI(ctx context.Context, userPrompt string) (string, error) {
	if s.cfg.APIKey == "" {
		return "", fmt.Errorf("API key is not configured")
	}

	requestBody := map[string]interface{}{
		"model":       s.cfg.Model,
		"temperature": s.cfg.Temperature,
		"max_tokens":  s.cfg.MaxTokens,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	}

	body, _ := json.Marshal(requestBody)

	baseURL := s.cfg.BaseURL
	if len(baseURL) > 0 && baseURL[len(baseURL)-1] != '/' {
		baseURL += "/"
	}
	endpoint := baseURL + "chat/completions"

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("LLM API call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var responseMap map[string]interface{}
	if err := json.Unmarshal(respBody, &responseMap); err != nil {
		return "", fmt.Errorf("failed to parse LLM response: %w", err)
	}

	choices, ok := responseMap["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "", fmt.Errorf("no choices in LLM response")
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid choice format")
	}

	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid message format")
	}

	content, _ := message["content"].(string)
	return content, nil
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
