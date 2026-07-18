// Package observability owns the app's Prometheus metrics: an HTTP request
// middleware, pgx pool and materialization collectors, and a dedicated
// /metrics listener. It is a pkg/app runtime concern — slices never import it.
//
// First pass exposes a Prometheus scrape endpoint directly (per the
// 2026-06-28 observability plan). OTLP metrics/traces can layer on later.
// Metrics carry no instance/service labels: Prometheus adds those from the
// scrape target, keeping cardinality out of the app.
package observability

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Config controls the metrics endpoint.
type Config struct {
	// MetricsAddr, when non-empty (e.g. "0.0.0.0:9101"), starts a dedicated
	// metrics HTTP listener serving Path. Empty disables the endpoint —
	// metrics are still collected, just not exposed. Default for dev/tests.
	// Bind to a host+port the monitoring server can reach, firewalled to it.
	MetricsAddr string
	// Path defaults to /metrics.
	Path string
}

// Runtime holds the registry, the request middleware, and the metrics server.
type Runtime struct {
	reg    *prometheus.Registry
	http   *httpMetrics
	srv    *http.Server
	logger *slog.Logger
}

type httpMetrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inFlight prometheus.Gauge
	respSize *prometheus.HistogramVec
}

// New builds the runtime, registers collectors, and starts the metrics
// listener when cfg.MetricsAddr is set. pool may be nil (pool/materialization
// collectors are then skipped).
func New(cfg Config, pool *pgxpool.Pool, logger *slog.Logger) *Runtime {
	if logger == nil {
		logger = slog.Default()
	}
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	h := &httpMetrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pletka_http_requests_total",
			Help: "Total HTTP requests by method, route pattern, status, and status class.",
		}, []string{"method", "route", "status", "status_class"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pletka_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route", "status_class"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "pletka_http_in_flight_requests",
			Help: "In-flight HTTP requests.",
		}),
		respSize: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pletka_http_response_size_bytes",
			Help:    "HTTP response size in bytes.",
			Buckets: prometheus.ExponentialBuckets(64, 4, 8),
		}, []string{"route"}),
	}
	reg.MustRegister(h.requests, h.duration, h.inFlight, h.respSize)

	if pool != nil {
		reg.MustRegister(newPoolCollector(pool), newMaterializationCollector(pool))
	}

	rt := &Runtime{reg: reg, http: h, logger: logger}

	if cfg.MetricsAddr != "" {
		path := cfg.Path
		if path == "" {
			path = "/metrics"
		}
		mux := http.NewServeMux()
		mux.Handle(path, promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
		rt.srv = &http.Server{
			Addr:              cfg.MetricsAddr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		}
		go func() {
			logger.Info("metrics endpoint listening", "addr", cfg.MetricsAddr, "path", path)
			if err := rt.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("metrics endpoint failed", "err", err)
			}
		}()
	}

	return rt
}

// Middleware instruments every request: count, duration, in-flight, and
// response size, labelled by the chi route pattern (never the raw URL).
func (rt *Runtime) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rt.http.inFlight.Inc()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		rt.http.inFlight.Dec()
		route := chi.RouteContext(r.Context()).RoutePattern()
		if route == "" {
			route = "unknown"
		}
		class := statusClass(rec.status)
		rt.http.requests.WithLabelValues(r.Method, route, strconv.Itoa(rec.status), class).Inc()
		rt.http.duration.WithLabelValues(r.Method, route, class).Observe(time.Since(start).Seconds())
		rt.http.respSize.WithLabelValues(route).Observe(float64(rec.written))
	})
}

// MustRegister adds collectors to the runtime's registry. Call during
// assembly, before Start; panics on duplicate registration (wiring-time).
func (rt *Runtime) MustRegister(cs ...prometheus.Collector) { rt.reg.MustRegister(cs...) }

// Close shuts down the metrics listener.
func (rt *Runtime) Close(ctx context.Context) error {
	if rt == nil || rt.srv == nil {
		return nil
	}
	return rt.srv.Shutdown(ctx)
}

// statusRecorder captures the response status and byte count.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written int
	wrote   bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.wrote = true
	n, err := s.ResponseWriter.Write(b)
	s.written += n
	return n, err
}

// statusClass buckets a status code into 2xx/3xx/4xx/5xx (low cardinality).
func statusClass(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	case code >= 300:
		return "3xx"
	case code >= 200:
		return "2xx"
	default:
		return "1xx"
	}
}
