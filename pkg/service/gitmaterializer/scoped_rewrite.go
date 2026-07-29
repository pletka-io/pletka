package gitmaterializer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

// scopedRewrite applies refs to workDir: delete refs remove the entity's
// file(s); non-delete refs are (re)written from current DB via the SAME
// single-entity writers the whole-tree path uses. For model/collection refs
// it also regenerates their override subtree so embedded resolved refs stay
// current.
//
// A non-delete ref for an entity that no longer exists in the DB (e.g. a
// cross-change_set update-then-delete race, or the documented dedup
// asymmetry in closure()) is skipped rather than failing the whole rewrite:
// the entity's own delete ref (from its own change_set) or the reconcile
// backstop removes its files. Treating this as fatal would wedge the
// change_set in a permanent retry loop (see
// TestProcessChangeSet_ConcurrentlyDeletedRefDoesNotWedgeChangeSet).
func (m *Materializer) scopedRewrite(ctx context.Context, workDir, projectID string, refs []ScopedRef) error {
	for _, r := range refs {
		if err := m.applyRef(ctx, workDir, projectID, r); err != nil {
			return err
		}
	}
	return nil
}

// applyRef applies one ref to the work tree: deletes the entity's file(s),
// or (re)writes it from current DB state and — for model/collection refs —
// regenerates its override subtree.
func (m *Materializer) applyRef(ctx context.Context, workDir, projectID string, r ScopedRef) error {
	if r.Delete {
		for _, rel := range deletePathsFor(r.EntityType, r.EntityID) {
			if err := os.RemoveAll(filepath.Join(workDir, rel)); err != nil {
				return fmt.Errorf("remove %s: %w", rel, err)
			}
		}
		return nil
	}
	if r.EntityType == "project_manifest" {
		// namespace_binding entries materialize into project.yaml;
		// project-ontology-version entries materialize into both
		// project.yaml and pletka.mod. Both files are always
		// regenerated for this ref rather than branching on which
		// change_log entity type triggered it (closure() never emits
		// a delete for this ref — see closure()'s doc comment).
		if err := m.writeProjectManifest(ctx, workDir, projectID); err != nil {
			return err
		}
		return m.writePletkaMod(ctx, workDir, projectID)
	}
	skipped, err := m.writeScopedEntity(ctx, workDir, projectID, r)
	if err != nil {
		return err
	}
	if skipped {
		return nil
	}
	if r.EntityType == entityTypeModel || r.EntityType == entityTypeCollection {
		return m.writeOverridesForOwner(ctx, workDir, projectID, r.EntityType, r.EntityID)
	}
	return nil
}

// writeScopedEntity writes r's single-entity file via the same writers the
// whole-tree path uses. skipped is true when the entity was concurrently
// deleted (pgx.ErrNoRows), matching scopedRewrite's documented skip
// semantics for that race.
func (m *Materializer) writeScopedEntity(ctx context.Context, workDir, projectID string, r ScopedRef) (bool, error) {
	var writeErr error
	switch r.EntityType {
	case entityTypeField:
		writeErr = m.writeOneField(ctx, workDir, projectID, r.EntityID)
	case entityTypeModel:
		writeErr = m.writeOneModel(ctx, workDir, r.EntityID)
	case entityTypeCollection:
		writeErr = m.writeOneCollection(ctx, workDir, r.EntityID)
	case entityTypeCategory:
		writeErr = m.writeOneCategory(ctx, workDir, r.EntityID)
	default:
		return false, nil
	}
	if writeErr == nil {
		return false, nil
	}
	if m.skipConcurrentlyDeleted(writeErr, r.EntityType, r.EntityID) {
		return true, nil
	}
	return false, writeErr
}

// skipConcurrentlyDeleted reports whether err is a pgx.ErrNoRows from a
// non-delete single-entity writer, meaning the entity was deleted from the
// DB after closure() ran (concurrently with, or by, another change_set).
// Logs at Info and returns true so the caller skips the ref instead of
// failing the whole rewrite.
func (m *Materializer) skipConcurrentlyDeleted(err error, entityType, entityID string) bool {
	if !errors.Is(err, pgx.ErrNoRows) {
		return false
	}
	m.logger.Info("scopedRewrite: entity concurrently deleted, skipping non-delete ref",
		"entity_type", entityType, "entity_id", entityID)
	return true
}

// writeOverridesForOwner regenerates one model/collection owner's
// overrides/ subtree from current DB state. The subtree is cleared first so
// placements removed since the last materialization drop out of the tree
// (mirrors resetWorkTree's intent, scoped to a single owner), then every
// current override is written via the same writeOverride helper the
// whole-tree path (writeScopedOverridesByProject) uses, so bytes stay
// identical.
func (m *Materializer) writeOverridesForOwner(ctx context.Context, workDir, projectID, ownerType, ownerID string) error {
	ownerDir := path.Dir(domain.FilePath(domain.PathSpec{EntityType: ownerType, EntityID: ownerID}))
	overridesDir := filepath.Join(workDir, ownerDir, "overrides")
	if err := os.RemoveAll(overridesDir); err != nil {
		return fmt.Errorf("clear overrides dir for %s %s: %w", ownerType, ownerID, err)
	}

	rows, err := m.queries.WeaveListOverridesForOwner(ctx, sqlcgen.WeaveListOverridesForOwnerParams{
		ProjectID:  projectID,
		EntityType: ownerType,
		EntityID:   ownerID,
	})
	if err != nil {
		return fmt.Errorf("list %s overrides for owner %s: %w", ownerType, ownerID, err)
	}
	pathType := ownerType + "_override"
	for _, row := range rows {
		if err := m.writeOverride(ctx, workDir, row, pathType, row.FieldID, row.EntityID); err != nil {
			return err
		}
	}
	return nil
}
