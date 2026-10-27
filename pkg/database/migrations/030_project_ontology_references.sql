-- +goose Up
-- Legacy subfield paths were never archived; leave historical rows unknown.
ALTER TABLE weave_fields_archive ADD COLUMN subfield_paths jsonb;
-- One inventory for capture and storage coverage. Locations use zero-based JSON
-- pointers; positions stored inside path elements are presentation metadata.
CREATE VIEW weave_project_semantic_elements AS
SELECT project_id, version_number, 'field'::text AS entity_kind, id AS entity_id,
       '/ontology_scope'::text AS location, ontology_scope AS element, 'class'::text AS default_kind
FROM weave_fields_archive
UNION ALL SELECT project_id, version_number, 'model', id, '/ontology_scope', ontology_scope, 'class' FROM weave_models_archive
UNION ALL SELECT project_id, version_number, 'collection', id, '/ontology_scope', ontology_scope, 'class' FROM weave_collections_archive
UNION ALL SELECT f.project_id, f.version_number, 'field', f.id, '/path_elements/' || (p.n-1), p.element, ''
FROM weave_fields_archive f CROSS JOIN LATERAL jsonb_array_elements(NULLIF(f.path_elements,'null'::jsonb)) WITH ORDINALITY p(element,n)
UNION ALL SELECT f.project_id, f.version_number, 'field', f.id,
    '/subfield_paths/' || (s.n-1) || '/path_elements/' || (p.n-1), p.element, ''
FROM weave_fields_archive f
CROSS JOIN LATERAL jsonb_array_elements(NULLIF(f.subfield_paths,'null'::jsonb)) WITH ORDINALITY s(element,n)
CROSS JOIN LATERAL jsonb_array_elements(NULLIF(s.element->'path_elements','null'::jsonb)) WITH ORDINALITY p(element,n);

CREATE VIEW weave_project_semantic_references AS
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

-- Storage counterpart of namespace.MatchesTerm: all declared aliases, exact
-- URI identity and the term's own QName. Never choose a preferred namespace.
-- +goose StatementBegin
CREATE FUNCTION weave_semantic_term_matches(ref text, term jsonb, aliases jsonb) RETURNS boolean
LANGUAGE sql IMMUTABLE AS $$
    SELECT ref<>'' AND COALESCE(term->>'uri','')<>'' AND (
        ref=term->>'uri' OR (strpos(ref,':')>1 AND substring(ref FROM strpos(ref,':')+1)<>''
        AND substring(ref FROM strpos(ref,':')+1 FOR 2)<>'//' AND (
            (split_part(ref,':',1)=term->>'prefix' AND substring(ref FROM strpos(ref,':')+1)=term->>'local_name')
            OR EXISTS(SELECT 1 FROM jsonb_array_elements(aliases) a
                WHERE a->>'prefix'=split_part(ref,':',1)
                  AND (a->>'namespace') || substring(ref FROM strpos(ref,':')+1)=term->>'uri')
        ))
    );
$$;
-- +goose StatementEnd
CREATE VIEW weave_project_semantic_candidates AS
WITH bodies AS (
    SELECT p.project_id,p.version_number,p.ontology_id,p.ontology_version_id,p.digest,
        (o.artifact::jsonb->>'body_json')::jsonb AS body
    FROM weave_release_ontology_dependencies p
    JOIN weave_ontology_releases o ON (o.ontology_id,o.version_id,o.digest)=(p.ontology_id,p.ontology_version_id,p.digest)
), bindings AS (
    SELECT release_project_id AS project_id,version_number,
        jsonb_agg(jsonb_build_object('prefix',prefix,'namespace',namespace)) AS aliases
    FROM weave_release_namespace_bindings_archive GROUP BY release_project_id,version_number
)
SELECT DISTINCT r.project_id,r.version_number,r.entity_kind,r.entity_id,r.location,
    b.ontology_id,b.ontology_version_id,b.digest,term->>'uri' AS uri
FROM weave_project_semantic_references r
JOIN bodies b USING(project_id,version_number)
LEFT JOIN bindings a USING(project_id,version_number)
CROSS JOIN LATERAL jsonb_array_elements(b.body ->
    CASE r.reference->>'kind' WHEN 'class' THEN 'classes' WHEN 'property' THEN 'properties' ELSE '' END) term
WHERE (r.reference->>'version_id'='' OR r.reference->>'version_id'=b.ontology_version_id)
  AND ((r.reference->>'prefix'='')=(r.reference->>'local_name'=''))
  AND (r.reference->>'uri'<>'' OR r.reference->>'prefix'<>'')
  AND (r.reference->>'uri'='' OR weave_semantic_term_matches(r.reference->>'uri',term,COALESCE(a.aliases,'[]') || COALESCE(b.body->'namespace_bindings','[]')))
  AND (r.reference->>'prefix'='' OR weave_semantic_term_matches((r.reference->>'prefix') || ':' || (r.reference->>'local_name'),term,COALESCE(a.aliases,'[]') || COALESCE(b.body->'namespace_bindings','[]')));

