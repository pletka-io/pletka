package conformance

import (
	"os"
	"path/filepath"
	"testing"
)

const fixtureService = `package demo

import (
	"context"

	"example.com/pkg/auth"
)

type Service struct{}

func (s *Service) GetThing(ctx context.Context, scope auth.ReadScope, id string) error { return nil }
func (s *Service) ListThings(ctx context.Context, projectID string) error              { return nil }
func (s *Service) CreateThing(ctx context.Context, id string) error                    { return nil }
func (s *Service) unexportedList(ctx context.Context) error                            { return nil }
func (s *Service) BatchUsageCounts(ctx context.Context, ids []string) error            { return nil }
func Helper(ctx context.Context) error                                                 { return nil }
`

func writeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	pkgDir := filepath.Join(root, "pkg", "weave", "demo")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "service.go"), []byte(fixtureService), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestScanSliceReadsFindsReadShapedMethods(t *testing.T) {
	got, err := scanSliceReads(writeFixture(t), VersionedSlice{Name: "demo", Pkg: "pkg/weave/demo"})
	if err != nil {
		t.Fatalf("scanSliceReads: %v", err)
	}

	byName := map[string]ReadMethod{}
	for _, m := range got {
		byName[m.Name] = m
	}

	// Read-shaped and scoped.
	if m, ok := byName["GetThing"]; !ok || !m.HasScope {
		t.Errorf("GetThing should be found with a scope, got %+v (present=%v)", m, ok)
	}
	// Read-shaped and unscoped -- the case the ratchet exists for.
	if m, ok := byName["ListThings"]; !ok || m.HasScope {
		t.Errorf("ListThings should be found without a scope, got %+v (present=%v)", m, ok)
	}
	// Batch* is a read prefix.
	if _, ok := byName["BatchUsageCounts"]; !ok {
		t.Error("BatchUsageCounts should be found: Batch is a read prefix")
	}
	// Mutations are hot by design and must not be demanded to take a scope.
	if _, ok := byName["CreateThing"]; ok {
		t.Error("CreateThing is a mutation and must not be scanned as a read")
	}
	// Unexported methods are not a public surface.
	if _, ok := byName["unexportedList"]; ok {
		t.Error("unexportedList is not exported and must not be scanned")
	}
	// Package-level functions are not Service methods.
	if _, ok := byName["Helper"]; ok {
		t.Error("Helper is not a *Service method and must not be scanned")
	}
}

func TestScanSliceReadsRecordsLineNumbers(t *testing.T) {
	got, err := scanSliceReads(writeFixture(t), VersionedSlice{Name: "demo", Pkg: "pkg/weave/demo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("no methods scanned; the line-number assertion would be vacuous")
	}
	for _, m := range got {
		if m.Line <= 0 {
			t.Errorf("%s has no line number; failures must be navigable", m.Name)
		}
	}
}

func TestReadMethodKeyIsSliceDotMethod(t *testing.T) {
	m := ReadMethod{Slice: "category", Name: "List"}
	if got := m.Key(); got != "category.List" {
		t.Errorf("Key() = %q, want category.List -- the allowlist is keyed on this", got)
	}
}
