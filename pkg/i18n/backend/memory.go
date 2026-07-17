package backend

import (
	"fmt"
	"strings"
	"sync"
	"time"
	
	"github.com/pletka-io/pletka/pkg/i18n"
)

// MemoryBackend implements an in-memory storage backend
type MemoryBackend struct {
	data      map[string]map[string]*i18n.Translation // [key][lang]translation
	languages map[string]bool
	mu        sync.RWMutex
}

// NewMemoryBackend creates a new in-memory backend
func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{
		data:      make(map[string]map[string]*i18n.Translation),
		languages: make(map[string]bool),
	}
}

// Get retrieves a translation
func (b *MemoryBackend) Get(key, lang string) (*i18n.Translation, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	if keyTrans, ok := b.data[key]; ok {
		if trans, ok := keyTrans[lang]; ok {
			// Return a copy to prevent external modification
			copy := *trans
			return &copy, nil
		}
	}
	
	return nil, fmt.Errorf("translation not found: %s[%s]", key, lang)
}

// Set stores a translation
func (b *MemoryBackend) Set(key, lang string, trans *i18n.Translation) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	// Initialize key map if needed
	if _, exists := b.data[key]; !exists {
		b.data[key] = make(map[string]*i18n.Translation)
	}
	
	// Create a copy to store
	copy := *trans
	copy.Key = key
	copy.Language = lang
	copy.UpdatedAt = time.Now()
	
	b.data[key][lang] = &copy
	b.languages[lang] = true
	
	return nil
}

// GetAll retrieves all translations for a language
func (b *MemoryBackend) GetAll(lang string) (map[string]*i18n.Translation, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	result := make(map[string]*i18n.Translation)
	
	for key, translations := range b.data {
		if trans, ok := translations[lang]; ok {
			// Return copies
			copy := *trans
			result[key] = &copy
		}
	}
	
	return result, nil
}

// Delete removes a translation
func (b *MemoryBackend) Delete(key, lang string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	if keyTrans, ok := b.data[key]; ok {
		delete(keyTrans, lang)
		
		// Remove key entirely if no translations left
		if len(keyTrans) == 0 {
			delete(b.data, key)
		}
	}
	
	return nil
}

// BulkGet retrieves multiple translations
func (b *MemoryBackend) BulkGet(keys []string, lang string) (map[string]*i18n.Translation, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	result := make(map[string]*i18n.Translation)
	
	for _, key := range keys {
		if keyTrans, ok := b.data[key]; ok {
			if trans, ok := keyTrans[lang]; ok {
				// Return a copy
				copy := *trans
				result[key] = &copy
			}
		}
	}
	
	return result, nil
}

// BulkSet stores multiple translations
func (b *MemoryBackend) BulkSet(translations map[string]*i18n.Translation) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	for key, trans := range translations {
		// Initialize key map if needed
		if _, exists := b.data[key]; !exists {
			b.data[key] = make(map[string]*i18n.Translation)
		}
		
		// Create a copy to store
		copy := *trans
		copy.UpdatedAt = time.Now()
		
		b.data[key][trans.Language] = &copy
		b.languages[trans.Language] = true
	}
	
	return nil
}

// Query searches for translations
func (b *MemoryBackend) Query(filters i18n.QueryFilters) ([]*i18n.TranslationSet, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	results := []*i18n.TranslationSet{}
	
	for key, translations := range b.data {
		// Apply filters
		if filters.Search != "" {
			found := false
			searchLower := strings.ToLower(filters.Search)
			
			// Search in key
			if strings.Contains(strings.ToLower(key), searchLower) {
				found = true
			}
			
			// Search in translations
			if !found {
				for _, trans := range translations {
					if strings.Contains(strings.ToLower(trans.Value), searchLower) ||
					   strings.Contains(strings.ToLower(trans.Comment), searchLower) {
						found = true
						break
					}
				}
			}
			
			if !found {
				continue
			}
		}
		
		// Filter by context
		if filters.Context != "" {
			hasContext := false
			for _, trans := range translations {
				if trans.Context == filters.Context {
					hasContext = true
					break
				}
			}
			if !hasContext {
				continue
			}
		}
		
		// Filter by language
		if filters.Language != "" {
			if _, hasLang := translations[filters.Language]; !hasLang {
				continue
			}
		}
		
		// Filter by status
		if filters.Status != "" {
			hasStatus := false
			for _, trans := range translations {
				if trans.Status == filters.Status {
					hasStatus = true
					break
				}
			}
			if !hasStatus {
				continue
			}
		}
		
		// Build translation set
		set := &i18n.TranslationSet{
			Key:          key,
			Translations: make(map[string]*i18n.Translation),
		}
		
		for lang, trans := range translations {
			// Return copies
			copy := *trans
			set.Translations[lang] = &copy
			
			if trans.Context != "" && set.Context == "" {
				set.Context = trans.Context
			}
		}
		
		results = append(results, set)
	}
	
	// Apply pagination
	if filters.PageSize > 0 {
		start := filters.PageNumber * filters.PageSize
		end := start + filters.PageSize
		
		if start > len(results) {
			return []*i18n.TranslationSet{}, nil
		}
		
		if end > len(results) {
			end = len(results)
		}
		
		return results[start:end], nil
	}
	
	return results, nil
}

// ListLanguages returns all languages
func (b *MemoryBackend) ListLanguages() ([]string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	languages := make([]string, 0, len(b.languages))
	for lang := range b.languages {
		languages = append(languages, lang)
	}
	
	return languages, nil
}

// ListKeys returns all translation keys
func (b *MemoryBackend) ListKeys() ([]string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	keys := make([]string, 0, len(b.data))
	for key := range b.data {
		keys = append(keys, key)
	}
	
	return keys, nil
}

// Stats returns storage statistics
func (b *MemoryBackend) Stats() (*i18n.StorageStats, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	totalTranslations := 0
	lastMod := time.Time{}
	
	for _, translations := range b.data {
		totalTranslations += len(translations)
		for _, trans := range translations {
			if trans.UpdatedAt.After(lastMod) {
				lastMod = trans.UpdatedAt
			}
		}
	}
	
	return &i18n.StorageStats{
		TotalTranslations: totalTranslations,
		TotalKeys:         len(b.data),
		TotalLanguages:    len(b.languages),
		LastModified:      lastMod,
	}, nil
}

// Clear removes all translations (useful for testing)
func (b *MemoryBackend) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	b.data = make(map[string]map[string]*i18n.Translation)
	b.languages = make(map[string]bool)
}