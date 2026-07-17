package project

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/domain"
)

// HierarchyReader is the parent-walk read surface the service needs
// from the legacy WeaveStore aggregate during the strangler-fig
// period. Once the legacy aggregate retires, this is replaced by
// direct slice composition.
//
// LinkedOntologies was removed — the canonical reader for
// "what ontology versions does a project have linked" lives at
// pkg/weave/projectontologyversion.Service.LinkedOntologies. This
// project slice doesn't need it; the projectpage slice consumes the
// canonical reader directly.
type HierarchyReader interface {
	ResolvedOntologyVersions(ctx context.Context, projectID string, opts domain.ResolvedOntologyVersionOpts) ([]domain.ResolvedOntologyVersion, error)
}

// MembershipWriter is the narrow surface Service.Create uses to upsert
// the creator's owner-membership at project create time. Satisfied by
// domain.WeaveStore.Memberships() in production. Optional — pass nil
// in test setups that don't care about membership wiring.
//
// The cleaner long-term shape is a single Service method that creates
// the project + the membership in one transaction; we do best-effort
// here because the existing Store.Create doesn't expose the tx
// boundary. A failed membership upsert logs but doesn't unwind the
// project — the synthesised owner row in the members slice list query
// is the safety net.
type MembershipWriter interface {
	Upsert(ctx context.Context, m domain.Membership) error
}

// Service composes Project read business logic over Store + cross-slice
// reads via HierarchyReader. Currently read-only — write methods will
// land on Service in a follow-up commit when Create/Update/Delete also
// migrate.
type Service struct {
	store     Store
	hierarchy HierarchyReader
	memberWrt MembershipWriter
	log       *slog.Logger
}

// NewService constructs a Service. nil log → slog.Default. hierarchy
// must be non-nil for ResolvedOntologyVersions; pass
// deps.Weave.Projects() during the strangler-fig period. memberWrt may
// be nil in tests; production should pass deps.Weave.Memberships() so
// project creators get an explicit owner-membership row.
func NewService(store Store, hierarchy HierarchyReader, memberWrt MembershipWriter, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: store, hierarchy: hierarchy, memberWrt: memberWrt, log: log}
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

type ErrForbidden struct {
	Capability string
	Resource   string
}

func (e *ErrForbidden) Error() string {
	return fmt.Sprintf("forbidden: requires %s on %s", e.Capability, e.Resource)
}

var errNotFound = errors.New("project: not found")

// IsNotFound reports whether err signals "not found".
func IsNotFound(err error) bool { return errors.Is(err, errNotFound) }

// ErrValidation collects per-field validation errors. Handlers map to
// 422 with the {"errors": {"<field>": ["<msg>"]}} shape FormRenderer
// expects.
type ErrValidation struct {
	Fields map[string][]string
}

func (e *ErrValidation) Error() string { return "project: validation error" }

// ErrConflict signals an attempted Create with an ID prefix that already
// exists. Handlers map to 409 (or 422 with id_prefix in the errors map
// when the form should render the conflict inline).
var ErrConflict = errors.New("project: id_prefix already in use")

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

// List returns projects matching opts plus total count for pagination.
// No auth gate — public list of projects (visibility filtering is per-row
// via the legacy /data endpoint's auth snapshot).
func (s *Service) List(ctx context.Context, opts ...domain.QueryOption) ([]*domain.Project, int64, error) {
	return s.store.List(ctx, opts...)
}

