package projectsnapshot_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

func TestContextCompositionRetainsOccurrencesLocksAndCapturedRefs(t *testing.T) {
	a, b := fixture(t)
	a.Content.Overrides[1].SetValue = ptr("fixed")
	a.Content.Overrides[0].SetValue = ptr("context cannot replace fixed")
	a.Content.Overrides[0].ExpectedValueType = ptr("stale type")
	a.Content.Fields[0].ExpectedValueType = ptr("literal")
	a.Content.Overrides = append(a.Content.Overrides, projectsnapshot.OverrideRecord{ID: 4, FieldID: "f", ProjectID: "A", VersionNumber: "1", EntityType: "model", EntityID: "m", Position: 8, DisplayName: []byte(`{"en":"Repeat"}`), IsRequired: ptr(true)})
	a.Content.Collections = []projectsnapshot.CollectionRecord{{ID: "empty", ProjectID: "A", VersionNumber: "1", UIName: []byte(`{"en":"Empty"}`)}}

	a.Content.Collections = append(a.Content.Collections, projectsnapshot.CollectionRecord{ID: "z", ProjectID: "A", VersionNumber: "1"}, projectsnapshot.CollectionRecord{ID: "a", ProjectID: "A", VersionNumber: "1"})
	a.Content.Overrides[0].PartOfCollectionID = ptr("z")
	a.Content.Overrides[0].Position = 1
	a.Content.Overrides[0].CollectionOrder = 4
	a.Content.Overrides[len(a.Content.Overrides)-1].PartOfCollectionID = ptr("a")
	a.Content.Overrides[len(a.Content.Overrides)-1].Position = 2
	a.Content.Overrides[len(a.Content.Overrides)-1].CollectionOrder = 4
	a.Content.Categories = []projectsnapshot.CategoryRecord{{ID: "category", ProjectID: "A", VersionNumber: "1"}}
	a.Content.Placements = []projectsnapshot.PlacementRecord{{ID: 1, ProjectID: "A", VersionNumber: "1", ModelID: "m", CollectionID: "empty"}, {ID: 2, ProjectID: "A", VersionNumber: "1", ModelID: "m", CollectionID: "empty", CategoryID: "category", MinOccurs: 2, IsHidden: true}}
	a = input(t, a.Evidence.Identity(), a.Content, a.Dependencies, a.Membership)
	complete, err := projectsnapshot.NewComplete(a, []projectsnapshot.ProjectInput{b})
	if err != nil {
		t.Fatal(err)
	}
	f, err := complete.Entity("model", "m")
	if err != nil {
		t.Fatal(err)
	}
	view, err := f.Compose()
	if err != nil {
		t.Fatal(err)
	}
	fields, placements := 0, 0
	zIndex, aIndex := -1, -1
	for i, g := range view.Groups {
		if g.Collection != nil {
			if g.Collection.ID == "z" {
				zIndex = i
			}
			if g.Collection.ID == "a" {
				aIndex = i
			}
		}
	}
	if zIndex < 0 || aIndex <= zIndex {
		t.Fatal("collection tie ignored field position")
	}
	for _, g := range view.Groups {
		if g.Placement != nil {
			placements++
			if len(g.Fields) != 0 {
				t.Fatal("empty placement gained fields")
			}
		}
		for _, field := range g.Fields {
			fields++
			if field.Body.ID == "f" {
				if field.FixedBy == nil || *field.Effective.SetValue != "fixed" || *field.Effective.SetValueEntryID != "entry" || *field.Effective.ExpectedValueType != "literal" {
					t.Fatal("base commitment or body type lost")
				}
				if field.Override.ID == 2 && (*field.Override.SetValue != "context cannot replace fixed" || *field.Override.IsRequired) {
					t.Fatal("original override changed")
				}
			}
			if field.Body.ID == "foreign" && field.Source.Source != b.Evidence.Identity() {
				t.Fatal("source context lost")
			}
			for _, ref := range field.Refs {
				if !ref.IsClosed || ref.Target.Source != a.Evidence.Identity() || len(ref.Concepts) != 1 || ref.Concepts[0].Entry.ID != "entry" || ref.ConceptNamespace != "https://concepts.example/" {
					t.Fatal("captured concept constraint lost")
				}
			}
		}
	}
	if fields != 3 || placements != 2 {
		t.Fatalf("occurrences=%d placements=%d", fields, placements)
	}
	if _, err := f.ComposeContext(a.Evidence.Identity(), "field", "unused"); err == nil {
		t.Fatal("escaped selected graph")
	}
	// Mutating any returned projection cannot affect a later composition.
	want, err := f.Compose()
	if err != nil {
		t.Fatal(err)
	}
	for i := range view.Groups {
		for j := range view.Groups[i].Fields {
			view.Groups[i].Fields[j].Body.UIName = []byte(`{}`)
			view.Groups[i].Fields[j].Effective.SetValue = ptr("changed")
		}
	}
	again, err := f.Compose()
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, again, cmp.AllowUnexported(projectsnapshot.Identity{})); diff != "" {
		t.Fatal(diff)
	}
}

