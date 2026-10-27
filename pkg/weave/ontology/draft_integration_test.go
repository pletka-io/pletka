//go:build integration

package ontology_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database"
	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
)

const draftOwner ontologybody.OntologyID = "EDITOR_ONTOLOGY"
const draftPrimary = `<?xml version="1.0"?><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#" xmlns:owl="http://www.w3.org/2002/07/owl#" xmlns:ed="https://editor.example/">
<owl:Ontology rdf:about="https://editor.example/"/>
<owl:Class rdf:about="https://editor.example/A"><rdfs:label xml:lang="en">Original A</rdfs:label></owl:Class>
<owl:ObjectProperty rdf:about="https://editor.example/p"><rdfs:label xml:lang="en">property</rdfs:label><rdfs:domain rdf:resource="https://editor.example/A"/><rdfs:range rdf:resource="https://editor.example/A"/></owl:ObjectProperty></rdf:RDF>`
const draftCompanion = `<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#" xmlns:owl="http://www.w3.org/2002/07/owl#" xmlns:ed="https://editor.example/"><owl:Class rdf:about="https://editor.example/B"><rdfs:label xml:lang="en">Companion B</rdfs:label><rdfs:subClassOf rdf:resource="https://editor.example/A"/></owl:Class></rdf:RDF>`

type draftTestBus struct {
	t      *testing.T
	pool   *pgxpool.Pool
	events []domain.Event
}

