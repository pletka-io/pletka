package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/pletka-io/pletka/pkg/app/cliruntime"
	"github.com/pletka-io/pletka/pkg/weave/project"
)

var projectDeleteCommit bool

func newProjectDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <projectID>",
		Short: "Delete a project and everything it owns (guarded, dry-run by default)",
		Long: `Deletes a project plus every row it owns: entities, overrides, categories,
concept lists, bindings, memberships, ontology links, change log/sets, release
archives, and the materialized git work tree.

Deletion is REFUSED while anything outside the project depends on it — child
projects (draft or release-pinned), adoption receipts in other projects, its
fields placed in other projects' containers, or value refs targeting its
entities. Deprecate the project (or untangle the dependents) instead.

Default is a dry-run report of what would be removed; pass --commit to write.`,
		Args: cobra.ExactArgs(1),
		RunE: runProjectDelete,
	}
	cmd.Flags().BoolVar(&projectDeleteCommit, "commit", false, "perform the deletion; without it the command only reports")
	return cmd
}

func runProjectDelete(cmd *cobra.Command, args []string) error {
	projectID := args[0]
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx := context.Background()

	pool, err := cliruntime.OpenPGXPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	store := project.NewPostgresStore(pool)
	svc := project.NewService(store, nil, nil, log)

	existing, err := store.GetByID(ctx, projectID)
	if err != nil {
		return fmt.Errorf("look up project %s: %w", projectID, err)
	}
	if existing == nil {
		return fmt.Errorf("project %s not found", projectID)
	}
	blockers, err := store.DeleteBlockers(ctx, projectID)
	if err != nil {
		return err
	}
	fmt.Printf("Project %s (%s)\n", projectID, existing.UIName.Get("en"))
	if blockers.Blocked() {
		fmt.Printf("BLOCKED: %s\n", blockers)
		fmt.Println("Deprecate the project or untangle the dependents first.")
		return fmt.Errorf("delete blocked")
	}
	fmt.Println("No external dependents — deletable.")

	if !projectDeleteCommit {
		fmt.Println("--commit not passed: dry-run, nothing deleted.")
		return nil
	}

	stats, err := svc.DeleteProject(ctx, projectID)
	if err != nil {
		var blocked *project.ErrDeleteBlocked
		if errors.As(err, &blocked) {
			return fmt.Errorf("%s", blocked.Error())
		}
		return err
	}
	fmt.Printf("Deleted: %d fields, %d models, %d collections, %d categories, %d overrides, %d archive rows, %d change-log rows, %d other rows.\n",
		stats.Fields, stats.Models, stats.Collections, stats.Categories,
		stats.Overrides, stats.ArchiveRows, stats.ChangeLogRows, stats.OtherRows)

	// Best-effort: remove the materialized git work tree.
	baseDir := cliruntime.GitDataDirFromViper()
	if baseDir == "" {
		baseDir = "./data/git-projects"
	}
	workDir := filepath.Join(baseDir, projectID)
	if _, err := os.Stat(workDir); err == nil {
		if err := os.RemoveAll(workDir); err != nil {
			fmt.Printf("WARN: could not remove work tree %s: %v\n", workDir, err)
		} else {
			fmt.Printf("Removed work tree %s\n", workDir)
		}
	}
	return nil
}
