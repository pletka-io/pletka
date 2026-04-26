// Package gitsync provides bidirectional synchronisation between Pletka's
// domain entities and a git repository: domain state can be exported to git
// (snapshot, materialise) and imported back from git (rehydrate, replay).
//
// Each pluggable per-domain syncer (gitsync/weave, gitsync/ontology, ...)
// implements the interfaces declared here. Cross-domain operations should
// compose syncers at the call site rather than entangling the implementations.
package gitsync

import "context"

// Ref names a target inside a Repo: typically a branch or tag, but
// implementations may interpret it more loosely (commit SHA, pseudo-ref).
type Ref string

// Repo identifies a git repository the syncer should read from or write to.
// Implementations decide whether the repo is local-clone-on-disk, in-memory,
// or remote-only via a gitprovider client.
type Repo struct {
	URL    string
	Branch Ref
	// Path is the optional working-copy path on disk. Empty means the
	// implementation is free to clone into a managed cache directory.
	Path string
}

// Exporter writes domain state into a git repo at a given ref.
//
// Export is expected to be idempotent: running it twice with no intervening
// domain changes must produce no commits.
type Exporter interface {
	Export(ctx context.Context, repo Repo, ref Ref) error
}

// Importer reads a git repo at a given ref and applies the encoded domain
// state to the underlying store.
//
// Import is expected to be idempotent: importing the same ref twice must
// converge to the same domain state.
type Importer interface {
	Import(ctx context.Context, repo Repo, ref Ref) error
}

// Syncer combines Exporter and Importer for domains that participate in
// both directions. Most callers should depend on the narrow interfaces
// rather than Syncer directly.
type Syncer interface {
	Exporter
	Importer
}
