package example

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
)

// expectedValueTypeCollection is the ResolvedField.ExpectedValueType value
// for fields that reference a collection; such a field is a container whose
// target collection's fields open in place in an example.
const expectedValueTypeCollection = "Collection"

// slotSegments splits a slot path into its optional group segment and its
// field segments (anchor first, leaf last). ok is false when any field
// segment is malformed or there are no field segments. A first segment that
// is not a field segment is returned as the group segment as is: whether it
// names the anchor's current group is placement's concern (B.2 reports a
// malformed group segment as a group mismatch).
func slotSegments(path string) (group string, fields []string, ok bool) {
	if path == "" {
		return "", nil, false
	}
	segs := strings.Split(path, "/")
	if !isFieldSegment(segs[0]) {
		if segs[0] == "" || len(segs) == 1 {
			return "", nil, false
		}
		group, segs = segs[0], segs[1:]
	}
	for _, seg := range segs {
		if !isFieldSegment(seg) {
			return "", nil, false
		}
	}
	return group, segs, true
}

func isFieldSegment(seg string) bool {
	if strings.Contains(seg, "/") {
		return false
	}
	_, _, ok := domain.ParseExampleSlotLeaf(seg)
	return ok
}

// containerPath is the path without its leaf segment ("" for a one-segment
// path). It generalises B.2's instancePath: for "C1:0/21:0" it is "C1:0",
// for "C1:0/302:1/55:0" it is "C1:0/302:1".
func containerPath(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i]
	}
	return ""
}

// groupPart is the group segment of a slot path, "" when the path starts
// with a field segment. For B.2 paths it equals instancePath.
func groupPart(path string) string {
	first, _, found := strings.Cut(path, "/")
	if !found || isFieldSegment(first) {
		return ""
	}
	return first
}

// anchorOverride is the override of the outermost field segment of v's slot
// path: the model-level slot that decides group placement. For a path that
// does not parse (or is empty) it falls back to v.OverrideID, which equals
// the anchor for every depth-0 path.
func anchorOverride(v domain.ExampleValue) int64 {
	_, fields, ok := slotSegments(v.SlotPath)
	if !ok {
		return v.OverrideID
	}
	oid, _, _ := domain.ParseExampleSlotLeaf(fields[0])
	return oid
}

// valueSlotPath is v's slot path, or its depth-0 path from override and
// occurrence when none is set (values not yet normalized, pre-B.1 rows).
func valueSlotPath(v domain.ExampleValue) string {
	if v.SlotPath == "" {
		return domain.ExampleSlot(v.OverrideID, v.OccurrenceIndex)
	}
	return v.SlotPath
}

type slotStatus int

const (
	slotOK      slotStatus = iota
	slotUnknown            // anchor or an inner override is not where the path says (removed field)
	slotMoved              // anchor's group segment disagrees with the anchor's current group (B.2 moved_group_value)
	slotInvalid            // container is not an expandable Collection field, or depth > cap
)

// staleAnchorMessage is the stale_override message for a value whose anchor
// is no longer on the model (unchanged since B.1).
const staleAnchorMessage = "This field slot no longer exists on the target model."

// staleInnerMessage is the stale_override message for a value whose inner
// override is no longer part of its container's target collection.
const staleInnerMessage = "This field is no longer part of the nested collection."

type resolvedSlot struct {
	status slotStatus
	field  domain.ResolvedField // leaf field when status == slotOK
	anchor int64                // outermost field override
	depth  int                  // nesting depth: field segments - 1
	reason string               // human message for slotUnknown/slotInvalid
}

// slotResolver resolves slot paths against one model view plus the
// collection views it needs, cached for the request.
type slotResolver struct {
	ctx         context.Context
	projectID   string
	views       ViewReader
	maxDepth    int
	groups      map[int64]string
	model       map[int64]domain.ResolvedField
	collections map[string][]domain.ResolvedField
}

func (s *Service) newSlotResolver(ctx context.Context, projectID string, view *domain.ModelView) *slotResolver {
	return &slotResolver{
		ctx:         ctx,
		projectID:   projectID,
		views:       s.views,
		maxDepth:    s.maxDepth,
		groups:      groupOfOverride(view),
		model:       buildFieldByOverride(view),
		collections: map[string][]domain.ResolvedField{},
	}
}

