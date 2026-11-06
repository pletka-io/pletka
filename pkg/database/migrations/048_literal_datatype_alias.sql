-- +goose Up
-- Path elements write the generic literal datatype as "rdf:literal", a house
-- convention that names no RDF term; the standards term is rdfs:Literal, which
-- the path builder substitutes. The derived datatype reference follows the
-- same alias so a literal terminus resolves against the defaults ontology.
-- The stored element is untouched; ontologyevidence.elementReferences is the
-- Go twin of this view and applies the same alias. Captured rows are not
-- rewritten: evidence captured before this migration is re-captured by the
-- next rebuild.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_semantic_element_references(element jsonb, default_kind text, base_location text)
RETURNS TABLE(location text, reference jsonb) LANGUAGE sql IMMUTABLE AS $$
    WITH refs AS (
        SELECT base_location AS location,element,
            CASE lower(COALESCE(NULLIF(element->>'type',''),default_kind))
                WHEN 'literal' THEN 'class' ELSE lower(COALESCE(NULLIF(element->>'type',''),default_kind)) END AS kind
        WHERE element IS NOT NULL AND element <> 'null'::jsonb AND element <> '{}'::jsonb
          AND lower(COALESCE(element->>'type','')) <> 'complete'
          AND (COALESCE(element->>'type','') <> '' OR COALESCE(element->>'uri','') <> '' OR COALESCE(element->>'prefix','') <> '' OR COALESCE(element->>'local_name','') <> '' OR COALESCE(element->>'ontology_version_id','') <> '')
        UNION ALL SELECT base_location || '/additional_types/' || (a.n-1),a.element,'class'
        FROM jsonb_array_elements(NULLIF(element->'additional_types','null'::jsonb)) WITH ORDINALITY a(element,n)
        UNION ALL SELECT base_location || '/datatype',
            jsonb_build_object('uri',CASE WHEN element->>'datatype' IN ('rdf:literal','rdf:Literal') THEN 'rdfs:Literal' ELSE element->>'datatype' END),'class'
        WHERE COALESCE(element->>'datatype','') <> ''
    )
    SELECT location,jsonb_build_object('kind',kind,'uri',COALESCE(element->>'uri',''),
        'prefix',COALESCE(element->>'prefix',''),'local_name',COALESCE(element->>'local_name',''),
        'version_id',COALESCE(element->>'ontology_version_id','')) FROM refs;
$$;
-- +goose StatementEnd
CREATE OR REPLACE VIEW weave_project_semantic_references AS
WITH refs AS (
    SELECT project_id, version_number, entity_kind, entity_id, location, element,
        CASE lower(COALESCE(NULLIF(element->>'type',''),default_kind))
            WHEN 'literal' THEN 'class' ELSE lower(COALESCE(NULLIF(element->>'type',''),default_kind)) END AS kind
    FROM weave_project_semantic_elements
    WHERE element IS NOT NULL AND element <> 'null'::jsonb AND element <> '{}'::jsonb
      AND lower(COALESCE(element->>'type','')) <> 'complete'
      AND (COALESCE(element->>'type','') <> '' OR COALESCE(element->>'uri','') <> '' OR COALESCE(element->>'prefix','') <> '' OR COALESCE(element->>'local_name','') <> '' OR COALESCE(element->>'ontology_version_id','') <> '')
    UNION ALL SELECT project_id, version_number, entity_kind, entity_id, location || '/additional_types/' || (a.n-1), a.element, 'class'
    FROM weave_project_semantic_elements e
    CROSS JOIN LATERAL jsonb_array_elements(NULLIF(e.element->'additional_types','null'::jsonb)) WITH ORDINALITY a(element,n)
    UNION ALL SELECT project_id, version_number, entity_kind, entity_id, location || '/datatype',
        -- "rdf:literal" is the house spelling of a literal with no specific
        -- datatype; it names no RDF term. The derived reference follows the
        -- alias to rdfs:Literal (as the path builder does); the element keeps it.
        jsonb_build_object('uri',CASE WHEN element->>'datatype' IN ('rdf:literal','rdf:Literal') THEN 'rdfs:Literal' ELSE element->>'datatype' END), 'class'
    FROM weave_project_semantic_elements WHERE COALESCE(element->>'datatype','') <> ''
)
SELECT project_id, version_number, entity_kind, entity_id, location,
    jsonb_build_object('kind',kind,'uri',COALESCE(element->>'uri',''),
        'prefix',COALESCE(element->>'prefix',''),'local_name',COALESCE(element->>'local_name',''),
        'version_id',COALESCE(element->>'ontology_version_id','')) AS reference
