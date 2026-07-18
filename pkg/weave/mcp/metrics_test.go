package mcp

import (
	"context"
	"errors"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// fakeMetricsRecorder captures ObserveToolCall invocations for assertions.
type fakeMetricsRecorder struct {
	calls []recordedCall
}

type recordedCall struct {
	tool    string
	outcome string
	seconds float64
}

func (f *fakeMetricsRecorder) ObserveToolCall(tool, outcome string, seconds float64) {
	f.calls = append(f.calls, recordedCall{tool: tool, outcome: outcome, seconds: seconds})
}

type metricsTestIn struct {
	Value string
}

type metricsTestOut struct {
	Echo string
}

func TestInstrumented_RecordsOkOnSuccess(t *testing.T) {
	rec := &fakeMetricsRecorder{}
	h := Host{Metrics: rec}
	handler := func(_ context.Context, _ *sdk.CallToolRequest, in metricsTestIn) (*sdk.CallToolResult, metricsTestOut, error) {
		return nil, metricsTestOut{Echo: in.Value}, nil
	}

	wrapped := instrumented(h, "list_projects", handler)
	_, out, err := wrapped(context.Background(), &sdk.CallToolRequest{}, metricsTestIn{Value: "hi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Echo != "hi" {
		t.Fatalf("expected passthrough output, got %q", out.Echo)
	}

	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 recorded call, got %d", len(rec.calls))
	}
	call := rec.calls[0]
	if call.tool != "list_projects" {
		t.Errorf("tool = %q, want list_projects", call.tool)
	}
	if call.outcome != "ok" {
		t.Errorf("outcome = %q, want ok", call.outcome)
	}
	if call.seconds < 0 {
		t.Errorf("seconds = %v, want >= 0", call.seconds)
	}
}

func TestInstrumented_RecordsErrorOnHandlerError(t *testing.T) {
	rec := &fakeMetricsRecorder{}
	h := Host{Metrics: rec}
	wantErr := errors.New("boom")
	handler := func(_ context.Context, _ *sdk.CallToolRequest, _ metricsTestIn) (*sdk.CallToolResult, metricsTestOut, error) {
		return nil, metricsTestOut{}, wantErr
	}

	wrapped := instrumented(h, "get_project", handler)
	_, _, err := wrapped(context.Background(), &sdk.CallToolRequest{}, metricsTestIn{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped handler to return original error, got %v", err)
	}

	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 recorded call, got %d", len(rec.calls))
	}
	if rec.calls[0].outcome != "error" {
		t.Errorf("outcome = %q, want error", rec.calls[0].outcome)
	}
	if rec.calls[0].tool != "get_project" {
		t.Errorf("tool = %q, want get_project", rec.calls[0].tool)
	}
}

func TestInstrumented_NilMetricsPassesThrough(t *testing.T) {
	h := Host{Metrics: nil}
	called := false
	handler := func(_ context.Context, _ *sdk.CallToolRequest, in metricsTestIn) (*sdk.CallToolResult, metricsTestOut, error) {
		called = true
		return nil, metricsTestOut{Echo: in.Value}, nil
	}

	wrapped := instrumented(h, "list_projects", handler)
	_, out, err := wrapped(context.Background(), &sdk.CallToolRequest{}, metricsTestIn{Value: "passthrough"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected underlying handler to run when Metrics is nil")
	}
	if out.Echo != "passthrough" {
		t.Fatalf("expected passthrough output, got %q", out.Echo)
	}
}
