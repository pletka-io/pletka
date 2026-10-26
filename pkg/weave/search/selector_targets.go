package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pletka-io/pletka/pkg/database/releasearchive"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

type selectorTarget struct {
	ProjectID string
	Version   string
}

const maxSelectorDepth = 10

// resolveProjectTargets walks the project's inheritance chain (when
// scope == "inherited") into a flat list of {project, version} search
// targets. rootVersion is the effective version for projectID itself —
// resolved by auth.ResolveContentVersion/auth.WithProjectVersionContext
// upstream, "" for the hot draft view. A non-empty rootVersion makes the
// root target read from the project's own archived (_archive) rows via
// searchArchivedFields/searchArchivedCollections, instead of the hot tables.
func resolveProjectTargets(ctx context.Context, pool *pgxpool.Pool, projectID, scope, rootVersion string) ([]selectorTarget, error) {
	queue := []selectorTarget{{ProjectID: projectID, Version: rootVersion}}
	visited := make(map[string]struct{})
	out := make([]selectorTarget, 0)

	for depth := 0; len(queue) > 0 && depth < maxSelectorDepth; depth++ {
		current := queue[0]
		queue = queue[1:]
		key := selectorTargetKey(current)
		if _, ok := visited[key]; ok {
			continue
		}
		visited[key] = struct{}{}
		out = append(out, current)
		if scope != "inherited" {
			continue
		}
		parents, err := listParentTargets(ctx, pool, current)
		if err != nil {
			return nil, err
		}
		for _, parent := range parents {
			if parent.ProjectID == "" {
				continue
			}
			if _, ok := visited[selectorTargetKey(parent)]; ok {
				continue
			}
			queue = append(queue, parent)
		}
	}

	return out, nil
}

func selectorTargetKey(target selectorTarget) string {
	return target.ProjectID + "@" + strings.TrimSpace(target.Version)
}

func listParentTargets(ctx context.Context, pool *pgxpool.Pool, target selectorTarget) ([]selectorTarget, error) {
	if strings.TrimSpace(target.Version) != "" {
		return listParentTargetsArchived(ctx, pool, target.ProjectID, target.Version)
	}
	return listParentTargetsDraft(ctx, pool, target.ProjectID)
}

func listParentTargetsDraft(ctx context.Context, pool *pgxpool.Pool, projectID string) ([]selectorTarget, error) {
	rows, err := pool.Query(ctx, `
		SELECT parent_project_id, source_mode, COALESCE(source_version, '')
		FROM weave_project_inheritance
		WHERE project_id = $1
		ORDER BY CASE WHEN is_primary THEN 0 ELSE 1 END, canonical_order ASC, parent_project_id ASC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query project inheritances: %w", err)
	}
	defer rows.Close()

	out := make([]selectorTarget, 0)
	for rows.Next() {
		var parentID, sourceMode, sourceVersion string
		if err := rows.Scan(&parentID, &sourceMode, &sourceVersion); err != nil {
			return nil, fmt.Errorf("scan project inheritance parent: %w", err)
		}
		next := selectorTarget{ProjectID: parentID}
		if normalizeSourceMode(sourceMode) == "release" {
			next.Version = strings.TrimSpace(sourceVersion)
		}
		out = append(out, next)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project inheritances: %w", err)
	}
	if len(out) > 0 {
		return out, nil
	}

	var parent *string
	if err := pool.QueryRow(ctx, `SELECT parent_project_id FROM weave_projects WHERE id = $1`, projectID).Scan(&parent); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("fallback project parent: %w", err)
	}
	if parent == nil || *parent == "" {
		return nil, nil
	}
	return []selectorTarget{{ProjectID: *parent}}, nil
}

func listParentTargetsArchived(ctx context.Context, pool *pgxpool.Pool, projectID, version string) ([]selectorTarget, error) {
	rows, err := releasearchive.Parents(ctx, sqlcgen.New(pool), projectID, version)
	if err != nil {
		return nil, err
	}
	out := make([]selectorTarget, 0, len(rows))
	for _, row := range rows {
		out = append(out, selectorTarget{ProjectID: row.ParentProjectID, Version: *row.SourceVersion})
	}
	return out, nil
}

func normalizeSourceMode(mode string) string {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "release" {
		return "release"
	}
	return "draft"
}
