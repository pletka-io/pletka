//go:build integration

package projectontologyversion_test

import (
	"context"
	"encoding/json"
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
// The forest is built purely from the extends relationship, NOT from
// compatible_base_versions (that field is populated inconsistently in real
// data — every AAAo extension declares the root crm version, some declare
// nothing — so it is not a reliable gate). The compat values seeded below
// are deliberately varied/bogus to prove the builder ignores them.
func seedExtendsHierarchy(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	crmOntID, _, crmVerStr := activeOntologyVersion(t, pool, "crm")
	aaaoOntID, aaaoVerID, aaaoVerStr := activeOntologyVersion(t, pool, "aaao")
	cproOntID, cproVerID, _ := activeOntologyVersion(t, pool, "cpro")
	pwroOntID, pwroVerID, pwroVerStr := activeOntologyVersion(t, pool, "pwro")
	globoOntID, globoVerID, _ := activeOntologyVersion(t, pool, "globo")

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed extends hierarchy: %v", err)
		}
	}

	// aaao extends crm; cpro and pwro extend aaao; globo extends pwro.
	exec(`UPDATE weave_ontologies SET ontology_type = 'extension', extends_ontology_id = $1 WHERE id = $2`, crmOntID, aaaoOntID)
	exec(`UPDATE weave_ontologies SET ontology_type = 'extension', extends_ontology_id = $1 WHERE id = $2`, aaaoOntID, cproOntID)
	exec(`UPDATE weave_ontologies SET ontology_type = 'extension', extends_ontology_id = $1 WHERE id = $2`, aaaoOntID, pwroOntID)
	exec(`UPDATE weave_ontologies SET ontology_type = 'extension', extends_ontology_id = $1 WHERE id = $2`, pwroOntID, globoOntID)

	// Compatibility is seeded deliberately varied to prove the forest IGNORES
	// it: pwro gets an outright BOGUS value, yet pwro (and its child globo)
	// must still surface — because inclusion is structural, not compat-gated.
	exec(`UPDATE weave_ontology_versions SET compatible_base_versions = $1 WHERE id = $2`, []string{crmVerStr}, aaaoVerID)
	exec(`UPDATE weave_ontology_versions SET compatible_base_versions = $1 WHERE id = $2`, []string{aaaoVerStr}, cproVerID)
	exec(`UPDATE weave_ontology_versions SET compatible_base_versions = $1 WHERE id = $2`, []string{"not-a-real-version"}, pwroVerID)
	exec(`UPDATE weave_ontology_versions SET compatible_base_versions = $1 WHERE id = $2`, []string{pwroVerStr}, globoVerID)
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

// seedEmptyProject inserts a minimal, throwaway project row (reusing the LA
// fixture's owner actor to satisfy the owner_id FK) and registers its
// cleanup. Returns the new project's id.
func seedEmptyProject(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()

	var ownerID string
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM weave_projects WHERE id = $1`, testdb.FixtureParent).Scan(&ownerID); err != nil {
		t.Fatalf("resolve fixture owner: %v", err)
	}

	const projectID = "ZCHAIN"
	cleanup := func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_ontology_versions WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id = $1`, projectID)
	}
	cleanup()
	t.Cleanup(cleanup)

	uiName, _ := json.Marshal(map[string]string{"en": "Ancestor Chain Probe"})
	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_projects (id, ui_name, description, status, owner_id, visibility, created_at, updated_at)
		VALUES ($1, $2, $2, 'draft', $3, 'private', NOW(), NOW())
	`, projectID, uiName, ownerID); err != nil {
		t.Fatalf("seed empty project: %v", err)
	}
	return projectID
}

// activeVersionIDForPrefix resolves just the active version id for an
// ontology prefix.
func activeVersionIDForPrefix(t *testing.T, pool *pgxpool.Pool, prefix string) string {
	t.Helper()
	_, versionID, _ := activeOntologyVersion(t, pool, prefix)
	return versionID
}

// linkedVersionIDs lists the ontology_version_id column of every
// weave_project_ontology_versions row for projectID.
func linkedVersionIDs(t *testing.T, pool *pgxpool.Pool, projectID string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT ontology_version_id FROM weave_project_ontology_versions WHERE project_id = $1`, projectID)
	if err != nil {
		t.Fatalf("query linked versions: %v", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan linked version: %v", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate linked versions: %v", err)
	}
	return out
}

