package cmd

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/pletka-io/pletka/pkg/app/cliruntime"
	"github.com/pletka-io/pletka/pkg/weave/pathaudit"
)

var weaveVerifyPathsOpts struct {
	project string
	format  string
}

func newWeaveVerifyPathsCommand() *cobra.Command {
	weaveVerifyPathsCmd := &cobra.Command{
		Use:   "verify-paths",
		Short: "Verify field path elements against the linked ontologies",
		Long: `Audits every weave_fields.path_elements entry against the project's
linked ontology versions. Each non-literal element's stored type is
checked against the source of truth — the weave_ontology_classes and
weave_ontology_properties tables — so type corruption (a class stored as
a property, or vice versa) and unresolvable qnames surface as errors.

Report-only: nothing is modified. Exits non-zero when any error is found.

Examples:
  pletka weave verify-paths
  pletka weave verify-paths --project LA
  pletka weave verify-paths --format json`,
		RunE: runWeaveVerifyPaths,
	}

	weaveVerifyPathsCmd.Flags().StringVar(&weaveVerifyPathsOpts.project, "project", "", "limit to one project (id or system_name); all projects when omitted")
	weaveVerifyPathsCmd.Flags().StringVar(&weaveVerifyPathsOpts.format, "format", "text", "output format: text, json, or csv (csv lists type-mismatched fields with their raw path)")
	return weaveVerifyPathsCmd
}

func runWeaveVerifyPaths(cmd *cobra.Command, args []string) error {
	switch weaveVerifyPathsOpts.format {
	case "text", "json", "csv":
	default:
		return fmt.Errorf("invalid --format %q: want text, json, or csv", weaveVerifyPathsOpts.format)
	}

	ctx := context.Background()
	pool, err := cliruntime.OpenPingedPGXPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	errs, scanned, err := pathaudit.NewService(pool).Audit(ctx, weaveVerifyPathsOpts.project)
	if err != nil {
		return err
	}

	switch weaveVerifyPathsOpts.format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(errs); err != nil {
			return fmt.Errorf("encode json: %w", err)
		}
	case "csv":
		if err := writeFlippedFieldCSV(ctx, pool, errs); err != nil {
			return err
		}
	default:
		printPathElementReport(errs, scanned)
	}

	if len(errs) > 0 {
		cmd.SilenceUsage = true
		return fmt.Errorf("%d path element error(s) found", len(errs))
	}
	return nil
}

func printPathElementReport(errs []pathaudit.PathElementError, scanned int) {
	fmt.Println("=== Path Element Verification ===")
	fmt.Printf("Non-literal elements scanned: %d\n", scanned)
	fmt.Printf("Errors: %d\n\n", len(errs))

	if len(errs) == 0 {
		fmt.Println("All path elements agree with their linked ontologies.")
		return
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "PROJECT\tFIELD\tPOS\tQNAME\tVERDICT")
	byVerdict := map[string]int{}
	for _, e := range errs {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n", e.Project, e.SemanticID, e.Position, e.Qname, e.Verdict)
		byVerdict[pathaudit.VerdictClass(e.Verdict)]++
	}
	tw.Flush()

	fmt.Println("\n--- Summary by kind ---")
	for _, kind := range []string{"type-mismatch", "missing", "ambiguous", "malformed"} {
		if n := byVerdict[kind]; n > 0 {
			fmt.Printf("  %-15s %d\n", kind, n)
		}
	}
}

// writeFlippedFieldCSV emits one CSV row per field carrying a
// type-mismatch ("flipped") element: project, semantic id, the flipped
// positions, a readable qname path, and the raw path_elements JSON.
func writeFlippedFieldCSV(ctx context.Context, pool *pgxpool.Pool, errs []pathaudit.PathElementError) error {
	// Group flipped positions by field, preserving first-seen order.
	type fieldKey struct{ project, semanticID string }
	positions := map[fieldKey][]int{}
	var order []fieldKey
	for _, e := range errs {
		if pathaudit.VerdictClass(e.Verdict) != "type-mismatch" {
			continue
		}
		k := fieldKey{e.Project, e.SemanticID}
		if _, seen := positions[k]; !seen {
			order = append(order, k)
		}
		positions[k] = append(positions[k], e.Position)
	}

	w := csv.NewWriter(os.Stdout)
	defer w.Flush()
	if err := w.Write([]string{"project", "semantic_id", "flipped_positions", "path", "raw_path_elements"}); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	if len(order) == 0 {
		return nil
	}

	projects := make([]string, len(order))
	semIDs := make([]string, len(order))
	for i, k := range order {
		projects[i] = k.project
		semIDs[i] = k.semanticID
	}

	const query = `
SELECT pr.system_name, f.semantic_id, f.path_elements::text
FROM weave_fields f
JOIN weave_projects pr ON pr.id = f.project_id
WHERE (pr.system_name, f.semantic_id) IN (
    SELECT * FROM unnest($1::text[], $2::text[])
)`
	rows, err := pool.Query(ctx, query, projects, semIDs)
	if err != nil {
		return fmt.Errorf("query raw paths: %w", err)
	}
	defer rows.Close()

	rawByKey := map[fieldKey]string{}
	for rows.Next() {
		var proj, sem, raw string
		if err := rows.Scan(&proj, &sem, &raw); err != nil {
			return fmt.Errorf("scan raw path row: %w", err)
		}
		rawByKey[fieldKey{proj, sem}] = raw
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate raw path rows: %w", err)
	}

	for _, k := range order {
		raw := rawByKey[k]
		posStrs := make([]string, len(positions[k]))
		for i, p := range positions[k] {
			posStrs[i] = strconv.Itoa(p)
		}
		record := []string{
			k.project,
			k.semanticID,
			strings.Join(posStrs, " "),
			pathElementsToString(raw),
			raw,
		}
		if err := w.Write(record); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}
	}
	return nil
}

// pathElementsToString renders the raw path_elements JSON as a readable
// "crm:E21_Person > crm:P1_is_identified_by > ..." chain.
func pathElementsToString(raw string) string {
	var elems []struct {
		Prefix    string `json:"prefix"`
		LocalName string `json:"local_name"`
	}
	if err := json.Unmarshal([]byte(raw), &elems); err != nil {
		return ""
	}
	parts := make([]string, 0, len(elems))
	for _, e := range elems {
		if e.Prefix != "" {
			parts = append(parts, e.Prefix+":"+e.LocalName)
		} else {
			parts = append(parts, e.LocalName)
		}
	}
	return strings.Join(parts, " > ")
}
