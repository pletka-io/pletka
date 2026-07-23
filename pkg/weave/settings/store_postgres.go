package settings

import (
	"context"
	"encoding/json"
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
	queries *sqlcgen.Queries
}

func NewPostgresStore(pool *pgxpool.Pool) Store {
	return &postgresStore{
		pool:    pool,
		queries: sqlcgen.New(pool),
	}
}

func (s *postgresStore) ReleaseVersions(ctx context.Context, projectID string) ([]string, error) {
	versions, err := s.queries.WeaveListProjectReleaseVersions(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("query project releases: %w", err)
	}
	out := make([]string, 0, len(versions))
	for _, version := range versions {
		out = append(out, strings.TrimSpace(version))
	}
	return out, nil
}

func (s *postgresStore) ReleaseArchived(ctx context.Context, projectID, version string) (bool, error) {
	archived, err := s.queries.WeaveIsReleaseArchived(ctx, sqlcgen.WeaveIsReleaseArchivedParams{ProjectID: projectID, Version: version})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check release archived: %w", err)
	}
	return archived, nil
}

func (s *postgresStore) VocabularySettingsState(ctx context.Context, projectID string) (VocabularySettingsState, error) {
	rows, err := s.queries.WeaveListVocabularySettingsOptions(ctx, projectID)
	if err != nil {
		return VocabularySettingsState{}, err
	}
	options := make([]VocabularySettingsOption, 0, len(rows))
	selected := []string{}
	for _, row := range rows {
		label := settingsTranslations(row.UiName)
		if len(label) == 0 {
			label = domain.Translations{"en": firstNonEmpty(row.SystemName, row.ID)}
		}
		desc := settingsTranslations(row.Description)
		if len(desc) == 0 && row.BaseUri != "" {
			desc = domain.Translations{"en": row.BaseUri}
		}
		options = append(options, VocabularySettingsOption{
			ID:          row.ID,
			SystemName:  row.SystemName,
			Label:       label,
			Description: desc,
			Status:      row.Status,
			BaseURI:     row.BaseUri,
			Selected:    row.Selected,
		})
		if row.Selected {
			selected = append(selected, row.ID)
		}
	}
	enforce, err := s.queries.WeaveGetProjectConceptListEnforcement(ctx, projectID)
	if err != nil {
		return VocabularySettingsState{}, err
	}
	return VocabularySettingsState{
		Options:  options,
		Selected: selected,
		Enforce:  enforce,
	}, nil
}

func (s *postgresStore) GlobalVocabularyIDs(ctx context.Context) (map[string]bool, error) {
	ids, err := s.queries.WeaveListGlobalVocabularyIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

func (s *postgresStore) UpdateVocabularySettings(ctx context.Context, projectID string, vocabularyIDs []string, enforceConceptLists bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin vocabulary settings update: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := s.queries.WithTx(tx)
	if err := q.WeaveDeactivateProjectVocabularies(ctx, projectID); err != nil {
		return fmt.Errorf("deactivate project vocabularies: %w", err)
	}
	for _, id := range vocabularyIDs {
		if err := q.WeaveUpsertActiveProjectVocabulary(ctx, sqlcgen.WeaveUpsertActiveProjectVocabularyParams{
			ProjectID:    projectID,
			VocabularyID: id,
		}); err != nil {
			return fmt.Errorf("upsert project vocabulary %q: %w", id, err)
		}
	}
	if err := q.WeaveUpdateProjectConceptListEnforcement(ctx, sqlcgen.WeaveUpdateProjectConceptListEnforcementParams{
		ID:                  projectID,
		EnforceConceptLists: enforceConceptLists,
	}); err != nil {
		return fmt.Errorf("update concept-list enforcement: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit vocabulary settings update: %w", err)
	}
	return nil
}

func settingsTranslations(raw []byte) domain.Translations {
	if len(raw) == 0 {
		return domain.Translations{}
	}
	var out domain.Translations
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return domain.Translations{}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
