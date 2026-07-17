package generators

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/views"
)

// BuildModelSnapshot creates a renderer-neutral export snapshot for a model.
func BuildModelSnapshot(in ModelSnapshotInput) *Snapshot {
	opts := normalizeOptions(in.Options)
	report := Report{}
	rootSlug := Slug(in.Model.SemanticID, in.Model.SystemName, in.Model.ID)
	root := views.TreeNode{
		Kind:       views.NodeModel,
		ID:         in.Model.ID,
		SemanticID: in.Model.SemanticID,
		SystemName: in.Model.SystemName,
		Label:      in.Model.UIName,
		Slug:       rootSlug,
		Path:       rootSlug,
		Scope:      copyPathElement(in.Model.OntologyScope),
	}

	fields := make([]FieldNode, 0)
	categorySlugs := newSlugAllocator()
	rootFieldSlugs := newSlugAllocator()
	for _, category := range in.View.Categories {
		catSlug := categorySlugs.Next(category.ID, category.Name.Get(opts.Lang))
		catPath := path.Join(root.Path, "category", catSlug)
		catNode := views.TreeNode{
			Kind:  views.NodeCategory,
			ID:    category.ID,
			Label: category.Name,
			Order: category.Position,
			Slug:  catSlug,
			Path:  catPath,
		}

		collectionSlugs := newSlugAllocator()
		for _, collection := range category.Collections {
			if collection.ID == directCollectionID {
				// Direct (non-collection) fields are one pseudo-collection
				// placement per category, so two such fields in the same
				// category share their intermediate path nodes.
				directScope := catSlug + "/" + directCollectionID
				for _, field := range collection.Fields {
					node, fieldNode := buildFieldTreeNode(field, root.Path, rootFieldSlugs, in.Model.OntologyScope, opts, &report, directScope)
					root.Children = append(root.Children, node)
					fields = append(fields, fieldNode)
				}
				continue
			}

			var collectionEntity *domain.Collection
			if c := in.Collections[collection.ID]; c != nil {
				collectionEntity = c
			}
			collectionSlug := collectionSlugs.Next(
				collectionSemanticID(collectionEntity),
				collectionSystemName(collectionEntity),
				collection.ID,
				collection.Name.Get(opts.Lang),
			)
			collectionPath := path.Join(catPath, "collection", collectionSlug)
			// One placement = one (category, collection) pair; node ids are
			// scoped to it so the same collection placed in two categories
			// does not merge.
			placementScope := catSlug + "/" + collectionSlug
			collNode := views.TreeNode{
				Kind:       views.NodeCollection,
				ID:         collection.ID,
				Label:      collection.Name,
				Order:      collection.Position,
				Slug:       collectionSlug,
				Path:       collectionPath,
				PathPrefix: copyPathElements(collection.SharedPathPrefix),
			}
			assignPathNodeChains(collNode.PathPrefix, placementScope)
			// Anchor class — the class the collection's fields converge
			// on (join-path tail), distinct from the declared Scope.
			if n := len(collNode.PathPrefix); n > 0 {
				collNode.Anchor = &collNode.PathPrefix[n-1]
			}
			scope := in.Model.OntologyScope
			if collectionEntity != nil {
				collNode.SemanticID = collectionEntity.SemanticID
				collNode.SystemName = collectionEntity.SystemName
				collNode.Scope = copyPathElement(collectionEntity.OntologyScope)
				scope = collectionEntity.OntologyScope
			}

			collMembership := fieldNodeCollection(collection, collectionEntity)
			fieldSlugs := newSlugAllocator()
			for _, field := range collection.Fields {
				node, fieldNode := buildFieldTreeNode(field, collectionPath, fieldSlugs, scope, opts, &report, placementScope)
				fieldNode.CollectionID = collMembership.id
				fieldNode.CollectionSemanticID = collMembership.semanticID
				fieldNode.CollectionSystemName = collMembership.systemName
				fieldNode.CollectionName = collMembership.name
				collNode.Children = append(collNode.Children, node)
				fields = append(fields, fieldNode)
			}
			catNode.Children = append(catNode.Children, collNode)
		}
		if len(catNode.Children) > 0 {
			root.Children = append(root.Children, catNode)
		}
	}

	// Snap-level collection composition (opt-in via Options.ComposeCollections).
	// Grafts every collection that anchors at class X under every
	// Collection-stub field whose path ends at X — recursively, with
	// loop and depth guards. Tree enrichment is a follow-up; renderers
	// that walk Fields directly (Arches) pick up the new graft nodes
	// today, renderers that walk Tree (RDF/SHACL/X3ML) still see the
	// un-composed tree until that follow-up lands.
	fields = composeCollections(in.View, in.Collections, in.ExtraAnchored, fields, opts, &report)

	snap := &Snapshot{
		RootKind:   EntityModel,
		Project:    in.Project,
		Model:      &in.Model,
		Fields:     fields,
		Tree:       &views.Tree{Root: root},
		Namespaces: in.Namespaces,
		Ontologies: in.Ontologies,
		Options:    opts,
		Report:     report,
	}
	assignShortPathNodeIDs(snap)
	return snap
}

