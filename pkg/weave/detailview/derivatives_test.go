package detailview

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/weave/generators"
)

// TestDerivativesOmitUnavailableFormats proves that derivative URLs backed
// by a generator renderer are emitted only when that renderer is registered
// in the build. Asserted on marshaled JSON so omitempty is part of the
// contract under test: an absent renderer must drop the field, not emit "".
func TestDerivativesOmitUnavailableFormats(t *testing.T) {
	ctx := context.Background()
	// Superadmin so every role/capability gate passes — this test isolates
	// the format-availability gate, not the auth gates.
	snap := &auth.AuthSnapshot{IsSuperAdmin: true}
	projectResource := auth.Resource{ID: "LA"}

	// coreOnly rejects the three platform-only formats.
	coreOnly := func(f generators.Format) bool {
		return f != generators.FormatSHACL &&
			f != generators.FormatArches &&
			f != generators.FormatCytoscape
	}
	allFormats := func(generators.Format) bool { return true }

	marshalKeys := func(t *testing.T, hasFormat func(generators.Format) bool) map[string]json.RawMessage {
		t.Helper()
		h := &Handler{hasFormat: hasFormat}
		cap := h.derivativesFor(ctx, snap, projectResource, "models", "01ABC", "")
		raw, err := json.Marshal(cap)
		if err != nil {
			t.Fatalf("marshal derivatives: %v", err)
		}
		var keys map[string]json.RawMessage
		if err := json.Unmarshal(raw, &keys); err != nil {
			t.Fatalf("unmarshal derivatives: %v", err)
		}
		return keys
	}

	t.Run("core-only build omits platform format urls", func(t *testing.T) {
		keys := marshalKeys(t, coreOnly)
		for _, k := range []string{"shacl_url", "arches_url", "cytoscape_url"} {
			if _, ok := keys[k]; ok {
				t.Errorf("expected %q absent when renderer unavailable, got present", k)
			}
		}
		// Core formats stay present.
		for _, k := range []string{"turtle_url", "jsonld_url", "diagram_url"} {
			if _, ok := keys[k]; !ok {
				t.Errorf("expected core format %q present, got absent", k)
			}
		}
	})

	t.Run("full build emits platform format urls", func(t *testing.T) {
		keys := marshalKeys(t, allFormats)
		for _, k := range []string{"shacl_url", "arches_url", "cytoscape_url"} {
			if _, ok := keys[k]; !ok {
				t.Errorf("expected %q present when renderer available, got absent", k)
			}
		}
	})
}
