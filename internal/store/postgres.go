package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"yolo-ave-mujica/internal/config"
)

func NewPostgresPool(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	return pgxpool.New(ctx, cfg.DatabaseURL)
}
