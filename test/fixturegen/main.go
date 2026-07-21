// Command fixturegen regenerates the synthetic curated fixture snapshots under
// test/fixtures/ from a scratch Postgres database. It seeds three synthetic
// projects through the real service/store layer, then materializes each as a
// self-contained git snapshot the testdb template hydrates.
//
// It is DSN-driven and stdlib+core only — it never provisions a container
// itself. The `make build-test-snapshot` target spins a throwaway
// postgres:18-alpine via `docker run`, passes its URL here, and removes it
// after. Keeping this a plain main package means `go list -deps ./...` stays
// free of any Docker/testcontainers dependency.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/pkg/database"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
)

// Synthetic fixture identities. These are NOT real customer projects — no PII,
// no real customer project ids. They mirror the testdb.Fixture* constants.
const (
	fixtureOwnerID = "FXORG"

	fixtureSingleID = "FXSINGLE"
	fixtureParentID = "FXPARENT"
	fixtureChildID  = "FXCHILD"

	ontologySlug      = "fixture-crm"
	ontologyPrefix    = "crm"
	ontologyNamespace = "http://www.cidoc-crm.org/cidoc-crm/"
	ontologyVersion   = "1.0"
	// ontologyVersion2 is a second version of the same synthetic ontology,
	// linked to FixtureParent only (not FixtureChild). It exists so a child's
	// resolved own+inherited ontology bundle is strictly larger than its own —
	// the inheritance regression guarded by projectontologyversion's
	// TestBundleForVersions_SmokesAgainstDB.
	ontologyVersion2 = "2.0"
)

// fixtureCRM is a minimal, synthetic CIDOC-CRM subset. It declares just enough
// real CRM class/property local-names that the qname assertions in the migrated
// tests stay valid. Classes: E21_Person, E42_Identifier, E55_Type, E67_Birth,
// E33_Linguistic_Object. Properties: P1_is_identified_by, P2_has_type,
// P4_has_time.
const fixtureCRM = `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:crm="` + ontologyNamespace + `"
         xmlns:owl="http://www.w3.org/2002/07/owl#"
         xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#">
  <owl:Ontology rdf:about="` + ontologyNamespace + `">
    <owl:versionInfo>` + ontologyVersion + `</owl:versionInfo>
  </owl:Ontology>

  <rdfs:Class rdf:about="` + ontologyNamespace + `E21_Person">
    <rdfs:label xml:lang="en">Person</rdfs:label>
  </rdfs:Class>
  <rdfs:Class rdf:about="` + ontologyNamespace + `E42_Identifier">
    <rdfs:label xml:lang="en">Identifier</rdfs:label>
  </rdfs:Class>
  <rdfs:Class rdf:about="` + ontologyNamespace + `E55_Type">
    <rdfs:label xml:lang="en">Type</rdfs:label>
  </rdfs:Class>
  <rdfs:Class rdf:about="` + ontologyNamespace + `E67_Birth">
    <rdfs:label xml:lang="en">Birth</rdfs:label>
  </rdfs:Class>
  <rdfs:Class rdf:about="` + ontologyNamespace + `E33_Linguistic_Object">
    <rdfs:label xml:lang="en">Linguistic Object</rdfs:label>
  </rdfs:Class>

  <rdf:Property rdf:about="` + ontologyNamespace + `P1_is_identified_by">
    <rdfs:label xml:lang="en">is identified by</rdfs:label>
    <rdfs:domain rdf:resource="` + ontologyNamespace + `E21_Person"/>
    <rdfs:range rdf:resource="` + ontologyNamespace + `E42_Identifier"/>
  </rdf:Property>
  <rdf:Property rdf:about="` + ontologyNamespace + `P2_has_type">
    <rdfs:label xml:lang="en">has type</rdfs:label>
    <rdfs:range rdf:resource="` + ontologyNamespace + `E55_Type"/>
  </rdf:Property>
  <rdf:Property rdf:about="` + ontologyNamespace + `P4_has_time">
    <rdfs:label xml:lang="en">has time-span</rdfs:label>
    <rdfs:domain rdf:resource="` + ontologyNamespace + `E67_Birth"/>
  </rdf:Property>
</rdf:RDF>`

