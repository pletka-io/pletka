package formschema

import "testing"

func TestLifecycleRowActions(t *testing.T) {
	actions := lifecycleRowActions("AME", "models")

	byID := map[string]RowAction{}
	for _, a := range actions {
		byID[a.ID] = a
	}
	for _, id := range []string{"edit", "deprecate", "activate", "delete"} {
		if _, ok := byID[id]; !ok {
			t.Errorf("missing row action %q", id)
		}
	}

	// deprecate/activate carry POST URLs gated by the per-row flags.
	if got := byID["deprecate"]; got.URLTemplate != "/projects/AME/models/{id}/deprecate" || got.Method != "POST" || got.VisibleWhen != "can_deprecate" {
		t.Errorf("deprecate action wrong: %+v", got)
	}
	if got := byID["activate"]; got.URLTemplate != "/projects/AME/models/{id}/activate" || got.VisibleWhen != "can_activate" {
		t.Errorf("activate action wrong: %+v", got)
	}
	// delete is owner-only, danger-styled, and carries no URL (uses the
	// Delete capability).
	if got := byID["delete"]; got.VisibleWhen != "owned && !in_use" || got.Style != "danger" || got.URLTemplate != "" {
		t.Errorf("delete action wrong: %+v", got)
	}
	// edit is owner-only so non-owned (inherited/adopted) rows don't offer it.
	if byID["edit"].VisibleWhen != "owned" {
		t.Errorf("edit should be owner-gated, got %q", byID["edit"].VisibleWhen)
	}
}
