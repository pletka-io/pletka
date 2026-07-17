package registry

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/vocabconnector"
	"github.com/pletka-io/pletka/pkg/weave/vocabconnector/aat"
	"github.com/pletka-io/pletka/pkg/weave/vocabconnector/csv"
	"github.com/pletka-io/pletka/pkg/weave/vocabconnector/local"
	"github.com/pletka-io/pletka/pkg/weave/vocabconnector/sparql"
)

type Registry struct {
	client *http.Client
}

func New(client *http.Client) *Registry {
	if client == nil {
		client = http.DefaultClient
	}
	return &Registry{client: client}
}

func (r *Registry) ForVocabulary(vocab *domain.Vocabulary) vocabconnector.Connector {
	if vocab == nil {
		return local.New()
	}
	switch strings.ToLower(strings.TrimSpace(vocab.ConnectorType)) {
	case "", "local":
		return local.New()
	case "aat":
		cfg := aat.Config{}
		_ = json.Unmarshal(vocab.Config, &cfg)
		if cfg.EndpointURL == "" {
			cfg.EndpointURL = "https://vocab.getty.edu/sparql"
		}
		return aat.New(cfg, r.client)
	case "sparql":
		return sparql.New()
	case "csv":
		return csv.New()
	default:
		return local.New()
	}
}
