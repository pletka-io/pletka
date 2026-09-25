package vocabservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
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

// hitText decodes a field that is a single string on the /suggest response
// (one hit, already resolved to one language) but a language-keyed object on
// the /concept response (the whole concept, every language it has). prefLabel
// and parentString both take this shape; broader does not, so it stays a
// plain *string below. A value that is neither shape (not a JSON string, not
// a JSON object) is left to the underlying json.Unmarshal error. On Fetch
// that reaches the caller as a real error rather than an empty entry; on
// Search it does not, because Search wraps everything get() returns —
// decode included — as ErrDegraded.
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
	if c.cfg.Vocab == "" {
		return nil, fmt.Errorf("fetch %q: %w", uri, errNoVocab)
	}
	lang := firstNonEmpty(opts.Lang, c.cfg.Lang, "en")

	// concept/{id} is not documented to take lang (only suggest and children
	// are): it returns every language the concept has, and the client picks
	// one from that. Sending an undocumented parameter is a request a future
	// implementation may reject.
	var hit suggestHit
	endpoint := "/vocab/" + url.PathEscape(c.cfg.Vocab) + "/concept/" + url.PathEscape(id)
	if err := c.get(ctx, endpoint, nil, &hit); err != nil {
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

// hitToEntry maps one decoded hit/concept to an Entry. lang is the language
// already resolved for this call (opts.Lang, then the row's configured
// language, then "en").
func hitToEntry(hit suggestHit, lang string) vocabconnector.Entry {
	entry := vocabconnector.Entry{
		URI:        NormalizeURI(hit.URI),
		ExternalID: hit.ID,
	}
	// hitLang is the language the label ended up in. The ancestor chain below
	// starts its own resolution there, but does not stay pinned to it — see
	// resolveChain.
	hitLang := lang
	switch {
	case hit.PrefLabel.byLang != nil:
		// /concept: prefLabel is a language map covering everything the
		// concept has. Resolve one language — requested, then English, then
		// whichever the concept actually has, chosen deterministically — and
		// store only that language plus English (when it differs). Storing
		// the whole map would put the concept's entire language set (up to
		// ~200 for TGN) in a row every other path stores one language in.
		if value, resolvedLang, ok := resolveByLang(hit.PrefLabel.byLang, lang); ok {
			hitLang = resolvedLang
			labels := domain.Translations{resolvedLang: value}
			if !strings.EqualFold(resolvedLang, "en") {
				if enValue, enKey, found := lookupLang(hit.PrefLabel.byLang, "en"); found {
					labels[enKey] = enValue
				}
			}
			entry.Label = labels
		}
	case hit.PrefLabel.text != "":
		// /suggest: prefLabel is one string, already resolved by the service
		// to hit.Lang. A hit with no label at all (the contract allows it)
		// leaves entry.Label unset rather than {"<lang>": ""}.
		hitLang = firstNonEmpty(hit.Lang, lang)
		entry.Label = domain.Translations{hitLang: hit.PrefLabel.text}
	}
	if hit.Broader != nil {
		entry.BroaderURI = NormalizeURI(*hit.Broader)
	}
	// The chain falls back on its own — label language, then English, then
	// whichever language the concept has a chain in at all — rather than
	// being pinned to hitLang: a concept can have ancestors only in a
	// language its own label never resolved to, and dropping them there is
	// exactly the permanently-empty stored path this connector exists to
	// avoid. ParentStringRefs is stamped with the chain's own language below,
	// so a curator sees an honest "these ancestors are in English" rather
	// than none, and never a Dutch label with Spanish ancestors mislabeled
	// as Dutch.
	chain, chainLang := resolveChain(hit.ParentString, hitLang)
	if chain != "" {
		entry.BroaderPath = SplitParentString(chain)
		entry.BroaderPathItems = ParentStringRefs(chain, chainLang)
		// parentString is nearest-first, so its first element IS the broader
		// concept; give that item the identity we already know.
		if len(entry.BroaderPathItems) > 0 && entry.BroaderURI != "" {
			entry.BroaderPathItems[0].URI = entry.BroaderURI
			entry.BroaderPathItems[0].ID = conceptID(entry.BroaderURI)
		}
	}
	return entry
}

// resolveChain picks the ancestor chain and the language it actually came in.
// field is nil for a hit with no parentString at all (a facet root, or a
// /suggest hit that never had one). labelLang is where resolution starts —
// the label's own language — not where it must end: see the comment at the
// call site.
func resolveChain(field *hitText, labelLang string) (chain, chainLang string) {
	if field == nil {
		return "", labelLang
	}
	if field.byLang != nil {
		if value, resolvedLang, ok := resolveByLang(field.byLang, labelLang); ok {
			return value, resolvedLang
		}
		return "", labelLang
	}
	return field.text, labelLang
}

// lookupLang finds byLang's value for lang, matched case-insensitively: the
// service itself matches lang case-insensitively and echoes back its own
// spelling, so an exact map lookup on the caller's spelling can miss an entry
// that is really there. A key present with an empty string counts as no
// value for that language, not a hit: the contract's own documented shape is
// to omit a language entirely rather than send it empty, but treating an
// empty string as present would store an empty label and, because label and
// chain resolution share this lookup, silently drop an ancestor chain that
// exists in another language.
func lookupLang(byLang map[string]string, lang string) (value, key string, ok bool) {
	if v, exists := byLang[lang]; exists && v != "" {
		return v, lang, true
	}
	for k, v := range byLang {
		if v != "" && strings.EqualFold(k, lang) {
			return v, k, true
		}
	}
	return "", "", false
}

// resolveByLang picks which language a language-keyed field is read in:
// requested language first, then English, then whichever language the
// response actually has. That last case is resolved deterministically (the
// lexicographically smallest key), not by map iteration order, so the same
// response always resolves to the same language. ok is false only when
// byLang holds no non-empty value in any language (empty values are treated
// as absent, see lookupLang).
func resolveByLang(byLang map[string]string, lang string) (value, resolvedLang string, ok bool) {
	if v, k, found := lookupLang(byLang, lang); found {
		return v, k, true
	}
	if !strings.EqualFold(lang, "en") {
		if v, k, found := lookupLang(byLang, "en"); found {
			return v, k, true
		}
	}
	keys := make([]string, 0, len(byLang))
	for k, v := range byLang {
		if v != "" {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return "", "", false
	}
	sort.Strings(keys)
	return byLang[keys[0]], keys[0], true
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
