package organization

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

type postgresStore struct {
	pool    *pgxpool.Pool
	db      sqlcgen.DBTX
	queries *sqlcgen.Queries
}

func NewPostgresStore(pool *pgxpool.Pool) Store {
	return &postgresStore{
		pool:    pool,
		db:      pool,
		queries: sqlcgen.New(pool),
	}
}

func (s *postgresStore) WithTx(tx pgx.Tx) Store {
	return &postgresStore{
		pool:    s.pool,
		db:      tx,
		queries: sqlcgen.New(tx),
	}
}

func (s *postgresStore) Create(ctx context.Context, org *domain.Organization) error {
	row, err := s.queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID:          org.ID,
		Type:        "organization",
		DisplayName: org.DisplayName,
		SystemName:  org.SystemName,
		Slug:        org.Slug,
		Acronym:     org.Acronym,
		Country:     org.Country,
		Website:     org.Website,
		Role:        "institution",
		Visibility:  org.Visibility,
		CreatedByID: org.CreatedByID,
	})
	if err != nil {
		return fmt.Errorf("create organization: %w", err)
	}
	*org = rowToOrganization(row)
	return nil
}

func (s *postgresStore) GetBySlug(ctx context.Context, slug string) (*domain.Organization, error) {
	row, err := s.queries.WeaveGetActorBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get organization by slug: %w", err)
	}
	if !isOrganizationType(row.Type) {
		return nil, nil
	}
	org := rowToOrganization(row)
	return &org, nil
}

func (s *postgresStore) ListByIDs(ctx context.Context, ids []string) ([]BrowseRow, error) {
	if len(ids) == 0 {
		return []BrowseRow{}, nil
	}
	const q = `
		SELECT wa.id, wa.created_at, wa.updated_at, wa.type, wa.display_name, wa.system_name,
		       wa.first_name, wa.last_name, wa.acronym, wa.email, wa.country, wa.website,
		       wa.orcid, wa.role, wa.parent_id, wa.staging_id, wa.slug, wa.visibility, wa.created_by_id,
		       COALESCE(p.project_count, 0)::bigint AS project_count
		FROM weave_actors wa
		LEFT JOIN (
		    SELECT owner_id, COUNT(*) AS project_count
		    FROM weave_projects
		    GROUP BY owner_id
		) p ON p.owner_id = wa.id
		WHERE wa.type = 'organization'
		  AND wa.id = ANY($1)
		ORDER BY wa.display_name`
	rows, err := s.pool.Query(ctx, q, ids)
	if err != nil {
		return nil, fmt.Errorf("list organizations by ids: %w", err)
	}
	defer rows.Close()

	out := make([]BrowseRow, 0, len(ids))
	for rows.Next() {
		record := sqlcgen.WeaveActor{}
		var projectCount int64
		if err := rows.Scan(
			&record.ID,
			&record.CreatedAt,
			&record.UpdatedAt,
			&record.Type,
			&record.DisplayName,
			&record.SystemName,
			&record.FirstName,
			&record.LastName,
			&record.Acronym,
			&record.Email,
			&record.Country,
			&record.Website,
			&record.Orcid,
			&record.Role,
			&record.ParentID,
			&record.StagingID,
			&record.Slug,
			&record.Visibility,
			&record.CreatedByID,
			&projectCount,
		); err != nil {
			return nil, fmt.Errorf("scan organization by ids: %w", err)
		}
		org := rowToOrganization(record)
		out = append(out, BrowseRow{
			Organization: &org,
			ProjectCount: projectCount,
		})
	}
	return out, rows.Err()
}

