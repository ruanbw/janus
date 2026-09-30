# Custom Error Pages (404 & 429) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide professional, responsive default HTML 404 & 429 error pages on short link visitor routes, allow tenant-level global custom 404/429 HTML pages, and allow per-rule custom HTML overrides for 404/429 actions while isolating management APIs to JSON responses.

**Architecture:** A three-tier resolution mechanism (Rule Custom HTML > Tenant Custom HTML > Embedded Default HTML) executes during shortlink miss / rule blocking in the redirect hot path with in-memory snapshot caching (zero disk IO). HTML templates are embedded via Go `embed`, and custom HTML strings (up to 512KB) are stored in PostgreSQL.

**Tech Stack:** Go (Gin, GORM, embed), PostgreSQL, Vue 3, Vite, TailwindCSS.

**Spec:** `.scratch/custom-error-pages/spec.md`

## Global Constraints

- Visitor routes (`/:code` and fallback landing routes) MUST return `Content-Type: text/html; charset=utf-8` on 404 and 429.
- Management APIs (`/api/*`) and internal endpoints (`/internal/*`, `/healthz`) MUST continue returning JSON responses (`writeErr`).
- Custom HTML text size limit: maximum 512KB (524,288 bytes) per page.
- Rule evaluation hot path MUST remain pure in-memory: custom HTML for rules travels inside `rules.Snapshot` and `rules.Decision`.
- Visit outcomes (`visits` table) MUST continue recording `outcome=failed` and reasons accurately (`rule_blocked`, `rule_throttled`, `link_disabled`, etc.).

## Review Focus

- Visitor requests with missing shortcode or inactive domain on valid tenant domain: MUST render tenant global 404 HTML (or default 404) instead of JSON.
- Requests with non-existent tenant/domain (unresolvable Host): MUST fall back to embedded default 404 HTML instead of panic or JSON.
- Rule action `notfound` / `throttle` with `pageMode = 'custom'` but empty `customHtml`: MUST fall back gracefully to tenant global page, then to default embedded page.
- HTML input exceeding 512KB: MUST be rejected by API validation with HTTP 400.
- XSS isolation in admin preview: Admin console preview MUST use sandboxed `<iframe>` to avoid injecting script execution into the console DOM.

---

### Task 1: 数据库迁移与 Store 层扩展

**Files:**
- Create: `migrations/0012_custom_error_pages.sql`
- Modify: `internal/store/store.go`
- Modify: `internal/store/rules.go`
- Test: `internal/store/rules_test.go`

**Interfaces:**
- Consumes: PostgreSQL schema, GORM models in `internal/store`
- Produces:
  - `store.Tenant`: `Custom404HTML string`, `Custom429HTML string`
  - `store.Rule`: `PageMode string` (`default` / `custom`), `CustomHTML string`
  - `(*Store).UpdateTenantErrorPages(ctx context.Context, tenantID int64, page404, page429 string) error`
  - `(*Store).GetTenantErrorPages(ctx context.Context, tenantID int64) (page404, page429 string, err error)`

- [ ] **Step 1: Write SQL migration `migrations/0012_custom_error_pages.sql`**
  Add columns to `tenants` and `rules` with CHECK constraints.
- [ ] **Step 2: Update models and store methods in `internal/store`**
  Add fields to `Tenant` and `Rule` structs; implement `UpdateTenantErrorPages` and `GetTenantErrorPages`.
- [ ] **Step 3: Write tests in `internal/store/rules_test.go`**
  Verify rule creation/update preserves `PageMode` and `CustomHTML`, and tenant error pages are updated/queried properly.
- [ ] **Step 4: Run tests**
  Run: `go test -v ./internal/store -run "TestRuleCustomErrorPages|TestTenantErrorPages"`
  Expected: PASS
- [ ] **Step 5: Commit**
  ```bash
  git add migrations/0012_custom_error_pages.sql internal/store/
  git commit -m "feat(store): add custom error pages columns and store methods"
  ```

---

### Task 2: 系统内置默认 HTML 模板与渲染包

**Files:**
- Create: `internal/httpapi/templates/default_404.html`
- Create: `internal/httpapi/templates/default_429.html`
- Create: `internal/httpapi/templates/templates.go`
- Test: `internal/httpapi/templates/templates_test.go`

