#!/usr/bin/env bash
set -euo pipefail

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
# shellcheck source=container-runtime.sh
source "$root/test/container-runtime.sh"
configure_container_runtime
configure_postgres_image "$root"

# Worktrees get separate containers and volumes; use TEST_POSTGRES_PORT if more
# than one inspection server needs to run at the same time.
project="pletka-inspect-$(printf '%s' "$root" | cksum | cut -d ' ' -f 1)"
compose=("${container_cli[@]}" compose -p "$project" -f "$root/test/postgres.compose.yaml")
case ${1:-} in
    up)
        "${compose[@]}" up -d
        ready=false
        for ((attempt=0; attempt<60; attempt++)); do
            if "${compose[@]}" exec -T postgres pg_isready -U postgres -d pletka_inspect >/dev/null 2>&1; then
                ready=true
                break
            fi
            sleep 1
        done
        if [[ "$ready" != true ]]; then
            echo "Inspection PostgreSQL did not become ready; inspect with make test-db-logs" >&2
            exit 1
        fi
        "${compose[@]}" exec -T postgres psql -U postgres -d pletka_inspect -c 'SELECT version();'
        echo "TEST_DATABASE_URL=postgres://postgres:pletka-test-only@127.0.0.1:${TEST_POSTGRES_PORT:-55432}/pletka_inspect?sslmode=disable"
        ;;
    down) "${compose[@]}" down ;;
    reset) "${compose[@]}" down --volumes ;;
    logs) "${compose[@]}" logs postgres ;;
    psql) shift; "${compose[@]}" exec -T postgres psql -U postgres -d pletka_inspect "$@" ;;
    config) "${compose[@]}" config ;;
    *) echo "Usage: bash test/postgres.sh {up|down|reset|logs|psql|config}" >&2; exit 2 ;;
esac
