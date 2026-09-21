package example

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
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
	// 602 is required in every TimeSpan instance (B.3 task 3), so the
	// instance carries it to stay valid.
	rec, err := createNested(svc, stringValue("C1:0/302:0/601:0", "F601", "1650"), stringValue("C1:0/302:0/602:0", "F602", "1660"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	stored := store.values[rec.Example.ID]
	if len(stored) != 2 || stored[0].OverrideID != 601 || stored[0].FieldID != "F601" || stored[0].SlotPath != "C1:0/302:0/601:0" {
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

// nestedModelField returns a pointer to model M1's field slot overrideID in
// the fixture, for tests that tweak its cardinality.
func nestedModelField(views fakeViews, overrideID int64) *domain.ResolvedField {
	for ci := range views.models["M1"].Categories[0].Collections {
		coll := &views.models["M1"].Categories[0].Collections[ci]
		for fi := range coll.Fields {
			if coll.Fields[fi].OverrideID == overrideID {
				return &coll.Fields[fi]
			}
		}
	}
	return nil
}

func slotPaths(values []domain.ExampleValue) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, v.SlotPath)
	}
	slices.Sort(out)
	return out
}

// issuesFor returns the issues with code on overrideID.
func issuesFor(issues []domain.ExampleIssue, code string, overrideID int64) []domain.ExampleIssue {
	var out []domain.ExampleIssue
	for _, is := range issues {
		if is.Code == code && is.OverrideID != nil && *is.OverrideID == overrideID {
			out = append(out, is)
		}
	}
	return out
}

