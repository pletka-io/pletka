-- +goose Up
-- A body seal is deliberately narrower than a published dependency closure.
-- Existing versions stay unsealed. No historical artifact is certified here.
ALTER TABLE weave_ontology_versions ADD COLUMN body_sealed_at timestamptz;
ALTER TABLE weave_ontology_versions ADD COLUMN body_write_revision bigint NOT NULL DEFAULT 0;

-- Make polymorphic relation ownership enforceable without discarding orphaned
-- historical evidence. NOT VALID constraints still apply to every new write.
ALTER TABLE weave_ontology_classes ADD CONSTRAINT weave_ontology_classes_version_id_unique UNIQUE (ontology_version_id, id);
ALTER TABLE weave_ontology_properties ADD CONSTRAINT weave_ontology_properties_version_id_unique UNIQUE (ontology_version_id, id);
ALTER TABLE weave_ontology_relations ADD COLUMN ontology_version_id text;
UPDATE weave_ontology_relations r SET ontology_version_id = c.ontology_version_id
FROM weave_ontology_classes c WHERE r.source_kind = 'class' AND r.source_id = c.id;
UPDATE weave_ontology_relations r SET ontology_version_id = p.ontology_version_id
FROM weave_ontology_properties p WHERE r.source_kind = 'property' AND r.source_id = p.id;
ALTER TABLE weave_ontology_relations ADD COLUMN source_class_id text GENERATED ALWAYS AS (CASE WHEN source_kind = 'class' THEN source_id END) STORED;
ALTER TABLE weave_ontology_relations ADD COLUMN source_property_id text GENERATED ALWAYS AS (CASE WHEN source_kind = 'property' THEN source_id END) STORED;
ALTER TABLE weave_ontology_relations ADD CONSTRAINT ontology_relation_owned CHECK (ontology_version_id IS NOT NULL) NOT VALID;
ALTER TABLE weave_ontology_relations ADD CONSTRAINT ontology_relation_class_fk FOREIGN KEY (ontology_version_id, source_class_id)
    REFERENCES weave_ontology_classes (ontology_version_id, id) ON DELETE CASCADE NOT VALID;
ALTER TABLE weave_ontology_relations ADD CONSTRAINT ontology_relation_property_fk FOREIGN KEY (ontology_version_id, source_property_id)
    REFERENCES weave_ontology_properties (ontology_version_id, id) ON DELETE CASCADE NOT VALID;
CREATE INDEX ontology_relations_version_idx ON weave_ontology_relations (ontology_version_id);

CREATE TABLE weave_ontology_body_seals (
    version_id text PRIMARY KEY REFERENCES weave_ontology_versions(id) ON DELETE RESTRICT,
    ontology_id text NOT NULL REFERENCES weave_ontologies(id) ON DELETE RESTRICT,
    format_version smallint NOT NULL CHECK (format_version = 1),
    body jsonb NOT NULL,
    digest text NOT NULL CHECK (digest ~ '^[0-9a-f]{64}$'),
    sealed_at timestamptz NOT NULL
);

-- +goose StatementBegin
CREATE FUNCTION weave_guard_ontology_version_body() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER guard_ontology_version_body BEFORE INSERT OR UPDATE OR DELETE ON weave_ontology_versions
FOR EACH ROW EXECUTE FUNCTION weave_guard_ontology_version_body();

-- +goose StatementBegin
CREATE FUNCTION weave_guard_ontology_body_child() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    old_version text;
    new_version text;
    owner_version text;
