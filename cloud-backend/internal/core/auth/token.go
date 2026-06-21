package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type TokenConfig struct {
	Secret          string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

type TokenIssuer struct {
	cfg TokenConfig
}

func NewTokenIssuer(cfg TokenConfig) *TokenIssuer {
	if cfg.Secret == "" {
		cfg.Secret = "development-only-change-me"
	}
	if cfg.AccessTokenTTL == 0 {
		cfg.AccessTokenTTL = time.Hour
	}
	if cfg.RefreshTokenTTL == 0 {
		cfg.RefreshTokenTTL = 30 * 24 * time.Hour
	}
	return &TokenIssuer{cfg: cfg}
}

func (i *TokenIssuer) AccessTokenTTL() time.Duration {
	return i.cfg.AccessTokenTTL
}

func (i *TokenIssuer) RefreshTokenTTL() time.Duration {
	return i.cfg.RefreshTokenTTL
}

func (i *TokenIssuer) IssueAccessToken(user User) (string, time.Time, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(i.cfg.AccessTokenTTL)
	claims := AccessClaims{
		UserID:    user.ID,
		Email:     user.Email,
		Nickname:  user.Nickname,
		TokenType: TokenTypeAccess,
		IssuedAt:  now.Unix(),
		ExpiresAt: expiresAt.Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	sig := i.sign(encodedPayload)
	return encodedPayload + "." + sig, expiresAt, nil
}

func (i *TokenIssuer) ValidateAccessToken(token string) (*AccessClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, ErrInvalidToken
	}
	expected := i.sign(parts[0])
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return nil, ErrInvalidToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var claims AccessClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrInvalidToken
	}
	if claims.TokenType != TokenTypeAccess || claims.UserID == "" {
		return nil, ErrInvalidToken
	}
	if time.Now().UTC().Unix() >= claims.ExpiresAt {
		return nil, ErrInvalidToken
	}
	return &claims, nil
}

func (i *TokenIssuer) NewRefreshToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if token == "" {
		return "", errors.New("failed to generate refresh token")
	}
	return token, nil
}

func (i *TokenIssuer) sign(payload string) string {
	mac := hmac.New(sha256.New, []byte(i.cfg.Secret))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
