package project

import "testing"

func TestFlattenOverrideDraftKeepsRealIDs(t *testing.T) {
	cats := []overrideEditorCategory{{
		Items: []overrideEditorItem{{
			Fields: []overrideEditorField{
				{FieldID: "F1", OverrideID: 42, Position: 1},
				{FieldID: "F2", OverrideID: -3, Position: 2},
			},
		}},
	}}
	rows := flattenOverrideDraft("P1", "model", "M1", cats)
	if rows[0].override.ID != 42 || rows[1].override.ID != 0 {
		t.Fatalf("ids = %d, %d; want 42, 0", rows[0].override.ID, rows[1].override.ID)
	}
}
