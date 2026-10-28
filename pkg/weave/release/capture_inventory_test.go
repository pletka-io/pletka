package release

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// This reviewed ledger is independent of query generation. New tables/columns
// require capture/exclusion and domain/portable dispositions.
type captureLedger struct {
	Format      int               `json:"format"`
	Sections    []captureSection  `json:"sections"`
	OtherTables map[string]string `json:"other_tables"`
}

type captureSection struct {
	LiveTable    string            `json:"live_table"`
	ArchiveTable string            `json:"archive_table"`
	LiveType     string            `json:"live_type"`
	ArchiveType  string            `json:"archive_type"`
	QueryFile    string            `json:"query_file"`
	Query        string            `json:"query"`
	LiveColumns  string            `json:"live_columns"`
	Projection   map[string]string `json:"projection"`
	Excluded     map[string]string `json:"excluded"`
	ArchiveKey   string            `json:"archive_key"`
	Selection    string            `json:"selection"`
	Domain       string            `json:"domain"`
	Portable     string            `json:"portable"`
}

func loadCaptureLedger(t *testing.T) captureLedger {
	t.Helper()
	f, err := os.Open("testdata/capture_inventory.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var ledger captureLedger
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&ledger); err != nil {
		t.Fatal(err)
	}
	if ledger.Format != 1 || len(ledger.Sections) < 22 {
		t.Fatal("capture ledger lost required sections")
	}
	return ledger
}

type captureSQL struct{ file, name, body string }

func capturedQueries(t *testing.T) map[string]captureSQL {
	t.Helper()
	result := map[string]captureSQL{}
	add := func(q captureSQL) {
		table, ok := insertTarget(q.body)
		if !ok || !strings.HasSuffix(table, "_archive") {
			return
		}
		if _, exists := result[table]; exists {
			t.Fatalf("duplicate capture for %s", table)
		}
		result[table] = q
	}
	for _, stmt := range snapshotStatements {
		add(captureSQL{"pkg/weave/release/service.go", "", stmt})
	}
	for _, file := range []string{"pkg/database/queries/weave_release_snapshots.sql", "pkg/database/queries/weave_release_dependencies.sql"} {
		data, err := os.ReadFile("../../../" + file)
		if err != nil {
			t.Fatal(err)
		}
		for _, part := range strings.Split(string(data), "-- name: ")[1:] {
			header, body, ok := strings.Cut(part, "\n")
			if !ok {
				t.Fatal("invalid named SQL")
			}
			add(captureSQL{file, strings.Fields(header)[0], body})
		}
	}
	return result
}

func generatedColumnShapes(t *testing.T) map[string][]string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "../../database/sqlcgen/models.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	result := map[string][]string{}
	ast.Inspect(f, func(n ast.Node) bool {
		typ, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := typ.Type.(*ast.StructType)
		if !ok {
			return true
		}
		for _, field := range st.Fields.List {
			if field.Tag == nil {
				continue
			}
			tag, err := strconv.Unquote(field.Tag.Value)
			if err != nil {
				t.Fatal(err)
			}
			result[typ.Name.Name] = append(result[typ.Name.Name], reflect.StructTag(tag).Get("json"))
		}
		return false
	})
	return result
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func checkColumns(t *testing.T, name string, actual, want []string) {
	t.Helper()
	actual = slices.Clone(actual)
	want = slices.Clone(want)
	slices.Sort(actual)
	slices.Sort(want)
	if !slices.Equal(actual, want) {
		t.Fatalf("%s changed without an inventory decision: got %v; want %v", name, actual, want)
	}
}

// Split reviewed SQL lists without losing commas inside COALESCE or DISTINCT ON.
// This fail-closed test parser is not a runtime SQL interpreter.
func splitCaptureExpressions(t *testing.T, sql string) []string {
	t.Helper()
	var out []string
	start, depth := 0, 0
	var quote byte
	for i := 0; i < len(sql); i++ {
		c := sql[i]
		if quote != 0 {
			if c == quote {
				if i+1 < len(sql) && sql[i+1] == quote {
					i++
				} else {
					quote = 0
				}
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(sql[start:i]))
				start = i + 1
			}
		}
		if depth < 0 {
			t.Fatal("unbalanced SQL expression")
		}
	}
	if quote != 0 || depth != 0 {
		t.Fatal("unterminated SQL expression")
	}
	return append(out, strings.TrimSpace(sql[start:]))
}

