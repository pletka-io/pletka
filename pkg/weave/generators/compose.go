package generators

import (
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
)

// composeCollections is the snap-level graft pass: for every
// Collection-stub field whose path ends at a CIDOC class X, copy every
// collection's children that anchor at X under the stub, recursively.
//
// Rule: each grafted copy carries the stub's PathElements + the
// collection's leaf path minus its SharedPathPrefix, so the resulting
// PathElements walk the same CIDOC backbone the curator would
// reconstruct by hand in the Arches graph designer.
//
// Loop-safe via a per-(target collection ID, anchor URI) stack guard.
// Depth-bounded via opts.ComposeMaxDepth (default 10).
//
// Returns the augmented fields slice — originals first, grafted copies
// appended in BFS order. The tree enrichment is a separate concern;
// the Arches renderer consumes Fields directly, so this is enough to
// land the OGEE Activity coverage gap (43 → ~244 leaves) without
// touching the tree side. Tree enrichment lands in a follow-up.
//
// Returns the input unchanged when opts.ComposeCollections is off, or
// when the model view has no Collection-stub fields.
func composeCollections(view domain.ModelView, collections map[string]*domain.Collection, extraAnchored map[string]ExtraAnchoredCollection, fields []FieldNode, opts Options, report *Report) []FieldNode {
	if !opts.ComposeCollections || len(fields) == 0 {
		return fields
	}
	byAnchor := indexCollectionsByAnchor(view, collections)
	// Append cross-project anchored collections (the snap service has
	// pre-loaded their fields). These are the collections that close
	// the OGEE Activity Duration gap: LA's Dimension, anchored at
	// E54_Dimension, grafted under timespan_duration stubs even
	// though no Activity field belongs to Dimension.
	for _, ea := range extraAnchored {
		if ea.Collection == nil || len(ea.SharedPathPrefix) == 0 {
			continue
		}
		anchor := ea.SharedPathPrefix[len(ea.SharedPathPrefix)-1].URI
		if anchor == "" {
			continue
		}
		byAnchor[anchor] = append(byAnchor[anchor], collectionAtAnchor{
			collectionID: ea.Collection.ID,
			systemName:   ea.Collection.SystemName,
			sharedPrefix: ea.SharedPathPrefix,
			fields:       ea.Fields,
		})
	}
	if len(byAnchor) == 0 {
		return fields
	}

	out := make([]FieldNode, 0, len(fields)*2)
	out = append(out, fields...)

	type queueItem struct {
		field FieldNode
		// graftStack records (collectionID + "|" + anchorURI) pairs
		// already on the active graft chain, so the same collection
		// cannot be re-grafted on the same anchor inside its own
		// composed subtree (cycle break).
		graftStack map[string]struct{}
		depth      int
	}

	queue := make([]queueItem, 0, len(fields))
	for _, f := range fields {
		if isCollectionStub(f.Field) {
			queue = append(queue, queueItem{
				field:      f,
				graftStack: map[string]struct{}{},
				depth:      0,
			})
		}
	}

	for len(queue) > 0 {
		q := queue[0]
		queue = queue[1:]

		if q.depth >= opts.ComposeMaxDepth {
			report.Warnings = append(report.Warnings, Diagnostic{
				Code:    "compose.depth_cap",
				Message: "graft recursion hit ComposeMaxDepth; further composition skipped",
				FieldID: q.field.Field.ID,
			})
			continue
		}

		anchor := terminalClassURI(q.field.Field.PathElements)
		if anchor == "" {
			continue
		}

		targets, ok := byAnchor[anchor]
		if !ok {
			continue
		}

		stubAlias := stubAliasForGraft(q.field)

		for _, target := range targets {
			stackKey := target.collectionID + "|" + anchor
			if _, on := q.graftStack[stackKey]; on {
				continue
			}
			newStack := copyStringSet(q.graftStack)
			newStack[stackKey] = struct{}{}

			for _, child := range target.fields {
				grafted := graftField(q.field, child, target.sharedPrefix)
				// GraftPath accumulates the alias chain so the Arches
				// renderer can mint per-path UUIDs and aliases.
				grafted.GraftPath = append(append([]string(nil), q.field.GraftPath...), stubAlias)
				// Slug becomes the leaf's unique part — its system name
				// with the source collection's own prefix stripped
				// ("name_content" → "content" inside the Name
				// collection). The renderer joins GraftPath + Slug to
				// build aliases that match real Arches' path-derived
				// pattern (e.g. activity_part_name_content rather than
				// activity_part_name_name_content).
				grafted.Slug = leafUniqueSlug(child.SystemName, target.systemName)
				out = append(out, grafted)
				if isCollectionStub(grafted.Field) {
					queue = append(queue, queueItem{
						field:      grafted,
						graftStack: newStack,
						depth:      q.depth + 1,
					})
				}
			}
		}
	}

	return out
}

