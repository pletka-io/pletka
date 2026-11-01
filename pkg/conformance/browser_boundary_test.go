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
	checkBrowserProjectionPath(t, filepath.Join(repoRoot(t), "pkg/browserhttp"))
}

func checkBrowserProjection(t *testing.T, pkg string) {
	t.Helper()
	checkBrowserProjectionPath(t, filepath.Join(repoRoot(t), "pkg/weave", pkg))
}
func checkBrowserProjectionPath(t *testing.T, dir string) {
	t.Helper()
	allowed := map[string]bool{"encoding/json": true, "fmt": true, "net/url": true, "slices": true, "sort": true, "strings": true, "time": true}
	for _, suffix := range []string{"browserinput", "domain", "projectsnapshot", "i18n", "formschema"} {
		allowed["github.com/pletka-io/pletka/pkg/"+suffix] = true
	}
	for _, file := range goFilesUnder(t, dir) {
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

// Inventory HTTP entry points have one source. Per-function store fallbacks cannot return.
func TestInventoryHandlersOnlyDelegateToSharedBoundary(t *testing.T) {
	for slice, names := range map[string][]string{"model": {"List", "ScopeClassesOptions"}, "collection": {"List", "ScopeClassesOptions", "CategoriesOptions"}, "field": {"List", "OntologyScopesFilter", "CategoriesFilter"}, "vocabulary": {"ListProjectConceptLists", "ConceptListVocabularyFilterOptions"}, "example": {"List"}} {
		path := filepath.Join(repoRoot(t), "pkg/weave", slice, "handler.go")
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil {
				continue
			}
			for _, name := range names {
				if fn.Name.Name != name {
					continue
				}
				seen[name] = true
				if len(fn.Body.List) != 1 {
					t.Errorf("%s.%s must only delegate to the shared inventory handler", slice, name)
					continue
				}
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					if sel, ok := n.(*ast.SelectorExpr); ok {
						if recv, ok := sel.X.(*ast.Ident); ok && recv.Name == "h" && sel.Sel.Name != "inventory" {
							t.Errorf("%s.%s reaches %s", slice, name, sel.Sel.Name)
						}
					}
					return true
				})
			}
		}
		for _, name := range names {
			if !seen[name] {
				t.Errorf("missing protected inventory method %s.%s", slice, name)
			}
		}
	}
}

// Shared browser handlers cannot acquire a slice store or editing service.
func TestBrowserHTTPHasNoContentStoreCapabilities(t *testing.T) {
	for _, file := range goFilesUnder(t, filepath.Join(repoRoot(t), "pkg/browserhttp")) {
		if strings.HasSuffix(file, "_test.go") {
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
			if strings.Contains(name, "/database/") || strings.Contains(name, "/service/") || strings.Contains(name, "/pkg/weave/") && !strings.HasSuffix(name, "/apierror") && !strings.HasSuffix(name, "/publication") {
				t.Errorf("browser boundary %s imports content capability %s", file, name)
			}
		}
	}
}
