package category

// Adapter methods that opt this slice's typed errors into
// apierror.FromError mapping. ErrEntityInUse stays out — its handler
// emission carries (entity_type, entity_id, semantic_id) the frontend
// reads to link to the still-referencing entity.
//
// See pkg/weave/apierror.

func (e *ErrValidation) ValidationFields() map[string][]string { return e.Fields }
func (e *ErrForbidden) IsForbidden() bool                      { return true }
