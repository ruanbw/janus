# ADR-0015: 轻量进程内事件总线与生命周期解耦 (In-Process Event Bus & Domain Events)

## 状态

已通过 (Accepted)

## 背景

在将 Janus 重构为通用应用基座后，外部二次开发系统（如营销计费系统、风控清洗系统、第三方告警或数据同步）经常需要在核心业务发生状态变化时感知并执行扩展逻辑（例如：短链创建后扣减点数、域名激活后通知用户、租户注册后初始化外部工作区等）。

若没有统一的事件解耦机制，二开开发者只能在原有业务代码中不断嵌入侵入式的业务调用，造成代码耦合甚至死锁。

因此，基座需要一个**轻量、并发安全、具备 Fail-Open 隔离机制的进程内事件总线（In-Process Event Bus）**。

---

## 架构决策

### 1. 事件定义规范 (`pkg/event`)

- **领域事件主题 (Topic)** 命名规范：`<entity>.<action>`，例如：
  - `link.created` (`EventLinkCreated`)
  - `link.deleted` (`EventLinkDeleted`)
  - `domain.verified` (`EventDomainVerified`)
  - `domain.failed` (`EventDomainFailed`)
  - `tenant.registered` (`EventTenantRegistered`)
- **事件结构 (Event Interface / Struct)**：
  - `Topic() string`
  - `OccurredAt() time.Time`
  - `Payload() any`

### 2. 进程内事件总线 (`Bus`)

- 提供并发安全的 `Subscribe(topic string, handler Handler)` 与 `Publish(ctx context.Context, e Event)`。
- **Fail-Open 铁律**：事件订阅者是在主流程之外异步或隔离执行的。任何订阅者内部发生的 `panic` 必须被总线内部捕获并记录日志，绝不允许带崩主线程或阻断主业务事务。
- 提供 `Reset()` 方法供单元测试状态隔离使用。

### 3. App Builder 挂载点

- `janus.WithEventListener(topic string, handler func(ctx context.Context, e event.Event))` 允许在 App 构建时声明式注入外部订阅逻辑。
- 在 `internal/store` 或 `internal/httpapi` 中，在成功操作（如短链创建、删除等）后派发对应事件。
