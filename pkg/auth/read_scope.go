package auth

import (
	"context"

	"github.com/pletka-io/pletka/pkg/domain"
)

// ReadScope is the version a read is served at. It is either the working
// state or one named release, and there is no third possibility.
//
// The zero value is INVALID on purpose. A read that receives it has a caller
// that forgot to name a scope, and failing there — loudly, at the first call
// in development — is the whole reason this is a type rather than a string.
// The predecessor was a bare version string in which "" meant draft, so a
// forgotten value was indistinguishable from a deliberate one; that is how a
// release view came to serve draft overrides.
type ReadScope struct {
	version string
	valid   bool
}

// Draft reads the working state.
func Draft() ReadScope { return ReadScope{valid: true} }

// Release reads the archive at version. Release("") is Draft(): callers build
// a scope from a version string that is empty for the working state, and
// turning that into an invalid scope would report the mistake at the read
// rather than where it was made.
func Release(version string) ReadScope {
	if version == "" {
		return Draft()
	}
	return ReadScope{version: version, valid: true}
}

// Valid reports whether a scope was actually chosen.
func (s ReadScope) Valid() bool { return s.valid }

// IsRelease reports whether this scope reads an archive rather than the
// working state.
func (s ReadScope) IsRelease() bool { return s.valid && s.version != "" }

// Version is the release version, or "" for the working state.
func (s ReadScope) Version() string { return s.version }

// String renders the scope for error messages and logs.
func (s ReadScope) String() string {
	switch {
	case !s.valid:
		return "invalid scope"
	case s.version == "":
		return "draft"
	default:
		return "release " + s.version
	}
}

// ResolveReadScope decides which scope a request reads at. It is
// ResolveEffectiveVersion's decision expressed as a scope: an explicit
// ?version= wins, a non-public project reads hot because only members reach
// it, a public editor sees the working state, and everyone else gets the
// latest release — falling back to the working state when there is none.
func ResolveReadScope(snap *AuthSnapshot, project *domain.Project, explicitVersion, latestRelease string) ReadScope {
	return Release(ResolveEffectiveVersion(snap, project, explicitVersion, latestRelease))
}

type readScopeKey struct{}

// WithReadScope stores the request's scope on the context. Installed by the
// middleware that already resolves the version.
func WithReadScope(ctx context.Context, s ReadScope) context.Context {
	return context.WithValue(ctx, readScopeKey{}, s)
}

// ReadScopeFromContext returns the request's scope.
//
// Call this at the request boundary — an HTTP handler or an MCP tool — and
// nowhere else. Services and stores receive the scope as a parameter. A
// service reaching in here is the ambient pattern wearing a new type, and it
// drifts exactly as ProjectVersionFromContext did.
//
// An absent scope returns the invalid zero value rather than draft, so a
// handler that forgot to install one fails at its first read.
func ReadScopeFromContext(ctx context.Context) ReadScope {
	if s, ok := ctx.Value(readScopeKey{}).(ReadScope); ok {
		return s
	}
	return ReadScope{}
}
