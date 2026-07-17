package exportgraph

import (
	"fmt"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/views"
)

// Build creates a graph-template view of a generator snapshot.
func Build(snap *generators.Snapshot) *Graph {
	if snap == nil {
		return &Graph{}
	}
	b := &builder{
		snap:        snap,
		nodesByKey:  make(map[string]*ResourceNode),
		groupByKey:  make(map[string]*VisualGroup),
		fieldGroups: make(map[string][]string),
		graph:       &Graph{Source: snap},
	}
	b.buildRoot()
	b.collectGroups()
	b.buildFields()
	b.resolveGroupBranches()
	return b.graph
}

type builder struct {
	snap *generators.Snapshot

	graph       *Graph
	nodesByKey  map[string]*ResourceNode
	groupByKey  map[string]*VisualGroup
	fieldGroups map[string][]string
}

func (b *builder) buildRoot() {
	scope := b.rootScope()
	key := "root:" + string(b.snap.RootKind) + ":" + stableRootID(b.snap)
	root := &ResourceNode{
		Key:             key,
		Class:           scope,
		IdentitySource:  IdentityStructural,
		AdditionalTypes: append([]domain.TypeRef(nil), scope.AdditionalTypes...),
	}
	b.graph.RootKey = key
	b.addNode(root)
}

func (b *builder) buildFields() {
	for i := range b.snap.Fields {
		b.addField(&b.snap.Fields[i])
	}
}

func (b *builder) addField(field *generators.FieldNode) {
	currentKey := b.graph.RootKey
	var pendingProperty *domain.PathElement

	for i := range field.Path {
		node := &field.Path[i]
		element := node.Element
		switch element.Type {
		case "class":
			if i == 0 && sameElement(element, b.nodesByKey[b.graph.RootKey].Class) {
				continue
			}
			if pendingProperty == nil {
				continue
			}
			nextKey := b.resourceKey(currentKey, *pendingProperty, element)
			next := b.nodesByKey[nextKey]
			if next == nil {
				next = &ResourceNode{
					Key:             nextKey,
					Class:           element,
					IdentitySource:  identitySource(element),
					AdditionalTypes: append([]domain.TypeRef(nil), element.AdditionalTypes...),
				}
				b.addNode(next)
			} else {
				next.Class = mergeClassElement(next.Class, element)
				next.AdditionalTypes = mergeTypeRefs(next.AdditionalTypes, element.AdditionalTypes)
			}
			addUniqueString(&next.InstanceIDs, element.InstanceID)
			addUniqueSourcePath(&next.SourcePaths, field.Field.ID, node.RelativePath)
			addUniqueFieldSource(&next.Source.Fields, field)
			addUniquePathSource(&next.Source.Paths, node)
			b.addResourceEdge(currentKey, *pendingProperty, nextKey, field.Field.ID)
			currentKey = nextKey
			pendingProperty = nil
		case "property":
			pendingProperty = &element
		case "literal":
			b.addLiteral(currentKey, pendingProperty, element, field)
			pendingProperty = nil
		default:
			if node.Role == generators.PathRoleLiteral {
				b.addLiteral(currentKey, pendingProperty, element, field)
				pendingProperty = nil
			}
		}
	}

	b.bindFieldToNode(currentKey, field)
}

func (b *builder) addLiteral(currentKey string, pendingProperty *domain.PathElement, literal domain.PathElement, field *generators.FieldNode) {
	current := b.nodesByKey[currentKey]
	if current == nil || pendingProperty == nil {
		return
	}
	predicate := *pendingProperty
	if predicate.URI == "" {
		predicate = literal
	}
	binding := LiteralBinding{
		Predicate: predicate,
		Field:     b.fieldBinding(field, currentKey),
	}
	current.Literals = append(current.Literals, binding)
	addUniqueFieldSource(&current.Source.Fields, field)
}

func (b *builder) bindFieldToNode(nodeKey string, field *generators.FieldNode) {
	binding := b.fieldBinding(field, nodeKey)
	b.graph.Fields = append(b.graph.Fields, binding)
	if node := b.nodesByKey[nodeKey]; node != nil {
		node.Fields = append(node.Fields, binding)
		addUniqueFieldSource(&node.Source.Fields, field)
	}
	for _, groupKey := range binding.GroupKeys {
		group := b.groupByKey[groupKey]
		if group == nil {
			continue
		}
		addUniqueString(&group.FieldIDs, field.Field.ID)
		if node := b.nodesByKey[nodeKey]; node != nil {
			addGroupBinding(&node.Groups, GroupBinding{Key: group.Key, Kind: group.Kind})
			addUniqueGroupSource(&node.Source.Groups, group.Source)
		}
	}
}