**Interfaces:**
- Consumes: `//go:embed`
- Produces:
  - `templates.Default404HTML() string`
  - `templates.Default429HTML() string`

- [ ] **Step 1: Create responsive static HTML templates**
  Write modern, clean, self-contained CSS templates in `default_404.html` and `default_429.html`.
- [ ] **Step 2: Create `templates.go` with embed filesystem**
  Expose helper functions `Default404HTML()` and `Default429HTML()`.
- [ ] **Step 3: Write unit test in `templates_test.go`**
  Verify embedded files are non-empty and contain status codes and basic layout tags.
- [ ] **Step 4: Run tests**
  Run: `go test -v ./internal/httpapi/templates`
  Expected: PASS
- [ ] **Step 5: Commit**
  ```bash
  git add internal/httpapi/templates/
  git commit -m "feat(httpapi): add embedded default 404 and 429 HTML templates"
  ```

---

### Task 3: 规则求值快照与 Decision 携带 HTML 配置

**Files:**
- Modify: `internal/rules/eval.go:53-85`
- Modify: `internal/rules/snapshot.go:180-218`
- Test: `internal/rules/eval_test.go`

**Interfaces:**
- Consumes: `store.Rule.PageMode`, `store.Rule.CustomHTML`
- Produces:
  - `rules.Decision`: fields `PageMode string`, `CustomHTML string`
  - `rules.Compiled`: fields `pageMode string`, `customHTML string`

- [ ] **Step 1: Write failing test in `internal/rules/eval_test.go`**
  Verify that when an action is `notfound` or `throttle`, `snap.Evaluate` populates `Decision.PageMode` and `Decision.CustomHTML`.
- [ ] **Step 2: Run test to verify failure**
  Run: `go test -v ./internal/rules -run TestDecisionCarriesCustomHTML`
  Expected: FAIL (fields not defined)
- [ ] **Step 3: Implement fields in `eval.go` and `snapshot.go`**
  Populate `Compiled.pageMode`, `Compiled.customHTML` during `compileRule`, and set `Decision.PageMode`, `Decision.CustomHTML` during `Evaluate`.
- [ ] **Step 4: Run test to verify it passes**
  Run: `go test -v ./internal/rules -run TestDecisionCarriesCustomHTML`
  Expected: PASS
- [ ] **Step 5: Commit**
  ```bash
  git add internal/rules/
  git commit -m "feat(rules): propagate pageMode and customHTML to Decision"
  ```

---

### Task 4: 短链访问链路 HTML 渲染与多级回退

**Files:**
- Modify: `internal/httpapi/redirect.go`
- Modify: `internal/httpapi/landing.go`
- Modify: `internal/httpapi/server.go` (cache or loader for tenant error pages if needed)
- Test: `internal/httpapi/rules_redirect_test.go`

**Interfaces:**
- Consumes: `templates.Default404HTML()`, `templates.Default429HTML()`, `rules.Decision`, `store.Tenant`
- Produces:
  - `(a *API) renderVisitorError(c *gin.Context, status int, tenantID int64, dec *rules.Decision)`:
    Renders custom rule HTML if custom, else tenant error page, else embedded default HTML. Sets `Content-Type: text/html; charset=utf-8`.

- [ ] **Step 1: Write integration tests in `rules_redirect_test.go`**
  - Test 1: Rule notfound with custom HTML renders rule custom HTML.
  - Test 2: Rule notfound with default mode renders tenant custom 404 HTML.
  - Test 3: Unmatched short link on tenant domain renders tenant custom 404 HTML.
  - Test 4: Link without tenant custom page falls back to embedded default HTML.
  - Test 5: Rule throttle renders 429 HTML.
  - Test 6: API error `/api/*` still returns JSON.
- [ ] **Step 2: Run tests to verify failure**
  Run: `go test -v ./internal/httpapi -run TestVisitorErrorPage`
  Expected: FAIL
- [ ] **Step 3: Implement `renderVisitorError` in `redirect.go` and replace `writeErr` in visitor handlers**
  Ensure visitor handlers in `redirect.go` and `landing.go` call `renderVisitorError` while API routes remain untouched.