// ListVisible returns only projects readable by the current caller, with
// pagination applied after capability filtering so totals match what the
// caller can actually see.
func (s *Service) ListVisible(ctx context.Context, opts ...domain.QueryOption) ([]*domain.Project, int64, error) {
	cfg := domain.ApplyOptions(opts)

	baseOpts := []domain.QueryOption{
		domain.WithOrderBy(cfg.OrderBy, cfg.OrderDesc),
	}
	if cfg.Search != "" {
		baseOpts = append(baseOpts, domain.WithSearch(cfg.Search))
	}
	for k, v := range cfg.Filters {
		baseOpts = append(baseOpts, domain.WithFilter(k, v))
	}

	projects, _, err := s.store.List(ctx, baseOpts...)
	if err != nil {
		return nil, 0, err
	}

	visible := make([]*domain.Project, 0, len(projects))
	for _, p := range projects {
		if p == nil || !s.CanRead(ctx, p) {
			continue
		}
		visible = append(visible, p)
	}

	total := int64(len(visible))
	if cfg.Offset >= len(visible) {
		return []*domain.Project{}, total, nil
	}
	start := cfg.Offset
	if start < 0 {
		start = 0
	}
	end := len(visible)
	if cfg.Limit > 0 && start+cfg.Limit < end {
		end = start + cfg.Limit
	}
	return slices.Clone(visible[start:end]), total, nil
}

// Get returns a single project, or (nil, nil) when not found. No auth
// gate at this layer; callers (handlers, other slices) gate.
func (s *Service) Get(ctx context.Context, id string) (*domain.Project, error) {
	return s.store.GetByID(ctx, id)
}

// GetByID is the narrow reader alias used by cross-cutting generator services.
func (s *Service) GetByID(ctx context.Context, id string) (*domain.Project, error) {
	return s.Get(ctx, id)
}

// StatsForProjects returns entity counts per project ID.
func (s *Service) StatsForProjects(ctx context.Context, ids []string) (map[string]*domain.WeaveProjectStats, error) {
	return s.store.StatsForProjects(ctx, ids)
}

// OwnersForProjects returns the owning institution per project ID.
func (s *Service) OwnersForProjects(ctx context.Context, ids []string) (map[string]*domain.ProjectActor, error) {
	return s.store.OwnersForProjects(ctx, ids)
}

// ListOwnerInstitutions returns institutions owning at least one project.
func (s *Service) ListOwnerInstitutions(ctx context.Context) ([]*domain.ProjectActor, error) {
	return s.store.ListOwnerInstitutions(ctx)
}

// ListVisibleOwnerInstitutions returns institutions that own at least one
// project visible to the current caller.
func (s *Service) ListVisibleOwnerInstitutions(ctx context.Context) ([]*domain.ProjectActor, error) {
	projects, _, err := s.ListVisible(ctx, domain.WithLimit(1000000))
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(projects))
	for _, p := range projects {
		if p == nil {
			continue
		}
		ids = append(ids, p.ID)
	}
	owners, err := s.OwnersForProjects(ctx, ids)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	out := make([]*domain.ProjectActor, 0, len(owners))
	for _, owner := range owners {
		if owner == nil {
			continue
		}
		if _, ok := seen[owner.ID]; ok {
			continue
		}
		seen[owner.ID] = struct{}{}
		out = append(out, owner)
	}
	slices.SortFunc(out, func(a, b *domain.ProjectActor) int {
		return strings.Compare(a.DisplayName, b.DisplayName)
	})
	return out, nil
}

// ListChildren returns the children of parentID.
func (s *Service) ListChildren(ctx context.Context, parentID string) ([]*domain.Project, error) {
	return s.store.ListChildren(ctx, parentID)
}

// ResolvedOntologyVersions delegates to HierarchyReader. Used by the
// data-API to flag projects as "incomplete" when no ontology is linked
// (own or inherited).
func (s *Service) ResolvedOntologyVersions(ctx context.Context, projectID string, opts domain.ResolvedOntologyVersionOpts) ([]domain.ResolvedOntologyVersion, error) {
	if s.hierarchy == nil {
		return nil, fmt.Errorf("hierarchy reader not configured")
	}
	return s.hierarchy.ResolvedOntologyVersions(ctx, projectID, opts)
}

// LinkedOntologies removed. Canonical reader is
// pkg/weave/projectontologyversion.Service.LinkedOntologies; consume it
// directly from the slice that needs it (projectpage today; future
// slices wire the same reader interface).

// ---------------------------------------------------------------------------
// IDPrefix validation + suggestions
// ---------------------------------------------------------------------------

// idPrefixRegex validates ID prefix format: 2-10 uppercase letters.
var idPrefixRegex = regexp.MustCompile(`^[A-Z]{2,10}$`)

