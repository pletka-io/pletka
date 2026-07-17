package generators

import (
	"fmt"
	"io"
	"strings"
)

// WriteSnapshotPaths emits one line per leaf field in tree-walk order
// (matching WriteSnapshotASCII), printing the full path as `->` notation
// with both the legacy InstanceID ("old") and the generated PathNodeID
// ("new") in square brackets on each class step. Used by curators to
// spot drift between the old hand-coded bracket IDs and the new
// PathNode-driven keying.
//
// Output layout for a model snapshot:
//
//	Model LAM.1 — activity [LA]  scope=crm:E7_Activity
//
//	LAF.21  activity_actor  [reference_model]
//	  crm:E7_Activity[old=HERF.200_1 new=activity_E7] -> crm:P14_carried_out_by -> crm:E39_Actor[old=HERF.200_2 new=actor_E39]
func WriteSnapshotPaths(w io.Writer, snap *Snapshot) error {
	if snap == nil {
		return fmt.Errorf("nil snapshot")
	}
	if _, err := fmt.Fprintln(w, snapshotASCIIHeader(snap)); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	root := newASCIINode("", "", "", "", "")
	for i := range snap.Fields {
		addFieldToASCII(root, &snap.Fields[i])
	}
	finaliseASCII(root)
	var fields []*FieldNode
	collectLeavesInTreeOrder(root, &fields)
	for _, f := range fields {
		writePathsField(w, f)
	}
	return nil
}

// collectLeavesInTreeOrder mirrors writeASCIINode's traversal: children
// first (DFS), then own leaves. This keeps the output ordered the same
// way as the ASCII tree so a curator can scan both side-by-side.
func collectLeavesInTreeOrder(n *asciiNode, out *[]*FieldNode) {
	for _, child := range n.orderedChildren() {
		collectLeavesInTreeOrder(child, out)
	}
	for _, leaf := range n.leaves {
		out2 := append(*out, leaf.field)
		*out = out2
	}
}

func writePathsField(w io.Writer, f *FieldNode) {
	sem := f.Field.SemanticID
	if sem == "" {
		sem = f.Field.ID
	}
	headerParts := []string{sem}
	if sn := strings.TrimSpace(f.Field.SystemName); sn != "" {
		headerParts = append(headerParts, sn)
	}
	if dt := strings.TrimSpace(f.Field.ExpectedValueType); dt != "" {
		headerParts = append(headerParts, "["+dt+"]")
	}
	if cs := strings.TrimSpace(f.CollectionSemanticID); cs != "" {
		headerParts = append(headerParts, "{"+cs+"}")
	}
	fmt.Fprintln(w, strings.Join(headerParts, "  "))
	fmt.Fprintln(w, "  "+pathArrowNotation(f))
	fmt.Fprintln(w)
}

func pathArrowNotation(f *FieldNode) string {
	if f == nil || len(f.Path) == 0 {
		return "(empty path)"
	}
	steps := make([]string, 0, len(f.Path))
	for _, pn := range f.Path {
		switch pn.Element.Type {
		case "class":
			steps = append(steps, pn.Element.PrefixedName()+pathIDBrackets(pn.Element.InstanceID, pn.Element.PathNodeID))
		case "property":
			steps = append(steps, pn.Element.PrefixedName())
		case "literal":
			steps = append(steps, pn.Element.PrefixedName())
		default:
			steps = append(steps, pn.Element.PrefixedName())
		}
	}
	return strings.Join(steps, " -> ")
}

// pathIDBrackets returns the labeled-id suffix appended to a class step.
// Skips empty sides so a step with only one of the two ids reads
// cleanly.
func pathIDBrackets(oldID, newID string) string {
	oldID = strings.TrimSpace(oldID)
	newID = strings.TrimSpace(newID)
	if oldID == "" && newID == "" {
		return ""
	}
	parts := make([]string, 0, 2)
	if oldID != "" {
		parts = append(parts, "old="+oldID)
	}
	if newID != "" {
		parts = append(parts, "new="+newID)
	}
	return "[" + strings.Join(parts, " ") + "]"
}
