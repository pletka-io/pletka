package weave

import (
	"context"
	"testing"
	"time"

	"github.com/pletka-io/pletka/store/sqlcgen"
)

func TestProjectOntologyVersionStoreNilGuard(t *testing.T) {
	ctx := context.Background()
	var store *projectOntologyVersionStore

	if _, err := store.Get(ctx, "TPC", "ov-1"); err == nil {
		t.Fatalf("nil Get() error = nil; want error")
	}
	if _, err := store.GetVersion(ctx, "TPC", "ov-1", "v1"); err == nil {
		t.Fatalf("nil GetVersion() error = nil; want error")
	}
	if _, err := store.List(ctx, "TPC"); err == nil {
		t.Fatalf("nil List() error = nil; want error")
	}
	if _, err := store.ListVersion(ctx, "TPC", "v1"); err == nil {
		t.Fatalf("nil ListVersion() error = nil; want error")
	}
}

func TestProjectOntologyVersionFromRow(t *testing.T) {
	addedAt := time.Date(2026, 5, 16, 15, 0, 0, 0, time.UTC)
	addedBy := "actor-1"
	isPrimary := true
	usageNotes := "Use for Linked Art alignment."

	link := projectOntologyVersionFromRow(sqlcgen.WeaveProjectOntologyVersion{
		ProjectID:         "TPC",
		OntologyVersionID: "la-v1",
		AddedAt:           addedAt,
		AddedByID:         &addedBy,
		IsPrimary:         &isPrimary,
		UsageNotes:        &usageNotes,
		VersionNumber:     "draft",
	})

	if link.ProjectID != "TPC" || link.OntologyVersionID != "la-v1" {
		t.Fatalf("link identity mapping failed: %#v", link)
	}
	if !link.IsPrimary || link.AddedByID == nil || *link.AddedByID != "actor-1" {
		t.Fatalf("link metadata mapping failed: %#v", link)
	}
	if link.UsageNotes == nil || *link.UsageNotes != usageNotes || !link.AddedAt.Equal(addedAt) {
		t.Fatalf("link notes/time mapping failed: %#v", link)
	}
}

func TestBoolFromPtr(t *testing.T) {
	if boolFromPtr(nil) {
		t.Fatalf("nil bool pointer should map to false")
	}
	value := true
	if !boolFromPtr(&value) {
		t.Fatalf("true pointer should map to true")
	}
	value = false
	if boolFromPtr(&value) {
		t.Fatalf("false pointer should map to false")
	}
}
