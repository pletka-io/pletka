package registry

import (
	"net/http"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/vocabconnector/local"
	"github.com/pletka-io/pletka/pkg/weave/vocabconnector/vocabservice"
)

func TestForVocabularyReturnsTheServiceConnectorWhenConfigured(t *testing.T) {
	r := New(http.DefaultClient, "https://vocab.example.test")
	got := r.ForVocabulary(&domain.Vocabulary{
		ConnectorType: "vocabservice",
		Config:        []byte(`{"vocab":"aat"}`),
	})
	if _, ok := got.(*vocabservice.Connector); !ok {
		t.Fatalf("got %T, want *vocabservice.Connector", got)
	}
}

// With no service configured the row cannot work, so fall back to local rather
// than hand back a connector that fails every call.
func TestForVocabularyFallsBackToLocalWithNoServiceConfigured(t *testing.T) {
	r := New(http.DefaultClient, "")
	got := r.ForVocabulary(&domain.Vocabulary{
		ConnectorType: "vocabservice",
		Config:        []byte(`{"vocab":"aat"}`),
	})
	if _, ok := got.(*local.Connector); !ok {
		t.Fatalf("got %T, want the local connector", got)
	}
}

// The connector type is matched case-insensitively and trimmed, as the others are.
func TestForVocabularyAcceptsUntidyConnectorType(t *testing.T) {
	r := New(http.DefaultClient, "https://vocab.example.test")
	got := r.ForVocabulary(&domain.Vocabulary{ConnectorType: "  VocabService ", Config: []byte(`{"vocab":"aat"}`)})
	if _, ok := got.(*vocabservice.Connector); !ok {
		t.Fatalf("got %T, want *vocabservice.Connector", got)
	}
}
