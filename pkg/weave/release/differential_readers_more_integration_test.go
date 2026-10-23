//go:build integration

package release

// The differential reader check, extended to the three converted slices
// beyond collection. Same contract as
// differential_readers_integration_test.go: cut a REAL release through
// snapshotStatements, then ask At(Draft()) and At(Release(v)) the same
// questions and require the same answers.
//
// These three are stricter than collection, because the cut treats their
// tables differently. Only the five entity tables (projects, categories,
// fields, models, collections) get the status rewrite; overrides, override
// refs, collection placements, examples and example values are copied column
// for column with no transformation at all. So for override and example the
// answers must match EXACTLY -- there is nothing legitimate to normalize, and
// any difference is a bug in one of the two queries.
//
// category is the one that still needs the status and version normalizations,
// for the same reason collection does.

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/category"
	"github.com/pletka-io/pletka/pkg/weave/example"
	"github.com/pletka-io/pletka/pkg/weave/override"
)

const (
	diffxProject = "DIFFX"
	diffxVersion = "1.0.0"
)

// diffxFixture is everything the three slices need to answer non-trivially:
// categories, a model, a collection, two fields, overrides at all three
// entity_type levels, refs on one of them, a placement, and an example with a
// value. Returns the override ids, which override.Reader addresses by int64.
type diffxFixture struct {
	baseOverrideID       int64
	modelOverrideID      int64
	collectionOverrideID int64
}

