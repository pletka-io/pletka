package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestOntologyPublicationHasOnePolicyBoundary(t *testing.T) {
	root := repoRoot(t)
	for _, dir := range []string{"pkg", "cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if strings.HasPrefix(rel, "pkg/database/sqlcgen/") {
				return nil
			}
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			for _, decl := range f.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				ast.Inspect(fn, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok || sel.Sel.Name != "importVersion" {
						return true
					}
					expected := "true"
					if fn.Name.Name == "StageLegacyVersion" {
						expected = "false"
					}
					valid := rel == "pkg/weave/ontology/store_postgres.go" && (fn.Name.Name == "ImportVersion" || fn.Name.Name == "StageLegacyVersion") && len(call.Args) == 3
					if valid {
						choice, ok := call.Args[2].(*ast.Ident)
						valid = ok && choice.Name == expected
					}
					if !valid {
						t.Errorf("%s:%s bypasses the two explicit import lifecycle entry points", rel, fn.Name.Name)
					}
					return true
				})
			}
			ast.Inspect(f, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				switch sel.Sel.Name {
				case "WeaveProjectOntologySelections", "WeavePublishedOntologyIdentity", "WeaveProjectOntologySourceInventories", "WeaveProjectOntologySourcePins", "WeaveInsertProjectOntologyPin", "WeaveMarkProjectOntologyInventory":
					if rel != "pkg/database/ontologyrelease/project_capture.go" {
						t.Errorf("%s bypasses project ontology capture", rel)
					}
				case "StageLegacyVersion":
					if rel != "pkg/weave/ontology/import_vendored.go" {
						t.Errorf("%s bypasses automatic publication via legacy staging", rel)
					}
				case "WeaveOntologyPublicationSource", "WeaveOntologyPublicationBody", "WeaveOntologyPublicationCandidates":
					if rel != "pkg/database/ontologyrelease/publish.go" && rel != "pkg/database/ontologyrelease/publish_current.go" && rel != "pkg/database/ontologyrelease/project_capture.go" {
						t.Errorf("%s reads raw ontology publications; use a bound release reader", rel)
					}
				case "WeaveRestoreOntologyArtifact":
					if rel != "pkg/database/ontologyrelease/restore.go" {
						t.Errorf("%s bypasses the artifact restore boundary", rel)
					}
				case "WeavePublishOntology":
					if rel != "pkg/database/ontologyrelease/publish.go" {
						t.Errorf("%s bypasses ontology publication policy", rel)
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
