//go:build integration

package example_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/database"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/database/txscope"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/service/browserread"
	"github.com/pletka-io/pletka/pkg/service/draftcapture"
	"github.com/pletka-io/pletka/pkg/service/projectread"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/example"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

type mutationFixture struct {
	pool    *pgxpool.Pool
	source  *browserread.Source
	service *example.Service
	ctx     context.Context
}

func newMutationFixture(t *testing.T) *mutationFixture {
	t.Helper()
	pool := testdb.EmptyPool(t)
	ctx := auth.WithPrincipal(auth.WithSnapshot(t.Context(), &auth.AuthSnapshot{IsSuperAdmin: true}), &auth.Principal{ActorID: "XO"})
	exec(t, pool, `INSERT INTO weave_actors(id,type,slug,display_name,visibility) VALUES('XO','organization','xo','Owner','public');
 INSERT INTO weave_projects(id,owner_id,visibility) VALUES('XS','XO','public'),('XP','XO','public'),('XQ','XO','public');
 INSERT INTO weave_models(id,project_id,ui_name) VALUES('XS.M','XS','{"en":"Selected model"}'),('XS.TARGET','XS','{"en":"Target"}'),('XS.OTHER','XS','{"en":"Other"}'),('XQ.M','XQ','{"en":"Unrelated"}');
 INSERT INTO weave_collections(id,project_id,ui_name) VALUES('XS.C','XS','{"en":"Nested source"}');
 INSERT INTO weave_vocabularies(id,project_id,connector_type) VALUES('XS.V','XS','local');
 INSERT INTO weave_vocabulary_entries(id,vocabulary_id,uri,label) VALUES('XS.ONE','XS.V','https://example.test/one','{"en":"One"}'),('XS.TWO','XS.V','https://example.test/two','{"en":"Two"}');
 INSERT INTO weave_concept_lists(id,project_id,vocabulary_id,is_closed) VALUES('XS.L','XS','XS.V',false);
 INSERT INTO weave_concept_list_entries(id,concept_list_id,vocabulary_entry_id) VALUES('XS.LE','XS.L','XS.ONE');
 INSERT INTO weave_fields(id,project_id,ui_name,expected_value_type,path_elements) VALUES('XS.F','XS','{"en":"Choice"}','Concept','[]');
 INSERT INTO weave_field_overrides(id,project_id,field_id,entity_type,entity_id,display_name) VALUES(42002,'XS','XS.F','collection','XS.C','{"en":"Selected choice"}');
 INSERT INTO weave_override_refs(override_id,ref_type,target_id,semantic_id) VALUES(42002,'concept_list','XS.L','');
 INSERT INTO weave_examples(id,project_id,entity_type,entity_id,title) VALUES('XS.LINK','XS','model','XS.TARGET','{"en":"Captured link"}'),('XQ.E','XQ','model','XQ.M','{"en":"Unrelated"}');`)
	cuts := release.NewService(release.NewPostgresStore(pool), pool, nil)
	if _, err := cuts.Create(ctx, "XS", release.CreateInput{Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	exec(t, pool, `INSERT INTO weave_project_inheritance(project_id,parent_project_id,source_mode,source_version) VALUES('XP','XS','release','1.0.0');
 INSERT INTO weave_fields(id,project_id,ui_name,expected_value_type,path_elements) VALUES('XP.N','XP','{"en":"Container"}','Collection','[]'),('XP.R','XP','{"en":"Reference"}','Model','[]'),('XP.S','XP','{"en":"Text"}','String','[]');
 INSERT INTO weave_field_overrides(id,project_id,field_id,entity_type,entity_id,display_name,position) VALUES(42001,'XP','XP.N','model','XS.M','{"en":"Container"}',0),(42003,'XP','XP.R','model','XS.M','{"en":"Reference"}',1),(42004,'XP','XP.S','model','XS.M','{"en":"Text"}',2);
 INSERT INTO weave_override_refs(override_id,ref_type,target_id,semantic_id) VALUES(42001,'collection_model','XS.C',''),(42003,'resource_model','XS.TARGET','');
 INSERT INTO weave_examples(id,project_id,entity_type,entity_id,title) VALUES('XP.LINK','XP','model','XS.TARGET','{"en":"Existing"}'),('XP.KEEP','XP','model','XS.M','{"en":"Foreign membership"}');
 INSERT INTO weave_example_values(example_id,override_id,field_id,value_kind,value_payload,linked_example_id,slot_path) VALUES('XP.KEEP',42003,'XP.R','example_ref','{"kind":"example_ref","example_id":"XS.LINK"}','XS.LINK','42003:0');`)
	source, err := browserread.New(readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead), projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := example.NewPostgresMutations(pool, source)
	if err != nil {
		t.Fatal(err)
	}
	return &mutationFixture{pool: pool, source: source, service: example.NewService(boundary), ctx: ctx}
}
func mutationInput(values ...domain.ExampleValue) example.CreateInput {
	return example.CreateInput{EntityType: domain.ExampleEntityTypeModel, EntityID: "XS.M", Title: domain.Translations{"en": "Parent"}, Values: values}
}
func conceptValue(uri string) domain.ExampleValue {
	return domain.ExampleValue{FieldID: "XS.F", SlotPath: "42001:0/42002:0", ValuePayload: domain.ExampleValuePayload{Kind: domain.ExampleValueKindConcept, ConceptURI: &uri}}
}
func referenceValue(id string) domain.ExampleValue {
	return domain.ExampleValue{FieldID: "XP.R", SlotPath: "42003:0", ValuePayload: domain.ExampleValuePayload{Kind: domain.ExampleValueKindExampleRef, ExampleID: &id}}
}
func draftStub(label string) domain.ExampleValue {
	return domain.ExampleValue{FieldID: "XP.R", SlotPath: "42003:0", ValuePayload: domain.ExampleValuePayload{Kind: domain.ExampleValueKindExampleRef, TargetLabel: &label}}
}
func exampleCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM weave_examples WHERE project_id='XP'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMutationsUseTheSamePinnedDefinitionsAsForms(t *testing.T) {
	f := newMutationFixture(t)
	exec(t, f.pool, `UPDATE weave_fields SET expected_value_type='String' WHERE id='XS.F';DELETE FROM weave_concept_list_entries WHERE concept_list_id='XS.L';UPDATE weave_examples SET entity_id='XS.OTHER',title='{"en":"Changed hot link"}' WHERE id='XS.LINK';`)
	cuts := release.NewService(release.NewPostgresStore(f.pool), f.pool, nil)
	if _, err := cuts.Create(f.ctx, "XS", release.CreateInput{Version: "1.1.0"}); err != nil {
		t.Fatal(err)
	}
	for _, uri := range []string{"https://example.test/one", "https://example.test/two"} {
		result, err := f.service.Create(f.ctx, auth.Draft(), "XP", mutationInput(conceptValue(uri), referenceValue("XS.LINK")))
		if err != nil {
			t.Fatal(err)
		}
		expected := uri == "https://example.test/one"
		if result.Validation.Valid != expected {
			t.Fatalf("source pin ignored for %s: %+v", uri, result.Validation)
		}
		detail, err := f.source.Detail(f.ctx, auth.Draft(), "XP", "example", result.Example.ID)
		if err != nil {
			t.Fatal(err)
		}
		read, form, err := example.ProjectRead(f.ctx, detail, 2, "en", nil)
		if err != nil {
			t.Fatal(err)
		}
		if read.Validation.Valid != expected || form.Target.Name["en"] != "Selected model" || form.Sections[0].Groups[0].Fields[0].Nested.Fields[0].ExpectedValueType != "Concept" {
			t.Fatalf("form/write definitions diverged: %+v %+v", read.Validation, form)
		}
		if len(form.LinkedExamples) != 1 || form.LinkedExamples[0].Title["en"] != "Captured link" {
			t.Fatalf("linked example came from hot rows: %+v", form.LinkedExamples)
		}
	}
	// A later sealed source release has exactly the same membership validation.
	exec(t, f.pool, `UPDATE weave_fields SET expected_value_type='Concept' WHERE id='XS.F';INSERT INTO weave_concept_list_entries(id,concept_list_id,vocabulary_entry_id) VALUES('XS.LE2','XS.L','XS.ONE');UPDATE weave_concept_lists SET is_closed=true WHERE id='XS.L';`)
	if _, err := cuts.Create(f.ctx, "XS", release.CreateInput{Version: "1.2.0"}); err != nil {
		t.Fatal(err)
	}
	exec(t, f.pool, `UPDATE weave_project_inheritance SET source_version='1.2.0' WHERE project_id='XP';`)
	for _, uri := range []string{"https://example.test/one", "https://example.test/two"} {
		result, err := f.service.Create(f.ctx, auth.Draft(), "XP", mutationInput(conceptValue(uri)))
		if err != nil || result.Validation.Valid != (uri == "https://example.test/one") {
			t.Fatalf("sealed membership: %+v %v", result, err)
		}
	}
}

func TestMutationParentAndStubsRollbackTogether(t *testing.T) {
	f := newMutationFixture(t)
	before := exampleCount(t, f.pool)
	bad := draftStub("Duplicate slot")
	bad.OccurrenceIndex = 1 // disagrees with its :0 path, after stub creation
	in := mutationInput(draftStub("Created before bad value"), bad)
	if result, err := f.service.Create(f.ctx, auth.Draft(), "XP", in); err == nil || result != nil {
		t.Fatalf("bad save: %+v %v", result, err)
	}
	if got := exampleCount(t, f.pool); got != before {
		t.Fatalf("validation leaked %d stubs", got-before)
	}
	if in.Values[0].ValuePayload.ExampleID != nil || in.Values[1].ValuePayload.ExampleID != nil {
		t.Fatal("failed save mutated caller input")
	}
	exec(t, f.pool, `CREATE FUNCTION reject_example_parent() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.title->>'en'='fail-parent' THEN RAISE EXCEPTION 'parent rejected';END IF;RETURN NEW;END$$;CREATE TRIGGER reject_example_parent BEFORE INSERT ON weave_examples FOR EACH ROW EXECUTE FUNCTION reject_example_parent();`)
	failed := mutationInput(draftStub("Must roll back"))
	failed.Title = domain.Translations{"en": "fail-parent"}
	if result, err := f.service.Create(f.ctx, auth.Draft(), "XP", failed); err == nil || result != nil {
		t.Fatalf("database failure: %+v %v", result, err)
	}
	if got := exampleCount(t, f.pool); got != before {
		t.Fatalf("DB failure leaked %d rows", got-before)
	}
	good := mutationInput(draftStub("One shared stub"))
	duplicate := draftStub("One shared stub")
	duplicate.SlotPath = "42003:1"
	good.Values = append(good.Values, duplicate)
	result, err := f.service.Create(f.ctx, auth.Draft(), "XP", good)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Values) != 2 || *result.Values[0].LinkedExampleID != *result.Values[1].LinkedExampleID || exampleCount(t, f.pool) != before+2 {
		t.Fatalf("stub deduplication/atomic parent: %+v", result)
	}
	if good.Values[0].ValuePayload.ExampleID != nil {
		t.Fatal("save mutated input values")
	}
}

type interceptSource struct {
	source *browserread.Source
	after  func()
	ctx    context.Context
	calls  int
}

func (s *interceptSource) Detail(ctx context.Context, scope auth.ReadScope, project, kind, id string) (browserinput.Detail, error) {
	s.ctx = ctx
	s.calls++
	v, err := s.source.Detail(ctx, scope, project, kind, id)
	if err == nil && s.after != nil {
		after := s.after
		s.after = nil
		after()
	}
	return v, err
}
func serviceWithIntercept(t *testing.T, f *mutationFixture, after func()) (*example.Service, *interceptSource) {
	t.Helper()
	source := &interceptSource{source: f.source, after: after}
	boundary, err := example.NewPostgresMutations(f.pool, source)
	if err != nil {
		t.Fatal(err)
	}
	return example.NewService(boundary), source
}

func TestMutationReadsOneSnapshotAndRejectsConcurrentUpdates(t *testing.T) {
	f := newMutationFixture(t)
	// Publish the alternate source before this operation; only the consumer's pin
	// changes between reading the form structure and validating the linked value.
	exec(t, f.pool, `UPDATE weave_examples SET entity_id='XS.OTHER' WHERE id='XS.LINK';`)
	cuts := release.NewService(release.NewPostgresStore(f.pool), f.pool, nil)
	if _, err := cuts.Create(f.ctx, "XS", release.CreateInput{Version: "1.1.0"}); err != nil {
		t.Fatal(err)
	}
	var intercept *interceptSource
	svc, intercept := serviceWithIntercept(t, f, func() {
		exec(t, f.pool, `UPDATE weave_project_inheritance SET source_version='1.1.0' WHERE project_id='XP';`)
		retained, err := f.source.Detail(intercept.ctx, auth.Draft(), "XP", "example", "XS.LINK")
		if err != nil {
			t.Fatal(err)
		}
		root, err := retained.Fragment.Root()
		if err != nil || root.Source.Version() != "1.0.0" {
			t.Fatalf("mixed read snapshot: %+v %v", root, err)
		}
	})
	// The existing project MVCC fence rejects the write after a concurrent repin.
	// No result or partial writes may escape, and the operation never retries.
	count := exampleCount(t, f.pool)
	result, err := svc.Create(f.ctx, auth.Draft(), "XP", mutationInput(referenceValue("XS.LINK")))
	if result != nil || mutationErrorCode(err) != apierror.CodeConflict || exampleCount(t, f.pool) != count {
		t.Fatalf("stale operation committed: %+v %v", result, err)
	}
	if _, err := f.source.Detail(intercept.ctx, auth.Draft(), "XP", "model", "XS.M"); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("escaped operation context: %v", err)
	}
	fresh, err := f.service.Create(f.ctx, auth.Draft(), "XP", mutationInput(referenceValue("XS.LINK")))
	if err != nil || fresh.Validation.Valid {
		t.Fatalf("fresh operation did not see change: %+v %v", fresh, err)
	}
	before := exampleCount(t, f.pool)
	svc, _ = serviceWithIntercept(t, f, func() {
		exec(t, f.pool, fmt.Sprintf(`UPDATE weave_examples SET title='{"en":"Concurrent author"}' WHERE id='%s'`, fresh.Example.ID))
	})
	changedTitle := domain.Translations{"en": "Stale overwrite"}
	changed, err := svc.Update(f.ctx, auth.Draft(), "XP", fresh.Example.ID, example.UpdateInput{Title: &changedTitle, Values: []domain.ExampleValue{draftStub("Conflict stub")}})
	if changed != nil || mutationErrorCode(err) != apierror.CodeConflict {
		t.Fatalf("concurrent overwrite: %+v %v", changed, err)
	}
	if exampleCount(t, f.pool) != before {
		t.Fatal("conflict leaked a stub")
	}
	var title string
	if err := f.pool.QueryRow(t.Context(), `SELECT title->>'en' FROM weave_examples WHERE id=$1`, fresh.Example.ID).Scan(&title); err != nil || title != "Concurrent author" {
		t.Fatalf("overwrote concurrent author: %q %v", title, err)
	}
}

