package weave

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	return testdb.Pool(t)
}

func TestWithChangeLog_ImplicitChangeSet(t *testing.T) {
	pool := testPool(t)
	store := NewPostgresStore(pool)
	ctx := context.Background()

	var capturedCSID int64
	err := store.WithChangeLog(ctx, func(cl *ChangeLogger) error {
		capturedCSID = cl.ChangeSetID()
		return nil
	})
	if err != nil {
		t.Fatalf("WithChangeLog: %v", err)
	}
	if capturedCSID == 0 {
		t.Errorf("expected non-zero change set id")
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_change_set WHERE id = $1`, capturedCSID)
	})
}

func TestWithChangeLog_BackfillsImplicitProject(t *testing.T) {
	pool := testPool(t)
	store := NewPostgresStore(pool)
	ctx := context.Background()

	var csID int64
	err := store.WithChangeLog(ctx, func(cl *ChangeLogger) error {
		csID = cl.ChangeSetID()
		// An implicit (empty-project) change set with an entry that carries a
		// real project should adopt that project, so the materializer can route
		// it instead of failing forever on WeaveGetProjectByID("").
		return cl.Record(domain.ChangeLogEntry{
			EntityType: "override",
			EntityID:   "TESTBF.1",
			Operation:  "create",
			ProjectID:  "TESTPUB_BACKFILL",
			FilePath:   "overrides/TESTBF.1.yaml",
			Payload:    []byte("{}"),
		})
	})
	if err != nil {
		t.Fatalf("WithChangeLog: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_change_log WHERE change_set_id = $1`, csID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_change_set WHERE id = $1`, csID)
	})

	var got string
	if err := pool.QueryRow(ctx, `SELECT project_id FROM weave_change_set WHERE id = $1`, csID).Scan(&got); err != nil {
		t.Fatalf("query change set: %v", err)
	}
	if got != "TESTPUB_BACKFILL" {
		t.Errorf("change set project_id: got %q want TESTPUB_BACKFILL", got)
	}
}

func TestWithChangeLog_ExplicitChangeSet(t *testing.T) {
	pool := testPool(t)
	store := NewPostgresStore(pool)
	ctx := context.Background()

	row, err := store.queries.WeaveCreateChangeSet(ctx, sqlcgen.WeaveCreateChangeSetParams{
		ProjectID:     "test-project",
		ActorName:     "Test User",
		ActorEmail:    "test@example.com",
		CommitMessage: "test change",
	})
	if err != nil {
		t.Fatalf("create change set: %v", err)
	}
	cs := &domain.ChangeSet{
		ID:            row.ID,
		ProjectID:     row.ProjectID,
		ActorName:     row.ActorName,
		ActorEmail:    row.ActorEmail,
		CommitMessage: row.CommitMessage,
	}
	ctx = domain.ContextWithChangeSet(ctx, cs)

	err = store.WithChangeLog(ctx, func(cl *ChangeLogger) error {
		if cl.ChangeSetID() != cs.ID {
			t.Errorf("expected change set id %d, got %d", cs.ID, cl.ChangeSetID())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithChangeLog: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_change_set WHERE id = $1`, cs.ID)
	})
}
