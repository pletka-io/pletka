//go:build integration

package projectontologyversion_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
	pov "github.com/pletka-io/pletka/pkg/weave/projectontologyversion"
)

// testOntologyReader and testOntologyVersionReader mirror the unexported
// sliceOntologyReader/sliceOntologyVersionReader adapters pkg/app builds in
// weave_helpers.go (buildProjectOntologyVersionHost's wiring) — reproduced
// here because those adapters are unexported to pkg/app.
type testOntologyReader struct{ store weaveontologyStore }
type testOntologyVersionReader struct{ store weaveontologyStore }

// weaveontologyStore narrows pkg/weave/ontology's Store to the methods this
// test file needs.
type weaveontologyStore interface {
	ListOntologies(ctx context.Context) ([]*domain.Ontology, error)
	GetOntology(ctx context.Context, id string) (*domain.Ontology, error)
	GetVersion(ctx context.Context, id string) (*domain.OntologyVersion, error)
	ListVersionsByOntology(ctx context.Context, ontologyID string) ([]*domain.OntologyVersion, error)
}

func (a testOntologyReader) List(ctx context.Context) ([]*domain.Ontology, error) {
	return a.store.ListOntologies(ctx)
}

func (a testOntologyReader) GetByID(ctx context.Context, id string) (*domain.Ontology, error) {
	return a.store.GetOntology(ctx, id)
}

func (a testOntologyVersionReader) GetByID(ctx context.Context, id string) (*domain.OntologyVersion, error) {
	return a.store.GetVersion(ctx, id)
}

func (a testOntologyVersionReader) ListByOntology(ctx context.Context, ontologyID string) ([]*domain.OntologyVersion, error) {
	return a.store.ListVersionsByOntology(ctx, ontologyID)
}

// newServiceForTest wires a *pov.Service the same way pkg/app does (see
// buildProjectOntologyVersionHost in pkg/app/weave_slice_hosts.go), using
// this file's local reader adapters since pkg/app's are unexported.
func newServiceForTest(pool *pgxpool.Pool, ontStore weaveontologyStore) *pov.Service {
	weaveStore := weave.NewPostgresStore(pool)
	return pov.NewService(
		pov.NewPostgresStore(pool),
		weaveStore.Projects(),
		testOntologyReader{store: ontStore},
		testOntologyVersionReader{store: ontStore},
		nil,
		nil,
	)
}

// crmActiveVersionID resolves the active crm ontology version id from the
// fixture rather than hardcoding it.
func crmActiveVersionID(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	_, versionID, _ := activeOntologyVersion(t, pool, "crm")
	return versionID
}

// activeOntologyVersion resolves (ontology_id, active_version_id,
// active_version_string) for a given ontology prefix.
func activeOntologyVersion(t *testing.T, pool *pgxpool.Pool, prefix string) (ontologyID, versionID, versionString string) {
	t.Helper()
	err := pool.QueryRow(context.Background(), `
		SELECT o.id, v.id, v.version_string
		FROM weave_ontology_versions v
		JOIN weave_ontologies o ON o.id = v.ontology_id
		WHERE o.prefix = $1 AND v.is_active = true
		LIMIT 1
	`, prefix).Scan(&ontologyID, &versionID, &versionString)
	if err != nil {
		t.Fatalf("resolve active %s version: %v", prefix, err)
	}
	return ontologyID, versionID, versionString
}

