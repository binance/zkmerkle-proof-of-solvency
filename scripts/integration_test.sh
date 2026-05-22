#!/bin/bash
set -euo pipefail

##############################################################################
# PoR Integration Test
#
# Runs the full pipeline: keygen -> witness -> dbtool -> prover -> verifier
# Uses ZKPOR_TEST_TIERS env var for small batch sizes.
#
# Prerequisites: docker, go 1.22+, mysql client
# Usage: ./scripts/integration_test.sh
##############################################################################

# --- Configuration ---
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK_DIR="${PROJECT_ROOT}/_integration_test"
TEST_TIERS="500:30,50:220"
EXPECT_TOTAL_USERS_ACCOUNT=10240
EXPECT_TOTAL_PROOFS=50
MYSQL_PORT=13306
REDIS_PORT=16379
MYSQL_CONTAINER="zkpos-integration-mysql"
REDIS_CONTAINER="zkpos-integration-redis"
MYSQL_USER="zkpos"
MYSQL_PASSWORD="zkpos@123"
MYSQL_DATABASE="zkpos"
MYSQL_ROOT_PASSWORD="zkpos@123"
DB_SUFFIX="0"
NUM_USER_PROOFS_TO_VERIFY=500
witness_done_marker=/tmp/witness_done

# Keygen file names derived from TEST_TIERS
KEYGEN_FILES=(
    "zkpor500_30.pk" "zkpor500_30.vk" "zkpor500_30.r1cs"
    "zkpor50_220.pk" "zkpor50_220.vk" "zkpor50_220.r1cs"
)

# --- Helper functions ---
log() { echo "=== [$(date '+%H:%M:%S')] $*"; }
log_step() { echo ""; echo "###############################################"; log "$*"; echo "###############################################"; }

cleanup() {
    log "Cleaning up previous run..."
    docker rm -f "$MYSQL_CONTAINER" "$REDIS_CONTAINER" 2>/dev/null || true
    # Keep keygen files (pk/vk/r1cs) for caching.
    # Remove config dir and proof csv only.
    rm -rf "${WORK_DIR}/config"
    log "Cleanup done (keygen files preserved in ${WORK_DIR})"
    rm -f "$witness_done_marker"
    log "Removed witness done marker if it existed"
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

wait_for_redis() {
    log "Waiting for Redis to be ready..."
    local max_wait=30
    local waited=0
    while ! docker exec "$REDIS_CONTAINER" redis-cli ping 2>/dev/null | grep -q PONG; do
        sleep 1
        waited=$((waited + 1))
        if [ "$waited" -ge "$max_wait" ]; then
            echo "ERROR: Redis not ready after ${max_wait}s"
            exit 1
        fi
    done
    log "Redis is ready (waited ${waited}s)"
}

write_config() {
    local config_content="$1"
    mkdir -p "${WORK_DIR}/config"
    echo "$config_content" > "${WORK_DIR}/config/config.json"
}

# --- Step 0: Clean up previous run ---
log_step "Step 0: Cleaning up previous run"
cleanup

# --- Step 1: Start Docker containers ---
log_step "Step 1: Starting Docker containers"

docker run -d --name "$MYSQL_CONTAINER" \
    -p "${MYSQL_PORT}:3306" \
    -e MYSQL_USER="$MYSQL_USER" \
    -e MYSQL_PASSWORD="$MYSQL_PASSWORD" \
    -e MYSQL_DATABASE="$MYSQL_DATABASE" \
    -e MYSQL_ROOT_PASSWORD="$MYSQL_ROOT_PASSWORD" \
    mysql

docker run -d --name "$REDIS_CONTAINER" \
    -p "${REDIS_PORT}:6379" \
    redis

wait_for_mysql
wait_for_redis

MYSQL_DSN="${MYSQL_USER}:${MYSQL_PASSWORD}@tcp(127.0.0.1:${MYSQL_PORT})/${MYSQL_DATABASE}?parseTime=true&readTimeout=60s"

# --- Step 2: Build all service binaries ---
log_step "Step 2: Building service binaries"

mkdir -p "$WORK_DIR"

cd "$PROJECT_ROOT"
go build -o "${WORK_DIR}/keygen"   ./src/keygen
go build -o "${WORK_DIR}/witness"  ./src/witness
go build -o "${WORK_DIR}/dbtool"   ./src/dbtool
go build -o "${WORK_DIR}/prover"   ./src/prover
go build -o "${WORK_DIR}/verifier" ./src/verifier
log "All binaries built"

# --- Step 3: Keygen (with caching) ---
log_step "Step 3: Keygen"

all_cached=true
for f in "${KEYGEN_FILES[@]}"; do
    if [ ! -f "${WORK_DIR}/${f}" ]; then
        all_cached=false
        break
    fi
done

if [ "$all_cached" = true ]; then
    log "All keygen files cached, skipping keygen"
else
    log "Running keygen (this may take a while)..."
    cd "$WORK_DIR"
    ZKPOR_TEST_TIERS="$TEST_TIERS" ./keygen
    cd "$PROJECT_ROOT"
    log "Keygen complete"
fi

# --- Step 4: Run witness ---
log_step "Step 4: Running witness"

write_config "$(cat <<EOF
{
    "MysqlDataSource": "${MYSQL_DSN}",
    "DbSuffix": "${DB_SUFFIX}",
    "UserDataFile": "${PROJECT_ROOT}/src/userdata"
}
EOF
)"

