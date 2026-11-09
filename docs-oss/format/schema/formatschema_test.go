package formatschema

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSchemasCompile(t *testing.T) {
	if _, err := New(); err != nil {
		t.Fatalf("embedded schemas do not compile: %v", err)
	}
}

func TestKindForPath(t *testing.T) {
	cases := map[string]string{
		"project.yaml":                           KindProject,
		"pletka.mod":                             KindMod,
		"ontology.yaml":                          KindOntology,
		"releases/index.yaml":                    KindReleasesIndex,
		"adoptions/index.yaml":                   KindReceiptsIndex,
		"forks/index.yaml":                       KindReceiptsIndex,
		"adoptions/field-PF.7.yaml":              KindAdoptionReceipt,
		"forks/model-PM.1.yaml":                  KindForkReceipt,
		"categories/X.CAT.1.yaml":                KindCategory,
		"concept-lists/X.CL.1.yaml":              KindConceptList,
		"fields/XF.1/field.yaml":                 KindField,
		"fields/XF.1/base-override.yaml":         KindOverride,
		"models/XM.1/model.yaml":                 KindModel,
		"collections/XC.1/collection.yaml":       KindCollection,
		"models/XM.1/overrides/XF.1@a.yaml":      KindOverride,
		"collections/XC.1/overrides/XF.1@b.yaml": KindOverride,
	}
	for rel, want := range cases {
		got, ok := KindForPath(rel)
		if !ok || got != want {
			t.Errorf("KindForPath(%q) = %q, %v; want %q", rel, got, ok, want)
		}
	}
	for _, rel := range []string{"README.md", "fields/XF.1/notes.yaml", "models/XM.1/overrides/x/y.yaml", "pletka.sum"} {
		if k, ok := KindForPath(rel); ok {
			t.Errorf("KindForPath(%q) = %q; want no kind", rel, k)
		}
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name, kind, yaml string
		want             []Violation // Message is not compared
	}{
		{"valid field, compact path", KindField,
			"semantic_id: XF.1\nontology_scope: crm:E21_Person\npath_elements: [crm:P1_is_identified_by, crm:E41_Appellation]\nontology_path: ->crm:P1\n", nil},
		{"unknown top-level key", KindField,
			"semantic_id: XF.1\ncolor: red\n",
			[]Violation{{Pointer: "/color", Keyword: "additionalProperties"}}},
		{"unknown nested key in a ref", KindOverride,
			"field_id: XF.1\nrefs:\n  - {ref_type: concept_list, semantic_id: X.CL.1, color: red}\n",
			[]Violation{{Pointer: "/refs/0/color", Keyword: "additionalProperties"}}},
		{"semantic id numbered 0", KindModel,
			"semantic_id: XM.0\n",
			[]Violation{{Pointer: "/semantic_id", Keyword: "pattern"}}},
		{"wrong type", KindOverride,
			"field_id: XF.1\nmax_occurs: many\n",
			[]Violation{{Pointer: "/max_occurs", Keyword: "type"}}},
		{"unknown schema_version", KindProject,
			"schema_version: 3\nproject: {id: X}\n",
			[]Violation{{Pointer: "/schema_version", Keyword: "enum"}}},
		{"v1 keys still valid", KindMod,
			"schema_version: 1\nmodule: {path: pletka.io/orgs/example/projects/X, project_id: X}\nrequire:\n  projects: [{module: pletka.io/orgs/example/projects/P, project_id: P, mode: parent, version: draft}]\n  ontologies: [{module: ontology.pletka.io/crm, ontology_id: A, ontology_version_id: B, version: 7.1.3}]\nreplace: []\n", nil},
		{"empty file", KindCategory, "",
			[]Violation{{Pointer: "", Keyword: "type"}}},
	}
	v := Default()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := v.Validate(tc.kind, []byte(tc.yaml))
			if err != nil {
				t.Fatal(err)
			}
			for i := range got {
				got[i].Message = ""
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("violations (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValidateSyntaxError(t *testing.T) {
	if _, err := Default().Validate(KindField, []byte("semantic_id: [unclosed\n")); err == nil {
		t.Fatal("want a YAML error")
	}
}

func TestLine(t *testing.T) {
	payload := []byte("field_id: XF.1\nrefs:\n  - ref_type: concept_list\n    semantic_id: X.CL.1\n    color: red\n")
	for pointer, want := range map[string]int{"": 1, "/field_id": 1, "/refs/0/color": 5, "/refs/0": 3, "/missing": 1} {
		if got := Line(payload, pointer); got != want {
			t.Errorf("Line(%q) = %d; want %d", pointer, got, want)
		}
	}
}
