package ids

import (
	"testing"

	"github.com/google/uuid"
)

func TestUUIDv5Deterministic(t *testing.T) {
	ns := uuid.NameSpaceURL
	first := UUIDv5(ns, "graph/OGEEM.1")
	second := UUIDv5(ns, "graph/OGEEM.1")
	if first != second {
		t.Fatalf("UUIDv5 not deterministic: %q != %q", first, second)
	}
}

func TestUUIDv5DistinctKeys(t *testing.T) {
	ns := uuid.NameSpaceURL
	a := UUIDv5(ns, "graph/OGEEM.1")
	b := UUIDv5(ns, "graph/OGEEM.2")
	if a == b {
		t.Fatalf("distinct keys produced the same UUID: %q", a)
	}
}

func TestUUIDv5DistinctNamespaces(t *testing.T) {
	a := UUIDv5(ProjectNamespace("https://linked.art/ns/ogee"), "graph/OGEEM.1")
	b := UUIDv5(ProjectNamespace("https://linked.art/ns/other"), "graph/OGEEM.1")
	if a == b {
		t.Fatalf("distinct namespaces produced the same UUID: %q", a)
	}
}

func TestUUIDv5Format(t *testing.T) {
	id := UUIDv5(uuid.NameSpaceURL, "graph/OGEEM.1")
	parsed, err := uuid.Parse(id)
	if err != nil {
		t.Fatalf("output not a valid UUID: %v", err)
	}
	if parsed.Version() != 5 {
		t.Fatalf("expected v5 UUID, got v%d", parsed.Version())
	}
	if parsed.Variant() != uuid.RFC4122 {
		t.Fatalf("expected RFC 4122 variant, got %v", parsed.Variant())
	}
}

func TestProjectNamespaceDeterministic(t *testing.T) {
	a := ProjectNamespace("https://linked.art/ns/ogee")
	b := ProjectNamespace("https://linked.art/ns/ogee")
	if a != b {
		t.Fatalf("ProjectNamespace not deterministic: %v != %v", a, b)
	}
}

// TestUUIDv5KnownVector pins one (namespace, key) → UUID pair so any future
// upstream change to the v5 algorithm fails loudly. The value was generated
// by this implementation on first run and is recorded here purely as a
// stability check.
func TestUUIDv5KnownVector(t *testing.T) {
	got := UUIDv5(uuid.NameSpaceURL, "graph/OGEEM.1/root")
	want := "393938e6-3427-5b82-b584-6a8986c074a0"
	if got != want {
		t.Fatalf("UUIDv5 vector drift: got %q want %q", got, want)
	}
}
