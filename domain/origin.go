package domain

// OriginKind is the canonical provenance state exposed by project-scoped views.
type OriginKind string

const (
	OriginOwn       OriginKind = "own"
	OriginInherited OriginKind = "inherited"
	OriginAdopted   OriginKind = "adopted"
	OriginForked    OriginKind = "forked"
)

// Origin describes where an item comes from relative to the current project.
type Origin struct {
	Kind               OriginKind `json:"kind"`
	SourceProjectID    string     `json:"source_project_id,omitempty"`
	SourceProjectLabel string     `json:"source_project_label,omitempty"`
	SourceEntityID     string     `json:"source_entity_id,omitempty"`
}

// AdoptedOrigin returns adopted provenance unless the source project is empty.
func AdoptedOrigin(sourceProjectID, sourceEntityID string) Origin {
	if sourceProjectID == "" {
		return Origin{Kind: OriginOwn}
	}
	return Origin{
		Kind:            OriginAdopted,
		SourceProjectID: sourceProjectID,
		SourceEntityID:  sourceEntityID,
	}
}

// OriginFromProject returns own provenance when the source is empty or the
// current project, otherwise inherited provenance.
func OriginFromProject(currentProjectID, sourceProjectID string) Origin {
	if sourceProjectID == "" || sourceProjectID == currentProjectID {
		return Origin{Kind: OriginOwn}
	}
	return Origin{
		Kind:            OriginInherited,
		SourceProjectID: sourceProjectID,
	}
}
