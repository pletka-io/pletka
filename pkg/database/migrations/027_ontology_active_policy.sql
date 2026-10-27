-- +goose Up
-- Move policy without inventing a selection or certifying historical content.
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM weave_ontology_versions WHERE is_active GROUP BY ontology_id HAVING count(*)>1) THEN
        RAISE EXCEPTION 'ambiguous active ontology selections; review before migration';
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE weave_ontology_versions ADD CONSTRAINT weave_ontology_versions_owner_id_unique UNIQUE(ontology_id,id);
CREATE TABLE weave_ontology_active_versions (
    ontology_id text PRIMARY KEY REFERENCES weave_ontologies(id) ON DELETE CASCADE,
    version_id text NOT NULL,
    selected_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(ontology_id,version_id) REFERENCES weave_ontology_versions(ontology_id,id) ON DELETE CASCADE
);
INSERT INTO weave_ontology_active_versions(ontology_id,version_id)
SELECT ontology_id,id FROM weave_ontology_versions WHERE is_active;
ALTER TABLE weave_ontology_versions DROP COLUMN is_active;

-- Keep live API/UI status derived, with no second mutable copy of the flag.
CREATE VIEW weave_ontology_version_status AS
SELECT v.*, (p.version_id IS NOT NULL)::boolean AS is_active
FROM weave_ontology_versions v
LEFT JOIN weave_ontology_active_versions p ON p.ontology_id=v.ontology_id AND p.version_id=v.id;

-- +goose StatementBegin
CREATE FUNCTION weave_set_ontology_active_version(selected_owner text, selected_version text, only_if_unset boolean) RETURNS boolean LANGUAGE plpgsql AS $$
BEGIN
    PERFORM 1 FROM weave_ontology_versions WHERE ontology_id=selected_owner AND id=selected_version FOR KEY SHARE;
    IF NOT FOUND THEN RETURN false; END IF;
    INSERT INTO weave_ontology_active_versions(ontology_id,version_id)
    VALUES(selected_owner,selected_version)
    ON CONFLICT(ontology_id) DO UPDATE SET version_id=EXCLUDED.version_id, selected_at=now()
    WHERE NOT only_if_unset AND weave_ontology_active_versions.version_id IS DISTINCT FROM EXCLUDED.version_id;
    RETURN true;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_guard_ontology_version_body() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP <> 'INSERT' AND OLD.body_sealed_at IS NOT NULL THEN
        -- Active selection is separate policy. Sealed version rows permit only no-ops.
        IF TG_OP = 'UPDATE' AND to_jsonb(NEW) = to_jsonb(OLD) THEN
            RETURN NEW;
        END IF;
        RAISE EXCEPTION 'ontology version body % is sealed; create a new version', OLD.id
            USING ERRCODE = '23514', CONSTRAINT = 'ontology_body_sealed';
    END IF;
    IF TG_OP <> 'DELETE' AND NEW.body_sealed_at IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM weave_ontology_body_seals s WHERE s.version_id = NEW.id
          AND s.ontology_id = NEW.ontology_id AND s.sealed_at = NEW.body_sealed_at
    ) THEN
        RAISE EXCEPTION 'ontology body seal must be created through the sealing operation'
            USING ERRCODE = '23514', CONSTRAINT = 'ontology_body_sealed';
    END IF;
    IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_restore_ontology_artifact(payload text, expected_digest text) RETURNS void LANGUAGE plpgsql AS $$
DECLARE
    doc jsonb := payload::jsonb;
    body jsonb := (doc->>'body_json')::jsonb;
    owner_id text := doc->>'ontology_id';
    release_id text := doc->>'version_id';
    existing weave_ontology_releases%ROWTYPE;
