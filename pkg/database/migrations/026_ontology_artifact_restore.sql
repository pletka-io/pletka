-- +goose Up
-- NULL retains the local JSONB rendering for preexisting/local seals. Portable
-- restores preserve exact supplied bytes after verifying their indexed content.
ALTER TABLE weave_ontology_body_seals ADD COLUMN body_text text;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_capture_ontology_body_seal() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    version_row weave_ontology_versions%ROWTYPE;
    supplied jsonb;
BEGIN
    UPDATE weave_ontology_versions SET body_write_revision = body_write_revision + 1
    WHERE id = NEW.version_id AND ontology_id = NEW.ontology_id AND body_sealed_at IS NULL
    RETURNING * INTO version_row;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'ontology version % is missing, belongs to another ontology or is already sealed', NEW.version_id
            USING ERRCODE = '23514', CONSTRAINT = 'ontology_body_sealed';
    END IF;
    IF version_row.class_count <> (SELECT count(*) FROM weave_ontology_classes WHERE ontology_version_id = NEW.version_id)
       OR version_row.property_count <> (SELECT count(*) FROM weave_ontology_properties WHERE ontology_version_id = NEW.version_id) THEN
        RAISE EXCEPTION 'ontology version body counts disagree with its terms' USING ERRCODE = '23514', CONSTRAINT = 'ontology_body_counts';
    END IF;
    -- The complete local body and its namespace interpretation context are
    -- captured by one statement. Mutable policy/audit fields are excluded.
    -- Imported/base ontology declarations are metadata, not resolved pins yet.
    SELECT jsonb_build_object(
        'format_version', 1,
        'ontology', to_jsonb(o) - ARRAY['created_at', 'updated_at'],
        'version', to_jsonb(version_row) - ARRAY['is_active', 'created_at', 'updated_at', 'parsed_at', 'body_sealed_at', 'body_write_revision'],
        'classes', COALESCE((SELECT jsonb_agg(to_jsonb(c) - ARRAY['created_at', 'updated_at'] ORDER BY c.id) FROM weave_ontology_classes c WHERE c.ontology_version_id = NEW.version_id), '[]'::jsonb),
        'properties', COALESCE((SELECT jsonb_agg(to_jsonb(p) - ARRAY['created_at', 'updated_at'] ORDER BY p.id) FROM weave_ontology_properties p WHERE p.ontology_version_id = NEW.version_id), '[]'::jsonb),
        'relations', COALESCE((SELECT jsonb_agg(to_jsonb(r) - ARRAY['source_class_id', 'source_property_id'] ORDER BY r.source_kind, r.source_id, r.rel_type, r.position, r.target_qname) FROM weave_ontology_relations r WHERE r.ontology_version_id = NEW.version_id), '[]'::jsonb),
        'companions', COALESCE((SELECT jsonb_agg(to_jsonb(c) ORDER BY c.position, c.filename) FROM weave_ontology_version_companions c WHERE c.ontology_version_id = NEW.version_id), '[]'::jsonb),
        'namespace_bindings', COALESCE((SELECT jsonb_agg(to_jsonb(b) - ARRAY['created_at', 'updated_at'] ORDER BY b.prefix, b.namespace, b.id) FROM weave_namespace_bindings b WHERE b.project_id IS NULL OR b.project_id = ''), '[]'::jsonb)
    ) INTO NEW.body FROM weave_ontologies o WHERE o.id = NEW.ontology_id;
    IF NEW.body_text IS NOT NULL THEN
        supplied := NEW.body_text::jsonb;
        IF (supplied->>'format_version') IS DISTINCT FROM '1'
           OR (supplied#>>'{ontology,id}') IS DISTINCT FROM NEW.ontology_id
           OR (supplied - ARRAY['ontology','namespace_bindings']) IS DISTINCT FROM (NEW.body - ARRAY['ontology','namespace_bindings'])
           OR jsonb_typeof(supplied->'namespace_bindings') IS DISTINCT FROM 'array' THEN
            RAISE EXCEPTION 'restored ontology body differs from its indexed content' USING ERRCODE='23514',CONSTRAINT='ontology_artifact_conflict';
        END IF;
        -- Descriptor and namespace context belong to the transported artifact,
        -- not the destination's current catalog or global namespace policy.
        NEW.body := supplied;
    END IF;
    NEW.format_version := 1;
    NEW.digest := encode(sha256(convert_to(COALESCE(NEW.body_text,NEW.body::text), 'UTF8')), 'hex');
    NEW.sealed_at := clock_timestamp();
    RETURN NEW;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_validate_ontology_release() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    doc jsonb := NEW.artifact::jsonb;
    sealed weave_ontology_body_seals%ROWTYPE;
    pin jsonb;
    source weave_ontology_releases%ROWTYPE;
    base_id text;
    defaults_id text;
BEGIN
    IF current_setting('transaction_isolation') NOT IN ('repeatable read', 'serializable') THEN
        RAISE EXCEPTION 'ontology publication requires a repeatable-read transaction' USING ERRCODE = '23514';
    END IF;
    SELECT * INTO STRICT sealed FROM weave_ontology_body_seals WHERE version_id = NEW.version_id;
    IF sealed.ontology_id <> NEW.ontology_id
       OR (doc->>'format_version') IS DISTINCT FROM '1'
       OR (doc->>'ontology_id') IS DISTINCT FROM NEW.ontology_id
       OR (doc->>'version_id') IS DISTINCT FROM NEW.version_id
       OR (doc->>'body_json') IS DISTINCT FROM COALESCE(sealed.body_text,sealed.body::text)
       OR (doc->>'body_digest') IS DISTINCT FROM sealed.digest
       OR jsonb_typeof(doc->'pins') IS DISTINCT FROM 'array'
       OR jsonb_typeof(doc->'requirements') IS DISTINCT FROM 'array' THEN
        RAISE EXCEPTION 'ontology publication does not match its sealed body' USING ERRCODE = '23514';
    END IF;
    -- Completeness of the requirement inventory is a storage invariant, not
    -- something each importing function can choose to omit. Exact resolution
    -- and compatibility policy live in the single publication boundary.
    base_id := sealed.body#>>'{ontology,extends_ontology_id}';
    IF base_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM jsonb_array_elements(doc->'requirements') r
        WHERE r->>'kind' = 'base' AND r->>'ontology_id' = base_id AND r->>'disposition' = 'pinned') THEN
        RAISE EXCEPTION 'ontology publication omits its base requirement' USING ERRCODE = '23514';
    END IF;
    -- A restored publication retains its original defaults decision. Never
    -- replace it with destination active/catalog policy.
	IF sealed.body_text IS NULL THEN
		SELECT id INTO defaults_id FROM weave_ontologies WHERE prefix = 'defaults';
		IF NOT EXISTS (SELECT 1 FROM jsonb_array_elements(doc->'requirements') r
			WHERE r->>'kind' = 'defaults' AND (
				(defaults_id IS NULL AND r->>'disposition' = 'absent') OR
				(defaults_id = NEW.ontology_id AND r->>'disposition' = 'self') OR
				(defaults_id <> NEW.ontology_id AND r->>'disposition' = 'pinned' AND r->>'ontology_id' = defaults_id))) THEN
			RAISE EXCEPTION 'ontology publication omits its defaults decision' USING ERRCODE = '23514';
		END IF;
	END IF;
    IF (SELECT count(*) FROM jsonb_array_elements(doc->'requirements') r WHERE r->>'kind'='defaults') <> 1
       OR NOT EXISTS (SELECT 1 FROM jsonb_array_elements(doc->'requirements') r WHERE r->>'kind'='defaults' AND (
           (r->>'disposition'='absent' AND r->>'ontology_id' IS NULL) OR
           (r->>'disposition'='self' AND sealed.body#>>'{ontology,prefix}'='defaults' AND r->>'ontology_id' IS NULL) OR
           (r->>'disposition'='pinned' AND EXISTS(SELECT 1 FROM weave_ontology_body_seals s
               JOIN jsonb_array_elements(doc->'pins') p ON p->>'version_id'=s.version_id
               WHERE s.ontology_id=r->>'ontology_id' AND s.body#>>'{ontology,prefix}'='defaults')))) THEN
        RAISE EXCEPTION 'ontology publication omits its defaults decision' USING ERRCODE='23514';
    END IF;
    IF EXISTS (
        SELECT 1 FROM jsonb_array_elements_text(COALESCE(NULLIF(sealed.body#>'{version,imported_ontologies}', 'null'::jsonb), '[]'::jsonb)) i
        WHERE NOT EXISTS (SELECT 1 FROM jsonb_array_elements(doc->'requirements') r
            WHERE r->>'kind' = 'import' AND r->>'name' = i AND r->>'disposition' IN ('pinned', 'external'))
    ) THEN
        RAISE EXCEPTION 'ontology publication omits an import requirement' USING ERRCODE = '23514';
    END IF;
    IF EXISTS (
        SELECT 1 FROM (
            SELECT r->>'target_qname' AS target FROM jsonb_array_elements(sealed.body->'relations') r
            UNION SELECT p->>'inverse_property_uri' FROM jsonb_array_elements(sealed.body->'properties') p
        ) refs WHERE target IS NOT NULL AND target <> '' AND NOT EXISTS (
            SELECT 1 FROM jsonb_array_elements(doc->'requirements') r
            WHERE r->>'kind' = 'reference' AND r->>'name' = refs.target AND r->>'disposition' IN ('local', 'pinned', 'external'))
    ) THEN
        RAISE EXCEPTION 'ontology publication omits a term reference' USING ERRCODE = '23514';
    END IF;
    IF EXISTS (SELECT 1 FROM jsonb_array_elements(doc->'requirements') r
               WHERE (r->>'disposition' = 'pinned') IS DISTINCT FROM (r->>'ontology_id' IS NOT NULL)) THEN
        RAISE EXCEPTION 'ontology requirement classification and pin disagree' USING ERRCODE = '23514';
    END IF;
    FOR pin IN SELECT value FROM jsonb_array_elements(doc->'pins') LOOP
        SELECT * INTO source FROM weave_ontology_releases
        WHERE ontology_id = pin->>'ontology_id' AND version_id = pin->>'version_id' AND digest = pin->>'digest';
        IF NOT FOUND OR source.ontology_id = NEW.ontology_id THEN
            RAISE EXCEPTION 'ontology dependency is missing, mismatched or cyclic' USING ERRCODE = '23514';
        END IF;
        IF NOT ((doc->'pins') @> (source.artifact::jsonb->'pins')) THEN
            RAISE EXCEPTION 'ontology publication omits or conflicts with source closure' USING ERRCODE = '23514';
        END IF;
    END LOOP;
    IF EXISTS (SELECT 1 FROM jsonb_array_elements(doc->'requirements') r
               WHERE r->>'ontology_id' IS NOT NULL AND NOT EXISTS (
                   SELECT 1 FROM jsonb_array_elements(doc->'pins') p WHERE p->>'ontology_id' = r->>'ontology_id')) THEN
        RAISE EXCEPTION 'ontology requirement has no retained pin' USING ERRCODE = '23514';
    END IF;
    NEW.digest := encode(sha256(convert_to(NEW.artifact, 'UTF8')), 'hex');
    NEW.published_at := clock_timestamp();
    RETURN NEW;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION weave_restore_ontology_artifact(payload text, expected_digest text) RETURNS void LANGUAGE plpgsql AS $$
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
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM weave_ontology_body_seals WHERE body_text IS NOT NULL) THEN
        RAISE EXCEPTION 'cannot downgrade restored portable ontology bodies';
    END IF;
END $$;
-- +goose StatementEnd
DROP FUNCTION weave_restore_ontology_artifact(text,text);
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_capture_ontology_body_seal() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    version_row weave_ontology_versions%ROWTYPE;
BEGIN
    UPDATE weave_ontology_versions SET body_write_revision = body_write_revision + 1
    WHERE id = NEW.version_id AND ontology_id = NEW.ontology_id AND body_sealed_at IS NULL
    RETURNING * INTO version_row;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'ontology version % is missing, belongs to another ontology or is already sealed', NEW.version_id
            USING ERRCODE = '23514', CONSTRAINT = 'ontology_body_sealed';
    END IF;
    IF version_row.class_count <> (SELECT count(*) FROM weave_ontology_classes WHERE ontology_version_id = NEW.version_id)
       OR version_row.property_count <> (SELECT count(*) FROM weave_ontology_properties WHERE ontology_version_id = NEW.version_id) THEN
        RAISE EXCEPTION 'ontology version body counts disagree with its terms' USING ERRCODE = '23514', CONSTRAINT = 'ontology_body_counts';
    END IF;
    -- The complete local body and its namespace interpretation context are
    -- captured by one statement. Mutable policy/audit fields are excluded.
    -- Imported/base ontology declarations are metadata, not resolved pins yet.
    SELECT jsonb_build_object(
        'format_version', 1,
        'ontology', to_jsonb(o) - ARRAY['created_at', 'updated_at'],
        'version', to_jsonb(version_row) - ARRAY['is_active', 'created_at', 'updated_at', 'parsed_at', 'body_sealed_at', 'body_write_revision'],
        'classes', COALESCE((SELECT jsonb_agg(to_jsonb(c) - ARRAY['created_at', 'updated_at'] ORDER BY c.id) FROM weave_ontology_classes c WHERE c.ontology_version_id = NEW.version_id), '[]'::jsonb),
        'properties', COALESCE((SELECT jsonb_agg(to_jsonb(p) - ARRAY['created_at', 'updated_at'] ORDER BY p.id) FROM weave_ontology_properties p WHERE p.ontology_version_id = NEW.version_id), '[]'::jsonb),
        'relations', COALESCE((SELECT jsonb_agg(to_jsonb(r) - ARRAY['source_class_id', 'source_property_id'] ORDER BY r.source_kind, r.source_id, r.rel_type, r.position, r.target_qname) FROM weave_ontology_relations r WHERE r.ontology_version_id = NEW.version_id), '[]'::jsonb),
        'companions', COALESCE((SELECT jsonb_agg(to_jsonb(c) ORDER BY c.position, c.filename) FROM weave_ontology_version_companions c WHERE c.ontology_version_id = NEW.version_id), '[]'::jsonb),
        'namespace_bindings', COALESCE((SELECT jsonb_agg(to_jsonb(b) - ARRAY['created_at', 'updated_at'] ORDER BY b.prefix, b.namespace, b.id) FROM weave_namespace_bindings b WHERE b.project_id IS NULL OR b.project_id = ''), '[]'::jsonb)
    ) INTO NEW.body FROM weave_ontologies o WHERE o.id = NEW.ontology_id;
    NEW.format_version := 1;
    NEW.digest := encode(sha256(convert_to(NEW.body::text, 'UTF8')), 'hex');
    NEW.sealed_at := clock_timestamp();
    RETURN NEW;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_validate_ontology_release() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    doc jsonb := NEW.artifact::jsonb;
    sealed weave_ontology_body_seals%ROWTYPE;
    pin jsonb;
    source weave_ontology_releases%ROWTYPE;
    base_id text;
    defaults_id text;
BEGIN
    IF current_setting('transaction_isolation') NOT IN ('repeatable read', 'serializable') THEN
        RAISE EXCEPTION 'ontology publication requires a repeatable-read transaction' USING ERRCODE = '23514';
    END IF;
    SELECT * INTO STRICT sealed FROM weave_ontology_body_seals WHERE version_id = NEW.version_id;
    IF sealed.ontology_id <> NEW.ontology_id
       OR (doc->>'format_version') IS DISTINCT FROM '1'
       OR (doc->>'ontology_id') IS DISTINCT FROM NEW.ontology_id
       OR (doc->>'version_id') IS DISTINCT FROM NEW.version_id
       OR (doc->>'body_json') IS DISTINCT FROM sealed.body::text
       OR (doc->>'body_digest') IS DISTINCT FROM sealed.digest
       OR jsonb_typeof(doc->'pins') IS DISTINCT FROM 'array'
       OR jsonb_typeof(doc->'requirements') IS DISTINCT FROM 'array' THEN
        RAISE EXCEPTION 'ontology publication does not match its sealed body' USING ERRCODE = '23514';
    END IF;
    -- Completeness of the requirement inventory is a storage invariant, not
    -- something each importing function can choose to omit. Exact resolution
    -- and compatibility policy live in the single publication boundary.
    base_id := sealed.body#>>'{ontology,extends_ontology_id}';
    IF base_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM jsonb_array_elements(doc->'requirements') r
        WHERE r->>'kind' = 'base' AND r->>'ontology_id' = base_id AND r->>'disposition' = 'pinned') THEN
        RAISE EXCEPTION 'ontology publication omits its base requirement' USING ERRCODE = '23514';
    END IF;
    SELECT id INTO defaults_id FROM weave_ontologies WHERE prefix = 'defaults';
    IF NOT EXISTS (SELECT 1 FROM jsonb_array_elements(doc->'requirements') r
        WHERE r->>'kind' = 'defaults' AND (
            (defaults_id IS NULL AND r->>'disposition' = 'absent') OR
            (defaults_id = NEW.ontology_id AND r->>'disposition' = 'self') OR
            (defaults_id <> NEW.ontology_id AND r->>'disposition' = 'pinned' AND r->>'ontology_id' = defaults_id))) THEN
        RAISE EXCEPTION 'ontology publication omits its defaults decision' USING ERRCODE = '23514';
    END IF;
    IF EXISTS (
        SELECT 1 FROM jsonb_array_elements_text(COALESCE(NULLIF(sealed.body#>'{version,imported_ontologies}', 'null'::jsonb), '[]'::jsonb)) i
        WHERE NOT EXISTS (SELECT 1 FROM jsonb_array_elements(doc->'requirements') r
            WHERE r->>'kind' = 'import' AND r->>'name' = i AND r->>'disposition' IN ('pinned', 'external'))
    ) THEN
        RAISE EXCEPTION 'ontology publication omits an import requirement' USING ERRCODE = '23514';
    END IF;
    IF EXISTS (
        SELECT 1 FROM (
            SELECT r->>'target_qname' AS target FROM jsonb_array_elements(sealed.body->'relations') r
            UNION SELECT p->>'inverse_property_uri' FROM jsonb_array_elements(sealed.body->'properties') p
        ) refs WHERE target IS NOT NULL AND target <> '' AND NOT EXISTS (
            SELECT 1 FROM jsonb_array_elements(doc->'requirements') r
            WHERE r->>'kind' = 'reference' AND r->>'name' = refs.target AND r->>'disposition' IN ('local', 'pinned', 'external'))
    ) THEN
        RAISE EXCEPTION 'ontology publication omits a term reference' USING ERRCODE = '23514';
    END IF;
    IF EXISTS (SELECT 1 FROM jsonb_array_elements(doc->'requirements') r
               WHERE (r->>'disposition' = 'pinned') IS DISTINCT FROM (r->>'ontology_id' IS NOT NULL)) THEN
        RAISE EXCEPTION 'ontology requirement classification and pin disagree' USING ERRCODE = '23514';
    END IF;
    FOR pin IN SELECT value FROM jsonb_array_elements(doc->'pins') LOOP
        SELECT * INTO source FROM weave_ontology_releases
        WHERE ontology_id = pin->>'ontology_id' AND version_id = pin->>'version_id' AND digest = pin->>'digest';
        IF NOT FOUND OR source.ontology_id = NEW.ontology_id THEN
            RAISE EXCEPTION 'ontology dependency is missing, mismatched or cyclic' USING ERRCODE = '23514';
        END IF;
        IF NOT ((doc->'pins') @> (source.artifact::jsonb->'pins')) THEN
            RAISE EXCEPTION 'ontology publication omits or conflicts with source closure' USING ERRCODE = '23514';
        END IF;
    END LOOP;
    IF EXISTS (SELECT 1 FROM jsonb_array_elements(doc->'requirements') r
               WHERE r->>'ontology_id' IS NOT NULL AND NOT EXISTS (
                   SELECT 1 FROM jsonb_array_elements(doc->'pins') p WHERE p->>'ontology_id' = r->>'ontology_id')) THEN
        RAISE EXCEPTION 'ontology requirement has no retained pin' USING ERRCODE = '23514';
    END IF;
    NEW.digest := encode(sha256(convert_to(NEW.artifact, 'UTF8')), 'hex');
    NEW.published_at := clock_timestamp();
    RETURN NEW;
END $$;
-- +goose StatementEnd
ALTER TABLE weave_ontology_body_seals DROP COLUMN body_text;
