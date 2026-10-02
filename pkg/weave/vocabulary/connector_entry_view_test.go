package vocabulary

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/vocabconnector"
)

// TestConnectorEntryView_carriesTreeFields proves the roots/children-specific
// fields (descendants_total, has_children, browse, note) survive the map from
// a connector Entry into the API view, and that order is preserved.
func TestConnectorEntryView_carriesTreeFields(t *testing.T) {
	n, d := 15, 498
	yes := true
	rows := []vocabconnector.Entry{
		{
			URI:              "http://x/gender",
			Label:            domain.Translations{"en": "Gender"},
			NarrowerTotal:    &n,
			DescendantsTotal: &n,
			Browse:           "descendants",
			Note:             "15 below",
		},
		{
			URI:              "http://x/colours",
			Label:            domain.Translations{"en": "Colours"},
			DescendantsTotal: &d,
			HasChildren:      &yes,
		},
	}
	views := connectorEntryViews("V1", rows)
	if len(views) != 2 {
		t.Fatalf("len = %d", len(views))
	}
	if views[0].URI != "http://x/gender" || views[1].URI != "http://x/colours" {
		t.Fatalf("order not preserved: %q, %q", views[0].URI, views[1].URI)
	}
	g := views[0]
	if g.VocabularyID != "V1" || g.Browse != "descendants" || g.Note != "15 below" {
		t.Errorf("gender view = %+v", g)
	}
	if g.DescendantsTotal == nil || *g.DescendantsTotal != 15 {
		t.Errorf("gender descendantsTotal = %v", g.DescendantsTotal)
	}
	c := views[1]
	if c.HasChildren == nil || !*c.HasChildren {
		t.Errorf("colours hasChildren = %v", c.HasChildren)
	}
	if c.DescendantsTotal == nil || *c.DescendantsTotal != 498 {
		t.Errorf("colours descendantsTotal = %v", c.DescendantsTotal)
	}
}
