package generators

import (
	"context"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

type fakeGraftReader struct {
	gotProject, gotModel string
	grafts               map[string]domain.GraftOverride
}

func (f *fakeGraftReader) GraftOverrides(_ context.Context, projectID, modelID string) (map[string]domain.GraftOverride, error) {
	f.gotProject, f.gotModel = projectID, modelID
	return f.grafts, nil
}

type graftProjectReader struct{}

func (graftProjectReader) GetByID(_ context.Context, id string) (*domain.Project, error) {
	return &domain.Project{Entity: domain.Entity{ID: id}}, nil
}

func (graftProjectReader) ResolvedOntologyVersions(context.Context, string, domain.ResolvedOntologyVersionOpts) ([]domain.ResolvedOntologyVersion, error) {
	return nil, nil
}

type graftModelReader struct{ view *domain.ModelView }

func (graftModelReader) Get(_ context.Context, projectID, id string) (*domain.Model, error) {
	return &domain.Model{Entity: domain.Entity{ID: id, SemanticID: "OGM.1", ProjectID: projectID}}, nil
}

func (r graftModelReader) View(context.Context, string, string) (*domain.ModelView, error) {
	return r.view, nil
}

type graftNamespaceReader struct{}

func (graftNamespaceReader) ListForProject(context.Context, string) ([]*domain.NamespaceBinding, error) {
	return nil, nil
}

// serviceWithNestedModel is a service over one model: a top-level stub on
// Timespan, which holds a stub on Name (refFixture). The graft of the inner
// stub sits at OGF.1/LAC.6.F2.
func serviceWithNestedModel() *Service {
	refs, view := refFixture()
	refs.colls["c-act"] = &domain.Collection{Entity: domain.Entity{ID: "c-act", SemanticID: "OGC.1", SystemName: "activity", ProjectID: "OG"}}
	return NewService(graftProjectReader{}, graftModelReader{view: view}, refs, nil, graftNamespaceReader{}, nil)
}

func TestSnapshotForModelAppliesGraftOverrides(t *testing.T) {
	svc := serviceWithNestedModel()
	const hidden = "OGF.1/LAC.6.F2"
	r := &fakeGraftReader{grafts: map[string]domain.GraftOverride{hidden: {Address: hidden, IsHidden: true}}}
	svc.SetGraftOverrideReader(r)
	snap, err := svc.SnapshotForModel(context.Background(), "OG", "m-1", Options{ComposeCollections: true})
	if err != nil {
		t.Fatal(err)
	}
	if r.gotProject != "OG" || r.gotModel != "OGM.1" {
		t.Fatalf("reader asked for %s/%s, want OG/OGM.1 (the tree's model id)", r.gotProject, r.gotModel)
	}
	var top bool
	for _, f := range snap.Fields {
		if strings.HasPrefix(f.Address, hidden) {
			t.Fatalf("hidden graft in snapshot: %s", f.Address)
		}
		top = top || f.Address == "OGF.1"
	}
	if !top {
		t.Fatal("top-level stub missing from snapshot")
	}
}

func TestSnapshotForModelWithoutGraftReaderGraftsAll(t *testing.T) {
	snap, err := serviceWithNestedModel().SnapshotForModel(context.Background(), "OG", "m-1", Options{ComposeCollections: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range snap.Fields {
		if f.Address == "OGF.1/LAC.6.F2/LAC.1.F1" {
			return
		}
	}
	t.Fatal("nested graft missing without a graft reader")
}
