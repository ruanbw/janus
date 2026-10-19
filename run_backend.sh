#!/bin/bash
# 开发环境启动后端(等价于 README「快速开始」第 4 步):
#   go run 不读 .env,因此这里用 set -a 把 .env 注入进程环境 —— 关键是 JANUS_CADDY_ASK_TOKEN,
#   后端只从进程环境读它,而 Caddy 容器用的是 .env 里那一份。缺了它授权端点一律 403
#   (fail-closed),表现为 Caddy 一张证书都签不出来:泛域名 / 自有域名握手全部失败。
set -euo pipefail

# JANUS_MIGRATIONS_DIR 的默认值是相对路径 migrations/,所以必须先切到仓库根
cd "$(cd "$(dirname "$0")" && pwd)"

if [ ! -f .env ]; then
	echo "缺少 .env:先按 README「快速开始」第 1 步生成(其中 JANUS_CADDY_ASK_TOKEN 必填)" >&2
	exit 1
fi

set -a
# shellcheck disable=SC1091
. ./.env
set +a

# http 开发必须 false:显式覆盖,避免 .env 被改成 true 后本地登录拿不到 cookie
export JANUS_COOKIE_SECURE=false
export JANUS_ADDR=:8080

exec go run ./cmd/janus
