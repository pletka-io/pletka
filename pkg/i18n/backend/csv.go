package backend

import (
	"encoding/csv"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
	
	"github.com/pletka-io/pletka/pkg/i18n"
)

// CSVBackend implements storage using CSV files
type CSVBackend struct {
	filename string
	data     map[string]map[string]*i18n.Translation // [key][lang]translation
	languages []string
	logger   *slog.Logger
	mu       sync.RWMutex
	options  CSVOptions
}

// CSVOptions configures the CSV backend
type CSVOptions struct {
	Delimiter      rune
	HasHeader      bool
	AutoSave       bool
	WatchFile      bool
	Logger         *slog.Logger
	BackupOnSave   bool
}

// NewCSVBackend creates a new CSV backend
func NewCSVBackend(filename string, opts CSVOptions) (*CSVBackend, error) {
	if opts.Delimiter == 0 {
		opts.Delimiter = ','
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	
	b := &CSVBackend{
		filename:  filename,
		data:      make(map[string]map[string]*i18n.Translation),
		languages: []string{},
		logger:    opts.Logger.With(slog.String("backend", "csv")),
		options:   opts,
	}
	
	// Load existing file
	if err := b.load(); err != nil {
		// File might not exist yet, which is OK
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load CSV: %w", err)
		}
		b.logger.Info("CSV file not found, starting with empty data", 
			slog.String("file", filename))
	}
	
	return b, nil
}

// load reads the CSV file
func (b *CSVBackend) load() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	file, err := os.Open(b.filename)
	if err != nil {
		return err
	}
	defer file.Close()
	
	reader := csv.NewReader(file)
	reader.Comma = b.options.Delimiter
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read CSV: %w", err)
	}
	
	if len(records) == 0 {
		return nil
	}
	
	// Parse header
	headers := make(map[string]int)
	languages := []string{}
	
	for idx, header := range records[0] {
		headers[header] = idx
		
		// Detect language columns (lang_XX format)
		if strings.HasPrefix(header, "lang_") {
			lang := strings.TrimPrefix(header, "lang_")
			languages = append(languages, lang)
		}
	}
	
	b.languages = languages
	
	// Required columns
	msgIDIdx, hasMsgID := headers["msgID"]
	if !hasMsgID {
		// Try alternative column names for backwards compatibility
		if idx, ok := headers["id"]; ok {
			msgIDIdx = idx
			hasMsgID = true
		}
	}
	
	if !hasMsgID {
		return fmt.Errorf("CSV missing required column: msgID or id")
	}
	
	// Optional columns
	contextIdx, _ := headers["context"]
	htmlIdx, _ := headers["html"]
	markdownIdx, _ := headers["markdown"]
	commentIdx, _ := headers["comment"]
	statusIdx, _ := headers["status"]
	
	// Process records
	for i := 1; i < len(records); i++ {
		record := records[i]
		if len(record) <= msgIDIdx {
			continue
		}
		
		key := record[msgIDIdx]
		if key == "" {
			continue
		}
		
		// Initialize key map if needed
		if _, exists := b.data[key]; !exists {
			b.data[key] = make(map[string]*i18n.Translation)
		}
		
		// Process each language
		for _, lang := range languages {
			langIdx, ok := headers["lang_"+lang]
			if !ok || len(record) <= langIdx {
				continue
			}
			
			value := record[langIdx]
			if value == "" {
				continue
			}
			
			trans := &i18n.Translation{
				Key:      key,
				Value:    value,
				Language: lang,
				Status:   i18n.StatusApproved,
			}
			
			// Set optional fields
			if contextIdx > 0 && len(record) > contextIdx {
				trans.Context = record[contextIdx]
			}
			
			if htmlIdx > 0 && len(record) > htmlIdx {
				trans.IsHTML = strings.EqualFold(record[htmlIdx], "true")
			}
			
			if markdownIdx > 0 && len(record) > markdownIdx {
				trans.IsMarkdown = strings.EqualFold(record[markdownIdx], "true")
			}
			
			if commentIdx > 0 && len(record) > commentIdx {
				trans.Comment = record[commentIdx]
			}
			
			if statusIdx > 0 && len(record) > statusIdx {
				trans.Status = i18n.TranslationStatus(record[statusIdx])
			}
			
			// Handle pipe-separated plural forms
			if strings.Contains(value, "|") {
				parts := strings.Split(value, "|")
				trans.PluralForms = map[i18n.PluralForm]string{
					i18n.PluralOne:   parts[0],
					i18n.PluralOther: parts[len(parts)-1],
				}
			}
			
			b.data[key][lang] = trans
		}
	}
	
	b.logger.Info("loaded translations from CSV",
		slog.String("file", b.filename),
		slog.Int("keys", len(b.data)),
		slog.Int("languages", len(b.languages)))
	
	return nil
}