FROM refs;

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_semantic_element_references(element jsonb, default_kind text, base_location text)
RETURNS TABLE(location text, reference jsonb) LANGUAGE sql IMMUTABLE AS $$
    WITH refs AS (
        SELECT base_location AS location,element,
            CASE lower(COALESCE(NULLIF(element->>'type',''),default_kind))
                WHEN 'literal' THEN 'class' ELSE lower(COALESCE(NULLIF(element->>'type',''),default_kind)) END AS kind
        WHERE element IS NOT NULL AND element <> 'null'::jsonb AND element <> '{}'::jsonb
          AND lower(COALESCE(element->>'type','')) <> 'complete'
          AND (COALESCE(element->>'type','') <> '' OR COALESCE(element->>'uri','') <> '' OR COALESCE(element->>'prefix','') <> '' OR COALESCE(element->>'local_name','') <> '' OR COALESCE(element->>'ontology_version_id','') <> '')
        UNION ALL SELECT base_location || '/additional_types/' || (a.n-1),a.element,'class'
        FROM jsonb_array_elements(NULLIF(element->'additional_types','null'::jsonb)) WITH ORDINALITY a(element,n)
        UNION ALL SELECT base_location || '/datatype',jsonb_build_object('uri',element->>'datatype'),'class'
        WHERE COALESCE(element->>'datatype','') <> ''
    )
    SELECT location,jsonb_build_object('kind',kind,'uri',COALESCE(element->>'uri',''),
        'prefix',COALESCE(element->>'prefix',''),'local_name',COALESCE(element->>'local_name',''),
        'version_id',COALESCE(element->>'ontology_version_id','')) FROM refs;
$$;
-- +goose StatementEnd
CREATE OR REPLACE VIEW weave_project_semantic_references AS
WITH refs AS (
    SELECT project_id, version_number, entity_kind, entity_id, location, element,
        CASE lower(COALESCE(NULLIF(element->>'type',''),default_kind))
            WHEN 'literal' THEN 'class' ELSE lower(COALESCE(NULLIF(element->>'type',''),default_kind)) END AS kind
    FROM weave_project_semantic_elements
    WHERE element IS NOT NULL AND element <> 'null'::jsonb AND element <> '{}'::jsonb
      AND lower(COALESCE(element->>'type','')) <> 'complete'
      AND (COALESCE(element->>'type','') <> '' OR COALESCE(element->>'uri','') <> '' OR COALESCE(element->>'prefix','') <> '' OR COALESCE(element->>'local_name','') <> '' OR COALESCE(element->>'ontology_version_id','') <> '')
    UNION ALL SELECT project_id, version_number, entity_kind, entity_id, location || '/additional_types/' || (a.n-1), a.element, 'class'
    FROM weave_project_semantic_elements e
    CROSS JOIN LATERAL jsonb_array_elements(NULLIF(e.element->'additional_types','null'::jsonb)) WITH ORDINALITY a(element,n)
    UNION ALL SELECT project_id, version_number, entity_kind, entity_id, location || '/datatype',
        jsonb_build_object('uri',element->>'datatype'), 'class'
    FROM weave_project_semantic_elements WHERE COALESCE(element->>'datatype','') <> ''
)
SELECT project_id, version_number, entity_kind, entity_id, location,
    jsonb_build_object('kind',kind,'uri',COALESCE(element->>'uri',''),
        'prefix',COALESCE(element->>'prefix',''),'local_name',COALESCE(element->>'local_name',''),
        'version_id',COALESCE(element->>'ontology_version_id','')) AS reference
FROM refs;
