package rdf

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
	nsutil "github.com/pletka-io/pletka/pkg/namespace"
)

// RDFParser handles parsing of RDF/XML files
type RDFParser struct {
	namespaces             nsutil.Manager
	allowMissingNamespaces bool
}

type ParserOption func(*RDFParser)

func WithNamespaceManager(mgr nsutil.Manager) ParserOption {
	return func(p *RDFParser) {
		if mgr != nil {
			p.namespaces = mgr
		}
	}
}

// WithAllowMissingNamespaces keeps legacy/reporting behaviour: parsing returns
// a ParseResult with MissingNamespaces populated instead of failing.
func WithAllowMissingNamespaces() ParserOption {
	return func(p *RDFParser) {
		p.allowMissingNamespaces = true
	}
}

// NewRDFParser creates a new RDF parser instance
func NewRDFParser(opts ...ParserOption) *RDFParser {
	p := &RDFParser{
		namespaces: nsutil.NewDefaultManager(),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// ParseResult contains the results of parsing an RDF file
type ParseResult struct {
	Ontology           *Ontology
	Version            *OntologyVersion
	Classes            []*OntologyClass
	Properties         []*OntologyProperty
	DeclaredNamespaces []NamespaceDeclaration
	MissingNamespaces  []MissingNamespace
	LabelWarnings      []LabelWarning
}

// NamespaceDeclaration is one prefix binding declared by the RDF/XML source.
type NamespaceDeclaration struct {
	Prefix    string `json:"prefix"`
	Namespace string `json:"namespace"`
}

// LabelWarning tracks classes or properties missing rdfs:label
type LabelWarning struct {
	Type      string `json:"type"` // "class" or "property"
	URI       string `json:"uri"`
	LocalName string `json:"local_name"`
}

// MissingNamespace tracks namespaces encountered without a declared prefix.
type MissingNamespace struct {
	Namespace string `json:"namespace"`
	Count     int    `json:"count"`
}

// ParseFile parses an RDF/XML file and returns structured data
func (p *RDFParser) ParseFile(filename string) (*ParseResult, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Get file info for metadata
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	return p.Parse(file, fileInfo.Name(), fileInfo.Size())
}

// Parse parses RDF/XML from a reader
func (p *RDFParser) Parse(reader io.Reader, filename string, fileSize int64) (*ParseResult, error) {
	raw, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read RDF source: %w", err)
	}

	declaredNamespaces, err := p.parseDeclaredNamespaces(raw)
	if err != nil {
		return nil, err
	}

	// Read and parse XML
	decoder := xml.NewDecoder(bytes.NewReader(raw))

	var rdfRoot RDFRoot
	if err := decoder.Decode(&rdfRoot); err != nil {
		return nil, fmt.Errorf("failed to parse XML: %w", err)
	}

	// Extract ontology information
	ontologyInfo, versionInfo := p.extractOntologyInfo(&rdfRoot, filename, fileSize)
	versionInfo.RDFContent = string(raw)
	sum := md5.Sum(raw)
	versionInfo.FileMD5 = hex.EncodeToString(sum[:])

	// Extract classes
	classes := p.extractClasses(&rdfRoot, versionInfo.ID)

	// Extract properties
	properties := p.extractProperties(&rdfRoot, versionInfo.ID)

	// Validate that all classes and properties have rdfs:label
	labelWarnings := p.validateLabels(classes, properties)

	// If namespace is still empty (rdfs files without xml:base or owl:Ontology),
	// infer it from the most common class URI namespace.
	if ontologyInfo.Namespace == "" && len(classes) > 0 {
		nsCounts := make(map[string]int)
		for _, c := range classes {
			base, _ := nsutil.SplitURI(c.URI)
			if base != "" {
				nsCounts[base]++
			}
		}
		var bestNS string
		var bestCount int
		for ns, count := range nsCounts {
			if count > bestCount {
				bestNS = ns
				bestCount = count
			}
		}
		if bestNS != "" {
			ontologyInfo.Namespace = bestNS
		}
	}

	// Resolve class/property prefixes using the caller-supplied namespace
	// manager plus any namespace declarations found in the RDF source.
	mgr := p.namespaces
	if mgr == nil {
		mgr = nsutil.NewDefaultManager()
	}
	if ontologyInfo.Namespace != "" && ontologyInfo.Prefix != "" {
		mgr.Put(ontologyInfo.Prefix, ontologyInfo.Namespace, 100)
	}
	if ontologyInfo.Namespace != "" && ontologyInfo.Prefix == "" {
		if ns, err := mgr.GetWithBase(ontologyInfo.Namespace); err == nil {
			ontologyInfo.Prefix = ns.Prefix
		}
	}

	missingNamespaces := ResolvePrefixes(classes, properties, mgr)
	if len(missingNamespaces) > 0 && !p.allowMissingNamespaces {
		return nil, &NamespaceResolutionError{MissingNamespaces: missingNamespaces}
	}

	// Update statistics
	versionInfo.ClassCount = len(classes)
	versionInfo.PropertyCount = len(properties)

	return &ParseResult{
		Ontology:           ontologyInfo,
		Version:            versionInfo,
		Classes:            classes,
		Properties:         properties,
		DeclaredNamespaces: declaredNamespaces,
		MissingNamespaces:  missingNamespaces,
		LabelWarnings:      labelWarnings,
	}, nil
}

var namespaceDeclarationPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\sxmlns:([A-Za-z_][A-Za-z0-9_.-]*)\s*=\s*"([^"]*)"`),
	regexp.MustCompile(`\sxmlns:([A-Za-z_][A-Za-z0-9_.-]*)\s*=\s*'([^']*)'`),
}

