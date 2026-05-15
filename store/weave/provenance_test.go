package weave

import (
	"context"
	"testing"
	"time"

	"github.com/pletka-io/pletka/domain"
	domainweave "github.com/pletka-io/pletka/domain/weave"
)

func TestAdoptionAndForkStoreNilGuards(t *testing.T) {
	ctx := context.Background()
	var adoptions *adoptionStore
	var forks *forkStore

	if _, err := adoptions.List(ctx); err == nil {
		t.Fatalf("nil adoption List() error = nil; want error")
	}
	if err := adoptions.ReplaceForContext(ctx, "TPC", "model", "M1", nil); err == nil {
		t.Fatalf("nil adoption ReplaceForContext() error = nil; want error")
	}
	if _, err := forks.List(ctx); err == nil {
		t.Fatalf("nil fork List() error = nil; want error")
	}
	if err := forks.Create(ctx, &domainweave.EntityFork{}); err == nil {
		t.Fatalf("nil fork Create() error = nil; want error")
	}
}

func TestBuildProvenanceQuery(t *testing.T) {
	cfg := domain.ApplyOptions([]domain.QueryOption{
		domain.WithProjectID("TPC"),
		domain.WithFilter("entity_type", " model "),
		domain.WithFilter("ignored", "value"),
		domain.WithVersion("v1"),
	})

	query := buildProvenanceQuery(cfg, map[string]string{"entity_type": "entity_type"})
	if query.where != " WHERE project_id = $1 AND entity_type = $2 AND version_number = $3" {
		t.Fatalf("where = %q", query.where)
	}
	if len(query.args) != 3 || query.args[0] != "TPC" || query.args[1] != "model" || query.args[2] != "v1" {
		t.Fatalf("args = %#v", query.args)
	}
}

func TestAdoptionKeyIncludesSourceVersion(t *testing.T) {
	base := domainweave.Adoption{
		ProjectID:         "TPC",
		ContextEntityType: "model",
		ContextEntityID:   "LAM.1",
		EntityType:        "field",
		SourceProjectID:   "LA",
		SourceEntityID:    "LAM.5",
	}
	versioned := base
	versioned.SourceVersion = "v1"

	if adoptionKey(base) == adoptionKey(versioned) {
		t.Fatalf("adoptionKey should include source version")
	}
}

func TestNullableTime(t *testing.T) {
	if nullableTime(time.Time{}) != nil {
		t.Fatalf("nullableTime(zero) != nil")
	}
	when := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	if got := nullableTime(when); got == nil || !got.Equal(when) {
		t.Fatalf("nullableTime(when) = %#v", got)
	}
}
