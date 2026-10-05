# ADR-0013: 架构基座化与 App Builder 模式 (Application Base & Builder Pattern)

## 状态

已提议 (Proposed) / 阶段一契约定义

## 背景

随着 Janus 系统向「阳光合规的企业级短链开源基座 + 私有高级对抗扩展（Janus-Cloak）」架构演进（参见 [ADR-0012: 开源基座功能剥离与插件化扩展点设计规范](0012-open-core-extension-points.md)），现有单体结构在工程边界与集成方式上面临以下核心挑战：

1. **入口装配与包可见性（Internal vs Pkg）**：
   - 现有的核心逻辑、配置、数据模型与扩展点大多分散在 `internal/*` 目录下（如 `internal/rules/enricher.go`、`internal/httpapi/actions.go` 等）。
   - 根据 Go 语言规范，`internal` 目录下的代码受到编译器保护，外部仓库（例如私有衍生工程 `janus-cloak`）无法导入。私有工程若想使用基座，必须以源码侵入分支（Fork）的方式进行维护，这破坏了「基座迭代时零代码冲突无缝合并」的核心演进目标。
2. **进程启动与生命周期装配固化**：
   - 目前 `cmd/janus/main.go` 内部直接编码了配置读取、DB 迁移、超管初始化、Worker 启动、Gin 路由组装与 HTTP 信号监听。外部工程无法以库的方式引入基座并优雅嵌入私有中间件或专属自定义路由（例如私有版专属的管理 API 或 S2S 回传端点 `/postback`）。
3. **扩展点暴露层级不清**：
   - ADR-0012 定义了五大扩展点（`FactEnricher`、`RuleInterceptor`、`ActionHandler`、`SDKInjector`、`PostVisitHook`），目前它们位于 `internal/rules` 和 `internal/httpapi` 中，私有版无法直接实现接口并注册。

因此，亟需在基座中确立公开接口规范、包依赖边界，并引入 **App Builder 模式**，使基座既可以作为开箱即用的独立二进制启动，又可以作为标准化 Go 模块被外部无缝包装集成。

---

## 架构决策

### 1. 包依赖规范与可见性分层（pkg vs internal）

严格确立清晰的单向依赖与可见性拓扑：

```text
外部衍生工程 / 启动入口 (cmd/janus, cloaker-main)
                   │
                   ▼
         ┌───────────────────┐
         │     pkg/janus     │ ── App 容器构建、生命周期、函数式 Option 配置
         └─────────┬─────────┘
                   │
                   ▼
         ┌───────────────────┐
         │    pkg/plugin     │ ── 公开扩展点接口声明、注册表契约、插件上下文对象
         └─────────┬─────────┘
                   │
                   ▼ (仅基座内部单向依赖，外部禁止穿透)
         ┌───────────────────┐
         │    internal/*     │ ── 核心业务、存储引擎、规则快照、DB/网络基础设施
         └───────────────────┘
```

- **`pkg/plugin`**：
  - 纯公共契约包，零内部重型依赖（不依赖 GORM、PGX、Gin 核心路由表等）。
  - 对外暴露五大扩展点接口及注册/查询/重置 API：`FactEnricher`、`RuleInterceptor`、`ActionHandler`、`SDKInjector`、`PostVisitHook`。
  - 定义扩展点运行时数据传输对象（DTO），如 `Fact`、`Decision`、`DeliveryContext`、`VisitRecord` 等，确保外部插件只需依赖 `pkg/plugin` 即可完成编译与实现。
- **`pkg/janus`**：
  - 公开基座应用构建器。
  - 导出 `App` 结构体、`New(opts ...Option) (*App, error)` 构造函数以及标准函数式 Option（如 `WithConfig`、`WithConfigFile`、`WithDB`、`WithRoutes` 等）。
  - 提供生命周期方法：`Run(ctx context.Context) error`、`Shutdown(ctx context.Context) error`、`Handler() http.Handler`。
- **`internal/*`**：
  - 私有实现与业务细节。`internal/httpapi` 与 `internal/rules` 作为基座内部执行流水线，通过薄适配层桥接或同步 `pkg/plugin` 中注册的插件。
  - 严禁 `internal` 反向依赖 `cmd` 或外部包；外部代码严禁穿透直接调用 `internal/*`。

---

### 2. App Builder 模式设计

为了提供高内聚、易装配的基座构建体验，采用函数式选项模式（Functional Options Pattern）：

```go
package janus

// App 是 Janus 运行时基座实例
type App struct { ... }

// Option 定义 App 配置选项函数
type Option func(*options) error

// New 基于给定的选项构建并校验 App 实例
func New(opts ...Option) (*App, error)
```

#### 2.1 支持的 Option 契约：
1. **`WithConfig(cfg Config)`**：
   - 显式传入预先加载或定制的配置结构体。
   - 若未提供，则自动读取环境变量兜底。
2. **`WithConfigFile(path string)`**：
   - 指定配置文件路径加载环境变量/.env 文件。
3. **`WithDB(db *gorm.DB)`**：
   - 允许外部传入外部已建立好的数据库连接，便于测试环境注入 SQLite/PG 测试库，或大型企业复用统一连接池。
4. **`WithRoutes(fn func(engine *gin.Engine))`**：
   - 自定义路由注入器。允许外部工程在基座 Gin 路由引擎上挂载私有中间件或新增专有子路由（如挂载私有 S2S 回传端点、自定义探针端点等），而无需修改基座内部代码。

