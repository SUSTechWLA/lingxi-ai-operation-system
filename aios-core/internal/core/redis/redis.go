package redis

import (
	"context"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/config"
)

func NewClient(cfg config.RedisConfig) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr(),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		zap.L().Fatal("Failed to connect to Redis", zap.Error(err))
	}

	zap.L().Info("Redis connected", zap.String("addr", cfg.Addr()))
	return client
}
