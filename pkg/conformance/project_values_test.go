package conformance

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

func TestProjectValuesCoverEveryReviewedColumnAndPreserveTypes(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "pkg/weave/release/testdata/capture_inventory.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ledger struct {
		Sections []struct {
			Archive    string            `json:"archive_table"`
			Type       string            `json:"archive_type"`
			Key        string            `json:"archive_key"`
			Projection map[string]string `json:"projection"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(raw, &ledger); err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(repoRoot(t), "pkg/database/sqlcgen/models.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	schema := map[string]map[string]string{}
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.TypeSpec)
		if !ok {
			return true
		}
		body, ok := spec.Type.(*ast.StructType)
		if !ok {
			return true
		}
		columns := map[string]string{}
		for _, field := range body.Fields.List {
			if field.Tag == nil {
				continue
			}
			tag, err := strconv.Unquote(field.Tag.Value)
			if err != nil {
				t.Fatal(err)
			}
			var typ bytes.Buffer
			if err := format.Node(&typ, token.NewFileSet(), field.Type); err != nil {
				t.Fatal(err)
			}
			columns[reflect.StructTag(tag).Get("json")] = typ.String()
		}
		schema[spec.Name.Name] = columns
		return true
	})
	sections := projectsnapshot.Coverage()
	content := reflect.TypeFor[projectsnapshot.Content]()
	if len(sections) != len(ledger.Sections) || content.NumField() != len(sections) {
		t.Fatal("section inventory differs from capture or typed content")
	}
	values := map[string]reflect.Type{}
	for i := range content.NumField() {
		f := content.Field(i)
		values[f.Tag.Get("json")] = f.Type.Elem()
	}
	for _, captured := range ledger.Sections {
		index := slices.IndexFunc(sections, func(s projectsnapshot.Section) bool { return s.Name == captured.Archive })
		if index < 0 {
			t.Fatalf("missing %s", captured.Archive)
		}
		section := sections[index]
		if section.Loader == "" || section.Portable == "" {
			t.Fatalf("%s lacks assembly or restore disposition", section.Name)
		}
		typ := values[section.Name]
		if typ == nil || typ.Name() != section.ValueType {
			t.Fatalf("missing typed section %s", section.Name)
		}
		if diff := cmp.Diff(strings.Fields(captured.Key), section.Key); diff != "" {
			t.Fatal(diff)
		}
		columns := []string{}
		jsonColumns := []string{}
		for i := range typ.NumField() {
			field := typ.Field(i)
			name := field.Tag.Get("json")
			if strings.Contains(name, ",") || name == "" {
				t.Fatalf("%s.%s loses explicit field presence", typ, field.Name)
			}
			columns = append(columns, name)
			if field.Type == reflect.TypeFor[[]byte]() {
				jsonColumns = append(jsonColumns, name)
			}
			want := schema[captured.Type][name]
			switch want {
			case "time.Time":
				want = "projectsnapshot.Timestamp"
			case "pgtype.Timestamptz":
				want = "*projectsnapshot.Timestamp"
			case "pgtype.Numeric":
				want = "*projectsnapshot.Decimal"
			case "pgtype.Date":
				want = "*projectsnapshot.Date"
			case "[]byte", "json.RawMessage":
				want = "[]uint8"
			}
			if field.Type.String() != want {
				t.Errorf("%s.%s loses storage type/nullability: %s want %s", typ, field.Name, field.Type, want)
			}
		}
		if diff := cmp.Diff(jsonColumns, section.JSONColumns); diff != "" {
			t.Errorf("%s unchecked JSON columns: %s", section.Name, diff)
		}
		expected := make([]string, 0, len(captured.Projection))
		for column := range captured.Projection {
			expected = append(expected, column)
		}
		slices.Sort(columns)
		slices.Sort(expected)
		slices.Sort(section.Columns)
		if diff := cmp.Diff(expected, columns); diff != "" {
			t.Errorf("%s field coverage: %s", section.Name, diff)
		}
		if diff := cmp.Diff(expected, section.Columns); diff != "" {
			t.Errorf("%s declared field mapping: %s", section.Name, diff)
		}
	}
}

func TestOnlyAssemblerConstructsCompleteProjectValues(t *testing.T) {
	root := repoRoot(t)
	for _, path := range goFilesUnder(t, root) {
		rel := mustRel(t, root, path)
		if strings.HasSuffix(rel, "_test.go") || strings.HasPrefix(rel, "pkg/projectsnapshot/") || strings.HasPrefix(rel, "pkg/service/projectread/") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		aliases := map[string]bool{}
		for _, imp := range file.Imports {
			name, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if name != "github.com/pletka-io/pletka/pkg/projectsnapshot" {
				continue
			}
			alias := "projectsnapshot"
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			if alias == "." {
				t.Errorf("%s dot-imports project construction", rel)
			}
			aliases[alias] = true
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if selector, ok := n.(*ast.SelectorExpr); ok && selector.Sel.Name == "NewComplete" {
				if name, ok := selector.X.(*ast.Ident); ok && aliases[name.Name] {
					t.Errorf("%s constructs complete content outside assembly", rel)
				}
			}
			return true
		})
	}
}
