package weave

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func TestResolveAdoptionOrigin(t *testing.T) {
	adopted := map[string]domain.Origin{
		adoptionOriginKey("LA", "LAF.11"): domain.AdoptedOrigin("LA", "LAF.11"),
	}

	got := ResolveAdoptionOrigin(adopted, "TPC", "LA", "LAF.11")
	if got.Kind != domain.OriginAdopted {
		t.Fatalf("expected adopted origin, got %#v", got)
	}

	fallback := ResolveAdoptionOrigin(adopted, "TPC", "LA", "LAF.12")
	if fallback.Kind != domain.OriginInherited {
		t.Fatalf("expected inherited fallback, got %#v", fallback)
	}
}