func seedDiffxProject(t *testing.T, pool *pgxpool.Pool) diffxFixture {
	t.Helper()
	ctx := context.Background()
	seedProject(t, pool, diffxProject)

	mustExec(t, pool, `INSERT INTO weave_categories (id, project_id, semantic_id, system_name, ui_name, status, canonical_order)
		VALUES ('DIFFXCAT.1', $1, 'DIFFX.CAT.1', 'first', '{"en":"First"}'::jsonb, 'draft', 1)`, diffxProject)
	mustExec(t, pool, `INSERT INTO weave_categories (id, project_id, semantic_id, system_name, ui_name, status, canonical_order)
		VALUES ('DIFFXCAT.2', $1, 'DIFFX.CAT.2', 'second', '{"en":"Second"}'::jsonb, 'published', 2)`, diffxProject)

	mustExec(t, pool, `INSERT INTO weave_models (id, project_id, system_name, ui_name, status)
		VALUES ('DIFFXM.1', $1, 'a_model', '{"en":"A model"}'::jsonb, 'draft')`, diffxProject)
	mustExec(t, pool, `INSERT INTO weave_collections (id, project_id, system_name, ui_name, status)
		VALUES ('DIFFXC.1', $1, 'a_collection', '{"en":"A collection"}'::jsonb, 'draft')`, diffxProject)

	for _, f := range []struct{ id, name string }{{"DIFFXF.1", "field_one"}, {"DIFFXF.2", "field_two"}} {
		mustExec(t, pool, `INSERT INTO weave_fields (id, project_id, system_name, ui_name, status, path_elements)
			VALUES ($1, $2, $3, '{"en":"A field"}'::jsonb, 'draft', '[]'::jsonb)`, f.id, diffxProject, f.name)
	}

	newOverride := func(fieldID, entityType, entityID, categoryID, displayName string, position int) int64 {
		var id int64
		var cat any
		if categoryID != "" {
			cat = categoryID
		}
		if err := pool.QueryRow(ctx, `INSERT INTO weave_field_overrides
			(field_id, project_id, entity_type, entity_id, category_id, display_name, position, is_required, min_occurs)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, true, 1) RETURNING id`,
			fieldID, diffxProject, entityType, entityID, cat,
			`{"en":"`+displayName+`"}`, position).Scan(&id); err != nil {
			t.Fatalf("seed override %s/%s: %v", entityType, fieldID, err)
		}
		return id
	}

	fx := diffxFixture{
		baseOverrideID:  newOverride("DIFFXF.1", "", "", "", "Base one", 0),
		modelOverrideID: newOverride("DIFFXF.1", "model", "DIFFXM.1", "DIFFXCAT.1", "On the model", 1),
		// The category_id on a COLLECTION override holds the category's
		// SEMANTIC id, not its ULID -- a legacy inconsistency with model
		// overrides, which hold the ULID. Both queries filter on
		// fo.category_id, so each path must be handed the id its own
		// entity_type stores (see store_postgres.go:482).
		collectionOverrideID: newOverride("DIFFXF.2", "collection", "DIFFXC.1", "DIFFX.CAT.2", "On the collection", 2),
	}

	// A BASE override sharing DIFFXCAT.1 with the model override above.
	// Without it the per-entity_type counts in ListWithCounts cannot be told
	// apart: DIFFXCAT.1 would hold exactly one override, so a count that
	// ignored entity_type would return the same number as one that honored
	// it, and the test would pass on a dropped filter. Verified by injecting
	// that exact bug -- it went undetected until this row existed.
	newOverride("DIFFXF.2", "", "", "DIFFXCAT.1", "Base two", 3)

	// Two refs on one override, at different positions: the archive's
	// conflict target is (override_id, ref_type, position, version_number),
	// so two positions prove the archive keeps both rather than collapsing.
	for i, target := range []string{"DIFFXM.1", "DIFFXC.1"} {
		mustExec(t, pool, `INSERT INTO weave_override_refs (override_id, ref_type, target_id, semantic_id, position)
			VALUES ($1, 'value_target', $2, $3, $4)`, fx.baseOverrideID, target, target, i)
	}

	mustExec(t, pool, `INSERT INTO weave_collection_placements
		(project_id, model_id, category_id, collection_id, is_required, min_occurs, max_occurs, is_hidden)
		VALUES ($1, 'DIFFXM.1', 'DIFFXCAT.1', 'DIFFXC.1', true, 1, 3, false)`, diffxProject)

	mustExec(t, pool, `INSERT INTO weave_examples (id, project_id, entity_type, entity_id, title, description, status)
		VALUES ('DIFFXE.1', $1, 'model', 'DIFFXM.1', '{"en":"An example"}'::jsonb, '{"en":"with a value"}'::jsonb, 'draft')`, diffxProject)
	mustExec(t, pool, `INSERT INTO weave_example_values
		(example_id, override_id, field_id, occurrence_index, value_kind, value_payload, text_value)
		VALUES ('DIFFXE.1', $1, 'DIFFXF.1', 0, 'string', '{"value":"hello"}'::jsonb, 'hello')`, fx.baseOverrideID)

	t.Cleanup(func() {
		bg := context.Background()
		for _, stmt := range []string{
			`DELETE FROM weave_example_values_archive WHERE example_id IN (SELECT id FROM weave_examples WHERE project_id=$1)`,
			`DELETE FROM weave_example_values WHERE example_id IN (SELECT id FROM weave_examples WHERE project_id=$1)`,
			`DELETE FROM weave_examples_archive WHERE project_id=$1`,
			`DELETE FROM weave_examples WHERE project_id=$1`,
			`DELETE FROM weave_override_refs_archive WHERE project_id=$1`,
			`DELETE FROM weave_override_refs WHERE override_id IN (SELECT id FROM weave_field_overrides WHERE project_id=$1)`,
			`DELETE FROM weave_collection_placements_archive WHERE project_id=$1`,
			`DELETE FROM weave_collection_placements WHERE project_id=$1`,
			`DELETE FROM weave_field_overrides_archive WHERE project_id=$1`,
			`DELETE FROM weave_field_overrides WHERE project_id=$1`,
			`DELETE FROM weave_collections_archive WHERE project_id=$1`,
			`DELETE FROM weave_collections WHERE project_id=$1`,
			`DELETE FROM weave_models_archive WHERE project_id=$1`,
			`DELETE FROM weave_models WHERE project_id=$1`,
			`DELETE FROM weave_fields_archive WHERE project_id=$1`,
			`DELETE FROM weave_fields WHERE project_id=$1`,
			`DELETE FROM weave_categories_archive WHERE project_id=$1`,
			`DELETE FROM weave_categories WHERE project_id=$1`,
			`DELETE FROM weave_releases WHERE project_id=$1`,
		} {
			if _, err := pool.Exec(bg, stmt, diffxProject); err != nil {
				t.Errorf("cleanup %q: %v", stmt, err)
			}
		}
	})
	return fx
}

