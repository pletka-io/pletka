package generators

import (
	"maps"
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
)

// composeCollections is the snap-level graft pass: every Collection-stub
// field grafts the collections its collection_model refs name, recursively
// (2026-11-11 nested-collections spec, decisions 1, 4, 5).
//
// Each grafted copy carries the stub's PathElements + the member's path
// minus the collection's SharedPathPrefix, the nested collection's
// identity, the stub's max as its card max, and GraftStubID. A stub
// without a ref grafts nothing (verify-git reports it); a ref missing from
// referenced is a compose.unresolved_ref warning.
//
// Loop-safe: a collection already on the graft chain, the stub's own
// collection included, is not grafted again. Depth-bounded via
// opts.ComposeMaxDepth (default 10).
//
// Returns the fields with grafted copies appended in BFS order, originals
// first, or the input unchanged when opts.ComposeCollections is off.
func composeCollections(fields []FieldNode, referenced map[string]ReferencedCollection, grafts map[string]domain.GraftOverride, opts Options, report *Report) []FieldNode {
	stampTopLevelAddresses(fields)
	if !opts.ComposeCollections || len(fields) == 0 {
		return fields
	}
	used := map[string]bool{}
	out := make([]FieldNode, 0, len(fields)*2)
	out = append(out, fields...)

	type queueItem struct {
		field      FieldNode
		graftStack map[string]struct{} // collection ids on the active graft chain
		depth      int
	}
	var queue []queueItem
	for _, f := range fields {
		if !isCollectionStub(f.Field) {
			continue
		}
		stack := map[string]struct{}{}
		if f.CollectionID != "" {
			stack[f.CollectionID] = struct{}{}
		}
		queue = append(queue, queueItem{field: f, graftStack: stack})
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
		stubAlias := stubAliasForGraft(q.field)
		for _, ref := range q.field.Field.CollectionModels {
			target, ok := referenced[ref.ID]
			if !ok || target.Missing != "" {
				why := target.Missing
				if !ok {
					why = "not loaded"
				}
				report.Warnings = append(report.Warnings, Diagnostic{
					Code:    "compose.unresolved_ref",
					Message: "collection_model " + ref.SemanticID + " " + why + "; nothing grafted under " + stubAlias,
					FieldID: q.field.Field.ID,
				})
				continue
			}
			if _, on := q.graftStack[ref.ID]; on {
				continue
			}
			stack := copyStringSet(q.graftStack)
			stack[ref.ID] = struct{}{}
			systemName := ""
			if target.Collection != nil {
				systemName = target.Collection.SystemName
			}
			for _, child := range target.Fields {
				addr := q.field.Address + "/" + child.SemanticID
				g, has := grafts[addr]
				if has {
					used[addr] = true
					if g.IsHidden {
						continue
					}
				}
				grafted := graftField(q.field, child, target)
				grafted.Address = addr
				if has {
					applyGraftOverride(&grafted, g)
				}
				// GraftPath accumulates the alias chain so the Arches
				// renderer can mint per-path UUIDs and aliases.
				grafted.GraftPath = append(append([]string(nil), q.field.GraftPath...), stubAlias)
				// Slug is the leaf's unique part: its system name with the
				// source collection's prefix stripped ("name_content" →
				// "content"); the renderer joins GraftPath + Slug.
				grafted.Slug = leafUniqueSlug(child.SystemName, systemName)
				out = append(out, grafted)
				if isCollectionStub(grafted.Field) {
					queue = append(queue, queueItem{field: grafted, graftStack: stack, depth: q.depth + 1})
				}
			}
		}
	}
	warnUnresolvedGrafts(grafts, used, report)
	return out
}

// stampTopLevelAddresses gives each top-level field its own semantic id as
// address; it holds whether or not the model is composed.
func stampTopLevelAddresses(fields []FieldNode) {
	for i := range fields {
		if fields[i].Address == "" {
			fields[i].Address = fields[i].Field.SemanticID
		}
	}
}

