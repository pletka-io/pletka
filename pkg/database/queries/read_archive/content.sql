-- name: ContentAdoptions :many
SELECT s.project_id AS project_id,
       s.context_entity_type AS context_entity_type,
       s.context_entity_id AS context_entity_id,
       s.entity_type AS entity_type,
       s.source_project_id AS source_project_id,
       s.source_entity_id AS source_entity_id,
       s.source_version AS source_version,
       (CASE WHEN isfinite(s.adopted_at) THEN to_char(s.adopted_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.adopted_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.adopted_at::text,'') END)::text AS adopted_at,
       s.created_by_id AS created_by_id,
       s.version_number AS version_number
FROM weave_adoptions_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT sqlc.arg(restricted)::boolean OR (((s.context_entity_type = 'model' AND s.context_entity_id = ANY(sqlc.arg(selected_model)::text[])) OR (s.context_entity_type = 'collection' AND s.context_entity_id = ANY(sqlc.arg(selected_collection)::text[])) OR (s.context_entity_type = 'field' AND s.context_entity_id = ANY(sqlc.arg(selected_field)::text[]))) OR (s.context_entity_type IN ('','project') AND ((s.entity_type = 'model' AND s.source_entity_id = ANY(sqlc.arg(selected_model)::text[])) OR (s.entity_type = 'collection' AND s.source_entity_id = ANY(sqlc.arg(selected_collection)::text[])) OR (s.entity_type = 'field' AND s.source_entity_id = ANY(sqlc.arg(selected_field)::text[])) OR (s.entity_type = 'category' AND s.source_entity_id = ANY(sqlc.arg(selected_category)::text[]))))))
ORDER BY s.project_id, s.context_entity_type, s.context_entity_id, s.entity_type, s.source_project_id, s.source_entity_id, s.source_version
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContentCategories :many
SELECT s.id AS id,
       (CASE WHEN isfinite(s.created_at) THEN to_char(s.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.created_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.created_at::text,'') END)::text AS created_at,
       (CASE WHEN isfinite(s.updated_at) THEN to_char(s.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.updated_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.updated_at::text,'') END)::text AS updated_at,
       s.semantic_id AS semantic_id,
       s.system_name AS system_name,
       s.ui_name AS ui_name,
       s.description AS description,
       s.status AS status,
       s.project_id AS project_id,
       s.canonical_order AS canonical_order,
       s.deprecated AS deprecated,
       s.version_number AS version_number
FROM weave_categories_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT sqlc.arg(restricted)::boolean OR (s.id = ANY(sqlc.arg(selected_category)::text[])))
ORDER BY s.canonical_order, s.id
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContentPlacements :many
SELECT s.id AS id,
       s.project_id AS project_id,
       s.model_id AS model_id,
       s.category_id AS category_id,
       s.collection_id AS collection_id,
       s.is_required AS is_required,
       s.min_occurs AS min_occurs,
       s.max_occurs AS max_occurs,
       s.is_hidden AS is_hidden,
       (CASE WHEN isfinite(s.created_at) THEN to_char(s.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.created_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.created_at::text,'') END)::text AS created_at,
       (CASE WHEN isfinite(s.updated_at) THEN to_char(s.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.updated_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.updated_at::text,'') END)::text AS updated_at,
       s.version_number AS version_number
FROM weave_collection_placements_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT sqlc.arg(restricted)::boolean OR (s.model_id = ANY(sqlc.arg(selected_model)::text[])))
ORDER BY s.model_id, s.category_id, s.collection_id, s.id
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContentCollections :many
SELECT s.id AS id,
       (CASE WHEN isfinite(s.created_at) THEN to_char(s.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.created_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.created_at::text,'') END)::text AS created_at,
       (CASE WHEN isfinite(s.updated_at) THEN to_char(s.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.updated_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.updated_at::text,'') END)::text AS updated_at,
       s.system_name AS system_name,
       s.ui_name AS ui_name,
       s.description AS description,
       s.status AS status,
       s.project_id AS project_id,
       s.ontology_scope AS ontology_scope,
       s.collection_number AS collection_number,
       s.canonical_collection_order AS canonical_collection_order,
       s.staging_id AS staging_id,
       s.deprecated AS deprecated,
       s.default_category_id AS default_category_id,
       s.version_number AS version_number
