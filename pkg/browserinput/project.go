package browserinput

import (
	"encoding/json"
	"fmt"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// ProjectHeader carries selected display metadata, never authorization policy.
type ProjectHeader struct {
	Record projectsnapshot.ProjectRecord
}

// Project returns a display-only projection. Ownership and visibility are
// deliberately absent so a captured header cannot become an auth resource.
func (h ProjectHeader) Project() (*domain.Project, error) {
	r := h.Record
	if r.ID == "" {
		return nil, fmt.Errorf("project header is uninitialized")
	}
	p := &domain.Project{Entity: domain.Entity{ID: r.ID, Status: domain.Status(r.Status)}, Namespace: value(r.Namespace), ParentProjectID: copyPointer(r.ParentProjectID), License: r.License, BaseURL: r.BaseURL, Topics: append([]string(nil), r.Topics...)}
	p.SystemName = value(r.SystemName)
	for _, pair := range []struct {
		data   []byte
		target *domain.Translations
	}{{r.UIName, &p.UIName}, {r.Description, &p.Description}, {r.README, &p.README}} {
		if len(pair.data) > 0 {
			if err := json.Unmarshal(pair.data, pair.target); err != nil {
				return nil, fmt.Errorf("decode project display metadata: %w", err)
			}
		}
	}
	return p, nil
}
func value[T any](p *T) (v T) {
	if p != nil {
		return *p
	}
	return v
}

// ProjectCounts counts owned rows, plus explicit project-level adoption receipts.
type ProjectCounts struct{ Models, Collections, Fields, Categories, Examples, ConceptLists, Adoptions int }

// OntologySummary uses an exact retained version; it contains no ontology body.
type OntologySummary struct {
	Digest                                           string
	VersionID, SourceProjectID, SourceVersion        string
	Name, Version, URI                               string
	Classes, Properties, ClassesUsed, PropertiesUsed int
	UsageUnavailable                                 bool
}

// ProjectSummary is the complete bounded input for project page schemas.
// Release history and current capabilities are separate operational inputs.
type ProjectSummary struct {
	Header     ProjectHeader
	Counts     ProjectCounts
	Credits    []domain.Attribution
	Ontologies []OntologySummary
}

func copyPointer[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
