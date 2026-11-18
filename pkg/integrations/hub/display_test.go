package hub_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/integrations/hub"
	"github.com/pletka-io/pletka/pkg/integrations/registry"
)

func TestService_ResolveManagedLinks_SuperAdminOnly(t *testing.T) {
	svc := hub.NewService(newFakeStore(), nil, nil, nil)
	svc.SetManagedLinksResolver(func(_ context.Context, id string) []hub.ManagedLink {
		return []hub.ManagedLink{{Label: "Admin", URL: "/admin/x/" + id}}
	})
	ctx := context.Background()

	if got := svc.ResolveManagedLinks(ctx, false, "I1"); got != nil {
		t.Fatalf("non-super-admin got links: %v", got)
	}
	got := svc.ResolveManagedLinks(ctx, true, "I1")
	if len(got) != 1 || got[0].URL != "/admin/x/I1" {
		t.Fatalf("super-admin links = %v", got)
	}
	if got := svc.ResolveManagedLinks(ctx, true, ""); got != nil {
		t.Fatalf("unmanaged config got links: %v", got)
	}
}

func TestConfigTarget(t *testing.T) {
	md := registry.Metadata{TargetField: "base_url"}
	cfg := &hub.ProjectIntegration{Config: map[string]any{"base_url": "https://a.example"}}
	if got := hub.ConfigTarget(md, cfg); got != "https://a.example" {
		t.Fatalf("target = %q", got)
	}
	if got := hub.ConfigTarget(registry.Metadata{}, cfg); got != "" {
		t.Fatalf("no TargetField should give empty, got %q", got)
	}
	managed := &hub.ProjectIntegration{ManagedInstanceID: "I1", Config: cfg.Config}
	if got := hub.ConfigTarget(md, managed); got != "" {
		t.Fatalf("managed config should defer to its badge, got %q", got)
	}
}
