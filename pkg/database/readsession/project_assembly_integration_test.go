//go:build integration

package readsession_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/service/projectread"
)

func TestProjectAssemblerKeepsNestedSourcesPinnedAndAuthorized(t *testing.T) {
	a, b, c := newFixture(t, "ASSEMBLY_A"), newFixture(t, "ASSEMBLY_B"), newFixture(t, "ASSEMBLY_C")
	c.cut(t)
	referTo(t, b, c)
	b.cut(t)
	referTo(t, a, b)
	a.cut(t)
	tr := &ontologyTrace{pids: map[uint32]bool{}}
	cfg := a.pool.Config()
	cfg.MaxConns, cfg.MinConns = 1, 0
	cfg.ConnConfig.Tracer = tr
	pool, err := pgxpool.NewWithConfig(a.ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	assembler, err := projectread.New(projectread.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	read := func(factory *readsession.Factory) (projectsnapshot.CompleteProject, error) {
		var value projectsnapshot.CompleteProject
		err := factory.Read(ctx, auth.Release("1.0.0"), a.project, func(s *readsession.Session) error { var err error; value, err = assembler.Project(ctx, s); return err })
		return value, err
	}
	factory := readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead)
	before, err := read(factory)
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.pids) != 1 {
		t.Fatal("source opened another connection")
	}
	ids, err := before.Projects()
	if err != nil || len(ids) != 3 {
		t.Fatalf("missing closure: %v", err)
	}
	if _, err := b.pool.Exec(ctx, `UPDATE weave_fields SET ui_name='{"en":"new source draft"}' WHERE project_id=$1`, b.project); err != nil {
		t.Fatal(err)
	}
	if err := cutVersion(t, b, "2.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.pool.Exec(ctx, `DELETE FROM weave_collections WHERE project_id=$1`, c.project); err != nil {
		t.Fatal(err)
	}
	after, err := read(factory)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		want, err := before.Content(id)
		if err != nil {
			t.Fatal(err)
		}
		got, err := after.Content(id)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatal(diff)
		}
	}

	readEntity := func(factory *readsession.Factory) (projectsnapshot.EntityFragment, error) {
		var value projectsnapshot.EntityFragment
		err := factory.Read(ctx, auth.Release("1.0.0"), a.project, func(s *readsession.Session) error {
			var e error
			value, e = assembler.Entity(ctx, s, "collection", a.collection)
			return e
		})
		return value, err
	}
	entity, err := readEntity(factory)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := before.Entity("collection", a.collection)
	if err != nil {
		t.Fatal(err)
	}
	selected, _ := expected.Projects()
	for _, id := range selected {
		want, _ := expected.Content(id)
		got, err := entity.Content(id)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatal(diff)
		}
	}
	if _, err := entity.Compose(); err != nil {
		t.Fatal(err)
	}
	denied := errors.New("source access revoked")
	inaccessible, err := read(readsession.NewFactory(pool).WithSourceAuthorization(func(_ context.Context, r auth.Resource) error { return denied }))
	if !errors.Is(err, denied) {
		t.Fatalf("source authorization was bypassed: %v", err)
	}
	if _, err := inaccessible.Root(); err == nil {
		t.Fatal("unauthorized partial graph returned")
	}
	inaccessibleEntity, err := readEntity(readsession.NewFactory(pool).WithSourceAuthorization(func(_ context.Context, _ auth.Resource) error { return denied }))
	if !errors.Is(err, denied) {
		t.Fatal("entity bypassed source authorization")
	}
	if _, err := inaccessibleEntity.Root(); err == nil {
		t.Fatal("unauthorized entity graph returned")
	}

}

// A chain discovered in a different order from SQL ordering must produce the
// same detached fragment, including pagination and references back into the chain.
func TestEntityDiscoveryPreservesOrderedClosureAndBudgets(t *testing.T) {
	f := newFixture(t, "ASSEMBLY_CHAIN")
	ids := []string{f.collection, f.project + ".z", f.project + ".a", f.project + ".10", f.project + ".2"}
	for _, id := range ids[1:] {
		if _, err := f.pool.Exec(f.ctx, `INSERT INTO weave_collections(id,project_id) VALUES ($1,$2)`, id, f.project); err != nil {
			t.Fatal(err)
		}
	}
	for i, id := range ids[1:] {
		var override int64
		if err := f.pool.QueryRow(f.ctx, `INSERT INTO weave_field_overrides(field_id,project_id,entity_type,entity_id) VALUES ($1,$2,'collection',$3) RETURNING id`, f.field, f.project, id).Scan(&override); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(f.ctx, `INSERT INTO weave_override_refs(override_id,ref_type,target_id,semantic_id,position) VALUES ($1,'collection_model',$2,$2,0)`, override, ids[(i+2)%len(ids)]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE weave_override_refs SET target_id=$1,semantic_id=$1 WHERE override_id=$2`, ids[1], f.override); err != nil {
		t.Fatal(err)
	}
	f.cut(t)
	tr := &ontologyTrace{pids: map[uint32]bool{}}
	cfg := f.pool.Config()
	cfg.ConnConfig.Tracer = tr
	pool, err := pgxpool.NewWithConfig(f.ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	factory := readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead)
	options := projectread.DefaultOptions()
	options.Limits.PageSize = 2
	assembler, err := projectread.New(options)
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []auth.ReadScope{auth.Draft(), auth.Release("1.0.0")} {
		err := factory.Read(f.ctx, scope, f.project, func(s *readsession.Session) error {
			complete, err := assembler.Project(f.ctx, s)
			if err != nil {
				return err
			}
			want, err := complete.Entity("collection", f.collection)
			if err != nil {
				return err
			}
			tr.queries = nil
			got, err := assembler.Entity(f.ctx, s, "collection", f.collection)
			if err != nil {
				return err
			}
			metadataReads := 0
			for _, query := range tr.queries {
				if strings.HasPrefix(query, "-- name: ContentProjects ") {
					metadataReads++
				}
			}
			if metadataReads != 1 {
				t.Fatalf("metadata loaded %d times during entity assembly", metadataReads)
			}
			identity, _ := want.Root()
			wantContent, _ := want.Content(identity.Source)
			gotContent, err := got.Content(identity.Source)
			if err != nil {
				return err
			}
			if len(gotContent.Collections) != len(ids) {
				t.Fatal("incomplete chain")
			}
			if diff := cmp.Diff(wantContent, gotContent); diff != "" {
				t.Fatal(diff)
			}
			for _, resource := range []string{"rows", "bytes"} {
				limited := options
				if resource == "rows" {
					limited.Limits.MaxRows = 1
				} else {
					limited.Limits.MaxBytes = 1
				}
				reader, err := projectread.New(limited)
				if err != nil {
					return err
				}
				partial, err := reader.Entity(f.ctx, s, "collection", f.collection)
				var budget *projectread.BudgetError
				if !errors.As(err, &budget) || budget.Resource != resource {
					t.Fatalf("%s budget bypassed: %v", resource, err)
				}
				if _, err := partial.Root(); err == nil {
					t.Fatal("partial fragment returned after budget failure")
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
