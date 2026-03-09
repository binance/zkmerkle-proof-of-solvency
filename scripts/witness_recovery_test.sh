#!/bin/bash
set -euo pipefail

##############################################################################
# Witness Recovery Test
#
# Tests the witness resume/recovery logic by:
#   1. Starting witness and killing it mid-way (SIGKILL)
#   2. Restarting witness and letting it complete
#   3. Validating final table row counts match expected values
#
# Only requires MySQL — no Redis, keygen, or prover.
#
# Prerequisites: docker, go 1.22+
# Usage: ./scripts/witness_recovery_test.sh
##############################################################################

# --- Configuration ---
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK_DIR="${PROJECT_ROOT}/_recovery_test"
# 500 asset tier accounts: 96
# 50 asset tier accounts: 9903
TEST_TIERS="500:18,50:111"
EXPECT_TOTAL_USERS_ACCOUNT=10098
EXPECT_TOTAL_WITNESS_BATCHES=96
MYSQL_PORT=13307
MYSQL_CONTAINER="zkpos-recovery-mysql"
MYSQL_USER="zkpos"
MYSQL_PASSWORD="zkpos@123"
MYSQL_DATABASE="zkpos"
MYSQL_ROOT_PASSWORD="zkpos@123"
DB_SUFFIX="0"

# Kill when userproof rows reach this threshold (roughly 30% of 10100).
KILL_AFTER_USERPROOF_ROWS=3000

# --- Helper functions ---
log() { echo "=== [$(date '+%H:%M:%S')] $*"; }
log_step() { echo ""; echo "###############################################"; log "$*"; echo "###############################################"; }

mysql_query() {
    docker exec "$MYSQL_CONTAINER" mysql -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" \
        "$MYSQL_DATABASE" -N -B -e "$1" 2>/dev/null
}

cleanup() {
    log "Cleaning up previous run..."
    docker rm -f "$MYSQL_CONTAINER" 2>/dev/null || true
    rm -rf "${WORK_DIR}/config"
    log "Cleanup done"
}

wait_for_mysql() {
    log "Waiting for MySQL to be ready..."
    local max_wait=60
    local waited=0
    while ! docker exec "$MYSQL_CONTAINER" mysqladmin ping -h 127.0.0.1 \
          -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" --silent 2>/dev/null; do
        sleep 2
        waited=$((waited + 2))
        if [ "$waited" -ge "$max_wait" ]; then
            echo "ERROR: MySQL not ready after ${max_wait}s"
            exit 1
        fi
    done
    log "MySQL is ready (waited ${waited}s)"
}

write_witness_config() {
    mkdir -p "${WORK_DIR}/config"
    cat > "${WORK_DIR}/config/config.json" <<EOF
{
    "MysqlDataSource": "${MYSQL_DSN}",
    "DbSuffix": "${DB_SUFFIX}",
    "UserDataFile": "${PROJECT_ROOT}/src/userdata"
}
EOF
}

# --- Step 0: Clean up previous run ---
log_step "Step 0: Cleaning up previous run"
cleanup

# --- Step 1: Start MySQL ---
log_step "Step 1: Starting MySQL"

docker run -d --name "$MYSQL_CONTAINER" \
    -p "${MYSQL_PORT}:3306" \
    -e MYSQL_USER="$MYSQL_USER" \
    -e MYSQL_PASSWORD="$MYSQL_PASSWORD" \
    -e MYSQL_DATABASE="$MYSQL_DATABASE" \
    -e MYSQL_ROOT_PASSWORD="$MYSQL_ROOT_PASSWORD" \
    mysql

wait_for_mysql

MYSQL_DSN="${MYSQL_USER}:${MYSQL_PASSWORD}@tcp(127.0.0.1:${MYSQL_PORT})/${MYSQL_DATABASE}?parseTime=true&readTimeout=60s"

# --- Step 2: Build witness binary ---
log_step "Step 2: Building witness binary"

mkdir -p "$WORK_DIR"
cd "$PROJECT_ROOT"
go build -o "${WORK_DIR}/witness" ./src/witness
log "Witness binary built"

# --- Step 3: Run witness (first run — will be killed mid-way) ---
log_step "Step 3: Running witness (first run — will be killed)"

write_witness_config

cd "$WORK_DIR"
ZKPOR_TEST_TIERS="$TEST_TIERS" ./witness &
WITNESS_PID=$!
cd "$PROJECT_ROOT"

log "Witness started with PID ${WITNESS_PID}"
log "Waiting for ${KILL_AFTER_USERPROOF_ROWS} userproof rows before killing..."

