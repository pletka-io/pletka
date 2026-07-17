package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthRateLimitReturns429AfterBurst(t *testing.T) {
	h := authRateLimit(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	var last int
	for i := 0; i < 30; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.RemoteAddr = "203.0.113.7:12345"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		last = rec.Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("after 30 rapid requests: got %d, want 429", last)
	}
}
