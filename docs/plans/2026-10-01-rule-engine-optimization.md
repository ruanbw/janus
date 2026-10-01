# Rule Engine Optimization & Two-Tier Architecture Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Optimize Janus's rule engine for zero-allocation lazy evaluation on the redirect hot path, implement unified server-side rule simulation API to eliminate frontend drift, abstract a two-tier evaluation architecture (upgraded native AST engine + pluggable `expr-lang/expr` bytecode VM), and support nested condition logic and radix tree IP matching.

**Architecture:** 
A two-tier hybrid architecture (Tier-1 Native AST + Tier-2 Expr VM) sharing a unified `ConditionEvaluator` interface within the in-memory snapshot cache. The redirect hot path uses `LazyVisitorContext` with bitmask caching and `net/netip.Addr` to eliminate eager UA scanning and heap allocations. Server-side trace simulation executes identical evaluation logic for admin previews, eliminating frontend duplication.

**Tech Stack:** Go 1.22+, `net/netip`, Gin, GORM, PostgreSQL (JSONB), `github.com/expr-lang/expr`, Vue 3, TypeScript.

**Spec:** `.scratch/rule-engine-optimization/spec.md`

## Global Constraints

- First-match-wins semantics (ADR 0008) MUST be strictly preserved across all rule types and evaluators.
- Scope isolation (ADR 0008) MUST be preserved: scoped rules with zero links never match.
- Rule evaluation MUST remain pure in-memory zero IO (ADR 0009): GeoIP injected before evaluation.
- Fail-open invariant MUST be guaranteed: any runtime error/panic in rule evaluation must fail-open (pass through) and never return 500 on visitor routes.
- Backward compatibility: existing database `conditions` JSONB rows MUST continue to parse and evaluate without mandatory data migration.
- Container builds MUST remain fully functional under `CGO_ENABLED=0` Alpine static builds.

## Review Focus

- Requests with IPv6 addresses and IPv6 CIDRs MUST be correctly evaluated with zero heap allocation.
- Visitor requests without any UA/device rules MUST completely skip UA parsing, lowercasing, and keyword scanning.
- Nested condition trees `(A AND B) OR (C AND D)` and `NOT (E)` MUST evaluate according to boolean logic while flat legacy condition arrays continue working seamlessly.
- Simulation API (`/api/rules/simulate`) MUST produce deterministic evaluation traces matching actual visitor redirect behavior.
- High-concurrency evaluation of tenant snapshots MUST remain data-race free under `go test -race`.

---

### Task 1: 核心热路径零分配与惰性上下文 (LazyVisitorContext & netip.Addr)

**Files:**
- Modify: `internal/rules/fields.go`
- Modify: `internal/rules/fields_test.go`
- Modify: `internal/httpapi/redirect.go:160-195`

**Interfaces:**
- Produces:
  - `type VisitorContext interface`
  - `type LazyVisitorContext struct`
  - `func AcquireVisitorContext(r *http.Request, country, asn string) *LazyVisitorContext`
  - `func ReleaseVisitorContext(ctx *LazyVisitorContext)`
  - `func (c *LazyVisitorContext) ClientIP() netip.Addr`
  - `func (c *LazyVisitorContext) Field(name string) (string, bool)`
- Consumes:
  - `net/netip` (Go 1.18+)
  - `*http.Request`

- [ ] **Step 1: Write the failing tests in `internal/rules/fields_test.go`**
  Add unit tests verifying `LazyVisitorContext`:
  - Verify UA parsing is skipped if device fields are never queried;
  - Verify `netip.Addr` parsing correctly handles IPv4, IPv6, loopback, private, and link-local;
  - Verify X-Forwarded-For is parsed without allocations when behind trusted private proxy;
  - Verify context acquire & release from `sync.Pool`.
- [ ] **Step 2: Run test to verify it fails**
  Run: `go test -v ./internal/rules -run "TestLazyVisitorContext"`
  Expected: FAIL with undefined types/methods.
- [ ] **Step 3: Implement `LazyVisitorContext` and `netip.Addr` integration in `internal/rules/fields.go`**
  - Define `LazyVisitorContext` with bitmask flags (`flagIP`, `flagUA`, `flagDevType`, etc.);
  - Implement zero-alloc IP extraction returning `netip.Addr`;
  - Implement lazy UA evaluation only on first access to `devtype`, `os`, `browser`;
  - Implement `sync.Pool` for `LazyVisitorContext`.
