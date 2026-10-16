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

// GetAuthContext 从 Gin 上下文中提取当前通过认证的租户与用户身份。
//
// 注意:当上下文未携带角色(既无 CtxRoleKey 也无 "auth.role")时,返回的
// AuthContext.Role 为空字符串,而不再静默补成 "tenant"。静默补默认会把"没拿到
// 角色"伪装成"确认是普通租户",让调用方无法区分;需要默认值的调用方请显式判定
// 空值。只要取到了租户 ID,第二个返回值仍为 true。
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
	eventListeners        []pendingListener
	extraFS               fs.FS
}

// WithConfig 显式传入预先加载或定制的配置结构体
func WithConfig(cfg Config) Option {
	return func(o *options) error {
		o.cfg = &cfg
		return nil
	}
}

// WithConfigFile 指定配置文件路径（支持 .env 或环境变量文件）。
// 文件在 New 时被解析并合并进本实例的 Config:进程环境里已存在的同名键优先
// (部署时可用环境变量覆盖文件),且不写回进程环境。同时传入 WithConfig 时以
// WithConfig 为准(显式结构体是调用方的完整配置)。
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

// WithProtectedRoutes 注入受保护的控制面路由组，外部 Handler 自动继承基座的多租户认证与 Casbin RBAC 鉴权。
//
// 需要数据库支撑:未提供 WithDB 且未配置 postgres 数据库地址时,New 会显式报错
// (否则这些 handler 只能挂在恒定 401 的死代码后、永远不可达)。
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

// WithEventListener 注册领域事件监听器。
//
// 订阅在 New 成功构建后统一完成,并在 App.Shutdown 时解绑 —— 避免全局单例订阅
// 在实例销毁后继续被触发(多实例互相串扰、重复注册)。未调用 Shutdown 就丢弃实例
// 仍会残留订阅,调用方应保证收尾时调用 Shutdown。
func WithEventListener(topic string, handler event.Handler) Option {
	return func(o *options) error {
		if topic == "" || handler == nil {
			return nil
		}
		o.eventListeners = append(o.eventListeners, pendingListener{topic: topic, handler: handler})
		return nil
	}
}

// pendingListener 待注册的事件监听器(构建成功后才真正订阅)。
type pendingListener struct {
	topic   string
	handler event.Handler
}

