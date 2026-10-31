package readsession

import (
	"context"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/readsql/draft"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

type draftContentReader struct {
	queries   draft.Querier
	projectID string
}

// ContentAdoptions returns one deterministic page of lossless owned records.
func (r *draftContentReader) ContentAdoptions(ctx context.Context, page Page) ([]projectsnapshot.AdoptionRecord, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ContentAdoptions(ctx, draft.ContentAdoptionsParams{SelectedCategory: page.IDs("category"), Restricted: page.Restricted(), SelectedModel: page.IDs("model"), SelectedCollection: page.IDs("collection"), SelectedField: page.IDs("field"), ProjectID: r.projectID, PageLimit: page.Limit, PageOffset: page.Offset})
	if err != nil {
		return nil, fmt.Errorf("read adoptions content: %w", err)
	}
	out := make([]projectsnapshot.AdoptionRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectsnapshot.AdoptionRecord{
			ProjectID:         row.ProjectID,
			ContextEntityType: row.ContextEntityType,
			ContextEntityID:   row.ContextEntityID,
			EntityType:        row.EntityType,
			SourceProjectID:   row.SourceProjectID,
			SourceEntityID:    row.SourceEntityID,
			SourceVersion:     row.SourceVersion,
			AdoptedAt:         projectsnapshot.Timestamp(row.AdoptedAt),
			CreatedByID:       row.CreatedByID,
			VersionNumber:     row.VersionNumber,
		})
	}
	return out, nil
}

// ContentCategories returns one deterministic page of lossless owned records.
func (r *draftContentReader) ContentCategories(ctx context.Context, page Page) ([]projectsnapshot.CategoryRecord, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ContentCategories(ctx, draft.ContentCategoriesParams{Restricted: page.Restricted(), SelectedCategory: page.IDs("category"), ProjectID: r.projectID, PageLimit: page.Limit, PageOffset: page.Offset})
	if err != nil {
		return nil, fmt.Errorf("read categories content: %w", err)
	}
	out := make([]projectsnapshot.CategoryRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectsnapshot.CategoryRecord{
			ID:             row.ID,
			CreatedAt:      projectsnapshot.Timestamp(row.CreatedAt),
			UpdatedAt:      projectsnapshot.Timestamp(row.UpdatedAt),
			SemanticID:     row.SemanticID,
			SystemName:     row.SystemName,
			UIName:         row.UiName,
			Description:    row.Description,
			Status:         row.Status,
			ProjectID:      row.ProjectID,
			CanonicalOrder: row.CanonicalOrder,
			Deprecated:     row.Deprecated,
			VersionNumber:  row.VersionNumber,
		})
	}
	return out, nil
}

// ContentForks returns one deterministic page of lossless owned records.
func (r *draftContentReader) ContentForks(ctx context.Context, page Page) ([]projectsnapshot.ForkRecord, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ContentForks(ctx, draft.ContentForksParams{Restricted: page.Restricted(), SelectedModel: page.IDs("model"), SelectedCollection: page.IDs("collection"), SelectedField: page.IDs("field"), ProjectID: r.projectID, PageLimit: page.Limit, PageOffset: page.Offset})
	if err != nil {
		return nil, fmt.Errorf("read forks content: %w", err)
	}
	out := make([]projectsnapshot.ForkRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectsnapshot.ForkRecord{
			ProjectID:       row.ProjectID,
			EntityType:      row.EntityType,
			ForkEntityID:    row.ForkEntityID,
			SourceProjectID: row.SourceProjectID,
			SourceEntityID:  row.SourceEntityID,
			SourceVersion:   row.SourceVersion,
			ForkedAt:        projectsnapshot.Timestamp(row.ForkedAt),
			CreatedByID:     row.CreatedByID,
			VersionNumber:   row.VersionNumber,
		})
	}
	return out, nil
}

