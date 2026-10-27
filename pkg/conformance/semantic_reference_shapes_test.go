package conformance

import (
	"reflect"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/domain"
)

// Adding a path attribute is an inventory decision even when it looks like
// display metadata. Force that review before the archive cut can omit it.
func TestSemanticAttributesRequireInventoryReview(t *testing.T) {
	for _, tc := range []struct {
		typ    reflect.Type
		fields []string
	}{
		{reflect.TypeFor[domain.PathElement](), []string{"Type", "URI", "Prefix", "LocalName", "Datatype", "Position", "Complete", "ClassCode", "InstanceID", "PathNode", "PathNodeID", "OntologyVersionID", "AdditionalTypes", "SubPropertyOf"}},
		{reflect.TypeFor[domain.TypeRef](), []string{"URI", "Prefix", "LocalName", "ClassCode"}},
		{reflect.TypeFor[domain.SubfieldPath](), []string{"PathElements", "ExpectedValueType", "Scope", "Source"}},
	} {
		actual := make([]string, tc.typ.NumField())
		for i := range actual {
			actual[i] = tc.typ.Field(i).Name
		}
		slices.Sort(actual)
		slices.Sort(tc.fields)
		if diff := cmp.Diff(tc.fields, actual); diff != "" {
			t.Errorf("%s changed: update the shared semantic inventory or document its exclusion in ADR-0015:\n%s", tc.typ.Name(), diff)
		}
	}
}

func TestSemanticLocationsRequireInventoryReview(t *testing.T) {
	for _, tc := range []struct {
		typ    reflect.Type
		fields []string
	}{
		{reflect.TypeFor[domain.Field](), []string{"OntologyScope", "PathElements", "SubfieldPaths"}},
		{reflect.TypeFor[domain.Model](), []string{"OntologyScope"}},
		{reflect.TypeFor[domain.Collection](), []string{"OntologyScope"}},
	} {
		var actual []string
		for i := range tc.typ.NumField() {
			field := tc.typ.Field(i)
			typ := field.Type
			for typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array || typ.Kind() == reflect.Pointer {
				typ = typ.Elem()
			}
			if typ == reflect.TypeFor[domain.PathElement]() || typ == reflect.TypeFor[domain.TypeRef]() || typ == reflect.TypeFor[domain.SubfieldPath]() {
				actual = append(actual, field.Name)
			}
		}
		slices.Sort(actual)
		slices.Sort(tc.fields)
		if diff := cmp.Diff(tc.fields, actual); diff != "" {
			t.Errorf("%s semantic locations changed: extend capture/storage coverage in the same change:\n%s", tc.typ.Name(), diff)
		}
	}
}
