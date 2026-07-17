package registry_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/integrations/registry"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

type fakeIntegration struct {
	id      string
	formats []registry.Format
}

func (f *fakeIntegration) ID() string                  { return f.id }
func (f *fakeIntegration) Metadata() registry.Metadata { return registry.Metadata{} }
func (f *fakeIntegration) AppliesTo() []registry.Format {
	return f.formats
}
func (f *fakeIntegration) SecretFields() []string { return nil }
func (f *fakeIntegration) ConfigSchema(registry.ConfigRequest, map[string]any) *formschema.FormSchema {
	return nil
}
func (f *fakeIntegration) ValidateConfig(raw map[string]any) (map[string]any, map[string][]string) {
	return raw, nil
}
func (f *fakeIntegration) Actions() []registry.ActionSpec { return nil }
func (f *fakeIntegration) RunAction(context.Context, string, registry.ActionInput) (registry.ActionResult, error) {
	return registry.ActionResult{}, nil
}

func TestNewRegistry_EmptyOK(t *testing.T) {
	r, err := registry.NewRegistry()
	if err != nil {
		t.Fatalf("empty: %v", err)
	}
	if got := r.All(); len(got) != 0 {
		t.Fatalf("want 0 integrations, got %d", len(got))
	}
}

func TestNewRegistry_NilSkipped(t *testing.T) {
	r, err := registry.NewRegistry(nil, &fakeIntegration{id: "a"}, nil)
	if err != nil {
		t.Fatalf("nil skip: %v", err)
	}
	if _, ok := r.Integration("a"); !ok {
		t.Fatalf("a not registered after nil-skip")
	}
}

func TestNewRegistry_RejectsEmptyID(t *testing.T) {
	_, err := registry.NewRegistry(&fakeIntegration{id: ""})
	if err == nil {
		t.Fatalf("want empty-id error, got nil")
	}
}

func TestNewRegistry_RejectsDuplicateID(t *testing.T) {
	_, err := registry.NewRegistry(
		&fakeIntegration{id: "threem"},
		&fakeIntegration{id: "threem"},
	)
	if err == nil {
		t.Fatalf("want duplicate-id error, got nil")
	}
}

func TestRegistry_NilSafeLookups(t *testing.T) {
	var r *registry.Registry
	if integ, ok := r.Integration("x"); integ != nil || ok {
		t.Fatalf("nil Integration() returned %v, %v", integ, ok)
	}
	if got := r.All(); got != nil {
		t.Fatalf("nil All() returned %v", got)
	}
	if got := r.ForFormat("x3ml-b"); got != nil {
		t.Fatalf("nil ForFormat() returned %v", got)
	}
}

func TestRegistry_ForFormatIntersection(t *testing.T) {
	a := &fakeIntegration{id: "a", formats: []registry.Format{"x3ml", "x3ml-b"}}
	b := &fakeIntegration{id: "b", formats: []registry.Format{"turtle"}}
	c := &fakeIntegration{id: "c", formats: nil}

	r, err := registry.NewRegistry(a, b, c)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	cases := []struct {
		name string
		fmt  registry.Format
		want []string
	}{
		{"x3ml-b matches a", "x3ml-b", []string{"a"}},
		{"turtle matches b", "turtle", []string{"b"}},
		{"json matches none", "json", nil},
		{"empty format returns nil", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := idsOf(r.ForFormat(tc.fmt))
			less := func(x, y string) bool { return x < y }
			if diff := cmp.Diff(tc.want, got, cmpopts.SortSlices(less), cmpopts.EquateEmpty()); diff != "" {
				t.Fatalf("ForFormat(%q) mismatch (-want +got):\n%s", tc.fmt, diff)
			}
		})
	}
}

func TestRegistry_AllReturnsEverything(t *testing.T) {
	a := &fakeIntegration{id: "a"}
	b := &fakeIntegration{id: "b"}
	r, err := registry.NewRegistry(a, b)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	got := idsOf(r.All())
	less := func(x, y string) bool { return x < y }
	if diff := cmp.Diff([]string{"a", "b"}, got, cmpopts.SortSlices(less)); diff != "" {
		t.Fatalf("All() mismatch (-want +got):\n%s", diff)
	}
}

func idsOf(integs []registry.Integration) []string {
	if integs == nil {
		return nil
	}
	out := make([]string, 0, len(integs))
	for _, i := range integs {
		out = append(out, i.ID())
	}
	return out
}
