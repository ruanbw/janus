<div align="center">

# Janus

**每一次跳转，皆为裁决。**
*Every jump is a verdict.*

[![license](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)](LICENSE)
[![go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](go.mod)
[![node](https://img.shields.io/badge/Node-%E2%89%A520.19-339933?logo=nodedotjs&logoColor=white)](web/package.json)
[![postgres](https://img.shields.io/badge/PostgreSQL-16-4169E1?logo=postgresql&logoColor=white)](docker-compose.yml)

[中文](README.md) · [English](README.en.md) · [Product page](https://ruanbw.github.io/janus/)

</div>

---

## What it is

Janus is a **self-hosted multi-tenant short-link platform**. Tenants sign up and manage their own domains and links; the system issues and renews HTTPS certificates automatically for every activated domain and redirects `domain/code` hits to the target URL.

Beyond redirects it ships landing-page links, visit detail with geographic attribution, a tenant-level access rule engine, custom error pages, and a Bearer JWT API for scripts.

- **Single-server deployment** — Go (Gin + GORM) + Vue 3 SPA (nginx) + PostgreSQL 16 + Caddy (on-demand TLS). `docker compose up -d` brings it all up.
- **Tenant isolation** — domains, links, rules and landing pages are fully isolated per tenant; the deployer holds a platform-admin role.
- **Certificates included** — one wildcard DNS record on the platform domain gives every tenant `<slug>.<platform-domain>`; tenant-owned domains get Let's Encrypt certificates from Caddy once activated.
- **Frontend/backend separation** — the frontend builds into its own nginx image; the backend image contains zero frontend files (ADR-0006).

```
Internet → Caddy (80/443, on-demand TLS)
             ├── /                 → nginx (frontend SPA)
             └── /api/*, /{code}   → Go backend → PostgreSQL 16
```

## Repository layout

```
cmd/janus/            # backend entrypoint (config → DB → init script → workers → HTTP)
internal/             # httpapi (routes/handlers), rules (rule engine), store, domain, geo, rbac…
migrations/init.sql   # single source of truth for schema, idempotent, run at startup
web/                  # frontend SPA (Vue 3 + Vite + Tailwind 4)
docker/               # Dockerfile (backend / web targets) + nginx.conf
docker-compose.yml    # development orchestration (Postgres + Caddy)
docker-compose.prod.yml, Caddyfile.prod  # production orchestration
docs/                 # deploy.md, testing.md, adr/
```

## Requirements

| Tool | Version |
| --- | --- |
| Docker Desktop (with Compose v2) | recent |
| Go | ≥ 1.26 |
| Node.js | ≥ 20.19 |
| pnpm | ≥ 9 (repo pins 10.x) |
| Caddy CLI (optional) | ≥ 2.x, to trust the local CA |

In development **infrastructure runs in Docker** (Postgres, Caddy) while the **backend and frontend run in your terminal**, so code changes take effect immediately.

## Quick start (development)

### 1. Start infrastructure

```bash
# generate .env from the template with the required on-demand TLS secret filled in
# NOTE: the redirect overwrites an existing .env (SMTP credentials, admin email) — back it up first
sed "s/^JANUS_CADDY_ASK_TOKEN=.*/JANUS_CADDY_ASK_TOKEN=$(openssl rand -hex 32)/" .env.example > .env
docker compose up -d    # postgres:5432, caddy:80/443
```

### 2. Local DNS

Edit `/etc/hosts` (needs admin / sudo):

```text
127.0.0.1 app.janus.test
127.0.0.1 alice.janus.test      # one line per tenant; hosts has no wildcards
```

These lines only serve the browser and Caddy. Platform default subdomains `<slug>.janus.test` are active on
creation and are never DNS-checked; activation of tenant-owned domains uses real DNS (deliberately bypassing
`/etc/hosts`), so adding hosts lines does not help there — see `docs/deploy.md` §5.

### 3. Trust the Caddy local CA

```bash
mkdir -p certs
docker compose exec caddy cat /data/caddy/pki/authorities/local/root.crt > certs/caddy-root.pem
sudo caddy trust --ca certs/caddy-root.pem
```

### 4. Start the backend

Run these from the **repository root** (`JANUS_MIGRATIONS_DIR` defaults to the relative path `migrations`):

```bash
# go run does not read .env (that is compose's behaviour): inject it into the process environment
set -a; . ./.env; set +a
JANUS_COOKIE_SECURE=false JANUS_ADDR=:8080 go run ./cmd/janus
```

Skipping the `set -a; . ./.env` line breaks TLS: the backend only reads `JANUS_CADDY_ASK_TOKEN` from the process
environment, while the Caddy container uses the copy in `.env`. When the two differ, the authorize endpoint
rejects everything (fail-closed) and Caddy can issue no certificate at all. `./run_backend.sh` does the same
thing (and `cd`s to the repository root first).

### 5. Start the frontend

```bash
corepack enable                        # if pnpm is missing: corepack installs the pinned 10.x
cd web && pnpm install && pnpm dev     # http://localhost:5173, /api proxied to :8080
```

### 6. Verify

```bash
curl -s  http://127.0.0.1:8080/healthz     # → {"status":"ok"}
curl -sk https://app.janus.test/healthz    # full chain through Caddy
```

Open `http://localhost:5173` and register an account; without SMTP configured, verification emails are printed to the backend terminal.

> Caddy binds host ports 80/443 — start it at different times from any other service using those ports.

To reach the **platform admin** UI, set `JANUS_SUPERADMIN_EMAIL=admin@janus.test` in `.env` and restart the
backend (idempotently initialised at startup). The super admin's first login requires the one-time setup token
sent by email, and the login page has no field for it yet — the first login is a three-step API flow, see
[`docs/deploy.md`](docs/deploy.md) §3 (`BASE=http://127.0.0.1:8080`, token from the backend terminal log).

### 7. Reset the development database

After changing `migrations/init.sql` you must recreate the database together with its data (the project keeps
no incremental migrations):

```bash
docker compose down -v && docker compose up -d     # -v drops the pgdata volume, all data gone
```

`-v` also drops `caddy_data`, which recreates the local CA root: redo step 3 (`caddy trust`) or the browser
will reject `https://*.janus.test` certificates.

## Configuration

Everything is configured through environment variables; see [`.env.example`](.env.example) for the full annotated template. Common ones:

| Variable | Dev default | Purpose |
| --- | --- | --- |
| `JANUS_ADDR` | `:8080` | HTTP listen address |
| `JANUS_PLATFORM_DOMAIN` | `janus.test` | Bare platform domain; tenant default domain is `<slug>.<platform-domain>` |
| `JANUS_SERVER_PUBLIC_IP` | `127.0.0.1` | IP compared during DNS activation of tenant-owned domains |
| `JANUS_PUBLIC_BASE_URL` | `https://app.janus.test` | Prefix for verification / reset links in emails |
| `JANUS_DATABASE_URL` | `postgres://janus:janus@localhost:5432/janus?sslmode=disable` | Backend connection string |
| `JANUS_SUPERADMIN_EMAIL` | empty | Platform admin email, idempotently created at startup; empty means no admin |
| `JANUS_COOKIE_SECURE` | `false` | Session cookie Secure flag — must be false over http |
| `JANUS_JWT_SECRET` | empty | JWT signing key; empty regenerates per start (issued tokens die on restart) |
| `JANUS_CADDY_ASK_TOKEN` | empty (**required**) | Shared secret for the on-demand TLS authorize endpoint, injected into both compose and backend |
| `JANUS_DNS_RETRY_INTERVAL` / `JANUS_DNS_MAX_AGE` | `5m` / `72h` | Domain activation retry interval / maximum wait |
| `JANUS_VISIT_RETENTION` | `2160h` | Visit retention (90 days) |
| `JANUS_SMTP_*` | empty | Real SMTP only when `HOST` is set; otherwise mails are logged |

> `.env` holds database and SMTP credentials and is git-ignored.

## Production deployment

Full steps and a go-live checklist live in **[`docs/deploy.md`](docs/deploy.md)** (currently written in Chinese). Minimal flow:

```bash
cp .env.example .env && chmod 600 .env
# These are hard requirements — compose exits immediately when they are empty:
#   JANUS_PLATFORM_DOMAIN     bare platform domain, e.g. example.com
#   JANUS_SERVER_PUBLIC_IP    public IP of the server
#   JANUS_DB_PASSWORD         do not keep the template's `janus`
#   JANUS_TRUSTED_PROXY_CIDRS 172.16.0.0/12 (default Docker bridge range)
#   JANUS_CADDY_ASK_TOKEN     openssl rand -hex 32
# Recommended as well: JANUS_JWT_SECRET (openssl rand -hex 32) and JANUS_SUPERADMIN_EMAIL.

sed -i 's/example\.com/YOUR-DOMAIN/g' Caddyfile.prod   # site addresses cannot use env vars
docker compose -f docker-compose.prod.yml up -d --build
```

Prerequisite DNS: both the bare platform domain and the wildcard `*.<platform-domain>` must point at the server's public IP. First boot runs the init script, seeds the free tier and creates the super admin; certificates are issued on demand.

Three things this snippet does not cover: **super admin first login** (it needs the setup token mailed to
`JANUS_SUPERADMIN_EMAIL` — `docs/deploy.md` §3), **upgrade / rollback** (§9) and **backups** (especially the
`caddy_data` volume: losing it forces every certificate to be re-issued — §10).

## Development conventions

- **Before committing**: backend `gofmt -l internal cmd`, `go vet ./...`, targeted `go test -count=1 ./internal/<pkg>/`; frontend `pnpm type-check` + `pnpm build` + `pnpm check:ui`.
- **Tests are the spec**: requirements live as Given-When-Then tests; run only the targeted package tests you touched, never `go test ./...`. See [`docs/testing.md`](docs/testing.md).
- **Database**: `migrations/init.sql` is the single source of truth for the schema — no incremental migrations; change the script and recreate the dev database.
- **New protected routes**: add the matching `policyText` entry in `internal/rbac/rbac.go`, or the request is denied with 403.
- **Architecture trade-offs**: record them in `docs/adr/NNNN-*.md` (context → trade-offs → consequences).

## Documentation

| Document | Content |
| --- | --- |
| [`docs/deploy.md`](docs/deploy.md) | Production deployment guide and go-live checklist |
| [`docs/testing.md`](docs/testing.md) | Test workflow and layered verification |
| [`web/README.md`](web/README.md) | Frontend stack, page inventory, request conventions |
| [`web/UI_KIT.md`](web/UI_KIT.md) | Frontend component contract and three-layer design tokens |
| [`docs/adr/`](docs/adr/) | Architecture decision records |

## License

**GNU Affero General Public License v3.0 (AGPL-3.0)** — full text in [`LICENSE`](LICENSE).

```
Copyright (C) 2026 ruanbw and Janus contributors
SPDX-License-Identifier: AGPL-3.0-only
```

Free to use, modify and redistribute, including in closed-source commercial settings; but if you offer the program's functionality to users over a network (i.e. run it as a service), you must provide those users with the corresponding source. Contact the author for commercial licensing or closed-source distribution.
