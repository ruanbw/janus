package janus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"janus/internal/bootstrap"
	"janus/internal/config"
	"janus/internal/db"
	"janus/internal/domain"
	"janus/internal/httpapi"
	"janus/internal/mailer"
	"janus/internal/store"
	"janus/pkg/event"
)

// Config 重新导出配置结构，方便外部工程配置
type Config = config.Config

// Metadata 重新导出通用的业务元数据 JSONB 字典类型
type Metadata = store.Metadata

// 上下文 Context Key 常量定义
const (
	CtxTenantIDKey = "janus.auth.tenant_id"
	CtxUserIDKey   = "janus.auth.user_id"
	CtxRoleKey     = "janus.auth.role"
)

// AuthContext 封装受保护路由当前的调用方身份
type AuthContext struct {
	TenantID int64  `json:"tenantId"`
	UserID   int64  `json:"userId"`
	Role     string `json:"role"`
}

// GetAuthContext 从 Gin 上下文中提取当前通过认证的租户与用户身份
func GetAuthContext(c *gin.Context) (*AuthContext, bool) {
	if c == nil {
		return nil, false
	}
	var ac AuthContext
	hasTenant := false

	if v, exists := c.Get(CtxTenantIDKey); exists {
		if id, ok := toInt64(v); ok && id > 0 {
			ac.TenantID = id
			hasTenant = true
		}
	}
	if v, exists := c.Get(CtxUserIDKey); exists {
		if id, ok := toInt64(v); ok && id > 0 {
			ac.UserID = id
		}
	}
	if v, exists := c.Get(CtxRoleKey); exists {
		if r, ok := v.(string); ok && r != "" {
			ac.Role = r
		}
	}

	if !hasTenant {
		if v, exists := c.Get("auth.tenant"); exists {
			if t, ok := v.(*store.Tenant); ok && t != nil {
				ac.TenantID = t.ID
				ac.UserID = t.ID
				hasTenant = true
			}
		}
		if v, exists := c.Get("auth.role"); exists {
			if r, ok := v.(string); ok && r != "" {
				ac.Role = r
			}
		}
	}

	if !hasTenant {
		return nil, false
	}
	if ac.UserID == 0 {
		ac.UserID = ac.TenantID
	}
	if ac.Role == "" {
		ac.Role = "tenant"
	}
	return &ac, true
}