// save writes the CSV file
func (b *CSVBackend) save() error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	// Backup if requested
	if b.options.BackupOnSave {
		backupName := fmt.Sprintf("%s.%s.bak", b.filename, time.Now().Format("20060102-150405"))
		if err := b.copyFile(b.filename, backupName); err != nil {
			b.logger.Warn("failed to create backup",
				slog.Any("err", err))
		}
	}
	
	file, err := os.Create(b.filename)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()
	
	writer := csv.NewWriter(file)
	writer.Comma = b.options.Delimiter
	defer writer.Flush()
	
	// Build header
	headers := []string{"msgID", "context"}
	for _, lang := range b.languages {
		headers = append(headers, "lang_"+lang)
	}
	headers = append(headers, "html", "markdown", "status", "comment")
	
	if err := writer.Write(headers); err != nil {
		return err
	}
	
	// Write records
	for key, translations := range b.data {
		record := make([]string, len(headers))
		record[0] = key
		
		// Get context from any translation
		var context, comment string
		var isHTML, isMarkdown bool
		var status i18n.TranslationStatus = i18n.StatusApproved
		
		for _, trans := range translations {
			if trans.Context != "" {
				context = trans.Context
			}
			if trans.Comment != "" {
				comment = trans.Comment
			}
			if trans.IsHTML {
				isHTML = true
			}
			if trans.IsMarkdown {
				isMarkdown = true
			}
			if trans.Status != "" {
				status = trans.Status
			}
		}
		
		record[1] = context
		
		// Add translations for each language
		for i, lang := range b.languages {
			if trans, ok := translations[lang]; ok {
				// Convert plural forms back to pipe format if needed
				if len(trans.PluralForms) > 0 {
					if one, hasOne := trans.PluralForms[i18n.PluralOne]; hasOne {
						if other, hasOther := trans.PluralForms[i18n.PluralOther]; hasOther {
							record[2+i] = fmt.Sprintf("%s|%s", one, other)
						} else {
							record[2+i] = trans.Value
						}
					} else {
						record[2+i] = trans.Value
					}
				} else {
					record[2+i] = trans.Value
				}
			}
		}
		
		// Add metadata
		record[len(headers)-4] = fmt.Sprintf("%t", isHTML)
		record[len(headers)-3] = fmt.Sprintf("%t", isMarkdown)
		record[len(headers)-2] = string(status)
		record[len(headers)-1] = comment
		
		if err := writer.Write(record); err != nil {
			return err
		}
	}
	
	return nil
}

// Get retrieves a translation
func (b *CSVBackend) Get(key, lang string) (*i18n.Translation, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	if keyTrans, ok := b.data[key]; ok {
		if trans, ok := keyTrans[lang]; ok {
			return trans, nil
		}
	}
	
	return nil, fmt.Errorf("translation not found: %s[%s]", key, lang)
}

