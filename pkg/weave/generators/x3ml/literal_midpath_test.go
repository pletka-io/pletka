package x3ml

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/weave/generators"
)

// TestBuildLinkFromPathStopsAtAMidPathLiteral is the regression gate for an
// ineffective break. The walk over the middle steps carries the comment
// "Literal mid-path is malformed; stop emitting and leave terminal handling to
// render whatever the snapshot carries" — but its `break` sat inside a
// `switch`, so it broke the switch rather than the loop, and every node after
// the literal kept appending steps. staticcheck SA4011 flagged it as
// "ineffective break statement. Did you mean to break out of the outer loop?"
//
// A literal mid-path is only reachable from a malformed stored path, which is
// exactly what `pletka weave verify-paths` exists to find — so the generator
// meeting one is not hypothetical, and emitting a longer relation for it is
// worse than emitting a truncated one, because the extra steps look deliberate.
func TestBuildLinkFromPathStopsAtAMidPathLiteral(t *testing.T) {
	node := func(role generators.PathRole, localName string, position int) generators.PathNode {
		return generators.PathNode{
			Element: pe(elementTypeFor(role), "crm", localName, position),
			Index:   position,
			Role:    role,
		}
	}

	// class → property → LITERAL → property → class.
	// Everything from the literal onward must be dropped from the emitted
	// relation; without the fix the two trailing steps are appended anyway.
	steps := []generators.PathNode{
		node(generators.PathRoleClass, "E22_Human-Made_Object", 0),
		node(generators.PathRoleProperty, "P43_has_dimension", 1),
		node(generators.PathRoleLiteral, "rdfs_Literal", 2),
		node(generators.PathRoleProperty, "P90_has_value", 3),
		node(generators.PathRoleClass, "E54_Dimension", 4),
	}

	link, ok := buildLinkFromPath(steps, "dimension_value", "root")
	if !ok {
		t.Fatalf("buildLinkFromPath returned ok=false, want a link to inspect")
	}

	// The middle walk covers steps[:len-1]; the terminal is handled separately.
	// Steps emitted from the middle walk must stop at the literal, so only the
	// class and property before it may appear.
	for _, step := range link.Path.TargetRelation.Steps {
		if step.Text == "crm:P90_has_value" {
			t.Errorf("relation contains %q, emitted after a mid-path literal — the walk did not stop", step.Text)
		}
	}
}

// elementTypeFor maps a path role onto the element type the snapshot would
// carry for it, so the fixture is shaped like real data rather than only
// satisfying the switch under test.
func elementTypeFor(role generators.PathRole) string {
	switch role {
	case generators.PathRoleProperty:
		return "property"
	case generators.PathRoleLiteral:
		return "literal"
	default:
		return "class"
	}
}
