package gitrestoreadmin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
)

type Service struct {
	store     Store
	previewer Previewer
	runner    Runner
	log       *slog.Logger
}

type CreateJobInput struct {
	SnapshotPath    string `json:"snapshot_path"`
	TargetProjectID string `json:"target_project_id"`
}

type Runner interface {
	Prepare(rootDir, targetProjectID string) (*gitmaterializer.RestorePlan, error)
	HydrateVendored(ctx context.Context, plan *gitmaterializer.RestorePlan) error
	HydrateShell(ctx context.Context, plan *gitmaterializer.RestorePlan) error
	HydrateEntities(ctx context.Context, plan *gitmaterializer.RestorePlan) error
	// AcquireProjectLock takes the project's exclusive restore advisory
	// lock (gitmaterializer's Materializer.LockProjectForRestore) and
	// returns a release func the caller must call exactly once (both real
	// implementations and both test stubs always return a non-nil
	// closure, but Service.Run nil-checks it anyway before deferring a
	// call to it — this interface is the package's extension point, and
	// that contract otherwise lives only in this comment). Must be called
	// before HydrateVendored — the very first phase, not merely before
	// HydrateOverrides — and held across every phase through
	// HydrateProvenance: HydrateVendored/HydrateShell/HydrateEntities are
	// safe to run unlocked in isolation today only because they're
	// upsert-only and id-preserving, not because they avoid tables a save
	// touches, so that safety is not durable enough to lock around only
	// part of the pipeline. HydrateOverrides and HydrateProvenance take no
	// lock of their own (see gitmaterializer's restore_lock.go): a
	// per-phase lock let a save queued behind the overrides phase slip in
	// and run before the provenance phase started, losing whatever it had
	// just written.
	AcquireProjectLock(ctx context.Context, projectID string) (release func() error, err error)
	HydrateOverrides(ctx context.Context, plan *gitmaterializer.RestorePlan) error
	HydrateProvenance(ctx context.Context, plan *gitmaterializer.RestorePlan) error
}

type materializerRunner struct {
	mat *gitmaterializer.Materializer
}

func NewMaterializerRunner(mat *gitmaterializer.Materializer) Runner {
	return materializerRunner{mat: mat}
}

func (r materializerRunner) Prepare(rootDir, targetProjectID string) (*gitmaterializer.RestorePlan, error) {
	return gitmaterializer.PrepareRestorePlan(rootDir, targetProjectID)
}

func (r materializerRunner) HydrateVendored(ctx context.Context, plan *gitmaterializer.RestorePlan) error {
	return r.mat.HydrateVendored(ctx, plan)
}

func (r materializerRunner) HydrateShell(ctx context.Context, plan *gitmaterializer.RestorePlan) error {
	return r.mat.HydrateProjectShell(ctx, plan)
}

func (r materializerRunner) HydrateEntities(ctx context.Context, plan *gitmaterializer.RestorePlan) error {
	return r.mat.HydrateProjectEntities(ctx, plan)
}

func (r materializerRunner) AcquireProjectLock(ctx context.Context, projectID string) (func() error, error) {
	return r.mat.LockProjectForRestore(ctx, projectID)
}

func (r materializerRunner) HydrateOverrides(ctx context.Context, plan *gitmaterializer.RestorePlan) error {
	return r.mat.HydrateProjectOverrides(ctx, plan)
}

func (r materializerRunner) HydrateProvenance(ctx context.Context, plan *gitmaterializer.RestorePlan) error {
	return r.mat.HydrateProjectProvenance(ctx, plan)
}

// NewService constructs a Service. p defaults to PreviewSnapshot and log
// defaults to slog.Default() when nil, matching NewHandler's own
// nil-tolerant pattern for the same *slog.Logger.
func NewService(store Store, p Previewer, r Runner, log *slog.Logger) *Service {
	if p == nil {
		p = previewerFunc(PreviewSnapshot)
	}
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: store, previewer: p, runner: r, log: log}
}

func (s *Service) Create(ctx context.Context, in CreateJobInput) (*Job, error) {
	principal := weaveauth.PrincipalFromContext(ctx)
	if principal == nil || strings.TrimSpace(principal.ActorID) == "" {
		return nil, errors.New("authentication required")
	}
	snapshotPath := strings.TrimSpace(in.SnapshotPath)
	targetProjectID := strings.TrimSpace(in.TargetProjectID)
	if snapshotPath == "" {
		return nil, &validationError{Fields: map[string][]string{
			"snapshot_path": {"Snapshot path is required."},
		}}
	}
	if targetProjectID == "" {
		return nil, &validationError{Fields: map[string][]string{
			"target_project_id": {"Target project ID is required."},
		}}
	}
	preview, err := s.previewer.Preview(snapshotPath)
	if err != nil {
		return nil, err
	}
	if retargetUnsupported(preview, targetProjectID) {
		return nil, &validationError{Fields: map[string][]string{
			"target_project_id": {"Retarget restore is not supported for snapshots with local project-owned semantic IDs. Restore into the original project ID on an empty target instead."},
		}}
	}
	exists, err := s.store.ProjectExists(ctx, targetProjectID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, &validationError{Fields: map[string][]string{
			"target_project_id": {fmt.Sprintf("Project %s already exists.", targetProjectID)},
		}}
	}
	return s.store.Create(ctx, Job{
		SnapshotPath:    snapshotPath,
		SourceProjectID: preview.ProjectID,
		TargetProjectID: targetProjectID,
		RequestedByID:   principal.ActorID,
		Status:          JobStatusPending,
		CurrentPhase:    "",
		Preview:         preview,
	})
}

func (s *Service) Get(ctx context.Context, id string) (*Job, error) {
	return s.store.Get(ctx, strings.TrimSpace(id))
}