// Set stores a translation
func (b *CSVBackend) Set(key, lang string, trans *i18n.Translation) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	// Ensure language is tracked
	if !b.hasLanguage(lang) {
		b.languages = append(b.languages, lang)
	}
	
	// Initialize key map if needed
	if _, exists := b.data[key]; !exists {
		b.data[key] = make(map[string]*i18n.Translation)
	}
	
	// Store translation
	trans.Key = key
	trans.Language = lang
	trans.UpdatedAt = time.Now()
	b.data[key][lang] = trans
	
	// Auto-save if enabled
	if b.options.AutoSave {
		return b.save()
	}
	
	return nil
}

// GetAll retrieves all translations for a language
func (b *CSVBackend) GetAll(lang string) (map[string]*i18n.Translation, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	result := make(map[string]*i18n.Translation)
	
	for key, translations := range b.data {
		if trans, ok := translations[lang]; ok {
			result[key] = trans
		}
	}
	
	return result, nil
}

// Delete removes a translation
func (b *CSVBackend) Delete(key, lang string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	if keyTrans, ok := b.data[key]; ok {
		delete(keyTrans, lang)
		
		// Remove key entirely if no translations left
		if len(keyTrans) == 0 {
			delete(b.data, key)
		}
	}
	
	if b.options.AutoSave {
		return b.save()
	}
	
	return nil
}

// ListLanguages returns all languages
func (b *CSVBackend) ListLanguages() ([]string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	result := make([]string, len(b.languages))
	copy(result, b.languages)
	return result, nil
}

// ListKeys returns all translation keys
func (b *CSVBackend) ListKeys() ([]string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	keys := make([]string, 0, len(b.data))
	for key := range b.data {
		keys = append(keys, key)
	}
	
	return keys, nil
}

// Stats returns storage statistics
func (b *CSVBackend) Stats() (*i18n.StorageStats, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	totalTranslations := 0
	for _, translations := range b.data {
		totalTranslations += len(translations)
	}
	
	info, _ := os.Stat(b.filename)
	var lastMod time.Time
	if info != nil {
		lastMod = info.ModTime()
	}
	
	return &i18n.StorageStats{
		TotalTranslations: totalTranslations,
		TotalKeys:         len(b.data),
		TotalLanguages:    len(b.languages),
		LastModified:      lastMod,
	}, nil
}

// BulkGet retrieves multiple translations
func (b *CSVBackend) BulkGet(keys []string, lang string) (map[string]*i18n.Translation, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	result := make(map[string]*i18n.Translation)
	
	for _, key := range keys {
		if keyTrans, ok := b.data[key]; ok {
			if trans, ok := keyTrans[lang]; ok {
				result[key] = trans
			}
		}
	}
	
	return result, nil
}

// BulkSet stores multiple translations
func (b *CSVBackend) BulkSet(translations map[string]*i18n.Translation) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	for key, trans := range translations {
		// Ensure language is tracked
		if !b.hasLanguage(trans.Language) {
			b.languages = append(b.languages, trans.Language)
		}
		
		// Initialize key map if needed
		if _, exists := b.data[key]; !exists {
			b.data[key] = make(map[string]*i18n.Translation)
		}
		
		trans.UpdatedAt = time.Now()
		b.data[key][trans.Language] = trans
	}
	
	if b.options.AutoSave {
		return b.save()
	}
	
	return nil
}

// Query searches for translations
func (b *CSVBackend) Query(filters i18n.QueryFilters) ([]*i18n.TranslationSet, error) {
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
					if strings.Contains(strings.ToLower(trans.Value), searchLower) {
						found = true
						break
					}
				}
			}
			
			if !found {
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
			set.Translations[lang] = trans
			if trans.Context != "" && set.Context == "" {
				set.Context = trans.Context
			}
		}
		
		results = append(results, set)
	}
	
	// Apply pagination
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

// hasLanguage checks if a language is tracked
func (b *CSVBackend) hasLanguage(lang string) bool {
	for _, l := range b.languages {
		if l == lang {
			return true
		}
	}
	return false
}

// copyFile creates a backup copy
func (b *CSVBackend) copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()
	
	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()
	
	_, err = io.Copy(destination, source)
	return err
}