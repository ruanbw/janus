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
	"sync/atomic"
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

// WithProtectedRoutes 注入受保护的控制面路由组(挂在基座 /api 下),外部 Handler 依次经过:
//   - 多租户认证(会话 cookie 或 Bearer JWT;封禁、token_version 吊销同基座);
//   - Casbin RBAC 授权:superadmin 全通;tenant 仅放行扩展**实际注册**的 (方法, 路径)。
//     挂在 /api/admin/ 下、或首段为参数/通配(如 /api/:x、/api/*all)的扩展路由
//     不对租户放行,即超管专属;
//   - CSRF:会话 cookie 认证的非安全方法(POST/PUT/PATCH/DELETE 等)必须携带
//     与会话匹配的 X-CSRF-Token 头,Bearer 认证不受影响。
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
	fallback              atomic.Pointer[http.Handler]
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

// initHandler 初始化底层 HTTP 路由。
//
// 只在 New 中调用一次:外部 WithRoutes 回调只执行一次,不会因 Run 中补齐 DB 而被重复注册。
// 业务路由通过 NoRoute 委托给 a.fallback,Run 连上数据库后只需原子替换 fallback,
// 无需重建 engine(旧实现在 Run 里再次 initHandler,导致路由回调执行两遍)。
func (a *App) initHandler() error {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.RedirectTrailingSlash = false

	// 内置标准健康检查
	engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 外部自定义路由优先于基座业务路由(与旧实现一致:基座路由挂在 NoRoute 上)
	for _, inject := range a.routeInjects {
		inject(engine)
	}

	engine.NoRoute(func(c *gin.Context) {
		if h := a.fallback.Load(); h != nil {
			(*h).ServeHTTP(c.Writer, c.Request)
			return
		}
		// 已配置 DatabaseURL 但 Run 尚未完成数据库初始化(或已停止)
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "service not ready"})
	})
	a.engine = engine

	switch {
	case a.store != nil:
		// 若提供了 Store 则组装完整业务路由
		a.setFallback(a.fullHandler())
	case a.cfg.DatabaseURL == "":
		// 无 DB 模式:保持轻量基础路由(支持无 DB 测试与自定义路由注入)
		a.setFallback(a.noDBHandler())
	default:
		// 已配置 DatabaseURL:完整业务路由等 Run 连上数据库后再组装,
		// 受保护路由回调届时由 httpapi.New 调用一次。
	}
	return nil
}

func (a *App) setFallback(h http.Handler) {
	if h == nil {
		a.fallback.Store(nil)
		return
	}
	a.fallback.Store(&h)
}

// fullHandler 基于 a.store 组装完整业务路由
func (a *App) fullHandler() http.Handler {
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

	return httpapi.New(httpapi.Deps{
		Store:           a.store,
		Mailer:          m,
		Cfg:             a.cfg,
		ProtectedRoutes: a.protectedRouteInjects,
	})
}

// noDBHandler 无 DB 模式下的兜底路由:受保护路由一律 401,其余 404
func (a *App) noDBHandler() http.Handler {
	h := gin.New()
	h.RedirectTrailingSlash = false
	if len(a.protectedRouteInjects) > 0 {
		prot := h.Group("/api", func(c *gin.Context) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		})
		for _, inject := range a.protectedRouteInjects {
			inject(prot)
		}
	}
	return h
}

// Handler 暴露底层的 http.Handler（供单元测试、边缘测试或自定义编排）
func (a *App) Handler() http.Handler {
	return a.engine
}

// validateDatabaseURL 接受 postgres:// 与 postgresql:// 两种 URL scheme(pgx/libpq 均支持),
// 以及 libpq 的 key=value DSN;其它 scheme 直接报错,而不是像旧实现那样静默以无 DB 模式启动。
func validateDatabaseURL(u string) error {
	i := strings.Index(u, "://")
	if i < 0 {
		return nil // key=value DSN,交给 pgx 解析
	}
	switch strings.ToLower(u[:i]) {
	case "postgres", "postgresql":
		return nil
	default:
		return fmt.Errorf("unsupported database url scheme %q (want postgres:// or postgresql://)", u[:i])
	}
}

