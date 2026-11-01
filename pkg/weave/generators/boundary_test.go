package generators_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/internal/testsnapshot"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/weave/generators"
)

func boundaryValue(t *testing.T) projectsnapshot.CompleteProject {
	return testsnapshot.GeneratorValue(t, domain.Project{Entity: domain.Entity{ID: "G"}}, domain.Model{}, domain.ModelView{}, domain.Collection{Entity: domain.Entity{ID: "C"}}, []domain.ResolvedField{resolvedField("F", "G.F", "Field", 1)})
}
func pointer[T any](v T) *T { return &v }

type failingSource struct {
	calls int
	err   error
}

func (s *failingSource) Entity(context.Context, auth.ReadScope, string, string, string) (projectsnapshot.EntityFragment, error) {
	s.calls++
	return projectsnapshot.EntityFragment{}, s.err
}
func (s *failingSource) Project(context.Context, auth.ReadScope, string) (projectsnapshot.CompleteProject, error) {
	s.calls++
	return projectsnapshot.CompleteProject{}, s.err
}

func TestGeneratorBoundaryRequiresSelectionAndPropagatesFailure(t *testing.T) {
	source := &failingSource{err: errors.New("archive unavailable")}
	service := generators.NewService(source, nil)
	if _, err := service.SnapshotForField(t.Context(), auth.ReadScope{}, "G", "F", generators.Options{}); err == nil || source.calls != 0 {
		t.Fatal("missing scope reached source")
	}
	if _, err := service.SnapshotForField(t.Context(), auth.Release("1"), "G", "F", generators.Options{}); !errors.Is(err, source.err) || source.calls != 1 {
		t.Fatal("source failure changed or retried")
	}
	service = generators.NewService(testsnapshot.Source{Value: boundaryValue(t)}, nil)
	if _, err := service.SnapshotForField(t.Context(), auth.Release("1"), "G", "F", generators.Options{}); err == nil {
		t.Fatal("draft value accepted for release request")
	}
	if _, err := service.SnapshotForField(t.Context(), auth.Draft(), "OTHER", "F", generators.Options{}); err == nil {
		t.Fatal("wrong project accepted")
	}
}

func TestGeneratorUsesRetainedTermsAndSealedConstraints(t *testing.T) {
	value := boundaryValue(t)
	root, _ := value.Root()
	c, _ := value.Content(root)
	// The effective prefix now means something else; the retained ontology body
	// still defines the exact original URI. Projection must supply a safe alias.
	c.Namespaces[0].Namespace = "https://unrelated.example/"
	c.Vocabularies = []projectsnapshot.VocabularyRecord{{ID: "V", ProjectID: "G"}}
	c.VocabularyEntries = []projectsnapshot.VocabularyEntryRecord{{ID: "E", VocabularyID: "V", URI: "pletka:one"}}
	c.ConceptLists = []projectsnapshot.ConceptListRecord{{ID: "L", ProjectID: "G", IsClosed: true, VocabularyID: pointer("V")}}
	c.ConceptEntries = []projectsnapshot.ConceptEntryRecord{{ID: "LE", ConceptListID: "L", VocabularyEntryID: "E"}}
	var base int64
	for i := range c.Overrides {
		if c.Overrides[i].EntityType == "" {
			base = c.Overrides[i].ID
			c.Overrides[i].IsRequired = pointer(false)
			c.Overrides[i].MaxOccurs = pointer(int32(0))
			c.Overrides[i].SetValueEntryID = pointer("E")
		}
	}
	c.OverrideRefs = []projectsnapshot.OverrideRefRecord{{OverrideID: base, ProjectID: "G", RefType: "concept_list", TargetID: "L"}}
	check := func(c projectsnapshot.Content) *generators.Snapshot {
		t.Helper()
		fragment, err := testsnapshot.Complete(t, c).Entity("field", "F")
		if err != nil {
			t.Fatal(err)
		}
		snap, err := generators.BuildSnapshot(fragment, generators.Options{})
		if err != nil {
			t.Fatal(err)
		}
		return snap
	}
	snap := check(c)
	field := snap.Fields[0]
	term := field.Path[0].Element
	if term.URI != "http://www.cidoc-crm.org/cidoc-crm/P1_is_identified_by" || snap.Namespaces.ByPrefix[term.Prefix]+term.LocalName != term.URI {
		t.Fatalf("term reinterpreted: %+v", term)
	}
	if !field.ConceptEnumClosed || len(field.ConceptEnum) != 1 || field.ConceptEnum[0] != "https://concepts.example/one" {
		t.Fatalf("lost sealed constraint: %+v", field)
	}
	if field.Field.IsRequired || field.Field.MaxOccurs == nil || *field.Field.MaxOccurs != 0 || field.Field.SetValueEntryID != "E" {
		t.Fatal("false, zero or fixed concept lost")
	}
	c.ConceptEntries = []projectsnapshot.ConceptEntryRecord{}
	empty := check(c)
	if !empty.Fields[0].ConceptEnumClosed || len(empty.Fields[0].ConceptEnum) != 0 {
		t.Fatal("empty sealed list became open")
	}
	raw, err := generators.EncodeInput(snap)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["schema_version"] != float64(1) || wire["selection"] == nil {
		t.Fatal("script input identity missing")
	}
	if _, err := generators.EncodeInput(&generators.Snapshot{}); err == nil {
		t.Fatal("unversioned input encoded")
	}
}

type mutatingRenderer struct{}