// applyGraftOverride sets a grafted copy's max and name from its override.
// MaxOccursSet with a nil MaxOccurs is unbounded. The stub's max becomes its
// children's CollectionMaxOccurs when it is grafted in turn.
func applyGraftOverride(grafted *FieldNode, g domain.GraftOverride) {
	if g.MaxOccursSet {
		grafted.Field.MaxOccurs = g.MaxOccurs
	}
	if len(g.DisplayName) > 0 {
		grafted.Field.DisplayName = g.DisplayName
	}
}

// warnUnresolvedGrafts reports each override address compose never reached,
// except those under a hidden graft.
func warnUnresolvedGrafts(grafts map[string]domain.GraftOverride, used map[string]bool, report *Report) {
	for _, addr := range slices.Sorted(maps.Keys(grafts)) {
		if used[addr] || hiddenAncestor(addr, grafts) {
			continue
		}
		report.Warnings = append(report.Warnings, Diagnostic{
			Code:    "compose.unresolved_graft",
			Message: "grafts.yaml address " + addr + " is not reached in the composed model",
		})
	}
}

// hiddenAncestor reports whether a proper prefix of addr is a hidden graft:
// addresses under a hidden branch are never reached and need no warning.
func hiddenAncestor(addr string, grafts map[string]domain.GraftOverride) bool {
	for i := strings.LastIndex(addr, "/"); i > 0; i = strings.LastIndex(addr[:i], "/") {
		if g, ok := grafts[addr[:i]]; ok && g.IsHidden {
			return true
		}
	}
	return false
}

// graftPrefix is what a graft strips from a referenced collection's member
// paths: the longest prefix every member shares, cut back so it ends at a
// class and leaves every member at least one step (a one-field collection
// keeps its last property and value).
func graftPrefix(fields []domain.ResolvedField) []domain.PathElement {
	p := computeSharedPathPrefix(fields)
	for _, f := range fields {
		if n := len(f.PathElements); n > 0 && len(p) >= n {
			p = p[:n-1]
		}
	}
	for len(p) > 0 && p[len(p)-1].Type != string(PathRoleClass) {
		p = p[:len(p)-1]
	}
	return p
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

// graftField builds a copy of child placed under stub: its PathElements
// become stub.PathElements + child.PathElements stripped of target's
// SharedPathPrefix; it carries target's identity and the stub's max.
//
// Threads the snap's PathNode chain through the grafted PathElements so
// assignShortPathNodeIDs (run on the snapshot after composeCollections)
// assigns a structurally unique {class_code}_{occurrence} PathNodeID to
// every grafted class node. The Arches renderer keys intermediates and
// leaves off those PathNodeIDs, so two grafts that walk identical
// chains share the same intermediate node — exactly the structural
// merging real Arches achieves via curator-typed instance_ids.
func graftField(stub FieldNode, child domain.ResolvedField, target ReferencedCollection) FieldNode {
	tail := stripSharedPrefix(child.PathElements, target.SharedPathPrefix)

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

	node := FieldNode{
		Field:               grafted,
		Slug:                child.SystemName,
		RelativePath:        stub.RelativePath,
		Scope:               stub.Scope,
		Path:                pathNodes,
		CollectionMaxOccurs: stub.Field.MaxOccurs,
		GraftStubID:         stub.Field.SemanticID,
	}
	if c := target.Collection; c != nil {
		node.CollectionID = c.ID
		node.CollectionSemanticID = c.SemanticID
		node.CollectionSystemName = c.SystemName
		node.CollectionName = c.UIName
	}
	return node
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
// matches the collection's SharedPathPrefix step for step (SameStep). If the child's path
// does not share the expected prefix (unusual — would mean the
// collection's prefix was mis-computed), the child path is returned
// unchanged.
func stripSharedPrefix(child, sharedPrefix []domain.PathElement) []domain.PathElement {
	n := len(sharedPrefix)
	if n == 0 || len(child) < n {
		return child
	}
	for i := 0; i < n; i++ {
		if !child[i].SameStep(sharedPrefix[i]) {
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
