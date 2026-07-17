package collection

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/domain"
)

type fakeCategoryReader struct {
	byID         map[string]*domain.Category
	byIdentifier map[string]map[string]*domain.Category // identifier -> projectID -> category
	getByIDErr   error
}

func (f *fakeCategoryReader) GetByID(_ context.Context, id string) (*domain.Category, error) {
	if f.getByIDErr != nil {
		return nil, f.getByIDErr
	}
	return f.byID[id], nil
}

func (f *fakeCategoryReader) GetByIdentifier(_ context.Context, identifier, projectID string) (*domain.Category, error) {
	if byProj, ok := f.byIdentifier[identifier]; ok {
		return byProj[projectID], nil
	}
	return nil, nil
}

func cat(id, projectID, systemName string) *domain.Category {
	return &domain.Category{
		Entity: domain.Entity{
			ID:         id,
			SemanticID: id,
			SystemName: systemName,
			ProjectID:  projectID,
		},
	}
}

func TestBuildCategoryRemap_Collection(t *testing.T) {
	t.Parallel()

	currentProject := "TPZ"
	// Seed list combines the collection-level DefaultCategoryID and per-row
	// CategoryIDs from a hypothetical source collection's overrides.
	seed := []string{
		"LA.CAT.3",  // default (will appear again from an override row)
		"LA.CAT.3",  // duplicate -> dedup
		"LA.CAT.7",  // no local equivalent -> clear
		"",          // empty -> skip
		"LA.CAT.99", // source category itself missing -> skip silently
	}

	tests := []struct {
		name   string
		reader *fakeCategoryReader
		want   map[string]string
	}{
		{
			name:   "nil reader: empty map; caller leaves source IDs untouched",
			reader: nil,
			want:   map[string]string{},
		},
		{
			name: "remap when local exists; clear when local missing; skip when source missing",
			reader: &fakeCategoryReader{
				byID: map[string]*domain.Category{
					"LA.CAT.3": cat("LA.CAT.3", "LA", "identifiers"),
					"LA.CAT.7": cat("LA.CAT.7", "LA", "annotations"),
					// LA.CAT.99 absent
				},
				byIdentifier: map[string]map[string]*domain.Category{
					"identifiers": {currentProject: cat("TPZ.CAT.5", currentProject, "identifiers")},
					// "annotations" absent in TPZ
				},
			},
			want: map[string]string{
				"LA.CAT.3": "TPZ.CAT.5",
				"LA.CAT.7": "",
			},
		},
		{
			name: "store error: row silently skipped, never panics",
			reader: &fakeCategoryReader{
				getByIDErr: errors.New("db blip"),
			},
			want: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reader CategoryReader
			if tt.reader != nil {
				reader = tt.reader
			}
			svc := &Service{categories: reader}
			got := svc.buildCategoryRemap(context.Background(), currentProject, seed)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("remap mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