func (b *draftTestBus) Subscribe(string, domain.EventHandler) {}
func (b *draftTestBus) Publish(ctx context.Context, event domain.Event) {
	var committed bool
	if err := b.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM weave_ontology_releases WHERE version_id=$1)`, event.EntityID).Scan(&committed); err != nil || !committed {
		b.t.Errorf("event before commit: %v", err)
	}
	b.events = append(b.events, event)
}
func draftFixture(t *testing.T) (*pgxpool.Pool, ontology.Store, *ontology.DraftService, ontology.DraftDocument, *draftTestBus) {
	t.Helper()
	pool := testdb.EmptyPool(t)
	store := ontology.NewPostgresStore(pool)
	if _, err := store.CreateOntology(t.Context(), ontology.CreateOntologyInput{ID: string(draftOwner), Prefix: "ed", Namespace: "https://editor.example/", Name: "Live catalog", OntologyType: domain.OntologyTypeBase}); err != nil {
		t.Fatal(err)
	}
	document := ontology.DraftDocument{FormatVersion: 1, Ontology: ontology.DraftOntology{Prefix: "ed", Namespace: "https://editor.example/", Name: "Draft catalog", Type: domain.OntologyTypeBase}, Primary: ontology.DraftSource{Filename: "main.rdf", Content: draftPrimary}, Companions: []ontology.DraftSource{{Filename: "companion.rdf", Content: draftCompanion, Description: "Companion"}}}
	bus := &draftTestBus{t: t, pool: pool}
	return pool, store, ontology.NewDraftService(pool, bus), document, bus
}
func draftRef(t *testing.T, snapshot ontology.DraftSnapshot) ontology.DraftReference {
	t.Helper()
	ref, err := ontology.NewDraftReference(snapshot.OntologyID, snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}
func draftArtifact(t *testing.T, pool *pgxpool.Pool, version string) ontologyrelease.Artifact {
	t.Helper()
	ref, err := ontologyrelease.NewReference(draftOwner, ontologybody.VersionID(version))
	if err != nil {
		t.Fatal(err)
	}
	var artifact ontologyrelease.Artifact
	if err := ontologyrelease.NewFactory(pool).Read(t.Context(), ref, func(r ontologyrelease.Reader) error {
		var err error
		artifact, err = r.Artifact(t.Context())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return artifact
}
func saveDraft(t *testing.T, s *ontology.DraftService, snapshot ontology.DraftSnapshot, document ontology.DraftDocument) ontology.DraftSnapshot {
	t.Helper()
	var out ontology.DraftSnapshot
	if err := s.Edit(t.Context(), draftRef(t, snapshot), snapshot.Revision, func(e ontology.DraftEditor) error {
		var err error
		out, err = e.Save(t.Context(), document)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return out
}
func TestOntologyDraftPublicationUsesFreshRowsAndStableArtifacts(t *testing.T) {
	pool, store, s, document, bus := draftFixture(t)
	snapshot, err := s.Start(t.Context(), draftOwner, document)
	if err != nil {
		t.Fatal(err)
	}
	var versions int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM weave_ontology_versions`).Scan(&versions); err != nil || versions != 0 {
		t.Fatalf("draft minted version rows: %d, %v", versions, err)
	}
	live, err := store.GetOntology(t.Context(), string(draftOwner))
	if err != nil || live.Name != "Live catalog" {
		t.Fatalf("draft changed catalog: %+v, %v", live, err)
	}
	first, err := s.Publish(t.Context(), draftRef(t, snapshot), snapshot.Revision, ontology.DraftPublishRequest{Version: "1", SetActive: true})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Created || first.VersionID == string(snapshot.ID) {
		t.Fatalf("draft/release identities conflated: %+v", first)
	}
	original := draftArtifact(t, pool, first.VersionID)
	document.Ontology.Name = "Edited catalog"
	document.Primary.Content = strings.ReplaceAll(document.Primary.Content, "Original A", "Edited A")
	snapshot = saveDraft(t, s, snapshot, document)
	if original != draftArtifact(t, pool, first.VersionID) {
		t.Fatal("editing changed a release")
	}
	second, err := s.Publish(t.Context(), draftRef(t, snapshot), snapshot.Revision, ontology.DraftPublishRequest{Version: "2", SetActive: true})
	if err != nil {
		t.Fatal(err)
	}
	if original != draftArtifact(t, pool, first.VersionID) {
		t.Fatal("later publication changed an old artifact")
	}
	var ids, uris int
	if err := pool.QueryRow(t.Context(), `SELECT count(DISTINCT id),count(DISTINCT uri) FROM weave_ontology_classes WHERE local_name='A'`).Scan(&ids, &uris); err != nil || ids != 2 || uris != 1 {
		t.Fatalf("term identities: ids=%d uris=%d err=%v", ids, uris, err)
	}
	var module string
	if err := pool.QueryRow(t.Context(), `SELECT source_module FROM weave_ontology_classes WHERE ontology_version_id=$1 AND local_name='B'`, second.VersionID).Scan(&module); err != nil || module != "companion.rdf" {
		t.Fatalf("companion ownership: %s, %v", module, err)
	}
	if err := store.SetActiveVersion(t.Context(), string(draftOwner), first.VersionID); err != nil {
		t.Fatal(err)
	}
	replay, err := s.Publish(t.Context(), draftRef(t, snapshot), snapshot.Revision, ontology.DraftPublishRequest{Version: "2", SetActive: true})
	if err != nil || replay.Created || replay.Digest != second.Digest {
		t.Fatalf("publish replay: %+v, %v", replay, err)
	}
	active, err := store.GetActiveVersion(t.Context(), string(draftOwner))
	if err != nil || active.ID != first.VersionID {
		t.Fatalf("replay changed active policy: %+v, %v", active, err)
	}
	if len(bus.events) != 2 {
		t.Fatalf("publication events=%d", len(bus.events))
	}
	if _, err := s.Publish(t.Context(), draftRef(t, snapshot), snapshot.Revision, ontology.DraftPublishRequest{Version: "different"}); apierror.FromError(err).Code != apierror.CodeConflict {
		t.Fatalf("different replay accepted: %v", err)
	}
	var savedName string
	if err := pool.QueryRow(t.Context(), `SELECT document#>>'{ontology,name}' FROM weave_ontology_draft_publications WHERE draft_id=$1 AND revision=1`, string(snapshot.ID)).Scan(&savedName); err != nil || savedName != "Draft catalog" {
		t.Fatalf("publication did not retain its draft: %s, %v", savedName, err)
	}
	for _, statement := range []string{`UPDATE weave_ontology_drafts SET revision=99`, `UPDATE weave_ontology_draft_publications SET digest=repeat('0',64)`, `DELETE FROM weave_ontology_draft_publications`, `DELETE FROM weave_ontology_drafts`, `TRUNCATE weave_ontology_draft_publications`, `UPDATE weave_ontology_classes SET local_name='mutated'`} {
		if _, err := pool.Exec(t.Context(), statement); err == nil {
			t.Fatalf("storage bypass succeeded: %s", statement)
		}
	}
	if _, err := pool.Exec(t.Context(), `UPDATE weave_ontology_active_versions SET version_id=$1`, string(snapshot.ID)); err == nil {
		t.Fatal("draft accepted as active version")
	}
	db := stdlib.OpenDB(*pool.Config().ConnConfig)
	defer db.Close()
	latest, err := database.LatestMigrationVersion()
	if err != nil {
		t.Fatal(err)
	}
	for version := latest; version > 28; version-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.MigrateDown(db); err == nil || !strings.Contains(err.Error(), "cannot downgrade stored ontology drafts") {
		t.Fatalf("downgrade discarded draft: %v", err)
	}
}

