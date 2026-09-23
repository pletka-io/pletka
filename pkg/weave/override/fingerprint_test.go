package override

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func fpRow(id int64, field, cat, coll string, pos int) domain.FieldOverride {
	return domain.FieldOverride{
		ID: id, FieldID: field, CategoryID: cat, PartOfCollectionID: coll, Position: pos,
		DisplayName: domain.Translations{"en": field},
	}
}

func TestFingerprintIgnoresIDsAndOrder(t *testing.T) {
	a := []domain.FieldOverride{fpRow(1, "F1", "C", "", 1), fpRow(2, "F2", "C", "", 2)}
	b := []domain.FieldOverride{fpRow(99, "F2", "C", "", 2), fpRow(98, "F1", "C", "", 1)}
	if Fingerprint(a, nil, nil) != Fingerprint(b, nil, nil) {
		t.Fatal("fingerprint changed with ids/order only")
	}
}

func TestFingerprintChangesWithContent(t *testing.T) {
	base := []domain.FieldOverride{fpRow(1, "F1", "C", "", 1)}
	want := Fingerprint(base, nil, nil)

	renamed := []domain.FieldOverride{fpRow(1, "F1", "C", "", 1)}
	renamed[0].DisplayName = domain.Translations{"en": "other"}
	moved := []domain.FieldOverride{fpRow(1, "F1", "OTHER", "", 1)}
	repositioned := []domain.FieldOverride{fpRow(1, "F1", "C", "", 7)}
	required := []domain.FieldOverride{fpRow(1, "F1", "C", "", 1)}
	required[0].IsRequired = true
	max := 2
	bounded := []domain.FieldOverride{fpRow(1, "F1", "C", "", 1)}
	bounded[0].MaxOccurs = &max

	for name, rows := range map[string][]domain.FieldOverride{
		"renamed": renamed, "moved": moved, "repositioned": repositioned,
		"required": required, "bounded": bounded,
	} {
		if Fingerprint(rows, nil, nil) == want {
			t.Errorf("%s: fingerprint unchanged", name)
		}
	}
}

func TestFingerprintCoversRefsAndPlacements(t *testing.T) {
	rows := []domain.FieldOverride{fpRow(1, "F1", "C", "", 1)}
	base := Fingerprint(rows, nil, nil)
	withRef := Fingerprint(rows, map[int64][]domain.OverrideRef{1: {{RefType: "resource_model", TargetID: "M1", Position: 1}}}, nil)
	if withRef == base {
		t.Fatal("refs not covered")
	}
	otherTarget := Fingerprint(rows, map[int64][]domain.OverrideRef{1: {{RefType: "resource_model", TargetID: "M2", Position: 1}}}, nil)
	if otherTarget == withRef {
		t.Fatal("ref target not covered")
	}
	one := 1
	withPlacement := Fingerprint(rows, nil, []domain.CollectionPlacement{{CategoryID: "C", CollectionID: "COL", MinOccurs: 1, MaxOccurs: &one}})
	if withPlacement == base {
		t.Fatal("placements not covered")
	}
}

func TestFingerprintStableAcrossCalls(t *testing.T) {
	rows := []domain.FieldOverride{fpRow(1, "F1", "C", "", 1), fpRow(2, "F2", "C", "COL", 2)}
	refs := map[int64][]domain.OverrideRef{2: {{RefType: "collection_model", TargetID: "COL2", Position: 1}}}
	first := Fingerprint(rows, refs, nil)
	second := Fingerprint(rows, refs, nil)
	if first != second {
		t.Fatalf("not deterministic: %s != %s", first, second)
	}
}
