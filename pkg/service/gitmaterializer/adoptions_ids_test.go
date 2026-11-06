package gitmaterializer

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/domain"
)

// TestRewriteAdoptionIDs pins that both the source entity (in the source
// project) and the context entity (in the adopting project) go through the
// resolver with their own type and project, and that an unresolved id is
// kept as written.
func TestRewriteAdoptionIDs(t *testing.T) {
	calls := map[string]string{}
	resolve := func(_ context.Context, entityType, projectID, id string) (string, bool) {
		calls[entityType+"/"+projectID+"/"+id] = ""
		switch id {
		case "01ULIDCATEGORY":
			return "PAR.CAT.11", true
		case "01ULIDMODEL":
			return "CHM.3", true
		}
		return "", false
	}
	in := []domain.Adoption{
		{ProjectID: "CH", ContextEntityType: "model", ContextEntityID: "01ULIDMODEL", EntityType: "category", SourceProjectID: "PAR", SourceEntityID: "01ULIDCATEGORY"},
		{ProjectID: "CH", ContextEntityType: "model", ContextEntityID: "CHM.1", EntityType: "field", SourceProjectID: "PAR", SourceEntityID: "PARF.7"},
	}
	got := rewriteAdoptionIDs(context.Background(), in, resolve)
	want := []domain.Adoption{
		{ProjectID: "CH", ContextEntityType: "model", ContextEntityID: "CHM.3", EntityType: "category", SourceProjectID: "PAR", SourceEntityID: "PAR.CAT.11"},
		{ProjectID: "CH", ContextEntityType: "model", ContextEntityID: "CHM.1", EntityType: "field", SourceProjectID: "PAR", SourceEntityID: "PARF.7"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("(-want +got):\n%s", diff)
	}
	for _, k := range []string{"category/PAR/01ULIDCATEGORY", "model/CH/01ULIDMODEL", "field/PAR/PARF.7", "model/CH/CHM.1"} {
		if _, ok := calls[k]; !ok {
			t.Errorf("resolver not asked for %s (asked: %v)", k, calls)
		}
	}
}
