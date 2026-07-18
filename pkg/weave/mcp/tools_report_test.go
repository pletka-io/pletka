package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

// reportFieldWithName is fieldWithPath (tools_entities_test.go) plus a
// SystemName, for name_collisions fixtures.
func reportFieldWithName(id, semantic, systemName, prefix, local string) *domain.Field {
	f := fieldWithPath(id, semantic, prefix, local)
	f.SystemName = systemName
	return f
}

// reportFieldNoPath builds a field with no ontology path at all — the case
// path_collisions must exclude from grouping.
func reportFieldNoPath(id, semantic, systemName string) *domain.Field {
	f := &domain.Field{}
	f.ID = id
	f.SemanticID = semantic
	f.SystemName = systemName
	f.Status = domain.Status("published")
	return f
}

func modelRef(semanticID string) domain.FieldUsageRef {
	return domain.FieldUsageRef{SemanticID: semanticID}
}

// TestProjectReportOrphans verifies a field with no owners is reported as an
// orphan and a placed field is not.
func TestProjectReportOrphans(t *testing.T) {
	h := testHostEntities()
	owned := reportFieldWithName("01A", "LAF.1", "owned_field", "crm", "P1_is_identified_by")
	orphan := reportFieldWithName("01B", "LAF.2", "orphan_field", "crm", "P2_has_type")
	h.Fields = &fakeFields{
		fields: []*domain.Field{owned, orphan},
		refs: map[string]domain.FieldUsageList{
			"01A": {Models: []domain.FieldUsageRef{modelRef("LAM.1")}},
		},
	}

	out, err := projectReport(context.Background(), h, projectReportInput{ProjectID: "LA", Checks: []string{"orphans"}})
	if err != nil {
		t.Fatalf("project_report: %v", err)
	}
	if len(out.Orphans) != 1 || out.Orphans[0].SemanticID != "LAF.2" {
		t.Fatalf("want orphan LAF.2 only, got %+v", out.Orphans)
	}
	if out.PathCollisions != nil || out.NameCollisions != nil {
		t.Fatalf("want other checks unset when only orphans requested, got %+v / %+v", out.PathCollisions, out.NameCollisions)
	}
	if out.FieldsScanned != 2 {
		t.Fatalf("want FieldsScanned 2, got %d", out.FieldsScanned)
	}
}

// TestProjectReportPathCollisionsSameModel verifies same_model is true when
// one model (by semantic ID) owns >=2 members of a path-collision group, and
// false when the group's owners are disjoint models.
func TestProjectReportPathCollisionsSameModel(t *testing.T) {
	h := testHostEntities()
	// Group 1: two fields sharing a path, both owned (in part) by LAM.1.
	g1a := reportFieldWithName("01A", "LAF.1", "field_a", "crm", "P1_is_identified_by")
	g1b := reportFieldWithName("01B", "LAF.2", "field_b", "crm", "P1_is_identified_by")
	// Group 2: two fields sharing a different path, owned by disjoint models.
	g2a := reportFieldWithName("01C", "LAF.3", "field_c", "crm", "P2_has_type")
	g2b := reportFieldWithName("01D", "LAF.4", "field_d", "crm", "P2_has_type")

	h.Fields = &fakeFields{
		fields: []*domain.Field{g1a, g1b, g2a, g2b},
		refs: map[string]domain.FieldUsageList{
			"01A": {Models: []domain.FieldUsageRef{modelRef("LAM.1")}},
			"01B": {Models: []domain.FieldUsageRef{modelRef("LAM.1")}},
			"01C": {Models: []domain.FieldUsageRef{modelRef("LAM.2")}},
			"01D": {Models: []domain.FieldUsageRef{modelRef("LAM.3")}},
		},
	}

	out, err := projectReport(context.Background(), h, projectReportInput{ProjectID: "LA", Checks: []string{"path_collisions"}})
	if err != nil {
		t.Fatalf("project_report: %v", err)
	}
	if len(out.PathCollisions) != 2 {
		t.Fatalf("want 2 collision groups, got %d: %+v", len(out.PathCollisions), out.PathCollisions)
	}
	var sawSameModelTrue, sawSameModelFalse bool
	for _, grp := range out.PathCollisions {
		if len(grp.Fields) != 2 {
			t.Fatalf("want 2 fields per group, got %d: %+v", len(grp.Fields), grp)
		}
		switch grp.Fields[0].SemanticID {
		case "LAF.1":
			if !grp.SameModel {
				t.Fatalf("want SameModel true for LAM.1-owned group, got %+v", grp)
			}
			sawSameModelTrue = true
		case "LAF.3":
			if grp.SameModel {
				t.Fatalf("want SameModel false for disjoint-owner group, got %+v", grp)
			}
			sawSameModelFalse = true
		default:
			t.Fatalf("unexpected group leading field %+v", grp)
		}
	}
	if !sawSameModelTrue || !sawSameModelFalse {
		t.Fatalf("want both same_model true and false groups, got %+v", out.PathCollisions)
	}
	// Fields within a group ordered by SemanticID ascending.
	for _, grp := range out.PathCollisions {
		if grp.Fields[0].SemanticID > grp.Fields[1].SemanticID {
			t.Fatalf("want fields ordered by SemanticID ascending, got %+v", grp.Fields)
		}
	}
}