func toInt64(v any) (int64, bool) {
	switch val := v.(type) {
	case int64:
		return val, true
	case int:
		return int64(val), true
	case float64:
		return int64(val), true
	case string:
		n, err := strconv.ParseInt(val, 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}

// Option 定义 App 配置选项函数
type Option func(*options) error

type options struct {
	cfg                   *Config
	configPath            string
	customDB              *gorm.DB
	routeInjects          []func(engine *gin.Engine)
	protectedRouteInjects []func(rg *gin.RouterGroup)
	extraFS               fs.FS
}

// WithConfig 显式传入预先加载或定制的配置结构体
func WithConfig(cfg Config) Option {
	return func(o *options) error {
		o.cfg = &cfg
		return nil
	}
}

// WithConfigFile 指定配置文件路径（支持 .env 或环境变量文件）
func WithConfigFile(path string) Option {
	return func(o *options) error {
		if path != "" {
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("config file not found: %w", err)
			}
		}
		o.configPath = path
		return nil
	}
}

// WithDB 允许外部注入已建立好的数据库连接（如测试库、SQLite 或统一连接池）
func WithDB(gdb *gorm.DB) Option {
	return func(o *options) error {
		o.customDB = gdb
		return nil
	}
}

// WithRoutes 注入自定义路由，允许外部挂载私有端点或中间件
func WithRoutes(fn func(engine *gin.Engine)) Option {
	return func(o *options) error {
		if fn != nil {
			o.routeInjects = append(o.routeInjects, fn)
		}
		return nil
	}
}

// WithProtectedRoutes 注入受保护的控制面路由组，外部 Handler 自动继承基座的多租户认证与 Casbin RBAC 鉴权
func WithProtectedRoutes(fn func(rg *gin.RouterGroup)) Option {
	return func(o *options) error {
		if fn != nil {
			o.protectedRouteInjects = append(o.protectedRouteInjects, fn)
		}
		return nil
	}
}

// WithExtraMigrations 注入外部工程的专有迁移脚本（支持 embed.FS、os.DirFS 或 MapFS）
func WithExtraMigrations(fsys fs.FS) Option {
	return func(o *options) error {
		o.extraFS = fsys
		return nil
	}
}

// WithExtraMigrationsDir 注入外部工程的文件目录路径迁移脚本（便捷封装）
func WithExtraMigrationsDir(dir string) Option {
	return func(o *options) error {
		if dir != "" {
			if _, err := os.Stat(dir); err != nil {
				return fmt.Errorf("extra migrations dir not found: %w", err)
			}
			o.extraFS = os.DirFS(dir)
		}
		return nil
	}
}

// WithEventListener 注册领域事件监听器
func WithEventListener(topic string, handler event.Handler) Option {
	return func(o *options) error {
		event.Subscribe(topic, handler)
		return nil
	}
}

// Event 重新导出领域事件公开接口
type Event = event.Event

// SimpleEvent 基础领域事件实现
type SimpleEvent struct {
	topic   string
	payload any
	at      time.Time
}

func (s SimpleEvent) Topic() string         { return s.topic }
func (s SimpleEvent) Payload() any          { return s.payload }
func (s SimpleEvent) OccurredAt() time.Time { return s.at }

// NewSimpleEvent 构造基础领域事件
func NewSimpleEvent(topic string, payload any) SimpleEvent {
	return SimpleEvent{
		topic:   topic,
		payload: payload,
		at:      time.Now(),
	}
}

// PublishEvent 便捷派发领域事件
func PublishEvent(ctx context.Context, e Event) {
	event.Publish(ctx, e)
}

// App 是 Janus 运行时基座实例
type App struct {
	cfg                   Config
	gdb                   *gorm.DB
	store                 *store.Store
	server                *http.Server
	engine                *gin.Engine
	routeInjects          []func(engine *gin.Engine)
	protectedRouteInjects []func(rg *gin.RouterGroup)
	extraFS               fs.FS
	workerDone            chan struct{}
	cancelRun             context.CancelFunc
	mu                    sync.Mutex
	running               bool
}

// New 基于给定的选项构建并校验 App 实例
func New(opts ...Option) (*App, error) {
	var opt options
	for _, fn := range opts {
		if err := fn(&opt); err != nil {
			return nil, err
		}
	}

	var cfg Config
	if opt.cfg != nil {
		cfg = *opt.cfg
	} else {
		var err error
		cfg, err = config.Load()
		if err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
	}

	// 校验配置合规性
	if err := validateAppConfig(cfg); err != nil {
		return nil, err
	}

	app := &App{
		cfg:                   cfg,
		gdb:                   opt.customDB,
		routeInjects:          opt.routeInjects,
		protectedRouteInjects: opt.protectedRouteInjects,
		extraFS:               opt.extraFS,
	}

	if app.gdb != nil {
		app.store = store.New(app.gdb)
	}

	// 初始化 HTTP Handler 引擎
	if err := app.initHandler(); err != nil {
		return nil, err
	}

	return app, nil
}

// validateAppConfig 校验配置有效性（包含端口和基本格式）
func validateAppConfig(cfg Config) error {
	// 如果配置了 Addr，校验其 host:port 格式及端口范围
	if cfg.Addr != "" {
		host, portStr, err := net.SplitHostPort(cfg.Addr)
		if err != nil {
			return fmt.Errorf("invalid addr %q: %w", cfg.Addr, err)
		}
		_ = host
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 0 || port > 65535 {
			return fmt.Errorf("invalid port in addr %q", cfg.Addr)
		}
	}

	// 执行基座内部声明式校验（时长、范围等，忽略空零值）
	if cfg.SessionTTL > 0 || cfg.LandingMaxFiles > 0 {
		if err := cfg.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// initHandler 初始化底层 HTTP 路由
func (a *App) initHandler() error {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.RedirectTrailingSlash = false

	// 内置标准健康检查
	engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 若提供了 Store 则组装完整业务路由，否则保持轻量基础路由（支持无 DB 测试与自定义路由注入）
	if a.store != nil {
		var smtpCfg *mailer.SMTPConfig
		if a.cfg.SMTPHost != "" {
			smtpCfg = &mailer.SMTPConfig{
				Host:     a.cfg.SMTPHost,
				Port:     a.cfg.SMTPPort,
				Username: a.cfg.SMTPUsername,
				Password: a.cfg.SMTPPassword,
				From:     a.cfg.SMTPFrom,
			}
		}
		m := mailer.NewMailer(mailer.Config{
			BaseURL: a.cfg.PublicBaseURL,
			SMTP:    smtpCfg,
		}, io.Discard)

		fullHandler := httpapi.New(httpapi.Deps{
			Store:           a.store,
			Mailer:          m,
			Cfg:             a.cfg,
			ProtectedRoutes: a.protectedRouteInjects,
		})
		engine.NoRoute(func(c *gin.Context) {
			fullHandler.ServeHTTP(c.Writer, c.Request)
		})
	} else if len(a.protectedRouteInjects) > 0 {
		prot := engine.Group("/api", func(c *gin.Context) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		})
		for _, inject := range a.protectedRouteInjects {
			inject(prot)
		}
	}

	// 注入外部自定义路由
	for _, inject := range a.routeInjects {
		inject(engine)
	}

	a.engine = engine
	return nil
}

// Handler 暴露底层的 http.Handler（供单元测试、边缘测试或自定义编排）
func (a *App) Handler() http.Handler {
	return a.engine
}

// Run 启动应用（包括数据库连接、Worker 任务与 HTTP 服务监听），该调用是阻塞式的
func (a *App) Run(ctx context.Context) error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return errors.New("app is already running")
	}
	a.running = true

	runCtx, cancel := context.WithCancel(ctx)
	a.cancelRun = cancel
	a.workerDone = make(chan struct{})

	// 若尚未注入 DB，且配置了有效的 DatabaseURL 则尝试连接（非强制：连接失败由外层捕获）
	if a.gdb == nil && a.cfg.DatabaseURL != "" && strings.HasPrefix(a.cfg.DatabaseURL, "postgres://") {
		pool, err := db.Connect(runCtx, a.cfg.DatabaseURL)
		if err == nil {
			if a.cfg.MigrationsDir != "" || a.extraFS != nil {
				_ = db.MigrateWithExtra(runCtx, pool, a.cfg.MigrationsDir, a.extraFS)
			}
			pool.Close()
		}

		gdb, err := db.OpenGORM(a.cfg.DatabaseURL)
		if err == nil {
			a.gdb = gdb
			a.store = store.New(gdb)
			if a.cfg.SuperadminEmail != "" {
				_ = bootstrap.Superadmin(runCtx, a.store, a.cfg.SuperadminEmail)
			}
			// 组装底层完整路由
			_ = a.initHandler()
		}
	} else if a.gdb != nil && a.extraFS != nil {
		if sqlDB, err := a.gdb.DB(); err == nil {
			_ = db.MigrateDBWithExtra(runCtx, sqlDB, "", a.extraFS)
		}
	}

	// 启动后台 worker
	if a.store != nil {
		go func() {
			defer close(a.workerDone)
			domain.NewWorker(a.store, a.cfg).Run(runCtx)
		}()
	} else {
		close(a.workerDone)
	}

	addr := a.cfg.Addr
	if addr == "" {
		addr = ":8080"
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           a.engine,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	a.server = srv
	a.mu.Unlock()

	serverErrCh := make(chan error, 1)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		cancel()
		return err
	}

	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
		} else {
			serverErrCh <- nil
		}
	}()

	select {
	case <-runCtx.Done():
		shutdownCtx, sCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer sCancel()
		_ = a.Shutdown(shutdownCtx)
		return runCtx.Err()
	case err := <-serverErrCh:
		return err
	}
}

