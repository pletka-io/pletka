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
		"project.yaml":                             KindProject,
		"pletka.mod":                               KindMod,
		"ontology.yaml":                            KindOntology,
		"releases/index.yaml":                      KindReleasesIndex,
		"adoptions/index.yaml":                     KindReceiptsIndex,
		"forks/index.yaml":                         KindReceiptsIndex,
		"adoptions/field-PF.7.yaml":                KindAdoptionReceipt,
		"forks/model-PM.1.yaml":                    KindForkReceipt,
		"categories/X.CAT.1.yaml":                  KindCategory,
		"concept-lists/X.CL.1.yaml":                KindConceptList,
		"vocabularies/SI.VOC.1/vocabulary.yaml":    KindVocabulary,
		"vocabularies/SI.VOC.1/concepts.yaml":      KindVocabularyConcepts,
		"concept-lists/SI.CL.4/entries.yaml":       KindConceptListEntries,
		"fields/XF.1/field.yaml":                   KindField,
		"fields/XF.1/base-override.yaml":           KindOverride,
		"models/XM.1/model.yaml":                   KindModel,
		"collections/XC.1/collection.yaml":         KindCollection,
		"models/XM.1/overrides/XF.1@a.yaml":        KindOverride,
		"collections/XC.1/overrides/XF.1@b.yaml":   KindOverride,
		"models/XM.1/placements/XC.1@X.CAT.1.yaml": KindPlacement,
		"models/XM.1/placements/XC.1@_.yaml":       KindPlacement,
	}
	for rel, want := range cases {
		got, ok := KindForPath(rel)
		if !ok || got != want {
			t.Errorf("KindForPath(%q) = %q, %v; want %q", rel, got, ok, want)
		}
	}
	for _, rel := range []string{"README.md", "fields/XF.1/notes.yaml", "models/XM.1/overrides/x/y.yaml", "collections/XC.1/placements/XC.2@_.yaml", "pletka.sum", "vocabularies/SI.VOC.1.yaml", "vocabularies/SI.VOC.1/other.yaml"} {
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
		{"semantic id numbered 0", KindModel, "semantic_id: XM.0\n", nil},
		{"semantic id with a leading zero", KindModel,
			"semantic_id: XM.01\n",
			[]Violation{{Pointer: "/semantic_id", Keyword: "pattern"}}},
		{"wrong type", KindOverride,
			"field_id: XF.1\nmax_occurs: many\n",
			[]Violation{{Pointer: "/max_occurs", Keyword: "type"}}},
		{"unknown schema_version", KindProject,
			"schema_version: 3\nproject: {id: X}\n",
			[]Violation{{Pointer: "/schema_version", Keyword: "enum"}}},
		{"v1 keys still valid", KindMod,
			"schema_version: 1\nmodule: {path: pletka.io/orgs/example/projects/X, project_id: X}\nrequire:\n  projects: [{module: pletka.io/orgs/example/projects/P, project_id: P, mode: parent, version: draft}]\n  ontologies: [{module: ontology.pletka.io/crm, ontology_id: A, ontology_version_id: B, version: 7.1.3}]\nreplace: []\n", nil},
		{"malformed path element: the real cause, not the other oneOf branch", KindField,
			"semantic_id: XF.1\npath_elements: [{type: class, urii: x}]\n",
			[]Violation{{Pointer: "/path_elements/0", Keyword: "required"}, {Pointer: "/path_elements/0/urii", Keyword: "additionalProperties"}}},
		{"valid placement", KindPlacement,
			"category_id: \"\"\ncollection_id: XC.1\nis_hidden: false\nis_required: false\nmax_occurs: null\nmin_occurs: 0\nmodel_id: XM.1\n", nil},
		{"placement without model_id, with an unknown key", KindPlacement,
			"collection_id: XC.1\ncolor: red\n",
			[]Violation{{Pointer: "", Keyword: "required"}, {Pointer: "/color", Keyword: "additionalProperties"}}},
		{"vocabulary ok", KindVocabulary, "semantic_id: SI.VOC.1\nsystem_name: arches\nbase_uri: http://localhost:8000/\n", nil},
		{"vocabulary unknown key", KindVocabulary, "semantic_id: SI.VOC.1\nfoo: 1\n",
			[]Violation{{Pointer: "/foo", Keyword: "additionalProperties"}}},
		{"concepts ok", KindVocabularyConcepts, "- uri: http://x/1\n  label: {en: A}\n  broader: [http://x/2, http://x/3]\n  top_concept: false\n", nil},
		{"concepts need uri", KindVocabularyConcepts, "- label: {en: A}\n",
			[]Violation{{Pointer: "/0", Keyword: "required"}}},
		{"entries ok", KindConceptListEntries, "- http://x/1\n- http://x/2\n", nil},
		{"concept list vocabulary", KindConceptList, "semantic_id: SI.CL.1\nvocabulary: SI.VOC.1\n", nil},
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

func TestKindForPathGrafts(t *testing.T) {
	if k, ok := KindForPath("models/SIM.25/grafts.yaml"); !ok || k != KindGrafts {
		t.Fatalf("KindForPath = %q %v, want grafts", k, ok)
	}
	if _, ok := KindForPath("generators/arches/survey/aliases.yaml"); ok {
		t.Fatal("generator files have no format kind")
	}
}

func TestGraftsSchema(t *testing.T) {
	good := []byte("LAF.177/LAF.100: {max_occurs: 1, display_name: {en: Name for Time Span}}\nLAF.177/LAF.100/LAF.31: {is_hidden: true}\n")
	if v, err := Default().Validate(KindGrafts, good); err != nil || len(v) != 0 {
		t.Fatalf("good file: %v %v", v, err)
	}
	for name, bad := range map[string]string{
		"top-level address": "LAF.177: {is_hidden: true}\n",
		"unknown key":       "LAF.177/LAF.100: {hidden: true}\n",
		"bad max":           "LAF.177/LAF.100: {max_occurs: two}\n",
	} {
		if v, _ := Default().Validate(KindGrafts, []byte(bad)); len(v) == 0 {
			t.Errorf("%s: no violation", name)
		}
	}
}
