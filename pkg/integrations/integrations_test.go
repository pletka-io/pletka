package integrations_test

import (
	"context"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/integrations"
	"github.com/pletka-io/pletka/pkg/integrations/registry"
)

func TestCoreIntegrations(t *testing.T) {
	got := integrationIDs(integrations.CoreIntegrations())
	want := []string{"threem"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("CoreIntegrations IDs mismatch (-want +got):\n%s", diff)
	}
}

func TestRegistryConstructors(t *testing.T) {
	if _, err := integrations.CoreRegistry(); err != nil {
		t.Fatalf("core registry rejected integration set: %v", err)
	}
}

func TestBuildRegistryStillComposesCoreWithExtras(t *testing.T) {
	reg, err := integrations.BuildRegistry(stubIntegration{id: "extra"})
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	got := integrationIDs(reg.All())
	want := []string{"extra", "threem"}
	sort.Strings(got)
	sort.Strings(want)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("BuildRegistry IDs mismatch (-want +got):\n%s", diff)
	}
}

type stubIntegration struct {
	id string
}

func (s stubIntegration) ID() string                   { return s.id }
func (s stubIntegration) Metadata() registry.Metadata  { return registry.Metadata{} }
func (s stubIntegration) AppliesTo() []registry.Format { return nil }
func (s stubIntegration) SecretFields() []string       { return nil }
func (s stubIntegration) ConfigSchema(registry.ConfigRequest, map[string]any) *formschema.FormSchema {
	return nil
}
func (s stubIntegration) ValidateConfig(raw map[string]any) (map[string]any, map[string][]string) {
	return raw, nil
}
func (s stubIntegration) Actions() []registry.ActionSpec { return nil }
func (s stubIntegration) RunAction(context.Context, string, registry.ActionInput) (registry.ActionResult, error) {
	return registry.ActionResult{}, nil
}

func integrationIDs(integs []registry.Integration) []string {
	out := make([]string, 0, len(integs))
	for _, integ := range integs {
		out = append(out, integ.ID())
	}
	return out
}
