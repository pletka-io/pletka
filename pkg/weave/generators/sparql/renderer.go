// Package sparql renders a weave generator snapshot as a SPARQL SELECT
// query (or COUNT variant). Mirrors the Python SparqlTransformer's
// observable shape — PREFIX block, SELECT of the field value variable,
// WHERE pattern that walks the field path with one Triple per property,
// optional rdfs:label clause, OPTIONAL Set_Value type filter — but
// consumes the resolved snapshot directly so we can drop the
// Airtable-formula complexity the Python version inherited.
//
// Variable naming: each class hop binds to its generated
// path_node_id, and the terminal value binds to the field's semantic id
// plus system name — so variables are traceable and never collide when a
// model/collection merges many field queries.
//
// Scopes: field roots emit one SELECT statement; model + collection
// roots emit one SELECT statement per resolved field, separated by a
// blank line so consumers can run them individually or feed the whole
// stream to a SPARQL endpoint that accepts multiple queries.
package sparql

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
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

	var b strings.Builder
	used := collectUsedPrefixes(snap)
	if err := writePrefixes(&b, snap.Namespaces, used); err != nil {
		return err
	}

	first := true
	for _, field := range snap.Fields {
		if !first {
			b.WriteByte('\n')
		}
		first = false
		writeFieldQuery(&b, snap, field)
	}

	_, err := io.WriteString(w, b.String())
	return err
}

// collectUsedPrefixes scans every field path and reports the namespace
// prefixes that appear, plus rdf+rdfs which the renderer always uses.
func collectUsedPrefixes(snap *generators.Snapshot) map[string]struct{} {
	used := map[string]struct{}{
		"rdf":  {},
		"rdfs": {},
	}
	for _, field := range snap.Fields {
		if field.Scope.Prefix != "" {
			used[field.Scope.Prefix] = struct{}{}
		}
		for _, t := range field.Scope.AdditionalTypes {
			if t.Prefix != "" {
				used[t.Prefix] = struct{}{}
			}
		}
		paths := append([][]generators.PathNode{field.Path}, field.SubPaths...)
		for _, p := range paths {
			for _, node := range p {
				if node.Element.Prefix != "" {
					used[node.Element.Prefix] = struct{}{}
				}
				for _, t := range node.Element.AdditionalTypes {
					if t.Prefix != "" {
						used[t.Prefix] = struct{}{}
					}
				}
			}
		}
	}
	return used
}

