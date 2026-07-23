package testdb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer/ontologyvendor"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
)

// Real-project fixture ids hydrated into every template clone. They are
// materialized snapshots of real, public institution projects (via `pletka
// project init-git --self-contained`, see `make update-fixtures`) and
// committed in the pletka-fixtures submodule under test/fixtures/<id>. They
// replace the former synthetic FXSINGLE/FXPARENT/FXCHILD generator
// (test/fixturegen, removed) with real ontology data — projects and
// institution actors are real and public (see fixtureOrgs in
// fixture_identities.go), but the mock users granted rights on them
// (fixture_identities.go's fixtureUsers) carry no PII.
const (
	// FixtureOntology is AME (Amazigh Enslavement), a self-contained snapshot
	// that additionally vendors its inheritance parents SRD, GLB, LA, and DHI
	// (plus PIR) and ~16 companion ontologies, including CIDOC-CRM and its
	// companions and the aaao (Appellative And Address Ontology) extension.
	// It drives cross-ontology and vendored-parent restore paths.
	FixtureOntology = "AME"
	// FixtureParent is LA (Living Archives), a self-contained project with no
	// inheritance parents of its own. It is also vendored by FixtureOntology
	// (AME) and FixtureChild (ING); hydrating it standalone converges the
	// shared LA rows via the materializer's upserting restore.
	FixtureParent = "LA"
	// FixtureChild is ING, which inherits FixtureParent (LA, primary) and SRD
	// (both vendored self-contained), and drives the inheritance,
	// search-across-parent, bundle, and vendored-parent restore paths.
	FixtureChild = "ING"
)

// hydrateFixtureOrder lists the fixture snapshots to load into the template.
// Each snapshot is self-contained (it vendors its own copy of any project it
// depends on), so there is no ordering dependency between them the way the
// former synthetic parent/child fixtures had — HydrateProjectSnapshot restores
// a snapshot's vendored parents before the snapshot's own project row in the
// same call. FixtureOntology (AME) is hydrated first since it vendors the
// widest set (SRD, GLB, LA, DHI, PIR); FixtureParent (LA) and FixtureChild
// (ING) follow, each upsert-converging the shared vendored LA/SRD rows.
var hydrateFixtureOrder = []string{FixtureOntology, FixtureParent, FixtureChild}

// fixturesRoot resolves the test/fixtures submodule directory relative to the
// module root, walking up from this source file to the go.mod (go test runs in
// the package dir, so a relative path is not reliable). The bool is false when
// the module root or the submodule directory cannot be located — e.g. a build
// with -trimpath, or a checkout that has not fetched the submodule.
func fixturesRoot() (string, bool) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", false
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			root := filepath.Join(dir, "test", "fixtures")
			if info, err := os.Stat(root); err == nil && info.IsDir() {
				return root, true
			}
			return "", false
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// newHydrator builds a Materializer wired with the ontology importer — the same
// wiring production restore uses — so vendored ontologies in a snapshot are
// hydrated through the ontology slice. The adapter comes from the shared leaf
// package ontologyvendor (also used by pkg/app.NewOntologyVendorImporter)
// rather than a local copy: internal/testdb is imported by slice tests, and
// pulling in pkg/app — which assembles every slice — would form an import
// cycle in those test builds, but ontologyvendor imports neither pkg/app nor
// internal/testdb, so both callers can share it.
func newHydrator(pool *pgxpool.Pool, baseDir string) *gitmaterializer.Materializer {
	svc := weaveontology.NewService(weaveontology.NewPostgresStore(pool), nil, nil)
	return gitmaterializer.NewMaterializer(pool, baseDir, nil).WithOntologyImporter(ontologyvendor.New(svc))
}

// hydrateFixtures loads the curated fixture snapshots into the freshly migrated
// template database, before any per-package clone is cut, so every clone
// inherits the same seed data. dsn points at the building template DB. It opens
// (and closes) its own pgx pool so no session outlives the build — the template
// promotion rename refuses to proceed while a session is connected.
//
// A missing submodule directory is reported as an error: under REQUIRE_DB the
// caller turns that into a hard failure, otherwise the whole DB suite is
// skipped (see skipOrFail).
func hydrateFixtures(ctx context.Context, dsn string) error {
	root, ok := fixturesRoot()
	if !ok {
		return fmt.Errorf("fixtures submodule not present; run `git submodule update --init test/fixtures` (or, if it is empty, `make update-fixtures`)")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("open pool for hydration: %w", err)
	}
	defer pool.Close()

	if err := seedFixtureIdentities(ctx, pool); err != nil {
		return err
	}

	for _, id := range hydrateFixtureOrder {
		snapshotDir := filepath.Join(root, id)
		if info, err := os.Stat(snapshotDir); err != nil || !info.IsDir() {
			return fmt.Errorf("fixture snapshot %s missing at %s; run `make update-fixtures`", id, snapshotDir)
		}
		// A fresh, per-project base dir keeps the materializer's scratch state
		// isolated between snapshots.
		mat := newHydrator(pool, filepath.Join(os.TempDir(), "testdb-hydrate", id))
		if _, err := mat.HydrateProjectSnapshot(ctx, snapshotDir); err != nil {
			return fmt.Errorf("hydrate fixture %s: %w", id, err)
		}
	}

	// Project-scope rights reference project ids that only exist once the
	// snapshots above are hydrated, so they are seeded last.
	if err := seedFixtureProjectRights(ctx, pool); err != nil {
		return err
	}
	return nil
}

// HydrateSnapshot loads a fixture snapshot into pool under a fresh target id,
// on top of the template baseline, and returns the restore plan. It is the
// parallel-safe way for a test to get an isolated extra copy (asID) of a
// fixture (name is one of the Fixture* ids) without colliding with the
// template-seeded copy.
func HydrateSnapshot(t *testing.T, pool *pgxpool.Pool, name, asID string) *gitmaterializer.RestorePlan {
	t.Helper()
	root, ok := fixturesRoot()
	if !ok {
		t.Skip("test/fixtures submodule not available")
	}
	snapshotDir := filepath.Join(root, name)
	if info, err := os.Stat(snapshotDir); err != nil || !info.IsDir() {
		t.Fatalf("fixture snapshot %s missing at %s; run `make update-fixtures`", name, snapshotDir)
	}
	mat := newHydrator(pool, t.TempDir())
	plan, err := mat.HydrateProjectSnapshotAs(context.Background(), snapshotDir, asID)
	if err != nil {
		t.Fatalf("hydrate fixture %s as %s: %v", name, asID, err)
	}
	return plan
}
