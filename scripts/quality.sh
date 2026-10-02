#!/usr/bin/env bash
# 分层机械防线 (Mechanical Gates)。
#
# 背景:所有 DB 黑盒测试共用同一个 janus_test 库,各包 Setup 会 TRUNCATE 业务表。
# advisory lock 只能跨包互斥,挡不住上一个包遗留的后台 goroutine 继续写库,
# 于是 `go test ./...`(无论并行还是 -p 1)都会非确定性失败。
# 因此本脚本为每个包创建独立测试库,让每一层门禁都可复现。
#
# 用法:
#   ./scripts/quality.sh tests [profile-out]   # 每包独立库跑测试,合并覆盖率
#   ./scripts/quality.sh crap                  # CRAP 复杂度/覆盖率门禁
#   ./scripts/quality.sh mutate <file.go>...   # 对指定文件做变异测试
#   ./scripts/quality.sh all                   # 全部 (等价于 tests + crap + mutate)
#
# 环境变量:
#   JANUS_ADMIN_DATABASE_URL  维护库连接串(需 CREATEDB),默认 janus 本地库
#   JANUS_GATE_DB_PREFIX      临时库名前缀,默认 janus_gate
#   GATE_PKGS                 覆盖待测包列表(默认 go list ./...)
#   CRAP_MAX                  CRAP 分数上限,默认 12
set -euo pipefail

cd "$(dirname "$0")/.."

# libpq 在 Homebrew 下是 keg-only,createdb/dropdb 不在默认 PATH。
for _d in /usr/local/opt/libpq/bin /opt/homebrew/opt/libpq/bin; do
  if [ -x "$_d/createdb" ]; then PATH="$_d:$PATH"; break; fi
done
# crap4go / mutate4go 通过 `go install` 装在 GOPATH/bin。
PATH="$(go env GOPATH)/bin:$PATH"
export PATH

for _c in createdb dropdb go; do
  command -v "$_c" >/dev/null || { echo "缺少工具: $_c" >&2; exit 1; }
done

ADMIN_DSN="${JANUS_ADMIN_DATABASE_URL:-postgres://janus:janus@localhost:5432/postgres?sslmode=disable}"
DB_PREFIX="${JANUS_GATE_DB_PREFIX:-janus_gate}"
CRAP_MAX="${CRAP_MAX:-12}"

# 从 DSN 解析 libpq 需要的连接参数。
_dsn="${ADMIN_DSN#postgres://}"
_creds="${_dsn%%@*}"; _hostpart="${_dsn#*@}"
export PGUSER="${_creds%%:*}"
export PGPASSWORD="${_creds#*:}"; [ "$PGPASSWORD" = "$_creds" ] && PGPASSWORD=""
export PGHOST="${_hostpart%%:*}"
_rest="${_hostpart#*:}"; export PGPORT="${_rest%%/*}"; [ "$PGPORT" = "$_rest" ] && PGPORT=5432
export PGCONNECT_TIMEOUT=10

PROFILE_DIR="target/gate"
mkdir -p "$PROFILE_DIR"

sanitize() { printf '%s' "$1" | tr -c 'a-zA-Z0-9' '_'; }

# 每个包一个独立库:建库 -> 跑测试 -> 收集覆盖率 -> 删库。
run_isolated_tests() {
  local merged="${1:-}"
  local pkgs="${GATE_PKGS:-$(go list ./...)}"
  local failed=0 tmp
  tmp="$(mktemp -d)"
  : > "$PROFILE_DIR/all.out"

  for pkg in $pkgs; do
    local db="${DB_PREFIX}_$(sanitize "$pkg")"
    dropdb --if-exists --force "$db" >/dev/null 2>&1 || true
    createdb "$db"
    echo "==> $pkg  (db: $db)"
    if JANUS_TEST_DATABASE_URL="postgres://${PGUSER}:${PGPASSWORD}@${PGHOST}:${PGPORT}/${db}?sslmode=disable" \
       go test -count=1 -covermode=atomic -coverprofile="$tmp/$(sanitize "$pkg").out" "$pkg"; then
      cat "$tmp/$(sanitize "$pkg").out" >> "$PROFILE_DIR/all.out"
    else
      failed=1
      echo "!!! $pkg FAILED" >&2
    fi
    dropdb --if-exists --force "$db" >/dev/null 2>&1 || true
  done

  rm -rf "$tmp"
  [ -n "$merged" ] && cp "$PROFILE_DIR/all.out" "$merged"
  return $failed
}

layer_tests() {
  # crap4go 会把 {coverprofile} 替换成 "-coverprofile=<path>" 传进来。
  local dest="${1:-}"
  case "$dest" in
    -coverprofile=*) dest="${dest#-coverprofile=}" ;;
  esac
  run_isolated_tests "${dest:-$PROFILE_DIR/coverage.out}"
}

layer_crap() {
  command -v crap4go >/dev/null || { echo "缺少 crap4go: go install github.com/unclebob/crap4go/cmd/crap4go@latest" >&2; exit 1; }
  mkdir -p "$PROFILE_DIR"
  # 复用同一套隔离测试,避免 crap4go 自己再跑一遍共享库的 go test。
  crap4go --test-command "./scripts/quality.sh tests {coverprofile}"
}

layer_mutate() {
  command -v mutate4go >/dev/null || { echo "缺少 mutate4go: go install github.com/unclebob/mutate4go/cmd/mutate4go@latest" >&2; exit 1; }
  [ "$#" -eq 0 ] && { echo "用法: $0 mutate <file.go>..." >&2; exit 1; }
  local f rc=0
  for f in "$@"; do
    echo "==> mutate $f"
    mutate4go "$f" --test-command "go test -count=1 ./$(dirname "$f")/..." || rc=1
  done
  return $rc
}

layer_mutate_changed() {
  local base="${1:-HEAD}"
  local files
  files="$(git diff --name-only --diff-filter=ACMR "$base" -- '*.go' | grep -v '_test\.go$' || true)"
  [ -z "$files" ] && { echo "==> 无变更的 .go 文件,跳过变异测试"; return 0; }
  # shellcheck disable=SC2086
  layer_mutate $files
}

cmd="${1:-all}"; shift || true
case "$cmd" in
  tests)  layer_tests "$@" ;;
  crap)   layer_crap ;;
  mutate) layer_mutate "$@" ;;
  changed) layer_mutate_changed "$@" ;;
  all)
    layer_tests
    layer_crap
    layer_mutate_changed
    ;;
  *) echo "未知命令: $cmd" >&2; exit 2 ;;
esac