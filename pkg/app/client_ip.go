package app

import (
	"net"
	"net/http"
	"strings"
)

// trustedProxyIP rewrites r.RemoteAddr to the client address the reverse proxy
// observed, so that everything downstream — the auth rate limiter, the request
// log, errortracking's own per-IP limiter — keys on an address a client cannot
// choose for itself.
//
// It replaces chi's middleware.RealIP, which took the LEFTMOST X-Forwarded-For
// entry. nginx appends to that header rather than replacing it, so the leftmost
// entry is whatever the client sent: under RealIP an attacker could hand
// authRateLimit a fresh bucket on every request simply by varying the header,
// which defeats the brute-force limiter it exists to be. chi deprecated RealIP
// for exactly this (GHSA-3fxj-6jh8-hvhx, GHSA-rjr7-jggh-pgcp,
// GHSA-9g5q-2w5x-hmxf).
//
// Replacing it in the same slot, rather than deleting it, is deliberate:
// without any such middleware r.RemoteAddr is the proxy's own address for every
// request, which would collapse both rate limiters into a single shared bucket.
func trustedProxyIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip := trustedClientIP(r); ip != "" {
			r.RemoteAddr = ip
		}
		next.ServeHTTP(w, r)
	})
}

// trustedClientIP returns the client address, preferring the sources a client
// cannot forge.
//
// Every instance sits behind nginx, configured (configs/deploy/pletka-instances.conf)
// with:
//
//	proxy_set_header X-Real-IP $remote_addr;                      # overwritten
//	proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;  # appended
//
// so X-Real-IP is set by the proxy and cannot be supplied by the client, while
// in X-Forwarded-For only the rightmost entry — the one nginx appended — is the
// address it actually observed. Everything to its left came from the request.
//
// Falls back to the peer address when neither header is present, which is the
// correct answer when nothing is proxying.
func trustedClientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.Split(fwd, ",")
		// Rightmost first: the proxy appends, so later entries are the ones it
		// vouched for. Skipping blanks keeps a header like " , " from yielding
		// an empty key, which would put every such request in one bucket.
		for i := len(parts) - 1; i >= 0; i-- {
			if ip := strings.TrimSpace(parts[i]); ip != "" {
				return ip
			}
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
