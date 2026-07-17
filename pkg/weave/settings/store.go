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
	Selected    bool
}

type VocabularySettingsState struct {
	Options  []VocabularySettingsOption
	Selected []string
	Enforce  bool
}

type Store interface {
	ReleaseVersions(ctx context.Context, projectID string) ([]string, error)
	VocabularySettingsState(ctx context.Context, projectID string) (VocabularySettingsState, error)
	GlobalVocabularyIDs(ctx context.Context) (map[string]bool, error)
	UpdateVocabularySettings(ctx context.Context, projectID string, vocabularyIDs []string, enforceConceptLists bool) error
}
