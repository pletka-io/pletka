package gitmaterializer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteOverrideFilesNumbersDuplicatesByPositionThenBytes(t *testing.T) {
	dir := t.TempDir()
	p := "models/XM.1/overrides/XF.1@X.CAT.1.yaml"
	// rows arrive in an arbitrary (row id) order
	files := []pendingOverride{
		{path: p, position: 7, payload: []byte("b\n")},
		{path: p, position: 3, payload: []byte("c\n")},
		{path: p, position: 7, payload: []byte("a\n")},
		{path: "models/XM.1/overrides/XF.2@_.yaml", position: 1, payload: []byte("x\n")},
	}
	if err := writeOverrideFiles(dir, files); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"models/XM.1/overrides/XF.1@X.CAT.1.yaml":   "c\n",
		"models/XM.1/overrides/XF.1@X.CAT.1~2.yaml": "a\n",
		"models/XM.1/overrides/XF.1@X.CAT.1~3.yaml": "b\n",
		"models/XM.1/overrides/XF.2@_.yaml":         "x\n",
	}
	for rel, body := range want {
		got, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		if string(got) != body {
			t.Fatalf("%s = %q, want %q", rel, got, body)
		}
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "models/XM.1/overrides"))
	if len(entries) != len(want) {
		t.Fatalf("%d files, want %d", len(entries), len(want))
	}
}
