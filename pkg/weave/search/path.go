package search

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

func normalizeScope(s string) string {
	if s == "inherited" {
		return "inherited"
	}
	return "project"
}

func parsePath(input string) (localNames []string, prefixes []string) {
	localNames = []string{}
	prefixes = []string{}

	input = strings.TrimSpace(input)
	if input == "" {
		return
	}

	input = strings.TrimPrefix(input, "->")

	segments := strings.Split(input, "->")
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}

		if idx := strings.IndexByte(seg, ':'); idx >= 0 {
			prefix := seg[:idx]
			local := seg[idx+1:]
			prefixes = append(prefixes, prefix)
			localNames = append(localNames, local)
		} else {
			prefixes = append(prefixes, "")
			localNames = append(localNames, seg)
		}
	}

	return
}

func resolveProjectChain(ctx context.Context, pool *pgxpool.Pool, projectID, scope string) ([]string, error) {
	const query = `
WITH RECURSIVE project_chain AS (
    SELECT id, ARRAY[id] AS path
    FROM weave_projects
    WHERE id = $1
    UNION ALL
    SELECT next_parent.parent_id, pc.path || next_parent.parent_id
    FROM project_chain pc
    JOIN LATERAL (
        SELECT wpi.parent_project_id AS parent_id
        FROM weave_project_inheritance wpi
        WHERE wpi.project_id = pc.id
        UNION ALL
        SELECT p.parent_project_id AS parent_id
        FROM weave_projects p
        WHERE p.id = pc.id
          AND p.parent_project_id IS NOT NULL
          AND NOT EXISTS (
              SELECT 1 FROM weave_project_inheritance wpi
              WHERE wpi.project_id = pc.id
          )
    ) next_parent ON TRUE
    WHERE $2 = 'inherited'
      AND next_parent.parent_id IS NOT NULL
      AND next_parent.parent_id <> ''
      AND NOT (next_parent.parent_id = ANY(pc.path))
)
SELECT id FROM project_chain`

	rows, err := pool.Query(ctx, query, projectID, scope)
	if err != nil {
		return nil, fmt.Errorf("resolve project chain: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan project chain row: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("project chain rows: %w", err)
	}

	return ids, nil
}

func resolvePathFieldIDs(ctx context.Context, pool *pgxpool.Pool, projectIDs []string, params domain.SearchParams) ([]string, error) {
	hasStart := params.PathStartsWith != ""
	hasEnd := params.PathEndsWith != ""
	hasDepth := params.PathDepth > 0

	if !hasStart && !hasEnd && !hasDepth {
		return nil, nil
	}

	var base []string

	if hasStart {
		localNames, prefixes := parsePath(params.PathStartsWith)
		if len(localNames) > 0 {
			ids, err := callMatchPathPrefix(ctx, pool, projectIDs, localNames, prefixes)
			if err != nil {
				return nil, fmt.Errorf("path_starts_with filter: %w", err)
			}
			base = ids
		}
	}

	if hasEnd {
		endLocalNames, endPrefixes := parsePath(params.PathEndsWith)
		if len(endLocalNames) > 0 {
			endLocal := endLocalNames[len(endLocalNames)-1]
			endPrefix := endPrefixes[len(endPrefixes)-1]

			endIDs, err := callPathEndsWith(ctx, pool, projectIDs, endLocal, endPrefix)
			if err != nil {
				return nil, fmt.Errorf("path_ends_with filter: %w", err)
			}

			base = intersect(base, endIDs)
		}
	}

	if hasDepth {
		depthIDs, err := callPathDepth(ctx, pool, projectIDs, params.PathDepth)
		if err != nil {
			return nil, fmt.Errorf("path_depth filter: %w", err)
		}

		base = intersect(base, depthIDs)
	}

	return base, nil
}

func callMatchPathPrefix(ctx context.Context, pool *pgxpool.Pool, projectIDs, elements, prefixes []string) ([]string, error) {
	const sql = `SELECT field_id FROM match_path_prefix($1::text[], $2::text[], $3::text[])`

	rows, err := pool.Query(ctx, sql, projectIDs, elements, prefixes)
	if err != nil {
		return nil, fmt.Errorf("match_path_prefix: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan match_path_prefix row: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("match_path_prefix rows: %w", err)
	}

	return ids, nil
}

func callPathEndsWith(ctx context.Context, pool *pgxpool.Pool, projectIDs []string, endLocal, endPrefix string) ([]string, error) {
	const endsSQL = `
SELECT DISTINCT pe.field_id FROM weave_path_elements pe
JOIN weave_fields f ON f.id = pe.field_id
WHERE f.project_id = ANY($3)
  AND starts_with(pe.local_name, $1)
  AND ($2 = '' OR pe.prefix = $2)
  AND pe.position = (
      SELECT MAX(pe2.position) FROM weave_path_elements pe2
      WHERE pe2.field_id = pe.field_id
  )`

	rows, err := pool.Query(ctx, endsSQL, endLocal, endPrefix, projectIDs)
	if err != nil {
		return nil, fmt.Errorf("path_ends_with query: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan path_ends_with row: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("path_ends_with rows: %w", err)
	}

	return ids, nil
}

func callPathDepth(ctx context.Context, pool *pgxpool.Pool, projectIDs []string, depth int) ([]string, error) {
	const depthSQL = `
SELECT pe.field_id FROM weave_path_elements pe
JOIN weave_fields f ON f.id = pe.field_id
WHERE f.project_id = ANY($2)
GROUP BY pe.field_id HAVING COUNT(*) = $1`

	rows, err := pool.Query(ctx, depthSQL, depth, projectIDs)
	if err != nil {
		return nil, fmt.Errorf("path_depth query: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan path_depth row: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("path_depth rows: %w", err)
	}

	return ids, nil
}

func intersect(base, next []string) []string {
	if base == nil {
		return next
	}

	set := make(map[string]struct{}, len(base))
	for _, id := range base {
		set[id] = struct{}{}
	}

	result := make([]string, 0)
	for _, id := range next {
		if _, ok := set[id]; ok {
			result = append(result, id)
		}
	}

	return result
}

// PathSuggestionsHandler handles GET /api/v1/projects/{projectID}/path-suggestions.
func (h *Handler) PathSuggestionsHandler(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		writeAPIError(w, "missing project id", http.StatusBadRequest)
		return
	}

	pool, ok := h.pool()
	if !ok {
		writeAPIError(w, "path suggestions not available", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	currentPath := r.URL.Query().Get("current_path")
	scope := normalizeScope(r.URL.Query().Get("scope"))
	query := r.URL.Query().Get("query")

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	targets, err := resolveProjectTargets(ctx, pool, projectID, scope, weaveauth.ProjectVersionFromContext(ctx))
	if err != nil {
		h.logger.Error("resolve project targets failed", "err", err, "project_id", projectID)
		writeAPIError(w, "failed to resolve project scope", http.StatusInternalServerError)
		return
	}

	localNames, prefixes := parsePath(currentPath)
	combined := make(map[string]domain.PathSuggestion)
	for _, target := range targets {
		var targetSuggestions []domain.PathSuggestion
		if strings.TrimSpace(target.Version) == "" {
			targetSuggestions, err = callPathNextSuggestions(ctx, pool, []string{target.ProjectID}, localNames, prefixes, query, limit)
		} else {
			targetSuggestions, err = callArchivedPathNextSuggestions(ctx, pool, target, localNames, prefixes, query)
		}
		if err != nil {
			h.logger.Error("path_next_suggestions failed", "err", err, "project_id", projectID, "target_id", target.ProjectID, "target_version", target.Version, slog.String("path", currentPath))
			writeAPIError(w, "path suggestions query failed", http.StatusInternalServerError)
			return
		}
		for _, suggestion := range targetSuggestions {
			key := suggestion.Prefix + "|" + suggestion.LocalName + "|" + suggestion.Type
			current, ok := combined[key]
			if !ok {
				combined[key] = suggestion
				continue
			}
			current.FieldCount += suggestion.FieldCount
			combined[key] = current
		}
	}

	suggestions := make([]domain.PathSuggestion, 0, len(combined))
	for _, suggestion := range combined {
		suggestions = append(suggestions, suggestion)
	}
	sort.SliceStable(suggestions, func(i, j int) bool {
		if suggestions[i].FieldCount != suggestions[j].FieldCount {
			return suggestions[i].FieldCount > suggestions[j].FieldCount
		}
		if suggestions[i].Display != suggestions[j].Display {
			return suggestions[i].Display < suggestions[j].Display
		}
		return suggestions[i].Type < suggestions[j].Type
	})
	if len(suggestions) > limit {
		suggestions = suggestions[:limit]
	}

	resp := domain.PathSuggestionsResponse{
		Suggestions: suggestions,
		CurrentPath: currentPath,
		Depth:       len(localNames),
	}

	writeJSON(w, http.StatusOK, resp)
}

func callPathNextSuggestions(ctx context.Context, pool *pgxpool.Pool, projectIDs, elements, prefixes []string, query string, limit int) ([]domain.PathSuggestion, error) {
	const sql = `SELECT local_name, prefix, elem_type, field_count FROM path_next_suggestions($1::text[], $2::text[], $3::text[], $4::text) LIMIT $5`

	rows, err := pool.Query(ctx, sql, projectIDs, elements, prefixes, query, limit)
	if err != nil {
		return nil, fmt.Errorf("path_next_suggestions: %w", err)
	}
	defer rows.Close()

	var suggestions []domain.PathSuggestion
	for rows.Next() {
		var s domain.PathSuggestion
		if err := rows.Scan(&s.LocalName, &s.Prefix, &s.Type, &s.FieldCount); err != nil {
			return nil, fmt.Errorf("scan path_next_suggestions row: %w", err)
		}

		if s.Prefix != "" {
			s.Display = s.Prefix + ":" + s.LocalName
		} else {
			s.Display = s.LocalName
		}

		suggestions = append(suggestions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("path_next_suggestions rows: %w", err)
	}

	if suggestions == nil {
		suggestions = []domain.PathSuggestion{}
	}

	return suggestions, nil
}

func callArchivedPathNextSuggestions(ctx context.Context, pool *pgxpool.Pool, target selectorTarget, elements, prefixes []string, query string) ([]domain.PathSuggestion, error) {
	rows, err := pool.Query(ctx, `
		SELECT path_elements
		FROM weave_fields_archive
		WHERE project_id = $1 AND version_number = $2 AND status IN ('draft', 'published')
	`, target.ProjectID, target.Version)
	if err != nil {
		return nil, fmt.Errorf("archived path suggestions query: %w", err)
	}
	defer rows.Close()

	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	combined := make(map[string]domain.PathSuggestion)
	for rows.Next() {
		var pathData []byte
		if err := rows.Scan(&pathData); err != nil {
			return nil, fmt.Errorf("scan archived path elements: %w", err)
		}
		pathElements := parsePathElements(pathData)
		if len(elements) == 0 {
			if len(pathElements) == 0 {
				continue
			}
			next := pathElements[0]
			if normalizedQuery != "" && !strings.Contains(strings.ToLower(next.LocalName), normalizedQuery) {
				continue
			}
			appendSuggestion(combined, next)
			continue
		}
		if !pathStartsWithElements(pathElements, elements, prefixes) {
			continue
		}
		if len(pathElements) <= len(elements) {
			continue
		}
		next := pathElements[len(elements)]
		if normalizedQuery != "" && !strings.Contains(strings.ToLower(next.LocalName), normalizedQuery) {
			continue
		}
		appendSuggestion(combined, next)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived path elements: %w", err)
	}

	suggestions := make([]domain.PathSuggestion, 0, len(combined))
	for _, suggestion := range combined {
		suggestions = append(suggestions, suggestion)
	}
	return suggestions, nil
}

func appendSuggestion(out map[string]domain.PathSuggestion, element domain.PathElement) {
	key := element.Prefix + "|" + element.LocalName + "|" + element.Type
	current := out[key]
	current.LocalName = element.LocalName
	current.Prefix = element.Prefix
	current.Type = element.Type
	current.FieldCount++
	if current.Prefix != "" {
		current.Display = current.Prefix + ":" + current.LocalName
	} else {
		current.Display = current.LocalName
	}
	out[key] = current
}
