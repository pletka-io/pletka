package conformance

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

// ReadMethod is one exported read-shaped method on a slice's *Service.
type ReadMethod struct {
	Slice    string
	Name     string
	Line     int
	HasScope bool
}

// Key is the allowlist key: "slice.Method".
func (m ReadMethod) Key() string { return m.Slice + "." + m.Name }

// mutationPrefixes are the method-name prefixes that denote a write.
// Everything else on a *Service is treated as a read.
//
// This is a denylist on purpose. An allowlist of read prefixes was tried
// first and hid roughly two dozen methods -- Lookup, Stats, IsInUse,
// ScopeClasses, RenderConceptListSKOS among them -- because they did not
// start with Get or List. A ratchet whose default is "unrecognized means
// ignored" is a list, not a ratchet: the names nobody thought of are exactly
// the ones that escape. Default deny means an unfamiliar name is a read
// until a mutation prefix says otherwise, so the cost of a wrong guess is a
// spurious allowlist entry rather than an invisible unscoped read.
// "Fork" and "Build" are deliberately NOT here. Both name reads as often as
// writes -- ForkOriginsForProject is a read on two slices -- and a mutation
// prefix that swallows a read reintroduces exactly the blind spot this
// denylist replaced. A fork or build mutation gets an allowlist entry
// instead, which is visible.
var mutationPrefixes = []string{
	"Create", "Update", "Delete", "Save", "Set", "Add", "Remove", "Reorder",
	"Activate", "Deprecate", "Adopt", "Clone", "Upsert", "Ensure",
	"Move", "Rename", "Publish", "Seal", "Unseal", "Import", "Apply",
	"Register", "Attach", "Detach", "Link", "Unlink", "Sync", "Rebuild",
	"Reconcile", "Enqueue", "Mark", "Touch", "Prune", "Purge", "Restore",
	"Replace", "Promote", "Demote", "Assign", "Revoke", "Invite", "Accept",
	"Reject", "Start", "Stop", "Cancel", "Retry", "Validate", "New",
	// With* are builders and option setters, not reads.
	"With",
}

// scanSliceReads parses <root>/<slice.Pkg>/service.go and returns every
// exported read-shaped method on *Service.
//
// Known limit, stated rather than implied: this checks the SIGNATURE. A
// method that takes auth.ReadScope and then ignores it passes. The signature
// is what makes an unscoped read unrepresentable at the call site, which is
// the property the spec asks for; whether the body honors it is what the
// slice's own tests are for.
func scanSliceReads(root string, s VersionedSlice) ([]ReadMethod, error) {
	path := filepath.Join(root, filepath.FromSlash(s.Pkg), "service.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	var out []ReadMethod
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
			continue // not a method
		}
		if !fn.Name.IsExported() || !isServiceReceiver(fn.Recv.List[0].Type) || !isReadShaped(fn.Name.Name) {
			continue
		}
		out = append(out, ReadMethod{
			Slice:    s.Name,
			Name:     fn.Name.Name,
			Line:     fset.Position(fn.Pos()).Line,
			HasScope: takesReadScope(fn.Type.Params),
		})
	}
	return out, nil
}

func isServiceReceiver(expr ast.Expr) bool {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == "Service"
}

// isReadShaped reports whether a method name denotes a read: anything that
// is not prefixed as a mutation.
func isReadShaped(name string) bool {
	for _, p := range mutationPrefixes {
		if strings.HasPrefix(name, p) {
			return false
		}
	}
	return true
}

// takesReadScope reports whether any parameter is auth.ReadScope. The
// package qualifier is matched loosely on the selector, so an aliased import
// of the auth package still counts.
func takesReadScope(params *ast.FieldList) bool {
	if params == nil {
		return false
	}
	for _, f := range params.List {
		sel, ok := f.Type.(*ast.SelectorExpr)
		if ok && sel.Sel != nil && sel.Sel.Name == "ReadScope" {
			return true
		}
	}
	return false
}
