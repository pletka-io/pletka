package mermaid

import (
	"context"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/generators/exportgraph"
)

type Renderer struct{}

func NewRenderer() *Renderer {
	return &Renderer{}
}

func (r *Renderer) Spec() generators.FormatSpec {
	return generators.FormatSpec{
		Format:        generators.FormatMermaid,
		ContentType:   "text/vnd.mermaid; charset=utf-8",
		FileExtension: ".mmd",
		RequiresTree:  true,
	}
}

func (r *Renderer) Render(ctx context.Context, snap *generators.Snapshot, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if snap == nil {
		return fmt.Errorf("mermaid renderer: snapshot is nil")
	}
	if len(snap.Report.Errors) > 0 {
		return fmt.Errorf("mermaid renderer: snapshot has %d error(s)", len(snap.Report.Errors))
	}

	cfg, err := DefaultClassificationConfig()
	if err != nil {
		return err
	}
	classifier := NewClassifier(cfg)
	var graph *graph
	switch snap.Options.MermaidMode {
	case "", generators.MermaidModeOntology:
		graph = buildOntologyGraph(snap, classifier, ontologyGroups(cfg.Groups))
	case generators.MermaidModeInstance:
		graph = buildInstanceGraph(snap, classifier, cfg.Groups)
	default:
		return fmt.Errorf("mermaid renderer: unsupported mode %q", snap.Options.MermaidMode)
	}
	_, err = io.WriteString(w, graph.Render())
	return err
}

type graph struct {
	groups   []ClassificationGroup
	nodes    []node
	nodeByID map[string]string
	edges    []edge
	seenEdge map[edge]struct{}
}

type node struct {
	id    string
	label string
	style string
	shape nodeShape
}

type edge struct {
	from      string
	to        string
	predicate string
}

type nodeShape string

const (
	nodeShapeClass    nodeShape = "class"
	nodeShapeResource nodeShape = "resource"
	nodeShapeLiteral  nodeShape = "literal"
)

func newGraph(groups []ClassificationGroup) *graph {
	return &graph{
		groups:   append([]ClassificationGroup(nil), groups...),
		nodeByID: make(map[string]string),
		seenEdge: make(map[edge]struct{}),
	}
}

func buildOntologyGraph(snap *generators.Snapshot, classifier Classifier, groups []ClassificationGroup) *graph {
	g := newGraph(groups)
	ontology := exportgraph.Build(snap)
	fieldOrder := make(map[string]int, len(snap.Fields))
	for _, field := range snap.Fields {
		fieldOrder[field.Field.ID] = field.Field.Position
	}
	nodeByKey := make(map[string]*exportgraph.ResourceNode, len(ontology.Nodes))
	for _, resource := range ontology.Nodes {
		nodeByKey[resource.Key] = resource
	}
	for _, resource := range ontology.Nodes {
		currentID := g.addClassNode(resource.Key, resource.Class, classifier)
		for _, relation := range orderedOntologyRelations(resource, fieldOrder) {
			switch {
			case relation.targetKey != "":
				target := nodeByKey[relation.targetKey]
				if target == nil {
					continue
				}
				nextID := g.addClassNode(target.Key, target.Class, classifier)
				g.addEdge(currentID, nextID, relation.predicate)
			case relation.literal != nil:
				element := ontologyLiteralElement(*relation.literal)
				nextID := g.addLiteralNode(ontologyLiteralNodeKey(resource.Key, relation.predicate, element), element)
				g.addEdge(currentID, nextID, relation.predicate)
			}
		}
	}
	return g
}

func buildInstanceGraph(snap *generators.Snapshot, classifier Classifier, groups []ClassificationGroup) *graph {
	g := newGraph(groups)
	for _, field := range fieldsInTreeOrder(snap) {
		groupPath := fieldGroupPath(snap, field)
		currentPath := groupPath
		currentID := g.addResourceNode(currentPath, resourceLabel(snap, currentPath), uriStyle(classifier.Group(field.Scope)))
		if typeName := prefixedName(field.Scope); typeName != "" {
			typeID := g.addClassNode("type/"+typeName, field.Scope, classifier)
			g.addEdge(currentID, typeID, "rdf:type")
		}

		classPath := make([]string, 0, len(field.Path))
		for i := 0; i < len(field.Path); i++ {
			step := field.Path[i]
			if step.Element.Type != "property" || i+1 >= len(field.Path) {
				continue
			}
			predicate := prefixedName(step.Element)
			if predicate == "" {
				continue
			}
			next := field.Path[i+1]
			switch next.Element.Type {
			case "class":
				classPath = append(classPath, next.Slug)
				nextPath := classResourcePath(groupPath, classPath)
				nextID := g.addResourceNode(nextPath, resourceLabel(snap, nextPath), uriStyle(classifier.Group(next.Element)))
				g.addEdge(currentID, nextID, predicate)
				if typeName := prefixedName(next.Element); typeName != "" {
					typeID := g.addClassNode("type/"+typeName, next.Element, classifier)
					g.addEdge(nextID, typeID, "rdf:type")
				}
				currentPath = nextPath
				currentID = nextID
				i++
			case "literal":
				nextID := g.addLiteralValueNode(currentPath+"/"+predicate, literalValue(field.Field))
				g.addEdge(currentID, nextID, predicate)
				i++
			}
		}
	}
	return g
}

