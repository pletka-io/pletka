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
// vocabularyStatusRemoved is the status a retired vocabulary reports to the
// settings screen. It is a presentation value, not a domain.Status: removal
// sets weave_vocabularies.deprecated, the axis domain-model.md defines for
// "retired, references intact, excluded from pickers".
const vocabularyStatusRemoved = "removed"

const vocabularyProjectSystemNameIndex = "idx_wv_project_system_name"

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
		// A removed vocabulary stays in the list — its concept lists still
		// resolve against it — so the status is what tells a curator it is
		// retired. Without this it renders identically to a live one and
		// "removed" is invisible, which is worse than not offering removal.
		status := row.Status
		if row.Deprecated {
			status = vocabularyStatusRemoved
		}
		options = append(options, VocabularySettingsOption{
			ID:          row.ID,
			SystemName:  row.SystemName,
			Label:       label,
			Description: desc,
			Status:      status,
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

// errVocabularyNotRemovable reports that a vocabulary cannot be retired.
// Only the local-terms row qualifies: it is every project's fallback for
// terms no thesaurus has, and a concept list with no source resolves against
// it. Implements apierror.Conflicter so this reads as a 409 with a reason
// rather than a bare failure.
type errVocabularyNotRemovable struct{}

func (e *errVocabularyNotRemovable) Error() string {
	return "the local terms vocabulary cannot be removed from a project"
}

func (e *errVocabularyNotRemovable) ConflictMessage() string { return e.Error() }

// AddServiceVocabulary enables a vocabulary the configured service serves:
// owning the row IS the enablement (#3599 vocabulary ownership), so this is
// nothing more than inserting the row. mount becomes both the system_name
// and the "vocab" the resolved config names; lang is optional.
func (s *postgresStore) AddServiceVocabulary(ctx context.Context, projectID string, mount ServiceMount) error {
	name := strings.TrimSpace(mount.Name)
	if name == "" {
		return fmt.Errorf("add service vocabulary: mount is required")
	}
	// Label the row the way the service labels the mount. Falling back to the
	// mount name keeps a row that the service could not describe readable
	// rather than blank.
	label := firstNonEmpty(strings.TrimSpace(mount.Label), name)
	uiName, err := json.Marshal(domain.Translations{"en": label})
	if err != nil {
		return fmt.Errorf("encode vocabulary label: %w", err)
	}
	config, err := json.Marshal(vocabularyConfig{Vocab: name, Lang: strings.TrimSpace(mount.Lang)})
	if err != nil {
		return fmt.Errorf("encode vocabulary config: %w", err)
	}
	// The query upserts: a mount this project removed earlier is revived,
	// and a mount that is genuinely still here updates nothing and returns
	// no row — which is the conflict.
	if _, err := s.queries.WeaveAddProjectServiceVocabulary(ctx, sqlcgen.WeaveAddProjectServiceVocabularyParams{
		ID:         ids.GenerateULID(),
		ProjectID:  projectID,
		SystemName: name,
		UiName:     uiName,
		BaseUri:    strings.TrimSpace(mount.BaseURI),
		Config:     config,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &errVocabularyAlreadyAdded{mount: name}
		}
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) && pgerr.ConstraintName == vocabularyProjectSystemNameIndex {
			return &errVocabularyAlreadyAdded{mount: name}
		}
		return fmt.Errorf("add service vocabulary: %w", err)
	}
	return nil
}

// RemoveVocabulary disables a project's vocabulary by deprecating its row.
//
// It is deliberately not a delete. Removal means "this project can no longer
// add from this source", not "unmake what was already built from it": the
// concept lists keep their pinned entries and keep resolving them, and only
// new work is refused. A delete cannot express that — the entries cascade
// away with the row, and weave_concept_lists' plain foreign key would refuse
// the delete anyway while any list still points at it.
//
// Deprecating also makes removal total rather than conditional. The previous
// delete returned in_use whenever a concept list referenced the vocabulary,
// so the one case where a curator most wants to stop new pins — a source they
// have already built on — was the one case they could not act on.
func (s *postgresStore) RemoveVocabulary(ctx context.Context, projectID, vocabularyID string) error {
	if _, err := s.queries.WeaveDeprecateProjectVocabulary(ctx, sqlcgen.WeaveDeprecateProjectVocabularyParams{
		ID:        vocabularyID,
		ProjectID: projectID,
	}); err != nil {
		// No row means the id did not match a removable vocabulary of this
		// project — in practice, the local-terms row, which the query
		// refuses. Reported as a conflict rather than a 404: the vocabulary
		// exists, it just cannot be retired.
		if errors.Is(err, pgx.ErrNoRows) {
			return &errVocabularyNotRemovable{}
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
