package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/config"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/tool"
)

const (
	chatSessionTTL     = 30 * time.Minute
	chatSessionPrefix  = "chat:session:"
	maxChatMessages    = 50
)

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatSession struct {
	Messages    []ChatMessage `json:"messages"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	MediaCount  int           `json:"media_count"`
}

type Suggestion struct {
	Text string `json:"text"`
	Type string `json:"type"`
}

type ChatGenerateResponse struct {
	SessionID   string                `json:"session_id"`
	Reply       string                `json:"reply"`
	Suggestions []Suggestion          `json:"suggestions,omitempty"`
	Fields      *ChatGeneratedFields  `json:"fields,omitempty"`
}

type ChatGeneratedFields struct {
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Body        string   `json:"body,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
}

type ChatGenerateRequest struct {
	SessionID      string              `json:"session_id"`
	Message        string              `json:"message"`
	CurrentContext CurrentContext      `json:"current_context"`
}

type CurrentContext struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Body        string   `json:"body"`
	Keywords    []string `json:"keywords"`
	MediaIDs    []string `json:"media_ids"`
	MediaCount  int      `json:"media_count"`
	MediaNames  []string `json:"media_names"`
}

type toolCallRequest struct {
	Name   string                 `json:"name"`
	Params map[string]interface{} `json:"params"`
}

type toolCallPlan struct {
	Reasoning string            `json:"reasoning"`
	Reply     string            `json:"reply"`
	Tools     []toolCallRequest `json:"tools"`
}

type generateParseResult struct {
	Reply       string
	Suggestions []Suggestion
	Fields      *ChatGeneratedFields
	ToolCall    *toolCallPlan
}

type ChatService struct {
	cfg             config.OpenAIConfig
	rdb             *redis.Client
	orchestratorURL string
	httpClient      *http.Client
	toolRegistry    *tool.ToolRegistry
}

func NewChatService(cfg config.OpenAIConfig, rdb *redis.Client, orchestratorURL string, tr *tool.ToolRegistry) *ChatService {
	return &ChatService{
		cfg:             cfg,
		rdb:             rdb,
		orchestratorURL: orchestratorURL,
		httpClient:      &http.Client{Timeout: time.Duration(cfg.Timeout) * time.Second},
		toolRegistry:    tr,
	}
}

// Generate handles conversational content generation with tool-calling capability.
// It calls the LLM directly (not through DAG) to determine intent and optionally
// executes tools through the Orchestrator → Worker → Tool DAG pipeline.
func (s *ChatService) Generate(ctx context.Context, req *ChatGenerateRequest) (*ChatGenerateResponse, error) {
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	// Load or create session
	session, err := s.loadSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session: %w", err)
	}
	if session == nil {
		session = &ChatSession{
			Messages:   make([]ChatMessage, 0, 8),
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			MediaCount: req.CurrentContext.MediaCount,
		}
	}

	// Append user message
	session.Messages = append(session.Messages, ChatMessage{Role: "user", Content: req.Message})
	session.UpdatedAt = time.Now()

	// Trim oldest messages if exceeding limit
	if len(session.Messages) > maxChatMessages {
		overflow := len(session.Messages) - maxChatMessages
		keepIdx := overflow
		if len(session.Messages) > 0 && session.Messages[0].Role == "system" {
			keepIdx = overflow + 1
		}
		session.Messages = session.Messages[keepIdx:]
	}

	// Phase 1: Call LLM with tool descriptions to determine intent
	toolDescs := s.getToolDescriptions()
	openAIMessages := s.buildGenerateMessages(session.Messages, req.CurrentContext, toolDescs)

	zap.L().Info("Calling LLM for intent determination",
		zap.String("sessionId", sessionID),
		zap.Int("messageCount", len(openAIMessages)),
		zap.Bool("hasTools", toolDescs != ""))

	phase1Reply, err := s.callOpenAI(ctx, openAIMessages)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse response
	parsed := s.parseGenerateResponse(phase1Reply)

	// Phase 2: If LLM requested tool execution, run tools via DAG and synthesize
	if parsed.ToolCall != nil && len(parsed.ToolCall.Tools) > 0 {
		zap.L().Info("AI requested tool execution",
			zap.Int("toolCount", len(parsed.ToolCall.Tools)),
			zap.String("reasoning", parsed.ToolCall.Reasoning))

		toolResults, execErr := s.executeTools(ctx, parsed.ToolCall.Tools)
		if execErr != nil {
			zap.L().Error("Tool execution failed, falling back to direct reply", zap.Error(execErr))
			// Fallback: use the intermediate reply
			return s.buildResponse(sessionID, session, parsed.Reply, parsed.Suggestions, parsed.Fields)
		}

		// Phase 3: Synthesize final response with tool results
		synthesisMessages := s.buildSynthesisMessages(session.Messages, req.CurrentContext, parsed.Reply, toolResults)
		synthesis, synErr := s.callOpenAI(ctx, synthesisMessages)
		if synErr != nil {
			zap.L().Error("Synthesis LLM call failed, using intermediate reply", zap.Error(synErr))
			return s.buildResponse(sessionID, session, parsed.Reply, parsed.Suggestions, parsed.Fields)
		}

		// Re-parse the synthesized response
		parsed = s.parseGenerateResponse(synthesis)
	}

	return s.buildResponse(sessionID, session, parsed.Reply, parsed.Suggestions, parsed.Fields)
}

