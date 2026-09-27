package settings

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDecodeAddVocabularyBodyAcceptsWhatTheWidgetSends closes the gap that
// put a 422 on alpha.
//
// The schema test asserts the field carries the right item_add_url, and
// svelte-check asserts the widget's types — but nothing compared the body
// one side sends against the keys the other decodes. The widget posted
// {"value":"aat"} to a handler reading only {"mount":...}, so every add
// failed validation while both suites stayed green.
//
// The literals below are the generic self-managing-field contract spelled
// out rather than referenced: the point is that this endpoint accepts
// exactly what a widget that knows nothing about vocabularies sends.
func TestDecodeAddVocabularyBodyAcceptsWhatTheWidgetSends(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantMount string
		wantLang  string
		wantErrOn string // field key expected in the validation errors, "" for none
	}{
		{
			name:      "the widget's generic value key",
			body:      `{"value":"aat"}`,
			wantMount: "aat",
		},
		{
			name:      "the endpoint's own mount key still works",
			body:      `{"mount":"aat"}`,
			wantMount: "aat",
		},
		{
			name:      "value wins when both are sent",
			body:      `{"value":"fish-monument-type","mount":"aat"}`,
			wantMount: "fish-monument-type",
		},
		{
			name:      "lang rides along",
			body:      `{"value":"aat","lang":"nl"}`,
			wantMount: "aat",
			wantLang:  "nl",
		},
		{
			name:      "whitespace is not a vocabulary",
			body:      `{"value":"   "}`,
			wantErrOn: "value",
		},
		{
			name:      "neither key is a validation error, not a silent success",
			body:      `{}`,
			wantErrOn: "value",
		},
		{
			name:      "a malformed body is reported as one",
			body:      `{`,
			wantErrOn: "body",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(),
				http.MethodPost, "/projects/P1/settings/vocabularies", strings.NewReader(tc.body))

			mount, lang, fieldErrs := decodeAddVocabularyBody(req)

			if tc.wantErrOn != "" {
				if fieldErrs == nil {
					t.Fatalf("got mount %q with no error, want a validation error on %q", mount, tc.wantErrOn)
				}
				if _, ok := fieldErrs[tc.wantErrOn]; !ok {
					t.Errorf("validation errors = %v, want a key %q", fieldErrs, tc.wantErrOn)
				}
				return
			}

			if fieldErrs != nil {
				t.Fatalf("validation errors = %v, want none", fieldErrs)
			}
			if mount != tc.wantMount {
				t.Errorf("mount = %q, want %q", mount, tc.wantMount)
			}
			if lang != tc.wantLang {
				t.Errorf("lang = %q, want %q", lang, tc.wantLang)
			}
		})
	}
}
