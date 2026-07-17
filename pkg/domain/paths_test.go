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
				EntityID:   "900",
				FieldID:    "LAF.18",
				OwnerType:  "model",
				OwnerID:    "LAM.15",
			},
			want: "models/LAM.15/overrides/LAF.18@900.yaml",
		},
		{
			name: "collection_override",
			spec: PathSpec{
				EntityType: "collection_override",
				EntityID:   "1234",
				FieldID:    "LAF.18",
				OwnerType:  "collection",
				OwnerID:    "LAC.8",
			},
			want: "collections/LAC.8/overrides/LAF.18@1234.yaml",
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
