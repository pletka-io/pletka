package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPathElementCompleteOmitemptyRoundTrip(t *testing.T) {
	// Complete=false must not serialize (omitempty), so existing stored
	// elements without the field are unaffected.
	plain := PathElement{Type: "class", URI: "crm:E21_Person", Prefix: "crm", LocalName: "E21_Person", Position: 0}
	b, err := json.Marshal(plain)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "complete") {
		t.Fatalf("Complete=false must be omitted, got %s", b)
	}

	// Complete=true serializes as "complete":true and round-trips.
	done := PathElement{Type: "class", URI: "skos:Concept", Prefix: "skos", LocalName: "Concept", Position: 1, Complete: true}
	b, err = json.Marshal(done)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"complete":true`) {
		t.Fatalf("Complete=true must serialize, got %s", b)
	}
	var back PathElement
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !back.Complete {
		t.Fatalf("Complete did not round-trip: %+v", back)
	}
}
