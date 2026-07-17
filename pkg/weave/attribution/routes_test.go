package attribution

import (
	"context"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	schemaregistry "github.com/pletka-io/pletka/pkg/schemaui/registry"
)

type descriptorProjectReader struct{}

func (descriptorProjectReader) GetByID(context.Context, string) (*domain.Project, error) {
	return nil, nil
}

func attributionDescriptorHost() Host {
	return Host{
		Store:    NewPostgresStore(nil),
		Projects: descriptorProjectReader{},
	}
}

func TestHostValidateRejectsMissingDependencies(t *testing.T) {
	err := (Host{}).Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want missing dependency error")
	}
	for _, name := range []string{"Store", "Projects"} {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("Validate() error = %q, want missing %s", err, name)
		}
	}
}

func TestDescriptorDeclaresAttributionContributions(t *testing.T) {
	desc, err := (Slice{Host: attributionDescriptorHost()}).Descriptor()
	if err != nil {
		t.Fatalf("Descriptor() error = %v", err)
	}

	if desc.ID != SliceID {
		t.Fatalf("descriptor ID = %q, want %q", desc.ID, SliceID)
	}
	if err := desc.Validate(); err != nil {
		t.Fatalf("descriptor should validate: %v", err)
	}
	if len(desc.Contributions.Mounts) != 1 {
		t.Fatalf("mount contributions = %d, want 1", len(desc.Contributions.Mounts))
	}

	mount := desc.Contributions.Mounts[0]
	if mount.ID != "attribution.routes" {
		t.Errorf("mount ID = %q, want attribution.routes", mount.ID)
	}
	if mount.Surface != schemaregistry.SurfaceSettings {
		t.Errorf("mount surface = %q, want %q", mount.Surface, schemaregistry.SurfaceSettings)
	}
	if mount.Pattern != "/projects/{projectID}/settings/attributions" {
		t.Errorf("mount pattern = %q", mount.Pattern)
	}
	if mount.Mount == nil {
		t.Fatal("mount function should be present")
	}
}
