// Package gorules holds go-ruleguard rules for gocritic's "ruleguard" check
// (wired via .golangci.yml linters.settings.gocritic). This file is loaded
// directly by golangci-lint's ruleguard engine, never compiled as part of
// the normal build — see the go:build tag below.
//
// go-ruleguard docs: https://github.com/quasilyte/go-ruleguard
//
//go:build ruleguard

package gorules

import "github.com/quasilyte/go-ruleguard/dsl"

// authResourceLiteral forbids constructing auth.Resource{...} composite
// literals outside pkg/auth. Project/org auth resources must be built via
// auth.ProjectResource / auth.OrgResource / auth.OrgResourceByID, which
// correctly populate OrgID and default Visibility — a bare literal
// silently drops OrgID and/or diverges on the Visibility default,
// reintroducing the bug class fixed by the auth-resource-consistency
// effort (Tasks A-F).
func authResourceLiteral(m dsl.Matcher) {
	// Anchored to end-of-path so this exempts only the auth package proper
	// (github.com/pletka-io/pletka/pkg/auth), its external test variant
	// (…/pkg/auth_test, produced for "package auth_test" files), and this
	// ruleguard subpackage (…/pkg/auth/ruleguard) — not an unrelated
	// package that merely starts with "auth" (e.g. a hypothetical
	// pkg/authfoo) or contains "auth" deeper in its path.
	m.Match(`auth.Resource{$*_}`).
		Where(!m.File().PkgPath.Matches(`pletka-io/pletka/pkg/auth(_test|/ruleguard)?$`)).
		Report(`construct auth resources via auth.ProjectResource(p); org via auth.OrgResource/OrgResourceByID — never a bare auth.Resource{} literal (drops OrgID / diverges on Visibility)`)
}
