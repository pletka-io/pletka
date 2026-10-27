package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishedOntologyVendorHasNoDatabaseCapability(t *testing.T) {
	path := filepath.Join(repoRoot(t), "pkg/service/gitmaterializer/ontology_vendor_values.go")
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range f.Imports {
		name := strings.Trim(imp.Path.Value, `"`)
		if strings.Contains(strings.Split(name, "/")[0], ".") && name != "github.com/pletka-io/pletka/pkg/database/ontologyrelease" {
			t.Errorf("published value writer imports capability %s", name)
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			if node.Recv != nil {
				t.Errorf("value writer %s must not retain a materializer receiver", node.Name)
			}
		case *ast.Ident:
			if node.Name == "Materializer" || node.Name == "NewFactory" {
				t.Errorf("value writer retains database access through %s", node.Name)
			}
		}
		return true
	})
}
