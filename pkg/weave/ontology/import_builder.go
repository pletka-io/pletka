package ontology

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
	parsedontology "github.com/pletka-io/pletka/pkg/weave/ontology/rdf"
)

type ImportNamespaceResolver struct {
	bindings []NamespaceBinding
}

type ImportVersionBuildOptions struct {
	VersionID              string
	VersionString          string
	CompatibleBaseVersions []string
	SetActive              bool
	ParsedAt               time.Time
	// CompanionSources maps a class/property URI to the companion module
	// file it was parsed from. URIs absent from the map (the version's
	// base file) get an empty SourceModule.
	CompanionSources map[string]string
}

type ImportUploadInput struct {
	OntologyID             string
	Filename               string
	Content                []byte
	VersionString          string
	CompatibleBaseVersions []string
	SetActive              bool
}

type ImportProbeResult struct {
	OntologyID             string                            `json:"ontology_id"`
	OntologyPrefix         string                            `json:"ontology_prefix"`
	VersionID              string                            `json:"version_id"`
	VersionString          string                            `json:"version_string"`
	Filename               string                            `json:"filename"`
	FileSize               int64                             `json:"file_size"`
	Namespace              string                            `json:"namespace,omitempty"`
	OntologyURI            string                            `json:"ontology_uri,omitempty"`
	VersionIRI             string                            `json:"version_iri,omitempty"`
	ClassCount             int                               `json:"class_count"`
	PropertyCount          int                               `json:"property_count"`
	RelationCount          int                               `json:"relation_count"`
	CompatibleBaseVersions []string                          `json:"compatible_base_versions,omitempty"`
	ImportedOntologies     []string                          `json:"imported_ontologies,omitempty"`
	MissingNamespaces      []parsedontology.MissingNamespace `json:"missing_namespaces,omitempty"`
	LabelWarnings          []parsedontology.LabelWarning     `json:"label_warnings,omitempty"`
	Warnings               []string                          `json:"warnings,omitempty"`
	ExistingVersion        bool                              `json:"existing_version"`
	CanImport              bool                              `json:"can_import"`
}

func BuildImportNamespaceResolver(bindings []NamespaceBinding) ImportNamespaceResolver {
	seen := map[string]string{}
	for _, binding := range bindings {
		prefix := strings.TrimSpace(binding.Prefix)
		namespace := strings.TrimSpace(binding.Namespace)
		if prefix == "" || namespace == "" {
			continue
		}
		if _, ok := seen[namespace]; !ok {
			seen[namespace] = prefix
		}
	}
	out := make([]NamespaceBinding, 0, len(seen))
	for namespace, prefix := range seen {
		out = append(out, NamespaceBinding{Prefix: prefix, Namespace: namespace})
	}
	sort.Slice(out, func(i, j int) bool {
		return len(out[i].Namespace) > len(out[j].Namespace)
	})
	return ImportNamespaceResolver{bindings: out}
}

func (r ImportNamespaceResolver) QnameFor(uri string) string {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return ""
	}
	for _, binding := range r.bindings {
		if strings.HasPrefix(uri, binding.Namespace) {
			return binding.Prefix + ":" + strings.TrimPrefix(uri, binding.Namespace)
		}
	}
	return uri
}

func (r ImportNamespaceResolver) PrefixForURI(uri string) string {
	qname := r.QnameFor(uri)
	if qname == uri {
		return ""
	}
	prefix, _, ok := strings.Cut(qname, ":")
	if !ok {
		return ""
	}
	return prefix
}

