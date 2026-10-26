# Database testing

Run unit tests with `make test`. Run database tests with:

```sh
make test-integration
make test-integration TEST_PACKAGES='./pkg/weave/release ./pkg/service/gitmaterializer'
```

The integration launcher requires a working PostgreSQL server: missing database
access is a failure, never a successful skip. It disables Go's test-result cache,
starts one container per invocation, applies embedded migrations and fixture
snapshots, and clones a separate database for each package. The fixture submodule
and built frontend must be present, as in CI. The launcher removes only its own
container and anonymous volumes on completion or interruption. Parallel runs use
different container names. It never removes a caller-supplied server.

## Docker and Podman

Docker is detected automatically; a `docker` command that wraps Podman selects
Podman. Override with `TEST_CONTAINER_RUNTIME=docker` or `podman`. Explicit
`DOCKER_HOST` is preserved, and cleanup uses the same API endpoint as the tests.

For rootless Podman on Linux, start the user API socket once:

```sh
systemctl --user start podman.socket
TEST_CONTAINER_RUNTIME=podman make test-integration
```

The launcher discovers `$XDG_RUNTIME_DIR/podman/podman.sock`, falling back to
`/run/user/$(id -u)/podman/podman.sock`. For another socket or a Podman machine,
set `DOCKER_HOST` explicitly. It does not change system services or install a
container runtime. See the [Testcontainers Podman instructions](https://golang.testcontainers.org/system_requirements/using_podman/).

## Image and production compatibility

[`internal/testdb/postgres-image.txt`](../../internal/testdb/postgres-image.txt)
is the shared image pin for Go tests, CI and the optional Compose database. It
currently pins the multi-platform PostgreSQL **18.6 / Alpine 3.24** image by digest.
This is a reproducible PG18 baseline, not a claim of exact production parity.
Override it for comparison runs with `TEST_POSTGRES_IMAGE`; update the checked-in
pin after verifying the target server's patch version, OS/libc, architecture,
extensions, encoding, locale provider/collation and relevant server settings.
An Alpine image may not reproduce a glibc-based server's locale behavior.

Direct `go test -tags=integration` also reads the pin, but bypasses the launcher's
runtime setup, mandatory-database flag and cleanup. Prefer the Make target.

## Optional persistent inspection database

Compose is useful for manual migration and audit investigation. Install Docker
Compose or a Podman Compose provider, then:

```sh
make test-db-up       # starts localhost:55432 and prints server version + test DSN
make test-db-psql     # psql inside the matching PostgreSQL container
make test-db-logs
make test-db-down     # stops the server; retains its data
make test-db-reset    # removes this worktree's inspection database and volume
```

On NixOS, a temporary provider is enough:
`nix shell nixpkgs#podman-compose -c make test-db-up`.
Podman's `compose` command [uses an external provider](https://docs.podman.io/en/latest/markdown/podman-compose.1.html).

Each worktree has a separate Compose project and volume. Set `TEST_POSTGRES_PORT`
when running multiple inspection servers. The service exposes only loopback and
uses the public local-only password `pletka-test-only`. It creates an empty
`pletka_inspect` database; migrations and data loading are explicit. PostgreSQL
18's volume is mounted at `/var/lib/postgresql`, following the
[official image layout](https://hub.docker.com/_/postgres).

To run fixture tests on this server:

```sh
TEST_DATABASE_URL='postgres://postgres:pletka-test-only@127.0.0.1:55432/pletka_inspect?sslmode=disable' \
  make test-integration TEST_PACKAGES='./pkg/weave/release ./pkg/service/gitmaterializer'
```

Package tests still clone and discard fixture databases; they do not populate
`pletka_inspect`. Use a dedicated disposable server for `TEST_DATABASE_URL`: the
harness creates/rebuilds a fixture template and requires database creation rights.
Use ordinary automated fixtures for regressions; inspect retained historical
artifacts separately when investigating archive ownership. Recreating today's
draft data cannot establish an old release's membership.

Changing the server major version requires a fresh inspection volume or an
explicit PostgreSQL upgrade. `test-db-down` alone retains the old cluster. Pin
changes do not silently delete it.

## Launcher checks

`python3 test/test_integration_launcher.py` checks runtime failure, test failure,
cleanup ownership, separate run names, explicit endpoints and caller-owned DSNs.
CI runs it without needing a container daemon.
