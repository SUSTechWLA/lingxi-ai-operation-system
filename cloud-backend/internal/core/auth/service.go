package auth

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repo   Repository
	tokens *TokenIssuer
}

func NewService(repo Repository, tokens *TokenIssuer) *Service {
	return &Service{repo: repo, tokens: tokens}
}

func (s *Service) Register(ctx context.Context, req RegisterRequest, client ClientInfo) (*AuthResponse, error) {
	email, err := normalizeEmail(req.Email)
	if err != nil {
		return nil, err
	}
	if err := ValidatePassword(req.Password); err != nil {
		return nil, err
	}
	if existing, err := s.repo.FindUserByEmail(ctx, email); err == nil && existing != nil {
		return nil, ErrEmailAlreadyRegistered
	} else if err != nil && !errors.Is(err, ErrUserNotFound) {
		return nil, err
	}
	hash, err := HashPassword(req.Password)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	user := &User{
		ID:           "u_" + uuid.NewString(),
		Email:        email,
		PasswordHash: hash,
		Nickname:     strings.TrimSpace(req.Nickname),
		Status:       UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, err
	}
	return s.issueSession(ctx, *user, client, true)
}

func (s *Service) Login(ctx context.Context, req LoginRequest, client ClientInfo) (*AuthResponse, error) {
	email, err := normalizeEmail(req.Email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	user, err := s.repo.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if user.Status != UserStatusActive {
		return nil, ErrUserDisabled
	}
	if err := VerifyPassword(user.PasswordHash, req.Password); err != nil {
		return nil, ErrInvalidCredentials
	}
	return s.issueSession(ctx, *user, client, true)
}

func (s *Service) Refresh(ctx context.Context, req RefreshRequest, client ClientInfo) (*AuthResponse, error) {
	plain := strings.TrimSpace(req.RefreshToken)
	if plain == "" {
		return nil, ErrInvalidToken
	}
	existing, err := s.repo.FindRefreshTokenByHash(ctx, HashRefreshToken(plain))
	if err != nil {
		return nil, ErrInvalidToken
	}
	if existing.RevokedAt != nil || !existing.ExpiresAt.After(time.Now().UTC()) {
		return nil, ErrInvalidToken
	}
	user, err := s.repo.FindUserByID(ctx, existing.UserID)
	if err != nil {
		return nil, ErrInvalidToken
	}
	if user.Status != UserStatusActive {
		return nil, ErrUserDisabled
	}
	resp, err := s.issueSession(ctx, *user, client, false)
	if err != nil {
		return nil, err
	}
	rotated, err := s.repo.FindRefreshTokenByHash(ctx, HashRefreshToken(resp.RefreshToken))
	if err != nil {
		return nil, err
	}
	if err := s.repo.RevokeRefreshToken(ctx, existing.ID, rotated.ID, time.Now().UTC()); err != nil {
		return nil, err
	}
	return resp, nil
}

func (s *Service) Logout(ctx context.Context, userID string, refreshToken string) error {
	plain := strings.TrimSpace(refreshToken)
	if userID == "" || plain == "" {
		return ErrUnauthorized
	}
	token, err := s.repo.FindRefreshTokenByHash(ctx, HashRefreshToken(plain))
	if err != nil {
		return ErrInvalidToken
	}
	if token.UserID != userID {
		return ErrUnauthorized
	}
	if token.RevokedAt != nil {
		return nil
	}
	return s.repo.RevokeRefreshToken(ctx, token.ID, "", time.Now().UTC())
}

func (s *Service) GetUser(ctx context.Context, userID string) (*UserResponse, error) {
	user, err := s.repo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Status != UserStatusActive {
		return nil, ErrUserDisabled
	}
	public := publicUser(*user)
	return &public, nil
}

func (s *Service) ValidateAccessToken(ctx context.Context, accessToken string) (*User, error) {
	claims, err := s.tokens.ValidateAccessToken(accessToken)
	if err != nil {
		return nil, err
	}
	user, err := s.repo.FindUserByID(ctx, claims.UserID)
	if err != nil {
		return nil, ErrUnauthorized
	}
	if user.Status != UserStatusActive {
		return nil, ErrUserDisabled
	}
	return user, nil
}

func (s *Service) issueSession(ctx context.Context, user User, client ClientInfo, updateLastLogin bool) (*AuthResponse, error) {
	access, expiresAt, err := s.tokens.IssueAccessToken(user)
	if err != nil {
		return nil, err
	}
	refresh, err := s.tokens.NewRefreshToken()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	refreshRecord := &RefreshToken{
		ID:        "rt_" + uuid.NewString(),
		UserID:    user.ID,
		TokenHash: HashRefreshToken(refresh),
		DeviceID:  strings.TrimSpace(client.DeviceID),
		UserAgent: client.UserAgent,
		IPAddress: client.IPAddress,
		ExpiresAt: now.Add(s.tokens.RefreshTokenTTL()),
		CreatedAt: now,
	}
	if err := s.repo.SaveRefreshToken(ctx, refreshRecord); err != nil {
		return nil, err
	}
	if refreshRecord.DeviceID != "" {
		_ = s.repo.UpsertDevice(ctx, &Device{
			ID:         refreshRecord.DeviceID,
			UserID:     user.ID,
			LastSeenAt: now,
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}
	if updateLastLogin {
		if err := s.repo.UpdateLastLogin(ctx, user.ID, now); err == nil {
			user.LastLoginAt = &now
		}
	}
	return &AuthResponse{
		User:         publicUser(user),
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(time.Until(expiresAt).Seconds()),
	}, nil
}

func normalizeEmail(email string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" {
		return "", ErrInvalidCredentials
	}
	if _, err := mail.ParseAddress(normalized); err != nil {
		return "", ErrInvalidCredentials
	}
	return normalized, nil
}
