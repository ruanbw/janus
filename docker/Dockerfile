# CLOAK 后端镜像:多阶段构建,单二进制 + 迁移文件(alpine)
FROM golang:1.26-alpine AS build
# 构建期模块代理(默认 proxy.golang.org,网络受限环境用 --build-arg GOPROXY=... 覆盖)
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=$GOPROXY
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/cloak ./cmd/cloak

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/cloak /app/cloak
COPY migrations /app/migrations
EXPOSE 8080
CMD ["/app/cloak"]
