package sparql

import (
	"context"

	"github.com/pletka-io/pletka/pkg/weave/vocabconnector"
)

type Connector struct{}

func New() *Connector {
	return &Connector{}
}

func (c *Connector) Search(context.Context, string, vocabconnector.SearchOpts) ([]vocabconnector.Entry, error) {
	return nil, vocabconnector.ErrNotImplemented
}

func (c *Connector) Fetch(context.Context, string, vocabconnector.SearchOpts) (*vocabconnector.Entry, error) {
	return nil, vocabconnector.ErrNotImplemented
}

// Roots is not implemented for the SPARQL connector.
func (c *Connector) Roots(context.Context, string) ([]vocabconnector.Entry, error) {
	return nil, vocabconnector.ErrNotImplemented
}

// Children is not implemented for the SPARQL connector.
func (c *Connector) Children(context.Context, string, string, int, int) ([]vocabconnector.Entry, error) {
	return nil, vocabconnector.ErrNotImplemented
}
