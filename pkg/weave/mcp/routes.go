package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/weave/ontology/autocomplete"
	"github.com/pletka-io/pletka/pkg/weave/vocabulary"
)

// SliceID identifies this module for slice registration/logging.
const SliceID = "mcp"

// ProjectReader is the project surface the tools need.
type ProjectReader interface {
	ListVisible(ctx context.Context, opts ...domain.QueryOption) ([]*domain.Project, int64, error)
	Get(ctx context.Context, id string) (*domain.Project, error)
	CanRead(ctx context.Context, p *domain.Project) bool
	StatsForProjects(ctx context.Context, ids []string) (map[string]*domain.WeaveProjectStats, error)
}

// OntologyLinkReader lists a project's linked ontology versions.
type OntologyLinkReader interface {
	LinkedOntologies(ctx context.Context, projectID string) ([]domain.LinkedOntology, error)
}

// NamespaceReader lists a project's prefix→namespace bindings.
type NamespaceReader interface {
	ListForProject(ctx context.Context, projectID string) ([]*domain.NamespaceBinding, error)
}

// FieldReader / ModelReader / CollectionReader / CategoryReader are the
// entity surfaces the tools need; satisfied by the slice services.
type FieldReader interface {
	List(ctx context.Context, projectID string, opts ...domain.QueryOption) ([]*domain.Field, int64, error)
	GetByIdentifier(ctx context.Context, projectID, identifier string) (*domain.Field, error)
}

// ModelReader is the model entity surface the tools need.
type ModelReader interface {
	List(ctx context.Context, projectID string, opts ...domain.QueryOption) ([]*domain.Model, int64, error)
	Get(ctx context.Context, projectID, id string) (*domain.Model, error)
}

// CollectionReader is the collection entity surface the tools need.
type CollectionReader interface {
	List(ctx context.Context, projectID string, opts ...domain.QueryOption) ([]*domain.Collection, int64, error)
	Get(ctx context.Context, projectID, id string) (*domain.Collection, error)
}

// CategoryReader is the category entity surface the tools need.
type CategoryReader interface {
	List(ctx context.Context, projectID string, opts ...domain.QueryOption) ([]*domain.Category, error)
	Get(ctx context.Context, projectID, id string) (*domain.Category, error)
}

// Autocompleter is the ontology suggestion surface (ontology.Service).
type Autocompleter interface {
	GetSuggestions(ctx context.Context, req autocomplete.Request) ([]autocomplete.Suggestion, error)
}

// VocabularyReader lists a project's vocabularies and concept lists.
type VocabularyReader interface {
	ListProjectVocabularies(ctx context.Context, projectID string) ([]vocabulary.VocabularyView, error)
	ListProjectConceptLists(ctx context.Context, projectID string) ([]vocabulary.ConceptListView, error)
}

// ToolMetricsRecorder observes one MCP tool call. Implemented in pkg/app
// over the observability registry; nil-safe at every call site.
type ToolMetricsRecorder interface {
	ObserveToolCall(tool, outcome string, seconds float64)
}

// Host is the dependency surface for the MCP module.
type Host struct {
	APIKeys     auth.APIKeyVerifier
	Weave       domain.WeaveStore // reuse payload (detailview.BuildReuse)
	Projects    ProjectReader
	Ontologies  OntologyLinkReader
	Namespaces  NamespaceReader
	Fields      FieldReader
	Models      ModelReader
	Collections CollectionReader
	Categories  CategoryReader
	Ontology    Autocompleter
	Vocabulary  VocabularyReader
	Languages   []formschema.LanguageInfo
	Logger      *slog.Logger
	// Metrics records per-tool call outcomes; nil disables recording.
	Metrics ToolMetricsRecorder
}

// Validate reports missing required dependencies.
func (h Host) Validate() error {
	var missing []string
	if h.APIKeys == nil {
		missing = append(missing, "APIKeys")
	}
	if h.Weave == nil {
		missing = append(missing, "Weave")
	}
	if h.Projects == nil {
		missing = append(missing, "Projects")
	}
	if h.Ontologies == nil {
		missing = append(missing, "Ontologies")
	}
	if h.Namespaces == nil {
		missing = append(missing, "Namespaces")
	}
	if h.Fields == nil {
		missing = append(missing, "Fields")
	}
	if h.Models == nil {
		missing = append(missing, "Models")
	}
	if h.Collections == nil {
		missing = append(missing, "Collections")
	}
	if h.Categories == nil {
		missing = append(missing, "Categories")
	}
	if h.Ontology == nil {
		missing = append(missing, "Ontology")
	}
	if h.Vocabulary == nil {
		missing = append(missing, "Vocabulary")
	}
	if len(missing) > 0 {
		return fmt.Errorf("mcp host missing required dependencies: %s", strings.Join(missing, ", "))
	}
	return nil
}

// Mount attaches the MCP endpoint at /mcp behind API-key auth.
func Mount(parent chi.Router, h Host) {
	if err := h.Validate(); err != nil {
		panic(err) // wiring-time fail-fast, sanctioned for Mount
	}
	server := newServer(h)
	handler := sdk.NewStreamableHTTPHandler(
		func(*http.Request) *sdk.Server { return server },
		// Stateless is required: it makes each tool call's ctx derive from the
		// authenticated HTTP request, which is what lets slice services read the
		// AuthSnapshot from context. Do not switch to stateful sessions without
		// redesigning auth propagation.
		&sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
	parent.Group(func(r chi.Router) {
		r.Use(auth.RequireAPIKey(h.APIKeys, h.Weave, h.Logger))
		r.Handle("/mcp", handler)
	})
}