// buildResponse constructs the final ChatGenerateResponse and manages session state.
func (s *ChatService) buildResponse(
	sessionID string,
	session *ChatSession,
	reply string,
	suggestions []Suggestion,
	fields *ChatGeneratedFields,
) (*ChatGenerateResponse, error) {
	// If complete fields returned, close the session (generation complete)
	if fields != nil {
		s.deleteSession(context.Background(), sessionID)
		return &ChatGenerateResponse{
			SessionID:   sessionID,
			Reply:       reply,
			Fields:      fields,
		}, nil
	}

	// Save AI reply and update session for continued conversation
	session.Messages = append(session.Messages, ChatMessage{Role: "assistant", Content: reply})
	if err := s.saveSession(context.Background(), sessionID, session); err != nil {
		zap.L().Warn("failed to save chat session", zap.String("session_id", sessionID), zap.Error(err))
	}

	return &ChatGenerateResponse{
		SessionID:   sessionID,
		Reply:       reply,
		Suggestions: suggestions,
	}, nil
}

// getToolDescriptions generates a detailed description of all available tools for the LLM system prompt.
// Uses manifests to provide parameter schemas, output schemas, sandbox requirements, and examples.
func (s *ChatService) getToolDescriptions() string {
	if s.toolRegistry == nil {
		return ""
	}

	manifests := s.toolRegistry.ListManifests()
	if len(manifests) == 0 {
		return ""
	}

	// Internal tools that shouldn't be user-invokable
	internalTools := map[string]bool{
		"chat_generate": true,
		"chat_revise":   true,
		"llm_api":       true,
		"external":      true,
	}

	var toolBlock string
	for _, m := range manifests {
		if internalTools[m.Name] {
			continue
		}
		if m.Description == "" {
			continue
		}

		toolBlock += fmt.Sprintf("### %s\n", m.Name)
		toolBlock += fmt.Sprintf("描述: %s\n", m.Description)

		if len(m.Parameters) > 0 {
			toolBlock += "参数:\n"
			for name, param := range m.Parameters {
				req := ""
				if param.Required {
					req = " (必填)"
				}
				toolBlock += fmt.Sprintf("  - %s: %s%s\n", name, param.Description, req)
			}
		}

		if len(m.Output) > 0 {
			toolBlock += "输出:\n"
			for name, out := range m.Output {
				toolBlock += fmt.Sprintf("  - %s: %s\n", name, out.Description)
			}
		}

		if m.Sandbox {
			toolBlock += "安全: 此工具在沙箱中执行，安全隔离\n"
		}

		if len(m.Examples) > 0 {
			toolBlock += "示例:\n"
			for _, ex := range m.Examples {
				inJSON, _ := json.Marshal(ex.Input)
				outJSON, _ := json.Marshal(ex.Output)
				toolBlock += fmt.Sprintf("  输入: %s\n  输出: %s\n", string(inJSON), string(outJSON))
			}
		}
		toolBlock += "\n"
	}

	if toolBlock == "" {
		return ""
	}

	return fmt.Sprintf(`## 可用工具
你可以调用以下工具来完成用户请求。当现有工具都不适合时，可以使用 bash 或 python 工具自行创建临时脚本来完成任务。

%s
## 工具调用规则
1. 当用户请求适合使用某工具时，输出：
   {"type": "tool_call", "reasoning": "为什么调用此工具", "reply": "给用户的回复", "tools": [{"name": "工具名", "params": {...}}]}
2. 系统将自动执行工具并把结果返回给你进行最终合成
3. 如果没有合适的现有工具，考虑使用 bash/python 创建临时脚本
4. bash 在沙箱中执行，支持常用命令（ls, cat, echo, curl, python3, node, grep 等）
5. python 可用于数组/对象处理、API 调用、文本处理等

## 自创工具指南
如果用户需求没有现有工具可用：
1. 使用 bash 可以执行终端命令、调用 curl 访问 API、运行脚本
2. 使用 python 可以处理数据、调用外部 API、批量操作
3. bash 的 command 参数传入要执行的 shell 命令
4. python 的 code 参数传入 python3 -c 执行的代码
5. 注意：bash 沙箱有命令白名单，可能不能安装软件包

否则，直接以对话或内容生成的方式回复用户（使用 question 或 complete 类型）。`, toolBlock)
}

