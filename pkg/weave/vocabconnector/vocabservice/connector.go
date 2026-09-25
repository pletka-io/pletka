package vocabservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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

// errNoVocab means a row has no vocabulary name configured: it is the row
// that is broken, not the service, so Search/Fetch say so without calling the
// service. Search still wraps it in ErrDegraded — a row that cannot be
// searched is an incomplete result like any other — so a misconfigured row
// does set the degraded flag; the throttled log line carries this error's
// text, which is what keeps the row's own breakage diagnosable rather than
// looking like a service outage.
var errNoVocab = errors.New("vocabulary service: no vocabulary configured for this row")

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

// suggestHit is a suggest result: an item plus the fields that explain why it
// matched and where it sits in the hierarchy. The same shape decodes a
// concept fetch (Task 2) — /concept's extra fields (narrower, altLabel, …)
// are simply ignored by this decode, and matched/matchedLabel/score are
// absent there and so decode to their zero values.
type suggestHit struct {
	item
	Matched      string  `json:"matched"`
	MatchedLabel *string `json:"matchedLabel"`
	Broader      *string `json:"broader"`
	Parents      []item  `json:"parents"`
	Score        float64 `json:"score"`
}

type suggestResponse struct {
	Results []suggestHit `json:"results"`
}

// conceptResponse is the /concept/{id} decode: the one item shape plus the
// fields only a concept fetch carries. altLabel is decoded and not mapped —
// Entry has nowhere to put it, and inventing a home is out of scope for this
// task. narrower/matches/broaderOther are not represented here at all, so
// the decoder ignores them the same way it already ignores /suggest's
// matched/matchedLabel/score on this endpoint.
type conceptResponse struct {
	item
	AltLabel  map[string][]string `json:"altLabel"`
	ScopeNote map[string]string   `json:"scopeNote"`
	Broader   *string             `json:"broader"`
	Parents   []item              `json:"parents"`
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
	if c.cfg.Vocab == "" {
		return nil, fmt.Errorf("search %q: %w: %w", query, vocabconnector.ErrDegraded, errNoVocab)
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
		out = append(out, hitToEntry(hit))
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
	if c.cfg.Vocab == "" {
		return nil, fmt.Errorf("fetch %q: %w", uri, errNoVocab)
	}
	lang := firstNonEmpty(opts.Lang, c.cfg.Lang, "en")

	// concept/{id}?lang= picks the display language for the concept and every
	// nested item, resolved server-side (falls back to en, then the first
	// key, on an unknown tag) — so, unlike v1's assumption, this must be sent.
	params := url.Values{}
	params.Set("lang", lang)
	var resp conceptResponse
	endpoint := "/vocab/" + url.PathEscape(c.cfg.Vocab) + "/concept/" + url.PathEscape(id)
	if err := c.get(ctx, endpoint, params, &resp); err != nil {
		return nil, fmt.Errorf("fetch %q: %w", uri, err)
	}
	entry := conceptToEntry(resp)
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
		return decodeServiceError(endpoint, resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return fmt.Errorf("decode %s: %w", endpoint, err)
	}
	return nil
}

// hitToEntry maps one decoded suggest hit to an Entry. The URI is stored
// exactly as the service sent it: v2 promises the publisher's canonical IRI,
// never rewritten.
func hitToEntry(hit suggestHit) vocabconnector.Entry {
	return entryFromItem(hit.item, hit.Broader, hit.Parents)
}

// conceptToEntry maps a decoded concept fetch to an Entry: the same
// URI/Label/broader/parents mapping hitToEntry does, plus ScopeNote, which
// only concept/{id} carries. ScopeNote is narrowed by the same rule as
// Label (display language plus English) — see narrowByLang.
func conceptToEntry(resp conceptResponse) vocabconnector.Entry {
	entry := entryFromItem(resp.item, resp.Broader, resp.Parents)
	entry.ScopeNote = narrowByLang(resp.Lang, resp.ScopeNote)
	return entry
}

// entryFromItem builds the URI/ExternalID/Label/broader/parents fields
// shared by a suggest hit and a concept fetch.
func entryFromItem(it item, broader *string, parents []item) vocabconnector.Entry {
	entry := vocabconnector.Entry{
		URI:        it.URI,
		ExternalID: it.ID,
		Label:      labelFor(it),
	}
	if broader != nil {
		entry.BroaderURI = *broader
	}
	if len(parents) > 0 {
		entry.BroaderPath = make([]string, 0, len(parents))
		entry.BroaderPathItems = make([]domain.VocabularyEntryRef, 0, len(parents))
		for _, parent := range parents {
			entry.BroaderPath = append(entry.BroaderPath, labelText(parent))
			entry.BroaderPathItems = append(entry.BroaderPathItems, itemToRef(parent))
		}
	}
	return entry
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
