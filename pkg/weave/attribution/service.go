package attribution

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

// ProjectReader is the narrow read surface this slice needs to gate
// writes on the calling actor's permissions. Implemented by the
// project slice's Service or any equivalent.
type ProjectReader interface {
	GetByID(ctx context.Context, id string) (*domain.Project, error)
}

// Service wraps Store with kind validation, ProjectEdit gating, and
// the reorder dance. Per ADR-0001 the handler depends only on the
// service, not on the store directly.
type Service struct {
	store    Store
	projects ProjectReader
	log      *slog.Logger
}

// NewService wires a Service. nil log → slog.Default.
func NewService(store Store, projects ProjectReader, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: store, projects: projects, log: log}
}

// validKinds enumerates the kinds the schema check constraint allows.
// Add a new kind here AND in the migration's CHECK clause together.
var validKinds = map[string]bool{
	"author":  true,
	"funder":  true,
	"adopter": true,
}

// ValidationError signals a 422-shaped validation failure. The
// handler converts it into the standard `{"errors": {...}}` envelope.
type ValidationError struct {
	Errors map[string][]string
}

func (e *ValidationError) Error() string {
	return "validation error"
}

// ErrNotFound signals that the project the caller asked about does
// not exist. Handler turns it into 404.
var ErrNotFound = errors.New("attribution: project not found")

// ErrForbidden signals the caller lacks ProjectEdit on the project.
var ErrForbidden = errors.New("attribution: forbidden")

// ListForProject returns the project's credit rows in display order.
// Gated on ProjectRead.
func (s *Service) ListForProject(ctx context.Context, projectID string) ([]domain.Attribution, error) {
	if _, err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.ListForProject(ctx, projectID)
}

// ListAllActors returns every actor in the system, scoped via the
// project context so the caller is gated on ProjectEdit before the
// curator can pick anyone to credit.
func (s *Service) ListAllActors(ctx context.Context, projectID string) ([]ActorOption, error) {
	if _, err := s.requireProjectEdit(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.ListAllActors(ctx)
}

// Create inserts an attribution row at the next position within
// (project, kind). Gated on ProjectEdit.
func (s *Service) Create(ctx context.Context, projectID string, in Input) (domain.Attribution, error) {
	if _, err := s.requireProjectEdit(ctx, projectID); err != nil {
		return domain.Attribution{}, err
	}
	in.ActorID = strings.TrimSpace(in.ActorID)
	in.Kind = strings.TrimSpace(in.Kind)
	verrs := map[string][]string{}
	if in.ActorID == "" {
		verrs["actor_id"] = []string{"actor_id is required"}
	}
	if in.Kind == "" {
		verrs["kind"] = []string{"kind is required"}
	} else if !validKinds[in.Kind] {
		verrs["kind"] = []string{"kind must be author, funder, or adopter"}
	}
	if len(verrs) > 0 {
		return domain.Attribution{}, &ValidationError{Errors: verrs}
	}
	return s.store.Create(ctx, projectID, in)
}

// UpdateNote edits one row's freeform note. Actor + kind + position
// are immutable. Gated on ProjectEdit.
func (s *Service) UpdateNote(ctx context.Context, projectID, actorID, kind string, position int, note string) error {
	if _, err := s.requireProjectEdit(ctx, projectID); err != nil {
		return err
	}
	return s.store.UpdateNote(ctx, projectID, actorID, kind, position, note)
}

// Delete removes a single row identified by the composite key.
// Gated on ProjectEdit.
func (s *Service) Delete(ctx context.Context, projectID, actorID, kind string, position int) error {
	if _, err := s.requireProjectEdit(ctx, projectID); err != nil {
		return err
	}
	return s.store.Delete(ctx, projectID, actorID, kind, position)
}

// Reorder reassigns positions within a (project, kind) bucket to
// match actorIDsInOrder. Two-phase to avoid PK collisions: stage all
// rows at negative positions, then write final positions. Gated on
// ProjectEdit.
func (s *Service) Reorder(ctx context.Context, projectID, kind string, actorIDsInOrder []string) error {
	if _, err := s.requireProjectEdit(ctx, projectID); err != nil {
		return err
	}
	if !validKinds[kind] {
		return &ValidationError{Errors: map[string][]string{"kind": {"kind must be author, funder, or adopter"}}}
	}
	rows, err := s.store.ListForProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("list attributions for reorder: %w", err)
	}
	// Phase 1: stage rows in this kind at negative positions in their
	// existing order. Record the staging position by actor ID for the
	// second pass.
	stagedBy := map[string]int{}
	staging := -1
	for _, row := range rows {
		if row.Kind != kind {
			continue
		}
		if err := s.store.SetPosition(ctx, projectID, row.ActorID, kind, row.Position, staging); err != nil {
			return fmt.Errorf("stage attribution position: %w", err)
		}
		stagedBy[row.ActorID] = staging
		staging--
	}
	// Phase 2: write the requested final positions.
	for newPos, actorID := range actorIDsInOrder {
		oldPos, ok := stagedBy[actorID]
		if !ok {
			continue
		}
		if err := s.store.SetPosition(ctx, projectID, actorID, kind, oldPos, newPos); err != nil {
			return fmt.Errorf("finalise attribution position: %w", err)
		}
	}
	return nil
}

// requireProjectRead gates a read call on the snapshot in context.
// Returns the loaded project on success.
func (s *Service) requireProjectRead(ctx context.Context, projectID string) (*domain.Project, error) {
	return s.requireProject(ctx, projectID, auth.ProjectRead)
}

// requireProjectEdit gates a mutating call on the snapshot.
func (s *Service) requireProjectEdit(ctx context.Context, projectID string) (*domain.Project, error) {
	return s.requireProject(ctx, projectID, auth.ProjectEdit)
}

func (s *Service) requireProject(ctx context.Context, projectID string, capability auth.Capability) (*domain.Project, error) {
	project, err := s.projects.GetByID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if project == nil {
		return nil, ErrNotFound
	}
	snap := auth.FromContext(ctx)
	if snap == nil {
		return nil, ErrForbidden
	}
	resource := auth.Resource{ScopeType: "project", ID: project.ID, Visibility: project.Visibility}
	if !snap.Can(capability, resource, nil) {
		return nil, ErrForbidden
	}
	return project, nil
}
