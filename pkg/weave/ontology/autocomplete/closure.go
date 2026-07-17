package autocomplete

// ClosureOpts controls a closure walk. Include and Cut are deliberately
// separate: Include filters the OUTPUT (a hidden node's descendants still
// surface); Cut prunes the WALK (descendants past a cut node are never
// visited). Using one mask for both is the bug pattern this split avoids.
type ClosureOpts struct {
	Include func(*Node) bool // emit in output if true (nil = include all)
	Cut     func(*Node) bool // stop walking this subtree if true (nil = never cut)
}

// closure performs a breadth-first walk from root along edges, returning
// every reachable node EXCLUDING root, subject to opts. Cut/Include apply
// only to discovered nodes, never to root. The seen set is pointer-keyed,
// so cycles terminate at O(graph size).
func closure(root *Node, edges func(*Node) []*Node, opts ClosureOpts) []*Node {
	seen := map[*Node]struct{}{root: {}}
	out := make([]*Node, 0)
	queue := []*Node{root}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, next := range edges(n) {
			if _, dup := seen[next]; dup {
				continue
			}
			seen[next] = struct{}{}
			if opts.Cut != nil && opts.Cut(next) {
				continue
			}
			if opts.Include == nil || opts.Include(next) {
				out = append(out, next)
			}
			queue = append(queue, next)
		}
	}
	return out
}
