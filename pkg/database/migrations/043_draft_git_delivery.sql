-- +goose Up
-- Delivery is separate from capture: the baseline and every prepared operation
-- survive process failure. No project is enrolled by this migration.
CREATE TABLE weave_draft_baselines (
 project_id text PRIMARY KEY REFERENCES weave_draft_projects(project_id),
 snapshot_digest text NOT NULL REFERENCES weave_draft_snapshots(digest),
 created_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO weave_draft_baselines(project_id,snapshot_digest)
SELECT p.project_id,CASE WHEN p.revision=0 THEN p.snapshot_digest ELSE
 (SELECT before_digest FROM weave_draft_mutations m WHERE m.project_id=p.project_id AND m.revision=1) END
FROM weave_draft_projects p;
CREATE TABLE weave_draft_audits (
 change_set_id bigint PRIMARY KEY REFERENCES weave_change_set(id),
 project_id text NOT NULL REFERENCES weave_draft_projects(project_id),
 revision bigint NOT NULL CHECK(revision>=0),
 coverage text NOT NULL CHECK(coverage IN ('baseline','mutation','legacy_coverage'))
);
-- Existing pre-delivery receipts cannot safely claim individual legacy audit
-- rows. Acknowledge them only once ALL already-captured revisions are delivered.
INSERT INTO weave_draft_audits
SELECT c.id,c.project_id,p.revision,'legacy_coverage' FROM weave_change_set c JOIN weave_draft_projects p ON p.project_id=c.project_id
WHERE c.kind='draft' AND c.processed_at IS NULL AND c.closed_at IS NOT NULL;
CREATE TABLE weave_draft_deliveries (
 project_id text NOT NULL REFERENCES weave_draft_projects(project_id),
 revision bigint NOT NULL CHECK(revision>=0),
 snapshot_digest text NOT NULL REFERENCES weave_draft_snapshots(digest),
 parent_commit text NOT NULL CHECK(parent_commit='' OR parent_commit ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
 commit_sha text NOT NULL CHECK(commit_sha ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
 tree_sha text NOT NULL CHECK(tree_sha ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
 prepared_at timestamptz NOT NULL DEFAULT now(),
 acknowledged_at timestamptz,
 PRIMARY KEY(project_id,revision)
);
CREATE UNIQUE INDEX weave_draft_one_pending_delivery ON weave_draft_deliveries(project_id) WHERE acknowledged_at IS NULL;
-- +goose StatementBegin
CREATE FUNCTION weave_start_draft_baseline() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM weave_change_set WHERE project_id=NEW.project_id AND kind='draft' AND closed_at IS NULL) THEN
  RAISE EXCEPTION 'close pending draft change sets before enrollment' USING ERRCODE='23514';
 END IF;
 INSERT INTO weave_draft_baselines(project_id,snapshot_digest) VALUES(NEW.project_id,NEW.snapshot_digest);
 INSERT INTO weave_draft_audits SELECT id,NEW.project_id,0,'baseline' FROM weave_change_set
 WHERE project_id=NEW.project_id AND kind='draft' AND closed_at IS NOT NULL AND processed_at IS NULL;
 RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER draft_delivery_baseline AFTER INSERT ON weave_draft_projects FOR EACH ROW EXECUTE FUNCTION weave_start_draft_baseline();
-- +goose StatementBegin
CREATE FUNCTION weave_link_draft_audit() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE captured_revision bigint;
BEGIN
 IF TG_OP='UPDATE' AND EXISTS(SELECT 1 FROM weave_draft_audits WHERE change_set_id=OLD.id) AND
  (to_jsonb(OLD)-ARRAY['processed_at','git_commit_sha','materialized_outcome','materialized_duration_ms','materialized_files','materialized_changed','materialized_error']) IS DISTINCT FROM
  (to_jsonb(NEW)-ARRAY['processed_at','git_commit_sha','materialized_outcome','materialized_duration_ms','materialized_files','materialized_changed','materialized_error']) THEN
  RAISE EXCEPTION 'captured audit identity is immutable' USING ERRCODE='23514';
 END IF;
 IF NEW.kind<>'draft' OR NEW.closed_at IS NULL THEN RETURN NEW; END IF;
 IF TG_OP='UPDATE' AND OLD.closed_at IS NOT NULL THEN RETURN NEW; END IF;
 -- Enrollment races cannot leave a late audit outside the explicit baseline.
 PERFORM weave_draft_fence('p:'||NEW.project_id);
 IF NOT EXISTS(SELECT 1 FROM weave_draft_projects WHERE project_id=NEW.project_id) THEN RETURN NEW; END IF;
 SELECT revision INTO captured_revision FROM weave_draft_mutations
 WHERE project_id=NEW.project_id AND transaction_id=pg_current_xact_id() AND after_digest IS NULL;
 IF captured_revision IS NULL THEN RAISE EXCEPTION 'enrolled draft audit requires its capture transaction' USING ERRCODE='23514'; END IF;
 INSERT INTO weave_draft_audits VALUES(NEW.id,NEW.project_id,captured_revision,'mutation');
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER draft_audit_revision AFTER INSERT OR UPDATE ON weave_change_set FOR EACH ROW EXECUTE FUNCTION weave_link_draft_audit();
-- +goose StatementBegin
CREATE FUNCTION weave_guard_draft_delivery() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE prior weave_draft_deliveries; expected_digest text;
BEGIN
 IF TG_OP='INSERT' THEN
  -- Serialize preparation even when different materializer hosts have different
  -- filesystem locks. Only one next revision may be prepared.
  PERFORM 1 FROM weave_draft_projects WHERE project_id=NEW.project_id FOR UPDATE;
  SELECT * INTO prior FROM weave_draft_deliveries WHERE project_id=NEW.project_id ORDER BY revision DESC LIMIT 1;
  IF FOUND AND (prior.acknowledged_at IS NULL OR NEW.revision<>prior.revision+1 OR NEW.parent_commit<>prior.commit_sha) THEN
   RAISE EXCEPTION 'draft delivery is out of order' USING ERRCODE='23514';
  ELSIF NOT FOUND AND NEW.revision<>0 THEN RAISE EXCEPTION 'draft delivery requires baseline zero' USING ERRCODE='23514'; END IF;
  IF NEW.revision=0 THEN SELECT snapshot_digest INTO expected_digest FROM weave_draft_baselines WHERE project_id=NEW.project_id;
  ELSE SELECT after_digest INTO expected_digest FROM weave_draft_mutations WHERE project_id=NEW.project_id AND revision=NEW.revision; END IF;
  IF expected_digest IS NULL OR NEW.snapshot_digest<>expected_digest OR NEW.acknowledged_at IS NOT NULL THEN
   RAISE EXCEPTION 'draft delivery snapshot is not captured' USING ERRCODE='23514';
  END IF;
 ELSIF TG_OP='UPDATE' THEN
  IF OLD.acknowledged_at IS NOT NULL OR NEW.acknowledged_at IS NULL OR
   (to_jsonb(OLD)-'acknowledged_at') IS DISTINCT FROM (to_jsonb(NEW)-'acknowledged_at') THEN
   RAISE EXCEPTION 'prepared draft delivery is immutable' USING ERRCODE='23514';
  END IF;
 ELSE RAISE EXCEPTION 'draft delivery history cannot be removed' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER draft_delivery_guard BEFORE INSERT OR UPDATE OR DELETE ON weave_draft_deliveries FOR EACH ROW EXECUTE FUNCTION weave_guard_draft_delivery();
CREATE TRIGGER draft_delivery_truncate BEFORE TRUNCATE ON weave_draft_deliveries FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_draft_immutable();
CREATE TRIGGER draft_baseline_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON weave_draft_baselines FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_draft_immutable();
CREATE TRIGGER draft_audits_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON weave_draft_audits FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_draft_immutable();
-- +goose StatementBegin
CREATE FUNCTION weave_guard_draft_audit_link() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF pg_trigger_depth()<2 OR NEW.coverage='legacy_coverage' OR NOT EXISTS(
  SELECT 1 FROM weave_change_set c WHERE c.id=NEW.change_set_id AND c.project_id=NEW.project_id AND c.kind='draft' AND c.closed_at IS NOT NULL
 ) THEN RAISE EXCEPTION 'draft audit coverage is owned by capture' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER draft_audit_link_guard BEFORE INSERT ON weave_draft_audits FOR EACH ROW EXECUTE FUNCTION weave_guard_draft_audit_link();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM weave_draft_baselines) THEN RAISE EXCEPTION 'cannot downgrade retained draft delivery history' USING ERRCODE='23514'; END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER draft_audit_revision ON weave_change_set;
DROP TRIGGER draft_delivery_baseline ON weave_draft_projects;
DROP TABLE weave_draft_deliveries,weave_draft_audits,weave_draft_baselines;
DROP FUNCTION weave_guard_draft_audit_link(),weave_guard_draft_delivery(),weave_link_draft_audit(),weave_start_draft_baseline();
