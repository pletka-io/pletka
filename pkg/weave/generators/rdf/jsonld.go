package rdf

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/pletka-io/pletka/pkg/weave/generators"
)

type JSONLDRenderer struct{}

func NewJSONLDRenderer() *JSONLDRenderer {
	return &JSONLDRenderer{}
}

func (r *JSONLDRenderer) Spec() generators.FormatSpec {
	return generators.FormatSpec{
		Format:        generators.FormatJSONLD,
		ContentType:   "application/ld+json; charset=utf-8",
		FileExtension: ".jsonld",
		RequiresTree:  true,
	}
}

func (r *JSONLDRenderer) Render(ctx context.Context, snap *generators.Snapshot, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if snap == nil {
		return fmt.Errorf("rdf json-ld renderer: snapshot is nil")
	}
	if len(snap.Report.Errors) > 0 {
		return fmt.Errorf("rdf json-ld renderer: snapshot has %d error(s)", len(snap.Report.Errors))
	}
	if snap.Namespaces.ProjectURI == "" {
		return fmt.Errorf("rdf json-ld renderer: project namespace is empty")
	}

	graph := buildGraph(snap)
	doc, err := jsonLDDocument(graph)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

func jsonLDDocument(graph *RDFGraph) (map[string]any, error) {
	context, err := jsonLDContext(graph)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"@context": context,
		"@graph":   jsonLDGraph(graph),
	}, nil
}

func jsonLDContext(graph *RDFGraph) (map[string]any, error) {
	ctx := map[string]any{}
	if base := baseURIFromOptions(graph.Options, graph.Namespaces); base != "" {
		ctx["@base"] = base
		ctx[basePrefix(graph.Options.BasePrefix)] = base
	}
	for prefix := range jsonLDUsedPrefixes(graph) {
		if prefix == "" || prefix == graph.Options.BasePrefix {
			continue
		}
		if ns, ok := graph.Namespaces.ResolvePrefix(prefix); ok {
			ctx[prefix] = ns
			continue
		}
		return nil, fmt.Errorf("rdf json-ld renderer: missing namespace binding for prefix %q", prefix)
	}
	return ctx, nil
}

func jsonLDUsedPrefixes(graph *RDFGraph) map[string]struct{} {
	used := make(map[string]struct{})
	for _, group := range graph.Groups {
		for _, predicates := range group.Triples.bySubj {
			for predicate, values := range predicates {
				if predicate == "rdf:type" {
					for _, value := range values {
						addCompactTermPrefix(used, value)
					}
					continue
				}
				addCompactTermPrefix(used, predicate)
			}
		}
	}
	return used
}

func addCompactTermPrefix(used map[string]struct{}, term string) {
	if strings.HasPrefix(term, "<") || strings.HasPrefix(term, "\"") {
		return
	}
	prefix, _, ok := strings.Cut(term, ":")
	if !ok || prefix == "" {
		return
	}
	used[prefix] = struct{}{}
}

func jsonLDGraph(graph *RDFGraph) []any {
	out := make([]any, 0)
	for _, group := range graph.Groups {
		for _, subject := range group.Triples.rootSubjects(graph.Options.RDFNodeMode == generators.RDFNodeModeBlankNode) {
			out = append(out, graph.GroupObject(group, subject, false, make(map[string]bool)))
		}
	}
	return out
}

func (graph *RDFGraph) GroupObject(group RDFGraphGroup, subject string, embedded bool, visited map[string]bool) map[string]any {
	obj := make(map[string]any)
	if !embedded || graph.Options.RDFNodeMode != generators.RDFNodeModeBlankNode {
		obj["@id"] = trimIRI(subject)
	}
	if visited[subject] {
		return obj
	}
	visited[subject] = true
	for _, predicate := range orderedPredicates(group.Triples.predsBySubj[subject]) {
		values := group.Triples.bySubj[subject][predicate]
		comments := group.Triples.comments[subject][predicate]
		if predicate == "rdf:type" {
			obj["@type"] = jsonLDTypeValue(values)
			continue
		}
		rendered := make([]any, 0, len(values))
		for i, value := range values {
			rendered = append(rendered, graph.jsonLDValue(group, value, commentAt(comments, i), visited))
		}
		obj[predicate] = compactJSONLDValue(rendered)
	}
	visited[subject] = false
	return obj
}

func (graph *RDFGraph) jsonLDValue(group RDFGraphGroup, value string, label string, visited map[string]bool) any {
	if isIRI(value) {
		id := trimIRI(value)
		if graph.Options.RDFNodeMode == generators.RDFNodeModeBlankNode {
			if _, ok := group.Triples.bySubj[value]; ok {
				obj := graph.GroupObject(group, value, true, visited)
				addJSONLDLabel(obj, label)
				return obj
			}
		}
		obj := map[string]any{"@id": id}
		addJSONLDLabel(obj, label)
		return obj
	}
	obj := map[string]any{"@value": unquoteLiteral(value)}
	addJSONLDLabel(obj, label)
	return obj
}

func jsonLDTypeValue(values []string) any {
	types := make([]string, 0, len(values))
	for _, value := range values {
		types = append(types, strings.TrimPrefix(strings.TrimSuffix(value, ">"), "<"))
	}
	if len(types) == 1 {
		return types[0]
	}
	return types
}

func compactJSONLDValue(values []any) any {
	if len(values) == 1 {
		return values[0]
	}
	return values
}

func commentAt(comments []string, index int) string {
	if len(comments) == 0 {
		return ""
	}
	if index < len(comments) {
		return comments[index]
	}
	return comments[0]
}

func addJSONLDLabel(obj map[string]any, label string) {
	if label != "" {
		obj["_label"] = label
	}
}

func (s *tripleSet) rootSubjects(blank bool) []string {
	if !blank {
		return append([]string(nil), s.subjects...)
	}
	referenced := s.referencedSubjects()
	out := make([]string, 0, len(s.subjects))
	for _, subject := range s.subjects {
		if _, ok := referenced[subject]; !ok {
			out = append(out, subject)
		}
	}
	if len(out) > 0 {
		return out
	}
	return append([]string(nil), s.subjects...)
}

func isIRI(value string) bool {
	return strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">")
}

func trimIRI(value string) string {
	return strings.TrimSuffix(strings.TrimPrefix(value, "<"), ">")
}

func unquoteLiteral(value string) string {
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return value
	}
	var out strings.Builder
	escaped := false
	for _, r := range value[1 : len(value)-1] {
		if escaped {
			switch r {
			case 'n':
				out.WriteByte('\n')
			case 'r':
				out.WriteByte('\r')
			default:
				out.WriteRune(r)
			}
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

func baseURIFromOptions(opts generators.Options, namespaces generators.NamespaceSet) string {
	base := strings.TrimSpace(opts.BaseURI)
	if base == "" {
		base = namespaces.ProjectURI
	}
	if base == "" {
		return ""
	}
	if !strings.HasSuffix(base, "/") && !strings.HasSuffix(base, "#") {
		base += "/"
	}
	return base
}

func basePrefix(prefix string) string {
	if prefix == "" {
		return "ex"
	}
	return prefix
}
