package projectsnapshot_test

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/ontologyartifact"
	"github.com/pletka-io/pletka/pkg/ontologyevidence"
	"github.com/pletka-io/pletka/pkg/projectbundle"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/releasedescriptor"
)

func changedDraftDescriptor(t *testing.T, id projectsnapshot.Identity, doc releasedescriptor.Document, old projectsnapshot.Evidence) projectsnapshot.Evidence {
	t.Helper()
	descriptor, err := releasedescriptor.Decode(encoded(t, doc), id.ProjectID(), "")
	if err != nil {
		t.Fatal(err)
	}
	publications, _ := old.Publications()
	refs, _ := old.References()
	evidence, err := projectsnapshot.DraftEvidence(id, descriptor, publications, refs)
	if err != nil {
		t.Fatal(err)
	}
	return evidence
}

func draftInput(t *testing.T, in projectsnapshot.ProjectInput) projectsnapshot.ProjectInput {
	t.Helper()
	id, err := projectsnapshot.Draft(in.Evidence.Identity().ProjectID())
	if err != nil {
		t.Fatal(err)
	}
	rows := reflect.ValueOf(&in.Content).Elem()
	for i := range rows.NumField() {
		section := rows.Field(i)
		for n := range section.Len() {
			section.Index(n).FieldByName("VersionNumber").SetString("")
		}
	}
	publications, err := in.Evidence.Publications()
	if err != nil {
		t.Fatal(err)
	}
	refs, err := in.Evidence.References()
	if err != nil {
		t.Fatal(err)
	}
	in.Evidence, err = projectsnapshot.DraftEvidence(id, descriptors(t, id, in.Dependencies, in.Content.OntologyLinks), publications, refs)
	if err != nil {
		t.Fatal(err)
	}
	in.Counts = counts(in.Content)
	return in
}

