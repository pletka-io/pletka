package readsession

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/database/readsql/archive"
	"github.com/pletka-io/pletka/pkg/database/readsql/draft"
	"github.com/pletka-io/pletka/pkg/database/readsql/projectontologies"
	"github.com/pletka-io/pletka/pkg/domain"
)

// SummaryLimit bounds non-paginated metadata inventories. Overflow is an error.
const SummaryLimit = 1000

// ProjectHeader reads only one owned metadata row and validates its capture.
func (s *Session) ProjectHeader(ctx context.Context) (browserinput.ProjectHeader, error) {
	if err := s.Check(ctx); err != nil {
		return browserinput.ProjectHeader{}, err
	}
	if s.Scope().IsRelease() {
		contract, err := s.ContentContract(ctx)
		if err != nil {
			return browserinput.ProjectHeader{}, err
		}
		switch contract {
		case 1:
			_, err = s.Content(ctx)
		case 2:
			_, err = s.LegacyContent(ctx)
		default:
			err = &NotInReleaseError{ProjectID: s.ProjectID(), Version: s.Scope().Version(), EntityID: "complete content contract"}
		}
		if err != nil {
			return browserinput.ProjectHeader{}, err
		}
	}
	rows, err := s.ContentRows()
	if err != nil {
		return browserinput.ProjectHeader{}, err
	}
	projects, err := rows.ContentProjects(ctx, Page{Limit: 2})
	if err != nil {
		return browserinput.ProjectHeader{}, err
	}
	if len(projects) != 1 || projects[0].ID != s.ProjectID() || projects[0].VersionNumber != s.Scope().Version() {
		return browserinput.ProjectHeader{}, fmt.Errorf("selected project header is missing or mismatched")
	}
	return browserinput.ProjectHeader{Record: projects[0]}, nil
}

// ProjectCounts performs aggregate queries; no entity inventory is materialized.
func (s *Session) ProjectCounts(ctx context.Context) (browserinput.ProjectCounts, error) {
	if err := s.Check(ctx); err != nil {
		return browserinput.ProjectCounts{}, err
	}
	if s.Scope().IsRelease() {
		q, err := s.ArchiveQueries()
		if err != nil {
			return browserinput.ProjectCounts{}, err
		}
		r, err := q.BrowserProjectCounts(ctx, archive.BrowserProjectCountsParams{ProjectID: s.ProjectID(), Version: s.Scope().Version()})
		return browserinput.ProjectCounts{Models: int(r.Models), Collections: int(r.Collections), Fields: int(r.Fields), Categories: int(r.Categories), Examples: int(r.Examples), ConceptLists: int(r.ConceptLists), Adoptions: int(r.Adoptions)}, err
	}
	q, err := s.DraftQueries()
	if err != nil {
		return browserinput.ProjectCounts{}, err
	}
	r, err := q.BrowserProjectCounts(ctx, s.ProjectID())
	return browserinput.ProjectCounts{Models: int(r.Models), Collections: int(r.Collections), Fields: int(r.Fields), Categories: int(r.Categories), Examples: int(r.Examples), ConceptLists: int(r.ConceptLists), Adoptions: int(r.Adoptions)}, err
}

// ProjectCredits joins selected credit rows to descriptors on the same snapshot.
func (s *Session) ProjectCredits(ctx context.Context) ([]domain.Attribution, error) {
	if err := s.Check(ctx); err != nil {
		return nil, err
	}
	out := []domain.Attribution{}
	if s.Scope().IsRelease() {
		q, err := s.ArchiveQueries()
		if err != nil {
			return nil, err
		}
		rows, err := q.BrowserProjectCredits(ctx, archive.BrowserProjectCreditsParams{ProjectID: s.ProjectID(), Version: s.Scope().Version(), RowLimit: SummaryLimit + 1})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if !r.Present {
				return nil, fmt.Errorf("credit actor %s missing from captured descriptors", r.ActorID)
			}
			out = append(out, domain.Attribution{ProjectID: s.ProjectID(), ActorID: r.ActorID, Kind: r.Kind, Position: int(r.Position), Note: r.Note, ActorName: r.ActorName, ActorType: r.ActorType})
		}
	} else {
		q, err := s.DraftQueries()
		if err != nil {
			return nil, err
		}
		rows, err := q.BrowserProjectCredits(ctx, draft.BrowserProjectCreditsParams{ProjectID: s.ProjectID(), RowLimit: SummaryLimit + 1})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if !r.Present {
				return nil, fmt.Errorf("credit actor %s is missing", r.ActorID)
			}
			out = append(out, domain.Attribution{ProjectID: s.ProjectID(), ActorID: r.ActorID, Kind: r.Kind, Position: int(r.Position), Note: r.Note, ActorName: r.ActorName, ActorType: r.ActorType})
		}
	}
	if len(out) > SummaryLimit {
		return nil, fmt.Errorf("project credit summary exceeds %d rows", SummaryLimit)
	}
	return out, nil
}

