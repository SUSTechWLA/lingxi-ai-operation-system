package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/model"
)

const (
	sessionTTL     = 30 * time.Minute
	sessionPrefix  = "skill:session:"
	maxMessages    = 50
	terminatedTTL  = 5 * time.Minute
)

// SessionManager handles Redis-backed session lifecycle for skill conversations.
type SessionManager struct {
	rdb *redis.Client
}

func NewSessionManager(rdb *redis.Client) *SessionManager {
	return &SessionManager{rdb: rdb}
}

// CreateSession initializes a new conversation context with optional media context.
func (m *SessionManager) CreateSession(ctx context.Context, userID string, mediaCtx model.MediaContext) (*model.ConversationContext, error) {
	sessionID := uuid.New().String()

	welcomeMsg := model.ChatMessage{
		Role:      "assistant",
		Content:   "你好！我是 AIOS 通用智能助手，可以帮你：\n\n• 编排和执行复杂任务工作流\n• 调用各类工具完成自动化操作\n• 生成和优化内容\n• 管理和分析数据\n\n有什么我可以帮你的吗？",
		Timestamp: time.Now(),
	}

	session := &model.ConversationContext{
		SessionID:    sessionID,
		UserID:       userID,
		Messages:     []model.ChatMessage{welcomeMsg},
		MediaContext: mediaCtx,
		Version:      1,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := m.SaveSession(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	zap.L().Info("Skill session created", zap.String("session_id", sessionID))
	return session, nil
}

// GetSession retrieves a conversation context from Redis.
// Returns nil if the session does not exist or has expired.
func (m *SessionManager) GetSession(ctx context.Context, sessionID string) (*model.ConversationContext, error) {
	key := sessionPrefix + sessionID
	data, err := m.rdb.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load session: %w", err)
	}

	var session model.ConversationContext
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}
	return &session, nil
}

// SaveSession persists a conversation context to Redis with TTL.
func (m *SessionManager) SaveSession(ctx context.Context, session *model.ConversationContext) error {
	session.UpdatedAt = time.Now()

	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	ttl := sessionTTL
	if session.Terminated {
		ttl = terminatedTTL
	}

	key := sessionPrefix + session.SessionID
	if err := m.rdb.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}
	return nil
}

// TerminateSession marks a session as terminated and shortens its TTL.
func (m *SessionManager) TerminateSession(ctx context.Context, sessionID string) error {
	session, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.Terminated = true
	return m.SaveSession(ctx, session)
}

// AppendMessage adds a message to the session and saves it.
func (m *SessionManager) AppendMessage(ctx context.Context, session *model.ConversationContext, msg model.ChatMessage) error {
	session.Messages = append(session.Messages, msg)

	// Trim oldest messages if exceeding limit
	if len(session.Messages) > maxMessages {
		overflow := len(session.Messages) - maxMessages
		session.Messages = session.Messages[overflow:]
	}

	return m.SaveSession(ctx, session)
}

