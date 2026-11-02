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

func TestArtifactStreamCapabilitiesStayInsideBindingInfrastructure(t *testing.T) {
	root := repoRoot(t)
	allowed := map[string]string{
		"github.com/pletka-io/pletka/pkg/database/internal/legacyartifact":    "pkg/database/readsession/session.go",
		"github.com/pletka-io/pletka/pkg/database/internal/legacyartifactsql": "pkg/database/internal/legacyartifact/reader.go",
	}
	for _, path := range goFilesUnder(t, filepath.Join(root, "pkg")) {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			name, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if owner, guarded := allowed[name]; guarded && filepath.ToSlash(mustRel(t, root, path)) != owner {
				t.Errorf("%s bypasses the bound artifact reader through %s", path, name)
			}
		}
	}
}

func TestArtifactStreamExecutesOnlyItsGeneratedQuery(t *testing.T) {
	path := filepath.Join(repoRoot(t), "pkg/database/internal/legacyartifact/reader.go")
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	ast.Inspect(f, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		method, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || method.Sel.Name != "Query" {
			return true
		}
		calls++
		if len(call.Args) != 5 {
			t.Error("artifact query changed parameter shape")
			return true
		}
		query, ok := call.Args[1].(*ast.SelectorExpr)
		if !ok {
			t.Error("artifact stream bypasses generated query")
			return true
		}
		pkg, ok := query.X.(*ast.Ident)
		if !ok || pkg.Name != "legacyartifactsql" || query.Sel.Name != "ReadLegacyArtifactChunks" {
			t.Error("artifact stream acquired another query")
		}
		return true
	})
	if calls != 1 {
		t.Fatalf("artifact stream has %d query calls, want one", calls)
	}
}
