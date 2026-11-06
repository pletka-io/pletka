package autocomplete

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
	nsutil "github.com/pletka-io/pletka/pkg/namespace"
)

// OntologyMeta is the ontology prefix + version label carried on each Node
// for the suggestion payload. VersionID is for traceability only.
type OntologyMeta struct {
	Prefix        string
	VersionString string
	VersionID     string
	// Namespace is the declared namespace of the owning ontology. It scopes the
	// "no domain => applies to all" rule to a property the ontology defines
	// (URI under this namespace) rather than one it merely imports.
	Namespace string
}

// Node is one qname in the project-union ontology DAG. Class-edge slices
// are empty on property nodes and vice versa (single closed taxonomy, no
// subtypes). Ghost nodes carry only Qname.
type Node struct {
	URI       string
	Qname     string
	Prefix    string
	LocalName string
	Type      string // "class" | "property"
	Label     domain.Translations
	Comment   domain.Translations

	// PropertyType is set for property nodes only ("DatatypeProperty",
	// "ObjectProperty", etc.) and drives the literal-suggestion path in
	// IndexedEngine.suggestRangeForProperty. Class nodes leave it "".
	PropertyType string

	// Class-side edges.
	Superclasses []*Node
	Subclasses   []*Node
	AsDomainOf   []*Node
	AsRangeOf    []*Node

	// Property-side edges.
	Domains    []*Node
	Ranges     []*Node
	SuperProps []*Node

	Meta  OntologyMeta
	Ghost bool

	// ownerNamespaces collects the namespace of every linked ontology version
	// that defines this qname. A node is collapsed across versions and Meta
	// keeps one of them; the "domain-less property under its own namespace"
	// rule must consider all, or an extension re-declaring a base term (to
	// add a range) would steal ownership and silence the property.
	ownerNamespaces []string
}

// literalValued reports whether a property of this type terminates a path in
// a literal rather than a class. DatatypeProperty does by definition;
// AnnotationProperty does in practice (labels, acronyms, notes) and is never
// declared with a class range an engine could continue from.
func literalValued(propertyType string) bool {
	return propertyType == "DatatypeProperty" || propertyType == "AnnotationProperty"
}

// IndexKey fully determines a project's autocomplete contract.
type IndexKey struct {
	VersionSet string // sorted version IDs joined by "|"
	Primary    string // primary ontology-version ID
	Lock       string // "live" | "release:<id>" (MVP: always "live")
}

// Index is the built pointer-graph for one IndexKey.
type Index struct {
	Key      IndexKey
	Versions []string
	Primary  string
	BuiltAt  time.Time
	ByQname  map[string]*Node

	// DomainlessProps are property nodes an ontology DEFINES (URI under its own
	// namespace) that declare no rdfs:domain. Per RDFS such a property applies
	// to any class, so suggestPropertiesForClass offers these on every class.
	// Imported external properties with no domain (dc:, foaf:, obo:…) are
	// excluded so scaffolding does not flood every class.
	DomainlessProps []*Node

	// ByURI indexes the same non-ghost nodes by their full URI so a scope or
	// path qname can resolve to its node even when its request prefix differs
	// from the stored one (readable obo: vs the import-assigned x<hash>:).
	ByURI map[string]*Node
	// namespaces expands a request qname to a URI using the project's runtime
	// bindings, feeding the ByURI fallback in lookupNode.
	namespaces nsutil.Manager
}

// lookupNode resolves a scope or path qname to its graph node. On a direct
// ByQname miss it expands the qname to a URI via the project's namespace
// bindings and retries by URI, so a readable prefix (e.g. obo:BFO_0000001)
// still resolves to a node the import stored under a different prefix for the
// same URI (e.g. x59a354be:BFO_0000001). Returns nil when neither resolves.
func (idx *Index) lookupNode(qname string) *Node {
	if n := idx.ByQname[qname]; n != nil {
		return n
	}
	if idx.namespaces == nil || idx.ByURI == nil {
		return nil
	}
	uri, err := idx.namespaces.Expand(qname)
	if err != nil || uri == "" || uri == qname {
		return nil
	}
	return idx.ByURI[uri]
}

// canonicalTargetQname maps a relation-target qname to the qname of the node
// that already owns the same URI, when the target uses a different prefix for
// that URI (e.g. a hash prefix for a class another ontology defines under a
// readable prefix). Returns "" when the manager is unavailable, the qname does
// not expand, or no other node owns the URI — leaving the caller on its
// existing handling.
func canonicalTargetQname(mgr nsutil.Manager, qnameByURI map[string]string, qname string) string {
	if mgr == nil || qname == "" || isFullURI(qname) {
		return ""
	}
	uri, err := mgr.Expand(qname)
	if err != nil || uri == "" {
		return ""
	}
	if canonical := qnameByURI[uri]; canonical != "" && canonical != qname {
		return canonical
	}
	return ""
}

