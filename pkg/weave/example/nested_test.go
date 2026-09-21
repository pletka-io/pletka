package example

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func TestNewServiceMaxNestingDepthDefault(t *testing.T) {
	svc := NewService(newFakeStore(), fakeViews{})
	if svc.maxDepth != defaultMaxNestingDepth {
		t.Fatalf("maxDepth = %d, want default %d", svc.maxDepth, defaultMaxNestingDepth)
	}
}

func TestNewServiceMaxNestingDepthOption(t *testing.T) {
	svc := NewService(newFakeStore(), fakeViews{}, WithMaxNestingDepth(3))
	if svc.maxDepth != 3 {
		t.Fatalf("maxDepth = %d, want 3", svc.maxDepth)
	}
}

func TestNewServiceMaxNestingDepthOptionZeroKeepsDefault(t *testing.T) {
	svc := NewService(newFakeStore(), fakeViews{}, WithMaxNestingDepth(0))
	if svc.maxDepth != defaultMaxNestingDepth {
		t.Fatalf("maxDepth = %d, want default %d", svc.maxDepth, defaultMaxNestingDepth)
	}
}

// nestedViews is the B.3 fixture: model M1 with direct field 11 and group C1
// holding string field 21, container 302 (-> LAC6), container 303 with no
// target and container 304 with two targets. LAC6 holds string 601, required
// string 602, container 603 (-> LAC1) and Model-typed 604 (-> M2); LAC1
// holds string 701.
func nestedViews() fakeViews {
	container := func(overrideID int64, fieldID string, targets ...string) domain.ResolvedField {
		f := resolvedField(overrideID, fieldID, fieldID, expectedValueTypeCollection, false, 0, nil)
		for _, id := range targets {
			f.CollectionModels = append(f.CollectionModels, domain.EntityRef{ID: id})
		}
		return f
	}
	model := resolvedField(604, "F604", "Maker", expectedValueTypeModel, false, 0, nil)
	model.ResourceModels = []domain.EntityRef{{ID: "M2"}}
	return fakeViews{
		models: map[string]*domain.ModelView{"M1": groupedModelView(nil,
			resolvedField(21, "F21", "Name", "String", false, 0, nil),
			container(302, "F302", "LAC6"),
			container(303, "F303"),
			container(304, "F304", "LAC6", "LAC1"),
		)},
		collections: map[string][]domain.ResolvedField{
			"LAC6": {
				resolvedField(601, "F601", "Begin", "String", false, 0, nil),
				resolvedField(602, "F602", "End", "String", true, 0, nil),
				container(603, "F603", "LAC1"),
				model,
			},
			"LAC1": {resolvedField(701, "F701", "Note", "String", false, 0, nil)},
		},
	}
}

func createNested(svc *Service, values ...domain.ExampleValue) (*ExampleRecord, error) {
	return svc.Create(context.Background(), "P1", CreateInput{EntityType: domain.ExampleEntityTypeModel, EntityID: "M1", Values: values})
}

func TestSlotSegments(t *testing.T) {
	cases := []struct {
		path   string
		group  string
		fields []string
		ok     bool
	}{
		{"21:0", "", []string{"21:0"}, true},
		{"C1:0/21:0", "C1:0", []string{"21:0"}, true},
		{"C1:0/302:1/601:0", "C1:0", []string{"302:1", "601:0"}, true},
		{"302:1/601:0", "", []string{"302:1", "601:0"}, true},
		{"", "", nil, false},
		{"C1:0", "", nil, false},
		{"C1:0/x/601:0", "", nil, false},
		{"C1:0/C2:0/21:0", "", nil, false},
		{"/21:0", "", nil, false},
	}
	for _, c := range cases {
		group, fields, ok := slotSegments(c.path)
		if ok != c.ok || group != c.group || !slices.Equal(fields, c.fields) {
			t.Errorf("slotSegments(%q) = %q, %v, %v; want %q, %v, %v", c.path, group, fields, ok, c.group, c.fields, c.ok)
		}
	}
}

