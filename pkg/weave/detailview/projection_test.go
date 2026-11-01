package detailview

import (
	"reflect"
	"testing"

	"github.com/pletka-io/pletka/internal/testsnapshot"
	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/domain"
)

func TestBrowserProjectionPreservesRawPathsAndRequiresExplicitFieldCounts(t *testing.T) {
	project := domain.Project{Entity: domain.Entity{ID: "P", UIName: domain.Translations{"en": "Project"}}}
	model := domain.Model{Entity: domain.Entity{ID: "M", ProjectID: "P"}}
	path := []domain.PathElement{{Type: "class", URI: "raw:Class", Prefix: "raw", LocalName: "Class"}}
	field := domain.ResolvedField{ID: "F", SystemName: "Field", DisplayName: domain.Translations{"en": "Context label"}, PathElements: path}
	view := domain.ModelView{Categories: []domain.CategoryGroup{{Collections: []domain.CollectionGroup{{ID: "__direct__", Fields: []domain.ResolvedField{field}}}}}}
	value := testsnapshot.GeneratorValue(t, project, model, view, domain.Collection{}, nil)
	fragment, err := value.Entity("model", "M")
	if err != nil {
		t.Fatal(err)
	}
	projected, err := projectDetail(browserinput.Detail{Fragment: fragment})
	if err != nil {
		t.Fatal(err)
	}
	got := projected.Response.Sections[0].Items[0].Fields[0]
	if !reflect.DeepEqual(got.PathElements, path) || got.DisplayName["en"] != "Context label" || got.SourceFieldURL != "/projects/P/fields/F" {
		t.Fatalf("raw editor data was interpreted or linked by guessed semantic ID: %+v", got)
	}
	consumer, err := fragment.Consumer()
	if err != nil {
		t.Fatal(err)
	}
	content, err := value.Content(consumer)
	if err != nil {
		t.Fatal(err)
	}
	// A field body without a base is legitimate editing state. Its usage
	// counts cannot be inferred from the partial structural graph.
	content.Overrides = content.Overrides[:1]
	value = testsnapshot.Complete(t, content)
	fragment, err = value.Entity("field", "F")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projectDetail(browserinput.Detail{Fragment: fragment}); err == nil {
		t.Fatal("missing aggregate became fabricated zero counts")
	}
	projected, err = projectDetail(browserinput.Detail{Fragment: fragment, FieldUsage: &domain.FieldUsageCounts{ModelsUsing: 1, OverrideRowCount: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if projected.Response.Entity.SystemName != "Field" || projected.Response.Entity.SetValue != "" || projected.Stats.ModelsUsing != 1 {
		t.Fatalf("field without a base: %+v", projected)
	}
	if _, err := fragment.Resolve(consumer, "field", "outside"); err == nil {
		t.Fatal("fragment resolution discovered unselected content")
	}
}

func TestBrowserStatsAndRowsShareVisibility(t *testing.T) {
	project := domain.Project{Entity: domain.Entity{ID: "P"}}
	model := domain.Model{Entity: domain.Entity{ID: "M", ProjectID: "P"}}
	fields := []domain.ResolvedField{{ID: "F", IsRequired: true}, {ID: "H", IsHidden: true}}
	view := domain.ModelView{Categories: []domain.CategoryGroup{{Collections: []domain.CollectionGroup{{ID: "__direct__", Fields: fields}}}}}
	value := testsnapshot.GeneratorValue(t, project, model, view, domain.Collection{}, nil)
	fragment, err := value.Entity("model", "M")
	if err != nil {
		t.Fatal(err)
	}
	projected, err := projectDetail(browserinput.Detail{Fragment: fragment})
	if err != nil {
		t.Fatal(err)
	}
	if projected.Stats.TotalFields != 1 || projected.Stats.RequiredFields != 1 || projected.Response.Sections[0].Items[0].FieldCount != 1 {
		t.Fatalf("hidden field leaked into rows or stats: %+v", projected)
	}
}