// eventSubscription 已注册监听器的解绑凭据。
type eventSubscription struct {
	topic string
	id    uint64
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
	eventSubs             []eventSubscription
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
	switch {
	case opt.cfg != nil:
		// 显式传入的结构体视为调用方的完整配置:不再叠加环境或配置文件,
		// 否则调用方刻意覆盖的字段(如测试注入的测试库 DSN)会被环境变量拽回去。
		cfg = *opt.cfg
	case opt.configPath != "":
		loaded, err := config.LoadWithEnvFile(opt.configPath)
		if err != nil {
			return nil, fmt.Errorf("load config file %s: %w", opt.configPath, err)
		}
		cfg = loaded
	default:
		loaded, err := config.Load()
		if err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
		cfg = loaded
	}

	// 校验配置合规性
	if err := validateAppConfig(cfg); err != nil {
		return nil, err
	}

	// 真正的「无 DB 模式」(既未注入 DB,也未配置 postgres DSN)下,受保护路由的
	// handler 只会挂在恒定 401 的死代码后、永远不可达。与其静默丢弃这些路由,
	// 不如显式失败:调用方要么提供 WithDB / postgres 地址,要么改用 WithRoutes。
	// 注意 DatabaseURL 为 postgres DSN 时不算无 DB —— Run 会连库并重建路由。
	if opt.customDB == nil && !isPostgresDSN(cfg.DatabaseURL) && len(opt.protectedRouteInjects) > 0 {
		return nil, errors.New("WithProtectedRoutes 需要数据库支撑认证与鉴权,但既未提供 WithDB 也未配置 postgres 数据库地址")
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

	// 所有可能失败的构建步骤都已完成,最后再注册事件订阅并记录解绑凭据。
	// 放在末尾可保证「New 失败时不会留下已注册的订阅」。
	for _, l := range opt.eventListeners {
		if id := event.SubscribeID(l.topic, l.handler); id != 0 {
			app.eventSubs = append(app.eventSubs, eventSubscription{topic: l.topic, id: id})
		}
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

	// 无条件执行基座内部声明式校验(时长、范围等)。
	// 旧实现用「SessionTTL > 0 || LandingMaxFiles > 0」这种符号启发式决定是否校验,
	// 于是整块字段为负(或恰好为零)时直接跳过:非正时长一路流到 worker 的
	// time.NewTicker 才 panic,而 panic 发生在 goroutine 里会终止整个进程。
	// Config.Validate 自身已对「整块未配置」的零值做容忍,不会误伤 SDK 轻量配置。
	if err := cfg.Validate(); err != nil {
		return err
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

	// 任何返回路径都必须复位 running:否则一次启动失败(bootstrap/监听失败/服务异常)
	// 就让 App 永久停在 running=true,后续 Run 只会返回 "already running",只能重建
	// 实例。正常收尾路径(ctx 取消 → Shutdown)里 Shutdown 也会复位,重复复位幂等。
	defer func() {
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
	}()

	// 数据库启动失败必须向上返回:吞掉错误只会让进程「看着健康」——实际没有数据库、
	// 没跑迁移、没挂业务路由,却照常对外监听 HTTP。
	if err := a.bootstrapDatabase(runCtx); err != nil {
		cancel()
		a.mu.Unlock()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			// 启动途中被调用方取消属于正常收尾,不能报成启动故障。
			return runCtx.Err()
		}
		return err
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
		// HTTP 服务已异常退出,但后台 worker 仍绑在 runCtx 上继续写库:
		// 必须 cancel 并等它收口,否则调用方以为 App 已停,worker goroutine
		// 却在继续以陈旧状态写库,连接/咨询锁随之一并泄漏。
		cancel()
		if a.workerDone != nil {
			select {
			case <-a.workerDone:
			case <-time.After(3 * time.Second):
			}
		}
		return err
	}
}

// bootstrapDatabase 按配置建立数据库连接、执行迁移、初始化超管并组装完整业务路由。
// 启动链路上的每一步错误都向上返回:继续启动等于把启动故障伪装成健康。
func (a *App) bootstrapDatabase(ctx context.Context) error {
	if a.gdb == nil && isPostgresDSN(a.cfg.DatabaseURL) {
		pool, err := db.Connect(ctx, a.cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("connect database: %w", err)
		}
		if a.cfg.MigrationsDir != "" || a.extraFS != nil {
			if err := db.MigrateWithExtra(ctx, pool, a.cfg.MigrationsDir, a.extraFS); err != nil {
				pool.Close()
				return fmt.Errorf("migrate database: %w", err)
			}
		}
		// schema 初始化完成即关闭:业务数据访问全部走 GORM,这个 pgxpool 此后没有使用者。
		pool.Close()

		gdb, err := db.OpenGORM(a.cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("open gorm: %w", err)
		}
		a.gdb = gdb
		a.store = store.New(gdb)
		if a.cfg.SuperadminEmail != "" {
			if err := bootstrap.Superadmin(ctx, a.store, a.cfg.SuperadminEmail); err != nil {
				return fmt.Errorf("init superadmin: %w", err)
			}
		}
		// 组装底层完整路由
		return a.initHandler()
	}

	if a.gdb != nil && a.extraFS != nil {
		sqlDB, err := a.gdb.DB()
		if err != nil {
			return fmt.Errorf("gorm sql db: %w", err)
		}
		if err := db.MigrateDBWithExtra(ctx, sqlDB, "", a.extraFS); err != nil {
			return fmt.Errorf("migrate extra: %w", err)
		}
	}
	return nil
}

// isPostgresDSN 判断 DSN 是否指向 Postgres。pgx 同时接受 postgres:// 与 postgresql://,
// 只认前者会让 postgresql:// 的配置被整段跳过:进程照常监听,却从不连库。
func isPostgresDSN(dsn string) bool {
	return strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://")
}

// Shutdown 优雅关闭服务
func (a *App) Shutdown(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cancelRun != nil {
		a.cancelRun()
	}

	// 解绑本实例注册的全局事件订阅:否则实例销毁后订阅仍挂在全局事件总线上,
	// 多实例场景会互相串扰、重复触发。
	for _, s := range a.eventSubs {
		event.Unsubscribe(s.topic, s.id)
	}
	a.eventSubs = nil

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
// {"version":1,"tested_at":"2026-10-12T16:48:53+08:00","module_hash":"56fde873ed52408de179e7483073ab23b8653f8617384a7ce0d8c139706e9df1","functions":[{"id":"func/GetAuthContext","name":"GetAuthContext","line":51,"end_line":100,"hash":"33ab8303acc7bb592e2db28c5bd2a3cc8fd1c5d9160aeeaa90f42b96767270ba"},{"id":"func/toInt64","name":"toInt64","line":102,"end_line":116,"hash":"4b2a32b0286c902a8f74a0f80e068dc612ca714dda96f465752b0fd951ac9840"},{"id":"func/WithConfig","name":"WithConfig","line":131,"end_line":136,"hash":"b106c8c7b707e97fb1509bda36d7d3d8c6b25d7de30af47502ec83a9d2b3020d"},{"id":"func/WithConfigFile","name":"WithConfigFile","line":142,"end_line":152,"hash":"a14a2851c83a8e91072f6dfd591e1a281edf12c2bd9c75b95aa548f67766b4d9"},{"id":"func/WithDB","name":"WithDB","line":155,"end_line":160,"hash":"6b83114b2223d90bd2e439e3be22834d86af6d3591378fc7aff2ede30ab3470e"},{"id":"func/WithRoutes","name":"WithRoutes","line":163,"end_line":170,"hash":"21ef0f977e91769dafc799ed601bafe0252f0e48f443f429d20cdd36a2af0191"},{"id":"func/WithProtectedRoutes","name":"WithProtectedRoutes","line":173,"end_line":180,"hash":"c9098511bff5085e0f63eee74d2c83310933c6ab2c982c7312d7026589339b70"},{"id":"func/WithExtraMigrations","name":"WithExtraMigrations","line":183,"end_line":188,"hash":"831a7b598b3a118f60d0039e0d22788344f472f187ec8b0f077bee5ffa90ca83"},{"id":"func/WithExtraMigrationsDir","name":"WithExtraMigrationsDir","line":191,"end_line":201,"hash":"2de2f7f2e2c5541476a22e34b121deeb343e497c4a37ec5d6f1047e66e5679b1"},{"id":"func/WithEventListener","name":"WithEventListener","line":204,"end_line":209,"hash":"3923c09176c804f79edd79c790fca21ee49ad3006bb818288686c5a83d109401"},{"id":"func/SimpleEvent.Topic","name":"SimpleEvent.Topic","line":221,"end_line":221,"hash":"635dbbccc7a0ee52e201628f9fe6d3ec56ade0089bdcd8b6293533134ec30701"},{"id":"func/SimpleEvent.Payload","name":"SimpleEvent.Payload","line":222,"end_line":222,"hash":"afd7c73d21e8bb101c5e2db185ad2d4323206657571f6f94c7aafe6bb09e0a05"},{"id":"func/SimpleEvent.OccurredAt","name":"SimpleEvent.OccurredAt","line":223,"end_line":223,"hash":"08cdfa0afff001643dc0cc41661ce27d26507c76da9fbc5daa892a2df3e43ca6"},{"id":"func/NewSimpleEvent","name":"NewSimpleEvent","line":226,"end_line":232,"hash":"bb771800cc33b255225ff8a899dc5bece95efa42a36fa7eefcb4f539fc1aaf85"},{"id":"func/PublishEvent","name":"PublishEvent","line":235,"end_line":237,"hash":"06d1767750c05883b6e12dd0c01563d92adc56a6c8dce6cbd4fc95bd4d05c9c2"},{"id":"func/New","name":"New","line":256,"end_line":307,"hash":"c46167174033539221cd6ea88396717a91021459ef900236cc3dc2229f7be981"},{"id":"func/validateAppConfig","name":"validateAppConfig","line":310,"end_line":331,"hash":"dd8e87bbd6f0b9798f30efb9cf1ffff158dd46c35ea45178e7cfec2b3ddd1cfc"},{"id":"func/App.initHandler","name":"App.initHandler","line":334,"end_line":386,"hash":"1745b5052b4d9968b2e53b8b0271bfe72637d962a0882d99a4819a7be6258679"},{"id":"func/App.Handler","name":"App.Handler","line":389,"end_line":391,"hash":"c585e630f00b671d3b81479b9d33ac2a9901d086aef3417046a9848a75e3be14"},{"id":"func/App.Run","name":"App.Run","line":394,"end_line":468,"hash":"417a564da71cdd75a4ae6a9079f79b14af772afe39cdec463e68615038519207"},{"id":"func/App.bootstrapDatabase","name":"App.bootstrapDatabase","line":472,"end_line":512,"hash":"fb21f35ee1c24c7768a82021f86eeae3f3412f08824b42ee2a5ffcd19516e43f"},{"id":"func/isPostgresDSN","name":"isPostgresDSN","line":516,"end_line":518,"hash":"7aecce45d3e9dcfaccdf797a5d01dd0b137b4b52d2ef03416d1e633fb4b1e37e"},{"id":"func/App.Shutdown","name":"App.Shutdown","line":521,"end_line":544,"hash":"6635f11f6ccb010bdb5f81a4b84cc18486513550eeae96971ea0fc01af410cd3"}]}
// mutate4go-manifest-end
