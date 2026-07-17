package version

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerReportsConfiguredInstance(t *testing.T) {
	handler := NewHandler(nil, nil, "demo")

	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rr := httptest.NewRecorder()

	handler.Check(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var payload Payload
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if got, want := payload.Instance, "demo"; got != want {
		t.Fatalf("Instance = %q, want %q", got, want)
	}
}
