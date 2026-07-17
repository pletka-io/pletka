package bootstrap

import (
	"fmt"
	"log/slog"
	"maps"

	i18nassets "github.com/pletka-io/pletka/pkg/assets/i18n"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/i18n/backend"
)

// NewEmbeddedManager builds an i18n manager from the JSON bundles embedded in
// pkg/assets/i18n.
func NewEmbeddedManager(logger *slog.Logger) (i18n.Manager, error) {
	if logger == nil {
		logger = slog.Default()
	}
	storage := backend.NewMemoryBackend()

	translations, err := i18nassets.LoadTranslations()
	if err != nil {
		return nil, fmt.Errorf("load embedded translations: %w", err)
	}
	languageMetadata, err := i18nassets.LoadLanguageMetadata()
	if err != nil {
		return nil, fmt.Errorf("load language metadata: %w", err)
	}

	languages := make([]i18n.Language, 0, len(languageMetadata))
	for _, langInfo := range languageMetadata {
		languages = append(languages, i18n.Language{
			Code:        langInfo.Code,
			Name:        langInfo.NativeName,
			EnglishName: langInfo.Name,
			Direction:   "ltr",
			Enabled:     langInfo.Enabled,
			Metadata: map[string]any{
				"flag": langInfo.Flag,
			},
		})
	}

	for lang, content := range translations {
		flattened := flattenJSON(content, "")
		for key, value := range flattened {
			strValue, ok := value.(string)
			if !ok {
				continue
			}
			storage.Set(key, lang, &i18n.Translation{
				Key:      key,
				Language: lang,
				Value:    strValue,
				Status:   "active",
			})
		}
		logger.Info("Loaded translations", "language", lang, "keys", len(flattened))
	}

	manager, err := i18n.New(i18n.Config{
		DefaultLanguage:  "en",
		FallbackLanguage: "en",
		Storage:          storage,
		Logger:           logger,
		Languages:        languages,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize i18n manager: %w", err)
	}
	return manager, nil
}

func flattenJSON(data map[string]any, prefix string) map[string]any {
	result := make(map[string]any)
	for key, value := range data {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		switch v := value.(type) {
		case map[string]any:
			maps.Copy(result, flattenJSON(v, fullKey))
		default:
			result[fullKey] = value
		}
	}
	return result
}