// resolve walks path from its anchor to its leaf. The error is only for a
// failed collection view read; an unresolvable path is reported through the
// status. A path without a group segment is treated as not yet placed and
// never reports slotMoved (placeInGroups gives it its group segment).
func (r *slotResolver) resolve(path string) (resolvedSlot, error) {
	group, fields, ok := slotSegments(path)
	if !ok {
		return resolvedSlot{status: slotInvalid, reason: fmt.Sprintf("Malformed slot path %q.", path)}, nil
	}
	anchor, _, _ := domain.ParseExampleSlotLeaf(fields[0])
	out := resolvedSlot{anchor: anchor, depth: len(fields) - 1}
	field, known := r.model[anchor]
	if !known {
		out.status, out.reason = slotUnknown, staleAnchorMessage
		return out, nil
	}
	if group != "" && !r.inGroup(group, anchor) {
		out.status = slotMoved
		return out, nil
	}
	for level := 1; level < len(fields); level++ {
		next, status, reason, err := r.step(field, fields[level], level)
		if err != nil {
			return resolvedSlot{}, err
		}
		if status != slotOK {
			out.status, out.reason = status, reason
			return out, nil
		}
		field = next
	}
	out.status, out.field = slotOK, field
	return out, nil
}

// step resolves one inner field segment at nesting level (1-based) inside
// container.
func (r *slotResolver) step(container domain.ResolvedField, seg string, level int) (domain.ResolvedField, slotStatus, string, error) {
	if strings.TrimSpace(container.ExpectedValueType) != expectedValueTypeCollection {
		return domain.ResolvedField{}, slotInvalid, fmt.Sprintf("Field slot %d is not a Collection field.", container.OverrideID), nil
	}
	collectionID, ok, note := r.expandTarget(container, level)
	if !ok {
		return domain.ResolvedField{}, slotInvalid, fmt.Sprintf("Field slot %d cannot hold nested values: %s.", container.OverrideID, note), nil
	}
	fields, err := r.collectionFields(collectionID)
	if err != nil {
		return domain.ResolvedField{}, slotInvalid, "", err
	}
	oid, _, _ := domain.ParseExampleSlotLeaf(seg)
	for _, f := range fields {
		if f.OverrideID == oid {
			return f, slotOK, "", nil
		}
	}
	return domain.ResolvedField{}, slotUnknown, staleInnerMessage, nil
}

// inGroup reports whether group segment names anchor's current group. A
// malformed segment reads as the direct bucket, as in groupMatches.
func (r *slotResolver) inGroup(group string, anchor int64) bool {
	coll, _, ok := domain.ParseExampleGroupSegment(group)
	if !ok {
		coll = ""
	}
	return coll == r.groups[anchor]
}

// collectionFields returns the (cached) CollectionView of collectionID.
func (r *slotResolver) collectionFields(collectionID string) ([]domain.ResolvedField, error) {
	if fields, ok := r.collections[collectionID]; ok {
		return fields, nil
	}
	fields, err := r.views.CollectionView(r.ctx, collectionID, r.projectID)
	if err != nil {
		return nil, fmt.Errorf("collection view %s: %w", collectionID, err)
	}
	r.collections[collectionID] = fields
	return fields, nil
}

// Notes expandTarget gives for a container that cannot open.
const (
	noteNoTarget       = "No target collection set"
	noteSeveralTargets = "Several target collections"
	noteNestingLimit   = "Nesting limit reached"
)

// expandTarget returns the single target collection of a container field and
// whether it may expand at the given nesting level (1-based). note explains a
// refusal ("No target collection set", "Several target collections", "Nesting limit reached").
func (r *slotResolver) expandTarget(field domain.ResolvedField, level int) (collectionID string, ok bool, note string) {
	switch len(field.CollectionModels) {
	case 0:
		return "", false, noteNoTarget
	case 1:
	default:
		return "", false, noteSeveralTargets
	}
	if level > r.maxDepth {
		return "", false, noteNestingLimit
	}
	return field.CollectionModels[0].ID, true, ""
}

