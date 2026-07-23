package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/pletka-io/pletka/pkg/app/cliruntime"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

var (
	rbVersion     string
	rbTitle       string
	rbDescription string
	rbActorID     string
	rbDryRun      bool
	rbMaterialize bool
)

func newReleaseBaselineCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release-baseline",
		Short: "Release every draft project as a baseline version (one-shot, idempotent)",
		Long: `Releases every project that has no release yet as a single baseline version,
in parent-first dependency order. For each project it first re-pins any parent
links that still follow draft onto the baseline version, then cuts the release
(which snapshots the project's entities into immutable (id, version_number)
rows). Already-released projects are skipped, so the command is safe to re-run.

Once cross-project parents are pinned to a baseline release, consumers read an
immutable snapshot instead of the source project's live draft — which is what
makes deletion and lifecycle gating behave sanely. This fixes the all-draft
state left by the Airtable migration.

The command runs as a system super-admin. Use --dry-run to print the plan
without writing anything.`,
		RunE: runReleaseBaseline,
	}
	cmd.Flags().StringVar(&rbVersion, "version", "0.1.0", "baseline release version (MAJOR.MINOR.PATCH)")
	cmd.Flags().StringVar(&rbTitle, "title", "First release after Airtable migration", "release title")
	cmd.Flags().StringVar(&rbDescription, "description", "Baseline release capturing the state migrated from Airtable.", "release description")
	cmd.Flags().StringVar(&rbActorID, "actor-id", "", "super-admin actor id for provenance (default: first super_admin actor)")
	cmd.Flags().BoolVar(&rbDryRun, "dry-run", false, "print the plan without writing anything")
	cmd.Flags().BoolVar(&rbMaterialize, "materialize", true, "run git materialization once at the end")
	return cmd
}

func runReleaseBaseline(cmd *cobra.Command, _ []string) error {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx := context.Background()

	pool, err := cliruntime.OpenPGXPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	actorID, err := resolveBaselineActor(ctx, pool, rbActorID)
	if err != nil {
		return err
	}
	// System super-admin context: IsSuperAdmin short-circuits every Can()
	// check, so release.Create's requireEdit passes for every project without
	// per-project resource wiring. The principal supplies created_by_id.
	ctx = auth.WithSnapshot(ctx, &auth.AuthSnapshot{ActorID: actorID, IsSuperAdmin: true})
	ctx = auth.WithPrincipal(ctx, &auth.Principal{ActorID: actorID, Role: "super_admin", IsActive: true})

	order, blocked, err := baselineReleaseOrder(ctx, pool, rbVersion)
	if err != nil {
		return err
	}
	if len(order) == 0 && len(blocked) == 0 {
		fmt.Println("Nothing to do — every project already has a", rbVersion, "release.")
		return nil
	}

	fmt.Printf("Baseline release %s as %q (actor %s)\n", rbVersion, rbTitle, actorID)
	fmt.Printf("Release order (parents first): %v\n", order)
	if len(blocked) > 0 {
		fmt.Printf("\nBLOCKED (inheritance cycle or downstream of one): %v\n", blocked)
		fmt.Println("These cannot be released parents-first. Break the cycle (adjust an")
		fmt.Println("inheritance link) and re-run — the rest will proceed regardless.")
	}
	fmt.Println()
	if rbDryRun {
		fmt.Println("--dry-run: no changes written.")
		return nil
	}

	svc := release.NewService(release.NewPostgresStore(pool), pool, log)
	released := 0
	for _, projectID := range order {
		// Re-pin any draft parent links onto the baseline version. Every parent
		// was released earlier in this pass (parent-first order), so the pinned
		// version exists. ponytail: uniform rbVersion — a baseline release
		// gives every project the same version, so a child's parents all sit at
		// rbVersion.
		if _, err := pool.Exec(ctx, `
			UPDATE weave_project_inheritance
			SET source_mode = 'release', source_version = $2
			WHERE project_id = $1 AND source_mode = 'draft'`, projectID, rbVersion); err != nil {
			return fmt.Errorf("re-pin parents of %s: %w", projectID, err)
		}

		if _, err := svc.Create(ctx, projectID, release.CreateInput{
			Version:     rbVersion,
			Title:       rbTitle,
			Description: rbDescription,
		}); err != nil {
			var conflict *release.ErrConflict
			if errors.As(err, &conflict) {
				fmt.Printf("  %s: already released, skipped\n", projectID)
				continue
			}
			return fmt.Errorf("release %s: %w", projectID, err)
		}
		released++
		fmt.Printf("  %s: released %s\n", projectID, rbVersion)
	}

	fmt.Printf("\nReleased %d project(s).\n", released)

	if rbMaterialize {
		baseDir := cliruntime.GitDataDirFromViper()
		if baseDir == "" {
			baseDir = "./data/git-projects"
		}
		mat := gitmaterializer.NewMaterializer(pool, baseDir, log).
			WithModuleHosts(cliruntime.ModuleHostFromViper(), cliruntime.OntologyHostFromViper())
		n, err := mat.ProcessPending(ctx, 100000)
		if err != nil {
			return fmt.Errorf("materialize baseline: %w", err)
		}
		fmt.Printf("Materialized %d change set(s) to git.\n", n)
	}
	return nil
}

