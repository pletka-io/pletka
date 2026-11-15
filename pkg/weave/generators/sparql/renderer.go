// Package sparql renders a weave generator snapshot as SPARQL SELECT
// queries (or the COUNT variant): a PREFIX block, then one query per field
// that walks the field's path through the export graph with one triple per
// property and an optional rdfs:label clause for resource values.
//
// Variable naming: the root is ?subject; every class node binds to its
// generated path_node_id, so two fields through the same node use the same
// variable; the terminal value binds to the field's semantic id plus system
// name. Every name passes through sparqlVar, every IRI is a prefixed name
// from a bound prefix or an IRIREF without forbidden characters, and comment
// lines are single lines: no snapshot text can change the query's shape.
//
// Class checks: model and collection scopes type ?subject (their identity);
// a field query does not, since a field is placed under many classes. Every
// class node between the root and the value is typed; the last node is not.
//
// Scopes: field roots emit one SELECT statement; model + collection roots
// emit one SELECT statement per field, separated by a blank line, so
// consumers can run them individually.
package sparql

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/generators/exportgraph"
	"github.com/pletka-io/pletka/pkg/weave/generators/rdfterm"
)

// Renderer writes SPARQL SELECT queries from a weave snapshot.
type Renderer struct{}

// NewRenderer constructs the SPARQL renderer.
func NewRenderer() *Renderer { return &Renderer{} }

// Spec implements generators.Renderer.
func (r *Renderer) Spec() generators.FormatSpec {
	return generators.FormatSpec{
		Format:        generators.FormatSPARQL,
		ContentType:   "application/sparql-query; charset=utf-8",
		FileExtension: ".rq",
		RequiresTree:  false,
	}
}

const (
	rdfNamespace  = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	rdfsNamespace = "http://www.w3.org/2000/01/rdf-schema#"
)

// Render implements generators.Renderer.
func (r *Renderer) Render(ctx context.Context, snap *generators.Snapshot, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if snap == nil {
		return fmt.Errorf("sparql renderer: snapshot is nil")
	}
	if len(snap.Report.Errors) > 0 {
		return fmt.Errorf("sparql renderer: snapshot has %d error(s)", len(snap.Report.Errors))
	}

	q := newQueryWriter(snap, exportgraph.Build(snap))
	var b strings.Builder
	if err := writePrefixes(&b, snap.Namespaces, collectUsedPrefixes(snap, q.root)); err != nil {
		return err
	}
	for i := range snap.Fields {
		if i > 0 {
			b.WriteByte('\n')
		}
		if err := q.writeField(&b, &snap.Fields[i]); err != nil {
			return err
		}
	}

	_, err := io.WriteString(w, b.String())
	return err
}

type queryWriter struct {
	snap  *generators.Snapshot
	graph *exportgraph.Graph
	root  *exportgraph.ResourceNode
	// nodeOf maps a field path's class step to the graph node it reaches.
	nodeOf map[*generators.PathNode]*exportgraph.ResourceNode
	// pos is each node's position in the graph, for nodes without an id.
	pos map[*exportgraph.ResourceNode]int
}

func newQueryWriter(snap *generators.Snapshot, g *exportgraph.Graph) *queryWriter {
	q := &queryWriter{
		snap:   snap,
		graph:  g,
		nodeOf: map[*generators.PathNode]*exportgraph.ResourceNode{},
		pos:    map[*exportgraph.ResourceNode]int{},
	}
	for i, n := range g.Nodes {
		if n.Key == g.RootKey {
			q.root = n
		}
		q.pos[n] = i
		for _, p := range n.Source.Paths {
			q.nodeOf[p] = n
		}
	}
	return q
}

// step is one property of a field path and the class node it reaches; a
// step ending on a literal (or on a bare property) has no node.
type step struct {
	predicate domain.PathElement
	nodeVar   string
	types     []domain.TypeRef
	literal   bool
}

// nodeVar names a graph node: ?subject for the root, the sanitised
// path_node_id when there is one, else its position in the graph.
func (q *queryWriter) nodeVar(n *exportgraph.ResourceNode) string {
	if n == q.root {
		return "?subject"
	}
	if id := n.Class.PathNodeID; id != "" {
		return "?" + sparqlVar(id)
	}
	return fmt.Sprintf("?s%d", q.pos[n])
}

