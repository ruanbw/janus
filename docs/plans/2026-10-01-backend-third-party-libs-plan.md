# Backend Third-Party Libraries Adoption Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate bespoke/handcrafted infrastructure wheels in backend logic modules (`radix.go`, `ratelimit.go`, `smtp.go`, `config.go`, `migrate.go`) and replace them with battle-tested, mature third-party libraries (`cidranger`, `x/time/rate`, `go-mail`, `caarlos0/env/v11`, `goose/v3`).

**Architecture:**
Wrap third-party libraries cleanly behind existing project domain interfaces and struct boundaries to ensure zero breaking changes to higher-level handlers, redirect evaluators, and test fixtures. Maintain exact status codes, error models, and fail-open guarantees.

**Tech Stack:** Go 1.22+, `github.com/yl2chen/cidranger`, `golang.org/x/time/rate`, `github.com/wneessen/go-mail`, `github.com/caarlos0/env/v11`, `github.com/pressly/goose/v3`, PostgreSQL.

**Spec:** `docs/superpowers/specs/2026-10-01-backend-third-party-libs-design.md`

---

## Global Constraints

- **Zero Handcrafted Wheels**: Do not write custom radix trie traversal, custom rate limiter windows/eviction, custom SMTP socket/MIME machines, custom environment variable reflection helpers, or custom migration file loaders.
- **Fail-open & Reliability**: Rule CIDR matching must never panic and must support both IPv4 and IPv6 CIDRs seamlessly. Rate limiter must strictly prevent unbounded memory leaks via eviction of stale visitor buckets.
- **API & Behavior Compatibility**: HTTP status codes, error payload codes (`E_RATE_LIMITED`), token delivery contracts, and database migration sequences must remain 100% backward compatible.
- **Verification First**: Every task must be verified with automated test suites before committing.

---

## File Structure & Responsibilities

- `internal/rules/radix.go`: Replace 200+ lines of custom bit-level slice trie with `cidranger.Ranger` implementation.
- `internal/rules/radix_test.go`: Verify IPv4, IPv6, prefix match, boundary cases, normalization using `cidranger`.
- `internal/httpapi/ratelimit.go`: Replace handcrafted fixed-window slice map with `golang.org/x/time/rate` token bucket limiter.
- `internal/httpapi/ratelimit_test.go`: Verify rate limiting across authentication endpoints (`/api/auth/register`, `/api/auth/login`, `/api/auth/forgot-password`).
- `internal/mailer/smtp.go`: Replace handcrafted raw TCP socket negotiation and MIME formatting with `github.com/wneessen/go-mail`.
- `internal/mailer/smtp_test.go`: Unit test client options, body construction, and TLS policy logic.
- `internal/config/config.go`: Replace manual `os.Getenv` string manipulation with `github.com/caarlos0/env/v11`.
- `internal/config/config_test.go`: Verify environment variable parsing, default values, and override behavior.
- `internal/db/migrate.go`: Replace custom file reader and table schema migration runner with `github.com/pressly/goose/v3` and `pgx/v5/stdlib`.
- `internal/db/migrate_test.go`: Verify migration runner correctly applies migrations idempotently.

---

## Tasks

### Task 1: Add Go module dependencies

- [x] **Step 1.1: Run `go get` for all 5 mature third-party libraries**
  - Run `go get github.com/yl2chen/cidranger`
  - Run `go get golang.org/x/time/rate`
  - Run `go get github.com/wneessen/go-mail`
  - Run `go get github.com/caarlos0/env/v11`
  - Run `go get github.com/pressly/goose/v3`
- [x] **Step 1.2: Run `go mod tidy` and verify build**
  - Run `go mod tidy`
  - Verify `go.mod` and `go.sum` are updated cleanly.
- [x] **Step 1.3: Commit changes**
  - Commit message: `build(deps): add cidranger, x/time/rate, go-mail, env, and goose`

---

### Task 2: Replace custom Radix Tree with `cidranger` in `internal/rules`

- [x] **Step 2.1: Implement `IPRanger` in `internal/rules/radix.go` using `cidranger`**
  - Use `cidranger.NewPCTrieRanger()` to construct compressed path trie.
  - Insert prefixes converted from `netip.Prefix` to `*net.IPNet`.
  - Provide `Contains(addr netip.Addr) bool` by querying `ranger.Contains(net.IP(addr.AsSlice()))`.
  - Remove all manual node allocation, bit slicing, and handcrafted binary search code.
