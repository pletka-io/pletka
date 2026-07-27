package app

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// recoverMiddleware recovers panics, logs the stack, and renders the branded
// 500 via render (nil → plain "Internal Server Error"). It sits where chi's
// Recoverer did — inside errortracking — so the 500 is still captured.
func recoverMiddleware(render func(http.ResponseWriter, *http.Request), log *slog.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic recovered", "err", rec, "path", r.URL.Path, "stack", string(debug.Stack()))
					if render != nil {
						render(w, r)
						return
					}
					http.Error(w, "Internal Server Error", http.StatusInternalServerError) //nolint:forbidigo // last-resort fallback when the branded renderer is unavailable
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