// indexStore is the narrow read surface buildIndex needs.
type indexStore interface {
	ListClassesByVersions(ctx context.Context, versionIDs []string) ([]*domain.OntologyClass, error)
	ListPropertiesByVersions(ctx context.Context, versionIDs []string) ([]*domain.OntologyProperty, error)
	ListRelationsByVersionsAndTypes(ctx context.Context, versionIDs, relTypes []string) ([]*domain.OntologyRelationWithSource, error)
	GetVersion(ctx context.Context, id string) (*domain.OntologyVersion, error)
	GetOntology(ctx context.Context, id string) (*domain.Ontology, error)
	// GetOntologyByPrefix and GetActiveVersion back the implicit defaults
	// lookup in IndexCache.resolve. Both return (nil, nil) when nothing
	// matches.
	GetOntologyByPrefix(ctx context.Context, prefix string) (*domain.Ontology, error)
	GetActiveVersion(ctx context.Context, ontologyID string) (*domain.OntologyVersion, error)
}

// keyForRequest builds a canonical IndexKey from the supplied version IDs,
// primary version, and lock string. Version IDs are sorted so the same set in
// different order produces identical keys.
func keyForRequest(versionIDs []string, primary, lock string) IndexKey {
	s := append([]string(nil), versionIDs...)
	sort.Strings(s)
	if lock == "" {
		lock = "live"
	}
	return IndexKey{VersionSet: strings.Join(s, "|"), Primary: primary, Lock: lock}
}

// edgePair is the deduplication key for a directed edge between two Nodes.
type edgePair struct{ src, tgt *Node }

