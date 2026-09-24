//go:build integration

package vocabulary_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/vocabulary"
)

// TestSealedList_RejectsAdd proves a sealed list locks its membership: terms
// can be added while open, are rejected while sealed, and can be added again
// after unsealing.
func TestSealedList_RejectsAdd(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	mustExec(t, pool, `INSERT INTO weave_actors (id, display_name, slug) VALUES ('own5','O5','own5')`)
	mustExec(t, pool, `INSERT INTO weave_projects (id, owner_id) VALUES ('SEALP','own5')`)
	mustExec(t, pool, `INSERT INTO weave_concept_lists (id, project_id) VALUES ('CLSEAL','SEALP')`)

	svc := vocabulary.NewService(pool, nil)
	label := func(s string) domain.Translations { return domain.Translations{"en": s} }

	if _, err := svc.CreateLocalTerm(ctx, "SEALP", "CLSEAL", vocabulary.CreateTermInput{Label: label("A")}); err != nil {
		t.Fatalf("add while open: %v", err)
	}
	if err := svc.SetListClosed(ctx, "SEALP", "CLSEAL", true); err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := svc.CreateLocalTerm(ctx, "SEALP", "CLSEAL", vocabulary.CreateTermInput{Label: label("B")}); err == nil {
		t.Fatal("expected adding to a sealed list to be rejected")
	}
	if err := svc.SetListClosed(ctx, "SEALP", "CLSEAL", false); err != nil {
		t.Fatalf("unseal: %v", err)
	}
	if _, err := svc.CreateLocalTerm(ctx, "SEALP", "CLSEAL", vocabulary.CreateTermInput{Label: label("C")}); err != nil {
		t.Fatalf("add after unseal: %v", err)
	}
}