func patchFixture(t *testing.T) (projectsnapshot.CompleteProject, projectsnapshot.ProjectInput, projectsnapshot.CompleteProject) {
	t.Helper()
	a, b := fixture(t)
	a.Content.Categories = []projectsnapshot.CategoryRecord{{ID: "cat", ProjectID: "A"}}
	a.Content.Collections = []projectsnapshot.CollectionRecord{{ID: "col", ProjectID: "A", DefaultCategoryID: ptr("cat")}}
	a.Content.Placements = []projectsnapshot.PlacementRecord{{ID: 4, ProjectID: "A", ModelID: "m", CollectionID: "col", CategoryID: "cat"}}
	a.Content.Broader = []projectsnapshot.BroaderRecord{{ID: "broader", SchemeID: "list", ConceptID: "entry", BroaderID: "entry"}}
	a.Content.Forks = []projectsnapshot.ForkRecord{{ProjectID: "A", EntityType: "field", ForkEntityID: "f", SourceProjectID: "B", SourceEntityID: "foreign", SourceVersion: "1"}}
	a.Content.Examples = []projectsnapshot.ExampleRecord{{ID: "example", ProjectID: "A", EntityType: "model", EntityID: "m"}}
	a.Content.ExampleValues = []projectsnapshot.ExampleValueRecord{{ID: 5, ExampleID: "example", FieldID: "f", OverrideID: 2, ValueKind: "number", NumberValue: ptr(projectsnapshot.Decimal("9007199254740993.123456789")), DateValue: ptr(projectsnapshot.Date("infinity")), ValuePayload: []byte(`null`)}}
	a.Content.ProjectActors = []projectsnapshot.ProjectActorRecord{{ProjectID: "A", ActorID: "owner", Role: "editor"}}
	a.Content.Attributions = []projectsnapshot.AttributionRecord{{ProjectID: "A", ActorID: "owner", Kind: "creator"}}
	a.Content.Inheritance = []projectsnapshot.InheritanceRecord{{ProjectID: "A", ParentProjectID: "B", SourceMode: "pinned", SourceVersion: ptr("1")}}
	a.Membership = append(a.Membership, projectsnapshot.Membership{Kind: "project", ID: "B", Source: b.Evidence.Identity()})
	a = draftInput(t, publicationInput(t, a, "publication"))
	base, err := projectsnapshot.NewComplete(a, []projectsnapshot.ProjectInput{b})
	if err != nil {
		t.Fatal(err)
	}
	source, err := projectsnapshot.NewComplete(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Use detached inventories so test edits cannot mutate a shared fixture.
	a, err = base.Input(a.Evidence.Identity())
	if err != nil {
		t.Fatal(err)
	}
	return base, a, source
}

func bundleBytes(t *testing.T, value projectsnapshot.CompleteProject) projectbundle.Files {
	t.Helper()
	files, err := projectbundle.Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestDraftPatchEverySectionRoundTripsWithoutLoss(t *testing.T) {
	base, in, source := patchFixture(t)
	id := in.Evidence.Identity()
	empty := emptyContent(id)
	evidence, err := projectsnapshot.DraftEvidence(id, descriptors(t, id, nil, nil), []ontologyartifact.ProjectArtifact{}, []ontologyartifact.TermResolution{})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := projectsnapshot.NewComplete(projectsnapshot.ProjectInput{Content: empty, Counts: counts(empty), Evidence: evidence, Dependencies: []projectsnapshot.Identity{}, Membership: []projectsnapshot.Membership{}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	beforeContext, _ := initial.DraftContext()
	afterContext, _ := base.DraftContext()
	for name, count := range in.Counts {
		if count == 0 {
			t.Fatalf("fixture omits captured section %s", name)
		}
	}
	inserted, err := projectsnapshot.PatchDraft(initial, projectsnapshot.DraftPatch{
		Target: id, Before: projectsnapshot.Content{Projects: empty.Projects}, After: in.Content,
		Context: &projectsnapshot.DraftContextChange{Before: beforeContext, After: afterContext}, Sources: []projectsnapshot.CompleteProject{source},
	})
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(bundleBytes(t, base), bundleBytes(t, inserted)); diff != "" {
		t.Fatal(diff)
	}
	// Explicit deletion of every child/reference plus source selection. Empty
	// After sections alone would not delete anything.
	removed, err := projectsnapshot.PatchDraft(inserted, projectsnapshot.DraftPatch{
		Target: id, Before: in.Content, After: projectsnapshot.Content{Projects: empty.Projects},
		Context: &projectsnapshot.DraftContextChange{Before: afterContext, After: beforeContext},
	})
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(bundleBytes(t, initial), bundleBytes(t, removed)); diff != "" {
		t.Fatal(diff)
	}
}

func TestDraftPatchEverySectionUpdateMatchesCompleteEncoding(t *testing.T) {
	base, original, source := patchFixture(t)
	sourceID, _ := source.Root()
	sourceIn, _ := source.Input(sourceID)
	id := original.Evidence.Identity()
	for _, section := range projectsnapshot.Coverage() {
		t.Run(section.Name, func(t *testing.T) {
			in, _ := base.Input(id)
			var before, after projectsnapshot.Content
			content := reflect.ValueOf(&in.Content).Elem()
			for i := range content.NumField() {
				if content.Type().Field(i).Tag.Get("json") != section.Name {
					continue
				}
				old := reflect.MakeSlice(content.Field(i).Type(), 1, 1)
				old.Index(0).Set(content.Field(i).Index(0))
				reflect.ValueOf(&before).Elem().Field(i).Set(old)
				row := content.Field(i).Index(0)
				// Alter a persisted scalar with no structural/semantic side effects.
				field := row.FieldByName("CreatedAt")
				switch section.ValueType {
				case "AdoptionRecord":
					field = row.FieldByName("AdoptedAt")
				case "ForkRecord":
					field = row.FieldByName("ForkedAt")
				case "InheritanceRecord":
					field = row.FieldByName("AdoptedAt")
				case "OntologyLinkRecord":
					field = row.FieldByName("AddedAt")
				case "OverrideRefRecord":
					field = row.FieldByName("Position")
				}
				switch field.Kind() {
				case reflect.String:
					field.SetString("2026-10-31T12:00:00Z")
				case reflect.Pointer:
					field.Set(reflect.ValueOf(ptr(projectsnapshot.Timestamp("infinity"))))
				case reflect.Int32:
					field.SetInt(42)
				default:
					t.Fatalf("no independently reviewed update for %s", section.ValueType)
				}
				next := reflect.MakeSlice(content.Field(i).Type(), 1, 1)
				next.Index(0).Set(row)
				reflect.ValueOf(&after).Elem().Field(i).Set(next)
			}
			want, err := projectsnapshot.NewComplete(in, []projectsnapshot.ProjectInput{sourceIn})
			if err != nil {
				t.Fatal(err)
			}
			got, err := projectsnapshot.PatchDraft(base, projectsnapshot.DraftPatch{Target: id, Before: before, After: after})
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(bundleBytes(t, want), bundleBytes(t, got)); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestDraftPatchDoesNotStealReferencedOrLaterEdits(t *testing.T) {
	base, in, _ := patchFixture(t)
	id := in.Evidence.Identity()
	before := in.Content.Overrides[0]
	after := before
	after.Position = 99
	first := projectsnapshot.DraftPatch{Target: id, Before: projectsnapshot.Content{Overrides: []projectsnapshot.OverrideRecord{before}}, After: projectsnapshot.Content{Overrides: []projectsnapshot.OverrideRecord{after}}}
	one, err := projectsnapshot.PatchDraft(base, first)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := one.Content(id)
	if diff := cmp.Diff(in.Content.Fields, data.Fields); diff != "" {
		t.Fatalf("referenced field was changed: %s", diff)
	}
	// Both edits exist before processing, but the field edit is a separate
	// operation. Its exact old row is still present after the override patch.
	fieldBefore := in.Content.Fields[0]
	fieldAfter := fieldBefore
	fieldAfter.UIName = []byte(`{"en":"Later field edit"}`)
	two, err := projectsnapshot.PatchDraft(one, projectsnapshot.DraftPatch{Target: id, Before: projectsnapshot.Content{Fields: []projectsnapshot.FieldRecord{fieldBefore}}, After: projectsnapshot.Content{Fields: []projectsnapshot.FieldRecord{fieldAfter}}})
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(bundleBytes(t, one), bundleBytes(t, two)) {
		t.Fatal("later edit became a no-op")
	}
	if _, err := projectsnapshot.PatchDraft(two, first); err == nil {
		t.Fatal("stale replay accepted")
	}
	// Input buffers remain caller-owned; the returned graph has its own copy.
	fieldAfter.UIName[0] = '!'
	if _, err := projectbundle.Encode(two); err != nil {
		t.Fatalf("caller corrupted patched value: %v", err)
	}
}

func TestDraftPatchRejectsUnclaimedRowsAndIncompleteCascades(t *testing.T) {
	base, in, source := patchFixture(t)
	id := in.Evidence.Identity()
	ctx, _ := base.DraftContext()
	for name, patch := range map[string]projectsnapshot.DraftPatch{
		"zero target":              {},
		"overwrite without before": {Target: id, After: projectsnapshot.Content{Fields: in.Content.Fields}},
		"duplicate before":         {Target: id, Before: projectsnapshot.Content{Fields: []projectsnapshot.FieldRecord{in.Content.Fields[0], in.Content.Fields[0]}}},
		"missing cascade":          {Target: id, Before: projectsnapshot.Content{Models: in.Content.Models}},
		"missing referenced body":  {Target: id, After: projectsnapshot.Content{Overrides: []projectsnapshot.OverrideRecord{{ID: 99, ProjectID: "A", FieldID: "missing"}}}},
		"foreign owner":            {Target: id, After: projectsnapshot.Content{Fields: []projectsnapshot.FieldRecord{{ID: "foreign", ProjectID: "B"}}}},
		"unloaded context":         {Target: id, Context: &projectsnapshot.DraftContextChange{Before: ctx}},
		"stale context":            {Target: id, Context: &projectsnapshot.DraftContextChange{After: ctx}},
		"draft source":             {Target: id, Sources: []projectsnapshot.CompleteProject{base}},
		"zero source":              {Target: id, Sources: []projectsnapshot.CompleteProject{{}}},
	} {
		t.Run(name, func(t *testing.T) {
			before := bundleBytes(t, base)
			got, err := projectsnapshot.PatchDraft(base, patch)
			if err == nil {
				t.Fatal("invalid patch accepted")
			}
			if _, err := got.Root(); err == nil {
				t.Fatal("failure returned a partial graph")
			}
			if diff := cmp.Diff(before, bundleBytes(t, base)); diff != "" {
				t.Fatal(diff)
			}
		})
	}
	if _, err := projectsnapshot.PatchDraft(source, projectsnapshot.DraftPatch{Target: id}); err == nil {
		t.Fatal("release baseline accepted")
	}
	if _, err := projectsnapshot.PatchDraft(projectsnapshot.CompleteProject{}, projectsnapshot.DraftPatch{Target: id}); err == nil {
		t.Fatal("zero baseline accepted")
	}
}

func TestDraftPatchNullAndContextChangesAreExplicit(t *testing.T) {
	base, in, _ := patchFixture(t)
	id := in.Evidence.Identity()
	before := in.Content.ExampleValues[0]
	after := before
	after.ValuePayload = nil // SQL NULL differs from authored JSON null.
	patch := projectsnapshot.DraftPatch{Target: id, Before: projectsnapshot.Content{ExampleValues: []projectsnapshot.ExampleValueRecord{before}}, After: projectsnapshot.Content{ExampleValues: []projectsnapshot.ExampleValueRecord{after}}}
	got, err := projectsnapshot.PatchDraft(base, patch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projectsnapshot.PatchDraft(got, patch); err == nil {
		t.Fatal("JSON null was accepted as SQL NULL in before image")
	}
	oldContext, _ := got.DraftContext()
	newContext := oldContext
	doc := oldContext.Evidence.Descriptors().Document()
	actor := doc.Actors["owner"]
	actor.DisplayName = "Explicit actor change"
	doc.Actors["owner"] = actor
	newContext.Evidence = changedDraftDescriptor(t, id, doc, oldContext.Evidence)
	next, err := projectsnapshot.PatchDraft(got, projectsnapshot.DraftPatch{Target: id, Context: &projectsnapshot.DraftContextChange{Before: oldContext, After: newContext}})
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(bundleBytes(t, got), bundleBytes(t, next)) {
		t.Fatal("explicit context change was lost")
	}
	if _, err := projectsnapshot.PatchDraft(next, projectsnapshot.DraftPatch{Target: id, Context: &projectsnapshot.DraftContextChange{Before: oldContext, After: newContext}}); err == nil {
		t.Fatal("stale context replay accepted")
	}
}

func TestDraftPatchSourceRepinRequiresExactCompleteRelease(t *testing.T) {
	base, in, source := patchFixture(t)
	id := in.Evidence.Identity()
	sourceID, _ := source.Root()
	sourceInput, _ := source.Input(sourceID)
	changed := sourceInput.Content
	changed.Fields[0].UIName = []byte(`{"en":"substituted source"}`)
	substituted, err := projectsnapshot.NewComplete(input(t, sourceID, changed, []projectsnapshot.Identity{}, []projectsnapshot.Membership{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projectsnapshot.PatchDraft(base, projectsnapshot.DraftPatch{Target: id, Sources: []projectsnapshot.CompleteProject{substituted}}); err == nil {
		t.Fatal("same source identity was silently replaced")
	}
	nextSourceID := identity(t, "B", "2")
	changed.Projects[0].VersionNumber = "2"
	changed.Fields[0].VersionNumber = "2"
	nextSource, err := projectsnapshot.NewComplete(input(t, nextSourceID, changed, []projectsnapshot.Identity{}, []projectsnapshot.Membership{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	prior, _ := base.DraftContext()
	next, _ := base.DraftContext()
	next.Dependencies = []projectsnapshot.Identity{nextSourceID}
	for i := range next.Membership {
		next.Membership[i].Source = nextSourceID
	}
	before := projectsnapshot.Content{Inheritance: in.Content.Inheritance, Adoptions: in.Content.Adoptions}
	after := projectsnapshot.Content{Inheritance: append([]projectsnapshot.InheritanceRecord{}, before.Inheritance...), Adoptions: append([]projectsnapshot.AdoptionRecord{}, before.Adoptions...)}
	after.Inheritance[0].SourceVersion = ptr("2")
	after.Adoptions[0].SourceVersion = "2"
	patch := projectsnapshot.DraftPatch{Target: id, Before: before, After: after, Context: &projectsnapshot.DraftContextChange{Before: prior, After: next}}
	if _, err := projectsnapshot.PatchDraft(base, patch); err == nil {
		t.Fatal("repin without exact source accepted")
	}
	patch.Sources = []projectsnapshot.CompleteProject{nextSource}
	got, err := projectsnapshot.PatchDraft(base, patch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := got.Content(sourceID); err == nil {
		t.Fatal("old unselected source leaked into patched closure")
	}
	retained, err := got.Content(nextSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(changed, retained); diff != "" {
		t.Fatal(diff)
	}
	// The original immutable source and baseline remain detached.
	original, _ := base.Content(sourceID)
	if original.Fields[0].UIName != nil {
		t.Fatal("repin changed original source")
	}
}

func TestDraftPatchSemanticChangesRequireMatchingEvidence(t *testing.T) {
	id, _ := projectsnapshot.Draft("semantic")
	body := `{"format_version":1,"ontology":{"id":"o"},"version":{"id":"v","ontology_id":"o"},"classes":[{"uri":"https://example/A"},{"uri":"https://example/B"}]}`
	doc := ontologyartifact.Document{FormatVersion: 1, OntologyID: "o", VersionID: "v", BodyJSON: body, BodyDigest: digest(body), Pins: []ontologyartifact.Pin{}, Requirements: []ontologyartifact.Requirement{}}
	raw := string(encoded(t, doc))
	artifact := ontologyartifact.Artifact{JSON: raw, Digest: digest(raw)}
	c := emptyContent(id)
	c.OntologyLinks = []projectsnapshot.OntologyLinkRecord{{ProjectID: id.ProjectID(), OntologyVersionID: "v"}}
	c.Fields = []projectsnapshot.FieldRecord{{ID: "f", ProjectID: id.ProjectID(), OntologyScope: []byte(`{"uri":"https://example/A"}`)}}
	makeEvidence := func(c projectsnapshot.Content) projectsnapshot.Evidence {
		t.Helper()
		elements, err := projectsnapshot.SemanticElements(c)
		if err != nil {
			t.Fatal(err)
		}
		inputs, err := ontologyevidence.InputReferences(elements)
		if err != nil || len(inputs) != 1 {
			t.Fatalf("semantic input missing: %v", err)
		}
		input := inputs[0]
		refs := []ontologyartifact.TermResolution{{EntityKind: input.EntityKind, EntityID: input.EntityID, Location: input.Location, Reference: ontologyartifact.TermReference(input.Reference), OntologyID: "o", VersionID: "v", Digest: artifact.Digest, URI: input.Reference.URI}}
		e, err := projectsnapshot.DraftEvidence(id, descriptors(t, id, nil, c.OntologyLinks), []ontologyartifact.ProjectArtifact{{Artifact: artifact, Linked: true}}, refs)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	base, err := projectsnapshot.NewComplete(projectsnapshot.ProjectInput{Content: c, Evidence: makeEvidence(c), Counts: counts(c), Dependencies: []projectsnapshot.Identity{}, Membership: []projectsnapshot.Membership{}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	before := c.Fields[0]
	c.Fields[0].OntologyScope = []byte(`{"uri":"https://example/B"}`)
	patch := projectsnapshot.DraftPatch{Target: id, Before: projectsnapshot.Content{Fields: []projectsnapshot.FieldRecord{before}}, After: projectsnapshot.Content{Fields: c.Fields}}
	if _, err := projectsnapshot.PatchDraft(base, patch); err == nil {
		t.Fatal("semantic edit borrowed missing or stale interpretation evidence")
	}
	prior, _ := base.DraftContext()
	next, _ := base.DraftContext()
	next.Evidence = makeEvidence(c)
	patch.Context = &projectsnapshot.DraftContextChange{Before: prior, After: next}
	got, err := projectsnapshot.PatchDraft(base, patch)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := got.Evidence(id)
	refs, _ := e.References()
	if refs[0].URI != "https://example/B" {
		t.Fatal("new semantic evidence missing")
	}
}
