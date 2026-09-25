package gitrestoreadmin

import (
	"context"
	"errors"
	"testing"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
)

func TestServiceCreate(t *testing.T) {
	store := &stubStore{}
	svc := NewService(store, previewerFunc(func(rootDir string) (*PreviewResult, error) {
		if rootDir != "/tmp/snapshot" {
			t.Fatalf("unexpected rootDir %q", rootDir)
		}
		return &PreviewResult{ProjectID: "TPC"}, nil
	}), stubRunner{}, nil)
	ctx := weaveauth.WithPrincipal(context.Background(), &weaveauth.Principal{ActorID: "actor-1"})

	job, err := svc.Create(ctx, CreateJobInput{
		SnapshotPath:    "/tmp/snapshot",
		TargetProjectID: "TPC",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if job.ID != "job-1" || store.created == nil {
		t.Fatalf("expected persisted job, got %#v", job)
	}
	if store.created.SourceProjectID != "TPC" || store.created.RequestedByID != "actor-1" {
		t.Fatalf("unexpected created job: %#v", store.created)
	}
}

func TestServiceCreate_ProjectExists(t *testing.T) {
	store := &stubStore{exists: true}
	svc := NewService(store, previewerFunc(func(rootDir string) (*PreviewResult, error) {
		return &PreviewResult{ProjectID: "TPC"}, nil
	}), stubRunner{}, nil)
	ctx := weaveauth.WithPrincipal(context.Background(), &weaveauth.Principal{ActorID: "actor-1"})

	_, err := svc.Create(ctx, CreateJobInput{
		SnapshotPath:    "/tmp/snapshot",
		TargetProjectID: "TPC",
	})
	var ve interface{ ValidationFields() map[string][]string }
	if !errors.As(err, &ve) {
		t.Fatalf("expected validation error, got %v", err)
	}
	if fields := ve.ValidationFields(); len(fields["target_project_id"]) == 0 {
		t.Fatalf("expected target_project_id validation, got %#v", fields)
	}
}

func TestServiceCreate_RequiresTarget(t *testing.T) {
	store := &stubStore{}
	svc := NewService(store, previewerFunc(func(rootDir string) (*PreviewResult, error) {
		return &PreviewResult{ProjectID: "TPC"}, nil
	}), stubRunner{}, nil)
	ctx := weaveauth.WithPrincipal(context.Background(), &weaveauth.Principal{ActorID: "actor-1"})

	_, err := svc.Create(ctx, CreateJobInput{SnapshotPath: "/tmp/snapshot"})
	var ve interface{ ValidationFields() map[string][]string }
	if !errors.As(err, &ve) {
		t.Fatalf("expected validation error, got %v", err)
	}
	if fields := ve.ValidationFields(); len(fields["target_project_id"]) == 0 {
		t.Fatalf("expected target_project_id validation, got %#v", fields)
	}
}

func TestServiceRun(t *testing.T) {
	store := &stubStore{
		get: &Job{ID: "job-1", SnapshotPath: "/tmp/snapshot", TargetProjectID: "TPC", Status: JobStatusPending, Preview: &PreviewResult{ProjectID: "TPC"}},
	}
	svc := NewService(store, nil, stubRunner{}, nil)

	job, err := svc.Run(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if job.Status != JobStatusCompleted {
		t.Fatalf("expected completed status, got %#v", job)
	}
	if job.FinishedAt == nil {
		t.Fatalf("expected finished timestamp, got %#v", job)
	}
}

// orderRecordingRunner is a Runner spy that records the order in which its
// hydration phases are invoked, so tests can assert Service.Run drives the
// vendored phase before shell/entities/overrides/provenance, and that the
// project lock brackets the WHOLE pipeline: acquired right after Prepare
// (before vendored, not merely before overrides), released only once
// provenance has run.
type orderRecordingRunner struct {
	calls *[]string
}

func (r orderRecordingRunner) Prepare(rootDir, targetProjectID string) (*gitmaterializer.RestorePlan, error) {
	*r.calls = append(*r.calls, "prepare")
	return &gitmaterializer.RestorePlan{ProjectID: targetProjectID}, nil
}
func (r orderRecordingRunner) HydrateVendored(ctx context.Context, plan *gitmaterializer.RestorePlan) error {
	*r.calls = append(*r.calls, "vendored")
	return nil
}
func (r orderRecordingRunner) HydrateShell(ctx context.Context, plan *gitmaterializer.RestorePlan) error {
	*r.calls = append(*r.calls, "shell")
	return nil
}
func (r orderRecordingRunner) HydrateEntities(ctx context.Context, plan *gitmaterializer.RestorePlan) error {
	*r.calls = append(*r.calls, "entities")
	return nil
}
func (r orderRecordingRunner) AcquireProjectLock(ctx context.Context, projectID string) (func() error, error) {
	*r.calls = append(*r.calls, "lock")
	return func() error {
		*r.calls = append(*r.calls, "unlock")
		return nil
	}, nil
}
func (r orderRecordingRunner) HydrateOverrides(ctx context.Context, plan *gitmaterializer.RestorePlan) error {
	*r.calls = append(*r.calls, "overrides")
	return nil
}
func (r orderRecordingRunner) HydrateProvenance(ctx context.Context, plan *gitmaterializer.RestorePlan) error {
	*r.calls = append(*r.calls, "provenance")
	return nil
}

func TestServiceRun_HydratesVendoredBeforeShell(t *testing.T) {
	store := &stubStore{
		get: &Job{ID: "job-1", SnapshotPath: "/tmp/snapshot", TargetProjectID: "TPC", Status: JobStatusPending, Preview: &PreviewResult{ProjectID: "TPC"}},
	}
	var calls []string
	svc := NewService(store, nil, orderRecordingRunner{calls: &calls}, nil)

	job, err := svc.Run(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if job.Status != JobStatusCompleted {
		t.Fatalf("expected completed status, got %#v", job)
	}

	want := []string{"prepare", "lock", "vendored", "shell", "entities", "overrides", "provenance", "unlock"}
	if len(calls) != len(want) {
		t.Fatalf("expected calls %v, got %v", want, calls)
	}
	for i, phase := range want {
		if calls[i] != phase {
			t.Fatalf("expected calls %v, got %v", want, calls)
		}
	}
}

// nilReleaseRunner is stubRunner with AcquireProjectLock returning a nil
// release closure and a nil error — a buggy-but-plausible Runner
// implementation (the interface's own doc comment is the only place the
// "never nil in practice" contract lives). Service.Run must tolerate this
// rather than panic calling a nil func.
type nilReleaseRunner struct {
	stubRunner
}

func (nilReleaseRunner) AcquireProjectLock(ctx context.Context, projectID string) (func() error, error) {
	return nil, nil
}

func TestServiceRunToleratesNilReleaseClosure(t *testing.T) {
	store := &stubStore{
		get: &Job{ID: "job-1", SnapshotPath: "/tmp/snapshot", TargetProjectID: "TPC", Status: JobStatusPending, Preview: &PreviewResult{ProjectID: "TPC"}},
	}
	svc := NewService(store, nil, nilReleaseRunner{}, nil)

	job, err := svc.Run(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if job.Status != JobStatusCompleted {
		t.Fatalf("expected completed status, got %#v", job)
	}
}

func TestServiceCreate_BlocksUnsafeRetarget(t *testing.T) {
	store := &stubStore{}
	svc := NewService(store, previewerFunc(func(rootDir string) (*PreviewResult, error) {
		return &PreviewResult{
			ProjectID: "TPC",
			Counts:    PreviewCounts{Models: 1},
		}, nil
	}), stubRunner{}, nil)
	ctx := weaveauth.WithPrincipal(context.Background(), &weaveauth.Principal{ActorID: "actor-1"})

	_, err := svc.Create(ctx, CreateJobInput{
		SnapshotPath:    "/tmp/snapshot",
		TargetProjectID: "TPC_COPY",
	})
	var ve interface{ ValidationFields() map[string][]string }
	if !errors.As(err, &ve) {
		t.Fatalf("expected validation error, got %v", err)
	}
	if fields := ve.ValidationFields(); len(fields["target_project_id"]) == 0 {
		t.Fatalf("expected target_project_id validation, got %#v", fields)
	}
}