// IsValidIDPrefix reports whether prefix matches the format rules.
func IsValidIDPrefix(prefix string) bool {
	return idPrefixRegex.MatchString(prefix)
}

// PrefixCheck is the result of IDPrefix availability + format check.
type PrefixCheck struct {
	Available       bool     `json:"available"`
	Valid           bool     `json:"valid"`
	Normalized      string   `json:"normalized,omitempty"`
	Message         string   `json:"message"`
	ValidationError bool     `json:"validation_error,omitempty"`
	Suggestions     []string `json:"suggestions,omitempty"`
}

// CheckIDPrefix runs format + availability checks. prefix is normalised
// to upper-case and trimmed before checking.
func (s *Service) CheckIDPrefix(ctx context.Context, prefix string) (*PrefixCheck, error) {
	if prefix == "" {
		return &PrefixCheck{Valid: false, Message: "ID prefix is required"}, nil
	}
	if !IsValidIDPrefix(prefix) {
		return &PrefixCheck{
			Valid:           false,
			Normalized:      prefix,
			Message:         "ID prefix must be 2-10 uppercase letters only",
			ValidationError: true,
		}, nil
	}

	existing, err := s.existingPrefixes(ctx)
	if err != nil {
		return nil, fmt.Errorf("load existing prefixes: %w", err)
	}

	if existing[prefix] {
		return &PrefixCheck{
			Valid:       true,
			Normalized:  prefix,
			Message:     fmt.Sprintf("'%s' is already in use", prefix),
			Suggestions: GenerateIDPrefixSuggestions(prefix, existing),
		}, nil
	}

	return &PrefixCheck{
		Available:  true,
		Valid:      true,
		Normalized: prefix,
		Message:    fmt.Sprintf("'%s' is available", prefix),
	}, nil
}

// existingPrefixes returns the set of in-use IDPrefixes, upper-cased.
func (s *Service) existingPrefixes(ctx context.Context) (map[string]bool, error) {
	projects, _, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(projects))
	for _, p := range projects {
		if p == nil {
			continue
		}
		out[p.ID] = true
	}
	return out, nil
}

// GenerateIDPrefixSuggestions returns up to 3 alternative prefixes that
// aren't in `existing`. Three strategies tried in order: numeric suffix,
// appended letter, prepended letter.
func GenerateIDPrefixSuggestions(prefix string, existing map[string]bool) []string {
	out := make([]string, 0, 3)

	for i := 2; i <= 9 && len(out) < 3; i++ {
		c := fmt.Sprintf("%s%d", prefix, i)
		if len(c) <= 10 && !existing[c] {
			out = append(out, c)
		}
	}

	if len(out) < 3 && len(prefix) < 10 {
		for c := 'A'; c <= 'Z' && len(out) < 3; c++ {
			candidate := prefix + string(c)
			if !existing[candidate] {
				out = append(out, candidate)
			}
		}
	}

	if len(out) < 3 && len(prefix) < 10 {
		for c := 'A'; c <= 'Z' && len(out) < 3; c++ {
			candidate := string(c) + prefix
			if !existing[candidate] {
				out = append(out, candidate)
			}
		}
	}

	return out
}

// ---------------------------------------------------------------------------
// Permissions
// ---------------------------------------------------------------------------

// CanEdit reports whether the caller's auth snapshot allows ProjectEdit
// on the given project. Used by the data-API to decide whether to render
// per-row "needs setup" badges.
func (s *Service) CanEdit(ctx context.Context, projectID, visibility string) bool {
	snap := auth.FromContext(ctx)
	res := auth.Resource{ScopeType: "project", ID: projectID, Visibility: visibility}
	return snap.Can(auth.ProjectEdit, res, nil)
}

