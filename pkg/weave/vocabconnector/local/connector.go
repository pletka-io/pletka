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
