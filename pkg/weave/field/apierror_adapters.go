package field

// Adapter methods opting this slice's typed errors into
// apierror.FromError. ErrEntityInUse + ErrSetupIncomplete are
// reachable from handlers that build their own envelopes (the
// override + setup paths); they stay out for now.
//
// See pkg/weave/apierror.

func (e *ErrValidation) ValidationFields() map[string][]string { return e.Fields }
func (e *ErrForbidden) IsForbidden() bool                      { return true }
