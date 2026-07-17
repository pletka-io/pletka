package router

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	schemaregistry "github.com/pletka-io/pletka/pkg/schemaui/registry"
)

func TestMountDescriptorContributionPanicsWhenMissing(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic for missing contribution")
		}
		if !strings.Contains(fmt.Sprint(recovered), `contribution "missing.routes" not found`) {
			t.Fatalf("panic = %v, want missing contribution message", recovered)
		}
	}()

	mountDescriptorContribution(chi.NewRouter(), schemaregistry.SliceDescriptor{ID: "test"}, "test", "missing.routes")
}

func TestMountDescriptorContributionMountsMatchingID(t *testing.T) {
	mounted := false
	mountDescriptorContribution(
		chi.NewRouter(),
		schemaregistry.SliceDescriptor{
			ID: "test",
			Contributions: schemaregistry.Contributions{
				Mounts: []schemaregistry.MountSpec{
					{
						ID: "test.routes",
						Mount: func(chi.Router) {
							mounted = true
						},
					},
				},
			},
		},
		"test",
		"test.routes",
	)

	if !mounted {
		t.Fatal("expected matching contribution to mount")
	}
}
