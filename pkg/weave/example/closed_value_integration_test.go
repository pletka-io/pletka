//go:build integration

package example_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/weave/example"
)

func exec(t *testing.T, pool *pgxpool.Pool, sql string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql); err != nil {
		t.Fatalf("seed exec failed: %v\nSQL: %s", err, sql)
	}
}

// TestConceptURIAllowedForLists_AlwaysExplicit proves value validation accepts
// only a list's explicit entries — open or sealed. A source-vocab term that is
// not an entry is rejected in both states; open/sealed governs list-membership
// editing, not which values a bound field accepts (#3599).
func TestConceptURIAllowedForLists_AlwaysExplicit(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	exec(t, pool, `INSERT INTO weave_actors (id, display_name, slug) VALUES ('cvo','CVO','cvo')`)
	exec(t, pool, `INSERT INTO weave_projects (id, owner_id) VALUES ('CVP','cvo')`)
	exec(t, pool, `INSERT INTO weave_vocabularies (id, connector_type, project_id) VALUES ('VSRC','aat','CVP')`)
	exec(t, pool, `INSERT INTO weave_vocabulary_entries (id, vocabulary_id, uri, label) VALUES ('E1','VSRC','uri:1','{"en":"One"}')`)
	exec(t, pool, `INSERT INTO weave_vocabulary_entries (id, vocabulary_id, uri, label) VALUES ('E2','VSRC','uri:2','{"en":"Two"}')`)
	exec(t, pool, `INSERT INTO weave_concept_lists (id, project_id, vocabulary_id, is_closed) VALUES ('CVCL','CVP','VSRC',false)`)
	exec(t, pool, `INSERT INTO weave_concept_list_entries (id, concept_list_id, vocabulary_entry_id, position) VALUES ('J1','CVCL','E1',1)`)

	// ConceptURIAllowedForLists is on the concrete store (consumed by the
	// service via a validator interface), not the public Store interface.
	type conceptValidator interface {
		ConceptURIAllowedForLists(ctx context.Context, uri string, conceptListIDs []string) (bool, error)
	}
	store := example.NewPostgresStore(pool).(conceptValidator)
	lists := []string{"CVCL"}

	// OPEN: uri:2 is from the source vocab but not an explicit entry -> rejected.
	if ok, err := store.ConceptURIAllowedForLists(ctx, "uri:2", lists); err != nil || ok {
		t.Fatalf("open list should reject a non-entry term: ok=%v err=%v", ok, err)
	}
	// OPEN: uri:1 (explicit entry) -> allowed.
	if ok, err := store.ConceptURIAllowedForLists(ctx, "uri:1", lists); err != nil || !ok {
		t.Fatalf("open list should allow an explicit entry: ok=%v err=%v", ok, err)
	}

	// Sealing changes nothing for value validation.
	exec(t, pool, `UPDATE weave_concept_lists SET is_closed = true WHERE id = 'CVCL'`)
	if ok, err := store.ConceptURIAllowedForLists(ctx, "uri:2", lists); err != nil || ok {
		t.Fatalf("sealed list should reject a non-entry term: ok=%v err=%v", ok, err)
	}
	if ok, err := store.ConceptURIAllowedForLists(ctx, "uri:1", lists); err != nil || !ok {
		t.Fatalf("sealed list should allow an explicit entry: ok=%v err=%v", ok, err)
	}
}
