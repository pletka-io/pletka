package weave

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/pletka-io/pletka/domain"
)

type provenanceQuery struct {
	where string
	args  []any
}

func buildProvenanceQuery(cfg *domain.QueryConfig, allowedFilters map[string]string) provenanceQuery {
	args := []any{}
	filters := []string{}
	add := func(column string, value any) {
		args = append(args, value)
		filters = append(filters, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	if cfg.ProjectID != "" {
		add("project_id", cfg.ProjectID)
	}
	keys := make([]string, 0, len(allowedFilters))
	for key := range allowedFilters {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		column := allowedFilters[key]
		if value, ok := cfg.Filters[key].(string); ok && strings.TrimSpace(value) != "" {
			add(column, strings.TrimSpace(value))
		}
	}
	if cfg.Version != "" {
		add("version_number", cfg.Version)
	}

	if len(filters) == 0 {
		return provenanceQuery{args: args}
	}
	return provenanceQuery{
		where: " WHERE " + strings.Join(filters, " AND "),
		args:  args,
	}
}

func nullableTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