// ContentFields returns one deterministic page of lossless owned records.
func (r *draftContentReader) ContentFields(ctx context.Context, page Page) ([]projectsnapshot.FieldRecord, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ContentFields(ctx, draft.ContentFieldsParams{Restricted: page.Restricted(), SelectedField: page.IDs("field"), ProjectID: r.projectID, PageLimit: page.Limit, PageOffset: page.Offset})
	if err != nil {
		return nil, fmt.Errorf("read fields content: %w", err)
	}
	out := make([]projectsnapshot.FieldRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectsnapshot.FieldRecord{
			ID:                row.ID,
			CreatedAt:         projectsnapshot.Timestamp(row.CreatedAt),
			UpdatedAt:         projectsnapshot.Timestamp(row.UpdatedAt),
			SemanticID:        row.SemanticID,
			SystemName:        row.SystemName,
			UIName:            row.UiName,
			Description:       row.Description,
			Status:            row.Status,
			ProjectID:         row.ProjectID,
			OntologyScope:     row.OntologyScope,
			OntologyPath:      row.OntologyPath,
			PathElements:      row.PathElements,
			ExpectedValueType: row.ExpectedValueType,
			Examples:          row.Examples,
			StagingID:         row.StagingID,
			Deprecated:        row.Deprecated,
			VersionNumber:     row.VersionNumber,
			SubfieldPaths:     row.SubfieldPaths,
		})
	}
	return out, nil
}

// ContentModels returns one deterministic page of lossless owned records.
func (r *draftContentReader) ContentModels(ctx context.Context, page Page) ([]projectsnapshot.ModelRecord, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ContentModels(ctx, draft.ContentModelsParams{Restricted: page.Restricted(), SelectedModel: page.IDs("model"), ProjectID: r.projectID, PageLimit: page.Limit, PageOffset: page.Offset})
	if err != nil {
		return nil, fmt.Errorf("read models content: %w", err)
	}
	out := make([]projectsnapshot.ModelRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectsnapshot.ModelRecord{
			ID:            row.ID,
			CreatedAt:     projectsnapshot.Timestamp(row.CreatedAt),
			UpdatedAt:     projectsnapshot.Timestamp(row.UpdatedAt),
			SystemName:    row.SystemName,
			UIName:        row.UiName,
			Description:   row.Description,
			Status:        row.Status,
			ProjectID:     row.ProjectID,
			OntologyScope: row.OntologyScope,
			StagingID:     row.StagingID,
			Deprecated:    row.Deprecated,
			VersionNumber: row.VersionNumber,
			ModelType:     row.ModelType,
		})
	}
	return out, nil
}

// ContentNamespaces returns one deterministic page of lossless owned records.
func (r *draftContentReader) ContentNamespaces(ctx context.Context, page Page) ([]projectsnapshot.NamespaceRecord, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ContentNamespaces(ctx, draft.ContentNamespacesParams{ProjectID: r.projectID, PageLimit: page.Limit, PageOffset: page.Offset})
	if err != nil {
		return nil, fmt.Errorf("read namespaces content: %w", err)
	}
	out := make([]projectsnapshot.NamespaceRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectsnapshot.NamespaceRecord{
			ReleaseProjectID: row.ReleaseProjectID,
			VersionNumber:    row.VersionNumber,
			ID:               row.ID,
			CreatedAt:        projectsnapshot.Timestamp(row.CreatedAt),
			UpdatedAt:        projectsnapshot.Timestamp(row.UpdatedAt),
			AirtableRaw:      row.AirtableRaw,
			AirtableContext:  row.AirtableContext,
			AirtableRef:      row.AirtableRef,
			SemanticID:       row.SemanticID,
			LongSemanticID:   row.LongSemanticID,
			SystemName:       row.SystemName,
			UIName:           row.UiName,
			Description:      row.Description,
			URI:              row.Uri,
			ProjectID:        row.ProjectID,
			Prefix:           row.Prefix,
			Namespace:        row.Namespace,
			Weight:           row.Weight,
			Source:           row.Source,
			OntologyID:       row.OntologyID,
		})
	}
	return out, nil
}

// ContentProjectActors returns one deterministic page of lossless owned records.
func (r *draftContentReader) ContentProjectActors(ctx context.Context, page Page) ([]projectsnapshot.ProjectActorRecord, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ContentProjectActors(ctx, draft.ContentProjectActorsParams{ProjectID: r.projectID, PageLimit: page.Limit, PageOffset: page.Offset})
	if err != nil {
		return nil, fmt.Errorf("read projectactors content: %w", err)
	}
	out := make([]projectsnapshot.ProjectActorRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectsnapshot.ProjectActorRecord{
			ProjectID:     row.ProjectID,
			ActorID:       row.ActorID,
			Role:          row.Role,
			CreatedAt:     projectsnapshot.Timestamp(row.CreatedAt),
			VersionNumber: row.VersionNumber,
		})
	}
	return out, nil
}