func (mutatingRenderer) Spec() generators.FormatSpec {
	return generators.FormatSpec{Format: "mutation-test"}
}
func (mutatingRenderer) Render(_ context.Context, s *generators.Snapshot, w io.Writer) error {
	s.Fields[0].Field.DisplayName["en"] = "changed by renderer"
	s.Namespaces.ByPrefix["crm"] = "changed"
	_, err := io.WriteString(w, "ok")
	return err
}
func TestRenderersCannotMutateAnotherRenderInput(t *testing.T) {
	fragment, err := boundaryValue(t).Entity("collection", "C")
	if err != nil {
		t.Fatal(err)
	}
	snap, err := generators.BuildSnapshot(fragment, generators.Options{})
	if err != nil {
		t.Fatal(err)
	}
	before, err := generators.EncodeInput(snap)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := generators.NewRegistry(mutatingRenderer{})
	if err != nil {
		t.Fatal(err)
	}
	service := generators.NewService(nil, registry)
	var out bytes.Buffer
	if err := service.RenderSnapshot(t.Context(), "mutation-test", snap, &out); err != nil {
		t.Fatal(err)
	}
	after, err := generators.EncodeInput(snap)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("renderer mutated shared input")
	}
}

func TestCompositionCannotDiscoverAnUnrecordedGlobalCollection(t *testing.T) {
	value := boundaryValue(t)
	root, _ := value.Root()
	c, _ := value.Content(root)
	c.Models = []projectsnapshot.ModelRecord{{ID: "M", ProjectID: "G", ModelType: "auxiliary"}}
	c.Fields[0].ExpectedValueType = pointer("Collection")
	c.Fields[0].PathElements = []byte(`[{"type":"class","prefix":"crm","local_name":"E1","uri":"crm:E1"}]`)
	c.Overrides = append(c.Overrides, projectsnapshot.OverrideRecord{ID: 100, ProjectID: "G", FieldID: "F", EntityType: "model", EntityID: "M"})
	fragment, err := testsnapshot.Complete(t, c).Entity("model", "M")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := generators.BuildSnapshot(fragment, generators.Options{ComposeCollections: true}); err == nil || !strings.Contains(err.Error(), "no retained target") {
		t.Fatalf("unrecorded graft accepted: %v", err)
	}
}

func TestCompositionUsesExplicitRetainedCollection(t *testing.T) {
	value := boundaryValue(t)
	root, _ := value.Root()
	c, _ := value.Content(root)
	c.Models = []projectsnapshot.ModelRecord{{ID: "M", ProjectID: "G", ModelType: "auxiliary"}}
	c.Fields[0].ExpectedValueType = pointer("Collection")
	c.Fields[0].PathElements = []byte(`[{"type":"class","prefix":"crm","local_name":"E1","uri":"crm:E1"}]`)
	for i, id := range []string{"A", "B"} {
		f := c.Fields[0]
		f.ID, f.SemanticID = id, pointer("G."+id)
		f.ExpectedValueType = pointer("String")
		property := []string{"P1", "P2"}[i]
		f.PathElements = []byte(`[{"type":"class","prefix":"crm","local_name":"E1","uri":"crm:E1"},{"type":"property","prefix":"crm","local_name":"` + property + `","uri":"crm:` + property + `"}]`)
		c.Fields = append(c.Fields, f)
	}
	c.Overrides = []projectsnapshot.OverrideRecord{
		{ID: 1, ProjectID: "G", FieldID: "F"},
		{ID: 2, ProjectID: "G", FieldID: "A"},
		{ID: 3, ProjectID: "G", FieldID: "B"},
		{ID: 100, ProjectID: "G", FieldID: "F", EntityType: "model", EntityID: "M"},
		{ID: 101, ProjectID: "G", FieldID: "A", EntityType: "collection", EntityID: "C"},
		{ID: 102, ProjectID: "G", FieldID: "B", EntityType: "collection", EntityID: "C"},
	}
	c.OverrideRefs = []projectsnapshot.OverrideRefRecord{{OverrideID: 100, ProjectID: "G", RefType: "collection_model", TargetID: "C", SemanticID: "C"}}
	fragment, err := testsnapshot.Complete(t, c).Entity("model", "M")
	if err != nil {
		t.Fatal(err)
	}
	snap, err := generators.BuildSnapshot(fragment, generators.Options{ComposeCollections: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Fields) != 3 || len(snap.Fields[1].GraftPath) == 0 || len(snap.Fields[2].GraftPath) == 0 {
		t.Fatalf("retained children were not grafted: %+v", snap.Fields)
	}
}

func TestRetainedIdentifiersPreferExactIDAndRejectAmbiguousAliases(t *testing.T) {
	value := boundaryValue(t)
	root, _ := value.Root()
	c, _ := value.Content(root)
	c.Models = []projectsnapshot.ModelRecord{
		{ID: "ALPHA", ProjectID: "G", SystemName: pointer("shared")},
		{ID: "BETA", ProjectID: "G", SystemName: pointer("shared")},
		{ID: "OTHER", ProjectID: "G", SystemName: pointer("ALPHA")},
	}
	value = testsnapshot.Complete(t, c)
	if id, err := generators.ResolveIdentifier(value, "model", "ALPHA"); err != nil || id != "ALPHA" {
		t.Fatalf("exact ID lost to alias: %q %v", id, err)
	}
	for _, alias := range []string{"shared", "", "missing"} {
		if _, err := generators.ResolveIdentifier(value, "model", alias); err == nil {
			t.Fatalf("invalid alias %q accepted", alias)
		}
	}
}
