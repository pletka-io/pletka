package gitmaterializer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/canonical"
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
	if m.baseDir != "" {
		unlock, err := acquireProjectLock(m.baseDir, projectID)
		if err != nil {
			return fmt.Errorf("acquire project lock %s: %w", projectID, err)
		}
		defer unlock()
	}
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
		if err := m.writeOneCategory(ctx, workDir, row.ID); err != nil {
			return err
		}
	}
	return nil
}

// writeConceptLists materializes concept-lists/<SEMID>.yaml for every concept
// list the project owns. Entries and broader edges stay in the database.
func (m *Materializer) writeConceptLists(ctx context.Context, workDir, projectID string) error {
	rows, err := m.queries.WeaveListConceptListsForTree(ctx, projectID)
	if err != nil {
		return fmt.Errorf("list concept lists: %w", err)
	}
	for _, row := range rows {
		list := rowToConceptList(row)
		payload, err := canonical.ConceptList(list, "")
		if err != nil {
			return fmt.Errorf("encode concept list %s: %w", list.ID, err)
		}
		path := domain.FilePath(domain.PathSpec{
			EntityType: entityTypeConceptList,
			EntityID:   identifierFor(list.SemanticID, list.ID),
		})
		if err := writeEntityFile(workDir, path, payload); err != nil {
			return err
		}
	}
	return nil
}

// writeOneCategory materializes a single category's file. Shared by
// writeCategories (loop) and the scoped rewrite so both paths are
// byte-identical.
func (m *Materializer) writeOneCategory(ctx context.Context, workDir, categoryID string) error {
	row, err := m.queries.WeaveGetCategoryByID(ctx, categoryID)
	if err != nil {
		return fmt.Errorf("get category %s: %w", categoryID, err)
	}
	cat := rowToCategory(row)
	payload, err := canonical.Category(cat)
	if err != nil {
		return fmt.Errorf("encode category %s: %w", cat.ID, err)
	}
	path := domain.FilePath(domain.PathSpec{
		EntityType: entityTypeCategory,
		EntityID:   identifierFor(cat.SemanticID, cat.ID),
	})
	return writeEntityFile(workDir, path, payload)
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
		if err := m.writeOneField(ctx, workDir, projectID, row.ID); err != nil {
			return err
		}
	}
	return nil
}

// writeOneField materializes a single field's file and its base override
// (one row max per (field, project)). Shared by writeFields (loop) and the
// scoped rewrite so both paths are byte-identical.
func (m *Materializer) writeOneField(ctx context.Context, workDir, projectID, fieldID string) error {
	row, err := m.queries.WeaveGetFieldByID(ctx, fieldID)
	if err != nil {
		return fmt.Errorf("get field %s: %w", fieldID, err)
	}
	f := rowToField(row)
	payload, err := canonical.Field(f)
	if err != nil {
		return fmt.Errorf("encode field %s: %w", f.ID, err)
	}
	fieldKey := identifierFor(f.SemanticID, f.ID)
	path := domain.FilePath(domain.PathSpec{
		EntityType: entityTypeField,
		EntityID:   fieldKey,
	})
	if err := writeEntityFile(workDir, path, payload); err != nil {
		return err
	}

	basePath := domain.FilePath(domain.PathSpec{
		EntityType: entityTypeBaseOverride,
		FieldID:    fieldKey,
	})
	if err := os.Remove(filepath.Join(workDir, basePath)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale base override for field %s: %w", f.ID, err)
	}

	base, err := m.queries.WeaveGetBaseOverride(ctx, sqlcgen.WeaveGetBaseOverrideParams{
		FieldID:   f.ID,
		ProjectID: projectID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("get base override for field %s: %w", f.ID, err)
	}
	return m.writeOverride(ctx, workDir, base, "base_override", fieldKey, "")
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
		if err := m.writeOneModel(ctx, workDir, row.ID); err != nil {
			return err
		}
	}
	return nil
}

