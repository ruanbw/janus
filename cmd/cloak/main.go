// CLOAK 后端入口:加载配置 → 连接 Postgres → 迁移 → 初始化超管 → 启动后台任务 → HTTP 服务。
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

	"cloak/internal/bootstrap"
	"cloak/internal/config"
	"cloak/internal/db"
	"cloak/internal/domain"
	"cloak/internal/httpapi"
	"cloak/internal/mailer"
	"cloak/internal/store"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool, cfg.MigrationsDir); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	st := store.New(pool)

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

	app := httpapi.New(httpapi.Deps{Store: st, Mailer: m, Cfg: cfg})

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
		log.Printf("cloak listening on %s", cfg.Addr)
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
	workerCancel()
	select {
	case <-workerDone:
	case <-time.After(5 * time.Second):
		log.Printf("worker did not stop within 5s")
	}
}
