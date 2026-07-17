package domain

import "errors"

// ErrReadOnly is returned by store mutation methods when the target row is
// not user-mutable (e.g. a system-sourced namespace binding).
var ErrReadOnly = errors.New("row is read only")
