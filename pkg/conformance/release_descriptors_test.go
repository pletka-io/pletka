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

// Keep released identity writers off all live actor/host helpers. The shared
// receipt serializers receive captured values; they cannot acquire a materializer.
func TestReleasedIdentityWritersCannotUseLiveDescriptors(t *testing.T) {
	required := map[string]bool{
		"loadProjectManifestAtVersion": false, "loadPletkaModManifestAtVersion": false,
		"loadPletkaProjectRequirementsAtVersion": false, "loadPletkaOntologyRequirementsAtVersion": false,
		"writeAdoptionManifestsVersion": false, "writeForkManifestsVersion": false,
	}
	for _, path := range goFilesUnder(t, filepath.Join(repoRoot(t), "pkg/service/gitmaterializer")) {
		if strings.HasSuffix(path, "_test.go") {
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
			if _, ok := required[fn.Name.Name]; !ok {
				continue
			}
			required[fn.Name.Name] = true
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				switch sel.Sel.Name {
				case "loadManifestActor", "WeaveGetActorByID", "projectModulePath", "ontologyModulePath", "artifactRequirement", "moduleHost", "ontologyHost", "encodeAdoptionReceipt", "encodeForkReceipt":
					t.Errorf("%s uses live descriptor capability %s", fn.Name.Name, sel.Sel.Name)
				}
				return true
			})
		}
	}
	for name, seen := range required {
		if !seen {
			t.Errorf("guarded identity writer disappeared: %s", name)
		}
	}
}

func TestReleaseDescriptorsHaveOneCaptureDoor(t *testing.T) {
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
		aliases := map[string]bool{}
		for _, imp := range file.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if path != "github.com/pletka-io/pletka/pkg/database/releasedescriptor" {
				continue
			}
			name := "releasedescriptor"
			if imp.Name != nil {
				name = imp.Name.Name
			}
			if name == "." {
				t.Errorf("%s hides descriptor capture behind a dot import", rel)
			}
			aliases[name] = true
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name == "WeaveInsertReleaseDescriptors" && rel != "pkg/database/releasedescriptor/capture.go" {
				t.Errorf("%s bypasses descriptor capture", rel)
			}
			if id, ok := sel.X.(*ast.Ident); ok && aliases[id.Name] && (sel.Sel.Name == "Capture" || sel.Sel.Name == "CaptureLegacy") && rel != "pkg/weave/release/capture_evidence.go" {
				t.Errorf("%s captures descriptors outside release creation", rel)
			}
			return true
		})
	}
}
