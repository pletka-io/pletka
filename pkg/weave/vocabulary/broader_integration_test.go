//go:build integration

package vocabulary_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/vocabulary"
)

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql); err != nil {
		t.Fatalf("seed exec failed: %v\nSQL: %s", err, sql)
	}
}

func TestConceptBroader_Integration(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	mustExec(t, pool, `INSERT INTO weave_actors (id, display_name, slug) VALUES ('owner2','Owner2','owner2')`)
	mustExec(t, pool, `INSERT INTO weave_projects (id, owner_id) VALUES ('HIER','owner2')`)
	mustExec(t, pool, `INSERT INTO weave_concept_lists (id, project_id) VALUES ('CLH1','HIER')`)

	svc := vocabulary.NewService(pool, nil)
	label := func(s string) domain.Translations { return domain.Translations{"en": s} }
	if _, err := svc.CreateLocalTerm(ctx, "HIER", "CLH1", vocabulary.CreateTermInput{Label: label("Etching")}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateLocalTerm(ctx, "HIER", "CLH1", vocabulary.CreateTermInput{Label: label("Print")}); err != nil {
		t.Fatal(err)
	}

	// Recover the two concept (vocabulary_entry) ids in insertion order.
	rows, err := pool.Query(ctx, `SELECT vocabulary_entry_id FROM weave_concept_list_entries WHERE concept_list_id='CLH1' ORDER BY position`)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) != 2 {
		t.Fatalf("want 2 terms, got %d", len(ids))
	}
	child, parent := ids[0], ids[1]

	scheme := "CLH1"
	edge, err := svc.AddBroader(ctx, domain.ConceptBroaderEdge{ConceptID: child, BroaderID: parent, SchemeID: &scheme})
	if err != nil {
		t.Fatalf("AddBroader: %v", err)
	}

	broader, err := svc.ListBroader(ctx, child)
	if err != nil {
		t.Fatal(err)
	}
	if len(broader) != 1 || broader[0].BroaderID != parent {
		t.Fatalf("ListBroader(child) = %+v, want one edge to parent", broader)
	}
	narrower, err := svc.ListNarrower(ctx, parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(narrower) != 1 || narrower[0].ConceptID != child {
		t.Fatalf("ListNarrower(parent) = %+v, want one edge from child", narrower)
	}

	// A self edge is rejected.
	if _, err := svc.AddBroader(ctx, domain.ConceptBroaderEdge{ConceptID: child, BroaderID: child}); err == nil {
		t.Fatal("expected self-broader to be rejected")
	}

	// Duplicate is a no-op (idempotent).
	if _, err := svc.AddBroader(ctx, domain.ConceptBroaderEdge{ConceptID: child, BroaderID: parent, SchemeID: &scheme}); err != nil {
		t.Fatalf("duplicate AddBroader: %v", err)
	}
	again, _ := svc.ListBroader(ctx, child)
	if len(again) != 1 {
		t.Fatalf("duplicate edge created a second row: %d", len(again))
	}

	// Remove.
	if err := svc.RemoveBroader(ctx, edge.ID); err != nil {
		t.Fatal(err)
	}
	empty, _ := svc.ListBroader(ctx, child)
	if len(empty) != 0 {
		t.Fatalf("edge not removed: %+v", empty)
	}
}
