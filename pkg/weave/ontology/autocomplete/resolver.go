package autocomplete

import (
	"context"
	"fmt"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
)

// ScopeResolver resolves free-text ontology scope strings (e.g.
// "E18 Physical Thing", "E18", "crm:E18_Physical_Thing") to a
// structured domain.PathElement. Backed by an in-memory cache built
// from the project's resolved ontology versions (own + inherited).
//
// Use NewScopeResolver once per project (or import session); reuse for
// all per-field scope lookups in that project.
//
// Multi-strategy match (highest-confidence first):
//  1. Exact qname match (crm:E18_Physical_Thing)
//  2. Exact LocalName match (E18_Physical_Thing)
//  3. Normalized match (case-insensitive, double-underscore collapse)
//  4. Prefix match at underscore boundary
//     (P140_assigned → P140_assigned_attribute_to)
//  5. Fuzzy containment in either direction
//
// Replaces the older ResolvePathElementWithMatchType +
// BuildMergedProjectCache flow. The regex-based ontoutil classifier is gone:
// type detection is by which map the entry lives in.
type ScopeResolver struct {
	// Class lookup maps. classes by qname for fast exact match;
	// classesByLocal for LocalName match; classesByNormalized for
	// case-insensitive + collapsed-underscore match.
	classesByQname      map[string]*domain.OntologyClass
	classesByLocal      map[string]*domain.OntologyClass
	classesByNormalized map[string]*domain.OntologyClass
	classesList         []*domain.OntologyClass

	propertiesByQname      map[string]*domain.OntologyProperty
	propertiesByLocal      map[string]*domain.OntologyProperty
	propertiesByNormalized map[string]*domain.OntologyProperty
	propertiesList         []*domain.OntologyProperty
}

// MatchKind tags how the resolver found the match. Used by the
// importer to flag "this needed a fuzzy match — please verify".
type MatchKind string

const (
	MatchExact       MatchKind = "exact"
	MatchPrefixStrip MatchKind = "prefix_stripped"
	MatchNormalized  MatchKind = "normalized"
	MatchPrefix      MatchKind = "prefix_match"
	MatchFuzzy       MatchKind = "fuzzy"
)

// ScopeMatch is the resolver's output. Element is the materialized
// PathElement; Match describes how confidence was assigned.
type ScopeMatch struct {
	Element   domain.PathElement
	Match     MatchKind
	Relevance float64 // 1.0 (exact) → 0.8 (fuzzy)
	Original  string  // input the caller passed
}

// NewScopeResolver walks the project's resolved ontology versions and
// fills the in-memory match maps. Returns ErrNoCoverage when the
// project has zero ontology classes — caller should treat that as a
// project-config bug, not a transient error.
func NewScopeResolver(ctx context.Context, store Store, projects ProjectReader, projectID string) (*ScopeResolver, error) {
	resolved, err := projects.ResolvedOntologyVersions(ctx, projectID, domain.ResolvedOntologyVersionOpts{})
	if err != nil {
		return nil, fmt.Errorf("resolve ontology versions: %w", err)
	}

	r := &ScopeResolver{
		classesByQname:         map[string]*domain.OntologyClass{},
		classesByLocal:         map[string]*domain.OntologyClass{},
		classesByNormalized:    map[string]*domain.OntologyClass{},
		propertiesByQname:      map[string]*domain.OntologyProperty{},
		propertiesByLocal:      map[string]*domain.OntologyProperty{},
		propertiesByNormalized: map[string]*domain.OntologyProperty{},
	}

	for _, rv := range resolved {
		vID := rv.Link.OntologyVersionID
		classes, err := store.ListClassesByVersion(ctx, vID)
		if err != nil {
			return nil, fmt.Errorf("list classes for version %s: %w", vID, err)
		}
		for _, c := range classes {
			r.classesByQname[c.Qname] = c
			r.classesByLocal[c.LocalName] = c
			r.classesByNormalized[normalizeForMatching(c.LocalName)] = c
			r.classesList = append(r.classesList, c)
		}
		props, err := store.ListPropertiesByVersion(ctx, vID)
		if err != nil {
			return nil, fmt.Errorf("list properties for version %s: %w", vID, err)
		}
		for _, p := range props {
			r.propertiesByQname[p.Qname] = p
			r.propertiesByLocal[p.LocalName] = p
			r.propertiesByNormalized[normalizeForMatching(p.LocalName)] = p
			r.propertiesList = append(r.propertiesList, p)
		}
	}

	if len(r.classesList) == 0 {
		return nil, ErrNoCoverage
	}
	return r, nil
}

// ErrNoCoverage means the project has no ontology classes — its
// weave_project_ontology_versions configuration is missing or empty.
var ErrNoCoverage = fmt.Errorf("autocomplete: no ontology classes for project")

// Resolve returns the best match for one of the supplied candidates,
// or nil when nothing resolves. Candidates are tried in order; first
// match wins. Used by the import tooling.
//
// Typical usage:
//
//	match := r.Resolve("E18", "E18 Physical Thing")
func (r *ScopeResolver) Resolve(candidates ...string) *ScopeMatch {
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if m := r.resolveOne(candidate); m != nil {
			return m
		}
	}
	return nil
}