func TestReleaseCaptureColumnInventory(t *testing.T) {
	ledger := loadCaptureLedger(t)
	queries := capturedQueries(t)
	shapes := generatedColumnShapes(t)
	for _, section := range ledger.Sections {
		t.Run(section.LiveTable, func(t *testing.T) {
			checkColumns(t, section.LiveType, shapes[section.LiveType], strings.Fields(section.LiveColumns))
			checkColumns(t, section.ArchiveType, shapes[section.ArchiveType], sortedKeys(section.Projection))
			for _, col := range strings.Fields(section.LiveColumns) {
				_, copied := section.Projection[col]
				reason, excluded := section.Excluded[col]
				if copied == excluded || (excluded && reason == "") {
					t.Fatalf("%s needs exactly one capture/exclusion disposition", col)
				}
			}
			for col := range section.Excluded {
				if !slices.Contains(strings.Fields(section.LiveColumns), col) {
					t.Fatalf("stale exclusion: %s", col)
				}
			}
			if section.Domain == "" || section.Portable == "" || section.Selection == "" || section.ArchiveKey == "" {
				t.Fatal("missing ownership, identity or consumer disposition")
			}
			q, ok := queries[section.ArchiveTable]
			if !ok || q.name != section.Query || q.file != section.QueryFile {
				t.Fatal("capture site moved or disappeared; review inventory")
			}
			cols, ok := insertColumns(q.body)
			if !ok {
				t.Fatal("missing capture columns")
			}
			values, ok := selectList(q.body)
			if !ok {
				t.Fatal("missing capture projection")
			}
			distinct := ""
			if strings.HasPrefix(values, "DISTINCT ON (") {
				end := strings.Index(values, ")")
				if end < 0 {
					t.Fatal("invalid DISTINCT ON")
				}
				distinct = values[:end+1]
				values = strings.TrimSpace(values[end+1:])
			}
			columns, expressions := splitCaptureExpressions(t, cols), splitCaptureExpressions(t, values)
			if len(columns) != len(expressions) || len(columns) != len(section.Projection) {
				t.Fatal("capture projection omitted/added columns")
			}
			seen := map[string]bool{}
			for i, col := range columns {
				col = strings.Trim(col, "\"")
				if seen[col] || expressions[i] != section.Projection[col] {
					t.Errorf("%s capture = %q, want %q", col, expressions[i], section.Projection[col])
				}
				seen[col] = true
			}
			_, scope, ok := strings.Cut(q.body, "FROM")
			if !ok {
				t.Fatal("missing ownership scope")
			}
			scope, _, _ = strings.Cut(scope, "ON CONFLICT")
			selection := strings.Join(strings.Fields("SELECT "+distinct+" FROM "+strings.TrimSuffix(strings.TrimSpace(scope), ";")), " ")
			if selection != section.Selection {
				t.Errorf("capture selection changed:\n%s\nwant:\n%s", selection, section.Selection)
			}
			delete(queries, section.ArchiveTable)
		})
	}
	if len(queries) != 0 {
		t.Fatalf("unclassified capture queries: %v", queries)
	}
	for table, reason := range ledger.OtherTables {
		if table == "" || reason == "" {
			t.Fatal("unexplained table exclusion")
		}
	}
}

func TestNamedCaptureQueriesRemainWired(t *testing.T) {
	var source strings.Builder
	for _, file := range []string{"service.go", "store_snapshot.go"} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		source.Write(data)
	}
	for _, section := range loadCaptureLedger(t).Sections {
		if section.Query != "" && !regexp.MustCompile(`\.`+regexp.QuoteMeta(section.Query)+`\(`).MatchString(source.String()) {
			t.Errorf("%s is no longer wired into the cut", section.Query)
		}
	}
}
