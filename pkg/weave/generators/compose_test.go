package generators

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

// tsName is the path of a stub on a Timespan collection that holds a Name:
// P4 → E52 → P1 → E33_E41.
func tsName() []domain.PathElement {
	return []domain.PathElement{
		pe("property", "crm", "P4_has_time-span", 0), pe("class", "crm", "E52_Time-Span", 1),
		pe("property", "crm", "P1_is_identified_by", 2), pe("class", "crm", "E33_E41_Linguistic_Appellation", 3),
	}
}

func nameCollection(id, semanticID string) ReferencedCollection {
	fields := []domain.ResolvedField{
		field("f-content-"+id, semanticID+".F1", "name_content", 1,
			pe("property", "crm", "P1_is_identified_by", 0), pe("class", "crm", "E33_E41_Linguistic_Appellation", 1),
			pe("property", "crm", "P190_has_symbolic_content", 2), pe("literal", "xsd", "string", 3)),
		field("f-lang-"+id, semanticID+".F2", "name_language", 2,
			pe("property", "crm", "P1_is_identified_by", 0), pe("class", "crm", "E33_E41_Linguistic_Appellation", 1),
			pe("property", "crm", "P72_has_language", 2), pe("class", "crm", "E56_Language", 3)),
	}
	return ReferencedCollection{
		Collection:       &domain.Collection{Entity: domain.Entity{ID: id, SemanticID: semanticID, SystemName: "name", UIName: domain.Translations{"en": "Name"}}},
		Fields:           fields,
		SharedPathPrefix: graftPrefix(fields),
	}
}

func stubField(id, semanticID, name string, path []domain.PathElement, refs ...string) domain.ResolvedField {
	f := field(id, semanticID, name, 1, path...)
	f.ExpectedValueType = "Collection"
	for _, r := range refs {
		f.CollectionModels = append(f.CollectionModels, domain.EntityRef{ID: r, SemanticID: r})
	}
	return f
}

func stubNode(f domain.ResolvedField, collectionID string) FieldNode {
	return FieldNode{Field: f, CollectionID: collectionID, CollectionSemanticID: collectionID, CollectionSystemName: "timespan"}
}

func composeOpts() Options { return Options{ComposeCollections: true, ComposeMaxDepth: 10} }

func graftedOf(out []FieldNode) []FieldNode {
	var g []FieldNode
	for _, f := range out {
		if len(f.GraftPath) > 0 {
			g = append(g, f)
		}
	}
	return g
}

func TestComposeGraftsReferencedCollectionOnly(t *testing.T) {
	stub := stubNode(stubField("s", "LAF.30", "timespan_name", tsName(), "c-name"), "c-ts")
	referenced := map[string]ReferencedCollection{
		"c-name":  nameCollection("c-name", "LAC.1"),
		"c-other": nameCollection("c-other", "LAC.99"), // same anchor class, not referenced
	}
	out := composeCollections([]FieldNode{stub}, referenced, nil, composeOpts(), &Report{})
	g := graftedOf(out)
	if len(g) != 2 {
		t.Fatalf("grafted %d fields, want the 2 of c-name", len(g))
	}
	for _, f := range g {
		if f.CollectionID != "c-name" || f.GraftStubID != "LAF.30" || f.GraftPath[0] != "timespan_name" {
			t.Fatalf("grafted field: %+v", f)
		}
	}
}

func TestComposeStubWithoutRefGraftsNothing(t *testing.T) {
	stub := stubNode(stubField("s", "LAF.30", "timespan_name", tsName()), "c-ts")
	out := composeCollections([]FieldNode{stub}, map[string]ReferencedCollection{"c-name": nameCollection("c-name", "LAC.1")}, nil, composeOpts(), &Report{})
	if len(out) != 1 {
		t.Fatalf("a stub without a ref grafted %d fields", len(out)-1)
	}
}

