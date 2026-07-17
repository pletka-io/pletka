package model

// Adapter methods that opt this slice's typed errors into
// apierror.FromError. ErrEntityInUse + ErrSetupIncomplete stay out —
// their handler emissions carry slice-specific fields the frontend
// reads.
//
// See pkg/weave/apierror.

func (e *ErrValidation) ValidationFields() map[string][]string { return e.Fields }
func (e *ErrForbidden) IsForbidden() bool                      { return true }
