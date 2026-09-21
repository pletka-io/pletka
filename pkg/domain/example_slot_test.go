package domain

import "testing"

func TestExampleSlotRoundTrip(t *testing.T) {
	if got := ExampleSlot(21, 0); got != "21:0" {
		t.Fatalf("ExampleSlot = %q", got)
	}
	cases := []struct {
		path  string
		oid   int64
		occ   int
		ok    bool
		depth int
	}{
		{"21:0", 21, 0, true, 1},
		{"302:1/415:2", 415, 2, true, 2},
		{"", 0, 0, false, 0},
		{"abc", 0, 0, false, 1},
		{"21", 0, 0, false, 1},
		{"21:x", 0, 0, false, 1},
		{"21:-1", 0, 0, false, 1},
		{"+21:0", 0, 0, false, 1},
		{"021:0", 0, 0, false, 1},
		{"21:+0", 0, 0, false, 1},
		{"0:0", 0, 0, false, 1},
	}
	for _, c := range cases {
		oid, occ, ok := ParseExampleSlotLeaf(c.path)
		if oid != c.oid || occ != c.occ || ok != c.ok {
			t.Errorf("ParseExampleSlotLeaf(%q) = (%d,%d,%v), want (%d,%d,%v)", c.path, oid, occ, ok, c.oid, c.occ, c.ok)
		}
		if d := ExampleSlotDepth(c.path); d != c.depth {
			t.Errorf("ExampleSlotDepth(%q) = %d, want %d", c.path, d, c.depth)
		}
	}
}

func TestExampleGroupSegment(t *testing.T) {
	if got := ExampleGroupSegment("LAC.1", 2); got != "LAC.1:2" {
		t.Fatalf("ExampleGroupSegment = %q", got)
	}
	cases := []struct {
		in       string
		coll     string
		instance int
		ok       bool
	}{
		{"LAC.1:0", "LAC.1", 0, true},
		{"LAC.1:12", "LAC.1", 12, true},
		{"C1:3", "C1", 3, true},
		{":0", "", 0, false},
		{"LAC.1", "", 0, false},
		{"LAC.1:", "", 0, false},
		{"LAC.1:-1", "", 0, false},
		{"LAC.1:+1", "", 0, false},
		{"LAC.1:01", "", 0, false},
		{"LAC.1:x", "", 0, false},
		{"a/b:0", "", 0, false},
	}
	for _, c := range cases {
		coll, n, ok := ParseExampleGroupSegment(c.in)
		if coll != c.coll || n != c.instance || ok != c.ok {
			t.Errorf("ParseExampleGroupSegment(%q) = (%q, %d, %v), want (%q, %d, %v)", c.in, coll, n, ok, c.coll, c.instance, c.ok)
		}
	}
}
