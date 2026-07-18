package apikey

import (
	"strings"
	"testing"
)

func TestBuildListSchema(t *testing.T) {
	s := BuildListSchema("en", nil)
	if s.DataURL != "/me/api-keys" {
		t.Fatalf("data_url = %q", s.DataURL)
	}
	if s.Caps.Create == nil || s.Caps.Create.FormSchemaURL != "/me/api-keys/form-schema" {
		t.Fatalf("create cap missing/wrong: %+v", s.Caps.Create)
	}
	if s.Caps.Delete != nil {
		t.Fatal("no delete cap — revoke only")
	}
	var revoke bool
	for _, a := range s.RowActions {
		if a.ID == "revoke" {
			revoke = true
			if a.Method != "POST" || a.URLTemplate != "/me/api-keys/{id}/revoke" {
				t.Fatalf("revoke action wrong: %+v", a)
			}
			if a.VisibleWhen != "!revoked_at" {
				t.Fatalf("revoke visible_when = %q", a.VisibleWhen)
			}
			if a.Style != "danger" {
				t.Fatalf("revoke style = %q", a.Style)
			}
		}
	}
	if !revoke {
		t.Fatal("revoke row action missing")
	}
}

func TestBuildCreateFormSchema(t *testing.T) {
	s := BuildCreateFormSchema("en", nil)
	if s.UI.RevealField != "secret" {
		t.Fatalf("reveal_field = %q", s.UI.RevealField)
	}
	if s.Endpoint == nil || s.Endpoint.URL != "/me/api-keys" || !strings.EqualFold(s.Endpoint.Method, "POST") {
		t.Fatalf("endpoint wrong: %+v", s.Endpoint)
	}
	var hasName bool
	for _, sec := range s.Sections {
		for _, f := range sec.Fields {
			if f.Name == "name" {
				hasName = true
			}
		}
	}
	if !hasName {
		t.Fatal("name field missing")
	}
}
