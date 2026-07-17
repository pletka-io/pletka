package threem_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/integrations/registry"
	"github.com/pletka-io/pletka/pkg/integrations/threem"
)

func TestIntegration_Identity(t *testing.T) {
	i := threem.New()
	if i.ID() != "threem" {
		t.Fatalf("id: %q", i.ID())
	}
	if got := i.SecretFields(); len(got) != 1 || got[0] != "password" {
		t.Fatalf("secret fields: %v", got)
	}
	got := i.AppliesTo()
	want := map[string]bool{"x3ml": true, "x3ml-b": true}
	for _, f := range got {
		if !want[f] {
			t.Fatalf("unexpected format %q", f)
		}
		delete(want, f)
	}
	if len(want) != 0 {
		t.Fatalf("missing formats: %v", want)
	}
}

func TestValidateConfig_RejectsEmptyBaseURL(t *testing.T) {
	i := threem.New()
	_, errs := i.ValidateConfig(map[string]any{"username": "u"})
	if len(errs["base_url"]) == 0 {
		t.Fatalf("expected base_url error, got %v", errs)
	}
}

func TestValidateConfig_RejectsNonHTTPScheme(t *testing.T) {
	i := threem.New()
	_, errs := i.ValidateConfig(map[string]any{
		"base_url": "ftp://3m.example",
		"username": "u",
	})
	if len(errs["base_url"]) == 0 {
		t.Fatalf("expected base_url scheme error, got %v", errs)
	}
}

func TestValidateConfig_Normalises(t *testing.T) {
	i := threem.New()
	out, errs := i.ValidateConfig(map[string]any{
		"base_url":        "https://3m.example/ ",
		"username":        " alice ",
		"password":        "secret",
		"timeout_seconds": 30,
	})
	if errs != nil {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if out["base_url"] != "https://3m.example" {
		t.Fatalf("base_url not trimmed: %q", out["base_url"])
	}
	if out["username"] != "alice" {
		t.Fatalf("username not trimmed: %q", out["username"])
	}
	if out["password"] != "secret" {
		t.Fatalf("password lost: %q", out["password"])
	}
	if out["timeout_seconds"] != 30 {
		t.Fatalf("timeout: %v", out["timeout_seconds"])
	}
}

func TestRunAction_EndToEndAgainstStub(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "abc", Path: "/"})
			w.Header().Set("Location", "/dashboard")
			w.WriteHeader(http.StatusFound)
		case "/upload":
			_, _ = w.Write([]byte("OK Mapping URI: http://cidoc_mappings.com/Mapping/Mapping7"))
		}
	}))
	t.Cleanup(srv.Close)

	i := threem.New()
	result, err := i.RunAction(context.Background(), "upload", registry.ActionInput{
		ProjectID:  "LA",
		EntityKind: "model",
		EntityID:   "LAM.1",
		Format:     "x3ml-b",
		Config: map[string]any{
			"base_url":        srv.URL,
			"username":        "alice",
			"password":        "pw",
			"timeout_seconds": 5,
			"login_path":      "/login",
			"upload_path":     "/upload",
		},
		Artifact: func(_ registry.Format) (io.Reader, string, error) {
			return strings.NewReader("MOCK_ZIP_BYTES"), "LAM.1.b.x3ml.zip", nil
		},
	})
	if err != nil {
		t.Fatalf("RunAction: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("status: %q", result.Status)
	}
	// LinkURL is the browseable editor URL built from base_url's host +
	// the editor template, not the raw cidoc_mappings URI.
	wantPrefix := srv.URL + "/3MEditor/Index?type=Mapping&id=Mapping7&lang=en"
	if result.LinkURL != wantPrefix {
		t.Fatalf("link: got %q want %q", result.LinkURL, wantPrefix)
	}
}

func TestBuildEditorURL_Defaults(t *testing.T) {
	got := threem.BuildEditorURL("https://3m.example/3M", "http://cidoc_mappings.com/Mapping/Mapping646", "/3MEditor/Index?type=Mapping&id={id}&lang=en")
	want := "https://3m.example/3MEditor/Index?type=Mapping&id=Mapping646&lang=en"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBuildEditorURL_FallsBackOnUnparsableMapping(t *testing.T) {
	// No mapping ID in the URI → fall back to the raw value rather than
	// fabricate an editor URL the user can't open.
	got := threem.BuildEditorURL("https://3m.example/3M", "garbage", "/3MEditor/Index?type=Mapping&id={id}&lang=en")
	if got != "garbage" {
		t.Fatalf("fallback: %q", got)
	}
}

func TestRunAction_BadCredentialsBecomesUserFacingError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/login?error")
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	i := threem.New()
	result, err := i.RunAction(context.Background(), "upload", registry.ActionInput{
		ProjectID:  "LA",
		EntityKind: "model",
		EntityID:   "LAM.1",
		Format:     "x3ml-b",
		Config: map[string]any{
			"base_url":   srv.URL,
			"username":   "alice",
			"password":   "wrong",
			"login_path": "/login",
		},
		Artifact: func(_ registry.Format) (io.Reader, string, error) {
			return strings.NewReader("x"), "x.zip", nil
		},
	})
	if err != nil {
		t.Fatalf("RunAction: %v", err)
	}
	if result.Status != "error" {
		t.Fatalf("status: %q", result.Status)
	}
}

func TestRunAction_UnknownAction(t *testing.T) {
	i := threem.New()
	if _, err := i.RunAction(context.Background(), "delete", registry.ActionInput{}); err == nil {
		t.Fatalf("want error for unknown action, got nil")
	}
}

func TestConfigSchema_RendersFields(t *testing.T) {
	i := threem.New()
	schema := i.ConfigSchema(registry.ConfigRequest{ProjectID: "LA"}, map[string]any{
		"base_url": "https://3m.example",
		"username": "alice",
	})
	if schema == nil {
		t.Fatalf("nil schema")
	}
	if len(schema.Sections) != 1 || len(schema.Sections[0].Fields) != 7 {
		t.Fatalf("unexpected schema shape: %+v", schema.Sections)
	}
	fieldNames := map[string]bool{}
	for _, f := range schema.Sections[0].Fields {
		fieldNames[f.Name] = true
		if f.Name == "password" && f.Value != "" {
			t.Fatalf("password value should always render empty, got %v", f.Value)
		}
	}
	for _, want := range []string{"base_url", "username", "password", "timeout_seconds"} {
		if !fieldNames[want] {
			t.Fatalf("missing field %q in schema", want)
		}
	}
	if schema.Endpoint == nil || schema.Endpoint.Method != "PUT" {
		t.Fatalf("schema endpoint: %+v", schema.Endpoint)
	}
}