// writeOneModel materializes a single model's file. Shared by writeModels
// (loop) and the scoped rewrite so both paths are byte-identical.
func (m *Materializer) writeOneModel(ctx context.Context, workDir, modelID string) error {
	row, err := m.queries.WeaveGetModelByID(ctx, modelID)
	if err != nil {
		return fmt.Errorf("get model %s: %w", modelID, err)
	}
	mod := rowToModel(row)
	payload, err := canonical.Model(mod)
	if err != nil {
		return fmt.Errorf("encode model %s: %w", mod.ID, err)
	}
	modelKey := identifierFor(mod.SemanticID, mod.ID)
	path := domain.FilePath(domain.PathSpec{
		EntityType: entityTypeModel,
		EntityID:   modelKey,
	})
	return writeEntityFile(workDir, path, payload)
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
		if err := m.writeOneCollection(ctx, workDir, row.ID); err != nil {
			return err
		}
	}
	return nil
}

// writeOneCollection materializes a single collection's file. Shared by
// writeCollections (loop) and the scoped rewrite so both paths are
// byte-identical. Uses rowToCollectionByID rather than the older
// rowToCollection (which predates default_category_id being added to the
// list output and doesn't populate it) so the encoded bytes match
// writeCollections exactly.
func (m *Materializer) writeOneCollection(ctx context.Context, workDir, collectionID string) error {
	row, err := m.queries.WeaveGetCollectionByID(ctx, collectionID)
	if err != nil {
		return fmt.Errorf("get collection %s: %w", collectionID, err)
	}
	col := rowToCollectionByID(row)
	payload, err := canonical.Collection(col)
	if err != nil {
		return fmt.Errorf("encode collection %s: %w", col.ID, err)
	}
	colKey := identifierFor(col.SemanticID, col.ID)
	path := domain.FilePath(domain.PathSpec{
		EntityType: entityTypeCollection,
		EntityID:   colKey,
	})
	return writeEntityFile(workDir, path, payload)
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
	files := make([]pendingOverride, 0, len(rows))
	for _, row := range rows {
		f, err := m.overrideFile(ctx, row, pathType, row.FieldID, row.EntityID)
		if err != nil {
			return err
		}
		files = append(files, f)
	}
	return writeOverrideFiles(workDir, files)
}

// writeOverride materializes a single override. Only the base override goes
// through here; scoped overrides are written per owner set via
// writeOverrideFiles so duplicate placements get stable names.
func (m *Materializer) writeOverride(
	ctx context.Context,
	workDir string,
	row sqlcgen.WeaveFieldOverride,
	pathType, fieldKey, ownerKey string,
) error {
	f, err := m.overrideFile(ctx, row, pathType, fieldKey, ownerKey)
	if err != nil {
		return err
	}
	return writeOverrideFiles(workDir, []pendingOverride{f})
}

// overrideFile encodes one override (base, model, or collection) and the path
// it materializes to. For base overrides, pathType is "base_override" and
// ownerKey is "". For scoped overrides, pathType is
// "model_override"/"collection_override" and ownerKey is the model or
// collection identifier used in the file path.
func (m *Materializer) overrideFile(
	ctx context.Context,
	row sqlcgen.WeaveFieldOverride,
	pathType, fieldKey, ownerKey string,
) (pendingOverride, error) {
	o := rowToOverride(row)

	if o.CategoryID != "" {
		catIdentifier, err := m.categoryIdentifierFor(ctx, o.CategoryID)
		if err != nil {
			return pendingOverride{}, fmt.Errorf("resolve category identifier for override %d: %w", o.ID, err)
		}
		o.CategoryID = catIdentifier
	}

	refRows, err := m.queries.WeaveListOverrideRefs(ctx, o.ID)
	if err != nil {
		return pendingOverride{}, fmt.Errorf("list override refs for %d: %w", o.ID, err)
	}
	refs := make([]domain.OverrideRef, 0, len(refRows))
	for _, rr := range refRows {
		refs = append(refs, rowToOverrideRef(rr))
	}

	payload, err := canonical.Override(o, refs)
	if err != nil {
		return pendingOverride{}, fmt.Errorf("encode override %d: %w", o.ID, err)
	}
	return newPendingOverride(o, payload, pathType, fieldKey, ownerKey), nil
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
