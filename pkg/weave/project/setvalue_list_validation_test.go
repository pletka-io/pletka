package project

import (
	"context"
	"log/slog"
	"testing"
)

type fakeConceptCheck struct{ allowed map[string]bool }

func (f fakeConceptCheck) ConceptURIInLists(_ context.Context, uri string, _ []string) (bool, error) {
	return f.allowed[uri], nil
}

// TestValidateSetValueAgainstLists proves a set_value on a list-bound field is
// rejected unless it's a list member; fields without a set_value or without a
// bound list are ignored (#3599).
func TestValidateSetValueAgainstLists(t *testing.T) {
	h := &Handler{log: slog.Default(), conceptCheck: fakeConceptCheck{allowed: map[string]bool{"uri:ok": true}}}

	categories := []overrideEditorCategory{{
		Items: []overrideEditorItem{{
			Fields: []overrideEditorField{
				{FieldID: "F1", SetValue: "uri:ok", ExpectedConceptLists: []string{"CL"}},  // member -> ok
				{FieldID: "F2", SetValue: "uri:bad", ExpectedConceptLists: []string{"CL"}}, // not a member -> error
				{FieldID: "F3", SetValue: "uri:bad"},                                       // no list -> skip
				{FieldID: "F4", ExpectedConceptLists: []string{"CL"}},                      // no set_value -> skip
			},
		}},
	}}

	errs := h.validateSetValueAgainstLists(context.Background(), categories)
	if len(errs) != 1 {
		t.Fatalf("expected exactly one field error, got %v", errs)
	}
	if _, ok := errs["F2"]; !ok {
		t.Fatalf("expected F2 to be rejected, got %v", errs)
	}
}
