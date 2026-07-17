package projectontologyversion

// TestService_PublishesEventOnMutations verifies that Service.Create,
// Update, and Delete each publish a ProjectOntologyVersionsChanged event
// on the wired EventBus. This is the unit-level gate for the bug where
// the EventBus was nil at mount time (router.go wired it after the
// projectontologyversion slice mounted), so no events were ever fired
// and the IndexCache was never invalidated on version-link changes.
//
// Test level chosen: pure unit test (no DB) using an in-memory fake Store
// and SimpleEventBus. The service's auth path is bypassed by injecting a
// super-admin AuthSnapshot (IsSuperAdmin = true short-circuits Can()
// checks) and a fake ProjectReader that returns the project. This avoids
// a testPool dependency while still exercising the real publish call-site
// in Create/Update/Delete.

import (
	"context"
	"testing"
	"time"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

// ---------------------------------------------------------------------------
// Minimal store fake — only the methods the publish path exercises.
// ---------------------------------------------------------------------------

type fakeStore struct {
	rows map[string]*domain.ProjectOntologyVersion // key: versionID
}

func newFakeStore() *fakeStore {
	return &fakeStore{rows: map[string]*domain.ProjectOntologyVersion{}}
}

func (s *fakeStore) List(_ context.Context, projectID string) ([]*domain.ProjectOntologyVersion, error) {
	var out []*domain.ProjectOntologyVersion
	for _, row := range s.rows {
		if row.ProjectID == projectID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (s *fakeStore) Get(_ context.Context, _, versionID string) (*domain.ProjectOntologyVersion, error) {
	return s.rows[versionID], nil
}

func (s *fakeStore) Create(_ context.Context, link *domain.ProjectOntologyVersion) error {
	cp := *link
	s.rows[link.OntologyVersionID] = &cp
	return nil
}

func (s *fakeStore) Update(_ context.Context, link *domain.ProjectOntologyVersion) error {
	if existing, ok := s.rows[link.OntologyVersionID]; ok {
		existing.IsPrimary = link.IsPrimary
		existing.UsageNotes = link.UsageNotes
	}
	return nil
}

func (s *fakeStore) Delete(_ context.Context, _, versionID string) error {
	delete(s.rows, versionID)
	return nil
}

func (s *fakeStore) SetPrimary(_ context.Context, _, versionID string) error {
	for _, row := range s.rows {
		row.IsPrimary = false
	}
	if row, ok := s.rows[versionID]; ok {
		row.IsPrimary = true
	}
	return nil
}

func (s *fakeStore) ListWithCounts(_ context.Context, _ string) ([]domain.ProjectOntologyVersionWithCounts, error) {
	return nil, nil
}

func (s *fakeStore) BundleForProject(_ context.Context, _ string) ([]domain.OntologyBundleEntry, error) {
	return nil, nil
}

func (s *fakeStore) BundleForVersions(_ context.Context, _ []string) ([]domain.OntologyBundleEntry, error) {
	return nil, nil
}

func (s *fakeStore) ListGrouped(_ context.Context, _ string) ([]domain.LinkedOntologyGroup, error) {
	return nil, nil
}

func (s *fakeStore) CountPathElementUsage(_ context.Context, _, _ string) (int64, error) {
	return 0, nil
}

func (s *fakeStore) SamplePathElementFields(_ context.Context, _, _ string, _ int) ([]domain.FieldUsageSample, error) {
	return nil, nil
}

func (s *fakeStore) OntologyUsage(_ context.Context, _, _ string) (int, int, error) {
	return 0, 0, nil
}

// ---------------------------------------------------------------------------
// Fake ProjectReader — returns a single project for ensureProjectExists.
// ---------------------------------------------------------------------------

type fakeProjectReader struct {
	project *domain.Project
}

func (r *fakeProjectReader) GetByID(_ context.Context, _ string) (*domain.Project, error) {
	return r.project, nil
}

func (r *fakeProjectReader) ResolvedOntologyVersions(_ context.Context, _ string, _ domain.ResolvedOntologyVersionOpts) ([]domain.ResolvedOntologyVersion, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// Fake OntologyReader + VersionReader — not exercised by publish path tests.
// ---------------------------------------------------------------------------

type noopOntologyReader struct{}

func (noopOntologyReader) List(_ context.Context) ([]*domain.Ontology, error) { return nil, nil }
func (noopOntologyReader) GetByID(_ context.Context, _ string) (*domain.Ontology, error) {
	return nil, nil
}

type noopVersionReader struct{}

func (noopVersionReader) GetByID(_ context.Context, _ string) (*domain.OntologyVersion, error) {
	return nil, nil
}
func (noopVersionReader) ListByOntology(_ context.Context, _ string) ([]*domain.OntologyVersion, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

const testProjectID = "test-project-01"
const testVersionID = "ont-version-01"

// superAdminCtx returns a context with a super-admin auth snapshot and a
// minimal project, which satisfies both requireProjectWrite and
// ensureProjectExists without a database.
func superAdminCtx() context.Context {
	snap := &weaveauth.AuthSnapshot{IsSuperAdmin: true}
	project := &domain.Project{}
	project.ID = testProjectID
	ctx := weaveauth.WithSnapshot(context.Background(), snap)
	ctx = weaveauth.WithProject(ctx, project)
	return ctx
}

func newPublishSvc(store Store, bus domain.EventBus) *Service {
	svc := NewService(
		store,
		&fakeProjectReader{project: func() *domain.Project {
			p := &domain.Project{}
			p.ID = testProjectID
			return p
		}()},
		noopOntologyReader{},
		noopVersionReader{},
		nil, // log → slog.Default
		nil, // runner → noop
	)
	svc.WithEventBus(bus)
	return svc
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestService_Create_PublishesEvent asserts that a successful Create call
// fires ProjectOntologyVersionsChanged with the correct ProjectID.
func TestService_Create_PublishesEvent(t *testing.T) {
	bus := domain.NewSimpleEventBus()

	var received []domain.Event
	bus.Subscribe(domain.EventProjectOntologyVersionsChanged, func(_ context.Context, e domain.Event) {
		received = append(received, e)
	})

	store := newFakeStore()
	svc := newPublishSvc(store, bus)

	_, err := svc.Create(superAdminCtx(), testProjectID, CreateInput{VersionID: testVersionID})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event after Create, got %d", len(received))
	}
	if got := received[0].Type; got != domain.EventProjectOntologyVersionsChanged {
		t.Errorf("event type = %q; want %q", got, domain.EventProjectOntologyVersionsChanged)
	}
	if got := received[0].ProjectID; got != testProjectID {
		t.Errorf("event ProjectID = %q; want %q", got, testProjectID)
	}
}

// TestService_Update_PublishesEvent asserts that a successful Update call
// fires ProjectOntologyVersionsChanged.
func TestService_Update_PublishesEvent(t *testing.T) {
	bus := domain.NewSimpleEventBus()

	var received []domain.Event
	bus.Subscribe(domain.EventProjectOntologyVersionsChanged, func(_ context.Context, e domain.Event) {
		received = append(received, e)
	})

	store := newFakeStore()
	// Pre-populate the store so Get succeeds.
	store.rows[testVersionID] = &domain.ProjectOntologyVersion{
		ProjectID:         testProjectID,
		OntologyVersionID: testVersionID,
		AddedAt:           time.Now(),
	}

	svc := newPublishSvc(store, bus)

	notes := "updated notes"
	_, err := svc.Update(superAdminCtx(), testProjectID, testVersionID, UpdateInput{UsageNotes: &notes})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event after Update, got %d", len(received))
	}
	if got := received[0].ProjectID; got != testProjectID {
		t.Errorf("event ProjectID = %q; want %q", got, testProjectID)
	}
}

// TestService_Delete_PublishesEvent asserts that a successful Delete call
// fires ProjectOntologyVersionsChanged.
func TestService_Delete_PublishesEvent(t *testing.T) {
	bus := domain.NewSimpleEventBus()

	var received []domain.Event
	bus.Subscribe(domain.EventProjectOntologyVersionsChanged, func(_ context.Context, e domain.Event) {
		received = append(received, e)
	})

	store := newFakeStore()
	store.rows[testVersionID] = &domain.ProjectOntologyVersion{
		ProjectID:         testProjectID,
		OntologyVersionID: testVersionID,
		AddedAt:           time.Now(),
	}

	svc := newPublishSvc(store, bus)

	err := svc.Delete(superAdminCtx(), testProjectID, testVersionID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event after Delete, got %d", len(received))
	}
	if got := received[0].ProjectID; got != testProjectID {
		t.Errorf("event ProjectID = %q; want %q", got, testProjectID)
	}
}

// TestService_Create_NoBus_NoPanic asserts that Create does not panic or
// error when no EventBus is wired (the nil-bus guard in
// publishProjectOntologyVersionsChanged).
func TestService_Create_NoBus_NoPanic(t *testing.T) {
	store := newFakeStore()
	svc := NewService(
		store,
		&fakeProjectReader{project: func() *domain.Project {
			p := &domain.Project{}
			p.ID = testProjectID
			return p
		}()},
		noopOntologyReader{},
		noopVersionReader{},
		nil, nil,
	)
	// Deliberately do NOT call WithEventBus.
	_, err := svc.Create(superAdminCtx(), testProjectID, CreateInput{VersionID: testVersionID})
	if err != nil {
		t.Fatalf("Create without bus: %v", err)
	}
}