BEGIN
    IF TG_OP <> 'INSERT' THEN old_version := OLD.ontology_version_id; END IF;
    IF TG_OP <> 'DELETE' THEN
        IF TG_TABLE_NAME IN ('weave_ontology_classes', 'weave_ontology_properties') THEN
            -- Reusing a deterministic term ID must not reconnect orphaned
            -- history to a new body through legacy source-ID readers.
            IF EXISTS (SELECT 1 FROM weave_ontology_relations r
                WHERE r.ontology_version_id IS NULL AND r.source_id = NEW.id
                  AND r.source_kind = CASE TG_TABLE_NAME WHEN 'weave_ontology_classes' THEN 'class' ELSE 'property' END) THEN
                RAISE EXCEPTION 'ontology term % has unowned historical relations; review the evidence before reuse', NEW.id
                    USING ERRCODE = '23514', CONSTRAINT = 'ontology_relation_unowned';
            END IF;
        END IF;
        IF TG_TABLE_NAME = 'weave_ontology_relations' THEN
            IF NEW.source_kind = 'class' THEN
                SELECT ontology_version_id INTO owner_version FROM weave_ontology_classes WHERE id = NEW.source_id;
            ELSE
                SELECT ontology_version_id INTO owner_version FROM weave_ontology_properties WHERE id = NEW.source_id;
            END IF;
            IF owner_version IS NULL OR (NEW.ontology_version_id IS NOT NULL AND NEW.ontology_version_id <> owner_version) THEN
                RAISE EXCEPTION 'ontology relation must belong to its source version'
                    USING ERRCODE = '23514', CONSTRAINT = 'ontology_relation_owned';
            END IF;
            NEW.ontology_version_id := owner_version;
        END IF;
        new_version := NEW.ontology_version_id;
    END IF;
    -- Updating the parent serializes all body writers with sealing. A stale
    -- repeatable-read writer fails serialization, rather than missing the seal.
    FOR owner_version IN SELECT DISTINCT v FROM unnest(ARRAY[old_version, new_version]) v WHERE v IS NOT NULL ORDER BY v LOOP
        UPDATE weave_ontology_versions SET body_write_revision = body_write_revision + 1
        WHERE id = owner_version AND body_sealed_at IS NULL;
        IF NOT FOUND AND EXISTS (SELECT 1 FROM weave_ontology_versions WHERE id = owner_version) THEN
            RAISE EXCEPTION 'ontology version body % is sealed; create a new version', owner_version
                USING ERRCODE = '23514', CONSTRAINT = 'ontology_body_sealed';
        END IF;
        -- Missing parents during DELETE are the ordinary unsealed FK cascade.
        IF NOT FOUND AND TG_OP <> 'DELETE' AND NOT EXISTS (SELECT 1 FROM weave_ontology_versions WHERE id = owner_version) THEN
            RAISE EXCEPTION 'ontology version % does not exist', owner_version USING ERRCODE = '23503';
        END IF;
    END LOOP;
    IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER guard_ontology_class_body BEFORE INSERT OR UPDATE OR DELETE ON weave_ontology_classes FOR EACH ROW EXECUTE FUNCTION weave_guard_ontology_body_child();
CREATE TRIGGER guard_ontology_property_body BEFORE INSERT OR UPDATE OR DELETE ON weave_ontology_properties FOR EACH ROW EXECUTE FUNCTION weave_guard_ontology_body_child();
CREATE TRIGGER guard_ontology_relation_body BEFORE INSERT OR UPDATE OR DELETE ON weave_ontology_relations FOR EACH ROW EXECUTE FUNCTION weave_guard_ontology_body_child();
CREATE TRIGGER guard_ontology_companion_body BEFORE INSERT OR UPDATE OR DELETE ON weave_ontology_version_companions FOR EACH ROW EXECUTE FUNCTION weave_guard_ontology_body_child();

-- +goose StatementBegin
CREATE FUNCTION weave_capture_ontology_body_seal() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER capture_ontology_body_seal BEFORE INSERT ON weave_ontology_body_seals FOR EACH ROW EXECUTE FUNCTION weave_capture_ontology_body_seal();

-- +goose StatementBegin
CREATE FUNCTION weave_finish_ontology_body_seal() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    UPDATE weave_ontology_versions SET body_sealed_at = NEW.sealed_at WHERE id = NEW.version_id;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER finish_ontology_body_seal AFTER INSERT ON weave_ontology_body_seals FOR EACH ROW EXECUTE FUNCTION weave_finish_ontology_body_seal();

