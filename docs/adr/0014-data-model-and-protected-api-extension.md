# ADR-0014: 数据模型扩展与受保护控制面 API 开放 (Data Model & Protected API Extension)

## 状态

已提议 (Proposed) / 阶段一契约定义

## 背景

在 Janus 架构演进路线中，[ADR-0012: 开源基座功能剥离与插件化扩展点设计规范](0012-open-core-extension-points.md) 确立了数据流热路径（Redirect / Landing）的 5 大微秒级插件化扩展点，而 [ADR-0013: 架构基座化与 App Builder 模式](0013-application-base-and-builder.md) 确立了公开公共包 `pkg/janus`、`pkg/plugin` 与私有包 `internal/*` 的单向依赖拓扑及生命周期控制。

随着进入「阶段二：数据模型扩展与控制面 API 开放」，外部高级商业与对抗衍生工程（例如 `janus-cloak` 斗篷/TDS 网关）在以独立二进制或库形态包装基座时，面临三大深层次的工程壁垒：

1. **核心模型定制属性无法无侵入挂载 (Metadata Inflexibility)**：
   - 现有的核心业务模型（如短链 `store.Link`）在数据库 DDL 和 Go 结构体中被严格硬编码。外部高级版需要在短链上附带专属业务参数（例如 Cloak 状态机 Safe Mode 阈值、Money Page 隐形反代 Proxy 目标、渠道投放归因 tags、防审探针模式等）。
   - 若每增加一个业务属性就修改基座数据表结构并在 `internal/store` 增加字段，将彻底破坏基座的阳光通用性，并导致基座版本升级时出现严重的 DDL 与代码冲突。
2. **私有工程专有表的数据库迁移无法纳入基座统一编排 (Migration Friction)**：
   - 私有高级版通常具备专属的数据表（如 `cloak_rules`、`postback_logs`、`fingerprint_profiles` 等）。
   - 目前基座在启动时仅按 `config.MigrationsDir` 执行内建的 `migrations/*.sql`。外部工程若脱离基座单独执行迁移，将破坏单二进制部署的一致性；若直接将 SQL 文件塞入基座 `migrations/` 目录，又会严重污染开源代码库。
3. **控制面专有 API 无法无缝复用基座认证与 RBAC 鉴权 (Auth & RBAC Silo)**：
   - 私有高级版需要对外暴露面向租户后台的管理 API（如 `/api/cloak/rules`、`/api/cloak/stats`、`/api/postback/config` 等）。
   - 目前基座暴露的 `janus.WithRoutes` 仅作用于根 `gin.Engine`（完全公开路由）。外部工程若自行编写鉴权，就必须复制基座的 JWT 验签、Cookie 会话解析、密码变更版本吊销（`token_version`）与 Casbin 权限模型，不仅增加维护负担，还极易产生安全漏洞与状态不一致。

因此，亟需确立核心模型元数据扩展、外部双轨迁移注入以及受保护控制面路由注入的标准化架构契约。

---

## 架构决策

### 1. 数据模型 Metadata JSONB 无侵入扩展契约

为了让基座核心模型（首期以 `Link` 短链为核心）支持外部模块无侵入存取业务元数据，做出以下规范：

#### 1.1 数据库字段与 DDL 契约
在 `links` 表中增加通用的 `metadata` 列：
```sql
ALTER TABLE links ADD COLUMN metadata JSONB NOT NULL DEFAULT '{}'::jsonb;
```
- 默认值必须为 `'{}'::jsonb`，且不允许为 `NULL`，确保所有查询与序列化具有确定性基底。

#### 1.2 Go 类型封装与防御性语义 (`janus.Metadata`)
定义通用的 `Metadata` 字典类型：
```go
package janus

// Metadata 是通用的业务元数据 JSONB 字典类型
type Metadata map[string]any
```
该类型必须严格实现以下标准接口：
- **`driver.Valuer` 接口**：
  - 序列化为有效 JSON 字节切片/文本。
  - **空安全契约**：当 `Metadata` 为 `nil` 或空 map 时，写入数据库的值必须为 `"{}"`，严禁序列化为 SQL `NULL` 或 `"null"`。