func TestComposeUnresolvedRefWarns(t *testing.T) {
	stub := stubNode(stubField("s", "LAF.30", "timespan_name", tsName(), "c-gone"), "c-ts")
	rep := &Report{}
	out := composeCollections([]FieldNode{stub}, map[string]ReferencedCollection{}, nil, composeOpts(), rep)
	if len(out) != 1 || len(rep.Warnings) != 1 || rep.Warnings[0].Code != "compose.unresolved_ref" {
		t.Fatalf("out %d, warnings %+v", len(out), rep.Warnings)
	}
}

func TestComposeNestedStubRecurses(t *testing.T) {
	// Activity → activity_timespan (stub, refs Timespan) → timespan_name (stub
	// inside Timespan, refs Name) → name_content
	tsFields := []domain.ResolvedField{
		field("f-begin", "LAC.6.F1", "timespan_begin", 1,
			pe("property", "crm", "P4_has_time-span", 0), pe("class", "crm", "E52_Time-Span", 1),
			pe("property", "crm", "P82a_begin_of_the_begin", 2), pe("literal", "xsd", "dateTime", 3)),
		stubField("f-tsname", "LAC.6.F2", "timespan_name", tsName(), "c-name"),
	}
	referenced := map[string]ReferencedCollection{
		"c-ts":   {Collection: &domain.Collection{Entity: domain.Entity{ID: "c-ts", SemanticID: "LAC.6", SystemName: "timespan"}}, Fields: tsFields, SharedPathPrefix: graftPrefix(tsFields)},
		"c-name": nameCollection("c-name", "LAC.1"),
	}
	stub := stubNode(stubField("s", "OGF.1", "activity_timespan", []domain.PathElement{
		pe("property", "crm", "P4_has_time-span", 0), pe("class", "crm", "E52_Time-Span", 1),
	}, "c-ts"), "c-act")
	out := composeCollections([]FieldNode{stub}, referenced, nil, composeOpts(), &Report{})
	var content *FieldNode
	for i := range out {
		if out[i].Field.SemanticID == "LAC.1.F1" {
			content = &out[i]
		}
	}
	if content == nil {
		t.Fatal("name_content not grafted through the nested stub")
	}
	if got := content.GraftPath; len(got) != 2 || got[0] != "activity_timespan" || got[1] != "name" {
		t.Fatalf("graft path %v, want [activity_timespan name]", got)
	}
	if content.GraftStubID != "LAC.6.F2" || content.CollectionID != "c-name" {
		t.Fatalf("nested graft: stub %q collection %q", content.GraftStubID, content.CollectionID)
	}
}

func TestComposeCycleStops(t *testing.T) {
	// c-a holds a stub to c-b, c-b holds a stub to c-a; the model's stub
	// sits in c-a and refs c-b: c-b is grafted once, c-a never.
	path := []domain.PathElement{pe("property", "crm", "P67i_is_referred_to_by", 0), pe("class", "crm", "E33_Linguistic_Object", 1)}
	mk := func(id, sem, ref string) ReferencedCollection {
		fs := []domain.ResolvedField{stubField("f-"+id, sem+".F1", id+"_part", path, ref)}
		return ReferencedCollection{Collection: &domain.Collection{Entity: domain.Entity{ID: id, SemanticID: sem, SystemName: id}}, Fields: fs, SharedPathPrefix: graftPrefix(fs)}
	}
	referenced := map[string]ReferencedCollection{"c-a": mk("c-a", "A", "c-b"), "c-b": mk("c-b", "B", "c-a")}
	stub := stubNode(stubField("s", "M.F1", "statement", path, "c-b"), "c-a")
	out := composeCollections([]FieldNode{stub}, referenced, nil, composeOpts(), &Report{})
	counts := map[string]int{}
	for _, f := range graftedOf(out) {
		counts[f.CollectionID]++
	}
	if counts["c-b"] != 1 || counts["c-a"] != 0 {
		t.Fatalf("grafts per collection: %v", counts)
	}
}