// buildIndex materialises the project-union DAG in two passes.
//
// Pass 1 inserts every class and property as a Node (collapsing duplicate
// qnames across versions so the primary version wins for Meta). Pass 2
// wires subclass_of / domain / range edges, creating ghost Nodes for
// relation targets that have no corresponding class or property row.
func buildIndex(ctx context.Context, store indexStore, namespaceBindings NamespaceBindingsFunc, key IndexKey, versionIDs []string, primary string, log *slog.Logger) (*Index, error) {
	start := time.Now()
	if len(versionIDs) > 50 {
		log.Warn("autocomplete index: project links many versions", "count", len(versionIDs))
	}

	idx := &Index{Key: key, Versions: versionIDs, Primary: primary, ByQname: map[string]*Node{}}

	// Per-version Meta lookup, memoised.
	metaFor := metaResolver(ctx, store, log)

	// Pass 1a: classes.
	classes, err := store.ListClassesByVersions(ctx, versionIDs)
	if err != nil {
		return nil, fmt.Errorf("list classes: %w", err)
	}
	for _, c := range classes {
		upsertNode(idx, primary, c.Qname, nodeTypeClass, c.URI, c.Prefix, c.LocalName, c.Label, c.Comment, metaFor(c.OntologyVersionID), c.OntologyVersionID)
	}

	// Pass 1b: properties.
	props, err := store.ListPropertiesByVersions(ctx, versionIDs)
	if err != nil {
		return nil, fmt.Errorf("list properties: %w", err)
	}
	for _, p := range props {
		upsertNode(idx, primary, p.Qname, nodeTypeProperty, p.URI, p.Prefix, p.LocalName, p.Label, p.Comment, metaFor(p.OntologyVersionID), p.OntologyVersionID)
		// Carry PropertyType onto the node so IndexedEngine can reproduce
		// DirectEngine's literal-suggestion logic without a DB round-trip.
		// Primary version wins (same rule as Meta); set unconditionally
		// because upsertNode only updates Meta on a primary-version collision.
		if n := idx.ByQname[p.Qname]; n != nil && !n.Ghost {
			if p.OntologyVersionID == primary || n.PropertyType == "" {
				n.PropertyType = p.PropertyType
			}
		}
	}
	qnameByURI := map[string]string{}
	idx.ByURI = map[string]*Node{}
	for qname, node := range idx.ByQname {
		if node != nil && node.URI != "" {
			qnameByURI[node.URI] = qname
			idx.ByURI[node.URI] = node
		}
	}
	// Namespace manager that expands a qname to a URI. Used both for the
	// lookupNode request-side fallback (idx.namespaces) and, below, to
	// canonicalise relation targets so a class referenced under a non-canonical
	// prefix (e.g. a hash prefix) merges into the node that defines the same
	// URI instead of stranding its edges on a ghost.
	nsMgr, err := newNamespaceManager(ctx, namespaceBindings)
	if err != nil {
		log.Warn("autocomplete index: namespace manager unavailable; qname URI resolution disabled", "err", err)
	} else {
		idx.namespaces = nsMgr
	}

	// Pass 2: wire edges (subclass_of, domain, range).
	rels, err := store.ListRelationsByVersionsAndTypes(ctx, versionIDs, []string{"subclass_of", "domain", "range"})
	if err != nil {
		return nil, fmt.Errorf("list relations: %w", err)
	}

	// Three independent dedupe maps — one per edge kind — so a class that is
	// both the domain and the range of a property keeps both edges.
	// ponytail: three plain maps over a kinded key — boring beats clever.
	seenSub := map[edgePair]struct{}{}
	seenDom := map[edgePair]struct{}{}
	seenRng := map[edgePair]struct{}{}
	qnames := &qnameResolver{namespaceBindings: namespaceBindings, manager: nsMgr}

	for _, r := range rels {
		src := idx.ByQname[r.SourceQname]
		if src == nil {
			// Source always materialised in pass 1; skip defensively.
			continue
		}
		targetQname := r.TargetQname
		if mapped := qnameByURI[targetQname]; mapped != "" {
			// Target was a full URI: use the canonical qname for it.
			targetQname = mapped
		} else if canonical := canonicalTargetQname(nsMgr, qnameByURI, targetQname); canonical != "" {
			// Target was a qname under a non-canonical prefix for a URI a real
			// node already owns (e.g. a hash-prefixed BFO class that another
			// ontology defines under obo:). Merge into that node so its
			// domain/subclass edges are not stranded on a ghost.
			targetQname = canonical
		} else if r.RelType == "range" && src.Type == nodeTypeProperty && literalValued(src.PropertyType) {
			var err error
			targetQname, err = normalizeRelationQname(ctx, qnames, targetQname)
			if err != nil {
				return nil, fmt.Errorf("normalize relation target %q: %w", r.TargetQname, err)
			}
		}
		tgt := idx.ByQname[targetQname]
		if tgt == nil {
			// Ghost: target qname not in any version's class/property tables.
			tgt = &Node{Qname: targetQname, Ghost: true}
			idx.ByQname[targetQname] = tgt
		}
		p := edgePair{src, tgt}
		switch r.RelType {
		case "subclass_of":
			if _, dup := seenSub[p]; !dup {
				seenSub[p] = struct{}{}
				src.Superclasses = append(src.Superclasses, tgt)
				tgt.Subclasses = append(tgt.Subclasses, src)
			}
		case "domain":
			if _, dup := seenDom[p]; !dup {
				seenDom[p] = struct{}{}
				src.Domains = append(src.Domains, tgt)
				tgt.AsDomainOf = append(tgt.AsDomainOf, src)
			}
		case "range":
			if _, dup := seenRng[p]; !dup {
				seenRng[p] = struct{}{}
				src.Ranges = append(src.Ranges, tgt)
				tgt.AsRangeOf = append(tgt.AsRangeOf, src)
			}
		}
	}

	wireImplicitRoot(idx)
	idx.DomainlessProps = collectDomainlessProps(idx)
	idx.BuiltAt = time.Now()
	detectCycles(idx, log)
	log.Info("autocomplete index built",
		"version_count", len(versionIDs), "build_ms", time.Since(start).Milliseconds(),
		"node_count", len(idx.ByQname))
	return idx, nil
}

// upsertNode creates or collapses a Node for qname. On collision with an
// existing non-ghost Node, the primary version's Meta wins; any other version
// is ignored so first-seen Meta is kept when no primary collision occurs. A
// previously-ghost Node is promoted to a real one.
func upsertNode(idx *Index, primary, qname, typ, uri, prefix, local string, label, comment domain.Translations, meta OntologyMeta, versionID string) {
	if n := idx.ByQname[qname]; n != nil {
		if !n.Ghost {
			if versionID == primary {
				n.Meta = meta // primary wins
			}
			n.ownerNamespaces = append(n.ownerNamespaces, meta.Namespace)
			// A re-declaration without a label (an extension importing a
			// term) must not hide the label the defining ontology gives it.
			if len(n.Label) == 0 && len(label) > 0 {
				n.Label = label
			}
			if len(n.Comment) == 0 && len(comment) > 0 {
				n.Comment = comment
			}
			return
		}
		// Promote ghost to real node.
		n.Ghost = false
		n.Type, n.URI, n.Prefix, n.LocalName, n.Label, n.Comment, n.Meta = typ, uri, prefix, local, label, comment, meta
		n.ownerNamespaces = append(n.ownerNamespaces, meta.Namespace)
		return
	}
	idx.ByQname[qname] = &Node{
		URI: uri, Qname: qname, Prefix: prefix, LocalName: local, Type: typ,
		Label: label, Comment: comment, Meta: meta, ownerNamespaces: []string{meta.Namespace},
	}
}

