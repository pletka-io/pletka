package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/pletka-io/pletka/pkg/app/cliruntime"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

var releaseArchiveMessage string

func newReleaseCommand() *cobra.Command {
	releaseCmd := &cobra.Command{
		Use:   "release",
		Short: "Release lifecycle operations",
	}

	archiveCmd := &cobra.Command{
		Use:   "archive <projectID> <version>",
		Short: "Archive a project release",
		Long: `Marks a release as archived: stamps archived_at/archived_message on the
immutable weave_releases row and enqueues a release_archived change set for
the git materializer. Releases are otherwise immutable — archiving is the
only lifecycle change.`,
		Args: cobra.ExactArgs(2),
		RunE: runReleaseArchive,
	}
	archiveCmd.Flags().StringVarP(&releaseArchiveMessage, "message", "m", "", "reason for archiving (required)")
	if err := archiveCmd.MarkFlagRequired("message"); err != nil {
		panic(err)
	}

	materializeMissingCmd := &cobra.Command{
		Use:   "materialize-missing",
		Short: "Backfill git tags for releases that predate the outbox change-set kind",
		Long: `Walks weave_releases and enqueues a 'release' change set for every version
whose tag is absent from its project's git repo, then drains the outbox
until fully processed. Idempotent: a version already tagged, or with an
unprocessed change set already queued, is skipped rather than re-enqueued.`,
		Args: cobra.NoArgs,
		RunE: runReleaseMaterializeMissing,
	}

	releaseCmd.AddCommand(archiveCmd)
	releaseCmd.AddCommand(materializeMissingCmd)
	return releaseCmd
}

func runReleaseArchive(cmd *cobra.Command, args []string) error {
	projectID, version := args[0], args[1]

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx := context.Background()

	pool, err := cliruntime.OpenPGXPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	actorID, err := resolveBaselineActor(ctx, pool, "")
	if err != nil {
		return err
	}
	// System super-admin context: IsSuperAdmin short-circuits every Can()
	// check, so Archive's requireEdit passes without per-project resource
	// wiring. The principal supplies the change-set actor_id.
	ctx = auth.WithSnapshot(ctx, &auth.AuthSnapshot{ActorID: actorID, IsSuperAdmin: true})
	ctx = auth.WithPrincipal(ctx, &auth.Principal{ActorID: actorID, Role: "super_admin", IsActive: true})

	svc := release.NewService(release.NewPostgresStore(pool), pool, log)
	archived, err := svc.Archive(ctx, projectID, version, release.ArchiveInput{Message: releaseArchiveMessage})
	if err != nil {
		var verr *release.ErrValidation
		if errors.As(err, &verr) {
			return fmt.Errorf("validation error: %s", formatValidationFields(verr.Fields))
		}
		return err
	}

	fmt.Printf("Archived %s@%s at %s\n", projectID, version, archived.ArchivedAt)
	return nil
}

// formatValidationFields renders a release.ErrValidation's Fields map as
// "field: msg1; msg2, field2: msg3", sorted by field name so CLI output is
// deterministic across runs (map iteration order is not) — e.g. "message: an
// archive message is required".
func formatValidationFields(fields map[string][]string) string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s: %s", name, strings.Join(fields[name], "; ")))
	}
	return strings.Join(parts, ", ")
}

func runReleaseMaterializeMissing(cmd *cobra.Command, _ []string) error {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx := context.Background()

	pool, err := cliruntime.OpenPGXPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	baseDir := cliruntime.GitDataDirFromViper()
	if baseDir == "" {
		baseDir = "./data/git-projects"
	}

	mat := gitmaterializer.NewMaterializer(pool, baseDir, log)
	enq, skip, err := mat.EnqueueMissingReleases(ctx)
	if err != nil {
		return fmt.Errorf("enqueue missing releases: %w", err)
	}

	// Drain with a progress guard rather than looping on ProcessPending's
	// returned count alone: a persistently failing release change set is
	// recorded via recordReleaseFailure (processed_at stays NULL so the
	// poller retries it — deliberate server-side retry semantics, not a
	// bug), so ProcessPending keeps counting it as "processed this pass"
	// forever and `n == 0` would never be reached. Track the remaining
	// unprocessed release-kind count instead: it must strictly decrease
	// pass over pass, or the drain is stuck and we stop and report it.
	start, err := countUnprocessedReleaseChangeSets(ctx, pool)
	if err != nil {
		return fmt.Errorf("count unprocessed release change sets: %w", err)
	}
	remaining := start
	for remaining > 0 {
		if _, err := mat.ProcessPending(ctx, 100); err != nil {
			return fmt.Errorf("materialize backfilled releases: %w", err)
		}
		next, err := countUnprocessedReleaseChangeSets(ctx, pool)
		if err != nil {
			return fmt.Errorf("count unprocessed release change sets: %w", err)
		}
		if !backfillProgressed(remaining, next) {
			remaining = next
			break
		}
		remaining = next
	}

	fmt.Printf("enqueued %d, skipped %d (tags already present)\n", enq, skip)
	fmt.Printf("drained %d, stuck %d\n", start-remaining, remaining)
	if remaining > 0 {
		fmt.Println("drain made no progress on a full pass — inspect materialized_error/materialized_outcome on weave_change_set for the stuck row(s)")
	}
	return nil
}

// backfillProgressed reports whether a materialize-missing drain pass made
// progress: the remaining unprocessed release-kind change-set count must have
// strictly decreased. Equal or increased counts mean the drain is stuck (see
// runReleaseMaterializeMissing for why ProcessPending's return count alone
// can't signal this). A pure decision function so it's unit-testable without
// a database.
func backfillProgressed(prevRemaining, currRemaining int) bool {
	return currRemaining < prevRemaining
}

// countUnprocessedReleaseChangeSets counts enqueued (closed) but not yet
// materialized 'release'/'release_archived' change sets across all projects —
// the materialize-missing drain-progress signal.
func countUnprocessedReleaseChangeSets(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT count(*) FROM weave_change_set
		WHERE processed_at IS NULL AND closed_at IS NOT NULL
		  AND kind IN ('release', 'release_archived')
	`).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
}
