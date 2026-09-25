package vocabservice

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/vocabconnector"
)

// defaultTimeout bounds one call. Authoring must never block on a remote
// service, so a call that overruns is canceled and reported as degraded.
const defaultTimeout = 3 * time.Second

const (
	defaultLimit = 50
	maxLimit     = 100
)

// Config is the per-vocabulary configuration stored in weave_vocabularies.config.
// BaseURL is filled in by the registry from the instance config, never from the
// row: one service per instance is the point of the design.
type Config struct {
	BaseURL string `json:"base_url,omitempty"`
	Vocab   string `json:"vocab,omitempty"`
	Lang    string `json:"lang,omitempty"`
	// Timeout bounds a single call; <= 0 falls back to defaultTimeout.
	Timeout time.Duration `json:"timeout,omitempty"`
}

// Connector speaks the vocabulary-service contract for one configured
// vocabulary (aat, tgn, ulan, or any future mount) over HTTP.
type Connector struct {
	cfg    Config
	client *http.Client
}

var _ vocabconnector.Connector = (*Connector)(nil)

// New builds a Connector for cfg. A nil client falls back to
// http.DefaultClient.
func New(cfg Config, client *http.Client) *Connector {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.Vocab = strings.TrimSpace(cfg.Vocab)
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &Connector{cfg: cfg, client: client}
}

// hitText decodes a field that is a single string on the /suggest response
// (one hit, already resolved to one language) but a language-keyed object on
// the /concept response (the whole concept, every language it has). prefLabel
// and parentString both take this shape; broader does not, so it stays a
// plain *string below.
type hitText struct {
	text   string
	byLang map[string]string
}

// UnmarshalJSON implements json.Unmarshaler, choosing the plain-string or
// language-map decode by sniffing the first byte of the value.
func (h *hitText) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if data[0] == '"' {
		return json.Unmarshal(data, &h.text)
	}
	return json.Unmarshal(data, &h.byLang)
}

type suggestHit struct {
	URI          string   `json:"uri"`
	ID           string   `json:"id"`
	PrefLabel    hitText  `json:"prefLabel"`
	Lang         string   `json:"lang"`
	ParentString *hitText `json:"parentString"`
	Broader      *string  `json:"broader"`
}

type suggestResponse struct {
	Results []suggestHit `json:"results"`
}

type vocabListResponse struct {
	Vocabs []VocabularyInfo `json:"vocabs"`
}

// VocabularyInfo is one entry of the service's own listing, used when
// configuring which vocabulary a row should serve.
type VocabularyInfo struct {
	Name     string `json:"name"`
	Label    string `json:"label,omitempty"`
	Concepts int    `json:"concepts,omitempty"`
}

// Search asks the service for suggestions. A failure returns no entries and
// ErrDegraded: the vocabulary service decides that autocomplete degrades to an
// empty list rather than an error, and needs to be able to tell that apart
// from a genuine no-match.
func (c *Connector) Search(ctx context.Context, query string, opts vocabconnector.SearchOpts) ([]vocabconnector.Entry, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	lang := firstNonEmpty(opts.Lang, c.cfg.Lang, "en")
	limit := opts.Limit
	switch {
	case limit <= 0:
		limit = defaultLimit
	case limit > maxLimit:
		limit = maxLimit
	}
	params := url.Values{}
	params.Set("q", query)
	params.Set("lang", lang)
	params.Set("limit", strconv.Itoa(limit))
	if parent := strings.TrimSpace(opts.ParentURI); parent != "" {
		params.Set("under", conceptID(parent))
	}

	var body suggestResponse
	if err := c.get(ctx, "/vocab/"+url.PathEscape(c.cfg.Vocab)+"/suggest", params, &body); err != nil {
		return nil, fmt.Errorf("suggest %q: %w: %w", c.cfg.Vocab, vocabconnector.ErrDegraded, err)
	}
	out := make([]vocabconnector.Entry, 0, len(body.Results))
	for _, hit := range body.Results {
		out = append(out, hitToEntry(hit, lang))
	}
	return out, nil
}

// Fetch resolves one concept by URI. Unlike Search this is an explicit lookup,
// so a failure is an error the caller should see.
func (c *Connector) Fetch(ctx context.Context, uri string, opts vocabconnector.SearchOpts) (*vocabconnector.Entry, error) {
	id := conceptID(uri)
	if id == "" {
		return nil, fmt.Errorf("fetch %q: no concept id in uri", uri)
	}
	lang := firstNonEmpty(opts.Lang, c.cfg.Lang, "en")
	params := url.Values{}
	params.Set("lang", lang)

	var hit suggestHit
	endpoint := "/vocab/" + url.PathEscape(c.cfg.Vocab) + "/concept/" + url.PathEscape(id)
	if err := c.get(ctx, endpoint, params, &hit); err != nil {
		return nil, fmt.Errorf("fetch %q: %w", uri, err)
	}
	entry := hitToEntry(hit, lang)
	return &entry, nil
}

// Vocabularies lists what the service serves. Configuration calls this; a
// curator typing never does, so it is not cached.
func (c *Connector) Vocabularies(ctx context.Context) ([]VocabularyInfo, error) {
	var body vocabListResponse
	if err := c.get(ctx, "/vocab", nil, &body); err != nil {
		return nil, fmt.Errorf("list vocabularies: %w", err)
	}
	return body.Vocabs, nil
}

func (c *Connector) get(ctx context.Context, endpoint string, params url.Values, into any) error {
	if c.cfg.BaseURL == "" {
		return fmt.Errorf("no vocabulary service configured")
	}
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	target := c.cfg.BaseURL + endpoint
	if len(params) > 0 {
		target += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("call %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("call %s: status %d", endpoint, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return fmt.Errorf("decode %s: %w", endpoint, err)
	}
	return nil
}

func hitToEntry(hit suggestHit, lang string) vocabconnector.Entry {
	entry := vocabconnector.Entry{
		URI:        NormalizeURI(hit.URI),
		ExternalID: hit.ID,
	}
	// prefLabel is one string in the requested language on /suggest, but a
	// full language map on /concept: carry every language we were given
	// there rather than pick one arbitrarily.
	hitLang := lang
	if hit.PrefLabel.byLang != nil {
		labels := make(domain.Translations, len(hit.PrefLabel.byLang))
		for l, v := range hit.PrefLabel.byLang {
			labels[l] = v
		}
		entry.Label = labels
	} else {
		hitLang = firstNonEmpty(hit.Lang, lang)
		entry.Label = domain.Translations{hitLang: hit.PrefLabel.text}
	}
	if hit.Broader != nil {
		entry.BroaderURI = NormalizeURI(*hit.Broader)
	}
	if hit.ParentString != nil {
		chain := hit.ParentString.text
		if hit.ParentString.byLang != nil {
			chain = hit.ParentString.byLang[hitLang]
		}
		if chain != "" {
			entry.BroaderPath = SplitParentString(chain)
			entry.BroaderPathItems = ParentStringRefs(chain, hitLang)
			// parentString is nearest-first, so its first element IS the broader
			// concept; give that item the identity we already know.
			if len(entry.BroaderPathItems) > 0 && entry.BroaderURI != "" {
				entry.BroaderPathItems[0].URI = entry.BroaderURI
				entry.BroaderPathItems[0].ID = conceptID(entry.BroaderURI)
			}
		}
	}
	return entry
}

// conceptID takes the last path segment of a concept URI, which is the
// identifier the service addresses concepts by.
func conceptID(uri string) string {
	uri = strings.TrimRight(strings.TrimSpace(uri), "/")
	if uri == "" {
		return ""
	}
	return path.Base(uri)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
