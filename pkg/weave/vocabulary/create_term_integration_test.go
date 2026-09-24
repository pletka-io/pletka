//go:build integration

package vocabulary_test

import (
	"context"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/vocabulary"
)

// TestCreateLocalTerm_Integration proves the AAT-unblock: a hand-typed term is
// created in the project's local vocabulary (curie URI, connector_type local),
// linked to the list, with no remote authority involved; the local vocabulary
// is provisioned once and reused.
func TestCreateLocalTerm_Integration(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `INSERT INTO weave_actors (id, display_name, slug) VALUES ($1,$2,$3)`, "owner1", "Owner", "owner1"); err != nil {
		t.Fatalf("seed actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_projects (id, owner_id) VALUES ($1,$2)`, "TESTVOC", "owner1"); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_concept_lists (id, project_id) VALUES ($1,$2)`, "CLVOC1", "TESTVOC"); err != nil {
		t.Fatalf("seed concept list: %v", err)
	}

	// registry nil -> local fallback; CreateLocalTerm never touches a connector.
	svc := vocabulary.NewService(pool, nil)

	entry, err := svc.CreateLocalTerm(ctx, "TESTVOC", "CLVOC1", vocabulary.CreateTermInput{
		Label: domain.Translations{"en": "Female"},
	})
	if err != nil {
		t.Fatalf("CreateLocalTerm: %v", err)
	}
	if entry == nil {
		t.Fatal("expected a concept-list entry view, got nil")
	}

	var uri, connType string
	if err := pool.QueryRow(ctx, `
SELECT e.uri, v.connector_type
FROM weave_concept_list_entries j
JOIN weave_vocabulary_entries e ON e.id = j.vocabulary_entry_id
JOIN weave_vocabularies v ON v.id = e.vocabulary_id
WHERE j.concept_list_id = $1`, "CLVOC1").Scan(&uri, &connType); err != nil {
		t.Fatalf("lookup linked term: %v", err)
	}
	if connType != "local" {
		t.Fatalf("term should live in a local vocabulary, got connector_type %q", connType)
	}
	if !strings.HasPrefix(uri, "pletka:concept/") {
		t.Fatalf("expected a pletka:concept/ curie, got %q", uri)
	}

	// A second term reuses the same local vocabulary (idempotent provisioning).
	if _, err := svc.CreateLocalTerm(ctx, "TESTVOC", "CLVOC1", vocabulary.CreateTermInput{
		Label: domain.Translations{"en": "Male"},
	}); err != nil {
		t.Fatalf("second CreateLocalTerm: %v", err)
	}
	var localVocabs int
	if err := pool.QueryRow(ctx, `
SELECT count(*) FROM weave_vocabularies WHERE project_id=$1 AND connector_type='local'`, "TESTVOC").Scan(&localVocabs); err != nil {
		t.Fatalf("count local vocabs: %v", err)
	}
	if localVocabs != 1 {
		t.Fatalf("expected exactly one reused local vocabulary, got %d", localVocabs)
	}
}
