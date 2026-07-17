package organization

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
)

type ErrValidation struct {
	Fields map[string][]string
}

func (e *ErrValidation) Error() string { return "validation error" }

type CreateInput struct {
	DisplayName string  `json:"display_name"`
	Slug        string  `json:"slug"`
	Acronym     *string `json:"acronym,omitempty"`
	Country     *string `json:"country,omitempty"`
	Website     *string `json:"website,omitempty"`
}

type UpdateGeneralInput struct {
	DisplayName string  `json:"display_name"`
	Acronym     *string `json:"acronym,omitempty"`
	Country     *string `json:"country,omitempty"`
	Website     *string `json:"website,omitempty"`
	Visibility  string  `json:"visibility"`
}

type BrowseItem struct {
	ID           string  `json:"id"`
	Slug         string  `json:"slug"`
	DisplayName  string  `json:"display_name"`
	Acronym      *string `json:"acronym,omitempty"`
	Country      *string `json:"country,omitempty"`
	Website      *string `json:"website,omitempty"`
	Visibility   string  `json:"visibility"`
	ProjectCount int64   `json:"project_count"`
}

type BrowseResult struct {
	Items []BrowseItem
	Total int64
}

type Service struct {
	store Store
	// sanctioned pool holder: tx-owning service — see docs-oss/architecture/slices.md
	pool *pgxpool.Pool
	log  *slog.Logger
}

func NewService(store Store, pool *pgxpool.Pool, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: store, pool: pool, log: log}
}

func (s *Service) GetBySlug(ctx context.Context, slug string) (*domain.Organization, error) {
	return s.store.GetBySlug(ctx, slug)
}

func (s *Service) ListByIDs(ctx context.Context, ids []string) ([]BrowseItem, error) {
	rows, err := s.store.ListByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	items := make([]BrowseItem, 0, len(rows))
	for _, row := range rows {
		org := row.Organization
		items = append(items, BrowseItem{
			ID:           org.Slug,
			Slug:         org.Slug,
			DisplayName:  org.DisplayName,
			Acronym:      org.Acronym,
			Country:      org.Country,
			Website:      org.Website,
			Visibility:   org.Visibility,
			ProjectCount: row.ProjectCount,
		})
	}
	return items, nil
}

// ListVisibleCountries returns the country codes that have at least
// one organization the caller can see. Used by the country filter
// dropdown so it only shows countries with actual rows.
func (s *Service) ListVisibleCountries(ctx context.Context, includePrivate bool, readableOrgIDs []string) ([]string, error) {
	return s.store.ListVisibleCountries(ctx, includePrivate, readableOrgIDs)
}

func (s *Service) Browse(ctx context.Context, in BrowseInput) (*BrowseResult, error) {
	rows, total, err := s.store.Browse(ctx, in)
	if err != nil {
		return nil, err
	}
	items := make([]BrowseItem, 0, len(rows))
	for _, row := range rows {
		org := row.Organization
		items = append(items, BrowseItem{
			ID:           org.Slug,
			Slug:         org.Slug,
			DisplayName:  org.DisplayName,
			Acronym:      org.Acronym,
			Country:      org.Country,
			Website:      org.Website,
			Visibility:   org.Visibility,
			ProjectCount: row.ProjectCount,
		})
	}
	return &BrowseResult{Items: items, Total: total}, nil
}

func (s *Service) CreateSelfOrganization(ctx context.Context, actorID string, in CreateInput) (*domain.Organization, error) {
	errs := validateCreate(in)
	if len(errs) > 0 {
		return nil, &ErrValidation{Fields: errs}
	}
	if actorID == "" {
		return nil, errors.New("organization: missing creator actor")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin organization create tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txStore := s.store.WithTx(tx)
	org := &domain.Organization{
		ID:          ids.GenerateULID(),
		DisplayName: strings.TrimSpace(in.DisplayName),
		Slug:        strings.TrimSpace(in.Slug),
		Acronym:     trimOptional(in.Acronym),
		Country:     trimOptional(in.Country),
		Website:     trimOptional(in.Website),
		Visibility:  "private",
		CreatedByID: &actorID,
	}
	if err := txStore.Create(ctx, org); err != nil {
		if pgerr := (*pgconn.PgError)(nil); errors.As(err, &pgerr) {
			switch pgerr.ConstraintName {
			case "idx_wa_slug":
				return nil, &ErrValidation{Fields: map[string][]string{"slug": {"slug is already in use"}}}
			case "idx_wa_system_name":
				return nil, &ErrValidation{Fields: map[string][]string{"slug": {"slug is already in use"}}}
			}
		}
		return nil, err
	}

	if err := sqlcgen.New(tx).WeaveMembershipUpsert(ctx, sqlcgen.WeaveMembershipUpsertParams{
		ActorID:   actorID,
		ScopeType: "org",
		ScopeID:   org.ID,
		Role:      "owner",
	}); err != nil {
		return nil, fmt.Errorf("seed org owner membership: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit organization create tx: %w", err)
	}
	return org, nil
}

func (s *Service) UpdateGeneral(ctx context.Context, org *domain.Organization, in UpdateGeneralInput) (*domain.Organization, error) {
	errs := validateGeneral(in)
	if len(errs) > 0 {
		return nil, &ErrValidation{Fields: errs}
	}
	next := *org
	next.DisplayName = strings.TrimSpace(in.DisplayName)
	next.Acronym = trimOptional(in.Acronym)
	next.Country = trimOptional(in.Country)
	next.Website = trimOptional(in.Website)
	next.Visibility = in.Visibility
	if err := s.store.UpdateGeneral(ctx, &next); err != nil {
		return nil, err
	}
	return &next, nil
}

func ReadableOrgIDs(ctx context.Context) (bool, []string) {
	snap := weaveauth.FromContext(ctx)
	if snap == nil {
		return false, nil
	}
	if snap.IsSuperAdmin {
		return true, nil
	}
	ids := make([]string, 0, len(snap.Roles))
	for key := range snap.Roles {
		if strings.HasPrefix(key, "org:") {
			ids = append(ids, strings.TrimPrefix(key, "org:"))
		}
	}
	return false, ids
}

func validateCreate(in CreateInput) map[string][]string {
	errs := map[string][]string{}
	if strings.TrimSpace(in.DisplayName) == "" {
		errs["display_name"] = append(errs["display_name"], "name is required")
	}
	slug := strings.TrimSpace(in.Slug)
	if slug == "" {
		errs["slug"] = append(errs["slug"], "slug is required")
	} else if err := weaveauth.ValidateSlug(slug); err != nil {
		errs["slug"] = append(errs["slug"], err.Error())
	}
	return errs
}

func validateGeneral(in UpdateGeneralInput) map[string][]string {
	errs := map[string][]string{}
	if strings.TrimSpace(in.DisplayName) == "" {
		errs["display_name"] = append(errs["display_name"], "name is required")
	}
	switch in.Visibility {
	case "public", "private":
	default:
		errs["visibility"] = append(errs["visibility"], "visibility must be public or private")
	}
	return errs
}

func trimOptional(v *string) *string {
	if v == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*v)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
