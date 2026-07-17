package namespacebinding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

// Service composes namespace-binding business logic over a Store. Handlers
// depend on *Service. Same shape as pkg/weave/category.Service.
type Service struct {
	store  Store
	log    *slog.Logger
	runner domain.ChangeLogRunner
}

type versionedNamespaceBindingReader interface {
	ListVersion(ctx context.Context, projectID, releaseVersion string) ([]*domain.NamespaceBinding, error)
}

// NewService constructs a Service backed by store. Pass nil for log to use
// slog.Default; pass nil for runner to use the noop changelog runner
// (entries dropped silently).
func NewService(store Store, log *slog.Logger, runner domain.ChangeLogRunner) *Service {
	if log == nil {
		log = slog.Default()
	}
	if runner == nil {
		runner = domain.NoopChangeLogRunner()
	}
	return &Service{store: store, log: log, runner: runner}
}

// ---------------------------------------------------------------------------
// Inputs / errors
// ---------------------------------------------------------------------------

// CreateInput is the shape accepted by Create. ProjectID is supplied by the
// handler from the URL, not the request body.
type CreateInput struct {
	Prefix    string
	Namespace string
	Weight    int64 // defaults to 10 when zero
}

// UpdateInput captures a partial update. Nil = leave alone.
type UpdateInput struct {
	Prefix    *string
	Namespace *string
	Weight    *int64
}

// ErrForbidden — caller lacks the required capability.
type ErrForbidden struct {
	Capability string
	Resource   string
}

func (e *ErrForbidden) Error() string {
	return fmt.Sprintf("forbidden: requires %s on %s", e.Capability, e.Resource)
}

// ErrValidation collects per-field validation errors. Maps to 422 with the
// FormRenderer-compatible {"errors": {field: [msg]}} shape.
type ErrValidation struct {
	Fields map[string][]string
}

func (e *ErrValidation) Error() string {
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	return "validation: " + strings.Join(keys, ", ")
}

// errNotFound is the slice-local sentinel for missing rows.
var errNotFound = errors.New("namespace binding not found")

// IsNotFound reports whether err signals "not found".
func IsNotFound(err error) bool { return errors.Is(err, errNotFound) }

type GlobalStatsReport struct {
	NamespaceBinding *domain.NamespaceBinding `json:"namespace_binding"`
	ProjectUsages    []ProjectUsageReport     `json:"project_usages"`
	ProjectCount     int                      `json:"project_count"`
	TotalUsage       int64                    `json:"total_usage"`
}

type ProjectUsageReport struct {
	ProjectID       string `json:"project_id"`
	FieldCount      int64  `json:"field_count"`
	ModelCount      int64  `json:"model_count"`
	CollectionCount int64  `json:"collection_count"`
	TotalCount      int64  `json:"total_count"`
}

type GlobalListInput struct {
	Search  string
	Source  string
	SortBy  string
	Page    int
	PerPage int
}

type GlobalListResult struct {
	Items []*domain.NamespaceBinding
	Total int
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

// List returns global + project-scoped bindings, mixed together as the UI
// renders them. Caller must have ProjectRead.
func (s *Service) List(ctx context.Context, projectID string) ([]*domain.NamespaceBinding, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		if vr, ok := s.store.(versionedNamespaceBindingReader); ok {
			return vr.ListVersion(ctx, projectID, version)
		}
	}
	return s.store.List(ctx, projectID)
}

// ListForProject is the narrow reader method used by cross-cutting generator
// services. It aliases List so the slice remains the namespace source of truth.
func (s *Service) ListForProject(ctx context.Context, projectID string) ([]*domain.NamespaceBinding, error) {
	return s.List(ctx, projectID)
}

// Get returns a single user-owned binding scoped to projectID, or
// errNotFound when missing / read-only / wrong project.
func (s *Service) Get(ctx context.Context, projectID, id string) (*domain.NamespaceBinding, error) {
	if err := s.requireProjectRead(ctx, projectID); err != nil {
		return nil, err
	}
	b, err := s.store.Get(ctx, projectID, id)
	if err != nil {
		if errors.Is(err, domain.ErrReadOnly) {
			return nil, err
		}
		// "not found" / "not owned" → map to errNotFound.
		return nil, errNotFound
	}
	return b, nil
}

