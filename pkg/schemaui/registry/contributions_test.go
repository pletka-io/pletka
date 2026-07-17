package registry

import (
	"context"
	"errors"
	"testing"
)

type testProvider struct {
	descriptor SliceDescriptor
	err        error
}

func (p testProvider) Descriptor() (SliceDescriptor, error) {
	if p.err != nil {
		return SliceDescriptor{}, p.err
	}
	return p.descriptor, nil
}

func TestSliceRegistryRegistersDescriptors(t *testing.T) {
	r, err := NewSliceRegistry(SliceDescriptor{
		ID: "category",
		Contributions: Contributions{
			Widgets: []WidgetContribution{{Kind: "category-list", Component: "CategoryList"}},
		},
	})
	if err != nil {
		t.Fatalf("NewSliceRegistry() error = %v", err)
	}

	descriptor, ok := r.Descriptor("category")
	if !ok {
		t.Fatal("Descriptor(category) not found")
	}
	if descriptor.ID != "category" {
		t.Fatalf("descriptor ID = %q, want category", descriptor.ID)
	}

	descriptors := r.Descriptors()
	if len(descriptors) != 1 {
		t.Fatalf("Descriptors() len = %d, want 1", len(descriptors))
	}
}

func TestSliceRegistryRegistersProviders(t *testing.T) {
	r, err := NewSliceRegistryFromProviders(testProvider{
		descriptor: SliceDescriptor{ID: "category"},
	})
	if err != nil {
		t.Fatalf("NewSliceRegistryFromProviders() error = %v", err)
	}
	if _, ok := r.Descriptor("category"); !ok {
		t.Fatal("Descriptor(category) not found")
	}
}

func TestSliceRegistrySurfacesProviderErrors(t *testing.T) {
	want := errors.New("missing host dependency")
	_, err := NewSliceRegistryFromProviders(testProvider{err: want})
	if !errors.Is(err, want) {
		t.Fatalf("NewSliceRegistryFromProviders() error = %v, want %v", err, want)
	}
}

func TestSliceRegistryRejectsDuplicateDescriptors(t *testing.T) {
	_, err := NewSliceRegistry(
		SliceDescriptor{ID: "category"},
		SliceDescriptor{ID: "category"},
	)
	if err == nil {
		t.Fatal("NewSliceRegistry() error = nil, want duplicate error")
	}
}

func TestSliceRegistryFiltersActiveDescriptors(t *testing.T) {
	r, err := NewSliceRegistry(
		SliceDescriptor{ID: "category"},
		SliceDescriptor{ID: "model"},
	)
	if err != nil {
		t.Fatalf("NewSliceRegistry() error = %v", err)
	}

	active, err := r.ActiveDescriptors(context.Background(), ActivationContext{
		Features: map[string]bool{"category": true},
	}, ActivationPolicyFunc(func(_ context.Context, d SliceDescriptor, req ActivationContext) (ActivationDecision, error) {
		return ActivationDecision{Active: req.Features[d.ID]}, nil
	}))
	if err != nil {
		t.Fatalf("ActiveDescriptors() error = %v", err)
	}
	if len(active) != 1 || active[0].ID != "category" {
		t.Fatalf("ActiveDescriptors() = %#v, want only category", active)
	}
}

func TestSliceRegistryAggregatesActiveContributions(t *testing.T) {
	r, err := NewSliceRegistry(
		SliceDescriptor{
			ID: "category",
			Contributions: Contributions{
				AdminSections: []AdminSection{{ID: "categories", Order: 20}},
				Settings:      []SettingsPanel{{ID: "category-settings", Order: 30}},
			},
		},
		SliceDescriptor{
			ID: "model",
			Contributions: Contributions{
				AdminSections: []AdminSection{{ID: "models", Order: 10}},
				Settings:      []SettingsPanel{{ID: "model-settings", Order: 5}},
			},
		},
	)
	if err != nil {
		t.Fatalf("NewSliceRegistry() error = %v", err)
	}

	contributions, err := r.ActiveContributions(context.Background(), ActivationContext{}, nil)
	if err != nil {
		t.Fatalf("ActiveContributions() error = %v", err)
	}
	if len(contributions.AdminSections) != 2 {
		t.Fatalf("AdminSections len = %d, want 2", len(contributions.AdminSections))
	}
	if contributions.AdminSections[0].ID != "models" {
		t.Fatalf("first admin section = %q, want models", contributions.AdminSections[0].ID)
	}
	if contributions.Settings[0].ID != "model-settings" {
		t.Fatalf("first settings panel = %q, want model-settings", contributions.Settings[0].ID)
	}
}
