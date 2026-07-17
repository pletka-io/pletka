package orgmembers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

type Service struct {
	store Store
	log   *slog.Logger
}

func NewService(store Store, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: store, log: log}
}

type ErrForbidden struct {
	Capability string
	Resource   string
}

func (e *ErrForbidden) Error() string {
	return fmt.Sprintf("forbidden: requires %s on %s", e.Capability, e.Resource)
}

type ErrValidation struct {
	Fields map[string][]string
}

func (e *ErrValidation) Error() string { return "orgmembers: validation error" }

var errNotFound = errors.New("orgmembers: not found")

func IsNotFound(err error) bool { return errors.Is(err, errNotFound) }

var validRoles = map[string]bool{
	"member": true,
	"admin":  true,
	"owner":  true,
}

func (s *Service) List(ctx context.Context, orgID string) ([]MemberRow, error) {
	if err := s.requireOrgRead(ctx); err != nil {
		return nil, err
	}
	return s.store.ListByOrg(ctx, orgID)
}

func (s *Service) Get(ctx context.Context, orgID, actorID string) (*MemberRow, error) {
	if err := s.requireOrgRead(ctx); err != nil {
		return nil, err
	}
	rows, err := s.store.ListByOrg(ctx, orgID)
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

type AddInput struct {
	ActorID     string `json:"actor_id"`
	EmailOrSlug string `json:"email_or_slug,omitempty"`
	Role        string `json:"role"`
}

func (s *Service) Add(ctx context.Context, orgID string, in AddInput) (*MemberRow, error) {
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
		errs["role"] = []string{fmt.Sprintf("Unknown role %q (expected member / admin / owner)", in.Role)}
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
		ScopeType: "org",
		ScopeID:   orgID,
		Role:      in.Role,
	}); err != nil {
		return nil, fmt.Errorf("upsert membership: %w", err)
	}

	out := *actor
	out.Role = in.Role
	out.IsOwner = in.Role == "owner"
	return &out, nil
}

func (s *Service) ListAvailableActors(ctx context.Context, orgID string) ([]MemberRow, error) {
	if err := s.requireOrgRead(ctx); err != nil {
		return nil, err
	}
	return s.store.ListAvailableActors(ctx, orgID)
}

type UpdateInput struct {
	Role string `json:"role"`
}

func (s *Service) UpdateRole(ctx context.Context, orgID, actorID string, in UpdateInput) (*MemberRow, error) {
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

	existing, err := s.Get(ctx, orgID, actorID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, errNotFound
	}
	rows, err := s.store.ListByOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if existing.Role == "owner" && role != "owner" && ownerCount(rows) <= 1 {
		return nil, &ErrValidation{Fields: map[string][]string{
			"role": {"cannot remove the last organization owner"},
		}}
	}

	if err := s.store.Upsert(ctx, domain.Membership{
		ActorID:   actorID,
		ScopeType: "org",
		ScopeID:   orgID,
		Role:      role,
	}); err != nil {
		return nil, fmt.Errorf("upsert membership: %w", err)
	}

	rows, err = s.store.ListByOrg(ctx, orgID)
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

func (s *Service) Remove(ctx context.Context, orgID, actorID string) error {
	if err := s.requireManageMembers(ctx); err != nil {
		return err
	}
	member, err := s.Get(ctx, orgID, actorID)
	if err != nil {
		return err
	}
	if member == nil {
		return errNotFound
	}
	rows, err := s.store.ListByOrg(ctx, orgID)
	if err != nil {
		return err
	}
	if member.Role == "owner" && ownerCount(rows) <= 1 {
		return &ErrValidation{Fields: map[string][]string{
			"actor_id": {"cannot remove the last organization owner"},
		}}
	}
	return s.store.Delete(ctx, orgID, actorID)
}

func ownerCount(rows []MemberRow) int {
	count := 0
	for _, row := range rows {
		if row.Role == "owner" {
			count++
		}
	}
	return count
}

func (s *Service) requireOrgRead(ctx context.Context) error {
	snap := weaveauth.FromContext(ctx)
	res := weaveauth.OrgResourceFromContext(ctx)
	if !snap.Can(weaveauth.OrgRead, res, nil) {
		return &ErrForbidden{Capability: string(weaveauth.OrgRead), Resource: "org:" + res.ID}
	}
	return nil
}

func (s *Service) requireManageMembers(ctx context.Context) error {
	snap := weaveauth.FromContext(ctx)
	res := weaveauth.OrgResourceFromContext(ctx)
	if !snap.Can(weaveauth.OrgManageMembers, res, nil) {
		return &ErrForbidden{Capability: string(weaveauth.OrgManageMembers), Resource: "org:" + res.ID}
	}
	return nil
}
