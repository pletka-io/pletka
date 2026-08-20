package gitmaterializer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/canonical"
)

// adoptedIDs groups the IDs of explicitly adopted upstream entities that
// should be materialized into the project's working tree as vendored
// read-only content. Inherited-only availability is represented through
// project.yaml + the inheritance graph, not copied as local entity files.
type adoptedIDs struct {
	fieldIDs      map[string]string // source entity semantic id -> source project id
	modelIDs      map[string]string // source entity semantic id -> source project id
	collectionIDs map[string]string // source entity semantic id -> source project id
}

func newAdoptedIDs() adoptedIDs {
	return adoptedIDs{
		fieldIDs:      make(map[string]string),
		modelIDs:      make(map[string]string),
		collectionIDs: make(map[string]string),
	}
}

// collectAdoptedIDs walks explicit project adoptions and gathers the upstream
// entities that should be materialized as vendored adopted content. Category
// copies are intentionally excluded here because they are already local rows
// written by writeCategories.
func (m *Materializer) collectAdoptedIDs(ctx context.Context, projectID string) (adoptedIDs, error) {
	adoptions, err := m.loadProjectAdoptions(ctx, projectID)
	if err != nil {
		return newAdoptedIDs(), fmt.Errorf("list project adoptions: %w", err)
	}
	return adoptedIDsFromAdoptions(adoptions, projectID), nil
}

func adoptedIDsFromAdoptions(adoptions []domain.Adoption, projectID string) adoptedIDs {
	ids := newAdoptedIDs()
	for _, adoption := range adoptions {
		sourceProjectID := strings.TrimSpace(adoption.SourceProjectID)
		sourceEntityID := strings.TrimSpace(adoption.SourceEntityID)
		if sourceProjectID == "" || sourceEntityID == "" || sourceProjectID == projectID {
			continue
		}
		switch adoption.EntityType {
		case "category":
			continue
		case "field":
			ids.fieldIDs[sourceEntityID] = sourceProjectID
		case "model":
			ids.modelIDs[sourceEntityID] = sourceProjectID
		case "collection":
			ids.collectionIDs[sourceEntityID] = sourceProjectID
		}
	}
	return ids
}

// writeAdoptedEntities materializes every referenced entity that does not
// belong to the current project, annotated with _adopted_from.
func (m *Materializer) writeAdoptedEntities(ctx context.Context, workDir, projectID string) error {
	ids, err := m.collectAdoptedIDs(ctx, projectID)
	if err != nil {
		return err
	}

	// Fields (and their base overrides when present).
	for fieldID, ownerProjectID := range ids.fieldIDs {
		row, err := m.queries.WeaveGetFieldByID(ctx, fieldID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return fmt.Errorf("get adopted field %s: %w", fieldID, err)
		}
		if row.ProjectID == projectID {
			continue
		}

		f := rowToField(row)
		payload, err := canonical.Field(f)
		if err != nil {
			return fmt.Errorf("encode adopted field %s: %w", fieldID, err)
		}
		annotated, err := addAdoptionMarker(payload, ownerProjectID)
		if err != nil {
			return err
		}
		path := domain.FilePath(domain.PathSpec{EntityType: "field", EntityID: fieldID})
		if err := writeEntityFile(workDir, path, annotated); err != nil {
			return err
		}

		if err := m.writeAdoptedBaseOverride(ctx, workDir, fieldID, ownerProjectID); err != nil {
			return err
		}
	}

	// Models.
	for modelID, ownerProjectID := range ids.modelIDs {
		if _, err := m.writeAdoptedModel(ctx, workDir, projectID, modelID, ownerProjectID); err != nil {
			return err
		}
	}

	// Collections.
	for colID, ownerProjectID := range ids.collectionIDs {
		if _, err := m.writeAdoptedCollection(ctx, workDir, projectID, colID, ownerProjectID); err != nil {
			return err
		}
	}

	return nil
}

func (m *Materializer) writeAdoptedModel(ctx context.Context, workDir, projectID, modelID, ownerProjectID string) (bool, error) {
	row, err := m.queries.WeaveGetModelByID(ctx, modelID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return true, nil
		}
		return false, fmt.Errorf("get adopted model %s: %w", modelID, err)
	}
	if row.ProjectID == projectID {
		return false, nil
	}

	mod := rowToModel(row)
	payload, err := canonical.Model(mod)
	if err != nil {
		return false, fmt.Errorf("encode adopted model %s: %w", modelID, err)
	}
	annotated, err := addAdoptionMarker(payload, ownerProjectID)
	if err != nil {
		return false, err
	}
	path := domain.FilePath(domain.PathSpec{EntityType: entityTypeModel, EntityID: modelID})
	return false, writeEntityFile(workDir, path, annotated)
}

func (m *Materializer) writeAdoptedCollection(ctx context.Context, workDir, projectID, collectionID, ownerProjectID string) (bool, error) {
	row, err := m.queries.WeaveGetCollectionByID(ctx, collectionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return true, nil
		}
		return false, fmt.Errorf("get adopted collection %s: %w", collectionID, err)
	}
	if row.ProjectID == projectID {
		return false, nil
	}

	col := rowToCollection(row)
	payload, err := canonical.Collection(col)
	if err != nil {
		return false, fmt.Errorf("encode adopted collection %s: %w", collectionID, err)
	}
	annotated, err := addAdoptionMarker(payload, ownerProjectID)
	if err != nil {
		return false, err
	}
	path := domain.FilePath(domain.PathSpec{EntityType: entityTypeCollection, EntityID: collectionID})
	return false, writeEntityFile(workDir, path, annotated)
}

// writeAdoptedBaseOverride writes the base override for an adopted field,
// looked up in the owning project. Silently skips if the owning project
// does not define one.
func (m *Materializer) writeAdoptedBaseOverride(
	ctx context.Context,
	workDir, fieldID, ownerProjectID string,
) error {
	row, err := m.queries.WeaveGetBaseOverride(ctx, sqlcgen.WeaveGetBaseOverrideParams{
		FieldID:   fieldID,
		ProjectID: ownerProjectID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("get adopted base override for %s: %w", fieldID, err)
	}

	o := rowToOverride(row)

	refRows, err := m.queries.WeaveListOverrideRefs(ctx, o.ID)
	if err != nil {
		return fmt.Errorf("list refs for adopted base override %d: %w", o.ID, err)
	}
	refs := make([]domain.OverrideRef, 0, len(refRows))
	for _, rr := range refRows {
		refs = append(refs, rowToOverrideRef(rr))
	}

	payload, err := canonical.Override(o, refs)
	if err != nil {
		return fmt.Errorf("encode adopted base override %d: %w", o.ID, err)
	}
	annotated, err := addAdoptionMarker(payload, ownerProjectID)
	if err != nil {
		return err
	}

	path := domain.FilePath(domain.PathSpec{
		EntityType: "base_override",
		EntityID:   fmt.Sprintf("%d", o.ID),
		FieldID:    fieldID,
	})
	return writeEntityFile(workDir, path, annotated)
}

// addAdoptionMarker decodes canonical YAML, inserts an "_adopted_from"
// key, and re-encodes. The underscore prefix marks the field as metadata
// (skipped on import) and sorts first under canonical alphabetical order.
func addAdoptionMarker(payload []byte, ownerProjectID string) ([]byte, error) {
	var m map[string]any
	if err := yaml.Unmarshal(payload, &m); err != nil {
		return nil, fmt.Errorf("parse canonical yaml: %w", err)
	}
	if m == nil {
		m = make(map[string]any)
	}
	m["_adopted_from"] = ownerProjectID
	return canonical.Encode(m)
}
