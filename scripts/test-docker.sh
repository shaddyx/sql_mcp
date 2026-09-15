#!/usr/bin/env bash
# Test the SQL Tool MCP server end-to-end against PostgreSQL, MySQL and SQLite.
# PostgreSQL and MySQL run in Docker; SQLite is tested directly from the binary.
#
# Usage:
#   ./scripts/test-docker.sh            # full run
#   ./scripts/test-docker.sh --keep     # leave containers running afterwards
#   ./scripts/test-docker.sh --no-up    # assume containers already running
set -euo pipefail
IFS=$'\n\t'

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
readonly LOG_DIR="${VIBE_SCRATCHPAD:-/tmp/sql-tool-logs}"

KEEP=0
NO_UP=0

usage() {
    echo "Usage: $0 [--keep] [--no-up]"
    echo "  --keep    leave Docker containers running after the test"
    echo "  --no-up   skip 'docker compose up'; assume the databases are already healthy"
    exit 2
}

for arg in "$@"; do
    case "${arg}" in
        --keep) KEEP=1 ;;
        --no-up) NO_UP=1 ;;
        -h|--help) usage ;;
        *) echo "unknown argument: ${arg}" >&2; usage ;;
    esac
done

log() { echo "[test-docker] $*"; }
die() { echo "[test-docker] ERROR: $*" >&2; exit 1; }

mkdir -p "${LOG_DIR}"

if [[ "${NO_UP}" -eq 0 ]]; then
    log "starting postgres and mysql containers"
    docker compose -f "${REPO_DIR}/docker-compose.yml" up -d

    log "waiting for databases to become healthy"
    for _ in $(seq 1 60); do
        healthy=1
        for svc in "postgres" "mysql"; do
            status="$(docker inspect --format '{{.State.Health.Status}}' "sql_tool-${svc}-1" 2>/dev/null || true)"
            if [[ "${status}" != "healthy" ]]; then
                healthy=0
                break
            fi
        done
        if [[ "${healthy}" -eq 1 ]]; then
            break
        fi
        sleep 2
    done

    if [[ "${healthy}" -ne 1 ]]; then
        die "databases did not become healthy within 120 seconds"
    fi
    log "databases are healthy"
fi

cleanup() {
    if [[ "${KEEP}" -eq 0 && "${NO_UP}" -eq 0 ]]; then
        log "stopping containers"
        docker compose -f "${REPO_DIR}/docker-compose.yml" down
    fi
}
trap cleanup EXIT

log "building server and running integration tests"
cd "${REPO_DIR}"

export SQL_TOOL_TEST_POSTGRES_URL="${SQL_TOOL_TEST_POSTGRES_URL:-postgresql://test:test@localhost:5433/testdb}"
export SQL_TOOL_TEST_MYSQL_URL="${SQL_TOOL_TEST_MYSQL_URL:-mysql://test:test@localhost:3307/testdb}"

go test -tags=integration -race -v ./integration/ > "${LOG_DIR}/integration-tests.log" 2>&1
status=$?

if [[ "${status}" -eq 0 ]]; then
    log "integration tests passed"
else
    log "integration tests FAILED (see ${LOG_DIR}/integration-tests.log)"
    exit "${status}"
fi
