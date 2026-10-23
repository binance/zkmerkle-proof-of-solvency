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
# Runs multiple tier configurations to cover different batch sizes.
#
# Only requires MySQL — no Redis, keygen, or prover.
#
# Prerequisites: docker, go 1.22+
# Usage: ./scripts/witness_recovery_test.sh
##############################################################################

# --- Configuration ---
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK_DIR="${PROJECT_ROOT}/_recovery_test"

# Each entry: "TEST_TIERS|EXPECT_TOTAL_USERS_ACCOUNT|EXPECT_TOTAL_WITNESS_BATCHES|KILL_MODE|KILL_THRESHOLD"
# KILL_MODE: "userproof" = kill when userproof rows >= threshold
#            "witness"   = kill when witness rows >= threshold
TIER_CONFIGS=(
    "500:200,50:1380|11240|9|userproof|3000"
    "500:18,50:111|10098|96|witness|30"
)

MYSQL_PORT=13307
MYSQL_CONTAINER="zkpos-recovery-mysql"
MYSQL_USER="zkpos"
MYSQL_PASSWORD="zkpos@123"
MYSQL_DATABASE="zkpos"
MYSQL_ROOT_PASSWORD="zkpos@123"
DB_SUFFIX="0"

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

reset_tables() {
    log "Dropping tables for fresh tier run..."
    mysql_query "DROP TABLE IF EXISTS witness${DB_SUFFIX}, userproof${DB_SUFFIX};" || true
    log "Tables dropped"
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

# --- run_recovery_test: runs crash-and-recover for one tier config ---
run_recovery_test() {
    local test_tiers="$1"
    local expect_users="$2"
    local expect_batches="$3"
    local kill_mode="$4"
    local kill_threshold="$5"
    local tier_index="$6"

    log_step "Tier ${tier_index}: TEST_TIERS=${test_tiers} (expect users=${expect_users}, batches=${expect_batches}, kill ${kill_mode}@${kill_threshold})"

    # Reset DB tables so each tier starts fresh.
    reset_tables
    write_witness_config

    # --- First run: start and kill mid-way ---
    log "Tier ${tier_index}: Running witness (first run — will be killed)"

    cd "$WORK_DIR"
    ZKPOR_TEST_TIERS="$test_tiers" ./witness &
    WITNESS_PID=$!
    cd "$PROJECT_ROOT"

    local kill_table
    if [ "$kill_mode" = "witness" ]; then
        kill_table="witness${DB_SUFFIX}"
    else
        kill_table="userproof${DB_SUFFIX}"
    fi

    log "Witness started with PID ${WITNESS_PID}"
    log "Waiting for ${kill_threshold} ${kill_table} rows before killing..."

    MAX_WAIT=300
    WAITED=0
    while true; do
        POLL_ROWS=$(mysql_query "SELECT COUNT(*) FROM ${kill_table}" 2>/dev/null || echo "0")
        if [ "$POLL_ROWS" -ge "$kill_threshold" ] 2>/dev/null; then
            break
        fi
        if ! kill -0 "$WITNESS_PID" 2>/dev/null; then
            echo "ERROR: witness process exited before ${kill_table} reached ${kill_threshold} rows (got ${POLL_ROWS})"
            exit 1
        fi
        sleep 0.2
        WAITED=$((WAITED + 1))
        if [ "$WAITED" -ge "$((MAX_WAIT * 5))" ]; then  # 0.2s intervals
            echo "ERROR: ${kill_table} did not reach ${kill_threshold} rows within ${MAX_WAIT}s (got ${POLL_ROWS})"
            kill -9 "$WITNESS_PID" 2>/dev/null || true
            wait "$WITNESS_PID" 2>/dev/null || true
            exit 1
        fi
    done

    log "${kill_table} has ${POLL_ROWS} rows, killing with SIGKILL..."
    kill -9 "$WITNESS_PID" 2>/dev/null || true
    wait "$WITNESS_PID" 2>/dev/null || true
    log "Witness killed"

    # Record partial progress.
    WITNESS_ROWS_BEFORE=$(mysql_query "SELECT COUNT(*) FROM witness${DB_SUFFIX}" 2>/dev/null || echo "0")
    USERPROOF_ROWS_BEFORE=$(mysql_query "SELECT COUNT(*) FROM userproof${DB_SUFFIX}" 2>/dev/null || echo "0")
    log "Partial progress — witness rows: ${WITNESS_ROWS_BEFORE}, userproof rows: ${USERPROOF_ROWS_BEFORE}"

    if [ "$WITNESS_ROWS_BEFORE" -ge "$expect_batches" ] && [ "$USERPROOF_ROWS_BEFORE" -ge "$expect_users" ]; then
        echo "ERROR: both tables completed before kill. Reduce kill threshold."
        exit 1
    fi
    log "Confirmed partial completion: witness ${WITNESS_ROWS_BEFORE}/${expect_batches}, userproof ${USERPROOF_ROWS_BEFORE}/${expect_users}"

    # --- Recovery run ---
    log "Tier ${tier_index}: Restarting witness (recovery run)"

    cd "$WORK_DIR"
    ZKPOR_TEST_TIERS="$test_tiers" ./witness
    cd "$PROJECT_ROOT"
    log "Witness recovery complete"

    # --- Validate ---
    log "Tier ${tier_index}: Validating table row counts"

    WITNESS_ROWS_AFTER=$(mysql_query "SELECT COUNT(*) FROM witness${DB_SUFFIX}")
    USERPROOF_ROWS_AFTER=$(mysql_query "SELECT COUNT(*) FROM userproof${DB_SUFFIX}")
    log "After recovery — witness rows: ${WITNESS_ROWS_AFTER}, userproof rows: ${USERPROOF_ROWS_AFTER}"

    if [ "$WITNESS_ROWS_AFTER" -ne "$expect_batches" ]; then
        echo "ERROR: witness rows after recovery ($WITNESS_ROWS_AFTER) != expected ($expect_batches)"
        exit 1
    fi
    log "CHECK PASSED: witness rows == ${expect_batches}"

    if [ "$USERPROOF_ROWS_AFTER" -ne "$expect_users" ]; then
        echo "ERROR: userproof rows after recovery ($USERPROOF_ROWS_AFTER) != expected ($expect_users)"
        exit 1
    fi
    log "CHECK PASSED: userproof rows == ${expect_users}"

    log_step "Tier ${tier_index} PASSED (crashed at witness ${WITNESS_ROWS_BEFORE}/${expect_batches}, userproof ${USERPROOF_ROWS_BEFORE}/${expect_users}, recovered successfully)"
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

# --- Step 3: Run recovery test for each tier config ---
TIER_INDEX=0
TOTAL_TIERS=${#TIER_CONFIGS[@]}
for tier_config in "${TIER_CONFIGS[@]}"; do
    TIER_INDEX=$((TIER_INDEX + 1))
    IFS='|' read -r test_tiers expect_users expect_batches kill_mode kill_threshold <<< "$tier_config"
    log_step "Running tier config ${TIER_INDEX}/${TOTAL_TIERS}: ${test_tiers} (kill ${kill_mode}@${kill_threshold})"
    run_recovery_test "$test_tiers" "$expect_users" "$expect_batches" "$kill_mode" "$kill_threshold" "${TIER_INDEX}/${TOTAL_TIERS}"
done

# --- Done ---
log_step "ALL ${TOTAL_TIERS} tier configs PASSED"
