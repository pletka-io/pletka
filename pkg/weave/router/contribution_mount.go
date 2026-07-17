package router

import (
	"fmt"

	"github.com/go-chi/chi/v5"

	schemaregistry "github.com/pletka-io/pletka/pkg/schemaui/registry"
)

func mountProviderContribution(r chi.Router, sliceLabel, mountID string, provider schemaregistry.DescriptorProvider) {
	descriptor, err := provider.Descriptor()
	if err != nil {
		panic(fmt.Errorf("mount %s slice: %w", sliceLabel, err))
	}
	mountDescriptorContribution(r, descriptor, sliceLabel, mountID)
}

func mountDescriptorContribution(r chi.Router, descriptor schemaregistry.SliceDescriptor, sliceLabel, mountID string) {
	for _, mount := range descriptor.Contributions.Mounts {
		if mount.ID == mountID {
			mount.Mount(r)
			return
		}
	}
	panic(fmt.Errorf("mount %s slice: contribution %q not found", sliceLabel, mountID))
}
