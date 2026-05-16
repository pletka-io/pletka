package weave

import (
	"context"
	"time"
)

// ProjectOntologyVersion records that a project is linked to an ontology
// version. It is the project-level contract materializers need before reading
// the ontology payload itself.
type ProjectOntologyVersion struct {
	ProjectID         string    `json:"project_id"`
	OntologyVersionID string    `json:"ontology_version_id"`
	AddedAt           time.Time `json:"added_at"`
	AddedByID         *string   `json:"added_by_id,omitempty"`
	IsPrimary         bool      `json:"is_primary"`
	UsageNotes        *string   `json:"usage_notes,omitempty"`
	VersionNumber     string    `json:"version_number,omitempty"`
}

// ProjectOntologyVersionStore reads ontology links attached to a project.
type ProjectOntologyVersionStore interface {
	Get(ctx context.Context, projectID, ontologyVersionID string) (*ProjectOntologyVersion, error)
	GetVersion(ctx context.Context, projectID, ontologyVersionID, version string) (*ProjectOntologyVersion, error)
	List(ctx context.Context, projectID string) ([]*ProjectOntologyVersion, error)
	ListVersion(ctx context.Context, projectID, version string) ([]*ProjectOntologyVersion, error)
}
