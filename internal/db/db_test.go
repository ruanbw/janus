package db

// 连接池参数测试:池的行为无法用"跑一条查询"观察,直接把参数本身固化成断言,
// 避免调优参数在后续重构中被悄悄丢掉(默认值都是静默的:MaxIdleConns=2、
// ConnMaxLifetime=0(永不过期),丢掉了也不会报错,只在高负载/长连接被掐断时暴露)。

import (
	"context"
	"testing"
	"time"
)

// TestPGXPoolConfig 固化 pgxpool(迁移/测试用的原生池)侧的连接参数。
func TestPGXPoolConfig(t *testing.T) {
	// Given 一个连接串
	cfg, err := pgxPoolConfig("postgres://user:pw@localhost:5432/janus_test?sslmode=disable")
	if err != nil {
		t.Fatalf("pgxPoolConfig: %v", err)
	}

	// When/Then 各项都在
	if cfg.MaxConns != pgxMaxConns {
		t.Fatalf("MaxConns = %d, want %d", cfg.MaxConns, pgxMaxConns)
	}
	if cfg.MinConns != pgxMinConns {
		t.Fatalf("MinConns = %d, want %d", cfg.MinConns, pgxMinConns)
	}
	if cfg.MaxConnLifetime != pgxMaxConnLifetime {
		t.Fatalf("MaxConnLifetime = %v, want %v", cfg.MaxConnLifetime, pgxMaxConnLifetime)
	}
	if cfg.MaxConnIdleTime != pgxMaxConnIdleTime {
		t.Fatalf("MaxConnIdleTime = %v, want %v", cfg.MaxConnIdleTime, pgxMaxConnIdleTime)
	}
	if cfg.HealthCheckPeriod != pgxHealthCheckPeriod {
		t.Fatalf("HealthCheckPeriod = %v, want %v", cfg.HealthCheckPeriod, pgxHealthCheckPeriod)
	}
	if cfg.MaxConnLifetime <= 0 || cfg.MaxConnIdleTime <= 0 || cfg.HealthCheckPeriod <= 0 {
		t.Fatal("pgxpool 生命周期/健康检查参数必须为正")
	}
}

// TestConnectAbortsRetryLoopOnContextCancel 固化 Connect 重试循环的 ctx 响应。
//
// 契约:ctx 取消后 Connect 必须在很短时间内返回错误,而不是把剩余的重试预算
// 睡完(30 次 × 1s 间隔 ≈ 30s,最坏 90s)。启动路径上取消通常意味着进程正在退出,
// 调用方等的是"尽快收口",不是"再等 30 秒"。
//
// 用法说明:目标端口不可达(pgxpool.NewWithConfig 在后台预热连接,不会因此失败),
// 因此必然进入重试循环;再在循环中途取消 ctx 观察返回耗时。
func TestConnectAbortsRetryLoopOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()

	// When 对一个不可达的库调用 Connect,并在重试途中取消 ctx
	start := time.Now()
	_, err := Connect(ctx, "postgres://janus:janus@127.0.0.1:1/janus_nope?sslmode=disable")
	elapsed := time.Since(start)

	// Then 返回错误(不再尝试)且很快返回
	if err == nil {
		t.Fatal("对不可达的库 Connect 却返回了 nil error")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("ctx 取消后 Connect 仍耗时 %v 才返回:重试循环的等待没有响应 ctx", elapsed)
	}
}
