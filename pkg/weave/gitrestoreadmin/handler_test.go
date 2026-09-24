package gitrestoreadmin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	"github.com/go-chi/chi/v5"
)

func TestHandlerPreview(t *testing.T) {
	h := NewHandler(nil, NewService(&stubStore{}, previewerFunc(func(rootDir string) (*PreviewResult, error) {
		if rootDir != "/tmp/snapshot" {
			t.Fatalf("unexpected rootDir %q", rootDir)
		}
		return &PreviewResult{
			ProjectID:   "TPC",
			ModulePath:  "pletka.io/orgs/test-org/projects/TPC",
			HasLockfile: true,
		}, nil
	}), stubRunner{}, nil))

	req := httptest.NewRequest(http.MethodPost, "/admin/git-restore/preview", bytes.NewBufferString(`{"snapshot_path":"/tmp/snapshot"}`))
	rec := httptest.NewRecorder()

	h.Preview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var got PreviewResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.ProjectID != "TPC" || got.ModulePath == "" || !got.HasLockfile {
		t.Fatalf("unexpected preview: %#v", got)
	}
}

func TestHandlerPreviewValidation(t *testing.T) {
	h := NewHandler(nil, NewService(&stubStore{}, nil, stubRunner{}, nil))

	req := httptest.NewRequest(http.MethodPost, "/admin/git-restore/preview", bytes.NewBufferString(`{"snapshot_path":"   "}`))
	rec := httptest.NewRecorder()

	h.Preview(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlerPreviewError(t *testing.T) {
	h := NewHandler(nil, NewService(&stubStore{}, previewerFunc(func(rootDir string) (*PreviewResult, error) {
		return nil, errors.New("load project manifest: file not found")
	}), stubRunner{}, nil))

	req := httptest.NewRequest(http.MethodPost, "/admin/git-restore/preview", bytes.NewBufferString(`{"snapshot_path":"/tmp/missing"}`))
	rec := httptest.NewRecorder()

	h.Preview(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlerCreateJob(t *testing.T) {
	store := &stubStore{}
	h := NewHandler(nil, NewService(store, previewerFunc(func(rootDir string) (*PreviewResult, error) {
		return &PreviewResult{ProjectID: "TPC"}, nil
	}), stubRunner{}, nil))
	req := httptest.NewRequest(http.MethodPost, "/admin/git-restore/jobs", bytes.NewBufferString(`{"snapshot_path":"/tmp/snapshot","target_project_id":"TPC"}`))
	req = req.WithContext(weaveauth.WithPrincipal(req.Context(), &weaveauth.Principal{ActorID: "actor-1"}))
	rec := httptest.NewRecorder()

	h.CreateJob(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	if store.created == nil || store.created.RequestedByID != "actor-1" || store.created.TargetProjectID != "TPC" {
		t.Fatalf("unexpected created job: %#v", store.created)
	}
}

func TestHandlerListJobs(t *testing.T) {
	store := &stubStore{
		list: []Job{{ID: "job-1", Status: JobStatusPending}},
	}
	h := NewHandler(nil, NewService(store, nil, stubRunner{}, nil))
	req := httptest.NewRequest(http.MethodGet, "/admin/git-restore/jobs?limit=10", nil)
	rec := httptest.NewRecorder()

	h.ListJobs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlerGetJob(t *testing.T) {
	store := &stubStore{
		get: &Job{ID: "job-1", Status: JobStatusPending},
	}
	h := NewHandler(nil, NewService(store, nil, stubRunner{}, nil))
	req := httptest.NewRequest(http.MethodGet, "/admin/git-restore/jobs/job-1", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("jobID", "job-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	h.GetJob(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlerRunJob(t *testing.T) {
	store := &stubStore{
		get: &Job{ID: "job-1", SnapshotPath: "/tmp/snapshot", TargetProjectID: "TPC", Status: JobStatusPending, Preview: &PreviewResult{ProjectID: "TPC"}},
	}
	h := NewHandler(nil, NewService(store, previewerFunc(func(rootDir string) (*PreviewResult, error) {
		return &PreviewResult{ProjectID: "TPC"}, nil
	}), stubRunner{}, nil))
	req := httptest.NewRequest(http.MethodPost, "/admin/git-restore/jobs/job-1/run", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("jobID", "job-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	h.RunJob(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

type stubStore struct {
	created *Job
	get     *Job
	list    []Job
	exists  bool
}

func (s *stubStore) Create(ctx context.Context, job Job) (*Job, error) {
	s.created = &job
	job.ID = "job-1"
	return &job, nil
}

func (s *stubStore) Get(ctx context.Context, id string) (*Job, error) {
	if s.get == nil {
		return nil, ErrJobNotFound
	}
	return s.get, nil
}

func (s *stubStore) List(ctx context.Context, limit int) ([]Job, error) {
	return s.list, nil
}

func (s *stubStore) ProjectExists(ctx context.Context, projectID string) (bool, error) {
	return s.exists, nil
}

func (s *stubStore) UpdateState(ctx context.Context, id string, status JobStatus, currentPhase, errorMessage string, startedAt, finishedAt *time.Time) (*Job, error) {
	if s.get == nil {
		s.get = &Job{ID: id}
	}
	s.get.Status = status
	s.get.CurrentPhase = currentPhase
	s.get.ErrorMessage = errorMessage
	if startedAt != nil {
		s.get.StartedAt = startedAt
	}
	if finishedAt != nil {
		s.get.FinishedAt = finishedAt
	}
	return s.get, nil
}

type stubRunner struct{}

func (stubRunner) Prepare(rootDir, targetProjectID string) (*gitmaterializer.RestorePlan, error) {
	return &gitmaterializer.RestorePlan{ProjectID: targetProjectID}, nil
}
func (stubRunner) HydrateVendored(ctx context.Context, plan *gitmaterializer.RestorePlan) error {
	return nil
}
func (stubRunner) HydrateShell(ctx context.Context, plan *gitmaterializer.RestorePlan) error {
	return nil
}
func (stubRunner) HydrateEntities(ctx context.Context, plan *gitmaterializer.RestorePlan) error {
	return nil
}
func (stubRunner) AcquireProjectLock(ctx context.Context, projectID string) (func() error, error) {
	return func() error { return nil }, nil
}
func (stubRunner) HydrateOverrides(ctx context.Context, plan *gitmaterializer.RestorePlan) error {
	return nil
}
func (stubRunner) HydrateProvenance(ctx context.Context, plan *gitmaterializer.RestorePlan) error {
	return nil
}
