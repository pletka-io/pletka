package project

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
)

// A blocked edit must distinguish an expired session (401, so the client can
// prompt re-login and keep unsaved edits) from an authenticated caller who
// genuinely lacks ProjectEdit (403). Before this split both returned a 403
// that read as a permissions problem — see the RDMM.11 save report.
func TestWriteEditDenied_AnonymousVsForbidden(t *testing.T) {
	tests := []struct {
		name       string
		snap       *weaveauth.AuthSnapshot
		wantStatus int
		wantCode   string
	}{
		{
			name:       "anonymous caller (lapsed session) -> 401 unauthorized",
			snap:       &weaveauth.AuthSnapshot{IsAnonymous: true},
			wantStatus: http.StatusUnauthorized,
			wantCode:   "unauthorized",
		},
		{
			name:       "authenticated but lacks ProjectEdit -> 403 forbidden",
			snap:       &weaveauth.AuthSnapshot{ActorID: "01ACTOR"},
			wantStatus: http.StatusForbidden,
			wantCode:   "forbidden",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := weaveauth.WithSnapshot(t.Context(), tc.snap)
			r := httptest.NewRequestWithContext(ctx, http.MethodPut, "/projects/P1/models/M1/overrides", nil)
			w := httptest.NewRecorder()

			writeEditDenied(w, r)

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			var body struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body %q: %v", w.Body.String(), err)
			}
			if body.Code != tc.wantCode {
				t.Fatalf("code = %q, want %q", body.Code, tc.wantCode)
			}
		})
	}
}
