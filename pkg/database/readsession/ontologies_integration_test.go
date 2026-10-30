//go:build integration

package readsession_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ontologyartifact"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
)

type ontologyTrace struct {
	mu      sync.Mutex
	pids    map[uint32]bool
	queries []string
}

func (tr *ontologyTrace) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.pids[conn.PgConn().PID()] = true
	tr.queries = append(tr.queries, data.SQL)
	return ctx
}
func (*ontologyTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// One connection also makes a nested factory fail on the deadline instead of
// silently obtaining another snapshot. The trace proves the source traversal and
// both ontology queries actually ran on the same backend and transaction.
func TestOntologyFacetSharesSessionAcrossRepeatedAndNestedSources(t *testing.T) {
	a, b, c := newFixture(t, "ONTO_A"), newFixture(t, "ONTO_B"), newFixture(t, "ONTO_C")
	store := ontology.NewPostgresStore(c.pool)
	defaults, err := store.GetOntologyByPrefix(c.ctx, "defaults")
	if err != nil || defaults == nil {
		t.Fatalf("defaults fixture: %v", err)
	}
	defaultsVersion := ontology.GenerateVersionID("session-defaults", "1")
	if _, err := store.ImportVersion(c.ctx, ontology.ImportVersionInput{Version: ontology.CreateVersionInput{ID: defaultsVersion, OntologyID: defaults.ID, VersionString: "session-1", RDFContent: "<defaults/>"}}); err != nil {
		t.Fatal(err)
	}
	const prefix = "session_ontology"
	owner, version := ontology.GenerateOntologyID(prefix), ontology.GenerateVersionID(prefix, "1")
	if _, err := store.CreateOntology(c.ctx, ontology.CreateOntologyInput{ID: owner, Prefix: prefix, Name: "Original", Namespace: "https://original.example/", OntologyType: domain.OntologyTypeBase}); err != nil {
		t.Fatal(err)
	}
	input := ontology.ImportVersionInput{
		Version:           ontology.CreateVersionInput{ID: version, OntologyID: owner, VersionString: "1", RDFContent: "<original/>"},
		Publication:       ontologyrelease.Options{Pins: map[string]string{defaults.ID: defaultsVersion}},
		Classes:           []ontology.ImportClass{{ID: ontology.GenerateClassID(version, "https://original.example/A"), URI: "https://original.example/A", Prefix: prefix, LocalName: "A"}},
		NamespaceBindings: []ontology.NamespaceBinding{{Prefix: prefix, Namespace: "https://original.example/"}},
		SetActive:         true,
	}
	if _, err := store.ImportVersion(c.ctx, input); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{version, defaultsVersion} {
		if _, err := c.pool.Exec(c.ctx, `INSERT INTO weave_project_ontology_versions(project_id,ontology_version_id,added_at) VALUES ($1,$2,now())`, c.project, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.pool.Exec(c.ctx, `UPDATE weave_fields SET path_elements=jsonb_build_array(jsonb_build_object('position',0,'type','class','prefix',$3::text,'local_name','A','uri','https://original.example/A','ontology_version_id',$2::text)) WHERE id=$1`, c.field, version, prefix); err != nil {
		t.Fatal(err)
	}
	c.cut(t)
	referTo(t, b, c)
	b.cut(t)
	referTo(t, a, b)
	a.cut(t)
	var expected []ontologyartifact.ProjectArtifact
	if err := ontologyrelease.NewFactory(c.pool).ReadProject(c.ctx, c.project, "1.0.0", func(r ontologyrelease.ProjectReader) error {
		var err error
		expected, err = r.Artifacts(c.ctx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(expected) != 2 {
		t.Fatalf("expected full ontology closure, got %d", len(expected))
	}

	// Make newer source content and ontology policy disagree with the retained cut.
	if _, err := c.pool.Exec(c.ctx, `UPDATE weave_ontologies SET name='Changed' WHERE id=$1`, owner); err != nil {
		t.Fatal(err)
	}
	input.Version.ID = ontology.GenerateVersionID(prefix, "2")
	input.Version.VersionString, input.Version.RDFContent = "2", "<later/>"
	input.Classes[0].ID = ontology.GenerateClassID(input.Version.ID, "https://original.example/A")
	if _, err := store.ImportVersion(c.ctx, input); err != nil {
		t.Fatal(err)
	}
	if _, err := c.pool.Exec(c.ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id=$1`, c.project); err != nil {
		t.Fatal(err)
	}
	if _, err := c.pool.Exec(c.ctx, `UPDATE weave_fields SET path_elements='[]' WHERE id=$1`, c.field); err != nil {
		t.Fatal(err)
	}
	if err := cutVersion(t, c, "2.0.0"); err != nil {
		t.Fatal(err)
	}

	tr := &ontologyTrace{pids: map[uint32]bool{}}
	config := a.pool.Config()
	config.MaxConns, config.MinConns = 1, 0
	config.ConnConfig.Tracer = tr
	pool, err := pgxpool.NewWithConfig(a.ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()
	var escaped readsession.OntologyReader
	var source *readsession.Session
	if err := readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead).Read(ctx, auth.Release("1.0.0"), a.project, func(s *readsession.Session) error {
		for range 2 {
			bs, err := s.Dependency(ctx, "collection", b.collection)
			if err != nil {
				return err
			}
			cs, err := bs.Dependency(ctx, "collection", c.collection)
			if err != nil {
				return err
			}
			source = cs
			if cs.Scope() != auth.Release("1.0.0") {
				t.Fatal("source drifted to latest")
			}
			if _, err := cs.Descriptors(ctx); err != nil {
				return err
			}
			if _, err := cs.Metadata(ctx); err != nil {
				return err
			}
			if _, err := cs.Content(ctx); err != nil {
				return err
			}
			r, err := cs.Ontologies(ctx)
			if err != nil {
				return err
			}
			escaped = r
			values, err := r.Artifacts(ctx)
			if err != nil {
				return err
			}
			if diff := cmp.Diff(expected, values); diff != "" {
				t.Fatalf("artifact bytes or link flags drifted (-want +got): %s", diff)
			}
			values[0].Artifact.JSON = "consumer mutation"
			refs, err := r.References(ctx)
			if err != nil {
				return err
			}
			if len(refs) != 1 || refs[0].VersionID != version || refs[0].EntityID != c.field || refs[0].URI != "https://original.example/A" {
				t.Fatalf("wrong captured references: %+v", refs)
			}
			refs[0].URI = "consumer mutation"
			refs, err = r.References(ctx)
			if err != nil {
				return err
			}
			if refs[0].URI != "https://original.example/A" {
				t.Fatal("reference mutation escaped")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := escaped.Artifacts(ctx); err == nil {
		t.Fatal("artifact reader escaped session")
	}
	if _, err := escaped.References(ctx); err == nil {
		t.Fatal("reference reader escaped session")
	}
	if _, err := source.Ontologies(ctx); err == nil {
		t.Fatal("closed source acquired reader")
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	begins, inventories, artifacts, references := 0, 0, 0, 0
	for _, sql := range tr.queries {
		if strings.HasPrefix(sql, "begin") {
			begins++
		}
		if strings.Contains(sql, "ProjectOntologyInventory") {
			inventories++
		}
		if strings.Contains(sql, "ProjectOntologyArtifacts") {
			artifacts++
		}
		if strings.Contains(sql, "ProjectOntologyReferences") {
			references++
		}
		if strings.Contains(sql, "weave_ontology_active_versions") || strings.Contains(sql, "WeaveLatest") {
			t.Fatalf("current policy query: %s", sql)
		}
	}
	if len(tr.pids) != 1 || begins != 1 || inventories != 2 || artifacts != 2 || references != 4 {
		t.Fatalf("mixed or missing transaction work: backends=%d begins=%d inventory=%d artifacts=%d references=%d", len(tr.pids), begins, inventories, artifacts, references)
	}
}

func TestOntologyFacetRejectsInvalidSessionsAndCleansUpCancellation(t *testing.T) {
	for _, s := range []*readsession.Session{nil, {}} {
		if _, err := s.Ontologies(t.Context()); err == nil {
			t.Fatal("zero session exposed ontology reader")
		}
	}
	f := newFixture(t, "ONTO_LIFETIME")
	f.cut(t)
	if err := f.factory.Read(f.ctx, auth.Draft(), f.project, func(s *readsession.Session) error {
		if _, err := s.Ontologies(f.ctx); err == nil {
			t.Fatal("draft exposed released ontologies")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	config := f.pool.Config()
	config.MaxConns, config.MinConns = 1, 0
	pool, err := pgxpool.NewWithConfig(f.ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	var escaped readsession.OntologyReader
	err = readsession.NewFactory(pool).Read(ctx, auth.Release("1.0.0"), f.project, func(s *readsession.Session) error {
		r, err := s.Ontologies(ctx)
		if err != nil {
			return err
		}
		escaped = r
		cancel()
		_, err = r.Artifacts(ctx)
		return err
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation cause lost: %v", err)
	}
	check, stop := context.WithTimeout(f.ctx, 2*time.Second)
	defer stop()
	if err := pool.Ping(check); err != nil {
		t.Fatalf("cancellation leaked connection: %v", err)
	}
	if _, err := escaped.References(check); err == nil {
		t.Fatal("canceled reader escaped using a new context")
	}
}
