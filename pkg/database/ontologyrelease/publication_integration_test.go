//go:build integration

package ontologyrelease_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database"
	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
)

func TestMain(m *testing.M) { os.Exit(testdb.Setup(m)) }

type fixture struct {
	pool  *pgxpool.Pool
	store ontology.Store
	input ontology.ImportVersionInput
}

func candidate(t *testing.T, name string) fixture {
	t.Helper()
	pool := testdb.Pool(t)
	store := ontology.NewPostgresStore(pool)
	owner, version := ontology.GenerateOntologyID(name), ontology.GenerateVersionID(name, "1")
	ns := "https://example.org/" + name + "/"
	_, err := store.CreateOntology(t.Context(), ontology.CreateOntologyInput{ID: owner, Prefix: name, Namespace: ns, Name: name, OntologyType: domain.OntologyTypeBase})
	if err != nil {
		t.Fatal(err)
	}
	return fixture{pool: pool, store: store, input: ontology.ImportVersionInput{
		Version:           ontology.CreateVersionInput{ID: version, OntologyID: owner, VersionString: "1", OntologyURI: ns, VersionIRI: ns + "1", RDFContent: "<rdf/>", OriginalFilename: "main.rdf"},
		SetActive:         true,
		Classes:           []ontology.ImportClass{{ID: ontology.GenerateClassID(version, ns+"A"), Prefix: name, LocalName: "A", URI: ns + "A"}},
		NamespaceBindings: []ontology.NamespaceBinding{{Prefix: name, Namespace: ns}},
		CompanionFiles:    []ontology.CompanionFileInput{{Filename: "extra.rdf", Content: "<extra/>"}},
	}}
}
func defaults(t *testing.T) string {
	t.Helper()
	pool := testdb.Pool(t)
	store := ontology.NewPostgresStore(pool)
	ont, err := store.GetOntologyByPrefix(t.Context(), "defaults")
	if err != nil {
		t.Fatal(err)
	}
	const version = "publication-defaults-1"
	if v, err := store.GetVersion(t.Context(), version); err == nil && v != nil {
		if err := store.SetActiveVersion(t.Context(), ont.ID, version); err != nil {
			t.Fatal(err)
		}
		return ont.ID
	}
	in := ontology.ImportVersionInput{Version: ontology.CreateVersionInput{ID: version, OntologyID: ont.ID, VersionString: "publication-1", RDFContent: "<defaults/>"}, SetActive: true}
	if _, err := ontology.NewService(store, nil, nil).ImportVersion(t.Context(), in); err != nil {
		t.Fatal(err)
	}
	return ont.ID
}
func (f fixture) publish(t *testing.T) {
	t.Helper()
	if _, err := ontology.NewService(f.store, nil, nil).ImportVersion(t.Context(), f.input); err != nil {
		t.Fatal(err)
	}
}
func (f fixture) ref(t *testing.T) ontologyrelease.Reference {
	t.Helper()
	ref, err := ontologyrelease.NewReference(ontologybody.OntologyID(f.input.Version.OntologyID), ontologybody.VersionID(f.input.Version.ID))
	if err != nil {
		t.Fatal(err)
	}
	return ref
}
func (f fixture) artifact(t *testing.T) ontologyrelease.Artifact {
	t.Helper()
	var a ontologyrelease.Artifact
	err := ontologyrelease.NewFactory(f.pool).Read(t.Context(), f.ref(t), func(r ontologyrelease.Reader) error { var err error; a, err = r.Artifact(t.Context()); return err })
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func (f fixture) extend(t *testing.T, base fixture) {
	t.Helper()
	_, err := f.pool.Exec(t.Context(), `UPDATE weave_ontologies SET extends_ontology_id=$1 WHERE id=$2`, base.input.Version.OntologyID, f.input.Version.OntologyID)
	if err != nil {
		t.Fatal(err)
	}
}
func requireFailure(t *testing.T, err error, contains string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), contains) {
		t.Fatalf("wanted %q, got %v", contains, err)
	}
}

