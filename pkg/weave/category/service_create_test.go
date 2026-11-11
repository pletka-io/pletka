package category

import (
	"context"
	"testing"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

// TestCreate_KeysCategoryBySemanticID pins that a category created through
// the service is keyed by its semantic id, like fields, models, collections
// and drafted categories. A ULID key leaks into field overrides as
// category_id and shows up raw in the UI.
func TestCreate_KeysCategoryBySemanticID(t *testing.T) {
	ctx := weaveauth.WithSnapshot(context.Background(), &weaveauth.AuthSnapshot{IsSuperAdmin: true})
	ctx = weaveauth.WithProject(ctx, &domain.Project{Entity: domain.Entity{ID: "INH"}, OwnerID: "ORG1"})

	svc := NewService(&detachTestStore{}, nil, nil, domain.NoopChangeLogRunner(), descriptorNumberer{})
	cat, err := svc.Create(ctx, "INH", CreateInput{SystemName: "rights", UIName: domain.Translations{"en": "Rights"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if cat.ID != "INH.CAT.1" || cat.SemanticID != "INH.CAT.1" {
		t.Fatalf("ID = %q, SemanticID = %q; want both INH.CAT.1", cat.ID, cat.SemanticID)
	}
}