func (g *graph) addClassNode(key string, element domain.PathElement, classifier Classifier) string {
	label := prefixedName(element)
	if label == "" {
		label = "unknown"
	}
	style := classifier.Group(element)
	return g.addNode(key, label, style, nodeShapeClass)
}

func (g *graph) addResourceNode(key string, label string, style string) string {
	if label == "" {
		label = key
	}
	return g.addNode("resource/"+key, label, style, nodeShapeResource)
}

func (g *graph) addLiteralNode(key string, element domain.PathElement) string {
	label := element.Datatype
	if label == "" {
		label = prefixedName(element)
	}
	if label == "" || label == "rdf:langString" {
		label = "rdfs:Literal"
	}
	if label == "rdf:literal" {
		label = "rdfs:Literal"
	}
	return g.addNode(key, label, "Literal", nodeShapeLiteral)
}

func (g *graph) addLiteralValueNode(key string, value string) string {
	if value == "" {
		value = `""`
	}
	return g.addNode("literal/"+key, mermaidLiteralLabel(value), "Literal", nodeShapeResource)
}

func (g *graph) addNode(key string, label string, style string, shape nodeShape) string {
	if id, ok := g.nodeByID[key]; ok {
		return id
	}
	id := fmt.Sprintf("%d", len(g.nodes))
	g.nodeByID[key] = id
	g.nodes = append(g.nodes, node{id: id, label: label, style: style, shape: shape})
	return id
}

func (g *graph) addEdge(from string, to string, predicate string) {
	if from == "" || to == "" || predicate == "" {
		return
	}
	e := edge{from: from, to: to, predicate: predicate}
	if _, ok := g.seenEdge[e]; ok {
		return
	}
	g.seenEdge[e] = struct{}{}
	g.edges = append(g.edges, e)
}

func (g *graph) Render() string {
	var b strings.Builder
	b.WriteString("graph TD\n")
	b.WriteString(formatClassDefs(g.groups))
	b.WriteByte('\n')
	for _, e := range g.edges {
		fmt.Fprintf(&b, "%s -->|%s| %s\n", g.nodeRef(e.from), e.predicate, g.nodeRef(e.to))
	}
	return b.String()
}

func (g *graph) nodeRef(id string) string {
	n := g.nodes[mustAtoi(id)]
	switch n.shape {
	case nodeShapeResource:
		return fmt.Sprintf("%s([\"%s\"]):::%s", id, escapeMermaidLabel(n.label), n.style)
	case nodeShapeLiteral:
		return fmt.Sprintf("%s[%s]:::%s", id, escapeMermaidLabel(n.label), n.style)
	default:
		return fmt.Sprintf("%s[\"%s\"]:::%s", id, escapeMermaidLabel(n.label), n.style)
	}
}

func snapRootScope(snap *generators.Snapshot) domain.PathElement {
	switch {
	case snap == nil:
		return domain.PathElement{}
	case snap.Collection != nil:
		return snap.Collection.OntologyScope
	case snap.Model != nil:
		return snap.Model.OntologyScope
	case snap.Field != nil:
		return snap.Field.OntologyScope
	default:
		return domain.PathElement{}
	}
}

func fieldsInTreeOrder(snap *generators.Snapshot) []generators.FieldNode {
	if snap == nil {
		return nil
	}
	// Snapshot builders already emit Fields in tree order. Keeping this helper
	// central makes it explicit that Mermaid must not re-sort by class/predicate.
	return append([]generators.FieldNode(nil), snap.Fields...)
}

func fieldGroupPath(snap *generators.Snapshot, field generators.FieldNode) string {
	if snap.RootKind == generators.EntityField {
		return field.RelativePath
	}
	if before, _, ok := strings.Cut(field.RelativePath, "/field/"); ok && before != "" {
		return before
	}
	return field.RelativePath
}

func classResourcePath(groupPath string, classPath []string) string {
	parts := append([]string{groupPath}, classPath...)
	return path.Join(parts...)
}

func resourceLabel(snap *generators.Snapshot, relativePath string) string {
	compact := compactResourcePath(relativePath)
	base := strings.TrimSpace(snap.Options.BaseURI)
	if base == "" {
		base = snap.Namespaces.ProjectURI
	}
	if base == "" {
		return compact
	}
	if !strings.HasSuffix(base, "/") && !strings.HasSuffix(base, "#") {
		base += "/"
	}
	return base + compact
}

func compactResourcePath(relativePath string) string {
	parts := strings.Split(strings.Trim(relativePath, "/"), "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "", "category", "collection", "field", "path":
			continue
		default:
			out = append(out, part)
		}
	}
	return strings.Join(out, "/")
}

