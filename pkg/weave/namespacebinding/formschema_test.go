package namespacebinding

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func TestBuildGlobalListSchema(t *testing.T) {
	schema := BuildGlobalListSchema("en", nil)
	if schema == nil {
		t.Fatal("nil schema")
	}
	if schema.DataURL != "/admin/namespaces" {
		t.Fatalf("DataURL=%q want /admin/namespaces", schema.DataURL)
	}
	if schema.Caps.Create == nil || schema.Caps.Edit == nil || schema.Caps.Delete == nil || schema.Caps.Stats == nil {
		t.Fatalf("expected create/edit/delete/stats caps, got %+v", schema.Caps)
	}
	if schema.Caps.Create.FormSchemaURL != "/admin/namespaces/form-schema" {
		t.Fatalf("create form schema url=%q", schema.Caps.Create.FormSchemaURL)
	}
	if schema.RowActions[0].VisibleWhen != "_mutable" {
		t.Fatalf("edit visible_when=%q want _mutable", schema.RowActions[0].VisibleWhen)
	}
}

func TestBuildGlobalEntityListSchema(t *testing.T) {
	schema := BuildGlobalEntityListSchema("en", nil)
	if schema == nil {
		t.Fatal("nil schema")
	}
	if schema.DataURL != "/admin/namespaces/data" {
		t.Fatalf("DataURL=%q want /admin/namespaces/data", schema.DataURL)
	}
	if schema.Search == nil || schema.Search.ParamName != "search" {
		t.Fatalf("unexpected search config: %+v", schema.Search)
	}
	if schema.Pagination == nil || schema.Pagination.PageSize != 25 {
		t.Fatalf("unexpected pagination: %+v", schema.Pagination)
	}
	if schema.Capabilities == nil || schema.Capabilities.Stats == nil {
		t.Fatalf("expected capabilities with stats, got %+v", schema.Capabilities)
	}
	if len(schema.RowActions) != 3 {
		t.Fatalf("row action count=%d want 3", len(schema.RowActions))
	}
}

func TestBuildGlobalEditForm(t *testing.T) {
	form := BuildGlobalEditForm(&domain.NamespaceBinding{
		ID:        "01TESTGLOBALNAMESPACE",
		Prefix:    "crm",
		Namespace: "http://www.cidoc-crm.org/cidoc-crm/",
		Weight:    5,
		Source:    "user",
	}, "en", nil)
	if form == nil {
		t.Fatal("nil form")
	}
	if form.Endpoint == nil || form.Endpoint.URL != "/admin/namespaces/01TESTGLOBALNAMESPACE" {
		t.Fatalf("endpoint=%+v", form.Endpoint)
	}
	if len(form.Sections) != 1 || len(form.Sections[0].Fields) != 3 {
		t.Fatalf("unexpected field count: %+v", form.Sections)
	}
}
