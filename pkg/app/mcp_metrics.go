package app

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/pletka-io/pletka/pkg/app/observability"
)

// mcpToolMetrics implements mcp.ToolMetricsRecorder over a Prometheus
// histogram, registered on the app's observability runtime.
type mcpToolMetrics struct {
	duration *prometheus.HistogramVec
}

// newMCPToolMetrics builds the per-tool MCP call histogram and registers it
// on obs's registry.
func newMCPToolMetrics(obs *observability.Runtime) *mcpToolMetrics {
	m := &mcpToolMetrics{
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "pletka",
			Subsystem: "mcp",
			Name:      "tool_duration_seconds",
			Help:      "Duration of MCP tool calls.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"tool", "outcome"}),
	}
	obs.MustRegister(m.duration)
	return m
}

// ObserveToolCall records one MCP tool call's duration, labelled by tool
// name and outcome ("ok" or "error").
func (m *mcpToolMetrics) ObserveToolCall(tool, outcome string, seconds float64) {
	m.duration.WithLabelValues(tool, outcome).Observe(seconds)
}