- [ ] **Step 4: Update `internal/httpapi/redirect.go` to use `AcquireVisitorContext` and `ReleaseVisitorContext`**
  Ensure context is acquired before `snap.Evaluate` and deferred released.
- [ ] **Step 5: Run tests to verify they pass**
  Run: `go test -v ./internal/rules ./internal/httpapi -run "TestLazyVisitorContext|TestFromRequest|TestRuleDecision"`
  Expected: PASS
- [ ] **Step 6: Commit**
  ```bash
  git add internal/rules/fields.go internal/rules/fields_test.go internal/httpapi/redirect.go
  git commit -m "perf(rules): implement LazyVisitorContext and netip.Addr zero-alloc extraction"
  ```

---

### Task 2: 运算符哈希优化与基准测试验证 (Operator Map Lookup & Benchmarks)

**Files:**
- Modify: `internal/rules/eval.go`
- Modify: `internal/rules/eval_test.go`

**Interfaces:**
- Produces:
  - `compiledCond.litMap map[string]struct{}` for $O(1)$ set membership test in `OpIn` / `OpNotIn`
  - Fast-path for ASCII equality and substring checks
- Consumes:
  - `VisitorContext`

- [ ] **Step 1: Write benchmarks in `internal/rules/eval_test.go`**
  Add `BenchmarkLazyEvaluate` comparing lazy vs eager evaluation on typical rules (e.g., country-only vs device rules).
- [ ] **Step 2: Implement pre-compiled map indexing in `compileCond` in `internal/rules/eval.go`**
  - For `OpIn` / `OpNotIn`, compile string values into `litMap: map[string]struct{}`;
  - In `matchSet`, if `c.litMap != nil`, perform $O(1)$ lookup with `strings.ToLower(raw)`;
  - Adapt `match` to accept `VisitorContext` (with backward compatibility for `*Fact`).
- [ ] **Step 3: Run all unit and benchmark tests**
  Run: `go test -v ./internal/rules -run . -bench=. -benchmem`
  Expected: All tests pass, allocations per op remain 0 B/op on evaluate path.
- [ ] **Step 4: Commit**
  ```bash
  git add internal/rules/eval.go internal/rules/eval_test.go
  git commit -m "perf(rules): optimize set operators with pre-compiled hash maps"
  ```

---

### Task 3: 服务端仿真 API 与 Trace 收集器 (Simulation API & Trace Collector)

**Files:**
- Modify: `internal/rules/eval.go`
- Modify: `internal/rules/snapshot.go`
- Modify: `internal/httpapi/rules.go`
- Test: `internal/httpapi/rules_test.go`
- Test: `internal/rules/snapshot_test.go`

**Interfaces:**
- Produces:
  - `type ConditionTrace struct`
  - `type StepTrace struct`
  - `type SimulationResult struct`
  - `func (s *Snapshot) Simulate(ctx VisitorContext, linkID int64, onlyRuleID *int64, draft *Compiled) SimulationResult`
  - Endpoint `POST /api/rules/simulate`
- Consumes:
  - `rules.Snapshot`
  - Store rule queries

- [ ] **Step 1: Write failing tests for rule simulation in `internal/rules/snapshot_test.go` and `internal/httpapi/rules_test.go`**
  - Test `Snapshot.Simulate` records detailed per-condition match traces (actual vs expected);
  - Test `POST /api/rules/simulate` validates inputs and returns 200 with `facts`, `steps`, and `verdict`;
  - Test simulation with `onlyRuleId` and with a draft rule override.
- [ ] **Step 2: Run test to verify it fails**
  Run: `go test -v ./internal/rules -run "TestSnapshotSimulate"`
  Expected: FAIL with undefined `Simulate`.
- [ ] **Step 3: Implement `Simulate` method on `Snapshot` in `internal/rules/snapshot.go`**
  Iterate rules, record `ConditionTrace` (field, actual, operator, expected, matched, description), step status (`hit`, `skip`, `disabled`), and generate `Decision` on first match.
- [ ] **Step 4: Implement handler `handleSimulateRules` in `internal/httpapi/rules.go`**
  Register `r.POST("/rules/simulate", a.handleSimulateRules)`:
  - Parse request: URL, IP, UserAgent, AcceptLanguage, Referer, ManualCountry, OnlyRuleID, DraftRule;
  - Resolve link ID from URL host & path if matched;
  - Acquire snapshot and run `Simulate`;
  - Return formatted JSON response.