// resolveAll resolves every value's slot path, in values order. The error
// is only for a failed collection view read.
func (r *slotResolver) resolveAll(values []domain.ExampleValue) ([]resolvedSlot, error) {
	out := make([]resolvedSlot, len(values))
	for i, v := range values {
		rs, err := r.resolve(valueSlotPath(v))
		if err != nil {
			return nil, err
		}
		out[i] = rs
	}
	return out, nil
}

// checkValue returns the issues of one value given its resolved slot.
// strict (Create/Update) turns an invalid path, or an unknown override on a
// nested path, into an input error; reads report both as warnings. A
// depth-0 value on a removed slot stays a stale_override warning in both
// modes, as before B.3.
func (s *Service) checkValue(ctx context.Context, rs resolvedSlot, value domain.ExampleValue, strict bool) ([]domain.ExampleIssue, error) {
	cp := containerPath(value.SlotPath)
	var out []domain.ExampleIssue
	switch rs.status {
	case slotOK:
		var err error
		out, err = s.valueIssues(ctx, value, rs.field)
		if err != nil {
			return nil, err
		}
	case slotMoved:
		return []domain.ExampleIssue{movedGroupValueIssue(value, cp)}, nil
	case slotUnknown:
		if strict && rs.depth > 0 {
			return nil, fmt.Errorf("value for field %s: slot_path %q does not resolve: %s", value.FieldID, value.SlotPath, rs.reason)
		}
		out = []domain.ExampleIssue{warningIssue("stale_override", nil, &value.OverrideID, &value.OccurrenceIndex, rs.reason)}
	case slotInvalid:
		if strict {
			return nil, fmt.Errorf("value for field %s: slot_path %q does not resolve: %s", value.FieldID, value.SlotPath, rs.reason)
		}
		fieldID := value.FieldID
		out = []domain.ExampleIssue{warningIssue("invalid_nesting", &fieldID, &value.OverrideID, &value.OccurrenceIndex, rs.reason)}
	}
	for i := range out {
		out[i].GroupPath = cp
	}
	return out, nil
}

// compactInstances renumbers instances to 0..n-1 in their existing order at
// every non-leaf segment of a slot path, outermost first: the group segment
// per collection (B.2), then each container segment per family (the path up
// to the container plus the container's override, e.g. "C1:0/302" for
// "C1:0/302:3/601:0"). The form always shows group instance 0, so a removed
// first instance must not leave a gap; nested instance numbers are kept
// dense the same way. Only values that resolved slotOK count and move: any
// other value (removed field, moved group, stale inner override, invalid
// nesting) is left exactly as stored and keeps no instance alive.
// Renumbering never changes a path's overrides, so slots stays valid.
func compactInstances(values []domain.ExampleValue, slots []resolvedSlot) {
	for level := 0; compactLevel(values, slots, level); level++ {
	}
}

// compactLevel compacts segment index level of every resolved path that has
// a non-leaf segment there, and reports whether any did.
func compactLevel(values []domain.ExampleValue, slots []resolvedSlot, level int) bool {
	seen := map[string]map[int]bool{}
	deeper := false
	for i, v := range values {
		segs := strings.Split(v.SlotPath, "/")
		if slots[i].status != slotOK || level >= len(segs)-1 {
			continue
		}
		deeper = true
		if fam, n, ok := instanceFamily(segs, level); ok {
			if seen[fam] == nil {
				seen[fam] = map[int]bool{}
			}
			seen[fam][n] = true
		}
	}
	renumber := make(map[string]map[int]int, len(seen))
	for fam, set := range seen {
		m := make(map[int]int, len(set))
		for i, n := range slices.Sorted(maps.Keys(set)) {
			m[n] = i
		}
		renumber[fam] = m
	}
	for i := range values {
		segs := strings.Split(values[i].SlotPath, "/")
		if slots[i].status != slotOK || level >= len(segs)-1 {
			continue
		}
		fam, n, ok := instanceFamily(segs, level)
		if !ok || renumber[fam][n] == n {
			continue
		}
		segs[level] = withInstance(segs[level], renumber[fam][n])
		values[i].SlotPath = strings.Join(segs, "/")
	}
	return deeper
}

