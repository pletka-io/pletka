//go:build integration

package testdb

import (
	"context"
	"fmt"
	"os"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// reuseContainerName is the stable, label-keyed name that lets parallel test
// binaries (and successive `go test` runs) attach to one shared Postgres
// instance instead of each spinning up their own. Ryuk still owns teardown.
const reuseContainerName = "pletka-testdb-pg18"

// containerImage pins the shared Postgres major version for the fixture server.
const containerImage = "postgres:18-alpine"

// init wires the container provider into setup.go's build-agnostic seam. Only
// this file (//go:build integration) imports testcontainers-go, so the untagged
// unit build leaves provideAdminDSN nil and never pulls the Docker deps. This
// init is the sanctioned deviation from core's no-init rule: TestMain calls
// Setup before any tagged code could inject the provider explicitly, so the
// seam must be populated at package load time.
func init() { provideAdminDSN = containerAdminDSN }

// containerAdminDSN returns an admin DSN for a running Postgres server. If
// TEST_DATABASE_URL is set it is returned verbatim (CI-managed Postgres, no
// container). Otherwise a shared, reuse-by-name Postgres container is started
// (or attached to if already running) and its connection string is returned.
func containerAdminDSN(ctx context.Context) (string, error) {
	if dsn := os.Getenv("TEST_DATABASE_URL"); dsn != "" {
		return dsn, nil
	}

	ctr, err := postgres.Run(ctx, containerImage,
		postgres.WithDatabase("postgres"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		postgres.BasicWaitStrategies(),
		testcontainers.WithReuseByName(reuseContainerName),
	)
	if err != nil {
		return "", fmt.Errorf("start postgres container: %w", err)
	}

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return "", fmt.Errorf("container connection string: %w", err)
	}
	return dsn, nil
}