func collectionSemanticID(collection *domain.Collection) string {
	if collection == nil {
		return ""
	}
	return collection.SemanticID
}

func collectionSystemName(collection *domain.Collection) string {
	if collection == nil {
		return ""
	}
	return collection.SystemName
}

// collectionMembership captures the per-view metadata a generator needs
// in order to group fields by their owning Pletka Collection without
// re-traversing the model tree.
type collectionMembership struct {
	id         string
	semanticID string
	systemName string
	name       domain.Translations
}

func fieldNodeCollection(view domain.CollectionGroup, entity *domain.Collection) collectionMembership {
	out := collectionMembership{
		id:   view.ID,
		name: view.Name,
	}
	if entity != nil {
		out.semanticID = entity.SemanticID
		out.systemName = entity.SystemName
		if entity.UIName != nil && len(entity.UIName) > 0 {
			out.name = entity.UIName
		}
	}
	return out
}

// BuildCollectionSnapshot creates a renderer-neutral export snapshot for a collection.
func BuildCollectionSnapshot(in CollectionSnapshotInput) *Snapshot {
	opts := normalizeOptions(in.Options)
	report := Report{}
	rootSlug := Slug(in.Collection.SemanticID, in.Collection.SystemName, in.Collection.ID)
	root := views.TreeNode{
		Kind:       views.NodeCollection,
		ID:         in.Collection.ID,
		SemanticID: in.Collection.SemanticID,
		SystemName: in.Collection.SystemName,
		Label:      in.Collection.UIName,
		Slug:       rootSlug,
		Path:       rootSlug,
		Scope:      copyPathElement(in.Collection.OntologyScope),
	}

	fields := append([]domain.ResolvedField(nil), in.Fields...)
	sort.SliceStable(fields, func(i, j int) bool {
		return fields[i].Position < fields[j].Position
	})

	fieldNodes := make([]FieldNode, 0, len(fields))
	fieldSlugs := newSlugAllocator()
	for _, field := range fields {
		node, fieldNode := buildFieldTreeNode(field, root.Path, fieldSlugs, in.Collection.OntologyScope, opts, &report, "")
		root.Children = append(root.Children, node)
		fieldNodes = append(fieldNodes, fieldNode)
	}

	snap := &Snapshot{
		RootKind:   EntityCollection,
		Project:    in.Project,
		Collection: &in.Collection,
		Fields:     fieldNodes,
		Tree:       &views.Tree{Root: root},
		Namespaces: in.Namespaces,
		Ontologies: in.Ontologies,
		Options:    opts,
		Report:     report,
	}
	assignShortPathNodeIDs(snap)
	return snap
}

