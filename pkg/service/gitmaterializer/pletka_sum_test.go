package gitmaterializer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEncodePletkaSum_Deterministic(t *testing.T) {
	entries := []pletkaSumEntry{
		{
			Module:  "ontology.pletka.io/linked-art",
			Version: "0.9",
			TreeSHA: "bbbb",
		},
		{
			Module:  "pletka.io/orgs/takin-solutions/projects/LA",
			Version: "draft",
			TreeSHA: "aaaa",
		},
	}

	got := string(encodePletkaSum(entries))
	gotAgain := string(encodePletkaSum(entries))
	if got != gotAgain {
		t.Fatal("encodePletkaSum is not deterministic")
	}

	want := "" +
		"ontology.pletka.io/linked-art 0.9 tree:sha256:bbbb\n" +
		"pletka.io/orgs/takin-solutions/projects/LA draft tree:sha256:aaaa\n"
	if got != want {
		t.Fatalf("unexpected pletka.sum:\n%s", got)
	}
}

func TestHashDirectoryTree_Deterministic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a", "one.txt"), []byte("alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := hashDirectoryTree(dir)
	if err != nil {
		t.Fatalf("hashDirectoryTree: %v", err)
	}
	second, err := hashDirectoryTree(dir)
	if err != nil {
		t.Fatalf("hashDirectoryTree second pass: %v", err)
	}
	if first != second {
		t.Fatalf("expected stable tree hash, got %s vs %s", first, second)
	}
}
