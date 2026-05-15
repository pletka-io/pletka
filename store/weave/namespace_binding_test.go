package weave

import (
	"context"
	"testing"

	domainweave "github.com/pletka-io/pletka/domain/weave"
	"github.com/pletka-io/pletka/store/sqlcgen"
)

func TestNamespaceBindingStoreNilGuard(t *testing.T) {
	var store *namespaceBindingStore
	ctx := context.Background()

	if _, err := store.List(ctx, "project"); err == nil {
		t.Fatalf("nil List() error = nil; want error")
	}
	if _, err := store.GetUser(ctx, "project", "id"); err == nil {
		t.Fatalf("nil GetUser() error = nil; want error")
	}
	if err := store.CreateUser(ctx, &domainweave.NamespaceBinding{ID: "id"}); err == nil {
		t.Fatalf("nil CreateUser() error = nil; want error")
	}
	if err := store.UpdateUser(ctx, &domainweave.NamespaceBinding{ID: "id"}); err == nil {
		t.Fatalf("nil UpdateUser() error = nil; want error")
	}
	if err := store.DeleteUser(ctx, "project", "id"); err == nil {
		t.Fatalf("nil DeleteUser() error = nil; want error")
	}
	if _, err := store.ExistsByPrefixAndNamespace(ctx, "crm", "http://example.test/", ""); err == nil {
		t.Fatalf("nil ExistsByPrefixAndNamespace() error = nil; want error")
	}
}

func TestNamespaceBindingFromRow(t *testing.T) {
	projectID := "TPC"
	binding := namespaceBindingFromRow(sqlcgen.WeaveNamespaceBinding{
		ID:        "binding-1",
		ProjectID: &projectID,
		Prefix:    "crm",
		Namespace: "http://www.cidoc-crm.org/cidoc-crm/",
		Weight:    10,
		Source:    "user",
	})

	if binding.ID != "binding-1" || binding.ProjectID != "TPC" || binding.Prefix != "crm" || binding.Namespace == "" || binding.Weight != 10 || binding.Source != "user" {
		t.Fatalf("namespaceBindingFromRow() = %#v", binding)
	}
}

func TestNamespaceProjectMatches(t *testing.T) {
	projectID := "TPC"
	empty := ""

	if !namespaceProjectMatches(nil, "") {
		t.Fatalf("nil row project should match global project")
	}
	if !namespaceProjectMatches(&empty, "") {
		t.Fatalf("empty row project should match global project")
	}
	if namespaceProjectMatches(&projectID, "") {
		t.Fatalf("project row should not match global project")
	}
	if !namespaceProjectMatches(&projectID, "TPC") {
		t.Fatalf("matching project row did not match")
	}
	if namespaceProjectMatches(nil, "TPC") {
		t.Fatalf("global row should not match project")
	}
}
