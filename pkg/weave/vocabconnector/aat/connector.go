package aat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/vocabconnector"
)

// defaultTimeout bounds every AAT query. The remote Getty SPARQL endpoint is a
// third-party service that can be slow or down; authoring must never block on
// it (#3599), so a query that overruns this deadline is canceled.
const defaultTimeout = 3 * time.Second

type Config struct {
	EndpointURL string `json:"endpoint_url,omitempty"`
	// Timeout bounds a single query; <= 0 falls back to defaultTimeout.
	Timeout time.Duration `json:"timeout,omitempty"`
}

type Connector struct {
	cfg    Config
	client *http.Client
}

func New(cfg Config, client *http.Client) *Connector {
	if cfg.EndpointURL == "" {
		cfg.EndpointURL = "https://vocab.getty.edu/sparql"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &Connector{cfg: cfg, client: client}
}

// Search degrades gracefully: a transport error, timeout, or non-2xx status
// from the remote authority returns empty results with a nil error so
// autocomplete/authoring never blocks or fails when Getty is unreachable
// (#3599). Use Fetch for explicit by-URI resolution where an error matters.
func (c *Connector) Search(ctx context.Context, query string, opts vocabconnector.SearchOpts) ([]vocabconnector.Entry, error) {
	query = strings.TrimSpace(query)
	if query == "" && strings.TrimSpace(opts.ParentURI) == "" {
		return nil, nil
	}
	entries, err := c.query(ctx, searchQuery(query, opts.Lang, opts.Limit, opts.ParentURI), opts.Lang)
	if err != nil {
		//nolint:nilerr // deliberate: a failed/slow remote authority yields no suggestions, never an error (#3599)
		return nil, nil
	}
	return entries, nil
}

func (c *Connector) Fetch(ctx context.Context, uri string, opts vocabconnector.SearchOpts) (*vocabconnector.Entry, error) {
	uri = NormalizeURI(uri)
	if uri == "" {
		return nil, nil
	}
	entries, err := c.query(ctx, fetchQuery(uri, opts.Lang), opts.Lang)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	return &entries[0], nil
}

func (c *Connector) query(ctx context.Context, sparql, lang string) ([]vocabconnector.Entry, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	endpoint, err := url.Parse(c.cfg.EndpointURL)
	if err != nil {
		return nil, fmt.Errorf("parse AAT endpoint: %w", err)
	}
	q := endpoint.Query()
	q.Set("query", sparql)
	q.Set("format", "application/sparql-results+json")
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build AAT request: %w", err)
	}
	req.Header.Set("Accept", "application/sparql-results+json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query AAT: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("query AAT: unexpected status %d", resp.StatusCode)
	}

	var payload sparqlResults
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode AAT response: %w", err)
	}
	return entriesFromResults(payload, lang), nil
}

type sparqlResults struct {
	Results struct {
		Bindings []map[string]binding `json:"bindings"`
	} `json:"results"`
}

type binding struct {
	Type  string `json:"type"`
	Value string `json:"value"`
	Lang  string `json:"xml:lang,omitempty"`
}

func entriesFromResults(payload sparqlResults, lang string) []vocabconnector.Entry {
	if lang == "" {
		lang = "en"
	}
	seen := map[string]int{}
	out := make([]vocabconnector.Entry, 0, len(payload.Results.Bindings))
	for _, row := range payload.Results.Bindings {
		subject := NormalizeURI(row["subject"].Value)
		if subject == "" {
			continue
		}
		label := strings.TrimSpace(row["label"].Value)
		idx, ok := seen[subject]
		if !ok {
			out = append(out, vocabconnector.Entry{
				URI:        subject,
				Label:      domain.Translations{},
				ScopeNote:  domain.Translations{},
				BroaderURI: NormalizeURI(row["broader"].Value),
				ExternalID: path.Base(subject),
			})
			idx = len(out) - 1
			seen[subject] = idx
		}
		if label != "" {
			out[idx].Label[lang] = label
		}
		if note := strings.TrimSpace(row["scopeNote"].Value); note != "" {
			out[idx].ScopeNote[lang] = note
		}
		if parentString := strings.TrimSpace(row["parentString"].Value); parentString != "" {
			out[idx].BroaderPath = splitParentString(parentString)
			out[idx].BroaderPathItems = splitParentStringRefs(parentString, lang)
		} else if broaderLabel := strings.TrimSpace(row["broaderLabel"].Value); broaderLabel != "" {
			out[idx].BroaderPath = appendUnique(out[idx].BroaderPath, broaderLabel)
			out[idx].BroaderPathItems = appendUniqueRef(out[idx].BroaderPathItems, broaderRef(row["broader"].Value, broaderLabel, lang))
		}
		if len(out[idx].BroaderPathItems) > 0 {
			attachBroaderURI(out[idx].BroaderPathItems, row["broader"].Value, row["broaderLabel"].Value, lang)
		}
	}
	return out
}

func splitParentString(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 && strings.TrimSpace(value) != "" {
		return []string{strings.TrimSpace(value)}
	}
	return out
}

func splitParentStringRefs(value, lang string) []domain.VocabularyEntryRef {
	parts := splitParentString(value)
	out := make([]domain.VocabularyEntryRef, 0, len(parts))
	for _, part := range parts {
		out = append(out, domain.VocabularyEntryRef{Label: domain.Translations{lang: part}})
	}
	return out
}

func broaderRef(uri, label, lang string) domain.VocabularyEntryRef {
	uri = NormalizeURI(uri)
	ref := domain.VocabularyEntryRef{
		URI:   uri,
		Label: domain.Translations{lang: strings.TrimSpace(label)},
	}
	if uri != "" {
		ref.ID = path.Base(uri)
	}
	return ref
}

func attachBroaderURI(items []domain.VocabularyEntryRef, uri, label, lang string) {
	uri = NormalizeURI(uri)
	label = strings.TrimSpace(label)
	if uri == "" || label == "" {
		return
	}
	for i := range items {
		if strings.EqualFold(items[i].Label.Get(lang, ""), label) {
			items[i].URI = uri
			items[i].ID = path.Base(uri)
			return
		}
	}
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func appendUniqueRef(values []domain.VocabularyEntryRef, value domain.VocabularyEntryRef) []domain.VocabularyEntryRef {
	for _, existing := range values {
		if existing.URI != "" && existing.URI == value.URI {
			return values
		}
		if existing.URI == "" && value.URI == "" && existing.Label.Get("en", "") == value.Label.Get("en", "") {
			return values
		}
	}
	return append(values, value)
}
