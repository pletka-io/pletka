package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestLegacyCapturePolicyHasOnlyExplicitLifecycleEntry(t *testing.T) {
	root := repoRoot(t)
	for _, path := range goFilesUnder(t, root) {
		rel := mustRel(t, root, path)
		if strings.HasSuffix(rel, "_test.go") || strings.HasPrefix(rel, "pkg/database/sqlcgen/") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				if literal, ok := n.(*ast.CompositeLit); ok {
					if name, ok := literal.Type.(*ast.Ident); ok && name.Name == "legacyEvidence" && (rel != "pkg/weave/release/service.go" || fn.Name.Name != "createLegacyInTx") {
						t.Errorf("%s:%s constructs legacy evidence outside its lifecycle", rel, fn.Name.Name)
					}
				}
				if sel, ok := n.(*ast.SelectorExpr); ok {
					if sel.Sel.Name == "createLegacyInTx" && (rel != "pkg/weave/release/rebuild.go" || fn.Name.Name != "legacyRebuild") {
						t.Errorf("%s:%s bypasses explicit legacy rebuild", rel, fn.Name.Name)
					}
					if sel.Sel.Name == "WeaveFinalizeLegacyReleaseCapture" && rel != "pkg/weave/release/capture_evidence.go" {
						t.Errorf("%s bypasses centralized capture finalization", rel)
					}
				}
				return true
			})
		}
	}
}