func (b *builder) fieldBinding(field *generators.FieldNode, nodeKey string) FieldBinding {
	return FieldBinding{
		Source:       field,
		ID:           field.Field.ID,
		SemanticID:   field.Field.SemanticID,
		SystemName:   field.Field.SystemName,
		Label:        field.Field.DisplayName,
		RelativePath: field.RelativePath,
		NodeKey:      nodeKey,
		GroupKeys:    append([]string(nil), b.fieldGroups[field.RelativePath]...),
	}
}

func (b *builder) addNode(node *ResourceNode) {
	b.nodesByKey[node.Key] = node
	b.graph.Nodes = append(b.graph.Nodes, node)
}

func (b *builder) addResourceEdge(sourceKey string, predicate domain.PathElement, targetKey string, fieldID string) {
	source := b.nodesByKey[sourceKey]
	if source == nil {
		return
	}
	for i := range source.Edges {
		edge := &source.Edges[i]
		if edge.TargetKey == targetKey && sameElement(edge.Predicate, predicate) {
			addUniqueString(&edge.FieldIDs, fieldID)
			return
		}
	}
	source.Edges = append(source.Edges, ResourceEdge{
		Predicate: predicate,
		TargetKey: targetKey,
		FieldIDs:  nonEmptyStrings(fieldID),
	})
}

func (b *builder) collectGroups() {
	if b.snap == nil || b.snap.Tree == nil {
		return
	}
	b.walkGroupTree(&b.snap.Tree.Root, "", nil)
}

func (b *builder) walkGroupTree(node *views.TreeNode, parentKey string, ancestors []string) {
	groupKey := ""
	nextAncestors := ancestors
	switch node.Kind {
	case views.NodeCategory, views.NodeCollection:
		groupKey = "group:" + node.Path
		group := &VisualGroup{
			Source:     node,
			Key:        groupKey,
			Kind:       node.Kind,
			ID:         node.ID,
			SemanticID: node.SemanticID,
			SystemName: node.SystemName,
			Label:      node.Label,
			Order:      node.Order,
			Path:       node.Path,
			ParentKey:  parentKey,
			PathPrefix: copyPathElements(node.PathPrefix),
		}
		b.groupByKey[groupKey] = group
		b.graph.Groups = append(b.graph.Groups, group)
		if parentKey != "" {
			if parent := b.groupByKey[parentKey]; parent != nil {
				addUniqueString(&parent.Children, groupKey)
			}
		}
		nextAncestors = append(append([]string(nil), ancestors...), groupKey)
	case views.NodeField:
		for _, groupKey := range ancestors {
			groupKeys := b.fieldGroups[node.Path]
			addUniqueString(&groupKeys, groupKey)
			b.fieldGroups[node.Path] = groupKeys
		}
	}

	nextParent := parentKey
	if groupKey != "" {
		nextParent = groupKey
	}
	for i := range node.Children {
		b.walkGroupTree(&node.Children[i], nextParent, nextAncestors)
	}
}

func (b *builder) resolveGroupBranches() {
	for _, group := range b.graph.Groups {
		if len(group.PathPrefix) == 0 {
			if group.Kind == views.NodeCategory {
				group.BranchKey = b.graph.RootKey
			}
			continue
		}
		group.BranchKey = b.keyForPath(group.PathPrefix)
	}
}

func (b *builder) keyForPath(elements []domain.PathElement) string {
	currentKey := b.graph.RootKey
	var pendingProperty *domain.PathElement
	for i, element := range elements {
		switch element.Type {
		case "class":
			if i == 0 && sameElement(element, b.nodesByKey[b.graph.RootKey].Class) {
				continue
			}
			if pendingProperty == nil {
				continue
			}
			currentKey = b.resourceKey(currentKey, *pendingProperty, element)
			pendingProperty = nil
		case "property":
			pendingProperty = &element
		}
	}
	return currentKey
}

// resourceKey is the export-graph node identity. It is the class
// element's generated path_node_id — the structural node id the
// snapshot stamps onto every path element. The
// parentKey/predicate/class structural fallback (no #instance= legacy
// splice) covers elements that carry no path_node_id.
func (b *builder) resourceKey(parentKey string, predicate domain.PathElement, class domain.PathElement) string {
	if class.PathNodeID != "" {
		return class.PathNodeID
	}
	return parentKey + "/" + elementIdentity(predicate) + "/" + elementIdentity(class)
}

