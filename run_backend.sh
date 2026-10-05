#!/bin/bash
export JANUS_COOKIE_SECURE=false
export JANUS_ADDR=:8080
export JANUS_PLATFORM_DOMAIN=cloak.test
export JANUS_PUBLIC_BASE_URL=https://app.cloak.test
# 从 .env 读取配置（包括 JANUS_CADDY_ASK_TOKEN 等）
if [ -f .env ]; then
  set -a
  source .env
  set +a
fi
export JANUS_PLATFORM_DOMAIN=cloak.test
export JANUS_PUBLIC_BASE_URL=https://app.cloak.test
exec go run ./cmd/janus