-- Presence, including an empty array, certifies semantic coverage independently
-- of migration025's narrower link/version inventory. No historical backfill.
CREATE TABLE weave_release_ontology_references (
    project_id text NOT NULL,
    version_number text NOT NULL,
    resolutions jsonb NOT NULL CHECK (jsonb_typeof(resolutions)='array'),
    PRIMARY KEY(project_id,version_number),
    FOREIGN KEY(project_id,version_number) REFERENCES weave_releases(project_id,version) ON DELETE CASCADE
);
-- +goose StatementBegin
CREATE FUNCTION weave_guard_project_semantic_references() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='TRUNCATE' THEN
        IF EXISTS(SELECT 1 FROM weave_release_ontology_references) THEN
            RAISE EXCEPTION 'cannot truncate captured semantic references' USING ERRCODE='23514';
        END IF;
        RETURN NULL;
    END IF;
    IF TG_OP='UPDATE' THEN
        RAISE EXCEPTION 'captured semantic references are immutable' USING ERRCODE='23514';
    END IF;
    IF TG_OP='DELETE' THEN
        IF EXISTS(SELECT 1 FROM weave_releases WHERE project_id=OLD.project_id AND version=OLD.version_number) THEN
            RAISE EXCEPTION 'semantic references are retained by their release' USING ERRCODE='23514';
        END IF;
        RETURN OLD;
    END IF;
    IF current_setting('transaction_isolation') NOT IN ('repeatable read','serializable') OR NOT EXISTS (
        SELECT 1 FROM weave_releases WHERE project_id=NEW.project_id AND version=NEW.version_number
        AND archive_ownership_version=0 AND ontology_dependencies_version=1
    ) THEN
        RAISE EXCEPTION 'semantic references require an unfinished repeatable-read cut with captured ontology pins' USING ERRCODE='23514';
    END IF;
    IF EXISTS (
        SELECT 1 FROM weave_release_project_dependencies d
        LEFT JOIN weave_release_ontology_references s ON s.project_id=d.source_project_id AND s.version_number=d.source_version
        WHERE d.project_id=NEW.project_id AND d.version_number=NEW.version_number AND s.project_id IS NULL
    ) THEN
        RAISE EXCEPTION 'source project semantic inventory is unverified' USING ERRCODE='23514';
    END IF;
    IF (SELECT count(*) FROM weave_project_semantic_references WHERE project_id=NEW.project_id AND version_number=NEW.version_number)
        <> jsonb_array_length(NEW.resolutions) OR EXISTS (
        SELECT 1 FROM weave_project_semantic_references r WHERE r.project_id=NEW.project_id AND r.version_number=NEW.version_number
        AND (SELECT count(*) FROM jsonb_array_elements(NEW.resolutions) x
            WHERE x->>'entity_kind'=r.entity_kind AND x->>'entity_id'=r.entity_id
              AND x->>'location'=r.location AND x->'reference'=r.reference) <> 1
    ) THEN
        RAISE EXCEPTION 'semantic inventory omits, duplicates or changes a reference' USING ERRCODE='23514';
    END IF;
    IF EXISTS (
        SELECT 1 FROM jsonb_array_elements(NEW.resolutions) x
        WHERE (SELECT count(*) FROM weave_project_semantic_candidates c
            WHERE c.project_id=NEW.project_id AND c.version_number=NEW.version_number
              AND c.entity_kind=x->>'entity_kind' AND c.entity_id=x->>'entity_id' AND c.location=x->>'location') <> 1
        OR NOT EXISTS (
            SELECT 1 FROM weave_project_semantic_candidates c
            WHERE c.project_id=NEW.project_id AND c.version_number=NEW.version_number
              AND c.entity_kind=x->>'entity_kind' AND c.entity_id=x->>'entity_id' AND c.location=x->>'location'
              AND c.ontology_id=x->>'ontology_id' AND c.ontology_version_id=x->>'version_id' AND c.digest=x->>'digest' AND c.uri=x->>'uri'
        )
    ) THEN
        RAISE EXCEPTION 'semantic inventory names a term outside its captured publications' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER guard_project_semantic_references BEFORE INSERT OR UPDATE OR DELETE ON weave_release_ontology_references FOR EACH ROW EXECUTE FUNCTION weave_guard_project_semantic_references();
CREATE TRIGGER guard_project_semantic_references_truncate BEFORE TRUNCATE ON weave_release_ontology_references FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_project_semantic_references();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM weave_release_ontology_references) THEN
        RAISE EXCEPTION 'cannot downgrade captured semantic references';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE weave_release_ontology_references;
DROP FUNCTION weave_guard_project_semantic_references();
DROP VIEW weave_project_semantic_candidates;
DROP FUNCTION weave_semantic_term_matches(text,jsonb,jsonb);
DROP VIEW weave_project_semantic_references;
DROP VIEW weave_project_semantic_elements;
ALTER TABLE weave_fields_archive DROP COLUMN subfield_paths;
