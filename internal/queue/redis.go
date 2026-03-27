package queue

import (
	"context"

	"github.com/redis/go-redis/v9"
	"yolo-ave-mujica/internal/config"
)

func NewRedisClient(cfg config.Config) *redis.Client {
	return redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
}

func Ping(ctx context.Context, client *redis.Client) error {
	return client.Ping(ctx).Err()
}