- **`sql.Scanner` 接口**：
  - 从数据库读取 `[]byte` 或 `string` 并反序列化为 `map[string]any`。
  - **防崩溃契约**：当数据库读取值为 `nil`、空字节或无法解析时，自动重置为空字典 `Metadata{}`，**绝不返回 nil**，确保调用端在访问 `link.Metadata["key"]` 时免除空指针 Panic 风险。
- **`GormDBDataType` 契约**：
  - 在 Postgres 方言下返回 `"jsonb"`，在通用方言下返回 `"json"`。

#### 1.3 模型集成与性能铁律
- `store.Link` 结构体新增字段：
  ```go
  Metadata Metadata `json:"metadata" gorm:"column:metadata;type:jsonb"`
  ```
- **热路径性能铁律 (Hot Path Invariant)**：
  - 短链跳转热路径（`GET /:code`）在定位短链时**不进行无谓的元数据深层复杂计算**；元数据仅作为静态属性由内存快照或扩展点按需提取，杜绝造成写放大或延迟抖动。

---

### 2. 双轨/外部扩展迁移支持 (`janus.WithExtraMigrations`)

为了保证外部衍生系统的数据模型生命周期与基座完全同步，引入双轨迁移机制（Dual-track Migrations）。

#### 2.1 函数式 Option 声明
在 `pkg/janus` 中增加配置选项：
```go
// WithExtraMigrations 注入外部工程的专有迁移脚本（支持 embed.FS、os.DirFS 或 MapFS）
func WithExtraMigrations(fsys fs.FS) Option

// WithExtraMigrationsDir 注入外部工程的文件目录路径迁移脚本（便捷封装）
func WithExtraMigrationsDir(dir string) Option
```

#### 2.2 编排执行流水线
迁移执行时遵循以下严密编排流程：
```text
            应用启动 / 迁移触发 (App.Run / Migrate)
                             │
                             ▼
              [获取分布式 PostgreSQL Advisory Lock]
         SELECT pg_advisory_lock(hashtext('janus-migrate'))
                             │
                             ▼
              【第一轨：基座核心迁移】
              执行基座 migrations/*.sql
              版本记录跟踪于: goose_db_version
                             │ (成功完成后)
                             ▼
              【第二轨：外部专有迁移】
              若注册了 WithExtraMigrations(fsys)
              执行 fsys 中的外部 Goose SQL 迁移
              版本记录跟踪于: goose_db_version_extra
                             │
                             ▼
              [释放 PostgreSQL Advisory Lock]
         SELECT pg_advisory_unlock(hashtext('janus-migrate'))
```

#### 2.3 版本隔离与安全性保障
- **版本表命名隔离**：外部迁移使用独立的 Goose 版本控制表（如 `goose_db_version_extra`，通过 `goose.WithTableName("goose_db_version_extra")` 或 `goose.SetTableName` 隔离），彻底消除外部迁移版本号（如 `0001_`）与基座内建迁移版本号之间的命名与顺序冲突。
- **全内存文件系统支持 (`fs.FS`)**：接受标准库 `io/fs.FS`，使外部工程可以通过 Go 1.16+ 的 `//go:embed migrations/*.sql` 将私有 SQL 内嵌在私有二进制中，无需额外在宿主机挂载文件目录。

---

### 3. 控制面扩展：受保护路由组注入 (`janus.WithProtectedRoutes`)

外部工程扩展的控制面 API 必须原生集成进基座的认证授权体系。

#### 3.1 函数式 Option 声明
在 `pkg/janus` 中增加受保护路由注入选项：
```go
// WithProtectedRoutes 注入受保护的控制面路由组，外部 Handler 自动继承基座的多租户认证与 Casbin RBAC 鉴权
func WithProtectedRoutes(fn func(rg *gin.RouterGroup)) Option
```

