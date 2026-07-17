package frontendmanifest_test

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/frontendmanifest"
	"gopkg.in/yaml.v3"
)

func TestCoreFrontendManifestMatchesSourceInventory(t *testing.T) {
	root := repoRoot(t)
	catalog := loadCoreCatalog(t, root)
	if err := catalog.ValidateFiles(); err != nil {
		t.Fatalf("core manifest source validation failed: %v", err)
	}

	islandDir := filepath.Join(root, "frontend", "src", "islands")
	entries, err := os.ReadDir(islandDir)
	if err != nil {
		t.Fatalf("read island dir: %v", err)
	}
	var want []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ts") {
			continue
		}
		want = append(want, strings.TrimSuffix(entry.Name(), ".ts"))
	}
	sort.Strings(want)
	if got := catalog.Names(frontendmanifest.KindIsland); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("manifest islands do not match frontend/src/islands\n got: %v\nwant: %v", got, want)
	}
}

func TestCoreFrontendManifestCoversBackendReferences(t *testing.T) {
	root := repoRoot(t)
	catalog := loadCoreCatalog(t, root)
	var refs []frontendmanifest.Reference
	refs = append(refs, collectFrontendRefCalls(t, root)...)
	refs = append(refs, collectTemplateIslandScriptRefs(t, root)...)
	refs = append(refs, collectGoIslandMountRefs(t, root)...)
	refs = append(refs, collectContentPageWidgetRefs(t, root)...)
	refs = append(refs, collectLiteralRefs(t, root, frontendmanifest.KindEntityListRowWidget, []string{
		`RowWidget:\s*"([^"]+)"`,
		`Widget:\s*"(default|editorial)"`,
	})...)
	refs = append(refs, collectLiteralRefs(t, root, frontendmanifest.KindEntityListEditorWidget, []string{
		`Widget:\s*"(example-workspace)"`,
	})...)

	if err := catalog.ValidateReferences(refs); err != nil {
		t.Fatal(err)
	}
}

func TestPlatformExampleFrontendManifest(t *testing.T) {
	root := repoRoot(t)
	core, err := frontendmanifest.LoadFile(filepath.Join(root, "frontend", "core.frontend.json"), true)
	if err != nil {
		t.Fatalf("load core frontend manifest: %v", err)
	}
	platform, err := frontendmanifest.LoadFile(filepath.Join(root, "frontend", "examples", "platform.frontend.json"), false)
	if err != nil {
		t.Fatalf("load platform example frontend manifest: %v", err)
	}
	catalog, err := frontendmanifest.NewCatalog(core, platform)
	if err != nil {
		t.Fatalf("build combined frontend catalog: %v", err)
	}
	if err := catalog.ValidateFiles(); err != nil {
		t.Fatalf("combined frontend source validation failed: %v", err)
	}
	if err := catalog.ValidateReferences([]frontendmanifest.Reference{
		{Kind: frontendmanifest.KindIsland, Name: "platform-dashboard", Source: "frontend/examples"},
		{Kind: frontendmanifest.KindFormWidget, Name: "platform-lookup", Source: "frontend/examples"},
		{Kind: frontendmanifest.KindFormWidget, Name: "select", Source: "frontend/examples override"},
		{Kind: frontendmanifest.KindEntityListRowWidget, Name: "platform-card", Source: "frontend/examples"},
	}); err != nil {
		t.Fatalf("platform example references failed validation: %v", err)
	}
}

func TestBackendFrontendReferencesUseMarkers(t *testing.T) {
	root := repoRoot(t)
	violations := collectRawFrontendReferenceLiterals(t, root)
	if len(violations) > 0 {
		t.Fatalf("backend frontend references must use frontendrefs marker helpers:\n- %s", strings.Join(violations, "\n- "))
	}
}

