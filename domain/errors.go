package domain

import "errors"

// ErrReadOnly is returned when a persistence method targets a row that is not
// user-mutable, such as a system-sourced namespace binding.
var ErrReadOnly = errors.New("row is read only")
