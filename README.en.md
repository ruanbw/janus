<div align="center">

# Janus

**每一次跳转，皆为裁决。**  
*Every jump is a verdict.*

[![license](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)](LICENSE)
[![go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](go.mod)
[![node](https://img.shields.io/badge/Node-%E2%89%A520.19-339933?logo=nodedotjs&logoColor=white)](web/package.json)
[![pnpm](https://img.shields.io/badge/pnpm-10-F69220?logo=pnpm&logoColor=white)](web/package.json)
[![postgres](https://img.shields.io/badge/PostgreSQL-16-4169E1?logo=postgresql&logoColor=white)](docker-compose.yml)
[![caddy](https://img.shields.io/badge/Caddy-2-2B6CB?logo=caddy&logoColor=white)](Caddyfile.prod)

[中文](README.md) · [English](README.en.md)

</div>

---

Janus is a **self-hosted multi-tenant short-link platform**: tenants sign up, bring their own domains, and the system issues and renews HTTPS certificates automatically for every activated domain, redirecting `domain/code` hits to a target URL. Beyond plain redirects it ships **landing-page links**, **visit detail with geographic attribution**, a **tenant-level access rule engine**, **custom error pages**, and a **Bearer JWT API** for scripts.

- **Single-server deployment** — Go backend (Gin + GORM) + Vue 3 SPA served by nginx + PostgreSQL 16 + Caddy (on-demand TLS, automatic Let's Encrypt issuance/renewal). `docker compose up -d` brings it all up.
- **Tenant isolation** — domains, links, rules and landing pages of one tenant are invisible to every other. The deployer holds a platform-admin role and can govern the whole platform.
- **Frontend/backend separation** — the frontend builds into its own nginx image; the backend image contains zero frontend files. A frontend change never requires rebuilding the backend (ADR-0006).
- **Works out of the box** — sign up with an email and you immediately get a `<slug>.<platform-domain>` test domain. No certificate or DNS busywork.

> Canonical terminology (tenant, short link, short code, landing page, target URL, rule, verdict, visit, certificate …) is defined in [`CONTEXT.md`](CONTEXT.md); this document uses those terms consistently.

---

## Contents

- [1. Overview](#1-overview)
- [2. Feature list](#2-feature-list)
- [3. Architecture](#3-architecture)
- [4. Repository layout](#4-repository-layout)
- [5. Requirements](#5-requirements)
- [6. Quick start (development)](#6-quick-start-development)
- [7. Development conventions](#7-development-conventions)
- [8. Testing](#8-testing)
- [9. Development vs. production](#9-development-vs-production)
- [10. Production deployment (brief)](#10-production-deployment-brief)
- [11. Troubleshooting](#11-troubleshooting)
- [12. API overview](#12-api-overview)
- [13. Contributing](#13-contributing)
- [14. License](#14-license)
- [15. Further reading](#15-further-reading)

---

## 1. Overview

### 1.1 What problem it solves

Off-the-shelf short-link services either charge per click or refuse to let you use your own domain. Janus's trade-off is simple: **it runs on your own server, and tenants bring their own domains**.

| Need | How Janus handles it |
| --- | --- |
| Short links on your own domain | Add a self-owned domain, pass a DNS activation check; Caddy on-demand TLS issues and renews the certificate |
| No manual certificate per tenant | One wildcard record `*.<platform-domain>` gives every tenant an automatic `<slug>.<platform-domain>` |
| Deployment must not depend on third parties | Only external dependencies are Let's Encrypt (issuance/renewal) and optional SMTP; database and certificates live on your box |
| Scripts and third-party integrations need access | Bearer JWT (24h) issued alongside the session cookie, so scripts skip cookies and CSRF |
| Traffic needs to be auditable | Every visit stores one row: action (redirect / landing view / click), success or failure and why, IP, device, country, referrer, timestamp |
| Access must be dispositioned by visitor profile | Tenant-scoped rule engine: 13 visitor fields combined by conditions, verdict is pass / rewrite target / 404 / throttle |

### 1.2 What happens on a visit

Full processing order of `GET https://<domain>/<code>` (implemented in [`internal/httpapi/redirect.go`](internal/httpapi/redirect.go)):

```
request arrives
  │
  ├─ 1. Host → resolve active domain       ── miss → 404 (no visit row)
  │
  ├─ 2. Code → resolve link + target pool  ── miss → 404 (no visit row)
  │
  ├─ 3. Link availability check           ── disabled / soft-deleted / no target /
  │      (rules do NOT take part)                 missing landing files
  │                                               → 404 + one outcome=failed row
  │
  ├─ 4. Rule evaluation (if link.rules_enabled)
  │      snapshot cached per tenant in memory → pass: continue
  │      ascending priority, first hit decides  → redirect: rewrite target
  │                                              / notfound: 404 / throttle: 429
  │      no rule matched → original redirect flow
  │
  ├─ 5. Landing link → 302 to external landing URL, or 302 to /<code>/ (hosted upload)
  │      Redirect link → pick one target round-robin → 302 (default) or 301 (permanent)
  │
  └─ 6. Write one visit row (action / outcome / IP / UA / referrer / language / country / rule)
```

Three hard constraints on this hot path (you must preserve them when editing `redirect.go`):

1. **Zero DB queries** — rules come from a per-tenant in-memory snapshot; tenants without rules never even build a visitor profile.
2. **Zero write amplification** — a rule hit updates no counter table; it only adds the single visit row ("hits in 24h" is aggregated at read time).
3. **Fail-open** — a snapshot that fails to load, or a panic during evaluation, is treated as "no match". A risk rule must never turn production links into 500s.

---

## 2. Feature list

### 2.1 Tenants and authentication

- **Email registration + verification** (anti-spam): a new tenant is `pending` until the email is verified, then turns `active` and its platform default domain is activated automatically.
- **Login / logout / remember me**: 30 days when "remember me" is checked, 24 hours otherwise; the super admin sets a password on first login.
- **Change password / forgot password / reset password**: reset tokens live 1 hour.
- **Platform admin (super admin)**: initialized from `JANUS_SUPERADMIN_EMAIL`, idempotently created on boot.
- **Two authentication modes**: browser session cookie (HTTP-only + CSRF double-submit token) for the console; `POST /api/auth/token` issues a Bearer JWT for scripts/CLIs (header auth, CSRF-immune).
- **Tiers and quotas**: a tier caps the number of links and self-owned domains; platform default domains never count toward quotas; exceeding a quota returns an explicit error including current usage and the limit.

### 2.2 Domains and certificates

- **Platform default domain**: every tenant automatically receives `<slug>.<platform-domain>`, activated right after email verification, exempt from quotas, deactivatable but not deletable.
- **Self-owned domains**: added by the tenant and validated by checking that DNS points at this server; once active, Caddy issues and renews a Let's Encrypt certificate automatically.
- **Activation retries**: re-checked every 5 minutes for up to 72 hours, marked `failed` afterwards, with a manual "re-check" button in the console.
- **Lifecycle**: deactivate / restore / delete (physical; all link associations on the domain must be cleared first).
- **Certificate status** is visible in the console.

### 2.3 Short links

- **Custom short code** or auto-generated (6 chars). The same code on different domains may point to different targets, and one tenant may reuse a code across several links.
- **Multiple targets**: a link can hold several target URLs, and each visit picks one **round-robin**.
- **Redirect mode**: temporary 302 (default) or permanent 301, configured per link.
- **Two types**: **redirect** (redirects straight to the target URL) and **landing** (goes to the landing page first, target URL after the click). The type is chosen at creation time but remains **editable afterwards** (`PATCH /api/links/:id` with `linkType`; the link edit form exposes it). A change takes effect immediately.
- **Enable / disable / delete**: soft delete by default (row, associations and visit data are retained), plus a permanent purge; the console supports batch soft delete and batch purge.
- **Per-link rule switch**: any link can opt out of rule evaluation entirely.

### 2.4 Landing-page links

The landing page source is one of two, switchable at any time (ADR-0005):

- **`url`** — point at an external landing page; visits redirect there.
- **`upload`** — upload a zip (must contain `index.html`) that the platform hosts under `code/`; bounded by total uncompressed size (10 MB default) and file count (500 default), with an extension allowlist.

Alongside it:

- **A per-link JS SDK** (`GET /<code>/sdk.js`, with that link's absolute click endpoint baked in): once a landing page includes it, button clicks are bound automatically.
- **Click endpoint** `GET /<code>/click`: increments the click counter and writes one `action=click` visit row (with IP, device and referrer). Clicks **do not** count toward the visit count.
- The click finally delivers the visitor to the target URL.

### 2.5 Visit detail and analytics

- **Visit detail** (ADR-0007): each visit records the **action** (redirect / landing view / click), the **outcome** (success / failure plus reason: link disabled / soft-deleted / no usable target / landing files missing), the visitor's IP, User-Agent, referrer, language, country and timestamp.
- **Counting rule**: only **successful** redirects and landing views count as visits; clicks and failures never do.
- **Attributable failures are never lost**: failures traceable to a specific link are recorded; a miss on a nonexistent code is not.
- **Overview page**: KPI cards + top-link ranking / device / OS / browser / referrer / country breakdowns + a **world map** choropleth (bundled country borders + `d3-geo` projection, lazily loaded, ADR-0010).
- **Overview stats come from a dedicated aggregation endpoint**, `GET /api/visits/overview`: full-tenant counters and per-dimension distributions in one SQL round trip (real `GROUP BY`, no sampling). The frontend no longer pulls visit rows and counts them itself — that inevitably drifts from the KPI definitions, treats "the latest 50 rows" as the whole, and covers only the first 100 links of the link list. The UA dimension returns **raw UA strings plus counts**; device/OS/browser labels are translated by the existing `ua-parser-js` so there is no second UA parser in Go.
- **CTR definition**: numerator and denominator come from the same SQL and the same retention window in the `visits` table; the denominator is **landing views only** (a redirect link can never produce a click, so including it would systematically depress CTR). The numerator is no longer the permanent `links.clicks` counter — it never decays while the denominator is trimmed by the 90-day cleanup, which made CTR drift monotonically upward past 100%.
- **Geographic attribution**: a bundled ip2region offline database (V4 + V6) resolves the visitor's country code, behind a 16-shard two-generation cache that also caches negative results (ADR-0009).
- **Retention**: visit rows are kept 90 days by default and pruned by a background worker (`JANUS_VISIT_RETENTION`).
- **Per-link detail view**: visit rows for a single link, including action, outcome and matched rule.

### 2.6 Rule engine

Tenant-scoped **access disposition rules**: one condition set plus one action (ADR-0008).

- **13 evaluable visitor fields**: `ip` (CIDR supported), `ipattr` (private / loopback / linklocal), `country`, `asn`, `lang`, `ref`, `utm`, `ua`, `devtype` (bot / mobile / tablet / desktop), `os`, `browser`, `path`, `domain`.
- **Operators**: in / not in / equals / not equals / contains / not contains / greater than / less than / regex (RE2, case-sensitive). The `ip` field matches CIDR ranges or literals, and numeric comparisons never match a non-numeric value. No nested groups in v1; `logic` decides "all must match" vs. "any may match".
- **Two authoring modes**: `visual` (condition tree, default) and `expression` (Expr language for advanced users, syntax checked by `POST /api/rules/validate-expr`).
- **Actions (verdicts)**: pass `pass` (records the hit, then continues the original flow) / rewrite target `redirect` (the rewritten URL does not join the link's target round-robin) / return 404 `notfound` (records `outcome=failed`, `reason=rule_blocked`) / throttle `throttle` (returns 429, records `reason=rule_throttled`); the last two do not count as visits.
- **Scope**: **global** applies to every link of the tenant; **specific links** applies only to explicitly associated links — the association is declared by the rule, and the rule is the single writer of that association (a link's "applicable rules" view is the same data). A link-scoped rule with zero associations never matches; the console marks that state explicitly.
- **Verdict semantics**: rules are evaluated in ascending `priority`, and the **first hit decides**; verdicts are never stacked. If nothing matches, the link's original target selection runs. Evaluation happens after the link availability check.
- **Rule simulation**: `POST /api/rules/simulate` replays evaluation through a same-source side path and answers, rule by rule, "which rule would this visitor hit, and why". Condition descriptions come from the backend so the UI never re-implements the decision and drifts.
- **Limits**: 200 rules per tenant. A tenant's rule set is cached in memory as one snapshot (1-minute TTL); **every rule write or association change must invalidate that tenant's snapshot**.

### 2.7 Custom error pages

- **Tenant-wide**: custom HTML for 404 and 429 pages.
- **Per rule**: a single rule can carry its own error page and mode (`default` / `custom`).
- Resolution order: rule-specific → tenant-wide → built-in default page (a static, theme-aware page).
- Applies to: misses, disabled / soft-deleted links, missing targets, missing landing files, rule `notfound` verdicts (404) and rule `throttle` verdicts (429).
- **Cached per tenant**: error pages use the same shape as the rule snapshot (lazy load + atomic swap + TTL backstop + explicit invalidation on edit). The miss path — the one easiest to trigger — no longer re-reads two 512KB `TEXT` columns on every request (see hard constraint #1 in 1.2).
- **Response headers**: error pages carry `Content-Security-Policy: sandbox allow-scripts allow-forms` (scripts run, but in an opaque origin that cannot read this site's session) and `X-Content-Type-Options: nosniff`; both 404 and 429 send `Cache-Control: no-store`, and 429 also sends `Retry-After: 60`. The admin preview iframe uses the same sandbox flags as production.

### 2.8 Platform administration (super admin)

- Browse all tenants, tenant detail, tier list;
- ban / unban accounts, change a tenant's tier;
- remove abusive domains.

### 2.9 API and security

- **Casbin RBAC**: two roles (`tenant` / `superadmin`) authorizing on the (role, HTTP method, path) triple, with the policy loaded in memory; a single `*` rule covers `/api/*` for super admins.
- **Rate limiting**: 10 requests per IP per minute on registration; 20 per IP per minute on login / verification / forgot / reset (`golang.org/x/time/rate` token buckets).
- **Passwords**: bcrypt; a dummy comparison runs even for unknown emails so response timing does not leak account existence.
- **Header-injection defense**: target URLs may use any protocol (open redirect by design) but control characters (CRLF) are rejected.
- **Authorization endpoint**: `/internal/caddy/authorize` is reachable only from the internal network, and refuses inactive domains — so pointing an arbitrary domain at this box cannot trigger certificate issuance.
- **Uniform error responses**: `{ code, message, details }` with stable codes and human-readable messages.

### 2.10 Platform operations

- **Migrations**: `migrations/*.sql` are applied automatically on boot (goose, idempotent).
- **Background workers**: DNS retries, certificate probing, visit pruning, session and email-token cleanup.
- **Pluggable mailer**: real SMTP when configured, otherwise a console mailer that prints messages to the backend log (handy in air-gapped environments).
- **Pluggable geo source**: abstracted by capability (ADR-0009); the shipped implementation is the bundled ip2region offline database. `asn` and `is_datacenter` stay empty because no source provides them (a rule depending on them simply never matches — values are never guessed).

---

## 3. Architecture

### 3.1 Request topology

```
                            Internet
                               │
                  ┌────────────┴─────────────┐
                  │  Caddy (80/443)          │
                  │  on-demand TLS           │   production: Let's Encrypt
                  │  ask → authorize endpoint│   development: local CA
                  └───────┬───────────┬──────┘
                          │           │
              platform domain (console)   *.<platform-domain>, self-owned domains
                          │           │
                  ┌───────┴───────────┴──────┐
                  │  Caddy: TLS + path split │
                  └───────┬───────────┬──────┘
                          │           │
              ┌───────────▼──┐   ┌────▼─────────────────┐
              │ nginx (web)  │   │  Go backend           │
              │ /            │   │  /api/*    REST API    │
              │ SPA + assets │   │  /{code}   redirect    │
              │              │   │  /{code}/… landing     │
              │              │   │  /internal/caddy/…     │
              │              │   │  background workers:   │
              │              │   │   DNS retry / cert     │
              │              │   │   probe / pruning      │
              └──────────────┘   └────┬─────────────────┘
                                        │
                       ┌────────────────┴────────────────┐
                       │  PostgreSQL 16                  │
                       │  + JANUS_LANDING_UPLOAD_DIR volume│
                       └─────────────────────────────────┘
```

### 3.2 Component responsibilities

| Component | Responsibility | Key point |
| --- | --- | --- |
| **Caddy** | TLS termination and certificate lifecycle | On-demand TLS: on the first handshake for an unknown domain, Caddy asks the app's `/internal/caddy/authorize` whether the domain is activated; if so it issues a Let's Encrypt certificate and renews automatically (ADR-0002). The app never edits Caddy config. |
| **Go backend** | Business logic, REST API, redirect hot path | Boot order: load config → connect Postgres → migrate → initialize super admin → start workers → serve HTTP |
| **nginx (web image)** | SPA assets and history fallback | Independent of the backend image; runs `pnpm build` during its own image build |
| **PostgreSQL** | All persistent state | Single source of truth; migration files ship with the image |
| **Frontend SPA** | Admin console | Vue 3 + TypeScript + Vite + Tailwind CSS 4 + Reka UI, with an in-house `App*` component kit |

### 3.3 Technology stack

**Backend** (Go 1.26)

| Concern | Choice |
| --- | --- |
| HTTP framework | `gin-gonic/gin` |
| Authorization | `casbin/casbin` + `gin-contrib/authz` (policy loaded in memory) |
| Data access | `gorm.io/gorm` + `gorm.io/driver/postgres` (on `pgx/v5`) |
| Migrations | `pressly/goose/v3` |
| Rule expressions | `expr-lang/expr` |
| JWT | `golang-jwt/jwt/v5` |
| Rate limiting | `golang.org/x/time/rate` |
| Passwords | `golang.org/x/crypto/bcrypt` |
| Mail | `wneessen/go-mail` (SMTP) |
| Config | `caarlos0/env/v11` |
| CIDR prefix tree | `yl2chen/cidranger` |
| GeoIP | `ip2region` offline xdb (V4 + V6, shipped inside the binary) |
| Testing | stdlib `testing` + `stretchr/testify`; black-box HTTP tests on `httptest.Server` with a real Postgres |

**Frontend** (Vue 3.5 + TypeScript)

| Concern | Choice |
| --- | --- |
| Build | Vite 6 |
| Styling | Tailwind CSS 4 (design tokens, dark mode) |
| Headless primitives | Reka UI (replaces the removed ant-design-vue) |
| Icons | `@lucide/vue` |
| State / routing | Pinia / Vue Router 4 |
| HTTP | Axios (wrapper + CSRF header + `ApiError`) |
| Validation / utils | `async-validator` / `@vueuse/core` / `ipaddr.js` |
| Charts & maps | `d3-geo` + `topojson-client` + `world-atlas` + `i18n-iso-countries` (lazy, overview page only) |

---

## 4. Repository layout

```
.
├── cmd/janus/                  # Backend entrypoint (config → DB → migrate → workers → HTTP)
├── internal/
│   ├── bootstrap/              # Boot-time initialization (idempotent super admin)
│   ├── config/                 # Env-var config (caarlos0/env)
│   ├── db/                     # Postgres connection + goose migrations
│   ├── domain/                 # Domain activation checks, cert probing, background workers
│   ├── geo/                    # IP geolocation (ip2region offline db + sharded cache)
│   ├── httpapi/                # HTTP routes and handlers (black-box tests live here)
│   │   └── templates/          # Built-in theme-aware 404 / 429 pages
│   ├── jwt/                    # JWT issuing and validation
│   ├── mailer/                 # Pluggable mail (SMTP / console)
│   ├── rbac/                   # Casbin policy text and authorization middleware
│   ├── rules/                  # Rule engine: fields, condition evaluation, tenant snapshot, simulation
│   ├── store/                  # Data access (tenants / domains / links / rules / visits)
│   └── testutil/               # Test infrastructure (httptest + real Postgres)
├── migrations/                 # goose SQL migrations (0001–0014, applied on boot)
├── web/                        # Frontend SPA (see web/README.md and web/UI_KIT.md)
│   ├── src/
│   │   ├── api/  types/  utils/ # Request layer, types, errors and helpers
│   │   ├── components/          # app/ (product) and ui/ (App* headless wrappers)
│   │   ├── layouts/  views/     # Console shell and feature pages
│   │   └── styles/              # Tailwind entry and the three token layers
│   ├── scripts/check-ui-consistency.mjs  # UI convention gate
│   └── dist/                   # Build output (not committed; built inside the image)
├── docker/
│   ├── Dockerfile              # Multi-stage: independent backend (Go) and web (nginx) targets
│   └── nginx.conf              # SPA fallback + long-lived asset caching
├── docker-compose.yml          # Development orchestration (Postgres + Caddy)
├── docker-compose.prod.yml     # Production orchestration (Postgres + backend + web + caddy)
├── Caddyfile / Caddyfile.prod  # Development (local CA) / production (Let's Encrypt)
├── .env.example                # Environment variable template
├── CONTEXT.md                  # Domain glossary
├── LICENSE                     # AGPL-3.0
└── docs/
    ├── deploy.md               # Production deployment guide
    ├── adr/                    # Architecture decision records (0001–0010)
    └── agents/                 # Agent collaboration conventions
```

---

## 5. Requirements

| Tool | Version | Purpose |
| --- | --- | --- |
| Docker Desktop (with Compose v2) | recent | Infrastructure: Postgres, Caddy |
| Go | ≥ 1.26 (`go.mod`) | Run the backend and its tests from a terminal |
| Node.js | ≥ 20.19 (`web/package.json` engines) | Frontend development and builds |
| pnpm | ≥ 9 (repo pins 10.x) | Frontend dependency management |
| Caddy CLI (optional) | ≥ 2.x | Trust the local CA in development (`caddy trust`) |

> In development, **infrastructure runs in Docker** (Postgres, Caddy) while **the backend and frontend run in your terminal** (`go run` + `pnpm dev`) for instant feedback; production is fully containerized.

---

## 6. Quick start (development)

### 6.1 Start infrastructure

```bash
docker compose up -d
```

| Service | Container port | Host mapping | Notes |
| --- | --- | --- | --- |
| `postgres` | 5432 | `127.0.0.1:5432` | PostgreSQL 16, dev credentials `janus/janus` |
| `caddy` | 443 | `127.0.0.1:443`, `127.0.0.1:80` | HTTPS entrypoint, on-demand TLS + local CA, reverse-proxying host `:8080` via `host.docker.internal` |

> Caddy takes host ports 80/443. If something else on your machine (an openresty, for example) binds them too, start them at different times.
> Port-less access relies on hosts entries pointing `*.janus.test` at `127.0.0.1` (see 6.2).

### 6.2 Local name resolution

Development supports both **direct localhost access** and **name-based access**. Name-based access needs the platform domain and your test tenant subdomains pointed at this machine, via SwitchHosts or `/etc/hosts`:

```text
127.0.0.1 app.janus.test
127.0.0.1 alice.janus.test
127.0.0.1 bob.janus.test
```

```bash
# Manual append (one line per new tenant; hosts has no wildcards)
sudo sh -c 'echo "127.0.0.1 alice.janus.test" >> /etc/hosts'
```

- `app.janus.test` serves the console. `<slug>.janus.test` is a tenant's platform default domain, used to verify link redirects.
- Test self-owned domains the same way (e.g. `127.0.0.1 links.example.test`). With `JANUS_SERVER_PUBLIC_IP=127.0.0.1`, Go's DNS check reads hosts, so you exercise the **real code path**.
- Prefer not to touch hosts? Use `curl --resolve` for temporary resolution (see 8.3).
- Vite's dev server rejects non-localhost Hosts by default (DNS-rebinding protection). The project allowlists `.janus.test` in `vite.config.ts` via `server.allowedHosts` (driven by `VITE_PLATFORM_DOMAIN` in `web/.env.development`); keep both in sync when you change the platform domain.

### 6.3 Trust the local Caddy CA

Development certificates are issued by Caddy's local CA, which browsers do not trust by default:

```bash
mkdir -p certs
docker compose exec caddy cat /data/caddy/pki/authorities/local/root.crt > certs/caddy-root.pem
sudo caddy trust --ca certs/caddy-root.pem
# macOS alternative:
# sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain certs/caddy-root.pem
```

> `certs/` is in `.gitignore`. Re-trust after recreating the Caddy data volume.

### 6.4 Run the backend

```bash
JANUS_COOKIE_SECURE=false JANUS_ADDR=:8080 go run ./cmd/janus
```

- `JANUS_COOKIE_SECURE=false` — development is plain HTTP, so `Secure` cookies would be dropped;
- `JANUS_ADDR=:8080` — matches the Vite proxy, the Caddy reverse proxy and the production compose; if you change the port, update the Vite proxy target and the Caddy upstream too;
- Everything else works on code defaults (Postgres `postgres://janus:janus@localhost:5432/janus`, platform domain `janus.test`, public IP `127.0.0.1`, console mailer);
- Override via environment variables, e.g. `JANUS_SUPERADMIN_EMAIL=admin@example.com go run ./cmd/janus`;
- `go run` does **not** read `.env` (that is compose's job) — export what you need on the command line; the full list lives in [.env.example](.env.example) and `internal/config/config.go`;
- with no SMTP configured, verification / reset emails are printed in this terminal (see 8.3, step 2).

Verify:

```bash
curl -s http://127.0.0.1:8080/healthz          # → {"status":"ok"}
curl -sk https://app.janus.test/healthz         # full path through Caddy
```

Restart with `Ctrl+C` + `go run` after backend changes; frontend changes hot-reload on their own.

### 6.5 Run the frontend

```bash
cd web
pnpm install     # first time
pnpm dev         # http://localhost:5173, /api proxied to http://localhost:8080
```

Open `http://localhost:5173` (or `http://app.janus.test:5173`). Development runs with `JANUS_COOKIE_SECURE=false`, so cookies work over HTTP.

### 6.6 Entry points

| Entry point | URL | Notes |
| --- | --- | --- |
| Console (localhost) | `http://localhost:5173` | Vite dev server with HMR |
| Console (by name) | `http://app.janus.test:5173` | Same, reached through hosts |
| Console (name + HTTPS) | `https://app.janus.test` | Caddy → Vite, to verify name / TLS / certificate shape |
| Health check | `https://app.janus.test/healthz` | `{"status":"ok"}` |
| Backend direct | `http://127.0.0.1:8080` | Bypasses Caddy / Vite, for debugging |

### 6.7 Environment variables

Development defaults work out of the box; `cp .env.example .env` when you need to override (compose reads `.env`, `go run` does not).

| Variable | Dev default | Description |
| --- | --- | --- |
| `JANUS_ADDR` | `:8080` | HTTP listen address |
| `JANUS_PLATFORM_DOMAIN` | `janus.test` | Bare platform domain; tenant default domains are `<slug>.<platform-domain>` |
| `JANUS_SERVER_PUBLIC_IP` | `127.0.0.1` | Address DNS activation checks compare against |
| `JANUS_PUBLIC_BASE_URL` | `https://app.janus.test` | Link prefix in verification / reset emails |
| `JANUS_DATABASE_URL` | `postgres://janus:janus@localhost:5432/janus?sslmode=disable` | Backend connection string |
| `JANUS_TEST_DATABASE_URL` | `…/janus_test…` | Test database (separate from the dev database) |
| `JANUS_MIGRATIONS_DIR` | `migrations` | Migration directory (`/app/migrations` in the container) |
| `JANUS_SUPERADMIN_EMAIL` | empty | Super admin email, initialized on boot (no super admin when empty) |
| `JANUS_COOKIE_SECURE` | `false` | Session cookie `Secure` flag; must be false for dev HTTP |
| `JANUS_SESSION_TTL` / `JANUS_SESSION_TTL_SHORT` | `720h` / `24h` | Remember-me / regular session |
| `JANUS_VERIFY_TOKEN_TTL` / `JANUS_RESET_TOKEN_TTL` | `24h` / `1h` | Verification / reset token lifetime |
| `JANUS_JWT_SECRET` / `JANUS_JWT_TTL` | empty / `24h` | JWT signing secret and lifetime; an empty secret is regenerated on every boot (invalidating issued tokens) and must be set in production |
| `JANUS_DNS_RETRY_INTERVAL` / `JANUS_DNS_MAX_AGE` | `5m` / `72h` | DNS retry interval / maximum wait |
| `JANUS_VISIT_RETENTION` / `JANUS_VISIT_CLEANUP_INTERVAL` | `2160h` / `24h` | Visit retention / cleanup interval |
| `JANUS_LANDING_UPLOAD_DIR` | `uploads` | Uploaded landing pages (persistent volume in production) |
| `JANUS_LANDING_MAX_ZIP_BYTES` / `JANUS_LANDING_MAX_FILES` | `10485760` / `500` | Uncompressed size limit / file count limit for landing zips |
| `JANUS_SMTP_HOST/PORT/USERNAME/PASSWORD/FROM` | empty / `465` | Real SMTP is enabled only when HOST is set; otherwise the console mailer |
| `JANUS_ACME_EMAIL` | empty | Let's Encrypt account email (production only, pairs with Caddyfile.prod) |
| `JANUS_DB_USER` / `JANUS_DB_PASSWORD` / `JANUS_DB_NAME` | `janus` ×3 | Used by the production compose to create the database; change the password in production |

> ⚠️ `.env` holds database and SMTP credentials. It is in `.gitignore` — **never commit it**.

---

## 7. Development conventions

### 7.1 Must pass before you commit

```bash
# Backend: format + tests
gofmt -l internal cmd            # no output
go vet ./...
go test ./...                    # needs the janus_test database (see 8.1)

# Frontend: types + UI gate + build
cd web
pnpm type-check
pnpm build                        # must come first
pnpm check:ui                     # passes only with zero violations (check 8 reads dist/; a
                                 # missing dist is now an error, not a silent skip)
```

> `pnpm build` must run **before** `pnpm check:ui`: check 8 verifies that the animation
> variants were actually emitted into `dist/assets`, and a missing `dist` now fails the
> gate instead of being skipped. This order is enforced by `.github/workflows/ci.yml`
> and by `docker/Dockerfile`.

### 7.2 Backend conventions

- **Layering**: `httpapi` (HTTP, auth, validation, rate limits) → `store` (data access and domain invariants) → Postgres; `rules`, `geo`, `domain`, `jwt`, `mailer` are reusable packages. The redirect hot path lives in `httpapi/redirect.go` and rule evaluation in `rules`, decoupled through narrow interfaces.
- **Comments explain "why"**: the house style records rejected alternatives and invariants (see the header comments of `internal/httpapi/redirect.go` and `internal/geo/geo.go`) instead of restating code. Read them before changing this path.
- **Every new protected route needs a Casbin policy**: append `p, tenant, <path>, <METHOD>` to `policyText` in `internal/rbac/rbac.go`; **missing it means a 403 from the authorization middleware even when logged in.** Note that under `keyMatch3` a bare path and a `/*` path are distinct policies and must be listed as a pair.
- **Uniform errors**: return `{ code, message, details }` through the helpers in `respond.go`; keep codes stable and messages human-readable.
- **Do not introduce DB queries or counter-table writes on the redirect hot path** (see the three hard constraints in 1.2).
- **Do not reinvent wheels**: networking, parsing, rate limiting, mail, migrations, config and CIDR matching all use established libraries (see 3.3). Confirm neither the standard library nor an existing dependency suffices before adding one.
- **Never invent geo values**: "not found" means empty, and an empty value makes a rule condition never match (ADR-0009).

### 7.3 Database migrations

- Migrations live in `migrations/`, named `NNNN_snake_case.sql`, executed by **goose**, with `-- +goose Up` / `-- +goose Down` headers.
- Append only: **never modify a published migration**; add a new backward-compatible one.
- After adding a migration, update the read/write code in `internal/store` and the related tests.

### 7.4 Frontend conventions

The full contract lives in [`web/UI_KIT.md`](web/UI_KIT.md); the gate script `web/scripts/check-ui-consistency.mjs` enforces it:

- **Three token layers**, defined in `:root` and `.dark` in `main.css` and mapped to Tailwind utilities through `@theme inline`: project layer (surface / ink / line / ok-warn-err), shadcn semantic layer (background / primary / destructive …), and control-state layer (control-bg / control-track / control-thumb).
- **Component state colors always use the semantic layers**: writing `dark:` patch classes inside `components/ui/` is forbidden — theme differences are expressed by the tokens themselves (e.g. a switch track is `data-[state=unchecked]:bg-control-track`, not `bg-surface-muted dark:bg-…`).
- **Only `App*` components**: no bare `<button>` / `<input>` / `<select>` / `<table>` in product views; use the wrappers in `components/ui/` (built on Reka UI headless primitives).
- **Never reintroduce ant-design-vue**; no legacy class names or legacy tokens; no hard-coded pure white `bg-white` / `text-white` (alpha variants are fine).
- **Responsive means Tailwind breakpoints**, never viewport checks in JS; where an element must adapt to its own width, use CSS container queries.
- **Dark mode**: `html.dark` is controlled by `stores/theme.ts` and persisted in localStorage.
- Icons come from `@lucide/vue`; the overview world-map dependencies must be dynamically `import()`ed so they load only on that page.

### 7.5 Order of work for a new feature

1. Align terminology first: add the new concept to [`CONTEXT.md`](CONTEXT.md) (with `_Avoid_` alternatives), and record any architectural trade-off in `docs/adr/NNNN-*.md`.
2. Migration → `store` → `httpapi` (route + RBAC policy + error codes).
3. If it touches decision logic (rule evaluation), add black-box tests in `internal/httpapi/*_test.go` using `testutil.Setup`.
4. Frontend: API types → page → route and navigation wiring.
5. Update the feature list in this README and the docs.

---

## 8. Testing

### 8.1 Backend tests

Backend tests are **black-box HTTP tests**: `httptest.Server` + a **real Postgres** + **real migrations**, with no internal mocking (`internal/testutil/testutil.go`). They need a `janus_test` database and skip automatically when it is unavailable.

```bash
docker compose up -d postgres
docker compose exec postgres psql -U janus -d janus -c 'CREATE DATABASE janus_test'

go test ./...                                   # everything
go test ./internal/httpapi/ -count=1 -v        # single package, uncached, verbose
go test ./... -cover                            # coverage
JANUS_TEST_DATABASE_URL='postgres://janus:janus@localhost:5432/other_test?sslmode=disable' go test ./...
```

Each test's `Setup` connects to the test database, applies migrations, `TRUNCATE`s business tables and re-seeds the free tier (100 links / 10 domains), so cases are isolated and repeatable. **If everything reports SKIP, the test database is missing** — create it as above.

### 8.2 Frontend quality gates

```bash
cd web
pnpm type-check   # vue-tsc --noEmit
pnpm build        # emit dist first — check 8 reads it
pnpm check:ui     # UI gate: legacy classes/tokens, undefined CSS variables, palette leaks,
                  # arbitrary font sizes, bare HTML primitives, state-color contrast in
                  # both themes, hard-coded pure white
```

The frontend has no unit-test framework; quality comes from type checking, the gate script and end-to-end verification.

### 8.3 End-to-end verification (curl)

With no SMTP configured, the verification link is printed in the terminal running the backend.

```bash
BASE=https://app.janus.test

# 1. Register → 201, status=pending, returns defaultDomain:"alice.janus.test"
curl -sk -X POST "$BASE/api/auth/register" -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"password123","slug":"alice"}'

# 2. Copy the verification link from the backend terminal ([Janus mailer] block)
#    https://app.janus.test/verify-email?token=<token>
curl -sk -X POST "$BASE/api/auth/verify-email" -H 'Content-Type: application/json' \
  -d '{"token":"<token>"}'                                # → {"status":"ok"}

# 3. Log in and keep the cookies
JAR=$(mktemp /tmp/janus-cookies.XXXXXX)
curl -sk -c "$JAR" -b "$JAR" -X POST "$BASE/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"password123","rememberMe":true}'

# 4. Bearer JWT for scripts (no cookies, no CSRF)
TOKEN=$(curl -sk -X POST "$BASE/api/auth/token" -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"password123"}' \
  | grep -o '"accessToken":"[^"]*"' | cut -d'"' -f4)
curl -sk "$BASE/api/auth/me" -H "Authorization: Bearer $TOKEN"

# 5. Read domains with Bearer; writes need the CSRF double-submit token
curl -sk "$BASE/api/domains" -H "Authorization: Bearer $TOKEN"
CSRF=$(awk '$6=="janus_csrf"{print $7}' "$JAR")
curl -sk -c "$JAR" -b "$JAR" "$BASE/api/domains"

# 6. Create a link (multiple targets are picked round-robin)
curl -sk -c "$JAR" -b "$JAR" -X POST "$BASE/api/links" \
  -H 'Content-Type: application/json' -H "X-CSRF-Token: $CSRF" \
  -d '{"targetUrls":["https://example.com","https://example.org"],"domainIds":[1]}'

# 7. Redirect: resolve the subdomain temporarily, no hosts edits needed
curl -sk -o /dev/null -w '%{http_code} %{redirect_url}\n' \
  --resolve alice.janus.test:443:127.0.0.1 "https://alice.janus.test/<code>"
# → 302 https://example.com/ or https://example.org/ (round-robin)

# 8. Miss → 404
curl -sk -o /dev/null -w '%{http_code}\n' \
  --resolve alice.janus.test:443:127.0.0.1 "https://alice.janus.test/not-exist"
```

Browser walkthrough (mirrors the production go-live checklist): open `https://app.janus.test` → register → verify → log in and see the default domain `active` → create a link → visit the code and see a 302 with the visit count +1 → visit a nonexistent code and get 404 → (optional) add a self-owned domain and exercise deactivate/restore/delete → (optional) log in as super admin and open the platform admin views.

### 8.4 Test data and reset

- The dev database `janus` and the test database `janus_test` are separate; `go test` only touches `janus_test`.
- To wipe all development data (volumes included — the certificate cache and local CA go with them, so re-trust afterwards):

```bash
docker compose down -v && docker compose up -d
```

---

## 9. Development vs. production

| Dimension | Development | Production |
| --- | --- | --- |
| Orchestration | Infrastructure in Docker (Postgres + Caddy); backend/frontend in a terminal (`go run` + `pnpm dev`) | `docker compose -f docker-compose.prod.yml up -d` (everything containerized) |
| Ports | Backend 8080; Caddy maps host 443/80 | Standard 80/443; backend exposes no host port |
| Name resolution | hosts points `app.janus.test` and test subdomains at `127.0.0.1` (`JANUS_SERVER_PUBLIC_IP=127.0.0.1`; Go reads hosts, exercising the real path) | Real DNS wildcard `*.<platform-domain>` |
| Certificates | Caddy local CA (`tls internal` + on-demand), trusted via `caddy trust` | Let's Encrypt (ACME, automatic issue/renew) |
| Mail | Console mailer (links printed in the backend log) | Real SMTP (credentials supplied by the deployer) |
| Cookies | `JANUS_COOKIE_SECURE=false` (HTTP) | Forced `true` (HTTPS) |
| Platform domain | `janus.test` (RFC-reserved test domain) | A real domain |
| Frontend assets | Vite dev server (5173) | Static build in the nginx image (built during image build, never committed) |
| Landing uploads | Local `uploads/` directory | Persistent `uploads` volume |

---

## 10. Production deployment (brief)

The full procedure lives in **[`docs/deploy.md`](docs/deploy.md)**. The short version:

```bash
# 1. DNS: A/AAAA for the platform domain + wildcard *.<platform-domain> → server public IP
dig +short example.com && dig +short any.example.com

# 2. Configure the environment
cp .env.example .env && chmod 600 .env
#    required: JANUS_PLATFORM_DOMAIN, JANUS_SERVER_PUBLIC_IP, JANUS_DB_PASSWORD
#    recommended: JANUS_SUPERADMIN_EMAIL, JANUS_ACME_EMAIL, JWT secret (see 6.7)

# 3. Caddyfile.prod has no env placeholders — replace the site address
sed -i '' 's/example\.com/YOUR-DOMAIN/g' Caddyfile.prod   # macOS
sed -i 's/example\.com/YOUR-DOMAIN/g' Caddyfile.prod      # Linux

# 4. Launch
docker compose -f docker-compose.prod.yml config --quiet
docker compose -f docker-compose.prod.yml up -d --build
docker compose -f docker-compose.prod.yml ps
```

**First boot does this automatically**: connect → goose migrations → seed the free tier → idempotently initialize the super admin from `JANUS_SUPERADMIN_EMAIL` → start background workers → listen on 8080. Certificates are issued **on demand**: once a tenant domain is activated, Caddy asks the authorization endpoint on the first visit and issues a Let's Encrypt certificate, renewing before expiry.

**Release and rollback**: a frontend change rebuilds only the `web` image, a backend change only the `backend` image (they never block each other). Images carry no version tag, so rollback means `git checkout <last good commit>` followed by `up -d --build`.

**Backups**: `pgdata` is mandatory (it holds tenants, links, visits and rules); `caddy_data` is recommended (losing it burns Let's Encrypt issuance quota); the `uploads` volume needs its own backup for hosted landing pages.

**Go-live checks**: `/healthz` returns ok, the console presents a Let's Encrypt certificate, registration receives a verification email, `<slug>.<platform-domain>` links redirect and the visit count grows, self-owned domains move `pending → active`, and the super admin can govern tenants.

### Known limitations

- **Let's Encrypt quota**: platform default domains are issued per subdomain, subject to "50 certificates per registered domain per week (including renewals)", aggregated per platform domain (ADR-0004). Plan accordingly as you scale.
- **Visits are retained for 90 days**, then pruned by a background worker; tune with `JANUS_VISIT_RETENTION`.
- **Geo is an offline database**: `asn` and `is_datacenter` have no source, so rules depending on them never match (by design, values are never guessed). The xdb files in `internal/geo/data/` must be updated manually to recognize new IP ranges.
- **The authorization endpoint is internal-only**: inactive domains are refused certificates. Never expose it publicly.
- **Rule limits**: 200 rules per tenant; snapshots have a 1-minute TTL and are actively invalidated on write (forgetting to invalidate means a freshly saved rule silently does nothing).

---

## 11. Troubleshooting

**Development**

| Symptom | Cause / fix |
| --- | --- |
| Certificate untrusted / `curl: (60)` | Local CA not trusted: run 6.3; use `curl -k` while debugging |
| Vite returns 403 by domain | Host allowlist mismatch: check `VITE_PLATFORM_DOMAIN` in `web/.env.development` |
| Caddy 502 | Backend is not running: confirm the `go run` from 6.4 and that 8080 answers `curl -s http://127.0.0.1:8080/healthz` |
| Registration returns 409 | Email or slug taken (leftover dev data): pick another slug or reset with 8.4 |
| Self-owned domain stuck `pending` | hosts entry missing or not pointing at 127.0.0.1; or the 5-minute retry hasn't fired — use "re-check" in the console |
| Domain `failed` | Not validated within 72 hours: check hosts and `JANUS_SERVER_PUBLIC_IP` |
| Redirect returns 404 | Unknown code, disabled domain/link, or the Host is not an active domain of that tenant |
| No email, and no `[Janus mailer]` in the terminal | `JANUS_SMTP_*` was exported, so real SMTP is used; unset them to fall back to the console mailer |
| Go changes have no effect | The dev backend is not in Docker: `Ctrl+C` and re-run `go run` |
| Frontend changes have no effect | Use the 6.5 dev server; if you are looking at the image, run `pnpm build` and rebuild the `web` image |
| Every test reports SKIP | The `janus_test` database does not exist: create it per 8.1 |
| Ports 80/443 busy | The dev compose Caddy takes them; do not run it alongside other services on those ports. If 8080 is taken, change `JANUS_ADDR` and update the proxy config |
| `pnpm check:ui` reports a contrast violation | A token value was changed without checking both `:root` and `.dark`; see `web/UI_KIT.md` |

**Production**

| Symptom | Cause / fix |
| --- | --- |
| compose fails with `JANUS_XXX must be set` | `.env` lacks a required variable (compose enforces with `:?`): fill in per 10 and 6.7 |
| Caddy fails with `expanding email address ... is empty` | `JANUS_ACME_EMAIL` is empty while `Caddyfile.prod` enables the `email` directive: set the variable or comment the directive |
| Certificates not issued / `cert_status=failed` | Read `docker compose logs caddy`; verify DNS, 80/443 reachability and that the authorization endpoint allows the domain |
| Platform default domain unreachable | Wildcard `*.<platform-domain>` missing or not propagated: verify with `dig` |
| Visitors see 404 | Domain/link disabled or deleted; or a rule returned a `notfound` verdict; or the Host is not an active domain |
| Visitors see 429 | A rule returned a `throttle` verdict: inspect priorities and conditions in the rule list |
| No verification/reset emails | Check the `.env` SMTP settings and `docker compose logs backend` |
| Stale UI after deploy | The frontend changed but the `web` image was not rebuilt: `docker compose -f docker-compose.prod.yml up -d --build web` |
| Disk pressure | Images and logs accumulate: `docker system prune` (carefully); watch `pgdata` and traffic |

---

## 12. API overview

**Authentication**

- Console: session cookie `janus_session` (HTTP-only / Secure / SameSite=Lax); write methods must send `X-CSRF-Token` (value from the `janus_csrf` cookie — double submit).
- Scripts: `POST /api/auth/token` returns an `accessToken`; send `Authorization: Bearer <token>` afterwards (header auth is CSRF-immune).
- Redirects: `GET /{code}`, domain resolved from the Host header, no auth.
- Internal: `GET /internal/caddy/authorize?domain=<fqdn>`, internal network only.

**Main endpoints**

| Group | Endpoints |
| --- | --- |
| Auth | `POST /api/auth/register`, `verify-email`, `login`, `token`, `logout`, `change-password`, `forgot-password`, `reset-password`; `GET /api/auth/me` |
| Domains | `GET/POST /api/domains`; `GET/PATCH/DELETE /api/domains/{id}`; `POST /api/domains/{id}/recheck` |
| Links | `GET/POST /api/links`; `GET/PATCH/DELETE /api/links/{id}`; `POST /api/links/{id}/purge`; `POST /api/links/batch-delete`; `POST /api/links/batch-purge`; `POST /api/links/{id}/landing`; `GET /api/links/{id}/visits`; `GET /api/links/{id}/stats`; `GET/PUT /api/links/{id}/rules` |
| Rules | `GET/POST /api/rules`; `GET /api/rules/options`; `GET/PATCH/DELETE /api/rules/{id}`; `POST /api/rules/simulate`; `POST /api/rules/validate-expr` |
| Tenant settings | `GET/PATCH /api/me`; `GET/PATCH /api/me/error-pages`; `GET /api/config` |
| Platform admin | `GET /api/admin/tenants`, `/api/admin/tenants/{id}`, `/api/admin/tiers`; `PATCH /api/admin/tenants/{id}`; `DELETE /api/admin/domains/{id}` |
| Redirects (public) | `GET /{code}`; landing links add `GET /{code}/`, `/{code}/click`, `/{code}/sdk.js`, `/{code}/<static file>` |
| Ops | `GET /healthz` |

**Authorization model**: Casbin, roles `tenant` / `superadmin`, authorizing on (role, method, path); the policy is loaded in memory from `policyText` in `internal/rbac/rbac.go`. **Every new protected route must add a policy**, otherwise authenticated requests still get a 403.

**Uniform error response**

```json
{ "code": "E_DOMAIN_LIMIT", "message": "域名数量已达上限 (5/10)", "details": {} }
```

---

## 13. Contributing

**Getting set up**: bring the development environment up following [6. Quick start](#6-quick-start-development) and create the test database as described in 8.1.

**Pre-commit checklist**

```bash
gofmt -l internal cmd      # no output
go vet ./...
go test ./...
cd web && pnpm type-check && pnpm check:ui && pnpm build
```

**Code style**

- Go: `gofmt`; comments explain "why" and invariants rather than restating code; check the standard library and existing dependencies before adding a new one (this repo deliberately delegates migrations, rate limiting, mail, config, CIDR matching and expression evaluation to established libraries).
- Vue: three token layers + `App*` components (see 7.4); no ant-design-vue; no `dark:` patches inside `components/ui/`; no bare HTML primitives; responsive behavior only via Tailwind breakpoints or container queries.
- Database: migrations are append-only; published migrations are never modified.

**Issues and specs**: tickets and specifications are markdown files under `.scratch/<feature-slug>/` (see [`docs/agents/issue-tracker.md`](docs/agents/issue-tracker.md)); terminology changes go to [`CONTEXT.md`](CONTEXT.md) (see [`docs/agents/domain.md`](docs/agents/domain.md)); architectural trade-offs go to `docs/adr/NNNN-*.md`, following the format of the existing ADRs (background → trade-offs → consequences). **Non-obvious decisions deserve an ADR, not just a commit message.**

---

## 14. License

Janus is licensed under the **GNU Affero General Public License v3.0 (AGPL-3.0)**. The full text is in [`LICENSE`](LICENSE).

```
Copyright (C) 2026 ruanbw and Janus contributors
SPDX-License-Identifier: AGPL-3.0-only
```

In short:

- You may use, modify, redistribute and integrate the software, including in closed-source commercial products (keep the copyright and license notices; see AGPL §4–6 for how source must be offered).
- **The key difference from GPL**: if you make the functionality available to users over a network — that is, by running it as a SaaS or online service — you must offer those users the corresponding source code. This matters for Janus in particular: providing Janus's online redirect or management service requires releasing the source of your modifications.
- The software is provided without warranty of any kind (see AGPL §15–17).
- For commercial licensing or closed-source distribution, contact the author separately.

**Third-party components**: this project depends on a number of third-party open-source components (Go ecosystem: gin, GORM, casbin, goose, go-mail, …; frontend: Vue, Tailwind CSS, Reka UI, d3-geo, topojson-client, world-atlas, …). Each keeps its own license. Notably `world-atlas` is ISC-licensed (redistribution permitted); its notices must be preserved when shipping it.

---

## 15. Further reading

| Document | Contents |
| --- | --- |
| [`CONTEXT.md`](CONTEXT.md) | **Domain glossary** — canonical definitions of tenant, link, short code, landing page, rule, verdict, visit, … |
| [`docs/deploy.md`](docs/deploy.md) | Production deployment guide and go-live checklist |
| [`web/README.md`](web/README.md) | Frontend stack, page inventory, auth and request conventions |
| [`web/UI_KIT.md`](web/UI_KIT.md) | Frontend component contract and the three design-token layers |
| [`docs/adr/`](docs/adr/) | Architecture decision records (Go backend, on-demand TLS, frontend/backend separation, landing-page SDK, visit detail, rule verdicts, GeoIP, world map) |
| [`docs/agents/`](docs/agents/) | Agent collaboration conventions |
| [`.scratch/janus/`](.scratch/janus/) | Requirement spec, full API contract, per-feature tickets |
| [`.env.example`](.env.example) | Environment variable template with per-variable notes |

**In one line**: Janus is a short-link service that works the moment DNS points at it — tenants bring domains, and certificates, analytics and rules come included.