func TestMutationOwnershipSourceAuthorizationAndOuterRollback(t *testing.T) {
	f := newMutationFixture(t)
	before := exampleCount(t, f.pool)
	if _, err := f.service.Update(f.ctx, auth.Draft(), "XP", "XQ.E", example.UpdateInput{}); mutationErrorCode(err) != apierror.CodeNotFound {
		t.Fatalf("cross-project update: %v", err)
	}
	if err := f.service.Delete(f.ctx, auth.Draft(), "XP", "XQ.E"); mutationErrorCode(err) != apierror.CodeNotFound {
		t.Fatalf("cross-project delete: %v", err)
	}
	for _, scope := range []auth.ReadScope{{}, auth.Release("1.0.0")} {
		if _, err := f.service.Create(f.ctx, scope, "XP", mutationInput()); err == nil {
			t.Fatal("non-draft mutation accepted")
		}
	}
	exec(t, f.pool, `UPDATE weave_projects SET visibility='private' WHERE id='XS';`)
	if _, err := f.service.Create(t.Context(), auth.Draft(), "XP", mutationInput()); mutationErrorCode(err) != apierror.CodeForbidden {
		t.Fatalf("private source read: %v", err)
	}
	if exampleCount(t, f.pool) != before {
		t.Fatal("denied writes changed data")
	}
	// A missing/unselected link must fail now that storage has no hot-target FK.
	for _, id := range []string{"MISSING", "XQ.E"} {
		for _, slot := range []string{"42003:0", "49999:0"} {
			value := referenceValue(id)
			value.SlotPath = slot
			result, err := f.service.Create(f.ctx, auth.Draft(), "XP", mutationInput(value))
			if result != nil || err == nil || exampleCount(t, f.pool) != before {
				t.Fatalf("unselected linked target persisted: %s %s %+v %v", id, slot, result, err)
			}
			result, err = f.service.Update(f.ctx, auth.Draft(), "XP", "XP.KEEP", example.UpdateInput{Values: []domain.ExampleValue{value}})
			if result != nil || err == nil {
				t.Fatalf("unselected linked target updated: %s %s %+v %v", id, slot, result, err)
			}
		}
	}
	// The operation creates a savepoint when a transaction is already owned.
	sentinel := errors.New("outer operation failed")
	err := txscope.Run(f.ctx, f.pool, func(bound context.Context) error {
		if _, err := f.service.Create(bound, auth.Draft(), "XP", mutationInput(draftStub("Outer rollback"))); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) || exampleCount(t, f.pool) != before {
		t.Fatalf("nested operation committed independently: %v", err)
	}
	// Deleting an owned broken example needs no read of its invalid old graph.
	exec(t, f.pool, `INSERT INTO weave_examples(id,project_id,entity_type,entity_id) VALUES('XP.BROKEN','XP','model','REMOVED.MODEL');`)
	if err := f.service.Delete(f.ctx, auth.Draft(), "XP", "XP.BROKEN"); err != nil {
		t.Fatal(err)
	}
}

func TestExampleMutationJoinsCaptureWithoutEnrollingOrdinarySaves(t *testing.T) {
	f := newMutationFixture(t)
	capture, err := draftcapture.New(f.pool, projectread.DefaultOptions(), auth.CheckProjectRead)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := capture.Enroll(f.ctx, "XP"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Create(f.ctx, auth.Draft(), "XP", mutationInput()); err == nil {
		t.Fatal("enrolled write bypassed capture")
	}
	request := draftcapture.Request{ProjectID: "XP", ActorID: "XO", ActorName: "Author", ActorEmail: "author@example.test", Message: "Create parent and stub"}
	receipt, err := capture.Mutate(f.ctx, request, func(bound context.Context) error {
		_, err := f.service.Create(bound, auth.Draft(), "XP", mutationInput(draftStub("Captured stub")))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Revision() != 1 {
		t.Fatalf("capture revision %d", receipt.Revision())
	}
	if _, err := receipt.Patch(); err != nil {
		t.Fatal(err)
	}
	before := exampleCount(t, f.pool)
	_, err = capture.Mutate(f.ctx, request, func(bound context.Context) error {
		if _, err := f.service.Create(bound, auth.Draft(), "XP", mutationInput(draftStub("Rolled back capture"))); err != nil {
			return err
		}
		return errors.New("cancel capture")
	})
	if err == nil || exampleCount(t, f.pool) != before {
		t.Fatal("capture failure left mutations")
	}
}

func mutationErrorCode(err error) apierror.Code {
	if err == nil {
		return ""
	}
	return apierror.FromError(err).Code
}
func exec(t *testing.T, pool *pgxpool.Pool, sql string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), sql); err != nil {
		t.Fatalf("seed exec failed: %v", err)
	}
}

func TestMutationKeepsArchivedLinksAndClearsOnlyOwnedDraftReferences(t *testing.T) {
	f := newMutationFixture(t)
	// A foreign consumer's archived target survives deletion of its source draft.
	if err := f.service.Delete(f.ctx, auth.Draft(), "XS", "XS.LINK"); err != nil {
		t.Fatal(err)
	}
	var link *string
	if err := f.pool.QueryRow(t.Context(), `SELECT linked_example_id FROM weave_example_values WHERE example_id='XP.KEEP'`).Scan(&link); err != nil || link == nil || *link != "XS.LINK" {
		t.Fatalf("foreign pin cleared: %v %v", link, err)
	}
	recorded, err := f.service.Create(f.ctx, auth.Draft(), "XP", mutationInput(referenceValue("XS.LINK")))
	if err != nil || !recorded.Validation.Valid {
		t.Fatalf("archived-only reference: %+v %v", recorded, err)
	}
	// Downgrade cannot recreate the hot FK by dropping legitimate retained links.
	db := stdlib.OpenDBFromPool(f.pool)
	t.Cleanup(func() {
		if err := database.Migrate(db); err != nil {
			t.Error(err)
		}
		_ = db.Close()
	})
	// Later migrations downgrade cleanly; the guard belongs to migration 044.
	latest, err := database.LatestMigrationVersion()
	if err != nil {
		t.Fatal(err)
	}
	for version := latest; version > 44; version-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
	var pgErr *pgconn.PgError
	if err := database.MigrateDown(db); !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Fatalf("archived-link downgrade guard: %v", err)
	}
	stale := referenceValue("XS.LINK")
	stale.SlotPath = "49999:0"
	if result, err := f.service.Create(f.ctx, auth.Draft(), "XP", mutationInput(stale)); err != nil || result.Values[0].LinkedExampleID == nil {
		t.Fatalf("valid retained link on removed slot: %+v %v", result, err)
	}
	// Own draft references retain their raw payload but drop the active link column,
	// allowing the existing missing-link validation to report the recorded value.
	own, err := f.service.Create(f.ctx, auth.Draft(), "XP", mutationInput(referenceValue("XP.LINK")))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.Delete(f.ctx, auth.Draft(), "XP", "XP.LINK"); err != nil {
		t.Fatal(err)
	}
	detail, err := f.source.Detail(f.ctx, auth.Draft(), "XP", "example", own.Example.ID)
	if err != nil {
		t.Fatal(err)
	}
	read, _, err := example.ProjectRead(f.ctx, detail, 2, "en", nil)
	if err != nil {
		t.Fatal(err)
	}
	if read.Validation.Valid || read.Values[0].LinkedExampleID != nil || read.Values[0].ValuePayload.ExampleID == nil || *read.Values[0].ValuePayload.ExampleID != "XP.LINK" {
		t.Fatalf("deleted own reference lost evidence: %+v", read)
	}
}