func TestGraftedFieldCarriesNestedCollection(t *testing.T) {
	two := 2
	stub := stubNode(stubField("s", "LAF.30", "timespan_name", tsName(), "c-name"), "c-ts")
	stub.Field.MaxOccurs = &two
	target := nameCollection("c-name", "LAC.1")
	got := graftField(stub, target.Fields[0], target)
	if got.CollectionID != "c-name" || got.CollectionSemanticID != "LAC.1" || got.CollectionSystemName != "name" || got.CollectionName["en"] != "Name" {
		t.Fatalf("identity: %+v", got)
	}
	if got.CollectionMaxOccurs == nil || *got.CollectionMaxOccurs != 2 || got.GraftStubID != "LAF.30" {
		t.Fatalf("max %v stub %q", got.CollectionMaxOccurs, got.GraftStubID)
	}
	if n := len(got.Field.PathElements); n != 6 || got.Field.PathElements[4].LocalName != "P190_has_symbolic_content" {
		t.Fatalf("composed path: %+v", got.Field.PathElements)
	}
}

func TestGraftPrefixLeavesATail(t *testing.T) {
	one := []domain.ResolvedField{field("f", "X.F1", "name_content", 1,
		pe("property", "crm", "P1_is_identified_by", 0), pe("class", "crm", "E33_E41_Linguistic_Appellation", 1),
		pe("property", "crm", "P190_has_symbolic_content", 2), pe("literal", "xsd", "string", 3))}
	if p := graftPrefix(one); len(p) != 2 || p[1].LocalName != "E33_E41_Linguistic_Appellation" {
		t.Fatalf("one-field prefix: %+v", p)
	}
	shared := []domain.ResolvedField{
		field("a", "X.F1", "note_a", 1, pe("property", "crm", "P1_is_identified_by", 0), pe("class", "crm", "E41_Appellation", 1), pe("property", "crm", "P3_has_note", 2), pe("literal", "xsd", "string", 3)),
		field("b", "X.F2", "note_b", 2, pe("property", "crm", "P1_is_identified_by", 0), pe("class", "crm", "E41_Appellation", 1), pe("property", "crm", "P3_has_note", 2), pe("class", "crm", "E62_String", 3)),
	}
	if p := graftPrefix(shared); len(p) != 2 || p[1].Type != "class" {
		t.Fatalf("prefix must end at a class: %+v", p)
	}
}

// Members that share a node below the collection's root keep it: the
// graft strips the root step only, so the shared node appears once under
// the stub instead of vanishing (one member, or all through one E13).
func TestGraftPrefixStopsAtTheRoot(t *testing.T) {
	one := []domain.ResolvedField{field("f", "X.F1", "uncertainty_type", 1,
		pe("property", "crm", "P14_carried_out_by", 0), pe("class", "crm", "E39_Actor", 1),
		pe("property", "crm", "P141i_was_assigned_by", 2), pe("class", "crm", "E13_Attribute_Assignment", 3),
		pe("property", "crm", "P2_has_type", 4), pe("class", "crm", "E55_Type", 5))}
	p := graftPrefix(one)
	if len(p) != 2 || p[1].LocalName != "E39_Actor" {
		t.Fatalf("prefix ends at the root class: %+v", p)
	}
	if tail := stripSharedPrefix(one[0].PathElements, p); len(tail) != 4 || tail[1].LocalName != "E13_Attribute_Assignment" {
		t.Fatalf("tail keeps the shared node: %+v", tail)
	}
}

// refReader is a CollectionReader + CollectionLookupByID + CollectionLookupView
// over maps. View refuses every collection, as the real reader refuses one
// the project does not own: members must come from LookupView, which
// resolves a collection in its owner project.
type refReader struct {
	colls  map[string]*domain.Collection
	fields map[string][]domain.ResolvedField
	err    error // LookupView fails with it (an infrastructure error)
}

func (r *refReader) Get(_ context.Context, _ auth.ReadScope, _, id string) (*domain.Collection, error) {
	return r.colls[id], nil
}

func (r *refReader) View(_ context.Context, _, _ string) ([]domain.ResolvedField, error) {
	return nil, errors.New("collection: not found")
}

func (r *refReader) LookupByID(_ context.Context, _ auth.ReadScope, id string) (*domain.Collection, error) {
	return r.colls[id], nil
}

func (r *refReader) LookupView(_ context.Context, id string) ([]domain.ResolvedField, error) {
	return r.fields[id], r.err
}

