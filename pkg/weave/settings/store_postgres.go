package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
)

// vocabularyProjectSystemNameIndex is the unique index (migration 014) that
// rejects a second add of the same mount for a project. Named so the
// constraint-name check in AddServiceVocabulary reads as intent, not a
// magic string.
const vocabularyProjectSystemNameIndex = "idx_wv_project_system_name"

// vocabularyConceptListFKConstraint is weave_concept_lists.vocabulary_id's
// foreign key (migration 013's own comment already flags it: "has no ON
// DELETE action, so the delete below would simply fail without this" — that
// clause describes the global-tier cleanup migration 013 performs itself,
// not a guarantee that a project-owned vocabulary can always be deleted).
// It carries no ON DELETE clause, i.e. RESTRICT: a concept list bound to a
// vocabulary blocks deleting that vocabulary at the database level.
const vocabularyConceptListFKConstraint = "weave_concept_lists_vocabulary_id_fkey"

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
		})
	}
	enforce, err := s.queries.WeaveGetProjectConceptListEnforcement(ctx, projectID)
	if err != nil {
		return VocabularySettingsState{}, err
	}
	namespace := ""
	if ns, err := s.queries.WeaveGetProjectConceptNamespace(ctx, projectID); err == nil && ns != nil {
		namespace = *ns
	}
	return VocabularySettingsState{
		Options:   options,
		Enforce:   enforce,
		Namespace: namespace,
	}, nil
}

func (s *postgresStore) UpdateVocabularySettings(ctx context.Context, projectID string, enforceConceptLists bool, conceptNamespace string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin vocabulary settings update: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := s.queries.WithTx(tx)
	if err := q.WeaveUpdateProjectConceptListEnforcement(ctx, sqlcgen.WeaveUpdateProjectConceptListEnforcementParams{
		ID:                  projectID,
		EnforceConceptLists: enforceConceptLists,
	}); err != nil {
		return fmt.Errorf("update concept-list enforcement: %w", err)
	}
	var ns *string
	if trimmed := strings.TrimSpace(conceptNamespace); trimmed != "" {
		ns = &trimmed
	}
	if err := q.WeaveUpdateProjectConceptNamespace(ctx, sqlcgen.WeaveUpdateProjectConceptNamespaceParams{
		ID:               projectID,
		ConceptNamespace: ns,
	}); err != nil {
		return fmt.Errorf("update concept namespace: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit vocabulary settings update: %w", err)
	}
	return nil
}

// vocabularyConfig is the shape stored in weave_vocabularies.config for a
// service-backed row: enough to re-resolve entries from the configured
// vocabulary service. Lang is omitted when empty rather than stored as "".
type vocabularyConfig struct {
	Vocab string `json:"vocab"`
	Lang  string `json:"lang,omitempty"`
}

// errVocabularyAlreadyAdded reports that a project already owns a service
// vocabulary at this mount. Implements apierror.Conflicter so
// apierror.FromError maps it to a 409, turning the raw unique-index
// violation into a message a curator can read instead of a raw constraint
// failure.
type errVocabularyAlreadyAdded struct {
	mount string
}

func (e *errVocabularyAlreadyAdded) Error() string {
	return fmt.Sprintf("vocabulary %q is already added to this project", e.mount)
}

func (e *errVocabularyAlreadyAdded) ConflictMessage() string { return e.Error() }

// errVocabularyInUse reports that a vocabulary can't be removed because a
// concept list still references it (weave_concept_lists_vocabulary_id_fkey
// has no ON DELETE clause, i.e. RESTRICT). Implements apierror.InUser so
// apierror.FromError maps it to a 409 with code "in_use" — the shape
// api-patterns.md specifies for a delete blocked by dependents — instead of
// the raw foreign-key violation surfacing as a 500. Naming the specific
// concept list would need a second query on this error path; "used by a
// concept list" is the cheap, honest message.
type errVocabularyInUse struct{}

func (e *errVocabularyInUse) Error() string {
	return "this vocabulary is used by a concept list and cannot be removed"
}

func (e *errVocabularyInUse) InUseMessage() string { return e.Error() }

// AddServiceVocabulary enables a vocabulary the configured service serves:
// owning the row IS the enablement (#3599 vocabulary ownership), so this is
// nothing more than inserting the row. mount becomes both the system_name
// and the "vocab" the resolved config names; lang is optional.
func (s *postgresStore) AddServiceVocabulary(ctx context.Context, projectID, mount, lang string) error {
	mount = strings.TrimSpace(mount)
	if mount == "" {
		return fmt.Errorf("add service vocabulary: mount is required")
	}
	uiName, err := json.Marshal(domain.Translations{"en": mount})
	if err != nil {
		return fmt.Errorf("encode vocabulary label: %w", err)
	}
	config, err := json.Marshal(vocabularyConfig{Vocab: mount, Lang: strings.TrimSpace(lang)})
	if err != nil {
		return fmt.Errorf("encode vocabulary config: %w", err)
	}
	if err := s.queries.WeaveAddProjectServiceVocabulary(ctx, sqlcgen.WeaveAddProjectServiceVocabularyParams{
		ID:         ids.GenerateULID(),
		ProjectID:  projectID,
		SystemName: mount,
		UiName:     uiName,
		Config:     config,
	}); err != nil {
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) && pgerr.ConstraintName == vocabularyProjectSystemNameIndex {
			return &errVocabularyAlreadyAdded{mount: mount}
		}
		return fmt.Errorf("add service vocabulary: %w", err)
	}
	return nil
}

// RemoveVocabulary disables a project's vocabulary by deleting its row.
// Its cached entries go with it via
// weave_vocabulary_entries_vocabulary_id_fkey's ON DELETE CASCADE — no
// explicit entry delete is needed; they simply re-resolve if the mount is
// added again.
func (s *postgresStore) RemoveVocabulary(ctx context.Context, projectID, vocabularyID string) error {
	if err := s.queries.WeaveDeleteProjectVocabulary(ctx, sqlcgen.WeaveDeleteProjectVocabularyParams{
		ID:        vocabularyID,
		ProjectID: projectID,
	}); err != nil {
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) && pgerr.ConstraintName == vocabularyConceptListFKConstraint {
			return &errVocabularyInUse{}
		}
		return fmt.Errorf("remove vocabulary: %w", err)
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
