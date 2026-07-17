package namespacebinding

import (
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/schemaui/registry"
)

func namespaceBindingDescriptorHost() Host {
	return Host{Store: NewPostgresStore(nil)}
}

func TestHostValidateRejectsMissingDependencies(t *testing.T) {
	err := (Host{}).Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want missing dependency error")
	}
	if !strings.Contains(err.Error(), "Store") {
		t.Fatalf("Validate() error = %q, want missing Store", err)
	}
}

func TestDescriptorDeclaresNamespaceBindingContributions(t *testing.T) {
	desc, err := (Slice{Host: namespaceBindingDescriptorHost()}).Descriptor()
	if err != nil {
		t.Fatalf("Descriptor() error = %v", err)
	}

	if desc.ID != SliceID {
		t.Fatalf("descriptor ID = %q, want %q", desc.ID, SliceID)
	}
	if err := desc.Validate(); err != nil {
		t.Fatalf("descriptor should validate: %v", err)
	}
	if len(desc.Contributions.Mounts) != 2 {
		t.Fatalf("mount contributions = %d, want 2", len(desc.Contributions.Mounts))
	}

	byID := map[string]registry.MountSpec{}
	for _, mount := range desc.Contributions.Mounts {
		byID[mount.ID] = mount
	}

	publicMount, ok := byID["namespacebinding.routes"]
	if !ok {
		t.Fatal("namespacebinding.routes mount not found")
	}
	if publicMount.Surface != registry.SurfacePublic {
		t.Errorf("public mount surface = %q, want %q", publicMount.Surface, registry.SurfacePublic)
	}
	if publicMount.Pattern != "/projects/{projectID}/namespace-bindings" {
		t.Errorf("public mount pattern = %q", publicMount.Pattern)
	}
	if publicMount.Mount == nil {
		t.Fatal("public mount function should be present")
	}

	adminMount, ok := byID["namespacebinding.admin.routes"]
	if !ok {
		t.Fatal("namespacebinding.admin.routes mount not found")
	}
	if adminMount.Surface != registry.SurfaceAdmin {
		t.Errorf("admin mount surface = %q, want %q", adminMount.Surface, registry.SurfaceAdmin)
	}
	if adminMount.Pattern != "/admin/namespaces" {
		t.Errorf("admin mount pattern = %q", adminMount.Pattern)
	}
	if adminMount.Mount == nil {
		t.Fatal("admin mount function should be present")
	}
}
