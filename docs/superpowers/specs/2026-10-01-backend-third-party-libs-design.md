# 后端纯逻辑模块全面采用成熟第三方库架构规格说明 (Backend Third-Party Libraries Adoption Spec)

Status: draft  
Date: 2026-10-01  
Source: 用户架构原则要求「前后端所有功能模块必须使用成熟的第三方库 不要自己实现」

## 1. 背景与目标

在 Janus 短链系统后端现存代码中，有若干核心逻辑模块采用了自行造轮子（或仅使用 Go 标准库手动拼装协议/数据结构）的实现方式，包括：
1. `internal/rules/radix.go`：手写 200+ 行 `IPRadixTree` 切片索引位前缀树；
2. `internal/httpapi/ratelimit.go`：手写固定窗口计数器与内存淘汰协程；
3. `internal/mailer/smtp.go`：手写底层 SMTP 协议交互状态机与 Header/MIME 拼接；
4. `internal/config/config.go`：手写大量 `os.Getenv` 与类型转换函数；
5. `internal/db/migrate.go`：手写扫描迁移目录、排序与手工操作版本表。

这些自研实现不仅增加了长期维护与边界缺陷的风险，而且放弃了成熟开源生态所具备的边界防御、性能调优和扩展能力。

**核心目标**：
全面替换上述 5 个自研逻辑模块为业界公认、高质量、维护活跃的成熟开源第三方库，彻底杜绝自研轮子，保持系统原有对外接口、性能表现与单元测试 100% 兼容。

---

## 2. 范围与边界

### 纳入范围 (In Scope)
- `internal/rules`：CIDR/IP 前缀匹配引入 `github.com/yl2chen/cidranger`，淘汰 `IPRadixTree` 手写位前缀树。
- `internal/httpapi`：认证限流引入官方扩展库 `golang.org/x/time/rate` 令牌桶算法，替换手写固定窗口限流。
- `internal/mailer`：SMTP 客户端引入现代高品质库 `github.com/wneessen/go-mail`，替换标准库手写状态机。
- `internal/config`：环境变量加载引入 `github.com/caarlos0/env/v11` 声明式解析，替换手写 `os.Getenv` 样板代码。
- `internal/db`：SQL 数据库迁移引入 `github.com/pressly/goose/v3`，替换手写读取文件与事务执行逻辑。

### 不在范围 (Non-Goals)
- 前端页面与 UI 组件：前端 shadcn-vue / Reka UI 组件与样式不在本次重构范围内。
- 后端已有成熟依赖库（如 `gin-gonic/gin`、`gorm.io/gorm`、`casbin/casbin/v2`、`golang-jwt/jwt/v5`、`expr-lang/expr`、`ip2region` 等）已属成熟选型，保持不动。
- 不变更 RESTful API 端点契约与数据库业务表结构。

---

## 3. 详细技术规范与改造方案

### 3.1 模块 1：CIDR 网段匹配 (`internal/rules`)
- **替换目标**：`github.com/yl2chen/cidranger`
- **设计方案**：
  1. 在 `internal/rules` 中封装 `IPRanger` 结构体：
     ```go
     type IPRanger struct {
         ranger cidranger.Ranger
     }
     ```
  2. 构造函数 `NewIPRanger(prefixes []netip.Prefix) *IPRanger`：
     - 使用 `cidranger.NewPCTrieRanger()` 初始化基于路径压缩 Trie 树的 Ranger。
     - 遍历 `netip.Prefix`，将其转换为 `*net.IPNet` 后调用 `ranger.Insert(cidranger.NewBasicRangerEntry(*ipnet))`。
  3. 判定方法 `(r *IPRanger) Contains(addr netip.Addr) bool`：
     - 将 `netip.Addr` 转为 `net.IP(addr.AsSlice())`，调用 `r.ranger.Contains(ip)`。
  4. 改造 `internal/rules/eval.go`：
     - 将 `CompiledCondition.radix` 字段类型由 `*IPRadixTree` 替换为 `*IPRanger`。
     - 彻底删除 `internal/rules/radix.go` 中所有手写二叉树节点分配（`radixNode`、`newNode`、`bitAt` 等 200+ 行代码）。
  5. 兼容性：`radix_test.go` 和 `eval_test.go` 全部保留，验证 IPv4/IPv6 单 IP、网段前缀、掩码规范化、边界值的一致性。