func TestOntologyDraftScopesRollbackAndConcurrency(t *testing.T) {
	pool, _, s, document, _ := draftFixture(t)
	snapshot, err := s.Start(t.Context(), draftOwner, document)
	if err != nil {
		t.Fatal(err)
	}
	ref := draftRef(t, snapshot)
	wrongOwner, err := ontology.NewDraftReference("wrong-owner", snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	missing, err := ontology.NewDraftReference(draftOwner, "missing")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []ontology.DraftReference{{}, wrongOwner, missing} {
		entered := false
		err := s.Read(t.Context(), bad, func(ontology.DraftReader) error { entered = true; return nil })
		if err == nil || entered {
			t.Fatal("invalid draft selection reached callback")
		}
	}
	var escaped ontology.DraftReader
	if err := s.Read(t.Context(), ref, func(r ontology.DraftReader) error { escaped = r; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := escaped.Snapshot(t.Context()); err == nil {
		t.Fatal("draft reader escaped")
	}
	sentinel := errors.New("abort edit")
	var escapedEditor ontology.DraftEditor
	err = s.Edit(t.Context(), ref, 1, func(e ontology.DraftEditor) error {
		escapedEditor = e
		if _, err := e.Save(t.Context(), document); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if _, err := escapedEditor.Save(t.Context(), document); err == nil {
		t.Fatal("draft editor escaped")
	}
	current, err := s.Current(t.Context(), draftOwner)
	if err != nil || current.Revision != 1 {
		t.Fatalf("edit rollback failed: %+v, %v", current, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	locked, release := make(chan struct{}), make(chan struct{})
	first, second := make(chan error, 1), make(chan error, 1)
	go func() {
		first <- s.Edit(ctx, ref, 1, func(e ontology.DraftEditor) error {
			close(locked)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
			_, err := e.Save(ctx, document)
			return err
		})
	}()
	<-locked
	secondEntered := false
	go func() {
		second <- s.Edit(ctx, ref, 1, func(ontology.DraftEditor) error { secondEntered = true; return nil })
	}()
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE query LIKE '%LockOntologyDraft%' AND wait_event_type='Lock')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; apierror.FromError(err).Code != apierror.CodeConflict || secondEntered {
		t.Fatalf("stale editor entered: %v", err)
	}
	cfg := pool.Config()
	cfg.MaxConns = 1
	one, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer one.Close()
	canceled, stop := context.WithCancel(t.Context())
	err = ontology.NewDraftService(one, nil).Read(canceled, ref, func(ontology.DraftReader) error { stop(); return canceled.Err() })
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	check, cancelCheck := context.WithTimeout(t.Context(), time.Second)
	defer cancelCheck()
	connection, err := one.Acquire(check)
	if err != nil {
		t.Fatalf("canceled session leaked connection: %v", err)
	}
	connection.Release()
}

func TestOntologyDraftFailuresPreserveDraftAndPriorPolicy(t *testing.T) {
	pool, store, s, document, bus := draftFixture(t)
	priorID := ontology.GenerateVersionID("ed", "existing")
	if _, err := store.CreateVersion(t.Context(), ontology.CreateVersionInput{ID: priorID, OntologyID: string(draftOwner), VersionString: "existing", RDFContent: "legacy unchanged", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	document.Primary.Content = "unfinished XML"
	snapshot, err := s.Start(t.Context(), draftOwner, document)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(t.Context(), draftRef(t, snapshot), 1, ontology.DraftPublishRequest{Version: "bad"}); apierror.FromError(err).Code != apierror.CodeValidation {
		t.Fatalf("unfinished source published: %v", err)
	}
	document.Primary.Content = strings.Replace(draftPrimary, `<owl:Ontology rdf:about="https://editor.example/"/>`, `<owl:Ontology rdf:about="https://editor.example/"><owl:imports rdf:resource="urn:unresolved"/></owl:Ontology>`, 1)
	document.Bindings = []ontology.DraftNamespace{{Prefix: "new_alias", Namespace: "https://new-alias.example/"}}
	snapshot = saveDraft(t, s, snapshot, document)
	if _, err := s.Publish(t.Context(), draftRef(t, snapshot), snapshot.Revision, ontology.DraftPublishRequest{Version: "missing-dependency", SetActive: true}); apierror.FromError(err).Code != apierror.CodeValidation {
		t.Fatalf("missing dependency published: %v", err)
	}
	var leaked int
	if err := pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM weave_ontology_versions WHERE id<>$1)+(SELECT count(*) FROM weave_ontology_releases)+(SELECT count(*) FROM weave_ontology_draft_publications)+(SELECT count(*) FROM weave_namespace_bindings WHERE prefix='new_alias')`, priorID).Scan(&leaked); err != nil || leaked != 0 {
		t.Fatalf("failed publication leaked %d rows: %v", leaked, err)
	}
	live, err := store.GetOntology(t.Context(), string(draftOwner))
	if err != nil || live.Name != "Live catalog" {
		t.Fatalf("failed publication changed descriptor: %+v %v", live, err)
	}
	document.Primary.Content = draftPrimary
	snapshot = saveDraft(t, s, snapshot, document)
	if _, err := s.Publish(t.Context(), draftRef(t, snapshot), snapshot.Revision, ontology.DraftPublishRequest{Version: "existing", SetActive: true}); apierror.FromError(err).Code != apierror.CodeConflict {
		t.Fatalf("existing identity overwritten: %v", err)
	}
	active, err := store.GetActiveVersion(t.Context(), string(draftOwner))
	if err != nil || active.ID != priorID || active.RDFContent != "legacy unchanged" {
		t.Fatalf("failed cut changed legacy content/policy: %+v %v", active, err)
	}
	if len(bus.events) != 0 {
		t.Fatal("failed publication emitted events")
	}
	current, err := s.Current(t.Context(), draftOwner)
	if err != nil || current.Revision != snapshot.Revision {
		t.Fatalf("failed publication consumed draft: %+v %v", current, err)
	}
}

func TestOntologyDraftStartsFromRetainedReleaseWithoutBackfill(t *testing.T) {
	pool, store, s, document, _ := draftFixture(t)
	// A saved global alias must remain global, retaining its manifest provenance.
	if err := store.UpsertNamespaceBindings(t.Context(), []ontology.NamespaceBinding{{Prefix: "shared_alias", Namespace: "https://shared-alias/", Weight: 80, Source: "manifest"}}); err != nil {
		t.Fatal(err)
	}

	parsed, err := ontology.NewImportRDFParser(nil).Parse(strings.NewReader(draftPrimary), "main.rdf", int64(len(draftPrimary)))
	if err != nil {
		t.Fatal(err)
	}
	input := ontology.BuildImportVersionInput(&domain.Ontology{ID: string(draftOwner), Prefix: "ed", Namespace: "https://editor.example/"}, ontology.ImportVersionBuildOptions{VersionID: ontology.GenerateVersionID("ed", "imported"), VersionString: "imported", SetActive: true}, parsed, ontology.BuildImportNamespaceResolver(nil))
	version, err := store.ImportVersion(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	before := draftArtifact(t, pool, version.ID)
	db := stdlib.OpenDB(*pool.Config().ConnConfig)
	defer db.Close()
	latest, err := database.LatestMigrationVersion()
	if err != nil {
		t.Fatal(err)
	}
	for version := latest; version >= 28; version-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Current(t.Context(), draftOwner); apierror.FromError(err).Code != apierror.CodeNotFound {
		t.Fatalf("migration inferred draft from active history: %v", err)
	}
	snapshot, err := s.StartFromRelease(t.Context(), draftOwner, ontologybody.VersionID(version.ID))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.BaseVersionID == nil || *snapshot.BaseVersionID != version.ID || snapshot.Document.Primary.Content != draftPrimary {
		t.Fatal("draft base differs from source artifact")
	}
	if _, err := s.Start(t.Context(), draftOwner, document); apierror.FromError(err).Code != apierror.CodeConflict {
		t.Fatalf("second draft overwrote workspace: %v", err)
	}
	if _, err := store.CreateOntology(t.Context(), ontology.CreateOntologyInput{ID: "FOREIGN_DRAFT", Prefix: "foreign", Namespace: "https://foreign/", Name: "Foreign", OntologyType: domain.OntologyTypeBase}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartFromRelease(t.Context(), "FOREIGN_DRAFT", ontologybody.VersionID(version.ID)); apierror.FromError(err).Code != apierror.CodeNotInRelease {
		t.Fatalf("foreign source release accepted: %v", err)
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO weave_ontology_drafts(id,ontology_id,base_version_id,document) VALUES ('forged','FOREIGN_DRAFT',$1,$2)`, version.ID, data); err == nil {
		t.Fatal("storage accepted foreign draft base")
	}
	if _, err := s.Publish(t.Context(), draftRef(t, snapshot), 1, ontology.DraftPublishRequest{Version: "editor-2"}); err != nil {
		t.Fatal(err)
	}
	var aliasOwner *string
	var aliasSource string
	var aliasWeight int64
	if err := pool.QueryRow(t.Context(), `SELECT ontology_id,source,weight FROM weave_namespace_bindings WHERE prefix='shared_alias'`).Scan(&aliasOwner, &aliasSource, &aliasWeight); err != nil || aliasOwner != nil || aliasSource != "manifest" || aliasWeight != 80 {
		t.Fatalf("fork changed global alias provenance: %v %s %d %v", aliasOwner, aliasSource, aliasWeight, err)
	}
	if before != draftArtifact(t, pool, version.ID) {
		t.Fatal("editing base changed original release")
	}
}

func TestOntologyDraftPublishConflictsWithConcurrentEdit(t *testing.T) {
	pool, _, s, document, bus := draftFixture(t)
	snapshot, err := s.Start(t.Context(), draftOwner, document)
	if err != nil {
		t.Fatal(err)
	}
	ref := draftRef(t, snapshot)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	locked, release := make(chan struct{}), make(chan struct{})
	edited, published := make(chan error, 1), make(chan error, 1)
	document.Ontology.Name = "Concurrent edit"
	go func() {
		edited <- s.Edit(ctx, ref, snapshot.Revision, func(e ontology.DraftEditor) error {
			close(locked)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
			_, err := e.Save(ctx, document)
			return err
		})
	}()
	select {
	case <-locked:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	go func() {
		_, err := s.Publish(ctx, ref, snapshot.Revision, ontology.DraftPublishRequest{Version: "racing", SetActive: true})
		published <- err
	}()
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE query LIKE '%LockOntologyDraft%' AND wait_event_type='Lock')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(release)
	if err := <-edited; err != nil {
		t.Fatal(err)
	}
	if err := <-published; apierror.FromError(err).Code != apierror.CodeConflict {
		t.Fatalf("stale publication: %v", err)
	}
	current, err := s.Current(t.Context(), draftOwner)
	if err != nil || current.Revision != 2 || current.Document.Ontology.Name != "Concurrent edit" {
		t.Fatalf("lost edit: %+v %v", current, err)
	}
	var versions int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM weave_ontology_versions`).Scan(&versions); err != nil || versions != 0 || len(bus.events) != 0 {
		t.Fatalf("stale publication escaped rollback: %d %v", versions, err)
	}
}
