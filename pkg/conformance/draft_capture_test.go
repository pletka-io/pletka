package conformance

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// Capture is a storage boundary: extending the release inventory requires an
// explicit owner mapping and all three guards, even without a domain audit kind.
func TestDraftCaptureMatchesReleaseInventory(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "pkg/weave/release/testdata/capture_inventory.json"))
	if err != nil {
		t.Fatal(err)
	}
	var inventory struct {
		Sections []struct {
			Live    string `json:"live_table"`
			Archive string `json:"archive_table"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(data, &inventory); err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{}
	for _, s := range inventory.Sections {
		expected[s.Live] = s.Archive
	}
	data, err = os.ReadFile(filepath.Join(root, "pkg/database/migrations/042_atomic_draft_capture.sql"))
	if err != nil {
		t.Fatal(err)
	}
	actual := map[string]string{}
	for _, m := range regexp.MustCompile(`\('(weave_[a-z_]+)','(weave_[a-z_]+)','([^']*)','([^']*)','([^']*)'\)`).FindAllStringSubmatch(string(data), -1) {
		if (m[3] == "") == (m[4] == "") || (m[4] != "" && m[5] == "") {
			t.Fatalf("missing or ambiguous owner: %v", m)
		}
		if _, ok := actual[m[1]]; ok {
			t.Fatalf("repeated capture table %s", m[1])
		}
		actual[m[1]] = m[2]
	}
	if diff := cmp.Diff(expected, actual); diff != "" {
		t.Fatal(diff)
	}
}

// The SQL guards also reject missed adapters at runtime. This ratchet catches
// common accidental pool escapes at review time; it is not a SQL security model.
func TestDraftTransactionRouting(t *testing.T) {
	root := repoRoot(t)
	owners := map[string]bool{"pkg/weave/store.go": true, "pkg/service/draftcapture/service.go": true}
	borrowers := map[string]bool{"pkg/database/readsession/session.go": true, "pkg/weave/override/store_postgres.go": true}
	for _, path := range goFilesUnder(t, filepath.Join(root, "pkg")) {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(relative, "pkg/database/txscope/") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "txscope" {
				if sel.Sel.Name == "Within" && !owners[relative] {
					t.Errorf("%s cannot own transaction scopes", relative)
				}
				if sel.Sel.Name == "Current" && !owners[relative] && !borrowers[relative] {
					t.Errorf("%s cannot borrow parent transaction handles", relative)
				}
			}
			if !strings.HasPrefix(relative, "pkg/weave/") {
				return true
			}
			if isDraftPool(sel.X) {
				switch sel.Sel.Name {
				case "BeginTx":
					if relative == "pkg/weave/store.go" {
						return true
					}
				case "Acquire":
					if relative == "pkg/weave/override/store_postgres.go" {
						return true
					}
				case "Exec", "Query", "QueryRow", "Begin", "AcquireFunc":
				default:
					return true
				}
				t.Errorf("%s bypasses transaction routing: pool.%s", relative, sel.Sel.Name)
			}
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "sqlcgen" && sel.Sel.Name == "New" && len(call.Args) == 1 && isDraftPool(call.Args[0]) {
				t.Errorf("%s binds sqlc directly to a pool", relative)
			}
			return true
		})
	}
}
func isDraftPool(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.Ident:
		return strings.EqualFold(e.Name, "pool")
	case *ast.SelectorExpr:
		return strings.EqualFold(e.Sel.Name, "pool")
	}
	return false
}
