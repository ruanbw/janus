package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// InitScriptName 内置初始化脚本的文件名。核心 schema 只有这一份事实来源:
// 没有版本表、没有增量迁移,改表直接改脚本并重建开发库(见 AGENTS.md)。
const InitScriptName = "init.sql"

// ExtraMigrationsTableName 外部专有迁移的版本表名，与核心 schema 的"无版本表"设计隔离。
const ExtraMigrationsTableName = "goose_db_version_extra"

// Migrate 应用内置初始化脚本(migrations/init.sql)。
func Migrate(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	return MigrateWithExtra(ctx, pool, dir, nil)
}

// MigrateExtra 执行外部专有迁移，使用独立的 goose_db_version_extra 表记录版本。
func MigrateExtra(ctx context.Context, pool *pgxpool.Pool, extraFS fs.FS) error {
	return MigrateWithExtra(ctx, pool, "", extraFS)
}

// MigrateWithExtra 统一在分布式锁下执行内置初始化脚本与外部专有迁移（双轨流水线）。
func MigrateWithExtra(ctx context.Context, pool *pgxpool.Pool, dir string, extraFS fs.FS) error {
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()

	return MigrateDBWithExtra(ctx, db, dir, extraFS)
}

// MigrateDBWithExtra 在已有 *sql.DB 实例上执行双轨初始化(先内置脚本，后外部迁移)。
func MigrateDBWithExtra(ctx context.Context, db *sql.DB, dir string, extraFS fs.FS) error {
	// 并发多副本同时启动时,两个进程可能同时执行 DDL。
	// 用 advisory lock 串行化;锁挂在一条专用连接上(会话级锁),
	// 期间这条连接不释放,其它副本阻塞等到锁释放后再执行幂等脚本。
	lockConn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer func() { _ = lockConn.Close() }()
	if _, err := lockConn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtext('janus-migrate'))`); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		_, _ = lockConn.ExecContext(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock(hashtext('janus-migrate'))`)
	}()

	// 第一轨：内置初始化脚本
	if dir != "" {
		if err := applyInitScript(ctx, db, dir); err != nil {
			return err
		}
	}

	// 第二轨：外部专有迁移(外部表需要增量演进,故保留 goose 版本表)
	if extraFS != nil {
		if err := applyExtraMigrations(ctx, db, extraFS); err != nil {
			return err
		}
	}

	return nil
}

// gooseMu 串行化对 goose 包级全局状态的读写(方言名/版本表名/BaseFS)。
//
// 为什么需要:goose.SetTableName/SetBaseFS 直接写包级变量,goose 自身不加锁。
// 进程内同时存在多个迁移入口时(SDK 多实例、内置初始化与外部专有迁移并存),
// 两个 goroutine 会互相覆盖版本表名与 BaseFS —— 迁移被记到错误的版本表,
// 或读到另一个 FS 的文件,表现为随机 "file does not exist" 与重复执行。
// 数据库侧的 advisory lock 只解决跨进程互斥,挡不住同进程内的并发。
var gooseMu sync.Mutex

// applyExtraMigrations 在互斥保护下执行 goose 外部迁移,并保证全局状态被还原。
func applyExtraMigrations(ctx context.Context, db *sql.DB, extraFS fs.FS) error {
	gooseMu.Lock()
	defer gooseMu.Unlock()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose set dialect: %w", err)
	}
	origTable := goose.TableName()
	// defer 还原:UpContext 中途返回/panic 也不能把全局状态留在别人的设置上。
	defer goose.SetTableName(origTable)
	goose.SetTableName(ExtraMigrationsTableName)
	defer goose.SetBaseFS(nil)
	goose.SetBaseFS(extraFS)

	err := goose.UpContext(ctx, db, ".")
	if err != nil && !errors.Is(err, goose.ErrNoMigrations) && !errors.Is(err, goose.ErrNoMigrationFiles) {
		return fmt.Errorf("goose extra up: %w", err)
	}
	return nil
}

// applyInitScript 读取并整段执行 dir/init.sql。
//
// 无参数 Exec 走 Postgres 简单协议:整段脚本在一个隐式事务里执行,
// 任何一句失败都整体回滚,不会留下半截 schema;脚本本身必须幂等(见文件头契约)。
func applyInitScript(ctx context.Context, db *sql.DB, dir string) error {
	path := filepath.Join(dir, InitScriptName)
	script, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read init script: %w", err)
	}
	if _, err := db.ExecContext(ctx, string(script)); err != nil {
		return fmt.Errorf("apply %s: %w", path, err)
	}
	return nil
}
