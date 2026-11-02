package projectread

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

func discoverySQL(t *testing.T, state string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "database", "queries", "read_"+state, "discovery.sql"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, part := range strings.Split(string(raw), "-- source: ")[1:] {
		name := strings.Fields(part)[0]
		if out[name] != "" {
			t.Fatalf("duplicate discovery source %s", name)
		}
		out[name] = strings.Split(part, "\nedges AS (")[0]
	}
	return out
}

func TestIdentityDiscoveryUsesTheContentSelectors(t *testing.T) {
	for _, state := range []string{"draft", "archive"} {
		projection := discoverySQL(t, state)
		raw, err := os.ReadFile(filepath.Join("..", "..", "database", "queries", "read_"+state, "content.sql"))
		if err != nil {
			t.Fatal(err)
		}
		content := map[string]string{}
		for _, part := range strings.Split(string(raw), "-- name: ")[1:] {
			content[strings.Fields(part)[0]] = part
		}
		for _, binding := range Bindings() {
			kinds, registered := discoveryKinds()[binding.Section]
			if !registered {
				t.Fatalf("new section %s needs an identity-discovery policy", binding.Section)
			}
			// Fork provenance has no structural edge. Metadata is loaded once
			// during final materialization, under the independent graph validator.
			expected := len(kinds) > 0 && binding.Query != "ContentForks"
			query, exists := projection[binding.Query]
			if exists != expected {
				t.Fatalf("%s/%s identity source coverage changed", state, binding.Query)
			}
			if !exists {
				continue
			}
			original := content[binding.Query]
			start, end := strings.Index(original, "\nFROM "), strings.LastIndex(original, "\nORDER BY ")
			if start < 0 || end <= start {
				t.Fatalf("cannot locate selector for %s", binding.Query)
			}
			selector := strings.ReplaceAll(original[start:end], "sqlc.arg(restricted)::boolean", "true")
			gates := slices.Clone(kinds)
			slices.Sort(gates)
			for i, kind := range gates {
				gates[i] = "cardinality(sqlc.arg(selected_" + kind + ")::text[]) > 0"
			}
			expectedSource := selector + "\n  AND (" + strings.Join(gates, " OR ") + ")\n),"
			actualStart := strings.Index(query, "\nFROM ")
			if actualStart < 0 || strings.TrimSpace(query[actualStart:]) != strings.TrimSpace(expectedSource) {
				t.Fatalf("%s/%s discovery changed its ownership, source join or selector", state, binding.Query)
			}
			delete(projection, binding.Query)
		}
		if len(projection) != 0 {
			t.Fatalf("unregistered identity sources: %v", projection)
		}
	}
}

// Populate every scalar column, then ask the independent pure oracle which ones
// carry references. A newly added reference cannot silently stay out of SQL just
// because an older integration fixture leaves its new column empty.
func TestIdentityProjectionCoversEveryReferenceColumn(t *testing.T) {
	for _, state := range []string{"draft", "archive"} {
		queries := discoverySQL(t, state)
		for _, binding := range Bindings() {
			for _, fallback := range []bool{false, true} {
				content := projectsnapshot.Content{}
				rows := reflect.ValueOf(&content).Elem().FieldByName(binding.Field)
				row := reflect.New(rows.Type().Elem()).Elem()
				columns := map[string]string{}
				for i := range row.NumField() {
					column := strings.Split(row.Type().Field(i).Tag.Get("json"), ",")[0]
					value := row.Field(i)
					if value.Kind() == reflect.Pointer {
						value.Set(reflect.New(value.Type().Elem()))
						value = value.Elem()
					}
					switch value.Kind() {
					case reflect.String:
						value.SetString("probe_" + column)
						columns[value.String()] = column
					case reflect.Int, reflect.Int32, reflect.Int64:
						value.SetInt(int64(10000 + i))
						columns[fmt.Sprint(value.Interface())] = column
					}
					if column == "entity_type" || column == "ref_type" {
						value.SetString("field")
					}
					if fallback && column == "target_id" {
						value.SetString("")
					}
				}
				rows.Set(reflect.Append(rows, row))
				refs, err := projectsnapshot.RequiredEntities(content)
				if err != nil {
					t.Fatalf("%s oracle: %v", binding.Section, err)
				}
				for _, ids := range refs {
					for _, id := range ids {
						column, ok := columns[id]
						if !ok || !strings.Contains(queries[binding.Query], "s."+column) {
							t.Fatalf("%s/%s lacks reference column %s (%s)", state, binding.Query, column, id)
						}
					}
				}
			}
		}
	}
}
