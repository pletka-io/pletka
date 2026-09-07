package ontology

import "testing"

// TestMergeSchemaQuery guards the F3b fix: the ontology page shell must forward
// the browser's query string onto the page-schema fetch so viewer tabs
// (?tab=extensions, ?tab=classes, ...) actually change what is rendered.
func TestMergeSchemaQuery(t *testing.T) {
	tests := []struct {
		name      string
		schemaURL string
		rawQuery  string
		want      string
	}{
		{
			name:      "no query keeps url",
			schemaURL: "/ontologies/families/cidoc-crm-family/page-schema",
			rawQuery:  "",
			want:      "/ontologies/families/cidoc-crm-family/page-schema",
		},
		{
			name:      "forwards tab (the family extensions bug)",
			schemaURL: "/ontologies/families/cidoc-crm-family/page-schema",
			rawQuery:  "tab=extensions",
			want:      "/ontologies/families/cidoc-crm-family/page-schema?tab=extensions",
		},
		{
			name:      "forwards version classes tab",
			schemaURL: "/ontologies/crm/7.1.3/page-schema",
			rawQuery:  "tab=classes",
			want:      "/ontologies/crm/7.1.3/page-schema?tab=classes",
		},
		{
			name:      "caller-pinned query is left untouched",
			schemaURL: "/ontologies/crm/7.1.3/page-schema?tab=properties",
			rawQuery:  "tab=classes",
			want:      "/ontologies/crm/7.1.3/page-schema?tab=properties",
		},
	}
	for _, tc := range tests {
		if got := mergeSchemaQuery(tc.schemaURL, tc.rawQuery); got != tc.want {
			t.Errorf("%s: mergeSchemaQuery(%q, %q) = %q; want %q", tc.name, tc.schemaURL, tc.rawQuery, got, tc.want)
		}
	}
}
