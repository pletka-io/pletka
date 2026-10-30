//go:build integration

package release

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/service/projectread"
)

func assembleFixture(ctx context.Context, t *testing.T, pool *pgxpool.Pool, project string, scope auth.ReadScope, options projectread.Options) (projectsnapshot.CompleteProject, error) {
	t.Helper()
	assembler, err := projectread.New(options)
	if err != nil {
		return projectsnapshot.CompleteProject{}, err
	}
	var value projectsnapshot.CompleteProject
	err = readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead).Read(ctx, scope, project, func(s *readsession.Session) error { var err error; value, err = assembler.Project(ctx, s); return err })
	return value, err
}

// The only normalization permitted by this parity test is the documented cut
// transformation: concrete version_number on each row and publication status on
// these seven entity sections. Example assessment status is never rewritten.
func normalizedCut(c projectsnapshot.Content) projectsnapshot.Content {
	v := reflect.ValueOf(&c).Elem()
	for i := range v.NumField() {
		rows := v.Field(i)
		for j := range rows.Len() {
			rows.Index(j).FieldByName("VersionNumber").SetString("9.0.1")
		}
	}
	publish := func(status *string) {
		if *status != "deprecated" {
			*status = "published"
		}
	}
	for i := range c.Projects {
		publish(&c.Projects[i].Status)
	}
	for i := range c.Models {
		publish(&c.Models[i].Status)
	}
	for i := range c.Fields {
		publish(&c.Fields[i].Status)
	}
	for i := range c.Categories {
		publish(&c.Categories[i].Status)
	}
	for i := range c.Collections {
		publish(&c.Collections[i].Status)
	}
	for i := range c.ConceptLists {
		publish(&c.ConceptLists[i].Status)
	}
	for i := range c.Vocabularies {
		publish(&c.Vocabularies[i].Status)
	}
	return c
}

