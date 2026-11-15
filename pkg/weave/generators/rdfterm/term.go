// Package rdfterm holds the lexical rules Turtle and SPARQL share: which
// path elements are datatypes, how a set value becomes an IRI or a quoted
// string, and how class lists are written.
package rdfterm

import (
	"fmt"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
)

// XSDNamespace is the XML Schema datatype namespace.
const XSDNamespace = "http://www.w3.org/2001/XMLSchema#"

// IsDatatype reports an element naming an XSD datatype (or rdf:langString),
// which some stored paths type as a class: the path then ends in a literal.
func IsDatatype(e domain.PathElement) bool {
	return e.Prefix == "xsd" || strings.HasPrefix(e.URI, XSDNamespace) ||
		e.PrefixedName() == "rdf:langString"
}

// Term writes a set value as an IRI when it is one (absolute http(s) or
// urn:, or a CURIE whose prefix the project binds), else as a quoted string.
func Term(v string, ns generators.NamespaceSet) string {
	if strings.ContainsFunc(v, NotIRIChar) {
		return Quote(v)
	}
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "urn:") {
		return "<" + v + ">"
	}
	if prefix, local, ok := strings.Cut(v, ":"); ok {
		if base, bound := ns.ResolvePrefix(prefix); bound {
			return "<" + base + local + ">"
		}
	}
	return Quote(v)
}

// UnboundPrefix reports a value shaped like a CURIE (prefix:local, no
// characters an IRI cannot hold, not an http(s) or urn IRI) whose prefix
// the project does not bind.
func UnboundPrefix(v string, ns generators.NamespaceSet) (string, bool) {
	if v == "" || strings.ContainsFunc(v, NotIRIChar) ||
		strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "urn:") {
		return "", false
	}
	prefix, _, ok := strings.Cut(v, ":")
	if !ok || prefix == "" {
		return "", false
	}
	if _, bound := ns.ResolvePrefix(prefix); bound {
		return "", false
	}
	return prefix, true
}

// NotIRIChar reports a character Turtle and SPARQL do not allow in an IRIREF.
func NotIRIChar(r rune) bool {
	return r <= 0x20 || strings.ContainsRune("<>\"{}|^`\\", r)
}

// Quote writes s as a string literal with ECHAR / UCHAR escapes. Turtle's
// STRING_LITERAL_QUOTE and SPARQL's take the same escapes.
func Quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
				continue
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// ClassList writes the class types of an element as a Turtle object list
// ("crmdig:D1, crm:E36"); types without a prefix or local name are skipped.
func ClassList(e domain.PathElement) string {
	types := e.AllTypes()
	names := make([]string, 0, len(types))
	for _, t := range types {
		if t.Prefix == "" || t.LocalName == "" {
			continue
		}
		names = append(names, t.Prefix+":"+t.LocalName)
	}
	return strings.Join(names, ", ")
}
