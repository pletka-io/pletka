// Package releasedeps captures and resolves concrete project dependency pins.
package releasedeps

import (
	"context"
	"fmt"
	"sort"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// Capture plans dependencies before persisting membership and closure. It runs
// in the release writer's transaction after owned rows have been archived.
func Capture(ctx context.Context, q *sqlcgen.Queries, projectID, version string) error {
	// The owning release lifecycle provides write authorization. Read sessions
	// supply their own mandatory source policy to the same selection algorithm.
	plan, err := planDependencies(ctx, q, q, projectID, version, func(context.Context, string) error { return nil })
	if err != nil {
		return err
	}

	for _, source := range sortedProjects(plan.pins) {
		if err := q.WeaveInsertReleaseProjectDependency(ctx, sqlcgen.WeaveInsertReleaseProjectDependencyParams{ProjectID: projectID, Version: version, SourceProjectID: source, SourceVersion: plan.pins[source]}); err != nil {
			return fmt.Errorf("retain source release %s@%s: %w", source, plan.pins[source], err)
		}
	}
	keys := make([]entityKey, 0, len(plan.entities))
	for key := range plan.entities {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].kind != keys[j].kind {
			return keys[i].kind < keys[j].kind
		}
		return keys[i].id < keys[j].id
	})
	for _, key := range keys {
		source := plan.entities[key]
		if err := q.WeaveInsertReleaseDependencyEntity(ctx, sqlcgen.WeaveInsertReleaseDependencyEntityParams{ProjectID: projectID, Version: version, EntityType: key.kind, EntityID: key.id, SourceProjectID: source, SourceVersion: plan.pins[source]}); err != nil {
			return fmt.Errorf("capture dependency %s %s: %w", key.kind, key.id, err)
		}
	}
	return q.WeaveMarkReleaseProjectDependencies(ctx, sqlcgen.WeaveMarkReleaseProjectDependenciesParams{ProjectID: projectID, Version: version})
}
