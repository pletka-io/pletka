package settings

import (
	"context"

	"github.com/pletka-io/pletka/pkg/domain"
)

type VocabularySettingsOption struct {
	ID          string
	SystemName  string
	Label       domain.Translations
	Description domain.Translations
	Status      string
	BaseURI     string
}

type VocabularySettingsState struct {
	Options   []VocabularySettingsOption
	Enforce   bool
	Namespace string // concept namespace override (F4, #3599); "" = platform default
}

type Store interface {
	ReleaseVersions(ctx context.Context, projectID string) ([]string, error)
	ReleaseArchived(ctx context.Context, projectID, version string) (bool, error)
	VocabularySettingsState(ctx context.Context, projectID string) (VocabularySettingsState, error)
	UpdateVocabularySettings(ctx context.Context, projectID string, enforceConceptLists bool, conceptNamespace string) error
	// AddServiceVocabulary enables a vocabulary the configured service
	// serves for a project: owning the row IS the enablement (#3599
	// vocabulary ownership). mount names what the service serves (e.g.
	// "aat"); lang may be empty. Returns a *apierror.Error-compatible
	// conflict (via apierror.FromError) when the project already has this
	// mount — see the unique index added in migration 014.
	AddServiceVocabulary(ctx context.Context, projectID, mount, lang string) error
	// RemoveVocabulary disables a project's vocabulary by deleting its row.
	// Its cached entries go with it via
	// weave_vocabulary_entries_vocabulary_id_fkey's ON DELETE CASCADE —
	// they simply re-resolve from the service if it is added again.
	RemoveVocabulary(ctx context.Context, projectID, vocabularyID string) error
}
