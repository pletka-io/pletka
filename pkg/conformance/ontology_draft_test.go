package conformance

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// A draft capability must never acquire a pool, a live Store, another owner, or
// publication methods. Keep the allowance structural so new methods fail closed.
func TestOntologyDraftCapabilitiesAreBoundAndSeparate(t *testing.T) {
	root := repoRoot(t)
	allowed := map[string]map[string]bool{
		"draftReader": {"readsql.Querier": true, "DraftReference": true, "bool": true},
		"draftEditor": {"*draftReader": true, "writesql.Querier": true, "int64": true},
		"DraftReader": {"func(context.Context) (DraftSnapshot, error)": true},
		"DraftEditor": {"DraftReader": true, "func(context.Context, DraftDocument) (DraftSnapshot, error)": true},
	}
	for _, name := range []string{"draft_reader.go", "draft_editor.go"} {
		path := filepath.Join(root, "pkg/weave/ontology", name)
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if strings.Contains(p, "/database/") && p != "github.com/pletka-io/pletka/pkg/database/readsql/ontologydraft" && (name != "draft_editor.go" || p != "github.com/pletka-io/pletka/pkg/database/writesql/ontologydraft") || strings.Contains(p, "pgxpool") || p == "database/sql" {
				t.Errorf("%s imports unrestricted capability %s", name, p)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if ts, ok := n.(*ast.TypeSpec); ok {
				fields := (*ast.FieldList)(nil)
				switch v := ts.Type.(type) {
				case *ast.StructType:
					fields = v.Fields
				case *ast.InterfaceType:
					fields = v.Methods
				}
				if fields == nil {
					t.Errorf("unexpected capability type %s", ts.Name)
					return true
				}
				for _, field := range fields.List {
					var b bytes.Buffer
					if err := format.Node(&b, token.NewFileSet(), field.Type); err != nil {
						t.Fatal(err)
					}
					if !allowed[ts.Name.Name][b.String()] {
						t.Errorf("%s retains unrestricted field/method %s", ts.Name, b.String())
					}
					if _, ok := ts.Type.(*ast.InterfaceType); ok && len(field.Names) > 0 && field.Names[0].Name != "Snapshot" && field.Names[0].Name != "Save" {
						t.Errorf("%s gained capability %s", ts.Name, field.Names[0].Name)
					}
				}
			}
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && (sel.Sel.Name == "New" || strings.Contains(sel.Sel.Name, "Factory")) {
					t.Errorf("%s constructs its own capability", name)
				}
			}
			return true
		})
	}
}

func TestOntologyDraftQueriesCannotReachVersionTables(t *testing.T) {
	tables := regexp.MustCompile(`(?i)\b(?:FROM|JOIN|UPDATE|INTO)\s+(weave_[a-z_]+)`)
	mutations := regexp.MustCompile(`(?i)\b(?:INSERT|DELETE|UPDATE|TRUNCATE|CALL)\b`)
	for _, dir := range []string{"read_ontology_draft", "write_ontology_draft"} {
		paths, err := filepath.Glob(filepath.Join(repoRoot(t), "pkg/database/queries", dir, "*.sql"))
		if err != nil || len(paths) == 0 {
			t.Fatalf("missing draft queries: %v", err)
		}
		for _, path := range paths {
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			sql := string(body)
			for _, match := range tables.FindAllStringSubmatch(sql, -1) {
				isReceipt := dir == "read_ontology_draft" && match[1] == "weave_ontology_draft_publications"
				if match[1] != "weave_ontology_drafts" && !isReceipt {
					t.Errorf("draft query reaches non-draft table %s", match[1])
				}
			}
			if dir == "read_ontology_draft" && mutations.MatchString(sql) {
				t.Errorf("draft reader can write")
			}
			for _, query := range strings.Split(sql, "-- name:")[1:] {
				if !strings.Contains(query, "ontology_id = @ontology_id") || !strings.Contains(query, "id = @draft_id") {
					t.Errorf("draft query lacks bound owner/identity: %s", path)
				}
			}
		}
	}
}

// HTTP and schema adapters may coordinate the lifecycle but must not regain
// writable stores or construct DB capabilities while rendering a form.
func TestOntologyEditorAdaptersCannotAcquireStores(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(repoRoot(t), "pkg/weave/ontology/editor_*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if strings.Contains(p, "/sqlcgen") || strings.Contains(p, "/readsql/") || strings.Contains(p, "/writesql/") || strings.Contains(p, "pgx") || p == "database/sql" {
				t.Errorf("editor adapter imports DB capability %s", p)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && (id.Name == "Store" || id.Name == "postgresStore" || id.Name == "NewPostgresStore" || id.Name == "NewDraftService" || id.Name == "writeImportedVersion" || id.Name == "publishImportedOntology") {
				t.Errorf("editor adapter accesses unrestricted lifecycle implementation %s", id.Name)
			}
			return true
		})
	}
}