// cutDiffx seeds and cuts, returning the fixture.
func cutDiffx(t *testing.T, pool *pgxpool.Pool) diffxFixture {
	t.Helper()
	fx := seedDiffxProject(t, pool)
	if err := runSnapshotStatements(context.Background(), pool, diffxProject, diffxVersion); err != nil {
		t.Fatalf("cut release: %v", err)
	}
	return fx
}

// The override slice. Nothing in its tables is transformed by the cut, so
// every answer must match exactly -- no normalization, no ignored fields.
func TestOverrideReaderAnswersTheSameOnBothPathsAfterARealCut(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	fx := cutDiffx(t, pool)

	store := override.NewPostgresStore(pool)
	draft, err := store.At(auth.Draft())
	if err != nil {
		t.Fatalf("At(Draft): %v", err)
	}
	release, err := store.At(auth.Release(diffxVersion))
	if err != nil {
		t.Fatalf("At(Release): %v", err)
	}

	// Overrides carry no version column in the domain type, so the two
	// answers are compared whole. A difference is a bug, full stop.
	diff := func(t *testing.T, what string, d, r any) {
		t.Helper()
		if delta := cmp.Diff(d, r); delta != "" {
			t.Errorf("%s: draft and release disagree (-draft +release):\n%s\n"+
				"These tables are copied column for column by the cut, so there is nothing "+
				"legitimate to differ here.", what, delta)
		}
	}

	t.Run("ListForEntity", func(t *testing.T) {
		for _, c := range []struct{ entityType, entityID string }{
			{"model", "DIFFXM.1"},
			{"collection", "DIFFXC.1"},
		} {
			d, err := draft.ListForEntity(ctx, c.entityType, c.entityID)
			if err != nil {
				t.Fatalf("draft ListForEntity: %v", err)
			}
			r, err := release.ListForEntity(ctx, c.entityType, c.entityID)
			if err != nil {
				t.Fatalf("release ListForEntity: %v", err)
			}
			if len(d) == 0 {
				t.Fatalf("fixture produced no %s overrides; fix the fixture", c.entityType)
			}
			diff(t, "ListForEntity "+c.entityType, d, r)
		}
	})

	t.Run("ListForField", func(t *testing.T) {
		d, err := draft.ListForField(ctx, "DIFFXF.1")
		if err != nil {
			t.Fatalf("draft ListForField: %v", err)
		}
		r, err := release.ListForField(ctx, "DIFFXF.1")
		if err != nil {
			t.Fatalf("release ListForField: %v", err)
		}
		if len(d) == 0 {
			t.Fatal("fixture produced no overrides for DIFFXF.1; fix the fixture")
		}
		diff(t, "ListForField", d, r)
	})

	t.Run("ListByProjectAndType", func(t *testing.T) {
		for _, et := range []string{"", "model", "collection"} {
			d, err := draft.ListByProjectAndType(ctx, diffxProject, et)
			if err != nil {
				t.Fatalf("draft ListByProjectAndType %q: %v", et, err)
			}
			r, err := release.ListByProjectAndType(ctx, diffxProject, et)
			if err != nil {
				t.Fatalf("release ListByProjectAndType %q: %v", et, err)
			}
			if len(d) == 0 {
				t.Fatalf("fixture produced no overrides of entity_type %q; fix the fixture", et)
			}
			diff(t, "ListByProjectAndType "+et, d, r)
		}
	})

	// Two refs at different positions. The archive conflicts on
	// (override_id, ref_type, position, version_number); a target that
	// ignored position would keep one of the two.
	t.Run("GetRefs", func(t *testing.T) {
		d, err := draft.GetRefs(ctx, fx.baseOverrideID)
		if err != nil {
			t.Fatalf("draft GetRefs: %v", err)
		}
		r, err := release.GetRefs(ctx, fx.baseOverrideID)
		if err != nil {
			t.Fatalf("release GetRefs: %v", err)
		}
		if len(d) != 2 {
			t.Fatalf("fixture produced %d refs, want 2; fix the fixture", len(d))
		}
		diff(t, "GetRefs", d, r)
	})

	t.Run("GetByID", func(t *testing.T) {
		for _, id := range []int64{fx.baseOverrideID, fx.modelOverrideID, fx.collectionOverrideID} {
			d, err := draft.GetByID(ctx, id)
			if err != nil {
				t.Fatalf("draft GetByID %d: %v", id, err)
			}
			r, err := release.GetByID(ctx, id)
			if err != nil {
				t.Fatalf("release GetByID %d: %v", id, err)
			}
			if d == nil || r == nil {
				t.Fatalf("GetByID %d: draft=%v release=%v", id, d != nil, r != nil)
			}
			diff(t, "GetByID", d, r)
		}
	})

	t.Run("GetBase", func(t *testing.T) {
		d, err := draft.GetBase(ctx, "DIFFXF.1", diffxProject)
		if err != nil {
			t.Fatalf("draft GetBase: %v", err)
		}
		r, err := release.GetBase(ctx, "DIFFXF.1", diffxProject)
		if err != nil {
			t.Fatalf("release GetBase: %v", err)
		}
		if d == nil || r == nil {
			t.Fatalf("GetBase: draft=%v release=%v", d != nil, r != nil)
		}
		diff(t, "GetBase", d, r)
	})

	t.Run("ListPlacements", func(t *testing.T) {
		d, err := draft.ListPlacements(ctx, diffxProject, "DIFFXM.1")
		if err != nil {
			t.Fatalf("draft ListPlacements: %v", err)
		}
		r, err := release.ListPlacements(ctx, diffxProject, "DIFFXM.1")
		if err != nil {
			t.Fatalf("release ListPlacements: %v", err)
		}
		if len(d) == 0 {
			t.Fatal("fixture produced no placements; fix the fixture")
		}
		diff(t, "ListPlacements", d, r)
	})
}