// instanceFamily names the set of sibling instances segs[level] belongs to,
// and its instance number. A group segment's family is its collection id; a
// container segment's family is the path before it plus "/" and its
// override. ok is false for a segment that parses as neither (a malformed
// group segment is left alone, as in B.2).
func instanceFamily(segs []string, level int) (family string, instance int, ok bool) {
	seg := segs[level]
	if oid, n, isField := domain.ParseExampleSlotLeaf(seg); isField {
		return strings.Join(segs[:level], "/") + "/" + strconv.FormatInt(oid, 10), n, true
	}
	return domain.ParseExampleGroupSegment(seg)
}

// withInstance renders seg (a group or container segment) with instance n.
func withInstance(seg string, n int) string {
	if oid, _, isField := domain.ParseExampleSlotLeaf(seg); isField {
		return domain.ExampleSlot(oid, n)
	}
	coll, _, _ := domain.ParseExampleGroupSegment(seg)
	return domain.ExampleGroupSegment(coll, n)
}

// nestedInstance is one nested collection instance that holds resolved
// values: the path of the instance it lives in and its container's override.
type nestedInstance struct {
	parent    string
	container int64
}

// nestedInstances maps every nested instance path holding at least one
// resolved value ("C1:0/302:1", and for depth 2 also "C1:0/302:1/603:0") to
// its parent path and container. Nested instances are on demand: an
// instance without values does not exist.
func nestedInstances(values []domain.ExampleValue, slots []resolvedSlot) map[string]nestedInstance {
	out := map[string]nestedInstance{}
	for i, v := range values {
		if slots[i].status != slotOK {
			continue
		}
		p := containerPath(v.SlotPath)
		for d := slots[i].depth; d > 0; d-- {
			parent := containerPath(p)
			oid, _, _ := domain.ParseExampleSlotLeaf(p)
			out[p] = nestedInstance{parent: parent, container: oid}
			p = parent
		}
	}
	return out
}

// nestedCardinalityIssues adds each container field's count (the number of
// its nested instances holding values) to counts under its parent instance,
// then checks required/min/max of every field of the target collection in
// each present nested instance. Callers check model-level fields with the
// updated counts afterwards, so a container with no nested values counts 0.
func (r *slotResolver) nestedCardinalityIssues(values []domain.ExampleValue, slots []resolvedSlot, counts map[string]int) ([]domain.ExampleIssue, error) {
	instances := nestedInstances(values, slots)
	for _, in := range instances {
		counts[slotCountKey(in.parent, in.container)]++
	}
	var issues []domain.ExampleIssue
	for _, path := range slices.Sorted(maps.Keys(instances)) {
		fields, level, err := r.instanceFields(path)
		if err != nil {
			return nil, err
		}
		for _, f := range fields {
			issues = append(issues, fieldCardinalityIssues(r.cardinalityField(f, level), path, counts[slotCountKey(path, f.OverrideID)])...)
		}
	}
	return issues, nil
}

// instanceFields returns the fields of the collection opened by the nested
// instance at path (whose last segment is the container), and the nesting
// level at which a container among those fields would open. A path that
// does not resolve to an expandable container yields no fields;
// nestedInstances only produces paths under resolved values, so that does
// not happen.
func (r *slotResolver) instanceFields(path string) ([]domain.ResolvedField, int, error) {
	rs, err := r.resolve(path)
	if err != nil || rs.status != slotOK {
		return nil, 0, err
	}
	collectionID, ok, _ := r.expandTarget(rs.field, rs.depth+1)
	if !ok {
		return nil, 0, nil
	}
	fields, err := r.collectionFields(collectionID)
	return fields, rs.depth + 2, err
}

// cardinalityField is field as the cardinality checks see it when a
// container among its siblings would open at nesting level (1 for a
// model-level slot, 2 inside a nested collection). A Collection container
// that cannot open there (no target, several targets, past the depth cap)
// can never be filled, so it is neither required nor has a minimum;
// max_occurs still applies.
func (r *slotResolver) cardinalityField(field domain.ResolvedField, level int) domain.ResolvedField {
	if strings.TrimSpace(field.ExpectedValueType) != expectedValueTypeCollection {
		return field
	}
	if _, ok, _ := r.expandTarget(field, level); ok {
		return field
	}
	field.IsRequired, field.MinOccurs = false, 0
	return field
}

