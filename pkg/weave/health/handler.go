package health

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/buildinfo"
)

// pingTimeout caps how long a pool ping is allowed to block before the
// probe gives up and returns 503. Health endpoints must answer fast.
const pingTimeout = 2 * time.Second

// Handler renders liveness probes against the pgx pool.
type Handler struct {
	// sanctioned pool holder: DB liveness ping, not domain data access —
	// see docs-oss/architecture/slices.md
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewHandler builds a Handler. nil pool = always-healthy probe (the
// canary boot path with --skip-migrations needs this for a clean
// startup sequence). nil logger -> slog.Default.
func NewHandler(pool *pgxpool.Pool, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{pool: pool, logger: logger}
}

// Check is the canonical handler — same behaviour for /healthz,
// /health, and /api/v1/health.
func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	bi := buildinfo.Get()
	status := map[string]any{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   bi.Version,
		"commit":    bi.Commit,
		"dirty":     bi.Dirty,
		"built_at":  bi.BuiltAt,
	}
	if bi.Branch != "" {
		status["branch"] = bi.Branch
	}

	if h.pool != nil {
		ctx, cancel := context.WithTimeout(r.Context(), pingTimeout)
		defer cancel()
		if err := h.pool.Ping(ctx); err != nil {
			status["status"] = "unhealthy"
			status["error"] = err.Error()
			writeJSON(w, http.StatusServiceUnavailable, status)
			return
		}
	}

	writeJSON(w, http.StatusOK, status)
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
