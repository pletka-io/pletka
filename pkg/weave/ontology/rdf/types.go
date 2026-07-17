package rdf

import (
	"encoding/json"
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
)

// Ontology is parser-level ontology metadata extracted from RDF.
type Ontology struct {
	ID          string
	Name        string
	Prefix      string
	Namespace   string
	Description domain.Translations
}

// OntologyVersion is parser-level version metadata extracted from RDF.
type OntologyVersion struct {
	ID                     string
	OntologyID             string
	VersionString          string
	IsActive               bool
	CompatibleBaseVersions []string
	RDFContent             string
	ParsedAt               time.Time
	OriginalFilename       string
	FileSize               int64
	FileMD5                string
	OntologyURI            string
	VersionIRI             string
	VersionInfo            domain.Translations
	ImportedOntologies     []string
	OntologyLabel          domain.Translations
	OntologyComment        domain.Translations
	OntologyMetadata       json.RawMessage
	ChangesFromPrevious    json.RawMessage
	ClassCount             int
	PropertyCount          int
}

// OntologyClass is parser-level class data extracted from RDF.
type OntologyClass struct {
	ID                string
	OntologyVersionID string
	URI               string
	LocalName         string
	Prefix            string
	Label             domain.Translations
	Comment           domain.Translations
	SuperClasses      []string
	SubClasses        []string
	EquivalentClasses []string
	DisjointClasses   []string
	OriginalData      json.RawMessage
}

// OntologyProperty is parser-level property data extracted from RDF.
type OntologyProperty struct {
	ID                   string
	OntologyVersionID    string
	URI                  string
	LocalName            string
	Prefix               string
	Label                domain.Translations
	Comment              domain.Translations
	DomainClasses        []string
	RangeClasses         []string
	PropertyType         string
	SuperProperties      []string
	SubProperties        []string
	InversePropertyURI   *string
	IsFunctional         bool
	IsInverseFunctional  bool
	IsTransitive         bool
	IsSymmetric          bool
	IsAsymmetric         bool
	IsReflexive          bool
	IsIrreflexive        bool
	EquivalentProperties []string
	DisjointProperties   []string
	OriginalData         json.RawMessage
}
