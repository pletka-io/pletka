// Package health serves the platform's liveness + readiness probes. This
// package is a handler-only module: it holds a *pgxpool.Pool directly only
// for the readiness ping, a bless-as-is exception tracked as pool-holder
// debt in the compliance backlog, and owns no store beyond that.
//
// Three endpoints, all returning the same small JSON payload:
//
//	GET /healthz         — k8s-style liveness probe (also /readyz alias)
//	GET /health          — operator-friendly health status
//	GET /api/v1/health   — back-compat path (legacy monitoring still hits it)
//
// Payload:
//
//	{"status":"healthy","timestamp":"<RFC3339>","version":"<platform>","commit":"<core>","dirty":false,"built_at":"<RFC3339>"}
//
// On DB failure (pgx pool ping fails): 503 + status="unhealthy" with the
// underlying error class. No template, no auth, no rate limit — these
// paths are hit by load balancers and monitoring tools.
//
// The old dashboard/check HTML endpoints were removed with the legacy
// template manager; this slice owns the remaining health surface.
package health
