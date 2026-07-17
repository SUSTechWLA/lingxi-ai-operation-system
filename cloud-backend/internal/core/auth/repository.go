package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	CreateUser(ctx context.Context, user *User) error
	FindUserByEmail(ctx context.Context, email string) (*User, error)
	FindUserByID(ctx context.Context, id string) (*User, error)
	UpdateLastLogin(ctx context.Context, id string, when time.Time) error
	SaveRefreshToken(ctx context.Context, token *RefreshToken) error
	FindRefreshTokenByHash(ctx context.Context, tokenHash string) (*RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, id string, replacedBy string, revokedAt time.Time) error
	UpsertDevice(ctx context.Context, device *Device) error
}

type PGRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{pool: pool}
}

func (r *PGRepository) CreateUser(ctx context.Context, user *User) error {
	if user.ID == "" {
		user.ID = "u_" + uuid.NewString()
	}
	now := time.Now().UTC()
	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	if user.UpdatedAt.IsZero() {
		user.UpdatedAt = now
	}
	if user.Status == "" {
		user.Status = UserStatusActive
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, nickname, avatar_url, status, created_at, updated_at, last_login_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		user.ID, user.Email, user.PasswordHash, user.Nickname, user.AvatarURL, string(user.Status),
		user.CreatedAt, user.UpdatedAt, user.LastLoginAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrEmailAlreadyRegistered
		}
		return err
	}
	return nil
}

func (r *PGRepository) FindUserByEmail(ctx context.Context, email string) (*User, error) {
	return r.findUser(ctx, `WHERE email=$1`, email)
}

func (r *PGRepository) FindUserByID(ctx context.Context, id string) (*User, error) {
	return r.findUser(ctx, `WHERE id=$1`, id)
}

func (r *PGRepository) findUser(ctx context.Context, where string, arg string) (*User, error) {
	var user User
	var status string
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, password_hash, COALESCE(nickname, ''), COALESCE(avatar_url, ''), status,
		        created_at, updated_at, last_login_at
		   FROM users `+where,
		arg,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Nickname, &user.AvatarURL, &status,
		&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	user.Status = UserStatus(status)
	return &user, nil
}

func (r *PGRepository) UpdateLastLogin(ctx context.Context, id string, when time.Time) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET last_login_at=$2, updated_at=$2 WHERE id=$1`, id, when)
	return err
}

func (r *PGRepository) SaveRefreshToken(ctx context.Context, token *RefreshToken) error {
	if token.ID == "" {
		token.ID = "rt_" + uuid.NewString()
	}
	now := time.Now().UTC()
	if token.CreatedAt.IsZero() {
		token.CreatedAt = now
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, device_id, user_agent, ip_address,
		 expires_at, revoked_at, replaced_by_token_id, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		token.ID, token.UserID, token.TokenHash, token.DeviceID, token.UserAgent, token.IPAddress,
		token.ExpiresAt, token.RevokedAt, token.ReplacedByTokenID, token.CreatedAt,
	)
	return err
}

func (r *PGRepository) FindRefreshTokenByHash(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	var token RefreshToken
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, token_hash, COALESCE(device_id, ''), COALESCE(user_agent, ''), COALESCE(ip_address, ''),
		        expires_at, revoked_at, COALESCE(replaced_by_token_id, ''), created_at
		   FROM refresh_tokens WHERE token_hash=$1`,
		tokenHash,
	).Scan(&token.ID, &token.UserID, &token.TokenHash, &token.DeviceID, &token.UserAgent, &token.IPAddress,
		&token.ExpiresAt, &token.RevokedAt, &token.ReplacedByTokenID, &token.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRefreshTokenNotFound
	}
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func (r *PGRepository) RevokeRefreshToken(ctx context.Context, id string, replacedBy string, revokedAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at=$2, replaced_by_token_id=$3 WHERE id=$1 AND revoked_at IS NULL`,
		id, revokedAt, replacedBy,
	)
	return err
}

func (r *PGRepository) UpsertDevice(ctx context.Context, device *Device) error {
	if device.ID == "" || device.UserID == "" {
		return nil
	}
	now := time.Now().UTC()
	if device.CreatedAt.IsZero() {
		device.CreatedAt = now
	}
	if device.UpdatedAt.IsZero() {
		device.UpdatedAt = now
	}
	if device.LastSeenAt.IsZero() {
		device.LastSeenAt = now
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO devices (id, user_id, device_name, device_type, platform, last_seen_at, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 ON CONFLICT (id) DO UPDATE SET user_id=$2, device_name=$3, device_type=$4, platform=$5,
		 last_seen_at=$6, updated_at=$8`,
		device.ID, device.UserID, device.DeviceName, device.DeviceType, device.Platform,
		device.LastSeenAt, device.CreatedAt, device.UpdatedAt,
	)
	return err
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate key")
}
