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

// 连接池参数集中在这里:调优只改一处,db_test.go 直接对常量与生效值断言。
const (
	// gormMaxOpenConns / gormMaxIdleConns —— GORM 是应用侧唯一数据路径,必须留出余量:
	// worker 的 3 个 guardedLoop(dns-retry / cert-probe / ownership-recheck)每轮先抢
	// 会话级咨询锁,TryAdvisoryLock 会**独占一条连接直到 release**(单轮最坏可跑几十分钟,
	// 见审查报告 worker 并发项),3 条连接被长期占住。原先 MaxOpenConns=10、
	// 默认 MaxIdleConns=2 的组合下:高峰要反复建连/握手,而长事务与咨询锁持有者
	// 又容易把并发 HTTP 查询挤到等连接。提到 20 并把 idle 对齐 ——
	// 空闲连接数不足是"反复握手"的直接原因,浪费的连接在池里闲着没有代价。
	gormMaxOpenConns = 20
	gormMaxIdleConns = 20
	// gormConnMaxLifetime:连接总寿命上限。NAT/云 LB 常在静默超时后单方面切断
	// TCP,池里那条"看起来还活着"的连接在下一次复用时(往往正好在事务中间)
	// 报 unexpected EOF。主动在 30 分钟内回收重建,远短于常见 NAT 超时。
	gormConnMaxLifetime = 30 * time.Minute
	// gormConnMaxIdleTime:空闲连接最多留 10 分钟,避免池里长期挂着已死的连接。
	gormConnMaxIdleTime = 10 * time.Minute

	// pgxpool(迁移与测试用的原生池,不承载业务查询)参数。
	pgxMaxConns = 10
	// pgxMinConns:预热少量连接。原先 MinConns=0,首个请求要现连(含 TLS/认证),
	// 冷启动那次迁移/探测会明显偏慢。
	pgxMinConns = 2
	// pgxMaxConnLifetime / pgxMaxConnIdleTime:同 GORM 侧的理由(对端静默切断)。
	pgxMaxConnLifetime = 30 * time.Minute
	pgxMaxConnIdleTime = 10 * time.Minute
	// pgxHealthCheckPeriod:后台定期探活并补齐 MinConns。
	pgxHealthCheckPeriod = time.Minute
)

// pgxPoolConfig 构造 pgxpool 配置(连接参数唯一出口,供 Connect 与测试共用)。
func pgxPoolConfig(url string) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = pgxMaxConns
	cfg.MinConns = pgxMinConns
	cfg.MaxConnLifetime = pgxMaxConnLifetime
	cfg.MaxConnIdleTime = pgxMaxConnIdleTime
	cfg.HealthCheckPeriod = pgxHealthCheckPeriod
	return cfg, nil
}

// Connect 建立连接池并等待数据库就绪(最多约 30 秒)。
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxPoolConfig(url)
	if err != nil {
		return nil, err
	}
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

// OpenGORM 建立 GORM ORM 连接(数据访问层使用;迁移与测试原生操作仍走 pgxpool)。
func OpenGORM(url string) (*gorm.DB, error) {
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
	sqlDB.SetMaxOpenConns(gormMaxOpenConns)
	// 默认 MaxIdleConns=2:高峰每次都要重新建连(含 TLS/认证),而且池里
	// 只留着 2 条"热"连接,并发一上来就是反复握手。
	sqlDB.SetMaxIdleConns(gormMaxIdleConns)
	// 0(默认)= 永不过期,连接被 NAT/LB 静默切断后仍会被复用并在事务中报错。
	sqlDB.SetConnMaxLifetime(gormConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(gormConnMaxIdleTime)
	return gdb, nil
}