func refFixture() (*refReader, *domain.ModelView) {
	name := nameCollection("c-name", "LAC.1")
	r := &refReader{
		colls: map[string]*domain.Collection{
			"c-ts":    {Entity: domain.Entity{ID: "c-ts", SemanticID: "LAC.6", SystemName: "timespan", ProjectID: "LA"}},
			"c-name":  name.Collection,
			"c-empty": {Entity: domain.Entity{ID: "c-empty", SemanticID: "LAC.99", SystemName: "empty", ProjectID: "LA"}},
		},
		fields: map[string][]domain.ResolvedField{
			"c-ts":   {stubField("f-tsname", "LAC.6.F2", "timespan_name", tsName(), "c-name")},
			"c-name": name.Fields,
		},
	}
	view := &domain.ModelView{Categories: []domain.CategoryGroup{{Collections: []domain.CollectionGroup{{
		ID:     "c-act",
		Fields: []domain.ResolvedField{stubField("s", "OGF.1", "activity_timespan", tsName()[:2], "c-ts", "c-gone", "c-empty")},
	}}}}}
	return r, view
}

func TestLoadReferencedCollectionsFollowsRefs(t *testing.T) {
	r, view := refFixture()
	s := &Service{collections: r}
	got, err := s.loadReferencedCollections(context.Background(), view, Options{ComposeCollections: true})
	if err != nil {
		t.Fatal(err)
	}
	if got["c-ts"].Missing != "" || got["c-ts"].Collection == nil || len(got["c-name"].Fields) != 2 {
		t.Fatalf("loaded: %+v", got)
	}
	if got["c-gone"].Missing != "not found" || got["c-empty"].Missing != "has no members" {
		t.Fatalf("reasons: gone %q, empty %q", got["c-gone"].Missing, got["c-empty"].Missing)
	}
	if off, err := s.loadReferencedCollections(context.Background(), view, Options{}); off != nil || err != nil {
		t.Fatal("composition off loads nothing")
	}
}

func TestLoadReferencedCollectionsFailsOnReaderError(t *testing.T) {
	r, view := refFixture()
	r.err = errors.New("connection reset")
	if _, err := (&Service{collections: r}).loadReferencedCollections(context.Background(), view, Options{ComposeCollections: true}); err == nil {
		t.Fatal("a reader error must fail the snapshot, not drop the graft")
	}
}

func TestComposeUnloadableRefSaysWhy(t *testing.T) {
	stub := stubNode(stubField("s", "LAF.30", "timespan_name", tsName(), "c-empty"), "c-ts")
	rep := &Report{}
	composeCollections([]FieldNode{stub}, map[string]ReferencedCollection{"c-empty": {Missing: "has no members"}}, nil, composeOpts(), rep)
	if len(rep.Warnings) != 1 || !strings.Contains(rep.Warnings[0].Message, "has no members") {
		t.Fatalf("warnings: %+v", rep.Warnings)
	}
}

// The editor wrote full URIs, the import prefixed names: one step either way.
func TestGraftPrefixIgnoresURIForm(t *testing.T) {
	full := func(e domain.PathElement) domain.PathElement {
		e.URI = "http://www.cidoc-crm.org/cidoc-crm/" + e.LocalName
		return e
	}
	fields := []domain.ResolvedField{
		field("a", "X.F1", "content", 1, pe("property", "crm", "P1_is_identified_by", 0), pe("class", "crm", "E33_E41_Linguistic_Appellation", 1), pe("property", "crm", "P190_has_symbolic_content", 2), pe("literal", "xsd", "string", 3)),
		field("b", "X.F2", "assignment", 2, full(pe("property", "crm", "P1_is_identified_by", 0)), full(pe("class", "crm", "E33_E41_Linguistic_Appellation", 1)), full(pe("property", "crm", "P141i_was_assigned_by", 2)), full(pe("class", "crm", "E13_Attribute_Assignment", 3))),
	}
	p := graftPrefix(fields)
	if len(p) != 2 {
		t.Fatalf("prefix: %+v", p)
	}
	if tail := stripSharedPrefix(fields[1].PathElements, p); len(tail) != 2 || tail[0].LocalName != "P141i_was_assigned_by" {
		t.Fatalf("tail: %+v", tail)
	}
}

