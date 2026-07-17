package organization

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/domain"
)

type BrowseInput struct {
	Search string
	// CountryCodes filters to orgs whose country is in this set.
	// Empty slice = no filter. Uses the same ISO 3166 alpha-2 codes
	// the general-settings country dropdown emits.
	CountryCodes   []string
	SortBy         string
	SortDesc       bool
	Page           int
	PerPage        int
	IncludePrivate bool
	ReadableOrgIDs []string
}

type BrowseRow struct {
	Organization *domain.Organization
	ProjectCount int64
}

type Store interface {
	WithTx(tx pgx.Tx) Store
	Create(ctx context.Context, org *domain.Organization) error
	GetBySlug(ctx context.Context, slug string) (*domain.Organization, error)
	ListByIDs(ctx context.Context, ids []string) ([]BrowseRow, error)
	UpdateGeneral(ctx context.Context, org *domain.Organization) error
	Browse(ctx context.Context, in BrowseInput) ([]BrowseRow, int64, error)
	// ListVisibleCountries returns distinct country codes for orgs the
	// caller can see. Used to populate the country filter dropdown.
	ListVisibleCountries(ctx context.Context, includePrivate bool, readableOrgIDs []string) ([]string, error)
}
