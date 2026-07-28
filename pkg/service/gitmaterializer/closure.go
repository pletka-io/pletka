package gitmaterializer

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// ScopedRef is one entity whose file(s) a change_set touches. Delete removes
// them; otherwise they are (re)written from current DB state.
type ScopedRef struct {
	EntityType string
	EntityID   string
	Delete     bool
}

// closure expands cs's change_log entries to the within-project set of
// entities whose materialized files derive from them. Refs are deduped by
// (EntityType, EntityID); the first occurrence's Delete flag wins for that
// key (a delete is never downgraded to a rewrite by a later placement ref
// for the same entity, since add() no-ops on repeat keys).
//
// Rules:
//   - model/collection/category entries map to themselves (Delete = op == "delete").
//   - field entries map to the field itself, plus every model/collection that
//     places it (via a weave_field_overrides row) as a non-delete rewrite —
//     their overrides/ subtree embeds the field and must be regenerated even
//     when the field itself was deleted (the placement row is gone too).
//   - override entries resolve to their owning model/collection (non-delete
//     rewrite), or to the field itself for a base override.
func (m *Materializer) closure(ctx context.Context, cs sqlcgen.WeaveChangeSet) ([]ScopedRef, error) {
	entries, err := m.queries.WeaveListChangeLogForChangeSet(ctx, cs.ID)
	if err != nil {
		return nil, fmt.Errorf("list change log for change set %d: %w", cs.ID, err)
	}

	seen := map[string]bool{}
	var out []ScopedRef
	add := func(entityType, entityID string, del bool) {
		key := entityType + "\x00" + entityID
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, ScopedRef{EntityType: entityType, EntityID: entityID, Delete: del})
	}

	for _, e := range entries {
		switch e.EntityType {
		case "model", "collection", "category":
			add(e.EntityType, e.EntityID, e.Operation == "delete")
		case "field":
			add("field", e.EntityID, e.Operation == "delete")
			owners, err := m.placingOwners(ctx, cs.ProjectID, e.EntityID)
			if err != nil {
				return nil, err
			}
			for _, o := range owners {
				add(o.EntityType, o.EntityID, false)
			}
		case "override":
			owner, ok, err := m.overrideOwner(ctx, e.EntityID)
			if err != nil {
				return nil, err
			}
			if !ok {
				// Override row already deleted (change_log entry outlived
				// it). No owner to rewrite from this entry alone; the
				// reconcile backstop (full-tree diff) covers this edge.
				continue
			}
			add(owner.EntityType, owner.EntityID, false)
		}
	}
	return out, nil
}

// placingOwners returns every model/collection in projectID that places
// fieldID via an override row (weave_field_overrides.entity_type in
// ('model', 'collection')).
func (m *Materializer) placingOwners(ctx context.Context, projectID, fieldID string) ([]ScopedRef, error) {
	rows, err := m.queries.WeaveClosurePlacementsForField(ctx, sqlcgen.WeaveClosurePlacementsForFieldParams{
		FieldID:   fieldID,
		ProjectID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("placing owners for field %s: %w", fieldID, err)
	}
	out := make([]ScopedRef, 0, len(rows))
	for _, r := range rows {
		out = append(out, ScopedRef{EntityType: r.EntityType, EntityID: r.EntityID})
	}
	return out, nil
}

// overrideOwner resolves a change_log "override" entry's EntityID (a
// weave_field_overrides row id, encoded as a decimal string) to the entity
// whose materialized file(s) must be rewritten: the owning model/collection
// for a model/collection override, or the field itself for a base override
// (entity_type ""). ok is false when the override row no longer exists —
// the caller skips the entry (see closure's comment on that case).
func (m *Materializer) overrideOwner(ctx context.Context, overrideID string) (ScopedRef, bool, error) {
	id, err := strconv.ParseInt(overrideID, 10, 64)
	if err != nil {
		return ScopedRef{}, false, fmt.Errorf("parse override id %q: %w", overrideID, err)
	}
	row, err := m.queries.WeaveClosureOverrideOwner(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ScopedRef{}, false, nil
		}
		return ScopedRef{}, false, fmt.Errorf("resolve override owner %d: %w", id, err)
	}
	if row.EntityType == "model" || row.EntityType == "collection" {
		return ScopedRef{EntityType: row.EntityType, EntityID: row.EntityID}, true, nil
	}
	// Base override (entity_type ""): no owning model/collection — the
	// field's own materialized files (including its base override) changed.
	return ScopedRef{EntityType: "field", EntityID: row.FieldID}, true, nil
}