// ContentAttributions returns one deterministic page of lossless owned records.
func (r *draftContentReader) ContentAttributions(ctx context.Context, page Page) ([]projectsnapshot.AttributionRecord, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ContentAttributions(ctx, draft.ContentAttributionsParams{ProjectID: r.projectID, PageLimit: page.Limit, PageOffset: page.Offset})
	if err != nil {
		return nil, fmt.Errorf("read attributions content: %w", err)
	}
	out := make([]projectsnapshot.AttributionRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectsnapshot.AttributionRecord{
			ProjectID:     row.ProjectID,
			ActorID:       row.ActorID,
			Kind:          row.Kind,
			Position:      row.Position,
			Note:          row.Note,
			CreatedAt:     projectsnapshot.Timestamp(row.CreatedAt),
			VersionNumber: row.VersionNumber,
		})
	}
	return out, nil
}

// ContentInheritance returns one deterministic page of lossless owned records.
func (r *draftContentReader) ContentInheritance(ctx context.Context, page Page) ([]projectsnapshot.InheritanceRecord, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ContentInheritance(ctx, draft.ContentInheritanceParams{Restricted: page.Restricted(), ProjectID: r.projectID, PageLimit: page.Limit, PageOffset: page.Offset})
	if err != nil {
		return nil, fmt.Errorf("read inheritance content: %w", err)
	}
	out := make([]projectsnapshot.InheritanceRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectsnapshot.InheritanceRecord{
			ProjectID:       row.ProjectID,
			ParentProjectID: row.ParentProjectID,
			IsPrimary:       row.IsPrimary,
			CanonicalOrder:  row.CanonicalOrder,
			AdoptedAt:       projectsnapshot.Timestamp(row.AdoptedAt),
			VersionNumber:   row.VersionNumber,
			SourceMode:      row.SourceMode,
			SourceVersion:   row.SourceVersion,
		})
	}
	return out, nil
}

// ContentOntologyLinks returns one deterministic page of lossless owned records.
func (r *draftContentReader) ContentOntologyLinks(ctx context.Context, page Page) ([]projectsnapshot.OntologyLinkRecord, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ContentOntologyLinks(ctx, draft.ContentOntologyLinksParams{ProjectID: r.projectID, PageLimit: page.Limit, PageOffset: page.Offset})
	if err != nil {
		return nil, fmt.Errorf("read ontologylinks content: %w", err)
	}
	out := make([]projectsnapshot.OntologyLinkRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectsnapshot.OntologyLinkRecord{
			ProjectID:         row.ProjectID,
			OntologyVersionID: row.OntologyVersionID,
			AddedAt:           projectsnapshot.Timestamp(row.AddedAt),
			AddedByID:         row.AddedByID,
			IsPrimary:         row.IsPrimary,
			UsageNotes:        row.UsageNotes,
			VersionNumber:     row.VersionNumber,
		})
	}
	return out, nil
}

// ContentProjects returns one deterministic page of lossless owned records.
func (r *draftContentReader) ContentProjects(ctx context.Context, page Page) ([]projectsnapshot.ProjectRecord, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ContentProjects(ctx, draft.ContentProjectsParams{ProjectID: r.projectID, PageLimit: page.Limit, PageOffset: page.Offset})
	if err != nil {
		return nil, fmt.Errorf("read projects content: %w", err)
	}
	out := make([]projectsnapshot.ProjectRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectsnapshot.ProjectRecord{
			ID:                        row.ID,
			CreatedAt:                 projectsnapshot.Timestamp(row.CreatedAt),
			UpdatedAt:                 projectsnapshot.Timestamp(row.UpdatedAt),
			SystemName:                row.SystemName,
			UIName:                    row.UiName,
			Description:               row.Description,
			Status:                    row.Status,
			Namespace:                 row.Namespace,
			ParentProjectID:           row.ParentProjectID,
			StagingID:                 row.StagingID,
			OwnerID:                   row.OwnerID,
			Visibility:                row.Visibility,
			Deprecated:                row.Deprecated,
			License:                   row.License,
			README:                    row.Readme,
			Topics:                    row.Topics,
			BaseURL:                   row.BaseUrl,
			CreatedByID:               row.CreatedByID,
			VersionNumber:             row.VersionNumber,
			ConceptNamespace:          row.ConceptNamespace,
			EffectiveConceptNamespace: &row.EffectiveConceptNamespace,
			EnforceConceptLists:       &row.EnforceConceptLists,
		})
	}
	return out, nil
}