- [x] **Step 2.2: Update `internal/rules/eval.go` to use `*IPRanger`**
  - Update `CompiledCondition.radix` to `*IPRanger`.
  - Update `compileCondition` to construct `*IPRanger`.
- [x] **Step 2.3: Verify with unit tests and benchmark tests**
  - Run `go test -v ./internal/rules/...`
  - Ensure zero regressions on IPv4/IPv6 tests in `radix_test.go` and `eval_test.go`.
- [x] **Step 2.4: Commit changes**
  - Commit message: `refactor(rules): replace custom radix tree with cidranger`

---

### Task 3: Replace custom rate limiter with `golang.org/x/time/rate` in `internal/httpapi`

- [x] **Step 3.1: Refactor `internal/httpapi/ratelimit.go` using `x/time/rate`**
  - Create token-bucket limiter mapping per IP with `rate.NewLimiter(rate.Limit(limit/window.Seconds()), limit)`.
  - Support automatic LRU/cleanup for stale visitor buckets to avoid memory leaks.
  - Retain `RateLimiter` interface and middleware factory `rateLimitMiddleware`.
- [x] **Step 3.2: Verify rate limiting tests**
  - Run `go test -v ./internal/httpapi/ -run "TestRegisterRateLimit|TestAuthRateLimitSharedAcrossLoginAndForgot"`
- [x] **Step 3.3: Commit changes**
  - Commit message: `refactor(httpapi): replace custom rate limiter with x/time/rate token bucket`

---

### Task 4: Replace custom SMTP client with `github.com/wneessen/go-mail` in `internal/mailer`

- [x] **Step 4.1: Refactor `internal/mailer/smtp.go` using `go-mail`**
  - Initialize client using `mail.NewClient` with timeout, port, SSL / STARTTLS configuration.
  - Build message using `mail.NewMsg()` with standard headers and UTF-8 body.
  - Eliminate manual TCP socket connection, SMTP greeting/EHLO handshake parsing, and manual MIME headers.
- [x] **Step 4.2: Add unit tests in `internal/mailer/smtp_test.go`**
  - Verify client options setup, message creation, and error handling.
  - Run `go test -v ./internal/mailer/...`
- [x] **Step 4.3: Commit changes**
  - Commit message: `refactor(mailer): replace manual SMTP protocol client with go-mail`

---

### Task 5: Replace manual config parsing with `github.com/caarlos0/env/v11` in `internal/config`

- [x] **Step 5.1: Add `env` tags to `Config` struct in `internal/config/config.go`**
  - Annotate all fields with `env:"CLOAK_..."` and `envDefault:"..."`.
  - Use `env.Parse(&cfg)` in `Load()`.
  - Remove redundant `getenv`, `getbool`, `getint`, `getdur` helper functions.
- [x] **Step 5.2: Verify config tests**
  - Add/update `internal/config/config_test.go` covering defaults, overrides, and duration/boolean parsing.
  - Run `go test -v ./internal/config/...`
- [x] **Step 5.3: Commit changes**
  - Commit message: `refactor(config): replace manual env parsing with caarlos0/env`

---

### Task 6: Replace custom migration runner with `github.com/pressly/goose/v3` in `internal/db`

- [x] **Step 6.1: Refactor `internal/db/migrate.go` using `goose/v3`**
  - Use `stdlib.OpenDBFromPool(pool)` from `pgx/v5/stdlib`.
  - Set dialect to postgres: `goose.SetDialect("postgres")`.
  - Sync legacy `schema_migrations` records to `goose_db_version` if needed for smooth upgrade.
  - Execute `goose.Up(db, dir)`.
- [x] **Step 6.2: Verify database migration tests**
  - Run `go test -v ./internal/db/...`
- [x] **Step 6.3: Commit changes**
  - Commit message: `refactor(db): replace custom migration runner with goose`

---

### Task 7: Full Integration & Regression Testing

- [x] **Step 7.1: Run full test suite across entire repository**
  - Run `go test -race ./...`
  - Verify all packages pass cleanly.
- [x] **Step 7.2: Code quality check and git review**
  - Review `git diff` to ensure zero custom wheels remain.
  - Update issue status in `.scratch/backend-third-party-libs/issues/`.