// fixtureCRMv2 is a second, minimal version (2.0) of the same synthetic CRM
// ontology. It declares one class FixtureChild's 1.0 link does not cover
// (E52_Time-Span) plus a property, so linking it to FixtureParent alone makes
// the child's resolved bundle strictly larger than its own-only bundle.
const fixtureCRMv2 = `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:crm="` + ontologyNamespace + `"
         xmlns:owl="http://www.w3.org/2002/07/owl#"
         xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#">
  <owl:Ontology rdf:about="` + ontologyNamespace + `">
    <owl:versionInfo>` + ontologyVersion2 + `</owl:versionInfo>
  </owl:Ontology>

  <rdfs:Class rdf:about="` + ontologyNamespace + `E21_Person">
    <rdfs:label xml:lang="en">Person</rdfs:label>
  </rdfs:Class>
  <rdfs:Class rdf:about="` + ontologyNamespace + `E52_Time-Span">
    <rdfs:label xml:lang="en">Time-Span</rdfs:label>
  </rdfs:Class>

  <rdf:Property rdf:about="` + ontologyNamespace + `P4_has_time-span">
    <rdfs:label xml:lang="en">has time-span</rdfs:label>
    <rdfs:domain rdf:resource="` + ontologyNamespace + `E67_Birth"/>
    <rdfs:range rdf:resource="` + ontologyNamespace + `E52_Time-Span"/>
  </rdf:Property>
</rdf:RDF>`

func main() {
	if err := run(); err != nil {
		slog.Error("fixturegen failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	dsn := flag.String("dsn", envDSN(), "Postgres DSN of a scratch database to seed and materialize from")
	out := flag.String("out", "test/fixtures", "output directory for the generated snapshots (the pletka-fixtures submodule)")
	flag.Parse()

	if *dsn == "" {
		return fmt.Errorf("no DSN: pass -dsn or set FIXTURE_DB_DSN / TEST_DATABASE_URL")
	}

	ctx := context.Background()

	// Migrate the scratch DB via database/sql (goose runs against *sql.DB).
	sqlDB, err := sql.Open("pgx", *dsn)
	if err != nil {
		return fmt.Errorf("open sql db: %w", err)
	}
	// A freshly started postgres container briefly restarts during init, so
	// retry the ping instead of failing on the first refused connection.
	if err := pingWithRetry(ctx, sqlDB, 60, time.Second); err != nil {
		sqlDB.Close()
		return fmt.Errorf("ping scratch db: %w", err)
	}
	if err := database.Migrate(sqlDB); err != nil {
		sqlDB.Close()
		return fmt.Errorf("migrate scratch db: %w", err)
	}
	sqlDB.Close()

	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		return fmt.Errorf("open pgx pool: %w", err)
	}
	defer pool.Close()

	if err := seed(ctx, pool); err != nil {
		return fmt.Errorf("seed fixtures: %w", err)
	}

	outDir, err := filepath.Abs(*out)
	if err != nil {
		return fmt.Errorf("resolve out dir: %w", err)
	}
	for _, id := range []string{fixtureSingleID, fixtureParentID, fixtureChildID} {
		if err := materialize(ctx, pool, outDir, id); err != nil {
			return fmt.Errorf("materialize %s: %w", id, err)
		}
	}
	slog.Info("fixtures generated", "out", outDir,
		"projects", []string{fixtureSingleID, fixtureParentID, fixtureChildID})
	return nil
}

func pingWithRetry(ctx context.Context, db *sql.DB, attempts int, wait time.Duration) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = db.PingContext(ctx); err == nil {
			return nil
		}
		time.Sleep(wait)
	}
	return err
}

func envDSN() string {
	if v := os.Getenv("FIXTURE_DB_DSN"); v != "" {
		return v
	}
	return os.Getenv("TEST_DATABASE_URL")
}

