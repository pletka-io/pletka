package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pletka-io/pletka/pkg/app/cliruntime"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
	"github.com/pletka-io/pletka/pkg/weave/ontology/autocomplete"
)

const replayFormatJSON = "json"

var weaveReplayPathsOpts struct {
	projects string
	format   string
}

func newWeaveReplayPathsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "replay-paths --project <id>[,<id>...]",
		Short: "Rebuild every stored field path through the path builder's engine and report steps it would not offer",
		Long: `replay-paths asks the autocomplete engine, step by step, the same questions the
path builder asks while a curator builds a path: is the scope class offered,
is the first property offered on that class, is the next class offered as the
property's range, and so on to the terminal. A stored path whose step the
engine does not offer cannot be rebuilt in the UI, whatever verify-paths says
about its terms existing.

Requests mirror the builder: current_path = scope class followed by the steps
so far, scope_class = the scope, parent projects included, range suggestions
included, query = the local name of the step being looked for.`,
		RunE: runWeaveReplayPaths,
	}
	cmd.Flags().StringVar(&weaveReplayPathsOpts.projects, "project", "", "project id, or several separated by commas (required)")
	cmd.Flags().StringVar(&weaveReplayPathsOpts.format, "format", "text", "output format: text or json")
	cmd.SilenceUsage = true
	return cmd
}

// replayFinding is a step the engine did not offer.
type replayFinding struct {
	Project    string   `json:"project"`
	SemanticID string   `json:"semantic_id"`
	Position   int      `json:"position"`
	Want       string   `json:"want"`
	Offered    []string `json:"offered"`
}

func pathElementQname(pe domain.PathElement) string {
	return strings.TrimSpace(pe.Prefix) + ":" + strings.TrimSpace(pe.LocalName)
}

func runWeaveReplayPaths(cmd *cobra.Command, args []string) error {
	projects := strings.Split(strings.TrimSpace(weaveReplayPathsOpts.projects), ",")
	if len(projects) == 0 || projects[0] == "" {
		return fmt.Errorf("--project is required")
	}
	switch weaveReplayPathsOpts.format {
	case "text", replayFormatJSON:
	default:
		return fmt.Errorf("invalid --format %q: want text or json", weaveReplayPathsOpts.format)
	}
	ctx := context.Background()
	pool, err := cliruntime.OpenPingedPGXPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	quiet := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	svc := weaveontology.NewService(weaveontology.NewPostgresStore(pool), weave.NewPostgresStore(pool).Projects(), quiet)
	queries := sqlcgen.New(pool)

	var findings []replayFinding
	checked, fieldsSeen := 0, 0
	for _, projectID := range projects {
		projectID = strings.TrimSpace(projectID)
		rows, err := queries.WeaveListFields(ctx, sqlcgen.WeaveListFieldsParams{ProjectID: projectID, ResultLimit: 1 << 20})
		if err != nil {
			return fmt.Errorf("list fields of %s: %w", projectID, err)
		}
		for _, row := range rows {
			fieldFindings, steps, err := replayField(ctx, svc.GetSuggestions, projectID, row)
			if err != nil {
				return err
			}
			if steps == 0 {
				continue
			}
			fieldsSeen++
			checked += steps
			findings = append(findings, fieldFindings...)
		}
	}
	if err := printReplay(weaveReplayPathsOpts.format, fieldsSeen, checked, findings); err != nil {
		return err
	}
	if len(findings) > 0 {
		return fmt.Errorf("%d path steps the builder would not offer", len(findings))
	}
	return nil
}

// replayField asks the engine every question the builder would ask for one
// stored field. Returns the steps not offered and the number of steps asked
// (0 for a field without a scope or path).
func replayField(ctx context.Context, suggest autocomplete.Suggester, projectID string, row sqlcgen.WeaveListFieldsRow) ([]replayFinding, int, error) {
	var scope domain.PathElement
	var elems []domain.PathElement
	if len(row.OntologyScope) > 0 {
		if err := json.Unmarshal(row.OntologyScope, &scope); err != nil {
			return nil, 0, fmt.Errorf("field %s: decode scope: %w", row.ID, err)
		}
	}
	if len(row.PathElements) > 0 {
		if err := json.Unmarshal(row.PathElements, &elems); err != nil {
			return nil, 0, fmt.Errorf("field %s: decode path: %w", row.ID, err)
		}
	}
	if scope.LocalName == "" || len(elems) == 0 {
		return nil, 0, nil
	}
	path := make([]string, 0, len(elems))
	uris := make([]string, 0, len(elems))
	for _, pe := range elems {
		path = append(path, pathElementQname(pe))
		uris = append(uris, pe.URI)
	}
	misses, steps, err := autocomplete.ReplayPath(ctx, suggest, projectID, pathElementQname(scope), scope.URI, path, uris)
	if err != nil {
		return nil, 0, fmt.Errorf("field %s: %w", row.ID, err)
	}
	findings := make([]replayFinding, 0, len(misses))
	for _, m := range misses {
		findings = append(findings, replayFinding{Project: projectID, SemanticID: derefSemanticID(row.SemanticID, row.ID), Position: m.Position, Want: m.Want, Offered: m.Offered})
	}
	return findings, steps, nil
}

func printReplay(format string, fieldsSeen, checked int, findings []replayFinding) error {
	if format == replayFormatJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"fields": fieldsSeen, "steps": checked, "findings": findings})
	}
	for _, f := range findings {
		where := "scope picker"
		if f.Position >= 0 {
			where = fmt.Sprintf("step %d", f.Position)
		}
		fmt.Printf("%-8s %-12s %-12s %-40s not offered (%d suggestions)\n", f.Project, f.SemanticID, where, f.Want, len(f.Offered))
	}
	fmt.Printf("\nFields: %d  Steps: %d  Not buildable: %d\n", fieldsSeen, checked, len(findings))
	return nil
}

func derefSemanticID(sem *string, id string) string {
	if sem != nil && *sem != "" {
		return *sem
	}
	return id
}
