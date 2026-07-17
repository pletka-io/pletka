package release

// Adapter methods opting this slice's typed errors into
// apierror.FromError. permissionDenied stays out — release surfaces
// 404 on auth-deny to avoid leaking project existence.
//
// See pkg/weave/apierror.

func (e *ErrValidation) ValidationFields() map[string][]string { return e.Fields }
func (e *ErrConflict) ConflictMessage() string                 { return e.Message }