func TestFormWidgetConstantsHaveManifestMarkers(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "pkg", "formschema", "types.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse formschema types: %v", err)
	}

	widgets := make(map[string]string)
	markers := make(map[string]bool)
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range valueSpec.Names {
				if !strings.HasPrefix(name.Name, "Widget") || name.Name == "WidgetHidden" {
					continue
				}
				if i >= len(valueSpec.Values) {
					continue
				}
				value, ok := stringLiteralValue(valueSpec.Values[i])
				if ok {
					widgets[name.Name] = value
				}
			}
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !isFrontendRefCall(call, "FormWidget") || len(call.Args) == 0 {
			return true
		}
		value, ok := stringLiteralValue(call.Args[0])
		if ok {
			markers[value] = true
		}
		return true
	})

	var missing []string
	for name, value := range widgets {
		if !markers[value] {
			missing = append(missing, name+" = "+strconv.Quote(value))
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("form widget constants missing frontendrefs.FormWidget markers:\n- %s", strings.Join(missing, "\n- "))
	}
}

func loadCoreCatalog(t *testing.T, root string) *frontendmanifest.Catalog {
	t.Helper()
	manifest, err := frontendmanifest.LoadFile(filepath.Join(root, "frontend", "core.frontend.json"), true)
	if err != nil {
		t.Fatalf("load core frontend manifest: %v", err)
	}
	catalog, err := frontendmanifest.NewCatalog(manifest)
	if err != nil {
		t.Fatalf("build core frontend catalog: %v", err)
	}
	return catalog
}

func collectRawFrontendReferenceLiterals(t *testing.T, root string) []string {
	t.Helper()
	var violations []string
	for _, dir := range []string{"pkg", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}

			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(node ast.Node) bool {
				lit, ok := node.(*ast.CompositeLit)
				if !ok {
					return true
				}
				if isIslandMountType(lit.Type) {
					collectRawIslandMountLiterals(&violations, root, path, fset, lit)
				}
				if isEntityListSchemaType(lit.Type) {
					collectRawEntityListSchemaLiterals(&violations, root, path, fset, lit)
				}
				if isViewModeType(lit.Type) {
					collectRawWidgetKeyLiteral(&violations, root, path, fset, lit, "frontendrefs.EntityListRowWidget")
				}
				if isEntityListEditorType(lit.Type) {
					collectRawWidgetKeyLiteral(&violations, root, path, fset, lit, "frontendrefs.EntityListEditorWidget")
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("scan raw frontend reference literals in %s: %v", dir, err)
		}
	}
	sort.Strings(violations)
	return violations
}

func collectRawIslandMountLiterals(violations *[]string, root, path string, fset *token.FileSet, lit *ast.CompositeLit) {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "Name":
			appendRawLiteralViolation(violations, root, path, fset, kv.Value, "frontendrefs.Island")
		case "Dependencies":
			deps, ok := kv.Value.(*ast.CompositeLit)
			if !ok {
				continue
			}
			for _, dep := range deps.Elts {
				appendRawLiteralViolation(violations, root, path, fset, dep, "frontendrefs.Island")
			}
		}
	}
}

func collectRawEntityListSchemaLiterals(violations *[]string, root, path string, fset *token.FileSet, lit *ast.CompositeLit) {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "RowWidget":
			appendRawLiteralViolation(violations, root, path, fset, kv.Value, "frontendrefs.EntityListRowWidget")
		case "Editor":
			collectRawEditorLiteral(violations, root, path, fset, kv.Value)
		case "ViewModes":
			collectRawViewModeLiterals(violations, root, path, fset, kv.Value)
		}
	}
}

func collectRawViewModeLiterals(violations *[]string, root, path string, fset *token.FileSet, expr ast.Expr) {
	list, ok := expr.(*ast.CompositeLit)
	if !ok {
		return
	}
	for _, elt := range list.Elts {
		viewMode, ok := elt.(*ast.CompositeLit)
		if !ok {
			continue
		}
		collectRawWidgetKeyLiteral(violations, root, path, fset, viewMode, "frontendrefs.EntityListRowWidget")
	}
}

