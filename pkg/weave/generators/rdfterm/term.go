// Package rdfterm holds the lexical rules Turtle and SPARQL share: which
// path elements are datatypes, how a set value becomes an IRI or a quoted
// string, and how class lists are written.
package rdfterm

import (
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
)

// XSDNamespace is the XML Schema datatype namespace.
const XSDNamespace = "http://www.w3.org/2001/XMLSchema#"

// IsDatatype reports an element naming an XSD datatype (or rdf:langString),
// which some stored paths type as a class: the path then ends in a literal.
func IsDatatype(e domain.PathElement) bool {
	return e.Prefix == "xsd" || strings.HasPrefix(e.URI, XSDNamespace) ||
		e.PrefixedName() == "rdf:langString"
}
