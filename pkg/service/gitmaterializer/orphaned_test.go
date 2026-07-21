//go:build integration

package gitmaterializer_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
)

// Change sets with an empty or since-deleted project_id (implicit CLI/import
// mutations, deleted projects) must be marked processed as 'orphaned' instead
// of being retried forever.
func TestProcessPending_OrphanedChangeSets(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	var emptyID, goneID int64
	for _, tc := range []struct {
		projectID string
		dst       *int64
	}{
		{"", &emptyID},
		{"ZZZ_NO_SUCH_PROJECT", &goneID},
	} {
		// closed_at far in the past so ProcessPending (ORDER BY closed_at
		// ASC, LIMIT 2) picks exactly these two and leaves the shared dev
		// DB's real pending queue untouched.
		if err := pool.QueryRow(ctx, `
			INSERT INTO weave_change_set (project_id, actor_name, actor_email, commit_message, closed_at)
			VALUES ($1, 'test', 'test@pletka.local', 'orphan test', '2000-01-01')
			RETURNING id`, tc.projectID).Scan(tc.dst); err != nil {
			t.Fatalf("insert change set (%q): %v", tc.projectID, err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_change_set WHERE id IN ($1, $2)`, emptyID, goneID)
	})

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	mat := gitmaterializer.NewMaterializer(pool, t.TempDir(), logger)
	if _, err := mat.ProcessPending(ctx, 2); err != nil {
		t.Fatalf("ProcessPending: %v", err)
	}

	for _, id := range []int64{emptyID, goneID} {
		var processed bool
		var outcome *string
		if err := pool.QueryRow(ctx, `
			SELECT processed_at IS NOT NULL, materialized_outcome
			FROM weave_change_set WHERE id = $1`, id).Scan(&processed, &outcome); err != nil {
			t.Fatalf("read back change set %d: %v", id, err)
		}
		if !processed {
			t.Errorf("change set %d: still unprocessed — will retry forever", id)
		}
		if outcome == nil || *outcome != "orphaned" {
			got := "<nil>"
			if outcome != nil {
				got = *outcome
			}
			t.Errorf("change set %d: outcome = %s, want orphaned", id, got)
		}
	}
}
