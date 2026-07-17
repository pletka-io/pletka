package apierror

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeValidation simulates a slice ErrValidation type opting in to
// the ValidationFielder interface.
type fakeValidation struct{ Fields map[string][]string }

func (e *fakeValidation) Error() string                          { return "validation" }
func (e *fakeValidation) ValidationFields() map[string][]string { return e.Fields }

type fakeConflict struct{ Msg string }

func (e *fakeConflict) Error() string           { return e.Msg }
func (e *fakeConflict) ConflictMessage() string { return e.Msg }

type fakeForbidden struct{}

func (e *fakeForbidden) Error() string     { return "no access to project:LA" }
func (e *fakeForbidden) IsForbidden() bool { return true }

type fakeInUse struct{ Msg string }

func (e *fakeInUse) Error() string        { return e.Msg }
func (e *fakeInUse) InUseMessage() string { return e.Msg }

type fakeNotFound struct{}

func (e *fakeNotFound) Error() string    { return "model AME.M.42 missing" }
func (e *fakeNotFound) IsNotFound() bool { return true }

func TestWrite_Shape(t *testing.T) {
	cases := []struct {
		name   string
		e      *Error
		status int
		body   map[string]any
	}{
		{
			name:   "bad request",
			e:      BadRequest("bad input"),
			status: 400,
			body:   map[string]any{"code": "bad_request", "error": "bad input"},
		},
		{
			name:   "validation includes fields + details",
			e:      Validation(map[string][]string{"name": {"required"}}),
			status: 422,
			body: map[string]any{
				"code":    "validation",
				"error":   "validation error",
				"message": "Please correct the highlighted fields and try again.",
				"errors":  map[string]any{"name": []any{"required"}},
			},
		},
		{
			name:   "in_use distinct from generic conflict",
			e:      InUse("3 fields still reference this category"),
			status: 409,
			body:   map[string]any{"code": "in_use", "error": "3 fields still reference this category"},
		},
		{
			name:   "internal hides cause",
			e:      Internal(),
			status: 500,
			body:   map[string]any{"code": "internal", "error": "internal server error"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			Write(rec, tc.e)
			if rec.Code != tc.status {
				t.Errorf("status = %d; want %d", rec.Code, tc.status)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
				t.Errorf("content-type = %q; want application/json", ct)
			}
			var got map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("unmarshal: %v\nbody=%s", err, rec.Body.String())
			}
			for k, want := range tc.body {
				switch w := want.(type) {
				case map[string]any:
					gm, ok := got[k].(map[string]any)
					if !ok {
						t.Errorf("field %q: not a map; got %T", k, got[k])
						continue
					}
					for fk, fv := range w {
						if gv, ok := gm[fk]; !ok {
							t.Errorf("field %q.%q missing", k, fk)
						} else if !sliceEqual(fv, gv) {
							t.Errorf("field %q.%q = %v; want %v", k, fk, gv, fv)
						}
					}
				default:
					if got[k] != want {
						t.Errorf("field %q = %v; want %v", k, got[k], want)
					}
				}
			}
		})
	}
}

func sliceEqual(want, got any) bool {
	w, wok := want.([]any)
	g, gok := got.([]any)
	if !wok || !gok || len(w) != len(g) {
		return false
	}
	for i := range w {
		if w[i] != g[i] {
			return false
		}
	}
	return true
}

func TestFromError_AdapterDispatch(t *testing.T) {
	cases := []struct {
		name     string
		in       error
		wantCode Code
	}{
		{"validation wins over others", &fakeValidation{Fields: map[string][]string{"x": {"y"}}}, CodeValidation},
		{"conflict", &fakeConflict{Msg: "duplicate"}, CodeConflict},
		{"forbidden", &fakeForbidden{}, CodeForbidden},
		{"in-use beats conflict ordering", &fakeInUse{Msg: "still referenced"}, CodeInUse},
		{"not found", &fakeNotFound{}, CodeNotFound},
		{"unknown error → internal", errors.New("kaboom"), CodeInternal},
		{"already an apierror.Error round-trips", BadRequest("nope"), CodeBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := FromError(tc.in)
			if out == nil {
				t.Fatalf("FromError returned nil")
			}
			if out.Code != tc.wantCode {
				t.Errorf("code = %q; want %q", out.Code, tc.wantCode)
			}
		})
	}
}

func TestFromError_NilSafe(t *testing.T) {
	if got := FromError(nil); got != nil {
		t.Errorf("FromError(nil) = %v; want nil", got)
	}
}

func TestWrite_NilDefaultsToInternal(t *testing.T) {
	rec := httptest.NewRecorder()
	Write(rec, nil)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("nil Error: status = %d; want 500", rec.Code)
	}
}
