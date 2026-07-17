package apierror

import (
	"encoding/json"
	"errors"
	"net/http"
)

// Code is the machine-readable error kind. The frontend can branch on
// these stable strings instead of fragile message text. Add new codes
// here as new error kinds emerge — the constant list IS the contract.
type Code string

const (
	CodeBadRequest        Code = "bad_request"
	CodeUnauthorized      Code = "unauthorized"
	CodeForbidden         Code = "forbidden"
	CodeNotFound          Code = "not_found"
	CodeConflict          Code = "conflict"
	CodeValidation        Code = "validation"
	CodeInUse             Code = "in_use"
	CodeInternal          Code = "internal"
	CodeMethodNotAllowed  Code = "method_not_allowed"
	CodeUnprocessable     Code = "unprocessable"
)

// Error is the canonical wire shape for every JSON error response.
// Status is the HTTP code Write will emit; the rest are JSON fields.
type Error struct {
	Status  int                 `json:"-"`
	Code    Code                `json:"code,omitempty"`
	Message string              `json:"error"`
	Details string              `json:"message,omitempty"`
	Fields  map[string][]string `json:"errors,omitempty"`
}

// Error satisfies the error interface so apierror.Error values can
// flow through errors.As / errors.Is chains alongside slice errors.
func (e *Error) Error() string {
	if e == nil {
		return "<nil apierror>"
	}
	return e.Message
}

// ----- Constructors -------------------------------------------------------

// BadRequest builds a 400 with msg as the human summary.
func BadRequest(msg string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: CodeBadRequest, Message: msg}
}

// Unauthorized builds a 401. Body kept generic — the gate-up-stream
// already knows the resource; auth handler decides the redirect.
func Unauthorized() *Error {
	return &Error{Status: http.StatusUnauthorized, Code: CodeUnauthorized, Message: "authentication required"}
}

// Forbidden builds a 403. Pass an empty msg to use the default
// "forbidden" — handler-specific messages are fine but the frontend
// keys on Code.
func Forbidden(msg string) *Error {
	if msg == "" {
		msg = "forbidden"
	}
	return &Error{Status: http.StatusForbidden, Code: CodeForbidden, Message: msg}
}

// NotFound builds a 404. Empty msg defaults to "not found" so callers
// don't reinvent the wheel for the common case.
func NotFound(msg string) *Error {
	if msg == "" {
		msg = "not found"
	}
	return &Error{Status: http.StatusNotFound, Code: CodeNotFound, Message: msg}
}

// Conflict builds a 409 (duplicate, optimistic-lock failure, etc.).
func Conflict(msg string) *Error {
	return &Error{Status: http.StatusConflict, Code: CodeConflict, Message: msg}
}

// InUse builds a 409 specifically for the "can't delete because
// dependents exist" case. Separate code so the frontend can render
// dependent-count UI that's distinct from a generic conflict.
func InUse(msg string) *Error {
	return &Error{Status: http.StatusConflict, Code: CodeInUse, Message: msg}
}

// Validation builds a 422 with per-field errors. The Details string
// is a fixed friendly message FormRenderer uses for the top-level
// banner; passing custom copy is fine but the default fits 95% of
// cases.
func Validation(fields map[string][]string) *Error {
	return &Error{
		Status:  http.StatusUnprocessableEntity,
		Code:    CodeValidation,
		Message: "validation error",
		Details: "Please correct the highlighted fields and try again.",
		Fields:  fields,
	}
}

// Internal builds a generic 500. Caller must already have logged the
// underlying err — this constructor does NOT preserve it (response
// body must not leak internal detail).
func Internal() *Error {
	return &Error{Status: http.StatusInternalServerError, Code: CodeInternal, Message: "internal server error"}
}

// InternalWith is Internal() with a caller-supplied human message.
// Use only when the message itself is operationally descriptive and
// safe to expose ("failed to list users", "failed to update profile",
// etc.) — never pass an underlying err.Error() through here, since
// that can leak query plans, file paths, or stack traces.
func InternalWith(msg string) *Error {
	if msg == "" {
		return Internal()
	}
	return &Error{Status: http.StatusInternalServerError, Code: CodeInternal, Message: msg}
}

// MethodNotAllowed builds a 405. The chi global handler also emits
// this shape; constructor exists for handlers that want to reject
// specific verbs explicitly.
func MethodNotAllowed() *Error {
	return &Error{Status: http.StatusMethodNotAllowed, Code: CodeMethodNotAllowed, Message: "method not allowed"}
}

// ----- Adapter interfaces -------------------------------------------------

// Slice service-error types implement one of these to opt into
// FromError mapping. Two-line method on the existing struct — no
// import dependency on apierror, no breaking change.

// ValidationFielder is implemented by slice ErrValidation types.
//
//	func (e *ErrValidation) ValidationFields() map[string][]string { return e.Fields }
type ValidationFielder interface {
	error
	ValidationFields() map[string][]string
}

// Conflicter is implemented by slice ErrConflict types whose body is
// the user-facing conflict description.
//
//	func (e *ErrConflict) ConflictMessage() string { return e.Message }
type Conflicter interface {
	error
	ConflictMessage() string
}

// Forbidder is implemented by slice ErrForbidden types. Most carry
// (Capability, Resource); the implementation just returns the
// already-formatted Error() string.
//
//	func (e *ErrForbidden) IsForbidden() bool { return true }
type Forbidder interface {
	error
	IsForbidden() bool
}

// NotFounder is implemented by slice errors that should map to 404.
// Optional — many slices use a sentinel return rather than a typed
// error; those handlers keep using NotFound("...") directly.
type NotFounder interface {
	error
	IsNotFound() bool
}

// InUser is implemented by slice ErrInUse types (e.g. delete blocked
// because dependents exist).
//
//	func (e *ErrInUse) InUseMessage() string { return e.Message }
type InUser interface {
	error
	InUseMessage() string
}

// ----- Mapping ------------------------------------------------------------

// FromError maps a service-layer error to the canonical Error.
// Recognises any error that satisfies one of the adapter interfaces;
// falls through to Internal() for unknown types. Caller is still
// expected to log the original err separately — FromError never
// preserves the underlying Go error in the response body.
//
// Order matters: validation runs first because field errors are
// rarely also forbidden / conflict; in-use runs before generic
// conflict so the "delete blocked" case keeps its dedicated code.
func FromError(err error) *Error {
	if err == nil {
		return nil
	}
	var v ValidationFielder
	if errors.As(err, &v) {
		return Validation(v.ValidationFields())
	}
	var iu InUser
	if errors.As(err, &iu) {
		return InUse(iu.InUseMessage())
	}
	var c Conflicter
	if errors.As(err, &c) {
		return Conflict(c.ConflictMessage())
	}
	var f Forbidder
	if errors.As(err, &f) {
		return Forbidden(f.Error())
	}
	var nf NotFounder
	if errors.As(err, &nf) {
		return NotFound(nf.Error())
	}
	var ae *Error
	if errors.As(err, &ae) {
		return ae
	}
	return Internal()
}

// ----- Writing ------------------------------------------------------------

// Write emits the canonical envelope at e.Status. nil writes a 500
// (defensive — shouldn't happen, but a nil deref here would mask the
// real bug).
func Write(w http.ResponseWriter, e *Error) {
	if e == nil {
		e = Internal()
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(e.Status)
	_ = json.NewEncoder(w).Encode(e) // best-effort: headers already sent, write failure isn't actionable
}
