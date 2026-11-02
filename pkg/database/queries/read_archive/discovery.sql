-- name: DiscoverContentIdentities :many
-- One non-recursive expansion inside the bound project/state. Keep selectors in
-- sync with content.sql; conformance checks compare the source predicates.
WITH
-- source: ContentAdoptions
selected_contentadoptions AS MATERIALIZED (
SELECT s.entity_type::text AS kind0,(s.source_entity_id)::text AS id0
FROM weave_adoptions_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT true OR (((s.context_entity_type = 'model' AND s.context_entity_id = ANY(sqlc.arg(selected_model)::text[])) OR (s.context_entity_type = 'collection' AND s.context_entity_id = ANY(sqlc.arg(selected_collection)::text[])) OR (s.context_entity_type = 'field' AND s.context_entity_id = ANY(sqlc.arg(selected_field)::text[]))) OR (s.context_entity_type IN ('','project') AND ((s.entity_type = 'model' AND s.source_entity_id = ANY(sqlc.arg(selected_model)::text[])) OR (s.entity_type = 'collection' AND s.source_entity_id = ANY(sqlc.arg(selected_collection)::text[])) OR (s.entity_type = 'field' AND s.source_entity_id = ANY(sqlc.arg(selected_field)::text[])) OR (s.entity_type = 'category' AND s.source_entity_id = ANY(sqlc.arg(selected_category)::text[]))))))
  AND (cardinality(sqlc.arg(selected_category)::text[]) > 0 OR cardinality(sqlc.arg(selected_collection)::text[]) > 0 OR cardinality(sqlc.arg(selected_field)::text[]) > 0 OR cardinality(sqlc.arg(selected_model)::text[]) > 0)
),
-- source: ContentCategories
selected_contentcategories AS MATERIALIZED (
SELECT 'category'::text AS kind0,(s.id)::text AS id0
FROM weave_categories_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT true OR (s.id = ANY(sqlc.arg(selected_category)::text[])))
  AND (cardinality(sqlc.arg(selected_category)::text[]) > 0)
),
-- source: ContentPlacements
selected_contentplacements AS MATERIALIZED (
SELECT 'model'::text AS kind0,(s.model_id)::text AS id0,
       'collection'::text AS kind1,(s.collection_id)::text AS id1,
       'category'::text AS kind2,(s.category_id)::text AS id2
FROM weave_collection_placements_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT true OR (s.model_id = ANY(sqlc.arg(selected_model)::text[])))
  AND (cardinality(sqlc.arg(selected_model)::text[]) > 0)
),
-- source: ContentCollections
selected_contentcollections AS MATERIALIZED (
SELECT 'collection'::text AS kind0,(s.id)::text AS id0,
       'category'::text AS kind1,(s.default_category_id)::text AS id1
FROM weave_collections_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT true OR (s.id = ANY(sqlc.arg(selected_collection)::text[])))
  AND (cardinality(sqlc.arg(selected_collection)::text[]) > 0)
),
-- source: ContentBroader
selected_contentbroader AS MATERIALIZED (
SELECT 'concept_list'::text AS kind0,(s.scheme_id)::text AS id0,
       'vocabulary_entry'::text AS kind1,(s.concept_id)::text AS id1,
       'vocabulary_entry'::text AS kind2,(s.broader_id)::text AS id2
FROM weave_concept_broader_archive s JOIN weave_concept_lists_archive p ON p.id=s.scheme_id AND p.version_number=s.version_number
WHERE p.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT true OR (s.scheme_id = ANY(sqlc.arg(selected_concept_list)::text[])))
  AND (cardinality(sqlc.arg(selected_concept_list)::text[]) > 0)
),
-- source: ContentConceptEntries
selected_contentconceptentries AS MATERIALIZED (
SELECT 'concept_list'::text AS kind0,(s.concept_list_id)::text AS id0,
       'vocabulary_entry'::text AS kind1,(s.vocabulary_entry_id)::text AS id1
FROM weave_concept_list_entries_archive s JOIN weave_concept_lists_archive p ON p.id=s.concept_list_id AND p.version_number=s.version_number
WHERE p.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT true OR (s.concept_list_id = ANY(sqlc.arg(selected_concept_list)::text[])))
  AND (cardinality(sqlc.arg(selected_concept_list)::text[]) > 0)
),
-- source: ContentConceptLists
selected_contentconceptlists AS MATERIALIZED (
SELECT 'concept_list'::text AS kind0,(s.id)::text AS id0,
       'vocabulary'::text AS kind1,(s.vocabulary_id)::text AS id1,
       'vocabulary_entry'::text AS kind2,(s.list_type)::text AS id2
FROM weave_concept_lists_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT true OR (s.id = ANY(sqlc.arg(selected_concept_list)::text[])))
  AND (cardinality(sqlc.arg(selected_concept_list)::text[]) > 0)
),
-- source: ContentExampleValues
selected_contentexamplevalues AS MATERIALIZED (
SELECT 'example'::text AS kind0,(s.example_id)::text AS id0,
       'example'::text AS kind1,(s.linked_example_id)::text AS id1,
       'field'::text AS kind2,(s.field_id)::text AS id2,
       'override'::text AS kind3,(s.override_id)::text AS id3,
       'collection'::text AS kind4,(s.part_of_collection_id)::text AS id4
FROM weave_example_values_archive s JOIN weave_examples_archive p ON p.id=s.example_id AND p.version_number=s.version_number
WHERE p.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT true OR (s.example_id = ANY(sqlc.arg(selected_example)::text[])))
  AND (cardinality(sqlc.arg(selected_example)::text[]) > 0)
),
-- source: ContentExamples
selected_contentexamples AS MATERIALIZED (
SELECT 'example'::text AS kind0,(s.id)::text AS id0,
       s.entity_type::text AS kind1,(s.entity_id)::text AS id1
FROM weave_examples_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT true OR (s.id = ANY(sqlc.arg(selected_example)::text[])))
  AND (cardinality(sqlc.arg(selected_example)::text[]) > 0)
),
-- source: ContentOverrides
selected_contentoverrides AS MATERIALIZED (
SELECT 'override'::text AS kind0,(s.id)::text AS id0,
       'field'::text AS kind1,(s.field_id)::text AS id1,
       'category'::text AS kind2,(s.category_id)::text AS id2,
       'collection'::text AS kind3,(s.part_of_collection_id)::text AS id3,
       'vocabulary_entry'::text AS kind4,(s.set_value_entry_id)::text AS id4,
       s.entity_type::text AS kind5,(CASE WHEN COALESCE(s.entity_type,'')<>'' THEN s.entity_id ELSE NULL END)::text AS id5
FROM weave_field_overrides_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT true OR (s.id::text IN (SELECT unnest(sqlc.arg(selected_override)::text[])) OR (s.entity_type = '' AND s.field_id = ANY(sqlc.arg(selected_field)::text[])) OR ((s.entity_type = 'model' AND s.entity_id = ANY(sqlc.arg(selected_model)::text[])) OR (s.entity_type = 'collection' AND s.entity_id = ANY(sqlc.arg(selected_collection)::text[])) OR (s.entity_type = 'field' AND s.entity_id = ANY(sqlc.arg(selected_field)::text[])))))
  AND (cardinality(sqlc.arg(selected_collection)::text[]) > 0 OR cardinality(sqlc.arg(selected_field)::text[]) > 0 OR cardinality(sqlc.arg(selected_model)::text[]) > 0 OR cardinality(sqlc.arg(selected_override)::text[]) > 0)
),
-- source: ContentFields
selected_contentfields AS MATERIALIZED (
SELECT 'field'::text AS kind0,(s.id)::text AS id0
FROM weave_fields_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT true OR (s.id = ANY(sqlc.arg(selected_field)::text[])))
  AND (cardinality(sqlc.arg(selected_field)::text[]) > 0)
),
-- source: ContentModels
selected_contentmodels AS MATERIALIZED (
SELECT 'model'::text AS kind0,(s.id)::text AS id0
FROM weave_models_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT true OR (s.id = ANY(sqlc.arg(selected_model)::text[])))
  AND (cardinality(sqlc.arg(selected_model)::text[]) > 0)
),
-- source: ContentOverrideRefs
selected_contentoverriderefs AS MATERIALIZED (
SELECT 'override'::text AS kind0,(s.override_id)::text AS id0,
       CASE s.ref_type WHEN 'resource_model' THEN 'model' WHEN 'collection_model' THEN 'collection' ELSE s.ref_type END::text AS kind1,(COALESCE(NULLIF(s.target_id,''),s.semantic_id))::text AS id1
FROM weave_override_refs_archive s JOIN weave_field_overrides_archive p ON p.id=s.override_id AND p.version_number=s.version_number
WHERE p.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT true OR (s.override_id::text IN (SELECT unnest(sqlc.arg(selected_override)::text[]))))
  AND (cardinality(sqlc.arg(selected_override)::text[]) > 0)
),
-- source: ContentVocabularies
selected_contentvocabularies AS MATERIALIZED (
SELECT 'vocabulary'::text AS kind0,(s.id)::text AS id0
FROM weave_vocabularies_archive s
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT true OR (s.id = ANY(sqlc.arg(selected_vocabulary)::text[])))
  AND (cardinality(sqlc.arg(selected_vocabulary)::text[]) > 0)
),
-- source: ContentVocabularyEntries
selected_contentvocabularyentries AS MATERIALIZED (
SELECT 'vocabulary_entry'::text AS kind0,(s.id)::text AS id0,
       'vocabulary'::text AS kind1,(s.vocabulary_id)::text AS id1
FROM weave_vocabulary_entries_archive s JOIN weave_vocabularies_archive p ON p.id=s.vocabulary_id AND p.version_number=s.version_number
WHERE p.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND (NOT true OR (s.id = ANY(sqlc.arg(selected_vocabulary_entry)::text[]) OR s.vocabulary_id = ANY(sqlc.arg(selected_vocabulary)::text[])))
  AND (cardinality(sqlc.arg(selected_vocabulary)::text[]) > 0 OR cardinality(sqlc.arg(selected_vocabulary_entry)::text[]) > 0)
),
edges AS (
SELECT kind0 AS kind,id0 AS id,false AS present FROM selected_contentadoptions
UNION ALL
SELECT kind0 AS kind,id0 AS id,true AS present FROM selected_contentcategories
UNION ALL
SELECT kind0 AS kind,id0 AS id,false AS present FROM selected_contentplacements
UNION ALL
SELECT kind1 AS kind,id1 AS id,false AS present FROM selected_contentplacements
UNION ALL
SELECT kind2 AS kind,id2 AS id,false AS present FROM selected_contentplacements
UNION ALL
SELECT kind0 AS kind,id0 AS id,true AS present FROM selected_contentcollections
UNION ALL
SELECT kind1 AS kind,id1 AS id,false AS present FROM selected_contentcollections
UNION ALL
SELECT kind0 AS kind,id0 AS id,false AS present FROM selected_contentbroader
UNION ALL
SELECT kind1 AS kind,id1 AS id,false AS present FROM selected_contentbroader
UNION ALL
SELECT kind2 AS kind,id2 AS id,false AS present FROM selected_contentbroader
UNION ALL
SELECT kind0 AS kind,id0 AS id,false AS present FROM selected_contentconceptentries
UNION ALL
SELECT kind1 AS kind,id1 AS id,false AS present FROM selected_contentconceptentries
UNION ALL
SELECT kind0 AS kind,id0 AS id,true AS present FROM selected_contentconceptlists
UNION ALL
SELECT kind1 AS kind,id1 AS id,false AS present FROM selected_contentconceptlists
UNION ALL
SELECT kind2 AS kind,id2 AS id,false AS present FROM selected_contentconceptlists
UNION ALL
SELECT kind0 AS kind,id0 AS id,false AS present FROM selected_contentexamplevalues
UNION ALL
SELECT kind1 AS kind,id1 AS id,false AS present FROM selected_contentexamplevalues
UNION ALL
SELECT kind2 AS kind,id2 AS id,false AS present FROM selected_contentexamplevalues
UNION ALL
SELECT kind3 AS kind,id3 AS id,false AS present FROM selected_contentexamplevalues
UNION ALL
SELECT kind4 AS kind,id4 AS id,false AS present FROM selected_contentexamplevalues
UNION ALL
SELECT kind0 AS kind,id0 AS id,true AS present FROM selected_contentexamples
UNION ALL
SELECT kind1 AS kind,id1 AS id,false AS present FROM selected_contentexamples
UNION ALL
SELECT kind0 AS kind,id0 AS id,true AS present FROM selected_contentoverrides
UNION ALL
SELECT kind1 AS kind,id1 AS id,false AS present FROM selected_contentoverrides
UNION ALL
SELECT kind2 AS kind,id2 AS id,false AS present FROM selected_contentoverrides
UNION ALL
SELECT kind3 AS kind,id3 AS id,false AS present FROM selected_contentoverrides
UNION ALL
SELECT kind4 AS kind,id4 AS id,false AS present FROM selected_contentoverrides
UNION ALL
SELECT kind5 AS kind,id5 AS id,false AS present FROM selected_contentoverrides
UNION ALL
SELECT kind0 AS kind,id0 AS id,true AS present FROM selected_contentfields
UNION ALL
SELECT kind0 AS kind,id0 AS id,true AS present FROM selected_contentmodels
UNION ALL
SELECT kind0 AS kind,id0 AS id,false AS present FROM selected_contentoverriderefs
UNION ALL
SELECT kind1 AS kind,id1 AS id,false AS present FROM selected_contentoverriderefs
UNION ALL
SELECT kind0 AS kind,id0 AS id,true AS present FROM selected_contentvocabularies
UNION ALL
SELECT kind0 AS kind,id0 AS id,true AS present FROM selected_contentvocabularyentries
UNION ALL
SELECT kind1 AS kind,id1 AS id,false AS present FROM selected_contentvocabularyentries
)
SELECT COALESCE(kind,'')::text AS kind,id::text AS id,bool_or(present)::boolean AS present
FROM edges WHERE id IS NOT NULL AND id<>'' AND id<>'0'
GROUP BY kind,id
ORDER BY kind COLLATE "C",id COLLATE "C"
LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint;
