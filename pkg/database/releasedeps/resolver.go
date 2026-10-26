package releasedeps

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// Resolution receives only captured membership queries. Capture's live/latest
// capabilities are unavailable here, even when called from the same factory.
type inventoryReader interface {
	WeaveReleaseDependencyState(context.Context, sqlcgen.WeaveReleaseDependencyStateParams) (sqlcgen.WeaveReleaseDependencyStateRow, error)
}

type sourceReader interface {
	inventoryReader
	WeaveGetReleaseDependencyEntity(context.Context, sqlcgen.WeaveGetReleaseDependencyEntityParams) (sqlcgen.WeaveReleaseDependencyEntity, error)
}

// UnavailableError indicates that a dependency cannot be proved from captured
// release state. It is also used for old releases without a dependency inventory.
type UnavailableError struct{ Detail string }

func (e *UnavailableError) Error() string { return e.Detail }

// IsNotInRelease selects the canonical missing release-content envelope.
func (e *UnavailableError) IsNotInRelease() bool { return true }

func unavailable(format string, args ...any) error {
	return &UnavailableError{Detail: fmt.Sprintf(format, args...)}
}

// Require checks both ownership and the project dependency inventory marker.
// It does not certify ontology artifacts or complete output coverage.
func Require(ctx context.Context, q inventoryReader, projectID, version string) error {
	row, err := q.WeaveReleaseDependencyState(ctx, sqlcgen.WeaveReleaseDependencyStateParams{ProjectID: projectID, Version: version})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (row.ArchiveOwnershipVersion != 1 || row.ProjectDependenciesVersion != 1)) {
		return unavailable("release %s@%s has no verified project dependency inventory", projectID, version)
	}
	if err != nil {
		return fmt.Errorf("check release dependencies: %w", err)
	}
	return nil
}

// EntitySource resolves captured direct membership only. Neither latest lookup
// nor live entity lookup is allowed when the consumer is already released.
func EntitySource(ctx context.Context, q sourceReader, project, version, kind, id string) (string, string, error) {
	if err := Require(ctx, q, project, version); err != nil {
		return "", "", err
	}
	row, err := q.WeaveGetReleaseDependencyEntity(ctx, sqlcgen.WeaveGetReleaseDependencyEntityParams{ProjectID: project, Version: version, EntityType: kind, EntityID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", unavailable("%s %s is not a captured dependency of %s@%s", kind, id, project, version)
	}
	if err != nil {
		return "", "", fmt.Errorf("read captured dependency: %w", err)
	}
	if err := Require(ctx, q, row.SourceProjectID, row.SourceVersion); err != nil {
		return "", "", err
	}
	return row.SourceProjectID, row.SourceVersion, nil
}
