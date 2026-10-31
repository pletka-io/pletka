//go:build integration

package release

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/projectbundle"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// Called with real draft/verified/legacy graphs after read sessions close. Compare
// every row and interpretation value, independently of re-encoded file equality.
func checkCompleteBundle(t *testing.T, original projectsnapshot.CompleteProject) {
	t.Helper()
	files, err := projectbundle.Encode(original)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := projectbundle.Decode(files, projectbundle.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	restored := imported.Snapshot()
	beforeID, _ := original.Root()
	afterID, _ := restored.Root()
	if beforeID != afterID {
		t.Fatal("root identity changed")
	}
	beforeIDs, _ := original.Projects()
	afterIDs, _ := restored.Projects()
	if !slices.Equal(beforeIDs, afterIDs) {
		t.Fatal("source selections changed")
	}
	for _, id := range beforeIDs {
		before, _ := original.Input(id)
		after, err := restored.Input(id)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(before.Dependencies, after.Dependencies) || !slices.Equal(before.Membership, after.Membership) {
			t.Fatal("source provenance changed")
		}
		if diff := cmp.Diff(sortedContent(t, before.Content), sortedContent(t, after.Content)); diff != "" {
			t.Fatal(diff)
		}
		if diff := cmp.Diff(before.Evidence.Descriptors().Document(), after.Evidence.Descriptors().Document()); diff != "" {
			t.Fatal(diff)
		}
		if before.Evidence.Contract() != after.Evidence.Contract() {
			t.Fatal("capture contract changed")
		}
		if !id.IsDraft() {
			a, _ := before.Evidence.CaptureSections()
			b, _ := after.Evidence.CaptureSections()
			if diff := cmp.Diff(a, b); diff != "" {
				t.Fatal(diff)
			}
		}
		if before.Evidence.Contract() == 2 {
			a, _ := before.Evidence.Legacy()
			b, _ := after.Evidence.Legacy()
			if a != b {
				t.Fatal("exact legacy bytes changed")
			}
		} else {
			a, _ := before.Evidence.Publications()
			b, _ := after.Evidence.Publications()
			if diff := cmp.Diff(sortedJSON(t, a), sortedJSON(t, b)); diff != "" {
				t.Fatal(diff)
			}
			ar, _ := before.Evidence.References()
			br, _ := after.Evidence.References()
			if diff := cmp.Diff(sortedJSON(t, ar), sortedJSON(t, br)); diff != "" {
				t.Fatal(diff)
			}
		}
	}
	reencoded, err := projectbundle.Encode(restored)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(files, reencoded); diff != "" {
		t.Fatal(diff)
	}
}
func sortedContent(t *testing.T, c projectsnapshot.Content) map[string][]string {
	t.Helper()
	v := reflect.ValueOf(c)
	out := map[string][]string{}
	for i := range v.NumField() {
		out[v.Type().Field(i).Tag.Get("json")] = sortedJSON(t, v.Field(i).Interface())
	}
	return out
}
func sortedJSON(t *testing.T, list any) []string {
	t.Helper()
	v := reflect.ValueOf(list)
	out := make([]string, 0, v.Len())
	for i := range v.Len() {
		b, err := json.Marshal(v.Index(i).Interface())
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(b))
	}
	slices.SortFunc(out, strings.Compare)
	return out
}
