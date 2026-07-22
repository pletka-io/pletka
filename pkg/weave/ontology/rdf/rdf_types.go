package rdf

import (
	"encoding/xml"
	"strings"
)

// RDF/XML data structures for type-safe parsing

// RDFRoot represents the root RDF element
type RDFRoot struct {
	XMLName    xml.Name      `xml:"RDF"`
	XMLBase    string        `xml:"http://www.w3.org/XML/1998/namespace base,attr"`
	Xmlns      string        `xml:"xmlns,attr"`
	XmlnsRDF   string        `xml:"rdf,attr"`
	XmlnsRDFS  string        `xml:"rdfs,attr"`
	XmlnsOWL   string        `xml:"owl,attr"`
	Ontology   *RDFOntology  `xml:"Ontology"`
	Classes    []RDFClass    `xml:"Class"`
	Properties []RDFProperty `xml:"Property"`

	// rdfs:Datatype typed nodes (xsd datatypes in the defaults ontology).
	// RDFS defines rdfs:Datatype as a subclass of rdfs:Class, so these
	// import as class rows.
	Datatypes []RDFClass `xml:"Datatype"`

	// OWL-style declarations (owl:ObjectProperty, owl:DatatypeProperty)
	ObjectProperties   []RDFProperty `xml:"ObjectProperty"`
	DatatypeProperties []RDFProperty `xml:"DatatypeProperty"`

	// rdf:Description elements (used by some ontologies instead of typed elements)
	Descriptions []RDFDescription `xml:"Description"`
}

// RDFOntology represents owl:Ontology element
type RDFOntology struct {
	About       string             `xml:"about,attr"`
	VersionIRI  string             `xml:"versionIRI"`
	VersionInfo string             `xml:"versionInfo"`
	Labels      []MultilingualText `xml:"label"`
	Comments    []MultilingualText `xml:"comment"`
	Imports     []Reference        `xml:"imports"`

	// Dublin Core metadata
	Creator     []string           `xml:"creator"`
	Contributor []string           `xml:"contributor"`
	Date        string             `xml:"date"`
	Rights      string             `xml:"rights"`
	Title       []MultilingualText `xml:"title"`
	Description []MultilingualText `xml:"description"`

	// Additional metadata
	PriorVersion string      `xml:"priorVersion"`
	SeeAlso      []Reference `xml:"seeAlso"`
}

// RDFClass represents rdfs:Class element
type RDFClass struct {
	About      string             `xml:"http://www.w3.org/1999/02/22-rdf-syntax-ns# about,attr"`
	ID         string             `xml:"http://www.w3.org/1999/02/22-rdf-syntax-ns# ID,attr"`
	Labels     []MultilingualText `xml:"label"`
	Comments   []MultilingualText `xml:"comment"`
	SubClassOf []Reference        `xml:"subClassOf"`

	// OWL constructs
	EquivalentClass []Reference `xml:"equivalentClass"`
	DisjointWith    []Reference `xml:"disjointWith"`
}

// ResolvedAbout returns the URI for this class, resolving rdf:ID against baseURI if needed.
func (c *RDFClass) ResolvedAbout(baseURI string) string {
	if c.About != "" {
		return c.About
	}
	if c.ID != "" {
		base := baseURI
		if base == "" {
			return c.ID
		}
		// rdf:ID is a fragment identifier relative to the base URI
		if strings.HasSuffix(base, "#") || strings.HasSuffix(base, "/") {
			return base + c.ID
		}
		return base + "#" + c.ID
	}
	return ""
}

// RDFProperty represents rdf:Property element
type RDFProperty struct {
	About         string             `xml:"http://www.w3.org/1999/02/22-rdf-syntax-ns# about,attr"`
	ID            string             `xml:"http://www.w3.org/1999/02/22-rdf-syntax-ns# ID,attr"`
	Labels        []MultilingualText `xml:"label"`
	Comments      []MultilingualText `xml:"comment"`
	Domain        []Reference        `xml:"domain"`
	Range         []Reference        `xml:"range"`
	SubPropertyOf []Reference        `xml:"subPropertyOf"`

	// OWL property characteristics
	InverseOf          *Reference  `xml:"inverseOf"`
	EquivalentProperty []Reference `xml:"equivalentProperty"`
	DisjointWith       []Reference `xml:"propertyDisjointWith"`

	// Property characteristics (flags)
	Functional        *EmptyElement `xml:"FunctionalProperty"`
	InverseFunctional *EmptyElement `xml:"InverseFunctionalProperty"`
	Transitive        *EmptyElement `xml:"TransitiveProperty"`
	Symmetric         *EmptyElement `xml:"SymmetricProperty"`
	Asymmetric        *EmptyElement `xml:"AsymmetricProperty"`
	Reflexive         *EmptyElement `xml:"ReflexiveProperty"`
	Irreflexive       *EmptyElement `xml:"IrreflexiveProperty"`

	// SourceType tracks how this property was declared (set during parsing, not from XML)
	SourceType string `xml:"-"`
}

