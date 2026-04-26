#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost}"
TEST_USER="${TEST_USER:-codex_e2e_smoke}"
TEST_PASS="${TEST_PASS:-CodexTest123!}"
INVITE_CODE="${INVITE_CODE:-codex-e2e-smoke-invite}"
FULL=0
MOCK=0

for arg in "$@"; do
  case "$arg" in
    --full) FULL=1 ;;
    --mock) MOCK=1 ;;
    *)
      printf 'Unknown argument: %s\n' "$arg"
      exit 2
      ;;
  esac
done

tmpdir="$(mktemp -d)"
mock_pid=""
restore_backend=0

cleanup() {
  if [[ -n "$mock_pid" ]]; then
    kill "$mock_pid" >/dev/null 2>&1 || true
    wait "$mock_pid" 2>/dev/null || true
  fi
  if [[ "$restore_backend" == "1" ]]; then
    log "Restoring backend service configuration"
    docker compose up -d --force-recreate backend >/dev/null
  fi
  rm -rf "$tmpdir"
}
trap cleanup EXIT

pass_count=0
fail_count=0
xfail_count=0

log() {
  printf '%s\n' "$*"
}

pass() {
  pass_count=$((pass_count + 1))
  log "PASS $1"
}

fail() {
  fail_count=$((fail_count + 1))
  log "FAIL $1"
  if [[ $# -gt 1 ]]; then
    log "  $2"
  fi
}

xfail() {
  xfail_count=$((xfail_count + 1))
  log "XFAIL $1"
  if [[ $# -gt 1 ]]; then
    log "  $2"
  fi
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    log "Missing required command: $1"
    exit 2
  fi
}

json_get() {
  node -e "const fs=require('fs'); const o=JSON.parse(fs.readFileSync(0,'utf8')); const path=process.argv[1].split('.'); let v=o; for (const key of path) v=v?.[key]; if (v === undefined || v === null) process.exit(1); process.stdout.write(String(v));" "$1"
}

http_status() {
  curl -sS -o "$tmpdir/body" -w '%{http_code}' "$@"
}

check_status() {
  local name="$1"
  local expected="$2"
  shift 2
  local status
  status="$(http_status "$@")"
  if [[ "$status" == "$expected" ]]; then
    pass "$name"
  else
    fail "$name" "expected HTTP $expected, got $status; body=$(cat "$tmpdir/body")"
  fi
}

post_json() {
  curl -sS -X POST "$1" -H 'Content-Type: application/json' -d "$2"
}

authorized_json() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  if [[ -n "$body" ]]; then
    curl -sS -X "$method" "$BASE_URL$path" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d "$body"
  else
    curl -sS -X "$method" "$BASE_URL$path" -H "Authorization: Bearer $TOKEN"
  fi
}

require_cmd curl
require_cmd node
require_cmd docker

if [[ "$MOCK" == "1" ]]; then
  log "Starting local Kimi mock"
  node scripts/mock-kimi.mjs >"$tmpdir/mock-kimi.log" 2>&1 &
  mock_pid="$!"
  for _ in {1..30}; do
    if curl -sS http://127.0.0.1:18081/v1/models >/dev/null 2>&1; then
      break
    fi
    sleep 0.2
  done

  cat >"$tmpdir/docker-compose.mock.yml" <<YAML
services:
  backend:
    extra_hosts:
      - "host.docker.internal:host-gateway"
    environment:
      KIMI_API_KEY: mock-key
      KIMI_BASE_URL: http://host.docker.internal:18081/v1
      KIMI_MODEL: moonshot-v1-8k
      ANALYZE_TIMEOUT: 15s
YAML
  docker compose -f docker-compose.yml -f "$tmpdir/docker-compose.mock.yml" up -d --force-recreate backend >/dev/null
  restore_backend=1
  for _ in {1..60}; do
    if curl -sS "$BASE_URL/health" >/dev/null 2>&1; then
      break
    fi
    sleep 0.5
  done
fi

log "Preparing smoke data for $TEST_USER"
docker compose exec -T postgres sh -lc "psql -v ON_ERROR_STOP=1 -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\" <<SQL
DELETE FROM analyses WHERE user_id IN (SELECT id FROM users WHERE username = '$TEST_USER');
DELETE FROM invite_codes WHERE used_by IN (SELECT id FROM users WHERE username = '$TEST_USER') OR code = '$INVITE_CODE';
DELETE FROM users WHERE username = '$TEST_USER';
INSERT INTO invite_codes (code) VALUES ('$INVITE_CODE');
SQL" >/dev/null

if [[ "$(curl -sS "$BASE_URL/health")" == '{"status":"ok"}' ]]; then
  pass "health"
else
  fail "health" "unexpected response from $BASE_URL/health"
fi

headers="$(curl -sS -D - -o /dev/null "$BASE_URL/health")"
if grep -qi 'X-Content-Type-Options: nosniff' <<<"$headers" &&
  grep -qi 'X-Frame-Options: DENY' <<<"$headers" &&
  grep -qi 'Referrer-Policy: same-origin' <<<"$headers"; then
  pass "security headers"
else
  fail "security headers" "missing one or more baseline security headers"
fi

check_status "history requires auth" 401 "$BASE_URL/api/history"

bad_register="$(post_json "$BASE_URL/api/auth/register" '{"username":"","password":"short","inviteCode":"bad"}')"
if grep -q '用户名不能为空' <<<"$bad_register"; then
  pass "register validates username/password"
else
  fail "register validates username/password" "$bad_register"
fi

bad_invite="$(post_json "$BASE_URL/api/auth/register" "{\"username\":\"$TEST_USER\",\"password\":\"$TEST_PASS\",\"inviteCode\":\"bad\"}")"
if grep -q '邀请码无效' <<<"$bad_invite"; then
  pass "register rejects bad invite"
else
  fail "register rejects bad invite" "$bad_invite"
fi

register_response="$(post_json "$BASE_URL/api/auth/register" "{\"username\":\"$TEST_USER\",\"password\":\"$TEST_PASS\",\"inviteCode\":\"$INVITE_CODE\"}")"
TOKEN="$(json_get token <<<"$register_response" || true)"
if [[ -n "${TOKEN:-}" ]]; then
  pass "register succeeds"
else
  fail "register succeeds" "$register_response"
  exit 1
fi

wrong_status="$(http_status -X POST "$BASE_URL/api/auth/login" -H 'Content-Type: application/json' -d "{\"username\":\"$TEST_USER\",\"password\":\"WrongPass123!\"}")"
if [[ "$wrong_status" == "401" ]]; then
  pass "login rejects wrong password"
else
  fail "login rejects wrong password" "expected HTTP 401, got $wrong_status"
fi

login_response="$(post_json "$BASE_URL/api/auth/login" "{\"username\":\"$TEST_USER\",\"password\":\"$TEST_PASS\"}")"
TOKEN="$(json_get token <<<"$login_response" || true)"
if [[ -n "${TOKEN:-}" ]]; then
  pass "login succeeds"
else
  fail "login succeeds" "$login_response"
  exit 1
fi

check_status "text analysis requires auth" 401 -X POST "$BASE_URL/api/analyze/text" -H 'Content-Type: application/json' -d '{"text":"我：你好\n对方：你好"}'

empty_status="$(http_status -X POST "$BASE_URL/api/analyze/text" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"text":"   "}')"
if [[ "$empty_status" == "400" ]]; then
  pass "text analysis rejects empty text"
else
  fail "text analysis rejects empty text" "expected HTTP 400, got $empty_status"
fi

sample_text='我：今天忙完了吗？
对方：差不多，刚缓过来一点。
我：那你这两天像是在连轴转。
对方：是啊，感觉脑子都快烧了，周末只想找个安静点的地方坐坐。
我：那先别安排太满，找个轻松的地方喝点东西？
对方：这个倒是可以。'

analysis_payload="$(SAMPLE_TEXT="$sample_text" node -e 'process.stdout.write(JSON.stringify({text: process.env.SAMPLE_TEXT}))')"
analysis="$(authorized_json POST /api/analyze/text "$analysis_payload")"
analysis_id="$(json_get id <<<"$analysis" || true)"
saved_flag="$(json_get saved <<<"$analysis" || true)"
ivi_score="$(json_get result.metrics.ivi.score <<<"$analysis" || true)"
if [[ -n "$analysis_id" && "$saved_flag" == "false" && -n "$ivi_score" ]]; then
  pass "text analysis succeeds"
else
  fail "text analysis succeeds" "$analysis"
fi

saved="$(curl -sS -X POST "$BASE_URL/api/analyses/save" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' --data-binary @<(printf '%s' "$analysis"))"
saved_id="$(json_get id <<<"$saved" || true)"
saved_state="$(json_get saved <<<"$saved" || true)"
if [[ -n "$saved_id" && "$saved_state" == "true" ]]; then
  pass "save analysis succeeds"
else
  fail "save analysis succeeds" "$saved"
fi

history="$(authorized_json GET /api/history)"
history_count="$(node -e 'const fs=require("fs"); const o=JSON.parse(fs.readFileSync(0,"utf8")); process.stdout.write(String(o.items?.length ?? 0));' <<<"$history")"
if [[ "$history_count" -ge 1 ]]; then
  pass "history lists saved analysis"
else
  fail "history lists saved analysis" "$history"
fi

detail="$(authorized_json GET "/api/history/$saved_id")"
detail_id="$(json_get id <<<"$detail" || true)"
if [[ -n "$saved_id" && "$detail_id" == "$saved_id" ]]; then
  pass "history detail loads"
else
  fail "history detail loads" "$detail"
fi

delete_response="$(authorized_json DELETE "/api/history/$saved_id")"
if [[ "$delete_response" == '{"ok":true}' ]]; then
  pass "delete history succeeds"
else
  fail "delete history succeeds" "$delete_response"
fi

metrics_body="$(curl -sS "$BASE_URL/metrics")"
if grep -q '^# HELP\|senti_' <<<"$metrics_body"; then
  pass "public metrics endpoint"
else
  xfail "public metrics endpoint" "known BUG-001: $BASE_URL/metrics returns frontend HTML through nginx"
fi

if [[ "$FULL" == "1" ]]; then
  image_path="${IMAGE_PATH:-chat_screenshot_example.png}"
  if [[ -f "$image_path" ]]; then
    image_analysis="$(curl -sS -X POST "$BASE_URL/api/analyze/image" -H "Authorization: Bearer $TOKEN" -F "image=@$image_path")"
    image_id="$(json_get id <<<"$image_analysis" || true)"
    image_text="$(json_get sourceText <<<"$image_analysis" || true)"
    if [[ -n "$image_id" ]]; then
      pass "image analysis succeeds"
      if grep -q 'Estimating resolution\|WA:' <<<"$image_text"; then
        xfail "image OCR cleanup" "known BUG-002: OCR source text still contains noise/prefix artifacts"
      else
        pass "image OCR cleanup"
      fi
    else
      fail "image analysis succeeds" "$image_analysis"
    fi
  else
    fail "image analysis fixture exists" "missing $image_path"
  fi
else
  log "SKIP image analysis slow path; run with --full to include it"
fi

docker compose exec -T postgres sh -lc "psql -v ON_ERROR_STOP=1 -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\" <<SQL
DELETE FROM analyses WHERE user_id IN (SELECT id FROM users WHERE username = '$TEST_USER');
DELETE FROM invite_codes WHERE used_by IN (SELECT id FROM users WHERE username = '$TEST_USER') OR code = '$INVITE_CODE';
DELETE FROM users WHERE username = '$TEST_USER';
SQL" >/dev/null

log "Summary: pass=$pass_count fail=$fail_count xfail=$xfail_count"
if [[ "$fail_count" -gt 0 ]]; then
  exit 1
fi
