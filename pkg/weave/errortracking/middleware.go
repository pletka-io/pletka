package errortracking

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
)

// maxResponseExcerptBytes caps how much response body we retain for
// 4xx/5xx rows. Enough to surface error envelopes without bloating
// the table.
const maxResponseExcerptBytes = 4 * 1024

// asyncWriteTimeout is the context budget for the background insert.
// Kept tight so a slow DB doesn't pile up goroutines.
const asyncWriteTimeout = 5 * time.Second

// Middleware returns a chi middleware that records server-side
// failures (status >= 400) into the given store. 2xx and 3xx
// responses are skipped entirely.
func Middleware(store *Store, logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if store == nil {
				next.ServeHTTP(w, r)
				return
			}

			started := time.Now()

			reqBody := readAndReplaceBody(r)

			rec := &captureWriter{
				ResponseWriter: w,
				status:         http.StatusOK,
				body:           &bytes.Buffer{},
			}

			next.ServeHTTP(rec, r)

			if rec.status < 400 {
				return
			}

			pattern := chi.RouteContext(r.Context()).RoutePattern()
			route := pattern
			if route == "" {
				route = r.URL.Path
			}
			method := r.Method
			status := rec.status
			duration := int(time.Since(started).Milliseconds())
			actorID := actorIDFromContext(r.Context())

			// Category at capture time. Empty chi pattern + 4xx means no
			// route matched — that's the canary's "true legacy gap"
			// signal. Non-empty pattern means a handler ran and chose
			// the response (auth gate denial, missing entity, validation
			// reject); 5xx is server-side regardless of routing.
			category := classifyServer(status, pattern)

			// Sanitise payloads up front. Postgres jsonb rejects NUL bytes
			// and unpaired surrogates (SQLSTATE 22P05), and arbitrary
			// upstream input (headers, referers, browser-extension noise)
			// can carry them. Scrubbing once on the way in avoids a
			// Postgres roundtrip + WARN log line on every such request.
			ev := Event{
				RequestID:  chimiddleware.GetReqID(r.Context()),
				OccurredAt: started,
				Source:     SourceServer,
				Category:   category,
				Route:      route,
				Method:     &method,
				Status:     &status,
				ActorID:    actorID,
				DurationMS: &duration,
				UserAgent:  stripNUL(r.UserAgent()),
				Request:    sanitiseRawMessage(buildRequestPayload(r, reqBody)),
				Response:   sanitiseRawMessage(buildResponsePayload(rec)),
				Error:      sanitiseRawMessage(buildErrorPayload(rec)),
			}

			go func(e Event) {
				ctx, cancel := context.WithTimeout(context.Background(), asyncWriteTimeout)
				defer cancel()
				if err := store.Insert(ctx, e); err != nil {
					logger.Warn("errortracking: insert failed",
						"err", err,
						"route", e.Route,
						"status", *e.Status,
						"request_id", e.RequestID)
				}
			}(ev)
		})
	}
}

// readAndReplaceBody reads up to maxPayloadBytes from r.Body and
// replaces r.Body with a new reader so the downstream handler sees
// the same bytes. Returns nil if there's nothing to read or the
// content type isn't JSON-shaped.
func readAndReplaceBody(r *http.Request) []byte {
	if r.Body == nil || r.Body == http.NoBody {
		return nil
	}
	ct := r.Header.Get("Content-Type")
	if ct != "" && !strings.Contains(strings.ToLower(ct), "json") {
		return nil
	}

	limited := io.LimitReader(r.Body, maxPayloadBytes+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		return nil
	}

	rest, _ := io.ReadAll(r.Body)
	full := append(buf, rest...)
	r.Body = io.NopCloser(bytes.NewReader(full))

	if len(buf) > maxPayloadBytes {
		return buf[:maxPayloadBytes]
	}
	return buf
}

