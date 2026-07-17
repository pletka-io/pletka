package generators

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
)

// WriteSnapshotASCII renders the renderer-neutral Snapshot as a
// human-readable ASCII tree keyed by PathNodeID/InstanceID — built so a
// curator can review how the generator names every semantic
// intermediate and groups every leaf field beneath it. One line per
// class-role PathElement, one line per leaf FieldNode hung underneath.
//
// Output layout for a model snapshot:
//
//	Model LAM.1 — activity [LA]  scope=crm:E7_Activity
//	└─ crm:E7_Activity  pn=activity_E7@1
//	   ├─ [crm:P14_carried_out_by] crm:E39_Actor  pn=activity_E7/actor_E39@1
//	   │  └─ • LAF.21 activity_actor  [reference_model]
//	   └─ ...
func WriteSnapshotASCII(w io.Writer, snap *Snapshot) error {
	if snap == nil {
		return fmt.Errorf("nil snapshot")
	}
	if _, err := fmt.Fprintln(w, snapshotASCIIHeader(snap)); err != nil {
		return err
	}
	root := newASCIINode("", "", "", "", "")
	for i := range snap.Fields {
		addFieldToASCII(root, &snap.Fields[i])
	}
	finaliseASCII(root)
	kids := root.orderedChildren()
	for i, child := range kids {
		writeASCIINode(w, child, "", i == len(kids)-1 && len(root.leaves) == 0)
	}
	for i, leaf := range root.leaves {
		writeASCIILeaf(w, leaf, "", i == len(root.leaves)-1)
	}
	return nil
}

type asciiNode struct {
	key       string
	pathID    string
	instance  string
	className string
	propStep  string
	children  map[string]*asciiNode
	keyOrder  []string
	leaves    []*asciiLeaf
}

type asciiLeaf struct {
	field *FieldNode
}

func newASCIINode(key, pathID, instance, className, propStep string) *asciiNode {
	return &asciiNode{
		key:       key,
		pathID:    pathID,
		instance:  instance,
		className: className,
		propStep:  propStep,
		children:  make(map[string]*asciiNode),
	}
}

func (n *asciiNode) orderedChildren() []*asciiNode {
	out := make([]*asciiNode, 0, len(n.keyOrder))
	for _, k := range n.keyOrder {
		out = append(out, n.children[k])
	}
	return out
}

func addFieldToASCII(root *asciiNode, f *FieldNode) {
	cur := root
	for i, pn := range f.Path {
		if pn.Element.Type != "class" {
			continue
		}
		key := asciiKey(pn.Element)
		child, ok := cur.children[key]
		if !ok {
			child = newASCIINode(
				key,
				pn.Element.PathNodeID,
				pn.Element.InstanceID,
				pn.Element.PrefixedName(),
				priorPropertyPrefixed(f.Path, i),
			)
			cur.children[key] = child
			cur.keyOrder = append(cur.keyOrder, key)
		}
		cur = child
	}
	cur.leaves = append(cur.leaves, &asciiLeaf{field: f})
}

func asciiKey(el domain.PathElement) string {
	id := strings.TrimSpace(el.PathNodeID)
	if id == "" {
		id = el.LocalName
	}
	if inst := strings.TrimSpace(el.InstanceID); inst != "" {
		return id + "@" + inst
	}
	return id
}

func priorPropertyPrefixed(path []PathNode, idx int) string {
	for i := idx - 1; i >= 0; i-- {
		switch path[i].Element.Type {
		case "property":
			return path[i].Element.PrefixedName()
		case "class":
			return ""
		}
	}
	return ""
}

func finaliseASCII(n *asciiNode) {
	sort.SliceStable(n.leaves, func(i, j int) bool {
		return asciiLeafSortKey(n.leaves[i].field) < asciiLeafSortKey(n.leaves[j].field)
	})
	for _, child := range n.children {
		finaliseASCII(child)
	}
}

func asciiLeafSortKey(f *FieldNode) string {
	if f == nil {
		return ""
	}
	if s := strings.TrimSpace(f.Field.SystemName); s != "" {
		return s
	}
	return f.Field.SemanticID
}

func writeASCIINode(w io.Writer, n *asciiNode, prefix string, last bool) {
	branch, cont := "├─ ", "│  "
	if last {
		branch, cont = "└─ ", "   "
	}
	label := n.className
	if n.propStep != "" {
		label = "[" + n.propStep + "] " + label
	}
	if n.pathID != "" {
		if n.instance != "" {
			label += "  pn=" + n.pathID + "@" + n.instance
		} else {
			label += "  pn=" + n.pathID
		}
	}
	fmt.Fprintln(w, prefix+branch+label)

	childPrefix := prefix + cont
	kids := n.orderedChildren()
	for i, child := range kids {
		isLast := i == len(kids)-1 && len(n.leaves) == 0
		writeASCIINode(w, child, childPrefix, isLast)
	}
	for i, leaf := range n.leaves {
		writeASCIILeaf(w, leaf, childPrefix, i == len(n.leaves)-1)
	}
}

func writeASCIILeaf(w io.Writer, leaf *asciiLeaf, prefix string, last bool) {
	branch := "├─ "
	if last {
		branch = "└─ "
	}
	f := leaf.field
	sem := f.Field.SemanticID
	if sem == "" {
		sem = f.Field.ID
	}
	parts := []string{"• " + sem}
	if sn := strings.TrimSpace(f.Field.SystemName); sn != "" {
		parts = append(parts, sn)
	}
	if dt := strings.TrimSpace(f.Field.ExpectedValueType); dt != "" {
		parts = append(parts, "["+dt+"]")
	}
	if cs := strings.TrimSpace(f.CollectionSemanticID); cs != "" {
		parts = append(parts, "{"+cs+"}")
	}
	if len(f.GraftPath) > 0 {
		parts = append(parts, "graft="+strings.Join(f.GraftPath, "→"))
	}
	fmt.Fprintln(w, prefix+branch+strings.Join(parts, "  "))
}

func snapshotASCIIHeader(snap *Snapshot) string {
	scope := ""
	switch {
	case snap.Model != nil:
		sem := snap.Model.SemanticID
		if sem == "" {
			sem = snap.Model.ID
		}
		if snap.Model.OntologyScope.LocalName != "" {
			scope = "  scope=" + snap.Model.OntologyScope.PrefixedName()
		}
		return fmt.Sprintf("Model %s — %s [%s]%s",
			sem, snap.Model.SystemName, snap.Project.ID, scope)
	case snap.Collection != nil:
		sem := snap.Collection.SemanticID
		if sem == "" {
			sem = snap.Collection.ID
		}
		return fmt.Sprintf("Collection %s — %s [%s]",
			sem, snap.Collection.SystemName, snap.Project.ID)
	case snap.Field != nil:
		sem := snap.Field.SemanticID
		if sem == "" {
			sem = snap.Field.ID
		}
		return fmt.Sprintf("Field %s — %s [%s]",
			sem, snap.Field.SystemName, snap.Project.ID)
	}
	return "(unknown root)"
}
