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

// replayStep is one question put to the engine while rebuilding a path.
type replayStep struct {
	Position    int      `json:"position"` // -1 = the scope class in the root picker
	Want        string   `json:"want"`     // qname the stored path has at this position
	WantURI     string   `json:"want_uri,omitempty"`
	CurrentPath []string `json:"current_path"`
}

// replayOffered reports whether the stored element is among the suggestions:
// by qname, or by IRI when the stored path carries another prefix for the
// same namespace than the one the builder shows.
func replayOffered(sugs []autocomplete.Suggestion, wantQname, wantURI string) bool {
	for _, s := range sugs {
		if s.Qname == wantQname {
			return true
		}
		if wantURI != "" && strings.Contains(wantURI, "://") && s.URI == wantURI {
			return true
		}
	}
	return false
}

// replayFinding is a step the engine did not offer.
type replayFinding struct {
	Project    string   `json:"project"`
	SemanticID string   `json:"semantic_id"`
	Position   int      `json:"position"`
	Want       string   `json:"want"`
	Offered    []string `json:"offered"`
}

// replaySteps lists the questions for one field: the scope class in the root
// picker, then each path element with the scope and the elements before it
// as current_path.
func replaySteps(scope string, path []string) []replayStep {
	steps := make([]replayStep, 0, 1+len(path))
	steps = append(steps, replayStep{Position: -1, Want: scope, CurrentPath: nil})
	for i, q := range path {
		cur := append([]string{scope}, path[:i]...)
		steps = append(steps, replayStep{Position: i, Want: q, CurrentPath: cur})
	}
	return steps
}

// withURIs attaches the stored IRIs to the steps built from qnames.
func withURIs(steps []replayStep, scope domain.PathElement, elems []domain.PathElement) []replayStep {
	for i := range steps {
		if steps[i].Position < 0 {
			steps[i].WantURI = scope.URI
		} else if steps[i].Position < len(elems) {
			steps[i].WantURI = elems[steps[i].Position].URI
		}
	}
	return steps
}

func pathElementQname(pe domain.PathElement) string {
	return strings.TrimSpace(pe.Prefix) + ":" + strings.TrimSpace(pe.LocalName)
}

func qnameLocal(q string) string {
	if i := strings.Index(q, ":"); i >= 0 {
		return q[i+1:]
	}
	return q
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
			fieldFindings, steps, err := replayField(ctx, svc, projectID, row)
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
func replayField(ctx context.Context, svc *weaveontology.Service, projectID string, row sqlcgen.WeaveListFieldsRow) ([]replayFinding, int, error) {
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
	for _, pe := range elems {
		path = append(path, pathElementQname(pe))
	}
	scopeQ := pathElementQname(scope)
	var findings []replayFinding
	steps := withURIs(replaySteps(scopeQ, path), scope, elems)
	for _, step := range steps {
		req := autocomplete.Request{
			ProjectID:               projectID,
			CurrentPath:             step.CurrentPath,
			Query:                   qnameLocal(step.Want),
			MaxResults:              200,
			IncludeParentProjects:   true,
			IncludeRangeSuggestions: true,
			Source:                  "replay",
		}
		if step.Position >= 0 {
			req.ScopeClass = scopeQ
		}
		sugs, err := svc.GetSuggestions(ctx, req)
		if err != nil {
			return nil, 0, fmt.Errorf("field %s step %d: %w", row.ID, step.Position, err)
		}
		if !replayOffered(sugs, step.Want, step.WantURI) {
			offered := make([]string, 0, len(sugs))
			for _, s := range sugs {
				offered = append(offered, s.Qname)
			}
			findings = append(findings, replayFinding{Project: projectID, SemanticID: derefSemanticID(row.SemanticID, row.ID), Position: step.Position, Want: step.Want, Offered: offered})
		}
	}
	return findings, len(steps), nil
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