// OwnOntologySummaries reads linked metadata from captured ontology evidence.
// Immutable body JSON is projected in SQL; bodies never cross this boundary.
func (s *Session) OwnOntologySummaries(ctx context.Context) ([]browserinput.OntologySummary, error) {
	if err := s.Check(ctx); err != nil {
		return nil, err
	}
	out := []browserinput.OntologySummary{}
	add := func(id string, raw []byte, present bool) error {
		if !present {
			return fmt.Errorf("ontology %s is missing from selected evidence", id)
		}
		var m struct {
			Name, Version, URI, Digest string
			Label                      domain.Translations
			Classes, Properties        int
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			return err
		}
		if len(m.Digest) != 64 {
			return fmt.Errorf("ontology %s has no retained body identity", id)
		}
		if m.Name == "" {
			m.Name = m.Label.Get("en", m.URI)
		}
		if m.Name == "" {
			m.Name = id
		}
		out = append(out, browserinput.OntologySummary{VersionID: id, Digest: m.Digest, SourceProjectID: s.ProjectID(), SourceVersion: s.Scope().Version(), Name: m.Name, Version: m.Version, URI: m.URI, Classes: m.Classes, Properties: m.Properties})
		return nil
	}
	if s.Scope().IsRelease() {
		q := s.ontologies
		present, err := q.BrowserSummaryEvidence(ctx, projectontologies.BrowserSummaryEvidenceParams{ProjectID: s.ProjectID(), Version: s.Scope().Version()})
		if err != nil {
			return nil, err
		}
		if !present {
			return nil, &NotInReleaseError{ProjectID: s.ProjectID(), Version: s.Scope().Version(), EntityID: "ontology summary evidence"}
		}
		rows, err := q.BrowserOwnOntologySummaries(ctx, projectontologies.BrowserOwnOntologySummariesParams{ProjectID: s.ProjectID(), Version: s.Scope().Version(), RowLimit: SummaryLimit + 1})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if err := add(r.OntologyVersionID, r.Metadata, r.Present); err != nil {
				return nil, err
			}
		}
	} else {
		q, err := s.DraftQueries()
		if err != nil {
			return nil, err
		}
		rows, err := q.BrowserOwnOntologySummaries(ctx, draft.BrowserOwnOntologySummariesParams{ProjectID: s.ProjectID(), RowLimit: SummaryLimit + 1})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if err := add(r.OntologyVersionID, r.Metadata, r.Present); err != nil {
				return nil, err
			}
		}
	}
	if len(out) > SummaryLimit {
		return nil, fmt.Errorf("ontology summary exceeds %d rows", SummaryLimit)
	}
	return out, nil
}

// OntologyUsage is a compact aggregate over selected root-project references.
type OntologyUsage struct {
	Classes, Properties int
	Known               bool
}

// OntologyUsages scans root references once for all selected ontology versions.
// Legacy ambiguity is explicit; callers must not display unknown usage as zero.
func (s *Session) OntologyUsages(ctx context.Context, versions []string) (map[string]OntologyUsage, error) {
	if err := s.Check(ctx); err != nil {
		return nil, err
	}
	if !s.Scope().IsRelease() {
		return nil, fmt.Errorf("captured ontology usage requires a release session")
	}
	if len(versions) > SummaryLimit {
		return nil, fmt.Errorf("ontology usage summary limit exceeded")
	}
	out := make(map[string]OntologyUsage, len(versions))
	rows, err := s.ontologies.BrowserOntologyUsage(ctx, projectontologies.BrowserOntologyUsageParams{ProjectID: s.ProjectID(), Version: s.Scope().Version(), OntologyVersions: versions})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.VersionID] = OntologyUsage{Classes: int(r.Classes), Properties: int(r.Properties), Known: r.Known}
	}
	if len(out) != len(versions) {
		return nil, fmt.Errorf("ontology usage summary is incomplete")
	}
	return out, nil
}

// DraftSummaryReferences normalizes only authored root references, not entity bodies.
func (s *Session) DraftSummaryReferences(ctx context.Context) ([]byte, error) {
	if err := s.Check(ctx); err != nil {
		return nil, err
	}
	q, err := s.DraftQueries()
	if err != nil {
		return nil, err
	}
	rows, err := q.BrowserOntologyReferences(ctx, s.ProjectID())
	if err != nil {
		return nil, err
	}
	if len(rows) > 10000 {
		return nil, fmt.Errorf("project summary exceeds 10000 distinct semantic references")
	}
	refs := make([]json.RawMessage, 0, len(rows))
	refs = append(refs, rows...)
	return json.Marshal(refs)
}

// MatchOntologyReferences counts authored references against this source's exact
// retained bodies. It cannot substitute a globally sealed body for a source cut.
func (s *Session) MatchOntologyReferences(ctx context.Context, versions []string, refs []byte) (map[string]OntologyUsage, error) {
	if err := s.Check(ctx); err != nil {
		return nil, err
	}
	if len(versions) > SummaryLimit {
		return nil, fmt.Errorf("ontology usage summary limit exceeded")
	}
	out := make(map[string]OntologyUsage, len(versions))
	if s.Scope().IsRelease() {
		rows, err := s.ontologies.BrowserMatchOntologyReferences(ctx, projectontologies.BrowserMatchOntologyReferencesParams{ProjectID: s.ProjectID(), Version: s.Scope().Version(), OntologyVersions: versions, ReferencesJson: refs})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out[r.VersionID] = OntologyUsage{Classes: int(r.Classes), Properties: int(r.Properties), Known: true}
		}
	} else {
		q, err := s.DraftQueries()
		if err != nil {
			return nil, err
		}
		rows, err := q.BrowserMatchOntologyReferences(ctx, draft.BrowserMatchOntologyReferencesParams{OntologyVersions: versions, ReferencesJson: refs})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out[r.VersionID] = OntologyUsage{Classes: int(r.Classes), Properties: int(r.Properties), Known: true}
		}
	}
	if len(out) != len(versions) {
		return nil, fmt.Errorf("ontology reference summary is incomplete")
	}
	return out, nil
}
