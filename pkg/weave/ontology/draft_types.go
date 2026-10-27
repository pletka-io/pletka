package ontology

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

// DraftID is an editor workspace identity, never a released version ID.
type DraftID string

// DraftReference binds one editor workspace to its ontology owner.
type DraftReference struct {
	owner ontologybody.OntologyID
	id    DraftID
}

// NewDraftReference rejects missing identities before binding a capability.
func NewDraftReference(owner ontologybody.OntologyID, id DraftID) (DraftReference, error) {
	if strings.TrimSpace(string(owner)) == "" || strings.TrimSpace(string(id)) == "" {
		return DraftReference{}, draftInvalid("draft", "ontology and draft identities are required")
	}
	return DraftReference{owner: owner, id: id}, nil
}

// DraftSource retains an editable source file; derived term rows are not shared with releases.
type DraftSource struct {
	Filename    string `json:"filename"`
	Content     string `json:"content"`
	Description string `json:"description,omitempty"`
}

// DraftOntology is the catalog descriptor to freeze at publication.
type DraftOntology struct {
	Prefix            string              `json:"prefix"`
	Namespace         string              `json:"namespace"`
	Name              string              `json:"name"`
	Description       domain.Translations `json:"description,omitempty"`
	FamilyID          *string             `json:"family_id,omitempty"`
	Type              domain.OntologyType `json:"ontology_type"`
	ExtendsOntologyID *string             `json:"extends_ontology_id,omitempty"`
	HomepageURL       string              `json:"homepage_url,omitempty"`
	SourceURL         string              `json:"source_url,omitempty"`
}

// DraftNamespace records source interpretation inputs without a mutable namespace reader.
type DraftNamespace struct {
	Prefix     string  `json:"prefix"`
	Namespace  string  `json:"namespace"`
	Weight     int64   `json:"weight,omitempty"`
	Source     string  `json:"source,omitempty"`
	OntologyID *string `json:"ontology_id,omitempty"`
}

// DraftDocument format 1 keeps RDF/XML sources authoritative. Invalid RDF can be
// saved while editing; publication parses and validates all derived content.
type DraftDocument struct {
	FormatVersion          int                     `json:"format_version"`
	Ontology               DraftOntology           `json:"ontology"`
	Primary                DraftSource             `json:"primary"`
	Companions             []DraftSource           `json:"companions,omitempty"`
	Bindings               []DraftNamespace        `json:"namespace_bindings,omitempty"`
	CompatibleBaseVersions []string                `json:"compatible_base_versions,omitempty"`
	Publication            ontologyrelease.Options `json:"publication,omitempty"`
}

// DraftSnapshot is a value that may leave a bounded draft session.
type DraftSnapshot struct {
	OntologyID    ontologybody.OntologyID `json:"ontology_id"`
	ID            DraftID                 `json:"draft_id"`
	BaseVersionID *string                 `json:"base_version_id,omitempty"`
	Revision      int64                   `json:"revision"`
	Document      DraftDocument           `json:"document"`
}

// DraftPublishRequest chooses a new release label and optional active selection.
// Replaying the same request for the same current revision returns its receipt.
type DraftPublishRequest struct {
	Version   string `json:"version"`
	SetActive bool   `json:"set_active"`
}

// DraftPublication identifies the immutable artifact produced from a revision.
type DraftPublication struct {
	OntologyID ontologybody.OntologyID `json:"ontology_id"`
	DraftID    DraftID                 `json:"draft_id"`
	Revision   int64                   `json:"revision"`
	VersionID  string                  `json:"version_id"`
	Digest     string                  `json:"artifact_digest"`
	Created    bool                    `json:"created"`
}

func draftInvalid(field, message string) error {
	return apierror.Validation(map[string][]string{field: {message}})
}
func (d DraftDocument) validate() error {
	if d.FormatVersion != 1 {
		return draftInvalid("format_version", "unsupported ontology draft format")
	}
	if strings.TrimSpace(d.Ontology.Name) == "" || strings.TrimSpace(d.Ontology.Prefix) == "" || strings.TrimSpace(d.Ontology.Namespace) == "" {
		return draftInvalid("ontology", "name, prefix and namespace are required")
	}
	if d.Ontology.Type != domain.OntologyTypeBase && d.Ontology.Type != domain.OntologyTypeExtension {
		return draftInvalid("ontology", "ontology_type must be base or extension")
	}
	if d.Ontology.Type == domain.OntologyTypeExtension && (d.Ontology.ExtendsOntologyID == nil || strings.TrimSpace(*d.Ontology.ExtendsOntologyID) == "") {
		return draftInvalid("ontology", "extension requires its base ontology")
	}
	seen := map[string]bool{}
	for _, source := range append([]DraftSource{d.Primary}, d.Companions...) {
		name := source.Filename
		if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00") {
			return draftInvalid("sources", "source files require distinct bare filenames")
		}
		if seen[name] {
			return draftInvalid("sources", fmt.Sprintf("duplicate source filename %s", name))
		}
		seen[name] = true
	}
	for _, binding := range d.Bindings {
		if strings.TrimSpace(binding.Prefix) == "" || strings.TrimSpace(binding.Namespace) == "" {
			return draftInvalid("namespace_bindings", "prefix and namespace are required")
		}
	}
	return nil
}

func (r DraftPublishRequest) validate() error {
	if strings.TrimSpace(r.Version) == "" || r.Version != strings.TrimSpace(r.Version) || r.Version == "." || r.Version == ".." || strings.ContainsAny(r.Version, "/\\\x00") {
		return draftInvalid("version", "a concrete release label safe for vendoring is required")
	}
	return nil
}