// widgetNestedCollection is the form widget of a Collection-typed field: its
// target collection's fields open in place instead of a value picker.
const widgetNestedCollection = "nested-collection"

// formBuilder builds the example form's fields for one BuildFormSchema
// request: values and issues bucketed by the instance they live in, the
// nested instances holding values, and the blank nested templates (built
// once per collection and level, cloned into every place they are used).
type formBuilder struct {
	r           *slotResolver
	byInstance  map[string][]domain.ExampleValue // slotCountKey(containerPath, leaf override)
	issuesByKey map[string][]domain.ExampleIssue // issueKey(GroupPath, override, occurrence)
	nested      map[string][]string              // slotCountKey(parent path, container) -> nested instance paths, by instance
	templates   map[string][]ExampleFormField    // collectionID|level -> blank fields
}

// newFormBuilder buckets the resolved values. Values that did not resolve
// (removed field, moved group, stale inner override, invalid nesting) are
// left out of the form, as they are out of validation counts.
func newFormBuilder(r *slotResolver, values []domain.ExampleValue, slots []resolvedSlot, issuesByKey map[string][]domain.ExampleIssue) *formBuilder {
	b := &formBuilder{
		r:           r,
		byInstance:  map[string][]domain.ExampleValue{},
		issuesByKey: issuesByKey,
		nested:      map[string][]string{},
		templates:   map[string][]ExampleFormField{},
	}
	for i, v := range values {
		if slots[i].status == slotOK {
			key := slotCountKey(containerPath(v.SlotPath), v.OverrideID)
			b.byInstance[key] = append(b.byInstance[key], v)
		}
	}
	for path, in := range nestedInstances(values, slots) {
		key := slotCountKey(in.parent, in.container)
		b.nested[key] = append(b.nested[key], path)
	}
	for _, paths := range b.nested {
		slices.SortFunc(paths, func(a, c string) int { return lastInstance(a) - lastInstance(c) })
	}
	return b
}

// blank is a builder sharing b's resolver and template cache but holding no
// values, issues or nested instances: it builds templates.
func (b *formBuilder) blank() *formBuilder {
	return &formBuilder{r: b.r, templates: b.templates}
}

// field builds f in the instance at path ("" for direct fields, "C1:0" in a
// group, "C1:0/302:1" in a nested instance). level is the nesting level at
// which f opens if it is a container (1 for model-level fields). A
// Collection-typed field gets the nested-collection widget, no occurrences,
// its target's template and its nested instances holding values.
func (b *formBuilder) field(f domain.ResolvedField, path string, level int) (ExampleFormField, error) {
	f = b.r.cardinalityField(f, level)
	out := buildExampleField(f, b.byInstance[slotCountKey(path, f.OverrideID)], b.issuesByKey, path)
	out.SlotPrefix = slotPrefix(path)
	if strings.TrimSpace(f.ExpectedValueType) != expectedValueTypeCollection {
		return out, nil
	}
	out.Widget, out.Occurrences = widgetNestedCollection, nil
	nested, err := b.template(f, level)
	if err != nil {
		return ExampleFormField{}, err
	}
	out.Nested = nested
	if !nested.Expandable {
		return out, nil
	}
	for _, p := range b.nested[slotCountKey(path, f.OverrideID)] {
		entry, err := b.instanceEntry(out, p, level+1)
		if err != nil {
			return ExampleFormField{}, err
		}
		out.NestedInstances = append(out.NestedInstances, entry)
	}
	return out, nil
}

// instanceEntry is the group entry of container's nested instance at path;
// its fields open containers at level.
func (b *formBuilder) instanceEntry(container ExampleFormField, path string, level int) (ExampleFormGroup, error) {
	cfs, err := b.r.collectionFields(container.Nested.CollectionID)
	if err != nil {
		return ExampleFormGroup{}, err
	}
	fields := make([]ExampleFormField, 0, len(cfs))
	for _, cf := range cfs {
		field, err := b.field(cf, path, level)
		if err != nil {
			return ExampleFormGroup{}, err
		}
		fields = append(fields, field)
	}
	return ExampleFormGroup{
		ID:         container.Nested.CollectionID,
		Label:      container.Nested.Label,
		Instance:   lastInstance(path),
		SlotPrefix: slotPrefix(path),
		Repeatable: container.Repeatable,
		MinOccurs:  container.MinOccurs,
		MaxOccurs:  container.MaxOccurs,
		Fields:     fields,
	}, nil
}