// BuildFieldSnapshot creates a renderer-neutral export snapshot for a single resolved field.
func BuildFieldSnapshot(in FieldSnapshotInput) *Snapshot {
	opts := normalizeOptions(in.Options)
	report := Report{}
	rootSlug := Slug(in.Field.SemanticID, in.Field.SystemName, in.Field.ID)
	node, fieldNode := buildFieldTreeNode(in.Resolved, "", newSlugAllocator(), in.Field.OntologyScope, opts, &report, "")
	node.Path = rootSlug
	fieldNode.RelativePath = rootSlug
	for i := range node.Children {
		if node.Children[i].Kind == views.NodePath {
			node.Children[i].Path = strings.Replace(node.Children[i].Path, path.Join("field", rootSlug), rootSlug, 1)
		}
	}
	for i := range fieldNode.Path {
		fieldNode.Path[i].RelativePath = strings.Replace(fieldNode.Path[i].RelativePath, path.Join("field", rootSlug), rootSlug, 1)
	}

	snap := &Snapshot{
		RootKind:   EntityField,
		Project:    in.Project,
		Field:      &in.Field,
		Fields:     []FieldNode{fieldNode},
		Tree:       &views.Tree{Root: node},
		Namespaces: in.Namespaces,
		Ontologies: in.Ontologies,
		Options:    opts,
		Report:     report,
	}
	assignShortPathNodeIDs(snap)
	return snap
}

type ModelSnapshotInput struct {
	Project     domain.Project
	Model       domain.Model
	View        domain.ModelView
	Collections map[string]*domain.Collection
	// ExtraAnchored is the set of collections discovered via the
	// cross-project anchor index that the model view does not own
	// but whose anchor class matches at least one Collection-stub
	// field's terminal class. Each entry carries its own resolved
	// field set (already loaded by the snap service), keyed by
	// collection ID. composeCollections indexes these alongside the
	// view's own collections so they can be grafted onto matching
	// stubs. Nil / empty disables cross-anchor composition.
	ExtraAnchored map[string]ExtraAnchoredCollection
	Namespaces    NamespaceSet
	Ontologies    []domain.ResolvedOntologyVersion
	Options       Options
}

// ExtraAnchoredCollection bundles one cross-project anchored
// collection's identity, fields, and computed shared path prefix —
// enough for composeCollections to include it in the byAnchor index
// without re-walking the resolver.
type ExtraAnchoredCollection struct {
	Collection       *domain.Collection
	Fields           []domain.ResolvedField
	SharedPathPrefix []domain.PathElement
}

type CollectionSnapshotInput struct {
	Project    domain.Project
	Collection domain.Collection
	Fields     []domain.ResolvedField
	Namespaces NamespaceSet
	Ontologies []domain.ResolvedOntologyVersion
	Options    Options
}

type FieldSnapshotInput struct {
	Project    domain.Project
	Field      domain.Field
	Resolved   domain.ResolvedField
	Namespaces NamespaceSet
	Ontologies []domain.ResolvedOntologyVersion
	Options    Options
}

const directCollectionID = "__direct__"

