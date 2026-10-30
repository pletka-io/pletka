package vocabulary

import (
	"context"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/database/readsql/archive"
	"github.com/pletka-io/pletka/pkg/database/snapshotrow"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

type archiveContentReader struct {
	queries   archive.Querier
	projectID string
	version   string
}

// ContentBroader returns one deterministic page of lossless owned records.
func (r *archiveContentReader) ContentBroader(ctx context.Context, page readsession.Page) ([]projectsnapshot.BroaderRecord, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ContentBroader(ctx, archive.ContentBroaderParams{ProjectID: r.projectID, PageLimit: page.Limit, PageOffset: page.Offset, Version: r.version})
	if err != nil {
		return nil, fmt.Errorf("read broader content: %w", err)
	}
	out := make([]projectsnapshot.BroaderRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectsnapshot.BroaderRecord{
			ID:            row.ID,
			ConceptID:     row.ConceptID,
			BroaderID:     row.BroaderID,
			SchemeID:      row.SchemeID,
			Position:      row.Position,
			VersionNumber: row.VersionNumber,
			CreatedAt:     snapshotrow.Nullable[projectsnapshot.Timestamp](row.CreatedAt),
		})
	}
	return out, nil
}

// ContentConceptEntries returns one deterministic page of lossless owned records.
func (r *archiveContentReader) ContentConceptEntries(ctx context.Context, page readsession.Page) ([]projectsnapshot.ConceptEntryRecord, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ContentConceptEntries(ctx, archive.ContentConceptEntriesParams{ProjectID: r.projectID, PageLimit: page.Limit, PageOffset: page.Offset, Version: r.version})
	if err != nil {
		return nil, fmt.Errorf("read conceptentries content: %w", err)
	}
	out := make([]projectsnapshot.ConceptEntryRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectsnapshot.ConceptEntryRecord{
			ID:                row.ID,
			ConceptListID:     row.ConceptListID,
			VocabularyEntryID: row.VocabularyEntryID,
			Position:          row.Position,
			CustomLabel:       row.CustomLabel,
			CreatedAt:         projectsnapshot.Timestamp(row.CreatedAt),
			UpdatedAt:         projectsnapshot.Timestamp(row.UpdatedAt),
			VersionNumber:     row.VersionNumber,
		})
	}
	return out, nil
}

// ContentConceptLists returns one deterministic page of lossless owned records.
func (r *archiveContentReader) ContentConceptLists(ctx context.Context, page readsession.Page) ([]projectsnapshot.ConceptListRecord, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ContentConceptLists(ctx, archive.ContentConceptListsParams{ProjectID: r.projectID, PageLimit: page.Limit, PageOffset: page.Offset, Version: r.version})
	if err != nil {
		return nil, fmt.Errorf("read conceptlists content: %w", err)
	}
	out := make([]projectsnapshot.ConceptListRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectsnapshot.ConceptListRecord{
			ID:            row.ID,
			CreatedAt:     projectsnapshot.Timestamp(row.CreatedAt),
			UpdatedAt:     projectsnapshot.Timestamp(row.UpdatedAt),
			SemanticID:    row.SemanticID,
			SystemName:    row.SystemName,
			UIName:        row.UiName,
			Description:   row.Description,
			Status:        row.Status,
			ProjectID:     row.ProjectID,
			ListType:      row.ListType,
			VocabularyID:  row.VocabularyID,
			VersionNumber: row.VersionNumber,
			IsClosed:      row.IsClosed,
		})
	}
	return out, nil
}

// ContentVocabularies returns one deterministic page of lossless owned records.
func (r *archiveContentReader) ContentVocabularies(ctx context.Context, page readsession.Page) ([]projectsnapshot.VocabularyRecord, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ContentVocabularies(ctx, archive.ContentVocabulariesParams{ProjectID: r.projectID, PageLimit: page.Limit, PageOffset: page.Offset, Version: r.version})
	if err != nil {
		return nil, fmt.Errorf("read vocabularies content: %w", err)
	}
	out := make([]projectsnapshot.VocabularyRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectsnapshot.VocabularyRecord{
			ID:            row.ID,
			ProjectID:     row.ProjectID,
			SemanticID:    row.SemanticID,
			SystemName:    row.SystemName,
			UIName:        row.UiName,
			Description:   row.Description,
			Status:        row.Status,
			ConnectorType: row.ConnectorType,
			BaseURI:       row.BaseUri,
			Config:        row.Config,
			Deprecated:    row.Deprecated,
			CreatedAt:     projectsnapshot.Timestamp(row.CreatedAt),
			UpdatedAt:     projectsnapshot.Timestamp(row.UpdatedAt),
			VersionNumber: row.VersionNumber,
		})
	}
	return out, nil
}

// ContentVocabularyEntries returns one deterministic page of lossless owned records.
func (r *archiveContentReader) ContentVocabularyEntries(ctx context.Context, page readsession.Page) ([]projectsnapshot.VocabularyEntryRecord, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ContentVocabularyEntries(ctx, archive.ContentVocabularyEntriesParams{ProjectID: r.projectID, PageLimit: page.Limit, PageOffset: page.Offset, Version: r.version})
	if err != nil {
		return nil, fmt.Errorf("read vocabularyentries content: %w", err)
	}
	out := make([]projectsnapshot.VocabularyEntryRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectsnapshot.VocabularyEntryRecord{
			ID:               row.ID,
			VocabularyID:     row.VocabularyID,
			URI:              row.Uri,
			Label:            row.Label,
			ScopeNote:        row.ScopeNote,
			BroaderURI:       row.BroaderUri,
			BroaderPath:      row.BroaderPath,
			BroaderPathItems: row.BroaderPathItems,
			ExternalID:       row.ExternalID,
			CreatedAt:        projectsnapshot.Timestamp(row.CreatedAt),
			UpdatedAt:        projectsnapshot.Timestamp(row.UpdatedAt),
			VersionNumber:    row.VersionNumber,
		})
	}
	return out, nil
}
