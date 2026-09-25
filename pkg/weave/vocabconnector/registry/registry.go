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
	"github.com/pletka-io/pletka/pkg/weave/vocabconnector/vocabservice"
)

// vocabServiceConnectorType is the domain.Vocabulary.ConnectorType value that
// routes a row to the vocabservice connector. Named so it is not a repeated
// string literal (also appears in registry_test.go).
const vocabServiceConnectorType = "vocabservice"

type Registry struct {
	client          *http.Client
	vocabServiceURL string
}

// New builds the connector registry. vocabServiceBaseURL is the one vocabulary
// service this instance talks to, from the instance config; empty means none is
// configured and rows asking for it fall back to local.
func New(client *http.Client, vocabServiceBaseURL string) *Registry {
	if client == nil {
		client = http.DefaultClient
	}
	return &Registry{client: client, vocabServiceURL: strings.TrimSpace(vocabServiceBaseURL)}
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
	case vocabServiceConnectorType:
		if r.vocabServiceURL == "" {
			// A row wants the service but this instance has none configured.
			// Fall back rather than return a connector that fails every call.
			return local.New()
		}
		cfg := vocabservice.Config{}
		_ = json.Unmarshal(vocab.Config, &cfg)
		cfg.BaseURL = r.vocabServiceURL
		return vocabservice.New(cfg, r.client)
	case "sparql":
		return sparql.New()
	case "csv":
		return csv.New()
	default:
		return local.New()
	}
}