// CanRead reports whether the caller's auth snapshot allows ProjectRead for p.
func (s *Service) CanRead(ctx context.Context, p *domain.Project) bool {
	if p == nil {
		return false
	}
	visibility := p.Visibility
	if visibility == "" {
		visibility = "public"
	}
	snap := auth.FromContext(ctx)
	res := auth.Resource{ScopeType: "project", ID: p.ID, OrgID: p.OwnerID, Visibility: visibility}
	return snap.Can(auth.ProjectRead, res, nil)
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

// CreateInput is the payload for Service.Create. IDPrefix is the
// natural key (= weave_projects.id) and is uppercased + validated by
// the service. SystemName is auto-derived from UIName when empty.
type CreateInput struct {
	UIName      domain.Translations
	Description domain.Translations
	IDPrefix    string
	SystemName  string
	OwnerID     string
	CreatedByID string
	Visibility  string // defaults to "private" when empty
}

// Create validates the input, checks ID-prefix collision, and inserts
// a new project row. Returns ErrValidation for malformed input,
// ErrConflict when the prefix is already taken.
//
// The HTTP handler keeps request parsing/auth wrapping and delegates the
// validation + write here. This keeps the slice as the single source of truth
// for project creation.
func (s *Service) Create(ctx context.Context, in CreateInput) (*domain.Project, error) {
	in.IDPrefix = strings.ToUpper(strings.TrimSpace(in.IDPrefix))

	errs := map[string][]string{}
	if len(in.UIName) == 0 {
		errs["ui_name"] = append(errs["ui_name"], "Project name is required")
	}
	if strings.TrimSpace(in.OwnerID) == "" {
		errs["owner_id"] = append(errs["owner_id"], "Owner is required")
	}
	if in.IDPrefix == "" {
		errs["id_prefix"] = append(errs["id_prefix"], "ID prefix is required")
	} else if !IsValidIDPrefix(in.IDPrefix) {
		errs["id_prefix"] = append(errs["id_prefix"], "ID prefix must be 2-10 uppercase letters only")
	}

	systemName := in.SystemName
	if systemName == "" {
		systemName = strings.ToLower(strings.ReplaceAll(in.UIName.Get("en", "project"), " ", "-"))
	}

	if len(errs) == 0 {
		existing, err := s.store.GetByID(ctx, in.IDPrefix)
		if err != nil {
			return nil, fmt.Errorf("check existing project: %w", err)
		}
		if existing != nil {
			errs["id_prefix"] = append(errs["id_prefix"], fmt.Sprintf("ID prefix %q is already in use", in.IDPrefix))
		}
	}

	if len(errs) > 0 {
		return nil, &ErrValidation{Fields: errs}
	}

	// New projects default to private — owner opts in to public via
	// the Settings → Visibility form once they're ready to share.
	// Migrated weaves (the importer) opt into public explicitly.
	visibility := in.Visibility
	if visibility == "" {
		visibility = "private"
	}

	project := &domain.Project{
		Entity: domain.Entity{
			ID:          in.IDPrefix,
			SemanticID:  in.IDPrefix,
			SystemName:  systemName,
			UIName:      in.UIName,
			Description: in.Description,
			Status:      domain.StatusDraft,
		},
		OwnerID:     in.OwnerID,
		CreatedByID: dbutil.TrimEmptyToNil(in.CreatedByID),
		Visibility:  visibility,
	}
	if err := s.store.Create(ctx, project); err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}

	// Best-effort owner-membership upsert. The project is canonical;
	// failure here doesn't unwind the project create, just gets logged.
	// The members slice list query also synthesises an owner row when
	// the explicit membership is missing, so the user-visible state
	// stays correct either way.
	//
	// TODO(import-v2): the importer should populate owner_id
	// from the source creator field and write the matching
	// membership row at import time, instead of leaving imported
	// projects relying on the synthesised fallback. Tracked separately.
	if s.memberWrt != nil && in.OwnerID != "" {
		if err := s.memberWrt.Upsert(ctx, domain.Membership{
			ActorID:   in.OwnerID,
			ScopeType: "project",
			ScopeID:   project.ID,
			Role:      "owner",
		}); err != nil {
			s.log.Warn("failed to upsert owner membership", "project_id", project.ID, "actor_id", in.OwnerID, "err", err)
		}
	}

	return project, nil
}
