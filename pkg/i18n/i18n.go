package i18n

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"sync"
	"time"
)

// Manager is the main i18n manager interface
type Manager interface {
	// Basic translation methods
	T(key, lang string, args ...any) string
	TSafe(key, lang string, args ...any) template.HTML
	TC(key, context, lang string, args ...any) string
	TN(key string, count int, lang string, args ...any) string

	// Formatting methods
	FormatNumber(num float64, lang string) string
	FormatCurrency(amount float64, currency, lang string) string
	FormatDate(t time.Time, format, lang string) string
	FormatPercent(value float64, lang string) string

	// Management methods
	Languages() []Language
	AddLanguage(lang Language) error
	SetFallbackLanguage(lang string)

	// Translation management
	Get(key, lang string) (*Translation, error)
	Set(key, lang string, translation *Translation) error
	Query(filters QueryFilters) ([]*TranslationSet, error)
	BulkUpdate(updates BulkUpdate) error

	Stats() (*Stats, error)

	// Resolve walks v recursively and enriches every LocalizedText
	// with bundle lookups for the given language. Mutates in place;
	// callers pass a pointer to the schema. Raw domain.Translations
	// values pass through unmodified — schemas can adopt LocalizedText
	// file by file without coordinated migration.
	Resolve(v any, lang string)
}

// Language represents a supported language
type Language struct {
	Code        string // ISO 639-1 code (e.g., "en", "nl")
	Name        string // Native name (e.g., "English", "Nederlands")
	EnglishName string // English name
	Direction   string // "ltr" or "rtl"
	Enabled     bool
	Metadata    map[string]interface{} // Additional metadata (e.g., flag emoji)
}

// Stats provides statistics about translations
type Stats struct {
	TotalKeys          int
	TotalLanguages     int
	TranslationsByLang map[string]int
	MissingByLang      map[string]int
	FuzzyByLang        map[string]int
	LastUpdated        time.Time
}

// manager is the default implementation
type manager struct {
	storage      Storage
	cache        Cache
	config       Config
	logger       *slog.Logger
	fallbackLang string
	languages    map[string]Language
	mu           sync.RWMutex
}

// Config holds i18n configuration
type Config struct {
	DefaultLanguage  string
	FallbackLanguage string
	Storage          Storage
	Cache            Cache
	Logger           *slog.Logger
	DebugMode        bool
	Languages        []Language
}

// New creates a new i18n manager
func New(config Config) (Manager, error) {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	if config.Storage == nil {
		return nil, fmt.Errorf("storage backend is required")
	}

	m := &manager{
		storage:      config.Storage,
		cache:        config.Cache,
		config:       config,
		logger:       config.Logger.With(slog.String("component", "i18n")),
		fallbackLang: config.FallbackLanguage,
		languages:    make(map[string]Language),
	}

	// Initialize languages
	for _, lang := range config.Languages {
		m.languages[lang.Code] = lang
	}

	// Set default fallback if not specified
	if m.fallbackLang == "" {
		m.fallbackLang = "en"
	}

	m.logger.Info("i18n manager initialized",
		slog.String("default", config.DefaultLanguage),
		slog.String("fallback", m.fallbackLang),
		slog.Int("languages", len(m.languages)))

	return m, nil
}

// T returns a translated string
func (m *manager) T(key, lang string, args ...any) string {
	trans, err := m.getTranslation(key, "", lang)
	if err != nil {
		if m.config.DebugMode {
			return fmt.Sprintf("missing_key_%s", key)
		}
		return key
	}

	// Format with arguments if provided
	if len(args) > 0 {
		return fmt.Sprintf(trans.Value, args...)
	}

	return trans.Value
}

// TSafe returns a translated string as template.HTML
func (m *manager) TSafe(key, lang string, args ...any) template.HTML {
	value := m.T(key, lang, args...)
	return template.HTML(value)
}

// TC returns a translated string with context
func (m *manager) TC(key, context, lang string, args ...any) string {
	trans, err := m.getTranslation(key, context, lang)
	if err != nil {
		if m.config.DebugMode {
			return fmt.Sprintf("missing_key_%s_context_%s", key, context)
		}
		return key
	}

	if len(args) > 0 {
		return fmt.Sprintf(trans.Value, args...)
	}

	return trans.Value
}

