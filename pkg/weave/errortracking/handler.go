package errortracking

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
)

// clientReportLimit caps how many client error reports we accept per
// actor (or anonymous IP) per minute. Cheap defence against a runaway
// SPA that pours errors into the endpoint.
const clientReportLimit = 60

// Handler exposes the HTTP surface for the slice: POST /errors/client
// for the browser reporter and GET /admin/errors for the viewer.
type Handler struct {
	store   *Store
	logger  *slog.Logger
	limiter *rateLimiter
}

// NewHandler builds the handler.
func NewHandler(store *Store, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		store:   store,
		logger:  logger,
		limiter: newRateLimiter(clientReportLimit, time.Minute),
	}
}

// ClientReport is the JSON envelope POSTed by frontend/src/lib/error-tracking.ts.
// All fields are optional; the endpoint records whatever it receives.
type ClientReport struct {
	OccurredAt  string          `json:"occurred_at"`
	PageURL     string          `json:"page_url"`
	Island      string          `json:"island"`
	Error       ClientErrorBody `json:"error"`
	Context     json.RawMessage `json:"context"`
	Breadcrumbs json.RawMessage `json:"breadcrumbs"`
}

// ClientErrorBody is the inner error payload from the browser.
type ClientErrorBody struct {
	Class   string `json:"class"`
	Message string `json:"message"`
	Stack   string `json:"stack"`
}

// PostClient handles POST /errors/client.
func (h *Handler) PostClient(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	key := rateLimitKey(r)
	if !h.limiter.allow(key) {
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}

	// 64KB: the client bounds stack (12KB) + message (2KB) + 20 small
	// breadcrumbs, so a well-formed report is far under this. The prior 32KB
	// cap rejected legitimate oversized-stack reports with a 400, silently
	// dropping the very errors worth capturing.
	var report ClientReport
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&report); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	occurred, err := time.Parse(time.RFC3339Nano, report.OccurredAt)
	if err != nil {
		occurred = time.Now()
	}

	route := report.PageURL
	if route == "" {
		route = "(client)"
	}

	requestEnv := map[string]any{
		"island": report.Island,
	}
	if len(report.Context) > 0 {
		requestEnv["context"] = report.Context
	}
	if len(report.Breadcrumbs) > 0 {
		requestEnv["breadcrumbs"] = report.Breadcrumbs
	}
	requestPayload, _ := json.Marshal(requestEnv)

	errorPayload, _ := json.Marshal(map[string]any{
		"class":   report.Error.Class,
		"message": report.Error.Message,
		"stack":   report.Error.Stack,
	})

	ev := Event{
		OccurredAt: occurred,
		Source:     SourceClient,
		Category:   CategoryClient,
		Route:      route,
		ActorID:    actorIDFromContext(r.Context()),
		UserAgent:  r.UserAgent(),
		Request:    requestPayload,
		Error:      errorPayload,
	}

	if err := h.store.Insert(r.Context(), ev); err != nil {
		h.logger.Warn("errortracking: client insert failed", "err", err, "route", route)
		http.Error(w, "store insert failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// List handles GET /admin/errors as JSON. Admin-gated by the slice's
// route mount.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status, _ := strconv.Atoi(q.Get("status"))
	limit, _ := strconv.Atoi(q.Get("limit"))

	results, err := h.store.List(r.Context(), ListFilter{
		Source:   q.Get("source"),
		Category: q.Get("category"),
		Status:   status,
		Route:    q.Get("route"),
		Limit:    limit,
	})
	if err != nil {
		h.logger.Error("errortracking: list failed", "err", err)
		http.Error(w, "list failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{ // best-effort: headers already sent, write failure isn't actionable
		"events": results,
		"count":  len(results),
	})
}

// rateLimitKey identifies the bucket: actor ID if authenticated,
// otherwise the remote address.
func rateLimitKey(r *http.Request) string {
	if actor := actorIDFromContext(r.Context()); actor != nil {
		return "actor:" + *actor
	}
	return "ip:" + r.RemoteAddr
}

// rateLimiter is a tiny per-key window counter. Plenty for this use.
type rateLimiter struct {
	limit  int
	window time.Duration
	mu     sync.Mutex
	hits   map[string]*bucket
}

type bucket struct {
	count int
	reset time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		limit:  limit,
		window: window,
		hits:   make(map[string]*bucket),
	}
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.hits[key]
	if !ok || now.After(b.reset) {
		rl.hits[key] = &bucket{count: 1, reset: now.Add(rl.window)}
		return true
	}
	if b.count >= rl.limit {
		return false
	}
	b.count++
	return true
}

// Ensure weaveauth is referenced (actorIDFromContext uses it indirectly
// through the middleware package). This blank assignment keeps gopls
// quiet if the symbol falls out of use during refactors.
var _ = weaveauth.FromContext