- [ ] **Step 5: Run tests to verify they pass**
  Run: `go test -v ./internal/rules ./internal/httpapi -run "TestSnapshotSimulate|TestSimulateRules"`
  Expected: PASS
- [ ] **Step 6: Commit**
  ```bash
  git add internal/rules/ internal/httpapi/
  git commit -m "feat(rules): implement server-side rule simulation API with condition tracing"
  ```

---

### Task 4: 前端模拟器接入统一服务端 API

**Files:**
- Modify: `web/src/api/rules.ts`
- Modify: `web/src/views/rules/RuleSimulatorView.vue`
- Modify: `web/src/views/rules/ruleSim.ts`

**Interfaces:**
- Produces:
  - `export function simulateRules(data: SimulateRulesReq): Promise<SimulateRulesResp>`
- Consumes:
  - `POST /api/rules/simulate`

- [ ] **Step 1: Add API client method in `web/src/api/rules.ts`**
  Define request/response TypeScript interfaces matching backend `SimulateRequest` and `SimulateResponse`.
- [ ] **Step 2: Update `RuleSimulatorView.vue` to invoke `simulateRules`**
  Replace local TypeScript execution with API call to backend `/api/rules/simulate`.
  Keep frontend rendering of `VisitorFacts`, `TraceStep` timeline, and preview modal unchanged.
- [ ] **Step 3: Verify TypeScript builds without errors**
  Run: `pnpm --dir web build` (or `vue-tsc --noEmit`)
  Expected: Zero type errors.
- [ ] **Step 4: Commit**
  ```bash
  git add web/src/api/rules.ts web/src/views/rules/RuleSimulatorView.vue web/src/views/rules/ruleSim.ts
  git commit -m "feat(web): connect RuleSimulatorView to server-side simulation API"
  ```

---

### Task 5: 复合条件树 AST 与向后兼容 JSONB (Condition AST & Backward Compatibility)

**Files:**
- Modify: `internal/store/rules.go`
- Modify: `internal/rules/eval.go`
- Test: `internal/store/rules_test.go`
- Test: `internal/rules/eval_test.go`

**Interfaces:**
- Produces:
  - `type ConditionGroupNode struct { Logic string; Children []ConditionNode }`
  - `type ConditionLeafNode struct { Field string; Operator string; Values []string }`
  - `type ConditionNode struct { Type string; Group *ConditionGroupNode; Leaf *ConditionLeafNode }`
  - Backward-compatible `RuleConditions.UnmarshalJSON` handling both `[...]` array and `{...}` tree
- Consumes:
  - PostgreSQL JSONB column `conditions`

- [ ] **Step 1: Write failing tests for AST deserialization and evaluation**
  - Test legacy flat JSON `[{"field":"ip","operator":"in","values":["1.1.1.1"]}]` unmarshals as root Group with `logic="all"`;
  - Test nested AST JSON with `group` containing nested `or`/`and` nodes;
  - Test evaluation of `(Country == US AND DevType == mobile) OR (Country == UK AND DevType == tablet)`.
- [ ] **Step 2: Implement AST data structures in `internal/store/rules.go`**
  Implement custom `UnmarshalJSON` and `Value()` for `RuleConditions` supporting both structures.
- [ ] **Step 3: Update `compileRule` and `compileNode` in `internal/rules/eval.go`**
  Support recursive evaluation of condition nodes with short-circuiting.
- [ ] **Step 4: Run tests to verify they pass**
  Run: `go test -v ./internal/store ./internal/rules -run "TestConditionAST|TestRuleConditionsJSON"`
  Expected: PASS
- [ ] **Step 5: Commit**
  ```bash
  git add internal/store/rules.go internal/rules/eval.go internal/store/rules_test.go internal/rules/eval_test.go
  git commit -m "feat(rules): support nested condition AST with backward-compatible JSONB unmarshaling"
  ```

---

### Task 6: IPRadixTree 基数树与新运算符扩展 (Radix Tree & Extended Operators)

**Files:**
- Create: `internal/rules/radix.go`
- Create: `internal/rules/radix_test.go`
- Modify: `internal/rules/eval.go`
- Modify: `internal/rules/eval_test.go`

**Interfaces:**
- Produces:
  - `type IPRadixTree struct`
  - `func NewIPRadixTree(prefixes []netip.Prefix) *IPRadixTree`
  - `func (t *IPRadixTree) Contains(addr netip.Addr) bool`
  - Operators: `OpStartsWith = "starts_with"`, `OpEndsWith = "ends_with"`, `OpInCIDR = "in_cidr"`

