//go:build integration

package readsession_test

import (
	"context"
	"errors"
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
	denied := errors.New("source access revoked")
	inaccessible, err := read(readsession.NewFactory(pool).WithSourceAuthorization(func(_ context.Context, r auth.Resource) error { return denied }))
	if !errors.Is(err, denied) {
		t.Fatalf("source authorization was bypassed: %v", err)
	}
	if _, err := inaccessible.Root(); err == nil {
		t.Fatal("unauthorized partial graph returned")
	}
}
