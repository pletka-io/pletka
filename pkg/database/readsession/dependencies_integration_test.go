//go:build integration

package readsession_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/collection"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

func cutVersion(t *testing.T, f fixture, version string) error {
	t.Helper()
	_, err := release.NewService(release.NewPostgresStore(f.pool), f.pool, nil).Create(f.ctx, f.project, release.CreateInput{Version: version})
	return err
}

func referTo(t *testing.T, consumer, source fixture) {
	t.Helper()
	if _, err := consumer.pool.Exec(consumer.ctx, `UPDATE weave_override_refs SET target_id=$1, semantic_id=$1 WHERE override_id=$2`, source.collection, consumer.override); err != nil {
		t.Fatal(err)
	}
}

func expectDependencyFailure(t *testing.T, f fixture, version, message string) {
	t.Helper()
	err := cutVersion(t, f, version)
	var validation *release.ErrValidation
	if !errors.As(err, &validation) || !strings.Contains(strings.Join(validation.Fields["dependencies"], " "), message) {
		t.Fatalf("expected dependency failure %q, got %#v (%v)", message, validation, err)
	}
	var rows int
	if err := f.pool.QueryRow(f.ctx, `SELECT
		(SELECT count(*) FROM weave_releases WHERE project_id=$1 AND version=$2) +
		(SELECT count(*) FROM weave_projects_archive WHERE id=$1 AND version_number=$2) +
		(SELECT count(*) FROM weave_collections_archive WHERE project_id=$1 AND version_number=$2) +
		(SELECT count(*) FROM weave_change_set WHERE project_id=$1 AND release_version=$2) +
		(SELECT count(*) FROM weave_release_project_dependencies WHERE project_id=$1 AND version_number=$2)`, f.project, version).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("failed cut retained %d metadata/content/dependency/outbox rows", rows)
	}
}

