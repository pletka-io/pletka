-- +goose Up
-- Existing bodies remain unverified. Publication is a distinct, irreversible
-- operation; it retains both its local body and every artifact in its closure.
CREATE TABLE weave_ontology_releases (
    version_id text PRIMARY KEY REFERENCES weave_ontology_body_seals(version_id) ON DELETE RESTRICT,
    ontology_id text NOT NULL REFERENCES weave_ontologies(id) ON DELETE RESTRICT,
    artifact text NOT NULL,
    digest text NOT NULL CHECK (digest ~ '^[0-9a-f]{64}$'),
    published_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (ontology_id, version_id, digest)
);
CREATE TABLE weave_ontology_release_dependencies (
    consumer_version_id text NOT NULL REFERENCES weave_ontology_releases(version_id) ON DELETE RESTRICT,
    ontology_id text NOT NULL,
    version_id text NOT NULL,
    digest text NOT NULL,
    PRIMARY KEY (consumer_version_id, ontology_id),
    FOREIGN KEY (ontology_id, version_id, digest)
        REFERENCES weave_ontology_releases(ontology_id, version_id, digest) ON DELETE RESTRICT
);
CREATE INDEX weave_ontology_release_dependencies_source ON weave_ontology_release_dependencies(version_id);

-- +goose StatementBegin
CREATE FUNCTION weave_validate_ontology_release() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER validate_ontology_release BEFORE INSERT ON weave_ontology_releases
    FOR EACH ROW EXECUTE FUNCTION weave_validate_ontology_release();

-- +goose StatementBegin
CREATE FUNCTION weave_capture_ontology_release_dependencies() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO weave_ontology_release_dependencies(consumer_version_id, ontology_id, version_id, digest)
    SELECT NEW.version_id, p->>'ontology_id', p->>'version_id', p->>'digest'
    FROM jsonb_array_elements(NEW.artifact::jsonb->'pins') p;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER capture_ontology_release_dependencies AFTER INSERT ON weave_ontology_releases
    FOR EACH ROW EXECUTE FUNCTION weave_capture_ontology_release_dependencies();

-- +goose StatementBegin
CREATE FUNCTION weave_check_ontology_dependency_insert() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM weave_ontology_releases r WHERE r.version_id = NEW.consumer_version_id
        AND r.artifact::jsonb->'pins' @> jsonb_build_array(jsonb_build_object(
            'ontology_id', NEW.ontology_id, 'version_id', NEW.version_id, 'digest', NEW.digest))) THEN
        RAISE EXCEPTION 'dependency must match the immutable publication' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER check_ontology_dependency_insert BEFORE INSERT ON weave_ontology_release_dependencies
    FOR EACH ROW EXECUTE FUNCTION weave_check_ontology_dependency_insert();
CREATE TRIGGER guard_ontology_release BEFORE UPDATE OR DELETE ON weave_ontology_releases
    FOR EACH ROW EXECUTE FUNCTION weave_reject_ontology_seal_change();
CREATE TRIGGER guard_ontology_release_dependencies BEFORE UPDATE OR DELETE ON weave_ontology_release_dependencies
    FOR EACH ROW EXECUTE FUNCTION weave_reject_ontology_seal_change();
CREATE TRIGGER guard_ontology_release_truncate BEFORE TRUNCATE ON weave_ontology_releases
    FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_ontology_body_truncate();
CREATE TRIGGER guard_ontology_dependencies_truncate BEFORE TRUNCATE ON weave_ontology_release_dependencies
    FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_ontology_body_truncate();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM weave_ontology_releases) THEN
        RAISE EXCEPTION 'cannot downgrade published ontology artifacts';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE weave_ontology_release_dependencies, weave_ontology_releases;
DROP FUNCTION weave_validate_ontology_release(), weave_capture_ontology_release_dependencies(), weave_check_ontology_dependency_insert();