func actorIDFromContext(ctx context.Context) *string {
	snap := weaveauth.FromContext(ctx)
	if snap == nil || snap.IsAnonymous || snap.ActorID == "" {
		return nil
	}
	id := snap.ActorID
	return &id
}

func buildRequestPayload(r *http.Request, body []byte) json.RawMessage {
	payload := redactJSONPayload(body)
	envelope := map[string]any{
		"headers_snapshot": snapshotHeaders(r.Header),
		"query":            r.URL.RawQuery,
	}
	if payload != nil {
		envelope["payload"] = json.RawMessage(payload)
	}
	out, err := json.Marshal(envelope)
	if err != nil {
		return json.RawMessage("{}")
	}
	return out
}

func buildResponsePayload(rec *captureWriter) json.RawMessage {
	excerpt := rec.body.Bytes()
	if len(excerpt) > maxResponseExcerptBytes {
		excerpt = excerpt[:maxResponseExcerptBytes]
	}
	out, err := json.Marshal(map[string]any{
		"excerpt":      string(excerpt),
		"content_type": rec.Header().Get("Content-Type"),
		"length":       rec.body.Len(),
	})
	if err != nil {
		return json.RawMessage("{}")
	}
	return out
}

func buildErrorPayload(rec *captureWriter) json.RawMessage {
	if rec.status < 500 {
		return json.RawMessage("{}")
	}
	msg := strings.TrimSpace(rec.body.String())
	if len(msg) > 1024 {
		msg = msg[:1024]
	}
	out, err := json.Marshal(map[string]any{
		"class":   fmt.Sprintf("HTTP%d", rec.status),
		"message": msg,
	})
	if err != nil {
		return json.RawMessage("{}")
	}
	return out
}

// stripNUL removes NUL bytes from a string. Postgres TEXT columns reject
// NUL, so user-agents / arbitrary input strings get scrubbed before
// retrying an insert.
func stripNUL(s string) string {
	if !strings.ContainsRune(s, '\x00') {
		return s
	}
	return strings.ReplaceAll(s, "\x00", "")
}

// sanitiseRawMessage round-trips a JSON payload through decode+encode so
// any embedded NUL escapes (which Postgres jsonb refuses with SQLSTATE
// 22P05) are dropped. Falls back to an empty JSON object on parse error
// so the retry insert always carries syntactically valid jsonb.
func sanitiseRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return json.RawMessage("{}")
	}
	v = scrubStrings(v)
	out, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return out
}

func scrubStrings(v any) any {
	switch t := v.(type) {
	case string:
		return stripNUL(t)
	case map[string]any:
		for k, val := range t {
			t[k] = scrubStrings(val)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = scrubStrings(val)
		}
		return t
	default:
		return v
	}
}

// classifyServer derives the Category for a server-side response from
// the status code + the chi route pattern. Empty pattern means chi
// did not match any registered route. See store.go for category
// semantics.
func classifyServer(status int, pattern string) Category {
	if status >= 500 {
		return CategoryServerError
	}
	if pattern == "" {
		return CategoryNoRoute
	}
	return CategoryHandlerDecision
}

// captureWriter wraps http.ResponseWriter so we can read back the
// status and an excerpt of the body after the handler returns.
type captureWriter struct {
	http.ResponseWriter
	status      int
	body        *bytes.Buffer
	wroteHeader bool
}

func (c *captureWriter) WriteHeader(status int) {
	if c.wroteHeader {
		return
	}
	c.wroteHeader = true
	c.status = status
	c.ResponseWriter.WriteHeader(status)
}

func (c *captureWriter) Write(b []byte) (int, error) {
	if !c.wroteHeader {
		c.WriteHeader(http.StatusOK)
	}
	if c.body.Len() < maxResponseExcerptBytes {
		remaining := maxResponseExcerptBytes - c.body.Len()
		if remaining > len(b) {
			remaining = len(b)
		}
		c.body.Write(b[:remaining])
	}
	return c.ResponseWriter.Write(b)
}