// graphSteps walks the field's primary path through the export graph: the
// class steps the graph made nodes for, then the literal ending if any. A
// path that goes on after a literal is malformed.
func (q *queryWriter) graphSteps(field *generators.FieldNode) ([]step, error) {
	var steps []step
	var pending *domain.PathElement
	last := q.root
	for i := range field.Path {
		pn := &field.Path[i]
		e := pn.Element
		switch {
		case e.Type == "property":
			pending = &e
		case e.Type == "class" && !rdfterm.IsDatatype(e):
			n, ok := q.nodeOf[pn]
			if !ok || pending == nil {
				continue
			}
			steps = append(steps, step{predicate: *pending, nodeVar: q.nodeVar(n), types: classTypes(n.Class, n.AdditionalTypes)})
			last = n
			pending = nil
		case e.Type == "class" || e.Type == "literal" || pn.Role == generators.PathRoleLiteral:
			if pending != nil {
				steps = append(steps, step{predicate: *pending, literal: true})
			}
			if continuesAfter(field.Path[i+1:]) {
				return nil, fmt.Errorf("sparql renderer: field %s: path continues after a literal", oneLine(field.Field.SemanticID))
			}
			return steps, q.checkBinding(field, last)
		}
	}
	if pending != nil {
		steps = append(steps, step{predicate: *pending, literal: true})
	}
	return steps, q.checkBinding(field, last)
}

// checkBinding confirms the walk reached the node the graph bound the
// field to.
func (q *queryWriter) checkBinding(field *generators.FieldNode, last *exportgraph.ResourceNode) error {
	for _, fb := range q.graph.Fields {
		if fb.Source == field && last != nil && fb.NodeKey != last.Key {
			return fmt.Errorf("sparql renderer: field %s: path walk ends at %s, graph binds %s",
				oneLine(field.Field.SemanticID), oneLine(last.Key), oneLine(fb.NodeKey))
		}
	}
	return nil
}

// continuesAfter reports a property or class in the rest of a path.
func continuesAfter(rest []generators.PathNode) bool {
	for _, pn := range rest {
		if pn.Element.Type == "property" || pn.Element.Type == "class" {
			return true
		}
	}
	return false
}

// subPathSteps walks a legacy subfield path, which the export graph does
// not hold: class steps bind to their path_node_id when set, else ?sN.
func subPathSteps(path []generators.PathNode, scope domain.PathElement, sem string) ([]step, error) {
	var steps []step
	var pending *domain.PathElement
	for i, pn := range path {
		e := pn.Element
		switch {
		case e.Type == "property":
			pending = &e
		case e.Type == "class" && !rdfterm.IsDatatype(e):
			if pending == nil || (i == 0 && e.SameStep(scope)) {
				continue
			}
			v := fmt.Sprintf("?s%d", len(steps)+1)
			if e.PathNodeID != "" {
				v = "?" + sparqlVar(e.PathNodeID)
			}
			steps = append(steps, step{predicate: *pending, nodeVar: v, types: classTypes(e, e.AdditionalTypes)})
			pending = nil
		case e.Type == "class" || e.Type == "literal" || pn.Role == generators.PathRoleLiteral:
			if pending != nil {
				steps = append(steps, step{predicate: *pending, literal: true})
			}
			if continuesAfter(path[i+1:]) {
				return nil, fmt.Errorf("sparql renderer: field %s: path continues after a literal", oneLine(sem))
			}
			return steps, nil
		}
	}
	if pending != nil {
		steps = append(steps, step{predicate: *pending, literal: true})
	}
	return steps, nil
}

func classTypes(e domain.PathElement, additional []domain.TypeRef) []domain.TypeRef {
	return append([]domain.TypeRef{{URI: e.URI, Prefix: e.Prefix, LocalName: e.LocalName}}, additional...)
}

// writeField emits the field's primary query, then one independent query
// per legacy subfield path (same field, distinct value variable): subfields
// are additional value paths on the same entity, not required joins.
func (q *queryWriter) writeField(b *strings.Builder, field *generators.FieldNode) error {
	steps, err := q.graphSteps(field)
	if err != nil {
		return err
	}
	value := valueVar(field)
	if err := q.writeQuery(b, field, steps, value); err != nil {
		return err
	}
	for i, sub := range field.SubPaths {
		b.WriteByte('\n')
		writeComment(b, fmt.Sprintf("legacy subfield path %d", i+1))
		subSteps, err := subPathSteps(sub, field.Scope, field.Field.SemanticID)
		if err != nil {
			return err
		}
		if err := q.writeQuery(b, field, subSteps, fmt.Sprintf("%s_p%d", value, i+1)); err != nil {
			return err
		}
	}
	return nil
}

