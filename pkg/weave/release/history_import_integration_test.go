//go:build integration

package release

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database"
	"github.com/pletka-io/pletka/pkg/database/advisorylock"
	"github.com/pletka-io/pletka/pkg/database/projectrestore"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/projectbundle"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	"github.com/pletka-io/pletka/pkg/service/projectread"
)

func catalogForImport(ctx context.Context, t *testing.T, pool *pgxpool.Pool, value projectsnapshot.CompleteProject) projectbundle.Catalog {
	t.Helper()
	ids, err := value.Projects()
	if err != nil {
		t.Fatal(err)
	}
	records := []projectbundle.ReleaseRecord{}
	for _, id := range ids {
		if id.IsDraft() {
			continue
		}
		record := projectbundle.ReleaseRecord{ProjectID: id.ProjectID(), Version: id.Version()}
		var at string
		err := pool.QueryRow(ctx, `SELECT title,description,created_at::text,created_by_id,archived_message FROM weave_releases WHERE project_id=$1 AND version=$2`, id.ProjectID(), id.Version()).Scan(&record.Title, &record.Description, &at, &record.CreatedByID, &record.ArchivedMessage)
		if err != nil {
			t.Fatal(err)
		}
		record.CreatedAt = projectsnapshot.Timestamp(at)
		records = append(records, record)
	}
	catalog, err := projectbundle.NewCatalog(value, records)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func importValue(t *testing.T, value projectsnapshot.CompleteProject) projectbundle.ImportedProject {
	t.Helper()
	files, err := projectbundle.Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := projectbundle.Decode(files, projectbundle.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	return imported
}

func checkHistoryImport(ctx context.Context, t *testing.T, source *pgxpool.Pool, value projectsnapshot.CompleteProject) {
	t.Helper()
	target := testdb.EmptyPool(t)
	imported := importValue(t, value)
	catalog := catalogForImport(ctx, t, source, value)
	dir := writeCompleteImport(t, value, catalog)
	preview, err := gitmaterializer.PreviewRestoreSnapshot(dir)
	if err != nil || !preview.PreservesHistory {
		t.Fatalf("complete preview failed: %v", err)
	}
	mat := gitmaterializer.NewMaterializer(target, t.TempDir(), nil)
	plan, err := mat.HydrateProjectSnapshot(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Complete == nil || plan.ImportResult == nil || plan.Snapshot != nil {
		t.Fatal("complete restore used legacy path")
	}
	result := *plan.ImportResult
	ids, _ := value.Projects()
	records, err := catalog.SelectedReleases(value)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Committed || result.Releases != len(records) || result.WorkingCopies != len(ids) {
		t.Fatalf("incomplete import: %+v", result)
	}
	staged := int64(0)
	for _, id := range ids {
		in, err := value.Input(id)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range in.Content.Fields {
			if f.StagingID != nil {
				staged++
			}
		}
	}
	if result.DetachedStagingRefs < staged {
		t.Fatalf("missing staging attachment report: %+v", result)
	}
	for _, id := range ids {
		if id.IsDraft() {
			if err := readsession.NewFactory(target).Read(ctx, auth.Draft(), id.ProjectID(), func(s *readsession.Session) error {
				if _, err := s.Origin(ctx); err == nil {
					t.Fatal("draft acquired immutable release provenance")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := readsession.NewFactory(source).Read(ctx, auth.Release(id.Version()), id.ProjectID(), func(s *readsession.Session) error {
			origin, err := s.Origin(ctx)
			if err != nil {
				return err
			}
			if origin.Imported || origin.ImportedAt != "" {
				t.Fatal("local capture reported as imported")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		var retainedSession *readsession.Session
		if err := readsession.NewFactory(target).Read(ctx, auth.Release(id.Version()), id.ProjectID(), func(s *readsession.Session) error {
			retainedSession = s
			origin, err := s.Origin(ctx)
			if err != nil {
				return err
			}
			if !origin.Imported || origin.ImportedAt == "" {
				t.Fatal("import provenance unavailable on normal session")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := retainedSession.Origin(ctx); err == nil {
			t.Fatal("closed session exposed provenance")
		}
		restored, err := assembleFixture(ctx, t, target, id.ProjectID(), auth.Release(id.Version()), projectread.DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		want, _ := value.Input(id)
		got, err := restored.Input(id)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(sortedContent(t, want.Content), sortedContent(t, got.Content)); diff != "" {
			t.Fatal(diff)
		}
		if diff := cmp.Diff(want.Evidence.Descriptors().Document(), got.Evidence.Descriptors().Document()); diff != "" {
			t.Fatal(diff)
		}
		if want.Evidence.Contract() == 2 {
			a, _ := want.Evidence.Legacy()
			b, _ := got.Evidence.Legacy()
			if a != b {
				t.Fatal("exact legacy evidence changed")
			}
		} else {
			a, _ := want.Evidence.Publications()
			b, _ := got.Evidence.Publications()
			if diff := cmp.Diff(sortedJSON(t, a), sortedJSON(t, b)); diff != "" {
				t.Fatal(diff)
			}
		}
		a, _ := want.Evidence.CaptureSections()
		b, _ := got.Evidence.CaptureSections()
		if diff := cmp.Diff(a, b); diff != "" {
			t.Fatal(diff)
		}
	}
	checkImportGuards(ctx, t, target, value, catalog)
	root, _ := value.Root()
	mustExec(t, target, `UPDATE weave_projects SET ui_name='{"en":"edited after import"}' WHERE id=$1`, root.ProjectID())
	repeated, err := projectrestore.Restore(ctx, target, imported, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if repeated.Releases != 0 || repeated.WorkingCopies != 0 || !repeated.Committed {
		t.Fatalf("non-idempotent import: %+v", repeated)
	}
	var title string
	if err := target.QueryRow(ctx, `SELECT ui_name->>'en' FROM weave_projects WHERE id=$1`, root.ProjectID()).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "edited after import" {
		t.Fatal("replay overwrote later edit")
	}
	conflictResult, err := projectrestore.Restore(ctx, source, imported, catalog)
	var conflict *projectrestore.ConflictError
	if !errors.As(err, &conflict) || conflictResult.Committed {
		t.Fatalf("existing local working copy was not protected: %+v %v", conflictResult, err)
	}
	if len(records) > 0 {
		changed := append([]projectbundle.ReleaseRecord(nil), records...)
		changed[0].Title += " conflicting catalog"
		changedCatalog, err := projectbundle.NewCatalog(value, changed)
		if err != nil {
			t.Fatal(err)
		}
		conflictResult, err = projectrestore.Restore(ctx, target, imported, changedCatalog)
		if !errors.As(err, &conflict) || conflictResult.Committed {
			t.Fatalf("different import overwrote retained target: %+v %v", conflictResult, err)
		}
	}
	db := stdlib.OpenDBFromPool(target)
	defer func() { _ = db.Close() }()
	if err := database.MigrateDown(db); err == nil || !strings.Contains(err.Error(), "cannot downgrade retained imported release history") {
		t.Fatalf("imported history/workspace downgrade not protected: %v", err)
	}
	for _, record := range records {
		var verified bool
		err := target.QueryRow(ctx, `SELECT r.created_at=$3::timestamptz AND r.created_by_id=$4 AND i.document->'receipt'=r.content_capture_receipt FROM weave_releases r JOIN weave_release_imports i ON i.project_id=r.project_id AND i.version_number=r.version WHERE r.project_id=$1 AND r.version=$2`, record.ProjectID, record.Version, string(record.CreatedAt), record.CreatedByID).Scan(&verified)
		if err != nil || !verified {
			t.Fatalf("original metadata/provenance lost: %v", err)
		}
	}
}

func TestCompleteHistoryImportOwnedAndDraft(t *testing.T) {
	source := testdb.EmptyPool(t)
	ctx, owner, project := seedZREL(t, source)
	ctx = releaseAuthCtx(ctx, owner)
	seedContentChildren(t, source)
	mustExec(t, source, `UPDATE weave_models SET model_type='auxiliary' WHERE project_id='ZREL'`)
	mustExec(t, source, `UPDATE weave_fields SET ui_name='null'::jsonb,description=NULL WHERE project_id='ZREL'`)
	mustExec(t, source, `INSERT INTO weave_import_staging(id,import_run,source_type,source_path,project_name,raw_data) VALUES (91001,'fixture','test','fixture','ZREL','{}')`)
	mustExec(t, source, `UPDATE weave_fields SET staging_id=91001 WHERE project_id='ZREL'`)
	mustExec(t, source, `UPDATE weave_example_values SET number_value=123456789012345678901234567890.123456789,date_value='infinity',value_payload='{"exact":123456789012345678901234567890}'`)
	draft, err := assembleFixture(ctx, t, source, project, auth.Draft(), projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	checkHistoryImport(ctx, t, source, draft)
	if _, err := NewService(NewPostgresStore(source), source, nil).Create(ctx, project, CreateInput{Version: "9.0.1"}); err != nil {
		t.Fatal(err)
	}
	value, err := assembleFixture(ctx, t, source, project, auth.Release("9.0.1"), projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	checkHistoryImport(ctx, t, source, value)
}

func TestCompleteHistoryImportLegacySource(t *testing.T) {
	ctx, source, svc := seedLegacyRebuild(t, true)
	plan, err := svc.PlanLegacyRebuild(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunLegacyRebuild(ctx, LegacyRebuildRequest{Plan: plan}, checkRebuild, finishRebuild); err != nil {
		t.Fatal(err)
	}
	value, err := assembleFixture(ctx, t, source, legacyProject, auth.Release(legacyVersion), projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	checkHistoryImport(ctx, t, source, value)
}

func writeCompleteImport(t *testing.T, value projectsnapshot.CompleteProject, catalog projectbundle.Catalog) string {
	t.Helper()
	files, err := projectbundle.Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	files[projectbundle.CatalogPath], err = projectbundle.EncodeCatalog(catalog)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for path, data := range files {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := gitmaterializer.PrepareRestorePlan(dir, "renamed"); err == nil {
		t.Fatal("history retargeted")
	}
	return dir
}

func checkImportGuards(ctx context.Context, t *testing.T, target *pgxpool.Pool, value projectsnapshot.CompleteProject, catalog projectbundle.Catalog) {
	t.Helper()
	records, err := catalog.SelectedReleases(value)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 {
		return
	}
	r := records[0]
	for _, statement := range []string{
		`UPDATE weave_release_imports SET document='{}' WHERE project_id=$1 AND version_number=$2`,
		`DELETE FROM weave_release_imports WHERE project_id=$1 AND version_number=$2`,
		`UPDATE weave_projects_archive SET ui_name='{"en":"substitution"}' WHERE id=$1 AND version_number=$2`,
		`UPDATE weave_releases SET content_capture_version=0 WHERE project_id=$1 AND version=$2`,
		`UPDATE weave_releases SET content_capture_receipt='{}' WHERE project_id=$1 AND version=$2`,
	} {
		if _, err := target.Exec(ctx, statement, r.ProjectID, r.Version); err == nil {
			t.Fatalf("raw SQL bypassed import immutability: %s", statement)
		}
	}
	if _, err := target.Exec(ctx, `TRUNCATE weave_release_imports`); err == nil {
		t.Fatal("import provenance truncated")
	}
	tx, err := target.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `INSERT INTO weave_releases(project_id,version,created_by_id) VALUES($1,'unfinished-import',$2)`, r.ProjectID, r.CreatedByID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO weave_release_imports(project_id,version_number,document) SELECT project_id,'unfinished-import',jsonb_set(document,'{header,version}','"unfinished-import"') FROM weave_release_imports WHERE project_id=$1 AND version_number=$2`, r.ProjectID, r.Version); err != nil {
		t.Fatal(err)
	}
	var pgerr *pgconn.PgError
	if err := tx.Commit(ctx); !errors.As(err, &pgerr) || pgerr.ConstraintName != "project_import_incomplete" {
		t.Fatalf("partial import committed: %v", err)
	}
}

func TestHistoryImportFailureRollsBackOntologiesSourcesAndRoot(t *testing.T) {
	ctx, source, svc := seedLegacyRebuild(t, true)
	plan, err := svc.PlanLegacyRebuild(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunLegacyRebuild(ctx, LegacyRebuildRequest{Plan: plan}, checkRebuild, finishRebuild); err != nil {
		t.Fatal(err)
	}
	value, err := assembleFixture(ctx, t, source, legacyProject, auth.Release(legacyVersion), projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	rootID, _ := value.Root()
	root, _ := value.Input(rootID)
	root.Content.Projects[0].UIName = []byte(`{"en":"substituted but same row count"}`)
	ids, _ := value.Projects()
	sources := []projectsnapshot.ProjectInput{}
	for _, id := range ids {
		if id != rootID {
			in, err := value.Input(id)
			if err != nil {
				t.Fatal(err)
			}
			sources = append(sources, in)
		}
	}
	corrupt, err := projectsnapshot.NewComplete(root, sources)
	if err != nil {
		t.Fatal(err)
	}
	target := testdb.EmptyPool(t)
	result, err := projectrestore.Restore(ctx, target, importValue(t, corrupt), catalogForImport(ctx, t, source, corrupt))
	var pgerr *pgconn.PgError
	if !errors.As(err, &pgerr) || pgerr.ConstraintName != "project_import_invalid" || result.Committed {
		t.Fatalf("substituted receipt accepted: %+v %v", result, err)
	}
	for _, table := range []string{"weave_projects", "weave_releases", "weave_release_imports", "weave_project_workspace_imports", "weave_ontology_versions", "weave_imported_legacy_ontology_snapshots"} {
		var count int
		if err := target.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("partial import retained %s: %d", table, count)
		}
	}
	checkHistoryImport(ctx, t, source, value)
}

func TestHistoryImportRootLockCancellationAndCleanup(t *testing.T) {
	source := testdb.EmptyPool(t)
	ctx, owner, project := seedZREL(t, source)
	ctx = releaseAuthCtx(ctx, owner)
	value, err := assembleFixture(ctx, t, source, project, auth.Draft(), projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	imported, catalog := importValue(t, value), catalogForImport(ctx, t, source, value)
	target := testdb.EmptyPool(t)
	held, err := target.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	key := advisorylock.ProjectLockKey(project)
	if err := advisorylock.Acquire(ctx, held, advisorylock.Exclusive, key); err != nil {
		t.Fatal(err)
	}
	blocked, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	result, err := projectrestore.Restore(blocked, target, imported, catalog)
	cancel()
	if err == nil || result.Committed {
		t.Fatal("restore bypassed busy root lock")
	}
	var count int
	if err := target.QueryRow(ctx, `SELECT count(*) FROM weave_projects`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("wrote before root lock: %v", err)
	}
	var timeout string
	if err := target.QueryRow(ctx, `SELECT current_setting('lock_timeout')`).Scan(&timeout); err != nil || timeout != "0" {
		t.Fatalf("restore leaked lock timeout: %q %v", timeout, err)
	}
	if err := advisorylock.Release(ctx, held, advisorylock.Exclusive, key, advisorylock.GraceTimeout); err != nil {
		t.Fatal(err)
	}
	if _, err := projectrestore.Restore(ctx, target, imported, catalog); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryImportPreservesDifferentHistoricalActorDescriptions(t *testing.T) {
	source := testdb.EmptyPool(t)
	ctx, owner, parent := seedZREL(t, source)
	ctx = releaseAuthCtx(ctx, owner)
	svc := NewService(NewPostgresStore(source), source, nil)
	if _, err := svc.Create(ctx, parent, CreateInput{Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	mustExec(t, source, `UPDATE weave_actors SET display_name='Changed after source release' WHERE id=$1`, owner)
	mustExec(t, source, `INSERT INTO weave_projects(id,owner_id,visibility) VALUES ('consumer',$1,'public')`, owner)
	mustExec(t, source, `INSERT INTO weave_models(id,project_id,model_type) VALUES ('consumer-model','consumer','auxiliary')`)
	mustExec(t, source, `INSERT INTO weave_field_overrides(field_id,project_id,entity_type,entity_id) VALUES ('ZRELF1','consumer','model','consumer-model')`)
	mustExec(t, source, `INSERT INTO weave_adoptions(project_id,context_entity_type,context_entity_id,entity_type,source_project_id,source_entity_id,source_version,created_by_id) VALUES ('consumer','model','consumer-model','field','ZREL','ZRELF1','1.0.0',$1)`, owner)
	if _, err := svc.Create(ctx, "consumer", CreateInput{Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	value, err := assembleFixture(ctx, t, source, "consumer", auth.Release("1.0.0"), projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	sourceID, err := projectsnapshot.Release(parent, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	sourceEvidence, err := value.Evidence(sourceID)
	if err != nil {
		t.Fatal(err)
	}
	sourceActor, err := sourceEvidence.Descriptors().Actor(&owner)
	if err != nil {
		t.Fatal(err)
	}
	root, _ := value.Root()
	rootEvidence, _ := value.Evidence(root)
	rootActor, err := rootEvidence.Descriptors().Actor(&owner)
	if err != nil {
		t.Fatal(err)
	}
	if sourceActor.DisplayName == rootActor.DisplayName {
		t.Fatal("fixture did not retain different actor contexts")
	}
	checkHistoryImport(ctx, t, source, value)
}
