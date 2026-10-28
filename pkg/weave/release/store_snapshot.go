package release

import (
	"context"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

func snapshotOwnedState(ctx context.Context, db sqlcgen.DBTX, projectID, version string) error {
	q := sqlcgen.New(db)
	count, err := q.WeaveSnapshotOwnedProject(ctx, sqlcgen.WeaveSnapshotOwnedProjectParams{ProjectID: projectID, Version: version})
	if err != nil {
		return fmt.Errorf("capture project: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("capture project: expected one project, captured %d", count)
	}
	if err := q.WeaveSnapshotOwnedModels(ctx, sqlcgen.WeaveSnapshotOwnedModelsParams{ProjectID: projectID, Version: version}); err != nil {
		return fmt.Errorf("capture models: %w", err)
	}
	if err := q.WeaveSnapshotOwnedInheritance(ctx, sqlcgen.WeaveSnapshotOwnedInheritanceParams{ProjectID: projectID, Version: version}); err != nil {
		return fmt.Errorf("capture inheritance: %w", err)
	}
	if err := q.WeaveSnapshotOwnedOntologyLinks(ctx, sqlcgen.WeaveSnapshotOwnedOntologyLinksParams{ProjectID: projectID, Version: version}); err != nil {
		return fmt.Errorf("capture ontology links: %w", err)
	}
	if err := q.WeaveSnapshotReleaseNamespaces(ctx, sqlcgen.WeaveSnapshotReleaseNamespacesParams{ProjectID: projectID, Version: version}); err != nil {
		return fmt.Errorf("capture namespaces: %w", err)
	}
	return nil
}
