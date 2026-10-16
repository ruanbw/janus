package db

// 连接池参数测试:池的行为无法用"跑一条查询"观察,直接把参数本身固化成断言,
// 避免调优参数在后续重构中被悄悄丢掉(默认值都是静默的:MaxIdleConns=2、
// ConnMaxLifetime=0(永不过期),丢掉了也不会报错,只在高负载/长连接被掐断时暴露)。

import (
	"testing"
)

// TestGORMPoolConfig 固化 GORM(*sql.DB)侧的连接池参数。
//
// 契约:MaxIdleConns 必须与 MaxOpenConns 对齐;必须设置 ConnMaxLifetime 与
// ConnMaxIdleTime(非 0=永不过期)。
func TestGORMPoolConfig(t *testing.T) {
	// Given 一个可用的数据库
	gdb, err := OpenGORM(testDatabaseURL())
	if err != nil {
		t.Skipf("test database not available (%v)", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatalf("gorm sql db: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	// When 读取池参数
	st := sqlDB.Stats()

	// Then MaxOpenConns 取常量值(不是驱动默认的 0=无上限)
	if st.MaxOpenConnections != gormMaxOpenConns {
		t.Fatalf("MaxOpenConnections = %d, want %d", st.MaxOpenConnections, gormMaxOpenConns)
	}
	// 空闲连接数必须与最大连接数同量级:默认 MaxIdleConns=2 会让高峰反复建连/握手
	if gormMaxIdleConns != gormMaxOpenConns {
		t.Fatalf("MaxIdleConns(%d) 与 MaxOpenConns(%d) 未对齐:高峰会反复建连/拆连",
			gormMaxIdleConns, gormMaxOpenConns)
	}
	// 连接必须有寿命上限:被 NAT/LB 静默切断的连接复用会在事务中间报 unexpected EOF
	if gormConnMaxLifetime <= 0 {
		t.Fatalf("ConnMaxLifetime = %v, want > 0(0 = 永不过期,连接被对端切断后仍会复用)", gormConnMaxLifetime)
	}
	if gormConnMaxIdleTime <= 0 {
		t.Fatalf("ConnMaxIdleTime = %v, want > 0", gormConnMaxIdleTime)
	}
	if gormConnMaxIdleTime > gormConnMaxLifetime {
		t.Fatalf("ConnMaxIdleTime(%v) > ConnMaxLifetime(%v):空闲回收应早于总寿命",
			gormConnMaxIdleTime, gormConnMaxLifetime)
	}
}

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
