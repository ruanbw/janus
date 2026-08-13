// Package db 负责 Postgres 连接池与 SQL 迁移。
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect 建立连接池并等待数据库就绪(最多约 30 秒)。
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = 10
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	var lastErr error
	for i := 0; i < 30; i++ {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err = pool.Ping(pingCtx)
		cancel()
		if err == nil {
			return pool, nil
		}
		lastErr = err
		time.Sleep(time.Second)
	}
	pool.Close()
	return nil, fmt.Errorf("database not reachable: %w", lastErr)
}
