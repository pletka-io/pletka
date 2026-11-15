package exportgraph_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/generators/exportgraph"
	"github.com/pletka-io/pletka/pkg/weave/views"
)

// The rendered document carries the step counts and the datatype literal,
// and decodes back into a Graph with the same content.
func TestRendererDocumentRoundTrips(t *testing.T) {
	f := tfield("F.1", 1, true, intp(3), tprop("crm:P1_is_identified_by"), tclass("crm:E41_Appellation", "n1"), tprop("crm:P190_has_symbolic_content"), tlit("xsd:string"))
	snap := &generators.Snapshot{
		Fields: []generators.FieldNode{f},
		Tree:   &views.Tree{Root: views.TreeNode{Scope: ptr(qel("class", "crm:E21_Person"))}},
	}
	var buf bytes.Buffer
	if err := exportgraph.NewRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatal(err)
	}
	doc := buf.String()
	for _, key := range []string{`"min"`, `"max"`, `"end"`, `"identity_source"`} {
		if !strings.Contains(doc, key) {
			t.Errorf("document lacks %s:\n%s", key, doc)
		}
	}
	var got exportgraph.Graph
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := exportgraph.Build(snap)
	if diff := cmp.Diff(want.RootKey, got.RootKey); diff != "" {
		t.Fatal(diff)
	}
	if len(got.Nodes) != len(want.Nodes) || len(got.Nodes) < 2 {
		t.Fatalf("nodes: got %d, want %d", len(got.Nodes), len(want.Nodes))
	}
	var again bytes.Buffer
	enc := json.NewEncoder(&again)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&got); err != nil {
		t.Fatal(err)
	}
	if again.String() != doc {
		t.Fatalf("re-encoded document differs:\n%s", again.String())
	}
}