// ResolvedAbout returns the URI for this property, resolving rdf:ID against baseURI if needed.
func (p *RDFProperty) ResolvedAbout(baseURI string) string {
	if p.About != "" {
		return p.About
	}
	if p.ID != "" {
		base := baseURI
		if base == "" {
			return p.ID
		}
		// rdf:ID is a fragment identifier relative to the base URI
		if strings.HasSuffix(base, "#") || strings.HasSuffix(base, "/") {
			return base + p.ID
		}
		return base + "#" + p.ID
	}
	return ""
}

// MultilingualText represents text with xml:lang attribute
type MultilingualText struct {
	Lang string `xml:"lang,attr"`
	Text string `xml:",chardata"`
}

// Reference represents a resource reference (rdf:resource)
type Reference struct {
	Resource string `xml:"resource,attr"`
}

// EmptyElement represents elements that are just markers (no content)
type EmptyElement struct{}

// Additional helper types for complex RDF constructs

// RDFDescription represents rdf:Description elements (for additional statements).
// Some ontologies (CRMarchaeo 2.1.1, CRMtex 2.0, CRMpe 3.1.2) declare classes
// and properties via rdf:Description + rdf:type instead of typed elements.
type RDFDescription struct {
	About         string             `xml:"http://www.w3.org/1999/02/22-rdf-syntax-ns# about,attr"`
	Type          *Reference         `xml:"type"`
	Labels        []MultilingualText `xml:"label"`
	Comments      []MultilingualText `xml:"comment"`
	SubClassOf    []Reference        `xml:"subClassOf"`
	Domain        []Reference        `xml:"domain"`
	Range         []Reference        `xml:"range"`
	SubPropertyOf []Reference        `xml:"subPropertyOf"`
	InverseOf     *Reference         `xml:"inverseOf"`

	// OWL constructs
	EquivalentClass    []Reference `xml:"equivalentClass"`
	DisjointWith       []Reference `xml:"disjointWith"`
	EquivalentProperty []Reference `xml:"equivalentProperty"`
	VersionInfo        string      `xml:"versionInfo"`
}

// OWLClass represents owl:Class elements (more specific than rdfs:Class)
type OWLClass struct {
	RDFClass

	// OWL-specific constructs
	OneOf          []Reference `xml:"oneOf"`
	UnionOf        []Reference `xml:"unionOf"`
	IntersectionOf []Reference `xml:"intersectionOf"`
	ComplementOf   []Reference `xml:"complementOf"`

	// Restrictions
	Restrictions []OWLRestriction `xml:"Restriction"`
}

// OWLRestriction represents owl:Restriction elements
type OWLRestriction struct {
	OnProperty     Reference  `xml:"onProperty"`
	SomeValuesFrom *Reference `xml:"someValuesFrom"`
	AllValuesFrom  *Reference `xml:"allValuesFrom"`
	HasValue       *Reference `xml:"hasValue"`
	MinCardinality *int       `xml:"minCardinality"`
	MaxCardinality *int       `xml:"maxCardinality"`
	Cardinality    *int       `xml:"cardinality"`
}

// Namespaces definitions commonly found in ontologies
type Namespaces struct {
	RDF    string
	RDFS   string
	OWL    string
	XSD    string
	SKOS   string
	CRM    string
	CRMSCI string
}

// StandardNamespaces contains standard namespace URIs
var StandardNamespaces = Namespaces{
	RDF:    "http://www.w3.org/1999/02/22-rdf-syntax-ns#",
	RDFS:   "http://www.w3.org/2000/01/rdf-schema#",
	OWL:    "http://www.w3.org/2002/07/owl#",
	XSD:    "http://www.w3.org/2001/XMLSchema#",
	SKOS:   "http://www.w3.org/2004/02/skos/core#",
	CRM:    "http://www.cidoc-crm.org/cidoc-crm/",
	CRMSCI: "http://www.cidoc-crm.org/extensions/crmsci/",
}