func TestFragmentSubsetCannotMasqueradeAsCompleteOrPullUnrelatedAdoptions(t *testing.T) {
	a, b := fixture(t)
	b.Content.Fields = append(b.Content.Fields, projectsnapshot.FieldRecord{ID: "unrelated", ProjectID: "B", VersionNumber: "1"})
	b = input(t, b.Evidence.Identity(), b.Content, b.Dependencies, b.Membership)
	a.Membership = append(a.Membership, projectsnapshot.Membership{Kind: "field", ID: "unrelated", Source: b.Evidence.Identity()})
	a.Content.Adoptions = append(a.Content.Adoptions,
		projectsnapshot.AdoptionRecord{ProjectID: "A", VersionNumber: "1", ContextEntityType: "project", ContextEntityID: "A", EntityType: "field", SourceProjectID: "B", SourceEntityID: "foreign", SourceVersion: "1"},
		projectsnapshot.AdoptionRecord{ProjectID: "A", VersionNumber: "1", ContextEntityType: "project", ContextEntityID: "A", EntityType: "field", SourceProjectID: "B", SourceEntityID: "unrelated", SourceVersion: "1"})
	a = input(t, a.Evidence.Identity(), a.Content, a.Dependencies, a.Membership)
	full, err := projectsnapshot.NewComplete(a, []projectsnapshot.ProjectInput{b})
	if err != nil {
		t.Fatal(err)
	}
	f, err := full.Entity("field", "foreign")
	if err != nil {
		t.Fatal(err)
	}
	ac, _ := f.Content(a.Evidence.Identity())
	bc, _ := f.Content(b.Evidence.Identity())
	if len(ac.Adoptions) != 1 || ac.Adoptions[0].SourceEntityID != "foreign" || len(bc.Fields) != 1 {
		t.Fatal("metadata pulled unrelated project adoptions")
	}
	a.Content = ac
	a.Counts = counts(ac)
	b.Content = bc
	b.Counts = counts(bc)
	if _, err := projectsnapshot.NewComplete(a, []projectsnapshot.ProjectInput{b}); err == nil {
		t.Fatal("subset passed complete receipt")
	}
	bounded, err := projectsnapshot.NewFragment(a, []projectsnapshot.ProjectInput{b}, "field", "foreign")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := bounded.Content(a.Evidence.Identity())
	if diff := cmp.Diff(ac, got); diff != "" {
		t.Fatal(diff)
	}
	b.Content.Fields = nil
	b.Counts = counts(b.Content)
	if _, err := projectsnapshot.NewFragment(a, []projectsnapshot.ProjectInput{b}, "field", "foreign"); err == nil {
		t.Fatal("missing selected body accepted")
	}
}

func TestZeroCompositionAndInvalidSelectionFail(t *testing.T) {
	if _, err := (projectsnapshot.EntityFragment{}).Compose(); err == nil {
		t.Fatal("zero composition")
	}
	for _, s := range []projectsnapshot.Selection{{"project": {"A"}}, {"field": {""}}, {"unknown": {"id"}}} {
		if s.Validate() == nil {
			t.Fatal("invalid selection accepted")
		}
	}
}