func TestContainerPath(t *testing.T) {
	for path, want := range map[string]string{"21:0": "", "C1:0/21:0": "C1:0", "C1:0/302:1/55:0": "C1:0/302:1"} {
		if got := containerPath(path); got != want {
			t.Errorf("containerPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestSlotResolverResolvesNestedLeaf(t *testing.T) {
	views := nestedViews()
	svc := NewService(newFakeStore(), views)
	r := svc.newSlotResolver(context.Background(), "P1", views.models["M1"])
	got, err := r.resolve("C1:0/302:0/601:0")
	if err != nil {
		t.Fatalf("resolve error = %v", err)
	}
	if got.status != slotOK || got.field.OverrideID != 601 || got.anchor != 302 {
		t.Fatalf("resolve = %+v, want OK leaf 601 anchor 302", got)
	}
	got, err = r.resolve("C1:0/302:0/603:0/701:0")
	if err != nil || got.status != slotOK || got.field.OverrideID != 701 {
		t.Fatalf("resolve depth 2 = %+v, %v; want OK leaf 701", got, err)
	}
}

func TestServiceCreateStoresNestedValue(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, nestedViews())
	rec, err := createNested(svc, stringValue("C1:0/302:0/601:0", "F601", "1650"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	stored := store.values[rec.Example.ID]
	if len(stored) != 1 || stored[0].OverrideID != 601 || stored[0].FieldID != "F601" || stored[0].SlotPath != "C1:0/302:0/601:0" {
		t.Fatalf("stored values = %+v", stored)
	}
	if !rec.Validation.Valid {
		t.Fatalf("issues = %+v, want valid", rec.Validation.Issues)
	}
}

func TestServiceNestedDepthTwoAndCap(t *testing.T) {
	const path = "C1:0/302:0/603:0/701:0"
	if _, err := createNested(NewService(newFakeStore(), nestedViews()), stringValue(path, "F701", "x")); err != nil {
		t.Fatalf("Create() at depth 2 with default cap: error = %v", err)
	}
	store := newFakeStore()
	capped := NewService(store, nestedViews(), WithMaxNestingDepth(1))
	_, err := createNested(capped, stringValue(path, "F701", "x"))
	if err == nil || !strings.Contains(err.Error(), "Nesting limit reached") {
		t.Fatalf("Create() with cap 1: err = %v, want nesting limit error", err)
	}
	ex := &domain.Example{ID: "EX1", ProjectID: "P1", EntityType: domain.ExampleEntityTypeModel, EntityID: "M1"}
	if err := store.CreateWithValues(context.Background(), ex, []domain.ExampleValue{stringValue(path, "F701", "x")}); err != nil {
		t.Fatal(err)
	}
	got, err := capped.Get(context.Background(), "P1", "EX1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !hasIssue(got.Validation.Issues, "invalid_nesting", domain.ExampleIssueWarning) {
		t.Fatalf("issues = %+v, want invalid_nesting warning", got.Validation.Issues)
	}
	for _, is := range got.Validation.Issues {
		if is.Code == "invalid_nesting" && is.GroupPath != "C1:0/302:0/603:0" {
			t.Fatalf("invalid_nesting group path = %q, want container path", is.GroupPath)
		}
	}
}

func TestServiceNestedUnknownInnerOverride(t *testing.T) {
	const path = "C1:0/302:0/999:0"
	store := newFakeStore()
	svc := NewService(store, nestedViews())
	if _, err := createNested(svc, stringValue(path, "F999", "x")); err == nil {
		t.Fatal("Create() error = nil, want an error for an inner override outside the collection")
	}
	ex := &domain.Example{ID: "EX1", ProjectID: "P1", EntityType: domain.ExampleEntityTypeModel, EntityID: "M1"}
	if err := store.CreateWithValues(context.Background(), ex, []domain.ExampleValue{stringValue(path, "F999", "x")}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(context.Background(), "P1", "EX1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	found := false
	for _, is := range got.Validation.Issues {
		if is.Code == "stale_override" && is.Severity == domain.ExampleIssueWarning {
			found = true
			if is.Message["en"] != "This field is no longer part of the nested collection." || is.GroupPath != "C1:0/302:0" {
				t.Fatalf("stale_override issue = %+v", is)
			}
		}
	}
	if !found {
		t.Fatalf("issues = %+v, want stale_override warning", got.Validation.Issues)
	}
}

func TestServiceCreateRejectsInvalidContainers(t *testing.T) {
	svc := NewService(newFakeStore(), nestedViews())
	for path, want := range map[string]string{
		"C1:0/21:0/601:0":  "not a Collection field",
		"C1:0/303:0/601:0": "No target collection set",
		"C1:0/304:0/601:0": "Several target collections",
	} {
		_, err := createNested(svc, stringValue(path, "F601", "x"))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("path %s: err = %v, want %q", path, err, want)
		}
	}
}

func TestServiceCreateMaterializesStubInNestedCollection(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, nestedViews())
	v := stubValue("", "Rembrandt")
	v.OverrideID, v.FieldID, v.SlotPath = 0, "F604", "C1:0/302:0/604:0"
	rec, err := createNested(svc, v)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	stub := findStub(store, rec.Example.ID)
	if stub == nil || stub.EntityID != "M2" {
		t.Fatalf("stub = %+v, want a draft of M2", stub)
	}
	if id := rec.Values[0].ValuePayload.ExampleID; id == nil || *id != stub.ID {
		t.Fatalf("value not linked to stub: %+v", rec.Values[0].ValuePayload)
	}
}