func TestImportPublishesClosureAndRetainsIdentity(t *testing.T) {
	defaultsID := defaults(t)
	base := candidate(t, "publication_base")
	base.publish(t)
	child := candidate(t, "publication_child")
	child.extend(t, base)
	child.input.Version.CompatibleBaseVersions = []string{"1"}
	child.input.Version.ImportedOntologies = []string{base.input.Version.VersionIRI}
	child.input.Relations = []domain.OntologyRelation{{SourceID: child.input.Classes[0].ID, SourceKind: "class", RelType: "subclass_of", TargetQname: "publication_base:A"}}
	child.publish(t)
	before := child.artifact(t)
	doc, err := before.Decode()
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Pins) != 2 {
		t.Fatalf("wanted base and defaults: %+v", doc.Pins)
	}
	var escaped ontologyrelease.Reader
	err = ontologyrelease.NewFactory(child.pool).Read(t.Context(), child.ref(t), func(r ontologyrelease.Reader) error {
		escaped = r
		for _, owner := range []string{base.input.Version.OntologyID, defaultsID} {
			a, err := r.Dependency(t.Context(), ontologybody.OntologyID(owner))
			if err != nil {
				return err
			}
			if _, err = a.Decode(); err != nil {
				return err
			}
		}
		_, err := r.Dependency(t.Context(), "uncaptured")
		if apierror.FromError(err).Code != apierror.CodeNotInRelease {
			t.Fatalf("unexpected dependency error: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = escaped.Artifact(t.Context())
	requireFailure(t, err, "closed")
	// New publications and live catalog, aliases and active flags cannot retarget
	// the captured body, including its source/companion bytes and descriptors.
	next := base
	next.input.Version.ID = ontology.GenerateVersionID("publication_base", "2")
	next.input.Version.VersionString = "2"
	next.input.Classes = nil
	next.publish(t)
	_, err = child.pool.Exec(t.Context(), `UPDATE weave_ontologies SET name='changed' WHERE id=$1`, base.input.Version.OntologyID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = child.pool.Exec(t.Context(), `UPDATE weave_namespace_bindings SET namespace='https://changed/' WHERE prefix='publication_base'`)
	if err != nil {
		t.Fatal(err)
	}
	if after := child.artifact(t); after != before {
		t.Fatal("policy/catalog edit changed artifact")
	}
	for _, sql := range []string{
		`UPDATE weave_ontology_releases SET artifact='{}' WHERE version_id=$1`,
		`DELETE FROM weave_ontology_releases WHERE version_id=$1`,
		`DELETE FROM weave_ontology_release_dependencies WHERE consumer_version_id=$1`,
		`DELETE FROM weave_ontology_versions WHERE id=$1`,
		`UPDATE weave_ontology_versions SET rdf_content='changed' WHERE id=$1`,
	} {
		if _, err := child.pool.Exec(t.Context(), sql, child.input.Version.ID); err == nil {
			t.Fatalf("mutation succeeded: %s", sql)
		}
	}
	for _, table := range []string{"weave_ontology_releases", "weave_ontology_release_dependencies"} {
		if _, err := child.pool.Exec(t.Context(), "TRUNCATE "+table+" CASCADE"); err == nil {
			t.Fatalf("truncate succeeded: %s", table)
		}
	}
	if err := base.store.DeleteVersion(t.Context(), base.input.Version.ID); err == nil {
		t.Fatal("source deletion succeeded")
	}
	if _, err := child.store.ImportVersion(t.Context(), child.input); apierror.FromError(err).Code != apierror.CodeConflict {
		t.Fatalf("force bypassed publication: %v", err)
	}
	corrupt := before
	corrupt.JSON += " "
	if _, err := corrupt.Decode(); err == nil {
		t.Fatal("tampering passed")
	}
}

func TestFailedPublicationRollsBackBodyAliasesAndActive(t *testing.T) {
	defaults(t)
	f := candidate(t, "publication_rollback")
	f.input.Version.ImportedOntologies = []string{"https://missing.example/import"}
	bus := domain.NewSimpleEventBus()
	events := 0
	bus.Subscribe(domain.EventOntologyVersionImported, func(context.Context, domain.Event) { events++ })
	svc := ontology.NewService(f.store, nil, nil).WireEventBus(bus)
	_, err := svc.ImportVersion(t.Context(), f.input)
	requireFailure(t, err, "unresolved")
	if events != 0 {
		t.Fatal("failed publication emitted an event")
	}
	if apierror.FromError(err).Code != apierror.CodeValidation {
		t.Fatalf("wrong error mapping: %v", err)
	}
	for _, sql := range []string{
		`SELECT count(*) FROM weave_ontology_versions WHERE id=$1`,
		`SELECT count(*) FROM weave_ontology_classes WHERE ontology_version_id=$1`,
		`SELECT count(*) FROM weave_ontology_body_seals WHERE version_id=$1`,
		`SELECT count(*) FROM weave_ontology_releases WHERE version_id=$1`,
	} {
		var n int
		if err := f.pool.QueryRow(t.Context(), sql, f.input.Version.ID).Scan(&n); err != nil || n != 0 {
			t.Fatalf("partial import: %d %v", n, err)
		}
	}
	var n int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM weave_namespace_bindings WHERE prefix='publication_rollback'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("alias leaked: %d %v", n, err)
	}
	f.input.Publication.ExternalImports = f.input.Version.ImportedOntologies
	if _, err := svc.ImportVersion(t.Context(), f.input); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("published import emitted %d events", events)
	}
	doc, err := f.artifact(t).Decode()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range doc.Requirements {
		if r.Kind == "import" && r.Disposition == "external" {
			found = true
		}
	}
	if !found {
		t.Fatal("external classification not preserved")
	}
}

func TestDefaultsChangeConflictsUntilExplicitlyPinned(t *testing.T) {
	owner := defaults(t)
	base := candidate(t, "publication_defaults_base")
	base.publish(t)
	newer := ontology.ImportVersionInput{Version: ontology.CreateVersionInput{ID: ontology.GenerateVersionID("defaults", "publication-2"), OntologyID: owner, VersionString: "publication-2", RDFContent: "<new-defaults/>"}, SetActive: true}
	if _, err := base.store.ImportVersion(t.Context(), newer); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := base.store.SetActiveVersion(context.Background(), owner, "publication-defaults-1"); err != nil {
			t.Error(err)
		}
	})
	child := candidate(t, "publication_defaults_child")
	child.extend(t, base)
	_, err := child.store.ImportVersion(t.Context(), child.input)
	requireFailure(t, err, "conflicting ontology dependency versions")
	child.input.Publication.Pins = map[string]string{owner: "publication-defaults-1"}
	child.publish(t)
	doc, err := child.artifact(t).Decode()
	if err != nil {
		t.Fatal(err)
	}
	for _, pin := range doc.Pins {
		if pin.OntologyID == owner && pin.VersionID != "publication-defaults-1" {
			t.Fatal("defaults pin drifted")
		}
	}
}

