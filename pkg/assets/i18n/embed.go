package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

//go:embed *.json
var translationFiles embed.FS

// LoadTranslations loads all embedded translation files
func LoadTranslations() (map[string]map[string]interface{}, error) {
	translations := make(map[string]map[string]interface{})
	
	err := fs.WalkDir(translationFiles, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		
		if d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		
		// Skip non-translation files
		if path == "languages.json" {
			return nil
		}
		
		// Extract language code from filename (e.g., "en.json" -> "en")
		lang := strings.TrimSuffix(filepath.Base(path), ".json")
		
		// Read the file
		data, err := translationFiles.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read translation file %s: %w", path, err)
		}
		
		// Parse JSON
		var content map[string]interface{}
		if err := json.Unmarshal(data, &content); err != nil {
			return fmt.Errorf("failed to parse translation file %s: %w", path, err)
		}
		
		translations[lang] = content
		return nil
	})
	
	if err != nil {
		return nil, err
	}
	
	return translations, nil
}

// GetTranslationFiles returns the embedded filesystem for direct access
func GetTranslationFiles() embed.FS {
	return translationFiles
}

// LanguageInfo represents metadata about a language
type LanguageInfo struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	NativeName string `json:"nativeName"`
	Flag       string `json:"flag"`
	Enabled    bool   `json:"enabled"`
}

// LoadLanguageMetadata loads language configuration from languages.json
func LoadLanguageMetadata() ([]LanguageInfo, error) {
	data, err := translationFiles.ReadFile("languages.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read languages.json: %w", err)
	}
	
	var config struct {
		Languages []LanguageInfo `json:"languages"`
	}
	
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse languages.json: %w", err)
	}
	
	// Filter to only enabled languages that have translation files
	var enabledLanguages []LanguageInfo
	for _, lang := range config.Languages {
		if lang.Enabled {
			// Check if translation file exists
			if _, err := translationFiles.ReadFile(lang.Code + ".json"); err == nil {
				enabledLanguages = append(enabledLanguages, lang)
			}
		}
	}
	
	return enabledLanguages, nil
}