// template describes container f's target collection at nesting level, with
// a blank copy of its fields when it can open there.
func (b *formBuilder) template(f domain.ResolvedField, level int) (*ExampleNestedCollection, error) {
	collectionID, ok, note := b.r.expandTarget(f, level)
	out := &ExampleNestedCollection{Expandable: ok, Note: note}
	if len(f.CollectionModels) == 1 {
		ref := f.CollectionModels[0]
		out.CollectionID, out.Label = ref.ID, ref.Name
		if len(out.Label) == 0 {
			out.Label = domain.Translations{"en": ref.ID}
		}
	}
	if !ok {
		return out, nil
	}
	key := collectionID + "|" + strconv.Itoa(level)
	fields, cached := b.templates[key]
	if !cached {
		cfs, err := b.r.collectionFields(collectionID)
		if err != nil {
			return nil, err
		}
		blank := b.blank()
		fields = make([]ExampleFormField, 0, len(cfs))
		for _, cf := range cfs {
			field, err := blank.field(cf, "", level+1)
			if err != nil {
				return nil, err
			}
			fields = append(fields, field)
		}
		b.templates[key] = fields
	}
	out.Fields = cloneFormFields(fields)
	return out, nil
}

// cloneFormFields deep-copies template fields so no two places in a form
// share a template's slices.
func cloneFormFields(fields []ExampleFormField) []ExampleFormField {
	out := slices.Clone(fields)
	for i := range out {
		out[i].Occurrences = slices.Clone(out[i].Occurrences)
		if n := out[i].Nested; n != nil {
			c := *n
			c.Fields = cloneFormFields(n.Fields)
			out[i].Nested = &c
		}
	}
	return out
}

// slotPrefix is the slot_prefix of the fields in the instance at path.
func slotPrefix(path string) string {
	if path == "" {
		return ""
	}
	return path + "/"
}

// lastInstance is the instance number of path's last segment (a group or
// container segment); 0 when it parses as neither.
func lastInstance(path string) int {
	seg := path[strings.LastIndex(path, "/")+1:]
	if _, n, ok := domain.ParseExampleSlotLeaf(seg); ok {
		return n
	}
	_, n, _ := domain.ParseExampleGroupSegment(seg)
	return n
}

// formGroupPresence lists, per collection group, the group instances
// holding resolved values (nested ones included), as validateValues counts
// them.
func formGroupPresence(values []domain.ExampleValue, slots []resolvedSlot) map[string]map[string]bool {
	present := map[string]map[string]bool{}
	for i, v := range values {
		if slots[i].status != slotOK {
			continue
		}
		gp := groupPart(v.SlotPath)
		if coll, _, ok := domain.ParseExampleGroupSegment(gp); ok {
			if present[coll] == nil {
				present[coll] = map[string]bool{}
			}
			present[coll][gp] = true
		}
	}
	return present
}

// routeIssues sorts a validation report's issues for the form: group-level
// issues by collection, field issues by issueKey(GroupPath, override,
// occurrence) (GroupPath is the instance the field lives in, nested
// instances included), and the rest to the top.
func routeIssues(issues []domain.ExampleIssue) (byKey, byGroup map[string][]domain.ExampleIssue, top []domain.ExampleIssue) {
	byKey, byGroup = map[string][]domain.ExampleIssue{}, map[string][]domain.ExampleIssue{}
	for _, issue := range issues {
		switch {
		case issue.CollectionID != nil:
			byGroup[*issue.CollectionID] = append(byGroup[*issue.CollectionID], issue)
		case issue.OverrideID == nil:
			top = append(top, issue)
		default:
			idx := -1
			if issue.OccurrenceIndex != nil {
				idx = *issue.OccurrenceIndex
			}
			key := issueKey(issue.GroupPath, *issue.OverrideID, idx)
			byKey[key] = append(byKey[key], issue)
		}
	}
	return byKey, byGroup, top
}