// Shutdown 优雅关闭服务
func (a *App) Shutdown(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cancelRun != nil {
		a.cancelRun()
	}

	var srvErr error
	if a.server != nil {
		srvErr = a.server.Shutdown(ctx)
		a.server = nil
	}

	if a.workerDone != nil {
		select {
		case <-a.workerDone:
		case <-ctx.Done():
		}
	}

	a.running = false
	return srvErr
}

// mutate4go-manifest-begin
// {"version":1,"tested_at":"2026-10-06T03:57:37+08:00","module_hash":"340213c02814e0eca7d0fae98f01604c45bc91917551435b39931c8ff730bbe0","functions":[{"id":"func/WithConfig","name":"WithConfig","line":42,"end_line":47,"hash":"b106c8c7b707e97fb1509bda36d7d3d8c6b25d7de30af47502ec83a9d2b3020d"},{"id":"func/WithConfigFile","name":"WithConfigFile","line":50,"end_line":60,"hash":"a14a2851c83a8e91072f6dfd591e1a281edf12c2bd9c75b95aa548f67766b4d9"},{"id":"func/WithDB","name":"WithDB","line":63,"end_line":68,"hash":"6b83114b2223d90bd2e439e3be22834d86af6d3591378fc7aff2ede30ab3470e"},{"id":"func/WithRoutes","name":"WithRoutes","line":71,"end_line":78,"hash":"21ef0f977e91769dafc799ed601bafe0252f0e48f443f429d20cdd36a2af0191"},{"id":"func/New","name":"New","line":95,"end_line":135,"hash":"42d37307e05362150f7b6eb7dcdd4faff6b83e8b0573976456978dae11a602d0"},{"id":"func/validateAppConfig","name":"validateAppConfig","line":138,"end_line":159,"hash":"dd8e87bbd6f0b9798f30efb9cf1ffff158dd46c35ea45178e7cfec2b3ddd1cfc"},{"id":"func/App.initHandler","name":"App.initHandler","line":162,"end_line":206,"hash":"ae96bde8e058bb4ea3105df0c5afea63873e6caae8e300c80056b56187bd9548"},{"id":"func/App.Handler","name":"App.Handler","line":209,"end_line":211,"hash":"c585e630f00b671d3b81479b9d33ac2a9901d086aef3417046a9848a75e3be14"},{"id":"func/App.Run","name":"App.Run","line":214,"end_line":298,"hash":"bd0a49c28fe8ebd055087416a7274930006069ca9fc31e5a9a58169b48df02ba"},{"id":"func/App.Shutdown","name":"App.Shutdown","line":301,"end_line":324,"hash":"6635f11f6ccb010bdb5f81a4b84cc18486513550eeae96971ea0fc01af410cd3"}]}
// mutate4go-manifest-end
