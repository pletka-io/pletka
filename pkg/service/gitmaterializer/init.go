package gitmaterializer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/canonical"
	"github.com/jackc/pgx/v5"
)

// listPageSize caps per-call rows when listing project entities.
// Large enough to avoid repeated roundtrips during bootstrap.
const listPageSize int32 = 10_000

// InitProject writes the complete state of a project to its working tree
// as the initial git state. Does NOT consume the change_log — this is a
// one-shot bootstrap that produces a single "Initial import" commit.
func (m *Materializer) InitProject(ctx context.Context, projectID string) error {
	return m.InitProjectWithOptions(ctx, projectID, InitProjectOptions{})
}

func (m *Materializer) InitProjectWithOptions(ctx context.Context, projectID string, opts InitProjectOptions) error {
	workDir := filepath.Join(m.baseDir, projectID)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("create work dir: %w", err)
	}

	git := newGitRunner(workDir, m.logger)
	if _, err := os.Stat(filepath.Join(workDir, ".git")); os.IsNotExist(err) {
		if err := git.Init(ctx); err != nil {
			return fmt.Errorf("git init: %w", err)
		}
	}

	if err := m.writeProjectTree(ctx, workDir, projectID); err != nil {
		return err
	}
	if opts.SelfContained {
		if err := m.writeVendorSnapshot(ctx, workDir, projectID); err != nil {
			return err
		}
	}
	if err := git.AddAll(ctx); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	msg := fmt.Sprintf("Initial import of project %s\n\nImported at %s",
		projectID, time.Now().UTC().Format(time.RFC3339))
	sha, err := git.Commit(ctx, msg, "pletka-system", "system@pletka.local")
	if err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	m.logger.Info("project initialized in git",
		"project_id", projectID, "commit_sha", sha, "work_dir", workDir)
	return nil
}

func writeEntityFile(workDir, relPath string, payload []byte) error {
	abs := filepath.Join(workDir, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(relPath), err)
	}
	if err := os.WriteFile(abs, payload, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", relPath, err)
	}
	return nil
}

func (m *Materializer) writeCategories(ctx context.Context, workDir, projectID string) error {
	rows, err := m.queries.WeaveListCategories(ctx, projectID)
	if err != nil {
		return fmt.Errorf("list categories: %w", err)
	}
	for _, row := range rows {
		cat := rowToCategory(row)
		payload, err := canonical.Category(cat)
		if err != nil {
			return fmt.Errorf("encode category %s: %w", cat.ID, err)
		}
		path := domain.FilePath(domain.PathSpec{
			EntityType: "category",
			EntityID:   identifierFor(cat.SemanticID, cat.ID),
		})
		if err := writeEntityFile(workDir, path, payload); err != nil {
			return err
		}
	}
	return nil
}