-- +goose StatementBegin
CREATE FUNCTION weave_reject_ontology_seal_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'sealed ontology bodies cannot be changed or removed'
        USING ERRCODE = '23514', CONSTRAINT = 'ontology_body_sealed';
END $$;
-- +goose StatementEnd
CREATE TRIGGER guard_ontology_body_seals BEFORE UPDATE OR DELETE ON weave_ontology_body_seals FOR EACH ROW EXECUTE FUNCTION weave_reject_ontology_seal_change();

-- +goose StatementBegin
CREATE FUNCTION weave_guard_ontology_body_truncate() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM weave_ontology_body_seals) THEN
        RAISE EXCEPTION 'cannot truncate sealed ontology bodies'
            USING ERRCODE = '23514', CONSTRAINT = 'ontology_body_sealed';
    END IF;
    RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER guard_ontology_version_truncate BEFORE TRUNCATE ON weave_ontology_versions FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_ontology_body_truncate();
CREATE TRIGGER guard_ontology_class_truncate BEFORE TRUNCATE ON weave_ontology_classes FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_ontology_body_truncate();
CREATE TRIGGER guard_ontology_property_truncate BEFORE TRUNCATE ON weave_ontology_properties FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_ontology_body_truncate();
CREATE TRIGGER guard_ontology_relation_truncate BEFORE TRUNCATE ON weave_ontology_relations FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_ontology_body_truncate();
CREATE TRIGGER guard_ontology_companion_truncate BEFORE TRUNCATE ON weave_ontology_version_companions FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_ontology_body_truncate();
CREATE TRIGGER guard_ontology_seal_truncate BEFORE TRUNCATE ON weave_ontology_body_seals FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_ontology_body_truncate();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM weave_ontology_body_seals) THEN
        RAISE EXCEPTION 'cannot downgrade sealed ontology bodies';
    END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER guard_ontology_version_body ON weave_ontology_versions;
DROP TRIGGER guard_ontology_class_body ON weave_ontology_classes;
DROP TRIGGER guard_ontology_property_body ON weave_ontology_properties;
DROP TRIGGER guard_ontology_relation_body ON weave_ontology_relations;
DROP TRIGGER guard_ontology_companion_body ON weave_ontology_version_companions;
DROP TRIGGER guard_ontology_version_truncate ON weave_ontology_versions;
DROP TRIGGER guard_ontology_class_truncate ON weave_ontology_classes;
DROP TRIGGER guard_ontology_property_truncate ON weave_ontology_properties;
DROP TRIGGER guard_ontology_relation_truncate ON weave_ontology_relations;
DROP TRIGGER guard_ontology_companion_truncate ON weave_ontology_version_companions;
DROP TABLE weave_ontology_body_seals;
DROP FUNCTION weave_guard_ontology_version_body(), weave_guard_ontology_body_child(), weave_capture_ontology_body_seal(), weave_finish_ontology_body_seal(), weave_reject_ontology_seal_change(), weave_guard_ontology_body_truncate();
ALTER TABLE weave_ontology_relations DROP CONSTRAINT ontology_relation_owned, DROP CONSTRAINT ontology_relation_class_fk, DROP CONSTRAINT ontology_relation_property_fk;
ALTER TABLE weave_ontology_relations DROP COLUMN source_class_id, DROP COLUMN source_property_id, DROP COLUMN ontology_version_id;
ALTER TABLE weave_ontology_classes DROP CONSTRAINT weave_ontology_classes_version_id_unique;
ALTER TABLE weave_ontology_properties DROP CONSTRAINT weave_ontology_properties_version_id_unique;
ALTER TABLE weave_ontology_versions DROP COLUMN body_sealed_at, DROP COLUMN body_write_revision;
