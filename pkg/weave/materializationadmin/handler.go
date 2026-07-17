package materializationadmin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
)

// Handler serves GET /admin/materialization.
type Handler struct {
	store  *Store
	logger *slog.Logger
}

// NewHandler builds the handler.
func NewHandler(store *Store, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{store: store, logger: logger}
}

// List handles GET /admin/materialization as JSON: a summary header (cost
// percentiles, outcome counts, and the live queue-depth / lag gauges) plus the
// most recent runs. Admin-gated by the slice's route mount.
//
// Query params: limit (recent runs, default 100, max 500), window_hours
// (summary window, default 168 = 7d).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	windowHours, _ := strconv.Atoi(q.Get("window_hours"))

	summary, err := h.store.Summary(r.Context(), windowHours)
	if err != nil {
		h.logger.Error("materializationadmin: summary failed", "err", err)
		http.Error(w, "summary failed", http.StatusInternalServerError)
		return
	}
	runs, err := h.store.Recent(r.Context(), limit)
	if err != nil {
		h.logger.Error("materializationadmin: recent failed", "err", err)
		http.Error(w, "recent failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{ // headers already sent; write failure isn't actionable
		"summary": summary,
		"runs":    runs,
		"count":   len(runs),
	})
}
