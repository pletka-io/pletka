package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/service/projectread"
)

func TestProjectAssemblyCoversEverySectionAndField(t *testing.T) {
	root := repoRoot(t)
	bindings := projectread.Bindings()
	sections := projectsnapshot.Coverage()
	if len(bindings) != len(sections) {
		t.Fatal("assembly inventory differs from capture values")
	}
	content := reflect.TypeFor[projectsnapshot.Content]()
	converted := map[string]string{"Collections": "collection", "Overrides": "override", "OverrideRefs": "override", "Placements": "override", "Vocabularies": "vocabulary", "VocabularyEntries": "vocabulary", "ConceptLists": "vocabulary", "ConceptEntries": "vocabulary", "Broader": "vocabulary", "Examples": "example", "ExampleValues": "example"}
	seen := map[string]bool{}
	for _, binding := range bindings {
		if seen[binding.Section] {
			t.Fatal("duplicate assembly section")
		}
		seen[binding.Section] = true
		i := slices.IndexFunc(sections, func(s projectsnapshot.Section) bool { return s.Name == binding.Section })
		if i < 0 {
			t.Fatalf("unclassified loader %s", binding.Section)
		}
		section := sections[i]
		field, ok := content.FieldByName(binding.Field)
		if !ok || field.Tag.Get("json") != section.Name || field.Type.Elem().Name() != binding.ValueType {
			t.Fatalf("wrong assembly destination: %+v", binding)
		}
		if section.Loader != binding.Reader+"."+binding.Query {
			t.Fatal("value mapping lacks implemented loader disposition")
		}
		owner, isConverted := converted[binding.Field]
		if binding.Transitional == isConverted || isConverted && binding.Owner != owner {
			t.Fatalf("converted slice downgraded: %+v", binding)
		}
		if binding.Owner == "" || binding.Replacement == "" || binding.Removal == "" {
			t.Fatal("adapter lifecycle missing")
		}
		for _, state := range []string{"draft", "archive"} {
			queryPath := filepath.Join(root, "pkg/database/queries/read_"+state, "content.sql")
			raw, err := os.ReadFile(queryPath)
			if err != nil {
				t.Fatal(err)
			}
			marker := "-- name: " + binding.Query + " :many\n"
			_, rest, ok := strings.Cut(string(raw), marker)
			if !ok {
				t.Fatalf("missing query %s", binding.Query)
			}
			body := strings.Split(rest, "-- name:")[0]
			projection := strings.Split(body, "\nFROM")[0]
			cols := []string{}
			for _, m := range regexp.MustCompile(`\bAS ([a-z_]+)`).FindAllStringSubmatch(projection, -1) {
				cols = append(cols, m[1])
			}
			want := append([]string{}, section.Columns...)
			slices.Sort(cols)
			slices.Sort(want)
			if diff := cmp.Diff(want, cols); diff != "" {
				t.Errorf("%s %s SQL projection: %s", state, binding.Query, diff)
			}
			if !strings.Contains(body, "sqlc.arg(project_id)") || !strings.Contains(body, "LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint") || !strings.Contains(body, "ORDER BY") {
				t.Fatalf("unbounded or unowned query %s", binding.Query)
			}
			if state == "archive" && !strings.Contains(body, "s.version_number=sqlc.arg(version)::text") {
				t.Fatalf("unbound version %s", binding.Query)
			}
			file := binding.DraftFile
			if state == "archive" {
				file = binding.ArchiveFile
			}
			checkLosslessMapper(t, root, file, state, binding, field.Type.Elem())
		}
	}
}

func checkLosslessMapper(t *testing.T, root, path, state string, binding projectread.Binding, record reflect.Type) {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, path), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var method *ast.FuncDecl
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == binding.Query {
			method = fn
		}
	}
	if method == nil {
		t.Fatalf("missing bound mapper %s.%s", path, binding.Query)
	}
	values := map[string]string{}
	queryFound := false
	ownerBound := false
	versionBound := state == "draft"
	ast.Inspect(method.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == binding.Query {
				queryFound = true
			}
		}
		literal, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		typ, ok := literal.Type.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		for _, element := range literal.Elts {
			kv, ok := element.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			if typ.Sel.Name == binding.Query+"Params" {
				sel, ok := kv.Value.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				receiver, ok := sel.X.(*ast.Ident)
				if !ok || receiver.Name != "r" {
					continue
				}
				if key.Name == "ProjectID" && sel.Sel.Name == "projectID" {
					ownerBound = true
				}
				if key.Name == "Version" && sel.Sel.Name == "version" {
					versionBound = true
				}
			}
			if typ.Sel.Name != binding.ValueType {
				continue
			}
			ast.Inspect(kv.Value, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok {
					if id, ok := sel.X.(*ast.Ident); ok && id.Name == "row" {
						values[key.Name] = sel.Sel.Name
					}
				}
				return true
			})
		}
		return true
	})
	if !queryFound || !ownerBound || !versionBound {
		t.Fatalf("mapper %s lacks bound query parameters", binding.Query)
	}
	source := generatedContentColumns(t, root, state, binding.Query)
	if len(values) != record.NumField() {
		t.Fatalf("%s mapper loses fields: %d want %d", binding.Query, len(values), record.NumField())
	}
	for i := range record.NumField() {
		f := record.Field(i)
		if source[values[f.Name]] != f.Tag.Get("json") {
			t.Errorf("%s.%s is mapped from wrong or absent column %s", binding.ValueType, f.Name, values[f.Name])
		}
	}
}

func generatedContentColumns(t *testing.T, root, state, query string) map[string]string {
	t.Helper()
	files := make([]*ast.File, 0, 2)
	types := map[string]*ast.StructType{}
	resultName := ""
	for _, name := range []string{"content.sql.go", "models.go"} {
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "pkg/database/readsql", state, name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
		ast.Inspect(f, func(n ast.Node) bool {
			if spec, ok := n.(*ast.TypeSpec); ok {
				if body, ok := spec.Type.(*ast.StructType); ok {
					types[spec.Name.Name] = body
				}
			}
			return true
		})
	}
	for _, f := range files {
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == query {
				array, ok := fn.Type.Results.List[0].Type.(*ast.ArrayType)
				if ok {
					resultName = array.Elt.(*ast.Ident).Name
				}
			}
		}
	}
	body := types[resultName]
	if body == nil {
		t.Fatalf("missing generated row for %s", query)
	}
	columns := map[string]string{}
	for _, f := range body.Fields.List {
		tag, err := strconv.Unquote(f.Tag.Value)
		if err != nil {
			t.Fatal(err)
		}
		columns[f.Names[0].Name] = reflect.StructTag(tag).Get("json")
	}
	return columns
}

func TestProjectAssemblerCannotAcquireAnotherReadCapability(t *testing.T) {
	root := repoRoot(t)
	for _, path := range goFilesUnder(t, filepath.Join(root, "pkg/service/projectread")) {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if strings.Contains(p, "pgx") || strings.Contains(p, "/database/") && !strings.HasSuffix(p, "/readsession") && !strings.HasSuffix(p, "/projectontology") {
				t.Errorf("assembler acquired runtime query capability %s", p)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if s, ok := n.(*ast.SelectorExpr); ok {
				switch s.Sel.Name {
				case "NewFactory", "DraftQueries", "ArchiveQueries", "BeginTx", "At":
					t.Errorf("assembler rebinds content using %s", s.Sel.Name)
				}
			}
			return true
		})
	}
}
