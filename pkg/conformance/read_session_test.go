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

// This registry is explicit: adding an unrelated legacy exception cannot silently
// remove a converted method from the gate. Inspect every file, not just service.go.
func TestPilotReadMethodsUseBoundSessions(t *testing.T) {
	converted := map[string][]string{
		"collection": {"List", "Get", "Stats", "ScopeClasses", "CategoriesUsed", "BatchCompositionCounts", "ListOverrides"},
		"override":   {"ListForEntity", "ListForField", "GetByID", "GetRefs", "ListPlacements"},
	}
	legacy := map[string]string{
		"collection.ListAdoptedByReference": "foreign reference membership awaits dependency closure",
		"collection.ListAdoptedExplicit":    "receipt store and source resolution await dependency session",
		"collection.ForkOriginsForProject":  "fork store has not joined the session",
		"collection.LookupByID":             "legacy aggregate/generator caller has no bound project",
		"collection.LookupView":             "legacy resolved-field assembler",
		"collection.AnchorIndex":            "cross-project discovery for legacy generator",
		"collection.View":                   "legacy resolved-field assembler",
		"collection.ForkFromSource":         "write orchestration; source conversion follows dependency resolver",
		"override.EntityFingerprint":        "write-side optimistic concurrency check",
	}
	for slice, names := range converted {
		seen := map[string]bool{}
		for _, path := range goFilesUnder(t, filepath.Join(repoRoot(t), "pkg/weave", slice)) {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv == nil || !isServiceReceiver(fn.Recv.List[0].Type) || !fn.Name.IsExported() || !isReadShaped(fn.Name.Name) {
					continue
				}
				key := slice + "." + fn.Name.Name
				if legacy[key] != "" {
					continue
				}
				seen[fn.Name.Name] = true
				usesSession := false
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					sel, ok := n.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					if inner, ok := sel.X.(*ast.SelectorExpr); ok {
						if inner.Sel.Name == "reads" && sel.Sel.Name == "Read" {
							usesSession = true
						}
						if inner.Sel.Name == "store" || inner.Sel.Name == "overrides" {
							t.Errorf("%s bypasses its read session through %s", key, inner.Sel.Name)
						}
					}
					if sel.Sel.Name == "At" || sel.Sel.Name == "requireOwn" {
						t.Errorf("%s uses legacy/live read %s", key, sel.Sel.Name)
					}
					return true
				})
				if !usesSession {
					t.Errorf("%s must use a bound read session", key)
				}
			}
		}
		for _, name := range names {
			if !seen[name] {
				t.Errorf("converted method %s.%s disappeared or was exempted", slice, name)
			}
		}
	}
}

func TestPilotReaderCapabilitiesAreStateSpecific(t *testing.T) {
	root := repoRoot(t)
	parameterCounts := map[string]map[string]int{
		"collection": {"Get": 2, "List": 2, "Usage": 2, "ScopeClasses": 1, "CategoriesUsed": 1, "CompositionCounts": 2},
		"override":   {"ListForEntity": 3, "ListForField": 2, "ListByType": 2, "GetRefs": 2, "GetByID": 2, "GetBase": 2, "ListPlacements": 2},
	}
	for _, slice := range []string{"collection", "override"} {
		for _, state := range []string{"draft", "archive"} {
			path := filepath.Join(root, "pkg/weave", slice, "reader_"+state+".go")
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			aliases := map[string]string{}
			for _, imp := range f.Imports {
				p, _ := strconv.Unquote(imp.Path.Value)
				name := filepath.Base(p)
				if imp.Name != nil {
					name = imp.Name.Name
				}
				aliases[name] = p
				if strings.Contains(p, "pgxpool") || p == "database/sql" {
					t.Errorf("%s holds an unrestricted DB import", path)
				}
				if strings.Contains(p, "/readsql/") && !strings.HasSuffix(p, "/"+state) {
					t.Errorf("%s imports the other state's queries", path)
				}
			}
			ast.Inspect(f, func(n ast.Node) bool {
				if st, ok := n.(*ast.StructType); ok {
					for _, field := range st.Fields.List {
						if id, ok := field.Type.(*ast.Ident); ok && id.Name == "string" {
							continue
						}
						sel, ok := field.Type.(*ast.SelectorExpr)
						if ok {
							if id, ok := sel.X.(*ast.Ident); ok && aliases[id.Name] == "github.com/pletka-io/pletka/pkg/database/readsql/"+state && sel.Sel.Name == "Querier" {
								continue
							}
						}
						t.Errorf("%s reader field must be a bound string or state-specific SELECT capability", path)
					}
				}
				if call, ok := n.(*ast.CallExpr); ok {
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
						if id, ok := sel.X.(*ast.Ident); ok && strings.Contains(aliases[id.Name], "/database/") && (sel.Sel.Name == "New" || strings.Contains(sel.Sel.Name, "Factory")) {
							t.Errorf("%s constructs its own DB capability", path)
						}
					}
				}
				return true
			})
		}
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "pkg/weave", slice, "read_sessions.go"), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok || ts.Name.Name != "Reader" {
				return true
			}
			iface := ts.Type.(*ast.InterfaceType)
			for _, method := range iface.Methods.List {
				if len(method.Names) != 1 {
					t.Errorf("%s.Reader must not embed an unrestricted interface", slice)
					continue
				}
				fn, ok := method.Type.(*ast.FuncType)
				if !ok {
					t.Errorf("%s.Reader contains a non-method capability", slice)
					continue
				}
				count := 0
				for _, param := range fn.Params.List {
					n := len(param.Names)
					if n == 0 {
						n = 1
					}
					count += n
				}
				if want, ok := parameterCounts[slice][method.Names[0].Name]; !ok || count != want {
					t.Errorf("%s.Reader.%s changed its bound signature: %d parameters", slice, method.Names[0].Name, count)
				}
				for _, param := range fn.Params.List {
					for _, name := range param.Names {
						if name.Name == "projectID" || name.Name == "version" || name.Name == "scope" {
							t.Errorf("%s.Reader can be retargeted by %s", slice, name.Name)
						}
					}
				}
			}
			return true
		})
	}
}

