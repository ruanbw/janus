// Janus 后端入口:加载配置 → 连接 Postgres → 迁移 → 初始化超管 → 启动后台任务 → HTTP 服务。
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"janus/internal/bootstrap"
	"janus/internal/config"
	"janus/internal/db"
	"janus/internal/domain"
	"janus/internal/httpapi"
	"janus/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid config: %v", err)
	}
	ctx := context.Background()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}

	if err := db.Migrate(ctx, pool, cfg.MigrationsDir); err != nil {
		pool.Close()
		log.Fatalf("init schema: %v", err)
	}
	// schema 初始化完成即关闭:业务数据访问全部走 GORM,这个 pgxpool(最多 10 条连接)
	// 此后没有任何使用者,留到进程退出等于白占 10 个连接。
	pool.Close()

	gdb, err := db.OpenGORM(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open gorm: %v", err)
	}
	st := store.New(gdb)

	// 超管初始化:bootstrap 确保存在 username=superadmin 的超管账号(种子口令 + 首登强制改密)。
	// 不再是条件初始化 —— 登录需要 username,没有环境变量可以跳过这一步。
	if err := bootstrap.Superadmin(ctx, st); err != nil {
		// 刻意硬失败(不降级为「跳过超管初始化 + 告警」):该 username 被普通租户占用,
		// 意味着平台自留的超管账号被抢注 —— 继续启动等于让平台以「没有超管」
		// 的状态对外服务,而这比启动失败更难被发现。运维清理占用后重启即可。
		// 拒绝提权的理由与幂等收敛见 internal/bootstrap/convergeOwnTenant。
		log.Fatalf("init superadmin: %v", err)
	}

	app := httpapi.New(httpapi.Deps{Store: st, Cfg: cfg})

	// 后台任务:DNS 重试、证书预签发探活、访问/会话清理。
	// Shutdown 后显式 cancel 并等待 worker 退出,避免进程退出时后台任务仍在写库。
	workerCtx, workerCancel := context.WithCancel(ctx)
	defer workerCancel()
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		domain.NewWorker(st, cfg).Run(workerCtx)
	}()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	// HTTP 服务异常退出不能走 log.Fatalf:那会跳过 worker 收口与优雅关闭。
	// 统一经 error channel 汇入退出路径,对齐 pkg/janus 的 serverErrCh 模式。
	serverErrCh := make(chan error, 1)
	go func() {
		log.Printf("janus listening on %s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
			return
		}
		serverErrCh <- nil
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	fatalErr := waitForShutdown(stop, serverErrCh)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
	workerCancel()
	select {
	case <-workerDone:
	// worker 现在会等所有循环收口才返回,而循环里可能有进行中的出网调用
	// (证书探活 10s 超时),所以等待窗口要盖得住单次调用的上限。
	case <-time.After(12 * time.Second):
		log.Printf("worker did not stop within 12s")
	}

	if fatalErr != nil {
		log.Printf("exit due to server error: %v", fatalErr)
		os.Exit(1)
	}
}

// waitForShutdown 阻塞直到收到退出信号或 HTTP 服务异常退出。
// 收到信号返回 nil;服务异常退出返回其错误(nil 表示服务正常关闭)。
func waitForShutdown(stop <-chan os.Signal, serverErrCh <-chan error) error {
	select {
	case <-stop:
		return nil
	case err := <-serverErrCh:
		return err
	}
}