// NamespaceResolutionError reports unresolved namespaces found while parsing.
type NamespaceResolutionError struct {
	MissingNamespaces []MissingNamespace
}

func (e *NamespaceResolutionError) Error() string {
	if e == nil || len(e.MissingNamespaces) == 0 {
		return "rdf namespace resolution failed"
	}
	parts := make([]string, 0, len(e.MissingNamespaces))
	for _, item := range e.MissingNamespaces {
		if item.Namespace == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s (%d)", item.Namespace, item.Count))
	}
	if len(parts) == 0 {
		return "rdf namespace resolution failed"
	}
	return "rdf namespace resolution failed: missing prefix binding for " + strings.Join(parts, ", ")
}

func (p *RDFParser) parseDeclaredNamespaces(raw []byte) ([]NamespaceDeclaration, error) {
	source := string(raw)
	seen := map[string]string{}
	var out []NamespaceDeclaration

	for _, pattern := range namespaceDeclarationPatterns {
		for _, match := range pattern.FindAllStringSubmatch(source, -1) {
			prefix := strings.TrimSpace(match[1])
			namespace := strings.TrimSpace(match[2])
			if prefix == "" || namespace == "" || seen[prefix] == namespace {
				continue
			}
			seen[prefix] = namespace
			out = append(out, NamespaceDeclaration{Prefix: prefix, Namespace: namespace})

			if p.namespaces != nil {
				if existing, err := p.namespaces.GetWithPrefix(prefix); err == nil {
					if existing.URI != namespace {
						return nil, fmt.Errorf("rdf namespace declaration conflict: prefix %q maps to %q, manager already maps it to %q", prefix, namespace, existing.URI)
					}
					continue
				}
				if _, err := p.namespaces.Put(prefix, namespace, 80); err != nil {
					return nil, fmt.Errorf("rdf namespace declaration %s=%q: %w", prefix, namespace, err)
				}
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Prefix < out[j].Prefix
	})
	return out, nil
}

// ResolvePrefixes assigns prefixes to classes and properties using the provided NamespaceManager.
// It returns a list of namespaces that could not be resolved to a prefix.
func ResolvePrefixes(classes []*OntologyClass, properties []*OntologyProperty, mgr nsutil.Manager) []MissingNamespace {
	missingCounts := make(map[string]int)

	for _, c := range classes {
		if c.Prefix == "" {
			base, _ := nsutil.SplitURI(c.URI)
			if base != "" {
				if ns, err := mgr.GetWithBase(base); err == nil {
					c.Prefix = ns.Prefix
					continue
				}
				missingCounts[base]++
			} else {
				missingCounts["(no namespace)"]++
			}
		}
	}

	for _, prop := range properties {
		if prop.Prefix == "" {
			base, _ := nsutil.SplitURI(prop.URI)
			if base != "" {
				if ns, err := mgr.GetWithBase(base); err == nil {
					prop.Prefix = ns.Prefix
					continue
				}
				missingCounts[base]++
			} else {
				missingCounts["(no namespace)"]++
			}
		}
	}

	var missing []MissingNamespace
	for ns, count := range missingCounts {
		missing = append(missing, MissingNamespace{Namespace: ns, Count: count})
	}
	sort.Slice(missing, func(i, j int) bool {
		return missing[i].Namespace < missing[j].Namespace
	})
	return missing
}

// extractOntologyInfo extracts core ontology and version information
func (p *RDFParser) extractOntologyInfo(rdf *RDFRoot, filename string, fileSize int64) (*Ontology, *OntologyVersion) {
	// Extract ontology metadata
	ontologyData := rdf.Ontology

	// Determine prefix and namespace from XML base or about attributes
	namespace := rdf.XMLBase
	if namespace == "" && ontologyData != nil {
		namespace = ontologyData.About
	}

	// Prefix is intentionally left empty unless provided elsewhere (no heuristics)
	prefix := ""

	// Create ontology record
	ont := &Ontology{
		Name:      p.extractName(ontologyData, filename),
		Prefix:    prefix,
		Namespace: namespace,
	}

	// Extract ontology description if available
	if ontologyData != nil && len(ontologyData.Comments) > 0 {
		ont.Description = domain.Translations(p.extractMultilingualText(ontologyData.Comments))
	}

	// Extract version information
	now := time.Now()
	version := &OntologyVersion{
		VersionString:    p.extractVersion(ontologyData, filename),
		OriginalFilename: filepath.Base(filename),
		FileSize:         fileSize,
		ParsedAt:         now,
		IsActive:         false, // Will be set by CLI flags or web form
	}

	// Extract multilingual version info and metadata if available
	if ontologyData != nil {
		if ontologyData.VersionInfo != "" {
			versionInfoMap := map[string]string{
				"en": ontologyData.VersionInfo,
			}
			version.VersionInfo = domain.Translations(versionInfoMap)
		}

		version.OntologyURI = ontologyData.About
		version.VersionIRI = ontologyData.VersionIRI

		// Extract ontology-level label and comment
		if len(ontologyData.Labels) > 0 {
			version.OntologyLabel = domain.Translations(p.extractMultilingualText(ontologyData.Labels))
		}
		if len(ontologyData.Comments) > 0 {
			version.OntologyComment = domain.Translations(p.extractMultilingualText(ontologyData.Comments))
		}

		// Extract additional metadata (Dublin Core etc.)
		metadata := make(map[string]interface{})
		if len(ontologyData.Creator) > 0 {
			metadata["dc:creator"] = ontologyData.Creator
		}
		if len(ontologyData.Contributor) > 0 {
			metadata["dc:contributor"] = ontologyData.Contributor
		}
		if ontologyData.Date != "" {
			metadata["dc:date"] = ontologyData.Date
		}
		if ontologyData.Rights != "" {
			metadata["dc:rights"] = ontologyData.Rights
		}
		if len(ontologyData.Title) > 0 {
			metadata["dc:title"] = p.extractMultilingualText(ontologyData.Title)
		}
		if len(ontologyData.Description) > 0 {
			metadata["dc:description"] = p.extractMultilingualText(ontologyData.Description)
		}
		if ontologyData.PriorVersion != "" {
			metadata["owl:priorVersion"] = ontologyData.PriorVersion
		}
		if len(ontologyData.SeeAlso) > 0 {
			metadata["rdfs:seeAlso"] = p.extractURIList(ontologyData.SeeAlso)
		}

		if len(metadata) > 0 {
			metadataJSON, err := json.Marshal(metadata)
			if err == nil {
				version.OntologyMetadata = json.RawMessage(metadataJSON)
			}
		}

		// Extract imported ontologies
		if len(ontologyData.Imports) > 0 {
			version.ImportedOntologies = []string(p.extractURIList(ontologyData.Imports))
		}
	}

	return ont, version
}

// extractClasses extracts class definitions from RDF
func (p *RDFParser) extractClasses(rdf *RDFRoot, versionID string) []*OntologyClass {
	var classes []*OntologyClass

	// Get base URI for resolving relative URIs
	baseURI := rdf.XMLBase
	if baseURI == "" && rdf.Ontology != nil {
		baseURI = rdf.Ontology.About
	}

	for _, rdfClass := range rdf.Classes {
		// Resolve URI: rdf:about takes priority, then rdf:ID
		uri := p.resolveURI(rdfClass.ResolvedAbout(baseURI), baseURI)

		class := &OntologyClass{
			OntologyVersionID: versionID,
			URI:               uri,
			LocalName:         p.extractLocalName(uri),
			Prefix:            "",
			Label:             domain.Translations(p.extractMultilingualText(rdfClass.Labels)),
			Comment:           domain.Translations(p.extractMultilingualText(rdfClass.Comments)),
			SuperClasses:      []string(p.resolveURIList(rdfClass.SubClassOf, baseURI)),
		}

		classes = append(classes, class)
	}

	// Extract classes from rdf:Description elements (Gap 2 pattern)
	for _, desc := range rdf.Descriptions {
		if desc.Type == nil {
			continue
		}
		typeURI := desc.Type.Resource
		if !isClassType(typeURI) {
			continue
		}
		uri := p.resolveURI(desc.About, baseURI)
		if uri == "" {
			continue
		}

		class := &OntologyClass{
			OntologyVersionID: versionID,
			URI:               uri,
			LocalName:         p.extractLocalName(uri),
			Prefix:            "",
			Label:             domain.Translations(p.extractMultilingualText(desc.Labels)),
			Comment:           domain.Translations(p.extractMultilingualText(desc.Comments)),
			SuperClasses:      []string(p.resolveURIList(desc.SubClassOf, baseURI)),
		}

		classes = append(classes, class)
	}

	return classes
}

// isClassType checks if a rdf:type URI indicates a class declaration
func isClassType(typeURI string) bool {
	return typeURI == "http://www.w3.org/2000/01/rdf-schema#Class" ||
		typeURI == "http://www.w3.org/2002/07/owl#Class"
}

// extractProperties extracts property definitions from RDF
func (p *RDFParser) extractProperties(rdf *RDFRoot, versionID string) []*OntologyProperty {
	var properties []*OntologyProperty

	// Get base URI for resolving relative URIs
	baseURI := rdf.XMLBase
	if baseURI == "" && rdf.Ontology != nil {
		baseURI = rdf.Ontology.About
	}

	// Helper to convert an RDFProperty to OntologyProperty
	convertProp := func(rdfProp *RDFProperty, propType string) *OntologyProperty {
		// Resolve URI: rdf:about takes priority, then rdf:ID
		uri := p.resolveURI(rdfProp.ResolvedAbout(baseURI), baseURI)

		if propType == "" {
			propType = p.determinePropertyType(rdfProp)
		}

		prop := &OntologyProperty{
			OntologyVersionID: versionID,
			URI:               uri,
			LocalName:         p.extractLocalName(uri),
			Prefix:            "",
			Label:             domain.Translations(p.extractMultilingualText(rdfProp.Labels)),
			Comment:           domain.Translations(p.extractMultilingualText(rdfProp.Comments)),
			DomainClasses:     []string(p.resolveURIList(rdfProp.Domain, baseURI)),
			RangeClasses:      []string(p.resolveURIList(rdfProp.Range, baseURI)),
			PropertyType:      propType,
			SuperProperties:   []string(p.resolveURIList(rdfProp.SubPropertyOf, baseURI)),
		}

		// Extract inverse property if available
		if rdfProp.InverseOf != nil {
			inverseURI := p.resolveURI(rdfProp.InverseOf.Resource, baseURI)
			prop.InversePropertyURI = &inverseURI
		}

		return prop
	}

	// 1. RDFS-style: <rdf:Property>
	for i := range rdf.Properties {
		properties = append(properties, convertProp(&rdf.Properties[i], ""))
	}

	// 2. OWL-style: <owl:ObjectProperty>
	for i := range rdf.ObjectProperties {
		properties = append(properties, convertProp(&rdf.ObjectProperties[i], "ObjectProperty"))
	}

	// 3. OWL-style: <owl:DatatypeProperty>
	for i := range rdf.DatatypeProperties {
		properties = append(properties, convertProp(&rdf.DatatypeProperties[i], "DatatypeProperty"))
	}

	// 4. rdf:Description pattern (Gap 2)
	for _, desc := range rdf.Descriptions {
		if desc.Type == nil {
			continue
		}
		propType := descriptionPropertyType(desc.Type.Resource)
		if propType == "" {
			continue
		}
		uri := p.resolveURI(desc.About, baseURI)
		if uri == "" {
			continue
		}

		prop := &OntologyProperty{
			OntologyVersionID: versionID,
			URI:               uri,
			LocalName:         p.extractLocalName(uri),
			Prefix:            "",
			Label:             domain.Translations(p.extractMultilingualText(desc.Labels)),
			Comment:           domain.Translations(p.extractMultilingualText(desc.Comments)),
			DomainClasses:     []string(p.resolveURIList(desc.Domain, baseURI)),
			RangeClasses:      []string(p.resolveURIList(desc.Range, baseURI)),
			PropertyType:      propType,
			SuperProperties:   []string(p.resolveURIList(desc.SubPropertyOf, baseURI)),
		}

		if desc.InverseOf != nil {
			inverseURI := p.resolveURI(desc.InverseOf.Resource, baseURI)
			prop.InversePropertyURI = &inverseURI
		}

		properties = append(properties, prop)
	}

	return properties
}

// validateLabels checks that all classes and properties have at least one rdfs:label
func (p *RDFParser) validateLabels(classes []*OntologyClass, properties []*OntologyProperty) []LabelWarning {
	var warnings []LabelWarning

	// Base OWL/RDFS classes that should be excluded from validation
	isBaseClass := func(uri string) bool {
		return uri == "http://www.w3.org/2000/01/rdf-schema#Class" ||
			uri == "http://www.w3.org/2000/01/rdf-schema#Resource" ||
			uri == "http://www.w3.org/2000/01/rdf-schema#Literal" ||
			uri == "http://www.w3.org/2000/01/rdf-schema#Container" ||
			uri == "http://www.w3.org/2000/01/rdf-schema#ContainerMembershipProperty" ||
			uri == "http://www.w3.org/2000/01/rdf-schema#Datatype" ||
			uri == "http://www.w3.org/2002/07/owl#Thing" ||
			uri == "http://www.w3.org/2002/07/owl#Class" ||
			uri == "http://www.w3.org/2002/07/owl#ObjectProperty" ||
			uri == "http://www.w3.org/2002/07/owl#DatatypeProperty" ||
			uri == "http://www.w3.org/2002/07/owl#AnnotationProperty" ||
			uri == "http://www.w3.org/2002/07/owl#Ontology" ||
			uri == "http://www.w3.org/2002/07/owl#TransitiveProperty" ||
			uri == "http://www.w3.org/2002/07/owl#SymmetricProperty" ||
			uri == "http://www.w3.org/2002/07/owl#FunctionalProperty" ||
			uri == "http://www.w3.org/2002/07/owl#InverseFunctionalProperty" ||
			uri == "http://www.w3.org/2002/07/owl#ReflexiveProperty" ||
			uri == "http://www.w3.org/2002/07/owl#IrreflexiveProperty" ||
			uri == "http://www.opengis.net/ont/geosparql#Feature" ||
			uri == "http://www.opengis.net/ont/geosparql#Geometry" ||
			uri == "http://www.opengis.net/ont/geosparql#SpatialObject" ||
			uri == "http://www.opengis.net/ont/geosparql#FeatureProperty" ||
			uri == "http://www.opengis.net/ont/geosparql#GeometryProperty"
	}

	for _, c := range classes {
		if c.Label == nil || len(c.Label) == 0 {
			// Skip OWL/RDFS base classes
			if isBaseClass(c.URI) {
				continue
			}
			warnings = append(warnings, LabelWarning{
				Type:      "class",
				URI:       c.URI,
				LocalName: c.LocalName,
			})
		}
	}

	for _, prop := range properties {
		if prop.Label == nil || len(prop.Label) == 0 {
			warnings = append(warnings, LabelWarning{
				Type:      "property",
				URI:       prop.URI,
				LocalName: prop.LocalName,
			})
		}
	}

	return warnings
}

// descriptionPropertyType returns the property type if the rdf:type URI indicates a property, or "" otherwise
func descriptionPropertyType(typeURI string) string {
	switch typeURI {
	case "http://www.w3.org/1999/02/22-rdf-syntax-ns#Property":
		return "ObjectProperty"
	case "http://www.w3.org/2002/07/owl#ObjectProperty":
		return "ObjectProperty"
	case "http://www.w3.org/2002/07/owl#DatatypeProperty":
		return "DatatypeProperty"
	case "http://www.w3.org/2002/07/owl#AnnotationProperty":
		return "AnnotationProperty"
	default:
		return ""
	}
}

// Helper methods for extraction

func (p *RDFParser) extractName(ont *RDFOntology, filename string) string {
	// Priority 1: Use ontology label if available
	if ont != nil && len(ont.Labels) > 0 {
		for _, label := range ont.Labels {
			if label.Text != "" {
				return label.Text
			}
		}
	}

	// Priority 2: Use title from metadata
	if ont != nil && len(ont.Title) > 0 {
		for _, title := range ont.Title {
			if title.Text != "" {
				return title.Text
			}
		}
	}

	// Priority 3: Extract from filename
	// Remove path, extension, and version suffix
	base := filepath.Base(filename)
	base = strings.TrimSuffix(base, filepath.Ext(base))

	// Remove version pattern (e.g., "_v7.1.3", "_v2", "_2.0")
	versionPatterns := []string{"_v", "_V"}
	for _, pattern := range versionPatterns {
		if idx := strings.LastIndex(base, pattern); idx != -1 {
			base = base[:idx]
		}
	}

	// Convert underscores to spaces for readability
	name := strings.ReplaceAll(base, "_", " ")

	// Don't apply heuristics based on content - use what's in the filename
	return name
}

func (p *RDFParser) extractVersion(ont *RDFOntology, filename string) string {
	if ont != nil && ont.VersionIRI != "" {
		// Extract version from version IRI
		parts := strings.Split(ont.VersionIRI, "/")
		if len(parts) > 0 {
			last := parts[len(parts)-1]
			if strings.TrimSuffix(last, "/") != "" {
				return strings.TrimSuffix(last, "/")
			}
		}
	}

	// Extract from filename (e.g., "CIDOC_CRM_v7.1.3.rdf" -> "7.1.3")
	base := filepath.Base(filename)
	if idx := strings.Index(base, "_v"); idx != -1 {
		version := base[idx+2:]
		version = strings.TrimSuffix(version, filepath.Ext(version))
		return version
	}

	return "1.0"
}

func (p *RDFParser) extractLocalName(uri string) string {
	// Extract local name from URI (everything after # or last /)
	if idx := strings.LastIndex(uri, "#"); idx != -1 {
		return uri[idx+1:]
	}
	if idx := strings.LastIndex(uri, "/"); idx != -1 {
		return uri[idx+1:]
	}
	return uri
}

func (p *RDFParser) extractMultilingualText(items []MultilingualText) map[string]string {
	result := make(map[string]string)
	for _, item := range items {
		lang := item.Lang
		if lang == "" {
			lang = "en" // Default to English
		}
		result[lang] = item.Text
	}
	return result
}

func (p *RDFParser) extractURIList(refs []Reference) []string {
	var uris []string
	for _, ref := range refs {
		if ref.Resource != "" {
			uris = append(uris, ref.Resource)
		}
	}
	return uris
}

func (p *RDFParser) determinePropertyType(prop *RDFProperty) string {
	// Heuristics to determine property type
	if len(prop.Range) > 0 {
		for _, rangeRef := range prop.Range {
			if strings.Contains(rangeRef.Resource, "Literal") ||
				strings.Contains(rangeRef.Resource, "string") ||
				strings.Contains(rangeRef.Resource, "int") ||
				strings.Contains(rangeRef.Resource, "date") {
				return "DatatypeProperty"
			}
		}
	}

	// Default to object property for ontologies like CIDOC-CRM
	return "ObjectProperty"
}

// resolveURI resolves a relative URI against a base URI
func (p *RDFParser) resolveURI(uri, baseURI string) string {
	// If URI is already absolute (contains ://), return as-is
	if strings.Contains(uri, "://") {
		return uri
	}

	// If URI is empty, return as-is
	if uri == "" {
		return uri
	}

	// If no base URI, return relative URI as-is
	if baseURI == "" {
		return uri
	}

	// Fragment URIs (starting with #) attach directly to the base
	if strings.HasPrefix(uri, "#") {
		return baseURI + uri
	}

	// Ensure base URI ends with / or #
	if !strings.HasSuffix(baseURI, "/") && !strings.HasSuffix(baseURI, "#") {
		baseURI += "/"
	}

	// Resolve relative URI against base
	return baseURI + uri
}

// resolveURIList resolves a list of URIs against a base URI
func (p *RDFParser) resolveURIList(refs []Reference, baseURI string) []string {
	var uris []string
	for _, ref := range refs {
		if ref.Resource != "" {
			uris = append(uris, p.resolveURI(ref.Resource, baseURI))
		}
	}
	return uris
}
