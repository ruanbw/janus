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
	"janus/internal/mailer"
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
		log.Fatalf("migrate: %v", err)
	}
	// 迁移完成即关闭:业务数据访问全部走 GORM,这个 pgxpool(最多 10 条连接)
	// 此后没有任何使用者,留到进程退出等于白占 10 个连接。
	pool.Close()

	gdb, err := db.OpenGORMWithPool(cfg.DatabaseURL, cfg.DBMaxOpenConns)
	if err != nil {
		log.Fatalf("open gorm: %v", err)
	}
	st := store.New(gdb)

	if cfg.SuperadminEmail != "" {
		if err := bootstrap.Superadmin(ctx, st, cfg.SuperadminEmail); err != nil {
			log.Fatalf("init superadmin: %v", err)
		}
	}

	// 邮件:配置 SMTP 后启用真实发送(见 .env.example),否则控制台 mailer
	var smtpCfg *mailer.SMTPConfig
	if cfg.SMTPHost != "" {
		smtpCfg = &mailer.SMTPConfig{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			Username: cfg.SMTPUsername,
			Password: cfg.SMTPPassword,
			From:     cfg.SMTPFrom,
		}
	}
	m := mailer.NewMailer(mailer.Config{
		BaseURL: cfg.PublicBaseURL,
		SMTP:    smtpCfg,
	}, os.Stdout)

	// 访问明细异步队列:跳转请求只入队,批量落库在后台完成(见 httpapi/visit_queue.go)。
	// 必须在 srv.Shutdown 之后 Close,把已入队的明细排空,否则重启/发布会丢掉最后一批访问。
	visitQueue := httpapi.NewVisitQueue(st, httpapi.VisitQueueConfig{
		Size: cfg.VisitQueueSize, Workers: cfg.VisitQueueWorkers, BatchSize: cfg.VisitQueueBatch,
	})
	app := httpapi.New(httpapi.Deps{Store: st, Mailer: m, Cfg: cfg, VisitQueue: visitQueue})

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
	go func() {
		log.Printf("janus listening on %s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
	// HTTP 已停止接收请求,此后不会再有明细入队:排空队列(最多 5s)。
	drainCtx, drainCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := visitQueue.Close(drainCtx); err != nil {
		log.Printf("visit queue drain: %v", err)
	}
	drainCancel()
	if n := visitQueue.Dropped(); n > 0 {
		log.Printf("visit queue: 运行期间因队列满共丢弃 %d 条访问明细", n)
	}
	workerCancel()
	select {
	case <-workerDone:
	// worker 现在会等所有循环收口才返回,而循环里可能有进行中的出网调用
	// (证书探活 10s 超时),所以等待窗口要盖得住单次调用的上限。
	case <-time.After(12 * time.Second):
		log.Printf("worker did not stop within 12s")
	}
}