func (m *Materializer) writeFields(ctx context.Context, workDir, projectID string) error {
	rows, err := m.queries.WeaveListFields(ctx, sqlcgen.WeaveListFieldsParams{
		ProjectID:    projectID,
		Search:       "",
		SortBy:       "",
		SortDesc:     false,
		ResultLimit:  listPageSize,
		ResultOffset: 0,
	})
	if err != nil {
		return fmt.Errorf("list fields: %w", err)
	}
	if int32(len(rows)) >= listPageSize {
		return fmt.Errorf("project %s has %d+ fields, exceeding page size %d: results would be truncated", projectID, len(rows), listPageSize)
	}
	for _, row := range rows {
		f := rowToFieldList(row)
		payload, err := canonical.Field(f)
		if err != nil {
			return fmt.Errorf("encode field %s: %w", f.ID, err)
		}
		fieldKey := identifierFor(f.SemanticID, f.ID)
		path := domain.FilePath(domain.PathSpec{
			EntityType: "field",
			EntityID:   fieldKey,
		})
		if err := writeEntityFile(workDir, path, payload); err != nil {
			return err
		}

		// Base override (one row max per (field, project)).
		base, err := m.queries.WeaveGetBaseOverride(ctx, sqlcgen.WeaveGetBaseOverrideParams{
			FieldID:   f.ID,
			ProjectID: projectID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return fmt.Errorf("get base override for field %s: %w", f.ID, err)
		}
		if err := m.writeOverride(ctx, workDir, base, "base_override", fieldKey, ""); err != nil {
			return err
		}
	}
	return nil
}

func (m *Materializer) writeModels(ctx context.Context, workDir, projectID string) error {
	rows, err := m.queries.WeaveListModels(ctx, sqlcgen.WeaveListModelsParams{
		ProjectID:    projectID,
		Search:       "",
		SortBy:       "",
		SortDesc:     false,
		ResultLimit:  listPageSize,
		ResultOffset: 0,
	})
	if err != nil {
		return fmt.Errorf("list models: %w", err)
	}
	if int32(len(rows)) >= listPageSize {
		return fmt.Errorf("project %s has %d+ models, exceeding page size %d: results would be truncated", projectID, len(rows), listPageSize)
	}
	for _, row := range rows {
		mod := rowToModelList(row)
		payload, err := canonical.Model(mod)
		if err != nil {
			return fmt.Errorf("encode model %s: %w", mod.ID, err)
		}
		modelKey := identifierFor(mod.SemanticID, mod.ID)
		path := domain.FilePath(domain.PathSpec{
			EntityType: "model",
			EntityID:   modelKey,
		})
		if err := writeEntityFile(workDir, path, payload); err != nil {
			return err
		}
	}
	return nil
}

func (m *Materializer) writeCollections(ctx context.Context, workDir, projectID string) error {
	rows, err := m.queries.WeaveListCollections(ctx, sqlcgen.WeaveListCollectionsParams{
		ProjectID:    projectID,
		Search:       "",
		SortBy:       "",
		SortDesc:     false,
		ResultLimit:  listPageSize,
		ResultOffset: 0,
	})
	if err != nil {
		return fmt.Errorf("list collections: %w", err)
	}
	if int32(len(rows)) >= listPageSize {
		return fmt.Errorf("project %s has %d+ collections, exceeding page size %d: results would be truncated", projectID, len(rows), listPageSize)
	}
	for _, row := range rows {
		col := rowToCollectionList(row)
		payload, err := canonical.Collection(col)
		if err != nil {
			return fmt.Errorf("encode collection %s: %w", col.ID, err)
		}
		colKey := identifierFor(col.SemanticID, col.ID)
		path := domain.FilePath(domain.PathSpec{
			EntityType: "collection",
			EntityID:   colKey,
		})
		if err := writeEntityFile(workDir, path, payload); err != nil {
			return err
		}
	}
	return nil
}

// writeScopedOverridesByProject materializes every override in the project
// for a given entity type (e.g. "model" or "collection"), regardless of
// whether the owning entity is defined in this project. Owners adopted from
// another project produce orphan directories containing only overrides — a
// meaningful signal that the entity definition comes from a vendored source.
func (m *Materializer) writeScopedOverridesByProject(
	ctx context.Context,
	workDir, projectID, entityType string,
) error {
	rows, err := m.queries.WeaveListOverridesByProjectAndType(ctx, sqlcgen.WeaveListOverridesByProjectAndTypeParams{
		ProjectID:  projectID,
		EntityType: entityType,
	})
	if err != nil {
		return fmt.Errorf("list %s overrides for project %s: %w", entityType, projectID, err)
	}
	pathType := entityType + "_override"
	for _, row := range rows {
		if err := m.writeOverride(ctx, workDir, row, pathType, row.FieldID, row.EntityID); err != nil {
			return err
		}
	}
	return nil
}

// writeOverride materializes a single override (base, model, or collection).
// For base overrides, pathType is "base_override" and ownerKey is "".
// For scoped overrides, pathType is "model_override"/"collection_override" and
// ownerKey is the model or collection identifier used in the file path.
func (m *Materializer) writeOverride(
	ctx context.Context,
	workDir string,
	row sqlcgen.WeaveFieldOverride,
	pathType, fieldKey, ownerKey string,
) error {
	o := rowToOverride(row)

	if o.CategoryID != "" {
		catIdentifier, err := m.categoryIdentifierFor(ctx, o.CategoryID)
		if err != nil {
			return fmt.Errorf("resolve category identifier for override %d: %w", o.ID, err)
		}
		o.CategoryID = catIdentifier
	}

	refRows, err := m.queries.WeaveListOverrideRefs(ctx, o.ID)
	if err != nil {
		return fmt.Errorf("list override refs for %d: %w", o.ID, err)
	}
	refs := make([]domain.OverrideRef, 0, len(refRows))
	for _, rr := range refRows {
		refs = append(refs, rowToOverrideRef(rr))
	}

	payload, err := canonical.Override(o, refs)
	if err != nil {
		return fmt.Errorf("encode override %d: %w", o.ID, err)
	}

	spec := domain.PathSpec{
		EntityType: pathType,
		EntityID:   fmt.Sprintf("%d", o.ID),
		FieldID:    fieldKey,
		OwnerID:    ownerKey,
	}
	switch pathType {
	case "model_override":
		spec.OwnerType = "model"
	case "collection_override":
		spec.OwnerType = "collection"
	}

	path := domain.FilePath(spec)
	return writeEntityFile(workDir, path, payload)
}

// identifierFor returns the semantic ID when present, falling back to the ULID.
// This matches how FilePath consumers expect EntityID to be populated.
func identifierFor(semanticID, id string) string {
	if semanticID != "" {
		return semanticID
	}
	return id
}

// categoryIdentifierFor returns the portable identifier (SemanticID, falling
// back to the raw ULID) for the category with the given ID, so canonical
// override files reference categories the same portable way field/model/
// collection files already do. Categories don't follow the ID==SemanticID
// convention fields/models/collections use (category.Service.Create always
// mints a fresh ULID for ID), so the raw ID captured on the override row is
// not itself portable across a restore that re-creates categories.
func (m *Materializer) categoryIdentifierFor(ctx context.Context, categoryID string) (string, error) {
	cat, err := m.queries.WeaveGetCategoryByID(ctx, categoryID)
	if err != nil {
		return "", fmt.Errorf("get category %s: %w", categoryID, err)
	}
	return identifierFor(derefStr(cat.SemanticID), cat.ID), nil
}
