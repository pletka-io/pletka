package ontology

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	parsedontology "github.com/pletka-io/pletka/pkg/weave/ontology/rdf"
)

func decodeFieldErrors(t *testing.T, body []byte) map[string][]string {
	t.Helper()
	var envelope struct {
		Errors map[string][]string `json:"errors"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return envelope.Errors
}

// A prefix clash is the curator's to fix, and they can only fix it if the
// response names the prefix and both URIs. Before this mapping the same
// upload returned a 500 with no detail.
func TestWriteServiceErrorMapsPrefixConflictToFieldError(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/admin/ontologies/x/versions/import", nil)

	err := fmt.Errorf("parse RDF: %w", &parsedontology.PrefixConflictError{
		Prefix:   "cto",
		Declared: "https://nfdi4culture.de/ontology/",
		Existing: "https://nfdi4culture.de/ontology#",
	})
	h.writeServiceError(rec, req, err, "version import")

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	msgs := decodeFieldErrors(t, rec.Body.Bytes())["rdf_file"]
	if len(msgs) != 1 {
		t.Fatalf("rdf_file errors=%v, want exactly one", msgs)
	}
	for _, want := range []string{"cto", "https://nfdi4culture.de/ontology/", "https://nfdi4culture.de/ontology#"} {
		if !strings.Contains(msgs[0], want) {
			t.Errorf("message %q does not name %q", msgs[0], want)
		}
	}
}

// A malformed file is equally the curator's to fix. xml.SyntaxError carries
// the line; a 500 threw it away.
func TestWriteServiceErrorMapsXMLSyntaxErrorToFieldError(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/admin/ontologies/x/versions/import", nil)

	err := fmt.Errorf("parse RDF: failed to parse XML: %w", &xml.SyntaxError{Line: 1, Msg: "expected /> in element"})
	h.writeServiceError(rec, req, err, "version import")

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	msgs := decodeFieldErrors(t, rec.Body.Bytes())["rdf_file"]
	if len(msgs) != 1 {
		t.Fatalf("rdf_file errors=%v, want exactly one", msgs)
	}
	if !strings.Contains(msgs[0], "line 1") || !strings.Contains(msgs[0], "expected /> in element") {
		t.Errorf("message %q does not carry the line and the XML complaint", msgs[0])
	}
}