func (s *Service) List(ctx context.Context, limit int) ([]Job, error) {
	return s.store.List(ctx, limit)
}

func (s *Service) Run(ctx context.Context, id string) (*Job, error) {
	job, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	switch job.Status {
	case JobStatusRunning:
		return nil, &conflictError{Message: "restore job is already running"}
	case JobStatusCompleted:
		return nil, &conflictError{Message: "restore job already completed"}
	}

	startedAt := time.Now().UTC()
	if _, err := s.store.UpdateState(ctx, job.ID, JobStatusRunning, RestorePhaseValidate, "", &startedAt, nil); err != nil {
		return nil, err
	}

	exists, err := s.store.ProjectExists(ctx, job.TargetProjectID)
	if err != nil {
		return s.failJob(ctx, job.ID, RestorePhaseValidate, err)
	}
	if exists {
		return s.failJob(ctx, job.ID, RestorePhaseValidate, &validationError{Fields: map[string][]string{
			"target_project_id": {fmt.Sprintf("Project %s already exists.", job.TargetProjectID)},
		}})
	}
	if retargetUnsupported(job.Preview, job.TargetProjectID) {
		return s.failJob(ctx, job.ID, RestorePhaseValidate, &validationError{Fields: map[string][]string{
			"target_project_id": {"Retarget restore is not supported for snapshots with local project-owned semantic IDs. Restore into the original project ID on an empty target instead."},
		}})
	}

	plan, err := s.runner.Prepare(job.SnapshotPath, job.TargetProjectID)
	if err != nil {
		return s.failJob(ctx, job.ID, RestorePhaseValidate, err)
	}

	// Acquired ONCE, as the very first thing after Prepare — before the
	// vendored phase's first write, not merely before overrides — and held
	// across every phase through provenance below (including every
	// job-state bookkeeping write in between). See Runner.AcquireProjectLock's
	// doc comment for why the lock now covers the whole pipeline instead
	// of only overrides+provenance.
	release, err := s.runner.AcquireProjectLock(ctx, job.TargetProjectID)
	if err != nil {
		return s.failJob(ctx, job.ID, RestorePhaseVendored, err)
	}
	defer func() {
		if release == nil {
			return
		}
		if unlockErr := release(); unlockErr != nil {
			s.log.Warn("git restore: release project lock", "project_id", job.TargetProjectID, "err", unlockErr)
		}
	}()

	if _, err := s.store.UpdateState(ctx, job.ID, JobStatusRunning, RestorePhaseVendored, "", nil, nil); err != nil {
		return nil, err
	}
	if err := s.runner.HydrateVendored(ctx, plan); err != nil {
		return s.failJob(ctx, job.ID, RestorePhaseVendored, err)
	}

	if _, err := s.store.UpdateState(ctx, job.ID, JobStatusRunning, RestorePhaseShell, "", nil, nil); err != nil {
		return nil, err
	}
	if err := s.runner.HydrateShell(ctx, plan); err != nil {
		return s.failJob(ctx, job.ID, RestorePhaseShell, err)
	}

	if _, err := s.store.UpdateState(ctx, job.ID, JobStatusRunning, RestorePhaseEntities, "", nil, nil); err != nil {
		return nil, err
	}
	if err := s.runner.HydrateEntities(ctx, plan); err != nil {
		return s.failJob(ctx, job.ID, RestorePhaseEntities, err)
	}

	if _, err := s.store.UpdateState(ctx, job.ID, JobStatusRunning, RestorePhaseOverrides, "", nil, nil); err != nil {
		return nil, err
	}
	if err := s.runner.HydrateOverrides(ctx, plan); err != nil {
		return s.failJob(ctx, job.ID, RestorePhaseOverrides, err)
	}

	if _, err := s.store.UpdateState(ctx, job.ID, JobStatusRunning, RestorePhaseProvenance, "", nil, nil); err != nil {
		return nil, err
	}
	if err := s.runner.HydrateProvenance(ctx, plan); err != nil {
		return s.failJob(ctx, job.ID, RestorePhaseProvenance, err)
	}

	finishedAt := time.Now().UTC()
	return s.store.UpdateState(ctx, job.ID, JobStatusCompleted, "", "", nil, &finishedAt)
}

func (s *Service) failJob(ctx context.Context, jobID, phase string, cause error) (*Job, error) {
	finishedAt := time.Now().UTC()
	job, updateErr := s.store.UpdateState(ctx, jobID, JobStatusFailed, phase, strings.TrimSpace(cause.Error()), nil, &finishedAt)
	if updateErr != nil {
		return nil, updateErr
	}
	return job, cause
}

const (
	RestorePhaseValidate   = "validate"
	RestorePhaseVendored   = "vendored"
	RestorePhaseShell      = "shell"
	RestorePhaseEntities   = "entities"
	RestorePhaseOverrides  = "overrides"
	RestorePhaseProvenance = "provenance"
)

type validationError struct {
	Fields map[string][]string
}

func (e *validationError) Error() string { return "validation error" }
func (e *validationError) ValidationFields() map[string][]string {
	return e.Fields
}

type conflictError struct {
	Message string
}

func (e *conflictError) Error() string { return e.Message }
func (e *conflictError) ConflictMessage() string {
	return e.Message
}

func retargetUnsupported(preview *PreviewResult, targetProjectID string) bool {
	if preview == nil {
		return false
	}
	if strings.TrimSpace(targetProjectID) == "" || strings.TrimSpace(targetProjectID) == strings.TrimSpace(preview.ProjectID) {
		return false
	}
	return preview.Counts.Categories > 0 ||
		preview.Counts.Fields > 0 ||
		preview.Counts.Models > 0 ||
		preview.Counts.Collections > 0
}
