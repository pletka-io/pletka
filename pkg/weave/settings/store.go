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
	GlobalVocabularyIDs(ctx context.Context) (map[string]bool, error)
	UpdateVocabularySettings(ctx context.Context, projectID string, enforceConceptLists bool, conceptNamespace string) error
}
