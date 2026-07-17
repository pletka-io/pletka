package mergedfs

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestMergeFirstFilesystemWins(t *testing.T) {
	merged := Merge(
		fstest.MapFS{"app.css": {Data: []byte("override")}},
		fstest.MapFS{"app.css": {Data: []byte("base")}},
	)

	got, err := fs.ReadFile(merged, "app.css")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "override" {
		t.Fatalf("ReadFile() = %q, want override", got)
	}
}

func TestMergeReadDirCombinesEntries(t *testing.T) {
	merged := Merge(
		fstest.MapFS{
			"css/app.css":     {Data: []byte("override")},
			"css/custom.css":  {Data: []byte("custom")},
			"images/logo.svg": {Data: []byte("<svg/>")},
		},
		fstest.MapFS{
			"css/app.css":  {Data: []byte("base")},
			"css/base.css": {Data: []byte("base")},
		},
	)

	entries, err := fs.ReadDir(merged, "css")
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	want := []string{"app.css", "base.css", "custom.css"}
	if len(got) != len(want) {
		t.Fatalf("ReadDir() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ReadDir() = %#v, want %#v", got, want)
		}
	}
}

func TestMergeMissingFile(t *testing.T) {
	_, err := Merge(fstest.MapFS{}).Open("missing.txt")
	if err == nil {
		t.Fatal("Open() returned nil error")
	}
}
