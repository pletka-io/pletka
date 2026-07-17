package formschema

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/i18n"
)

// lifecycleRowActions builds the edit/stats/deprecate/activate/delete row
// actions shared by the model, collection, and field browse lists so the
// EntityListView surfaces the same lifecycle the detail-page kebab does.
//
// entityPath is the plural route segment ("models"/"collections"/"fields").
// The visibility keys (owned/can_deprecate/can_activate) are per-row booleans
// the list data endpoints emit. edit and stats are handled in the frontend
// (open the inline form / stats modal); deprecate and activate POST their
// URLs; delete carries no URL and uses the schema's Delete capability.
// deprecatedFlagBadge is the red "Deprecated" pill a list row shows when the
// entity's deprecated flag is set. status and deprecated are orthogonal (a
// draft entity can be deprecated), so this is separate from the status badge —
// the "flag" badge type renders its label only when the keyed field is truthy.
func deprecatedFlagBadge() BadgeConfig {
	return BadgeConfig{Key: "deprecated", Type: "flag", Label: i18n.L("common.deprecated", "Deprecated"), Style: "red"}
}

func lifecycleRowActions(projectID, entityPath string) []RowAction {
	base := fmt.Sprintf("/projects/%s/%s/{id}", projectID, entityPath)
	// No "stats" action: the row-level StatsModal is entity-shape-specific and
	// the canonical stats view is the detail page's Statistics tab.
	return []RowAction{
		{ID: "edit", Icon: "pencil", Label: i18n.L("common.edit", "Edit"), VisibleWhen: "owned"},
		{
			ID:          "deprecate",
			Icon:        "archive-box",
			Label:       i18n.L("common.deprecate", "Deprecate"),
			VisibleWhen: "can_deprecate",
			URLTemplate: base + "/deprecate",
			Method:      "POST",
		},
		{
			ID:          "activate",
			Icon:        "arrow-uturn-up",
			Label:       i18n.L("common.activate", "Activate"),
			VisibleWhen: "can_activate",
			URLTemplate: base + "/activate",
			Method:      "POST",
		},
		{
			ID:    "delete",
			Icon:  "trash",
			Label: i18n.L("common.delete", "Delete"),
			Style: "danger",
			// Owned AND not in use: a linked entity shows Deprecate only. Rows
			// that don't emit in_use (models/collections today) fall back to
			// owned, since a missing flag reads as not-in-use.
			VisibleWhen: "owned && !in_use",
		},
	}
}