func buildFieldTreeNode(field domain.ResolvedField, parentPath string, alloc *slugAllocator, scope domain.PathElement, opts Options, report *Report, placementScope string) (views.TreeNode, FieldNode) {
	fieldSlug := alloc.Next(field.SemanticID, field.SystemName, field.ID)
	fieldPath := path.Join(parentPath, "field", fieldSlug)
	pathNodes := buildPathNodes(field.ID, field.PathElements, fieldPath, opts, report, placementScope)
	// Legacy <br><br> subfield paths: each gets its own path-node chain under
	// a distinct sub-path so slugs/relative-paths never collide with the
	// primary. Renderers iterate Path + SubPaths under the same field binding.
	var subPaths [][]PathNode
	for i, sf := range field.SubfieldPaths {
		if len(sf.PathElements) == 0 {
			continue
		}
		subPath := path.Join(fieldPath, "sub", strconv.Itoa(i))
		subPaths = append(subPaths, buildPathNodes(field.ID, sf.PathElements, subPath, opts, report, placementScope))
	}
	node := views.TreeNode{
		Kind:       views.NodeField,
		ID:         field.ID,
		SemanticID: field.SemanticID,
		SystemName: field.SystemName,
		Label:      field.DisplayName,
		Order:      field.Position,
		Slug:       fieldSlug,
		Path:       fieldPath,
	}
	for _, pathNode := range pathNodes {
		element := pathNode.Element
		node.Children = append(node.Children, views.TreeNode{
			Kind:    views.NodePath,
			ID:      element.PrefixedName(),
			Order:   pathNode.Index,
			Slug:    pathNode.Slug,
			Path:    pathNode.RelativePath,
			Element: &element,
		})
	}
	return node, FieldNode{
		Field:        field,
		Slug:         fieldSlug,
		RelativePath: fieldPath,
		Scope:        scope,
		Path:         pathNodes,
		SubPaths:     subPaths,
	}
}

func buildPathNodes(fieldID string, elements []domain.PathElement, fieldPath string, opts Options, report *Report, placementScope string) []PathNode {
	out := make([]PathNode, 0, len(elements))
	alloc := newSlugAllocator()
	chain := placementChain(placementScope)
	for i, element := range elements {
		if strings.HasPrefix(element.LocalName, "^") {
			diagnostic := Diagnostic{
				Code:      "legacy_inverse_path",
				Message:   "legacy inverse path marker found in PathElement.LocalName",
				FieldID:   fieldID,
				PathIndex: i,
			}
			switch opts.LegacyInverseMode {
			case LegacyInverseSkip:
				report.Warnings = append(report.Warnings, diagnostic)
				continue
			case LegacyInverseNormalize:
				report.Warnings = append(report.Warnings, diagnostic)
				element.LocalName = strings.TrimPrefix(element.LocalName, "^")
				element.URI = strings.Replace(element.URI, ":^", ":", 1)
			default:
				report.Errors = append(report.Errors, diagnostic)
			}
		}

		slug := alloc.Next(element.ClassCode, element.PrefixedName(), element.URI, element.LocalName)

		// path_node: the structural ancestor chain — slugified qnames of
		// every step from the path root down to this node. Class nodes
		// reached through the same prefix get the same chain, so it is
		// the node-coreference key. The short path_node_id
		// is derived from it in a snapshot-wide pass (assignShortPathNodeIDs).
		chain = append(chain, slugifyQName(element.PrefixedName()))
		if element.Type == "class" {
			element.PathNode = strings.Join(chain, "/")
		}

		// TODO(path-direction): Weave paths are currently persisted as canonical
		// forward property chains. If inverse traversal becomes a persisted
		// PathElement feature, add explicit direction metadata and update RDF/SPARQL
		// generation to swap subject/object for inverse steps. Do not encode inverse
		// direction in LocalName.
		out = append(out, PathNode{
			Element:      element,
			Index:        i,
			Direction:    PathDirectionForward,
			Role:         pathRole(element, i, len(elements)),
			Slug:         slug,
			RelativePath: path.Join(fieldPath, "path", slug),
		})
	}
	return out
}

// slugifyQName normalises an ontology qname into a path_node_id token:
// lowercased, with colons, dots, slashes, hashes and spaces collapsed to
// underscores.
func slugifyQName(qname string) string {
	r := strings.NewReplacer(":", "_", ".", "_", "/", "_", "#", "_", " ", "_")
	return strings.ToLower(r.Replace(strings.TrimSpace(qname)))
}

// placementChain seeds a path_node chain with the placement scope —
// the (category, collection) the field sits in — so nodes in different
// placements never collide. Empty scope (collection/field snapshots)
// yields a bare ontology chain.
func placementChain(placementScope string) []string {
	if placementScope == "" {
		return nil
	}
	return []string{placementScope}
}

