//go:build integration

package testdb

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// defaultContainerImage is also read by test/postgres.sh for manual inspection.
// Keep the image in one file so CI, direct Go runs and Compose use the same pin.
//
//go:embed postgres-image.txt
var defaultContainerImage string

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

	containerImage := strings.TrimSpace(os.Getenv("TEST_POSTGRES_IMAGE"))
	if containerImage == "" {
		containerImage = strings.TrimSpace(defaultContainerImage)
	}
	containerName := os.Getenv("PLETKA_TEST_CONTAINER_NAME")
	if containerName == "" {
		// Direct go test runs may reuse a server, but never one for another image.
		digest := sha256.Sum256([]byte(containerImage))
		containerName = fmt.Sprintf("pletka-testdb-%x", digest[:6])
	}
	slog.InfoContext(ctx, "testdb: container", "image", containerImage, "name", containerName)
	ctr, err := postgres.Run(ctx, containerImage,
		postgres.WithDatabase("postgres"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		postgres.BasicWaitStrategies(),
		testcontainers.WithReuseByName(containerName),
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
