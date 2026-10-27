package ontology

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	parsedontology "github.com/pletka-io/pletka/pkg/weave/ontology/rdf"
)

const companionDescriptionKey = "description"

func compileOntologyDraft(owner string, document DraftDocument, request DraftPublishRequest) (ImportVersionInput, error) {
	if err := document.validate(); err != nil {
		return ImportVersionInput{}, err
	}
	bindings := []NamespaceBinding{}
	for _, binding := range document.Bindings {
		bindings = append(bindings, NamespaceBinding(binding))
	}
	// Explicit bindings retain their provenance. The default own namespace is
	// added only if the document did not already provide it.
	bindings = MergeNamespaceBindings(bindings, []NamespaceBinding{{Prefix: document.Ontology.Prefix, Namespace: document.Ontology.Namespace, OntologyID: &owner}})
	parser := NewImportRDFParser(bindings)
	primary, err := parseDraftSource(parser, document.Primary)
	if err != nil {
		return ImportVersionInput{}, err
	}
	bindings = MergeNamespaceBindings(bindings, importNamespaceBindingsFromDeclarations(owner, primary.DeclaredNamespaces))
	sources := map[string]string{}
	files := make([]CompanionFileInput, 0, len(document.Companions))
	meta := make([]map[string]any, 0, len(document.Companions))
	for _, source := range document.Companions {
		companion, err := parseDraftSource(NewImportRDFParser(bindings), source)
		if err != nil {
			return ImportVersionInput{}, err
		}
		for _, c := range companion.Classes {
			sources[c.URI] = source.Filename
		}
		for _, p := range companion.Properties {
			sources[p.URI] = source.Filename
		}
		primary.Classes = append(primary.Classes, companion.Classes...)
		primary.Properties = append(primary.Properties, companion.Properties...)
		primary.DeclaredNamespaces = append(primary.DeclaredNamespaces, companion.DeclaredNamespaces...)
		bindings = MergeNamespaceBindings(bindings, importNamespaceBindingsFromDeclarations(owner, companion.DeclaredNamespaces))
		files = append(files, CompanionFileInput{Filename: source.Filename, Description: source.Description, Content: source.Content})
		meta = append(meta, map[string]any{"file": source.Filename, companionDescriptionKey: source.Description, "class_count": len(companion.Classes), "property_count": len(companion.Properties)})
	}
	if len(meta) > 0 && primary.Version != nil {
		primary.Version.OntologyMetadata = mergeCompanionMetadata(primary.Version.OntologyMetadata, meta)
	}
	descriptor := &domain.Ontology{ID: owner, Prefix: document.Ontology.Prefix, Namespace: document.Ontology.Namespace, Name: document.Ontology.Name}
	input := BuildImportVersionInput(descriptor, ImportVersionBuildOptions{VersionID: GenerateVersionID(document.Ontology.Prefix, request.Version), VersionString: request.Version, CompatibleBaseVersions: document.CompatibleBaseVersions, SetActive: request.SetActive, CompanionSources: sources}, primary, BuildImportNamespaceResolver(bindings))
	input.CompanionFiles = files
	input.NamespaceBindings = bindings
	input.Publication = document.Publication
	return input, nil
}

func parseDraftSource(parser *parsedontology.RDFParser, source DraftSource) (*parsedontology.ParseResult, error) {
	// Format 1 supports the existing RDF/XML parser, without temporary files or
	// network resolution. An unrelated XML document must not become an empty ontology.
	decoder := xml.NewDecoder(strings.NewReader(source.Content))
	for {
		token, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = fmt.Errorf("empty RDF source")
			}
			return nil, draftInvalid("sources", source.Filename+": "+err.Error())
		}
		if start, ok := token.(xml.StartElement); ok {
			if start.Name.Space != "http://www.w3.org/1999/02/22-rdf-syntax-ns#" || start.Name.Local != "RDF" {
				return nil, draftInvalid("sources", source.Filename+": expected an RDF/XML document")
			}
			break
		}
	}
	parsed, err := parser.Parse(strings.NewReader(source.Content), source.Filename, int64(len(source.Content)))
	if err != nil {
		return nil, draftInvalid("sources", source.Filename+": "+err.Error())
	}
	return parsed, nil
}