func TestRefOnlyDependencyUsesCapturedReleaseAfterSourceDeletion(t *testing.T) {
	a, b := newFixture(t, "DEP_REF_A"), newFixture(t, "DEP_REF_B")
	b.cut(t)
	referTo(t, a, b)
	a.cut(t)
	if _, err := b.pool.Exec(b.ctx, `UPDATE weave_collections SET system_name='new source draft' WHERE id=$1`, b.collection); err != nil {
		t.Fatal(err)
	}
	if err := cutVersion(t, b, "2.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := collection.NewPostgresStore(b.pool).Delete(b.ctx, b.collection); err != nil {
		t.Fatal(err)
	}
	var escaped *readsession.Session
	err := a.factory.Read(a.ctx, auth.Release("1.0.0"), a.project, func(s *readsession.Session) error {
		source, err := s.Dependency(a.ctx, "collection", b.collection)
		if err != nil {
			return err
		}
		escaped = source
		if source.ProjectID() != b.project || source.Scope() != auth.Release("1.0.0") {
			t.Fatalf("wrong source selection: %s %s", source.ProjectID(), source.Scope())
		}
		r, err := collection.BindReader(source)
		if err != nil {
			return err
		}
		c, err := r.Get(a.ctx, b.collection)
		if err != nil {
			return err
		}
		if c.SystemName != "before" {
			t.Fatalf("source drifted: %+v", c)
		}
		_, err = s.Dependency(a.ctx, "collection", "unrecorded")
		if apierror.FromError(err).Code != apierror.CodeNotInRelease {
			t.Fatalf("missing pin: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := collection.BindReader(escaped); err == nil {
		t.Fatal("source reader outlived parent session")
	}
}

func TestUnpinnedAdoptionIsFrozenAndReadThroughPublicCollectionService(t *testing.T) {
	a, b := newFixture(t, "DEP_ADOPT_A"), newFixture(t, "DEP_ADOPT_B")
	b.cut(t)
	if _, err := a.pool.Exec(a.ctx, `INSERT INTO weave_adoptions(project_id,context_entity_type,context_entity_id,entity_type,source_project_id,source_entity_id)
		VALUES ($1,'project',$1,'collection',$2,$3)`, a.project, b.project, b.collection); err != nil {
		t.Fatal(err)
	}
	a.cut(t)
	var archivedPin, draftPin string
	if err := a.pool.QueryRow(a.ctx, `SELECT source_version FROM weave_adoptions_archive WHERE project_id=$1 AND version_number='1.0.0'`, a.project).Scan(&archivedPin); err != nil {
		t.Fatal(err)
	}
	if err := a.pool.QueryRow(a.ctx, `SELECT source_version FROM weave_adoptions WHERE project_id=$1`, a.project).Scan(&draftPin); err != nil {
		t.Fatal(err)
	}
	if archivedPin != "1.0.0" || draftPin != "" {
		t.Fatalf("pin capture changed draft or omitted release pin: archive=%q draft=%q", archivedPin, draftPin)
	}
	if _, err := b.pool.Exec(b.ctx, `UPDATE weave_collections SET system_name='later' WHERE id=$1`, b.collection); err != nil {
		t.Fatal(err)
	}
	if err := cutVersion(t, b, "2.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := collection.NewPostgresStore(b.pool).Delete(b.ctx, b.collection); err != nil {
		t.Fatal(err)
	}
	service := collection.NewService(collection.NewPostgresStore(a.pool), collection.NewReadSessions(a.factory), nil, weave.NewPostgresStore(a.pool).Adoptions(), nil, nil, nil, nil, nil, nil, nil, nil)
	rows, _, err := service.ListAdoptedExplicit(a.ctx, auth.Release("1.0.0"), a.project)
	if err != nil || len(rows) != 1 || rows[0].SystemName != "before" {
		t.Fatalf("released adoption drifted: %+v, %v", rows, err)
	}
}

func TestDependencyClosureRetainsTransitiveSourceReleases(t *testing.T) {
	a, b, c := newFixture(t, "DEP_TREE_A"), newFixture(t, "DEP_TREE_B"), newFixture(t, "DEP_TREE_C")
	c.cut(t)
	referTo(t, b, c)
	b.cut(t)
	referTo(t, a, b)
	a.cut(t)
	var pins int
	if err := a.pool.QueryRow(a.ctx, `SELECT count(*) FROM weave_release_project_dependencies WHERE project_id=$1 AND version_number='1.0.0'`, a.project).Scan(&pins); err != nil {
		t.Fatal(err)
	}
	if pins != 2 {
		t.Fatalf("transitive closure has %d pins, want 2", pins)
	}
	err := a.factory.Read(a.ctx, auth.Release("1.0.0"), a.project, func(s *readsession.Session) error {
		bs, err := s.Dependency(a.ctx, "collection", b.collection)
		if err != nil {
			return err
		}
		cs, err := bs.Dependency(a.ctx, "collection", c.collection)
		if err != nil {
			return err
		}
		if cs.ProjectID() != c.project || cs.Scope() != auth.Release("1.0.0") {
			t.Fatalf("wrong transitive source: %s %s", cs.ProjectID(), cs.Scope())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`DELETE FROM weave_releases WHERE project_id=$1 AND version='1.0.0'`,
		`DELETE FROM weave_projects WHERE id=$1`,
	} {
		_, err := c.pool.Exec(c.ctx, query, c.project)
		var pgerr *pgconn.PgError
		if !errors.As(err, &pgerr) || (pgerr.Code != "23503" && pgerr.Code != "23001") {
			t.Fatalf("source retention failed: %v", err)
		}
	}
	// Retiring a source is allowed: already-pinned readers keep their identity.
	if _, err := c.pool.Exec(c.ctx, `UPDATE weave_releases SET archived_at=now() WHERE project_id=$1 AND version='1.0.0'`, c.project); err != nil {
		t.Fatal(err)
	}
}

func TestUnpublishedDependencyRejectsWholeCutThenRetrySucceeds(t *testing.T) {
	a, b := newFixture(t, "DEP_NONE_A"), newFixture(t, "DEP_NONE_B")
	referTo(t, a, b)
	expectDependencyFailure(t, a, "1.0.0", "has no published release")
	b.cut(t)
	a.cut(t)
}

func TestPlacementOnlyDependencyAndTraversalIndependentConflicts(t *testing.T) {
	a, b, c := newFixture(t, "DEP_PICK_A"), newFixture(t, "DEP_PICK_B"), newFixture(t, "DEP_PICK_C")
	c.cut(t)
	referTo(t, b, c)
	b.cut(t)
	if err := cutVersion(t, c, "2.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.pool.Exec(a.ctx, `INSERT INTO weave_models(id,project_id) VALUES ($1,$2)`, a.project+".m", a.project); err != nil {
		t.Fatal(err)
	}
	if _, err := a.pool.Exec(a.ctx, `INSERT INTO weave_collection_placements(project_id,model_id,collection_id) VALUES ($1,$2,$3)`, a.project, a.project+".m", b.collection); err != nil {
		t.Fatal(err)
	}
	// B's latest pins C@1. A also directly requests unpinned C (latest is 2).
	// Selecting B first must not silently make A's direct C request use 1.
	referTo(t, a, c)
	expectDependencyFailure(t, a, "1.0.0", "conflicting release pins")
	// Remove A's direct C edge: the placement alone discovers B and its C pin.
	referTo(t, a, a)
	a.cut(t)
	var pin string
	if err := a.pool.QueryRow(a.ctx, `SELECT source_version FROM weave_release_dependency_entities WHERE project_id=$1 AND entity_type='collection' AND entity_id=$2`, a.project, b.collection).Scan(&pin); err != nil {
		t.Fatal(err)
	}
	if pin != "1.0.0" {
		t.Fatalf("placement source pin = %q", pin)
	}
}

func TestDependencyCaptureRejectsCyclesConflictingPinsAndUnverifiedHistory(t *testing.T) {
	a, b := newFixture(t, "DEP_BAD_A"), newFixture(t, "DEP_BAD_B")
	a.cut(t)
	referTo(t, b, a)
	b.cut(t)
	referTo(t, a, b)
	expectDependencyFailure(t, a, "2.0.0", "cycle")

	c, d := newFixture(t, "DEP_CONFLICT_C"), newFixture(t, "DEP_CONFLICT_D")
	d.cut(t)
	if err := cutVersion(t, d, "2.0.0"); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"1.0.0", "2.0.0"} {
		if _, err := c.pool.Exec(c.ctx, `INSERT INTO weave_adoptions(project_id,context_entity_type,context_entity_id,entity_type,source_project_id,source_entity_id,source_version) VALUES ($1,'project',$1,'collection',$2,$3,$4)`, c.project, d.project, d.collection, version); err != nil {
			t.Fatal(err)
		}
	}
	expectDependencyFailure(t, c, "1.0.0", "conflicting release pins")
	if _, err := c.pool.Exec(c.ctx, `DELETE FROM weave_adoptions WHERE project_id=$1`, c.project); err != nil {
		t.Fatal(err)
	}
	referTo(t, c, d)
	if _, err := d.pool.Exec(d.ctx, `UPDATE weave_releases SET project_dependencies_version=0 WHERE project_id=$1 AND version='2.0.0'`, d.project); err != nil {
		t.Fatal(err)
	}
	expectDependencyFailure(t, c, "1.0.0", "no verified project dependency inventory")
}
