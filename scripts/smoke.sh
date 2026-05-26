#!/usr/bin/env sh
set -eu

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
SERVER_BIN="${SERVER_BIN:-./bin/server}"
CONFIG_PATH="${CONFIG_PATH:-config/config.yaml.example}"
USE_EXISTING_SERVER="${USE_EXISTING_SERVER:-0}"

cleanup() {
	if [ -n "${SERVER_PID:-}" ]; then
		kill "$SERVER_PID" >/dev/null 2>&1 || true
		wait "$SERVER_PID" >/dev/null 2>&1 || true
	fi
}
trap cleanup EXIT INT TERM

if [ "$USE_EXISTING_SERVER" != "1" ]; then
	POSTGRES_DSN="${INTERVIEW_POSTGRES_DSN:-}"
	INTERVIEW_LLM_MODE=mock \
	INTERVIEW_EMBEDDING_MODE=mock \
	INTERVIEW_POSTGRES_DSN="$POSTGRES_DSN" \
		"$SERVER_BIN" -config "$CONFIG_PATH" >/tmp/interview-agent-smoke.log 2>&1 &
	SERVER_PID=$!
fi

tries=0
until curl -fsS "$BASE_URL/healthz" >/dev/null; do
	tries=$((tries + 1))
	if [ "$tries" -ge 30 ]; then
		echo "server did not become healthy"
		if [ "$USE_EXISTING_SERVER" != "1" ]; then
			cat /tmp/interview-agent-smoke.log
		fi
		exit 1
	fi
	sleep 1
done

curl -fsS "$BASE_URL/readyz" >/dev/null
curl -fsS "$BASE_URL/api/ping" >/dev/null

START_BODY='{"session_id":"smoke-1","user_id":"smoke","jd_text":"需要 Go 后端工程师，熟悉 Redis 和并发编程","resume_text":"两年 Go 后端经验，做过 Redis 缓存服务"}'
START_RESP="$(curl -fsS "$BASE_URL/api/interview/start" \
	-H "Content-Type: application/json" \
	-d "$START_BODY")"
case "$START_RESP" in
	*"\"question\""*) ;;
	*)
		echo "interview/start did not return a question"
		echo "$START_RESP"
		exit 1
		;;
esac

GET_RESP="$(curl -fsS "$BASE_URL/api/interview/sessions/smoke-1")"
case "$GET_RESP" in
	*"\"session_id\":\"smoke-1\""*|*"\"session_id\": \"smoke-1\""*) ;;
	*)
		echo "interview/sessions/:id did not return smoke session"
		echo "$GET_RESP"
		exit 1
		;;
esac

ANSWER_BODY='{"session_id":"smoke-1","user_id":"smoke","answer":"G 是 goroutine，M 是线程，P 负责本地队列和调度。"}'
ANSWER_RESP="$(curl -fsS "$BASE_URL/api/interview/answer" \
	-H "Content-Type: application/json" \
	-d "$ANSWER_BODY")"
case "$ANSWER_RESP" in
	*"\"report\""*) ;;
	*)
		echo "interview/answer did not return a report"
		echo "$ANSWER_RESP"
		exit 1
		;;
esac

LIST_RESP="$(curl -fsS "$BASE_URL/api/interview/sessions?user_id=smoke&limit=10")"
case "$LIST_RESP" in
	*"\"session_id\":\"smoke-1\""*|*"\"session_id\": \"smoke-1\""*) ;;
	*)
		echo "interview/sessions list did not include smoke session"
		echo "$LIST_RESP"
		exit 1
		;;
esac

echo "smoke ok: healthz, readyz, api/ping, interview start/get/answer/list"
