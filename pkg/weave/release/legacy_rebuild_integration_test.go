//go:build integration

package release

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database"
	"github.com/pletka-io/pletka/pkg/database/ontologysnapshot"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/database/rebuildlease"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
)

const legacyProject = "legacy-root"
const legacyVersion = "0.1.0"

func seedLegacyRebuild(t *testing.T, withSource bool) (context.Context, *pgxpool.Pool, *Service) {
	t.Helper()
	p := testdb.EmptyPool(t)
	ctx := releaseAuthCtx(t.Context(), "legacy-owner")
	for _, sql := range []string{
		`INSERT INTO weave_actors(id,type,slug,display_name) VALUES ('legacy-owner','organization','legacy-owner','Owner')`,
		`INSERT INTO weave_projects(id,owner_id,ui_name) VALUES ('legacy-root','legacy-owner','{"en":"Current draft"}')`,
		`INSERT INTO weave_models(id,project_id) VALUES ('legacy-model','legacy-root')`,
		`INSERT INTO weave_fields(id,project_id,path_elements) VALUES ('legacy-field','legacy-root','[{"position":0,"type":"class","uri":"https://example.org/crm/E1","prefix":"crm","local_name":"E1"},{"position":1,"type":"class","uri":"https://example.org/crm/E1","prefix":"crm","local_name":"E1","ontology_version_id":"legacy-crm-6.2"},{"position":2,"type":"literal","uri":"rdfs:Literal","local_name":"Literal","prefix":"rdfs","datatype":"rdf:literal"}]')`,
		`INSERT INTO weave_releases(project_id,version,title,created_by_id) VALUES ('legacy-root','0.1.0','Old release','legacy-owner')`,
		`INSERT INTO weave_projects_archive(id,owner_id,version_number,ui_name) VALUES ('legacy-root','legacy-owner','0.1.0','{"en":"Old archive"}')`,
	} {
		mustExec(t, p, sql)
	}
	store := ontology.NewPostgresStore(p)
	if _, err := store.CreateOntology(ctx, ontology.CreateOntologyInput{ID: "legacy-crm", Prefix: "crm", Namespace: "https://example.org/crm/", Name: "CRM", OntologyType: domain.OntologyTypeBase}); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"6.2", "7.1.3"} {
		_, err := store.StageLegacyVersion(ctx, ontology.ImportVersionInput{
			Version:           ontology.CreateVersionInput{ID: "legacy-crm-" + v, OntologyID: "legacy-crm", VersionString: v, RDFContent: "<rdf version='" + v + "'/>"},
			Classes:           []ontology.ImportClass{{ID: "legacy-class-" + v, Prefix: "crm", LocalName: "E1", URI: "https://example.org/crm/E1", Label: domain.Translations{"en": v}}},
			NamespaceBindings: []ontology.NamespaceBinding{{Prefix: "crm", Namespace: "https://example.org/crm/"}},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	mustExec(t, p, `INSERT INTO weave_project_ontology_versions(project_id,ontology_version_id,added_at,is_primary) VALUES ('legacy-root','legacy-crm-6.2',now(),true),('legacy-root','legacy-crm-7.1.3',now(),false)`)
	if withSource {
		for _, sql := range []string{
			`INSERT INTO weave_projects(id,owner_id) VALUES ('legacy-source','legacy-owner')`,
			`INSERT INTO weave_fields(id,project_id,ontology_scope) VALUES ('legacy-source-field','legacy-source','{"prefix":"crm","local_name":"E1"}')`,
			`INSERT INTO weave_releases(project_id,version,created_by_id) VALUES ('legacy-source','0.1.0','legacy-owner')`,
			`INSERT INTO weave_project_ontology_versions(project_id,ontology_version_id,added_at,is_primary) VALUES ('legacy-source','legacy-crm-7.1.3',now(),true)`,
			`INSERT INTO weave_field_overrides(field_id,project_id,entity_type,entity_id) VALUES ('legacy-source-field','legacy-root','model','legacy-model')`,
		} {
			mustExec(t, p, sql)
		}
	}
	return ctx, p, NewService(NewPostgresStore(p), p, nil)
}

func readLegacySnapshot(ctx context.Context, t *testing.T, p *pgxpool.Pool) ontologysnapshot.Artifact {
	t.Helper()
	var receipt readsession.LegacyContentReceipt
	var saved *readsession.Session
	err := readsession.NewFactory(p).WithSourceAuthorization(auth.CheckProjectRead).Read(ctx, auth.Release(legacyVersion), legacyProject, func(s *readsession.Session) error {
		saved = s
		sources, err := s.Dependencies(ctx)
		if err != nil {
			return err
		}
		for _, source := range sources {
			bound, err := s.ProjectDependency(ctx, source.ProjectID)
			if err != nil {
				return err
			}
			contract, err := bound.ContentContract(ctx)
			if err != nil {
				return err
			}
			if contract != 2 {
				t.Fatalf("legacy dependency promoted to contract %d", contract)
			}
		}
		if _, err := s.Ontologies(ctx); apierror.FromError(err).Code != apierror.CodeNotInRelease {
			t.Fatalf("legacy evidence acquired verified ontology reader: %v", err)
		}
		if _, err := s.Content(ctx); err == nil {
			t.Error("legacy release passed verified-content boundary")
		}
		receipt, err = s.LegacyContent(ctx)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := saved.LegacyContent(ctx); err == nil {
		t.Fatal("legacy receipt escaped session lifetime")
	}
	for _, section := range loadCaptureLedger(t).Sections {
		if _, ok := receipt.Section(section.ArchiveTable); !ok {
			t.Fatalf("missing structural section %s", section.ArchiveTable)
		}
	}
	var a ontologysnapshot.Artifact
	err = ontologysnapshot.NewFactory(p).Read(ctx, receipt.Ontology(), func(r ontologysnapshot.Reader) error { var err error; a, err = r.Snapshot(ctx); return err })
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestLegacyRebuildFinalizesMixedEvidenceAndResumes(t *testing.T) {
	ctx, p, svc := seedLegacyRebuild(t, true)
	plan, err := svc.PlanLegacyRebuild(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Format != 2 || len(plan.Projects) != 2 || plan.Projects[0].ProjectID != "legacy-source" {
		t.Fatalf("wrong legacy plan: %+v", plan)
	}
	request := LegacyRebuildRequest{Plan: plan}
	if _, err := svc.RunRebuild(ctx, legacyRequest(request), checkRebuild, finishRebuild); err == nil {
		t.Fatal("ordinary lifecycle accepted legacy contract")
	}
	op, err := svc.RunLegacyRebuild(ctx, request, checkRebuild, func(context.Context, RebuildOperation) (map[string]string, error) {
		return nil, errors.New("publication interrupted")
	})
	if err == nil || op.ID == "" || len(op.Releases) != 2 {
		t.Fatalf("missing recoverable operation: %+v %v", op, err)
	}
	if release, err := rebuildlease.Runtime(ctx, p); err == nil {
		release()
		t.Fatal("pending legacy publication admitted runtime")
	}
	for _, r := range op.Releases {
		var receipt struct {
			Format int    `json:"format_version"`
			Kind   string `json:"evidence_kind"`
			Digest string `json:"ontology_snapshot_digest"`
		}
		if err := json.Unmarshal(r.Receipt, &receipt); err != nil {
			t.Fatal(err)
		}
		if receipt.Format != 2 || receipt.Kind != "legacy_ontology_snapshot" || len(receipt.Digest) != 64 {
			t.Fatalf("wrong receipt: %+v", receipt)
		}
	}
	a := readLegacySnapshot(ctx, t, p)
	d, err := a.Decode()
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Inventory) != 2 || len(d.Sources) != 1 {
		t.Fatal("snapshot lost selected versions or source")
	}
	if _, err := a.Resolve("field", "legacy-field", "/path_elements/0"); err == nil {
		t.Fatal("ambiguous reference became a definition")
	}
	definition, err := a.Resolve("field", "legacy-source-field", "/ontology_scope")
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := definition.Evidence()
	if err != nil || c.VersionID != "legacy-crm-7.1.3" {
		t.Fatalf("lost source meaning: %+v %v", c, err)
	}
	var verified int
	mustScan(t, p, `SELECT (SELECT count(*) FROM weave_releases WHERE ontology_dependencies_version<>0)+(SELECT count(*) FROM weave_release_ontology_references)+(SELECT count(*) FROM weave_ontology_releases)`, nil, &verified)
	if verified != 0 {
		t.Fatal("legacy capture manufactured verified semantic evidence")
	}
	for _, sql := range []string{
		`UPDATE weave_fields_archive SET path_elements='[]' WHERE project_id='legacy-root'`,
		`UPDATE weave_models_archive SET model_type='changed' WHERE project_id='legacy-root'`,
		`DELETE FROM weave_release_dependency_entities WHERE project_id='legacy-root'`,
		`UPDATE weave_releases SET content_capture_version=1 WHERE project_id='legacy-root'`,
		`UPDATE weave_releases SET ontology_dependencies_version=1 WHERE project_id='legacy-root'`,
		`UPDATE weave_releases SET content_capture_receipt='{}' WHERE project_id='legacy-root'`,
	} {
		_, err := p.Exec(ctx, sql)
		requireContentRejection(t, err)
	}
	mustExec(t, p, `UPDATE weave_projects SET ui_name='{"en":"Later draft"}' WHERE id='legacy-root'`)
	mustExec(t, p, `UPDATE weave_namespace_bindings SET namespace='https://later.example/' WHERE prefix='crm'`)
	finished, err := svc.RunLegacyRebuild(ctx, request, checkRebuild, finishRebuild)
	if err != nil {
		t.Fatal(err)
	}
	if !finished.Complete || string(finished.Releases[0].Receipt) != string(op.Releases[0].Receipt) {
		t.Fatal("resume recaptured evidence")
	}
	if got := readLegacySnapshot(ctx, t, p); got != a {
		t.Fatal("snapshot followed later drafts")
	}
	if _, err := svc.PlanLegacyRebuild(ctx, []string{legacyProject}); err == nil {
		t.Fatal("completed legacy capture selected for destructive rebuild")
	}
	db := stdlib.OpenDB(*p.Config().ConnConfig)
	defer db.Close()
	latest, err := database.LatestMigrationVersion()
	if err != nil {
		t.Fatal(err)
	}
	for version := latest; version > 36; version-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.MigrateDown(db); err == nil || !strings.Contains(err.Error(), "cannot downgrade retained legacy content") {
		t.Fatalf("downgrade: %v", err)
	}
}

func TestLegacyRebuildRejectsStaleOntologyAndRollsBackWholeBatch(t *testing.T) {
	for _, mode := range []string{"stale-ontology", "consumer-failure"} {
		t.Run(mode, func(t *testing.T) {
			ctx, p, svc := seedLegacyRebuild(t, true)
			plan, err := svc.PlanLegacyRebuild(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "stale-ontology" {
				mustExec(t, p, `UPDATE weave_ontology_versions SET rdf_content='changed' WHERE id='legacy-crm-6.2'`)
			} else {
				mustExec(t, p, `CREATE FUNCTION fail_legacy_consumer() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.project_id='legacy-root' THEN RAISE EXCEPTION 'consumer failed'; END IF; RETURN NEW; END $$`)
				mustExec(t, p, `CREATE TRIGGER fail_legacy_consumer BEFORE INSERT ON weave_models_archive FOR EACH ROW EXECUTE FUNCTION fail_legacy_consumer()`)
			}
			_, err = svc.RunLegacyRebuild(ctx, LegacyRebuildRequest{Plan: plan}, checkRebuild, finishRebuild)
			if err == nil {
				t.Fatal("invalid batch succeeded")
			}
			if mode == "stale-ontology" && !strings.Contains(err.Error(), "inputs or configuration changed") {
				t.Fatalf("stale body not fingerprinted: %v", err)
			}
			var evidence, old int
			mustScan(t, p, `SELECT (SELECT count(*) FROM weave_legacy_ontology_snapshots)+(SELECT count(*) FROM weave_ontology_body_seals)+(SELECT count(*) FROM weave_release_rebuild_operations)+(SELECT count(*) FROM weave_change_set WHERE kind='release')`, nil, &evidence)
			mustScan(t, p, `SELECT count(*) FROM weave_releases WHERE content_capture_version=0`, nil, &old)
			if evidence != 0 || old != 2 {
				t.Fatalf("rollback leaked cut: evidence=%d old=%d", evidence, old)
			}
		})
	}
}

type changedLegacyEvidence struct {
	legacyEvidence
	change func() error
}

func (e changedLegacyEvidence) finalize(ctx context.Context, q *sqlcgen.Queries, project, version string) error {
	if err := e.change(); err != nil {
		return err
	}
	return e.legacyEvidence.finalize(ctx, q, project, version)
}

func TestLegacyFinalizationBindsEvidenceToSameStructuralCut(t *testing.T) {
	ctx, p, svc := seedLegacyRebuild(t, false)
	tx, err := p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck
	e := changedLegacyEvidence{change: func() error {
		for _, sql := range []string{
			`UPDATE weave_fields SET ontology_scope='{"prefix":"crm","local_name":"changed"}' WHERE id='legacy-field'`,
			`UPDATE weave_fields_archive SET ontology_scope='{"prefix":"crm","local_name":"changed"}' WHERE id='legacy-field' AND version_number='1.0.0'`,
		} {
			if _, err := tx.Exec(ctx, sql); err != nil {
				return err
			}
		}
		return nil
	}}
	_, err = svc.createWithEvidence(ctx, tx, legacyProject, CreateInput{Version: "1.0.0"}, "legacy-owner", e)
	var pgerr *pgconn.PgError
	if !errors.As(err, &pgerr) || pgerr.ConstraintName != "legacy_context_incomplete" {
		t.Fatalf("changed cut acquired receipt: %v", err)
	}
}

func TestOrdinaryCreateStillRejectsLegacySemantics(t *testing.T) {
	ctx, p, svc := seedLegacyRebuild(t, false)
	_, err := svc.Create(ctx, legacyProject, CreateInput{Version: "1.0.0"})
	var validation *ErrValidation
	if !errors.As(err, &validation) || len(validation.Fields["ontologies"]) == 0 {
		t.Fatalf("ordinary create weakened: %v", err)
	}
	var count int
	mustScan(t, p, `SELECT (SELECT count(*) FROM weave_releases WHERE version='1.0.0')+(SELECT count(*) FROM weave_legacy_ontology_snapshots)`, nil, &count)
	if count != 0 {
		t.Fatal("ordinary create leaked legacy evidence")
	}
}

func TestLegacyReceiptRejectsVerifiedDraftAndZeroSessions(t *testing.T) {
	p := testdb.EmptyPool(t)
	ctx, owner, project := seedZREL(t, p)
	ctx = releaseAuthCtx(ctx, owner)
	if _, err := NewService(NewPostgresStore(p), p, nil).Create(ctx, project, CreateInput{Version: "9.0.1"}); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []auth.ReadScope{auth.Release("9.0.1"), auth.Draft()} {
		err := readsession.NewFactory(p).Read(ctx, scope, project, func(s *readsession.Session) error { _, err := s.LegacyContent(ctx); return err })
		if err == nil {
			t.Fatal("wrong evidence kind admitted legacy receipt")
		}
	}
	if _, err := (&readsession.Session{}).LegacyContent(ctx); err == nil {
		t.Fatal("unbound receipt read succeeded")
	}
}

func TestLegacyFinalizationCannotCertifyAnEarlierTransaction(t *testing.T) {
	ctx, p, _ := seedLegacyRebuild(t, false)
	tx, err := p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck
	_, err = tx.Exec(ctx, `UPDATE weave_releases SET archive_ownership_version=1,metadata_capture_version=1,project_dependencies_version=1,content_capture_version=2 WHERE project_id='legacy-root'`)
	requireContentRejection(t, err)
}

func TestLegacyFinalizationRequiresAllOntologyModuleDescriptors(t *testing.T) {
	ctx, p, svc := seedLegacyRebuild(t, false)
	mustExec(t, p, `CREATE FUNCTION strip_legacy_modules() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN NEW.document=jsonb_set(NEW.document,'{ontology_modules}','{}'); RETURN NEW; END $$`)
	mustExec(t, p, `CREATE TRIGGER zz_strip_legacy_modules BEFORE INSERT ON weave_release_descriptors FOR EACH ROW EXECUTE FUNCTION strip_legacy_modules()`)
	plan, err := svc.PlanLegacyRebuild(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.RunLegacyRebuild(ctx, LegacyRebuildRequest{Plan: plan}, checkRebuild, finishRebuild)
	if err == nil || !strings.Contains(err.Error(), "omits an ontology module") {
		t.Fatalf("incomplete module descriptors accepted: %v", err)
	}
	var captured int
	mustScan(t, p, `SELECT count(*) FROM weave_legacy_ontology_snapshots`, nil, &captured)
	if captured != 0 {
		t.Fatal("failed descriptor validation left a snapshot")
	}
}