### 3.2 模块 2：API 认证限流 (`internal/httpapi`)
- **替换目标**：`golang.org/x/time/rate`
- **设计方案**：
  1. 重写 `internal/httpapi/ratelimit.go`：
     - 使用 Go 官方令牌桶库 `golang.org/x/time/rate`。
     - 定义结构体：
       ```go
       type tokenBucketLimiter struct {
           limit    rate.Limit
           burst    int
           mu       sync.Mutex
           visitors map[string]*visitorBucket
       }
       type visitorBucket struct {
           limiter  *rate.Limiter
           lastSeen time.Time
       }
       ```
     - 速率计算：若 `limit > 0 && window > 0`，产出速率为 `rate.Limit(float64(limit) / window.Seconds())`，桶容量 `burst = limit`。
     - `Allow(key string) bool`：获取或创建该 IP 对应的 `*rate.Limiter`，刷新 `lastSeen`，调用 `limiter.Allow()`。
     - 淘汰清理：当 `visitors` 容量超过上限（如 8192）或后台定时触发时，清理掉超过 `window * 2` 未访问的过期 bucket，防止恶意伪造 IP 导致内存无界增长。
  2. 兼容性：现有测试 `ratelimit_test.go` 中的超限拦截状态码 `429 Too Many Requests` 与响应体 `E_RATE_LIMITED` 保持完全一致。

### 3.3 模块 3：SMTP 邮件客户端 (`internal/mailer`)
- **替换目标**：`github.com/wneessen/go-mail`
- **设计方案**：
  1. 重写 `internal/mailer/smtp.go` 中的 `SMTPMailer`：
     - 保持现有 `mailer.Mailer` 接口不变：
       - `SendVerifyEmail(to, token string) error`
       - `SendResetEmail(to, token string) error`
  2. 客户端初始化与配置：
     - 使用 `mail.NewClient(m.cfg.Host, mail.WithPort(m.cfg.Port), mail.WithTimeout(30*time.Second))`。
     - 安全策略：
       - 当端口为 465 时启用隐式 SSL：`mail.WithSSL()`。
       - 当端口为 587 或其他端口时强制 STARTTLS：`mail.WithTLSPolicy(mail.TLSMandatory)`。
       - 用户名非空时配置认证：`mail.WithSMTPAuth(mail.SMTPAuthPlain), mail.WithUsername(cfg.Username), mail.WithPassword(cfg.Password)`。
  3. 邮件报文构造：
     - 使用 `mail.NewMsg()`：
       - `msg.From(from)`
       - `msg.To(to)`
       - `msg.Subject(subject)`
       - `msg.SetBodyString(mail.TypeTextPlain, body)`
     - 调用 `client.DialAndSend(msg)`。
  4. 彻底删除手工 `net.DialTimeout`、手写状态机握手与字符串 MIME 拼接逻辑。

### 3.4 模块 4：配置声明式解析 (`internal/config`)
- **替换目标**：`github.com/caarlos0/env/v11`
- **设计方案**：
  1. 升级 `Config` 结构体，添加声明式 struct tag：
     ```go
     type Config struct {
         Addr            string        `env:"JANUS_ADDR" envDefault:":8080"`
         DatabaseURL     string        `env:"JANUS_DATABASE_URL" envDefault:"postgres://postgres:postgres@localhost:5432/janus?sslmode=disable"`
         PlatformDomain  string        `env:"JANUS_PLATFORM_DOMAIN" envDefault:"janus.test"`
         ServerPublicIP  string        `env:"JANUS_SERVER_PUBLIC_IP" envDefault:"127.0.0.1"`
         SuperadminEmail string        `env:"JANUS_SUPERADMIN_EMAIL"`
         CookieSecure    bool          `env:"JANUS_COOKIE_SECURE" envDefault:"true"`
         SessionTTL      time.Duration `env:"JANUS_SESSION_TTL" envDefault:"720h"`
         SessionTTLShort time.Duration `env:"JANUS_SESSION_TTL_SHORT" envDefault:"24h"`
         VerifyTokenTTL  time.Duration `env:"JANUS_VERIFY_TOKEN_TTL" envDefault:"24h"`
         ResetTokenTTL   time.Duration `env:"JANUS_RESET_TOKEN_TTL" envDefault:"1h"`
         JWTSecret       string        `env:"JANUS_JWT_SECRET"`
         JWTTTL          time.Duration `env:"JANUS_JWT_TTL" envDefault:"24h"`
         DNSRetryInterval  time.Duration `env:"JANUS_DNS_RETRY_INTERVAL" envDefault:"1m"`
         DNSMaxAge         time.Duration `env:"JANUS_DNS_MAX_AGE" envDefault:"72h"`
         VisitRetention    time.Duration `env:"JANUS_VISIT_RETENTION" envDefault:"2160h"`
         VisitCleanupEvery time.Duration `env:"JANUS_VISIT_CLEANUP_EVERY" envDefault:"24h"`
         LandingUploadDir   string        `env:"JANUS_LANDING_UPLOAD_DIR" envDefault:"./uploads"`
         LandingMaxZipBytes int64         `env:"JANUS_LANDING_MAX_ZIP_BYTES" envDefault:"20971520"`
         LandingMaxFiles    int           `env:"JANUS_LANDING_MAX_FILES" envDefault:"200"`
         MigrationsDir string        `env:"JANUS_MIGRATIONS_DIR" envDefault:"migrations"`
         PublicBaseURL string        `env:"JANUS_PUBLIC_BASE_URL" envDefault:"http://app.janus.test:5173"`
         SMTPHost      string        `env:"JANUS_SMTP_HOST"`
         SMTPPort      int           `env:"JANUS_SMTP_PORT" envDefault:"587"`
         SMTPUsername  string        `env:"JANUS_SMTP_USERNAME"`
         SMTPPassword  string        `env:"JANUS_SMTP_PASSWORD"`
         SMTPFrom      string        `env:"JANUS_SMTP_FROM"`
     }
     ```
  2. `config.Load()` 直接调用 `env.Parse(&cfg)`。
  3. 删除 `config.go` 中所有手写 `getenv`、`getbool`、`getint`、`getdur` 函数。

