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
	edge.Endings = append(edge.Endings, Ending{FieldID: field.Field.ID, Min: min, Max: max, Field: field})
	if len(edge.Endings) == 1 {
		// overwrites any placement counts; a field with a collection
		// re-applies the placement next, made stricter by this ending
		edge.Min, edge.Max = min, max
	}
}

// applyPlacement marks edge as the entry into a placed collection and gives
// it the placement's counts, made stricter by the first field that ends on
// it. When field itself ends on edge, its ending gets the placement's
// counts made stricter by its own.
func applyPlacement(edge *ResourceEdge, field *generators.FieldNode) {
	min, max := EffectiveMin(field.CollectionIsRequired, field.CollectionMinOccurs), field.CollectionMaxOccurs
	if n := len(edge.Endings); n > 0 && edge.Endings[n-1].Field == field {
		end := &edge.Endings[n-1]
		end.Min, end.Max = tighten(min, max, end.Min, end.Max)
	}
	edge.Entry = true
	edge.Min, edge.Max = min, max
	if len(edge.Endings) > 0 {
		edge.Min, edge.Max = tighten(edge.Min, edge.Max, edge.Endings[0].Min, edge.Endings[0].Max)
	}
}

// tighten returns the stricter of two counts: the larger min, the smaller
// max.
func tighten(min int, max *int, otherMin int, otherMax *int) (int, *int) {
	min = maxInt(min, otherMin)
	if otherMax != nil && (max == nil || *otherMax < *max) {
		max = otherMax
	}
	return min, max
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
