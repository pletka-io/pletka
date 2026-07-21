//go:build integration

package ontology_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/weave"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
)

// ---------------------------------------------------------------------------
// Edge-provenance smoke test — requires local DB at localhost:5433
// ---------------------------------------------------------------------------

// TestEdgeProvenance_AME_SubclassOf_Smoke verifies that a known subclass_of
// edge in AME (crm:E2_Temporal_Entity → crm:E1_CRM_Entity) is found in at
// least one linked version.
func TestEdgeProvenance_AME_SubclassOf_Smoke(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	store := weaveontology.NewPostgresStore(pool)
	ws := weave.NewPostgresStore(pool)
	svc := weaveontology.NewService(store, ws.Projects(), nil)

	versions, err := svc.EdgeProvenance(
		context.Background(),
		"AME",
		"crm:E2_Temporal_Entity",
		"crm:E1_CRM_Entity",
		"subclass_of",
	)
	if err != nil {
		t.Fatalf("EdgeProvenance: %v", err)
	}
	if len(versions) == 0 {
		t.Fatal("expected at least one version declaring crm:E2_Temporal_Entity subclass_of crm:E1_CRM_Entity in AME")
	}
	for _, v := range versions {
		if v.VersionID == "" {
			t.Errorf("version entry missing version_id: %+v", v)
		}
		if v.VersionString == "" {
			t.Errorf("version entry missing version_string: %+v", v)
		}
		if v.Prefix == "" {
			t.Errorf("version entry missing prefix: %+v", v)
		}
	}
	t.Logf("found %d version(s): %+v", len(versions), versions)
}