// writeQuery emits one SELECT (or COUNT) query over the steps. The last
// step's object is the value variable, or the field's set value: a set
// value ends the path on the value itself, and the query selects the node
// that carries it.
func (q *queryWriter) writeQuery(b *strings.Builder, field *generators.FieldNode, steps []step, value string) error {
	sem := field.Field.SemanticID
	if sem == "" {
		sem = field.Field.ID
	}
	sem = oneLine(sem)

	var body strings.Builder
	if q.snap.RootKind != generators.EntityField && q.root != nil {
		if err := q.writeTypeTriple(&body, "?subject", classTypes(q.root.Class, q.root.AdditionalTypes), sem); err != nil {
			return err
		}
	}

	selected, labelled := value, false
	subject := "?subject"
	vars := []string{"?subject"}
	for i, s := range steps {
		pred, err := q.name(s.predicate.Prefix, s.predicate.LocalName, s.predicate.URI, sem)
		if err != nil {
			return err
		}
		if i < len(steps)-1 {
			fmt.Fprintf(&body, "  %s %s %s .\n", subject, pred, s.nodeVar)
			vars = append(vars, s.nodeVar)
			if err := q.writeTypeTriple(&body, s.nodeVar, s.types, sem); err != nil {
				return err
			}
			subject = s.nodeVar
			continue
		}
		object := value
		if sv := field.Field.SetValue; sv != "" {
			object = rdfterm.Term(sv, q.snap.Namespaces)
			// Term expands a bound prefix without checking its namespace
			if !isIRIRef(object) {
				return fmt.Errorf("sparql renderer: field %s: set value %q is not an IRI", sem, sv)
			}
			selected = subject
		} else {
			labelled = !s.literal && wantsLabelClause(field.Field.ExpectedValueType)
		}
		fmt.Fprintf(&body, "  %s %s %s .\n", subject, pred, object)
	}

	if selected == value {
		vars = append(vars, value)
	}
	if labelled && !q.snap.Options.SPARQLCount {
		vars = append(vars, value+"_label")
	}
	if q.snap.Options.SPARQLCount {
		vars = append(vars, "?count")
	}
	if err := checkCollisions(vars, sem); err != nil {
		return err
	}

	if q.snap.Options.SPARQLCount {
		fmt.Fprintf(b, "SELECT (COUNT(%s) AS ?count) WHERE {\n", selected)
	} else {
		fmt.Fprintf(b, "SELECT DISTINCT %s WHERE {\n", selected)
	}
	b.WriteString(body.String())
	if labelled && !q.snap.Options.SPARQLCount {
		fmt.Fprintf(b, "  OPTIONAL { %s rdfs:label %s_label . }\n", value, value)
	}
	b.WriteString("}\n")
	if q.snap.Options.SPARQLLimit > 0 && !q.snap.Options.SPARQLCount {
		fmt.Fprintf(b, "LIMIT %d\n", q.snap.Options.SPARQLLimit)
	}
	return nil
}

// isIRIRef reports an <IRI> whose body holds no character an IRIREF
// cannot.
func isIRIRef(s string) bool {
	return len(s) >= 2 && s[0] == '<' && s[len(s)-1] == '>' && !strings.ContainsFunc(s[1:len(s)-1], rdfterm.NotIRIChar)
}

// checkCollisions refuses a query in which two roles share a variable.
func checkCollisions(vars []string, sem string) error {
	seen := make(map[string]bool, len(vars))
	for _, v := range vars {
		if seen[v] {
			return fmt.Errorf("sparql renderer: field %s: variable %s collides", sem, v)
		}
		seen[v] = true
	}
	return nil
}

// writeTypeTriple emits "subject a C1, C2 ." for the types that carry a
// name; nothing when none does.
func (q *queryWriter) writeTypeTriple(b *strings.Builder, subject string, types []domain.TypeRef, sem string) error {
	names := make([]string, 0, len(types))
	for _, t := range types {
		if (t.Prefix == "" || t.LocalName == "") && t.URI == "" {
			continue
		}
		n, err := q.name(t.Prefix, t.LocalName, t.URI, sem)
		if err != nil {
			return err
		}
		names = append(names, n)
	}
	if len(names) > 0 {
		fmt.Fprintf(b, "  %s a %s .\n", subject, strings.Join(names, ", "))
	}
	return nil
}

var (
	pnPrefix = regexp.MustCompile(`^[A-Za-z]([A-Za-z0-9_.-]*[A-Za-z0-9_-])?$`)
	pnLocal  = regexp.MustCompile(`^[A-Za-z0-9_]([A-Za-z0-9_.-]*[A-Za-z0-9_-])?$`)
)

