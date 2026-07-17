// Package rdf renders weave generator snapshots as RDF serializations.
package rdf

import (
	"context"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/views"
)

// TurtleRenderer writes a deterministic Turtle template from a weave snapshot.
type TurtleRenderer struct{}

func NewTurtleRenderer() *TurtleRenderer {
	return &TurtleRenderer{}
}

func (r *TurtleRenderer) Spec() generators.FormatSpec {
	return generators.FormatSpec{
		Format:        generators.FormatTurtle,
		ContentType:   "text/turtle; charset=utf-8",
		FileExtension: ".ttl",
		RequiresTree:  true,
	}
}

func (r *TurtleRenderer) Render(ctx context.Context, snap *generators.Snapshot, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if snap == nil {
		return fmt.Errorf("rdf turtle renderer: snapshot is nil")
	}
	if len(snap.Report.Errors) > 0 {
		return fmt.Errorf("rdf turtle renderer: snapshot has %d error(s)", len(snap.Report.Errors))
	}
	if snap.Namespaces.ProjectURI == "" {
		return fmt.Errorf("rdf turtle renderer: project namespace is empty")
	}

	graph := buildGraph(snap)
	var b strings.Builder
	baseURI := baseURI(snap)
	if err := writePrefixes(&b, graph.Namespaces, graph.Options.BasePrefix, baseURI, graph.UsedPrefixes); err != nil {
		return err
	}
	writeTurtleGraph(&b, graph)
	_, err := io.WriteString(w, b.String())
	return err
}

