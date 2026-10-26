#!/usr/bin/env bash
# Sourced by the test launchers. Keep container operations on the same endpoint
# as Testcontainers; never clean up through an unrelated default daemon.

configure_container_runtime() {
    local runtime=${TEST_CONTAINER_RUNTIME:-}
    local podman_socket=
    if [[ -z "$runtime" ]]; then
        if [[ ${DOCKER_HOST:-} == *podman* ]]; then
            runtime=podman
        elif command -v docker >/dev/null 2>&1; then
            # Podman's docker compatibility executable may print "docker version".
            # Its API socket field identifies it without relying on that banner.
            podman_socket=$(docker info --format '{{.Host.RemoteSocket.Path}}' 2>/dev/null) || podman_socket=
            if [[ -n "$podman_socket" || $(docker --version) == *[Pp]odman* ]]; then
                runtime=podman
            else
                runtime=docker
            fi
        else
            runtime=podman
        fi
    fi
    case "$runtime" in
        docker|podman) ;;
        *) echo "TEST_CONTAINER_RUNTIME must be docker or podman" >&2; return 1 ;;
    esac
    if ! command -v "$runtime" >/dev/null 2>&1; then
        echo "Install $runtime or set TEST_CONTAINER_RUNTIME to an available runtime" >&2
        return 1
    fi

    container_cli=("$runtime")
    if [[ "$runtime" == podman ]]; then
        if [[ -z ${DOCKER_HOST:-} ]]; then
            local socket=${podman_socket:-"${XDG_RUNTIME_DIR:-/run/user/$(id -u)}/podman/podman.sock"}
            if [[ ! -S "$socket" ]]; then
                echo "Podman API socket unavailable: $socket. Start podman.socket or set DOCKER_HOST explicitly." >&2
                return 1
            fi
            export DOCKER_HOST="unix://$socket"
        fi
        container_cli+=(--remote --url "$DOCKER_HOST")
    elif [[ -z ${DOCKER_HOST:-} ]]; then
        # Respect Docker Desktop/custom contexts when exporting the endpoint to Go.
        DOCKER_HOST=$(docker context inspect --format '{{.Endpoints.docker.Host}}') || return 1
        if [[ -z "$DOCKER_HOST" ]]; then
            echo "Docker context has no endpoint; set DOCKER_HOST explicitly" >&2
            return 1
        fi
        export DOCKER_HOST
    fi
    if [[ "$runtime" == docker ]]; then
        # A CLI flag also takes precedence over an inherited DOCKER_CONTEXT.
        container_cli+=(--host "$DOCKER_HOST")
    fi
    if ! "${container_cli[@]}" info >/dev/null; then
        echo "Container API unavailable; database tests cannot run" >&2
        return 1
    fi
    export TEST_CONTAINER_RUNTIME="$runtime"
}

configure_postgres_image() {
    local root=$1
    if [[ -z ${TEST_POSTGRES_IMAGE:-} ]]; then
        TEST_POSTGRES_IMAGE=$(<"$root/internal/testdb/postgres-image.txt")
    fi
    export TEST_POSTGRES_IMAGE
}