// The example slice. Also copied column for column, with one wrinkle: the
// domain type carries VersionNumber, which differs by construction.
func TestExampleReaderAnswersTheSameOnBothPathsAfterARealCut(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	cutDiffx(t, pool)

	store := example.NewPostgresStore(pool)
	draft, err := store.At(auth.Draft())
	if err != nil {
		t.Fatalf("At(Draft): %v", err)
	}
	release, err := store.At(auth.Release(diffxVersion))
	if err != nil {
		t.Fatalf("At(Release): %v", err)
	}

	ignoreVersion := cmpopts.IgnoreFields(domain.Example{}, "VersionNumber")

	t.Run("GetByID", func(t *testing.T) {
		d, err := draft.GetByID(ctx, "DIFFXE.1")
		if err != nil {
			t.Fatalf("draft GetByID: %v", err)
		}
		r, err := release.GetByID(ctx, "DIFFXE.1")
		if err != nil {
			t.Fatalf("release GetByID: %v", err)
		}
		if d == nil || r == nil {
			t.Fatalf("GetByID: draft=%v release=%v", d != nil, r != nil)
		}
		if delta := cmp.Diff(d, r, ignoreVersion); delta != "" {
			t.Errorf("GetByID disagrees (-draft +release):\n%s", delta)
		}
	})

	t.Run("List", func(t *testing.T) {
		d, dTotal, err := draft.List(ctx, domain.WithProjectID(diffxProject))
		if err != nil {
			t.Fatalf("draft List: %v", err)
		}
		r, rTotal, err := release.List(ctx, domain.WithProjectID(diffxProject))
		if err != nil {
			t.Fatalf("release List: %v", err)
		}
		if len(d) == 0 {
			t.Fatal("fixture produced no examples; fix the fixture")
		}
		if dTotal != rTotal {
			t.Errorf("totals differ: draft %d, release %d", dTotal, rTotal)
		}
		if delta := cmp.Diff(d, r, ignoreVersion); delta != "" {
			t.Errorf("List disagrees (-draft +release):\n%s", delta)
		}
	})

	// The other half of the contract, and the one a draft/release diff cannot
	// state: after the cut, changing the DRAFT must not change what the
	// release answers. The example slice builds its archive reads by
	// substituting the table name rather than writing a second query, so the
	// mirror-divergence class is nearly unrepresentable here -- but a release
	// reader that fell back to the live table would still pass every
	// equivalence check above, because before any mutation the two tables
	// hold the same rows. This is what catches that.
	t.Run("the release answer does not move when the draft changes", func(t *testing.T) {
		before, err := release.GetByID(ctx, "DIFFXE.1")
		if err != nil {
			t.Fatalf("release GetByID before: %v", err)
		}
		mustExec(t, pool, `UPDATE weave_examples SET title = '{"en":"Retitled after the cut"}'::jsonb
			WHERE id = 'DIFFXE.1'`)

		d, err := draft.GetByID(ctx, "DIFFXE.1")
		if err != nil {
			t.Fatalf("draft GetByID after: %v", err)
		}
		if d.Title.Get("en") != "Retitled after the cut" {
			t.Fatalf("the draft did not pick up the update (%q), so this subtest proves nothing", d.Title.Get("en"))
		}

		after, err := release.GetByID(ctx, "DIFFXE.1")
		if err != nil {
			t.Fatalf("release GetByID after: %v", err)
		}
		if after.Title.Get("en") != before.Title.Get("en") {
			t.Errorf("the release answer changed when the draft changed: %q then %q.\n"+
				"The release path is reading the live table.", before.Title.Get("en"), after.Title.Get("en"))
		}
	})

	t.Run("ListValues", func(t *testing.T) {
		d, err := draft.ListValues(ctx, "DIFFXE.1")
		if err != nil {
			t.Fatalf("draft ListValues: %v", err)
		}
		r, err := release.ListValues(ctx, "DIFFXE.1")
		if err != nil {
			t.Fatalf("release ListValues: %v", err)
		}
		if len(d) == 0 {
			t.Fatal("fixture produced no example values; fix the fixture")
		}
		if delta := cmp.Diff(d, r); delta != "" {
			t.Errorf("ListValues disagrees (-draft +release):\n%s\n"+
				"weave_example_values is copied column for column by the cut.", delta)
		}
	})
}

