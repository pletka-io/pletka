//go:build integration

package override

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

// Scratch coordinates for the SaveForEntity changelog test: real LA fixture
// fields (LAF.5, LAF.6) placed on a scratch model id that does not collide
// with any real LA model or the other override integration tests'
// (TSTSTABLE.1, TSTFP.1) scratch ids.
const (
	clTestProjectID  = "LA"
	clTestEntityType = "model"
	clTestEntityID   = "TSTCHANGELOG.1"
)

func cleanupCLEntity(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM weave_field_overrides WHERE entity_type = $1 AND entity_id = $2`,
			clTestEntityType, clTestEntityID)
	})
}

// testChangeLogRunner is a minimal stand-in for weave.NewChangeLogRunner:
// pkg/weave imports pkg/weave/override (the ADR-0006 exception), so a test
// in this package importing pkg/weave back would cycle. It mirrors the
// production shape closely enough to exercise the real weave_change_log
// write path this test asserts against: one weave_change_set row per Run
// call, one weave_change_log row per Record call, flushed only when fn
// succeeds.
type testChangeLogRunner struct {
	pool            *pgxpool.Pool
	lastChangeSetID int64
}

func (r *testChangeLogRunner) Run(ctx context.Context, fn func(context.Context, domain.ChangeLogRecorder) error) error {
	q := sqlcgen.New(r.pool)
	cs, err := q.WeaveCreateChangeSet(ctx, sqlcgen.WeaveCreateChangeSetParams{
		ProjectID:     clTestProjectID,
		ActorName:     "test",
		ActorEmail:    "test@example.com",
		CommitMessage: "test save",
	})
	if err != nil {
		return err
	}
	r.lastChangeSetID = cs.ID

	rec := &testChangeLogRecorder{}
	if err := fn(ctx, rec); err != nil {
		return err
	}
	for _, entry := range rec.entries {
		if _, err := q.WeaveCreateChangeLogEntry(ctx, sqlcgen.WeaveCreateChangeLogEntryParams{
			ChangeSetID:     cs.ID,
			EntityType:      entry.EntityType,
			EntityID:        entry.EntityID,
			Operation:       entry.Operation,
			ProjectID:       entry.ProjectID,
			FilePath:        entry.FilePath,
			Payload:         entry.Payload,
			PreviousPayload: entry.PreviousPayload,
		}); err != nil {
			return err
		}
	}
	return nil
}

// testChangeLogRecorder buffers entries like production's *ChangeLogger,
// and — this is the part fix round 1 finding 4 added — enforces the exact
// same payload/previous_payload rules ChangeLogger.Record does
// (pkg/weave/changelogger.go), with the same error text. Without this, an
// entry recordDiff builds wrong (e.g. an update missing previous_payload)
// would pass silently here while aborting every real save.
type testChangeLogRecorder struct {
	entries []domain.ChangeLogEntry
}

func (r *testChangeLogRecorder) Record(_ context.Context, entry domain.ChangeLogEntry) error {
	switch entry.Operation {
	case "create":
		if entry.PreviousPayload != nil {
			return fmt.Errorf("create operation must not have previous_payload")
		}
		if entry.Payload == nil {
			return fmt.Errorf("create operation requires payload")
		}
	case "update":
		if entry.PreviousPayload == nil {
			return fmt.Errorf("update operation requires previous_payload")
		}
		if entry.Payload == nil {
			return fmt.Errorf("update operation requires payload")
		}
	case "delete":
		if entry.PreviousPayload == nil {
			return fmt.Errorf("delete operation requires previous_payload")
		}
		if entry.Payload != nil {
			return fmt.Errorf("delete operation must not have payload")
		}
	default:
		return fmt.Errorf("unknown operation: %s", entry.Operation)
	}
	r.entries = append(r.entries, entry)
	return nil
}

// writeCtx carries a super-admin snapshot so SaveForEntity's ProjectEdit
// gate passes without needing a real membership row.
func writeCtx() context.Context {
	return weaveauth.WithSnapshot(context.Background(), &weaveauth.AuthSnapshot{IsSuperAdmin: true})
}

// TestTestChangeLogRecorderRejectsWhatProductionRejects proves the guard
// added for fix round 1 finding 4 actually works: without it, a bad entry
// recordDiff might one day build (e.g. an update missing previous_payload)
// would pass silently through the test's own recorder while aborting a
// real save. No database needed — this is pure validation logic.
func TestTestChangeLogRecorderRejectsWhatProductionRejects(t *testing.T) {
	valid := domain.ChangeLogEntry{Operation: "update", Payload: []byte("{}"), PreviousPayload: []byte("{}")}

	tests := []struct {
		name  string
		entry domain.ChangeLogEntry
		valid bool
	}{
		{"valid create", domain.ChangeLogEntry{Operation: "create", Payload: []byte("{}")}, true},
		{"create with previous_payload", domain.ChangeLogEntry{Operation: "create", Payload: []byte("{}"), PreviousPayload: []byte("{}")}, false},
		{"create without payload", domain.ChangeLogEntry{Operation: "create"}, false},
		{"valid update", valid, true},
		{"update without previous_payload", domain.ChangeLogEntry{Operation: "update", Payload: []byte("{}")}, false},
		{"update without payload", domain.ChangeLogEntry{Operation: "update", PreviousPayload: []byte("{}")}, false},
		{"valid delete", domain.ChangeLogEntry{Operation: "delete", PreviousPayload: []byte("{}")}, true},
		{"delete with payload", domain.ChangeLogEntry{Operation: "delete", Payload: []byte("{}"), PreviousPayload: []byte("{}")}, false},
		{"delete without previous_payload", domain.ChangeLogEntry{Operation: "delete"}, false},
		{"unknown operation", domain.ChangeLogEntry{Operation: "rename"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := &testChangeLogRecorder{}
			err := rec.Record(context.Background(), tc.entry)
			if tc.valid && err != nil {
				t.Errorf("Record() = %v, want nil", err)
			}
			if !tc.valid && err == nil {
				t.Error("Record() = nil, want a validation error")
			}
		})
	}
}

// TestSaveForEntityLogsKeptRowAsChangedAndNewRowWithRealID covers the two
// change-log bugs Task 4 fixes:
//
//  1. ComputeDiff pairs rows by id only, while ReplaceForEntity (via
//     matchOverrides) also reuses an existing row by its (field, category,
//     collection) key when the incoming row has no id. Before the fix, a
//     row the save actually kept — resent without an id, the shape sent by
//     the raw PUT …/models/{id}/overrides and …/collections/{id}/overrides
//     routes (which decode straight from client JSON) and by ops/MCP/CLI
//     writers — logged as a delete-plus-create pair instead of one update.
//     (The editor itself round-trips real ids for every persisted row it
//     edits and mints negative temporary ids for new ones, so it never hit
//     this particular bug.)
//  2. "create" entries carried EntityID "0" because the diff ran before
//     the database assigned the new row's id. This one did affect the
//     editor.
//
// It sends two new rows in the same save — one with ID==0, one carrying an
// unknown non-zero id (also an insert) — because a single Added row can't
// catch a permutation bug in the correspondence between diff.AddedIdx and
// diff.Added (fix round 1, finding 3).
func TestSaveForEntityLogsKeptRowAsChangedAndNewRowWithRealID(t *testing.T) {
	pool := testdb.Pool(t)
	store := NewPostgresStore(pool)
	runner := &testChangeLogRunner{pool: pool}
	svc := NewService(store, nil, runner)
	ctx := writeCtx()
	cleanupCLEntity(t, pool)

	f5 := laFieldID(t, pool, "LAF.5")
	f6 := laFieldID(t, pool, "LAF.6")

	initial := []domain.FieldOverride{
		{FieldID: f5, ProjectID: clTestProjectID, Position: 1, DisplayName: domain.Translations{"en": "Name Type"}},
		{FieldID: f6, ProjectID: clTestProjectID, Position: 2, DisplayName: domain.Translations{"en": "Name Value"}},
	}
	if _, _, err := svc.SaveForEntity(ctx, clTestProjectID, clTestEntityType, clTestEntityID, initial, "initial save"); err != nil {
		t.Fatalf("initial save: %v", err)
	}
	id5 := initial[0].ID
	if id5 == 0 {
		t.Fatal("expected initial save to assign an id")
	}

	// Resend without ids, as the raw PUT route / ops-MCP-CLI writers would:
	// the f5 row keeps its content except for a changed display name, the
	// f6 row is unchanged, and two genuinely new rows are added — one
	// id-less, one carrying an id that names nothing in the existing set.
	const unknownID int64 = 999999999
	resend := []domain.FieldOverride{
		{FieldID: f5, ProjectID: clTestProjectID, Position: 1, DisplayName: domain.Translations{"en": "Name Type Changed"}},
		{FieldID: f6, ProjectID: clTestProjectID, Position: 2, DisplayName: domain.Translations{"en": "Name Value"}},
		{FieldID: f6, ProjectID: clTestProjectID, CategoryID: "TSTCHANGELOGCATA", Position: 3},
		{ID: unknownID, FieldID: f5, ProjectID: clTestProjectID, CategoryID: "TSTCHANGELOGCATB", Position: 4},
	}
	if _, _, err := svc.SaveForEntity(ctx, clTestProjectID, clTestEntityType, clTestEntityID, resend, "second save"); err != nil {
		t.Fatalf("second save: %v", err)
	}
	newRowIDA, newRowIDB := resend[2].ID, resend[3].ID
	if newRowIDA == 0 || newRowIDB == 0 {
		t.Fatalf("expected both new rows to get assigned ids, got %d and %d", newRowIDA, newRowIDB)
	}
	if newRowIDB == unknownID {
		t.Fatalf("expected the unknown-id row to get a fresh id, still carries the sent id %d", unknownID)
	}

	entries, err := sqlcgen.New(pool).WeaveListChangeLogForChangeSet(context.Background(), runner.lastChangeSetID)
	if err != nil {
		t.Fatalf("list change log: %v", err)
	}

	var updates, deletes int
	createdIDs := map[string]bool{}
	createdPayloads := map[string]json.RawMessage{}
	var updateEntry *sqlcgen.WeaveChangeLog
	for i := range entries {
		e := &entries[i]
		switch e.Operation {
		case "update":
			updates++
			updateEntry = e
		case "create":
			createdIDs[e.EntityID] = true
			createdPayloads[e.EntityID] = e.Payload
		case "delete":
			deletes++
		}
	}

	if updates != 1 {
		t.Fatalf("update entries = %d, want 1 (the f5 display-name change)", updates)
	}
	if deletes != 0 {
		t.Errorf("delete entries = %d, want 0 — the id-less f5/f6 rows were kept, not dropped", deletes)
	}

	wantCreated := map[string]bool{
		fmt.Sprintf("%d", newRowIDA): true,
		fmt.Sprintf("%d", newRowIDB): true,
	}
	if len(createdIDs) != len(wantCreated) {
		t.Fatalf("create entries logged ids %v, want exactly %v", setKeys(createdIDs), setKeys(wantCreated))
	}
	for id := range wantCreated {
		if !createdIDs[id] {
			t.Errorf("no create entry logged for new row id %s", id)
		}
	}
	for id := range createdIDs {
		if !wantCreated[id] {
			t.Errorf("create entry logged for unexpected id %s (want the two new rows' own ids, not a placeholder)", id)
		}
	}

	// Matching the two ids as a SET cannot tell an entry carrying its own
	// row's content from one carrying the other's: swapping them is a
	// symmetry of that set. The two new rows sit in different categories,
	// so pair each logged id with the content it claims to describe.
	for id, wantCategory := range map[string]string{
		fmt.Sprintf("%d", newRowIDA): "TSTCHANGELOGCATA",
		fmt.Sprintf("%d", newRowIDB): "TSTCHANGELOGCATB",
	} {
		payload, ok := createdPayloads[id]
		if !ok {
			continue // already reported above
		}
		var logged struct {
			CategoryID string `json:"category_id"`
		}
		if err := json.Unmarshal(payload, &logged); err != nil {
			t.Fatalf("decode create payload for id %s: %v", id, err)
		}
		if logged.CategoryID != wantCategory {
			t.Errorf("create entry for id %s carries category %q, want %q — the two new rows' ids were paired with each other's content",
				id, logged.CategoryID, wantCategory)
		}
	}

	// The update entry must name the row that was actually kept, not just
	// "some id that isn't zero" — and its payload/previous_payload must
	// reflect the actual before/after content, not e.g. both sides equal
	// to the after-state (which would still pass a weaker "isn't empty"
	// check but would misreport what changed).
	wantUpdateID := fmt.Sprintf("%d", id5)
	if updateEntry == nil {
		t.Fatal("no update entry logged")
	}
	if updateEntry.EntityID != wantUpdateID {
		t.Errorf("update entry EntityID = %q, want %q (the kept f5 row)", updateEntry.EntityID, wantUpdateID)
	}
	var before, after struct {
		DisplayName domain.Translations `json:"display_name"`
	}
	if err := json.Unmarshal(updateEntry.PreviousPayload, &before); err != nil {
		t.Fatalf("decode previous_payload: %v", err)
	}
	if err := json.Unmarshal(updateEntry.Payload, &after); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if before.DisplayName.Get("en") != "Name Type" {
		t.Errorf("update previous_payload display_name = %q, want %q", before.DisplayName.Get("en"), "Name Type")
	}
	if after.DisplayName.Get("en") != "Name Type Changed" {
		t.Errorf("update payload display_name = %q, want %q", after.DisplayName.Get("en"), "Name Type Changed")
	}
}

func setKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
