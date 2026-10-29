-- +goose Up
-- Exact local-body restore shared by published artifacts and legacy evidence.
-- +goose StatementBegin
CREATE FUNCTION weave_restore_ontology_body(owner_id text, release_id text, payload text, expected_digest text) RETURNS void LANGUAGE plpgsql
SET search_path=public,pg_temp AS $$
DECLARE body jsonb := payload::jsonb; existing weave_ontology_body_seals%ROWTYPE;
BEGIN
    IF current_setting('transaction_isolation') NOT IN ('repeatable read','serializable') THEN
        RAISE EXCEPTION 'body restore requires repeatable read' USING ERRCODE='23514',CONSTRAINT='ontology_artifact_conflict';
    END IF;
    IF COALESCE(btrim(owner_id),'')='' OR COALESCE(btrim(release_id),'')=''
       OR expected_digest IS NULL OR payload IS NULL
       OR encode(sha256(convert_to(payload,'UTF8')),'hex') IS DISTINCT FROM expected_digest
       OR (body->>'format_version') IS DISTINCT FROM '1'
       OR (body#>>'{ontology,id}') IS DISTINCT FROM owner_id
       OR (body#>>'{version,id}') IS DISTINCT FROM release_id
       OR (body#>>'{version,ontology_id}') IS DISTINCT FROM owner_id
       OR jsonb_typeof(body->'ontology') IS DISTINCT FROM 'object'
       OR jsonb_typeof(body->'version') IS DISTINCT FROM 'object'
       OR jsonb_typeof(body->'classes') IS DISTINCT FROM 'array'
       OR jsonb_typeof(body->'properties') IS DISTINCT FROM 'array'
       OR jsonb_typeof(body->'relations') IS DISTINCT FROM 'array'
       OR jsonb_typeof(body->'companions') IS DISTINCT FROM 'array'
       OR jsonb_typeof(body->'namespace_bindings') IS DISTINCT FROM 'array' THEN
        RAISE EXCEPTION 'invalid ontology body identity, shape or digest' USING ERRCODE='23514',CONSTRAINT='ontology_artifact_conflict';
    END IF;
    -- Same absent-row fence as published restore. Existing sealed/published
    -- content is reusable only byte-for-byte; unsealed history is not certified.
    PERFORM pg_advisory_xact_lock(hashtextextended('ontology-artifact:' || release_id,0));
    SELECT * INTO existing FROM weave_ontology_body_seals WHERE version_id=release_id;
    IF FOUND THEN
        IF existing.ontology_id=owner_id AND existing.digest=expected_digest
           AND COALESCE(existing.body_text,existing.body::text)=payload THEN RETURN; END IF;
        RAISE EXCEPTION 'existing ontology body has different sealed content' USING ERRCODE='23514',CONSTRAINT='ontology_artifact_conflict';
    END IF;
    IF EXISTS(SELECT 1 FROM weave_ontology_versions WHERE id=release_id) THEN
        RAISE EXCEPTION 'existing ontology body is unsealed; refusing to overwrite or certify it' USING ERRCODE='23514',CONSTRAINT='ontology_artifact_conflict';
    END IF;
    IF EXISTS (SELECT 1 FROM jsonb_array_elements(body->'classes') r WHERE r->>'ontology_version_id' IS DISTINCT FROM release_id)
       OR EXISTS (SELECT 1 FROM jsonb_array_elements(body->'properties') r WHERE r->>'ontology_version_id' IS DISTINCT FROM release_id)
       OR EXISTS (SELECT 1 FROM jsonb_array_elements(body->'relations') r WHERE r->>'ontology_version_id' IS DISTINCT FROM release_id)
       -- Companion ownership is char(26), whose JSON rendering retains padding.
       -- Compare with bpchar semantics without truncating either identity. The
       -- seal still verifies and retains the original, unmodified body bytes.
       OR EXISTS (SELECT 1 FROM jsonb_array_elements(body->'companions') r WHERE (r->>'ontology_version_id')::bpchar IS DISTINCT FROM release_id::bpchar) THEN
        RAISE EXCEPTION 'body contains foreign indexed content' USING ERRCODE='23514',CONSTRAINT='ontology_artifact_conflict';
    END IF;
    -- Optional catalog relationships may be absent at the destination. The
    -- original descriptor remains exact in the seal, as do namespace bindings.
    INSERT INTO weave_ontologies(id,prefix,namespace,name,description,family_id,ontology_type,extends_ontology_id,homepage_url,source_url,created_by_id)
    SELECT o.id,o.prefix,o.namespace,o.name,o.description,
        CASE WHEN EXISTS(SELECT 1 FROM weave_ontology_families f WHERE f.id=o.family_id) THEN o.family_id END,
        o.ontology_type,CASE WHEN EXISTS(SELECT 1 FROM weave_ontologies base WHERE base.id=o.extends_ontology_id) THEN o.extends_ontology_id END,o.homepage_url,o.source_url,o.created_by_id
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
    INSERT INTO weave_ontology_body_seals(ontology_id,version_id,body_text) VALUES(owner_id,release_id,payload);
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
    PERFORM weave_restore_ontology_body(owner_id,release_id,doc->>'body_json',doc->>'body_digest');
    INSERT INTO weave_ontology_releases(ontology_id,version_id,artifact,digest) VALUES(owner_id,release_id,payload,expected_digest);
    PERFORM weave_set_ontology_active_version(owner_id,release_id,true);
END $$;
-- +goose StatementEnd

-- Imported evidence is not a local release or a storage-derived capture. No
-- release/project FK or completion marker: partial project hydration cannot
-- establish those contracts, and retargeting must not rewrite the origin.
CREATE TABLE weave_imported_legacy_ontology_snapshots (
    project_id text NOT NULL CHECK (btrim(project_id)<>''),
    version_number text NOT NULL CHECK (btrim(version_number)<>''),
    document text NOT NULL,
    digest text NOT NULL CHECK (digest ~ '^[0-9a-f]{64}$'),
    PRIMARY KEY (project_id,version_number)
);

-- +goose StatementBegin
CREATE FUNCTION weave_validate_imported_legacy_ontology_snapshot() RETURNS trigger LANGUAGE plpgsql
SET search_path=public,pg_temp AS $$
DECLARE doc jsonb := NEW.document::jsonb; source jsonb; context_doc jsonb; item jsonb;
BEGIN
    IF encode(sha256(convert_to(NEW.document,'UTF8')),'hex') IS DISTINCT FROM NEW.digest
       OR (doc->>'format_version') IS DISTINCT FROM '1'
       OR (doc->>'project_id') IS DISTINCT FROM NEW.project_id
       OR (doc->>'version') IS DISTINCT FROM NEW.version_number THEN
        RAISE EXCEPTION 'imported snapshot identity or digest mismatch' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
    END IF;
    -- Artifact.Decode owns semantic coverage and source-graph validation. Storage
    -- independently checks source hashes, scoped selections and exact retained
    -- bodies, without deriving or blessing a local project release.
    FOR source IN SELECT value FROM jsonb_array_elements(COALESCE(NULLIF(doc->'sources','null'),'[]')) LOOP
        context_doc := (source->>'json')::jsonb;
        IF (context_doc->>'format_version') IS DISTINCT FROM '1'
           OR COALESCE(btrim(context_doc->>'project_id'),'')=''
           OR COALESCE(btrim(context_doc->>'version'),'')=''
           OR source->>'json' IS NULL OR source->>'digest' IS NULL
           OR encode(sha256(convert_to(source->>'json','UTF8')),'hex') IS DISTINCT FROM source->>'digest'
           OR jsonb_array_length(COALESCE(NULLIF(context_doc->'sources','null'),'[]'))<>0 THEN
            RAISE EXCEPTION 'imported snapshot source identity or digest mismatch' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
        END IF;
    END LOOP;
    FOR context_doc IN SELECT doc UNION ALL
        SELECT (value->>'json')::jsonb FROM jsonb_array_elements(COALESCE(NULLIF(doc->'sources','null'),'[]'))
    LOOP
        FOR item IN SELECT value FROM jsonb_array_elements(COALESCE(NULLIF(context_doc->'inventory','null'),'[]')) LOOP
            IF (item#>>'{selection,project_id}') IS DISTINCT FROM context_doc->>'project_id'
               OR (item#>>'{selection,version_number}') IS DISTINCT FROM context_doc->>'version'
               OR (item#>>'{selection,ontology_version_id}') IS DISTINCT FROM item->>'version_id'
               OR NOT EXISTS (SELECT 1 FROM weave_ontology_body_seals s WHERE s.ontology_id=item->>'ontology_id'
                   AND s.version_id=item->>'version_id' AND s.digest=item->>'digest'
                   AND COALESCE(s.body_text,s.body::text)=item->>'body_json') THEN
                RAISE EXCEPTION 'imported snapshot body or selection differs from retained evidence' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
            END IF;
        END LOOP;
    END LOOP;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER validate_imported_legacy_ontology_snapshot BEFORE INSERT ON weave_imported_legacy_ontology_snapshots
FOR EACH ROW EXECUTE FUNCTION weave_validate_imported_legacy_ontology_snapshot();

-- +goose StatementBegin
CREATE FUNCTION weave_guard_imported_legacy_ontology_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP<>'TRUNCATE' OR EXISTS(SELECT 1 FROM weave_imported_legacy_ontology_snapshots) THEN
        RAISE EXCEPTION 'imported legacy ontology evidence is retained and immutable' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
    END IF;
    RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER guard_imported_legacy_ontology_snapshot BEFORE UPDATE OR DELETE ON weave_imported_legacy_ontology_snapshots
FOR EACH ROW EXECUTE FUNCTION weave_guard_imported_legacy_ontology_snapshot();
CREATE TRIGGER guard_imported_legacy_ontology_snapshot_truncate BEFORE TRUNCATE ON weave_imported_legacy_ontology_snapshots
FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_imported_legacy_ontology_snapshot();

-- +goose StatementBegin
CREATE FUNCTION weave_retain_imported_legacy_ontology_snapshot(payload text, expected_digest text) RETURNS void LANGUAGE plpgsql
SET search_path=public,pg_temp AS $$
DECLARE doc jsonb := payload::jsonb; existing weave_imported_legacy_ontology_snapshots%ROWTYPE;
BEGIN
    IF current_setting('transaction_isolation') NOT IN ('repeatable read','serializable') THEN
        RAISE EXCEPTION 'legacy ontology restore requires repeatable read' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended('imported-legacy-ontology:' || jsonb_build_array(doc->>'project_id',doc->>'version')::text,0));
    SELECT * INTO existing FROM weave_imported_legacy_ontology_snapshots WHERE project_id=doc->>'project_id' AND version_number=doc->>'version';
    IF FOUND THEN
        IF existing.document=payload AND existing.digest=expected_digest THEN RETURN; END IF;
        RAISE EXCEPTION 'existing imported snapshot has different content' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
    END IF;
    INSERT INTO weave_imported_legacy_ontology_snapshots(project_id,version_number,document,digest)
    VALUES(doc->>'project_id',doc->>'version',payload,expected_digest);
END $$;
-- +goose StatementEnd

-- Captures must hash the original transported body text, not its JSONB rendering.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_legacy_context_v1(project text, release_version text) RETURNS jsonb LANGUAGE plpgsql STABLE
SET search_path=public,pg_temp SET timezone='UTC' SET datestyle='ISO, YMD' AS $$
DECLARE inventory jsonb; bindings jsonb; elements jsonb; assessments jsonb; source_refs jsonb; dependencies jsonb;
BEGIN
    IF EXISTS(SELECT 1 FROM weave_project_ontology_versions_archive l LEFT JOIN weave_ontology_body_seals b ON b.version_id=l.ontology_version_id
        WHERE l.project_id=project AND l.version_number=release_version AND b.version_id IS NULL) THEN
        RAISE EXCEPTION 'legacy context lacks a selected sealed body' USING ERRCODE='23514',CONSTRAINT='legacy_context_incomplete';
    END IF;
    IF EXISTS(SELECT 1 FROM weave_release_project_dependencies d LEFT JOIN weave_legacy_ontology_snapshots s ON s.project_id=d.source_project_id AND s.version_number=d.source_version
        WHERE d.project_id=project AND d.version_number=release_version AND s.project_id IS NULL) THEN
        RAISE EXCEPTION 'legacy context lacks a captured source' USING ERRCODE='23514',CONSTRAINT='legacy_context_incomplete';
    END IF;
    SELECT COALESCE(jsonb_agg(jsonb_build_object('ontology_id',s.ontology_id,'version_id',s.version_id,
        'digest',s.digest,'body_json',COALESCE(s.body_text,s.body::text),'selection',to_jsonb(l)) ORDER BY s.version_id COLLATE "C"),'[]')
        INTO inventory FROM weave_project_ontology_versions_archive l
        JOIN weave_ontology_body_seals s ON s.version_id=l.ontology_version_id
        WHERE l.project_id=project AND l.version_number=release_version;
    SELECT COALESCE(jsonb_agg(to_jsonb(b) ORDER BY b.id COLLATE "C"),'[]') INTO bindings
        FROM weave_release_namespace_bindings_archive b WHERE b.release_project_id=project AND b.version_number=release_version;
    -- Retain original type/datatype/URI/QName/version values, including elements
    -- that have no reference. Normalization below never rewrites these inputs.
    SELECT COALESCE(jsonb_agg(jsonb_build_object('entity_kind',entity_kind,'entity_id',entity_id,
        'location',location,'default_kind',default_kind,'original',element)
        ORDER BY entity_kind COLLATE "C",entity_id COLLATE "C",location COLLATE "C"),'[]') INTO elements
        FROM weave_project_semantic_elements WHERE project_id=project AND version_number=release_version;
    SELECT COALESCE(jsonb_agg(jsonb_build_object('entity_kind',entity_kind,'entity_id',entity_id,'location',location,
        'reference',reference) || weave_legacy_ontology_assessment(reference,inventory,bindings)
        ORDER BY entity_kind COLLATE "C",entity_id COLLATE "C",location COLLATE "C"),'[]') INTO assessments
        FROM weave_project_semantic_references WHERE project_id=project AND version_number=release_version;
    SELECT COALESCE(jsonb_agg(jsonb_build_object('project_id',s.project_id,'version',s.version_number,'digest',s.context_digest)
        ORDER BY s.project_id COLLATE "C",s.version_number COLLATE "C"),'[]') INTO source_refs
        FROM weave_release_project_dependencies d JOIN weave_legacy_ontology_snapshots s
          ON s.project_id=d.source_project_id AND s.version_number=d.source_version
        WHERE d.project_id=project AND d.version_number=release_version;
    SELECT COALESCE(jsonb_agg(to_jsonb(e) ORDER BY entity_type COLLATE "C",entity_id COLLATE "C"),'[]') INTO dependencies
        FROM weave_release_dependency_entities e WHERE project_id=project AND version_number=release_version;
    RETURN jsonb_build_object('format_version',1,'project_id',project,'version',release_version,
        'inventory',inventory,'bindings',bindings,'elements',elements,'assessments',assessments,'sources','[]'::jsonb,'source_refs',source_refs,'dependencies',dependencies);
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM weave_imported_legacy_ontology_snapshots) THEN
        RAISE EXCEPTION 'cannot discard imported legacy ontology evidence';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE weave_imported_legacy_ontology_snapshots;
DROP FUNCTION weave_retain_imported_legacy_ontology_snapshot(text,text),weave_validate_imported_legacy_ontology_snapshot(),weave_guard_imported_legacy_ontology_snapshot();
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
DROP FUNCTION weave_restore_ontology_body(text,text,text,text);
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_legacy_context_v1(project text, release_version text) RETURNS jsonb LANGUAGE plpgsql STABLE
SET search_path=public,pg_temp SET timezone='UTC' SET datestyle='ISO, YMD' AS $$
DECLARE inventory jsonb; bindings jsonb; elements jsonb; assessments jsonb; source_refs jsonb; dependencies jsonb;
BEGIN
    IF EXISTS(SELECT 1 FROM weave_project_ontology_versions_archive l LEFT JOIN weave_ontology_body_seals b ON b.version_id=l.ontology_version_id
        WHERE l.project_id=project AND l.version_number=release_version AND b.version_id IS NULL) THEN
        RAISE EXCEPTION 'legacy context lacks a selected sealed body' USING ERRCODE='23514',CONSTRAINT='legacy_context_incomplete';
    END IF;
    IF EXISTS(SELECT 1 FROM weave_release_project_dependencies d LEFT JOIN weave_legacy_ontology_snapshots s ON s.project_id=d.source_project_id AND s.version_number=d.source_version
        WHERE d.project_id=project AND d.version_number=release_version AND s.project_id IS NULL) THEN
        RAISE EXCEPTION 'legacy context lacks a captured source' USING ERRCODE='23514',CONSTRAINT='legacy_context_incomplete';
    END IF;
    SELECT COALESCE(jsonb_agg(jsonb_build_object('ontology_id',s.ontology_id,'version_id',s.version_id,
        'digest',s.digest,'body_json',s.body::text,'selection',to_jsonb(l)) ORDER BY s.version_id COLLATE "C"),'[]')
        INTO inventory FROM weave_project_ontology_versions_archive l
        JOIN weave_ontology_body_seals s ON s.version_id=l.ontology_version_id
        WHERE l.project_id=project AND l.version_number=release_version;
    SELECT COALESCE(jsonb_agg(to_jsonb(b) ORDER BY b.id COLLATE "C"),'[]') INTO bindings
        FROM weave_release_namespace_bindings_archive b WHERE b.release_project_id=project AND b.version_number=release_version;
    -- Retain original type/datatype/URI/QName/version values, including elements
    -- that have no reference. Normalization below never rewrites these inputs.
    SELECT COALESCE(jsonb_agg(jsonb_build_object('entity_kind',entity_kind,'entity_id',entity_id,
        'location',location,'default_kind',default_kind,'original',element)
        ORDER BY entity_kind COLLATE "C",entity_id COLLATE "C",location COLLATE "C"),'[]') INTO elements
        FROM weave_project_semantic_elements WHERE project_id=project AND version_number=release_version;
    SELECT COALESCE(jsonb_agg(jsonb_build_object('entity_kind',entity_kind,'entity_id',entity_id,'location',location,
        'reference',reference) || weave_legacy_ontology_assessment(reference,inventory,bindings)
        ORDER BY entity_kind COLLATE "C",entity_id COLLATE "C",location COLLATE "C"),'[]') INTO assessments
        FROM weave_project_semantic_references WHERE project_id=project AND version_number=release_version;
    SELECT COALESCE(jsonb_agg(jsonb_build_object('project_id',s.project_id,'version',s.version_number,'digest',s.context_digest)
        ORDER BY s.project_id COLLATE "C",s.version_number COLLATE "C"),'[]') INTO source_refs
        FROM weave_release_project_dependencies d JOIN weave_legacy_ontology_snapshots s
          ON s.project_id=d.source_project_id AND s.version_number=d.source_version
        WHERE d.project_id=project AND d.version_number=release_version;
    SELECT COALESCE(jsonb_agg(to_jsonb(e) ORDER BY entity_type COLLATE "C",entity_id COLLATE "C"),'[]') INTO dependencies
        FROM weave_release_dependency_entities e WHERE project_id=project AND version_number=release_version;
    RETURN jsonb_build_object('format_version',1,'project_id',project,'version',release_version,
        'inventory',inventory,'bindings',bindings,'elements',elements,'assessments',assessments,'sources','[]'::jsonb,'source_refs',source_refs,'dependencies',dependencies);
END $$;
-- +goose StatementEnd
