package override

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

// Required and Min are one concept: every write stores them in agreement.
func TestWritesKeepRequiredAndMinInSync(t *testing.T) {
	minOnly := &domain.FieldOverride{MinOccurs: 1}
	if c := toCreateParams(minOnly); !*c.IsRequired || *c.MinOccurs != 1 {
		t.Errorf("create min 1: required %v min %d, want true 1", *c.IsRequired, *c.MinOccurs)
	}
	requiredOnly := &domain.FieldOverride{IsRequired: true}
	if u := toUpdateParams(requiredOnly); !*u.IsRequired || *u.MinOccurs != 1 {
		t.Errorf("update required: required %v min %d, want true 1", *u.IsRequired, *u.MinOccurs)
	}
	if p := placementParams(&domain.CollectionPlacement{IsRequired: true}); !p.IsRequired || p.MinOccurs != 1 {
		t.Errorf("placement required: required %v min %d, want true 1", p.IsRequired, p.MinOccurs)
	}
	if p := placementParams(&domain.CollectionPlacement{MinOccurs: 2}); !p.IsRequired || p.MinOccurs != 2 {
		t.Errorf("placement min 2: required %v min %d, want true 2", p.IsRequired, p.MinOccurs)
	}
}