// buildGenerateMessages builds the OpenAI messages array from session history, current context, and tool descriptions.
func (s *ChatService) buildGenerateMessages(messages []ChatMessage, ctx CurrentContext, toolDescriptions string) []map[string]string {
	mediaInfo := ""
	if ctx.MediaCount > 0 {
		mediaInfo = fmt.Sprintf("\n用户已上传 %d 个素材文件（图片/视频），素材已被成功接收。", ctx.MediaCount)
		if len(ctx.MediaNames) > 0 {
			mediaInfo += fmt.Sprintf("\n素材文件名：%v", ctx.MediaNames)
		}
		mediaInfo += "\n重要：当用户要求生成内容（如生成标题、简介、正文等）时，请直接根据已有素材信息进行创作，不要询问需要用户提供更多图片或素材。"
	}
	if len(ctx.MediaIDs) > 0 {
		mediaInfo += " 素材ID: " + fmt.Sprintf("%v", ctx.MediaIDs)
	}
	if ctx.Title != "" || ctx.Description != "" {
		mediaInfo += fmt.Sprintf("\n当前已有内容:\n  标题: %s\n  简介: %s", ctx.Title, ctx.Description)
	}

	systemPrompt := fmt.Sprintf(`你是一个专业的内容创作助手，帮助用户创作优质的自媒体内容。通过对话方式了解用户需求，然后生成完整内容。

## 工作流程
1. 如果信息不足，一次只问一个问题，引导用户补充信息
2. 需要了解的信息包括：主题/话题、写作风格(治愈/干货/专业/幽默等)、目标受众、传达的核心感受
3. 当信息足够时(至少确定主题+风格)，生成完整内容
4. 重要：如果用户主动要求生成内容（如"生成标题和简介"、"帮我写内容"等），或用户已上传素材且明确要求生成，应视为信息足够，直接生成完整内容

## 回答格式
- 如果还需要更多信息：
{"type": "question", "reply": "你的引导性问题", "suggestions": [{"text": "选项1", "type": "style"}, {"text": "选项2", "type": "style"}]}

- 如果信息足够，生成完整内容：
{"type": "complete", "reply": "内容已生成！", "fields": {"title": "生成的标题", "description": "生成的简介", "body": "生成的正文", "keywords": ["关键词1", "关键词2"]}}

%s
%s
请用中文回复。`, toolDescriptions, mediaInfo)

	openAIMessages := make([]map[string]string, 0, len(messages)+1)
	openAIMessages = append(openAIMessages, map[string]string{"role": "system", "content": systemPrompt})
	for _, m := range messages {
		openAIMessages = append(openAIMessages, map[string]string{"role": m.Role, "content": m.Content})
	}

	return openAIMessages
}

