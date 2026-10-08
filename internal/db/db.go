// Package db 负责 Postgres 连接池与 SQL 迁移。
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
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

// DefaultMaxOpenConns 业务连接池默认最大连接数(JANUS_DB_MAX_OPEN_CONNS 未配置或为 0 时)。
//
// 原来写死 10:跳转热路径每次访问要 3~6 次 DB 往返,突发流量下 10 条连接很快排满,
// 排队时长直接叠加到访客的跳转延迟上。25 仍远低于 Postgres 默认 max_connections=100,
// 给迁移、后台 worker 与运维连接留足余量。
const DefaultMaxOpenConns = 25

// OpenGORM 建立 GORM ORM 连接(数据访问层使用;迁移与测试原生操作仍走 pgxpool)。
// 连接池大小取 DefaultMaxOpenConns;需要按配置调整时用 OpenGORMWithPool。
func OpenGORM(url string) (*gorm.DB, error) {
	return OpenGORMWithPool(url, DefaultMaxOpenConns)
}

// OpenGORMWithPool 同 OpenGORM,连接池上限由调用方指定(<=0 时取 DefaultMaxOpenConns)。
//
// 空闲连接上限与最大连接数取同一个值:database/sql 默认只保留 2 条空闲连接,
// 高峰期多出来的连接用完即关、下一个请求再重新握手,等于把建连成本摊进每次跳转。
func OpenGORMWithPool(url string, maxOpenConns int) (*gorm.DB, error) {
	if maxOpenConns <= 0 {
		maxOpenConns = DefaultMaxOpenConns
	}
	gdb, err := gorm.Open(postgres.Open(url), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open gorm: %w", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("gorm sql db: %w", err)
	}
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxOpenConns)
	// 空闲过久的连接回收掉:低峰期不白占 Postgres 连接槽,也避开中间件(NAT/LB)静默断链。
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	return gdb, nil
}
