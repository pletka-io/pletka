package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestDependencyPlanningCannotAcquireWriteCapabilities(t *testing.T) {
	for _, name := range []string{"planner.go", "closure.go", "draft_reader.go", "archive_reader.go"} {
		path := filepath.Join(repoRoot(t), "pkg/database/releasedeps", name)
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if strings.Contains(p, "pgxpool") || p == "database/sql" || strings.Contains(p, "writesql") || p == "github.com/pletka-io/pletka/pkg/database/readsession" {
				t.Errorf("%s imports database construction %s", name, p)
			}
			if (name == "closure.go" || name == "archive_reader.go") && strings.Contains(p, "dependencydraft") {
				t.Errorf("%s has draft query capability", name)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				switch sel.Sel.Name {
				case "Queries", "DBTX", "Tx", "Pool", "New", "Begin", "BeginTx", "Connect", "Open", "Capture":
					t.Errorf("%s acquires unrestricted capability %s", name, sel.Sel.Name)
				}
				if strings.HasPrefix(sel.Sel.Name, "WeaveInsert") || strings.HasPrefix(sel.Sel.Name, "WeaveMark") {
					t.Errorf("%s can persist a read plan", name)
				}
				if name == "closure.go" || name == "archive_reader.go" {
					if strings.Contains(sel.Sel.Name, "Latest") || strings.Contains(sel.Sel.Name, "Draft") {
						t.Errorf("%s resolves released content from current policy", name)
					}
				}
			}
			return true
		})
	}
}

func TestDependencyQueryPackagesAreSelectOnlyAndStateSpecific(t *testing.T) {
	queryNames := regexp.MustCompile(`-- name: (\w+)`)
	allowed := map[string]map[string]bool{
		"draft":   {"WeaveDraftDependencyReferences": true, "WeaveDraftDependencyEntityOwner": true, "WeaveLatestDependencyRelease": true, "WeaveDraftFollowingDependencies": true},
		"archive": {"WeaveReleaseDependencyState": true, "WeaveListReleaseProjectDependencies": true, "WeaveGetReleaseDependencyEntity": true, "WeaveListReleaseDependencyEntities": true, "WeaveReleaseContainsDependencyEntity": true},
	}
	writes := regexp.MustCompile(`(?i)\b(?:INSERT|UPDATE|DELETE|MERGE|TRUNCATE|CALL|CREATE|ALTER|DROP)\b`)
	tables := regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+(weave_[a-z_]+)`)
	for _, state := range []string{"draft", "archive"} {
		paths, err := filepath.Glob(filepath.Join(repoRoot(t), "pkg/database/queries/read_dependency_"+state, "*.sql"))
		if err != nil || len(paths) == 0 {
			t.Fatalf("query sources missing: %v", err)
		}
		for _, path := range paths {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range queryNames.FindAllStringSubmatch(string(data), -1) {
				if !allowed[state][name[1]] {
					t.Errorf("%s adds an unreviewed %s dependency capability", path, name[1])
				}
			}
			lines := []string{}
			for _, line := range strings.Split(string(data), "\n") {
				if !strings.HasPrefix(strings.TrimSpace(line), "--") {
					lines = append(lines, line)
				}
			}
			sql := strings.Join(lines, "\n")
			if writes.MatchString(sql) {
				t.Errorf("%s contains a write", path)
			}
			for _, m := range tables.FindAllStringSubmatch(sql, -1) {
				captured := strings.HasSuffix(m[1], "_archive") || m[1] == "weave_release_project_dependencies" || m[1] == "weave_release_dependency_entities"
				if m[1] != "weave_releases" && captured != (state == "archive") {
					t.Errorf("%s crosses state through %s", path, m[1])
				}
			}
		}
	}
	path := filepath.Join(repoRoot(t), "pkg/database/queries/read_project_access/access.sql")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := []string{}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			lines = append(lines, line)
		}
	}
	if strings.Join(strings.Fields(strings.Join(lines, " ")), " ") != "SELECT id, owner_id, visibility FROM weave_projects WHERE id = @project_id::text;" {
		t.Fatal("source authorization gained content columns or a different policy source")
	}
}
