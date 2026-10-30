package releasedeps

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

type entityKey struct{ kind, id string }

// These interfaces contain SELECTs only. Capture can supply its queries;
// read sessions supply generated read-only capabilities through private adapters.
type draftReader interface {
	WeaveDraftDependencyReferences(context.Context, string) ([]sqlcgen.WeaveDraftDependencyReferencesRow, error)
	WeaveDraftDependencyEntityOwner(context.Context, sqlcgen.WeaveDraftDependencyEntityOwnerParams) (string, error)
	WeaveLatestDependencyRelease(context.Context, string) (string, error)
	WeaveDraftFollowingDependencies(context.Context, string) ([]string, error)
}
type planner struct {
	*closure
	draft    draftReader
	entities map[entityKey]string
}

func planDependencies(ctx context.Context, draft draftReader, archive archiveReader, project, ownVersion string, authorize func(context.Context, string) error) (Plan, error) {
	if strings.TrimSpace(project) == "" || authorize == nil {
		return Plan{}, fmt.Errorf("dependency planning requires a project and source authorization")
	}
	following, err := draft.WeaveDraftFollowingDependencies(ctx, project)
	if err != nil {
		return Plan{}, fmt.Errorf("read dependency source modes: %w", err)
	}
	if len(following) != 0 {
		return Plan{}, unavailable("project %s explicitly follows source drafts %s; release dependency reads require release selections", project, strings.Join(following, ", "))
	}
	refs, err := draft.WeaveDraftDependencyReferences(ctx, project)
	if err != nil {
		return Plan{}, fmt.Errorf("inventory project dependencies: %w", err)
	}
	c := &planner{closure: newClosure(project, archive, authorize), draft: draft, entities: map[entityKey]string{}}
	// Reconcile explicit pins and their closure before choosing any latest release.
	for _, ref := range refs {
		if ref.SourceProjectID != "" && ref.SourceProjectID != project && strings.TrimSpace(ref.SourceVersion) != "" {
			if err := c.selectSource(ctx, ref.SourceProjectID, ref.SourceVersion); err != nil {
				return Plan{}, err
			}
		}
	}
	choices, err := c.resolveDirectSources(ctx, refs)
	if err != nil {
		return Plan{}, err
	}
	for _, source := range sortedProjects(choices) {
		if err := c.selectSource(ctx, source, choices[source]); err != nil {
			return Plan{}, err
		}
	}
	for _, ref := range refs {
		if err := c.reference(ctx, ref, ownVersion); err != nil {
			return Plan{}, err
		}
	}
	return Plan{pins: c.pins, entities: c.entities}, nil
}

func (c *planner) resolveDirectSources(ctx context.Context, refs []sqlcgen.WeaveDraftDependencyReferencesRow) (map[string]string, error) {
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
				ref.SourceProjectID, err = c.draft.WeaveDraftDependencyEntityOwner(ctx, sqlcgen.WeaveDraftDependencyEntityOwnerParams{EntityType: key.kind, EntityID: key.id})
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
		if err := c.authorize(ctx, ref.SourceProjectID); err != nil {
			return nil, err
		}
		pin, err := c.draft.WeaveLatestDependencyRelease(ctx, ref.SourceProjectID)
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

func (c *planner) reference(ctx context.Context, ref sqlcgen.WeaveDraftDependencyReferencesRow, ownVersion string) error {
	key := entityKey{ref.EntityType, ref.EntityID}
	source := ref.SourceProjectID
	version := c.pins[source]
	if source == c.project {
		version = ownVersion
	}
	var exists bool
	var err error
	if source == c.project && ownVersion == "" {
		var owner string
		owner, err = c.draft.WeaveDraftDependencyEntityOwner(ctx, sqlcgen.WeaveDraftDependencyEntityOwnerParams{EntityType: key.kind, EntityID: key.id})
		exists = owner == c.project
		if errors.Is(err, pgx.ErrNoRows) {
			err = nil
			exists = false
		}
	} else {
		exists, err = c.contains(ctx, source, version, key)
	}
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
