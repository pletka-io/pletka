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
	want := Fingerprint([]domain.FieldOverride{fpRow(1, "F1", "C", "", 1)}, nil, nil)
	max := 2

	for name, mutate := range map[string]func(*domain.FieldOverride){
		"field":            func(r *domain.FieldOverride) { r.FieldID = "F2" },
		"category":         func(r *domain.FieldOverride) { r.CategoryID = "OTHER" },
		"collection":       func(r *domain.FieldOverride) { r.PartOfCollectionID = "COL" },
		"position":         func(r *domain.FieldOverride) { r.Position = 7 },
		"collection_order": func(r *domain.FieldOverride) { r.CollectionOrder = 3 },
		"display_name":     func(r *domain.FieldOverride) { r.DisplayName = domain.Translations{"en": "other"} },
		"description":      func(r *domain.FieldOverride) { r.Description = domain.Translations{"en": "note"} },
		"collection_name":  func(r *domain.FieldOverride) { r.CollectionName = domain.Translations{"en": "Birth"} },
		"set_value":        func(r *domain.FieldOverride) { r.SetValue = "fixed" },
		"is_required":      func(r *domain.FieldOverride) { r.IsRequired = true },
		"min_occurs":       func(r *domain.FieldOverride) { r.MinOccurs = 1 },
		"max_occurs":       func(r *domain.FieldOverride) { r.MaxOccurs = &max },
		"is_hidden":        func(r *domain.FieldOverride) { r.IsHidden = true },
		"visibility":       func(r *domain.FieldOverride) { r.Visibility = "internal" },
	} {
		row := fpRow(1, "F1", "C", "", 1)
		mutate(&row)
		if Fingerprint([]domain.FieldOverride{row}, nil, nil) == want {
			t.Errorf("%s: fingerprint unchanged", name)
		}
	}
}

func TestFingerprintSeparatorsCannotBeFaked(t *testing.T) {
	split := []domain.FieldOverride{fpRow(1, "F1", "cat", "a|b", 1)}
	shifted := []domain.FieldOverride{fpRow(1, "F1", "cat|a", "b", 1)}
	if Fingerprint(split, nil, nil) == Fingerprint(shifted, nil, nil) {
		t.Error("a pipe inside an id fakes a field boundary")
	}

	oneRow := []domain.FieldOverride{fpRow(1, "F1", "C", "", 1)}
	oneRow[0].SetValue = "first\n\"F2\"|\"C\"|\"\"|1"
	twoRows := []domain.FieldOverride{fpRow(1, "F1", "C", "", 1), fpRow(2, "F2", "C", "", 1)}
	if Fingerprint(oneRow, nil, nil) == Fingerprint(twoRows, nil, nil) {
		t.Error("a newline inside a set value fakes a row boundary")
	}
}

func TestFingerprintChangesWithPlacementContent(t *testing.T) {
	base := domain.CollectionPlacement{CategoryID: "C", CollectionID: "COL"}
	want := Fingerprint(nil, nil, []domain.CollectionPlacement{base})
	max := 2

	for name, mutate := range map[string]func(*domain.CollectionPlacement){
		"category":    func(p *domain.CollectionPlacement) { p.CategoryID = "OTHER" },
		"collection":  func(p *domain.CollectionPlacement) { p.CollectionID = "COL2" },
		"is_required": func(p *domain.CollectionPlacement) { p.IsRequired = true },
		"min_occurs":  func(p *domain.CollectionPlacement) { p.MinOccurs = 1 },
		"max_occurs":  func(p *domain.CollectionPlacement) { p.MaxOccurs = &max },
		"is_hidden":   func(p *domain.CollectionPlacement) { p.IsHidden = true },
	} {
		placement := base
		mutate(&placement)
		if Fingerprint(nil, nil, []domain.CollectionPlacement{placement}) == want {
			t.Errorf("%s: fingerprint unchanged", name)
		}
	}
}

func TestFingerprintChangesWithRefContent(t *testing.T) {
	rows := []domain.FieldOverride{fpRow(1, "F1", "C", "", 1)}
	ref := domain.OverrideRef{RefType: "resource_model", TargetID: "M1", Position: 1}
	want := Fingerprint(rows, map[int64][]domain.OverrideRef{1: {ref}}, nil)

	for name, mutate := range map[string]func(*domain.OverrideRef){
		"ref_type":  func(r *domain.OverrideRef) { r.RefType = "collection_model" },
		"target_id": func(r *domain.OverrideRef) { r.TargetID = "M2" },
		"position":  func(r *domain.OverrideRef) { r.Position = 2 },
	} {
		changed := ref
		mutate(&changed)
		if Fingerprint(rows, map[int64][]domain.OverrideRef{1: {changed}}, nil) == want {
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