func TestExplicitPublicationRollbackAndSnapshot(t *testing.T) {
	defaults(t)
	base := candidate(t, "publication_snapshot_base")
	base.publish(t)
	newer := base
	newer.input.Version.ID = ontology.GenerateVersionID("publication_snapshot_base", "2")
	newer.input.Version.VersionString = "2"
	newer.input.Classes = nil
	newer.input.SetActive = false
	newer.publish(t)
	child := candidate(t, "publication_snapshot_child")
	child.extend(t, base)
	if _, err := child.store.StageLegacyVersion(t.Context(), child.input); err != nil {
		t.Fatal(err)
	}
	bodyRef, err := ontologybody.NewReference(ontologybody.OntologyID(child.input.Version.OntologyID), ontologybody.VersionID(child.input.Version.ID))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := child.pool.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck
	q := sqlcgen.New(tx)
	if err := ontologybody.Seal(t.Context(), q, bodyRef); err != nil {
		t.Fatal(err)
	}
	if err := base.store.SetActiveVersion(t.Context(), base.input.Version.OntologyID, newer.input.Version.ID); err != nil {
		t.Fatal(err)
	}
	if err := ontologyrelease.Capture(t.Context(), q, bodyRef, ontologyrelease.Options{}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	doc, err := child.artifact(t).Decode()
	if err != nil {
		t.Fatal(err)
	}
	for _, pin := range doc.Pins {
		if pin.OntologyID == base.input.Version.OntologyID && pin.VersionID != base.input.Version.ID {
			t.Fatal("publication mixed snapshots")
		}
	}

	legacy := candidate(t, "publication_operator")
	legacy.input.Version.ImportedOntologies = []string{"https://explicit.example/external"}
	if _, err := legacy.store.StageLegacyVersion(t.Context(), legacy.input); err != nil {
		t.Fatal(err)
	}
	ref, err := ontologybody.NewReference(ontologybody.OntologyID(legacy.input.Version.OntologyID), ontologybody.VersionID(legacy.input.Version.ID))
	if err != nil {
		t.Fatal(err)
	}
	_, err = ontologyrelease.PublishCurrent(t.Context(), legacy.pool, ref, ontologyrelease.Options{})
	requireFailure(t, err, "unresolved")
	var sealed bool
	if err := legacy.pool.QueryRow(t.Context(), `SELECT body_sealed_at IS NOT NULL FROM weave_ontology_versions WHERE id=$1`, legacy.input.Version.ID).Scan(&sealed); err != nil || sealed {
		t.Fatalf("failed explicit publication left seal: %v %v", sealed, err)
	}
	a, err := ontologyrelease.PublishCurrent(t.Context(), legacy.pool, ref, ontologyrelease.Options{ExternalImports: legacy.input.Version.ImportedOntologies})
	if err != nil {
		t.Fatal(err)
	}
	if a != legacy.artifact(t) {
		t.Fatal("publication receipt differs from stored artifact")
	}
	_, err = ontologyrelease.PublishCurrent(t.Context(), legacy.pool, ref, ontologyrelease.Options{})
	requireFailure(t, err, "already published")

	db := stdlib.OpenDB(*legacy.pool.Config().ConnConfig)
	defer db.Close()
	t.Cleanup(func() {
		db := stdlib.OpenDB(*legacy.pool.Config().ConnConfig)
		defer db.Close()
		if err := database.Migrate(db); err != nil {
			t.Error(err)
		}
	})
	latest, err := database.LatestMigrationVersion()
	if err != nil {
		t.Fatal(err)
	}
	for version := latest; version > 24; version-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.MigrateDown(db); err == nil || !strings.Contains(err.Error(), "cannot downgrade published ontology artifacts") {
		t.Fatalf("unsafe downgrade: %v", err)
	}
}

func TestRawPublicationCannotOmitRequirementsOrClosure(t *testing.T) {
	defaults(t)
	grand := candidate(t, "publication_raw_grand")
	grand.publish(t)
	base := candidate(t, "publication_raw_base")
	base.extend(t, grand)
	base.publish(t)
	child := candidate(t, "publication_raw_child")
	child.extend(t, base)
	if _, err := child.store.StageLegacyVersion(t.Context(), child.input); err != nil {
		t.Fatal(err)
	}
	ref, err := ontologybody.NewReference(ontologybody.OntologyID(child.input.Version.OntologyID), ontologybody.VersionID(child.input.Version.ID))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := child.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck
	if err := ontologybody.Seal(t.Context(), sqlcgen.New(tx), ref); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	var body ontologybody.Body
	if err := ontologybody.NewFactory(child.pool).Read(t.Context(), ref, func(r ontologybody.Reader) error { var err error; body, err = r.Body(t.Context()); return err }); err != nil {
		t.Fatal(err)
	}
	baseArtifact := base.artifact(t)
	baseDoc, err := baseArtifact.Decode()
	if err != nil {
		t.Fatal(err)
	}
	pins := append([]ontologyrelease.Pin{{OntologyID: baseDoc.OntologyID, VersionID: baseDoc.VersionID, Digest: baseArtifact.Digest}}, baseDoc.Pins...)
	requirements := []ontologyrelease.Requirement{{Kind: "base", Name: baseDoc.OntologyID, Disposition: "pinned", OntologyID: baseDoc.OntologyID}}
	for _, r := range baseDoc.Requirements {
		if r.Kind == "defaults" {
			requirements = append(requirements, r)
		}
	}
	doc := ontologyrelease.Document{FormatVersion: 1, OntologyID: child.input.Version.OntologyID, VersionID: child.input.Version.ID, BodyJSON: body.CanonicalJSON, BodyDigest: body.Digest, Pins: pins, Requirements: requirements}
	insert := func(d ontologyrelease.Document) error {
		data, err := json.Marshal(d)
		if err != nil {
			return err
		}
		tx, err := child.pool.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
		if err != nil {
			return err
		}
		defer tx.Rollback(context.Background()) //nolint:errcheck
		if _, err = tx.Exec(t.Context(), `INSERT INTO weave_ontology_releases(ontology_id,version_id,artifact,digest) VALUES ($1,$2,$3,'ignored')`, d.OntologyID, d.VersionID, string(data)); err != nil {
			return err
		}
		return tx.Commit(t.Context())
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	_, err = child.pool.Exec(t.Context(), `INSERT INTO weave_ontology_releases(ontology_id,version_id,artifact,digest) VALUES ($1,$2,$3,'ignored')`, doc.OntologyID, doc.VersionID, string(data))
	requireFailure(t, err, "repeatable-read")
	bad := doc
	bad.BodyJSON = "{}"
	requireFailure(t, insert(bad), "sealed body")
	bad = doc
	bad.Requirements = []ontologyrelease.Requirement{}
	requireFailure(t, insert(bad), "base requirement")
	bad = doc
	bad.Pins = nil
	for _, pin := range pins {
		if pin.OntologyID != grand.input.Version.OntologyID {
			bad.Pins = append(bad.Pins, pin)
		}
	}
	requireFailure(t, insert(bad), "source closure")
	bad = doc
	bad.Requirements = append([]ontologyrelease.Requirement{}, requirements...)
	for i, r := range bad.Requirements {
		if r.Kind == "defaults" {
			bad.Requirements[i] = ontologyrelease.Requirement{Kind: "defaults", Name: "defaults", Disposition: "absent"}
		}
	}
	requireFailure(t, insert(bad), "defaults decision")
	bad = doc
	bad.Pins = append(append([]ontologyrelease.Pin{}, pins...), pins[0])
	if err := insert(bad); err == nil {
		t.Fatal("duplicate closure owner accepted")
	}
	if _, err := ontologyrelease.PublishCurrent(t.Context(), child.pool, ref, ontologyrelease.Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := child.pool.Exec(t.Context(), `INSERT INTO weave_ontology_release_dependencies(consumer_version_id,ontology_id,version_id,digest) VALUES ($1,'wrong','wrong','wrong')`, child.input.Version.ID); err == nil {
		t.Fatal("forged retained pin accepted")
	}
}

func TestReferencesAndCompatibilityFailClosed(t *testing.T) {
	defaults(t)
	base := candidate(t, "publication_references_base")
	base.publish(t)
	f := candidate(t, "publication_references")
	f.extend(t, base)
	f.input.Version.CompatibleBaseVersions = []string{"99"}
	_, err := f.store.ImportVersion(t.Context(), f.input)
	requireFailure(t, err, "incompatible")
	f.input.Version.CompatibleBaseVersions = []string{"1"}
	f.input.Relations = []domain.OntologyRelation{{SourceID: f.input.Classes[0].ID, SourceKind: "class", RelType: "subclass_of", TargetQname: "publication_references_base:Missing"}}
	_, err = f.store.ImportVersion(t.Context(), f.input)
	requireFailure(t, err, "absent from the publication closure")
	f.input.Publication.ExternalReferences = []string{"publication_references_base:Missing"}
	f.publish(t)
	// A relation alone also requires a concrete source pin, even when a mutable
	// catalog/term lookup could locate the requested term.
	extra := candidate(t, "publication_explicit")
	extra.input.Relations = []domain.OntologyRelation{{SourceID: extra.input.Classes[0].ID, SourceKind: "class", RelType: "subclass_of", TargetQname: "publication_references_base:A"}}
	_, err = extra.store.ImportVersion(t.Context(), extra.input)
	requireFailure(t, err, "provide an ontology pin")
	extra.input.Publication.Pins = map[string]string{base.input.Version.OntologyID: base.input.Version.ID}
	extra.publish(t)
}

func TestClosureConflictsCyclesAndUnpublishedInputs(t *testing.T) {
	defaults(t)
	base := candidate(t, "publication_conflict_base")
	base.publish(t)
	a := candidate(t, "publication_conflict_a")
	a.extend(t, base)
	a.publish(t)
	newer := base
	newer.input.Version.ID = ontology.GenerateVersionID("publication_conflict_base", "2")
	newer.input.Version.VersionString = "2"
	newer.input.Classes = nil
	newer.publish(t)
	b := candidate(t, "publication_conflict_b")
	b.extend(t, base)
	b.input.Publication.Pins = map[string]string{a.input.Version.OntologyID: a.input.Version.ID}
	_, err := b.store.ImportVersion(t.Context(), b.input)
	requireFailure(t, err, "conflicting ontology dependency versions")
	// A new base release cannot depend on A, whose closure contains this owner.
	cyclic := base
	cyclic.input.Version.ID = ontology.GenerateVersionID("publication_conflict_base", "cycle")
	cyclic.input.Version.VersionString = "cycle"
	cyclic.input.Classes = nil
	cyclic.input.Publication.Pins = map[string]string{a.input.Version.OntologyID: a.input.Version.ID}
	_, err = cyclic.store.ImportVersion(t.Context(), cyclic.input)
	requireFailure(t, err, "cyclic")
	legacy := candidate(t, "publication_unverified")
	if _, err := legacy.store.StageLegacyVersion(t.Context(), legacy.input); err != nil {
		t.Fatal(err)
	}
	consumer := candidate(t, "publication_unverified_consumer")
	consumer.extend(t, legacy)
	_, err = consumer.store.ImportVersion(t.Context(), consumer.input)
	requireFailure(t, err, "must be published")
	entered := false
	err = ontologyrelease.NewFactory(legacy.pool).Read(t.Context(), legacy.ref(t), func(ontologyrelease.Reader) error { entered = true; return nil })
	if entered || apierror.FromError(err).Code != apierror.CodeNotInRelease {
		t.Fatalf("unpublished reader entered: %v", err)
	}
}

func TestPublicationReaderOwnershipCancellationAndStorageForgery(t *testing.T) {
	defaults(t)
	f := candidate(t, "publication_reader")
	f.publish(t)
	wrong, err := ontologyrelease.NewReference("wrong-owner", ontologybody.VersionID(f.input.Version.ID))
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []ontologyrelease.Reference{wrong, {}} {
		entered := false
		err := ontologyrelease.NewFactory(f.pool).Read(t.Context(), ref, func(ontologyrelease.Reader) error { entered = true; return nil })
		if err == nil || entered {
			t.Fatalf("invalid reader entered: %v", err)
		}
	}
	config := f.pool.Config()
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, cancel := context.WithCancel(t.Context())
	err = ontologyrelease.NewFactory(pool).Read(ctx, f.ref(t), func(ontologyrelease.Reader) error { cancel(); return ctx.Err() })
	if err == nil {
		t.Fatal("cancellation lost")
	}
	if err := pool.Ping(t.Context()); err != nil {
		t.Fatal(err)
	}
	// Supplying a forged body through raw SQL is rejected by storage, independent
	// of the publication API. Duplicate/different source pins are also rejected.
	staged := candidate(t, "publication_forged")
	if _, err := staged.store.StageLegacyVersion(t.Context(), staged.input); err != nil {
		t.Fatal(err)
	}
	artifact := f.artifact(t)
	var doc map[string]any
	if err := json.Unmarshal([]byte(artifact.JSON), &doc); err != nil {
		t.Fatal(err)
	}
	doc["ontology_id"] = staged.input.Version.OntologyID
	doc["version_id"] = staged.input.Version.ID
	bytes, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO weave_ontology_releases(ontology_id,version_id,artifact,digest) VALUES ($1,$2,$3,'forged')`, staged.input.Version.OntologyID, staged.input.Version.ID, string(bytes)); err == nil {
		t.Fatal("forged publication accepted")
	}
}
