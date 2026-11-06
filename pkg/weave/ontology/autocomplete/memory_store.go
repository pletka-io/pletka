package autocomplete

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/pletka-io/pletka/pkg/domain"
)

// MemoryVersion is one ontology version held in memory: the rows the index
// is built from, with Qname and SourceQname already filled (the database
// computes them as generated columns; an offline caller fills them itself).
type MemoryVersion struct {
	Version    *domain.OntologyVersion
	Ontology   *domain.Ontology
	Classes    []*domain.OntologyClass
	Properties []*domain.OntologyProperty
	Relations  []domain.OntologyRelationWithSource
}

// MemoryStore serves the index from MemoryVersions. It is the second reader
// of the one rule set: the same buildIndex runs over it as over Postgres.
type MemoryStore struct {
	versions   map[string]MemoryVersion
	ontologies map[string]*domain.Ontology
	byPrefix   map[string]*domain.Ontology
	active     map[string]*domain.OntologyVersion
}

// NewMemoryStore indexes the given versions. For an ontology given several
// times, the lexically highest VersionString is the active one.
func NewMemoryStore(versions ...MemoryVersion) *MemoryStore {
	s := &MemoryStore{
		versions:   map[string]MemoryVersion{},
		ontologies: map[string]*domain.Ontology{},
		byPrefix:   map[string]*domain.Ontology{},
		active:     map[string]*domain.OntologyVersion{},
	}
	for _, v := range versions {
		s.versions[v.Version.ID] = v
		s.ontologies[v.Ontology.ID] = v.Ontology
		s.byPrefix[v.Ontology.Prefix] = v.Ontology
		if cur := s.active[v.Ontology.ID]; cur == nil || cur.VersionString < v.Version.VersionString {
			s.active[v.Ontology.ID] = v.Version
		}
	}
	return s
}

// VersionIDs lists every version in the store, sorted.
func (s *MemoryStore) VersionIDs() []string {
	ids := make([]string, 0, len(s.versions))
	for id := range s.versions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// ListClassesByVersions returns the classes of the given versions.
func (s *MemoryStore) ListClassesByVersions(_ context.Context, versionIDs []string) ([]*domain.OntologyClass, error) {
	var out []*domain.OntologyClass
	for _, id := range versionIDs {
		out = append(out, s.versions[id].Classes...)
	}
	return out, nil
}

// ListPropertiesByVersions returns the properties of the given versions.
func (s *MemoryStore) ListPropertiesByVersions(_ context.Context, versionIDs []string) ([]*domain.OntologyProperty, error) {
	var out []*domain.OntologyProperty
	for _, id := range versionIDs {
		out = append(out, s.versions[id].Properties...)
	}
	return out, nil
}

// ListRelationsByVersionsAndTypes returns the relations of the given versions
// whose type is among relTypes (all types when relTypes is empty).
func (s *MemoryStore) ListRelationsByVersionsAndTypes(_ context.Context, versionIDs, relTypes []string) ([]*domain.OntologyRelationWithSource, error) {
	want := make(map[string]bool, len(relTypes))
	for _, t := range relTypes {
		want[t] = true
	}
	var out []*domain.OntologyRelationWithSource
	for _, id := range versionIDs {
		rels := s.versions[id].Relations
		for i := range rels {
			if len(want) == 0 || want[rels[i].RelType] {
				out = append(out, &rels[i])
			}
		}
	}
	return out, nil
}

// GetVersion returns the version with the given id.
func (s *MemoryStore) GetVersion(_ context.Context, id string) (*domain.OntologyVersion, error) {
	v, ok := s.versions[id]
	if !ok {
		return nil, fmt.Errorf("version %s not in memory store", id)
	}
	return v.Version, nil
}

// GetOntology returns the ontology with the given id.
func (s *MemoryStore) GetOntology(_ context.Context, id string) (*domain.Ontology, error) {
	o, ok := s.ontologies[id]
	if !ok {
		return nil, fmt.Errorf("ontology %s not in memory store", id)
	}
	return o, nil
}

// GetOntologyByPrefix returns the ontology registered under prefix, or nil.
func (s *MemoryStore) GetOntologyByPrefix(_ context.Context, prefix string) (*domain.Ontology, error) {
	return s.byPrefix[prefix], nil
}

// GetActiveVersion returns the highest version of the ontology, or nil.
func (s *MemoryStore) GetActiveVersion(_ context.Context, ontologyID string) (*domain.OntologyVersion, error) {
	return s.active[ontologyID], nil
}

// OfflineEngine steps through one fixed index the way the builder does.
type OfflineEngine struct {
	idx *Index
}

// NewOfflineEngine builds the index over every version in the store, with
// no primary version and the given namespace bindings.
func NewOfflineEngine(ctx context.Context, store *MemoryStore, bindings []NamespaceBinding, log *slog.Logger) (*OfflineEngine, error) {
	if log == nil {
		log = slog.Default()
	}
	ids := store.VersionIDs()
	var fn NamespaceBindingsFunc
	if len(bindings) > 0 {
		fn = func(context.Context) ([]NamespaceBinding, error) { return bindings, nil }
	}
	idx, err := buildIndex(ctx, store, fn, keyForRequest(ids, "", "live"), ids, "", log)
	if err != nil {
		return nil, fmt.Errorf("build offline index: %w", err)
	}
	return &OfflineEngine{idx: idx}, nil
}

// Suggest answers one builder question against the fixed index. Its shape
// is the Suggester the path replay takes.
func (e *OfflineEngine) Suggest(_ context.Context, req Request) ([]Suggestion, error) {
	if req.MaxResults <= 0 {
		req.MaxResults = 50
	}
	return (&IndexedEngine{}).suggestFromIndex(e.idx, req)
}

// Index exposes the built index for callers that look terms up directly.
func (e *OfflineEngine) Index() *Index { return e.idx }