- [ ] **Step 4: Run tests to verify pass**
  Run: `go test -v ./internal/httpapi -run TestVisitorErrorPage`
  Expected: PASS
- [ ] **Step 5: Commit**
  ```bash
  git add internal/httpapi/
  git commit -m "feat(httpapi): render HTML error pages on visitor redirect routes"
  ```

---

### Task 5: 管理端 API（租户设置与规则配置接口扩充）

**Files:**
- Modify: `internal/httpapi/me.go`
- Modify: `internal/httpapi/rules.go`
- Test: `internal/httpapi/me_test.go`
- Test: `internal/httpapi/rules_test.go`

**Interfaces:**
- Consumes: `store.Store`, HTTP handlers
- Produces:
  - `GET /api/me/error-pages`: returns `{ custom404Html, custom429Html }`
  - `PATCH /api/me/error-pages`: accepts payload `{ custom404Html, custom429Html }`, validates max 512KB, invalidates rule cache.
  - `POST /api/rules` & `PATCH /api/rules/:id`: supports `pageMode`, `customHtml` (validates `pageMode in ('default', 'custom')` and len <= 512KB).

- [ ] **Step 1: Write tests for error pages endpoints and rule payload in `me_test.go` & `rules_test.go`**
  Test size limits (>512KB returns 400), valid updates, and rule updates.
- [ ] **Step 2: Run tests to verify failure**
  Run: `go test -v ./internal/httpapi -run "TestTenantErrorPagesAPI|TestRuleErrorPagePayload"`
  Expected: FAIL
- [ ] **Step 3: Implement endpoints in `me.go` and update `rules.go`**
  Add route handlers, payload validation, and store calls.
- [ ] **Step 4: Run tests to verify pass**
  Run: `go test -v ./internal/httpapi -run "TestTenantErrorPagesAPI|TestRuleErrorPagePayload"`
  Expected: PASS
- [ ] **Step 5: Commit**
  ```bash
  git add internal/httpapi/
  git commit -m "feat(httpapi): add tenant error pages endpoints and rule page fields"
  ```

---

### Task 6: 前端控制台界面与交互联动

**Files:**
- Modify: `web/src/types/api.ts`
- Modify: `web/src/api/rules.ts`
- Modify: `web/src/api/me.ts`
- Create: `web/src/views/settings/ErrorPagesCard.vue`
- Modify: `web/src/views/settings/SettingsView.vue` (or me view)
- Modify: `web/src/views/rules/RuleFormView.vue`
- Modify: `web/src/views/rules/RuleSimulatorView.vue`

**Interfaces:**
- Consumes: `/api/me/error-pages`, `/api/rules`
- Produces:
  - Tenant Error Pages settings UI with upload, code editor, and live sandbox iframe preview.
  - Rule form condition/action UI with conditional page mode and HTML upload/preview for `notfound` & `throttle`.
  - Rule simulator preview for blocked visitor HTML.

- [ ] **Step 1: Update TypeScript types in `web/src/types/api.ts`**
  Add `pageMode?: 'default' | 'custom'`, `customHtml?: string` to `Rule` / `RuleCreatePayload` / `RuleUpdatePayload`. Add `TenantErrorPages` types.
- [ ] **Step 2: Update API functions in `web/src/api/rules.ts` and `web/src/api/me.ts`**
  Add `getTenantErrorPages` and `updateTenantErrorPages`.
- [ ] **Step 3: Build `ErrorPagesCard.vue` in settings view**
  Create upload button, HTML editor, character count, and sandbox `<iframe>` preview modal.
- [ ] **Step 4: Update `RuleFormView.vue`**
  Add pageMode and customHtml controls when `action === 'notfound' || action === 'throttle'`.
- [ ] **Step 5: Update `RuleSimulatorView.vue`**
  Add "查看访客端拦截页面" button on verdict card.
- [ ] **Step 6: Build frontend to verify no type or compilation errors**
  Run: `cd web && pnpm build`
  Expected: Build succeeds with 0 errors.
- [ ] **Step 7: Commit**
  ```bash
  git add web/
  git commit -m "feat(web): add error pages management in settings, rule form, and simulator"
  ```
