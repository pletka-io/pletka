package gitrestoreadmin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
)

type Service struct {
	store     Store
	previewer Previewer
	runner    Runner
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

func (r materializerRunner) HydrateOverrides(ctx context.Context, plan *gitmaterializer.RestorePlan) error {
	return r.mat.HydrateProjectOverrides(ctx, plan)
}

func (r materializerRunner) HydrateProvenance(ctx context.Context, plan *gitmaterializer.RestorePlan) error {
	return r.mat.HydrateProjectProvenance(ctx, plan)
}

func NewService(store Store, p Previewer, r Runner) *Service {
	if p == nil {
		p = previewerFunc(PreviewSnapshot)
	}
	return &Service{store: store, previewer: p, runner: r}
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
