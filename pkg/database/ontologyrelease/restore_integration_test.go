//go:build integration

package ontologyrelease_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database"
	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
)

func TestPortableRestorePreservesExactBytesAndRejectsCollisions(t *testing.T) {
	defaults(t)
	source := candidate(t, "portable_source")
	source.input.Properties = []ontology.ImportProperty{{ID: ontology.GeneratePropertyID(source.input.Version.ID, "https://portable/p"), Prefix: "portable_source", LocalName: "p", URI: "https://portable/p", IsTransitive: true}}
	source.input.Relations = []domain.OntologyRelation{{SourceID: source.input.Properties[0].ID, SourceKind: "property", RelType: "domain", TargetQname: "portable_source:A"}}
	source.publish(t)
	root := source.artifact(t)
	var artifacts []ontologyrelease.Artifact
	if err := ontologyrelease.NewFactory(source.pool).Read(t.Context(), source.ref(t), func(r ontologyrelease.Reader) error {
		artifacts = append(artifacts, root)
		doc, err := root.Decode()
		if err != nil {
			return err
		}
		for _, pin := range doc.Pins {
			a, err := r.Dependency(t.Context(), ontologybody.OntologyID(pin.OntologyID))
			if err != nil {
				return err
			}
			artifacts = append(artifacts, a)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	target := testdb.EmptyPool(t)
	if _, err := target.Exec(t.Context(), `INSERT INTO weave_namespace_bindings(id,prefix,namespace,weight,source,created_at,updated_at) VALUES ('destination-alias','portable_source','https://different.example/',100,'user',now(),now())`); err != nil {
		t.Fatal(err)
	}
	if err := ontologyrelease.Restore(t.Context(), target, artifacts); err != nil {
		t.Fatal(err)
	}
	if err := ontologyrelease.Restore(t.Context(), target, artifacts); err != nil {
		t.Fatalf("verified no-op: %v", err)
	}
	var restored ontologyrelease.Artifact
	if err := ontologyrelease.NewFactory(target).Read(t.Context(), source.ref(t), func(r ontologyrelease.Reader) error {
		var err error
		restored, err = r.Artifact(t.Context())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if restored != root {
		t.Fatal("restore changed exact artifact bytes")
	}
	var namespace string
	if err := target.QueryRow(t.Context(), `SELECT namespace FROM weave_namespace_bindings WHERE id='destination-alias'`).Scan(&namespace); err != nil || namespace != "https://different.example/" {
		t.Fatalf("restore rewrote ambient namespace: %s %v", namespace, err)
	}
	var classes, properties, relations, companions int
	if err := target.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM weave_ontology_classes WHERE ontology_version_id=$1),(SELECT count(*) FROM weave_ontology_properties WHERE ontology_version_id=$1),(SELECT count(*) FROM weave_ontology_relations WHERE ontology_version_id=$1),(SELECT count(*) FROM weave_ontology_version_companions WHERE ontology_version_id=$1)`, source.input.Version.ID).Scan(&classes, &properties, &relations, &companions); err != nil {
		t.Fatal(err)
	}
	if classes != 1 || properties != 1 || relations != 1 || companions != 1 {
		t.Fatalf("lost derived content: %d %d %d %d", classes, properties, relations, companions)
	}
	doc, err := root.Decode()
	if err != nil {
		t.Fatal(err)
	}
	// Even an internally consistent alternate digest for the same ID conflicts.
	doc.BodyJSON = strings.Replace(doc.BodyJSON, "<rdf/>", "<other/>", 1)
	doc.BodyDigest = fmt.Sprintf("%x", sha256.Sum256([]byte(doc.BodyJSON)))
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	different := ontologyrelease.Artifact{JSON: string(data), Digest: fmt.Sprintf("%x", sha256.Sum256(data))}
	err = ontologyrelease.Restore(t.Context(), target, []ontologyrelease.Artifact{different})
	if apierror.FromError(err).Code != apierror.CodeConflict {
		t.Fatalf("mismatched existing ID was accepted: %v", err)
	}
	if _, err := target.Exec(t.Context(), `UPDATE weave_ontology_classes SET local_name='changed' WHERE ontology_version_id=$1`, source.input.Version.ID); err == nil {
		t.Fatal("restored body mutable")
	}
	db := stdlib.OpenDB(*target.Config().ConnConfig)
	defer db.Close()
	latest, err := database.LatestMigrationVersion()
	if err != nil {
		t.Fatal(err)
	}
	for version := latest; version > 26; version-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
	requireFailure(t, database.MigrateDown(db), "cannot downgrade restored portable ontology bodies")

	collision := testdb.EmptyPool(t)
	store := ontology.NewPostgresStore(collision)
	if _, err := store.CreateOntology(t.Context(), ontology.CreateOntologyInput{ID: source.input.Version.OntologyID, Prefix: "portable_source", Namespace: "https://different/", Name: "existing", OntologyType: domain.OntologyTypeBase}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.StageLegacyVersion(t.Context(), source.input); err != nil {
		t.Fatal(err)
	}
	err = ontologyrelease.Restore(t.Context(), collision, artifacts)
	if apierror.FromError(err).Code != apierror.CodeConflict {
		t.Fatalf("unverified existing ID accepted: %v", err)
	}
	var count int
	if err := collision.QueryRow(t.Context(), `SELECT count(*) FROM weave_ontology_releases`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed bundle left publications: %d %v", count, err)
	}
}
