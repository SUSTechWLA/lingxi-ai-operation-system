package auth

import (
	"errors"
	"time"
)

type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusDisabled UserStatus = "disabled"
	UserStatusDeleted  UserStatus = "deleted"
)

type TokenType string

const (
	TokenTypeAccess TokenType = "access"
)

var (
	ErrEmailAlreadyRegistered = errors.New("email already registered")
	ErrInvalidCredentials     = errors.New("invalid email or password")
	ErrInvalidToken           = errors.New("invalid token")
	ErrRefreshTokenNotFound   = errors.New("refresh token not found")
	ErrUnauthorized           = errors.New("unauthorized")
	ErrUserDisabled           = errors.New("user is disabled")
	ErrUserNotFound           = errors.New("user not found")
	ErrWeakPassword           = errors.New("password must be at least 8 characters and include letters and numbers")
)

type User struct {
	ID           string     `json:"id"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	Nickname     string     `json:"nickname,omitempty"`
	AvatarURL    string     `json:"avatarUrl,omitempty"`
	Status       UserStatus `json:"status"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	LastLoginAt  *time.Time `json:"lastLoginAt,omitempty"`
}

type UserResponse struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Nickname    string     `json:"nickname,omitempty"`
	AvatarURL   string     `json:"avatarUrl,omitempty"`
	Status      UserStatus `json:"status"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	LastLoginAt *time.Time `json:"lastLoginAt,omitempty"`
}

type RegisterRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
	Nickname string `json:"nickname,omitempty"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type AuthResponse struct {
	User         UserResponse `json:"user"`
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int64        `json:"expires_in"`
}

type ClientInfo struct {
	DeviceID  string
	UserAgent string
	IPAddress string
}

type RefreshToken struct {
	ID                string
	UserID            string
	TokenHash         string
	DeviceID          string
	UserAgent         string
	IPAddress         string
	ExpiresAt         time.Time
	RevokedAt         *time.Time
	ReplacedByTokenID string
	CreatedAt         time.Time
}

type Device struct {
	ID         string
	UserID     string
	DeviceName string
	DeviceType string
	Platform   string
	LastSeenAt time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type AccessClaims struct {
	UserID    string    `json:"sub"`
	Email     string    `json:"email"`
	Nickname  string    `json:"nickname,omitempty"`
	TokenType TokenType `json:"token_type"`
	IssuedAt  int64     `json:"iat"`
	ExpiresAt int64     `json:"exp"`
}

func publicUser(user User) UserResponse {
	return UserResponse{
		ID:          user.ID,
		Email:       user.Email,
		Nickname:    user.Nickname,
		AvatarURL:   user.AvatarURL,
		Status:      user.Status,
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
		LastLoginAt: user.LastLoginAt,
	}
}