func TestProjectAssemblerLosslessPagesAndCutParity(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx, owner, project := seedZREL(t, pool)
	ctx = releaseAuthCtx(ctx, owner)
	seedContentChildren(t, pool)
	for _, sql := range []string{
		`INSERT INTO weave_fields(id,project_id,ui_name,path_elements) SELECT 'page-'||i,'ZREL',jsonb_build_object('en','Field '||i),'[]' FROM generate_series(1,305)i`,
		`UPDATE weave_models SET model_type='auxiliary' WHERE project_id='ZREL'`,
		`INSERT INTO weave_collections(id,project_id,default_category_id) VALUES ('empty','ZREL','ZREL.CAT.1')`,
		`INSERT INTO weave_categories(id,project_id) VALUES ('ZREL.CAT.2','ZREL')`,
		`INSERT INTO weave_collection_placements(project_id,model_id,collection_id,category_id,min_occurs) VALUES ('ZREL','ZRELM.1','empty','ZREL.CAT.1',0),('ZREL','ZRELM.1','empty','ZREL.CAT.2',2)`,
		`INSERT INTO weave_field_overrides(field_id,project_id,entity_type,entity_id,is_required,display_name) VALUES ('ZRELF1','ZREL','model','ZRELM.1',false,'{"en":"Repeated"}')`,
		`UPDATE weave_example_values SET number_value=123456789012345678901234567890.123456789,date_value='infinity',slot_path='[1,2]',value_payload='{"exact":123456789012345678901234567890}'`,
		`INSERT INTO weave_example_values(example_id,field_id,override_id,occurrence_index,slot_path,date_value,value_kind,value_payload) SELECT example_id,field_id,override_id,1,'[3,4]','2020-04-05',value_kind,value_payload FROM weave_example_values`,
		`INSERT INTO weave_example_values(example_id,field_id,override_id,text_value,value_kind,value_payload,slot_path) VALUES ('receipt-example','stale-field',999999,'retained','string','{"value":"retained"}','[9]')`,
		`INSERT INTO weave_project_actors(project_id,actor_id,role) VALUES ('ZREL','ZREL_OWNER','owner'),('ZREL','ZREL_OWNER','editor')`,
		`INSERT INTO weave_project_attributions(project_id,actor_id,kind,position,note) VALUES ('ZREL','ZREL_OWNER','author',0,'first'),('ZREL','ZREL_OWNER','author',1,'second')`,
		`INSERT INTO weave_namespace_bindings(id,project_id,prefix,namespace,weight,source,airtable_raw,created_at,updated_at) VALUES ('alias-a','ZREL','alias','https://a/',20,'ontology','{"source":"legacy"}',now(),now()),('alias-b','ZREL','alias','https://b/',20,'ontology','{"source":"legacy"}',now(),now())`,
	} {
		mustExec(t, pool, sql)
	}
	opts := projectread.DefaultOptions()
	opts.Limits.PageSize = 7
	before, err := assembleFixture(ctx, t, pool, project, auth.Draft(), opts)
	if err != nil {
		t.Fatal(err)
	}
	beforeID, err := before.Root()
	if err != nil {
		t.Fatal(err)
	}
	want, err := before.Content(beforeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(want.Fields) != 306 || len(want.Placements) != 2 || len(want.Attributions) != 2 || len(want.ExampleValues) != 3 {
		t.Fatal("pagination or repeated records lost")
	}
	if got := want.ExampleValues[0].NumberValue; got == nil || string(*got) != "123456789012345678901234567890.123456789" {
		t.Fatal("numeric precision lost")
	}
	if _, err := NewService(NewPostgresStore(pool), pool, nil).Create(ctx, project, CreateInput{Version: "9.0.1"}); err != nil {
		t.Fatal(err)
	}
	// Connection policy must not change retained scalar representations.
	cfg := pool.Config()
	cfg.ConnConfig.RuntimeParams["DateStyle"] = "SQL, DMY"
	cfg.ConnConfig.RuntimeParams["TimeZone"] = "Pacific/Auckland"
	regional, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer regional.Close()
	released, err := assembleFixture(ctx, t, regional, project, auth.Release("9.0.1"), opts)
	if err != nil {
		t.Fatal(err)
	}
	releasedID, err := released.Root()
	if err != nil {
		t.Fatal(err)
	}
	got, err := released.Content(releasedID)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(normalizedCut(want), got); diff != "" {
		t.Fatal(diff)
	}
	mustExec(t, pool, `UPDATE weave_fields SET ui_name='{"en":"changed"}' WHERE project_id='ZREL'`)
	mustExec(t, pool, `UPDATE weave_actors SET slug='new-identity' WHERE id='ZREL_OWNER'`)
	again, err := assembleFixture(ctx, t, pool, project, auth.Release("9.0.1"), opts)
	if err != nil {
		t.Fatal(err)
	}
	stable, err := again.Content(releasedID)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(got, stable); diff != "" {
		t.Fatal(diff)
	}
	opts.Limits.MaxRows = 10
	truncated, err := assembleFixture(ctx, t, pool, project, auth.Release("9.0.1"), opts)
	var budget *projectread.BudgetError
	if !errors.As(err, &budget) {
		t.Fatalf("budget did not fail: %v", err)
	}
	if _, err := truncated.Root(); err == nil {
		t.Fatal("budget returned partial value")
	}
}

func TestProjectAssemblerPreservesLegacySourceEvidence(t *testing.T) {
	ctx, pool, svc := seedLegacyRebuild(t, true)
	plan, err := svc.PlanLegacyRebuild(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunLegacyRebuild(ctx, LegacyRebuildRequest{Plan: plan}, checkRebuild, finishRebuild); err != nil {
		t.Fatal(err)
	}
	value, err := assembleFixture(ctx, t, pool, legacyProject, auth.Release(legacyVersion), projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	id, err := value.Root()
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := value.Evidence(id)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Contract() != 2 {
		t.Fatal("legacy contract promoted")
	}
	artifact, err := evidence.Legacy()
	if err != nil {
		t.Fatal(err)
	}
	document, err := artifact.Decode()
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Assessments) < 2 || len(document.SourceRefs) != 1 {
		t.Fatal("legacy evidence or source closure lost")
	}
	if _, err := evidence.Publications(); err == nil {
		t.Fatal("legacy exposed as verified")
	}
}

type assemblyTrace struct {
	pids      map[uint32]bool
	queries   []string
	mutate    func()
	triggered bool
}

func (tr *assemblyTrace) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	tr.pids[conn.PgConn().PID()] = true
	tr.queries = append(tr.queries, data.SQL)
	if !tr.triggered && strings.HasPrefix(data.SQL, "-- name: ContentFields") {
		tr.triggered = true
		tr.mutate()
	}
	return ctx
}
func (*assemblyTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestProjectAssemblerOneSnapshotAndDetachedLifetime(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx, owner, project := seedZREL(t, pool)
	ctx = releaseAuthCtx(ctx, owner)
	mustExec(t, pool, `INSERT INTO weave_project_attributions(project_id,actor_id,kind,note) VALUES ('ZREL','ZREL_OWNER','author','before')`)
	tr := &assemblyTrace{pids: map[uint32]bool{}}
	tr.mutate = func() {
		mustExec(t, pool, `UPDATE weave_fields SET system_name='changed' WHERE project_id='ZREL'`)
		mustExec(t, pool, `UPDATE weave_field_overrides SET display_name='{"en":"changed"}' WHERE project_id='ZREL'`)
		mustExec(t, pool, `UPDATE weave_projects SET ui_name='{"en":"changed"}' WHERE id='ZREL'`)
		mustExec(t, pool, `UPDATE weave_project_attributions SET note='changed' WHERE project_id='ZREL'`)
		mustExec(t, pool, `UPDATE weave_actors SET display_name='changed' WHERE id='ZREL_OWNER'`)
	}
	cfg := pool.Config()
	cfg.MaxConns, cfg.MinConns = 1, 0
	cfg.ConnConfig.Tracer = tr
	one, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(one.Close)
	assembler, err := projectread.New(projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var value projectsnapshot.CompleteProject
	var escaped *readsession.Session
	err = readsession.NewFactory(one).WithSourceAuthorization(auth.CheckProjectRead).Read(bounded, auth.Draft(), project, func(s *readsession.Session) error {
		escaped = s
		var err error
		value, err = assembler.Project(bounded, s)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	one.Close()
	id, err := value.Root()
	if err != nil {
		t.Fatal(err)
	}
	c, err := value.Content(id)
	if err != nil {
		t.Fatal(err)
	}
	if c.Attributions[0].Note != "before" {
		t.Fatal("credit escaped snapshot")
	}
	if !tr.triggered || len(tr.pids) != 1 || len(tr.queries) < 22 {
		t.Fatalf("did not prove one backend/all sections: %d", len(tr.queries))
	}
	if c.Fields[0].SystemName == nil || *c.Fields[0].SystemName != "release_probe" || strings.Contains(string(c.Projects[0].UIName), "changed") || strings.Contains(string(c.Overrides[0].DisplayName), "changed") {
		t.Fatal("mixed section snapshot")
	}
	e, err := value.Evidence(id)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := e.Descriptors().Actor(&owner)
	if err != nil || actor.DisplayName == "changed" {
		t.Fatalf("descriptor escaped snapshot: %v", err)
	}
	c.Fields[0].UIName[0] = '!'
	fresh, err := value.Content(id)
	if err != nil || fresh.Fields[0].UIName[0] == '!' {
		t.Fatalf("value buffer shared: %v", err)
	}
	if _, err := assembler.Project(ctx, escaped); err == nil {
		t.Fatal("closed session accepted")
	}
}