# Poll DB until enough userproof rows exist, then kill.
# We use userproof rows (not witness rows) because userproof is faster
# and we need to kill before it completes to test its recovery logic.
MAX_WAIT=300
WAITED=0
while true; do
    UP_ROWS=$(mysql_query "SELECT COUNT(*) FROM userproof${DB_SUFFIX}" 2>/dev/null || echo "0")
    if [ "$UP_ROWS" -ge "$KILL_AFTER_USERPROOF_ROWS" ] 2>/dev/null; then
        break
    fi
    # Also check if witness process has already exited (both services finished).
    if ! kill -0 "$WITNESS_PID" 2>/dev/null; then
        echo "ERROR: witness process exited before userproof reached ${KILL_AFTER_USERPROOF_ROWS} rows (got ${UP_ROWS})"
        exit 1
    fi
    sleep 0.2
    WAITED=$((WAITED + 1))
    if [ "$WAITED" -ge "$((MAX_WAIT * 5))" ]; then  # 0.2s intervals
        echo "ERROR: userproof did not reach ${KILL_AFTER_USERPROOF_ROWS} rows within ${MAX_WAIT}s (got ${UP_ROWS})"
        kill -9 "$WITNESS_PID" 2>/dev/null || true
        wait "$WITNESS_PID" 2>/dev/null || true
        exit 1
    fi
done

log "Userproof has ${UP_ROWS} rows, killing with SIGKILL..."
kill -9 "$WITNESS_PID" 2>/dev/null || true
wait "$WITNESS_PID" 2>/dev/null || true
log "Witness killed"

# Record partial progress.
WITNESS_ROWS_BEFORE=$(mysql_query "SELECT COUNT(*) FROM witness${DB_SUFFIX}")
USERPROOF_ROWS_BEFORE=$(mysql_query "SELECT COUNT(*) FROM userproof${DB_SUFFIX}" 2>/dev/null || echo "0")
log "Partial progress — witness rows: ${WITNESS_ROWS_BEFORE}, userproof rows: ${USERPROOF_ROWS_BEFORE}"

if [ "$USERPROOF_ROWS_BEFORE" -ge "$EXPECT_TOTAL_USERS_ACCOUNT" ]; then
    echo "ERROR: userproof completed before kill (got ${USERPROOF_ROWS_BEFORE} rows). Reduce KILL_AFTER_USERPROOF_ROWS."
    exit 1
fi
if [ "$USERPROOF_ROWS_BEFORE" -le 0 ]; then
    echo "ERROR: no userproof rows written before crash"
    exit 1
fi
log "Confirmed partial completion: witness ${WITNESS_ROWS_BEFORE}/${EXPECT_TOTAL_WITNESS_BATCHES}, userproof ${USERPROOF_ROWS_BEFORE}/${EXPECT_TOTAL_USERS_ACCOUNT}"

# --- Step 4: Restart witness (recovery run) ---
log_step "Step 4: Restarting witness (recovery run)"

cd "$WORK_DIR"
ZKPOR_TEST_TIERS="$TEST_TIERS" ./witness
cd "$PROJECT_ROOT"
log "Witness recovery complete"

# --- Step 5: Validate final table row counts ---
log_step "Step 5: Validating table row counts"

WITNESS_ROWS_AFTER=$(mysql_query "SELECT COUNT(*) FROM witness${DB_SUFFIX}")
USERPROOF_ROWS_AFTER=$(mysql_query "SELECT COUNT(*) FROM userproof${DB_SUFFIX}")
log "After recovery — witness rows: ${WITNESS_ROWS_AFTER}, userproof rows: ${USERPROOF_ROWS_AFTER}"

if [ "$WITNESS_ROWS_AFTER" -ne "$EXPECT_TOTAL_WITNESS_BATCHES" ]; then
    echo "ERROR: witness rows after recovery ($WITNESS_ROWS_AFTER) != expected ($EXPECT_TOTAL_WITNESS_BATCHES)"
    exit 1
fi
log "CHECK PASSED: witness rows == ${EXPECT_TOTAL_WITNESS_BATCHES}"

if [ "$USERPROOF_ROWS_AFTER" -ne "$EXPECT_TOTAL_USERS_ACCOUNT" ]; then
    echo "ERROR: userproof rows after recovery ($USERPROOF_ROWS_AFTER) != expected ($EXPECT_TOTAL_USERS_ACCOUNT)"
    exit 1
fi
log "CHECK PASSED: userproof rows == ${EXPECT_TOTAL_USERS_ACCOUNT}"

# --- Done ---
log_step "Witness recovery test PASSED (crashed at ${WITNESS_ROWS_BEFORE}/${EXPECT_TOTAL_WITNESS_BATCHES} batches, recovered successfully)"