### 3.5 模块 5：SQL 数据库迁移 (`internal/db`)
- **替换目标**：`github.com/pressly/goose/v3`
- **设计方案**：
  1. 重写 `internal/db/migrate.go`：
     - 使用 `github.com/jackc/pgx/v5/stdlib` 从现有 `*pgxpool.Pool` 获取标准 `*sql.DB`：
       ```go
       db := stdlib.OpenDBFromPool(pool)
       defer db.Close()
       ```
     - 设置方言为 Postgres：`goose.SetDialect("postgres")`。
     - 平滑兼容：现有库使用的是 `schema_migrations` 表（存储 `0001_init.sql` 文件名）。若存在历史表，自动将其同步登记入 goose 的版本表 `goose_db_version`，确保已有 14 个版本的迁移文件不被重复执行。
     - 执行迁移：`goose.Up(db, dir)`。
  2. 保持对外签名 `Migrate(ctx context.Context, pool *pgxpool.Pool, dir string) error` 不变。

---

## 4. 实施计划与步骤

1. **Step 1：引入 Go 模块依赖**
   - 执行 `go get github.com/yl2chen/cidranger`
   - 执行 `go get github.com/wneessen/go-mail`
   - 执行 `go get github.com/caarlos0/env/v11`
   - 执行 `go get github.com/pressly/goose/v3`
   - 执行 `go get golang.org/x/time/rate`
2. **Step 2：CIDR 前缀匹配重构**（`internal/rules`）并运行全部单元测试与基准测试。
3. **Step 3：API 内存令牌桶限流重构**（`internal/httpapi`）并运行认证接口限流测试。
4. **Step 4：SMTP 邮件客户端重构**（`internal/mailer`）并编写单元测试验证客户端参数配置与消息构造。
5. **Step 5：配置解析声明式迁移**（`internal/config`）并验证默认值及环境变量覆盖。
6. **Step 6：数据库迁移切换为 goose**（`internal/db`）并验证迁移测试与向后兼容性。
7. **Step 7：全量测试回归与文档更新**（运行 `go test ./...`，确认全部包通过）。

---

## 5. 验收标准

- `go test ./...` 全量通过，无任何失败用例。
- `internal/rules/radix.go` 中的自研树代码完全被 `cidranger` 替代。
- `internal/httpapi/ratelimit.go` 中的自研固定窗口计数器完全被 `x/time/rate` 替代。
- `internal/mailer/smtp.go` 中的自写协议交互完全被 `go-mail` 替代。
- `internal/config/config.go` 中的自写转换解析完全被 `caarlos0/env/v11` 替代。
- `internal/db/migrate.go` 中的自写文件扫描与迁移完全被 `goose/v3` 替代。
- 整个后端代码中没有任何自行实现的底层网络树、限流、协议或迁移轮子。
