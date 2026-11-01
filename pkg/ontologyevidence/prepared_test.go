package ontologyevidence

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestPreparedInterpretationReusesExactInputsAndDetachesCopies(t *testing.T) {
	source := artifact(t, restoreContext(t, "source"))
	d := restoreContext(t, "root")
	d.Sources = []Source{{JSON: source.JSON, Digest: source.Digest}}
	d.SourceRefs = []ContextReference{{ProjectID: "source", Version: "1", Digest: source.Digest}}
	d.Elements[0].EntityID = "local"
	d.Assessments[0].EntityID = "local"
	d.Dependencies = []Dependency{{ProjectID: "root", Version: "1", EntityKind: "field", EntityID: "f", SourceProject: "source", SourceVersion: "1"}}
	a := artifact(t, d)
	var p Preparer
	first, err := p.Prepare(a)
	if err != nil {
		t.Fatal(err)
	}
	again, err := p.Prepare(a)
	if err != nil || first.value != again.value {
		t.Fatal("identical artifact was not reused")
	}
	child, err := p.Prepare(source)
	if err != nil {
		t.Fatal(err)
	}
	if first.value.contexts[child.value.root] != child.value.contexts[child.value.root] || len(p.documents) != 2 || len(p.bodies) != 2 {
		t.Fatal("shared context/body was decoded separately")
	}
	if err := ValidatePreparedBundle([]Prepared{first, child}); err != nil {
		t.Fatal(err)
	}
	want, err := first.Document()
	if err != nil {
		t.Fatal(err)
	}
	copy, err := first.Document()
	if err != nil {
		t.Fatal(err)
	}
	copy.Inventory[0].Selection[0] = '!'
	copy.Inventory[0].BodyJSON = "forged"
	copy.Elements[0].Original[0] = '!'
	copy.Assessments[0].Candidates[0].URI = "forged"
	copy.Sources[0].JSON = "forged"
	copy.SourceRefs[0].Digest = "forged"
	copy.Dependencies[0].SourceVersion = "forged"
	got, _ := again.Document()
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal(diff)
	}
	// The capability is independent of the preparer's lifetime and later inputs.
	p = Preparer{}
	r, err := first.Resolver()
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Resolve("field", "f", "/path_elements/0")
	var unresolved *UnresolvedError
	if !errors.As(err, &unresolved) || unresolved.Status != Ambiguous {
		t.Fatalf("source interpretation changed: %v", err)
	}
	stored, err := first.Artifact()
	if err != nil || stored != a {
		t.Fatal("exact original bytes changed")
	}
}

func TestPreparedCacheCannotValidateForgedOrIncompleteInputs(t *testing.T) {
	d := valueFixture(t)
	a := artifact(t, d)
	var p Preparer
	if _, err := p.Prepare(a); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Prepare(Artifact{JSON: a.JSON + " ", Digest: a.Digest}); err == nil {
		t.Fatal("old digest accepted edited bytes")
	}
	d.Inventory[0].OntologyID = "other"
	if _, err := p.Prepare(artifact(t, d)); err == nil {
		t.Fatal("cached body bypassed selected identity")
	}
	d = valueFixture(t)
	d.SourceRefs = []ContextReference{{ProjectID: "absent", Version: "1", Digest: a.Digest}}
	for range 2 {
		if _, err := p.Prepare(artifact(t, d)); err == nil {
			t.Fatal("partial local validation bypassed missing closure")
		}
	}
	if err := ValidatePreparedBundle([]Prepared{{}}); err == nil {
		t.Fatal("zero prepared bundle accepted")
	}
	if _, err := (Prepared{}).Document(); err == nil {
		t.Fatal("zero document accepted")
	}
	if _, err := (Prepared{}).Reference(); err == nil {
		t.Fatal("zero reference accepted")
	}
	if _, err := (Prepared{}).Resolver(); err == nil {
		t.Fatal("zero resolver accepted")
	}
	// Recomputed outer hash cannot hide changed assessment evidence.
	d = valueFixture(t)
	d.Assessments[0].Candidates = nil
	if _, err := p.Prepare(artifact(t, d)); err == nil {
		t.Fatal("forged cached assessment accepted")
	}
}

func TestPreparedDocumentCopyPreservesNilAndEmptyCollections(t *testing.T) {
	d := valueFixture(t)
	d.Bindings = nil
	d.Dependencies = []Dependency{}
	var p Preparer
	value, err := p.Prepare(artifact(t, d))
	if err != nil {
		t.Fatal(err)
	}
	copy, err := value.Document()
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(d)
	b, _ := json.Marshal(copy)
	if string(a) != string(b) {
		t.Fatal("copy normalized collections")
	}
}

func TestPreparedRemainsImmutableWhilePreparerAddsRelatedContext(t *testing.T) {
	source := artifact(t, restoreContext(t, "source"))
	var preparer Preparer
	value, err := preparer.Prepare(source)
	if err != nil {
		t.Fatal(err)
	}
	d := restoreContext(t, "root")
	d.Sources = []Source{{JSON: source.JSON, Digest: source.Digest}}
	d.SourceRefs = []ContextReference{{ProjectID: "source", Version: "1", Digest: source.Digest}}
	root := artifact(t, d)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 100 {
			doc, err := value.Document()
			if err != nil || doc.ProjectID != "source" {
				t.Error("detached context changed")
				return
			}
			if err := ValidatePreparedBundle([]Prepared{value}); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	_, err = preparer.Prepare(root)
	<-done
	if err != nil {
		t.Fatal(err)
	}
}
