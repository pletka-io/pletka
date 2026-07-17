package category

import (
	"context"
	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"testing"
)

func TestUpdate_DetachesProjectCategoryAdoptionOnStructuralEdit(t *testing.T) {
	ctx := context.Background()
	ctx = weaveauth.WithSnapshot(ctx, &weaveauth.AuthSnapshot{IsSuperAdmin: true})
	ctx = weaveauth.WithProject(ctx, &domain.Project{
		Entity:     domain.Entity{ID: "INH"},
		OwnerID:    "ORG1",
		Visibility: "private",
	})

	store := &detachTestStore{
		byID: map[string]*domain.Category{
			"cat-1": {
				Entity: domain.Entity{
					ID:         "cat-1",
					ProjectID:  "INH",
					SemanticID: "INH.CAT.1",
					SystemName: "names_and_classifications",
					UIName:     domain.Translations{"en": "Names and Identifiers"},
					Status:     domain.StatusDraft,
				},
			},
		},
		byIdentifier: map[string]*domain.Category{
			"LA|LA.CAT.1": {
				Entity: domain.Entity{
					ID:         "LA.CAT.1",
					ProjectID:  "LA",
					SemanticID: "LA.CAT.1",
					SystemName: "names_and_classifications",
					UIName:     domain.Translations{"en": "Names and Identifiers"},
					Status:     domain.StatusDraft,
				},
			},
		},
	}
	adoptions := &detachTestAdoptionStore{
		adoptions: []domain.Adoption{
			{
				ProjectID:         "INH",
				ContextEntityType: "project",
				ContextEntityID:   "INH",
				EntityType:        "category",
				SourceProjectID:   "LA",
				SourceEntityID:    "LA.CAT.1",
			},
		},
	}
	svc := NewService(store, adoptions, nil, domain.NoopChangeLogRunner(), nil)

	updated, err := svc.Update(ctx, "INH", "cat-1", UpdateInput{
		UIName: &domain.Translations{"en": "Names and Identifiers (Local)"},
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got := updated.UIName.Get("en", ""); got != "Names and Identifiers (Local)" {
		t.Fatalf("updated UIName = %q, want local rename", got)
	}
	if len(adoptions.replaced) != 0 {
		t.Fatalf("replaced adoptions len = %d, want 0 after detach", len(adoptions.replaced))
	}
	if !adoptions.replaceCalled {
		t.Fatalf("expected ReplaceForContext to be called")
	}
}

type detachTestStore struct {
	byID         map[string]*domain.Category
	byIdentifier map[string]*domain.Category
}

func (s *detachTestStore) Create(context.Context, *domain.Category) error { return nil }

func (s *detachTestStore) GetByID(_ context.Context, projectID, id string) (*domain.Category, error) {
	row := s.byID[id]
	if row == nil || row.ProjectID != projectID {
		return nil, nil
	}
	dup := *row
	return &dup, nil
}

func (s *detachTestStore) GetByIdentifier(_ context.Context, projectID, identifier string) (*domain.Category, error) {
	row := s.byIdentifier[projectID+"|"+identifier]
	if row == nil {
		return nil, nil
	}
	dup := *row
	return &dup, nil
}

func (s *detachTestStore) Update(_ context.Context, c *domain.Category) error {
	dup := *c
	s.byID[c.ID] = &dup
	return nil
}

func (s *detachTestStore) UpdateFields(context.Context, string, map[string]any) error { return nil }
func (s *detachTestStore) Delete(context.Context, string, string) error               { return nil }
func (s *detachTestStore) DeleteWithReassignment(context.Context, string, string, string, string) error {
	return nil
}
func (s *detachTestStore) List(context.Context, string, ...domain.QueryOption) ([]*domain.Category, error) {
	return nil, nil
}
func (s *detachTestStore) Count(context.Context, string, ...domain.QueryOption) (int64, error) {
	return 0, nil
}
func (s *detachTestStore) ListWithCounts(context.Context, string) ([]WithCounts, error) {
	return nil, nil
}
func (s *detachTestStore) Reorder(context.Context, string, []string) error { return nil }
func (s *detachTestStore) ModelFieldOverrides(context.Context, string, string) ([]domain.OverrideEntry, error) {
	return nil, nil
}
func (s *detachTestStore) CollectionFieldOverrides(context.Context, string, string) ([]domain.OverrideEntry, error) {
	return nil, nil
}
func (s *detachTestStore) Deprecate(context.Context, string, string) error { return nil }
func (s *detachTestStore) Activate(context.Context, string, string) error  { return nil }
func (s *detachTestStore) IsInUse(context.Context, string, string, string) (bool, error) {
	return false, nil
}

type detachTestAdoptionStore struct {
	adoptions      []domain.Adoption
	replaced       []domain.Adoption
	replaceCalled  bool
	replaceProject string
	replaceCtxType string
	replaceCtxID   string
}

func (s *detachTestAdoptionStore) List(_ context.Context, _ ...domain.QueryOption) ([]domain.Adoption, error) {
	out := make([]domain.Adoption, len(s.adoptions))
	copy(out, s.adoptions)
	return out, nil
}

func (s *detachTestAdoptionStore) ReplaceForContext(_ context.Context, projectID, contextEntityType, contextEntityID string, adoptions []domain.Adoption) error {
	s.replaceCalled = true
	s.replaceProject = projectID
	s.replaceCtxType = contextEntityType
	s.replaceCtxID = contextEntityID
	s.replaced = make([]domain.Adoption, len(adoptions))
	copy(s.replaced, adoptions)
	return nil
}
