package autocomplete

import (
	"context"
	"log/slog"
)

// Mode selects which engine backs GetSuggestions.
type Mode string

const (
	// ModeIndexed routes every request through the IndexedEngine (default).
	ModeIndexed Mode = "indexed"
	// ModeDirect routes every request through the DirectEngine (DB round-trips).
	ModeDirect Mode = "direct"
	// ModeIndexedWithFallback tries the IndexedEngine first; on error falls back
	// to the DirectEngine.
	ModeIndexedWithFallback Mode = "indexed-with-fallback"
)

// DispatchConfig selects the engine and the per-request override policy.
type DispatchConfig struct {
	Mode                    Mode
	AllowPerRequestOverride bool
}

// suggester is the one-method seam both engines satisfy. It exists so tests
// can inject fakes into DispatchEngine without DB fixtures.
// ponytail: one-method seam, justified — it's the dispatch boundary AND the test seam.
type suggester interface {
	GetSuggestions(ctx context.Context, req Request) ([]Suggestion, error)
}

// DispatchEngine routes each autocomplete request to the indexed or direct
// engine. The configured mode applies globally; super-admins may override
// it to "direct" per-request when AllowPerRequestOverride is true.
type DispatchEngine struct {
	indexed       suggester
	direct        *DirectEngine
	directSug     suggester // defaults to direct; swappable in tests
	mode          Mode
	allowOverride bool
	log           *slog.Logger
}

// NewDispatch wires the dispatcher. cfg.Mode defaults to ModeIndexed when empty.
func NewDispatch(indexed *IndexedEngine, direct *DirectEngine, cfg DispatchConfig, log *slog.Logger) *DispatchEngine {
	if log == nil {
		log = slog.Default()
	}
	mode := cfg.Mode
	if mode == "" {
		mode = ModeIndexed
	}
	return &DispatchEngine{
		indexed:       indexed,
		direct:        direct,
		directSug:     direct,
		mode:          mode,
		allowOverride: cfg.AllowPerRequestOverride,
		log:           log,
	}
}

// Direct exposes the concrete DirectEngine for surfaces that remain
// direct-only (OntologyLabels, ScopeResolver).
func (d *DispatchEngine) Direct() *DirectEngine { return d.direct }

// modeFor returns the effective Mode for req. Returns ModeDirect only when
// all three conditions hold: override is configured, the caller is a
// super-admin, and req.Source == "direct".
func (d *DispatchEngine) modeFor(req Request) Mode {
	if d.allowOverride && req.SuperAdmin && req.Source == "direct" {
		return ModeDirect
	}
	return d.mode
}

// GetSuggestions dispatches per modeFor(req).
func (d *DispatchEngine) GetSuggestions(ctx context.Context, req Request) ([]Suggestion, error) {
	switch d.modeFor(req) {
	case ModeDirect:
		return d.directSug.GetSuggestions(ctx, req)
	case ModeIndexedWithFallback:
		sug, err := d.indexed.GetSuggestions(ctx, req)
		if err != nil {
			d.log.Warn("indexed engine errored; falling back to direct", "err", err)
			return d.directSug.GetSuggestions(ctx, req)
		}
		return sug, nil
	default:
		return d.indexed.GetSuggestions(ctx, req)
	}
}