func (s *postgresStore) UpdateGeneral(ctx context.Context, org *domain.Organization) error {
	const q = `
		UPDATE weave_actors
		SET display_name = $2,
		    acronym = $3,
		    country = $4,
		    website = $5,
		    visibility = $6,
		    updated_at = NOW()
		WHERE id = $1
		  AND type = 'organization'
		RETURNING id, created_at, updated_at, type, display_name, system_name, first_name, last_name,
		          acronym, email, country, website, orcid, role, parent_id, staging_id, slug, visibility, created_by_id`
	record := sqlcgen.WeaveActor{}
	err := s.db.QueryRow(ctx, q,
		org.ID,
		org.DisplayName,
		org.Acronym,
		org.Country,
		org.Website,
		org.Visibility,
	).Scan(
		&record.ID,
		&record.CreatedAt,
		&record.UpdatedAt,
		&record.Type,
		&record.DisplayName,
		&record.SystemName,
		&record.FirstName,
		&record.LastName,
		&record.Acronym,
		&record.Email,
		&record.Country,
		&record.Website,
		&record.Orcid,
		&record.Role,
		&record.ParentID,
		&record.StagingID,
		&record.Slug,
		&record.Visibility,
		&record.CreatedByID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("organization not found")
		}
		return fmt.Errorf("update organization: %w", err)
	}
	*org = rowToOrganization(record)
	return nil
}

func (s *postgresStore) Browse(ctx context.Context, in BrowseInput) ([]BrowseRow, int64, error) {
	page := in.Page
	if page <= 0 {
		page = 1
	}
	perPage := in.PerPage
	if perPage <= 0 {
		perPage = 30
	}
	sortBy := in.SortBy
	if sortBy == "" {
		sortBy = "display_name"
	}
	search := strings.TrimSpace(in.Search)

	rows, err := s.queries.WeaveListOrganizations(ctx, sqlcgen.WeaveListOrganizationsParams{
		Search:         search,
		CountryCodes:   in.CountryCodes,
		IncludePrivate: in.IncludePrivate,
		ReadableOrgIds: in.ReadableOrgIDs,
		SortBy:         sortBy,
		SortDesc:       in.SortDesc,
		ResultOffset:   int32((page - 1) * perPage),
		ResultLimit:    int32(perPage),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("browse organizations: %w", err)
	}
	total, err := s.queries.WeaveCountOrganizations(ctx, sqlcgen.WeaveCountOrganizationsParams{
		Search:         search,
		CountryCodes:   in.CountryCodes,
		IncludePrivate: in.IncludePrivate,
		ReadableOrgIds: in.ReadableOrgIDs,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count organizations: %w", err)
	}
	out := make([]BrowseRow, 0, len(rows))
	for _, row := range rows {
		org := rowToOrganization(sqlcgen.WeaveActor{
			ID:          row.ID,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
			Type:        row.Type,
			DisplayName: row.DisplayName,
			SystemName:  row.SystemName,
			FirstName:   row.FirstName,
			LastName:    row.LastName,
			Acronym:     row.Acronym,
			Email:       row.Email,
			Country:     row.Country,
			Website:     row.Website,
			Orcid:       row.Orcid,
			Role:        row.Role,
			ParentID:    row.ParentID,
			StagingID:   row.StagingID,
			Slug:        row.Slug,
			Visibility:  row.Visibility,
			CreatedByID: row.CreatedByID,
		})
		out = append(out, BrowseRow{
			Organization: &org,
			ProjectCount: row.ProjectCount,
		})
	}
	return out, total, nil
}

func (s *postgresStore) ListVisibleCountries(ctx context.Context, includePrivate bool, readableOrgIDs []string) ([]string, error) {
	rows, err := s.queries.WeaveListOrganizationCountries(ctx, sqlcgen.WeaveListOrganizationCountriesParams{
		IncludePrivate: includePrivate,
		ReadableOrgIds: readableOrgIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("list organization countries: %w", err)
	}
	out := make([]string, 0, len(rows))
	for _, c := range rows {
		if c == nil || *c == "" {
			continue
		}
		out = append(out, *c)
	}
	return out, nil
}

func rowToOrganization(row sqlcgen.WeaveActor) domain.Organization {
	return domain.Organization{
		ID:          row.ID,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
		DisplayName: row.DisplayName,
		SystemName:  row.SystemName,
		Acronym:     row.Acronym,
		Country:     row.Country,
		Website:     row.Website,
		Slug:        row.Slug,
		Visibility:  normalizeVisibility(row.Visibility),
		CreatedByID: row.CreatedByID,
	}
}

func normalizeVisibility(v string) string {
	if v == "" {
		return "private"
	}
	return v
}

func isOrganizationType(t string) bool {
	return t == "organization"
}