func TestPilotQueriesCannotCrossStateOrWrite(t *testing.T) {
	tables := regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+(weave_[a-z_]+)`)
	writes := regexp.MustCompile(`(?i)\b(?:INSERT|UPDATE|DELETE|MERGE|TRUNCATE|ALTER|CREATE|DROP)\b`)
	for _, state := range []string{"draft", "archive"} {
		paths, err := filepath.Glob(filepath.Join(repoRoot(t), "pkg/database/queries/read_"+state, "*.sql"))
		if err != nil {
			t.Fatal(err)
		}
		if len(paths) == 0 {
			t.Fatalf("no %s pilot queries found", state)
		}
		for _, path := range paths {
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var sql strings.Builder
			for _, line := range strings.Split(string(body), "\n") {
				if !strings.HasPrefix(strings.TrimSpace(line), "--") {
					sql.WriteString(line + "\n")
				}
			}
			if writes.MatchString(sql.String()) {
				t.Errorf("%s contains a write", path)
			}
			for _, match := range tables.FindAllStringSubmatch(sql.String(), -1) {
				isArchive := strings.HasSuffix(match[1], "_archive") || match[1] == "weave_release_descriptors" || match[1] == "weave_legacy_ontology_snapshots"
				// This session facet reads only the release's capture certificate,
				// never mutable project content from the release catalog.
				isArchive = isArchive || (match[1] == "weave_releases" && filepath.Base(path) == "metadata.sql")
				if isArchive != (state == "archive") {
					t.Errorf("%s accesses other state table %s", path, match[1])
				}
			}
		}
	}
}

func TestPilotQueryPackagesAreOnlyUsedByBindingInfrastructure(t *testing.T) {
	root := repoRoot(t)
	for _, path := range goFilesUnder(t, root) {
		rel := mustRel(t, root, path)
		if strings.HasSuffix(rel, "_test.go") || strings.HasPrefix(rel, "pkg/database/readsql/") || (rel == "pkg/database/readsession/session.go" || rel == "pkg/database/readsession/descriptors.go" || rel == "pkg/database/readsession/metadata.go" || rel == "pkg/database/readsession/legacy_content.go" || rel == "pkg/database/readsession/ontologies.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if !strings.Contains(p, "/database/readsql/") {
				allowedWrite := p == "github.com/pletka-io/pletka/pkg/database/writesql/ontologydraft" && (rel == "pkg/weave/ontology/draft_session.go" || rel == "pkg/weave/ontology/draft_editor.go" || rel == "pkg/weave/ontology/draft_publish.go")
				if strings.Contains(p, "/database/writesql/") && !allowedWrite {
					t.Errorf("%s imports unrestricted draft writes", rel)
				}
				continue
			}
			allowed := rel == "pkg/weave/collection/reader_draft.go" || rel == "pkg/weave/collection/reader_archive.go" || rel == "pkg/weave/override/reader_draft.go" || rel == "pkg/weave/override/reader_archive.go"
			if p == "github.com/pletka-io/pletka/pkg/database/readsql/ontologydraft" && (rel == "pkg/weave/ontology/draft_reader.go" || rel == "pkg/weave/ontology/draft_session.go" || rel == "pkg/weave/ontology/draft_publish.go") {
				allowed = true
			}
			if p == "github.com/pletka-io/pletka/pkg/database/readsql/ontologybody" && (rel == "pkg/database/ontologybody/session.go" || rel == "pkg/database/ontologybody/reader.go") {
				allowed = true
			}
			if p == "github.com/pletka-io/pletka/pkg/database/readsql/ontologyrelease" && (rel == "pkg/database/ontologyrelease/session.go" || rel == "pkg/database/ontologyrelease/reader.go") {
				allowed = true
			}
			if p == "github.com/pletka-io/pletka/pkg/database/readsql/projectontologies" && (rel == "pkg/database/readsession/ontologies.go" || rel == "pkg/database/readsession/session.go") {
				allowed = true
			}
			if p == "github.com/pletka-io/pletka/pkg/database/readsql/importedontologysnapshot" && (rel == "pkg/database/ontologysnapshot/imported_reader.go" || rel == "pkg/database/ontologysnapshot/imported_session.go") {
				continue
			}
			if p == "github.com/pletka-io/pletka/pkg/database/readsql/ontologysnapshot" && (rel == "pkg/database/ontologysnapshot/reader.go" || rel == "pkg/database/ontologysnapshot/session.go") {
				allowed = true
			}
			if !allowed {
				t.Errorf("%s imports raw pilot queries; use a bound slice reader", rel)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			// Include method values as well as direct calls so assigning a
			// query accessor to a local variable cannot bypass this boundary.
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name != "DraftQueries" && sel.Sel.Name != "ArchiveQueries" {
				return true
			}
			if rel != "pkg/weave/collection/read_sessions.go" && rel != "pkg/weave/override/read_sessions.go" {
				t.Errorf("%s obtains raw session queries instead of a bound reader", rel)
			}
			return true
		})
	}
}