cd "$WORK_DIR"
ZKPOR_TEST_TIERS="$TEST_TIERS" ./witness -witness_done_marker "$witness_done_marker" &
WITNESS_PID=$!
cd "$PROJECT_ROOT"
# Wait for witness generation to complete.
while [ ! -f "$witness_done_marker" ]; do
    if ! kill -0 $WITNESS_PID 2>/dev/null; then
        # Process exited — check marker one last time in case it was created just before exit.
        [ -f "$witness_done_marker" ] && break
        echo "ERROR: witness process exited before witness generation completed"
        exit 1
    fi
    sleep 1
    log "Waiting for witness generation to complete..."
done
rm -f "$witness_done_marker"
log "Witness complete"

# --- Step 5: Push tasks to Redis ---
log_step "Step 5: Pushing tasks to Redis"

write_config "$(cat <<EOF
{
    "MysqlDataSource": "${MYSQL_DSN}",
    "DbSuffix": "${DB_SUFFIX}",
    "Redis": {
        "Host": "127.0.0.1:${REDIS_PORT}",
        "Password": ""
    }
}
EOF
)"

cd "$WORK_DIR"
./dbtool -push_task_to_redis
cd "$PROJECT_ROOT"
log "Tasks pushed to Redis"

# --- Step 6: Run prover ---
log_step "Step 6: Running prover"

write_config "$(cat <<EOF
{
    "MysqlDataSource": "${MYSQL_DSN}",
    "DbSuffix": "${DB_SUFFIX}",
    "Redis": {
        "Host": "127.0.0.1:${REDIS_PORT}",
        "Password": ""
    },
    "ZkKeyName": ["zkpor50_220", "zkpor500_30"],
    "AssetsCountTiers": [50, 500]
}
EOF
)"

cd "$WORK_DIR"
ZKPOR_TEST_TIERS="$TEST_TIERS" ./prover
cd "$PROJECT_ROOT"
log "Prover complete"

wait $WITNESS_PID
log "Witness and userproof process complete"

# --- Step 7: Export proofs to CSV ---
log_step "Step 7: Exporting proofs to CSV"

write_config "$(cat <<EOF
{
    "MysqlDataSource": "${MYSQL_DSN}",
    "DbSuffix": "${DB_SUFFIX}",
    "Redis": {
        "Host": "127.0.0.1:${REDIS_PORT}",
        "Password": ""
    }
}
EOF
)"

cd "$WORK_DIR"
./dbtool -export_proof_csv config/proof.csv
cd "$PROJECT_ROOT"
log "Proof CSV exported"

# --- Step 7.5: Validate table row counts ---
log_step "Step 7.5: Validating table row counts"

MYSQL_QUERY="docker exec $MYSQL_CONTAINER mysql -u$MYSQL_USER -p$MYSQL_PASSWORD $MYSQL_DATABASE -N -B -e"