func nestedFixture() ([]FieldNode, map[string]ReferencedCollection) {
	tsFields := []domain.ResolvedField{
		field("f-begin", "LAC.6.F1", "timespan_begin", 1,
			pe("property", "crm", "P4_has_time-span", 0), pe("class", "crm", "E52_Time-Span", 1),
			pe("property", "crm", "P82a_begin_of_the_begin", 2), pe("literal", "xsd", "dateTime", 3)),
		stubField("f-tsname", "LAC.6.F2", "timespan_name", tsName(), "c-name"),
	}
	referenced := map[string]ReferencedCollection{
		"c-ts":   {Collection: &domain.Collection{Entity: domain.Entity{ID: "c-ts", SemanticID: "LAC.6", SystemName: "timespan"}}, Fields: tsFields, SharedPathPrefix: graftPrefix(tsFields)},
		"c-name": nameCollection("c-name", "LAC.1"),
	}
	stub := stubNode(stubField("s", "OGF.1", "activity_timespan", []domain.PathElement{
		pe("property", "crm", "P4_has_time-span", 0), pe("class", "crm", "E52_Time-Span", 1),
	}, "c-ts"), "c-act")
	return []FieldNode{stub}, referenced
}

func byAddress(out []FieldNode) map[string]FieldNode {
	m := map[string]FieldNode{}
	for _, f := range out {
		m[f.Address] = f
	}
	return m
}

func keys(m map[string]FieldNode) []string {
	k := make([]string, 0, len(m))
	for a := range m {
		k = append(k, a)
	}
	sort.Strings(k)
	return k
}

func TestComposeStampsAddresses(t *testing.T) {
	fields, ref := nestedFixture()
	got := byAddress(composeCollections(fields, ref, nil, composeOpts(), &Report{}))
	for _, a := range []string{"OGF.1", "OGF.1/LAC.6.F1", "OGF.1/LAC.6.F2", "OGF.1/LAC.6.F2/LAC.1.F1", "OGF.1/LAC.6.F2/LAC.1.F2"} {
		if _, ok := got[a]; !ok {
			t.Errorf("no field at %s (have %v)", a, keys(got))
		}
	}
}

func TestComposeStampsAddressesWithoutComposition(t *testing.T) {
	fields, ref := nestedFixture()
	out := composeCollections(fields, ref, nil, Options{}, &Report{})
	if len(out) != 1 || out[0].Address != "OGF.1" {
		t.Fatalf("top-level address not stamped: %+v", out)
	}
}

func TestComposeHiddenGraftDropsSubtree(t *testing.T) {
	fields, ref := nestedFixture()
	grafts := map[string]domain.GraftOverride{"OGF.1/LAC.6.F2": {Address: "OGF.1/LAC.6.F2", IsHidden: true}}
	got := byAddress(composeCollections(fields, ref, grafts, composeOpts(), &Report{}))
	for a := range got {
		if strings.HasPrefix(a, "OGF.1/LAC.6.F2") {
			t.Fatalf("hidden branch still grafted: %s", a)
		}
	}
	if _, ok := got["OGF.1/LAC.6.F1"]; !ok {
		t.Fatal("sibling of the hidden branch dropped")
	}
}

func TestComposeGraftMaxAndNameApply(t *testing.T) {
	fields, ref := nestedFixture()
	one := 1
	grafts := map[string]domain.GraftOverride{"OGF.1/LAC.6.F2": {Address: "OGF.1/LAC.6.F2", MaxOccurs: &one, MaxOccursSet: true, DisplayName: domain.Translations{"en": "Name for Time Span"}}}
	got := byAddress(composeCollections(fields, ref, grafts, composeOpts(), &Report{}))
	stub := got["OGF.1/LAC.6.F2"]
	if stub.Field.MaxOccurs == nil || *stub.Field.MaxOccurs != 1 || stub.Field.DisplayName["en"] != "Name for Time Span" {
		t.Fatalf("stub override not applied: max %v name %v", stub.Field.MaxOccurs, stub.Field.DisplayName)
	}
	child := got["OGF.1/LAC.6.F2/LAC.1.F1"]
	if child.CollectionMaxOccurs == nil || *child.CollectionMaxOccurs != 1 {
		t.Fatalf("nested card max %v, want 1 from the graft override", child.CollectionMaxOccurs)
	}
}

