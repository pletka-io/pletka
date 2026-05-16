package weave

import (
	"context"
	"testing"

	domainweave "github.com/pletka-io/pletka/domain/weave"
)

func TestProjectInheritanceStoreNilGuard(t *testing.T) {
	ctx := context.Background()
	var store *projectInheritanceStore

	if _, err := store.List(ctx, "TPC"); err == nil {
		t.Fatalf("nil List() error = nil; want error")
	}
	if _, err := store.ListVersion(ctx, "TPC", "v1"); err == nil {
		t.Fatalf("nil ListVersion() error = nil; want error")
	}
	if _, err := store.ListByParent(ctx, "LA"); err == nil {
		t.Fatalf("nil ListByParent() error = nil; want error")
	}
	if err := store.Add(ctx, domainweave.ProjectInheritance{}); err == nil {
		t.Fatalf("nil Add() error = nil; want error")
	}
	if err := store.Remove(ctx, "TPC", "LA"); err == nil {
		t.Fatalf("nil Remove() error = nil; want error")
	}
	if err := store.SetPrimary(ctx, "TPC", "LA"); err == nil {
		t.Fatalf("nil SetPrimary() error = nil; want error")
	}
	if err := store.Reorder(ctx, "TPC", []string{"LA"}); err == nil {
		t.Fatalf("nil Reorder() error = nil; want error")
	}
}

func TestProjectInheritanceStoreEmptyReorderSkipsPool(t *testing.T) {
	var store *projectInheritanceStore
	if err := store.Reorder(context.Background(), "TPC", nil); err != nil {
		t.Fatalf("empty Reorder() error = %v; want nil", err)
	}
}

func TestNormalizeInheritanceSourceMode(t *testing.T) {
	if got := normalizeInheritanceSourceMode(domainweave.DependencySourceRelease); got != domainweave.DependencySourceRelease {
		t.Fatalf("release normalized to %q", got)
	}
	if got := normalizeInheritanceSourceMode(""); got != domainweave.DependencySourceDraft {
		t.Fatalf("empty normalized to %q", got)
	}
	if got := normalizeInheritanceSourceMode("unknown"); got != domainweave.DependencySourceDraft {
		t.Fatalf("unknown normalized to %q", got)
	}
}

func TestNullableTrimmedString(t *testing.T) {
	if nullableTrimmedString("") != nil {
		t.Fatalf("empty string should map to nil")
	}
	if nullableTrimmedString("   ") != nil {
		t.Fatalf("blank string should map to nil")
	}
	got := nullableTrimmedString(" v1 ")
	if got == nil || *got != "v1" {
		t.Fatalf("nullableTrimmedString() = %#v", got)
	}
}
