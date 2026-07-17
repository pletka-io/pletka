package collection

// Adapter methods opting this slice's typed errors into
// apierror.FromError. ErrEntityInUse + ErrSetupIncomplete carry
// slice-specific payload and stay inline in writeServiceError.
//
// See pkg/weave/apierror.

func (e *ErrValidation) ValidationFields() map[string][]string { return e.Fields }
func (e *ErrForbidden) IsForbidden() bool                      { return true }