FROM weave_collections_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT sqlc.arg(restricted)::boolean OR (s.id = ANY(sqlc.arg(selected_collection)::text[])))
ORDER BY s.id
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContentBroader :many
SELECT s.id AS id,
       s.concept_id AS concept_id,
       s.broader_id AS broader_id,
       s.scheme_id AS scheme_id,
       s.position AS position,
       s.version_number AS version_number,
       (CASE WHEN isfinite(s.created_at) THEN to_char(s.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.created_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.created_at::text,'') END)::text AS created_at
FROM weave_concept_broader_archive s JOIN weave_concept_lists_archive p ON p.id=s.scheme_id AND p.version_number=s.version_number
WHERE p.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT sqlc.arg(restricted)::boolean OR (s.scheme_id = ANY(sqlc.arg(selected_concept_list)::text[])))
ORDER BY s.scheme_id, s.position, s.id
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContentConceptEntries :many
SELECT s.id AS id,
       s.concept_list_id AS concept_list_id,
       s.vocabulary_entry_id AS vocabulary_entry_id,
       s.position AS position,
       s.custom_label AS custom_label,
       (CASE WHEN isfinite(s.created_at) THEN to_char(s.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.created_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.created_at::text,'') END)::text AS created_at,
       (CASE WHEN isfinite(s.updated_at) THEN to_char(s.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.updated_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.updated_at::text,'') END)::text AS updated_at,
       s.version_number AS version_number
FROM weave_concept_list_entries_archive s JOIN weave_concept_lists_archive p ON p.id=s.concept_list_id AND p.version_number=s.version_number
WHERE p.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT sqlc.arg(restricted)::boolean OR (s.concept_list_id = ANY(sqlc.arg(selected_concept_list)::text[])))
ORDER BY s.concept_list_id, s.position, s.id
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContentConceptLists :many
SELECT s.id AS id,
       (CASE WHEN isfinite(s.created_at) THEN to_char(s.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.created_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.created_at::text,'') END)::text AS created_at,
       (CASE WHEN isfinite(s.updated_at) THEN to_char(s.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.updated_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.updated_at::text,'') END)::text AS updated_at,
       s.semantic_id AS semantic_id,
       s.system_name AS system_name,
       s.ui_name AS ui_name,
       s.description AS description,
       s.status AS status,
       s.project_id AS project_id,
       s.list_type AS list_type,
       s.vocabulary_id AS vocabulary_id,
       s.version_number AS version_number,
       s.is_closed AS is_closed
FROM weave_concept_lists_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT sqlc.arg(restricted)::boolean OR (s.id = ANY(sqlc.arg(selected_concept_list)::text[])))
ORDER BY s.id
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContentForks :many
SELECT s.project_id AS project_id,
       s.entity_type AS entity_type,
       s.fork_entity_id AS fork_entity_id,
       s.source_project_id AS source_project_id,
       s.source_entity_id AS source_entity_id,
       s.source_version AS source_version,
       (CASE WHEN isfinite(s.forked_at) THEN to_char(s.forked_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.forked_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.forked_at::text,'') END)::text AS forked_at,
       s.created_by_id AS created_by_id,
       s.version_number AS version_number