// getTranslation retrieves a translation with caching
func (m *manager) getTranslation(key, contextStr, lang string) (*Translation, error) {
	// Try cache first
	cacheKey := m.makeCacheKey(key, contextStr, lang)

	if m.cache != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		if trans, err := m.cache.Get(ctx, cacheKey); err == nil && trans != nil {
			m.logger.Debug("cache hit", slog.String("key", cacheKey))
			return trans, nil
		}
	}

	// Try requested language
	trans, err := m.storage.Get(key, lang)
	if err == nil && trans != nil {
		if contextStr == "" || trans.Context == contextStr {
			m.cacheTranslation(cacheKey, trans)
			return trans, nil
		}
	}

	// Try fallback language
	if lang != m.fallbackLang {
		trans, err = m.storage.Get(key, m.fallbackLang)
		if err == nil && trans != nil {
			if contextStr == "" || trans.Context == contextStr {
				m.cacheTranslation(cacheKey, trans)
				return trans, nil
			}
		}
	}

	return nil, fmt.Errorf("translation not found: %s", key)
}

// Implement missing Manager interface methods

func (m *manager) FormatNumber(num float64, lang string) string {
	// TODO: Implement proper number formatting
	return fmt.Sprintf("%.2f", num)
}

func (m *manager) FormatCurrency(amount float64, currency, lang string) string {
	// TODO: Implement proper currency formatting
	return fmt.Sprintf("%.2f %s", amount, currency)
}

func (m *manager) FormatDate(t time.Time, format, lang string) string {
	// TODO: Implement proper date formatting
	return t.Format("2006-01-02")
}

func (m *manager) FormatPercent(value float64, lang string) string {
	// TODO: Implement proper percent formatting
	return fmt.Sprintf("%.1f%%", value*100)
}

func (m *manager) Languages() []Language {
	m.mu.RLock()
	defer m.mu.RUnlock()

	languages := make([]Language, 0, len(m.languages))
	for _, lang := range m.languages {
		languages = append(languages, lang)
	}
	return languages
}

func (m *manager) AddLanguage(lang Language) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.languages[lang.Code] = lang
	return nil
}

func (m *manager) SetFallbackLanguage(lang string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.fallbackLang = lang
}

func (m *manager) Get(key, lang string) (*Translation, error) {
	return m.getTranslation(key, "", lang)
}

func (m *manager) Set(key, lang string, translation *Translation) error {
	return m.storage.Set(key, lang, translation)
}

func (m *manager) Query(filters QueryFilters) ([]*TranslationSet, error) {
	return m.storage.Query(filters)
}

func (m *manager) BulkUpdate(updates BulkUpdate) error {
	translations := make(map[string]*Translation)
	for _, update := range updates.Updates {
		key := fmt.Sprintf("%s:%s", update.Key, update.Language)
		translations[key] = &Translation{
			Key:      update.Key,
			Language: update.Language,
			Value:    update.Value,
			Status:   update.Status,
		}
	}
	return m.storage.BulkSet(translations)
}

func (m *manager) Stats() (*Stats, error) {
	storageStats, err := m.storage.Stats()
	if err != nil {
		return nil, err
	}

	return &Stats{
		TotalKeys:          storageStats.TotalKeys,
		TotalLanguages:     storageStats.TotalLanguages,
		LastUpdated:        storageStats.LastModified,
		TranslationsByLang: make(map[string]int),
		MissingByLang:      make(map[string]int),
		FuzzyByLang:        make(map[string]int),
	}, nil
}

// makeCacheKey creates a cache key
func (m *manager) makeCacheKey(key, context, lang string) string {
	if context != "" {
		return fmt.Sprintf("%s:%s:%s", lang, context, key)
	}
	return fmt.Sprintf("%s:%s", lang, key)
}

// cacheTranslation stores a translation in cache
func (m *manager) cacheTranslation(key string, trans *Translation) {
	if m.cache != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		if err := m.cache.Set(ctx, key, trans, 1*time.Hour); err != nil {
			m.logger.Debug("cache set failed",
				slog.String("key", key),
				slog.Any("err", err))
		}
	}
}
