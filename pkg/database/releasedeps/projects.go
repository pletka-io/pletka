// Package releasedeps captures and resolves immutable project dependency pins.
// It never resolves a released consumer through a source's current draft.
package releasedeps

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

type entityKey struct{ kind, id string }

type capture struct {
	queries  *sqlcgen.Queries
	project  string
	pins     map[string]string
	loaded   map[string]bool
	entities map[entityKey]string
}

// Capture records direct entity membership and the transitive project closure.
// Call inside the release cut transaction, after own rows were archived. Sources
// must already have captured inventories: historical pins are never guessed.
// All explicit source choices are reconciled before any unpinned reference uses
// latest. Conflicting versions of one source project fail rather than depend on
// traversal order; the current reader/assembler identity has one source per project.
func Capture(ctx context.Context, q *sqlcgen.Queries, projectID, version string) error {
	refs, err := q.WeaveDraftDependencyReferences(ctx, projectID)
	if err != nil {
		return fmt.Errorf("inventory project dependencies: %w", err)
	}
	c := &capture{queries: q, project: projectID, pins: map[string]string{}, loaded: map[string]bool{}, entities: map[entityKey]string{}}
	for _, ref := range refs {
		if ref.SourceProjectID != "" && ref.SourceProjectID != projectID && strings.TrimSpace(ref.SourceVersion) != "" {
			if err := c.selectSource(ctx, ref.SourceProjectID, ref.SourceVersion); err != nil {
				return err
			}
		}
	}
	choices, err := c.resolveDirectSources(ctx, refs)
	if err != nil {
		return err
	}
	for _, source := range sortedProjects(choices) {
		if err := c.selectSource(ctx, source, choices[source]); err != nil {
			return err
		}
	}
	for _, ref := range refs {
		if err := c.reference(ctx, ref, version); err != nil {
			return err
		}
	}
	for _, source := range sortedProjects(c.pins) {
		if err := q.WeaveInsertReleaseProjectDependency(ctx, sqlcgen.WeaveInsertReleaseProjectDependencyParams{ProjectID: projectID, Version: version, SourceProjectID: source, SourceVersion: c.pins[source]}); err != nil {
			return fmt.Errorf("retain source release %s@%s: %w", source, c.pins[source], err)
		}
	}
	keys := make([]entityKey, 0, len(c.entities))
	for key := range c.entities {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].kind != keys[j].kind {
			return keys[i].kind < keys[j].kind
		}
		return keys[i].id < keys[j].id
	})
	for _, key := range keys {
		source := c.entities[key]
		if err := q.WeaveInsertReleaseDependencyEntity(ctx, sqlcgen.WeaveInsertReleaseDependencyEntityParams{ProjectID: projectID, Version: version, EntityType: key.kind, EntityID: key.id, SourceProjectID: source, SourceVersion: c.pins[source]}); err != nil {
			return fmt.Errorf("capture dependency %s %s: %w", key.kind, key.id, err)
		}
	}
	return q.WeaveMarkReleaseProjectDependencies(ctx, sqlcgen.WeaveMarkReleaseProjectDependenciesParams{ProjectID: projectID, Version: version})
}

