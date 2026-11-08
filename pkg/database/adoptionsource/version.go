// Package adoptionsource names the source release an adoption receipt was
// taken from when its writer gave none. Every writer of weave_adoptions (the
// adoption store and project restore) uses it, so they cannot disagree.
package adoptionsource

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

// Version returns the consumer's pinned release of the source when that
// release holds the entity, else the newest live release that does. A
// consumer that follows the source's draft on purpose, or an entity no
// release holds, gets an empty version; a release cut refuses while one
// remains.
func Version(ctx context.Context, q *sqlcgen.Queries, projectID, sourceProjectID, entityType, entityID string) (string, error) {
	pin, err := q.WeaveAdoptionSourcePin(ctx, sqlcgen.WeaveAdoptionSourcePinParams{ProjectID: projectID, ParentProjectID: sourceProjectID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return "", fmt.Errorf("read parent link for adoption: %w", err)
	case pin.SourceMode == string(domain.DependencySourceDraft):
		return "", nil
	case pin.SourceVersion != "":
		ok, err := q.WeaveReleaseContainsEntity(ctx, sqlcgen.WeaveReleaseContainsEntityParams{SourceProjectID: sourceProjectID, Version: pin.SourceVersion, EntityType: entityType, EntityID: entityID})
		if err != nil {
			return "", fmt.Errorf("check pinned release for adoption: %w", err)
		}
		if ok {
			return pin.SourceVersion, nil
		}
	}
	version, err := q.WeaveLatestReleaseContaining(ctx, sqlcgen.WeaveLatestReleaseContainingParams{SourceProjectID: sourceProjectID, EntityType: entityType, EntityID: entityID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find release holding adopted entity: %w", err)
	}
	return version, nil
}