// setupDB 连接数据库、执行迁移并初始化超管。任何一步失败都返回错误,由 Run 终止启动。
// 返回值 owned 为本次 Run 自行打开的 GORM 连接(需在 Run 退出时关闭);WithDB 注入的连接不归 App 管。
func (a *App) setupDB(ctx context.Context) (owned *gorm.DB, err error) {
	needMigrate := a.cfg.MigrationsDir != "" || a.extraFS != nil

	if a.gdb != nil {
		// WithDB 注入:同样执行基座核心迁移(旧实现传 dir="",核心迁移从不执行)与外部迁移
		if !needMigrate {
			return nil, nil
		}
		sqlDB, err := a.gdb.DB()
		if err != nil {
			return nil, fmt.Errorf("get sql db: %w", err)
		}
		if err := db.MigrateDBWithExtra(ctx, sqlDB, a.cfg.MigrationsDir, a.extraFS); err != nil {
			return nil, fmt.Errorf("migrate: %w", err)
		}
		return nil, nil
	}

	if a.cfg.DatabaseURL == "" {
		return nil, nil
	}
	if err := validateDatabaseURL(a.cfg.DatabaseURL); err != nil {
		return nil, err
	}

	pool, err := db.Connect(ctx, a.cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
	}
	if needMigrate {
		err = db.MigrateWithExtra(ctx, pool, a.cfg.MigrationsDir, a.extraFS)
	}
	// 迁移完成即关闭:业务数据访问全部走 GORM
	pool.Close()
	if err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	gdb, err := db.OpenGORM(a.cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open gorm: %w", err)
	}
	st := store.New(gdb)
	if a.cfg.SuperadminEmail != "" {
		if err := bootstrap.Superadmin(ctx, st, a.cfg.SuperadminEmail); err != nil {
			closeGORM(gdb)
			return nil, fmt.Errorf("init superadmin: %w", err)
		}
	}

	a.mu.Lock()
	a.gdb = gdb
	a.store = st
	a.mu.Unlock()
	// 组装完整业务路由(受保护路由回调在此执行一次)
	a.setFallback(a.fullHandler())
	return gdb, nil
}

func closeGORM(gdb *gorm.DB) {
	if gdb == nil {
		return
	}
	if sqlDB, err := gdb.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

// Run 启动应用（包括数据库连接、迁移、Worker 任务与 HTTP 服务监听），该调用是阻塞式的。
//
// 返回值约定:
//   - 数据库连接/迁移/超管初始化/端口监听失败:返回对应错误;
//   - ctx 取消或调用 Shutdown 触发的优雅停止:返回 nil;
//   - HTTP 服务异常退出:先停止 worker 与服务,再返回该错误。
//
// 无论何种方式退出,running 都会复位,Run 自行打开的数据库连接都会关闭。
func (a *App) Run(ctx context.Context) error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return errors.New("app is already running")
	}
	a.running = true

	runCtx, cancel := context.WithCancel(ctx)
	a.cancelRun = cancel
	workerDone := make(chan struct{})
	a.workerDone = workerDone
	a.mu.Unlock()

	var (
		owned         *gorm.DB
		workerStarted bool
	)
	defer func() {
		cancel()
		if !workerStarted {
			close(workerDone)
		} else {
			// 关闭 DB 前等待 worker 退出(无 worker 时已关闭),避免其继续使用已关闭的连接池
			select {
			case <-workerDone:
			case <-time.After(5 * time.Second):
			}
		}
		a.mu.Lock()
		if owned != nil {
			closeGORM(owned)
			a.gdb = nil
			a.store = nil
			a.setFallback(nil)
		}
		a.server = nil
		a.running = false
		a.mu.Unlock()
	}()

	// 数据库初始化期间不持锁,Shutdown 可随时通过 cancelRun 中断启动
	var err error
	owned, err = a.setupDB(runCtx)
	if err != nil {
		if runCtx.Err() != nil {
			return nil // 启动期间被请求停止
		}
		return err
	}

	addr := a.cfg.Addr
	if addr == "" {
		addr = ":8080"
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           a.engine,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	a.mu.Lock()
	if runCtx.Err() != nil {
		// 在启动过程中已被 Shutdown/ctx 取消
		a.mu.Unlock()
		_ = listener.Close()
		return nil
	}
	a.server = srv
	st := a.store
	a.mu.Unlock()

	// 启动后台 worker;无 Store 时立即关闭 workerDone,Shutdown 无需等待
	workerStarted = true
	if st != nil {
		go func() {
			defer close(workerDone)
			domain.NewWorker(st, a.cfg).Run(runCtx)
		}()
	} else {
		close(workerDone)
	}

	serverErrCh := make(chan error, 1)
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
		// 请求的优雅停止返回 nil,而不是 context.Canceled
		return a.Shutdown(shutdownCtx)
	case err := <-serverErrCh:
		if err == nil {
			return nil // Shutdown 已关闭服务
		}
		shutdownCtx, sCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer sCancel()
		_ = a.Shutdown(shutdownCtx)
		return fmt.Errorf("http serve: %w", err)
	}
}