#### 2.2 构建校验约束（Validation Invariants）：
- `New(opts ...)` 执行时必须执行配置校验（如端口合法性、必填项、DB 连接串格式等）。
- 若 Option 函数执行出错（例如配置文件不存在），`New` 必须立刻返回明确的 `error`，杜绝未定义状态。

---

### 3. 生命周期契约 (Lifecycle Contract)

`App` 必须提供统一且严格的生命周期管理接口：

```go
type App struct { ... }

// Run 启动应用（包括数据库迁移、后台任务 Worker、HTTP 监听）
// 该调用是阻塞式的，直到上下文被取消 (ctx.Done()) 或遇到致命运行时错误
func (a *App) Run(ctx context.Context) error

// Shutdown 优雅关闭服务
// 关闭 HTTP 监听，等待后台任务（如证书巡检、异步日志写队列）在超时前优雅退出，回收数据库连接池
func (a *App) Shutdown(ctx context.Context) error

// Handler 暴露底层的 http.Handler
// 专门供黑盒测试 (httptest.NewServer / testutil)、边缘环境或集成入现有 HTTP 编排中直接调用
func (a *App) Handler() http.Handler
```

- **非阻塞 vs 阻塞**：`Run` 保持前台阻塞运行，与标准库 `http.Server` 配合，接收 `context.Context` 控制退出。
- **可测试性保障**：外部测试可以通过 `janus.New()` 构造实例后直接调用 `app.Handler()` 配合 `httptest.ResponseRecorder` 进行纳秒级测试，无需监听真实物理端口。

---

### 4. 与 `pkg/plugin` 的分工与协作

`pkg/plugin` 与 `pkg/janus` 各司其职，遵循单一职责原则：

| 模块 | 职责定位 | 生命周期关系 | 状态模式 |
| :--- | :--- | :--- | :--- |
| **`pkg/plugin`** | **全局扩展点契约与注册表**<br>暴露 5 大插件点及上下文数据结构 | 独立于特定 App 实例；通常在 `init()` 阶段或 `janus.New()` 之前完成注册 | 并发安全全局单例注册表（带快照读与 Reset 机制） |
| **`pkg/janus`** | **应用程序聚合与运行基座**<br>管理配置、存储、Worker、HTTP 路由流水线 | 拥有明确的 `New -> Run -> Shutdown` 状态生命周期 | 实例级对象（可支持多实例测试） |

#### 5 大插件扩展点在 `pkg/plugin` 中的契约：
1. **`FactEnricher`**：
   - `Name() string`
   - `Enrich(ctx context.Context, r *http.Request, fact *Fact)`
   - 规则求值前注入自定义访客画像（如 ASN、机房属性、外部探针标记）。
2. **`RuleInterceptor`**：
   - `Name() string`
   - `Intercept(ctx context.Context, r *http.Request, linkID int64, fact *Fact) (*Decision, bool)`
   - 规则遍历前短路判定（如 Safe Mode 纯白放行、紧急熔断）。
3. **`ActionHandler`**：
   - `func(c *gin.Context, dctx DeliveryContext) bool`
   - 自定义交付动作（如 `"proxy"` CURL 隐形反向代理代吐）。
4. **`SDKInjector`**：
   - `Name() string`
   - `WrapSDK(domain string, code string, rawSDK string) string`
   - 前端 SDK 动态包裹增强（如注入 Anti-Headless 探针）。
5. **`PostVisitHook`**：
   - `func(ctx context.Context, r *http.Request, visit VisitRecord)`
   - 访问记录生成后的异步非阻塞事件回调（如 ClickID 归因、S2S 回传）。

#### 铁律约束：
- **并发安全性**：注册表支持高并发动态读写，读操作使用快照机制，写操作使用互斥锁保护。
- **异常隔离（Fail-Open）**：任何插件在执行过程中抛出 `panic`，注册表调度器必须 `recover` 并记录日志，决不能导致访客 HTTP 请求崩溃（返回 500）或后台进程崩溃退出。
- **测试隔离（Reset）**：提供 `Reset()` 方法清空注册表状态，确保测试用例之间无状态污染。

---

## 阶段执行与实施路线 (Red-Green-Refactor)

为严格落实 Uncle Bob 的「测试即规格」与 AGENTS.md 规范：

1. **第一阶段：测试先行与契约确认 (Red)** [当前阶段]：
   - 编写 ADR-0013 确定基座与 App Builder 规范。
   - 编写 `pkg/plugin/plugin_test.go` 与 `pkg/janus/builder_test.go` 测试规格。
   - **严禁编写任何业务实现代码**，运行测试确认编译失败/红态（Red）。
   - 由人类审查断言完备性与契约边界。
2. **第二阶段：最小实现与防篡改 (Green)**：
   - 实现 `pkg/plugin` 与 `pkg/janus`，保持原有 internal 兼容迁移。
   - 跑通所有质量门禁（Go 黑盒测试、CRAP、变异测试、E2E）。
3. **第三阶段：重构与清理 (Refactor)**：
   - 收敛 `cmd/janus/main.go`，改为通过 `janus.New` 启动。
   - 完成 internal 扩展点到 pkg 扩展点的平滑委托。
