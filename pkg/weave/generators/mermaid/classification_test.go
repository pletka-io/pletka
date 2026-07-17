package mermaid

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func TestDefaultClassificationConfigIsEmbedded(t *testing.T) {
	cfg, err := DefaultClassificationConfig()
	if err != nil {
		t.Fatalf("DefaultClassificationConfig() error: %v", err)
	}
	if len(cfg.Groups) == 0 {
		t.Fatal("default config has no groups")
	}
	if got := cfg.ClassGroupsByURI()["http://www.cidoc-crm.org/cidoc-crm/E39_Actor"]; got != "Actor" {
		t.Fatalf("E39 group = %q, want Actor", got)
	}
}

func TestMissingClassificationsReportsMissingAndEmptyGroups(t *testing.T) {
	classes := []*domain.OntologyClass{
		{
			URI:       "http://example.org/ontology/Classified",
			Qname:     "ex:Classified",
			LocalName: "Classified",
		},
		{
			URI:       "http://example.org/ontology/Empty",
			Qname:     "ex:Empty",
			LocalName: "Empty",
		},
		{
			URI:       "http://example.org/ontology/Missing",
			Qname:     "ex:Missing",
			LocalName: "Missing",
		},
	}
	cfg := ClassificationConfig{
		Classes: []ClassClassification{
			{URI: "http://example.org/ontology/Classified", Group: "Actor"},
			{URI: "http://example.org/ontology/Empty"},
		},
	}

	report := MissingClassifications(nil, nil, classes, cfg)
	if len(report.MissingClasses) != 2 {
		t.Fatalf("missing count = %d, want 2: %#v", len(report.MissingClasses), report.MissingClasses)
	}
	if report.MissingClasses[0].Reason != "empty_group" {
		t.Fatalf("first reason = %q, want empty_group", report.MissingClasses[0].Reason)
	}
	if report.MissingClasses[1].Reason != "missing" {
		t.Fatalf("second reason = %q, want missing", report.MissingClasses[1].Reason)
	}
}