func writePrefixes(b *strings.Builder, namespaces generators.NamespaceSet, basePrefix string, baseURI string, usedPrefixes map[string]struct{}) error {
	if basePrefix == "" {
		basePrefix = "ex"
	}
	if baseURI != "" {
		fmt.Fprintf(b, "@base <%s> .\n", baseURI)
		fmt.Fprintf(b, "@prefix %s: <%s> .\n", basePrefix, baseURI)
	}

	var missing []string
	for prefix := range usedPrefixes {
		if prefix == "" || prefix == basePrefix {
			continue
		}
		if _, ok := namespaces.ResolvePrefix(prefix); ok {
			continue
		}
		missing = append(missing, prefix)
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("rdf turtle renderer: missing namespace binding(s) for prefix: %s", strings.Join(missing, ", "))
	}
	prefixes := make([]string, 0, len(usedPrefixes))
	for prefix := range usedPrefixes {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	for _, prefix := range prefixes {
		if prefix == "" || prefix == basePrefix {
			continue
		}
		namespace, _ := namespaces.ResolvePrefix(prefix)
		if prefix == namespaces.ProjectPrefix && namespace == baseURI {
			continue
		}
		fmt.Fprintf(b, "@prefix %s: <%s> .\n", prefix, namespace)
	}
	b.WriteByte('\n')
	return nil
}

func collectUsedPrefixes(snap *generators.Snapshot) map[string]struct{} {
	used := make(map[string]struct{})
	if snap.Options.RDFNodeMode != generators.RDFNodeModeBlankNode {
		used["rdf"] = struct{}{}
	}
	for _, field := range fieldsInTreeOrder(snap) {
		addElementPrefix(used, fieldSubjectScope(snap, field))
		for _, node := range field.Path {
			if node.Element.Type == "literal" {
				continue
			}
			addElementPrefix(used, node.Element)
		}
	}
	return used
}

func addElementPrefix(used map[string]struct{}, element domain.PathElement) {
	if element.Prefix != "" {
		used[element.Prefix] = struct{}{}
	}
	for _, additional := range element.AdditionalTypes {
		if additional.Prefix != "" {
			used[additional.Prefix] = struct{}{}
		}
	}
}

func baseURI(snap *generators.Snapshot) string {
	base := strings.TrimSpace(snap.Options.BaseURI)
	if base == "" {
		base = snap.Namespaces.ProjectURI
	}
	if base == "" {
		return ""
	}
	if !strings.HasSuffix(base, "/") && !strings.HasSuffix(base, "#") {
		base += "/"
	}
	return base
}

type RDFGraph struct {
	Namespaces   generators.NamespaceSet
	Options      generators.Options
	UsedPrefixes map[string]struct{}
	Groups       []RDFGraphGroup
}

type RDFGraphGroup struct {
	Comment string
	Triples *tripleSet
}

func buildGraph(snap *generators.Snapshot) *RDFGraph {
	graph := &RDFGraph{
		Namespaces:   snap.Namespaces,
		Options:      snap.Options,
		UsedPrefixes: collectUsedPrefixes(snap),
	}
	if snap.RootKind == generators.EntityModel {
		comments := groupComments(snap)
		triples := newTripleSet()
		for _, field := range fieldsInTreeOrder(snap) {
			writeField(triples, snap, field, comments[fieldGroupPath(snap, field)])
		}
		graph.addGroup("", triples)
		return graph
	}
	lastGroup := ""
	triples := newTripleSet()
	comments := groupComments(snap)
	for _, field := range fieldsInTreeOrder(snap) {
		group := fieldGroupPath(snap, field)
		if lastGroup != "" && group != lastGroup {
			addModelGroupLink(snap, triples, lastGroup)
			graph.addGroup(comments[lastGroup], triples)
			triples = newTripleSet()
		}
		lastGroup = group
		writeField(triples, snap, field, "")
	}
	addModelGroupLink(snap, triples, lastGroup)
	graph.addGroup(comments[lastGroup], triples)
	return graph
}

func (g *RDFGraph) addGroup(comment string, triples *tripleSet) {
	if triples == nil || len(triples.subjects) == 0 {
		return
	}
	g.Groups = append(g.Groups, RDFGraphGroup{
		Comment: comment,
		Triples: triples,
	})
}

func addModelGroupLink(snap *generators.Snapshot, triples *tripleSet, group string) {
	if snap == nil || snap.RootKind != generators.EntityModel || snap.Tree == nil || group == "" || group == snap.Tree.Root.Path {
		return
	}
	triples.Add(resourceIRI(snap, group), "dcterms:isPartOf", resourceIRI(snap, snap.Tree.Root.Path))
}

func writeTurtleGraph(b *strings.Builder, graph *RDFGraph) {
	for _, group := range graph.Groups {
		writeComment(b, group.Comment)
		if graph.Options.RDFNodeMode == generators.RDFNodeModeBlankNode {
			group.Triples.WriteBlankNodes(b)
			continue
		}
		group.Triples.Write(b)
	}
}

func groupComments(snap *generators.Snapshot) map[string]string {
	comments := make(map[string]string)
	if snap == nil || snap.Tree == nil {
		return comments
	}
	if snap.Collection != nil {
		comments[snap.Tree.Root.Path] = collectionComment(snap.Collection.SemanticID, snap.Collection.SystemName, snap.Collection.UIName, snap.Options.Lang)
	}
	if snap.RootKind != generators.EntityModel {
		return comments
	}
	var walk func(node views.TreeNode)
	walk = func(node views.TreeNode) {
		if node.Kind == views.NodeCollection {
			comments[node.Path] = collectionComment(node.SemanticID, node.SystemName, node.Label, snap.Options.Lang)
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(snap.Tree.Root)
	return comments
}

func collectionComment(semanticID string, systemName string, label domain.Translations, lang string) string {
	name := strings.TrimSpace(label.Get(lang))
	if name == "" {
		name = strings.TrimSpace(systemName)
	}
	switch {
	case semanticID != "" && name != "":
		return "Collection " + semanticID + " " + name
	case semanticID != "":
		return "Collection " + semanticID
	case name != "":
		return "Collection " + name
	default:
		return ""
	}
}

func writeComment(b *strings.Builder, comment string) {
	for _, line := range commentLines(comment) {
		fmt.Fprintf(b, "# %s\n", line)
	}
}

func fieldsInTreeOrder(snap *generators.Snapshot) []generators.FieldNode {
	if snap == nil || snap.Tree == nil {
		return nil
	}
	byPath := make(map[string]generators.FieldNode, len(snap.Fields))
	for _, field := range snap.Fields {
		byPath[field.RelativePath] = field
	}

	ordered := make([]generators.FieldNode, 0, len(snap.Fields))
	seen := make(map[string]struct{}, len(snap.Fields))
	var walk func(node views.TreeNode)
	walk = func(node views.TreeNode) {
		if node.Kind == views.NodeField {
			if field, ok := byPath[node.Path]; ok {
				ordered = append(ordered, field)
				seen[field.RelativePath] = struct{}{}
			}
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(snap.Tree.Root)

	for _, field := range snap.Fields {
		if _, ok := seen[field.RelativePath]; ok {
			continue
		}
		ordered = append(ordered, field)
	}
	return ordered
}

type tripleSet struct {
	subjects        []string
	seen            map[string]struct{}
	bySubj          map[string]map[string][]string
	predsBySubj     map[string][]string
	seenPredByID    map[string]struct{}
	subjectComments map[string][]string
	comments        map[string]map[string][]string
	seenComment     map[string]struct{}
}

func newTripleSet() *tripleSet {
	return &tripleSet{
		seen:            make(map[string]struct{}),
		bySubj:          make(map[string]map[string][]string),
		predsBySubj:     make(map[string][]string),
		seenPredByID:    make(map[string]struct{}),
		subjectComments: make(map[string][]string),
		comments:        make(map[string]map[string][]string),
		seenComment:     make(map[string]struct{}),
	}
}

func (s *tripleSet) Add(subject, predicate, object string) {
	s.AddComment(subject, predicate, object, "")
}

func (s *tripleSet) AddComment(subject, predicate, object string, comment string) {
	s.addComment(subject, predicate, comment)
	key := subject + "\x00" + predicate + "\x00" + object
	if _, ok := s.seen[key]; ok {
		return
	}
	s.seen[key] = struct{}{}
	if _, ok := s.bySubj[subject]; !ok {
		s.bySubj[subject] = make(map[string][]string)
		s.subjects = append(s.subjects, subject)
	}
	predKey := subject + "\x00" + predicate
	if _, ok := s.seenPredByID[predKey]; !ok {
		s.seenPredByID[predKey] = struct{}{}
		s.predsBySubj[subject] = append(s.predsBySubj[subject], predicate)
	}
	s.bySubj[subject][predicate] = append(s.bySubj[subject][predicate], object)
}

func (s *tripleSet) addComment(subject, predicate, comment string) {
	for _, line := range commentLines(comment) {
		key := subject + "\x00" + predicate + "\x00" + line
		if _, ok := s.seenComment[key]; ok {
			continue
		}
		s.seenComment[key] = struct{}{}
		if _, ok := s.comments[subject]; !ok {
			s.comments[subject] = make(map[string][]string)
		}
		s.comments[subject][predicate] = append(s.comments[subject][predicate], line)
	}
}

func (s *tripleSet) AddSubjectComment(subject, comment string) {
	for _, line := range commentLines(comment) {
		key := "subject\x00" + subject + "\x00" + line
		if _, ok := s.seenComment[key]; ok {
			continue
		}
		s.seenComment[key] = struct{}{}
		s.subjectComments[subject] = append(s.subjectComments[subject], line)
	}
}

func (s *tripleSet) Write(b *strings.Builder) {
	if len(s.subjects) == 0 {
		return
	}
	for subjectIndex, subject := range s.subjects {
		if subjectIndex > 0 {
			b.WriteByte('\n')
		}
		predicates := orderedPredicates(s.predsBySubj[subject])
		b.WriteString(subject)
		for predicateIndex, predicate := range predicates {
			comments := s.comments[subject][predicate]
			if predicateIndex == 0 {
				if len(comments) == 0 && len(s.subjectComments[subject]) == 0 {
					b.WriteByte(' ')
				} else {
					b.WriteByte('\n')
				}
			} else {
				b.WriteString(" ;\n")
			}
			if predicateIndex == 0 {
				for _, comment := range s.subjectComments[subject] {
					fmt.Fprintf(b, "    # %s\n", comment)
				}
			}
			for _, comment := range comments {
				fmt.Fprintf(b, "    # %s\n", comment)
			}
			if predicateIndex > 0 || len(comments) > 0 || len(s.subjectComments[subject]) > 0 {
				b.WriteString("    ")
			}
			objects := s.bySubj[subject][predicate]
			fmt.Fprintf(b, "%s ", predicate)
			for objectIndex, object := range objects {
				if objectIndex > 0 {
					b.WriteString(",\n        ")
				}
				b.WriteString(object)
			}
		}
		b.WriteString(" .\n")
	}
	b.WriteByte('\n')
}

func (s *tripleSet) WriteBlankNodes(b *strings.Builder) {
	if len(s.subjects) == 0 {
		return
	}
	referenced := s.referencedSubjects()
	wrote := false
	for _, subject := range s.subjects {
		if _, ok := referenced[subject]; ok {
			continue
		}
		if wrote {
			b.WriteByte('\n')
		}
		s.writeResourceSubject(b, subject, make(map[string]bool))
		wrote = true
	}
	if !wrote {
		for _, subject := range s.subjects {
			if wrote {
				b.WriteByte('\n')
			}
			s.writeResourceSubject(b, subject, make(map[string]bool))
			wrote = true
		}
	}
	b.WriteByte('\n')
}

func (s *tripleSet) referencedSubjects() map[string]struct{} {
	referenced := make(map[string]struct{})
	for _, predicates := range s.bySubj {
		for _, objects := range predicates {
			for _, object := range objects {
				if _, ok := s.bySubj[object]; ok {
					referenced[object] = struct{}{}
				}
			}
		}
	}
	return referenced
}

func (s *tripleSet) writeResourceSubject(b *strings.Builder, subject string, visited map[string]bool) {
	b.WriteString(subject)
	s.writePredicateList(b, subject, 0, visited, true)
	b.WriteString(" .\n")
}

func (s *tripleSet) writeBlankNode(b *strings.Builder, subject string, predicateIndent int, visited map[string]bool) {
	if visited[subject] {
		b.WriteString(subject)
		return
	}
	visited[subject] = true
	b.WriteString("[")
	s.writePredicateList(b, subject, predicateIndent, visited, false)
	b.WriteByte('\n')
	writeIndent(b, predicateIndent)
	b.WriteString("]")
	visited[subject] = false
}

func (s *tripleSet) writePredicateList(b *strings.Builder, subject string, baseIndent int, visited map[string]bool, resourceSubject bool) {
	predicates := orderedPredicates(s.predsBySubj[subject])
	predicateIndent := baseIndent + 4
	for predicateIndex, predicate := range predicates {
		comments := s.comments[subject][predicate]
		if predicateIndex == 0 {
			if len(comments) == 0 && len(s.subjectComments[subject]) == 0 && resourceSubject {
				b.WriteByte(' ')
			} else {
				b.WriteByte('\n')
			}
		} else {
			b.WriteString(" ;\n")
		}
		if predicateIndex == 0 {
			for _, comment := range s.subjectComments[subject] {
				writeIndent(b, predicateIndent)
				fmt.Fprintf(b, "# %s\n", comment)
			}
		}
		for _, comment := range comments {
			writeIndent(b, predicateIndent)
			fmt.Fprintf(b, "# %s\n", comment)
		}
		if predicateIndex > 0 || len(comments) > 0 || len(s.subjectComments[subject]) > 0 || !resourceSubject {
			writeIndent(b, predicateIndent)
		}
		objects := s.bySubj[subject][predicate]
		fmt.Fprintf(b, "%s ", blankNodePredicate(predicate))
		for objectIndex, object := range objects {
			if objectIndex > 0 {
				b.WriteString(",\n")
				writeIndent(b, predicateIndent+4)
			}
			if _, ok := s.bySubj[object]; ok {
				s.writeBlankNode(b, object, predicateIndent, visited)
				continue
			}
			b.WriteString(object)
		}
	}
}

func writeIndent(b *strings.Builder, n int) {
	for i := 0; i < n; i++ {
		b.WriteByte(' ')
	}
}

func blankNodePredicate(predicate string) string {
	if predicate == "rdf:type" {
		return "a"
	}
	return predicate
}

func orderedPredicates(in []string) []string {
	predicates := make([]string, 0, len(in))
	for _, predicate := range in {
		if predicate == "rdf:type" {
			predicates = append(predicates, predicate)
			break
		}
	}
	for _, predicate := range in {
		if predicate != "rdf:type" {
			predicates = append(predicates, predicate)
		}
	}
	if len(predicates) == 0 && len(in) > 0 {
		predicates = append(predicates, "rdf:type")
	}
	return predicates
}

func commentLines(comment string) []string {
	comment = strings.ReplaceAll(comment, "\r\n", "\n")
	comment = strings.ReplaceAll(comment, "\r", "\n")
	lines := strings.Split(comment, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func writeField(triples *tripleSet, snap *generators.Snapshot, field generators.FieldNode, branchComment string) {
	subjectPath := fieldSubjectPath(snap, field)
	subject := resourceIRI(snap, subjectPath)
	addTypeTriples(triples, subject, fieldSubjectScope(snap, field))

	currentSubject := subject
	currentPath := subjectPath
	classPath := make([]string, 0, len(field.Path))
	for i := 0; i < len(field.Path); i++ {
		node := field.Path[i]
		if node.Direction != generators.PathDirectionForward {
			continue
		}
		if node.Role != generators.PathRoleProperty && node.Element.Type != "property" {
			continue
		}
		if i+1 >= len(field.Path) {
			continue
		}

		predicate := term(node.Element)
		next := field.Path[i+1]
		comment := ""
		switch next.Element.Type {
		case "class":
			objectPath := classResourcePath(snap, currentPath, field, classPath, node.Slug, next.Slug)
			object := resourceIRI(snap, objectPath)
			if branchComment != "" {
				triples.AddSubjectComment(object, branchComment)
				branchComment = ""
			}
			if i+2 >= len(field.Path) {
				comment = fieldDescription(field.Field, snap.Options.Lang)
			}
			triples.AddComment(currentSubject, predicate, object, comment)
			addTypeTriples(triples, object, next.Element)
			currentPath = objectPath
			currentSubject = object
			classPath = append(classPath, next.Slug)
			i++
		case "literal":
			comment = fieldDescription(field.Field, snap.Options.Lang)
			triples.AddComment(currentSubject, predicate, literalValue(field.Field), comment)
			i++
		}
	}
}

func fieldDescription(field domain.ResolvedField, lang string) string {
	label := strings.TrimSpace(field.DisplayName.Get(lang))
	head := ""
	switch {
	case field.SemanticID != "" && label != "":
		head = field.SemanticID + " " + label
	case field.SemanticID != "" && field.SystemName != "":
		head = field.SemanticID + " " + field.SystemName
	case field.SemanticID != "":
		head = field.SemanticID
	case label != "":
		head = label
	default:
		head = field.SystemName
	}
	description := strings.TrimSpace(field.Description.Get(lang))
	if description == "" || description == head {
		return head
	}
	return head + "\n" + description
}

func fieldResourcePath(snap *generators.Snapshot, field generators.FieldNode) string {
	return fieldGroupPath(snap, field)
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

func fieldSubjectPath(snap *generators.Snapshot, field generators.FieldNode) string {
	if snap != nil && snap.RootKind == generators.EntityModel && snap.Tree != nil {
		return snap.Tree.Root.Path
	}
	return fieldGroupPath(snap, field)
}

func fieldSubjectScope(snap *generators.Snapshot, field generators.FieldNode) domain.PathElement {
	if snap != nil && snap.RootKind == generators.EntityModel && snap.Tree != nil && snap.Tree.Root.Scope != nil {
		return *snap.Tree.Root.Scope
	}
	return field.Scope
}

func classResourcePath(
	snap *generators.Snapshot,
	currentPath string,
	field generators.FieldNode,
	classPath []string,
	propertySlug string,
	classSlug string,
) string {
	if snap != nil && snap.RootKind == generators.EntityModel {
		return path.Join(currentPath, propertySlug, classSlug)
	}
	parts := append([]string{fieldGroupPath(snap, field)}, classPath...)
	parts = append(parts, classSlug)
	return path.Join(parts...)
}

func resourceIRI(snap *generators.Snapshot, relativePath string) string {
	return "<" + compactResourcePath(relativePath) + ">"
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

func term(element domain.PathElement) string {
	if name := prefixedName(element); name != "" {
		return name
	}
	if element.URI != "" {
		return "<" + element.URI + ">"
	}
	return "<urn:pletka:unknown>"
}

func prefixedName(element domain.PathElement) string {
	if element.Prefix == "" || element.LocalName == "" {
		return ""
	}
	return element.Prefix + ":" + strings.TrimPrefix(element.LocalName, "^")
}

// addTypeTriples emits one rdf:type triple per class on the element.
// The tripleSet groups objects under the same predicate, producing
// "?s a Class1, Class2 ." in Turtle output.
func addTypeTriples(triples *tripleSet, subject string, element domain.PathElement) {
	for _, t := range element.AllTypes() {
		name := typeRefName(t)
		if name == "" {
			continue
		}
		triples.Add(subject, "rdf:type", name)
	}
}

func typeRefName(t domain.TypeRef) string {
	if t.Prefix == "" || t.LocalName == "" {
		return ""
	}
	return t.Prefix + ":" + strings.TrimPrefix(t.LocalName, "^")
}

func literalValue(field domain.ResolvedField) string {
	value := field.SetValue
	if value == "" {
		value = SlugLiteral(field.SystemName, field.SemanticID, field.ID) + "_value"
	}
	return `"` + escapeLiteral(value) + `"`
}

func SlugLiteral(parts ...string) string {
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
