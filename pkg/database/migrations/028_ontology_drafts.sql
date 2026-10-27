-- +goose Up
ALTER TABLE weave_ontology_releases ADD CONSTRAINT weave_ontology_release_owner_version_unique UNIQUE(ontology_id,version_id);
CREATE TABLE weave_ontology_drafts (
    id text PRIMARY KEY,
    ontology_id text NOT NULL UNIQUE REFERENCES weave_ontologies(id) ON DELETE CASCADE,
    base_version_id text,
    revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
    document jsonb NOT NULL CHECK(jsonb_typeof(document)='object' AND (document->>'format_version') IS NOT DISTINCT FROM '1'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(ontology_id,id),
    FOREIGN KEY(ontology_id,base_version_id) REFERENCES weave_ontology_releases(ontology_id,version_id) ON DELETE RESTRICT
);
CREATE TABLE weave_ontology_draft_publications (
    ontology_id text NOT NULL,
    draft_id text NOT NULL,
    revision bigint NOT NULL,
    version_id text NOT NULL UNIQUE,
    digest text NOT NULL,
    request jsonb NOT NULL CHECK(jsonb_typeof(request)='object'),
    document jsonb NOT NULL DEFAULT '{}'::jsonb,
    published_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(draft_id,revision),
    FOREIGN KEY(ontology_id,draft_id) REFERENCES weave_ontology_drafts(ontology_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(ontology_id,version_id,digest) REFERENCES weave_ontology_releases(ontology_id,version_id,digest) ON DELETE RESTRICT
);
-- +goose StatementBegin
CREATE FUNCTION weave_guard_ontology_draft() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='INSERT' THEN
        IF NEW.revision<>1 THEN RAISE EXCEPTION 'new ontology draft requires revision one' USING ERRCODE='23514'; END IF;
        NEW.created_at := clock_timestamp();
    ELSE
        IF (NEW.id,NEW.ontology_id,NEW.base_version_id,NEW.revision,NEW.created_at) IS DISTINCT FROM (OLD.id,OLD.ontology_id,OLD.base_version_id,OLD.revision,OLD.created_at) THEN
            RAISE EXCEPTION 'ontology draft identity and revision are managed by storage' USING ERRCODE='23514';
        END IF;
        NEW.revision := OLD.revision+1;
    END IF;
    NEW.updated_at := clock_timestamp();
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER guard_ontology_draft BEFORE INSERT OR UPDATE ON weave_ontology_drafts FOR EACH ROW EXECUTE FUNCTION weave_guard_ontology_draft();
-- +goose StatementBegin
CREATE FUNCTION weave_guard_ontology_draft_publication() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE current_draft weave_ontology_drafts%ROWTYPE;
BEGIN
    IF TG_OP='TRUNCATE' AND NOT EXISTS(SELECT 1 FROM weave_ontology_draft_publications) THEN RETURN NULL; END IF;
    IF TG_OP<>'INSERT' THEN
        RAISE EXCEPTION 'ontology draft publication receipts are immutable' USING ERRCODE='23514';
    END IF;
    IF current_setting('transaction_isolation') NOT IN ('repeatable read','serializable') THEN
        RAISE EXCEPTION 'ontology draft publication requires repeatable read' USING ERRCODE='23514';
    END IF;
    SELECT * INTO STRICT current_draft FROM weave_ontology_drafts WHERE id=NEW.draft_id AND ontology_id=NEW.ontology_id FOR UPDATE;
    IF current_draft.revision<>NEW.revision THEN
        RAISE EXCEPTION 'ontology draft revision changed' USING ERRCODE='23514';
    END IF;
    NEW.document := current_draft.document;
    NEW.published_at := clock_timestamp();
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER guard_ontology_draft_publication BEFORE INSERT OR UPDATE OR DELETE ON weave_ontology_draft_publications FOR EACH ROW EXECUTE FUNCTION weave_guard_ontology_draft_publication();
CREATE TRIGGER guard_ontology_draft_publication_truncate BEFORE TRUNCATE ON weave_ontology_draft_publications FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_ontology_draft_publication();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM weave_ontology_drafts) THEN
        RAISE EXCEPTION 'cannot downgrade stored ontology drafts';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE weave_ontology_draft_publications;
DROP TABLE weave_ontology_drafts;
DROP FUNCTION weave_guard_ontology_draft_publication(),weave_guard_ontology_draft();
ALTER TABLE weave_ontology_releases DROP CONSTRAINT weave_ontology_release_owner_version_unique;
