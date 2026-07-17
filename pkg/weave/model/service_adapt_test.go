package model

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/domain"
)

// fakeCategoryReader is a table-driven CategoryReader stub used by the
// Adapt-remap tests. byID drives GetByID lookups; byIdentifier drives
// system_name lookups within a project. Returning nil from either is
// treated as "not found" by the production code.
type fakeCategoryReader struct {
	byID          map[string]*domain.Category
	byIdentifier  map[string]map[string]*domain.Category // identifier -> projectID -> category
	getByIDErr    error
	getByIdentErr error
}

func (f *fakeCategoryReader) GetByID(_ context.Context, id string) (*domain.Category, error) {
	if f.getByIDErr != nil {
		return nil, f.getByIDErr
	}
	return f.byID[id], nil
}

func (f *fakeCategoryReader) GetByIdentifier(_ context.Context, identifier, projectID string) (*domain.Category, error) {
	if f.getByIdentErr != nil {
		return nil, f.getByIdentErr
	}
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

func TestBuildCategoryRemap(t *testing.T) {
	t.Parallel()

	currentProject := "TPZ"
	overrides := []domain.FieldOverride{
		{FieldID: "LAF.1", CategoryID: "LA.CAT.3"},  // source category exists locally → remap
		{FieldID: "LAF.2", CategoryID: "LA.CAT.7"},  // source category has no local match → clear
		{FieldID: "LAF.3", CategoryID: ""},          // no category → skip
		{FieldID: "LAF.4", CategoryID: "LA.CAT.3"},  // duplicate of first → seen-set caught
		{FieldID: "LAF.5", CategoryID: "LA.CAT.99"}, // source category itself missing → clear (treated as miss)
	}

	tests := []struct {
		name      string
		reader    *fakeCategoryReader
		want      map[string]string
		wantEmpty bool
	}{
		{
			name:      "nil reader: empty map, source IDs flow through caller-side",
			reader:    nil,
			want:      map[string]string{},
			wantEmpty: true,
		},
		{
			name: "remap when local exists; clear when local missing or source missing",
			reader: &fakeCategoryReader{
				byID: map[string]*domain.Category{
					"LA.CAT.3": cat("LA.CAT.3", "LA", "identifiers"),
					"LA.CAT.7": cat("LA.CAT.7", "LA", "annotations"),
					// LA.CAT.99 deliberately absent — exercises the "source itself missing" path
				},
				byIdentifier: map[string]map[string]*domain.Category{
					"identifiers": {currentProject: cat("TPZ.CAT.5", currentProject, "identifiers")},
					// "annotations" deliberately absent in TPZ — exercises clear-on-miss
				},
			},
			want: map[string]string{
				"LA.CAT.3": "TPZ.CAT.5", // remapped
				"LA.CAT.7": "",          // local missing → clear
				// LA.CAT.99 never reaches the second lookup, never enters the map
			},
		},
		{
			name: "all source categories missing → empty map (caller clears via missing key)",
			reader: &fakeCategoryReader{
				byID:         map[string]*domain.Category{},
				byIdentifier: map[string]map[string]*domain.Category{},
			},
			want: map[string]string{},
		},
		{
			name: "GetByID errors are silent → row skipped, not panicked",
			reader: &fakeCategoryReader{
				getByIDErr: errors.New("transient db blip"),
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
			got := svc.buildCategoryRemap(context.Background(), currentProject, overrides)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("remap mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