func (s *Service) ListGlobal(ctx context.Context) ([]*domain.NamespaceBinding, error) {
	if err := s.requireSuperAdmin(ctx); err != nil {
		return nil, err
	}
	return s.store.ListGlobal(ctx)
}

func (s *Service) GetGlobal(ctx context.Context, id string) (*domain.NamespaceBinding, error) {
	if err := s.requireSuperAdmin(ctx); err != nil {
		return nil, err
	}
	b, err := s.store.GetGlobal(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrReadOnly) {
			return nil, err
		}
		return nil, errNotFound
	}
	return b, nil
}

func (s *Service) ListGlobalBrowse(ctx context.Context, in GlobalListInput) (*GlobalListResult, error) {
	if err := s.requireSuperAdmin(ctx); err != nil {
		return nil, err
	}
	rows, err := s.store.ListGlobal(ctx)
	if err != nil {
		return nil, err
	}

	search := strings.TrimSpace(strings.ToLower(in.Search))
	source := strings.TrimSpace(strings.ToLower(in.Source))
	filtered := make([]*domain.NamespaceBinding, 0, len(rows))
	for _, row := range rows {
		if source != "" && strings.ToLower(row.Source) != source {
			continue
		}
		if search != "" {
			haystack := strings.ToLower(strings.Join([]string{
				row.Prefix,
				row.Namespace,
				row.Source,
			}, " "))
			if !strings.Contains(haystack, search) {
				continue
			}
		}
		filtered = append(filtered, row)
	}

	sortBy := in.SortBy
	if sortBy == "" {
		sortBy = "prefix"
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		a, b := filtered[i], filtered[j]
		switch sortBy {
		case "namespace":
			if a.Namespace == b.Namespace {
				return a.Prefix < b.Prefix
			}
			return a.Namespace < b.Namespace
		case "weight":
			if a.Weight == b.Weight {
				return a.Prefix < b.Prefix
			}
			return a.Weight < b.Weight
		case "source":
			if a.Source == b.Source {
				return a.Prefix < b.Prefix
			}
			return a.Source < b.Source
		default:
			if a.Prefix == b.Prefix {
				return a.Namespace < b.Namespace
			}
			return a.Prefix < b.Prefix
		}
	})

	total := len(filtered)
	page := in.Page
	if page < 1 {
		page = 1
	}
	perPage := in.PerPage
	if perPage <= 0 {
		perPage = 25
	}
	start := (page - 1) * perPage
	if start >= total {
		return &GlobalListResult{Items: []*domain.NamespaceBinding{}, Total: total}, nil
	}
	end := start + perPage
	if end > total {
		end = total
	}
	return &GlobalListResult{
		Items: filtered[start:end],
		Total: total,
	}, nil
}

func (s *Service) StatsGlobal(ctx context.Context, id string) (*GlobalStatsReport, error) {
	if err := s.requireSuperAdmin(ctx); err != nil {
		return nil, err
	}
	binding, err := s.store.Lookup(ctx, id)
	if err != nil {
		return nil, errNotFound
	}
	if binding.ProjectID != "" {
		return nil, errNotFound
	}
	usages, err := s.store.UsageByProject(ctx, binding.Prefix)
	if err != nil {
		return nil, err
	}
	report := &GlobalStatsReport{
		NamespaceBinding: binding,
		ProjectUsages:    make([]ProjectUsageReport, 0, len(usages)),
		ProjectCount:     len(usages),
	}
	for _, usage := range usages {
		row := ProjectUsageReport{
			ProjectID:       usage.ProjectID,
			FieldCount:      usage.FieldCount,
			ModelCount:      usage.ModelCount,
			CollectionCount: usage.CollectionCount,
			TotalCount:      usage.TotalCount(),
		}
		report.ProjectUsages = append(report.ProjectUsages, row)
		report.TotalUsage += row.TotalCount
	}
	return report, nil
}

// ---------------------------------------------------------------------------
// Writes
// ---------------------------------------------------------------------------

