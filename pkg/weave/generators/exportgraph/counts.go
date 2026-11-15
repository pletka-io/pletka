package exportgraph

import "github.com/pletka-io/pletka/pkg/weave/generators"

// EffectiveMin is the stricter of Min and Required (Required <=> Min >= 1).
func EffectiveMin(required bool, min int) int {
	if required && min < 1 {
		return 1
	}
	return min
}

// addEnding records that field's path ends on edge. The first ending also
// sets the edge's own counts.
func addEnding(edge *ResourceEdge, field *generators.FieldNode) {
	min, max := EffectiveMin(field.Field.IsRequired, field.Field.MinOccurs), field.Field.MaxOccurs
	edge.Endings = append(edge.Endings, Ending{FieldID: field.Field.ID, Min: min, Max: max})
	if len(edge.Endings) == 1 {
		// overwrites any placement counts; a field with a collection
		// re-applies the placement next, made stricter by this ending
		edge.Min, edge.Max = min, max
	}
}

// applyPlacement marks edge as the entry into a placed collection and gives
// it the placement's counts, made stricter by a field that ends on it.
func applyPlacement(edge *ResourceEdge, field *generators.FieldNode) {
	edge.Entry = true
	edge.Min = EffectiveMin(field.CollectionIsRequired, field.CollectionMinOccurs)
	edge.Max = field.CollectionMaxOccurs
	if len(edge.Endings) > 0 {
		tighten(edge, edge.Endings[0].Min, edge.Endings[0].Max)
	}
}

func tighten(edge *ResourceEdge, min int, max *int) {
	edge.Min = maxInt(edge.Min, min)
	if max != nil && (edge.Max == nil || *max < *edge.Max) {
		edge.Max = max
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// propagateRequired makes an intermediate step required when anything below
// it is, up to a collection entry or a field's own step, which keep their
// counts.
func propagateRequired(g *Graph, nodes map[string]*ResourceNode) {
	done := map[string]bool{}
	needs := map[string]bool{}
	var visit func(key string) bool
	visit = func(key string) bool {
		if done[key] {
			return needs[key]
		}
		done[key] = true // a cycle reads as "no value needed" on re-entry
		n := nodes[key]
		if n == nil {
			return false
		}
		need := false
		for _, l := range n.Literals {
			if l.Min >= 1 {
				need = true
			}
		}
		for i := range n.Edges {
			e := &n.Edges[i]
			below := visit(e.TargetKey)
			if below && len(e.Endings) == 0 && !e.Entry && e.Min < 1 {
				e.Min = 1
			}
			if e.Min >= 1 {
				need = true
			}
		}
		needs[key] = need
		return need
	}
	visit(g.RootKey)
}
