package local

import (
	"context"

	"github.com/pletka-io/pletka/pkg/weave/vocabconnector"
)

type Connector struct{}

func New() *Connector {
	return &Connector{}
}

func (c *Connector) Search(context.Context, string, vocabconnector.SearchOpts) ([]vocabconnector.Entry, error) {
	return nil, nil
}

func (c *Connector) Fetch(context.Context, string, vocabconnector.SearchOpts) (*vocabconnector.Entry, error) {
	return nil, nil
}

// Roots returns none: a local vocabulary has no service hierarchy. Empty, not
// an error — callers treat "no roots" as "fall back to plain search".
func (c *Connector) Roots(context.Context, string) ([]vocabconnector.Entry, error) {
	return nil, nil
}

// Children returns none: a local vocabulary has no service hierarchy.
func (c *Connector) Children(context.Context, string, string, int, int) ([]vocabconnector.Entry, error) {
	return nil, nil
}