func (b *builder) rootScope() domain.PathElement {
	if b.snap == nil {
		return domain.PathElement{}
	}
	if b.snap.Model != nil {
		return b.snap.Model.OntologyScope
	}
	if b.snap.Collection != nil {
		return b.snap.Collection.OntologyScope
	}
	if b.snap.Field != nil {
		return b.snap.Field.OntologyScope
	}
	if b.snap.Tree != nil && b.snap.Tree.Root.Scope != nil {
		return *b.snap.Tree.Root.Scope
	}
	if len(b.snap.Fields) > 0 {
		return b.snap.Fields[0].Scope
	}
	return domain.PathElement{}
}

func stableRootID(snap *generators.Snapshot) string {
	if snap == nil {
		return "unknown"
	}
	switch {
	case snap.Model != nil:
		return firstNonEmpty(snap.Model.SemanticID, snap.Model.SystemName, snap.Model.ID)
	case snap.Collection != nil:
		return firstNonEmpty(snap.Collection.SemanticID, snap.Collection.SystemName, snap.Collection.ID)
	case snap.Field != nil:
		return firstNonEmpty(snap.Field.SemanticID, snap.Field.SystemName, snap.Field.ID)
	case snap.Tree != nil:
		return firstNonEmpty(snap.Tree.Root.SemanticID, snap.Tree.Root.SystemName, snap.Tree.Root.ID, snap.Tree.Root.Slug)
	default:
		return "unknown"
	}
}

func identitySource(element domain.PathElement) IdentitySource {
	// A generated path_node_id is a structural identity. The legacy
	// hand-coded instance_id only reports when no path_node_id exists.
	if element.PathNodeID != "" {
		return IdentityStructural
	}
	if element.InstanceID != "" {
		return IdentityInstanceID
	}
	return IdentityStructural
}

func elementIdentity(element domain.PathElement) string {
	if element.URI != "" {
		return element.URI
	}
	if name := element.PrefixedName(); name != ":" {
		return name
	}
	return fmt.Sprintf("%s:%s:%s", element.Type, element.Prefix, element.LocalName)
}

func sameElement(a, b domain.PathElement) bool {
	return elementIdentity(a) == elementIdentity(b)
}

func mergeClassElement(existing, incoming domain.PathElement) domain.PathElement {
	if existing.URI == "" {
		existing = incoming
	}
	existing.AdditionalTypes = mergeTypeRefs(existing.AdditionalTypes, incoming.AdditionalTypes)
	if existing.InstanceID == "" {
		existing.InstanceID = incoming.InstanceID
	}
	return existing
}

func mergeTypeRefs(existing []domain.TypeRef, incoming []domain.TypeRef) []domain.TypeRef {
	out := append([]domain.TypeRef(nil), existing...)
	for _, item := range incoming {
		key := item.URI
		if key == "" {
			key = item.PrefixedName()
		}
		found := false
		for _, existingItem := range out {
			existingKey := existingItem.URI
			if existingKey == "" {
				existingKey = existingItem.PrefixedName()
			}
			if existingKey == key {
				found = true
				break
			}
		}
		if !found {
			out = append(out, item)
		}
	}
	return out
}

func addUniqueString(out *[]string, value string) {
	if value == "" {
		return
	}
	for _, existing := range *out {
		if existing == value {
			return
		}
	}
	*out = append(*out, value)
}

func addUniqueSourcePath(out *[]SourcePathBinding, fieldID string, path string) {
	if fieldID == "" && path == "" {
		return
	}
	for _, existing := range *out {
		if existing.FieldID == fieldID && existing.Path == path {
			return
		}
	}
	*out = append(*out, SourcePathBinding{FieldID: fieldID, Path: path})
}

func addGroupBinding(out *[]GroupBinding, binding GroupBinding) {
	if binding.Key == "" {
		return
	}
	for _, existing := range *out {
		if existing.Key == binding.Key {
			return
		}
	}
	*out = append(*out, binding)
}

func addUniqueFieldSource(out *[]*generators.FieldNode, value *generators.FieldNode) {
	if value == nil {
		return
	}
	for _, existing := range *out {
		if existing == value {
			return
		}
	}
	*out = append(*out, value)
}

func addUniquePathSource(out *[]*generators.PathNode, value *generators.PathNode) {
	if value == nil {
		return
	}
	for _, existing := range *out {
		if existing == value {
			return
		}
	}
	*out = append(*out, value)
}

func addUniqueGroupSource(out *[]*views.TreeNode, value *views.TreeNode) {
	if value == nil {
		return
	}
	for _, existing := range *out {
		if existing == value {
			return
		}
	}
	*out = append(*out, value)
}

func nonEmptyStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func copyPathElements(in []domain.PathElement) []domain.PathElement {
	out := make([]domain.PathElement, len(in))
	for i, element := range in {
		out[i] = element
		out[i].AdditionalTypes = append([]domain.TypeRef(nil), element.AdditionalTypes...)
	}
	return out
}
