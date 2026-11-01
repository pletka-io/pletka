package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// CSV serializers may not grow new data access around the single complete source.
// Current access checks stay in the HTTP handler; projection files remain pure.
func TestProjectCSVUsesDetachedContent(t *testing.T) {
	for _, path := range goFilesUnder(t, filepath.Join(repoRoot(t), "pkg/weave/project/csvexport")) {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		syntax, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(path)
		pure := strings.HasPrefix(name, "projection") || name == "writers.go" || name == "zip.go"
		for _, imp := range syntax.Imports {
			pkg, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(pkg, "/pkg/database") || strings.Contains(pkg, "/pkg/service") || strings.Contains(pkg, "pgx") || pkg == "database/sql" || strings.Contains(pkg, "/internal/test") {
				t.Errorf("%s imports loading capability %s", name, pkg)
			}
			if strings.Contains(pkg, "/pkg/weave/") && !strings.HasSuffix(pkg, "/apierror") && !strings.HasSuffix(pkg, "/errresp") {
				t.Errorf("%s imports peer slice %s", name, pkg)
			}
			if pure && (pkg == "net/http" || strings.HasSuffix(pkg, "/auth")) {
				t.Errorf("%s projection holds request capability %s", name, pkg)
			}
		}
		ast.Inspect(syntax, func(n ast.Node) bool {
			selector, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if strings.HasPrefix(selector.Sel.Name, "Weave") && strings.HasSuffix(selector.Sel.Name, "Store") {
				t.Errorf("%s retains aggregate store %s", name, selector.Sel.Name)
			}
			return true
		})
	}
}
