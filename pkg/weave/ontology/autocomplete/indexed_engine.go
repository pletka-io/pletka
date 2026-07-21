package autocomplete

import (
	"context"
	"strings"
)

// IndexedEngine answers suggestions by walking a cache-built pointer graph.
// Zero DB round-trips on the hot path once the index is warm.
type IndexedEngine struct {
	cache *IndexCache
}

// NewIndexed constructs an IndexedEngine over cache.
func NewIndexed(cache *IndexCache) *IndexedEngine { return &IndexedEngine{cache: cache} }

// GetSuggestions mirrors DirectEngine.GetSuggestions semantics over the index.
func (e *IndexedEngine) GetSuggestions(ctx context.Context, req Request) ([]Suggestion, error) {
	if req.MaxResults <= 0 {
		req.MaxResults = 50
	}
	idx, err := e.cache.For(ctx, req)
	if err != nil {
		return nil, err
	}
	if idx == nil {
		return nil, nil
	}

	if len(req.CurrentPath) == 0 {
		return e.suggestRootClasses(idx, req), nil
	}
	last := strings.TrimSpace(req.CurrentPath[len(req.CurrentPath)-1])
	if last == "" {
		return nil, nil
	}
	n := idx.ByQname[last]
	if n == nil || n.Ghost {
		return nil, nil
	}
	switch n.Type {
	case "class":
		return e.suggestPropertiesForClass(idx, n, req), nil
	case "property":
		return e.suggestRangeForProperty(idx, n, req), nil
	}
	return nil, nil
}

// suggestRootClasses returns all non-ghost class nodes that match the query.
// The filter mirrors DirectEngine's SearchClasses SQL which matches on
// local_name OR qname OR label — so a query that only matches the qname
// prefix (e.g. "aaao:") must also pass here.
func (e *IndexedEngine) suggestRootClasses(idx *Index, req Request) []Suggestion {
	q := strings.TrimSpace(req.Query)
	out := make([]Suggestion, 0)
	for _, n := range idx.ByQname {
		if n.Type != "class" || n.Ghost {
			continue
		}
		if q != "" && !matchesQuery(n.LocalName, n.Label, q) && !strings.Contains(strings.ToLower(n.Qname), strings.ToLower(q)) {
			continue
		}
		out = append(out, nodeToSuggestion(n))
		if len(out) >= req.MaxResults {
			break
		}
	}
	return out
}

// suggestPropertiesForClass returns properties whose domain includes class or
// any ancestor in the full cross-version lineage.
func (e *IndexedEngine) suggestPropertiesForClass(idx *Index, class *Node, req Request) []Suggestion {
	q := strings.TrimSpace(req.Query)
	// Lineage = class + all superclass ancestors (cross-version edges already
	// wired into the graph, so a single closure spans ontologies).
	lineage := append([]*Node{class},
		closure(class, func(n *Node) []*Node { return n.Superclasses }, ClosureOpts{})...)
	for _, extra := range atScope(req) {
		if en := idx.ByQname[extra]; en != nil && !en.Ghost {
			lineage = append(lineage, en)
			lineage = append(lineage,
				closure(en, func(n *Node) []*Node { return n.Superclasses }, ClosureOpts{})...)
		}
	}
	seen := map[string]struct{}{}
	out := make([]Suggestion, 0)
	for _, anc := range lineage {
		for _, p := range anc.AsDomainOf {
			if p.Ghost {
				continue
			}
			if _, dup := seen[p.Qname]; dup {
				continue
			}
			if q != "" && !matchesQuery(p.LocalName, p.Label, q) {
				continue
			}
			seen[p.Qname] = struct{}{}
			out = append(out, nodeToSuggestion(p))
			if len(out) >= req.MaxResults {
				return out
			}
		}
	}
	return out
}

// suggestRangeForProperty returns class suggestions from the property's range
// relations, including subclasses for each real range target. For
// DatatypeProperty nodes it reproduces DirectEngine's literal suggestion
// logic exactly.
func (e *IndexedEngine) suggestRangeForProperty(idx *Index, prop *Node, req Request) []Suggestion {
	q := strings.TrimSpace(req.Query)
	isDatatype := prop.PropertyType == "DatatypeProperty"
	seen := map[string]struct{}{}
	out := make([]Suggestion, 0)
	for _, rng := range prop.Ranges {
		if rng.Ghost {
			// Dangling range target — for a DatatypeProperty this is a literal.
			if isDatatype {
				s := literalSuggestionFromQname(rng.Qname)
				if _, dup := seen[s.Qname]; !dup && (q == "" || matchesQuery(s.LocalName, s.Label, q)) {
					seen[s.Qname] = struct{}{}
					out = append(out, s)
					if len(out) >= req.MaxResults {
						return out
					}
				}
			}
			continue
		}
		if _, dup := seen[rng.Qname]; !dup {
			seen[rng.Qname] = struct{}{}
			if q == "" || matchesQuery(rng.LocalName, rng.Label, q) {
				out = append(out, nodeToSuggestion(rng))
				if len(out) >= req.MaxResults {
					return out
				}
			}
		}
		// Subclasses of the range class (more-specific picks).
		for _, sub := range closure(rng, func(n *Node) []*Node { return n.Subclasses }, ClosureOpts{}) {
			if sub.Ghost {
				continue
			}
			if _, dup := seen[sub.Qname]; dup {
				continue
			}
			if q != "" && !matchesQuery(sub.LocalName, sub.Label, q) {
				continue
			}
			seen[sub.Qname] = struct{}{}
			out = append(out, nodeToSuggestion(sub))
			if len(out) >= req.MaxResults {
				return out
			}
		}
	}
	// DatatypeProperty with no explicit range relation (or all ranges pointed
	// at classes we don't have): still let the path terminate by emitting a
	// generic rdfs:Literal.
	if isDatatype && len(out) == 0 {
		out = append(out, literalSuggestionFromQname(genericLiteralQname))
	}
	return out
}

// nodeToSuggestion converts a Node to a Suggestion for the API response.
func nodeToSuggestion(n *Node) Suggestion {
	return Suggestion{
		URI: n.URI, Qname: n.Qname, Prefix: n.Prefix, LocalName: n.LocalName,
		Label: n.Label, Comment: n.Comment, Type: n.Type,
		OntologyPrefix: n.Meta.Prefix, OntologyVersion: n.Meta.VersionString,
	}
}
