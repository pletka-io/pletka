package i18n

import "time"

// Storage interface for translation persistence
type Storage interface {
	// Basic operations
	Get(key, lang string) (*Translation, error)
	GetAll(lang string) (map[string]*Translation, error)
	Set(key, lang string, trans *Translation) error
	Delete(key, lang string) error
	
	// Bulk operations
	BulkGet(keys []string, lang string) (map[string]*Translation, error)
	BulkSet(translations map[string]*Translation) error
	
	// Query operations
	Query(filters QueryFilters) ([]*TranslationSet, error)
	ListLanguages() ([]string, error)
	ListKeys() ([]string, error)
	
	// Statistics
	Stats() (*StorageStats, error)
}

// StorageStats provides storage statistics
type StorageStats struct {
	TotalTranslations int
	TotalKeys         int
	TotalLanguages    int
	LastModified      time.Time
}