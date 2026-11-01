package projectread

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

func TestDiscoverySelectorsCoverEveryDraftAndArchiveQuery(t *testing.T) {
	registry := discoveryKinds()
	if len(registry) != len(Bindings()) {
		t.Fatal("discovery coverage differs from assembly")
	}
	parameter := regexp.MustCompile(`sqlc.arg\(selected_([a-z_]+)\)`)
	for _, kind := range []string{"draft", "archive"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "database", "queries", "read_"+kind, "content.sql"))
		if err != nil {
			t.Fatal(err)
		}
		queries := map[string]string{}
		for _, part := range strings.Split(string(data), "-- name: ")[1:] {
			queries[strings.Fields(part)[0]] = part
		}
		for _, binding := range Bindings() {
			expected, ok := registry[binding.Section]
			if !ok {
				t.Fatalf("section %s has no discovery policy", binding.Section)
			}
			sql, ok := queries[binding.Query]
			if !ok {
				t.Fatalf("missing query %s", binding.Query)
			}
			actual := []string{}
			for _, match := range parameter.FindAllStringSubmatch(sql, -1) {
				actual = append(actual, match[1])
			}
			slices.Sort(actual)
			actual = slices.Compact(actual)
			expected = slices.Clone(expected)
			slices.Sort(expected)
			if diff := cmp.Diff(expected, actual); diff != "" {
				t.Fatalf("%s/%s selector drift: %s", kind, binding.Section, diff)
			}
		}
	}
}

func TestIncrementalDiscoveryNeverRequeuesOldIdentities(t *testing.T) {
	visited := projectsnapshot.Selection{}
	current := projectsnapshot.Selection{"field": {"a"}}
	first := nextSelection(current, visited)
	current["field"][0] = "a" // The caller still owns its current selection.
	current["field"] = append(current["field"], "b")
	current["override"] = []string{"10"}
	second := nextSelection(current, visited)
	if diff := cmp.Diff(projectsnapshot.Selection{"field": {"a"}}, first); diff != "" {
		t.Fatal(diff)
	}
	if diff := cmp.Diff(projectsnapshot.Selection{"field": {"b"}, "override": {"10"}}, second); diff != "" {
		t.Fatal(diff)
	}
	if len(nextSelection(current, visited)) != 0 {
		t.Fatal("stable selection requeued")
	}
	if _, err := discoverSection("unregistered", current); err == nil {
		t.Fatal("unknown section silently skipped")
	}
	if err := checkDiscovered(projectsnapshot.Content{Fields: []projectsnapshot.FieldRecord{{ID: "unseen"}}}, current); err == nil {
		t.Fatal("new materialized identity escaped discovery")
	}
}

func TestLoadedChildIdentityDoesNotRepeatDiscovery(t *testing.T) {
	visited := projectsnapshot.Selection{}
	content := projectsnapshot.Content{VocabularyEntries: []projectsnapshot.VocabularyEntryRecord{{ID: "child", VocabularyID: "parent"}}}
	if err := discoveredChildren(content, visited); err != nil {
		t.Fatal(err)
	}
	requests, err := projectsnapshot.RequiredEntities(content)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(requests["vocabulary_entry"], "child") {
		t.Fatal("child omitted from final materialization")
	}
	delta := nextSelection(requests, visited)
	if len(delta["vocabulary_entry"]) != 0 || !slices.Contains(delta["vocabulary"], "parent") {
		t.Fatal("child repeated or parent not discovered")
	}
}
