//go:build integration

package readsession_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/collection"
	"github.com/pletka-io/pletka/pkg/weave/override"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

func TestMain(m *testing.M) { os.Exit(testdb.Setup(m)) }

type fixture struct {
	pool                       *pgxpool.Pool
	ctx                        context.Context
	factory                    *readsession.Factory
	project, collection, field string
	override                   int64
}

func newFixture(t *testing.T, project string) fixture {
	t.Helper()
	pool := testdb.Pool(t)
	ctx := auth.WithSnapshot(context.Background(), &auth.AuthSnapshot{IsSuperAdmin: true})
	ctx = auth.WithPrincipal(ctx, &auth.Principal{ActorID: "unite"})
	f := fixture{pool: pool, ctx: ctx, factory: readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead), project: project, collection: project + ".c", field: project + ".f"}
	for _, q := range []string{
		`INSERT INTO weave_projects(id,owner_id) VALUES ($1,'unite')`,
		`INSERT INTO weave_collections(id,project_id,system_name) VALUES ($1||'.c',$1,'before')`,
		`INSERT INTO weave_fields(id,project_id,system_name) VALUES ($1||'.f',$1,'field')`,
	} {
		if _, err := pool.Exec(ctx, q, project); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, `INSERT INTO weave_field_overrides(field_id,project_id,entity_type,entity_id,display_name) VALUES ($1,$2,'collection',$3,'{"en":"before"}') RETURNING id`, f.field, project, f.collection).Scan(&f.override); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_override_refs(override_id,ref_type,target_id,semantic_id,position) VALUES ($1,'collection_model',$2,$2,0)`, f.override, f.collection); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f fixture) cut(t *testing.T) {
	t.Helper()
	_, err := release.NewService(release.NewPostgresStore(f.pool), f.pool, nil).Create(f.ctx, f.project, release.CreateInput{Version: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReleasedCollectionAndOverridesSurviveLiveDeletion(t *testing.T) {
	f := newFixture(t, "RSP_DELETE")
	f.cut(t)
	store := collection.NewPostgresStore(f.pool)
	if err := store.Delete(f.ctx, f.collection); err != nil {
		t.Fatal(err)
	}
	service := collection.NewService(store, collection.NewReadSessions(f.factory), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	c, err := service.Get(f.ctx, auth.Release("1.0.0"), f.project, f.collection)
	if err != nil || c == nil || c.SystemName != "before" {
		t.Fatalf("archived body: %+v, %v", c, err)
	}
	rows, err := service.ListOverrides(f.ctx, auth.Release("1.0.0"), f.project, f.collection)
	if err != nil || len(rows) != 1 || rows[0].DisplayName["en"] != "before" {
		t.Fatalf("archived overrides: %+v, %v", rows, err)
	}
	_, err = service.Get(f.ctx, auth.Release("1.0.0"), f.project, "absent")
	if got := apierror.FromError(err).Code; got != apierror.CodeNotInRelease {
		t.Fatalf("missing collection code=%s err=%v", got, err)
	}
	refs, err := override.NewService(override.NewPostgresStore(f.pool), override.NewReadSessions(f.factory), nil, nil).GetRefs(f.ctx, auth.Release("1.0.0"), f.project, f.override)
	if err != nil || len(refs) != 1 {
		t.Fatalf("archived refs: %+v, %v", refs, err)
	}
}

func TestBoundReadersCannotReadAnotherProjectsIDs(t *testing.T) {
	a, b := newFixture(t, "RSP_A"), newFixture(t, "RSP_B")
	a.cut(t)
	b.cut(t)
	for _, scope := range []auth.ReadScope{auth.Draft(), auth.Release("1.0.0")} {
		err := a.factory.Read(a.ctx, scope, a.project, func(s *readsession.Session) error {
			c, err := collection.BindReader(s)
			if err != nil {
				return err
			}
			got, err := c.Get(a.ctx, b.collection)
			if got != nil || (scope.IsRelease() && apierror.FromError(err).Code != apierror.CodeNotInRelease) {
				t.Fatalf("foreign body: %+v, %v", got, err)
			}
			if _, _, err := c.List(a.ctx, domain.WithProjectID(b.project)); err == nil {
				t.Fatal("query options retargeted project")
			}
			if _, _, err := c.List(a.ctx, domain.WithVersion("9.0.0")); err == nil {
				t.Fatal("query options retargeted version")
			}
			counts, err := c.CompositionCounts(a.ctx, []string{b.collection})
			if err != nil {
				return err
			}
			if len(counts) != 0 {
				t.Fatalf("foreign counts: %v", counts)
			}
			o, err := override.BindReader(s)
			if err != nil {
				return err
			}
			rows, err := o.ListForEntity(a.ctx, "collection", b.collection)
			if err != nil {
				return err
			}
			if len(rows) != 0 {
				t.Fatalf("foreign overrides: %v", rows)
			}
			refs, err := o.GetRefs(a.ctx, b.override)
			if err == nil || len(refs) != 0 {
				t.Fatalf("foreign refs: %v, %v", refs, err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestCollectionUsageKeepsCallerMembershipWithoutLiveForeignLabels(t *testing.T) {
	a, b := newFixture(t, "RSP_USAGE_A"), newFixture(t, "RSP_USAGE_B")
	b.cut(t)
	// A uses B's field. B also references A's collection, but B's usage is not
	// part of A's content. Both publish the same textual version deliberately.
	if _, err := a.pool.Exec(a.ctx, `UPDATE weave_field_overrides SET field_id=$1 WHERE id=$2`, b.field, a.override); err != nil {
		t.Fatal(err)
	}
	if _, err := b.pool.Exec(b.ctx, `UPDATE weave_override_refs SET target_id=$1 WHERE override_id=$2`, a.collection, b.override); err != nil {
		t.Fatal(err)
	}
	a.cut(t)
	service := collection.NewService(collection.NewPostgresStore(a.pool), collection.NewReadSessions(a.factory), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	check := func(scope auth.ReadScope, wantCount int64) {
		t.Helper()
		stats, err := service.Stats(a.ctx, scope, a.project, a.collection)
		if err != nil {
			t.Fatal(err)
		}
		if stats.Usage.FieldCount != wantCount || int64(len(stats.Usage.FieldSamples)) != wantCount {
			t.Fatalf("%s caller usage: %+v", scope, stats.Usage)
		}
		if wantCount != 0 && stats.Usage.FieldSamples[0].FieldName != b.field {
			t.Fatalf("%s resolved an unpinned foreign label: %+v", scope, stats.Usage)
		}
	}
	check(auth.Draft(), 1)
	check(auth.Release("1.0.0"), 1)
	if _, err := b.pool.Exec(b.ctx, `UPDATE weave_fields SET system_name='changed foreign draft' WHERE id=$1`, b.field); err != nil {
		t.Fatal(err)
	}
	if _, err := a.pool.Exec(a.ctx, `DELETE FROM weave_field_overrides WHERE id=$1`, a.override); err != nil {
		t.Fatal(err)
	}
	check(auth.Draft(), 0)
	check(auth.Release("1.0.0"), 1)
}

func TestSessionSharesOneSnapshotAndClosesEscapedReaders(t *testing.T) {
	f := newFixture(t, "RSP_SNAPSHOT")
	var escaped collection.Reader
	var session *readsession.Session
	err := f.factory.Read(f.ctx, auth.Draft(), f.project, func(s *readsession.Session) error {
		session = s
		c, err := collection.BindReader(s)
		if err != nil {
			return err
		}
		escaped = c
		before, err := c.Get(f.ctx, f.collection)
		if err != nil {
			return err
		}
		if _, err := f.pool.Exec(f.ctx, `UPDATE weave_collections SET system_name='after' WHERE id=$1`, f.collection); err != nil {
			return err
		}
		if _, err := f.pool.Exec(f.ctx, `UPDATE weave_field_overrides SET display_name='{"en":"after"}' WHERE id=$1`, f.override); err != nil {
			return err
		}
		after, err := c.Get(f.ctx, f.collection)
		if err != nil {
			return err
		}
		o, err := override.BindReader(s)
		if err != nil {
			return err
		}
		rows, err := o.ListForEntity(f.ctx, "collection", f.collection)
		if err != nil {
			return err
		}
		if before.SystemName != "before" || after.SystemName != "before" || len(rows) != 1 || rows[0].DisplayName["en"] != "before" {
			return fmt.Errorf("mixed snapshot: %+v %+v %+v", before, after, rows)
		}
		if _, err := s.ArchiveQueries(); err == nil {
			return errors.New("draft session exposed archive capability")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := escaped.Get(f.ctx, f.collection); !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatalf("escaped reader: %v", err)
	}
	if _, err := session.DraftQueries(); err == nil {
		t.Fatal("closed session exposed queries")
	}
	err = f.factory.Read(f.ctx, auth.Draft(), f.project, func(s *readsession.Session) error {
		c, err := collection.BindReader(s)
		if err != nil {
			return err
		}
		row, err := c.Get(f.ctx, f.collection)
		if err != nil {
			return err
		}
		if row.SystemName != "after" {
			return fmt.Errorf("new session saw %q", row.SystemName)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSessionRejectsInvalidSelectionsAndReleasesConnections(t *testing.T) {
	f := newFixture(t, "RSP_LIFETIME")
	f.cut(t)
	for _, scope := range []auth.ReadScope{{}, auth.Release(""), auth.Release(" "), auth.Release("9.9.9")} {
		called := false
		err := f.factory.Read(f.ctx, scope, f.project, func(*readsession.Session) error { called = true; return nil })
		if err == nil || called {
			t.Fatalf("invalid selection %v reached callback: %v", scope, err)
		}
	}
	if err := f.factory.Read(f.ctx, auth.Draft(), "", func(*readsession.Session) error { return nil }); err == nil {
		t.Fatal("empty project accepted")
	}
	err := f.factory.Read(f.ctx, auth.Release("1.0.0"), f.project, func(s *readsession.Session) error {
		if _, err := s.DraftQueries(); err == nil {
			t.Fatal("release session exposed hot queries")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	config := f.pool.Config()
	config.MaxConns = 1
	config.MinConns = 0
	pool, err := pgxpool.NewWithConfig(f.ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	factory := readsession.NewFactory(pool)
	canceled, cancel := context.WithCancel(f.ctx)
	sentinel := errors.New("callback failed")
	if err := factory.Read(canceled, auth.Draft(), f.project, func(*readsession.Session) error { cancel(); return sentinel }); !errors.Is(err, sentinel) {
		t.Fatalf("callback cause lost: %v", err)
	}
	ctx, stop := context.WithTimeout(f.ctx, 2*time.Second)
	defer stop()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("session leaked only pool connection: %v", err)
	}
	if err := factory.Read(ctx, auth.Draft(), f.project, func(*readsession.Session) error { return nil }); err != nil {
		t.Fatal(err)
	}
}