// collectionAtAnchor is the byAnchor map's value shape.
type collectionAtAnchor struct {
	collectionID string
	systemName   string // for leaf-alias prefix stripping during graft
	sharedPrefix []domain.PathElement
	fields       []domain.ResolvedField
}

// indexCollectionsByAnchor produces anchor URI → list of matching
// collections, drawn from every category in the model view. Collections
// without a shared path prefix (terminal class unknown) are skipped —
// they have no structural anchor to graft against.
func indexCollectionsByAnchor(view domain.ModelView, collections map[string]*domain.Collection) map[string][]collectionAtAnchor {
	out := map[string][]collectionAtAnchor{}
	for _, cat := range view.Categories {
		for _, c := range cat.Collections {
			if c.ID == directCollectionID {
				continue
			}
			if len(c.SharedPathPrefix) == 0 {
				continue
			}
			anchor := c.SharedPathPrefix[len(c.SharedPathPrefix)-1].URI
			if anchor == "" {
				continue
			}
			systemName := ""
			if entity := collections[c.ID]; entity != nil {
				systemName = entity.SystemName
			}
			out[anchor] = append(out[anchor], collectionAtAnchor{
				collectionID: c.ID,
				systemName:   systemName,
				sharedPrefix: c.SharedPathPrefix,
				fields:       c.Fields,
			})
		}
	}
	return out
}

// isCollectionStub returns true when a field's ExpectedValueType marks
// it as a semantic stub that, in real Arches, carries grafted children.
//
// The canonical type strings are "Collection" / "Collection Model"; compare
// case-insensitively because curator-supplied overrides occasionally arrive
// lower-cased.
func isCollectionStub(f domain.ResolvedField) bool {
	switch strings.ToLower(strings.TrimSpace(f.ExpectedValueType)) {
	case "collection", "collection model":
		return true
	}
	return false
}

// terminalClassURI returns the URI of the last "class" element in a
// path. Empty when the path is empty or the terminal step is not a
// class (e.g. a literal-terminated field, which is never a Collection
// stub anyway).
func terminalClassURI(elements []domain.PathElement) string {
	for i := len(elements) - 1; i >= 0; i-- {
		if elements[i].Type == "class" {
			return elements[i].URI
		}
	}
	return ""
}

// graftField builds a copy of child placed under stub: its PathElements
// become stub.PathElements + child.PathElements stripped of the
// collection's SharedPathPrefix.
//
// Threads the snap's PathNode chain through the grafted PathElements so
// assignShortPathNodeIDs (run on the snapshot after composeCollections)
// assigns a structurally unique {class_code}_{occurrence} PathNodeID to
// every grafted class node. The Arches renderer keys intermediates and
// leaves off those PathNodeIDs, so two grafts that walk identical
// chains share the same intermediate node — exactly the structural
// merging real Arches achieves via curator-typed instance_ids.
func graftField(stub FieldNode, child domain.ResolvedField, sharedPrefix []domain.PathElement) FieldNode {
	tail := stripSharedPrefix(child.PathElements, sharedPrefix)

	// buildPathNodes only sets PathNode on the FieldNode.Path[] slice
	// (its local element copies) — Field.PathElements stays bare. So
	// we draw the stub's chain from stub.Path[].Element instead and
	// re-use those PathNode-carrying copies for the composed path's
	// stub prefix. The result: the grafted FieldNode's composedPath
	// has PathNode populated on every class element from root to
	// graft anchor.
	stubElements := make([]domain.PathElement, 0, len(stub.Path))
	for _, pn := range stub.Path {
		stubElements = append(stubElements, pn.Element)
	}
	if len(stubElements) == 0 {
		// Fallback: stub came from a generator path that didn't run
		// buildPathNodes (rare; happens for snapshot.Field tests). Use
		// the raw PathElements; no PathNode chain available either way.
		stubElements = copyPathElements(stub.Field.PathElements)
	}
	seedChain := stubTerminalChain(stubElements)

	composedPath := make([]domain.PathElement, 0, len(stubElements)+len(tail))
	composedPath = append(composedPath, stubElements...)
	tailCopy := copyPathElements(tail)

	// Walk the tail elements, extending the chain one slug per step,
	// and stamp PathNode on each class element. Leaf literal /
	// property elements don't carry PathNode (matches the convention
	// in buildPathNodes).
	chain := append([]string(nil), seedChain...)
	for i := range tailCopy {
		chain = append(chain, slugifyQName(tailCopy[i].PrefixedName()))
		if tailCopy[i].Type == "class" {
			tailCopy[i].PathNode = strings.Join(chain, "/")
		}
	}
	composedPath = append(composedPath, tailCopy...)

	grafted := child
	grafted.PathElements = composedPath

	// Build the FieldNode.Path slice mirroring buildPathNodes —
	// assignShortPathNodeIDs walks snap.Fields[fi].Path[pi].Element,
	// so the composed PathElements must surface there to receive
	// PathNodeIDs.
	pathNodes := make([]PathNode, 0, len(composedPath))
	for i, el := range composedPath {
		pathNodes = append(pathNodes, PathNode{
			Element:   el,
			Index:     i,
			Direction: PathDirectionForward,
			Role:      pathRole(el, i, len(composedPath)),
			Slug:      slugifyQName(el.PrefixedName()),
		})
	}

	return FieldNode{
		Field:                grafted,
		Slug:                 child.SystemName,
		RelativePath:         stub.RelativePath,
		Scope:                stub.Scope,
		Path:                 pathNodes,
		CollectionID:         stub.CollectionID,
		CollectionSemanticID: stub.CollectionSemanticID,
		CollectionSystemName: stub.CollectionSystemName,
		CollectionName:       stub.CollectionName,
	}
}