func prefixedName(element domain.PathElement) string {
	return element.PrefixedName()
}

func isAnnotationPredicate(predicate string) bool {
	return predicate == "rdfs:label"
}

func ontologyGroups(groups []ClassificationGroup) []ClassificationGroup {
	out := make([]ClassificationGroup, 0, len(groups))
	for _, group := range groups {
		if strings.HasSuffix(group.ID, "_URI") {
			continue
		}
		out = append(out, group)
	}
	return out
}

func uriStyle(classStyle string) string {
	if classStyle == "" {
		return "Default_URI"
	}
	if strings.HasSuffix(classStyle, "_URI") {
		return classStyle
	}
	return classStyle + "_URI"
}

func literalValue(field domain.ResolvedField) string {
	value := field.SetValue
	if value == "" {
		value = slugLiteral(field.SystemName, field.SemanticID, field.ID) + "_value"
	}
	return `"` + escapeLiteral(value) + `"`
}

func slugLiteral(parts ...string) string {
	for _, part := range parts {
		if s := strings.Trim(strings.ReplaceAll(generators.Slug(part), "-", "_"), "_"); s != "" {
			return s
		}
	}
	return "value"
}

func escapeLiteral(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, "\r", `\r`)
	return value
}

func mermaidLiteralLabel(value string) string {
	value = strings.TrimPrefix(strings.TrimSuffix(value, `"`), `"`)
	return "''" + value + "''"
}

type ontologyRelation struct {
	order     int
	index     int
	predicate string
	targetKey string
	literal   *exportgraph.LiteralBinding
}

func orderedOntologyRelations(resource *exportgraph.ResourceNode, fieldOrder map[string]int) []ontologyRelation {
	relations := make([]ontologyRelation, 0, len(resource.Edges)+len(resource.Literals))
	for i := range resource.Edges {
		outgoing := resource.Edges[i]
		predicate := prefixedName(outgoing.Predicate)
		if predicate == "" || isAnnotationPredicate(predicate) {
			continue
		}
		relations = append(relations, ontologyRelation{
			order:     minFieldOrder(outgoing.FieldIDs, fieldOrder),
			index:     len(relations),
			predicate: predicate,
			targetKey: outgoing.TargetKey,
		})
	}
	for i := range resource.Literals {
		literal := resource.Literals[i]
		predicate := prefixedName(literal.Predicate)
		if predicate == "" || isAnnotationPredicate(predicate) {
			continue
		}
		relations = append(relations, ontologyRelation{
			order:     literalFieldOrder(literal, fieldOrder),
			index:     len(relations),
			predicate: predicate,
			literal:   &literal,
		})
	}
	sort.SliceStable(relations, func(i, j int) bool {
		if relations[i].order != relations[j].order {
			return relations[i].order < relations[j].order
		}
		return relations[i].index < relations[j].index
	})
	return relations
}

func minFieldOrder(fieldIDs []string, fieldOrder map[string]int) int {
	best := int(^uint(0) >> 1)
	found := false
	for _, fieldID := range fieldIDs {
		order, ok := fieldOrder[fieldID]
		if !ok {
			continue
		}
		if !found || order < best {
			best = order
			found = true
		}
	}
	if !found {
		return best
	}
	return best
}

func literalFieldOrder(binding exportgraph.LiteralBinding, fieldOrder map[string]int) int {
	if binding.Field.Source != nil {
		return binding.Field.Source.Field.Position
	}
	if order, ok := fieldOrder[binding.Field.ID]; ok {
		return order
	}
	return int(^uint(0) >> 1)
}

func ontologyLiteralElement(binding exportgraph.LiteralBinding) domain.PathElement {
	if binding.Field.Source != nil {
		for i := len(binding.Field.Source.Path) - 1; i >= 0; i-- {
			node := binding.Field.Source.Path[i]
			if node.Element.Type == "literal" || node.Role == generators.PathRoleLiteral {
				return node.Element
			}
		}
	}
	return domain.PathElement{
		Type:      "literal",
		URI:       "rdf:literal",
		Prefix:    "rdf",
		LocalName: "literal",
		Datatype:  "rdf:literal",
	}
}

func ontologyLiteralNodeKey(parentKey string, predicate string, element domain.PathElement) string {
	key := parentKey + "/" + predicate
	switch {
	case element.Datatype != "":
		return key + "/" + element.Datatype
	case element.URI != "":
		return key + "/" + element.URI
	default:
		return key + "/" + element.PrefixedName()
	}
}

func escapeMermaidLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func formatClassDefs(groups []ClassificationGroup) string {
	var b strings.Builder
	for _, group := range groups {
		if group.ID == "" {
			continue
		}
		fill := group.Fill
		if fill == "" {
			fill = "#FFFFFF"
		}
		stroke := group.Stroke
		if stroke == "" {
			stroke = "#000000"
		}
		fmt.Fprintf(&b, "classDef %s fill:%s,stroke:%s;\n", group.ID, fill, stroke)
	}
	return b.String()
}

func mustAtoi(s string) int {
	var n int
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}
