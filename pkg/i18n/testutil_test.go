package i18n_test

import (
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/i18n/backend"
)

func newMemoryManager(config i18n.Config) (i18n.Manager, error) {
	if config.Storage == nil {
		config.Storage = backend.NewMemoryBackend()
	}
	if config.DefaultLanguage == "" {
		config.DefaultLanguage = "en"
	}
	if config.FallbackLanguage == "" {
		config.FallbackLanguage = "en"
	}
	if len(config.Languages) == 0 {
		config.Languages = []i18n.Language{
			{Code: "en", Name: "English", EnglishName: "English", Direction: "ltr", Enabled: true},
		}
	}
	return i18n.New(config)
}
