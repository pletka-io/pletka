package hub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

type postgresStore struct {
	queries *sqlcgen.Queries
	pool    *pgxpool.Pool
}

var _ Store = (*postgresStore)(nil)

// NewPostgresStore returns a Store backed by the supplied pgx pool.
func NewPostgresStore(pool *pgxpool.Pool) Store {
	return &postgresStore{
		queries: sqlcgen.New(pool),
		pool:    pool,
	}
}

func (s *postgresStore) Get(ctx context.Context, projectID, integrationID, configID string) (*ProjectIntegration, error) {
	row, err := s.queries.WeaveProjectIntegrationGet(ctx, sqlcgen.WeaveProjectIntegrationGetParams{
		ProjectID:     projectID,
		IntegrationID: integrationID,
		ConfigID:      configID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get project integration: %w", err)
	}
	return rowToProjectIntegration(row)
}

func (s *postgresStore) ListForProject(ctx context.Context, projectID string) ([]*ProjectIntegration, error) {
	rows, err := s.queries.WeaveProjectIntegrationsListForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project integrations: %w", err)
	}
	return rowsToProjectIntegrations(rows)
}

func (s *postgresStore) ListConfigsForIntegration(ctx context.Context, projectID, integrationID string) ([]*ProjectIntegration, error) {
	rows, err := s.queries.WeaveProjectIntegrationsListForIntegration(ctx, sqlcgen.WeaveProjectIntegrationsListForIntegrationParams{
		ProjectID:     projectID,
		IntegrationID: integrationID,
	})
	if err != nil {
		return nil, fmt.Errorf("list project integration configs: %w", err)
	}
	return rowsToProjectIntegrations(rows)
}

func (s *postgresStore) ListEnabledForProject(ctx context.Context, projectID string) ([]*ProjectIntegration, error) {
	rows, err := s.queries.WeaveProjectIntegrationsListEnabledForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list enabled project integrations: %w", err)
	}
	return rowsToProjectIntegrations(rows)
}

func (s *postgresStore) Upsert(ctx context.Context, pi *ProjectIntegration) (*ProjectIntegration, error) {
	cfg := pi.Config
	if cfg == nil {
		cfg = map[string]any{}
	}
	cfgBytes, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	var secretRef *string
	if pi.SecretRef != "" {
		s := pi.SecretRef
		secretRef = &s
	}
	var managed *string
	if pi.ManagedInstanceID != "" {
		m := pi.ManagedInstanceID
		managed = &m
	}
	row, err := s.queries.WeaveProjectIntegrationUpsert(ctx, sqlcgen.WeaveProjectIntegrationUpsertParams{
		ProjectID:         pi.ProjectID,
		IntegrationID:     pi.IntegrationID,
		ConfigID:          pi.ConfigID,
		Label:             pi.Label,
		Enabled:           pi.Enabled,
		Config:            cfgBytes,
		SecretRef:         secretRef,
		ManagedInstanceID: managed,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert project integration: %w", err)
	}
	return rowToProjectIntegration(row)
}

func (s *postgresStore) Delete(ctx context.Context, projectID, integrationID, configID string) error {
	if err := s.queries.WeaveProjectIntegrationDelete(ctx, sqlcgen.WeaveProjectIntegrationDeleteParams{
		ProjectID:     projectID,
		IntegrationID: integrationID,
		ConfigID:      configID,
	}); err != nil {
		return fmt.Errorf("delete project integration: %w", err)
	}
	return nil
}

func rowToProjectIntegration(row sqlcgen.WeaveProjectIntegration) (*ProjectIntegration, error) {
	cfg := map[string]any{}
	if len(row.Config) > 0 {
		if err := json.Unmarshal(row.Config, &cfg); err != nil {
			return nil, fmt.Errorf("unmarshal config: %w", err)
		}
	}
	secretRef := ""
	if row.SecretRef != nil {
		secretRef = *row.SecretRef
	}
	managed := ""
	if row.ManagedInstanceID != nil {
		managed = *row.ManagedInstanceID
	}
	return &ProjectIntegration{
		ProjectID:         row.ProjectID,
		IntegrationID:     row.IntegrationID,
		ConfigID:          row.ConfigID,
		Label:             row.Label,
		Enabled:           row.Enabled,
		Config:            cfg,
		SecretRef:         secretRef,
		ManagedInstanceID: managed,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}, nil
}

func rowsToProjectIntegrations(rows []sqlcgen.WeaveProjectIntegration) ([]*ProjectIntegration, error) {
	out := make([]*ProjectIntegration, 0, len(rows))
	for _, row := range rows {
		pi, err := rowToProjectIntegration(row)
		if err != nil {
			return nil, err
		}
		out = append(out, pi)
	}
	return out, nil
}
