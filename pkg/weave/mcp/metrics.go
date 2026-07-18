package mcp

import (
	"context"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// instrumented wraps a tool handler with per-call metrics. outcome is
// "ok" or "error" (tool errors only; transport errors never reach here).
func instrumented[In, Out any](h Host, tool string, f sdk.ToolHandlerFor[In, Out]) sdk.ToolHandlerFor[In, Out] {
	if h.Metrics == nil {
		return f
	}
	return func(ctx context.Context, req *sdk.CallToolRequest, in In) (*sdk.CallToolResult, Out, error) {
		start := time.Now()
		res, out, err := f(ctx, req, in)
		outcome := "ok"
		if err != nil {
			outcome = "error"
		}
		h.Metrics.ObserveToolCall(tool, outcome, time.Since(start).Seconds())
		return res, out, err
	}
}