func BuildImportVersionInput(ont *domain.Ontology, opts ImportVersionBuildOptions, pr *parsedontology.ParseResult, nsResolver ImportNamespaceResolver) ImportVersionInput {
	parsedAt := opts.ParsedAt
	if parsedAt.IsZero() {
		parsedAt = time.Now()
	}
	versionString := strings.TrimSpace(opts.VersionString)
	if versionString == "" && pr.Version != nil {
		versionString = pr.Version.VersionString
	}
	versionID := opts.VersionID
	if versionID == "" {
		versionID = GenerateVersionID(ont.Prefix, versionString)
	}

	versionIn := CreateVersionInput{
		ID:                     versionID,
		OntologyID:             ont.ID,
		VersionString:          versionString,
		IsActive:               false,
		CompatibleBaseVersions: opts.CompatibleBaseVersions,
		ParsedAt:               &parsedAt,
	}
	if pr.Version != nil {
		versionIn.RDFContent = pr.Version.RDFContent
		versionIn.OriginalFilename = pr.Version.OriginalFilename
		versionIn.FileSize = pr.Version.FileSize
		versionIn.FileMD5 = pr.Version.FileMD5
		versionIn.OntologyURI = pr.Version.OntologyURI
		versionIn.VersionIRI = pr.Version.VersionIRI
		versionIn.VersionInfo = pr.Version.VersionInfo
		versionIn.ImportedOntologies = []string(pr.Version.ImportedOntologies)
		versionIn.OntologyLabel = pr.Version.OntologyLabel
		versionIn.OntologyComment = pr.Version.OntologyComment
		versionIn.OntologyMetadata = []byte(pr.Version.OntologyMetadata)
	}

	classes := make([]ImportClass, 0, len(pr.Classes))
	relations := make([]domain.OntologyRelation, 0)
	seenClass := make(map[string]string, len(pr.Classes))
	for _, c := range pr.Classes {
		id, exists := seenClass[c.URI]
		if !exists {
			id = GenerateClassID(versionID, c.URI)
			seenClass[c.URI] = id
			classes = append(classes, ImportClass{
				ID:           id,
				URI:          c.URI,
				Prefix:       normalizedImportPrefix(c.Prefix, c.URI, nsResolver),
				LocalName:    c.LocalName,
				Label:        c.Label,
				Comment:      c.Comment,
				SourceModule: opts.CompanionSources[c.URI],
			})
		}
		relations = appendImportRelations(relations, id, "class", "subclass_of", []string(c.SuperClasses), nsResolver)
		relations = appendImportRelations(relations, id, "class", "equivalent_class", []string(c.EquivalentClasses), nsResolver)
		relations = appendImportRelations(relations, id, "class", "disjoint_class", []string(c.DisjointClasses), nsResolver)
	}

	properties := make([]ImportProperty, 0, len(pr.Properties))
	seenProp := make(map[string]string, len(pr.Properties))
	for _, p := range pr.Properties {
		id, exists := seenProp[p.URI]
		if !exists {
			id = GeneratePropertyID(versionID, p.URI)
			seenProp[p.URI] = id
			properties = append(properties, ImportProperty{
				ID:                  id,
				URI:                 p.URI,
				Prefix:              normalizedImportPrefix(p.Prefix, p.URI, nsResolver),
				LocalName:           p.LocalName,
				Label:               p.Label,
				Comment:             p.Comment,
				PropertyType:        p.PropertyType,
				IsFunctional:        p.IsFunctional,
				IsInverseFunctional: p.IsInverseFunctional,
				IsTransitive:        p.IsTransitive,
				IsSymmetric:         p.IsSymmetric,
				IsAsymmetric:        p.IsAsymmetric,
				IsReflexive:         p.IsReflexive,
				IsIrreflexive:       p.IsIrreflexive,
				InversePropertyURI:  derefString(p.InversePropertyURI),
				SourceModule:        opts.CompanionSources[p.URI],
			})
		}
		relations = appendImportRelations(relations, id, "property", "subproperty_of", []string(p.SuperProperties), nsResolver)
		relations = appendImportRelations(relations, id, "property", "equivalent_property", []string(p.EquivalentProperties), nsResolver)
		relations = appendImportRelations(relations, id, "property", "disjoint_property", []string(p.DisjointProperties), nsResolver)
		relations = appendImportRelations(relations, id, "property", "domain", []string(p.DomainClasses), nsResolver)
		relations = appendImportRelations(relations, id, "property", "range", []string(p.RangeClasses), nsResolver)
		if inv := derefString(p.InversePropertyURI); inv != "" {
			relations = appendImportRelations(relations, id, "property", "inverse_property", []string{inv}, nsResolver)
		}
	}

	return ImportVersionInput{
		Version:           versionIn,
		SetActive:         opts.SetActive,
		NamespaceBindings: importNamespaceBindingsFromDeclarations(ont.ID, pr.DeclaredNamespaces),
		Classes:           classes,
		Properties:        properties,
		Relations:         relations,
	}
}

func importNamespaceBindingsFromDeclarations(ontologyID string, declarations []parsedontology.NamespaceDeclaration) []NamespaceBinding {
	seen := map[string]struct{}{}
	bindings := make([]NamespaceBinding, 0, len(declarations))
	for _, ns := range declarations {
		prefix := strings.TrimSpace(ns.Prefix)
		namespace := strings.TrimSpace(ns.Namespace)
		if prefix == "" || namespace == "" {
			continue
		}
		key := prefix + "\x00" + namespace
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		bindings = append(bindings, NamespaceBinding{
			Prefix:     prefix,
			Namespace:  namespace,
			Weight:     NamespaceBindingWeightRDFDeclared,
			Source:     NamespaceBindingSourceRDFDeclared,
			OntologyID: stringPtr(ontologyID),
		})
	}
	return bindings
}