// buildSynthesisMessages builds messages for the synthesis call with tool execution results.
func (s *ChatService) buildSynthesisMessages(
	messages []ChatMessage,
	ctx CurrentContext,
	intermediateReply string,
	toolResults map[string]interface{},
) []map[string]string {
	resultsJSON, _ := json.MarshalIndent(toolResults, "", "  ")

	synthesisPrompt := fmt.Sprintf(`你是 AI 创作助手。之前你决定调用工具来完成用户请求，以下是工具执行的结果。

你的中间回复：
%s

工具执行结果：
%s

请根据工具执行结果，生成最终回复。
- 如果结果包含可直接应用的字段，输出 complete 类型
- 如果需要进一步与用户对话，输出 question 类型
- 直接给出最终答复，不要问"是否要应用这些更改"

## 回答格式
{"type": "complete", "reply": "回复内容", "fields": {"title": "标题", "description": "简介", "keywords": ["关键词"]}}
{"type": "question", "reply": "回复内容", "suggestions": [{"text": "选项", "type": "type"}]}
{"type": "direct", "reply": "回复内容"}`, intermediateReply, string(resultsJSON))

	openAIMessages := make([]map[string]string, 0, len(messages)+2)
	openAIMessages = append(openAIMessages, map[string]string{"role": "system", "content": string(synthesisPrompt)})
	for _, m := range messages {
		openAIMessages = append(openAIMessages, map[string]string{"role": m.Role, "content": m.Content})
	}
	openAIMessages = append(openAIMessages, map[string]string{
		"role":    "system",
		"content": "请根据工具执行结果生成最终回复。",
	})

	return openAIMessages
}

// callOpenAI makes a direct HTTP call to the OpenAI API with the given messages.
func (s *ChatService) callOpenAI(ctx context.Context, messages []map[string]string) (string, error) {
	if s.cfg.APIKey == "" {
		return "", fmt.Errorf("API key is not configured")
	}

	requestBody := map[string]interface{}{
		"model":       s.cfg.Model,
		"temperature": s.cfg.Temperature,
		"max_tokens":  s.cfg.MaxTokens,
		"messages":    messages,
	}

	body, _ := json.Marshal(requestBody)

	baseURL := s.cfg.BaseURL
	if len(baseURL) > 0 && baseURL[len(baseURL)-1] != '/' {
		baseURL += "/"
	}
	endpoint := baseURL + "chat/completions"

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
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

	msg, ok := choice["message"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid message format")
	}

	content, _ := msg["content"].(string)
	if content == "" {
		return "", fmt.Errorf("empty response from LLM")
	}

	return content, nil
}

// executeTools creates a DAG through the orchestrator to execute the requested tools and returns their results.
func (s *ChatService) executeTools(ctx context.Context, tools []toolCallRequest) (map[string]interface{}, error) {
	if len(tools) == 0 {
		return nil, nil
	}

	// Build DAG nodes
	ts := time.Now().UnixMilli()
	nodes := make([]map[string]interface{}, 0, len(tools))
	nodeIDs := make([]string, 0, len(tools))

	for i, toolReq := range tools {
		nodeID := fmt.Sprintf("chat-tool-%s-%d-%d", toolReq.Name, ts, i)
		nodeIDs = append(nodeIDs, nodeID)

		node := map[string]interface{}{
			"id":   nodeID,
			"type": "TOOL",
			"name": toolReq.Name,
		}

		// Pass parameters directly as the input
		if toolReq.Params != nil {
			node["input"] = toolReq.Params
		} else {
			node["input"] = map[string]interface{}{}
		}

		nodes = append(nodes, node)
	}

	dagPayload := map[string]interface{}{
		"nodes": nodes,
		"edges": []map[string]interface{}{},
	}

	dagBody, _ := json.Marshal(dagPayload)

	// Submit DAG to orchestrator (creates task + submits DAG in one call)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", s.orchestratorURL+"/api/node", bytes.NewReader(dagBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create DAG submit request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to submit DAG: %w", err)
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

	zap.L().Info("Tool execution DAG submitted",
		zap.String("taskId", taskID),
		zap.Int("toolCount", len(tools)))

	// Poll for node results (max 60s, every 500ms)
	return s.pollToolResults(ctx, taskID, nodeIDs)
}

