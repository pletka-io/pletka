-- +goose Up
-- Capturing a verified cut's semantic references took minutes for forty fields
-- against a 2,000-term ontology. Two costs compounded: the guard trigger ran
-- two correlated subqueries over weave_project_semantic_candidates for every
-- resolution, and the view itself called weave_semantic_term_matches (which
-- unnests the alias array) once per reference × term pair. The view now expands
-- each reference into the exact URIs and QName pairs a term may carry and joins
-- terms on equality; the guard evaluates that inventory once per statement.
-- Matching semantics, output columns, checks and messages are unchanged.
CREATE OR REPLACE VIEW weave_project_semantic_candidates AS
WITH bodies AS (
    SELECT p.project_id,p.version_number,p.ontology_id,p.ontology_version_id,p.digest,
        (o.artifact::jsonb->>'body_json')::jsonb AS body
    FROM weave_release_ontology_dependencies p
    JOIN weave_ontology_releases o ON (o.ontology_id,o.version_id,o.digest)=(p.ontology_id,p.ontology_version_id,p.digest)
), bindings AS (
    SELECT release_project_id AS project_id,version_number,
        jsonb_agg(jsonb_build_object('prefix',prefix,'namespace',namespace)) AS aliases
    FROM weave_release_namespace_bindings_archive GROUP BY release_project_id,version_number
), terms AS (
    SELECT b.project_id,b.version_number,b.ontology_id,b.ontology_version_id,b.digest,'class'::text AS kind,
        term->>'uri' AS uri,term->>'prefix' AS prefix,term->>'local_name' AS local_name
    FROM bodies b CROSS JOIN LATERAL jsonb_array_elements(COALESCE(b.body->'classes','[]'::jsonb)) term
    WHERE COALESCE(term->>'uri','')<>''
    UNION ALL
    SELECT b.project_id,b.version_number,b.ontology_id,b.ontology_version_id,b.digest,'property'::text,
        term->>'uri',term->>'prefix',term->>'local_name'
    FROM bodies b CROSS JOIN LATERAL jsonb_array_elements(COALESCE(b.body->'properties','[]'::jsonb)) term
    WHERE COALESCE(term->>'uri','')<>''
), refs AS (
    -- One row per string the reference must match: its URI, its QName, or both.
    SELECT r.project_id,r.version_number,r.entity_kind,r.entity_id,r.location,
        r.reference->>'kind' AS kind,r.reference->>'version_id' AS version_id,r.reference->>'uri' AS ref,
        (CASE WHEN r.reference->>'uri'<>'' THEN 1 ELSE 0 END)+(CASE WHEN r.reference->>'prefix'<>'' THEN 1 ELSE 0 END) AS required
    FROM weave_project_semantic_references r
    WHERE ((r.reference->>'prefix'='')=(r.reference->>'local_name'='')) AND r.reference->>'uri'<>''
    UNION ALL
    SELECT r.project_id,r.version_number,r.entity_kind,r.entity_id,r.location,
        r.reference->>'kind',r.reference->>'version_id',(r.reference->>'prefix')||':'||(r.reference->>'local_name'),
        (CASE WHEN r.reference->>'uri'<>'' THEN 1 ELSE 0 END)+(CASE WHEN r.reference->>'prefix'<>'' THEN 1 ELSE 0 END)
    FROM weave_project_semantic_references r
    WHERE ((r.reference->>'prefix'='')=(r.reference->>'local_name'='')) AND r.reference->>'prefix'<>''
), keys AS (
    -- Storage counterpart of namespace.MatchesTerm, expanded ahead of the join:
    -- the exact URI, the term's own QName, and every declared alias expansion.
    SELECT r.project_id,r.version_number,r.entity_kind,r.entity_id,r.location,r.kind,r.ref,r.required,
        b.ontology_id,b.ontology_version_id,b.digest,
        r.ref AS key_uri,NULL::text AS key_prefix,NULL::text AS key_local
    FROM refs r JOIN bodies b USING (project_id,version_number)
    WHERE r.version_id='' OR r.version_id=b.ontology_version_id
    UNION ALL
    SELECT r.project_id,r.version_number,r.entity_kind,r.entity_id,r.location,r.kind,r.ref,r.required,
        b.ontology_id,b.ontology_version_id,b.digest,
        NULL::text,split_part(r.ref,':',1),substring(r.ref FROM strpos(r.ref,':')+1)
    FROM refs r JOIN bodies b USING (project_id,version_number)
    WHERE (r.version_id='' OR r.version_id=b.ontology_version_id)
      AND strpos(r.ref,':')>1 AND substring(r.ref FROM strpos(r.ref,':')+1)<>''
      AND substring(r.ref FROM strpos(r.ref,':')+1 FOR 2)<>'//'
    UNION ALL
    SELECT r.project_id,r.version_number,r.entity_kind,r.entity_id,r.location,r.kind,r.ref,r.required,
        b.ontology_id,b.ontology_version_id,b.digest,
        (al->>'namespace')||substring(r.ref FROM strpos(r.ref,':')+1),NULL::text,NULL::text
    FROM refs r JOIN bodies b USING (project_id,version_number)
    LEFT JOIN bindings a USING (project_id,version_number)
    CROSS JOIN LATERAL jsonb_array_elements(COALESCE(a.aliases,'[]'::jsonb)||COALESCE(b.body->'namespace_bindings','[]'::jsonb)) al
    WHERE (r.version_id='' OR r.version_id=b.ontology_version_id)
      AND strpos(r.ref,':')>1 AND substring(r.ref FROM strpos(r.ref,':')+1)<>''
      AND substring(r.ref FROM strpos(r.ref,':')+1 FOR 2)<>'//'
      AND al->>'prefix'=split_part(r.ref,':',1)
), matched AS (
    SELECT DISTINCT project_id,version_number,entity_kind,entity_id,location,ref,required,
        ontology_id,ontology_version_id,digest,uri
    FROM (
        SELECT k.project_id,k.version_number,k.entity_kind,k.entity_id,k.location,k.ref,k.required,
            k.ontology_id,k.ontology_version_id,k.digest,t.uri
        FROM keys k
        JOIN terms t ON (t.project_id,t.version_number,t.ontology_id,t.ontology_version_id,t.digest,t.kind)
            =(k.project_id,k.version_number,k.ontology_id,k.ontology_version_id,k.digest,k.kind)
            AND t.uri=k.key_uri
        WHERE k.key_uri IS NOT NULL
        UNION ALL
        SELECT k.project_id,k.version_number,k.entity_kind,k.entity_id,k.location,k.ref,k.required,
            k.ontology_id,k.ontology_version_id,k.digest,t.uri
        FROM keys k
        JOIN terms t ON (t.project_id,t.version_number,t.ontology_id,t.ontology_version_id,t.digest,t.kind)
            =(k.project_id,k.version_number,k.ontology_id,k.ontology_version_id,k.digest,k.kind)
            AND t.prefix=k.key_prefix AND t.local_name=k.key_local
        WHERE k.key_prefix IS NOT NULL
    ) u
)
-- A term is a candidate only when it matches every string the reference carries.
SELECT project_id,version_number,entity_kind,entity_id,location,ontology_id,ontology_version_id,digest,uri
FROM matched
GROUP BY project_id,version_number,entity_kind,entity_id,location,ontology_id,ontology_version_id,digest,uri,required
HAVING count(*)=required;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_guard_project_semantic_references() RETURNS trigger LANGUAGE plpgsql AS $$
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
    -- Every stored reference appears exactly once in the resolutions, unchanged.
    IF (SELECT count(*) FROM weave_project_semantic_references WHERE project_id=NEW.project_id AND version_number=NEW.version_number)
        <> jsonb_array_length(NEW.resolutions) OR EXISTS (
        SELECT 1 FROM weave_project_semantic_references r
        LEFT JOIN (
            SELECT x->>'entity_kind' AS entity_kind,x->>'entity_id' AS entity_id,x->>'location' AS location,
                   x->'reference' AS reference,count(*) AS n
            FROM jsonb_array_elements(NEW.resolutions) x
            GROUP BY 1,2,3,4
        ) s ON s.entity_kind=r.entity_kind AND s.entity_id=r.entity_id AND s.location=r.location AND s.reference=r.reference
        WHERE r.project_id=NEW.project_id AND r.version_number=NEW.version_number AND COALESCE(s.n,0)<>1
    ) THEN
        RAISE EXCEPTION 'semantic inventory omits, duplicates or changes a reference' USING ERRCODE='23514';
    END IF;
    -- Every resolution names the single candidate term at its location.
    IF EXISTS (
        WITH candidates AS MATERIALIZED (
            SELECT entity_kind,entity_id,location,ontology_id,ontology_version_id,digest,uri
            FROM weave_project_semantic_candidates
            WHERE project_id=NEW.project_id AND version_number=NEW.version_number
        ), counts AS (
            SELECT entity_kind,entity_id,location,count(*) AS n FROM candidates GROUP BY 1,2,3
        )
        SELECT 1 FROM jsonb_array_elements(NEW.resolutions) x
        LEFT JOIN counts c ON c.entity_kind=x->>'entity_kind' AND c.entity_id=x->>'entity_id' AND c.location=x->>'location'
        LEFT JOIN candidates m ON m.entity_kind=x->>'entity_kind' AND m.entity_id=x->>'entity_id' AND m.location=x->>'location'
            AND m.ontology_id=x->>'ontology_id' AND m.ontology_version_id=x->>'version_id' AND m.digest=x->>'digest' AND m.uri=x->>'uri'
        WHERE COALESCE(c.n,0)<>1 OR m.entity_kind IS NULL
    ) THEN
        RAISE EXCEPTION 'semantic inventory names a term outside its captured publications' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose Down
-- Restore the per-pair view and per-resolution guard from migration 030.
-- Captured rows are unaffected.
CREATE OR REPLACE VIEW weave_project_semantic_candidates AS
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

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_guard_project_semantic_references() RETURNS trigger LANGUAGE plpgsql AS $$
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
