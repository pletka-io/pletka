package gitmaterializer

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/canonical"
)

// richPathElements exercises every attribute the path_elements JSONB
// contract carries (verified against the live DB: uri, type, prefix,
// local_name, position, class_code, instance_id, datatype, complete,
// sub_property_of, additional_types). Order is deliberately non-alphabetical
// so an order regression cannot pass.
func richPathElements() []domain.PathElement {
	return []domain.PathElement{
		{
			Type: "property", URI: "crm:P67_refers_to", Prefix: "crm",
			LocalName: "P67_refers_to", Position: 0,
		},
		{
			Type: "class", URI: "crm:E6_Destruction", Prefix: "crm",
			LocalName: "E6_Destruction", Position: 1,
			ClassCode: "E6", InstanceID: "62_1",
			AdditionalTypes: []domain.TypeRef{{
				URI: "crm:E7_Activity", Prefix: "crm",
				LocalName: "E7_Activity", ClassCode: "E7",
			}},
		},
		{
			Type: "property", URI: "crm:P67.1_has_type", Prefix: "crm",
			LocalName: "P67.1_has_type", Position: 2,
			SubPropertyOf: "P67",
		},
		{
			Type: "literal", URI: "rdfs:Literal", Prefix: "rdfs",
			LocalName: "Literal", Position: 3,
			Datatype: "xsd:date", Complete: true,
		},
	}
}

// TestFieldSnapshotRoundTripLossless proves the write side
// (canonical.Field) and the restore side (decodeCanonicalYAML +
// decodePathElement*/decodeSubfieldPaths) preserve every path attribute,
// the subfield paths, and element order exactly.
func TestFieldSnapshotRoundTripLossless(t *testing.T) {
	field := &domain.Field{
		Entity: domain.Entity{
			SemanticID: "TSTF.1",
			SystemName: "lossless_probe",
			UIName:     domain.Translations{"en": "Lossless Probe"},
			Status:     "published",
			ProjectID:  "TST",
		},
		OntologyScope: domain.PathElement{
			Type: "class", URI: "aaao:ZE8_Ownership_Status", Prefix: "aaao",
			LocalName: "ZE8_Ownership_Status", ClassCode: "ZE8",
		},
		PathElements:      richPathElements(),
		ExpectedValueType: "Date",
		SubfieldPaths: []domain.SubfieldPath{
			{
				PathElements: []domain.PathElement{
					{
						Type: "property", URI: "crm:P1_is_identified_by", Prefix: "crm",
						LocalName: "P1_is_identified_by", Position: 0, InstanceID: "HERF.1_1",
					},
					{
						Type: "class", URI: "crm:E41_Appellation", Prefix: "crm",
						LocalName: "E41_Appellation", Position: 1, ClassCode: "E41",
					},
				},
				ExpectedValueType: "String",
				Scope:             "crm:E21_Person",
				Source:            "airtable",
			},
		},
	}

	payload, err := canonical.Field(field)
	if err != nil {
		t.Fatalf("canonical.Field: %v", err)
	}

	doc, err := decodeCanonicalYAML[canonicalFieldDoc](payload)
	if err != nil {
		t.Fatalf("decode canonical yaml: %v", err)
	}

	gotScope, err := decodePathElement(doc.OntologyScope, "class", 0)
	if err != nil {
		t.Fatalf("decode scope: %v", err)
	}
	if diff := cmp.Diff(field.OntologyScope, gotScope); diff != "" {
		t.Errorf("ontology_scope not lossless (-want +got):\n%s", diff)
	}

	gotElements, err := decodePathElements(doc.PathElements)
	if err != nil {
		t.Fatalf("decode path_elements: %v", err)
	}
	if diff := cmp.Diff(field.PathElements, gotElements); diff != "" {
		t.Errorf("path_elements not lossless (-want +got):\n%s", diff)
	}

	gotSubfields, err := decodeSubfieldPaths(doc.SubfieldPaths)
	if err != nil {
		t.Fatalf("decode subfield_paths: %v", err)
	}
	if diff := cmp.Diff(field.SubfieldPaths, gotSubfields); diff != "" {
		t.Errorf("subfield_paths not lossless (-want +got):\n%s", diff)
	}
}