FROM weave_entity_forks_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT sqlc.arg(restricted)::boolean OR ((s.entity_type = 'model' AND s.fork_entity_id = ANY(sqlc.arg(selected_model)::text[])) OR (s.entity_type = 'collection' AND s.fork_entity_id = ANY(sqlc.arg(selected_collection)::text[])) OR (s.entity_type = 'field' AND s.fork_entity_id = ANY(sqlc.arg(selected_field)::text[]))))
ORDER BY s.project_id, s.entity_type, s.fork_entity_id
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContentExampleValues :many
SELECT s.id AS id,
       s.example_id AS example_id,
       s.override_id AS override_id,
       s.field_id AS field_id,
       s.part_of_collection_id AS part_of_collection_id,
       s.occurrence_index AS occurrence_index,
       s.value_kind AS value_kind,
       s.value_payload AS value_payload,
       s.text_value AS text_value,
       COALESCE(s.number_value::text,'')::text AS number_value,
       (CASE WHEN isfinite(s.date_value) THEN to_char(s.date_value,'YYYY-MM-DD') || CASE WHEN EXTRACT(YEAR FROM s.date_value)<0 THEN ' BC' ELSE '' END ELSE COALESCE(s.date_value::text,'') END)::text AS date_value,
       s.uri_value AS uri_value,
       s.concept_uri AS concept_uri,
       s.linked_example_id AS linked_example_id,
       s.slot_path AS slot_path,
       (CASE WHEN isfinite(s.created_at) THEN to_char(s.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.created_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.created_at::text,'') END)::text AS created_at,
       (CASE WHEN isfinite(s.updated_at) THEN to_char(s.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.updated_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.updated_at::text,'') END)::text AS updated_at,
       s.version_number AS version_number
FROM weave_example_values_archive s JOIN weave_examples_archive p ON p.id=s.example_id AND p.version_number=s.version_number
WHERE p.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT sqlc.arg(restricted)::boolean OR (s.example_id = ANY(sqlc.arg(selected_example)::text[])))
ORDER BY s.id
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContentExamples :many
SELECT s.id AS id,
       s.project_id AS project_id,
       s.entity_type AS entity_type,
       s.entity_id AS entity_id,
       s.title AS title,
       s.description AS description,
       s.status AS status,
       (CASE WHEN isfinite(s.created_at) THEN to_char(s.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.created_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.created_at::text,'') END)::text AS created_at,
       (CASE WHEN isfinite(s.updated_at) THEN to_char(s.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.updated_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.updated_at::text,'') END)::text AS updated_at,
       s.version_number AS version_number
FROM weave_examples_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT sqlc.arg(restricted)::boolean OR (s.id = ANY(sqlc.arg(selected_example)::text[])))
ORDER BY s.id
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContentOverrides :many
SELECT s.id AS id,
       s.field_id AS field_id,
       s.project_id AS project_id,
       s.entity_type AS entity_type,
       s.entity_id AS entity_id,
       s.position AS position,
       s.collection_order AS collection_order,
       s.display_name AS display_name,
       s.description AS description,
       s.collection_name AS collection_name,
       s.category_id AS category_id,
       s.part_of_collection_id AS part_of_collection_id,
       s.expected_value_type AS expected_value_type,
       s.set_value AS set_value,
       s.is_required AS is_required,
       s.min_occurs AS min_occurs,
       s.max_occurs AS max_occurs,
       s.is_hidden AS is_hidden,
       s.visibility AS visibility,
       s.staging_id AS staging_id,
       (CASE WHEN isfinite(s.created_at) THEN to_char(s.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.created_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.created_at::text,'') END)::text AS created_at,
       (CASE WHEN isfinite(s.updated_at) THEN to_char(s.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.updated_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.updated_at::text,'') END)::text AS updated_at,
       s.content_hash AS content_hash,
       s.version_number AS version_number,
       s.set_value_entry_id AS set_value_entry_id
FROM weave_field_overrides_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT sqlc.arg(restricted)::boolean OR (s.id::text = ANY(sqlc.arg(selected_override)::text[]) OR (s.entity_type = '' AND s.field_id = ANY(sqlc.arg(selected_field)::text[])) OR ((s.entity_type = 'model' AND s.entity_id = ANY(sqlc.arg(selected_model)::text[])) OR (s.entity_type = 'collection' AND s.entity_id = ANY(sqlc.arg(selected_collection)::text[])) OR (s.entity_type = 'field' AND s.entity_id = ANY(sqlc.arg(selected_field)::text[])))))
ORDER BY s.entity_type, s.entity_id, s.position, s.id
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContentFields :many
SELECT s.id AS id,
       (CASE WHEN isfinite(s.created_at) THEN to_char(s.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.created_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.created_at::text,'') END)::text AS created_at,
       (CASE WHEN isfinite(s.updated_at) THEN to_char(s.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.updated_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.updated_at::text,'') END)::text AS updated_at,
       s.semantic_id AS semantic_id,
       s.system_name AS system_name,
       s.ui_name AS ui_name,
       s.description AS description,
       s.status AS status,
       s.project_id AS project_id,
       s.ontology_scope AS ontology_scope,
       s.ontology_path AS ontology_path,
       s.path_elements AS path_elements,
       s.expected_value_type AS expected_value_type,
       s.examples AS examples,
       s.staging_id AS staging_id,
       s.deprecated AS deprecated,
       s.version_number AS version_number,
       s.subfield_paths AS subfield_paths
