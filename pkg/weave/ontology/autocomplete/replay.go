package autocomplete

import (
	"context"
	"fmt"
	"strings"
)

// ReplayStep is one question put to the engine while rebuilding a stored
// path the way the path builder asks it: the scope class in the root picker
// (Position -1), then each element with the scope and the elements before
// it as CurrentPath.
type ReplayStep struct {
	Position    int      `json:"position"`
	Want        string   `json:"want"`
	WantURI     string   `json:"want_uri,omitempty"`
	CurrentPath []string `json:"current_path"`
}

// ReplayMiss is a step the engine did not offer.
type ReplayMiss struct {
	Position int      `json:"position"`
	Want     string   `json:"want"`
	Offered  []string `json:"offered"`
}

// Suggester answers one builder question. Both the ontology service's
// GetSuggestions and OfflineEngine.Suggest have this shape.
type Suggester func(ctx context.Context, req Request) ([]Suggestion, error)

// ReplaySteps lists the questions for one stored path. scopeURI and
// pathURIs may be empty; when given they let Offered match across prefixes.
func ReplaySteps(scope, scopeURI string, path, pathURIs []string) []ReplayStep {
	steps := make([]ReplayStep, 0, 1+len(path))
	steps = append(steps, ReplayStep{Position: -1, Want: scope, WantURI: scopeURI})
	for i, q := range path {
		cur := make([]string, 0, 1+i)
		cur = append(cur, scope)
		cur = append(cur, path[:i]...)
		step := ReplayStep{Position: i, Want: q, CurrentPath: cur}
		if i < len(pathURIs) {
			step.WantURI = pathURIs[i]
		}
		steps = append(steps, step)
	}
	return steps
}

// Offered reports whether the stored element is among the suggestions: by
// qname, or by IRI when the stored path carries another prefix for the same
// namespace than the one the builder shows.
func Offered(sugs []Suggestion, wantQname, wantURI string) bool {
	for _, s := range sugs {
		if s.Qname == wantQname {
			return true
		}
		if wantURI != "" && strings.Contains(wantURI, "://") && s.URI == wantURI {
			return true
		}
	}
	return false
}

// QnameLocal is the local part of a prefixed name (the whole name when it
// has no prefix).
func QnameLocal(q string) string {
	if i := strings.Index(q, ":"); i >= 0 {
		return q[i+1:]
	}
	return q
}

// ReplayPath asks suggest every question the builder would ask for one
// stored path and returns the steps it would not offer, plus the number of
// steps asked. Requests mirror the builder: parent projects and range
// suggestions included, the scope class set from the first step on, and
// the query the local name of the element looked for.
func ReplayPath(ctx context.Context, suggest Suggester, projectID, scope, scopeURI string, path, pathURIs []string) ([]ReplayMiss, int, error) {
	steps := ReplaySteps(scope, scopeURI, path, pathURIs)
	var misses []ReplayMiss
	for _, step := range steps {
		req := Request{
			ProjectID:               projectID,
			CurrentPath:             step.CurrentPath,
			Query:                   QnameLocal(step.Want),
			MaxResults:              200,
			IncludeParentProjects:   true,
			IncludeRangeSuggestions: true,
			Source:                  "replay",
		}
		if step.Position >= 0 {
			req.ScopeClass = scope
		}
		sugs, err := suggest(ctx, req)
		if err != nil {
			return nil, 0, fmt.Errorf("replay step %d of %s: %w", step.Position, scope, err)
		}
		if Offered(sugs, step.Want, step.WantURI) {
			continue
		}
		offered := make([]string, 0, len(sugs))
		for _, s := range sugs {
			offered = append(offered, s.Qname)
		}
		misses = append(misses, ReplayMiss{Position: step.Position, Want: step.Want, Offered: offered})
	}
	return misses, len(steps), nil
}
