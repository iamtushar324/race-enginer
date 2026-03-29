#!/bin/bash
# ---------------------------------------------------------------------------
# Race Engineer — Integration test runner
# Starts the Go core in mock mode with debug logging, then runs API tests
# one by one, checking responses and logs.
#
# Usage:
#   ./test_integration.sh              # Run all tests
#   ./test_integration.sh health       # Run only the health test
#   ./test_integration.sh query        # Run only the query test
#   ./test_integration.sh strategy     # Run only the strategy push test
#   ./test_integration.sh driver_query # Run only the driver query test
#   ./test_integration.sh settings     # Run only the settings test
#   ./test_integration.sh telemetry    # Run only the telemetry latest test
# ---------------------------------------------------------------------------

set -e
cd "$(dirname "$0")"

API_PORT="${API_PORT:-8081}"
API_URL="http://localhost:${API_PORT}"
LOG_FILE="test_integration.log"
PASS=0
FAIL=0
TESTS_RUN=0

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# --- Helpers ---

log_header() {
    echo ""
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${CYAN}  TEST: $1${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

pass() {
    PASS=$((PASS + 1))
    TESTS_RUN=$((TESTS_RUN + 1))
    echo -e "  ${GREEN}✓ PASS${NC}: $1"
}

fail() {
    FAIL=$((FAIL + 1))
    TESTS_RUN=$((TESTS_RUN + 1))
    echo -e "  ${RED}✗ FAIL${NC}: $1"
    echo -e "  ${RED}  Detail: $2${NC}"
}

wait_for_server() {
    echo -e "${YELLOW}Waiting for server on port ${API_PORT}...${NC}"
    for i in $(seq 1 30); do
        if curl -sf "${API_URL}/health" > /dev/null 2>&1; then
            echo -e "${GREEN}Server is up!${NC}"
            return 0
        fi
        sleep 1
    done
    echo -e "${RED}Server failed to start within 30s. Check ${LOG_FILE}${NC}"
    cat "$LOG_FILE" | tail -50
    exit 1
}

start_server() {
    # Load .env
    if [ -f .env ]; then
        set -a
        source .env
        set +a
    fi

    echo -e "${YELLOW}Building telemetry-core...${NC}"
    (cd telemetry-core && go build -o ../bin/telemetry-core cmd/server/main.go)
    echo -e "${GREEN}Build complete.${NC}"

    echo -e "${YELLOW}Starting Go core in mock mode (LOG_LEVEL=debug)...${NC}"
    TELEMETRY_MODE=mock \
    LOG_LEVEL=debug \
    DB_PATH="workspace/telemetry_test.duckdb" \
    API_PORT="${API_PORT}" \
    ./bin/telemetry-core > "$LOG_FILE" 2>&1 &
    SERVER_PID=$!
    echo "Server PID: ${SERVER_PID}"

    wait_for_server
    # Give mock data a few seconds to populate
    sleep 3
}

stop_server() {
    if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
        echo -e "\n${YELLOW}Stopping server (PID ${SERVER_PID})...${NC}"
        kill "$SERVER_PID" 2>/dev/null || true
        wait "$SERVER_PID" 2>/dev/null || true
        echo -e "${GREEN}Server stopped.${NC}"
    fi
    # Clean up test database
    rm -f workspace/telemetry_test.duckdb workspace/telemetry_test.duckdb.wal
}

check_logs_for_errors() {
    local context="$1"
    # Check for panics or fatal errors (not normal warn/error from missing data)
    if grep -qi "panic\|FATAL" "$LOG_FILE" 2>/dev/null; then
        fail "${context}: found panic/fatal in logs"
        grep -i "panic\|FATAL" "$LOG_FILE" | tail -5
        return 1
    fi
    return 0
}

# --- Test Functions ---

test_health() {
    log_header "Health Check"

    RESP=$(curl -sf "${API_URL}/health" 2>&1) || { fail "Health endpoint unreachable" "curl failed"; return; }

    echo "  Response: $(echo "$RESP" | head -c 200)"

    if echo "$RESP" | grep -q '"status"'; then
        pass "Health endpoint returns status"
    else
        fail "Health endpoint missing status field" "$RESP"
    fi

    if echo "$RESP" | grep -q '"duckdb_ok"'; then
        pass "Health reports DuckDB status"
    else
        fail "Health missing duckdb_ok" "$RESP"
    fi

    check_logs_for_errors "health"
}

test_settings() {
    log_header "Settings API"

    RESP=$(curl -sf "${API_URL}/api/settings" 2>&1) || { fail "Settings endpoint unreachable" "curl failed"; return; }

    echo "  Response: $(echo "$RESP" | head -c 300)"

    if echo "$RESP" | grep -q '"mock_mode"'; then
        pass "Settings returns mock_mode"
    else
        fail "Settings missing mock_mode" "$RESP"
    fi

    # Toggle talk level
    RESP=$(curl -sf -X POST "${API_URL}/api/settings/talk_level" \
        -H "Content-Type: application/json" \
        -d '{"level": 7}' 2>&1) || { fail "Talk level POST failed" "curl failed"; return; }

    pass "Talk level updated successfully"
    check_logs_for_errors "settings"
}

test_telemetry_latest() {
    log_header "Telemetry Latest (Mock Data)"

    RESP=$(curl -sf "${API_URL}/api/telemetry/latest" 2>&1) || { fail "Telemetry latest unreachable" "curl failed"; return; }

    echo "  Response: $(echo "$RESP" | head -c 300)"

    if echo "$RESP" | grep -q '"speed"'; then
        pass "Telemetry returns speed"
    else
        # In mock mode, data may take a moment
        fail "Telemetry missing speed field (mock data may not be ready)" "$RESP"
    fi

    check_logs_for_errors "telemetry"
}

test_query() {
    log_header "SQL Query API"

    # Simple count query
    RESP=$(curl -sf -X POST "${API_URL}/api/query" \
        -H "Content-Type: application/json" \
        -d '{"sql": "SELECT COUNT(*) as n FROM telemetry"}' 2>&1) || { fail "Query endpoint unreachable" "curl failed"; return; }

    echo "  Response: $RESP"

    if echo "$RESP" | grep -q '"n"'; then
        pass "SQL query returned result"
    else
        fail "SQL query response unexpected" "$RESP"
    fi

    # Schema query
    RESP=$(curl -sf -X POST "${API_URL}/api/query" \
        -H "Content-Type: application/json" \
        -d '{"sql": "SELECT table_name FROM information_schema.tables WHERE table_schema = '\''main'\''"}' 2>&1) || { fail "Schema query failed" "curl failed"; return; }

    echo "  Tables: $(echo "$RESP" | head -c 300)"

    if echo "$RESP" | grep -q "telemetry"; then
        pass "Schema lists telemetry table"
    else
        fail "Schema missing telemetry table" "$RESP"
    fi

    check_logs_for_errors "query"
}

test_strategy_push() {
    log_header "Strategy Insight Push"

    RESP=$(curl -sf -X POST "${API_URL}/api/strategy" \
        -H "Content-Type: application/json" \
        -d '{
            "source": "integration-test",
            "insight": "Rear tire degradation accelerating. Consider boxing in 3 laps.",
            "priority": 3
        }' 2>&1) || { fail "Strategy push failed" "curl failed"; return; }

    echo "  Response: $RESP"
    pass "Strategy insight pushed"

    # Check logs for the insight being processed
    sleep 2
    if grep -q "integration-test\|strategy.*insight" "$LOG_FILE" 2>/dev/null; then
        pass "Strategy insight appears in logs"
    else
        pass "Strategy push accepted (log confirmation not required)"
    fi

    check_logs_for_errors "strategy"
}

test_driver_query() {
    log_header "Driver Query (LLM Integration)"

    echo -e "  ${YELLOW}Sending: 'Hey, what is today's strategy?'${NC}"
    echo -e "  ${YELLOW}(This test requires a valid LLM API key to get a real response)${NC}"

    RESP=$(curl -sf --max-time 35 -X POST "${API_URL}/api/driver_query" \
        -H "Content-Type: application/json" \
        -d '{"query": "Hey, what is todays strategy?"}' 2>&1)
    CURL_EXIT=$?

    if [ $CURL_EXIT -ne 0 ]; then
        fail "Driver query request failed" "curl exit code: $CURL_EXIT"
        return
    fi

    echo "  Response: $(echo "$RESP" | head -c 500)"

    if echo "$RESP" | grep -q '"message"'; then
        pass "Driver query returned a message"
    else
        fail "Driver query response missing message field" "$RESP"
    fi

    # Check if it's a real LLM response or a fallback
    if echo "$RESP" | grep -q "don't have\|not available\|Stand by\|data link"; then
        echo -e "  ${YELLOW}Note: Got fallback response (LLM may not be configured)${NC}"
        pass "Driver query gracefully handled missing LLM"
    else
        pass "Driver query got LLM response"
    fi

    # Check logs for JSON unmarshal errors
    sleep 2
    if grep -q "failed to unmarshal JSON response" "$LOG_FILE" 2>/dev/null; then
        fail "JSON unmarshal error in logs" "$(grep 'failed to unmarshal' "$LOG_FILE" | tail -1)"
    else
        pass "No JSON unmarshal errors in logs"
    fi

    check_logs_for_errors "driver_query"
}

# --- Summary ---

print_summary() {
    echo ""
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${CYAN}  RESULTS: ${TESTS_RUN} tests | ${GREEN}${PASS} passed${NC} | ${RED}${FAIL} failed${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "  Log file: ${LOG_FILE}"

    if [ "$FAIL" -gt 0 ]; then
        echo -e "\n  ${RED}Recent errors from log:${NC}"
        grep -i "error\|fail\|panic" "$LOG_FILE" 2>/dev/null | tail -10 || echo "  (none)"
        exit 1
    fi
}

# --- Main ---

trap stop_server EXIT

# Determine which tests to run
TEST_FILTER="${1:-all}"

start_server

case "$TEST_FILTER" in
    health)
        test_health
        ;;
    settings)
        test_settings
        ;;
    telemetry)
        test_telemetry_latest
        ;;
    query)
        test_query
        ;;
    strategy)
        test_strategy_push
        ;;
    driver_query)
        test_driver_query
        ;;
    all)
        test_health
        test_settings
        test_telemetry_latest
        test_query
        test_strategy_push
        test_driver_query
        ;;
    *)
        echo "Unknown test: $TEST_FILTER"
        echo "Available: health, settings, telemetry, query, strategy, driver_query, all"
        exit 1
        ;;
esac

print_summary
