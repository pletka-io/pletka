package releasedeps

import (
	"context"
	"fmt"
	"strings"

	archive "github.com/pletka-io/pletka/pkg/database/readsql/dependencyarchive"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// ReadCaptured validates retained project closure and direct entity membership.
// It has no draft/latest capability. Authorization applies to every source before
// any plan is returned; inventory markers are never treated as access grants.
func ReadCaptured(ctx context.Context, queries archive.Querier, project, version string, authorize func(context.Context, string) error) (Plan, error) {
	if queries == nil || strings.TrimSpace(project) == "" || strings.TrimSpace(version) == "" || authorize == nil {
		return Plan{}, fmt.Errorf("captured dependencies require a project release and source authorization")
	}
	a := archiveAdapter{queries}
	if err := Require(ctx, a, project, version); err != nil {
		return Plan{}, err
	}
	rows, err := queries.WeaveListReleaseProjectDependencies(ctx, archive.WeaveListReleaseProjectDependenciesParams{ProjectID: project, Version: version})
	if err != nil {
		return Plan{}, err
	}
	c := newClosure(project, a, authorize)
	for _, row := range rows {
		if err := c.selectSource(ctx, row.SourceProjectID, row.SourceVersion); err != nil {
			return Plan{}, err
		}
	}
	// A retained inventory must already contain its transitive closure. Never
	// repair a historical omission from a source, even if it can be found today.
	if len(c.pins) != len(rows) {
		return Plan{}, unavailable("incomplete captured project closure for %s@%s", project, version)
	}
	entities, err := queries.WeaveListReleaseDependencyEntities(ctx, archive.WeaveListReleaseDependencyEntitiesParams{ProjectID: project, Version: version})
	if err != nil {
		return Plan{}, err
	}
	membership := map[entityKey]string{}
	for _, e := range entities {
		if pin, ok := c.pins[e.SourceProjectID]; !ok || pin != e.SourceVersion {
			return Plan{}, unavailable("entity dependency has no matching project pin")
		}
		key := entityKey{e.EntityType, e.EntityID}
		exists, err := c.contains(ctx, e.SourceProjectID, e.SourceVersion, key)
		if err != nil {
			return Plan{}, err
		}
		if !exists {
			return Plan{}, unavailable("%s %s is not in source release %s@%s", key.kind, key.id, e.SourceProjectID, e.SourceVersion)
		}
		membership[key] = e.SourceProjectID
	}
	return Plan{pins: c.pins, entities: membership}, nil
}

type archiveAdapter struct{ queries archive.Querier }

func (a archiveAdapter) WeaveReleaseDependencyState(ctx context.Context, p sqlcgen.WeaveReleaseDependencyStateParams) (sqlcgen.WeaveReleaseDependencyStateRow, error) {
	row, err := a.queries.WeaveReleaseDependencyState(ctx, archive.WeaveReleaseDependencyStateParams(p))
	return sqlcgen.WeaveReleaseDependencyStateRow(row), err
}
func (a archiveAdapter) WeaveReleaseContainsDependencyEntity(ctx context.Context, p sqlcgen.WeaveReleaseContainsDependencyEntityParams) (bool, error) {
	return a.queries.WeaveReleaseContainsDependencyEntity(ctx, archive.WeaveReleaseContainsDependencyEntityParams(p))
}
func (a archiveAdapter) WeaveListReleaseProjectDependencies(ctx context.Context, p sqlcgen.WeaveListReleaseProjectDependenciesParams) ([]sqlcgen.WeaveReleaseProjectDependency, error) {
	rows, err := a.queries.WeaveListReleaseProjectDependencies(ctx, archive.WeaveListReleaseProjectDependenciesParams(p))
	if err != nil {
		return nil, err
	}
	out := make([]sqlcgen.WeaveReleaseProjectDependency, len(rows))
	for i, row := range rows {
		out[i] = sqlcgen.WeaveReleaseProjectDependency(row)
	}
	return out, nil
}
