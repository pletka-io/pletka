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

// The browser's projection and all its shared helpers are value-only. New
// request, session or store access belongs in the source or capability layer.
func TestBrowserProjectionOnlyImportsValues(t *testing.T) {
	checkBrowserProjection(t, "detailview")
	checkBrowserProjection(t, "projectpage")
}

func checkBrowserProjection(t *testing.T, pkg string) {
	t.Helper()
	allowed := map[string]bool{"encoding/json": true, "fmt": true, "net/url": true, "slices": true, "sort": true, "strings": true, "time": true}
	for _, suffix := range []string{"browserinput", "domain", "projectsnapshot", "i18n", "formschema"} {
		allowed["github.com/pletka-io/pletka/pkg/"+suffix] = true
	}
	for _, file := range goFilesUnder(t, filepath.Join(repoRoot(t), "pkg/weave", pkg)) {
		if !strings.HasPrefix(filepath.Base(file), "projection") || strings.HasSuffix(file, "_test.go") {
			continue
		}
		syntax, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range syntax.Imports {
			name, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if !allowed[name] {
				t.Errorf("browser projection %s imports capability %s", file, name)
			}
		}
		ast.Inspect(syntax, func(node ast.Node) bool {
			if id, ok := node.(*ast.Ident); ok && id.Name == "Handler" {
				t.Errorf("projection %s can reach handler capabilities", file)
			}
			return true
		})
	}
}