func (r *ScopeResolver) resolveOne(element string) *ScopeMatch {
	normalized := normalizeForMatching(element)

	// 1. Exact qname or LocalName match (class first, then property).
	if c, ok := r.classesByQname[element]; ok {
		return &ScopeMatch{Element: classToElement(c), Match: MatchExact, Relevance: 1.0, Original: element}
	}
	if c, ok := r.classesByLocal[element]; ok {
		return &ScopeMatch{Element: classToElement(c), Match: MatchExact, Relevance: 1.0, Original: element}
	}
	if p, ok := r.propertiesByQname[element]; ok {
		return &ScopeMatch{Element: propertyToElement(p), Match: MatchExact, Relevance: 1.0, Original: element}
	}
	if p, ok := r.propertiesByLocal[element]; ok {
		return &ScopeMatch{Element: propertyToElement(p), Match: MatchExact, Relevance: 1.0, Original: element}
	}

	// 2. Strip prefix — "crm:E18_Physical_Thing" → "E18_Physical_Thing"
	bare := stripPrefix(element)
	if bare != element {
		if c, ok := r.classesByLocal[bare]; ok {
			return &ScopeMatch{Element: classToElement(c), Match: MatchPrefixStrip, Relevance: 0.95, Original: element}
		}
		if p, ok := r.propertiesByLocal[bare]; ok {
			return &ScopeMatch{Element: propertyToElement(p), Match: MatchPrefixStrip, Relevance: 0.95, Original: element}
		}
	}

	// 3. Normalized form — case-insensitive, collapsed underscores
	if c, ok := r.classesByNormalized[normalized]; ok {
		return &ScopeMatch{Element: classToElement(c), Match: MatchNormalized, Relevance: 0.9, Original: element}
	}
	if p, ok := r.propertiesByNormalized[normalized]; ok {
		return &ScopeMatch{Element: propertyToElement(p), Match: MatchNormalized, Relevance: 0.9, Original: element}
	}

	// 4. Prefix match at underscore boundary
	for _, c := range r.classesList {
		if isPrefixMatch(normalized, normalizeForMatching(c.LocalName)) {
			return &ScopeMatch{Element: classToElement(c), Match: MatchPrefix, Relevance: 0.85, Original: element}
		}
	}
	for _, p := range r.propertiesList {
		if isPrefixMatch(normalized, normalizeForMatching(p.LocalName)) {
			return &ScopeMatch{Element: propertyToElement(p), Match: MatchPrefix, Relevance: 0.85, Original: element}
		}
	}

	// 5. Fuzzy containment in either direction
	for _, c := range r.classesList {
		if fuzzyContains(normalized, normalizeForMatching(c.LocalName)) {
			return &ScopeMatch{Element: classToElement(c), Match: MatchFuzzy, Relevance: 0.8, Original: element}
		}
	}
	for _, p := range r.propertiesList {
		if fuzzyContains(normalized, normalizeForMatching(p.LocalName)) {
			return &ScopeMatch{Element: propertyToElement(p), Match: MatchFuzzy, Relevance: 0.8, Original: element}
		}
	}

	return nil
}

// ----------------------------------------------------------------------------
// Pure string helpers — replace the regex-based ontoutil classifiers.
// ----------------------------------------------------------------------------

// stripPrefix removes a "prefix:" segment from a qname-like string.
// "crm:E18_Physical_Thing" → "E18_Physical_Thing".
func stripPrefix(s string) string {
	if i := strings.Index(s, ":"); i > 0 {
		return s[i+1:]
	}
	return s
}

// normalizeForMatching strips prefix, collapses double underscores,
// and lowercases. Used as the key for the normalized map + the input
// to fuzzy/prefix matching.
func normalizeForMatching(s string) string {
	s = stripPrefix(s)
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}
	return strings.ToLower(s)
}

// isPrefixMatch returns true when 'short' is a prefix of 'full' AND
// the next character is an underscore — a real word-boundary match.
// "p140_assigned" matches "p140_assigned_attribute_to" but not
// "p140_assignedattributeto".
func isPrefixMatch(short, full string) bool {
	if len(short) >= len(full) || !strings.HasPrefix(full, short) {
		return false
	}
	return full[len(short)] == '_'
}

// fuzzyContains accepts substring containment in either direction.
func fuzzyContains(a, b string) bool {
	return strings.Contains(a, b) || strings.Contains(b, a)
}

// ----------------------------------------------------------------------------
// PathElement constructors
// ----------------------------------------------------------------------------

func classToElement(c *domain.OntologyClass) domain.PathElement {
	return domain.PathElement{
		Type:              "class",
		URI:               c.Qname, // qname form ("crm:E21_Person") matches the importer's expectation
		Prefix:            c.Prefix,
		LocalName:         c.LocalName,
		ClassCode:         shortCode(c.LocalName),
		OntologyVersionID: c.OntologyVersionID,
	}
}

func propertyToElement(p *domain.OntologyProperty) domain.PathElement {
	return domain.PathElement{
		Type:              "property",
		URI:               p.Qname,
		Prefix:            p.Prefix,
		LocalName:         p.LocalName,
		OntologyVersionID: p.OntologyVersionID,
	}
}
