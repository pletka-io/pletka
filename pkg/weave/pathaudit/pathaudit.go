package pathaudit

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// defaultsOntologyPrefix is the well-known prefix of the implicit defaults
// ontology (standard-vocabulary terms: rdfs:Literal, skos:Concept, xsd
// datatypes, rdfs:label, ...). No project ever links it through
// weave_project_ontology_versions — per the linkage decision it is an
// ontology-level dependency implicit to every project — so its active
// version is unioned into the resolution scope directly instead of being
// discovered via a project link. Matches the constant gitmaterializer uses
// to vendor it (pkg/service/gitmaterializer/vendor_snapshot.go).
const defaultsOntologyPrefix = "defaults"

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
// always pass one. Resolution scope for every project is its linked
// ontology versions union the defaults ontology's active version, when one
// is imported (see resolveDefaultsVersionID).
func (s *Service) Audit(ctx context.Context, projectID string) (errs []PathElementError, scanned int, err error) {
	defaultsVersionID, err := s.resolveDefaultsVersionID(ctx)
	if err != nil {
		return nil, 0, err
	}

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
      AND coalesce(e.elem->>'type', '') NOT IN ('literal', 'complete')
      AND ($1 = '' OR pr.id = $1 OR pr.system_name = $1)
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
      AND coalesce(e.elem->>'type', '') NOT IN ('literal', 'complete')
      AND ($1 = '' OR pr.id = $1 OR pr.system_name = $1)
), classified AS (
    -- A qname resolves either through the project's own linked ontology
    -- versions, or through the defaults ontology's active version ($2) —
    -- an implicit, ontology-level dependency no project ever links.
    SELECT e.*,
        EXISTS (
            SELECT 1 FROM weave_ontology_classes c
            WHERE c.prefix = e.prefix
              AND c.local_name = e.local_name
              AND (
                c.ontology_version_id = $2
                OR EXISTS (
                    SELECT 1 FROM weave_project_ontology_versions pov
                    JOIN weave_projects p2 ON p2.id = pov.project_id
                    WHERE p2.system_name = e.project
                      AND pov.ontology_version_id = c.ontology_version_id
                )
              )
        ) AS in_classes,
        EXISTS (
            SELECT 1 FROM weave_ontology_properties pr2
            WHERE pr2.prefix = e.prefix
              AND pr2.local_name = e.local_name
              AND (
                pr2.ontology_version_id = $2
                OR EXISTS (
                    SELECT 1 FROM weave_project_ontology_versions pov
                    JOIN weave_projects p2 ON p2.id = pov.project_id
                    WHERE p2.system_name = e.project
                      AND pov.ontology_version_id = pr2.ontology_version_id
                )
              )
        ) AS in_props
    FROM elems e
)
SELECT project, semantic_id, position, prefix, local_name, stored_type,
       in_classes, in_props,
       (in_classes AND NOT in_props AND stored_type = 'class')
         OR (in_props AND NOT in_classes AND stored_type = 'property') AS ok
FROM classified
ORDER BY project, semantic_id, position`

	rows, err := s.pool.Query(ctx, query, projectID, defaultsVersionID)
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

// resolveDefaultsVersionID looks up the active version of the implicit
// defaults ontology (prefix "defaults") — the same lookup gitmaterializer
// uses to vendor it. Returns "" when no defaults ontology is imported (or
// it has no active version): callers treat that as "nothing to union",
// leaving Audit's behavior identical to before defaults existed.
func (s *Service) resolveDefaultsVersionID(ctx context.Context) (string, error) {
	queries := sqlcgen.New(s.pool)
	ontology, err := queries.WeaveGetOntologyByPrefix(ctx, defaultsOntologyPrefix)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("look up defaults ontology: %w", err)
	}
	version, err := queries.WeaveGetActiveOntologyVersion(ctx, ontology.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("look up defaults ontology active version: %w", err)
	}
	return version.ID, nil
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
