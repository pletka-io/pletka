package ontologyrelease

import (
	"encoding/json"

	"github.com/pletka-io/pletka/pkg/namespace"
)

// ProjectArtifact is a captured artifact plus the project's direct link policy.
type ProjectArtifact struct {
	Artifact Artifact
	Linked   bool
	Primary  bool
}

// Content exposes frozen serialization inputs without a database capability.
// Full body bytes remain in Document for lossless restore of derived terms.
type Content struct {
	Ontology struct {
		ID        string
		Prefix    string
		Namespace string
		Name      string
		Type      string `json:"ontology_type"`
		SourceURL string `json:"source_url"`
	} `json:"ontology"`
	Version struct {
		ID       string
		Version  string   `json:"version_string"`
		RDF      string   `json:"rdf_content"`
		Filename string   `json:"original_filename"`
		Imports  []string `json:"imported_ontologies"`
	} `json:"version"`
	Companions []struct {
		Filename    string
		Description string
		Content     string
		Position    int
	} `json:"companions"`
	Bindings []namespace.Binding `json:"namespace_bindings"`
}

// Content verifies the artifact before exposing its frozen file/descriptor data.
func (a Artifact) Content() (Content, error) {
	var c Content
	doc, err := a.Decode()
	if err != nil {
		return c, err
	}
	err = json.Unmarshal([]byte(doc.BodyJSON), &c)
	return c, err
}
