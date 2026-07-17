package orgmembers

// Adapter methods that opt this slice's typed errors into
// apierror.FromError mapping. One method per type, no logic — keeps
// the slice's error definitions in service.go untouched.
//
// See pkg/weave/apierror.

func (e *ErrValidation) ValidationFields() map[string][]string { return e.Fields }
func (e *ErrForbidden) IsForbidden() bool                      { return true }