func (s *Service) ProbeImportVersion(ctx context.Context, in ImportUploadInput) (*ImportProbeResult, ImportVersionInput, error) {
	ont, err := s.GetOntology(ctx, in.OntologyID)
	if err != nil {
		return nil, ImportVersionInput{}, err
	}
	bindings, err := s.store.NamespaceBindings(ctx)
	if err != nil {
		return nil, ImportVersionInput{}, fmt.Errorf("load namespace bindings: %w", err)
	}
	bindings = append(bindings, NamespaceBinding{Prefix: ont.Prefix, Namespace: ont.Namespace})

	parser := NewImportRDFParser(bindings, parsedontology.WithAllowMissingNamespaces())
	pr, err := parser.Parse(bytes.NewReader(in.Content), in.Filename, int64(len(in.Content)))
	if err != nil {
		return nil, ImportVersionInput{}, fmt.Errorf("parse RDF: %w", err)
	}
	if pr.Ontology != nil && pr.Ontology.Prefix != "" && pr.Ontology.Namespace != "" {
		bindings = append(bindings, NamespaceBinding{Prefix: pr.Ontology.Prefix, Namespace: pr.Ontology.Namespace})
	}
	for _, ns := range pr.DeclaredNamespaces {
		bindings = append(bindings, NamespaceBinding{Prefix: ns.Prefix, Namespace: ns.Namespace})
	}
	nsResolver := BuildImportNamespaceResolver(bindings)
	versionString := strings.TrimSpace(in.VersionString)
	if versionString == "" && pr.Version != nil {
		versionString = pr.Version.VersionString
	}
	input := BuildImportVersionInput(ont, ImportVersionBuildOptions{
		VersionString:          versionString,
		CompatibleBaseVersions: in.CompatibleBaseVersions,
		SetActive:              in.SetActive,
	}, pr, nsResolver)
	existing := false
	if current, err := s.store.GetVersion(ctx, input.Version.ID); err == nil && current != nil {
		existing = true
	} else if err != nil {
		return nil, ImportVersionInput{}, err
	}
	warnings := make([]string, 0, 2)
	if existing {
		warnings = append(warnings, "A version with this ID already exists and will be replaced.")
	}
	if len(pr.LabelWarnings) > 0 {
		warnings = append(warnings, fmt.Sprintf("%d classes/properties are missing labels.", len(pr.LabelWarnings)))
	}
	if len(pr.MissingNamespaces) > 0 {
		warnings = append(warnings, formatMissingNamespaceWarning(pr.MissingNamespaces))
	}
	unresolvedPrefixes := unresolvedImportPrefixes(input)
	if len(unresolvedPrefixes) > 0 {
		warnings = append(warnings, fmt.Sprintf("%d classes/properties still have no namespace prefix.", len(unresolvedPrefixes)))
	}
	return &ImportProbeResult{
		OntologyID:             ont.ID,
		OntologyPrefix:         ont.Prefix,
		VersionID:              input.Version.ID,
		VersionString:          input.Version.VersionString,
		Filename:               in.Filename,
		FileSize:               int64(len(in.Content)),
		Namespace:              ont.Namespace,
		OntologyURI:            input.Version.OntologyURI,
		VersionIRI:             input.Version.VersionIRI,
		ClassCount:             len(input.Classes),
		PropertyCount:          len(input.Properties),
		RelationCount:          len(input.Relations),
		CompatibleBaseVersions: input.Version.CompatibleBaseVersions,
		ImportedOntologies:     input.Version.ImportedOntologies,
		MissingNamespaces:      pr.MissingNamespaces,
		LabelWarnings:          pr.LabelWarnings,
		Warnings:               warnings,
		ExistingVersion:        existing,
		CanImport:              len(input.Classes)+len(input.Properties) > 0 && input.Version.VersionString != "" && len(pr.MissingNamespaces) == 0 && len(unresolvedPrefixes) == 0,
	}, input, nil
}

func formatMissingNamespaceWarning(missing []parsedontology.MissingNamespace) string {
	if len(missing) == 0 {
		return ""
	}
	parts := make([]string, 0, len(missing))
	for _, item := range missing {
		if item.Namespace == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s (%d)", item.Namespace, item.Count))
	}
	if len(parts) == 0 {
		return "RDF file references namespaces without prefix bindings."
	}
	return "RDF file references namespaces without prefix bindings: " + strings.Join(parts, ", ")
}

func unresolvedImportPrefixes(input ImportVersionInput) []string {
	var unresolved []string
	for _, class := range input.Classes {
		if strings.TrimSpace(class.Prefix) == "" {
			unresolved = append(unresolved, "class "+class.URI)
		}
	}
	for _, prop := range input.Properties {
		if strings.TrimSpace(prop.Prefix) == "" {
			unresolved = append(unresolved, "property "+prop.URI)
		}
	}
	sort.Strings(unresolved)
	return unresolved
}

func appendImportRelations(into []domain.OntologyRelation, sourceID, sourceKind, relType string, uris []string, nsResolver ImportNamespaceResolver) []domain.OntologyRelation {
	for i, uri := range uris {
		uri = strings.TrimSpace(uri)
		if uri == "" {
			continue
		}
		into = append(into, domain.OntologyRelation{
			SourceID:    sourceID,
			SourceKind:  sourceKind,
			RelType:     relType,
			TargetQname: nsResolver.QnameFor(uri),
			Position:    i,
		})
	}
	return into
}

func normalizedImportPrefix(prefix, uri string, nsResolver ImportNamespaceResolver) string {
	prefix = strings.TrimSpace(prefix)
	if prefix != "" {
		return prefix
	}
	return nsResolver.PrefixForURI(uri)
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
