package projectsnapshot_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/namespace"
	"github.com/pletka-io/pletka/pkg/ontologyartifact"
	"github.com/pletka-io/pletka/pkg/ontologyevidence"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/releasecontent"
	"github.com/pletka-io/pletka/pkg/releasedescriptor"
)

func ptr[T any](v T) *T { return &v }
func encoded(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func digest(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
func identity(t *testing.T, project, version string) projectsnapshot.Identity {
	t.Helper()
	id, err := projectsnapshot.Release(project, version)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func emptyContent(id projectsnapshot.Identity) projectsnapshot.Content {
	c := projectsnapshot.Content{}
	v := reflect.ValueOf(&c).Elem()
	for i := range v.NumField() {
		v.Field(i).Set(reflect.MakeSlice(v.Field(i).Type(), 0, 0))
	}
	c.Projects = []projectsnapshot.ProjectRecord{{ID: id.ProjectID(), VersionNumber: id.Version(), OwnerID: "owner", EffectiveConceptNamespace: ptr("https://concepts.example/"), EnforceConceptLists: ptr(false)}}
	return c
}

func counts(c projectsnapshot.Content) map[string]int64 {
	result := map[string]int64{}
	v := reflect.ValueOf(c)
	typ := v.Type()
	for i := range v.NumField() {
		result[typ.Field(i).Tag.Get("json")] = int64(v.Field(i).Len())
	}
	return result
}

func descriptors(t *testing.T, id projectsnapshot.Identity, deps []projectsnapshot.Identity, links []projectsnapshot.OntologyLinkRecord) releasedescriptor.Snapshot {
	t.Helper()
	d := releasedescriptor.Document{Format: 1, ProjectID: id.ProjectID(), Version: id.Version(), Actors: map[string]releasedescriptor.Actor{"owner": {ActorID: "owner", DisplayName: "Historical owner"}}, ProjectModules: map[string]string{id.ProjectID(): "module/" + id.ProjectID()}, OntologyModules: map[string]string{}}
	for _, dep := range deps {
		d.ProjectModules[dep.ProjectID()] = "module/" + dep.ProjectID()
	}
	for _, link := range links {
		d.OntologyModules[link.OntologyVersionID] = "ontology/" + link.OntologyVersionID
	}
	result, err := releasedescriptor.Decode(encoded(t, d), id.ProjectID(), id.Version())
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func receipt(t *testing.T, c projectsnapshot.Content, format int, ontologyDigest string) []byte {
	t.Helper()
	sections := map[string]releasecontent.Section{}
	for name, count := range counts(c) {
		sections[name] = releasecontent.Section{Rows: count, SHA256: strings.Repeat("a", 64)}
	}
	d := map[string]any{"format_version": format, "sections": sections}
	if format == 2 {
		d["evidence_kind"] = "legacy_ontology_snapshot"
		d["ontology_snapshot_digest"] = ontologyDigest
	}
	return encoded(t, d)
}

func input(t *testing.T, id projectsnapshot.Identity, c projectsnapshot.Content, deps []projectsnapshot.Identity, members []projectsnapshot.Membership) projectsnapshot.ProjectInput {
	t.Helper()
	r, err := releasecontent.DecodeVerified(receipt(t, c, 1, ""), id.ProjectID(), id.Version())
	if err != nil {
		t.Fatal(err)
	}
	e, err := projectsnapshot.VerifiedEvidence(r, descriptors(t, id, deps, c.OntologyLinks), []ontologyartifact.ProjectArtifact{}, []ontologyartifact.TermResolution{})
	if err != nil {
		t.Fatal(err)
	}
	return projectsnapshot.ProjectInput{Content: c, Evidence: e, Counts: counts(c), Dependencies: deps, Membership: members}
}

func fixture(t *testing.T) (projectsnapshot.ProjectInput, projectsnapshot.ProjectInput) {
	t.Helper()
	a, b := identity(t, "A", "1"), identity(t, "B", "1")
	source := emptyContent(b)
	source.Fields = []projectsnapshot.FieldRecord{{ID: "foreign", ProjectID: "B", VersionNumber: "1"}}
	own := emptyContent(a)
	own.Models = []projectsnapshot.ModelRecord{{ID: "m", ProjectID: "A", VersionNumber: "1", ModelType: "auxiliary"}}
	own.Fields = []projectsnapshot.FieldRecord{{ID: "f", ProjectID: "A", VersionNumber: "1", OntologyPath: ptr("authored path"), UIName: []byte(`{ "en" : "Field" }`)}, {ID: "unused", ProjectID: "A", VersionNumber: "1"}}
	own.Overrides = []projectsnapshot.OverrideRecord{
		{ID: 2, FieldID: "f", ProjectID: "A", VersionNumber: "1", EntityType: "model", EntityID: "m", Position: 8, IsRequired: ptr(false)},
		{ID: 1, FieldID: "f", ProjectID: "A", VersionNumber: "1", SetValueEntryID: ptr("entry")},
		{ID: 3, FieldID: "foreign", ProjectID: "A", VersionNumber: "1", EntityType: "model", EntityID: "m", Position: 8},
	}
	own.Vocabularies = []projectsnapshot.VocabularyRecord{{ID: "v", ProjectID: "A", VersionNumber: "1"}}
	own.VocabularyEntries = []projectsnapshot.VocabularyEntryRecord{{ID: "entry", VocabularyID: "v", VersionNumber: "1", Label: []byte(`{"en":"Entry"}`)}}
	own.ConceptLists = []projectsnapshot.ConceptListRecord{{ID: "list", ProjectID: "A", VersionNumber: "1", IsClosed: true, VocabularyID: ptr("v")}}
	own.ConceptEntries = []projectsnapshot.ConceptEntryRecord{{ID: "membership", ConceptListID: "list", VocabularyEntryID: "entry", VersionNumber: "1", Position: 7}}
	own.OverrideRefs = []projectsnapshot.OverrideRefRecord{{OverrideID: 2, RefType: "concept_list", TargetID: "list", ProjectID: "A", VersionNumber: "1"}}
	own.Adoptions = []projectsnapshot.AdoptionRecord{{ProjectID: "A", VersionNumber: "1", ContextEntityType: "model", ContextEntityID: "m", EntityType: "field", SourceProjectID: "B", SourceEntityID: "foreign", SourceVersion: "1"}}
	own.Namespaces = []projectsnapshot.NamespaceRecord{
		{ID: "n2", ReleaseProjectID: "A", VersionNumber: "1", Prefix: "crm", Namespace: "https://two/", Weight: 20, Source: "ontology", AirtableRaw: []byte(`{"retained":true}`)},
		{ID: "n1", ReleaseProjectID: "A", VersionNumber: "1", Prefix: "crm", Namespace: "https://one/", Weight: 20, Source: "ontology"},
	}
	return input(t, a, own, []projectsnapshot.Identity{b}, []projectsnapshot.Membership{{Kind: "field", ID: "foreign", Source: b}}), input(t, b, source, []projectsnapshot.Identity{}, []projectsnapshot.Membership{})
}

func TestCompleteAndFragmentPreserveLosslessRecordsAndSourceIdentity(t *testing.T) {
	a, b := fixture(t)
	complete, err := projectsnapshot.NewComplete(a, []projectsnapshot.ProjectInput{b})
	if err != nil {
		t.Fatal(err)
	}
	root, err := complete.Root()
	if err != nil {
		t.Fatal(err)
	}
	want, err := complete.Content(root)
	if err != nil {
		t.Fatal(err)
	}
	a.Content.Fields[0].UIName[0] = '!'
	*a.Content.Overrides[0].IsRequired = true
	a.Dependencies[0] = projectsnapshot.Identity{}
	got, err := complete.Content(root)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal(diff)
	}
	got.Fields[0].UIName[0] = '!'
	*got.Fields[0].OntologyPath = "changed"
	got.Namespaces[0].AirtableRaw[0] = '!'
	again, err := complete.Content(root)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, again); diff != "" {
		t.Fatal(diff)
	}
	if again.Overrides[0].ID != 2 || again.Overrides[1].IsRequired != nil || again.Overrides[0].IsRequired == nil || *again.Overrides[0].IsRequired {
		t.Fatal("ordering or null override semantics lost")
	}
	registry, err := complete.Namespaces(root)
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := registry.Bindings()
	if err != nil || len(bindings) != 2 {
		t.Fatalf("aliases lost: %v", err)
	}
	if _, err := registry.Effective(); err == nil {
		t.Fatal("complete structure promoted tied aliases")
	}
	fragment, err := complete.Entity("model", "m")
	if err != nil {
		t.Fatal(err)
	}
	entity, err := fragment.Root()
	if err != nil || entity.Source != root || entity.ID != "m" {
		t.Fatalf("wrong fragment root: %+v %v", entity, err)
	}
	partial, err := fragment.Content(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(partial.Fields) != 1 || partial.Fields[0].ID != "f" || len(partial.Overrides) != 3 || len(partial.ConceptEntries) != 1 || len(partial.VocabularyEntries) != 1 {
		t.Fatalf("incorrect closed fragment: %+v", partial)
	}
	foreign, err := fragment.Content(b.Evidence.Identity())
	if err != nil || len(foreign.Fields) != 1 {
		t.Fatalf("foreign body omitted: %v", err)
	}
	if reflect.TypeFor[projectsnapshot.EntityFragment]().ConvertibleTo(reflect.TypeFor[projectsnapshot.CompleteProject]()) {
		t.Fatal("fragment converts to complete")
	}
	forged := a
	forged.Content = partial
	forged.Counts = counts(partial)
	forged.Dependencies = []projectsnapshot.Identity{b.Evidence.Identity()}
	if _, err := projectsnapshot.NewComplete(forged, []projectsnapshot.ProjectInput{b}); err == nil {
		t.Fatal("fragment passed complete receipt")
	}
}

func TestCompleteConstructionRejectsMissingOrConflictingCoverage(t *testing.T) {
	cases := map[string]func(*projectsnapshot.ProjectInput, *projectsnapshot.ProjectInput){
		"unloaded section": func(a, b *projectsnapshot.ProjectInput) { a.Content.Forks = nil },
		"truncated count":  func(a, b *projectsnapshot.ProjectInput) { a.Counts["weave_fields_archive"]++ },
		"substituted count": func(a, b *projectsnapshot.ProjectInput) {
			delete(a.Counts, "weave_fields_archive")
			a.Counts["unknown"] = 2
		},
		"source body missing":       func(a, b *projectsnapshot.ProjectInput) { b.Content.Fields[0].ID = "other" },
		"wrong source pin":          func(a, b *projectsnapshot.ProjectInput) { a.Dependencies[0] = identity(t, "B", "2") },
		"foreign membership absent": func(a, b *projectsnapshot.ProjectInput) { a.Membership = []projectsnapshot.Membership{} },
		"conflicting membership":    func(a, b *projectsnapshot.ProjectInput) { a.Membership = append(a.Membership, a.Membership[0]) },
		"unloaded membership":       func(a, b *projectsnapshot.ProjectInput) { a.Membership = nil },
		"wrong row owner":           func(a, b *projectsnapshot.ProjectInput) { a.Content.Fields[0].ProjectID = "B" },
		"wrong row version":         func(a, b *projectsnapshot.ProjectInput) { a.Content.Fields[0].VersionNumber = "2" },
		"duplicate row":             func(a, b *projectsnapshot.ProjectInput) { a.Content.Fields[1] = a.Content.Fields[0] },
		"closed list target absent": func(a, b *projectsnapshot.ProjectInput) { a.Content.ConceptEntries[0].VocabularyEntryID = "gone" },
		"child parent absent":       func(a, b *projectsnapshot.ProjectInput) { a.Content.VocabularyEntries[0].VocabularyID = "gone" },
		"invalid authored JSON":     func(a, b *projectsnapshot.ProjectInput) { a.Content.Fields[0].UIName = []byte(`{} false`) },
		"new semantic input uncovered": func(a, b *projectsnapshot.ProjectInput) {
			a.Content.Fields[0].OntologyScope = []byte(`{"type":"class","uri":"https://term"}`)
		},
		"missing actor descriptor": func(a, b *projectsnapshot.ProjectInput) { a.Content.Adoptions[0].CreatedByID = ptr("unknown") },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			a, b := fixture(t)
			change(&a, &b)
			value, err := projectsnapshot.NewComplete(a, []projectsnapshot.ProjectInput{b})
			if err == nil {
				t.Fatal("invalid input accepted")
			}
			if _, err := value.Root(); err == nil {
				t.Fatal("failure returned usable partial value")
			}
		})
	}
}

func TestStaleSavedValuesKeepExactNumericDateAndSlotEvidence(t *testing.T) {
	id := identity(t, "A", "1")
	c := emptyContent(id)
	c.Models = []projectsnapshot.ModelRecord{{ID: "m", ProjectID: "A", VersionNumber: "1", ModelType: "example"}}
	c.Examples = []projectsnapshot.ExampleRecord{{ID: "e", ProjectID: "A", VersionNumber: "1", EntityType: "model", EntityID: "m", Status: "assessment-required"}}
	c.ExampleValues = []projectsnapshot.ExampleValueRecord{{ID: 1, ExampleID: "e", FieldID: "deleted", OverrideID: 999, VersionNumber: "1", NumberValue: ptr(projectsnapshot.Decimal("12345678901234567890.12345678900")), DateValue: ptr(projectsnapshot.Date("infinity")), SlotPath: "999:4/obsolete:2", ValuePayload: []byte(`{ "unknown" : [1, 2] }`)}}
	value, err := projectsnapshot.NewComplete(input(t, id, c, []projectsnapshot.Identity{}, []projectsnapshot.Membership{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := value.Entity("example", "e")
	if err != nil {
		t.Fatal(err)
	}
	got, err := fragment.Content(id)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(c.ExampleValues, got.ExampleValues); diff != "" {
		t.Fatal(diff)
	}
	if got.Examples[0].Status != "assessment-required" {
		t.Fatal("assessment status rewritten")
	}
}

func TestZeroAndWrongSelectionValuesFailClosed(t *testing.T) {
	if _, err := (projectsnapshot.CompleteProject{}).Content(projectsnapshot.Identity{}); err == nil {
		t.Fatal("zero complete usable")
	}
	if _, err := (projectsnapshot.EntityFragment{}).Projects(); err == nil {
		t.Fatal("zero fragment usable")
	}
	if _, err := projectsnapshot.Release("A", ""); err == nil {
		t.Fatal("empty release became draft")
	}
	a, b := fixture(t)
	value, err := projectsnapshot.NewComplete(a, []projectsnapshot.ProjectInput{b})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := value.Content(identity(t, "B", "2")); err == nil {
		t.Fatal("caller retargeted source version")
	}
	if _, err := value.Entity("field", "absent"); err == nil {
		t.Fatal("missing root accepted")
	}
	if _, err := value.Entity("project", "A"); err == nil {
		t.Fatal("whole project disguised as fragment")
	}
	if _, err := (projectsnapshot.Evidence{}).Publications(); err == nil {
		t.Fatal("zero evidence usable")
	}
	if _, err := a.Evidence.Legacy(); err == nil {
		t.Fatal("verified evidence acquired legacy kind")
	}
}

func legacyInput(t *testing.T, id projectsnapshot.Identity, c projectsnapshot.Content) projectsnapshot.ProjectInput {
	t.Helper()
	elements, err := projectsnapshot.SemanticElements(c)
	if err != nil {
		t.Fatal(err)
	}
	d := ontologyevidence.Document{FormatVersion: 1, ProjectID: id.ProjectID(), Version: id.Version(), Inventory: []ontologyevidence.Body{}, Bindings: []namespace.Binding{}, Elements: elements, Assessments: []ontologyevidence.Assessment{}, Sources: []ontologyevidence.Source{}, SourceRefs: []ontologyevidence.ContextReference{}, Dependencies: []ontologyevidence.Dependency{}}
	for _, row := range c.Namespaces {
		d.Bindings = append(d.Bindings, namespace.Binding{Prefix: row.Prefix, Namespace: row.Namespace, Weight: row.Weight, Source: row.Source})
	}
	for _, link := range c.OntologyLinks {
		body := string(encoded(t, map[string]any{"format_version": 1, "ontology": map[string]string{"id": "crm"}, "version": map[string]string{"id": link.OntologyVersionID, "ontology_id": "crm", "version_string": "7.1.3"}, "classes": []map[string]string{{"uri": "https://example/E1", "prefix": "crm", "local_name": "E1"}}}))
		d.Inventory = append(d.Inventory, ontologyevidence.Body{OntologyID: "crm", VersionID: link.OntologyVersionID, Digest: digest(body), BodyJSON: body, Selection: encoded(t, link)})
	}
	refs, err := ontologyevidence.InputReferences(elements)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range refs {
		status, candidates, err := ontologyevidence.AssessReference(ref.Reference, d.Inventory, d.Bindings)
		if err != nil {
			t.Fatal(err)
		}
		d.Assessments = append(d.Assessments, ontologyevidence.Assessment{EntityKind: ref.EntityKind, EntityID: ref.EntityID, Location: ref.Location, Reference: ref.Reference, Status: status, Candidates: candidates})
	}
	raw := string(encoded(t, d))
	artifact := ontologyevidence.Artifact{JSON: raw, Digest: digest(raw)}
	receipt, err := releasecontent.DecodeLegacy(receipt(t, c, 2, artifact.Digest), id.ProjectID(), id.Version())
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := projectsnapshot.LegacyEvidence(receipt, descriptors(t, id, nil, c.OntologyLinks), artifact)
	if err != nil {
		t.Fatal(err)
	}
	return projectsnapshot.ProjectInput{Content: c, Evidence: evidence, Counts: counts(c), Dependencies: []projectsnapshot.Identity{}, Membership: []projectsnapshot.Membership{}}
}

func TestLegacyCompleteContentPreservesUnresolvedMeaning(t *testing.T) {
	id := identity(t, "legacy", "1")
	c := emptyContent(id)
	c.Fields = []projectsnapshot.FieldRecord{{ID: "f", ProjectID: "legacy", VersionNumber: "1", OntologyScope: []byte(`{"prefix":"crm","local_name":"E1"}`)}}
	c.OntologyLinks = []projectsnapshot.OntologyLinkRecord{{ProjectID: "legacy", VersionNumber: "1", OntologyVersionID: "a", IsPrimary: ptr(true)}, {ProjectID: "legacy", VersionNumber: "1", OntologyVersionID: "b"}}
	in := legacyInput(t, id, c)
	value, err := projectsnapshot.NewComplete(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := value.Evidence(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := evidence.Publications(); err == nil {
		t.Fatal("legacy structure promoted to verified semantics")
	}
	artifact, err := evidence.Legacy()
	if err != nil {
		t.Fatal(err)
	}
	d, err := artifact.Decode()
	if err != nil || d.Assessments[0].Status != ontologyevidence.Ambiguous {
		t.Fatalf("ambiguity lost: %v", err)
	}
	in.Content.Fields[0].OntologyScope = []byte(`{"prefix":"crm","local_name":"E2"}`)
	if _, err := projectsnapshot.NewComplete(in, nil); err == nil {
		t.Fatal("authored content drifted from retained legacy evidence")
	}
}

func TestForeignFieldFragmentRetainsConsumerBaseOverride(t *testing.T) {
	a, b := fixture(t)
	a.Content.Overrides = append(a.Content.Overrides, projectsnapshot.OverrideRecord{ID: 4, FieldID: "foreign", ProjectID: "A", VersionNumber: "1", DisplayName: []byte(`{"en":"Consumer default"}`)})
	a = input(t, a.Evidence.Identity(), a.Content, a.Dependencies, a.Membership)
	value, err := projectsnapshot.NewComplete(a, []projectsnapshot.ProjectInput{b})
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range []struct{ kind, id string }{{"field", "foreign"}, {"model", "m"}} {
		fragment, err := value.Entity(root.kind, root.id)
		if err != nil {
			t.Fatal(err)
		}
		content, err := fragment.Content(a.Evidence.Identity())
		if err != nil {
			t.Fatal(err)
		}
		if root.kind == "field" && len(content.Models) != 0 {
			t.Fatal("field root pulled in unrelated contextual adoption")
		}
		found := false
		for _, override := range content.Overrides {
			if override.ID == 4 {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s fragment lost consumer defaults", root.kind)
		}
	}
}

func TestLegacyContentRejectsChangedSelectionFlags(t *testing.T) {
	id := identity(t, "legacy", "1")
	for _, original := range []*bool{nil, ptr(false), ptr(true)} {
		c := emptyContent(id)
		c.OntologyLinks = []projectsnapshot.OntologyLinkRecord{{ProjectID: "legacy", VersionNumber: "1", OntologyVersionID: "a", IsPrimary: original}}
		in := legacyInput(t, id, c)
		if original == nil {
			in.Content.OntologyLinks[0].IsPrimary = ptr(false)
		} else {
			in.Content.OntologyLinks[0].IsPrimary = nil
		}
		if _, err := projectsnapshot.NewComplete(in, nil); err == nil {
			t.Fatal("changed nullable primary flag accepted")
		}
	}
}

func TestExplicitDraftAndSourceClosureRejection(t *testing.T) {
	draft, err := projectsnapshot.Draft("draft")
	if err != nil {
		t.Fatal(err)
	}
	c := emptyContent(draft)
	e, err := projectsnapshot.DraftEvidence(draft, descriptors(t, draft, nil, nil), []ontologyartifact.ProjectArtifact{}, []ontologyartifact.TermResolution{})
	if err != nil {
		t.Fatal(err)
	}
	in := projectsnapshot.ProjectInput{Content: c, Evidence: e, Counts: counts(c), Dependencies: []projectsnapshot.Identity{}, Membership: []projectsnapshot.Membership{}}
	if _, err := projectsnapshot.NewComplete(in, nil); err != nil {
		t.Fatal(err)
	}
	a, b := fixture(t)
	if _, err := projectsnapshot.NewComplete(a, []projectsnapshot.ProjectInput{b, in}); err == nil {
		t.Fatal("source draft accepted")
	}
	extraID := identity(t, "extra", "1")
	extra := input(t, extraID, emptyContent(extraID), []projectsnapshot.Identity{}, []projectsnapshot.Membership{})
	if _, err := projectsnapshot.NewComplete(a, []projectsnapshot.ProjectInput{b, extra}); err == nil {
		t.Fatal("unreachable source accepted")
	}
	b = input(t, b.Evidence.Identity(), b.Content, []projectsnapshot.Identity{a.Evidence.Identity()}, []projectsnapshot.Membership{})
	if _, err := projectsnapshot.NewComplete(a, []projectsnapshot.ProjectInput{b}); err == nil {
		t.Fatal("source cycle accepted")
	}
}

func TestPublishedSemanticsCoverPrimaryAdditionalDatatypeAndSubfieldInputs(t *testing.T) {
	id := identity(t, "semantic", "1")
	c := emptyContent(id)
	c.OntologyLinks = []projectsnapshot.OntologyLinkRecord{{ProjectID: "semantic", VersionNumber: "1", OntologyVersionID: "v", IsPrimary: ptr(true)}}
	c.Fields = []projectsnapshot.FieldRecord{{ID: "f", ProjectID: "semantic", VersionNumber: "1",
		OntologyScope: []byte(`{"prefix":"crm","local_name":"E1","additional_types":[{"uri":"https://example/E2"}]}`),
		PathElements:  []byte(`[{"type":"property","prefix":"crm","local_name":"P1"},{"type":"literal","uri":"https://example/Value","datatype":"https://example/Value"}]`),
		SubfieldPaths: []byte(`[{"name":"sub","path_elements":[{"type":"class","uri":"https://example/E2","additional_types":[{"uri":"https://example/E1"}]}]}]`),
	}}
	body := `{"format_version":1,"ontology":{"id":"crm"},"version":{"id":"v","ontology_id":"crm"},"classes":[{"uri":"https://example/E1","prefix":"crm","local_name":"E1"},{"uri":"https://example/E2","prefix":"crm","local_name":"E2"},{"uri":"https://example/Value","prefix":"crm","local_name":"Value"}],"properties":[{"uri":"https://example/P1","prefix":"crm","local_name":"P1"}]}`
	doc := ontologyartifact.Document{FormatVersion: 1, OntologyID: "crm", VersionID: "v", BodyJSON: body, BodyDigest: digest(body), Pins: []ontologyartifact.Pin{}, Requirements: []ontologyartifact.Requirement{}}
	raw := string(encoded(t, doc))
	artifact := ontologyartifact.Artifact{JSON: raw, Digest: digest(raw)}
	publications := []ontologyartifact.ProjectArtifact{{Artifact: artifact, Linked: true, Primary: true}}
	elements, err := projectsnapshot.SemanticElements(c)
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := ontologyevidence.InputReferences(elements)
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 7 {
		t.Fatalf("semantic locations omitted: %+v", inputs)
	}
	inventory := []ontologyevidence.Body{{OntologyID: "crm", VersionID: "v", BodyJSON: body, Digest: digest(body)}}
	assessor, err := ontologyevidence.NewAssessor(inventory, []namespace.Binding{})
	if err != nil {
		t.Fatal(err)
	}
	inventory[0].BodyJSON = "corrupted after construction"
	refs := make([]ontologyartifact.TermResolution, 0, len(inputs))
	for _, ref := range inputs {
		status, candidates, err := assessor.Assess(ref.Reference)
		if err != nil || status != ontologyevidence.Resolved {
			t.Fatalf("%s: %s %v", ref.Location, status, err)
		}
		refs = append(refs, ontologyartifact.TermResolution{EntityKind: ref.EntityKind, EntityID: ref.EntityID, Location: ref.Location, Reference: ontologyartifact.TermReference(ref.Reference), OntologyID: "crm", VersionID: "v", Digest: artifact.Digest, URI: candidates[0].URI})
		candidates[0].URI = "mutated"
	}
	r, err := releasecontent.DecodeVerified(receipt(t, c, 1, ""), id.ProjectID(), id.Version())
	if err != nil {
		t.Fatal(err)
	}
	makeInput := func(refs []ontologyartifact.TermResolution) projectsnapshot.ProjectInput {
		t.Helper()
		e, err := projectsnapshot.VerifiedEvidence(r, descriptors(t, id, nil, c.OntologyLinks), publications, refs)
		if err != nil {
			t.Fatal(err)
		}
		return projectsnapshot.ProjectInput{Content: c, Evidence: e, Counts: counts(c), Dependencies: []projectsnapshot.Identity{}, Membership: []projectsnapshot.Membership{}}
	}
	in := makeInput(refs)
	if _, err := projectsnapshot.NewComplete(in, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := projectsnapshot.NewComplete(makeInput(refs[:len(refs)-1]), nil); err == nil {
		t.Fatal("omitted subfield resolution accepted")
	}
	wrong := append([]ontologyartifact.TermResolution{}, refs...)
	wrong[0].URI = "https://example/E2"
	if _, err := projectsnapshot.NewComplete(makeInput(wrong), nil); err == nil {
		t.Fatal("wrong but present term URI accepted")
	}
	if _, err := projectsnapshot.NewFragment(in, nil, "field", "f"); err != nil {
		t.Fatal(err)
	}
	if _, err := projectsnapshot.NewFragment(makeInput(refs[:len(refs)-1]), nil, "field", "f"); err == nil {
		t.Fatal("fragment accepted omitted selected subfield resolution")
	}
	if _, err := projectsnapshot.NewFragment(makeInput(wrong), nil, "field", "f"); err == nil {
		t.Fatal("fragment accepted substituted selected term")
	}

	if _, _, err := (ontologyevidence.Assessor{}).Assess(inputs[0].Reference); err == nil {
		t.Fatal("zero semantic assessor accepted")
	}
}

func TestFragmentPreservesRepeatedEmptyCollectionPlacements(t *testing.T) {
	id := identity(t, "empty-placements", "1")
	c := emptyContent(id)
	c.Models = []projectsnapshot.ModelRecord{{ID: "m", ProjectID: id.ProjectID(), VersionNumber: id.Version()}}
	c.Collections = []projectsnapshot.CollectionRecord{{ID: "c", ProjectID: id.ProjectID(), VersionNumber: id.Version()}}
	c.Placements = []projectsnapshot.PlacementRecord{
		{ID: 2, ProjectID: id.ProjectID(), VersionNumber: id.Version(), ModelID: "m", CollectionID: "c", MinOccurs: 1},
		{ID: 1, ProjectID: id.ProjectID(), VersionNumber: id.Version(), ModelID: "m", CollectionID: "c", IsHidden: true},
	}
	value, err := projectsnapshot.NewComplete(input(t, id, c, []projectsnapshot.Identity{}, []projectsnapshot.Membership{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := value.Entity("model", "m")
	if err != nil {
		t.Fatal(err)
	}
	got, err := fragment.Content(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Collections) != 1 || len(got.Fields) != 0 {
		t.Fatal("empty collection omitted or invented fields")
	}
	if diff := cmp.Diff(c.Placements, got.Placements); diff != "" {
		t.Fatal(diff)
	}
}

func publicationInput(t *testing.T, in projectsnapshot.ProjectInput, label string) projectsnapshot.ProjectInput {
	t.Helper()
	id := in.Evidence.Identity()
	in.Content.OntologyLinks = []projectsnapshot.OntologyLinkRecord{{ProjectID: id.ProjectID(), VersionNumber: id.Version(), OntologyVersionID: "shared"}}
	body := string(encoded(t, map[string]any{"format_version": 1, "ontology": map[string]string{"id": "shared-ontology", "label": label}, "version": map[string]string{"id": "shared", "ontology_id": "shared-ontology"}}))
	doc := ontologyartifact.Document{FormatVersion: 1, OntologyID: "shared-ontology", VersionID: "shared", BodyJSON: body, BodyDigest: digest(body), Pins: []ontologyartifact.Pin{}, Requirements: []ontologyartifact.Requirement{}}
	raw := string(encoded(t, doc))
	r, err := releasecontent.DecodeVerified(receipt(t, in.Content, 1, ""), id.ProjectID(), id.Version())
	if err != nil {
		t.Fatal(err)
	}
	in.Evidence, err = projectsnapshot.VerifiedEvidence(r, descriptors(t, id, in.Dependencies, in.Content.OntologyLinks), []ontologyartifact.ProjectArtifact{{Artifact: ontologyartifact.Artifact{JSON: raw, Digest: digest(raw)}, Linked: true}}, []ontologyartifact.TermResolution{})
	if err != nil {
		t.Fatal(err)
	}
	in.Counts = counts(in.Content)
	return in
}

func TestSharedSourceDescriptorsAndOntologyBodiesAgree(t *testing.T) {
	a, b := fixture(t)
	a = publicationInput(t, a, "original")
	b = publicationInput(t, b, "original")
	if _, err := projectsnapshot.NewComplete(a, []projectsnapshot.ProjectInput{b}); err != nil {
		t.Fatal(err)
	}
	changed := publicationInput(t, b, "same version, different retained bytes")
	if _, err := projectsnapshot.NewComplete(a, []projectsnapshot.ProjectInput{changed}); err == nil {
		t.Fatal("conflicting ontology bytes accepted")
	}
	publications, err := b.Evidence.Publications()
	if err != nil {
		t.Fatal(err)
	}
	publications[0].Artifact.JSON = " " + publications[0].Artifact.JSON
	publications[0].Artifact.Digest = digest(publications[0].Artifact.JSON)
	br, err := releasecontent.DecodeVerified(receipt(t, b.Content, 1, ""), b.Evidence.Identity().ProjectID(), b.Evidence.Identity().Version())
	if err != nil {
		t.Fatal(err)
	}
	changed = b
	changed.Evidence, err = projectsnapshot.VerifiedEvidence(br, b.Evidence.Descriptors(), publications, []ontologyartifact.TermResolution{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projectsnapshot.NewComplete(a, []projectsnapshot.ProjectInput{changed}); err == nil {
		t.Fatal("conflicting exact publication bytes accepted")
	}
	a, b = fixture(t)
	id := b.Evidence.Identity()
	d := releasedescriptor.Document{Format: 1, ProjectID: id.ProjectID(), Version: id.Version(), Actors: map[string]releasedescriptor.Actor{"owner": {ActorID: "owner"}}, ProjectModules: map[string]string{id.ProjectID(): "changed/module"}, OntologyModules: map[string]string{}}
	descriptor, err := releasedescriptor.Decode(encoded(t, d), id.ProjectID(), id.Version())
	if err != nil {
		t.Fatal(err)
	}
	r, err := releasecontent.DecodeVerified(receipt(t, b.Content, 1, ""), id.ProjectID(), id.Version())
	if err != nil {
		t.Fatal(err)
	}
	b.Evidence, err = projectsnapshot.VerifiedEvidence(r, descriptor, []ontologyartifact.ProjectArtifact{}, []ontologyartifact.TermResolution{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projectsnapshot.NewComplete(a, []projectsnapshot.ProjectInput{b}); err == nil {
		t.Fatal("source module substitution accepted")
	}
}
