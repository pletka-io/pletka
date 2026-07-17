package integrations

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
)

// ProjectArtifactContext is the narrow app-built context platform
// integrations receive when constructing project-level artifact providers.
type ProjectArtifactContext struct {
	Pool       *pgxpool.Pool
	Weave      domain.WeaveStore
	Logger     *slog.Logger
	Generators *generators.Service
}