func (c *capture) resolveDirectSources(ctx context.Context, refs []sqlcgen.WeaveDraftDependencyReferencesRow) (map[string]string, error) {
	// Explicit choices (including their captured closure) constrain unpinned
	// references. Resolve every remaining direct source to latest BEFORE merging
	// any of their closures; otherwise whichever edge is visited first wins.
	choices := make(map[string]string, len(c.pins))
	for project, pin := range c.pins {
		choices[project] = pin
	}
	owners := map[entityKey]string{}
	for _, ref := range refs {
		if ref.SourceProjectID == "" {
			continue
		}
		key := entityKey{ref.EntityType, ref.EntityID}
		if old, ok := owners[key]; ok && old != ref.SourceProjectID {
			return nil, unavailable("conflicting source owners for %s %s", key.kind, key.id)
		}
		owners[key] = ref.SourceProjectID
	}
	for i := range refs {
		ref := &refs[i]
		key := entityKey{ref.EntityType, ref.EntityID}
		if ref.SourceProjectID == "" {
			ref.SourceProjectID = owners[key]
			if ref.SourceProjectID == "" {
				var err error
				ref.SourceProjectID, err = c.queries.WeaveDraftDependencyEntityOwner(ctx, sqlcgen.WeaveDraftDependencyEntityOwnerParams{EntityType: key.kind, EntityID: key.id})
				if errors.Is(err, pgx.ErrNoRows) {
					ref.SourceProjectID, err = c.archivedOwner(ctx, key)
				}
				if err != nil {
					return nil, err
				}
			}
		}
		if ref.SourceProjectID == "" {
			return nil, unavailable("%s %s has no owning project", key.kind, key.id)
		}
		if ref.SourceProjectID == c.project || choices[ref.SourceProjectID] != "" {
			continue
		}
		pin, err := c.queries.WeaveLatestDependencyRelease(ctx, ref.SourceProjectID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, unavailable("source project %s has no published release for %s %s", ref.SourceProjectID, key.kind, key.id)
		}
		if err != nil {
			return nil, fmt.Errorf("resolve source release: %w", err)
		}
		choices[ref.SourceProjectID] = pin
	}
	return choices, nil
}

func sortedProjects(pins map[string]string) []string {
	projects := make([]string, 0, len(pins))
	for project := range pins {
		projects = append(projects, project)
	}
	sort.Strings(projects)
	return projects
}

func (c *capture) selectSource(ctx context.Context, project, version string) error {
	if project == c.project {
		return unavailable("release dependency cycle reaches %s", project)
	}
	if old, ok := c.pins[project]; ok && old != version {
		return unavailable("conflicting release pins for %s: %s and %s", project, old, version)
	}
	c.pins[project] = version
	if c.loaded[project] {
		return nil
	}
	if err := Require(ctx, c.queries, project, version); err != nil {
		return err
	}
	exists, err := c.contains(ctx, project, version, entityKey{"project", project})
	if err != nil {
		return err
	}
	if !exists {
		return unavailable("project %s is missing from its release %s", project, version)
	}
	c.loaded[project] = true
	rows, err := c.queries.WeaveListReleaseProjectDependencies(ctx, sqlcgen.WeaveListReleaseProjectDependenciesParams{ProjectID: project, Version: version})
	if err != nil {
		return fmt.Errorf("read captured dependencies of %s@%s: %w", project, version, err)
	}
	for _, row := range rows {
		if err := c.selectSource(ctx, row.SourceProjectID, row.SourceVersion); err != nil {
			return err
		}
	}
	return nil
}

func (c *capture) reference(ctx context.Context, ref sqlcgen.WeaveDraftDependencyReferencesRow, ownVersion string) error {
	key := entityKey{ref.EntityType, ref.EntityID}
	source := ref.SourceProjectID
	version := c.pins[source]
	if source == c.project {
		version = ownVersion
	}
	exists, err := c.contains(ctx, source, version, key)
	if err != nil {
		return err
	}
	if !exists {
		return unavailable("%s %s is not in source release %s@%s", key.kind, key.id, source, version)
	}
	if source != c.project {
		c.entities[key] = source
	}
	return nil
}

func (c *capture) contains(ctx context.Context, project, version string, key entityKey) (bool, error) {
	return c.queries.WeaveReleaseContainsDependencyEntity(ctx, sqlcgen.WeaveReleaseContainsDependencyEntityParams{ProjectID: project, Version: version, EntityType: key.kind, EntityID: key.id})
}

func (c *capture) archivedOwner(ctx context.Context, key entityKey) (string, error) {
	owner := ""
	for project, version := range c.pins {
		ok, err := c.contains(ctx, project, version, key)
		if err != nil {
			return "", err
		}
		if ok {
			if owner != "" && owner != project {
				return "", unavailable("ambiguous archived owner of %s %s", key.kind, key.id)
			}
			owner = project
		}
	}
	if owner == "" {
		return "", unavailable("unresolved %s dependency %s", key.kind, key.id)
	}
	return owner, nil
}
