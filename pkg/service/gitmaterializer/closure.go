package gitmaterializer

import (
	"context"
	"encoding/json"
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
//   - model/collection entries map to themselves (Delete = op == "delete").
//   - category entries map to themselves too, EXCEPT a category delete:
//     categories are the one entity where ID != SemanticID, but files are
//     written at categories/<SemanticID>.yaml while a delete change_log
//     entry's EntityID is the ULID. The ref's EntityID is resolved to the
//     SemanticID from the entry's previous_payload so deletePathsFor removes
//     the file that actually exists on disk (see categorySemanticIDFromPayload).
//   - field entries map to the field itself, plus every model/collection that
//     places it (via a weave_field_overrides row) as a non-delete rewrite —
//     their overrides/ subtree embeds the field and must be regenerated even
//     when the field itself was deleted (the placement row is gone too).
//   - override entries resolve to their owning model/collection (non-delete
//     rewrite), or to the field itself for a base override.
//   - namespace_binding/project-ontology-version entries (project-level
//     "draft" change_log kinds — no owning model/collection/field) map to a
//     single synthetic "project_manifest" ref, always a non-delete rewrite:
//     namespace bindings materialize into project.yaml and ontology versions
//     into both project.yaml and pletka.mod, so scopedRewrite rewrites both
//     files for this ref. Without this case these entries produced no refs at
//     all, so the change_set drained as a noop with no commit (see
//     TestClosureNamespaceBindingMapsToProjectManifest /
//     TestClosureProjectOntologyVersionMapsToProjectManifest).
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
		case "model", "collection":
			add(e.EntityType, e.EntityID, e.Operation == "delete")
		case "category":
			del := e.Operation == "delete"
			id := e.EntityID
			if del {
				if semanticID, ok := categorySemanticIDFromPayload(e.PreviousPayload); ok {
					id = semanticID
				} else {
					m.logger.Warn("category delete: previous_payload missing/unparseable semantic_id, falling back to raw entity id; reconcile will heal any drift",
						"change_set_id", cs.ID, "entity_id", e.EntityID)
				}
			}
			add("category", id, del)
		case "namespace_binding", "project-ontology-version":
			add("project_manifest", cs.ProjectID, false)
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
			owner, ok, err := m.overrideOwner(ctx, cs.ProjectID, e.EntityID)
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
// (entity_type ""). Scoped to projectID — a cross-project override id (which
// should never happen) is treated the same as a missing row. ok is false
// when the override row doesn't exist in this project — the caller skips
// the entry (see closure's comment on that case).
func (m *Materializer) overrideOwner(ctx context.Context, projectID, overrideID string) (ScopedRef, bool, error) {
	id, err := strconv.ParseInt(overrideID, 10, 64)
	if err != nil {
		return ScopedRef{}, false, fmt.Errorf("parse override id %q: %w", overrideID, err)
	}
	row, err := m.queries.WeaveClosureOverrideOwner(ctx, sqlcgen.WeaveClosureOverrideOwnerParams{
		ID:        id,
		ProjectID: projectID,
	})
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

// categorySemanticIDFromPayload extracts semantic_id from a category
// change_log entry's previous_payload (the JSON-marshalled domain.Category
// captured at delete time — see pkg/weave/category/service.go's
// marshalCategory). ok is false when payload is empty, unparseable, or the
// field is blank, so the caller can fall back to the raw (ULID) entity id.
func categorySemanticIDFromPayload(payload []byte) (string, bool) {
	if len(payload) == 0 {
		return "", false
	}
	var v struct {
		SemanticID string `json:"semantic_id"`
	}
	if err := json.Unmarshal(payload, &v); err != nil {
		return "", false
	}
	if v.SemanticID == "" {
		return "", false
	}
	return v.SemanticID, true
}