- [ ] **Step 1: Write failing tests in `internal/rules/radix_test.go`**
  Test IPv4 and IPv6 CIDR insertion and lookup (exact match, prefix match, mismatch).
- [ ] **Step 2: Implement `IPRadixTree` in `internal/rules/radix.go`**
  Implement bitwise radix tree traversing bits of `netip.Addr`.
- [ ] **Step 3: Integrate `IPRadixTree` and new string prefix/suffix operators in `internal/rules/eval.go`**
  - Add `starts_with` and `ends_with` to whitelist and evaluator;
  - For IP conditions, pre-compile CIDRs into `IPRadixTree` for $O(1)$ lookups.
- [ ] **Step 4: Run tests and benchmarks**
  Run: `go test -v ./internal/rules -run "TestRadix|TestExtendedOperators"`
  Expected: PASS
- [ ] **Step 5: Commit**
  ```bash
  git add internal/rules/radix.go internal/rules/radix_test.go internal/rules/eval.go internal/rules/eval_test.go
  git commit -m "feat(rules): add IPRadixTree for O(1) CIDR matching and prefix/suffix operators"
  ```

---

### Task 7: 引入 Expr 驱动与双层 ConditionEvaluator 架构 (Expr VM Integration)

**Files:**
- Modify: `go.mod`
- Create: `migrations/0013_rule_expression.sql`
- Create: `internal/rules/evaluator.go`
- Modify: `internal/rules/snapshot.go`
- Modify: `internal/store/rules.go`
- Modify: `internal/httpapi/rules.go`
- Test: `internal/rules/evaluator_test.go`

**Interfaces:**
- Produces:
  - `type ConditionEvaluator interface { Match(ctx VisitorContext) bool }`
  - `type nativeVisualEvaluator struct`
  - `type exprEvaluator struct { program *vm.Program }`
  - `store.Rule`: `RuleType string` (`visual` / `expression`), `Expression string`
  - Endpoint `POST /api/rules/validate-expr` for expression syntax checking
- Consumes:
  - `github.com/expr-lang/expr`

- [ ] **Step 1: Add `github.com/expr-lang/expr` to `go.mod` and run `go mod tidy`**
- [ ] **Step 2: Create migration `migrations/0013_rule_expression.sql`**
  Add `rule_type VARCHAR(20) NOT NULL DEFAULT 'visual'` and `expression TEXT NOT NULL DEFAULT ''` to `rules`.
- [ ] **Step 3: Update `store.Rule` and store methods in `internal/store/rules.go`**
  Add `RuleType` and `Expression` fields to `Rule` struct and `RuleUpdate`.
- [ ] **Step 4: Implement `ConditionEvaluator` and `exprEvaluator` in `internal/rules/evaluator.go`**
  - Compile expressions with `expr.Env(&Fact{})` and `expr.AsBool()`;
  - Integrate into `Snapshot` compile step based on `rule.RuleType`.
- [ ] **Step 5: Add expression validation endpoint in `internal/httpapi/rules.go`**
  Implement `POST /api/rules/validate-expr` checking syntax and compile errors.
- [ ] **Step 6: Write unit tests in `internal/rules/evaluator_test.go`**
  Verify expression rules evaluate correctly and fail-open on runtime error.
- [ ] **Step 7: Run all tests**
  Run: `go test -v ./internal/rules ./internal/store ./internal/httpapi -run "TestExpr"`
  Expected: PASS
- [ ] **Step 8: Commit**
  ```bash
  git add go.mod go.sum migrations/0013_rule_expression.sql internal/rules/ internal/store/ internal/httpapi/
  git commit -m "feat(rules): integrate expr-lang engine for advanced expression rules"
  ```

---

### Task 8: 全链路回归、静态检查与性能基准验证

**Files:**
- Test: 全库现有及新增测试

- [ ] **Step 1: Run complete backend test suite with race detector**
  Run: `go test -race ./internal/...`
  Expected: All packages pass without data races.
- [ ] **Step 2: Run rule engine benchmarks to verify performance improvements**
  Run: `go test -bench=. -benchmem ./internal/rules`
  Verify: Hot-path evaluation retains 0 B/op, 0 allocs/op, latency <= 350ns.
- [ ] **Step 3: Run web frontend build and type checking**
  Run: `cd web && pnpm build`
  Expected: Clean build with zero TypeScript / Vite errors.
- [ ] **Step 4: Final verification and commit**
  ```bash
  git status
  ```
