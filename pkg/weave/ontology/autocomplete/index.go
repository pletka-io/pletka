package autocomplete

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
)

// OntologyMeta is the ontology prefix + version label carried on each Node
// for the suggestion payload. VersionID is for traceability only.
type OntologyMeta struct {
	Prefix        string
	VersionString string
	VersionID     string
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
}

// indexStore is the narrow read surface buildIndex needs.
type indexStore interface {
	ListClassesByVersions(ctx context.Context, versionIDs []string) ([]*domain.OntologyClass, error)
	ListPropertiesByVersions(ctx context.Context, versionIDs []string) ([]*domain.OntologyProperty, error)
	ListRelationsByVersionsAndTypes(ctx context.Context, versionIDs, relTypes []string) ([]*domain.OntologyRelationWithSource, error)
	GetVersion(ctx context.Context, id string) (*domain.OntologyVersion, error)
	GetOntology(ctx context.Context, id string) (*domain.Ontology, error)
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
		upsertNode(idx, primary, c.Qname, "class", c.URI, c.Prefix, c.LocalName, c.Label, c.Comment, metaFor(c.OntologyVersionID), c.OntologyVersionID)
	}

	// Pass 1b: properties.
	props, err := store.ListPropertiesByVersions(ctx, versionIDs)
	if err != nil {
		return nil, fmt.Errorf("list properties: %w", err)
	}
	for _, p := range props {
		upsertNode(idx, primary, p.Qname, "property", p.URI, p.Prefix, p.LocalName, p.Label, p.Comment, metaFor(p.OntologyVersionID), p.OntologyVersionID)
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
	for qname, node := range idx.ByQname {
		if node != nil && node.URI != "" {
			qnameByURI[node.URI] = qname
		}
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
	qnames := &qnameResolver{namespaceBindings: namespaceBindings}

	for _, r := range rels {
		src := idx.ByQname[r.SourceQname]
		if src == nil {
			// Source always materialised in pass 1; skip defensively.
			continue
		}
		targetQname := r.TargetQname
		if mapped := qnameByURI[targetQname]; mapped != "" {
			targetQname = mapped
		} else if r.RelType == "range" && src.Type == "property" && src.PropertyType == "DatatypeProperty" {
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
			return
		}
		// Promote ghost to real node.
		n.Ghost = false
		n.Type, n.URI, n.Prefix, n.LocalName, n.Label, n.Comment, n.Meta = typ, uri, prefix, local, label, comment, meta
		return
	}
	idx.ByQname[qname] = &Node{
		URI: uri, Qname: qname, Prefix: prefix, LocalName: local, Type: typ,
		Label: label, Comment: comment, Meta: meta,
	}
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
			}
		}
		cache[versionID] = m
		return m
	}
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
