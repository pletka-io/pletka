//go:build integration

package override

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/domain"
)

// Scratch coordinates for ReplaceForEntity id-preservation tests: real LA
// fixture fields (LAF.5, LAF.6) placed on a scratch model id that does not
// collide with any real LA model.
const (
	replaceTestProjectID  = "LA"
	replaceTestEntityType = "model"
	replaceTestEntityID   = "TSTSTABLE.1"
)

// laFieldID resolves the real weave_fields.id backing a LA fixture field's
// semantic id, so these tests exercise real fixture data (the materializer
// mints the row id at hydrate time, so it cannot be hardcoded).
func laFieldID(t *testing.T, pool *pgxpool.Pool, semanticID string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`SELECT id FROM weave_fields WHERE project_id = $1 AND semantic_id = $2`,
		replaceTestProjectID, semanticID).Scan(&id)
	if err != nil {
		t.Fatalf("resolve field %s: %v", semanticID, err)
	}
	return id
}

// cleanupReplaceEntity registers cleanup of every override row on the
// scratch entity, so consecutive tests in this file don't see each other's
// leftovers.
func cleanupReplaceEntity(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM weave_field_overrides WHERE entity_type = $1 AND entity_id = $2`,
			replaceTestEntityType, replaceTestEntityID)
	})
}

func TestReplaceForEntityKeepsIDs(t *testing.T) {
	pool := testdb.Pool(t)
	store := NewPostgresStore(pool)
	ctx := context.Background()
	cleanupReplaceEntity(t, pool)

	f5 := laFieldID(t, pool, "LAF.5")
	f6 := laFieldID(t, pool, "LAF.6")

	overrides := []domain.FieldOverride{
		{FieldID: f5, ProjectID: replaceTestProjectID, Position: 1, DisplayName: domain.Translations{"en": "Name Type"}},
		{FieldID: f6, ProjectID: replaceTestProjectID, Position: 2, DisplayName: domain.Translations{"en": "Name Value"}},
	}
	if err := store.ReplaceForEntity(ctx, replaceTestEntityType, replaceTestEntityID, overrides); err != nil {
		t.Fatalf("first save: %v", err)
	}
	id5, id6 := overrides[0].ID, overrides[1].ID
	if id5 == 0 || id6 == 0 {
		t.Fatalf("expected generated ids, got %d, %d", id5, id6)
	}

	overrides[0].DisplayName = domain.Translations{"en": "Name Type Updated"}
	overrides[1].DisplayName = domain.Translations{"en": "Name Value Updated"}
	if err := store.ReplaceForEntity(ctx, replaceTestEntityType, replaceTestEntityID, overrides); err != nil {
		t.Fatalf("second save: %v", err)
	}
	if overrides[0].ID != id5 || overrides[1].ID != id6 {
		t.Fatalf("save changed ids: got %d, %d want %d, %d", overrides[0].ID, overrides[1].ID, id5, id6)
	}

	list, err := store.ListForEntity(ctx, replaceTestEntityType, replaceTestEntityID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := map[int64]string{}
	for _, o := range list {
		got[o.ID] = o.DisplayName.Get("en")
	}
	if got[id5] != "Name Type Updated" {
		t.Errorf("display name for %d = %q, want %q", id5, got[id5], "Name Type Updated")
	}
	if got[id6] != "Name Value Updated" {
		t.Errorf("display name for %d = %q, want %q", id6, got[id6], "Name Value Updated")
	}
}

func TestReplaceForEntityReusesByKeyWithoutIDs(t *testing.T) {
	pool := testdb.Pool(t)
	store := NewPostgresStore(pool)
	ctx := context.Background()
	cleanupReplaceEntity(t, pool)

	f5 := laFieldID(t, pool, "LAF.5")
	f6 := laFieldID(t, pool, "LAF.6")

	overrides := []domain.FieldOverride{
		{FieldID: f5, ProjectID: replaceTestProjectID, Position: 1},
		{FieldID: f6, ProjectID: replaceTestProjectID, Position: 2},
	}
	if err := store.ReplaceForEntity(ctx, replaceTestEntityType, replaceTestEntityID, overrides); err != nil {
		t.Fatalf("first save: %v", err)
	}
	id5, id6 := overrides[0].ID, overrides[1].ID

	// Resend the same rows with ID left at 0, as a naive editor payload would.
	resend := []domain.FieldOverride{
		{FieldID: f5, ProjectID: replaceTestProjectID, Position: 1},
		{FieldID: f6, ProjectID: replaceTestProjectID, Position: 2},
	}
	if err := store.ReplaceForEntity(ctx, replaceTestEntityType, replaceTestEntityID, resend); err != nil {
		t.Fatalf("second save: %v", err)
	}
	if resend[0].ID != id5 || resend[1].ID != id6 {
		t.Fatalf("ids not reused by key: got %d, %d want %d, %d", resend[0].ID, resend[1].ID, id5, id6)
	}
}

func TestReplaceForEntityRemovesOnlyDropped(t *testing.T) {
	pool := testdb.Pool(t)
	store := NewPostgresStore(pool)
	ctx := context.Background()
	cleanupReplaceEntity(t, pool)

	f5 := laFieldID(t, pool, "LAF.5")
	f6 := laFieldID(t, pool, "LAF.6")

	overrides := []domain.FieldOverride{
		{FieldID: f5, ProjectID: replaceTestProjectID, Position: 1},
		{FieldID: f6, ProjectID: replaceTestProjectID, Position: 2},
	}
	if err := store.ReplaceForEntity(ctx, replaceTestEntityType, replaceTestEntityID, overrides); err != nil {
		t.Fatalf("first save: %v", err)
	}
	id5, id6 := overrides[0].ID, overrides[1].ID

	keep := []domain.FieldOverride{
		{ID: id6, FieldID: f6, ProjectID: replaceTestProjectID, Position: 1},
	}
	if err := store.ReplaceForEntity(ctx, replaceTestEntityType, replaceTestEntityID, keep); err != nil {
		t.Fatalf("second save: %v", err)
	}
	if keep[0].ID != id6 {
		t.Fatalf("kept row changed id: got %d want %d", keep[0].ID, id6)
	}

	got, err := store.GetByID(ctx, id5)
	if err != nil {
		t.Fatalf("get dropped row: %v", err)
	}
	if got != nil {
		t.Fatalf("dropped row %d still exists", id5)
	}
}

func TestReplaceForEntityKeepsExampleValues(t *testing.T) {
	pool := testdb.Pool(t)
	store := NewPostgresStore(pool)
	ctx := context.Background()
	cleanupReplaceEntity(t, pool)

	f5 := laFieldID(t, pool, "LAF.5")
	overrides := []domain.FieldOverride{
		{FieldID: f5, ProjectID: replaceTestProjectID, Position: 1},
	}
	if err := store.ReplaceForEntity(ctx, replaceTestEntityType, replaceTestEntityID, overrides); err != nil {
		t.Fatalf("first save: %v", err)
	}
	overrideID := overrides[0].ID

	const exampleID = "01TSTSTABLE000000000000001"
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_examples WHERE id = $1`, exampleID)
	})
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_examples (id, project_id, entity_type, entity_id) VALUES ($1, $2, $3, $4)`,
		exampleID, replaceTestProjectID, replaceTestEntityType, replaceTestEntityID,
	); err != nil {
		t.Fatalf("insert example: %v", err)
	}

	slotPath := fmt.Sprintf("%d:0", overrideID)
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_example_values (example_id, override_id, field_id, value_kind, value_payload, slot_path)
		 VALUES ($1, $2, $3, 'string', $4::jsonb, $5)`,
		exampleID, overrideID, f5, `{"kind":"string","string_value":"x"}`, slotPath,
	); err != nil {
		t.Fatalf("insert example value: %v", err)
	}

	overrides[0].DisplayName = domain.Translations{"en": "Changed"}
	if err := store.ReplaceForEntity(ctx, replaceTestEntityType, replaceTestEntityID, overrides); err != nil {
		t.Fatalf("second save: %v", err)
	}
	if overrides[0].ID != overrideID {
		t.Fatalf("save changed id: got %d want %d", overrides[0].ID, overrideID)
	}

	var gotOverrideID int64
	err := pool.QueryRow(ctx,
		`SELECT override_id FROM weave_example_values WHERE example_id = $1 AND slot_path = $2`,
		exampleID, slotPath,
	).Scan(&gotOverrideID)
	if err != nil {
		t.Fatalf("example value row missing after save: %v", err)
	}
	if gotOverrideID != overrideID {
		t.Fatalf("example value anchored to wrong override: got %d want %d", gotOverrideID, overrideID)
	}
}