// TestProjectReportNameCollisionsSameModel is the name_collisions analog of
// TestProjectReportPathCollisionsSameModel.
func TestProjectReportNameCollisionsSameModel(t *testing.T) {
	h := testHostEntities()
	g1a := reportFieldNoPath("01A", "LAF.1", "shared_name")
	g1b := reportFieldNoPath("01B", "LAF.2", "shared_name")
	g2a := reportFieldNoPath("01C", "LAF.3", "other_shared_name")
	g2b := reportFieldNoPath("01D", "LAF.4", "other_shared_name")

	h.Fields = &fakeFields{
		fields: []*domain.Field{g1a, g1b, g2a, g2b},
		refs: map[string]domain.FieldUsageList{
			"01A": {Models: []domain.FieldUsageRef{modelRef("LAM.1")}},
			"01B": {Models: []domain.FieldUsageRef{modelRef("LAM.1")}},
			"01C": {Models: []domain.FieldUsageRef{modelRef("LAM.2")}},
			"01D": {Models: []domain.FieldUsageRef{modelRef("LAM.3")}},
		},
	}

	out, err := projectReport(context.Background(), h, projectReportInput{ProjectID: "LA", Checks: []string{"name_collisions"}})
	if err != nil {
		t.Fatalf("project_report: %v", err)
	}
	if len(out.NameCollisions) != 2 {
		t.Fatalf("want 2 collision groups, got %d: %+v", len(out.NameCollisions), out.NameCollisions)
	}
	for _, grp := range out.NameCollisions {
		switch grp.Value {
		case "shared_name":
			if !grp.SameModel {
				t.Fatalf("want SameModel true for shared_name group, got %+v", grp)
			}
		case "other_shared_name":
			if grp.SameModel {
				t.Fatalf("want SameModel false for other_shared_name group, got %+v", grp)
			}
		default:
			t.Fatalf("unexpected group value %q", grp.Value)
		}
	}
}

// TestProjectReportUnknownCheck verifies an unrecognized check name errors
// and names the valid set.
func TestProjectReportUnknownCheck(t *testing.T) {
	h := testHostEntities()
	h.Fields = &fakeFields{}
	_, err := projectReport(context.Background(), h, projectReportInput{ProjectID: "LA", Checks: []string{"bogus"}})
	if err == nil {
		t.Fatal("want error for unknown check")
	}
	for _, want := range []string{"orphans", "path_collisions", "name_collisions"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("want error naming %q, got %v", want, err)
		}
	}
}

// TestProjectReportEmptyChecksRunsAll verifies an empty Checks input runs
// all three checks.
func TestProjectReportEmptyChecksRunsAll(t *testing.T) {
	h := testHostEntities()
	orphan := reportFieldWithName("01A", "LAF.1", "orphan_field", "crm", "P1_is_identified_by")
	g1a := reportFieldWithName("01B", "LAF.2", "shared_name", "crm", "P2_has_type")
	g1b := reportFieldWithName("01C", "LAF.3", "shared_name", "crm", "P2_has_type")

	h.Fields = &fakeFields{fields: []*domain.Field{orphan, g1a, g1b}}

	out, err := projectReport(context.Background(), h, projectReportInput{ProjectID: "LA"})
	if err != nil {
		t.Fatalf("project_report: %v", err)
	}
	if len(out.Orphans) != 3 {
		// all three fields are unowned (no refs configured)
		t.Fatalf("want 3 orphans, got %d: %+v", len(out.Orphans), out.Orphans)
	}
	if len(out.PathCollisions) != 1 || out.PathCollisions[0].Value == "" {
		t.Fatalf("want 1 path collision group, got %+v", out.PathCollisions)
	}
	if len(out.NameCollisions) != 1 || out.NameCollisions[0].Value != "shared_name" {
		t.Fatalf("want 1 name collision group 'shared_name', got %+v", out.NameCollisions)
	}
}

// TestProjectReportEmptyPathExcluded verifies fields with no ontology path
// are excluded from path_collisions grouping, even though they'd otherwise
// share the same (empty) key.
func TestProjectReportEmptyPathExcluded(t *testing.T) {
	h := testHostEntities()
	a := reportFieldNoPath("01A", "LAF.1", "field_a")
	b := reportFieldNoPath("01B", "LAF.2", "field_b")
	h.Fields = &fakeFields{fields: []*domain.Field{a, b}}

	out, err := projectReport(context.Background(), h, projectReportInput{ProjectID: "LA", Checks: []string{"path_collisions"}})
	if err != nil {
		t.Fatalf("project_report: %v", err)
	}
	if len(out.PathCollisions) != 0 {
		t.Fatalf("want no path collisions for empty-path fields, got %+v", out.PathCollisions)
	}
}

// TestProjectReportTruncated verifies Truncated is set when the store's
// total exceeds the rows the fallbackScanLimit-bounded fetch returned.
func TestProjectReportTruncated(t *testing.T) {
	h := testHostEntities()
	fields := []*domain.Field{
		reportFieldNoPath("01A", "LAF.1", "a"),
		reportFieldNoPath("01B", "LAF.2", "b"),
		reportFieldNoPath("01C", "LAF.3", "c"),
	}
	h.Fields = &fakeFields{fields: fields, totalOverride: 5}

	out, err := projectReport(context.Background(), h, projectReportInput{ProjectID: "LA"})
	if err != nil {
		t.Fatalf("project_report: %v", err)
	}
	if !out.Truncated {
		t.Fatal("want Truncated true when store total exceeds scanned rows")
	}
	if out.FieldsScanned != 3 {
		t.Fatalf("want FieldsScanned 3 (rows actually returned), got %d", out.FieldsScanned)
	}
}
