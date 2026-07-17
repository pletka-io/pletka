// Package buildinfo exposes the platform's build identity — version,
// git commit, branch, dirty flag, build timestamp. Variables here are
// the targets for `-ldflags "-X github.com/pletka-io/pletka/pkg/buildinfo.X=..."`
// at build time. The Makefile and deploy scripts populate them.
//
// The package is a leaf with no internal imports so any slice or
// command can read build metadata without forming a cycle.
package buildinfo

// These vars get rewritten by `go build -ldflags "-X ..."`. Defaults
// are the values when running `go run` locally or during tests.
var (
	// Version is the semver / release tag of the platform.
	Version = "dev"

	// GitCommit is the short SHA of HEAD at build time.
	GitCommit = "unknown"

	// GitBranch is the branch name HEAD was on at build time.
	GitBranch = "unknown"

	// GitDirty is "dirty" when the working tree had uncommitted
	// changes at build time, empty string otherwise.
	GitDirty = ""

	// BuildTime is RFC3339 UTC, set at build time.
	BuildTime = "unknown"
)

// Info is the snapshot returned by Get(). Pure data, no methods —
// callers JSON-encode it directly.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Branch    string `json:"branch,omitempty"`
	Dirty     bool   `json:"dirty"`
	BuiltAt   string `json:"built_at"`
}

// Get returns a snapshot of the current build identity. Cheap; safe
// to call per request.
func Get() Info {
	return Info{
		Version: Version,
		Commit:  GitCommit,
		Branch:  GitBranch,
		Dirty:   GitDirty != "",
		BuiltAt: BuildTime,
	}
}