// The category slice. Needs the same two normalizations collection does,
// because weave_categories is one of the five tables whose status the cut
// rewrites.
func TestCategoryReaderAnswersTheSameOnBothPathsAfterARealCut(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	cutDiffx(t, pool)

	store := category.NewPostgresStore(pool)
	draft, err := store.At(auth.Draft())
	if err != nil {
		t.Fatalf("At(Draft): %v", err)
	}
	release, err := store.At(auth.Release(diffxVersion))
	if err != nil {
		t.Fatalf("At(Release): %v", err)
	}

	// Status is rewritten by the cut and VersionNumber is added by it. Both
	// are checked explicitly below rather than ignored silently.
	normalized := cmpopts.IgnoreFields(domain.Entity{}, "Status", "VersionNumber")

	t.Run("GetByID", func(t *testing.T) {
		for _, id := range []string{"DIFFXCAT.1", "DIFFXCAT.2"} {
			d, err := draft.GetByID(ctx, diffxProject, id)
			if err != nil {
				t.Fatalf("draft GetByID %s: %v", id, err)
			}
			r, err := release.GetByID(ctx, diffxProject, id)
			if err != nil {
				t.Fatalf("release GetByID %s: %v", id, err)
			}
			if d == nil || r == nil {
				t.Fatalf("GetByID %s: draft=%v release=%v", id, d != nil, r != nil)
			}
			if delta := cmp.Diff(d, r, normalized); delta != "" {
				t.Errorf("GetByID %s disagrees beyond status and version (-draft +release):\n%s", id, delta)
			}
			if want := archivedStatus(d.Status); r.Status != want {
				t.Errorf("GetByID %s: release status %q, want %q (the cut's rewrite of %q)", id, r.Status, want, d.Status)
			}
		}
	})

	t.Run("GetByIdentifier", func(t *testing.T) {
		d, err := draft.GetByIdentifier(ctx, diffxProject, "DIFFX.CAT.1")
		if err != nil {
			t.Fatalf("draft GetByIdentifier: %v", err)
		}
		r, err := release.GetByIdentifier(ctx, diffxProject, "DIFFX.CAT.1")
		if err != nil {
			t.Fatalf("release GetByIdentifier: %v", err)
		}
		if d == nil || r == nil {
			t.Fatalf("GetByIdentifier: draft=%v release=%v", d != nil, r != nil)
		}
		if delta := cmp.Diff(d, r, normalized); delta != "" {
			t.Errorf("GetByIdentifier disagrees beyond status and version (-draft +release):\n%s", delta)
		}
	})

	t.Run("List and Count", func(t *testing.T) {
		d, err := draft.List(ctx, diffxProject)
		if err != nil {
			t.Fatalf("draft List: %v", err)
		}
		r, err := release.List(ctx, diffxProject)
		if err != nil {
			t.Fatalf("release List: %v", err)
		}
		if len(d) == 0 {
			t.Fatal("fixture produced no categories; fix the fixture")
		}
		if delta := cmp.Diff(d, r, normalized); delta != "" {
			t.Errorf("List disagrees beyond status and version (-draft +release):\n%s", delta)
		}
		dc, err := draft.Count(ctx, diffxProject)
		if err != nil {
			t.Fatalf("draft Count: %v", err)
		}
		rc, err := release.Count(ctx, diffxProject)
		if err != nil {
			t.Fatalf("release Count: %v", err)
		}
		if dc != rc {
			t.Errorf("Count differs: draft %d, release %d", dc, rc)
		}
	})

	t.Run("ListWithCounts", func(t *testing.T) {
		d, err := draft.ListWithCounts(ctx, diffxProject)
		if err != nil {
			t.Fatalf("draft ListWithCounts: %v", err)
		}
		r, err := release.ListWithCounts(ctx, diffxProject)
		if err != nil {
			t.Fatalf("release ListWithCounts: %v", err)
		}
		if len(d) == 0 {
			t.Fatal("fixture produced no categories with counts; fix the fixture")
		}
		if delta := cmp.Diff(d, r, normalized); delta != "" {
			t.Errorf("ListWithCounts disagrees beyond status and version (-draft +release):\n%s\n"+
				"A count computed from a different set of overrides on the two paths shows up here.", delta)
		}
	})

	t.Run("ModelFieldOverrides", func(t *testing.T) {
		d, err := draft.ModelFieldOverrides(ctx, diffxProject, "DIFFXCAT.1")
		if err != nil {
			t.Fatalf("draft ModelFieldOverrides: %v", err)
		}
		r, err := release.ModelFieldOverrides(ctx, diffxProject, "DIFFXCAT.1")
		if err != nil {
			t.Fatalf("release ModelFieldOverrides: %v", err)
		}
		if len(d) == 0 {
			t.Fatal("fixture produced no model field overrides in DIFFXCAT.1; fix the fixture")
		}
		if delta := cmp.Diff(d, r); delta != "" {
			t.Errorf("ModelFieldOverrides disagrees (-draft +release):\n%s", delta)
		}
	})

	t.Run("CollectionFieldOverrides", func(t *testing.T) {
		d, err := draft.CollectionFieldOverrides(ctx, diffxProject, "DIFFX.CAT.2")
		if err != nil {
			t.Fatalf("draft CollectionFieldOverrides: %v", err)
		}
		r, err := release.CollectionFieldOverrides(ctx, diffxProject, "DIFFX.CAT.2")
		if err != nil {
			t.Fatalf("release CollectionFieldOverrides: %v", err)
		}
		if len(d) == 0 {
			t.Fatal("fixture produced no collection field overrides in DIFFX.CAT.2; fix the fixture")
		}
		if delta := cmp.Diff(d, r); delta != "" {
			t.Errorf("CollectionFieldOverrides disagrees (-draft +release):\n%s", delta)
		}
	})

	t.Run("IsInUse", func(t *testing.T) {
		for _, c := range []struct{ id, semanticID string }{
			{"DIFFXCAT.1", "DIFFX.CAT.1"},
			{"DIFFXCAT.2", "DIFFX.CAT.2"},
		} {
			d, err := draft.IsInUse(ctx, diffxProject, c.id, c.semanticID)
			if err != nil {
				t.Fatalf("draft IsInUse %s: %v", c.id, err)
			}
			r, err := release.IsInUse(ctx, diffxProject, c.id, c.semanticID)
			if err != nil {
				t.Fatalf("release IsInUse %s: %v", c.id, err)
			}
			if d != r {
				t.Errorf("IsInUse %s: draft %v, release %v -- the two paths disagree on whether the category is used", c.id, d, r)
			}
		}
	})
}