// stubTerminalChain returns the slug chain of the stub's terminal class
// element — already populated by buildPathNodes when the un-grafted
// field was first added to the snap. Falls back to a fresh slug chain
// (URI-only) when the stub has no PathNode set yet (rare; happens for
// the root scope or when a stub is itself the product of a graft whose
// chain hasn't propagated). The fresh chain gives a deterministic
// fallback so assignShortPathNodeIDs always finds something to key on.
func stubTerminalChain(elements []domain.PathElement) []string {
	for i := len(elements) - 1; i >= 0; i-- {
		if elements[i].Type == "class" && elements[i].PathNode != "" {
			return strings.Split(elements[i].PathNode, "/")
		}
	}
	chain := make([]string, 0, len(elements))
	for _, e := range elements {
		chain = append(chain, slugifyQName(e.PrefixedName()))
	}
	return chain
}

// stripSharedPrefix removes the longest leading run of elements that
// matches the collection's SharedPathPrefix by URI. If the child's path
// does not share the expected prefix (unusual — would mean the
// collection's prefix was mis-computed), the child path is returned
// unchanged.
func stripSharedPrefix(child, sharedPrefix []domain.PathElement) []domain.PathElement {
	n := len(sharedPrefix)
	if n == 0 || len(child) < n {
		return child
	}
	for i := 0; i < n; i++ {
		if child[i].URI != sharedPrefix[i].URI {
			return child
		}
	}
	return child[n:]
}

// stubAliasForGraft returns the alias slug used to chain a stub into
// its grafted children's GraftPath. For top-level stubs (no prior
// graft), the field's SystemName is the curator-authored alias
// (e.g. "activity_part_name"). For stubs that are themselves the
// product of an earlier graft, the SystemName still carries the
// source collection's prefix (e.g. an inner "timespan_name" stub
// grafted under "data_assignment_timespan"), so use the already-
// stripped Slug instead — otherwise the chain doubles up
// ("data_assignment_timespan_timespan_name_…") and diverges from the
// real Arches alias shape.
func stubAliasForGraft(stub FieldNode) string {
	if len(stub.GraftPath) > 0 {
		if s := strings.TrimSpace(stub.Slug); s != "" {
			return s
		}
	}
	if name := strings.TrimSpace(stub.Field.SystemName); name != "" {
		return name
	}
	if stub.Slug != "" {
		return stub.Slug
	}
	return stub.Field.SemanticID
}

// leafUniqueSlug returns the leaf's distinguishing part by stripping
// the source collection's systemname prefix from the leaf's systemname.
// "name_content" inside collection "name" → "content". Falls back to
// the unmodified systemname when the prefix does not match (collection
// SystemName empty, or leaf SystemName doesn't start with it).
func leafUniqueSlug(leafSystemName, collSystemName string) string {
	if collSystemName == "" {
		return leafSystemName
	}
	pfx := collSystemName + "_"
	if strings.HasPrefix(leafSystemName, pfx) {
		return strings.TrimPrefix(leafSystemName, pfx)
	}
	if leafSystemName == collSystemName {
		// Edge case: the leaf IS named exactly the collection name.
		// Keep it so the alias doesn't collapse to the empty string.
		return leafSystemName
	}
	return leafSystemName
}

func copyStringSet(in map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(in)+1)
	for k := range in {
		out[k] = struct{}{}
	}
	return out
}