// definedByOwner reports whether the node's URI sits under the namespace of
// any linked ontology version that defines it.
func definedByOwner(n *Node) bool {
	for _, ns := range n.ownerNamespaces {
		if ns != "" && strings.HasPrefix(n.URI, ns) {
			return true
		}
	}
	return false
}

// metaResolver returns a memoised function that resolves an OntologyVersionID
// to an OntologyMeta by fetching the version + its parent ontology once.
// Meta is cosmetic (prefix + version string for the suggestion payload); a
// DB error degrades gracefully to blank values but is logged so it doesn't
// vanish silently.
func metaResolver(ctx context.Context, store indexStore, log *slog.Logger) func(versionID string) OntologyMeta {
	cache := map[string]OntologyMeta{}
	return func(versionID string) OntologyMeta {
		if m, ok := cache[versionID]; ok {
			return m
		}
		m := OntologyMeta{VersionID: versionID}
		v, err := store.GetVersion(ctx, versionID)
		if err != nil {
			log.Warn("metaResolver: get version failed; prefix/version will be blank",
				"version_id", versionID, "err", err)
		}
		if v != nil {
			m.VersionString = v.VersionString
			o, err := store.GetOntology(ctx, v.OntologyID)
			if err != nil {
				log.Warn("metaResolver: get ontology failed; prefix will be blank",
					"ontology_id", v.OntologyID, "err", err)
			}
			if o != nil {
				m.Prefix = o.Prefix
				m.Namespace = o.Namespace
			}
		}
		cache[versionID] = m
		return m
	}
}

// collectDomainlessProps returns the property nodes an ontology defines (URI
// under its own namespace) that have no domain edge. Sorted by qname so the
// suggestion order is stable. Imported external properties (URI outside the
// owning ontology's namespace) are skipped, as are ghosts and properties that
// have a domain in any linked version.
func collectDomainlessProps(idx *Index) []*Node {
	var out []*Node
	for _, n := range idx.ByQname {
		if n == nil || n.Ghost || n.Type != nodeTypeProperty || len(n.Domains) > 0 {
			continue
		}
		if !definedByOwner(n) {
			continue
		}
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Qname < out[j].Qname })
	return out
}

// wireImplicitRoot makes rdfs:Resource the superclass of every class that
// declares no real superclass. Every RDF class is an rdfs:Resource, so a
// property whose domain is rdfs:Resource (the defaults ontology's rdfs:label,
// rdfs:comment, ...) reaches every class through the ordinary superclass
// closure, with no engine-side special case. A class whose only parents are
// ghosts (its parent ontology is not linked) counts as having no real
// superclass and gets the root too. When no version in the index defines
// rdfs:Resource as a class, nothing is wired and the index is unchanged.
//
// ponytail: members of a subclass_of cycle all have a real superclass and
// so never reach the root; detectCycles already flags such imports.
func wireImplicitRoot(idx *Index) {
	root := idx.ByQname[resourceQname]
	if root == nil || root.Ghost || root.Type != nodeTypeClass {
		return
	}
	for _, n := range idx.ByQname {
		if n == nil || n == root || n.Ghost || n.Type != nodeTypeClass {
			continue
		}
		if hasRealSuperclass(n) {
			continue
		}
		n.Superclasses = append(n.Superclasses, root)
		root.Subclasses = append(root.Subclasses, n)
	}
}

// hasRealSuperclass reports whether n has at least one non-ghost superclass.
func hasRealSuperclass(n *Node) bool {
	for _, s := range n.Superclasses {
		if !s.Ghost {
			return true
		}
	}
	return false
}

// detectCycles logs (does not fail) when subclass_of forms a loop — signals a
// malformed import. The closure walk tolerates cycles regardless.
func detectCycles(idx *Index, log *slog.Logger) {
	for _, n := range idx.ByQname {
		seen := map[*Node]struct{}{n: {}}
		queue := append([]*Node(nil), n.Superclasses...)
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			if cur == n {
				log.Warn("autocomplete index: subclass cycle detected", "qname", n.Qname)
				break
			}
			if _, dup := seen[cur]; dup {
				continue
			}
			seen[cur] = struct{}{}
			queue = append(queue, cur.Superclasses...)
		}
	}
}

// BuildIndexForTest builds an index from a live Store using default "live"
// lock. Exported for smoke tests only.
func BuildIndexForTest(ctx context.Context, store indexStore, versionIDs []string, primary string) (*Index, error) {
	key := keyForRequest(versionIDs, primary, "live")
	return buildIndex(ctx, store, nil, key, versionIDs, primary, slog.Default())
}
