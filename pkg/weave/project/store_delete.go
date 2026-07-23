package project

import (
	"context"
	"fmt"
)

// DeleteBlockers counts everything outside the project that depends on it.
// Entity ids are semantic and carry the owning project's identity, but
// ownership checks join on the owning tables rather than trusting prefixes.
func (s *postgresStore) DeleteBlockers(ctx context.Context, id string) (DeleteBlockers, error) {
	var b DeleteBlockers
	row := s.pool.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM weave_project_inheritance WHERE parent_project_id = $1),
		  (SELECT count(*) FROM weave_adoptions WHERE source_project_id = $1 AND project_id <> $1),
		  (SELECT count(*)
		     FROM weave_field_overrides fo
		     JOIN weave_fields f ON fo.field_id = f.id
		    WHERE f.project_id = $1
		      AND fo.entity_type <> '' AND fo.entity_id <> ''
		      AND NOT EXISTS (SELECT 1 FROM weave_models m WHERE m.id = fo.entity_id AND m.project_id = $1)
		      AND NOT EXISTS (SELECT 1 FROM weave_collections c WHERE c.id = fo.entity_id AND c.project_id = $1)),
		  (SELECT count(*)
		     FROM weave_override_refs r
		     JOIN weave_field_overrides fo ON r.override_id = fo.id
		     JOIN weave_fields f ON fo.field_id = f.id
		    WHERE f.project_id <> $1
		      AND (r.target_id IN (SELECT semantic_id FROM weave_fields WHERE project_id = $1 AND semantic_id IS NOT NULL)
		        OR r.target_id IN (SELECT id FROM weave_models WHERE project_id = $1)
		        OR r.target_id IN (SELECT id FROM weave_collections WHERE project_id = $1)))
	`, id)
	if err := row.Scan(&b.Children, &b.AdoptionsElsewhere, &b.ExternalPlacements, &b.ExternalValueRefs); err != nil {
		return DeleteBlockers{}, fmt.Errorf("count delete blockers for %s: %w", id, err)
	}
	return b, nil
}

// DeleteCascade removes the project and every row it owns in one
// transaction. FK cascades from weave_projects cover adoptions, forks,
// inheritance, integrations, vocab links, releases, attributions and
// project_actors; weave_path_elements and weave_override_refs cascade from
// their parents. Everything else is deleted explicitly — the entity tables
// deliberately carry no FK to weave_projects.
func (s *postgresStore) DeleteCascade(ctx context.Context, id string) (DeleteStats, error) {
	var stats DeleteStats
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return stats, fmt.Errorf("begin delete-project tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Overrides first: rows keyed by the project's fields OR living in the
	// project's containers (adopted foreign fields placed here). Their
	// override_refs cascade.
	res, err := tx.Exec(ctx, `
		DELETE FROM weave_field_overrides fo
		WHERE fo.field_id IN (SELECT id FROM weave_fields WHERE project_id = $1)
		   OR fo.entity_id IN (SELECT id FROM weave_models WHERE project_id = $1)
		   OR fo.entity_id IN (SELECT id FROM weave_collections WHERE project_id = $1)
	`, id)
	if err != nil {
		return stats, fmt.Errorf("delete overrides: %w", err)
	}
	stats.Overrides = res.RowsAffected()

	type step struct {
		name string
		sql  string
		into *int64
	}
	steps := []step{
		{"collection placements", `DELETE FROM weave_collection_placements WHERE project_id = $1`, &stats.OtherRows},
		{"field ontology refs", `DELETE FROM weave_field_ontology_refs WHERE project_id = $1`, &stats.OtherRows},
		{"fields", `DELETE FROM weave_fields WHERE project_id = $1`, &stats.Fields},
		{"models", `DELETE FROM weave_models WHERE project_id = $1`, &stats.Models},
		{"collections", `DELETE FROM weave_collections WHERE project_id = $1`, &stats.Collections},
		{"categories", `DELETE FROM weave_categories WHERE project_id = $1`, &stats.Categories},
		{"concept lists", `DELETE FROM weave_concept_lists WHERE project_id = $1`, &stats.OtherRows},
		{"vocabularies", `DELETE FROM weave_vocabularies WHERE project_id = $1`, &stats.OtherRows},
		{"entity counters", `DELETE FROM weave_entity_counters WHERE project_id = $1`, &stats.OtherRows},
		{"namespace bindings", `DELETE FROM weave_namespace_bindings WHERE project_id = $1`, &stats.OtherRows},
		{"ontology links", `DELETE FROM weave_project_ontology_versions WHERE project_id = $1`, &stats.OtherRows},
		{"memberships", `DELETE FROM weave_memberships WHERE scope_type = 'project' AND scope_id = $1`, &stats.OtherRows},
		{"import staging", `DELETE FROM weave_import_staging WHERE project_id = $1`, &stats.OtherRows},
		{"change log", `DELETE FROM weave_change_log WHERE project_id = $1`, &stats.ChangeLogRows},
		{"change sets", `DELETE FROM weave_change_set WHERE project_id = $1`, &stats.ChangeLogRows},
		{"restore jobs", `DELETE FROM admin_git_restore_jobs WHERE source_project_id = $1 OR target_project_id = $1`, &stats.OtherRows},
	}
	for _, archive := range []string{
		"weave_fields_archive", "weave_models_archive", "weave_collections_archive",
		"weave_categories_archive", "weave_concept_lists_archive",
		"weave_field_overrides_archive", "weave_override_refs_archive",
		"weave_adoptions_archive", "weave_namespace_bindings_archive",
		"weave_project_inheritance_archive", "weave_project_ontology_versions_archive",
		"weave_change_log_archive", "weave_change_set_archive", "weave_entity_forks_archive",
	} {
		steps = append(steps, step{archive, fmt.Sprintf(`DELETE FROM %s WHERE project_id = $1`, archive), &stats.ArchiveRows})
	}

	for _, st := range steps {
		res, err := tx.Exec(ctx, st.sql, id)
		if err != nil {
			return stats, fmt.Errorf("delete %s: %w", st.name, err)
		}
		*st.into += res.RowsAffected()
	}

	res, err = tx.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, id)
	if err != nil {
		return stats, fmt.Errorf("delete project row: %w", err)
	}
	stats.ProjectDeleted = res.RowsAffected() == 1

	if err := tx.Commit(ctx); err != nil {
		return stats, fmt.Errorf("commit delete-project tx: %w", err)
	}
	return stats, nil
}
