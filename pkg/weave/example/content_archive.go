package example

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

// ContentExamples returns one deterministic page of lossless owned records.
func (r *archiveContentReader) ContentExamples(ctx context.Context, page readsession.Page) ([]projectsnapshot.ExampleRecord, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ContentExamples(ctx, archive.ContentExamplesParams{ProjectID: r.projectID, PageLimit: page.Limit, PageOffset: page.Offset, Version: r.version})
	if err != nil {
		return nil, fmt.Errorf("read examples content: %w", err)
	}
	out := make([]projectsnapshot.ExampleRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectsnapshot.ExampleRecord{
			ID:            row.ID,
			ProjectID:     row.ProjectID,
			EntityType:    row.EntityType,
			EntityID:      row.EntityID,
			Title:         row.Title,
			Description:   row.Description,
			Status:        row.Status,
			CreatedAt:     projectsnapshot.Timestamp(row.CreatedAt),
			UpdatedAt:     projectsnapshot.Timestamp(row.UpdatedAt),
			VersionNumber: row.VersionNumber,
		})
	}
	return out, nil
}

// ContentExampleValues returns one deterministic page of lossless owned records.
func (r *archiveContentReader) ContentExampleValues(ctx context.Context, page readsession.Page) ([]projectsnapshot.ExampleValueRecord, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ContentExampleValues(ctx, archive.ContentExampleValuesParams{ProjectID: r.projectID, PageLimit: page.Limit, PageOffset: page.Offset, Version: r.version})
	if err != nil {
		return nil, fmt.Errorf("read examplevalues content: %w", err)
	}
	out := make([]projectsnapshot.ExampleValueRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectsnapshot.ExampleValueRecord{
			ID:                 row.ID,
			ExampleID:          row.ExampleID,
			OverrideID:         row.OverrideID,
			FieldID:            row.FieldID,
			PartOfCollectionID: row.PartOfCollectionID,
			OccurrenceIndex:    row.OccurrenceIndex,
			ValueKind:          row.ValueKind,
			ValuePayload:       row.ValuePayload,
			TextValue:          row.TextValue,
			NumberValue:        snapshotrow.Nullable[projectsnapshot.Decimal](row.NumberValue),
			DateValue:          snapshotrow.Nullable[projectsnapshot.Date](row.DateValue),
			URIValue:           row.UriValue,
			ConceptURI:         row.ConceptUri,
			LinkedExampleID:    row.LinkedExampleID,
			SlotPath:           row.SlotPath,
			CreatedAt:          projectsnapshot.Timestamp(row.CreatedAt),
			UpdatedAt:          projectsnapshot.Timestamp(row.UpdatedAt),
			VersionNumber:      row.VersionNumber,
		})
	}
	return out, nil
}
