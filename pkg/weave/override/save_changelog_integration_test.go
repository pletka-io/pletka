//go:build integration

package override

import (
	"context"
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

type testChangeLogRecorder struct {
	entries []domain.ChangeLogEntry
}

func (r *testChangeLogRecorder) Record(_ context.Context, entry domain.ChangeLogEntry) error {
	r.entries = append(r.entries, entry)
	return nil
}

// writeCtx carries a super-admin snapshot so SaveForEntity's ProjectEdit
// gate passes without needing a real membership row.
func writeCtx() context.Context {
	return weaveauth.WithSnapshot(context.Background(), &weaveauth.AuthSnapshot{IsSuperAdmin: true})
}

// TestSaveForEntityLogsKeptRowAsChangedAndNewRowWithRealID covers the two
// change-log bugs Task 4 fixes:
//
//  1. ComputeDiff pairs rows by id only, while ReplaceForEntity (via
//     matchOverrides) also reuses an existing row by its (field, category,
//     collection) key when the incoming row has no id. Before the fix, a
//     row the save actually kept — resent without an id, as the editor's
//     own payload shape does — logged as a delete-plus-create pair instead
//     of one update.
//  2. "create" entries carried EntityID "0" because the diff ran before
//     the database assigned the new row's id.
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

	// Resend as an id-less editor payload would: the f5 row keeps its
	// content except for a changed display name, the f6 row is unchanged,
	// and a genuinely new row (f6 again, under a different category —
	// no existing row shares that key) is added.
	resend := []domain.FieldOverride{
		{FieldID: f5, ProjectID: clTestProjectID, Position: 1, DisplayName: domain.Translations{"en": "Name Type Changed"}},
		{FieldID: f6, ProjectID: clTestProjectID, Position: 2, DisplayName: domain.Translations{"en": "Name Value"}},
		{FieldID: f6, ProjectID: clTestProjectID, CategoryID: "TSTCHANGELOGCAT", Position: 3},
	}
	if _, _, err := svc.SaveForEntity(ctx, clTestProjectID, clTestEntityType, clTestEntityID, resend, "second save"); err != nil {
		t.Fatalf("second save: %v", err)
	}
	newRowID := resend[2].ID
	if newRowID == 0 {
		t.Fatal("expected the genuinely new row to get an assigned id")
	}

	entries, err := sqlcgen.New(pool).WeaveListChangeLogForChangeSet(context.Background(), runner.lastChangeSetID)
	if err != nil {
		t.Fatalf("list change log: %v", err)
	}

	var updates, creates, deletes int
	for _, e := range entries {
		switch e.Operation {
		case "update":
			updates++
			if e.EntityID == "0" {
				t.Errorf("update entry carries EntityID %q", e.EntityID)
			}
		case "create":
			creates++
			if e.EntityID == "0" {
				t.Errorf("create entry carries EntityID %q, want the new row's real id", e.EntityID)
			}
		case "delete":
			deletes++
		}
	}

	if updates != 1 {
		t.Errorf("update entries = %d, want 1 (the f5 display-name change)", updates)
	}
	if deletes != 0 {
		t.Errorf("delete entries = %d, want 0 — the id-less f5/f6 rows were kept, not dropped", deletes)
	}
	if creates != 1 {
		t.Errorf("create entries = %d, want 1 (the genuinely new row)", creates)
	}

	wantNewRowID := fmt.Sprintf("%d", newRowID)
	var newRowLogged bool
	for _, e := range entries {
		if e.Operation == "create" {
			if e.EntityID != wantNewRowID {
				t.Errorf("create entry EntityID = %q, want %q", e.EntityID, wantNewRowID)
			}
			newRowLogged = true
		}
	}
	if !newRowLogged {
		t.Fatal("no create entry found for the new row")
	}
}
