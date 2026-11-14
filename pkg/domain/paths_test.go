package domain

import "testing"

func TestFilePath(t *testing.T) {
	cases := []struct {
		name string
		spec PathSpec
		want string
	}{
		{
			name: "project",
			spec: PathSpec{EntityType: "project", EntityID: "LA"},
			want: "project.yaml",
		},
		{
			name: "category",
			spec: PathSpec{EntityType: "category", EntityID: "LA.CAT.5"},
			want: "categories/LA.CAT.5.yaml",
		},
		{
			name: "field",
			spec: PathSpec{EntityType: "field", EntityID: "LAF.18"},
			want: "fields/LAF.18/field.yaml",
		},
		{
			name: "model",
			spec: PathSpec{EntityType: "model", EntityID: "LAM.15"},
			want: "models/LAM.15/model.yaml",
		},
		{
			name: "collection",
			spec: PathSpec{EntityType: "collection", EntityID: "LAC.8"},
			want: "collections/LAC.8/collection.yaml",
		},
		{
			name: "base_override",
			spec: PathSpec{EntityType: "base_override", EntityID: "900", FieldID: "LAF.18"},
			want: "fields/LAF.18/base-override.yaml",
		},
		{
			name: "model_override",
			spec: PathSpec{
				EntityType: "model_override",
				EntityID:   OverrideToken("LA.CAT.2", ""),
				FieldID:    "LAF.18",
				OwnerType:  "model",
				OwnerID:    "LAM.15",
			},
			want: "models/LAM.15/overrides/LAF.18@LA.CAT.2.yaml",
		},
		{
			name: "collection_override",
			spec: PathSpec{
				EntityType: "collection_override",
				EntityID:   OverrideToken("", "LAC.3"),
				FieldID:    "LAF.18",
				OwnerType:  "collection",
				OwnerID:    "LAC.8",
			},
			want: "collections/LAC.8/overrides/LAF.18@_@LAC.3.yaml",
		},
		{
			name: "unknown",
			spec: PathSpec{EntityType: "weird", EntityID: "xyz"},
			want: "unknown/weird-xyz.yaml",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FilePath(tc.spec)
			if got != tc.want {
				t.Errorf("FilePath(%+v) = %q, want %q", tc.spec, got, tc.want)
			}
		})
	}
}

func TestVocabularyPaths(t *testing.T) {
	spec := PathSpec{EntityType: EntityTypeVocabularyPath, EntityID: "SI.VOC.1"}
	if got := FilePath(spec); got != "vocabularies/SI.VOC.1/vocabulary.yaml" {
		t.Fatalf("FilePath = %q", got)
	}
	if got := VocabularyConceptsPath("SI.VOC.1"); got != "vocabularies/SI.VOC.1/concepts.yaml" {
		t.Fatalf("VocabularyConceptsPath = %q", got)
	}
	if got := ConceptListEntriesPath("SI.CL.4"); got != "concept-lists/SI.CL.4/entries.yaml" {
		t.Fatalf("ConceptListEntriesPath = %q", got)
	}
}