// assignPathNodeChains stamps the structural ancestor chain (PathNode)
// onto each class element of a raw path-element slice — used for the
// collection join prefix, which does not pass through buildPathNodes.
// Mutates in place.
func assignPathNodeChains(elements []domain.PathElement, placementScope string) {
	chain := placementChain(placementScope)
	for i := range elements {
		chain = append(chain, slugifyQName(elements[i].PrefixedName()))
		if elements[i].Type == "class" {
			elements[i].PathNode = strings.Join(chain, "/")
		}
	}
}

// shortClassCode is the human-readable stem of a class element's short
// id — the CRM class code lowercased (E33_E41 → e33_e41), falling back
// to the slugified local name for classes without a code.
func shortClassCode(e domain.PathElement) string {
	if c := strings.TrimSpace(e.ClassCode); c != "" {
		return strings.ToLower(c)
	}
	return slugifyQName(e.LocalName)
}

// eachClassElement visits every class PathElement reachable from the
// snapshot — the flat field paths, the tree's path nodes and each
// collection's join prefix — yielding a pointer so callers can mutate.
func eachClassElement(snap *Snapshot, fn func(*domain.PathElement)) {
	for fi := range snap.Fields {
		for pi := range snap.Fields[fi].Path {
			if e := &snap.Fields[fi].Path[pi].Element; e.Type == "class" {
				fn(e)
			}
		}
	}
	if snap.Tree == nil {
		return
	}
	var walk func(n *views.TreeNode)
	walk = func(n *views.TreeNode) {
		if n.Element != nil && n.Element.Type == "class" {
			fn(n.Element)
		}
		for i := range n.PathPrefix {
			if n.PathPrefix[i].Type == "class" {
				fn(&n.PathPrefix[i])
			}
		}
		for i := range n.Children {
			walk(&n.Children[i])
		}
	}
	walk(&snap.Tree.Root)
}

// assignShortPathNodeIDs derives the short path_node_id from each class
// element's PathNode chain. Distinct chains are grouped by class code
// and ordered by the chain string, so every node reached through the
// same prefix gets the same {class_code}_{occurrence} id, deterministic
// and reorder-stable within the snapshot.
func assignShortPathNodeIDs(snap *Snapshot) {
	byCode := map[string][]string{}
	codeOf := map[string]string{}
	seen := map[string]bool{}
	eachClassElement(snap, func(e *domain.PathElement) {
		if e.PathNode == "" || seen[e.PathNode] {
			return
		}
		seen[e.PathNode] = true
		code := shortClassCode(*e)
		byCode[code] = append(byCode[code], e.PathNode)
		codeOf[e.PathNode] = code
	})
	idByChain := make(map[string]string, len(seen))
	for code, chains := range byCode {
		sort.Strings(chains)
		for i, chain := range chains {
			idByChain[chain] = fmt.Sprintf("%s_%d", code, i+1)
		}
	}
	eachClassElement(snap, func(e *domain.PathElement) {
		if e.PathNode != "" {
			e.PathNodeID = idByChain[e.PathNode]
		}
	})
}

func pathRole(element domain.PathElement, index int, total int) PathRole {
	if index == 0 && element.Type == "class" {
		return PathRoleScope
	}
	if index == total-1 {
		return PathRoleTerminal
	}
	switch element.Type {
	case "property":
		return PathRoleProperty
	case "class":
		return PathRoleClass
	case "literal":
		return PathRoleLiteral
	default:
		return PathRole(element.Type)
	}
}

func copyPathElement(in domain.PathElement) *domain.PathElement {
	out := in
	out.AdditionalTypes = append([]domain.TypeRef(nil), in.AdditionalTypes...)
	return &out
}

func copyPathElements(in []domain.PathElement) []domain.PathElement {
	out := make([]domain.PathElement, len(in))
	for i, element := range in {
		out[i] = element
		out[i].AdditionalTypes = append([]domain.TypeRef(nil), element.AdditionalTypes...)
	}
	return out
}
