// Package conformance holds build-failing checks on the read model.
//
// The invariant: a read in a versioned slice names the version it reads at.
// A versioned slice is one owning a table with an _archive counterpart, so
// the set is derived from the schema rather than from judgment.
//
// The allowlist (allowlist.go) is the phase-out plan -- visible, shrinking,
// reviewable in a diff. It has permanent members and that is correct: some
// slices audit the working state by design, some compare live against a
// release as their entire job, and mutations always run hot. The rule is
// that every entry names why, not that the list reaches zero.
//
// What these checks cannot see: they read signatures and file contents, not
// behavior. A read that takes a scope and ignores it passes. The signature is
// what makes an unscoped read unrepresentable at the call site, which is the
// property the design asks for; whether the body honors it is the slice's own
// tests' job.
//
// A permanent exemption covers the slice, never its callers. A slice being
// exempt because comparing live against a release is its job says nothing
// about whether a caller should be asking it while rendering a release, so a
// permanent entry carries that obligation explicitly or it reads as blanket
// permission.
package conformance