func collectRawEditorLiteral(violations *[]string, root, path string, fset *token.FileSet, expr ast.Expr) {
	if unary, ok := expr.(*ast.UnaryExpr); ok {
		expr = unary.X
	}
	editor, ok := expr.(*ast.CompositeLit)
	if !ok {
		return
	}
	collectRawWidgetKeyLiteral(violations, root, path, fset, editor, "frontendrefs.EntityListEditorWidget")
}

func collectRawWidgetKeyLiteral(violations *[]string, root, path string, fset *token.FileSet, lit *ast.CompositeLit, helper string) {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Widget" {
			continue
		}
		appendRawLiteralViolation(violations, root, path, fset, kv.Value, helper)
	}
}

func appendRawLiteralViolation(violations *[]string, root, path string, fset *token.FileSet, expr ast.Expr, helper string) {
	value, ok := stringLiteralValue(expr)
	if !ok || value == "" {
		return
	}
	*violations = append(*violations, sourcePos(root, path, fset, expr.Pos())+" uses raw "+strconv.Quote(value)+"; wrap with "+helper+"(...)")
}

func collectFrontendRefCalls(t *testing.T, root string) []frontendmanifest.Reference {
	t.Helper()
	var refs []frontendmanifest.Reference
	for _, dir := range []string{"pkg", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}

			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := selector.X.(*ast.Ident)
				if !ok || pkg.Name != "frontendrefs" {
					return true
				}
				kind, ok := frontendRefKind(selector.Sel.Name)
				if !ok {
					return true
				}
				name, ok := stringLiteralValue(call.Args[0])
				if !ok {
					return true
				}
				source := sourcePos(root, path, fset, call.Args[0].Pos())
				refs = append(refs, frontendmanifest.Reference{Kind: kind, Name: name, Source: source})
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("scan frontendrefs in %s: %v", dir, err)
		}
	}
	return refs
}

func frontendRefKind(name string) (frontendmanifest.Kind, bool) {
	switch name {
	case "GlobalEntry":
		return frontendmanifest.KindGlobalEntry, true
	case "Island":
		return frontendmanifest.KindIsland, true
	case "FormWidget":
		return frontendmanifest.KindFormWidget, true
	case "EntityListRowWidget":
		return frontendmanifest.KindEntityListRowWidget, true
	case "EntityListEditorWidget":
		return frontendmanifest.KindEntityListEditorWidget, true
	case "ContentWidget":
		return frontendmanifest.KindContentWidget, true
	default:
		return "", false
	}
}

func isFrontendRefCall(call *ast.CallExpr, name string) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && pkg.Name == "frontendrefs" && selector.Sel.Name == name
}

func collectTemplateIslandScriptRefs(t *testing.T, root string) []frontendmanifest.Reference {
	t.Helper()
	re := regexp.MustCompile(`islandScripts\s+"([^"]+)"`)
	var refs []frontendmanifest.Reference
	walkFiles(t, filepath.Join(root, "pkg"), func(path string, body string) {
		if !strings.HasSuffix(path, ".gohtml") {
			return
		}
		for _, match := range re.FindAllStringSubmatch(body, -1) {
			kind := frontendmanifest.KindIsland
			if strings.HasPrefix(match[1], "global-") {
				kind = frontendmanifest.KindGlobalEntry
			}
			refs = append(refs, frontendmanifest.Reference{Kind: kind, Name: match[1], Source: rel(root, path)})
		}
	})
	return refs
}

func collectGoIslandMountRefs(t *testing.T, root string) []frontendmanifest.Reference {
	t.Helper()
	var refs []frontendmanifest.Reference
	for _, dir := range []string{"pkg", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}

			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(node ast.Node) bool {
				lit, ok := node.(*ast.CompositeLit)
				if !ok || !isIslandMountType(lit.Type) {
					return true
				}
				for _, elt := range lit.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					key, ok := kv.Key.(*ast.Ident)
					if !ok {
						continue
					}
					switch key.Name {
					case "Name":
						appendStringRef(&refs, root, path, fset, kv.Value)
					case "Dependencies":
						deps, ok := kv.Value.(*ast.CompositeLit)
						if !ok {
							continue
						}
						for _, dep := range deps.Elts {
							appendStringRef(&refs, root, path, fset, dep)
						}
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("scan island mounts in %s: %v", dir, err)
		}
	}
	return refs
}