// Shutdown 优雅关闭服务:取消 Run 上下文、关闭 HTTP 服务并等待 worker 退出。
// running 状态与 Run 自行打开的数据库连接由 Run 退出时统一复位/关闭。
func (a *App) Shutdown(ctx context.Context) error {
	// 只在取快照时持锁:等待 worker/服务退出期间不持锁,避免与 Run 的启动/退出路径互相阻塞
	a.mu.Lock()
	cancelRun, srv, workerDone := a.cancelRun, a.server, a.workerDone
	a.server = nil
	a.mu.Unlock()

	if cancelRun != nil {
		cancelRun()
	}

	var srvErr error
	if srv != nil {
		srvErr = srv.Shutdown(ctx)
	}

	if workerDone != nil {
		select {
		case <-workerDone:
		case <-ctx.Done():
		}
	}

	return srvErr
}

// mutate4go-manifest-begin
// {"version":1,"tested_at":"2026-10-06T03:57:37+08:00","module_hash":"340213c02814e0eca7d0fae98f01604c45bc91917551435b39931c8ff730bbe0","functions":[{"id":"func/WithConfig","name":"WithConfig","line":42,"end_line":47,"hash":"b106c8c7b707e97fb1509bda36d7d3d8c6b25d7de30af47502ec83a9d2b3020d"},{"id":"func/WithConfigFile","name":"WithConfigFile","line":50,"end_line":60,"hash":"a14a2851c83a8e91072f6dfd591e1a281edf12c2bd9c75b95aa548f67766b4d9"},{"id":"func/WithDB","name":"WithDB","line":63,"end_line":68,"hash":"6b83114b2223d90bd2e439e3be22834d86af6d3591378fc7aff2ede30ab3470e"},{"id":"func/WithRoutes","name":"WithRoutes","line":71,"end_line":78,"hash":"21ef0f977e91769dafc799ed601bafe0252f0e48f443f429d20cdd36a2af0191"},{"id":"func/New","name":"New","line":95,"end_line":135,"hash":"42d37307e05362150f7b6eb7dcdd4faff6b83e8b0573976456978dae11a602d0"},{"id":"func/validateAppConfig","name":"validateAppConfig","line":138,"end_line":159,"hash":"dd8e87bbd6f0b9798f30efb9cf1ffff158dd46c35ea45178e7cfec2b3ddd1cfc"},{"id":"func/App.initHandler","name":"App.initHandler","line":162,"end_line":206,"hash":"ae96bde8e058bb4ea3105df0c5afea63873e6caae8e300c80056b56187bd9548"},{"id":"func/App.Handler","name":"App.Handler","line":209,"end_line":211,"hash":"c585e630f00b671d3b81479b9d33ac2a9901d086aef3417046a9848a75e3be14"},{"id":"func/App.Run","name":"App.Run","line":214,"end_line":298,"hash":"bd0a49c28fe8ebd055087416a7274930006069ca9fc31e5a9a58169b48df02ba"},{"id":"func/App.Shutdown","name":"App.Shutdown","line":301,"end_line":324,"hash":"6635f11f6ccb010bdb5f81a4b84cc18486513550eeae96971ea0fc01af410cd3"}]}
// mutate4go-manifest-end
