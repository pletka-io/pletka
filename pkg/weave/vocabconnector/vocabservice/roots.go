package vocabservice

import (
	"context"
	"fmt"
	"maps"
	"net/url"
	"strconv"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/vocabconnector"
)

// rootItem is a curated browse root (contract v2.6): the one item shape plus
// the curated display Label, the Browse hint, and an operator Note. The
// curated Label is a separate, hand-maintained display name ("Gender") that
// differs from the concept's own prefLabel ("gender identity"), so it wins
// over labelFor when present.
type rootItem struct {
	item
	Label  map[string]string `json:"label"`
	Browse string            `json:"browse"`
	Note   string            `json:"note"`
}

type rootsResponse struct {
	Roots []rootItem `json:"roots"`
}

// childItem is one entry of a children listing: the item shape plus whether it
// has children of its own, so a tree picker knows to show an expander without
// a second call.
type childItem struct {
	item
	HasChildren *bool `json:"hasChildren,omitempty"`
}

type childrenResponse struct {
	Children []childItem `json:"children"`
}

const (
	defaultChildrenLimit = 100
	maxChildrenLimit     = 1000
)

// Roots returns the vocabulary's curated browse roots, each carrying its
// Browse hint, Note and DescendantsTotal (requested via descendants=1). Empty
// (not an error) for a mount with no curated roots.
func (c *Connector) Roots(ctx context.Context, lang string) ([]vocabconnector.Entry, error) {
	if c.cfg.Vocab == "" {
		return nil, fmt.Errorf("roots: %w: %w", vocabconnector.ErrDegraded, errNoVocab)
	}
	params := url.Values{}
	params.Set("lang", firstNonEmpty(lang, c.cfg.Lang, "en"))
	params.Set("descendants", "1")

	var body rootsResponse
	if err := c.get(ctx, "/vocab/"+url.PathEscape(c.cfg.Vocab)+"/roots", params, &body); err != nil {
		return nil, fmt.Errorf("roots %q: %w: %w", c.cfg.Vocab, vocabconnector.ErrDegraded, err)
	}
	out := make([]vocabconnector.Entry, 0, len(body.Roots))
	for _, r := range body.Roots {
		entry := entryFromItem(r.item, nil, nil)
		entry.Browse = r.Browse
		entry.Note = r.Note
		// The curated display name wins over the concept's own prefLabel.
		if curated := translationsFrom(r.Label); len(curated) > 0 {
			entry.Label = curated
		}
		out = append(out, entry)
	}
	return out, nil
}

// Children returns the direct children of one concept (depth=1), paginated by
// limit/offset, each carrying HasChildren and DescendantsTotal. Empty (not an
// error) when the concept is a leaf.
func (c *Connector) Children(ctx context.Context, rawConceptID, lang string, limit, offset int) ([]vocabconnector.Entry, error) {
	id := conceptID(rawConceptID)
	if id == "" {
		return nil, fmt.Errorf("children: no concept id")
	}
	if c.cfg.Vocab == "" {
		return nil, fmt.Errorf("children %q: %w: %w", rawConceptID, vocabconnector.ErrDegraded, errNoVocab)
	}
	switch {
	case limit <= 0:
		limit = defaultChildrenLimit
	case limit > maxChildrenLimit:
		limit = maxChildrenLimit
	}
	if offset < 0 {
		offset = 0
	}
	params := url.Values{}
	params.Set("lang", firstNonEmpty(lang, c.cfg.Lang, "en"))
	params.Set("descendants", "1")
	params.Set("limit", strconv.Itoa(limit))
	params.Set("offset", strconv.Itoa(offset))

	var body childrenResponse
	endpoint := "/vocab/" + url.PathEscape(c.cfg.Vocab) + "/children/" + url.PathEscape(id)
	if err := c.get(ctx, endpoint, params, &body); err != nil {
		return nil, fmt.Errorf("children %q: %w: %w", rawConceptID, vocabconnector.ErrDegraded, err)
	}
	out := make([]vocabconnector.Entry, 0, len(body.Children))
	for _, child := range body.Children {
		entry := entryFromItem(child.item, nil, nil)
		entry.HasChildren = child.HasChildren
		out = append(out, entry)
	}
	return out, nil
}

// translationsFrom copies a plain language→text map into a Translations,
// returning nil for an empty map so Label stays unset rather than non-nil-empty.
func translationsFrom(m map[string]string) domain.Translations {
	if len(m) == 0 {
		return nil
	}
	t := make(domain.Translations, len(m))
	maps.Copy(t, m)
	return t
}
