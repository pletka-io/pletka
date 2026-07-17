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
)

var weaveVerifyPathsOpts struct {
	project         string
	format          string
	excludeStandard bool
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
  pletka weave verify-paths --exclude-standard --format json`,
		RunE: runWeaveVerifyPaths,
	}

	weaveVerifyPathsCmd.Flags().StringVar(&weaveVerifyPathsOpts.project, "project", "", "limit to one project (id or system_name); all projects when omitted")
	weaveVerifyPathsCmd.Flags().StringVar(&weaveVerifyPathsOpts.format, "format", "text", "output format: text, json, or csv (csv lists type-mismatched fields with their raw path)")
	weaveVerifyPathsCmd.Flags().BoolVar(&weaveVerifyPathsOpts.excludeStandard, "exclude-standard", false, "skip rdf/rdfs/xsd/xsl/dc/dcterms/skos/owl/schema elements (not part of CRM-family ontologies)")
	return weaveVerifyPathsCmd
}

// pathElementError is one verified-and-failed path element.
type pathElementError struct {
	Project    string `json:"project"`
	SemanticID string `json:"semantic_id"`
	Position   int    `json:"position"`
	Qname      string `json:"qname"`
	StoredType string `json:"stored_type"`
	Verdict    string `json:"verdict"`
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

	errs, scanned, err := collectPathElementErrors(ctx, pool, weaveVerifyPathsOpts.project, weaveVerifyPathsOpts.excludeStandard)
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

// collectPathElementErrors walks every field's path_elements and returns
// the elements whose stored type disagrees with the ontology tables, or
// whose qname resolves to neither table. scanned is the count of
// non-literal elements inspected.
func collectPathElementErrors(ctx context.Context, pool *pgxpool.Pool, project string, excludeStandard bool) (errs []pathElementError, scanned int, err error) {
	const query = `
WITH elems AS (
    -- Primary path elements.
    SELECT
        pr.system_name AS project,
        f.semantic_id AS semantic_id,
        coalesce((e.elem->>'position')::int, (e.ord - 1)::int) AS position,
        coalesce(e.elem->>'prefix', '') AS prefix,
        coalesce(e.elem->>'local_name', '') AS local_name,
        coalesce(e.elem->>'type', '') AS stored_type
    FROM weave_fields f
    JOIN weave_projects pr ON pr.id = f.project_id
    CROSS JOIN LATERAL jsonb_array_elements(f.path_elements)
        WITH ORDINALITY AS e(elem, ord)
    WHERE f.path_elements IS NOT NULL
      AND jsonb_typeof(f.path_elements) = 'array'
      AND coalesce(e.elem->>'type', '') <> 'literal'
      AND ($1 = '' OR pr.id = $1 OR pr.system_name = $1)
      AND ($2 = false OR coalesce(e.elem->>'prefix', '') NOT IN
            ('rdf', 'rdfs', 'xsd', 'xsl', 'dc', 'dcterms', 'skos', 'owl', 'schema'))
    UNION ALL
    -- Legacy subfield path elements (first-class). The subfield index is
    -- appended to the field id ("SRD1F.5#1") so failures are attributable.
    SELECT
        pr.system_name AS project,
        f.semantic_id || '#' || (sf.sord - 1)::text AS semantic_id,
        coalesce((e.elem->>'position')::int, (e.ord - 1)::int) AS position,
        coalesce(e.elem->>'prefix', '') AS prefix,
        coalesce(e.elem->>'local_name', '') AS local_name,
        coalesce(e.elem->>'type', '') AS stored_type
    FROM weave_fields f
    JOIN weave_projects pr ON pr.id = f.project_id
    CROSS JOIN LATERAL jsonb_array_elements(f.subfield_paths)
        WITH ORDINALITY AS sf(subfield, sord)
    CROSS JOIN LATERAL jsonb_array_elements(sf.subfield->'path_elements')
        WITH ORDINALITY AS e(elem, ord)
    WHERE f.subfield_paths IS NOT NULL
      AND jsonb_typeof(f.subfield_paths) = 'array'
      AND coalesce(e.elem->>'type', '') <> 'literal'
      AND ($1 = '' OR pr.id = $1 OR pr.system_name = $1)
      AND ($2 = false OR coalesce(e.elem->>'prefix', '') NOT IN
            ('rdf', 'rdfs', 'xsd', 'xsl', 'dc', 'dcterms', 'skos', 'owl', 'schema'))
), classified AS (
    SELECT e.*,
        EXISTS (
            SELECT 1 FROM weave_ontology_classes c
            JOIN weave_project_ontology_versions pov
              ON pov.ontology_version_id = c.ontology_version_id
            JOIN weave_projects p2 ON p2.id = pov.project_id
            WHERE p2.system_name = e.project
              AND c.prefix = e.prefix
              AND c.local_name = e.local_name
        ) AS in_classes,
        EXISTS (
            SELECT 1 FROM weave_ontology_properties pr2
            JOIN weave_project_ontology_versions pov
              ON pov.ontology_version_id = pr2.ontology_version_id
            JOIN weave_projects p2 ON p2.id = pov.project_id
            WHERE p2.system_name = e.project
              AND pr2.prefix = e.prefix
              AND pr2.local_name = e.local_name
        ) AS in_props
    FROM elems e
)
SELECT project, semantic_id, position, prefix, local_name, stored_type,
       in_classes, in_props,
       (in_classes AND NOT in_props AND stored_type = 'class')
         OR (in_props AND NOT in_classes AND stored_type = 'property') AS ok
FROM classified
ORDER BY project, semantic_id, position`

	rows, err := pool.Query(ctx, query, project, excludeStandard)
	if err != nil {
		return nil, 0, fmt.Errorf("query path elements: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			proj, sem, prefix, localName, storedType string
			position                                 int
			inClasses, inProps, ok                   bool
		)
		if err := rows.Scan(&proj, &sem, &position, &prefix, &localName, &storedType, &inClasses, &inProps, &ok); err != nil {
			return nil, 0, fmt.Errorf("scan path element row: %w", err)
		}
		scanned++
		if ok {
			continue
		}
		errs = append(errs, pathElementError{
			Project:    proj,
			SemanticID: sem,
			Position:   position,
			Qname:      prefix + ":" + localName,
			StoredType: storedType,
			Verdict:    pathElementVerdict(localName, storedType, inClasses, inProps),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate path element rows: %w", err)
	}
	return errs, scanned, nil
}

// pathElementVerdict explains why an element failed verification.
func pathElementVerdict(localName, storedType string, inClasses, inProps bool) string {
	if localName == "" {
		return "malformed: empty local_name"
	}
	switch {
	case inClasses && inProps:
		return "ambiguous: qname in both classes and properties"
	case inClasses:
		return fmt.Sprintf("type-mismatch: stored=%q, ontology=class", storedType)
	case inProps:
		return fmt.Sprintf("type-mismatch: stored=%q, ontology=property", storedType)
	default:
		return "missing: qname not in any linked ontology version"
	}
}

func printPathElementReport(errs []pathElementError, scanned int) {
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
		byVerdict[verdictClass(e.Verdict)]++
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
func writeFlippedFieldCSV(ctx context.Context, pool *pgxpool.Pool, errs []pathElementError) error {
	// Group flipped positions by field, preserving first-seen order.
	type fieldKey struct{ project, semanticID string }
	positions := map[fieldKey][]int{}
	var order []fieldKey
	for _, e := range errs {
		if verdictClass(e.Verdict) != "type-mismatch" {
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

// verdictClass reduces a verdict message to its leading kind keyword.
func verdictClass(verdict string) string {
	for _, kind := range []string{"type-mismatch", "missing", "ambiguous", "malformed"} {
		if len(verdict) >= len(kind) && verdict[:len(kind)] == kind {
			return kind
		}
	}
	return "other"
}
