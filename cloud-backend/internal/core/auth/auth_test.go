package auth

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := HashPassword("Password123456")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if hash == "Password123456" {
		t.Fatal("password hash must not equal plaintext")
	}
	if err := VerifyPassword(hash, "Password123456"); err != nil {
		t.Fatalf("expected password to verify: %v", err)
	}
	if err := VerifyPassword(hash, "wrong-password"); err == nil {
		t.Fatal("wrong password should not verify")
	}
}

func TestTokenIssuerCreatesAndValidatesAccessToken(t *testing.T) {
	issuer := NewTokenIssuer(TokenConfig{
		Secret:          "unit-test-secret",
		AccessTokenTTL:  time.Hour,
		RefreshTokenTTL: 30 * 24 * time.Hour,
	})

	token, expiresAt, err := issuer.IssueAccessToken(User{
		ID:       "u_123",
		Email:    "user@example.com",
		Nickname: "User",
		Status:   UserStatusActive,
	})
	if err != nil {
		t.Fatalf("IssueAccessToken returned error: %v", err)
	}
	if token == "" {
		t.Fatal("access token should not be empty")
	}
	if !expiresAt.After(time.Now()) {
		t.Fatalf("expiresAt should be in the future: %s", expiresAt)
	}

	claims, err := issuer.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("ValidateAccessToken returned error: %v", err)
	}
	if claims.UserID != "u_123" {
		t.Fatalf("claims.UserID = %q, want u_123", claims.UserID)
	}
	if claims.Email != "user@example.com" {
		t.Fatalf("claims.Email = %q", claims.Email)
	}
	if claims.TokenType != TokenTypeAccess {
		t.Fatalf("claims.TokenType = %q", claims.TokenType)
	}
}

func TestTokenIssuerRejectsExpiredToken(t *testing.T) {
	issuer := NewTokenIssuer(TokenConfig{
		Secret:          "unit-test-secret",
		AccessTokenTTL:  -time.Minute,
		RefreshTokenTTL: 30 * 24 * time.Hour,
	})
	token, _, err := issuer.IssueAccessToken(User{ID: "u_123", Email: "user@example.com", Status: UserStatusActive})
	if err != nil {
		t.Fatalf("IssueAccessToken returned error: %v", err)
	}
	if _, err := issuer.ValidateAccessToken(token); err == nil {
		t.Fatal("expired access token should be rejected")
	}
}

func TestServiceRegisterLoginRefreshAndLogout(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryRepository()
	svc := NewService(repo, NewTokenIssuer(TokenConfig{
		Secret:          "unit-test-secret",
		AccessTokenTTL:  time.Hour,
		RefreshTokenTTL: 30 * 24 * time.Hour,
	}))
	client := ClientInfo{DeviceID: "device-1", UserAgent: "go-test", IPAddress: "127.0.0.1"}

	registered, err := svc.Register(ctx, RegisterRequest{
		Email:    "User@Example.com",
		Password: "Password123456",
		Nickname: "User",
	}, client)
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if registered.User.ID == "" {
		t.Fatal("registered user should have an id")
	}
	if registered.User.Email != "user@example.com" {
		t.Fatalf("email should be normalized, got %q", registered.User.Email)
	}
	if registered.AccessToken == "" || registered.RefreshToken == "" {
		t.Fatal("register should return both tokens")
	}

	if _, err := svc.Register(ctx, RegisterRequest{
		Email:    "user@example.com",
		Password: "Password123456",
	}, client); err == nil {
		t.Fatal("duplicate email registration should fail")
	}

	loggedIn, err := svc.Login(ctx, LoginRequest{
		Email:    "USER@example.com",
		Password: "Password123456",
	}, client)
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
	if loggedIn.User.ID != registered.User.ID {
		t.Fatalf("logged in user id = %q, want %q", loggedIn.User.ID, registered.User.ID)
	}
	if loggedIn.RefreshToken == registered.RefreshToken {
		t.Fatal("login should issue a new refresh token")
	}

	refreshed, err := svc.Refresh(ctx, RefreshRequest{RefreshToken: loggedIn.RefreshToken}, client)
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}
	if refreshed.User.ID != registered.User.ID {
		t.Fatalf("refreshed user id = %q, want %q", refreshed.User.ID, registered.User.ID)
	}
	if refreshed.RefreshToken == loggedIn.RefreshToken {
		t.Fatal("refresh should rotate refresh token")
	}
	if _, err := svc.Refresh(ctx, RefreshRequest{RefreshToken: loggedIn.RefreshToken}, client); err == nil {
		t.Fatal("old refresh token should be revoked after rotation")
	}

	if err := svc.Logout(ctx, registered.User.ID, refreshed.RefreshToken); err != nil {
		t.Fatalf("Logout returned error: %v", err)
	}
	if _, err := svc.Refresh(ctx, RefreshRequest{RefreshToken: refreshed.RefreshToken}, client); err == nil {
		t.Fatal("logged out refresh token should be revoked")
	}
}

