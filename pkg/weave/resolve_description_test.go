//go:build integration

package weave_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/pletka-io/pletka/pkg/weave"
)

// descFixture seeds one model and one collection; each test field gets a
// description on the field itself and optional collection and model
// overrides. Empty strings stand for an override with description {}; a
// missing collection override is cOverride == nil.
type descFixture struct {
	pool                  *pgxpool.Pool
	projectID             string
	modelID, collectionID string
	archive               string // version_number, "" for live tables
}

func newDescFixture(t *testing.T, archive string) *descFixture {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()
	f := &descFixture{pool: pool, projectID: ids.GenerateULID(), modelID: ids.GenerateULID(), collectionID: ids.GenerateULID(), archive: archive}
	mustExec(t, pool, `INSERT INTO weave_models (id, status, project_id) VALUES ($1, 'draft', $2)`, f.modelID, f.projectID)
	mustExec(t, pool, `INSERT INTO weave_collections (id, status, project_id) VALUES ($1, 'draft', $2)`, f.collectionID, f.projectID)
	t.Cleanup(func() {
		for _, tbl := range []string{"weave_field_overrides_archive", "weave_fields_archive", "weave_field_overrides", "weave_fields", "weave_models", "weave_collections"} {
			_, _ = pool.Exec(ctx, `DELETE FROM `+tbl+` WHERE project_id = $1`, f.projectID)
		}
	})
	return f
}

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func descJSON(s string) []byte {
	if s == "" {
		return []byte(`{}`)
	}
	return []byte(`{"en":"` + s + `"}`)
}

// field inserts a field with description fieldDesc, a collection override
// (when cOverride != nil) and a model override placed in the collection
// when inCollection is true.
func (f *descFixture) field(t *testing.T, fieldDesc string, cOverride *string, mOverride string, inCollection bool) string {
	t.Helper()
	fieldID := ids.GenerateULID()
	fieldsTbl, overridesTbl := "weave_fields", "weave_field_overrides"
	if f.archive != "" {
		fieldsTbl, overridesTbl = "weave_fields_archive", "weave_field_overrides_archive"
	}
	mustExec(t, f.pool, `INSERT INTO `+fieldsTbl+` (id, status, project_id, description, version_number) VALUES ($1, 'draft', $2, $3, $4)`,
		fieldID, f.projectID, descJSON(fieldDesc), f.archive)
	ins := `INSERT INTO ` + overridesTbl + `
		(field_id, project_id, entity_type, entity_id, part_of_collection_id, display_name, description, version_number)
		VALUES ($1, $2, $3, $4, $5, '{"en":"x"}', $6, $7)`
	if cOverride != nil {
		mustExec(t, f.pool, ins, fieldID, f.projectID, "collection", f.collectionID, "", descJSON(*cOverride), f.archive)
	}
	part := ""
	if inCollection {
		part = f.collectionID
	}
	mustExec(t, f.pool, ins, fieldID, f.projectID, "model", f.modelID, part, descJSON(mOverride), f.archive)
	return fieldID
}

func (f *descFixture) ctx() context.Context {
	if f.archive == "" {
		return context.Background()
	}
	return weaveauth.WithProjectVersion(context.Background(), f.archive)
}

func modelDescriptions(t *testing.T, f *descFixture) map[string]string {
	t.Helper()
	mv, err := weave.NewPostgresStore(f.pool).ModelView(f.ctx(), f.modelID, f.projectID)
	if err != nil {
		t.Fatalf("ModelView: %v", err)
	}
	got := map[string]string{}
	for _, cg := range mv.Categories {
		for _, cl := range cg.Collections {
			for _, rf := range cl.Fields {
				got[rf.ID] = rf.Description["en"]
			}
		}
	}
	return got
}

func strp(s string) *string { return &s }

// An empty override description inherits: model -> collection (the one the
// model override places the field in) -> field.
func testDescriptionChain(t *testing.T, archive string) {
	f := newDescFixture(t, archive)
	modelWins := f.field(t, "F", strp("C"), "M", true)
	collectionWins := f.field(t, "F", strp("C"), "", true)
	fieldWins := f.field(t, "F", strp(""), "", true)
	noCollectionOverride := f.field(t, "F", nil, "", true)
	direct := f.field(t, "F", strp("C"), "", false)

	got := modelDescriptions(t, f)
	for id, want := range map[string]string{modelWins: "M", collectionWins: "C", fieldWins: "F", noCollectionOverride: "F", direct: "F"} {
		if got[id] != want {
			t.Errorf("field %s: description %q, want %q", id, got[id], want)
		}
	}

	var rows []domain.ResolvedField
	var err error
	rows, err = weave.NewPostgresStore(f.pool).CollectionView(f.ctx(), f.collectionID, f.projectID)
	if err != nil {
		t.Fatalf("CollectionView: %v", err)
	}
	cv := map[string]string{}
	for _, rf := range rows {
		cv[rf.ID] = rf.Description["en"]
	}
	if cv[collectionWins] != "C" || cv[fieldWins] != "F" {
		t.Errorf("CollectionView descriptions = %v, want C for %s and F for %s", cv, collectionWins, fieldWins)
	}
}

func TestResolve_DescriptionFallsThrough(t *testing.T)        { testDescriptionChain(t, "") }
func TestResolve_DescriptionFallsThroughVersion(t *testing.T) { testDescriptionChain(t, "1.0.0") }