// Create validates the input, checks for duplicate (prefix, namespace) pairs,
// inserts the row, and emits a changelog entry. Default weight is 10.
func (s *Service) Create(ctx context.Context, projectID string, in CreateInput) (*domain.NamespaceBinding, error) {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return nil, err
	}
	if errs := validateCreate(in); len(errs) > 0 {
		return nil, &ErrValidation{Fields: errs}
	}

	// Duplicate-pair check before insert. The DB also has a unique index;
	// the explicit check gives a clean 422 instead of a 500 with a
	// driver-specific error string.
	exists, err := s.store.ExistsByPrefixAndNamespace(ctx, in.Prefix, in.Namespace, "")
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, &ErrValidation{Fields: map[string][]string{
			"prefix": {"a binding with this prefix and namespace already exists"},
		}}
	}

	weight := in.Weight
	if weight == 0 {
		weight = 10
	}

	b := &domain.NamespaceBinding{
		ProjectID: projectID,
		Prefix:    in.Prefix,
		Namespace: in.Namespace,
		Weight:    weight,
		Source:    "user",
	}

	err = s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Create(ctx, b); err != nil {
			return err
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType: "namespace_binding",
			EntityID:   b.ID,
			Operation:  "create",
			ProjectID:  projectID,
			Payload:    marshalBinding(b),
		})
	})
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Update applies a partial update. Performs the duplicate check when prefix
// or namespace changes.
func (s *Service) Update(ctx context.Context, projectID, id string, in UpdateInput) (*domain.NamespaceBinding, error) {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return nil, err
	}

	existing, err := s.store.Get(ctx, projectID, id)
	if err != nil {
		if errors.Is(err, domain.ErrReadOnly) {
			return nil, err
		}
		return nil, errNotFound
	}

	prev := *existing
	applyUpdate(existing, in)

	if errs := validateMutable(existing); len(errs) > 0 {
		return nil, &ErrValidation{Fields: errs}
	}

	// Duplicate-pair check only when the (prefix, namespace) pair changed.
	if existing.Prefix != prev.Prefix || existing.Namespace != prev.Namespace {
		exists, err := s.store.ExistsByPrefixAndNamespace(ctx, existing.Prefix, existing.Namespace, id)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, &ErrValidation{Fields: map[string][]string{
				"prefix": {"a binding with this prefix and namespace already exists"},
			}}
		}
	}

	err = s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Update(ctx, existing); err != nil {
			return err
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "namespace_binding",
			EntityID:        id,
			Operation:       "update",
			ProjectID:       projectID,
			PreviousPayload: marshalBinding(&prev),
			Payload:         marshalBinding(existing),
		})
	})
	if err != nil {
		return nil, err
	}
	return existing, nil
}

func (s *Service) CreateGlobal(ctx context.Context, in CreateInput) (*domain.NamespaceBinding, error) {
	if err := s.requireSuperAdmin(ctx); err != nil {
		return nil, err
	}
	if errs := validateCreate(in); len(errs) > 0 {
		return nil, &ErrValidation{Fields: errs}
	}
	exists, err := s.store.ExistsByPrefixAndNamespace(ctx, in.Prefix, in.Namespace, "")
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, &ErrValidation{Fields: map[string][]string{
			"prefix": {"a binding with this prefix and namespace already exists"},
		}}
	}
	weight := in.Weight
	if weight == 0 {
		weight = 10
	}
	b := &domain.NamespaceBinding{
		Prefix:    in.Prefix,
		Namespace: in.Namespace,
		Weight:    weight,
		Source:    "user",
	}
	err = s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.CreateGlobal(ctx, b); err != nil {
			return err
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType: "namespace_binding",
			EntityID:   b.ID,
			Operation:  "create",
			Payload:    marshalBinding(b),
		})
	})
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (s *Service) UpdateGlobal(ctx context.Context, id string, in UpdateInput) (*domain.NamespaceBinding, error) {
	if err := s.requireSuperAdmin(ctx); err != nil {
		return nil, err
	}
	existing, err := s.store.GetGlobal(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrReadOnly) {
			return nil, err
		}
		return nil, errNotFound
	}
	prev := *existing
	applyUpdate(existing, in)
	if errs := validateMutable(existing); len(errs) > 0 {
		return nil, &ErrValidation{Fields: errs}
	}
	if existing.Prefix != prev.Prefix || existing.Namespace != prev.Namespace {
		exists, err := s.store.ExistsByPrefixAndNamespace(ctx, existing.Prefix, existing.Namespace, id)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, &ErrValidation{Fields: map[string][]string{
				"prefix": {"a binding with this prefix and namespace already exists"},
			}}
		}
	}
	err = s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.UpdateGlobal(ctx, existing); err != nil {
			return err
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "namespace_binding",
			EntityID:        id,
			Operation:       "update",
			PreviousPayload: marshalBinding(&prev),
			Payload:         marshalBinding(existing),
		})
	})
	if err != nil {
		return nil, err
	}
	return existing, nil
}

