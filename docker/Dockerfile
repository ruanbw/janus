# CLOAK 镜像:前后端分离(ADR-0006),两个独立构建目标,互不依赖。
#   backend = Go API 服务(纯后端,镜像内无任何前端文件)
#   web     = nginx 静态服务(纯前端,镜像内无任何后端代码)
# 生产由 Caddy 按路径分流:/api、/internal、/healthz → backend,其余 → web。
# 前端改版只重建 web 镜像,后端改版只重建 backend 镜像,无需交叉重建。

# ---------- 前端构建 ----------
FROM node:22-alpine AS web-build
# 构建期 npm registry(默认官方源,网络受限环境用 --build-arg NPM_REGISTRY=... 覆盖)
ARG NPM_REGISTRY=https://registry.npmjs.org
ENV NPM_CONFIG_REGISTRY=$NPM_REGISTRY
WORKDIR /web
# corepack 按 package.json 的 packageManager 字段启用 pnpm 版本
RUN corepack enable
# 先装依赖:package.json / lock 文件不变时该层可命中缓存
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
# 门禁必须在 build **之前**跑完，且必须非零即失败:
#   type-check 挡住类型错误，check:ui 挡住 UI 规范违规。
# 放在这里而不是只写在 README 里，是因为 README 的清单只对"记得手动跑"的人有效 ——
# 违规前端曾经可以直接进生产镜像。check:ui 的第 8 项要读 dist 产物，
# 所以它的 build 依赖由 CI workflow 里的顺序保证(见 .github/workflows/ci.yml)。
RUN pnpm type-check && pnpm build && pnpm check:ui

# ---------- 前端产物(nginx) ----------
FROM nginx:1.29-alpine AS web
COPY docker/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=web-build /web/dist /usr/share/nginx/html
EXPOSE 80

# ---------- 后端构建 ----------
FROM golang:1.26-alpine AS backend-build
# 构建期模块代理(默认 proxy.golang.org,网络受限环境用 --build-arg GOPROXY=... 覆盖)
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=$GOPROXY
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY migrations/ migrations/
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/cloak ./cmd/cloak

# ---------- 后端产物 ----------
FROM alpine:3.21 AS backend
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=backend-build /out/cloak /app/cloak
COPY migrations /app/migrations
EXPOSE 8080
CMD ["/app/cloak"]