// TestModelCollectionScopeRoundTripLossless covers the scope element on
// models and collections (full map, not compact qname).
func TestModelCollectionScopeRoundTripLossless(t *testing.T) {
	scope := domain.PathElement{
		Type: "class", URI: "crm:E67_Birth", Prefix: "crm",
		LocalName: "E67_Birth", ClassCode: "E67",
	}

	model := &domain.Model{
		Entity:        domain.Entity{SemanticID: "TSTM.1", UIName: domain.Translations{"en": "M"}, Status: "draft"},
		OntologyScope: scope,
	}
	payload, err := canonical.Model(model)
	if err != nil {
		t.Fatalf("canonical.Model: %v", err)
	}
	doc, err := decodeCanonicalYAML[canonicalModelDoc](payload)
	if err != nil {
		t.Fatalf("decode model yaml: %v", err)
	}
	got, err := decodePathElement(doc.OntologyScope, "class", 0)
	if err != nil {
		t.Fatalf("decode model scope: %v", err)
	}
	if diff := cmp.Diff(scope, got); diff != "" {
		t.Errorf("model scope not lossless (-want +got):\n%s", diff)
	}

	col := &domain.Collection{
		Entity:        domain.Entity{SemanticID: "TSTC.1", UIName: domain.Translations{"en": "C"}, Status: "draft"},
		OntologyScope: scope,
	}
	payload, err = canonical.Collection(col)
	if err != nil {
		t.Fatalf("canonical.Collection: %v", err)
	}
	cdoc, err := decodeCanonicalYAML[canonicalCollectionDoc](payload)
	if err != nil {
		t.Fatalf("decode collection yaml: %v", err)
	}
	got, err = decodePathElement(cdoc.OntologyScope, "class", 0)
	if err != nil {
		t.Fatalf("decode collection scope: %v", err)
	}
	if diff := cmp.Diff(scope, got); diff != "" {
		t.Errorf("collection scope not lossless (-want +got):\n%s", diff)
	}
}

// TestDecodePathElementLegacyCompat: snapshots written before the lossless
// format carry compact qname strings — they must keep decoding via the
// best-effort parser (type inferred, multi-type "/" split).
func TestDecodePathElementLegacyCompat(t *testing.T) {
	doc, err := decodeCanonicalYAML[canonicalFieldDoc]([]byte(`
semantic_id: OLDF.1
ontology_scope: crm:E21_Person
path_elements:
  - crm:P1_is_identified_by
  - crm:E41_Appellation/crmdig:D1_Digital_Object
`))
	if err != nil {
		t.Fatalf("decode legacy yaml: %v", err)
	}

	scope, err := decodePathElement(doc.OntologyScope, "class", 0)
	if err != nil {
		t.Fatalf("decode legacy scope: %v", err)
	}
	if scope.URI != "crm:E21_Person" || scope.Type != "class" {
		t.Errorf("legacy scope mismatch: %+v", scope)
	}

	elements, err := decodePathElements(doc.PathElements)
	if err != nil {
		t.Fatalf("decode legacy elements: %v", err)
	}
	if len(elements) != 2 {
		t.Fatalf("want 2 legacy elements, got %d", len(elements))
	}
	if elements[0].URI != "crm:P1_is_identified_by" || elements[0].Position != 0 {
		t.Errorf("legacy element 0 mismatch: %+v", elements[0])
	}
	if len(elements[1].AdditionalTypes) != 1 || elements[1].AdditionalTypes[0].Prefix != "crmdig" {
		t.Errorf("legacy multi-type element mismatch: %+v", elements[1])
	}
}
