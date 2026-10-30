package projectontology

import "fmt"

// InvalidError reports a selection that cannot produce a published closure.
// Database and cancellation errors retain their original types instead.
type InvalidError struct{ Detail string }

func (e *InvalidError) Error() string { return e.Detail }

// ValidationFields uses the existing publication validation field.
func (e *InvalidError) ValidationFields() map[string][]string {
	return map[string][]string{"publication": {e.Detail}}
}
func invalid(format string, args ...any) error {
	return &InvalidError{Detail: fmt.Sprintf(format, args...)}
}
