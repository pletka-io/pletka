package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConceptListIsClosedMarshals(t *testing.T) {
	b, err := json.Marshal(ConceptList{IsClosed: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"is_closed":true`) {
		t.Fatalf("expected is_closed true, got %s", b)
	}
}

func TestConceptBroaderEdgeSchemeOmitempty(t *testing.T) {
	// nil scheme -> global edge, scheme_id omitted.
	b, err := json.Marshal(ConceptBroaderEdge{ID: "e1", ConceptID: "c1", BroaderID: "b1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "scheme_id") {
		t.Fatalf("expected scheme_id omitted for a global edge, got %s", b)
	}
	// scheme set -> present.
	s := "S1"
	b2, err := json.Marshal(ConceptBroaderEdge{ID: "e1", ConceptID: "c1", BroaderID: "b1", SchemeID: &s})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b2), `"scheme_id":"S1"`) {
		t.Fatalf("expected scheme_id present, got %s", b2)
	}
}
