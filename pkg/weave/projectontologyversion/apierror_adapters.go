package projectontologyversion

// Adapter methods that opt this slice's typed errors into
// apierror.FromError mapping. ErrInUse stays out — its handler
// emission carries slice-specific fields (field_count, field_samples)
// that the frontend reads.
//
// See pkg/weave/apierror.

func (e *ErrValidation) ValidationFields() map[string][]string { return e.Fields }
func (e *ErrForbidden) IsForbidden() bool                      { return true }
