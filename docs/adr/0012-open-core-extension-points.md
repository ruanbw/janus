# Janus 开源基座功能剥离与插件化扩展点设计规范

> **文档性质**：系统演进设计规范与实施计划  
> **核心目标**：将当前 `janus` 项目重构并收敛为**100% 阳光合规的高性能通用企业级短链开源基座**；同时定义与预埋五大非侵入式插件化扩展点，使私有高级版（`Janus-cloak` 斗篷/TDS 网关）能够通过外部注册注入高级对抗能力，并在基座迭代时实现**零代码冲突无缝合并**。

---

## 目录
1. [执行原则与架构定位](#一-执行原则与架构定位)
2. [基座必须剥离的功能清单与边界界定](#二-基座必须剥离的功能清单与边界界定)
   - [2.1 现有代码中需剔除/重构的模块](#21-现有代码中需剔除重构的模块)
   - [2.2 严禁并入基座的高级商业与对抗功能](#22-严禁并入基座的高级商业与对抗功能)
3. [基座保留核心能力一览与正当性说明](#三-基座保留核心能力一览与正当性说明)
4. [基座五大核心插件化扩展点详细设计](#四-基座五大核心插件化扩展点详细设计)
   - [扩展点 1：访客画像富化器 (FactEnricher)](#扩展点-1-访客画像富化器-factenricher)
   - [扩展点 2：前置裁决拦截器 (RuleInterceptor)](#扩展点-2-前置裁决拦截器-ruleinterceptor)
   - [扩展点 3：交付动作处理器注册表 (ActionHandler)](#扩展点-3-交付动作处理器注册表-actionhandler)
   - [扩展点 4：前端 SDK 注入钩子 (SDKInjector)](#扩展点-4-前端-sdk-注入钩子-sdkinjector)
   - [扩展点 5：后置访问与归因钩子 (PostVisitHook)](#扩展点-5-后置访问与归因钩子-postvisithook)
5. [扩展点在 HTTP 请求流水线中的拓扑流向](#五-扩展点在-http-请求流水线中的拓扑流向)
6. [热路径性能铁律与稳定性约束 (Hot Path Invariants)](#六-热路径性能铁律与稳定性约束-hot-path-invariants)
7. [基座改造实施详细步骤与检视清单](#七-基座改造实施详细步骤与检视清单)

---

## 一、 执行原则与架构定位

为保证开源基座作为通用短链在 GitHub 等公开平台获得社区信任与合规背书，同时确保商业版享有坚实的技术壁垒：

1. **公开基座定位**：自托管、多租户、现代化企业级短链分发网关。强调自带域名（BYOD）、自动化 TLS（Caddy On-Demand）、纯内存高性能规则路由、静态落地页托管（ADR-0005）与运营审计看板。
2. **私有高级版定位**：面向跨境投放、高对抗风控清洗与 ROI 归因闭环的专业斗篷（Cloaker）与流量分发网关（TDS）。
3. **架构解耦铁律**：
   - **单向依赖**：私有版依赖并包装基座，基座代码绝对不出现任何 `cloak`、`antidetect`、`spider`、`proxy_curl` 等特定商业业务字眼。
   - **注册表模式（Registration Pattern）**：基座只暴露公共 Go 接口与全局并发安全注册表；未注册插件时，基座纯零开销、100% 走原生轻量逻辑。

---

## 二、 基座必须剥离的功能清单与边界界定

### 2.1 现有代码中需剔除/重构的模块

经过对当前代码审查，当前代码库中存在若干针对外部高级风控/机房判定的实现与硬编码逻辑，需要进行彻底剥离或重构：

| 模块 / 源码路径 | 现状分析 | 剥离与改造方案 |
| :--- | :--- | :--- |
| **外部商业风控情报 SaaS 数据源**<br>`internal/geo/ipinfo.go`<br>`internal/geo/ipqualityscore.go`<br>`internal/geo/ipapi.go` | 这 3 个文件实现了外部商业风控平台的 HTTP 查询，直接解析提取 `Hosting`、`Proxy`、`VPN`、`Tor`、`IsCrawler` 等反爬/防审对抗属性。 | **彻底从基座中剔除**。<br>基座仅保留纯离线、开源且双许可（Apache-2.0/MIT）的 `ip2region`（`xdb.go`）以提供合规的国家码解析。高级情报查询移交给扩展点 `FactEnricher` 由私有版实现。 |
| **配置项与初始化分支**<br>`internal/config/config.go`<br>`internal/httpapi/server.go` | `JANUS_GEO_PROVIDER` 支持配置 `ipinfo` / `ipqualityscore` / `ipapi`，且带有 `JANUS_GEO_API_KEY` 与超时控制。 | **收敛配置**。<br>基座移除上述商业服务配置项，仅保留离线库机制。SaaS 接入改由高级版插件启动时按需读取自身私有配置。 |
| **规则校验中硬编码的 ASN 占位符特判**<br>`internal/httpapi/rules.go`<br>`internal/rules/fields.go`<br>`internal/rules/evaluator.go` | 源码中因目前无数据源，在多处硬编码了诸如：`asn 字段当前没有数据源, 依赖它的条件恒不命中, 不能用于规则`。 | **重构字段体系**。<br>基座原生只声明其具备数据源的合规基础字段（`country`、`device`、`os`、`browser`、`lang`、`ref`、`utm_*`）。将高阶字段准入与提取解耦，通过 `FactEnricher` 动态声明/注入，移除死代码特判。 |
| **机房属性字段占位**<br>`internal/geo/geo.go` (`IsDatacenter`)<br>`internal/store/visits.go` (`is_datacenter`) | 基座目前在访问明细表中保留了 `is_datacenter` 列，且当前离线库无法填充，恒为空或 `false`。 | **字段收窄**。<br>基座访问明细表移除或不再主动宣传机房拦截列，避免开源代码被代码审计归类为灰产斗篷雏形。 |

---

### 2.2 严禁并入基座的高级商业与对抗功能

以下特性属于典型的斗篷（Cloaker/TDS）对抗核心资产，**坚决不得出现在基座源码中**，统一由私有衍生工程在 `internal/cloak/` 专属目录内实现：

1. **离线高精情报库与敏感黑名单**：
   - MaxMind GeoLite2-ASN 离线库；
   - AWS / GCP / Cloudflare / Hetzner / DigitalOcean 等云厂商机房 ASN 拦截列表；
   - Meta / Google / TikTok 官方广告审核中心爬虫网段与节点 IP 黑名单。
2. **反无头浏览器（Anti-Headless）与客户端环境探针**：
   - 前端采集检测 `navigator.webdriver === true` 自动化标识；
   - 提取 Canvas / WebGL 软渲染（SwiftShader）特征；
   - 客户端系统时区（`Intl.DateTimeFormat`）与服务端 IP 归属地时区一致性比对。
3. **CURL 服务端反向代理隐形交付（Proxy 模式）**：
   - 外部浏览器地址栏 URL 维持合规白域名不变，Go 后端在服务器内向 Money Page（敏感落地页）发起异步 HTTP 拉取，替换静态资源路径后以 200 OK 直接渲染输出，彻底规避审核爬虫抓包 302 重定向终点。
4. **营销归因闭环 (ClickID 透传 & S2S Postback & CAPI)**：
   - 捕获 `fbclid`、`gclid`、`ttclid` 参数并生成全局唯一短链点击追踪 `click_id`；
   - 暴露对外 `/postback` 接口接收下游转化，并调用 Meta CAPI / Google Offline Conversions API 将事件回传。
5. **审核期流控状态机 (Safe Mode) 与一键熔断 (Panic Switch)**：
   - 广告投放生命周期状态机：新活动新建前 N 小时 100% 强制放行（输出合规页面）；
   - 风控预警时的一键全局阻断（全量返回 404 或白页）。

---

## 三、 基座保留核心能力一览与正当性说明

开源基座保留以下功能，确保作为通用企业级短链具备行业极高的竞争力：

| 模块 | 包含能力 | 合规与商业正当性 |
| :--- | :--- | :--- |
| **租户与权限体系** | 邮箱注册验证、会话/API Bearer JWT 认证、租户配额隔离、Casbin RBAC 超管治理 | 企业内部多部门或合规多团队自建短链 SaaS 的标准基建。 |
| **自带域名 (BYOD)** | 租户绑定独立域名、DNS TXT/A 记录校验、Caddy On-Demand TLS 自动申请与续期 | 彻底解决自托管短链域名证书难维护的痛点，完全合规。 |
| **基础短链核心** | 自定义短码、目标池 Round-Robin 轮询分流、合规 301/302 重定向 | 通用短链网关基础核心。 |
| **静态落地页托管** | Zip 上传解压、静态文件安全分发、内置 `/{code}/click` 跳转端点（ADR-0005） | 类似轻量 Netlify/Vercel，支持企业合规营销宣传页一站式托管。 |
| **纯内存规则引擎** | 纳秒级纯内存快照求值、Fail-open 兜底；支持合规维度（国家/语言/系统/设备/Referrer）路由 | 解决跨国电商“不同语言跳不同站”、“不同设备跳不同应用商店”的正当需求。 |
| **审计与统计大屏** | 访问明细流水日志（`visits`）、全球访客分布大屏、设备/系统统计看板 | 标准运营数据分析与可观测性看板。 |

---

## 四、 基座五大核心插件化扩展点详细设计

为了保证基座升级时，私有衍生项目能够通过标准依赖注入扩展能力，基座预埋 5 个扩展点：

```text
                客户端发起 GET /{code} 请求
                             │
                             ▼
                    [1. 域名与短链定位]
                             │
                             ▼
              ★ 扩展点 1: FactEnricher (画像富化)
             (Cloak 注入: ASN、机房属性、探针 Token)
                             │
                             ▼
              ★ 扩展点 2: RuleInterceptor (前置拦截)
             (Cloak 注入: Safe Mode 纯白放行 / 一键熔断)
                             │ (未短路拦截)
                             ▼
                    [3. 内存规则快照求值]
                             │
                             ▼
              ★ 扩展点 3: ActionHandler (动作执行)
             (基座: 302/404/429; Cloak 注入: CURL 反代)
                             │
                             ▼
              ★ 扩展点 4: SDKInjector (前端探针)
             (访问 /{code}/sdk.js 时包裹反爬探针)
                             │
                             ▼
              ★ 扩展点 5: PostVisitHook (后置归因)
             (Cloak 注入: ClickID 映射、S2S 转化回传)
```

---

### 扩展点 1: 访客画像富化器 (FactEnricher)

- **文件定位**：`internal/rules/enricher.go`
- **调用位置**：`internal/httpapi/redirect.go` 的 `ruleDecision()` 中。
- **触发时机**：基座基础 `Fact`（国家、UA、语言、设备等）构造完成之后、规则快照求值之前。
- **接口与注册表规范**：

```go
package rules

import (
	"context"
	"net/http"
	"sync"
)

// FactEnricher 允许外部在规则求值前向 Fact 注入额外的扩展画像字段。
// 铁律：实现必须是纯内存或本地离线库操作，耗时必须在微秒级，严禁引入数据库查询或网络 RPC。
type FactEnricher interface {
	Name() string
	Enrich(ctx context.Context, r *http.Request, fact *Fact)
}

var (
	enrichersMu sync.RWMutex
	enrichers   []FactEnricher
)

// RegisterEnricher 注册外部画像富化器
func RegisterEnricher(e FactEnricher) {
	enrichersMu.Lock()
	defer enrichersMu.Unlock()
	enrichers = append(enrichers, e)
}

// ApplyEnrichers 对 Fact 执行所有已注册的画像富化器
func ApplyEnrichers(ctx context.Context, r *http.Request, fact *Fact) {
	enrichersMu.RLock()
	defer enrichersMu.RUnlock()
	for _, e := range enrichers {
		e.Enrich(ctx, r, fact)
	}
}
```

- **职责界限**：
  - **基座**：仅提取基础国家码（`ip2region`）、设备分类、操作系统、语言、Referrer。
  - **Cloak 私有版**：通过 `RegisterEnricher` 接入 `GeoLite2-ASN.mmdb` 离线库，注入 `ASN`、机房标记及前端探针回传 Token。

---

### 扩展点 2: 前置裁决拦截器 (RuleInterceptor)

- **文件定位**：`internal/rules/interceptor.go`
- **调用位置**：`internal/httpapi/redirect.go` 的 `ruleDecision()` 中，规则快照求值前。
- **触发时机**：在定位到短链后，决定是否短路整个规则求值流程。
- **接口与注册表规范**：

```go
package rules

import (
	"context"
	"net/http"
	"sync"
)

// RuleInterceptor 允许在遍历规则快照之前短路求值逻辑，直接给出最终决议。
type RuleInterceptor interface {
	Name() string
	// Intercept 返回 (*Decision, true) 表示短路生效，系统将跳过后续规则匹配直接采用该决议；
	// 返回 (nil, false) 则表示放行，继续进行后续规则求值。
	Intercept(ctx context.Context, r *http.Request, linkID int64, fact *Fact) (*Decision, bool)
}

var (
	interceptorsMu sync.RWMutex
	interceptors   []RuleInterceptor
)

// RegisterInterceptor 注册前置规则拦截器
func RegisterInterceptor(i RuleInterceptor) {
	interceptorsMu.Lock()
	defer interceptorsMu.Unlock()
	interceptors = append(interceptors, i)
}

// CheckInterceptors 依次执行前置拦截器，首个命中的拦截器将短路后续流程
func CheckInterceptors(ctx context.Context, r *http.Request, linkID int64, fact *Fact) (*Decision, bool) {
	interceptorsMu.RLock()
	defer interceptorsMu.RUnlock()
	for _, i := range interceptors {
		if d, ok := i.Intercept(ctx, r, linkID, fact); ok && d != nil {
			return d, true
		}
	}
	return nil, false
}
```

- **职责界限**：
  - **基座**：默认不注册拦截器，所有流量正常穿透到内存快照求值。
  - **Cloak 私有版**：注册 `SafeModeInterceptor`（广告审核期强制白页放行）与 `PanicInterceptor`（一键紧急熔断，强制全量 404/白页拦截）。

---

### 扩展点 3: 交付动作处理器注册表 (ActionHandler)

- **文件定位**：`internal/httpapi/actions.go`
- **调用位置**：`internal/httpapi/redirect.go` 的 `applyRuleDecision()`。
- **触发时机**：规则裁决完成，向客户端响应内容时。
- **接口与注册表规范**：

```go
package httpapi

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"

	"janus/internal/rules"
	"janus/internal/store"
)

// DeliveryContext 传递给交付动作处理器的上下文信息
type DeliveryContext struct {
	Link     *store.Link
	Domain   *store.Domain
	Decision rules.Decision
	Target   string
}

// ActionHandler 执行具体的 HTTP 内容交付（如重定向、渲染错误页或服务端代吐）
type ActionHandler func(c *gin.Context, dctx DeliveryContext)

var (
	actionHandlersMu sync.RWMutex
	actionHandlers   = make(map[string]ActionHandler)
)

// RegisterActionHandler 允许外部注册新型交付动作处理器（例如 "proxy"）
func RegisterActionHandler(action string, handler ActionHandler) {
	actionHandlersMu.Lock()
	defer actionHandlersMu.Unlock()
	actionHandlers[action] = handler
}

// GetActionHandler 获取指定动作的处理函数，不存在时返回 nil
func GetActionHandler(action string) ActionHandler {
	actionHandlersMu.RLock()
	defer actionHandlersMu.RUnlock()
	return actionHandlers[action]
}
```

- **职责界限**：
  - **基座**：默认内置 `redirect` (301/302 重定向)、`notfound` (404 页面)、`throttle` (429 页面)。
  - **Cloak 私有版**：注册 `"proxy"` 动作处理器，执行 CURL 服务端反向代理隐形代吐。

---

### 扩展点 4: 前端 SDK 注入钩子 (SDKInjector)

- **文件定位**：`internal/httpapi/sdk_hook.go`
- **调用位置**：`internal/httpapi/landing.go` 的 `handleLandingSDK()`。
- **触发时机**：落地页请求 `GET /{code}/sdk.js` 准备输出 JS 脚本时。
- **接口与注册表规范**：

```go
package httpapi

import "sync"

// SDKInjector 允许对基座原生生成的 sdk.js 进行包装与增强
type SDKInjector interface {
	Name() string
	WrapSDK(domain string, code string, rawSDK string) string
}

var (
	sdkInjectorMu sync.RWMutex
	activeSDKInjector SDKInjector
)

// SetSDKInjector 设置活跃的 SDK 注入器
func SetSDKInjector(injector SDKInjector) {
	sdkInjectorMu.Lock()
	defer sdkInjectorMu.Unlock()
	activeSDKInjector = injector
}

// WrapGeneratedSDK 执行包装逻辑，未注入时原样返回基座 SDK
func WrapGeneratedSDK(domain, code string, rawSDK string) string {
	sdkInjectorMu.RLock()
	defer sdkInjectorMu.RUnlock()
	if activeSDKInjector != nil {
		return activeSDKInjector.WrapSDK(domain, code, rawSDK)
	}
	return rawSDK
}
```

- **职责界限**：
  - **基座**：仅输出原生透明合规的点击上报 SDK（`window.Janus.click()`）。
  - **Cloak 私有版**：在 SDK 中包裹高对抗反爬探针脚本（Anti-Headless、Canvas 软渲染检测、时区核验加密 Payload）。

---

### 扩展点 5: 后置访问与归因钩子 (PostVisitHook)

- **文件定位**：`internal/httpapi/hooks.go`
- **调用位置**：`internal/httpapi/redirect.go` 与 `landing.go` 的 `recordVisit()` 执行完毕后。
- **触发时机**：异步触发，通过独立 goroutine 执行，完全不阻塞访客跳转响应。
- **接口与注册表规范**：

```go
package httpapi

import (
	"context"
	"net/http"
	"sync"

	"janus/internal/store"
)

// PostVisitHook 访问日志持久化完成后的异步回调
type PostVisitHook func(ctx context.Context, r *http.Request, visit *store.VisitRecord)

var (
	postHooksMu sync.RWMutex
	postHooks   []PostVisitHook
)

// RegisterPostVisitHook 注册后置访问钩子
func RegisterPostVisitHook(hook PostVisitHook) {
	postHooksMu.Lock()
	defer postHooksMu.Unlock()
	postHooks = append(postHooks, hook)
}

// TriggerPostVisitHooks 异步触发全部注册的后置钩子
func TriggerPostVisitHooks(ctx context.Context, r *http.Request, visit *store.VisitRecord) {
	postHooksMu.RLock()
	if len(postHooks) == 0 {
		postHooksMu.RUnlock()
		return
	}
	hooks := make([]PostVisitHook, len(postHooks))
	copy(hooks, postHooks)
	postHooksMu.RUnlock()

	// 必须异步执行，绝对不拖慢访客跳转热路径
	go func() {
		for _, h := range hooks {
			h(ctx, r, visit)
		}
	}()
}
```

- **职责界限**：
  - **基座**：仅完成单行 `visits` 访问记录落库，不执行任何外部通知。
  - **Cloak 私有版**：注册 `AttributionHook`，提取 `fbclid` / `gclid`，建立 ClickID 映射表；在下游 Postback 时调用 Meta CAPI / Google Conversions 回传。

---

## 五、 扩展点在 HTTP 请求流水线中的拓扑流向

```text
                 客户端发起 GET /{code} 请求
                              │
                              ▼
                     [1. 域名解析与短链查找]
                              │ (短链有效且启用规则)
                              ▼
               ★ 扩展点 1: FactEnricher (画像富化)
              ------------------------------------
              基座提取: Country, UA, OS, Browser, Lang
              Cloak注入: ASN, IsDatacenter, Fingerprint
                              │
                              ▼
               ★ 扩展点 2: RuleInterceptor (前置拦截)
              ---------------------------------------
              Cloak注入: Safe Mode (纯白放行) / 紧急熔断
                              │
              ┌───────────────┴───────────────┐
      (短路命中拦截)                    (未触发拦截: 放行)
              │                               │
              │                               ▼
              │                     [3. 内存规则快照求值]
              │                     (逐条规则纳秒级匹配)
              │                               │
              └───────────────┬───────────────┘
                              │ (得出终审决议 Decision)
                              ▼
               ★ 扩展点 3: ActionHandler (动作执行)
              ------------------------------------
              基座处理: 301/302 重定向、404、429
              Cloak处理: CURL 服务端反向代理 (Proxy 模式)
                              │
                              ▼
               ★ 扩展点 5: PostVisitHook (后置归因)
              ------------------------------------
              异步协程触发: ClickID 映射绑定、CAPI 转化回传
                              │
                              ▼
                     响应交付访客浏览器完成
```

---

## 六、 热路径性能铁律与稳定性约束 (Hot Path Invariants)

所有扩展点与插件实现必须严格恪守以下三大性能不变式：

1. **绝对零 DB 查询 (Zero DB Query Invariant)**：
   - 规则求值、画像富化（`FactEnricher`）只能查**内存常驻数据结构或本地 MMDB 二进制树**，绝对不允许在请求热路径中执行 SQL 查库或发起网络 RPC。富化耗时必须控制在 50 微秒以内。
2. **绝对零写放大 (Zero Write Amplification Invariant)**：
   - 规则命中和拦截仅落一行访问流水，严禁执行计数器累加更新。所有的商业归因、S2S 事件回传必须交由 `PostVisitHook` 异步 goroutine 处理。
3. **Fail-Open 兜底保障 (Fail-Open Principle)**：
   - 任何扩展点执行过程中如遇内部 `panic` 或异常，必须内部捕获并记录日志，一律**静默退化为基座原生默认逻辑**，绝对不允许因为插件异常向访客抛出 500 内部服务错误。

---

## 七、 基座改造实施详细步骤与检视清单

实施基座净化与扩展点改造的具体任务拆解：

```text
[任务清单]
├── 步骤 1: 剥离 geo 外部商业 SaaS 依赖 (移除 ipinfo/ipapi/ipqualityscore)
├── 步骤 2: 规则引擎与画像字段解耦 (移除 hardcoded ASN 恒空特判)
├── 步骤 3: 新增 internal/rules/enricher.go 与 interceptor.go
├── 步骤 4: 新增 internal/httpapi/actions.go, sdk_hook.go 与 hooks.go
├── 步骤 5: 在 redirect.go, eval.go, landing.go 中挂载扩展点
└── 步骤 6: 编写扩展点独立单元测试，运行机械门禁校验保证零回退
```

| 步骤 | 任务目标 | 核心涉及文件 | 验收标准 |
| :---: | :--- | :--- | :--- |
| **Step 1** | **剥离外部商业风控 SaaS** | `internal/geo/ipinfo.go`<br>`internal/geo/ipqualityscore.go`<br>`internal/geo/ipapi.go`<br>`internal/config/config.go` | 删除上述 3 个商业 SaaS 文件；`config.go` 移除 `JANUS_GEO_PROVIDER` 中针对商业平台的配置；基座仅保留 `xdb.go` 离线库。 |
| **Step 2** | **清理硬编码字段补丁** | `internal/rules/fields.go`<br>`internal/httpapi/rules.go` | 清理源码中关于“ASN 恒空”等硬编码错误提示特判，规则校验完全对齐当前基座有效字段。 |
| **Step 3** | **预埋规则层扩展点** | `internal/rules/enricher.go`<br>`internal/rules/interceptor.go` | 定义 `FactEnricher` 与 `RuleInterceptor` 接口及并发安全注册表；在规则求值入口挂载。 |
| **Step 4** | **预埋交付与归因扩展点** | `internal/httpapi/actions.go`<br>`internal/httpapi/sdk_hook.go`<br>`internal/httpapi/hooks.go` | 定义 `ActionHandler`、`SDKInjector`、`PostVisitHook` 接口与并发安全注册表；在交付路径挂载。 |
| **Step 5** | **热路径接入与防篡改验证** | `internal/httpapi/redirect.go`<br>`internal/httpapi/landing.go` | 将原有硬编码分发替换为调用注册表；在未注册任何插件的基座环境下，行为与原有逻辑 100% 保持一致。 |
| **Step 6** | **测试验证** | `internal/rules/*_test.go`<br>`internal/httpapi/*_test.go` | 编写扩展点注册、并发调用及 Fail-open 兜底单测；确保通过全部黑盒测试与质量门禁。 |
