package conformance

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// This ratchets the reviewed gaps, not a claim that domain audit payloads are
// complete storage patches. New captured sections and new emitting audit kinds
// require an explicit review instead of falling through the legacy writer.
func TestDraftWriteInventoryCoversCapturedSectionsAndActualAuditKinds(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "pkg/service/gitmaterializer/testdata/draft_write_inventory.json"))
	if err != nil {
		t.Fatal(err)
	}
	var review []struct {
		Section    string   `json:"section"`
		Paths      []string `json:"write_paths"`
		AuditKinds []string `json:"observed_audit_kinds"`
		Review     string   `json:"review"`
	}
	if err := json.Unmarshal(raw, &review); err != nil {
		t.Fatal(err)
	}
	sections, kinds := map[string]bool{}, map[string]bool{}
	for _, row := range review {
		if sections[row.Section] || len(row.Paths) == 0 || row.Review == "" || row.AuditKinds == nil {
			t.Fatalf("incomplete or repeated draft write review: %+v", row)
		}
		sections[row.Section] = true
		for _, path := range row.Paths {
			if _, err := os.Stat(filepath.Join(root, path)); err != nil {
				t.Fatal(err)
			}
		}
		for _, kind := range row.AuditKinds {
			kinds[kind] = true
		}
	}
	if len(sections) != len(projectsnapshot.Coverage()) {
		t.Fatal("captured section count differs from reviewed write paths")
	}
	for _, section := range projectsnapshot.Coverage() {
		if !sections[section.Name] {
			t.Errorf("missing write review for %s", section.Name)
		}
	}
	emitted := map[string]bool{}
	for _, path := range goFilesUnder(t, filepath.Join(root, "pkg")) {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			selector, ok := lit.Type.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "ChangeLogEntry" {
				return true
			}
			for _, entry := range lit.Elts {
				kv, ok := entry.(*ast.KeyValueExpr)
				if !ok {
					t.Errorf("%s uses an unreviewable positional audit entry", path)
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok || key.Name != "EntityType" {
					continue
				}
				kind := auditStringLiteral(t, path, kv.Value)
				emitted[kind] = true
			}
			return true
		})
	}
	if diff := cmp.Diff(kinds, emitted); diff != "" {
		t.Fatalf("reviewed versus emitting audit kinds: %s", diff)
	}
	if diff := cmp.Diff(emitted, acceptedDraftAuditKinds(t, root)); diff != "" {
		t.Fatalf("emitting versus accepted audit kinds: %s", diff)
	}
}

func acceptedDraftAuditKinds(t *testing.T, root string) map[string]bool {
	t.Helper()
	constants := map[string]string{}
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "pkg/service/gitmaterializer/closure.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Values) != 1 || len(spec.Names) != 1 {
			return true
		}
		if lit, ok := spec.Values[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			constants[spec.Names[0].Name] = auditStringLiteral(t, "closure constant", lit)
		}
		return true
	})
	handled := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok && fn.Name.Name != "closure" {
			return false
		}
		sw, ok := n.(*ast.SwitchStmt)
		if !ok {
			return true
		}
		tag, ok := sw.Tag.(*ast.SelectorExpr)
		if !ok || tag.Sel.Name != "EntityType" {
			return true
		}
		for _, statement := range sw.Body.List {
			for _, expr := range statement.(*ast.CaseClause).List {
				handled[auditKindExpression(t, expr, constants)] = true
			}
		}
		return false
	})
	file, err = parser.ParseFile(token.NewFileSet(), filepath.Join(root, "pkg/service/gitmaterializer/draft_audit.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	accepted := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || !slices.ContainsFunc(spec.Names, func(id *ast.Ident) bool { return id.Name == "legacyDraftAuditKinds" }) {
			return true
		}
		lit, ok := spec.Values[0].(*ast.CompositeLit)
		if !ok {
			t.Fatal("legacy audit vocabulary is no longer explicit")
		}
		for _, element := range lit.Elts {
			kv := element.(*ast.KeyValueExpr)
			accepted[auditKindExpression(t, kv.Key, constants)] = true
		}
		return false
	})
	if diff := cmp.Diff(handled, accepted); diff != "" {
		t.Fatalf("closure dispatch versus accepted audit kinds: %s", diff)
	}
	return accepted
}

func auditKindExpression(t *testing.T, expr ast.Expr, constants map[string]string) string {
	t.Helper()
	if key, ok := expr.(*ast.Ident); ok {
		kind, exists := constants[key.Name]
		if !exists {
			t.Fatalf("unknown audit constant %s", key.Name)
		}
		return kind
	}
	return auditStringLiteral(t, "legacy audit vocabulary", expr)
}

func auditStringLiteral(t *testing.T, path string, expr ast.Expr) string {
	t.Helper()
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		t.Fatalf("%s has a dynamic audit kind requiring explicit review", path)
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