// writePrefixes emits PREFIX declarations for every namespace that
// appears in any field path. Prefixes referenced by the snapshot but
// missing from the bundle surface as a hard error so the caller knows
// the project's namespace bindings are incomplete.
func writePrefixes(b *strings.Builder, namespaces generators.NamespaceSet, used map[string]struct{}) error {
	var missing []string
	for prefix := range used {
		if prefix == "rdf" || prefix == "rdfs" {
			continue
		}
		if _, ok := namespaces.ResolvePrefix(prefix); !ok {
			missing = append(missing, prefix)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("sparql renderer: missing namespace binding(s) for prefix: %s", strings.Join(missing, ", "))
	}

	prefixes := make([]string, 0, len(used))
	for prefix := range used {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	for _, prefix := range prefixes {
		switch prefix {
		case "rdf":
			b.WriteString("PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>\n")
		case "rdfs":
			b.WriteString("PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>\n")
		default:
			namespace, _ := namespaces.ResolvePrefix(prefix)
			fmt.Fprintf(b, "PREFIX %s: <%s>\n", prefix, namespace)
		}
	}
	b.WriteByte('\n')
	return nil
}

// writeFieldQuery emits one SELECT (or COUNT) query for a single
// FieldNode. Variable layout:
//
//	?subject — root entity matching the field's scope class
//	?{path_node_id} — intermediate path resources (one per class hop)
//	?{semID}_{systemName} — terminal value (literal or resource)
//
// rdfs:label is added as an OPTIONAL clause when the terminal is a
// non-temporal resource — matches the Python heuristic.
func writeFieldQuery(b *strings.Builder, snap *generators.Snapshot, field generators.FieldNode) {
	// Primary path query, then one independent query per legacy subfield path
	// (same field binding / scope class, distinct value variable). Each path
	// is a self-contained BGP — subfields are additional value-paths on the
	// same entity, not required joins, so they never constrain each other.
	writeOnePathQuery(b, snap, field, field.Path, valueVar(field))
	for i, sub := range field.SubPaths {
		b.WriteByte('\n')
		fmt.Fprintf(b, "# legacy subfield path %d\n", i+1)
		writeOnePathQuery(b, snap, field, sub, fmt.Sprintf("%s_p%d", valueVar(field), i+1))
	}
}

// writeOnePathQuery emits one SELECT (or COUNT) query for a single path of a
// field, binding the terminal to value.
func writeOnePathQuery(b *strings.Builder, snap *generators.Snapshot, field generators.FieldNode, pathNodes []generators.PathNode, value string) {
	if snap.Options.SPARQLCount {
		writeQueryHeader(b, fmt.Sprintf("SELECT (COUNT(%s) AS ?count) WHERE {", value))
	} else {
		writeQueryHeader(b, fmt.Sprintf("SELECT DISTINCT %s WHERE {", value))
	}

	// Type triple for the field's scope class.
	writeTypeTriple(b, "?subject", field.Scope)

	// Path triples. Each class hop's variable is its path_node_id; the
	// terminal value lands in the field's own value variable.
	subject := "?subject"
	classNames := classVarNames(pathNodes)
	for i, node := range pathNodes {
		switch node.Role {
		case generators.PathRoleProperty:
			object := nextObjectVar(pathNodes, i, classNames, value)
			fmt.Fprintf(b, "  %s %s:%s %s .\n", subject, node.Element.Prefix, node.Element.LocalName, object)
			subject = object
		case generators.PathRoleClass:
			// Class hops emit a type triple on the subject we just bound.
			if subject != "?subject" {
				writeTypeTriple(b, subject, node.Element)
			}
		case generators.PathRoleLiteral, generators.PathRoleTerminal:
			// Terminal values land in the value var directly via the
			// preceding property triple — nothing to emit here.
		}
	}

	// Set_Value (override / base) → crm:P2_has_type filter, mirroring Python.
	if setValue := fieldSetValue(field.Field); setValue != "" {
		fmt.Fprintf(b, "  %s <http://www.cidoc-crm.org/cidoc-crm/P2_has_type> <%s> .\n", value, setValue)
	}

	// rdfs:label OPTIONAL clause for non-temporal resource terminals.
	if !snap.Options.SPARQLCount && wantsLabelClause(pathNodes, field.Field.ExpectedValueType) {
		fmt.Fprintf(b, "  OPTIONAL { %s rdfs:label %s_label . }\n", value, value)
	}

	b.WriteString("}\n")
	if snap.Options.SPARQLLimit > 0 && !snap.Options.SPARQLCount {
		fmt.Fprintf(b, "LIMIT %d\n", snap.Options.SPARQLLimit)
	}
}

func writeQueryHeader(b *strings.Builder, line string) {
	b.WriteString(line)
	b.WriteByte('\n')
}

// classVarNames assigns a variable name to every intermediate class hop
// in the path: its generated path_node_id, so the
// variable traces back to the node and class nodes reached through the
// same placement+prefix coreference instead of colliding on a bare ?sN
// counter. ?subject stays the scope class. Falls back to ?sN only for
// elements that carry no path_node_id.
func classVarNames(path []generators.PathNode) map[int]string {
	names := map[int]string{}
	counter := 0
	for i, node := range path {
		if node.Role != generators.PathRoleClass {
			continue
		}
		counter++
		if id := node.Element.PathNodeID; id != "" {
			names[i] = "?" + id
		} else {
			names[i] = fmt.Sprintf("?s%d", counter)
		}
	}
	return names
}

// sparqlVar normalises an identifier into a SPARQL variable token —
// dots, dashes, spaces and colons collapsed to underscores.
func sparqlVar(s string) string {
	r := strings.NewReplacer(".", "_", "-", "_", " ", "_", ":", "_")
	return r.Replace(strings.TrimSpace(s))
}

// valueVar is the SPARQL variable bound to a field's terminal value —
// the field's semantic id + system name, so each field's value is a
// distinct, traceable column rather than every field merging onto a
// shared ?value.
func valueVar(field generators.FieldNode) string {
	base := field.Field.SemanticID
	if base == "" {
		base = field.Field.ID
	}
	if sn := field.Field.SystemName; sn != "" {
		base += "_" + sn
	}
	return "?" + sparqlVar(base)
}

// nextObjectVar returns the variable name for the property triple's
// object position. Property at path index i targets the next class node
// (or the literal terminal which collapses onto the field value var).
func nextObjectVar(path []generators.PathNode, i int, classNames map[int]string, value string) string {
	for j := i + 1; j < len(path); j++ {
		switch path[j].Role {
		case generators.PathRoleClass:
			if name, ok := classNames[j]; ok {
				return name
			}
			return fmt.Sprintf("?s%d", j)
		case generators.PathRoleLiteral, generators.PathRoleTerminal:
			return value
		}
	}
	// Property is the path's last node — its object is the value.
	return value
}

// writeTypeTriple emits "subject a Class1, Class2 ." with one class per
// type on the element (primary + additionals). No output when neither
// prefix nor local-name resolves.
func writeTypeTriple(b *strings.Builder, subject string, element domain.PathElement) {
	types := element.AllTypes()
	names := make([]string, 0, len(types))
	for _, t := range types {
		if t.Prefix == "" || t.LocalName == "" {
			continue
		}
		names = append(names, t.Prefix+":"+t.LocalName)
	}
	if len(names) == 0 {
		return
	}
	fmt.Fprintf(b, "  %s a %s .\n", subject, strings.Join(names, ", "))
}

// fieldSetValue returns the resolved Set_Value URI for this field.
// The snapshot already applies the override chain, so the field-level
// value is authoritative.
func fieldSetValue(rf domain.ResolvedField) string {
	return rf.SetValue
}

// wantsLabelClause returns true when the terminal value is a resource
// for which rdfs:label is meaningful. Date / Integer literals get
// nothing; resource terminals get an OPTIONAL clause that callers can
// use to surface a human-readable label.
func wantsLabelClause(pathNodes []generators.PathNode, expectedValueType string) bool {
	if len(pathNodes) == 0 {
		return false
	}
	last := pathNodes[len(pathNodes)-1]
	if last.Role == generators.PathRoleLiteral {
		return false
	}
	switch strings.ToLower(expectedValueType) {
	case "date", "integer", "string", "decimal", "boolean":
		return false
	}
	return true
}
