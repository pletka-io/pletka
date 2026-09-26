package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestTrustedClientIPIgnoresClientSuppliedForwardedFor is the regression gate
// for a rate-limiter bypass: chi's middleware.RealIP rewrote r.RemoteAddr to
// the LEFTMOST X-Forwarded-For entry, which the client supplies. authRateLimit
// keys its per-IP brute-force limiter on r.RemoteAddr, so varying that header
// handed an attacker a fresh bucket on every request and defeated the limiter
// entirely. chi deprecated RealIP for exactly this (GHSA-3fxj-6jh8-hvhx,
// GHSA-rjr7-jggh-pgcp, GHSA-9g5q-2w5x-hmxf).
//
// Our nginx sets `X-Real-IP $remote_addr` (overwritten, so a client cannot
// forge it) and `X-Forwarded-For $proxy_add_x_forwarded_for` (appended, so the
// RIGHTMOST entry is the address nginx actually observed and everything to its
// left is whatever the client sent).
func TestTrustedClientIPIgnoresClientSuppliedForwardedFor(t *testing.T) {
	const (
		realClient = "203.0.113.7"
		attacker   = "198.51.100.99"
	)

	tests := []struct {
		name       string
		remoteAddr string
		realIP     string
		forwarded  string
		want       string
	}{
		{
			// The live shape: nginx overwrites X-Real-IP and appends to XFF, so
			// the forged leftmost entry must lose to both.
			name:       "forged leftmost XFF loses to X-Real-IP",
			remoteAddr: "127.0.0.1:41234",
			realIP:     realClient,
			forwarded:  attacker + ", " + realClient,
			want:       realClient,
		},
		{
			// Without X-Real-IP, the rightmost XFF entry is the one the proxy
			// appended; everything left of it is client-controlled.
			name:       "without X-Real-IP the rightmost XFF entry wins",
			remoteAddr: "127.0.0.1:41234",
			forwarded:  attacker + ", " + realClient,
			want:       realClient,
		},
		{
			// A lone forged entry with no proxy in front must not be trusted
			// over the actual peer.
			name:       "no proxy headers falls back to the peer address",
			remoteAddr: realClient + ":41234",
			want:       realClient,
		},
		{
			// Whitespace and empty entries are ordinary in XFF and must not
			// produce an empty key, which would collapse every such request
			// into one shared bucket.
			name:       "blank entries never yield an empty key",
			remoteAddr: realClient + ":41234",
			forwarded:  " , ",
			want:       realClient,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/auth/login", nil)
			r.RemoteAddr = tc.remoteAddr
			if tc.realIP != "" {
				r.Header.Set("X-Real-IP", tc.realIP)
			}
			if tc.forwarded != "" {
				r.Header.Set("X-Forwarded-For", tc.forwarded)
			}
			if got := trustedClientIP(r); got != tc.want {
				t.Errorf("trustedClientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestTrustedProxyIPMiddlewareRewritesRemoteAddr pins the middleware itself:
// every downstream reader of r.RemoteAddr — the auth limiter, the request log,
// and errortracking's own per-IP limiter in another package — must see the
// trusted address without any of them changing.
func TestTrustedProxyIPMiddlewareRewritesRemoteAddr(t *testing.T) {
	const realClient = "203.0.113.7"

	var seen string
	handler := trustedProxyIP(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = r.RemoteAddr
	}))

	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/auth/login", nil)
	r.RemoteAddr = "127.0.0.1:41234"
	r.Header.Set("X-Real-IP", realClient)
	r.Header.Set("X-Forwarded-For", "198.51.100.99, "+realClient)

	handler.ServeHTTP(httptest.NewRecorder(), r)

	if seen != realClient {
		t.Errorf("downstream RemoteAddr = %q, want %q — a forged header reached the rate-limit key", seen, realClient)
	}
}
