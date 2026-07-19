package pathaudit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PathElementError is one invalid path step found by Audit.
type PathElementError struct {
	Project    string `json:"project"`
	SemanticID string `json:"semantic_id"`
	Position   int    `json:"position"`
	Qname      string `json:"qname"`
	StoredType string `json:"stored_type"`
	Verdict    string `json:"verdict"`
}

// VerdictClass reduces a verdict message to its leading kind keyword:
// "malformed" | "ambiguous" | "type-mismatch" | "missing" | "other".
func VerdictClass(verdict string) string {
	for _, kind := range []string{"type-mismatch", "missing", "ambiguous", "malformed"} {
		if len(verdict) >= len(kind) && verdict[:len(kind)] == kind {
			return kind
		}
	}
	return "other"
}

// Service audits stored ontology paths against a project's linked ontology
// versions.
type Service struct {
	// sanctioned pool holder: single raw-SQL audit sweep, lifted from the verify-paths CLI (see doc.go)
	pool *pgxpool.Pool
}

// NewService builds a Service backed by pool.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Audit walks every field's path_elements and returns the elements whose
// stored type disagrees with the ontology tables, or whose qname resolves
// to neither table. scanned is the count of non-literal elements
// inspected. projectID empty means all projects (CLI mode); MCP callers
// always pass one.
func (s *Service) Audit(ctx context.Context, projectID string, excludeStandard bool) (errs []PathElementError, scanned int, err error) {
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

	rows, err := s.pool.Query(ctx, query, projectID, excludeStandard)
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
		errs = append(errs, PathElementError{
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
