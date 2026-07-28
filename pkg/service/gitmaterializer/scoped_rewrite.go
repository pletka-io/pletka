package gitmaterializer

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

// scopedRewrite applies refs to workDir: delete refs remove the entity's
// file(s); non-delete refs are (re)written from current DB via the SAME
// single-entity writers the whole-tree path uses. For model/collection refs
// it also regenerates their override subtree so embedded resolved refs stay
// current.
func (m *Materializer) scopedRewrite(ctx context.Context, workDir, projectID string, refs []ScopedRef) error {
	for _, r := range refs {
		if r.Delete {
			for _, rel := range deletePathsFor(r.EntityType, r.EntityID) {
				if err := os.RemoveAll(filepath.Join(workDir, rel)); err != nil {
					return fmt.Errorf("remove %s: %w", rel, err)
				}
			}
			continue
		}
		switch r.EntityType {
		case "field":
			if err := m.writeOneField(ctx, workDir, projectID, r.EntityID); err != nil {
				return err
			}
		case "model":
			if err := m.writeOneModel(ctx, workDir, projectID, r.EntityID); err != nil {
				return err
			}
			if err := m.writeOverridesForOwner(ctx, workDir, projectID, "model", r.EntityID); err != nil {
				return err
			}
		case "collection":
			if err := m.writeOneCollection(ctx, workDir, projectID, r.EntityID); err != nil {
				return err
			}
			if err := m.writeOverridesForOwner(ctx, workDir, projectID, "collection", r.EntityID); err != nil {
				return err
			}
		case "category":
			if err := m.writeOneCategory(ctx, workDir, projectID, r.EntityID); err != nil {
				return err
			}
		}
	}
	return nil
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