// getStored stores values verbatim (no normalization) and reads them back
// through Get, the lenient path.
func getStored(t *testing.T, svc *Service, store *fakeStore, values ...domain.ExampleValue) *ExampleRecord {
	t.Helper()
	ex := &domain.Example{ID: "EX1", ProjectID: "P1", EntityType: domain.ExampleEntityTypeModel, EntityID: "M1"}
	if err := store.CreateWithValues(context.Background(), ex, values); err != nil {
		t.Fatal(err)
	}
	rec, err := svc.Get(context.Background(), "P1", "EX1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	return rec
}

func TestServiceCompactsNestedInstances(t *testing.T) {
	rec, err := createNested(NewService(newFakeStore(), nestedViews()),
		stringValue("C1:0/302:0/601:0", "F601", "1650"),
		stringValue("C1:0/302:3/601:0", "F601", "1700"),
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	want := []string{"C1:0/302:0/601:0", "C1:0/302:1/601:0"}
	if got := slotPaths(rec.Values); !slices.Equal(got, want) {
		t.Fatalf("slot paths = %v, want %v", got, want)
	}
}

func TestServiceNestedRequiredPerInstance(t *testing.T) {
	rec, err := createNested(NewService(newFakeStore(), nestedViews()),
		stringValue("C1:0/302:0/601:0", "F601", "1650"),
		stringValue("C1:0/302:0/602:0", "F602", "1660"),
		stringValue("C1:0/302:1/601:0", "F601", "1700"),
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	got := issuesFor(rec.Validation.Issues, "missing_required_value", 602)
	if len(got) != 1 || got[0].GroupPath != "C1:0/302:1" || got[0].Severity != domain.ExampleIssueError {
		t.Fatalf("missing_required_value on 602 = %+v, want one error at C1:0/302:1", got)
	}

	rec, err = createNested(NewService(newFakeStore(), nestedViews()), stringValue("C1:0/21:0", "F21", "x"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	for _, is := range rec.Validation.Issues {
		if is.OverrideID != nil && *is.OverrideID == 602 {
			t.Fatalf("issue on 602 without a TimeSpan instance: %+v", is)
		}
	}
}

func TestServiceNestedContainerCardinality(t *testing.T) {
	views := nestedViews()
	one := 1
	nestedModelField(views, 302).MaxOccurs = &one
	rec, err := createNested(NewService(newFakeStore(), views),
		stringValue("C1:0/302:0/602:0", "F602", "a"),
		stringValue("C1:0/302:1/602:0", "F602", "b"),
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	got := issuesFor(rec.Validation.Issues, "max_occurs", 302)
	if len(got) != 1 || got[0].GroupPath != "C1:0" {
		t.Fatalf("max_occurs on 302 = %+v, want one at C1:0", got)
	}

	views = nestedViews()
	nestedModelField(views, 302).IsRequired = true
	rec, err = createNested(NewService(newFakeStore(), views), stringValue("C1:0/21:0", "F21", "x"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	got = issuesFor(rec.Validation.Issues, "missing_required_value", 302)
	if len(got) != 1 || got[0].GroupPath != "C1:0" {
		t.Fatalf("missing_required_value on 302 = %+v, want one at C1:0", got)
	}

	rec, err = createNested(NewService(newFakeStore(), views), stringValue("C1:0/302:0/602:0", "F602", "a"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got := issuesFor(rec.Validation.Issues, "missing_required_value", 302); len(got) != 0 {
		t.Fatalf("missing_required_value on 302 with one TimeSpan = %+v, want none", got)
	}
}

func TestServiceCompactsGroupAndNestedLevels(t *testing.T) {
	rec, err := createNested(NewService(newFakeStore(), nestedViews()),
		stringValue("C1:1/302:2/601:0", "F601", "1650"),
		stringValue("C1:1/302:2/603:4/701:0", "F701", "n"),
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	want := []string{"C1:0/302:0/601:0", "C1:0/302:0/603:0/701:0"}
	if got := slotPaths(rec.Values); !slices.Equal(got, want) {
		t.Fatalf("slot paths = %v, want %v", got, want)
	}
}

// A nested value whose inner override left the collection is not a resolved
// value: it keeps neither a group instance nor a nested instance alive and
// is left exactly where it is stored.
func TestServiceStaleNestedValueDoesNotCountOrMove(t *testing.T) {
	store := newFakeStore()
	rec := getStored(t, NewService(store, nestedViews()), store,
		stringValue("C1:1/21:0", "F21", "x"),
		stringValue("C1:3/302:2/999:0", "F999", "stale"),
	)
	want := []string{"C1:0/21:0", "C1:3/302:2/999:0"}
	if got := slotPaths(rec.Values); !slices.Equal(got, want) {
		t.Fatalf("slot paths = %v, want %v", got, want)
	}

	store = newFakeStore()
	rec = getStored(t, NewService(store, nestedViews()), store,
		stringValue("C1:0/302:0/999:0", "F999", "stale"),
		stringValue("C1:0/302:1/602:0", "F602", "1700"),
	)
	want = []string{"C1:0/302:0/602:0", "C1:0/302:0/999:0"}
	if got := slotPaths(rec.Values); !slices.Equal(got, want) {
		t.Fatalf("slot paths = %v, want %v", got, want)
	}
}

// A container that cannot open (no target collection) can never be filled,
// so it is never reported as missing.
func TestServiceRequiredContainerWithoutTargetNotMissing(t *testing.T) {
	views := nestedViews()
	nestedModelField(views, 303).IsRequired = true
	nestedModelField(views, 303).MinOccurs = 1
	rec, err := createNested(NewService(newFakeStore(), views), stringValue("C1:0/21:0", "F21", "x"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	for _, code := range []string{"missing_required_value", "min_occurs"} {
		if got := issuesFor(rec.Validation.Issues, code, 303); len(got) != 0 {
			t.Fatalf("%s on 303 = %+v, want none", code, got)
		}
	}
}

// A container nested past the depth cap cannot open either; below the cap
// the same required container is reported per nested instance.
func TestServiceRequiredContainerPastCapNotMissing(t *testing.T) {
	values := []domain.ExampleValue{
		stringValue("C1:0/302:0/601:0", "F601", "1650"),
		stringValue("C1:0/302:0/602:0", "F602", "1660"),
	}
	views := nestedViews()
	views.collections["LAC6"][2].IsRequired = true // 603 -> LAC1
	rec, err := createNested(NewService(newFakeStore(), views), values...)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got := issuesFor(rec.Validation.Issues, "missing_required_value", 603); len(got) != 1 || got[0].GroupPath != "C1:0/302:0" {
		t.Fatalf("default cap: missing_required_value on 603 = %+v, want one at C1:0/302:0", got)
	}
	rec, err = createNested(NewService(newFakeStore(), views, WithMaxNestingDepth(1)), values...)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got := issuesFor(rec.Validation.Issues, "missing_required_value", 603); len(got) != 0 {
		t.Fatalf("cap 1: missing_required_value on 603 = %+v, want none", got)
	}
}

func nestedFormSchema(t *testing.T, svc *Service, mode, exampleID string) *ExampleFormSchema {
	t.Helper()
	schema, err := svc.BuildFormSchema(context.Background(), "P1", mode, string(domain.ExampleEntityTypeModel), "M1", exampleID, "en", nil)
	if err != nil {
		t.Fatalf("BuildFormSchema() error = %v", err)
	}
	return schema
}

// formField returns field overrideID of the entry for group groupID
// instance instance in schema, or fails.
func formField(t *testing.T, schema *ExampleFormSchema, groupID string, instance int, overrideID int64) ExampleFormField {
	t.Helper()
	for _, sec := range schema.Sections {
		for _, g := range sec.Groups {
			if g.ID != groupID || g.Instance != instance {
				continue
			}
			for _, f := range g.Fields {
				if f.OverrideID == overrideID {
					return f
				}
			}
		}
	}
	t.Fatalf("field %d not found in %s instance %d", overrideID, groupID, instance)
	return ExampleFormField{}
}

func overrideIDs(fields []ExampleFormField) []int64 {
	out := make([]int64, 0, len(fields))
	for _, f := range fields {
		out = append(out, f.OverrideID)
	}
	return out
}

func TestBuildFormSchemaNestedTemplate(t *testing.T) {
	schema := nestedFormSchema(t, NewService(newFakeStore(), nestedViews()), formschema.ModeCreate, "")
	f := formField(t, schema, "C1", 0, 302)
	if f.Widget != widgetNestedCollection || f.Occurrences != nil || f.NestedInstances != nil {
		t.Fatalf("302 = widget %q occurrences %+v instances %+v, want nested-collection without either", f.Widget, f.Occurrences, f.NestedInstances)
	}
	n := f.Nested
	if n == nil || !n.Expandable || n.CollectionID != "LAC6" || n.Note != "" {
		t.Fatalf("302 nested = %+v, want expandable LAC6", n)
	}
	if got := overrideIDs(n.Fields); !slices.Equal(got, []int64{601, 602, 603, 604}) {
		t.Fatalf("302 template fields = %v", got)
	}
	for _, tf := range n.Fields {
		if tf.SlotPrefix != "" {
			t.Fatalf("template field %d slot_prefix = %q, want empty", tf.OverrideID, tf.SlotPrefix)
		}
	}
	inner := n.Fields[2].Nested
	if n.Fields[2].Widget != "nested-collection" || inner == nil || !inner.Expandable || inner.CollectionID != "LAC1" {
		t.Fatalf("603 nested = %+v, want expandable LAC1", inner)
	}
	if got := overrideIDs(inner.Fields); !slices.Equal(got, []int64{701}) {
		t.Fatalf("603 template fields = %v", got)
	}
	if n.Fields[0].Nested != nil {
		t.Fatalf("601 nested = %+v, want none", n.Fields[0].Nested)
	}

	capped := nestedFormSchema(t, NewService(newFakeStore(), nestedViews(), WithMaxNestingDepth(1)), formschema.ModeCreate, "")
	inner = formField(t, capped, "C1", 0, 302).Nested.Fields[2].Nested
	if inner == nil || inner.Expandable || inner.Note != noteNestingLimit || len(inner.Fields) != 0 {
		t.Fatalf("capped 603 nested = %+v, want not expandable with nesting limit note", inner)
	}
}

func TestBuildFormSchemaNestedInstances(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, nestedViews())
	rec, err := createNested(svc,
		stringValue("C1:0/302:0/601:0", "F601", "1650"),
		stringValue("C1:0/302:0/602:0", "F602", "1660"),
		stringValue("C1:0/302:1/601:0", "F601", "1700"),
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	f := formField(t, nestedFormSchema(t, svc, formschema.ModeEdit, rec.Example.ID), "C1", 0, 302)
	if len(f.NestedInstances) != 2 {
		t.Fatalf("302 nested_instances = %+v, want 2", f.NestedInstances)
	}
	for k, want := range []string{"1650", "1700"} {
		in := f.NestedInstances[k]
		prefix := "C1:0/302:" + string(rune('0'+k)) + "/"
		if in.ID != "LAC6" || in.Instance != k || in.SlotPrefix != prefix || !in.Repeatable {
			t.Fatalf("instance %d = %+v, want LAC6 prefix %s", k, in, prefix)
		}
		if got := overrideIDs(in.Fields); !slices.Equal(got, []int64{601, 602, 603, 604}) {
			t.Fatalf("instance %d fields = %v", k, got)
		}
		v := in.Fields[0]
		if v.SlotPrefix != prefix || len(v.Occurrences) != 1 || v.Occurrences[0].Value.StringValue == nil || *v.Occurrences[0].Value.StringValue != want {
			t.Fatalf("instance %d field 601 = %+v, want %s at %s", k, v, want, prefix)
		}
	}
	// 602 is required per instance: the issue lands in instance 1 only.
	if got := f.NestedInstances[0].Fields[1].Issues; len(got) != 0 {
		t.Fatalf("instance 0 field 602 issues = %+v, want none", got)
	}
	if got := f.NestedInstances[1].Fields[1].Issues; len(got) != 1 || got[0].Code != "missing_required_value" {
		t.Fatalf("instance 1 field 602 issues = %+v, want missing_required_value", got)
	}
	if f.NestedInstances[1].Fields[2].Nested == nil || f.NestedInstances[1].Fields[2].NestedInstances != nil {
		t.Fatalf("instance 1 field 603 = %+v, want template and no instances", f.NestedInstances[1].Fields[2])
	}
}

func TestBuildFormSchemaNestedDepthTwoAndContainerIssues(t *testing.T) {
	views := nestedViews()
	one := 1
	nestedModelField(views, 302).MaxOccurs = &one
	store := newFakeStore()
	svc := NewService(store, views)
	rec, err := createNested(svc,
		stringValue("C1:0/302:0/602:0", "F602", "a"),
		stringValue("C1:0/302:1/602:0", "F602", "b"),
		stringValue("C1:0/302:1/603:0/701:0", "F701", "deep"),
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	f := formField(t, nestedFormSchema(t, svc, formschema.ModeEdit, rec.Example.ID), "C1", 0, 302)
	if len(f.Issues) != 1 || f.Issues[0].Code != "max_occurs" {
		t.Fatalf("302 issues = %+v, want max_occurs", f.Issues)
	}
	if len(f.NestedInstances) != 2 {
		t.Fatalf("302 nested_instances = %d, want 2", len(f.NestedInstances))
	}
	c603 := f.NestedInstances[1].Fields[2]
	if len(c603.NestedInstances) != 1 || c603.NestedInstances[0].ID != "LAC1" || c603.NestedInstances[0].SlotPrefix != "C1:0/302:1/603:0/" {
		t.Fatalf("603 nested_instances = %+v", c603.NestedInstances)
	}
	leaf := c603.NestedInstances[0].Fields[0]
	if leaf.SlotPrefix != "C1:0/302:1/603:0/" || leaf.Occurrences[0].Value.StringValue == nil || *leaf.Occurrences[0].Value.StringValue != "deep" {
		t.Fatalf("701 = %+v, want deep", leaf)
	}
	if got := f.NestedInstances[0].Fields[2].NestedInstances; got != nil {
		t.Fatalf("instance 0 field 603 nested_instances = %+v, want none", got)
	}
}

func TestBuildFormSchemaContainerWithoutTarget(t *testing.T) {
	views := nestedViews()
	nestedModelField(views, 303).IsRequired = true
	nestedModelField(views, 303).MinOccurs = 1
	schema := nestedFormSchema(t, NewService(newFakeStore(), views), formschema.ModeCreate, "")
	f := formField(t, schema, "C1", 0, 303)
	if f.Widget != widgetNestedCollection || f.Nested == nil || f.Nested.Expandable || f.Nested.Note != noteNoTarget || f.Nested.Fields != nil {
		t.Fatalf("303 = widget %q nested %+v, want not expandable with note", f.Widget, f.Nested)
	}
	if f.Required || f.MinOccurs != 0 {
		t.Fatalf("303 required=%v min_occurs=%d, want false/0 (a container that cannot open is never required)", f.Required, f.MinOccurs)
	}
	if f := formField(t, schema, "C1", 0, 304); f.Nested == nil || f.Nested.Expandable || f.Nested.Note != noteSeveralTargets {
		t.Fatalf("304 nested = %+v, want several targets note", f.Nested)
	}
}

// A stored value without a slot path reads as its depth-0 slot placed in
// its group, not as the malformed path "C1:0/".
func TestServiceGetPlacesValueWithoutSlotPath(t *testing.T) {
	store := newFakeStore()
	v := stringValue("", "F21", "x")
	v.OverrideID = 21
	rec := getStored(t, NewService(store, nestedViews()), store, v)
	if got := rec.Values[0].SlotPath; got != "C1:0/21:0" {
		t.Fatalf("slot path = %q, want C1:0/21:0", got)
	}
	if hasIssue(rec.Validation.Issues, "invalid_nesting", domain.ExampleIssueWarning) {
		t.Fatalf("issues = %+v, want no invalid_nesting", rec.Validation.Issues)
	}
}

// ownedViews wraps fakeViews with the weave store's Collections() lookup:
// owners maps a collection id to its owning project, and CollectionView in
// any other project returns no fields, as the real override query does. It
// records the project every CollectionView call was made with, and counts
// ModelView and CollectionView calls.
type ownedViews struct {
	fakeViews
	owners      map[string]string
	projects    map[string][]string
	modelCalls  int
	collections map[string]int
}

func newOwnedViews(owners map[string]string) *ownedViews {
	return &ownedViews{fakeViews: nestedViews(), owners: owners, projects: map[string][]string{}, collections: map[string]int{}}
}

func (v *ownedViews) ModelView(ctx context.Context, modelID, projectID string) (*domain.ModelView, error) {
	v.modelCalls++
	return v.fakeViews.ModelView(ctx, modelID, projectID)
}

func (v *ownedViews) CollectionView(ctx context.Context, collectionID, projectID string) ([]domain.ResolvedField, error) {
	v.projects[collectionID] = append(v.projects[collectionID], projectID)
	v.collections[collectionID]++
	if owner, ok := v.owners[collectionID]; ok && owner != projectID {
		return nil, nil // the override query filters on the owning project
	}
	return v.fakeViews.CollectionView(ctx, collectionID, projectID)
}

func (v *ownedViews) Collections() domain.WeaveCollectionStore {
	return ownerStore{owners: v.owners}
}

// ownerStore answers GetByID from owners; every other method panics.
type ownerStore struct {
	domain.WeaveCollectionStore
	owners map[string]string
}

func (s ownerStore) GetByID(_ context.Context, id string) (*domain.Collection, error) {
	owner, ok := s.owners[id]
	if !ok {
		return nil, nil
	}
	c := &domain.Collection{}
	c.ID, c.ProjectID = id, owner
	return c, nil
}

// A container's target collection owned by another project (LA) is read in
// that project, not the example's (P1).
func TestServiceReadsNestedCollectionInOwningProject(t *testing.T) {
	views := newOwnedViews(map[string]string{"LAC6": "LA"})
	rec, err := createNested(NewService(newFakeStore(), views),
		stringValue("C1:0/302:0/601:0", "F601", "1650"),
		stringValue("C1:0/302:0/602:0", "F602", "1660"),
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !rec.Validation.Valid {
		t.Fatalf("issues = %+v, want valid", rec.Validation.Issues)
	}
	for _, p := range views.projects["LAC6"] {
		if p != "LA" {
			t.Fatalf("CollectionView(LAC6) projects = %v, want only LA", views.projects["LAC6"])
		}
	}
	if len(views.projects["LAC6"]) == 0 {
		t.Fatal("CollectionView(LAC6) never called")
	}
}

// A target collection without fields cannot open: the container is not
// expandable, carries a note, and is never required.
func TestServiceTargetWithoutFieldsNotExpandable(t *testing.T) {
	views := nestedViews()
	views.collections["LAC6"] = nil
	nestedModelField(views, 302).IsRequired = true
	nestedModelField(views, 302).MinOccurs = 1
	svc := NewService(newFakeStore(), views)
	f := formField(t, nestedFormSchema(t, svc, formschema.ModeCreate, ""), "C1", 0, 302)
	if f.Nested == nil || f.Nested.Expandable || f.Nested.Note != "Target collection has no fields" {
		t.Fatalf("302 nested = %+v, want not expandable with note", f.Nested)
	}
	rec, err := createNested(svc, stringValue("C1:0/21:0", "F21", "x"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	for _, code := range []string{"missing_required_value", "min_occurs"} {
		if got := issuesFor(rec.Validation.Issues, code, 302); len(got) != 0 {
			t.Fatalf("%s on 302 = %+v, want none", code, got)
		}
	}
}

// One slot resolver per request: a Create, and an edit-mode form build,
// read the model view once and each collection view once.
func TestServiceOneResolverPerRequest(t *testing.T) {
	views := newOwnedViews(nil)
	store := newFakeStore()
	svc := NewService(store, views)
	stub := stubValue("", "Rembrandt")
	stub.OverrideID, stub.FieldID, stub.SlotPath = 0, "F604", "C1:0/302:0/604:0"
	rec, err := createNested(svc,
		stringValue("C1:0/302:0/602:0", "F602", "1660"),
		stringValue("C1:0/302:0/603:0/701:0", "F701", "x"),
		stub,
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	check := func(what string) {
		t.Helper()
		if views.modelCalls != 1 || views.collections["LAC6"] != 1 || views.collections["LAC1"] != 1 {
			t.Fatalf("%s: ModelView calls = %d, CollectionView calls = %v; want 1 each", what, views.modelCalls, views.collections)
		}
	}
	check("Create")
	views.modelCalls, views.collections = 0, map[string]int{}
	nestedFormSchema(t, svc, formschema.ModeEdit, rec.Example.ID)
	check("edit form")
}

// A write that fails on a bad nested path creates no stub draft, even when
// another value carries a valid typed label.
func TestServiceBadNestedPathCreatesNoStubDraft(t *testing.T) {
	store := newFakeStore()
	stub := stubValue("", "Rembrandt")
	stub.OverrideID, stub.FieldID, stub.SlotPath = 0, "F604", "C1:0/302:0/604:0"
	_, err := createNested(NewService(store, nestedViews()), stub, stringValue("C1:0/302:0/999:0", "F999", "x"))
	if err == nil || !strings.Contains(err.Error(), "does not resolve") {
		t.Fatalf("Create() error = %v, want an unresolvable slot_path error", err)
	}
	if len(store.examples) != 0 {
		t.Fatalf("examples = %+v, want no draft created", store.examples)
	}
}