BEGIN
    IF current_setting('transaction_isolation') NOT IN ('repeatable read','serializable') THEN
        RAISE EXCEPTION 'artifact restore requires repeatable read' USING ERRCODE='23514';
    END IF;
    IF encode(sha256(convert_to(payload,'UTF8')),'hex') IS DISTINCT FROM expected_digest
       OR encode(sha256(convert_to(doc->>'body_json','UTF8')),'hex') IS DISTINCT FROM doc->>'body_digest'
       OR (doc->>'format_version') IS DISTINCT FROM '1'
       OR (body#>>'{version,id}') IS DISTINCT FROM release_id
       OR (body#>>'{version,ontology_id}') IS DISTINCT FROM owner_id THEN
        RAISE EXCEPTION 'invalid ontology artifact identity or digest' USING ERRCODE='23514',CONSTRAINT='ontology_artifact_conflict';
    END IF;
    -- Serialize restores of the same stable identity, including the absent row.
    PERFORM pg_advisory_xact_lock(hashtextextended('ontology-artifact:' || release_id,0));
    SELECT * INTO existing FROM weave_ontology_releases WHERE version_id=release_id;
    IF FOUND THEN
        IF existing.ontology_id=owner_id AND existing.digest=expected_digest AND existing.artifact=payload THEN RETURN; END IF;
        RAISE EXCEPTION 'existing ontology ID has different published content' USING ERRCODE='23514',CONSTRAINT='ontology_artifact_conflict';
    END IF;
    IF EXISTS(SELECT 1 FROM weave_ontology_versions WHERE id=release_id) THEN
        RAISE EXCEPTION 'existing ontology ID is unverified; refusing to overwrite or certify it' USING ERRCODE='23514',CONSTRAINT='ontology_artifact_conflict';
    END IF;
    IF EXISTS (SELECT 1 FROM jsonb_array_elements(body->'classes') r WHERE r->>'ontology_version_id' IS DISTINCT FROM release_id)
       OR EXISTS (SELECT 1 FROM jsonb_array_elements(body->'properties') r WHERE r->>'ontology_version_id' IS DISTINCT FROM release_id)
       OR EXISTS (SELECT 1 FROM jsonb_array_elements(body->'relations') r WHERE r->>'ontology_version_id' IS DISTINCT FROM release_id)
       OR EXISTS (SELECT 1 FROM jsonb_array_elements(body->'companions') r WHERE r->>'ontology_version_id' IS DISTINCT FROM release_id) THEN
        RAISE EXCEPTION 'artifact contains foreign indexed content' USING ERRCODE='23514',CONSTRAINT='ontology_artifact_conflict';
    END IF;
    INSERT INTO weave_ontologies(id,prefix,namespace,name,description,family_id,ontology_type,extends_ontology_id,homepage_url,source_url,created_by_id)
    SELECT o.id,o.prefix,o.namespace,o.name,o.description,
        CASE WHEN EXISTS(SELECT 1 FROM weave_ontology_families f WHERE f.id=o.family_id) THEN o.family_id END,
        o.ontology_type,o.extends_ontology_id,o.homepage_url,o.source_url,o.created_by_id
    FROM jsonb_populate_record(NULL::weave_ontologies,body->'ontology') o ON CONFLICT(id) DO NOTHING;
    INSERT INTO weave_ontology_versions(id,ontology_id,version_string,compatible_base_versions,rdf_content,original_filename,file_size,file_md5,ontology_uri,version_iri,version_info,imported_ontologies,ontology_label,ontology_comment,ontology_metadata,class_count,property_count)
    SELECT v.id,v.ontology_id,v.version_string,v.compatible_base_versions,v.rdf_content,v.original_filename,v.file_size,v.file_md5,v.ontology_uri,v.version_iri,v.version_info,v.imported_ontologies,v.ontology_label,v.ontology_comment,v.ontology_metadata,v.class_count,v.property_count
    FROM jsonb_populate_record(NULL::weave_ontology_versions,body->'version') v;
    INSERT INTO weave_ontology_classes(id,ontology_version_id,prefix,local_name,uri,label,comment,natural_sort_key,source_module)
    SELECT c.id,c.ontology_version_id,c.prefix,c.local_name,c.uri,c.label,c.comment,c.natural_sort_key,c.source_module
    FROM jsonb_populate_recordset(NULL::weave_ontology_classes,body->'classes') c;
    INSERT INTO weave_ontology_properties(id,ontology_version_id,prefix,local_name,uri,label,comment,property_type,is_functional,is_inverse_functional,is_transitive,is_symmetric,is_asymmetric,is_reflexive,is_irreflexive,inverse_property_uri,natural_sort_key,source_module)
    SELECT p.id,p.ontology_version_id,p.prefix,p.local_name,p.uri,p.label,p.comment,p.property_type,p.is_functional,p.is_inverse_functional,p.is_transitive,p.is_symmetric,p.is_asymmetric,p.is_reflexive,p.is_irreflexive,p.inverse_property_uri,p.natural_sort_key,p.source_module
    FROM jsonb_populate_recordset(NULL::weave_ontology_properties,body->'properties') p;
    INSERT INTO weave_ontology_relations(ontology_version_id,source_id,source_kind,rel_type,target_qname,position)
    SELECT r.ontology_version_id,r.source_id,r.source_kind,r.rel_type,r.target_qname,r.position
    FROM jsonb_populate_recordset(NULL::weave_ontology_relations,body->'relations') r;
    INSERT INTO weave_ontology_version_companions(ontology_version_id,filename,description,content,position)
    SELECT c.ontology_version_id,c.filename,c.description,c.content,c.position
    FROM jsonb_populate_recordset(NULL::weave_ontology_version_companions,body->'companions') c;
    INSERT INTO weave_ontology_body_seals(ontology_id,version_id,body_text) VALUES(owner_id,release_id,doc->>'body_json');
    INSERT INTO weave_ontology_releases(ontology_id,version_id,artifact,digest) VALUES(owner_id,release_id,payload,expected_digest);
    PERFORM weave_set_ontology_active_version(owner_id,release_id,true);
END $$;
-- +goose StatementEnd

-- +goose Down
-- Selected identities can be represented in the former one-active-version flag.
-- That older schema has no selection timestamp; artifact bytes are unaffected.
DROP VIEW weave_ontology_version_status;
ALTER TABLE weave_ontology_versions ADD COLUMN is_active boolean NOT NULL DEFAULT false;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_guard_ontology_version_body() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP <> 'INSERT' AND OLD.body_sealed_at IS NOT NULL THEN
        -- Active selection is legacy mutable policy, excluded from the body.
        -- Even audit timestamps must not change as a side effect of selection.
        IF TG_OP = 'UPDATE' AND (to_jsonb(NEW) - 'is_active') = (to_jsonb(OLD) - 'is_active') THEN
            RETURN NEW;
        END IF;
        RAISE EXCEPTION 'ontology version body % is sealed; create a new version', OLD.id
            USING ERRCODE = '23514', CONSTRAINT = 'ontology_body_sealed';
    END IF;
    IF TG_OP <> 'DELETE' AND NEW.body_sealed_at IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM weave_ontology_body_seals s WHERE s.version_id = NEW.id
          AND s.ontology_id = NEW.ontology_id AND s.sealed_at = NEW.body_sealed_at
    ) THEN
        RAISE EXCEPTION 'ontology body seal must be created through the sealing operation'
            USING ERRCODE = '23514', CONSTRAINT = 'ontology_body_sealed';
    END IF;
    IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd

UPDATE weave_ontology_versions v SET is_active=true
FROM weave_ontology_active_versions p WHERE v.ontology_id=p.ontology_id AND v.id=p.version_id;
CREATE UNIQUE INDEX idx_weave_ontology_versions_active_per_ontology ON weave_ontology_versions(ontology_id) WHERE is_active=true;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_restore_ontology_artifact(payload text, expected_digest text) RETURNS void LANGUAGE plpgsql AS $$
DECLARE
    doc jsonb := payload::jsonb;
    body jsonb := (doc->>'body_json')::jsonb;
    owner_id text := doc->>'ontology_id';
    release_id text := doc->>'version_id';
    existing weave_ontology_releases%ROWTYPE;
BEGIN
    IF current_setting('transaction_isolation') NOT IN ('repeatable read','serializable') THEN
        RAISE EXCEPTION 'artifact restore requires repeatable read' USING ERRCODE='23514';
    END IF;
    IF encode(sha256(convert_to(payload,'UTF8')),'hex') IS DISTINCT FROM expected_digest
       OR encode(sha256(convert_to(doc->>'body_json','UTF8')),'hex') IS DISTINCT FROM doc->>'body_digest'
       OR (doc->>'format_version') IS DISTINCT FROM '1'
       OR (body#>>'{version,id}') IS DISTINCT FROM release_id
       OR (body#>>'{version,ontology_id}') IS DISTINCT FROM owner_id THEN
        RAISE EXCEPTION 'invalid ontology artifact identity or digest' USING ERRCODE='23514',CONSTRAINT='ontology_artifact_conflict';
    END IF;
    -- Serialize restores of the same stable identity, including the absent row.
    PERFORM pg_advisory_xact_lock(hashtextextended('ontology-artifact:' || release_id,0));
    SELECT * INTO existing FROM weave_ontology_releases WHERE version_id=release_id;
    IF FOUND THEN
        IF existing.ontology_id=owner_id AND existing.digest=expected_digest AND existing.artifact=payload THEN RETURN; END IF;
        RAISE EXCEPTION 'existing ontology ID has different published content' USING ERRCODE='23514',CONSTRAINT='ontology_artifact_conflict';
    END IF;
    IF EXISTS(SELECT 1 FROM weave_ontology_versions WHERE id=release_id) THEN
        RAISE EXCEPTION 'existing ontology ID is unverified; refusing to overwrite or certify it' USING ERRCODE='23514',CONSTRAINT='ontology_artifact_conflict';
    END IF;
    IF EXISTS (SELECT 1 FROM jsonb_array_elements(body->'classes') r WHERE r->>'ontology_version_id' IS DISTINCT FROM release_id)
       OR EXISTS (SELECT 1 FROM jsonb_array_elements(body->'properties') r WHERE r->>'ontology_version_id' IS DISTINCT FROM release_id)
       OR EXISTS (SELECT 1 FROM jsonb_array_elements(body->'relations') r WHERE r->>'ontology_version_id' IS DISTINCT FROM release_id)
       OR EXISTS (SELECT 1 FROM jsonb_array_elements(body->'companions') r WHERE r->>'ontology_version_id' IS DISTINCT FROM release_id) THEN
        RAISE EXCEPTION 'artifact contains foreign indexed content' USING ERRCODE='23514',CONSTRAINT='ontology_artifact_conflict';
    END IF;
    INSERT INTO weave_ontologies(id,prefix,namespace,name,description,family_id,ontology_type,extends_ontology_id,homepage_url,source_url,created_by_id)
    SELECT o.id,o.prefix,o.namespace,o.name,o.description,
        CASE WHEN EXISTS(SELECT 1 FROM weave_ontology_families f WHERE f.id=o.family_id) THEN o.family_id END,
        o.ontology_type,o.extends_ontology_id,o.homepage_url,o.source_url,o.created_by_id
    FROM jsonb_populate_record(NULL::weave_ontologies,body->'ontology') o ON CONFLICT(id) DO NOTHING;
    INSERT INTO weave_ontology_versions(id,ontology_id,version_string,compatible_base_versions,rdf_content,original_filename,file_size,file_md5,ontology_uri,version_iri,version_info,imported_ontologies,ontology_label,ontology_comment,ontology_metadata,class_count,property_count)
    SELECT v.id,v.ontology_id,v.version_string,v.compatible_base_versions,v.rdf_content,v.original_filename,v.file_size,v.file_md5,v.ontology_uri,v.version_iri,v.version_info,v.imported_ontologies,v.ontology_label,v.ontology_comment,v.ontology_metadata,v.class_count,v.property_count
    FROM jsonb_populate_record(NULL::weave_ontology_versions,body->'version') v;
    INSERT INTO weave_ontology_classes(id,ontology_version_id,prefix,local_name,uri,label,comment,natural_sort_key,source_module)
    SELECT c.id,c.ontology_version_id,c.prefix,c.local_name,c.uri,c.label,c.comment,c.natural_sort_key,c.source_module
    FROM jsonb_populate_recordset(NULL::weave_ontology_classes,body->'classes') c;
    INSERT INTO weave_ontology_properties(id,ontology_version_id,prefix,local_name,uri,label,comment,property_type,is_functional,is_inverse_functional,is_transitive,is_symmetric,is_asymmetric,is_reflexive,is_irreflexive,inverse_property_uri,natural_sort_key,source_module)
    SELECT p.id,p.ontology_version_id,p.prefix,p.local_name,p.uri,p.label,p.comment,p.property_type,p.is_functional,p.is_inverse_functional,p.is_transitive,p.is_symmetric,p.is_asymmetric,p.is_reflexive,p.is_irreflexive,p.inverse_property_uri,p.natural_sort_key,p.source_module
    FROM jsonb_populate_recordset(NULL::weave_ontology_properties,body->'properties') p;
    INSERT INTO weave_ontology_relations(ontology_version_id,source_id,source_kind,rel_type,target_qname,position)
    SELECT r.ontology_version_id,r.source_id,r.source_kind,r.rel_type,r.target_qname,r.position
    FROM jsonb_populate_recordset(NULL::weave_ontology_relations,body->'relations') r;
    INSERT INTO weave_ontology_version_companions(ontology_version_id,filename,description,content,position)
    SELECT c.ontology_version_id,c.filename,c.description,c.content,c.position
    FROM jsonb_populate_recordset(NULL::weave_ontology_version_companions,body->'companions') c;
    INSERT INTO weave_ontology_body_seals(ontology_id,version_id,body_text) VALUES(owner_id,release_id,doc->>'body_json');
    INSERT INTO weave_ontology_releases(ontology_id,version_id,artifact,digest) VALUES(owner_id,release_id,payload,expected_digest);
    UPDATE weave_ontology_versions SET is_active=true WHERE id=release_id
        AND NOT EXISTS(SELECT 1 FROM weave_ontology_versions WHERE ontology_id=owner_id AND is_active);
END $$;
-- +goose StatementEnd

DROP FUNCTION weave_set_ontology_active_version(text,text,boolean);
DROP TABLE weave_ontology_active_versions;
ALTER TABLE weave_ontology_versions DROP CONSTRAINT weave_ontology_versions_owner_id_unique;