func TestServiceRejectsWeakPasswordAndBadLogin(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryRepository()
	svc := NewService(repo, NewTokenIssuer(TokenConfig{
		Secret:          "unit-test-secret",
		AccessTokenTTL:  time.Hour,
		RefreshTokenTTL: 30 * 24 * time.Hour,
	}))
	client := ClientInfo{}

	if _, err := svc.Register(ctx, RegisterRequest{Email: "bad@example.com", Password: "short"}, client); err == nil {
		t.Fatal("weak password should be rejected")
	}

	if _, err := svc.Register(ctx, RegisterRequest{Email: "user@example.com", Password: "Password123456"}, client); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if _, err := svc.Login(ctx, LoginRequest{Email: "user@example.com", Password: "wrong-password"}, client); err == nil {
		t.Fatal("bad login should be rejected")
	}
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		usersByID:      map[string]*User{},
		usersByEmail:   map[string]*User{},
		refreshByID:    map[string]*RefreshToken{},
		refreshByHash:  map[string]*RefreshToken{},
		devicesByID:    map[string]*Device{},
		nextUserNumber: 1,
	}
}

type memoryRepository struct {
	usersByID      map[string]*User
	usersByEmail   map[string]*User
	refreshByID    map[string]*RefreshToken
	refreshByHash  map[string]*RefreshToken
	devicesByID    map[string]*Device
	nextUserNumber int
}

func (r *memoryRepository) CreateUser(_ context.Context, user *User) error {
	if _, exists := r.usersByEmail[user.Email]; exists {
		return ErrEmailAlreadyRegistered
	}
	if user.ID == "" {
		user.ID = "u_mem"
	}
	cp := *user
	r.usersByID[user.ID] = &cp
	r.usersByEmail[user.Email] = &cp
	return nil
}

func (r *memoryRepository) FindUserByEmail(_ context.Context, email string) (*User, error) {
	user, ok := r.usersByEmail[email]
	if !ok {
		return nil, ErrUserNotFound
	}
	cp := *user
	return &cp, nil
}

func (r *memoryRepository) FindUserByID(_ context.Context, id string) (*User, error) {
	user, ok := r.usersByID[id]
	if !ok {
		return nil, ErrUserNotFound
	}
	cp := *user
	return &cp, nil
}

func (r *memoryRepository) UpdateLastLogin(_ context.Context, id string, when time.Time) error {
	user, ok := r.usersByID[id]
	if !ok {
		return ErrUserNotFound
	}
	user.LastLoginAt = &when
	return nil
}

func (r *memoryRepository) SaveRefreshToken(_ context.Context, token *RefreshToken) error {
	cp := *token
	r.refreshByID[token.ID] = &cp
	r.refreshByHash[token.TokenHash] = &cp
	return nil
}

func (r *memoryRepository) FindRefreshTokenByHash(_ context.Context, tokenHash string) (*RefreshToken, error) {
	token, ok := r.refreshByHash[tokenHash]
	if !ok {
		return nil, ErrRefreshTokenNotFound
	}
	cp := *token
	return &cp, nil
}

func (r *memoryRepository) RevokeRefreshToken(_ context.Context, id string, replacedBy string, revokedAt time.Time) error {
	token, ok := r.refreshByID[id]
	if !ok {
		return ErrRefreshTokenNotFound
	}
	token.RevokedAt = &revokedAt
	token.ReplacedByTokenID = replacedBy
	if byHash, ok := r.refreshByHash[token.TokenHash]; ok {
		byHash.RevokedAt = &revokedAt
		byHash.ReplacedByTokenID = replacedBy
	}
	return nil
}

func (r *memoryRepository) UpsertDevice(_ context.Context, device *Device) error {
	if strings.TrimSpace(device.ID) == "" {
		return nil
	}
	cp := *device
	r.devicesByID[device.ID] = &cp
	return nil
}