// name writes an ontology term: a prefixed name when prefix and local name
// are plain names, else the IRI the bound prefix expands to, else the
// element's own absolute IRI. Anything else is an error, never raw text.
func (q *queryWriter) name(prefix, local, uri, sem string) (string, error) {
	if prefix != "" && local != "" {
		if pnPrefix.MatchString(prefix) && pnLocal.MatchString(local) {
			return prefix + ":" + local, nil
		}
		if base, ok := resolvePrefix(q.snap.Namespaces, prefix); ok && !strings.ContainsFunc(base+local, rdfterm.NotIRIChar) {
			return "<" + base + local + ">", nil
		}
	}
	if (strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") || strings.HasPrefix(uri, "urn:")) &&
		!strings.ContainsFunc(uri, rdfterm.NotIRIChar) {
		return "<" + uri + ">", nil
	}
	return "", fmt.Errorf("sparql renderer: field %s: term %q is not an IRI", sem, prefix+":"+local)
}

func resolvePrefix(ns generators.NamespaceSet, prefix string) (string, bool) {
	switch prefix {
	case "rdf":
		return rdfNamespace, true
	case "rdfs":
		return rdfsNamespace, true
	}
	return ns.ResolvePrefix(prefix)
}

// collectUsedPrefixes reports the namespace prefixes of the root class (for
// model and collection scopes) and every field path, plus rdf+rdfs which
// the renderer always uses.
func collectUsedPrefixes(snap *generators.Snapshot, root *exportgraph.ResourceNode) map[string]struct{} {
	used := map[string]struct{}{"rdf": {}, "rdfs": {}}
	add := func(e domain.PathElement) {
		if e.Prefix != "" {
			used[e.Prefix] = struct{}{}
		}
		for _, t := range e.AdditionalTypes {
			if t.Prefix != "" {
				used[t.Prefix] = struct{}{}
			}
		}
	}
	if root != nil && snap.RootKind != generators.EntityField {
		add(root.Class)
	}
	for _, field := range snap.Fields {
		add(field.Scope)
		for _, p := range append([][]generators.PathNode{field.Path}, field.SubPaths...) {
			for _, node := range p {
				add(node.Element)
			}
		}
	}
	return used
}

// writePrefixes emits PREFIX declarations for every used prefix. A prefix
// the bundle does not bind is a hard error so the caller knows the
// project's namespace bindings are incomplete; a prefix that is not a
// plain name is not declared (its terms are written as full IRIs).
func writePrefixes(b *strings.Builder, namespaces generators.NamespaceSet, used map[string]struct{}) error {
	var missing []string
	for prefix := range used {
		if _, ok := resolvePrefix(namespaces, prefix); !ok {
			missing = append(missing, prefix)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("sparql renderer: missing namespace binding(s) for prefix: %s", oneLine(strings.Join(missing, ", ")))
	}

	prefixes := make([]string, 0, len(used))
	for prefix := range used {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	for _, prefix := range prefixes {
		if !pnPrefix.MatchString(prefix) {
			continue
		}
		namespace, _ := resolvePrefix(namespaces, prefix)
		if strings.ContainsFunc(namespace, rdfterm.NotIRIChar) {
			return fmt.Errorf("sparql renderer: namespace for prefix %s is not an IRI: %q", prefix, namespace)
		}
		fmt.Fprintf(b, "PREFIX %s: <%s>\n", prefix, namespace)
	}
	b.WriteByte('\n')
	return nil
}

// writeComment writes a single-line comment.
func writeComment(b *strings.Builder, text string) {
	b.WriteString("# " + oneLine(text) + "\n")
}

// oneLine replaces control characters (line breaks among them) with spaces.
func oneLine(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == 0x85 || r == 0x2028 || r == 0x2029 {
			return ' '
		}
		return r
	}, s)
}

// sparqlVar turns an identifier into a SPARQL variable name: only
// [A-Za-z0-9_] survive (everything else becomes _), and a leading digit is
// prefixed with v_.
func sparqlVar(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	switch {
	case out == "":
		return "v"
	case out[0] >= '0' && out[0] <= '9':
		return "v_" + out
	}
	return out
}

// valueVar is the variable bound to a field's terminal value: the field's
// semantic id plus system name, so each field's value is a distinct,
// traceable column.
func valueVar(field *generators.FieldNode) string {
	base := field.Field.SemanticID
	if base == "" {
		base = field.Field.ID
	}
	if sn := field.Field.SystemName; sn != "" {
		base += "_" + sn
	}
	return "?" + sparqlVar(base)
}

// wantsLabelClause reports a resource value type for which rdfs:label is
// meaningful; literal value types get none.
func wantsLabelClause(expectedValueType string) bool {
	switch strings.ToLower(expectedValueType) {
	case "date", "integer", "string", "decimal", "boolean":
		return false
	}
	return true
}
