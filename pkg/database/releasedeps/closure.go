package releasedeps

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

type archiveReader interface {
	inventoryReader
	WeaveListReleaseProjectDependencies(context.Context, sqlcgen.WeaveListReleaseProjectDependenciesParams) ([]sqlcgen.WeaveReleaseProjectDependency, error)
	WeaveReleaseContainsDependencyEntity(context.Context, sqlcgen.WeaveReleaseContainsDependencyEntityParams) (bool, error)
}

// Closure validation has no live-owner, latest-release or write capability.
type closure struct {
	archive   archiveReader
	project   string
	pins      map[string]string
	loaded    map[string]bool
	visiting  map[string]bool
	authorize func(context.Context, string) error
}

func newClosure(project string, archive archiveReader, authorize func(context.Context, string) error) *closure {
	return &closure{project: project, archive: archive, authorize: authorize, pins: map[string]string{}, loaded: map[string]bool{}, visiting: map[string]bool{}}
}

func sortedProjects(pins map[string]string) []string {
	projects := make([]string, 0, len(pins))
	for project := range pins {
		projects = append(projects, project)
	}
	sort.Strings(projects)
	return projects
}

func (c *closure) selectSource(ctx context.Context, project, version string) error {
	if strings.TrimSpace(project) == "" || strings.TrimSpace(version) == "" {
		return unavailable("source requires a concrete project release")
	}
	if project == c.project || c.visiting[project] {
		return unavailable("release dependency cycle reaches %s", project)
	}
	if old, ok := c.pins[project]; ok && old != version {
		return unavailable("conflicting release pins for %s: %s and %s", project, old, version)
	}
	c.pins[project] = version
	if c.loaded[project] {
		return nil
	}
	if err := c.authorize(ctx, project); err != nil {
		return err
	}
	if err := Require(ctx, c.archive, project, version); err != nil {
		return err
	}
	exists, err := c.contains(ctx, project, version, entityKey{"project", project})
	if err != nil {
		return err
	}
	if !exists {
		return unavailable("project %s is missing from its release %s", project, version)
	}
	c.visiting[project] = true
	defer delete(c.visiting, project)
	rows, err := c.archive.WeaveListReleaseProjectDependencies(ctx, sqlcgen.WeaveListReleaseProjectDependenciesParams{ProjectID: project, Version: version})
	if err != nil {
		return fmt.Errorf("read captured dependencies of %s@%s: %w", project, version, err)
	}
	for _, row := range rows {
		if err := c.selectSource(ctx, row.SourceProjectID, row.SourceVersion); err != nil {
			return err
		}
	}
	c.loaded[project] = true
	return nil
}

func (c *closure) contains(ctx context.Context, project, version string, key entityKey) (bool, error) {
	return c.archive.WeaveReleaseContainsDependencyEntity(ctx, sqlcgen.WeaveReleaseContainsDependencyEntityParams{ProjectID: project, Version: version, EntityType: key.kind, EntityID: key.id})
}

func (c *closure) archivedOwner(ctx context.Context, key entityKey) (string, error) {
	owner := ""
	for _, project := range sortedProjects(c.pins) {
		version := c.pins[project]
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
