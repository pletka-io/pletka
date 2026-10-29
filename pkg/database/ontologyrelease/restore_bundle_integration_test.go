//go:build integration

package ontologyrelease_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
)

func portableRestoreBundle(t *testing.T, f fixture) []ontologyrelease.Artifact {
	t.Helper()
	root := f.artifact(t)
	doc, err := root.Decode()
	if err != nil {
		t.Fatal(err)
	}
	bundle := make([]ontologyrelease.Artifact, 1, 1+len(doc.Pins))
	bundle[0] = root
	err = ontologyrelease.NewFactory(f.pool).Read(t.Context(), f.ref(t), func(r ontologyrelease.Reader) error {
		for _, pin := range doc.Pins {
			a, err := r.Dependency(t.Context(), ontologybody.OntologyID(pin.OntologyID))
			if err != nil {
				return err
			}
			bundle = append(bundle, a)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func requireRestoredBundle(t *testing.T, pool *pgxpool.Pool, bundle []ontologyrelease.Artifact) {
	t.Helper()
	for _, want := range bundle {
		doc, err := want.Decode()
		if err != nil {
			t.Fatal(err)
		}
		ref, err := ontologyrelease.NewReference(ontologybody.OntologyID(doc.OntologyID), ontologybody.VersionID(doc.VersionID))
		if err != nil {
			t.Fatal(err)
		}
		err = ontologyrelease.NewFactory(pool).Read(t.Context(), ref, func(r ontologyrelease.Reader) error {
			got, err := r.Artifact(t.Context())
			if err != nil {
				return err
			}
			if got != want {
				t.Fatalf("restored artifact %s changed bytes or digest", doc.VersionID)
			}
			for _, pin := range doc.Pins {
				dependency, err := r.Dependency(t.Context(), ontologybody.OntologyID(pin.OntologyID))
				if err != nil {
					return err
				}
				dep, err := dependency.Decode()
				if err != nil {
					return err
				}
				if dep.OntologyID != pin.OntologyID || dep.VersionID != pin.VersionID || dependency.Digest != pin.Digest {
					t.Fatalf("restored dependency retargeted: %+v", pin)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func requireEmptyRestore(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var versions, seals, releases int
	err := pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM weave_ontology_versions), (SELECT count(*) FROM weave_ontology_body_seals), (SELECT count(*) FROM weave_ontology_releases)`).Scan(&versions, &seals, &releases)
	if err != nil || versions != 0 || seals != 0 || releases != 0 {
		t.Fatalf("failed restore left versions/seals/releases: %d/%d/%d, %v", versions, seals, releases, err)
	}
}

func TestPortableRestoreIndependentOwnerVersions(t *testing.T) {
	defaults(t)
	first := candidate(t, "restore_bundle_versions")
	first.publish(t)
	second := first
	second.input.Version.ID = ontology.GenerateVersionID("restore_bundle_versions", "2")
	second.input.Version.VersionString = "2"
	second.input.Version.VersionIRI += "/2"
	second.input.Version.RDFContent = "<second/>"
	second.input.Classes = append([]ontology.ImportClass(nil), first.input.Classes...)
	second.input.Classes[0].ID = ontology.GenerateClassID(second.input.Version.ID, second.input.Classes[0].URI)
	second.publish(t)
	// Both roots share an owner, but keep their own bodies and the exact common
	// defaults pin. The duplicated defaults artifact must be harmless.
	bundle := append(portableRestoreBundle(t, second), portableRestoreBundle(t, first)...)
	different := first.artifact(t)
	different.JSON += "\n"
	different.Digest = fmt.Sprintf("%x", sha256.Sum256([]byte(different.JSON)))

	t.Run("restore and retry exact publications", func(t *testing.T) {
		target := testdb.EmptyPool(t)
		for range 2 {
			if err := ontologyrelease.Restore(t.Context(), target, bundle); err != nil {
				t.Fatal(err)
			}
			requireRestoredBundle(t, target, bundle)
		}
		err := ontologyrelease.Restore(t.Context(), target, []ontologyrelease.Artifact{second.artifact(t), different})
		if apierror.FromError(err).Code != apierror.CodeConflict {
			t.Fatalf("different bytes for an existing ID were accepted: %v", err)
		}
		requireRestoredBundle(t, target, bundle)
	})
	t.Run("same ID different bytes within bundle", func(t *testing.T) {
		target := testdb.EmptyPool(t)
		conflicting := append([]ontologyrelease.Artifact{different}, bundle...)
		requireFailure(t, ontologyrelease.Restore(t.Context(), target, conflicting), "conflicting artifact bytes")
		requireEmptyRestore(t, target)
	})
	t.Run("defaults absent from bundle and storage", func(t *testing.T) {
		target := testdb.EmptyPool(t)
		err := ontologyrelease.Restore(t.Context(), target, []ontologyrelease.Artifact{first.artifact(t), second.artifact(t)})
		requireFailure(t, err, "ontology publication omits its defaults decision")
		requireEmptyRestore(t, target)
	})
	t.Run("dependencies already restored", func(t *testing.T) {
		target := testdb.EmptyPool(t)
		if err := ontologyrelease.Restore(t.Context(), target, portableRestoreBundle(t, first)); err != nil {
			t.Fatal(err)
		}
		if err := ontologyrelease.Restore(t.Context(), target, []ontologyrelease.Artifact{second.artifact(t)}); err != nil {
			t.Fatal(err)
		}
		requireRestoredBundle(t, target, bundle)
	})
}

func TestPortableRestoreRejectsConflictingTransitiveClosure(t *testing.T) {
	defaults(t)
	first := candidate(t, "restore_closure_versions")
	first.publish(t)
	second := first
	second.input.Version.ID = ontology.GenerateVersionID("restore_closure_versions", "2")
	second.input.Version.VersionString = "2"
	second.input.Version.VersionIRI += "/2"
	second.input.Classes = nil
	second.publish(t)
	child := candidate(t, "restore_closure_child")
	child.input.Publication.Pins = map[string]string{first.input.Version.OntologyID: first.input.Version.ID}
	child.publish(t)
	root := candidate(t, "restore_closure_root")
	root.input.Publication.Pins = map[string]string{child.input.Version.OntologyID: child.input.Version.ID}
	root.publish(t)
	bundle := portableRestoreBundle(t, root)
	doc, err := bundle[0].Decode()
	if err != nil {
		t.Fatal(err)
	}
	changed := false
	for i, pin := range doc.Pins {
		if pin.OntologyID == first.input.Version.OntologyID {
			doc.Pins[i].VersionID = second.input.Version.ID
			doc.Pins[i].Digest = second.artifact(t).Digest
			changed = true
		}
	}
	if !changed {
		t.Fatal("fixture did not capture the transitive dependency")
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	bundle[0] = ontologyrelease.Artifact{JSON: string(data), Digest: fmt.Sprintf("%x", sha256.Sum256(data))}
	if _, err := bundle[0].Decode(); err != nil {
		t.Fatalf("fixture must pass decoding before storage checks its closure: %v", err)
	}
	bundle = append(bundle, second.artifact(t))
	target := testdb.EmptyPool(t)
	// Every in-bundle pin has the right owner and digest, but root now selects
	// version 2 while its child still requires version 1 of that same owner.
	err = ontologyrelease.Restore(t.Context(), target, bundle)
	requireFailure(t, err, "ontology publication omits or conflicts with source closure")
	requireEmptyRestore(t, target)
}