#### 3.2 路由树拓扑与中间件继承
注入的路由组 `rg` 挂载在基座的核心 `/api` 路由组下，自动穿透以下安全防线：
1. **全局 Panic 恢复与日志中间件** (`panicRecoverAndLog()`)：拦截未预期的 panic 并格式化为 500 JSON 错误，防止服务崩溃；
2. **多租户双轨统一认证中间件** (`authenticate()`)：
   - 自动支持 `Authorization: Bearer <JWT>` 与 `janus_session` Cookie；
   - 自动校验租户封禁状态（`status != 'banned'`）；
   - 自动校验改密版本号（`token_version`），一旦密码重置立即吊销老 Token；
3. **Casbin RBAC 授权中间件** (`authorize()`)：
   - `superadmin` 角色：直接由基座全局通配规则 `p, superadmin, /api/*, *` 自动放行；
   - `tenant` 角色：在挂载受保护路由时，基座自动将挂载的路径与方法注册进 Casbin 策略池（或为扩展路由赋予租户默认放行权限），确保合法的普通租户能够顺畅访问受保护扩展 API，而未认证请求一律被拦截并返回 401。

#### 3.3 租户上下文提取契约 (`AuthContext`)
外部 Handler 严禁穿透直接读取 `internal/store` 的私有结构，基座在 `pkg/janus` 导出标准化的认证上下文与提取助手：

```go
package janus

import "github.com/gin-gonic/gin"

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
func GetAuthContext(c *gin.Context) (*AuthContext, bool)
```

外部 Handler 可以采用以下直观方式使用：
```go
janus.WithProtectedRoutes(func(rg *gin.RouterGroup) {
    rg.POST("/cloak/campaigns", func(c *gin.Context) {
        auth, ok := janus.GetAuthContext(c)
        if !ok {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
            return
        }
        // 直接使用 auth.TenantID, auth.UserID, auth.Role
        c.JSON(http.StatusOK, gin.H{
            "tenantId": auth.TenantID,
            "userId":   auth.UserID,
            "role":     auth.Role,
        })
    })
})
```

---

## 边界与不变式 (Architectural Invariants)

1. **可见性单向铁律 (Pkg vs Internal Invariant)**：
   - 外部衍生项目仅允许引用 `pkg/janus` 与 `pkg/plugin`，禁止跨包引用 `internal/*`。
   - 所有上下文与数据模型扩展（`Metadata`、`AuthContext`、`WithProtectedRoutes`、`WithExtraMigrations`）必须在 `pkg/janus` 形成统一门面导出。
2. **安全闭环与防篡改 (Security Fail-Closed Invariant)**：
   - 受保护路由必须经过 `authenticate()` 校验，缺少凭证一律严格返回 `401 Unauthorized`；
   - 权限未满足一律严格返回 `403 Forbidden`，不发生信息泄漏。
3. **测试即唯一规格 (Uncle Bob Invariant)**：
   - 行为契约通过 `pkg/janus/protected_routes_test.go` 与 `pkg/janus/metadata_test.go` 可执行测试呈现，作为唯一的活文档。

---

## 实施阶段 (Red-Green-Refactor)

- **第一阶段：测试先行与契约确认 (Red)** [当前阶段]：
  - 编写 ADR-0014 规范；
  - 编写 `pkg/janus/protected_routes_test.go` 与 `pkg/janus/metadata_test.go` 测试规格；
  - **严禁编写任何业务实现代码**，运行测试套件确认呈预期的编译失败/红态（Red）。
- **第二阶段：最小实现与防篡改 (Green)**：
  - 新增迁移脚本 `migrations/0003_links_metadata.sql`；
  - 在 `internal/store` 与 `pkg/janus` 中实现 `Metadata`、`WithExtraMigrations`、`WithProtectedRoutes` 与 `GetAuthContext`；
  - 跑通全部机械门禁校验（Go 黑盒测试、CRAP、变异测试、E2E）。
- **第三阶段：重构与基座对齐 (Refactor)**：
  - 优化 goose 双轨迁移连接复用与日志沉淀；
  - 完善 Casbin 策略动态同步机制。