// resolveBaselineActor returns the requested actor id, or the first super_admin
// actor when none is given.
func resolveBaselineActor(ctx context.Context, pool *pgxpool.Pool, requested string) (string, error) {
	if requested != "" {
		return requested, nil
	}
	var id string
	err := pool.QueryRow(ctx, `SELECT id FROM weave_actors WHERE role = 'super_admin' ORDER BY id LIMIT 1`).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("find a super_admin actor (pass --actor-id): %w", err)
	}
	return id, nil
}

// baselineReleaseOrder returns the projects still needing the baseline release,
// topologically sorted parents-first over the inheritance graph, plus the set
// that cannot be ordered (part of a dependency cycle, or downstream of one).
// Projects that already have the version are excluded.
func baselineReleaseOrder(ctx context.Context, pool *pgxpool.Pool, version string) (order []string, blocked []string, err error) {
	// Pending = draft-shell projects without this release yet.
	rows, err := pool.Query(ctx, `
		SELECT p.id
		FROM weave_projects p
		WHERE p.version_number = ''
		  AND NOT EXISTS (
		    SELECT 1 FROM weave_releases r WHERE r.project_id = p.id AND r.version = $1
		  )`, version)
	if err != nil {
		return nil, nil, fmt.Errorf("list pending projects: %w", err)
	}
	pending := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, nil, err
		}
		pending[id] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	// Parents per pending child, restricted to other pending projects. A parent
	// outside the pending set is already released → treated as satisfied.
	parents := map[string]map[string]bool{}
	for id := range pending {
		parents[id] = map[string]bool{}
	}
	erows, err := pool.Query(ctx, `SELECT project_id, parent_project_id FROM weave_project_inheritance`)
	if err != nil {
		return nil, nil, fmt.Errorf("list inheritance edges: %w", err)
	}
	for erows.Next() {
		var child, parent string
		if err := erows.Scan(&child, &parent); err != nil {
			erows.Close()
			return nil, nil, err
		}
		if pending[child] && pending[parent] {
			parents[child][parent] = true
		}
	}
	erows.Close()
	if err := erows.Err(); err != nil {
		return nil, nil, err
	}

	// Kahn's algorithm: emit a project once all its pending parents are emitted.
	// Whatever can't progress is part of a cycle or downstream of one.
	remaining := len(pending)
	for remaining > 0 {
		progressed := false
		for id := range pending {
			if pending[id] && len(parents[id]) == 0 {
				order = append(order, id)
				pending[id] = false
				remaining--
				for _, ps := range parents {
					delete(ps, id)
				}
				progressed = true
			}
		}
		if !progressed {
			for id, still := range pending {
				if still {
					blocked = append(blocked, id)
				}
			}
			break
		}
	}
	return order, blocked, nil
}
