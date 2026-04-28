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
)

const (
	chatSessionTTL   = 30 * time.Minute
	chatSessionPrefix = "chat:session:"
	maxChatMessages   = 50
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

type ChatReviseResponse struct {
	Reply  string               `json:"reply"`
	Fields ChatGeneratedFields  `json:"fields"`
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
}

type ChatReviseRequest struct {
	Message       string              `json:"message"`
	CurrentFields CurrentContext      `json:"current_fields"`
}

type ChatService struct {
	cfg        config.OpenAIConfig
	rdb        *redis.Client
	httpClient *http.Client
}

func NewChatService(cfg config.OpenAIConfig, rdb *redis.Client) *ChatService {
	return &ChatService{
		cfg:        cfg,
		rdb:        rdb,
		httpClient: &http.Client{Timeout: time.Duration(cfg.Timeout) * time.Second},
	}
}

// Generate handles conversational content generation.
// It maintains session state in Redis and returns either a guiding question
// with suggestions, or complete generated fields when enough info is gathered.
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
		// Keep the system prompt (first message) and recent N-1 messages
		overflow := len(session.Messages) - maxChatMessages
		keepIdx := overflow
		// If the first message is system prompt, keep it
		if len(session.Messages) > 0 && session.Messages[0].Role == "system" {
			keepIdx = overflow + 1
		}
		session.Messages = session.Messages[keepIdx:]
	}

	// Build OpenAI messages
	openAIMessages := s.buildGenerateMessages(session.Messages, req.CurrentContext)

	// Call OpenAI
	reply, err := s.callOpenAI(ctx, openAIMessages)
	if err != nil {
		return nil, fmt.Errorf("openai call failed: %w", err)
	}

	// Parse response
	parsed := s.parseGenerateResponse(reply)

	// If complete fields returned, close the session (no need to save)
	if parsed.Fields != nil {
		s.deleteSession(ctx, sessionID)
		return &ChatGenerateResponse{
			SessionID: sessionID,
			Reply:     parsed.Reply,
			Fields:    parsed.Fields,
		}, nil
	}

	// Otherwise, save AI reply and update session
	session.Messages = append(session.Messages, ChatMessage{Role: "assistant", Content: reply})
	if err := s.saveSession(ctx, sessionID, session); err != nil {
		zap.L().Warn("failed to save chat session", zap.String("session_id", sessionID), zap.Error(err))
	}

	return &ChatGenerateResponse{
		SessionID:   sessionID,
		Reply:       parsed.Reply,
		Suggestions: parsed.Suggestions,
	}, nil
}

// Revise handles natural language content revision.
// It sends the user's request + current field values to OpenAI and returns modified fields.
func (s *ChatService) Revise(ctx context.Context, req *ChatReviseRequest) (*ChatReviseResponse, error) {
	messages := s.buildReviseMessages(req)

	reply, err := s.callOpenAI(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("openai call failed: %w", err)
	}

	parsed := s.parseReviseResponse(reply)

	return &ChatReviseResponse{
		Reply:  parsed.Reply,
		Fields: parsed.Fields,
	}, nil
}

func (s *ChatService) buildGenerateMessages(messages []ChatMessage, ctx CurrentContext) []map[string]string {
	mediaInfo := ""
	if ctx.MediaCount > 0 {
		mediaInfo = fmt.Sprintf("\n用户已上传 %d 个素材文件。", ctx.MediaCount)
	}
	if len(ctx.MediaIDs) > 0 {
		mediaInfo += " 素材ID: " + fmt.Sprintf("%v", ctx.MediaIDs)
	}
	if ctx.Title != "" || ctx.Description != "" {
		mediaInfo += fmt.Sprintf("\n当前已有内容:\n  标题: %s\n  简介: %s", ctx.Title, ctx.Description)
	}

	systemPrompt := `你是一个专业的内容创作助手，帮助用户创作优质的自媒体内容。通过对话方式了解用户需求，然后生成完整内容。

## 工作流程
1. 如果信息不足，一次只问一个问题，引导用户补充信息
2. 需要了解的信息包括：主题/话题、写作风格(治愈/干货/专业/幽默等)、目标受众、传达的核心感受
3. 当信息足够时(至少确定主题+风格)，生成完整内容

## 回答格式
- 如果还需要更多信息：
{"type": "question", "reply": "你的引导性问题", "suggestions": [{"text": "选项1", "type": "style"}, {"text": "选项2", "type": "style"}]}

- 如果信息足够，生成完整内容：
{"type": "complete", "reply": "内容已生成！", "fields": {"title": "生成的标题", "description": "生成的简介", "body": "生成的正文", "keywords": ["关键词1", "关键词2"]}}

请用中文回复。` + mediaInfo

	openAIMessages := make([]map[string]string, 0, len(messages)+1)
	openAIMessages = append(openAIMessages, map[string]string{"role": "system", "content": systemPrompt})
	for _, m := range messages {
		openAIMessages = append(openAIMessages, map[string]string{"role": m.Role, "content": m.Content})
	}

	return openAIMessages
}

func (s *ChatService) buildReviseMessages(req *ChatReviseRequest) []map[string]string {
	currentContent := fmt.Sprintf(`当前内容：
标题：%s
简介：%s
关键词：%v`, req.CurrentFields.Title, req.CurrentFields.Description, req.CurrentFields.Keywords)

	systemPrompt := `你是一个内容修改助手。根据用户的修改要求，只修改指定的字段，其他字段保持不变。

## 回答格式
{
  "type": "revise",
  "reply": "简要说明修改了什么",
  "fields": {
    "title": "修改后的标题（如果没修改则留空）",
    "description": "修改后的简介（如果没修改则留空）",
    "keywords": ["修改后的关键词（如果没修改则留空数组）"]
  }
}

请用中文回复。\n\n` + currentContent

	userMessage := fmt.Sprintf("用户要求：%s", req.Message)

	return []map[string]string{
		{"role": "system", "content": systemPrompt},
		{"role": "user", "content": userMessage},
	}
}

type generateParseResult struct {
	Reply       string
	Suggestions []Suggestion
	Fields      *ChatGeneratedFields
}

func (s *ChatService) parseGenerateResponse(raw string) generateParseResult {
	var parsed struct {
		Type        string              `json:"type"`
		Reply       string              `json:"reply"`
		Suggestions []Suggestion        `json:"suggestions"`
		Fields      *ChatGeneratedFields `json:"fields"`
	}

	if err := json.Unmarshal([]byte(raw), &parsed); err == nil && parsed.Type != "" {
		return generateParseResult{
			Reply:       parsed.Reply,
			Suggestions: parsed.Suggestions,
			Fields:      parsed.Fields,
		}
	}

	// If not JSON, treat entire response as reply text
	return generateParseResult{
		Reply: raw,
	}
}

type reviseParseResult struct {
	Reply  string
	Fields ChatGeneratedFields
}

func (s *ChatService) parseReviseResponse(raw string) reviseParseResult {
	var parsed struct {
		Type   string              `json:"type"`
		Reply  string              `json:"reply"`
		Fields ChatGeneratedFields `json:"fields"`
	}

	if err := json.Unmarshal([]byte(raw), &parsed); err == nil && parsed.Type == "revise" {
		return reviseParseResult{
			Reply:  parsed.Reply,
			Fields: parsed.Fields,
		}
	}

	// Fallback: return raw text as reply
	return reviseParseResult{
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

// callOpenAI sends a request to the OpenAI-compatible API and returns the response content.
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
