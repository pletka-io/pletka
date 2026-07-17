package members

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

// Service composes member business logic over Store + auth gates.
type Service struct {
	store Store
	log   *slog.Logger
}

// NewService constructs a Service. nil log → slog.Default.
func NewService(store Store, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: store, log: log}
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// ErrForbidden signals the caller doesn't hold the required capability
// on the project.
type ErrForbidden struct {
	Capability string
	Resource   string
}

func (e *ErrForbidden) Error() string {
	return fmt.Sprintf("forbidden: requires %s on %s", e.Capability, e.Resource)
}

// ErrValidation collects per-field validation errors, mapped to 422 by
// the handler with the FormRenderer-compatible {"errors": {...}} shape.
type ErrValidation struct {
	Fields map[string][]string
}

func (e *ErrValidation) Error() string { return "members: validation error" }

// errNotFound signals the membership row didn't exist.
var errNotFound = errors.New("members: not found")

func IsNotFound(err error) bool { return errors.Is(err, errNotFound) }

// validRoles are the project-scope role names accepted by Add / Update.
var validRoles = map[string]bool{
	"viewer":      true,
	"contributor": true,
	"maintainer":  true,
	"owner":       true,
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

// List returns all members of a project. Read gate.
func (s *Service) List(ctx context.Context, projectID string) ([]MemberRow, error) {
	if err := s.requireProjectRead(ctx); err != nil {
		return nil, err
	}
	return s.store.ListByProject(ctx, projectID)
}

// Get returns a single member row by actor ID, or (nil, nil) when no
// matching membership exists. Used to prefill the edit form.
func (s *Service) Get(ctx context.Context, projectID, actorID string) (*MemberRow, error) {
	if err := s.requireProjectRead(ctx); err != nil {
		return nil, err
	}
	rows, err := s.store.ListByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.ActorID == actorID {
			row := r
			return &row, nil
		}
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// Writes
// ---------------------------------------------------------------------------

// AddInput is the payload accepted by Service.Add. The Add Member form
// posts the chosen actor's ULID; EmailOrSlug remains as a fallback for
// legacy callers / API users who only have an email or slug handle.
type AddInput struct {
	ActorID     string `json:"actor_id"`
	EmailOrSlug string `json:"email_or_slug,omitempty"`
	Role        string `json:"role"`
}

// Add resolves the actor (by ULID, then by email/slug) and upserts a
// project membership. ErrValidation when input is malformed or the
// actor can't be found; ErrForbidden when the caller lacks
// project.manage_members.
func (s *Service) Add(ctx context.Context, projectID string, in AddInput) (*MemberRow, error) {
	if err := s.requireManageMembers(ctx); err != nil {
		return nil, err
	}

	in.ActorID = strings.TrimSpace(in.ActorID)
	in.EmailOrSlug = strings.TrimSpace(in.EmailOrSlug)
	in.Role = strings.TrimSpace(strings.ToLower(in.Role))

	errs := map[string][]string{}
	if in.ActorID == "" && in.EmailOrSlug == "" {
		errs["actor_id"] = []string{"Select a user"}
	}
	if in.Role == "" {
		errs["role"] = []string{"Role is required"}
	} else if !validRoles[in.Role] {
		errs["role"] = []string{fmt.Sprintf("Unknown role %q (expected viewer / contributor / maintainer / owner)", in.Role)}
	}
	if len(errs) > 0 {
		return nil, &ErrValidation{Fields: errs}
	}

	var (
		actor *MemberRow
		err   error
	)
	if in.ActorID != "" {
		actor, err = s.store.GetActorByID(ctx, in.ActorID)
		if err != nil {
			return nil, fmt.Errorf("get actor: %w", err)
		}
		if actor == nil {
			return nil, &ErrValidation{Fields: map[string][]string{
				"actor_id": {"Selected user no longer exists"},
			}}
		}
	} else {
		actor, err = s.store.FindActorByEmailOrSlug(ctx, in.EmailOrSlug)
		if err != nil {
			return nil, fmt.Errorf("find actor: %w", err)
		}
		if actor == nil {
			return nil, &ErrValidation{Fields: map[string][]string{
				"email_or_slug": {fmt.Sprintf("No user found for %q", in.EmailOrSlug)},
			}}
		}
	}

	if err := s.store.Upsert(ctx, domain.Membership{
		ActorID:   actor.ActorID,
		ScopeType: "project",
		ScopeID:   projectID,
		Role:      in.Role,
	}); err != nil {
		return nil, fmt.Errorf("upsert membership: %w", err)
	}

	out := *actor
	out.Role = in.Role
	return &out, nil
}

// ListAvailableActors returns actors who can be added as members of
// projectID (i.e. not yet members and not the owner). Read gate: same
// as List — caller must be able to read the project.
func (s *Service) ListAvailableActors(ctx context.Context, projectID string) ([]MemberRow, error) {
	if err := s.requireProjectRead(ctx); err != nil {
		return nil, err
	}
	return s.store.ListAvailableActors(ctx, projectID)
}

// UpdateInput is the payload accepted by Service.UpdateRole.
type UpdateInput struct {
	Role string `json:"role"`
}

// UpdateRole changes an existing membership's role. ErrValidation on
// unknown role; ErrForbidden when caller lacks project.manage_members.
func (s *Service) UpdateRole(ctx context.Context, projectID, actorID string, in UpdateInput) (*MemberRow, error) {
	if err := s.requireManageMembers(ctx); err != nil {
		return nil, err
	}
	role := strings.TrimSpace(strings.ToLower(in.Role))
	if role == "" {
		return nil, &ErrValidation{Fields: map[string][]string{"role": {"Role is required"}}}
	}
	if !validRoles[role] {
		return nil, &ErrValidation{Fields: map[string][]string{
			"role": {fmt.Sprintf("Unknown role %q", role)},
		}}
	}

	if err := s.store.Upsert(ctx, domain.Membership{
		ActorID:   actorID,
		ScopeType: "project",
		ScopeID:   projectID,
		Role:      role,
	}); err != nil {
		return nil, fmt.Errorf("upsert membership: %w", err)
	}

	// Re-list to find the actor + display info; cheap (small N).
	rows, err := s.store.ListByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.ActorID == actorID {
			row := r
			return &row, nil
		}
	}
	return nil, errNotFound
}

// Remove drops a project membership. ErrForbidden when the caller
// lacks project.manage_members. Removing the project owner is rejected
// at the service layer: the frontend hides the Remove button on owner
// rows, and this is the server-side guarantee that a crafted DELETE
// can't strip a project of its creator while owner_id still points
// at them.
func (s *Service) Remove(ctx context.Context, projectID, actorID string) error {
	if err := s.requireManageMembers(ctx); err != nil {
		return err
	}
	ownerID, err := s.store.ProjectOwnerID(ctx, projectID)
	if err != nil {
		return fmt.Errorf("check project owner: %w", err)
	}
	if ownerID != "" && ownerID == actorID {
		return &ErrValidation{Fields: map[string][]string{
			"actor_id": {"cannot remove the project owner; transfer ownership first"},
		}}
	}
	return s.store.Delete(ctx, projectID, actorID)
}

// ---------------------------------------------------------------------------
// Auth gates
// ---------------------------------------------------------------------------

func (s *Service) requireProjectRead(ctx context.Context) error {
	snap := auth.FromContext(ctx)
	res := auth.ProjectResourceFromContext(ctx)
	if !snap.Can(auth.ProjectRead, res, nil) {
		return &ErrForbidden{Capability: string(auth.ProjectRead), Resource: "project:" + res.ID}
	}
	return nil
}

func (s *Service) requireManageMembers(ctx context.Context) error {
	snap := auth.FromContext(ctx)
	res := auth.ProjectResourceFromContext(ctx)
	if !snap.Can(auth.ProjectManageMembers, res, nil) {
		return &ErrForbidden{Capability: string(auth.ProjectManageMembers), Resource: "project:" + res.ID}
	}
	return nil
}
