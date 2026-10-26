#!/usr/bin/env bash
set -euo pipefail

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
# shellcheck source=container-runtime.sh
source "$root/test/container-runtime.sh"
cd "$root"

# A deliberate integration run must never report success because DB setup skipped.
export REQUIRE_DB=1
if [[ $# == 0 ]]; then
    set -- ./...
fi
if [[ -n ${TEST_DATABASE_URL:-} ]]; then
    # An explicit server is caller-owned. Never start or remove containers here.
    exec go test -tags=integration -count=1 "$@"
fi

configure_container_runtime
configure_postgres_image "$root"
run_dir=$(mktemp -d "${TMPDIR:-/tmp}/pletka-test.XXXXXXXX")
export PLETKA_TEST_CONTAINER_NAME="pletka-testdb-${run_dir##*/}"
# Parallel package processes share this run's server; the launcher owns teardown.
export TESTCONTAINERS_RYUK_DISABLED=true
test_pid=
cleanup() {
    local status=$?
    trap - EXIT
    if "${container_cli[@]}" container inspect "$PLETKA_TEST_CONTAINER_NAME" >/dev/null 2>&1; then
        if ! "${container_cli[@]}" rm -f -v "$PLETKA_TEST_CONTAINER_NAME" >/dev/null; then
            echo "Could not remove test container $PLETKA_TEST_CONTAINER_NAME" >&2
            if [[ $status == 0 ]]; then status=1; fi
        fi
    fi
    rmdir "$run_dir"
    exit "$status"
}
interrupt() {
    local status=$1
    if [[ -n "$test_pid" ]]; then
        kill -TERM "$test_pid" 2>/dev/null || true
        wait "$test_pid" 2>/dev/null || true
    fi
    exit "$status"
}
trap cleanup EXIT
trap 'interrupt 130' INT
trap 'interrupt 143' TERM
echo "Integration database: $TEST_POSTGRES_IMAGE ($TEST_CONTAINER_RUNTIME, $PLETKA_TEST_CONTAINER_NAME)" >&2
go test -tags=integration -count=1 "$@" &
test_pid=$!
wait "$test_pid"
