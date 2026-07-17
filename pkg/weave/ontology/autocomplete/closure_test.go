package autocomplete

import (
	"sort"
	"testing"
)

func qnames(ns []*Node) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.Qname
	}
	sort.Strings(out)
	return out
}

// graph: A -> B -> D, A -> C -> D  (D reached twice), plus a cycle D -> A.
func testGraph() *Node {
	a := &Node{Qname: "A"}
	b := &Node{Qname: "B"}
	c := &Node{Qname: "C"}
	d := &Node{Qname: "D"}
	a.Subclasses = []*Node{b, c}
	b.Subclasses = []*Node{d}
	c.Subclasses = []*Node{d}
	d.Subclasses = []*Node{a} // cycle
	return a
}

func TestClosure_VisitsEachNodeOnce_TolerantOfCycles(t *testing.T) {
	got := qnames(closure(testGraph(),
		func(n *Node) []*Node { return n.Subclasses }, ClosureOpts{}))
	want := []string{"B", "C", "D"} // root A excluded from output, but all descendants included
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("want %v, got %v", want, got)
		}
	}
}

func TestClosure_IncludeFiltersOutputButWalkContinues(t *testing.T) {
	// Exclude B from output but its child D must still surface.
	got := qnames(closure(testGraph(),
		func(n *Node) []*Node { return n.Subclasses },
		ClosureOpts{Include: func(n *Node) bool { return n.Qname != "B" }}))
	for _, q := range got {
		if q == "B" {
			t.Fatalf("B should be excluded from output, got %v", got)
		}
	}
	hasD := false
	for _, q := range got {
		if q == "D" {
			hasD = true
		}
	}
	if !hasD {
		t.Fatalf("D (child of excluded B) must still surface, got %v", got)
	}
}

func TestClosure_CutStopsSubtreeTraversal(t *testing.T) {
	// Cut at B: B not emitted AND B's subtree not walked. D still reachable via C.
	got := qnames(closure(testGraph(),
		func(n *Node) []*Node { return n.Subclasses },
		ClosureOpts{Cut: func(n *Node) bool { return n.Qname == "B" }}))
	for _, q := range got {
		if q == "B" {
			t.Fatalf("cut node B must not appear, got %v", got)
		}
	}
	// D reachable through C, so present; if only reachable through B it would be absent.
	hasD := false
	for _, q := range got {
		if q == "D" {
			hasD = true
		}
	}
	if !hasD {
		t.Fatalf("D reachable via C must still surface, got %v", got)
	}
}