FROM weave_fields_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT sqlc.arg(restricted)::boolean OR (s.id = ANY(sqlc.arg(selected_field)::text[])))
ORDER BY s.id
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContentModels :many
SELECT s.id AS id,
       (CASE WHEN isfinite(s.created_at) THEN to_char(s.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.created_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.created_at::text,'') END)::text AS created_at,
       (CASE WHEN isfinite(s.updated_at) THEN to_char(s.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.updated_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.updated_at::text,'') END)::text AS updated_at,
       s.system_name AS system_name,
       s.ui_name AS ui_name,
       s.description AS description,
       s.status AS status,
       s.project_id AS project_id,
       s.ontology_scope AS ontology_scope,
       s.staging_id AS staging_id,
       s.deprecated AS deprecated,
       s.version_number AS version_number,
       s.model_type AS model_type
FROM weave_models_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT sqlc.arg(restricted)::boolean OR (s.id = ANY(sqlc.arg(selected_model)::text[])))
ORDER BY s.id
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContentNamespaces :many
SELECT s.release_project_id AS release_project_id,
       s.version_number AS version_number,
       s.id AS id,
       (CASE WHEN isfinite(s.created_at) THEN to_char(s.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.created_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.created_at::text,'') END)::text AS created_at,
       (CASE WHEN isfinite(s.updated_at) THEN to_char(s.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.updated_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.updated_at::text,'') END)::text AS updated_at,
       s.airtable_raw AS airtable_raw,
       s.airtable_context AS airtable_context,
       s.airtable_ref AS airtable_ref,
       s.semantic_id AS semantic_id,
       s.long_semantic_id AS long_semantic_id,
       s.system_name AS system_name,
       s.ui_name AS ui_name,
       s.description AS description,
       s.uri AS uri,
       s.project_id AS project_id,
       s.prefix AS prefix,
       s.namespace AS namespace,
       s.weight AS weight,
       s.source AS source,
       s.ontology_id AS ontology_id
FROM weave_release_namespace_bindings_archive s
WHERE s.release_project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
ORDER BY s.release_project_id, s.id
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContentOverrideRefs :many
SELECT s.override_id AS override_id,
       s.ref_type AS ref_type,
       s.target_id AS target_id,
       s.semantic_id AS semantic_id,
       s.position AS position,
       s.project_id AS project_id,
       s.version_number AS version_number
FROM weave_override_refs_archive s JOIN weave_field_overrides_archive p ON p.id=s.override_id AND p.version_number=s.version_number
WHERE p.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT sqlc.arg(restricted)::boolean OR (s.override_id::text = ANY(sqlc.arg(selected_override)::text[])))
ORDER BY s.override_id, s.ref_type, s.position
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContentProjectActors :many
SELECT s.project_id AS project_id,
       s.actor_id AS actor_id,
       s.role AS role,
       (CASE WHEN isfinite(s.created_at) THEN to_char(s.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.created_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.created_at::text,'') END)::text AS created_at,
       s.version_number AS version_number
FROM weave_project_actors_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
ORDER BY s.project_id, s.actor_id, s.role
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContentAttributions :many
SELECT s.project_id AS project_id,
       s.actor_id AS actor_id,
       s.kind AS kind,
       s.position AS position,
       s.note AS note,
       (CASE WHEN isfinite(s.created_at) THEN to_char(s.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.created_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.created_at::text,'') END)::text AS created_at,
       s.version_number AS version_number
