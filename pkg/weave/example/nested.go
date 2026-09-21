package example

import (
	"context"
	"fmt"
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

// expandTarget returns the single target collection of a container field and
// whether it may expand at the given nesting level (1-based). note explains a
// refusal ("No target collection set", "Several target collections", "Nesting limit reached").
func (r *slotResolver) expandTarget(field domain.ResolvedField, level int) (collectionID string, ok bool, note string) {
	switch len(field.CollectionModels) {
	case 0:
		return "", false, "No target collection set"
	case 1:
	default:
		return "", false, "Several target collections"
	}
	if level > r.maxDepth {
		return "", false, "Nesting limit reached"
	}
	return field.CollectionModels[0].ID, true, ""
}

// checkValue resolves one value's slot path and returns its issues, and
// whether it is a resolved value that counts toward cardinality. strict
// (Create/Update) turns an invalid path, or an unknown override on a nested
// path, into an input error; reads report both as warnings. A depth-0 value
// on a removed slot stays a stale_override warning in both modes, as before
// B.3.
func (s *Service) checkValue(ctx context.Context, r *slotResolver, value domain.ExampleValue, strict bool) ([]domain.ExampleIssue, bool, error) {
	rs, err := r.resolve(valueSlotPath(value))
	if err != nil {
		return nil, false, err
	}
	cp := containerPath(value.SlotPath)
	var out []domain.ExampleIssue
	switch rs.status {
	case slotOK:
		out, err = s.valueIssues(ctx, value, rs.field)
		if err != nil {
			return nil, false, err
		}
	case slotMoved:
		return []domain.ExampleIssue{movedGroupValueIssue(value, cp)}, false, nil
	case slotUnknown:
		if strict && rs.depth > 0 {
			return nil, false, fmt.Errorf("value for field %s: slot_path %q does not resolve: %s", value.FieldID, value.SlotPath, rs.reason)
		}
		out = []domain.ExampleIssue{warningIssue("stale_override", nil, &value.OverrideID, &value.OccurrenceIndex, rs.reason)}
	case slotInvalid:
		if strict {
			return nil, false, fmt.Errorf("value for field %s: slot_path %q does not resolve: %s", value.FieldID, value.SlotPath, rs.reason)
		}
		fieldID := value.FieldID
		out = []domain.ExampleIssue{warningIssue("invalid_nesting", &fieldID, &value.OverrideID, &value.OccurrenceIndex, rs.reason)}
	}
	for i := range out {
		out[i].GroupPath = cp
	}
	return out, rs.status == slotOK, nil
}
