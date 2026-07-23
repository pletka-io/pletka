package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"

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
		return err
	}

	fmt.Printf("Archived %s@%s at %s\n", projectID, version, archived.ArchivedAt)
	return nil
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

	for {
		n, err := mat.ProcessPending(ctx, 100)
		if err != nil {
			return fmt.Errorf("materialize backfilled releases: %w", err)
		}
		if n == 0 {
			break
		}
	}

	fmt.Printf("enqueued %d, skipped %d (tags already present)\n", enq, skip)
	return nil
}