// seed creates the owner actor, imports the synthetic ontology, and builds the
// three fixture projects through the service/store layer.
func seed(ctx context.Context, pool *pgxpool.Pool) error {
	q := sqlcgen.New(pool)

	if _, err := q.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID:          fixtureOwnerID,
		Type:        "organization",
		DisplayName: "Fixture Org",
		Slug:        "fixture-org",
		Role:        "",
		Visibility:  "private",
	}); err != nil {
		return fmt.Errorf("create owner actor: %w", err)
	}

	ontologyID := weaveontology.GenerateOntologyID(ontologySlug)
	versionID := weaveontology.GenerateVersionID(ontologySlug, ontologyVersion)

	rdfDir, err := os.MkdirTemp("", "fixturegen-crm-*")
	if err != nil {
		return fmt.Errorf("temp rdf dir: %w", err)
	}
	defer os.RemoveAll(rdfDir)
	if err := os.WriteFile(filepath.Join(rdfDir, "crm.rdf"), []byte(fixtureCRM), 0o644); err != nil {
		return fmt.Errorf("write rdf fixture: %w", err)
	}
	if err := os.WriteFile(filepath.Join(rdfDir, "crm2.rdf"), []byte(fixtureCRMv2), 0o644); err != nil {
		return fmt.Errorf("write rdf fixture v2: %w", err)
	}

	ontologySvc := weaveontology.NewService(weaveontology.NewPostgresStore(pool), nil, nil)
	if err := ontologySvc.ImportVendoredVersion(ctx, weaveontology.VendoredOntologyImportRequest{
		OntologyID:    ontologyID,
		VersionID:     versionID,
		VersionString: ontologyVersion,
		Slug:          ontologySlug,
		Title:         "Fixture CRM",
		Kind:          "base",
		Namespace:     ontologyNamespace,
		Prefixes:      []string{ontologyPrefix},
		BaseDir:       rdfDir,
		Files:         []string{"crm.rdf"},
	}); err != nil {
		return fmt.Errorf("import vendored ontology: %w", err)
	}

	// Second version of the same ontology, linked to FixtureParent only below.
	versionID2 := weaveontology.GenerateVersionID(ontologySlug, ontologyVersion2)
	if err := ontologySvc.ImportVendoredVersion(ctx, weaveontology.VendoredOntologyImportRequest{
		OntologyID:    ontologyID,
		VersionID:     versionID2,
		VersionString: ontologyVersion2,
		Slug:          ontologySlug,
		Title:         "Fixture CRM",
		Kind:          "base",
		Namespace:     ontologyNamespace,
		Prefixes:      []string{ontologyPrefix},
		BaseDir:       rdfDir,
		Files:         []string{"crm2.rdf"},
	}); err != nil {
		return fmt.Errorf("import vendored ontology v2: %w", err)
	}

	// FixtureSingle — one self-contained project, linked ontology, three fields
	// exercising crm: qnames, plus a display category.
	if err := createProject(ctx, q, fixtureSingleID, "Fixture Single", nil); err != nil {
		return err
	}
	if err := linkOntology(ctx, q, fixtureSingleID, versionID); err != nil {
		return err
	}
	if err := createCategory(ctx, q, fixtureSingleID, "FXSINGLE.CAT.1", "identity", "Identity"); err != nil {
		return err
	}
	if err := createField(ctx, q, fixtureSingleID, "FXSINGLEF.1", "actor_name", "Actor Name",
		classPE("E21_Person"),
		[]domain.PathElement{propPE("P1_is_identified_by", 0), classPEAt("E42_Identifier", 1)}, "text"); err != nil {
		return err
	}
	if err := createField(ctx, q, fixtureSingleID, "FXSINGLEF.2", "actor_type", "Actor Type",
		classPE("E21_Person"),
		[]domain.PathElement{propPE("P2_has_type", 0), classPEAt("E55_Type", 1)}, "reference"); err != nil {
		return err
	}
	if err := createField(ctx, q, fixtureSingleID, "FXSINGLEF.3", "birth_time", "Birth Time",
		classPE("E67_Birth"),
		[]domain.PathElement{propPE("P4_has_time", 0), literalPE(1)}, "date"); err != nil {
		return err
	}

	// FixtureParent — a self-contained parent with the field FixtureChild
	// inherits.
	if err := createProject(ctx, q, fixtureParentID, "Fixture Parent", nil); err != nil {
		return err
	}
	if err := linkOntology(ctx, q, fixtureParentID, versionID); err != nil {
		return err
	}
	// FixtureParent additionally links ontology version 2.0 (secondary), which
	// FixtureChild does not link directly. This is the only ontology version
	// present via inheritance but not ownership, so FixtureChild's resolved
	// bundle is strictly larger than its own-only bundle.
	if err := linkOntologySecondary(ctx, q, fixtureParentID, versionID2); err != nil {
		return err
	}
	if err := createField(ctx, q, fixtureParentID, "FXPARENTF.1", "actor_name", "Actor Name",
		classPE("E21_Person"),
		[]domain.PathElement{propPE("P1_is_identified_by", 0), classPEAt("E42_Identifier", 1)}, "text"); err != nil {
		return err
	}

	// FixtureChild — inherits FixtureParent (primary, draft), links the same
	// ontology, owns one field.
	parent := fixtureParentID
	if err := createProject(ctx, q, fixtureChildID, "Fixture Child", &parent); err != nil {
		return err
	}
	if err := linkOntology(ctx, q, fixtureChildID, versionID); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_project_inheritance (
			project_id, parent_project_id, is_primary, canonical_order, adopted_at, source_mode
		) VALUES ($1, $2, true, 0, NOW(), 'draft')
	`, fixtureChildID, fixtureParentID); err != nil {
		return fmt.Errorf("seed inheritance %s->%s: %w", fixtureChildID, fixtureParentID, err)
	}
	if err := createField(ctx, q, fixtureChildID, "FXCHILDF.1", "child_note", "Child Note",
		classPE("E33_Linguistic_Object"),
		[]domain.PathElement{propPE("P2_has_type", 0), classPEAt("E55_Type", 1)}, "reference"); err != nil {
		return err
	}

	return nil
}

func createProject(ctx context.Context, q *sqlcgen.Queries, id, name string, parentID *string) error {
	uiName, _ := json.Marshal(map[string]string{"en": name})
	desc, _ := json.Marshal(map[string]string{"en": "Synthetic fixture project " + id})
	if _, err := q.WeaveCreateProject(ctx, sqlcgen.WeaveCreateProjectParams{
		ID:              id,
		UiName:          uiName,
		Description:     desc,
		Status:          "draft",
		OwnerID:         fixtureOwnerID,
		Visibility:      "private",
		ParentProjectID: parentID,
	}); err != nil {
		return fmt.Errorf("create project %s: %w", id, err)
	}
	return nil
}

func linkOntology(ctx context.Context, q *sqlcgen.Queries, projectID, versionID string) error {
	isPrimary := true
	notes := ""
	if _, err := q.WeaveCreateProjectOntologyVersion(ctx, sqlcgen.WeaveCreateProjectOntologyVersionParams{
		ProjectID:         projectID,
		OntologyVersionID: versionID,
		IsPrimary:         &isPrimary,
		UsageNotes:        &notes,
	}); err != nil {
		return fmt.Errorf("link ontology to %s: %w", projectID, err)
	}
	return nil
}

func linkOntologySecondary(ctx context.Context, q *sqlcgen.Queries, projectID, versionID string) error {
	isPrimary := false
	notes := ""
	if _, err := q.WeaveCreateProjectOntologyVersion(ctx, sqlcgen.WeaveCreateProjectOntologyVersionParams{
		ProjectID:         projectID,
		OntologyVersionID: versionID,
		IsPrimary:         &isPrimary,
		UsageNotes:        &notes,
	}); err != nil {
		return fmt.Errorf("link secondary ontology to %s: %w", projectID, err)
	}
	return nil
}

func createCategory(ctx context.Context, q *sqlcgen.Queries, projectID, semanticID, systemName, name string) error {
	uiName, _ := json.Marshal(map[string]string{"en": name})
	desc, _ := json.Marshal(map[string]string{"en": name + " fields"})
	sid := semanticID
	sn := systemName
	if _, err := q.WeaveCreateCategory(ctx, sqlcgen.WeaveCreateCategoryParams{
		ID:             ids.GenerateULID(),
		SemanticID:     &sid,
		SystemName:     &sn,
		UiName:         uiName,
		Description:    desc,
		Status:         "draft",
		ProjectID:      projectID,
		CanonicalOrder: 1,
	}); err != nil {
		return fmt.Errorf("create category %s: %w", semanticID, err)
	}
	return nil
}

func createField(ctx context.Context, q *sqlcgen.Queries, projectID, semanticID, systemName, name string, scope domain.PathElement, path []domain.PathElement, evt string) error {
	f := &domain.Field{PathElements: path}
	ontologyPath := f.OntologyPath()

	uiName, _ := json.Marshal(map[string]string{"en": name})
	desc, _ := json.Marshal(map[string]string{"en": name + " field"})
	scopeJSON, err := json.Marshal(scope)
	if err != nil {
		return fmt.Errorf("marshal scope for %s: %w", semanticID, err)
	}
	pathJSON, err := json.Marshal(path)
	if err != nil {
		return fmt.Errorf("marshal path for %s: %w", semanticID, err)
	}

	sid := semanticID
	sn := systemName
	op := ontologyPath
	et := evt
	if _, err := q.WeaveCreateField(ctx, sqlcgen.WeaveCreateFieldParams{
		ID:                ids.GenerateULID(),
		SemanticID:        &sid,
		SystemName:        &sn,
		UiName:            uiName,
		Description:       desc,
		Status:            "published",
		ProjectID:         projectID,
		OntologyScope:     scopeJSON,
		OntologyPath:      &op,
		PathElements:      pathJSON,
		ExpectedValueType: &et,
	}); err != nil {
		return fmt.Errorf("create field %s: %w", semanticID, err)
	}
	return nil
}

func classPE(localName string) domain.PathElement {
	return domain.PathElement{Type: "class", Prefix: ontologyPrefix, LocalName: localName, URI: ontologyPrefix + ":" + localName}
}

func classPEAt(localName string, pos int) domain.PathElement {
	pe := classPE(localName)
	pe.Position = pos
	return pe
}

func propPE(localName string, pos int) domain.PathElement {
	return domain.PathElement{Type: "property", Prefix: ontologyPrefix, LocalName: localName, URI: ontologyPrefix + ":" + localName, Position: pos}
}

func literalPE(pos int) domain.PathElement {
	return domain.PathElement{Type: "literal", Prefix: "rdfs", LocalName: "Literal", Position: pos}
}

// materialize writes a self-contained snapshot of projectID under outDir and
// strips the per-project .git working repo the materializer creates — the
// fixtures submodule tracks the snapshot files themselves, not nested git
// histories, and HydrateProjectSnapshot reads the files directly.
func materialize(ctx context.Context, pool *pgxpool.Pool, outDir, projectID string) error {
	if err := os.RemoveAll(filepath.Join(outDir, projectID)); err != nil {
		return fmt.Errorf("clean %s: %w", projectID, err)
	}
	mat := gitmaterializer.NewMaterializer(pool, outDir, slog.Default())
	if err := mat.InitProjectWithOptions(ctx, projectID, gitmaterializer.InitProjectOptions{SelfContained: true}); err != nil {
		return fmt.Errorf("init self-contained: %w", err)
	}
	if err := os.RemoveAll(filepath.Join(outDir, projectID, ".git")); err != nil {
		return fmt.Errorf("strip nested .git for %s: %w", projectID, err)
	}
	return nil
}