USERPROOF_ROWS=$($MYSQL_QUERY "SELECT COUNT(*) FROM userproof${DB_SUFFIX}")
PROOF_ROWS=$($MYSQL_QUERY "SELECT COUNT(*) FROM proof${DB_SUFFIX}")

log "userproof rows: ${USERPROOF_ROWS} (expected: ${EXPECT_TOTAL_USERS_ACCOUNT})"
log "proof rows:     ${PROOF_ROWS} (expected: ${EXPECT_TOTAL_PROOFS})"

if [ "$USERPROOF_ROWS" -ne "$EXPECT_TOTAL_USERS_ACCOUNT" ]; then
    echo "ERROR: userproof rows ($USERPROOF_ROWS) != expected ($EXPECT_TOTAL_USERS_ACCOUNT)"
    exit 1
fi
log "CHECK PASSED: userproof rows == ${EXPECT_TOTAL_USERS_ACCOUNT}"

if [ "$PROOF_ROWS" -ne "$EXPECT_TOTAL_PROOFS" ]; then
    echo "ERROR: proof rows ($PROOF_ROWS) != expected ($EXPECT_TOTAL_PROOFS)"
    exit 1
fi
log "CHECK PASSED: proof rows == ${EXPECT_TOTAL_PROOFS}"

# --- Step 8: Batch verification ---
log_step "Step 8: Batch proof verification"

# Build verifier config: get CexAssetsInfo from latest witness
cd "$WORK_DIR"
CEX_ASSETS_JSON=$(./dbtool -query_cex_assets)
cd "$PROJECT_ROOT"

cat > "${WORK_DIR}/config/config.json" <<EOF
{
    "ProofTable": "config/proof.csv",
    "ZkKeyName": ["zkpor50_220", "zkpor500_30"],
    "AssetsCountTiers": [50, 500],
    "CexAssetsInfo": ${CEX_ASSETS_JSON}
}
EOF

cd "$WORK_DIR"
./verifier
cd "$PROJECT_ROOT"
log "Batch verification PASSED"

# --- Step 9: Random user proof verification ---
log_step "Step 9: User proof verification (${NUM_USER_PROOFS_TO_VERIFY} random users)"

# Get max account index
write_config "$(cat <<EOF
{
    "MysqlDataSource": "${MYSQL_DSN}",
    "DbSuffix": "${DB_SUFFIX}",
    "Redis": {
        "Host": "127.0.0.1:${REDIS_PORT}",
        "Password": ""
    }
}
EOF
)"

# Get total user count by querying the latest account index
MAX_INDEX=$(docker exec "$MYSQL_CONTAINER" mysql -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" \
    "$MYSQL_DATABASE" -N -B -e "SELECT MAX(account_index) FROM userproof${DB_SUFFIX}")

if [ -z "$MAX_INDEX" ] || [ "$MAX_INDEX" = "NULL" ]; then
    echo "ERROR: No user proofs found"
    exit 1
fi
log "Total accounts: 0..${MAX_INDEX}"

# Generate random indices and verify each
PASS_COUNT=0
for i in $(seq 1 "$NUM_USER_PROOFS_TO_VERIFY"); do
    RANDOM_INDEX=$((RANDOM % (MAX_INDEX + 1)))
    log "Verifying user proof for account index ${RANDOM_INDEX} (${i}/${NUM_USER_PROOFS_TO_VERIFY})..."

    cd "$WORK_DIR"
    USER_CONFIG=$(./dbtool -query_account_data "$RANDOM_INDEX")
    cd "$PROJECT_ROOT"

    echo "$USER_CONFIG" > "${WORK_DIR}/config/user_config.json"

    cd "$WORK_DIR"
    OUTPUT=$(./verifier -user 2>&1)
    cd "$PROJECT_ROOT"

    if echo "$OUTPUT" | grep -q "verify pass"; then
        log "  Account ${RANDOM_INDEX}: PASS"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        echo "  Account ${RANDOM_INDEX}: FAIL"
        echo "  Output: ${OUTPUT}"
        exit 1
    fi
done

log "All ${PASS_COUNT}/${NUM_USER_PROOFS_TO_VERIFY} user proofs verified"

# --- Done ---
log_step "Integration test PASSED"