// TestDeletingOverrideKeepsExampleValues covers the migration 010 contract:
// removing a placement (override row) must not cascade-delete the example
// values anchored to it. Before the migration, weave_example_values has an
// ON DELETE CASCADE fkey on override_id, so this is RED until the fkey is
// dropped.
func TestDeletingOverrideKeepsExampleValues(t *testing.T) {
	pool := testdb.Pool(t)
	store := NewPostgresStore(pool)
	ctx := context.Background()
	cleanupReplaceEntity(t, pool)

	f5 := laFieldID(t, pool, "LAF.5")
	overrides := []domain.FieldOverride{
		{FieldID: f5, ProjectID: replaceTestProjectID, Position: 1},
	}
	if err := store.ReplaceForEntity(ctx, replaceTestEntityType, replaceTestEntityID, overrides); err != nil {
		t.Fatalf("first save: %v", err)
	}
	overrideID := overrides[0].ID

	const exampleID = "01TSTSTABLE000000000000002"
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_example_values WHERE example_id = $1`, exampleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_examples WHERE id = $1`, exampleID)
	})
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_examples (id, project_id, entity_type, entity_id) VALUES ($1, $2, $3, $4)`,
		exampleID, replaceTestProjectID, replaceTestEntityType, replaceTestEntityID,
	); err != nil {
		t.Fatalf("insert example: %v", err)
	}

	slotPath := fmt.Sprintf("%d:0", overrideID)
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_example_values (example_id, override_id, field_id, value_kind, value_payload, slot_path)
		 VALUES ($1, $2, $3, 'string', $4::jsonb, $5)`,
		exampleID, overrideID, f5, `{"kind":"string","string_value":"x"}`, slotPath,
	); err != nil {
		t.Fatalf("insert example value: %v", err)
	}

	// Save without the anchored row: ReplaceForEntity drops the placement.
	if err := store.ReplaceForEntity(ctx, replaceTestEntityType, replaceTestEntityID, nil); err != nil {
		t.Fatalf("second save: %v", err)
	}

	got, err := store.GetByID(ctx, overrideID)
	if err != nil {
		t.Fatalf("get dropped override: %v", err)
	}
	if got != nil {
		t.Fatalf("expected override %d to be removed", overrideID)
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM weave_example_values WHERE example_id = $1 AND slot_path = $2`,
		exampleID, slotPath,
	).Scan(&count); err != nil {
		t.Fatalf("query example value: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected example value to survive placement removal, got count=%d", count)
	}
}
