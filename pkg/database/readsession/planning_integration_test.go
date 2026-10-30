//go:build integration

package readsession_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/collection"
)

func planSources(ctx context.Context, t *testing.T, f fixture, scope auth.ReadScope) ([]readsession.Source, error) {
	t.Helper()
	var sources []readsession.Source
	err := f.factory.Read(ctx, scope, f.project, func(s *readsession.Session) error { var err error; sources, err = s.Dependencies(ctx); return err })
	return sources, err
}
func adoptSource(t *testing.T, consumer, source fixture, version string) {
	t.Helper()
	if _, err := consumer.pool.Exec(consumer.ctx, `INSERT INTO weave_adoptions(project_id,context_entity_type,context_entity_id,entity_type,source_project_id,source_entity_id,source_version) VALUES ($1,'project',$1,'collection',$2,$3,$4)`, consumer.project, source.project, source.collection, version); err != nil {
		t.Fatal(err)
	}
}

func TestDraftPlanSharesSnapshotCachesSourcesAndMatchesCapture(t *testing.T) {
	a, b, c := newFixture(t, "PLAN_A"), newFixture(t, "PLAN_B"), newFixture(t, "PLAN_C")
	c.cut(t)
	referTo(t, b, c)
	b.cut(t)
	if err := cutVersion(t, c, "2.0.0"); err != nil {
		t.Fatal(err)
	}
	referTo(t, a, b)
	adoptSource(t, a, b, "")
	tr := &ontologyTrace{pids: map[uint32]bool{}}
	config := a.pool.Config()
	config.MaxConns, config.MinConns = 1, 0
	config.ConnConfig.Tracer = tr
	pool, err := pgxpool.NewWithConfig(a.ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	factory := readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead)
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	want := []readsession.Source{{ProjectID: b.project, Version: "1.0.0"}, {ProjectID: c.project, Version: "1.0.0"}}
	var escaped *readsession.Session
	err = factory.Read(ctx, auth.Draft(), a.project, func(s *readsession.Session) error {
		first, err := s.Dependencies(ctx)
		if err != nil {
			return err
		}
		if diff := cmp.Diff(want, first); diff != "" {
			t.Fatal(diff)
		}
		first[0].Version = "changed by caller"
		bs, err := s.ProjectDependency(ctx, b.project)
		if err != nil {
			return err
		}
		bsAgain, err := s.Dependency(ctx, "collection", b.collection)
		if err != nil {
			return err
		}
		if bs != bsAgain {
			t.Fatal("source was rebound instead of cached by concrete identity")
		}
		cs, err := bs.Dependency(ctx, "collection", c.collection)
		if err != nil {
			return err
		}
		directC, err := s.ProjectDependency(ctx, c.project)
		if err != nil {
			return err
		}
		if cs != directC {
			t.Fatal("nested source did not share root source cache")
		}
		escaped = cs
		if _, err := s.ProjectDependency(ctx, "unrecorded"); apierror.FromError(err).Code != apierror.CodeNotInRelease {
			t.Fatalf("unrecorded project: %v", err)
		}
		if _, err := s.Dependency(ctx, "collection", a.collection); apierror.FromError(err).Code != apierror.CodeNotInRelease {
			t.Fatalf("own content became foreign: %v", err)
		}
		if _, err := b.pool.Exec(b.ctx, `UPDATE weave_collections SET system_name='later' WHERE id=$1`, b.collection); err != nil {
			return err
		}
		if err := cutVersion(t, b, "2.0.0"); err != nil {
			return err
		}
		second, err := s.Dependencies(ctx)
		if err != nil {
			return err
		}
		if diff := cmp.Diff(want, second); diff != "" {
			t.Fatal(diff)
		}
		reader, err := collection.BindReader(bs)
		if err != nil {
			return err
		}
		row, err := reader.Get(ctx, b.collection)
		if err != nil {
			return err
		}
		if row.SystemName != "before" {
			t.Fatal("new source release leaked into selected plan")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := escaped.Dependencies(a.ctx); err == nil {
		t.Fatal("source plan outlived root")
	}
	tr.mu.Lock()
	begins, latest, refs := 0, 0, 0
	for _, sql := range tr.queries {
		if strings.HasPrefix(sql, "begin") {
			begins++
			if !strings.Contains(sql, "read only") {
				t.Fatal("planning transaction is writable")
			}
		}
		if strings.Contains(sql, "WeaveLatestDependencyRelease") {
			latest++
		}
		if strings.Contains(sql, "WeaveDraftDependencyReferences") {
			refs++
		}
	}
	backends := len(tr.pids)
	tr.mu.Unlock()
	if backends != 1 || begins != 1 || latest != 1 || refs != 1 {
		t.Fatalf("session snapshot/selection counts: backends=%d begins=%d latest=%d refs=%d", backends, begins, latest, refs)
	}
	var writes int
	if err := a.pool.QueryRow(a.ctx, `SELECT (SELECT count(*) FROM weave_releases WHERE project_id=$1)+(SELECT count(*) FROM weave_release_project_dependencies WHERE project_id=$1)+(SELECT count(*) FROM weave_release_dependency_entities WHERE project_id=$1)`, a.project).Scan(&writes); err != nil || writes != 0 {
		t.Fatalf("planning persisted evidence: %d %v", writes, err)
	}
	var pin string
	if err := a.pool.QueryRow(a.ctx, `SELECT source_version FROM weave_adoptions WHERE project_id=$1`, a.project).Scan(&pin); err != nil || pin != "" {
		t.Fatalf("planning edited adoption: %q %v", pin, err)
	}
	fresh, err := planSources(a.ctx, t, a, auth.Draft())
	if err != nil {
		t.Fatal(err)
	}
	want[0].Version, want[1].Version = "2.0.0", "2.0.0"
	if diff := cmp.Diff(want, fresh); diff != "" {
		t.Fatal(diff)
	}
	a.cut(t)
	captured, err := planSources(a.ctx, t, a, auth.Release("1.0.0"))
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(fresh, captured); diff != "" {
		t.Fatalf("capture diverges from draft planning: %s", diff)
	}
}

func TestDraftPlanningFailsClosedOnUnavailableOrConflictingSources(t *testing.T) {
	cases := []struct {
		name, contains string
		setup          func(*testing.T, fixture, fixture)
	}{
		{"unpublished", "no published release", func(t *testing.T, a, b fixture) { referTo(t, a, b) }},
		{"unverified latest", "no verified project dependency inventory", func(t *testing.T, a, b fixture) {
			b.cut(t)
			referTo(t, a, b)
			if _, err := b.pool.Exec(b.ctx, `INSERT INTO weave_releases(project_id,version,created_by_id) VALUES ($1,'2.0.0','unite')`, b.project); err != nil {
				t.Fatal(err)
			}
		}},
		{"conflicting explicit pins", "conflicting release pins", func(t *testing.T, a, b fixture) {
			b.cut(t)
			if err := cutVersion(t, b, "2.0.0"); err != nil {
				t.Fatal(err)
			}
			adoptSource(t, a, b, "1.0.0")
			adoptSource(t, a, b, "2.0.0")
		}},
		{"explicit follow draft", "explicitly follows source drafts", func(t *testing.T, a, b fixture) {
			b.cut(t)
			if _, err := a.pool.Exec(a.ctx, `INSERT INTO weave_project_inheritance(project_id,parent_project_id,source_mode) VALUES ($1,$2,'draft')`, a.project, b.project); err != nil {
				t.Fatal(err)
			}
		}},
		{"source cycle", "cycle", func(t *testing.T, a, b fixture) { a.cut(t); referTo(t, b, a); b.cut(t); referTo(t, a, b) }},
		{"missing in pinned release", "not in source release", func(t *testing.T, a, b fixture) {
			b.cut(t)
			extra := b
			extra.collection = b.project + ".extra"
			if _, err := b.pool.Exec(b.ctx, `INSERT INTO weave_collections(id,project_id) VALUES ($1,$2)`, extra.collection, b.project); err != nil {
				t.Fatal(err)
			}
			adoptSource(t, a, extra, "1.0.0")
		}},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, b := newFixture(t, fmt.Sprintf("PLAN_FAIL_%d_A", i)), newFixture(t, fmt.Sprintf("PLAN_FAIL_%d_B", i))
			tc.setup(t, a, b)
			sources, err := planSources(a.ctx, t, a, auth.Draft())
			if err == nil || !strings.Contains(err.Error(), tc.contains) || len(sources) != 0 {
				t.Fatalf("partial or unexpected plan: %v %v", sources, err)
			}
		})
	}
}

func TestDraftExplicitPinCanReadDeletedLiveSource(t *testing.T) {
	a, b := newFixture(t, "PLAN_GONE_A"), newFixture(t, "PLAN_GONE_B")
	b.cut(t)
	adoptSource(t, a, b, "1.0.0")
	if err := collection.NewPostgresStore(b.pool).Delete(b.ctx, b.collection); err != nil {
		t.Fatal(err)
	}
	if err := a.factory.Read(a.ctx, auth.Draft(), a.project, func(s *readsession.Session) error {
		source, err := s.Dependency(a.ctx, "collection", b.collection)
		if err != nil {
			return err
		}
		reader, err := collection.BindReader(source)
		if err != nil {
			return err
		}
		row, err := reader.Get(a.ctx, b.collection)
		if err != nil {
			return err
		}
		if row.SystemName != "before" {
			t.Fatal("archived source changed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSourceAuthorizationIsRequiredAndChecksCurrentTransitivePolicy(t *testing.T) {
	a, b, c := newFixture(t, "PLAN_AUTH_A"), newFixture(t, "PLAN_AUTH_B"), newFixture(t, "PLAN_AUTH_C")
	for _, f := range []fixture{a, b, c} {
		if _, err := f.pool.Exec(f.ctx, `UPDATE weave_projects SET visibility='public' WHERE id=$1`, f.project); err != nil {
			t.Fatal(err)
		}
	}
	c.cut(t)
	referTo(t, b, c)
	b.cut(t)
	referTo(t, a, b)
	a.cut(t)
	// The release carries public visibility; current access policy is private.
	if _, err := c.pool.Exec(c.ctx, `UPDATE weave_projects SET visibility='private' WHERE id=$1`, c.project); err != nil {
		t.Fatal(err)
	}
	anonymous := context.Background()
	for _, scope := range []auth.ReadScope{auth.Draft(), auth.Release("1.0.0")} {
		sources, err := planSources(anonymous, t, a, scope)
		if apierror.FromError(err).Code != apierror.CodeForbidden || len(sources) != 0 {
			t.Fatalf("source permission leak (%s): %v %v", scope, sources, err)
		}
	}
	if err := readsession.NewFactory(a.pool).Read(a.ctx, auth.Draft(), a.project, func(s *readsession.Session) error {
		if _, err := s.Dependencies(a.ctx); err == nil {
			t.Fatal("unconfigured source policy granted traversal")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Cached source identities must not cache an authorization grant.
	if err := a.factory.Read(a.ctx, auth.Release("1.0.0"), a.project, func(s *readsession.Session) error {
		if _, err := s.Dependencies(a.ctx); err != nil {
			return err
		}
		if _, err := s.ProjectDependency(a.ctx, c.project); err != nil {
			return err
		}
		if _, err := s.ProjectDependency(anonymous, c.project); apierror.FromError(err).Code != apierror.CodeForbidden {
			t.Fatalf("cached source bypassed policy: %v", err)
		}
		if sources, err := s.Dependencies(anonymous); apierror.FromError(err).Code != apierror.CodeForbidden || len(sources) != 0 {
			t.Fatalf("cached plan leaked sources: %v %v", sources, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// A privileged job chooses its policy explicitly at the factory boundary.
	calls := map[string]bool{}
	trusted := readsession.NewFactory(a.pool).WithSourceAuthorization(func(_ context.Context, r auth.Resource) error { calls[r.ID] = true; return nil })
	if err := trusted.Read(anonymous, auth.Release("1.0.0"), a.project, func(s *readsession.Session) error { _, err := s.Dependencies(anonymous); return err }); err != nil {
		t.Fatal(err)
	}
	if !calls[b.project] || !calls[c.project] {
		t.Fatal("nested source bypassed configured policy")
	}
}

func TestDependencyPlanningRejectsInvalidLifetimeAndCleansUpCancellation(t *testing.T) {
	for _, s := range []*readsession.Session{nil, {}} {
		if _, err := s.Dependencies(t.Context()); err == nil {
			t.Fatal("zero session planned sources")
		}
		if _, err := s.ProjectDependency(t.Context(), "p"); err == nil {
			t.Fatal("zero session opened project")
		}
		if _, err := s.Dependency(t.Context(), "field", "f"); err == nil {
			t.Fatal("zero session opened entity")
		}
	}
	a, b := newFixture(t, "PLAN_CANCEL_A"), newFixture(t, "PLAN_CANCEL_B")
	b.cut(t)
	referTo(t, a, b)
	config := a.pool.Config()
	config.MaxConns, config.MinConns = 1, 0
	pool, err := pgxpool.NewWithConfig(a.ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, cancel := context.WithCancel(a.ctx)
	defer cancel()
	err = readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead).Read(ctx, auth.Draft(), a.project, func(s *readsession.Session) error {
		if _, err := s.Dependencies(ctx); err != nil {
			return err
		}
		cancel()
		_, err := s.ProjectDependency(ctx, b.project)
		return err
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation cause lost: %v", err)
	}
	check, stop := context.WithTimeout(a.ctx, 2*time.Second)
	defer stop()
	if err := pool.Ping(check); err != nil {
		t.Fatalf("planning leaked connection: %v", err)
	}
}
