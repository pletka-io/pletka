package category

import (
	"context"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/schemaui/registry"
)

type descriptorSchemaStore struct{}

func (descriptorSchemaStore) GetByID(context.Context, string) (*domain.Category, error) {
	return nil, nil
}

type descriptorAdoptionStore struct{}

func (descriptorAdoptionStore) List(context.Context, ...domain.QueryOption) ([]domain.Adoption, error) {
	return nil, nil
}

func (descriptorAdoptionStore) ReplaceForContext(context.Context, string, string, string, []domain.Adoption) error {
	return nil
}

type descriptorNumberer struct{}

func (descriptorNumberer) AllocateEntityNumber(context.Context, string, string) (int64, error) {
	return 1, nil
}

func descriptorHost() Host {
	return Host{
		Store:       NewPostgresStore(nil),
		SchemaStore: descriptorSchemaStore{},
		Adoptions:   descriptorAdoptionStore{},
		Numberer:    descriptorNumberer{},
	}
}

func TestHostValidateRejectsMissingDependencies(t *testing.T) {
	err := (Host{}).Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want missing dependency error")
	}
	for _, name := range []string{"Store", "SchemaStore", "Adoptions", "Numberer"} {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("Validate() error = %q, want missing %s", err, name)
		}
	}
}

func TestDescriptorDeclaresCategoryContributions(t *testing.T) {
	desc, err := (Slice{Host: descriptorHost()}).Descriptor()
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
	if mount.ID != "category.routes" {
		t.Errorf("mount ID = %q, want category.routes", mount.ID)
	}
	if mount.Surface != registry.SurfacePublic {
		t.Errorf("mount surface = %q, want %q", mount.Surface, registry.SurfacePublic)
	}
	if mount.Pattern != "/projects/{projectID}/categories" {
		t.Errorf("mount pattern = %q", mount.Pattern)
	}
	if mount.Mount == nil {
		t.Fatal("mount function should be present")
	}
}

func TestDescriptorDeclaresSchemaContribution(t *testing.T) {
	desc, err := (Slice{Host: descriptorHost()}).Descriptor()
	if err != nil {
		t.Fatalf("Descriptor() error = %v", err)
	}

	if len(desc.Contributions.Schemas) != 1 {
		t.Fatalf("schema contributions = %d, want 1", len(desc.Contributions.Schemas))
	}
	schema := desc.Contributions.Schemas[0]
	if schema.EntityType != SliceID {
		t.Errorf("schema entity type = %q, want %q", schema.EntityType, SliceID)
	}
	if schema.Form == nil {
		t.Fatal("schema form provider should be present")
	}
}