FROM weave_project_attributions_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
ORDER BY s.project_id, s.kind, s.position, s.actor_id
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContentInheritance :many
SELECT s.project_id AS project_id,
       s.parent_project_id AS parent_project_id,
       s.is_primary AS is_primary,
       s.canonical_order AS canonical_order,
       (CASE WHEN isfinite(s.adopted_at) THEN to_char(s.adopted_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.adopted_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.adopted_at::text,'') END)::text AS adopted_at,
       s.version_number AS version_number,
       s.source_mode AS source_mode,
       s.source_version AS source_version
FROM weave_project_inheritance_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT sqlc.arg(restricted)::boolean OR (FALSE))
ORDER BY s.canonical_order, s.parent_project_id
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContentOntologyLinks :many
SELECT s.project_id AS project_id,
       s.ontology_version_id AS ontology_version_id,
       (CASE WHEN isfinite(s.added_at) THEN to_char(s.added_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.added_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.added_at::text,'') END)::text AS added_at,
       s.added_by_id AS added_by_id,
       s.is_primary AS is_primary,
       s.usage_notes AS usage_notes,
       s.version_number AS version_number
FROM weave_project_ontology_versions_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
ORDER BY s.project_id, s.ontology_version_id
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContentProjects :many
SELECT s.id AS id,
       (CASE WHEN isfinite(s.created_at) THEN to_char(s.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.created_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.created_at::text,'') END)::text AS created_at,
       (CASE WHEN isfinite(s.updated_at) THEN to_char(s.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.updated_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.updated_at::text,'') END)::text AS updated_at,
       s.system_name AS system_name,
       s.ui_name AS ui_name,
       s.description AS description,
       s.status AS status,
       s.namespace AS namespace,
       s.parent_project_id AS parent_project_id,
       s.staging_id AS staging_id,
       s.owner_id AS owner_id,
       s.visibility AS visibility,
       s.deprecated AS deprecated,
       s.license AS license,
       s.readme AS readme,
       s.topics AS topics,
       s.base_url AS base_url,
       s.created_by_id AS created_by_id,
       s.version_number AS version_number,
       s.concept_namespace AS concept_namespace,
       s.effective_concept_namespace AS effective_concept_namespace,
       s.enforce_concept_lists AS enforce_concept_lists
FROM weave_projects_archive s
WHERE s.id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
ORDER BY s.id
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContentVocabularies :many
SELECT s.id AS id,
       s.project_id AS project_id,
       s.semantic_id AS semantic_id,
       s.system_name AS system_name,
       s.ui_name AS ui_name,
       s.description AS description,
       s.status AS status,
       s.connector_type AS connector_type,
       s.base_uri AS base_uri,
       s.config AS config,
       s.deprecated AS deprecated,
       (CASE WHEN isfinite(s.created_at) THEN to_char(s.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.created_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.created_at::text,'') END)::text AS created_at,
       (CASE WHEN isfinite(s.updated_at) THEN to_char(s.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.updated_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.updated_at::text,'') END)::text AS updated_at,
       s.version_number AS version_number
FROM weave_vocabularies_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT sqlc.arg(restricted)::boolean OR (s.id = ANY(sqlc.arg(selected_vocabulary)::text[])))
ORDER BY s.id
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContentVocabularyEntries :many
SELECT s.id AS id,
       s.vocabulary_id AS vocabulary_id,
       s.uri AS uri,
       s.label AS label,
       s.scope_note AS scope_note,
       s.broader_uri AS broader_uri,
       s.broader_path AS broader_path,
       s.broader_path_items AS broader_path_items,
       s.external_id AS external_id,
       (CASE WHEN isfinite(s.created_at) THEN to_char(s.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.created_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.created_at::text,'') END)::text AS created_at,
       (CASE WHEN isfinite(s.updated_at) THEN to_char(s.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM s.updated_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE COALESCE(s.updated_at::text,'') END)::text AS updated_at,
       s.version_number AS version_number
FROM weave_vocabulary_entries_archive s JOIN weave_vocabularies_archive p ON p.id=s.vocabulary_id AND p.version_number=s.version_number
WHERE p.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT sqlc.arg(restricted)::boolean OR (s.id = ANY(sqlc.arg(selected_vocabulary_entry)::text[]) OR s.vocabulary_id = ANY(sqlc.arg(selected_vocabulary)::text[])))
ORDER BY s.id
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;
