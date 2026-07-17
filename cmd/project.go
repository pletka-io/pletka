package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/pletka-io/pletka/pkg/app"
	"github.com/pletka-io/pletka/pkg/app/cliruntime"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
)

func newProjectCommand() *cobra.Command {
	projectCmd := &cobra.Command{
		Use:   "project",
		Short: "Project management commands",
	}

	initGitCmd := &cobra.Command{
		Use:   "init-git <projectID>",
		Short: "Initialize a git repository from a project's current database state",
		Long: `Walks all entities in a project (categories, fields, models, collections and
their overrides) and writes them as canonical YAML into a per-project working
tree. Produces a single "Initial import" git commit.

This bypasses the change_log outbox — it is a one-shot bootstrap for a project
that already has data in the database but has not yet been materialized to git.`,
		Args: cobra.ExactArgs(1),
		RunE: runInitGit,
	}

	loadGitCmd := &cobra.Command{
		Use:   "load-git <snapshotDir>",
		Short: "Hydrate a project snapshot from a git-materialized directory back into the database",
		Long: `Loads a git-materialized project snapshot from disk and restores its project
shell, inheritance, ontology links, namespaces, local entities, overrides,
adoption receipts, and fork receipts into the database.

This is the reverse path of project init-git and is intended for restore and
round-trip verification work.`,
		Args: cobra.ExactArgs(1),
		RunE: runLoadGit,
	}

	projectCmd.AddCommand(initGitCmd)
	projectCmd.AddCommand(loadGitCmd)
	projectCmd.AddCommand(newReleaseBaselineCommand())
	initGitCmd.Flags().StringVar(&initGitOutputDir, "output-dir", "",
		"directory for the project working tree (default: ./data/git-projects/<projectID>)")
	initGitCmd.Flags().BoolVar(&initGitSelfContained, "self-contained", false,
		"include vendor/ snapshots for parent projects and linked ontology sources")
	return projectCmd
}

var initGitOutputDir string
var initGitSelfContained bool

func runInitGit(cmd *cobra.Command, args []string) error {
	projectID := args[0]

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
	if initGitOutputDir != "" {
		baseDir = initGitOutputDir
	}

	mat := gitmaterializer.NewMaterializer(pool, baseDir, log)
	if err := mat.InitProjectWithOptions(ctx, projectID, gitmaterializer.InitProjectOptions{
		SelfContained: initGitSelfContained,
	}); err != nil {
		return fmt.Errorf("init-git: %w", err)
	}

	fmt.Printf("Project %s initialized at %s/%s\n", projectID, baseDir, projectID)
	return nil
}

func runLoadGit(cmd *cobra.Command, args []string) error {
	rootDir := args[0]

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx := context.Background()
	pool, err := cliruntime.OpenPGXPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	ontologyStore := weaveontology.NewPostgresStore(pool)
	ontologySvc := weaveontology.NewService(ontologyStore, nil, log)

	mat := gitmaterializer.NewMaterializer(pool, "", log).
		WithOntologyImporter(app.NewOntologyVendorImporter(ontologySvc))
	plan, err := mat.HydrateProjectSnapshot(ctx, rootDir)
	if err != nil {
		return fmt.Errorf("load-git: %w", err)
	}

	fmt.Printf("Project %s hydrated from %s\n", plan.ProjectID, rootDir)
	return nil
}