// Delete removes a user-owned binding. Returns domain.ErrReadOnly when the
// target row is system / imported.
func (s *Service) Delete(ctx context.Context, projectID, id string) error {
	if err := s.requireProjectWrite(ctx, projectID); err != nil {
		return err
	}

	existing, err := s.store.Get(ctx, projectID, id)
	if err != nil {
		if errors.Is(err, domain.ErrReadOnly) {
			return err
		}
		return errNotFound
	}

	return s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.Delete(ctx, projectID, id); err != nil {
			return err
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType:      "namespace_binding",
			EntityID:        id,
			Operation:       "delete",
			ProjectID:       projectID,
			PreviousPayload: marshalBinding(existing),
		})
	})
}

func (s *Service) DeleteGlobal(ctx context.Context, id string) error {
	if err := s.requireSuperAdmin(ctx); err != nil {
		return err
	}
	existing, err := s.store.Lookup(ctx, id)
	if err != nil {
		return errNotFound
	}
	if existing.ProjectID != "" {
		return errNotFound
	}
	return s.runner.Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
		if err := s.store.DeleteGlobal(ctx, id); err != nil {
			return err
		}
		return rec.Record(ctx, domain.ChangeLogEntry{
			EntityType: "namespace_binding",
			EntityID:   id,
			Operation:  "delete",
			Payload:    marshalBinding(existing),
		})
	})
}

// ---------------------------------------------------------------------------
// Permissions (delegates to auth.AuthSnapshot)
// ---------------------------------------------------------------------------

func (s *Service) requireProjectRead(ctx context.Context, projectID string) error {
	snap := auth.FromContext(ctx)
	res := auth.ProjectResourceFromContext(ctx)
	if !snap.Can(auth.ProjectRead, res, nil) {
		return &ErrForbidden{Capability: string(auth.ProjectRead), Resource: "project:" + projectID}
	}
	return nil
}

func (s *Service) requireProjectWrite(ctx context.Context, projectID string) error {
	snap := auth.FromContext(ctx)
	res := auth.ProjectResourceFromContext(ctx)
	if !snap.Can(auth.ProjectEdit, res, nil) {
		return &ErrForbidden{Capability: string(auth.ProjectEdit), Resource: "project:" + projectID}
	}
	return nil
}

func (s *Service) requireSuperAdmin(ctx context.Context) error {
	snap := auth.FromContext(ctx)
	if snap == nil || !snap.IsSuperAdmin {
		return &ErrForbidden{Capability: "super_admin", Resource: "admin"}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Validation + helpers
// ---------------------------------------------------------------------------

func validateCreate(in CreateInput) map[string][]string {
	errs := map[string][]string{}
	if strings.TrimSpace(in.Prefix) == "" {
		errs["prefix"] = []string{"prefix is required"}
	}
	if strings.TrimSpace(in.Namespace) == "" {
		errs["namespace"] = []string{"namespace is required"}
	}
	return errs
}

func validateMutable(b *domain.NamespaceBinding) map[string][]string {
	errs := map[string][]string{}
	if strings.TrimSpace(b.Prefix) == "" {
		errs["prefix"] = []string{"prefix is required"}
	}
	if strings.TrimSpace(b.Namespace) == "" {
		errs["namespace"] = []string{"namespace is required"}
	}
	return errs
}

func applyUpdate(b *domain.NamespaceBinding, in UpdateInput) {
	if in.Prefix != nil {
		b.Prefix = *in.Prefix
	}
	if in.Namespace != nil {
		b.Namespace = *in.Namespace
	}
	if in.Weight != nil {
		b.Weight = *in.Weight
	}
}

// marshalBinding serializes a binding for changelog payloads. Returns nil
// on nil input or marshal error.
func marshalBinding(b *domain.NamespaceBinding) []byte {
	if b == nil {
		return nil
	}
	out, err := json.Marshal(b)
	if err != nil {
		return nil
	}
	return out
}