// A graft override with max_occurs: null makes a stub whose own max is 1
// unbounded in that graft.
func TestComposeGraftNullMaxUnbounds(t *testing.T) {
	fields, ref := nestedFixture()
	one := 1
	ts := ref["c-ts"]
	ts.Fields[1].MaxOccurs = &one
	grafts := map[string]domain.GraftOverride{"OGF.1/LAC.6.F2": {Address: "OGF.1/LAC.6.F2", MaxOccursSet: true}}
	got := byAddress(composeCollections(fields, ref, grafts, composeOpts(), &Report{}))
	if m := got["OGF.1/LAC.6.F2"].Field.MaxOccurs; m != nil {
		t.Fatalf("stub max %v, want unbounded", *m)
	}
	if m := got["OGF.1/LAC.6.F2/LAC.1.F1"].CollectionMaxOccurs; m != nil {
		t.Fatalf("nested card max %v, want unbounded", *m)
	}
}

func TestComposeGraftOverrideAppliesToOneGraftOnly(t *testing.T) {
	fields, ref := nestedFixture()
	second := stubNode(stubField("s2", "OGF.2", "other_timespan", []domain.PathElement{
		pe("property", "crm", "P4_has_time-span", 0), pe("class", "crm", "E52_Time-Span", 1),
	}, "c-ts"), "c-act")
	fields = append(fields, second)
	grafts := map[string]domain.GraftOverride{"OGF.1/LAC.6.F2": {Address: "OGF.1/LAC.6.F2", IsHidden: true}}
	got := byAddress(composeCollections(fields, ref, grafts, composeOpts(), &Report{}))
	if _, ok := got["OGF.2/LAC.6.F2/LAC.1.F1"]; !ok {
		t.Fatal("override on OGF.1's graft hid OGF.2's")
	}
}

func TestComposeUnresolvedGraftWarns(t *testing.T) {
	fields, ref := nestedFixture()
	rep := &Report{}
	grafts := map[string]domain.GraftOverride{"OGF.1/LAC.9.F9": {Address: "OGF.1/LAC.9.F9", IsHidden: true}}
	composeCollections(fields, ref, grafts, composeOpts(), rep)
	if len(rep.Warnings) != 1 || rep.Warnings[0].Code != "compose.unresolved_graft" {
		t.Fatalf("warnings %+v", rep.Warnings)
	}
}

func TestComposeNoWarningUnderHiddenGraft(t *testing.T) {
	fields, ref := nestedFixture()
	rep := &Report{}
	one := 1
	grafts := map[string]domain.GraftOverride{
		"OGF.1/LAC.6.F2":          {Address: "OGF.1/LAC.6.F2", IsHidden: true},
		"OGF.1/LAC.6.F2/LAC.1.F1": {Address: "OGF.1/LAC.6.F2/LAC.1.F1", MaxOccurs: &one, MaxOccursSet: true},
	}
	composeCollections(fields, ref, grafts, composeOpts(), rep)
	if len(rep.Warnings) != 0 {
		t.Fatalf("warnings under a hidden graft: %+v", rep.Warnings)
	}
}

func TestComposeSkipsHiddenMembers(t *testing.T) {
	fields, ref := nestedFixture()
	ts := ref["c-ts"]
	ts.Fields = append([]domain.ResolvedField(nil), ts.Fields...)
	ts.Fields[0].IsHidden = true
	ref["c-ts"] = ts
	got := byAddress(composeCollections(fields, ref, nil, composeOpts(), &Report{}))
	if _, ok := got["OGF.1/LAC.6.F1"]; ok {
		t.Fatalf("hidden collection member grafted: %v", keys(got))
	}
	if _, ok := got["OGF.1/LAC.6.F2"]; !ok {
		t.Fatalf("visible member missing: %v", keys(got))
	}
}