// contains reports whether want is present in list.
func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
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

	// globo (extends pwro) must NOT appear as a direct child of crm ...
	if findNode(forest, "globo") != nil {
		t.Fatal("globo (grandchild) must not be a crm root")
	}
	// ... but must be present, nested, under pwro: the full structural drill
	// crm -> aaao -> pwro -> globo (each node placed by extends_ontology_id,
	// independent of any compatibility declaration).
	if !pwro.HasChildren || len(pwro.Children) == 0 {
		t.Fatal("pwro should expand to its own extensions")
	}
	if findNode(pwro.Children, "globo") == nil {
		t.Fatal("globo should be nested under pwro (crm -> aaao -> pwro -> globo)")
	}

	// pwro carries a deliberately bogus compatible_base_versions, yet it
	// surfaced above (as aaao's child) with globo nested under it — proof
	// that inclusion is structural (extends_ontology_id), not compat-gated.
}

// TestCreate_AutoLinksAncestorChain proves that linking a deep extension
// (cpro, which extends aaao, which extends crm) auto-links the intermediate
// ancestor (aaao) even when only cpro was explicitly selected — the base
// (crm) is already linked via CreateInput.VersionID.
func TestCreate_AutoLinksAncestorChain(t *testing.T) {
	ctx := weaveauth.WithSnapshot(context.Background(), &weaveauth.AuthSnapshot{IsSuperAdmin: true})
	pool := testdb.Pool(t)

	seedExtendsHierarchy(t, pool)

	ontStore := weaveontology.NewPostgresStore(pool)
	svc := newServiceForTest(pool, ontStore)

	// Fresh throwaway project linking crm as base + cpro as an extension.
	projectID := seedEmptyProject(t, pool)
	crmVer := crmActiveVersionID(t, pool)
	cproVer := activeVersionIDForPrefix(t, pool, "cpro")
	aaaoVer := activeVersionIDForPrefix(t, pool, "aaao")

	if _, err := svc.Create(ctx, projectID, pov.CreateInput{
		VersionID:  crmVer,
		Extensions: []string{cproVer}, // aaao NOT selected explicitly
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	linked := linkedVersionIDs(t, pool, projectID)
	for _, want := range []string{crmVer, cproVer, aaaoVer} {
		if !contains(linked, want) {
			t.Fatalf("expected %s linked; got %v", want, linked)
		}
	}
}

// findGroupByPrefix locates the own PaneGroup whose base has the given
// ontology prefix.
func findGroupByPrefix(groups []pov.PaneGroup, prefix string) *pov.PaneGroup {
	for i := range groups {
		if groups[i].Base != nil && groups[i].Base.Prefix == prefix {
			return &groups[i]
		}
	}
	return nil
}

// hasBaseGroup reports whether view has an own PaneGroup base-level
// heading for the given ontology prefix.
func hasBaseGroup(view *pov.PaneView, prefix string) bool {
	return findGroupByPrefix(view.Groups, prefix) != nil
}

// groupContains reports whether the base-level heading for basePrefix
// lists childPrefix among its nested extensions.
func groupContains(view *pov.PaneView, basePrefix, childPrefix string) bool {
	g := findGroupByPrefix(view.Groups, basePrefix)
	if g == nil {
		return false
	}
	for _, ext := range g.Extensions {
		if ext.Prefix == childPrefix {
			return true
		}
	}
	return false
}

// baseGroupPrefixes lists the ontology prefixes of every base-level
// heading in view, for failure messages.
func baseGroupPrefixes(view *pov.PaneView) []string {
	out := make([]string, 0, len(view.Groups))
	for _, g := range view.Groups {
		if g.Base != nil {
			out = append(out, g.Base.Prefix)
		}
	}
	return out
}

// TestPaneView_GroupsSecondaryBase proves that linking a deep extension
// (cpro, which extends aaao, which extends crm) promotes aaao to its own
// base-level PaneGroup heading in the settings ontology pane — instead of
// aaao's linked child (cpro) being dropped into the "unattached
// extensions" orphan bucket, which is what buildOwnGroups did before this
// fix (cpro's extends_ontology_id points at aaao, not at a true
// ontology_type=="base" row, so it never matched a group).
func TestPaneView_GroupsSecondaryBase(t *testing.T) {
	ctx := weaveauth.WithSnapshot(context.Background(), &weaveauth.AuthSnapshot{IsSuperAdmin: true})
	pool := testdb.Pool(t)

	seedExtendsHierarchy(t, pool)

	ontStore := weaveontology.NewPostgresStore(pool)
	svc := newServiceForTest(pool, ontStore)

	projectID := seedEmptyProject(t, pool)
	crmVer := crmActiveVersionID(t, pool)
	cproVer := activeVersionIDForPrefix(t, pool, "cpro")
	if _, err := svc.Create(ctx, projectID, pov.CreateInput{
		VersionID:  crmVer,
		Extensions: []string{cproVer}, // aaao auto-linked as an ancestor (Task 2)
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	view, err := svc.PaneView(ctx, projectID)
	if err != nil {
		t.Fatalf("pane view: %v", err)
	}

	// Expect two base-level headings: crm (true base) and aaao (promoted
	// because cpro is linked under it).
	if !hasBaseGroup(view, "crm") || !hasBaseGroup(view, "aaao") {
		t.Fatalf("want base groups crm + aaao; got %v", baseGroupPrefixes(view))
	}
	if !groupContains(view, "aaao", "cpro") {
		t.Fatal("cpro should nest under the aaao base-level heading")
	}
	// cpro must not also appear flattened into crm's own heading, and
	// aaao must not appear twice (once promoted, once still flat under
	// crm).
	if groupContains(view, "crm", "cpro") {
		t.Fatal("cpro should not be listed under crm; it nests under aaao")
	}
	if groupContains(view, "crm", "aaao") {
		t.Fatal("aaao should not be listed as a flat extension under crm; it's promoted to its own heading")
	}
}

// TestListView_LabelsItemsIndividually guards #3554 item 4: the GET / list
// view must carry each linked ontology's own name per item (base AND its
// extensions), not reuse the group's base label for every row. Before the
// fix renderOwnGroup labelled every item with BaseLabel, so an extension
// showed the base's name.
func TestListView_LabelsItemsIndividually(t *testing.T) {
	ctx := weaveauth.WithSnapshot(context.Background(), &weaveauth.AuthSnapshot{IsSuperAdmin: true})
	pool := testdb.Pool(t)

	seedExtendsHierarchy(t, pool)
	ontStore := weaveontology.NewPostgresStore(pool)
	svc := newServiceForTest(pool, ontStore)

	projectID := seedEmptyProject(t, pool)
	crmVer := crmActiveVersionID(t, pool)
	aaaoVer := activeVersionIDForPrefix(t, pool, "aaao")
	if _, err := svc.Create(ctx, projectID, pov.CreateInput{
		VersionID:  crmVer,
		Extensions: []string{aaaoVer}, // crm base + aaao extension directly under it
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	view, err := svc.ListView(ctx, projectID)
	if err != nil {
		t.Fatalf("list view: %v", err)
	}
	if len(view.OwnGroups) != 1 {
		t.Fatalf("want 1 own group (crm base + aaao ext); got %d", len(view.OwnGroups))
	}
	g := view.OwnGroups[0]
	if len(g.Items) != 2 {
		t.Fatalf("want 2 items (base + extension); got %d", len(g.Items))
	}
	names := map[string]bool{}
	for _, it := range g.Items {
		if it.OntologyName == "" {
			t.Fatalf("item %s has empty OntologyName", it.Link.OntologyVersionID)
		}
		names[it.OntologyName] = true
	}
	if len(names) != 2 {
		t.Fatalf("items must carry distinct per-ontology names, not the shared base label; got names %v", names)
	}
}
