package domain

// OriginKind is the canonical provenance state the API exposes for an entity
// or reference in a project-scoped view.
type OriginKind string

const (
	OriginOwn       OriginKind = "own"
	OriginInherited OriginKind = "inherited"
	// OriginAdopted is the "explicit" adoption kind: the current project
	// holds an adoption receipt for the source entity. Per the
	// adopt/adapt rollout decision (Q4), explicit wins over
	// reference-adopted when both apply.
	OriginAdopted OriginKind = "adopted"
	// OriginAdoptedReference is the implicit-by-reference adoption kind:
	// no receipt, but an override row in the current project references
	// the source entity (Q3 reference set). Surfaces in the list rule
	// alongside Owned + Adapted + Adopted (explicit).
	OriginAdoptedReference OriginKind = "adopted_reference"
	OriginForked           OriginKind = "forked"
)

// Origin describes where an item comes from relative to the current
// project/view context. For now most API surfaces use only own vs inherited;
// adopted and forked are reserved for the upcoming reuse work.
type Origin struct {
	Kind               OriginKind `json:"kind"`
	SourceProjectID    string     `json:"source_project_id,omitempty"`
	SourceProjectLabel string     `json:"source_project_label,omitempty"`
	SourceEntityID     string     `json:"source_entity_id,omitempty"`
}

func OwnOrigin() Origin {
	return Origin{Kind: OriginOwn}
}

func InheritedOrigin(sourceProjectID string) Origin {
	if sourceProjectID == "" {
		return OwnOrigin()
	}
	return Origin{
		Kind:            OriginInherited,
		SourceProjectID: sourceProjectID,
	}
}

func AdoptedOrigin(sourceProjectID, sourceEntityID string) Origin {
	if sourceProjectID == "" {
		return OwnOrigin()
	}
	return Origin{
		Kind:            OriginAdopted,
		SourceProjectID: sourceProjectID,
		SourceEntityID:  sourceEntityID,
	}
}

func OriginFromProject(currentProjectID, sourceProjectID string) Origin {
	if sourceProjectID == "" || sourceProjectID == currentProjectID {
		return OwnOrigin()
	}
	return InheritedOrigin(sourceProjectID)
}

func OriginFromSemanticID(currentProjectID, semanticOrEntityID string) Origin {
	sid := ParseSemanticID(semanticOrEntityID)
	if !sid.Valid() {
		return OwnOrigin()
	}
	return OriginFromProject(currentProjectID, sid.ProjectID)
}