// pollToolResults polls the orchestrator for all tool node results and returns them as a map.
func (s *ChatService) pollToolResults(ctx context.Context, taskID string, nodeIDs []string) (map[string]interface{}, error) {
	maxAttempts := 120 // 120 * 500ms = 60s
	pollInterval := 500 * time.Millisecond

	nodeIDSet := make(map[string]bool, len(nodeIDs))
	for _, id := range nodeIDs {
		nodeIDSet[id] = true
	}

	for i := 0; i < maxAttempts; i++ {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled while polling tool results: %w", ctx.Err())
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

		nodes, ok := taskResult["nodes"].([]interface{})
		if !ok || len(nodes) == 0 {
			time.Sleep(pollInterval)
			continue
		}

		results := make(map[string]interface{})
		allComplete := true

		for _, n := range nodes {
			node, ok := n.(map[string]interface{})
			if !ok {
				continue
			}

			nid, _ := node["id"].(string)
			if !nodeIDSet[nid] {
				continue
			}

			status, _ := node["status"].(string)
			switch status {
			case "SUCCESS":
				results[nid] = extractToolOutput(node)
			case "FAILED":
				errMsg, _ := node["errorMessage"].(string)
				if errMsg == "" {
					errMsg = "unknown error"
				}
				results[nid] = map[string]interface{}{"error": errMsg}
				// Mark as complete even on failure
				results[nid] = extractToolOutput(node)
			default:
				// Still running
				allComplete = false
			}
		}

		if allComplete {
			// Collect all results into a single map
			combined := make(map[string]interface{})
			for _, n := range nodes {
				node, ok := n.(map[string]interface{})
				if !ok {
					continue
				}
				nid, _ := node["id"].(string)
				if !nodeIDSet[nid] {
					continue
				}
				combined[nid] = results[nid]
			}
			combined["_task_id"] = taskID
			return combined, nil
		}

		time.Sleep(pollInterval)
	}

	return nil, fmt.Errorf("tool execution timed out for task %s", taskID)
}

// extractToolOutput extracts the tool result from a node's output, handling both
// stdout and direct output fields.
func extractToolOutput(node map[string]interface{}) map[string]interface{} {
	output, ok := node["output"].(map[string]interface{})
	if !ok {
		// Try direct fields on the node
		result := make(map[string]interface{})
		if stdout, ok := node["stdout"].(string); ok {
			result["_stdout"] = stdout
		}
		if err, ok := node["errorMessage"].(string); ok {
			result["_error"] = err
		}
		return result
	}

	// Try to parse stdout as tool result JSON
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

// buildGenerateMessages is kept for backward compatibility
// (used from buildGenerateMessages with toolDescriptions)
func (s *ChatService) parseGenerateResponse(raw string) generateParseResult {
	// Try with tool_call support first
	var toolParsed struct {
		Type        string               `json:"type"`
		Reply       string               `json:"reply"`
		Suggestions []Suggestion         `json:"suggestions"`
		Fields      *ChatGeneratedFields `json:"fields"`
		Reasoning   string               `json:"reasoning"`
		Tools       []toolCallRequest    `json:"tools"`
	}

	if err := json.Unmarshal([]byte(raw), &toolParsed); err == nil && toolParsed.Type != "" {
		result := generateParseResult{
			Reply:       toolParsed.Reply,
			Suggestions: toolParsed.Suggestions,
			Fields:      toolParsed.Fields,
		}

		// Handle tool_call type
		if toolParsed.Type == "tool_call" && len(toolParsed.Tools) > 0 {
			result.ToolCall = &toolCallPlan{
				Reasoning: toolParsed.Reasoning,
				Reply:     toolParsed.Reply,
				Tools:     toolParsed.Tools,
			}
		}

		return result
	}

	// If not JSON, treat entire response as reply text
	return generateParseResult{
		Reply: raw,
	}
}

// --- Redis session management ---

func (s *ChatService) sessionKey(sessionID string) string {
	return chatSessionPrefix + sessionID
}

func (s *ChatService) loadSession(ctx context.Context, sessionID string) (*ChatSession, error) {
	data, err := s.rdb.Get(ctx, s.sessionKey(sessionID)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var session ChatSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *ChatService) saveSession(ctx context.Context, sessionID string, session *ChatSession) error {
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, s.sessionKey(sessionID), data, chatSessionTTL).Err()
}

func (s *ChatService) deleteSession(ctx context.Context, sessionID string) error {
	return s.rdb.Del(ctx, s.sessionKey(sessionID)).Err()
}