func collectContentPageWidgetRefs(t *testing.T, root string) []frontendmanifest.Reference {
	t.Helper()
	type contentFrontmatter struct {
		Blocks []struct {
			Type string `yaml:"type"`
		} `yaml:"blocks"`
	}

	var refs []frontendmanifest.Reference
	pagesDir := filepath.Join(root, "pkg", "weave", "content", "pages")
	walkFiles(t, pagesDir, func(path string, body string) {
		if !strings.HasSuffix(path, ".md") {
			return
		}
		fmBytes, ok := markdownFrontmatter([]byte(body))
		if !ok {
			return
		}
		var fm contentFrontmatter
		if err := yaml.Unmarshal(fmBytes, &fm); err != nil {
			t.Fatalf("parse content frontmatter %s: %v", rel(root, path), err)
		}
		for _, block := range fm.Blocks {
			refs = append(refs, frontendmanifest.Reference{
				Kind:   frontendmanifest.KindContentWidget,
				Name:   block.Type,
				Source: rel(root, path),
			})
		}
	})
	return refs
}

func markdownFrontmatter(body []byte) ([]byte, bool) {
	if !bytes.HasPrefix(body, []byte("---\n")) {
		return nil, false
	}
	rest := body[len("---\n"):]
	idx := bytes.Index(rest, []byte("\n---"))
	if idx < 0 {
		return nil, false
	}
	return rest[:idx], true
}

func isIslandMountType(expr ast.Expr) bool {
	return typeName(expr) == "IslandMount"
}

func isEntityListSchemaType(expr ast.Expr) bool {
	return typeName(expr) == "EntityListSchema"
}

func isViewModeType(expr ast.Expr) bool {
	return typeName(expr) == "ViewMode"
}

func isEntityListEditorType(expr ast.Expr) bool {
	return typeName(expr) == "EntityListEditor"
}

func typeName(expr ast.Expr) string {
	switch typ := expr.(type) {
	case *ast.Ident:
		return typ.Name
	case *ast.SelectorExpr:
		return typ.Sel.Name
	default:
		return ""
	}
}

func appendStringRef(refs *[]frontendmanifest.Reference, root, path string, fset *token.FileSet, expr ast.Expr) {
	name, ok := stringLiteralValue(expr)
	if !ok {
		return
	}
	*refs = append(*refs, frontendmanifest.Reference{Kind: frontendmanifest.KindIsland, Name: name, Source: sourcePos(root, path, fset, expr.Pos())})
}

func stringLiteralValue(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

func sourcePos(root, path string, fset *token.FileSet, pos token.Pos) string {
	source := rel(root, path)
	if p := fset.Position(pos); p.Line > 0 {
		source += ":" + strconv.Itoa(p.Line)
	}
	return source
}

func collectLiteralRefs(t *testing.T, root string, kind frontendmanifest.Kind, patterns []string) []frontendmanifest.Reference {
	t.Helper()
	var refs []frontendmanifest.Reference
	var regexes []*regexp.Regexp
	for _, pattern := range patterns {
		regexes = append(regexes, regexp.MustCompile(pattern))
	}
	for _, dir := range []string{"pkg", "cmd"} {
		walkFiles(t, filepath.Join(root, dir), func(path string, body string) {
			if !strings.HasSuffix(path, ".go") {
				return
			}
			for _, re := range regexes {
				for _, match := range re.FindAllStringSubmatch(body, -1) {
					refs = append(refs, frontendmanifest.Reference{Kind: kind, Name: match[1], Source: rel(root, path)})
				}
			}
		})
	}
	return refs
}

func walkFiles(t *testing.T, root string, visit func(path string, body string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		visit(path, string(body))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working dir: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func rel(root, path string) string {
	out, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(out)
}
