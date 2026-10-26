package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// Capture may inspect working rows and latest policy. Resolution of an existing
// release must retain only its two captured-membership capabilities.
func TestReleasedDependencyResolverHasNoLiveCapability(t *testing.T) {
	path := filepath.Join(repoRoot(t), "pkg/database/releasedeps/resolver.go")
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{
		"WeaveReleaseDependencyState":     true,
		"WeaveGetReleaseDependencyEntity": true,
	}
	seen := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.InterfaceType:
			for _, method := range node.Methods.List {
				if len(method.Names) == 0 {
					id, ok := method.Type.(*ast.Ident)
					if !ok || id.Name != "inventoryReader" {
						t.Error("resolver embeds unrestricted query capability")
					}
					continue
				}
				for _, name := range method.Names {
					if !allowed[name.Name] {
						t.Errorf("resolver gains query %s", name.Name)
					}
					seen[name.Name] = true
				}
			}
		case *ast.SelectorExpr:
			switch node.Sel.Name {
			case "New", "Queries", "DBTX", "Tx", "Pool", "Begin", "BeginTx", "Connect", "Open":
				t.Errorf("resolver gains unrestricted database access through %s", node.Sel.Name)
			}
		}
		return true
	})
	for name := range allowed {
		if !seen[name] {
			t.Errorf("missing restricted resolver capability %s", name)
		}
	}
}
