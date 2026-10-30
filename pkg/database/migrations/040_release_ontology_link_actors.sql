-- +goose Up
-- Lossless project assembly retains ontology-link provenance. Its actor belongs
-- in the same captured descriptor context as owners, credits and adoption authors.
-- Existing immutable descriptor documents are deliberately not backfilled.
CREATE OR REPLACE VIEW weave_release_actor_references AS
SELECT id AS project_id, version_number, owner_id AS actor_id FROM weave_projects_archive
UNION SELECT id, version_number, created_by_id FROM weave_projects_archive
UNION SELECT project_id, version, created_by_id FROM weave_releases
UNION SELECT project_id, version_number, created_by_id FROM weave_adoptions_archive
UNION SELECT project_id, version_number, created_by_id FROM weave_entity_forks_archive
UNION SELECT project_id, version_number, actor_id FROM weave_project_attributions_archive
UNION SELECT project_id, version_number, actor_id FROM weave_project_actors_archive
UNION SELECT project_id, version_number, added_by_id FROM weave_project_ontology_versions_archive;

-- +goose Down
CREATE OR REPLACE VIEW weave_release_actor_references AS
SELECT id AS project_id, version_number, owner_id AS actor_id FROM weave_projects_archive
UNION SELECT id, version_number, created_by_id FROM weave_projects_archive
UNION SELECT project_id, version, created_by_id FROM weave_releases
UNION SELECT project_id, version_number, created_by_id FROM weave_adoptions_archive
UNION SELECT project_id, version_number, created_by_id FROM weave_entity_forks_archive
UNION SELECT project_id, version_number, actor_id FROM weave_project_attributions_archive
UNION SELECT project_id, version_number, actor_id FROM weave_project_actors_archive;