// seedExtendsHierarchy establishes an extends chain crm -> aaao -> {cpro,
// pwro} -> globo (pwro's child) on top of the vendored fixture data.
//
// The fixture's vendored-ontology import path (gitmaterializer/ontologyvendor
// -> Service.ImportVendoredVersion) deliberately does not persist
// ExtendsOntologyID/CompatibleBaseVersions from the manifest — see
// VendoredOntologyImportRequest.Imports's doc comment ("metadata only; not
// yet persisted") in pkg/weave/ontology/import_vendored.go. So aaao/cpro/
// pwro/globo land as independent ontologies with no extends relationship or
// compatibility declared, even though the real production import path (the
// platform-owned Airtable/ops ontology loader) does set both. This helper
// reproduces that missing piece directly against the test DB clone so the
// forest-building logic in ListAvailableExtensions can be exercised against
// real Ontology/OntologyVersion rows fetched through the real store, rather
// than against hand-built domain.Ontology fakes.
//
// Compatibility is seeded PARENT-relative, matching how
// ListAvailableExtensions actually gates each tree level: aaao's version
// declares compatibility with crm's version string; cpro's and pwro's
// versions declare compatibility with aaao's version string (not crm's);
// globo's version declares compatibility with pwro's version string (not
// crm's or aaao's). This is what a real ontology author would declare —
// "I extend aaao 2.2" — and it is the only way to exercise the per-parent
// gating this test covers; seeding every level against crm's version
// string would mask a bug where the forest builder gates grandchildren
// against the wrong ancestor.
func seedExtendsHierarchy(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	crmOntID, _, crmVerStr := activeOntologyVersion(t, pool, "crm")
	aaaoOntID, aaaoVerID, aaaoVerStr := activeOntologyVersion(t, pool, "aaao")
	cproOntID, cproVerID, _ := activeOntologyVersion(t, pool, "cpro")
	pwroOntID, pwroVerID, pwroVerStr := activeOntologyVersion(t, pool, "pwro")
	globoOntID, globoVerID, _ := activeOntologyVersion(t, pool, "globo")
	// crmdig is wired as a structural child of aaao but never given a
	// compatible_base_versions entry that matches aaao's version — the
	// negative case: an extension incompatible with its parent must be
	// pruned from the forest, not offered under aaao (and, per the
	// pruning rule, not reattached anywhere else either).
	crmdigOntID, crmdigVerID, _ := activeOntologyVersion(t, pool, "crmdig")

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed extends hierarchy: %v", err)
		}
	}

	// aaao extends crm; cpro, pwro, and crmdig extend aaao; globo extends pwro.
	exec(`UPDATE weave_ontologies SET ontology_type = 'extension', extends_ontology_id = $1 WHERE id = $2`, crmOntID, aaaoOntID)
	exec(`UPDATE weave_ontologies SET ontology_type = 'extension', extends_ontology_id = $1 WHERE id = $2`, aaaoOntID, cproOntID)
	exec(`UPDATE weave_ontologies SET ontology_type = 'extension', extends_ontology_id = $1 WHERE id = $2`, aaaoOntID, pwroOntID)
	exec(`UPDATE weave_ontologies SET ontology_type = 'extension', extends_ontology_id = $1 WHERE id = $2`, pwroOntID, globoOntID)
	exec(`UPDATE weave_ontologies SET ontology_type = 'extension', extends_ontology_id = $1 WHERE id = $2`, aaaoOntID, crmdigOntID)

	// Declare each extension version compatible with the specific version
	// of the thing it actually extends (its parent in the tree), not the
	// root crm base version.
	exec(`UPDATE weave_ontology_versions SET compatible_base_versions = $1 WHERE id = $2`, []string{crmVerStr}, aaaoVerID)
	exec(`UPDATE weave_ontology_versions SET compatible_base_versions = $1 WHERE id = $2`, []string{aaaoVerStr}, cproVerID)
	exec(`UPDATE weave_ontology_versions SET compatible_base_versions = $1 WHERE id = $2`, []string{aaaoVerStr}, pwroVerID)
	exec(`UPDATE weave_ontology_versions SET compatible_base_versions = $1 WHERE id = $2`, []string{pwroVerStr}, globoVerID)
	// crmdig declares compatibility with something other than aaao's
	// version — it must not surface as aaao's child.
	exec(`UPDATE weave_ontology_versions SET compatible_base_versions = $1 WHERE id = $2`, []string{"not-a-real-version"}, crmdigVerID)
}

// findNode locates a node by prefix at the given level of a forest.
func findNode(nodes []pov.ExtensionTreeNode, prefix string) *pov.ExtensionTreeNode {
	for i := range nodes {
		if nodes[i].Prefix == prefix {
			return &nodes[i]
		}
	}
	return nil
}

func TestListAvailableExtensions_ReturnsForestByExtends(t *testing.T) {
	ctx := weaveauth.WithSnapshot(context.Background(), &weaveauth.AuthSnapshot{IsSuperAdmin: true})
	pool := testdb.Pool(t)

	seedExtendsHierarchy(t, pool)

	ontStore := weaveontology.NewPostgresStore(pool)
	svc := newServiceForTest(pool, ontStore)

	baseVersionID := crmActiveVersionID(t, pool)
	forest, err := svc.ListAvailableExtensions(ctx, testdb.FixtureParent, baseVersionID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	// aaao is a root (extends crm) and carries children (cpro/pwro).
	aaao := findNode(forest, "aaao")
	if aaao == nil {
		t.Fatal("aaao missing from crm's direct children")
	}
	if !aaao.HasChildren || len(aaao.Children) == 0 {
		t.Fatal("aaao should expand to its own extensions")
	}
	if findNode(aaao.Children, "cpro") == nil {
		t.Fatal("cpro should be a child of aaao, not a root under crm")
	}
	pwro := findNode(aaao.Children, "pwro")
	if pwro == nil {
		t.Fatal("pwro should be a child of aaao, not a root under crm")
	}

	// globo (extends pwro, gated against pwro's version) must NOT appear as
	// a direct child of crm ...
	if findNode(forest, "globo") != nil {
		t.Fatal("globo (grandchild) must not be a crm root")
	}
	// ... but must be present, nested, under pwro: the full drill
	// crm -> aaao -> pwro -> globo is what proves per-parent gating (each
	// level is checked against its own parent's chosen version, not
	// against the root crm version globo never declared compatibility
	// with).
	if !pwro.HasChildren || len(pwro.Children) == 0 {
		t.Fatal("pwro should expand to its own extensions")
	}
	if findNode(pwro.Children, "globo") == nil {
		t.Fatal("globo should be nested under pwro (crm -> aaao -> pwro -> globo)")
	}

	// crmdig is structurally wired as a child of aaao but declares
	// compatibility with neither aaao's version nor crm's — an extension
	// incompatible with its parent must be pruned, not offered under aaao
	// and not reattached anywhere else in the forest.
	if findNode(aaao.Children, "crmdig") != nil {
		t.Fatal("crmdig is incompatible with aaao's version and must be pruned from aaao's children")
	}
	if findNode(forest, "crmdig") != nil {
		t.Fatal("crmdig must not be reattached as a crm root either")
	}
}
